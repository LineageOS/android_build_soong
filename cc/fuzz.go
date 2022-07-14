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
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
	"android/soong/fuzz"
)

func init() {
	android.RegisterModuleType("cc_afl_fuzz", AFLFuzzFactory)
	android.RegisterModuleType("cc_fuzz", LibFuzzFactory)
	android.RegisterSingletonType("cc_fuzz_packaging", fuzzPackagingFactory)
	android.RegisterSingletonType("cc_afl_fuzz_packaging", fuzzAFLPackagingFactory)
}

type FuzzProperties struct {
	AFLEnabled  bool `blueprint:"mutated"`
	AFLAddFlags bool `blueprint:"mutated"`
}

type fuzzer struct {
	Properties FuzzProperties
}

func (fuzzer *fuzzer) flags(ctx ModuleContext, flags Flags) Flags {
	if fuzzer.Properties.AFLAddFlags {
		flags.Local.CFlags = append(flags.Local.CFlags, "-fsanitize-coverage=trace-pc-guard")
	}

	return flags
}

func (fuzzer *fuzzer) props() []interface{} {
	return []interface{}{&fuzzer.Properties}
}

func fuzzMutatorDeps(mctx android.TopDownMutatorContext) {
	currentModule, ok := mctx.Module().(*Module)
	if !ok {
		return
	}

	if currentModule.fuzzer == nil || !currentModule.fuzzer.Properties.AFLEnabled {
		return
	}

	mctx.WalkDeps(func(child android.Module, parent android.Module) bool {
		c, ok := child.(*Module)
		if !ok {
			return false
		}

		if c.sanitize == nil {
			return false
		}

		isFuzzerPointer := c.sanitize.getSanitizerBoolPtr(Fuzzer)
		if isFuzzerPointer == nil || !*isFuzzerPointer {
			return false
		}

		if c.fuzzer == nil {
			return false
		}

		c.fuzzer.Properties.AFLEnabled = true
		c.fuzzer.Properties.AFLAddFlags = true
		return true
	})
}

func fuzzMutator(mctx android.BottomUpMutatorContext) {
	if c, ok := mctx.Module().(*Module); ok && c.fuzzer != nil {
		if !c.fuzzer.Properties.AFLEnabled {
			return
		}

		if c.Binary() {
			m := mctx.CreateVariations("afl")
			m[0].(*Module).fuzzer.Properties.AFLEnabled = true
			m[0].(*Module).fuzzer.Properties.AFLAddFlags = true
		} else {
			m := mctx.CreateVariations("", "afl")
			m[0].(*Module).fuzzer.Properties.AFLEnabled = false
			m[0].(*Module).fuzzer.Properties.AFLAddFlags = false

			m[1].(*Module).fuzzer.Properties.AFLEnabled = true
			m[1].(*Module).fuzzer.Properties.AFLAddFlags = true
		}
	}
}

// cc_fuzz creates a host/device fuzzer binary. Host binaries can be found at
// $ANDROID_HOST_OUT/fuzz/, and device binaries can be found at /data/fuzz on
// your device, or $ANDROID_PRODUCT_OUT/data/fuzz in your build tree.
func LibFuzzFactory() android.Module {
	module := NewFuzzer(android.HostAndDeviceSupported, fuzz.Cc)
	return module.Init()
}

// cc_afl_fuzz creates a host/device AFL++ fuzzer binary.
// AFL++ is an open source framework used to fuzz libraries
// Host binaries can be found at $ANDROID_HOST_OUT/afl_fuzz/ and device
// binaries can be found at $ANDROID_PRODUCT_OUT/data/afl_fuzz in your
// build tree
func AFLFuzzFactory() android.Module {
	module := NewFuzzer(android.HostAndDeviceSupported, fuzz.AFL)
	return module.Init()
}

type fuzzBinary struct {
	*binaryDecorator
	*baseCompiler
	fuzzPackagedModule  fuzz.FuzzPackagedModule
	installedSharedDeps []string
	fuzzType            fuzz.FuzzType
}

func (fuzz *fuzzBinary) fuzzBinary() bool {
	return true
}

func (fuzz *fuzzBinary) linkerProps() []interface{} {
	props := fuzz.binaryDecorator.linkerProps()
	props = append(props, &fuzz.fuzzPackagedModule.FuzzProperties)
	return props
}

