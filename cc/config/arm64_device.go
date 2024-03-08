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
		"armv9-a": []string{
			"-march=armv8.2-a+dotprod",
			"-mbranch-protection=standard",
			"-fno-stack-protector",
		},
	}

	arm64Ldflags = []string{
		"-Wl,--hash-style=gnu",
		"-Wl,-z,separate-code",
	}

	arm64Lldflags = arm64Ldflags

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
			// Use cortex-a53 because kryo385 is not supported in clang.
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

func init() {
	exportedVars.ExportStringListStaticVariable("Arm64Ldflags", arm64Ldflags)

	exportedVars.ExportStringList("Arm64Lldflags", arm64Lldflags)
	pctx.VariableFunc("Arm64Lldflags", func(ctx android.PackageVarContext) string {
		maxPageSizeFlag := "-Wl,-z,max-page-size=" + ctx.Config().MaxPageSizeSupported()
		flags := append(arm64Lldflags, maxPageSizeFlag)
		return strings.Join(flags, " ")
	})

	exportedVars.ExportStringList("Arm64Cflags", arm64Cflags)
	pctx.VariableFunc("Arm64Cflags", func(ctx android.PackageVarContext) string {
		flags := arm64Cflags
		if ctx.Config().NoBionicPageSizeMacro() {
			flags = append(flags, "-D__BIONIC_NO_PAGE_SIZE_MACRO")
		}
		return strings.Join(flags, " ")
	})

	exportedVars.ExportStringListStaticVariable("Arm64Cppflags", arm64Cppflags)

	exportedVars.ExportVariableReferenceDict("Arm64ArchVariantCflags", arm64ArchVariantCflagsVar)
	exportedVars.ExportVariableReferenceDict("Arm64CpuVariantCflags", arm64CpuVariantCflagsVar)
	exportedVars.ExportVariableReferenceDict("Arm64CpuVariantLdflags", arm64CpuVariantLdflags)

	exportedVars.ExportStringListStaticVariable("Arm64Armv8ACflags", arm64ArchVariantCflags["armv8-a"])
	exportedVars.ExportStringListStaticVariable("Arm64Armv8ABranchProtCflags", arm64ArchVariantCflags["armv8-a-branchprot"])
	exportedVars.ExportStringListStaticVariable("Arm64Armv82ACflags", arm64ArchVariantCflags["armv8-2a"])
	exportedVars.ExportStringListStaticVariable("Arm64Armv82ADotprodCflags", arm64ArchVariantCflags["armv8-2a-dotprod"])
	exportedVars.ExportStringListStaticVariable("Arm64Armv9ACflags", arm64ArchVariantCflags["armv9-a"])

	exportedVars.ExportStringListStaticVariable("Arm64CortexA53Cflags", arm64CpuVariantCflags["cortex-a53"])
	exportedVars.ExportStringListStaticVariable("Arm64CortexA55Cflags", arm64CpuVariantCflags["cortex-a55"])
	exportedVars.ExportStringListStaticVariable("Arm64KryoCflags", arm64CpuVariantCflags["kryo"])
	exportedVars.ExportStringListStaticVariable("Arm64ExynosM1Cflags", arm64CpuVariantCflags["exynos-m1"])
	exportedVars.ExportStringListStaticVariable("Arm64ExynosM2Cflags", arm64CpuVariantCflags["exynos-m2"])

	exportedVars.ExportStringListStaticVariable("Arm64FixCortexA53Ldflags", []string{"-Wl,--fix-cortex-a53-843419"})
}

var (
	arm64ArchVariantCflagsVar = map[string]string{
		"armv8-a":            "${config.Arm64Armv8ACflags}",
		"armv8-a-branchprot": "${config.Arm64Armv8ABranchProtCflags}",
		"armv8-2a":           "${config.Arm64Armv82ACflags}",
		"armv8-2a-dotprod":   "${config.Arm64Armv82ADotprodCflags}",
		"armv9-a":            "${config.Arm64Armv9ACflags}",
	}

	arm64CpuVariantCflagsVar = map[string]string{
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

	arm64CpuVariantLdflags = map[string]string{
		"cortex-a53": "${config.Arm64FixCortexA53Ldflags}",
		"cortex-a72": "${config.Arm64FixCortexA53Ldflags}",
		"cortex-a73": "${config.Arm64FixCortexA53Ldflags}",
		"kryo":       "${config.Arm64FixCortexA53Ldflags}",
		"exynos-m1":  "${config.Arm64FixCortexA53Ldflags}",
		"exynos-m2":  "${config.Arm64FixCortexA53Ldflags}",
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

func (t *toolchainArm64) IncludeFlags() string {
	return ""
}

func (t *toolchainArm64) ClangTriple() string {
	return "aarch64-linux-android"
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
	case "armv9-a":
		// Nothing extra for armv8-a/armv8-2a
	default:
		panic(fmt.Sprintf("Unknown ARM architecture version: %q", arch.ArchVariant))
	}

	toolchainCflags := []string{arm64ArchVariantCflagsVar[arch.ArchVariant]}
	toolchainCflags = append(toolchainCflags,
		variantOrDefault(arm64CpuVariantCflagsVar, arch.CpuVariant))

	extraLdflags := variantOrDefault(arm64CpuVariantLdflags, arch.CpuVariant)
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
