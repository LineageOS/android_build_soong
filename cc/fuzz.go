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
	android.RegisterModuleType("cc_fuzz", LibFuzzFactory)
	android.RegisterParallelSingletonType("cc_fuzz_packaging", fuzzPackagingFactory)
	android.RegisterParallelSingletonType("cc_fuzz_presubmit_packaging", fuzzPackagingFactoryPresubmit)
}

type FuzzProperties struct {
	FuzzFramework fuzz.Framework `blueprint:"mutated"`
}

type fuzzer struct {
	Properties FuzzProperties
}

func (fuzzer *fuzzer) flags(ctx ModuleContext, flags Flags) Flags {
	if fuzzer.Properties.FuzzFramework == fuzz.AFL {
		flags.Local.CFlags = append(flags.Local.CFlags, []string{
			"-fsanitize-coverage=trace-pc-guard",
			"-Wno-unused-result",
			"-Wno-unused-parameter",
			"-Wno-unused-function",
		}...)
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

	if currentModule.fuzzer == nil {
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

		c.fuzzer.Properties.FuzzFramework = currentModule.fuzzer.Properties.FuzzFramework
		return true
	})
}

// cc_fuzz creates a host/device fuzzer binary. Host binaries can be found at
// $ANDROID_HOST_OUT/fuzz/, and device binaries can be found at /data/fuzz on
// your device, or $ANDROID_PRODUCT_OUT/data/fuzz in your build tree.
func LibFuzzFactory() android.Module {
	module := NewFuzzer(android.HostAndDeviceSupported)
	module.testModule = true
	return module.Init()
}

type fuzzBinary struct {
	*binaryDecorator
	*baseCompiler
	fuzzPackagedModule  fuzz.FuzzPackagedModule
	installedSharedDeps []string
	sharedLibraries     android.RuleBuilderInstalls
	data                []android.DataPath
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
	if ctx.Config().Getenv("FUZZ_FRAMEWORK") == "AFL" {
		deps.HeaderLibs = append(deps.HeaderLibs, "libafl_headers")
	} else {
		deps.StaticLibs = append(deps.StaticLibs, config.LibFuzzerRuntimeLibrary(ctx.toolchain()))
		// Fuzzers built with HWASAN should use the interceptors for better
		// mutation based on signals in strcmp, memcpy, etc. This is only needed for
		// fuzz targets, not generic HWASAN-ified binaries or libraries.
		if module, ok := ctx.Module().(*Module); ok {
			if module.IsSanitizerEnabled(Hwasan) {
				deps.StaticLibs = append(deps.StaticLibs, config.LibFuzzerRuntimeInterceptors(ctx.toolchain()))
			}
		}
	}

	deps = fuzzBin.binaryDecorator.linkerDeps(ctx, deps)
	return deps
}

func (fuzz *fuzzBinary) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	subdir := "lib"
	if ctx.inVendor() {
		subdir = "lib/vendor"
	}

	flags = fuzz.binaryDecorator.linkerFlags(ctx, flags)
	// RunPaths on devices isn't instantiated by the base linker. `../lib` for
	// installed fuzz targets (both host and device), and `./lib` for fuzz
	// target packages.
	flags.Local.LdFlags = append(flags.Local.LdFlags, `-Wl,-rpath,\$$ORIGIN/`+subdir)

	// When running on device, fuzz targets with vendor: true set will be in
	// fuzzer_name/vendor/fuzzer_name (note the extra 'vendor' and thus need to
	// link with libraries in ../../lib/. Non-vendor binaries only need to look
	// one level up, in ../lib/.
	if ctx.inVendor() {
		flags.Local.LdFlags = append(flags.Local.LdFlags, `-Wl,-rpath,\$$ORIGIN/../../`+subdir)
	} else {
		flags.Local.LdFlags = append(flags.Local.LdFlags, `-Wl,-rpath,\$$ORIGIN/../`+subdir)
	}

	return flags
}

