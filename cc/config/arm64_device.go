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
		"armv8-2a": []string{
			"-march=armv8.2a",
		},
	}

	arm64Ldflags = []string{
		"-Wl,-m,aarch64_elf64_le_vec",
		"-Wl,--hash-style=gnu",
		"-fuse-ld=gold",
		"-Wl,--icf=safe",
	}

	arm64Cppflags = []string{}

	arm64CpuVariantCflags = map[string][]string{
		"cortex-a53": []string{
			"-mcpu=cortex-a53",
		},
		"cortex-a55": []string{
			// The cortex-a55 target is not yet supported,
			// so use cortex-a53.
			"-mcpu=cortex-a53",
		},
		"cortex-a75": []string{
			// Use the cortex-a53 since it is similar to the little
			// core (cortex-a55) and is sensitive to ordering.
			// The cortex-a55 target is not yet supported.
			"-mcpu=cortex-a53",
		},
		"kryo": []string{
			// Use the cortex-a57 cpu since some compilers
			// don't support a Kryo specific target yet.
			"-mcpu=cortex-a57",
		},
		"exynos-m1": []string{
			"-mcpu=exynos-m1",
		},
		"exynos-m2": []string{
			"-mcpu=exynos-m2",
		},
	}

	arm64ClangCpuVariantCflags = copyVariantFlags(arm64CpuVariantCflags)
)

const (
	arm64GccVersion = "4.9"
)

func init() {
	android.RegisterArchVariants(android.Arm64,
		"armv8_a",
		"armv8_2a",
		"cortex-a53",
		"cortex-a55",
		"cortex-a73",
		"cortex-a75",
		"kryo",
		"kryo300",
		"exynos-m1",
		"exynos-m2",
		"denver64")

	// Clang supports specific Kryo targeting
	replaceFirst(arm64ClangCpuVariantCflags["kryo"], "-mcpu=cortex-a57", "-mcpu=kryo")

	pctx.StaticVariable("arm64GccVersion", arm64GccVersion)

	pctx.SourcePathVariable("Arm64GccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/aarch64/aarch64-linux-android-${arm64GccVersion}")

	pctx.StaticVariable("Arm64Cflags", strings.Join(arm64Cflags, " "))
	pctx.StaticVariable("Arm64Ldflags", strings.Join(arm64Ldflags, " "))
	pctx.StaticVariable("Arm64Cppflags", strings.Join(arm64Cppflags, " "))
	pctx.StaticVariable("Arm64IncludeFlags", bionicHeaders("arm64"))

	pctx.StaticVariable("Arm64ClangCflags", strings.Join(ClangFilterUnknownCflags(arm64Cflags), " "))
	pctx.StaticVariable("Arm64ClangLdflags", strings.Join(ClangFilterUnknownCflags(arm64Ldflags), " "))
	pctx.StaticVariable("Arm64ClangCppflags", strings.Join(ClangFilterUnknownCflags(arm64Cppflags), " "))

	pctx.StaticVariable("Arm64ClangArmv8ACflags", strings.Join(arm64ArchVariantCflags["armv8-a"], " "))
	pctx.StaticVariable("Arm64ClangArmv82ACflags", strings.Join(arm64ArchVariantCflags["armv8-2a"], " "))

	pctx.StaticVariable("Arm64CortexA53Cflags",
		strings.Join(arm64CpuVariantCflags["cortex-a53"], " "))
	pctx.StaticVariable("Arm64ClangCortexA53Cflags",
		strings.Join(arm64ClangCpuVariantCflags["cortex-a53"], " "))

	pctx.StaticVariable("Arm64CortexA55Cflags",
		strings.Join(arm64CpuVariantCflags["cortex-a55"], " "))
	pctx.StaticVariable("Arm64ClangCortexA55Cflags",
		strings.Join(arm64ClangCpuVariantCflags["cortex-a55"], " "))

	pctx.StaticVariable("Arm64KryoCflags",
		strings.Join(arm64CpuVariantCflags["kryo"], " "))
	pctx.StaticVariable("Arm64ClangKryoCflags",
		strings.Join(arm64ClangCpuVariantCflags["kryo"], " "))

	pctx.StaticVariable("Arm64ExynosM1Cflags",
		strings.Join(arm64CpuVariantCflags["cortex-a53"], " "))
	pctx.StaticVariable("Arm64ClangExynosM1Cflags",
		strings.Join(arm64ClangCpuVariantCflags["exynos-m1"], " "))

	pctx.StaticVariable("Arm64ExynosM2Cflags",
		strings.Join(arm64CpuVariantCflags["cortex-a53"], " "))
	pctx.StaticVariable("Arm64ClangExynosM2Cflags",
		strings.Join(arm64ClangCpuVariantCflags["exynos-m2"], " "))
}

