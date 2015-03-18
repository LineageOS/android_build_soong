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

	"android/soong/common"
)

type toolchainFactory func(archVariant string, cpuVariant string) toolchain

var toolchainFactories = map[common.HostOrDevice]map[common.ArchType]toolchainFactory{
	common.Host:   make(map[common.ArchType]toolchainFactory),
	common.Device: make(map[common.ArchType]toolchainFactory),
}

func registerToolchainFactory(hod common.HostOrDevice, arch common.ArchType,
	factory toolchainFactory) {

	toolchainFactories[hod][arch] = factory
}

type toolchain interface {
	GccRoot() string
	GccTriple() string
	Cflags() string
	Cppflags() string
	Ldflags() string
	IncludeFlags() string
	InstructionSetFlags(string) (string, error)

	ClangTriple() string
	ClangCflags() string
	ClangCppflags() string
	ClangLdflags() string

	Is64Bit() bool
}

type toolchainBase struct {
}

func (toolchainBase) InstructionSetFlags(s string) (string, error) {
	if s != "" {
		return "", fmt.Errorf("instruction_set: %s is not a supported instruction set", s)
	}
	return "", nil
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
