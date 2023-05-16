// Copyright 2023 Google Inc. All rights reserved.
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

package device_config

import (
	"android/soong/android"
	"github.com/google/blueprint"
)

// Properties for "device_config_value"
type ValuesModule struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties struct {
		// aconfig files, relative to this Android.bp file
		Srcs []string `android:"path"`

		// Release config flag namespace
		Namespace string
	}
}

func ValuesFactory() android.Module {
	module := &ValuesModule{}

	android.InitAndroidModule(module)
	android.InitDefaultableModule(module)
	module.AddProperties(&module.properties)
	// TODO: bp2build
	//android.InitBazelModule(module)

	return module
}

// Provider published by device_config_value_set
type valuesProviderData struct {
	// The namespace that this values module values
	Namespace string

	// The values aconfig files, relative to the root of the tree
	Values android.Paths
}

var valuesProviderKey = blueprint.NewProvider(valuesProviderData{})

func (module *ValuesModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(module.properties.Namespace) == 0 {
		ctx.PropertyErrorf("namespace", "missing namespace property")
	}

	// Provide the our source files list to the device_config_value_set as a list of files
	providerData := valuesProviderData{
		Namespace: module.properties.Namespace,
		Values:    android.PathsForModuleSrc(ctx, module.properties.Srcs),
	}
	ctx.SetProvider(valuesProviderKey, providerData)
}
