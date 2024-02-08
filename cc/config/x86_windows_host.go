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
	"path/filepath"
	"strings"

	"android/soong/android"
)

var (
	windowsCflags = []string{
		"-DUSE_MINGW",
		"-DWIN32_LEAN_AND_MEAN",
		"-Wno-unused-parameter",

		// Workaround differences in <stdint.h> between host and target.
		// Context: http://b/12708004
		"-D__STDC_CONSTANT_MACROS",

		// Use C99-compliant printf functions (%zd).
		"-D__USE_MINGW_ANSI_STDIO=1",
		// Admit to using >= Windows 7. Both are needed because of <_mingw.h>.
		"-D_WIN32_WINNT=0x0601",
		"-DWINVER=0x0601",
		// Get 64-bit off_t and related functions.
		"-D_FILE_OFFSET_BITS=64",

		// Don't adjust the layout of bitfields like msvc does.
		"-mno-ms-bitfields",

		"--sysroot ${WindowsGccRoot}/${WindowsGccTriple}",
	}

	windowsIncludeFlags = []string{
		"-isystem ${WindowsGccRoot}/${WindowsGccTriple}/include",
	}

	windowsCppflags = []string{}

	windowsX86Cppflags = []string{
		// Use SjLj exceptions for 32-bit.  libgcc_eh implements SjLj
		// exception model for 32-bit.
		"-fsjlj-exceptions",
	}

	windowsX8664Cppflags = []string{}

	windowsLdflags = []string{
		"-Wl,--dynamicbase",
		"-Wl,--nxcompat",
	}
	windowsLldflags = append(windowsLdflags, []string{
		"-Wl,--Xlink=-Brepro", // Enable deterministic build
	}...)

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

		"-B${WindowsGccRoot}/${WindowsGccTriple}/bin",
		"-B${WindowsGccRoot}/lib/gcc/${WindowsGccTriple}/4.8.3/32",
		"-L${WindowsGccRoot}/lib/gcc/${WindowsGccTriple}/4.8.3/32",
		"-B${WindowsGccRoot}/${WindowsGccTriple}/lib32",
	}

	windowsX8664Ldflags = []string{
		"-m64",
		"-L${WindowsGccRoot}/${WindowsGccTriple}/lib64",
		"-Wl,--high-entropy-va",
		"-static-libgcc",

		"-B${WindowsGccRoot}/${WindowsGccTriple}/bin",
		"-B${WindowsGccRoot}/lib/gcc/${WindowsGccTriple}/4.8.3",
		"-L${WindowsGccRoot}/lib/gcc/${WindowsGccTriple}/4.8.3",
		"-B${WindowsGccRoot}/${WindowsGccTriple}/lib64",
	}

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

	pctx.StaticVariable("WindowsCflags", strings.Join(windowsCflags, " "))
	pctx.StaticVariable("WindowsLdflags", strings.Join(windowsLdflags, " "))
	pctx.StaticVariable("WindowsLldflags", strings.Join(windowsLldflags, " "))
	pctx.StaticVariable("WindowsCppflags", strings.Join(windowsCppflags, " "))

	pctx.StaticVariable("WindowsX86Cflags", strings.Join(windowsX86Cflags, " "))
	pctx.StaticVariable("WindowsX8664Cflags", strings.Join(windowsX8664Cflags, " "))
	pctx.StaticVariable("WindowsX86Ldflags", strings.Join(windowsX86Ldflags, " "))
	pctx.StaticVariable("WindowsX86Lldflags", strings.Join(windowsX86Ldflags, " "))
	pctx.StaticVariable("WindowsX8664Ldflags", strings.Join(windowsX8664Ldflags, " "))
	pctx.StaticVariable("WindowsX8664Lldflags", strings.Join(windowsX8664Ldflags, " "))
	pctx.StaticVariable("WindowsX86Cppflags", strings.Join(windowsX86Cppflags, " "))
	pctx.StaticVariable("WindowsX8664Cppflags", strings.Join(windowsX8664Cppflags, " "))

	pctx.StaticVariable("WindowsIncludeFlags", strings.Join(windowsIncludeFlags, " "))
	// Yasm flags
	pctx.StaticVariable("WindowsX86YasmFlags", "-f win32 -m x86")
	pctx.StaticVariable("WindowsX8664YasmFlags", "-f win64 -m amd64")
}

type toolchainWindows struct {
	cFlags, ldFlags string
	toolchainBase
	toolchainNoCrt
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

func (t *toolchainWindows) ToolchainCflags() string {
	return "-B" + filepath.Join("${config.WindowsGccRoot}", "${config.WindowsGccTriple}", "bin")
}

func (t *toolchainWindows) ToolchainLdflags() string {
	return "-B" + filepath.Join("${config.WindowsGccRoot}", "${config.WindowsGccTriple}", "bin")
}

func (t *toolchainWindows) IncludeFlags() string {
	return "${config.WindowsIncludeFlags}"
}

func (t *toolchainWindowsX86) ClangTriple() string {
	return "i686-windows-gnu"
}

func (t *toolchainWindowsX8664) ClangTriple() string {
	return "x86_64-pc-windows-gnu"
}

func (t *toolchainWindowsX86) Cflags() string {
	return "${config.WindowsCflags} ${config.WindowsX86Cflags}"
}

func (t *toolchainWindowsX8664) Cflags() string {
	return "${config.WindowsCflags} ${config.WindowsX8664Cflags}"
}

func (t *toolchainWindowsX86) Cppflags() string {
	return "${config.WindowsCppflags} ${config.WindowsX86Cppflags}"
}

func (t *toolchainWindowsX8664) Cppflags() string {
	return "${config.WindowsCppflags} ${config.WindowsX8664Cppflags}"
}

func (t *toolchainWindowsX86) Ldflags() string {
	return "${config.WindowsLdflags} ${config.WindowsX86Ldflags}"
}

func (t *toolchainWindowsX86) Lldflags() string {
	return "${config.WindowsLldflags} ${config.WindowsX86Lldflags}"
}

func (t *toolchainWindowsX8664) Ldflags() string {
	return "${config.WindowsLdflags} ${config.WindowsX8664Ldflags}"
}

func (t *toolchainWindowsX8664) Lldflags() string {
	return "${config.WindowsLldflags} ${config.WindowsX8664Lldflags}"
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
