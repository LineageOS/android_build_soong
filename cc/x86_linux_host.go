package cc

import (
	"runtime"
	"strings"

	"android/soong/common"
)

var (
	linuxCflags = []string{
		"-fno-exceptions", // from build/core/combo/select.mk
		"-Wno-multichar",  // from build/core/combo/select.mk

		"-Wa,--noexecstack",

		"-fPIC",
		"-no-canonical-prefixes",
		"-include ${SrcDir}/build/core/combo/include/arch/linux-x86/AndroidConfig.h",

		// Disable new longjmp in glibc 2.11 and later. See bug 2967937. Same for 2.15?
		"-U_FORTIFY_SOURCE",
		"-D_FORTIFY_SOURCE=0",

		// Workaround differences in inttypes.h between host and target.
		//See bug 12708004.
		"-D__STDC_FORMAT_MACROS",
		"-D__STDC_CONSTANT_MACROS",

		// HOST_RELEASE_CFLAGS
		"-O2", // from build/core/combo/select.mk
		"-g",  // from build/core/combo/select.mk
		"-fno-strict-aliasing", // from build/core/combo/select.mk
	}

	linuxLdflags = []string{
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
	}

	// Extended cflags
	linuxX86Cflags = []string{
		"-msse3",
		"-mfpmath=sse",
		"-m32",
		"-march=prescott",
	}

	linuxX8664Cflags = []string{
		"-m64",
	}

	linuxX86Ldflags = []string{
		"-m32",
		`-Wl,-rpath,\$$ORIGIN/../lib`,
	}

	linuxX8664Ldflags = []string{
		"-m64",
		`-Wl,-rpath,\$$ORIGIN/../lib64`,
	}

	linuxClangCflags = append([]string{
		"--gcc-toolchain=${linuxGccRoot}",
		"--sysroot=${linuxGccRoot}/sysroot",
	}, clangFilterUnknownCflags(linuxCflags)...)

	linuxClangLdflags = append([]string{
		"--gcc-toolchain=${linuxGccRoot}",
		"--sysroot=${linuxGccRoot}/sysroot",
	}, clangFilterUnknownCflags(linuxLdflags)...)

	linuxX86ClangLdflags = append([]string{
		"-B${linuxGccRoot}/lib/gcc/${linuxGccTriple}/${linuxGccVersion}/32",
		"-L${linuxGccRoot}/lib/gcc/${linuxGccTriple}/${linuxGccVersion}/32",
		"-L${linuxGccRoot}/${linuxGccTriple}/lib32",
	}, clangFilterUnknownCflags(linuxX86Ldflags)...)

	linuxX8664ClangLdflags = append([]string{
		"-B${linuxGccRoot}/lib/gcc/${linuxGccTriple}/${linuxGccVersion}",
		"-L${linuxGccRoot}/lib/gcc/${linuxGccTriple}/${linuxGccVersion}",
		"-L${linuxGccRoot}/${linuxGccTriple}/lib64",
	}, clangFilterUnknownCflags(linuxX8664Ldflags)...)

	linuxClangCppflags = []string{
		"-isystem ${linuxGccRoot}/${linuxGccTriple}/include/c++/${linuxGccVersion}",
		"-isystem ${linuxGccRoot}/${linuxGccTriple}/include/c++/${linuxGccVersion}/backward",
	}

	linuxX86ClangCppflags = []string{
		"-isystem ${linuxGccRoot}/${linuxGccTriple}/include/c++/${linuxGccVersion}/${linuxGccTriple}/32",
	}

	linuxX8664ClangCppflags = []string{
		"-isystem ${linuxGccRoot}/${linuxGccTriple}/include/c++/${linuxGccVersion}/${linuxGccTriple}",
	}
)

