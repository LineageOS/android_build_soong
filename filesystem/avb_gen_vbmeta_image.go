// Copyright (C) 2022 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package filesystem

import (
	"fmt"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

type avbGenVbmetaImage struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties avbGenVbmetaImageProperties

	output     android.OutputPath
	installDir android.InstallPath
}

type avbGenVbmetaImageProperties struct {
	// Source file of this image. Can reference a genrule type module with the ":module" syntax.
	Src *string `android:"path,arch_variant"`

	// Name of the image partition. Defaults to the name of this module.
	Partition_name *string

	// The salt in hex. Required for reproducible builds.
	Salt *string
}

// The avbGenVbmetaImage generates an unsigned VBMeta image output for the given image.
func avbGenVbmetaImageFactory() android.Module {
	module := &avbGenVbmetaImage{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	android.InitDefaultableModule(module)
	return module
}

func (a *avbGenVbmetaImage) installFileName() string {
	return a.Name() + ".img"
}

func (a *avbGenVbmetaImage) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	builder := android.NewRuleBuilder(pctx, ctx)
	cmd := builder.Command().BuiltTool("avbtool").Text("add_hash_footer")
	cmd.Flag("--dynamic_partition_size")
	cmd.Flag("--do_not_append_vbmeta_image")

	partition_name := proptools.StringDefault(a.properties.Partition_name, a.Name())
	cmd.FlagWithArg("--partition_name ", partition_name)

	if a.properties.Src == nil {
		ctx.PropertyErrorf("src", "missing source file")
		return
	}
	input := android.PathForModuleSrc(ctx, proptools.String(a.properties.Src))
	cmd.FlagWithInput("--image ", input)

	if a.properties.Salt == nil {
		ctx.PropertyErrorf("salt", "missing salt value")
		return
	}
	cmd.FlagWithArg("--salt ", proptools.String(a.properties.Salt))

	a.output = android.PathForModuleOut(ctx, a.installFileName()).OutputPath
	cmd.FlagWithOutput("--output_vbmeta_image ", a.output)
	builder.Build("avbGenVbmetaImage", fmt.Sprintf("avbGenVbmetaImage %s", ctx.ModuleName()))
}

var _ android.AndroidMkEntriesProvider = (*avbGenVbmetaImage)(nil)

// Implements android.AndroidMkEntriesProvider
func (a *avbGenVbmetaImage) AndroidMkEntries() []android.AndroidMkEntries {
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

var _ android.OutputFileProducer = (*avbGenVbmetaImage)(nil)

// Implements android.OutputFileProducer
func (a *avbGenVbmetaImage) OutputFiles(tag string) (android.Paths, error) {
	if tag == "" {
		return []android.Path{a.output}, nil
	}
	return nil, fmt.Errorf("unsupported module reference tag %q", tag)
}

type avbGenVbmetaImageDefaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

// avb_gen_vbmeta_image_defaults provides a set of properties that can be inherited by other
// avb_gen_vbmeta_image modules. A module can use the properties from an
// avb_gen_vbmeta_image_defaults using `defaults: ["<:default_module_name>"]`. Properties of both
// modules are erged (when possible) by prepending the default module's values to the depending
// module's values.
func avbGenVbmetaImageDefaultsFactory() android.Module {
	module := &avbGenVbmetaImageDefaults{}
	module.AddProperties(&avbGenVbmetaImageProperties{})
	android.InitDefaultsModule(module)
	return module
}
