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
	"runtime"
	"strings"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
)

func init() {
	pctx.HostBinToolVariable("ndkStubGenerator", "ndkstubgen")
	pctx.HostBinToolVariable("stg", "stg")
	pctx.HostBinToolVariable("stgdiff", "stgdiff")
}

var (
	genStubSrc = pctx.AndroidStaticRule("genStubSrc",
		blueprint.RuleParams{
			Command: "$ndkStubGenerator --arch $arch --api $apiLevel " +
				"--api-map $apiMap $flags $in $out",
			CommandDeps: []string{"$ndkStubGenerator"},
		}, "arch", "apiLevel", "apiMap", "flags")

	// $headersList should include paths to public headers. All types
	// that are defined outside of public headers will be excluded from
	// ABI monitoring.
	//
	// STG tool doesn't access content of files listed in $headersList,
	// so there is no need to add them to dependencies.
	stg = pctx.AndroidStaticRule("stg",
		blueprint.RuleParams{
			Command:     "$stg -S :$symbolList --file-filter :$headersList --elf $in -o $out",
			CommandDeps: []string{"$stg"},
		}, "symbolList", "headersList")

	stgdiff = pctx.AndroidStaticRule("stgdiff",
		blueprint.RuleParams{
			// Need to create *some* output for ninja. We don't want to use tee
			// because we don't want to spam the build output with "nothing
			// changed" messages, so redirect output message to $out, and if
			// changes were detected print the output and fail.
			Command:     "$stgdiff $args --stg $in -o $out || (cat $out && echo 'Run $$ANDROID_BUILD_TOP/development/tools/ndk/update_ndk_abi.sh to update the ABI dumps.' && false)",
			CommandDeps: []string{"$stgdiff"},
		}, "args")

	ndkLibrarySuffix = ".ndk"

	ndkKnownLibsKey = android.NewOnceKey("ndkKnownLibsKey")
	// protects ndkKnownLibs writes during parallel BeginMutator.
	ndkKnownLibsLock sync.Mutex

	stubImplementation = dependencyTag{name: "stubImplementation"}
)

// The First_version and Unversioned_until properties of this struct should not
// be used directly, but rather through the ApiLevel returning methods
// firstVersion() and unversionedUntil().

// Creates a stub shared library based on the provided version file.
//
// Example:
//
// ndk_library {
//
//	name: "libfoo",
//	symbol_file: "libfoo.map.txt",
//	first_version: "9",
//
// }
type libraryProperties struct {
	// Relative path to the symbol map.
	// An example file can be seen here: TODO(danalbert): Make an example.
	Symbol_file *string `android:"path"`

	// The first API level a library was available. A library will be generated
	// for every API level beginning with this one.
	First_version *string

	// The first API level that library should have the version script applied.
	// This defaults to the value of first_version, and should almost never be
	// used. This is only needed to work around platform bugs like
	// https://github.com/android-ndk/ndk/issues/265.
	Unversioned_until *string

	// Headers presented by this library to the Public API Surface
	Export_header_libs []string
}

type stubDecorator struct {
	*libraryDecorator

	properties libraryProperties

	versionScriptPath     android.ModuleGenPath
	parsedCoverageXmlPath android.ModuleOutPath
	installPath           android.Path
	abiDumpPath           android.OutputPath
	abiDiffPaths          android.Paths

	apiLevel         android.ApiLevel
	firstVersion     android.ApiLevel
	unversionedUntil android.ApiLevel
}

var _ versionedInterface = (*stubDecorator)(nil)

func shouldUseVersionScript(ctx BaseModuleContext, stub *stubDecorator) bool {
	return stub.apiLevel.GreaterThanOrEqualTo(stub.unversionedUntil)
}

func (stub *stubDecorator) implementationModuleName(name string) string {
	return strings.TrimSuffix(name, ndkLibrarySuffix)
}

