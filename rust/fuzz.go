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
	"android/soong/fuzz"
	"android/soong/rust/config"
)

func init() {
	android.RegisterModuleType("rust_fuzz", RustFuzzFactory)
	android.RegisterSingletonType("rust_fuzz_packaging", rustFuzzPackagingFactory)
}

type fuzzDecorator struct {
	*binaryDecorator

	fuzzPackagedModule fuzz.FuzzPackagedModule
}

var _ compiler = (*fuzzDecorator)(nil)

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
		&fuzzer.fuzzPackagedModule.FuzzProperties)
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
	fuzz.FuzzPackager
}

func rustFuzzPackagingFactory() android.Singleton {
	return &rustFuzzPackager{}
}

func (s *rustFuzzPackager) GenerateBuildActions(ctx android.SingletonContext) {
	// Map between each architecture + host/device combination.
	archDirs := make(map[fuzz.ArchOs][]fuzz.FileToZip)

	// List of individual fuzz targets.
	s.FuzzTargets = make(map[string]bool)

	// Map tracking whether each shared library has an install rule to avoid duplicate install rules from
	// multiple fuzzers that depend on the same shared library.
	sharedLibraryInstalled := make(map[string]bool)

	ctx.VisitAllModules(func(module android.Module) {
		// Discard non-fuzz targets.
		rustModule, ok := module.(*Module)
		if !ok {
			return
		}

		if ok := fuzz.IsValid(rustModule.FuzzModule); !ok || rustModule.Properties.PreventInstall {
			return
		}

		fuzzModule, ok := rustModule.compiler.(*fuzzDecorator)
		if !ok {
			return
		}

		hostOrTargetString := "target"
		if rustModule.Host() {
			hostOrTargetString = "host"
		}

		archString := rustModule.Arch().ArchType.String()
		archDir := android.PathForIntermediates(ctx, "fuzz", hostOrTargetString, archString)
		archOs := fuzz.ArchOs{HostOrTarget: hostOrTargetString, Arch: archString, Dir: archDir.String()}

		var files []fuzz.FileToZip
		builder := android.NewRuleBuilder(pctx, ctx)

		// Package the artifacts (data, corpus, config and dictionary into a zipfile.
		files = s.PackageArtifacts(ctx, module, fuzzModule.fuzzPackagedModule, archDir, builder)

		// The executable.
		files = append(files, fuzz.FileToZip{rustModule.UnstrippedOutputFile(), ""})

		// Grab the list of required shared libraries.
		sharedLibraries := fuzz.CollectAllSharedDependencies(ctx, module, cc.UnstrippedOutputFile, cc.IsValidSharedDependency)

		// Package shared libraries
		files = append(files, cc.GetSharedLibsToZip(sharedLibraries, rustModule, &s.FuzzPackager, archString, "lib", &sharedLibraryInstalled)...)

		archDirs[archOs], ok = s.BuildZipFile(ctx, module, fuzzModule.fuzzPackagedModule, files, builder, archDir, archString, hostOrTargetString, archOs, archDirs)
		if !ok {
			return
		}

	})
	s.CreateFuzzPackage(ctx, archDirs, fuzz.Rust, pctx)
}

func (s *rustFuzzPackager) MakeVars(ctx android.MakeVarsContext) {
	packages := s.Packages.Strings()
	sort.Strings(packages)

	ctx.Strict("SOONG_RUST_FUZZ_PACKAGING_ARCH_MODULES", strings.Join(packages, " "))

	// Preallocate the slice of fuzz targets to minimize memory allocations.
	s.PreallocateSlice(ctx, "ALL_RUST_FUZZ_TARGETS")
}

func (fuzz *fuzzDecorator) install(ctx ModuleContext) {
	fuzz.binaryDecorator.baseCompiler.dir = filepath.Join(
		"fuzz", ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzz.binaryDecorator.baseCompiler.dir64 = filepath.Join(
		"fuzz", ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzz.binaryDecorator.baseCompiler.install(ctx)

	if fuzz.fuzzPackagedModule.FuzzProperties.Corpus != nil {
		fuzz.fuzzPackagedModule.Corpus = android.PathsForModuleSrc(ctx, fuzz.fuzzPackagedModule.FuzzProperties.Corpus)
	}
	if fuzz.fuzzPackagedModule.FuzzProperties.Data != nil {
		fuzz.fuzzPackagedModule.Data = android.PathsForModuleSrc(ctx, fuzz.fuzzPackagedModule.FuzzProperties.Data)
	}
	if fuzz.fuzzPackagedModule.FuzzProperties.Dictionary != nil {
		fuzz.fuzzPackagedModule.Dictionary = android.PathForModuleSrc(ctx, *fuzz.fuzzPackagedModule.FuzzProperties.Dictionary)
	}
}
