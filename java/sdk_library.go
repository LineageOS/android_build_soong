// Copyright 2018 Google Inc. All rights reserved.
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
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/dexpreopt"
)

const (
	sdkXmlFileSuffix = ".xml"
)

// A tag to associated a dependency with a specific api scope.
type scopeDependencyTag struct {
	blueprint.BaseDependencyTag
	name     string
	apiScope *apiScope

	// Function for extracting appropriate path information from the dependency.
	depInfoExtractor func(paths *scopePaths, ctx android.ModuleContext, dep android.Module) error
}

// Extract tag specific information from the dependency.
func (tag scopeDependencyTag) extractDepInfo(ctx android.ModuleContext, dep android.Module, paths *scopePaths) {
	err := tag.depInfoExtractor(paths, ctx, dep)
	if err != nil {
		ctx.ModuleErrorf("has an invalid {scopeDependencyTag: %s} dependency on module %s: %s", tag.name, ctx.OtherModuleName(dep), err.Error())
	}
}

var _ android.ReplaceSourceWithPrebuilt = (*scopeDependencyTag)(nil)

func (tag scopeDependencyTag) ReplaceSourceWithPrebuilt() bool {
	return false
}

// Provides information about an api scope, e.g. public, system, test.
type apiScope struct {
	// The name of the api scope, e.g. public, system, test
	name string

	// The api scope that this scope extends.
	extends *apiScope

	// The legacy enabled status for a specific scope can be dependent on other
	// properties that have been specified on the library so it is provided by
	// a function that can determine the status by examining those properties.
	legacyEnabledStatus func(module *SdkLibrary) bool

	// The default enabled status for non-legacy behavior, which is triggered by
	// explicitly enabling at least one api scope.
	defaultEnabledStatus bool

	// Gets a pointer to the scope specific properties.
	scopeSpecificProperties func(module *SdkLibrary) *ApiScopeProperties

	// The name of the field in the dynamically created structure.
	fieldName string

	// The name of the property in the java_sdk_library_import
	propertyName string

	// The tag to use to depend on the stubs library module.
	stubsTag scopeDependencyTag

	// The tag to use to depend on the stubs source module (if separate from the API module).
	stubsSourceTag scopeDependencyTag

	// The tag to use to depend on the API file generating module (if separate from the stubs source module).
	apiFileTag scopeDependencyTag

	// The tag to use to depend on the stubs source and API module.
	stubsSourceAndApiTag scopeDependencyTag

	// The scope specific prefix to add to the api file base of "current.txt" or "removed.txt".
	apiFilePrefix string

	// The scope specific prefix to add to the sdk library module name to construct a scope specific
	// module name.
	moduleSuffix string

	// SDK version that the stubs library is built against. Note that this is always
	// *current. Older stubs library built with a numbered SDK version is created from
	// the prebuilt jar.
	sdkVersion string

	// The annotation that identifies this API level, empty for the public API scope.
	annotation string

	// Extra arguments to pass to droidstubs for this scope.
	//
	// This is not used directly but is used to construct the droidstubsArgs.
	extraArgs []string

	// The args that must be passed to droidstubs to generate the API and stubs source
	// for this scope, constructed dynamically by initApiScope().
	//
	// The API only includes the additional members that this scope adds over the scope
	// that it extends.
	//
	// The stubs source must include the definitions of everything that is in this
	// api scope and all the scopes that this one extends.
	droidstubsArgs []string

	// Whether the api scope can be treated as unstable, and should skip compat checks.
	unstable bool
}

// Initialize a scope, creating and adding appropriate dependency tags
func initApiScope(scope *apiScope) *apiScope {
	name := scope.name
	scopeByName[name] = scope
	allScopeNames = append(allScopeNames, name)
	scope.propertyName = strings.ReplaceAll(name, "-", "_")
	scope.fieldName = proptools.FieldNameForProperty(scope.propertyName)
	scope.stubsTag = scopeDependencyTag{
		name:             name + "-stubs",
		apiScope:         scope,
		depInfoExtractor: (*scopePaths).extractStubsLibraryInfoFromDependency,
	}
	scope.stubsSourceTag = scopeDependencyTag{
		name:             name + "-stubs-source",
		apiScope:         scope,
		depInfoExtractor: (*scopePaths).extractStubsSourceInfoFromDep,
	}
	scope.apiFileTag = scopeDependencyTag{
		name:             name + "-api",
		apiScope:         scope,
		depInfoExtractor: (*scopePaths).extractApiInfoFromDep,
	}
	scope.stubsSourceAndApiTag = scopeDependencyTag{
		name:             name + "-stubs-source-and-api",
		apiScope:         scope,
		depInfoExtractor: (*scopePaths).extractStubsSourceAndApiInfoFromApiStubsProvider,
	}

	// To get the args needed to generate the stubs source append all the args from
	// this scope and all the scopes it extends as each set of args adds additional
	// members to the stubs.
	var scopeSpecificArgs []string
	if scope.annotation != "" {
		scopeSpecificArgs = []string{"--show-annotation", scope.annotation}
	}
	for s := scope; s != nil; s = s.extends {
		scopeSpecificArgs = append(scopeSpecificArgs, s.extraArgs...)

		// Ensure that the generated stubs includes all the API elements from the API scope
		// that this scope extends.
		if s != scope && s.annotation != "" {
			scopeSpecificArgs = append(scopeSpecificArgs, "--show-for-stub-purposes-annotation", s.annotation)
		}
	}

	// Escape any special characters in the arguments. This is needed because droidstubs
	// passes these directly to the shell command.
	scope.droidstubsArgs = proptools.ShellEscapeList(scopeSpecificArgs)

	return scope
}

func (scope *apiScope) stubsLibraryModuleNameSuffix() string {
	return ".stubs" + scope.moduleSuffix
}

func (scope *apiScope) stubsLibraryModuleName(baseName string) string {
	return baseName + scope.stubsLibraryModuleNameSuffix()
}

func (scope *apiScope) stubsSourceModuleName(baseName string) string {
	return baseName + ".stubs.source" + scope.moduleSuffix
}

func (scope *apiScope) apiModuleName(baseName string) string {
	return baseName + ".api" + scope.moduleSuffix
}

func (scope *apiScope) String() string {
	return scope.name
}

type apiScopes []*apiScope

func (scopes apiScopes) Strings(accessor func(*apiScope) string) []string {
	var list []string
	for _, scope := range scopes {
		list = append(list, accessor(scope))
	}
	return list
}

var (
	scopeByName    = make(map[string]*apiScope)
	allScopeNames  []string
	apiScopePublic = initApiScope(&apiScope{
		name: "public",

		// Public scope is enabled by default for both legacy and non-legacy modes.
		legacyEnabledStatus: func(module *SdkLibrary) bool {
			return true
		},
		defaultEnabledStatus: true,

		scopeSpecificProperties: func(module *SdkLibrary) *ApiScopeProperties {
			return &module.sdkLibraryProperties.Public
		},
		sdkVersion: "current",
	})
	apiScopeSystem = initApiScope(&apiScope{
		name:                "system",
		extends:             apiScopePublic,
		legacyEnabledStatus: (*SdkLibrary).generateTestAndSystemScopesByDefault,
		scopeSpecificProperties: func(module *SdkLibrary) *ApiScopeProperties {
			return &module.sdkLibraryProperties.System
		},
		apiFilePrefix: "system-",
		moduleSuffix:  ".system",
		sdkVersion:    "system_current",
		annotation:    "android.annotation.SystemApi(client=android.annotation.SystemApi.Client.PRIVILEGED_APPS)",
	})
	apiScopeTest = initApiScope(&apiScope{
		name:                "test",
		extends:             apiScopeSystem,
		legacyEnabledStatus: (*SdkLibrary).generateTestAndSystemScopesByDefault,
		scopeSpecificProperties: func(module *SdkLibrary) *ApiScopeProperties {
			return &module.sdkLibraryProperties.Test
		},
		apiFilePrefix: "test-",
		moduleSuffix:  ".test",
		sdkVersion:    "test_current",
		annotation:    "android.annotation.TestApi",
		unstable:      true,
	})
	apiScopeModuleLib = initApiScope(&apiScope{
		name:    "module-lib",
		extends: apiScopeSystem,
		// The module-lib scope is disabled by default in legacy mode.
		//
		// Enabling this would break existing usages.
		legacyEnabledStatus: func(module *SdkLibrary) bool {
			return false
		},
		scopeSpecificProperties: func(module *SdkLibrary) *ApiScopeProperties {
			return &module.sdkLibraryProperties.Module_lib
		},
		apiFilePrefix: "module-lib-",
		moduleSuffix:  ".module_lib",
		sdkVersion:    "module_current",
		annotation:    "android.annotation.SystemApi(client=android.annotation.SystemApi.Client.MODULE_LIBRARIES)",
	})
	apiScopeSystemServer = initApiScope(&apiScope{
		name:    "system-server",
		extends: apiScopePublic,
		// The system-server scope is disabled by default in legacy mode.
		//
		// Enabling this would break existing usages.
		legacyEnabledStatus: func(module *SdkLibrary) bool {
			return false
		},
		scopeSpecificProperties: func(module *SdkLibrary) *ApiScopeProperties {
			return &module.sdkLibraryProperties.System_server
		},
		apiFilePrefix: "system-server-",
		moduleSuffix:  ".system_server",
		sdkVersion:    "system_server_current",
		annotation:    "android.annotation.SystemApi(client=android.annotation.SystemApi.Client.SYSTEM_SERVER)",
		extraArgs: []string{
			"--hide-annotation", "android.annotation.Hide",
			// com.android.* classes are okay in this interface"
			"--hide", "InternalClasses",
		},
	})
	allApiScopes = apiScopes{
		apiScopePublic,
		apiScopeSystem,
		apiScopeTest,
		apiScopeModuleLib,
		apiScopeSystemServer,
	}
)

var (
	javaSdkLibrariesLock sync.Mutex
)

// TODO: these are big features that are currently missing
// 1) disallowing linking to the runtime shared lib
// 2) HTML generation

func init() {
	RegisterSdkLibraryBuildComponents(android.InitRegistrationContext)

	android.RegisterMakeVarsProvider(pctx, func(ctx android.MakeVarsContext) {
		javaSdkLibraries := javaSdkLibraries(ctx.Config())
		sort.Strings(*javaSdkLibraries)
		ctx.Strict("JAVA_SDK_LIBRARIES", strings.Join(*javaSdkLibraries, " "))
	})

	// Register sdk member types.
	android.RegisterSdkMemberType(javaSdkLibrarySdkMemberType)
}

func RegisterSdkLibraryBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_sdk_library", SdkLibraryFactory)
	ctx.RegisterModuleType("java_sdk_library_import", sdkLibraryImportFactory)
}

// Properties associated with each api scope.
type ApiScopeProperties struct {
	// Indicates whether the api surface is generated.
	//
	// If this is set for any scope then all scopes must explicitly specify if they
	// are enabled. This is to prevent new usages from depending on legacy behavior.
	//
	// Otherwise, if this is not set for any scope then the default  behavior is
	// scope specific so please refer to the scope specific property documentation.
	Enabled *bool

	// The sdk_version to use for building the stubs.
	//
	// If not specified then it will use an sdk_version determined as follows:
	//
	// 1) If the sdk_version specified on the java_sdk_library is none then this
	// will be none. This is used for java_sdk_library instances that are used
	// to create stubs that contribute to the core_current sdk version.
	// 2) Otherwise, it is assumed that this library extends but does not
	// contribute directly to a specific sdk_version and so this uses the
	// sdk_version appropriate for the api scope. e.g. public will use
	// sdk_version: current, system will use sdk_version: system_current, etc.
	//
	// This does not affect the sdk_version used for either generating the stubs source
	// or the API file. They both have to use the same sdk_version as is used for
	// compiling the implementation library.
	Sdk_version *string
}