func ndkLibraryVersions(ctx android.BaseMutatorContext, from android.ApiLevel) []string {
	var versions []android.ApiLevel
	versionStrs := []string{}
	for _, version := range ctx.Config().AllSupportedApiLevels() {
		if version.GreaterThanOrEqualTo(from) {
			versions = append(versions, version)
			versionStrs = append(versionStrs, version.String())
		}
	}
	versionStrs = append(versionStrs, android.FutureApiLevel.String())

	return versionStrs
}

func (this *stubDecorator) stubsVersions(ctx android.BaseMutatorContext) []string {
	if !ctx.Module().Enabled() {
		return nil
	}
	if ctx.Target().NativeBridge == android.NativeBridgeEnabled {
		ctx.Module().Disable()
		return nil
	}
	firstVersion, err := nativeApiLevelFromUser(ctx,
		String(this.properties.First_version))
	if err != nil {
		ctx.PropertyErrorf("first_version", err.Error())
		return nil
	}
	return ndkLibraryVersions(ctx, firstVersion)
}

func (this *stubDecorator) initializeProperties(ctx BaseModuleContext) bool {
	this.apiLevel = nativeApiLevelOrPanic(ctx, this.stubsVersion())

	var err error
	this.firstVersion, err = nativeApiLevelFromUser(ctx,
		String(this.properties.First_version))
	if err != nil {
		ctx.PropertyErrorf("first_version", err.Error())
		return false
	}

	str := proptools.StringDefault(this.properties.Unversioned_until, "minimum")
	this.unversionedUntil, err = nativeApiLevelFromUser(ctx, str)
	if err != nil {
		ctx.PropertyErrorf("unversioned_until", err.Error())
		return false
	}

	return true
}

