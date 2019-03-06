// Copyright 2019 Google Inc. All rights reserved.
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
	"android/soong/android"

	"github.com/google/blueprint"
)

type DeviceHostConverter struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties DeviceHostConverterProperties

	headerJars                    android.Paths
	implementationJars            android.Paths
	implementationAndResourceJars android.Paths
	resourceJars                  android.Paths
}

type DeviceHostConverterProperties struct {
	// List of modules whose contents will be visible to modules that depend on this module.
	Libs []string
}

type DeviceForHost struct {
	DeviceHostConverter
}

// java_device_for_host makes the classes.jar output of a device java_library module available to host
// java_library modules.
//
// It is rarely necessary, and its used is restricted to a few whitelisted projects.
func DeviceForHostFactory() android.Module {
	module := &DeviceForHost{}

	module.AddProperties(&module.properties)

	InitJavaModule(module, android.HostSupported)
	return module
}

type HostForDevice struct {
	DeviceHostConverter
}

// java_host_for_device makes the classes.jar output of a host java_library module available to device
// java_library modules.
//
// It is rarely necessary, and its used is restricted to a few whitelisted projects.
func HostForDeviceFactory() android.Module {
	module := &HostForDevice{}

	module.AddProperties(&module.properties)

	InitJavaModule(module, android.DeviceSupported)
	return module
}

var deviceHostConverterDepTag = dependencyTag{name: "device_host_converter"}

func (d *DeviceForHost) DepsMutator(ctx android.BottomUpMutatorContext) {
	variation := []blueprint.Variation{{Mutator: "arch", Variation: "android_common"}}
	ctx.AddFarVariationDependencies(variation, deviceHostConverterDepTag, d.properties.Libs...)
}

func (d *HostForDevice) DepsMutator(ctx android.BottomUpMutatorContext) {
	variation := []blueprint.Variation{{Mutator: "arch", Variation: ctx.Config().BuildOsCommonVariant}}
	ctx.AddFarVariationDependencies(variation, deviceHostConverterDepTag, d.properties.Libs...)
}

func (d *DeviceHostConverter) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(d.properties.Libs) < 1 {
		ctx.PropertyErrorf("libs", "at least one dependency is required")
	}

	ctx.VisitDirectDepsWithTag(deviceHostConverterDepTag, func(m android.Module) {
		if dep, ok := m.(Dependency); ok {
			d.headerJars = append(d.headerJars, dep.HeaderJars()...)
			d.implementationJars = append(d.implementationJars, dep.ImplementationJars()...)
			d.implementationAndResourceJars = append(d.implementationAndResourceJars, dep.ImplementationAndResourcesJars()...)
			d.resourceJars = append(d.resourceJars, dep.ResourceJars()...)
		} else {
			ctx.PropertyErrorf("libs", "module %q cannot be used as a dependency", ctx.OtherModuleName(m))
		}
	})
}

var _ Dependency = (*DeviceHostConverter)(nil)

func (d *DeviceHostConverter) HeaderJars() android.Paths {
	return d.headerJars
}

func (d *DeviceHostConverter) ImplementationJars() android.Paths {
	return d.implementationJars
}

func (d *DeviceHostConverter) ResourceJars() android.Paths {
	return d.resourceJars
}

func (d *DeviceHostConverter) ImplementationAndResourcesJars() android.Paths {
	return d.implementationAndResourceJars
}

func (d *DeviceHostConverter) DexJar() android.Path {
	return nil
}

func (d *DeviceHostConverter) AidlIncludeDirs() android.Paths {
	return nil
}

func (d *DeviceHostConverter) ExportedSdkLibs() []string {
	return nil
}
