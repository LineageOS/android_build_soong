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
	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

const (
	protoTypeDefault = "lite"
)

// genProto creates a rule to convert a .proto file to generated .pb.cc and .pb.h files and returns
// the paths to the generated files.
func genProto(ctx android.ModuleContext, protoFile android.Path, flags builderFlags) (cc, header android.WritablePath) {
	var ccFile, headerFile android.ModuleGenPath

	srcSuffix := ".cc"
	if flags.protoC {
		srcSuffix = ".c"
	}

	if flags.proto.CanonicalPathFromRoot {
		ccFile = android.GenPathWithExt(ctx, "proto", protoFile, "pb"+srcSuffix)
		headerFile = android.GenPathWithExt(ctx, "proto", protoFile, "pb.h")
	} else {
		rel := protoFile.Rel()
		ccFile = android.PathForModuleGen(ctx, "proto", pathtools.ReplaceExtension(rel, "pb"+srcSuffix))
		headerFile = android.PathForModuleGen(ctx, "proto", pathtools.ReplaceExtension(rel, "pb.h"))
	}

	protoDeps := flags.proto.Deps
	if flags.protoOptionsFile {
		optionsFile := pathtools.ReplaceExtension(protoFile.String(), "options")
		optionsPath := android.PathForSource(ctx, optionsFile)
		protoDeps = append(android.Paths{optionsPath}, protoDeps...)
	}

	outDir := flags.proto.Dir
	depFile := ccFile.ReplaceExtension(ctx, "d")
	outputs := android.WritablePaths{ccFile, headerFile}

	rule := android.NewRuleBuilder(pctx, ctx)

	android.ProtoRule(rule, protoFile, flags.proto, protoDeps, outDir, depFile, outputs)

	rule.Build("protoc_"+protoFile.Rel(), "protoc "+protoFile.Rel())

	return ccFile, headerFile
}

func protoDeps(ctx DepsContext, deps Deps, p *android.ProtoProperties, static bool) Deps {
	var lib string

	if String(p.Proto.Plugin) == "" {
		switch proptools.StringDefault(p.Proto.Type, protoTypeDefault) {
		case "full":
			if ctx.useSdk() {
				lib = "libprotobuf-cpp-full-ndk"
				static = true
			} else {
				lib = "libprotobuf-cpp-full"
			}
		case "lite":
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
		case "nanopb-c-16bit":
			lib = "libprotobuf-c-nano-16bit"
			static = true
		case "nanopb-c-enable_malloc-16bit":
			lib = "libprotobuf-c-nano-enable_malloc-16bit"
			static = true
		case "nanopb-c-32bit":
			lib = "libprotobuf-c-nano-32bit"
			static = true
		case "nanopb-c-enable_malloc-32bit":
			lib = "libprotobuf-c-nano-enable_malloc-32bit"
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
	}

	return deps
}

func protoFlags(ctx ModuleContext, flags Flags, p *android.ProtoProperties) Flags {
	flags.Local.CFlags = append(flags.Local.CFlags, "-DGOOGLE_PROTOBUF_NO_RTTI")

	flags.proto = android.GetProtoFlags(ctx, p)
	if flags.proto.CanonicalPathFromRoot {
		flags.Local.CommonFlags = append(flags.Local.CommonFlags, "-I"+flags.proto.SubDir.String())
	}
	flags.Local.CommonFlags = append(flags.Local.CommonFlags, "-I"+flags.proto.Dir.String())

	if String(p.Proto.Plugin) == "" {
		var plugin string

		switch String(p.Proto.Type) {
		case "nanopb-c", "nanopb-c-enable_malloc", "nanopb-c-16bit", "nanopb-c-enable_malloc-16bit", "nanopb-c-32bit", "nanopb-c-enable_malloc-32bit":
			flags.protoC = true
			flags.protoOptionsFile = true
			flags.proto.OutTypeFlag = "--nanopb_out"
			// Disable nanopb timestamps to support remote caching.
			flags.proto.OutParams = append(flags.proto.OutParams, "-T")
			plugin = "protoc-gen-nanopb"
		case "full":
			flags.proto.OutTypeFlag = "--cpp_out"
		case "lite":
			flags.proto.OutTypeFlag = "--cpp_out"
			flags.proto.OutParams = append(flags.proto.OutParams, "lite")
		case "":
			// TODO(b/119714316): this should be equivalent to "lite" in
			// order to match protoDeps, but some modules are depending on
			// this behavior
			flags.proto.OutTypeFlag = "--cpp_out"
		default:
			ctx.PropertyErrorf("proto.type", "unknown proto type %q",
				String(p.Proto.Type))
		}

		if plugin != "" {
			path := ctx.Config().HostToolPath(ctx, plugin)
			flags.proto.Deps = append(flags.proto.Deps, path)
			flags.proto.Flags = append(flags.proto.Flags, "--plugin="+path.String())
		}
	}

	return flags
}
