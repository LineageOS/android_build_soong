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
	"android/soong/android"
	"android/soong/cc/config"
	"fmt"
	"strconv"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// This file contains the basic functionality for linking against static libraries and shared
// libraries.  Final linking into libraries or executables is handled in library.go, binary.go, etc.

type BaseLinkerProperties struct {
	// list of modules whose object files should be linked into this module
	// in their entirety.  For static library modules, all of the .o files from the intermediate
	// directory of the dependency will be linked into this modules .a file.  For a shared library,
	// the dependency's .a file will be linked into this module using -Wl,--whole-archive.
	Whole_static_libs []string `android:"arch_variant,variant_prepend"`

	// list of modules that should be statically linked into this module.
	Static_libs []string `android:"arch_variant,variant_prepend"`

	// list of modules that should be dynamically linked into this module.
	Shared_libs []string `android:"arch_variant"`

	// list of modules that should only provide headers for this module.
	Header_libs []string `android:"arch_variant,variant_prepend"`

	// list of module-specific flags that will be used for all link steps
	Ldflags []string `android:"arch_variant"`

	// list of system libraries that will be dynamically linked to
	// shared library and executable modules.  If unset, generally defaults to libc,
	// libm, and libdl.  Set to [] to prevent linking against the defaults.
	System_shared_libs []string `android:"arch_variant"`

	// allow the module to contain undefined symbols.  By default,
	// modules cannot contain undefined symbols that are not satisified by their immediate
	// dependencies.  Set this flag to true to remove --no-undefined from the linker flags.
	// This flag should only be necessary for compiling low-level libraries like libc.
	Allow_undefined_symbols *bool `android:"arch_variant"`

	// don't link in libclang_rt.builtins-*.a
	No_libcrt *bool `android:"arch_variant"`

	// Use clang lld instead of gnu ld.
	Use_clang_lld *bool `android:"arch_variant"`

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
	Export_header_lib_headers []string `android:"arch_variant"`

	// list of generated headers to re-export include directories from. Entries must be
	// present in generated_headers.
	Export_generated_headers []string `android:"arch_variant"`

	// don't link in crt_begin and crt_end.  This flag should only be necessary for
	// compiling crt or libc.
	Nocrt *bool `android:"arch_variant"`

	// group static libraries.  This can resolve missing symbols issues with interdependencies
	// between static libraries, but it is generally better to order them correctly instead.
	Group_static_libs *bool `android:"arch_variant"`

	// list of modules that should be installed with this module.  This is similar to 'required'
	// but '.vendor' suffix will be appended to the module names if the shared libraries have
	// vendor variants and this module uses VNDK.
	Runtime_libs []string `android:"arch_variant"`

	Target struct {
		Vendor struct {
			// list of shared libs that only should be used to build the vendor
			// variant of the C/C++ module.
			Shared_libs []string

			// list of static libs that only should be used to build the vendor
			// variant of the C/C++ module.
			Static_libs []string

			// list of shared libs that should not be used to build the vendor variant
			// of the C/C++ module.
			Exclude_shared_libs []string

			// list of static libs that should not be used to build the vendor variant
			// of the C/C++ module.
			Exclude_static_libs []string

			// list of header libs that should not be used to build the vendor variant
			// of the C/C++ module.
			Exclude_header_libs []string

			// list of runtime libs that should not be installed along with the vendor
			// variant of the C/C++ module.
			Exclude_runtime_libs []string

			// version script for this vendor variant
			Version_script *string `android:"arch_variant"`
		}
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
		}
		Ramdisk struct {
			// list of static libs that only should be used to build the recovery
			// variant of the C/C++ module.
			Static_libs []string

			// list of shared libs that should not be used to build
			// the ramdisk variant of the C/C++ module.
			Exclude_shared_libs []string

			// list of static libs that should not be used to build
			// the ramdisk variant of the C/C++ module.
			Exclude_static_libs []string
		}
	}

	// make android::build:GetBuildNumber() available containing the build ID.
	Use_version_lib *bool `android:"arch_variant"`

	// Generate compact dynamic relocation table, default true.
	Pack_relocations *bool `android:"arch_variant"`

	// local file name to pass to the linker as --version_script
	Version_script *string `android:"path,arch_variant"`

	// list of static libs that should not be used to build this module
	Exclude_static_libs []string
}