func getNDKKnownLibs(config android.Config) *[]string {
	return config.Once(ndkKnownLibsKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func (c *stubDecorator) compilerInit(ctx BaseModuleContext) {
	c.baseCompiler.compilerInit(ctx)

	name := ctx.baseModuleName()
	if strings.HasSuffix(name, ndkLibrarySuffix) {
		ctx.PropertyErrorf("name", "Do not append %q manually, just use the base name", ndkLibrarySuffix)
	}

	ndkKnownLibsLock.Lock()
	defer ndkKnownLibsLock.Unlock()
	ndkKnownLibs := getNDKKnownLibs(ctx.Config())
	for _, lib := range *ndkKnownLibs {
		if lib == name {
			return
		}
	}
	*ndkKnownLibs = append(*ndkKnownLibs, name)
}

var stubLibraryCompilerFlags = []string{
	// We're knowingly doing some otherwise unsightly things with builtin
	// functions here. We're just generating stub libraries, so ignore it.
	"-Wno-incompatible-library-redeclaration",
	"-Wno-incomplete-setjmp-declaration",
	"-Wno-builtin-requires-header",
	"-Wno-invalid-noreturn",
	"-Wall",
	"-Werror",
	// These libraries aren't actually used. Don't worry about unwinding
	// (avoids the need to link an unwinder into a fake library).
	"-fno-unwind-tables",
}

func init() {
	config.ExportStringList("StubLibraryCompilerFlags", stubLibraryCompilerFlags)
}

func addStubLibraryCompilerFlags(flags Flags) Flags {
	flags.Global.CFlags = append(flags.Global.CFlags, stubLibraryCompilerFlags...)
	// All symbols in the stubs library should be visible.
	if inList("-fvisibility=hidden", flags.Local.CFlags) {
		flags.Local.CFlags = append(flags.Local.CFlags, "-fvisibility=default")
	}
	return flags
}

func (stub *stubDecorator) compilerFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags {
	flags = stub.baseCompiler.compilerFlags(ctx, flags, deps)
	return addStubLibraryCompilerFlags(flags)
}

type ndkApiOutputs struct {
	stubSrc       android.ModuleGenPath
	versionScript android.ModuleGenPath
	symbolList    android.ModuleGenPath
}

func parseNativeAbiDefinition(ctx ModuleContext, symbolFile string,
	apiLevel android.ApiLevel, genstubFlags string) ndkApiOutputs {

	stubSrcPath := android.PathForModuleGen(ctx, "stub.c")
	versionScriptPath := android.PathForModuleGen(ctx, "stub.map")
	symbolFilePath := android.PathForModuleSrc(ctx, symbolFile)
	symbolListPath := android.PathForModuleGen(ctx, "abi_symbol_list.txt")
	apiLevelsJson := android.GetApiLevelsJson(ctx)
	ctx.Build(pctx, android.BuildParams{
		Rule:        genStubSrc,
		Description: "generate stubs " + symbolFilePath.Rel(),
		Outputs: []android.WritablePath{stubSrcPath, versionScriptPath,
			symbolListPath},
		Input:     symbolFilePath,
		Implicits: []android.Path{apiLevelsJson},
		Args: map[string]string{
			"arch":     ctx.Arch().ArchType.String(),
			"apiLevel": apiLevel.String(),
			"apiMap":   apiLevelsJson.String(),
			"flags":    genstubFlags,
		},
	})

	return ndkApiOutputs{
		stubSrc:       stubSrcPath,
		versionScript: versionScriptPath,
		symbolList:    symbolListPath,
	}
}

func compileStubLibrary(ctx ModuleContext, flags Flags, src android.Path) Objects {
	// libc/libm stubs libraries end up mismatching with clang's internal definition of these
	// functions (which have noreturn attributes and other things). Because we just want to create a
	// stub with symbol definitions, and types aren't important in C, ignore the mismatch.
	flags.Local.ConlyFlags = append(flags.Local.ConlyFlags, "-fno-builtin")
	return compileObjs(ctx, flagsToBuilderFlags(flags), "",
		android.Paths{src}, nil, nil, nil, nil)
}

func (this *stubDecorator) findImplementationLibrary(ctx ModuleContext) android.Path {
	dep := ctx.GetDirectDepWithTag(strings.TrimSuffix(ctx.ModuleName(), ndkLibrarySuffix),
		stubImplementation)
	if dep == nil {
		ctx.ModuleErrorf("Could not find implementation for stub")
		return nil
	}
	impl, ok := dep.(*Module)
	if !ok {
		ctx.ModuleErrorf("Implementation for stub is not correct module type")
		return nil
	}
	output := impl.UnstrippedOutputFile()
	if output == nil {
		ctx.ModuleErrorf("implementation module (%s) has no output", impl)
		return nil
	}

	return output
}

func (this *stubDecorator) libraryName(ctx ModuleContext) string {
	return strings.TrimSuffix(ctx.ModuleName(), ndkLibrarySuffix)
}

func (this *stubDecorator) findPrebuiltAbiDump(ctx ModuleContext,
	apiLevel android.ApiLevel) android.OptionalPath {

	subpath := filepath.Join("prebuilts/abi-dumps/ndk", apiLevel.String(),
		ctx.Arch().ArchType.String(), this.libraryName(ctx), "abi.stg")
	return android.ExistentPathForSource(ctx, subpath)
}

// Feature flag.
func canDumpAbi(config android.Config) bool {
	if runtime.GOOS == "darwin" {
		return false
	}
	// abidw doesn't currently handle top-byte-ignore correctly. Disable ABI
	// dumping for those configs while we wait for a fix. We'll still have ABI
	// checking coverage from non-hwasan builds.
	// http://b/190554910
	if android.InList("hwaddress", config.SanitizeDevice()) {
		return false
	}
	// http://b/156513478
	return config.ReleaseNdkAbiMonitored()
}

// Feature flag to disable diffing against prebuilts.
func canDiffAbi(config android.Config) bool {
	return config.ReleaseNdkAbiMonitored()
}

func (this *stubDecorator) dumpAbi(ctx ModuleContext, symbolList android.Path) {
	implementationLibrary := this.findImplementationLibrary(ctx)
	this.abiDumpPath = getNdkAbiDumpInstallBase(ctx).Join(ctx,
		this.apiLevel.String(), ctx.Arch().ArchType.String(),
		this.libraryName(ctx), "abi.stg")
	headersList := getNdkABIHeadersFile(ctx)
	ctx.Build(pctx, android.BuildParams{
		Rule:        stg,
		Description: fmt.Sprintf("stg %s", implementationLibrary),
		Input:       implementationLibrary,
		Implicits: []android.Path{
			symbolList,
			headersList,
		},
		Output: this.abiDumpPath,
		Args: map[string]string{
			"symbolList":  symbolList.String(),
			"headersList": headersList.String(),
		},
	})
}

func findNextApiLevel(ctx ModuleContext, apiLevel android.ApiLevel) *android.ApiLevel {
	apiLevels := append(ctx.Config().AllSupportedApiLevels(),
		android.FutureApiLevel)
	for _, api := range apiLevels {
		if api.GreaterThan(apiLevel) {
			return &api
		}
	}
	return nil
}

func (this *stubDecorator) diffAbi(ctx ModuleContext) {
	// Catch any ABI changes compared to the checked-in definition of this API
	// level.
	abiDiffPath := android.PathForModuleOut(ctx, "stgdiff.timestamp")
	prebuiltAbiDump := this.findPrebuiltAbiDump(ctx, this.apiLevel)
	missingPrebuiltErrorTemplate :=
		"Did not find prebuilt ABI dump for %q (%q). Generate with " +
			"//development/tools/ndk/update_ndk_abi.sh."
	missingPrebuiltError := fmt.Sprintf(
		missingPrebuiltErrorTemplate, this.libraryName(ctx),
		prebuiltAbiDump.InvalidReason())
	if !prebuiltAbiDump.Valid() {
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.ErrorRule,
			Output: abiDiffPath,
			Args: map[string]string{
				"error": missingPrebuiltError,
			},
		})
	} else {
		ctx.Build(pctx, android.BuildParams{
			Rule: stgdiff,
			Description: fmt.Sprintf("Comparing ABI %s %s", prebuiltAbiDump,
				this.abiDumpPath),
			Output: abiDiffPath,
			Inputs: android.Paths{prebuiltAbiDump.Path(), this.abiDumpPath},
			Args: map[string]string{
				"args": "--format=small",
			},
		})
	}
	this.abiDiffPaths = append(this.abiDiffPaths, abiDiffPath)

	// Also ensure that the ABI of the next API level (if there is one) matches
	// this API level. *New* ABI is allowed, but any changes to APIs that exist
	// in this API level are disallowed.
	if !this.apiLevel.IsCurrent() && prebuiltAbiDump.Valid() {
		nextApiLevel := findNextApiLevel(ctx, this.apiLevel)
		if nextApiLevel == nil {
			panic(fmt.Errorf("could not determine which API level follows "+
				"non-current API level %s", this.apiLevel))
		}
		nextAbiDiffPath := android.PathForModuleOut(ctx,
			"abidiff_next.timestamp")
		nextAbiDump := this.findPrebuiltAbiDump(ctx, *nextApiLevel)
		missingNextPrebuiltError := fmt.Sprintf(
			missingPrebuiltErrorTemplate, this.libraryName(ctx),
			nextAbiDump.InvalidReason())
		if !nextAbiDump.Valid() {
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.ErrorRule,
				Output: nextAbiDiffPath,
				Args: map[string]string{
					"error": missingNextPrebuiltError,
				},
			})
		} else {
			ctx.Build(pctx, android.BuildParams{
				Rule: stgdiff,
				Description: fmt.Sprintf(
					"Comparing ABI to the next API level %s %s",
					prebuiltAbiDump, nextAbiDump),
				Output: nextAbiDiffPath,
				Inputs: android.Paths{
					prebuiltAbiDump.Path(), nextAbiDump.Path()},
				Args: map[string]string{
					"args": "--format=small --ignore=interface_addition",
				},
			})
		}
		this.abiDiffPaths = append(this.abiDiffPaths, nextAbiDiffPath)
	}
}

