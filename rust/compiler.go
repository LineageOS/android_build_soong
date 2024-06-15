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
	"android/soong/cc"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/rust/config"
)

type RustLinkage int

const (
	DefaultLinkage RustLinkage = iota
	RlibLinkage
	DylibLinkage
)

type compiler interface {
	initialize(ctx ModuleContext)
	compilerFlags(ctx ModuleContext, flags Flags) Flags
	cfgFlags(ctx ModuleContext, flags Flags) Flags
	featureFlags(ctx ModuleContext, flags Flags) Flags
	compilerProps() []interface{}
	compile(ctx ModuleContext, flags Flags, deps PathDeps) buildOutput
	compilerDeps(ctx DepsContext, deps Deps) Deps
	crateName() string
	edition() string
	features() []string
	rustdoc(ctx ModuleContext, flags Flags, deps PathDeps) android.OptionalPath

	// Output directory in which source-generated code from dependencies is
	// copied. This is equivalent to Cargo's OUT_DIR variable.
	cargoOutDir() android.OptionalPath

	// cargoPkgVersion returns the value of the Cargo_pkg_version property.
	cargoPkgVersion() string

	// cargoEnvCompat returns whether Cargo environment variables should be used.
	cargoEnvCompat() bool

	inData() bool
	install(ctx ModuleContext)
	relativeInstallPath() string
	everInstallable() bool

	nativeCoverage() bool

	Disabled() bool
	SetDisabled()

	stdLinkage(ctx *depsContext) RustLinkage
	noStdlibs() bool

	unstrippedOutputFilePath() android.Path
	strippedOutputFilePath() android.OptionalPath

	checkedCrateRootPath() (android.Path, error)
}

func (compiler *baseCompiler) edition() string {
	return proptools.StringDefault(compiler.Properties.Edition, config.DefaultEdition)
}

func (compiler *baseCompiler) setNoStdlibs() {
	compiler.Properties.No_stdlibs = proptools.BoolPtr(true)
}

func (compiler *baseCompiler) disableLints() {
	compiler.Properties.Lints = proptools.StringPtr("none")
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

	incorrectSourcesError = "srcs can only contain one path for a rust file and source providers prefixed by \":\""
	genSubDir             = "out/"
)

