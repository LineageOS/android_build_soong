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
	x86RustFlags            = []string{}
	x86ArchFeatureRustFlags = map[string][]string{}
	x86LinkFlags            = []string{}

	x86ArchVariantRustFlags = map[string][]string{
		"":              []string{},
		"atom":          []string{"-C target-cpu=atom"},
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
		// use prescott for x86_64, like cc/config/x86_device.go
		"x86_64": []string{"-C target-cpu=prescott"},
	}
)

func init() {
	registerToolchainFactory(android.Android, android.X86, x86ToolchainFactory)

	pctx.StaticVariable("X86ToolchainRustFlags", strings.Join(x86RustFlags, " "))
	pctx.StaticVariable("X86ToolchainLinkFlags", strings.Join(x86LinkFlags, " "))

	for variant, rustFlags := range x86ArchVariantRustFlags {
		pctx.StaticVariable("X86"+variant+"VariantRustFlags",
			strings.Join(rustFlags, " "))
	}

	ExportedVars.ExportStringListStaticVariable("DEVICE_X86_RUSTC_FLAGS", x86RustFlags)
}

type toolchainX86 struct {
	toolchain32Bit
	toolchainRustFlags string
}

func (t *toolchainX86) RustTriple() string {
	return "i686-linux-android"
}

func (t *toolchainX86) ToolchainLinkFlags() string {
	// Prepend the lld flags from cc_config so we stay in sync with cc
	return "${config.DeviceGlobalLinkFlags} ${cc_config.X86Lldflags} ${config.X86ToolchainLinkFlags}"
}

func (t *toolchainX86) ToolchainRustFlags() string {
	return t.toolchainRustFlags
}

func (t *toolchainX86) RustFlags() string {
	return "${config.X86ToolchainRustFlags}"
}

func (t *toolchainX86) Supported() bool {
	return true
}

func (toolchainX86) LibclangRuntimeLibraryArch() string {
	return "i686"
}

func x86ToolchainFactory(arch android.Arch) Toolchain {
	toolchainRustFlags := []string{
		"${config.X86ToolchainRustFlags}",
		"${config.X86" + arch.ArchVariant + "VariantRustFlags}",
	}

	toolchainRustFlags = append(toolchainRustFlags, deviceGlobalRustFlags...)

	for _, feature := range arch.ArchFeatures {
		toolchainRustFlags = append(toolchainRustFlags, x86ArchFeatureRustFlags[feature]...)
	}

	return &toolchainX86{
		toolchainRustFlags: strings.Join(toolchainRustFlags, " "),
	}
}
