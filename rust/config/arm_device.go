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
	ArmRustFlags            = []string{}
	ArmArchFeatureRustFlags = map[string][]string{}
	ArmLinkFlags            = []string{
		"-Wl,--icf=safe",
		"-Wl,-m,armelf",
	}

	ArmArchVariantRustFlags = map[string][]string{
		"armv7-a":      []string{},
		"armv7-a-neon": []string{},
		"armv8-a":      []string{},
		"armv8-2a":     []string{},
	}
)

func init() {
	registerToolchainFactory(android.Android, android.Arm, ArmToolchainFactory)

	pctx.StaticVariable("ArmToolchainRustFlags", strings.Join(ArmRustFlags, " "))
	pctx.StaticVariable("ArmToolchainLinkFlags", strings.Join(ArmLinkFlags, " "))

	for variant, rustFlags := range ArmArchVariantRustFlags {
		pctx.StaticVariable("Arm"+variant+"VariantRustFlags",
			strings.Join(rustFlags, " "))
	}

}

type toolchainArm struct {
	toolchain64Bit
	toolchainRustFlags string
}

func (t *toolchainArm) RustTriple() string {
	return "arm-linux-androideabi"
}

func (t *toolchainArm) ToolchainLinkFlags() string {
	return "${config.DeviceGlobalLinkFlags} ${config.ArmToolchainLinkFlags}"
}

func (t *toolchainArm) ToolchainRustFlags() string {
	return t.toolchainRustFlags
}

func (t *toolchainArm) RustFlags() string {
	return "${config.ArmToolchainRustFlags}"
}

func (t *toolchainArm) Supported() bool {
	return true
}

func ArmToolchainFactory(arch android.Arch) Toolchain {
	toolchainRustFlags := []string{
		"${config.ArmToolchainRustFlags}",
		"${config.Arm" + arch.ArchVariant + "VariantRustFlags}",
	}

	toolchainRustFlags = append(toolchainRustFlags, deviceGlobalRustFlags...)

	for _, feature := range arch.ArchFeatures {
		toolchainRustFlags = append(toolchainRustFlags, ArmArchFeatureRustFlags[feature]...)
	}

	return &toolchainArm{
		toolchainRustFlags: strings.Join(toolchainRustFlags, " "),
	}
}
