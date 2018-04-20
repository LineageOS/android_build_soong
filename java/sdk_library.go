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
	"path"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

var (
	sdkStubsLibrarySuffix = ".stubs"
	sdkSystemApiSuffix    = ".system"
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
)

// java_sdk_library is to make a Java library that implements optional platform APIs to apps.
// It is actually a wrapper of several modules: 1) stubs library that clients are linked against
// to, 2) droiddoc module that internally generates API stubs source files, 3) the real runtime
// shared library that implements the APIs, and 4) XML file for adding the runtime lib to the
// classpath at runtime if requested via <uses-library>.
//
// TODO: these are big features that are currently missing
// 1) check for API consistency
// 2) install stubs libs as the dist artifacts
// 3) ensuring that apps have appropriate <uses-library> tag
// 4) disallowing linking to the runtime shared lib
// 5) HTML generation

func init() {
	android.RegisterModuleType("java_sdk_library", sdkLibraryFactory)

	android.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("java_sdk_library", sdkLibraryMutator).Parallel()
	})
}

type sdkLibraryProperties struct {
	// list of source files used to compile the Java module.  May be .java, .logtags, .proto,
	// or .aidl files.
	Srcs []string `android:"arch_variant"`

	// list of of java libraries that will be in the classpath
	Libs []string `android:"arch_variant"`

	// list of java libraries that will be compiled into the resulting runtime jar.
	// These libraries are not compiled into the stubs jar.
	Static_libs []string `android:"arch_variant"`

	// list of package names that will be documented and publicized as API
	Api_packages []string

	// TODO: determines whether to create HTML doc or not
	//Html_doc *bool
}

type sdkLibrary struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties sdkLibraryProperties

	publicApiStubsPath android.Paths
	systemApiStubsPath android.Paths
}

func (module *sdkLibrary) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies to the stubs library
	ctx.AddDependency(ctx.Module(), publicApiStubsTag, module.stubsName(false))
	ctx.AddDependency(ctx.Module(), systemApiStubsTag, module.stubsName(true))
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
			default:
				ctx.ModuleErrorf("depends on module %q of unknown tag %q", otherName, tag)
			}
		}
	})
}

// Module name of the stubs library
func (module *sdkLibrary) stubsName(forSystemApi bool) string {
	stubsName := module.BaseModuleName() + sdkStubsLibrarySuffix
	if forSystemApi {
		stubsName = stubsName + sdkSystemApiSuffix
	}
	return stubsName
}

