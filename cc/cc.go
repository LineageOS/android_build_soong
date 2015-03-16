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
	"blueprint"
	"blueprint/pathtools"
	"fmt"
	"path/filepath"
	"strings"

	"android/soong/common"
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

// CcProperties describes properties used to compile all C or C++ modules
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
}

type unusedProperties struct {
	Asan            bool
	Native_coverage bool
	Strip           string
	Tags            []string
	Required        []string
}

// Building C/C++ code is handled by objects that satisfy this interface via composition
type ccModuleType interface {
	common.AndroidModule

	// Return the cflags that are specific to this _type_ of module
	moduleTypeCflags(common.AndroidModuleContext, toolchain) []string

	// Return the ldflags that are specific to this _type_ of module
	moduleTypeLdflags(common.AndroidModuleContext, toolchain) []string

	// Create a ccDeps struct that collects the module dependency info.  Can also
	// modify ccFlags in order to add dependency include directories, etc.
	collectDeps(common.AndroidModuleContext, ccFlags) (ccDeps, ccFlags)

	// Compile objects into final module
	compileModule(common.AndroidModuleContext, ccFlags, ccDeps, []string)

	// Return the output file (.o, .a or .so) for use by other modules
	outputFile() string
}

func (c *ccBase) GenerateAndroidBuildActions(ctx common.AndroidModuleContext) {
	toolchain := c.findToolchain(ctx)
	if ctx.Failed() {
		return
	}

	flags := c.flags(ctx, toolchain)
	if ctx.Failed() {
		return
	}

	flags = c.addStlFlags(ctx, flags)
	if ctx.Failed() {
		return
	}

	deps, flags := c.ccModuleType().collectDeps(ctx, flags)
	if ctx.Failed() {
		return
	}

	objFiles := c.compileObjs(ctx, flags, deps)
	if ctx.Failed() {
		return
	}

	c.ccModuleType().compileModule(ctx, flags, deps, objFiles)
	if ctx.Failed() {
		return
	}
}

func (c *ccBase) ccModuleType() ccModuleType {
	return c.module
}

var _ common.AndroidDynamicDepender = (*ccBase)(nil)

func (c *ccBase) findToolchain(ctx common.AndroidModuleContext) toolchain {
	arch := ctx.Arch()
	factory := toolchainFactories[arch.HostOrDevice][arch.ArchType]
	if factory == nil {
		panic(fmt.Sprintf("Toolchain not found for %s arch %q",
			arch.HostOrDevice.String(), arch.String()))
	}
	return factory(arch.ArchVariant, arch.CpuVariant)
}

type ccDeps struct {
	staticLibs, sharedLibs, lateStaticLibs, wholeStaticLibs, objFiles, includeDirs []string

	crtBegin, crtEnd string
}

type ccFlags struct {
	globalFlags []string
	asFlags     []string
	cFlags      []string
	conlyFlags  []string
	cppFlags    []string
	ldFlags     []string
	ldLibs      []string
	includeDirs []string
	nocrt       bool
	toolchain   toolchain
	clang       bool

	extraStaticLibs []string
	extraSharedLibs []string
}

// ccBase contains the properties and members used by all C/C++ module types
type ccBase struct {
	common.AndroidModuleBase
	module ccModuleType

	properties ccProperties
	unused     unusedProperties

	installPath string
}

func (c *ccBase) moduleTypeCflags(ctx common.AndroidModuleContext, toolchain toolchain) []string {
	return nil
}

func (c *ccBase) moduleTypeLdflags(ctx common.AndroidModuleContext, toolchain toolchain) []string {
	return nil
}

func (c *ccBase) AndroidDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	ctx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}}, c.properties.Whole_static_libs...)
	ctx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}}, c.properties.Static_libs...)
	ctx.AddVariationDependencies([]blueprint.Variation{{"link", "shared"}}, c.properties.Shared_libs...)

	return nil
}

