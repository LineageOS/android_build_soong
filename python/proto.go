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

package python

import (
	"android/soong/android"
)

func genProto(ctx android.ModuleContext, protoFile android.Path, flags android.ProtoFlags) android.Path {
	// Using protoFile.Base() would generate duplicate source errors in some cases, so we use Rel() instead
	srcsZipFile := android.PathForModuleGen(ctx, protoFile.Rel()+".srcszip")

	outDir := srcsZipFile.ReplaceExtension(ctx, "tmp")
	depFile := srcsZipFile.ReplaceExtension(ctx, "srcszip.d")

	rule := android.NewRuleBuilder(pctx, ctx)

	rule.Command().Text("rm -rf").Flag(outDir.String())
	rule.Command().Text("mkdir -p").Flag(outDir.String())

	android.ProtoRule(rule, protoFile, flags, flags.Deps, outDir, depFile, nil)

	// Proto generated python files have an unknown package name in the path, so package the entire output directory
	// into a srcszip.
	zipCmd := rule.Command().
		BuiltTool("soong_zip").
		FlagWithOutput("-o ", srcsZipFile)
	zipCmd.FlagWithArg("-C ", outDir.String()).
		FlagWithArg("-D ", outDir.String())

	rule.Command().Text("rm -rf").Flag(outDir.String())

	rule.Build("protoc_"+protoFile.Rel(), "protoc "+protoFile.Rel())

	return srcsZipFile
}
