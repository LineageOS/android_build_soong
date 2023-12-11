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

package java

import (
	"android/soong/android"
)

type GeneratedJavaLibraryModule struct {
	Library
	callbacks  GeneratedJavaLibraryCallbacks
	moduleName string

	// true if we've already called DepsMutator. Can't call AddLibrary or AddSharedLibrary
	// after DepsMutator.
	depsMutatorDone bool
}

type GeneratedJavaLibraryCallbacks interface {
	// Called from inside DepsMutator, gives a chance to AddDependencies
	DepsMutator(module *GeneratedJavaLibraryModule, ctx android.BottomUpMutatorContext)

	// Called from inside GenerateAndroidBuildActions. Add the build rules to
	// make the srcjar, and return the path to it.
	GenerateSourceJarBuildActions(module *GeneratedJavaLibraryModule, ctx android.ModuleContext) android.Path
}

// GeneratedJavaLibraryModuleFactory provides a utility for modules that are generated
// source code, including ones outside the java package to build jar files
// from that generated source.
//
// To use GeneratedJavaLibraryModule, call GeneratedJavaLibraryModuleFactory with
// a callback interface and a properties object to add to the module.
//
// These modules will have some properties blocked, and it will be an error if
// modules attempt to set them. See the list of property names in GeneratedAndroidBuildActions
// for the list of those properties.
func GeneratedJavaLibraryModuleFactory(moduleName string, callbacks GeneratedJavaLibraryCallbacks, properties interface{}) android.Module {
	module := &GeneratedJavaLibraryModule{
		callbacks:  callbacks,
		moduleName: moduleName,
	}
	module.addHostAndDeviceProperties()
	module.initModuleAndImport(module)
	android.InitApexModule(module)
	InitJavaModule(module, android.HostAndDeviceSupported)
	if properties != nil {
		module.AddProperties(properties)
	}
	return module
}

// Add a java shared library as a dependency, as if they had said `libs: [ "name" ]`
func (module *GeneratedJavaLibraryModule) AddSharedLibrary(name string) {
	if module.depsMutatorDone {
		panic("GeneratedJavaLibraryModule.AddLibrary called after DepsMutator")
	}
	module.Library.properties.Libs = append(module.Library.properties.Libs, name)
}

// Add a java shared library as a dependency, as if they had said `libs: [ "name" ]`
func (module *GeneratedJavaLibraryModule) AddStaticLibrary(name string) {
	if module.depsMutatorDone {
		panic("GeneratedJavaLibraryModule.AddStaticLibrary called after DepsMutator")
	}
	module.Library.properties.Static_libs = append(module.Library.properties.Static_libs, name)
}

func (module *GeneratedJavaLibraryModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	module.callbacks.DepsMutator(module, ctx)
	module.depsMutatorDone = true
	module.Library.DepsMutator(ctx)
}

func checkPropertyEmpty(ctx android.ModuleContext, module *GeneratedJavaLibraryModule, name string, value []string) {
	if len(value) != 0 {
		ctx.PropertyErrorf(name, "%s not allowed on %s", name, module.moduleName)
	}
}

func (module *GeneratedJavaLibraryModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// These modules are all-generated, so disallow these properties to keep it simple.
	// No additional sources
	checkPropertyEmpty(ctx, module, "srcs", module.Library.properties.Srcs)
	checkPropertyEmpty(ctx, module, "common_srcs", module.Library.properties.Common_srcs)
	checkPropertyEmpty(ctx, module, "exclude_srcs", module.Library.properties.Exclude_srcs)
	checkPropertyEmpty(ctx, module, "java_resource_dirs", module.Library.properties.Java_resource_dirs)
	checkPropertyEmpty(ctx, module, "exclude_java_resource_dirs", module.Library.properties.Exclude_java_resource_dirs)
	// Restrict these for no good reason other than to limit the surface area. If there's a
	// good use case put them back.
	checkPropertyEmpty(ctx, module, "plugins", module.Library.properties.Plugins)
	checkPropertyEmpty(ctx, module, "exported_plugins", module.Library.properties.Exported_plugins)

	srcJarPath := module.callbacks.GenerateSourceJarBuildActions(module, ctx)
	module.Library.properties.Generated_srcjars = append(module.Library.properties.Generated_srcjars, srcJarPath)
	module.Library.GenerateAndroidBuildActions(ctx)
}
