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
	"path/filepath"
	"strconv"

	"android/soong/android"
)

func genProto(ctx android.ModuleContext, protoFiles android.Paths, flags android.ProtoFlags) android.Paths {
	// Shard proto files into groups of 100 to avoid having to recompile all of them if one changes and to avoid
	// hitting command line length limits.
	shards := android.ShardPaths(protoFiles, 100)

	srcJarFiles := make(android.Paths, 0, len(shards))

	for i, shard := range shards {
		srcJarFile := android.PathForModuleGen(ctx, "proto", "proto"+strconv.Itoa(i)+".srcjar")
		srcJarFiles = append(srcJarFiles, srcJarFile)

		outDir := srcJarFile.ReplaceExtension(ctx, "tmp")

		rule := android.NewRuleBuilder(pctx, ctx)

		rule.Command().Text("rm -rf").Flag(outDir.String())
		rule.Command().Text("mkdir -p").Flag(outDir.String())

		for _, protoFile := range shard {
			depFile := srcJarFile.InSameDir(ctx, protoFile.String()+".d")
			rule.Command().Text("mkdir -p").Flag(filepath.Dir(depFile.String()))
			android.ProtoRule(rule, protoFile, flags, flags.Deps, outDir, depFile, nil)
		}

		// Proto generated java files have an unknown package name in the path, so package the entire output directory
		// into a srcjar.
		rule.Command().
			BuiltTool("soong_zip").
			Flag("-srcjar").
			Flag("-write_if_changed").
			FlagWithOutput("-o ", srcJarFile).
			FlagWithArg("-C ", outDir.String()).
			FlagWithArg("-D ", outDir.String())

		rule.Command().Text("rm -rf").Flag(outDir.String())

		rule.Restat()

		ruleName := "protoc"
		ruleDesc := "protoc"
		if len(shards) > 1 {
			ruleName += "_" + strconv.Itoa(i)
			ruleDesc += " " + strconv.Itoa(i)
		}

		rule.Build(ruleName, ruleDesc)
	}

	return srcJarFiles
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
			if ctx.Host() || ctx.BazelConversionMode() {
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
		var typeToPlugin string
		switch String(p.Proto.Type) {
		case "micro":
			flags.proto.OutTypeFlag = "--javamicro_out"
			typeToPlugin = "javamicro"
		case "nano":
			flags.proto.OutTypeFlag = "--javanano_out"
			typeToPlugin = "javanano"
		case "lite", "":
			flags.proto.OutTypeFlag = "--java_out"
			flags.proto.OutParams = append(flags.proto.OutParams, "lite")
		case "full":
			flags.proto.OutTypeFlag = "--java_out"
		default:
			ctx.PropertyErrorf("proto.type", "unknown proto type %q",
				String(p.Proto.Type))
		}

		if typeToPlugin != "" {
			hostTool := ctx.Config().HostToolPath(ctx, "protoc-gen-"+typeToPlugin)
			flags.proto.Deps = append(flags.proto.Deps, hostTool)
			flags.proto.Flags = append(flags.proto.Flags, "--plugin=protoc-gen-"+typeToPlugin+"="+hostTool.String())
		}
	}

	flags.proto.OutParams = append(flags.proto.OutParams, j.Proto.Output_params...)

	return flags
}
