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

	"android/soong/android"
	"android/soong/fuzz"
)

func init() {
	RegisterJavaFuzzBuildComponents(android.InitRegistrationContext)
}

func RegisterJavaFuzzBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_fuzz_host", FuzzFactory)
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

	module.initModuleAndImport(module)
	android.InitSdkAwareModule(module)
	InitJavaModule(module, android.HostSupported)
	return module
}
