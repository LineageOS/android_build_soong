// Copyright (C) 2021 The Android Open Source Project
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

package java

import (
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"
	"android/soong/testing"

	"github.com/google/blueprint/proptools"

	"github.com/google/blueprint"
)

func init() {
	registerBootclasspathFragmentBuildComponents(android.InitRegistrationContext)

	android.RegisterSdkMemberType(BootclasspathFragmentSdkMemberType)
}

func registerBootclasspathFragmentBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("bootclasspath_fragment", bootclasspathFragmentFactory)
	ctx.RegisterModuleType("bootclasspath_fragment_test", testBootclasspathFragmentFactory)
	ctx.RegisterModuleType("prebuilt_bootclasspath_fragment", prebuiltBootclasspathFragmentFactory)
}

// BootclasspathFragmentSdkMemberType is the member type used to add bootclasspath_fragments to
// the SDK snapshot. It is exported for use by apex.
var BootclasspathFragmentSdkMemberType = &bootclasspathFragmentMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName: "bootclasspath_fragments",
		SupportsSdk:  true,
	},
}

type bootclasspathFragmentContentDependencyTag struct {
	blueprint.BaseDependencyTag
}

// Avoid having to make bootclasspath_fragment content visible to the bootclasspath_fragment.
//
// This is a temporary workaround to make it easier to migrate to bootclasspath_fragment modules
// with proper dependencies.
// TODO(b/177892522): Remove this and add needed visibility.
func (b bootclasspathFragmentContentDependencyTag) ExcludeFromVisibilityEnforcement() {
}

// The bootclasspath_fragment contents must never depend on prebuilts.
func (b bootclasspathFragmentContentDependencyTag) ReplaceSourceWithPrebuilt() bool {
	return false
}

// SdkMemberType causes dependencies added with this tag to be automatically added to the sdk as if
// they were specified using java_boot_libs or java_sdk_libs.
func (b bootclasspathFragmentContentDependencyTag) SdkMemberType(child android.Module) android.SdkMemberType {
	// If the module is a java_sdk_library then treat it as if it was specified in the java_sdk_libs
	// property, otherwise treat if it was specified in the java_boot_libs property.
	if javaSdkLibrarySdkMemberType.IsInstance(child) {
		return javaSdkLibrarySdkMemberType
	}

	return javaBootLibsSdkMemberType
}

func (b bootclasspathFragmentContentDependencyTag) ExportMember() bool {
	return true
}

// Contents of bootclasspath fragments in an apex are considered to be directly in the apex, as if
// they were listed in java_libs.
func (b bootclasspathFragmentContentDependencyTag) CopyDirectlyInAnyApex() {}

// Contents of bootclasspath fragments require files from prebuilt apex files.
func (b bootclasspathFragmentContentDependencyTag) RequiresFilesFromPrebuiltApex() {}

// The tag used for the dependency between the bootclasspath_fragment module and its contents.
var bootclasspathFragmentContentDepTag = bootclasspathFragmentContentDependencyTag{}

var _ android.ExcludeFromVisibilityEnforcementTag = bootclasspathFragmentContentDepTag
var _ android.ReplaceSourceWithPrebuilt = bootclasspathFragmentContentDepTag
var _ android.SdkMemberDependencyTag = bootclasspathFragmentContentDepTag
var _ android.CopyDirectlyInAnyApexTag = bootclasspathFragmentContentDepTag
var _ android.RequiresFilesFromPrebuiltApexTag = bootclasspathFragmentContentDepTag

func IsBootclasspathFragmentContentDepTag(tag blueprint.DependencyTag) bool {
	return tag == bootclasspathFragmentContentDepTag
}

// Properties that can be different when coverage is enabled.
type BootclasspathFragmentCoverageAffectedProperties struct {
	// The contents of this bootclasspath_fragment, could be either java_library, or java_sdk_library.
	//
	// A java_sdk_library specified here will also be treated as if it was specified on the stub_libs
	// property.
	//
	// The order of this list matters as it is the order that is used in the bootclasspath.
	Contents []string

	// The properties for specifying the API stubs provided by this fragment.
	BootclasspathAPIProperties
}

type bootclasspathFragmentProperties struct {
	// The name of the image this represents.
	//
	// If specified then it must be one of "art" or "boot".
	Image_name *string

	// Properties whose values need to differ with and without coverage.
	BootclasspathFragmentCoverageAffectedProperties
	Coverage BootclasspathFragmentCoverageAffectedProperties

	// Hidden API related properties.
	HiddenAPIFlagFileProperties

	// The list of additional stub libraries which this fragment's contents use but which are not
	// provided by another bootclasspath_fragment.
	//
	// Note, "android-non-updatable" is treated specially. While no such module exists it is treated
	// as if it was a java_sdk_library. So, when public API stubs are needed then it will be replaced
	// with "android-non-updatable.stubs", with "androidn-non-updatable.system.stubs" when the system
	// stubs are needed and so on.
	Additional_stubs []string

	// Properties that allow a fragment to depend on other fragments. This is needed for hidden API
	// processing as it needs access to all the classes used by a fragment including those provided
	// by other fragments.
	BootclasspathFragmentsDepsProperties
}