type sdkLibraryProperties struct {
	// List of source files that are needed to compile the API, but are not part of runtime library.
	Api_srcs []string `android:"arch_variant"`

	// Visibility for impl library module. If not specified then defaults to the
	// visibility property.
	Impl_library_visibility []string

	// Visibility for stubs library modules. If not specified then defaults to the
	// visibility property.
	Stubs_library_visibility []string

	// Visibility for stubs source modules. If not specified then defaults to the
	// visibility property.
	Stubs_source_visibility []string

	// List of Java libraries that will be in the classpath when building the implementation lib
	Impl_only_libs []string `android:"arch_variant"`

	// List of Java libraries that will included in the implementation lib.
	Impl_only_static_libs []string `android:"arch_variant"`

	// List of Java libraries that will be in the classpath when building stubs
	Stub_only_libs []string `android:"arch_variant"`

	// List of Java libraries that will included in stub libraries
	Stub_only_static_libs []string `android:"arch_variant"`

	// list of package names that will be documented and publicized as API.
	// This allows the API to be restricted to a subset of the source files provided.
	// If this is unspecified then all the source files will be treated as being part
	// of the API.
	Api_packages []string

	// list of package names that must be hidden from the API
	Hidden_api_packages []string

	// the relative path to the directory containing the api specification files.
	// Defaults to "api".
	Api_dir *string

	// Determines whether a runtime implementation library is built; defaults to false.
	//
	// If true then it also prevents the module from being used as a shared module, i.e.
	// it is as is shared_library: false, was set.
	Api_only *bool

	// local files that are used within user customized droiddoc options.
	Droiddoc_option_files []string

	// additional droiddoc options.
	// Available variables for substitution:
	//
	//  $(location <label>): the path to the droiddoc_option_files with name <label>
	Droiddoc_options []string

	// is set to true, Metalava will allow framework SDK to contain annotations.
	Annotations_enabled *bool

	// a list of top-level directories containing files to merge qualifier annotations
	// (i.e. those intended to be included in the stubs written) from.
	Merge_annotations_dirs []string

	// a list of top-level directories containing Java stub files to merge show/hide annotations from.
	Merge_inclusion_annotations_dirs []string

	// If set to true then don't create dist rules.
	No_dist *bool

	// The stem for the artifacts that are copied to the dist, if not specified
	// then defaults to the base module name.
	//
	// For each scope the following artifacts are copied to the apistubs/<scope>
	// directory in the dist.
	// * stubs impl jar -> <dist-stem>.jar
	// * API specification file -> api/<dist-stem>.txt
	// * Removed API specification file -> api/<dist-stem>-removed.txt
	//
	// Also used to construct the name of the filegroup (created by prebuilt_apis)
	// that references the latest released API and remove API specification files.
	// * API specification filegroup -> <dist-stem>.api.<scope>.latest
	// * Removed API specification filegroup -> <dist-stem>-removed.api.<scope>.latest
	// * API incompatibilities baseline filegroup -> <dist-stem>-incompatibilities.api.<scope>.latest
	Dist_stem *string

	// The subdirectory for the artifacts that are copied to the dist directory.  If not specified
	// then defaults to "unknown".  Should be set to "android" for anything that should be published
	// in the public Android SDK.
	Dist_group *string

	// A compatibility mode that allows historical API-tracking files to not exist.
	// Do not use.
	Unsafe_ignore_missing_latest_api bool

	// indicates whether system and test apis should be generated.
	Generate_system_and_test_apis bool `blueprint:"mutated"`

	// The properties specific to the public api scope
	//
	// Unless explicitly specified by using public.enabled the public api scope is
	// enabled by default in both legacy and non-legacy mode.
	Public ApiScopeProperties

	// The properties specific to the system api scope
	//
	// In legacy mode the system api scope is enabled by default when sdk_version
	// is set to something other than "none".
	//
	// In non-legacy mode the system api scope is disabled by default.
	System ApiScopeProperties

	// The properties specific to the test api scope
	//
	// In legacy mode the test api scope is enabled by default when sdk_version
	// is set to something other than "none".
	//
	// In non-legacy mode the test api scope is disabled by default.
	Test ApiScopeProperties

	// The properties specific to the module-lib api scope
	//
	// Unless explicitly specified by using test.enabled the module-lib api scope is
	// disabled by default.
	Module_lib ApiScopeProperties

	// The properties specific to the system-server api scope
	//
	// Unless explicitly specified by using test.enabled the module-lib api scope is
	// disabled by default.
	System_server ApiScopeProperties

	// Determines if the stubs are preferred over the implementation library
	// for linking, even when the client doesn't specify sdk_version. When this
	// is set to true, such clients are provided with the widest API surface that
	// this lib provides. Note however that this option doesn't affect the clients
	// that are in the same APEX as this library. In that case, the clients are
	// always linked with the implementation library. Default is false.
	Default_to_stubs *bool

	// Properties related to api linting.
	Api_lint struct {
		// Enable api linting.
		Enabled *bool
	}

	// TODO: determines whether to create HTML doc or not
	//Html_doc *bool
}

// Paths to outputs from java_sdk_library and java_sdk_library_import.
//
// Fields that are android.Paths are always set (during GenerateAndroidBuildActions).
// OptionalPaths are always set by java_sdk_library but may not be set by
// java_sdk_library_import as not all instances provide that information.
type scopePaths struct {
	// The path (represented as Paths for convenience when returning) to the stubs header jar.
	//
	// That is the jar that is created by turbine.
	stubsHeaderPath android.Paths

	// The path (represented as Paths for convenience when returning) to the stubs implementation jar.
	//
	// This is not the implementation jar, it still only contains stubs.
	stubsImplPath android.Paths

	// The dex jar for the stubs.
	//
	// This is not the implementation jar, it still only contains stubs.
	stubsDexJarPath OptionalDexJarPath

	// The API specification file, e.g. system_current.txt.
	currentApiFilePath android.OptionalPath

	// The specification of API elements removed since the last release.
	removedApiFilePath android.OptionalPath

	// The stubs source jar.
	stubsSrcJar android.OptionalPath

	// Extracted annotations.
	annotationsZip android.OptionalPath
}

func (paths *scopePaths) extractStubsLibraryInfoFromDependency(ctx android.ModuleContext, dep android.Module) error {
	if ctx.OtherModuleHasProvider(dep, JavaInfoProvider) {
		lib := ctx.OtherModuleProvider(dep, JavaInfoProvider).(JavaInfo)
		paths.stubsHeaderPath = lib.HeaderJars
		paths.stubsImplPath = lib.ImplementationJars

		libDep := dep.(UsesLibraryDependency)
		paths.stubsDexJarPath = libDep.DexJarBuildPath()
		return nil
	} else {
		return fmt.Errorf("expected module that has JavaInfoProvider, e.g. java_library")
	}
}

func (paths *scopePaths) treatDepAsApiStubsProvider(dep android.Module, action func(provider ApiStubsProvider)) error {
	if apiStubsProvider, ok := dep.(ApiStubsProvider); ok {
		action(apiStubsProvider)
		return nil
	} else {
		return fmt.Errorf("expected module that implements ApiStubsProvider, e.g. droidstubs")
	}
}

func (paths *scopePaths) treatDepAsApiStubsSrcProvider(dep android.Module, action func(provider ApiStubsSrcProvider)) error {
	if apiStubsProvider, ok := dep.(ApiStubsSrcProvider); ok {
		action(apiStubsProvider)
		return nil
	} else {
		return fmt.Errorf("expected module that implements ApiStubsSrcProvider, e.g. droidstubs")
	}
}

func (paths *scopePaths) extractApiInfoFromApiStubsProvider(provider ApiStubsProvider) {
	paths.annotationsZip = android.OptionalPathForPath(provider.AnnotationsZip())
	paths.currentApiFilePath = android.OptionalPathForPath(provider.ApiFilePath())
	paths.removedApiFilePath = android.OptionalPathForPath(provider.RemovedApiFilePath())
}

func (paths *scopePaths) extractApiInfoFromDep(ctx android.ModuleContext, dep android.Module) error {
	return paths.treatDepAsApiStubsProvider(dep, func(provider ApiStubsProvider) {
		paths.extractApiInfoFromApiStubsProvider(provider)
	})
}

func (paths *scopePaths) extractStubsSourceInfoFromApiStubsProviders(provider ApiStubsSrcProvider) {
	paths.stubsSrcJar = android.OptionalPathForPath(provider.StubsSrcJar())
}

func (paths *scopePaths) extractStubsSourceInfoFromDep(ctx android.ModuleContext, dep android.Module) error {
	return paths.treatDepAsApiStubsSrcProvider(dep, func(provider ApiStubsSrcProvider) {
		paths.extractStubsSourceInfoFromApiStubsProviders(provider)
	})
}

func (paths *scopePaths) extractStubsSourceAndApiInfoFromApiStubsProvider(ctx android.ModuleContext, dep android.Module) error {
	return paths.treatDepAsApiStubsProvider(dep, func(provider ApiStubsProvider) {
		paths.extractApiInfoFromApiStubsProvider(provider)
		paths.extractStubsSourceInfoFromApiStubsProviders(provider)
	})
}

type commonToSdkLibraryAndImportProperties struct {
	// The naming scheme to use for the components that this module creates.
	//
	// If not specified then it defaults to "default".
	//
	// This is a temporary mechanism to simplify conversion from separate modules for each
	// component that follow a different naming pattern to the default one.
	//
	// TODO(b/155480189) - Remove once naming inconsistencies have been resolved.
	Naming_scheme *string

	// Specifies whether this module can be used as an Android shared library; defaults
	// to true.
	//
	// An Android shared library is one that can be referenced in a <uses-library> element
	// in an AndroidManifest.xml.
	Shared_library *bool

	// Files containing information about supported java doc tags.
	Doctag_files []string `android:"path"`

	// Signals that this shared library is part of the bootclasspath starting
	// on the version indicated in this attribute.
	//
	// This will make platforms at this level and above to ignore
	// <uses-library> tags with this library name because the library is already
	// available
	On_bootclasspath_since *string

	// Signals that this shared library was part of the bootclasspath before
	// (but not including) the version indicated in this attribute.
	//
	// The system will automatically add a <uses-library> tag with this library to
	// apps that target any SDK less than the version indicated in this attribute.
	On_bootclasspath_before *string

	// Indicates that PackageManager should ignore this shared library if the
	// platform is below the version indicated in this attribute.
	//
	// This means that the device won't recognise this library as installed.
	Min_device_sdk *string

	// Indicates that PackageManager should ignore this shared library if the
	// platform is above the version indicated in this attribute.
	//
	// This means that the device won't recognise this library as installed.
	Max_device_sdk *string
}

// commonSdkLibraryAndImportModule defines the interface that must be provided by a module that
// embeds the commonToSdkLibraryAndImport struct.
type commonSdkLibraryAndImportModule interface {
	android.SdkAware

	BaseModuleName() string
}

// Common code between sdk library and sdk library import
type commonToSdkLibraryAndImport struct {
	module commonSdkLibraryAndImportModule

	scopePaths map[*apiScope]*scopePaths

	namingScheme sdkLibraryComponentNamingScheme

	commonSdkLibraryProperties commonToSdkLibraryAndImportProperties

	// Paths to commonSdkLibraryProperties.Doctag_files
	doctagPaths android.Paths

	// Functionality related to this being used as a component of a java_sdk_library.
	EmbeddableSdkLibraryComponent
}

func (c *commonToSdkLibraryAndImport) initCommon(module commonSdkLibraryAndImportModule) {
	c.module = module

	module.AddProperties(&c.commonSdkLibraryProperties)

	// Initialize this as an sdk library component.
	c.initSdkLibraryComponent(module)
}

func (c *commonToSdkLibraryAndImport) initCommonAfterDefaultsApplied(ctx android.DefaultableHookContext) bool {
	schemeProperty := proptools.StringDefault(c.commonSdkLibraryProperties.Naming_scheme, "default")
	switch schemeProperty {
	case "default":
		c.namingScheme = &defaultNamingScheme{}
	default:
		ctx.PropertyErrorf("naming_scheme", "expected 'default' but was %q", schemeProperty)
		return false
	}

	namePtr := proptools.StringPtr(c.module.BaseModuleName())
	c.sdkLibraryComponentProperties.SdkLibraryName = namePtr

	// Only track this sdk library if this can be used as a shared library.
	if c.sharedLibrary() {
		// Use the name specified in the module definition as the owner.
		c.sdkLibraryComponentProperties.SdkLibraryToImplicitlyTrack = namePtr
	}

	return true
}

// uniqueApexVariations provides common implementation of the ApexModule.UniqueApexVariations
// method.
func (c *commonToSdkLibraryAndImport) uniqueApexVariations() bool {
	// A java_sdk_library that is a shared library produces an XML file that makes the shared library
	// usable from an AndroidManifest.xml's <uses-library> entry. That XML file contains the name of
	// the APEX and so it needs a unique variation per APEX.
	return c.sharedLibrary()
}

func (c *commonToSdkLibraryAndImport) generateCommonBuildActions(ctx android.ModuleContext) {
	c.doctagPaths = android.PathsForModuleSrc(ctx, c.commonSdkLibraryProperties.Doctag_files)
}

// Module name of the runtime implementation library
func (c *commonToSdkLibraryAndImport) implLibraryModuleName() string {
	return c.module.BaseModuleName() + ".impl"
}

// Module name of the XML file for the lib
func (c *commonToSdkLibraryAndImport) xmlPermissionsModuleName() string {
	return c.module.BaseModuleName() + sdkXmlFileSuffix
}

// Name of the java_library module that compiles the stubs source.
func (c *commonToSdkLibraryAndImport) stubsLibraryModuleName(apiScope *apiScope) string {
	baseName := c.module.BaseModuleName()
	return c.module.SdkMemberComponentName(baseName, func(name string) string {
		return c.namingScheme.stubsLibraryModuleName(apiScope, name)
	})
}

// Name of the droidstubs module that generates the stubs source and may also
// generate/check the API.
func (c *commonToSdkLibraryAndImport) stubsSourceModuleName(apiScope *apiScope) string {
	baseName := c.module.BaseModuleName()
	return c.module.SdkMemberComponentName(baseName, func(name string) string {
		return c.namingScheme.stubsSourceModuleName(apiScope, name)
	})
}

// The component names for different outputs of the java_sdk_library.
//
// They are similar to the names used for the child modules it creates
const (
	stubsSourceComponentName = "stubs.source"

	apiTxtComponentName = "api.txt"

	removedApiTxtComponentName = "removed-api.txt"

	annotationsComponentName = "annotations.zip"
)

// A regular expression to match tags that reference a specific stubs component.
//
// It will only match if given a valid scope and a valid component. It is verfy strict
// to ensure it does not accidentally match a similar looking tag that should be processed
// by the embedded Library.
var tagSplitter = func() *regexp.Regexp {
	// Given a list of literal string items returns a regular expression that will
	// match any one of the items.
	choice := func(items ...string) string {
		return `\Q` + strings.Join(items, `\E|\Q`) + `\E`
	}

	// Regular expression to match one of the scopes.
	scopesRegexp := choice(allScopeNames...)

	// Regular expression to match one of the components.
	componentsRegexp := choice(stubsSourceComponentName, apiTxtComponentName, removedApiTxtComponentName, annotationsComponentName)

	// Regular expression to match any combination of one scope and one component.
	return regexp.MustCompile(fmt.Sprintf(`^\.(%s)\.(%s)$`, scopesRegexp, componentsRegexp))
}()

