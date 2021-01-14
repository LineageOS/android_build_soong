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
	"strings"

	"android/soong/android"
)

var (
	llndkLibrarySuffix = ".llndk"
	llndkHeadersSuffix = ".llndk"
)

// Holds properties to describe a stub shared library based on the provided version file.
// The stub library will actually be built by the cc_library module that points to this
// module with the llndk_stubs property.
// TODO(ccross): move the properties from llndk_library modules directly into the cc_library
//  modules and remove the llndk_library modules.
//
// Example:
//
// llndk_library {
//     name: "libfoo",
//     symbol_file: "libfoo.map.txt",
//     export_include_dirs: ["include_vndk"],
// }
//
type llndkLibraryProperties struct {
	// Relative path to the symbol map.
	// An example file can be seen here: TODO(danalbert): Make an example.
	Symbol_file *string

	// Whether to export any headers as -isystem instead of -I. Mainly for use by
	// bionic/libc.
	Export_headers_as_system *bool

	// Which headers to process with versioner. This really only handles
	// bionic/libc/include right now.
	Export_preprocessed_headers []string

	// Whether the system library uses symbol versions.
	Unversioned *bool

	// list of llndk headers to re-export include directories from.
	Export_llndk_headers []string `android:"arch_variant"`

	// whether this module can be directly depended upon by libs that are installed
	// to /vendor and /product.
	// When set to true, this module can only be depended on by VNDK libraries, not
	// vendor nor product libraries. This effectively hides this module from
	// non-system modules. Default value is false.
	Private *bool
}

type llndkStubDecorator struct {
	*libraryDecorator

	Properties llndkLibraryProperties
}

var _ versionedInterface = (*llndkStubDecorator)(nil)

func (stub *llndkStubDecorator) compilerFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags {
	return flags
}

func (stub *llndkStubDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	return Objects{}
}

func (stub *llndkStubDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	return deps
}

func (stub *llndkStubDecorator) Name(name string) string {
	if strings.HasSuffix(name, llndkLibrarySuffix) {
		return name
	}
	return name + llndkLibrarySuffix
}

func (stub *llndkStubDecorator) linkerProps() []interface{} {
	props := stub.libraryDecorator.linkerProps()
	return append(props, &stub.Properties)
}

func (stub *llndkStubDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	stub.libraryDecorator.libName = stub.implementationModuleName(ctx.ModuleName())
	return stub.libraryDecorator.linkerFlags(ctx, flags)
}

func (stub *llndkStubDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps,
	objs Objects) android.Path {
	return nil
}

func (stub *llndkStubDecorator) nativeCoverage() bool {
	return false
}

func (stub *llndkStubDecorator) implementationModuleName(name string) string {
	return strings.TrimSuffix(name, llndkLibrarySuffix)
}

func (stub *llndkStubDecorator) buildStubs() bool {
	return true
}

func NewLLndkStubLibrary() *Module {
	module, library := NewLibrary(android.DeviceSupported)
	module.stl = nil
	module.sanitize = nil
	library.disableStripping()

	stub := &llndkStubDecorator{
		libraryDecorator: library,
	}
	module.compiler = stub
	module.linker = stub
	module.installer = nil
	module.library = stub

	return module
}

// llndk_library creates a stub llndk shared library based on the provided
// version file. Example:
//
//    llndk_library {
//        name: "libfoo",
//        symbol_file: "libfoo.map.txt",
//        export_include_dirs: ["include_vndk"],
//    }
func LlndkLibraryFactory() android.Module {
	module := NewLLndkStubLibrary()
	return module.Init()
}

// isVestigialLLNDKModule returns true if m is a vestigial llndk_library module used to provide
// properties to the LLNDK variant of a cc_library.
func isVestigialLLNDKModule(m *Module) bool {
	_, ok := m.linker.(*llndkStubDecorator)
	return ok
}

type llndkHeadersDecorator struct {
	*libraryDecorator
}

func (llndk *llndkHeadersDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps.HeaderLibs = append(deps.HeaderLibs, llndk.Properties.Llndk.Export_llndk_headers...)
	deps.ReexportHeaderLibHeaders = append(deps.ReexportHeaderLibHeaders,
		llndk.Properties.Llndk.Export_llndk_headers...)
	return deps
}

// llndk_headers contains a set of c/c++ llndk headers files which are imported
// by other soongs cc modules.
func llndkHeadersFactory() android.Module {
	module, library := NewLibrary(android.DeviceSupported)
	library.HeaderOnly()
	module.stl = nil
	module.sanitize = nil

	decorator := &llndkHeadersDecorator{
		libraryDecorator: library,
	}

	module.compiler = nil
	module.linker = decorator
	module.installer = nil
	module.library = decorator

	module.Init()

	return module
}

func init() {
	android.RegisterModuleType("llndk_library", LlndkLibraryFactory)
	android.RegisterModuleType("llndk_headers", llndkHeadersFactory)
}