type HiddenAPIPackageProperties struct {
	Hidden_api struct {
		// Contains prefixes of a package hierarchy that is provided solely by this
		// bootclasspath_fragment.
		//
		// This affects the signature patterns file that is used to select the subset of monolithic
		// hidden API flags. See split_packages property for more details.
		Package_prefixes []string

		// A list of individual packages that are provided solely by this
		// bootclasspath_fragment but which cannot be listed in package_prefixes
		// because there are sub-packages which are provided by other modules.
		//
		// This should only be used for legacy packages. New packages should be
		// covered by a package prefix.
		Single_packages []string

		// The list of split packages provided by this bootclasspath_fragment.
		//
		// A split package is one that contains classes which are provided by multiple
		// bootclasspath_fragment modules.
		//
		// This defaults to "*" - which treats all packages as being split. A module that has no split
		// packages must specify an empty list.
		//
		// This affects the signature patterns file that is generated by a bootclasspath_fragment and
		// used to select the subset of monolithic hidden API flags against which the flags generated
		// by the bootclasspath_fragment are compared.
		//
		// The signature patterns file selects the subset of monolithic hidden API flags using a number
		// of patterns, i.e.:
		// * The qualified name (including package) of an outermost class, e.g. java/lang/Character.
		//   This selects all the flags for all the members of this class and any nested classes.
		// * A package wildcard, e.g. java/lang/*. This selects all the flags for all the members of all
		//   the classes in this package (but not in sub-packages).
		// * A recursive package wildcard, e.g. java/**. This selects all the flags for all the members
		//   of all the classes in this package and sub-packages.
		//
		// The signature patterns file is constructed as follows:
		// * All the signatures are retrieved from the all-flags.csv file.
		// * The member and inner class names are removed.
		// * If a class is in a split package then that is kept, otherwise the class part is removed
		//   and replaced with a wildcard, i.e. *.
		// * If a package matches a package prefix then the package is removed.
		// * All the package prefixes are added with a recursive wildcard appended to each, i.e. **.
		// * The resulting patterns are sorted.
		//
		// So, by default (i.e. without specifying any package_prefixes or split_packages) the signature
		// patterns is a list of class names, because there are no package packages and all packages are
		// assumed to be split.
		//
		// If any split packages are specified then only those packages are treated as split and all
		// other packages are treated as belonging solely to the bootclasspath_fragment and so they use
		// wildcard package patterns.
		//
		// So, if an empty list of split packages is specified then the signature patterns file just
		// includes a wildcard package pattern for every package provided by the bootclasspath_fragment.
		//
		// If split_packages are specified and a package that is split is not listed then it could lead
		// to build failures as it will select monolithic flags that are generated by another
		// bootclasspath_fragment to compare against the flags provided by this fragment. The latter
		// will obviously not contain those flags and that can cause the comparison and build to fail.
		//
		// If any package prefixes are specified then any matching packages are removed from the
		// signature patterns and replaced with a single recursive package pattern.
		//
		// It is not strictly necessary to specify either package_prefixes or split_packages as the
		// defaults will produce a valid set of signature patterns. However, those patterns may include
		// implementation details, e.g. names of implementation classes or packages, which will be
		// exported to the sdk snapshot in the signature patterns file. That is something that should be
		// avoided where possible. Specifying package_prefixes and split_packages allows those
		// implementation details to be excluded from the snapshot.
		Split_packages []string
	}
}

type SourceOnlyBootclasspathProperties struct {
	HiddenAPIPackageProperties
	Coverage HiddenAPIPackageProperties
}

type BootclasspathFragmentModule struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase
	ClasspathFragmentBase

	// True if this fragment is for testing purposes.
	testFragment bool

	properties bootclasspathFragmentProperties

	sourceOnlyProperties SourceOnlyBootclasspathProperties

	// Path to the boot image profile.
	profilePath    android.WritablePath
	profilePathErr error
}

// commonBootclasspathFragment defines the methods that are implemented by both source and prebuilt
// bootclasspath fragment modules.
type commonBootclasspathFragment interface {
	// produceHiddenAPIOutput produces the all-flags.csv and intermediate files and encodes the flags
	// into dex files.
	//
	// Returns a *HiddenAPIOutput containing the paths for the generated files. Returns nil if the
	// module cannot contribute to hidden API processing, e.g. because it is a prebuilt module in a
	// versioned sdk.
	produceHiddenAPIOutput(ctx android.ModuleContext, contents []android.Module, fragments []android.Module, input HiddenAPIFlagInput) *HiddenAPIOutput

	// getProfilePath returns the path to the boot image profile.
	getProfilePath() android.Path
}

var _ commonBootclasspathFragment = (*BootclasspathFragmentModule)(nil)

func bootclasspathFragmentFactory() android.Module {
	m := &BootclasspathFragmentModule{}
	m.AddProperties(&m.properties, &m.sourceOnlyProperties)
	android.InitApexModule(m)
	initClasspathFragment(m, BOOTCLASSPATH)
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(m)

	android.AddLoadHook(m, func(ctx android.LoadHookContext) {
		// If code coverage has been enabled for the framework then append the properties with
		// coverage specific properties.
		if ctx.Config().IsEnvTrue("EMMA_INSTRUMENT_FRAMEWORK") {
			err := proptools.AppendProperties(&m.properties.BootclasspathFragmentCoverageAffectedProperties, &m.properties.Coverage, nil)
			if err != nil {
				ctx.PropertyErrorf("coverage", "error trying to append coverage specific properties: %s", err)
				return
			}

			err = proptools.AppendProperties(&m.sourceOnlyProperties.HiddenAPIPackageProperties, &m.sourceOnlyProperties.Coverage, nil)
			if err != nil {
				ctx.PropertyErrorf("coverage", "error trying to append hidden api coverage specific properties: %s", err)
				return
			}
		}
	})
	return m
}

func testBootclasspathFragmentFactory() android.Module {
	m := bootclasspathFragmentFactory().(*BootclasspathFragmentModule)
	m.testFragment = true
	return m
}

