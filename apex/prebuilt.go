// Copyright (C) 2019 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apex

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"android/soong/android"
	"android/soong/java"
	"android/soong/provenance"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

var (
	extractMatchingApex = pctx.StaticRule(
		"extractMatchingApex",
		blueprint.RuleParams{
			Command: `rm -rf "$out" && ` +
				`${extract_apks} -o "${out}" -allow-prereleased=${allow-prereleased} ` +
				`-sdk-version=${sdk-version} -skip-sdk-check=${skip-sdk-check} -abis=${abis} ` +
				`-screen-densities=all -extract-single ` +
				`${in}`,
			CommandDeps: []string{"${extract_apks}"},
		},
		"abis", "allow-prereleased", "sdk-version", "skip-sdk-check")
)

type prebuilt interface {
	isForceDisabled() bool
	InstallFilename() string
}

type prebuiltCommon struct {
	android.ModuleBase
	prebuilt android.Prebuilt

	// Properties common to both prebuilt_apex and apex_set.
	prebuiltCommonProperties *PrebuiltCommonProperties

	installDir      android.InstallPath
	installFilename string
	installedFile   android.InstallPath
	outputApex      android.WritablePath

	// fragment for this apex for apexkeys.txt
	apexKeysPath android.WritablePath

	// A list of apexFile objects created in prebuiltCommon.initApexFilesForAndroidMk which are used
	// to create make modules in prebuiltCommon.AndroidMkEntries.
	apexFilesForAndroidMk []apexFile

	// Installed locations of symlinks for backward compatibility.
	compatSymlinks android.InstallPaths

	hostRequired        []string
	requiredModuleNames []string
}

type sanitizedPrebuilt interface {
	hasSanitizedSource(sanitizer string) bool
}

type PrebuiltCommonProperties struct {
	SelectedApexProperties

	// Canonical name of this APEX. Used to determine the path to the activated APEX on
	// device (/apex/<apex_name>). If unspecified, follows the name property.
	Apex_name *string

	ForceDisable bool `blueprint:"mutated"`

	// whether the extracted apex file is installable.
	Installable *bool

	// optional name for the installed apex. If unspecified, name of the
	// module is used as the file name
	Filename *string

	// names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string

	// List of java libraries that are embedded inside this prebuilt APEX bundle and for which this
	// APEX bundle will create an APEX variant and provide dex implementation jars for use by
	// dexpreopt and boot jars package check.
	Exported_java_libs []string

	// List of bootclasspath fragments inside this prebuilt APEX bundle and for which this APEX
	// bundle will create an APEX variant.
	Exported_bootclasspath_fragments []string

	// List of systemserverclasspath fragments inside this prebuilt APEX bundle and for which this
	// APEX bundle will create an APEX variant.
	Exported_systemserverclasspath_fragments []string
}

// initPrebuiltCommon initializes the prebuiltCommon structure and performs initialization of the
// module that is common to Prebuilt and ApexSet.
func (p *prebuiltCommon) initPrebuiltCommon(module android.Module, properties *PrebuiltCommonProperties) {
	p.prebuiltCommonProperties = properties
	android.InitSingleSourcePrebuiltModule(module.(android.PrebuiltInterface), properties, "Selected_apex")
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
}

func (p *prebuiltCommon) ApexVariationName() string {
	return proptools.StringDefault(p.prebuiltCommonProperties.Apex_name, p.ModuleBase.BaseModuleName())
}

func (p *prebuiltCommon) Prebuilt() *android.Prebuilt {
	return &p.prebuilt
}

func (p *prebuiltCommon) isForceDisabled() bool {
	return p.prebuiltCommonProperties.ForceDisable
}

func (p *prebuiltCommon) checkForceDisable(ctx android.ModuleContext) bool {
	forceDisable := false

	// Force disable the prebuilts when we are doing unbundled build. We do unbundled build
	// to build the prebuilts themselves.
	forceDisable = forceDisable || ctx.Config().UnbundledBuild()

	// b/137216042 don't use prebuilts when address sanitizer is on, unless the prebuilt has a sanitized source
	sanitized := ctx.Module().(sanitizedPrebuilt)
	forceDisable = forceDisable || (android.InList("address", ctx.Config().SanitizeDevice()) && !sanitized.hasSanitizedSource("address"))
	forceDisable = forceDisable || (android.InList("hwaddress", ctx.Config().SanitizeDevice()) && !sanitized.hasSanitizedSource("hwaddress"))

	if forceDisable && p.prebuilt.SourceExists() {
		p.prebuiltCommonProperties.ForceDisable = true
		return true
	}
	return false
}

func (p *prebuiltCommon) InstallFilename() string {
	return proptools.StringDefault(p.prebuiltCommonProperties.Filename, p.BaseModuleName()+imageApexSuffix)
}