// For OutputFileProducer interface
//
// .<scope>.<component name>, for all ComponentNames (for example: .public.removed-api.txt)
func (c *commonToSdkLibraryAndImport) commonOutputFiles(tag string) (android.Paths, error) {
	if groups := tagSplitter.FindStringSubmatch(tag); groups != nil {
		scopeName := groups[1]
		component := groups[2]

		if scope, ok := scopeByName[scopeName]; ok {
			paths := c.findScopePaths(scope)
			if paths == nil {
				return nil, fmt.Errorf("%q does not provide api scope %s", c.module.BaseModuleName(), scopeName)
			}

			switch component {
			case stubsSourceComponentName:
				if paths.stubsSrcJar.Valid() {
					return android.Paths{paths.stubsSrcJar.Path()}, nil
				}

			case apiTxtComponentName:
				if paths.currentApiFilePath.Valid() {
					return android.Paths{paths.currentApiFilePath.Path()}, nil
				}

			case removedApiTxtComponentName:
				if paths.removedApiFilePath.Valid() {
					return android.Paths{paths.removedApiFilePath.Path()}, nil
				}

			case annotationsComponentName:
				if paths.annotationsZip.Valid() {
					return android.Paths{paths.annotationsZip.Path()}, nil
				}
			}

			return nil, fmt.Errorf("%s not available for api scope %s", component, scopeName)
		} else {
			return nil, fmt.Errorf("unknown scope %s in %s", scope, tag)
		}

	} else {
		switch tag {
		case ".doctags":
			if c.doctagPaths != nil {
				return c.doctagPaths, nil
			} else {
				return nil, fmt.Errorf("no doctag_files specified on %s", c.module.BaseModuleName())
			}
		}
		return nil, nil
	}
}

func (c *commonToSdkLibraryAndImport) getScopePathsCreateIfNeeded(scope *apiScope) *scopePaths {
	if c.scopePaths == nil {
		c.scopePaths = make(map[*apiScope]*scopePaths)
	}
	paths := c.scopePaths[scope]
	if paths == nil {
		paths = &scopePaths{}
		c.scopePaths[scope] = paths
	}

	return paths
}

func (c *commonToSdkLibraryAndImport) findScopePaths(scope *apiScope) *scopePaths {
	if c.scopePaths == nil {
		return nil
	}

	return c.scopePaths[scope]
}

// If this does not support the requested api scope then find the closest available
// scope it does support. Returns nil if no such scope is available.
func (c *commonToSdkLibraryAndImport) findClosestScopePath(scope *apiScope) *scopePaths {
	for s := scope; s != nil; s = s.extends {
		if paths := c.findScopePaths(s); paths != nil {
			return paths
		}
	}

	// This should never happen outside tests as public should be the base scope for every
	// scope and is enabled by default.
	return nil
}

func (c *commonToSdkLibraryAndImport) selectHeaderJarsForSdkVersion(ctx android.BaseModuleContext, sdkVersion android.SdkSpec) android.Paths {

	// If a specific numeric version has been requested then use prebuilt versions of the sdk.
	if !sdkVersion.ApiLevel.IsPreview() {
		return PrebuiltJars(ctx, c.module.BaseModuleName(), sdkVersion)
	}

	paths := c.selectScopePaths(ctx, sdkVersion.Kind)
	if paths == nil {
		return nil
	}

	return paths.stubsHeaderPath
}

// selectScopePaths returns the *scopePaths appropriate for the specific kind.
//
// If the module does not support the specific kind then it will return the *scopePaths for the
// closest kind which is a subset of the requested kind. e.g. if requesting android.SdkModule then
// it will return *scopePaths for android.SdkSystem if available or android.SdkPublic of not.
func (c *commonToSdkLibraryAndImport) selectScopePaths(ctx android.BaseModuleContext, kind android.SdkKind) *scopePaths {
	apiScope := sdkKindToApiScope(kind)

	paths := c.findClosestScopePath(apiScope)
	if paths == nil {
		var scopes []string
		for _, s := range allApiScopes {
			if c.findScopePaths(s) != nil {
				scopes = append(scopes, s.name)
			}
		}
		ctx.ModuleErrorf("requires api scope %s from %s but it only has %q available", apiScope.name, c.module.BaseModuleName(), scopes)
		return nil
	}

	return paths
}

// sdkKindToApiScope maps from android.SdkKind to apiScope.
func sdkKindToApiScope(kind android.SdkKind) *apiScope {
	var apiScope *apiScope
	switch kind {
	case android.SdkSystem:
		apiScope = apiScopeSystem
	case android.SdkModule:
		apiScope = apiScopeModuleLib
	case android.SdkTest:
		apiScope = apiScopeTest
	case android.SdkSystemServer:
		apiScope = apiScopeSystemServer
	default:
		apiScope = apiScopePublic
	}
	return apiScope
}

// to satisfy SdkLibraryDependency interface
func (c *commonToSdkLibraryAndImport) SdkApiStubDexJar(ctx android.BaseModuleContext, kind android.SdkKind) OptionalDexJarPath {
	paths := c.selectScopePaths(ctx, kind)
	if paths == nil {
		return makeUnsetDexJarPath()
	}

	return paths.stubsDexJarPath
}

// to satisfy SdkLibraryDependency interface
func (c *commonToSdkLibraryAndImport) SdkRemovedTxtFile(ctx android.BaseModuleContext, kind android.SdkKind) android.OptionalPath {
	apiScope := sdkKindToApiScope(kind)
	paths := c.findScopePaths(apiScope)
	if paths == nil {
		return android.OptionalPath{}
	}

	return paths.removedApiFilePath
}

func (c *commonToSdkLibraryAndImport) sdkComponentPropertiesForChildLibrary() interface{} {
	componentProps := &struct {
		SdkLibraryName              *string
		SdkLibraryToImplicitlyTrack *string
	}{}

	namePtr := proptools.StringPtr(c.module.BaseModuleName())
	componentProps.SdkLibraryName = namePtr

	if c.sharedLibrary() {
		// Mark the stubs library as being components of this java_sdk_library so that
		// any app that includes code which depends (directly or indirectly) on the stubs
		// library will have the appropriate <uses-library> invocation inserted into its
		// manifest if necessary.
		componentProps.SdkLibraryToImplicitlyTrack = namePtr
	}

	return componentProps
}

func (c *commonToSdkLibraryAndImport) sharedLibrary() bool {
	return proptools.BoolDefault(c.commonSdkLibraryProperties.Shared_library, true)
}

// Check if the stub libraries should be compiled for dex
func (c *commonToSdkLibraryAndImport) stubLibrariesCompiledForDex() bool {
	// Always compile the dex file files for the stub libraries if they will be used on the
	// bootclasspath.
	return !c.sharedLibrary()
}

// Properties related to the use of a module as an component of a java_sdk_library.
type SdkLibraryComponentProperties struct {
	// The name of the java_sdk_library/_import module.
	SdkLibraryName *string `blueprint:"mutated"`

	// The name of the java_sdk_library/_import to add to a <uses-library> entry
	// in the AndroidManifest.xml of any Android app that includes code that references
	// this module. If not set then no java_sdk_library/_import is tracked.
	SdkLibraryToImplicitlyTrack *string `blueprint:"mutated"`
}

// Structure to be embedded in a module struct that needs to support the
// SdkLibraryComponentDependency interface.
type EmbeddableSdkLibraryComponent struct {
	sdkLibraryComponentProperties SdkLibraryComponentProperties
}

func (e *EmbeddableSdkLibraryComponent) initSdkLibraryComponent(module android.Module) {
	module.AddProperties(&e.sdkLibraryComponentProperties)
}

// to satisfy SdkLibraryComponentDependency
func (e *EmbeddableSdkLibraryComponent) SdkLibraryName() *string {
	return e.sdkLibraryComponentProperties.SdkLibraryName
}

// to satisfy SdkLibraryComponentDependency
func (e *EmbeddableSdkLibraryComponent) OptionalSdkLibraryImplementation() *string {
	// For shared libraries, this is the same as the SDK library name. If a Java library or app
	// depends on a component library (e.g. a stub library) it still needs to know the name of the
	// run-time library and the corresponding module that provides the implementation. This name is
	// passed to manifest_fixer (to be added to AndroidManifest.xml) and added to CLC (to be used
	// in dexpreopt).
	//
	// For non-shared SDK (component or not) libraries this returns `nil`, as they are not
	// <uses-library> and should not be added to the manifest or to CLC.
	return e.sdkLibraryComponentProperties.SdkLibraryToImplicitlyTrack
}

// Implemented by modules that are (or possibly could be) a component of a java_sdk_library
// (including the java_sdk_library) itself.
type SdkLibraryComponentDependency interface {
	UsesLibraryDependency

	// SdkLibraryName returns the name of the java_sdk_library/_import module.
	SdkLibraryName() *string

	// The name of the implementation library for the optional SDK library or nil, if there isn't one.
	OptionalSdkLibraryImplementation() *string
}

// Make sure that all the module types that are components of java_sdk_library/_import
// and which can be referenced (directly or indirectly) from an android app implement
// the SdkLibraryComponentDependency interface.
var _ SdkLibraryComponentDependency = (*Library)(nil)
var _ SdkLibraryComponentDependency = (*Import)(nil)
var _ SdkLibraryComponentDependency = (*SdkLibrary)(nil)
var _ SdkLibraryComponentDependency = (*SdkLibraryImport)(nil)

// Provides access to sdk_version related files, e.g. header and implementation jars.
type SdkLibraryDependency interface {
	SdkLibraryComponentDependency

	// Get the header jars appropriate for the supplied sdk_version.
	//
	// These are turbine generated jars so they only change if the externals of the
	// class changes but it does not contain and implementation or JavaDoc.
	SdkHeaderJars(ctx android.BaseModuleContext, sdkVersion android.SdkSpec) android.Paths

	// Get the implementation jars appropriate for the supplied sdk version.
	//
	// These are either the implementation jar for the whole sdk library or the implementation
	// jars for the stubs. The latter should only be needed when generating JavaDoc as otherwise
	// they are identical to the corresponding header jars.
	SdkImplementationJars(ctx android.BaseModuleContext, sdkVersion android.SdkSpec) android.Paths

	// SdkApiStubDexJar returns the dex jar for the stubs. It is needed by the hiddenapi processing
	// tool which processes dex files.
	SdkApiStubDexJar(ctx android.BaseModuleContext, kind android.SdkKind) OptionalDexJarPath

	// SdkRemovedTxtFile returns the optional path to the removed.txt file for the specified sdk kind.
	SdkRemovedTxtFile(ctx android.BaseModuleContext, kind android.SdkKind) android.OptionalPath

	// sharedLibrary returns true if this can be used as a shared library.
	sharedLibrary() bool
}

type SdkLibrary struct {
	Library

	sdkLibraryProperties sdkLibraryProperties

	// Map from api scope to the scope specific property structure.
	scopeToProperties map[*apiScope]*ApiScopeProperties

	commonToSdkLibraryAndImport
}

var _ SdkLibraryDependency = (*SdkLibrary)(nil)

func (module *SdkLibrary) generateTestAndSystemScopesByDefault() bool {
	return module.sdkLibraryProperties.Generate_system_and_test_apis
}

func (module *SdkLibrary) getGeneratedApiScopes(ctx android.EarlyModuleContext) apiScopes {
	// Check to see if any scopes have been explicitly enabled. If any have then all
	// must be.
	anyScopesExplicitlyEnabled := false
	for _, scope := range allApiScopes {
		scopeProperties := module.scopeToProperties[scope]
		if scopeProperties.Enabled != nil {
			anyScopesExplicitlyEnabled = true
			break
		}
	}

	var generatedScopes apiScopes
	enabledScopes := make(map[*apiScope]struct{})
	for _, scope := range allApiScopes {
		scopeProperties := module.scopeToProperties[scope]
		// If any scopes are explicitly enabled then ignore the legacy enabled status.
		// This is to ensure that any new usages of this module type do not rely on legacy
		// behaviour.
		defaultEnabledStatus := false
		if anyScopesExplicitlyEnabled {
			defaultEnabledStatus = scope.defaultEnabledStatus
		} else {
			defaultEnabledStatus = scope.legacyEnabledStatus(module)
		}
		enabled := proptools.BoolDefault(scopeProperties.Enabled, defaultEnabledStatus)
		if enabled {
			enabledScopes[scope] = struct{}{}
			generatedScopes = append(generatedScopes, scope)
		}
	}

	// Now check to make sure that any scope that is extended by an enabled scope is also
	// enabled.
	for _, scope := range allApiScopes {
		if _, ok := enabledScopes[scope]; ok {
			extends := scope.extends
			if extends != nil {
				if _, ok := enabledScopes[extends]; !ok {
					ctx.ModuleErrorf("enabled api scope %q depends on disabled scope %q", scope, extends)
				}
			}
		}
	}

	return generatedScopes
}

var _ android.ModuleWithMinSdkVersionCheck = (*SdkLibrary)(nil)

func (module *SdkLibrary) CheckMinSdkVersion(ctx android.ModuleContext) {
	android.CheckMinSdkVersion(ctx, module.MinSdkVersion(ctx).ApiLevel, func(c android.ModuleContext, do android.PayloadDepsCallback) {
		ctx.WalkDeps(func(child android.Module, parent android.Module) bool {
			isExternal := !module.depIsInSameApex(ctx, child)
			if am, ok := child.(android.ApexModule); ok {
				if !do(ctx, parent, am, isExternal) {
					return false
				}
			}
			return !isExternal
		})
	})
}

type sdkLibraryComponentTag struct {
	blueprint.BaseDependencyTag
	name string
}

// Mark this tag so dependencies that use it are excluded from visibility enforcement.
func (t sdkLibraryComponentTag) ExcludeFromVisibilityEnforcement() {}

var xmlPermissionsFileTag = sdkLibraryComponentTag{name: "xml-permissions-file"}

func IsXmlPermissionsFileDepTag(depTag blueprint.DependencyTag) bool {
	if dt, ok := depTag.(sdkLibraryComponentTag); ok {
		return dt == xmlPermissionsFileTag
	}
	return false
}

var implLibraryTag = sdkLibraryComponentTag{name: "impl-library"}

// Add the dependencies on the child modules in the component deps mutator.
func (module *SdkLibrary) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	for _, apiScope := range module.getGeneratedApiScopes(ctx) {
		// Add dependencies to the stubs library
		ctx.AddVariationDependencies(nil, apiScope.stubsTag, module.stubsLibraryModuleName(apiScope))

		// Add a dependency on the stubs source in order to access both stubs source and api information.
		ctx.AddVariationDependencies(nil, apiScope.stubsSourceAndApiTag, module.stubsSourceModuleName(apiScope))
	}

	if module.requiresRuntimeImplementationLibrary() {
		// Add dependency to the rule for generating the implementation library.
		ctx.AddDependency(module, implLibraryTag, module.implLibraryModuleName())

		if module.sharedLibrary() {
			// Add dependency to the rule for generating the xml permissions file
			ctx.AddDependency(module, xmlPermissionsFileTag, module.xmlPermissionsModuleName())
		}
	}
}