type BaseCompilerProperties struct {
	// path to the source file that is the main entry point of the program (e.g. main.rs or lib.rs).
	// Only a single source file can be defined. Modules which generate source can be included by prefixing
	// the module name with ":", for example ":libfoo_bindgen"
	//
	// If no source file is defined, a single generated source module can be defined to be used as the main source.
	Srcs []string `android:"path,arch_variant"`

	// Entry point that is passed to rustc to begin the compilation. E.g. main.rs or lib.rs.
	// When this property is set,
	//    * sandboxing is enabled for this module, and
	//    * the srcs attribute is interpreted as a list of all source files potentially
	//          used in compilation, including the entrypoint, and
	//    * compile_data can be used to add additional files used in compilation that
	//          not directly used as source files.
	Crate_root *string `android:"path,arch_variant"`

	// name of the lint set that should be used to validate this module.
	//
	// Possible values are "default" (for using a sensible set of lints
	// depending on the module's location), "android" (for the strictest
	// lint set that applies to all Android platform code), "vendor" (for
	// a relaxed set) and "none" (for ignoring all lint warnings and
	// errors). The default value is "default".
	Lints *string

	// flags to pass to rustc. To enable configuration options or features, use the "cfgs" or "features" properties.
	Flags []string `android:"arch_variant"`

	// flags to pass to the linker
	Ld_flags []string `android:"arch_variant"`

	// list of rust rlib crate dependencies
	Rlibs []string `android:"arch_variant"`

	// list of rust automatic crate dependencies.
	// Rustlibs linkage is rlib for host targets and dylib for device targets.
	Rustlibs []string `android:"arch_variant"`

	// list of rust proc_macro crate dependencies
	Proc_macros []string `android:"arch_variant"`

	// list of C shared library dependencies
	Shared_libs []string `android:"arch_variant"`

	// list of C static library dependencies. These dependencies do not normally propagate to dependents
	// and may need to be redeclared. See whole_static_libs for bundling static dependencies into a library.
	Static_libs []string `android:"arch_variant"`

	// Similar to static_libs, but will bundle the static library dependency into a library. This is helpful
	// to avoid having to redeclare the dependency for dependents of this library, but in some cases may also
	// result in bloat if multiple dependencies all include the same static library whole.
	//
	// The common use case for this is when the static library is unlikely to be a dependency of other modules to avoid
	// having to redeclare the static library dependency for every dependent module.
	// If you are not sure what to, for rust_library modules most static dependencies should go in static_libraries,
	// and for rust_ffi modules most static dependencies should go into whole_static_libraries.
	//
	// For rust_ffi static variants, these libraries will be included in the resulting static library archive.
	//
	// For rust_library rlib variants, these libraries will be bundled into the resulting rlib library. This will
	// include all of the static libraries symbols in any dylibs or binaries which use this rlib as well.
	Whole_static_libs []string `android:"arch_variant"`

	// list of Rust system library dependencies.
	//
	// This is usually only needed when `no_stdlibs` is true, in which case it can be used to depend on system crates
	// like `core` and `alloc`.
	Stdlibs []string `android:"arch_variant"`

	// crate name, required for modules which produce Rust libraries: rust_library, rust_ffi and SourceProvider
	// modules which create library variants (rust_bindgen). This must be the expected extern crate name used in
	// source, and is required to conform to an enforced format matching library output files (if the output file is
	// lib<someName><suffix>, the crate_name property must be <someName>).
	Crate_name string `android:"arch_variant"`

	// list of features to enable for this crate
	Features []string `android:"arch_variant"`

	// list of configuration options to enable for this crate. To enable features, use the "features" property.
	Cfgs []string `android:"arch_variant"`

	// specific rust edition that should be used if the default version is not desired
	Edition *string `android:"arch_variant"`

	// sets name of the output
	Stem *string `android:"arch_variant"`

	// append to name of output
	Suffix *string `android:"arch_variant"`

	// install to a subdirectory of the default install path for the module
	Relative_install_path *string `android:"arch_variant"`

	// whether to suppress inclusion of standard crates - defaults to false
	No_stdlibs *bool `android:"arch_variant"`

	// Change the rustlibs linkage to select rlib linkage by default for device targets.
	// Also link libstd as an rlib as well on device targets.
	// Note: This is the default behavior for host targets.
	//
	// This is primarily meant for rust_binary and rust_ffi modules where the default
	// linkage of libstd might need to be overridden in some use cases. This should
	// generally be avoided with other module types since it may cause collisions at
	// linkage if all dependencies of the root binary module do not link against libstd
	// the same way.
	Prefer_rlib *bool `android:"arch_variant"`

	// Enables emitting certain Cargo environment variables. Only intended to be used for compatibility purposes.
	// Will set CARGO_CRATE_NAME to the crate_name property's value.
	// Will set CARGO_BIN_NAME to the output filename value without the extension.
	Cargo_env_compat *bool

	// If cargo_env_compat is true, sets the CARGO_PKG_VERSION env var to this value.
	Cargo_pkg_version *string
}

type baseCompiler struct {
	Properties BaseCompilerProperties

	// Install related
	dir      string
	dir64    string
	subDir   string
	relative string
	path     android.InstallPath
	location installLocation
	sanitize *sanitize

	distFile android.OptionalPath

	installDeps android.InstallPaths

	// unstripped output file.
	unstrippedOutputFile android.Path

	// stripped output file.
	strippedOutputFile android.OptionalPath

	// If a crate has a source-generated dependency, a copy of the source file
	// will be available in cargoOutDir (equivalent to Cargo OUT_DIR).
	// This is stored internally because it may not be available during
	// singleton-generation passes like rustdoc/rust_project.json, but should
	// be stashed during initial generation.
	cachedCargoOutDir android.ModuleOutPath
	// Calculated crate root cached internally because ModuleContext is not
	// available to singleton targets like rustdoc/rust_project.json
	cachedCrateRootPath android.Path
	// If cachedCrateRootPath is nil after initialization, this will contain
	// an explanation of why
	cachedCrateRootError error
}

func (compiler *baseCompiler) Disabled() bool {
	return false
}

func (compiler *baseCompiler) SetDisabled() {
	panic("baseCompiler does not implement SetDisabled()")
}

func (compiler *baseCompiler) noStdlibs() bool {
	return Bool(compiler.Properties.No_stdlibs)
}

func (compiler *baseCompiler) coverageOutputZipPath() android.OptionalPath {
	panic("baseCompiler does not implement coverageOutputZipPath()")
}

func (compiler *baseCompiler) preferRlib() bool {
	return Bool(compiler.Properties.Prefer_rlib)
}