// Create a ccFlags struct that collects the compile flags from global values,
// per-target values, module type values, and per-module Blueprints properties
func (c *ccBase) flags(ctx common.AndroidModuleContext, toolchain toolchain) ccFlags {

	arch := ctx.Arch()

	flags := ccFlags{
		cFlags:     c.properties.Cflags,
		cppFlags:   c.properties.Cppflags,
		conlyFlags: c.properties.Conlyflags,
		ldFlags:    c.properties.Ldflags,
		asFlags:    c.properties.Asflags,
		nocrt:      c.properties.Nocrt,
		toolchain:  toolchain,
		clang:      c.properties.Clang,
	}

	if arch.HostOrDevice.Host() {
		// TODO: allow per-module clang disable for host
		flags.clang = true
	}

	if flags.clang {
		flags.cFlags = clangFilterUnknownCflags(flags.cFlags)
		flags.cFlags = append(flags.cFlags, c.properties.Clang_cflags...)
		flags.asFlags = append(flags.asFlags, c.properties.Clang_asflags...)
		flags.cppFlags = clangFilterUnknownCflags(flags.cppFlags)
		flags.conlyFlags = clangFilterUnknownCflags(flags.conlyFlags)
		flags.ldFlags = clangFilterUnknownCflags(flags.ldFlags)

		target := "-target " + toolchain.ClangTriple()
		gccPrefix := "-B" + filepath.Join(toolchain.GccRoot(), toolchain.GccTriple(), "bin")

		flags.cFlags = append(flags.cFlags, target, gccPrefix)
		flags.asFlags = append(flags.asFlags, target, gccPrefix)
		flags.ldFlags = append(flags.ldFlags, target, gccPrefix)

		if arch.HostOrDevice.Host() {
			gccToolchain := "--gcc-toolchain=" + toolchain.GccRoot()
			sysroot := "--sysroot=" + filepath.Join(toolchain.GccRoot(), "sysroot")

			// TODO: also need more -B, -L flags to make host builds hermetic
			flags.cFlags = append(flags.cFlags, gccToolchain, sysroot)
			flags.asFlags = append(flags.asFlags, gccToolchain, sysroot)
			flags.ldFlags = append(flags.ldFlags, gccToolchain, sysroot)
		}
	}

	flags.includeDirs = pathtools.PrefixPaths(c.properties.Include_dirs, ctx.Config().(Config).SrcDir())
	localIncludeDirs := pathtools.PrefixPaths(c.properties.Local_include_dirs, common.ModuleSrcDir(ctx))
	flags.includeDirs = append(flags.includeDirs, localIncludeDirs...)

	if !c.properties.No_default_compiler_flags {
		flags.includeDirs = append(flags.includeDirs, []string{
			common.ModuleSrcDir(ctx),
			common.ModuleOutDir(ctx),
			common.ModuleGenDir(ctx),
		}...)

		if arch.HostOrDevice.Device() && !c.properties.Allow_undefined_symbols {
			flags.ldFlags = append(flags.ldFlags, "-Wl,--no-undefined")
		}

		if flags.clang {
			flags.cppFlags = append(flags.cppFlags, "${commonClangGlobalCppflags}")
			flags.globalFlags = []string{
				"${commonGlobalIncludes}",
				toolchain.IncludeFlags(),
				toolchain.ClangCflags(),
				"${commonClangGlobalCflags}",
				fmt.Sprintf("${%sClangGlobalCflags}", arch.HostOrDevice),
			}
		} else {
			flags.cppFlags = append(flags.cppFlags, "${commonGlobalCppflags}")
			flags.globalFlags = []string{
				"${commonGlobalIncludes}",
				toolchain.IncludeFlags(),
				toolchain.Cflags(),
				"${commonGlobalCflags}",
				fmt.Sprintf("${%sGlobalCflags}", arch.HostOrDevice),
			}
		}

		if arch.HostOrDevice.Host() {
			flags.ldFlags = append(flags.ldFlags, c.properties.Host_ldlibs...)
		}

		if arch.HostOrDevice.Device() {
			if c.properties.Rtti {
				flags.cppFlags = append(flags.cppFlags, "-frtti")
			} else {
				flags.cppFlags = append(flags.cppFlags, "-fno-rtti")
			}
		}

		flags.asFlags = append(flags.asFlags, "-D__ASSEMBLY__")

		if flags.clang {
			flags.cppFlags = append(flags.cppFlags, toolchain.ClangCppflags())
			flags.ldFlags = append(flags.ldFlags, toolchain.ClangLdflags())
		} else {
			flags.cppFlags = append(flags.cppFlags, toolchain.Cppflags())
			flags.ldFlags = append(flags.ldFlags, toolchain.Ldflags())
		}
	}

	flags.cFlags = append(flags.cFlags, c.ccModuleType().moduleTypeCflags(ctx, toolchain)...)
	flags.ldFlags = append(flags.ldFlags, c.ccModuleType().moduleTypeLdflags(ctx, toolchain)...)

	// Optimization to reduce size of build.ninja
	// Replace the long list of flags for each file with a module-local variable
	ctx.Variable(pctx, "cflags", strings.Join(flags.cFlags, " "))
	ctx.Variable(pctx, "cppflags", strings.Join(flags.cppFlags, " "))
	ctx.Variable(pctx, "asflags", strings.Join(flags.asFlags, " "))
	flags.cFlags = []string{"$cflags"}
	flags.cppFlags = []string{"$cppflags"}
	flags.asFlags = []string{"$asflags"}

	return flags
}

