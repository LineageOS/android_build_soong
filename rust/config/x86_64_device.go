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
	x86_64RustFlags = []string{
		"-C force-frame-pointers=y",
	}
	x86_64ArchFeatureRustFlags = map[string][]string{}
	x86_64LinkFlags            = []string{}

	x86_64ArchVariantRustFlags = map[string][]string{
		"":              []string{},
		"broadwell":     []string{"-C target-cpu=broadwell"},
		"goldmont":      []string{"-C target-cpu=goldmont"},
		"goldmont-plus": []string{"-C target-cpu=goldmont-plus"},
		"haswell":       []string{"-C target-cpu=haswell"},
		"ivybridge":     []string{"-C target-cpu=ivybridge"},
		"sandybridge":   []string{"-C target-cpu=sandybridge"},
		"silvermont":    []string{"-C target-cpu=silvermont"},
		"skylake":       []string{"-C target-cpu=skylake"},
		//TODO: Add target-cpu=stoneyridge when rustc supports it.
		"stoneyridge": []string{""},
		"tremont":     []string{"-C target-cpu=tremont"},
	}
)

func init() {
	registerToolchainFactory(android.Android, android.X86_64, x86_64ToolchainFactory)

	pctx.StaticVariable("X86_64ToolchainRustFlags", strings.Join(x86_64RustFlags, " "))
	pctx.StaticVariable("X86_64ToolchainLinkFlags", strings.Join(x86_64LinkFlags, " "))

	for variant, rustFlags := range x86_64ArchVariantRustFlags {
		pctx.StaticVariable("X86_64"+variant+"VariantRustFlags",
			strings.Join(rustFlags, " "))
	}
	ExportedVars.ExportStringListStaticVariable("DEVICE_X86_64_RUSTC_FLAGS", x86_64RustFlags)
}

type toolchainX86_64 struct {
	toolchain64Bit
	toolchainRustFlags string
}

func (t *toolchainX86_64) RustTriple() string {
	return "x86_64-linux-android"
}

func (t *toolchainX86_64) ToolchainLinkFlags() string {
	// Prepend the lld flags from cc_config so we stay in sync with cc
	return "${config.DeviceGlobalLinkFlags} ${cc_config.X86_64Lldflags} ${config.X86_64ToolchainLinkFlags}"
}

func (t *toolchainX86_64) ToolchainRustFlags() string {
	return t.toolchainRustFlags
}

func (t *toolchainX86_64) RustFlags() string {
	return "${config.X86_64ToolchainRustFlags}"
}

func (t *toolchainX86_64) Supported() bool {
	return true
}

func (toolchainX86_64) LibclangRuntimeLibraryArch() string {
	return "x86_64"
}

func x86_64ToolchainFactory(arch android.Arch) Toolchain {
	toolchainRustFlags := []string{
		"${config.X86_64ToolchainRustFlags}",
		"${config.X86_64" + arch.ArchVariant + "VariantRustFlags}",
	}

	toolchainRustFlags = append(toolchainRustFlags, deviceGlobalRustFlags...)

	for _, feature := range arch.ArchFeatures {
		toolchainRustFlags = append(toolchainRustFlags, x86_64ArchFeatureRustFlags[feature]...)
	}

	return &toolchainX86_64{
		toolchainRustFlags: strings.Join(toolchainRustFlags, " "),
	}
}
