// Copyright 2018 Google Inc. All rights reserved.
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
	"android/soong/android"
)

var fuchsiaArm64SysRoot string = "prebuilts/fuchsia_sdk/arch/arm64/sysroot"
var fuchsiaArm64PrebuiltLibsRoot string = "fuchsia/prebuilt_libs/"

type toolchainFuchsiaArm64 struct {
	toolchain64Bit
	toolchainFuchsia
}

func (t *toolchainFuchsiaArm64) Name() string {
	return "arm64"
}

func (t *toolchainFuchsiaArm64) GccRoot() string {
	return "${config.Arm64GccRoot}"
}

func (t *toolchainFuchsiaArm64) GccTriple() string {
	return "aarch64-linux-android"
}

func (t *toolchainFuchsiaArm64) GccVersion() string {
	return arm64GccVersion
}

func (t *toolchainFuchsiaArm64) IncludeFlags() string {
	return ""
}

func (t *toolchainFuchsiaArm64) ClangTriple() string {
	return "arm64-fuchsia-android"
}

func (t *toolchainFuchsiaArm64) Cppflags() string {
	return "-Wno-error=deprecated-declarations"
}

func (t *toolchainFuchsiaArm64) Ldflags() string {
	return "--target=arm64-fuchsia --sysroot=" + fuchsiaArm64SysRoot + " -L" + fuchsiaArm64PrebuiltLibsRoot + "/aarch64-fuchsia/lib " + "-Lprebuilts/fuchsia_sdk/arch/arm64/dist/"
}

func (t *toolchainFuchsiaArm64) Lldflags() string {
	return "--target=arm64-fuchsia --sysroot=" + fuchsiaArm64SysRoot + " -L" + fuchsiaArm64PrebuiltLibsRoot + "/aarch64-fuchsia/lib " + "-Lprebuilts/fuchsia_sdk/arch/arm64/dist/"
}

func (t *toolchainFuchsiaArm64) Cflags() string {
	return "--target=arm64-fuchsia --sysroot=" + fuchsiaArm64SysRoot + " -I" + fuchsiaArm64SysRoot + "/include"
}

func (t *toolchainFuchsiaArm64) ToolchainCflags() string {
	return "-march=armv8-a"
}

var toolchainArm64FuchsiaSingleton Toolchain = &toolchainFuchsiaArm64{}

func arm64FuchsiaToolchainFactory(arch android.Arch) Toolchain {
	return toolchainArm64FuchsiaSingleton
}

func init() {
	registerToolchainFactory(android.Fuchsia, android.Arm64, arm64FuchsiaToolchainFactory)
}
