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
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

func init() {
	pctx.HostBinToolVariable("protocCmd", "aprotoc")
	pctx.HostBinToolVariable("depFixCmd", "dep_fixer")
}

var (
	proto = pctx.AndroidStaticRule("protoc",
		blueprint.RuleParams{
			Command: `rm -rf $out.tmp && mkdir -p $out.tmp && ` +
				`$protocCmd $protoOut=$protoOutParams:$out.tmp --dependency_out=$out.d -I $protoBase $protoFlags $in && ` +
				`$depFixCmd $out.d && ` +
				`${config.SoongZipCmd} -jar -o $out -C $out.tmp -D $out.tmp && rm -rf $out.tmp`,
			CommandDeps: []string{
				"$protocCmd",
				"$depFixCmd",
				"${config.SoongZipCmd}",
			},
			Depfile: "${out}.d",
			Deps:    blueprint.DepsGCC,
		}, "protoBase", "protoFlags", "protoOut", "protoOutParams")
)

func genProto(ctx android.ModuleContext, protoFile android.Path, flags javaBuilderFlags) android.Path {
	srcJarFile := android.GenPathWithExt(ctx, "proto", protoFile, "srcjar")

	var protoBase string
	if flags.protoRoot {
		protoBase = "."
	} else {
		protoBase = strings.TrimSuffix(protoFile.String(), protoFile.Rel())
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        proto,
		Description: "protoc " + protoFile.Rel(),
		Output:      srcJarFile,
		Input:       protoFile,
		Args: map[string]string{
			"protoBase":      protoBase,
			"protoOut":       flags.protoOutTypeFlag,
			"protoOutParams": flags.protoOutParams,
			"protoFlags":     strings.Join(flags.protoFlags, " "),
		},
	})

	return srcJarFile
}

func protoDeps(ctx android.BottomUpMutatorContext, p *android.ProtoProperties) {
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

func protoFlags(ctx android.ModuleContext, j *CompilerProperties, p *android.ProtoProperties,
	flags javaBuilderFlags) javaBuilderFlags {

	switch String(p.Proto.Type) {
	case "micro":
		flags.protoOutTypeFlag = "--javamicro_out"
	case "nano":
		flags.protoOutTypeFlag = "--javanano_out"
	case "lite":
		flags.protoOutTypeFlag = "--java_out"
		flags.protoOutParams = "lite"
	case "full", "":
		flags.protoOutTypeFlag = "--java_out"
	default:
		ctx.PropertyErrorf("proto.type", "unknown proto type %q",
			String(p.Proto.Type))
	}

	if len(j.Proto.Output_params) > 0 {
		if flags.protoOutParams != "" {
			flags.protoOutParams += ","
		}
		flags.protoOutParams += strings.Join(j.Proto.Output_params, ",")
	}

	flags.protoFlags = android.ProtoFlags(ctx, p)
	flags.protoRoot = android.ProtoCanonicalPathFromRoot(ctx, p)

	return flags
}
