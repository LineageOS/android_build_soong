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
)

// This file contains the basic C/C++/assembly to .o compliation steps

type BaseCompilerProperties struct {
	// list of source files used to compile the C/C++ module.  May be .c, .cpp, or .S files.
	Srcs []string `android:"arch_variant"`

	// list of source files that should not be used to build the C/C++ module.
	// This is most useful in the arch/multilib variants to remove non-common files
	Exclude_srcs []string `android:"arch_variant"`

	// list of module-specific flags that will be used for C and C++ compiles.
	Cflags []string `android:"arch_variant"`

	// list of module-specific flags that will be used for C++ compiles
	Cppflags []string `android:"arch_variant"`

	// list of module-specific flags that will be used for C compiles
	Conlyflags []string `android:"arch_variant"`

	// list of module-specific flags that will be used for .S compiles
	Asflags []string `android:"arch_variant"`

	// list of module-specific flags that will be used for C and C++ compiles when
	// compiling with clang
	Clang_cflags []string `android:"arch_variant"`

	// list of module-specific flags that will be used for .S compiles when
	// compiling with clang
	Clang_asflags []string `android:"arch_variant"`

	// list of module-specific flags that will be used for .y and .yy compiles
	Yaccflags []string

	// the instruction set architecture to use to compile the C/C++
	// module.
	Instruction_set string `android:"arch_variant"`

	// list of directories relative to the root of the source tree that will
	// be added to the include path using -I.
	// If possible, don't use this.  If adding paths from the current directory use
	// local_include_dirs, if adding paths from other modules use export_include_dirs in
	// that module.
	Include_dirs []string `android:"arch_variant"`

	// list of directories relative to the Blueprints file that will
	// be added to the include path using -I
	Local_include_dirs []string `android:"arch_variant"`

	// list of generated sources to compile. These are the names of gensrcs or
	// genrule modules.
	Generated_sources []string `android:"arch_variant"`

	// list of generated headers to add to the include path. These are the names
	// of genrule modules.
	Generated_headers []string `android:"arch_variant"`

	// pass -frtti instead of -fno-rtti
	Rtti *bool

	Debug, Release struct {
		// list of module-specific flags that will be used for C and C++ compiles in debug or
		// release builds
		Cflags []string `android:"arch_variant"`
	} `android:"arch_variant"`
}

type baseCompiler struct {
	Properties BaseCompilerProperties
}

var _ compiler = (*baseCompiler)(nil)

func (compiler *baseCompiler) appendCflags(flags []string) {
	compiler.Properties.Cflags = append(compiler.Properties.Cflags, flags...)
}

func (compiler *baseCompiler) appendAsflags(flags []string) {
	compiler.Properties.Asflags = append(compiler.Properties.Asflags, flags...)
}

func (compiler *baseCompiler) props() []interface{} {
	return []interface{}{&compiler.Properties}
}

func (compiler *baseCompiler) begin(ctx BaseModuleContext) {}

func (compiler *baseCompiler) deps(ctx BaseModuleContext, deps Deps) Deps {
	deps.GeneratedSources = append(deps.GeneratedSources, compiler.Properties.Generated_sources...)
	deps.GeneratedHeaders = append(deps.GeneratedHeaders, compiler.Properties.Generated_headers...)

	return deps
}

