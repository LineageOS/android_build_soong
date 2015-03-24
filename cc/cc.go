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
	"github.com/google/blueprint/pathtools"

	"android/soong/common"
	"android/soong/genrule"
)

type Config interface {
	SrcDir() string
	PrebuiltOS() string
}

var (
	HostPrebuiltTag = pctx.VariableConfigMethod("HostPrebuiltTag", Config.PrebuiltOS)
	SrcDir          = pctx.VariableConfigMethod("SrcDir", Config.SrcDir)

	LibcRoot = pctx.StaticVariable("LibcRoot", "${SrcDir}/bionic/libc")
	LibmRoot = pctx.StaticVariable("LibmRoot", "${SrcDir}/bionic/libm")
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

		// COMMON_RELEASE_CFLAGS
		"-DNDEBUG",
		"-UDEBUG",
	}

	deviceGlobalCflags = []string{
		// TARGET_ERROR_FLAGS
		"-Werror=return-type",
		"-Werror=non-virtual-dtor",
		"-Werror=address",
		"-Werror=sequence-point",
	}

	hostGlobalCflags = []string{}

	commonGlobalCppflags = []string{
		"-Wsign-promo",
		"-std=gnu++11",
	}
)

func init() {
	pctx.StaticVariable("commonGlobalCflags", strings.Join(commonGlobalCflags, " "))
	pctx.StaticVariable("deviceGlobalCflags", strings.Join(deviceGlobalCflags, " "))
	pctx.StaticVariable("hostGlobalCflags", strings.Join(hostGlobalCflags, " "))

	pctx.StaticVariable("commonGlobalCppflags", strings.Join(commonGlobalCppflags, " "))

	pctx.StaticVariable("commonClangGlobalCflags",
		strings.Join(clangFilterUnknownCflags(commonGlobalCflags), " "))
	pctx.StaticVariable("deviceClangGlobalCflags",
		strings.Join(clangFilterUnknownCflags(deviceGlobalCflags), " "))
	pctx.StaticVariable("hostClangGlobalCflags",
		strings.Join(clangFilterUnknownCflags(hostGlobalCflags), " "))
	pctx.StaticVariable("commonClangGlobalCppflags",
		strings.Join(clangFilterUnknownCflags(commonGlobalCppflags), " "))

	// Everything in this list is a crime against abstraction and dependency tracking.
	// Do not add anything to this list.
	pctx.StaticVariable("commonGlobalIncludes", strings.Join([]string{
		"-isystem ${SrcDir}/system/core/include",
		"-isystem ${SrcDir}/hardware/libhardware/include",
		"-isystem ${SrcDir}/hardware/libhardware_legacy/include",
		"-isystem ${SrcDir}/hardware/ril/include",
		"-isystem ${SrcDir}/libnativehelper/include",
		"-isystem ${SrcDir}/frameworks/native/include",
		"-isystem ${SrcDir}/frameworks/native/opengl/include",
		"-isystem ${SrcDir}/frameworks/av/include",
		"-isystem ${SrcDir}/frameworks/base/include",
	}, " "))

	pctx.StaticVariable("clangPath", "${SrcDir}/prebuilts/clang/${HostPrebuiltTag}/host/3.6/bin/")
}

