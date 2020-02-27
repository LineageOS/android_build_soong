// Copyright 2016 Google Inc. All rights reserved.
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
	linuxCflags = []string{
		"-fdiagnostics-color",

		"-Wa,--noexecstack",

		"-fPIC",

		"-U_FORTIFY_SOURCE",
		"-D_FORTIFY_SOURCE=2",
		"-fstack-protector",

		// Workaround differences in inttypes.h between host and target.
		//See bug 12708004.
		"-D__STDC_FORMAT_MACROS",
		"-D__STDC_CONSTANT_MACROS",
	}

	linuxLdflags = []string{
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
		"-Wl,--no-undefined-version",
	}

	// Extended cflags
	linuxX86Cflags = []string{
		"-msse3",
		"-mfpmath=sse",
		"-m32",
		"-march=prescott",
		"-D_FILE_OFFSET_BITS=64",
		"-D_LARGEFILE_SOURCE=1",
	}

	linuxX8664Cflags = []string{
		"-m64",
	}

	linuxX86Ldflags = []string{
		"-m32",
	}

	linuxX8664Ldflags = []string{
		"-m64",
	}

	linuxClangCflags = append(ClangFilterUnknownCflags(linuxCflags), []string{
		"--gcc-toolchain=${LinuxGccRoot}",
		"--sysroot ${LinuxGccRoot}/sysroot",
		"-fstack-protector-strong",
	}...)

	linuxClangLdflags = append(ClangFilterUnknownCflags(linuxLdflags), []string{
		"--gcc-toolchain=${LinuxGccRoot}",
		"--sysroot ${LinuxGccRoot}/sysroot",
	}...)

	linuxClangLldflags = ClangFilterUnknownLldflags(linuxClangLdflags)

	linuxX86ClangLdflags = append(ClangFilterUnknownCflags(linuxX86Ldflags), []string{
		"-B${LinuxGccRoot}/lib/gcc/${LinuxGccTriple}/${LinuxGccVersion}/32",
		"-L${LinuxGccRoot}/lib/gcc/${LinuxGccTriple}/${LinuxGccVersion}/32",
		"-L${LinuxGccRoot}/${LinuxGccTriple}/lib32",
	}...)

	linuxX86ClangLldflags = ClangFilterUnknownLldflags(linuxX86ClangLdflags)

	linuxX8664ClangLdflags = append(ClangFilterUnknownCflags(linuxX8664Ldflags), []string{
		"-B${LinuxGccRoot}/lib/gcc/${LinuxGccTriple}/${LinuxGccVersion}",
		"-L${LinuxGccRoot}/lib/gcc/${LinuxGccTriple}/${LinuxGccVersion}",
		"-L${LinuxGccRoot}/${LinuxGccTriple}/lib64",
	}...)

	linuxX8664ClangLldflags = ClangFilterUnknownLldflags(linuxX8664ClangLdflags)

	linuxAvailableLibraries = addPrefix([]string{
		"c",
		"dl",
		"gcc",
		"gcc_s",
		"m",
		"ncurses",
		"pthread",
		"resolv",
		"rt",
		"util",
	}, "-l")
)

const (
	linuxGccVersion   = "4.8.3"
	linuxGlibcVersion = "2.17"
)

