//
// Copyright (C) 2021 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package rust

import (
	"path"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/rust/config"

	"github.com/google/blueprint/proptools"
)

// This module is used to compile the rust toolchain libraries
// When RUST_PREBUILTS_VERSION is set, the library will generated
// from the given Rust version.
func init() {
	android.RegisterModuleType("rust_toolchain_library",
		rustToolchainLibraryFactory)
	android.RegisterModuleType("rust_toolchain_library_rlib",
		rustToolchainLibraryRlibFactory)
	android.RegisterModuleType("rust_toolchain_library_dylib",
		rustToolchainLibraryDylibFactory)
	android.RegisterModuleType("rust_stdlib_prebuilt_host",
		rustHostPrebuiltSysrootLibraryFactory)
}

type toolchainLibraryProperties struct {
	// path to the toolchain crate root, relative to the top of the toolchain source
	Toolchain_crate_root *string `android:"arch_variant"`
	// path to the rest of the toolchain srcs, relative to the top of the toolchain source
	Toolchain_srcs []string `android:"arch_variant"`
}

type toolchainLibraryDecorator struct {
	*libraryDecorator
	Properties toolchainLibraryProperties
}

// toolchainCrateRoot implements toolchainCompiler.
func (t *toolchainLibraryDecorator) toolchainCrateRoot() *string {
	return t.Properties.Toolchain_crate_root
}

// toolchainSrcs implements toolchainCompiler.
func (t *toolchainLibraryDecorator) toolchainSrcs() []string {
	return t.Properties.Toolchain_srcs
}

// rust_toolchain_library produces all rust variants.
func rustToolchainLibraryFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyRust()

	return initToolchainLibrary(module, library)
}

// rust_toolchain_library_dylib produces a dylib.
func rustToolchainLibraryDylibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyDylib()

	return initToolchainLibrary(module, library)
}

// rust_toolchain_library_rlib produces an rlib.
func rustToolchainLibraryRlibFactory() android.Module {
	module, library := NewRustLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyRlib()

	return initToolchainLibrary(module, library)
}

func initToolchainLibrary(module *Module, library *libraryDecorator) android.Module {
	toolchainLibrary := &toolchainLibraryDecorator{
		libraryDecorator: library,
	}
	module.compiler = toolchainLibrary
	module.AddProperties(&toolchainLibrary.Properties)
	android.AddLoadHook(module, rustSetToolchainSource)

	return module.Init()
}

type toolchainCompiler interface {
	toolchainCrateRoot() *string
	toolchainSrcs() []string
}

var _ toolchainCompiler = (*toolchainLibraryDecorator)(nil)

func rustSetToolchainSource(ctx android.LoadHookContext) {
	if toolchainLib, ok := ctx.Module().(*Module).compiler.(toolchainCompiler); ok {
		prefix := filepath.Join(GetRustPrebuiltVersion(ctx))
		versionedCrateRoot := path.Join(prefix, android.String(toolchainLib.toolchainCrateRoot()))
		versionedSrcs := make([]string, len(toolchainLib.toolchainSrcs()))
		for i, src := range toolchainLib.toolchainSrcs() {
			versionedSrcs[i] = path.Join(prefix, src)
		}

		type props struct {
			Crate_root *string
			Srcs       []string
		}
		p := &props{}
		p.Crate_root = &versionedCrateRoot
		p.Srcs = versionedSrcs
		ctx.AppendProperties(p)
	} else {
		ctx.ModuleErrorf("Called rustSetToolchainSource on a non-Toolchain Module.")
	}
}

// GetRustPrebuiltVersion returns the RUST_PREBUILTS_VERSION env var, or the default version if it is not defined.
func GetRustPrebuiltVersion(ctx android.LoadHookContext) string {
	return ctx.Config().GetenvWithDefault("RUST_PREBUILTS_VERSION", config.RustDefaultVersion)
}

type hostPrebuiltTargetProps struct {
	Suffix *string
	Dylib  struct {
		Srcs []string
	}
	Rlib struct {
		Srcs []string
	}
	Link_dirs          []string
	Enabled            *bool
	Force_use_prebuilt *bool
}

type hostPrebuiltProps struct {
	Name    *string
	Enabled *bool
	Target  struct {
		Linux_glibc_x86_64 hostPrebuiltTargetProps
		Linux_glibc_x86    hostPrebuiltTargetProps
		Linux_musl_x86_64  hostPrebuiltTargetProps
		Linux_musl_x86     hostPrebuiltTargetProps
		Linux_musl_arm64   hostPrebuiltTargetProps
		Darwin_x86_64      hostPrebuiltTargetProps
	}
}

