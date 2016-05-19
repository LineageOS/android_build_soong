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

var (
	armToolchainCflags = []string{
		"-mthumb-interwork",
		"-msoft-float",
	}

	armCflags = []string{
		"-fno-exceptions", // from build/core/combo/select.mk
		"-Wno-multichar",  // from build/core/combo/select.mk
		"-ffunction-sections",
		"-fdata-sections",
		"-funwind-tables",
		"-fstack-protector-strong",
		"-Wa,--noexecstack",
		"-Werror=format-security",
		"-D_FORTIFY_SOURCE=2",
		"-fno-short-enums",
		"-no-canonical-prefixes",
		"-fno-canonical-system-headers",

		"-fno-builtin-sin",
		"-fno-strict-volatile-bitfields",

		// TARGET_RELEASE_CFLAGS
		"-DNDEBUG",
		"-g",
		"-Wstrict-aliasing=2",
		"-fgcse-after-reload",
		"-frerun-cse-after-loop",
		"-frename-registers",
	}

	armCppflags = []string{
		"-fvisibility-inlines-hidden",
	}

	armLdflags = []string{
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
		"-Wl,--build-id=md5",
		"-Wl,--warn-shared-textrel",
		"-Wl,--fatal-warnings",
		"-Wl,--icf=safe",
		"-Wl,--hash-style=gnu",
		"-Wl,--no-undefined-version",
	}

	armArmCflags = []string{
		"-O2",
		"-fomit-frame-pointer",
		"-fstrict-aliasing",
		"-funswitch-loops",
	}

	armThumbCflags = []string{
		"-mthumb",
		"-Os",
		"-fomit-frame-pointer",
		"-fno-strict-aliasing",
	}

	armArchVariantCflags = map[string][]string{
		"armv5te": []string{
			"-march=armv5te",
			"-mtune=xscale",
			"-D__ARM_ARCH_5__",
			"-D__ARM_ARCH_5T__",
			"-D__ARM_ARCH_5E__",
			"-D__ARM_ARCH_5TE__",
		},
		"armv7-a": []string{
			"-march=armv7-a",
			"-mfloat-abi=softfp",
			"-mfpu=vfpv3-d16",
		},
		"armv7-a-neon": []string{
			"-mfloat-abi=softfp",
			"-mfpu=neon",
		},
	}

	armCpuVariantCflags = map[string][]string{
		"": []string{
			"-march=armv7-a",
		},
		"cortex-a7": []string{
			"-mcpu=cortex-a7",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"cortex-a8": []string{
			"-mcpu=cortex-a8",
		},
		"cortex-a15": []string{
			"-mcpu=cortex-a15",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"krait": []string{
			"-mcpu=cortex-a15",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
	}

	armClangCpuVariantCflags  = copyVariantFlags(armCpuVariantCflags)
	armClangArchVariantCflags = copyVariantFlags(armArchVariantCflags)
)

const (
	armGccVersion = "4.9"
)

func init() {
	replaceFirst := func(slice []string, from, to string) {
		if slice[0] != from {
			panic(fmt.Errorf("Expected %q, found %q", from, to))
		}

		slice[0] = to
	}

	replaceFirst(armClangArchVariantCflags["armv5te"], "-march=armv5te", "-march=armv5t")
	replaceFirst(armClangCpuVariantCflags["krait"], "-mcpu=cortex-a15", "-mcpu=krait")
	armClangCpuVariantCflags["krait"] = append(armClangCpuVariantCflags["krait"], "-mfpu=neon-vfpv4")

	pctx.StaticVariable("armGccVersion", armGccVersion)

	pctx.SourcePathVariable("armGccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/arm/arm-linux-androideabi-${armGccVersion}")

	pctx.StaticVariable("armGccTriple", "arm-linux-androideabi")

	pctx.StaticVariable("armToolchainCflags", strings.Join(armToolchainCflags, " "))
	pctx.StaticVariable("armCflags", strings.Join(armCflags, " "))
	pctx.StaticVariable("armLdflags", strings.Join(armLdflags, " "))
	pctx.StaticVariable("armCppflags", strings.Join(armCppflags, " "))
	pctx.StaticVariable("armIncludeFlags", strings.Join([]string{
		"-isystem ${LibcRoot}/arch-arm/include",
		"-isystem ${LibcRoot}/include",
		"-isystem ${LibcRoot}/kernel/uapi",
		"-isystem ${LibcRoot}/kernel/common",
		"-isystem ${LibcRoot}/kernel/uapi/asm-arm",
	}, " "))

	// Extended cflags

	// ARM vs. Thumb instruction set flags
	pctx.StaticVariable("armArmCflags", strings.Join(armArmCflags, " "))
	pctx.StaticVariable("armThumbCflags", strings.Join(armThumbCflags, " "))

	// Architecture variant cflags
	pctx.StaticVariable("armArmv5TECflags", strings.Join(armArchVariantCflags["armv5te"], " "))
	pctx.StaticVariable("armArmv7ACflags", strings.Join(armArchVariantCflags["armv7-a"], " "))
	pctx.StaticVariable("armArmv7ANeonCflags", strings.Join(armArchVariantCflags["armv7-a-neon"], " "))

	// Cpu variant cflags
	pctx.StaticVariable("armGenericCflags", strings.Join(armCpuVariantCflags[""], " "))
	pctx.StaticVariable("armCortexA7Cflags", strings.Join(armCpuVariantCflags["cortex-a7"], " "))
	pctx.StaticVariable("armCortexA8Cflags", strings.Join(armCpuVariantCflags["cortex-a8"], " "))
	pctx.StaticVariable("armCortexA15Cflags", strings.Join(armCpuVariantCflags["cortex-a15"], " "))
	pctx.StaticVariable("armKraitCflags", strings.Join(armCpuVariantCflags["krait"], " "))

	// Clang cflags
	pctx.StaticVariable("armToolchainClangCflags", strings.Join(clangFilterUnknownCflags(armToolchainCflags), " "))
	pctx.StaticVariable("armClangCflags", strings.Join(clangFilterUnknownCflags(armCflags), " "))
	pctx.StaticVariable("armClangLdflags", strings.Join(clangFilterUnknownCflags(armLdflags), " "))
	pctx.StaticVariable("armClangCppflags", strings.Join(clangFilterUnknownCflags(armCppflags), " "))

	// Clang ARM vs. Thumb instruction set cflags
	pctx.StaticVariable("armClangArmCflags", strings.Join(clangFilterUnknownCflags(armArmCflags), " "))
	pctx.StaticVariable("armClangThumbCflags", strings.Join(clangFilterUnknownCflags(armThumbCflags), " "))

	// Clang arch variant cflags
	pctx.StaticVariable("armClangArmv5TECflags",
		strings.Join(armClangArchVariantCflags["armv5te"], " "))
	pctx.StaticVariable("armClangArmv7ACflags",
		strings.Join(armClangArchVariantCflags["armv7-a"], " "))
	pctx.StaticVariable("armClangArmv7ANeonCflags",
		strings.Join(armClangArchVariantCflags["armv7-a-neon"], " "))

	// Clang cpu variant cflags
	pctx.StaticVariable("armClangGenericCflags",
		strings.Join(armClangCpuVariantCflags[""], " "))
	pctx.StaticVariable("armClangCortexA7Cflags",
		strings.Join(armClangCpuVariantCflags["cortex-a7"], " "))
	pctx.StaticVariable("armClangCortexA8Cflags",
		strings.Join(armClangCpuVariantCflags["cortex-a8"], " "))
	pctx.StaticVariable("armClangCortexA15Cflags",
		strings.Join(armClangCpuVariantCflags["cortex-a15"], " "))
	pctx.StaticVariable("armClangKraitCflags",
		strings.Join(armClangCpuVariantCflags["krait"], " "))
}

var (
	armArchVariantCflagsVar = map[string]string{
		"armv5te":      "${armArmv5TECflags}",
		"armv7-a":      "${armArmv7ACflags}",
		"armv7-a-neon": "${armArmv7ANeonCflags}",
	}

	armCpuVariantCflagsVar = map[string]string{
		"":               "${armGenericCflags}",
		"cortex-a7":      "${armCortexA7Cflags}",
		"cortex-a8":      "${armCortexA8Cflags}",
		"cortex-a15":     "${armCortexA15Cflags}",
		"cortex-a53":     "${armCortexA7Cflags}",
		"cortex-a53.a57": "${armCortexA7Cflags}",
		"krait":          "${armKraitCflags}",
		"denver":         "${armCortexA15Cflags}",
	}

	armClangArchVariantCflagsVar = map[string]string{
		"armv5te":      "${armClangArmv5TECflags}",
		"armv7-a":      "${armClangArmv7ACflags}",
		"armv7-a-neon": "${armClangArmv7ANeonCflags}",
	}

	armClangCpuVariantCflagsVar = map[string]string{
		"":               "${armClangGenericCflags}",
		"cortex-a7":      "${armClangCortexA7Cflags}",
		"cortex-a8":      "${armClangCortexA8Cflags}",
		"cortex-a15":     "${armClangCortexA15Cflags}",
		"cortex-a53":     "${armClangCortexA7Cflags}",
		"cortex-a53.a57": "${armClangCortexA7Cflags}",
		"krait":          "${armClangKraitCflags}",
		"denver":         "${armClangCortexA15Cflags}",
	}
)

type toolchainArm struct {
	toolchain32Bit
	ldflags                               string
	toolchainCflags, toolchainClangCflags string
}

func (t *toolchainArm) Name() string {
	return "arm"
}

func (t *toolchainArm) GccRoot() string {
	return "${armGccRoot}"
}

func (t *toolchainArm) GccTriple() string {
	return "${armGccTriple}"
}

func (t *toolchainArm) GccVersion() string {
	return armGccVersion
}

func (t *toolchainArm) ToolchainCflags() string {
	return t.toolchainCflags
}

func (t *toolchainArm) Cflags() string {
	return "${armCflags}"
}

func (t *toolchainArm) Cppflags() string {
	return "${armCppflags}"
}

func (t *toolchainArm) Ldflags() string {
	return t.ldflags
}

func (t *toolchainArm) IncludeFlags() string {
	return "${armIncludeFlags}"
}

func (t *toolchainArm) InstructionSetFlags(isa string) (string, error) {
	switch isa {
	case "arm":
		return "${armArmCflags}", nil
	case "thumb", "":
		return "${armThumbCflags}", nil
	default:
		return t.toolchainBase.InstructionSetFlags(isa)
	}
}

func (t *toolchainArm) ClangTriple() string {
	return "${armGccTriple}"
}

func (t *toolchainArm) ToolchainClangCflags() string {
	return t.toolchainClangCflags
}

func (t *toolchainArm) ClangCflags() string {
	return "${armClangCflags}"
}

func (t *toolchainArm) ClangCppflags() string {
	return "${armClangCppflags}"
}

func (t *toolchainArm) ClangLdflags() string {
	return t.ldflags
}

func (t *toolchainArm) ClangInstructionSetFlags(isa string) (string, error) {
	switch isa {
	case "arm":
		return "${armClangArmCflags}", nil
	case "thumb", "":
		return "${armClangThumbCflags}", nil
	default:
		return t.toolchainBase.ClangInstructionSetFlags(isa)
	}
}

func (toolchainArm) AddressSanitizerRuntimeLibrary() string {
	return "libclang_rt.asan-arm-android.so"
}

func armToolchainFactory(arch android.Arch) Toolchain {
	var fixCortexA8 string
	toolchainCflags := make([]string, 2, 3)
	toolchainClangCflags := make([]string, 2, 3)

	toolchainCflags[0] = "${armToolchainCflags}"
	toolchainCflags[1] = armArchVariantCflagsVar[arch.ArchVariant]
	toolchainClangCflags[0] = "${armToolchainClangCflags}"
	toolchainClangCflags[1] = armClangArchVariantCflagsVar[arch.ArchVariant]

	switch arch.ArchVariant {
	case "armv7-a-neon":
		switch arch.CpuVariant {
		case "cortex-a8", "":
			// Generic ARM might be a Cortex A8 -- better safe than sorry
			fixCortexA8 = "-Wl,--fix-cortex-a8"
		default:
			fixCortexA8 = "-Wl,--no-fix-cortex-a8"
		}

		toolchainCflags = append(toolchainCflags,
			variantOrDefault(armCpuVariantCflagsVar, arch.CpuVariant))
		toolchainClangCflags = append(toolchainClangCflags,
			variantOrDefault(armClangCpuVariantCflagsVar, arch.CpuVariant))
	case "armv7-a":
		fixCortexA8 = "-Wl,--fix-cortex-a8"
	case "armv5te":
		// Nothing extra for armv5te
	default:
		panic(fmt.Sprintf("Unknown ARM architecture version: %q", arch.ArchVariant))
	}

	return &toolchainArm{
		toolchainCflags: strings.Join(toolchainCflags, " "),
		ldflags: strings.Join([]string{
			"${armLdflags}",
			fixCortexA8,
		}, " "),
		toolchainClangCflags: strings.Join(toolchainClangCflags, " "),
	}
}

func init() {
	registerDeviceToolchainFactory(android.Arm, armToolchainFactory)
}
