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

	x86_64Lldflags = ClangFilterUnknownLldflags(x86_64Ldflags)

	x86_64ArchVariantCflags = map[string][]string{
		"": []string{
			"-march=x86-64",
		},
		"broadwell": []string{
			"-march=broadwell",
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

const (
	x86_64GccVersion = "4.9"
)

func init() {
	android.RegisterDefaultArchVariantFeatures(android.Android, android.X86_64,
		"ssse3",
		"sse4",
		"sse4_1",
		"sse4_2",
		"popcnt")

	pctx.StaticVariable("x86_64GccVersion", x86_64GccVersion)

	pctx.SourcePathVariable("X86_64GccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/x86/x86_64-linux-android-${x86_64GccVersion}")

	pctx.StaticVariable("X86_64ToolchainCflags", "-m64")
	pctx.StaticVariable("X86_64ToolchainLdflags", "-m64")

	pctx.StaticVariable("X86_64Ldflags", strings.Join(x86_64Ldflags, " "))
	pctx.StaticVariable("X86_64Lldflags", strings.Join(x86_64Lldflags, " "))
	pctx.StaticVariable("X86_64IncludeFlags", bionicHeaders("x86"))

	// Clang cflags
	pctx.StaticVariable("X86_64ClangCflags", strings.Join(ClangFilterUnknownCflags(x86_64Cflags), " "))
	pctx.StaticVariable("X86_64ClangLdflags", strings.Join(ClangFilterUnknownCflags(x86_64Ldflags), " "))
	pctx.StaticVariable("X86_64ClangLldflags", strings.Join(ClangFilterUnknownCflags(x86_64Lldflags), " "))
	pctx.StaticVariable("X86_64ClangCppflags", strings.Join(ClangFilterUnknownCflags(x86_64Cppflags), " "))

	// Yasm flags
	pctx.StaticVariable("X86_64YasmFlags", "-f elf64 -m amd64")

	// Extended cflags

	// Architecture variant cflags
	for variant, cflags := range x86_64ArchVariantCflags {
		pctx.StaticVariable("X86_64"+variant+"VariantClangCflags",
			strings.Join(ClangFilterUnknownCflags(cflags), " "))
	}
}

type toolchainX86_64 struct {
	toolchain64Bit
	toolchainClangCflags string
}

func (t *toolchainX86_64) Name() string {
	return "x86_64"
}

func (t *toolchainX86_64) GccRoot() string {
	return "${config.X86_64GccRoot}"
}

func (t *toolchainX86_64) GccTriple() string {
	return "x86_64-linux-android"
}

func (t *toolchainX86_64) GccVersion() string {
	return x86_64GccVersion
}

func (t *toolchainX86_64) IncludeFlags() string {
	return "${config.X86_64IncludeFlags}"
}

func (t *toolchainX86_64) ClangTriple() string {
	return t.GccTriple()
}

func (t *toolchainX86_64) ToolchainClangLdflags() string {
	return "${config.X86_64ToolchainLdflags}"
}

func (t *toolchainX86_64) ToolchainClangCflags() string {
	return t.toolchainClangCflags
}

func (t *toolchainX86_64) ClangCflags() string {
	return "${config.X86_64ClangCflags}"
}

func (t *toolchainX86_64) ClangCppflags() string {
	return "${config.X86_64ClangCppflags}"
}

func (t *toolchainX86_64) ClangLdflags() string {
	return "${config.X86_64Ldflags}"
}

func (t *toolchainX86_64) ClangLldflags() string {
	return "${config.X86_64Lldflags}"
}

func (t *toolchainX86_64) YasmFlags() string {
	return "${config.X86_64YasmFlags}"
}

func (toolchainX86_64) LibclangRuntimeLibraryArch() string {
	return "x86_64"
}

func x86_64ToolchainFactory(arch android.Arch) Toolchain {
	toolchainClangCflags := []string{
		"${config.X86_64ToolchainCflags}",
		"${config.X86_64" + arch.ArchVariant + "VariantClangCflags}",
	}

	for _, feature := range arch.ArchFeatures {
		toolchainClangCflags = append(toolchainClangCflags, x86_64ArchFeatureCflags[feature]...)
	}

	return &toolchainX86_64{
		toolchainClangCflags: strings.Join(toolchainClangCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.X86_64, x86_64ToolchainFactory)
}
