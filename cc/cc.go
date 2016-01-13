// Copyright 2015 Google Inc. All rights reserved.
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

// This file contains the module types for compiling C/C++ for Android, and converts the properties
// into the flags and filenames necessary to pass to the compiler.  The final creation of the rules
// is handled in builder.go

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong"
	"android/soong/common"
	"android/soong/genrule"
)

func init() {
	soong.RegisterModuleType("cc_library_static", CCLibraryStaticFactory)
	soong.RegisterModuleType("cc_library_shared", CCLibrarySharedFactory)
	soong.RegisterModuleType("cc_library", CCLibraryFactory)
	soong.RegisterModuleType("cc_object", CCObjectFactory)
	soong.RegisterModuleType("cc_binary", CCBinaryFactory)
	soong.RegisterModuleType("cc_test", CCTestFactory)
	soong.RegisterModuleType("cc_benchmark", CCBenchmarkFactory)
	soong.RegisterModuleType("cc_defaults", CCDefaultsFactory)

	soong.RegisterModuleType("toolchain_library", ToolchainLibraryFactory)
	soong.RegisterModuleType("ndk_prebuilt_library", NdkPrebuiltLibraryFactory)
	soong.RegisterModuleType("ndk_prebuilt_object", NdkPrebuiltObjectFactory)
	soong.RegisterModuleType("ndk_prebuilt_static_stl", NdkPrebuiltStaticStlFactory)
	soong.RegisterModuleType("ndk_prebuilt_shared_stl", NdkPrebuiltSharedStlFactory)

	soong.RegisterModuleType("cc_library_host_static", CCLibraryHostStaticFactory)
	soong.RegisterModuleType("cc_library_host_shared", CCLibraryHostSharedFactory)
	soong.RegisterModuleType("cc_binary_host", CCBinaryHostFactory)
	soong.RegisterModuleType("cc_test_host", CCTestHostFactory)
	soong.RegisterModuleType("cc_benchmark_host", CCBenchmarkHostFactory)

	// LinkageMutator must be registered after common.ArchMutator, but that is guaranteed by
	// the Go initialization order because this package depends on common, so common's init
	// functions will run first.
	common.RegisterBottomUpMutator("link", linkageMutator)
	common.RegisterBottomUpMutator("test_per_src", testPerSrcMutator)
	common.RegisterBottomUpMutator("deps", depsMutator)
}

var (
	HostPrebuiltTag = pctx.VariableConfigMethod("HostPrebuiltTag", common.Config.PrebuiltOS)

	LibcRoot = pctx.SourcePathVariable("LibcRoot", "bionic/libc")
	LibmRoot = pctx.SourcePathVariable("LibmRoot", "bionic/libm")
)

// Flags used by lots of devices.  Putting them in package static variables will save bytes in
// build.ninja so they aren't repeated for every file
var (
	commonGlobalCflags = []string{
		"-DANDROID",
		"-fmessage-length=0",
		"-W",
		"-Wall",
		"-Wno-unused",
		"-Winit-self",
		"-Wpointer-arith",
		"-fdebug-prefix-map=/proc/self/cwd=",

		// COMMON_RELEASE_CFLAGS
		"-DNDEBUG",
		"-UDEBUG",
	}

	deviceGlobalCflags = []string{
		"-fdiagnostics-color",

		// TARGET_ERROR_FLAGS
		"-Werror=return-type",
		"-Werror=non-virtual-dtor",
		"-Werror=address",
		"-Werror=sequence-point",
	}

	hostGlobalCflags = []string{}

	commonGlobalCppflags = []string{
		"-Wsign-promo",
	}

	illegalFlags = []string{
		"-w",
	}
)

func init() {
	pctx.StaticVariable("commonGlobalCflags", strings.Join(commonGlobalCflags, " "))
	pctx.StaticVariable("deviceGlobalCflags", strings.Join(deviceGlobalCflags, " "))
	pctx.StaticVariable("hostGlobalCflags", strings.Join(hostGlobalCflags, " "))

	pctx.StaticVariable("commonGlobalCppflags", strings.Join(commonGlobalCppflags, " "))

	pctx.StaticVariable("commonClangGlobalCflags",
		strings.Join(append(clangFilterUnknownCflags(commonGlobalCflags), "${clangExtraCflags}"), " "))
	pctx.StaticVariable("deviceClangGlobalCflags",
		strings.Join(append(clangFilterUnknownCflags(deviceGlobalCflags), "${clangExtraTargetCflags}"), " "))
	pctx.StaticVariable("hostClangGlobalCflags",
		strings.Join(clangFilterUnknownCflags(hostGlobalCflags), " "))
	pctx.StaticVariable("commonClangGlobalCppflags",
		strings.Join(append(clangFilterUnknownCflags(commonGlobalCppflags), "${clangExtraCppflags}"), " "))

	// Everything in this list is a crime against abstraction and dependency tracking.
	// Do not add anything to this list.
	pctx.PrefixedPathsForOptionalSourceVariable("commonGlobalIncludes", "-isystem ",
		[]string{
			"system/core/include",
			"hardware/libhardware/include",
			"hardware/libhardware_legacy/include",
			"hardware/ril/include",
			"libnativehelper/include",
			"frameworks/native/include",
			"frameworks/native/opengl/include",
			"frameworks/av/include",
			"frameworks/base/include",
		})
	// This is used by non-NDK modules to get jni.h. export_include_dirs doesn't help
	// with this, since there is no associated library.
	pctx.PrefixedPathsForOptionalSourceVariable("commonNativehelperInclude", "-I",
		[]string{"libnativehelper/include/nativehelper"})

	pctx.SourcePathVariable("clangPath", "prebuilts/clang/host/${HostPrebuiltTag}/3.8/bin")
}

type CCModuleContext common.AndroidBaseContext

// Building C/C++ code is handled by objects that satisfy this interface via composition
type CCModuleType interface {
	common.AndroidModule

	// Modify property values after parsing Blueprints file but before starting dependency
	// resolution or build rule generation
	ModifyProperties(CCModuleContext)

	// Modify the ccFlags
	flags(common.AndroidModuleContext, CCFlags) CCFlags

	// Return list of dependency names for use in depsMutator
	depNames(common.AndroidBaseContext, CCDeps) CCDeps

	// Add dynamic dependencies
	depsMutator(common.AndroidBottomUpMutatorContext)

	// Compile objects into final module
	compileModule(common.AndroidModuleContext, CCFlags, CCPathDeps, common.Paths)

	// Install the built module.
	installModule(common.AndroidModuleContext, CCFlags)

	// Return the output file (.o, .a or .so) for use by other modules
	outputFile() common.OptionalPath
}

type CCDeps struct {
	StaticLibs, SharedLibs, LateStaticLibs, WholeStaticLibs []string

	ObjFiles common.Paths

	Cflags, ReexportedCflags []string

	CrtBegin, CrtEnd string
}

type CCPathDeps struct {
	StaticLibs, SharedLibs, LateStaticLibs, WholeStaticLibs common.Paths

	ObjFiles               common.Paths
	WholeStaticLibObjFiles common.Paths

	Cflags, ReexportedCflags []string

	CrtBegin, CrtEnd common.OptionalPath
}

type CCFlags struct {
	GlobalFlags []string // Flags that apply to C, C++, and assembly source files
	AsFlags     []string // Flags that apply to assembly source files
	CFlags      []string // Flags that apply to C and C++ source files
	ConlyFlags  []string // Flags that apply to C source files
	CppFlags    []string // Flags that apply to C++ source files
	YaccFlags   []string // Flags that apply to Yacc source files
	LdFlags     []string // Flags that apply to linker command lines

	Nocrt     bool
	Toolchain Toolchain
	Clang     bool
}

