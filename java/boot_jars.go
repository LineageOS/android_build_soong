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

func init() {
	android.RegisterSingletonType("boot_jars", bootJarsSingletonFactory)
}

func bootJarsSingletonFactory() android.Singleton {
	return &bootJarsSingleton{}
}

type bootJarsSingleton struct{}

func populateMapFromConfiguredJarList(ctx android.SingletonContext, moduleToApex map[string]string, list android.ConfiguredJarList, name string) bool {
	for i := 0; i < list.Len(); i++ {
		module := list.Jar(i)
		// Ignore jacocoagent it is only added when instrumenting and so has no impact on
		// app compatibility.
		if module == "jacocoagent" {
			continue
		}
		apex := list.Apex(i)
		if existing, ok := moduleToApex[module]; ok {
			ctx.Errorf("Configuration property %q is invalid as it contains multiple references to module (%s) in APEXes (%s and %s)",
				module, existing, apex)
			return false
		}

		moduleToApex[module] = apex
	}

	return true
}

// isActiveModule returns true if the given module should be considered for boot
// jars, i.e. if it's enabled and the preferred one in case of source and
// prebuilt alternatives.
func isActiveModule(module android.Module) bool {
	if !module.Enabled() {
		return false
	}
	if module.IsReplacedByPrebuilt() {
		// A source module that has been replaced by a prebuilt counterpart.
		return false
	}
	if prebuilt, ok := module.(android.PrebuiltInterface); ok {
		if p := prebuilt.Prebuilt(); p != nil {
			return p.UsePrebuilt()
		}
	}
	return true
}

func (b *bootJarsSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	config := ctx.Config()
	if config.SkipBootJarsCheck() {
		return
	}

	// Populate a map from module name to APEX from the boot jars. If there is a
	// problem such as duplicate modules then fail and return immediately. Note
	// that both module and APEX names are tracked by base names here, so we need
	// to be careful to remove "prebuilt_" prefixes when comparing them with
	// actual modules and APEX bundles.
	moduleToApex := make(map[string]string)
	if !populateMapFromConfiguredJarList(ctx, moduleToApex, config.NonUpdatableBootJars(), "BootJars") ||
		!populateMapFromConfiguredJarList(ctx, moduleToApex, config.UpdatableBootJars(), "UpdatableBootJars") {
		return
	}

	// Map from module name to the correct apex variant.
	nameToApexVariant := make(map[string]android.Module)

	// Scan all the modules looking for the module/apex variants corresponding to the
	// boot jars.
	ctx.VisitAllModules(func(module android.Module) {
		if !isActiveModule(module) {
			return
		}

		name := android.RemoveOptionalPrebuiltPrefix(ctx.ModuleName(module))
		if apex, ok := moduleToApex[name]; ok {
			apexInfo := ctx.ModuleProvider(module, android.ApexInfoProvider).(android.ApexInfo)
			if (apex == "platform" && apexInfo.IsForPlatform()) || apexInfo.InApexByBaseName(apex) {
				// The module name/apex variant should be unique in the system but double check
				// just in case something has gone wrong.
				if existing, ok := nameToApexVariant[name]; ok {
					ctx.Errorf("found multiple variants matching %s:%s: %q and %q", apex, name, existing, module)
				}
				nameToApexVariant[name] = module
			}
		}
	})

	timestamp := android.PathForOutput(ctx, "boot-jars-package-check/stamp")

	rule := android.NewRuleBuilder(pctx, ctx)
	checkBootJars := rule.Command().BuiltTool("check_boot_jars").
		Input(ctx.Config().HostToolPath(ctx, "dexdump")).
		Input(android.PathForSource(ctx, "build/soong/scripts/check_boot_jars/package_allowed_list.txt"))

	// If this is not an unbundled build and missing dependencies are not allowed
	// then all the boot jars listed must have been found.
	strict := !config.UnbundledBuild() && !config.AllowMissingDependencies()

	// Iterate over the module names on the boot classpath in order
	for _, name := range android.SortedStringKeys(moduleToApex) {
		if apexVariant, ok := nameToApexVariant[name]; ok {
			if dep, ok := apexVariant.(interface{ DexJarBuildPath() android.Path }); ok {
				// Add the dex implementation jar for the module to be checked.
				checkBootJars.Input(dep.DexJarBuildPath())
			} else {
				ctx.Errorf("module %q is of type %q which is not supported as a boot jar", name, ctx.ModuleType(apexVariant))
			}
		} else if strict {
			ctx.Errorf("boot jars package check failed as it could not find module %q for apex %q", name, moduleToApex[name])
		}
	}

	checkBootJars.Text("&& touch").Output(timestamp)
	rule.Build("boot_jars_package_check", "check boot jar packages")

	// The check-boot-jars phony target depends on the timestamp created if the check succeeds.
	ctx.Phony("check-boot-jars", timestamp)

	// The droidcore phony target depends on the check-boot-jars phony target
	ctx.Phony("droidcore", android.PathForPhony(ctx, "check-boot-jars"))
}
