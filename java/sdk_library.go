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
	"android/soong/genrule"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

var (
	sdkStubsLibrarySuffix = ".stubs"
	sdkSystemApiSuffix    = ".system"
	sdkTestApiSuffix      = ".test"
	sdkDocsSuffix         = ".docs"
	sdkXmlFileSuffix      = ".xml"
)

type stubsLibraryDependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

type syspropLibraryInterface interface {
	SyspropJavaModule() *SdkLibrary
}

var (
	publicApiStubsTag = dependencyTag{name: "public"}
	systemApiStubsTag = dependencyTag{name: "system"}
	testApiStubsTag   = dependencyTag{name: "test"}
	publicApiFileTag  = dependencyTag{name: "publicApi"}
	systemApiFileTag  = dependencyTag{name: "systemApi"}
	testApiFileTag    = dependencyTag{name: "testApi"}
)

type apiScope int

const (
	apiScopePublic apiScope = iota
	apiScopeSystem
	apiScopeTest
)

var (
	javaSdkLibrariesLock sync.Mutex
)

// java_sdk_library is to make a Java library that implements optional platform APIs to apps.
// It is actually a wrapper of several modules: 1) stubs library that clients are linked against
// to, 2) droiddoc module that internally generates API stubs source files, 3) the real runtime
// shared library that implements the APIs, and 4) XML file for adding the runtime lib to the
// classpath at runtime if requested via <uses-library>.
//
// TODO: these are big features that are currently missing
// 1) disallowing linking to the runtime shared lib
// 2) HTML generation

func init() {
	android.RegisterModuleType("java_sdk_library", SdkLibraryFactory)

	android.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("java_sdk_library", SdkLibraryMutator).Parallel()
	})

	android.RegisterMakeVarsProvider(pctx, func(ctx android.MakeVarsContext) {
		javaSdkLibraries := javaSdkLibraries(ctx.Config())
		sort.Strings(*javaSdkLibraries)
		ctx.Strict("JAVA_SDK_LIBRARIES", strings.Join(*javaSdkLibraries, " "))
	})
}

type sdkLibraryProperties struct {
	// list of optional source files that are part of API but not part of runtime library.
	Api_srcs []string `android:"arch_variant"`

	// List of Java libraries that will be in the classpath when building stubs
	Stub_only_libs []string `android:"arch_variant"`

	// list of package names that will be documented and publicized as API
	Api_packages []string

	// list of package names that must be hidden from the API
	Hidden_api_packages []string

	// local files that are used within user customized droiddoc options.
	Droiddoc_option_files []string

	// additional droiddoc options
	// Available variables for substitution:
	//
	//  $(location <label>): the path to the droiddoc_option_files with name <label>
	Droiddoc_options []string

	// the java library (in classpath) for documentation that provides java srcs and srcjars.
	Srcs_lib *string

	// the base dirs under srcs_lib will be scanned for java srcs.
	Srcs_lib_whitelist_dirs []string

	// the sub dirs under srcs_lib_whitelist_dirs will be scanned for java srcs.
	// Defaults to "android.annotation".
	Srcs_lib_whitelist_pkgs []string

	// a list of top-level directories containing files to merge qualifier annotations
	// (i.e. those intended to be included in the stubs written) from.
	Merge_annotations_dirs []string

	// a list of top-level directories containing Java stub files to merge show/hide annotations from.
	Merge_inclusion_annotations_dirs []string

	// If set to true, the path of dist files is apistubs/core. Defaults to false.
	Core_lib *bool

	// don't create dist rules.
	No_dist *bool `blueprint:"mutated"`

	// TODO: determines whether to create HTML doc or not
	//Html_doc *bool
}

type SdkLibrary struct {
	Library

	sdkLibraryProperties sdkLibraryProperties

	publicApiStubsPath android.Paths
	systemApiStubsPath android.Paths
	testApiStubsPath   android.Paths

	publicApiStubsImplPath android.Paths
	systemApiStubsImplPath android.Paths
	testApiStubsImplPath   android.Paths

	publicApiFilePath android.Path
	systemApiFilePath android.Path
	testApiFilePath   android.Path
}

var _ Dependency = (*SdkLibrary)(nil)
var _ SdkLibraryDependency = (*SdkLibrary)(nil)