func (p *prebuiltCommon) Name() string {
	return p.prebuilt.Name(p.ModuleBase.Name())
}

func (p *prebuiltCommon) Overrides() []string {
	return p.prebuiltCommonProperties.Overrides
}

func (p *prebuiltCommon) installable() bool {
	return proptools.BoolDefault(p.prebuiltCommonProperties.Installable, true)
}

// initApexFilesForAndroidMk initializes the prebuiltCommon.apexFilesForAndroidMk field from the
// modules that this depends upon.
func (p *prebuiltCommon) initApexFilesForAndroidMk(ctx android.ModuleContext) {
	// Walk the dependencies of this module looking for the java modules that it exports.
	ctx.WalkDeps(func(child, parent android.Module) bool {
		tag := ctx.OtherModuleDependencyTag(child)

		name := android.RemoveOptionalPrebuiltPrefix(ctx.OtherModuleName(child))
		if java.IsBootclasspathFragmentContentDepTag(tag) ||
			java.IsSystemServerClasspathFragmentContentDepTag(tag) || tag == exportedJavaLibTag {
			// If the exported java module provides a dex jar path then add it to the list of apexFiles.
			path := child.(interface {
				DexJarBuildPath() java.OptionalDexJarPath
			}).DexJarBuildPath()
			if path.IsSet() {
				af := apexFile{
					module:              child,
					moduleDir:           ctx.OtherModuleDir(child),
					androidMkModuleName: name,
					builtFile:           path.Path(),
					class:               javaSharedLib,
				}
				if module, ok := child.(java.DexpreopterInterface); ok {
					for _, install := range module.DexpreoptBuiltInstalledForApex() {
						af.requiredModuleNames = append(af.requiredModuleNames, install.FullModuleName())
					}
				}
				p.apexFilesForAndroidMk = append(p.apexFilesForAndroidMk, af)
			}
		} else if tag == exportedBootclasspathFragmentTag {
			_, ok := child.(*java.PrebuiltBootclasspathFragmentModule)
			if !ok {
				ctx.PropertyErrorf("exported_bootclasspath_fragments", "%q is not a prebuilt_bootclasspath_fragment module", name)
				return false
			}
			// Visit the children of the bootclasspath_fragment.
			return true
		} else if tag == exportedSystemserverclasspathFragmentTag {
			// Visit the children of the systemserver_fragment.
			return true
		}

		return false
	})
}

func (p *prebuiltCommon) addRequiredModules(entries *android.AndroidMkEntries) {
	for _, fi := range p.apexFilesForAndroidMk {
		entries.AddStrings("LOCAL_REQUIRED_MODULES", fi.requiredModuleNames...)
		entries.AddStrings("LOCAL_TARGET_REQUIRED_MODULES", fi.targetRequiredModuleNames...)
		entries.AddStrings("LOCAL_HOST_REQUIRED_MODULES", fi.hostRequiredModuleNames...)
	}
	entries.AddStrings("LOCAL_REQUIRED_MODULES", p.requiredModuleNames...)
}

func (p *prebuiltCommon) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := []android.AndroidMkEntries{
		{
			Class:         "ETC",
			OutputFile:    android.OptionalPathForPath(p.outputApex),
			Include:       "$(BUILD_PREBUILT)",
			Host_required: p.hostRequired,
			ExtraEntries: []android.AndroidMkExtraEntriesFunc{
				func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
					entries.SetString("LOCAL_MODULE_PATH", p.installDir.String())
					entries.SetString("LOCAL_MODULE_STEM", p.installFilename)
					entries.SetPath("LOCAL_SOONG_INSTALLED_MODULE", p.installedFile)
					entries.SetString("LOCAL_SOONG_INSTALL_PAIRS", p.outputApex.String()+":"+p.installedFile.String())
					entries.AddStrings("LOCAL_SOONG_INSTALL_SYMLINKS", p.compatSymlinks.Strings()...)
					entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !p.installable())
					entries.AddStrings("LOCAL_OVERRIDES_MODULES", p.prebuiltCommonProperties.Overrides...)
					entries.SetString("LOCAL_APEX_KEY_PATH", p.apexKeysPath.String())
					p.addRequiredModules(entries)
				},
			},
		},
	}

	// Iterate over the apexFilesForAndroidMk list and create an AndroidMkEntries struct for each
	// file. This provides similar behavior to that provided in apexBundle.AndroidMk() as it makes the
	// apex specific variants of the exported java modules available for use from within make.
	apexName := p.BaseModuleName()
	for _, fi := range p.apexFilesForAndroidMk {
		entries := p.createEntriesForApexFile(fi, apexName)
		entriesList = append(entriesList, entries)
	}

	return entriesList
}

