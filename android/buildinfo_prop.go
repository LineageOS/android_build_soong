// Copyright 2022 Google Inc. All rights reserved.
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
	"fmt"

	"github.com/google/blueprint/proptools"
)

func init() {
	ctx := InitRegistrationContext
	ctx.RegisterModuleType("buildinfo_prop", buildinfoPropFactory)
}

type buildinfoPropProperties struct {
	// Whether this module is directly installable to one of the partitions. Default: true.
	Installable *bool

	Product_config *string `android:"path"`
}

type buildinfoPropModule struct {
	ModuleBase

	properties buildinfoPropProperties

	outputFilePath OutputPath
	installPath    InstallPath
}

var _ OutputFileProducer = (*buildinfoPropModule)(nil)

func (p *buildinfoPropModule) installable() bool {
	return proptools.BoolDefault(p.properties.Installable, true)
}

// OutputFileProducer
func (p *buildinfoPropModule) OutputFiles(tag string) (Paths, error) {
	if tag != "" {
		return nil, fmt.Errorf("unsupported tag %q", tag)
	}
	return Paths{p.outputFilePath}, nil
}

func shouldAddBuildThumbprint(config Config) bool {
	knownOemProperties := []string{
		"ro.product.brand",
		"ro.product.name",
		"ro.product.device",
	}

	for _, knownProp := range knownOemProperties {
		if InList(knownProp, config.OemProperties()) {
			return true
		}
	}
	return false
}

func (p *buildinfoPropModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	if ctx.ModuleName() != "buildinfo.prop" || ctx.ModuleDir() != "build/soong" {
		ctx.ModuleErrorf("There can only be one buildinfo_prop module in build/soong")
		return
	}
	p.outputFilePath = PathForModuleOut(ctx, p.Name()).OutputPath
	if !ctx.Config().KatiEnabled() {
		WriteFileRule(ctx, p.outputFilePath, "# no buildinfo.prop if kati is disabled")
		return
	}

	rule := NewRuleBuilder(pctx, ctx)

	config := ctx.Config()

	cmd := rule.Command().BuiltTool("buildinfo")

	cmd.FlagWithInput("--build-hostname-file=", config.BuildHostnameFile(ctx))
	// Note: depending on BuildNumberFile will cause the build.prop file to be rebuilt
	// every build, but that's intentional.
	cmd.FlagWithInput("--build-number-file=", config.BuildNumberFile(ctx))
	// Export build thumbprint only if the product has specified at least one oem fingerprint property
	// b/17888863
	if shouldAddBuildThumbprint(config) {
		// In the previous make implementation, a dependency was not added on the thumbprint file
		cmd.FlagWithArg("--build-thumbprint-file=", config.BuildThumbprintFile(ctx).String())
	}
	cmd.FlagWithArg("--build-username=", config.Getenv("BUILD_USERNAME"))
	// Technically we should also have a dependency on BUILD_DATETIME_FILE,
	// but it can be either an absolute or relative path, which is hard to turn into
	// a Path object. So just rely on the BuildNumberFile always changing to cause
	// us to rebuild.
	cmd.FlagWithArg("--date-file=", ctx.Config().Getenv("BUILD_DATETIME_FILE"))
	cmd.FlagWithInput("--platform-preview-sdk-fingerprint-file=", ApiFingerprintPath(ctx))
	cmd.FlagWithInput("--product-config=", PathForModuleSrc(ctx, proptools.String(p.properties.Product_config)))
	cmd.FlagWithOutput("--out=", p.outputFilePath)

	rule.Build(ctx.ModuleName(), "generating buildinfo props")

	if !p.installable() {
		p.SkipInstall()
	}

	p.installPath = PathForModuleInstall(ctx)
	ctx.InstallFile(p.installPath, p.Name(), p.outputFilePath)
}

func (p *buildinfoPropModule) AndroidMkEntries() []AndroidMkEntries {
	return []AndroidMkEntries{{
		Class:      "ETC",
		OutputFile: OptionalPathForPath(p.outputFilePath),
		ExtraEntries: []AndroidMkExtraEntriesFunc{
			func(ctx AndroidMkExtraEntriesContext, entries *AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", p.installPath.String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", p.outputFilePath.Base())
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !p.installable())
			},
		},
	}}
}

// buildinfo_prop module generates a build.prop file, which contains a set of common
// system/build.prop properties, such as ro.build.version.*.  Not all properties are implemented;
// currently this module is only for microdroid.
func buildinfoPropFactory() Module {
	module := &buildinfoPropModule{}
	module.AddProperties(&module.properties)
	InitAndroidModule(module)
	return module
}
