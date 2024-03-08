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
	x86_64Cflags = []string{
		// Help catch common 32/64-bit errors.
		"-Werror=implicit-function-declaration",
	}

	x86_64Cppflags = []string{}

	x86_64Ldflags = []string{
		"-Wl,--hash-style=gnu",
	}

	X86_64Lldflags = x86_64Ldflags

	x86_64ArchVariantCflags = map[string][]string{
		"": []string{
			"-march=x86-64",
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

	x86_64ArchFeatureCflags = map[string][]string{
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

		"popcnt": []string{"-mpopcnt"},
		"aes_ni": []string{"-maes"},
	}
)

func init() {
	exportedVars.ExportStringListStaticVariable("X86_64ToolchainCflags", []string{"-m64"})
	exportedVars.ExportStringListStaticVariable("X86_64ToolchainLdflags", []string{"-m64"})

	exportedVars.ExportStringListStaticVariable("X86_64Ldflags", x86_64Ldflags)
	exportedVars.ExportStringList("X86_64Lldflags", X86_64Lldflags)
	pctx.VariableFunc("X86_64Lldflags", func(ctx android.PackageVarContext) string {
		maxPageSizeFlag := "-Wl,-z,max-page-size=" + ctx.Config().MaxPageSizeSupported()
		flags := append(X86_64Lldflags, maxPageSizeFlag)
		return strings.Join(flags, " ")
	})

	// Clang cflags
	exportedVars.ExportStringList("X86_64Cflags", x86_64Cflags)
	pctx.VariableFunc("X86_64Cflags", func(ctx android.PackageVarContext) string {
		flags := x86_64Cflags
		if ctx.Config().NoBionicPageSizeMacro() {
			flags = append(flags, "-D__BIONIC_NO_PAGE_SIZE_MACRO")
		}
		return strings.Join(flags, " ")
	})

	exportedVars.ExportStringListStaticVariable("X86_64Cppflags", x86_64Cppflags)

	// Yasm flags
	exportedVars.ExportStringListStaticVariable("X86_64YasmFlags", []string{
		"-f elf64",
		"-m amd64",
	})

	// Extended cflags

	exportedVars.ExportStringListDict("X86_64ArchVariantCflags", x86_64ArchVariantCflags)
	exportedVars.ExportStringListDict("X86_64ArchFeatureCflags", x86_64ArchFeatureCflags)

	// Architecture variant cflags
	for variant, cflags := range x86_64ArchVariantCflags {
		pctx.StaticVariable("X86_64"+variant+"VariantCflags", strings.Join(cflags, " "))
	}
}

type toolchainX86_64 struct {
	toolchainBionic
	toolchain64Bit
	toolchainCflags string
}

func (t *toolchainX86_64) Name() string {
	return "x86_64"
}

func (t *toolchainX86_64) IncludeFlags() string {
	return ""
}

func (t *toolchainX86_64) ClangTriple() string {
	return "x86_64-linux-android"
}

func (t *toolchainX86_64) ToolchainLdflags() string {
	return "${config.X86_64ToolchainLdflags}"
}

func (t *toolchainX86_64) ToolchainCflags() string {
	return t.toolchainCflags
}

func (t *toolchainX86_64) Cflags() string {
	return "${config.X86_64Cflags}"
}

func (t *toolchainX86_64) Cppflags() string {
	return "${config.X86_64Cppflags}"
}

func (t *toolchainX86_64) Ldflags() string {
	return "${config.X86_64Ldflags}"
}

func (t *toolchainX86_64) Lldflags() string {
	return "${config.X86_64Lldflags}"
}

func (t *toolchainX86_64) YasmFlags() string {
	return "${config.X86_64YasmFlags}"
}

func (toolchainX86_64) LibclangRuntimeLibraryArch() string {
	return "x86_64"
}

func x86_64ToolchainFactory(arch android.Arch) Toolchain {
	// Error now rather than having a confusing Ninja error
	if _, ok := x86_64ArchVariantCflags[arch.ArchVariant]; !ok {
		panic(fmt.Sprintf("Unknown x86_64 architecture version: %q", arch.ArchVariant))
	}

	toolchainCflags := []string{
		"${config.X86_64ToolchainCflags}",
		"${config.X86_64" + arch.ArchVariant + "VariantCflags}",
	}

	for _, feature := range arch.ArchFeatures {
		toolchainCflags = append(toolchainCflags, x86_64ArchFeatureCflags[feature]...)
	}

	return &toolchainX86_64{
		toolchainCflags: strings.Join(toolchainCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.X86_64, x86_64ToolchainFactory)
}