func (fuzz *fuzzBinary) moduleInfoJSON(ctx ModuleContext, moduleInfoJSON *android.ModuleInfoJSON) {
	fuzz.binaryDecorator.moduleInfoJSON(ctx, moduleInfoJSON)
	moduleInfoJSON.Class = []string{"EXECUTABLES"}
}

// IsValidSharedDependency takes a module and determines if it is a unique shared library
// that should be installed in the fuzz target output directories. This function
// returns true, unless:
//   - The module is not an installable shared library, or
//   - The module is a header or stub, or
//   - The module is a prebuilt and its source is available, or
//   - The module is a versioned member of an SDK snapshot.
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

	return true
}

func SharedLibraryInstallLocation(
	libraryBase string, isHost bool, isVendor bool, fuzzDir string, archString string) string {
	installLocation := "$(PRODUCT_OUT)/data"
	if isHost {
		installLocation = "$(HOST_OUT)"
	}
	subdir := "lib"
	if isVendor {
		subdir = "lib/vendor"
	}
	installLocation = filepath.Join(
		installLocation, fuzzDir, archString, subdir, libraryBase)
	return installLocation
}

// Get the device-only shared library symbols install directory.
func SharedLibrarySymbolsInstallLocation(libraryBase string, isVendor bool, fuzzDir string, archString string) string {
	subdir := "lib"
	if isVendor {
		subdir = "lib/vendor"
	}
	return filepath.Join("$(PRODUCT_OUT)/symbols/data/", fuzzDir, archString, subdir, libraryBase)
}

