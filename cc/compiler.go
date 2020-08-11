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
	"regexp"
	"strconv"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
)

var (
	allowedManualInterfacePaths = []string{"vendor/", "hardware/"}
)

// This file contains the basic C/C++/assembly to .o compliation steps

type BaseCompilerProperties struct {
	// list of source files used to compile the C/C++ module.  May be .c, .cpp, or .S files.
	// srcs may reference the outputs of other modules that produce source files like genrule
	// or filegroup using the syntax ":module".
	Srcs []string `android:"path,arch_variant"`

	// list of source files that should not be used to build the C/C++ module.
	// This is most useful in the arch/multilib variants to remove non-common files
	Exclude_srcs []string `android:"path,arch_variant"`

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

	// the instruction set architecture to use to compile the C/C++
	// module.
	Instruction_set *string `android:"arch_variant"`

	// list of directories relative to the root of the source tree that will
	// be added to the include path using -I.
	// If possible, don't use this.  If adding paths from the current directory use
	// local_include_dirs, if adding paths from other modules use export_include_dirs in
	// that module.
	Include_dirs []string `android:"arch_variant,variant_prepend"`

	// list of directories relative to the Blueprints file that will
	// be added to the include path using -I
	Local_include_dirs []string `android:"arch_variant,variant_prepend"`

	// Add the directory containing the Android.bp file to the list of include
	// directories. Defaults to true.
	Include_build_directory *bool

	// list of generated sources to compile. These are the names of gensrcs or
	// genrule modules.
	Generated_sources []string `android:"arch_variant"`

	// list of generated headers to add to the include path. These are the names
	// of genrule modules.
	Generated_headers []string `android:"arch_variant"`

	// pass -frtti instead of -fno-rtti
	Rtti *bool

	// C standard version to use. Can be a specific version (such as "gnu11"),
	// "experimental" (which will use draft versions like C1x when available),
	// or the empty string (which will use the default).
	C_std *string

	// C++ standard version to use. Can be a specific version (such as
	// "gnu++11"), "experimental" (which will use draft versions like C++1z when
	// available), or the empty string (which will use the default).
	Cpp_std *string

	// if set to false, use -std=c++* instead of -std=gnu++*
	Gnu_extensions *bool

	Yacc *YaccProperties

	Aidl struct {
		// list of directories that will be added to the aidl include paths.
		Include_dirs []string

		// list of directories relative to the Blueprints file that will
		// be added to the aidl include paths.
		Local_include_dirs []string

		// whether to generate traces (for systrace) for this interface
		Generate_traces *bool
	}

	Renderscript struct {
		// list of directories that will be added to the llvm-rs-cc include paths
		Include_dirs []string

		// list of flags that will be passed to llvm-rs-cc
		Flags []string

		// Renderscript API level to target
		Target_api *string
	}

	Debug, Release struct {
		// list of module-specific flags that will be used for C and C++ compiles in debug or
		// release builds
		Cflags []string `android:"arch_variant"`
	} `android:"arch_variant"`

	Target struct {
		Vendor struct {
			// list of source files that should only be used in the
			// vendor variant of the C/C++ module.
			Srcs []string `android:"path"`

			// list of source files that should not be used to
			// build the vendor variant of the C/C++ module.
			Exclude_srcs []string `android:"path"`

			// List of additional cflags that should be used to build the vendor
			// variant of the C/C++ module.
			Cflags []string
		}
		Recovery struct {
			// list of source files that should only be used in the
			// recovery variant of the C/C++ module.
			Srcs []string `android:"path"`

			// list of source files that should not be used to
			// build the recovery variant of the C/C++ module.
			Exclude_srcs []string `android:"path"`

			// List of additional cflags that should be used to build the recovery
			// variant of the C/C++ module.
			Cflags []string
		}
	}

	Proto struct {
		// Link statically against the protobuf runtime
		Static *bool `android:"arch_variant"`
	} `android:"arch_variant"`

	// Stores the original list of source files before being cleared by library reuse
	OriginalSrcs []string `blueprint:"mutated"`

	// Build and link with OpenMP
	Openmp *bool `android:"arch_variant"`

	// Adds __ANDROID_APEX_<APEX_MODULE_NAME>__ macro defined for apex variants in addition to __ANDROID_APEX__
	Use_apex_name_macro *bool
}

