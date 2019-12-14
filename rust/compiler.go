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

package rust

import (
	"fmt"
	"path/filepath"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/rust/config"
)

func getEdition(compiler *baseCompiler) string {
	return proptools.StringDefault(compiler.Properties.Edition, config.DefaultEdition)
}

func getDenyWarnings(compiler *baseCompiler) bool {
	return BoolDefault(compiler.Properties.Deny_warnings, config.DefaultDenyWarnings)
}

func (compiler *baseCompiler) setNoStdlibs() {
	compiler.Properties.No_stdlibs = proptools.BoolPtr(true)
}

func NewBaseCompiler(dir, dir64 string, location installLocation) *baseCompiler {
	return &baseCompiler{
		Properties: BaseCompilerProperties{},
		dir:        dir,
		dir64:      dir64,
		location:   location,
	}
}

type installLocation int

const (
	InstallInSystem installLocation = 0
	InstallInData                   = iota
)

type BaseCompilerProperties struct {
	// whether to pass "-D warnings" to rustc. Defaults to true.
	Deny_warnings *bool

	// flags to pass to rustc
	Flags []string `android:"path,arch_variant"`

	// flags to pass to the linker
	Ld_flags []string `android:"path,arch_variant"`

	// list of rust rlib crate dependencies
	Rlibs []string `android:"arch_variant"`

	// list of rust dylib crate dependencies
	Dylibs []string `android:"arch_variant"`

	// list of rust proc_macro crate dependencies
	Proc_macros []string `android:"arch_variant"`

	// list of C shared library dependencies
	Shared_libs []string `android:"arch_variant"`

	// list of C static library dependencies
	Static_libs []string `android:"arch_variant"`

	// crate name, required for libraries. This must be the expected extern crate name used in source
	Crate_name string `android:"arch_variant"`

	// list of features to enable for this crate
	Features []string `android:"arch_variant"`

	// specific rust edition that should be used if the default version is not desired
	Edition *string `android:"arch_variant"`

	// sets name of the output
	Stem *string `android:"arch_variant"`

	// append to name of output
	Suffix *string `android:"arch_variant"`

	// install to a subdirectory of the default install path for the module
	Relative_install_path *string `android:"arch_variant"`

	// whether to suppress inclusion of standard crates - defaults to false
	No_stdlibs *bool
}

type baseCompiler struct {
	Properties    BaseCompilerProperties
	pathDeps      android.Paths
	rustFlagsDeps android.Paths
	linkFlagsDeps android.Paths
	flags         string
	linkFlags     string
	depFlags      []string
	linkDirs      []string
	edition       string
	src           android.Path //rustc takes a single src file

	// Install related
	dir      string
	dir64    string
	subDir   string
	relative string
	path     android.InstallPath
	location installLocation
}

var _ compiler = (*baseCompiler)(nil)

func (compiler *baseCompiler) inData() bool {
	return compiler.location == InstallInData
}

func (compiler *baseCompiler) compilerProps() []interface{} {
	return []interface{}{&compiler.Properties}
}

func (compiler *baseCompiler) featuresToFlags(features []string) []string {
	flags := []string{}
	for _, feature := range features {
		flags = append(flags, "--cfg 'feature=\""+feature+"\"'")
	}
	return flags
}

func (compiler *baseCompiler) compilerFlags(ctx ModuleContext, flags Flags) Flags {

	if getDenyWarnings(compiler) {
		flags.RustFlags = append(flags.RustFlags, "-D warnings")
	}
	flags.RustFlags = append(flags.RustFlags, compiler.Properties.Flags...)
	flags.RustFlags = append(flags.RustFlags, compiler.featuresToFlags(compiler.Properties.Features)...)
	flags.RustFlags = append(flags.RustFlags, "--edition="+getEdition(compiler))
	flags.LinkFlags = append(flags.LinkFlags, compiler.Properties.Ld_flags...)
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, config.GlobalRustFlags...)
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, ctx.toolchain().ToolchainRustFlags())
	flags.GlobalLinkFlags = append(flags.GlobalLinkFlags, ctx.toolchain().ToolchainLinkFlags())

	if ctx.Host() && !ctx.Windows() {
		rpath_prefix := `\$$ORIGIN/`
		if ctx.Darwin() {
			rpath_prefix = "@loader_path/"
		}

		var rpath string
		if ctx.toolchain().Is64Bit() {
			rpath = "lib64"
		} else {
			rpath = "lib"
		}
		flags.LinkFlags = append(flags.LinkFlags, "-Wl,-rpath,"+rpath_prefix+rpath)
		flags.LinkFlags = append(flags.LinkFlags, "-Wl,-rpath,"+rpath_prefix+"../"+rpath)
	}

	return flags
}

