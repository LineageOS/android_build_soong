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

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
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

	// if set to false, use -std=c++* instead of -std=gnu++*
	Gnu_extensions *bool

	Debug, Release struct {
		// list of module-specific flags that will be used for C and C++ compiles in debug or
		// release builds
		Cflags []string `android:"arch_variant"`
	} `android:"arch_variant"`
}

func NewBaseCompiler() *baseCompiler {
	return &baseCompiler{}
}

type baseCompiler struct {
	Properties BaseCompilerProperties
	Proto      ProtoProperties
	deps       android.Paths
}

var _ compiler = (*baseCompiler)(nil)

func (compiler *baseCompiler) appendCflags(flags []string) {
	compiler.Properties.Cflags = append(compiler.Properties.Cflags, flags...)
}

func (compiler *baseCompiler) appendAsflags(flags []string) {
	compiler.Properties.Asflags = append(compiler.Properties.Asflags, flags...)
}

func (compiler *baseCompiler) compilerProps() []interface{} {
	return []interface{}{&compiler.Properties}
}

func (compiler *baseCompiler) compilerInit(ctx BaseModuleContext) {}

func (compiler *baseCompiler) compilerDeps(ctx BaseModuleContext, deps Deps) Deps {
	deps.GeneratedSources = append(deps.GeneratedSources, compiler.Properties.Generated_sources...)
	deps.GeneratedHeaders = append(deps.GeneratedHeaders, compiler.Properties.Generated_headers...)

	if compiler.hasProto() {
		deps = protoDeps(ctx, deps, &compiler.Proto)
	}

	return deps
}