// Modify ccFlags structs with STL library info
func (c *ccBase) addStlFlags(ctx common.AndroidModuleContext, flags ccFlags) ccFlags {
	if !c.properties.No_default_compiler_flags {
		arch := ctx.Arch()
		stl := "libc++" // TODO: mingw needs libstdc++
		if c.properties.Stl != "" {
			stl = c.properties.Stl
		}

		stlStatic := false
		if strings.HasSuffix(stl, "_static") {
			stlStatic = true
		}

		switch stl {
		case "libc++", "libc++_static":
			flags.cFlags = append(flags.cFlags, "-D_USING_LIBCXX")
			flags.includeDirs = append(flags.includeDirs, "${SrcDir}/external/libcxx/include")
			if arch.HostOrDevice.Host() {
				flags.cppFlags = append(flags.cppFlags, "-nostdinc++")
				flags.ldFlags = append(flags.ldFlags, "-nodefaultlibs")
				flags.ldLibs = append(flags.ldLibs, "-lc", "-lm", "-lpthread")
			}
			if stlStatic {
				flags.extraStaticLibs = append(flags.extraStaticLibs, "libc++_static")
			} else {
				flags.extraSharedLibs = append(flags.extraSharedLibs, "libc++")
			}
		case "stlport", "stlport_static":
			if arch.HostOrDevice.Device() {
				flags.includeDirs = append(flags.includeDirs,
					"${SrcDir}/external/stlport/stlport",
					"${SrcDir}/bionic/libstdc++/include",
					"${SrcDir}/bionic")
				if stlStatic {
					flags.extraStaticLibs = append(flags.extraStaticLibs, "libstdc++", "libstlport_static")
				} else {
					flags.extraSharedLibs = append(flags.extraSharedLibs, "libstdc++", "libstlport")
				}
			}
		case "ndk":
			panic("TODO")
		case "libstdc++":
			// Using bionic's basic libstdc++. Not actually an STL. Only around until the
			// tree is in good enough shape to not need it.
			// Host builds will use GNU libstdc++.
			if arch.HostOrDevice.Device() {
				flags.includeDirs = append(flags.includeDirs, "${SrcDir}/bionic/libstdc++/include")
				flags.extraSharedLibs = append(flags.extraSharedLibs, "libstdc++")
			}
		case "none":
			if arch.HostOrDevice.Host() {
				flags.cppFlags = append(flags.cppFlags, "-nostdinc++")
				flags.ldFlags = append(flags.ldFlags, "-nodefaultlibs")
				flags.ldLibs = append(flags.ldLibs, "-lc", "-lm")
			}
		default:
			ctx.ModuleErrorf("stl: %q is not a supported STL", stl)
		}

	}
	return flags
}

// Compile a list of source files into objects a specified subdirectory
func (c *ccBase) customCompileObjs(ctx common.AndroidModuleContext, flags ccFlags,
	deps ccDeps, subdir string, srcFiles []string) []string {

	srcFiles = pathtools.PrefixPaths(srcFiles, common.ModuleSrcDir(ctx))
	srcFiles = common.ExpandGlobs(ctx, srcFiles)

	return TransformSourceToObj(ctx, subdir, srcFiles, ccFlagsToBuilderFlags(flags))
}

// Compile files listed in c.properties.Srcs into objects
func (c *ccBase) compileObjs(ctx common.AndroidModuleContext, flags ccFlags,
	deps ccDeps) []string {

	if c.properties.SkipCompileObjs {
		return nil
	}

	return c.customCompileObjs(ctx, flags, deps, "", c.properties.Srcs)
}

