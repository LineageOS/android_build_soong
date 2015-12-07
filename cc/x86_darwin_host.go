package cc

import (
	"strings"

	"android/soong/common"
)

var (
	darwinCflags = []string{
		"-fno-exceptions", // from build/core/combo/select.mk
		"-Wno-multichar",  // from build/core/combo/select.mk

		"-fdiagnostics-color",

		"-fPIC",
		"-funwind-tables",

		// Workaround differences in inttypes.h between host and target.
		//See bug 12708004.
		"-D__STDC_FORMAT_MACROS",
		"-D__STDC_CONSTANT_MACROS",

		// HOST_RELEASE_CFLAGS
		"-O2", // from build/core/combo/select.mk
		"-g",  // from build/core/combo/select.mk
		"-fno-strict-aliasing", // from build/core/combo/select.mk
		"-isysroot ${macSdkRoot}",
		"-mmacosx-version-min=10.9",
		"-DMACOSX_DEPLOYMENT_TARGET=10.9",
	}

	darwinCppflags = []string{
		"-isystem ${macSdkPath}/Toolchains/XcodeDefault.xctoolchain/usr/include/c++/v1",
	}

	darwinLdflags = []string{
		"-isysroot ${macSdkRoot}",
		"-Wl,-syslibroot,${macSdkRoot}",
		"-mmacosx-version-min=10.9",
	}

	// Extended cflags
	darwinX86Cflags = []string{
		"-m32",
	}

	darwinX8664Cflags = []string{
		"-m64",
	}

	darwinX86Ldflags = []string{
		"-m32",
		"-Wl,-rpath,@loader_path/../lib",
		"-Wl,-rpath,@loader_path/lib",
	}

	darwinX8664Ldflags = []string{
		"-m64",
		"-Wl,-rpath,@loader_path/../lib64",
		"-Wl,-rpath,@loader_path/lib64",
	}

	darwinClangCflags = append([]string{
		"-integrated-as",
	}, clangFilterUnknownCflags(darwinCflags)...)

	darwinClangLdflags = clangFilterUnknownCflags(darwinLdflags)

	darwinX86ClangLdflags = clangFilterUnknownCflags(darwinX86Ldflags)

	darwinX8664ClangLdflags = clangFilterUnknownCflags(darwinX8664Ldflags)

	darwinClangCppflags = clangFilterUnknownCflags(darwinCppflags)
)

const (
	darwinGccVersion = "4.2.1"
)

func init() {
	pctx.StaticVariable("macSdkPath", "/Applications/Xcode.app/Contents/Developer")
	pctx.StaticVariable("macSdkRoot", "${macSdkPath}/Platforms/MacOSX.platform/Developer/SDKs/MacOSX10.9.sdk")

	pctx.StaticVariable("darwinGccVersion", darwinGccVersion)
	pctx.StaticVariable("darwinGccRoot",
		"${SrcDir}/prebuilts/gcc/${HostPrebuiltTag}/host/i686-apple-darwin-${darwinGccVersion}")

	pctx.StaticVariable("darwinGccTriple", "i686-apple-darwin11")

	pctx.StaticVariable("darwinCflags", strings.Join(darwinCflags, " "))
	pctx.StaticVariable("darwinLdflags", strings.Join(darwinLdflags, " "))
	pctx.StaticVariable("darwinCppflags", strings.Join(darwinCppflags, " "))

	pctx.StaticVariable("darwinClangCflags", strings.Join(darwinClangCflags, " "))
	pctx.StaticVariable("darwinClangLdflags", strings.Join(darwinClangLdflags, " "))
	pctx.StaticVariable("darwinClangCppflags", strings.Join(darwinClangCppflags, " "))

	// Extended cflags
	pctx.StaticVariable("darwinX86Cflags", strings.Join(darwinX86Cflags, " "))
	pctx.StaticVariable("darwinX8664Cflags", strings.Join(darwinX8664Cflags, " "))
	pctx.StaticVariable("darwinX86Ldflags", strings.Join(darwinX86Ldflags, " "))
	pctx.StaticVariable("darwinX8664Ldflags", strings.Join(darwinX8664Ldflags, " "))

	pctx.StaticVariable("darwinX86ClangCflags",
		strings.Join(clangFilterUnknownCflags(darwinX86Cflags), " "))
	pctx.StaticVariable("darwinX8664ClangCflags",
		strings.Join(clangFilterUnknownCflags(darwinX8664Cflags), " "))
	pctx.StaticVariable("darwinX86ClangLdflags", strings.Join(darwinX86ClangLdflags, " "))
	pctx.StaticVariable("darwinX8664ClangLdflags", strings.Join(darwinX8664ClangLdflags, " "))
}