func (fuzz *fuzzBinary) linkerInit(ctx BaseModuleContext) {
	fuzz.binaryDecorator.linkerInit(ctx)
}

func (fuzzBin *fuzzBinary) linkerDeps(ctx DepsContext, deps Deps) Deps {
	if fuzzBin.fuzzType == fuzz.AFL {
		deps.HeaderLibs = append(deps.HeaderLibs, "libafl_headers")
		deps = fuzzBin.binaryDecorator.linkerDeps(ctx, deps)
		return deps

	} else {
		deps.StaticLibs = append(deps.StaticLibs, config.LibFuzzerRuntimeLibrary(ctx.toolchain()))
		deps = fuzzBin.binaryDecorator.linkerDeps(ctx, deps)
		return deps
	}
}

func (fuzz *fuzzBinary) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = fuzz.binaryDecorator.linkerFlags(ctx, flags)
	// RunPaths on devices isn't instantiated by the base linker. `../lib` for
	// installed fuzz targets (both host and device), and `./lib` for fuzz
	// target packages.
	flags.Local.LdFlags = append(flags.Local.LdFlags, `-Wl,-rpath,\$$ORIGIN/../lib`)
	flags.Local.LdFlags = append(flags.Local.LdFlags, `-Wl,-rpath,\$$ORIGIN/lib`)

	return flags
}

func UnstrippedOutputFile(module android.Module) android.Path {
	if mod, ok := module.(LinkableInterface); ok {
		return mod.UnstrippedOutputFile()
	}
	panic("UnstrippedOutputFile called on non-LinkableInterface module: " + module.Name())
}

// IsValidSharedDependency takes a module and determines if it is a unique shared library
// that should be installed in the fuzz target output directories. This function
// returns true, unless:
//  - The module is not an installable shared library, or
//  - The module is a header or stub, or
//  - The module is a prebuilt and its source is available, or
//  - The module is a versioned member of an SDK snapshot.
func IsValidSharedDependency(dependency android.Module) bool {
	// TODO(b/144090547): We should be parsing these modules using
	// ModuleDependencyTag instead of the current brute-force checking.

	linkable, ok := dependency.(LinkableInterface)
	if !ok || !linkable.CcLibraryInterface() {
		// Discard non-linkables.
		return false
	}

	if !linkable.Shared() {
		// Discard static libs.
		return false
	}

	if lib := moduleLibraryInterface(dependency); lib != nil && lib.buildStubs() && linkable.CcLibrary() {
		// Discard stubs libs (only CCLibrary variants). Prebuilt libraries should not
		// be excluded on the basis of they're not CCLibrary()'s.
		return false
	}

	// We discarded module stubs libraries above, but the LLNDK prebuilts stubs
	// libraries must be handled differently - by looking for the stubDecorator.
	// Discard LLNDK prebuilts stubs as well.
	if ccLibrary, isCcLibrary := dependency.(*Module); isCcLibrary {
		if _, isLLndkStubLibrary := ccLibrary.linker.(*stubDecorator); isLLndkStubLibrary {
			return false
		}
		// Discard installable:false libraries because they are expected to be absent
		// in runtime.
		if !proptools.BoolDefault(ccLibrary.Installable(), true) {
			return false
		}
	}

	// If the same library is present both as source and a prebuilt we must pick
	// only one to avoid a conflict. Always prefer the source since the prebuilt
	// probably won't be built with sanitizers enabled.
	if prebuilt := android.GetEmbeddedPrebuilt(dependency); prebuilt != nil && prebuilt.SourceExists() {
		return false
	}

	// Discard versioned members of SDK snapshots, because they will conflict with
	// unversioned ones.
	if sdkMember, ok := dependency.(android.SdkAware); ok && !sdkMember.ContainingSdk().Unversioned() {
		return false
	}

	return true
}

func sharedLibraryInstallLocation(
	libraryPath android.Path, isHost bool, fuzzDir string, archString string) string {
	installLocation := "$(PRODUCT_OUT)/data"
	if isHost {
		installLocation = "$(HOST_OUT)"
	}
	installLocation = filepath.Join(
		installLocation, fuzzDir, archString, "lib", libraryPath.Base())
	return installLocation
}

// Get the device-only shared library symbols install directory.
func sharedLibrarySymbolsInstallLocation(libraryPath android.Path, fuzzDir string, archString string) string {
	return filepath.Join("$(PRODUCT_OUT)/symbols/data/", fuzzDir, archString, "/lib/", libraryPath.Base())
}

