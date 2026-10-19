// Copyright 2016 Google Inc. All rights reserved.
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

package cc

import (
	"fmt"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/cc/config"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// This file contains the basic functionality for linking against static libraries and shared
// libraries.  Final linking into libraries or executables is handled in library.go, binary.go, etc.

const (
	packRelocationsDefault = true
)

type BaseLinkerProperties struct {
	// list of modules whose object files should be linked into this module
	// in their entirety.  For static library modules, all of the .o files from the intermediate
	// directory of the dependency will be linked into this modules .a file.  For a shared library,
	// the dependency's .a file will be linked into this module using -Wl,--whole-archive.
	Whole_static_libs proptools.Configurable[[]string] `android:"arch_variant,variant_prepend"`

	// list of modules that should be statically linked into this module.
	Static_libs proptools.Configurable[[]string] `android:"arch_variant,variant_prepend"`

	// list of modules that should be dynamically linked into this module.
	Shared_libs proptools.Configurable[[]string] `android:"arch_variant"`

	// list of modules that should only provide headers for this module.
	Header_libs proptools.Configurable[[]string] `android:"arch_variant,variant_prepend"`

	// List of lfi libs to statically link into this module. The lfi libs must be cc binaries
	// with lfi: { enabled: true }.
	//
	// Lfi stands for Lightweight Fault Isolation and is a form of in-process sandboxing. The
	// lfi libs each are run in their own sandbox.
	Lfi_libs []string

	// list of module-specific flags that will be used for all link steps
	Ldflags proptools.Configurable[[]string] `android:"arch_variant"`

	// list of system libraries that will be dynamically linked to
	// shared library and executable modules.  If unset, generally defaults to libc,
	// libm, and libdl.  Set to [] to prevent linking against the defaults.
	System_shared_libs []string `android:"arch_variant"`

	// allow the module to contain undefined symbols.  By default,
	// modules cannot contain undefined symbols that are not satisified by their immediate
	// dependencies.  Set this flag to true to remove --no-undefined from the linker flags.
	// This flag should only be necessary for compiling low-level libraries like libc.
	Allow_undefined_symbols *bool `android:"arch_variant"`

	// ignore max page size. By default, max page size must be the
	// max page size set for the target.
	Ignore_max_page_size *bool `android:"arch_variant"`

	// don't link in libclang_rt.builtins-*.a
	No_libcrt *bool `android:"arch_variant"`

	// -l arguments to pass to linker for host-provided shared libraries
	Host_ldlibs []string `android:"arch_variant"`

	// list of shared libraries to re-export include directories from. Entries must be
	// present in shared_libs.
	Export_shared_lib_headers []string `android:"arch_variant"`

	// list of static libraries to re-export include directories from. Entries must be
	// present in static_libs.
	Export_static_lib_headers []string `android:"arch_variant"`

	// list of header libraries to re-export include directories from. Entries must be
	// present in header_libs.
	Export_header_lib_headers proptools.Configurable[[]string] `android:"arch_variant,variant_prepend"`

	// list of generated headers to re-export include directories from. Entries must be
	// present in generated_headers.
	Export_generated_headers []string `android:"arch_variant"`

	// don't link in crt_begin and crt_end.  This flag should only be necessary for
	// compiling crt or libc.
	Nocrt *bool `android:"arch_variant"`

	// don't link in crt_pad_segment. This flag is currently only used internal to
	// soong for testing and for vndk prebuilt shared libraries.
	No_crt_pad_segment *bool `android:"arch_variant"`

	// deprecated and ignored because lld makes it unnecessary. See b/189475744.
	Group_static_libs *bool `android:"arch_variant"`

	// list of modules that should be installed with this module.  This is similar to 'required'
	// but '.vendor' suffix will be appended to the module names if the shared libraries have
	// vendor variants and this module uses VNDK.
	Runtime_libs []string `android:"arch_variant"`

	// list of runtime libs that should not be installed along with this module.
	Exclude_runtime_libs []string `android:"arch_variant"`

	Target struct {
		Vendor, Product struct {
			// list of shared libs that only should be used to build vendor or
			// product variant of the C/C++ module.
			Shared_libs []string

			// list of static libs that only should be used to build vendor or
			// product variant of the C/C++ module.
			Static_libs []string

			// list of header libs that only should be used to build vendor or product
			// variant of the C/C++ module.
			Header_libs []string

			// list of shared libs that should not be used to build vendor or
			// product variant of the C/C++ module.
			Exclude_shared_libs []string

			// list of static libs that should not be used to build vendor or
			// product variant of the C/C++ module.
			Exclude_static_libs []string

			// list of header libs that should not be used to build vendor or
			// product variant of the C/C++ module.
			Exclude_header_libs []string

			// list of runtime libs that should not be installed along with the
			// vendor or product variant of the C/C++ module.
			Exclude_runtime_libs []string

			// version script for vendor or product variant
			Version_script *string `android:"arch_variant"`
		} `android:"arch_variant"`
		Recovery struct {
			// list of shared libs that only should be used to build the recovery
			// variant of the C/C++ module.
			Shared_libs []string

			// list of static libs that only should be used to build the recovery
			// variant of the C/C++ module.
			Static_libs []string

			// list of shared libs that should not be used to build
			// the recovery variant of the C/C++ module.
			Exclude_shared_libs []string

			// list of static libs that should not be used to build
			// the recovery variant of the C/C++ module.
			Exclude_static_libs []string

			// list of header libs that should not be used to build the recovery variant
			// of the C/C++ module.
			Exclude_header_libs []string

			// list of runtime libs that should not be installed along with the
			// recovery variant of the C/C++ module.
			Exclude_runtime_libs []string
		}
		Ramdisk struct {
			// list of static libs that only should be used to build the ramdisk
			// variant of the C/C++ module.
			Static_libs []string

			// list of shared libs that should not be used to build
			// the ramdisk variant of the C/C++ module.
			Exclude_shared_libs []string

			// list of static libs that should not be used to build
			// the ramdisk variant of the C/C++ module.
			Exclude_static_libs []string

			// list of runtime libs that should not be installed along with the
			// ramdisk variant of the C/C++ module.
			Exclude_runtime_libs []string
		}
		Vendor_ramdisk struct {
			// list of shared libs that should not be used to build
			// the vendor ramdisk variant of the C/C++ module.
			Exclude_shared_libs []string

			// list of static libs that should not be used to build
			// the vendor ramdisk variant of the C/C++ module.
			Exclude_static_libs []string

			// list of runtime libs that should not be installed along with the
			// vendor ramdisk variant of the C/C++ module.
			Exclude_runtime_libs []string
		}
		Platform struct {
			// list of shared libs that should be use to build the platform variant
			// of a module that sets sdk_version.  This should rarely be necessary,
			// in most cases the same libraries are available for the SDK and platform
			// variants.
			Shared_libs []string

			// list of ehader libs that only should be used to build platform variant of
			// the C/C++ module.
			Header_libs []string

			// list of shared libs that should not be used to build the platform variant
			// of the C/C++ module.
			Exclude_shared_libs []string
		}
		Apex struct {
			// list of shared libs that should not be used to build the apex variant of
			// the C/C++ module.
			Exclude_shared_libs []string

			// list of static libs that should not be used to build the apex
			// variant of the C/C++ module.
			Exclude_static_libs []string
		}
		Non_apex struct {
			// list of shared libs that should not be used to build the non-apex
			// variant of the C/C++ module.
			Exclude_shared_libs []string
		}
	} `android:"arch_variant"`

	// make android::build:GetBuildNumber() available containing the build ID.
	Use_version_lib *bool `android:"arch_variant"`

	// Generate compact dynamic relocation table, default true.
	Pack_relocations *bool `android:"arch_variant"`

	// local file name to pass to the linker as --version-script.  Not supported on darwin, and will fail to build
	// if provided to the darwin variant of a module.
	Version_script *string `android:"path,arch_variant"`

	// local file name to pass to the linker as --dynamic-list.  Not supported on darwin, and will fail to build
	// if provided to the darwin variant of a module.
	Dynamic_list *string `android:"path,arch_variant"`

	// local files to pass to the linker as --script.  Not supported on darwin or windows, and will fail to build
	// if provided to the darwin or windows variant of a module.
	Linker_scripts proptools.Configurable[[]string] `android:"path,arch_variant"`

	// list of static libs that should not be used to build this module
	Exclude_static_libs []string `android:"arch_variant"`

	// list of shared libs that should not be used to build this module
	Exclude_shared_libs []string `android:"arch_variant"`

	// Xom controls whether or not xom should be enabled for this module. Setting
	// this to false will disable xom for all dependents which link this module
	// statically.
	Xom *bool
}

func (blp *BaseLinkerProperties) crt() bool {
	// Since crt is enabled for almost every module compiling against the Bionic runtime,
	// we interpret `nil` as  enabled.
	return blp.Nocrt == nil || !*blp.Nocrt
}

func (blp *BaseLinkerProperties) libCrt() bool {
	return blp.No_libcrt == nil || !*blp.No_libcrt
}

func (blp *BaseLinkerProperties) crtPadSegment() bool {
	return blp.No_crt_pad_segment == nil || !*blp.No_crt_pad_segment
}

func NewBaseLinker(sanitize *sanitize) *baseLinker {
	return &baseLinker{sanitize: sanitize}
}

// baseLinker provides support for shared_libs, static_libs, and whole_static_libs properties
type baseLinker struct {
	Properties        BaseLinkerProperties
	dynamicProperties struct {
		BuildStubs bool `blueprint:"mutated"`
	}

	sanitize *sanitize
}

// linkerInit initializes dynamic properties of the linker.
func (linker *baseLinker) linkerInit(ctx BaseModuleContext) {
}

func (linker *baseLinker) linkerProps() []interface{} {
	return []interface{}{&linker.Properties, &linker.dynamicProperties}
}

func (linker *baseLinker) baseLinkerProps() BaseLinkerProperties {
	return linker.Properties
}

func (linker *baseLinker) Xom() *bool {
	return linker.Properties.Xom
}

func (linker *baseLinker) extraOutputFilePaths() map[string]android.Paths {
	return nil
}

func CoalesceLibs(ctx android.BaseModuleContext, props *BaseLinkerProperties, deps Deps) Deps {
	mod, ok := ctx.Module().(*Module)
	if !ok {
		ctx.ModuleErrorf("CoalesceLibs can only be called from a cc module")
	}

	deps.WholeStaticLibs = append(deps.WholeStaticLibs, props.Whole_static_libs.GetOrDefault(ctx, nil)...)
	deps.HeaderLibs = append(deps.HeaderLibs, props.Header_libs.GetOrDefault(ctx, nil)...)
	deps.StaticLibs = append(deps.StaticLibs, props.Static_libs.GetOrDefault(ctx, nil)...)
	deps.SharedLibs = append(deps.SharedLibs, props.Shared_libs.GetOrDefault(ctx, nil)...)
	deps.RuntimeLibs = append(deps.RuntimeLibs, props.Runtime_libs...)
	deps.LfiLibs = props.Lfi_libs

	deps.ReexportHeaderLibHeaders = append(deps.ReexportHeaderLibHeaders, props.Export_header_lib_headers.GetOrDefault(ctx, nil)...)
	deps.ReexportStaticLibHeaders = append(deps.ReexportStaticLibHeaders, props.Export_static_lib_headers...)
	deps.ReexportSharedLibHeaders = append(deps.ReexportSharedLibHeaders, props.Export_shared_lib_headers...)
	deps.ReexportGeneratedHeaders = append(deps.ReexportGeneratedHeaders, props.Export_generated_headers...)

	deps.SharedLibs = removeListFromList(deps.SharedLibs, props.Exclude_shared_libs)
	deps.StaticLibs = removeListFromList(deps.StaticLibs, props.Exclude_static_libs)
	deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, props.Exclude_static_libs)
	deps.RuntimeLibs = removeListFromList(deps.RuntimeLibs, props.Exclude_runtime_libs)

	// Record the libraries that need to be excluded when building for APEX. Unlike other
	// target.*.exclude_* properties, SharedLibs and StaticLibs are not modified here because
	// this module hasn't yet passed the apexMutator. Therefore, we can't tell whether this is
	// an apex variant of not. Record the exclude list in the deps struct for now. The info is
	// used to mark the dependency tag when adding dependencies to the deps. Then inside
	// GenerateAndroidBuildActions, the marked dependencies are ignored (i.e. not used) for APEX
	// variants.
	deps.ExcludeLibsForApex = append(deps.ExcludeLibsForApex, props.Target.Apex.Exclude_shared_libs...)
	deps.ExcludeLibsForApex = append(deps.ExcludeLibsForApex, props.Target.Apex.Exclude_static_libs...)
	// Record the libraries that need to be excluded when building for non-APEX variants
	// for the same reason above. This is used for marking deps and marked deps are
	// ignored for non-apex variants.
	deps.ExcludeLibsForNonApex = append(deps.ExcludeLibsForNonApex, props.Target.Non_apex.Exclude_shared_libs...)

	if Bool(props.Use_version_lib) {
		deps.WholeStaticLibs = append(deps.WholeStaticLibs, "libbuildversion")
	}

	if mod.InVendor() {
		deps.SharedLibs = append(deps.SharedLibs, props.Target.Vendor.Shared_libs...)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, props.Target.Vendor.Exclude_shared_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, props.Target.Vendor.Exclude_shared_libs)
		deps.StaticLibs = append(deps.StaticLibs, props.Target.Vendor.Static_libs...)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, props.Target.Vendor.Exclude_static_libs)
		deps.HeaderLibs = append(deps.HeaderLibs, props.Target.Vendor.Header_libs...)
		deps.HeaderLibs = removeListFromList(deps.HeaderLibs, props.Target.Vendor.Exclude_header_libs)
		deps.ReexportHeaderLibHeaders = removeListFromList(deps.ReexportHeaderLibHeaders, props.Target.Vendor.Exclude_header_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, props.Target.Vendor.Exclude_static_libs)
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, props.Target.Vendor.Exclude_static_libs)
		deps.RuntimeLibs = removeListFromList(deps.RuntimeLibs, props.Target.Vendor.Exclude_runtime_libs)
	}

	if mod.InProduct() {
		deps.SharedLibs = append(deps.SharedLibs, props.Target.Product.Shared_libs...)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, props.Target.Product.Exclude_shared_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, props.Target.Product.Exclude_shared_libs)
		deps.StaticLibs = append(deps.StaticLibs, props.Target.Product.Static_libs...)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, props.Target.Product.Exclude_static_libs)
		deps.HeaderLibs = removeListFromList(deps.HeaderLibs, props.Target.Product.Exclude_header_libs)
		deps.ReexportHeaderLibHeaders = removeListFromList(deps.ReexportHeaderLibHeaders, props.Target.Product.Exclude_header_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, props.Target.Product.Exclude_static_libs)
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, props.Target.Product.Exclude_static_libs)
		deps.RuntimeLibs = removeListFromList(deps.RuntimeLibs, props.Target.Product.Exclude_runtime_libs)
	}

	if mod.InRecovery() {
		deps.SharedLibs = append(deps.SharedLibs, props.Target.Recovery.Shared_libs...)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, props.Target.Recovery.Exclude_shared_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, props.Target.Recovery.Exclude_shared_libs)
		deps.StaticLibs = append(deps.StaticLibs, props.Target.Recovery.Static_libs...)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, props.Target.Recovery.Exclude_static_libs)
		deps.HeaderLibs = removeListFromList(deps.HeaderLibs, props.Target.Recovery.Exclude_header_libs)
		deps.ReexportHeaderLibHeaders = removeListFromList(deps.ReexportHeaderLibHeaders, props.Target.Recovery.Exclude_header_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, props.Target.Recovery.Exclude_static_libs)
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, props.Target.Recovery.Exclude_static_libs)
		deps.RuntimeLibs = removeListFromList(deps.RuntimeLibs, props.Target.Recovery.Exclude_runtime_libs)
	}

	if mod.InRamdisk() {
		deps.SharedLibs = removeListFromList(deps.SharedLibs, props.Target.Ramdisk.Exclude_shared_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, props.Target.Ramdisk.Exclude_shared_libs)
		deps.StaticLibs = append(deps.StaticLibs, props.Target.Ramdisk.Static_libs...)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, props.Target.Ramdisk.Exclude_static_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, props.Target.Ramdisk.Exclude_static_libs)
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, props.Target.Ramdisk.Exclude_static_libs)
		deps.RuntimeLibs = removeListFromList(deps.RuntimeLibs, props.Target.Ramdisk.Exclude_runtime_libs)
	}

	if mod.InVendorRamdisk() {
		deps.SharedLibs = removeListFromList(deps.SharedLibs, props.Target.Vendor_ramdisk.Exclude_shared_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, props.Target.Vendor_ramdisk.Exclude_shared_libs)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, props.Target.Vendor_ramdisk.Exclude_static_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, props.Target.Vendor_ramdisk.Exclude_static_libs)
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, props.Target.Vendor_ramdisk.Exclude_static_libs)
		deps.RuntimeLibs = removeListFromList(deps.RuntimeLibs, props.Target.Vendor_ramdisk.Exclude_runtime_libs)
	}

	if !mod.UseSdk() {
		deps.SharedLibs = append(deps.SharedLibs, props.Target.Platform.Shared_libs...)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, props.Target.Platform.Exclude_shared_libs)
		deps.HeaderLibs = append(deps.HeaderLibs, props.Target.Platform.Header_libs...)
	}

	deps.SystemSharedLibs = props.System_shared_libs

	return deps
}

