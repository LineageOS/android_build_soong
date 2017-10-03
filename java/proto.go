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
				`$protocCmd $protoOut=$protoOutFlags:$outDir $protoFlags $in && ` +
				`find $outDir -name "*.java" > $out`,
			CommandDeps: []string{"$protocCmd"},
		}, "protoFlags", "protoOut", "protoOutFlags", "outDir")
)

func genProto(ctx android.ModuleContext, protoFiles android.Paths,
	protoFlags string, protoOut, protoOutFlags string) android.WritablePath {

	protoFileList := android.PathForModuleGen(ctx, "proto.filelist")

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:        proto,
		Description: "protoc " + protoFiles[0].Rel(),
		Output:      protoFileList,
		Inputs:      protoFiles,
		Args: map[string]string{
			"outDir":        android.ProtoDir(ctx).String(),
			"protoOut":      protoOut,
			"protoOutFlags": protoOutFlags,
			"protoFlags":    protoFlags,
		},
	})

	return protoFileList
}

func protoDeps(ctx android.BottomUpMutatorContext, p *android.ProtoProperties) {
	switch proptools.String(p.Proto.Type) {
	case "micro":
		ctx.AddDependency(ctx.Module(), staticLibTag, "libprotobuf-java-micro")
	case "nano":
		ctx.AddDependency(ctx.Module(), staticLibTag, "libprotobuf-java-nano")
	case "stream":
		// TODO(ccross): add dependency on protoc-gen-java-stream binary
		ctx.PropertyErrorf("proto.type", `"stream" not supported yet`)
		// No library for stream protobufs
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

func protoFlags(ctx android.ModuleContext, p *android.ProtoProperties, flags javaBuilderFlags) javaBuilderFlags {
	switch proptools.String(p.Proto.Type) {
	case "micro":
		flags.protoOutFlag = "--javamicro_out"
	case "nano":
		flags.protoOutFlag = "--javanano_out"
	case "stream":
		flags.protoOutFlag = "--javastream_out"
	case "lite", "":
		flags.protoOutFlag = "--java_out"
	default:
		ctx.PropertyErrorf("proto.type", "unknown proto type %q",
			proptools.String(p.Proto.Type))
	}
	return flags
}
