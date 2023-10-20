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
	"android/soong/android"
)

var (
	linuxBionicCflags = []string{
		"-Wa,--noexecstack",

		"-fPIC",

		"-U_FORTIFY_SOURCE",
		"-D_FORTIFY_SOURCE=2",
		"-fstack-protector-strong",

		// From x86_64_device
		"-ffunction-sections",
		"-fno-short-enums",
		"-funwind-tables",

		// Tell clang where the gcc toolchain is
		"--gcc-toolchain=${LinuxBionicGccRoot}",

		// This is normally in ClangExtraTargetCflags, but this is considered host
		"-nostdlibinc",
	}

	linuxBionicLdflags = []string{
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
		"-Wl,--build-id=md5",
		"-Wl,--fatal-warnings",
		"-Wl,--hash-style=gnu",
		"-Wl,--no-undefined-version",

		// Use the device gcc toolchain
		"--gcc-toolchain=${LinuxBionicGccRoot}",
	}

	linuxBionicLldflags = append(linuxBionicLdflags,
		"-Wl,--compress-debug-sections=zstd",
	)

	// Embed the linker into host bionic binaries. This is needed to support host bionic,
	// as the linux kernel requires that the ELF interpreter referenced by PT_INTERP be
	// either an absolute path, or relative from CWD. To work around this, we extract
	// the load sections from the runtime linker ELF binary and embed them into each host
	// bionic binary, omitting the PT_INTERP declaration. The kernel will treat it as a static
	// binary, and then we use a special entry point to fix up the arguments passed by
	// the kernel before jumping to the embedded linker.
	linuxBionicCrtBeginSharedBinary = append(android.CopyOf(bionicCrtBeginSharedBinary),
		"host_bionic_linker_script")
)

const (
	x86_64GccVersion = "4.9"
)

func init() {
	exportedVars.ExportStringListStaticVariable("LinuxBionicCflags", linuxBionicCflags)
	exportedVars.ExportStringListStaticVariable("LinuxBionicLdflags", linuxBionicLdflags)
	exportedVars.ExportStringListStaticVariable("LinuxBionicLldflags", linuxBionicLldflags)

	// Use the device gcc toolchain for now
	exportedVars.ExportStringStaticVariable("LinuxBionicGccVersion", x86_64GccVersion)
	exportedVars.ExportSourcePathVariable("LinuxBionicGccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/x86/x86_64-linux-android-${LinuxBionicGccVersion}")
}

type toolchainLinuxBionic struct {
	toolchain64Bit
	toolchainBionic
}

func (t *toolchainLinuxBionic) Name() string {
	return "x86_64"
}

func (t *toolchainLinuxBionic) IncludeFlags() string {
	return ""
}

func (t *toolchainLinuxBionic) ClangTriple() string {
	// TODO: we don't have a triple yet b/31393676
	return "x86_64-linux-android"
}

func (t *toolchainLinuxBionic) Cflags() string {
	return "${config.LinuxBionicCflags}"
}

func (t *toolchainLinuxBionic) Cppflags() string {
	return ""
}

func (t *toolchainLinuxBionic) Ldflags() string {
	return "${config.LinuxBionicLdflags}"
}

func (t *toolchainLinuxBionic) Lldflags() string {
	return "${config.LinuxBionicLldflags}"
}

func (t *toolchainLinuxBionic) ToolchainCflags() string {
	return "-m64 -march=x86-64" +
		// TODO: We're not really android, but we don't have a triple yet b/31393676
		" -U__ANDROID__"
}

func (t *toolchainLinuxBionic) ToolchainLdflags() string {
	return "-m64"
}

func (t *toolchainLinuxBionic) AvailableLibraries() []string {
	return nil
}

func (toolchainLinuxBionic) LibclangRuntimeLibraryArch() string {
	return "x86_64"
}

func (toolchainLinuxBionic) CrtBeginSharedBinary() []string {
	return linuxBionicCrtBeginSharedBinary
}

var toolchainLinuxBionicSingleton Toolchain = &toolchainLinuxBionic{}

func linuxBionicToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxBionicSingleton
}

func init() {
	registerToolchainFactory(android.LinuxBionic, android.X86_64, linuxBionicToolchainFactory)
}
