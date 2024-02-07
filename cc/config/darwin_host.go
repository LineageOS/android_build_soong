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

package config

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"android/soong/android"
)

var (
	darwinCflags = []string{
		"-fPIC",
		"-funwind-tables",

		"-isysroot ${macSdkRoot}",
		"-mmacosx-version-min=${macMinVersion}",
		"-DMACOSX_DEPLOYMENT_TARGET=${macMinVersion}",

		"-m64",

		"-integrated-as",
		"-fstack-protector-strong",
	}

	darwinLdflags = []string{
		"-isysroot ${macSdkRoot}",
		"-Wl,-syslibroot,${macSdkRoot}",
		"-mmacosx-version-min=${macMinVersion}",
		"-m64",
		"-mlinker-version=305",
	}

	darwinSupportedSdkVersions = []string{
		"11",
		"12",
	}

	darwinAvailableLibraries = append(
		addPrefix([]string{
			"c",
			"dl",
			"m",
			"ncurses",
			"objc",
			"pthread",
		}, "-l"),
		"-framework AppKit",
		"-framework CoreFoundation",
		"-framework Foundation",
		"-framework IOKit",
		"-framework Security",
		"-framework SystemConfiguration",
	)
)

func init() {
	pctx.VariableFunc("macSdkRoot", func(ctx android.PackageVarContext) string {
		return getMacTools(ctx).sdkRoot
	})
	pctx.StaticVariable("macMinVersion", "10.14")
	pctx.VariableFunc("MacArPath", func(ctx android.PackageVarContext) string {
		return getMacTools(ctx).arPath
	})

	pctx.VariableFunc("MacLipoPath", func(ctx android.PackageVarContext) string {
		return getMacTools(ctx).lipoPath
	})

	pctx.VariableFunc("MacStripPath", func(ctx android.PackageVarContext) string {
		return getMacTools(ctx).stripPath
	})

	pctx.VariableFunc("MacToolPath", func(ctx android.PackageVarContext) string {
		return getMacTools(ctx).toolPath
	})

	pctx.StaticVariable("DarwinCflags", strings.Join(darwinCflags, " "))
	pctx.StaticVariable("DarwinLdflags", strings.Join(darwinLdflags, " "))
	pctx.StaticVariable("DarwinLldflags", strings.Join(darwinLdflags, " "))

	pctx.StaticVariable("DarwinYasmFlags", "-f macho -m amd64")
}

func MacStripPath(ctx android.PathContext) string {
	return getMacTools(ctx).stripPath
}

type macPlatformTools struct {
	once sync.Once
	err  error

	sdkRoot   string
	arPath    string
	lipoPath  string
	stripPath string
	toolPath  string
}

var macTools = &macPlatformTools{}

func getMacTools(ctx android.PathContext) *macPlatformTools {
	macTools.once.Do(func() {
		xcrunTool := "/usr/bin/xcrun"

		xcrun := func(args ...string) string {
			if macTools.err != nil {
				return ""
			}

			bytes, err := exec.Command(xcrunTool, append([]string{"--sdk", "macosx"}, args...)...).Output()
			if err != nil {
				macTools.err = fmt.Errorf("xcrun %q failed with: %q", args, err)
				return ""
			}

			return strings.TrimSpace(string(bytes))
		}

		sdkVersion := xcrun("--show-sdk-version")
		sdkVersionSupported := false
		for _, version := range darwinSupportedSdkVersions {
			if version == sdkVersion || strings.HasPrefix(sdkVersion, version+".") {
				sdkVersionSupported = true
			}
		}
		if !sdkVersionSupported {
			macTools.err = fmt.Errorf("Unsupported macOS SDK version %q not in %v", sdkVersion, darwinSupportedSdkVersions)
			return
		}

		macTools.sdkRoot = xcrun("--show-sdk-path")

		macTools.arPath = xcrun("--find", "ar")
		macTools.lipoPath = xcrun("--find", "lipo")
		macTools.stripPath = xcrun("--find", "strip")
		macTools.toolPath = filepath.Dir(xcrun("--find", "ld"))
	})
	if macTools.err != nil {
		android.ReportPathErrorf(ctx, "%q", macTools.err)
	}
	return macTools
}

type toolchainDarwin struct {
	cFlags, ldFlags string
	toolchain64Bit
	toolchainNoCrt
	toolchainBase
}

type toolchainDarwinX86 struct {
	toolchainDarwin
}

type toolchainDarwinArm struct {
	toolchainDarwin
}

func (t *toolchainDarwinArm) Name() string {
	return "arm64"
}

func (t *toolchainDarwinX86) Name() string {
	return "x86_64"
}

func (t *toolchainDarwin) IncludeFlags() string {
	return ""
}

func (t *toolchainDarwinArm) ClangTriple() string {
	return "aarch64-apple-darwin"
}

func (t *toolchainDarwinX86) ClangTriple() string {
	return "x86_64-apple-darwin"
}

func (t *toolchainDarwin) Cflags() string {
	return "${config.DarwinCflags}"
}

func (t *toolchainDarwin) Cppflags() string {
	return ""
}

func (t *toolchainDarwin) Ldflags() string {
	return "${config.DarwinLdflags}"
}

func (t *toolchainDarwin) Lldflags() string {
	return "${config.DarwinLldflags}"
}

func (t *toolchainDarwin) YasmFlags() string {
	return "${config.DarwinYasmFlags}"
}

func (t *toolchainDarwin) ShlibSuffix() string {
	return ".dylib"
}

func (t *toolchainDarwin) ExecutableSuffix() string {
	return ""
}

func (t *toolchainDarwin) AvailableLibraries() []string {
	return darwinAvailableLibraries
}

func (t *toolchainDarwin) ToolchainCflags() string {
	return "-B${config.MacToolPath}"
}

func (t *toolchainDarwin) ToolchainLdflags() string {
	return "-B${config.MacToolPath}"
}

var toolchainDarwinArmSingleton Toolchain = &toolchainDarwinArm{}
var toolchainDarwinX86Singleton Toolchain = &toolchainDarwinX86{}

func darwinArmToolchainFactory(arch android.Arch) Toolchain {
	return toolchainDarwinArmSingleton
}

func darwinX86ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainDarwinX86Singleton
}

func init() {
	registerToolchainFactory(android.Darwin, android.Arm64, darwinArmToolchainFactory)
	registerToolchainFactory(android.Darwin, android.X86_64, darwinX86ToolchainFactory)
}