// createEntriesForApexFile creates an AndroidMkEntries for the supplied apexFile
func (p *prebuiltCommon) createEntriesForApexFile(fi apexFile, apexName string) android.AndroidMkEntries {
	moduleName := fi.androidMkModuleName + "." + apexName
	entries := android.AndroidMkEntries{
		Class:        fi.class.nameInMake(),
		OverrideName: moduleName,
		OutputFile:   android.OptionalPathForPath(fi.builtFile),
		Include:      "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", p.installDir.String())
				entries.SetString("LOCAL_SOONG_INSTALLED_MODULE", filepath.Join(p.installDir.String(), fi.stem()))
				entries.SetString("LOCAL_SOONG_INSTALL_PAIRS",
					fi.builtFile.String()+":"+filepath.Join(p.installDir.String(), fi.stem()))

				// soong_java_prebuilt.mk sets LOCAL_MODULE_SUFFIX := .jar  Therefore
				// we need to remove the suffix from LOCAL_MODULE_STEM, otherwise
				// we will have foo.jar.jar
				entries.SetString("LOCAL_MODULE_STEM", strings.TrimSuffix(fi.stem(), ".jar"))
				entries.SetString("LOCAL_SOONG_DEX_JAR", fi.builtFile.String())
				entries.SetString("LOCAL_DEX_PREOPT", "false")
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string) {
				// m <module_name> will build <module_name>.<apex_name> as well.
				if fi.androidMkModuleName != moduleName {
					fmt.Fprintf(w, ".PHONY: %s\n", fi.androidMkModuleName)
					fmt.Fprintf(w, "%s: %s\n", fi.androidMkModuleName, moduleName)
				}
			},
		},
	}
	return entries
}

// prebuiltApexModuleCreator defines the methods that need to be implemented by prebuilt_apex and
// apex_set in order to create the modules needed to provide access to the prebuilt .apex file.
type prebuiltApexModuleCreator interface {
	createPrebuiltApexModules(ctx android.TopDownMutatorContext)
}

// prebuiltApexModuleCreatorMutator is the mutator responsible for invoking the
// prebuiltApexModuleCreator's createPrebuiltApexModules method.
//
// It is registered as a pre-arch mutator as it must run after the ComponentDepsMutator because it
// will need to access dependencies added by that (exported modules) but must run before the
// DepsMutator so that the deapexer module it creates can add dependencies onto itself from the
// exported modules.
func prebuiltApexModuleCreatorMutator(ctx android.TopDownMutatorContext) {
	module := ctx.Module()
	if creator, ok := module.(prebuiltApexModuleCreator); ok {
		creator.createPrebuiltApexModules(ctx)
	}
}

func (p *prebuiltCommon) hasExportedDeps() bool {
	return len(p.prebuiltCommonProperties.Exported_java_libs) > 0 ||
		len(p.prebuiltCommonProperties.Exported_bootclasspath_fragments) > 0 ||
		len(p.prebuiltCommonProperties.Exported_systemserverclasspath_fragments) > 0
}

// prebuiltApexContentsDeps adds dependencies onto the prebuilt apex module's contents.
func (p *prebuiltCommon) prebuiltApexContentsDeps(ctx android.BottomUpMutatorContext) {
	module := ctx.Module()

	for _, dep := range p.prebuiltCommonProperties.Exported_java_libs {
		prebuiltDep := android.PrebuiltNameFromSource(dep)
		ctx.AddDependency(module, exportedJavaLibTag, prebuiltDep)
	}

	for _, dep := range p.prebuiltCommonProperties.Exported_bootclasspath_fragments {
		prebuiltDep := android.PrebuiltNameFromSource(dep)
		ctx.AddDependency(module, exportedBootclasspathFragmentTag, prebuiltDep)
	}

	for _, dep := range p.prebuiltCommonProperties.Exported_systemserverclasspath_fragments {
		prebuiltDep := android.PrebuiltNameFromSource(dep)
		ctx.AddDependency(module, exportedSystemserverclasspathFragmentTag, prebuiltDep)
	}
}

// Implements android.DepInInSameApex
func (p *prebuiltCommon) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	tag := ctx.OtherModuleDependencyTag(dep)
	_, ok := tag.(exportedDependencyTag)
	return ok
}

