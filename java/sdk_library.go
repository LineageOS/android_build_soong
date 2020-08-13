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
)

const (
	sdkXmlFileSuffix    = ".xml"
	permissionsTemplate = `<?xml version=\"1.0\" encoding=\"utf-8\"?>\n` +
		`<!-- Copyright (C) 2018 The Android Open Source Project\n` +
		`\n` +
		`    Licensed under the Apache License, Version 2.0 (the \"License\");\n` +
		`    you may not use this file except in compliance with the License.\n` +
		`    You may obtain a copy of the License at\n` +
		`\n` +
		`        http://www.apache.org/licenses/LICENSE-2.0\n` +
		`\n` +
		`    Unless required by applicable law or agreed to in writing, software\n` +
		`    distributed under the License is distributed on an \"AS IS\" BASIS,\n` +
		`    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.\n` +
		`    See the License for the specific language governing permissions and\n` +
		`    limitations under the License.\n` +
		`-->\n` +
		`<permissions>\n` +
		`    <library name=\"%s\" file=\"%s\"/>\n` +
		`</permissions>\n`
)

// A tag to associated a dependency with a specific api scope.
type scopeDependencyTag struct {
	blueprint.BaseDependencyTag
	name     string
	apiScope *apiScope

	// Function for extracting appropriate path information from the dependency.
	depInfoExtractor func(paths *scopePaths, dep android.Module) error
}

// Extract tag specific information from the dependency.
func (tag scopeDependencyTag) extractDepInfo(ctx android.ModuleContext, dep android.Module, paths *scopePaths) {
	err := tag.depInfoExtractor(paths, dep)
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

	// Extra arguments to pass to droidstubs for this scope.
	droidstubsArgs []string

	// The args that must be passed to droidstubs to generate the stubs source
	// for this scope.
	//
	// The stubs source must include the definitions of everything that is in this
	// api scope and all the scopes that this one extends.
	droidstubsArgsForGeneratingStubsSource []string

	// The args that must be passed to droidstubs to generate the API for this scope.
	//
	// The API only includes the additional members that this scope adds over the scope
	// that it extends.
	droidstubsArgsForGeneratingApi []string

	// True if the stubs source and api can be created by the same metalava invocation.
	createStubsSourceAndApiTogether bool

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
	var stubsSourceArgs []string
	for s := scope; s != nil; s = s.extends {
		stubsSourceArgs = append(stubsSourceArgs, s.droidstubsArgs...)
	}
	scope.droidstubsArgsForGeneratingStubsSource = stubsSourceArgs

	// Currently the args needed to generate the API are the same as the args
	// needed to add additional members.
	apiArgs := scope.droidstubsArgs
	scope.droidstubsArgsForGeneratingApi = apiArgs

	// If the args needed to generate the stubs and API are the same then they
	// can be generated in a single invocation of metalava, otherwise they will
	// need separate invocations.
	scope.createStubsSourceAndApiTogether = reflect.DeepEqual(stubsSourceArgs, apiArgs)

	return scope
}