// Properties used to compile all C or C++ modules
type CCBaseProperties struct {
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

	// list of module-specific flags that will be used for .y and .yy compiles
	Yaccflags []string

	// list of module-specific flags that will be used for all link steps
	Ldflags []string `android:"arch_variant"`

	// the instruction set architecture to use to compile the C/C++
	// module.
	Instruction_set string `android:"arch_variant"`

	// list of directories relative to the root of the source tree that will
	// be added to the include path using -I.
	// If possible, don't use this.  If adding paths from the current directory use
	// local_include_dirs, if adding paths from other modules use export_include_dirs in
	// that module.
	Include_dirs []string `android:"arch_variant"`

	// list of files relative to the root of the source tree that will be included
	// using -include.
	// If possible, don't use this.
	Include_files []string `android:"arch_variant"`

	// list of directories relative to the Blueprints file that will
	// be added to the include path using -I
	Local_include_dirs []string `android:"arch_variant"`

	// list of files relative to the Blueprints file that will be included
	// using -include.
	// If possible, don't use this.
	Local_include_files []string `android:"arch_variant"`

	// list of directories relative to the Blueprints file that will
	// be added to the include path using -I for any module that links against this module
	Export_include_dirs []string `android:"arch_variant"`

	// list of module-specific flags that will be used for C and C++ compiles when
	// compiling with clang
	Clang_cflags []string `android:"arch_variant"`

	// list of module-specific flags that will be used for .S compiles when
	// compiling with clang
	Clang_asflags []string `android:"arch_variant"`

	// list of system libraries that will be dynamically linked to
	// shared library and executable modules.  If unset, generally defaults to libc
	// and libm.  Set to [] to prevent linking against libc and libm.
	System_shared_libs []string

	// list of modules whose object files should be linked into this module
	// in their entirety.  For static library modules, all of the .o files from the intermediate
	// directory of the dependency will be linked into this modules .a file.  For a shared library,
	// the dependency's .a file will be linked into this module using -Wl,--whole-archive.
	Whole_static_libs []string `android:"arch_variant"`

	// list of modules that should be statically linked into this module.
	Static_libs []string `android:"arch_variant"`

	// list of modules that should be dynamically linked into this module.
	Shared_libs []string `android:"arch_variant"`

	// allow the module to contain undefined symbols.  By default,
	// modules cannot contain undefined symbols that are not satisified by their immediate
	// dependencies.  Set this flag to true to remove --no-undefined from the linker flags.
	// This flag should only be necessary for compiling low-level libraries like libc.
	Allow_undefined_symbols *bool

	// don't link in crt_begin and crt_end.  This flag should only be necessary for
	// compiling crt or libc.
	Nocrt *bool `android:"arch_variant"`

	// don't link in libgcc.a
	No_libgcc *bool

	// don't insert default compiler flags into asflags, cflags,
	// cppflags, conlyflags, ldflags, or include_dirs
	No_default_compiler_flags *bool

	// compile module with clang instead of gcc
	Clang *bool `android:"arch_variant"`

	// pass -frtti instead of -fno-rtti
	Rtti *bool

	// -l arguments to pass to linker for host-provided shared libraries
	Host_ldlibs []string `android:"arch_variant"`

	// select the STL library to use.  Possible values are "libc++", "libc++_static",
	// "stlport", "stlport_static", "ndk", "libstdc++", or "none".  Leave blank to select the
	// default
	Stl string

	// Set for combined shared/static libraries to prevent compiling object files a second time
	SkipCompileObjs bool `blueprint:"mutated"`

	Debug, Release struct {
		// list of module-specific flags that will be used for C and C++ compiles in debug or
		// release builds
		Cflags []string `android:"arch_variant"`
	} `android:"arch_variant"`

	// Minimum sdk version supported when compiling against the ndk
	Sdk_version string

	// install to a subdirectory of the default install path for the module
	Relative_install_path string
}

type CCUnusedProperties struct {
	Native_coverage  *bool
	Required         []string
	Sanitize         []string `android:"arch_variant"`
	Sanitize_recover []string
	Strip            string
	Tags             []string
}

// CCBase contains the properties and members used by all C/C++ module types, and implements
// the blueprint.Module interface.  It expects to be embedded into an outer specialization struct,
// and uses a ccModuleType interface to that struct to create the build steps.
type CCBase struct {
	common.AndroidModuleBase
	common.DefaultableModule
	module CCModuleType

	Properties CCBaseProperties

	unused CCUnusedProperties

	installPath string

	savedDepNames CCDeps
}

func newCCBase(base *CCBase, module CCModuleType, hod common.HostOrDeviceSupported,
	multilib common.Multilib, props ...interface{}) (blueprint.Module, []interface{}) {

	base.module = module

	props = append(props, &base.Properties, &base.unused)

	_, props = common.InitAndroidArchModule(module, hod, multilib, props...)

	return common.InitDefaultableModule(module, base, props...)
}

func (c *CCBase) GenerateAndroidBuildActions(ctx common.AndroidModuleContext) {
	toolchain := c.findToolchain(ctx)
	if ctx.Failed() {
		return
	}

	flags := c.collectFlags(ctx, toolchain)
	if ctx.Failed() {
		return
	}

	deps := c.depsToPaths(ctx, c.savedDepNames)
	if ctx.Failed() {
		return
	}

	flags.CFlags = append(flags.CFlags, deps.Cflags...)

	objFiles := c.compileObjs(ctx, flags)
	if ctx.Failed() {
		return
	}

	generatedObjFiles := c.compileGeneratedObjs(ctx, flags)
	if ctx.Failed() {
		return
	}

	objFiles = append(objFiles, generatedObjFiles...)

	c.ccModuleType().compileModule(ctx, flags, deps, objFiles)
	if ctx.Failed() {
		return
	}

	c.ccModuleType().installModule(ctx, flags)
	if ctx.Failed() {
		return
	}
}

func (c *CCBase) ccModuleType() CCModuleType {
	return c.module
}

func (c *CCBase) findToolchain(ctx common.AndroidModuleContext) Toolchain {
	arch := ctx.Arch()
	hod := ctx.HostOrDevice()
	ht := ctx.HostType()
	factory := toolchainFactories[hod][ht][arch.ArchType]
	if factory == nil {
		ctx.ModuleErrorf("Toolchain not found for %s %s arch %q", hod.String(), ht.String(), arch.String())
		return nil
	}
	return factory(arch)
}

func (c *CCBase) ModifyProperties(ctx CCModuleContext) {
}

func (c *CCBase) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	depNames.WholeStaticLibs = append(depNames.WholeStaticLibs, c.Properties.Whole_static_libs...)
	depNames.StaticLibs = append(depNames.StaticLibs, c.Properties.Static_libs...)
	depNames.SharedLibs = append(depNames.SharedLibs, c.Properties.Shared_libs...)

	return depNames
}

func (c *CCBase) depsMutator(ctx common.AndroidBottomUpMutatorContext) {
	c.savedDepNames = c.module.depNames(ctx, CCDeps{})
	c.savedDepNames.WholeStaticLibs = lastUniqueElements(c.savedDepNames.WholeStaticLibs)
	c.savedDepNames.StaticLibs = lastUniqueElements(c.savedDepNames.StaticLibs)
	c.savedDepNames.SharedLibs = lastUniqueElements(c.savedDepNames.SharedLibs)

	staticLibs := c.savedDepNames.WholeStaticLibs
	staticLibs = append(staticLibs, c.savedDepNames.StaticLibs...)
	staticLibs = append(staticLibs, c.savedDepNames.LateStaticLibs...)
	ctx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}}, staticLibs...)

	ctx.AddVariationDependencies([]blueprint.Variation{{"link", "shared"}}, c.savedDepNames.SharedLibs...)

	ctx.AddDependency(ctx.Module(), c.savedDepNames.ObjFiles.Strings()...)
	if c.savedDepNames.CrtBegin != "" {
		ctx.AddDependency(ctx.Module(), c.savedDepNames.CrtBegin)
	}
	if c.savedDepNames.CrtEnd != "" {
		ctx.AddDependency(ctx.Module(), c.savedDepNames.CrtEnd)
	}
}

func depsMutator(ctx common.AndroidBottomUpMutatorContext) {
	if c, ok := ctx.Module().(CCModuleType); ok {
		c.ModifyProperties(ctx)
		c.depsMutator(ctx)
	}
}