func (fuzzBin *fuzzBinary) install(ctx ModuleContext, file android.Path) {
	fuzzBin.fuzzPackagedModule = PackageFuzzModule(ctx, fuzzBin.fuzzPackagedModule, pctx)

	installBase := "fuzz"

	// Grab the list of required shared libraries.
	fuzzBin.sharedLibraries, _ = CollectAllSharedDependencies(ctx)

	// TODO: does not mirror Android linkernamespaces
	// the logic here has special cases for vendor, but it would need more work to
	// work in arbitrary partitions, so just surface errors early for a few cases
	//
	// Even without these, there are certain situations across linkernamespaces
	// that this won't support. For instance, you might have:
	//
	//     my_fuzzer (vendor) -> libbinder_ndk (core) -> libbinder (vendor)
	//
	// This dependency chain wouldn't be possible to express in the current
	// logic because all the deps currently match the variant of the source
	// module.

	for _, ruleBuilderInstall := range fuzzBin.sharedLibraries {
		install := ruleBuilderInstall.To
		fuzzBin.installedSharedDeps = append(fuzzBin.installedSharedDeps,
			SharedLibraryInstallLocation(
				install, ctx.Host(), ctx.inVendor(), installBase, ctx.Arch().ArchType.String()))

		// Also add the dependency on the shared library symbols dir.
		if !ctx.Host() {
			fuzzBin.installedSharedDeps = append(fuzzBin.installedSharedDeps,
				SharedLibrarySymbolsInstallLocation(install, ctx.inVendor(), installBase, ctx.Arch().ArchType.String()))
		}
	}

	for _, d := range fuzzBin.fuzzPackagedModule.Corpus {
		fuzzBin.data = append(fuzzBin.data, android.DataPath{SrcPath: d, RelativeInstallPath: "corpus", WithoutRel: true})
	}

	for _, d := range fuzzBin.fuzzPackagedModule.Data {
		fuzzBin.data = append(fuzzBin.data, android.DataPath{SrcPath: d, RelativeInstallPath: "data"})
	}

	if d := fuzzBin.fuzzPackagedModule.Dictionary; d != nil {
		fuzzBin.data = append(fuzzBin.data, android.DataPath{SrcPath: d, WithoutRel: true})
	}

	if d := fuzzBin.fuzzPackagedModule.Config; d != nil {
		fuzzBin.data = append(fuzzBin.data, android.DataPath{SrcPath: d, WithoutRel: true})
	}

	fuzzBin.binaryDecorator.baseInstaller.dir = filepath.Join(
		installBase, ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzzBin.binaryDecorator.baseInstaller.dir64 = filepath.Join(
		installBase, ctx.Target().Arch.ArchType.String(), ctx.ModuleName())
	fuzzBin.binaryDecorator.baseInstaller.installTestData(ctx, fuzzBin.data)
	fuzzBin.binaryDecorator.baseInstaller.install(ctx, file)
}

func PackageFuzzModule(ctx android.ModuleContext, fuzzPackagedModule fuzz.FuzzPackagedModule, pctx android.PackageContext) fuzz.FuzzPackagedModule {
	fuzzPackagedModule.Corpus = android.PathsForModuleSrc(ctx, fuzzPackagedModule.FuzzProperties.Corpus)

	fuzzPackagedModule.Data = android.PathsForModuleSrc(ctx, fuzzPackagedModule.FuzzProperties.Data)

	if fuzzPackagedModule.FuzzProperties.Dictionary != nil {
		fuzzPackagedModule.Dictionary = android.PathForModuleSrc(ctx, *fuzzPackagedModule.FuzzProperties.Dictionary)
		if fuzzPackagedModule.Dictionary.Ext() != ".dict" {
			ctx.PropertyErrorf("dictionary",
				"Fuzzer dictionary %q does not have '.dict' extension",
				fuzzPackagedModule.Dictionary.String())
		}
	}

	if fuzzPackagedModule.FuzzProperties.Fuzz_config != nil {
		configPath := android.PathForModuleOut(ctx, "config").Join(ctx, "config.json")
		android.WriteFileRule(ctx, configPath, fuzzPackagedModule.FuzzProperties.Fuzz_config.String())
		fuzzPackagedModule.Config = configPath
	}
	return fuzzPackagedModule
}

func NewFuzzer(hod android.HostOrDeviceSupported) *Module {
	module, binary := newBinary(hod)
	baseInstallerPath := "fuzz"

	binary.baseInstaller = NewBaseInstaller(baseInstallerPath, baseInstallerPath, InstallInData)

	fuzzBin := &fuzzBinary{
		binaryDecorator: binary,
		baseCompiler:    NewBaseCompiler(),
	}
	module.compiler = fuzzBin
	module.linker = fuzzBin
	module.installer = fuzzBin

	module.fuzzer.Properties.FuzzFramework = fuzz.LibFuzzer

	// The fuzzer runtime is not present for darwin host modules, disable cc_fuzz modules when targeting darwin.
	android.AddLoadHook(module, func(ctx android.LoadHookContext) {

		extraProps := struct {
			Sanitize struct {
				Fuzzer *bool
			}
			Target struct {
				Darwin struct {
					Enabled *bool
				}
				Linux_bionic struct {
					Enabled *bool
				}
			}
		}{}
		extraProps.Sanitize.Fuzzer = BoolPtr(true)
		extraProps.Target.Darwin.Enabled = BoolPtr(false)
		extraProps.Target.Linux_bionic.Enabled = BoolPtr(false)
		ctx.AppendProperties(&extraProps)

		targetFramework := fuzz.GetFramework(ctx, fuzz.Cc)
		if !fuzz.IsValidFrameworkForModule(targetFramework, fuzz.Cc, fuzzBin.fuzzPackagedModule.FuzzProperties.Fuzzing_frameworks) {
			ctx.Module().Disable()
			return
		}

		if targetFramework == fuzz.AFL {
			fuzzBin.baseCompiler.Properties.Srcs = append(fuzzBin.baseCompiler.Properties.Srcs, ":aflpp_driver", ":afl-compiler-rt")
			module.fuzzer.Properties.FuzzFramework = fuzz.AFL
		}
	})

	return module
}

// Responsible for generating GNU Make rules that package fuzz targets into
// their architecture & target/host specific zip file.
type ccRustFuzzPackager struct {
	fuzz.FuzzPackager
	fuzzPackagingArchModules         string
	fuzzTargetSharedDepsInstallPairs string
	allFuzzTargetsName               string
	onlyIncludePresubmits            bool
}

func fuzzPackagingFactory() android.Singleton {

	fuzzPackager := &ccRustFuzzPackager{
		fuzzPackagingArchModules:         "SOONG_FUZZ_PACKAGING_ARCH_MODULES",
		fuzzTargetSharedDepsInstallPairs: "FUZZ_TARGET_SHARED_DEPS_INSTALL_PAIRS",
		allFuzzTargetsName:               "ALL_FUZZ_TARGETS",
		onlyIncludePresubmits:            false,
	}
	return fuzzPackager
}

func fuzzPackagingFactoryPresubmit() android.Singleton {

	fuzzPackager := &ccRustFuzzPackager{
		fuzzPackagingArchModules:         "SOONG_PRESUBMIT_FUZZ_PACKAGING_ARCH_MODULES",
		fuzzTargetSharedDepsInstallPairs: "PRESUBMIT_FUZZ_TARGET_SHARED_DEPS_INSTALL_PAIRS",
		allFuzzTargetsName:               "ALL_PRESUBMIT_FUZZ_TARGETS",
		onlyIncludePresubmits:            true,
	}
	return fuzzPackager
}

func (s *ccRustFuzzPackager) GenerateBuildActions(ctx android.SingletonContext) {
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
		ccModule, ok := module.(LinkableInterface)
		if !ok || ccModule.PreventInstall() {
			return
		}
		// Discard non-fuzz targets.
		if ok := fuzz.IsValid(ccModule.FuzzModuleStruct()); !ok {
			return
		}

		sharedLibsInstallDirPrefix := "lib"
		if ccModule.InVendor() {
			sharedLibsInstallDirPrefix = "lib/vendor"
		}

		if !ccModule.IsFuzzModule() {
			return
		}

		hostOrTargetString := "target"
		if ccModule.Target().HostCross {
			hostOrTargetString = "host_cross"
		} else if ccModule.Host() {
			hostOrTargetString = "host"
		}
		if s.onlyIncludePresubmits == true {
			hostOrTargetString = "presubmit-" + hostOrTargetString
		}

		fpm := fuzz.FuzzPackagedModule{}
		if ok {
			fpm = ccModule.FuzzPackagedModule()
		}

		intermediatePath := "fuzz"

		archString := ccModule.Target().Arch.ArchType.String()
		archDir := android.PathForIntermediates(ctx, intermediatePath, hostOrTargetString, archString)
		archOs := fuzz.ArchOs{HostOrTarget: hostOrTargetString, Arch: archString, Dir: archDir.String()}

		var files []fuzz.FileToZip
		builder := android.NewRuleBuilder(pctx, ctx)

		// Package the corpus, data, dict and config into a zipfile.
		files = s.PackageArtifacts(ctx, module, fpm, archDir, builder)

		// Package shared libraries
		files = append(files, GetSharedLibsToZip(ccModule.FuzzSharedLibraries(), ccModule, &s.FuzzPackager, archString, sharedLibsInstallDirPrefix, &sharedLibraryInstalled)...)

		// The executable.
		files = append(files, fuzz.FileToZip{SourceFilePath: android.OutputFileForModule(ctx, ccModule, "unstripped")})

		if s.onlyIncludePresubmits == true {
			if fpm.FuzzProperties.Fuzz_config == nil {
				return
			}
			if !BoolDefault(fpm.FuzzProperties.Fuzz_config.Use_for_presubmit, false) {
				return
			}
		}
		archDirs[archOs], ok = s.BuildZipFile(ctx, module, fpm, files, builder, archDir, archString, hostOrTargetString, archOs, archDirs)
		if !ok {
			return
		}
	})

	s.CreateFuzzPackage(ctx, archDirs, fuzz.Cc, pctx)
}

