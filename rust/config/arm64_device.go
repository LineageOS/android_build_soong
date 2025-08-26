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

	cc_config "android/soong/cc/config"
)

var (
	Arm64RustFlags = []string{
		"-C force-frame-pointers=y",
	}
	Arm64ArchFeatureRustFlags = map[string][]string{
		// branch-protection=bti,pac-ret is equivalent to Clang's mbranch-protection=standard
		"branchprot": {
			"-Z branch-protection=bti,pac-ret",
			"-Z stack-protector=none",
		},
	}
	Arm64LinkFlags = []string{}

	// We could simply pass "-C target-feature=+v8.2a" and similar, but "v8.2a" and the other
	// architecture version target-features are marked unstable and spam warnings in the build log,
	// even though they're just aliases for groups of other features, most of which are stable.
	// As a workaround, we'll simply look at this file and enable the constituent features:
	// https://doc.rust-lang.org/nightly/nightly-rustc/src/rustc_target/target_features.rs.html

	// Mandatory extensions from ARMv8.1-A and ARMv8.2-A
	armv82aFeatures = "-C target-feature=+crc,+lse,+rdm,+pan,+lor,+vh,+ras,+dpb"
	// Mandatory extensions from ARMv8.3-A, ARMv8.4-A and ARMv8.5-A
	armv85aFeatures = "-C target-feature=+rcpc,+paca,+pacg,+jsconv,+dotprod,+dit,+flagm,+ssbs,+sb,+dpb2,+bti"
	// Mandatory extensions from ARMv8.6-A and ARMv8.7-A
	// "wfxt" is marked unstable, so we don't include it yet.
	armv87aFeatures = "-C target-feature=+bf16,+i8mm"

	Arm64ArchVariantRustFlags = map[string][]string{
		"armv8-a":            {},
		"armv8-a-branchprot": {},
		"armv8-2a": {
			armv82aFeatures,
		},
		"armv8-2a-dotprod": {
			armv82aFeatures,
			"-C target-feature=+dotprod",
		},
		"armv8-5a": {
			armv82aFeatures,
			armv85aFeatures,
		},
		"armv8-7a": {
			armv82aFeatures,
			armv85aFeatures,
			armv87aFeatures,
		},

		// All of the Armv9 entries below should have "-C target-feature=+sve2",
		// but a large subset of Rust code runs in VMs that are incompatible with SVE.
		// TODO: b/409859765 - apply a solution similar to cc_baremetal_defaults.
		"armv9-a": {
			armv82aFeatures,
			armv85aFeatures,
		},
		"armv9-2a": {
			armv82aFeatures,
			armv85aFeatures,
			armv87aFeatures,
		},
		// ARMv9.3-A adds +hbc,+mops but they're both unstable
		"armv9-3a": {
			armv82aFeatures,
			armv85aFeatures,
			armv87aFeatures,
		},
		// ARMv9.4-A adds +cssc but it's unstable
		"armv9-4a": {
			armv82aFeatures,
			armv85aFeatures,
			armv87aFeatures,
		},
	}
)

func init() {
	registerToolchainFactory(android.Android, android.Arm64, Arm64ToolchainFactory)

	pctx.StaticVariable("Arm64ToolchainRustFlags", strings.Join(Arm64RustFlags, " "))
	pctx.StaticVariable("Arm64ToolchainLinkFlags", strings.Join(Arm64LinkFlags, " "))

	for variant, rustFlags := range Arm64ArchVariantRustFlags {
		pctx.VariableFunc("Arm64"+variant+"VariantRustFlags", func(ctx android.PackageVarContext) string {
			if ctx.Config().ReleaseRustUseArmTargetArchVariant() {
				return strings.Join(rustFlags, " ")
			}
			return ""
		})
	}

	pctx.StaticVariable("DEVICE_ARM64_RUSTC_FLAGS", strings.Join(Arm64RustFlags, " "))
}

type toolchainArm64 struct {
	toolchain64Bit
	toolchainRustFlags string
	ldflags string
}

func (t *toolchainArm64) RustTriple() string {
	return "aarch64-linux-android"
}

func (t *toolchainArm64) ToolchainLinkFlags() string {
	// Prepend the lld flags from cc_config so we stay in sync with cc
	return "${config.DeviceGlobalLinkFlags} " + t.ldflags + " ${config.Arm64ToolchainLinkFlags}"
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

	cc_toolchain := cc_config.FindToolchain(android.Android, arch)

	return &toolchainArm64{
		toolchainRustFlags: strings.Join(toolchainRustFlags, " "),
		ldflags: strings.ReplaceAll(cc_toolchain.Ldflags(), "${config.", "${cc_config."),
	}
}