// ccProperties describes properties used to compile all C or C++ modules
type ccProperties struct {
	// srcs: list of source files used to compile the C/C++ module.  May be .c, .cpp, or .S files.
	Srcs []string `android:"arch_variant,arch_subtract"`

	// cflags: list of module-specific flags that will be used for C and C++ compiles.
	Cflags []string `android:"arch_variant"`

	// cppflags: list of module-specific flags that will be used for C++ compiles
	Cppflags []string `android:"arch_variant"`

	// conlyflags: list of module-specific flags that will be used for C compiles
	Conlyflags []string `android:"arch_variant"`

	// asflags: list of module-specific flags that will be used for .S compiles
	Asflags []string `android:"arch_variant"`

	// ldflags: list of module-specific flags that will be used for all link steps
	Ldflags []string `android:"arch_variant"`

	// instruction_set: the instruction set architecture to use to compile the C/C++
	// module.
	Instruction_set string `android:"arch_variant"`

	// include_dirs: list of directories relative to the root of the source tree that will
	// be added to the include path using -I.
	// If possible, don't use this.  If adding paths from the current directory use
	// local_include_dirs, if adding paths from other modules use export_include_dirs in
	// that module.
	Include_dirs []string `android:"arch_variant"`

	// local_include_dirs: list of directories relative to the Blueprints file that will
	// be added to the include path using -I
	Local_include_dirs []string `android:"arch_variant"`

	// export_include_dirs: list of directories relative to the Blueprints file that will
	// be added to the include path using -I for any module that links against this module
	Export_include_dirs []string

	// clang_cflags: list of module-specific flags that will be used for C and C++ compiles when
	// compiling with clang
	Clang_cflags []string `android:"arch_variant"`

	// clang_asflags: list of module-specific flags that will be used for .S compiles when
	// compiling with clang
	Clang_asflags []string `android:"arch_variant"`

	// system_shared_libs: list of system libraries that will be dynamically linked to
	// shared library and executable modules.  If unset, generally defaults to libc
	// and libm.  Set to [] to prevent linking against libc and libm.
	System_shared_libs []string

	// whole_static_libs: list of modules whose object files should be linked into this module
	// in their entirety.  For static library modules, all of the .o files from the intermediate
	// directory of the dependency will be linked into this modules .a file.  For a shared library,
	// the dependency's .a file will be linked into this module using -Wl,--whole-archive.
	Whole_static_libs []string `android:"arch_variant"`

	// static_libs: list of modules that should be statically linked into this module.
	Static_libs []string `android:"arch_variant"`

	// shared_libs: list of modules that should be dynamically linked into this module.
	Shared_libs []string `android:"arch_variant"`

	// allow_undefined_symbols: allow the module to contain undefined symbols.  By default,
	// modules cannot contain undefined symbols that are not satisified by their immediate
	// dependencies.  Set this flag to true to remove --no-undefined from the linker flags.
	// This flag should only be necessary for compiling low-level libraries like libc.
	Allow_undefined_symbols bool

	// nocrt: don't link in crt_begin and crt_end.  This flag should only be necessary for
	// compiling crt or libc.
	Nocrt bool `android:"arch_variant"`

	// no_default_compiler_flags: don't insert default compiler flags into asflags, cflags,
	// cppflags, conlyflags, ldflags, or include_dirs
	No_default_compiler_flags bool

	// clang: compile module with clang instead of gcc
	Clang bool `android:"arch_variant"`

	// rtti: pass -frtti instead of -fno-rtti
	Rtti bool

	// host_ldlibs: -l arguments to pass to linker for host-provided shared libraries
	Host_ldlibs []string `android:"arch_variant"`

	// stl: select the STL library to use.  Possible values are "libc++", "libc++_static",
	// "stlport", "stlport_static", "ndk", "libstdc++", or "none".  Leave blank to select the
	// default
	Stl string

	// Set for combined shared/static libraries to prevent compiling object files a second time
	SkipCompileObjs bool `blueprint:"mutated"`

	Debug struct {
		Cflags []string `android:"arch_variant"`
	} `android:"arch_variant"`
	Release struct {
		Cflags []string `android:"arch_variant"`
	} `android:"arch_variant"`

	// Minimum sdk version supported when compiling against the ndk
	Sdk_version string
}

type unusedProperties struct {
	Asan            bool
	Native_coverage bool
	Strip           string
	Tags            []string
	Required        []string
}

// Building C/C++ code is handled by objects that satisfy this interface via composition
type CCModuleType interface {
	common.AndroidModule

	// Modify the ccFlags
	Flags(common.AndroidModuleContext, CCFlags) CCFlags

	// Return list of dependency names for use in AndroidDynamicDependencies and in depsToPaths
	DepNames(common.AndroidBaseContext, CCDeps) CCDeps

	// Compile objects into final module
	compileModule(common.AndroidModuleContext, CCFlags, CCDeps, []string)

	// Install the built module.
	installModule(common.AndroidModuleContext, CCFlags)

	// Return the output file (.o, .a or .so) for use by other modules
	outputFile() string
}

type CCDeps struct {
	StaticLibs, SharedLibs, LateStaticLibs, WholeStaticLibs, ObjFiles, IncludeDirs []string

	WholeStaticLibObjFiles []string

	CrtBegin, CrtEnd string
}

type CCFlags struct {
	GlobalFlags []string
	AsFlags     []string
	CFlags      []string
	ConlyFlags  []string
	CppFlags    []string
	LdFlags     []string
	LdLibs      []string
	IncludeDirs []string
	Nocrt       bool
	Toolchain   Toolchain
	Clang       bool
}

// ccBase contains the properties and members used by all C/C++ module types, and implements
// the blueprint.Module interface.  It expects to be embedded into an outer specialization struct,
// and uses a ccModuleType interface to that struct to create the build steps.
type ccBase struct {
	common.AndroidModuleBase
	module CCModuleType

	properties ccProperties
	unused     unusedProperties

	installPath string
}

func newCCBase(base *ccBase, module CCModuleType, hod common.HostOrDeviceSupported,
	multilib common.Multilib, props ...interface{}) (blueprint.Module, []interface{}) {

	base.module = module

	props = append(props, &base.properties, &base.unused)

	return common.InitAndroidArchModule(module, hod, multilib, props...)
}