// apexInfoMutator marks any modules for which this apex exports a file as requiring an apex
// specific variant and checks that they are supported.
//
// The apexMutator will ensure that the ApexInfo objects passed to BuildForApex(ApexInfo) are
// associated with the apex specific variant using the ApexInfoProvider for later retrieval.
//
// Unlike the source apex module type the prebuilt_apex module type cannot share compatible variants
// across prebuilt_apex modules. That is because there is no way to determine whether two
// prebuilt_apex modules that export files for the same module are compatible. e.g. they could have
// been built from different source at different times or they could have been built with different
// build options that affect the libraries.
//
// While it may be possible to provide sufficient information to determine whether two prebuilt_apex
// modules were compatible it would be a lot of work and would not provide much benefit for a couple
// of reasons:
//   - The number of prebuilt_apex modules that will be exporting files for the same module will be
//     low as the prebuilt_apex only exports files for the direct dependencies that require it and
//     very few modules are direct dependencies of multiple prebuilt_apex modules, e.g. there are a
//     few com.android.art* apex files that contain the same contents and could export files for the
//     same modules but only one of them needs to do so. Contrast that with source apex modules which
//     need apex specific variants for every module that contributes code to the apex, whether direct
//     or indirect.
//   - The build cost of a prebuilt_apex variant is generally low as at worst it will involve some
//     extra copying of files. Contrast that with source apex modules that has to build each variant
//     from source.
func (p *prebuiltCommon) apexInfoMutator(mctx android.TopDownMutatorContext) {

	// Collect direct dependencies into contents.
	contents := make(map[string]android.ApexMembership)

	// Collect the list of dependencies.
	var dependencies []android.ApexModule
	mctx.WalkDeps(func(child, parent android.Module) bool {
		// If the child is not in the same apex as the parent then exit immediately and do not visit
		// any of the child's dependencies.
		if !android.IsDepInSameApex(mctx, parent, child) {
			return false
		}

		tag := mctx.OtherModuleDependencyTag(child)
		depName := mctx.OtherModuleName(child)
		if exportedTag, ok := tag.(exportedDependencyTag); ok {
			propertyName := exportedTag.name

			// It is an error if the other module is not a prebuilt.
			if !android.IsModulePrebuilt(child) {
				mctx.PropertyErrorf(propertyName, "%q is not a prebuilt module", depName)
				return false
			}

			// It is an error if the other module is not an ApexModule.
			if _, ok := child.(android.ApexModule); !ok {
				mctx.PropertyErrorf(propertyName, "%q is not usable within an apex", depName)
				return false
			}
		}

		// Ignore any modules that do not implement ApexModule as they cannot have an APEX specific
		// variant.
		if _, ok := child.(android.ApexModule); !ok {
			return false
		}

		// Strip off the prebuilt_ prefix if present before storing content to ensure consistent
		// behavior whether there is a corresponding source module present or not.
		depName = android.RemoveOptionalPrebuiltPrefix(depName)

		// Remember if this module was added as a direct dependency.
		direct := parent == mctx.Module()
		contents[depName] = contents[depName].Add(direct)

		// Add the module to the list of dependencies that need to have an APEX variant.
		dependencies = append(dependencies, child.(android.ApexModule))

		return true
	})

	// Create contents for the prebuilt_apex and store it away for later use.
	apexContents := android.NewApexContents(contents)
	mctx.SetProvider(ApexBundleInfoProvider, ApexBundleInfo{
		Contents: apexContents,
	})

	// Create an ApexInfo for the prebuilt_apex.
	apexVariationName := p.ApexVariationName()
	apexInfo := android.ApexInfo{
		ApexVariationName: apexVariationName,
		InApexVariants:    []string{apexVariationName},
		InApexModules:     []string{p.ModuleBase.BaseModuleName()}, // BaseModuleName() to avoid the prebuilt_ prefix.
		ApexContents:      []*android.ApexContents{apexContents},
		ForPrebuiltApex:   true,
	}

	// Mark the dependencies of this module as requiring a variant for this module.
	for _, am := range dependencies {
		am.BuildForApex(apexInfo)
	}
}

// prebuiltApexSelectorModule is a private module type that is only created by the prebuilt_apex
// module. It selects the apex to use and makes it available for use by prebuilt_apex and the
// deapexer.
type prebuiltApexSelectorModule struct {
	android.ModuleBase

	apexFileProperties ApexFileProperties

	inputApex android.Path
}