func NewBaseLinker(sanitize *sanitize) *baseLinker {
	return &baseLinker{sanitize: sanitize}
}

// baseLinker provides support for shared_libs, static_libs, and whole_static_libs properties
type baseLinker struct {
	Properties        BaseLinkerProperties
	dynamicProperties struct {
		RunPaths   []string `blueprint:"mutated"`
		BuildStubs bool     `blueprint:"mutated"`
	}

	sanitize *sanitize
}

func (linker *baseLinker) appendLdflags(flags []string) {
	linker.Properties.Ldflags = append(linker.Properties.Ldflags, flags...)
}

func (linker *baseLinker) linkerInit(ctx BaseModuleContext) {
	if ctx.toolchain().Is64Bit() {
		linker.dynamicProperties.RunPaths = append(linker.dynamicProperties.RunPaths, "../lib64", "lib64")
	} else {
		linker.dynamicProperties.RunPaths = append(linker.dynamicProperties.RunPaths, "../lib", "lib")
	}
}

func (linker *baseLinker) linkerProps() []interface{} {
	return []interface{}{&linker.Properties, &linker.dynamicProperties}
}

func (linker *baseLinker) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps.WholeStaticLibs = append(deps.WholeStaticLibs, linker.Properties.Whole_static_libs...)
	deps.HeaderLibs = append(deps.HeaderLibs, linker.Properties.Header_libs...)
	deps.StaticLibs = append(deps.StaticLibs, linker.Properties.Static_libs...)
	deps.SharedLibs = append(deps.SharedLibs, linker.Properties.Shared_libs...)
	deps.RuntimeLibs = append(deps.RuntimeLibs, linker.Properties.Runtime_libs...)

	deps.ReexportHeaderLibHeaders = append(deps.ReexportHeaderLibHeaders, linker.Properties.Export_header_lib_headers...)
	deps.ReexportStaticLibHeaders = append(deps.ReexportStaticLibHeaders, linker.Properties.Export_static_lib_headers...)
	deps.ReexportSharedLibHeaders = append(deps.ReexportSharedLibHeaders, linker.Properties.Export_shared_lib_headers...)
	deps.ReexportGeneratedHeaders = append(deps.ReexportGeneratedHeaders, linker.Properties.Export_generated_headers...)

	deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, linker.Properties.Exclude_static_libs)

	if Bool(linker.Properties.Use_version_lib) {
		deps.WholeStaticLibs = append(deps.WholeStaticLibs, "libbuildversion")
	}

	if ctx.useVndk() {
		deps.SharedLibs = append(deps.SharedLibs, linker.Properties.Target.Vendor.Shared_libs...)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, linker.Properties.Target.Vendor.Exclude_shared_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, linker.Properties.Target.Vendor.Exclude_shared_libs)
		deps.StaticLibs = append(deps.StaticLibs, linker.Properties.Target.Vendor.Static_libs...)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, linker.Properties.Target.Vendor.Exclude_static_libs)
		deps.HeaderLibs = removeListFromList(deps.HeaderLibs, linker.Properties.Target.Vendor.Exclude_header_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, linker.Properties.Target.Vendor.Exclude_static_libs)
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, linker.Properties.Target.Vendor.Exclude_static_libs)
		deps.RuntimeLibs = removeListFromList(deps.RuntimeLibs, linker.Properties.Target.Vendor.Exclude_runtime_libs)
	}

	if ctx.inRecovery() {
		deps.SharedLibs = append(deps.SharedLibs, linker.Properties.Target.Recovery.Shared_libs...)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, linker.Properties.Target.Recovery.Exclude_shared_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, linker.Properties.Target.Recovery.Exclude_shared_libs)
		deps.StaticLibs = append(deps.StaticLibs, linker.Properties.Target.Recovery.Static_libs...)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, linker.Properties.Target.Recovery.Exclude_static_libs)
		deps.HeaderLibs = removeListFromList(deps.HeaderLibs, linker.Properties.Target.Recovery.Exclude_header_libs)
		deps.ReexportHeaderLibHeaders = removeListFromList(deps.ReexportHeaderLibHeaders, linker.Properties.Target.Recovery.Exclude_header_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, linker.Properties.Target.Recovery.Exclude_static_libs)
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, linker.Properties.Target.Recovery.Exclude_static_libs)
	}

	if ctx.inRamdisk() {
		deps.SharedLibs = removeListFromList(deps.SharedLibs, linker.Properties.Target.Recovery.Exclude_shared_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, linker.Properties.Target.Recovery.Exclude_shared_libs)
		deps.StaticLibs = append(deps.StaticLibs, linker.Properties.Target.Recovery.Static_libs...)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, linker.Properties.Target.Recovery.Exclude_static_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, linker.Properties.Target.Recovery.Exclude_static_libs)
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, linker.Properties.Target.Recovery.Exclude_static_libs)
	}

	if ctx.toolchain().Bionic() {
		// libclang_rt.builtins and libatomic have to be last on the command line
		if !Bool(linker.Properties.No_libcrt) {
			deps.LateStaticLibs = append(deps.LateStaticLibs, config.BuiltinsRuntimeLibrary(ctx.toolchain()))
			deps.LateStaticLibs = append(deps.LateStaticLibs, "libatomic")
		}

		systemSharedLibs := linker.Properties.System_shared_libs
		if systemSharedLibs == nil {
			// Provide a default system_shared_libs if it is unspecified. Note: If an
			// empty list [] is specified, it implies that the module declines the
			// default system_shared_libs.
			systemSharedLibs = []string{"libc", "libm", "libdl"}
		}

		if inList("libdl", deps.SharedLibs) {
			// If system_shared_libs has libc but not libdl, make sure shared_libs does not
			// have libdl to avoid loading libdl before libc.
			if inList("libc", systemSharedLibs) {
				if !inList("libdl", systemSharedLibs) {
					ctx.PropertyErrorf("shared_libs",
						"libdl must be in system_shared_libs, not shared_libs")
				}
				_, deps.SharedLibs = removeFromList("libdl", deps.SharedLibs)
			}
		}

		if inList("libc_scudo", deps.SharedLibs) {
			// libc_scudo is an alternate implementation of all
			// allocation functions (malloc, free), that uses
			// the scudo allocator instead of the default native
			// allocator. If this library is in the list, make
			// sure it's first so it properly overrides the
			// allocation functions of all other shared libraries.
			_, deps.SharedLibs = removeFromList("libc_scudo", deps.SharedLibs)
			deps.SharedLibs = append([]string{"libc_scudo"}, deps.SharedLibs...)
		}

		// If libc and libdl are both in system_shared_libs make sure libdl comes after libc
		// to avoid loading libdl before libc.
		if inList("libdl", systemSharedLibs) && inList("libc", systemSharedLibs) &&
			indexList("libdl", systemSharedLibs) < indexList("libc", systemSharedLibs) {
			ctx.PropertyErrorf("system_shared_libs", "libdl must be after libc")
		}

		deps.LateSharedLibs = append(deps.LateSharedLibs, systemSharedLibs...)
	}

	if ctx.Fuchsia() {
		if ctx.ModuleName() != "libbioniccompat" &&
			ctx.ModuleName() != "libcompiler_rt-extras" &&
			ctx.ModuleName() != "libcompiler_rt" {
			deps.StaticLibs = append(deps.StaticLibs, "libbioniccompat")
		}
		if ctx.ModuleName() != "libcompiler_rt" && ctx.ModuleName() != "libcompiler_rt-extras" {
			deps.LateStaticLibs = append(deps.LateStaticLibs, "libcompiler_rt")
		}

	}

	if ctx.Windows() {
		deps.LateStaticLibs = append(deps.LateStaticLibs, "libwinpthread")
	}

	return deps
}