func (module *SdkLibrary) DepsMutator(ctx android.BottomUpMutatorContext) {
	useBuiltStubs := !ctx.Config().UnbundledBuildUsePrebuiltSdks()
	// Add dependencies to the stubs library
	if useBuiltStubs {
		ctx.AddVariationDependencies(nil, publicApiStubsTag, module.stubsName(apiScopePublic))
	}
	ctx.AddVariationDependencies(nil, publicApiFileTag, module.docsName(apiScopePublic))

	if !Bool(module.properties.No_standard_libs) {
		if useBuiltStubs {
			ctx.AddVariationDependencies(nil, systemApiStubsTag, module.stubsName(apiScopeSystem))
			ctx.AddVariationDependencies(nil, testApiStubsTag, module.stubsName(apiScopeTest))
		}
		ctx.AddVariationDependencies(nil, systemApiFileTag, module.docsName(apiScopeSystem))
		ctx.AddVariationDependencies(nil, testApiFileTag, module.docsName(apiScopeTest))
	}

	module.Library.deps(ctx)
}

func (module *SdkLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	module.Library.GenerateAndroidBuildActions(ctx)

	// Record the paths to the header jars of the library (stubs and impl).
	// When this java_sdk_library is dependened from others via "libs" property,
	// the recorded paths will be returned depending on the link type of the caller.
	ctx.VisitDirectDeps(func(to android.Module) {
		otherName := ctx.OtherModuleName(to)
		tag := ctx.OtherModuleDependencyTag(to)

		if lib, ok := to.(Dependency); ok {
			switch tag {
			case publicApiStubsTag:
				module.publicApiStubsPath = lib.HeaderJars()
				module.publicApiStubsImplPath = lib.ImplementationJars()
			case systemApiStubsTag:
				module.systemApiStubsPath = lib.HeaderJars()
				module.systemApiStubsImplPath = lib.ImplementationJars()
			case testApiStubsTag:
				module.testApiStubsPath = lib.HeaderJars()
				module.testApiStubsImplPath = lib.ImplementationJars()
			}
		}
		if doc, ok := to.(ApiFilePath); ok {
			switch tag {
			case publicApiFileTag:
				module.publicApiFilePath = doc.ApiFilePath()
			case systemApiFileTag:
				module.systemApiFilePath = doc.ApiFilePath()
			case testApiFileTag:
				module.testApiFilePath = doc.ApiFilePath()
			default:
				ctx.ModuleErrorf("depends on module %q of unknown tag %q", otherName, tag)
			}
		}
	})
}

func (module *SdkLibrary) AndroidMk() android.AndroidMkData {
	data := module.Library.AndroidMk()
	data.Required = append(data.Required, module.xmlFileName())

	data.Custom = func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
		android.WriteAndroidMkData(w, data)

		module.Library.AndroidMkHostDex(w, name, data)
		if !Bool(module.sdkLibraryProperties.No_dist) {
			// Create a phony module that installs the impl library, for the case when this lib is
			// in PRODUCT_PACKAGES.
			owner := module.ModuleBase.Owner()
			if owner == "" {
				if Bool(module.sdkLibraryProperties.Core_lib) {
					owner = "core"
				} else {
					owner = "android"
				}
			}
			// Create dist rules to install the stubs libs to the dist dir
			if len(module.publicApiStubsPath) == 1 {
				fmt.Fprintln(w, "$(call dist-for-goals,sdk win_sdk,"+
					module.publicApiStubsImplPath.Strings()[0]+
					":"+path.Join("apistubs", owner, "public",
					module.BaseModuleName()+".jar")+")")
			}
			if len(module.systemApiStubsPath) == 1 {
				fmt.Fprintln(w, "$(call dist-for-goals,sdk win_sdk,"+
					module.systemApiStubsImplPath.Strings()[0]+
					":"+path.Join("apistubs", owner, "system",
					module.BaseModuleName()+".jar")+")")
			}
			if len(module.testApiStubsPath) == 1 {
				fmt.Fprintln(w, "$(call dist-for-goals,sdk win_sdk,"+
					module.testApiStubsImplPath.Strings()[0]+
					":"+path.Join("apistubs", owner, "test",
					module.BaseModuleName()+".jar")+")")
			}
			if module.publicApiFilePath != nil {
				fmt.Fprintln(w, "$(call dist-for-goals,sdk win_sdk,"+
					module.publicApiFilePath.String()+
					":"+path.Join("apistubs", owner, "public", "api",
					module.BaseModuleName()+".txt")+")")
			}
			if module.systemApiFilePath != nil {
				fmt.Fprintln(w, "$(call dist-for-goals,sdk win_sdk,"+
					module.systemApiFilePath.String()+
					":"+path.Join("apistubs", owner, "system", "api",
					module.BaseModuleName()+".txt")+")")
			}
			if module.testApiFilePath != nil {
				fmt.Fprintln(w, "$(call dist-for-goals,sdk win_sdk,"+
					module.testApiFilePath.String()+
					":"+path.Join("apistubs", owner, "test", "api",
					module.BaseModuleName()+".txt")+")")
			}
		}
	}
	return data
}

