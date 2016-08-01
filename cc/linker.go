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

	// list of module-specific flags that will be used for all link steps
	Ldflags []string `android:"arch_variant"`

	// don't insert default compiler flags into asflags, cflags,
	// cppflags, conlyflags, ldflags, or include_dirs
	No_default_compiler_flags *bool

	// list of system libraries that will be dynamically linked to
	// shared library and executable modules.  If unset, generally defaults to libc
	// and libm.  Set to [] to prevent linking against libc and libm.
	System_shared_libs []string

	// allow the module to contain undefined symbols.  By default,
	// modules cannot contain undefined symbols that are not satisified by their immediate
	// dependencies.  Set this flag to true to remove --no-undefined from the linker flags.
	// This flag should only be necessary for compiling low-level libraries like libc.
	Allow_undefined_symbols *bool

	// don't link in libgcc.a
	No_libgcc *bool

	// -l arguments to pass to linker for host-provided shared libraries
	Host_ldlibs []string `android:"arch_variant"`

	// list of shared libraries to re-export include directories from. Entries must be
	// present in shared_libs.
	Export_shared_lib_headers []string `android:"arch_variant"`

	// list of static libraries to re-export include directories from. Entries must be
	// present in static_libs.
	Export_static_lib_headers []string `android:"arch_variant"`

	// don't link in crt_begin and crt_end.  This flag should only be necessary for
	// compiling crt or libc.
	Nocrt *bool `android:"arch_variant"`
}

// baseLinker provides support for shared_libs, static_libs, and whole_static_libs properties
type baseLinker struct {
	Properties        BaseLinkerProperties
	dynamicProperties struct {
		VariantIsShared       bool     `blueprint:"mutated"`
		VariantIsStatic       bool     `blueprint:"mutated"`
		VariantIsStaticBinary bool     `blueprint:"mutated"`
		RunPaths              []string `blueprint:"mutated"`
	}
}

func (linker *baseLinker) appendLdflags(flags []string) {
	linker.Properties.Ldflags = append(linker.Properties.Ldflags, flags...)
}

func (linker *baseLinker) linkerInit(ctx BaseModuleContext) {
	if ctx.toolchain().Is64Bit() {
		linker.dynamicProperties.RunPaths = []string{"../lib64", "lib64"}
	} else {
		linker.dynamicProperties.RunPaths = []string{"../lib", "lib"}
	}
}

func (linker *baseLinker) linkerProps() []interface{} {
	return []interface{}{&linker.Properties, &linker.dynamicProperties}
}

func (linker *baseLinker) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	deps.WholeStaticLibs = append(deps.WholeStaticLibs, linker.Properties.Whole_static_libs...)
	deps.StaticLibs = append(deps.StaticLibs, linker.Properties.Static_libs...)
	deps.SharedLibs = append(deps.SharedLibs, linker.Properties.Shared_libs...)

	deps.ReexportStaticLibHeaders = append(deps.ReexportStaticLibHeaders, linker.Properties.Export_static_lib_headers...)
	deps.ReexportSharedLibHeaders = append(deps.ReexportSharedLibHeaders, linker.Properties.Export_shared_lib_headers...)

	if !ctx.sdk() && ctx.ModuleName() != "libcompiler_rt-extras" {
		deps.LateStaticLibs = append(deps.LateStaticLibs, "libcompiler_rt-extras")
	}

	if ctx.Device() {
		// libgcc and libatomic have to be last on the command line
		deps.LateStaticLibs = append(deps.LateStaticLibs, "libatomic")
		if !Bool(linker.Properties.No_libgcc) {
			deps.LateStaticLibs = append(deps.LateStaticLibs, "libgcc")
		}

		if !linker.static() {
			if linker.Properties.System_shared_libs != nil {
				deps.LateSharedLibs = append(deps.LateSharedLibs,
					linker.Properties.System_shared_libs...)
			} else if !ctx.sdk() {
				deps.LateSharedLibs = append(deps.LateSharedLibs, "libc", "libm")
			}
		}

		if ctx.sdk() {
			deps.SharedLibs = append(deps.SharedLibs,
				"libc",
				"libm",
			)
		}
	}

	return deps
}

func (linker *baseLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	toolchain := ctx.toolchain()

	flags.Nocrt = Bool(linker.Properties.Nocrt)

	if !ctx.noDefaultCompilerFlags() {
		if ctx.Device() && !Bool(linker.Properties.Allow_undefined_symbols) {
			flags.LdFlags = append(flags.LdFlags, "-Wl,--no-undefined")
		}

		if flags.Clang {
			flags.LdFlags = append(flags.LdFlags, toolchain.ClangLdflags())
		} else {
			flags.LdFlags = append(flags.LdFlags, toolchain.Ldflags())
		}

		if ctx.Host() {
			CheckBadHostLdlibs(ctx, "host_ldlibs", linker.Properties.Host_ldlibs)

			flags.LdFlags = append(flags.LdFlags, linker.Properties.Host_ldlibs...)
		}
	}

	CheckBadLinkerFlags(ctx, "ldflags", linker.Properties.Ldflags)

	flags.LdFlags = append(flags.LdFlags, linker.Properties.Ldflags...)

	if ctx.Host() && !linker.static() {
		rpath_prefix := `\$$ORIGIN/`
		if ctx.Darwin() {
			rpath_prefix = "@loader_path/"
		}

		for _, rpath := range linker.dynamicProperties.RunPaths {
			flags.LdFlags = append(flags.LdFlags, "-Wl,-rpath,"+rpath_prefix+rpath)
		}
	}

	if flags.Clang {
		flags.LdFlags = append(flags.LdFlags, toolchain.ToolchainClangLdflags())
	} else {
		flags.LdFlags = append(flags.LdFlags, toolchain.ToolchainLdflags())
	}

	return flags
}

func (linker *baseLinker) static() bool {
	return linker.dynamicProperties.VariantIsStatic
}

func (linker *baseLinker) staticBinary() bool {
	return linker.dynamicProperties.VariantIsStaticBinary
}

func (linker *baseLinker) setStatic(static bool) {
	linker.dynamicProperties.VariantIsStatic = static
}

func (linker *baseLinker) isDependencyRoot() bool {
	return false
}

type baseLinkerInterface interface {
	// Returns true if the build options for the module have selected a static or shared build
	buildStatic() bool
	buildShared() bool

	// Sets whether a specific variant is static or shared
	setStatic(bool)

	// Returns whether a specific variant is a static library or binary
	static() bool

	// Returns whether a module is a static binary
	staticBinary() bool

	// Returns true for dependency roots (binaries)
	// TODO(ccross): also handle dlopenable libraries
	isDependencyRoot() bool
}