func init() {
	pctx.StaticVariable("LinuxGccVersion", linuxGccVersion)
	pctx.StaticVariable("LinuxGlibcVersion", linuxGlibcVersion)
	// Most places use the full GCC version. A few only use up to the first two numbers.
	if p := strings.Split(linuxGccVersion, "."); len(p) > 2 {
		pctx.StaticVariable("ShortLinuxGccVersion", strings.Join(p[:2], "."))
	} else {
		pctx.StaticVariable("ShortLinuxGccVersion", linuxGccVersion)
	}

	pctx.SourcePathVariable("LinuxGccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/host/x86_64-linux-glibc${LinuxGlibcVersion}-${ShortLinuxGccVersion}")

	pctx.StaticVariable("LinuxGccTriple", "x86_64-linux")

	pctx.StaticVariable("LinuxClangCflags", strings.Join(linuxClangCflags, " "))
	pctx.StaticVariable("LinuxClangLdflags", strings.Join(linuxClangLdflags, " "))
	pctx.StaticVariable("LinuxClangLldflags", strings.Join(linuxClangLldflags, " "))

	pctx.StaticVariable("LinuxX86ClangCflags",
		strings.Join(ClangFilterUnknownCflags(linuxX86Cflags), " "))
	pctx.StaticVariable("LinuxX8664ClangCflags",
		strings.Join(ClangFilterUnknownCflags(linuxX8664Cflags), " "))
	pctx.StaticVariable("LinuxX86ClangLdflags", strings.Join(linuxX86ClangLdflags, " "))
	pctx.StaticVariable("LinuxX86ClangLldflags", strings.Join(linuxX86ClangLldflags, " "))
	pctx.StaticVariable("LinuxX8664ClangLdflags", strings.Join(linuxX8664ClangLdflags, " "))
	pctx.StaticVariable("LinuxX8664ClangLldflags", strings.Join(linuxX8664ClangLldflags, " "))
	// Yasm flags
	pctx.StaticVariable("LinuxX86YasmFlags", "-f elf32 -m x86")
	pctx.StaticVariable("LinuxX8664YasmFlags", "-f elf64 -m amd64")
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
	return "${config.LinuxGccRoot}"
}

func (t *toolchainLinux) GccTriple() string {
	return "${config.LinuxGccTriple}"
}

func (t *toolchainLinux) GccVersion() string {
	return linuxGccVersion
}

func (t *toolchainLinux) IncludeFlags() string {
	return ""
}

func (t *toolchainLinuxX86) ClangTriple() string {
	return "i686-linux-gnu"
}

func (t *toolchainLinuxX86) ClangCflags() string {
	return "${config.LinuxClangCflags} ${config.LinuxX86ClangCflags}"
}

func (t *toolchainLinuxX86) ClangCppflags() string {
	return ""
}

func (t *toolchainLinuxX8664) ClangTriple() string {
	return "x86_64-linux-gnu"
}

func (t *toolchainLinuxX8664) ClangCflags() string {
	return "${config.LinuxClangCflags} ${config.LinuxX8664ClangCflags}"
}

func (t *toolchainLinuxX8664) ClangCppflags() string {
	return ""
}

func (t *toolchainLinuxX86) ClangLdflags() string {
	return "${config.LinuxClangLdflags} ${config.LinuxX86ClangLdflags}"
}

func (t *toolchainLinuxX86) ClangLldflags() string {
	return "${config.LinuxClangLldflags} ${config.LinuxX86ClangLldflags}"
}

func (t *toolchainLinuxX8664) ClangLdflags() string {
	return "${config.LinuxClangLdflags} ${config.LinuxX8664ClangLdflags}"
}

func (t *toolchainLinuxX8664) ClangLldflags() string {
	return "${config.LinuxClangLldflags} ${config.LinuxX8664ClangLldflags}"
}

func (t *toolchainLinuxX86) YasmFlags() string {
	return "${config.LinuxX86YasmFlags}"
}

func (t *toolchainLinuxX8664) YasmFlags() string {
	return "${config.LinuxX8664YasmFlags}"
}

func (toolchainLinuxX86) LibclangRuntimeLibraryArch() string {
	return "i386"
}

func (toolchainLinuxX8664) LibclangRuntimeLibraryArch() string {
	return "x86_64"
}

func (t *toolchainLinux) AvailableLibraries() []string {
	return linuxAvailableLibraries
}

func (t *toolchainLinux) Bionic() bool {
	return false
}

var toolchainLinuxX86Singleton Toolchain = &toolchainLinuxX86{}
var toolchainLinuxX8664Singleton Toolchain = &toolchainLinuxX8664{}

func linuxX86ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxX86Singleton
}

func linuxX8664ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxX8664Singleton
}

func init() {
	registerToolchainFactory(android.Linux, android.X86, linuxX86ToolchainFactory)
	registerToolchainFactory(android.Linux, android.X86_64, linuxX8664ToolchainFactory)
}