// Module name of the docs
func (module *sdkLibrary) docsName(forSystemApi bool) string {
	docsName := module.BaseModuleName() + sdkDocsSuffix
	if forSystemApi {
		docsName = docsName + sdkSystemApiSuffix
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
func (module *sdkLibrary) sdkVersion(forSystemApi bool) string {
	if forSystemApi {
		return "system_current"
	} else {
		return "current"
	}
}

// $(INTERNAL_PLATFORM_<apiTagName>_API_FILE) points to the generated
// api file for the current source
// TODO: remove this when apicheck is done in soong
func (module *sdkLibrary) apiTagName(forSystemApi bool) string {
	apiTagName := strings.Replace(strings.ToUpper(module.BaseModuleName()), ".", "_", -1)
	if forSystemApi {
		apiTagName = apiTagName + "_SYSTEM"
	}
	return apiTagName
}

// returns the path (relative to this module) to the API txt file. Files are located
// ./<api_dir>/<api_level>.txt where <api_level> is either current, system-current, removed,
// or system-removed.
func (module *sdkLibrary) apiFilePath(apiLevel string, forSystemApi bool) string {
	apiDir := "api"
	apiFile := apiLevel
	if forSystemApi {
		apiFile = "system-" + apiFile
	}
	apiFile = apiFile + ".txt"

	return path.Join(apiDir, apiFile)
}

// Creates a static java library that has API stubs
func (module *sdkLibrary) createStubsLibrary(mctx android.TopDownMutatorContext, forSystemApi bool) {
	props := struct {
		Name              *string
		Srcs              []string
		Sdk_version       *string
		Soc_specific      *bool
		Device_specific   *bool
		Product_specific  *bool
		Product_variables struct {
			Unbundled_build struct {
				Enabled *bool
			}
		}
	}{}

	props.Name = proptools.StringPtr(module.stubsName(forSystemApi))
	// sources are generated from the droiddoc
	props.Srcs = []string{":" + module.docsName(forSystemApi)}
	props.Sdk_version = proptools.StringPtr(module.sdkVersion(forSystemApi))
	// Unbundled apps will use the prebult one from /prebuilts/sdk
	props.Product_variables.Unbundled_build.Enabled = proptools.BoolPtr(false)

	if module.SocSpecific() {
		props.Soc_specific = proptools.BoolPtr(true)
	} else if module.DeviceSpecific() {
		props.Device_specific = proptools.BoolPtr(true)
	} else if module.ProductSpecific() {
		props.Product_specific = proptools.BoolPtr(true)
	}

	mctx.CreateModule(android.ModuleFactoryAdaptor(LibraryFactory(false)), &props)
}

// Creates a droiddoc module that creates stubs source files from the given full source
// files
func (module *sdkLibrary) createDocs(mctx android.TopDownMutatorContext, forSystemApi bool) {
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
	}{}

	props.Name = proptools.StringPtr(module.docsName(forSystemApi))
	props.Srcs = module.properties.Srcs
	props.Custom_template = proptools.StringPtr("droiddoc-templates-sdk")
	props.Installable = proptools.BoolPtr(false)
	props.Libs = module.properties.Libs

	droiddocArgs := " -hide 110 -hide 111 -hide 113 -hide 121 -hide 125 -hide 126 -hide 127 -hide 128" +
		" -stubpackages " + strings.Join(module.properties.Api_packages, ":") +
		" -nodocs"
	if forSystemApi {
		droiddocArgs = droiddocArgs + " -showAnnotation android.annotation.SystemApi"
	}
	props.Args = proptools.StringPtr(droiddocArgs)

	// List of APIs identified from the provided source files are created. They are later
	// compared against to the not-yet-released (a.k.a current) list of APIs and to the
	// last-released (a.k.a numbered) list of API.
	// TODO: If any incompatible change is detected, break the build
	currentApiFileName := "current.txt"
	removedApiFileName := "removed.txt"
	if forSystemApi {
		currentApiFileName = "system-" + currentApiFileName
		removedApiFileName = "system-" + removedApiFileName
	}
	currentApiFileName = path.Join("api", currentApiFileName)
	removedApiFileName = path.Join("api", removedApiFileName)
	props.Api_tag_name = proptools.StringPtr(module.apiTagName(forSystemApi))
	// Note: the exact names of these two are not important because they are always
	// referenced by the make variable $(INTERNAL_PLATFORM_<TAG_NAME>_API_FILE)
	props.Api_filename = proptools.StringPtr(currentApiFileName)
	props.Removed_api_filename = proptools.StringPtr(removedApiFileName)

	// Includes the main framework source to ensure that doclava has access to the
	// visibility information for the base classes of the mock classes. Without it
	// otherwise hidden methods could be visible.
	// TODO: remove the need for this
	props.Srcs_lib = proptools.StringPtr("framework")
	props.Srcs_lib_whitelist_dirs = []string{"core/java"}
	props.Srcs_lib_whitelist_pkgs = []string{"android"}
	// These libs are required by doclava to parse the sources from framework.
	// If we don't add them to the classpath, errors messages are generated by doclava,
	// though they don't break the build.
	props.Libs = append(props.Libs, "conscrypt", "bouncycastle", "okhttp")

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
		Required         []string
	}{}

	props.Name = proptools.StringPtr(module.implName())
	props.Srcs = module.properties.Srcs
	props.Libs = module.properties.Libs
	props.Static_libs = module.properties.Static_libs
	// XML file is installed along with the impl lib
	props.Required = []string{module.xmlFileName()}

	if module.SocSpecific() {
		props.Soc_specific = proptools.BoolPtr(true)
	} else if module.DeviceSpecific() {
		props.Device_specific = proptools.BoolPtr(true)
	} else if module.ProductSpecific() {
		props.Product_specific = proptools.BoolPtr(true)
	}

	mctx.CreateModule(android.ModuleFactoryAdaptor(LibraryFactory(true)), &props)
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
		Srcs             []string
		Sub_dir          *string
		Soc_specific     *bool
		Device_specific  *bool
		Product_specific *bool
	}{}
	etcProps.Name = proptools.StringPtr(module.xmlFileName())
	etcProps.Srcs = []string{":" + module.xmlFileName() + "-gen"}
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

// For a java_sdk_library module, create internal modules for stubs, docs,
// runtime libs and xml file. If requested, the stubs and docs are created twice
// once for public API level and once for system API level
func sdkLibraryMutator(mctx android.TopDownMutatorContext) {
	if module, ok := mctx.Module().(*sdkLibrary); ok {
		// for public API stubs
		module.createStubsLibrary(mctx, false)
		module.createDocs(mctx, false)

		// for system API stubs
		module.createStubsLibrary(mctx, true)
		module.createDocs(mctx, true)

		// for runtime
		module.createXmlFile(mctx)
		module.createImplLibrary(mctx)
	}
}

func sdkLibraryFactory() android.Module {
	module := &sdkLibrary{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}