func (linker *baseLinker) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps = CoalesceLibs(ctx, &linker.Properties, deps)

	if deps.SystemSharedLibs == nil {
		// Provide a default system_shared_libs if it is unspecified. Note: If an
		// empty list [] is specified, it implies that the module declines the
		// default system_shared_libs.
		deps.SystemSharedLibs = append(deps.SystemSharedLibs, ctx.toolchain().DefaultSharedLibraries()...)
	}

	if ctx.toolchain().Bionic() {
		// libclang_rt.builtins has to be last on the command line
		if linker.Properties.libCrt() && !ctx.header() {
			deps.UnexportedStaticLibs = append(deps.UnexportedStaticLibs, config.BuiltinsRuntimeLibrary())
		}

		if inList("libdl", deps.SharedLibs) {
			// If system_shared_libs has libc but not libdl, make sure shared_libs does not
			// have libdl to avoid loading libdl before libc.
			if inList("libc", deps.SystemSharedLibs) {
				if !inList("libdl", deps.SystemSharedLibs) {
					ctx.PropertyErrorf("shared_libs",
						"libdl must be in system_shared_libs, not shared_libs")
				}
				_, deps.SharedLibs = removeFromList("libdl", deps.SharedLibs)
			}
		}

		// If libc and libdl are both in system_shared_libs make sure libdl comes after libc
		// to avoid loading libdl before libc.
		if inList("libdl", deps.SystemSharedLibs) && inList("libc", deps.SystemSharedLibs) &&
			indexList("libdl", deps.SystemSharedLibs) < indexList("libc", deps.SystemSharedLibs) {
			ctx.PropertyErrorf("system_shared_libs", "libdl must be after libc")
		}
	} else if ctx.toolchain().Musl() {
		if linker.Properties.libCrt() && !ctx.header() {
			deps.UnexportedStaticLibs = append(deps.UnexportedStaticLibs, config.BuiltinsRuntimeLibrary())
		}
	} else if ctx.Os() == android.Linux && !ctx.header() && !ctx.static() {
		// TODO(b/440132952): This should be handled by the clang driver once we use --rtlib=compiler-rt
		deps.UnexportedStaticLibs = append(deps.UnexportedStaticLibs, config.BuiltinsRuntimeLibrary())
	}

	deps.LateSharedLibs = append(deps.LateSharedLibs, deps.SystemSharedLibs...)

	if ctx.Windows() && ctx.ModuleName() != "libwinpthread" {
		deps.LateStaticLibs = append(deps.LateStaticLibs, "libwinpthread")
	}

	return deps
}

