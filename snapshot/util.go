// Copyright 2021 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package snapshot

import "android/soong/android"

func WriteStringToFileRule(ctx android.SingletonContext, content, out string) android.OutputPath {
	outPath := android.PathForOutput(ctx, out)
	android.WriteFileRule(ctx, outPath, content)
	return outPath
}

func CopyFileRule(pctx android.PackageContext, ctx android.SingletonContext, path android.Path, out string) android.OutputPath {
	outPath := android.PathForOutput(ctx, out)
	ctx.Build(pctx, android.BuildParams{
		Rule:        android.Cp,
		Input:       path,
		Output:      outPath,
		Description: "copy " + path.String() + " -> " + out,
		Args: map[string]string{
			"cpFlags": "-f -L",
		},
	})
	return outPath
}

// zip snapshot
func zipSnapshot(ctx android.SingletonContext, dir string, baseName string, snapshotOutputs android.Paths) android.OptionalPath {
	zipPath := android.PathForOutput(
		ctx, dir, baseName+".zip")

	zipRule := android.NewRuleBuilder(pctx, ctx)
	rspFile := android.PathForOutput(
		ctx, dir, baseName+"_list.rsp")

	zipRule.Command().
		BuiltTool("soong_zip").
		FlagWithOutput("-o ", zipPath).
		FlagWithArg("-C ", android.PathForOutput(ctx, dir).String()).
		FlagWithRspFileInputList("-r ", rspFile, snapshotOutputs)

	zipRule.Build(zipPath.String(), baseName+" snapshot "+zipPath.String())
	return android.OptionalPathForPath(zipPath)
}
