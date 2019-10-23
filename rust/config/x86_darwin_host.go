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
)

var (
	DarwinRustFlags      = []string{}
	DarwinRustLinkFlags  = []string{}
	darwinX8664Rustflags = []string{}
	darwinX8664Linkflags = []string{}
)

func init() {
	registerToolchainFactory(android.Darwin, android.X86_64, darwinX8664ToolchainFactory)
	pctx.StaticVariable("DarwinToolchainRustFlags", strings.Join(DarwinRustFlags, " "))
	pctx.StaticVariable("DarwinToolchainLinkFlags", strings.Join(DarwinRustLinkFlags, " "))
	pctx.StaticVariable("DarwinToolchainX8664RustFlags", strings.Join(darwinX8664Rustflags, " "))
	pctx.StaticVariable("DarwinToolchainX8664LinkFlags", strings.Join(darwinX8664Linkflags, " "))

}

type toolchainDarwin struct {
	toolchainRustFlags string
	toolchainLinkFlags string
}

type toolchainDarwinX8664 struct {
	toolchain64Bit
	toolchainDarwin
}

func (toolchainDarwinX8664) Supported() bool {
	return true
}

func (toolchainDarwinX8664) Bionic() bool {
	return false
}

func (t *toolchainDarwinX8664) Name() string {
	return "x86_64"
}

func (t *toolchainDarwinX8664) RustTriple() string {
	return "x86_64-apple-darwin"
}

func (t *toolchainDarwin) ShlibSuffix() string {
	return ".dylib"
}

func (t *toolchainDarwinX8664) ToolchainLinkFlags() string {
	return "${config.DarwinToolchainLinkFlags} ${config.DarwinToolchainX8664LinkFlags}"
}

func (t *toolchainDarwinX8664) ToolchainRustFlags() string {
	return "${config.DarwinToolchainRustFlags} ${config.DarwinToolchainX8664RustFlags}"
}

func darwinX8664ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainDarwinX8664Singleton
}

var toolchainDarwinX8664Singleton Toolchain = &toolchainDarwinX8664{}