// Create a ccFlags struct that collects the compile flags from global values,
// per-target values, module type values, and per-module Blueprints properties
func (c *CCBase) collectFlags(ctx common.AndroidModuleContext, toolchain Toolchain) CCFlags {
	flags := CCFlags{
		CFlags:     c.Properties.Cflags,
		CppFlags:   c.Properties.Cppflags,
		ConlyFlags: c.Properties.Conlyflags,
		LdFlags:    c.Properties.Ldflags,
		AsFlags:    c.Properties.Asflags,
		YaccFlags:  c.Properties.Yaccflags,
		Nocrt:      Bool(c.Properties.Nocrt),
		Toolchain:  toolchain,
		Clang:      Bool(c.Properties.Clang),
	}

	// Include dir cflags
	rootIncludeDirs := common.PathsForSource(ctx, c.Properties.Include_dirs)
	localIncludeDirs := common.PathsForModuleSrc(ctx, c.Properties.Local_include_dirs)
	flags.GlobalFlags = append(flags.GlobalFlags,
		includeDirsToFlags(localIncludeDirs),
		includeDirsToFlags(rootIncludeDirs))

	rootIncludeFiles := common.PathsForSource(ctx, c.Properties.Include_files)
	localIncludeFiles := common.PathsForModuleSrc(ctx, c.Properties.Local_include_files)

	flags.GlobalFlags = append(flags.GlobalFlags,
		includeFilesToFlags(rootIncludeFiles),
		includeFilesToFlags(localIncludeFiles))

	if !Bool(c.Properties.No_default_compiler_flags) {
		if c.Properties.Sdk_version == "" || ctx.Host() {
			flags.GlobalFlags = append(flags.GlobalFlags,
				"${commonGlobalIncludes}",
				toolchain.IncludeFlags(),
				"${commonNativehelperInclude}")
		}

		flags.GlobalFlags = append(flags.GlobalFlags, []string{
			"-I" + common.PathForModuleSrc(ctx).String(),
			"-I" + common.PathForModuleOut(ctx).String(),
			"-I" + common.PathForModuleGen(ctx).String(),
		}...)
	}

	if c.Properties.Clang == nil {
		if ctx.Host() {
			flags.Clang = true
		}

		if ctx.Device() && ctx.AConfig().DeviceUsesClang() {
			flags.Clang = true
		}
	}

	if !toolchain.ClangSupported() {
		flags.Clang = false
	}

	instructionSet := c.Properties.Instruction_set
	instructionSetFlags, err := toolchain.InstructionSetFlags(instructionSet)
	if flags.Clang {
		instructionSetFlags, err = toolchain.ClangInstructionSetFlags(instructionSet)
	}
	if err != nil {
		ctx.ModuleErrorf("%s", err)
	}

	// TODO: debug
	flags.CFlags = append(flags.CFlags, c.Properties.Release.Cflags...)

	if flags.Clang {
		flags.CFlags = clangFilterUnknownCflags(flags.CFlags)
		flags.CFlags = append(flags.CFlags, c.Properties.Clang_cflags...)
		flags.AsFlags = append(flags.AsFlags, c.Properties.Clang_asflags...)
		flags.CppFlags = clangFilterUnknownCflags(flags.CppFlags)
		flags.ConlyFlags = clangFilterUnknownCflags(flags.ConlyFlags)
		flags.LdFlags = clangFilterUnknownCflags(flags.LdFlags)

		target := "-target " + toolchain.ClangTriple()
		gccPrefix := "-B" + filepath.Join(toolchain.GccRoot(), toolchain.GccTriple(), "bin")

		flags.CFlags = append(flags.CFlags, target, gccPrefix)
		flags.AsFlags = append(flags.AsFlags, target, gccPrefix)
		flags.LdFlags = append(flags.LdFlags, target, gccPrefix)
	}

	if !Bool(c.Properties.No_default_compiler_flags) {
		if ctx.Device() && !Bool(c.Properties.Allow_undefined_symbols) {
			flags.LdFlags = append(flags.LdFlags, "-Wl,--no-undefined")
		}

		flags.GlobalFlags = append(flags.GlobalFlags, instructionSetFlags)

		if flags.Clang {
			flags.AsFlags = append(flags.AsFlags, toolchain.ClangAsflags())
			flags.CppFlags = append(flags.CppFlags, "${commonClangGlobalCppflags}")
			flags.GlobalFlags = append(flags.GlobalFlags,
				toolchain.ClangCflags(),
				"${commonClangGlobalCflags}",
				fmt.Sprintf("${%sClangGlobalCflags}", ctx.HostOrDevice()))

			flags.ConlyFlags = append(flags.ConlyFlags, "${clangExtraConlyflags}")
		} else {
			flags.CppFlags = append(flags.CppFlags, "${commonGlobalCppflags}")
			flags.GlobalFlags = append(flags.GlobalFlags,
				toolchain.Cflags(),
				"${commonGlobalCflags}",
				fmt.Sprintf("${%sGlobalCflags}", ctx.HostOrDevice()))
		}

		if Bool(ctx.AConfig().ProductVariables.Brillo) {
			flags.GlobalFlags = append(flags.GlobalFlags, "-D__BRILLO__")
		}

		if ctx.Device() {
			if Bool(c.Properties.Rtti) {
				flags.CppFlags = append(flags.CppFlags, "-frtti")
			} else {
				flags.CppFlags = append(flags.CppFlags, "-fno-rtti")
			}
		}

		flags.AsFlags = append(flags.AsFlags, "-D__ASSEMBLY__")

		if flags.Clang {
			flags.CppFlags = append(flags.CppFlags, toolchain.ClangCppflags())
			flags.LdFlags = append(flags.LdFlags, toolchain.ClangLdflags())
		} else {
			flags.CppFlags = append(flags.CppFlags, toolchain.Cppflags())
			flags.LdFlags = append(flags.LdFlags, toolchain.Ldflags())
		}

		if ctx.Host() {
			flags.LdFlags = append(flags.LdFlags, c.Properties.Host_ldlibs...)
		}
	}

	if flags.Clang {
		flags.GlobalFlags = append(flags.GlobalFlags, toolchain.ToolchainClangCflags())
	} else {
		flags.GlobalFlags = append(flags.GlobalFlags, toolchain.ToolchainCflags())
	}
	flags.LdFlags = append(flags.LdFlags, toolchain.ToolchainLdflags())

	flags = c.ccModuleType().flags(ctx, flags)

	if c.Properties.Sdk_version == "" {
		if ctx.Host() && !flags.Clang {
			// The host GCC doesn't support C++14 (and is deprecated, so likely
			// never will). Build these modules with C++11.
			flags.CppFlags = append(flags.CppFlags, "-std=gnu++11")
		} else {
			flags.CppFlags = append(flags.CppFlags, "-std=gnu++14")
		}
	}

	flags.CFlags, _ = filterList(flags.CFlags, illegalFlags)
	flags.CppFlags, _ = filterList(flags.CppFlags, illegalFlags)
	flags.ConlyFlags, _ = filterList(flags.ConlyFlags, illegalFlags)

	// Optimization to reduce size of build.ninja
	// Replace the long list of flags for each file with a module-local variable
	ctx.Variable(pctx, "cflags", strings.Join(flags.CFlags, " "))
	ctx.Variable(pctx, "cppflags", strings.Join(flags.CppFlags, " "))
	ctx.Variable(pctx, "asflags", strings.Join(flags.AsFlags, " "))
	flags.CFlags = []string{"$cflags"}
	flags.CppFlags = []string{"$cppflags"}
	flags.AsFlags = []string{"$asflags"}

	return flags
}

func (c *CCBase) flags(ctx common.AndroidModuleContext, flags CCFlags) CCFlags {
	return flags
}

// Compile a list of source files into objects a specified subdirectory
func (c *CCBase) customCompileObjs(ctx common.AndroidModuleContext, flags CCFlags,
	subdir string, srcFiles, excludes []string) common.Paths {

	buildFlags := ccFlagsToBuilderFlags(flags)

	inputFiles := ctx.ExpandSources(srcFiles, excludes)
	srcPaths, deps := genSources(ctx, inputFiles, buildFlags)

	return TransformSourceToObj(ctx, subdir, srcPaths, buildFlags, deps)
}

// Compile files listed in c.Properties.Srcs into objects
func (c *CCBase) compileObjs(ctx common.AndroidModuleContext, flags CCFlags) common.Paths {

	if c.Properties.SkipCompileObjs {
		return nil
	}

	return c.customCompileObjs(ctx, flags, "", c.Properties.Srcs, c.Properties.Exclude_srcs)
}

// Compile generated source files from dependencies
func (c *CCBase) compileGeneratedObjs(ctx common.AndroidModuleContext, flags CCFlags) common.Paths {
	var srcs common.Paths

	if c.Properties.SkipCompileObjs {
		return nil
	}

	ctx.VisitDirectDeps(func(module blueprint.Module) {
		if gen, ok := module.(genrule.SourceFileGenerator); ok {
			srcs = append(srcs, gen.GeneratedSourceFiles()...)
		}
	})

	if len(srcs) == 0 {
		return nil
	}

	return TransformSourceToObj(ctx, "", srcs, ccFlagsToBuilderFlags(flags), nil)
}

func (c *CCBase) outputFile() common.OptionalPath {
	return common.OptionalPath{}
}

func (c *CCBase) depsToPathsFromList(ctx common.AndroidModuleContext,
	names []string) (modules []common.AndroidModule,
	outputFiles common.Paths, exportedFlags []string) {

	for _, n := range names {
		found := false
		ctx.VisitDirectDeps(func(m blueprint.Module) {
			otherName := ctx.OtherModuleName(m)
			if otherName != n {
				return
			}

			if a, ok := m.(CCModuleType); ok {
				if !a.Enabled() {
					// If a cc_library host+device module depends on a library that exists as both
					// cc_library_shared and cc_library_host_shared, it will end up with two
					// dependencies with the same name, one of which is marked disabled for each
					// of host and device.  Ignore the disabled one.
					return
				}
				if a.HostOrDevice() != ctx.HostOrDevice() {
					ctx.ModuleErrorf("host/device mismatch between %q and %q", ctx.ModuleName(),
						otherName)
					return
				}

				if outputFile := a.outputFile(); outputFile.Valid() {
					if found {
						ctx.ModuleErrorf("multiple modules satisified dependency on %q", otherName)
						return
					}
					outputFiles = append(outputFiles, outputFile.Path())
					modules = append(modules, a)
					if i, ok := a.(ccExportedFlagsProducer); ok {
						exportedFlags = append(exportedFlags, i.exportedFlags()...)
					}
					found = true
				} else {
					ctx.ModuleErrorf("module %q missing output file", otherName)
					return
				}
			} else {
				ctx.ModuleErrorf("module %q not an android module", otherName)
				return
			}
		})
		if !found && !inList(n, ctx.GetMissingDependencies()) {
			ctx.ModuleErrorf("unsatisified dependency on %q", n)
		}
	}

	return modules, outputFiles, exportedFlags
}

// Convert dependency names to paths.  Takes a CCDeps containing names and returns a CCPathDeps
// containing paths
func (c *CCBase) depsToPaths(ctx common.AndroidModuleContext, depNames CCDeps) CCPathDeps {
	var depPaths CCPathDeps
	var newCflags []string

	var wholeStaticLibModules []common.AndroidModule

	wholeStaticLibModules, depPaths.WholeStaticLibs, newCflags =
		c.depsToPathsFromList(ctx, depNames.WholeStaticLibs)
	depPaths.Cflags = append(depPaths.Cflags, newCflags...)
	depPaths.ReexportedCflags = append(depPaths.ReexportedCflags, newCflags...)

	for _, m := range wholeStaticLibModules {
		if staticLib, ok := m.(ccLibraryInterface); ok && staticLib.static() {
			depPaths.WholeStaticLibObjFiles =
				append(depPaths.WholeStaticLibObjFiles, staticLib.allObjFiles()...)
		} else {
			ctx.ModuleErrorf("module %q not a static library", ctx.OtherModuleName(m))
		}
	}

	_, depPaths.StaticLibs, newCflags = c.depsToPathsFromList(ctx, depNames.StaticLibs)
	depPaths.Cflags = append(depPaths.Cflags, newCflags...)

	_, depPaths.LateStaticLibs, newCflags = c.depsToPathsFromList(ctx, depNames.LateStaticLibs)
	depPaths.Cflags = append(depPaths.Cflags, newCflags...)

	_, depPaths.SharedLibs, newCflags = c.depsToPathsFromList(ctx, depNames.SharedLibs)
	depPaths.Cflags = append(depPaths.Cflags, newCflags...)

	ctx.VisitDirectDeps(func(m blueprint.Module) {
		if obj, ok := m.(ccObjectProvider); ok {
			otherName := ctx.OtherModuleName(m)
			if otherName == depNames.CrtBegin {
				if !Bool(c.Properties.Nocrt) {
					depPaths.CrtBegin = obj.object().outputFile()
				}
			} else if otherName == depNames.CrtEnd {
				if !Bool(c.Properties.Nocrt) {
					depPaths.CrtEnd = obj.object().outputFile()
				}
			} else {
				output := obj.object().outputFile()
				if output.Valid() {
					depPaths.ObjFiles = append(depPaths.ObjFiles, output.Path())
				} else {
					ctx.ModuleErrorf("module %s did not provide an output file", otherName)
				}
			}
		}
	})

	return depPaths
}

