// Copyright 2020 Google Inc. All rights reserved.
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
	// This is a host toolchain but flags for device toolchain are required
	// as the flags are actually for Bionic-based builds.
	linuxCrossCflags = append(deviceGlobalCflags,
		// clang by default enables PIC when the clang triple is set to *-android.
		// See toolchain/llvm-project/clang/lib/Driver/ToolChains/CommonArgs.cpp#920.
		// However, for this host target, we don't set "-android" to avoid __ANDROID__ macro
		// which stands for "Android device target". Keeping PIC on is required because
		// many modules we have (e.g. Bionic) assume PIC.
		"-fpic",

		// This is normally in ClangExtraTargetCflags, but that's for device and we need
		// the same for host
		"-nostdlibinc",
	)

	linuxCrossLdflags = []string{
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
		"-Wl,--build-id=md5",
		"-Wl,--fatal-warnings",
		"-Wl,--hash-style=gnu",
		"-Wl,--no-undefined-version",
	}

	linuxCrossLldflags = append(linuxCrossLdflags,
		"-Wl,--compress-debug-sections=zstd",
	)

	// Embed the linker into host bionic binaries. This is needed to support host bionic,
	// as the linux kernel requires that the ELF interpreter referenced by PT_INTERP be
	// either an absolute path, or relative from CWD. To work around this, we extract
	// the load sections from the runtime linker ELF binary and embed them into each host
	// bionic binary, omitting the PT_INTERP declaration. The kernel will treat it as a static
	// binary, and then we use a special entry point to fix up the arguments passed by
	// the kernel before jumping to the embedded linker.
	linuxArm64CrtBeginSharedBinary = append(android.CopyOf(bionicCrtBeginSharedBinary),
		"host_bionic_linker_script")
)

func init() {
	exportedVars.ExportStringListStaticVariable("LinuxBionicArm64Cflags", linuxCrossCflags)
	exportedVars.ExportStringListStaticVariable("LinuxBionicArm64Ldflags", linuxCrossLdflags)
	exportedVars.ExportStringListStaticVariable("LinuxBionicArm64Lldflags", linuxCrossLldflags)
}

// toolchain config for ARM64 Linux CrossHost. Almost everything is the same as the ARM64 Android
// target. The overridden methods below show the differences.
type toolchainLinuxBionicArm64 struct {
	toolchainArm64
}

func (toolchainLinuxBionicArm64) ClangTriple() string {
	// Note the absence of "-android" suffix. The compiler won't define __ANDROID__
	return "aarch64-linux"
}

func (toolchainLinuxBionicArm64) Cflags() string {
	// The inherited flags + extra flags
	return "${config.Arm64Cflags} ${config.LinuxBionicArm64Cflags}"
}

func (toolchainLinuxBionicArm64) CrtBeginSharedBinary() []string {
	return linuxArm64CrtBeginSharedBinary
}

func linuxBionicArm64ToolchainFactory(arch android.Arch) Toolchain {
	archVariant := "armv8-a" // for host, default to armv8-a
	toolchainCflags := []string{arm64ArchVariantCflagsVar[archVariant]}

	// We don't specify CPU architecture for host. Conservatively assume
	// the host CPU needs the fix
	extraLdflags := "-Wl,--fix-cortex-a53-843419"

	ret := toolchainLinuxBionicArm64{}

	// add the extra ld and lld flags
	ret.toolchainArm64.ldflags = strings.Join([]string{
		"${config.Arm64Ldflags}",
		"${config.LinuxBionicArm64Ldflags}",
		extraLdflags,
	}, " ")
	ret.toolchainArm64.lldflags = strings.Join([]string{
		"${config.Arm64Lldflags}",
		"${config.LinuxBionicArm64Ldflags}",
		extraLdflags,
	}, " ")
	ret.toolchainArm64.toolchainCflags = strings.Join(toolchainCflags, " ")
	return &ret
}

func init() {
	registerToolchainFactory(android.LinuxBionic, android.Arm64, linuxBionicArm64ToolchainFactory)
}
