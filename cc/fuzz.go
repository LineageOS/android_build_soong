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
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
)

type FuzzConfig struct {
	// Email address of people to CC on bugs or contact about this fuzz target.
	Cc []string `json:"cc,omitempty"`
	// Specify whether to enable continuous fuzzing on devices. Defaults to true.
	Fuzz_on_haiku_device *bool `json:"fuzz_on_haiku_device,omitempty"`
	// Specify whether to enable continuous fuzzing on host. Defaults to true.
	Fuzz_on_haiku_host *bool `json:"fuzz_on_haiku_host,omitempty"`
	// Component in Google's bug tracking system that bugs should be filed to.
	Componentid *int64 `json:"componentid,omitempty"`
	// Hotlists in Google's bug tracking system that bugs should be marked with.
	Hotlists []string `json:"hotlists,omitempty"`
	// Specify whether this fuzz target was submitted by a researcher. Defaults
	// to false.
	Researcher_submitted *bool `json:"researcher_submitted,omitempty"`
	// Specify who should be acknowledged for CVEs in the Android Security
	// Bulletin.
	Acknowledgement []string `json:"acknowledgement,omitempty"`
	// Additional options to be passed to libfuzzer when run in Haiku.
	Libfuzzer_options []string `json:"libfuzzer_options,omitempty"`
	// Additional options to be passed to HWASAN when running on-device in Haiku.
	Hwasan_options []string `json:"hwasan_options,omitempty"`
	// Additional options to be passed to HWASAN when running on host in Haiku.
	Asan_options []string `json:"asan_options,omitempty"`
}

func (f *FuzzConfig) String() string {
	b, err := json.Marshal(f)
	if err != nil {
		panic(err)
	}

	return string(b)
}

type FuzzProperties struct {
	// Optional list of seed files to be installed to the fuzz target's output
	// directory.
	Corpus []string `android:"path"`
	// Optional list of data files to be installed to the fuzz target's output
	// directory. Directory structure relative to the module is preserved.
	Data []string `android:"path"`
	// Optional dictionary to be installed to the fuzz target's output directory.
	Dictionary *string `android:"path"`
	// Config for running the target on fuzzing infrastructure.
	Fuzz_config *FuzzConfig
}

func init() {
	android.RegisterModuleType("cc_fuzz", FuzzFactory)
	android.RegisterSingletonType("cc_fuzz_packaging", fuzzPackagingFactory)
}

// cc_fuzz creates a host/device fuzzer binary. Host binaries can be found at
// $ANDROID_HOST_OUT/fuzz/, and device binaries can be found at /data/fuzz on
// your device, or $ANDROID_PRODUCT_OUT/data/fuzz in your build tree.
func FuzzFactory() android.Module {
	module := NewFuzz(android.HostAndDeviceSupported)
	return module.Init()
}

func NewFuzzInstaller() *baseInstaller {
	return NewBaseInstaller("fuzz", "fuzz", InstallInData)
}

type fuzzBinary struct {
	*binaryDecorator
	*baseCompiler

	Properties            FuzzProperties
	dictionary            android.Path
	corpus                android.Paths
	corpusIntermediateDir android.Path
	config                android.Path
	data                  android.Paths
	dataIntermediateDir   android.Path
	installedSharedDeps   []string
}

func (fuzz *fuzzBinary) linkerProps() []interface{} {
	props := fuzz.binaryDecorator.linkerProps()
	props = append(props, &fuzz.Properties)
	return props
}

func (fuzz *fuzzBinary) linkerInit(ctx BaseModuleContext) {
	fuzz.binaryDecorator.linkerInit(ctx)
}

func (fuzz *fuzzBinary) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps.StaticLibs = append(deps.StaticLibs,
		config.LibFuzzerRuntimeLibrary(ctx.toolchain()))
	deps = fuzz.binaryDecorator.linkerDeps(ctx, deps)
	return deps
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

// This function performs a breadth-first search over the provided module's
// dependencies using `visitDirectDeps` to enumerate all shared library
// dependencies. We require breadth-first expansion, as otherwise we may
// incorrectly use the core libraries (sanitizer runtimes, libc, libdl, etc.)
// from a dependency. This may cause issues when dependencies have explicit
// sanitizer tags, as we may get a dependency on an unsanitized libc, etc.
func collectAllSharedDependencies(ctx android.SingletonContext, module android.Module) android.Paths {
	var fringe []android.Module

	seen := make(map[string]bool)

	// Enumerate the first level of dependencies, as we discard all non-library
	// modules in the BFS loop below.
	ctx.VisitDirectDeps(module, func(dep android.Module) {
		if isValidSharedDependency(dep) {
			fringe = append(fringe, dep)
		}
	})

	var sharedLibraries android.Paths

	for i := 0; i < len(fringe); i++ {
		module := fringe[i]
		if seen[module.Name()] {
			continue
		}
		seen[module.Name()] = true

		ccModule := module.(*Module)
		sharedLibraries = append(sharedLibraries, ccModule.UnstrippedOutputFile())
		ctx.VisitDirectDeps(module, func(dep android.Module) {
			if isValidSharedDependency(dep) && !seen[dep.Name()] {
				fringe = append(fringe, dep)
			}
		})
	}

	return sharedLibraries
}

