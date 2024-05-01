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

package java

import (
	"android/soong/android"
)

// isActiveModule returns true if the given module should be considered for boot
// jars, i.e. if it's enabled and the preferred one in case of source and
// prebuilt alternatives.
func isActiveModule(module android.Module) bool {
	if !module.Enabled() {
		return false
	}
	return android.IsModulePreferred(module)
}

// buildRuleForBootJarsPackageCheck generates the build rule to perform the boot jars package
// check.
func buildRuleForBootJarsPackageCheck(ctx android.ModuleContext, bootDexJarByModule bootDexJarByModule) {
	bootDexJars := bootDexJarByModule.bootDexJarsWithoutCoverage()
	if len(bootDexJars) == 0 {
		return
	}

	timestamp := android.PathForOutput(ctx, "boot-jars-package-check/stamp")

	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().BuiltTool("check_boot_jars").
		Input(ctx.Config().HostToolPath(ctx, "dexdump")).
		Input(android.PathForSource(ctx, "build/soong/scripts/check_boot_jars/package_allowed_list.txt")).
		Inputs(bootDexJars).
		Text("&& touch").Output(timestamp)
	rule.Build("boot_jars_package_check", "check boot jar packages")

	// The check-boot-jars phony target depends on the timestamp created if the check succeeds.
	ctx.Phony("check-boot-jars", timestamp)

	// The droidcore phony target depends on the check-boot-jars phony target
	ctx.Phony("droidcore", android.PathForPhony(ctx, "check-boot-jars"))
}
