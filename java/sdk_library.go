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
	sdkImplLibrarySuffix  = ".impl"
	sdkXmlFileSuffix      = ".xml"
)

type stubsLibraryDependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var (
	publicApiStubsTag = dependencyTag{name: "public"}
	systemApiStubsTag = dependencyTag{name: "system"}
	testApiStubsTag   = dependencyTag{name: "test"}
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
	android.RegisterModuleType("java_sdk_library", sdkLibraryFactory)

	android.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("java_sdk_library", sdkLibraryMutator).Parallel()
	})

	android.RegisterMakeVarsProvider(pctx, func(ctx android.MakeVarsContext) {
		javaSdkLibraries := javaSdkLibraries(ctx.Config())
		sort.Strings(*javaSdkLibraries)
		ctx.Strict("JAVA_SDK_LIBRARIES", strings.Join(*javaSdkLibraries, " "))
	})
}

type sdkLibraryProperties struct {
	// list of source files used to compile the Java module.  May be .java, .logtags, .proto,
	// or .aidl files.
	Srcs []string `android:"arch_variant"`

	// list of optional source files that are part of API but not part of runtime library.
	Api_srcs []string `android:"arch_variant"`

	// list of of java libraries that will be in the classpath
	Libs []string `android:"arch_variant"`

	// list of java libraries that will be compiled into the resulting runtime jar.
	// These libraries are not compiled into the stubs jar.
	Static_libs []string `android:"arch_variant"`

	// List of Java libraries that will be in the classpath when building stubs
	Stub_only_libs []string `android:"arch_variant"`

	// list of package names that will be documented and publicized as API
	Api_packages []string

	// list of package names that must be hidden from the API
	Hidden_api_packages []string

	Errorprone struct {
		// List of javac flags that should only be used when running errorprone.
		Javacflags []string
	}

	// TODO: determines whether to create HTML doc or not
	//Html_doc *bool
}

type sdkLibrary struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties       sdkLibraryProperties
	deviceProperties CompilerDeviceProperties

	publicApiStubsPath android.Paths
	systemApiStubsPath android.Paths
	testApiStubsPath   android.Paths
}

func (module *sdkLibrary) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies to the stubs library
	ctx.AddDependency(ctx.Module(), publicApiStubsTag, module.stubsName(apiScopePublic))
	ctx.AddDependency(ctx.Module(), systemApiStubsTag, module.stubsName(apiScopeSystem))
	ctx.AddDependency(ctx.Module(), testApiStubsTag, module.stubsName(apiScopeTest))
}

func (module *sdkLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Record the paths to the header jars of the stubs library.
	// When this java_sdk_library is dependened from others via "libs" property,
	// the recorded paths will be returned depending on the link type of the caller.
	ctx.VisitDirectDeps(func(to android.Module) {
		otherName := ctx.OtherModuleName(to)
		tag := ctx.OtherModuleDependencyTag(to)

		if stubs, ok := to.(Dependency); ok {
			switch tag {
			case publicApiStubsTag:
				module.publicApiStubsPath = stubs.HeaderJars()
			case systemApiStubsTag:
				module.systemApiStubsPath = stubs.HeaderJars()
			case testApiStubsTag:
				module.testApiStubsPath = stubs.HeaderJars()
			default:
				ctx.ModuleErrorf("depends on module %q of unknown tag %q", otherName, tag)
			}
		}
	})
}

func (module *sdkLibrary) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			// Create a phony module that installs the impl library, for the case when this lib is
			// in PRODUCT_PACKAGES.
			fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
			fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
			fmt.Fprintln(w, "LOCAL_MODULE :=", name)
			fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES := "+module.implName())
			fmt.Fprintln(w, "include $(BUILD_PHONY_PACKAGE)")
			// Create dist rules to install the stubs libs to the dist dir
			if len(module.publicApiStubsPath) == 1 {
				fmt.Fprintln(w, "$(call dist-for-goals,sdk win_sdk,"+
					module.publicApiStubsPath.Strings()[0]+
					":"+path.Join("apistubs", "public", module.BaseModuleName()+".jar")+")")
			}
			if len(module.systemApiStubsPath) == 1 {
				fmt.Fprintln(w, "$(call dist-for-goals,sdk win_sdk,"+
					module.systemApiStubsPath.Strings()[0]+
					":"+path.Join("apistubs", "system", module.BaseModuleName()+".jar")+")")
			}
			if len(module.testApiStubsPath) == 1 {
				fmt.Fprintln(w, "$(call dist-for-goals,sdk win_sdk,"+
					module.testApiStubsPath.Strings()[0]+
					":"+path.Join("apistubs", "test", module.BaseModuleName()+".jar")+")")
			}
		},
	}
}