func privateApexSelectorModuleFactory() android.Module {
	module := &prebuiltApexSelectorModule{}
	module.AddProperties(
		&module.apexFileProperties,
	)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func (p *prebuiltApexSelectorModule) Srcs() android.Paths {
	return android.Paths{p.inputApex}
}

func (p *prebuiltApexSelectorModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.inputApex = android.SingleSourcePathFromSupplier(ctx, p.apexFileProperties.prebuiltApexSelector, "src")
}

type Prebuilt struct {
	prebuiltCommon

	properties PrebuiltProperties

	inputApex android.Path

	provenanceMetaDataFile android.OutputPath
}

type ApexFileProperties struct {
	// the path to the prebuilt .apex file to import.
	//
	// This cannot be marked as `android:"arch_variant"` because the `prebuilt_apex` is only mutated
	// for android_common. That is so that it will have the same arch variant as, and so be compatible
	// with, the source `apex` module type that it replaces.
	Src  *string `android:"path"`
	Arch struct {
		Arm struct {
			Src *string `android:"path"`
		}
		Arm64 struct {
			Src *string `android:"path"`
		}
		Riscv64 struct {
			Src *string `android:"path"`
		}
		X86 struct {
			Src *string `android:"path"`
		}
		X86_64 struct {
			Src *string `android:"path"`
		}
	}
}

// prebuiltApexSelector selects the correct prebuilt APEX file for the build target.
//
// The ctx parameter can be for any module not just the prebuilt module so care must be taken not
// to use methods on it that are specific to the current module.
//
// See the ApexFileProperties.Src property.
func (p *ApexFileProperties) prebuiltApexSelector(ctx android.BaseModuleContext, prebuilt android.Module) []string {
	multiTargets := prebuilt.MultiTargets()
	if len(multiTargets) != 1 {
		ctx.OtherModuleErrorf(prebuilt, "compile_multilib shouldn't be \"both\" for prebuilt_apex")
		return nil
	}
	var src string
	switch multiTargets[0].Arch.ArchType {
	case android.Arm:
		src = String(p.Arch.Arm.Src)
	case android.Arm64:
		src = String(p.Arch.Arm64.Src)
	case android.Riscv64:
		src = String(p.Arch.Riscv64.Src)
		// HACK: fall back to arm64 prebuilts, the riscv64 ones don't exist yet.
		if src == "" {
			src = String(p.Arch.Arm64.Src)
		}
	case android.X86:
		src = String(p.Arch.X86.Src)
	case android.X86_64:
		src = String(p.Arch.X86_64.Src)
	}
	if src == "" {
		src = String(p.Src)
	}

	if src == "" {
		if ctx.Config().AllowMissingDependencies() {
			ctx.AddMissingDependencies([]string{ctx.OtherModuleName(prebuilt)})
		} else {
			ctx.OtherModuleErrorf(prebuilt, "prebuilt_apex does not support %q", multiTargets[0].Arch.String())
		}
		// Drop through to return an empty string as the src (instead of nil) to avoid the prebuilt
		// logic from reporting a more general, less useful message.
	}

	return []string{src}
}

type PrebuiltProperties struct {
	ApexFileProperties

	PrebuiltCommonProperties
}

func (a *Prebuilt) hasSanitizedSource(sanitizer string) bool {
	return false
}

func (p *Prebuilt) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{p.outputApex}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

// prebuilt_apex imports an `.apex` file into the build graph as if it was built with apex.
func PrebuiltFactory() android.Module {
	module := &Prebuilt{}
	module.AddProperties(&module.properties)
	module.initPrebuiltCommon(module, &module.properties.PrebuiltCommonProperties)

	return module
}

func createApexSelectorModule(ctx android.TopDownMutatorContext, name string, apexFileProperties *ApexFileProperties) {
	props := struct {
		Name *string
	}{
		Name: proptools.StringPtr(name),
	}

	ctx.CreateModule(privateApexSelectorModuleFactory,
		&props,
		apexFileProperties,
	)
}

// createDeapexerModuleIfNeeded will create a deapexer module if it is needed.
//
// A deapexer module is only needed when the prebuilt apex specifies one or more modules in either
// the `exported_java_libs` or `exported_bootclasspath_fragments` properties as that indicates that
// the listed modules need access to files from within the prebuilt .apex file.
func (p *prebuiltCommon) createDeapexerModuleIfNeeded(ctx android.TopDownMutatorContext, deapexerName string, apexFileSource string) {
	// Only create the deapexer module if it is needed.
	if !p.hasExportedDeps() {
		return
	}

	// Compute the deapexer properties from the transitive dependencies of this module.
	commonModules := []string{}
	exportedFiles := []string{}
	ctx.WalkDeps(func(child, parent android.Module) bool {
		tag := ctx.OtherModuleDependencyTag(child)

		// If the child is not in the same apex as the parent then ignore it and all its children.
		if !android.IsDepInSameApex(ctx, parent, child) {
			return false
		}

		name := android.RemoveOptionalPrebuiltPrefix(ctx.OtherModuleName(child))
		if _, ok := tag.(android.RequiresFilesFromPrebuiltApexTag); ok {
			commonModules = append(commonModules, name)

			requiredFiles := child.(android.RequiredFilesFromPrebuiltApex).RequiredFilesFromPrebuiltApex(ctx)
			exportedFiles = append(exportedFiles, requiredFiles...)

			// Visit the dependencies of this module just in case they also require files from the
			// prebuilt apex.
			return true
		}

		return false
	})

	// Create properties for deapexer module.
	deapexerProperties := &DeapexerProperties{
		// Remove any duplicates from the common modules lists as a module may be included via a direct
		// dependency as well as transitive ones.
		CommonModules: android.SortedUniqueStrings(commonModules),
	}

	// Populate the exported files property in a fixed order.
	deapexerProperties.ExportedFiles = android.SortedUniqueStrings(exportedFiles)

	props := struct {
		Name          *string
		Selected_apex *string
	}{
		Name:          proptools.StringPtr(deapexerName),
		Selected_apex: proptools.StringPtr(apexFileSource),
	}
	ctx.CreateModule(privateDeapexerFactory,
		&props,
		deapexerProperties,
	)
}

func apexSelectorModuleName(baseModuleName string) string {
	return baseModuleName + ".apex.selector"
}

func prebuiltApexExportedModuleName(ctx android.BottomUpMutatorContext, name string) string {
	// The prebuilt_apex should be depending on prebuilt modules but as this runs after
	// prebuilt_rename the prebuilt module may or may not be using the prebuilt_ prefixed named. So,
	// check to see if the prefixed name is in use first, if it is then use that, otherwise assume
	// the unprefixed name is the one to use. If the unprefixed one turns out to be a source module
	// and not a renamed prebuilt module then that will be detected and reported as an error when
	// processing the dependency in ApexInfoMutator().
	prebuiltName := android.PrebuiltNameFromSource(name)
	if ctx.OtherModuleExists(prebuiltName) {
		name = prebuiltName
	}
	return name
}

type exportedDependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

// Mark this tag so dependencies that use it are excluded from visibility enforcement.
//
// This does allow any prebuilt_apex to reference any module which does open up a small window for
// restricted visibility modules to be referenced from the wrong prebuilt_apex. However, doing so
// avoids opening up a much bigger window by widening the visibility of modules that need files
// provided by the prebuilt_apex to include all the possible locations they may be defined, which
// could include everything below vendor/.
//
// A prebuilt_apex that references a module via this tag will have to contain the appropriate files
// corresponding to that module, otherwise it will fail when attempting to retrieve the files from
// the .apex file. It will also have to be included in the module's apex_available property too.
// That makes it highly unlikely that a prebuilt_apex would reference a restricted module
// incorrectly.
func (t exportedDependencyTag) ExcludeFromVisibilityEnforcement() {}

func (t exportedDependencyTag) RequiresFilesFromPrebuiltApex() {}

var _ android.RequiresFilesFromPrebuiltApexTag = exportedDependencyTag{}

var (
	exportedJavaLibTag                       = exportedDependencyTag{name: "exported_java_libs"}
	exportedBootclasspathFragmentTag         = exportedDependencyTag{name: "exported_bootclasspath_fragments"}
	exportedSystemserverclasspathFragmentTag = exportedDependencyTag{name: "exported_systemserverclasspath_fragments"}
)

var _ prebuiltApexModuleCreator = (*Prebuilt)(nil)

// createPrebuiltApexModules creates modules necessary to export files from the prebuilt apex to the
// build.
//
// If this needs to make files from within a `.apex` file available for use by other Soong modules,
// e.g. make dex implementation jars available for java_import modules listed in exported_java_libs,
// it does so as follows:
//
//  1. It creates a `deapexer` module that actually extracts the files from the `.apex` file and
//     makes them available for use by other modules, at both Soong and ninja levels.
//
//  2. It adds a dependency onto those modules and creates an apex specific variant similar to what
//     an `apex` module does. That ensures that code which looks for specific apex variant, e.g.
//     dexpreopt, will work the same way from source and prebuilt.
//
//  3. The `deapexer` module adds a dependency from the modules that require the exported files onto
//     itself so that they can retrieve the file paths to those files.
//
// It also creates a child module `selector` that is responsible for selecting the appropriate
// input apex for both the prebuilt_apex and the deapexer. That is needed for a couple of reasons:
//
//  1. To dedup the selection logic so it only runs in one module.
//
//  2. To allow the deapexer to be wired up to a different source for the input apex, e.g. an
//     `apex_set`.
//
//     prebuilt_apex
//     /      |      \
//     /         |         \
//     V            V            V
//     selector  <---  deapexer  <---  exported java lib
func (p *Prebuilt) createPrebuiltApexModules(ctx android.TopDownMutatorContext) {
	baseModuleName := p.BaseModuleName()

	apexSelectorModuleName := apexSelectorModuleName(baseModuleName)
	createApexSelectorModule(ctx, apexSelectorModuleName, &p.properties.ApexFileProperties)

	apexFileSource := ":" + apexSelectorModuleName
	p.createDeapexerModuleIfNeeded(ctx, deapexerModuleName(baseModuleName), apexFileSource)

	// Add a source reference to retrieve the selected apex from the selector module.
	p.prebuiltCommonProperties.Selected_apex = proptools.StringPtr(apexFileSource)
}

func (p *Prebuilt) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	p.prebuiltApexContentsDeps(ctx)
}

