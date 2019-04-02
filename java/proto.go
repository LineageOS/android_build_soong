// Copyright 2017 Google Inc. All rights reserved.
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
)

func genProto(ctx android.ModuleContext, protoFile android.Path, flags android.ProtoFlags) android.Path {
	srcJarFile := android.GenPathWithExt(ctx, "proto", protoFile, "srcjar")

	outDir := srcJarFile.ReplaceExtension(ctx, "tmp")
	depFile := srcJarFile.ReplaceExtension(ctx, "srcjar.d")

	rule := android.NewRuleBuilder()

	rule.Command().Text("rm -rf").Flag(outDir.String())
	rule.Command().Text("mkdir -p").Flag(outDir.String())

	android.ProtoRule(ctx, rule, protoFile, flags, flags.Deps, outDir, depFile, nil)

	// Proto generated java files have an unknown package name in the path, so package the entire output directory
	// into a srcjar.
	rule.Command().
		Tool(ctx.Config().HostToolPath(ctx, "soong_zip")).
		Flag("-jar").
		FlagWithOutput("-o ", srcJarFile).
		FlagWithArg("-C ", outDir.String()).
		FlagWithArg("-D ", outDir.String())

	rule.Command().Text("rm -rf").Flag(outDir.String())

	rule.Build(pctx, ctx, "protoc_"+protoFile.Rel(), "protoc "+protoFile.Rel())

	return srcJarFile
}

func protoDeps(ctx android.BottomUpMutatorContext, p *android.ProtoProperties) {
	if String(p.Proto.Plugin) == "" {
		switch String(p.Proto.Type) {
		case "micro":
			ctx.AddVariationDependencies(nil, staticLibTag, "libprotobuf-java-micro")
		case "nano":
			ctx.AddVariationDependencies(nil, staticLibTag, "libprotobuf-java-nano")
		case "lite", "":
			ctx.AddVariationDependencies(nil, staticLibTag, "libprotobuf-java-lite")
		case "full":
			if ctx.Host() {
				ctx.AddVariationDependencies(nil, staticLibTag, "libprotobuf-java-full")
			} else {
				ctx.PropertyErrorf("proto.type", "full java protos only supported on the host")
			}
		default:
			ctx.PropertyErrorf("proto.type", "unknown proto type %q",
				String(p.Proto.Type))
		}
	}
}

func protoFlags(ctx android.ModuleContext, j *CompilerProperties, p *android.ProtoProperties,
	flags javaBuilderFlags) javaBuilderFlags {

	flags.proto = android.GetProtoFlags(ctx, p)

	if String(p.Proto.Plugin) == "" {
		switch String(p.Proto.Type) {
		case "micro":
			flags.proto.OutTypeFlag = "--javamicro_out"
		case "nano":
			flags.proto.OutTypeFlag = "--javanano_out"
		case "lite":
			flags.proto.OutTypeFlag = "--java_out"
			flags.proto.OutParams = append(flags.proto.OutParams, "lite")
		case "full", "":
			flags.proto.OutTypeFlag = "--java_out"
		default:
			ctx.PropertyErrorf("proto.type", "unknown proto type %q",
				String(p.Proto.Type))
		}
	}

	flags.proto.OutParams = append(flags.proto.OutParams, j.Proto.Output_params...)

	return flags
}