// Module name of the stubs library
func (module *sdkLibrary) stubsName(apiScope apiScope) string {
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
func (module *sdkLibrary) docsName(apiScope apiScope) string {
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
func (module *sdkLibrary) implName() string {
	return module.BaseModuleName() + sdkImplLibrarySuffix
}

// File path to the runtime implementation library
func (module *sdkLibrary) implPath() string {
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
func (module *sdkLibrary) xmlFileName() string {
	return module.BaseModuleName() + sdkXmlFileSuffix
}

// SDK version that the stubs library is built against. Note that this is always
// *current. Older stubs library built with a numberd SDK version is created from
// the prebuilt jar.
func (module *sdkLibrary) sdkVersion(apiScope apiScope) string {
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
func (module *sdkLibrary) apiTagName(apiScope apiScope) string {
	apiTagName := strings.Replace(strings.ToUpper(module.BaseModuleName()), ".", "_", -1)
	switch apiScope {
	case apiScopeSystem:
		apiTagName = apiTagName + "_SYSTEM"
	case apiScopeTest:
		apiTagName = apiTagName + "_TEST"
	}
	return apiTagName
}

func (module *sdkLibrary) latestApiFilegroupName(apiScope apiScope) string {
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

func (module *sdkLibrary) latestRemovedApiFilegroupName(apiScope apiScope) string {
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
func (module *sdkLibrary) createStubsLibrary(mctx android.TopDownMutatorContext, apiScope apiScope) {
	props := struct {
		Name              *string
		Srcs              []string
		Sdk_version       *string
		Libs              []string
		Soc_specific      *bool
		Device_specific   *bool
		Product_specific  *bool
		Product_variables struct {
			Unbundled_build struct {
				Enabled *bool
			}
			Pdk struct {
				Enabled *bool
			}
		}
	}{}

	props.Name = proptools.StringPtr(module.stubsName(apiScope))
	// sources are generated from the droiddoc
	props.Srcs = []string{":" + module.docsName(apiScope)}
	props.Sdk_version = proptools.StringPtr(module.sdkVersion(apiScope))
	props.Libs = module.properties.Stub_only_libs
	// Unbundled apps will use the prebult one from /prebuilts/sdk
	props.Product_variables.Unbundled_build.Enabled = proptools.BoolPtr(false)
	props.Product_variables.Pdk.Enabled = proptools.BoolPtr(false)

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
func (module *sdkLibrary) createDocs(mctx android.TopDownMutatorContext, apiScope apiScope) {
	props := struct {
		Name                    *string
		Srcs                    []string
		Custom_template         *string
		Installable             *bool
		Srcs_lib                *string
		Srcs_lib_whitelist_dirs []string
		Srcs_lib_whitelist_pkgs []string
		Libs                    []string
		Args                    *string
		Api_tag_name            *string
		Api_filename            *string
		Removed_api_filename    *string
		Check_api               struct {
			Current       ApiToCheck
			Last_released ApiToCheck
		}
		Aidl struct {
			Include_dirs       []string
			Local_include_dirs []string
		}
	}{}

	props.Name = proptools.StringPtr(module.docsName(apiScope))
	props.Srcs = append(props.Srcs, module.properties.Srcs...)
	props.Srcs = append(props.Srcs, module.properties.Api_srcs...)
	props.Custom_template = proptools.StringPtr("droiddoc-templates-sdk")
	props.Installable = proptools.BoolPtr(false)
	// A droiddoc module has only one Libs property and doesn't distinguish between
	// shared libs and static libs. So we need to add both of these libs to Libs property.
	props.Libs = module.properties.Libs
	props.Libs = append(props.Libs, module.properties.Static_libs...)
	props.Aidl.Include_dirs = module.deviceProperties.Aidl.Include_dirs
	props.Aidl.Local_include_dirs = module.deviceProperties.Aidl.Local_include_dirs

	droiddocArgs := " -hide 110 -hide 111 -hide 113 -hide 121 -hide 125 -hide 126 -hide 127 -hide 128" +
		" -stubpackages " + strings.Join(module.properties.Api_packages, ":") +
		" " + android.JoinWithPrefix(module.properties.Hidden_api_packages, "-hidePackage ") +
		" -nodocs"
	switch apiScope {
	case apiScopeSystem:
		droiddocArgs = droiddocArgs + " -showAnnotation android.annotation.SystemApi"
	case apiScopeTest:
		droiddocArgs = droiddocArgs + " -showAnnotation android.annotation.TestApi"
	}
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
	// any change is reported as error
	props.Check_api.Current.Args = proptools.StringPtr("-error 2 -error 3 -error 4 -error 5 " +
		"-error 6 -error 7 -error 8 -error 9 -error 10 -error 11 -error 12 -error 13 " +
		"-error 14 -error 15 -error 16 -error 17 -error 18 -error 19 -error 20 " +
		"-error 21 -error 23 -error 24 -error 25 -error 26 -error 27")

	// check against the latest released API
	props.Check_api.Last_released.Api_file = proptools.StringPtr(
		module.latestApiFilegroupName(apiScope))
	props.Check_api.Last_released.Removed_api_file = proptools.StringPtr(
		module.latestRemovedApiFilegroupName(apiScope))
	// backward incompatible changes are reported as error
	props.Check_api.Last_released.Args = proptools.StringPtr("-hide 2 -hide 3 -hide 4 -hide 5 " +
		"-hide 6 -hide 24 -hide 25 -hide 26 -hide 27 " +
		"-error 7 -error 8 -error 9 -error 10 -error 11 -error 12 -error 13 -error 14 " +
		"-error 15 -error 16 -error 17 -error 18")

	// Include the part of the framework source. This is required for the case when
	// API class is extending from the framework class. In that case, doclava needs
	// to know whether the base class is hidden or not. Since that information is
	// encoded as @hide string in the comment, we need source files for the classes,
	// not the compiled ones.
	props.Srcs_lib = proptools.StringPtr("framework")
	props.Srcs_lib_whitelist_dirs = []string{"core/java"}
	// Add android.annotation package to give access to the framework-defined
	// annotations such as SystemApi, NonNull, etc.
	props.Srcs_lib_whitelist_pkgs = []string{"android.annotation"}
	// These libs are required by doclava to parse the framework sources add via
	// Src_lib and Src_lib_whitelist_* properties just above.
	// If we don't add them to the classpath, errors messages are generated by doclava,
	// though they don't break the build.
	props.Libs = append(props.Libs, "conscrypt", "bouncycastle", "okhttp", "framework")

	mctx.CreateModule(android.ModuleFactoryAdaptor(DroiddocFactory), &props)
}

// Creates the runtime library. This is not directly linkable from other modules.
func (module *sdkLibrary) createImplLibrary(mctx android.TopDownMutatorContext) {
	props := struct {
		Name             *string
		Srcs             []string
		Libs             []string
		Static_libs      []string
		Soc_specific     *bool
		Device_specific  *bool
		Product_specific *bool
		Installable      *bool
		Required         []string
		Errorprone       struct {
			Javacflags []string
		}
	}{}

	props.Name = proptools.StringPtr(module.implName())
	props.Srcs = module.properties.Srcs
	props.Libs = module.properties.Libs
	props.Static_libs = module.properties.Static_libs
	props.Installable = proptools.BoolPtr(true)
	// XML file is installed along with the impl lib
	props.Required = []string{module.xmlFileName()}
	props.Errorprone.Javacflags = module.properties.Errorprone.Javacflags

	if module.SocSpecific() {
		props.Soc_specific = proptools.BoolPtr(true)
	} else if module.DeviceSpecific() {
		props.Device_specific = proptools.BoolPtr(true)
	} else if module.ProductSpecific() {
		props.Product_specific = proptools.BoolPtr(true)
	}

	mctx.CreateModule(android.ModuleFactoryAdaptor(LibraryFactory), &props, &module.deviceProperties)
}

// Creates the xml file that publicizes the runtime library
func (module *sdkLibrary) createXmlFile(mctx android.TopDownMutatorContext) {
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

// to satisfy SdkLibraryDependency interface
func (module *sdkLibrary) HeaderJars(linkType linkType) android.Paths {
	// This module is just a wrapper for the stubs.
	if linkType == javaSystem || linkType == javaPlatform {
		return module.systemApiStubsPath
	} else {
		return module.publicApiStubsPath
	}
}

func javaSdkLibraries(config android.Config) *[]string {
	return config.Once("javaSdkLibraries", func() interface{} {
		return &[]string{}
	}).(*[]string)
}

// For a java_sdk_library module, create internal modules for stubs, docs,
// runtime libs and xml file. If requested, the stubs and docs are created twice
// once for public API level and once for system API level
func sdkLibraryMutator(mctx android.TopDownMutatorContext) {
	if module, ok := mctx.Module().(*sdkLibrary); ok {
		if module.properties.Srcs == nil {
			mctx.PropertyErrorf("srcs", "java_sdk_library must specify srcs")
		}
		if module.properties.Api_packages == nil {
			mctx.PropertyErrorf("api_packages", "java_sdk_library must specify api_packages")
		}

		// for public API stubs
		module.createStubsLibrary(mctx, apiScopePublic)
		module.createDocs(mctx, apiScopePublic)

		// for system API stubs
		module.createStubsLibrary(mctx, apiScopeSystem)
		module.createDocs(mctx, apiScopeSystem)

		// for test API stubs
		module.createStubsLibrary(mctx, apiScopeTest)
		module.createDocs(mctx, apiScopeTest)

		// for runtime
		module.createXmlFile(mctx)
		module.createImplLibrary(mctx)

		// record java_sdk_library modules so that they are exported to make
		javaSdkLibraries := javaSdkLibraries(mctx.Config())
		javaSdkLibrariesLock.Lock()
		defer javaSdkLibrariesLock.Unlock()
		*javaSdkLibraries = append(*javaSdkLibraries, module.BaseModuleName())
	}
}

func sdkLibraryFactory() android.Module {
	module := &sdkLibrary{}
	module.AddProperties(&module.properties)
	module.AddProperties(&module.deviceProperties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}
