// Copyright 2021 Google Inc. All rights reserved.
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

package java

import (
	"path/filepath"
	"sort"
	"strings"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/fuzz"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

const (
	hostString   = "host"
	targetString = "target"
	deviceString = "device"
)

// Any shared libs for these deps will also be packaged
var artDeps = []string{"libdl_android"}

func init() {
	RegisterJavaFuzzBuildComponents(android.InitRegistrationContext)
}

func RegisterJavaFuzzBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_fuzz", JavaFuzzFactory)
	ctx.RegisterParallelSingletonType("java_fuzz_packaging", javaFuzzPackagingFactory)
}

type JavaFuzzTest struct {
	Test
	fuzzPackagedModule fuzz.FuzzPackagedModule
	jniFilePaths       android.Paths
}

// java_fuzz builds and links sources into a `.jar` file for the device.
// This generates .class files in a jar which can then be instrumented before
// fuzzing in Android Runtime (ART: Android OS on emulator or device)
func JavaFuzzFactory() android.Module {
	module := &JavaFuzzTest{}

	module.addHostAndDeviceProperties()
	module.AddProperties(&module.testProperties)
	module.AddProperties(&module.fuzzPackagedModule.FuzzProperties)

	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.Module.dexpreopter.isTest = true
	module.Module.linter.properties.Lint.Test = proptools.BoolPtr(true)

	android.AddLoadHook(module, func(ctx android.LoadHookContext) {
		disableLinuxBionic := struct {
			Target struct {
				Linux_bionic struct {
					Enabled *bool
				}
			}
		}{}
		disableLinuxBionic.Target.Linux_bionic.Enabled = proptools.BoolPtr(false)
		ctx.AppendProperties(&disableLinuxBionic)
	})

	InitJavaModuleMultiTargets(module, android.HostAndDeviceSupported)
	return module
}

func (j *JavaFuzzTest) DepsMutator(ctx android.BottomUpMutatorContext) {
	if j.Os().Class.String() == deviceString {
		j.testProperties.Jni_libs = append(j.testProperties.Jni_libs, artDeps...)
	}

	if len(j.testProperties.Jni_libs) > 0 {
		if j.fuzzPackagedModule.FuzzProperties.Fuzz_config == nil {
			config := &fuzz.FuzzConfig{}
			j.fuzzPackagedModule.FuzzProperties.Fuzz_config = config
		}
		// this will be used by the ingestion pipeline to determine the version
		// of jazzer to add to the fuzzer package
		j.fuzzPackagedModule.FuzzProperties.Fuzz_config.IsJni = proptools.BoolPtr(true)
		for _, target := range ctx.MultiTargets() {
			sharedLibVariations := append(target.Variations(), blueprint.Variation{Mutator: "link", Variation: "shared"})
			ctx.AddFarVariationDependencies(sharedLibVariations, jniLibTag, j.testProperties.Jni_libs...)
		}
	}

	j.deps(ctx)
}

func (j *JavaFuzzTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if j.fuzzPackagedModule.FuzzProperties.Corpus != nil {
		j.fuzzPackagedModule.Corpus = android.PathsForModuleSrc(ctx, j.fuzzPackagedModule.FuzzProperties.Corpus)
	}
	if j.fuzzPackagedModule.FuzzProperties.Data != nil {
		j.fuzzPackagedModule.Data = android.PathsForModuleSrc(ctx, j.fuzzPackagedModule.FuzzProperties.Data)
	}
	if j.fuzzPackagedModule.FuzzProperties.Dictionary != nil {
		j.fuzzPackagedModule.Dictionary = android.PathForModuleSrc(ctx, *j.fuzzPackagedModule.FuzzProperties.Dictionary)
	}
	if j.fuzzPackagedModule.FuzzProperties.Fuzz_config != nil {
		configPath := android.PathForModuleOut(ctx, "config").Join(ctx, "config.json")
		android.WriteFileRule(ctx, configPath, j.fuzzPackagedModule.FuzzProperties.Fuzz_config.String())
		j.fuzzPackagedModule.Config = configPath
	}

	_, sharedDeps := cc.CollectAllSharedDependencies(ctx)
	for _, dep := range sharedDeps {
		sharedLibInfo := ctx.OtherModuleProvider(dep, cc.SharedLibraryInfoProvider).(cc.SharedLibraryInfo)
		if sharedLibInfo.SharedLibrary != nil {
			arch := "lib"
			if sharedLibInfo.Target.Arch.ArchType.Multilib == "lib64" {
				arch = "lib64"
			}

			libPath := android.PathForModuleOut(ctx, filepath.Join(arch, sharedLibInfo.SharedLibrary.Base()))
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.Cp,
				Input:  sharedLibInfo.SharedLibrary,
				Output: libPath,
			})
			j.jniFilePaths = append(j.jniFilePaths, libPath)
		} else {
			ctx.PropertyErrorf("jni_libs", "%q of type %q is not supported", dep.Name(), ctx.OtherModuleType(dep))
		}

	}

	j.Test.GenerateAndroidBuildActions(ctx)
}

