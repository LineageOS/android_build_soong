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
	"fmt"

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
	System_shared_libs []string

	// allow the module to contain undefined symbols.  By default,
	// modules cannot contain undefined symbols that are not satisified by their immediate
	// dependencies.  Set this flag to true to remove --no-undefined from the linker flags.
	// This flag should only be necessary for compiling low-level libraries like libc.
	Allow_undefined_symbols *bool `android:"arch_variant"`

	// don't link in libgcc.a
	No_libgcc *bool

	// Use clang lld instead of gnu ld.
	Use_clang_lld *bool

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

	Target struct {
		Vendor struct {
			// list of shared libs that should not be used to build
			// the vendor variant of the C/C++ module.
			Exclude_shared_libs []string

			// list of static libs that should not be used to build
			// the vendor variant of the C/C++ module.
			Exclude_static_libs []string
		}
	}

	// make android::build:GetBuildNumber() available containing the build ID.
	Use_version_lib *bool `android:"arch_variant"`
}

func NewBaseLinker() *baseLinker {
	return &baseLinker{}
}

// baseLinker provides support for shared_libs, static_libs, and whole_static_libs properties
type baseLinker struct {
	Properties        BaseLinkerProperties
	dynamicProperties struct {
		RunPaths []string `blueprint:"mutated"`
	}
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

func (linker *baseLinker) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	deps.WholeStaticLibs = append(deps.WholeStaticLibs, linker.Properties.Whole_static_libs...)
	deps.HeaderLibs = append(deps.HeaderLibs, linker.Properties.Header_libs...)
	deps.StaticLibs = append(deps.StaticLibs, linker.Properties.Static_libs...)
	deps.SharedLibs = append(deps.SharedLibs, linker.Properties.Shared_libs...)

	deps.ReexportHeaderLibHeaders = append(deps.ReexportHeaderLibHeaders, linker.Properties.Export_header_lib_headers...)
	deps.ReexportStaticLibHeaders = append(deps.ReexportStaticLibHeaders, linker.Properties.Export_static_lib_headers...)
	deps.ReexportSharedLibHeaders = append(deps.ReexportSharedLibHeaders, linker.Properties.Export_shared_lib_headers...)
	deps.ReexportGeneratedHeaders = append(deps.ReexportGeneratedHeaders, linker.Properties.Export_generated_headers...)

	if Bool(linker.Properties.Use_version_lib) {
		deps.WholeStaticLibs = append(deps.WholeStaticLibs, "libbuildversion")
	}

	if ctx.useVndk() {
		deps.SharedLibs = removeListFromList(deps.SharedLibs, linker.Properties.Target.Vendor.Exclude_shared_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, linker.Properties.Target.Vendor.Exclude_shared_libs)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, linker.Properties.Target.Vendor.Exclude_static_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, linker.Properties.Target.Vendor.Exclude_static_libs)
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, linker.Properties.Target.Vendor.Exclude_static_libs)
	}

	if ctx.ModuleName() != "libcompiler_rt-extras" {
		deps.LateStaticLibs = append(deps.LateStaticLibs, "libcompiler_rt-extras")
	}

	if ctx.toolchain().Bionic() {
		// libgcc and libatomic have to be last on the command line
		deps.LateStaticLibs = append(deps.LateStaticLibs, "libatomic")
		if !Bool(linker.Properties.No_libgcc) {
			deps.LateStaticLibs = append(deps.LateStaticLibs, "libgcc")
		}

		if !ctx.static() {
			systemSharedLibs := linker.Properties.System_shared_libs
			if systemSharedLibs == nil {
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

			// If libc and libdl are both in system_shared_libs make sure libd comes after libc
			// to avoid loading libdl before libc.
			if inList("libdl", systemSharedLibs) && inList("libc", systemSharedLibs) &&
				indexList("libdl", systemSharedLibs) < indexList("libc", systemSharedLibs) {
				ctx.PropertyErrorf("system_shared_libs", "libdl must be after libc")
			}

			deps.LateSharedLibs = append(deps.LateSharedLibs, systemSharedLibs...)
		} else if ctx.useSdk() || ctx.useVndk() {
			deps.LateSharedLibs = append(deps.LateSharedLibs, "libc", "libm", "libdl")
		}
	}

	if ctx.Windows() {
		deps.LateStaticLibs = append(deps.LateStaticLibs, "libwinpthread")
	}

	return deps
}

