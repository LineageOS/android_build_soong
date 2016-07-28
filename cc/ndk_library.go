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
	"strconv"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

var (
	toolPath = pctx.SourcePathVariable("toolPath", "build/soong/cc/gen_stub_libs.py")

	genStubSrc = pctx.StaticRule("genStubSrc",
		blueprint.RuleParams{
			Command:     "$toolPath --arch $arch --api $apiLevel $in $out",
			Description: "genStubSrc $out",
			CommandDeps: []string{"$toolPath"},
		}, "arch", "apiLevel")

	ndkLibrarySuffix = ".ndk"
)

// Creates a stub shared library based on the provided version file.
//
// The name of the generated file will be based on the module name by stripping
// the ".ndk" suffix from the module name. Module names must end with ".ndk"
// (as a convention to allow soong to guess the NDK name of a dependency when
// needed). "libfoo.ndk" will generate "libfoo.so.
//
// Example:
//
// ndk_library {
//     name: "libfoo.ndk",
//     symbol_file: "libfoo.map.txt",
//     first_version: "9",
// }
//
type libraryProperties struct {
	// Relative path to the symbol map.
	// An example file can be seen here: TODO(danalbert): Make an example.
	Symbol_file string

	// The first API level a library was available. A library will be generated
	// for every API level beginning with this one.
	First_version string

	// Private property for use by the mutator that splits per-API level.
	ApiLevel int `blueprint:"mutated"`
}

type stubCompiler struct {
	baseCompiler

	properties libraryProperties
}

// OMG GO
func intMin(a int, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func generateStubApiVariants(mctx android.BottomUpMutatorContext, c *stubCompiler) {
	minVersion := 9 // Minimum version supported by the NDK.
	// TODO(danalbert): Use PlatformSdkVersion when possible.
	// This is an interesting case because for the moment we actually need 24
	// even though the latest released version in aosp is 23. prebuilts/ndk/r11
	// has android-24 versions of libraries, and as platform libraries get
	// migrated the libraries in prebuilts will need to depend on them.
	//
	// Once everything is all moved over to the new stuff (when there isn't a
	// prebuilts/ndk any more) then this should be fixable, but for now I think
	// it needs to remain as-is.
	maxVersion := 24
	firstArchVersions := map[string]int{
		"arm":    9,
		"arm64":  21,
		"mips":   9,
		"mips64": 21,
		"x86":    9,
		"x86_64": 21,
	}

	// If the NDK drops support for a platform version, we don't want to have to
	// fix up every module that was using it as its minimum version. Clip to the
	// supported version here instead.
	firstVersion, err := strconv.Atoi(c.properties.First_version)
	if err != nil {
		mctx.ModuleErrorf("Invalid first_version value (must be int): %q",
			c.properties.First_version)
	}
	if firstVersion < minVersion {
		firstVersion = minVersion
	}

	arch := mctx.Arch().ArchType.String()
	firstArchVersion, ok := firstArchVersions[arch]
	if !ok {
		panic(fmt.Errorf("Arch %q not found in firstArchVersions", arch))
	}
	firstGenVersion := intMin(firstVersion, firstArchVersion)
	versionStrs := make([]string, maxVersion-firstGenVersion+1)
	for version := firstGenVersion; version <= maxVersion; version++ {
		versionStrs[version-firstGenVersion] = strconv.Itoa(version)
	}

	modules := mctx.CreateVariations(versionStrs...)
	for i, module := range modules {
		module.(*Module).compiler.(*stubCompiler).properties.ApiLevel = firstGenVersion + i
	}
}

func ndkApiMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok {
		if compiler, ok := m.compiler.(*stubCompiler); ok {
			generateStubApiVariants(mctx, compiler)
		}
	}
}

func (c *stubCompiler) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Paths {
	arch := ctx.Arch().ArchType.String()

	if !strings.HasSuffix(ctx.ModuleName(), ndkLibrarySuffix) {
		ctx.ModuleErrorf("ndk_library modules names must be suffixed with %q\n",
			ndkLibrarySuffix)
	}
	libName := strings.TrimSuffix(ctx.ModuleName(), ndkLibrarySuffix)
	fileBase := fmt.Sprintf("%s.%s.%d", libName, arch, c.properties.ApiLevel)
	stubSrcName := fileBase + ".c"
	stubSrcPath := android.PathForModuleGen(ctx, stubSrcName)
	versionScriptName := fileBase + ".map"
	versionScriptPath := android.PathForModuleGen(ctx, versionScriptName)
	symbolFilePath := android.PathForModuleSrc(ctx, c.properties.Symbol_file)
	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:    genStubSrc,
		Outputs: []android.WritablePath{stubSrcPath, versionScriptPath},
		Input:   symbolFilePath,
		Args: map[string]string{
			"arch":     arch,
			"apiLevel": strconv.Itoa(c.properties.ApiLevel),
		},
	})

	flags.CFlags = append(flags.CFlags,
		// We're knowingly doing some otherwise unsightly things with builtin
		// functions here. We're just generating stub libraries, so ignore it.
		"-Wno-incompatible-library-redeclaration",
		"-Wno-builtin-requires-header",
		"-Wno-invalid-noreturn",

		// These libraries aren't actually used. Don't worry about unwinding
		// (avoids the need to link an unwinder into a fake library).
		"-fno-unwind-tables",
	)

	subdir := ""
	srcs := []string{}
	excludeSrcs := []string{}
	extraSrcs := []android.Path{stubSrcPath}
	extraDeps := []android.Path{}
	return c.baseCompiler.compileObjs(ctx, flags, subdir, srcs, excludeSrcs,
		extraSrcs, extraDeps)
}

type stubLinker struct {
	libraryLinker
}

func (linker *stubLinker) deps(ctx BaseModuleContext, deps Deps) Deps {
	return Deps{}
}

func (linker *stubLinker) flags(ctx ModuleContext, flags Flags) Flags {
	linker.libraryLinker.libName = strings.TrimSuffix(ctx.ModuleName(),
		ndkLibrarySuffix)
	return linker.libraryLinker.flags(ctx, flags)
}

type stubInstaller struct {
	baseInstaller

	compiler *stubCompiler

	installPath string
}

var _ installer = (*stubInstaller)(nil)

func (installer *stubInstaller) install(ctx ModuleContext, path android.Path) {
	arch := ctx.Target().Arch.ArchType.Name
	apiLevel := installer.compiler.properties.ApiLevel

	// arm64 isn't actually a multilib toolchain, so unlike the other LP64
	// architectures it's just installed to lib.
	libDir := "lib"
	if ctx.toolchain().Is64Bit() && arch != "arm64" {
		libDir = "lib64"
	}

	installDir := getNdkInstallBase(ctx).Join(ctx, fmt.Sprintf(
		"platforms/android-%d/arch-%s/usr/%s", apiLevel, arch, libDir))
	installer.installPath = ctx.InstallFile(installDir, path).String()
}

func newStubLibrary() *Module {
	module := newModule(android.DeviceSupported, android.MultilibBoth)
	module.stl = nil

	linker := &stubLinker{}
	linker.dynamicProperties.BuildShared = true
	linker.dynamicProperties.BuildStatic = false
	linker.stripper.StripProperties.Strip.None = true
	module.linker = linker

	compiler := &stubCompiler{}
	module.compiler = compiler
	module.installer = &stubInstaller{baseInstaller{
		dir:   "lib",
		dir64: "lib64",
	}, compiler, ""}

	return module
}

func ndkLibraryFactory() (blueprint.Module, []interface{}) {
	module := newStubLibrary()
	return android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibBoth,
		&module.compiler.(*stubCompiler).properties)
}