// Check whether the SDK version is not older than the specific one
func CheckSdkVersionAtLeast(ctx ModuleContext, SdkVersion android.ApiLevel) bool {
	if ctx.minSdkVersion() == "current" {
		return true
	}
	parsedSdkVersion, err := NativeApiLevelFromUser(ctx, ctx.minSdkVersion())
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version",
			"Invalid min_sdk_version value (must be int or current): %q",
			ctx.minSdkVersion())
	}
	if parsedSdkVersion.LessThan(SdkVersion) {
		return false
	}
	return true
}

// ModuleContext extends BaseModuleContext
// BaseModuleContext should know if LLD is used?
func CommonLinkerFlags(ctx android.ModuleContext, flags Flags, toolchain config.Toolchain, allow_undefined_symbols bool) Flags {
	hod := "Host"
	if ctx.Os().Class == android.Device {
		hod = "Device"
	}

	mod, ok := ctx.Module().(LinkableInterface)
	if !ok {
		ctx.ModuleErrorf("trying to add CommonLinkerFlags to non-LinkableInterface module.")
		return flags
	}
	flags.Global.LdFlags = append(flags.Global.LdFlags, fmt.Sprintf("${config.%sGlobalLdflags}", hod))

	if allow_undefined_symbols {
		if ctx.Darwin() {
			// darwin defaults to treating undefined symbols as errors
			flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,-undefined,dynamic_lookup")
		}
	} else if !ctx.Darwin() && !ctx.Windows() {
		flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,--no-undefined")
	}

	ldFlags := toolchain.Ldflags(ctx)
	flags.Global.LdFlags = append(flags.Global.LdFlags, ldFlags.Flags)
	flags.LdFlagsDeps = append(flags.LdFlagsDeps, ldFlags.Deps...)

	if !toolchain.Bionic() && ctx.Os() != android.LinuxMusl && !toolchain.Lfi() {
		if !ctx.Windows() {
			// Add -ldl, -lpthread, -lm and -lrt to host builds to match the default behavior of device
			// builds
			flags.Global.LdFlags = append(flags.Global.LdFlags,
				"-ldl",
				"-lpthread",
				"-lm",
			)
			if !ctx.Darwin() {
				flags.Global.LdFlags = append(flags.Global.LdFlags, "-lrt")
			}
		}
	}
	staticLib := mod.CcLibraryInterface() && mod.Static()
	if ctx.Host() && !ctx.Windows() && !staticLib {
		flags.Global.LdFlags = append(flags.Global.LdFlags, RpathFlags(ctx)...)
	}
	if ctx.Device() {
		flags.Local.LdFlags = append(flags.Local.LdFlags, XomFlags(ctx)...)
	}
	toolchainLdFlags := toolchain.ToolchainLdflags()
	flags.Global.LdFlags = append(flags.Global.LdFlags, toolchainLdFlags.Flags)
	flags.LdFlagsDeps = append(flags.LdFlagsDeps, toolchainLdFlags.Deps...)
	return flags
}

