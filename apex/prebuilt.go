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
	"strconv"
	"strings"

	"android/soong/android"
	"android/soong/java"
	"github.com/google/blueprint"

	"github.com/google/blueprint/proptools"
)

var (
	extractMatchingApex = pctx.StaticRule(
		"extractMatchingApex",
		blueprint.RuleParams{
			Command: `rm -rf "$out" && ` +
				`${extract_apks} -o "${out}" -allow-prereleased=${allow-prereleased} ` +
				`-sdk-version=${sdk-version} -abis=${abis} -screen-densities=all -extract-single ` +
				`${in}`,
			CommandDeps: []string{"${extract_apks}"},
		},
		"abis", "allow-prereleased", "sdk-version")
)

type prebuilt interface {
	isForceDisabled() bool
	InstallFilename() string
}

type prebuiltCommon struct {
	prebuilt   android.Prebuilt
	properties prebuiltCommonProperties
}

type sanitizedPrebuilt interface {
	hasSanitizedSource(sanitizer string) bool
}

type prebuiltCommonProperties struct {
	ForceDisable bool `blueprint:"mutated"`
}

func (p *prebuiltCommon) Prebuilt() *android.Prebuilt {
	return &p.prebuilt
}

func (p *prebuiltCommon) isForceDisabled() bool {
	return p.properties.ForceDisable
}

func (p *prebuiltCommon) checkForceDisable(ctx android.ModuleContext) bool {
	// If the device is configured to use flattened APEX, force disable the prebuilt because
	// the prebuilt is a non-flattened one.
	forceDisable := ctx.Config().FlattenApex()

	// Force disable the prebuilts when we are doing unbundled build. We do unbundled build
	// to build the prebuilts themselves.
	forceDisable = forceDisable || ctx.Config().UnbundledBuild()

	// Force disable the prebuilts when coverage is enabled.
	forceDisable = forceDisable || ctx.DeviceConfig().NativeCoverageEnabled()
	forceDisable = forceDisable || ctx.Config().IsEnvTrue("EMMA_INSTRUMENT")

	// b/137216042 don't use prebuilts when address sanitizer is on, unless the prebuilt has a sanitized source
	sanitized := ctx.Module().(sanitizedPrebuilt)
	forceDisable = forceDisable || (android.InList("address", ctx.Config().SanitizeDevice()) && !sanitized.hasSanitizedSource("address"))
	forceDisable = forceDisable || (android.InList("hwaddress", ctx.Config().SanitizeDevice()) && !sanitized.hasSanitizedSource("hwaddress"))

	if forceDisable && p.prebuilt.SourceExists() {
		p.properties.ForceDisable = true
		return true
	}
	return false
}

type Prebuilt struct {
	android.ModuleBase
	prebuiltCommon

	properties PrebuiltProperties

	inputApex       android.Path
	installDir      android.InstallPath
	installFilename string
	outputApex      android.WritablePath

	// list of commands to create symlinks for backward compatibility.
	// these commands will be attached as LOCAL_POST_INSTALL_CMD
	compatSymlinks []string
}

