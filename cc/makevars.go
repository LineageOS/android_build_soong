// Copyright 2016 Google Inc. All rights reserved.
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

package cc

import (
	"fmt"
	"path/filepath"
	"strings"

	"android/soong/android"
)

func init() {
	android.RegisterMakeVarsProvider(pctx, makeVarsProvider)
}

func makeVarsProvider(ctx android.MakeVarsContext) {
	ctx.Strict("LLVM_PREBUILTS_VERSION", "${clangVersion}")
	ctx.Strict("LLVM_PREBUILTS_BASE", "${clangBase}")
	ctx.Strict("LLVM_PREBUILTS_PATH", "${clangBin}")
	ctx.Strict("CLANG", "${clangBin}/clang")
	ctx.Strict("CLANG_CXX", "${clangBin}/clang++")
	ctx.Strict("LLVM_AS", "${clangBin}/llvm-as")
	ctx.Strict("LLVM_LINK", "${clangBin}/llvm-link")

	hostType := android.CurrentHostType()
	arches := ctx.Config().HostArches[hostType]
	makeVarsToolchain(ctx, "", android.Host, hostType, arches[0])
	if len(arches) > 1 {
		makeVarsToolchain(ctx, "2ND_", android.Host, hostType, arches[1])
	}

	if winArches, ok := ctx.Config().HostArches[android.Windows]; ok {
		makeVarsToolchain(ctx, "", android.Host, android.Windows, winArches[0])
		if len(winArches) > 1 {
			makeVarsToolchain(ctx, "2ND_", android.Host, android.Windows, winArches[1])
		}
	}

	arches = ctx.Config().DeviceArches
	makeVarsToolchain(ctx, "", android.Device, android.NoHostType, arches[0])
	if len(arches) > 1 {
		makeVarsToolchain(ctx, "2ND_", android.Device, android.NoHostType, arches[1])
	}
}

func makeVarsToolchain(ctx android.MakeVarsContext, secondPrefix string,
	hod android.HostOrDevice, ht android.HostType, arch android.Arch) {
	var typePrefix string
	if hod.Host() {
		if ht == android.Windows {
			typePrefix = "HOST_CROSS_"
		} else {
			typePrefix = "HOST_"
		}
	} else {
		typePrefix = "TARGET_"
	}
	makePrefix := secondPrefix + typePrefix

	toolchain := toolchainFactories[hod][ht][arch.ArchType](arch)

	var productExtraCflags string
	var productExtraLdflags string
	if hod.Device() && Bool(ctx.Config().ProductVariables.Brillo) {
		productExtraCflags += "-D__BRILLO__"
	}
	if hod.Host() && Bool(ctx.Config().ProductVariables.HostStaticBinaries) {
		productExtraLdflags += "-static"
	}

	ctx.StrictSorted(makePrefix+"GLOBAL_CFLAGS", strings.Join([]string{
		toolchain.ToolchainCflags(),
		"${commonGlobalCflags}",
		fmt.Sprintf("${%sGlobalCflags}", hod),
		productExtraCflags,
		toolchain.Cflags(),
	}, " "))
	ctx.StrictSorted(makePrefix+"GLOBAL_CONLYFLAGS", "")
	ctx.StrictSorted(makePrefix+"GLOBAL_CPPFLAGS", strings.Join([]string{
		"${commonGlobalCppflags}",
		toolchain.Cppflags(),
	}, " "))
	ctx.StrictSorted(makePrefix+"GLOBAL_LDFLAGS", strings.Join([]string{
		toolchain.ToolchainLdflags(),
		productExtraLdflags,
		toolchain.Ldflags(),
	}, " "))

	if toolchain.ClangSupported() {
		clangPrefix := secondPrefix + "CLANG_" + typePrefix
		clangExtras := "-target " + toolchain.ClangTriple()
		if ht != android.Darwin {
			clangExtras += " -B" + filepath.Join(toolchain.GccRoot(), toolchain.GccTriple(), "bin")
		}

		ctx.StrictSorted(clangPrefix+"GLOBAL_CFLAGS", strings.Join([]string{
			toolchain.ToolchainClangCflags(),
			"${commonClangGlobalCflags}",
			"${clangExtraCflags}",
			fmt.Sprintf("${%sClangGlobalCflags}", hod),
			productExtraCflags,
			toolchain.ClangCflags(),
			clangExtras,
		}, " "))
		ctx.StrictSorted(clangPrefix+"GLOBAL_CONLYFLAGS", "${clangExtraConlyflags}")
		ctx.StrictSorted(clangPrefix+"GLOBAL_CPPFLAGS", strings.Join([]string{
			"${commonClangGlobalCppflags}",
			toolchain.ClangCppflags(),
		}, " "))
		ctx.StrictSorted(clangPrefix+"GLOBAL_LDFLAGS", strings.Join([]string{
			toolchain.ToolchainClangLdflags(),
			productExtraLdflags,
			toolchain.ClangLdflags(),
			clangExtras,
		}, " "))
	}

	ctx.Strict(makePrefix+"CC", gccCmd(toolchain, "gcc"))
	ctx.Strict(makePrefix+"CXX", gccCmd(toolchain, "g++"))

	if ht == android.Darwin {
		ctx.Strict(makePrefix+"AR", "${macArPath}")
	} else {
		ctx.Strict(makePrefix+"AR", gccCmd(toolchain, "ar"))
		ctx.Strict(makePrefix+"READELF", gccCmd(toolchain, "readelf"))
		ctx.Strict(makePrefix+"NM", gccCmd(toolchain, "nm"))
	}

	if ht == android.Windows {
		ctx.Strict(makePrefix+"OBJDUMP", gccCmd(toolchain, "objdump"))
	}

	if hod.Device() {
		ctx.Strict(makePrefix+"OBJCOPY", gccCmd(toolchain, "objcopy"))
		ctx.Strict(makePrefix+"LD", gccCmd(toolchain, "ld"))
		ctx.Strict(makePrefix+"STRIP", gccCmd(toolchain, "strip"))
	}

	ctx.Strict(makePrefix+"TOOLS_PREFIX", gccCmd(toolchain, ""))
}
