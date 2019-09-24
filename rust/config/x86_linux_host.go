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
	LinuxRustFlags      = []string{}
	LinuxRustLinkFlags  = []string{}
	linuxX86Rustflags   = []string{}
	linuxX86Linkflags   = []string{}
	linuxX8664Rustflags = []string{}
	linuxX8664Linkflags = []string{}
)

func init() {
	registerToolchainFactory(android.Linux, android.X86_64, linuxX8664ToolchainFactory)
	registerToolchainFactory(android.Linux, android.X86, linuxX86ToolchainFactory)

	pctx.StaticVariable("LinuxToolchainRustFlags", strings.Join(LinuxRustFlags, " "))
	pctx.StaticVariable("LinuxToolchainLinkFlags", strings.Join(LinuxRustLinkFlags, " "))
	pctx.StaticVariable("LinuxToolchainX86RustFlags", strings.Join(linuxX86Rustflags, " "))
	pctx.StaticVariable("LinuxToolchainX86LinkFlags", strings.Join(linuxX86Linkflags, " "))
	pctx.StaticVariable("LinuxToolchainX8664RustFlags", strings.Join(linuxX8664Rustflags, " "))
	pctx.StaticVariable("LinuxToolchainX8664LinkFlags", strings.Join(linuxX8664Linkflags, " "))

}

type toolchainLinux struct {
	toolchainRustFlags string
	toolchainLinkFlags string
}

type toolchainLinuxX86 struct {
	toolchain32Bit
	toolchainLinux
}

type toolchainLinuxX8664 struct {
	toolchain64Bit
	toolchainLinux
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

func (t *toolchainLinuxX8664) RustTriple() string {
	return "x86_64-unknown-linux-gnu"
}

func (t *toolchainLinuxX8664) ToolchainLinkFlags() string {
	return "${config.LinuxToolchainLinkFlags} ${config.LinuxToolchainX8664LinkFlags}"
}

func (t *toolchainLinuxX8664) ToolchainRustFlags() string {
	return "${config.LinuxToolchainRustFlags} ${config.LinuxToolchainX8664RustFlags}"
}

func linuxX8664ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxX8664Singleton
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

func (t *toolchainLinuxX86) RustTriple() string {
	return "i686-unknown-linux-gnu"
}

func (t *toolchainLinuxX86) ToolchainLinkFlags() string {
	return "${config.LinuxToolchainLinkFlags} ${config.LinuxToolchainX86LinkFlags}"
}

func (t *toolchainLinuxX86) ToolchainRustFlags() string {
	return "${config.LinuxToolchainRustFlags} ${config.LinuxToolchainX86RustFlags}"
}

func linuxX86ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxX86Singleton
}

var toolchainLinuxX8664Singleton Toolchain = &toolchainLinuxX8664{}
var toolchainLinuxX86Singleton Toolchain = &toolchainLinuxX86{}
