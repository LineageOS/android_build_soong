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
	mipsCflags = []string{
		"-fomit-frame-pointer",
		"-Umips",
	}

	mipsClangCflags = append(mipsCflags, []string{
		"-fPIC",
		"-fintegrated-as",
	}...)

	mipsCppflags = []string{}

	mipsLdflags = []string{
		"-Wl,--allow-shlib-undefined",
	}

	mipsToolchainLdflags = []string{
		"-Wl,-melf32ltsmip",
	}

	mipsArchVariantCflags = map[string][]string{
		"mips32-fp": []string{
			"-mips32",
			"-mfp32",
			"-modd-spreg",
			"-mno-synci",
		},
		"mips32r2-fp": []string{
			"-mips32r2",
			"-mfp32",
			"-modd-spreg",
			"-msynci",
		},
		"mips32r2-fp-xburst": []string{
			"-mips32r2",
			"-mfp32",
			"-modd-spreg",
			"-mno-fused-madd",
			"-mno-synci",
		},
		"mips32r2dsp-fp": []string{
			"-mips32r2",
			"-mfp32",
			"-modd-spreg",
			"-mdsp",
			"-msynci",
		},
		"mips32r2dspr2-fp": []string{
			"-mips32r2",
			"-mfp32",
			"-modd-spreg",
			"-mdspr2",
			"-msynci",
		},
		"mips32r6": []string{
			"-mips32r6",
			"-mfp64",
			"-mno-odd-spreg",
			"-msynci",
		},
	}
)

const (
	mipsGccVersion = "4.9"
)

func init() {
	pctx.StaticVariable("mipsGccVersion", mipsGccVersion)

	pctx.SourcePathVariable("MipsGccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/mips/mips64el-linux-android-${mipsGccVersion}")

	pctx.StaticVariable("MipsToolchainLdflags", strings.Join(mipsToolchainLdflags, " "))
	pctx.StaticVariable("MipsIncludeFlags", bionicHeaders("mips"))

	// Clang cflags
	pctx.StaticVariable("MipsClangCflags", strings.Join(ClangFilterUnknownCflags(mipsClangCflags), " "))
	pctx.StaticVariable("MipsClangLdflags", strings.Join(ClangFilterUnknownCflags(mipsLdflags), " "))
	pctx.StaticVariable("MipsClangCppflags", strings.Join(ClangFilterUnknownCflags(mipsCppflags), " "))

	// Extended cflags

	// Architecture variant cflags
	for variant, cflags := range mipsArchVariantCflags {
		pctx.StaticVariable("Mips"+variant+"VariantClangCflags",
			strings.Join(ClangFilterUnknownCflags(cflags), " "))
	}
}

type toolchainMips struct {
	toolchain32Bit
	clangCflags          string
	toolchainClangCflags string
}

func (t *toolchainMips) Name() string {
	return "mips"
}

func (t *toolchainMips) GccRoot() string {
	return "${config.MipsGccRoot}"
}

func (t *toolchainMips) GccTriple() string {
	return "mips64el-linux-android"
}

func (t *toolchainMips) GccVersion() string {
	return mipsGccVersion
}

func (t *toolchainMips) IncludeFlags() string {
	return "${config.MipsIncludeFlags}"
}

func (t *toolchainMips) ClangTriple() string {
	return "mipsel-linux-android"
}

func (t *toolchainMips) ToolchainClangLdflags() string {
	return "${config.MipsToolchainLdflags}"
}

func (t *toolchainMips) ToolchainClangCflags() string {
	return t.toolchainClangCflags
}

func (t *toolchainMips) ClangAsflags() string {
	return "-fPIC -fno-integrated-as"
}

func (t *toolchainMips) ClangCflags() string {
	return t.clangCflags
}

func (t *toolchainMips) ClangCppflags() string {
	return "${config.MipsClangCppflags}"
}

func (t *toolchainMips) ClangLdflags() string {
	return "${config.MipsClangLdflags}"
}

func (t *toolchainMips) ClangLldflags() string {
	// TODO: define and use MipsClangLldflags
	return "${config.MipsClangLdflags}"
}

func (toolchainMips) LibclangRuntimeLibraryArch() string {
	return "mips"
}

func mipsToolchainFactory(arch android.Arch) Toolchain {
	return &toolchainMips{
		clangCflags:          "${config.MipsClangCflags}",
		toolchainClangCflags: "${config.Mips" + arch.ArchVariant + "VariantClangCflags}",
	}
}

func init() {
	registerToolchainFactory(android.Android, android.Mips, mipsToolchainFactory)
}