type ccLinkedProperties struct {
	VariantIsShared       bool `blueprint:"mutated"`
	VariantIsStatic       bool `blueprint:"mutated"`
	VariantIsStaticBinary bool `blueprint:"mutated"`
}

// CCLinked contains the properties and members used by libraries and executables
type CCLinked struct {
	CCBase
	dynamicProperties ccLinkedProperties
}

func newCCDynamic(dynamic *CCLinked, module CCModuleType, hod common.HostOrDeviceSupported,
	multilib common.Multilib, props ...interface{}) (blueprint.Module, []interface{}) {

	props = append(props, &dynamic.dynamicProperties)

	return newCCBase(&dynamic.CCBase, module, hod, multilib, props...)
}

func (c *CCLinked) systemSharedLibs(ctx common.AndroidBaseContext) []string {
	if c.Properties.System_shared_libs != nil {
		return c.Properties.System_shared_libs
	} else if ctx.Device() && c.Properties.Sdk_version == "" {
		return []string{"libc", "libm"}
	} else {
		return nil
	}
}

func (c *CCLinked) stl(ctx common.AndroidBaseContext) string {
	if c.Properties.Sdk_version != "" && ctx.Device() {
		switch c.Properties.Stl {
		case "":
			return "ndk_system"
		case "c++_shared", "c++_static",
			"stlport_shared", "stlport_static",
			"gnustl_static":
			return "ndk_lib" + c.Properties.Stl
		default:
			ctx.ModuleErrorf("stl: %q is not a supported STL with sdk_version set", c.Properties.Stl)
			return ""
		}
	}

	if ctx.HostType() == common.Windows {
		switch c.Properties.Stl {
		case "libc++", "libc++_static", "libstdc++", "":
			// libc++ is not supported on mingw
			return "libstdc++"
		case "none":
			return ""
		default:
			ctx.ModuleErrorf("stl: %q is not a supported STL", c.Properties.Stl)
			return ""
		}
	} else {
		switch c.Properties.Stl {
		case "libc++", "libc++_static",
			"libstdc++":
			return c.Properties.Stl
		case "none":
			return ""
		case "":
			if c.static() {
				return "libc++_static"
			} else {
				return "libc++"
			}
		default:
			ctx.ModuleErrorf("stl: %q is not a supported STL", c.Properties.Stl)
			return ""
		}
	}
}

var hostDynamicGccLibs, hostStaticGccLibs map[common.HostType][]string

func init() {
	hostDynamicGccLibs = map[common.HostType][]string{
		common.Linux:  []string{"-lgcc_s", "-lgcc", "-lc", "-lgcc_s", "-lgcc"},
		common.Darwin: []string{"-lc", "-lSystem"},
		common.Windows: []string{"-lmsvcr110", "-lmingw32", "-lgcc", "-lmoldname",
			"-lmingwex", "-lmsvcrt", "-ladvapi32", "-lshell32", "-luser32",
			"-lkernel32", "-lmingw32", "-lgcc", "-lmoldname", "-lmingwex",
			"-lmsvcrt"},
	}
	hostStaticGccLibs = map[common.HostType][]string{
		common.Linux:   []string{"-Wl,--start-group", "-lgcc", "-lgcc_eh", "-lc", "-Wl,--end-group"},
		common.Darwin:  []string{"NO_STATIC_HOST_BINARIES_ON_DARWIN"},
		common.Windows: []string{"NO_STATIC_HOST_BINARIES_ON_WINDOWS"},
	}
}

func (c *CCLinked) flags(ctx common.AndroidModuleContext, flags CCFlags) CCFlags {
	stl := c.stl(ctx)
	if ctx.Failed() {
		return flags
	}

	switch stl {
	case "libc++", "libc++_static":
		flags.CFlags = append(flags.CFlags, "-D_USING_LIBCXX")
		if ctx.Host() {
			flags.CppFlags = append(flags.CppFlags, "-nostdinc++")
			flags.LdFlags = append(flags.LdFlags, "-nodefaultlibs")
			flags.LdFlags = append(flags.LdFlags, "-lm", "-lpthread")
			if c.staticBinary() {
				flags.LdFlags = append(flags.LdFlags, hostStaticGccLibs[ctx.HostType()]...)
			} else {
				flags.LdFlags = append(flags.LdFlags, hostDynamicGccLibs[ctx.HostType()]...)
			}
		} else {
			if ctx.Arch().ArchType == common.Arm {
				flags.LdFlags = append(flags.LdFlags, "-Wl,--exclude-libs,libunwind_llvm.a")
			}
		}
	case "libstdc++":
		// Using bionic's basic libstdc++. Not actually an STL. Only around until the
		// tree is in good enough shape to not need it.
		// Host builds will use GNU libstdc++.
		if ctx.Device() {
			flags.CFlags = append(flags.CFlags, "-I"+common.PathForSource(ctx, "bionic/libstdc++/include").String())
		}
	case "ndk_system":
		ndkSrcRoot := common.PathForSource(ctx, "prebuilts/ndk/current/sources/cxx-stl/system/include")
		flags.CFlags = append(flags.CFlags, "-isystem "+ndkSrcRoot.String())
	case "ndk_libc++_shared", "ndk_libc++_static":
		// TODO(danalbert): This really shouldn't be here...
		flags.CppFlags = append(flags.CppFlags, "-std=c++11")
	case "ndk_libstlport_shared", "ndk_libstlport_static", "ndk_libgnustl_static":
		// Nothing
	case "":
		// None or error.
		if ctx.Host() {
			flags.CppFlags = append(flags.CppFlags, "-nostdinc++")
			flags.LdFlags = append(flags.LdFlags, "-nodefaultlibs")
			if c.staticBinary() {
				flags.LdFlags = append(flags.LdFlags, hostStaticGccLibs[ctx.HostType()]...)
			} else {
				flags.LdFlags = append(flags.LdFlags, hostDynamicGccLibs[ctx.HostType()]...)
			}
		}
	default:
		panic(fmt.Errorf("Unknown stl in CCLinked.Flags: %q", stl))
	}

	return flags
}

func (c *CCLinked) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	depNames = c.CCBase.depNames(ctx, depNames)

	stl := c.stl(ctx)
	if ctx.Failed() {
		return depNames
	}

	switch stl {
	case "libstdc++":
		if ctx.Device() {
			depNames.SharedLibs = append(depNames.SharedLibs, stl)
		}
	case "libc++", "libc++_static":
		if stl == "libc++" {
			depNames.SharedLibs = append(depNames.SharedLibs, stl)
		} else {
			depNames.StaticLibs = append(depNames.StaticLibs, stl)
		}
		if ctx.Device() {
			if ctx.Arch().ArchType == common.Arm {
				depNames.StaticLibs = append(depNames.StaticLibs, "libunwind_llvm")
			}
			if c.staticBinary() {
				depNames.StaticLibs = append(depNames.StaticLibs, "libdl")
			} else {
				depNames.SharedLibs = append(depNames.SharedLibs, "libdl")
			}
		}
	case "":
		// None or error.
	case "ndk_system":
		// TODO: Make a system STL prebuilt for the NDK.
		// The system STL doesn't have a prebuilt (it uses the system's libstdc++), but it does have
		// its own includes. The includes are handled in CCBase.Flags().
		depNames.SharedLibs = append([]string{"libstdc++"}, depNames.SharedLibs...)
	case "ndk_libc++_shared", "ndk_libstlport_shared":
		depNames.SharedLibs = append(depNames.SharedLibs, stl)
	case "ndk_libc++_static", "ndk_libstlport_static", "ndk_libgnustl_static":
		depNames.StaticLibs = append(depNames.StaticLibs, stl)
	default:
		panic(fmt.Errorf("Unknown stl in CCLinked.depNames: %q", stl))
	}

	if ctx.ModuleName() != "libcompiler_rt-extras" {
		depNames.StaticLibs = append(depNames.StaticLibs, "libcompiler_rt-extras")
	}

	if ctx.Device() {
		// libgcc and libatomic have to be last on the command line
		depNames.LateStaticLibs = append(depNames.LateStaticLibs, "libatomic")
		if !Bool(c.Properties.No_libgcc) {
			depNames.LateStaticLibs = append(depNames.LateStaticLibs, "libgcc")
		}

		if !c.static() {
			depNames.SharedLibs = append(depNames.SharedLibs, c.systemSharedLibs(ctx)...)
		}

		if c.Properties.Sdk_version != "" {
			version := c.Properties.Sdk_version
			depNames.SharedLibs = append(depNames.SharedLibs,
				"ndk_libc."+version,
				"ndk_libm."+version,
			)
		}
	}

	return depNames
}