func (m *BootclasspathFragmentModule) bootclasspathFragmentPropertyCheck(ctx android.EarlyModuleContext) {
	contents := m.properties.Contents
	if len(contents) == 0 {
		ctx.PropertyErrorf("contents", "required property is missing")
		return
	}

	if m.properties.Image_name == nil {
		// Nothing to do.
		return
	}

	imageName := proptools.String(m.properties.Image_name)
	if imageName != "art" {
		ctx.PropertyErrorf("image_name", `unknown image name %q, expected "art"`, imageName)
		return
	}

	// Get the configuration for the art apex jars. Do not use getImageConfig(ctx) here as this is
	// too early in the Soong processing for that to work.
	global := dexpreopt.GetGlobalConfig(ctx)
	modules := global.ArtApexJars
	configuredJars := modules.CopyOfJars()

	// Skip the check if the configured jars list is empty as that is a common configuration when
	// building targets that do not result in a system image.
	if len(configuredJars) == 0 {
		return
	}

	if !reflect.DeepEqual(configuredJars, contents) {
		ctx.ModuleErrorf("inconsistency in specification of contents. ArtApexJars configuration specifies %#v, contents property specifies %#v",
			configuredJars, contents)
	}

	// Make sure that the apex specified in the configuration is consistent and is one for which
	// this boot image is available.
	commonApex := ""
	for i := 0; i < modules.Len(); i++ {
		apex := modules.Apex(i)
		jar := modules.Jar(i)
		if apex == "platform" {
			ctx.ModuleErrorf("ArtApexJars is invalid as it requests a platform variant of %q", jar)
			continue
		}
		if !m.AvailableFor(apex) {
			ctx.ModuleErrorf("ArtApexJars configuration incompatible with this module, ArtApexJars expects this to be in apex %q but this is only in apexes %q",
				apex, m.ApexAvailable())
			continue
		}
		if commonApex == "" {
			commonApex = apex
		} else if commonApex != apex {
			ctx.ModuleErrorf("ArtApexJars configuration is inconsistent, expected all jars to be in the same apex but it specifies apex %q and %q",
				commonApex, apex)
		}
	}
}

var BootclasspathFragmentApexContentInfoProvider = blueprint.NewProvider[BootclasspathFragmentApexContentInfo]()

// BootclasspathFragmentApexContentInfo contains the bootclasspath_fragments contributions to the
// apex contents.
type BootclasspathFragmentApexContentInfo struct {
	// Map from the base module name (without prebuilt_ prefix) of a fragment's contents module to the
	// hidden API encoded dex jar path.
	contentModuleDexJarPaths bootDexJarByModule

	// Path to the image profile file on host (or empty, if profile is not generated).
	profilePathOnHost android.Path

	// Install path of the boot image profile if it needs to be installed in the APEX, or empty if not
	// needed.
	profileInstallPathInApex string
}

// DexBootJarPathForContentModule returns the path to the dex boot jar for specified module.
//
// The dex boot jar is one which has had hidden API encoding performed on it.
func (i BootclasspathFragmentApexContentInfo) DexBootJarPathForContentModule(module android.Module) (android.Path, error) {
	// A bootclasspath_fragment cannot use a prebuilt library so Name() will return the base name
	// without a prebuilt_ prefix so is safe to use as the key for the contentModuleDexJarPaths.
	name := module.Name()
	if dexJar, ok := i.contentModuleDexJarPaths[name]; ok {
		return dexJar, nil
	} else {
		return nil, fmt.Errorf("unknown bootclasspath_fragment content module %s, expected one of %s",
			name, strings.Join(android.SortedKeys(i.contentModuleDexJarPaths), ", "))
	}
}

func (i BootclasspathFragmentApexContentInfo) DexBootJarPathMap() bootDexJarByModule {
	return i.contentModuleDexJarPaths
}

func (i BootclasspathFragmentApexContentInfo) ProfilePathOnHost() android.Path {
	return i.profilePathOnHost
}

func (i BootclasspathFragmentApexContentInfo) ProfileInstallPathInApex() string {
	return i.profileInstallPathInApex
}

func (b *BootclasspathFragmentModule) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	tag := ctx.OtherModuleDependencyTag(dep)

	// If the module is a default module, do not check the tag
	if _, ok := dep.(*Defaults); ok {
		return true
	}
	if IsBootclasspathFragmentContentDepTag(tag) {
		// Boot image contents are automatically added to apex.
		return true
	}
	if android.IsMetaDependencyTag(tag) {
		// Cross-cutting metadata dependencies are metadata.
		return false
	}
	panic(fmt.Errorf("boot_image module %q should not have a dependency on %q via tag %s", b, dep, android.PrettyPrintTag(tag)))
}

func (b *BootclasspathFragmentModule) ShouldSupportSdkVersion(ctx android.BaseModuleContext, sdkVersion android.ApiLevel) error {
	return nil
}

// ComponentDepsMutator adds dependencies onto modules before any prebuilt modules without a
// corresponding source module are renamed. This means that adding a dependency using a name without
// a prebuilt_ prefix will always resolve to a source module and when using a name with that prefix
// it will always resolve to a prebuilt module.
func (b *BootclasspathFragmentModule) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	module := ctx.Module()
	_, isSourceModule := module.(*BootclasspathFragmentModule)

	for _, name := range b.properties.Contents {
		// A bootclasspath_fragment must depend only on other source modules, while the
		// prebuilt_bootclasspath_fragment must only depend on other prebuilt modules.
		//
		// TODO(b/177892522) - avoid special handling of jacocoagent.
		if !isSourceModule && name != "jacocoagent" {
			name = android.PrebuiltNameFromSource(name)
		}
		ctx.AddDependency(module, bootclasspathFragmentContentDepTag, name)
	}

}

func (b *BootclasspathFragmentModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies onto all the modules that provide the API stubs for classes on this
	// bootclasspath fragment.
	hiddenAPIAddStubLibDependencies(ctx, b.properties.apiScopeToStubLibs())

	for _, additionalStubModule := range b.properties.Additional_stubs {
		for _, apiScope := range hiddenAPISdkLibrarySupportedScopes {
			// Add a dependency onto a possibly scope specific stub library.
			scopeSpecificDependency := apiScope.scopeSpecificStubModule(ctx, additionalStubModule)
			tag := hiddenAPIStubsDependencyTag{apiScope: apiScope, fromAdditionalDependency: true}
			ctx.AddVariationDependencies(nil, tag, scopeSpecificDependency)
		}
	}

	if !dexpreopt.IsDex2oatNeeded(ctx) {
		return
	}

	// Add a dependency onto the dex2oat tool which is needed for creating the boot image. The
	// path is retrieved from the dependency by GetGlobalSoongConfig(ctx).
	dexpreopt.RegisterToolDeps(ctx)
}

