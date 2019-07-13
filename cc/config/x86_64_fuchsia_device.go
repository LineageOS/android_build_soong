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

var fuchsiaSysRoot string = "prebuilts/fuchsia_sdk/arch/x64/sysroot"
var fuchsiaPrebuiltLibsRoot string = "fuchsia/prebuilt_libs"

type toolchainFuchsia struct {
	cFlags, ldFlags string
}

type toolchainFuchsiaX8664 struct {
	toolchain64Bit
	toolchainFuchsia
}

func (t *toolchainFuchsiaX8664) Name() string {
	return "x86_64"
}

func (t *toolchainFuchsiaX8664) GccRoot() string {
	return "${config.X86_64GccRoot}"
}

func (t *toolchainFuchsiaX8664) GccTriple() string {
	return "x86_64-linux-android"
}

func (t *toolchainFuchsiaX8664) GccVersion() string {
	return x86_64GccVersion
}

func (t *toolchainFuchsiaX8664) Cflags() string {
	return ""
}

func (t *toolchainFuchsiaX8664) Cppflags() string {
	return ""
}

func (t *toolchainFuchsiaX8664) Ldflags() string {
	return ""
}

func (t *toolchainFuchsiaX8664) IncludeFlags() string {
	return ""
}

func (t *toolchainFuchsiaX8664) ClangTriple() string {
	return "x86_64-fuchsia-android"
}

func (t *toolchainFuchsiaX8664) ClangCppflags() string {
	return "-Wno-error=deprecated-declarations"
}

func (t *toolchainFuchsiaX8664) ClangLdflags() string {
	return "--target=x86_64-fuchsia --sysroot=" + fuchsiaSysRoot + " -L" + fuchsiaPrebuiltLibsRoot + "/x86_64-fuchsia/lib " + "-Lprebuilts/fuchsia_sdk/arch/x64/dist/"

}

func (t *toolchainFuchsiaX8664) ClangLldflags() string {
	return "--target=x86_64-fuchsia --sysroot=" + fuchsiaSysRoot + " -L" + fuchsiaPrebuiltLibsRoot + "/x86_64-fuchsia/lib " + "-Lprebuilts/fuchsia_sdk/arch/x64/dist/"
}

func (t *toolchainFuchsiaX8664) ClangCflags() string {
	return "--target=x86_64-fuchsia --sysroot=" + fuchsiaSysRoot + " -I" + fuchsiaSysRoot + "/include"
}

func (t *toolchainFuchsiaX8664) Bionic() bool {
	return false
}

func (t *toolchainFuchsiaX8664) YasmFlags() string {
	return "-f elf64 -m amd64"
}

func (t *toolchainFuchsiaX8664) ToolchainClangCflags() string {
	return "-mssse3"
}

var toolchainFuchsiaSingleton Toolchain = &toolchainFuchsiaX8664{}

func fuchsiaToolchainFactory(arch android.Arch) Toolchain {
	return toolchainFuchsiaSingleton
}

func init() {
	registerToolchainFactory(android.Fuchsia, android.X86_64, fuchsiaToolchainFactory)
}
