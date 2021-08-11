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
	"strings"

	"android/soong/android"
)

var (
	arm64Cflags = []string{
		// Help catch common 32/64-bit errors.
		"-Werror=implicit-function-declaration",
	}

	arm64ArchVariantCflags = map[string][]string{
		"armv8-a": []string{
			"-march=armv8-a",
		},
		"armv8-a-branchprot": []string{
			"-march=armv8-a",
			"-mbranch-protection=standard",
		},
		"armv8-2a": []string{
			"-march=armv8.2-a",
		},
		"armv8-2a-dotprod": []string{
			"-march=armv8.2-a+dotprod",
		},
	}

	arm64Ldflags = []string{
		"-Wl,--hash-style=gnu",
		"-Wl,-z,separate-code",
	}

	arm64Lldflags = append(arm64Ldflags,
		"-Wl,-z,max-page-size=4096")

	arm64Cppflags = []string{}

	arm64CpuVariantCflags = map[string][]string{
		"cortex-a53": []string{
			"-mcpu=cortex-a53",
		},
		"cortex-a55": []string{
			"-mcpu=cortex-a55",
		},
		"cortex-a75": []string{
			// Use the cortex-a55 since it is similar to the little
			// core (cortex-a55) and is sensitive to ordering.
			"-mcpu=cortex-a55",
		},
		"cortex-a76": []string{
			// Use the cortex-a55 since it is similar to the little
			// core (cortex-a55) and is sensitive to ordering.
			"-mcpu=cortex-a55",
		},
		"kryo": []string{
			"-mcpu=kryo",
		},
		"kryo385": []string{
			// Use cortex-a53 because kryo385 is not supported in GCC/clang.
			"-mcpu=cortex-a53",
		},
		"exynos-m1": []string{
			"-mcpu=exynos-m1",
		},
		"exynos-m2": []string{
			"-mcpu=exynos-m2",
		},
	}
)

const (
	arm64GccVersion = "4.9"
)

func init() {
	pctx.StaticVariable("arm64GccVersion", arm64GccVersion)

	pctx.SourcePathVariable("Arm64GccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/aarch64/aarch64-linux-android-${arm64GccVersion}")

	exportStringListStaticVariable("Arm64Ldflags", arm64Ldflags)
	exportStringListStaticVariable("Arm64Lldflags", arm64Lldflags)

	exportStringListStaticVariable("Arm64Cflags", arm64Cflags)
	exportStringListStaticVariable("Arm64Cppflags", arm64Cppflags)

	exportedStringListDictVars.Set("Arm64ArchVariantCflags", arm64ArchVariantCflags)
	exportedStringListDictVars.Set("Arm64CpuVariantCflags", arm64CpuVariantCflags)

	pctx.StaticVariable("Arm64Armv8ACflags", strings.Join(arm64ArchVariantCflags["armv8-a"], " "))
	pctx.StaticVariable("Arm64Armv8ABranchProtCflags", strings.Join(arm64ArchVariantCflags["armv8-a-branchprot"], " "))
	pctx.StaticVariable("Arm64Armv82ACflags", strings.Join(arm64ArchVariantCflags["armv8-2a"], " "))
	pctx.StaticVariable("Arm64Armv82ADotprodCflags", strings.Join(arm64ArchVariantCflags["armv8-2a-dotprod"], " "))

	pctx.StaticVariable("Arm64CortexA53Cflags", strings.Join(arm64CpuVariantCflags["cortex-a53"], " "))
	pctx.StaticVariable("Arm64CortexA55Cflags", strings.Join(arm64CpuVariantCflags["cortex-a55"], " "))
	pctx.StaticVariable("Arm64KryoCflags", strings.Join(arm64CpuVariantCflags["kryo"], " "))
	pctx.StaticVariable("Arm64ExynosM1Cflags", strings.Join(arm64CpuVariantCflags["exynos-m1"], " "))
	pctx.StaticVariable("Arm64ExynosM2Cflags", strings.Join(arm64CpuVariantCflags["exynos-m2"], " "))
}