func (c *ccBase) outputFile() string {
	return ""
}

func (c *ccBase) collectDepsFromList(ctx common.AndroidModuleContext,
	names []string) (modules []common.AndroidModule,
	outputFiles []string, exportedIncludeDirs []string) {

	for _, n := range names {
		found := false
		ctx.VisitDirectDeps(func(m blueprint.Module) {
			otherName := ctx.OtherModuleName(m)
			if otherName != n {
				return
			}

			if a, ok := m.(ccModuleType); ok {
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

func (c *ccBase) collectDeps(ctx common.AndroidModuleContext, flags ccFlags) (ccDeps, ccFlags) {
	var deps ccDeps
	var newIncludeDirs []string

	wholeStaticLibNames := c.properties.Whole_static_libs
	_, deps.wholeStaticLibs, newIncludeDirs = c.collectDepsFromList(ctx, wholeStaticLibNames)

	deps.includeDirs = append(deps.includeDirs, newIncludeDirs...)

	staticLibNames := c.properties.Static_libs
	staticLibNames = append(staticLibNames, flags.extraStaticLibs...)
	_, deps.staticLibs, newIncludeDirs = c.collectDepsFromList(ctx, staticLibNames)
	deps.includeDirs = append(deps.includeDirs, newIncludeDirs...)

	return deps, flags
}

// ccDynamic contains the properties and members used by shared libraries and dynamic executables
type ccDynamic struct {
	ccBase
}

const defaultSystemSharedLibraries = "__default__"

func (c *ccDynamic) systemSharedLibs() []string {

	if len(c.properties.System_shared_libs) == 1 &&
		c.properties.System_shared_libs[0] == defaultSystemSharedLibraries {

		if c.HostOrDevice().Host() {
			return []string{}
		} else {
			return []string{"libc", "libm"}
		}
	}
	return c.properties.System_shared_libs
}

var (
	stlSharedLibs     = []string{"libc++", "libstlport", "libstdc++"}
	stlSharedHostLibs = []string{"libc++"}
	stlStaticLibs     = []string{"libc++_static", "libstlport_static", "libstdc++"}
	stlStaticHostLibs = []string{"libc++_static"}
)

func (c *ccDynamic) AndroidDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	deps := c.ccBase.AndroidDynamicDependencies(ctx)

	if c.HostOrDevice().Device() {
		ctx.AddVariationDependencies([]blueprint.Variation{{"link", "shared"}}, c.systemSharedLibs()...)
		ctx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}},
			"libcompiler_rt-extras",
			"libgcov",
			"libatomic",
			"libgcc")

		if c.properties.Stl != "none" {
			ctx.AddVariationDependencies([]blueprint.Variation{{"link", "shared"}}, stlSharedLibs...)
			ctx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}}, stlStaticLibs...)
		}
	} else {
		if c.properties.Stl != "none" {
			ctx.AddVariationDependencies([]blueprint.Variation{{"link", "shared"}}, stlSharedHostLibs...)
			ctx.AddVariationDependencies([]blueprint.Variation{{"link", "static"}}, stlStaticHostLibs...)
		}
	}

	return deps
}

