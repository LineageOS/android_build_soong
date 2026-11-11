// Copyright (C) 2021 The Android Open Source Project
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

package kernel

import (
	"android/soong/android"
	"android/soong/cc/config"
	"android/soong/kernel/common"

	"github.com/google/blueprint/proptools"
)

var (
	pctx = android.NewPackageContext("android/soong/kernel")
)

func init() {
	pctx.Import("android/soong/android")
	pctx.Import("android/soong/cc/config")
	registerKernelBuildComponents(android.InitRegistrationContext)
}

func registerKernelBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("prebuilt_kernel_modules", PrebuiltKernelModulesFactory)
}

var PrepareForTestWithPrebuiltKernelModules = android.FixtureRegisterWithContext(registerKernelBuildComponents)

type prebuiltKernelModules struct {
	android.ModuleBase

	properties PrebuiltKernelModulesProperties
}

type PrebuiltKernelModulesProperties struct {
	// List or filegroup of prebuilt kernel module files. Should have .ko suffix.
	Srcs proptools.Configurable[[]string] `android:"path,arch_variant"`

	// List of kernel module filenames that will be loaded via modules.load.
	// Should have .ko suffix.
	// If empty, this will default to filenames of Srcs.
	// If no sources should be loaded, set `Load_by_default` to false instead.
	Src_filenames_to_load []string `android:"path,arch_variant"`

	// List or filegroup of prebuilt kernel module files for 16k. Should have .ko suffix.
	// These files will be installed in lib/modules/16k-mode/
	// These files are ONLY loaded during the Second Boot Stage when the device is in 16k mode.
	Srcs_16k []string `android:"path,arch_variant"`

	// List of system_dlkm prebuilt_kernel_modules that the local kernel modules
	// depend on. Use `:module_name{.modules.zip}` here.
	// The deps will be assembled into intermediates directory for running depmod
	// but will not be added to the current module's installed files.
	System_dep *string `android:"path,arch_variant"`

	// If false, then srcs will not be included in modules.load.
	// This feature is used by system_dlkm
	Load_by_default *bool

	Blocklist_file *string `android:"path"`

	// Path to the kernel module options file
	Options_file *string `android:"path"`

	// Kernel version that these modules are for. Kernel modules are installed to
	// /lib/modules/<kernel_version> directory in the corresponding partition. Default is "".
	Kernel_version *string

	// Whether this module is directly installable to one of the partitions. Default is true
	Installable *bool

	// Whether debug symbols should be stripped from the *.ko files.
	// Defaults to true.
	Strip_debug_symbols *bool

	// Properties related to loading the kernel modules from a zip file.
	// This is useful if you want the list of kernel modules to be dynamic, and unknown at analysis
	// time, for example when supplying the kernel modules via CIPD.
	//
	// Most of these properties are mutually exclusive with the other, non-zip properties. But
	// some such as srcs will be merged with the contents / information from the zip file.
	Zip struct {
		// The zip file containing the kernel modules and other files like the load/blocklist files.
		Src *string `android:"path,arch_variant"`

		// The name of the load file inside of the zip file. Only modules listed in it will
		// be installed.
		Load_file *string

		// List of extra kernel modules to add to the load file.
		Extra_loads []string

		// The name of the blocklist file inside of the zip file.
		Blocklist_file *string

		// Name of a .cfg file inside of the zip that's loaded by init.insmod.sh. This is just used
		// to determine the list of 16k kernel modules, taken from all the modprobe| lines
		// in the cfg file.
		Srcs_16k_cfg_file *string
	}
}