func NewBaseCompiler() *baseCompiler {
	return &baseCompiler{}
}

type baseCompiler struct {
	Properties BaseCompilerProperties
	Proto      android.ProtoProperties
	cFlagsDeps android.Paths
	pathDeps   android.Paths
	flags      builderFlags

	// Sources that were passed to the C/C++ compiler
	srcs android.Paths

	// Sources that were passed in the Android.bp file, including generated sources generated by
	// other modules and filegroups. May include source files that have not yet been translated to
	// C/C++ (.aidl, .proto, etc.)
	srcsBeforeGen android.Paths
}

var _ compiler = (*baseCompiler)(nil)

type CompiledInterface interface {
	Srcs() android.Paths
}

func (compiler *baseCompiler) Srcs() android.Paths {
	return append(android.Paths{}, compiler.srcs...)
}

func (compiler *baseCompiler) appendCflags(flags []string) {
	compiler.Properties.Cflags = append(compiler.Properties.Cflags, flags...)
}

func (compiler *baseCompiler) appendAsflags(flags []string) {
	compiler.Properties.Asflags = append(compiler.Properties.Asflags, flags...)
}

func (compiler *baseCompiler) compilerProps() []interface{} {
	return []interface{}{&compiler.Properties, &compiler.Proto}
}

func (compiler *baseCompiler) compilerInit(ctx BaseModuleContext) {}

func (compiler *baseCompiler) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps.GeneratedSources = append(deps.GeneratedSources, compiler.Properties.Generated_sources...)
	deps.GeneratedHeaders = append(deps.GeneratedHeaders, compiler.Properties.Generated_headers...)

	android.ProtoDeps(ctx, &compiler.Proto)
	if compiler.hasSrcExt(".proto") {
		deps = protoDeps(ctx, deps, &compiler.Proto, Bool(compiler.Properties.Proto.Static))
	}

	if Bool(compiler.Properties.Openmp) {
		deps.StaticLibs = append(deps.StaticLibs, "libomp")
	}

	return deps
}

// Return true if the module is in the WarningAllowedProjects.
func warningsAreAllowed(subdir string) bool {
	subdir += "/"
	return android.HasAnyPrefix(subdir, config.WarningAllowedProjects)
}

func addToModuleList(ctx ModuleContext, key android.OnceKey, module string) {
	getNamedMapForConfig(ctx.Config(), key).Store(module, true)
}

