// Copyright 2019 Google Inc. All rights reserved.
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
	"fmt"
	"path/filepath"

	"android/soong/android"
	"github.com/google/blueprint"
)

func init() {
	registerPlatformCompatConfigBuildComponents(android.InitRegistrationContext)

	android.RegisterSdkMemberType(CompatConfigSdkMemberType)
}

var CompatConfigSdkMemberType = &compatConfigMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName: "compat_configs",
		SupportsSdk:  true,
	},
}

func registerPlatformCompatConfigBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterParallelSingletonType("platform_compat_config_singleton", platformCompatConfigSingletonFactory)
	ctx.RegisterModuleType("platform_compat_config", PlatformCompatConfigFactory)
	ctx.RegisterModuleType("prebuilt_platform_compat_config", prebuiltCompatConfigFactory)
	ctx.RegisterModuleType("global_compat_config", globalCompatConfigFactory)
}

var PrepareForTestWithPlatformCompatConfig = android.FixtureRegisterWithContext(registerPlatformCompatConfigBuildComponents)

func platformCompatConfigPath(ctx android.PathContext) android.OutputPath {
	return android.PathForOutput(ctx, "compat_config", "merged_compat_config.xml")
}

type platformCompatConfigProperties struct {
	Src *string `android:"path"`
}

type platformCompatConfig struct {
	android.ModuleBase

	properties     platformCompatConfigProperties
	installDirPath android.InstallPath
	configFile     android.OutputPath
	metadataFile   android.OutputPath
}

func (p *platformCompatConfig) compatConfigMetadata() android.Path {
	return p.metadataFile
}

func (p *platformCompatConfig) CompatConfig() android.OutputPath {
	return p.configFile
}

func (p *platformCompatConfig) SubDir() string {
	return "compatconfig"
}

type platformCompatConfigMetadataProvider interface {
	compatConfigMetadata() android.Path
}

type PlatformCompatConfigIntf interface {
	android.Module

	CompatConfig() android.OutputPath
	// Sub dir under etc dir.
	SubDir() string
}

var _ PlatformCompatConfigIntf = (*platformCompatConfig)(nil)
var _ platformCompatConfigMetadataProvider = (*platformCompatConfig)(nil)

func (p *platformCompatConfig) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	rule := android.NewRuleBuilder(pctx, ctx)

	configFileName := p.Name() + ".xml"
	metadataFileName := p.Name() + "_meta.xml"
	p.configFile = android.PathForModuleOut(ctx, configFileName).OutputPath
	p.metadataFile = android.PathForModuleOut(ctx, metadataFileName).OutputPath
	path := android.PathForModuleSrc(ctx, String(p.properties.Src))

	rule.Command().
		BuiltTool("process-compat-config").
		FlagWithInput("--jar ", path).
		FlagWithOutput("--device-config ", p.configFile).
		FlagWithOutput("--merged-config ", p.metadataFile)

	p.installDirPath = android.PathForModuleInstall(ctx, "etc", "compatconfig")
	rule.Build(configFileName, "Extract compat/compat_config.xml and install it")

}

func (p *platformCompatConfig) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(p.configFile),
		Include:    "$(BUILD_PREBUILT)",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", p.installDirPath.String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", p.configFile.Base())
			},
		},
	}}
}