// Add other dependencies as normal.
func (module *SdkLibrary) DepsMutator(ctx android.BottomUpMutatorContext) {
	var missingApiModules []string
	for _, apiScope := range module.getGeneratedApiScopes(ctx) {
		if apiScope.unstable {
			continue
		}
		if m := android.SrcIsModule(module.latestApiFilegroupName(apiScope)); !ctx.OtherModuleExists(m) {
			missingApiModules = append(missingApiModules, m)
		}
		if m := android.SrcIsModule(module.latestRemovedApiFilegroupName(apiScope)); !ctx.OtherModuleExists(m) {
			missingApiModules = append(missingApiModules, m)
		}
		if m := android.SrcIsModule(module.latestIncompatibilitiesFilegroupName(apiScope)); !ctx.OtherModuleExists(m) {
			missingApiModules = append(missingApiModules, m)
		}
	}
	if len(missingApiModules) != 0 && !module.sdkLibraryProperties.Unsafe_ignore_missing_latest_api {
		m := module.Name() + " is missing tracking files for previously released library versions.\n"
		m += "You need to do one of the following:\n"
		m += "- Add `unsafe_ignore_missing_latest_api: true` to your blueprint (to disable compat tracking)\n"
		m += "- Add a set of prebuilt txt files representing the last released version of this library for compat checking.\n"
		m += "  (the current set of API files can be used as a seed for this compatibility tracking\n"
		m += "\n"
		m += "The following filegroup modules are missing:\n  "
		m += strings.Join(missingApiModules, "\n  ") + "\n"
		m += "Please see the documentation of the prebuilt_apis module type (and a usage example in prebuilts/sdk) for a convenient way to generate these."
		ctx.ModuleErrorf(m)
	}
	if module.requiresRuntimeImplementationLibrary() {
		// Only add the deps for the library if it is actually going to be built.
		module.Library.deps(ctx)
	}
}

func (module *SdkLibrary) OutputFiles(tag string) (android.Paths, error) {
	paths, err := module.commonOutputFiles(tag)
	if paths != nil || err != nil {
		return paths, err
	}
	if module.requiresRuntimeImplementationLibrary() {
		return module.Library.OutputFiles(tag)
	}
	if tag == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("unsupported module reference tag %q", tag)
}

func (module *SdkLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if proptools.String(module.deviceProperties.Min_sdk_version) != "" {
		module.CheckMinSdkVersion(ctx)
	}

	module.generateCommonBuildActions(ctx)

	// Only build an implementation library if required.
	if module.requiresRuntimeImplementationLibrary() {
		module.Library.GenerateAndroidBuildActions(ctx)
	}

	// Collate the components exported by this module. All scope specific modules are exported but
	// the impl and xml component modules are not.
	exportedComponents := map[string]struct{}{}

	// Record the paths to the header jars of the library (stubs and impl).
	// When this java_sdk_library is depended upon from others via "libs" property,
	// the recorded paths will be returned depending on the link type of the caller.
	ctx.VisitDirectDeps(func(to android.Module) {
		tag := ctx.OtherModuleDependencyTag(to)

		// Extract information from any of the scope specific dependencies.
		if scopeTag, ok := tag.(scopeDependencyTag); ok {
			apiScope := scopeTag.apiScope
			scopePaths := module.getScopePathsCreateIfNeeded(apiScope)

			// Extract information from the dependency. The exact information extracted
			// is determined by the nature of the dependency which is determined by the tag.
			scopeTag.extractDepInfo(ctx, to, scopePaths)

			exportedComponents[ctx.OtherModuleName(to)] = struct{}{}
		}
	})

	// Make the set of components exported by this module available for use elsewhere.
	exportedComponentInfo := android.ExportedComponentsInfo{Components: android.SortedStringKeys(exportedComponents)}
	ctx.SetProvider(android.ExportedComponentsInfoProvider, exportedComponentInfo)
}

func (module *SdkLibrary) AndroidMkEntries() []android.AndroidMkEntries {
	if !module.requiresRuntimeImplementationLibrary() {
		return nil
	}
	entriesList := module.Library.AndroidMkEntries()
	if module.sharedLibrary() {
		entries := &entriesList[0]
		entries.Required = append(entries.Required, module.xmlPermissionsModuleName())
	}
	return entriesList
}

// The dist path of the stub artifacts
func (module *SdkLibrary) apiDistPath(apiScope *apiScope) string {
	return path.Join("apistubs", module.distGroup(), apiScope.name)
}

// Get the sdk version for use when compiling the stubs library.
func (module *SdkLibrary) sdkVersionForStubsLibrary(mctx android.EarlyModuleContext, apiScope *apiScope) string {
	scopeProperties := module.scopeToProperties[apiScope]
	if scopeProperties.Sdk_version != nil {
		return proptools.String(scopeProperties.Sdk_version)
	}

	sdkDep := decodeSdkDep(mctx, android.SdkContext(&module.Library))
	if sdkDep.hasStandardLibs() {
		// If building against a standard sdk then use the sdk version appropriate for the scope.
		return apiScope.sdkVersion
	} else {
		// Otherwise, use no system module.
		return "none"
	}
}

func (module *SdkLibrary) distStem() string {
	return proptools.StringDefault(module.sdkLibraryProperties.Dist_stem, module.BaseModuleName())
}

// distGroup returns the subdirectory of the dist path of the stub artifacts.
func (module *SdkLibrary) distGroup() string {
	return proptools.StringDefault(module.sdkLibraryProperties.Dist_group, "unknown")
}

func (module *SdkLibrary) latestApiFilegroupName(apiScope *apiScope) string {
	return ":" + module.distStem() + ".api." + apiScope.name + ".latest"
}

func (module *SdkLibrary) latestRemovedApiFilegroupName(apiScope *apiScope) string {
	return ":" + module.distStem() + "-removed.api." + apiScope.name + ".latest"
}

func (module *SdkLibrary) latestIncompatibilitiesFilegroupName(apiScope *apiScope) string {
	return ":" + module.distStem() + "-incompatibilities.api." + apiScope.name + ".latest"
}

func childModuleVisibility(childVisibility []string) []string {
	if childVisibility == nil {
		// No child visibility set. The child will use the visibility of the sdk_library.
		return nil
	}

	// Prepend an override to ignore the sdk_library's visibility, and rely on the child visibility.
	var visibility []string
	visibility = append(visibility, "//visibility:override")
	visibility = append(visibility, childVisibility...)
	return visibility
}

// Creates the implementation java library
func (module *SdkLibrary) createImplLibrary(mctx android.DefaultableHookContext) {
	visibility := childModuleVisibility(module.sdkLibraryProperties.Impl_library_visibility)

	props := struct {
		Name           *string
		Visibility     []string
		Instrument     bool
		Libs           []string
		Static_libs    []string
		Apex_available []string
	}{
		Name:       proptools.StringPtr(module.implLibraryModuleName()),
		Visibility: visibility,
		// Set the instrument property to ensure it is instrumented when instrumentation is required.
		Instrument: true,
		// Set the impl_only libs. Note that the module's "Libs" get appended as well, via the
		// addition of &module.properties below.
		Libs: module.sdkLibraryProperties.Impl_only_libs,
		// Set the impl_only static libs. Note that the module's "static_libs" get appended as well, via the
		// addition of &module.properties below.
		Static_libs: module.sdkLibraryProperties.Impl_only_static_libs,
		// Pass the apex_available settings down so that the impl library can be statically
		// embedded within a library that is added to an APEX. Needed for updatable-media.
		Apex_available: module.ApexAvailable(),
	}

	properties := []interface{}{
		&module.properties,
		&module.protoProperties,
		&module.deviceProperties,
		&module.dexProperties,
		&module.dexpreoptProperties,
		&module.linter.properties,
		&props,
		module.sdkComponentPropertiesForChildLibrary(),
	}
	mctx.CreateModule(LibraryFactory, properties...)
}

// Creates a static java library that has API stubs
func (module *SdkLibrary) createStubsLibrary(mctx android.DefaultableHookContext, apiScope *apiScope) {
	props := struct {
		Name           *string
		Visibility     []string
		Srcs           []string
		Installable    *bool
		Sdk_version    *string
		System_modules *string
		Patch_module   *string
		Libs           []string
		Static_libs    []string
		Compile_dex    *bool
		Java_version   *string
		Openjdk9       struct {
			Srcs       []string
			Javacflags []string
		}
		Dist struct {
			Targets []string
			Dest    *string
			Dir     *string
			Tag     *string
		}
	}{}

	props.Name = proptools.StringPtr(module.stubsLibraryModuleName(apiScope))
	props.Visibility = childModuleVisibility(module.sdkLibraryProperties.Stubs_library_visibility)
	// sources are generated from the droiddoc
	props.Srcs = []string{":" + module.stubsSourceModuleName(apiScope)}
	sdkVersion := module.sdkVersionForStubsLibrary(mctx, apiScope)
	props.Sdk_version = proptools.StringPtr(sdkVersion)
	props.System_modules = module.deviceProperties.System_modules
	props.Patch_module = module.properties.Patch_module
	props.Installable = proptools.BoolPtr(false)
	props.Libs = module.sdkLibraryProperties.Stub_only_libs
	props.Static_libs = module.sdkLibraryProperties.Stub_only_static_libs
	// The stub-annotations library contains special versions of the annotations
	// with CLASS retention policy, so that they're kept.
	if proptools.Bool(module.sdkLibraryProperties.Annotations_enabled) {
		props.Libs = append(props.Libs, "stub-annotations")
	}
	props.Openjdk9.Srcs = module.properties.Openjdk9.Srcs
	props.Openjdk9.Javacflags = module.properties.Openjdk9.Javacflags
	// We compile the stubs for 1.8 in line with the main android.jar stubs, and potential
	// interop with older developer tools that don't support 1.9.
	props.Java_version = proptools.StringPtr("1.8")

	// The imports need to be compiled to dex if the java_sdk_library requests it.
	compileDex := module.dexProperties.Compile_dex
	if module.stubLibrariesCompiledForDex() {
		compileDex = proptools.BoolPtr(true)
	}
	props.Compile_dex = compileDex

	// Dist the class jar artifact for sdk builds.
	if !Bool(module.sdkLibraryProperties.No_dist) {
		props.Dist.Targets = []string{"sdk", "win_sdk"}
		props.Dist.Dest = proptools.StringPtr(fmt.Sprintf("%v.jar", module.distStem()))
		props.Dist.Dir = proptools.StringPtr(module.apiDistPath(apiScope))
		props.Dist.Tag = proptools.StringPtr(".jar")
	}

	mctx.CreateModule(LibraryFactory, &props, module.sdkComponentPropertiesForChildLibrary())
}