// Create a Flags struct that collects the compile flags from global values,
// per-target values, module type values, and per-module Blueprints properties
func (compiler *baseCompiler) compilerFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags {
	tc := ctx.toolchain()
	modulePath := android.PathForModuleSrc(ctx).String()

	compiler.srcsBeforeGen = android.PathsForModuleSrcExcludes(ctx, compiler.Properties.Srcs, compiler.Properties.Exclude_srcs)
	compiler.srcsBeforeGen = append(compiler.srcsBeforeGen, deps.GeneratedSources...)

	CheckBadCompilerFlags(ctx, "cflags", compiler.Properties.Cflags)
	CheckBadCompilerFlags(ctx, "cppflags", compiler.Properties.Cppflags)
	CheckBadCompilerFlags(ctx, "conlyflags", compiler.Properties.Conlyflags)
	CheckBadCompilerFlags(ctx, "asflags", compiler.Properties.Asflags)
	CheckBadCompilerFlags(ctx, "vendor.cflags", compiler.Properties.Target.Vendor.Cflags)
	CheckBadCompilerFlags(ctx, "recovery.cflags", compiler.Properties.Target.Recovery.Cflags)

	esc := proptools.NinjaAndShellEscapeList

	flags.Local.CFlags = append(flags.Local.CFlags, esc(compiler.Properties.Cflags)...)
	flags.Local.CppFlags = append(flags.Local.CppFlags, esc(compiler.Properties.Cppflags)...)
	flags.Local.ConlyFlags = append(flags.Local.ConlyFlags, esc(compiler.Properties.Conlyflags)...)
	flags.Local.AsFlags = append(flags.Local.AsFlags, esc(compiler.Properties.Asflags)...)
	flags.Local.YasmFlags = append(flags.Local.YasmFlags, esc(compiler.Properties.Asflags)...)

	flags.Yacc = compiler.Properties.Yacc

	// Include dir cflags
	localIncludeDirs := android.PathsForModuleSrc(ctx, compiler.Properties.Local_include_dirs)
	if len(localIncludeDirs) > 0 {
		f := includeDirsToFlags(localIncludeDirs)
		flags.Local.CommonFlags = append(flags.Local.CommonFlags, f)
		flags.Local.YasmFlags = append(flags.Local.YasmFlags, f)
	}
	rootIncludeDirs := android.PathsForSource(ctx, compiler.Properties.Include_dirs)
	if len(rootIncludeDirs) > 0 {
		f := includeDirsToFlags(rootIncludeDirs)
		flags.Local.CommonFlags = append(flags.Local.CommonFlags, f)
		flags.Local.YasmFlags = append(flags.Local.YasmFlags, f)
	}

	if compiler.Properties.Include_build_directory == nil ||
		*compiler.Properties.Include_build_directory {
		flags.Local.CommonFlags = append(flags.Local.CommonFlags, "-I"+modulePath)
		flags.Local.YasmFlags = append(flags.Local.YasmFlags, "-I"+modulePath)
	}

	if !(ctx.useSdk() || ctx.useVndk()) || ctx.Host() {
		flags.SystemIncludeFlags = append(flags.SystemIncludeFlags,
			"${config.CommonGlobalIncludes}",
			tc.IncludeFlags(),
			"${config.CommonNativehelperInclude}")
	}

	if ctx.useSdk() {
		// TODO: Switch to --sysroot.
		// The NDK headers are installed to a common sysroot. While a more
		// typical Soong approach would be to only make the headers for the
		// library you're using available, we're trying to emulate the NDK
		// behavior here, and the NDK always has all the NDK headers available.
		flags.SystemIncludeFlags = append(flags.SystemIncludeFlags,
			"-isystem "+getCurrentIncludePath(ctx).String(),
			"-isystem "+getCurrentIncludePath(ctx).Join(ctx, config.NDKTriple(tc)).String())
	}

	if ctx.useVndk() {
		flags.Global.CommonFlags = append(flags.Global.CommonFlags, "-D__ANDROID_VNDK__")
	}

	if ctx.inRecovery() {
		flags.Global.CommonFlags = append(flags.Global.CommonFlags, "-D__ANDROID_RECOVERY__")
	}

	if ctx.apexVariationName() != "" {
		flags.Global.CommonFlags = append(flags.Global.CommonFlags, "-D__ANDROID_APEX__")
		if Bool(compiler.Properties.Use_apex_name_macro) {
			flags.Global.CommonFlags = append(flags.Global.CommonFlags, "-D__ANDROID_APEX_"+makeDefineString(ctx.apexVariationName())+"__")
		}
		if ctx.Device() {
			flags.Global.CommonFlags = append(flags.Global.CommonFlags, "-D__ANDROID_SDK_VERSION__="+strconv.Itoa(ctx.apexSdkVersion()))
		}
	}

	if ctx.Target().NativeBridge == android.NativeBridgeEnabled {
		flags.Global.CommonFlags = append(flags.Global.CommonFlags, "-D__ANDROID_NATIVE_BRIDGE__")
	}

	instructionSet := String(compiler.Properties.Instruction_set)
	if flags.RequiredInstructionSet != "" {
		instructionSet = flags.RequiredInstructionSet
	}
	instructionSetFlags, err := tc.ClangInstructionSetFlags(instructionSet)
	if err != nil {
		ctx.ModuleErrorf("%s", err)
	}

	CheckBadCompilerFlags(ctx, "release.cflags", compiler.Properties.Release.Cflags)

	// TODO: debug
	flags.Local.CFlags = append(flags.Local.CFlags, esc(compiler.Properties.Release.Cflags)...)

	CheckBadCompilerFlags(ctx, "clang_cflags", compiler.Properties.Clang_cflags)
	CheckBadCompilerFlags(ctx, "clang_asflags", compiler.Properties.Clang_asflags)

	flags.Local.CFlags = config.ClangFilterUnknownCflags(flags.Local.CFlags)
	flags.Local.CFlags = append(flags.Local.CFlags, esc(compiler.Properties.Clang_cflags)...)
	flags.Local.AsFlags = append(flags.Local.AsFlags, esc(compiler.Properties.Clang_asflags)...)
	flags.Local.CppFlags = config.ClangFilterUnknownCflags(flags.Local.CppFlags)
	flags.Local.ConlyFlags = config.ClangFilterUnknownCflags(flags.Local.ConlyFlags)
	flags.Local.LdFlags = config.ClangFilterUnknownCflags(flags.Local.LdFlags)

	target := "-target " + tc.ClangTriple()
	if ctx.Os().Class == android.Device {
		version := ctx.sdkVersion()
		if version == "" || version == "current" {
			target += strconv.Itoa(android.FutureApiLevel)
		} else {
			target += version
		}
	}

	gccPrefix := "-B" + config.ToolPath(tc)

	flags.Global.CFlags = append(flags.Global.CFlags, target, gccPrefix)
	flags.Global.AsFlags = append(flags.Global.AsFlags, target, gccPrefix)
	flags.Global.LdFlags = append(flags.Global.LdFlags, target, gccPrefix)

	hod := "Host"
	if ctx.Os().Class == android.Device {
		hod = "Device"
	}

	flags.Global.CommonFlags = append(flags.Global.CommonFlags, instructionSetFlags)
	flags.Global.ConlyFlags = append([]string{"${config.CommonGlobalConlyflags}"}, flags.Global.ConlyFlags...)
	flags.Global.CppFlags = append([]string{fmt.Sprintf("${config.%sGlobalCppflags}", hod)}, flags.Global.CppFlags...)

	flags.Global.AsFlags = append(flags.Global.AsFlags, tc.ClangAsflags())
	flags.Global.CppFlags = append([]string{"${config.CommonClangGlobalCppflags}"}, flags.Global.CppFlags...)
	flags.Global.CommonFlags = append(flags.Global.CommonFlags,
		tc.ClangCflags(),
		"${config.CommonClangGlobalCflags}",
		fmt.Sprintf("${config.%sClangGlobalCflags}", hod))

	if isThirdParty(modulePath) {
		flags.Global.CommonFlags = append([]string{"${config.ClangExternalCflags}"}, flags.Global.CommonFlags...)
	}

	if tc.Bionic() {
		if Bool(compiler.Properties.Rtti) {
			flags.Local.CppFlags = append(flags.Local.CppFlags, "-frtti")
		} else {
			flags.Local.CppFlags = append(flags.Local.CppFlags, "-fno-rtti")
		}
	}

	flags.Global.AsFlags = append(flags.Global.AsFlags, "-D__ASSEMBLY__")

	flags.Global.CppFlags = append(flags.Global.CppFlags, tc.ClangCppflags())

	flags.Global.YasmFlags = append(flags.Global.YasmFlags, tc.YasmFlags())

	flags.Global.CommonFlags = append(flags.Global.CommonFlags, tc.ToolchainClangCflags())

	cStd := config.CStdVersion
	if String(compiler.Properties.C_std) == "experimental" {
		cStd = config.ExperimentalCStdVersion
	} else if String(compiler.Properties.C_std) != "" {
		cStd = String(compiler.Properties.C_std)
	}

	cppStd := String(compiler.Properties.Cpp_std)
	switch String(compiler.Properties.Cpp_std) {
	case "":
		cppStd = config.CppStdVersion
	case "experimental":
		cppStd = config.ExperimentalCppStdVersion
	}

	if compiler.Properties.Gnu_extensions != nil && *compiler.Properties.Gnu_extensions == false {
		cStd = gnuToCReplacer.Replace(cStd)
		cppStd = gnuToCReplacer.Replace(cppStd)
	}

	flags.Local.ConlyFlags = append([]string{"-std=" + cStd}, flags.Local.ConlyFlags...)
	flags.Local.CppFlags = append([]string{"-std=" + cppStd}, flags.Local.CppFlags...)

	if ctx.useVndk() {
		flags.Local.CFlags = append(flags.Local.CFlags, esc(compiler.Properties.Target.Vendor.Cflags)...)
	}

	if ctx.inRecovery() {
		flags.Local.CFlags = append(flags.Local.CFlags, esc(compiler.Properties.Target.Recovery.Cflags)...)
	}

	// We can enforce some rules more strictly in the code we own. strict
	// indicates if this is code that we can be stricter with. If we have
	// rules that we want to apply to *our* code (but maybe can't for
	// vendor/device specific things), we could extend this to be a ternary
	// value.
	strict := true
	if strings.HasPrefix(modulePath, "external/") {
		strict = false
	}

	// Can be used to make some annotations stricter for code we can fix
	// (such as when we mark functions as deprecated).
	if strict {
		flags.Global.CFlags = append(flags.Global.CFlags, "-DANDROID_STRICT")
	}

	if compiler.hasSrcExt(".proto") {
		flags = protoFlags(ctx, flags, &compiler.Proto)
	}

	if compiler.hasSrcExt(".y") || compiler.hasSrcExt(".yy") {
		flags.Local.CommonFlags = append(flags.Local.CommonFlags,
			"-I"+android.PathForModuleGen(ctx, "yacc", ctx.ModuleDir()).String())
	}

	if compiler.hasSrcExt(".mc") {
		flags.Local.CommonFlags = append(flags.Local.CommonFlags,
			"-I"+android.PathForModuleGen(ctx, "windmc", ctx.ModuleDir()).String())
	}

	if compiler.hasSrcExt(".aidl") {
		if len(compiler.Properties.Aidl.Local_include_dirs) > 0 {
			localAidlIncludeDirs := android.PathsForModuleSrc(ctx, compiler.Properties.Aidl.Local_include_dirs)
			flags.aidlFlags = append(flags.aidlFlags, includeDirsToFlags(localAidlIncludeDirs))
		}
		if len(compiler.Properties.Aidl.Include_dirs) > 0 {
			rootAidlIncludeDirs := android.PathsForSource(ctx, compiler.Properties.Aidl.Include_dirs)
			flags.aidlFlags = append(flags.aidlFlags, includeDirsToFlags(rootAidlIncludeDirs))
		}

		if Bool(compiler.Properties.Aidl.Generate_traces) {
			flags.aidlFlags = append(flags.aidlFlags, "-t")
		}

		flags.Local.CommonFlags = append(flags.Local.CommonFlags,
			"-I"+android.PathForModuleGen(ctx, "aidl").String())
	}

	if compiler.hasSrcExt(".rscript") || compiler.hasSrcExt(".fs") {
		flags = rsFlags(ctx, flags, &compiler.Properties)
	}

	if compiler.hasSrcExt(".sysprop") {
		flags.Local.CommonFlags = append(flags.Local.CommonFlags,
			"-I"+android.PathForModuleGen(ctx, "sysprop", "include").String())
	}

	if len(compiler.Properties.Srcs) > 0 {
		module := ctx.ModuleDir() + "/Android.bp:" + ctx.ModuleName()
		if inList("-Wno-error", flags.Local.CFlags) || inList("-Wno-error", flags.Local.CppFlags) {
			addToModuleList(ctx, modulesUsingWnoErrorKey, module)
		} else if !inList("-Werror", flags.Local.CFlags) && !inList("-Werror", flags.Local.CppFlags) {
			if warningsAreAllowed(ctx.ModuleDir()) {
				addToModuleList(ctx, modulesAddedWallKey, module)
				flags.Local.CFlags = append([]string{"-Wall"}, flags.Local.CFlags...)
			} else {
				flags.Local.CFlags = append([]string{"-Wall", "-Werror"}, flags.Local.CFlags...)
			}
		}
	}

	if Bool(compiler.Properties.Openmp) {
		flags.Local.CFlags = append(flags.Local.CFlags, "-fopenmp")
	}

	// Exclude directories from manual binder interface allowed list.
	//TODO(b/145621474): Move this check into IInterface.h when clang-tidy no longer uses absolute paths.
	if android.HasAnyPrefix(ctx.ModuleDir(), allowedManualInterfacePaths) {
		flags.Local.CFlags = append(flags.Local.CFlags, "-DDO_NOT_CHECK_MANUAL_BINDER_INTERFACES")
	}

	return flags
}

