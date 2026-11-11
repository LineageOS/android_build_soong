// Copyright 2025 Google Inc. All rights reserved.
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
	"android/soong/android"
	"android/soong/cc/config"
)

func init() {
	android.RegisterParallelSingletonType("llvm_coverage_tools_zip", llvmCoverageToolsZipFactory)
}

func llvmCoverageToolsZipFactory() android.Singleton {
	return &llvmCoverageToolsZipSingleton{}
}

type llvmCoverageToolsZipSingleton struct{}

// GenerateBuildActions implements android.Singleton.
func (l *llvmCoverageToolsZipSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	if !ctx.DeviceConfig().ClangCoverageEnabled() {
		return
	}

	clangBase := config.ClangPath(ctx, "")
	llvmProfdata := config.ClangPath(ctx, "bin/llvm-profdata")
	llvmCov := config.ClangPath(ctx, "bin/llvm-cov")
	llvmLib := config.ClangToolsRuntimeLib(ctx)
	llvmCoverageToolsZip := android.PathForOutput(ctx, "llvm-profdata.zip")

	builder := android.NewRuleBuilder(pctx, ctx).SandboxDisabled()
	builder.Command().BuiltTool("soong_zip").
		Flag("-d").
		FlagWithOutput("-o ", llvmCoverageToolsZip).
		FlagWithArg("-C ", clangBase.String()).
		FlagWithInput("-f ", llvmProfdata).
		FlagWithInput("-f ", llvmLib).
		FlagWithInput("-f ", llvmCov)
	builder.Build("llvm_coverage_tools_zip", "llvm coverage tools zip")

	ctx.Phony("llvm_coverage_tools_zip", llvmCoverageToolsZip)
	ctx.DistForGoals([]string{"droidcore-unbundled", "apps_only", "llvm_coverage_tools_zip"}, llvmCoverageToolsZip)
}