func (c *ccDynamic) collectDeps(ctx common.AndroidModuleContext, flags ccFlags) (ccDeps, ccFlags) {
	var newIncludeDirs []string

	deps, flags := c.ccBase.collectDeps(ctx, flags)

	systemSharedLibs := c.systemSharedLibs()
	sharedLibNames := make([]string, 0, len(c.properties.Shared_libs)+len(systemSharedLibs)+
		len(flags.extraSharedLibs))
	sharedLibNames = append(sharedLibNames, c.properties.Shared_libs...)
	sharedLibNames = append(sharedLibNames, systemSharedLibs...)
	sharedLibNames = append(sharedLibNames, flags.extraSharedLibs...)
	_, deps.sharedLibs, newIncludeDirs = c.collectDepsFromList(ctx, sharedLibNames)
	deps.includeDirs = append(deps.includeDirs, newIncludeDirs...)

	if ctx.Arch().HostOrDevice.Device() {
		var staticLibs []string
		staticLibNames := []string{"libcompiler_rt-extras"}
		_, staticLibs, newIncludeDirs = c.collectDepsFromList(ctx, staticLibNames)
		deps.staticLibs = append(deps.staticLibs, staticLibs...)
		deps.includeDirs = append(deps.includeDirs, newIncludeDirs...)

		// libgcc and libatomic have to be last on the command line
		staticLibNames = []string{"libgcov", "libatomic", "libgcc"}
		_, staticLibs, newIncludeDirs = c.collectDepsFromList(ctx, staticLibNames)
		deps.lateStaticLibs = append(deps.lateStaticLibs, staticLibs...)
		deps.includeDirs = append(deps.includeDirs, newIncludeDirs...)
	}

	ctx.VisitDirectDeps(func(m blueprint.Module) {
		if obj, ok := m.(*ccObject); ok {
			otherName := ctx.OtherModuleName(m)
			if strings.HasPrefix(otherName, "crtbegin") {
				if !c.properties.Nocrt {
					deps.crtBegin = obj.outputFile()
				}
			} else if strings.HasPrefix(otherName, "crtend") {
				if !c.properties.Nocrt {
					deps.crtEnd = obj.outputFile()
				}
			} else {
				ctx.ModuleErrorf("object module type only support for crtbegin and crtend, found %q",
					ctx.OtherModuleName(m))
			}
		}
	})

	flags.includeDirs = append(flags.includeDirs, deps.includeDirs...)

	return deps, flags
}

type ccExportedIncludeDirsProducer interface {
	exportedIncludeDirs() []string
}

//
// Combined static+shared libraries
//