func (c *stubDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	if !strings.HasSuffix(String(c.properties.Symbol_file), ".map.txt") {
		ctx.PropertyErrorf("symbol_file", "must end with .map.txt")
	}

	if !c.buildStubs() {
		// NDK libraries have no implementation variant, nothing to do
		return Objects{}
	}

	if !c.initializeProperties(ctx) {
		// Emits its own errors, so we don't need to.
		return Objects{}
	}

	symbolFile := String(c.properties.Symbol_file)
	nativeAbiResult := parseNativeAbiDefinition(ctx, symbolFile, c.apiLevel, "")
	objs := compileStubLibrary(ctx, flags, nativeAbiResult.stubSrc)
	c.versionScriptPath = nativeAbiResult.versionScript
	if canDumpAbi(ctx.Config()) {
		c.dumpAbi(ctx, nativeAbiResult.symbolList)
		if canDiffAbi(ctx.Config()) {
			c.diffAbi(ctx)
		}
	}
	if c.apiLevel.IsCurrent() && ctx.PrimaryArch() {
		c.parsedCoverageXmlPath = parseSymbolFileForAPICoverage(ctx, symbolFile)
	}
	return objs
}

// Add a dependency on the header modules of this ndk_library
func (linker *stubDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	return Deps{
		HeaderLibs: linker.properties.Export_header_libs,
	}
}

