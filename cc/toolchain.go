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
	"strings"

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

	AddressSanitizerRuntimeLibrary() string

	AvailableLibraries() []string
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

func (toolchainBase) AddressSanitizerRuntimeLibrary() string {
	return ""
}

func (toolchainBase) AvailableLibraries() []string {
	return []string{}
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

func bionicHeaders(bionicArch, kernelArch string) string {
	return strings.Join([]string{
		"-isystem bionic/libc/arch-" + bionicArch + "/include",
		"-isystem bionic/libc/include",
		"-isystem bionic/libc/kernel/uapi",
		"-isystem bionic/libc/kernel/uapi/asm-" + kernelArch,
		"-isystem bionic/libc/kernel/android/uapi",
	}, " ")
}