var _ ApexInfoMutator = (*Prebuilt)(nil)

func (p *Prebuilt) ApexInfoMutator(mctx android.TopDownMutatorContext) {
	p.apexInfoMutator(mctx)
}

func (p *Prebuilt) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.apexKeysPath = writeApexKeys(ctx, p)
	// TODO(jungjw): Check the key validity.
	p.inputApex = android.OptionalPathForModuleSrc(ctx, p.prebuiltCommonProperties.Selected_apex).Path()
	p.installDir = android.PathForModuleInstall(ctx, "apex")
	p.installFilename = p.InstallFilename()
	if !strings.HasSuffix(p.installFilename, imageApexSuffix) {
		ctx.ModuleErrorf("filename should end in %s for prebuilt_apex", imageApexSuffix)
	}
	p.outputApex = android.PathForModuleOut(ctx, p.installFilename)
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Input:  p.inputApex,
		Output: p.outputApex,
	})

	if p.prebuiltCommon.checkForceDisable(ctx) {
		p.HideFromMake()
		return
	}

	// Save the files that need to be made available to Make.
	p.initApexFilesForAndroidMk(ctx)

	// in case that prebuilt_apex replaces source apex (using prefer: prop)
	p.compatSymlinks = makeCompatSymlinks(p.BaseModuleName(), ctx)
	// or that prebuilt_apex overrides other apexes (using overrides: prop)
	for _, overridden := range p.prebuiltCommonProperties.Overrides {
		p.compatSymlinks = append(p.compatSymlinks, makeCompatSymlinks(overridden, ctx)...)
	}

	if p.installable() {
		p.installedFile = ctx.InstallFile(p.installDir, p.installFilename, p.inputApex, p.compatSymlinks...)
		p.provenanceMetaDataFile = provenance.GenerateArtifactProvenanceMetaData(ctx, p.inputApex, p.installedFile)
	}
}

