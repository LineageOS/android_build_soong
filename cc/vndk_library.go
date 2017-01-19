// Copyright 2017 Google Inc. All rights reserved.
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
	"github.com/google/blueprint"

	"android/soong/android"
	"android/soong/cc/config"
)

type vndkExtLibraryProperties struct {
	// Name of the VNDK library module that this VNDK-ext library is extending.
	// This library will have the same file name and soname as the original VNDK
	// library, but will be installed in /system/lib/vndk-ext rather
	// than /system/lib/vndk.
	Extends string
}

type vndkExtLibraryDecorator struct {
	*libraryDecorator

	properties vndkExtLibraryProperties
}

func init() {
	android.RegisterModuleType("vndk_ext_library", vndkExtLibraryFactory)
}

func (deco *vndkExtLibraryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	extends := deco.properties.Extends
	if extends != "" {
		if config.IsVndkLibrary(extends) {
			// TODO(jiyong): ensure that the module referenced by 'extends' exists. Don't know how...
			// Adding a dependency was not successful because it leads to circular dependency
			// in between this and the 'extends' module.
			// Ideally, this should be something like follows:
			// otherCtx = findModuleByName(deco.properties.Extends)
			// if otherCtx != nil && otherCtx.isVndk() {
			//     deco.libaryDecorator.libName = otherCtx.getBaseName()
			// }
			deco.libraryDecorator.libName = extends
		} else {
			ctx.PropertyErrorf("extends", "%s should be a VNDK or VNDK-indirect library", extends)
		}
	} else {
		ctx.PropertyErrorf("extends", "missing. A VNDK-ext library must extend existing VNDK library")
	}
	return deco.libraryDecorator.linkerFlags(ctx, flags)
}

func (deco *vndkExtLibraryDecorator) install(ctx ModuleContext, file android.Path) {
	deco.libraryDecorator.baseInstaller.subDir = "vndk-ext"
	deco.libraryDecorator.baseInstaller.install(ctx, file)
}

func vndkExtLibraryFactory() (blueprint.Module, []interface{}) {
	module, library := NewLibrary(android.DeviceSupported)
	library.BuildOnlyShared()

	_, props := module.Init()

	deco := &vndkExtLibraryDecorator{
		libraryDecorator: library,
	}

	module.installer = deco
	module.linker = deco

	props = append(props, &deco.properties)

	return module, props
}