func (linker *baseLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	toolchain := ctx.toolchain()
	allow_undefined_symbols := Bool(linker.Properties.Allow_undefined_symbols)

	flags = CommonLinkerFlags(ctx, flags, toolchain, allow_undefined_symbols)

	if toolchain.Glibc() || ctx.Os() == android.Windows || ctx.Os() == android.Darwin {
		CheckBadHostLdlibs(ctx, "host_ldlibs", linker.Properties.Host_ldlibs)
		flags.Local.LdFlags = append(flags.Local.LdFlags, linker.Properties.Host_ldlibs...)
	}

	ldflags := linker.Properties.Ldflags.GetOrDefault(ctx, nil)
	CheckBadLinkerFlags(ctx, "ldflags", ldflags)

	if !BoolDefault(linker.Properties.Pack_relocations, packRelocationsDefault) {
		flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,--pack-dyn-relocs=none")
	} else if ctx.Device() {
		// SHT_RELR relocations are only supported at API level >= 30.
		// ANDROID_RELR relocations were supported at API level >= 28.
		// Relocation packer was supported at API level >= 23.
		// Do the best we can...
		if (!ctx.useSdk() && ctx.minSdkVersion() == "") || CheckSdkVersionAtLeast(ctx, android.FirstShtRelrVersion) {
			flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,--pack-dyn-relocs=android+relr")
		} else if CheckSdkVersionAtLeast(ctx, android.FirstAndroidRelrVersion) {
			flags.Global.LdFlags = append(flags.Global.LdFlags,
				"-Wl,--pack-dyn-relocs=android+relr",
				"-Wl,--use-android-relr-tags")
		} else if CheckSdkVersionAtLeast(ctx, android.FirstPackedRelocationsVersion) {
			flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,--pack-dyn-relocs=android")
		}
	}

	flags.Local.LdFlags = append(flags.Local.LdFlags, proptools.NinjaAndShellEscapeList(ldflags)...)

	// Version_script is not needed when linking stubs lib where the version
	// script is created from the symbol map file.
	if !linker.dynamicProperties.BuildStubs {
		versionScript := ctx.ExpandOptionalSource(
			linker.Properties.Version_script, "version_script")

		if ctx.inVendor() && linker.Properties.Target.Vendor.Version_script != nil {
			versionScript = ctx.ExpandOptionalSource(
				linker.Properties.Target.Vendor.Version_script,
				"target.vendor.version_script")
		} else if ctx.inProduct() && linker.Properties.Target.Product.Version_script != nil {
			versionScript = ctx.ExpandOptionalSource(
				linker.Properties.Target.Product.Version_script,
				"target.product.version_script")
		}

		if versionScript.Valid() {
			if ctx.Darwin() {
				ctx.AddMissingDependencies([]string{"version_script_not_supported_on_darwin"})
			} else {
				flags.Local.LdFlags = append(flags.Local.LdFlags,
					config.VersionScriptFlagPrefix+versionScript.String())
				flags.LdFlagsDeps = append(flags.LdFlagsDeps, versionScript.Path())

				if linker.sanitize.isSanitizerEnabled(cfi) {
					cfiExportsMap := android.PathForSource(ctx, cfiExportsMapPath+"/"+cfiExportsMapFilename)
					flags.Local.LdFlags = append(flags.Local.LdFlags,
						config.VersionScriptFlagPrefix+cfiExportsMap.String())
					flags.LdFlagsDeps = append(flags.LdFlagsDeps, cfiExportsMap)
				}
			}
		}

		dynamicList := android.OptionalPathForModuleSrc(ctx, linker.Properties.Dynamic_list)
		if dynamicList.Valid() {
			if ctx.Darwin() {
				ctx.AddMissingDependencies([]string{"dynamic_list_not_supported_on_darwin"})
			} else {
				flags.Local.LdFlags = append(flags.Local.LdFlags,
					"-Wl,--dynamic-list,"+dynamicList.String())
				flags.LdFlagsDeps = append(flags.LdFlagsDeps, dynamicList.Path())
			}
		}

		linkerScriptPaths := android.PathsForModuleSrc(ctx, linker.Properties.Linker_scripts.GetOrDefault(ctx, nil))
		if len(linkerScriptPaths) > 0 {
			if ctx.Darwin() {
				ctx.AddMissingDependencies([]string{"linker_scripts_not_supported_on_darwin"})
			} else if ctx.Windows() {
				ctx.PropertyErrorf("linker_scripts", "Only supported for ELF files")
			} else {
				for _, linkerScriptPath := range linkerScriptPaths {
					flags.Local.LdFlags = append(flags.Local.LdFlags,
						"-Wl,--script,"+linkerScriptPath.String())
					flags.LdFlagsDeps = append(flags.LdFlagsDeps, linkerScriptPath)
				}
			}
		}
	}

	return flags
}