type ccLibrary struct {
	ccDynamic

	primary           *ccLibrary
	primaryObjFiles   []string
	objFiles          []string
	exportIncludeDirs []string
	out               string

	libraryProperties struct {
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

func NewCCLibrary() (blueprint.Module, []interface{}) {
	module := &ccLibrary{}
	module.module = module
	module.properties.System_shared_libs = []string{defaultSystemSharedLibraries}
	module.libraryProperties.BuildShared = true
	module.libraryProperties.BuildStatic = true

	return common.InitAndroidModule(module, common.HostAndDeviceSupported, "both",
		&module.properties, &module.unused, &module.libraryProperties)
}

func (c *ccLibrary) AndroidDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	if c.libraryProperties.IsShared {
		deps := c.ccDynamic.AndroidDynamicDependencies(ctx)
		if c.HostOrDevice().Device() {
			deps = append(deps, "crtbegin_so", "crtend_so")
		}
		return deps
	} else {
		return c.ccBase.AndroidDynamicDependencies(ctx)
	}
}

func (c *ccLibrary) collectDeps(ctx common.AndroidModuleContext, flags ccFlags) (ccDeps, ccFlags) {
	if c.libraryProperties.IsStatic {
		deps, flags := c.ccBase.collectDeps(ctx, flags)
		wholeStaticLibNames := c.properties.Whole_static_libs
		wholeStaticLibs, _, _ := c.collectDepsFromList(ctx, wholeStaticLibNames)

		for _, m := range wholeStaticLibs {
			if staticLib, ok := m.(*ccLibrary); ok && staticLib.libraryProperties.IsStatic {
				deps.objFiles = append(deps.objFiles, staticLib.allObjFiles()...)
			} else {
				ctx.ModuleErrorf("module %q not a static library", ctx.OtherModuleName(m))
			}
		}

		return deps, flags
	} else if c.libraryProperties.IsShared {
		return c.ccDynamic.collectDeps(ctx, flags)
	} else {
		panic("Not shared or static")
	}
}

func (c *ccLibrary) outputFile() string {
	return c.out
}

func (c *ccLibrary) allObjFiles() []string {
	return c.objFiles
}

func (c *ccLibrary) exportedIncludeDirs() []string {
	return c.exportIncludeDirs
}

func (c *ccLibrary) moduleTypeCflags(ctx common.AndroidModuleContext, toolchain toolchain) []string {
	return []string{"-fPIC"}
}

func (c *ccLibrary) moduleTypeLdflags(ctx common.AndroidModuleContext, toolchain toolchain) []string {
	if c.libraryProperties.IsShared {
		libName := ctx.ModuleName()
		// GCC for Android assumes that -shared means -Bsymbolic, use -Wl,-shared instead
		sharedFlag := "-Wl,-shared"
		if c.properties.Clang || ctx.Arch().HostOrDevice.Host() {
			sharedFlag = "-shared"
		}
		if ctx.Arch().HostOrDevice.Device() {
			return []string{
				"-nostdlib",
				"-Wl,--gc-sections",
				sharedFlag,
				"-Wl,-soname," + libName,
			}
		} else {
			return []string{
				"-Wl,--gc-sections",
				sharedFlag,
				"-Wl,-soname," + libName,
			}
		}
	} else {
		return nil
	}
}

func (c *ccLibrary) compileStaticLibrary(ctx common.AndroidModuleContext,
	flags ccFlags, deps ccDeps, objFiles []string) {

	staticFlags := flags
	staticFlags.cFlags = append(staticFlags.cFlags, c.libraryProperties.Static.Cflags...)
	objFilesStatic := c.customCompileObjs(ctx, staticFlags, deps, common.DeviceStaticLibrary,
		c.libraryProperties.Static.Srcs)

	objFiles = append(objFiles, objFilesStatic...)

	var includeDirs []string

	wholeStaticLibNames := c.properties.Whole_static_libs
	wholeStaticLibs, _, newIncludeDirs := c.collectDepsFromList(ctx, wholeStaticLibNames)
	includeDirs = append(includeDirs, newIncludeDirs...)

	for _, m := range wholeStaticLibs {
		if staticLib, ok := m.(*ccLibrary); ok && staticLib.libraryProperties.IsStatic {
			objFiles = append(objFiles, staticLib.allObjFiles()...)
		} else {
			ctx.ModuleErrorf("module %q not a static library", ctx.OtherModuleName(m))
		}
	}

	staticLibNames := c.properties.Static_libs
	_, _, newIncludeDirs = c.collectDepsFromList(ctx, staticLibNames)
	includeDirs = append(includeDirs, newIncludeDirs...)

	ctx.VisitDirectDeps(func(m blueprint.Module) {
		if obj, ok := m.(*ccObject); ok {
			otherName := ctx.OtherModuleName(m)
			if !strings.HasPrefix(otherName, "crtbegin") && !strings.HasPrefix(otherName, "crtend") {
				objFiles = append(objFiles, obj.outputFile())
			}
		}
	})

	outputFile := filepath.Join(common.ModuleOutDir(ctx), ctx.ModuleName()+staticLibraryExtension)

	TransformObjToStaticLib(ctx, objFiles, ccFlagsToBuilderFlags(flags), outputFile)

	c.objFiles = objFiles
	c.out = outputFile
	c.exportIncludeDirs = pathtools.PrefixPaths(c.properties.Export_include_dirs,
		common.ModuleSrcDir(ctx))

	ctx.CheckbuildFile(outputFile)
}

func (c *ccLibrary) compileSharedLibrary(ctx common.AndroidModuleContext,
	flags ccFlags, deps ccDeps, objFiles []string) {

	sharedFlags := flags
	sharedFlags.cFlags = append(sharedFlags.cFlags, c.libraryProperties.Shared.Cflags...)
	objFilesShared := c.customCompileObjs(ctx, sharedFlags, deps, common.DeviceSharedLibrary,
		c.libraryProperties.Shared.Srcs)

	objFiles = append(objFiles, objFilesShared...)

	outputFile := filepath.Join(common.ModuleOutDir(ctx), ctx.ModuleName()+sharedLibraryExtension)

	TransformObjToDynamicBinary(ctx, objFiles, deps.sharedLibs, deps.staticLibs,
		deps.lateStaticLibs, deps.wholeStaticLibs, deps.crtBegin, deps.crtEnd,
		ccFlagsToBuilderFlags(flags), outputFile)

	c.out = outputFile
	c.exportIncludeDirs = pathtools.PrefixPaths(c.properties.Export_include_dirs,
		common.ModuleSrcDir(ctx))

	installDir := "lib"
	if flags.toolchain.Is64Bit() {
		installDir = "lib64"
	}

	ctx.InstallFile(installDir, outputFile)
}

func (c *ccLibrary) compileModule(ctx common.AndroidModuleContext,
	flags ccFlags, deps ccDeps, objFiles []string) {

	// Reuse the object files from the matching static library if it exists
	if c.primary == c {
		c.primaryObjFiles = objFiles
	} else {
		objFiles = append([]string(nil), c.primary.primaryObjFiles...)
	}

	if c.libraryProperties.IsStatic {
		c.compileStaticLibrary(ctx, flags, deps, objFiles)
	} else {
		c.compileSharedLibrary(ctx, flags, deps, objFiles)
	}
}

//
// Objects (for crt*.o)
//

type ccObject struct {
	ccBase
	out string
}

func NewCCObject() (blueprint.Module, []interface{}) {
	module := &ccObject{}
	module.module = module

	return common.InitAndroidModule(module, common.DeviceSupported, "both",
		&module.properties, &module.unused)
}

func (*ccObject) AndroidDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	// object files can't have any dynamic dependencies
	return nil
}