// ccLinkedInterface interface is used on ccLinked to deal with static or shared variants
type ccLinkedInterface interface {
	// Returns true if the build options for the module have selected a static or shared build
	buildStatic() bool
	buildShared() bool

	// Sets whether a specific variant is static or shared
	setStatic(bool)

	// Returns whether a specific variant is a static library or binary
	static() bool

	// Returns whether a module is a static binary
	staticBinary() bool
}

var _ ccLinkedInterface = (*CCLibrary)(nil)
var _ ccLinkedInterface = (*CCBinary)(nil)

func (c *CCLinked) static() bool {
	return c.dynamicProperties.VariantIsStatic
}

func (c *CCLinked) staticBinary() bool {
	return c.dynamicProperties.VariantIsStaticBinary
}

func (c *CCLinked) setStatic(static bool) {
	c.dynamicProperties.VariantIsStatic = static
}

type ccExportedFlagsProducer interface {
	exportedFlags() []string
}

//
// Combined static+shared libraries
//

type CCLibraryProperties struct {
	BuildStatic bool `blueprint:"mutated"`
	BuildShared bool `blueprint:"mutated"`
	Static      struct {
		Srcs              []string `android:"arch_variant"`
		Exclude_srcs      []string `android:"arch_variant"`
		Cflags            []string `android:"arch_variant"`
		Whole_static_libs []string `android:"arch_variant"`
		Static_libs       []string `android:"arch_variant"`
		Shared_libs       []string `android:"arch_variant"`
	} `android:"arch_variant"`
	Shared struct {
		Srcs              []string `android:"arch_variant"`
		Exclude_srcs      []string `android:"arch_variant"`
		Cflags            []string `android:"arch_variant"`
		Whole_static_libs []string `android:"arch_variant"`
		Static_libs       []string `android:"arch_variant"`
		Shared_libs       []string `android:"arch_variant"`
	} `android:"arch_variant"`

	// local file name to pass to the linker as --version_script
	Version_script *string `android:"arch_variant"`
	// local file name to pass to the linker as -unexported_symbols_list
	Unexported_symbols_list *string `android:"arch_variant"`
	// local file name to pass to the linker as -force_symbols_not_weak_list
	Force_symbols_not_weak_list *string `android:"arch_variant"`
	// local file name to pass to the linker as -force_symbols_weak_list
	Force_symbols_weak_list *string `android:"arch_variant"`
}

type CCLibrary struct {
	CCLinked

	reuseFrom     ccLibraryInterface
	reuseObjFiles common.Paths
	objFiles      common.Paths
	exportFlags   []string
	out           common.Path
	systemLibs    []string

	LibraryProperties CCLibraryProperties
}

func (c *CCLibrary) buildStatic() bool {
	return c.LibraryProperties.BuildStatic
}

func (c *CCLibrary) buildShared() bool {
	return c.LibraryProperties.BuildShared
}

type ccLibraryInterface interface {
	ccLinkedInterface
	ccLibrary() *CCLibrary
	setReuseFrom(ccLibraryInterface)
	getReuseFrom() ccLibraryInterface
	getReuseObjFiles() common.Paths
	allObjFiles() common.Paths
}

var _ ccLibraryInterface = (*CCLibrary)(nil)

func (c *CCLibrary) ccLibrary() *CCLibrary {
	return c
}

func NewCCLibrary(library *CCLibrary, module CCModuleType,
	hod common.HostOrDeviceSupported) (blueprint.Module, []interface{}) {

	return newCCDynamic(&library.CCLinked, module, hod, common.MultilibBoth,
		&library.LibraryProperties)
}

func CCLibraryFactory() (blueprint.Module, []interface{}) {
	module := &CCLibrary{}

	module.LibraryProperties.BuildShared = true
	module.LibraryProperties.BuildStatic = true

	return NewCCLibrary(module, module, common.HostAndDeviceSupported)
}

func (c *CCLibrary) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	depNames = c.CCLinked.depNames(ctx, depNames)
	if c.static() {
		depNames.WholeStaticLibs = append(depNames.WholeStaticLibs, c.LibraryProperties.Static.Whole_static_libs...)
		depNames.StaticLibs = append(depNames.StaticLibs, c.LibraryProperties.Static.Static_libs...)
		depNames.SharedLibs = append(depNames.SharedLibs, c.LibraryProperties.Static.Shared_libs...)
	} else {
		if ctx.Device() {
			if c.Properties.Sdk_version == "" {
				depNames.CrtBegin = "crtbegin_so"
				depNames.CrtEnd = "crtend_so"
			} else {
				depNames.CrtBegin = "ndk_crtbegin_so." + c.Properties.Sdk_version
				depNames.CrtEnd = "ndk_crtend_so." + c.Properties.Sdk_version
			}
		}
		depNames.WholeStaticLibs = append(depNames.WholeStaticLibs, c.LibraryProperties.Shared.Whole_static_libs...)
		depNames.StaticLibs = append(depNames.StaticLibs, c.LibraryProperties.Shared.Static_libs...)
		depNames.SharedLibs = append(depNames.SharedLibs, c.LibraryProperties.Shared.Shared_libs...)
	}

	c.systemLibs = c.systemSharedLibs(ctx)

	return depNames
}

func (c *CCLibrary) outputFile() common.OptionalPath {
	return common.OptionalPathForPath(c.out)
}

func (c *CCLibrary) getReuseObjFiles() common.Paths {
	return c.reuseObjFiles
}

func (c *CCLibrary) setReuseFrom(reuseFrom ccLibraryInterface) {
	c.reuseFrom = reuseFrom
}

func (c *CCLibrary) getReuseFrom() ccLibraryInterface {
	return c.reuseFrom
}

func (c *CCLibrary) allObjFiles() common.Paths {
	return c.objFiles
}

func (c *CCLibrary) exportedFlags() []string {
	return c.exportFlags
}

func (c *CCLibrary) flags(ctx common.AndroidModuleContext, flags CCFlags) CCFlags {
	flags = c.CCLinked.flags(ctx, flags)

	// MinGW spits out warnings about -fPIC even for -fpie?!) being ignored because
	// all code is position independent, and then those warnings get promoted to
	// errors.
	if ctx.HostType() != common.Windows {
		flags.CFlags = append(flags.CFlags, "-fPIC")
	}

	if c.static() {
		flags.CFlags = append(flags.CFlags, c.LibraryProperties.Static.Cflags...)
	} else {
		flags.CFlags = append(flags.CFlags, c.LibraryProperties.Shared.Cflags...)
	}

	if !c.static() {
		libName := ctx.ModuleName()
		// GCC for Android assumes that -shared means -Bsymbolic, use -Wl,-shared instead
		sharedFlag := "-Wl,-shared"
		if flags.Clang || ctx.Host() {
			sharedFlag = "-shared"
		}
		if ctx.Device() {
			flags.LdFlags = append(flags.LdFlags, "-nostdlib")
		}

		if ctx.Darwin() {
			flags.LdFlags = append(flags.LdFlags,
				"-dynamiclib",
				"-single_module",
				//"-read_only_relocs suppress",
				"-install_name @rpath/"+libName+flags.Toolchain.ShlibSuffix(),
			)
		} else {
			flags.LdFlags = append(flags.LdFlags,
				"-Wl,--gc-sections",
				sharedFlag,
				"-Wl,-soname,"+libName+flags.Toolchain.ShlibSuffix(),
			)
		}
	}

	return flags
}

func (c *CCLibrary) compileStaticLibrary(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCPathDeps, objFiles common.Paths) {

	staticFlags := flags
	objFilesStatic := c.customCompileObjs(ctx, staticFlags, common.DeviceStaticLibrary,
		c.LibraryProperties.Static.Srcs, c.LibraryProperties.Static.Exclude_srcs)

	objFiles = append(objFiles, objFilesStatic...)
	objFiles = append(objFiles, deps.WholeStaticLibObjFiles...)

	outputFile := common.PathForModuleOut(ctx, ctx.ModuleName()+staticLibraryExtension)

	if ctx.Darwin() {
		TransformDarwinObjToStaticLib(ctx, objFiles, ccFlagsToBuilderFlags(flags), outputFile)
	} else {
		TransformObjToStaticLib(ctx, objFiles, ccFlagsToBuilderFlags(flags), outputFile)
	}

	c.objFiles = objFiles
	c.out = outputFile

	includeDirs := common.PathsForModuleSrc(ctx, c.Properties.Export_include_dirs)
	c.exportFlags = []string{includeDirsToFlags(includeDirs)}
	c.exportFlags = append(c.exportFlags, deps.ReexportedCflags...)

	ctx.CheckbuildFile(outputFile)
}

