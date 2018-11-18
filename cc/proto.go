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
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"

	"android/soong/android"
)

func init() {
	pctx.HostBinToolVariable("protocCmd", "aprotoc")
	pctx.HostBinToolVariable("depFixCmd", "dep_fixer")
}

var (
	proto = pctx.AndroidStaticRule("protoc",
		blueprint.RuleParams{
			Command: "$protocCmd $protoOut=$protoOutParams:$outDir --dependency_out=$out.d -I $protoBase $protoFlags $in && " +
				`$depFixCmd $out.d`,
			CommandDeps: []string{"$protocCmd", "$depFixCmd"},
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
		}, "protoFlags", "protoOut", "protoOutParams", "protoBase", "outDir")
)

// genProto creates a rule to convert a .proto file to generated .pb.cc and .pb.h files and returns
// the paths to the generated files.
func genProto(ctx android.ModuleContext, protoFile android.Path, flags builderFlags) (ccFile, headerFile android.WritablePath) {

	srcSuffix := ".cc"
	if flags.protoC {
		srcSuffix = ".c"
	}

	var protoBase string
	if flags.protoRoot {
		protoBase = "."
		ccFile = android.GenPathWithExt(ctx, "proto", protoFile, "pb"+srcSuffix)
		headerFile = android.GenPathWithExt(ctx, "proto", protoFile, "pb.h")
	} else {
		rel := protoFile.Rel()
		protoBase = strings.TrimSuffix(protoFile.String(), rel)
		ccFile = android.PathForModuleGen(ctx, "proto", pathtools.ReplaceExtension(rel, "pb"+srcSuffix))
		headerFile = android.PathForModuleGen(ctx, "proto", pathtools.ReplaceExtension(rel, "pb.h"))
	}

	protoDeps := flags.protoDeps
	if flags.protoOptionsFile {
		optionsFile := pathtools.ReplaceExtension(protoFile.String(), "options")
		optionsPath := android.ExistentPathForSource(ctx, optionsFile)
		if optionsPath.Valid() {
			protoDeps = append(android.Paths{optionsPath.Path()}, protoDeps...)
		}
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:           proto,
		Description:    "protoc " + protoFile.Rel(),
		Output:         ccFile,
		ImplicitOutput: headerFile,
		Input:          protoFile,
		Implicits:      protoDeps,
		Args: map[string]string{
			"outDir":         android.ProtoDir(ctx).String(),
			"protoFlags":     flags.protoFlags,
			"protoOut":       flags.protoOutTypeFlag,
			"protoOutParams": flags.protoOutParams,
			"protoBase":      protoBase,
		},
	})

	return ccFile, headerFile
}

func protoDeps(ctx BaseModuleContext, deps Deps, p *android.ProtoProperties, static bool) Deps {
	var lib string

	switch String(p.Proto.Type) {
	case "full":
		if ctx.useSdk() {
			lib = "libprotobuf-cpp-full-ndk"
			static = true
		} else {
			lib = "libprotobuf-cpp-full"
		}
	case "lite", "":
		if ctx.useSdk() {
			lib = "libprotobuf-cpp-lite-ndk"
			static = true
		} else {
			lib = "libprotobuf-cpp-lite"
		}
	case "nanopb-c":
		lib = "libprotobuf-c-nano"
		static = true
	case "nanopb-c-enable_malloc":
		lib = "libprotobuf-c-nano-enable_malloc"
		static = true
	default:
		ctx.PropertyErrorf("proto.type", "unknown proto type %q",
			String(p.Proto.Type))
	}

	if static {
		deps.StaticLibs = append(deps.StaticLibs, lib)
		deps.ReexportStaticLibHeaders = append(deps.ReexportStaticLibHeaders, lib)
	} else {
		deps.SharedLibs = append(deps.SharedLibs, lib)
		deps.ReexportSharedLibHeaders = append(deps.ReexportSharedLibHeaders, lib)
	}

	return deps
}

func protoFlags(ctx ModuleContext, flags Flags, p *android.ProtoProperties) Flags {
	flags.CFlags = append(flags.CFlags, "-DGOOGLE_PROTOBUF_NO_RTTI")

	flags.ProtoRoot = android.ProtoCanonicalPathFromRoot(ctx, p)
	if flags.ProtoRoot {
		flags.GlobalFlags = append(flags.GlobalFlags, "-I"+android.ProtoSubDir(ctx).String())
	}
	flags.GlobalFlags = append(flags.GlobalFlags, "-I"+android.ProtoDir(ctx).String())

	flags.protoFlags = android.ProtoFlags(ctx, p)

	var plugin string

	switch String(p.Proto.Type) {
	case "nanopb-c", "nanopb-c-enable_malloc":
		flags.protoC = true
		flags.protoOptionsFile = true
		flags.protoOutTypeFlag = "--nanopb_out"
		plugin = "protoc-gen-nanopb"
	case "full":
		flags.protoOutTypeFlag = "--cpp_out"
	case "lite":
		flags.protoOutTypeFlag = "--cpp_out"
		flags.protoOutParams = append(flags.protoOutParams, "lite")
	case "":
		// TODO(b/119714316): this should be equivalent to "lite" in
		// order to match protoDeps, but some modules are depending on
		// this behavior
		flags.protoOutTypeFlag = "--cpp_out"
	default:
		ctx.PropertyErrorf("proto.type", "unknown proto type %q",
			String(p.Proto.Type))
	}

	if plugin != "" {
		path := ctx.Config().HostToolPath(ctx, plugin)
		flags.protoDeps = append(flags.protoDeps, path)
		flags.protoFlags = append(flags.protoFlags, "--plugin="+path.String())
	}

	return flags
}
