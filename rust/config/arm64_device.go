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
	Arm64RustFlags = []string{
		"-C force-frame-pointers=y",
	}
	Arm64ArchFeatureRustFlags = map[string][]string{}
	Arm64LinkFlags            = []string{}

	Arm64ArchVariantRustFlags = map[string][]string{
		"armv8-a": []string{},
		"armv8-a-branchprot": []string{
			// branch-protection=bti,pac-ret is equivalent to Clang's mbranch-protection=standard
			"-Z branch-protection=bti,pac-ret",
		},
		"armv8-2a":         []string{},
		"armv8-2a-dotprod": []string{},
		"armv9-a": []string{
			// branch-protection=bti,pac-ret is equivalent to Clang's mbranch-protection=standard
			"-Z branch-protection=bti,pac-ret",
			"-Z stack-protector=none",
		},
	}
)

func init() {
	registerToolchainFactory(android.Android, android.Arm64, Arm64ToolchainFactory)

	pctx.StaticVariable("Arm64ToolchainRustFlags", strings.Join(Arm64RustFlags, " "))
	pctx.StaticVariable("Arm64ToolchainLinkFlags", strings.Join(Arm64LinkFlags, " "))

	for variant, rustFlags := range Arm64ArchVariantRustFlags {
		pctx.StaticVariable("Arm64"+variant+"VariantRustFlags",
			strings.Join(rustFlags, " "))
	}

	ExportedVars.ExportStringListStaticVariable("DEVICE_ARM64_RUSTC_FLAGS", Arm64RustFlags)
}

type toolchainArm64 struct {
	toolchain64Bit
	toolchainRustFlags string
}

func (t *toolchainArm64) RustTriple() string {
	return "aarch64-linux-android"
}

func (t *toolchainArm64) ToolchainLinkFlags() string {
	// Prepend the lld flags from cc_config so we stay in sync with cc
	return "${config.DeviceGlobalLinkFlags} ${cc_config.Arm64Lldflags} ${config.Arm64ToolchainLinkFlags}"
}

func (t *toolchainArm64) ToolchainRustFlags() string {
	return t.toolchainRustFlags
}

func (t *toolchainArm64) RustFlags() string {
	return "${config.Arm64ToolchainRustFlags}"
}

func (t *toolchainArm64) Supported() bool {
	return true
}

func (toolchainArm64) LibclangRuntimeLibraryArch() string {
	return "aarch64"
}

func Arm64ToolchainFactory(arch android.Arch) Toolchain {
	archVariant := arch.ArchVariant
	if archVariant == "" {
		// arch variants defaults to armv8-a. This is mostly for
		// the host target which borrows toolchain configs from here.
		archVariant = "armv8-a"
	}

	toolchainRustFlags := []string{
		"${config.Arm64ToolchainRustFlags}",
		"${config.Arm64" + archVariant + "VariantRustFlags}",
	}

	toolchainRustFlags = append(toolchainRustFlags, deviceGlobalRustFlags...)

	for _, feature := range arch.ArchFeatures {
		toolchainRustFlags = append(toolchainRustFlags, Arm64ArchFeatureRustFlags[feature]...)
	}

	return &toolchainArm64{
		toolchainRustFlags: strings.Join(toolchainRustFlags, " "),
	}
}