func (c *ccBase) GenerateAndroidBuildActions(ctx common.AndroidModuleContext) {
	toolchain := c.findToolchain(ctx)
	if ctx.Failed() {
		return
	}

	flags := c.collectFlags(ctx, toolchain)
	if ctx.Failed() {
		return
	}

	depNames := c.module.DepNames(ctx, CCDeps{})
	if ctx.Failed() {
		return
	}

	deps := c.depsToPaths(ctx, depNames)
	if ctx.Failed() {
		return
	}

	flags.IncludeDirs = append(flags.IncludeDirs, deps.IncludeDirs...)

	objFiles := c.compileObjs(ctx, flags, deps)
	if ctx.Failed() {
		return
	}

	generatedObjFiles := c.compileGeneratedObjs(ctx, flags, deps)
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

func (c *ccBase) ccModuleType() CCModuleType {
	return c.module
}

var _ common.AndroidDynamicDepender = (*ccBase)(nil)

func (c *ccBase) findToolchain(ctx common.AndroidModuleContext) Toolchain {
	arch := ctx.Arch()
	factory := toolchainFactories[arch.HostOrDevice][arch.ArchType]
	if factory == nil {
		panic(fmt.Sprintf("Toolchain not found for %s arch %q",
			arch.HostOrDevice.String(), arch.String()))
	}
	return factory(arch.ArchVariant, arch.CpuVariant)
}

func (c *ccBase) DepNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	depNames.WholeStaticLibs = append(depNames.WholeStaticLibs, c.properties.Whole_static_libs...)
	depNames.StaticLibs = append(depNames.StaticLibs, c.properties.Static_libs...)
	depNames.SharedLibs = append(depNames.SharedLibs, c.properties.Shared_libs...)

	stl := c.stl(ctx)
	if ctx.Failed() {
		return depNames
	}

	switch stl {
	case "libc++", "libstdc++":
		depNames.SharedLibs = append(depNames.SharedLibs, stl)
	case "libc++_static":
		depNames.StaticLibs = append(depNames.StaticLibs, stl)
	case "stlport":
		depNames.SharedLibs = append(depNames.SharedLibs, "libstdc++", "libstlport")
	case "stlport_static":
		depNames.StaticLibs = append(depNames.StaticLibs, "libstdc++", "libstlport_static")
	}

	return depNames
}

func (c *ccBase) AndroidDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	depNames := CCDeps{}
	depNames = c.module.DepNames(ctx, depNames)
	staticLibs := depNames.WholeStaticLibs
	staticLibs = append(staticLibs, depNames.StaticLibs...)
	staticLibs = append(staticLibs, depNames.LateStaticLibs...)
	ctx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}}, staticLibs...)

	ctx.AddVariationDependencies([]blueprint.Variation{{"link", "shared"}}, depNames.SharedLibs...)

	ret := append([]string(nil), depNames.ObjFiles...)
	if depNames.CrtBegin != "" {
		ret = append(ret, depNames.CrtBegin)
	}
	if depNames.CrtEnd != "" {
		ret = append(ret, depNames.CrtEnd)
	}

	return ret
}

