// Copyright 2022 Google Inc. All rights reserved.
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
	"strings"

	"android/soong/android"
)

var (
	riscv64Cflags = []string{
		// Help catch common 32/64-bit errors. (This is duplicated in all 64-bit
		// architectures' cflags.)
		"-Werror=implicit-function-declaration",
		// This is already the driver's Android default, but duplicated here (and
		// below) for ease of experimentation with additional extensions.
		"-march=rv64gcv_zba_zbb_zbs",
		// TODO: move to driver (https://github.com/google/android-riscv64/issues/111)
		"-mno-strict-align",
		// TODO: remove when qemu V works (https://gitlab.com/qemu-project/qemu/-/issues/1976)
		// (Note that we'll probably want to wait for berberis to be good enough
		// that most people don't care about qemu's V performance either!)
		"-mno-implicit-float",
		// TODO: remove when clang default changed (https://github.com/google/android-riscv64/issues/124)
		"-mllvm -jump-is-expensive=false",
	}

	riscv64ArchVariantCflags = map[string][]string{}

	riscv64Ldflags = []string{
		// This is already the driver's Android default, but duplicated here (and
		// above) for ease of experimentation with additional extensions.
		"-march=rv64gcv_zba_zbb_zbs",
		// TODO: remove when clang default changed (https://github.com/google/android-riscv64/issues/124)
		"-Wl,-mllvm -Wl,-jump-is-expensive=false",
	}

	riscv64Lldflags = append(riscv64Ldflags,
		"-Wl,-z,max-page-size=4096",
	)

	riscv64Cppflags = []string{}

	riscv64CpuVariantCflags = map[string][]string{}
)

const ()

func init() {

	pctx.StaticVariable("Riscv64Ldflags", strings.Join(riscv64Ldflags, " "))
	pctx.StaticVariable("Riscv64Lldflags", strings.Join(riscv64Lldflags, " "))

	pctx.StaticVariable("Riscv64Cflags", strings.Join(riscv64Cflags, " "))
	pctx.StaticVariable("Riscv64Cppflags", strings.Join(riscv64Cppflags, " "))
}

var (
	riscv64ArchVariantCflagsVar = map[string]string{}

	riscv64CpuVariantCflagsVar = map[string]string{}

	riscv64CpuVariantLdflags = map[string]string{}
)

type toolchainRiscv64 struct {
	toolchainBionic
	toolchain64Bit

	ldflags         string
	lldflags        string
	toolchainCflags string
}

func (t *toolchainRiscv64) Name() string {
	return "riscv64"
}

func (t *toolchainRiscv64) IncludeFlags() string {
	return ""
}

func (t *toolchainRiscv64) ClangTriple() string {
	return "riscv64-linux-android"
}

func (t *toolchainRiscv64) Cflags() string {
	return "${config.Riscv64Cflags}"
}

func (t *toolchainRiscv64) Cppflags() string {
	return "${config.Riscv64Cppflags}"
}

func (t *toolchainRiscv64) Ldflags() string {
	return t.ldflags
}

func (t *toolchainRiscv64) Lldflags() string {
	return t.lldflags
}

func (t *toolchainRiscv64) ToolchainCflags() string {
	return t.toolchainCflags
}

func (toolchainRiscv64) LibclangRuntimeLibraryArch() string {
	return "riscv64"
}

func riscv64ToolchainFactory(arch android.Arch) Toolchain {
	switch arch.ArchVariant {
	case "":
	default:
		panic(fmt.Sprintf("Unknown Riscv64 architecture version: %q", arch.ArchVariant))
	}

	toolchainCflags := []string{riscv64ArchVariantCflagsVar[arch.ArchVariant]}
	toolchainCflags = append(toolchainCflags,
		variantOrDefault(riscv64CpuVariantCflagsVar, arch.CpuVariant))

	extraLdflags := variantOrDefault(riscv64CpuVariantLdflags, arch.CpuVariant)
	return &toolchainRiscv64{
		ldflags: strings.Join([]string{
			"${config.Riscv64Ldflags}",
			extraLdflags,
		}, " "),
		lldflags: strings.Join([]string{
			"${config.Riscv64Lldflags}",
			extraLdflags,
		}, " "),
		toolchainCflags: strings.Join(toolchainCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.Riscv64, riscv64ToolchainFactory)
}
