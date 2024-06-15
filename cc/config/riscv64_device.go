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
		// Help catch common 32/64-bit errors.
		"-Werror=implicit-function-declaration",
		"-march=rv64gcv_zba_zbb_zbs",
		"-munaligned-access",
		// Until https://gitlab.com/qemu-project/qemu/-/issues/1976 is fixed...
		"-mno-implicit-float",
		// (https://github.com/google/android-riscv64/issues/124)
		"-mllvm -jump-is-expensive=false",
	}

	riscv64ArchVariantCflags = map[string][]string{}

	riscv64Ldflags = []string{
		"-Wl,--hash-style=gnu",
		"-march=rv64gcv_zba_zbb_zbs",
		"-munaligned-access",
		// We should change the default for this in clang, but for now...
		// (https://github.com/google/android-riscv64/issues/124)
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

	exportedVars.ExportStringListStaticVariable("Riscv64Ldflags", riscv64Ldflags)
	exportedVars.ExportStringListStaticVariable("Riscv64Lldflags", riscv64Lldflags)

	exportedVars.ExportStringListStaticVariable("Riscv64Cflags", riscv64Cflags)
	exportedVars.ExportStringListStaticVariable("Riscv64Cppflags", riscv64Cppflags)

	exportedVars.ExportVariableReferenceDict("Riscv64ArchVariantCflags", riscv64ArchVariantCflagsVar)
	exportedVars.ExportVariableReferenceDict("Riscv64CpuVariantCflags", riscv64CpuVariantCflagsVar)
	exportedVars.ExportVariableReferenceDict("Riscv64CpuVariantLdflags", riscv64CpuVariantLdflags)
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
