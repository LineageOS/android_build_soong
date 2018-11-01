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
	linuxBionicCflags = ClangFilterUnknownCflags([]string{
		"-fdiagnostics-color",

		"-Wa,--noexecstack",

		"-fPIC",

		"-U_FORTIFY_SOURCE",
		"-D_FORTIFY_SOURCE=2",
		"-fstack-protector-strong",

		// From x86_64_device
		"-ffunction-sections",
		"-finline-functions",
		"-finline-limit=300",
		"-fno-short-enums",
		"-funswitch-loops",
		"-funwind-tables",
		"-fno-canonical-system-headers",

		// Tell clang where the gcc toolchain is
		"--gcc-toolchain=${LinuxBionicGccRoot}",

		// This is normally in ClangExtraTargetCflags, but this is considered host
		"-nostdlibinc",
	})

	linuxBionicLdflags = ClangFilterUnknownCflags([]string{
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
		"-Wl,--build-id=md5",
		"-Wl,--warn-shared-textrel",
		"-Wl,--fatal-warnings",
		"-Wl,--hash-style=gnu",
		"-Wl,--no-undefined-version",

		// Use the device gcc toolchain
		"--gcc-toolchain=${LinuxBionicGccRoot}",
	})

	linuxBionicLldflags = ClangFilterUnknownLldflags(linuxBionicLdflags)
)

func init() {
	pctx.StaticVariable("LinuxBionicCflags", strings.Join(linuxBionicCflags, " "))
	pctx.StaticVariable("LinuxBionicLdflags", strings.Join(linuxBionicLdflags, " "))
	pctx.StaticVariable("LinuxBionicLldflags", strings.Join(linuxBionicLldflags, " "))

	pctx.StaticVariable("LinuxBionicIncludeFlags", bionicHeaders("x86"))

	// Use the device gcc toolchain for now
	pctx.StaticVariable("LinuxBionicGccRoot", "${X86_64GccRoot}")
}

type toolchainLinuxBionic struct {
	toolchain64Bit
}

func (t *toolchainLinuxBionic) Name() string {
	return "x86_64"
}

func (t *toolchainLinuxBionic) GccRoot() string {
	return "${config.LinuxBionicGccRoot}"
}

func (t *toolchainLinuxBionic) GccTriple() string {
	return "x86_64-linux-android"
}

func (t *toolchainLinuxBionic) GccVersion() string {
	return "4.9"
}

func (t *toolchainLinuxBionic) IncludeFlags() string {
	return "${config.LinuxBionicIncludeFlags}"
}

func (t *toolchainLinuxBionic) ClangTriple() string {
	// TODO: we don't have a triple yet b/31393676
	return "x86_64-linux-android"
}

func (t *toolchainLinuxBionic) ClangCflags() string {
	return "${config.LinuxBionicCflags}"
}

func (t *toolchainLinuxBionic) ClangCppflags() string {
	return ""
}

func (t *toolchainLinuxBionic) ClangLdflags() string {
	return "${config.LinuxBionicLdflags}"
}

func (t *toolchainLinuxBionic) ClangLldflags() string {
	return "${config.LinuxBionicLldflags}"
}

func (t *toolchainLinuxBionic) ToolchainClangCflags() string {
	return "-m64 -march=x86-64" +
		// TODO: We're not really android, but we don't have a triple yet b/31393676
		" -U__ANDROID__"
}

func (t *toolchainLinuxBionic) ToolchainClangLdflags() string {
	return "-m64"
}

func (t *toolchainLinuxBionic) AvailableLibraries() []string {
	return nil
}

func (t *toolchainLinuxBionic) Bionic() bool {
	return true
}

func (toolchainLinuxBionic) LibclangRuntimeLibraryArch() string {
	return "x86_64"
}

var toolchainLinuxBionicSingleton Toolchain = &toolchainLinuxBionic{}

func linuxBionicToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxBionicSingleton
}

func init() {
	registerToolchainFactory(android.LinuxBionic, android.X86_64, linuxBionicToolchainFactory)
}