// Creates a droidstubs module that creates stubs source files from the given full source
// files and also updates and checks the API specification files.
func (module *SdkLibrary) createStubsSourcesAndApi(mctx android.DefaultableHookContext, apiScope *apiScope, name string, scopeSpecificDroidstubsArgs []string) {
	props := struct {
		Name                             *string
		Visibility                       []string
		Srcs                             []string
		Installable                      *bool
		Sdk_version                      *string
		System_modules                   *string
		Libs                             []string
		Output_javadoc_comments          *bool
		Arg_files                        []string
		Args                             *string
		Java_version                     *string
		Annotations_enabled              *bool
		Merge_annotations_dirs           []string
		Merge_inclusion_annotations_dirs []string
		Generate_stubs                   *bool
		Previous_api                     *string
		Check_api                        struct {
			Current       ApiToCheck
			Last_released ApiToCheck

			Api_lint struct {
				Enabled       *bool
				New_since     *string
				Baseline_file *string
			}
		}
		Aidl struct {
			Include_dirs       []string
			Local_include_dirs []string
		}
		Dists []android.Dist
	}{}

	// The stubs source processing uses the same compile time classpath when extracting the
	// API from the implementation library as it does when compiling it. i.e. the same
	// * sdk version
	// * system_modules
	// * libs (static_libs/libs)

	props.Name = proptools.StringPtr(name)
	props.Visibility = childModuleVisibility(module.sdkLibraryProperties.Stubs_source_visibility)
	props.Srcs = append(props.Srcs, module.properties.Srcs...)
	props.Srcs = append(props.Srcs, module.sdkLibraryProperties.Api_srcs...)
	props.Sdk_version = module.deviceProperties.Sdk_version
	props.System_modules = module.deviceProperties.System_modules
	props.Installable = proptools.BoolPtr(false)
	// A droiddoc module has only one Libs property and doesn't distinguish between
	// shared libs and static libs. So we need to add both of these libs to Libs property.
	props.Libs = module.properties.Libs
	props.Libs = append(props.Libs, module.properties.Static_libs...)
	props.Aidl.Include_dirs = module.deviceProperties.Aidl.Include_dirs
	props.Aidl.Local_include_dirs = module.deviceProperties.Aidl.Local_include_dirs
	props.Java_version = module.properties.Java_version

	props.Annotations_enabled = module.sdkLibraryProperties.Annotations_enabled
	props.Merge_annotations_dirs = module.sdkLibraryProperties.Merge_annotations_dirs
	props.Merge_inclusion_annotations_dirs = module.sdkLibraryProperties.Merge_inclusion_annotations_dirs

	droidstubsArgs := []string{}
	if len(module.sdkLibraryProperties.Api_packages) != 0 {
		droidstubsArgs = append(droidstubsArgs, "--stub-packages "+strings.Join(module.sdkLibraryProperties.Api_packages, ":"))
	}
	if len(module.sdkLibraryProperties.Hidden_api_packages) != 0 {
		droidstubsArgs = append(droidstubsArgs,
			android.JoinWithPrefix(module.sdkLibraryProperties.Hidden_api_packages, " --hide-package "))
	}
	droidstubsArgs = append(droidstubsArgs, module.sdkLibraryProperties.Droiddoc_options...)
	disabledWarnings := []string{
		"BroadcastBehavior",
		"DeprecationMismatch",
		"HiddenSuperclass",
		"HiddenTypeParameter",
		"MissingPermission",
		"SdkConstant",
		"Todo",
		"Typo",
		"UnavailableSymbol",
	}
	droidstubsArgs = append(droidstubsArgs, android.JoinWithPrefix(disabledWarnings, "--hide "))

	// Output Javadoc comments for public scope.
	if apiScope == apiScopePublic {
		props.Output_javadoc_comments = proptools.BoolPtr(true)
	}

	// Add in scope specific arguments.
	droidstubsArgs = append(droidstubsArgs, scopeSpecificDroidstubsArgs...)
	props.Arg_files = module.sdkLibraryProperties.Droiddoc_option_files
	props.Args = proptools.StringPtr(strings.Join(droidstubsArgs, " "))

	// List of APIs identified from the provided source files are created. They are later
	// compared against to the not-yet-released (a.k.a current) list of APIs and to the
	// last-released (a.k.a numbered) list of API.
	currentApiFileName := apiScope.apiFilePrefix + "current.txt"
	removedApiFileName := apiScope.apiFilePrefix + "removed.txt"
	apiDir := module.getApiDir()
	currentApiFileName = path.Join(apiDir, currentApiFileName)
	removedApiFileName = path.Join(apiDir, removedApiFileName)

	// check against the not-yet-release API
	props.Check_api.Current.Api_file = proptools.StringPtr(currentApiFileName)
	props.Check_api.Current.Removed_api_file = proptools.StringPtr(removedApiFileName)

	if !(apiScope.unstable || module.sdkLibraryProperties.Unsafe_ignore_missing_latest_api) {
		// check against the latest released API
		latestApiFilegroupName := proptools.StringPtr(module.latestApiFilegroupName(apiScope))
		props.Previous_api = latestApiFilegroupName
		props.Check_api.Last_released.Api_file = latestApiFilegroupName
		props.Check_api.Last_released.Removed_api_file = proptools.StringPtr(
			module.latestRemovedApiFilegroupName(apiScope))
		props.Check_api.Last_released.Baseline_file = proptools.StringPtr(
			module.latestIncompatibilitiesFilegroupName(apiScope))

		if proptools.Bool(module.sdkLibraryProperties.Api_lint.Enabled) {
			// Enable api lint.
			props.Check_api.Api_lint.Enabled = proptools.BoolPtr(true)
			props.Check_api.Api_lint.New_since = latestApiFilegroupName

			// If it exists then pass a lint-baseline.txt through to droidstubs.
			baselinePath := path.Join(apiDir, apiScope.apiFilePrefix+"lint-baseline.txt")
			baselinePathRelativeToRoot := path.Join(mctx.ModuleDir(), baselinePath)
			paths, err := mctx.GlobWithDeps(baselinePathRelativeToRoot, nil)
			if err != nil {
				mctx.ModuleErrorf("error checking for presence of %s: %s", baselinePathRelativeToRoot, err)
			}
			if len(paths) == 1 {
				props.Check_api.Api_lint.Baseline_file = proptools.StringPtr(baselinePath)
			} else if len(paths) != 0 {
				mctx.ModuleErrorf("error checking for presence of %s: expected one path, found: %v", baselinePathRelativeToRoot, paths)
			}
		}
	}

	if !Bool(module.sdkLibraryProperties.No_dist) {
		// Dist the api txt and removed api txt artifacts for sdk builds.
		distDir := proptools.StringPtr(path.Join(module.apiDistPath(apiScope), "api"))
		for _, p := range []struct {
			tag     string
			pattern string
		}{
			{tag: ".api.txt", pattern: "%s.txt"},
			{tag: ".removed-api.txt", pattern: "%s-removed.txt"},
		} {
			props.Dists = append(props.Dists, android.Dist{
				Targets: []string{"sdk", "win_sdk"},
				Dir:     distDir,
				Dest:    proptools.StringPtr(fmt.Sprintf(p.pattern, module.distStem())),
				Tag:     proptools.StringPtr(p.tag),
			})
		}
	}

	mctx.CreateModule(DroidstubsFactory, &props)
}

// Implements android.ApexModule
func (module *SdkLibrary) DepIsInSameApex(mctx android.BaseModuleContext, dep android.Module) bool {
	depTag := mctx.OtherModuleDependencyTag(dep)
	if depTag == xmlPermissionsFileTag {
		return true
	}
	return module.Library.DepIsInSameApex(mctx, dep)
}

// Implements android.ApexModule
func (module *SdkLibrary) UniqueApexVariations() bool {
	return module.uniqueApexVariations()
}

// Creates the xml file that publicizes the runtime library
func (module *SdkLibrary) createXmlFile(mctx android.DefaultableHookContext) {
	moduleMinApiLevel := module.Library.MinSdkVersion(mctx).ApiLevel
	var moduleMinApiLevelStr = moduleMinApiLevel.String()
	if moduleMinApiLevel == android.NoneApiLevel {
		moduleMinApiLevelStr = "current"
	}
	props := struct {
		Name                      *string
		Lib_name                  *string
		Apex_available            []string
		On_bootclasspath_since    *string
		On_bootclasspath_before   *string
		Min_device_sdk            *string
		Max_device_sdk            *string
		Sdk_library_min_api_level *string
	}{
		Name:                      proptools.StringPtr(module.xmlPermissionsModuleName()),
		Lib_name:                  proptools.StringPtr(module.BaseModuleName()),
		Apex_available:            module.ApexProperties.Apex_available,
		On_bootclasspath_since:    module.commonSdkLibraryProperties.On_bootclasspath_since,
		On_bootclasspath_before:   module.commonSdkLibraryProperties.On_bootclasspath_before,
		Min_device_sdk:            module.commonSdkLibraryProperties.Min_device_sdk,
		Max_device_sdk:            module.commonSdkLibraryProperties.Max_device_sdk,
		Sdk_library_min_api_level: &moduleMinApiLevelStr,
	}

	mctx.CreateModule(sdkLibraryXmlFactory, &props)
}

func PrebuiltJars(ctx android.BaseModuleContext, baseName string, s android.SdkSpec) android.Paths {
	var ver android.ApiLevel
	var kind android.SdkKind
	if s.UsePrebuilt(ctx) {
		ver = s.ApiLevel
		kind = s.Kind
	} else {
		// We don't have prebuilt SDK for the specific sdkVersion.
		// Instead of breaking the build, fallback to use "system_current"
		ver = android.FutureApiLevel
		kind = android.SdkSystem
	}

	dir := filepath.Join("prebuilts", "sdk", ver.String(), kind.String())
	jar := filepath.Join(dir, baseName+".jar")
	jarPath := android.ExistentPathForSource(ctx, jar)
	if !jarPath.Valid() {
		if ctx.Config().AllowMissingDependencies() {
			return android.Paths{android.PathForSource(ctx, jar)}
		} else {
			ctx.PropertyErrorf("sdk_library", "invalid sdk version %q, %q does not exist", s.Raw, jar)
		}
		return nil
	}
	return android.Paths{jarPath.Path()}
}

// Check to see if the other module is within the same set of named APEXes as this module.
//
// If either this or the other module are on the platform then this will return
// false.
func withinSameApexesAs(ctx android.BaseModuleContext, other android.Module) bool {
	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	otherApexInfo := ctx.OtherModuleProvider(other, android.ApexInfoProvider).(android.ApexInfo)
	return len(otherApexInfo.InApexVariants) > 0 && reflect.DeepEqual(apexInfo.InApexVariants, otherApexInfo.InApexVariants)
}

func (module *SdkLibrary) sdkJars(ctx android.BaseModuleContext, sdkVersion android.SdkSpec, headerJars bool) android.Paths {
	// If the client doesn't set sdk_version, but if this library prefers stubs over
	// the impl library, let's provide the widest API surface possible. To do so,
	// force override sdk_version to module_current so that the closest possible API
	// surface could be found in selectHeaderJarsForSdkVersion
	if module.defaultsToStubs() && !sdkVersion.Specified() {
		sdkVersion = android.SdkSpecFrom(ctx, "module_current")
	}

	// Only provide access to the implementation library if it is actually built.
	if module.requiresRuntimeImplementationLibrary() {
		// Check any special cases for java_sdk_library.
		//
		// Only allow access to the implementation library in the following condition:
		// * No sdk_version specified on the referencing module.
		// * The referencing module is in the same apex as this.
		if sdkVersion.Kind == android.SdkPrivate || withinSameApexesAs(ctx, module) {
			if headerJars {
				return module.HeaderJars()
			} else {
				return module.ImplementationJars()
			}
		}
	}

	return module.selectHeaderJarsForSdkVersion(ctx, sdkVersion)
}

// to satisfy SdkLibraryDependency interface
func (module *SdkLibrary) SdkHeaderJars(ctx android.BaseModuleContext, sdkVersion android.SdkSpec) android.Paths {
	return module.sdkJars(ctx, sdkVersion, true /*headerJars*/)
}

// to satisfy SdkLibraryDependency interface
func (module *SdkLibrary) SdkImplementationJars(ctx android.BaseModuleContext, sdkVersion android.SdkSpec) android.Paths {
	return module.sdkJars(ctx, sdkVersion, false /*headerJars*/)
}

var javaSdkLibrariesKey = android.NewOnceKey("javaSdkLibraries")

