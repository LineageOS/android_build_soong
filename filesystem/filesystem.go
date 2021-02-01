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

package filesystem

import (
	"fmt"

	"android/soong/android"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	android.RegisterModuleType("android_filesystem", filesystemFactory)
}

type filesystem struct {
	android.ModuleBase
	android.PackagingBase

	properties filesystemProperties

	output     android.OutputPath
	installDir android.InstallPath
}

type filesystemProperties struct {
	// When set to true, sign the image with avbtool. Default is false.
	Use_avb *bool

	// Path to the private key that avbtool will use to sign this filesystem image.
	// TODO(jiyong): allow apex_key to be specified here
	Avb_private_key *string `android:"path"`

	// Hash and signing algorithm for avbtool. Default is SHA256_RSA4096.
	Avb_algorithm *string

	// Type of the filesystem. Currently, ext4 and compressed_cpio are supported. Default is
	// ext4.
	Type *string
}

// android_filesystem packages a set of modules and their transitive dependencies into a filesystem
// image. The filesystem images are expected to be mounted in the target device, which means the
// modules in the filesystem image are built for the target device (i.e. Android, not Linux host).
// The modules are placed in the filesystem image just like they are installed to the ordinary
// partitions like system.img. For example, cc_library modules are placed under ./lib[64] directory.
func filesystemFactory() android.Module {
	module := &filesystem{}
	module.AddProperties(&module.properties)
	android.InitPackageModule(module)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

var dependencyTag = struct {
	blueprint.BaseDependencyTag
	android.InstallAlwaysNeededDependencyTag
}{}

func (f *filesystem) DepsMutator(ctx android.BottomUpMutatorContext) {
	f.AddDeps(ctx, dependencyTag)
}

type fsType int

const (
	ext4Type fsType = iota
	compressedCpioType
	unknown
)

func (f *filesystem) fsType(ctx android.ModuleContext) fsType {
	typeStr := proptools.StringDefault(f.properties.Type, "ext4")
	switch typeStr {
	case "ext4":
		return ext4Type
	case "compressed_cpio":
		return compressedCpioType
	default:
		ctx.PropertyErrorf("type", "%q not supported", typeStr)
		return unknown
	}
}

func (f *filesystem) installFileName() string {
	return f.BaseModuleName() + ".img"
}

var pctx = android.NewPackageContext("android/soong/filesystem")

func (f *filesystem) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	switch f.fsType(ctx) {
	case ext4Type:
		f.output = f.buildImageUsingBuildImage(ctx)
	case compressedCpioType:
		f.output = f.buildCompressedCpioImage(ctx)
	default:
		return
	}

	f.installDir = android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(f.installDir, f.installFileName(), f.output)
}

func (f *filesystem) buildImageUsingBuildImage(ctx android.ModuleContext) android.OutputPath {
	zipFile := android.PathForModuleOut(ctx, "temp.zip").OutputPath
	f.CopyDepsToZip(ctx, zipFile)

	rootDir := android.PathForModuleOut(ctx, "root").OutputPath
	builder := android.NewRuleBuilder(pctx, ctx)
	builder.Command().
		BuiltTool("zipsync").
		FlagWithArg("-d ", rootDir.String()). // zipsync wipes this. No need to clear.
		Input(zipFile)

	propFile, toolDeps := f.buildPropFile(ctx)
	output := android.PathForModuleOut(ctx, f.installFileName()).OutputPath
	builder.Command().BuiltTool("build_image").
		Text(rootDir.String()). // input directory
		Input(propFile).
		Implicits(toolDeps).
		Output(output).
		Text(rootDir.String()) // directory where to find fs_config_files|dirs

	// rootDir is not deleted. Might be useful for quick inspection.
	builder.Build("build_filesystem_image", fmt.Sprintf("Creating filesystem %s", f.BaseModuleName()))

	return output
}