var (
	arm64CpuVariantCflagsVar = map[string]string{
		"":           "",
		"cortex-a53": "${config.Arm64CortexA53Cflags}",
		"cortex-a55": "${config.Arm64CortexA55Cflags}",
		"cortex-a73": "${config.Arm64CortexA53Cflags}",
		"cortex-a75": "${config.Arm64CortexA55Cflags}",
		"kryo":       "${config.Arm64KryoCflags}",
		"exynos-m1":  "${config.Arm64ExynosM1Cflags}",
		"exynos-m2":  "${config.Arm64ExynosM2Cflags}",
	}

	arm64ClangArchVariantCflagsVar = map[string]string{
		"armv8-a":  "${config.Arm64ClangArmv8ACflags}",
		"armv8-2a": "${config.Arm64ClangArmv82ACflags}",
	}

	arm64ClangCpuVariantCflagsVar = map[string]string{
		"":           "",
		"cortex-a53": "${config.Arm64ClangCortexA53Cflags}",
		"cortex-a55": "${config.Arm64ClangCortexA55Cflags}",
		"cortex-a73": "${config.Arm64ClangCortexA53Cflags}",
		"cortex-a75": "${config.Arm64ClangCortexA55Cflags}",
		"kryo":       "${config.Arm64ClangKryoCflags}",
		"exynos-m1":  "${config.Arm64ClangExynosM1Cflags}",
		"exynos-m2":  "${config.Arm64ClangExynosM2Cflags}",
	}
)

type toolchainArm64 struct {
	toolchain64Bit

	ldflags              string
	toolchainCflags      string
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

func (t *toolchainArm64) ToolchainCflags() string {
	return t.toolchainCflags
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

func (t *toolchainArm64) IncludeFlags() string {
	return "${config.Arm64IncludeFlags}"
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

func (t *toolchainArm64) ToolchainClangCflags() string {
	return t.toolchainClangCflags
}

func (toolchainArm64) SanitizerRuntimeLibraryArch() string {
	return "aarch64"
}

func arm64ToolchainFactory(arch android.Arch) Toolchain {
	switch arch.ArchVariant {
	case "armv8-a":
	case "armv8-2a":
		// Nothing extra for armv8-a/armv8-2a
	default:
		panic(fmt.Sprintf("Unknown ARM architecture version: %q", arch.ArchVariant))
	}

	toolchainClangCflags := []string{arm64ClangArchVariantCflagsVar[arch.ArchVariant]}
	toolchainClangCflags = append(toolchainClangCflags,
		variantOrDefault(arm64ClangCpuVariantCflagsVar, arch.CpuVariant))

	var extraLdflags string
	switch arch.CpuVariant {
	case "cortex-a53", "cortex-a73", "kryo", "exynos-m1", "exynos-m2",
		// This variant might not need the workaround but leave it
		// in the list since it has had the workaround on before.
		"denver64":
		extraLdflags = "-Wl,--fix-cortex-a53-843419"
	}

	return &toolchainArm64{
		ldflags: strings.Join([]string{
			"${config.Arm64Ldflags}",
			extraLdflags,
		}, " "),
		toolchainCflags:      variantOrDefault(arm64CpuVariantCflagsVar, arch.CpuVariant),
		toolchainClangCflags: strings.Join(toolchainClangCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.Arm64, arm64ToolchainFactory)
}