func (c *ccObject) collectDeps(ctx common.AndroidModuleContext, flags ccFlags) (ccDeps, ccFlags) {
	deps, flags := c.ccBase.collectDeps(ctx, flags)
	ctx.VisitDirectDeps(func(m blueprint.Module) {
		if obj, ok := m.(*ccObject); ok {
			deps.objFiles = append(deps.objFiles, obj.outputFile())
		} else {
			ctx.ModuleErrorf("Unknown module type for dependency %q", ctx.OtherModuleName(m))
		}
	})

	return deps, flags
}

func (c *ccObject) compileModule(ctx common.AndroidModuleContext,
	flags ccFlags, deps ccDeps, objFiles []string) {

	objFiles = append(objFiles, deps.objFiles...)

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

func (c *ccObject) outputFile() string {
	return c.out
}

//
// Executables
//

type ccBinary struct {
	ccDynamic
	binaryProperties binaryProperties
}

type binaryProperties struct {
	// static_executable: compile executable with -static
	Static_executable bool

	// stem: set the name of the output
	Stem string `android:"arch_variant"`

	// prefix_symbols: if set, add an extra objcopy --prefix-symbols= step
	Prefix_symbols string
}

func (c *ccBinary) getStem(ctx common.AndroidModuleContext) string {
	if c.binaryProperties.Stem != "" {
		return c.binaryProperties.Stem
	}
	return ctx.ModuleName()
}

func (c *ccBinary) AndroidDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	deps := c.ccDynamic.AndroidDynamicDependencies(ctx)
	if c.HostOrDevice().Device() {
		if c.binaryProperties.Static_executable {
			deps = append(deps, "crtbegin_static", "crtend_android")
		} else {
			deps = append(deps, "crtbegin_dynamic", "crtend_android")
		}
	}
	return deps
}

func NewCCBinary() (blueprint.Module, []interface{}) {
	module := &ccBinary{}
	module.module = module

	module.properties.System_shared_libs = []string{defaultSystemSharedLibraries}
	return common.InitAndroidModule(module, common.HostAndDeviceSupported, "first", &module.properties,
		&module.unused, &module.binaryProperties)
}

func (c *ccBinary) moduleTypeCflags(ctx common.AndroidModuleContext, toolchain toolchain) []string {
	return []string{"-fpie"}
}

func (c *ccBinary) moduleTypeLdflags(ctx common.AndroidModuleContext, toolchain toolchain) []string {
	if ctx.Arch().HostOrDevice.Device() {
		linker := "/system/bin/linker"
		if toolchain.Is64Bit() {
			linker = "/system/bin/linker64"
		}

		return []string{
			"-nostdlib",
			"-Bdynamic",
			fmt.Sprintf("-Wl,-dynamic-linker,%s", linker),
			"-Wl,--gc-sections",
			"-Wl,-z,nocopyreloc",
		}
	}

	return nil
}

func (c *ccBinary) compileModule(ctx common.AndroidModuleContext,
	flags ccFlags, deps ccDeps, objFiles []string) {

	if !c.binaryProperties.Static_executable && inList("libc", c.properties.Static_libs) {
		ctx.ModuleErrorf("statically linking libc to dynamic executable, please remove libc\n" +
			"from static libs or set static_executable: true")
	}

	outputFile := filepath.Join(common.ModuleOutDir(ctx), c.getStem(ctx))

	TransformObjToDynamicBinary(ctx, objFiles, deps.sharedLibs, deps.staticLibs,
		deps.lateStaticLibs, deps.wholeStaticLibs, deps.crtBegin, deps.crtEnd,
		ccFlagsToBuilderFlags(flags), outputFile)

	ctx.InstallFile("bin", outputFile)
}