// Create a Flags struct that collects the compile flags from global values,
// per-target values, module type values, and per-module Blueprints properties
func (compiler *baseCompiler) flags(ctx ModuleContext, flags Flags) Flags {
	toolchain := ctx.toolchain()

	CheckBadCompilerFlags(ctx, "cflags", compiler.Properties.Cflags)
	CheckBadCompilerFlags(ctx, "cppflags", compiler.Properties.Cppflags)
	CheckBadCompilerFlags(ctx, "conlyflags", compiler.Properties.Conlyflags)
	CheckBadCompilerFlags(ctx, "asflags", compiler.Properties.Asflags)

	flags.CFlags = append(flags.CFlags, compiler.Properties.Cflags...)
	flags.CppFlags = append(flags.CppFlags, compiler.Properties.Cppflags...)
	flags.ConlyFlags = append(flags.ConlyFlags, compiler.Properties.Conlyflags...)
	flags.AsFlags = append(flags.AsFlags, compiler.Properties.Asflags...)
	flags.YaccFlags = append(flags.YaccFlags, compiler.Properties.Yaccflags...)

	// Include dir cflags
	rootIncludeDirs := android.PathsForSource(ctx, compiler.Properties.Include_dirs)
	localIncludeDirs := android.PathsForModuleSrc(ctx, compiler.Properties.Local_include_dirs)
	flags.GlobalFlags = append(flags.GlobalFlags,
		includeDirsToFlags(localIncludeDirs),
		includeDirsToFlags(rootIncludeDirs))

	if !ctx.noDefaultCompilerFlags() {
		if !ctx.sdk() || ctx.Host() {
			flags.GlobalFlags = append(flags.GlobalFlags,
				"${commonGlobalIncludes}",
				toolchain.IncludeFlags(),
				"${commonNativehelperInclude}")
		}

		flags.GlobalFlags = append(flags.GlobalFlags, []string{
			"-I" + android.PathForModuleSrc(ctx).String(),
			"-I" + android.PathForModuleOut(ctx).String(),
			"-I" + android.PathForModuleGen(ctx).String(),
		}...)
	}

	if ctx.sdk() {
		// The NDK headers are installed to a common sysroot. While a more
		// typical Soong approach would be to only make the headers for the
		// library you're using available, we're trying to emulate the NDK
		// behavior here, and the NDK always has all the NDK headers available.
		flags.GlobalFlags = append(flags.GlobalFlags,
			"-isystem "+getCurrentIncludePath(ctx).String(),
			"-isystem "+getCurrentIncludePath(ctx).Join(ctx, toolchain.ClangTriple()).String())

		// Traditionally this has come from android/api-level.h, but with the
		// libc headers unified it must be set by the build system since we
		// don't have per-API level copies of that header now.
		flags.GlobalFlags = append(flags.GlobalFlags,
			"-D__ANDROID_API__="+ctx.sdkVersion())

		// Until the full NDK has been migrated to using ndk_headers, we still
		// need to add the legacy sysroot includes to get the full set of
		// headers.
		legacyIncludes := fmt.Sprintf(
			"prebuilts/ndk/current/platforms/android-%s/arch-%s/usr/include",
			ctx.sdkVersion(), ctx.Arch().ArchType.String())
		flags.GlobalFlags = append(flags.GlobalFlags, "-isystem "+legacyIncludes)
	}

	instructionSet := compiler.Properties.Instruction_set
	if flags.RequiredInstructionSet != "" {
		instructionSet = flags.RequiredInstructionSet
	}
	instructionSetFlags, err := toolchain.InstructionSetFlags(instructionSet)
	if flags.Clang {
		instructionSetFlags, err = toolchain.ClangInstructionSetFlags(instructionSet)
	}
	if err != nil {
		ctx.ModuleErrorf("%s", err)
	}

	CheckBadCompilerFlags(ctx, "release.cflags", compiler.Properties.Release.Cflags)

	// TODO: debug
	flags.CFlags = append(flags.CFlags, compiler.Properties.Release.Cflags...)

	if flags.Clang {
		CheckBadCompilerFlags(ctx, "clang_cflags", compiler.Properties.Clang_cflags)
		CheckBadCompilerFlags(ctx, "clang_asflags", compiler.Properties.Clang_asflags)

		flags.CFlags = clangFilterUnknownCflags(flags.CFlags)
		flags.CFlags = append(flags.CFlags, compiler.Properties.Clang_cflags...)
		flags.AsFlags = append(flags.AsFlags, compiler.Properties.Clang_asflags...)
		flags.CppFlags = clangFilterUnknownCflags(flags.CppFlags)
		flags.ConlyFlags = clangFilterUnknownCflags(flags.ConlyFlags)
		flags.LdFlags = clangFilterUnknownCflags(flags.LdFlags)

		target := "-target " + toolchain.ClangTriple()
		var gccPrefix string
		if !ctx.Darwin() {
			gccPrefix = "-B" + filepath.Join(toolchain.GccRoot(), toolchain.GccTriple(), "bin")
		}

		flags.CFlags = append(flags.CFlags, target, gccPrefix)
		flags.AsFlags = append(flags.AsFlags, target, gccPrefix)
		flags.LdFlags = append(flags.LdFlags, target, gccPrefix)
	}

	hod := "host"
	if ctx.Os().Class == android.Device {
		hod = "device"
	}

	if !ctx.noDefaultCompilerFlags() {
		flags.GlobalFlags = append(flags.GlobalFlags, instructionSetFlags)

		if flags.Clang {
			flags.AsFlags = append(flags.AsFlags, toolchain.ClangAsflags())
			flags.CppFlags = append(flags.CppFlags, "${commonClangGlobalCppflags}")
			flags.GlobalFlags = append(flags.GlobalFlags,
				toolchain.ClangCflags(),
				"${commonClangGlobalCflags}",
				fmt.Sprintf("${%sClangGlobalCflags}", hod))

			flags.ConlyFlags = append(flags.ConlyFlags, "${clangExtraConlyflags}")
		} else {
			flags.CppFlags = append(flags.CppFlags, "${commonGlobalCppflags}")
			flags.GlobalFlags = append(flags.GlobalFlags,
				toolchain.Cflags(),
				"${commonGlobalCflags}",
				fmt.Sprintf("${%sGlobalCflags}", hod))
		}

		if Bool(ctx.AConfig().ProductVariables.Brillo) {
			flags.GlobalFlags = append(flags.GlobalFlags, "-D__BRILLO__")
		}

		if ctx.Device() {
			if Bool(compiler.Properties.Rtti) {
				flags.CppFlags = append(flags.CppFlags, "-frtti")
			} else {
				flags.CppFlags = append(flags.CppFlags, "-fno-rtti")
			}
		}

		flags.AsFlags = append(flags.AsFlags, "-D__ASSEMBLY__")

		if flags.Clang {
			flags.CppFlags = append(flags.CppFlags, toolchain.ClangCppflags())
		} else {
			flags.CppFlags = append(flags.CppFlags, toolchain.Cppflags())
		}
	}

	if flags.Clang {
		flags.GlobalFlags = append(flags.GlobalFlags, toolchain.ToolchainClangCflags())
	} else {
		flags.GlobalFlags = append(flags.GlobalFlags, toolchain.ToolchainCflags())
	}

	if !ctx.sdk() {
		if ctx.Host() && !flags.Clang {
			// The host GCC doesn't support C++14 (and is deprecated, so likely
			// never will). Build these modules with C++11.
			flags.CppFlags = append(flags.CppFlags, "-std=gnu++11")
		} else {
			flags.CppFlags = append(flags.CppFlags, "-std=gnu++14")
		}
	}

	// We can enforce some rules more strictly in the code we own. strict
	// indicates if this is code that we can be stricter with. If we have
	// rules that we want to apply to *our* code (but maybe can't for
	// vendor/device specific things), we could extend this to be a ternary
	// value.
	strict := true
	if strings.HasPrefix(android.PathForModuleSrc(ctx).String(), "external/") {
		strict = false
	}

	// Can be used to make some annotations stricter for code we can fix
	// (such as when we mark functions as deprecated).
	if strict {
		flags.CFlags = append(flags.CFlags, "-DANDROID_STRICT")
	}

	return flags
}

