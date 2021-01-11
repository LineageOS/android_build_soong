// Copyright 2020 Google Inc. All rights reserved.
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

package bp2build

import (
	"android/soong/android"
	"os"
)

// The Bazel bp2build singleton is responsible for writing .bzl files that are equivalent to
// Android.bp files that are capable of being built with Bazel.
func init() {
	android.RegisterBazelConverterPreSingletonType("androidbp_to_build", AndroidBpToBuildSingleton)
}

func AndroidBpToBuildSingleton() android.Singleton {
	return &androidBpToBuildSingleton{
		name: "bp2build",
	}
}

type androidBpToBuildSingleton struct {
	name      string
	outputDir android.OutputPath
}

func (s *androidBpToBuildSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	s.outputDir = android.PathForOutput(ctx, s.name)
	android.RemoveAllOutputDir(s.outputDir)

	if !ctx.Config().IsEnvTrue("CONVERT_TO_BAZEL") {
		return
	}

	ruleShims := CreateRuleShims(android.ModuleTypeFactories())

	buildToTargets := GenerateSoongModuleTargets(ctx)

	filesToWrite := CreateBazelFiles(ruleShims, buildToTargets)
	for _, f := range filesToWrite {
		if err := s.writeFile(ctx, f); err != nil {
			ctx.Errorf("Failed to write %q (dir %q) due to %q", f.Basename, f.Dir, err)
		}
	}
}

func (s *androidBpToBuildSingleton) getOutputPath(ctx android.PathContext, dir string) android.OutputPath {
	return s.outputDir.Join(ctx, dir)
}

func (s *androidBpToBuildSingleton) writeFile(ctx android.PathContext, f BazelFile) error {
	return writeReadOnlyFile(ctx, s.getOutputPath(ctx, f.Dir), f.Basename, f.Contents)
}

// The auto-conversion directory should be read-only, sufficient for bazel query. The files
// are not intended to be edited by end users.
func writeReadOnlyFile(ctx android.PathContext, dir android.OutputPath, baseName, content string) error {
	android.CreateOutputDirIfNonexistent(dir, os.ModePerm)
	pathToFile := dir.Join(ctx, baseName)

	// 0444 is read-only
	err := android.WriteFileToOutputDir(pathToFile, []byte(content), 0444)

	return err
}