type ApexFileProperties struct {
	// the path to the prebuilt .apex file to import.
	//
	// This cannot be marked as `android:"arch_variant"` because the `prebuilt_apex` is only mutated
	// for android_common. That is so that it will have the same arch variant as, and so be compatible
	// with, the source `apex` module type that it replaces.
	Src  *string
	Arch struct {
		Arm struct {
			Src *string
		}
		Arm64 struct {
			Src *string
		}
		X86 struct {
			Src *string
		}
		X86_64 struct {
			Src *string
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
	case android.X86:
		src = String(p.Arch.X86.Src)
	case android.X86_64:
		src = String(p.Arch.X86_64.Src)
	default:
		ctx.OtherModuleErrorf(prebuilt, "prebuilt_apex does not support %q", multiTargets[0].Arch.String())
		return nil
	}
	if src == "" {
		src = String(p.Src)
	}

	return []string{src}
}

type PrebuiltProperties struct {
	ApexFileProperties
	DeapexerProperties

	Installable *bool
	// Optional name for the installed apex. If unspecified, name of the
	// module is used as the file name
	Filename *string

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string
}

func (a *Prebuilt) hasSanitizedSource(sanitizer string) bool {
	return false
}

func (p *Prebuilt) installable() bool {
	return p.properties.Installable == nil || proptools.Bool(p.properties.Installable)
}

func (p *Prebuilt) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{p.outputApex}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (p *Prebuilt) InstallFilename() string {
	return proptools.StringDefault(p.properties.Filename, p.BaseModuleName()+imageApexSuffix)
}

func (p *Prebuilt) Name() string {
	return p.prebuiltCommon.prebuilt.Name(p.ModuleBase.Name())
}

// prebuilt_apex imports an `.apex` file into the build graph as if it was built with apex.
//
// If this needs to make files from within a `.apex` file available for use by other Soong modules,
// e.g. make dex implementation jars available for java_import modules isted in exported_java_libs,
// it does so as follows:
//
// 1. It creates a `deapexer` module that actually extracts the files from the `.apex` file and
//    makes them available for use by other modules, at both Soong and ninja levels.
//
// 2. It adds a dependency onto those modules and creates an apex specific variant similar to what
//    an `apex` module does. That ensures that code which looks for specific apex variant, e.g.
//    dexpreopt, will work the same way from source and prebuilt.
//
// 3. The `deapexer` module adds a dependency from the modules that require the exported files onto
//    itself so that they can retrieve the file paths to those files.
//
func PrebuiltFactory() android.Module {
	module := &Prebuilt{}
	module.AddProperties(&module.properties)
	android.InitPrebuiltModuleWithSrcSupplier(module, module.properties.prebuiltApexSelector, "src")
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)

	android.AddLoadHook(module, func(ctx android.LoadHookContext) {
		props := struct {
			Name *string
		}{
			Name: proptools.StringPtr(module.BaseModuleName() + ".deapexer"),
		}
		ctx.CreateModule(privateDeapexerFactory,
			&props,
			&module.properties.ApexFileProperties,
			&module.properties.DeapexerProperties,
		)
	})

	return module
}

func prebuiltApexExportedModuleName(ctx android.BottomUpMutatorContext, name string) string {
	// The prebuilt_apex should be depending on prebuilt modules but as this runs after
	// prebuilt_rename the prebuilt module may or may not be using the prebuilt_ prefixed named. So,
	// check to see if the prefixed name is in use first, if it is then use that, otherwise assume
	// the unprefixed name is the one to use. If the unprefixed one turns out to be a source module
	// and not a renamed prebuilt module then that will be detected and reported as an error when
	// processing the dependency in ApexInfoMutator().
	prebuiltName := "prebuilt_" + name
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

var (
	exportedJavaLibTag = exportedDependencyTag{name: "exported_java_lib"}
)

func (p *Prebuilt) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies onto the java modules that represent the java libraries that are provided by
	// and exported from this prebuilt apex.
	for _, lib := range p.properties.Exported_java_libs {
		dep := prebuiltApexExportedModuleName(ctx, lib)
		ctx.AddFarVariationDependencies(ctx.Config().AndroidCommonTarget.Variations(), exportedJavaLibTag, dep)
	}
}

var _ ApexInfoMutator = (*Prebuilt)(nil)

