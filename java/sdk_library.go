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
	"android/soong/android"

	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

const (
	sdkStubsLibrarySuffix = ".stubs"
	sdkSystemApiSuffix    = ".system"
	sdkTestApiSuffix      = ".test"
	sdkStubsSourceSuffix  = ".stubs.source"
	sdkXmlFileSuffix      = ".xml"
	permissionsTemplate   = `<?xml version=\"1.0\" encoding=\"utf-8\"?>\n` +
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
}

// Provides information about an api scope, e.g. public, system, test.
type apiScope struct {
	// The name of the api scope, e.g. public, system, test
	name string

	// The tag to use to depend on the stubs library module.
	stubsTag scopeDependencyTag

	// The tag to use to depend on the stubs
	apiFileTag scopeDependencyTag

	// The scope specific prefix to add to the api file base of "current.txt" or "removed.txt".
	apiFilePrefix string

	// The scope specific prefix to add to the sdk library module name to construct a scope specific
	// module name.
	moduleSuffix string

	// The suffix to add to the make variable that references the location of the api file.
	apiFileMakeVariableSuffix string

	// SDK version that the stubs library is built against. Note that this is always
	// *current. Older stubs library built with a numbered SDK version is created from
	// the prebuilt jar.
	sdkVersion string
}

// Initialize a scope, creating and adding appropriate dependency tags
func initApiScope(scope *apiScope) *apiScope {
	//apiScope := &scope
	scope.stubsTag = scopeDependencyTag{
		name:     scope.name + "-stubs",
		apiScope: scope,
	}
	scope.apiFileTag = scopeDependencyTag{
		name:     scope.name + "-api",
		apiScope: scope,
	}
	return scope
}

func (scope *apiScope) stubsModuleName(baseName string) string {
	return baseName + sdkStubsLibrarySuffix + scope.moduleSuffix
}

func (scope *apiScope) docsModuleName(baseName string) string {
	return baseName + sdkStubsSourceSuffix + scope.moduleSuffix
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
	apiScopePublic = initApiScope(&apiScope{
		name:       "public",
		sdkVersion: "current",
	})
	apiScopeSystem = initApiScope(&apiScope{
		name:                      "system",
		apiFilePrefix:             "system-",
		moduleSuffix:              sdkSystemApiSuffix,
		apiFileMakeVariableSuffix: "_SYSTEM",
		sdkVersion:                "system_current",
	})
	apiScopeTest = initApiScope(&apiScope{
		name:                      "test",
		apiFilePrefix:             "test-",
		moduleSuffix:              sdkTestApiSuffix,
		apiFileMakeVariableSuffix: "_TEST",
		sdkVersion:                "test_current",
	})
	allApiScopes = apiScopes{
		apiScopePublic,
		apiScopeSystem,
		apiScopeTest,
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
}

func RegisterSdkLibraryBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_sdk_library", SdkLibraryFactory)
	ctx.RegisterModuleType("java_sdk_library_import", sdkLibraryImportFactory)
}

type sdkLibraryProperties struct {
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

	// If set to true there is no runtime library.
	Api_only *bool

	// local files that are used within user customized droiddoc options.
	Droiddoc_option_files []string

	// additional droiddoc options
	// Available variables for substitution:
	//
	//  $(location <label>): the path to the droiddoc_option_files with name <label>
	Droiddoc_options []string

	// a list of top-level directories containing files to merge qualifier annotations
	// (i.e. those intended to be included in the stubs written) from.
	Merge_annotations_dirs []string

	// a list of top-level directories containing Java stub files to merge show/hide annotations from.
	Merge_inclusion_annotations_dirs []string

	// If set to true, the path of dist files is apistubs/core. Defaults to false.
	Core_lib *bool

	// don't create dist rules.
	No_dist *bool `blueprint:"mutated"`

	// indicates whether system and test apis should be managed.
	Has_system_and_test_apis bool `blueprint:"mutated"`

	// TODO: determines whether to create HTML doc or not
	//Html_doc *bool
}

type scopePaths struct {
	stubsHeaderPath android.Paths
	stubsImplPath   android.Paths
	apiFilePath     android.Path
}

// Common code between sdk library and sdk library import
type commonToSdkLibraryAndImport struct {
	scopePaths map[*apiScope]*scopePaths
}