func (c *CCLibrary) compileSharedLibrary(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCPathDeps, objFiles common.Paths) {

	sharedFlags := flags
	objFilesShared := c.customCompileObjs(ctx, sharedFlags, common.DeviceSharedLibrary,
		c.LibraryProperties.Shared.Srcs, c.LibraryProperties.Shared.Exclude_srcs)

	objFiles = append(objFiles, objFilesShared...)

	outputFile := common.PathForModuleOut(ctx, ctx.ModuleName()+flags.Toolchain.ShlibSuffix())

	var linkerDeps common.Paths

	versionScript := common.OptionalPathForModuleSrc(ctx, c.LibraryProperties.Version_script)
	unexportedSymbols := common.OptionalPathForModuleSrc(ctx, c.LibraryProperties.Unexported_symbols_list)
	forceNotWeakSymbols := common.OptionalPathForModuleSrc(ctx, c.LibraryProperties.Force_symbols_not_weak_list)
	forceWeakSymbols := common.OptionalPathForModuleSrc(ctx, c.LibraryProperties.Force_symbols_weak_list)
	if !ctx.Darwin() {
		if versionScript.Valid() {
			sharedFlags.LdFlags = append(sharedFlags.LdFlags, "-Wl,--version-script,"+versionScript.String())
			linkerDeps = append(linkerDeps, versionScript.Path())
		}
		if unexportedSymbols.Valid() {
			ctx.PropertyErrorf("unexported_symbols_list", "Only supported on Darwin")
		}
		if forceNotWeakSymbols.Valid() {
			ctx.PropertyErrorf("force_symbols_not_weak_list", "Only supported on Darwin")
		}
		if forceWeakSymbols.Valid() {
			ctx.PropertyErrorf("force_symbols_weak_list", "Only supported on Darwin")
		}
	} else {
		if versionScript.Valid() {
			ctx.PropertyErrorf("version_script", "Not supported on Darwin")
		}
		if unexportedSymbols.Valid() {
			sharedFlags.LdFlags = append(sharedFlags.LdFlags, "-Wl,-unexported_symbols_list,"+unexportedSymbols.String())
			linkerDeps = append(linkerDeps, unexportedSymbols.Path())
		}
		if forceNotWeakSymbols.Valid() {
			sharedFlags.LdFlags = append(sharedFlags.LdFlags, "-Wl,-force_symbols_not_weak_list,"+forceNotWeakSymbols.String())
			linkerDeps = append(linkerDeps, forceNotWeakSymbols.Path())
		}
		if forceWeakSymbols.Valid() {
			sharedFlags.LdFlags = append(sharedFlags.LdFlags, "-Wl,-force_symbols_weak_list,"+forceWeakSymbols.String())
			linkerDeps = append(linkerDeps, forceWeakSymbols.Path())
		}
	}

	TransformObjToDynamicBinary(ctx, objFiles, deps.SharedLibs, deps.StaticLibs,
		deps.LateStaticLibs, deps.WholeStaticLibs, linkerDeps, deps.CrtBegin, deps.CrtEnd, false,
		ccFlagsToBuilderFlags(sharedFlags), outputFile)

	c.out = outputFile
	includeDirs := common.PathsForModuleSrc(ctx, c.Properties.Export_include_dirs)
	c.exportFlags = []string{includeDirsToFlags(includeDirs)}
	c.exportFlags = append(c.exportFlags, deps.ReexportedCflags...)
}

func (c *CCLibrary) compileModule(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCPathDeps, objFiles common.Paths) {

	// Reuse the object files from the matching static library if it exists
	if c.getReuseFrom().ccLibrary() == c {
		c.reuseObjFiles = objFiles
	} else {
		if c.getReuseFrom().ccLibrary().LibraryProperties.Static.Cflags == nil &&
			c.LibraryProperties.Shared.Cflags == nil {
			objFiles = append(common.Paths(nil), c.getReuseFrom().getReuseObjFiles()...)
		}
	}

	if c.static() {
		c.compileStaticLibrary(ctx, flags, deps, objFiles)
	} else {
		c.compileSharedLibrary(ctx, flags, deps, objFiles)
	}
}

func (c *CCLibrary) installStaticLibrary(ctx common.AndroidModuleContext, flags CCFlags) {
	// Static libraries do not get installed.
}

func (c *CCLibrary) installSharedLibrary(ctx common.AndroidModuleContext, flags CCFlags) {
	installDir := "lib"
	if flags.Toolchain.Is64Bit() {
		installDir = "lib64"
	}

	ctx.InstallFile(common.PathForModuleInstall(ctx, installDir, c.Properties.Relative_install_path), c.out)
}

func (c *CCLibrary) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	if c.static() {
		c.installStaticLibrary(ctx, flags)
	} else {
		c.installSharedLibrary(ctx, flags)
	}
}

//
// Objects (for crt*.o)
//

type ccObjectProvider interface {
	object() *ccObject
}

type ccObject struct {
	CCBase
	out common.OptionalPath
}

func (c *ccObject) object() *ccObject {
	return c
}

func CCObjectFactory() (blueprint.Module, []interface{}) {
	module := &ccObject{}

	return newCCBase(&module.CCBase, module, common.DeviceSupported, common.MultilibBoth)
}

func (*ccObject) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	// object files can't have any dynamic dependencies
	return CCDeps{}
}

func (c *ccObject) compileModule(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCPathDeps, objFiles common.Paths) {

	objFiles = append(objFiles, deps.ObjFiles...)

	var outputFile common.Path
	if len(objFiles) == 1 {
		outputFile = objFiles[0]
	} else {
		output := common.PathForModuleOut(ctx, ctx.ModuleName()+objectExtension)
		TransformObjsToObj(ctx, objFiles, ccFlagsToBuilderFlags(flags), output)
		outputFile = output
	}

	c.out = common.OptionalPathForPath(outputFile)

	ctx.CheckbuildFile(outputFile)
}

func (c *ccObject) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	// Object files do not get installed.
}

func (c *ccObject) outputFile() common.OptionalPath {
	return c.out
}

var _ ccObjectProvider = (*ccObject)(nil)

//
// Executables
//

type CCBinaryProperties struct {
	// compile executable with -static
	Static_executable *bool

	// set the name of the output
	Stem string `android:"arch_variant"`

	// append to the name of the output
	Suffix string `android:"arch_variant"`

	// if set, add an extra objcopy --prefix-symbols= step
	Prefix_symbols string
}

type CCBinary struct {
	CCLinked
	out              common.Path
	installFile      common.Path
	BinaryProperties CCBinaryProperties
}

func (c *CCBinary) buildStatic() bool {
	return Bool(c.BinaryProperties.Static_executable)
}

func (c *CCBinary) buildShared() bool {
	return !Bool(c.BinaryProperties.Static_executable)
}

func (c *CCBinary) getStem(ctx common.AndroidModuleContext) string {
	stem := ctx.ModuleName()
	if c.BinaryProperties.Stem != "" {
		stem = c.BinaryProperties.Stem
	}

	return stem + c.BinaryProperties.Suffix
}

func (c *CCBinary) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	depNames = c.CCLinked.depNames(ctx, depNames)
	if ctx.Device() {
		if c.Properties.Sdk_version == "" {
			if Bool(c.BinaryProperties.Static_executable) {
				depNames.CrtBegin = "crtbegin_static"
			} else {
				depNames.CrtBegin = "crtbegin_dynamic"
			}
			depNames.CrtEnd = "crtend_android"
		} else {
			if Bool(c.BinaryProperties.Static_executable) {
				depNames.CrtBegin = "ndk_crtbegin_static." + c.Properties.Sdk_version
			} else {
				depNames.CrtBegin = "ndk_crtbegin_dynamic." + c.Properties.Sdk_version
			}
			depNames.CrtEnd = "ndk_crtend_android." + c.Properties.Sdk_version
		}

		if Bool(c.BinaryProperties.Static_executable) {
			if c.stl(ctx) == "libc++_static" {
				depNames.StaticLibs = append(depNames.StaticLibs, "libm", "libc", "libdl")
			}
			// static libraries libcompiler_rt, libc and libc_nomalloc need to be linked with
			// --start-group/--end-group along with libgcc.  If they are in deps.StaticLibs,
			// move them to the beginning of deps.LateStaticLibs
			var groupLibs []string
			depNames.StaticLibs, groupLibs = filterList(depNames.StaticLibs,
				[]string{"libc", "libc_nomalloc", "libcompiler_rt"})
			depNames.LateStaticLibs = append(groupLibs, depNames.LateStaticLibs...)
		}
	}
	return depNames
}

func NewCCBinary(binary *CCBinary, module CCModuleType,
	hod common.HostOrDeviceSupported, multilib common.Multilib,
	props ...interface{}) (blueprint.Module, []interface{}) {

	props = append(props, &binary.BinaryProperties)

	return newCCDynamic(&binary.CCLinked, module, hod, multilib, props...)
}

func CCBinaryFactory() (blueprint.Module, []interface{}) {
	module := &CCBinary{}

	return NewCCBinary(module, module, common.HostAndDeviceSupported, common.MultilibFirst)
}

func (c *CCBinary) ModifyProperties(ctx CCModuleContext) {
	if ctx.Darwin() {
		c.BinaryProperties.Static_executable = proptools.BoolPtr(false)
	}
	if Bool(c.BinaryProperties.Static_executable) {
		c.dynamicProperties.VariantIsStaticBinary = true
	}
}

