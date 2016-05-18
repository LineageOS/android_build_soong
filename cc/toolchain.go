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

package cc

import (
	"fmt"

	"android/soong/android"
)

type toolchainFactory func(arch android.Arch) Toolchain

var toolchainFactories = map[android.HostOrDevice]map[android.HostType]map[android.ArchType]toolchainFactory{
	android.Host: map[android.HostType]map[android.ArchType]toolchainFactory{
		android.Linux:   make(map[android.ArchType]toolchainFactory),
		android.Darwin:  make(map[android.ArchType]toolchainFactory),
		android.Windows: make(map[android.ArchType]toolchainFactory),
	},
	android.Device: map[android.HostType]map[android.ArchType]toolchainFactory{
		android.NoHostType: make(map[android.ArchType]toolchainFactory),
	},
}

func registerDeviceToolchainFactory(arch android.ArchType, factory toolchainFactory) {
	toolchainFactories[android.Device][android.NoHostType][arch] = factory
}

func registerHostToolchainFactory(ht android.HostType, arch android.ArchType, factory toolchainFactory) {
	toolchainFactories[android.Host][ht][arch] = factory
}

type Toolchain interface {
	Name() string

	GccRoot() string
	GccTriple() string
	// GccVersion should return a real value, not a ninja reference
	GccVersion() string

	ToolchainCflags() string
	ToolchainLdflags() string
	Cflags() string
	Cppflags() string
	Ldflags() string
	IncludeFlags() string
	InstructionSetFlags(string) (string, error)

	ClangSupported() bool
	ClangTriple() string
	ToolchainClangCflags() string
	ToolchainClangLdflags() string
	ClangAsflags() string
	ClangCflags() string
	ClangCppflags() string
	ClangLdflags() string
	ClangInstructionSetFlags(string) (string, error)

	Is64Bit() bool

	ShlibSuffix() string
	ExecutableSuffix() string

	SystemCppCppflags() string
	SystemCppLdflags() string

	AddressSanitizerRuntimeLibrary() string
}

type toolchainBase struct {
}

func (toolchainBase) InstructionSetFlags(s string) (string, error) {
	if s != "" {
		return "", fmt.Errorf("instruction_set: %s is not a supported instruction set", s)
	}
	return "", nil
}

func (toolchainBase) ClangInstructionSetFlags(s string) (string, error) {
	if s != "" {
		return "", fmt.Errorf("instruction_set: %s is not a supported instruction set", s)
	}
	return "", nil
}

func (toolchainBase) ToolchainCflags() string {
	return ""
}

func (toolchainBase) ToolchainLdflags() string {
	return ""
}

func (toolchainBase) ToolchainClangCflags() string {
	return ""
}

func (toolchainBase) ToolchainClangLdflags() string {
	return ""
}

func (toolchainBase) ClangSupported() bool {
	return true
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

func (toolchainBase) SystemCppCppflags() string {
	return ""
}

func (toolchainBase) SystemCppLdflags() string {
	return ""
}

func (toolchainBase) AddressSanitizerRuntimeLibrary() string {
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
