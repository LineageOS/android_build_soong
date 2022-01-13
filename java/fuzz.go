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
	"github.com/google/blueprint/proptools"
	"sort"
	"strings"

	"android/soong/android"
	"android/soong/fuzz"
)

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
}

func (j *JavaFuzzLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	j.Library.GenerateAndroidBuildActions(ctx)

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
}

// java_fuzz builds and links sources into a `.jar` file for the host.
//
// By default, a java_fuzz produces a `.jar` file containing `.class` files.
// This jar is not suitable for installing on a device.
func FuzzFactory() android.Module {
	module := &JavaFuzzLibrary{}

	module.addHostProperties()
	module.Module.properties.Installable = proptools.BoolPtr(false)
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
	InitJavaModule(module, android.HostSupported)
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
		javaModule, ok := module.(*JavaFuzzLibrary)
		if !ok {
			return
		}

		fuzzModuleValidator := fuzz.FuzzModule{
			javaModule.ModuleBase,
			javaModule.DefaultableModuleBase,
			javaModule.ApexModuleBase,
		}

		if ok := fuzz.IsValid(fuzzModuleValidator); !ok || *javaModule.Module.properties.Installable {
			return
		}

		hostOrTargetString := "target"
		if javaModule.Host() {
			hostOrTargetString = "host"
		}
		archString := javaModule.Arch().ArchType.String()

		archDir := android.PathForIntermediates(ctx, "fuzz", hostOrTargetString, archString)
		archOs := fuzz.ArchOs{HostOrTarget: hostOrTargetString, Arch: archString, Dir: archDir.String()}

		var files []fuzz.FileToZip
		builder := android.NewRuleBuilder(pctx, ctx)

		// Package the artifacts (data, corpus, config and dictionary into a zipfile.
		files = s.PackageArtifacts(ctx, module, javaModule.fuzzPackagedModule, archDir, builder)

		// Add .jar
		files = append(files, fuzz.FileToZip{javaModule.outputFile, ""})

		archDirs[archOs], ok = s.BuildZipFile(ctx, module, javaModule.fuzzPackagedModule, files, builder, archDir, archString, "host", archOs, archDirs)
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
