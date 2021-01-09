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
)

func init() {
	android.RegisterModuleType("android_filesystem", filesystemFactory)
}

type filesystem struct {
	android.ModuleBase
	android.PackagingBase

	output     android.OutputPath
	installDir android.InstallPath
}

// android_filesystem packages a set of modules and their transitive dependencies into a filesystem
// image. The filesystem images are expected to be mounted in the target device, which means the
// modules in the filesystem image are built for the target device (i.e. Android, not Linux host).
// The modules are placed in the filesystem image just like they are installed to the ordinary
// partitions like system.img. For example, cc_library modules are placed under ./lib[64] directory.
func filesystemFactory() android.Module {
	module := &filesystem{}
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

func (f *filesystem) installFileName() string {
	return f.BaseModuleName() + ".img"
}

var pctx = android.NewPackageContext("android/soong/filesystem")

func (f *filesystem) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	zipFile := android.PathForModuleOut(ctx, "temp.zip").OutputPath
	f.CopyDepsToZip(ctx, zipFile)

	rootDir := android.PathForModuleOut(ctx, "root").OutputPath
	builder := android.NewRuleBuilder(pctx, ctx)
	builder.Command().
		BuiltTool("zipsync").
		FlagWithArg("-d ", rootDir.String()). // zipsync wipes this. No need to clear.
		Input(zipFile)

	mkuserimg := ctx.Config().HostToolPath(ctx, "mkuserimg_mke2fs")
	propFile := android.PathForModuleOut(ctx, "prop").OutputPath
	// TODO(jiyong): support more filesystem types other than ext4
	propsText := fmt.Sprintf(`mount_point=system\n`+
		`fs_type=ext4\n`+
		`use_dynamic_partition_size=true\n`+
		`ext_mkuserimg=%s\n`, mkuserimg.String())
	builder.Command().Text("echo").Flag("-e").Flag(`"` + propsText + `"`).
		Text(">").Output(propFile).
		Implicit(mkuserimg)

	f.output = android.PathForModuleOut(ctx, f.installFileName()).OutputPath
	builder.Command().BuiltTool("build_image").
		Text(rootDir.String()). // input directory
		Input(propFile).
		Output(f.output).
		Text(rootDir.String()) // directory where to find fs_config_files|dirs

	// rootDir is not deleted. Might be useful for quick inspection.
	builder.Build("build_filesystem_image", fmt.Sprintf("Creating filesystem %s", f.BaseModuleName()))

	f.installDir = android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(f.installDir, f.installFileName(), f.output)
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