// Create a Flags struct that collects the compile flags from global values,
// per-target values, module type values, and per-module Blueprints properties
func (compiler *baseCompiler) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	tc := ctx.toolchain()

	CheckBadCompilerFlags(ctx, "cflags", compiler.Properties.Cflags)
	CheckBadCompilerFlags(ctx, "cppflags", compiler.Properties.Cppflags)
	CheckBadCompilerFlags(ctx, "conlyflags", compiler.Properties.Conlyflags)
	CheckBadCompilerFlags(ctx, "asflags", compiler.Properties.Asflags)

	esc := proptools.NinjaAndShellEscape

	flags.CFlags = append(flags.CFlags, esc(compiler.Properties.Cflags)...)
	flags.CppFlags = append(flags.CppFlags, esc(compiler.Properties.Cppflags)...)
	flags.ConlyFlags = append(flags.ConlyFlags, esc(compiler.Properties.Conlyflags)...)
	flags.AsFlags = append(flags.AsFlags, esc(compiler.Properties.Asflags)...)
	flags.YaccFlags = append(flags.YaccFlags, esc(compiler.Properties.Yaccflags)...)

	// Include dir cflags
	rootIncludeDirs := android.PathsForSource(ctx, compiler.Properties.Include_dirs)
	localIncludeDirs := android.PathsForModuleSrc(ctx, compiler.Properties.Local_include_dirs)
	flags.GlobalFlags = append(flags.GlobalFlags,
		includeDirsToFlags(localIncludeDirs),
		includeDirsToFlags(rootIncludeDirs))

	if !ctx.noDefaultCompilerFlags() {
		if !ctx.sdk() || ctx.Host() {
			flags.GlobalFlags = append(flags.GlobalFlags,
				"${config.CommonGlobalIncludes}",
				"${config.CommonGlobalSystemIncludes}",
				tc.IncludeFlags(),
				"${config.CommonNativehelperInclude}")
		}

		flags.GlobalFlags = append(flags.GlobalFlags, "-I"+android.PathForModuleSrc(ctx).String())
	}

	if ctx.sdk() {
		// The NDK headers are installed to a common sysroot. While a more
		// typical Soong approach would be to only make the headers for the
		// library you're using available, we're trying to emulate the NDK
		// behavior here, and the NDK always has all the NDK headers available.
		flags.GlobalFlags = append(flags.GlobalFlags,
			"-isystem "+getCurrentIncludePath(ctx).String(),
			"-isystem "+getCurrentIncludePath(ctx).Join(ctx, tc.ClangTriple()).String())

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
	instructionSetFlags, err := tc.InstructionSetFlags(instructionSet)
	if flags.Clang {
		instructionSetFlags, err = tc.ClangInstructionSetFlags(instructionSet)
	}
	if err != nil {
		ctx.ModuleErrorf("%s", err)
	}

	CheckBadCompilerFlags(ctx, "release.cflags", compiler.Properties.Release.Cflags)

	// TODO: debug
	flags.CFlags = append(flags.CFlags, esc(compiler.Properties.Release.Cflags)...)

	if flags.Clang {
		CheckBadCompilerFlags(ctx, "clang_cflags", compiler.Properties.Clang_cflags)
		CheckBadCompilerFlags(ctx, "clang_asflags", compiler.Properties.Clang_asflags)

		flags.CFlags = config.ClangFilterUnknownCflags(flags.CFlags)
		flags.CFlags = append(flags.CFlags, esc(compiler.Properties.Clang_cflags)...)
		flags.AsFlags = append(flags.AsFlags, esc(compiler.Properties.Clang_asflags)...)
		flags.CppFlags = config.ClangFilterUnknownCflags(flags.CppFlags)
		flags.ConlyFlags = config.ClangFilterUnknownCflags(flags.ConlyFlags)
		flags.LdFlags = config.ClangFilterUnknownCflags(flags.LdFlags)

		target := "-target " + tc.ClangTriple()
		var gccPrefix string
		if !ctx.Darwin() {
			gccPrefix = "-B" + filepath.Join(tc.GccRoot(), tc.GccTriple(), "bin")
		}

		flags.CFlags = append(flags.CFlags, target, gccPrefix)
		flags.AsFlags = append(flags.AsFlags, target, gccPrefix)
		flags.LdFlags = append(flags.LdFlags, target, gccPrefix)
	}

	hod := "Host"
	if ctx.Os().Class == android.Device {
		hod = "Device"
	}

	if !ctx.noDefaultCompilerFlags() {
		flags.GlobalFlags = append(flags.GlobalFlags, instructionSetFlags)
		flags.ConlyFlags = append(flags.ConlyFlags, "${config.CommonGlobalConlyflags}")

		if flags.Clang {
			flags.AsFlags = append(flags.AsFlags, tc.ClangAsflags())
			flags.CppFlags = append(flags.CppFlags, "${config.CommonClangGlobalCppflags}")
			flags.GlobalFlags = append(flags.GlobalFlags,
				tc.ClangCflags(),
				"${config.CommonClangGlobalCflags}",
				fmt.Sprintf("${config.%sClangGlobalCflags}", hod))
		} else {
			flags.CppFlags = append(flags.CppFlags, "${config.CommonGlobalCppflags}")
			flags.GlobalFlags = append(flags.GlobalFlags,
				tc.Cflags(),
				"${config.CommonGlobalCflags}",
				fmt.Sprintf("${config.%sGlobalCflags}", hod))
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
			flags.CppFlags = append(flags.CppFlags, tc.ClangCppflags())
		} else {
			flags.CppFlags = append(flags.CppFlags, tc.Cppflags())
		}
	}

	if flags.Clang {
		flags.GlobalFlags = append(flags.GlobalFlags, tc.ToolchainClangCflags())
	} else {
		flags.GlobalFlags = append(flags.GlobalFlags, tc.ToolchainCflags())
	}

	if !ctx.sdk() {
		cStd := config.CStdVersion
		cppStd := config.CppStdVersion

		if !flags.Clang {
			// GCC uses an invalid C++14 ABI (emits calls to
			// __cxa_throw_bad_array_length, which is not a valid C++ RT ABI).
			// http://b/25022512
			cppStd = config.GccCppStdVersion
		} else if ctx.Host() && !flags.Clang {
			// The host GCC doesn't support C++14 (and is deprecated, so likely
			// never will). Build these modules with C++11.
			cppStd = config.GccCppStdVersion
		}

		if compiler.Properties.Gnu_extensions != nil && *compiler.Properties.Gnu_extensions == false {
			cStd = gnuToCReplacer.Replace(cStd)
			cppStd = gnuToCReplacer.Replace(cppStd)
		}

		flags.ConlyFlags = append([]string{"-std=" + cStd}, flags.ConlyFlags...)
		flags.CppFlags = append([]string{"-std=" + cppStd}, flags.CppFlags...)
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

	if compiler.hasProto() {
		flags = protoFlags(ctx, flags, &compiler.Proto)
	}

	return flags
}

func (compiler *baseCompiler) hasProto() bool {
	for _, src := range compiler.Properties.Srcs {
		if filepath.Ext(src) == ".proto" {
			return true
		}
	}

	return false
}

var gnuToCReplacer = strings.NewReplacer("gnu", "c")

func ndkPathDeps(ctx ModuleContext) android.Paths {
	if ctx.sdk() {
		// The NDK sysroot timestamp file depends on all the NDK sysroot files
		// (headers and libraries).
		return android.Paths{getNdkSysrootTimestampFile(ctx)}
	}
	return nil
}

func (compiler *baseCompiler) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	pathDeps := deps.GeneratedHeaders
	pathDeps = append(pathDeps, ndkPathDeps(ctx)...)

	srcs := ctx.ExpandSources(compiler.Properties.Srcs, compiler.Properties.Exclude_srcs)
	srcs = append(srcs, deps.GeneratedSources...)

	buildFlags := flagsToBuilderFlags(flags)

	srcs, genDeps := genSources(ctx, srcs, buildFlags)

	pathDeps = append(pathDeps, genDeps...)
	pathDeps = append(pathDeps, flags.CFlagsDeps...)

	compiler.deps = pathDeps

	// Compile files listed in c.Properties.Srcs into objects
	objs := compileObjs(ctx, buildFlags, "", srcs, compiler.deps)

	if ctx.Failed() {
		return Objects{}
	}

	return objs
}

// Compile a list of source files into objects a specified subdirectory
func compileObjs(ctx android.ModuleContext, flags builderFlags,
	subdir string, srcFiles, deps android.Paths) Objects {

	return TransformSourceToObj(ctx, subdir, srcFiles, flags, deps)
}