func (compiler *baseCompiler) stdLinkage(ctx *depsContext) RustLinkage {
	// For devices, we always link stdlibs in as dylibs by default.
	if compiler.preferRlib() {
		return RlibLinkage
	} else if ctx.Device() {
		return DylibLinkage
	} else {
		return RlibLinkage
	}
}

var _ compiler = (*baseCompiler)(nil)

func (compiler *baseCompiler) inData() bool {
	return compiler.location == InstallInData
}

func (compiler *baseCompiler) compilerProps() []interface{} {
	return []interface{}{&compiler.Properties}
}

func (compiler *baseCompiler) cfgsToFlags() []string {
	flags := []string{}
	for _, cfg := range compiler.Properties.Cfgs {
		flags = append(flags, "--cfg '"+cfg+"'")
	}

	return flags
}

func (compiler *baseCompiler) features() []string {
	return compiler.Properties.Features
}

func (compiler *baseCompiler) featuresToFlags() []string {
	flags := []string{}
	for _, feature := range compiler.features() {
		flags = append(flags, "--cfg 'feature=\""+feature+"\"'")
	}

	return flags
}

func (compiler *baseCompiler) featureFlags(ctx ModuleContext, flags Flags) Flags {
	flags.RustFlags = append(flags.RustFlags, compiler.featuresToFlags()...)
	flags.RustdocFlags = append(flags.RustdocFlags, compiler.featuresToFlags()...)

	return flags
}

func (compiler *baseCompiler) cfgFlags(ctx ModuleContext, flags Flags) Flags {
	if ctx.RustModule().InVendorOrProduct() {
		compiler.Properties.Cfgs = append(compiler.Properties.Cfgs, "android_vndk")
		if ctx.RustModule().InVendor() {
			compiler.Properties.Cfgs = append(compiler.Properties.Cfgs, "android_vendor")
		} else if ctx.RustModule().InProduct() {
			compiler.Properties.Cfgs = append(compiler.Properties.Cfgs, "android_product")
		}
	}

	flags.RustFlags = append(flags.RustFlags, compiler.cfgsToFlags()...)
	flags.RustdocFlags = append(flags.RustdocFlags, compiler.cfgsToFlags()...)
	return flags
}

func (compiler *baseCompiler) compilerFlags(ctx ModuleContext, flags Flags) Flags {

	lintFlags, err := config.RustcLintsForDir(ctx.ModuleDir(), compiler.Properties.Lints)
	if err != nil {
		ctx.PropertyErrorf("lints", err.Error())
	}

	// linkage-related flags are disallowed.
	for _, s := range compiler.Properties.Ld_flags {
		if strings.HasPrefix(s, "-Wl,-l") || strings.HasPrefix(s, "-Wl,-L") {
			ctx.PropertyErrorf("ld_flags", "'-Wl,-l' and '-Wl,-L' flags cannot be manually specified")
		}
	}
	for _, s := range compiler.Properties.Flags {
		if strings.HasPrefix(s, "-l") || strings.HasPrefix(s, "-L") {
			ctx.PropertyErrorf("flags", "'-l' and '-L' flags cannot be manually specified")
		}
		if strings.HasPrefix(s, "--extern") {
			ctx.PropertyErrorf("flags", "'--extern' flag cannot be manually specified")
		}
		if strings.HasPrefix(s, "-Clink-args=") || strings.HasPrefix(s, "-C link-args=") {
			ctx.PropertyErrorf("flags", "'-C link-args' flag cannot be manually specified")
		}
	}

	flags.RustFlags = append(flags.RustFlags, lintFlags)
	flags.RustFlags = append(flags.RustFlags, compiler.Properties.Flags...)
	flags.RustFlags = append(flags.RustFlags, "--edition="+compiler.edition())
	flags.RustdocFlags = append(flags.RustdocFlags, "--edition="+compiler.edition())
	flags.LinkFlags = append(flags.LinkFlags, compiler.Properties.Ld_flags...)
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, config.GlobalRustFlags...)
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, ctx.toolchain().ToolchainRustFlags())
	flags.GlobalLinkFlags = append(flags.GlobalLinkFlags, ctx.toolchain().ToolchainLinkFlags())
	flags.EmitXrefs = ctx.Config().EmitXrefRules()

	if ctx.Host() && !ctx.Windows() {
		flags.LinkFlags = append(flags.LinkFlags, cc.RpathFlags(ctx)...)
	}

	if ctx.Os() == android.Linux {
		// Add -lc, -lrt, -ldl, -lpthread, -lm and -lgcc_s to glibc builds to match
		// the default behavior of device builds.
		flags.LinkFlags = append(flags.LinkFlags, config.LinuxHostGlobalLinkFlags...)
	} else if ctx.Os() == android.Darwin {
		// Add -lc, -ldl, -lpthread and -lm to glibc darwin builds to match the default
		// behavior of device builds.
		flags.LinkFlags = append(flags.LinkFlags,
			"-lc",
			"-ldl",
			"-lpthread",
			"-lm",
		)
	}
	return flags
}