func (f *filesystem) buildPropFile(ctx android.ModuleContext) (propFile android.OutputPath, toolDeps android.Paths) {
	type prop struct {
		name  string
		value string
	}

	var props []prop
	var deps android.Paths
	addStr := func(name string, value string) {
		props = append(props, prop{name, value})
	}
	addPath := func(name string, path android.Path) {
		props = append(props, prop{name, path.String()})
		deps = append(deps, path)
	}

	// Type string that build_image.py accepts.
	fsTypeStr := func(t fsType) string {
		switch t {
		// TODO(jiyong): add more types like f2fs, erofs, etc.
		case ext4Type:
			return "ext4"
		}
		panic(fmt.Errorf("unsupported fs type %v", t))
	}

	addStr("fs_type", fsTypeStr(f.fsType(ctx)))
	addStr("mount_point", "system")
	addStr("use_dynamic_partition_size", "true")
	addPath("ext_mkuserimg", ctx.Config().HostToolPath(ctx, "mkuserimg_mke2fs"))
	// b/177813163 deps of the host tools have to be added. Remove this.
	for _, t := range []string{"mke2fs", "e2fsdroid", "tune2fs"} {
		deps = append(deps, ctx.Config().HostToolPath(ctx, t))
	}

	if proptools.Bool(f.properties.Use_avb) {
		addStr("avb_hashtree_enable", "true")
		addPath("avb_avbtool", ctx.Config().HostToolPath(ctx, "avbtool"))
		algorithm := proptools.StringDefault(f.properties.Avb_algorithm, "SHA256_RSA4096")
		addStr("avb_algorithm", algorithm)
		key := android.PathForModuleSrc(ctx, proptools.String(f.properties.Avb_private_key))
		addPath("avb_key_path", key)
		addStr("avb_add_hashtree_footer_args", "--do_not_generate_fec")
		addStr("partition_name", f.Name())
	}

	propFile = android.PathForModuleOut(ctx, "prop").OutputPath
	builder := android.NewRuleBuilder(pctx, ctx)
	builder.Command().Text("rm").Flag("-rf").Output(propFile)
	for _, p := range props {
		builder.Command().
			Text("echo").
			Flag(`"` + p.name + "=" + p.value + `"`).
			Text(">>").Output(propFile)
	}
	builder.Build("build_filesystem_prop", fmt.Sprintf("Creating filesystem props for %s", f.BaseModuleName()))
	return propFile, deps
}

func (f *filesystem) buildCompressedCpioImage(ctx android.ModuleContext) android.OutputPath {
	if proptools.Bool(f.properties.Use_avb) {
		ctx.PropertyErrorf("use_avb", "signing compresed cpio image using avbtool is not supported."+
			"Consider adding this to bootimg module and signing the entire boot image.")
	}

	zipFile := android.PathForModuleOut(ctx, "temp.zip").OutputPath
	f.CopyDepsToZip(ctx, zipFile)

	rootDir := android.PathForModuleOut(ctx, "root").OutputPath
	builder := android.NewRuleBuilder(pctx, ctx)
	builder.Command().
		BuiltTool("zipsync").
		FlagWithArg("-d ", rootDir.String()). // zipsync wipes this. No need to clear.
		Input(zipFile)

	output := android.PathForModuleOut(ctx, f.installFileName()).OutputPath
	builder.Command().
		BuiltTool("mkbootfs").
		Text(rootDir.String()). // input directory
		Text("|").
		BuiltTool("lz4").
		Flag("--favor-decSpeed"). // for faster boot
		Flag("-12").              // maximum compression level
		Flag("-l").               // legacy format for kernel
		Text(">").Output(output)

	// rootDir is not deleted. Might be useful for quick inspection.
	builder.Build("build_compressed_cpio_image", fmt.Sprintf("Creating filesystem %s", f.BaseModuleName()))

	return output
}

var _ android.AndroidMkEntriesProvider = (*filesystem)(nil)

// Implements android.AndroidMkEntriesProvider
func (f *filesystem) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(f.output),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", f.installDir.ToMakePath().String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", f.installFileName())
			},
		},
	}}
}

// Filesystem is the public interface for the filesystem struct. Currently, it's only for the apex
// package to have access to the output file.
type Filesystem interface {
	android.Module
	OutputPath() android.Path
}

var _ Filesystem = (*filesystem)(nil)

func (f *filesystem) OutputPath() android.Path {
	return f.output
}