func (p *PrebuiltKernelModulesProperties) resolve(ctx android.ModuleContext, command *android.RuleBuilderCommand) common.PrebuiltKernelModulesPropertiesJSON {
	var systemDep *string
	if p.System_dep != nil {
		systemDep = proptools.StringPtr(command.PathForInputFromFile(android.PathForModuleSrc(ctx, *p.System_dep)))
	}
	var zip *string
	if p.Zip.Src != nil {
		zip = proptools.StringPtr(command.PathForInputFromFile(android.PathForModuleSrc(ctx, *p.Zip.Src)))
	}
	var blocklistFile *string
	if p.Blocklist_file != nil {
		blocklistFile = proptools.StringPtr(command.PathForInputFromFile(android.PathForModuleSrc(ctx, *p.Blocklist_file)))
	}
	var optionsFile *string
	if p.Options_file != nil {
		optionsFile = proptools.StringPtr(command.PathForInputFromFile(android.PathForModuleSrc(ctx, *p.Options_file)))
	}

	var srcs []string
	for _, src := range android.PathsForModuleSrc(ctx, p.Srcs.GetOrDefault(ctx, nil)) {
		srcs = append(srcs, command.PathForInputFromFile(src))
	}

	var srcs_16k []string
	for _, src := range android.PathsForModuleSrc(ctx, p.Srcs_16k) {
		srcs_16k = append(srcs_16k, command.PathForInputFromFile(src))
	}

	return common.PrebuiltKernelModulesPropertiesJSON{
		Srcs:                  srcs,
		Src_filenames_to_load: p.Src_filenames_to_load,
		Srcs_16k:              srcs_16k,
		System_dep:            systemDep,
		Load_by_default:       p.Load_by_default,
		Blocklist_file:        blocklistFile,
		Options_file:          optionsFile,
		Kernel_version:        p.Kernel_version,
		Installable:           p.Installable,
		Strip_debug_symbols:   p.Strip_debug_symbols,
		Zip: common.ZipProperties{
			Src:               zip,
			Load_file:         p.Zip.Load_file,
			Extra_loads:       p.Zip.Extra_loads,
			Blocklist_file:    p.Zip.Blocklist_file,
			Srcs_16k_cfg_file: p.Zip.Srcs_16k_cfg_file,
		},
	}
}