func (compiler *baseCompiler) compile(ctx ModuleContext, flags Flags, deps PathDeps) buildOutput {
	panic(fmt.Errorf("baseCrater doesn't know how to crate things!"))
}

func (compiler *baseCompiler) rustdoc(ctx ModuleContext, flags Flags,
	deps PathDeps) android.OptionalPath {

	return android.OptionalPath{}
}

func (compiler *baseCompiler) initialize(ctx ModuleContext) {
	compiler.cachedCargoOutDir = android.PathForModuleOut(ctx, genSubDir)
	if compiler.Properties.Crate_root == nil {
		compiler.cachedCrateRootPath, compiler.cachedCrateRootError = srcPathFromModuleSrcs(ctx, compiler.Properties.Srcs)
	} else {
		compiler.cachedCrateRootPath = android.PathForModuleSrc(ctx, *compiler.Properties.Crate_root)
		compiler.cachedCrateRootError = nil
	}
}

func (compiler *baseCompiler) cargoOutDir() android.OptionalPath {
	return android.OptionalPathForPath(compiler.cachedCargoOutDir)
}

func (compiler *baseCompiler) cargoEnvCompat() bool {
	return Bool(compiler.Properties.Cargo_env_compat)
}

func (compiler *baseCompiler) cargoPkgVersion() string {
	return String(compiler.Properties.Cargo_pkg_version)
}

func (compiler *baseCompiler) unstrippedOutputFilePath() android.Path {
	return compiler.unstrippedOutputFile
}

func (compiler *baseCompiler) strippedOutputFilePath() android.OptionalPath {
	return compiler.strippedOutputFile
}

func (compiler *baseCompiler) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps.Rlibs = append(deps.Rlibs, compiler.Properties.Rlibs...)
	deps.Rustlibs = append(deps.Rustlibs, compiler.Properties.Rustlibs...)
	deps.ProcMacros = append(deps.ProcMacros, compiler.Properties.Proc_macros...)
	deps.StaticLibs = append(deps.StaticLibs, compiler.Properties.Static_libs...)
	deps.WholeStaticLibs = append(deps.WholeStaticLibs, compiler.Properties.Whole_static_libs...)
	deps.SharedLibs = append(deps.SharedLibs, compiler.Properties.Shared_libs...)
	deps.Stdlibs = append(deps.Stdlibs, compiler.Properties.Stdlibs...)

	if !Bool(compiler.Properties.No_stdlibs) {
		for _, stdlib := range config.Stdlibs {
			// If we're building for the build host, use the prebuilt stdlibs, unless the host
			// is linux_bionic which doesn't have prebuilts.
			if ctx.Host() && !ctx.Target().HostCross && ctx.Target().Os != android.LinuxBionic {
				stdlib = "prebuilt_" + stdlib
			}
			deps.Stdlibs = append(deps.Stdlibs, stdlib)
		}
	}
	return deps
}

func bionicDeps(ctx DepsContext, deps Deps, static bool) Deps {
	bionicLibs := []string{}
	bionicLibs = append(bionicLibs, "liblog")
	bionicLibs = append(bionicLibs, "libc")
	bionicLibs = append(bionicLibs, "libm")
	bionicLibs = append(bionicLibs, "libdl")

	if static {
		deps.StaticLibs = append(deps.StaticLibs, bionicLibs...)
	} else {
		deps.SharedLibs = append(deps.SharedLibs, bionicLibs...)
	}
	if ctx.RustModule().StaticExecutable() {
		deps.StaticLibs = append(deps.StaticLibs, "libunwind")
	}
	if libRuntimeBuiltins := config.BuiltinsRuntimeLibrary(ctx.toolchain()); libRuntimeBuiltins != "" {
		deps.StaticLibs = append(deps.StaticLibs, libRuntimeBuiltins)
	}
	return deps
}