func (s *ccRustFuzzPackager) MakeVars(ctx android.MakeVarsContext) {
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
func GetSharedLibsToZip(sharedLibraries android.RuleBuilderInstalls, module LinkableInterface, s *fuzz.FuzzPackager, archString string, destinationPathPrefix string, sharedLibraryInstalled *map[string]bool) []fuzz.FileToZip {
	var files []fuzz.FileToZip

	fuzzDir := "fuzz"

	for _, ruleBuilderInstall := range sharedLibraries {
		library := ruleBuilderInstall.From
		install := ruleBuilderInstall.To
		files = append(files, fuzz.FileToZip{
			SourceFilePath:        library,
			DestinationPathPrefix: destinationPathPrefix,
			DestinationPath:       install,
		})

		// For each architecture-specific shared library dependency, we need to
		// install it to the output directory. Setup the install destination here,
		// which will be used by $(copy-many-files) in the Make backend.
		installDestination := SharedLibraryInstallLocation(
			install, module.Host(), module.InVendor(), fuzzDir, archString)
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
			symbolsInstallDestination := SharedLibrarySymbolsInstallLocation(install, module.InVendor(), fuzzDir, archString)
			symbolsInstallDestination = strings.ReplaceAll(symbolsInstallDestination, "$", "$$")
			s.SharedLibInstallStrings = append(s.SharedLibInstallStrings,
				library.String()+":"+symbolsInstallDestination)
		}
	}
	return files
}

