// Copyright 2019 The Android Open Source Project
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
	LinuxRustFlags     = []string{}
	LinuxMuslRustFlags = []string{
		// disable rustc's builtin fallbacks for crt objects
		"-C link_self_contained=no",
		// force rustc to use a dynamic musl libc
		"-C target-feature=-crt-static",
		"-Z link-native-libraries=no",
	}
	LinuxRustLinkFlags = []string{
		"-B${cc_config.ClangBin}",
		"-fuse-ld=lld",
		"-Wl,--undefined-version",
	}
	LinuxRustGlibcLinkFlags = []string{
		"--sysroot ${cc_config.LinuxGccRoot}/sysroot",
	}
	LinuxRustMuslLinkFlags = []string{
		"--sysroot /dev/null",
		"-nodefaultlibs",
		"-nostdlib",
		"-Wl,--no-dynamic-linker",
	}
	linuxX86Rustflags   = []string{}
	linuxX86Linkflags   = []string{}
	linuxX8664Rustflags = []string{}
	linuxX8664Linkflags = []string{}
)

func init() {
	registerToolchainFactory(android.Linux, android.X86_64, linuxGlibcX8664ToolchainFactory)
	registerToolchainFactory(android.Linux, android.X86, linuxGlibcX86ToolchainFactory)

	registerToolchainFactory(android.LinuxMusl, android.X86_64, linuxMuslX8664ToolchainFactory)
	registerToolchainFactory(android.LinuxMusl, android.X86, linuxMuslX86ToolchainFactory)

	pctx.StaticVariable("LinuxToolchainRustFlags", strings.Join(LinuxRustFlags, " "))
	pctx.StaticVariable("LinuxMuslToolchainRustFlags", strings.Join(LinuxMuslRustFlags, " "))
	pctx.StaticVariable("LinuxToolchainLinkFlags", strings.Join(LinuxRustLinkFlags, " "))
	pctx.StaticVariable("LinuxGlibcToolchainLinkFlags", strings.Join(LinuxRustGlibcLinkFlags, " "))
	pctx.StaticVariable("LinuxMuslToolchainLinkFlags", strings.Join(LinuxRustMuslLinkFlags, " "))
	pctx.StaticVariable("LinuxToolchainX86RustFlags", strings.Join(linuxX86Rustflags, " "))
	pctx.StaticVariable("LinuxToolchainX86LinkFlags", strings.Join(linuxX86Linkflags, " "))
	pctx.StaticVariable("LinuxToolchainX8664RustFlags", strings.Join(linuxX8664Rustflags, " "))
	pctx.StaticVariable("LinuxToolchainX8664LinkFlags", strings.Join(linuxX8664Linkflags, " "))

}

// Base 64-bit linux rust toolchain
type toolchainLinuxX8664 struct {
	toolchain64Bit
}

func (toolchainLinuxX8664) Supported() bool {
	return true
}

func (toolchainLinuxX8664) Bionic() bool {
	return false
}

func (t *toolchainLinuxX8664) Name() string {
	return "x86_64"
}

func (t *toolchainLinuxX8664) ToolchainLinkFlags() string {
	// Prepend the lld flags from cc_config so we stay in sync with cc
	return "${cc_config.LinuxLldflags} ${cc_config.LinuxX8664Lldflags} " +
		"${config.LinuxToolchainLinkFlags} ${config.LinuxToolchainX8664LinkFlags}"
}

func (t *toolchainLinuxX8664) ToolchainRustFlags() string {
	return "${config.LinuxToolchainRustFlags} ${config.LinuxToolchainX8664RustFlags}"
}

// Specialization of the 64-bit linux rust toolchain for glibc.  Adds the gnu rust triple and
// sysroot linker flags.
type toolchainLinuxGlibcX8664 struct {
	toolchainLinuxX8664
}

func (t *toolchainLinuxX8664) RustTriple() string {
	return "x86_64-unknown-linux-gnu"
}

