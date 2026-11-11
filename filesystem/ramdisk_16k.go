// Copyright (C) 2025 The Android Open Source Project
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
	"android/soong/android"
	"android/soong/cc/config"
	_ "android/soong/cc/config"
	"android/soong/filesystem/ramdisk_16k/common"

	"github.com/google/blueprint/proptools"
)

type ramdisk16kImg struct {
	android.ModuleBase
	properties Ramdisk16kImgProperties
}

type Ramdisk16kImgProperties struct {
	// List or filegroup of prebuilt kernel module files. Should have .ko suffix.
	Srcs []string `android:"path,arch_variant"`

	// The zip of kernel modules from system_dlkm. Use `:module_name{.modules.zip}` here.
	// Modules in this zip will not be stripped, as stripping would remove the signature
	// of the kernel modules, and GKI modules must be signed for the kernel to load them.
	// https://cs.android.com/android/platform/superproject/main/+/main:build/make/core/Makefile;l=1124;drc=a951ebf0198006f7fd38073a05c442d0eb92f97b
	System_dep *string `android:"path,arch_variant"`

	// List specifying load order of kernel modules.
	Load []string

	// Path to the prebuilt 16KB kernel
	Kernel *string `android:"path"`

	// properties for getting the kernel modules from a zip file.
	// Useful if the exact list of kernel modules to install isn't known at analysis time.
	Zip struct {
		// The zip file. Kernel modules will be looked up under the 16kb/ folder.
		Src *string `android:"path,arch_variant"`

		// Names of modules that will be removed from the load file.
		Extra_blocked_modules []string
	}
}

func (p *Ramdisk16kImgProperties) resolve(ctx android.ModuleContext, cmd *android.RuleBuilderCommand) common.Ramdisk16kImgPropertiesJSON {
	var system_dep *string
	if p.System_dep != nil {
		system_dep = proptools.StringPtr(cmd.PathForInputFromFile(android.PathForModuleSrc(ctx, *p.System_dep)))
	}
	var zip *string
	if p.Zip.Src != nil {
		zip = proptools.StringPtr(cmd.PathForInputFromFile(android.PathForModuleSrc(ctx, *p.Zip.Src)))
	}
	var srcs []string
	for _, src := range android.PathsForModuleSrc(ctx, p.Srcs) {
		srcs = append(srcs, cmd.PathForInputFromFile(src))
	}
	var kernel *string
	if p.Kernel != nil {
		kernel = proptools.StringPtr(cmd.PathForInputFromFile(android.PathForModuleSrc(ctx, *p.Kernel)))
	}
	return common.Ramdisk16kImgPropertiesJSON{
		Srcs:       srcs,
		System_dep: system_dep,
		Load:       p.Load,
		Kernel:     kernel,
		Zip: common.ZipProperties{
			Src:                   zip,
			Extra_blocked_modules: p.Zip.Extra_blocked_modules,
		},
	}
}

func Ramdisk16kImgFactory() android.Module {
	module := &ramdisk16kImg{}
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	module.AddProperties(&module.properties)
	return module
}

func (p *ramdisk16kImg) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddHostToolDependencies("ramdisk_16k_builder", "extract_kernel", "depmod", "lz4", "mkbootfs")
}

// Extracts version information from the kernel and packages the .ko modules in
// a version-specific subdirectory of the .img file.
func (p *ramdisk16kImg) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(p.properties.Srcs) == 0 && p.properties.Zip.Src == nil {
		return
	}
	outputDir := android.PathForModuleOut(ctx, "ramdisk_16k")
	output := outputDir.Join(ctx, "ramdisk_16k.img")
	intermediatesDir := outputDir.Join(ctx, "intermediates")

	llvmStrip := config.ClangPath(ctx, "bin/llvm-strip")
	// llvm-strip is a symlink to llvm-objcopy
	llvmObjcopy := config.ClangPath(ctx, "bin/llvm-objcopy")
	llvmLib := config.ClangToolsRuntimeLib(ctx)

	builder := android.NewRuleBuilder(pctx, ctx).SandboxDisabled().Sbox(
		outputDir,
		android.PathForModuleOut(ctx, "ramdisk_16k_intermediates.textproto"),
	)

	cmd := builder.Command()

	propsFile := android.PathForModuleOut(ctx, "props.json")
	props := p.properties.resolve(ctx, cmd)
	android.WriteFileRule(ctx, propsFile, props.ToJSON())

	// Determine the kernel version during execution.
	cmd.BuiltTool("ramdisk_16k_builder").
		Flag("--extract_kernel").BuiltTool("extract_kernel").
		Flag("--depmod").BuiltTool("depmod").
		Flag("--llvm-strip").Input(llvmStrip).Implicit(llvmLib).Implicit(llvmObjcopy).
		Flag("--lz4").BuiltTool("lz4").
		Flag("--mkbootfs").BuiltTool("mkbootfs").
		// depmod needs libc++
		// TODO: Get this from the depmod dep automatically
		ImplicitTool(ctx.Config().HostCcSharedLibPath(ctx, "libc++")).
		Input(propsFile).
		Text(intermediatesDir.String()).
		Output(output).
		Implicits(android.PathsForModuleSrc(ctx, p.properties.Srcs))

	if p.properties.System_dep != nil {
		cmd.Implicit(android.PathForModuleSrc(ctx, *p.properties.System_dep))
	}
	if p.properties.Kernel != nil {
		cmd.Implicit(android.PathForModuleSrc(ctx, *p.properties.Kernel))
	}
	if p.properties.Zip.Src != nil {
		cmd.Implicit(android.PathForModuleSrc(ctx, *p.properties.Zip.Src))
	}

	builder.Build("ramdisk_16k", "ramdisk_16k")

	ctx.ModulePhonyFiles(output)
	android.SetProvider(ctx, FilesystemProvider, FilesystemInfo{
		Output: output,
	})
	android.SetProvider(ctx, ramdiskFragmentInfoProvider, ramdiskFragmentInfo{
		Output:       output,
		Ramdisk_name: "16K",
	})
}
