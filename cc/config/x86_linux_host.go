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
		"-Wa,--noexecstack",

		"-fPIC",

		"-U_FORTIFY_SOURCE",
		"-D_FORTIFY_SOURCE=2",
		"-fstack-protector",

		"--gcc-toolchain=${LinuxGccRoot}",
		"-fstack-protector-strong",
	}

	linuxGlibcCflags = []string{
		"--sysroot ${LinuxGccRoot}/sysroot",
	}

	linuxMuslCflags = []string{
		"-D_LIBCPP_HAS_MUSL_LIBC",
		"-DANDROID_HOST_MUSL",
		"-nostdlibinc",
		"--sysroot /dev/null",
	}

	linuxLdflags = []string{
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
		"-Wl,--no-undefined-version",

		"--gcc-toolchain=${LinuxGccRoot}",
	}

	linuxLldflags = append(linuxLdflags,
		"-Wl,--compress-debug-sections=zstd",
	)

	linuxGlibcLdflags = []string{
		"--sysroot ${LinuxGccRoot}/sysroot",
	}

	linuxMuslLdflags = []string{
		"-nostdlib",
		"--sysroot /dev/null",
	}

	// Extended cflags
	linuxX86Cflags = []string{
		"-msse3",
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
		"-B${LinuxGccRoot}/lib/gcc/${LinuxGccTriple}/${LinuxGccVersion}/32",
		"-L${LinuxGccRoot}/lib/gcc/${LinuxGccTriple}/${LinuxGccVersion}/32",
		"-L${LinuxGccRoot}/${LinuxGccTriple}/lib32",
	}

	linuxX8664Ldflags = []string{
		"-m64",
		"-B${LinuxGccRoot}/lib/gcc/${LinuxGccTriple}/${LinuxGccVersion}",
		"-L${LinuxGccRoot}/lib/gcc/${LinuxGccTriple}/${LinuxGccVersion}",
		"-L${LinuxGccRoot}/${LinuxGccTriple}/lib64",
	}

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

	muslCrtBeginStaticBinary, muslCrtEndStaticBinary   = []string{"libc_musl_crtbegin_static"}, []string{"libc_musl_crtend"}
	muslCrtBeginSharedBinary, muslCrtEndSharedBinary   = []string{"libc_musl_crtbegin_dynamic"}, []string{"libc_musl_crtend"}
	muslCrtBeginSharedLibrary, muslCrtEndSharedLibrary = []string{"libc_musl_crtbegin_so"}, []string{"libc_musl_crtend_so"}

	MuslDefaultSharedLibraries = []string{"libc_musl"}
)

const (
	linuxGccVersion   = "4.8.3"
	linuxGlibcVersion = "2.17"
)

func init() {
	exportedVars.ExportStringStaticVariable("LinuxGccVersion", linuxGccVersion)
	exportedVars.ExportStringStaticVariable("LinuxGlibcVersion", linuxGlibcVersion)

	// Most places use the full GCC version. A few only use up to the first two numbers.
	if p := strings.Split(linuxGccVersion, "."); len(p) > 2 {
		exportedVars.ExportStringStaticVariable("ShortLinuxGccVersion", strings.Join(p[:2], "."))
	} else {
		exportedVars.ExportStringStaticVariable("ShortLinuxGccVersion", linuxGccVersion)
	}

	exportedVars.ExportSourcePathVariable("LinuxGccRoot",
		"prebuilts/gcc/linux-x86/host/x86_64-linux-glibc${LinuxGlibcVersion}-${ShortLinuxGccVersion}")

	exportedVars.ExportStringListStaticVariable("LinuxGccTriple", []string{"x86_64-linux"})

	exportedVars.ExportStringListStaticVariable("LinuxCflags", linuxCflags)
	exportedVars.ExportStringListStaticVariable("LinuxLdflags", linuxLdflags)
	exportedVars.ExportStringListStaticVariable("LinuxLldflags", linuxLldflags)
	exportedVars.ExportStringListStaticVariable("LinuxGlibcCflags", linuxGlibcCflags)
	exportedVars.ExportStringListStaticVariable("LinuxGlibcLdflags", linuxGlibcLdflags)
	exportedVars.ExportStringListStaticVariable("LinuxGlibcLldflags", linuxGlibcLdflags)
	exportedVars.ExportStringListStaticVariable("LinuxMuslCflags", linuxMuslCflags)
	exportedVars.ExportStringListStaticVariable("LinuxMuslLdflags", linuxMuslLdflags)
	exportedVars.ExportStringListStaticVariable("LinuxMuslLldflags", linuxMuslLdflags)

	exportedVars.ExportStringListStaticVariable("LinuxX86Cflags", linuxX86Cflags)
	exportedVars.ExportStringListStaticVariable("LinuxX8664Cflags", linuxX8664Cflags)
	exportedVars.ExportStringListStaticVariable("LinuxX86Ldflags", linuxX86Ldflags)
	exportedVars.ExportStringListStaticVariable("LinuxX86Lldflags", linuxX86Ldflags)
	exportedVars.ExportStringListStaticVariable("LinuxX8664Ldflags", linuxX8664Ldflags)
	exportedVars.ExportStringListStaticVariable("LinuxX8664Lldflags", linuxX8664Ldflags)
	// Yasm flags
	exportedVars.ExportStringListStaticVariable("LinuxX86YasmFlags", []string{"-f elf32 -m x86"})
	exportedVars.ExportStringListStaticVariable("LinuxX8664YasmFlags", []string{"-f elf64 -m amd64"})
}