func (c *commonToSdkLibraryAndImport) getScopePaths(scope *apiScope) *scopePaths {
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

type SdkLibrary struct {
	Library

	sdkLibraryProperties sdkLibraryProperties

	commonToSdkLibraryAndImport
}

var _ Dependency = (*SdkLibrary)(nil)
var _ SdkLibraryDependency = (*SdkLibrary)(nil)

func (module *SdkLibrary) getActiveApiScopes() apiScopes {
	if module.sdkLibraryProperties.Has_system_and_test_apis {
		return allApiScopes
	} else {
		return apiScopes{apiScopePublic}
	}
}

var xmlPermissionsFileTag = dependencyTag{name: "xml-permissions-file"}

func IsXmlPermissionsFileDepTag(depTag blueprint.DependencyTag) bool {
	if dt, ok := depTag.(dependencyTag); ok {
		return dt == xmlPermissionsFileTag
	}
	return false
}

func (module *SdkLibrary) DepsMutator(ctx android.BottomUpMutatorContext) {
	for _, apiScope := range module.getActiveApiScopes() {
		// Add dependencies to the stubs library
		ctx.AddVariationDependencies(nil, apiScope.stubsTag, module.stubsName(apiScope))

		// And the api file
		ctx.AddVariationDependencies(nil, apiScope.apiFileTag, module.docsName(apiScope))
	}

	if !proptools.Bool(module.sdkLibraryProperties.Api_only) {
		// Add dependency to the rule for generating the xml permissions file
		ctx.AddDependency(module, xmlPermissionsFileTag, module.xmlFileName())
	}

	module.Library.deps(ctx)
}

func (module *SdkLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Don't build an implementation library if this is api only.
	if !proptools.Bool(module.sdkLibraryProperties.Api_only) {
		module.Library.GenerateAndroidBuildActions(ctx)
	}

	// Record the paths to the header jars of the library (stubs and impl).
	// When this java_sdk_library is depended upon from others via "libs" property,
	// the recorded paths will be returned depending on the link type of the caller.
	ctx.VisitDirectDeps(func(to android.Module) {
		otherName := ctx.OtherModuleName(to)
		tag := ctx.OtherModuleDependencyTag(to)

		if lib, ok := to.(Dependency); ok {
			if scopeTag, ok := tag.(scopeDependencyTag); ok {
				apiScope := scopeTag.apiScope
				scopePaths := module.getScopePaths(apiScope)
				scopePaths.stubsHeaderPath = lib.HeaderJars()
				scopePaths.stubsImplPath = lib.ImplementationJars()
			}
		}
		if doc, ok := to.(ApiFilePath); ok {
			if scopeTag, ok := tag.(scopeDependencyTag); ok {
				apiScope := scopeTag.apiScope
				scopePaths := module.getScopePaths(apiScope)
				scopePaths.apiFilePath = doc.ApiFilePath()
			} else {
				ctx.ModuleErrorf("depends on module %q of unknown tag %q", otherName, tag)
			}
		}
	})
}

func (module *SdkLibrary) AndroidMkEntries() []android.AndroidMkEntries {
	if proptools.Bool(module.sdkLibraryProperties.Api_only) {
		return nil
	}
	entriesList := module.Library.AndroidMkEntries()
	entries := &entriesList[0]
	entries.Required = append(entries.Required, module.xmlFileName())
	return entriesList
}

// Module name of the stubs library
func (module *SdkLibrary) stubsName(apiScope *apiScope) string {
	return apiScope.stubsModuleName(module.BaseModuleName())
}

// Module name of the docs
func (module *SdkLibrary) docsName(apiScope *apiScope) string {
	return apiScope.docsModuleName(module.BaseModuleName())
}

// Module name of the runtime implementation library
func (module *SdkLibrary) implName() string {
	return module.BaseModuleName()
}

// Module name of the XML file for the lib
func (module *SdkLibrary) xmlFileName() string {
	return module.BaseModuleName() + sdkXmlFileSuffix
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
func (module *SdkLibrary) sdkVersionForStubsLibrary(mctx android.LoadHookContext, apiScope *apiScope) string {
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

// Creates a static java library that has API stubs
func (module *SdkLibrary) createStubsLibrary(mctx android.LoadHookContext, apiScope *apiScope) {
	props := struct {
		Name                *string
		Srcs                []string
		Installable         *bool
		Sdk_version         *string
		System_modules      *string
		Patch_module        *string
		Libs                []string
		Soc_specific        *bool
		Device_specific     *bool
		Product_specific    *bool
		System_ext_specific *bool
		Compile_dex         *bool
		Java_version        *string
		Product_variables   struct {
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

	props.Name = proptools.StringPtr(module.stubsName(apiScope))
	// sources are generated from the droiddoc
	props.Srcs = []string{":" + module.docsName(apiScope)}
	sdkVersion := module.sdkVersionForStubsLibrary(mctx, apiScope)
	props.Sdk_version = proptools.StringPtr(sdkVersion)
	props.System_modules = module.Library.Module.deviceProperties.System_modules
	props.Patch_module = module.Library.Module.properties.Patch_module
	props.Installable = proptools.BoolPtr(false)
	props.Libs = module.sdkLibraryProperties.Stub_only_libs
	props.Product_variables.Pdk.Enabled = proptools.BoolPtr(false)
	props.Openjdk9.Srcs = module.Library.Module.properties.Openjdk9.Srcs
	props.Openjdk9.Javacflags = module.Library.Module.properties.Openjdk9.Javacflags
	props.Java_version = module.Library.Module.properties.Java_version
	if module.Library.Module.deviceProperties.Compile_dex != nil {
		props.Compile_dex = module.Library.Module.deviceProperties.Compile_dex
	}

	if module.SocSpecific() {
		props.Soc_specific = proptools.BoolPtr(true)
	} else if module.DeviceSpecific() {
		props.Device_specific = proptools.BoolPtr(true)
	} else if module.ProductSpecific() {
		props.Product_specific = proptools.BoolPtr(true)
	} else if module.SystemExtSpecific() {
		props.System_ext_specific = proptools.BoolPtr(true)
	}
	// Dist the class jar artifact for sdk builds.
	if !Bool(module.sdkLibraryProperties.No_dist) {
		props.Dist.Targets = []string{"sdk", "win_sdk"}
		props.Dist.Dest = proptools.StringPtr(fmt.Sprintf("%v.jar", module.BaseModuleName()))
		props.Dist.Dir = proptools.StringPtr(module.apiDistPath(apiScope))
		props.Dist.Tag = proptools.StringPtr(".jar")
	}

	mctx.CreateModule(LibraryFactory, &props)
}

// Creates a droiddoc module that creates stubs source files from the given full source
// files
func (module *SdkLibrary) createStubsSources(mctx android.LoadHookContext, apiScope *apiScope) {
	props := struct {
		Name                             *string
		Srcs                             []string
		Installable                      *bool
		Sdk_version                      *string
		System_modules                   *string
		Libs                             []string
		Arg_files                        []string
		Args                             *string
		Java_version                     *string
		Merge_annotations_dirs           []string
		Merge_inclusion_annotations_dirs []string
		Check_api                        struct {
			Current                   ApiToCheck
			Last_released             ApiToCheck
			Ignore_missing_latest_api *bool
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

	sdkDep := decodeSdkDep(mctx, sdkContext(&module.Library))
	// Use the platform API if standard libraries were requested, otherwise use
	// no default libraries.
	sdkVersion := ""
	if !sdkDep.hasStandardLibs() {
		sdkVersion = "none"
	}

	props.Name = proptools.StringPtr(module.docsName(apiScope))
	props.Srcs = append(props.Srcs, module.Library.Module.properties.Srcs...)
	props.Sdk_version = proptools.StringPtr(sdkVersion)
	props.System_modules = module.Library.Module.deviceProperties.System_modules
	props.Installable = proptools.BoolPtr(false)
	// A droiddoc module has only one Libs property and doesn't distinguish between
	// shared libs and static libs. So we need to add both of these libs to Libs property.
	props.Libs = module.Library.Module.properties.Libs
	props.Libs = append(props.Libs, module.Library.Module.properties.Static_libs...)
	props.Aidl.Include_dirs = module.Library.Module.deviceProperties.Aidl.Include_dirs
	props.Aidl.Local_include_dirs = module.Library.Module.deviceProperties.Aidl.Local_include_dirs
	props.Java_version = module.Library.Module.properties.Java_version

	props.Merge_annotations_dirs = module.sdkLibraryProperties.Merge_annotations_dirs
	props.Merge_inclusion_annotations_dirs = module.sdkLibraryProperties.Merge_inclusion_annotations_dirs

	droiddocArgs := []string{}
	if len(module.sdkLibraryProperties.Api_packages) != 0 {
		droiddocArgs = append(droiddocArgs, "--stub-packages "+strings.Join(module.sdkLibraryProperties.Api_packages, ":"))
	}
	if len(module.sdkLibraryProperties.Hidden_api_packages) != 0 {
		droiddocArgs = append(droiddocArgs,
			android.JoinWithPrefix(module.sdkLibraryProperties.Hidden_api_packages, " --hide-package "))
	}
	droiddocArgs = append(droiddocArgs, module.sdkLibraryProperties.Droiddoc_options...)
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
	droiddocArgs = append(droiddocArgs, android.JoinWithPrefix(disabledWarnings, "--hide "))

	switch apiScope {
	case apiScopeSystem:
		droiddocArgs = append(droiddocArgs, "-showAnnotation android.annotation.SystemApi")
	case apiScopeTest:
		droiddocArgs = append(droiddocArgs, " -showAnnotation android.annotation.TestApi")
	}
	props.Arg_files = module.sdkLibraryProperties.Droiddoc_option_files
	props.Args = proptools.StringPtr(strings.Join(droiddocArgs, " "))

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

	// check against the latest released API
	props.Check_api.Last_released.Api_file = proptools.StringPtr(
		module.latestApiFilegroupName(apiScope))
	props.Check_api.Last_released.Removed_api_file = proptools.StringPtr(
		module.latestRemovedApiFilegroupName(apiScope))
	props.Check_api.Ignore_missing_latest_api = proptools.BoolPtr(true)

	// Dist the api txt artifact for sdk builds.
	if !Bool(module.sdkLibraryProperties.No_dist) {
		props.Dist.Targets = []string{"sdk", "win_sdk"}
		props.Dist.Dest = proptools.StringPtr(fmt.Sprintf("%v.txt", module.BaseModuleName()))
		props.Dist.Dir = proptools.StringPtr(path.Join(module.apiDistPath(apiScope), "api"))
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
func (module *SdkLibrary) createXmlFile(mctx android.LoadHookContext) {
	props := struct {
		Name                *string
		Lib_name            *string
		Soc_specific        *bool
		Device_specific     *bool
		Product_specific    *bool
		System_ext_specific *bool
		Apex_available      []string
	}{
		Name:           proptools.StringPtr(module.xmlFileName()),
		Lib_name:       proptools.StringPtr(module.BaseModuleName()),
		Apex_available: module.ApexProperties.Apex_available,
	}

	if module.SocSpecific() {
		props.Soc_specific = proptools.BoolPtr(true)
	} else if module.DeviceSpecific() {
		props.Device_specific = proptools.BoolPtr(true)
	} else if module.ProductSpecific() {
		props.Product_specific = proptools.BoolPtr(true)
	} else if module.SystemExtSpecific() {
		props.System_ext_specific = proptools.BoolPtr(true)
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

func (module *SdkLibrary) sdkJars(
	ctx android.BaseModuleContext,
	sdkVersion sdkSpec,
	headerJars bool) android.Paths {

	// If a specific numeric version has been requested then use prebuilt versions of the sdk.
	if sdkVersion.version.isNumbered() {
		return PrebuiltJars(ctx, module.BaseModuleName(), sdkVersion)
	} else {
		if !sdkVersion.specified() {
			if headerJars {
				return module.Library.HeaderJars()
			} else {
				return module.Library.ImplementationJars()
			}
		}
		var apiScope *apiScope
		switch sdkVersion.kind {
		case sdkSystem:
			apiScope = apiScopeSystem
		case sdkTest:
			apiScope = apiScopeTest
		case sdkPrivate:
			return module.Library.HeaderJars()
		default:
			apiScope = apiScopePublic
		}

		paths := module.getScopePaths(apiScope)
		if headerJars {
			return paths.stubsHeaderPath
		} else {
			return paths.stubsImplPath
		}
	}
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
func (module *SdkLibrary) CreateInternalModules(mctx android.LoadHookContext) {
	if len(module.Library.Module.properties.Srcs) == 0 {
		mctx.PropertyErrorf("srcs", "java_sdk_library must specify srcs")
		return
	}

	// If this builds against standard libraries (i.e. is not part of the core libraries)
	// then assume it provides both system and test apis. Otherwise, assume it does not and
	// also assume it does not contribute to the dist build.
	sdkDep := decodeSdkDep(mctx, sdkContext(&module.Library))
	hasSystemAndTestApis := sdkDep.hasStandardLibs()
	module.sdkLibraryProperties.Has_system_and_test_apis = hasSystemAndTestApis
	module.sdkLibraryProperties.No_dist = proptools.BoolPtr(!hasSystemAndTestApis)

	missing_current_api := false

	activeScopes := module.getActiveApiScopes()

	apiDir := module.getApiDir()
	for _, scope := range activeScopes {
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
			strings.Join(activeScopes.Strings(func(s *apiScope) string { return s.apiFilePrefix }), " "))
		return
	}

	for _, scope := range activeScopes {
		module.createStubsLibrary(mctx, scope)
		module.createStubsSources(mctx, scope)
	}

	if !proptools.Bool(module.sdkLibraryProperties.Api_only) {
		// for runtime
		module.createXmlFile(mctx)

		// record java_sdk_library modules so that they are exported to make
		javaSdkLibraries := javaSdkLibraries(mctx.Config())
		javaSdkLibrariesLock.Lock()
		defer javaSdkLibrariesLock.Unlock()
		*javaSdkLibraries = append(*javaSdkLibraries, module.BaseModuleName())
	}
}

func (module *SdkLibrary) InitSdkLibraryProperties() {
	module.AddProperties(
		&module.sdkLibraryProperties,
		&module.Library.Module.properties,
		&module.Library.Module.dexpreoptProperties,
		&module.Library.Module.deviceProperties,
		&module.Library.Module.protoProperties,
	)

	module.Library.Module.properties.Installable = proptools.BoolPtr(true)
	module.Library.Module.deviceProperties.IsSDKLibrary = true
}

// java_sdk_library is a special Java library that provides optional platform APIs to apps.
// In practice, it can be viewed as a combination of several modules: 1) stubs library that clients
// are linked against to, 2) droiddoc module that internally generates API stubs source files,
// 3) the real runtime shared library that implements the APIs, and 4) XML file for adding
// the runtime lib to the classpath at runtime if requested via <uses-library>.
func SdkLibraryFactory() android.Module {
	module := &SdkLibrary{}
	module.InitSdkLibraryProperties()
	android.InitApexModule(module)
	InitJavaModule(module, android.HostAndDeviceSupported)
	android.AddLoadHook(module, func(ctx android.LoadHookContext) { module.CreateInternalModules(ctx) })
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
}

type sdkLibraryImportProperties struct {
	// List of shared java libs, common to all scopes, that this module has
	// dependencies to
	Libs []string

	// Properties associated with the public api scope.
	Public sdkLibraryScopeProperties

	// Properties associated with the system api scope.
	System sdkLibraryScopeProperties

	// Properties associated with the test api scope.
	Test sdkLibraryScopeProperties
}

type sdkLibraryImport struct {
	android.ModuleBase
	android.DefaultableModuleBase
	prebuilt android.Prebuilt

	properties sdkLibraryImportProperties

	commonToSdkLibraryAndImport
}

var _ SdkLibraryDependency = (*sdkLibraryImport)(nil)

// java_sdk_library_import imports a prebuilt java_sdk_library.
func sdkLibraryImportFactory() android.Module {
	module := &sdkLibraryImport{}

	module.AddProperties(&module.properties)

	android.InitPrebuiltModule(module, &[]string{""})
	InitJavaModule(module, android.HostAndDeviceSupported)

	android.AddLoadHook(module, func(mctx android.LoadHookContext) { module.createInternalModules(mctx) })
	return module
}

func (module *sdkLibraryImport) Prebuilt() *android.Prebuilt {
	return &module.prebuilt
}

func (module *sdkLibraryImport) Name() string {
	return module.prebuilt.Name(module.ModuleBase.Name())
}

func (module *sdkLibraryImport) createInternalModules(mctx android.LoadHookContext) {

	// If the build is configured to use prebuilts then force this to be preferred.
	if mctx.Config().UnbundledBuildUsePrebuiltSdks() {
		module.prebuilt.ForcePrefer()
	}

	for apiScope, scopeProperties := range module.scopeProperties() {
		if len(scopeProperties.Jars) == 0 {
			continue
		}

		// Creates a java import for the jar with ".stubs" suffix
		props := struct {
			Name                *string
			Soc_specific        *bool
			Device_specific     *bool
			Product_specific    *bool
			System_ext_specific *bool
			Sdk_version         *string
			Libs                []string
			Jars                []string
			Prefer              *bool
		}{}

		props.Name = proptools.StringPtr(apiScope.stubsModuleName(module.BaseModuleName()))
		props.Sdk_version = scopeProperties.Sdk_version
		// Prepend any of the libs from the legacy public properties to the libs for each of the
		// scopes to avoid having to duplicate them in each scope.
		props.Libs = append(module.properties.Libs, scopeProperties.Libs...)
		props.Jars = scopeProperties.Jars

		if module.SocSpecific() {
			props.Soc_specific = proptools.BoolPtr(true)
		} else if module.DeviceSpecific() {
			props.Device_specific = proptools.BoolPtr(true)
		} else if module.ProductSpecific() {
			props.Product_specific = proptools.BoolPtr(true)
		} else if module.SystemExtSpecific() {
			props.System_ext_specific = proptools.BoolPtr(true)
		}

		// If the build should use prebuilt sdks then set prefer to true on the stubs library.
		// That will cause the prebuilt version of the stubs to override the source version.
		if mctx.Config().UnbundledBuildUsePrebuiltSdks() {
			props.Prefer = proptools.BoolPtr(true)
		}

		mctx.CreateModule(ImportFactory, &props)
	}

	javaSdkLibraries := javaSdkLibraries(mctx.Config())
	javaSdkLibrariesLock.Lock()
	defer javaSdkLibrariesLock.Unlock()
	*javaSdkLibraries = append(*javaSdkLibraries, module.BaseModuleName())
}

func (module *sdkLibraryImport) scopeProperties() map[*apiScope]*sdkLibraryScopeProperties {
	p := make(map[*apiScope]*sdkLibraryScopeProperties)
	p[apiScopePublic] = &module.properties.Public
	p[apiScopeSystem] = &module.properties.System
	p[apiScopeTest] = &module.properties.Test
	return p
}

func (module *sdkLibraryImport) DepsMutator(ctx android.BottomUpMutatorContext) {
	for apiScope, scopeProperties := range module.scopeProperties() {
		if len(scopeProperties.Jars) == 0 {
			continue
		}

		// Add dependencies to the prebuilt stubs library
		ctx.AddVariationDependencies(nil, apiScope.stubsTag, apiScope.stubsModuleName(module.BaseModuleName()))
	}
}

func (module *sdkLibraryImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Record the paths to the prebuilt stubs library.
	ctx.VisitDirectDeps(func(to android.Module) {
		tag := ctx.OtherModuleDependencyTag(to)

		if lib, ok := to.(Dependency); ok {
			if scopeTag, ok := tag.(scopeDependencyTag); ok {
				apiScope := scopeTag.apiScope
				scopePaths := module.getScopePaths(apiScope)
				scopePaths.stubsHeaderPath = lib.HeaderJars()
			}
		}
	})
}

func (module *sdkLibraryImport) sdkJars(
	ctx android.BaseModuleContext,
	sdkVersion sdkSpec) android.Paths {

	// If a specific numeric version has been requested then use prebuilt versions of the sdk.
	if sdkVersion.version.isNumbered() {
		return PrebuiltJars(ctx, module.BaseModuleName(), sdkVersion)
	}

	var apiScope *apiScope
	switch sdkVersion.kind {
	case sdkSystem:
		apiScope = apiScopeSystem
	case sdkTest:
		apiScope = apiScopeTest
	default:
		apiScope = apiScopePublic
	}

	paths := module.getScopePaths(apiScope)
	return paths.stubsHeaderPath
}

// to satisfy SdkLibraryDependency interface
func (module *sdkLibraryImport) SdkHeaderJars(ctx android.BaseModuleContext, sdkVersion sdkSpec) android.Paths {
	// This module is just a wrapper for the prebuilt stubs.
	return module.sdkJars(ctx, sdkVersion)
}

// to satisfy SdkLibraryDependency interface
func (module *sdkLibraryImport) SdkImplementationJars(ctx android.BaseModuleContext, sdkVersion sdkSpec) android.Paths {
	// This module is just a wrapper for the stubs.
	return module.sdkJars(ctx, sdkVersion)
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
	if apexName := module.ApexName(); apexName != "" {
		// TODO(b/146468504): ApexName() is only a soong module name, not apex name.
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
