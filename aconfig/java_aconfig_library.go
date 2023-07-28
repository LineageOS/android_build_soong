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
	"android/soong/android"
	"android/soong/java"
	"fmt"
	"github.com/google/blueprint"
)

type declarationsTagType struct {
	blueprint.BaseDependencyTag
}

var declarationsTag = declarationsTagType{}

type JavaAconfigDeclarationsLibraryProperties struct {
	// name of the aconfig_declarations module to generate a library for
	Aconfig_declarations string

	// whether to generate test mode version of the library
	Test bool
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
	if callbacks.properties.Test {
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
