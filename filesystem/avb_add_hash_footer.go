// Copyright (C) 2022 The Android Open Source Project
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

package filesystem

import (
	"fmt"
	"strconv"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

type avbAddHashFooter struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties avbAddHashFooterProperties

	output     android.OutputPath
	installDir android.InstallPath
}

type avbProp struct {
	// Name of a property
	Name *string

	// Value of a property. Can't be used together with `file`.
	Value *string

	// File from which the value of the prop is read from. Can't be used together with `value`.
	File *string `android:"path,arch_variant"`
}

type avbAddHashFooterProperties struct {
	// Source file of this image. Can reference a genrule type module with the ":module" syntax.
	Src *string `android:"path,arch_variant"`

	// Set the name of the output. Defaults to <module_name>.img.
	Filename *string

	// Name of the image partition. Defaults to the name of this module.
	Partition_name *string

	// Size of the partition. Defaults to dynamically calculating the size.
	Partition_size *int64

	// Path to the private key that avbtool will use to sign this image.
	Private_key *string `android:"path"`

	// Algorithm that avbtool will use to sign this image. Default is SHA256_RSA4096.
	Algorithm *string

	// The salt in hex. Required for reproducible builds.
	Salt *string

	// List of properties to add to the footer
	Props []avbProp

	// The index used to prevent rollback of the image on device.
	Rollback_index *int64

	// Include descriptors from images
	Include_descriptors_from_images []string `android:"path,arch_variant"`
}

// The AVB footer adds verification information to the image.
func avbAddHashFooterFactory() android.Module {
	module := &avbAddHashFooter{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

func (a *avbAddHashFooter) installFileName() string {
	return proptools.StringDefault(a.properties.Filename, a.BaseModuleName()+".img")
}

func (a *avbAddHashFooter) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	builder := android.NewRuleBuilder(pctx, ctx)

	if a.properties.Src == nil {
		ctx.PropertyErrorf("src", "missing source file")
		return
	}
	input := android.PathForModuleSrc(ctx, proptools.String(a.properties.Src))
	a.output = android.PathForModuleOut(ctx, a.installFileName()).OutputPath
	builder.Command().Text("cp").Input(input).Output(a.output)

	cmd := builder.Command().BuiltTool("avbtool").Text("add_hash_footer")

	partition_name := proptools.StringDefault(a.properties.Partition_name, a.BaseModuleName())
	cmd.FlagWithArg("--partition_name ", partition_name)

	if a.properties.Partition_size == nil {
		cmd.Flag("--dynamic_partition_size")
	} else {
		partition_size := proptools.Int(a.properties.Partition_size)
		cmd.FlagWithArg("--partition_size ", strconv.Itoa(partition_size))
	}

	key := android.PathForModuleSrc(ctx, proptools.String(a.properties.Private_key))
	cmd.FlagWithInput("--key ", key)

	algorithm := proptools.StringDefault(a.properties.Algorithm, "SHA256_RSA4096")
	cmd.FlagWithArg("--algorithm ", algorithm)

	if a.properties.Salt == nil {
		ctx.PropertyErrorf("salt", "missing salt value")
		return
	}
	cmd.FlagWithArg("--salt ", proptools.String(a.properties.Salt))

	imagePaths := android.PathsForModuleSrc(ctx, a.properties.Include_descriptors_from_images)
	for _, imagePath := range imagePaths {
		cmd.FlagWithInput("--include_descriptors_from_image ", imagePath)
	}

	for _, prop := range a.properties.Props {
		addAvbProp(ctx, cmd, prop)
	}

	if a.properties.Rollback_index != nil {
		rollbackIndex := proptools.Int(a.properties.Rollback_index)
		if rollbackIndex < 0 {
			ctx.PropertyErrorf("rollback_index", "Rollback index must be non-negative")
		}
		cmd.Flag(fmt.Sprintf(" --rollback_index %d", rollbackIndex))
	}

	cmd.FlagWithOutput("--image ", a.output)

	builder.Build("avbAddHashFooter", fmt.Sprintf("avbAddHashFooter %s", ctx.ModuleName()))

	a.installDir = android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(a.installDir, a.installFileName(), a.output)
}

func addAvbProp(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, prop avbProp) {
	name := proptools.String(prop.Name)
	value := proptools.String(prop.Value)
	file := proptools.String(prop.File)
	if name == "" {
		ctx.PropertyErrorf("name", "can't be empty")
		return
	}
	if value == "" && file == "" {
		ctx.PropertyErrorf("value", "either value or file should be set")
		return
	}
	if value != "" && file != "" {
		ctx.PropertyErrorf("value", "value and file can't be set at the same time")
		return
	}

	if value != "" {
		cmd.FlagWithArg("--prop ", proptools.ShellEscape(fmt.Sprintf("%s:%s", name, value)))
	} else {
		p := android.PathForModuleSrc(ctx, file)
		cmd.Implicit(p)
		cmd.FlagWithArg("--prop_from_file ", proptools.ShellEscape(fmt.Sprintf("%s:%s", name, cmd.PathForInput(p))))
	}
}

var _ android.AndroidMkEntriesProvider = (*avbAddHashFooter)(nil)

// Implements android.AndroidMkEntriesProvider
func (a *avbAddHashFooter) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(a.output),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", a.installDir.String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", a.installFileName())
			},
		},
	}}
}

var _ Filesystem = (*avbAddHashFooter)(nil)

func (a *avbAddHashFooter) OutputPath() android.Path {
	return a.output
}

func (a *avbAddHashFooter) SignedOutputPath() android.Path {
	return a.OutputPath() // always signed
}

// TODO(b/185115783): remove when not needed as input to a prebuilt_etc rule
var _ android.SourceFileProducer = (*avbAddHashFooter)(nil)

// Implements android.SourceFileProducer
func (a *avbAddHashFooter) Srcs() android.Paths {
	return append(android.Paths{}, a.output)
}

type avbAddHashFooterDefaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

// avb_add_hash_footer_defaults provides a set of properties that can be inherited by other
// avb_add_hash_footer modules. A module can use the properties from an avb_add_hash_footer_defaults
// using `defaults: ["<:default_module_name>"]`. Properties of both modules are erged (when
// possible) by prepending the default module's values to the depending module's values.
func avbAddHashFooterDefaultsFactory() android.Module {
	module := &avbAddHashFooterDefaults{}
	module.AddProperties(&avbAddHashFooterProperties{})
	android.InitDefaultsModule(module)
	return module
}