func (linker *stubDecorator) moduleInfoJSON(ctx ModuleContext, moduleInfoJSON *android.ModuleInfoJSON) {
	linker.libraryDecorator.moduleInfoJSON(ctx, moduleInfoJSON)
	// Overwrites the SubName computed by libraryDecorator
	moduleInfoJSON.SubName = ndkLibrarySuffix + "." + linker.apiLevel.String()
}

func (linker *stubDecorator) Name(name string) string {
	return name + ndkLibrarySuffix
}

func (stub *stubDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	stub.libraryDecorator.libName = ctx.baseModuleName()
	return stub.libraryDecorator.linkerFlags(ctx, flags)
}

func (stub *stubDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps,
	objs Objects) android.Path {

	if !stub.buildStubs() {
		// NDK libraries have no implementation variant, nothing to do
		return nil
	}

	if shouldUseVersionScript(ctx, stub) {
		linkerScriptFlag := "-Wl,--version-script," + stub.versionScriptPath.String()
		flags.Local.LdFlags = append(flags.Local.LdFlags, linkerScriptFlag)
		flags.LdFlagsDeps = append(flags.LdFlagsDeps, stub.versionScriptPath)
	}

	stub.libraryDecorator.skipAPIDefine = true
	return stub.libraryDecorator.link(ctx, flags, deps, objs)
}

func (stub *stubDecorator) nativeCoverage() bool {
	return false
}

// Returns the install path for unversioned NDK libraries (currently only static
// libraries).
func getUnversionedLibraryInstallPath(ctx ModuleContext) android.InstallPath {
	return getNdkSysrootBase(ctx).Join(ctx, "usr/lib", config.NDKTriple(ctx.toolchain()))
}

// Returns the install path for versioned NDK libraries. These are most often
// stubs, but the same paths are used for CRT objects.
func getVersionedLibraryInstallPath(ctx ModuleContext, apiLevel android.ApiLevel) android.InstallPath {
	return getUnversionedLibraryInstallPath(ctx).Join(ctx, apiLevel.String())
}

func (stub *stubDecorator) install(ctx ModuleContext, path android.Path) {
	installDir := getVersionedLibraryInstallPath(ctx, stub.apiLevel)
	stub.installPath = ctx.InstallFile(installDir, path.Base(), path)
}

func newStubLibrary() *Module {
	module, library := NewLibrary(android.DeviceSupported)
	library.BuildOnlyShared()
	module.stl = nil
	module.sanitize = nil
	library.disableStripping()

	stub := &stubDecorator{
		libraryDecorator: library,
	}
	module.compiler = stub
	module.linker = stub
	module.installer = stub
	module.library = stub

	module.Properties.AlwaysSdk = true
	module.Properties.Sdk_version = StringPtr("current")

	module.AddProperties(&stub.properties, &library.MutatedProperties)

	return module
}

// ndk_library creates a library that exposes a stub implementation of functions
// and variables for use at build time only.
func NdkLibraryFactory() android.Module {
	module := newStubLibrary()
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibBoth)
	return module
}