// ApexInfoMutator marks any modules for which this apex exports a file as requiring an apex
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
// * The number of prebuilt_apex modules that will be exporting files for the same module will be
//   low as the prebuilt_apex only exports files for the direct dependencies that require it and
//   very few modules are direct dependencies of multiple prebuilt_apex modules, e.g. there are a
//   few com.android.art* apex files that contain the same contents and could export files for the
//   same modules but only one of them needs to do so. Contrast that with source apex modules which
//   need apex specific variants for every module that contributes code to the apex, whether direct
//   or indirect.
// * The build cost of a prebuilt_apex variant is generally low as at worst it will involve some
//   extra copying of files. Contrast that with source apex modules that has to build each variant
//   from source.
func (p *Prebuilt) ApexInfoMutator(mctx android.TopDownMutatorContext) {

	// Collect direct dependencies into contents.
	contents := make(map[string]android.ApexMembership)

	// Collect the list of dependencies.
	var dependencies []android.ApexModule
	mctx.VisitDirectDeps(func(m android.Module) {
		tag := mctx.OtherModuleDependencyTag(m)
		if tag == exportedJavaLibTag {
			depName := mctx.OtherModuleName(m)

			// It is an error if the other module is not a prebuilt.
			if _, ok := m.(android.PrebuiltInterface); !ok {
				mctx.PropertyErrorf("exported_java_libs", "%q is not a prebuilt module", depName)
				return
			}

			// It is an error if the other module is not an ApexModule.
			if _, ok := m.(android.ApexModule); !ok {
				mctx.PropertyErrorf("exported_java_libs", "%q is not usable within an apex", depName)
				return
			}

			// Strip off the prebuilt_ prefix if present before storing content to ensure consistent
			// behavior whether there is a corresponding source module present or not.
			depName = android.RemoveOptionalPrebuiltPrefix(depName)

			// Remember that this module was added as a direct dependency.
			contents[depName] = contents[depName].Add(true)

			// Add the module to the list of dependencies that need to have an APEX variant.
			dependencies = append(dependencies, m.(android.ApexModule))
		}
	})

	// Create contents for the prebuilt_apex and store it away for later use.
	apexContents := android.NewApexContents(contents)
	mctx.SetProvider(ApexBundleInfoProvider, ApexBundleInfo{
		Contents: apexContents,
	})

	// Create an ApexInfo for the prebuilt_apex.
	apexInfo := android.ApexInfo{
		ApexVariationName: mctx.ModuleName(),
		InApexes:          []string{mctx.ModuleName()},
		ApexContents:      []*android.ApexContents{apexContents},
		ForPrebuiltApex:   true,
	}

	// Mark the dependencies of this module as requiring a variant for this module.
	for _, am := range dependencies {
		am.BuildForApex(apexInfo)
	}
}

func (p *Prebuilt) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// TODO(jungjw): Check the key validity.
	p.inputApex = p.Prebuilt().SingleSourcePath(ctx)
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

	if p.installable() {
		ctx.InstallFile(p.installDir, p.installFilename, p.inputApex)
	}

	// in case that prebuilt_apex replaces source apex (using prefer: prop)
	p.compatSymlinks = makeCompatSymlinks(p.BaseModuleName(), ctx)
	// or that prebuilt_apex overrides other apexes (using overrides: prop)
	for _, overridden := range p.properties.Overrides {
		p.compatSymlinks = append(p.compatSymlinks, makeCompatSymlinks(overridden, ctx)...)
	}
}

func (p *Prebuilt) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(p.inputApex),
		Include:    "$(BUILD_PREBUILT)",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", p.installDir.ToMakePath().String())
				entries.SetString("LOCAL_MODULE_STEM", p.installFilename)
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !p.installable())
				entries.AddStrings("LOCAL_OVERRIDES_MODULES", p.properties.Overrides...)
				if len(p.compatSymlinks) > 0 {
					entries.SetString("LOCAL_POST_INSTALL_CMD", strings.Join(p.compatSymlinks, " && "))
				}
			},
		},
	}}
}

type ApexSet struct {
	android.ModuleBase
	prebuiltCommon

	properties ApexSetProperties

	installDir      android.InstallPath
	installFilename string
	outputApex      android.WritablePath

	// list of commands to create symlinks for backward compatibility.
	// these commands will be attached as LOCAL_POST_INSTALL_CMD
	compatSymlinks []string
}

type ApexSetProperties struct {
	// the .apks file path that contains prebuilt apex files to be extracted.
	Set *string

	Sanitized struct {
		None struct {
			Set *string
		}
		Address struct {
			Set *string
		}
		Hwaddress struct {
			Set *string
		}
	}

	// whether the extracted apex file installable.
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

	// apexes in this set use prerelease SDK version
	Prerelease *bool
}