type javaFuzzPackager struct {
	fuzz.FuzzPackager
}

func javaFuzzPackagingFactory() android.Singleton {
	return &javaFuzzPackager{}
}

func (s *javaFuzzPackager) GenerateBuildActions(ctx android.SingletonContext) {
	// Map between each architecture + host/device combination.
	archDirs := make(map[fuzz.ArchOs][]fuzz.FileToZip)

	s.FuzzTargets = make(map[string]bool)
	ctx.VisitAllModules(func(module android.Module) {
		// Discard non-fuzz targets.
		javaFuzzModule, ok := module.(*JavaFuzzTest)
		if !ok {
			return
		}

		hostOrTargetString := "target"
		if javaFuzzModule.Target().HostCross {
			hostOrTargetString = "host_cross"
		} else if javaFuzzModule.Host() {
			hostOrTargetString = "host"
		}

		fuzzModuleValidator := fuzz.FuzzModule{
			javaFuzzModule.ModuleBase,
			javaFuzzModule.DefaultableModuleBase,
			javaFuzzModule.ApexModuleBase,
		}

		if ok := fuzz.IsValid(fuzzModuleValidator); !ok {
			return
		}

		archString := javaFuzzModule.Arch().ArchType.String()
		archDir := android.PathForIntermediates(ctx, "fuzz", hostOrTargetString, archString)
		archOs := fuzz.ArchOs{HostOrTarget: hostOrTargetString, Arch: archString, Dir: archDir.String()}

		var files []fuzz.FileToZip
		builder := android.NewRuleBuilder(pctx, ctx)

		// Package the artifacts (data, corpus, config and dictionary) into a zipfile.
		files = s.PackageArtifacts(ctx, module, javaFuzzModule.fuzzPackagedModule, archDir, builder)

		// Add .jar
		if !javaFuzzModule.Host() {
			files = append(files, fuzz.FileToZip{SourceFilePath: javaFuzzModule.implementationJarFile, DestinationPathPrefix: "classes"})
		}

		files = append(files, fuzz.FileToZip{SourceFilePath: javaFuzzModule.outputFile})

		// Add jni .so files
		for _, fPath := range javaFuzzModule.jniFilePaths {
			files = append(files, fuzz.FileToZip{SourceFilePath: fPath})
		}

		archDirs[archOs], ok = s.BuildZipFile(ctx, module, javaFuzzModule.fuzzPackagedModule, files, builder, archDir, archString, hostOrTargetString, archOs, archDirs)
		if !ok {
			return
		}
	})
	s.CreateFuzzPackage(ctx, archDirs, fuzz.Java, pctx)
}

func (s *javaFuzzPackager) MakeVars(ctx android.MakeVarsContext) {
	packages := s.Packages.Strings()
	sort.Strings(packages)
	ctx.Strict("SOONG_JAVA_FUZZ_PACKAGING_ARCH_MODULES", strings.Join(packages, " "))
	// Preallocate the slice of fuzz targets to minimize memory allocations.
	s.PreallocateSlice(ctx, "ALL_JAVA_FUZZ_TARGETS")
}