func javaSdkLibraries(config android.Config) *[]string {
	return config.Once(javaSdkLibrariesKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func (module *SdkLibrary) getApiDir() string {
	return proptools.StringDefault(module.sdkLibraryProperties.Api_dir, "api")
}

// For a java_sdk_library module, create internal modules for stubs, docs,
// runtime libs and xml file. If requested, the stubs and docs are created twice
// once for public API level and once for system API level
func (module *SdkLibrary) CreateInternalModules(mctx android.DefaultableHookContext) {
	// If the module has been disabled then don't create any child modules.
	if !module.Enabled() {
		return
	}

	if len(module.properties.Srcs) == 0 {
		mctx.PropertyErrorf("srcs", "java_sdk_library must specify srcs")
		return
	}

	// If this builds against standard libraries (i.e. is not part of the core libraries)
	// then assume it provides both system and test apis.
	sdkDep := decodeSdkDep(mctx, android.SdkContext(&module.Library))
	hasSystemAndTestApis := sdkDep.hasStandardLibs()
	module.sdkLibraryProperties.Generate_system_and_test_apis = hasSystemAndTestApis

	missingCurrentApi := false

	generatedScopes := module.getGeneratedApiScopes(mctx)

	apiDir := module.getApiDir()
	for _, scope := range generatedScopes {
		for _, api := range []string{"current.txt", "removed.txt"} {
			path := path.Join(mctx.ModuleDir(), apiDir, scope.apiFilePrefix+api)
			p := android.ExistentPathForSource(mctx, path)
			if !p.Valid() {
				if mctx.Config().AllowMissingDependencies() {
					mctx.AddMissingDependencies([]string{path})
				} else {
					mctx.ModuleErrorf("Current api file %#v doesn't exist", path)
					missingCurrentApi = true
				}
			}
		}
	}

	if missingCurrentApi {
		script := "build/soong/scripts/gen-java-current-api-files.sh"
		p := android.ExistentPathForSource(mctx, script)

		if !p.Valid() {
			panic(fmt.Sprintf("script file %s doesn't exist", script))
		}

		mctx.ModuleErrorf("One or more current api files are missing. "+
			"You can update them by:\n"+
			"%s %q %s && m update-api",
			script, filepath.Join(mctx.ModuleDir(), apiDir),
			strings.Join(generatedScopes.Strings(func(s *apiScope) string { return s.apiFilePrefix }), " "))
		return
	}

	for _, scope := range generatedScopes {
		// Use the stubs source name for legacy reasons.
		module.createStubsSourcesAndApi(mctx, scope, module.stubsSourceModuleName(scope), scope.droidstubsArgs)

		module.createStubsLibrary(mctx, scope)
	}

	if module.requiresRuntimeImplementationLibrary() {
		// Create child module to create an implementation library.
		//
		// This temporarily creates a second implementation library that can be explicitly
		// referenced.
		//
		// TODO(b/156618935) - update comment once only one implementation library is created.
		module.createImplLibrary(mctx)

		// Only create an XML permissions file that declares the library as being usable
		// as a shared library if required.
		if module.sharedLibrary() {
			module.createXmlFile(mctx)
		}

		// record java_sdk_library modules so that they are exported to make
		javaSdkLibraries := javaSdkLibraries(mctx.Config())
		javaSdkLibrariesLock.Lock()
		defer javaSdkLibrariesLock.Unlock()
		*javaSdkLibraries = append(*javaSdkLibraries, module.BaseModuleName())
	}

	// Add the impl_only_libs and impl_only_static_libs *after* we're done using them in submodules.
	module.properties.Libs = append(module.properties.Libs, module.sdkLibraryProperties.Impl_only_libs...)
	module.properties.Static_libs = append(module.properties.Static_libs, module.sdkLibraryProperties.Impl_only_static_libs...)
}

func (module *SdkLibrary) InitSdkLibraryProperties() {
	module.addHostAndDeviceProperties()
	module.AddProperties(&module.sdkLibraryProperties)

	module.initSdkLibraryComponent(module)

	module.properties.Installable = proptools.BoolPtr(true)
	module.deviceProperties.IsSDKLibrary = true
}

func (module *SdkLibrary) requiresRuntimeImplementationLibrary() bool {
	return !proptools.Bool(module.sdkLibraryProperties.Api_only)
}

func (module *SdkLibrary) defaultsToStubs() bool {
	return proptools.Bool(module.sdkLibraryProperties.Default_to_stubs)
}

// Defines how to name the individual component modules the sdk library creates.
type sdkLibraryComponentNamingScheme interface {
	stubsLibraryModuleName(scope *apiScope, baseName string) string

	stubsSourceModuleName(scope *apiScope, baseName string) string
}

type defaultNamingScheme struct {
}

func (s *defaultNamingScheme) stubsLibraryModuleName(scope *apiScope, baseName string) string {
	return scope.stubsLibraryModuleName(baseName)
}

func (s *defaultNamingScheme) stubsSourceModuleName(scope *apiScope, baseName string) string {
	return scope.stubsSourceModuleName(baseName)
}

var _ sdkLibraryComponentNamingScheme = (*defaultNamingScheme)(nil)

func moduleStubLinkType(name string) (stub bool, ret sdkLinkType) {
	// This suffix-based approach is fragile and could potentially mis-trigger.
	// TODO(b/155164730): Clean this up when modules no longer reference sdk_lib stubs directly.
	if strings.HasSuffix(name, apiScopePublic.stubsLibraryModuleNameSuffix()) {
		if name == "hwbinder.stubs" || name == "libcore_private.stubs" {
			// Due to a previous bug, these modules were not considered stubs, so we retain that.
			return false, javaPlatform
		}
		return true, javaSdk
	}
	if strings.HasSuffix(name, apiScopeSystem.stubsLibraryModuleNameSuffix()) {
		return true, javaSystem
	}
	if strings.HasSuffix(name, apiScopeModuleLib.stubsLibraryModuleNameSuffix()) {
		return true, javaModule
	}
	if strings.HasSuffix(name, apiScopeTest.stubsLibraryModuleNameSuffix()) {
		return true, javaSystem
	}
	return false, javaPlatform
}

// java_sdk_library is a special Java library that provides optional platform APIs to apps.
// In practice, it can be viewed as a combination of several modules: 1) stubs library that clients
// are linked against to, 2) droiddoc module that internally generates API stubs source files,
// 3) the real runtime shared library that implements the APIs, and 4) XML file for adding
// the runtime lib to the classpath at runtime if requested via <uses-library>.
func SdkLibraryFactory() android.Module {
	module := &SdkLibrary{}

	// Initialize information common between source and prebuilt.
	module.initCommon(module)

	module.InitSdkLibraryProperties()
	android.InitApexModule(module)
	android.InitSdkAwareModule(module)
	InitJavaModule(module, android.HostAndDeviceSupported)

	// Initialize the map from scope to scope specific properties.
	scopeToProperties := make(map[*apiScope]*ApiScopeProperties)
	for _, scope := range allApiScopes {
		scopeToProperties[scope] = scope.scopeSpecificProperties(module)
	}
	module.scopeToProperties = scopeToProperties

	// Add the properties containing visibility rules so that they are checked.
	android.AddVisibilityProperty(module, "impl_library_visibility", &module.sdkLibraryProperties.Impl_library_visibility)
	android.AddVisibilityProperty(module, "stubs_library_visibility", &module.sdkLibraryProperties.Stubs_library_visibility)
	android.AddVisibilityProperty(module, "stubs_source_visibility", &module.sdkLibraryProperties.Stubs_source_visibility)

	module.SetDefaultableHook(func(ctx android.DefaultableHookContext) {
		// If no implementation is required then it cannot be used as a shared library
		// either.
		if !module.requiresRuntimeImplementationLibrary() {
			// If shared_library has been explicitly set to true then it is incompatible
			// with api_only: true.
			if proptools.Bool(module.commonSdkLibraryProperties.Shared_library) {
				ctx.PropertyErrorf("api_only/shared_library", "inconsistent settings, shared_library and api_only cannot both be true")
			}
			// Set shared_library: false.
			module.commonSdkLibraryProperties.Shared_library = proptools.BoolPtr(false)
		}

		if module.initCommonAfterDefaultsApplied(ctx) {
			module.CreateInternalModules(ctx)
		}
	})
	return module
}

//
// SDK library prebuilts
//

// Properties associated with each api scope.
type sdkLibraryScopeProperties struct {
	Jars []string `android:"path"`

	Sdk_version *string

	// List of shared java libs that this module has dependencies to
	Libs []string

	// The stubs source.
	Stub_srcs []string `android:"path"`

	// The current.txt
	Current_api *string `android:"path"`

	// The removed.txt
	Removed_api *string `android:"path"`

	// Annotation zip
	Annotations *string `android:"path"`
}

type sdkLibraryImportProperties struct {
	// List of shared java libs, common to all scopes, that this module has
	// dependencies to
	Libs []string

	// If set to true, compile dex files for the stubs. Defaults to false.
	Compile_dex *bool

	// If not empty, classes are restricted to the specified packages and their sub-packages.
	Permitted_packages []string
}

type SdkLibraryImport struct {
	android.ModuleBase
	android.DefaultableModuleBase
	prebuilt android.Prebuilt
	android.ApexModuleBase
	android.SdkBase

	hiddenAPI
	dexpreopter

	properties sdkLibraryImportProperties

	// Map from api scope to the scope specific property structure.
	scopeProperties map[*apiScope]*sdkLibraryScopeProperties

	commonToSdkLibraryAndImport

	// The reference to the implementation library created by the source module.
	// Is nil if the source module does not exist.
	implLibraryModule *Library

	// The reference to the xml permissions module created by the source module.
	// Is nil if the source module does not exist.
	xmlPermissionsFileModule *sdkLibraryXml

	// Build path to the dex implementation jar obtained from the prebuilt_apex, if any.
	dexJarFile OptionalDexJarPath

	// Expected install file path of the source module(sdk_library)
	// or dex implementation jar obtained from the prebuilt_apex, if any.
	installFile android.Path
}

var _ SdkLibraryDependency = (*SdkLibraryImport)(nil)

// The type of a structure that contains a field of type sdkLibraryScopeProperties
// for each apiscope in allApiScopes, e.g. something like:
// struct {
//   Public sdkLibraryScopeProperties
//   System sdkLibraryScopeProperties
//   ...
// }
var allScopeStructType = createAllScopePropertiesStructType()

// Dynamically create a structure type for each apiscope in allApiScopes.
func createAllScopePropertiesStructType() reflect.Type {
	var fields []reflect.StructField
	for _, apiScope := range allApiScopes {
		field := reflect.StructField{
			Name: apiScope.fieldName,
			Type: reflect.TypeOf(sdkLibraryScopeProperties{}),
		}
		fields = append(fields, field)
	}

	return reflect.StructOf(fields)
}

// Create an instance of the scope specific structure type and return a map
// from apiscope to a pointer to each scope specific field.
func createPropertiesInstance() (interface{}, map[*apiScope]*sdkLibraryScopeProperties) {
	allScopePropertiesPtr := reflect.New(allScopeStructType)
	allScopePropertiesStruct := allScopePropertiesPtr.Elem()
	scopeProperties := make(map[*apiScope]*sdkLibraryScopeProperties)

	for _, apiScope := range allApiScopes {
		field := allScopePropertiesStruct.FieldByName(apiScope.fieldName)
		scopeProperties[apiScope] = field.Addr().Interface().(*sdkLibraryScopeProperties)
	}

	return allScopePropertiesPtr.Interface(), scopeProperties
}

// java_sdk_library_import imports a prebuilt java_sdk_library.
func sdkLibraryImportFactory() android.Module {
	module := &SdkLibraryImport{}

	allScopeProperties, scopeToProperties := createPropertiesInstance()
	module.scopeProperties = scopeToProperties
	module.AddProperties(&module.properties, allScopeProperties)

	// Initialize information common between source and prebuilt.
	module.initCommon(module)

	android.InitPrebuiltModule(module, &[]string{""})
	android.InitApexModule(module)
	android.InitSdkAwareModule(module)
	InitJavaModule(module, android.HostAndDeviceSupported)

	module.SetDefaultableHook(func(mctx android.DefaultableHookContext) {
		if module.initCommonAfterDefaultsApplied(mctx) {
			module.createInternalModules(mctx)
		}
	})
	return module
}

var _ PermittedPackagesForUpdatableBootJars = (*SdkLibraryImport)(nil)

func (module *SdkLibraryImport) PermittedPackagesForUpdatableBootJars() []string {
	return module.properties.Permitted_packages
}

func (module *SdkLibraryImport) Prebuilt() *android.Prebuilt {
	return &module.prebuilt
}

func (module *SdkLibraryImport) Name() string {
	return module.prebuilt.Name(module.ModuleBase.Name())
}

func (module *SdkLibraryImport) createInternalModules(mctx android.DefaultableHookContext) {

	// If the build is configured to use prebuilts then force this to be preferred.
	if mctx.Config().AlwaysUsePrebuiltSdks() {
		module.prebuilt.ForcePrefer()
	}

	for apiScope, scopeProperties := range module.scopeProperties {
		if len(scopeProperties.Jars) == 0 {
			continue
		}

		module.createJavaImportForStubs(mctx, apiScope, scopeProperties)

		if len(scopeProperties.Stub_srcs) > 0 {
			module.createPrebuiltStubsSources(mctx, apiScope, scopeProperties)
		}
	}

	javaSdkLibraries := javaSdkLibraries(mctx.Config())
	javaSdkLibrariesLock.Lock()
	defer javaSdkLibrariesLock.Unlock()
	*javaSdkLibraries = append(*javaSdkLibraries, module.BaseModuleName())
}

func (module *SdkLibraryImport) createJavaImportForStubs(mctx android.DefaultableHookContext, apiScope *apiScope, scopeProperties *sdkLibraryScopeProperties) {
	// Creates a java import for the jar with ".stubs" suffix
	props := struct {
		Name        *string
		Sdk_version *string
		Libs        []string
		Jars        []string
		Compile_dex *bool

		android.UserSuppliedPrebuiltProperties
	}{}
	props.Name = proptools.StringPtr(module.stubsLibraryModuleName(apiScope))
	props.Sdk_version = scopeProperties.Sdk_version
	// Prepend any of the libs from the legacy public properties to the libs for each of the
	// scopes to avoid having to duplicate them in each scope.
	props.Libs = append(module.properties.Libs, scopeProperties.Libs...)
	props.Jars = scopeProperties.Jars

	// The imports are preferred if the java_sdk_library_import is preferred.
	props.CopyUserSuppliedPropertiesFromPrebuilt(&module.prebuilt)

	// The imports need to be compiled to dex if the java_sdk_library_import requests it.
	compileDex := module.properties.Compile_dex
	if module.stubLibrariesCompiledForDex() {
		compileDex = proptools.BoolPtr(true)
	}
	props.Compile_dex = compileDex

	mctx.CreateModule(ImportFactory, &props, module.sdkComponentPropertiesForChildLibrary())
}

func (module *SdkLibraryImport) createPrebuiltStubsSources(mctx android.DefaultableHookContext, apiScope *apiScope, scopeProperties *sdkLibraryScopeProperties) {
	props := struct {
		Name *string
		Srcs []string

		android.UserSuppliedPrebuiltProperties
	}{}
	props.Name = proptools.StringPtr(module.stubsSourceModuleName(apiScope))
	props.Srcs = scopeProperties.Stub_srcs

	// The stubs source is preferred if the java_sdk_library_import is preferred.
	props.CopyUserSuppliedPropertiesFromPrebuilt(&module.prebuilt)

	mctx.CreateModule(PrebuiltStubsSourcesFactory, &props)
}

// Add the dependencies on the child module in the component deps mutator so that it
// creates references to the prebuilt and not the source modules.
func (module *SdkLibraryImport) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	for apiScope, scopeProperties := range module.scopeProperties {
		if len(scopeProperties.Jars) == 0 {
			continue
		}

		// Add dependencies to the prebuilt stubs library
		ctx.AddVariationDependencies(nil, apiScope.stubsTag, android.PrebuiltNameFromSource(module.stubsLibraryModuleName(apiScope)))

		if len(scopeProperties.Stub_srcs) > 0 {
			// Add dependencies to the prebuilt stubs source library
			ctx.AddVariationDependencies(nil, apiScope.stubsSourceTag, android.PrebuiltNameFromSource(module.stubsSourceModuleName(apiScope)))
		}
	}
}

// Add other dependencies as normal.
func (module *SdkLibraryImport) DepsMutator(ctx android.BottomUpMutatorContext) {

	implName := module.implLibraryModuleName()
	if ctx.OtherModuleExists(implName) {
		ctx.AddVariationDependencies(nil, implLibraryTag, implName)

		xmlPermissionsModuleName := module.xmlPermissionsModuleName()
		if module.sharedLibrary() && ctx.OtherModuleExists(xmlPermissionsModuleName) {
			// Add dependency to the rule for generating the xml permissions file
			ctx.AddDependency(module, xmlPermissionsFileTag, xmlPermissionsModuleName)
		}
	}
}

func (module *SdkLibraryImport) AndroidMkEntries() []android.AndroidMkEntries {
	// For an SDK library imported from a prebuilt APEX, we don't need a Make module for itself, as we
	// don't need to install it. However, we need to add its dexpreopt outputs as sub-modules, if it
	// is preopted.
	dexpreoptEntries := module.dexpreopter.AndroidMkEntriesForApex()
	return append(dexpreoptEntries, android.AndroidMkEntries{Disabled: true})
}

var _ android.ApexModule = (*SdkLibraryImport)(nil)

// Implements android.ApexModule
func (module *SdkLibraryImport) DepIsInSameApex(mctx android.BaseModuleContext, dep android.Module) bool {
	depTag := mctx.OtherModuleDependencyTag(dep)
	if depTag == xmlPermissionsFileTag {
		return true
	}

	// None of the other dependencies of the java_sdk_library_import are in the same apex
	// as the one that references this module.
	return false
}

// Implements android.ApexModule
func (module *SdkLibraryImport) ShouldSupportSdkVersion(ctx android.BaseModuleContext,
	sdkVersion android.ApiLevel) error {
	// we don't check prebuilt modules for sdk_version
	return nil
}

// Implements android.ApexModule
func (module *SdkLibraryImport) UniqueApexVariations() bool {
	return module.uniqueApexVariations()
}

// MinSdkVersion - Implements hiddenAPIModule
func (module *SdkLibraryImport) MinSdkVersion(ctx android.EarlyModuleContext) android.SdkSpec {
	return android.SdkSpecNone
}

var _ hiddenAPIModule = (*SdkLibraryImport)(nil)

func (module *SdkLibraryImport) OutputFiles(tag string) (android.Paths, error) {
	paths, err := module.commonOutputFiles(tag)
	if paths != nil || err != nil {
		return paths, err
	}
	if module.implLibraryModule != nil {
		return module.implLibraryModule.OutputFiles(tag)
	} else {
		return nil, nil
	}
}

func (module *SdkLibraryImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	module.generateCommonBuildActions(ctx)

	// Assume that source module(sdk_library) is installed in /<sdk_library partition>/framework
	module.installFile = android.PathForModuleInstall(ctx, "framework", module.Stem()+".jar")

	// Record the paths to the prebuilt stubs library and stubs source.
	ctx.VisitDirectDeps(func(to android.Module) {
		tag := ctx.OtherModuleDependencyTag(to)

		// Extract information from any of the scope specific dependencies.
		if scopeTag, ok := tag.(scopeDependencyTag); ok {
			apiScope := scopeTag.apiScope
			scopePaths := module.getScopePathsCreateIfNeeded(apiScope)

			// Extract information from the dependency. The exact information extracted
			// is determined by the nature of the dependency which is determined by the tag.
			scopeTag.extractDepInfo(ctx, to, scopePaths)
		} else if tag == implLibraryTag {
			if implLibrary, ok := to.(*Library); ok {
				module.implLibraryModule = implLibrary
			} else {
				ctx.ModuleErrorf("implementation library must be of type *java.Library but was %T", to)
			}
		} else if tag == xmlPermissionsFileTag {
			if xmlPermissionsFileModule, ok := to.(*sdkLibraryXml); ok {
				module.xmlPermissionsFileModule = xmlPermissionsFileModule
			} else {
				ctx.ModuleErrorf("xml permissions file module must be of type *sdkLibraryXml but was %T", to)
			}
		}
	})

	// Populate the scope paths with information from the properties.
	for apiScope, scopeProperties := range module.scopeProperties {
		if len(scopeProperties.Jars) == 0 {
			continue
		}

		paths := module.getScopePathsCreateIfNeeded(apiScope)
		paths.annotationsZip = android.OptionalPathForModuleSrc(ctx, scopeProperties.Annotations)
		paths.currentApiFilePath = android.OptionalPathForModuleSrc(ctx, scopeProperties.Current_api)
		paths.removedApiFilePath = android.OptionalPathForModuleSrc(ctx, scopeProperties.Removed_api)
	}

	if ctx.Device() {
		// If this is a variant created for a prebuilt_apex then use the dex implementation jar
		// obtained from the associated deapexer module.
		ai := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
		if ai.ForPrebuiltApex {
			// Get the path of the dex implementation jar from the `deapexer` module.
			di := android.FindDeapexerProviderForModule(ctx)
			if di == nil {
				return // An error has been reported by FindDeapexerProviderForModule.
			}
			if dexOutputPath := di.PrebuiltExportPath(apexRootRelativePathToJavaLib(module.BaseModuleName())); dexOutputPath != nil {
				dexJarFile := makeDexJarPathFromPath(dexOutputPath)
				module.dexJarFile = dexJarFile
				installPath := android.PathForModuleInPartitionInstall(
					ctx, "apex", ai.ApexVariationName, apexRootRelativePathToJavaLib(module.BaseModuleName()))
				module.installFile = installPath
				module.initHiddenAPI(ctx, dexJarFile, module.findScopePaths(apiScopePublic).stubsImplPath[0], nil)

				// Dexpreopting.
				module.dexpreopter.installPath = module.dexpreopter.getInstallPath(ctx, installPath)
				module.dexpreopter.isSDKLibrary = true
				module.dexpreopter.uncompressedDex = shouldUncompressDex(ctx, &module.dexpreopter)
				module.dexpreopt(ctx, dexOutputPath)
			} else {
				// This should never happen as a variant for a prebuilt_apex is only created if the
				// prebuilt_apex has been configured to export the java library dex file.
				ctx.ModuleErrorf("internal error: no dex implementation jar available from prebuilt APEX %s", di.ApexModuleName())
			}
		}
	}
}

func (module *SdkLibraryImport) sdkJars(ctx android.BaseModuleContext, sdkVersion android.SdkSpec, headerJars bool) android.Paths {

	// For consistency with SdkLibrary make the implementation jar available to libraries that
	// are within the same APEX.
	implLibraryModule := module.implLibraryModule
	if implLibraryModule != nil && withinSameApexesAs(ctx, module) {
		if headerJars {
			return implLibraryModule.HeaderJars()
		} else {
			return implLibraryModule.ImplementationJars()
		}
	}

	return module.selectHeaderJarsForSdkVersion(ctx, sdkVersion)
}

// to satisfy SdkLibraryDependency interface
func (module *SdkLibraryImport) SdkHeaderJars(ctx android.BaseModuleContext, sdkVersion android.SdkSpec) android.Paths {
	// This module is just a wrapper for the prebuilt stubs.
	return module.sdkJars(ctx, sdkVersion, true)
}

// to satisfy SdkLibraryDependency interface
func (module *SdkLibraryImport) SdkImplementationJars(ctx android.BaseModuleContext, sdkVersion android.SdkSpec) android.Paths {
	// This module is just a wrapper for the stubs.
	return module.sdkJars(ctx, sdkVersion, false)
}

// to satisfy UsesLibraryDependency interface
func (module *SdkLibraryImport) DexJarBuildPath() OptionalDexJarPath {
	// The dex implementation jar extracted from the .apex file should be used in preference to the
	// source.
	if module.dexJarFile.IsSet() {
		return module.dexJarFile
	}
	if module.implLibraryModule == nil {
		return makeUnsetDexJarPath()
	} else {
		return module.implLibraryModule.DexJarBuildPath()
	}
}

// to satisfy UsesLibraryDependency interface
func (module *SdkLibraryImport) DexJarInstallPath() android.Path {
	return module.installFile
}

// to satisfy UsesLibraryDependency interface
func (module *SdkLibraryImport) ClassLoaderContexts() dexpreopt.ClassLoaderContextMap {
	return nil
}

// to satisfy apex.javaDependency interface
func (module *SdkLibraryImport) JacocoReportClassesFile() android.Path {
	if module.implLibraryModule == nil {
		return nil
	} else {
		return module.implLibraryModule.JacocoReportClassesFile()
	}
}

// to satisfy apex.javaDependency interface
func (module *SdkLibraryImport) LintDepSets() LintDepSets {
	if module.implLibraryModule == nil {
		return LintDepSets{}
	} else {
		return module.implLibraryModule.LintDepSets()
	}
}

func (module *SdkLibraryImport) GetStrictUpdatabilityLinting() bool {
	if module.implLibraryModule == nil {
		return false
	} else {
		return module.implLibraryModule.GetStrictUpdatabilityLinting()
	}
}

func (module *SdkLibraryImport) SetStrictUpdatabilityLinting(strictLinting bool) {
	if module.implLibraryModule != nil {
		module.implLibraryModule.SetStrictUpdatabilityLinting(strictLinting)
	}
}

// to satisfy apex.javaDependency interface
func (module *SdkLibraryImport) Stem() string {
	return module.BaseModuleName()
}

var _ ApexDependency = (*SdkLibraryImport)(nil)

// to satisfy java.ApexDependency interface
func (module *SdkLibraryImport) HeaderJars() android.Paths {
	if module.implLibraryModule == nil {
		return nil
	} else {
		return module.implLibraryModule.HeaderJars()
	}
}

// to satisfy java.ApexDependency interface
func (module *SdkLibraryImport) ImplementationAndResourcesJars() android.Paths {
	if module.implLibraryModule == nil {
		return nil
	} else {
		return module.implLibraryModule.ImplementationAndResourcesJars()
	}
}

// to satisfy java.DexpreopterInterface interface
func (module *SdkLibraryImport) IsInstallable() bool {
	return true
}

var _ android.RequiredFilesFromPrebuiltApex = (*SdkLibraryImport)(nil)

func (module *SdkLibraryImport) RequiredFilesFromPrebuiltApex(ctx android.BaseModuleContext) []string {
	name := module.BaseModuleName()
	return requiredFilesFromPrebuiltApexForImport(name)
}

//
// java_sdk_library_xml
//
type sdkLibraryXml struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase

	properties sdkLibraryXmlProperties

	outputFilePath android.OutputPath
	installDirPath android.InstallPath

	hideApexVariantFromMake bool
}

