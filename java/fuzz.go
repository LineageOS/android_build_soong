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
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/fuzz"
)

type jniProperties struct {
	// list of jni libs
	Jni_libs []string

	// sanitization
	Sanitizers []string
}

func init() {
	RegisterJavaFuzzBuildComponents(android.InitRegistrationContext)
}

func RegisterJavaFuzzBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_fuzz_host", FuzzFactory)
	ctx.RegisterSingletonType("java_fuzz_packaging", javaFuzzPackagingFactory)
}

type JavaFuzzLibrary struct {
	Library
	fuzzPackagedModule fuzz.FuzzPackagedModule
	jniProperties      jniProperties
	jniFilePaths       android.Paths
}

// IsSanitizerEnabledForJni implemented to make JavaFuzzLibrary implement
// cc.JniSanitizeable. It returns a bool for whether a cc dependency should be
// sanitized for the given sanitizer or not.
func (j *JavaFuzzLibrary) IsSanitizerEnabledForJni(ctx android.BaseModuleContext, sanitizerName string) bool {
	// TODO: once b/231370928 is resolved, please uncomment the loop
	// 	for _, s := range j.jniProperties.Sanitizers {
	// 		if sanitizerName == s {
	// 			return true
	// 		}
	// 	}
	return false
}

func (j *JavaFuzzLibrary) DepsMutator(mctx android.BottomUpMutatorContext) {
	if len(j.jniProperties.Jni_libs) > 0 {
		if j.fuzzPackagedModule.FuzzProperties.Fuzz_config == nil {
			config := &fuzz.FuzzConfig{}
			j.fuzzPackagedModule.FuzzProperties.Fuzz_config = config
		}
		// this will be used by the ingestion pipeline to determine the version
		// of jazzer to add to the fuzzer package
		j.fuzzPackagedModule.FuzzProperties.Fuzz_config.IsJni = proptools.BoolPtr(true)

		for _, target := range mctx.MultiTargets() {
			sharedLibVariations := append(target.Variations(), blueprint.Variation{Mutator: "link", Variation: "shared"})
			mctx.AddFarVariationDependencies(sharedLibVariations, cc.JniFuzzLibTag, j.jniProperties.Jni_libs...)
		}
	}
	j.Library.DepsMutator(mctx)
}

func (j *JavaFuzzLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
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

	ctx.VisitDirectDepsWithTag(cc.JniFuzzLibTag, func(dep android.Module) {
		sharedLibInfo := ctx.OtherModuleProvider(dep, cc.SharedLibraryInfoProvider).(cc.SharedLibraryInfo)
		if sharedLibInfo.SharedLibrary != nil {
			libPath := android.PathForModuleOut(ctx, sharedLibInfo.SharedLibrary.Base())
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.Cp,
				Input:  sharedLibInfo.SharedLibrary,
				Output: libPath,
			})
			j.jniFilePaths = append(j.jniFilePaths, libPath)
		} else {
			ctx.PropertyErrorf("jni_libs", "%q of type %q is not supported", dep.Name(), ctx.OtherModuleType(dep))
		}
	})

	j.Library.GenerateAndroidBuildActions(ctx)
}

// java_fuzz builds and links sources into a `.jar` file for the host.
//
// By default, a java_fuzz produces a `.jar` file containing `.class` files.
// This jar is not suitable for installing on a device.
func FuzzFactory() android.Module {
	module := &JavaFuzzLibrary{}

	module.addHostProperties()
	module.AddProperties(&module.jniProperties)
	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.AddProperties(&module.fuzzPackagedModule.FuzzProperties)

	// java_fuzz packaging rules collide when both linux_glibc and linux_bionic are enabled, disable the linux_bionic variants.
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

	module.initModuleAndImport(module)
	android.InitSdkAwareModule(module)
	InitJavaModuleMultiTargets(module, android.HostSupported)
	return module
}

// Responsible for generating rules that package fuzz targets into
// their architecture & target/host specific zip file.
type javaFuzzPackager struct {
	fuzz.FuzzPackager
}

func javaFuzzPackagingFactory() android.Singleton {
	return &javaFuzzPackager{}
}

func (s *javaFuzzPackager) GenerateBuildActions(ctx android.SingletonContext) {
	// Map between each architecture + host/device combination.
	archDirs := make(map[fuzz.ArchOs][]fuzz.FileToZip)

	// List of individual fuzz targets.
	s.FuzzTargets = make(map[string]bool)

	ctx.VisitAllModules(func(module android.Module) {
		// Discard non-fuzz targets.
		javaFuzzModule, ok := module.(*JavaFuzzLibrary)
		if !ok {
			return
		}

		if javaFuzzModule.Target().HostCross {
			return
		}

		fuzzModuleValidator := fuzz.FuzzModule{
			javaFuzzModule.ModuleBase,
			javaFuzzModule.DefaultableModuleBase,
			javaFuzzModule.ApexModuleBase,
		}

		if ok := fuzz.IsValid(fuzzModuleValidator); !ok {
			return
		}

		hostOrTargetString := "target"
		if javaFuzzModule.Host() {
			hostOrTargetString = "host"
		}
		archString := javaFuzzModule.Arch().ArchType.String()

		archDir := android.PathForIntermediates(ctx, "fuzz", hostOrTargetString, archString)
		archOs := fuzz.ArchOs{HostOrTarget: hostOrTargetString, Arch: archString, Dir: archDir.String()}

		var files []fuzz.FileToZip
		builder := android.NewRuleBuilder(pctx, ctx)

		// Package the artifacts (data, corpus, config and dictionary into a zipfile.
		files = s.PackageArtifacts(ctx, module, javaFuzzModule.fuzzPackagedModule, archDir, builder)

		// Add .jar
		files = append(files, fuzz.FileToZip{javaFuzzModule.outputFile, ""})

		// Add jni .so files
		for _, fPath := range javaFuzzModule.jniFilePaths {
			files = append(files, fuzz.FileToZip{fPath, ""})
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