// Module name of the stubs library
func (module *SdkLibrary) stubsName(apiScope apiScope) string {
	stubsName := module.BaseModuleName() + sdkStubsLibrarySuffix
	switch apiScope {
	case apiScopeSystem:
		stubsName = stubsName + sdkSystemApiSuffix
	case apiScopeTest:
		stubsName = stubsName + sdkTestApiSuffix
	}
	return stubsName
}

// Module name of the docs
func (module *SdkLibrary) docsName(apiScope apiScope) string {
	docsName := module.BaseModuleName() + sdkDocsSuffix
	switch apiScope {
	case apiScopeSystem:
		docsName = docsName + sdkSystemApiSuffix
	case apiScopeTest:
		docsName = docsName + sdkTestApiSuffix
	}
	return docsName
}

// Module name of the runtime implementation library
func (module *SdkLibrary) implName() string {
	return module.BaseModuleName()
}

// File path to the runtime implementation library
func (module *SdkLibrary) implPath() string {
	partition := "system"
	if module.SocSpecific() {
		partition = "vendor"
	} else if module.DeviceSpecific() {
		partition = "odm"
	} else if module.ProductSpecific() {
		partition = "product"
	}
	return "/" + partition + "/framework/" + module.implName() + ".jar"
}

// Module name of the XML file for the lib
func (module *SdkLibrary) xmlFileName() string {
	return module.BaseModuleName() + sdkXmlFileSuffix
}

// SDK version that the stubs library is built against. Note that this is always
// *current. Older stubs library built with a numberd SDK version is created from
// the prebuilt jar.
func (module *SdkLibrary) sdkVersion(apiScope apiScope) string {
	switch apiScope {
	case apiScopePublic:
		return "current"
	case apiScopeSystem:
		return "system_current"
	case apiScopeTest:
		return "test_current"
	default:
		return "current"
	}
}

// $(INTERNAL_PLATFORM_<apiTagName>_API_FILE) points to the generated
// api file for the current source
// TODO: remove this when apicheck is done in soong
func (module *SdkLibrary) apiTagName(apiScope apiScope) string {
	apiTagName := strings.Replace(strings.ToUpper(module.BaseModuleName()), ".", "_", -1)
	switch apiScope {
	case apiScopeSystem:
		apiTagName = apiTagName + "_SYSTEM"
	case apiScopeTest:
		apiTagName = apiTagName + "_TEST"
	}
	return apiTagName
}

func (module *SdkLibrary) latestApiFilegroupName(apiScope apiScope) string {
	name := ":" + module.BaseModuleName() + ".api."
	switch apiScope {
	case apiScopePublic:
		name = name + "public"
	case apiScopeSystem:
		name = name + "system"
	case apiScopeTest:
		name = name + "test"
	}
	name = name + ".latest"
	return name
}

func (module *SdkLibrary) latestRemovedApiFilegroupName(apiScope apiScope) string {
	name := ":" + module.BaseModuleName() + "-removed.api."
	switch apiScope {
	case apiScopePublic:
		name = name + "public"
	case apiScopeSystem:
		name = name + "system"
	case apiScopeTest:
		name = name + "test"
	}
	name = name + ".latest"
	return name
}

