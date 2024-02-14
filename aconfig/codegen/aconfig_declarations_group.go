// Copyright 2024 Google Inc. All rights reserved.
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
	"android/soong/aconfig"
	"android/soong/android"
	"fmt"

	"github.com/google/blueprint"
)

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var (
	aconfigDeclarationsGroupTag = dependencyTag{name: "aconfigDeclarationsGroup"}
	javaAconfigLibraryTag       = dependencyTag{name: "javaAconfigLibrary"}
	ccAconfigLibraryTag         = dependencyTag{name: "ccAconfigLibrary"}
	rustAconfigLibraryTag       = dependencyTag{name: "rustAconfigLibrary"}
)

type AconfigDeclarationsGroup struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties AconfigDeclarationsGroupProperties

	aconfigDeclarationNames      []string
	intermediateCacheOutputPaths android.Paths
	javaSrcjars                  android.Paths
}

type AconfigDeclarationsGroupProperties struct {

	// Name of the aconfig_declarations_group modules
	Aconfig_declarations_groups []string

	// Name of the java_aconfig_library modules
	Java_aconfig_libraries []string

	// Name of the cc_aconfig_library modules
	Cc_aconfig_libraries []string

	// Name of the rust_aconfig_library modules
	Rust_aconfig_libraries []string
}

func AconfigDeclarationsGroupFactory() android.Module {
	module := &AconfigDeclarationsGroup{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	android.InitDefaultableModule(module)
	return module
}

func (adg *AconfigDeclarationsGroup) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddDependency(ctx.Module(), aconfigDeclarationsGroupTag, adg.properties.Aconfig_declarations_groups...)
	ctx.AddDependency(ctx.Module(), javaAconfigLibraryTag, adg.properties.Java_aconfig_libraries...)
	ctx.AddDependency(ctx.Module(), ccAconfigLibraryTag, adg.properties.Cc_aconfig_libraries...)
	ctx.AddDependency(ctx.Module(), rustAconfigLibraryTag, adg.properties.Rust_aconfig_libraries...)
}

func (adg *AconfigDeclarationsGroup) VisitDeps(ctx android.ModuleContext) {
	ctx.VisitDirectDeps(func(dep android.Module) {
		tag := ctx.OtherModuleDependencyTag(dep)
		if provider, ok := android.OtherModuleProvider(ctx, dep, aconfig.CodegenInfoProvider); ok {

			// aconfig declaration names and cache files are collected for all aconfig library dependencies
			adg.aconfigDeclarationNames = append(adg.aconfigDeclarationNames, provider.AconfigDeclarations...)
			adg.intermediateCacheOutputPaths = append(adg.intermediateCacheOutputPaths, provider.IntermediateCacheOutputPaths...)

			switch tag {
			case aconfigDeclarationsGroupTag:
				// Will retrieve outputs from another language codegen modules when support is added
				adg.javaSrcjars = append(adg.javaSrcjars, provider.Srcjars...)
			case javaAconfigLibraryTag:
				adg.javaSrcjars = append(adg.javaSrcjars, provider.Srcjars...)
			}
		}
	})
}

func (adg *AconfigDeclarationsGroup) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	adg.VisitDeps(ctx)
	adg.aconfigDeclarationNames = android.FirstUniqueStrings(adg.aconfigDeclarationNames)
	adg.intermediateCacheOutputPaths = android.FirstUniquePaths(adg.intermediateCacheOutputPaths)

	android.SetProvider(ctx, aconfig.CodegenInfoProvider, aconfig.CodegenInfo{
		AconfigDeclarations:          adg.aconfigDeclarationNames,
		IntermediateCacheOutputPaths: adg.intermediateCacheOutputPaths,
		Srcjars:                      adg.javaSrcjars,
	})
}

var _ android.OutputFileProducer = (*AconfigDeclarationsGroup)(nil)

func (adg *AconfigDeclarationsGroup) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return adg.intermediateCacheOutputPaths, nil
	case ".srcjars":
		return adg.javaSrcjars, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %s", tag)
	}
}

func (adg *AconfigDeclarationsGroup) Srcjars() android.Paths {
	return adg.javaSrcjars
}

func (adg *AconfigDeclarationsGroup) AconfigDeclarations() []string {
	return adg.aconfigDeclarationNames
}
