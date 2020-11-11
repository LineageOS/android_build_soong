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
	"android/soong/android"
	"strings"
)

var (
	// This is a host toolchain but flags for device toolchain are required
	// as the flags are actually for Bionic-based builds.
	linuxCrossCflags = ClangFilterUnknownCflags(append(deviceGlobalCflags,
		// clang by default enables PIC when the clang triple is set to *-android.
		// See toolchain/llvm-project/clang/lib/Driver/ToolChains/CommonArgs.cpp#920.
		// However, for this host target, we don't set "-android" to avoid __ANDROID__ macro
		// which stands for "Android device target". Keeping PIC on is required because
		// many modules we have (e.g. Bionic) assume PIC.
		"-fpic",

		// This is normally in ClangExtraTargetCflags, but that's for device and we need
		// the same for host
		"-nostdlibinc",
	))

	linuxCrossLdflags = ClangFilterUnknownCflags([]string{
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
		"-Wl,--build-id=md5",
		"-Wl,--warn-shared-textrel",
		"-Wl,--fatal-warnings",
		"-Wl,--hash-style=gnu",
		"-Wl,--no-undefined-version",
	})
)

func init() {
	pctx.StaticVariable("LinuxBionicArm64Cflags", strings.Join(linuxCrossCflags, " "))
	pctx.StaticVariable("LinuxBionicArm64Ldflags", strings.Join(linuxCrossLdflags, " "))
}

// toolchain config for ARM64 Linux CrossHost. Almost everything is the same as the ARM64 Android
// target. The overridden methods below show the differences.
type toolchainLinuxArm64 struct {
	toolchainArm64
}

func (toolchainLinuxArm64) ClangTriple() string {
	// Note the absence of "-android" suffix. The compiler won't define __ANDROID__
	return "aarch64-linux"
}

func (toolchainLinuxArm64) ClangCflags() string {
	// The inherited flags + extra flags
	return "${config.Arm64ClangCflags} ${config.LinuxBionicArm64Cflags}"
}

func linuxArm64ToolchainFactory(arch android.Arch) Toolchain {
	archVariant := "armv8-a" // for host, default to armv8-a
	toolchainClangCflags := []string{arm64ClangArchVariantCflagsVar[archVariant]}

	// We don't specify CPU architecture for host. Conservatively assume
	// the host CPU needs the fix
	extraLdflags := "-Wl,--fix-cortex-a53-843419"

	ret := toolchainLinuxArm64{}

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
	ret.toolchainArm64.toolchainClangCflags = strings.Join(toolchainClangCflags, " ")
	return &ret
}

func init() {
	registerToolchainFactory(android.LinuxBionic, android.Arm64, linuxArm64ToolchainFactory)
}