type sdkLibraryXmlProperties struct {
	// canonical name of the lib
	Lib_name *string

	// Signals that this shared library is part of the bootclasspath starting
	// on the version indicated in this attribute.
	//
	// This will make platforms at this level and above to ignore
	// <uses-library> tags with this library name because the library is already
	// available
	On_bootclasspath_since *string

	// Signals that this shared library was part of the bootclasspath before
	// (but not including) the version indicated in this attribute.
	//
	// The system will automatically add a <uses-library> tag with this library to
	// apps that target any SDK less than the version indicated in this attribute.
	On_bootclasspath_before *string

	// Indicates that PackageManager should ignore this shared library if the
	// platform is below the version indicated in this attribute.
	//
	// This means that the device won't recognise this library as installed.
	Min_device_sdk *string

	// Indicates that PackageManager should ignore this shared library if the
	// platform is above the version indicated in this attribute.
	//
	// This means that the device won't recognise this library as installed.
	Max_device_sdk *string

	// The SdkLibrary's min api level as a string
	//
	// This value comes from the ApiLevel of the MinSdkVersion property.
	Sdk_library_min_api_level *string
}

// java_sdk_library_xml builds the permission xml file for a java_sdk_library.
// Not to be used directly by users. java_sdk_library internally uses this.
func sdkLibraryXmlFactory() android.Module {
	module := &sdkLibraryXml{}

	module.AddProperties(&module.properties)

	android.InitApexModule(module)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)

	return module
}

func (module *sdkLibraryXml) UniqueApexVariations() bool {
	// sdkLibraryXml needs a unique variation per APEX because the generated XML file contains the path to the
	// mounted APEX, which contains the name of the APEX.
	return true
}

// from android.PrebuiltEtcModule
func (module *sdkLibraryXml) BaseDir() string {
	return "etc"
}

// from android.PrebuiltEtcModule
func (module *sdkLibraryXml) SubDir() string {
	return "permissions"
}

// from android.PrebuiltEtcModule
func (module *sdkLibraryXml) OutputFile() android.OutputPath {
	return module.outputFilePath
}

// from android.ApexModule
func (module *sdkLibraryXml) AvailableFor(what string) bool {
	return true
}

func (module *sdkLibraryXml) DepsMutator(ctx android.BottomUpMutatorContext) {
	// do nothing
}

var _ android.ApexModule = (*sdkLibraryXml)(nil)

// Implements android.ApexModule
func (module *sdkLibraryXml) ShouldSupportSdkVersion(ctx android.BaseModuleContext,
	sdkVersion android.ApiLevel) error {
	// sdkLibraryXml doesn't need to be checked separately because java_sdk_library is checked
	return nil
}

// File path to the runtime implementation library
func (module *sdkLibraryXml) implPath(ctx android.ModuleContext) string {
	implName := proptools.String(module.properties.Lib_name)
	if apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo); !apexInfo.IsForPlatform() {
		// TODO(b/146468504): ApexVariationName() is only a soong module name, not apex name.
		// In most cases, this works fine. But when apex_name is set or override_apex is used
		// this can be wrong.
		return fmt.Sprintf("/apex/%s/javalib/%s.jar", apexInfo.ApexVariationName, implName)
	}
	partition := "system"
	if module.SocSpecific() {
		partition = "vendor"
	} else if module.DeviceSpecific() {
		partition = "odm"
	} else if module.ProductSpecific() {
		partition = "product"
	} else if module.SystemExtSpecific() {
		partition = "system_ext"
	}
	return "/" + partition + "/framework/" + implName + ".jar"
}

func formattedOptionalSdkLevelAttribute(ctx android.ModuleContext, attrName string, value *string) string {
	if value == nil {
		return ""
	}
	apiLevel, err := android.ApiLevelFromUser(ctx, *value)
	if err != nil {
		// attributes in bp files have underscores but in the xml have dashes.
		ctx.PropertyErrorf(strings.ReplaceAll(attrName, "-", "_"), err.Error())
		return ""
	}
	if apiLevel.IsCurrent() {
		// passing "current" would always mean a future release, never the current (or the current in
		// progress) which means some conditions would never be triggered.
		ctx.PropertyErrorf(strings.ReplaceAll(attrName, "-", "_"),
			`"current" is not an allowed value for this attribute`)
		return ""
	}
	// "safeValue" is safe because it translates finalized codenames to a string
	// with their SDK int.
	safeValue := apiLevel.String()
	return formattedOptionalAttribute(attrName, &safeValue)
}

// formats an attribute for the xml permissions file if the value is not null
// returns empty string otherwise
func formattedOptionalAttribute(attrName string, value *string) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf(`        %s=\"%s\"\n`, attrName, *value)
}

func (module *sdkLibraryXml) permissionsContents(ctx android.ModuleContext) string {
	libName := proptools.String(module.properties.Lib_name)
	libNameAttr := formattedOptionalAttribute("name", &libName)
	filePath := module.implPath(ctx)
	filePathAttr := formattedOptionalAttribute("file", &filePath)
	implicitFromAttr := formattedOptionalSdkLevelAttribute(ctx, "on-bootclasspath-since", module.properties.On_bootclasspath_since)
	implicitUntilAttr := formattedOptionalSdkLevelAttribute(ctx, "on-bootclasspath-before", module.properties.On_bootclasspath_before)
	minSdkAttr := formattedOptionalSdkLevelAttribute(ctx, "min-device-sdk", module.properties.Min_device_sdk)
	maxSdkAttr := formattedOptionalSdkLevelAttribute(ctx, "max-device-sdk", module.properties.Max_device_sdk)
	// <library> is understood in all android versions whereas <apex-library> is only understood from API T (and ignored before that).
	// similarly, min_device_sdk is only understood from T. So if a library is using that, we need to use the apex-library to make sure this library is not loaded before T
	var libraryTag string
	if module.properties.Min_device_sdk != nil {
		libraryTag = `    <apex-library\n`
	} else {
		libraryTag = `    <library\n`
	}

	return strings.Join([]string{
		`<?xml version=\"1.0\" encoding=\"utf-8\"?>\n`,
		`<!-- Copyright (C) 2018 The Android Open Source Project\n`,
		`\n`,
		`    Licensed under the Apache License, Version 2.0 (the \"License\");\n`,
		`    you may not use this file except in compliance with the License.\n`,
		`    You may obtain a copy of the License at\n`,
		`\n`,
		`        http://www.apache.org/licenses/LICENSE-2.0\n`,
		`\n`,
		`    Unless required by applicable law or agreed to in writing, software\n`,
		`    distributed under the License is distributed on an \"AS IS\" BASIS,\n`,
		`    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.\n`,
		`    See the License for the specific language governing permissions and\n`,
		`    limitations under the License.\n`,
		`-->\n`,
		`<permissions>\n`,
		libraryTag,
		libNameAttr,
		filePathAttr,
		implicitFromAttr,
		implicitUntilAttr,
		minSdkAttr,
		maxSdkAttr,
		`    />\n`,
		`</permissions>\n`}, "")
}

