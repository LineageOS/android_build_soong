// Copyright 2020 The Android Open Source Project
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

package rust

import (
	"path/filepath"
	"sort"
	"strings"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/rust/config"
)

func init() {
	android.RegisterModuleType("rust_fuzz", RustFuzzFactory)
	android.RegisterSingletonType("rust_fuzz_packaging", rustFuzzPackagingFactory)
}

type fuzzDecorator struct {
	*binaryDecorator

	Properties            cc.FuzzProperties
	dictionary            android.Path
	corpus                android.Paths
	corpusIntermediateDir android.Path
	config                android.Path
	data                  android.Paths
	dataIntermediateDir   android.Path
}

var _ compiler = (*binaryDecorator)(nil)

// rust_binary produces a binary that is runnable on a device.
func RustFuzzFactory() android.Module {
	module, _ := NewRustFuzz(android.HostAndDeviceSupported)
	return module.Init()
}

func NewRustFuzz(hod android.HostOrDeviceSupported) (*Module, *fuzzDecorator) {
	module, binary := NewRustBinary(hod)
	fuzz := &fuzzDecorator{
		binaryDecorator: binary,
	}

	// Change the defaults for the binaryDecorator's baseCompiler
	fuzz.binaryDecorator.baseCompiler.dir = "fuzz"
	fuzz.binaryDecorator.baseCompiler.dir64 = "fuzz"
	fuzz.binaryDecorator.baseCompiler.location = InstallInData
	module.sanitize.SetSanitizer(cc.Fuzzer, true)
	module.compiler = fuzz
	return module, fuzz
}

func (fuzzer *fuzzDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = fuzzer.binaryDecorator.compilerFlags(ctx, flags)

	// `../lib` for installed fuzz targets (both host and device), and `./lib` for fuzz target packages.
	flags.LinkFlags = append(flags.LinkFlags, `-Wl,-rpath,\$$ORIGIN/../lib`)
	flags.LinkFlags = append(flags.LinkFlags, `-Wl,-rpath,\$$ORIGIN/lib`)

	return flags
}

func (fuzzer *fuzzDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	if libFuzzerRuntimeLibrary := config.LibFuzzerRuntimeLibrary(ctx.toolchain()); libFuzzerRuntimeLibrary != "" {
		deps.StaticLibs = append(deps.StaticLibs, libFuzzerRuntimeLibrary)
	}
	deps.SharedLibs = append(deps.SharedLibs, "libc++")
	deps.Rlibs = append(deps.Rlibs, "liblibfuzzer_sys")

	deps = fuzzer.binaryDecorator.compilerDeps(ctx, deps)

	return deps
}

func (fuzzer *fuzzDecorator) compilerProps() []interface{} {
	return append(fuzzer.binaryDecorator.compilerProps(),
		&fuzzer.Properties)
}

func (fuzzer *fuzzDecorator) stdLinkage(ctx *depsContext) RustLinkage {
	return RlibLinkage
}

func (fuzzer *fuzzDecorator) autoDep(ctx android.BottomUpMutatorContext) autoDep {
	return rlibAutoDep
}

// Responsible for generating GNU Make rules that package fuzz targets into
// their architecture & target/host specific zip file.
type rustFuzzPackager struct {
	packages    android.Paths
	fuzzTargets map[string]bool
}

func rustFuzzPackagingFactory() android.Singleton {
	return &rustFuzzPackager{}
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

func (s *rustFuzzPackager) GenerateBuildActions(ctx android.SingletonContext) {

	// Map between each architecture + host/device combination.
	archDirs := make(map[archOs][]fileToZip)

	// List of individual fuzz targets.
	s.fuzzTargets = make(map[string]bool)

	ctx.VisitAllModules(func(module android.Module) {
		// Discard non-fuzz targets.
		rustModule, ok := module.(*Module)
		if !ok {
			return
		}

		fuzzModule, ok := rustModule.compiler.(*fuzzDecorator)
		if !ok {
			return
		}

		// Discard ramdisk + vendor_ramdisk + recovery modules, they're duplicates of
		// fuzz targets we're going to package anyway.
		if !rustModule.Enabled() || rustModule.Properties.PreventInstall ||
			rustModule.InRamdisk() || rustModule.InVendorRamdisk() || rustModule.InRecovery() {
			return
		}

		// Discard modules that are in an unavailable namespace.
		if !rustModule.ExportedToMake() {
			return
		}

		hostOrTargetString := "target"
		if rustModule.Host() {
			hostOrTargetString = "host"
		}

		archString := rustModule.Arch().ArchType.String()
		archDir := android.PathForIntermediates(ctx, "fuzz", hostOrTargetString, archString)
		archOs := archOs{hostOrTarget: hostOrTargetString, arch: archString, dir: archDir.String()}

		var files []fileToZip
		builder := android.NewRuleBuilder(pctx, ctx)

		// Package the corpora into a zipfile.
		if fuzzModule.corpus != nil {
			corpusZip := archDir.Join(ctx, module.Name()+"_seed_corpus.zip")
			command := builder.Command().BuiltTool("soong_zip").
				Flag("-j").
				FlagWithOutput("-o ", corpusZip)
			rspFile := corpusZip.ReplaceExtension(ctx, "rsp")
			command.FlagWithRspFileInputList("-r ", rspFile, fuzzModule.corpus)
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

		// The executable.
		files = append(files, fileToZip{rustModule.unstrippedOutputFile.Path(), ""})

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

		// Don't add modules to 'make haiku-rust' that are set to not be
		// exported to the fuzzing infrastructure.
		if config := fuzzModule.Properties.Fuzz_config; config != nil {
			if rustModule.Host() && !BoolDefault(config.Fuzz_on_haiku_host, true) {
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
		outputFile := android.PathForOutput(ctx, "fuzz-rust-"+hostOrTarget+"-"+arch+".zip")
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

func (s *rustFuzzPackager) MakeVars(ctx android.MakeVarsContext) {
	packages := s.packages.Strings()
	sort.Strings(packages)

	ctx.Strict("SOONG_RUST_FUZZ_PACKAGING_ARCH_MODULES", strings.Join(packages, " "))

	// Preallocate the slice of fuzz targets to minimise memory allocations.
	fuzzTargets := make([]string, 0, len(s.fuzzTargets))
	for target, _ := range s.fuzzTargets {
		fuzzTargets = append(fuzzTargets, target)
	}
	sort.Strings(fuzzTargets)
	ctx.Strict("ALL_RUST_FUZZ_TARGETS", strings.Join(fuzzTargets, " "))
}

func (fuzz *fuzzDecorator) install(ctx ModuleContext) {
	fuzz.binaryDecorator.baseCompiler.dir = filepath.Join(
		"fuzz", ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzz.binaryDecorator.baseCompiler.dir64 = filepath.Join(
		"fuzz", ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzz.binaryDecorator.baseCompiler.install(ctx)

	if fuzz.Properties.Corpus != nil {
		fuzz.corpus = android.PathsForModuleSrc(ctx, fuzz.Properties.Corpus)
	}
	if fuzz.Properties.Data != nil {
		fuzz.data = android.PathsForModuleSrc(ctx, fuzz.Properties.Data)
	}
	if fuzz.Properties.Dictionary != nil {
		fuzz.dictionary = android.PathForModuleSrc(ctx, *fuzz.Properties.Dictionary)
	}
}
