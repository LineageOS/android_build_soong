// Copyright 2022 Google Inc. All rights reserved.
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
	linuxArmRustflags   = []string{}
	linuxArmLinkflags   = []string{}
	linuxArm64Rustflags = []string{}
	linuxArm64Linkflags = []string{}
)

func init() {
	registerToolchainFactory(android.LinuxMusl, android.Arm64, linuxMuslArm64ToolchainFactory)
	registerToolchainFactory(android.LinuxMusl, android.Arm, linuxMuslArmToolchainFactory)

	pctx.StaticVariable("LinuxToolchainArmRustFlags", strings.Join(linuxArmRustflags, " "))
	pctx.StaticVariable("LinuxToolchainArmLinkFlags", strings.Join(linuxArmLinkflags, " "))
	pctx.StaticVariable("LinuxToolchainArm64RustFlags", strings.Join(linuxArm64Rustflags, " "))
	pctx.StaticVariable("LinuxToolchainArm64LinkFlags", strings.Join(linuxArm64Linkflags, " "))
}

// Base 64-bit linux rust toolchain
type toolchainLinuxArm64 struct {
	toolchain64Bit
}

func (toolchainLinuxArm64) Supported() bool {
	return true
}

func (toolchainLinuxArm64) Bionic() bool {
	return false
}

func (t *toolchainLinuxArm64) Name() string {
	return "arm64"
}

func (t *toolchainLinuxArm64) ToolchainLinkFlags() string {
	// Prepend the lld flags from cc_config so we stay in sync with cc
	return "${cc_config.LinuxLldflags} ${cc_config.LinuxArm64Lldflags} " +
		"${config.LinuxToolchainLinkFlags} ${config.LinuxToolchainArm64LinkFlags}"
}

func (t *toolchainLinuxArm64) ToolchainRustFlags() string {
	return "${config.LinuxToolchainRustFlags} ${config.LinuxToolchainArm64RustFlags}"
}

// Specialization of the 64-bit linux rust toolchain for musl.  Adds the musl rust triple and
// linker flags to avoid using the host sysroot.
type toolchainLinuxMuslArm64 struct {
	toolchainLinuxArm64
}

func (t *toolchainLinuxMuslArm64) RustTriple() string {
	return "aarch64-unknown-linux-musl"
}

func (t *toolchainLinuxMuslArm64) ToolchainLinkFlags() string {
	return t.toolchainLinuxArm64.ToolchainLinkFlags() + " " + "${config.LinuxMuslToolchainLinkFlags}"
}

func (t *toolchainLinuxMuslArm64) ToolchainRustFlags() string {
	return t.toolchainLinuxArm64.ToolchainRustFlags() + " " + "${config.LinuxMuslToolchainRustFlags}"
}

func linuxMuslArm64ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxMuslArm64Singleton
}

// Base 32-bit linux rust toolchain
type toolchainLinuxArm struct {
	toolchain32Bit
}

func (toolchainLinuxArm) Supported() bool {
	return true
}

func (toolchainLinuxArm) Bionic() bool {
	return false
}

func (t *toolchainLinuxArm) Name() string {
	return "arm"
}

func (toolchainLinuxArm) LibclangRuntimeLibraryArch() string {
	return "arm"
}

func (toolchainLinuxArm64) LibclangRuntimeLibraryArch() string {
	return "arm64"
}

func (t *toolchainLinuxArm) ToolchainLinkFlags() string {
	// Prepend the lld flags from cc_config so we stay in sync with cc
	return "${cc_config.LinuxLldflags} ${cc_config.LinuxArmLldflags} " +
		"${config.LinuxToolchainLinkFlags} ${config.LinuxToolchainArmLinkFlags}"
}

func (t *toolchainLinuxArm) ToolchainRustFlags() string {
	return "${config.LinuxToolchainRustFlags} ${config.LinuxToolchainArmRustFlags}"
}

// Specialization of the 32-bit linux rust toolchain for musl.  Adds the musl rust triple and
// linker flags to avoid using the host sysroot.
type toolchainLinuxMuslArm struct {
	toolchainLinuxArm
}

func (t *toolchainLinuxMuslArm) RustTriple() string {
	return "arm-unknown-linux-musleabihf"
}

func (t *toolchainLinuxMuslArm) ToolchainLinkFlags() string {
	return t.toolchainLinuxArm.ToolchainLinkFlags() + " " + "${config.LinuxMuslToolchainLinkFlags}"
}

func (t *toolchainLinuxMuslArm) ToolchainRustFlags() string {
	return t.toolchainLinuxArm.ToolchainRustFlags() + " " + "${config.LinuxMuslToolchainRustFlags}"
}

func linuxMuslArmToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxMuslArmSingleton
}

var toolchainLinuxMuslArm64Singleton Toolchain = &toolchainLinuxMuslArm64{}
var toolchainLinuxMuslArmSingleton Toolchain = &toolchainLinuxMuslArm{}