func (p *Prebuilt) ProvenanceMetaDataFile() android.OutputPath {
	return p.provenanceMetaDataFile
}

// prebuiltApexExtractorModule is a private module type that is only created by the prebuilt_apex
// module. It extracts the correct apex to use and makes it available for use by apex_set.
type prebuiltApexExtractorModule struct {
	android.ModuleBase

	properties ApexExtractorProperties

	extractedApex android.WritablePath
}

func privateApexExtractorModuleFactory() android.Module {
	module := &prebuiltApexExtractorModule{}
	module.AddProperties(
		&module.properties,
	)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func (p *prebuiltApexExtractorModule) Srcs() android.Paths {
	return android.Paths{p.extractedApex}
}

func (p *prebuiltApexExtractorModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	srcsSupplier := func(ctx android.BaseModuleContext, prebuilt android.Module) []string {
		return p.properties.prebuiltSrcs(ctx)
	}
	defaultAllowPrerelease := ctx.Config().IsEnvTrue("SOONG_ALLOW_PRERELEASE_APEXES")
	apexSet := android.SingleSourcePathFromSupplier(ctx, srcsSupplier, "set")
	p.extractedApex = android.PathForModuleOut(ctx, "extracted", apexSet.Base())
	// Filter out NativeBridge archs (b/260115309)
	abis := java.SupportedAbis(ctx, true)
	ctx.Build(pctx,
		android.BuildParams{
			Rule:        extractMatchingApex,
			Description: "Extract an apex from an apex set",
			Inputs:      android.Paths{apexSet},
			Output:      p.extractedApex,
			Args: map[string]string{
				"abis":              strings.Join(abis, ","),
				"allow-prereleased": strconv.FormatBool(proptools.BoolDefault(p.properties.Prerelease, defaultAllowPrerelease)),
				"sdk-version":       ctx.Config().PlatformSdkVersion().String(),
				"skip-sdk-check":    strconv.FormatBool(ctx.Config().IsEnvTrue("SOONG_SKIP_APPSET_SDK_CHECK")),
			},
		})
}

type ApexSet struct {
	prebuiltCommon

	properties ApexSetProperties
}

type ApexExtractorProperties struct {
	// the .apks file path that contains prebuilt apex files to be extracted.
	Set *string `android:"path"`

	Sanitized struct {
		None struct {
			Set *string `android:"path"`
		}
		Address struct {
			Set *string `android:"path"`
		}
		Hwaddress struct {
			Set *string `android:"path"`
		}
	}

	// apexes in this set use prerelease SDK version
	Prerelease *bool
}

func (e *ApexExtractorProperties) prebuiltSrcs(ctx android.BaseModuleContext) []string {
	var srcs []string
	if e.Set != nil {
		srcs = append(srcs, *e.Set)
	}

	sanitizers := ctx.Config().SanitizeDevice()

	if android.InList("address", sanitizers) && e.Sanitized.Address.Set != nil {
		srcs = append(srcs, *e.Sanitized.Address.Set)
	} else if android.InList("hwaddress", sanitizers) && e.Sanitized.Hwaddress.Set != nil {
		srcs = append(srcs, *e.Sanitized.Hwaddress.Set)
	} else if e.Sanitized.None.Set != nil {
		srcs = append(srcs, *e.Sanitized.None.Set)
	}

	return srcs
}

type ApexSetProperties struct {
	ApexExtractorProperties

	PrebuiltCommonProperties
}

func (a *ApexSet) hasSanitizedSource(sanitizer string) bool {
	if sanitizer == "address" {
		return a.properties.Sanitized.Address.Set != nil
	}
	if sanitizer == "hwaddress" {
		return a.properties.Sanitized.Hwaddress.Set != nil
	}

	return false
}

func (a *ApexSet) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{a.outputApex}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

// prebuilt_apex imports an `.apex` file into the build graph as if it was built with apex.
func apexSetFactory() android.Module {
	module := &ApexSet{}
	module.AddProperties(&module.properties)
	module.initPrebuiltCommon(module, &module.properties.PrebuiltCommonProperties)

	return module
}

func createApexExtractorModule(ctx android.TopDownMutatorContext, name string, apexExtractorProperties *ApexExtractorProperties) {
	props := struct {
		Name *string
	}{
		Name: proptools.StringPtr(name),
	}

	ctx.CreateModule(privateApexExtractorModuleFactory,
		&props,
		apexExtractorProperties,
	)
}

func apexExtractorModuleName(baseModuleName string) string {
	return baseModuleName + ".apex.extractor"
}

var _ prebuiltApexModuleCreator = (*ApexSet)(nil)

// createPrebuiltApexModules creates modules necessary to export files from the apex set to other
// modules.
//
// This effectively does for apex_set what Prebuilt.createPrebuiltApexModules does for a
// prebuilt_apex except that instead of creating a selector module which selects one .apex file
// from those provided this creates an extractor module which extracts the appropriate .apex file
// from the zip file containing them.
func (a *ApexSet) createPrebuiltApexModules(ctx android.TopDownMutatorContext) {
	baseModuleName := a.BaseModuleName()

	apexExtractorModuleName := apexExtractorModuleName(baseModuleName)
	createApexExtractorModule(ctx, apexExtractorModuleName, &a.properties.ApexExtractorProperties)

	apexFileSource := ":" + apexExtractorModuleName
	a.createDeapexerModuleIfNeeded(ctx, deapexerModuleName(baseModuleName), apexFileSource)

	// After passing the arch specific src properties to the creating the apex selector module
	a.prebuiltCommonProperties.Selected_apex = proptools.StringPtr(apexFileSource)
}

func (a *ApexSet) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	a.prebuiltApexContentsDeps(ctx)
}