func PlatformCompatConfigFactory() android.Module {
	module := &platformCompatConfig{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

type compatConfigMemberType struct {
	android.SdkMemberTypeBase
}

func (b *compatConfigMemberType) AddDependencies(ctx android.SdkDependencyContext, dependencyTag blueprint.DependencyTag, names []string) {
	ctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (b *compatConfigMemberType) IsInstance(module android.Module) bool {
	_, ok := module.(*platformCompatConfig)
	return ok
}

func (b *compatConfigMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	return ctx.SnapshotBuilder().AddPrebuiltModule(member, "prebuilt_platform_compat_config")
}

func (b *compatConfigMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &compatConfigSdkMemberProperties{}
}

type compatConfigSdkMemberProperties struct {
	android.SdkMemberPropertiesBase

	Metadata android.Path
}

func (b *compatConfigSdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	module := variant.(*platformCompatConfig)
	b.Metadata = module.metadataFile
}

func (b *compatConfigSdkMemberProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	builder := ctx.SnapshotBuilder()
	if b.Metadata != nil {
		snapshotRelativePath := filepath.Join("compat_configs", ctx.Name(), b.Metadata.Base())
		builder.CopyToSnapshot(b.Metadata, snapshotRelativePath)
		propertySet.AddProperty("metadata", snapshotRelativePath)
	}
}

var _ android.SdkMemberType = (*compatConfigMemberType)(nil)

// A prebuilt version of the platform compat config module.
type prebuiltCompatConfigModule struct {
	android.ModuleBase
	prebuilt android.Prebuilt

	properties prebuiltCompatConfigProperties

	metadataFile android.Path
}

type prebuiltCompatConfigProperties struct {
	Metadata *string `android:"path"`
}

func (module *prebuiltCompatConfigModule) Prebuilt() *android.Prebuilt {
	return &module.prebuilt
}

func (module *prebuiltCompatConfigModule) Name() string {
	return module.prebuilt.Name(module.ModuleBase.Name())
}

func (module *prebuiltCompatConfigModule) compatConfigMetadata() android.Path {
	return module.metadataFile
}

var _ platformCompatConfigMetadataProvider = (*prebuiltCompatConfigModule)(nil)

func (module *prebuiltCompatConfigModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	module.metadataFile = module.prebuilt.SingleSourcePath(ctx)
}

// A prebuilt version of platform_compat_config that provides the metadata.
func prebuiltCompatConfigFactory() android.Module {
	m := &prebuiltCompatConfigModule{}
	m.AddProperties(&m.properties)
	android.InitSingleSourcePrebuiltModule(m, &m.properties, "Metadata")
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibCommon)
	return m
}

// compat singleton rules
type platformCompatConfigSingleton struct {
	metadata android.Path
}

func (p *platformCompatConfigSingleton) GenerateBuildActions(ctx android.SingletonContext) {

	var compatConfigMetadata android.Paths

	ctx.VisitAllModules(func(module android.Module) {
		if !module.Enabled() {
			return
		}
		if c, ok := module.(platformCompatConfigMetadataProvider); ok {
			if !android.IsModulePreferred(module) {
				return
			}
			metadata := c.compatConfigMetadata()
			compatConfigMetadata = append(compatConfigMetadata, metadata)
		}
	})

	if compatConfigMetadata == nil {
		// nothing to do.
		return
	}

	rule := android.NewRuleBuilder(pctx, ctx)
	outputPath := platformCompatConfigPath(ctx)

	rule.Command().
		BuiltTool("process-compat-config").
		FlagForEachInput("--xml ", compatConfigMetadata).
		FlagWithOutput("--merged-config ", outputPath)

	rule.Build("merged-compat-config", "Merge compat config")

	p.metadata = outputPath
}

func (p *platformCompatConfigSingleton) MakeVars(ctx android.MakeVarsContext) {
	if p.metadata != nil {
		ctx.Strict("INTERNAL_PLATFORM_MERGED_COMPAT_CONFIG", p.metadata.String())
	}
}

func platformCompatConfigSingletonFactory() android.Singleton {
	return &platformCompatConfigSingleton{}
}

// ============== merged_compat_config =================
type globalCompatConfigProperties struct {
	// name of the file into which the metadata will be copied.
	Filename *string
}

type globalCompatConfig struct {
	android.ModuleBase

	properties globalCompatConfigProperties

	outputFilePath android.OutputPath
}

func (c *globalCompatConfig) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	filename := String(c.properties.Filename)

	inputPath := platformCompatConfigPath(ctx)
	c.outputFilePath = android.PathForModuleOut(ctx, filename).OutputPath

	// This ensures that outputFilePath has the correct name for others to
	// use, as the source file may have a different name.
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Output: c.outputFilePath,
		Input:  inputPath,
	})
}

func (h *globalCompatConfig) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{h.outputFilePath}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

// global_compat_config provides access to the merged compat config xml file generated by the build.
func globalCompatConfigFactory() android.Module {
	module := &globalCompatConfig{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibCommon)
	return module
}