type toolchainDarwin struct {
	cFlags, ldFlags string
}

type toolchainDarwinX86 struct {
	toolchain32Bit
	toolchainDarwin
}

type toolchainDarwinX8664 struct {
	toolchain64Bit
	toolchainDarwin
}

func (t *toolchainDarwinX86) Name() string {
	return "x86"
}

func (t *toolchainDarwinX8664) Name() string {
	return "x86_64"
}

func (t *toolchainDarwin) GccRoot() string {
	return "${darwinGccRoot}"
}

func (t *toolchainDarwin) GccTriple() string {
	return "${darwinGccTriple}"
}

func (t *toolchainDarwin) GccVersion() string {
	return darwinGccVersion
}

func (t *toolchainDarwin) Cflags() string {
	return "${darwinCflags} ${darwinX86Cflags}"
}

func (t *toolchainDarwinX8664) Cflags() string {
	return "${darwinCflags} ${darwinX8664Cflags}"
}

func (t *toolchainDarwin) Cppflags() string {
	return "${darwinCppflags}"
}

func (t *toolchainDarwinX86) Ldflags() string {
	return "${darwinLdflags} ${darwinX86Ldflags}"
}

func (t *toolchainDarwinX8664) Ldflags() string {
	return "${darwinLdflags} ${darwinX8664Ldflags}"
}

func (t *toolchainDarwin) IncludeFlags() string {
	return ""
}

func (t *toolchainDarwinX86) ClangTriple() string {
	return "i686-darwin-gnu"
}

func (t *toolchainDarwinX86) ClangCflags() string {
	return "${darwinClangCflags} ${darwinX86ClangCflags}"
}

func (t *toolchainDarwinX86) ClangCppflags() string {
	return "${darwinClangCppflags}"
}

func (t *toolchainDarwinX8664) ClangTriple() string {
	return "x86_64-darwin-gnu"
}

func (t *toolchainDarwinX8664) ClangCflags() string {
	return "${darwinClangCflags} ${darwinX8664ClangCflags}"
}

func (t *toolchainDarwinX8664) ClangCppflags() string {
	return "${darwinClangCppflags}"
}

func (t *toolchainDarwinX86) ClangLdflags() string {
	return "${darwinClangLdflags} ${darwinX86ClangLdflags}"
}

func (t *toolchainDarwinX8664) ClangLdflags() string {
	return "${darwinClangLdflags} ${darwinX8664ClangLdflags}"
}

func (t *toolchainDarwin) ShlibSuffix() string {
	return ".dylib"
}

var toolchainDarwinX86Singleton Toolchain = &toolchainDarwinX86{}
var toolchainDarwinX8664Singleton Toolchain = &toolchainDarwinX8664{}

func darwinX86ToolchainFactory(arch common.Arch) Toolchain {
	return toolchainDarwinX86Singleton
}

func darwinX8664ToolchainFactory(arch common.Arch) Toolchain {
	return toolchainDarwinX8664Singleton
}

func init() {
	registerHostToolchainFactory(common.Darwin, common.X86, darwinX86ToolchainFactory)
	registerHostToolchainFactory(common.Darwin, common.X86_64, darwinX8664ToolchainFactory)
}