var _ ApexInfoMutator = (*ApexSet)(nil)

func (a *ApexSet) ApexInfoMutator(mctx android.TopDownMutatorContext) {
	a.apexInfoMutator(mctx)
}

func (a *ApexSet) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.apexKeysPath = writeApexKeys(ctx, a)
	a.installFilename = a.InstallFilename()
	if !strings.HasSuffix(a.installFilename, imageApexSuffix) && !strings.HasSuffix(a.installFilename, imageCapexSuffix) {
		ctx.ModuleErrorf("filename should end in %s or %s for apex_set", imageApexSuffix, imageCapexSuffix)
	}

	inputApex := android.OptionalPathForModuleSrc(ctx, a.prebuiltCommonProperties.Selected_apex).Path()
	a.outputApex = android.PathForModuleOut(ctx, a.installFilename)
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Input:  inputApex,
		Output: a.outputApex,
	})

	if a.prebuiltCommon.checkForceDisable(ctx) {
		a.HideFromMake()
		return
	}

	// Save the files that need to be made available to Make.
	a.initApexFilesForAndroidMk(ctx)

	a.installDir = android.PathForModuleInstall(ctx, "apex")
	if a.installable() {
		a.installedFile = ctx.InstallFile(a.installDir, a.installFilename, a.outputApex)
	}

	// in case that apex_set replaces source apex (using prefer: prop)
	a.compatSymlinks = makeCompatSymlinks(a.BaseModuleName(), ctx)
	// or that apex_set overrides other apexes (using overrides: prop)
	for _, overridden := range a.prebuiltCommonProperties.Overrides {
		a.compatSymlinks = append(a.compatSymlinks, makeCompatSymlinks(overridden, ctx)...)
	}
}

type systemExtContext struct {
	android.ModuleContext
}

func (*systemExtContext) SystemExtSpecific() bool {
	return true
}
