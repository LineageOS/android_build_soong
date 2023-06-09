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
	"android/soong/java"
	"fmt"
	"github.com/google/blueprint"
)

type definitionsTagType struct {
	blueprint.BaseDependencyTag
}

var definitionsTag = definitionsTagType{}

type JavaDeviceConfigDefinitionsLibraryProperties struct {
	// name of the device_config_definitions module to generate a library for
	Device_config_definitions string
}

type JavaDeviceConfigDefinitionsLibraryCallbacks struct {
	properties JavaDeviceConfigDefinitionsLibraryProperties
}

func JavaDefinitionsLibraryFactory() android.Module {
	callbacks := &JavaDeviceConfigDefinitionsLibraryCallbacks{}
	return java.GeneratedJavaLibraryModuleFactory("java_device_config_definitions_library", callbacks, &callbacks.properties)
}

func (callbacks *JavaDeviceConfigDefinitionsLibraryCallbacks) DepsMutator(module *java.GeneratedJavaLibraryModule, ctx android.BottomUpMutatorContext) {
	definitions := callbacks.properties.Device_config_definitions
	if len(definitions) == 0 {
		// TODO: Add test for this case
		ctx.PropertyErrorf("device_config_definitions", "device_config_definitions property required")
	} else {
		ctx.AddDependency(ctx.Module(), definitionsTag, definitions)
	}
}

func (callbacks *JavaDeviceConfigDefinitionsLibraryCallbacks) GenerateSourceJarBuildActions(ctx android.ModuleContext) android.Path {
	// Get the values that came from the global RELEASE_DEVICE_CONFIG_VALUE_SETS flag
	definitionsModules := ctx.GetDirectDepsWithTag(definitionsTag)
	if len(definitionsModules) != 1 {
		panic(fmt.Errorf("Exactly one device_config_definitions property required"))
	}
	definitions := ctx.OtherModuleProvider(definitionsModules[0], definitionsProviderKey).(definitionsProviderData)

	srcJarPath := android.PathForModuleGen(ctx, ctx.ModuleName()+".srcjar")
	ctx.Build(pctx, android.BuildParams{
		Rule:        srcJarRule,
		Input:       definitions.intermediatePath,
		Output:      srcJarPath,
		Description: "device_config.srcjar",
	})

	return srcJarPath
}
