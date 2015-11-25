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
	"strings"

	"android/soong/common"
)

var (
	windowsCflags = []string{
		"-fno-exceptions", // from build/core/combo/select.mk
		"-Wno-multichar",  // from build/core/combo/select.mk

		"-DUSE_MINGW",
		"-DWIN32_LEAN_AND_MEAN",
		"-Wno-unused-parameter",
		"-m32",

		// Workaround differences in inttypes.h between host and target.
		//See bug 12708004.
		"-D__STDC_FORMAT_MACROS",
		"-D__STDC_CONSTANT_MACROS",

		// Use C99-compliant printf functions (%zd).
		"-D__USE_MINGW_ANSI_STDIO=1",
		// Admit to using >= Win2K. Both are needed because of <_mingw.h>.
		"-D_WIN32_WINNT=0x0500",
		"-DWINVER=0x0500",
		// Get 64-bit off_t and related functions.
		"-D_FILE_OFFSET_BITS=64",

		// HOST_RELEASE_CFLAGS
		"-O2", // from build/core/combo/select.mk
		"-g",  // from build/core/combo/select.mk
		"-fno-strict-aliasing", // from build/core/combo/select.mk
	}

	windowsIncludeFlags = []string{
		"-I${windowsGccRoot}/${windowsGccTriple}/include",
		"-I${windowsGccRoot}/lib/gcc/${windowsGccTriple}/4.8.3/include",
	}

	windowsLdflags = []string{
		"-m32",
		"-L${windowsGccRoot}/${windowsGccTriple}",
		"--enable-stdcall-fixup",
	}
)

func init() {
	pctx.StaticVariable("windowsGccVersion", "4.8")

	pctx.StaticVariable("windowsGccRoot",
		"${SrcDir}/prebuilts/gcc/${HostPrebuiltTag}/host/x86_64-w64-mingw32-${windowsGccVersion}")

	pctx.StaticVariable("windowsGccTriple", "x86_64-w64-mingw32")

	pctx.StaticVariable("windowsCflags", strings.Join(windowsCflags, " "))
	pctx.StaticVariable("windowsLdflags", strings.Join(windowsLdflags, " "))
}

type toolchainWindows struct {
	toolchain32Bit

	cFlags, ldFlags string
}

func (t *toolchainWindows) Name() string {
	return "x86"
}

func (t *toolchainWindows) GccRoot() string {
	return "${windowsGccRoot}"
}

func (t *toolchainWindows) GccTriple() string {
	return "${windowsGccTriple}"
}

func (t *toolchainWindows) GccVersion() string {
	return "${windowsGccVersion}"
}

func (t *toolchainWindows) Cflags() string {
	return "${windowsCflags}"
}

func (t *toolchainWindows) Cppflags() string {
	return ""
}

func (t *toolchainWindows) Ldflags() string {
	return "${windowsLdflags}"
}

func (t *toolchainWindows) IncludeFlags() string {
	return ""
}

func (t *toolchainWindows) ClangSupported() bool {
	return false
}

func (t *toolchainWindows) ClangTriple() string {
	panic("Clang is not supported under mingw")
}

func (t *toolchainWindows) ClangCflags() string {
	panic("Clang is not supported under mingw")
}

func (t *toolchainWindows) ClangCppflags() string {
	panic("Clang is not supported under mingw")
}

func (t *toolchainWindows) ClangLdflags() string {
	panic("Clang is not supported under mingw")
}

func (t *toolchainWindows) ShlibSuffix() string {
	return ".dll"
}

func (t *toolchainWindows) ExecutableSuffix() string {
	return ".exe"
}

var toolchainWindowsSingleton Toolchain = &toolchainWindows{}

func windowsToolchainFactory(arch common.Arch) Toolchain {
	return toolchainWindowsSingleton
}

func init() {
	registerHostToolchainFactory(common.Windows, common.X86, windowsToolchainFactory)
}