func (compiler *baseCompiler) hasSrcExt(ext string) bool {
	for _, src := range compiler.srcsBeforeGen {
		if src.Ext() == ext {
			return true
		}
	}
	for _, src := range compiler.Properties.Srcs {
		if filepath.Ext(src) == ext {
			return true
		}
	}
	for _, src := range compiler.Properties.OriginalSrcs {
		if filepath.Ext(src) == ext {
			return true
		}
	}

	return false
}

func (compiler *baseCompiler) uniqueApexVariations() bool {
	return Bool(compiler.Properties.Use_apex_name_macro)
}

// makeDefineString transforms a name of an APEX module into a value to be used as value for C define
// For example, com.android.foo => COM_ANDROID_FOO
func makeDefineString(name string) string {
	return strings.ReplaceAll(strings.ToUpper(name), ".", "_")
}

var gnuToCReplacer = strings.NewReplacer("gnu", "c")

func ndkPathDeps(ctx ModuleContext) android.Paths {
	if ctx.useSdk() {
		// The NDK sysroot timestamp file depends on all the NDK sysroot files
		// (headers and libraries).
		return android.Paths{getNdkBaseTimestampFile(ctx)}
	}
	return nil
}

func (compiler *baseCompiler) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	pathDeps := deps.GeneratedDeps
	pathDeps = append(pathDeps, ndkPathDeps(ctx)...)

	buildFlags := flagsToBuilderFlags(flags)

	srcs := append(android.Paths(nil), compiler.srcsBeforeGen...)

	srcs, genDeps := genSources(ctx, srcs, buildFlags)
	pathDeps = append(pathDeps, genDeps...)

	compiler.pathDeps = pathDeps
	compiler.cFlagsDeps = flags.CFlagsDeps

	// Save src, buildFlags and context
	compiler.srcs = srcs

	// Compile files listed in c.Properties.Srcs into objects
	objs := compileObjs(ctx, buildFlags, "", srcs, pathDeps, compiler.cFlagsDeps)

	if ctx.Failed() {
		return Objects{}
	}

	return objs
}

// Compile a list of source files into objects a specified subdirectory
func compileObjs(ctx android.ModuleContext, flags builderFlags,
	subdir string, srcFiles, pathDeps android.Paths, cFlagsDeps android.Paths) Objects {

	return TransformSourceToObj(ctx, subdir, srcFiles, flags, pathDeps, cFlagsDeps)
}

var thirdPartyDirPrefixExceptions = []*regexp.Regexp{
	regexp.MustCompile("^vendor/[^/]*google[^/]*/"),
	regexp.MustCompile("^hardware/google/"),
	regexp.MustCompile("^hardware/interfaces/"),
	regexp.MustCompile("^hardware/libhardware[^/]*/"),
	regexp.MustCompile("^hardware/ril/"),
}

func isThirdParty(path string) bool {
	thirdPartyDirPrefixes := []string{"external/", "vendor/", "hardware/"}

	if android.HasAnyPrefix(path, thirdPartyDirPrefixes) {
		for _, prefix := range thirdPartyDirPrefixExceptions {
			if prefix.MatchString(path) {
				return false
			}
		}
	}
	return true
}