// Create a ccFlags struct that collects the compile flags from global values,
// per-target values, module type values, and per-module Blueprints properties
func (c *ccBase) collectFlags(ctx common.AndroidModuleContext, toolchain Toolchain) CCFlags {
	flags := CCFlags{
		CFlags:     c.properties.Cflags,
		CppFlags:   c.properties.Cppflags,
		ConlyFlags: c.properties.Conlyflags,
		LdFlags:    c.properties.Ldflags,
		AsFlags:    c.properties.Asflags,
		Nocrt:      c.properties.Nocrt,
		Toolchain:  toolchain,
		Clang:      c.properties.Clang,
	}
	instructionSet := c.properties.Instruction_set
	instructionSetFlags, err := toolchain.InstructionSetFlags(instructionSet)
	if err != nil {
		ctx.ModuleErrorf("%s", err)
	}

	// TODO: debug
	flags.CFlags = append(flags.CFlags, c.properties.Release.Cflags...)

	if ctx.Host() {
		// TODO: allow per-module clang disable for host
		flags.Clang = true
	}

	if flags.Clang {
		flags.CFlags = clangFilterUnknownCflags(flags.CFlags)
		flags.CFlags = append(flags.CFlags, c.properties.Clang_cflags...)
		flags.AsFlags = append(flags.AsFlags, c.properties.Clang_asflags...)
		flags.CppFlags = clangFilterUnknownCflags(flags.CppFlags)
		flags.ConlyFlags = clangFilterUnknownCflags(flags.ConlyFlags)
		flags.LdFlags = clangFilterUnknownCflags(flags.LdFlags)

		flags.CFlags = append(flags.CFlags, "${clangExtraCflags}")
		flags.ConlyFlags = append(flags.ConlyFlags, "${clangExtraConlyflags}")
		if ctx.Device() {
			flags.CFlags = append(flags.CFlags, "${clangExtraTargetCflags}")
		}

		target := "-target " + toolchain.ClangTriple()
		gccPrefix := "-B" + filepath.Join(toolchain.GccRoot(), toolchain.GccTriple(), "bin")

		flags.CFlags = append(flags.CFlags, target, gccPrefix)
		flags.AsFlags = append(flags.AsFlags, target, gccPrefix)
		flags.LdFlags = append(flags.LdFlags, target, gccPrefix)

		if ctx.Host() {
			gccToolchain := "--gcc-toolchain=" + toolchain.GccRoot()
			sysroot := "--sysroot=" + filepath.Join(toolchain.GccRoot(), "sysroot")

			// TODO: also need more -B, -L flags to make host builds hermetic
			flags.CFlags = append(flags.CFlags, gccToolchain, sysroot)
			flags.AsFlags = append(flags.AsFlags, gccToolchain, sysroot)
			flags.LdFlags = append(flags.LdFlags, gccToolchain, sysroot)
		}
	}

	flags.IncludeDirs = pathtools.PrefixPaths(c.properties.Include_dirs, ctx.Config().(Config).SrcDir())
	localIncludeDirs := pathtools.PrefixPaths(c.properties.Local_include_dirs, common.ModuleSrcDir(ctx))
	flags.IncludeDirs = append(flags.IncludeDirs, localIncludeDirs...)

	if !c.properties.No_default_compiler_flags {
		flags.IncludeDirs = append(flags.IncludeDirs, []string{
			common.ModuleSrcDir(ctx),
			common.ModuleOutDir(ctx),
			common.ModuleGenDir(ctx),
		}...)

		if c.properties.Sdk_version == "" {
			flags.IncludeDirs = append(flags.IncludeDirs, "${SrcDir}/libnativehelper/include/nativehelper")
		}

		if ctx.Device() && !c.properties.Allow_undefined_symbols {
			flags.LdFlags = append(flags.LdFlags, "-Wl,--no-undefined")
		}

		if flags.Clang {
			flags.CppFlags = append(flags.CppFlags, "${commonClangGlobalCppflags}")
			flags.GlobalFlags = []string{
				"${commonGlobalIncludes}",
				toolchain.IncludeFlags(),
				instructionSetFlags,
				toolchain.ClangCflags(),
				"${commonClangGlobalCflags}",
				fmt.Sprintf("${%sClangGlobalCflags}", ctx.Arch().HostOrDevice),
			}
		} else {
			flags.CppFlags = append(flags.CppFlags, "${commonGlobalCppflags}")
			flags.GlobalFlags = []string{
				"${commonGlobalIncludes}",
				toolchain.IncludeFlags(),
				instructionSetFlags,
				toolchain.Cflags(),
				"${commonGlobalCflags}",
				fmt.Sprintf("${%sGlobalCflags}", ctx.Arch().HostOrDevice),
			}
		}

		if ctx.Host() {
			flags.LdFlags = append(flags.LdFlags, c.properties.Host_ldlibs...)
		}

		if ctx.Device() {
			if c.properties.Rtti {
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
	}

	flags = c.ccModuleType().Flags(ctx, flags)

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

func (c *ccBase) stl(ctx common.AndroidBaseContext) string {
	switch c.properties.Stl {
	case "libc++", "libc++_static",
		"stlport", "stlport_static",
		"libstdc++":
		return c.properties.Stl
	case "none":
		return ""
	case "":
		return "libc++" // TODO: mingw needs libstdc++
	case "ndk":
		panic("TODO: stl: ndk")
	default:
		ctx.ModuleErrorf("stl: %q is not a supported STL", c.properties.Stl)
		return ""
	}
}

func (c *ccBase) Flags(ctx common.AndroidModuleContext, flags CCFlags) CCFlags {
	stl := c.stl(ctx)
	if ctx.Failed() {
		return flags
	}

	switch stl {
	case "libc++", "libc++_static":
		flags.CFlags = append(flags.CFlags, "-D_USING_LIBCXX")
		flags.IncludeDirs = append(flags.IncludeDirs, "${SrcDir}/external/libcxx/include")
		if ctx.Host() {
			flags.CppFlags = append(flags.CppFlags, "-nostdinc++")
			flags.LdFlags = append(flags.LdFlags, "-nodefaultlibs")
			flags.LdLibs = append(flags.LdLibs, "-lc", "-lm", "-lpthread")
		}
	case "stlport", "stlport_static":
		if ctx.Device() {
			flags.IncludeDirs = append(flags.IncludeDirs,
				"${SrcDir}/external/stlport/stlport",
				"${SrcDir}/bionic/libstdc++/include",
				"${SrcDir}/bionic")
		}
	case "ndk":
		panic("TODO")
	case "libstdc++":
		// Using bionic's basic libstdc++. Not actually an STL. Only around until the
		// tree is in good enough shape to not need it.
		// Host builds will use GNU libstdc++.
		if ctx.Device() {
			flags.IncludeDirs = append(flags.IncludeDirs, "${SrcDir}/bionic/libstdc++/include")
		}
	case "":
		if ctx.Host() {
			flags.CppFlags = append(flags.CppFlags, "-nostdinc++")
			flags.LdFlags = append(flags.LdFlags, "-nodefaultlibs")
			flags.LdLibs = append(flags.LdLibs, "-lc", "-lm")
		}
	default:
		panic(fmt.Errorf("Unknown stl: %q", stl))
	}

	return flags
}

// Compile a list of source files into objects a specified subdirectory
func (c *ccBase) customCompileObjs(ctx common.AndroidModuleContext, flags CCFlags,
	deps CCDeps, subdir string, srcFiles []string) []string {

	srcFiles = pathtools.PrefixPaths(srcFiles, common.ModuleSrcDir(ctx))
	srcFiles = common.ExpandGlobs(ctx, srcFiles)

	return TransformSourceToObj(ctx, subdir, srcFiles, ccFlagsToBuilderFlags(flags))
}

// Compile files listed in c.properties.Srcs into objects
func (c *ccBase) compileObjs(ctx common.AndroidModuleContext, flags CCFlags,
	deps CCDeps) []string {

	if c.properties.SkipCompileObjs {
		return nil
	}

	return c.customCompileObjs(ctx, flags, deps, "", c.properties.Srcs)
}

// Compile generated source files from dependencies
func (c *ccBase) compileGeneratedObjs(ctx common.AndroidModuleContext, flags CCFlags,
	deps CCDeps) []string {
	var srcs []string

	if c.properties.SkipCompileObjs {
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

	return TransformSourceToObj(ctx, "", srcs, ccFlagsToBuilderFlags(flags))
}

func (c *ccBase) outputFile() string {
	return ""
}

func (c *ccBase) depsToPathsFromList(ctx common.AndroidModuleContext,
	names []string) (modules []common.AndroidModule,
	outputFiles []string, exportedIncludeDirs []string) {

	for _, n := range names {
		found := false
		ctx.VisitDirectDeps(func(m blueprint.Module) {
			otherName := ctx.OtherModuleName(m)
			if otherName != n {
				return
			}

			if a, ok := m.(CCModuleType); ok {
				if a.Disabled() {
					// If a cc_library host+device module depends on a library that exists as both
					// cc_library_shared and cc_library_host_shared, it will end up with two
					// dependencies with the same name, one of which is marked disabled for each
					// of host and device.  Ignore the disabled one.
					return
				}
				if a.HostOrDevice() != ctx.Arch().HostOrDevice {
					ctx.ModuleErrorf("host/device mismatch between %q and %q", ctx.ModuleName(),
						otherName)
					return
				}

				if outputFile := a.outputFile(); outputFile != "" {
					if found {
						ctx.ModuleErrorf("multiple modules satisified dependency on %q", otherName)
						return
					}
					outputFiles = append(outputFiles, outputFile)
					modules = append(modules, a)
					if i, ok := a.(ccExportedIncludeDirsProducer); ok {
						exportedIncludeDirs = append(exportedIncludeDirs, i.exportedIncludeDirs()...)
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
		if !found {
			ctx.ModuleErrorf("unsatisified dependency on %q", n)
		}
	}

	return modules, outputFiles, exportedIncludeDirs
}

// Convert depenedency names to paths.  Takes a CCDeps containing names and returns a CCDeps
// containing paths
func (c *ccBase) depsToPaths(ctx common.AndroidModuleContext, depNames CCDeps) CCDeps {
	var depPaths CCDeps
	var newIncludeDirs []string

	var wholeStaticLibModules []common.AndroidModule

	wholeStaticLibModules, depPaths.WholeStaticLibs, newIncludeDirs =
		c.depsToPathsFromList(ctx, depNames.WholeStaticLibs)
	depPaths.IncludeDirs = append(depPaths.IncludeDirs, newIncludeDirs...)

	for _, m := range wholeStaticLibModules {
		if staticLib, ok := m.(ccLibraryInterface); ok && staticLib.static() {
			depPaths.WholeStaticLibObjFiles =
				append(depPaths.WholeStaticLibObjFiles, staticLib.allObjFiles()...)
		} else {
			ctx.ModuleErrorf("module %q not a static library", ctx.OtherModuleName(m))
		}
	}

	_, depPaths.StaticLibs, newIncludeDirs = c.depsToPathsFromList(ctx, depNames.StaticLibs)
	depPaths.IncludeDirs = append(depPaths.IncludeDirs, newIncludeDirs...)

	_, depPaths.LateStaticLibs, newIncludeDirs = c.depsToPathsFromList(ctx, depNames.LateStaticLibs)
	depPaths.IncludeDirs = append(depPaths.IncludeDirs, newIncludeDirs...)

	_, depPaths.SharedLibs, newIncludeDirs = c.depsToPathsFromList(ctx, depNames.SharedLibs)
	depPaths.IncludeDirs = append(depPaths.IncludeDirs, newIncludeDirs...)

	ctx.VisitDirectDeps(func(m blueprint.Module) {
		if obj, ok := m.(*ccObject); ok {
			otherName := ctx.OtherModuleName(m)
			if otherName == depNames.CrtBegin {
				if !c.properties.Nocrt {
					depPaths.CrtBegin = obj.outputFile()
				}
			} else if otherName == depNames.CrtEnd {
				if !c.properties.Nocrt {
					depPaths.CrtEnd = obj.outputFile()
				}
			} else {
				depPaths.ObjFiles = append(depPaths.ObjFiles, obj.outputFile())
			}
		}
	})

	return depPaths
}

// ccDynamic contains the properties and members used by shared libraries and dynamic executables
type ccDynamic struct {
	ccBase
}

func newCCDynamic(dynamic *ccDynamic, module CCModuleType, hod common.HostOrDeviceSupported,
	multilib common.Multilib, props ...interface{}) (blueprint.Module, []interface{}) {

	dynamic.properties.System_shared_libs = []string{defaultSystemSharedLibraries}

	return newCCBase(&dynamic.ccBase, module, hod, multilib, props...)
}

const defaultSystemSharedLibraries = "__default__"

func (c *ccDynamic) systemSharedLibs(ctx common.AndroidBaseContext) []string {
	if len(c.properties.System_shared_libs) == 1 &&
		c.properties.System_shared_libs[0] == defaultSystemSharedLibraries {

		if ctx.Host() {
			return []string{}
		} else {
			return []string{"libc", "libm"}
		}
	}
	return c.properties.System_shared_libs
}

func (c *ccDynamic) DepNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	depNames = c.ccBase.DepNames(ctx, depNames)

	if ctx.Device() {
		depNames.StaticLibs = append(depNames.StaticLibs, "libcompiler_rt-extras")
		// libgcc and libatomic have to be last on the command line
		depNames.LateStaticLibs = append(depNames.LateStaticLibs, "libgcov", "libatomic", "libgcc")
		depNames.SharedLibs = append(depNames.SharedLibs, c.systemSharedLibs(ctx)...)
	}

	return depNames
}

type ccExportedIncludeDirsProducer interface {
	exportedIncludeDirs() []string
}

//
// Combined static+shared libraries
//

type CCLibrary struct {
	ccDynamic

	primary           *CCLibrary
	primaryObjFiles   []string
	objFiles          []string
	exportIncludeDirs []string
	out               string

	LibraryProperties struct {
		BuildStatic bool `blueprint:"mutated"`
		BuildShared bool `blueprint:"mutated"`
		IsShared    bool `blueprint:"mutated"`
		IsStatic    bool `blueprint:"mutated"`

		Static struct {
			Srcs   []string `android:"arch_variant"`
			Cflags []string `android:"arch_variant"`
		} `android:"arch_variant"`
		Shared struct {
			Srcs   []string `android:"arch_variant"`
			Cflags []string `android:"arch_variant"`
		} `android:"arch_variant"`
	}
}

type ccLibraryInterface interface {
	ccLibrary() *CCLibrary
	static() bool
	shared() bool
	allObjFiles() []string
}

func (c *CCLibrary) ccLibrary() *CCLibrary {
	return c
}

func (c *CCLibrary) static() bool {
	return c.LibraryProperties.IsStatic
}

func (c *CCLibrary) shared() bool {
	return c.LibraryProperties.IsShared
}

func NewCCLibrary(library *CCLibrary, module CCModuleType,
	hod common.HostOrDeviceSupported) (blueprint.Module, []interface{}) {

	return newCCDynamic(&library.ccDynamic, module, hod, common.MultilibBoth,
		&library.LibraryProperties)
}

func CCLibraryFactory() (blueprint.Module, []interface{}) {
	module := &CCLibrary{}

	module.LibraryProperties.BuildShared = true
	module.LibraryProperties.BuildStatic = true

	return NewCCLibrary(module, module, common.HostAndDeviceSupported)
}

func (c *CCLibrary) DepNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	if c.shared() {
		depNames = c.ccDynamic.DepNames(ctx, depNames)
		if ctx.Device() {
			depNames.CrtBegin = "crtbegin_so"
			depNames.CrtEnd = "crtend_so"
		}
	} else {
		depNames = c.ccBase.DepNames(ctx, depNames)
	}

	return depNames
}

func (c *CCLibrary) outputFile() string {
	return c.out
}

func (c *CCLibrary) allObjFiles() []string {
	return c.objFiles
}

func (c *CCLibrary) exportedIncludeDirs() []string {
	return c.exportIncludeDirs
}

func (c *CCLibrary) Flags(ctx common.AndroidModuleContext, flags CCFlags) CCFlags {
	flags = c.ccDynamic.Flags(ctx, flags)

	flags.CFlags = append(flags.CFlags, "-fPIC")

	if c.LibraryProperties.IsShared {
		libName := ctx.ModuleName()
		// GCC for Android assumes that -shared means -Bsymbolic, use -Wl,-shared instead
		sharedFlag := "-Wl,-shared"
		if c.properties.Clang || ctx.Host() {
			sharedFlag = "-shared"
		}
		if ctx.Device() {
			flags.LdFlags = append(flags.LdFlags, "-nostdlib")
		}

		flags.LdFlags = append(flags.LdFlags,
			"-Wl,--gc-sections",
			sharedFlag,
			"-Wl,-soname,"+libName+sharedLibraryExtension,
		)
	}

	return flags
}

func (c *CCLibrary) compileStaticLibrary(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCDeps, objFiles []string) {

	staticFlags := flags
	staticFlags.CFlags = append(staticFlags.CFlags, c.LibraryProperties.Static.Cflags...)
	objFilesStatic := c.customCompileObjs(ctx, staticFlags, deps, common.DeviceStaticLibrary,
		c.LibraryProperties.Static.Srcs)

	objFiles = append(objFiles, objFilesStatic...)
	objFiles = append(objFiles, deps.WholeStaticLibObjFiles...)

	outputFile := filepath.Join(common.ModuleOutDir(ctx), ctx.ModuleName()+staticLibraryExtension)

	TransformObjToStaticLib(ctx, objFiles, ccFlagsToBuilderFlags(flags), outputFile)

	c.objFiles = objFiles
	c.out = outputFile
	c.exportIncludeDirs = pathtools.PrefixPaths(c.properties.Export_include_dirs,
		common.ModuleSrcDir(ctx))

	ctx.CheckbuildFile(outputFile)
}

func (c *CCLibrary) compileSharedLibrary(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCDeps, objFiles []string) {

	sharedFlags := flags
	sharedFlags.CFlags = append(sharedFlags.CFlags, c.LibraryProperties.Shared.Cflags...)
	objFilesShared := c.customCompileObjs(ctx, sharedFlags, deps, common.DeviceSharedLibrary,
		c.LibraryProperties.Shared.Srcs)

	objFiles = append(objFiles, objFilesShared...)

	outputFile := filepath.Join(common.ModuleOutDir(ctx), ctx.ModuleName()+sharedLibraryExtension)

	TransformObjToDynamicBinary(ctx, objFiles, deps.SharedLibs, deps.StaticLibs,
		deps.LateStaticLibs, deps.WholeStaticLibs, deps.CrtBegin, deps.CrtEnd,
		ccFlagsToBuilderFlags(flags), outputFile)

	c.out = outputFile
	c.exportIncludeDirs = pathtools.PrefixPaths(c.properties.Export_include_dirs,
		common.ModuleSrcDir(ctx))
}

func (c *CCLibrary) compileModule(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCDeps, objFiles []string) {

	// Reuse the object files from the matching static library if it exists
	if c.primary == c {
		c.primaryObjFiles = objFiles
	} else {
		objFiles = append([]string(nil), c.primary.primaryObjFiles...)
	}

	if c.LibraryProperties.IsStatic {
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

	ctx.InstallFile(installDir, c.out)
}

func (c *CCLibrary) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	if c.LibraryProperties.IsStatic {
		c.installStaticLibrary(ctx, flags)
	} else {
		c.installSharedLibrary(ctx, flags)
	}
}

//
// Objects (for crt*.o)
//

type ccObject struct {
	ccBase
	out string
}

func CCObjectFactory() (blueprint.Module, []interface{}) {
	module := &ccObject{}

	return newCCBase(&module.ccBase, module, common.DeviceSupported, common.MultilibBoth)
}

func (*ccObject) AndroidDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	// object files can't have any dynamic dependencies
	return nil
}

func (*ccObject) DepNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	// object files can't have any dynamic dependencies
	return CCDeps{}
}

func (c *ccObject) compileModule(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCDeps, objFiles []string) {

	objFiles = append(objFiles, deps.ObjFiles...)

	var outputFile string
	if len(objFiles) == 1 {
		outputFile = objFiles[0]
	} else {
		outputFile = filepath.Join(common.ModuleOutDir(ctx), ctx.ModuleName()+".o")
		TransformObjsToObj(ctx, objFiles, ccFlagsToBuilderFlags(flags), outputFile)
	}

	c.out = outputFile

	ctx.CheckbuildFile(outputFile)
}

func (c *ccObject) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	// Object files do not get installed.
}

func (c *ccObject) outputFile() string {
	return c.out
}

//
// Executables
//

type CCBinary struct {
	ccDynamic
	out              string
	BinaryProperties struct {
		// static_executable: compile executable with -static
		Static_executable bool

		// stem: set the name of the output
		Stem string `android:"arch_variant"`

		// prefix_symbols: if set, add an extra objcopy --prefix-symbols= step
		Prefix_symbols string
	}
}

func (c *CCBinary) getStem(ctx common.AndroidModuleContext) string {
	if c.BinaryProperties.Stem != "" {
		return c.BinaryProperties.Stem
	}
	return ctx.ModuleName()
}

func (c *CCBinary) DepNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	depNames = c.ccDynamic.DepNames(ctx, depNames)
	if ctx.Device() {
		if c.BinaryProperties.Static_executable {
			depNames.CrtBegin = "crtbegin_static"
		} else {
			depNames.CrtBegin = "crtbegin_dynamic"
		}
		depNames.CrtEnd = "crtend_android"
	}
	return depNames
}

func NewCCBinary(binary *CCBinary, module CCModuleType,
	hod common.HostOrDeviceSupported) (blueprint.Module, []interface{}) {

	return newCCDynamic(&binary.ccDynamic, module, hod, common.MultilibFirst,
		&binary.BinaryProperties)
}

func CCBinaryFactory() (blueprint.Module, []interface{}) {
	module := &CCBinary{}

	return NewCCBinary(module, module, common.HostAndDeviceSupported)
}

func (c *CCBinary) Flags(ctx common.AndroidModuleContext, flags CCFlags) CCFlags {
	flags = c.ccDynamic.Flags(ctx, flags)

	flags.CFlags = append(flags.CFlags, "-fpie")

	if ctx.Device() {
		linker := "/system/bin/linker"
		if flags.Toolchain.Is64Bit() {
			linker = "/system/bin/linker64"
		}

		flags.LdFlags = append(flags.LdFlags,
			"-nostdlib",
			"-Bdynamic",
			fmt.Sprintf("-Wl,-dynamic-linker,%s", linker),
			"-Wl,--gc-sections",
			"-Wl,-z,nocopyreloc",
		)
	}

	return flags
}

func (c *CCBinary) compileModule(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCDeps, objFiles []string) {

	if !c.BinaryProperties.Static_executable && inList("libc", c.properties.Static_libs) {
		ctx.ModuleErrorf("statically linking libc to dynamic executable, please remove libc\n" +
			"from static libs or set static_executable: true")
	}

	outputFile := filepath.Join(common.ModuleOutDir(ctx), c.getStem(ctx))
	c.out = outputFile

	TransformObjToDynamicBinary(ctx, objFiles, deps.SharedLibs, deps.StaticLibs,
		deps.LateStaticLibs, deps.WholeStaticLibs, deps.CrtBegin, deps.CrtEnd,
		ccFlagsToBuilderFlags(flags), outputFile)
}

func (c *CCBinary) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	ctx.InstallFile("bin", c.out)
}

type ccTest struct {
	CCBinary

	testProperties struct {
		// test_per_src: Create a separate test for each source file.  Useful when there is
		// global state that can not be torn down and reset between each test suite.
		Test_per_src bool
	}
}

func (c *ccTest) Flags(ctx common.AndroidModuleContext, flags CCFlags) CCFlags {
	flags = c.CCBinary.Flags(ctx, flags)

	flags.CFlags = append(flags.CFlags, "-DGTEST_HAS_STD_STRING")
	if ctx.Host() {
		flags.CFlags = append(flags.CFlags, "-O0", "-g")
		flags.LdLibs = append(flags.LdLibs, "-lpthread")
	}

	// TODO(danalbert): Make gtest export its dependencies.
	flags.IncludeDirs = append(flags.IncludeDirs,
		filepath.Join(ctx.Config().(Config).SrcDir(), "external/gtest/include"))

	return flags
}

func (c *ccTest) DepNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	depNames = c.CCBinary.DepNames(ctx, depNames)
	depNames.StaticLibs = append(depNames.StaticLibs, "libgtest", "libgtest_main")
	return depNames
}

func (c *ccTest) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	if ctx.Device() {
		ctx.InstallFile("../data/nativetest/"+ctx.ModuleName(), c.out)
	} else {
		c.CCBinary.installModule(ctx, flags)
	}
}