// Creates a static java library that has API stubs
func (module *SdkLibrary) createStubsLibrary(mctx android.TopDownMutatorContext, apiScope apiScope) {
	props := struct {
		Name              *string
		Srcs              []string
		Sdk_version       *string
		Libs              []string
		Soc_specific      *bool
		Device_specific   *bool
		Product_specific  *bool
		Compile_dex       *bool
		No_standard_libs  *bool
		System_modules    *string
		Java_version      *string
		Product_variables struct {
			Unbundled_build struct {
				Enabled *bool
			}
			Pdk struct {
				Enabled *bool
			}
		}
		Openjdk9 struct {
			Srcs       []string
			Javacflags []string
		}
	}{}

	props.Name = proptools.StringPtr(module.stubsName(apiScope))
	// sources are generated from the droiddoc
	props.Srcs = []string{":" + module.docsName(apiScope)}
	props.Sdk_version = proptools.StringPtr(module.sdkVersion(apiScope))
	props.Libs = module.sdkLibraryProperties.Stub_only_libs
	// Unbundled apps will use the prebult one from /prebuilts/sdk
	if mctx.Config().UnbundledBuildUsePrebuiltSdks() {
		props.Product_variables.Unbundled_build.Enabled = proptools.BoolPtr(false)
	}
	props.Product_variables.Pdk.Enabled = proptools.BoolPtr(false)
	props.No_standard_libs = module.Library.Module.properties.No_standard_libs
	props.System_modules = module.Library.Module.deviceProperties.System_modules
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
	}

	mctx.CreateModule(android.ModuleFactoryAdaptor(LibraryFactory), &props)
}

// Creates a droiddoc module that creates stubs source files from the given full source
// files
func (module *SdkLibrary) createDocs(mctx android.TopDownMutatorContext, apiScope apiScope) {
	props := struct {
		Name                             *string
		Srcs                             []string
		Installable                      *bool
		Srcs_lib                         *string
		Srcs_lib_whitelist_dirs          []string
		Srcs_lib_whitelist_pkgs          []string
		Libs                             []string
		Arg_files                        []string
		Args                             *string
		Api_tag_name                     *string
		Api_filename                     *string
		Removed_api_filename             *string
		No_standard_libs                 *bool
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
	}{}

	props.Name = proptools.StringPtr(module.docsName(apiScope))
	props.Srcs = append(props.Srcs, module.Library.Module.properties.Srcs...)
	props.Srcs = append(props.Srcs, module.sdkLibraryProperties.Api_srcs...)
	props.Installable = proptools.BoolPtr(false)
	// A droiddoc module has only one Libs property and doesn't distinguish between
	// shared libs and static libs. So we need to add both of these libs to Libs property.
	props.Libs = module.Library.Module.properties.Libs
	props.Libs = append(props.Libs, module.Library.Module.properties.Static_libs...)
	props.Aidl.Include_dirs = module.Library.Module.deviceProperties.Aidl.Include_dirs
	props.Aidl.Local_include_dirs = module.Library.Module.deviceProperties.Aidl.Local_include_dirs
	props.No_standard_libs = module.Library.Module.properties.No_standard_libs
	props.Java_version = module.Library.Module.properties.Java_version

	props.Merge_annotations_dirs = module.sdkLibraryProperties.Merge_annotations_dirs
	props.Merge_inclusion_annotations_dirs = module.sdkLibraryProperties.Merge_inclusion_annotations_dirs

	droiddocArgs := " --stub-packages " + strings.Join(module.sdkLibraryProperties.Api_packages, ":") +
		" " + android.JoinWithPrefix(module.sdkLibraryProperties.Hidden_api_packages, " --hide-package ") +
		" " + android.JoinWithPrefix(module.sdkLibraryProperties.Droiddoc_options, " ") +
		" --hide MissingPermission --hide BroadcastBehavior " +
		"--hide HiddenSuperclass --hide DeprecationMismatch --hide UnavailableSymbol " +
		"--hide SdkConstant --hide HiddenTypeParameter --hide Todo --hide Typo"

	switch apiScope {
	case apiScopeSystem:
		droiddocArgs = droiddocArgs + " -showAnnotation android.annotation.SystemApi"
	case apiScopeTest:
		droiddocArgs = droiddocArgs + " -showAnnotation android.annotation.TestApi"
	}
	props.Arg_files = module.sdkLibraryProperties.Droiddoc_option_files
	props.Args = proptools.StringPtr(droiddocArgs)

	// List of APIs identified from the provided source files are created. They are later
	// compared against to the not-yet-released (a.k.a current) list of APIs and to the
	// last-released (a.k.a numbered) list of API.
	currentApiFileName := "current.txt"
	removedApiFileName := "removed.txt"
	switch apiScope {
	case apiScopeSystem:
		currentApiFileName = "system-" + currentApiFileName
		removedApiFileName = "system-" + removedApiFileName
	case apiScopeTest:
		currentApiFileName = "test-" + currentApiFileName
		removedApiFileName = "test-" + removedApiFileName
	}
	currentApiFileName = path.Join("api", currentApiFileName)
	removedApiFileName = path.Join("api", removedApiFileName)
	// TODO(jiyong): remove these three props
	props.Api_tag_name = proptools.StringPtr(module.apiTagName(apiScope))
	props.Api_filename = proptools.StringPtr(currentApiFileName)
	props.Removed_api_filename = proptools.StringPtr(removedApiFileName)

	// check against the not-yet-release API
	props.Check_api.Current.Api_file = proptools.StringPtr(currentApiFileName)
	props.Check_api.Current.Removed_api_file = proptools.StringPtr(removedApiFileName)

	// check against the latest released API
	props.Check_api.Last_released.Api_file = proptools.StringPtr(
		module.latestApiFilegroupName(apiScope))
	props.Check_api.Last_released.Removed_api_file = proptools.StringPtr(
		module.latestRemovedApiFilegroupName(apiScope))
	props.Check_api.Ignore_missing_latest_api = proptools.BoolPtr(true)
	props.Srcs_lib = module.sdkLibraryProperties.Srcs_lib
	props.Srcs_lib_whitelist_dirs = module.sdkLibraryProperties.Srcs_lib_whitelist_dirs
	props.Srcs_lib_whitelist_pkgs = module.sdkLibraryProperties.Srcs_lib_whitelist_pkgs

	mctx.CreateModule(android.ModuleFactoryAdaptor(DroidstubsFactory), &props)
}

