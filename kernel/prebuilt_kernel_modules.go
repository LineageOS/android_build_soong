// Copyright (C) 2021 The Android Open Source Project
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

package kernel

import (
	"fmt"
	"path/filepath"
	"strings"

	"android/soong/android"
	_ "android/soong/cc/config"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	pctx.Import("android/soong/cc/config")
	registerKernelBuildComponents(android.InitRegistrationContext)
}

func registerKernelBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("prebuilt_kernel_modules", prebuiltKernelModulesFactory)
}

type prebuiltKernelModules struct {
	android.ModuleBase

	properties prebuiltKernelModulesProperties

	installDir android.InstallPath
}

type prebuiltKernelModulesProperties struct {
	// List or filegroup of prebuilt kernel module files. Should have .ko suffix.
	Srcs []string `android:"path,arch_variant"`

	// Kernel version that these modules are for. Kernel modules are installed to
	// /lib/modules/<kernel_version> directory in the corresponding partition. Default is "".
	Kernel_version *string

	// Whether this module is directly installable to one of the partitions. Default is true
	Installable *bool
}

// prebuilt_kernel_modules installs a set of prebuilt kernel module files to the correct directory.
// In addition, this module builds modules.load, modules.dep, modules.softdep and modules.alias
// using depmod and installs them as well.
func prebuiltKernelModulesFactory() android.Module {
	module := &prebuiltKernelModules{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

func (pkm *prebuiltKernelModules) installable() bool {
	return proptools.BoolDefault(pkm.properties.Installable, true)
}

func (pkm *prebuiltKernelModules) KernelVersion() string {
	return proptools.StringDefault(pkm.properties.Kernel_version, "")
}

func (pkm *prebuiltKernelModules) DepsMutator(ctx android.BottomUpMutatorContext) {
	// do nothing
}

func (pkm *prebuiltKernelModules) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if !pkm.installable() {
		pkm.SkipInstall()
	}
	modules := android.PathsForModuleSrc(ctx, pkm.properties.Srcs)

	depmodOut := runDepmod(ctx, modules)
	strippedModules := stripDebugSymbols(ctx, modules)

	installDir := android.PathForModuleInstall(ctx, "lib", "modules")
	if pkm.KernelVersion() != "" {
		installDir = installDir.Join(ctx, pkm.KernelVersion())
	}

	for _, m := range strippedModules {
		ctx.InstallFile(installDir, filepath.Base(m.String()), m)
	}
	ctx.InstallFile(installDir, "modules.load", depmodOut.modulesLoad)
	ctx.InstallFile(installDir, "modules.dep", depmodOut.modulesDep)
	ctx.InstallFile(installDir, "modules.softdep", depmodOut.modulesSoftdep)
	ctx.InstallFile(installDir, "modules.alias", depmodOut.modulesAlias)
}

var (
	pctx = android.NewPackageContext("android/soong/kernel")

	stripRule = pctx.AndroidStaticRule("strip",
		blueprint.RuleParams{
			Command:     "$stripCmd -o $out --strip-debug $in",
			CommandDeps: []string{"$stripCmd"},
		}, "stripCmd")
)

func stripDebugSymbols(ctx android.ModuleContext, modules android.Paths) android.OutputPaths {
	dir := android.PathForModuleOut(ctx, "stripped").OutputPath
	var outputs android.OutputPaths

	for _, m := range modules {
		stripped := dir.Join(ctx, filepath.Base(m.String()))
		ctx.Build(pctx, android.BuildParams{
			Rule:   stripRule,
			Input:  m,
			Output: stripped,
			Args: map[string]string{
				"stripCmd": "${config.ClangBin}/llvm-strip",
			},
		})
		outputs = append(outputs, stripped)
	}

	return outputs
}

type depmodOutputs struct {
	modulesLoad    android.OutputPath
	modulesDep     android.OutputPath
	modulesSoftdep android.OutputPath
	modulesAlias   android.OutputPath
}

func runDepmod(ctx android.ModuleContext, modules android.Paths) depmodOutputs {
	baseDir := android.PathForModuleOut(ctx, "depmod").OutputPath
	fakeVer := "0.0" // depmod demands this anyway
	modulesDir := baseDir.Join(ctx, "lib", "modules", fakeVer)

	builder := android.NewRuleBuilder(pctx, ctx)

	// Copy the module files to a temporary dir
	builder.Command().Text("rm").Flag("-rf").Text(modulesDir.String())
	builder.Command().Text("mkdir").Flag("-p").Text(modulesDir.String())
	for _, m := range modules {
		builder.Command().Text("cp").Input(m).Text(modulesDir.String())
	}

	// Enumerate modules to load
	modulesLoad := modulesDir.Join(ctx, "modules.load")
	var basenames []string
	for _, m := range modules {
		basenames = append(basenames, filepath.Base(m.String()))
	}
	builder.Command().
		Text("echo").Flag("\"" + strings.Join(basenames, " ") + "\"").
		Text("|").Text("tr").Flag("\" \"").Flag("\"\\n\"").
		Text(">").Output(modulesLoad)

	// Run depmod to build modules.dep/softdep/alias files
	modulesDep := modulesDir.Join(ctx, "modules.dep")
	modulesSoftdep := modulesDir.Join(ctx, "modules.softdep")
	modulesAlias := modulesDir.Join(ctx, "modules.alias")
	builder.Command().
		BuiltTool("depmod").
		FlagWithArg("-b ", baseDir.String()).
		Text(fakeVer).
		ImplicitOutput(modulesDep).
		ImplicitOutput(modulesSoftdep).
		ImplicitOutput(modulesAlias)

	builder.Build("depmod", fmt.Sprintf("depmod %s", ctx.ModuleName()))

	return depmodOutputs{modulesLoad, modulesDep, modulesSoftdep, modulesAlias}
}