func (a *ApexSet) prebuiltSrcs(ctx android.BaseModuleContext) []string {
	var srcs []string
	if a.properties.Set != nil {
		srcs = append(srcs, *a.properties.Set)
	}

	var sanitizers []string
	if ctx.Host() {
		sanitizers = ctx.Config().SanitizeHost()
	} else {
		sanitizers = ctx.Config().SanitizeDevice()
	}

	if android.InList("address", sanitizers) && a.properties.Sanitized.Address.Set != nil {
		srcs = append(srcs, *a.properties.Sanitized.Address.Set)
	} else if android.InList("hwaddress", sanitizers) && a.properties.Sanitized.Hwaddress.Set != nil {
		srcs = append(srcs, *a.properties.Sanitized.Hwaddress.Set)
	} else if a.properties.Sanitized.None.Set != nil {
		srcs = append(srcs, *a.properties.Sanitized.None.Set)
	}

	return srcs
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

func (a *ApexSet) installable() bool {
	return a.properties.Installable == nil || proptools.Bool(a.properties.Installable)
}

func (a *ApexSet) InstallFilename() string {
	return proptools.StringDefault(a.properties.Filename, a.BaseModuleName()+imageApexSuffix)
}

func (a *ApexSet) Name() string {
	return a.prebuiltCommon.prebuilt.Name(a.ModuleBase.Name())
}

func (a *ApexSet) Overrides() []string {
	return a.properties.Overrides
}

// prebuilt_apex imports an `.apex` file into the build graph as if it was built with apex.
func apexSetFactory() android.Module {
	module := &ApexSet{}
	module.AddProperties(&module.properties)

	srcsSupplier := func(ctx android.BaseModuleContext, _ android.Module) []string {
		return module.prebuiltSrcs(ctx)
	}

	android.InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, "set")
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func (a *ApexSet) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.installFilename = a.InstallFilename()
	if !strings.HasSuffix(a.installFilename, imageApexSuffix) {
		ctx.ModuleErrorf("filename should end in %s for apex_set", imageApexSuffix)
	}

	apexSet := a.prebuiltCommon.prebuilt.SingleSourcePath(ctx)
	a.outputApex = android.PathForModuleOut(ctx, a.installFilename)
	ctx.Build(pctx,
		android.BuildParams{
			Rule:        extractMatchingApex,
			Description: "Extract an apex from an apex set",
			Inputs:      android.Paths{apexSet},
			Output:      a.outputApex,
			Args: map[string]string{
				"abis":              strings.Join(java.SupportedAbis(ctx), ","),
				"allow-prereleased": strconv.FormatBool(proptools.Bool(a.properties.Prerelease)),
				"sdk-version":       ctx.Config().PlatformSdkVersion().String(),
			},
		})

	if a.prebuiltCommon.checkForceDisable(ctx) {
		a.HideFromMake()
		return
	}

	a.installDir = android.PathForModuleInstall(ctx, "apex")
	if a.installable() {
		ctx.InstallFile(a.installDir, a.installFilename, a.outputApex)
	}

	// in case that apex_set replaces source apex (using prefer: prop)
	a.compatSymlinks = makeCompatSymlinks(a.BaseModuleName(), ctx)
	// or that apex_set overrides other apexes (using overrides: prop)
	for _, overridden := range a.properties.Overrides {
		a.compatSymlinks = append(a.compatSymlinks, makeCompatSymlinks(overridden, ctx)...)
	}
}

func (a *ApexSet) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(a.outputApex),
		Include:    "$(BUILD_PREBUILT)",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", a.installDir.ToMakePath().String())
				entries.SetString("LOCAL_MODULE_STEM", a.installFilename)
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !a.installable())
				entries.AddStrings("LOCAL_OVERRIDES_MODULES", a.properties.Overrides...)
				if len(a.compatSymlinks) > 0 {
					entries.SetString("LOCAL_POST_INSTALL_CMD", strings.Join(a.compatSymlinks, " && "))
				}
			},
		},
	}}
}