type toolchainLinux struct {
	toolchainBase
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

func (t *toolchainLinux) IncludeFlags() string {
	return ""
}

func (t *toolchainLinuxX86) Cflags() string {
	return "${config.LinuxCflags} ${config.LinuxX86Cflags}"
}

func (t *toolchainLinuxX86) Cppflags() string {
	return ""
}

func (t *toolchainLinuxX8664) Cflags() string {
	return "${config.LinuxCflags} ${config.LinuxX8664Cflags}"
}

func (t *toolchainLinuxX8664) Cppflags() string {
	return ""
}

func (t *toolchainLinuxX86) Ldflags() string {
	return "${config.LinuxLdflags} ${config.LinuxX86Ldflags}"
}

func (t *toolchainLinuxX86) Lldflags() string {
	return "${config.LinuxLldflags} ${config.LinuxX86Lldflags}"
}

func (t *toolchainLinuxX8664) Ldflags() string {
	return "${config.LinuxLdflags} ${config.LinuxX8664Ldflags}"
}

func (t *toolchainLinuxX8664) Lldflags() string {
	return "${config.LinuxLldflags} ${config.LinuxX8664Lldflags}"
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

func (toolchainLinux) ShlibSuffix() string {
	return ".so"
}

func (toolchainLinux) ExecutableSuffix() string {
	return ""
}

// glibc specialization of the linux toolchain

type toolchainGlibc struct {
	toolchainNoCrt
}

func (toolchainGlibc) Glibc() bool { return true }

func (toolchainGlibc) Cflags() string {
	return "${config.LinuxGlibcCflags}"
}

func (toolchainGlibc) Ldflags() string {
	return "${config.LinuxGlibcLdflags}"
}

func (toolchainGlibc) Lldflags() string {
	return "${config.LinuxGlibcLldflags}"
}

type toolchainLinuxGlibcX86 struct {
	toolchainLinuxX86
	toolchainGlibc
}

type toolchainLinuxGlibcX8664 struct {
	toolchainLinuxX8664
	toolchainGlibc
}

func (t *toolchainLinuxGlibcX86) ClangTriple() string {
	return "i686-linux-gnu"
}

func (t *toolchainLinuxGlibcX86) Cflags() string {
	return t.toolchainLinuxX86.Cflags() + " " + t.toolchainGlibc.Cflags()
}

func (t *toolchainLinuxGlibcX86) Ldflags() string {
	return t.toolchainLinuxX86.Ldflags() + " " + t.toolchainGlibc.Ldflags()
}

func (t *toolchainLinuxGlibcX86) Lldflags() string {
	return t.toolchainLinuxX86.Lldflags() + " " + t.toolchainGlibc.Lldflags()
}

func (t *toolchainLinuxGlibcX8664) ClangTriple() string {
	return "x86_64-linux-gnu"
}

func (t *toolchainLinuxGlibcX8664) Cflags() string {
	return t.toolchainLinuxX8664.Cflags() + " " + t.toolchainGlibc.Cflags()
}

func (t *toolchainLinuxGlibcX8664) Ldflags() string {
	return t.toolchainLinuxX8664.Ldflags() + " " + t.toolchainGlibc.Ldflags()
}

func (t *toolchainLinuxGlibcX8664) Lldflags() string {
	return t.toolchainLinuxX8664.Lldflags() + " " + t.toolchainGlibc.Lldflags()
}

var toolchainLinuxGlibcX86Singleton Toolchain = &toolchainLinuxGlibcX86{}
var toolchainLinuxGlibcX8664Singleton Toolchain = &toolchainLinuxGlibcX8664{}

func linuxGlibcX86ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxGlibcX86Singleton
}

func linuxGlibcX8664ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxGlibcX8664Singleton
}

