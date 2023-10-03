// Copyright 2023 Google Inc. All rights reserved.
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

package aconfig

import (
	"fmt"

	"android/soong/android"
	"android/soong/bazel"
	"android/soong/java"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type declarationsTagType struct {
	blueprint.BaseDependencyTag
}

var declarationsTag = declarationsTagType{}

type JavaAconfigDeclarationsLibraryProperties struct {
	// name of the aconfig_declarations module to generate a library for
	Aconfig_declarations string

	// whether to generate test mode version of the library
	Test *bool
}

type JavaAconfigDeclarationsLibraryCallbacks struct {
	properties JavaAconfigDeclarationsLibraryProperties
}

func JavaDeclarationsLibraryFactory() android.Module {
	callbacks := &JavaAconfigDeclarationsLibraryCallbacks{}
	return java.GeneratedJavaLibraryModuleFactory("java_aconfig_library", callbacks, &callbacks.properties)
}

func (callbacks *JavaAconfigDeclarationsLibraryCallbacks) DepsMutator(module *java.GeneratedJavaLibraryModule, ctx android.BottomUpMutatorContext) {
	declarations := callbacks.properties.Aconfig_declarations
	if len(declarations) == 0 {
		// TODO: Add test for this case
		ctx.PropertyErrorf("aconfig_declarations", "aconfig_declarations property required")
	} else {
		ctx.AddDependency(ctx.Module(), declarationsTag, declarations)
	}

	// Add aconfig-annotations-lib as a dependency for the optimization / code stripping annotations
	module.AddSharedLibrary("aconfig-annotations-lib")
}

func (callbacks *JavaAconfigDeclarationsLibraryCallbacks) GenerateSourceJarBuildActions(module *java.GeneratedJavaLibraryModule, ctx android.ModuleContext) android.Path {
	// Get the values that came from the global RELEASE_ACONFIG_VALUE_SETS flag
	declarationsModules := ctx.GetDirectDepsWithTag(declarationsTag)
	if len(declarationsModules) != 1 {
		panic(fmt.Errorf("Exactly one aconfig_declarations property required"))
	}
	declarations := ctx.OtherModuleProvider(declarationsModules[0], declarationsProviderKey).(declarationsProviderData)

	// Generate the action to build the srcjar
	srcJarPath := android.PathForModuleGen(ctx, ctx.ModuleName()+".srcjar")
	var mode string
	if proptools.Bool(callbacks.properties.Test) {
		mode = "test"
	} else {
		mode = "production"
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        javaRule,
		Input:       declarations.IntermediatePath,
		Output:      srcJarPath,
		Description: "aconfig.srcjar",
		Args: map[string]string{
			"mode": mode,
		},
	})

	// Tell the java module about the .aconfig files, so they can be propagated up the dependency chain.
	// TODO: It would be nice to have that propagation code here instead of on java.Module and java.JavaInfo.
	module.AddAconfigIntermediate(declarations.IntermediatePath)

	return srcJarPath
}

type bazelJavaAconfigLibraryAttributes struct {
	Aconfig_declarations bazel.LabelAttribute
	Test                 *bool
	Sdk_version          *string
}

func (callbacks *JavaAconfigDeclarationsLibraryCallbacks) Bp2build(ctx android.Bp2buildMutatorContext, module *java.GeneratedJavaLibraryModule) {
	if ctx.ModuleType() != "java_aconfig_library" {
		return
	}

	// By default, soong builds the aconfig java library with private_current, however
	// bazel currently doesn't support it so we default it to system_current. One reason
	// is that the dependency of all java_aconfig_library aconfig-annotations-lib is
	// built with system_current. For the java aconfig library itself it doesn't really
	// matter whether it uses private API or system API because the only module it uses
	// is DeviceConfig which is in system, and the rdeps of the java aconfig library
	// won't change its sdk version either, so this should be fine.
	// Ideally we should only use the default value if it is not set by the user, but
	// bazel only supports a limited sdk versions, for example, the java_aconfig_library
	// modules in framework/base use core_platform which is not supported by bazel yet.
	// TODO(b/302148527): change soong to default to system_current as well.
	sdkVersion := "system_current"
	attrs := bazelJavaAconfigLibraryAttributes{
		Aconfig_declarations: *bazel.MakeLabelAttribute(android.BazelLabelForModuleDepSingle(ctx, callbacks.properties.Aconfig_declarations).Label),
		Test:                 callbacks.properties.Test,
		Sdk_version:          &sdkVersion,
	}
	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "java_aconfig_library",
		Bzl_load_location: "//build/bazel/rules/java:java_aconfig_library.bzl",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{Name: ctx.ModuleName()}, &attrs)
}
