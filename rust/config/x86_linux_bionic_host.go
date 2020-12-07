// Copyright 2020 The Android Open Source Project
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
	LinuxBionicRustFlags     = []string{}
	LinuxBionicRustLinkFlags = []string{
		"-B${cc_config.ClangBin}",
		"-fuse-ld=lld",
		"-Wl,--undefined-version",
		"-nostdlib",
	}
)

func init() {
	registerToolchainFactory(android.LinuxBionic, android.X86_64, linuxBionicX8664ToolchainFactory)

	pctx.StaticVariable("LinuxBionicToolchainRustFlags", strings.Join(LinuxBionicRustFlags, " "))
	pctx.StaticVariable("LinuxBionicToolchainLinkFlags", strings.Join(LinuxBionicRustLinkFlags, " "))
}

type toolchainLinuxBionicX8664 struct {
	toolchain64Bit
}

func (toolchainLinuxBionicX8664) Supported() bool {
	return true
}

func (toolchainLinuxBionicX8664) Bionic() bool {
	return true
}

func (t *toolchainLinuxBionicX8664) Name() string {
	return "x86_64"
}

func (t *toolchainLinuxBionicX8664) RustTriple() string {
	return "x86_64-linux-android"
}

func (t *toolchainLinuxBionicX8664) ToolchainLinkFlags() string {
	// Prepend the lld flags from cc_config so we stay in sync with cc
	return "${cc_config.LinuxBionicLldflags} ${config.LinuxBionicToolchainLinkFlags}"
}

func (t *toolchainLinuxBionicX8664) ToolchainRustFlags() string {
	return "${config.LinuxBionicToolchainRustFlags}"
}

func linuxBionicX8664ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxBionicX8664Singleton
}

var toolchainLinuxBionicX8664Singleton Toolchain = &toolchainLinuxBionicX8664{}