func (b *BootclasspathFragmentModule) BootclasspathDepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies on all the fragments.
	b.properties.BootclasspathFragmentsDepsProperties.addDependenciesOntoFragments(ctx)
}

func (b *BootclasspathFragmentModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Only perform a consistency check if this module is the active module. That will prevent an
	// unused prebuilt that was created without instrumentation from breaking an instrumentation
	// build.
	if isActiveModule(ctx.Module()) {
		b.bootclasspathFragmentPropertyCheck(ctx)
	}

	// Generate classpaths.proto config
	b.generateClasspathProtoBuildActions(ctx)

	// Gather the bootclasspath fragment's contents.
	var contents []android.Module
	ctx.VisitDirectDeps(func(module android.Module) {
		tag := ctx.OtherModuleDependencyTag(module)
		if IsBootclasspathFragmentContentDepTag(tag) {
			contents = append(contents, module)
		}
	})

	fragments := gatherApexModulePairDepsWithTag(ctx, bootclasspathFragmentDepTag)

	// Perform hidden API processing.
	hiddenAPIOutput := b.generateHiddenAPIBuildActions(ctx, contents, fragments)

	if android.IsModulePrebuilt(ctx.Module()) {
		b.profilePath = ctx.Module().(*PrebuiltBootclasspathFragmentModule).produceBootImageProfile(ctx)
	} else {
		b.profilePath = b.produceBootImageProfileFromSource(ctx, contents, hiddenAPIOutput.EncodedBootDexFilesByModule)
		// Provide the apex content info. A prebuilt fragment cannot contribute to an apex.
		b.provideApexContentInfo(ctx, hiddenAPIOutput, b.profilePath)
	}

	// In order for information about bootclasspath_fragment modules to be added to module-info.json
	// it is necessary to output an entry to Make. As bootclasspath_fragment modules are part of an
	// APEX there can be multiple variants, including the default/platform variant and only one can
	// be output to Make but it does not really matter which variant is output. The default/platform
	// variant is the first (ctx.PrimaryModule()) and is usually hidden from make so this just picks
	// the last variant (ctx.FinalModule()).
	if ctx.Module() != ctx.FinalModule() {
		b.HideFromMake()
	}
	android.SetProvider(ctx, testing.TestModuleProviderKey, testing.TestModuleProviderData{})
}

// getProfileProviderApex returns the name of the apex that provides a boot image profile, or an
// empty string if this module should not provide a boot image profile.
func (b *BootclasspathFragmentModule) getProfileProviderApex(ctx android.BaseModuleContext) string {
	// Only use the profile from the module that is preferred.
	if !isActiveModule(ctx.Module()) {
		return ""
	}

	// Bootclasspath fragment modules that are for the platform do not produce boot related files.
	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	for _, apex := range apexInfo.InApexVariants {
		if isProfileProviderApex(ctx, apex) {
			return apex
		}
	}

	return ""
}

// provideApexContentInfo creates, initializes and stores the apex content info for use by other
// modules.
func (b *BootclasspathFragmentModule) provideApexContentInfo(ctx android.ModuleContext, hiddenAPIOutput *HiddenAPIOutput, profile android.WritablePath) {
	// Construct the apex content info from the config.
	info := BootclasspathFragmentApexContentInfo{
		// Populate the apex content info with paths to the dex jars.
		contentModuleDexJarPaths: hiddenAPIOutput.EncodedBootDexFilesByModule,
	}

	if profile != nil {
		info.profilePathOnHost = profile
		info.profileInstallPathInApex = ProfileInstallPathInApex
	}

	// Make the apex content info available for other modules.
	android.SetProvider(ctx, BootclasspathFragmentApexContentInfoProvider, info)
}

// generateClasspathProtoBuildActions generates all required build actions for classpath.proto config
func (b *BootclasspathFragmentModule) generateClasspathProtoBuildActions(ctx android.ModuleContext) {
	var classpathJars []classpathJar
	configuredJars := b.configuredJars(ctx)
	if "art" == proptools.String(b.properties.Image_name) {
		// ART and platform boot jars must have a corresponding entry in DEX2OATBOOTCLASSPATH
		classpathJars = configuredJarListToClasspathJars(ctx, configuredJars, BOOTCLASSPATH, DEX2OATBOOTCLASSPATH)
	} else {
		classpathJars = configuredJarListToClasspathJars(ctx, configuredJars, b.classpathType)
	}
	b.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, configuredJars, classpathJars)
}

func (b *BootclasspathFragmentModule) configuredJars(ctx android.ModuleContext) android.ConfiguredJarList {
	global := dexpreopt.GetGlobalConfig(ctx)

	if "art" == proptools.String(b.properties.Image_name) {
		return global.ArtApexJars
	}

	possibleUpdatableModules := gatherPossibleApexModuleNamesAndStems(ctx, b.properties.Contents, bootclasspathFragmentContentDepTag)
	jars, unknown := global.ApexBootJars.Filter(possibleUpdatableModules)

	// TODO(satayev): for apex_test we want to include all contents unconditionally to classpaths
	// config. However, any test specific jars would not be present in ApexBootJars. Instead,
	// we should check if we are creating a config for apex_test via ApexInfo and amend the values.
	// This is an exception to support end-to-end test for SdkExtensions, until such support exists.
	if android.InList("test_framework-sdkextensions", possibleUpdatableModules) {
		jars = jars.Append("com.android.sdkext", "test_framework-sdkextensions")
	} else if android.InList("test_framework-apexd", possibleUpdatableModules) {
		jars = jars.Append("com.android.apex.test_package", "test_framework-apexd")
	} else if global.ApexBootJars.Len() != 0 {
		unknown = android.RemoveListFromList(unknown, b.properties.Coverage.Contents)
		_, unknown = android.RemoveFromList("core-icu4j", unknown)
		// This module only exists in car products.
		// So ignore it even if it is not in PRODUCT_APEX_BOOT_JARS.
		// TODO(b/202896428): Add better way to handle this.
		_, unknown = android.RemoveFromList("android.car-module", unknown)
		if isActiveModule(ctx.Module()) && len(unknown) > 0 {
			ctx.ModuleErrorf("%s in contents must also be declared in PRODUCT_APEX_BOOT_JARS", unknown)
		}
	}
	return jars
}

