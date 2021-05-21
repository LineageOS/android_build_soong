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
		"-Wl,--hash-style=gnu",
		"-Wl,-m,armelf",
	}

	armLldflags = ClangFilterUnknownLldflags(armLdflags)

	armArmCflags = []string{
		"-fstrict-aliasing",
	}

	armThumbCflags = []string{
		"-mthumb",
		"-Os",
	}

	armClangArchVariantCflags = map[string][]string{
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
		"armv8-2a": []string{
			"-march=armv8.2-a",
			"-mfloat-abi=softfp",
			"-mfpu=neon-fp-armv8",
		},
	}

	armClangCpuVariantCflags = map[string][]string{
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
		"cortex-a76": []string{
			"-mcpu=cortex-a55",
			"-mfpu=neon-fp-armv8",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"krait": []string{
			"-mcpu=krait",
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
		"kryo385": []string{
			// Use cortex-a53 because kryo385 is not supported in GCC/clang.
			"-mcpu=cortex-a53",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
	}
)

const (
	name          = "arm"
	armGccVersion = "4.9"
	gccTriple     = "arm-linux-androideabi"
	clangTriple   = "armv7a-linux-androideabi"
)

func init() {
	pctx.StaticVariable("armGccVersion", armGccVersion)

	pctx.SourcePathVariable("ArmGccRoot", "prebuilts/gcc/${HostPrebuiltTag}/arm/arm-linux-androideabi-${armGccVersion}")

	// Just exported. Not created as a Ninja static variable.
	exportedStringVars.Set("ArmClangTriple", clangTriple)

	exportStringListStaticVariable("ArmLdflags", armLdflags)
	exportStringListStaticVariable("ArmLldflags", armLldflags)

	// Clang cflags
	exportStringListStaticVariable("ArmToolchainClangCflags", ClangFilterUnknownCflags(armToolchainCflags))
	exportStringListStaticVariable("ArmClangCflags", ClangFilterUnknownCflags(armCflags))
	exportStringListStaticVariable("ArmClangLdflags", ClangFilterUnknownCflags(armLdflags))
	exportStringListStaticVariable("ArmClangLldflags", ClangFilterUnknownCflags(armLldflags))
	exportStringListStaticVariable("ArmClangCppflags", ClangFilterUnknownCflags(armCppflags))

	// Clang ARM vs. Thumb instruction set cflags
	exportStringListStaticVariable("ArmClangArmCflags", ClangFilterUnknownCflags(armArmCflags))
	exportStringListStaticVariable("ArmClangThumbCflags", ClangFilterUnknownCflags(armThumbCflags))

	// Clang arch variant cflags
	exportStringListStaticVariable("ArmClangArmv7ACflags", armClangArchVariantCflags["armv7-a"])
	exportStringListStaticVariable("ArmClangArmv7ANeonCflags", armClangArchVariantCflags["armv7-a-neon"])
	exportStringListStaticVariable("ArmClangArmv8ACflags", armClangArchVariantCflags["armv8-a"])
	exportStringListStaticVariable("ArmClangArmv82ACflags", armClangArchVariantCflags["armv8-2a"])

	// Clang cpu variant cflags
	exportStringListStaticVariable("ArmClangGenericCflags", armClangCpuVariantCflags[""])
	exportStringListStaticVariable("ArmClangCortexA7Cflags", armClangCpuVariantCflags["cortex-a7"])
	exportStringListStaticVariable("ArmClangCortexA8Cflags", armClangCpuVariantCflags["cortex-a8"])
	exportStringListStaticVariable("ArmClangCortexA15Cflags", armClangCpuVariantCflags["cortex-a15"])
	exportStringListStaticVariable("ArmClangCortexA53Cflags", armClangCpuVariantCflags["cortex-a53"])
	exportStringListStaticVariable("ArmClangCortexA55Cflags", armClangCpuVariantCflags["cortex-a55"])
	exportStringListStaticVariable("ArmClangKraitCflags", armClangCpuVariantCflags["krait"])
	exportStringListStaticVariable("ArmClangKryoCflags", armClangCpuVariantCflags["kryo"])
}

var (
	armClangArchVariantCflagsVar = map[string]string{
		"armv7-a":      "${config.ArmClangArmv7ACflags}",
		"armv7-a-neon": "${config.ArmClangArmv7ANeonCflags}",
		"armv8-a":      "${config.ArmClangArmv8ACflags}",
		"armv8-2a":     "${config.ArmClangArmv82ACflags}",
	}

	armClangCpuVariantCflagsVar = map[string]string{
		"":               "${config.ArmClangGenericCflags}",
		"cortex-a7":      "${config.ArmClangCortexA7Cflags}",
		"cortex-a8":      "${config.ArmClangCortexA8Cflags}",
		"cortex-a15":     "${config.ArmClangCortexA15Cflags}",
		"cortex-a53":     "${config.ArmClangCortexA53Cflags}",
		"cortex-a53.a57": "${config.ArmClangCortexA53Cflags}",
		"cortex-a55":     "${config.ArmClangCortexA55Cflags}",
		"cortex-a72":     "${config.ArmClangCortexA53Cflags}",
		"cortex-a73":     "${config.ArmClangCortexA53Cflags}",
		"cortex-a75":     "${config.ArmClangCortexA55Cflags}",
		"cortex-a76":     "${config.ArmClangCortexA55Cflags}",
		"krait":          "${config.ArmClangKraitCflags}",
		"kryo":           "${config.ArmClangKryoCflags}",
		"kryo385":        "${config.ArmClangCortexA53Cflags}",
		"exynos-m1":      "${config.ArmClangCortexA53Cflags}",
		"exynos-m2":      "${config.ArmClangCortexA53Cflags}",
	}
)

type toolchainArm struct {
	toolchain32Bit
	ldflags              string
	lldflags             string
	toolchainClangCflags string
}

func (t *toolchainArm) Name() string {
	return name
}

func (t *toolchainArm) GccRoot() string {
	return "${config.ArmGccRoot}"
}

func (t *toolchainArm) GccTriple() string {
	return gccTriple
}

func (t *toolchainArm) GccVersion() string {
	return armGccVersion
}

func (t *toolchainArm) IncludeFlags() string {
	return ""
}

func (t *toolchainArm) ClangTriple() string {
	// http://b/72619014 work around llvm LTO bug.
	return clangTriple
}

func (t *toolchainArm) ndkTriple() string {
	// Use current NDK include path, while ClangTriple is changed.
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

func (t *toolchainArm) ClangLldflags() string {
	return t.lldflags // TODO: handle V8 cases
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

func (toolchainArm) LibclangRuntimeLibraryArch() string {
	return name
}

func armToolchainFactory(arch android.Arch) Toolchain {
	var fixCortexA8 string
	toolchainClangCflags := make([]string, 2, 3)

	toolchainClangCflags[0] = "${config.ArmToolchainClangCflags}"
	toolchainClangCflags[1] = armClangArchVariantCflagsVar[arch.ArchVariant]

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
	case "armv8-a", "armv8-2a":
		// Nothing extra for armv8-a/armv8-2a
	default:
		panic(fmt.Sprintf("Unknown ARM architecture version: %q", arch.ArchVariant))
	}

	return &toolchainArm{
		ldflags: strings.Join([]string{
			"${config.ArmLdflags}",
			fixCortexA8,
		}, " "),
		lldflags:             "${config.ArmLldflags}",
		toolchainClangCflags: strings.Join(toolchainClangCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.Arm, armToolchainFactory)
}