//
// Static library
//

func NewCCLibraryStatic() (blueprint.Module, []interface{}) {
	module := &ccLibrary{}
	module.module = module
	module.libraryProperties.BuildStatic = true

	return common.InitAndroidModule(module, common.HostAndDeviceSupported, "both",
		&module.properties, &module.unused)
}

//
// Shared libraries
//

func NewCCLibraryShared() (blueprint.Module, []interface{}) {
	module := &ccLibrary{}
	module.module = module
	module.properties.System_shared_libs = []string{defaultSystemSharedLibraries}
	module.libraryProperties.BuildShared = true

	return common.InitAndroidModule(module, common.HostAndDeviceSupported, "both",
		&module.properties, &module.unused)
}

//
// Host static library
//

func NewCCLibraryHostStatic() (blueprint.Module, []interface{}) {
	module := &ccLibrary{}
	module.module = module
	module.libraryProperties.BuildStatic = true

	return common.InitAndroidModule(module, common.HostSupported, "both",
		&module.properties, &module.unused)
}

//
// Host Shared libraries
//

func NewCCLibraryHostShared() (blueprint.Module, []interface{}) {
	module := &ccLibrary{}
	module.module = module
	module.libraryProperties.BuildShared = true

	return common.InitAndroidModule(module, common.HostSupported, "both",
		&module.properties, &module.unused)
}

//
// Host Binaries
//

func NewCCBinaryHost() (blueprint.Module, []interface{}) {
	module := &ccBinary{}
	module.module = module

	return common.InitAndroidModule(module, common.HostSupported, "first",
		&module.properties, &module.unused)
}

//
// Device libraries shipped with gcc
//

type toolchainLibrary struct {
	ccLibrary
}

func (*toolchainLibrary) AndroidDynamicDependencies(ctx common.AndroidDynamicDependerModuleContext) []string {
	// toolchain libraries can't have any dependencies
	return nil
}

func (*toolchainLibrary) collectDeps(ctx common.AndroidModuleContext, flags ccFlags) (ccDeps, ccFlags) {
	// toolchain libraries can't have any dependencies
	return ccDeps{}, flags
}

func NewToolchainLibrary() (blueprint.Module, []interface{}) {
	module := &toolchainLibrary{}
	module.module = module
	module.libraryProperties.BuildStatic = true

	return common.InitAndroidModule(module, common.DeviceSupported, "both")
}

func (c *toolchainLibrary) compileModule(ctx common.AndroidModuleContext,
	flags ccFlags, deps ccDeps, objFiles []string) {

	libName := ctx.ModuleName() + staticLibraryExtension
	outputFile := filepath.Join(common.ModuleOutDir(ctx), libName)

	CopyGccLib(ctx, libName, ccFlagsToBuilderFlags(flags), outputFile)

	c.out = outputFile

	ctx.CheckbuildFile(outputFile)
}

func LinkageMutator(mctx blueprint.EarlyMutatorContext) {
	if c, ok := mctx.Module().(*ccLibrary); ok {
		var modules []blueprint.Module
		if c.libraryProperties.BuildStatic && c.libraryProperties.BuildShared {
			modules = mctx.CreateLocalVariations("static", "shared")
			modules[0].(*ccLibrary).libraryProperties.IsStatic = true
			modules[1].(*ccLibrary).libraryProperties.IsShared = true
		} else if c.libraryProperties.BuildStatic {
			modules = mctx.CreateLocalVariations("static")
			modules[0].(*ccLibrary).libraryProperties.IsStatic = true
		} else if c.libraryProperties.BuildShared {
			modules = mctx.CreateLocalVariations("shared")
			modules[0].(*ccLibrary).libraryProperties.IsShared = true
		} else {
			panic("ccLibrary not static or shared")
		}
		primary := modules[0].(*ccLibrary)
		for _, m := range modules {
			m.(*ccLibrary).primary = primary
			if m != primary {
				m.(*ccLibrary).properties.SkipCompileObjs = true
			}
		}
	} else if _, ok := mctx.Module().(*toolchainLibrary); ok {
		mctx.CreateLocalVariations("static")
	}
}