func ndkPathDeps(ctx ModuleContext) android.Paths {
	if ctx.sdk() {
		// The NDK sysroot timestamp file depends on all the NDK sysroot files
		// (headers and libraries).
		return android.Paths{getNdkSysrootTimestampFile(ctx)}
	}
	return nil
}

func (compiler *baseCompiler) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Paths {
	pathDeps := deps.GeneratedHeaders
	pathDeps = append(pathDeps, ndkPathDeps(ctx)...)
	// Compile files listed in c.Properties.Srcs into objects
	objFiles := compiler.compileObjs(ctx, flags, "",
		compiler.Properties.Srcs, compiler.Properties.Exclude_srcs,
		deps.GeneratedSources, pathDeps)

	if ctx.Failed() {
		return nil
	}

	return objFiles
}

// Compile a list of source files into objects a specified subdirectory
func (compiler *baseCompiler) compileObjs(ctx android.ModuleContext, flags Flags,
	subdir string, srcFiles, excludes []string, extraSrcs, deps android.Paths) android.Paths {

	buildFlags := flagsToBuilderFlags(flags)

	inputFiles := ctx.ExpandSources(srcFiles, excludes)
	inputFiles = append(inputFiles, extraSrcs...)
	srcPaths, gendeps := genSources(ctx, inputFiles, buildFlags)

	deps = append(deps, gendeps...)
	deps = append(deps, flags.CFlagsDeps...)

	return TransformSourceToObj(ctx, subdir, srcPaths, buildFlags, deps)
}
