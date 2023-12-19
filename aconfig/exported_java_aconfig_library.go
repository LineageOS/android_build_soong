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

package aconfig

import (
	"android/soong/android"
)

func ExportedJavaDeclarationsLibraryFactory() android.Singleton {
	return &exportedJavaDeclarationsLibrarySingleton{}
}

type exportedJavaDeclarationsLibrarySingleton struct {
	intermediatePath android.OutputPath
}

func (this *exportedJavaDeclarationsLibrarySingleton) GenerateBuildActions(ctx android.SingletonContext) {
	// Find all of the aconfig_declarations modules
	var cacheFiles android.Paths
	ctx.VisitAllModules(func(module android.Module) {
		decl, ok := android.SingletonModuleProvider(ctx, module, android.AconfigDeclarationsProviderKey)
		if !ok {
			return
		}
		cacheFiles = append(cacheFiles, decl.IntermediateCacheOutputPath)
	})

	// Generate build action for aconfig
	this.intermediatePath = android.PathForIntermediates(ctx, "exported_java_aconfig_library.jar")
	ctx.Build(pctx, android.BuildParams{
		Rule:        exportedJavaRule,
		Inputs:      cacheFiles,
		Output:      this.intermediatePath,
		Description: "exported_java_aconfig_library",
		Args: map[string]string{
			"cache_files": android.JoinPathsWithPrefix(cacheFiles, " "),
		},
	})
	ctx.Phony("exported_java_aconfig_library", this.intermediatePath)
}

func (this *exportedJavaDeclarationsLibrarySingleton) MakeVars(ctx android.MakeVarsContext) {
	ctx.DistForGoalWithFilename("sdk", this.intermediatePath, "android-flags.jar")
}