// RpathFlags returns the rpath linker flags for current target to search the following directories relative
// to the binary:
//
//   - "." to find libraries alongside tests
//   - "lib[64]" to find libraries in a subdirectory of the binaries' directory
//   - "../lib[64]" to find libraries when the binaries are in a bin directory
//   - "../../lib[64]" to find libraries in out/host/linux-x86/lib64 when the test or binary is in
//     out/host/linux-x86/nativetest/<test dir>/<test>
//   - "../../../lib[[64] to find libraries in out/host/linux-x86/lib64 when the test or binary is in
//     out/host/linux-x86/testcases/<test dir>/<CPU>/<test>
func RpathFlags(ctx android.ModuleContext) []string {
	key := struct {
		os   android.OsType
		arch android.ArchType
	}{ctx.Target().Os, ctx.Target().Arch.ArchType}

	return ctx.Config().OnceStringSlice(android.NewCustomOnceKey(key), func() []string {
		rpathPrefix := `\$$ORIGIN/`
		if key.os == android.Darwin {
			rpathPrefix = "@loader_path/"
		}

		var libDir string
		if key.arch.Multilib == "lib64" {
			libDir = "lib64"
		} else {
			libDir = "lib"
		}

		return []string{
			"-Wl,-rpath," + rpathPrefix,
			"-Wl,-rpath," + rpathPrefix + libDir,
			"-Wl,-rpath," + rpathPrefix + filepath.Join("..", libDir),
			"-Wl,-rpath," + rpathPrefix + filepath.Join("../..", libDir),
			"-Wl,-rpath," + rpathPrefix + filepath.Join("../../..", libDir),
		}
	})
}

