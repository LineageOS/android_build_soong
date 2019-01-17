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
	mips64Cflags = []string{
		"-Umips",

		// Help catch common 32/64-bit errors.
		"-Werror=implicit-function-declaration",
	}

	mips64ClangCflags = append(mips64Cflags, []string{
		"-fintegrated-as",
	}...)

	mips64Cppflags = []string{}

	mips64Ldflags = []string{
		"-Wl,--allow-shlib-undefined",
	}

	mips64ArchVariantCflags = map[string][]string{
		"mips64r2": []string{
			"-mips64r2",
			"-msynci",
		},
		"mips64r6": []string{
			"-mips64r6",
			"-msynci",
		},
	}
)

const (
	mips64GccVersion = "4.9"
)

func init() {
	pctx.StaticVariable("mips64GccVersion", mips64GccVersion)

	pctx.SourcePathVariable("Mips64GccRoot",
		"prebuilts/gcc/${HostPrebuiltTag}/mips/mips64el-linux-android-${mips64GccVersion}")

	pctx.StaticVariable("Mips64IncludeFlags", bionicHeaders("mips"))

	// Clang cflags
	pctx.StaticVariable("Mips64ClangCflags", strings.Join(ClangFilterUnknownCflags(mips64ClangCflags), " "))
	pctx.StaticVariable("Mips64ClangLdflags", strings.Join(ClangFilterUnknownCflags(mips64Ldflags), " "))
	pctx.StaticVariable("Mips64ClangCppflags", strings.Join(ClangFilterUnknownCflags(mips64Cppflags), " "))

	// Extended cflags

	// Architecture variant cflags
	for variant, cflags := range mips64ArchVariantCflags {
		pctx.StaticVariable("Mips64"+variant+"VariantClangCflags",
			strings.Join(ClangFilterUnknownCflags(cflags), " "))
	}
}

type toolchainMips64 struct {
	toolchain64Bit
	clangCflags          string
	toolchainClangCflags string
}

func (t *toolchainMips64) Name() string {
	return "mips64"
}

func (t *toolchainMips64) GccRoot() string {
	return "${config.Mips64GccRoot}"
}

func (t *toolchainMips64) GccTriple() string {
	return "mips64el-linux-android"
}

func (t *toolchainMips64) GccVersion() string {
	return mips64GccVersion
}

func (t *toolchainMips64) IncludeFlags() string {
	return "${config.Mips64IncludeFlags}"
}

func (t *toolchainMips64) ClangTriple() string {
	return t.GccTriple()
}

func (t *toolchainMips64) ToolchainClangCflags() string {
	return t.toolchainClangCflags
}

func (t *toolchainMips64) ClangAsflags() string {
	return "-fno-integrated-as"
}

func (t *toolchainMips64) ClangCflags() string {
	return t.clangCflags
}

func (t *toolchainMips64) ClangCppflags() string {
	return "${config.Mips64ClangCppflags}"
}

func (t *toolchainMips64) ClangLdflags() string {
	return "${config.Mips64ClangLdflags}"
}

func (t *toolchainMips64) ClangLldflags() string {
	// TODO: define and use Mips64ClangLldflags
	return "${config.Mips64ClangLdflags}"
}

func (toolchainMips64) LibclangRuntimeLibraryArch() string {
	return "mips64"
}

func mips64ToolchainFactory(arch android.Arch) Toolchain {
	return &toolchainMips64{
		clangCflags:          "${config.Mips64ClangCflags}",
		toolchainClangCflags: "${config.Mips64" + arch.ArchVariant + "VariantClangCflags}",
	}
}

func init() {
	registerToolchainFactory(android.Android, android.Mips64, mips64ToolchainFactory)
}
