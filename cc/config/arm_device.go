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

	armLldflags = armLdflags

	armFixCortexA8LdFlags = []string{"-Wl,--fix-cortex-a8"}

	armNoFixCortexA8LdFlags = []string{"-Wl,--no-fix-cortex-a8"}

	armArmCflags = []string{}

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
		"armv8-2a": []string{
			"-march=armv8.2-a",
			"-mfloat-abi=softfp",
			"-mfpu=neon-fp-armv8",
		},
	}

	armCpuVariantCflags = map[string][]string{
		"cortex-a7": []string{
			"-mcpu=cortex-a7",
			"-mfpu=neon-vfpv4",
			// Fake an ARM compiler flag as these processors support LPAE which clang
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
			// Fake an ARM compiler flag as these processors support LPAE which clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"cortex-a32": []string{
			"-mcpu=cortex-a32",
			"-mfpu=neon-vfpv4",
			// Fake an ARM compiler flag as these processors support LPAE which clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"cortex-a53": []string{
			"-mcpu=cortex-a53",
			"-mfpu=neon-fp-armv8",
			// Fake an ARM compiler flag as these processors support LPAE which clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"cortex-a55": []string{
			"-mcpu=cortex-a55",
			"-mfpu=neon-fp-armv8",
			// Fake an ARM compiler flag as these processors support LPAE which clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"cortex-a75": []string{
			"-mcpu=cortex-a55",
			"-mfpu=neon-fp-armv8",
			// Fake an ARM compiler flag as these processors support LPAE which clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"cortex-a76": []string{
			"-mcpu=cortex-a55",
			"-mfpu=neon-fp-armv8",
			// Fake an ARM compiler flag as these processors support LPAE which clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"krait": []string{
			"-mcpu=krait",
			"-mfpu=neon-vfpv4",
			// Fake an ARM compiler flag as these processors support LPAE which clang
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
			// Fake an ARM compiler flag as these processors support LPAE which clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
		"kryo385": []string{
			// Use cortex-a53 because kryo385 is not supported in clang.
			"-mcpu=cortex-a53",
			// Fake an ARM compiler flag as these processors support LPAE which clang
			// don't advertise.
			// TODO This is a hack and we need to add it for each processor that supports LPAE until some
			// better solution comes around. See Bug 27340895
			"-D__ARM_FEATURE_LPAE=1",
		},
	}
)

const (
	name        = "arm"
	ndkTriple   = "arm-linux-androideabi"
	clangTriple = "armv7a-linux-androideabi"
)

func init() {
	// Just exported. Not created as a Ninja static variable.
	exportedVars.ExportString("ArmClangTriple", clangTriple)

	exportedVars.ExportStringListStaticVariable("ArmLdflags", armLdflags)
	exportedVars.ExportStringListStaticVariable("ArmLldflags", armLldflags)

	exportedVars.ExportStringListStaticVariable("ArmFixCortexA8LdFlags", armFixCortexA8LdFlags)
	exportedVars.ExportStringListStaticVariable("ArmNoFixCortexA8LdFlags", armNoFixCortexA8LdFlags)

	// Clang cflags
	exportedVars.ExportStringListStaticVariable("ArmToolchainCflags", armToolchainCflags)
	exportedVars.ExportStringListStaticVariable("ArmCflags", armCflags)
	exportedVars.ExportStringListStaticVariable("ArmCppflags", armCppflags)

	// Clang ARM vs. Thumb instruction set cflags
	exportedVars.ExportStringListStaticVariable("ArmArmCflags", armArmCflags)
	exportedVars.ExportStringListStaticVariable("ArmThumbCflags", armThumbCflags)

	exportedVars.ExportVariableReferenceDict("ArmArchVariantCflags", armArchVariantCflagsVar)
	exportedVars.ExportVariableReferenceDict("ArmCpuVariantCflags", armCpuVariantCflagsVar)

	// Clang arch variant cflags
	exportedVars.ExportStringListStaticVariable("ArmArmv7ACflags", armArchVariantCflags["armv7-a"])
	exportedVars.ExportStringListStaticVariable("ArmArmv7ANeonCflags", armArchVariantCflags["armv7-a-neon"])
	exportedVars.ExportStringListStaticVariable("ArmArmv8ACflags", armArchVariantCflags["armv8-a"])
	exportedVars.ExportStringListStaticVariable("ArmArmv82ACflags", armArchVariantCflags["armv8-2a"])

	// Clang cpu variant cflags
	exportedVars.ExportStringListStaticVariable("ArmGenericCflags", armCpuVariantCflags[""])
	exportedVars.ExportStringListStaticVariable("ArmCortexA7Cflags", armCpuVariantCflags["cortex-a7"])
	exportedVars.ExportStringListStaticVariable("ArmCortexA8Cflags", armCpuVariantCflags["cortex-a8"])
	exportedVars.ExportStringListStaticVariable("ArmCortexA15Cflags", armCpuVariantCflags["cortex-a15"])
	exportedVars.ExportStringListStaticVariable("ArmCortexA32Cflags", armCpuVariantCflags["cortex-a32"])
	exportedVars.ExportStringListStaticVariable("ArmCortexA53Cflags", armCpuVariantCflags["cortex-a53"])
	exportedVars.ExportStringListStaticVariable("ArmCortexA55Cflags", armCpuVariantCflags["cortex-a55"])
	exportedVars.ExportStringListStaticVariable("ArmKraitCflags", armCpuVariantCflags["krait"])
	exportedVars.ExportStringListStaticVariable("ArmKryoCflags", armCpuVariantCflags["kryo"])
}

