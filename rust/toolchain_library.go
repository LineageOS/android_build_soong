//
// Copyright (C) 2021 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package rust

import (
	"path"

	"android/soong/android"
	"android/soong/rust/config"
)

// This module is used to compile the rust toolchain libraries
// When RUST_PREBUILTS_VERSION is set, the library will generated
// from the given Rust version.
func init() {
	android.RegisterModuleType("rust_toolchain_library",
		rustToolchainLibraryFactory)
	android.RegisterModuleType("rust_toolchain_library_rlib",
		rustToolchainLibraryRlibFactory)
	android.RegisterModuleType("rust_toolchain_library_dylib",
		rustToolchainLibraryDylibFactory)
}

type toolchainLibraryProperties struct {
	// path to the toolchain source, relative to the top of the toolchain source
	Toolchain_src *string `android:"arch_variant"`
}

type toolchainLibraryDecorator struct {
	*libraryDecorator
	Properties toolchainLibraryProperties
}

// rust_toolchain_library produces all rust variants.
func rustToolchainLibraryFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyRust()

	return initToolchainLibrary(module, library)
}

// rust_toolchain_library_dylib produces a dylib.
func rustToolchainLibraryDylibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyDylib()

	return initToolchainLibrary(module, library)
}

// rust_toolchain_library_rlib produces an rlib.
func rustToolchainLibraryRlibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyRlib()

	return initToolchainLibrary(module, library)
}

func initToolchainLibrary(module *Module, library *libraryDecorator) android.Module {
	toolchainLibrary := &toolchainLibraryDecorator{
		libraryDecorator: library,
	}
	module.compiler = toolchainLibrary
	module.AddProperties(&toolchainLibrary.Properties)
	android.AddLoadHook(module, rustSetToolchainSource)

	return module.Init()
}

func rustSetToolchainSource(ctx android.LoadHookContext) {
	if toolchainLib, ok := ctx.Module().(*Module).compiler.(*toolchainLibraryDecorator); ok {
		prefix := "linux-x86/" + GetRustPrebuiltVersion(ctx)
		newSrcs := []string{path.Join(prefix, android.String(toolchainLib.Properties.Toolchain_src))}

		type props struct {
			Srcs []string
		}
		p := &props{}
		p.Srcs = newSrcs
		ctx.AppendProperties(p)

	} else {
		ctx.ModuleErrorf("Called rustSetToolchainSource on a non-Rust Module.")
	}
}

// GetRustPrebuiltVersion returns the RUST_PREBUILTS_VERSION env var, or the default version if it is not defined.
func GetRustPrebuiltVersion(ctx android.LoadHookContext) string {
	return ctx.AConfig().GetenvWithDefault("RUST_PREBUILTS_VERSION", config.RustDefaultVersion)
}