func (linker *baseLinker) useClangLld(ctx ModuleContext) bool {
	// Clang lld is not ready for for Darwin host executables yet.
	// See https://lld.llvm.org/AtomLLD.html for status of lld for Mach-O.
	if ctx.Darwin() {
		return false
	}
	if linker.Properties.Use_clang_lld != nil {
		return Bool(linker.Properties.Use_clang_lld)
	}
	return true
}

// Check whether the SDK version is not older than the specific one
func CheckSdkVersionAtLeast(ctx ModuleContext, SdkVersion int) bool {
	if ctx.sdkVersion() == "current" {
		return true
	}
	parsedSdkVersion, err := strconv.Atoi(ctx.sdkVersion())
	if err != nil {
		ctx.PropertyErrorf("sdk_version",
			"Invalid sdk_version value (must be int or current): %q",
			ctx.sdkVersion())
	}
	if parsedSdkVersion < SdkVersion {
		return false
	}
	return true
}

// ModuleContext extends BaseModuleContext
// BaseModuleContext should know if LLD is used?
func (linker *baseLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	toolchain := ctx.toolchain()

	hod := "Host"
	if ctx.Os().Class == android.Device {
		hod = "Device"
	}

	if linker.useClangLld(ctx) {
		flags.Global.LdFlags = append(flags.Global.LdFlags, fmt.Sprintf("${config.%sGlobalLldflags}", hod))
		if !BoolDefault(linker.Properties.Pack_relocations, true) {
			flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,--pack-dyn-relocs=none")
		} else if ctx.Device() {
			// The SHT_RELR relocations is only supported by API level >= 28.
			// Do not turn this on if older version NDK is used.
			if !ctx.useSdk() || CheckSdkVersionAtLeast(ctx, 28) {
				flags.Global.LdFlags = append(flags.Global.LdFlags,
					"-Wl,--pack-dyn-relocs=android+relr",
					"-Wl,--use-android-relr-tags")
			} else if CheckSdkVersionAtLeast(ctx, 23) {
				flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,--pack-dyn-relocs=android")
			}
		}
	} else {
		flags.Global.LdFlags = append(flags.Global.LdFlags, fmt.Sprintf("${config.%sGlobalLdflags}", hod))
	}
	if Bool(linker.Properties.Allow_undefined_symbols) {
		if ctx.Darwin() {
			// darwin defaults to treating undefined symbols as errors
			flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,-undefined,dynamic_lookup")
		}
	} else if !ctx.Darwin() && !ctx.Windows() {
		flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,--no-undefined")
	}

	if linker.useClangLld(ctx) {
		flags.Global.LdFlags = append(flags.Global.LdFlags, toolchain.ClangLldflags())
	} else {
		flags.Global.LdFlags = append(flags.Global.LdFlags, toolchain.ClangLdflags())
	}

	if !ctx.toolchain().Bionic() && !ctx.Fuchsia() {
		CheckBadHostLdlibs(ctx, "host_ldlibs", linker.Properties.Host_ldlibs)

		flags.Local.LdFlags = append(flags.Local.LdFlags, linker.Properties.Host_ldlibs...)

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

	if ctx.Fuchsia() {
		flags.Global.LdFlags = append(flags.Global.LdFlags, "-lfdio", "-lzircon")
	}

	if ctx.toolchain().LibclangRuntimeLibraryArch() != "" {
		flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,--exclude-libs="+config.BuiltinsRuntimeLibrary(ctx.toolchain())+".a")
	}

	CheckBadLinkerFlags(ctx, "ldflags", linker.Properties.Ldflags)

	flags.Local.LdFlags = append(flags.Local.LdFlags, proptools.NinjaAndShellEscapeList(linker.Properties.Ldflags)...)

	if ctx.Host() && !ctx.Windows() {
		rpath_prefix := `\$$ORIGIN/`
		if ctx.Darwin() {
			rpath_prefix = "@loader_path/"
		}

		if !ctx.static() {
			for _, rpath := range linker.dynamicProperties.RunPaths {
				flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,-rpath,"+rpath_prefix+rpath)
			}
		}
	}

	if ctx.useSdk() && (ctx.Arch().ArchType != android.Mips && ctx.Arch().ArchType != android.Mips64) {
		// The bionic linker now has support gnu style hashes (which are much faster!), but shipping
		// to older devices requires the old style hash. Fortunately, we can build with both and
		// it'll work anywhere.
		// This is not currently supported on MIPS architectures.
		flags.Global.LdFlags = append(flags.Global.LdFlags, "-Wl,--hash-style=both")
	}

	flags.Global.LdFlags = append(flags.Global.LdFlags, toolchain.ToolchainClangLdflags())

	if Bool(linker.Properties.Group_static_libs) {
		flags.GroupStaticLibs = true
	}

	// Version_script is not needed when linking stubs lib where the version
	// script is created from the symbol map file.
	if !linker.dynamicProperties.BuildStubs {
		versionScript := ctx.ExpandOptionalSource(
			linker.Properties.Version_script, "version_script")

		if ctx.useVndk() && linker.Properties.Target.Vendor.Version_script != nil {
			versionScript = ctx.ExpandOptionalSource(
				linker.Properties.Target.Vendor.Version_script,
				"target.vendor.version_script")
		}

		if versionScript.Valid() {
			if ctx.Darwin() {
				ctx.PropertyErrorf("version_script", "Not supported on Darwin")
			} else {
				flags.Local.LdFlags = append(flags.Local.LdFlags,
					"-Wl,--version-script,"+versionScript.String())
				flags.LdFlagsDeps = append(flags.LdFlagsDeps, versionScript.Path())

				if linker.sanitize.isSanitizerEnabled(cfi) {
					cfiExportsMap := android.PathForSource(ctx, cfiExportsMapPath)
					flags.Local.LdFlags = append(flags.Local.LdFlags,
						"-Wl,--version-script,"+cfiExportsMap.String())
					flags.LdFlagsDeps = append(flags.LdFlagsDeps, cfiExportsMap)
				}
			}
		}
	}

	return flags
}

func (linker *baseLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {
	panic(fmt.Errorf("baseLinker doesn't know how to link"))
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
		CommandDeps: []string{"$symbolInjectCmd"},
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

// Rule to generate .bss symbol ordering file.

var (
	_                      = pctx.SourcePathVariable("genSortedBssSymbolsPath", "build/soong/scripts/gen_sorted_bss_symbols.sh")
	gen_sorted_bss_symbols = pctx.AndroidStaticRule("gen_sorted_bss_symbols",
		blueprint.RuleParams{
			Command:     "CROSS_COMPILE=$crossCompile $genSortedBssSymbolsPath ${in} ${out}",
			CommandDeps: []string{"$genSortedBssSymbolsPath", "${crossCompile}nm"},
		},
		"crossCompile")
)

func (linker *baseLinker) sortBssSymbolsBySize(ctx ModuleContext, in android.Path, symbolOrderingFile android.ModuleOutPath, flags builderFlags) string {
	crossCompile := gccCmd(flags.toolchain, "")
	ctx.Build(pctx, android.BuildParams{
		Rule:        gen_sorted_bss_symbols,
		Description: "generate bss symbol order " + symbolOrderingFile.Base(),
		Output:      symbolOrderingFile,
		Input:       in,
		Args: map[string]string{
			"crossCompile": crossCompile,
		},
	})
	return "-Wl,--symbol-ordering-file," + symbolOrderingFile.String()
}