var (
	armArchVariantCflagsVar = map[string]string{
		"armv7-a":      "${config.ArmArmv7ACflags}",
		"armv7-a-neon": "${config.ArmArmv7ANeonCflags}",
		"armv8-a":      "${config.ArmArmv8ACflags}",
		"armv8-2a":     "${config.ArmArmv82ACflags}",
	}

	armCpuVariantCflagsVar = map[string]string{
		"":               "${config.ArmGenericCflags}",
		"cortex-a7":      "${config.ArmCortexA7Cflags}",
		"cortex-a8":      "${config.ArmCortexA8Cflags}",
		"cortex-a9":      "${config.ArmGenericCflags}",
		"cortex-a15":     "${config.ArmCortexA15Cflags}",
		"cortex-a32":     "${config.ArmCortexA32Cflags}",
		"cortex-a53":     "${config.ArmCortexA53Cflags}",
		"cortex-a53.a57": "${config.ArmCortexA53Cflags}",
		"cortex-a55":     "${config.ArmCortexA55Cflags}",
		"cortex-a72":     "${config.ArmCortexA53Cflags}",
		"cortex-a73":     "${config.ArmCortexA53Cflags}",
		"cortex-a75":     "${config.ArmCortexA55Cflags}",
		"cortex-a76":     "${config.ArmCortexA55Cflags}",
		"krait":          "${config.ArmKraitCflags}",
		"kryo":           "${config.ArmKryoCflags}",
		"kryo385":        "${config.ArmCortexA53Cflags}",
		"exynos-m1":      "${config.ArmCortexA53Cflags}",
		"exynos-m2":      "${config.ArmCortexA53Cflags}",
	}
)

type toolchainArm struct {
	toolchainBionic
	toolchain32Bit
	ldflags         string
	lldflags        string
	toolchainCflags string
}

func (t *toolchainArm) Name() string {
	return name
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
	return ndkTriple
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

func (t *toolchainArm) Lldflags() string {
	return t.lldflags // TODO: handle V8 cases
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

func (toolchainArm) LibclangRuntimeLibraryArch() string {
	return name
}

func armToolchainFactory(arch android.Arch) Toolchain {
	var fixCortexA8 string
	toolchainCflags := make([]string, 2, 3)

	toolchainCflags[0] = "${config.ArmToolchainCflags}"
	toolchainCflags[1] = armArchVariantCflagsVar[arch.ArchVariant]

	toolchainCflags = append(toolchainCflags,
		variantOrDefault(armCpuVariantCflagsVar, arch.CpuVariant))

	switch arch.ArchVariant {
	case "armv7-a-neon":
		switch arch.CpuVariant {
		case "cortex-a8", "":
			// Generic ARM might be a Cortex A8 -- better safe than sorry
			fixCortexA8 = "${config.ArmFixCortexA8LdFlags}"
		default:
			fixCortexA8 = "${config.ArmNoFixCortexA8LdFlags}"
		}
	case "armv7-a":
		fixCortexA8 = "${config.ArmFixCortexA8LdFlags}"
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
		lldflags:        "${config.ArmLldflags}",
		toolchainCflags: strings.Join(toolchainCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.Arm, armToolchainFactory)
}
