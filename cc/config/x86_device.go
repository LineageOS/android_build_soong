// Copyright 2015 Google Inc. All rights reserved.
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
	"fmt"
	"strings"

	"android/soong/android"
)

var (
	x86Cflags = []string{
		"-msse3",

		// -mstackrealign is needed to realign stack in native code
		// that could be called from JNI, so that movaps instruction
		// will work on assumed stack aligned local variables.
		"-mstackrealign",
	}

	x86Cppflags = []string{}

	x86Ldflags = []string{
		"-Wl,--hash-style=gnu",
	}

	x86ArchVariantCflags = map[string][]string{
		"": []string{
			"-march=prescott",
		},
		"x86_64": []string{
			"-march=prescott",
		},
		"atom": []string{
			"-march=atom",
		},
		"broadwell": []string{
			"-march=broadwell",
		},
		"goldmont": []string{
			"-march=goldmont",
		},
		"goldmont-plus": []string{
			"-march=goldmont-plus",
		},
		"goldmont-without-sha-xsaves": []string{
			"-march=goldmont",
			"-mno-sha",
			"-mno-xsaves",
		},
		"haswell": []string{
			"-march=core-avx2",
		},
		"ivybridge": []string{
			"-march=core-avx-i",
		},
		"sandybridge": []string{
			"-march=corei7",
		},
		"silvermont": []string{
			"-march=slm",
		},
		"skylake": []string{
			"-march=skylake",
		},
		"stoneyridge": []string{
			"-march=bdver4",
		},
		"tremont": []string{
			"-march=tremont",
		},
	}

	x86ArchFeatureCflags = map[string][]string{
		"ssse3":  []string{"-mssse3"},
		"sse4":   []string{"-msse4"},
		"sse4_1": []string{"-msse4.1"},
		"sse4_2": []string{"-msse4.2"},

		// Not all cases there is performance gain by enabling -mavx -mavx2
		// flags so these flags are not enabled by default.
		// if there is performance gain in individual library components,
		// the compiler flags can be set in corresponding bp files.
		// "avx":    []string{"-mavx"},
		// "avx2":   []string{"-mavx2"},
		// "avx512": []string{"-mavx512"}

		"aes_ni": []string{"-maes"},
	}
)

func init() {
	exportedVars.ExportStringListStaticVariable("X86ToolchainCflags", []string{"-m32"})
	exportedVars.ExportStringListStaticVariable("X86ToolchainLdflags", []string{"-m32"})

	exportedVars.ExportStringListStaticVariable("X86Ldflags", x86Ldflags)
	exportedVars.ExportStringListStaticVariable("X86Lldflags", x86Ldflags)

	// Clang cflags
	exportedVars.ExportStringListStaticVariable("X86Cflags", x86Cflags)
	exportedVars.ExportStringListStaticVariable("X86Cppflags", x86Cppflags)

	// Yasm flags
	exportedVars.ExportStringListStaticVariable("X86YasmFlags", []string{
		"-f elf32",
		"-m x86",
	})

	// Extended cflags
	exportedVars.ExportStringListDict("X86ArchVariantCflags", x86ArchVariantCflags)
	exportedVars.ExportStringListDict("X86ArchFeatureCflags", x86ArchFeatureCflags)

	// Architecture variant cflags
	for variant, cflags := range x86ArchVariantCflags {
		pctx.StaticVariable("X86"+variant+"VariantCflags", strings.Join(cflags, " "))
	}
}

type toolchainX86 struct {
	toolchainBionic
	toolchain32Bit
	toolchainCflags string
}

func (t *toolchainX86) Name() string {
	return "x86"
}

func (t *toolchainX86) IncludeFlags() string {
	return ""
}

func (t *toolchainX86) ClangTriple() string {
	return "i686-linux-android"
}

func (t *toolchainX86) ToolchainLdflags() string {
	return "${config.X86ToolchainLdflags}"
}

func (t *toolchainX86) ToolchainCflags() string {
	return t.toolchainCflags
}

func (t *toolchainX86) Cflags() string {
	return "${config.X86Cflags}"
}

func (t *toolchainX86) Cppflags() string {
	return "${config.X86Cppflags}"
}

func (t *toolchainX86) Ldflags() string {
	return "${config.X86Ldflags}"
}

func (t *toolchainX86) Lldflags() string {
	return "${config.X86Lldflags}"
}

func (t *toolchainX86) YasmFlags() string {
	return "${config.X86YasmFlags}"
}

func (toolchainX86) LibclangRuntimeLibraryArch() string {
	return "i686"
}

func x86ToolchainFactory(arch android.Arch) Toolchain {
	// Error now rather than having a confusing Ninja error
	if _, ok := x86ArchVariantCflags[arch.ArchVariant]; !ok {
		panic(fmt.Sprintf("Unknown x86 architecture version: %q", arch.ArchVariant))
	}

	toolchainCflags := []string{
		"${config.X86ToolchainCflags}",
		"${config.X86" + arch.ArchVariant + "VariantCflags}",
	}

	for _, feature := range arch.ArchFeatures {
		toolchainCflags = append(toolchainCflags, x86ArchFeatureCflags[feature]...)
	}

	return &toolchainX86{
		toolchainCflags: strings.Join(toolchainCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.X86, x86ToolchainFactory)
}