func (t *toolchainLinuxGlibcX8664) ToolchainLinkFlags() string {
	return t.toolchainLinuxX8664.ToolchainLinkFlags() + " " + "${config.LinuxGlibcToolchainLinkFlags}"
}

func linuxGlibcX8664ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxGlibcX8664Singleton
}

// Specialization of the 64-bit linux rust toolchain for musl.  Adds the musl rust triple and
// linker flags to avoid using the host sysroot.
type toolchainLinuxMuslX8664 struct {
	toolchainLinuxX8664
}

func (t *toolchainLinuxMuslX8664) RustTriple() string {
	return "x86_64-unknown-linux-musl"
}

func (t *toolchainLinuxMuslX8664) ToolchainLinkFlags() string {
	return t.toolchainLinuxX8664.ToolchainLinkFlags() + " " + "${config.LinuxMuslToolchainLinkFlags}"
}

func (t *toolchainLinuxMuslX8664) ToolchainRustFlags() string {
	return t.toolchainLinuxX8664.ToolchainRustFlags() + " " + "${config.LinuxMuslToolchainRustFlags}"
}

func linuxMuslX8664ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxMuslX8664Singleton
}

// Base 32-bit linux rust toolchain
type toolchainLinuxX86 struct {
	toolchain32Bit
}

func (toolchainLinuxX86) Supported() bool {
	return true
}

func (toolchainLinuxX86) Bionic() bool {
	return false
}

func (t *toolchainLinuxX86) Name() string {
	return "x86"
}

func (toolchainLinuxX86) LibclangRuntimeLibraryArch() string {
	return "i386"
}

func (toolchainLinuxX8664) LibclangRuntimeLibraryArch() string {
	return "x86_64"
}

func (t *toolchainLinuxX86) ToolchainLinkFlags() string {
	// Prepend the lld flags from cc_config so we stay in sync with cc
	return "${cc_config.LinuxLldflags} ${cc_config.LinuxX86Lldflags} " +
		"${config.LinuxToolchainLinkFlags} ${config.LinuxToolchainX86LinkFlags}"
}

func (t *toolchainLinuxX86) ToolchainRustFlags() string {
	return "${config.LinuxToolchainRustFlags} ${config.LinuxToolchainX86RustFlags}"
}

// Specialization of the 32-bit linux rust toolchain for glibc.  Adds the gnu rust triple and
// sysroot linker flags.
type toolchainLinuxGlibcX86 struct {
	toolchainLinuxX86
}

func (t *toolchainLinuxGlibcX86) RustTriple() string {
	return "i686-unknown-linux-gnu"
}

func (t *toolchainLinuxGlibcX86) ToolchainLinkFlags() string {
	return t.toolchainLinuxX86.ToolchainLinkFlags() + " " + "${config.LinuxGlibcToolchainLinkFlags}"
}

func linuxGlibcX86ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxGlibcX86Singleton
}

// Specialization of the 32-bit linux rust toolchain for musl.  Adds the musl rust triple and
// linker flags to avoid using the host sysroot.
type toolchainLinuxMuslX86 struct {
	toolchainLinuxX86
}

func (t *toolchainLinuxMuslX86) RustTriple() string {
	return "i686-unknown-linux-musl"
}

func (t *toolchainLinuxMuslX86) ToolchainLinkFlags() string {
	return t.toolchainLinuxX86.ToolchainLinkFlags() + " " + "${config.LinuxMuslToolchainLinkFlags}"
}

func (t *toolchainLinuxMuslX86) ToolchainRustFlags() string {
	return t.toolchainLinuxX86.ToolchainRustFlags() + " " + "${config.LinuxMuslToolchainRustFlags}"
}

func linuxMuslX86ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxMuslX86Singleton
}

var toolchainLinuxGlibcX8664Singleton Toolchain = &toolchainLinuxGlibcX8664{}
var toolchainLinuxGlibcX86Singleton Toolchain = &toolchainLinuxGlibcX86{}
var toolchainLinuxMuslX8664Singleton Toolchain = &toolchainLinuxMuslX8664{}
var toolchainLinuxMuslX86Singleton Toolchain = &toolchainLinuxMuslX86{}
