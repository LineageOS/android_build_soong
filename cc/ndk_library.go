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
	"sync"

	"github.com/google/blueprint"

	"android/soong/android"
)

var (
	toolPath = pctx.SourcePathVariable("toolPath", "build/soong/cc/gen_stub_libs.py")

	genStubSrc = pctx.AndroidStaticRule("genStubSrc",
		blueprint.RuleParams{
			Command:     "$toolPath --arch $arch --api $apiLevel $vndk $in $out",
			Description: "genStubSrc $out",
			CommandDeps: []string{"$toolPath"},
		}, "arch", "apiLevel", "vndk")

	ndkLibrarySuffix = ".ndk"

	ndkPrebuiltSharedLibs = []string{
		"android",
		"c",
		"dl",
		"EGL",
		"GLESv1_CM",
		"GLESv2",
		"GLESv3",
		"jnigraphics",
		"log",
		"mediandk",
		"m",
		"OpenMAXAL",
		"OpenSLES",
		"stdc++",
		"vulkan",
		"z",
	}
	ndkPrebuiltSharedLibraries = addPrefix(append([]string(nil), ndkPrebuiltSharedLibs...), "lib")

	// These libraries have migrated over to the new ndk_library, which is added
	// as a variation dependency via depsMutator.
	ndkMigratedLibs     = []string{}
	ndkMigratedLibsLock sync.Mutex // protects ndkMigratedLibs writes during parallel beginMutator
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

	// The first API level that library should have the version script applied.
	// This defaults to the value of first_version, and should almost never be
	// used. This is only needed to work around platform bugs like
	// https://github.com/android-ndk/ndk/issues/265.
	Unversioned_until string

	// Private property for use by the mutator that splits per-API level.
	ApiLevel string `blueprint:"mutated"`
}

type stubDecorator struct {
	*libraryDecorator

	properties libraryProperties

	versionScriptPath android.ModuleGenPath
	installPath       string
}

// OMG GO
func intMax(a int, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

func normalizeNdkApiLevel(apiLevel string, arch android.Arch) (string, error) {
	if apiLevel == "current" {
		return apiLevel, nil
	}

	minVersion := 9 // Minimum version supported by the NDK.
	firstArchVersions := map[string]int{
		"arm":    9,
		"arm64":  21,
		"mips":   9,
		"mips64": 21,
		"x86":    9,
		"x86_64": 21,
	}

	archStr := arch.ArchType.String()
	firstArchVersion, ok := firstArchVersions[archStr]
	if !ok {
		panic(fmt.Errorf("Arch %q not found in firstArchVersions", archStr))
	}

	if apiLevel == "minimum" {
		return strconv.Itoa(firstArchVersion), nil
	}

	// If the NDK drops support for a platform version, we don't want to have to
	// fix up every module that was using it as its SDK version. Clip to the
	// supported version here instead.
	version, err := strconv.Atoi(apiLevel)
	if err != nil {
		return "", fmt.Errorf("API level must be an integer (is %q)", apiLevel)
	}
	version = intMax(version, minVersion)

	return strconv.Itoa(intMax(version, firstArchVersion)), nil
}

func getFirstGeneratedVersion(firstSupportedVersion string, platformVersion int) (int, error) {
	if firstSupportedVersion == "current" {
		return platformVersion + 1, nil
	}

	return strconv.Atoi(firstSupportedVersion)
}

func shouldUseVersionScript(stub *stubDecorator) (bool, error) {
	// unversioned_until is normally empty, in which case we should use the version script.
	if stub.properties.Unversioned_until == "" {
		return true, nil
	}

	if stub.properties.Unversioned_until == "current" {
		if stub.properties.ApiLevel == "current" {
			return true, nil
		} else {
			return false, nil
		}
	}

	if stub.properties.ApiLevel == "current" {
		return true, nil
	}

	unversionedUntil, err := strconv.Atoi(stub.properties.Unversioned_until)
	if err != nil {
		return true, err
	}

	version, err := strconv.Atoi(stub.properties.ApiLevel)
	if err != nil {
		return true, err
	}

	return version >= unversionedUntil, nil
}

func generateStubApiVariants(mctx android.BottomUpMutatorContext, c *stubDecorator) {
	platformVersion := mctx.AConfig().PlatformSdkVersionInt()

	firstSupportedVersion, err := normalizeNdkApiLevel(c.properties.First_version,
		mctx.Arch())
	if err != nil {
		mctx.PropertyErrorf("first_version", err.Error())
	}

	firstGenVersion, err := getFirstGeneratedVersion(firstSupportedVersion, platformVersion)
	if err != nil {
		// In theory this is impossible because we've already run this through
		// normalizeNdkApiLevel above.
		mctx.PropertyErrorf("first_version", err.Error())
	}

	var versionStrs []string
	for version := firstGenVersion; version <= platformVersion; version++ {
		versionStrs = append(versionStrs, strconv.Itoa(version))
	}
	versionStrs = append(versionStrs, "current")

	modules := mctx.CreateVariations(versionStrs...)
	for i, module := range modules {
		module.(*Module).compiler.(*stubDecorator).properties.ApiLevel = versionStrs[i]
	}
}

func ndkApiMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok {
		if compiler, ok := m.compiler.(*stubDecorator); ok {
			generateStubApiVariants(mctx, compiler)
		}
	}
}