// Creates the xml file that publicizes the runtime library
func (module *SdkLibrary) createXmlFile(mctx android.TopDownMutatorContext) {
	template := `
<?xml version="1.0" encoding="utf-8"?>
<!-- Copyright (C) 2018 The Android Open Source Project

     Licensed under the Apache License, Version 2.0 (the "License");
     you may not use this file except in compliance with the License.
     You may obtain a copy of the License at
  
          http://www.apache.org/licenses/LICENSE-2.0
  
     Unless required by applicable law or agreed to in writing, software
     distributed under the License is distributed on an "AS IS" BASIS,
     WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
     See the License for the specific language governing permissions and
     limitations under the License.
-->

<permissions>
    <library name="%s" file="%s"/>
</permissions>
`
	// genrule to generate the xml file content from the template above
	// TODO: preserve newlines in the generate xml file. Newlines are being squashed
	// in the ninja file. Do we need to have an external tool for this?
	xmlContent := fmt.Sprintf(template, module.BaseModuleName(), module.implPath())
	genruleProps := struct {
		Name *string
		Cmd  *string
		Out  []string
	}{}
	genruleProps.Name = proptools.StringPtr(module.xmlFileName() + "-gen")
	genruleProps.Cmd = proptools.StringPtr("echo '" + xmlContent + "' > $(out)")
	genruleProps.Out = []string{module.xmlFileName()}
	mctx.CreateModule(android.ModuleFactoryAdaptor(genrule.GenRuleFactory), &genruleProps)

	// creates a prebuilt_etc module to actually place the xml file under
	// <partition>/etc/permissions
	etcProps := struct {
		Name             *string
		Src              *string
		Sub_dir          *string
		Soc_specific     *bool
		Device_specific  *bool
		Product_specific *bool
	}{}
	etcProps.Name = proptools.StringPtr(module.xmlFileName())
	etcProps.Src = proptools.StringPtr(":" + module.xmlFileName() + "-gen")
	etcProps.Sub_dir = proptools.StringPtr("permissions")
	if module.SocSpecific() {
		etcProps.Soc_specific = proptools.BoolPtr(true)
	} else if module.DeviceSpecific() {
		etcProps.Device_specific = proptools.BoolPtr(true)
	} else if module.ProductSpecific() {
		etcProps.Product_specific = proptools.BoolPtr(true)
	}
	mctx.CreateModule(android.ModuleFactoryAdaptor(android.PrebuiltEtcFactory), &etcProps)
}