func rustHostPrebuiltSysrootLibraryFactory() android.Module {
	module, _ := NewPrebuiltLibrary(android.HostSupportedNoCross)
	android.AddLoadHook(module, constructLibProps( /*rlib=*/ true /*solib=*/, true))
	return module.Init()
}

func constructLibProps(rlib, solib bool) func(ctx android.LoadHookContext) {
	return func(ctx android.LoadHookContext) {
		rustDir := path.Join(GetRustPrebuiltVersion(ctx), "lib", "rustlib")
		name := android.RemoveOptionalPrebuiltPrefix(ctx.ModuleName())
		name = strings.Replace(name, ".rust_sysroot", "", -1)
		_, platform := filepath.Split(ctx.ModuleDir())

		p := hostPrebuiltProps{}
		p.Enabled = proptools.BoolPtr(false)

		if platform == "linux-x86" && ctx.Config().BuildOS == android.Linux {
			p.Target.Linux_glibc_x86_64.addPrebuiltToTarget(ctx, name, rustDir, "linux-x86", "x86_64-unknown-linux-gnu", rlib, solib)
			p.Target.Linux_glibc_x86.addPrebuiltToTarget(ctx, name, rustDir, "linux-x86", "i686-unknown-linux-gnu", rlib, solib)
		} else if platform == "linux-musl-x86" && ctx.Config().BuildOS == android.LinuxMusl && (ctx.Config().BuildArch == android.X86 || ctx.Config().BuildArch == android.X86_64) {
			p.Target.Linux_musl_x86_64.addPrebuiltToTarget(ctx, name, rustDir, "linux-musl-x86", "x86_64-unknown-linux-musl", rlib, solib)
			p.Target.Linux_musl_x86.addPrebuiltToTarget(ctx, name, rustDir, "linux-musl-x86", "i686-unknown-linux-musl", rlib, solib)
		} else if platform == "linux-arm64" && ctx.Config().BuildOS == android.LinuxMusl && ctx.Config().BuildArch == android.Arm64 {
			p.Target.Linux_musl_arm64.addPrebuiltToTarget(ctx, name, rustDir, "linux-arm64", "aarch64-unknown-linux-musl", rlib, solib)
		} else if platform == "darwin" && ctx.Config().BuildOS == android.Darwin {
			p.Target.Darwin_x86_64.addPrebuiltToTarget(ctx, name, rustDir, "darwin-x86", "x86_64-apple-darwin", rlib, solib)
		} else {
			p.Name = proptools.StringPtr("prebuilt_" + ctx.ModuleName() + "." + platform)
		}

		ctx.AppendProperties(&p)
	}
}

func (target *hostPrebuiltTargetProps) addPrebuiltToTarget(ctx android.LoadHookContext, libName, rustDir, platform, arch string, rlib, solib bool) {
	dir := path.Join(rustDir, arch, "lib")
	target.Link_dirs = []string{dir}
	target.Enabled = proptools.BoolPtr(true)
	target.Force_use_prebuilt = proptools.BoolPtr(true)
	if rlib {
		rlib, suffix := getPrebuilt(ctx, dir, libName, ".rlib")
		target.Rlib.Srcs = []string{rlib}
		target.Suffix = proptools.StringPtr(suffix)
	}
	if solib {
		// The suffixes are the same between the dylib and the rlib,
		// so it's okay if we overwrite the rlib suffix
		var soSuffix string
		if strings.Contains(platform, "darwin") {
			soSuffix = ".dylib"
		} else {
			soSuffix = ".so"
		}
		dylib, suffix := getPrebuilt(ctx, dir, libName, soSuffix)
		target.Dylib.Srcs = []string{dylib}
		target.Suffix = proptools.StringPtr(suffix)
	}
}

// getPrebuilt returns the module relative Rust library path and the suffix hash.
func getPrebuilt(ctx android.LoadHookContext, dir, lib, extension string) (string, string) {
	globPath := path.Join(ctx.ModuleDir(), dir, lib) + "-*" + extension
	libMatches := ctx.Glob(globPath, nil)

	if len(libMatches) != 1 {
		ctx.ModuleErrorf("Unexpected number of matches for prebuilt libraries at path %q, found %d matches", globPath, len(libMatches))
		return "", ""
	}

	// Collect the suffix by trimming the extension from the Base, then removing the library name and hyphen.
	suffix := strings.TrimSuffix(libMatches[0].Base(), extension)[len(lib)+1:]

	// Get the relative path from the match by trimming out the module directory.
	relPath := strings.TrimPrefix(libMatches[0].String(), ctx.ModuleDir()+"/")

	return relPath, suffix
}
