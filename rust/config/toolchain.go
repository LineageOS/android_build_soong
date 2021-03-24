// Copyright 2019 The Android Open Source Project
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

package config

import (
	"android/soong/android"
)

type Toolchain interface {
	RustTriple() string
	ToolchainRustFlags() string
	ToolchainLinkFlags() string

	SharedLibSuffix() string
	StaticLibSuffix() string
	RlibSuffix() string
	DylibSuffix() string
	ProcMacroSuffix() string
	ExecutableSuffix() string

	Is64Bit() bool
	Supported() bool

	Bionic() bool

	LibclangRuntimeLibraryArch() string
}

type toolchainBase struct {
}

func (toolchainBase) RustTriple() string {
	panic("toolchainBase does not define a triple.")
}

func (toolchainBase) ToolchainRustFlags() string {
	panic("toolchainBase does not provide rust flags.")
}

func (toolchainBase) ToolchainLinkFlags() string {
	panic("toolchainBase does not provide link flags.")
}

func (toolchainBase) Is64Bit() bool {
	panic("toolchainBase cannot determine datapath width.")
}

func (toolchainBase) Bionic() bool {
	return true
}

type toolchain64Bit struct {
	toolchainBase
}

func (toolchain64Bit) Is64Bit() bool {
	return true
}

type toolchain32Bit struct {
	toolchainBase
}

func (toolchain32Bit) Is64Bit() bool {
	return false
}

func (toolchain32Bit) Bionic() bool {
	return true
}

func (toolchainBase) ExecutableSuffix() string {
	return ""
}

func (toolchainBase) SharedLibSuffix() string {
	return ".so"
}

func (toolchainBase) StaticLibSuffix() string {
	return ".a"
}

func (toolchainBase) RlibSuffix() string {
	return ".rlib"
}
func (toolchainBase) DylibSuffix() string {
	return ".dylib.so"
}

func (toolchainBase) ProcMacroSuffix() string {
	return ".so"
}

func (toolchainBase) Supported() bool {
	return false
}

func (toolchainBase) LibclangRuntimeLibraryArch() string {
	return ""
}

func BuiltinsRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "builtins")
}

func LibFuzzerRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "fuzzer")
}

func LibclangRuntimeLibrary(t Toolchain, library string) string {
	arch := t.LibclangRuntimeLibraryArch()
	if arch == "" {
		return ""
	}
	if !t.Bionic() {
		return "libclang_rt." + library + "-" + arch
	}
	return "libclang_rt." + library + "-" + arch + "-android"
}

func LibRustRuntimeLibrary(t Toolchain, library string) string {
	arch := t.LibclangRuntimeLibraryArch()
	if arch == "" {
		return ""
	}
	if !t.Bionic() {
		return "librustc_rt." + library + "-" + arch
	}
	return "librustc_rt." + library + "-" + arch + "-android"
}

func toolchainBaseFactory() Toolchain {
	return &toolchainBase{}
}

type toolchainFactory func(arch android.Arch) Toolchain

var toolchainFactories = make(map[android.OsType]map[android.ArchType]toolchainFactory)

func registerToolchainFactory(os android.OsType, arch android.ArchType, factory toolchainFactory) {
	if toolchainFactories[os] == nil {
		toolchainFactories[os] = make(map[android.ArchType]toolchainFactory)
	}
	toolchainFactories[os][arch] = factory
}

func FindToolchain(os android.OsType, arch android.Arch) Toolchain {
	factory := toolchainFactories[os][arch.ArchType]
	if factory == nil {
		return toolchainBaseFactory()
	}
	return factory(arch)
}