func (compiler *baseCompiler) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Path {
	panic(fmt.Errorf("baseCrater doesn't know how to crate things!"))
}

func (compiler *baseCompiler) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps.Rlibs = append(deps.Rlibs, compiler.Properties.Rlibs...)
	deps.Dylibs = append(deps.Dylibs, compiler.Properties.Dylibs...)
	deps.ProcMacros = append(deps.ProcMacros, compiler.Properties.Proc_macros...)
	deps.StaticLibs = append(deps.StaticLibs, compiler.Properties.Static_libs...)
	deps.SharedLibs = append(deps.SharedLibs, compiler.Properties.Shared_libs...)

	if !Bool(compiler.Properties.No_stdlibs) {
		for _, stdlib := range config.Stdlibs {
			// If we're building for host, use the compiler's stdlibs
			if ctx.Host() {
				stdlib = stdlib + "_" + ctx.toolchain().RustTriple()
			}

			// This check is technically insufficient - on the host, where
			// static linking is the default, if one of our static
			// dependencies uses a dynamic library, we need to dynamically
			// link the stdlib as well.
			if (len(deps.Dylibs) > 0) || (!ctx.Host()) {
				// Dynamically linked stdlib
				deps.Dylibs = append(deps.Dylibs, stdlib)
			}
		}
	}
	return deps
}

func (compiler *baseCompiler) bionicDeps(ctx DepsContext, deps Deps) Deps {
	deps.SharedLibs = append(deps.SharedLibs, "liblog")
	deps.SharedLibs = append(deps.SharedLibs, "libc")
	deps.SharedLibs = append(deps.SharedLibs, "libm")
	deps.SharedLibs = append(deps.SharedLibs, "libdl")

	//TODO(b/141331117) libstd requires libgcc on Android
	deps.StaticLibs = append(deps.StaticLibs, "libgcc")

	return deps
}

func (compiler *baseCompiler) crateName() string {
	return compiler.Properties.Crate_name
}

func (compiler *baseCompiler) installDir(ctx ModuleContext) android.InstallPath {
	dir := compiler.dir
	if ctx.toolchain().Is64Bit() && compiler.dir64 != "" {
		dir = compiler.dir64
	}
	if !ctx.Host() || ctx.Target().NativeBridge == android.NativeBridgeEnabled {
		dir = filepath.Join(dir, ctx.Arch().ArchType.String())
	}
	return android.PathForModuleInstall(ctx, dir, compiler.subDir,
		compiler.relativeInstallPath(), compiler.relative)
}

func (compiler *baseCompiler) install(ctx ModuleContext, file android.Path) {
	compiler.path = ctx.InstallFile(compiler.installDir(ctx), file.Base(), file)
}

func (compiler *baseCompiler) getStem(ctx ModuleContext) string {
	return compiler.getStemWithoutSuffix(ctx) + String(compiler.Properties.Suffix)
}

func (compiler *baseCompiler) getStemWithoutSuffix(ctx BaseModuleContext) string {
	stem := ctx.baseModuleName()
	if String(compiler.Properties.Stem) != "" {
		stem = String(compiler.Properties.Stem)
	}

	return stem
}

func (compiler *baseCompiler) relativeInstallPath() string {
	return String(compiler.Properties.Relative_install_path)
}

func srcPathFromModuleSrcs(ctx ModuleContext, srcs []string) android.Path {
	srcPaths := android.PathsForModuleSrc(ctx, srcs)
	if len(srcPaths) != 1 {
		ctx.PropertyErrorf("srcs", "srcs can only contain one path for rust modules")
	}
	return srcPaths[0]
}
