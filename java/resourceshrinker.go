// Copyright 2022 Google Inc. All rights reserved.
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

	"github.com/google/blueprint"
)

var shrinkResources = pctx.AndroidStaticRule("shrinkResources",
	blueprint.RuleParams{
		Command:     `${config.ResourceShrinkerCmd} --output $out --input $in --raw_resources $raw_resources`,
		CommandDeps: []string{"${config.ResourceShrinkerCmd}"},
	}, "raw_resources")

func ShrinkResources(ctx android.ModuleContext, apk android.Path, outputFile android.WritablePath) {
	protoFile := android.PathForModuleOut(ctx, apk.Base()+".proto.apk")
	aapt2Convert(ctx, protoFile, apk, "proto")
	strictModeFile := android.PathForSource(ctx, "prebuilts/cmdline-tools/shrinker.xml")
	protoOut := android.PathForModuleOut(ctx, apk.Base()+".proto.out.apk")
	ctx.Build(pctx, android.BuildParams{
		Rule:   shrinkResources,
		Input:  protoFile,
		Output: protoOut,
		Args: map[string]string{
			"raw_resources": strictModeFile.String(),
		},
	})
	aapt2Convert(ctx, outputFile, protoOut, "binary")
}
