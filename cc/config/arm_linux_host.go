// Copyright 2022 Google Inc. All rights reserved.
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

import "android/soong/android"

var (
	linuxArmCflags = []string{
		"-march=armv7a",
	}

	linuxArm64Cflags = []string{}

	linuxArmLdflags = []string{
		"-march=armv7a",
	}

	linuxArmLldflags = append(linuxArmLdflags,
		"-Wl,--compress-debug-sections=zstd",
	)

	linuxArm64Ldflags = []string{}

	linuxArm64Lldflags = append(linuxArm64Ldflags,
		"-Wl,--compress-debug-sections=zstd",
	)
)

func init() {
	exportedVars.ExportStringListStaticVariable("LinuxArmCflags", linuxArmCflags)
	exportedVars.ExportStringListStaticVariable("LinuxArm64Cflags", linuxArm64Cflags)
	exportedVars.ExportStringListStaticVariable("LinuxArmLdflags", linuxArmLdflags)
	exportedVars.ExportStringListStaticVariable("LinuxArmLldflags", linuxArmLldflags)
	exportedVars.ExportStringListStaticVariable("LinuxArm64Ldflags", linuxArm64Ldflags)
	exportedVars.ExportStringListStaticVariable("LinuxArm64Lldflags", linuxArm64Lldflags)

	exportedVars.ExportStringListStaticVariable("LinuxArmYasmFlags", []string{"-f elf32 -m arm"})
	exportedVars.ExportStringListStaticVariable("LinuxArm64YasmFlags", []string{"-f elf64 -m aarch64"})

}

// Musl arm+arm64
type toolchainLinuxArm struct {
	toolchain32Bit
	toolchainLinux
}

type toolchainLinuxArm64 struct {
	toolchain64Bit
	toolchainLinux
}

func (t *toolchainLinuxArm) Name() string {
	return "arm"
}

func (t *toolchainLinuxArm64) Name() string {
	return "arm64"
}

func (t *toolchainLinuxArm) Cflags() string {
	return "${config.LinuxCflags} ${config.LinuxArmCflags}"
}

func (t *toolchainLinuxArm) Cppflags() string {
	return ""
}

func (t *toolchainLinuxArm64) Cflags() string {
	return "${config.LinuxCflags} ${config.LinuxArm64Cflags}"
}

func (t *toolchainLinuxArm64) Cppflags() string {
	return ""
}

func (t *toolchainLinuxArm) Ldflags() string {
	return "${config.LinuxLdflags} ${config.LinuxArmLdflags}"
}

func (t *toolchainLinuxArm) Lldflags() string {
	return "${config.LinuxLldflags} ${config.LinuxArmLldflags}"
}

func (t *toolchainLinuxArm64) Ldflags() string {
	return "${config.LinuxLdflags} ${config.LinuxArm64Ldflags}"
}

func (t *toolchainLinuxArm64) Lldflags() string {
	return "${config.LinuxLldflags} ${config.LinuxArm64Lldflags}"
}

func (t *toolchainLinuxArm) YasmFlags() string {
	return "${config.LinuxArmYasmFlags}"
}

func (t *toolchainLinuxArm64) YasmFlags() string {
	return "${config.LinuxArm64YasmFlags}"
}

func (toolchainLinuxArm) LibclangRuntimeLibraryArch() string {
	return "arm"
}

func (toolchainLinuxArm64) LibclangRuntimeLibraryArch() string {
	return "arm64"
}

func (t *toolchainLinuxArm) InstructionSetFlags(isa string) (string, error) {
	// TODO: Is no thumb OK?
	return t.toolchainBase.InstructionSetFlags("")
}

type toolchainLinuxMuslArm struct {
	toolchainLinuxArm
	toolchainMusl
}

type toolchainLinuxMuslArm64 struct {
	toolchainLinuxArm64
	toolchainMusl
}

func (t *toolchainLinuxMuslArm) ClangTriple() string {
	return "arm-linux-musleabihf"
}

func (t *toolchainLinuxMuslArm) Cflags() string {
	return t.toolchainLinuxArm.Cflags() + " " + t.toolchainMusl.Cflags()
}

func (t *toolchainLinuxMuslArm) Ldflags() string {
	return t.toolchainLinuxArm.Ldflags() + " " + t.toolchainMusl.Ldflags()
}

func (t *toolchainLinuxMuslArm) Lldflags() string {
	return t.toolchainLinuxArm.Lldflags() + " " + t.toolchainMusl.Lldflags()
}

func (t *toolchainLinuxMuslArm64) ClangTriple() string {
	return "aarch64-linux-musl"
}

func (t *toolchainLinuxMuslArm64) Cflags() string {
	return t.toolchainLinuxArm64.Cflags() + " " + t.toolchainMusl.Cflags()
}

func (t *toolchainLinuxMuslArm64) Ldflags() string {
	return t.toolchainLinuxArm64.Ldflags() + " " + t.toolchainMusl.Ldflags()
}

func (t *toolchainLinuxMuslArm64) Lldflags() string {
	return t.toolchainLinuxArm64.Lldflags() + " " + t.toolchainMusl.Lldflags()
}

var toolchainLinuxMuslArmSingleton Toolchain = &toolchainLinuxMuslArm{}
var toolchainLinuxMuslArm64Singleton Toolchain = &toolchainLinuxMuslArm64{}

func linuxMuslArmToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxMuslArmSingleton
}

func linuxMuslArm64ToolchainFactory(arch android.Arch) Toolchain {
	return toolchainLinuxMuslArm64Singleton
}

func init() {
	registerToolchainFactory(android.LinuxMusl, android.Arm, linuxMuslArmToolchainFactory)
	registerToolchainFactory(android.LinuxMusl, android.Arm64, linuxMuslArm64ToolchainFactory)
}