func (c *stubDecorator) compilerInit(ctx BaseModuleContext) {
	c.baseCompiler.compilerInit(ctx)

	name := strings.TrimSuffix(ctx.ModuleName(), ".ndk")
	ndkMigratedLibsLock.Lock()
	defer ndkMigratedLibsLock.Unlock()
	for _, lib := range ndkMigratedLibs {
		if lib == name {
			return
		}
	}
	ndkMigratedLibs = append(ndkMigratedLibs, name)
}

func compileStubLibrary(ctx ModuleContext, flags Flags, symbolFile, apiLevel, vndk string) (Objects, android.ModuleGenPath) {
	arch := ctx.Arch().ArchType.String()

	stubSrcPath := android.PathForModuleGen(ctx, "stub.c")
	versionScriptPath := android.PathForModuleGen(ctx, "stub.map")
	symbolFilePath := android.PathForModuleSrc(ctx, symbolFile)
	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:    genStubSrc,
		Outputs: []android.WritablePath{stubSrcPath, versionScriptPath},
		Input:   symbolFilePath,
		Args: map[string]string{
			"arch":     arch,
			"apiLevel": apiLevel,
			"vndk":     vndk,
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
	srcs := []android.Path{stubSrcPath}
	return compileObjs(ctx, flagsToBuilderFlags(flags), subdir, srcs, nil), versionScriptPath
}

func (c *stubDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	if !strings.HasSuffix(ctx.ModuleName(), ndkLibrarySuffix) {
		ctx.ModuleErrorf("ndk_library modules names must be suffixed with %q\n",
			ndkLibrarySuffix)
	}
	objs, versionScript := compileStubLibrary(ctx, flags, c.properties.Symbol_file, c.properties.ApiLevel, "")
	c.versionScriptPath = versionScript
	return objs
}

func (linker *stubDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	return Deps{}
}

func (stub *stubDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	stub.libraryDecorator.libName = strings.TrimSuffix(ctx.ModuleName(),
		ndkLibrarySuffix)
	return stub.libraryDecorator.linkerFlags(ctx, flags)
}

func (stub *stubDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps,
	objs Objects) android.Path {

	useVersionScript, err := shouldUseVersionScript(stub)
	if err != nil {
		ctx.ModuleErrorf(err.Error())
	}

	if useVersionScript {
		linkerScriptFlag := "-Wl,--version-script," + stub.versionScriptPath.String()
		flags.LdFlags = append(flags.LdFlags, linkerScriptFlag)
	}

	return stub.libraryDecorator.link(ctx, flags, deps, objs)
}

func (stub *stubDecorator) install(ctx ModuleContext, path android.Path) {
	arch := ctx.Target().Arch.ArchType.Name
	apiLevel := stub.properties.ApiLevel

	// arm64 isn't actually a multilib toolchain, so unlike the other LP64
	// architectures it's just installed to lib.
	libDir := "lib"
	if ctx.toolchain().Is64Bit() && arch != "arm64" {
		libDir = "lib64"
	}

	installDir := getNdkInstallBase(ctx).Join(ctx, fmt.Sprintf(
		"platforms/android-%s/arch-%s/usr/%s", apiLevel, arch, libDir))
	stub.installPath = ctx.InstallFile(installDir, path).String()
}

func newStubLibrary() (*Module, []interface{}) {
	module, library := NewLibrary(android.DeviceSupported)
	library.BuildOnlyShared()
	module.stl = nil
	module.sanitize = nil
	library.StripProperties.Strip.None = true

	stub := &stubDecorator{
		libraryDecorator: library,
	}
	module.compiler = stub
	module.linker = stub
	module.installer = stub

	return module, []interface{}{&stub.properties, &library.MutatedProperties}
}

func ndkLibraryFactory() (blueprint.Module, []interface{}) {
	module, properties := newStubLibrary()
	return android.InitAndroidArchModule(module, android.DeviceSupported,
		android.MultilibBoth, properties...)
}
