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
)

func init() {
	android.RegisterModuleType("android_filesystem", filesystemFactory)
}

type filesystem struct {
	android.ModuleBase
	android.PackagingBase
}

func filesystemFactory() android.Module {
	module := &filesystem{}
	android.InitPackageModule(module)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func (f *filesystem) DepsMutator(ctx android.BottomUpMutatorContext) {
	f.AddDeps(ctx)
}

var pctx = android.NewPackageContext("android/soong/filesystem")

func (f *filesystem) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	zipFile := android.PathForModuleOut(ctx, "temp.zip").OutputPath
	f.CopyDepsToZip(ctx, zipFile)

	rootDir := android.PathForModuleOut(ctx, "root").OutputPath
	builder := android.NewRuleBuilder()
	builder.Command().
		BuiltTool(ctx, "zipsync").
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

	image := android.PathForModuleOut(ctx, "filesystem.img").OutputPath
	builder.Command().BuiltTool(ctx, "build_image").
		Text(rootDir.String()). // input directory
		Input(propFile).
		Output(image).
		Text(rootDir.String()) // directory where to find fs_config_files|dirs

	// rootDir is not deleted. Might be useful for quick inspection.
	builder.Build(pctx, ctx, "build_filesystem_image", fmt.Sprintf("Creating filesystem %s", f.BaseModuleName()))
}