func init() {
	pctx.StaticVariable("linuxGccVersion", "4.8")

	pctx.StaticVariable("linuxGccRoot",
		"${SrcDir}/prebuilts/gcc/${HostPrebuiltTag}/host/x86_64-linux-glibc2.15-${linuxGccVersion}")

	pctx.StaticVariable("linuxGccTriple", "x86_64-linux")

	pctx.StaticVariable("linuxCflags", strings.Join(linuxCflags, " "))
	pctx.StaticVariable("linuxLdflags", strings.Join(linuxLdflags, " "))

	pctx.StaticVariable("linuxClangCflags", strings.Join(linuxClangCflags, " "))
	pctx.StaticVariable("linuxClangLdflags", strings.Join(linuxClangLdflags, " "))
	pctx.StaticVariable("linuxClangCppflags", strings.Join(linuxClangCppflags, " "))

	// Extended cflags
	pctx.StaticVariable("linuxX86Cflags", strings.Join(linuxX86Cflags, " "))
	pctx.StaticVariable("linuxX8664Cflags", strings.Join(linuxX8664Cflags, " "))
	pctx.StaticVariable("linuxX86Ldflags", strings.Join(linuxX86Ldflags, " "))
	pctx.StaticVariable("linuxX8664Ldflags", strings.Join(linuxX8664Ldflags, " "))

	pctx.StaticVariable("linuxX86ClangCflags",
		strings.Join(clangFilterUnknownCflags(linuxX86Cflags), " "))
	pctx.StaticVariable("linuxX8664ClangCflags",
		strings.Join(clangFilterUnknownCflags(linuxX8664Cflags), " "))
	pctx.StaticVariable("linuxX86ClangLdflags", strings.Join(linuxX86ClangLdflags, " "))
	pctx.StaticVariable("linuxX8664ClangLdflags", strings.Join(linuxX8664ClangLdflags, " "))
	pctx.StaticVariable("linuxX86ClangCppflags", strings.Join(linuxX86ClangCppflags, " "))
	pctx.StaticVariable("linuxX8664ClangCppflags", strings.Join(linuxX8664ClangCppflags, " "))
}

type toolchainLinux struct {
	cFlags, ldFlags string
}

type toolchainLinuxX86 struct {
	toolchain32Bit
	toolchainLinux
}

type toolchainLinuxX8664 struct {
	toolchain64Bit
	toolchainLinux
}

func (t *toolchainLinuxX86) Name() string {
	return "x86"
}

func (t *toolchainLinuxX8664) Name() string {
	return "x86_64"
}

func (t *toolchainLinux) GccRoot() string {
	return "${linuxGccRoot}"
}

func (t *toolchainLinux) GccTriple() string {
	return "${linuxGccTriple}"
}

func (t *toolchainLinux) GccVersion() string {
	return "${linuxGccVersion}"
}

func (t *toolchainLinuxX86) Cflags() string {
	return "${linuxCflags} ${linuxX86Cflags}"
}

func (t *toolchainLinuxX8664) Cflags() string {
	return "${linuxCflags} ${linuxX8664Cflags}"
}

func (t *toolchainLinux) Cppflags() string {
	return ""
}

func (t *toolchainLinuxX86) Ldflags() string {
	return "${linuxLdflags} ${linuxX86Ldflags}"
}

func (t *toolchainLinuxX8664) Ldflags() string {
	return "${linuxLdflags} ${linuxX8664Ldflags}"
}

func (t *toolchainLinux) IncludeFlags() string {
	return ""
}

func (t *toolchainLinuxX86) ClangTriple() string {
	return "i686-linux-gnu"
}

func (t *toolchainLinuxX86) ClangCflags() string {
	return "${linuxClangCflags} ${linuxX86ClangCflags}"
}

func (t *toolchainLinuxX86) ClangCppflags() string {
	return "${linuxClangCppflags} ${linuxX86ClangCppflags}"
}

func (t *toolchainLinuxX8664) ClangTriple() string {
	return "x86_64-linux-gnu"
}

func (t *toolchainLinuxX8664) ClangCflags() string {
	return "${linuxClangCflags} ${linuxX8664ClangCflags}"
}

func (t *toolchainLinuxX8664) ClangCppflags() string {
	return "${linuxClangCppflags} ${linuxX8664ClangCppflags}"
}

func (t *toolchainLinuxX86) ClangLdflags() string {
	return "${linuxClangLdflags} ${linuxX86ClangLdflags}"
}

func (t *toolchainLinuxX8664) ClangLdflags() string {
	return "${linuxClangLdflags} ${linuxX8664ClangLdflags}"
}

var toolchainLinuxX86Singleton Toolchain = &toolchainLinuxX86{}
var toolchainLinuxX8664Singleton Toolchain = &toolchainLinuxX8664{}

func linuxX86ToolchainFactory(archVariant string, cpuVariant string) Toolchain {
	return toolchainLinuxX86Singleton
}

func linuxX8664ToolchainFactory(archVariant string, cpuVariant string) Toolchain {
	return toolchainLinuxX8664Singleton
}

func init() {
	if runtime.GOOS == "linux" {
		registerToolchainFactory(common.Host, common.X86, linuxX86ToolchainFactory)
		registerToolchainFactory(common.Host, common.X86_64, linuxX8664ToolchainFactory)
	}
}
