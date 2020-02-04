// Copyright 2016 Google Inc. All rights reserved.
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

package cc

import (
	"fmt"

	"android/soong/android"
)

//
// Objects (for crt*.o)
//

func init() {
	android.RegisterModuleType("cc_object", ObjectFactory)
}

type objectLinker struct {
	*baseLinker
	Properties ObjectLinkerProperties
}

// cc_object runs the compiler without running the linker. It is rarely
// necessary, but sometimes used to generate .s files from .c files to use as
// input to a cc_genrule module.
func ObjectFactory() android.Module {
	module := newBaseModule(android.HostAndDeviceSupported, android.MultilibBoth)
	module.linker = &objectLinker{
		baseLinker: NewBaseLinker(nil),
	}
	module.compiler = NewBaseCompiler()

	// Clang's address-significance tables are incompatible with ld -r.
	module.compiler.appendCflags([]string{"-fno-addrsig"})

	module.stl = &stl{}
	return module.Init()
}

func (object *objectLinker) appendLdflags(flags []string) {
	panic(fmt.Errorf("appendLdflags on objectLinker not supported"))
}

func (object *objectLinker) linkerProps() []interface{} {
	return []interface{}{&object.Properties}
}

func (*objectLinker) linkerInit(ctx BaseModuleContext) {}

func (object *objectLinker) linkerDeps(ctx DepsContext, deps Deps) Deps {
	if ctx.useVndk() && ctx.toolchain().Bionic() {
		// Needed for VNDK builds where bionic headers aren't automatically added.
		deps.LateSharedLibs = append(deps.LateSharedLibs, "libc")
	}

	deps.ObjFiles = append(deps.ObjFiles, object.Properties.Objs...)
	return deps
}

func (*objectLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags.LdFlags = append(flags.LdFlags, ctx.toolchain().ToolchainClangLdflags())

	return flags
}

func (object *objectLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	objs = objs.Append(deps.Objs)

	var outputFile android.Path
	builderFlags := flagsToBuilderFlags(flags)

	if len(objs.objFiles) == 1 {
		outputFile = objs.objFiles[0]

		if String(object.Properties.Prefix_symbols) != "" {
			output := android.PathForModuleOut(ctx, ctx.ModuleName()+objectExtension)
			TransformBinaryPrefixSymbols(ctx, String(object.Properties.Prefix_symbols), outputFile,
				builderFlags, output)
			outputFile = output
		}
	} else {
		output := android.PathForModuleOut(ctx, ctx.ModuleName()+objectExtension)
		outputFile = output

		if String(object.Properties.Prefix_symbols) != "" {
			input := android.PathForModuleOut(ctx, "unprefixed", ctx.ModuleName()+objectExtension)
			TransformBinaryPrefixSymbols(ctx, String(object.Properties.Prefix_symbols), input,
				builderFlags, output)
			output = input
		}

		TransformObjsToObj(ctx, objs.objFiles, builderFlags, output)
	}

	ctx.CheckbuildFile(outputFile)
	return outputFile
}

func (object *objectLinker) unstrippedOutputFilePath() android.Path {
	return nil
}

func (object *objectLinker) nativeCoverage() bool {
	return true
}

func (object *objectLinker) coverageOutputFilePath() android.OptionalPath {
	return android.OptionalPath{}
}