func (linker *baseLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {
	panic(fmt.Errorf("baseLinker doesn't know how to link"))
}

func (linker *baseLinker) linkerSpecifiedDeps(ctx android.ConfigurableEvaluatorContext, module *Module, specifiedDeps specifiedDeps) specifiedDeps {
	eval := module.ConfigurableEvaluator(ctx)
	specifiedDeps.sharedLibs = append(specifiedDeps.sharedLibs, linker.Properties.Shared_libs.GetOrDefault(eval, nil)...)

	// Must distinguish nil and [] in system_shared_libs - ensure that [] in
	// either input list doesn't come out as nil.
	if specifiedDeps.systemSharedLibs == nil {
		specifiedDeps.systemSharedLibs = linker.Properties.System_shared_libs
	} else {
		specifiedDeps.systemSharedLibs = append(specifiedDeps.systemSharedLibs, linker.Properties.System_shared_libs...)
	}

	return specifiedDeps
}

func (linker *baseLinker) moduleInfoJSON(ctx ModuleContext, moduleInfoJSON *android.ModuleInfoJSON) {
}

// Injecting version symbols
// Some host modules want a version number, but we don't want to rebuild it every time.  Optionally add a step
// after linking that injects a constant placeholder with the current version number.

func init() {
	pctx.HostBinToolVariable("symbolInjectCmd", "symbol_inject")
}

var injectVersionSymbol = pctx.AndroidStaticRule("injectVersionSymbol",
	blueprint.RuleParams{
		Command: "$symbolInjectCmd -i $in -o $out -s soong_build_number " +
			"-from 'SOONG BUILD NUMBER PLACEHOLDER' -v $$(cat $buildNumberFile)",
		CommandDeps:     []string{"$symbolInjectCmd"},
		SandboxDisabled: true,
	},
	"buildNumberFile")

func (linker *baseLinker) injectVersionSymbol(ctx ModuleContext, in android.Path, out android.WritablePath) {
	buildNumberFile := ctx.Config().BuildNumberFile(ctx)
	ctx.Build(pctx, android.BuildParams{
		Rule:        injectVersionSymbol,
		Description: "inject version symbol",
		Input:       in,
		Output:      out,
		OrderOnly:   android.Paths{buildNumberFile},
		Args: map[string]string{
			"buildNumberFile": buildNumberFile.String(),
		},
	})
}

var checkElfFileRule = pctx.AndroidStaticRule("checkElfFile",
	blueprint.RuleParams{
		Command: "${checkElfFileCmd} --skip-unknown-elf-machine $args" +
			" --llvm-readobj=${config.ClangBin}/llvm-readobj $in" +
			" && touch $out",
		CommandDeps:     []string{"$checkElfFileCmd", "${config.ClangBin}/llvm-readobj"},
		SandboxDisabled: true,
	}, "args")

// checkElfFile creates a rule that runs a prebuilt ELF file (shared library or binary) through the check_elf_file
// command to verify that the DT_NEEDED entries in the prebuilt match the shared_libs listed in the Android.bp file,
// the DT_SONAME matches the module name, that symbols can be resolved and that the alignment matches the page size.
// The output file should be used as a validation dependency of the rule that copies the prebuilt.
func (linker *baseLinker) checkElfFile(ctx ModuleContext, sharedLibStem string, deps PathDeps, elfFile android.Path) android.Path {
	var args []string
	var implicits android.Paths
	if !Bool(linker.Properties.Ignore_max_page_size) && ctx.Config().DeviceCheckPrebuiltMaxPageSize() {
		args = append(args, "--max-page-size "+ctx.Config().MaxPageSizeSupported())
	}
	if Bool(linker.Properties.Allow_undefined_symbols) {
		args = append(args, "--allow-undefined-symbols")
	}
	if sharedLibStem != "" {
		args = append(args, "--soname "+sharedLibStem)
	}
	for _, sharedLib := range deps.EarlySharedLibs {
		args = append(args, "--shared-lib "+sharedLib.String())
		implicits = append(implicits, sharedLib)
	}
	for _, sharedLib := range deps.SharedLibs {
		args = append(args, "--shared-lib "+sharedLib.String())
		implicits = append(implicits, sharedLib)
	}
	for _, sharedLib := range deps.LateSharedLibs {
		args = append(args, "--shared-lib "+sharedLib.String())
		args = append(args, "--system-shared-lib "+sharedLib.Base())
		implicits = append(implicits, sharedLib)
	}

	outputFile := android.PathForModuleOut(ctx, elfFile.Base()+".check_elf_file")

	ctx.Build(pctx, android.BuildParams{
		Rule:        checkElfFileRule,
		Description: "check elf file",
		Output:      outputFile,
		Input:       elfFile,
		Implicits:   implicits,
		Args: map[string]string{
			"args": strings.Join(args, " "),
		},
	})

	return outputFile
}

// CheckPrebuiltElfFile runs check_elf_file on a prebuilt ELF file from another module type, resolving
// its DT_NEEDED entries and undefined symbols against sharedLibs.
func CheckPrebuiltElfFile(ctx android.ModuleContext, elfFile android.Path, sharedLibs android.Paths) android.Path {
	var args []string
	if ctx.Config().DeviceCheckPrebuiltMaxPageSize() {
		args = append(args, "--max-page-size "+ctx.Config().MaxPageSizeSupported())
	}
	for _, sharedLib := range sharedLibs {
		args = append(args, "--shared-lib "+sharedLib.String())
	}

	outputFile := android.PathForModuleOut(ctx, elfFile.Base()+".check_elf_file")

	ctx.Build(pctx, android.BuildParams{
		Rule:        checkElfFileRule,
		Description: "check elf file",
		Output:      outputFile,
		Input:       elfFile,
		Implicits:   sharedLibs,
		Args: map[string]string{
			"args": strings.Join(args, " "),
		},
	})

	return outputFile
}

func XomFlags(ctx android.ModuleContext) []string {
	flags := []string{}

	// XOM is only supported on arm64 devices currently.
	if ctx.Arch().ArchType != android.Arm64 {
		return flags
	}

	// If XOM is explicitly false, do nothing
	if XomDisabledByModule(ctx) {
		return flags
	}

	// XOM is only supported on LinkableInterfaces (Rust and CC modules)
	mod, ok := ctx.Module().(LinkableInterface)
	if !ok {
		return flags
	}

	enableXom := false
	// If XOM is enabled by the config and it's not disabled by path, enable XOM for this module
	if ctx.Config().EnableXOM() && !ctx.Config().XOMDisabledForPath(ctx.ModuleDir()) {
		enableXom = true
	}
	// If XOM is explicitly true, enable it for this module.
	if XomEnabledByModule(ctx) {
		enableXom = true
	}

	// If any static dependencies have XOM disabled, we should disable XOM in this module (unless explicitly set),
	// the assumption being if it's been explicitly disabled then there's probably incompatible
	// code in the library which may get pulled in.
	if enableXom {
		ctx.VisitDirectDepsProxy(func(dep android.ModuleProxy) {
			depInfo, hasLinkableInfo := android.OtherModuleProvider(ctx, dep, LinkableInfoProvider)

			// If the dep isn't statically linked or isn't a Linkable, ignore
			if !hasLinkableInfo || !(depInfo.Static || depInfo.Rlib) {
				return
			}

			// If XOM is explicitly disabled, disable it for this module
			if depInfo.Xom != nil && !*depInfo.Xom {
				enableXom = false
				return
			}

			// If XOM is disabled by path and XOM is not explicitly set to true, disable it for this module.
			if depInfo.XomDisabledByPath && !Bool(depInfo.Xom) {
				enableXom = false
				return
			}
		})
	}

	// Enable execute-only if none of the dependencies disable it,
	// or if it's explicitly set true (allows overriding deps or paths disabling it).
	if enableXom || Bool(mod.Xom()) {
		flags = append(flags,
			"-Wl,--execute-only",
			"-Wl,--rosegment",
		)
	}

	return flags
}

// XomDisabledByModule returns true if XOM is explicitly disabled by the module
// otherwise it returns false
func XomDisabledByModule(ctx android.BaseModuleContext) bool {
	c, ok := ctx.Module().(LinkableInterface)
	if ok && c.Xom() != nil && !*c.Xom() {
		return true
	}

	// disable XOM for Apex modules
	if info, ok := android.ModuleProvider(ctx, android.ApexInfoProvider); ok && !info.IsForPlatform() {
		return true
	}

	return false
}

// XomEnabledByModule returns true if XOM is explicitly enabled by the module
// otherwise it returns false
func XomEnabledByModule(ctx android.BaseModuleContext) bool {
	c, ok := ctx.Module().(LinkableInterface)
	if ok && c.Xom() != nil && *c.Xom() {
		return true
	}
	return false
}

// XomEnabledForModule returns whether the module implicitly or explicitly has XOM
// enabled. This differs from XomEnabledByModule, which checks if XOM is explicitly
// enabled.
func XomEnabledForModule(ctx android.BaseModuleContext) bool {
	return ctx.Arch().ArchType == android.Arm64 && ctx.Config().EnableXOM() &&
		!ctx.Config().XOMDisabledForPath(ctx.ModuleDir()) && !XomDisabledByModule(ctx)
}