// prebuilt_kernel_modules installs a set of prebuilt kernel module files to the correct directory.
// In addition, this module builds modules.load, modules.dep, modules.softdep and modules.alias
// using depmod and installs them as well.
func PrebuiltKernelModulesFactory() android.Module {
	module := &prebuiltKernelModules{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

func (pkm *prebuiltKernelModules) installable() bool {
	return proptools.BoolDefault(pkm.properties.Installable, true)
}

func (pkm *prebuiltKernelModules) KernelVersion() string {
	return proptools.StringDefault(pkm.properties.Kernel_version, "")
}

func (pkm *prebuiltKernelModules) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddHostToolDependencies("kernel_modules_builder", "zipsync", "soong_zip", "merge_zips", "depmod")
}

func (pkm *prebuiltKernelModules) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if !pkm.installable() {
		pkm.SkipInstall()
	}

	var deps []android.Path
	deps = append(deps, android.PathsForModuleSrc(ctx, pkm.properties.Srcs.GetOrDefault(ctx, nil))...)
	deps = append(deps, android.PathsForModuleSrc(ctx, pkm.properties.Srcs_16k)...)
	systemModulesZip := android.OptionalPathForModuleSrc(ctx, pkm.properties.System_dep)
	if systemModulesZip.Valid() {
		deps = append(deps, systemModulesZip.Path())
	}
	blocklistFile := android.OptionalPathForModuleSrc(ctx, pkm.properties.Blocklist_file)
	if blocklistFile.Valid() {
		deps = append(deps, blocklistFile.Path())
	}
	optionsFile := android.OptionalPathForModuleSrc(ctx, pkm.properties.Options_file)
	if optionsFile.Valid() {
		deps = append(deps, optionsFile.Path())
	}
	if proptools.String(pkm.properties.Zip.Src) != "" {
		deps = append(deps, android.PathForModuleSrc(ctx, *pkm.properties.Zip.Src))
	}

	propsFile := android.PathForModuleOut(ctx, "props.json")
	sboxDir := android.PathForModuleOut(ctx, "sbox")
	sboxManifest := android.PathForModuleOut(ctx, "sbox.manifest")
	loadFile := sboxDir.Join(ctx, "modules.load")
	installsZip := sboxDir.Join(ctx, "installs.zip")
	tempDir := sboxDir.Join(ctx, "temp")

	builder := android.NewRuleBuilder(pctx, ctx).
		SandboxDisabled().
		Sbox(sboxDir, sboxManifest)

	llvmStrip := config.ClangPath(ctx, "bin/llvm-strip")
	// llvm-strip is a symlink to llvm-objcopy
	llvmObjcopy := config.ClangPath(ctx, "bin/llvm-objcopy")
	llvmLib := config.ClangToolsRuntimeLib(ctx)

	cmd := builder.Command()

	props := pkm.properties.resolve(ctx, cmd)
	android.WriteFileRule(ctx, propsFile, props.ToJSON())

	cmd.BuiltTool("kernel_modules_builder").
		Flag("--soong_zip").BuiltTool("soong_zip").
		Flag("--zipsync").BuiltTool("zipsync").
		Flag("--merge_zips").BuiltTool("merge_zips").
		Flag("--depmod").BuiltTool("depmod").
		Flag("--llvm-strip").Input(llvmStrip).Implicit(llvmLib).Implicit(llvmObjcopy).
		// depmod needs libc++
		// TODO: Get this from the depmod dep automatically
		ImplicitTool(ctx.Config().HostCcSharedLibPath(ctx, "libc++")).
		Input(propsFile).
		Text(partition(ctx)).
		Text(tempDir.String()).
		Output(loadFile).
		Output(installsZip).
		Implicits(deps)
	builder.Build("zip_modules", "zip kernel modules")

	installDir := android.PathForModuleInstall(ctx, "lib", "modules")
	// Kernel module is installed to vendor_ramdisk/lib/modules regardless of product
	// configuration. This matches the behavior in make and prevents the files from being
	// installed in `vendor_ramdisk/first_stage_ramdisk`.
	if pkm.InstallInVendorRamdisk() {
		installDir = android.PathForModuleInPartitionInstall(ctx, "vendor_ramdisk", "lib", "modules")
	} else if pkm.InstallInVendorKernelRamdisk() {
		installDir = android.PathForModuleInPartitionInstall(ctx, "vendor_kernel_ramdisk", "lib", "modules")
	}

	if pkm.KernelVersion() != "" {
		installDir = installDir.Join(ctx, pkm.KernelVersion())
	}

	var dests []string
	var srcs []string
	// Use ANDROID-GEN to identify the source of module.* files which are generated in the build process.
	// See the use of ANDROID-GEN in build/make/core/Makefile
	androidGen := "ANDROID-GEN"
	// Add ANDROID-GEN once to match the modules.load file in dests
	srcs = append(srcs, androidGen)
	installPath := ctx.InstallFileWithExtraFilesZip(installDir, "modules.load", loadFile, installsZip)
	dests = append(dests, installPath.String())

	ctx.SetOutputFiles(android.Paths{installsZip}, ".modules.zip")

	// TODO(b/466436522): we no longer include the files in the zip file here.
	android.SetProvider(ctx, android.PrebuiltKernelModulesComplianceMetadataProvider,
		android.PrebuiltKernelModulesComplianceMetadata{
			Srcs:  srcs,
			Dests: dests,
		})
}

// Return a string representation of the dlkm partition this module is installed on,
// for the kernel_modules_builder tool.
func partition(ctx android.ModuleContext) string {
	if ctx.InstallInSystemDlkm() {
		return "system_dlkm"
	} else if ctx.InstallInVendorDlkm() {
		return "vendor_dlkm"
	} else if ctx.InstallInOdmDlkm() {
		return "odm_dlkm"
	} else if ctx.InstallInVendorRamdisk() {
		return "vendor_ramdisk"
	} else if ctx.InstallInVendorKernelRamdisk() {
		return "vendor_kernel_ramdisk"
	} else {
		// not an android dlkm module.
		return "other"
	}
}
