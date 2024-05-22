// Copyright 2023 Google Inc. All rights reserved.
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

package build_flags

import (
	"android/soong/android"
)

// A singleton module that collects all of the build flags declared in the
// tree into a single combined file for export to the external flag setting
// server (inside Google it's Gantry).
//
// Note that this is ALL build_declarations modules present in the tree, not just
// ones that are relevant to the product currently being built, so that that infra
// doesn't need to pull from multiple builds and merge them.
func AllBuildFlagDeclarationsFactory() android.Singleton {
	return &allBuildFlagDeclarationsSingleton{}
}

type allBuildFlagDeclarationsSingleton struct {
	intermediateBinaryProtoPath android.OutputPath
	intermediateTextProtoPath   android.OutputPath
}

func (this *allBuildFlagDeclarationsSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	// Find all of the build_flag_declarations modules
	var intermediateFiles android.Paths
	ctx.VisitAllModules(func(module android.Module) {
		decl, ok := android.SingletonModuleProvider(ctx, module, BuildFlagDeclarationsProviderKey)
		if !ok {
			return
		}
		intermediateFiles = append(intermediateFiles, decl.IntermediateCacheOutputPath)
	})

	// Generate build action for build_flag (binary proto output)
	this.intermediateBinaryProtoPath = android.PathForIntermediates(ctx, "all_build_flag_declarations.pb")
	ctx.Build(pctx, android.BuildParams{
		Rule:        allDeclarationsRule,
		Inputs:      intermediateFiles,
		Output:      this.intermediateBinaryProtoPath,
		Description: "all_build_flag_declarations",
		Args: map[string]string{
			"intermediates": android.JoinPathsWithPrefix(intermediateFiles, "--intermediate "),
		},
	})
	ctx.Phony("all_build_flag_declarations", this.intermediateBinaryProtoPath)

	// Generate build action for build_flag (text proto output)
	this.intermediateTextProtoPath = android.PathForIntermediates(ctx, "all_build_flag_declarations.textproto")
	ctx.Build(pctx, android.BuildParams{
		Rule:        allDeclarationsRuleTextProto,
		Input:       this.intermediateBinaryProtoPath,
		Output:      this.intermediateTextProtoPath,
		Description: "all_build_flag_declarations_textproto",
	})
	ctx.Phony("all_build_flag_declarations_textproto", this.intermediateTextProtoPath)
}

func (this *allBuildFlagDeclarationsSingleton) MakeVars(ctx android.MakeVarsContext) {
	ctx.DistForGoal("droid", this.intermediateBinaryProtoPath)
	for _, goal := range []string{"docs", "droid", "sdk"} {
		ctx.DistForGoalWithFilename(goal, this.intermediateBinaryProtoPath, "build_flags/all_flags.pb")
		ctx.DistForGoalWithFilename(goal, this.intermediateTextProtoPath, "build_flags/all_flags.textproto")
	}
}