// CollectAllSharedDependencies search over the provided module's dependencies using
// VisitDirectDeps and WalkDeps to enumerate all shared library dependencies.
// VisitDirectDeps is used first to avoid incorrectly using the core libraries (sanitizer
// runtimes, libc, libdl, etc.) from a dependency. This may cause issues when dependencies
// have explicit sanitizer tags, as we may get a dependency on an unsanitized libc, etc.
func CollectAllSharedDependencies(ctx android.ModuleContext) (android.RuleBuilderInstalls, []android.Module) {
	seen := make(map[string]bool)
	recursed := make(map[string]bool)
	deps := []android.Module{}

	var sharedLibraries android.RuleBuilderInstalls

	// Enumerate the first level of dependencies, as we discard all non-library
	// modules in the BFS loop below.
	ctx.VisitDirectDeps(func(dep android.Module) {
		if !IsValidSharedDependency(dep) {
			return
		}
		sharedLibraryInfo, hasSharedLibraryInfo := android.OtherModuleProvider(ctx, dep, SharedLibraryInfoProvider)
		if !hasSharedLibraryInfo {
			return
		}
		if seen[ctx.OtherModuleName(dep)] {
			return
		}
		seen[ctx.OtherModuleName(dep)] = true
		deps = append(deps, dep)

		installDestination := sharedLibraryInfo.SharedLibrary.Base()
		ruleBuilderInstall := android.RuleBuilderInstall{android.OutputFileForModule(ctx, dep, "unstripped"), installDestination}
		sharedLibraries = append(sharedLibraries, ruleBuilderInstall)
	})

	ctx.WalkDeps(func(child, parent android.Module) bool {

		// If this is a Rust module which is not rust_ffi_shared, we still want to bundle any transitive
		// shared dependencies (even for rust_ffi_static)
		if rustmod, ok := child.(LinkableInterface); ok && rustmod.RustLibraryInterface() && !rustmod.Shared() {
			if recursed[ctx.OtherModuleName(child)] {
				return false
			}
			recursed[ctx.OtherModuleName(child)] = true
			return true
		}

		if !IsValidSharedDependency(child) {
			return false
		}
		sharedLibraryInfo, hasSharedLibraryInfo := android.OtherModuleProvider(ctx, child, SharedLibraryInfoProvider)
		if !hasSharedLibraryInfo {
			return false
		}
		if !seen[ctx.OtherModuleName(child)] {
			seen[ctx.OtherModuleName(child)] = true
			deps = append(deps, child)

			installDestination := sharedLibraryInfo.SharedLibrary.Base()
			ruleBuilderInstall := android.RuleBuilderInstall{android.OutputFileForModule(ctx, child, "unstripped"), installDestination}
			sharedLibraries = append(sharedLibraries, ruleBuilderInstall)
		}

		if recursed[ctx.OtherModuleName(child)] {
			return false
		}
		recursed[ctx.OtherModuleName(child)] = true
		return true
	})

	return sharedLibraries, deps
}