// generateHiddenAPIBuildActions generates all the hidden API related build rules.
func (b *BootclasspathFragmentModule) generateHiddenAPIBuildActions(ctx android.ModuleContext, contents []android.Module, fragments []android.Module) *HiddenAPIOutput {

	// Create hidden API input structure.
	input := b.createHiddenAPIFlagInput(ctx, contents, fragments)

	// Delegate the production of the hidden API all-flags.csv file to a module type specific method.
	common := ctx.Module().(commonBootclasspathFragment)
	output := common.produceHiddenAPIOutput(ctx, contents, fragments, input)

	// If the source or prebuilts module does not provide a signature patterns file then generate one
	// from the flags.
	// TODO(b/192868581): Remove once the source and prebuilts provide a signature patterns file of
	//  their own.
	if output.SignaturePatternsPath == nil {
		output.SignaturePatternsPath = buildRuleSignaturePatternsFile(
			ctx, output.AllFlagsPath, []string{"*"}, nil, nil, "")
	}

	// Initialize a HiddenAPIInfo structure.
	hiddenAPIInfo := HiddenAPIInfo{
		// The monolithic hidden API processing needs access to the flag files that override the default
		// flags from all the fragments whether or not they actually perform their own hidden API flag
		// generation. That is because the monolithic hidden API processing uses those flag files to
		// perform its own flag generation.
		FlagFilesByCategory: input.FlagFilesByCategory,

		// Other bootclasspath_fragments that depend on this need the transitive set of stub dex jars
		// from this to resolve any references from their code to classes provided by this fragment
		// and the fragments this depends upon.
		TransitiveStubDexJarsByScope: input.transitiveStubDexJarsByScope(),
	}

	// The monolithic hidden API processing also needs access to all the output files produced by
	// hidden API processing of this fragment.
	hiddenAPIInfo.HiddenAPIFlagOutput = output.HiddenAPIFlagOutput

	//  Provide it for use by other modules.
	android.SetProvider(ctx, HiddenAPIInfoProvider, hiddenAPIInfo)

	return output
}

// createHiddenAPIFlagInput creates a HiddenAPIFlagInput struct and initializes it with information derived
// from the properties on this module and its dependencies.
func (b *BootclasspathFragmentModule) createHiddenAPIFlagInput(ctx android.ModuleContext, contents []android.Module, fragments []android.Module) HiddenAPIFlagInput {
	// Merge the HiddenAPIInfo from all the fragment dependencies.
	dependencyHiddenApiInfo := newHiddenAPIInfo()
	dependencyHiddenApiInfo.mergeFromFragmentDeps(ctx, fragments)

	// Create hidden API flag input structure.
	input := newHiddenAPIFlagInput()

	// Update the input structure with information obtained from the stub libraries.
	input.gatherStubLibInfo(ctx, contents)

	// Populate with flag file paths from the properties.
	input.extractFlagFilesFromProperties(ctx, &b.properties.HiddenAPIFlagFileProperties)

	// Populate with package rules from the properties.
	input.extractPackageRulesFromProperties(&b.sourceOnlyProperties.HiddenAPIPackageProperties)

	input.gatherPropertyInfo(ctx, contents)

	// Add the stub dex jars from this module's fragment dependencies.
	input.DependencyStubDexJarsByScope.addStubDexJarsByModule(dependencyHiddenApiInfo.TransitiveStubDexJarsByScope)

	return input
}

// isTestFragment returns true if the current module is a test bootclasspath_fragment.
func (b *BootclasspathFragmentModule) isTestFragment() bool {
	return b.testFragment
}

// generateHiddenApiFlagRules generates rules to generate hidden API flags and compute the signature
// patterns file.
func (b *BootclasspathFragmentModule) generateHiddenApiFlagRules(ctx android.ModuleContext, contents []android.Module, input HiddenAPIFlagInput, bootDexInfoByModule bootDexInfoByModule, suffix string) HiddenAPIFlagOutput {
	// Generate the rules to create the hidden API flags and update the supplied hiddenAPIInfo with the
	// paths to the created files.
	flagOutput := hiddenAPIFlagRulesForBootclasspathFragment(ctx, bootDexInfoByModule, contents, input, suffix)

	// If the module specifies split_packages or package_prefixes then use those to generate the
	// signature patterns.
	splitPackages := input.SplitPackages
	packagePrefixes := input.PackagePrefixes
	singlePackages := input.SinglePackages
	if splitPackages != nil || packagePrefixes != nil || singlePackages != nil {
		flagOutput.SignaturePatternsPath = buildRuleSignaturePatternsFile(
			ctx, flagOutput.AllFlagsPath, splitPackages, packagePrefixes, singlePackages, suffix)
	} else if !b.isTestFragment() {
		ctx.ModuleErrorf(`Must specify at least one of the split_packages, package_prefixes and single_packages properties
  If this is a new bootclasspath_fragment or you are unsure what to do add the
  the following to the bootclasspath_fragment:
      hidden_api: {split_packages: ["*"]},
  and then run the following:
      m analyze_bcpf && analyze_bcpf --bcpf %q
  it will analyze the bootclasspath_fragment and provide hints as to what you
  should specify here. If you are happy with its suggestions then you can add
  the --fix option and it will fix them for you.`, b.BaseModuleName())
	}
	return flagOutput
}

