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
	"android/soong/android"
	"android/soong/cc"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"fmt"
	"strings"
)

type ccDeclarationsTagType struct {
	blueprint.BaseDependencyTag
}

var ccDeclarationsTag = ccDeclarationsTagType{}

const baseLibDep = "server_configurable_flags"

type CcAconfigLibraryProperties struct {
	// name of the aconfig_declarations module to generate a library for
	Aconfig_declarations string

	// default mode is "production", the other accepted modes are:
	// "test": to generate test mode version of the library
	// "exported": to generate exported mode version of the library
	// "force-read-only": to generate force-read-only mode version of the library
	// an error will be thrown if the mode is not supported
	Mode *string
}

type CcAconfigLibraryCallbacks struct {
	properties *CcAconfigLibraryProperties

	generatedDir android.WritablePath
	headerDir    android.WritablePath
	generatedCpp android.WritablePath
	generatedH   android.WritablePath
}

func CcAconfigLibraryFactory() android.Module {
	callbacks := &CcAconfigLibraryCallbacks{
		properties: &CcAconfigLibraryProperties{},
	}
	return cc.GeneratedCcLibraryModuleFactory("cc_aconfig_library", callbacks)
}

func (this *CcAconfigLibraryCallbacks) GeneratorInit(ctx cc.BaseModuleContext) {
}

func (this *CcAconfigLibraryCallbacks) GeneratorProps() []interface{} {
	return []interface{}{this.properties}
}

func (this *CcAconfigLibraryCallbacks) GeneratorDeps(ctx cc.DepsContext, deps cc.Deps) cc.Deps {
	// Add a dependency for the declarations module
	declarations := this.properties.Aconfig_declarations
	if len(declarations) == 0 {
		ctx.PropertyErrorf("aconfig_declarations", "aconfig_declarations property required")
	} else {
		ctx.AddDependency(ctx.Module(), ccDeclarationsTag, declarations)
	}

	mode := proptools.StringDefault(this.properties.Mode, "production")

	// Add a dependency for the aconfig flags base library if it is not forced read only
	if mode != "force-read-only" {
		deps.SharedLibs = append(deps.SharedLibs, baseLibDep)
	}
	// TODO: It'd be really nice if we could reexport this library and not make everyone do it.

	return deps
}

func (this *CcAconfigLibraryCallbacks) GeneratorSources(ctx cc.ModuleContext) cc.GeneratedSource {
	result := cc.GeneratedSource{}

	// Get the values that came from the global RELEASE_ACONFIG_VALUE_SETS flag
	declarationsModules := ctx.GetDirectDepsWithTag(ccDeclarationsTag)
	if len(declarationsModules) != 1 {
		panic(fmt.Errorf("Exactly one aconfig_declarations property required"))
	}
	declarations, _ := android.OtherModuleProvider(ctx, declarationsModules[0], android.AconfigDeclarationsProviderKey)

	// Figure out the generated file paths.  This has to match aconfig's codegen_cpp.rs.
	this.generatedDir = android.PathForModuleGen(ctx)

	this.headerDir = android.PathForModuleGen(ctx, "include")
	result.IncludeDirs = []android.Path{this.headerDir}
	result.ReexportedDirs = []android.Path{this.headerDir}

	basename := strings.ReplaceAll(declarations.Package, ".", "_")

	this.generatedCpp = android.PathForModuleGen(ctx, basename+".cc")
	result.Sources = []android.Path{this.generatedCpp}

	this.generatedH = android.PathForModuleGen(ctx, "include", basename+".h")
	result.Headers = []android.Path{this.generatedH}

	return result
}

func (this *CcAconfigLibraryCallbacks) GeneratorFlags(ctx cc.ModuleContext, flags cc.Flags, deps cc.PathDeps) cc.Flags {
	return flags
}

func (this *CcAconfigLibraryCallbacks) GeneratorBuildActions(ctx cc.ModuleContext, flags cc.Flags, deps cc.PathDeps) {
	// Get the values that came from the global RELEASE_ACONFIG_VALUE_SETS flag
	declarationsModules := ctx.GetDirectDepsWithTag(ccDeclarationsTag)
	if len(declarationsModules) != 1 {
		panic(fmt.Errorf("Exactly one aconfig_declarations property required"))
	}
	declarations, _ := android.OtherModuleProvider(ctx, declarationsModules[0], android.AconfigDeclarationsProviderKey)

	mode := proptools.StringDefault(this.properties.Mode, "production")
	if !isModeSupported(mode) {
		ctx.PropertyErrorf("mode", "%q is not a supported mode", mode)
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:  cppRule,
		Input: declarations.IntermediateCacheOutputPath,
		Outputs: []android.WritablePath{
			this.generatedCpp,
			this.generatedH,
		},
		Description: "cc_aconfig_library",
		Args: map[string]string{
			"gendir": this.generatedDir.String(),
			"mode":   mode,
		},
	})
}
