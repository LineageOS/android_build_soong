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
	"strings"

	"android/soong/android"
)

var (
	windowsCflags = []string{
		"-DUSE_MINGW",
		"-DWIN32_LEAN_AND_MEAN",
		"-Wno-unused-parameter",

		// Workaround differences in inttypes.h between host and target.
		//See bug 12708004.
		"-D__STDC_FORMAT_MACROS",
		"-D__STDC_CONSTANT_MACROS",

		// Use C99-compliant printf functions (%zd).
		"-D__USE_MINGW_ANSI_STDIO=1",
		// Admit to using >= Windows 7. Both are needed because of <_mingw.h>.
		"-D_WIN32_WINNT=0x0601",
		"-DWINVER=0x0601",
		// Get 64-bit off_t and related functions.
		"-D_FILE_OFFSET_BITS=64",

		"--sysroot ${WindowsGccRoot}/${WindowsGccTriple}",
	}
	windowsClangCflags = append(ClangFilterUnknownCflags(windowsCflags), []string{}...)

	windowsIncludeFlags = []string{
		"-isystem ${WindowsGccRoot}/${WindowsGccTriple}/include",
	}

	windowsClangCppflags = []string{}

	windowsX86ClangCppflags = []string{}

	windowsX8664ClangCppflags = []string{}

	windowsLdflags = []string{
		"--enable-stdcall-fixup",
		"-Wl,--dynamicbase",
		"-Wl,--nxcompat",
	}
	windowsLldflags = []string{
		"-Wl,--Xlink=-Brepro", // Enable deterministic build
	}
	windowsClangLdflags  = append(ClangFilterUnknownCflags(windowsLdflags), []string{}...)
	windowsClangLldflags = append(ClangFilterUnknownLldflags(windowsClangLdflags), windowsLldflags...)

	windowsX86Cflags = []string{
		"-m32",
	}

	windowsX8664Cflags = []string{
		"-m64",
	}

	windowsX86Ldflags = []string{
		"-m32",
		"-Wl,--large-address-aware",
		"-L${WindowsGccRoot}/${WindowsGccTriple}/lib32",
		"-static-libgcc",
	}
	windowsX86ClangLdflags = append(ClangFilterUnknownCflags(windowsX86Ldflags), []string{
		"-B${WindowsGccRoot}/${WindowsGccTriple}/bin",
		"-B${WindowsGccRoot}/lib/gcc/${WindowsGccTriple}/4.8.3/32",
		"-L${WindowsGccRoot}/lib/gcc/${WindowsGccTriple}/4.8.3/32",
		"-B${WindowsGccRoot}/${WindowsGccTriple}/lib32",
	}...)
	windowsX86ClangLldflags = ClangFilterUnknownLldflags(windowsX86ClangLdflags)

	windowsX8664Ldflags = []string{
		"-m64",
		"-L${WindowsGccRoot}/${WindowsGccTriple}/lib64",
		"-Wl,--high-entropy-va",
		"-static-libgcc",
	}
	windowsX8664ClangLdflags = append(ClangFilterUnknownCflags(windowsX8664Ldflags), []string{
		"-B${WindowsGccRoot}/${WindowsGccTriple}/bin",
		"-B${WindowsGccRoot}/lib/gcc/${WindowsGccTriple}/4.8.3",
		"-L${WindowsGccRoot}/lib/gcc/${WindowsGccTriple}/4.8.3",
		"-B${WindowsGccRoot}/${WindowsGccTriple}/lib64",
	}...)
	windowsX8664ClangLldflags = ClangFilterUnknownLldflags(windowsX8664ClangLdflags)

	windowsAvailableLibraries = addPrefix([]string{
		"gdi32",
		"imagehlp",
		"iphlpapi",
		"netapi32",
		"oleaut32",
		"ole32",
		"opengl32",
		"powrprof",
		"psapi",
		"pthread",
		"userenv",
		"uuid",
		"version",
		"ws2_32",
		"windowscodecs",
	}, "-l")
)

const (
	windowsGccVersion = "4.8"
)