func (module *SdkLibrary) PrebuiltJars(ctx android.BaseContext, sdkVersion string) android.Paths {
	var api, v string
	if sdkVersion == "" {
		api = "system"
		v = "current"
	} else if strings.Contains(sdkVersion, "_") {
		t := strings.Split(sdkVersion, "_")
		api = t[0]
		v = t[1]
	} else {
		api = "public"
		v = sdkVersion
	}
	dir := filepath.Join("prebuilts", "sdk", v, api)
	jar := filepath.Join(dir, module.BaseModuleName()+".jar")
	jarPath := android.ExistentPathForSource(ctx, jar)
	if !jarPath.Valid() {
		ctx.PropertyErrorf("sdk_library", "invalid sdk version %q, %q does not exist", v, jar)
		return nil
	}
	return android.Paths{jarPath.Path()}
}

// to satisfy SdkLibraryDependency interface
func (module *SdkLibrary) SdkHeaderJars(ctx android.BaseContext, sdkVersion string) android.Paths {
	// This module is just a wrapper for the stubs.
	if ctx.Config().UnbundledBuildUsePrebuiltSdks() {
		return module.PrebuiltJars(ctx, sdkVersion)
	} else {
		if strings.HasPrefix(sdkVersion, "system_") {
			return module.systemApiStubsPath
		} else if sdkVersion == "" {
			return module.Library.HeaderJars()
		} else {
			return module.publicApiStubsPath
		}
	}
}

// to satisfy SdkLibraryDependency interface
func (module *SdkLibrary) SdkImplementationJars(ctx android.BaseContext, sdkVersion string) android.Paths {
	// This module is just a wrapper for the stubs.
	if ctx.Config().UnbundledBuildUsePrebuiltSdks() {
		return module.PrebuiltJars(ctx, sdkVersion)
	} else {
		if strings.HasPrefix(sdkVersion, "system_") {
			return module.systemApiStubsImplPath
		} else if sdkVersion == "" {
			return module.Library.ImplementationJars()
		} else {
			return module.publicApiStubsImplPath
		}
	}
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

// For a java_sdk_library module, create internal modules for stubs, docs,
// runtime libs and xml file. If requested, the stubs and docs are created twice
// once for public API level and once for system API level
func SdkLibraryMutator(mctx android.TopDownMutatorContext) {
	if module, ok := mctx.Module().(*SdkLibrary); ok {
		module.createInternalModules(mctx)
	} else if module, ok := mctx.Module().(syspropLibraryInterface); ok {
		module.SyspropJavaModule().createInternalModules(mctx)
	}
}

func (module *SdkLibrary) createInternalModules(mctx android.TopDownMutatorContext) {
	if len(module.Library.Module.properties.Srcs) == 0 {
		mctx.PropertyErrorf("srcs", "java_sdk_library must specify srcs")
	}

	if len(module.sdkLibraryProperties.Api_packages) == 0 {
		mctx.PropertyErrorf("api_packages", "java_sdk_library must specify api_packages")
	}

	missing_current_api := false

	for _, scope := range []string{"", "system-", "test-"} {
		for _, api := range []string{"current.txt", "removed.txt"} {
			path := path.Join(mctx.ModuleDir(), "api", scope+api)
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
			"%s %q && m update-api", script, mctx.ModuleDir())
		return
	}

	// for public API stubs
	module.createStubsLibrary(mctx, apiScopePublic)
	module.createDocs(mctx, apiScopePublic)

	if !Bool(module.properties.No_standard_libs) {
		// for system API stubs
		module.createStubsLibrary(mctx, apiScopeSystem)
		module.createDocs(mctx, apiScopeSystem)

		// for test API stubs
		module.createStubsLibrary(mctx, apiScopeTest)
		module.createDocs(mctx, apiScopeTest)

		// for runtime
		module.createXmlFile(mctx)
	}

	// record java_sdk_library modules so that they are exported to make
	javaSdkLibraries := javaSdkLibraries(mctx.Config())
	javaSdkLibrariesLock.Lock()
	defer javaSdkLibrariesLock.Unlock()
	*javaSdkLibraries = append(*javaSdkLibraries, module.BaseModuleName())
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

func SdkLibraryFactory() android.Module {
	module := &SdkLibrary{}
	module.InitSdkLibraryProperties()
	InitJavaModule(module, android.HostAndDeviceSupported)
	return module
}