// musl specialization of the linux toolchain

type toolchainMusl struct {
}

func (toolchainMusl) Musl() bool { return true }

func (toolchainMusl) CrtBeginStaticBinary() []string       { return muslCrtBeginStaticBinary }
func (toolchainMusl) CrtBeginSharedBinary() []string       { return muslCrtBeginSharedBinary }
func (toolchainMusl) CrtBeginSharedLibrary() []string      { return muslCrtBeginSharedLibrary }
func (toolchainMusl) CrtEndStaticBinary() []string         { return muslCrtEndStaticBinary }
func (toolchainMusl) CrtEndSharedBinary() []string         { return muslCrtEndSharedBinary }
func (toolchainMusl) CrtEndSharedLibrary() []string        { return muslCrtEndSharedLibrary }
func (toolchainMusl) CrtPadSegmentSharedLibrary() []string { return nil }

func (toolchainMusl) DefaultSharedLibraries() []string { return MuslDefaultSharedLibraries }

func (toolchainMusl) Cflags() string {
	return "${config.LinuxMuslCflags}"
}

func (toolchainMusl) Ldflags() string {
	return "${config.LinuxMuslLdflags}"
}

func (toolchainMusl) Lldflags() string {
	return "${config.LinuxMuslLldflags}"
}

type toolchainLinuxMuslX86 struct {
	toolchainLinuxX86
	toolchainMusl
}

type toolchainLinuxMuslX8664 struct {
	toolchainLinuxX8664
	toolchainMusl
}

func (t *toolchainLinuxMuslX86) ClangTriple() string {
	return "i686-linux-musl"
}

func (t *toolchainLinuxMuslX86) Cflags() string {
	return t.toolchainLinuxX86.Cflags() + " " + t.toolchainMusl.Cflags()
}

func (t *toolchainLinuxMuslX86) Ldflags() string {
	return t.toolchainLinuxX86.Ldflags() + " " + t.toolchainMusl.Ldflags()
}

func (t *toolchainLinuxMuslX86) Lldflags() string {
	return t.toolchainLinuxX86.Lldflags() + " " + t.toolchainMusl.Lldflags()
}

func (t *toolchainLinuxMuslX8664) ClangTriple() string {
	return "x86_64-linux-musl"
}

func (t *toolchainLinuxMuslX8664) Cflags() string {
	return t.toolchainLinuxX8664.Cflags() + " " + t.toolchainMusl.Cflags()
}

func (t *toolchainLinuxMuslX8664) Ldflags() string {
	return t.toolchainLinuxX8664.Ldflags() + " " + t.toolchainMusl.Ldflags()
}

func (t *toolchainLinuxMuslX8664) Lldflags() string {
	return t.toolchainLinuxX8664.Lldflags() + " " + t.toolchainMusl.Lldflags()
}

var toolchainLinuxMuslX86Singleton Toolchain = &toolchainLinuxMuslX86{}
var toolchainLinuxMuslX8664Singleton Toolchain = &toolchainLinuxMuslX8664{}

func linuxMuslX86ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxMuslX86Singleton
}

func linuxMuslX8664ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxMuslX8664Singleton
}

func init() {
	registerToolchainFactory(android.Linux, android.X86, linuxGlibcX86ToolchainFactory)
	registerToolchainFactory(android.Linux, android.X86_64, linuxGlibcX8664ToolchainFactory)
	registerToolchainFactory(android.LinuxMusl, android.X86, linuxMuslX86ToolchainFactory)
	registerToolchainFactory(android.LinuxMusl, android.X86_64, linuxMuslX8664ToolchainFactory)
}
