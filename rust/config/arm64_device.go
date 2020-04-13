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
	Arm64RustFlags            = []string{}
	Arm64ArchFeatureRustFlags = map[string][]string{}
	Arm64LinkFlags            = []string{
		"-Wl,--icf=safe",
		"-Wl,-z,max-page-size=4096",

		"-Wl,-z,separate-code",
	}

	Arm64ArchVariantRustFlags = map[string][]string{
		"armv8-a":  []string{},
		"armv8-2a": []string{},
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

}

type toolchainArm64 struct {
	toolchain64Bit
	toolchainRustFlags string
}

func (t *toolchainArm64) RustTriple() string {
	return "aarch64-linux-android"
}

func (t *toolchainArm64) ToolchainLinkFlags() string {
	return "${config.DeviceGlobalLinkFlags} ${config.Arm64ToolchainLinkFlags}"
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

func Arm64ToolchainFactory(arch android.Arch) Toolchain {
	toolchainRustFlags := []string{
		"${config.Arm64ToolchainRustFlags}",
		"${config.Arm64" + arch.ArchVariant + "VariantRustFlags}",
	}

	toolchainRustFlags = append(toolchainRustFlags, deviceGlobalRustFlags...)

	for _, feature := range arch.ArchFeatures {
		toolchainRustFlags = append(toolchainRustFlags, Arm64ArchFeatureRustFlags[feature]...)
	}

	return &toolchainArm64{
		toolchainRustFlags: strings.Join(toolchainRustFlags, " "),
	}
}
