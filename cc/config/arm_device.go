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
	armToolchainCflags = []string{
		"-mthumb-interwork",
		"-msoft-float",
	}

	armCflags = []string{
		"-fomit-frame-pointer",
	}

	armCppflags = []string{}

	armLdflags = []string{
		"-Wl,--icf=safe",
		"-Wl,--hash-style=gnu",
		"-Wl,-m,armelf",
	}

	armArmCflags = []string{
		"-fstrict-aliasing",
	}

	armThumbCflags = []string{
		"-mthumb",
		"-Os",
	}

	armArchVariantCflags = map[string][]string{
		"armv7-a": []string{
			"-march=armv7-a",
			"-mfloat-abi=softfp",
			"-mfpu=vfpv3-d16",
		},
		"armv7-a-neon": []string{
			"-march=armv7-a",
			"-mfloat-abi=softfp",
			"-mfpu=neon",
		},
		"armv8-a": []string{
			"-march=armv8-a",
			"-mfloat-abi=softfp",
			"-mfpu=neon-fp-armv8",
		},
	}

	armCpuVariantCflags = map[string][]string{
		"cortex-a7": []string{
			"-mcpu=cortex-a7",
			"-mfpu=neon-vfpv4",
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
			"-mfpu=neon-vfpv4",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"cortex-a53": []string{
			"-mcpu=cortex-a53",
			"-mfpu=neon-fp-armv8",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"cortex-a55": []string{
			"-mcpu=cortex-a55",
			"-mfpu=neon-fp-armv8",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"cortex-a75": []string{
			"-mcpu=cortex-a55",
			"-mfpu=neon-fp-armv8",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"krait": []string{
			"-mcpu=cortex-a15",
			"-mfpu=neon-vfpv4",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"kryo": []string{
			// Use cortex-a53 because the GNU assembler doesn't recognize -mcpu=kryo
			// even though clang does.
			"-mcpu=cortex-a53",
			"-mfpu=neon-fp-armv8",
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
	android.RegisterArchFeatures(android.Arm,
		"neon")

	android.RegisterArchVariants(android.Arm,
		"armv7-a",
		"armv7-a-neon",
		"armv8-a",
		"cortex-a7",
		"cortex-a8",
		"cortex-a9",
		"cortex-a15",
		"cortex-a53",
		"cortex-a53-a57",
		"cortex-a55",
		"cortex-a73",
		"cortex-a75",
		"krait",
		"kryo",
		"exynos-m1",
		"exynos-m2",
		"denver")

	android.RegisterArchVariantFeatures(android.Arm, "armv7-a-neon", "neon")
	android.RegisterArchVariantFeatures(android.Arm, "armv8-a", "neon")

	// Krait is not supported by GCC, but is supported by Clang, so
	// override the definitions when building modules with Clang.
	replaceFirst(armClangCpuVariantCflags["krait"], "-mcpu=cortex-a15", "-mcpu=krait")

	// The reason we use "-march=armv8-a+crc", instead of "-march=armv8-a", for
	// gcc is the latter would conflict with any specified/supported -mcpu!
	// All armv8-a cores supported by gcc 4.9 support crc, so it's safe
	// to add +crc. Besides, the use of gcc is only for legacy code.
	replaceFirst(armArchVariantCflags["armv8-a"], "-march=armv8-a", "-march=armv8-a+crc")

	pctx.StaticVariable("armGccVersion", armGccVersion)

	pctx.SourcePathVariable("ArmGccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/arm/arm-linux-androideabi-${armGccVersion}")

	pctx.StaticVariable("ArmToolchainCflags", strings.Join(armToolchainCflags, " "))
	pctx.StaticVariable("ArmCflags", strings.Join(armCflags, " "))
	pctx.StaticVariable("ArmLdflags", strings.Join(armLdflags, " "))
	pctx.StaticVariable("ArmCppflags", strings.Join(armCppflags, " "))
	pctx.StaticVariable("ArmIncludeFlags", bionicHeaders("arm"))

	// Extended cflags

	// ARM vs. Thumb instruction set flags
	pctx.StaticVariable("ArmArmCflags", strings.Join(armArmCflags, " "))
	pctx.StaticVariable("ArmThumbCflags", strings.Join(armThumbCflags, " "))

	// Architecture variant cflags
	pctx.StaticVariable("ArmArmv7ACflags", strings.Join(armArchVariantCflags["armv7-a"], " "))
	pctx.StaticVariable("ArmArmv7ANeonCflags", strings.Join(armArchVariantCflags["armv7-a-neon"], " "))
	pctx.StaticVariable("ArmArmv8ACflags", strings.Join(armArchVariantCflags["armv8-a"], " "))

	// Cpu variant cflags
	pctx.StaticVariable("ArmGenericCflags", strings.Join(armCpuVariantCflags[""], " "))
	pctx.StaticVariable("ArmCortexA7Cflags", strings.Join(armCpuVariantCflags["cortex-a7"], " "))
	pctx.StaticVariable("ArmCortexA8Cflags", strings.Join(armCpuVariantCflags["cortex-a8"], " "))
	pctx.StaticVariable("ArmCortexA15Cflags", strings.Join(armCpuVariantCflags["cortex-a15"], " "))
	pctx.StaticVariable("ArmCortexA53Cflags", strings.Join(armCpuVariantCflags["cortex-a53"], " "))
	pctx.StaticVariable("ArmCortexA55Cflags", strings.Join(armCpuVariantCflags["cortex-a55"], " "))
	pctx.StaticVariable("ArmKraitCflags", strings.Join(armCpuVariantCflags["krait"], " "))
	pctx.StaticVariable("ArmKryoCflags", strings.Join(armCpuVariantCflags["kryo"], " "))

	// Clang cflags
	pctx.StaticVariable("ArmToolchainClangCflags", strings.Join(ClangFilterUnknownCflags(armToolchainCflags), " "))
	pctx.StaticVariable("ArmClangCflags", strings.Join(ClangFilterUnknownCflags(armCflags), " "))
	pctx.StaticVariable("ArmClangLdflags", strings.Join(ClangFilterUnknownCflags(armLdflags), " "))
	pctx.StaticVariable("ArmClangCppflags", strings.Join(ClangFilterUnknownCflags(armCppflags), " "))

	// Clang ARM vs. Thumb instruction set cflags
	pctx.StaticVariable("ArmClangArmCflags", strings.Join(ClangFilterUnknownCflags(armArmCflags), " "))
	pctx.StaticVariable("ArmClangThumbCflags", strings.Join(ClangFilterUnknownCflags(armThumbCflags), " "))

	// Clang arch variant cflags
	pctx.StaticVariable("ArmClangArmv7ACflags",
		strings.Join(armClangArchVariantCflags["armv7-a"], " "))
	pctx.StaticVariable("ArmClangArmv7ANeonCflags",
		strings.Join(armClangArchVariantCflags["armv7-a-neon"], " "))
	pctx.StaticVariable("ArmClangArmv8ACflags",
		strings.Join(armClangArchVariantCflags["armv8-a"], " "))

	// Clang cpu variant cflags
	pctx.StaticVariable("ArmClangGenericCflags",
		strings.Join(armClangCpuVariantCflags[""], " "))
	pctx.StaticVariable("ArmClangCortexA7Cflags",
		strings.Join(armClangCpuVariantCflags["cortex-a7"], " "))
	pctx.StaticVariable("ArmClangCortexA8Cflags",
		strings.Join(armClangCpuVariantCflags["cortex-a8"], " "))
	pctx.StaticVariable("ArmClangCortexA15Cflags",
		strings.Join(armClangCpuVariantCflags["cortex-a15"], " "))
	pctx.StaticVariable("ArmClangCortexA53Cflags",
		strings.Join(armClangCpuVariantCflags["cortex-a53"], " "))
	pctx.StaticVariable("ArmClangCortexA55Cflags",
		strings.Join(armClangCpuVariantCflags["cortex-a55"], " "))
	pctx.StaticVariable("ArmClangKraitCflags",
		strings.Join(armClangCpuVariantCflags["krait"], " "))
	pctx.StaticVariable("ArmClangKryoCflags",
		strings.Join(armClangCpuVariantCflags["kryo"], " "))
}

var (
	armArchVariantCflagsVar = map[string]string{
		"armv7-a":      "${config.ArmArmv7ACflags}",
		"armv7-a-neon": "${config.ArmArmv7ANeonCflags}",
		"armv8-a":      "${config.ArmArmv8ACflags}",
	}

	armCpuVariantCflagsVar = map[string]string{
		"":               "${config.ArmGenericCflags}",
		"cortex-a7":      "${config.ArmCortexA7Cflags}",
		"cortex-a8":      "${config.ArmCortexA8Cflags}",
		"cortex-a15":     "${config.ArmCortexA15Cflags}",
		"cortex-a53":     "${config.ArmCortexA53Cflags}",
		"cortex-a53.a57": "${config.ArmCortexA53Cflags}",
		"cortex-a55":     "${config.ArmCortexA55Cflags}",
		"cortex-a73":     "${config.ArmCortexA53Cflags}",
		"cortex-a75":     "${config.ArmCortexA55Cflags}",
		"krait":          "${config.ArmKraitCflags}",
		"kryo":           "${config.ArmKryoCflags}",
		"exynos-m1":      "${config.ArmCortexA53Cflags}",
		"exynos-m2":      "${config.ArmCortexA53Cflags}",
		"denver":         "${config.ArmCortexA15Cflags}",
	}

	armClangArchVariantCflagsVar = map[string]string{
		"armv7-a":      "${config.ArmClangArmv7ACflags}",
		"armv7-a-neon": "${config.ArmClangArmv7ANeonCflags}",
		"armv8-a":      "${config.ArmClangArmv8ACflags}",
	}

	armClangCpuVariantCflagsVar = map[string]string{
		"":               "${config.ArmClangGenericCflags}",
		"cortex-a7":      "${config.ArmClangCortexA7Cflags}",
		"cortex-a8":      "${config.ArmClangCortexA8Cflags}",
		"cortex-a15":     "${config.ArmClangCortexA15Cflags}",
		"cortex-a53":     "${config.ArmClangCortexA53Cflags}",
		"cortex-a53.a57": "${config.ArmClangCortexA53Cflags}",
		"cortex-a55":     "${config.ArmClangCortexA55Cflags}",
		"cortex-a73":     "${config.ArmClangCortexA53Cflags}",
		"cortex-a75":     "${config.ArmClangCortexA55Cflags}",
		"krait":          "${config.ArmClangKraitCflags}",
		"kryo":           "${config.ArmClangKryoCflags}",
		"exynos-m1":      "${config.ArmClangCortexA53Cflags}",
		"exynos-m2":      "${config.ArmClangCortexA53Cflags}",
		"denver":         "${config.ArmClangCortexA15Cflags}",
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
	return "${config.ArmGccRoot}"
}

func (t *toolchainArm) GccTriple() string {
	return "arm-linux-androideabi"
}

func (t *toolchainArm) GccVersion() string {
	return armGccVersion
}

func (t *toolchainArm) ToolchainCflags() string {
	return t.toolchainCflags
}

func (t *toolchainArm) Cflags() string {
	return "${config.ArmCflags}"
}

func (t *toolchainArm) Cppflags() string {
	return "${config.ArmCppflags}"
}

func (t *toolchainArm) Ldflags() string {
	return t.ldflags
}

func (t *toolchainArm) IncludeFlags() string {
	return "${config.ArmIncludeFlags}"
}

func (t *toolchainArm) InstructionSetFlags(isa string) (string, error) {
	switch isa {
	case "arm":
		return "${config.ArmArmCflags}", nil
	case "thumb", "":
		return "${config.ArmThumbCflags}", nil
	default:
		return t.toolchainBase.InstructionSetFlags(isa)
	}
}

func (t *toolchainArm) ClangTriple() string {
	return t.GccTriple()
}

func (t *toolchainArm) ToolchainClangCflags() string {
	return t.toolchainClangCflags
}

func (t *toolchainArm) ClangCflags() string {
	return "${config.ArmClangCflags}"
}

func (t *toolchainArm) ClangCppflags() string {
	return "${config.ArmClangCppflags}"
}

func (t *toolchainArm) ClangLdflags() string {
	return t.ldflags
}

func (t *toolchainArm) ClangInstructionSetFlags(isa string) (string, error) {
	switch isa {
	case "arm":
		return "${config.ArmClangArmCflags}", nil
	case "thumb", "":
		return "${config.ArmClangThumbCflags}", nil
	default:
		return t.toolchainBase.ClangInstructionSetFlags(isa)
	}
}

func (toolchainArm) SanitizerRuntimeLibraryArch() string {
	return "arm"
}

func armToolchainFactory(arch android.Arch) Toolchain {
	var fixCortexA8 string
	toolchainCflags := make([]string, 2, 3)
	toolchainClangCflags := make([]string, 2, 3)

	toolchainCflags[0] = "${config.ArmToolchainCflags}"
	toolchainCflags[1] = armArchVariantCflagsVar[arch.ArchVariant]
	toolchainClangCflags[0] = "${config.ArmToolchainClangCflags}"
	toolchainClangCflags[1] = armClangArchVariantCflagsVar[arch.ArchVariant]

	toolchainCflags = append(toolchainCflags,
		variantOrDefault(armCpuVariantCflagsVar, arch.CpuVariant))
	toolchainClangCflags = append(toolchainClangCflags,
		variantOrDefault(armClangCpuVariantCflagsVar, arch.CpuVariant))

	switch arch.ArchVariant {
	case "armv7-a-neon":
		switch arch.CpuVariant {
		case "cortex-a8", "":
			// Generic ARM might be a Cortex A8 -- better safe than sorry
			fixCortexA8 = "-Wl,--fix-cortex-a8"
		default:
			fixCortexA8 = "-Wl,--no-fix-cortex-a8"
		}
	case "armv7-a":
		fixCortexA8 = "-Wl,--fix-cortex-a8"
	case "armv8-a":
		// Nothing extra for armv8-a
	default:
		panic(fmt.Sprintf("Unknown ARM architecture version: %q", arch.ArchVariant))
	}

	return &toolchainArm{
		toolchainCflags: strings.Join(toolchainCflags, " "),
		ldflags: strings.Join([]string{
			"${config.ArmLdflags}",
			fixCortexA8,
		}, " "),
		toolchainClangCflags: strings.Join(toolchainClangCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.Arm, armToolchainFactory)
}