func (module *sdkLibraryXml) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	module.hideApexVariantFromMake = !ctx.Provider(android.ApexInfoProvider).(android.ApexInfo).IsForPlatform()

	libName := proptools.String(module.properties.Lib_name)
	module.selfValidate(ctx)
	xmlContent := module.permissionsContents(ctx)

	module.outputFilePath = android.PathForModuleOut(ctx, libName+".xml").OutputPath
	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().
		Text("/bin/bash -c \"echo -e '" + xmlContent + "'\" > ").
		Output(module.outputFilePath)

	rule.Build("java_sdk_xml", "Permission XML")

	module.installDirPath = android.PathForModuleInstall(ctx, "etc", module.SubDir())
}

func (module *sdkLibraryXml) AndroidMkEntries() []android.AndroidMkEntries {
	if module.hideApexVariantFromMake {
		return []android.AndroidMkEntries{{
			Disabled: true,
		}}
	}

	return []android.AndroidMkEntries{{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(module.outputFilePath),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_TAGS", "optional")
				entries.SetString("LOCAL_MODULE_PATH", module.installDirPath.String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", module.outputFilePath.Base())
			},
		},
	}}
}

func (module *sdkLibraryXml) selfValidate(ctx android.ModuleContext) {
	module.validateAtLeastTAttributes(ctx)
	module.validateMinAndMaxDeviceSdk(ctx)
	module.validateMinMaxDeviceSdkAndModuleMinSdk(ctx)
	module.validateOnBootclasspathBeforeRequirements(ctx)
}

func (module *sdkLibraryXml) validateAtLeastTAttributes(ctx android.ModuleContext) {
	t := android.ApiLevelOrPanic(ctx, "Tiramisu")
	module.attrAtLeastT(ctx, t, module.properties.Min_device_sdk, "min_device_sdk")
	module.attrAtLeastT(ctx, t, module.properties.Max_device_sdk, "max_device_sdk")
	module.attrAtLeastT(ctx, t, module.properties.On_bootclasspath_before, "on_bootclasspath_before")
	module.attrAtLeastT(ctx, t, module.properties.On_bootclasspath_since, "on_bootclasspath_since")
}

func (module *sdkLibraryXml) attrAtLeastT(ctx android.ModuleContext, t android.ApiLevel, attr *string, attrName string) {
	if attr != nil {
		if level, err := android.ApiLevelFromUser(ctx, *attr); err == nil {
			// we will inform the user of invalid inputs when we try to write the
			// permissions xml file so we don't need to do it here
			if t.GreaterThan(level) {
				ctx.PropertyErrorf(attrName, "Attribute value needs to be at least T")
			}
		}
	}
}

func (module *sdkLibraryXml) validateMinAndMaxDeviceSdk(ctx android.ModuleContext) {
	if module.properties.Min_device_sdk != nil && module.properties.Max_device_sdk != nil {
		min, minErr := android.ApiLevelFromUser(ctx, *module.properties.Min_device_sdk)
		max, maxErr := android.ApiLevelFromUser(ctx, *module.properties.Max_device_sdk)
		if minErr == nil && maxErr == nil {
			// we will inform the user of invalid inputs when we try to write the
			// permissions xml file so we don't need to do it here
			if min.GreaterThan(max) {
				ctx.ModuleErrorf("min_device_sdk can't be greater than max_device_sdk")
			}
		}
	}
}

func (module *sdkLibraryXml) validateMinMaxDeviceSdkAndModuleMinSdk(ctx android.ModuleContext) {
	moduleMinApi := android.ApiLevelOrPanic(ctx, *module.properties.Sdk_library_min_api_level)
	if module.properties.Min_device_sdk != nil {
		api, err := android.ApiLevelFromUser(ctx, *module.properties.Min_device_sdk)
		if err == nil {
			if moduleMinApi.GreaterThan(api) {
				ctx.PropertyErrorf("min_device_sdk", "Can't be less than module's min sdk (%s)", moduleMinApi)
			}
		}
	}
	if module.properties.Max_device_sdk != nil {
		api, err := android.ApiLevelFromUser(ctx, *module.properties.Max_device_sdk)
		if err == nil {
			if moduleMinApi.GreaterThan(api) {
				ctx.PropertyErrorf("max_device_sdk", "Can't be less than module's min sdk (%s)", moduleMinApi)
			}
		}
	}
}

func (module *sdkLibraryXml) validateOnBootclasspathBeforeRequirements(ctx android.ModuleContext) {
	moduleMinApi := android.ApiLevelOrPanic(ctx, *module.properties.Sdk_library_min_api_level)
	if module.properties.On_bootclasspath_before != nil {
		t := android.ApiLevelOrPanic(ctx, "Tiramisu")
		// if we use the attribute, then we need to do this validation
		if moduleMinApi.LessThan(t) {
			// if minAPi is < T, then we need to have min_device_sdk (which only accepts T+)
			if module.properties.Min_device_sdk == nil {
				ctx.PropertyErrorf("on_bootclasspath_before", "Using this property requires that the module's min_sdk_version or the shared library's min_device_sdk is at least T")
			}
		}
	}
}

type sdkLibrarySdkMemberType struct {
	android.SdkMemberTypeBase
}

func (s *sdkLibrarySdkMemberType) AddDependencies(ctx android.SdkDependencyContext, dependencyTag blueprint.DependencyTag, names []string) {
	ctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (s *sdkLibrarySdkMemberType) IsInstance(module android.Module) bool {
	_, ok := module.(*SdkLibrary)
	return ok
}

func (s *sdkLibrarySdkMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	return ctx.SnapshotBuilder().AddPrebuiltModule(member, "java_sdk_library_import")
}

func (s *sdkLibrarySdkMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &sdkLibrarySdkMemberProperties{}
}

var javaSdkLibrarySdkMemberType = &sdkLibrarySdkMemberType{
	android.SdkMemberTypeBase{
		PropertyName: "java_sdk_libs",
		SupportsSdk:  true,
	},
}

type sdkLibrarySdkMemberProperties struct {
	android.SdkMemberPropertiesBase

	// Scope to per scope properties.
	Scopes map[*apiScope]*scopeProperties

	// The Java stubs source files.
	Stub_srcs []string

	// The naming scheme.
	Naming_scheme *string

	// True if the java_sdk_library_import is for a shared library, false
	// otherwise.
	Shared_library *bool

	// True if the stub imports should produce dex jars.
	Compile_dex *bool

	// The paths to the doctag files to add to the prebuilt.
	Doctag_paths android.Paths

	Permitted_packages []string

	// Signals that this shared library is part of the bootclasspath starting
	// on the version indicated in this attribute.
	//
	// This will make platforms at this level and above to ignore
	// <uses-library> tags with this library name because the library is already
	// available
	On_bootclasspath_since *string

	// Signals that this shared library was part of the bootclasspath before
	// (but not including) the version indicated in this attribute.
	//
	// The system will automatically add a <uses-library> tag with this library to
	// apps that target any SDK less than the version indicated in this attribute.
	On_bootclasspath_before *string

	// Indicates that PackageManager should ignore this shared library if the
	// platform is below the version indicated in this attribute.
	//
	// This means that the device won't recognise this library as installed.
	Min_device_sdk *string

	// Indicates that PackageManager should ignore this shared library if the
	// platform is above the version indicated in this attribute.
	//
	// This means that the device won't recognise this library as installed.
	Max_device_sdk *string
}

type scopeProperties struct {
	Jars           android.Paths
	StubsSrcJar    android.Path
	CurrentApiFile android.Path
	RemovedApiFile android.Path
	AnnotationsZip android.Path `supported_build_releases:"Tiramisu+"`
	SdkVersion     string
}

func (s *sdkLibrarySdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	sdk := variant.(*SdkLibrary)

	s.Scopes = make(map[*apiScope]*scopeProperties)
	for _, apiScope := range allApiScopes {
		paths := sdk.findScopePaths(apiScope)
		if paths == nil {
			continue
		}

		jars := paths.stubsImplPath
		if len(jars) > 0 {
			properties := scopeProperties{}
			properties.Jars = jars
			properties.SdkVersion = sdk.sdkVersionForStubsLibrary(ctx.SdkModuleContext(), apiScope)
			properties.StubsSrcJar = paths.stubsSrcJar.Path()
			if paths.currentApiFilePath.Valid() {
				properties.CurrentApiFile = paths.currentApiFilePath.Path()
			}
			if paths.removedApiFilePath.Valid() {
				properties.RemovedApiFile = paths.removedApiFilePath.Path()
			}
			// The annotations zip is only available for modules that set annotations_enabled: true.
			if paths.annotationsZip.Valid() {
				properties.AnnotationsZip = paths.annotationsZip.Path()
			}
			s.Scopes[apiScope] = &properties
		}
	}

	s.Naming_scheme = sdk.commonSdkLibraryProperties.Naming_scheme
	s.Shared_library = proptools.BoolPtr(sdk.sharedLibrary())
	s.Compile_dex = sdk.dexProperties.Compile_dex
	s.Doctag_paths = sdk.doctagPaths
	s.Permitted_packages = sdk.PermittedPackagesForUpdatableBootJars()
	s.On_bootclasspath_since = sdk.commonSdkLibraryProperties.On_bootclasspath_since
	s.On_bootclasspath_before = sdk.commonSdkLibraryProperties.On_bootclasspath_before
	s.Min_device_sdk = sdk.commonSdkLibraryProperties.Min_device_sdk
	s.Max_device_sdk = sdk.commonSdkLibraryProperties.Max_device_sdk
}

func (s *sdkLibrarySdkMemberProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	if s.Naming_scheme != nil {
		propertySet.AddProperty("naming_scheme", proptools.String(s.Naming_scheme))
	}
	if s.Shared_library != nil {
		propertySet.AddProperty("shared_library", *s.Shared_library)
	}
	if s.Compile_dex != nil {
		propertySet.AddProperty("compile_dex", *s.Compile_dex)
	}
	if len(s.Permitted_packages) > 0 {
		propertySet.AddProperty("permitted_packages", s.Permitted_packages)
	}

	for _, apiScope := range allApiScopes {
		if properties, ok := s.Scopes[apiScope]; ok {
			scopeSet := propertySet.AddPropertySet(apiScope.propertyName)

			scopeDir := filepath.Join("sdk_library", s.OsPrefix(), apiScope.name)

			var jars []string
			for _, p := range properties.Jars {
				dest := filepath.Join(scopeDir, ctx.Name()+"-stubs.jar")
				ctx.SnapshotBuilder().CopyToSnapshot(p, dest)
				jars = append(jars, dest)
			}
			scopeSet.AddProperty("jars", jars)

			if ctx.SdkModuleContext().Config().IsEnvTrue("SOONG_SDK_SNAPSHOT_USE_SRCJAR") {
				// Copy the stubs source jar into the snapshot zip as is.
				srcJarSnapshotPath := filepath.Join(scopeDir, ctx.Name()+".srcjar")
				ctx.SnapshotBuilder().CopyToSnapshot(properties.StubsSrcJar, srcJarSnapshotPath)
				scopeSet.AddProperty("stub_srcs", []string{srcJarSnapshotPath})
			} else {
				// Merge the stubs source jar into the snapshot zip so that when it is unpacked
				// the source files are also unpacked.
				snapshotRelativeDir := filepath.Join(scopeDir, ctx.Name()+"_stub_sources")
				ctx.SnapshotBuilder().UnzipToSnapshot(properties.StubsSrcJar, snapshotRelativeDir)
				scopeSet.AddProperty("stub_srcs", []string{snapshotRelativeDir})
			}

			if properties.CurrentApiFile != nil {
				currentApiSnapshotPath := filepath.Join(scopeDir, ctx.Name()+".txt")
				ctx.SnapshotBuilder().CopyToSnapshot(properties.CurrentApiFile, currentApiSnapshotPath)
				scopeSet.AddProperty("current_api", currentApiSnapshotPath)
			}

			if properties.RemovedApiFile != nil {
				removedApiSnapshotPath := filepath.Join(scopeDir, ctx.Name()+"-removed.txt")
				ctx.SnapshotBuilder().CopyToSnapshot(properties.RemovedApiFile, removedApiSnapshotPath)
				scopeSet.AddProperty("removed_api", removedApiSnapshotPath)
			}

			if properties.AnnotationsZip != nil {
				annotationsSnapshotPath := filepath.Join(scopeDir, ctx.Name()+"_annotations.zip")
				ctx.SnapshotBuilder().CopyToSnapshot(properties.AnnotationsZip, annotationsSnapshotPath)
				scopeSet.AddProperty("annotations", annotationsSnapshotPath)
			}

			if properties.SdkVersion != "" {
				scopeSet.AddProperty("sdk_version", properties.SdkVersion)
			}
		}
	}

	if len(s.Doctag_paths) > 0 {
		dests := []string{}
		for _, p := range s.Doctag_paths {
			dest := filepath.Join("doctags", p.Rel())
			ctx.SnapshotBuilder().CopyToSnapshot(p, dest)
			dests = append(dests, dest)
		}
		propertySet.AddProperty("doctag_files", dests)
	}
}