func (c *CCBinary) flags(ctx common.AndroidModuleContext, flags CCFlags) CCFlags {
	flags = c.CCLinked.flags(ctx, flags)

	if ctx.Host() {
		flags.LdFlags = append(flags.LdFlags, "-pie")
		if ctx.HostType() == common.Windows {
			flags.LdFlags = append(flags.LdFlags, "-Wl,-e_mainCRTStartup")
		}
	}

	// MinGW spits out warnings about -fPIC even for -fpie?!) being ignored because
	// all code is position independent, and then those warnings get promoted to
	// errors.
	if ctx.HostType() != common.Windows {
		flags.CFlags = append(flags.CFlags, "-fpie")
	}

	if ctx.Device() {
		if Bool(c.BinaryProperties.Static_executable) {
			// Clang driver needs -static to create static executable.
			// However, bionic/linker uses -shared to overwrite.
			// Linker for x86 targets does not allow coexistance of -static and -shared,
			// so we add -static only if -shared is not used.
			if !inList("-shared", flags.LdFlags) {
				flags.LdFlags = append(flags.LdFlags, "-static")
			}

			flags.LdFlags = append(flags.LdFlags,
				"-nostdlib",
				"-Bstatic",
				"-Wl,--gc-sections",
			)

		} else {
			linker := "/system/bin/linker"
			if flags.Toolchain.Is64Bit() {
				linker = "/system/bin/linker64"
			}

			flags.LdFlags = append(flags.LdFlags,
				"-pie",
				"-nostdlib",
				"-Bdynamic",
				fmt.Sprintf("-Wl,-dynamic-linker,%s", linker),
				"-Wl,--gc-sections",
				"-Wl,-z,nocopyreloc",
			)
		}
	} else if ctx.Darwin() {
		flags.LdFlags = append(flags.LdFlags, "-Wl,-headerpad_max_install_names")
	}

	return flags
}

func (c *CCBinary) compileModule(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCPathDeps, objFiles common.Paths) {

	if !Bool(c.BinaryProperties.Static_executable) && inList("libc", c.Properties.Static_libs) {
		ctx.ModuleErrorf("statically linking libc to dynamic executable, please remove libc\n" +
			"from static libs or set static_executable: true")
	}

	outputFile := common.PathForModuleOut(ctx, c.getStem(ctx)+flags.Toolchain.ExecutableSuffix())
	c.out = outputFile
	if c.BinaryProperties.Prefix_symbols != "" {
		afterPrefixSymbols := outputFile
		outputFile = common.PathForModuleOut(ctx, c.getStem(ctx)+".intermediate")
		TransformBinaryPrefixSymbols(ctx, c.BinaryProperties.Prefix_symbols, outputFile,
			ccFlagsToBuilderFlags(flags), afterPrefixSymbols)
	}

	var linkerDeps common.Paths

	TransformObjToDynamicBinary(ctx, objFiles, deps.SharedLibs, deps.StaticLibs,
		deps.LateStaticLibs, deps.WholeStaticLibs, linkerDeps, deps.CrtBegin, deps.CrtEnd, true,
		ccFlagsToBuilderFlags(flags), outputFile)
}

func (c *CCBinary) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	c.installFile = ctx.InstallFile(common.PathForModuleInstall(ctx, "bin", c.Properties.Relative_install_path), c.out)
}

func (c *CCBinary) HostToolPath() common.OptionalPath {
	if c.HostOrDevice().Host() {
		return common.OptionalPathForPath(c.installFile)
	}
	return common.OptionalPath{}
}

func (c *CCBinary) binary() *CCBinary {
	return c
}

type testPerSrc interface {
	binary() *CCBinary
	testPerSrc() bool
}

var _ testPerSrc = (*CCTest)(nil)

func testPerSrcMutator(mctx common.AndroidBottomUpMutatorContext) {
	if test, ok := mctx.Module().(testPerSrc); ok {
		if test.testPerSrc() {
			testNames := make([]string, len(test.binary().Properties.Srcs))
			for i, src := range test.binary().Properties.Srcs {
				testNames[i] = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
			}
			tests := mctx.CreateLocalVariations(testNames...)
			for i, src := range test.binary().Properties.Srcs {
				tests[i].(testPerSrc).binary().Properties.Srcs = []string{src}
				tests[i].(testPerSrc).binary().BinaryProperties.Stem = testNames[i]
			}
		}
	}
}

type CCTestProperties struct {
	// if set, build against the gtest library. Defaults to true.
	Gtest bool

	// Create a separate binary for each source file.  Useful when there is
	// global state that can not be torn down and reset between each test suite.
	Test_per_src *bool
}

type CCTest struct {
	CCBinary

	TestProperties CCTestProperties
}

func (c *CCTest) flags(ctx common.AndroidModuleContext, flags CCFlags) CCFlags {
	flags = c.CCBinary.flags(ctx, flags)
	if !c.TestProperties.Gtest {
		return flags
	}

	flags.CFlags = append(flags.CFlags, "-DGTEST_HAS_STD_STRING")
	if ctx.Host() {
		flags.CFlags = append(flags.CFlags, "-O0", "-g")

		if ctx.HostType() == common.Windows {
			flags.CFlags = append(flags.CFlags, "-DGTEST_OS_WINDOWS")
		} else {
			flags.CFlags = append(flags.CFlags, "-DGTEST_OS_LINUX")
			flags.LdFlags = append(flags.LdFlags, "-lpthread")
		}
	} else {
		flags.CFlags = append(flags.CFlags, "-DGTEST_OS_LINUX_ANDROID")
	}

	// TODO(danalbert): Make gtest export its dependencies.
	flags.CFlags = append(flags.CFlags,
		"-I"+common.PathForSource(ctx, "external/gtest/include").String())

	return flags
}

func (c *CCTest) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	if c.TestProperties.Gtest {
		depNames.StaticLibs = append(depNames.StaticLibs, "libgtest_main", "libgtest")
	}
	depNames = c.CCBinary.depNames(ctx, depNames)
	return depNames
}

func (c *CCTest) InstallInData() bool {
	return true
}

func (c *CCTest) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	installDir := "nativetest"
	if flags.Toolchain.Is64Bit() {
		installDir = "nativetest64"
	}
	ctx.InstallFile(common.PathForModuleInstall(ctx, installDir, ctx.ModuleName()), c.out)
}

func (c *CCTest) testPerSrc() bool {
	return Bool(c.TestProperties.Test_per_src)
}

func NewCCTest(test *CCTest, module CCModuleType,
	hod common.HostOrDeviceSupported, props ...interface{}) (blueprint.Module, []interface{}) {

	props = append(props, &test.TestProperties)

	return NewCCBinary(&test.CCBinary, module, hod, common.MultilibBoth, props...)
}

func CCTestFactory() (blueprint.Module, []interface{}) {
	module := &CCTest{}
	module.TestProperties.Gtest = true

	return NewCCTest(module, module, common.HostAndDeviceSupported)
}

type CCBenchmark struct {
	CCBinary
}

func (c *CCBenchmark) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	depNames = c.CCBinary.depNames(ctx, depNames)
	depNames.StaticLibs = append(depNames.StaticLibs, "libbenchmark", "libbase")
	return depNames
}

func (c *CCBenchmark) InstallInData() bool {
	return true
}

func (c *CCBenchmark) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	if ctx.Device() {
		installDir := "nativetest"
		if flags.Toolchain.Is64Bit() {
			installDir = "nativetest64"
		}
		ctx.InstallFile(common.PathForModuleInstall(ctx, installDir, ctx.ModuleName()), c.out)
	} else {
		c.CCBinary.installModule(ctx, flags)
	}
}

func NewCCBenchmark(test *CCBenchmark, module CCModuleType,
	hod common.HostOrDeviceSupported, props ...interface{}) (blueprint.Module, []interface{}) {

	return NewCCBinary(&test.CCBinary, module, hod, common.MultilibFirst, props...)
}

func CCBenchmarkFactory() (blueprint.Module, []interface{}) {
	module := &CCBenchmark{}

	return NewCCBenchmark(module, module, common.HostAndDeviceSupported)
}

//
// Static library
//

func CCLibraryStaticFactory() (blueprint.Module, []interface{}) {
	module := &CCLibrary{}
	module.LibraryProperties.BuildStatic = true

	return NewCCLibrary(module, module, common.HostAndDeviceSupported)
}

//
// Shared libraries
//

func CCLibrarySharedFactory() (blueprint.Module, []interface{}) {
	module := &CCLibrary{}
	module.LibraryProperties.BuildShared = true

	return NewCCLibrary(module, module, common.HostAndDeviceSupported)
}

//
// Host static library
//

func CCLibraryHostStaticFactory() (blueprint.Module, []interface{}) {
	module := &CCLibrary{}
	module.LibraryProperties.BuildStatic = true

	return NewCCLibrary(module, module, common.HostSupported)
}

//
// Host Shared libraries
//

func CCLibraryHostSharedFactory() (blueprint.Module, []interface{}) {
	module := &CCLibrary{}
	module.LibraryProperties.BuildShared = true

	return NewCCLibrary(module, module, common.HostSupported)
}

//
// Host Binaries
//

func CCBinaryHostFactory() (blueprint.Module, []interface{}) {
	module := &CCBinary{}

	return NewCCBinary(module, module, common.HostSupported, common.MultilibFirst)
}

//
// Host Tests
//

func CCTestHostFactory() (blueprint.Module, []interface{}) {
	module := &CCTest{}
	return NewCCTest(module, module, common.HostSupported)
}

//
// Host Benchmarks
//

func CCBenchmarkHostFactory() (blueprint.Module, []interface{}) {
	module := &CCBenchmark{}
	return NewCCBinary(&module.CCBinary, module, common.HostSupported, common.MultilibFirst)
}

//
// Defaults
//
type CCDefaults struct {
	common.AndroidModuleBase
	common.DefaultsModule
}

func (*CCDefaults) GenerateAndroidBuildActions(ctx common.AndroidModuleContext) {
}

