/*
 * Copyright (C) 2020 The Android Open Source Project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package apex

import (
	"github.com/google/blueprint"

	"android/soong/android"
)

func init() {
	android.RegisterSingletonType("apex_depsinfo_singleton", apexDepsInfoSingletonFactory)
}

type apexDepsInfoSingleton struct {
	// Output file with all flatlists from updatable modules' deps-info combined
	updatableFlatListsPath android.OutputPath
}

func apexDepsInfoSingletonFactory() android.Singleton {
	return &apexDepsInfoSingleton{}
}

var combineFilesRule = pctx.AndroidStaticRule("combineFilesRule",
	blueprint.RuleParams{
		Command:        "cat $out.rsp | xargs cat > $out",
		Rspfile:        "$out.rsp",
		RspfileContent: "$in",
	},
)

func (s *apexDepsInfoSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	updatableFlatLists := android.Paths{}
	ctx.VisitAllModules(func(module android.Module) {
		if binaryInfo, ok := module.(android.ApexBundleDepsInfoIntf); ok {
			if path := binaryInfo.FlatListPath(); path != nil {
				if binaryInfo.Updatable() {
					updatableFlatLists = append(updatableFlatLists, path)
				}
			}
		}
	})

	s.updatableFlatListsPath = android.PathForOutput(ctx, "apex", "depsinfo", "updatable-flatlists.txt")
	ctx.Build(pctx, android.BuildParams{
		Rule:        combineFilesRule,
		Description: "Generate " + s.updatableFlatListsPath.String(),
		Inputs:      updatableFlatLists,
		Output:      s.updatableFlatListsPath,
	})
}
