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

var aconfigSupportedModes = []string{"production", "test", "exported", "force-read-only"}

type JavaAconfigDeclarationsLibraryProperties struct {
	// name of the aconfig_declarations module to generate a library for
	Aconfig_declarations string

	// default mode is "production", the other accepted modes are:
	// "test": to generate test mode version of the library
	// "exported": to generate exported mode version of the library
	// "force-read-only": to generate force-read-only mode version of the library
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

	// "libcore_aconfig_flags_lib" module has a circular dependency because the shared libraries
	// are built on core_current and the module is used to flag the APIs in the core_current.
	// http://b/316554963#comment2 has the details of the circular dependency chain.
	// If a java_aconfig_library uses "none" sdk_version, it should include and build these
	// annotation files as the shared library themselves.
	var addLibraries bool = module.Library.Module.SdkVersion(ctx).Kind != android.SdkNone
	if addLibraries {
		// Add aconfig-annotations-lib as a dependency for the optimization / code stripping annotations
		module.AddSharedLibrary("aconfig-annotations-lib")
		// TODO(b/303773055): Remove the annotation after access issue is resolved.
		module.AddSharedLibrary("unsupportedappusage")
	}
}

func (callbacks *JavaAconfigDeclarationsLibraryCallbacks) GenerateSourceJarBuildActions(module *java.GeneratedJavaLibraryModule, ctx android.ModuleContext) android.Path {
	// Get the values that came from the global RELEASE_ACONFIG_VALUE_SETS flag
	declarationsModules := ctx.GetDirectDepsWithTag(declarationsTag)
	if len(declarationsModules) != 1 {
		panic(fmt.Errorf("Exactly one aconfig_declarations property required"))
	}
	declarations, _ := android.OtherModuleProvider(ctx, declarationsModules[0], android.AconfigDeclarationsProviderKey)

	// Generate the action to build the srcjar
	srcJarPath := android.PathForModuleGen(ctx, ctx.ModuleName()+".srcjar")

	mode := proptools.StringDefault(callbacks.properties.Mode, "production")
	if !isModeSupported(mode) {
		ctx.PropertyErrorf("mode", "%q is not a supported mode", mode)
	}
	// TODO: uncomment this part after internal clean up
	//if mode == "exported" && !declarations.Exportable {
	//	// if mode is exported, the corresponding aconfig_declaration must mark its
	//	// exportable property true
	//	ctx.PropertyErrorf("mode", "exported mode requires its aconfig_declaration has exportable prop true")
	//}

	ctx.Build(pctx, android.BuildParams{
		Rule:        javaRule,
		Input:       declarations.IntermediateCacheOutputPath,
		Output:      srcJarPath,
		Description: "aconfig.srcjar",
		Args: map[string]string{
			"mode": mode,
		},
	})

	if declarations.Exportable {
		// Mark our generated code as possibly needing jarjar repackaging
		// The repackaging only happens when the corresponding aconfig_declaration
		// has property exportable true
		module.AddJarJarRenameRule(declarations.Package+".Flags", "")
		module.AddJarJarRenameRule(declarations.Package+".FeatureFlags", "")
		module.AddJarJarRenameRule(declarations.Package+".FeatureFlagsImpl", "")
		module.AddJarJarRenameRule(declarations.Package+".FakeFeatureFlagsImpl", "")
	}

	android.SetProvider(ctx, aconfig.CodegenInfoProvider, aconfig.CodegenInfo{
		AconfigDeclarations:          []string{declarationsModules[0].Name()},
		IntermediateCacheOutputPaths: android.Paths{declarations.IntermediateCacheOutputPath},
		Srcjars:                      android.Paths{srcJarPath},
	})

	return srcJarPath
}

func isModeSupported(mode string) bool {
	return android.InList(mode, aconfigSupportedModes)
}