func CCTestFactory() (blueprint.Module, []interface{}) {
	module := &ccTest{}
	return newCCDynamic(&module.ccDynamic, module, common.HostAndDeviceSupported,
		common.MultilibFirst, &module.BinaryProperties, &module.testProperties)
}

func TestPerSrcMutator(mctx blueprint.EarlyMutatorContext) {
	if test, ok := mctx.Module().(*ccTest); ok {
		if test.testProperties.Test_per_src {
			testNames := make([]string, len(test.properties.Srcs))
			for i, src := range test.properties.Srcs {
				testNames[i] = strings.TrimSuffix(src, filepath.Ext(src))
			}
			tests := mctx.CreateLocalVariations(testNames...)
			for i, src := range test.properties.Srcs {
				tests[i].(*ccTest).properties.Srcs = []string{src}
				tests[i].(*ccTest).BinaryProperties.Stem = testNames[i]
			}
		}
	}
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

	return NewCCBinary(module, module, common.HostSupported)
}

//
// Device libraries shipped with gcc
//

type toolchainLibrary struct {
	CCLibrary
}

func (*toolchainLibrary) AndroidDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	// toolchain libraries can't have any dependencies
	return nil
}

func (*toolchainLibrary) DepNames(ctx common.AndroidBaseContext, depNames CCDeps) CCDeps {
	// toolchain libraries can't have any dependencies
	return CCDeps{}
}

