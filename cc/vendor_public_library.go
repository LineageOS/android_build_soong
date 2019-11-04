// Copyright 2018 Google Inc. All rights reserved.
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
	"strings"
	"sync"

	"android/soong/android"
)

var (
	vendorPublicLibrarySuffix = ".vendorpublic"

	vendorPublicLibrariesKey  = android.NewOnceKey("vendorPublicLibraries")
	vendorPublicLibrariesLock sync.Mutex
)

func vendorPublicLibraries(config android.Config) *[]string {
	return config.Once(vendorPublicLibrariesKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

// Creates a stub shared library for a vendor public library. Vendor public libraries
// are vendor libraries (owned by them and installed to /vendor partition) that are
// exposed to Android apps via JNI. The libraries are made public by being listed in
// /vendor/etc/public.libraries.txt.
//
// This stub library is a build-time only artifact that provides symbols that are
// exposed from a vendor public library.
//
// Example:
//
// vendor_public_library {
//     name: "libfoo",
//     symbol_file: "libfoo.map.txt",
//     export_public_headers: ["libfoo_headers"],
// }
//
// cc_headers {
//     name: "libfoo_headers",
//     export_include_dirs: ["include"],
// }
//
type vendorPublicLibraryProperties struct {
	// Relative path to the symbol map.
	Symbol_file *string

	// Whether the system library uses symbol versions.
	Unversioned *bool

	// list of header libs to re-export include directories from.
	Export_public_headers []string `android:"arch_variant"`
}

type vendorPublicLibraryStubDecorator struct {
	*libraryDecorator

	Properties vendorPublicLibraryProperties

	versionScriptPath android.ModuleGenPath
}

func (stub *vendorPublicLibraryStubDecorator) Name(name string) string {
	return name + vendorPublicLibrarySuffix
}

func (stub *vendorPublicLibraryStubDecorator) compilerInit(ctx BaseModuleContext) {
	stub.baseCompiler.compilerInit(ctx)

	name := ctx.baseModuleName()
	if strings.HasSuffix(name, vendorPublicLibrarySuffix) {
		ctx.PropertyErrorf("name", "Do not append %q manually, just use the base name", vendorPublicLibrarySuffix)
	}

	vendorPublicLibrariesLock.Lock()
	defer vendorPublicLibrariesLock.Unlock()
	vendorPublicLibraries := vendorPublicLibraries(ctx.Config())
	for _, lib := range *vendorPublicLibraries {
		if lib == name {
			return
		}
	}
	*vendorPublicLibraries = append(*vendorPublicLibraries, name)
}

func (stub *vendorPublicLibraryStubDecorator) compilerFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags {
	flags = stub.baseCompiler.compilerFlags(ctx, flags, deps)
	return addStubLibraryCompilerFlags(flags)
}

func (stub *vendorPublicLibraryStubDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	objs, versionScript := compileStubLibrary(ctx, flags, String(stub.Properties.Symbol_file), "current", "")
	stub.versionScriptPath = versionScript
	return objs
}

func (stub *vendorPublicLibraryStubDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	headers := stub.Properties.Export_public_headers
	deps.HeaderLibs = append(deps.HeaderLibs, headers...)
	deps.ReexportHeaderLibHeaders = append(deps.ReexportHeaderLibHeaders, headers...)
	return deps
}

func (stub *vendorPublicLibraryStubDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	stub.libraryDecorator.libName = strings.TrimSuffix(ctx.ModuleName(), vendorPublicLibrarySuffix)
	return stub.libraryDecorator.linkerFlags(ctx, flags)
}

func (stub *vendorPublicLibraryStubDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps,
	objs Objects) android.Path {
	if !Bool(stub.Properties.Unversioned) {
		linkerScriptFlag := "-Wl,--version-script," + stub.versionScriptPath.String()
		flags.Local.LdFlags = append(flags.Local.LdFlags, linkerScriptFlag)
		flags.LdFlagsDeps = append(flags.LdFlagsDeps, stub.versionScriptPath)
	}
	return stub.libraryDecorator.link(ctx, flags, deps, objs)
}

// vendor_public_library creates a stub shared library for a vendor public
// library. This stub library is a build-time only artifact that provides
// symbols that are exposed from a vendor public library. Example:
//
//    vendor_public_library {
//        name: "libfoo",
//        symbol_file: "libfoo.map.txt",
//        export_public_headers: ["libfoo_headers"],
//    }
func vendorPublicLibraryFactory() android.Module {
	module, library := NewLibrary(android.DeviceSupported)
	library.BuildOnlyShared()
	module.stl = nil
	module.sanitize = nil
	library.StripProperties.Strip.None = BoolPtr(true)

	stub := &vendorPublicLibraryStubDecorator{
		libraryDecorator: library,
	}
	module.compiler = stub
	module.linker = stub
	module.installer = nil

	module.AddProperties(
		&stub.Properties,
		&library.MutatedProperties,
		&library.flagExporter.Properties)

	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibBoth)
	return module
}

func init() {
	android.RegisterModuleType("vendor_public_library", vendorPublicLibraryFactory)
}
