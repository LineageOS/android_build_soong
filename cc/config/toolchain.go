// Copyright 2015 Google Inc. All rights reserved.
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
	"fmt"
	"path/filepath"

	"android/soong/android"
)

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
		panic(fmt.Errorf("Toolchain not found for %s arch %q", os.String(), arch.String()))
	}
	return factory(arch)
}

type Toolchain interface {
	Name() string

	GccRoot() string
	GccTriple() string
	// GccVersion should return a real value, not a ninja reference
	GccVersion() string
	ToolPath() string

	IncludeFlags() string

	ClangTriple() string
	ToolchainClangCflags() string
	ToolchainClangLdflags() string
	ClangAsflags() string
	ClangCflags() string
	ClangCppflags() string
	ClangLdflags() string
	ClangLldflags() string
	ClangInstructionSetFlags(string) (string, error)

	ndkTriple() string

	YasmFlags() string

	WindresFlags() string

	Is64Bit() bool

	ShlibSuffix() string
	ExecutableSuffix() string

	LibclangRuntimeLibraryArch() string

	AvailableLibraries() []string

	Bionic() bool
}

type toolchainBase struct {
}

func (t *toolchainBase) ndkTriple() string {
	return ""
}

func NDKTriple(toolchain Toolchain) string {
	triple := toolchain.ndkTriple()
	if triple == "" {
		// Use the clang triple if there is no explicit NDK triple
		triple = toolchain.ClangTriple()
	}
	return triple
}

func (toolchainBase) ClangInstructionSetFlags(s string) (string, error) {
	if s != "" {
		return "", fmt.Errorf("instruction_set: %s is not a supported instruction set", s)
	}
	return "", nil
}

func (toolchainBase) ToolchainClangCflags() string {
	return ""
}

func (toolchainBase) ToolchainClangLdflags() string {
	return ""
}

func (toolchainBase) ShlibSuffix() string {
	return ".so"
}

func (toolchainBase) ExecutableSuffix() string {
	return ""
}

func (toolchainBase) ClangAsflags() string {
	return ""
}

func (toolchainBase) YasmFlags() string {
	return ""
}

func (toolchainBase) WindresFlags() string {
	return ""
}

func (toolchainBase) LibclangRuntimeLibraryArch() string {
	return ""
}

func (toolchainBase) AvailableLibraries() []string {
	return []string{}
}

func (toolchainBase) Bionic() bool {
	return true
}

func (t toolchainBase) ToolPath() string {
	return ""
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

func variantOrDefault(variants map[string]string, choice string) string {
	if ret, ok := variants[choice]; ok {
		return ret
	}
	return variants[""]
}

func addPrefix(list []string, prefix string) []string {
	for i := range list {
		list[i] = prefix + list[i]
	}
	return list
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

func BuiltinsRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "builtins")
}

func AddressSanitizerRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "asan")
}

func HWAddressSanitizerRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "hwasan")
}

func HWAddressSanitizerStaticLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "hwasan_static")
}

func UndefinedBehaviorSanitizerRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "ubsan_standalone")
}

func UndefinedBehaviorSanitizerMinimalRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "ubsan_minimal")
}

func ThreadSanitizerRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "tsan")
}

func ProfileRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "profile")
}

func ScudoRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "scudo")
}

func ScudoMinimalRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "scudo_minimal")
}

func LibFuzzerRuntimeLibrary(t Toolchain) string {
	return LibclangRuntimeLibrary(t, "fuzzer")
}

func ToolPath(t Toolchain) string {
	if p := t.ToolPath(); p != "" {
		return p
	}
	return filepath.Join(t.GccRoot(), t.GccTriple(), "bin")
}

var inList = android.InList
