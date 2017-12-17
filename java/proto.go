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
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	pctx.HostBinToolVariable("protocCmd", "aprotoc")
}

var (
	proto = pctx.AndroidStaticRule("protoc",
		blueprint.RuleParams{
			Command: `rm -rf $outDir && mkdir -p $outDir && ` +
				`$protocCmd $protoOut=$protoOutParams:$outDir $protoFlags $in && ` +
				`${config.SoongZipCmd} -jar -o $out -C $outDir -D $outDir`,
			CommandDeps: []string{
				"$protocCmd",
				"${config.SoongZipCmd}",
			},
		}, "protoFlags", "protoOut", "protoOutParams", "outDir")
)

func genProto(ctx android.ModuleContext, outputSrcJar android.WritablePath,
	protoFiles android.Paths, protoFlags []string, protoOut, protoOutParams string) {

	ctx.Build(pctx, android.BuildParams{
		Rule:        proto,
		Description: "protoc " + protoFiles[0].Rel(),
		Output:      outputSrcJar,
		Inputs:      protoFiles,
		Args: map[string]string{
			"outDir":         android.ProtoDir(ctx).String(),
			"protoOut":       protoOut,
			"protoOutParams": protoOutParams,
			"protoFlags":     strings.Join(protoFlags, " "),
		},
	})
}

func protoDeps(ctx android.BottomUpMutatorContext, p *android.ProtoProperties) {
	switch proptools.String(p.Proto.Type) {
	case "micro":
		ctx.AddDependency(ctx.Module(), staticLibTag, "libprotobuf-java-micro")
	case "nano":
		ctx.AddDependency(ctx.Module(), staticLibTag, "libprotobuf-java-nano")
	case "lite", "":
		ctx.AddDependency(ctx.Module(), staticLibTag, "libprotobuf-java-lite")
	case "full":
		if ctx.Host() {
			ctx.AddDependency(ctx.Module(), staticLibTag, "libprotobuf-java-full")
		} else {
			ctx.PropertyErrorf("proto.type", "full java protos only supported on the host")
		}
	default:
		ctx.PropertyErrorf("proto.type", "unknown proto type %q",
			proptools.String(p.Proto.Type))
	}
}

func protoFlags(ctx android.ModuleContext, j *CompilerProperties, p *android.ProtoProperties,
	flags javaBuilderFlags) javaBuilderFlags {

	switch proptools.String(p.Proto.Type) {
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
			proptools.String(p.Proto.Type))
	}

	if len(j.Proto.Output_params) > 0 {
		if flags.protoOutParams != "" {
			flags.protoOutParams += ","
		}
		flags.protoOutParams += strings.Join(j.Proto.Output_params, ",")
	}

	flags.protoFlags = android.ProtoFlags(ctx, p)

	return flags
}
