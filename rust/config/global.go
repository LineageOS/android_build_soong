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
	RustDefaultVersion = "1.42.0"
	RustDefaultBase    = "prebuilts/rust/"
	DefaultEdition     = "2018"
	Stdlibs            = []string{
		"libstd",
		"libtest",
	}

	DefaultDenyWarnings = true

	GlobalRustFlags = []string{
		"--remap-path-prefix $$(pwd)=",
		"-C codegen-units=1",
		"-C opt-level=3",
		"-C relocation-model=pic",
	}

	deviceGlobalRustFlags = []string{}

	deviceGlobalLinkFlags = []string{
		"-Bdynamic",
		"-nostdlib",
		"-Wl,-z,noexecstack",
		"-Wl,-z,relro",
		"-Wl,-z,now",
		"-Wl,--build-id=md5",
		"-Wl,--warn-shared-textrel",
		"-Wl,--fatal-warnings",

		"-Wl,--pack-dyn-relocs=android+relr",
		"-Wl,--no-undefined",
		"-Wl,--hash-style=gnu",
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

	pctx.ImportAs("ccConfig", "android/soong/cc/config")
	pctx.StaticVariable("RustLinker", "${ccConfig.ClangBin}/clang++")
	pctx.StaticVariable("RustLinkerArgs", "-B ${ccConfig.ClangBin} -fuse-ld=lld")

	pctx.StaticVariable("DeviceGlobalLinkFlags", strings.Join(deviceGlobalLinkFlags, " "))

}
