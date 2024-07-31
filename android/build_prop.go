// Copyright 2024 Google Inc. All rights reserved.
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

package android

import (
	"github.com/google/blueprint/proptools"
)

func init() {
	ctx := InitRegistrationContext
	ctx.RegisterModuleType("build_prop", buildPropFactory)
}

type buildPropProperties struct {
	// Output file name. Defaults to "build.prop"
	Stem *string

	// List of prop names to exclude. This affects not only common build properties but also
	// properties in prop_files.
	Block_list []string

	// Path to the input prop files. The contents of the files are directly
	// emitted to the output
	Prop_files []string `android:"path"`

	// Files to be appended at the end of build.prop. These files are appended after
	// post_process_props without any further checking.
	Footer_files []string `android:"path"`

	// Path to a JSON file containing product configs.
	Product_config *string `android:"path"`
}

type buildPropModule struct {
	ModuleBase

	properties buildPropProperties

	outputFilePath OutputPath
	installPath    InstallPath
}

func (p *buildPropModule) stem() string {
	return proptools.StringDefault(p.properties.Stem, "build.prop")
}

func (p *buildPropModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	p.outputFilePath = PathForModuleOut(ctx, "build.prop").OutputPath
	if !ctx.Config().KatiEnabled() {
		WriteFileRule(ctx, p.outputFilePath, "# no build.prop if kati is disabled")
		ctx.SetOutputFiles(Paths{p.outputFilePath}, "")
		return
	}

	partition := p.PartitionTag(ctx.DeviceConfig())
	if partition != "system" {
		ctx.PropertyErrorf("partition", "unsupported partition %q: only \"system\" is supported", partition)
		return
	}

	rule := NewRuleBuilder(pctx, ctx)

	config := ctx.Config()

	cmd := rule.Command().BuiltTool("gen_build_prop")

	cmd.FlagWithInput("--build-hostname-file=", config.BuildHostnameFile(ctx))
	cmd.FlagWithInput("--build-number-file=", config.BuildNumberFile(ctx))
	// shouldn't depend on BuildFingerprintFile and BuildThumbprintFile to prevent from rebuilding
	// on every incremental build.
	cmd.FlagWithArg("--build-fingerprint-file=", config.BuildFingerprintFile(ctx).String())
	// Export build thumbprint only if the product has specified at least one oem fingerprint property
	// b/17888863
	if shouldAddBuildThumbprint(config) {
		// In the previous make implementation, a dependency was not added on the thumbprint file
		cmd.FlagWithArg("--build-thumbprint-file=", config.BuildThumbprintFile(ctx).String())
	}
	cmd.FlagWithArg("--build-username=", config.Getenv("BUILD_USERNAME"))
	// shouldn't depend on BUILD_DATETIME_FILE to prevent from rebuilding on every incremental
	// build.
	cmd.FlagWithArg("--date-file=", ctx.Config().Getenv("BUILD_DATETIME_FILE"))
	cmd.FlagWithInput("--platform-preview-sdk-fingerprint-file=", ApiFingerprintPath(ctx))
	cmd.FlagWithInput("--product-config=", PathForModuleSrc(ctx, proptools.String(p.properties.Product_config)))
	cmd.FlagWithArg("--partition=", partition)
	cmd.FlagWithOutput("--out=", p.outputFilePath)

	postProcessCmd := rule.Command().BuiltTool("post_process_props")
	if ctx.DeviceConfig().BuildBrokenDupSysprop() {
		postProcessCmd.Flag("--allow-dup")
	}
	postProcessCmd.FlagWithArg("--sdk-version ", config.PlatformSdkVersion().String())
	postProcessCmd.FlagWithInput("--kernel-version-file-for-uffd-gc ", PathForOutput(ctx, "dexpreopt/kernel_version_for_uffd_gc.txt"))
	postProcessCmd.Text(p.outputFilePath.String())
	postProcessCmd.Flags(p.properties.Block_list)

	rule.Command().Text("echo").Text(proptools.NinjaAndShellEscape("# end of file")).FlagWithArg(">> ", p.outputFilePath.String())

	rule.Build(ctx.ModuleName(), "generating build.prop")

	p.installPath = PathForModuleInstall(ctx)
	ctx.InstallFile(p.installPath, p.stem(), p.outputFilePath)

	ctx.SetOutputFiles(Paths{p.outputFilePath}, "")
}

// build_prop module generates {partition}/build.prop file. At first common build properties are
// printed based on Soong config variables. And then prop_files are printed as-is. Finally,
// post_process_props tool is run to check if the result build.prop is valid or not.
func buildPropFactory() Module {
	module := &buildPropModule{}
	module.AddProperties(&module.properties)
	InitAndroidArchModule(module, DeviceSupported, MultilibCommon)
	return module
}
