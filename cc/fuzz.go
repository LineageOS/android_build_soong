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

	"android/soong/android"
	"android/soong/cc/config"
)

type FuzzConfig struct {
	// Email address of people to CC on bugs or contact about this fuzz target.
	Cc []string `json:"cc,omitempty"`
	// Boolean specifying whether to disable the fuzz target from running
	// automatically in continuous fuzzing infrastructure.
	Disable *bool `json:"disable,omitempty"`
	// Component in Google's bug tracking system that bugs should be filed to.
	Componentid *int64 `json:"componentid,omitempty"`
	// Hotlists in Google's bug tracking system that bugs should be marked with.
	Hotlists []string `json:"hotlists,omitempty"`
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
func collectAllSharedDependencies(
	module android.Module,
	sharedDeps map[string]android.Path,
	ctx android.SingletonContext) {
	var fringe []android.Module

	// Enumerate the first level of dependencies, as we discard all non-library
	// modules in the BFS loop below.
	ctx.VisitDirectDeps(module, func(dep android.Module) {
		if isValidSharedDependency(dep, sharedDeps) {
			fringe = append(fringe, dep)
		}
	})

	for i := 0; i < len(fringe); i++ {
		module := fringe[i]
		if _, exists := sharedDeps[module.Name()]; exists {
			continue
		}

		ccModule := module.(*Module)
		sharedDeps[ccModule.Name()] = ccModule.UnstrippedOutputFile()
		ctx.VisitDirectDeps(module, func(dep android.Module) {
			if isValidSharedDependency(dep, sharedDeps) {
				fringe = append(fringe, dep)
			}
		})
	}
}

// This function takes a module and determines if it is a unique shared library
// that should be installed in the fuzz target output directories. This function
// returns true, unless:
//  - The module already exists in `sharedDeps`, or
//  - The module is not a shared library, or
//  - The module is a header, stub, or vendor-linked library.
func isValidSharedDependency(
	dependency android.Module,
	sharedDeps map[string]android.Path) bool {
	// TODO(b/144090547): We should be parsing these modules using
	// ModuleDependencyTag instead of the current brute-force checking.

	if linkable, ok := dependency.(LinkableInterface); !ok || // Discard non-linkables.
		!linkable.CcLibraryInterface() || !linkable.Shared() || // Discard static libs.
		linkable.UseVndk() || // Discard vendor linked libraries.
		// Discard stubs libs (only CCLibrary variants). Prebuilt libraries should not
		// be excluded on the basis of they're not CCLibrary()'s.
		(linkable.CcLibrary() && linkable.BuildStubs()) {
		return false
	}

	// We discarded module stubs libraries above, but the LLNDK prebuilts stubs
	// libraries must be handled differently - by looking for the stubDecorator.
	// Discard LLNDK prebuilts stubs as well.
	if ccLibrary, isCcLibrary := dependency.(*Module); isCcLibrary {
		if _, isLLndkStubLibrary := ccLibrary.linker.(*stubDecorator); isLLndkStubLibrary {
			return false
		}
	}

	// If this library has already been traversed, we don't need to do any more work.
	if _, exists := sharedDeps[dependency.Name()]; exists {
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

func (fuzz *fuzzBinary) install(ctx ModuleContext, file android.Path) {
	fuzz.binaryDecorator.baseInstaller.dir = filepath.Join(
		"fuzz", ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzz.binaryDecorator.baseInstaller.dir64 = filepath.Join(
		"fuzz", ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzz.binaryDecorator.baseInstaller.install(ctx, file)

	fuzz.corpus = android.PathsForModuleSrc(ctx, fuzz.Properties.Corpus)
	builder := android.NewRuleBuilder()
	intermediateDir := android.PathForModuleOut(ctx, "corpus")
	for _, entry := range fuzz.corpus {
		builder.Command().Text("cp").
			Input(entry).
			Output(intermediateDir.Join(ctx, entry.Base()))
	}
	builder.Build(pctx, ctx, "copy_corpus", "copy corpus")
	fuzz.corpusIntermediateDir = intermediateDir

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
		ctx.Build(pctx, android.BuildParams{
			Rule:        android.WriteFile,
			Description: "fuzzer infrastructure configuration",
			Output:      configPath,
			Args: map[string]string{
				"content": fuzz.Properties.Fuzz_config.String(),
			},
		})
		fuzz.config = configPath
	}

	// Grab the list of required shared libraries.
	sharedLibraries := make(map[string]android.Path)
	ctx.WalkDeps(func(child, parent android.Module) bool {
		if isValidSharedDependency(child, sharedLibraries) {
			sharedLibraries[child.Name()] = child.(*Module).UnstrippedOutputFile()
			return true
		}
		return false
	})

	for _, lib := range sharedLibraries {
		fuzz.installedSharedDeps = append(fuzz.installedSharedDeps,
			sharedLibraryInstallLocation(
				lib, ctx.Host(), ctx.Arch().ArchType.String()))
	}

	sort.Strings(fuzz.installedSharedDeps)
}

func NewFuzz(hod android.HostOrDeviceSupported) *Module {
	module, binary := NewBinary(hod)

	binary.baseInstaller = NewFuzzInstaller()
	module.sanitize.SetSanitizer(fuzzer, true)

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

type archAndLibraryKey struct {
	ArchDir android.OutputPath
	Library android.Path
}

func (s *fuzzPackager) GenerateBuildActions(ctx android.SingletonContext) {
	// Map between each architecture + host/device combination, and the files that
	// need to be packaged (in the tuple of {source file, destination folder in
	// archive}).
	archDirs := make(map[android.OutputPath][]fileToZip)

	// List of shared library dependencies for each architecture + host/device combo.
	archSharedLibraryDeps := make(map[archAndLibraryKey]bool)

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

		// Discard vendor-NDK-linked + recovery modules, they're duplicates of
		// fuzz targets we're going to package anyway.
		if !ccModule.Enabled() || ccModule.Properties.PreventInstall ||
			ccModule.UseVndk() || ccModule.InRecovery() {
			return
		}

		s.fuzzTargets[module.Name()] = true

		hostOrTargetString := "target"
		if ccModule.Host() {
			hostOrTargetString = "host"
		}

		archString := ccModule.Arch().ArchType.String()
		archDir := android.PathForIntermediates(ctx, "fuzz", hostOrTargetString, archString)

		// Grab the list of required shared libraries.
		sharedLibraries := make(map[string]android.Path)
		collectAllSharedDependencies(module, sharedLibraries, ctx)

		var files []fileToZip
		builder := android.NewRuleBuilder()

		// Package the corpora into a zipfile.
		if fuzzModule.corpus != nil {
			corpusZip := archDir.Join(ctx, module.Name()+"_seed_corpus.zip")
			command := builder.Command().BuiltTool(ctx, "soong_zip").
				Flag("-j").
				FlagWithOutput("-o ", corpusZip)
			command.FlagWithRspFileInputList("-l ", fuzzModule.corpus)
			files = append(files, fileToZip{corpusZip, ""})
		}

		// Find and mark all the transiently-dependent shared libraries for
		// packaging.
		for _, library := range sharedLibraries {
			files = append(files, fileToZip{library, "lib"})

			if _, exists := archSharedLibraryDeps[archAndLibraryKey{archDir, library}]; exists {
				continue
			}

			// For each architecture-specific shared library dependency, we need to
			// install it to the output directory. Setup the install destination here,
			// which will be used by $(copy-many-files) in the Make backend.
			archSharedLibraryDeps[archAndLibraryKey{archDir, library}] = true
			installDestination := sharedLibraryInstallLocation(
				library, ccModule.Host(), archString)
			// Escape all the variables, as the install destination here will be called
			// via. $(eval) in Make.
			installDestination = strings.ReplaceAll(
				installDestination, "$", "$$")
			s.sharedLibInstallStrings = append(s.sharedLibInstallStrings,
				library.String()+":"+installDestination)
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
		command := builder.Command().BuiltTool(ctx, "soong_zip").
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

		builder.Build(pctx, ctx, "create-"+fuzzZip.String(),
			"Package "+module.Name()+" for "+archString+"-"+hostOrTargetString)

		archDirs[archDir] = append(archDirs[archDir], fileToZip{fuzzZip, ""})
	})

	for archDir, filesToZip := range archDirs {
		arch := archDir.Base()
		hostOrTarget := filepath.Base(filepath.Dir(archDir.String()))
		builder := android.NewRuleBuilder()
		outputFile := android.PathForOutput(ctx, "fuzz-"+hostOrTarget+"-"+arch+".zip")
		s.packages = append(s.packages, outputFile)

		command := builder.Command().BuiltTool(ctx, "soong_zip").
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

		builder.Build(pctx, ctx, "create-fuzz-package-"+arch+"-"+hostOrTarget,
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