func ToolchainLibraryFactory() (blueprint.Module, []interface{}) {
	module := &toolchainLibrary{}

	module.LibraryProperties.BuildStatic = true

	return newCCBase(&module.ccBase, module, common.DeviceSupported, common.MultilibBoth,
		&module.LibraryProperties)
}

func (c *toolchainLibrary) compileModule(ctx common.AndroidModuleContext,
	flags CCFlags, deps CCDeps, objFiles []string) {

	libName := ctx.ModuleName() + staticLibraryExtension
	outputFile := filepath.Join(common.ModuleOutDir(ctx), libName)

	CopyGccLib(ctx, libName, ccFlagsToBuilderFlags(flags), outputFile)

	c.out = outputFile

	ctx.CheckbuildFile(outputFile)
}

func (c *toolchainLibrary) installModule(ctx common.AndroidModuleContext, flags CCFlags) {
	// Toolchain libraries do not get installed.
}

func LinkageMutator(mctx blueprint.EarlyMutatorContext) {
	if c, ok := mctx.Module().(ccLibraryInterface); ok {
		var modules []blueprint.Module
		if c.ccLibrary().LibraryProperties.BuildStatic && c.ccLibrary().LibraryProperties.BuildShared {
			modules = mctx.CreateLocalVariations("static", "shared")
			modules[0].(ccLibraryInterface).ccLibrary().LibraryProperties.IsStatic = true
			modules[1].(ccLibraryInterface).ccLibrary().LibraryProperties.IsShared = true
		} else if c.ccLibrary().LibraryProperties.BuildStatic {
			modules = mctx.CreateLocalVariations("static")
			modules[0].(ccLibraryInterface).ccLibrary().LibraryProperties.IsStatic = true
		} else if c.ccLibrary().LibraryProperties.BuildShared {
			modules = mctx.CreateLocalVariations("shared")
			modules[0].(ccLibraryInterface).ccLibrary().LibraryProperties.IsShared = true
		} else {
			panic(fmt.Errorf("ccLibrary %q not static or shared", mctx.ModuleName()))
		}
		primary := modules[0].(ccLibraryInterface).ccLibrary()
		for _, m := range modules {
			m.(ccLibraryInterface).ccLibrary().primary = primary
			if m.(ccLibraryInterface).ccLibrary() != primary {
				m.(ccLibraryInterface).ccLibrary().properties.SkipCompileObjs = true
			}
		}
	}
}