// produceHiddenAPIOutput produces the hidden API all-flags.csv file (and supporting files)
// for the fragment as well as encoding the flags in the boot dex jars.
func (b *BootclasspathFragmentModule) produceHiddenAPIOutput(ctx android.ModuleContext, contents []android.Module, fragments []android.Module, input HiddenAPIFlagInput) *HiddenAPIOutput {
	// Gather information about the boot dex files for the boot libraries provided by this fragment.
	bootDexInfoByModule := extractBootDexInfoFromModules(ctx, contents)

	// Generate the flag file needed to encode into the dex files.
	flagOutput := b.generateHiddenApiFlagRules(ctx, contents, input, bootDexInfoByModule, "")

	// Encode those flags into the dex files of the contents of this fragment.
	encodedBootDexFilesByModule := hiddenAPIEncodeRulesForBootclasspathFragment(ctx, bootDexInfoByModule, flagOutput.AllFlagsPath)

	// Store that information for return for use by other rules.
	output := &HiddenAPIOutput{
		HiddenAPIFlagOutput:         flagOutput,
		EncodedBootDexFilesByModule: encodedBootDexFilesByModule,
	}

	// Get the ApiLevel associated with SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE, defaulting to current
	// if not set.
	config := ctx.Config()
	targetApiLevel := android.ApiLevelOrPanic(ctx,
		config.GetenvWithDefault("SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE", "current"))

	// Filter the contents list to remove any modules that do not support the target build release.
	// The current build release supports all the modules.
	contentsForSdkSnapshot := []android.Module{}
	for _, module := range contents {
		// If the module has a min_sdk_version that is higher than the target build release then it will
		// not work on the target build release and so must not be included in the sdk snapshot.
		minApiLevel := android.MinApiLevelForSdkSnapshot(ctx, module)
		if minApiLevel.GreaterThan(targetApiLevel) {
			continue
		}

		contentsForSdkSnapshot = append(contentsForSdkSnapshot, module)
	}

	var flagFilesByCategory FlagFilesByCategory
	if len(contentsForSdkSnapshot) != len(contents) {
		// The sdk snapshot has different contents to the runtime fragment so it is not possible to
		// reuse the hidden API information generated for the fragment. So, recompute that information
		// for the sdk snapshot.
		filteredInput := b.createHiddenAPIFlagInput(ctx, contentsForSdkSnapshot, fragments)

		// Gather information about the boot dex files for the boot libraries provided by this fragment.
		filteredBootDexInfoByModule := extractBootDexInfoFromModules(ctx, contentsForSdkSnapshot)
		flagOutput = b.generateHiddenApiFlagRules(ctx, contentsForSdkSnapshot, filteredInput, filteredBootDexInfoByModule, "-for-sdk-snapshot")
		flagFilesByCategory = filteredInput.FlagFilesByCategory
	} else {
		// The sdk snapshot has the same contents as the runtime fragment so reuse that information.
		flagFilesByCategory = input.FlagFilesByCategory
	}

	// Make the information available for the sdk snapshot.
	android.SetProvider(ctx, HiddenAPIInfoForSdkProvider, HiddenAPIInfoForSdk{
		FlagFilesByCategory: flagFilesByCategory,
		HiddenAPIFlagOutput: flagOutput,
	})

	return output
}

// produceBootImageProfileFromSource builds the boot image profile from the source if it is required.
func (b *BootclasspathFragmentModule) produceBootImageProfileFromSource(ctx android.ModuleContext, contents []android.Module, modules bootDexJarByModule) android.WritablePath {
	apex := b.getProfileProviderApex(ctx)
	if apex == "" {
		return nil
	}

	dexPaths := make(android.Paths, 0, len(contents))
	dexLocations := make([]string, 0, len(contents))
	for _, module := range contents {
		dexPaths = append(dexPaths, modules[module.Name()])
		dexLocations = append(dexLocations, filepath.Join("/", "apex", apex, "javalib", module.Name()+".jar"))
	}

	// Build a profile for the modules in this fragment.
	return bootImageProfileRuleCommon(ctx, b.Name(), dexPaths, dexLocations)
}

func (b *BootclasspathFragmentModule) AndroidMkEntries() []android.AndroidMkEntries {
	// Use the generated classpath proto as the output.
	outputFile := b.outputFilepath
	// Create a fake entry that will cause this to be added to the module-info.json file.
	entriesList := []android.AndroidMkEntries{{
		Class:      "FAKE",
		OutputFile: android.OptionalPathForPath(outputFile),
		Include:    "$(BUILD_PHONY_PACKAGE)",
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string) {
				// Allow the bootclasspath_fragment to be built by simply passing its name on the command
				// line.
				fmt.Fprintln(w, ".PHONY:", b.Name())
				fmt.Fprintln(w, b.Name()+":", outputFile.String())
			},
		},
	}}
	return entriesList
}

func (b *BootclasspathFragmentModule) getProfilePath() android.Path {
	return b.profilePath
}

// Collect information for opening IDE project files in java/jdeps.go.
func (b *BootclasspathFragmentModule) IDEInfo(dpInfo *android.IdeInfo) {
	dpInfo.Deps = append(dpInfo.Deps, b.properties.Contents...)
}

type bootclasspathFragmentMemberType struct {
	android.SdkMemberTypeBase
}