func (fuzzBin *fuzzBinary) install(ctx ModuleContext, file android.Path) {
	installBase := "fuzz"
	if fuzzBin.fuzzType == fuzz.AFL {
		installBase = "afl_fuzz"
	}

	fuzzBin.binaryDecorator.baseInstaller.dir = filepath.Join(
		installBase, ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzzBin.binaryDecorator.baseInstaller.dir64 = filepath.Join(
		installBase, ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzzBin.binaryDecorator.baseInstaller.install(ctx, file)

	fuzzBin.fuzzPackagedModule.Corpus = android.PathsForModuleSrc(ctx, fuzzBin.fuzzPackagedModule.FuzzProperties.Corpus)
	builder := android.NewRuleBuilder(pctx, ctx)
	intermediateDir := android.PathForModuleOut(ctx, "corpus")
	for _, entry := range fuzzBin.fuzzPackagedModule.Corpus {
		builder.Command().Text("cp").
			Input(entry).
			Output(intermediateDir.Join(ctx, entry.Base()))
	}
	builder.Build("copy_corpus", "copy corpus")
	fuzzBin.fuzzPackagedModule.CorpusIntermediateDir = intermediateDir

	fuzzBin.fuzzPackagedModule.Data = android.PathsForModuleSrc(ctx, fuzzBin.fuzzPackagedModule.FuzzProperties.Data)
	builder = android.NewRuleBuilder(pctx, ctx)
	intermediateDir = android.PathForModuleOut(ctx, "data")
	for _, entry := range fuzzBin.fuzzPackagedModule.Data {
		builder.Command().Text("cp").
			Input(entry).
			Output(intermediateDir.Join(ctx, entry.Rel()))
	}
	builder.Build("copy_data", "copy data")
	fuzzBin.fuzzPackagedModule.DataIntermediateDir = intermediateDir

	if fuzzBin.fuzzPackagedModule.FuzzProperties.Dictionary != nil {
		fuzzBin.fuzzPackagedModule.Dictionary = android.PathForModuleSrc(ctx, *fuzzBin.fuzzPackagedModule.FuzzProperties.Dictionary)
		if fuzzBin.fuzzPackagedModule.Dictionary.Ext() != ".dict" {
			ctx.PropertyErrorf("dictionary",
				"Fuzzer dictionary %q does not have '.dict' extension",
				fuzzBin.fuzzPackagedModule.Dictionary.String())
		}
	}

	if fuzzBin.fuzzPackagedModule.FuzzProperties.Fuzz_config != nil {
		configPath := android.PathForModuleOut(ctx, "config").Join(ctx, "config.json")
		android.WriteFileRule(ctx, configPath, fuzzBin.fuzzPackagedModule.FuzzProperties.Fuzz_config.String())
		fuzzBin.fuzzPackagedModule.Config = configPath
	}

	// Grab the list of required shared libraries.
	seen := make(map[string]bool)
	var sharedLibraries android.Paths
	ctx.WalkDeps(func(child, parent android.Module) bool {
		if seen[child.Name()] {
			return false
		}
		seen[child.Name()] = true

		if IsValidSharedDependency(child) {
			sharedLibraries = append(sharedLibraries, child.(*Module).UnstrippedOutputFile())
			return true
		}
		return false
	})

	for _, lib := range sharedLibraries {
		fuzzBin.installedSharedDeps = append(fuzzBin.installedSharedDeps,
			sharedLibraryInstallLocation(
				lib, ctx.Host(), installBase, ctx.Arch().ArchType.String()))

		// Also add the dependency on the shared library symbols dir.
		if !ctx.Host() {
			fuzzBin.installedSharedDeps = append(fuzzBin.installedSharedDeps,
				sharedLibrarySymbolsInstallLocation(lib, installBase, ctx.Arch().ArchType.String()))
		}
	}
}

func NewFuzzer(hod android.HostOrDeviceSupported, fuzzType fuzz.FuzzType) *Module {
	module, binary := newBinary(hod, false)
	baseInstallerPath := "fuzz"
	if fuzzType == fuzz.AFL {
		baseInstallerPath = "afl_fuzz"
	}

	binary.baseInstaller = NewBaseInstaller(baseInstallerPath, baseInstallerPath, InstallInData)
	module.sanitize.SetSanitizer(Fuzzer, true)

	fuzzBin := &fuzzBinary{
		binaryDecorator: binary,
		baseCompiler:    NewBaseCompiler(),
		fuzzType:        fuzzType,
	}
	module.compiler = fuzzBin
	module.linker = fuzzBin
	module.installer = fuzzBin

	// The fuzzer runtime is not present for darwin host modules, disable cc_fuzz modules when targeting darwin.
	android.AddLoadHook(module, func(ctx android.LoadHookContext) {
		disableDarwinAndLinuxBionic := struct {
			Target struct {
				Darwin struct {
					Enabled *bool
				}
				Linux_bionic struct {
					Enabled *bool
				}
			}
		}{}
		disableDarwinAndLinuxBionic.Target.Darwin.Enabled = BoolPtr(false)
		disableDarwinAndLinuxBionic.Target.Linux_bionic.Enabled = BoolPtr(false)
		ctx.AppendProperties(&disableDarwinAndLinuxBionic)
	})

	if fuzzType == fuzz.AFL {
		// Add cc_objects to Srcs
		fuzzBin.baseCompiler.Properties.Srcs = append(fuzzBin.baseCompiler.Properties.Srcs, ":aflpp_driver", ":afl-compiler-rt")
		module.fuzzer.Properties.AFLEnabled = true
		module.compiler.appendCflags([]string{
			"-Wno-unused-result",
			"-Wno-unused-parameter",
			"-Wno-unused-function",
		})
	}

	return module
}

// Responsible for generating GNU Make rules that package fuzz targets into
// their architecture & target/host specific zip file.
type ccFuzzPackager struct {
	fuzz.FuzzPackager
	fuzzPackagingArchModules         string
	fuzzTargetSharedDepsInstallPairs string
	allFuzzTargetsName               string
}

func fuzzPackagingFactory() android.Singleton {

	fuzzPackager := &ccFuzzPackager{
		fuzzPackagingArchModules:         "SOONG_FUZZ_PACKAGING_ARCH_MODULES",
		fuzzTargetSharedDepsInstallPairs: "FUZZ_TARGET_SHARED_DEPS_INSTALL_PAIRS",
		allFuzzTargetsName:               "ALL_FUZZ_TARGETS",
	}
	fuzzPackager.FuzzType = fuzz.Cc
	return fuzzPackager
}

func fuzzAFLPackagingFactory() android.Singleton {
	fuzzPackager := &ccFuzzPackager{
		fuzzPackagingArchModules:         "SOONG_AFL_FUZZ_PACKAGING_ARCH_MODULES",
		fuzzTargetSharedDepsInstallPairs: "AFL_FUZZ_TARGET_SHARED_DEPS_INSTALL_PAIRS",
		allFuzzTargetsName:               "ALL_AFL_FUZZ_TARGETS",
	}
	fuzzPackager.FuzzType = fuzz.AFL
	return fuzzPackager
}

func (s *ccFuzzPackager) GenerateBuildActions(ctx android.SingletonContext) {
	// Map between each architecture + host/device combination, and the files that
	// need to be packaged (in the tuple of {source file, destination folder in
	// archive}).
	archDirs := make(map[fuzz.ArchOs][]fuzz.FileToZip)

	// List of individual fuzz targets, so that 'make fuzz' also installs the targets
	// to the correct output directories as well.
	s.FuzzTargets = make(map[string]bool)

	// Map tracking whether each shared library has an install rule to avoid duplicate install rules from
	// multiple fuzzers that depend on the same shared library.
	sharedLibraryInstalled := make(map[string]bool)

	ctx.VisitAllModules(func(module android.Module) {
		ccModule, ok := module.(*Module)
		if !ok || ccModule.Properties.PreventInstall {
			return
		}

		// Discard non-fuzz targets.
		if ok := fuzz.IsValid(ccModule.FuzzModule); !ok {
			return
		}

		sharedLibsInstallDirPrefix := "lib"
		fuzzModule, ok := ccModule.compiler.(*fuzzBinary)
		if !ok || fuzzModule.fuzzType != s.FuzzType {
			return
		}

		hostOrTargetString := "target"
		if ccModule.Host() {
			hostOrTargetString = "host"
		}

		fpm := fuzz.FuzzPackagedModule{}
		if ok {
			fpm = fuzzModule.fuzzPackagedModule
		}

		intermediatePath := "fuzz"
		if s.FuzzType == fuzz.AFL {
			intermediatePath = "afl_fuzz"
		}

		archString := ccModule.Arch().ArchType.String()
		archDir := android.PathForIntermediates(ctx, intermediatePath, hostOrTargetString, archString)
		archOs := fuzz.ArchOs{HostOrTarget: hostOrTargetString, Arch: archString, Dir: archDir.String()}

		// Grab the list of required shared libraries.
		sharedLibraries := fuzz.CollectAllSharedDependencies(ctx, module, UnstrippedOutputFile, IsValidSharedDependency)

		var files []fuzz.FileToZip
		builder := android.NewRuleBuilder(pctx, ctx)

		// Package the corpus, data, dict and config into a zipfile.
		files = s.PackageArtifacts(ctx, module, fpm, archDir, builder)

		// Package shared libraries
		files = append(files, GetSharedLibsToZip(sharedLibraries, ccModule, &s.FuzzPackager, archString, sharedLibsInstallDirPrefix, &sharedLibraryInstalled)...)

		// The executable.
		files = append(files, fuzz.FileToZip{ccModule.UnstrippedOutputFile(), ""})

		archDirs[archOs], ok = s.BuildZipFile(ctx, module, fpm, files, builder, archDir, archString, hostOrTargetString, archOs, archDirs)
		if !ok {
			return
		}
	})

	s.CreateFuzzPackage(ctx, archDirs, s.FuzzType, pctx)
}

func (s *ccFuzzPackager) MakeVars(ctx android.MakeVarsContext) {
	packages := s.Packages.Strings()
	sort.Strings(packages)
	sort.Strings(s.FuzzPackager.SharedLibInstallStrings)
	// TODO(mitchp): Migrate this to use MakeVarsContext::DistForGoal() when it's
	// ready to handle phony targets created in Soong. In the meantime, this
	// exports the phony 'fuzz' target and dependencies on packages to
	// core/main.mk so that we can use dist-for-goals.

	ctx.Strict(s.fuzzPackagingArchModules, strings.Join(packages, " "))

	ctx.Strict(s.fuzzTargetSharedDepsInstallPairs,
		strings.Join(s.FuzzPackager.SharedLibInstallStrings, " "))

	// Preallocate the slice of fuzz targets to minimise memory allocations.
	s.PreallocateSlice(ctx, s.allFuzzTargetsName)
}

// GetSharedLibsToZip finds and marks all the transiently-dependent shared libraries for
// packaging.
func GetSharedLibsToZip(sharedLibraries android.Paths, module LinkableInterface, s *fuzz.FuzzPackager, archString string, destinationPathPrefix string, sharedLibraryInstalled *map[string]bool) []fuzz.FileToZip {
	var files []fuzz.FileToZip

	fuzzDir := "fuzz"
	if s.FuzzType == fuzz.AFL {
		fuzzDir = "afl_fuzz"
	}

	for _, library := range sharedLibraries {
		files = append(files, fuzz.FileToZip{library, destinationPathPrefix})

		// For each architecture-specific shared library dependency, we need to
		// install it to the output directory. Setup the install destination here,
		// which will be used by $(copy-many-files) in the Make backend.
		installDestination := sharedLibraryInstallLocation(
			library, module.Host(), fuzzDir, archString)
		if (*sharedLibraryInstalled)[installDestination] {
			continue
		}
		(*sharedLibraryInstalled)[installDestination] = true

		// Escape all the variables, as the install destination here will be called
		// via. $(eval) in Make.
		installDestination = strings.ReplaceAll(
			installDestination, "$", "$$")
		s.SharedLibInstallStrings = append(s.SharedLibInstallStrings,
			library.String()+":"+installDestination)

		// Ensure that on device, the library is also reinstalled to the /symbols/
		// dir. Symbolized DSO's are always installed to the device when fuzzing, but
		// we want symbolization tools (like `stack`) to be able to find the symbols
		// in $ANDROID_PRODUCT_OUT/symbols automagically.
		if !module.Host() {
			symbolsInstallDestination := sharedLibrarySymbolsInstallLocation(library, fuzzDir, archString)
			symbolsInstallDestination = strings.ReplaceAll(symbolsInstallDestination, "$", "$$")
			s.SharedLibInstallStrings = append(s.SharedLibInstallStrings,
				library.String()+":"+symbolsInstallDestination)
		}
	}
	return files
}