func CCDefaultsFactory() (blueprint.Module, []interface{}) {
	module := &CCDefaults{}

	propertyStructs := []interface{}{
		&CCBaseProperties{},
		&CCLibraryProperties{},
		&CCBinaryProperties{},
		&CCTestProperties{},
		&CCUnusedProperties{},
	}

	_, propertyStructs = common.InitAndroidArchModule(module, common.HostAndDeviceDefault,
		common.MultilibDefault, propertyStructs...)

	return common.InitDefaultsModule(module, module, propertyStructs...)
}

//
// Device libraries shipped with gcc
//

type toolchainLibrary struct {
	CCLibrary
}

func (*toolchainLibrary) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	// toolchain libraries can't have any dependencies
	return CCDeps{}
}

func ToolchainLibraryFactory() (blueprint.Module, []interface{}) {
	module := &toolchainLibrary{}

	module.LibraryProperties.BuildStatic = true

	return newCCBase(&module.CCBase, module, common.DeviceSupported, common.MultilibBoth,
		&module.LibraryProperties)
}

func (c *toolchainLibrary) compileModule(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCPathDeps, objFiles common.Paths) {

	libName := ctx.ModuleName() + staticLibraryExtension
	outputFile := common.PathForModuleOut(ctx, libName)

	CopyGccLib(ctx, libName, ccFlagsToBuilderFlags(flags), outputFile)

	c.out = outputFile

	ctx.CheckbuildFile(outputFile)
}

func (c *toolchainLibrary) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	// Toolchain libraries do not get installed.
}

// NDK prebuilt libraries.
//
// These differ from regular prebuilts in that they aren't stripped and usually aren't installed
// either (with the exception of the shared STLs, which are installed to the app's directory rather
// than to the system image).

func getNdkLibDir(ctx common.AndroidModuleContext, toolchain Toolchain, version string) common.SourcePath {
	return common.PathForSource(ctx, fmt.Sprintf("prebuilts/ndk/current/platforms/android-%s/arch-%s/usr/lib",
		version, toolchain.Name()))
}

func ndkPrebuiltModuleToPath(ctx common.AndroidModuleContext, toolchain Toolchain,
	ext string, version string) common.Path {

	// NDK prebuilts are named like: ndk_NAME.EXT.SDK_VERSION.
	// We want to translate to just NAME.EXT
	name := strings.Split(strings.TrimPrefix(ctx.ModuleName(), "ndk_"), ".")[0]
	dir := getNdkLibDir(ctx, toolchain, version)
	return dir.Join(ctx, name+ext)
}

type ndkPrebuiltObject struct {
	ccObject
}

func (*ndkPrebuiltObject) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	// NDK objects can't have any dependencies
	return CCDeps{}
}

func NdkPrebuiltObjectFactory() (blueprint.Module, []interface{}) {
	module := &ndkPrebuiltObject{}
	return newCCBase(&module.CCBase, module, common.DeviceSupported, common.MultilibBoth)
}

func (c *ndkPrebuiltObject) compileModule(ctx common.AndroidModuleContext, flags CCFlags,
	deps CCPathDeps, objFiles common.Paths) {
	// A null build step, but it sets up the output path.
	if !strings.HasPrefix(ctx.ModuleName(), "ndk_crt") {
		ctx.ModuleErrorf("NDK prebuilts must have an ndk_crt prefixed name")
	}

	c.out = common.OptionalPathForPath(ndkPrebuiltModuleToPath(ctx, flags.Toolchain, objectExtension, c.Properties.Sdk_version))
}

func (c *ndkPrebuiltObject) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	// Objects do not get installed.
}

var _ ccObjectProvider = (*ndkPrebuiltObject)(nil)

type ndkPrebuiltLibrary struct {
	CCLibrary
}

func (*ndkPrebuiltLibrary) depNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	// NDK libraries can't have any dependencies
	return CCDeps{}
}

func NdkPrebuiltLibraryFactory() (blueprint.Module, []interface{}) {
	module := &ndkPrebuiltLibrary{}
	module.LibraryProperties.BuildShared = true
	return NewCCLibrary(&module.CCLibrary, module, common.DeviceSupported)
}

func (c *ndkPrebuiltLibrary) compileModule(ctx common.AndroidModuleContext, flags CCFlags,
	deps CCPathDeps, objFiles common.Paths) {
	// A null build step, but it sets up the output path.
	if !strings.HasPrefix(ctx.ModuleName(), "ndk_lib") {
		ctx.ModuleErrorf("NDK prebuilts must have an ndk_lib prefixed name")
	}

	includeDirs := common.PathsForModuleSrc(ctx, c.Properties.Export_include_dirs)
	c.exportFlags = []string{common.JoinWithPrefix(includeDirs.Strings(), "-isystem ")}

	c.out = ndkPrebuiltModuleToPath(ctx, flags.Toolchain, flags.Toolchain.ShlibSuffix(),
		c.Properties.Sdk_version)
}

func (c *ndkPrebuiltLibrary) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	// NDK prebuilt libraries do not get installed.
}

// The NDK STLs are slightly different from the prebuilt system libraries:
//     * Are not specific to each platform version.
//     * The libraries are not in a predictable location for each STL.

type ndkPrebuiltStl struct {
	ndkPrebuiltLibrary
}

type ndkPrebuiltStaticStl struct {
	ndkPrebuiltStl
}

type ndkPrebuiltSharedStl struct {
	ndkPrebuiltStl
}

func NdkPrebuiltSharedStlFactory() (blueprint.Module, []interface{}) {
	module := &ndkPrebuiltSharedStl{}
	module.LibraryProperties.BuildShared = true
	return NewCCLibrary(&module.CCLibrary, module, common.DeviceSupported)
}

func NdkPrebuiltStaticStlFactory() (blueprint.Module, []interface{}) {
	module := &ndkPrebuiltStaticStl{}
	module.LibraryProperties.BuildStatic = true
	return NewCCLibrary(&module.CCLibrary, module, common.DeviceSupported)
}

func getNdkStlLibDir(ctx common.AndroidModuleContext, toolchain Toolchain, stl string) common.SourcePath {
	gccVersion := toolchain.GccVersion()
	var libDir string
	switch stl {
	case "libstlport":
		libDir = "cxx-stl/stlport/libs"
	case "libc++":
		libDir = "cxx-stl/llvm-libc++/libs"
	case "libgnustl":
		libDir = fmt.Sprintf("cxx-stl/gnu-libstdc++/%s/libs", gccVersion)
	}

	if libDir != "" {
		ndkSrcRoot := "prebuilts/ndk/current/sources"
		return common.PathForSource(ctx, ndkSrcRoot).Join(ctx, libDir, ctx.Arch().Abi[0])
	}

	ctx.ModuleErrorf("Unknown NDK STL: %s", stl)
	return common.PathForSource(ctx, "")
}

func (c *ndkPrebuiltStl) compileModule(ctx common.AndroidModuleContext, flags CCFlags,
	deps CCPathDeps, objFiles common.Paths) {
	// A null build step, but it sets up the output path.
	if !strings.HasPrefix(ctx.ModuleName(), "ndk_lib") {
		ctx.ModuleErrorf("NDK prebuilts must have an ndk_lib prefixed name")
	}

	includeDirs := common.PathsForModuleSrc(ctx, c.Properties.Export_include_dirs)
	c.exportFlags = []string{includeDirsToFlags(includeDirs)}

	libName := strings.TrimPrefix(ctx.ModuleName(), "ndk_")
	libExt := flags.Toolchain.ShlibSuffix()
	if c.LibraryProperties.BuildStatic {
		libExt = staticLibraryExtension
	}

	stlName := strings.TrimSuffix(libName, "_shared")
	stlName = strings.TrimSuffix(stlName, "_static")
	libDir := getNdkStlLibDir(ctx, flags.Toolchain, stlName)
	c.out = libDir.Join(ctx, libName+libExt)
}

func linkageMutator(mctx common.AndroidBottomUpMutatorContext) {
	if c, ok := mctx.Module().(ccLinkedInterface); ok {
		var modules []blueprint.Module
		if c.buildStatic() && c.buildShared() {
			modules = mctx.CreateLocalVariations("static", "shared")
			modules[0].(ccLinkedInterface).setStatic(true)
			modules[1].(ccLinkedInterface).setStatic(false)
		} else if c.buildStatic() {
			modules = mctx.CreateLocalVariations("static")
			modules[0].(ccLinkedInterface).setStatic(true)
		} else if c.buildShared() {
			modules = mctx.CreateLocalVariations("shared")
			modules[0].(ccLinkedInterface).setStatic(false)
		} else {
			panic(fmt.Errorf("ccLibrary %q not static or shared", mctx.ModuleName()))
		}

		if _, ok := c.(ccLibraryInterface); ok {
			reuseFrom := modules[0].(ccLibraryInterface)
			for _, m := range modules {
				m.(ccLibraryInterface).setReuseFrom(reuseFrom)
			}
		}
	}
}

// lastUniqueElements returns all unique elements of a slice, keeping the last copy of each
// modifies the slice contents in place, and returns a subslice of the original slice
func lastUniqueElements(list []string) []string {
	totalSkip := 0
	for i := len(list) - 1; i >= totalSkip; i-- {
		skip := 0
		for j := i - 1; j >= totalSkip; j-- {
			if list[i] == list[j] {
				skip++
			} else {
				list[j+skip] = list[j]
			}
		}
		totalSkip += skip
	}
	return list[totalSkip:]
}

var Bool = proptools.Bool
