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
	_ "android/soong/cc/config"
)

var pctx = android.NewPackageContext("android/soong/rust/config")

var (
	RustDefaultVersion = "1.49.0"
	RustDefaultBase    = "prebuilts/rust/"
	DefaultEdition     = "2018"
	Stdlibs            = []string{
		"libstd",
		"libtest",
	}

	// Mapping between Soong internal arch types and std::env constants.
	// Required as Rust uses aarch64 when Soong uses arm64.
	StdEnvArch = map[android.ArchType]string{
		android.Arm:    "arm",
		android.Arm64:  "aarch64",
		android.X86:    "x86",
		android.X86_64: "x86_64",
	}

	GlobalRustFlags = []string{
		"--remap-path-prefix $$(pwd)=",
		"-C codegen-units=1",
		"-C debuginfo=2",
		"-C opt-level=3",
		"-C relocation-model=pic",
	}

	deviceGlobalRustFlags = []string{
		"-C panic=abort",
	}

	deviceGlobalLinkFlags = []string{
		// Prepend the lld flags from cc_config so we stay in sync with cc
		"${cc_config.DeviceGlobalLldflags}",

		// Override cc's --no-undefined-version to allow rustc's generated alloc functions
		"-Wl,--undefined-version",

		"-Bdynamic",
		"-nostdlib",
		"-Wl,--pack-dyn-relocs=android+relr",
		"-Wl,--use-android-relr-tags",
		"-Wl,--no-undefined",
		"-B${cc_config.ClangBin}",
	}
)

func init() {
	pctx.SourcePathVariable("RustDefaultBase", RustDefaultBase)
	pctx.VariableConfigMethod("HostPrebuiltTag", android.Config.PrebuiltOS)

	pctx.VariableFunc("RustBase", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("RUST_PREBUILTS_BASE"); override != "" {
			return override
		}
		return "${RustDefaultBase}"
	})

	pctx.VariableFunc("RustVersion", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("RUST_PREBUILTS_VERSION"); override != "" {
			return override
		}
		return RustDefaultVersion
	})

	pctx.StaticVariable("RustPath", "${RustBase}/${HostPrebuiltTag}/${RustVersion}")
	pctx.StaticVariable("RustBin", "${RustPath}/bin")

	pctx.ImportAs("cc_config", "android/soong/cc/config")
	pctx.StaticVariable("RustLinker", "${cc_config.ClangBin}/clang++")
	pctx.StaticVariable("RustLinkerArgs", "")

	pctx.StaticVariable("DeviceGlobalLinkFlags", strings.Join(deviceGlobalLinkFlags, " "))

}
