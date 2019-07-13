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
	x86Cflags = []string{}

	x86ClangCflags = append(x86Cflags, []string{
		"-msse3",

		// -mstackrealign is needed to realign stack in native code
		// that could be called from JNI, so that movaps instruction
		// will work on assumed stack aligned local variables.
		"-mstackrealign",
	}...)

	x86Cppflags = []string{}

	x86Ldflags = []string{
		"-Wl,--hash-style=gnu",
	}

	x86Lldflags = ClangFilterUnknownLldflags(x86Ldflags)

	x86ArchVariantCflags = map[string][]string{
		"": []string{
			"-march=prescott",
		},
		"x86_64": []string{
			"-march=prescott",
		},
		"atom": []string{
			"-march=atom",
			"-mfpmath=sse",
		},
		"broadwell": []string{
			"-march=broadwell",
			"-mfpmath=sse",
		},
		"haswell": []string{
			"-march=core-avx2",
			"-mfpmath=sse",
		},
		"ivybridge": []string{
			"-march=core-avx-i",
			"-mfpmath=sse",
		},
		"sandybridge": []string{
			"-march=corei7",
			"-mfpmath=sse",
		},
		"silvermont": []string{
			"-march=slm",
			"-mfpmath=sse",
		},
		"skylake": []string{
			"-march=skylake",
			"-mfpmath=sse",
		},
		"stoneyridge": []string{
			"-march=bdver4",
			"-mfpmath=sse",
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

const (
	x86GccVersion = "4.9"
)

func init() {
	pctx.StaticVariable("x86GccVersion", x86GccVersion)

	pctx.SourcePathVariable("X86GccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/x86/x86_64-linux-android-${x86GccVersion}")

	pctx.StaticVariable("X86ToolchainCflags", "-m32")
	pctx.StaticVariable("X86ToolchainLdflags", "-m32")

	pctx.StaticVariable("X86Ldflags", strings.Join(x86Ldflags, " "))
	pctx.StaticVariable("X86Lldflags", strings.Join(x86Lldflags, " "))
	pctx.StaticVariable("X86IncludeFlags", bionicHeaders("x86"))

	// Clang cflags
	pctx.StaticVariable("X86ClangCflags", strings.Join(ClangFilterUnknownCflags(x86ClangCflags), " "))
	pctx.StaticVariable("X86ClangLdflags", strings.Join(ClangFilterUnknownCflags(x86Ldflags), " "))
	pctx.StaticVariable("X86ClangLldflags", strings.Join(ClangFilterUnknownCflags(x86Lldflags), " "))
	pctx.StaticVariable("X86ClangCppflags", strings.Join(ClangFilterUnknownCflags(x86Cppflags), " "))

	// Yasm flags
	pctx.StaticVariable("X86YasmFlags", "-f elf32 -m x86")

	// Extended cflags

	// Architecture variant cflags
	for variant, cflags := range x86ArchVariantCflags {
		pctx.StaticVariable("X86"+variant+"VariantClangCflags",
			strings.Join(ClangFilterUnknownCflags(cflags), " "))
	}
}

type toolchainX86 struct {
	toolchain32Bit
	toolchainClangCflags string
}

func (t *toolchainX86) Name() string {
	return "x86"
}

func (t *toolchainX86) GccRoot() string {
	return "${config.X86GccRoot}"
}

func (t *toolchainX86) GccTriple() string {
	return "x86_64-linux-android"
}

func (t *toolchainX86) GccVersion() string {
	return x86GccVersion
}

func (t *toolchainX86) IncludeFlags() string {
	return "${config.X86IncludeFlags}"
}

func (t *toolchainX86) ClangTriple() string {
	return "i686-linux-android"
}

func (t *toolchainX86) ToolchainClangLdflags() string {
	return "${config.X86ToolchainLdflags}"
}

func (t *toolchainX86) ToolchainClangCflags() string {
	return t.toolchainClangCflags
}

func (t *toolchainX86) ClangCflags() string {
	return "${config.X86ClangCflags}"
}

func (t *toolchainX86) ClangCppflags() string {
	return "${config.X86ClangCppflags}"
}

func (t *toolchainX86) ClangLdflags() string {
	return "${config.X86Ldflags}"
}

func (t *toolchainX86) ClangLldflags() string {
	return "${config.X86Lldflags}"
}

func (t *toolchainX86) YasmFlags() string {
	return "${config.X86YasmFlags}"
}

func (toolchainX86) LibclangRuntimeLibraryArch() string {
	return "i686"
}

func x86ToolchainFactory(arch android.Arch) Toolchain {
	toolchainClangCflags := []string{
		"${config.X86ToolchainCflags}",
		"${config.X86" + arch.ArchVariant + "VariantClangCflags}",
	}

	for _, feature := range arch.ArchFeatures {
		toolchainClangCflags = append(toolchainClangCflags, x86ArchFeatureCflags[feature]...)
	}

	return &toolchainX86{
		toolchainClangCflags: strings.Join(toolchainClangCflags, " "),
	}
}

func init() {
	registerToolchainFactory(android.Android, android.X86, x86ToolchainFactory)
}
