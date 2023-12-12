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
	"fmt"
	"io"

	"android/soong/android"
	"android/soong/dexpreopt"
)

type DeviceHostConverter struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties DeviceHostConverterProperties

	headerJars                    android.Paths
	implementationJars            android.Paths
	implementationAndResourceJars android.Paths
	resourceJars                  android.Paths

	srcJarArgs []string
	srcJarDeps android.Paths

	combinedHeaderJar         android.Path
	combinedImplementationJar android.Path
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
// It is rarely necessary, and its usage is restricted to a few allowed projects.
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
// It is rarely necessary, and its usage is restricted to a few allowed projects.
func HostForDeviceFactory() android.Module {
	module := &HostForDevice{}

	module.AddProperties(&module.properties)

	InitJavaModule(module, android.DeviceSupported)
	return module
}

var deviceHostConverterDepTag = dependencyTag{name: "device_host_converter"}

func (d *DeviceForHost) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddFarVariationDependencies(ctx.Config().AndroidCommonTarget.Variations(),
		deviceHostConverterDepTag, d.properties.Libs...)
}

func (d *HostForDevice) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddFarVariationDependencies(ctx.Config().BuildOSCommonTarget.Variations(),
		deviceHostConverterDepTag, d.properties.Libs...)
}

func (d *DeviceHostConverter) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(d.properties.Libs) < 1 {
		ctx.PropertyErrorf("libs", "at least one dependency is required")
	}

	ctx.VisitDirectDepsWithTag(deviceHostConverterDepTag, func(m android.Module) {
		if ctx.OtherModuleHasProvider(m, JavaInfoProvider) {
			dep := ctx.OtherModuleProvider(m, JavaInfoProvider).(JavaInfo)
			d.headerJars = append(d.headerJars, dep.HeaderJars...)
			d.implementationJars = append(d.implementationJars, dep.ImplementationJars...)
			d.implementationAndResourceJars = append(d.implementationAndResourceJars, dep.ImplementationAndResourcesJars...)
			d.resourceJars = append(d.resourceJars, dep.ResourceJars...)

			d.srcJarArgs = append(d.srcJarArgs, dep.SrcJarArgs...)
			d.srcJarDeps = append(d.srcJarDeps, dep.SrcJarDeps...)
		} else {
			ctx.PropertyErrorf("libs", "module %q cannot be used as a dependency", ctx.OtherModuleName(m))
		}
	})

	jarName := ctx.ModuleName() + ".jar"

	if len(d.implementationAndResourceJars) > 1 {
		outputFile := android.PathForModuleOut(ctx, "combined", jarName)
		TransformJarsToJar(ctx, outputFile, "combine", d.implementationAndResourceJars,
			android.OptionalPath{}, false, nil, nil)
		d.combinedImplementationJar = outputFile
	} else if len(d.implementationAndResourceJars) == 1 {
		d.combinedImplementationJar = d.implementationAndResourceJars[0]
	}

	if len(d.headerJars) > 1 {
		outputFile := android.PathForModuleOut(ctx, "turbine-combined", jarName)
		TransformJarsToJar(ctx, outputFile, "turbine combine", d.headerJars,
			android.OptionalPath{}, false, nil, []string{"META-INF/TRANSITIVE"})
		d.combinedHeaderJar = outputFile
	} else if len(d.headerJars) == 1 {
		d.combinedHeaderJar = d.headerJars[0]
	}

	ctx.SetProvider(JavaInfoProvider, JavaInfo{
		HeaderJars:                     d.headerJars,
		ImplementationAndResourcesJars: d.implementationAndResourceJars,
		ImplementationJars:             d.implementationJars,
		ResourceJars:                   d.resourceJars,
		SrcJarArgs:                     d.srcJarArgs,
		SrcJarDeps:                     d.srcJarDeps,
		// TODO: Not sure if aconfig flags that have been moved between device and host variants
		// make sense.
	})

}

func (d *DeviceHostConverter) HeaderJars() android.Paths {
	return d.headerJars
}

func (d *DeviceHostConverter) ImplementationAndResourcesJars() android.Paths {
	return d.implementationAndResourceJars
}

func (d *DeviceHostConverter) DexJarBuildPath() android.Path {
	return nil
}

func (d *DeviceHostConverter) DexJarInstallPath() android.Path {
	return nil
}

func (d *DeviceHostConverter) AidlIncludeDirs() android.Paths {
	return nil
}

func (d *DeviceHostConverter) ClassLoaderContexts() dexpreopt.ClassLoaderContextMap {
	return nil
}

func (d *DeviceHostConverter) JacocoReportClassesFile() android.Path {
	return nil
}

func (d *DeviceHostConverter) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(d.combinedImplementationJar),
		// Make does not support Windows Java modules
		Disabled: d.Os() == android.Windows,
		Include:  "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
				fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", d.combinedHeaderJar.String())
				fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", d.combinedImplementationJar.String())
			},
		},
	}
}
