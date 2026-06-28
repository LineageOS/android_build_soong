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
	"fmt"
	"path/filepath"
	"strings"

	"android/soong/android"
)

const (
	linuxGccVersion   = "4.8.3"
	linuxGccTriple    = "x86_64-linux"
	linuxGlibcVersion = "2.17"
)

var (
	linuxCflags = []string{
		"-Wa,--noexecstack",

		"-fPIC",

		"-fno-omit-frame-pointer",

		"-U_FORTIFY_SOURCE",
		"-D_FORTIFY_SOURCE=3",
		"-fstack-protector",

		"--gcc-toolchain=${LinuxGccRoot}",
		"-fstack-protector-strong",
	}

	linuxGlibcCflags = []string{
		"--sysroot ${LinuxGccRoot}/sysroot",
	}

	linuxMuslCflags = []string{
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

		"-Wl,--compress-debug-sections=zstd",

		"-Wl,--build-id=md5",
	}

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
		"-msse3",
		"-m64",
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

func LinuxX86Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	linuxGccRoot := LinuxGccRoot()
	depsPhony := ctx.CreateNinjaPhonyOnce("linuxX86LdFlagsDeps", []string{
		filepath.Join(linuxGccRoot, "lib/gcc", linuxGccTriple, linuxGccVersion, "32", "*"),
		filepath.Join(linuxGccRoot, linuxGccTriple, "lib32", "*"),
	})
	return FlagsWithDeps{
		Flags: strings.Join([]string{
			"-m32",
			"-B" + linuxGccRoot + "/lib/gcc/" + linuxGccTriple + "/" + linuxGccVersion + "/32",
			"-L" + linuxGccRoot + "/lib/gcc/" + linuxGccTriple + "/" + linuxGccVersion + "/32",
			"-L" + linuxGccRoot + "/" + linuxGccTriple + "/lib32",
		}, " "),
		Deps: android.Paths{depsPhony},
	}
}

func LinuxX8664Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	linuxGccRoot := LinuxGccRoot()
	depsPhony := ctx.CreateNinjaPhonyOnce("linuxX8664LdFlagsDeps", []string{
		filepath.Join(linuxGccRoot, "lib/gcc", linuxGccTriple, linuxGccVersion, "*"),
		filepath.Join(linuxGccRoot, linuxGccTriple, "lib64", "*"),
	})
	return FlagsWithDeps{
		Flags: strings.Join([]string{
			"-m64",
			"-B" + linuxGccRoot + "/lib/gcc/" + linuxGccTriple + "/" + linuxGccVersion,
			"-L" + linuxGccRoot + "/lib/gcc/" + linuxGccTriple + "/" + linuxGccVersion,
			"-L" + linuxGccRoot + "/" + linuxGccTriple + "/lib64",
		}, " "),
		Deps: android.Paths{depsPhony},
	}
}

func init() {
	pctx.StaticVariable("LinuxGccVersion", linuxGccVersion)
	pctx.StaticVariable("LinuxGlibcVersion", linuxGlibcVersion)

	pctx.SourcePathVariable("LinuxGccRoot", LinuxGccRoot())

	pctx.StaticVariable("LinuxGccTriple", linuxGccTriple)

	pctx.StaticVariable("LinuxCflags", strings.Join(linuxCflags, " "))
	pctx.StaticVariable("LinuxLdflags", strings.Join(linuxLdflags, " "))
	pctx.StaticVariable("LinuxGlibcCflags", strings.Join(linuxGlibcCflags, " "))
	pctx.StaticVariable("LinuxGlibcLdflags", strings.Join(linuxGlibcLdflags, " "))
	pctx.StaticVariable("LinuxMuslCflags", strings.Join(linuxMuslCflags, " "))
	pctx.StaticVariable("LinuxMuslLdflags", strings.Join(linuxMuslLdflags, " "))

	pctx.StaticVariable("LinuxX86Cflags", strings.Join(linuxX86Cflags, " "))
	pctx.StaticVariable("LinuxX8664Cflags", strings.Join(linuxX8664Cflags, " "))
	// Yasm flags
	pctx.StaticVariable("LinuxX86YasmFlags", "-f elf32 -m x86")
	pctx.StaticVariable("LinuxX8664YasmFlags", "-f elf64 -m amd64")
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

func (t *toolchainLinuxX86) Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	return FlagsWithDeps{
		Flags: "${config.LinuxLdflags}",
	}.Append(LinuxX86Ldflags(ctx))
}

func (t *toolchainLinuxX8664) Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	preflags := FlagsWithDeps{
		Flags: "${config.LinuxLdflags}",
	}
	return preflags.Append(LinuxX8664Ldflags(ctx))
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

func (toolchainGlibc) Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	return FlagsWithDeps{
		Flags: "${config.LinuxGlibcLdflags}",
	}
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

func (t *toolchainLinuxGlibcX86) Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	return t.toolchainLinuxX86.Ldflags(ctx).Append(t.toolchainGlibc.Ldflags(ctx))
}

func (t *toolchainLinuxGlibcX8664) ClangTriple() string {
	return "x86_64-linux-gnu"
}

func (t *toolchainLinuxGlibcX8664) Cflags() string {
	return t.toolchainLinuxX8664.Cflags() + " " + t.toolchainGlibc.Cflags()
}

func (t *toolchainLinuxGlibcX8664) Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	return t.toolchainLinuxX8664.Ldflags(ctx).Append(t.toolchainGlibc.Ldflags(ctx))
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

func (toolchainMusl) Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	return FlagsWithDeps{
		Flags: "${config.LinuxMuslLdflags}",
	}
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

func (t *toolchainLinuxMuslX86) Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	return t.toolchainLinuxX86.Ldflags(ctx).Append(t.toolchainMusl.Ldflags(ctx))
}

func (t *toolchainLinuxMuslX8664) ClangTriple() string {
	return "x86_64-linux-musl"
}

func (t *toolchainLinuxMuslX8664) Cflags() string {
	return t.toolchainLinuxX8664.Cflags() + " " + t.toolchainMusl.Cflags()
}

func (t *toolchainLinuxMuslX8664) Ldflags(ctx ToolchainFlagsContext) FlagsWithDeps {
	return t.toolchainLinuxX8664.Ldflags(ctx).Append(t.toolchainMusl.Ldflags(ctx))
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

func LinuxGccRoot() string {
	shortVersion := linuxGlibcVersion
	if p := strings.Split(linuxGccVersion, "."); len(p) > 2 {
		shortVersion = strings.Join(p[:2], ".")
	}
	return fmt.Sprintf("prebuilts/gcc/linux-x86/host/x86_64-linux-glibc%s-%s", linuxGlibcVersion, shortVersion)
}

func LinuxGccVersion() string {
	return linuxGccVersion
}

func LinuxGccTriple() string {
	return linuxGccTriple
}
