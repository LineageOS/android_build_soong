package cc

import (
	"fmt"
	"strings"

	"android/soong/common"
)

var (
	armCflags = []string{
		"-fno-exceptions", // from build/core/combo/select.mk
		"-Wno-multichar",  // from build/core/combo/select.mk
		"-msoft-float",
		"-ffunction-sections",
		"-fdata-sections",
		"-funwind-tables",
		"-fstack-protector",
		"-Wa,--noexecstack",
		"-Werror=format-security",
		"-D_FORTIFY_SOURCE=2",
		"-fno-short-enums",
		"-no-canonical-prefixes",
		"-fno-canonical-system-headers",
		"-include ${SrcDir}/build/core/combo/include/arch/linux-arm/AndroidConfig.h",

		"-fno-builtin-sin",
		"-fno-strict-volatile-bitfields",

		"-mthumb-interwork",

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
		"cortex-a7": []string{
			"-mcpu=cortex-a7",
		},
		"cortex-a8": []string{
			"-mcpu=cortex-a8",
		},
		"cortex-a15": []string{
			"-mcpu=cortex-a15",
			// Fake an ARM compiler flag as these processors support LPAE which GCC/clang
			// don't advertise.
			"-D__ARM_FEATURE_LPAE=1",
		},
	}

	armClangCpuVariantCflags  = copyVariantFlags(armCpuVariantCflags)
	armClangArchVariantCflags = copyVariantFlags(armArchVariantCflags)
)

func copyVariantFlags(m map[string][]string) map[string][]string {
	ret := make(map[string][]string, len(m))
	for k, v := range m {
		l := make([]string, len(m[k]))
		for i := range m[k] {
			l[i] = v[i]
		}
		ret[k] = l
	}
	return ret
}

func init() {
	replaceFirst := func(slice []string, from, to string) {
		if slice[0] != from {
			panic(fmt.Errorf("Expected %q, found %q", from, to))
		}

		slice[0] = to
	}

	replaceFirst(armClangArchVariantCflags["armv5te"], "-march=armv5te", "-march=armv5t")
	armClangCpuVariantCflags["krait"] = []string{
		"-mcpu=krait",
	}

	pctx.StaticVariable("armGccVersion", "4.9")

	pctx.StaticVariable("armGccRoot",
		"${SrcDir}/prebuilts/gcc/${HostPrebuiltTag}/arm/arm-linux-androideabi-${armGccVersion}")

	pctx.StaticVariable("armGccTriple", "arm-linux-androideabi")

	pctx.StaticVariable("armCflags", strings.Join(armCflags, " "))
	pctx.StaticVariable("armLdflags", strings.Join(armLdflags, " "))
	pctx.StaticVariable("armCppflags", strings.Join(armCppflags, " "))
	pctx.StaticVariable("armIncludeFlags", strings.Join([]string{
		"-isystem ${LibcRoot}/arch-arm/include",
		"-isystem ${LibcRoot}/include",
		"-isystem ${LibcRoot}/kernel/uapi",
		"-isystem ${LibcRoot}/kernel/uapi/asm-arm",
		"-isystem ${LibmRoot}/include",
		"-isystem ${LibmRoot}/include/arm",
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
	pctx.StaticVariable("armCortexA7Cflags", strings.Join(armCpuVariantCflags["cortex-a7"], " "))
	pctx.StaticVariable("armCortexA8Cflags", strings.Join(armCpuVariantCflags["cortex-a8"], " "))
	pctx.StaticVariable("armCortexA15Cflags", strings.Join(armCpuVariantCflags["cortex-a15"], " "))

	// Clang cflags
	pctx.StaticVariable("armClangCflags", strings.Join(clangFilterUnknownCflags(armCflags), " "))
	pctx.StaticVariable("armClangLdflags", strings.Join(clangFilterUnknownCflags(armLdflags), " "))
	pctx.StaticVariable("armClangCppflags", strings.Join(clangFilterUnknownCflags(armCppflags), " "))

	// Clang cpu variant cflags
	pctx.StaticVariable("armClangArmv5TECflags",
		strings.Join(armClangArchVariantCflags["armv5te"], " "))
	pctx.StaticVariable("armClangArmv7ACflags",
		strings.Join(armClangArchVariantCflags["armv7-a"], " "))
	pctx.StaticVariable("armClangArmv7ANeonCflags",
		strings.Join(armClangArchVariantCflags["armv7-a-neon"], " "))

	// Clang cpu variant cflags
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
		"":           "",
		"cortex-a7":  "${armCortexA7Cflags}",
		"cortex-a8":  "${armCortexA8Cflags}",
		"cortex-a15": "${armCortexA15Cflags}",
		"krait":      "${armCortexA15Cflags}",
		"denver":     "${armCortexA15Cflags}",
	}

	armClangArchVariantCflagsVar = map[string]string{
		"armv5te":      "${armClangArmv5TECflags}",
		"armv7-a":      "${armClangArmv7ACflags}",
		"armv7-a-neon": "${armClangArmv7ANeonCflags}",
	}

	armClangCpuVariantCflagsVar = map[string]string{
		"":           "",
		"cortex-a7":  "${armClangCortexA7Cflags}",
		"cortex-a8":  "${armClangCortexA8Cflags}",
		"cortex-a15": "${armClangCortexA15Cflags}",
		"krait":      "${armClangKraitCflags}",
		"denver":     "${armClangCortexA15Cflags}",
	}
)

type toolchainArm struct {
	toolchain32Bit
	cflags, ldflags, clangCflags string
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
	return "${armGccVersion}"
}

func (t *toolchainArm) Cflags() string {
	return t.cflags
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

func (t *toolchainArm) ClangCflags() string {
	return t.clangCflags
}

func (t *toolchainArm) ClangCppflags() string {
	return "${armClangCppflags}"
}

func (t *toolchainArm) ClangLdflags() string {
	return t.ldflags
}

func armToolchainFactory(archVariant string, cpuVariant string) Toolchain {
	var fixCortexA8 string
	switch cpuVariant {
	case "cortex-a8", "":
		// Generic ARM might be a Cortex A8 -- better safe than sorry
		fixCortexA8 = "-Wl,--fix-cortex-a8"
	default:
		fixCortexA8 = "-Wl,--no-fix-cortex-a8"
	}

	return &toolchainArm{
		cflags: strings.Join([]string{
			"${armCflags}",
			armArchVariantCflagsVar[archVariant],
			armCpuVariantCflagsVar[cpuVariant],
		}, " "),
		ldflags: strings.Join([]string{
			"${armLdflags}",
			fixCortexA8,
		}, " "),
		clangCflags: strings.Join([]string{
			"${armClangCflags}",
			armClangArchVariantCflagsVar[archVariant],
			armClangCpuVariantCflagsVar[cpuVariant],
		}, " "),
	}
}

func init() {
	registerToolchainFactory(common.Device, common.Arm, armToolchainFactory)
}