func (b *bootclasspathFragmentMemberType) AddDependencies(ctx android.SdkDependencyContext, dependencyTag blueprint.DependencyTag, names []string) {
	ctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (b *bootclasspathFragmentMemberType) IsInstance(module android.Module) bool {
	_, ok := module.(*BootclasspathFragmentModule)
	return ok
}

func (b *bootclasspathFragmentMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	if b.PropertyName == "boot_images" {
		return ctx.SnapshotBuilder().AddPrebuiltModule(member, "prebuilt_boot_image")
	} else {
		return ctx.SnapshotBuilder().AddPrebuiltModule(member, "prebuilt_bootclasspath_fragment")
	}
}

func (b *bootclasspathFragmentMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &bootclasspathFragmentSdkMemberProperties{}
}

type bootclasspathFragmentSdkMemberProperties struct {
	android.SdkMemberPropertiesBase

	// The image name
	Image_name *string

	// Contents of the bootclasspath fragment
	Contents []string

	// Stub_libs properties.
	Stub_libs               []string
	Core_platform_stub_libs []string

	// Fragment properties
	Fragments []ApexVariantReference

	// Flag files by *hiddenAPIFlagFileCategory
	Flag_files_by_category FlagFilesByCategory

	// The path to the generated annotation-flags.csv file.
	Annotation_flags_path android.OptionalPath

	// The path to the generated metadata.csv file.
	Metadata_path android.OptionalPath

	// The path to the generated index.csv file.
	Index_path android.OptionalPath

	// The path to the generated stub-flags.csv file.
	Stub_flags_path android.OptionalPath `supported_build_releases:"S"`

	// The path to the generated all-flags.csv file.
	All_flags_path android.OptionalPath `supported_build_releases:"S"`

	// The path to the generated signature-patterns.csv file.
	Signature_patterns_path android.OptionalPath `supported_build_releases:"Tiramisu+"`

	// The path to the generated filtered-stub-flags.csv file.
	Filtered_stub_flags_path android.OptionalPath `supported_build_releases:"Tiramisu+"`

	// The path to the generated filtered-flags.csv file.
	Filtered_flags_path android.OptionalPath `supported_build_releases:"Tiramisu+"`
}

func (b *bootclasspathFragmentSdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	module := variant.(*BootclasspathFragmentModule)

	b.Image_name = module.properties.Image_name
	b.Contents = module.properties.Contents

	// Get the hidden API information from the module.
	mctx := ctx.SdkModuleContext()
	hiddenAPIInfo, _ := android.OtherModuleProvider(mctx, module, HiddenAPIInfoForSdkProvider)
	b.Flag_files_by_category = hiddenAPIInfo.FlagFilesByCategory

	// Copy all the generated file paths.
	b.Annotation_flags_path = android.OptionalPathForPath(hiddenAPIInfo.AnnotationFlagsPath)
	b.Metadata_path = android.OptionalPathForPath(hiddenAPIInfo.MetadataPath)
	b.Index_path = android.OptionalPathForPath(hiddenAPIInfo.IndexPath)

	b.Stub_flags_path = android.OptionalPathForPath(hiddenAPIInfo.StubFlagsPath)
	b.All_flags_path = android.OptionalPathForPath(hiddenAPIInfo.AllFlagsPath)

	b.Signature_patterns_path = android.OptionalPathForPath(hiddenAPIInfo.SignaturePatternsPath)
	b.Filtered_stub_flags_path = android.OptionalPathForPath(hiddenAPIInfo.FilteredStubFlagsPath)
	b.Filtered_flags_path = android.OptionalPathForPath(hiddenAPIInfo.FilteredFlagsPath)

	// Copy stub_libs properties.
	b.Stub_libs = module.properties.Api.Stub_libs
	b.Core_platform_stub_libs = module.properties.Core_platform_api.Stub_libs

	// Copy fragment properties.
	b.Fragments = module.properties.Fragments
}

func (b *bootclasspathFragmentSdkMemberProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	if b.Image_name != nil {
		propertySet.AddProperty("image_name", *b.Image_name)
	}

	builder := ctx.SnapshotBuilder()
	requiredMemberDependency := builder.SdkMemberReferencePropertyTag(true)

	if len(b.Contents) > 0 {
		propertySet.AddPropertyWithTag("contents", b.Contents, requiredMemberDependency)
	}

	if len(b.Stub_libs) > 0 {
		apiPropertySet := propertySet.AddPropertySet("api")
		apiPropertySet.AddPropertyWithTag("stub_libs", b.Stub_libs, requiredMemberDependency)
	}
	if len(b.Core_platform_stub_libs) > 0 {
		corePlatformApiPropertySet := propertySet.AddPropertySet("core_platform_api")
		corePlatformApiPropertySet.AddPropertyWithTag("stub_libs", b.Core_platform_stub_libs, requiredMemberDependency)
	}
	if len(b.Fragments) > 0 {
		propertySet.AddProperty("fragments", b.Fragments)
	}

	hiddenAPISet := propertySet.AddPropertySet("hidden_api")
	hiddenAPIDir := "hiddenapi"

	// Copy manually curated flag files specified on the bootclasspath_fragment.
	if b.Flag_files_by_category != nil {
		for _, category := range HiddenAPIFlagFileCategories {
			paths := b.Flag_files_by_category[category]
			if len(paths) > 0 {
				dests := []string{}
				for _, p := range paths {
					dest := filepath.Join(hiddenAPIDir, p.Base())
					builder.CopyToSnapshot(p, dest)
					dests = append(dests, dest)
				}
				hiddenAPISet.AddProperty(category.PropertyName(), dests)
			}
		}
	}

	copyOptionalPath := func(path android.OptionalPath, property string) {
		if path.Valid() {
			p := path.Path()
			dest := filepath.Join(hiddenAPIDir, p.Base())
			builder.CopyToSnapshot(p, dest)
			hiddenAPISet.AddProperty(property, dest)
		}
	}

	// Copy all the generated files, if available.
	copyOptionalPath(b.Annotation_flags_path, "annotation_flags")
	copyOptionalPath(b.Metadata_path, "metadata")
	copyOptionalPath(b.Index_path, "index")

	copyOptionalPath(b.Stub_flags_path, "stub_flags")
	copyOptionalPath(b.All_flags_path, "all_flags")

	copyOptionalPath(b.Signature_patterns_path, "signature_patterns")
	copyOptionalPath(b.Filtered_stub_flags_path, "filtered_stub_flags")
	copyOptionalPath(b.Filtered_flags_path, "filtered_flags")
}

var _ android.SdkMemberType = (*bootclasspathFragmentMemberType)(nil)

// prebuiltBootclasspathFragmentProperties contains additional prebuilt_bootclasspath_fragment
// specific properties.
type prebuiltBootclasspathFragmentProperties struct {
	Hidden_api struct {
		// The path to the annotation-flags.csv file created by the bootclasspath_fragment.
		Annotation_flags *string `android:"path"`

		// The path to the metadata.csv file created by the bootclasspath_fragment.
		Metadata *string `android:"path"`

		// The path to the index.csv file created by the bootclasspath_fragment.
		Index *string `android:"path"`

		// The path to the signature-patterns.csv file created by the bootclasspath_fragment.
		Signature_patterns *string `android:"path"`

		// The path to the stub-flags.csv file created by the bootclasspath_fragment.
		Stub_flags *string `android:"path"`

		// The path to the all-flags.csv file created by the bootclasspath_fragment.
		All_flags *string `android:"path"`

		// The path to the filtered-stub-flags.csv file created by the bootclasspath_fragment.
		Filtered_stub_flags *string `android:"path"`

		// The path to the filtered-flags.csv file created by the bootclasspath_fragment.
		Filtered_flags *string `android:"path"`
	}
}

// A prebuilt version of the bootclasspath_fragment module.
//
// At the moment this is basically just a bootclasspath_fragment module that can be used as a
// prebuilt. Eventually as more functionality is migrated into the bootclasspath_fragment module
// type from the various singletons then this will diverge.
type PrebuiltBootclasspathFragmentModule struct {
	BootclasspathFragmentModule
	prebuilt android.Prebuilt

	// Additional prebuilt specific properties.
	prebuiltProperties prebuiltBootclasspathFragmentProperties
}

func (module *PrebuiltBootclasspathFragmentModule) Prebuilt() *android.Prebuilt {
	return &module.prebuilt
}

func (module *PrebuiltBootclasspathFragmentModule) Name() string {
	return module.prebuilt.Name(module.ModuleBase.Name())
}

// produceHiddenAPIOutput returns a path to the prebuilt all-flags.csv or nil if none is specified.
func (module *PrebuiltBootclasspathFragmentModule) produceHiddenAPIOutput(ctx android.ModuleContext, contents []android.Module, fragments []android.Module, input HiddenAPIFlagInput) *HiddenAPIOutput {
	pathForOptionalSrc := func(src *string, defaultPath android.Path) android.Path {
		if src == nil {
			return defaultPath
		}
		return android.PathForModuleSrc(ctx, *src)
	}
	pathForSrc := func(property string, src *string) android.Path {
		if src == nil {
			ctx.PropertyErrorf(property, "is required but was not specified")
			return android.PathForModuleSrc(ctx, "missing", property)
		}
		return android.PathForModuleSrc(ctx, *src)
	}

	output := HiddenAPIOutput{
		HiddenAPIFlagOutput: HiddenAPIFlagOutput{
			AnnotationFlagsPath:   pathForSrc("hidden_api.annotation_flags", module.prebuiltProperties.Hidden_api.Annotation_flags),
			MetadataPath:          pathForSrc("hidden_api.metadata", module.prebuiltProperties.Hidden_api.Metadata),
			IndexPath:             pathForSrc("hidden_api.index", module.prebuiltProperties.Hidden_api.Index),
			SignaturePatternsPath: pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Signature_patterns, nil),
			// TODO: Temporarily handle stub_flags/all_flags properties until prebuilts have been updated.
			StubFlagsPath: pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Stub_flags, nil),
			AllFlagsPath:  pathForOptionalSrc(module.prebuiltProperties.Hidden_api.All_flags, nil),
		},
	}

	// TODO: Temporarily fallback to stub_flags/all_flags properties until prebuilts have been updated.
	output.FilteredStubFlagsPath = pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Filtered_stub_flags, output.StubFlagsPath)
	output.FilteredFlagsPath = pathForOptionalSrc(module.prebuiltProperties.Hidden_api.Filtered_flags, output.AllFlagsPath)

	return &output
}