func (scope *apiScope) stubsLibraryModuleName(baseName string) string {
	return baseName + ".stubs" + scope.moduleSuffix
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
		apiFilePrefix:  "system-",
		moduleSuffix:   ".system",
		sdkVersion:     "system_current",
		droidstubsArgs: []string{"-showAnnotation android.annotation.SystemApi\\(client=android.annotation.SystemApi.Client.PRIVILEGED_APPS\\)"},
	})
	apiScopeTest = initApiScope(&apiScope{
		name:                "test",
		extends:             apiScopePublic,
		legacyEnabledStatus: (*SdkLibrary).generateTestAndSystemScopesByDefault,
		scopeSpecificProperties: func(module *SdkLibrary) *ApiScopeProperties {
			return &module.sdkLibraryProperties.Test
		},
		apiFilePrefix:  "test-",
		moduleSuffix:   ".test",
		sdkVersion:     "test_current",
		droidstubsArgs: []string{"-showAnnotation android.annotation.TestApi"},
		unstable:       true,
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
		droidstubsArgs: []string{
			"--show-annotation android.annotation.SystemApi\\(client=android.annotation.SystemApi.Client.MODULE_LIBRARIES\\)",
		},
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
		droidstubsArgs: []string{
			"--show-annotation android.annotation.SystemApi\\(client=android.annotation.SystemApi.Client.SYSTEM_SERVER\\) ",
			"--hide-annotation android.annotation.Hide",
			// com.android.* classes are okay in this interface"
			"--hide InternalClasses",
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
	android.RegisterSdkMemberType(&sdkLibrarySdkMemberType{
		android.SdkMemberTypeBase{
			PropertyName: "java_sdk_libs",
			SupportsSdk:  true,
		},
	})
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
	// 1) If the sdk_version specified on the java_sdk_library is none then this
	//    will be none. This is used for java_sdk_library instances that are used
	//    to create stubs that contribute to the core_current sdk version.
	// 2) Otherwise, it is assumed that this library extends but does not contribute
	//    directly to a specific sdk_version and so this uses the sdk_version appropriate
	//    for the api scope. e.g. public will use sdk_version: current, system will use
	//    sdk_version: system_current, etc.
	//
	// This does not affect the sdk_version used for either generating the stubs source
	// or the API file. They both have to use the same sdk_version as is used for
	// compiling the implementation library.
	Sdk_version *string
}

type sdkLibraryProperties struct {
	// Visibility for impl library module. If not specified then defaults to the
	// visibility property.
	Impl_library_visibility []string

	// Visibility for stubs library modules. If not specified then defaults to the
	// visibility property.
	Stubs_library_visibility []string

	// Visibility for stubs source modules. If not specified then defaults to the
	// visibility property.
	Stubs_source_visibility []string

	// List of Java libraries that will be in the classpath when building stubs
	Stub_only_libs []string `android:"arch_variant"`

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

	// additional droiddoc options
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

	// If set to true, the path of dist files is apistubs/core. Defaults to false.
	Core_lib *bool

	// don't create dist rules.
	No_dist *bool `blueprint:"mutated"`

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

	// The API specification file, e.g. system_current.txt.
	currentApiFilePath android.OptionalPath

	// The specification of API elements removed since the last release.
	removedApiFilePath android.OptionalPath

	// The stubs source jar.
	stubsSrcJar android.OptionalPath
}

func (paths *scopePaths) extractStubsLibraryInfoFromDependency(dep android.Module) error {
	if lib, ok := dep.(Dependency); ok {
		paths.stubsHeaderPath = lib.HeaderJars()
		paths.stubsImplPath = lib.ImplementationJars()
		return nil
	} else {
		return fmt.Errorf("expected module that implements Dependency, e.g. java_library")
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
	paths.currentApiFilePath = android.OptionalPathForPath(provider.ApiFilePath())
	paths.removedApiFilePath = android.OptionalPathForPath(provider.RemovedApiFilePath())
}

func (paths *scopePaths) extractApiInfoFromDep(dep android.Module) error {
	return paths.treatDepAsApiStubsProvider(dep, func(provider ApiStubsProvider) {
		paths.extractApiInfoFromApiStubsProvider(provider)
	})
}

func (paths *scopePaths) extractStubsSourceInfoFromApiStubsProviders(provider ApiStubsSrcProvider) {
	paths.stubsSrcJar = android.OptionalPathForPath(provider.StubsSrcJar())
}

func (paths *scopePaths) extractStubsSourceInfoFromDep(dep android.Module) error {
	return paths.treatDepAsApiStubsSrcProvider(dep, func(provider ApiStubsSrcProvider) {
		paths.extractStubsSourceInfoFromApiStubsProviders(provider)
	})
}

func (paths *scopePaths) extractStubsSourceAndApiInfoFromApiStubsProvider(dep android.Module) error {
	return paths.treatDepAsApiStubsProvider(dep, func(provider ApiStubsProvider) {
		paths.extractApiInfoFromApiStubsProvider(provider)
		paths.extractStubsSourceInfoFromApiStubsProviders(provider)
	})
}

type commonToSdkLibraryAndImportProperties struct {
	// The naming scheme to use for the components that this module creates.
	//
	// If not specified then it defaults to "default". The other allowable value is
	// "framework-modules" which matches the scheme currently used by framework modules
	// for the equivalent components represented as separate Soong modules.
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
}

// Common code between sdk library and sdk library import
type commonToSdkLibraryAndImport struct {
	moduleBase *android.ModuleBase

	scopePaths map[*apiScope]*scopePaths

	namingScheme sdkLibraryComponentNamingScheme

	commonSdkLibraryProperties commonToSdkLibraryAndImportProperties

	// Functionality related to this being used as a component of a java_sdk_library.
	EmbeddableSdkLibraryComponent
}

func (c *commonToSdkLibraryAndImport) initCommon(moduleBase *android.ModuleBase) {
	c.moduleBase = moduleBase

	moduleBase.AddProperties(&c.commonSdkLibraryProperties)

	// Initialize this as an sdk library component.
	c.initSdkLibraryComponent(moduleBase)
}

func (c *commonToSdkLibraryAndImport) initCommonAfterDefaultsApplied(ctx android.DefaultableHookContext) bool {
	schemeProperty := proptools.StringDefault(c.commonSdkLibraryProperties.Naming_scheme, "default")
	switch schemeProperty {
	case "default":
		c.namingScheme = &defaultNamingScheme{}
	case "framework-modules":
		c.namingScheme = &frameworkModulesNamingScheme{}
	default:
		ctx.PropertyErrorf("naming_scheme", "expected 'default' but was %q", schemeProperty)
		return false
	}

	// Only track this sdk library if this can be used as a shared library.
	if c.sharedLibrary() {
		// Use the name specified in the module definition as the owner.
		c.sdkLibraryComponentProperties.SdkLibraryToImplicitlyTrack = proptools.StringPtr(c.moduleBase.BaseModuleName())
	}

	return true
}

// Module name of the runtime implementation library
func (c *commonToSdkLibraryAndImport) implLibraryModuleName() string {
	return c.moduleBase.BaseModuleName() + ".impl"
}

// Module name of the XML file for the lib
func (c *commonToSdkLibraryAndImport) xmlPermissionsModuleName() string {
	return c.moduleBase.BaseModuleName() + sdkXmlFileSuffix
}

// Name of the java_library module that compiles the stubs source.
func (c *commonToSdkLibraryAndImport) stubsLibraryModuleName(apiScope *apiScope) string {
	return c.namingScheme.stubsLibraryModuleName(apiScope, c.moduleBase.BaseModuleName())
}

// Name of the droidstubs module that generates the stubs source and may also
// generate/check the API.
func (c *commonToSdkLibraryAndImport) stubsSourceModuleName(apiScope *apiScope) string {
	return c.namingScheme.stubsSourceModuleName(apiScope, c.moduleBase.BaseModuleName())
}

// Name of the droidstubs module that generates/checks the API. Only used if it
// requires different arts to the stubs source generating module.
func (c *commonToSdkLibraryAndImport) apiModuleName(apiScope *apiScope) string {
	return c.namingScheme.apiModuleName(apiScope, c.moduleBase.BaseModuleName())
}

// The component names for different outputs of the java_sdk_library.
//
// They are similar to the names used for the child modules it creates
const (
	stubsSourceComponentName = "stubs.source"

	apiTxtComponentName = "api.txt"

	removedApiTxtComponentName = "removed-api.txt"
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
	componentsRegexp := choice(stubsSourceComponentName, apiTxtComponentName, removedApiTxtComponentName)

	// Regular expression to match any combination of one scope and one component.
	return regexp.MustCompile(fmt.Sprintf(`^\.(%s)\.(%s)$`, scopesRegexp, componentsRegexp))
}()

// For OutputFileProducer interface
//
// .<scope>.stubs.source
// .<scope>.api.txt
// .<scope>.removed-api.txt
func (c *commonToSdkLibraryAndImport) commonOutputFiles(tag string) (android.Paths, error) {
	if groups := tagSplitter.FindStringSubmatch(tag); groups != nil {
		scopeName := groups[1]
		component := groups[2]

		if scope, ok := scopeByName[scopeName]; ok {
			paths := c.findScopePaths(scope)
			if paths == nil {
				return nil, fmt.Errorf("%q does not provide api scope %s", c.moduleBase.BaseModuleName(), scopeName)
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
			}

			return nil, fmt.Errorf("%s not available for api scope %s", component, scopeName)
		} else {
			return nil, fmt.Errorf("unknown scope %s in %s", scope, tag)
		}

	} else {
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

func (c *commonToSdkLibraryAndImport) selectHeaderJarsForSdkVersion(ctx android.BaseModuleContext, sdkVersion sdkSpec) android.Paths {

	// If a specific numeric version has been requested then use prebuilt versions of the sdk.
	if sdkVersion.version.isNumbered() {
		return PrebuiltJars(ctx, c.moduleBase.BaseModuleName(), sdkVersion)
	}

	var apiScope *apiScope
	switch sdkVersion.kind {
	case sdkSystem:
		apiScope = apiScopeSystem
	case sdkModule:
		apiScope = apiScopeModuleLib
	case sdkTest:
		apiScope = apiScopeTest
	case sdkSystemServer:
		apiScope = apiScopeSystemServer
	default:
		apiScope = apiScopePublic
	}

	paths := c.findClosestScopePath(apiScope)
	if paths == nil {
		var scopes []string
		for _, s := range allApiScopes {
			if c.findScopePaths(s) != nil {
				scopes = append(scopes, s.name)
			}
		}
		ctx.ModuleErrorf("requires api scope %s from %s but it only has %q available", apiScope.name, c.moduleBase.BaseModuleName(), scopes)
		return nil
	}

	return paths.stubsHeaderPath
}

func (c *commonToSdkLibraryAndImport) sdkComponentPropertiesForChildLibrary() interface{} {
	componentProps := &struct {
		SdkLibraryToImplicitlyTrack *string
	}{}

	if c.sharedLibrary() {
		// Mark the stubs library as being components of this java_sdk_library so that
		// any app that includes code which depends (directly or indirectly) on the stubs
		// library will have the appropriate <uses-library> invocation inserted into its
		// manifest if necessary.
		componentProps.SdkLibraryToImplicitlyTrack = proptools.StringPtr(c.moduleBase.BaseModuleName())
	}

	return componentProps
}

// Check if this can be used as a shared library.
func (c *commonToSdkLibraryAndImport) sharedLibrary() bool {
	return proptools.BoolDefault(c.commonSdkLibraryProperties.Shared_library, true)
}

// Properties related to the use of a module as an component of a java_sdk_library.
type SdkLibraryComponentProperties struct {

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

func (e *EmbeddableSdkLibraryComponent) initSdkLibraryComponent(moduleBase *android.ModuleBase) {
	moduleBase.AddProperties(&e.sdkLibraryComponentProperties)
}

// to satisfy SdkLibraryComponentDependency
func (e *EmbeddableSdkLibraryComponent) OptionalImplicitSdkLibrary() []string {
	if e.sdkLibraryComponentProperties.SdkLibraryToImplicitlyTrack != nil {
		return []string{*e.sdkLibraryComponentProperties.SdkLibraryToImplicitlyTrack}
	}
	return nil
}

// Implemented by modules that are (or possibly could be) a component of a java_sdk_library
// (including the java_sdk_library) itself.
type SdkLibraryComponentDependency interface {
	// The optional name of the sdk library that should be implicitly added to the
	// AndroidManifest of an app that contains code which references the sdk library.
	//
	// Returns an array containing 0 or 1 items rather than a *string to make it easier
	// to append this to the list of exported sdk libraries.
	OptionalImplicitSdkLibrary() []string
}

// Make sure that all the module types that are components of java_sdk_library/_import
// and which can be referenced (directly or indirectly) from an android app implement
// the SdkLibraryComponentDependency interface.
var _ SdkLibraryComponentDependency = (*Library)(nil)
var _ SdkLibraryComponentDependency = (*Import)(nil)
var _ SdkLibraryComponentDependency = (*SdkLibrary)(nil)
var _ SdkLibraryComponentDependency = (*SdkLibraryImport)(nil)

// Provides access to sdk_version related header and implentation jars.
type SdkLibraryDependency interface {
	SdkLibraryComponentDependency

	// Get the header jars appropriate for the supplied sdk_version.
	//
	// These are turbine generated jars so they only change if the externals of the
	// class changes but it does not contain and implementation or JavaDoc.
	SdkHeaderJars(ctx android.BaseModuleContext, sdkVersion sdkSpec) android.Paths

	// Get the implementation jars appropriate for the supplied sdk version.
	//
	// These are either the implementation jar for the whole sdk library or the implementation
	// jars for the stubs. The latter should only be needed when generating JavaDoc as otherwise
	// they are identical to the corresponding header jars.
	SdkImplementationJars(ctx android.BaseModuleContext, sdkVersion sdkSpec) android.Paths
}

type SdkLibrary struct {
	Library

	sdkLibraryProperties sdkLibraryProperties

	// Map from api scope to the scope specific property structure.
	scopeToProperties map[*apiScope]*ApiScopeProperties

	commonToSdkLibraryAndImport
}

var _ Dependency = (*SdkLibrary)(nil)
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

		// If the stubs source and API cannot be generated together then add an additional dependency on
		// the API module.
		if apiScope.createStubsSourceAndApiTogether {
			// Add a dependency on the stubs source in order to access both stubs source and api information.
			ctx.AddVariationDependencies(nil, apiScope.stubsSourceAndApiTag, module.stubsSourceModuleName(apiScope))
		} else {
			// Add separate dependencies on the creators of the stubs source files and the API.
			ctx.AddVariationDependencies(nil, apiScope.stubsSourceTag, module.stubsSourceModuleName(apiScope))
			ctx.AddVariationDependencies(nil, apiScope.apiFileTag, module.apiModuleName(apiScope))
		}
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
	if module.requiresRuntimeImplementationLibrary() {
		// Only add the deps for the library if it is actually going to be built.
		module.Library.deps(ctx)
	}
}

func (module *SdkLibrary) OutputFiles(tag string) (android.Paths, error) {
	paths, err := module.commonOutputFiles(tag)
	if paths == nil && err == nil {
		return module.Library.OutputFiles(tag)
	} else {
		return paths, err
	}
}

func (module *SdkLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Only build an implementation library if required.
	if module.requiresRuntimeImplementationLibrary() {
		module.Library.GenerateAndroidBuildActions(ctx)
	}

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
		}
	})
}

func (module *SdkLibrary) AndroidMkEntries() []android.AndroidMkEntries {
	if !module.requiresRuntimeImplementationLibrary() {
		return nil
	}
	entriesList := module.Library.AndroidMkEntries()
	entries := &entriesList[0]
	entries.Required = append(entries.Required, module.xmlPermissionsModuleName())
	return entriesList
}

// The dist path of the stub artifacts
func (module *SdkLibrary) apiDistPath(apiScope *apiScope) string {
	if module.ModuleBase.Owner() != "" {
		return path.Join("apistubs", module.ModuleBase.Owner(), apiScope.name)
	} else if Bool(module.sdkLibraryProperties.Core_lib) {
		return path.Join("apistubs", "core", apiScope.name)
	} else {
		return path.Join("apistubs", "android", apiScope.name)
	}
}

// Get the sdk version for use when compiling the stubs library.
func (module *SdkLibrary) sdkVersionForStubsLibrary(mctx android.EarlyModuleContext, apiScope *apiScope) string {
	scopeProperties := module.scopeToProperties[apiScope]
	if scopeProperties.Sdk_version != nil {
		return proptools.String(scopeProperties.Sdk_version)
	}

	sdkDep := decodeSdkDep(mctx, sdkContext(&module.Library))
	if sdkDep.hasStandardLibs() {
		// If building against a standard sdk then use the sdk version appropriate for the scope.
		return apiScope.sdkVersion
	} else {
		// Otherwise, use no system module.
		return "none"
	}
}

func (module *SdkLibrary) latestApiFilegroupName(apiScope *apiScope) string {
	return ":" + module.BaseModuleName() + ".api." + apiScope.name + ".latest"
}

func (module *SdkLibrary) latestRemovedApiFilegroupName(apiScope *apiScope) string {
	return ":" + module.BaseModuleName() + "-removed.api." + apiScope.name + ".latest"
}

// Creates the implementation java library
func (module *SdkLibrary) createImplLibrary(mctx android.DefaultableHookContext) {

	moduleNamePtr := proptools.StringPtr(module.BaseModuleName())

	props := struct {
		Name              *string
		Visibility        []string
		Instrument        bool
		ConfigurationName *string
	}{
		Name:       proptools.StringPtr(module.implLibraryModuleName()),
		Visibility: module.sdkLibraryProperties.Impl_library_visibility,
		// Set the instrument property to ensure it is instrumented when instrumentation is required.
		Instrument: true,

		// Make the created library behave as if it had the same name as this module.
		ConfigurationName: moduleNamePtr,
	}

	properties := []interface{}{
		&module.properties,
		&module.protoProperties,
		&module.deviceProperties,
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
		Name              *string
		Visibility        []string
		Srcs              []string
		Installable       *bool
		Sdk_version       *string
		System_modules    *string
		Patch_module      *string
		Libs              []string
		Compile_dex       *bool
		Java_version      *string
		Product_variables struct {
			Pdk struct {
				Enabled *bool
			}
		}
		Openjdk9 struct {
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

	// If stubs_library_visibility is not set then the created module will use the
	// visibility of this module.
	visibility := module.sdkLibraryProperties.Stubs_library_visibility
	props.Visibility = visibility

	// sources are generated from the droiddoc
	props.Srcs = []string{":" + module.stubsSourceModuleName(apiScope)}
	sdkVersion := module.sdkVersionForStubsLibrary(mctx, apiScope)
	props.Sdk_version = proptools.StringPtr(sdkVersion)
	props.System_modules = module.deviceProperties.System_modules
	props.Patch_module = module.properties.Patch_module
	props.Installable = proptools.BoolPtr(false)
	props.Libs = module.sdkLibraryProperties.Stub_only_libs
	// The stub-annotations library contains special versions of the annotations
	// with CLASS retention policy, so that they're kept.
	if proptools.Bool(module.sdkLibraryProperties.Annotations_enabled) {
		props.Libs = append(props.Libs, "stub-annotations")
	}
	props.Product_variables.Pdk.Enabled = proptools.BoolPtr(false)
	props.Openjdk9.Srcs = module.properties.Openjdk9.Srcs
	props.Openjdk9.Javacflags = module.properties.Openjdk9.Javacflags
	// We compile the stubs for 1.8 in line with the main android.jar stubs, and potential
	// interop with older developer tools that don't support 1.9.
	props.Java_version = proptools.StringPtr("1.8")
	if module.deviceProperties.Compile_dex != nil {
		props.Compile_dex = module.deviceProperties.Compile_dex
	}

	// Dist the class jar artifact for sdk builds.
	if !Bool(module.sdkLibraryProperties.No_dist) {
		props.Dist.Targets = []string{"sdk", "win_sdk"}
		props.Dist.Dest = proptools.StringPtr(fmt.Sprintf("%v.jar", module.BaseModuleName()))
		props.Dist.Dir = proptools.StringPtr(module.apiDistPath(apiScope))
		props.Dist.Tag = proptools.StringPtr(".jar")
	}

	mctx.CreateModule(LibraryFactory, &props, module.sdkComponentPropertiesForChildLibrary())
}

// Creates a droidstubs module that creates stubs source files from the given full source
// files and also updates and checks the API specification files.
func (module *SdkLibrary) createStubsSourcesAndApi(mctx android.DefaultableHookContext, apiScope *apiScope, name string, createStubSources, createApi bool, scopeSpecificDroidstubsArgs []string) {
	props := struct {
		Name                             *string
		Visibility                       []string
		Srcs                             []string
		Installable                      *bool
		Sdk_version                      *string
		System_modules                   *string
		Libs                             []string
		Arg_files                        []string
		Args                             *string
		Java_version                     *string
		Annotations_enabled              *bool
		Merge_annotations_dirs           []string
		Merge_inclusion_annotations_dirs []string
		Generate_stubs                   *bool
		Check_api                        struct {
			Current                   ApiToCheck
			Last_released             ApiToCheck
			Ignore_missing_latest_api *bool

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
		Dist struct {
			Targets []string
			Dest    *string
			Dir     *string
		}
	}{}

	// The stubs source processing uses the same compile time classpath when extracting the
	// API from the implementation library as it does when compiling it. i.e. the same
	// * sdk version
	// * system_modules
	// * libs (static_libs/libs)

	props.Name = proptools.StringPtr(name)

	// If stubs_source_visibility is not set then the created module will use the
	// visibility of this module.
	visibility := module.sdkLibraryProperties.Stubs_source_visibility
	props.Visibility = visibility

	props.Srcs = append(props.Srcs, module.properties.Srcs...)
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
		"MissingPermission",
		"BroadcastBehavior",
		"HiddenSuperclass",
		"DeprecationMismatch",
		"UnavailableSymbol",
		"SdkConstant",
		"HiddenTypeParameter",
		"Todo",
		"Typo",
	}
	droidstubsArgs = append(droidstubsArgs, android.JoinWithPrefix(disabledWarnings, "--hide "))

	if !createStubSources {
		// Stubs are not required.
		props.Generate_stubs = proptools.BoolPtr(false)
	}

	// Add in scope specific arguments.
	droidstubsArgs = append(droidstubsArgs, scopeSpecificDroidstubsArgs...)
	props.Arg_files = module.sdkLibraryProperties.Droiddoc_option_files
	props.Args = proptools.StringPtr(strings.Join(droidstubsArgs, " "))

	if createApi {
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

		if !apiScope.unstable {
			// check against the latest released API
			latestApiFilegroupName := proptools.StringPtr(module.latestApiFilegroupName(apiScope))
			props.Check_api.Last_released.Api_file = latestApiFilegroupName
			props.Check_api.Last_released.Removed_api_file = proptools.StringPtr(
				module.latestRemovedApiFilegroupName(apiScope))
			props.Check_api.Ignore_missing_latest_api = proptools.BoolPtr(true)

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

		// Dist the api txt artifact for sdk builds.
		if !Bool(module.sdkLibraryProperties.No_dist) {
			props.Dist.Targets = []string{"sdk", "win_sdk"}
			props.Dist.Dest = proptools.StringPtr(fmt.Sprintf("%v.txt", module.BaseModuleName()))
			props.Dist.Dir = proptools.StringPtr(path.Join(module.apiDistPath(apiScope), "api"))
		}
	}

	mctx.CreateModule(DroidstubsFactory, &props)
}

func (module *SdkLibrary) DepIsInSameApex(mctx android.BaseModuleContext, dep android.Module) bool {
	depTag := mctx.OtherModuleDependencyTag(dep)
	if depTag == xmlPermissionsFileTag {
		return true
	}
	return module.Library.DepIsInSameApex(mctx, dep)
}

// Creates the xml file that publicizes the runtime library
func (module *SdkLibrary) createXmlFile(mctx android.DefaultableHookContext) {
	props := struct {
		Name           *string
		Lib_name       *string
		Apex_available []string
	}{
		Name:           proptools.StringPtr(module.xmlPermissionsModuleName()),
		Lib_name:       proptools.StringPtr(module.BaseModuleName()),
		Apex_available: module.ApexProperties.Apex_available,
	}

	mctx.CreateModule(sdkLibraryXmlFactory, &props)
}

func PrebuiltJars(ctx android.BaseModuleContext, baseName string, s sdkSpec) android.Paths {
	var ver sdkVersion
	var kind sdkKind
	if s.usePrebuilt(ctx) {
		ver = s.version
		kind = s.kind
	} else {
		// We don't have prebuilt SDK for the specific sdkVersion.
		// Instead of breaking the build, fallback to use "system_current"
		ver = sdkVersionCurrent
		kind = sdkSystem
	}

	dir := filepath.Join("prebuilts", "sdk", ver.String(), kind.String())
	jar := filepath.Join(dir, baseName+".jar")
	jarPath := android.ExistentPathForSource(ctx, jar)
	if !jarPath.Valid() {
		if ctx.Config().AllowMissingDependencies() {
			return android.Paths{android.PathForSource(ctx, jar)}
		} else {
			ctx.PropertyErrorf("sdk_library", "invalid sdk version %q, %q does not exist", s.raw, jar)
		}
		return nil
	}
	return android.Paths{jarPath.Path()}
}

// Get the apex name for module, "" if it is for platform.
func getApexNameForModule(module android.Module) string {
	if apex, ok := module.(android.ApexModule); ok {
		return apex.ApexVariationName()
	}

	return ""
}

// Check to see if the other module is within the same named APEX as this module.
//
// If either this or the other module are on the platform then this will return
// false.
func withinSameApexAs(module android.ApexModule, other android.Module) bool {
	name := module.ApexVariationName()
	return name != "" && getApexNameForModule(other) == name
}

func (module *SdkLibrary) sdkJars(ctx android.BaseModuleContext, sdkVersion sdkSpec, headerJars bool) android.Paths {
	// If the client doesn't set sdk_version, but if this library prefers stubs over
	// the impl library, let's provide the widest API surface possible. To do so,
	// force override sdk_version to module_current so that the closest possible API
	// surface could be found in selectHeaderJarsForSdkVersion
	if module.defaultsToStubs() && !sdkVersion.specified() {
		sdkVersion = sdkSpecFrom("module_current")
	}

	// Only provide access to the implementation library if it is actually built.
	if module.requiresRuntimeImplementationLibrary() {
		// Check any special cases for java_sdk_library.
		//
		// Only allow access to the implementation library in the following condition:
		// * No sdk_version specified on the referencing module.
		// * The referencing module is in the same apex as this.
		if sdkVersion.kind == sdkPrivate || withinSameApexAs(module, ctx.Module()) {
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
func (module *SdkLibrary) SdkHeaderJars(ctx android.BaseModuleContext, sdkVersion sdkSpec) android.Paths {
	return module.sdkJars(ctx, sdkVersion, true /*headerJars*/)
}

// to satisfy SdkLibraryDependency interface
func (module *SdkLibrary) SdkImplementationJars(ctx android.BaseModuleContext, sdkVersion sdkSpec) android.Paths {
	return module.sdkJars(ctx, sdkVersion, false /*headerJars*/)
}

func (module *SdkLibrary) SetNoDist() {
	module.sdkLibraryProperties.No_dist = proptools.BoolPtr(true)
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
	// then assume it provides both system and test apis. Otherwise, assume it does not and
	// also assume it does not contribute to the dist build.
	sdkDep := decodeSdkDep(mctx, sdkContext(&module.Library))
	hasSystemAndTestApis := sdkDep.hasStandardLibs()
	module.sdkLibraryProperties.Generate_system_and_test_apis = hasSystemAndTestApis
	module.sdkLibraryProperties.No_dist = proptools.BoolPtr(!hasSystemAndTestApis)

	missing_current_api := false

	generatedScopes := module.getGeneratedApiScopes(mctx)

	apiDir := module.getApiDir()
	for _, scope := range generatedScopes {
		for _, api := range []string{"current.txt", "removed.txt"} {
			path := path.Join(mctx.ModuleDir(), apiDir, scope.apiFilePrefix+api)
			p := android.ExistentPathForSource(mctx, path)
			if !p.Valid() {
				mctx.ModuleErrorf("Current api file %#v doesn't exist", path)
				missing_current_api = true
			}
		}
	}

	if missing_current_api {
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
		stubsSourceArgs := scope.droidstubsArgsForGeneratingStubsSource
		stubsSourceModuleName := module.stubsSourceModuleName(scope)

		// If the args needed to generate the stubs and API are the same then they
		// can be generated in a single invocation of metalava, otherwise they will
		// need separate invocations.
		if scope.createStubsSourceAndApiTogether {
			// Use the stubs source name for legacy reasons.
			module.createStubsSourcesAndApi(mctx, scope, stubsSourceModuleName, true, true, stubsSourceArgs)
		} else {
			module.createStubsSourcesAndApi(mctx, scope, stubsSourceModuleName, true, false, stubsSourceArgs)

			apiArgs := scope.droidstubsArgsForGeneratingApi
			apiName := module.apiModuleName(scope)
			module.createStubsSourcesAndApi(mctx, scope, apiName, false, true, apiArgs)
		}

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
}

func (module *SdkLibrary) InitSdkLibraryProperties() {
	module.addHostAndDeviceProperties()
	module.AddProperties(&module.sdkLibraryProperties)

	module.initSdkLibraryComponent(&module.ModuleBase)

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

	apiModuleName(scope *apiScope, baseName string) string
}

type defaultNamingScheme struct {
}

func (s *defaultNamingScheme) stubsLibraryModuleName(scope *apiScope, baseName string) string {
	return scope.stubsLibraryModuleName(baseName)
}

func (s *defaultNamingScheme) stubsSourceModuleName(scope *apiScope, baseName string) string {
	return scope.stubsSourceModuleName(baseName)
}

func (s *defaultNamingScheme) apiModuleName(scope *apiScope, baseName string) string {
	return scope.apiModuleName(baseName)
}

var _ sdkLibraryComponentNamingScheme = (*defaultNamingScheme)(nil)

type frameworkModulesNamingScheme struct {
}

func (s *frameworkModulesNamingScheme) moduleSuffix(scope *apiScope) string {
	suffix := scope.name
	if scope == apiScopeModuleLib {
		suffix = "module_libs_"
	}
	return suffix
}

func (s *frameworkModulesNamingScheme) stubsLibraryModuleName(scope *apiScope, baseName string) string {
	return fmt.Sprintf("%s-stubs-%sapi", baseName, s.moduleSuffix(scope))
}

func (s *frameworkModulesNamingScheme) stubsSourceModuleName(scope *apiScope, baseName string) string {
	return fmt.Sprintf("%s-stubs-srcs-%sapi", baseName, s.moduleSuffix(scope))
}

func (s *frameworkModulesNamingScheme) apiModuleName(scope *apiScope, baseName string) string {
	return fmt.Sprintf("%s-api-%sapi", baseName, s.moduleSuffix(scope))
}

var _ sdkLibraryComponentNamingScheme = (*frameworkModulesNamingScheme)(nil)

func moduleStubLinkType(name string) (stub bool, ret linkType) {
	// This suffix-based approach is fragile and could potentially mis-trigger.
	// TODO(b/155164730): Clean this up when modules no longer reference sdk_lib stubs directly.
	if strings.HasSuffix(name, ".stubs.public") || strings.HasSuffix(name, "-stubs-publicapi") {
		return true, javaSdk
	}
	if strings.HasSuffix(name, ".stubs.system") || strings.HasSuffix(name, "-stubs-systemapi") {
		return true, javaSystem
	}
	if strings.HasSuffix(name, ".stubs.module_lib") || strings.HasSuffix(name, "-stubs-module_libs_api") {
		return true, javaModule
	}
	if strings.HasSuffix(name, ".stubs.test") {
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
	module.initCommon(&module.ModuleBase)

	module.InitSdkLibraryProperties()
	android.InitApexModule(module)
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
}

type sdkLibraryImportProperties struct {
	// List of shared java libs, common to all scopes, that this module has
	// dependencies to
	Libs []string
}

type SdkLibraryImport struct {
	android.ModuleBase
	android.DefaultableModuleBase
	prebuilt android.Prebuilt
	android.ApexModuleBase
	android.SdkBase

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
	module.initCommon(&module.ModuleBase)

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

func (module *SdkLibraryImport) Prebuilt() *android.Prebuilt {
	return &module.prebuilt
}

func (module *SdkLibraryImport) Name() string {
	return module.prebuilt.Name(module.ModuleBase.Name())
}

func (module *SdkLibraryImport) createInternalModules(mctx android.DefaultableHookContext) {

	// If the build is configured to use prebuilts then force this to be preferred.
	if mctx.Config().UnbundledBuildUsePrebuiltSdks() {
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
		Prefer      *bool
	}{}
	props.Name = proptools.StringPtr(module.stubsLibraryModuleName(apiScope))
	props.Sdk_version = scopeProperties.Sdk_version
	// Prepend any of the libs from the legacy public properties to the libs for each of the
	// scopes to avoid having to duplicate them in each scope.
	props.Libs = append(module.properties.Libs, scopeProperties.Libs...)
	props.Jars = scopeProperties.Jars

	// The imports are preferred if the java_sdk_library_import is preferred.
	props.Prefer = proptools.BoolPtr(module.prebuilt.Prefer())

	mctx.CreateModule(ImportFactory, &props, module.sdkComponentPropertiesForChildLibrary())
}

func (module *SdkLibraryImport) createPrebuiltStubsSources(mctx android.DefaultableHookContext, apiScope *apiScope, scopeProperties *sdkLibraryScopeProperties) {
	props := struct {
		Name   *string
		Srcs   []string
		Prefer *bool
	}{}
	props.Name = proptools.StringPtr(module.stubsSourceModuleName(apiScope))
	props.Srcs = scopeProperties.Stub_srcs
	mctx.CreateModule(PrebuiltStubsSourcesFactory, &props)

	// The stubs source is preferred if the java_sdk_library_import is preferred.
	props.Prefer = proptools.BoolPtr(module.prebuilt.Prefer())
}

// Add the dependencies on the child module in the component deps mutator so that it
// creates references to the prebuilt and not the source modules.
func (module *SdkLibraryImport) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	for apiScope, scopeProperties := range module.scopeProperties {
		if len(scopeProperties.Jars) == 0 {
			continue
		}

		// Add dependencies to the prebuilt stubs library
		ctx.AddVariationDependencies(nil, apiScope.stubsTag, "prebuilt_"+module.stubsLibraryModuleName(apiScope))

		if len(scopeProperties.Stub_srcs) > 0 {
			// Add dependencies to the prebuilt stubs source library
			ctx.AddVariationDependencies(nil, apiScope.stubsSourceTag, "prebuilt_"+module.stubsSourceModuleName(apiScope))
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

func (module *SdkLibraryImport) DepIsInSameApex(mctx android.BaseModuleContext, dep android.Module) bool {
	depTag := mctx.OtherModuleDependencyTag(dep)
	if depTag == xmlPermissionsFileTag {
		return true
	}

	// None of the other dependencies of the java_sdk_library_import are in the same apex
	// as the one that references this module.
	return false
}

func (module *SdkLibraryImport) OutputFiles(tag string) (android.Paths, error) {
	return module.commonOutputFiles(tag)
}

func (module *SdkLibraryImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
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
		paths.currentApiFilePath = android.OptionalPathForModuleSrc(ctx, scopeProperties.Current_api)
		paths.removedApiFilePath = android.OptionalPathForModuleSrc(ctx, scopeProperties.Removed_api)
	}
}

func (module *SdkLibraryImport) sdkJars(ctx android.BaseModuleContext, sdkVersion sdkSpec, headerJars bool) android.Paths {

	// For consistency with SdkLibrary make the implementation jar available to libraries that
	// are within the same APEX.
	implLibraryModule := module.implLibraryModule
	if implLibraryModule != nil && withinSameApexAs(module, ctx.Module()) {
		if headerJars {
			return implLibraryModule.HeaderJars()
		} else {
			return implLibraryModule.ImplementationJars()
		}
	}

	return module.selectHeaderJarsForSdkVersion(ctx, sdkVersion)
}

// to satisfy SdkLibraryDependency interface
func (module *SdkLibraryImport) SdkHeaderJars(ctx android.BaseModuleContext, sdkVersion sdkSpec) android.Paths {
	// This module is just a wrapper for the prebuilt stubs.
	return module.sdkJars(ctx, sdkVersion, true)
}

// to satisfy SdkLibraryDependency interface
func (module *SdkLibraryImport) SdkImplementationJars(ctx android.BaseModuleContext, sdkVersion sdkSpec) android.Paths {
	// This module is just a wrapper for the stubs.
	return module.sdkJars(ctx, sdkVersion, false)
}

// to satisfy apex.javaDependency interface
func (module *SdkLibraryImport) DexJar() android.Path {
	if module.implLibraryModule == nil {
		return nil
	} else {
		return module.implLibraryModule.DexJar()
	}
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
}

type sdkLibraryXmlProperties struct {
	// canonical name of the lib
	Lib_name *string
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

// File path to the runtime implementation library
func (module *sdkLibraryXml) implPath() string {
	implName := proptools.String(module.properties.Lib_name)
	if apexName := module.ApexVariationName(); apexName != "" {
		// TODO(b/146468504): ApexVariationName() is only a soong module name, not apex name.
		// In most cases, this works fine. But when apex_name is set or override_apex is used
		// this can be wrong.
		return fmt.Sprintf("/apex/%s/javalib/%s.jar", apexName, implName)
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

func (module *sdkLibraryXml) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	libName := proptools.String(module.properties.Lib_name)
	xmlContent := fmt.Sprintf(permissionsTemplate, libName, module.implPath())

	module.outputFilePath = android.PathForModuleOut(ctx, libName+".xml").OutputPath
	rule := android.NewRuleBuilder()
	rule.Command().
		Text("/bin/bash -c \"echo -e '" + xmlContent + "'\" > ").
		Output(module.outputFilePath)

	rule.Build(pctx, ctx, "java_sdk_xml", "Permission XML")

	module.installDirPath = android.PathForModuleInstall(ctx, "etc", module.SubDir())
}

func (module *sdkLibraryXml) AndroidMkEntries() []android.AndroidMkEntries {
	if !module.IsForPlatform() {
		return []android.AndroidMkEntries{android.AndroidMkEntries{
			Disabled: true,
		}}
	}

	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(module.outputFilePath),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_TAGS", "optional")
				entries.SetString("LOCAL_MODULE_PATH", module.installDirPath.ToMakePath().String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", module.outputFilePath.Base())
			},
		},
	}}
}

type sdkLibrarySdkMemberType struct {
	android.SdkMemberTypeBase
}

func (s *sdkLibrarySdkMemberType) AddDependencies(mctx android.BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string) {
	mctx.AddVariationDependencies(nil, dependencyTag, names...)
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

type sdkLibrarySdkMemberProperties struct {
	android.SdkMemberPropertiesBase

	// Scope to per scope properties.
	Scopes map[*apiScope]scopeProperties

	// Additional libraries that the exported stubs libraries depend upon.
	Libs []string

	// The Java stubs source files.
	Stub_srcs []string

	// The naming scheme.
	Naming_scheme *string

	// True if the java_sdk_library_import is for a shared library, false
	// otherwise.
	Shared_library *bool
}

type scopeProperties struct {
	Jars           android.Paths
	StubsSrcJar    android.Path
	CurrentApiFile android.Path
	RemovedApiFile android.Path
	SdkVersion     string
}

func (s *sdkLibrarySdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	sdk := variant.(*SdkLibrary)

	s.Scopes = make(map[*apiScope]scopeProperties)
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
			s.Scopes[apiScope] = properties
		}
	}

	s.Libs = sdk.properties.Libs
	s.Naming_scheme = sdk.commonSdkLibraryProperties.Naming_scheme
	s.Shared_library = proptools.BoolPtr(sdk.sharedLibrary())
}

func (s *sdkLibrarySdkMemberProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	if s.Naming_scheme != nil {
		propertySet.AddProperty("naming_scheme", proptools.String(s.Naming_scheme))
	}
	if s.Shared_library != nil {
		propertySet.AddProperty("shared_library", *s.Shared_library)
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

			// Merge the stubs source jar into the snapshot zip so that when it is unpacked
			// the source files are also unpacked.
			snapshotRelativeDir := filepath.Join(scopeDir, ctx.Name()+"_stub_sources")
			ctx.SnapshotBuilder().UnzipToSnapshot(properties.StubsSrcJar, snapshotRelativeDir)
			scopeSet.AddProperty("stub_srcs", []string{snapshotRelativeDir})

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

			if properties.SdkVersion != "" {
				scopeSet.AddProperty("sdk_version", properties.SdkVersion)
			}
		}
	}

	if len(s.Libs) > 0 {
		propertySet.AddPropertyWithTag("libs", s.Libs, ctx.SnapshotBuilder().SdkMemberReferencePropertyTag(false))
	}
}
