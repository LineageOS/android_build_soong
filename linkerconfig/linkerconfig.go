// Copyright (C) 2020 The Android Open Source Project
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

package linkerconfig

import (
	"android/soong/android"
	"android/soong/etc"

	"github.com/google/blueprint/proptools"
)

var (
	pctx = android.NewPackageContext("android/soong/linkerconfig")
)

func init() {
	pctx.HostBinToolVariable("conv_linker_config", "conv_linker_config")
	android.RegisterModuleType("linker_config", linkerConfigFactory)
}

type linkerConfigProperties struct {
	// source linker configuration property file
	Src *string `android:"path"`

	// If set to true, allow module to be installed to one of the partitions.
	// Default value is true.
	// Installable should be marked as false for APEX configuration to avoid
	// conflicts of configuration on /system/etc directory.
	Installable *bool
}

type linkerConfig struct {
	android.ModuleBase
	properties linkerConfigProperties

	outputFilePath android.OutputPath
	installDirPath android.InstallPath
}

// Implement PrebuiltEtcModule interface to fit in APEX prebuilt list.
var _ etc.PrebuiltEtcModule = (*linkerConfig)(nil)

func (l *linkerConfig) BaseDir() string {
	return "etc"
}

func (l *linkerConfig) SubDir() string {
	return ""
}

func (l *linkerConfig) OutputFile() android.OutputPath {
	return l.outputFilePath
}

func (l *linkerConfig) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	inputFile := android.PathForModuleSrc(ctx, android.String(l.properties.Src))
	l.outputFilePath = android.PathForModuleOut(ctx, "linker.config.pb").OutputPath
	l.installDirPath = android.PathForModuleInstall(ctx, "etc")
	linkerConfigRule := android.NewRuleBuilder(pctx, ctx)
	linkerConfigRule.Command().
		BuiltTool("conv_linker_config").
		Flag("proto").
		FlagWithInput("-s ", inputFile).
		FlagWithOutput("-o ", l.outputFilePath)
	linkerConfigRule.Build("conv_linker_config",
		"Generate linker config protobuf "+l.outputFilePath.String())

	if proptools.BoolDefault(l.properties.Installable, true) {
		ctx.InstallFile(l.installDirPath, l.outputFilePath.Base(), l.outputFilePath)
	}
}

// linker_config generates protobuf file from json file. This protobuf file will be used from
// linkerconfig while generating ld.config.txt. Format of this file can be found from
// https://android.googlesource.com/platform/system/linkerconfig/+/master/README.md
func linkerConfigFactory() android.Module {
	m := &linkerConfig{}
	m.AddProperties(&m.properties)
	android.InitAndroidArchModule(m, android.HostAndDeviceSupported, android.MultilibFirst)
	return m
}

func (l *linkerConfig) AndroidMkEntries() []android.AndroidMkEntries {
	installable := proptools.BoolDefault(l.properties.Installable, true)
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(l.outputFilePath),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", l.installDirPath.ToMakePath().String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", l.outputFilePath.Base())
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !installable)
				entries.SetString("LINKER_CONFIG_PATH_"+l.Name(), l.OutputFile().String())
			},
		},
	}}
}