func (linker *baseLinker) useClangLld(ctx ModuleContext) bool {
	if linker.Properties.Use_clang_lld != nil {
		return Bool(linker.Properties.Use_clang_lld)
	}
	return ctx.Config().UseClangLld()
}

// ModuleContext extends BaseModuleContext
// BaseModuleContext should know if LLD is used?
func (linker *baseLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	toolchain := ctx.toolchain()

	hod := "Host"
	if ctx.Os().Class == android.Device {
		hod = "Device"
	}

	if flags.Clang && linker.useClangLld(ctx) {
		flags.LdFlags = append(flags.LdFlags, fmt.Sprintf("${config.%sGlobalLldflags}", hod))
	} else {
		flags.LdFlags = append(flags.LdFlags, fmt.Sprintf("${config.%sGlobalLdflags}", hod))
	}
	if Bool(linker.Properties.Allow_undefined_symbols) {
		if ctx.Darwin() {
			// darwin defaults to treating undefined symbols as errors
			flags.LdFlags = append(flags.LdFlags, "-Wl,-undefined,dynamic_lookup")
		}
	} else if !ctx.Darwin() {
		flags.LdFlags = append(flags.LdFlags, "-Wl,--no-undefined")
	}

	if flags.Clang && linker.useClangLld(ctx) {
		flags.LdFlags = append(flags.LdFlags, toolchain.ClangLldflags())
	} else if flags.Clang {
		flags.LdFlags = append(flags.LdFlags, toolchain.ClangLdflags())
	} else {
		flags.LdFlags = append(flags.LdFlags, toolchain.Ldflags())
	}

	if !ctx.toolchain().Bionic() {
		CheckBadHostLdlibs(ctx, "host_ldlibs", linker.Properties.Host_ldlibs)

		flags.LdFlags = append(flags.LdFlags, linker.Properties.Host_ldlibs...)

		if !ctx.Windows() {
			// Add -ldl, -lpthread, -lm and -lrt to host builds to match the default behavior of device
			// builds
			flags.LdFlags = append(flags.LdFlags,
				"-ldl",
				"-lpthread",
				"-lm",
			)
			if !ctx.Darwin() {
				flags.LdFlags = append(flags.LdFlags, "-lrt")
			}
		}
	}

	CheckBadLinkerFlags(ctx, "ldflags", linker.Properties.Ldflags)

	flags.LdFlags = append(flags.LdFlags, proptools.NinjaAndShellEscape(linker.Properties.Ldflags)...)

	if ctx.Host() {
		rpath_prefix := `\$$ORIGIN/`
		if ctx.Darwin() {
			rpath_prefix = "@loader_path/"
		}

		if !ctx.static() {
			for _, rpath := range linker.dynamicProperties.RunPaths {
				flags.LdFlags = append(flags.LdFlags, "-Wl,-rpath,"+rpath_prefix+rpath)
			}
		}
	}

	if ctx.useSdk() && (ctx.Arch().ArchType != android.Mips && ctx.Arch().ArchType != android.Mips64) {
		// The bionic linker now has support gnu style hashes (which are much faster!), but shipping
		// to older devices requires the old style hash. Fortunately, we can build with both and
		// it'll work anywhere.
		// This is not currently supported on MIPS architectures.
		flags.LdFlags = append(flags.LdFlags, "-Wl,--hash-style=both")
	}

	if flags.Clang {
		flags.LdFlags = append(flags.LdFlags, toolchain.ToolchainClangLdflags())
	} else {
		flags.LdFlags = append(flags.LdFlags, toolchain.ToolchainLdflags())
	}

	if Bool(linker.Properties.Group_static_libs) {
		flags.GroupStaticLibs = true
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
			"-from 'SOONG BUILD NUMBER PLACEHOLDER' -v $buildNumberFromFile",
		CommandDeps: []string{"$symbolInjectCmd"},
	},
	"buildNumberFromFile")

func (linker *baseLinker) injectVersionSymbol(ctx ModuleContext, in android.Path, out android.WritablePath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        injectVersionSymbol,
		Description: "inject version symbol",
		Input:       in,
		Output:      out,
		Args: map[string]string{
			"buildNumberFromFile": ctx.Config().BuildNumberFromFile(),
		},
	})
}