func init() {
	pctx.StaticVariable("WindowsGccVersion", windowsGccVersion)

	pctx.SourcePathVariable("WindowsGccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/host/x86_64-w64-mingw32-${WindowsGccVersion}")

	pctx.StaticVariable("WindowsGccTriple", "x86_64-w64-mingw32")

	pctx.StaticVariable("WindowsClangCflags", strings.Join(windowsClangCflags, " "))
	pctx.StaticVariable("WindowsClangLdflags", strings.Join(windowsClangLdflags, " "))
	pctx.StaticVariable("WindowsClangLldflags", strings.Join(windowsClangLldflags, " "))
	pctx.StaticVariable("WindowsClangCppflags", strings.Join(windowsClangCppflags, " "))

	pctx.StaticVariable("WindowsX86ClangCflags",
		strings.Join(ClangFilterUnknownCflags(windowsX86Cflags), " "))
	pctx.StaticVariable("WindowsX8664ClangCflags",
		strings.Join(ClangFilterUnknownCflags(windowsX8664Cflags), " "))
	pctx.StaticVariable("WindowsX86ClangLdflags", strings.Join(windowsX86ClangLdflags, " "))
	pctx.StaticVariable("WindowsX86ClangLldflags", strings.Join(windowsX86ClangLldflags, " "))
	pctx.StaticVariable("WindowsX8664ClangLdflags", strings.Join(windowsX8664ClangLdflags, " "))
	pctx.StaticVariable("WindowsX8664ClangLldflags", strings.Join(windowsX8664ClangLldflags, " "))
	pctx.StaticVariable("WindowsX86ClangCppflags", strings.Join(windowsX86ClangCppflags, " "))
	pctx.StaticVariable("WindowsX8664ClangCppflags", strings.Join(windowsX8664ClangCppflags, " "))

	pctx.StaticVariable("WindowsIncludeFlags", strings.Join(windowsIncludeFlags, " "))
	// Yasm flags
	pctx.StaticVariable("WindowsX86YasmFlags", "-f win32 -m x86")
	pctx.StaticVariable("WindowsX8664YasmFlags", "-f win64 -m amd64")
}

type toolchainWindows struct {
	cFlags, ldFlags string
}

type toolchainWindowsX86 struct {
	toolchain32Bit
	toolchainWindows
}

type toolchainWindowsX8664 struct {
	toolchain64Bit
	toolchainWindows
}

func (t *toolchainWindowsX86) Name() string {
	return "x86"
}

func (t *toolchainWindowsX8664) Name() string {
	return "x86_64"
}

func (t *toolchainWindows) GccRoot() string {
	return "${config.WindowsGccRoot}"
}

func (t *toolchainWindows) GccTriple() string {
	return "${config.WindowsGccTriple}"
}

func (t *toolchainWindows) GccVersion() string {
	return windowsGccVersion
}

func (t *toolchainWindows) IncludeFlags() string {
	return "${config.WindowsIncludeFlags}"
}

func (t *toolchainWindowsX86) WindresFlags() string {
	return "-F pe-i386"
}

func (t *toolchainWindowsX8664) WindresFlags() string {
	return "-F pe-x86-64"
}

func (t *toolchainWindowsX86) ClangTriple() string {
	return "i686-windows-gnu"
}

func (t *toolchainWindowsX8664) ClangTriple() string {
	return "x86_64-pc-windows-gnu"
}

func (t *toolchainWindowsX86) ClangCflags() string {
	return "${config.WindowsClangCflags} ${config.WindowsX86ClangCflags}"
}

func (t *toolchainWindowsX8664) ClangCflags() string {
	return "${config.WindowsClangCflags} ${config.WindowsX8664ClangCflags}"
}

func (t *toolchainWindowsX86) ClangCppflags() string {
	return "${config.WindowsClangCppflags} ${config.WindowsX86ClangCppflags}"
}

func (t *toolchainWindowsX8664) ClangCppflags() string {
	return "${config.WindowsClangCppflags} ${config.WindowsX8664ClangCppflags}"
}

func (t *toolchainWindowsX86) ClangLdflags() string {
	return "${config.WindowsClangLdflags} ${config.WindowsX86ClangLdflags}"
}

func (t *toolchainWindowsX86) ClangLldflags() string {
	return "${config.WindowsClangLldflags} ${config.WindowsX86ClangLldflags}"
}

func (t *toolchainWindowsX8664) ClangLdflags() string {
	return "${config.WindowsClangLdflags} ${config.WindowsX8664ClangLdflags}"
}

func (t *toolchainWindowsX8664) ClangLldflags() string {
	return "${config.WindowsClangLldflags} ${config.WindowsX8664ClangLldflags}"
}

func (t *toolchainWindowsX86) YasmFlags() string {
	return "${config.WindowsX86YasmFlags}"
}

func (t *toolchainWindowsX8664) YasmFlags() string {
	return "${config.WindowsX8664YasmFlags}"
}

func (t *toolchainWindows) ShlibSuffix() string {
	return ".dll"
}

func (t *toolchainWindows) ExecutableSuffix() string {
	return ".exe"
}

func (t *toolchainWindows) AvailableLibraries() []string {
	return windowsAvailableLibraries
}

func (t *toolchainWindows) Bionic() bool {
	return false
}

var toolchainWindowsX86Singleton Toolchain = &toolchainWindowsX86{}
var toolchainWindowsX8664Singleton Toolchain = &toolchainWindowsX8664{}

func windowsX86ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainWindowsX86Singleton
}

func windowsX8664ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainWindowsX8664Singleton
}

func init() {
	registerToolchainFactory(android.Windows, android.X86, windowsX86ToolchainFactory)
	registerToolchainFactory(android.Windows, android.X86_64, windowsX8664ToolchainFactory)
}