// This function takes a module and determines if it is a unique shared library
// that should be installed in the fuzz target output directories. This function
// returns true, unless:
//  - The module is not an installable shared library, or
//  - The module is a header, stub, or vendor-linked library, or
//  - The module is a prebuilt and its source is available, or
//  - The module is a versioned member of an SDK snapshot.
func isValidSharedDependency(dependency android.Module) bool {
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

	if linkable.UseVndk() {
		// Discard vendor linked libraries.
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
		if !proptools.BoolDefault(ccLibrary.Properties.Installable, true) {
			return false
		}
	}

	// If the same library is present both as source and a prebuilt we must pick
	// only one to avoid a conflict. Always prefer the source since the prebuilt
	// probably won't be built with sanitizers enabled.
	if prebuilt, ok := dependency.(android.PrebuiltInterface); ok &&
		prebuilt.Prebuilt() != nil && prebuilt.Prebuilt().SourceExists() {
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
	libraryPath android.Path, isHost bool, archString string) string {
	installLocation := "$(PRODUCT_OUT)/data"
	if isHost {
		installLocation = "$(HOST_OUT)"
	}
	installLocation = filepath.Join(
		installLocation, "fuzz", archString, "lib", libraryPath.Base())
	return installLocation
}

// Get the device-only shared library symbols install directory.
func sharedLibrarySymbolsInstallLocation(libraryPath android.Path, archString string) string {
	return filepath.Join("$(PRODUCT_OUT)/symbols/data/fuzz/", archString, "/lib/", libraryPath.Base())
}

func (fuzz *fuzzBinary) install(ctx ModuleContext, file android.Path) {
	fuzz.binaryDecorator.baseInstaller.dir = filepath.Join(
		"fuzz", ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzz.binaryDecorator.baseInstaller.dir64 = filepath.Join(
		"fuzz", ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzz.binaryDecorator.baseInstaller.install(ctx, file)

	fuzz.corpus = android.PathsForModuleSrc(ctx, fuzz.Properties.Corpus)
	builder := android.NewRuleBuilder(pctx, ctx)
	intermediateDir := android.PathForModuleOut(ctx, "corpus")
	for _, entry := range fuzz.corpus {
		builder.Command().Text("cp").
			Input(entry).
			Output(intermediateDir.Join(ctx, entry.Base()))
	}
	builder.Build("copy_corpus", "copy corpus")
	fuzz.corpusIntermediateDir = intermediateDir

	fuzz.data = android.PathsForModuleSrc(ctx, fuzz.Properties.Data)
	builder = android.NewRuleBuilder(pctx, ctx)
	intermediateDir = android.PathForModuleOut(ctx, "data")
	for _, entry := range fuzz.data {
		builder.Command().Text("cp").
			Input(entry).
			Output(intermediateDir.Join(ctx, entry.Rel()))
	}
	builder.Build("copy_data", "copy data")
	fuzz.dataIntermediateDir = intermediateDir

	if fuzz.Properties.Dictionary != nil {
		fuzz.dictionary = android.PathForModuleSrc(ctx, *fuzz.Properties.Dictionary)
		if fuzz.dictionary.Ext() != ".dict" {
			ctx.PropertyErrorf("dictionary",
				"Fuzzer dictionary %q does not have '.dict' extension",
				fuzz.dictionary.String())
		}
	}

	if fuzz.Properties.Fuzz_config != nil {
		configPath := android.PathForModuleOut(ctx, "config").Join(ctx, "config.json")
		android.WriteFileRule(ctx, configPath, fuzz.Properties.Fuzz_config.String())
		fuzz.config = configPath
	}

	// Grab the list of required shared libraries.
	seen := make(map[string]bool)
	var sharedLibraries android.Paths
	ctx.WalkDeps(func(child, parent android.Module) bool {
		if seen[child.Name()] {
			return false
		}
		seen[child.Name()] = true

		if isValidSharedDependency(child) {
			sharedLibraries = append(sharedLibraries, child.(*Module).UnstrippedOutputFile())
			return true
		}
		return false
	})

	for _, lib := range sharedLibraries {
		fuzz.installedSharedDeps = append(fuzz.installedSharedDeps,
			sharedLibraryInstallLocation(
				lib, ctx.Host(), ctx.Arch().ArchType.String()))

		// Also add the dependency on the shared library symbols dir.
		if !ctx.Host() {
			fuzz.installedSharedDeps = append(fuzz.installedSharedDeps,
				sharedLibrarySymbolsInstallLocation(lib, ctx.Arch().ArchType.String()))
		}
	}
}

func NewFuzz(hod android.HostOrDeviceSupported) *Module {
	module, binary := NewBinary(hod)

	binary.baseInstaller = NewFuzzInstaller()
	module.sanitize.SetSanitizer(Fuzzer, true)

	fuzz := &fuzzBinary{
		binaryDecorator: binary,
		baseCompiler:    NewBaseCompiler(),
	}
	module.compiler = fuzz
	module.linker = fuzz
	module.installer = fuzz

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

	return module
}

// Responsible for generating GNU Make rules that package fuzz targets into
// their architecture & target/host specific zip file.
type fuzzPackager struct {
	packages                android.Paths
	sharedLibInstallStrings []string
	fuzzTargets             map[string]bool
}

func fuzzPackagingFactory() android.Singleton {
	return &fuzzPackager{}
}

type fileToZip struct {
	SourceFilePath        android.Path
	DestinationPathPrefix string
}

type archOs struct {
	hostOrTarget string
	arch         string
	dir          string
}

func (s *fuzzPackager) GenerateBuildActions(ctx android.SingletonContext) {
	// Map between each architecture + host/device combination, and the files that
	// need to be packaged (in the tuple of {source file, destination folder in
	// archive}).
	archDirs := make(map[archOs][]fileToZip)

	// Map tracking whether each shared library has an install rule to avoid duplicate install rules from
	// multiple fuzzers that depend on the same shared library.
	sharedLibraryInstalled := make(map[string]bool)

	// List of individual fuzz targets, so that 'make fuzz' also installs the targets
	// to the correct output directories as well.
	s.fuzzTargets = make(map[string]bool)

	ctx.VisitAllModules(func(module android.Module) {
		// Discard non-fuzz targets.
		ccModule, ok := module.(*Module)
		if !ok {
			return
		}

		fuzzModule, ok := ccModule.compiler.(*fuzzBinary)
		if !ok {
			return
		}

		// Discard ramdisk + vendor_ramdisk + recovery modules, they're duplicates of
		// fuzz targets we're going to package anyway.
		if !ccModule.Enabled() || ccModule.Properties.PreventInstall ||
			ccModule.InRamdisk() || ccModule.InVendorRamdisk() || ccModule.InRecovery() {
			return
		}

		// Discard modules that are in an unavailable namespace.
		if !ccModule.ExportedToMake() {
			return
		}

		hostOrTargetString := "target"
		if ccModule.Host() {
			hostOrTargetString = "host"
		}

		archString := ccModule.Arch().ArchType.String()
		archDir := android.PathForIntermediates(ctx, "fuzz", hostOrTargetString, archString)
		archOs := archOs{hostOrTarget: hostOrTargetString, arch: archString, dir: archDir.String()}

		// Grab the list of required shared libraries.
		sharedLibraries := collectAllSharedDependencies(ctx, module)

		var files []fileToZip
		builder := android.NewRuleBuilder(pctx, ctx)

		// Package the corpora into a zipfile.
		if fuzzModule.corpus != nil {
			corpusZip := archDir.Join(ctx, module.Name()+"_seed_corpus.zip")
			command := builder.Command().BuiltTool("soong_zip").
				Flag("-j").
				FlagWithOutput("-o ", corpusZip)
			command.FlagWithRspFileInputList("-r ", fuzzModule.corpus)
			files = append(files, fileToZip{corpusZip, ""})
		}

		// Package the data into a zipfile.
		if fuzzModule.data != nil {
			dataZip := archDir.Join(ctx, module.Name()+"_data.zip")
			command := builder.Command().BuiltTool("soong_zip").
				FlagWithOutput("-o ", dataZip)
			for _, f := range fuzzModule.data {
				intermediateDir := strings.TrimSuffix(f.String(), f.Rel())
				command.FlagWithArg("-C ", intermediateDir)
				command.FlagWithInput("-f ", f)
			}
			files = append(files, fileToZip{dataZip, ""})
		}

		// Find and mark all the transiently-dependent shared libraries for
		// packaging.
		for _, library := range sharedLibraries {
			files = append(files, fileToZip{library, "lib"})

			// For each architecture-specific shared library dependency, we need to
			// install it to the output directory. Setup the install destination here,
			// which will be used by $(copy-many-files) in the Make backend.
			installDestination := sharedLibraryInstallLocation(
				library, ccModule.Host(), archString)
			if sharedLibraryInstalled[installDestination] {
				continue
			}
			sharedLibraryInstalled[installDestination] = true

			// Escape all the variables, as the install destination here will be called
			// via. $(eval) in Make.
			installDestination = strings.ReplaceAll(
				installDestination, "$", "$$")
			s.sharedLibInstallStrings = append(s.sharedLibInstallStrings,
				library.String()+":"+installDestination)

			// Ensure that on device, the library is also reinstalled to the /symbols/
			// dir. Symbolized DSO's are always installed to the device when fuzzing, but
			// we want symbolization tools (like `stack`) to be able to find the symbols
			// in $ANDROID_PRODUCT_OUT/symbols automagically.
			if !ccModule.Host() {
				symbolsInstallDestination := sharedLibrarySymbolsInstallLocation(library, archString)
				symbolsInstallDestination = strings.ReplaceAll(symbolsInstallDestination, "$", "$$")
				s.sharedLibInstallStrings = append(s.sharedLibInstallStrings,
					library.String()+":"+symbolsInstallDestination)
			}
		}

		// The executable.
		files = append(files, fileToZip{ccModule.UnstrippedOutputFile(), ""})

		// The dictionary.
		if fuzzModule.dictionary != nil {
			files = append(files, fileToZip{fuzzModule.dictionary, ""})
		}

		// Additional fuzz config.
		if fuzzModule.config != nil {
			files = append(files, fileToZip{fuzzModule.config, ""})
		}

		fuzzZip := archDir.Join(ctx, module.Name()+".zip")
		command := builder.Command().BuiltTool("soong_zip").
			Flag("-j").
			FlagWithOutput("-o ", fuzzZip)
		for _, file := range files {
			if file.DestinationPathPrefix != "" {
				command.FlagWithArg("-P ", file.DestinationPathPrefix)
			} else {
				command.Flag("-P ''")
			}
			command.FlagWithInput("-f ", file.SourceFilePath)
		}

		builder.Build("create-"+fuzzZip.String(),
			"Package "+module.Name()+" for "+archString+"-"+hostOrTargetString)

		// Don't add modules to 'make haiku' that are set to not be exported to the
		// fuzzing infrastructure.
		if config := fuzzModule.Properties.Fuzz_config; config != nil {
			if ccModule.Host() && !BoolDefault(config.Fuzz_on_haiku_host, true) {
				return
			} else if !BoolDefault(config.Fuzz_on_haiku_device, true) {
				return
			}
		}

		s.fuzzTargets[module.Name()] = true
		archDirs[archOs] = append(archDirs[archOs], fileToZip{fuzzZip, ""})
	})

	var archOsList []archOs
	for archOs := range archDirs {
		archOsList = append(archOsList, archOs)
	}
	sort.Slice(archOsList, func(i, j int) bool { return archOsList[i].dir < archOsList[j].dir })

	for _, archOs := range archOsList {
		filesToZip := archDirs[archOs]
		arch := archOs.arch
		hostOrTarget := archOs.hostOrTarget
		builder := android.NewRuleBuilder(pctx, ctx)
		outputFile := android.PathForOutput(ctx, "fuzz-"+hostOrTarget+"-"+arch+".zip")
		s.packages = append(s.packages, outputFile)

		command := builder.Command().BuiltTool("soong_zip").
			Flag("-j").
			FlagWithOutput("-o ", outputFile).
			Flag("-L 0") // No need to try and re-compress the zipfiles.

		for _, fileToZip := range filesToZip {
			if fileToZip.DestinationPathPrefix != "" {
				command.FlagWithArg("-P ", fileToZip.DestinationPathPrefix)
			} else {
				command.Flag("-P ''")
			}
			command.FlagWithInput("-f ", fileToZip.SourceFilePath)
		}

		builder.Build("create-fuzz-package-"+arch+"-"+hostOrTarget,
			"Create fuzz target packages for "+arch+"-"+hostOrTarget)
	}
}

func (s *fuzzPackager) MakeVars(ctx android.MakeVarsContext) {
	packages := s.packages.Strings()
	sort.Strings(packages)
	sort.Strings(s.sharedLibInstallStrings)
	// TODO(mitchp): Migrate this to use MakeVarsContext::DistForGoal() when it's
	// ready to handle phony targets created in Soong. In the meantime, this
	// exports the phony 'fuzz' target and dependencies on packages to
	// core/main.mk so that we can use dist-for-goals.
	ctx.Strict("SOONG_FUZZ_PACKAGING_ARCH_MODULES", strings.Join(packages, " "))
	ctx.Strict("FUZZ_TARGET_SHARED_DEPS_INSTALL_PAIRS",
		strings.Join(s.sharedLibInstallStrings, " "))

	// Preallocate the slice of fuzz targets to minimise memory allocations.
	fuzzTargets := make([]string, 0, len(s.fuzzTargets))
	for target, _ := range s.fuzzTargets {
		fuzzTargets = append(fuzzTargets, target)
	}
	sort.Strings(fuzzTargets)
	ctx.Strict("ALL_FUZZ_TARGETS", strings.Join(fuzzTargets, " "))
}