// produceBootImageProfile extracts the boot image profile from the APEX if available.
func (module *PrebuiltBootclasspathFragmentModule) produceBootImageProfile(ctx android.ModuleContext) android.WritablePath {
	// This module does not provide a boot image profile.
	if module.getProfileProviderApex(ctx) == "" {
		return nil
	}

	di, err := android.FindDeapexerProviderForModule(ctx)
	if err != nil {
		// An error was found, possibly due to multiple apexes in the tree that export this library
		// Defer the error till a client tries to call getProfilePath
		module.profilePathErr = err
		return nil // An error has been reported by FindDeapexerProviderForModule.
	}

	return di.PrebuiltExportPath(ProfileInstallPathInApex)
}

func (b *PrebuiltBootclasspathFragmentModule) getProfilePath() android.Path {
	if b.profilePathErr != nil {
		panic(b.profilePathErr.Error())
	}
	return b.profilePath
}

var _ commonBootclasspathFragment = (*PrebuiltBootclasspathFragmentModule)(nil)

// RequiredFilesFromPrebuiltApex returns the list of all files the prebuilt_bootclasspath_fragment
// requires from a prebuilt .apex file.
//
// If there is no image config associated with this fragment then it returns nil. Otherwise, it
// returns the files that are listed in the image config.
func (module *PrebuiltBootclasspathFragmentModule) RequiredFilesFromPrebuiltApex(ctx android.BaseModuleContext) []string {
	for _, apex := range module.ApexProperties.Apex_available {
		if isProfileProviderApex(ctx, apex) {
			return []string{ProfileInstallPathInApex}
		}
	}
	return nil
}

func (module *PrebuiltBootclasspathFragmentModule) UseProfileGuidedDexpreopt() bool {
	return false
}

var _ android.RequiredFilesFromPrebuiltApex = (*PrebuiltBootclasspathFragmentModule)(nil)

func prebuiltBootclasspathFragmentFactory() android.Module {
	m := &PrebuiltBootclasspathFragmentModule{}
	m.AddProperties(&m.properties, &m.prebuiltProperties)
	// This doesn't actually have any prebuilt files of its own so pass a placeholder for the srcs
	// array.
	android.InitPrebuiltModule(m, &[]string{"placeholder"})
	android.InitApexModule(m)
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibCommon)

	return m
}