func muslDeps(ctx DepsContext, deps Deps, static bool) Deps {
	muslLibs := []string{"libc_musl"}
	if static {
		deps.StaticLibs = append(deps.StaticLibs, muslLibs...)
	} else {
		deps.SharedLibs = append(deps.SharedLibs, muslLibs...)
	}
	if libRuntimeBuiltins := config.BuiltinsRuntimeLibrary(ctx.toolchain()); libRuntimeBuiltins != "" {
		deps.StaticLibs = append(deps.StaticLibs, libRuntimeBuiltins)
	}

	return deps
}

func (compiler *baseCompiler) crateName() string {
	return compiler.Properties.Crate_name
}

func (compiler *baseCompiler) everInstallable() bool {
	// Most modules are installable, so return true by default.
	return true
}

func (compiler *baseCompiler) installDir(ctx ModuleContext) android.InstallPath {
	dir := compiler.dir
	if ctx.toolchain().Is64Bit() && compiler.dir64 != "" {
		dir = compiler.dir64
	}
	if ctx.Target().NativeBridge == android.NativeBridgeEnabled {
		dir = filepath.Join(dir, ctx.Target().NativeBridgeRelativePath)
	}
	if !ctx.Host() && ctx.Config().HasMultilibConflict(ctx.Arch().ArchType) {
		dir = filepath.Join(dir, ctx.Arch().ArchType.String())
	}

	if compiler.location == InstallInData && ctx.RustModule().InVendorOrProduct() {
		if ctx.RustModule().InProduct() {
			dir = filepath.Join(dir, "product")
		} else if ctx.RustModule().InVendor() {
			dir = filepath.Join(dir, "vendor")
		} else {
			ctx.ModuleErrorf("Unknown data+VNDK installation kind")
		}
	}

	return android.PathForModuleInstall(ctx, dir, compiler.subDir,
		compiler.relativeInstallPath(), compiler.relative)
}

func (compiler *baseCompiler) nativeCoverage() bool {
	return false
}

func (compiler *baseCompiler) install(ctx ModuleContext) {
	path := ctx.RustModule().OutputFile()
	compiler.path = ctx.InstallFile(compiler.installDir(ctx), path.Path().Base(), path.Path(), compiler.installDeps...)
}

func (compiler *baseCompiler) installTestData(ctx ModuleContext, data []android.DataPath) {
	installedData := ctx.InstallTestData(compiler.installDir(ctx), data)
	compiler.installDeps = append(compiler.installDeps, installedData...)
}

func (compiler *baseCompiler) getStem(ctx ModuleContext) string {
	return compiler.getStemWithoutSuffix(ctx) + String(compiler.Properties.Suffix)
}

func (compiler *baseCompiler) getStemWithoutSuffix(ctx BaseModuleContext) string {
	stem := ctx.ModuleName()
	if String(compiler.Properties.Stem) != "" {
		stem = String(compiler.Properties.Stem)
	}

	return stem
}

func (compiler *baseCompiler) relativeInstallPath() string {
	return String(compiler.Properties.Relative_install_path)
}

func (compiler *baseCompiler) checkedCrateRootPath() (android.Path, error) {
	return compiler.cachedCrateRootPath, compiler.cachedCrateRootError
}

func crateRootPath(ctx ModuleContext, compiler compiler) android.Path {
	root, err := compiler.checkedCrateRootPath()
	if err != nil {
		ctx.PropertyErrorf("srcs", err.Error())
	}
	return root
}

// Returns the Path for the main source file along with Paths for generated source files from modules listed in srcs.
func srcPathFromModuleSrcs(ctx ModuleContext, srcs []string) (android.Path, error) {
	// The srcs can contain strings with prefix ":".
	// They are dependent modules of this module, with android.SourceDepTag.
	// They are not the main source file compiled by rustc.
	numSrcs := 0
	srcIndex := 0
	for i, s := range srcs {
		if android.SrcIsModule(s) == "" {
			numSrcs++
			srcIndex = i
		}
	}
	if numSrcs > 1 {
		return nil, errors.New(incorrectSourcesError)
	}

	// If a main source file is not provided we expect only a single SourceProvider module to be defined
	// within srcs, with the expectation that the first source it provides is the entry point.
	if srcIndex != 0 {
		return nil, errors.New("main source file must be the first in srcs")
	} else if numSrcs > 1 {
		return nil, errors.New("only a single generated source module can be defined without a main source file.")
	}

	// TODO: b/297264540 - once all modules are sandboxed, we need to select the proper
	// entry point file from Srcs rather than taking the first one
	paths := android.PathsForModuleSrc(ctx, srcs)
	if len(paths) == 0 {
		return nil, errors.New("srcs must not be empty")
	}
	return paths[srcIndex], nil
}