var (
	arm64ArchVariantCflagsVar = map[string]string{
		"armv8-a":            "${config.Arm64Armv8ACflags}",
		"armv8-a-branchprot": "${config.Arm64Armv8ABranchProtCflags}",
		"armv8-2a":           "${config.Arm64Armv82ACflags}",
		"armv8-2a-dotprod":   "${config.Arm64Armv82ADotprodCflags}",
	}

	arm64CpuVariantCflagsVar = map[string]string{
		"":           "",
		"cortex-a53": "${config.Arm64CortexA53Cflags}",
		"cortex-a55": "${config.Arm64CortexA55Cflags}",
		"cortex-a72": "${config.Arm64CortexA53Cflags}",
		"cortex-a73": "${config.Arm64CortexA53Cflags}",
		"cortex-a75": "${config.Arm64CortexA55Cflags}",
		"cortex-a76": "${config.Arm64CortexA55Cflags}",
		"kryo":       "${config.Arm64KryoCflags}",
		"kryo385":    "${config.Arm64CortexA53Cflags}",
		"exynos-m1":  "${config.Arm64ExynosM1Cflags}",
		"exynos-m2":  "${config.Arm64ExynosM2Cflags}",
	}
)

type toolchainArm64 struct {
	toolchainBionic
	toolchain64Bit

	ldflags         string
	lldflags        string
	toolchainCflags string
}

func (t *toolchainArm64) Name() string {
	return "arm64"
}

func (t *toolchainArm64) GccRoot() string {
	return "${config.Arm64GccRoot}"
}

func (t *toolchainArm64) GccTriple() string {
	return "aarch64-linux-android"
}

func (t *toolchainArm64) GccVersion() string {
	return arm64GccVersion
}

func (t *toolchainArm64) IncludeFlags() string {
	return ""
}

func (t *toolchainArm64) ClangTriple() string {
	return t.GccTriple()
}

func (t *toolchainArm64) Cflags() string {
	return "${config.Arm64Cflags}"
}

func (t *toolchainArm64) Cppflags() string {
	return "${config.Arm64Cppflags}"
}

func (t *toolchainArm64) Ldflags() string {
	return t.ldflags
}

func (t *toolchainArm64) Lldflags() string {
	return t.lldflags
}

func (t *toolchainArm64) ToolchainCflags() string {
	return t.toolchainCflags
}

func (toolchainArm64) LibclangRuntimeLibraryArch() string {
	return "aarch64"
}

func arm64ToolchainFactory(arch android.Arch) Toolchain {
	switch arch.ArchVariant {
	case "armv8-a":
	case "armv8-a-branchprot":
	case "armv8-2a":
	case "armv8-2a-dotprod":
		// Nothing extra for armv8-a/armv8-2a
	default:
		panic(fmt.Sprintf("Unknown ARM architecture version: %q", arch.ArchVariant))
	}

	toolchainCflags := []string{arm64ArchVariantCflagsVar[arch.ArchVariant]}
	toolchainCflags = append(toolchainCflags,
		variantOrDefault(arm64CpuVariantCflagsVar, arch.CpuVariant))

	var extraLdflags string
	switch arch.CpuVariant {
	case "cortex-a53", "cortex-a72", "cortex-a73", "kryo", "exynos-m1", "exynos-m2":
		extraLdflags = "-Wl,--fix-cortex-a53-843419"
	}

	return &toolchainArm64{
		ldflags: strings.Join([]string{
			"${config.Arm64Ldflags}",
			extraLdflags,
		}, " "),
		lldflags: strings.Join([]string{
			"${config.Arm64Lldflags}",
			extraLdflags,
		}, " "),
		toolchainCflags: strings.Join(toolchainCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.Arm64, arm64ToolchainFactory)
}
