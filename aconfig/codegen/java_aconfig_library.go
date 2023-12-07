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

package codegen

import (
	"fmt"

	"android/soong/aconfig"
	"android/soong/android"
	"android/soong/java"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type declarationsTagType struct {
	blueprint.BaseDependencyTag
}

var declarationsTag = declarationsTagType{}

var aconfigSupportedModes = []string{"production", "test", "exported"}

type JavaAconfigDeclarationsLibraryProperties struct {
	// name of the aconfig_declarations module to generate a library for
	Aconfig_declarations string

	// default mode is "production", the other accepted modes are:
	// "test": to generate test mode version of the library
	// "exported": to generate exported mode version of the library
	// an error will be thrown if the mode is not supported
	Mode *string
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
	// TODO(b/303773055): Remove the annotation after access issue is resolved.
	module.AddSharedLibrary("unsupportedappusage")
}

func (callbacks *JavaAconfigDeclarationsLibraryCallbacks) GenerateSourceJarBuildActions(module *java.GeneratedJavaLibraryModule, ctx android.ModuleContext) android.Path {
	// Get the values that came from the global RELEASE_ACONFIG_VALUE_SETS flag
	declarationsModules := ctx.GetDirectDepsWithTag(declarationsTag)
	if len(declarationsModules) != 1 {
		panic(fmt.Errorf("Exactly one aconfig_declarations property required"))
	}
	declarations := ctx.OtherModuleProvider(declarationsModules[0], aconfig.DeclarationsProviderKey).(aconfig.DeclarationsProviderData)

	// Generate the action to build the srcjar
	srcJarPath := android.PathForModuleGen(ctx, ctx.ModuleName()+".srcjar")

	mode := proptools.StringDefault(callbacks.properties.Mode, "production")
	if !isModeSupported(mode) {
		ctx.PropertyErrorf("mode", "%q is not a supported mode", mode)
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        javaRule,
		Input:       declarations.IntermediateCacheOutputPath,
		Output:      srcJarPath,
		Description: "aconfig.srcjar",
		Args: map[string]string{
			"mode": mode,
		},
	})

	return srcJarPath
}

func isModeSupported(mode string) bool {
	return android.InList(mode, aconfigSupportedModes)
}
