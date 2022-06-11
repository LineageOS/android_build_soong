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
			// Disable BTI until drm vendors stop using OS libraries as sources
			// of gadgets (https://issuetracker.google.com/216395195).
			"-mbranch-protection=pac-ret",
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

	arm64Lldflags = append(ClangFilterUnknownLldflags(arm64Ldflags),
		"-Wl,-z,max-page-size=4096")

	arm64Cppflags = []string{}

	arm64ClangCpuVariantCflags = map[string][]string{
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

	pctx.StaticVariable("Arm64Ldflags", strings.Join(arm64Ldflags, " "))
	pctx.StaticVariable("Arm64Lldflags", strings.Join(arm64Lldflags, " "))

	pctx.StaticVariable("Arm64ClangCflags", strings.Join(ClangFilterUnknownCflags(arm64Cflags), " "))
	pctx.StaticVariable("Arm64ClangLdflags", strings.Join(ClangFilterUnknownCflags(arm64Ldflags), " "))
	pctx.StaticVariable("Arm64ClangLldflags", strings.Join(ClangFilterUnknownCflags(arm64Lldflags), " "))
	pctx.StaticVariable("Arm64ClangCppflags", strings.Join(ClangFilterUnknownCflags(arm64Cppflags), " "))

	pctx.StaticVariable("Arm64ClangArmv8ACflags", strings.Join(arm64ArchVariantCflags["armv8-a"], " "))
	pctx.StaticVariable("Arm64ClangArmv8ABranchProtCflags", strings.Join(arm64ArchVariantCflags["armv8-a-branchprot"], " "))
	pctx.StaticVariable("Arm64ClangArmv82ACflags", strings.Join(arm64ArchVariantCflags["armv8-2a"], " "))
	pctx.StaticVariable("Arm64ClangArmv82ADotprodCflags", strings.Join(arm64ArchVariantCflags["armv8-2a-dotprod"], " "))

	pctx.StaticVariable("Arm64ClangCortexA53Cflags",
		strings.Join(arm64ClangCpuVariantCflags["cortex-a53"], " "))

	pctx.StaticVariable("Arm64ClangCortexA55Cflags",
		strings.Join(arm64ClangCpuVariantCflags["cortex-a55"], " "))

	pctx.StaticVariable("Arm64ClangKryoCflags",
		strings.Join(arm64ClangCpuVariantCflags["kryo"], " "))

	pctx.StaticVariable("Arm64ClangExynosM1Cflags",
		strings.Join(arm64ClangCpuVariantCflags["exynos-m1"], " "))

	pctx.StaticVariable("Arm64ClangExynosM2Cflags",
		strings.Join(arm64ClangCpuVariantCflags["exynos-m2"], " "))
}

var (
	arm64ClangArchVariantCflagsVar = map[string]string{
		"armv8-a":            "${config.Arm64ClangArmv8ACflags}",
		"armv8-a-branchprot": "${config.Arm64ClangArmv8ABranchProtCflags}",
		"armv8-2a":           "${config.Arm64ClangArmv82ACflags}",
		"armv8-2a-dotprod":   "${config.Arm64ClangArmv82ADotprodCflags}",
	}

	arm64ClangCpuVariantCflagsVar = map[string]string{
		"":           "",
		"cortex-a53": "${config.Arm64ClangCortexA53Cflags}",
		"cortex-a55": "${config.Arm64ClangCortexA55Cflags}",
		"cortex-a72": "${config.Arm64ClangCortexA53Cflags}",
		"cortex-a73": "${config.Arm64ClangCortexA53Cflags}",
		"cortex-a75": "${config.Arm64ClangCortexA55Cflags}",
		"cortex-a76": "${config.Arm64ClangCortexA55Cflags}",
		"kryo":       "${config.Arm64ClangKryoCflags}",
		"kryo385":    "${config.Arm64ClangCortexA53Cflags}",
		"exynos-m1":  "${config.Arm64ClangExynosM1Cflags}",
		"exynos-m2":  "${config.Arm64ClangExynosM2Cflags}",
	}
)

type toolchainArm64 struct {
	toolchain64Bit

	ldflags              string
	lldflags             string
	toolchainClangCflags string
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

func (t *toolchainArm64) ClangCflags() string {
	return "${config.Arm64ClangCflags}"
}

func (t *toolchainArm64) ClangCppflags() string {
	return "${config.Arm64ClangCppflags}"
}

func (t *toolchainArm64) ClangLdflags() string {
	return t.ldflags
}

func (t *toolchainArm64) ClangLldflags() string {
	return t.lldflags
}

func (t *toolchainArm64) ToolchainClangCflags() string {
	return t.toolchainClangCflags
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

	toolchainClangCflags := []string{arm64ClangArchVariantCflagsVar[arch.ArchVariant]}
	toolchainClangCflags = append(toolchainClangCflags,
		variantOrDefault(arm64ClangCpuVariantCflagsVar, arch.CpuVariant))

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
		toolchainClangCflags: strings.Join(toolchainClangCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.Arm64, arm64ToolchainFactory)
}
