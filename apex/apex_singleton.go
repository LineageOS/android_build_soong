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
	"android/soong/android"
	"github.com/google/blueprint"
)

func init() {
	android.RegisterSingletonType("apex_depsinfo_singleton", apexDepsInfoSingletonFactory)
}

type apexDepsInfoSingleton struct {
	allowedApexDepsInfoCheckResult android.OutputPath
}

func apexDepsInfoSingletonFactory() android.Singleton {
	return &apexDepsInfoSingleton{}
}

var (
	mergeApexDepsInfoFilesRule = pctx.AndroidStaticRule("mergeApexDepsInfoFilesRule", blueprint.RuleParams{
		Command:        "cat $out.rsp | xargs cat > $out",
		Rspfile:        "$out.rsp",
		RspfileContent: "$in",
	})

	// Filter out apex dependencies that are external or safe to ignore for build determinism.
	filterApexDepsRule = pctx.AndroidStaticRule("filterApexDepsRule", blueprint.RuleParams{
		Command: "cat ${in}" +
			// Only track non-external dependencies, i.e. those that end up in the binary...
			" | grep -v '(external)'" +
			// ...and those that are safe in any apex but can be different per product.
			" | grep -v 'libgcc_stripped'" +
			" | grep -v 'libunwind_llvm'" +
			" | grep -v 'ndk_crtbegin_so.19'" +
			" | grep -v 'ndk_crtbegin_so.21'" +
			" | grep -v 'ndk_crtbegin_so.27'" +
			" | grep -v 'ndk_crtend_so.19'" +
			" | grep -v 'ndk_crtend_so.21'" +
			" | grep -v 'ndk_crtend_so.27'" +
			" | grep -v 'ndk_libunwind'" +
			" | grep -v 'prebuilt_libclang_rt.builtins-aarch64-android'" +
			" | grep -v 'prebuilt_libclang_rt.builtins-arm-android'" +
			" | grep -v 'prebuilt_libclang_rt.builtins-i686-android'" +
			" | grep -v 'prebuilt_libclang_rt.builtins-x86_64-android'" +
			" | grep -v 'libclang_rt.hwasan-aarch64-android.llndk'" +
			" > ${out}",
	})

	diffAllowedApexDepsInfoRule = pctx.AndroidStaticRule("diffAllowedApexDepsInfoRule", blueprint.RuleParams{
		// Diff two given lists while ignoring comments in the allowed deps file
		Description: "Diff ${allowed_flatlists} and ${merged_flatlists}",
		Command: `
			if grep -v '^#' ${allowed_flatlists} | diff -B ${merged_flatlists} -; then
			   touch ${out};
			else
				echo -e "\n******************************";
				echo "ERROR: go/apex-allowed-deps-error";
				echo "******************************";
				echo "Detected changes to allowed dependencies in updatable modules.";
				echo "To fix and update build/soong/apex/allowed_deps.txt, please run:";
				echo "$$ (croot && build/soong/scripts/update-apex-allowed-deps.sh)";
				echo "Members of mainline-modularization@google.com will review the changes.";
				echo -e "******************************\n";
				exit 1;
			fi;`,
	}, "allowed_flatlists", "merged_flatlists")
)

func (s *apexDepsInfoSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	modulePaths := map[string]android.Path{}
	ctx.VisitAllModules(func(module android.Module) {
		if binaryInfo, ok := module.(android.ApexBundleDepsInfoIntf); ok {
			if !binaryInfo.Updatable() {
				return
			}
			if path := binaryInfo.FlatListPath(); path.String() != "" {
				// TODO(b/159734404): don't use module.String() to sort modules/variants.
				// This is needed though, as an order of module variants may be
				// different between products.
				ms := module.String()
				if _, ok := modulePaths[ms]; !ok {
					modulePaths[ms] = path
				} else if modulePaths[ms] != path {
					ctx.Errorf("Mismatching output paths for the same module %v:\n%v\n%v", ms, modulePaths[ms], path)
				}
			}
		}
	})

	updatableFlatLists := android.Paths{}
	// Avoid non-determinism by sorting module and variation names.
	for _, key := range android.FirstUniqueStrings(android.SortedStringKeys(modulePaths)) {
		updatableFlatLists = append(updatableFlatLists, modulePaths[key])
	}

	// Merge all individual flatlists of updatable modules into a single output file
	updatableFlatListsPath := android.PathForOutput(ctx, "apex", "depsinfo", "updatable-flatlists.txt")
	ctx.Build(pctx, android.BuildParams{
		Rule:   mergeApexDepsInfoFilesRule,
		Inputs: updatableFlatLists,
		Output: updatableFlatListsPath,
	})

	// Build a filtered version of updatable flatlists without external dependencies
	filteredFlatLists := android.PathForOutput(ctx, "apex", "depsinfo", "filtered-updatable-flatlists.txt")
	ctx.Build(pctx, android.BuildParams{
		Rule:   filterApexDepsRule,
		Input:  updatableFlatListsPath,
		Output: filteredFlatLists,
	})

	// Check filtered version against allowed deps
	allowedDeps := android.ExistentPathForSource(ctx, "build/soong/apex/allowed_deps.txt").Path()
	s.allowedApexDepsInfoCheckResult = android.PathForOutput(ctx, filteredFlatLists.Rel()+".check")
	ctx.Build(pctx, android.BuildParams{
		Rule:   diffAllowedApexDepsInfoRule,
		Input:  filteredFlatLists,
		Output: s.allowedApexDepsInfoCheckResult,
		Args: map[string]string{
			"allowed_flatlists": allowedDeps.String(),
			"merged_flatlists":  filteredFlatLists.String(),
		},
	})
}

func (s *apexDepsInfoSingleton) MakeVars(ctx android.MakeVarsContext) {
	// Export check result to Make. The path is added to droidcore.
	ctx.Strict("APEX_ALLOWED_DEPS_CHECK", s.allowedApexDepsInfoCheckResult.String())
}
