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

package cc

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"

	"android/soong/android"
	"android/soong/cc/config"
)

// LibraryProperties is a collection of properties shared by cc library rules.
type LibraryProperties struct {
	// local file name to pass to the linker as -unexported_symbols_list
	Unexported_symbols_list *string `android:"path,arch_variant"`
	// local file name to pass to the linker as -force_symbols_not_weak_list
	Force_symbols_not_weak_list *string `android:"path,arch_variant"`
	// local file name to pass to the linker as -force_symbols_weak_list
	Force_symbols_weak_list *string `android:"path,arch_variant"`

	// rename host libraries to prevent overlap with system installed libraries
	Unique_host_soname *bool

	Aidl struct {
		// export headers generated from .aidl sources
		Export_aidl_headers *bool
	}

	Proto struct {
		// export headers generated from .proto sources
		Export_proto_headers *bool
	}

	Sysprop struct {
		// Whether platform owns this sysprop library.
		Platform *bool
	} `blueprint:"mutated"`

	Static_ndk_lib *bool

	Stubs struct {
		// Relative path to the symbol map. The symbol map provides the list of
		// symbols that are exported for stubs variant of this library.
		Symbol_file *string `android:"path"`

		// List versions to generate stubs libs for.
		Versions []string
	}

	// set the name of the output
	Stem *string `android:"arch_variant"`

	// set suffix of the name of the output
	Suffix *string `android:"arch_variant"`

	Target struct {
		Vendor, Product struct {
			// set suffix of the name of the output
			Suffix *string `android:"arch_variant"`
		}
	}

	// Names of modules to be overridden. Listed modules can only be other shared libraries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden libraries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other library will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string

	// Properties for ABI compatibility checker
	Header_abi_checker struct {
		// Enable ABI checks (even if this is not an LLNDK/VNDK lib)
		Enabled *bool

		// Path to a symbol file that specifies the symbols to be included in the generated
		// ABI dump file
		Symbol_file *string `android:"path"`

		// Symbol versions that should be ignored from the symbol file
		Exclude_symbol_versions []string

		// Symbol tags that should be ignored from the symbol file
		Exclude_symbol_tags []string

		// Run checks on all APIs (in addition to the ones referred by
		// one of exported ELF symbols.)
		Check_all_apis *bool
	}

	// Order symbols in .bss section by their sizes.  Only useful for shared libraries.
	Sort_bss_symbols_by_size *bool

	// Inject boringssl hash into the shared library.  This is only intended for use by external/boringssl.
	Inject_bssl_hash *bool `android:"arch_variant"`

	// If this is an LLNDK library, the name of the equivalent llndk_library module.
	Llndk_stubs *string

	// If this is an LLNDK library, properties to describe the LLNDK stubs.  Will be copied from
	// the module pointed to by llndk_stubs if it is set.
	Llndk llndkLibraryProperties
}

// StaticProperties is a properties stanza to affect only attributes of the "static" variants of a
// library module.
type StaticProperties struct {
	Static StaticOrSharedProperties `android:"arch_variant"`
}

// SharedProperties is a properties stanza to affect only attributes of the "shared" variants of a
// library module.
type SharedProperties struct {
	Shared StaticOrSharedProperties `android:"arch_variant"`
}

// StaticOrSharedProperties is an embedded struct representing properties to affect attributes of
// either only the "static" variants or only the "shared" variants of a library module. These override
// the base properties of the same name.
// Use `StaticProperties` or `SharedProperties`, depending on which variant is needed.
// `StaticOrSharedProperties` exists only to avoid duplication.
type StaticOrSharedProperties struct {
	Srcs []string `android:"path,arch_variant"`

	Sanitized Sanitized `android:"arch_variant"`

	Cflags []string `android:"arch_variant"`

	Enabled            *bool    `android:"arch_variant"`
	Whole_static_libs  []string `android:"arch_variant"`
	Static_libs        []string `android:"arch_variant"`
	Shared_libs        []string `android:"arch_variant"`
	System_shared_libs []string `android:"arch_variant"`

	Export_shared_lib_headers []string `android:"arch_variant"`
	Export_static_lib_headers []string `android:"arch_variant"`

	Apex_available []string `android:"arch_variant"`
}

type LibraryMutatedProperties struct {
	// Build a static variant
	BuildStatic bool `blueprint:"mutated"`
	// Build a shared variant
	BuildShared bool `blueprint:"mutated"`
	// This variant is shared
	VariantIsShared bool `blueprint:"mutated"`
	// This variant is static
	VariantIsStatic bool `blueprint:"mutated"`

	// This variant is a stubs lib
	BuildStubs bool `blueprint:"mutated"`
	// Version of the stubs lib
	StubsVersion string `blueprint:"mutated"`
	// List of all stubs versions associated with an implementation lib
	AllStubsVersions []string `blueprint:"mutated"`
}

type FlagExporterProperties struct {
	// list of directories relative to the Blueprints file that will
	// be added to the include path (using -I) for this module and any module that links
	// against this module.  Directories listed in export_include_dirs do not need to be
	// listed in local_include_dirs.
	Export_include_dirs []string `android:"arch_variant"`

	// list of directories that will be added to the system include path
	// using -isystem for this module and any module that links against this module.
	Export_system_include_dirs []string `android:"arch_variant"`

	Target struct {
		Vendor, Product struct {
			// list of exported include directories, like
			// export_include_dirs, that will be applied to
			// vendor or product variant of this library.
			// This will overwrite any other declarations.
			Override_export_include_dirs []string
		}
	}
}

func init() {
	RegisterLibraryBuildComponents(android.InitRegistrationContext)
}

func RegisterLibraryBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_library_static", LibraryStaticFactory)
	ctx.RegisterModuleType("cc_library_shared", LibrarySharedFactory)
	ctx.RegisterModuleType("cc_library", LibraryFactory)
	ctx.RegisterModuleType("cc_library_host_static", LibraryHostStaticFactory)
	ctx.RegisterModuleType("cc_library_host_shared", LibraryHostSharedFactory)
}

// cc_library creates both static and/or shared libraries for a device and/or
// host. By default, a cc_library has a single variant that targets the device.
// Specifying `host_supported: true` also creates a library that targets the
// host.
func LibraryFactory() android.Module {
	module, _ := NewLibrary(android.HostAndDeviceSupported)
	// Can be used as both a static and a shared library.
	module.sdkMemberTypes = []android.SdkMemberType{
		sharedLibrarySdkMemberType,
		staticLibrarySdkMemberType,
		staticAndSharedLibrarySdkMemberType,
	}
	return module.Init()
}

// cc_library_static creates a static library for a device and/or host binary.
func LibraryStaticFactory() android.Module {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyStatic()
	module.sdkMemberTypes = []android.SdkMemberType{staticLibrarySdkMemberType}
	return module.Init()
}

// cc_library_shared creates a shared library for a device and/or host.
func LibrarySharedFactory() android.Module {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.BuildOnlyShared()
	module.sdkMemberTypes = []android.SdkMemberType{sharedLibrarySdkMemberType}
	return module.Init()
}

// cc_library_host_static creates a static library that is linkable to a host
// binary.
func LibraryHostStaticFactory() android.Module {
	module, library := NewLibrary(android.HostSupported)
	library.BuildOnlyStatic()
	module.sdkMemberTypes = []android.SdkMemberType{staticLibrarySdkMemberType}
	return module.Init()
}

// cc_library_host_shared creates a shared library that is usable on a host.
func LibraryHostSharedFactory() android.Module {
	module, library := NewLibrary(android.HostSupported)
	library.BuildOnlyShared()
	module.sdkMemberTypes = []android.SdkMemberType{sharedLibrarySdkMemberType}
	return module.Init()
}

// flagExporter is a separated portion of libraryDecorator pertaining to exported
// include paths and flags. Keeping this dependency-related information separate
// from the rest of library information is helpful in keeping data more structured
// and explicit.
type flagExporter struct {
	Properties FlagExporterProperties

	dirs       android.Paths // Include directories to be included with -I
	systemDirs android.Paths // System include directories to be included with -isystem
	flags      []string      // Exported raw flags.
	deps       android.Paths
	headers    android.Paths
}

// exportedIncludes returns the effective include paths for this module and
// any module that links against this module. This is obtained from
// the export_include_dirs property in the appropriate target stanza.
func (f *flagExporter) exportedIncludes(ctx ModuleContext) android.Paths {
	if ctx.inVendor() && f.Properties.Target.Vendor.Override_export_include_dirs != nil {
		return android.PathsForModuleSrc(ctx, f.Properties.Target.Vendor.Override_export_include_dirs)
	}
	if ctx.inProduct() && f.Properties.Target.Product.Override_export_include_dirs != nil {
		return android.PathsForModuleSrc(ctx, f.Properties.Target.Product.Override_export_include_dirs)
	}
	return android.PathsForModuleSrc(ctx, f.Properties.Export_include_dirs)
}

// exportIncludes registers the include directories and system include directories to be exported
// transitively to modules depending on this module.
func (f *flagExporter) exportIncludes(ctx ModuleContext) {
	f.dirs = append(f.dirs, f.exportedIncludes(ctx)...)
	f.systemDirs = append(f.systemDirs, android.PathsForModuleSrc(ctx, f.Properties.Export_system_include_dirs)...)
}

// exportIncludesAsSystem registers the include directories and system include directories to be
// exported transitively both as system include directories to modules depending on this module.
func (f *flagExporter) exportIncludesAsSystem(ctx ModuleContext) {
	// all dirs are force exported as system
	f.systemDirs = append(f.systemDirs, f.exportedIncludes(ctx)...)
	f.systemDirs = append(f.systemDirs, android.PathsForModuleSrc(ctx, f.Properties.Export_system_include_dirs)...)
}

// reexportDirs registers the given directories as include directories to be exported transitively
// to modules depending on this module.
func (f *flagExporter) reexportDirs(dirs ...android.Path) {
	f.dirs = append(f.dirs, dirs...)
}

// reexportSystemDirs registers the given directories as system include directories
// to be exported transitively to modules depending on this module.
func (f *flagExporter) reexportSystemDirs(dirs ...android.Path) {
	f.systemDirs = append(f.systemDirs, dirs...)
}

// reexportFlags registers the flags to be exported transitively to modules depending on this
// module.
func (f *flagExporter) reexportFlags(flags ...string) {
	if android.PrefixInList(flags, "-I") || android.PrefixInList(flags, "-isystem") {
		panic(fmt.Errorf("Exporting invalid flag %q: "+
			"use reexportDirs or reexportSystemDirs to export directories", flag))
	}
	f.flags = append(f.flags, flags...)
}

func (f *flagExporter) reexportDeps(deps ...android.Path) {
	f.deps = append(f.deps, deps...)
}

// addExportedGeneratedHeaders does nothing but collects generated header files.
// This can be differ to exportedDeps which may contain phony files to minimize ninja.
func (f *flagExporter) addExportedGeneratedHeaders(headers ...android.Path) {
	f.headers = append(f.headers, headers...)
}

func (f *flagExporter) setProvider(ctx android.ModuleContext) {
	ctx.SetProvider(FlagExporterInfoProvider, FlagExporterInfo{
		IncludeDirs:       f.dirs,
		SystemIncludeDirs: f.systemDirs,
		Flags:             f.flags,
		Deps:              f.deps,
		GeneratedHeaders:  f.headers,
	})
}

// libraryDecorator wraps baseCompiler, baseLinker and baseInstaller to provide library-specific
// functionality: static vs. shared linkage, reusing object files for shared libraries
type libraryDecorator struct {
	Properties        LibraryProperties
	StaticProperties  StaticProperties
	SharedProperties  SharedProperties
	MutatedProperties LibraryMutatedProperties

	// For reusing static library objects for shared library
	reuseObjects Objects

	// table-of-contents file to optimize out relinking when possible
	tocFile android.OptionalPath

	flagExporter
	stripper Stripper

	// For whole_static_libs
	objects Objects

	// Uses the module's name if empty, but can be overridden. Does not include
	// shlib suffix.
	libName string

	sabi *sabi

	// Output archive of gcno coverage information files
	coverageOutputFile android.OptionalPath

	// linked Source Abi Dump
	sAbiOutputFile android.OptionalPath

	// Source Abi Diff
	sAbiDiff android.OptionalPath

	// Location of the static library in the sysroot. Empty if the library is
	// not included in the NDK.
	ndkSysrootPath android.Path

	// Location of the linked, unstripped library for shared libraries
	unstrippedOutputFile android.Path

	// Location of the file that should be copied to dist dir when requested
	distFile android.Path

	versionScriptPath android.OptionalPath

	postInstallCmds []string

	// If useCoreVariant is true, the vendor variant of a VNDK library is
	// not installed.
	useCoreVariant       bool
	checkSameCoreVariant bool

	skipAPIDefine bool

	// Decorated interfaces
	*baseCompiler
	*baseLinker
	*baseInstaller

	collectedSnapshotHeaders android.Paths
}

// collectHeadersForSnapshot collects all exported headers from library.
// It globs header files in the source tree for exported include directories,
// and tracks generated header files separately.
//
// This is to be called from GenerateAndroidBuildActions, and then collected
// header files can be retrieved by snapshotHeaders().
func (l *libraryDecorator) collectHeadersForSnapshot(ctx android.ModuleContext) {
	ret := android.Paths{}

	// Headers in the source tree should be globbed. On the contrast, generated headers
	// can't be globbed, and they should be manually collected.
	// So, we first filter out intermediate directories (which contains generated headers)
	// from exported directories, and then glob headers under remaining directories.
	for _, path := range append(android.CopyOfPaths(l.flagExporter.dirs), l.flagExporter.systemDirs...) {
		dir := path.String()
		// Skip if dir is for generated headers
		if strings.HasPrefix(dir, android.PathForOutput(ctx).String()) {
			continue
		}
		// libeigen wrongly exports the root directory "external/eigen". But only two
		// subdirectories "Eigen" and "unsupported" contain exported header files. Even worse
		// some of them have no extension. So we need special treatment for libeigen in order
		// to glob correctly.
		if dir == "external/eigen" {
			// Only these two directories contains exported headers.
			for _, subdir := range []string{"Eigen", "unsupported/Eigen"} {
				glob, err := ctx.GlobWithDeps("external/eigen/"+subdir+"/**/*", nil)
				if err != nil {
					ctx.ModuleErrorf("glob failed: %#v", err)
					return
				}
				for _, header := range glob {
					if strings.HasSuffix(header, "/") {
						continue
					}
					ext := filepath.Ext(header)
					if ext != "" && ext != ".h" {
						continue
					}
					ret = append(ret, android.PathForSource(ctx, header))
				}
			}
			continue
		}
		glob, err := ctx.GlobWithDeps(dir+"/**/*", nil)
		if err != nil {
			ctx.ModuleErrorf("glob failed: %#v", err)
			return
		}
		isLibcxx := strings.HasPrefix(dir, "external/libcxx/include")
		j := 0
		for i, header := range glob {
			if isLibcxx {
				// Glob all files under this special directory, because of C++ headers with no
				// extension.
				if !strings.HasSuffix(header, "/") {
					continue
				}
			} else {
				// Filter out only the files with extensions that are headers.
				found := false
				for _, ext := range headerExts {
					if strings.HasSuffix(header, ext) {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			if i != j {
				glob[j] = glob[i]
			}
			j++
		}
		glob = glob[:j]
	}

	// Collect generated headers
	for _, header := range append(android.CopyOfPaths(l.flagExporter.headers), l.flagExporter.deps...) {
		// TODO(b/148123511): remove exportedDeps after cleaning up genrule
		if strings.HasSuffix(header.Base(), "-phony") {
			continue
		}
		ret = append(ret, header)
	}

	l.collectedSnapshotHeaders = ret
}

// This returns all exported header files, both generated ones and headers from source tree.
// collectHeadersForSnapshot() must be called before calling this.
func (l *libraryDecorator) snapshotHeaders() android.Paths {
	if l.collectedSnapshotHeaders == nil {
		panic("snapshotHeaders() must be called after collectHeadersForSnapshot()")
	}
	return l.collectedSnapshotHeaders
}

// linkerProps returns the list of properties structs relevant for this library. (For example, if
// the library is cc_shared_library, then static-library properties are omitted.)
func (library *libraryDecorator) linkerProps() []interface{} {
	var props []interface{}
	props = append(props, library.baseLinker.linkerProps()...)
	props = append(props,
		&library.Properties,
		&library.MutatedProperties,
		&library.flagExporter.Properties,
		&library.stripper.StripProperties)

	if library.MutatedProperties.BuildShared {
		props = append(props, &library.SharedProperties)
	}
	if library.MutatedProperties.BuildStatic {
		props = append(props, &library.StaticProperties)
	}

	return props
}

// linkerFlags takes a Flags struct and augments it to contain linker flags that are defined by this
// library, or that are implied by attributes of this library (such as whether this library is a
// shared library).
func (library *libraryDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = library.baseLinker.linkerFlags(ctx, flags)

	// MinGW spits out warnings about -fPIC even for -fpie?!) being ignored because
	// all code is position independent, and then those warnings get promoted to
	// errors.
	if !ctx.Windows() {
		flags.Global.CFlags = append(flags.Global.CFlags, "-fPIC")
	}

	if library.static() {
		flags.Local.CFlags = append(flags.Local.CFlags, library.StaticProperties.Static.Cflags...)
	} else if library.shared() {
		flags.Local.CFlags = append(flags.Local.CFlags, library.SharedProperties.Shared.Cflags...)
	}

	if library.shared() {
		libName := library.getLibName(ctx)
		var f []string
		if ctx.toolchain().Bionic() {
			f = append(f,
				"-nostdlib",
				"-Wl,--gc-sections",
			)
		}

		if ctx.Darwin() {
			f = append(f,
				"-dynamiclib",
				"-single_module",
				"-install_name @rpath/"+libName+flags.Toolchain.ShlibSuffix(),
			)
			if ctx.Arch().ArchType == android.X86 {
				f = append(f,
					"-read_only_relocs suppress",
				)
			}
		} else {
			f = append(f, "-shared")
			if !ctx.Windows() {
				f = append(f, "-Wl,-soname,"+libName+flags.Toolchain.ShlibSuffix())
			}
		}

		flags.Global.LdFlags = append(flags.Global.LdFlags, f...)
	}

	return flags
}

// compilerFlags takes a Flags and augments it to contain compile flags from global values,
// per-target values, module type values, per-module Blueprints properties, extra flags from
// `flags`, and generated sources from `deps`.
func (library *libraryDecorator) compilerFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags {
	exportIncludeDirs := library.flagExporter.exportedIncludes(ctx)
	if len(exportIncludeDirs) > 0 {
		f := includeDirsToFlags(exportIncludeDirs)
		flags.Local.CommonFlags = append(flags.Local.CommonFlags, f)
		flags.Local.YasmFlags = append(flags.Local.YasmFlags, f)
	}

	flags = library.baseCompiler.compilerFlags(ctx, flags, deps)
	if ctx.IsLlndk() {
		// LLNDK libraries ignore most of the properties on the cc_library and use the
		// LLNDK-specific properties instead.
		// Wipe all the module-local properties, leaving only the global properties.
		flags.Local = LocalOrGlobalFlags{}
	}
	if library.buildStubs() {
		// Remove -include <file> when compiling stubs. Otherwise, the force included
		// headers might cause conflicting types error with the symbols in the
		// generated stubs source code. e.g.
		// double acos(double); // in header
		// void acos() {} // in the generated source code
		removeInclude := func(flags []string) []string {
			ret := flags[:0]
			for _, f := range flags {
				if strings.HasPrefix(f, "-include ") {
					continue
				}
				ret = append(ret, f)
			}
			return ret
		}
		flags.Local.CommonFlags = removeInclude(flags.Local.CommonFlags)
		flags.Local.CFlags = removeInclude(flags.Local.CFlags)

		flags = addStubLibraryCompilerFlags(flags)
	}
	return flags
}

func (library *libraryDecorator) headerAbiCheckerEnabled() bool {
	return Bool(library.Properties.Header_abi_checker.Enabled)
}

func (library *libraryDecorator) headerAbiCheckerExplicitlyDisabled() bool {
	return !BoolDefault(library.Properties.Header_abi_checker.Enabled, true)
}

func (library *libraryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	if ctx.IsLlndk() {
		// This is the vendor variant of an LLNDK library, build the LLNDK stubs.
		vndkVer := ctx.Module().(*Module).VndkVersion()
		if !inList(vndkVer, ctx.Config().PlatformVersionActiveCodenames()) || vndkVer == "" {
			// For non-enforcing devices, vndkVer is empty. Use "current" in that case, too.
			vndkVer = "current"
		}
		if library.stubsVersion() != "" {
			vndkVer = library.stubsVersion()
		}
		objs, versionScript := compileStubLibrary(ctx, flags, String(library.Properties.Llndk.Symbol_file), vndkVer, "--llndk")
		if !Bool(library.Properties.Llndk.Unversioned) {
			library.versionScriptPath = android.OptionalPathForPath(versionScript)
		}
		return objs
	}
	if library.buildStubs() {
		objs, versionScript := compileStubLibrary(ctx, flags, String(library.Properties.Stubs.Symbol_file), library.MutatedProperties.StubsVersion, "--apex")
		library.versionScriptPath = android.OptionalPathForPath(versionScript)
		return objs
	}

	if !library.buildShared() && !library.buildStatic() {
		if len(library.baseCompiler.Properties.Srcs) > 0 {
			ctx.PropertyErrorf("srcs", "cc_library_headers must not have any srcs")
		}
		if len(library.StaticProperties.Static.Srcs) > 0 {
			ctx.PropertyErrorf("static.srcs", "cc_library_headers must not have any srcs")
		}
		if len(library.SharedProperties.Shared.Srcs) > 0 {
			ctx.PropertyErrorf("shared.srcs", "cc_library_headers must not have any srcs")
		}
		return Objects{}
	}
	if library.sabi.shouldCreateSourceAbiDump() {
		exportIncludeDirs := library.flagExporter.exportedIncludes(ctx)
		var SourceAbiFlags []string
		for _, dir := range exportIncludeDirs.Strings() {
			SourceAbiFlags = append(SourceAbiFlags, "-I"+dir)
		}
		for _, reexportedInclude := range library.sabi.Properties.ReexportedIncludes {
			SourceAbiFlags = append(SourceAbiFlags, "-I"+reexportedInclude)
		}
		flags.SAbiFlags = SourceAbiFlags
		totalLength := len(library.baseCompiler.Properties.Srcs) + len(deps.GeneratedSources) +
			len(library.SharedProperties.Shared.Srcs) + len(library.StaticProperties.Static.Srcs)
		if totalLength > 0 {
			flags.SAbiDump = true
		}
	}
	objs := library.baseCompiler.compile(ctx, flags, deps)
	library.reuseObjects = objs
	buildFlags := flagsToBuilderFlags(flags)

	if library.static() {
		srcs := android.PathsForModuleSrc(ctx, library.StaticProperties.Static.Srcs)
		objs = objs.Append(compileObjs(ctx, buildFlags, android.DeviceStaticLibrary,
			srcs, library.baseCompiler.pathDeps, library.baseCompiler.cFlagsDeps))
	} else if library.shared() {
		srcs := android.PathsForModuleSrc(ctx, library.SharedProperties.Shared.Srcs)
		objs = objs.Append(compileObjs(ctx, buildFlags, android.DeviceSharedLibrary,
			srcs, library.baseCompiler.pathDeps, library.baseCompiler.cFlagsDeps))
	}

	return objs
}

type libraryInterface interface {
	versionedInterface

	static() bool
	shared() bool
	objs() Objects
	reuseObjs() Objects
	toc() android.OptionalPath

	// Returns true if the build options for the module have selected a static or shared build
	buildStatic() bool
	buildShared() bool

	// Sets whether a specific variant is static or shared
	setStatic()
	setShared()

	// Check whether header_abi_checker is enabled or explicitly disabled.
	headerAbiCheckerEnabled() bool
	headerAbiCheckerExplicitlyDisabled() bool

	// Write LOCAL_ADDITIONAL_DEPENDENCIES for ABI diff
	androidMkWriteAdditionalDependenciesForSourceAbiDiff(w io.Writer)

	availableFor(string) bool
}

type versionedInterface interface {
	buildStubs() bool
	setBuildStubs()
	hasStubsVariants() bool
	setStubsVersion(string)
	stubsVersion() string

	stubsVersions(ctx android.BaseMutatorContext) []string
	setAllStubsVersions([]string)
	allStubsVersions() []string

	implementationModuleName(name string) string
	hasLLNDKStubs() bool
}

var _ libraryInterface = (*libraryDecorator)(nil)
var _ versionedInterface = (*libraryDecorator)(nil)

func (library *libraryDecorator) getLibNameHelper(baseModuleName string, inVendor bool, inProduct bool) string {
	name := library.libName
	if name == "" {
		name = String(library.Properties.Stem)
		if name == "" {
			name = baseModuleName
		}
	}

	suffix := ""
	if inVendor {
		suffix = String(library.Properties.Target.Vendor.Suffix)
	} else if inProduct {
		suffix = String(library.Properties.Target.Product.Suffix)
	}
	if suffix == "" {
		suffix = String(library.Properties.Suffix)
	}

	return name + suffix
}

// getLibName returns the actual canonical name of the library (the name which
// should be passed to the linker via linker flags).
func (library *libraryDecorator) getLibName(ctx BaseModuleContext) string {
	name := library.getLibNameHelper(ctx.baseModuleName(), ctx.inVendor(), ctx.inProduct())

	if ctx.IsVndkExt() {
		// vndk-ext lib should have the same name with original lib
		ctx.VisitDirectDepsWithTag(vndkExtDepTag, func(module android.Module) {
			originalName := module.(*Module).outputFile.Path()
			name = strings.TrimSuffix(originalName.Base(), originalName.Ext())
		})
	}

	if ctx.Host() && Bool(library.Properties.Unique_host_soname) {
		if !strings.HasSuffix(name, "-host") {
			name = name + "-host"
		}
	}

	return name
}

var versioningMacroNamesListMutex sync.Mutex

func (library *libraryDecorator) linkerInit(ctx BaseModuleContext) {
	location := InstallInSystem
	if library.baseLinker.sanitize.inSanitizerDir() {
		location = InstallInSanitizerDir
	}
	library.baseInstaller.location = location
	library.baseLinker.linkerInit(ctx)
	// Let baseLinker know whether this variant is for stubs or not, so that
	// it can omit things that are not required for linking stubs.
	library.baseLinker.dynamicProperties.BuildStubs = library.buildStubs()

	if library.buildStubs() {
		macroNames := versioningMacroNamesList(ctx.Config())
		myName := versioningMacroName(ctx.ModuleName())
		versioningMacroNamesListMutex.Lock()
		defer versioningMacroNamesListMutex.Unlock()
		if (*macroNames)[myName] == "" {
			(*macroNames)[myName] = ctx.ModuleName()
		} else if (*macroNames)[myName] != ctx.ModuleName() {
			ctx.ModuleErrorf("Macro name %q for versioning conflicts with macro name from module %q ", myName, (*macroNames)[myName])
		}
	}
}

func (library *libraryDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	if ctx.IsLlndk() {
		// LLNDK libraries ignore most of the properties on the cc_library and use the
		// LLNDK-specific properties instead.
		return deps
	}

	deps = library.baseCompiler.compilerDeps(ctx, deps)

	return deps
}

func (library *libraryDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	if ctx.IsLlndk() {
		// LLNDK libraries ignore most of the properties on the cc_library and use the
		// LLNDK-specific properties instead.
		deps.HeaderLibs = append(deps.HeaderLibs, library.Properties.Llndk.Export_llndk_headers...)
		deps.ReexportHeaderLibHeaders = append(deps.ReexportHeaderLibHeaders,
			library.Properties.Llndk.Export_llndk_headers...)
		return deps
	}

	if library.static() {
		// Compare with nil because an empty list needs to be propagated.
		if library.StaticProperties.Static.System_shared_libs != nil {
			library.baseLinker.Properties.System_shared_libs = library.StaticProperties.Static.System_shared_libs
		}
	} else if library.shared() {
		// Compare with nil because an empty list needs to be propagated.
		if library.SharedProperties.Shared.System_shared_libs != nil {
			library.baseLinker.Properties.System_shared_libs = library.SharedProperties.Shared.System_shared_libs
		}
	}

	deps = library.baseLinker.linkerDeps(ctx, deps)

	if library.static() {
		deps.WholeStaticLibs = append(deps.WholeStaticLibs,
			library.StaticProperties.Static.Whole_static_libs...)
		deps.StaticLibs = append(deps.StaticLibs, library.StaticProperties.Static.Static_libs...)
		deps.SharedLibs = append(deps.SharedLibs, library.StaticProperties.Static.Shared_libs...)

		deps.ReexportSharedLibHeaders = append(deps.ReexportSharedLibHeaders, library.StaticProperties.Static.Export_shared_lib_headers...)
		deps.ReexportStaticLibHeaders = append(deps.ReexportStaticLibHeaders, library.StaticProperties.Static.Export_static_lib_headers...)
	} else if library.shared() {
		if ctx.toolchain().Bionic() && !Bool(library.baseLinker.Properties.Nocrt) {
			deps.CrtBegin = "crtbegin_so"
			deps.CrtEnd = "crtend_so"
		}
		deps.WholeStaticLibs = append(deps.WholeStaticLibs, library.SharedProperties.Shared.Whole_static_libs...)
		deps.StaticLibs = append(deps.StaticLibs, library.SharedProperties.Shared.Static_libs...)
		deps.SharedLibs = append(deps.SharedLibs, library.SharedProperties.Shared.Shared_libs...)

		deps.ReexportSharedLibHeaders = append(deps.ReexportSharedLibHeaders, library.SharedProperties.Shared.Export_shared_lib_headers...)
		deps.ReexportStaticLibHeaders = append(deps.ReexportStaticLibHeaders, library.SharedProperties.Shared.Export_static_lib_headers...)
	}
	if ctx.inVendor() {
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, library.baseLinker.Properties.Target.Vendor.Exclude_static_libs)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, library.baseLinker.Properties.Target.Vendor.Exclude_shared_libs)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, library.baseLinker.Properties.Target.Vendor.Exclude_static_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, library.baseLinker.Properties.Target.Vendor.Exclude_shared_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, library.baseLinker.Properties.Target.Vendor.Exclude_static_libs)
	}
	if ctx.inProduct() {
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, library.baseLinker.Properties.Target.Product.Exclude_static_libs)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, library.baseLinker.Properties.Target.Product.Exclude_shared_libs)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, library.baseLinker.Properties.Target.Product.Exclude_static_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, library.baseLinker.Properties.Target.Product.Exclude_shared_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, library.baseLinker.Properties.Target.Product.Exclude_static_libs)
	}
	if ctx.inRecovery() {
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, library.baseLinker.Properties.Target.Recovery.Exclude_static_libs)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, library.baseLinker.Properties.Target.Recovery.Exclude_shared_libs)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, library.baseLinker.Properties.Target.Recovery.Exclude_static_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, library.baseLinker.Properties.Target.Recovery.Exclude_shared_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, library.baseLinker.Properties.Target.Recovery.Exclude_static_libs)
	}
	if ctx.inRamdisk() {
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, library.baseLinker.Properties.Target.Ramdisk.Exclude_static_libs)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, library.baseLinker.Properties.Target.Ramdisk.Exclude_shared_libs)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, library.baseLinker.Properties.Target.Ramdisk.Exclude_static_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, library.baseLinker.Properties.Target.Ramdisk.Exclude_shared_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, library.baseLinker.Properties.Target.Ramdisk.Exclude_static_libs)
	}
	if ctx.inVendorRamdisk() {
		deps.WholeStaticLibs = removeListFromList(deps.WholeStaticLibs, library.baseLinker.Properties.Target.Vendor_ramdisk.Exclude_static_libs)
		deps.SharedLibs = removeListFromList(deps.SharedLibs, library.baseLinker.Properties.Target.Vendor_ramdisk.Exclude_shared_libs)
		deps.StaticLibs = removeListFromList(deps.StaticLibs, library.baseLinker.Properties.Target.Vendor_ramdisk.Exclude_static_libs)
		deps.ReexportSharedLibHeaders = removeListFromList(deps.ReexportSharedLibHeaders, library.baseLinker.Properties.Target.Vendor_ramdisk.Exclude_shared_libs)
		deps.ReexportStaticLibHeaders = removeListFromList(deps.ReexportStaticLibHeaders, library.baseLinker.Properties.Target.Vendor_ramdisk.Exclude_static_libs)
	}

	return deps
}

func (library *libraryDecorator) linkerSpecifiedDeps(specifiedDeps specifiedDeps) specifiedDeps {
	specifiedDeps = library.baseLinker.linkerSpecifiedDeps(specifiedDeps)
	var properties StaticOrSharedProperties
	if library.static() {
		properties = library.StaticProperties.Static
	} else if library.shared() {
		properties = library.SharedProperties.Shared
	}

	specifiedDeps.sharedLibs = append(specifiedDeps.sharedLibs, properties.Shared_libs...)

	// Must distinguish nil and [] in system_shared_libs - ensure that [] in
	// either input list doesn't come out as nil.
	if specifiedDeps.systemSharedLibs == nil {
		specifiedDeps.systemSharedLibs = properties.System_shared_libs
	} else {
		specifiedDeps.systemSharedLibs = append(specifiedDeps.systemSharedLibs, properties.System_shared_libs...)
	}

	specifiedDeps.sharedLibs = android.FirstUniqueStrings(specifiedDeps.sharedLibs)
	if len(specifiedDeps.systemSharedLibs) > 0 {
		// Skip this if systemSharedLibs is either nil or [], to ensure they are
		// retained.
		specifiedDeps.systemSharedLibs = android.FirstUniqueStrings(specifiedDeps.systemSharedLibs)
	}
	return specifiedDeps
}

func (library *libraryDecorator) linkStatic(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	library.objects = deps.WholeStaticLibObjs.Copy()
	library.objects = library.objects.Append(objs)

	fileName := ctx.ModuleName() + staticLibraryExtension
	outputFile := android.PathForModuleOut(ctx, fileName)
	builderFlags := flagsToBuilderFlags(flags)

	if Bool(library.baseLinker.Properties.Use_version_lib) {
		if ctx.Host() {
			versionedOutputFile := outputFile
			outputFile = android.PathForModuleOut(ctx, "unversioned", fileName)
			library.injectVersionSymbol(ctx, outputFile, versionedOutputFile)
		} else {
			versionedOutputFile := android.PathForModuleOut(ctx, "versioned", fileName)
			library.distFile = versionedOutputFile
			library.injectVersionSymbol(ctx, outputFile, versionedOutputFile)
		}
	}

	transformObjToStaticLib(ctx, library.objects.objFiles, deps.WholeStaticLibsFromPrebuilts, builderFlags, outputFile, objs.tidyFiles)

	library.coverageOutputFile = transformCoverageFilesToZip(ctx, library.objects, ctx.ModuleName())

	ctx.CheckbuildFile(outputFile)

	if library.static() {
		ctx.SetProvider(StaticLibraryInfoProvider, StaticLibraryInfo{
			StaticLibrary: outputFile,
			ReuseObjects:  library.reuseObjects,
			Objects:       library.objects,

			TransitiveStaticLibrariesForOrdering: android.NewDepSetBuilder(android.TOPOLOGICAL).
				Direct(outputFile).
				Transitive(deps.TranstiveStaticLibrariesForOrdering).
				Build(),
		})
	}

	if library.header() {
		ctx.SetProvider(HeaderLibraryInfoProvider, HeaderLibraryInfo{})
	}

	return outputFile
}

func (library *libraryDecorator) linkShared(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	var linkerDeps android.Paths
	linkerDeps = append(linkerDeps, flags.LdFlagsDeps...)

	unexportedSymbols := ctx.ExpandOptionalSource(library.Properties.Unexported_symbols_list, "unexported_symbols_list")
	forceNotWeakSymbols := ctx.ExpandOptionalSource(library.Properties.Force_symbols_not_weak_list, "force_symbols_not_weak_list")
	forceWeakSymbols := ctx.ExpandOptionalSource(library.Properties.Force_symbols_weak_list, "force_symbols_weak_list")
	if !ctx.Darwin() {
		if unexportedSymbols.Valid() {
			ctx.PropertyErrorf("unexported_symbols_list", "Only supported on Darwin")
		}
		if forceNotWeakSymbols.Valid() {
			ctx.PropertyErrorf("force_symbols_not_weak_list", "Only supported on Darwin")
		}
		if forceWeakSymbols.Valid() {
			ctx.PropertyErrorf("force_symbols_weak_list", "Only supported on Darwin")
		}
	} else {
		if unexportedSymbols.Valid() {
			flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,-unexported_symbols_list,"+unexportedSymbols.String())
			linkerDeps = append(linkerDeps, unexportedSymbols.Path())
		}
		if forceNotWeakSymbols.Valid() {
			flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,-force_symbols_not_weak_list,"+forceNotWeakSymbols.String())
			linkerDeps = append(linkerDeps, forceNotWeakSymbols.Path())
		}
		if forceWeakSymbols.Valid() {
			flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,-force_symbols_weak_list,"+forceWeakSymbols.String())
			linkerDeps = append(linkerDeps, forceWeakSymbols.Path())
		}
	}
	if library.versionScriptPath.Valid() {
		linkerScriptFlags := "-Wl,--version-script," + library.versionScriptPath.String()
		flags.Local.LdFlags = append(flags.Local.LdFlags, linkerScriptFlags)
		linkerDeps = append(linkerDeps, library.versionScriptPath.Path())
	}

	fileName := library.getLibName(ctx) + flags.Toolchain.ShlibSuffix()
	outputFile := android.PathForModuleOut(ctx, fileName)
	unstrippedOutputFile := outputFile

	var implicitOutputs android.WritablePaths
	if ctx.Windows() {
		importLibraryPath := android.PathForModuleOut(ctx, pathtools.ReplaceExtension(fileName, "lib"))

		flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,--out-implib="+importLibraryPath.String())
		implicitOutputs = append(implicitOutputs, importLibraryPath)
	}

	builderFlags := flagsToBuilderFlags(flags)

	// Optimize out relinking against shared libraries whose interface hasn't changed by
	// depending on a table of contents file instead of the library itself.
	tocFile := outputFile.ReplaceExtension(ctx, flags.Toolchain.ShlibSuffix()[1:]+".toc")
	library.tocFile = android.OptionalPathForPath(tocFile)
	transformSharedObjectToToc(ctx, outputFile, tocFile, builderFlags)

	stripFlags := flagsToStripFlags(flags)
	needsStrip := library.stripper.NeedsStrip(ctx)
	if library.buildStubs() {
		// No need to strip stubs libraries
		needsStrip = false
	}
	if needsStrip {
		if ctx.Darwin() {
			stripFlags.StripUseGnuStrip = true
		}
		strippedOutputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unstripped", fileName)
		library.stripper.StripExecutableOrSharedLib(ctx, outputFile, strippedOutputFile, stripFlags)
	}
	library.unstrippedOutputFile = outputFile

	outputFile = maybeInjectBoringSSLHash(ctx, outputFile, library.Properties.Inject_bssl_hash, fileName)

	if Bool(library.baseLinker.Properties.Use_version_lib) {
		if ctx.Host() {
			versionedOutputFile := outputFile
			outputFile = android.PathForModuleOut(ctx, "unversioned", fileName)
			library.injectVersionSymbol(ctx, outputFile, versionedOutputFile)
		} else {
			versionedOutputFile := android.PathForModuleOut(ctx, "versioned", fileName)
			library.distFile = versionedOutputFile

			if library.stripper.NeedsStrip(ctx) {
				out := android.PathForModuleOut(ctx, "versioned-stripped", fileName)
				library.distFile = out
				library.stripper.StripExecutableOrSharedLib(ctx, versionedOutputFile, out, stripFlags)
			}

			library.injectVersionSymbol(ctx, outputFile, versionedOutputFile)
		}
	}

	sharedLibs := deps.EarlySharedLibs
	sharedLibs = append(sharedLibs, deps.SharedLibs...)
	sharedLibs = append(sharedLibs, deps.LateSharedLibs...)

	linkerDeps = append(linkerDeps, deps.EarlySharedLibsDeps...)
	linkerDeps = append(linkerDeps, deps.SharedLibsDeps...)
	linkerDeps = append(linkerDeps, deps.LateSharedLibsDeps...)
	linkerDeps = append(linkerDeps, objs.tidyFiles...)

	if Bool(library.Properties.Sort_bss_symbols_by_size) && !library.buildStubs() {
		unsortedOutputFile := android.PathForModuleOut(ctx, "unsorted", fileName)
		transformObjToDynamicBinary(ctx, objs.objFiles, sharedLibs,
			deps.StaticLibs, deps.LateStaticLibs, deps.WholeStaticLibs,
			linkerDeps, deps.CrtBegin, deps.CrtEnd, false, builderFlags, unsortedOutputFile, implicitOutputs)

		symbolOrderingFile := android.PathForModuleOut(ctx, "unsorted", fileName+".symbol_order")
		symbolOrderingFlag := library.baseLinker.sortBssSymbolsBySize(ctx, unsortedOutputFile, symbolOrderingFile, builderFlags)
		builderFlags.localLdFlags += " " + symbolOrderingFlag
		linkerDeps = append(linkerDeps, symbolOrderingFile)
	}

	transformObjToDynamicBinary(ctx, objs.objFiles, sharedLibs,
		deps.StaticLibs, deps.LateStaticLibs, deps.WholeStaticLibs,
		linkerDeps, deps.CrtBegin, deps.CrtEnd, false, builderFlags, outputFile, implicitOutputs)

	objs.coverageFiles = append(objs.coverageFiles, deps.StaticLibObjs.coverageFiles...)
	objs.coverageFiles = append(objs.coverageFiles, deps.WholeStaticLibObjs.coverageFiles...)

	objs.sAbiDumpFiles = append(objs.sAbiDumpFiles, deps.StaticLibObjs.sAbiDumpFiles...)
	objs.sAbiDumpFiles = append(objs.sAbiDumpFiles, deps.WholeStaticLibObjs.sAbiDumpFiles...)

	library.coverageOutputFile = transformCoverageFilesToZip(ctx, objs, library.getLibName(ctx))
	library.linkSAbiDumpFiles(ctx, objs, fileName, unstrippedOutputFile)

	var staticAnalogue *StaticLibraryInfo
	if static := ctx.GetDirectDepsWithTag(staticVariantTag); len(static) > 0 {
		s := ctx.OtherModuleProvider(static[0], StaticLibraryInfoProvider).(StaticLibraryInfo)
		staticAnalogue = &s
	}

	ctx.SetProvider(SharedLibraryInfoProvider, SharedLibraryInfo{
		TableOfContents:         android.OptionalPathForPath(tocFile),
		SharedLibrary:           unstrippedOutputFile,
		UnstrippedSharedLibrary: library.unstrippedOutputFile,
		CoverageSharedLibrary:   library.coverageOutputFile,
		StaticAnalogue:          staticAnalogue,
	})

	stubs := ctx.GetDirectDepsWithTag(stubImplDepTag)
	if len(stubs) > 0 {
		var stubsInfo []SharedStubLibrary
		for _, stub := range stubs {
			stubInfo := ctx.OtherModuleProvider(stub, SharedLibraryInfoProvider).(SharedLibraryInfo)
			flagInfo := ctx.OtherModuleProvider(stub, FlagExporterInfoProvider).(FlagExporterInfo)
			stubsInfo = append(stubsInfo, SharedStubLibrary{
				Version:           moduleLibraryInterface(stub).stubsVersion(),
				SharedLibraryInfo: stubInfo,
				FlagExporterInfo:  flagInfo,
			})
		}
		ctx.SetProvider(SharedLibraryStubsProvider, SharedLibraryStubsInfo{
			SharedStubLibraries: stubsInfo,

			IsLLNDK: ctx.IsLlndk(),
		})
	}

	return unstrippedOutputFile
}

func (library *libraryDecorator) unstrippedOutputFilePath() android.Path {
	return library.unstrippedOutputFile
}

func (library *libraryDecorator) disableStripping() {
	library.stripper.StripProperties.Strip.None = BoolPtr(true)
}

func (library *libraryDecorator) nativeCoverage() bool {
	if library.header() || library.buildStubs() {
		return false
	}
	return true
}

func (library *libraryDecorator) coverageOutputFilePath() android.OptionalPath {
	return library.coverageOutputFile
}

func getRefAbiDumpFile(ctx ModuleContext, vndkVersion, fileName string) android.Path {
	// The logic must be consistent with classifySourceAbiDump.
	isNdk := ctx.isNdk(ctx.Config())
	isLlndkOrVndk := ctx.IsLlndkPublic() || (ctx.useVndk() && ctx.isVndk())

	refAbiDumpTextFile := android.PathForVndkRefAbiDump(ctx, vndkVersion, fileName, isNdk, isLlndkOrVndk, false)
	refAbiDumpGzipFile := android.PathForVndkRefAbiDump(ctx, vndkVersion, fileName, isNdk, isLlndkOrVndk, true)

	if refAbiDumpTextFile.Valid() {
		if refAbiDumpGzipFile.Valid() {
			ctx.ModuleErrorf(
				"Two reference ABI dump files are found: %q and %q. Please delete the stale one.",
				refAbiDumpTextFile, refAbiDumpGzipFile)
			return nil
		}
		return refAbiDumpTextFile.Path()
	}
	if refAbiDumpGzipFile.Valid() {
		return unzipRefDump(ctx, refAbiDumpGzipFile.Path(), fileName)
	}
	return nil
}

func (library *libraryDecorator) linkSAbiDumpFiles(ctx ModuleContext, objs Objects, fileName string, soFile android.Path) {
	if library.sabi.shouldCreateSourceAbiDump() {
		var vndkVersion string

		if ctx.useVndk() {
			// For modules linking against vndk, follow its vndk version
			vndkVersion = ctx.Module().(*Module).VndkVersion()
		} else {
			// Regard the other modules as PLATFORM_VNDK_VERSION
			vndkVersion = ctx.DeviceConfig().PlatformVndkVersion()
		}

		exportIncludeDirs := library.flagExporter.exportedIncludes(ctx)
		var SourceAbiFlags []string
		for _, dir := range exportIncludeDirs.Strings() {
			SourceAbiFlags = append(SourceAbiFlags, "-I"+dir)
		}
		for _, reexportedInclude := range library.sabi.Properties.ReexportedIncludes {
			SourceAbiFlags = append(SourceAbiFlags, "-I"+reexportedInclude)
		}
		exportedHeaderFlags := strings.Join(SourceAbiFlags, " ")
		library.sAbiOutputFile = transformDumpToLinkedDump(ctx, objs.sAbiDumpFiles, soFile, fileName, exportedHeaderFlags,
			android.OptionalPathForModuleSrc(ctx, library.symbolFileForAbiCheck(ctx)),
			library.Properties.Header_abi_checker.Exclude_symbol_versions,
			library.Properties.Header_abi_checker.Exclude_symbol_tags)

		addLsdumpPath(classifySourceAbiDump(ctx) + ":" + library.sAbiOutputFile.String())

		refAbiDumpFile := getRefAbiDumpFile(ctx, vndkVersion, fileName)
		if refAbiDumpFile != nil {
			library.sAbiDiff = sourceAbiDiff(ctx, library.sAbiOutputFile.Path(),
				refAbiDumpFile, fileName, exportedHeaderFlags,
				Bool(library.Properties.Header_abi_checker.Check_all_apis),
				ctx.IsLlndk(), ctx.isNdk(ctx.Config()), ctx.IsVndkExt())
		}
	}
}

func processLLNDKHeaders(ctx ModuleContext, srcHeaderDir string, outDir android.ModuleGenPath) android.Path {
	srcDir := android.PathForModuleSrc(ctx, srcHeaderDir)
	srcFiles := ctx.GlobFiles(filepath.Join(srcDir.String(), "**/*.h"), nil)

	var installPaths []android.WritablePath
	for _, header := range srcFiles {
		headerDir := filepath.Dir(header.String())
		relHeaderDir, err := filepath.Rel(srcDir.String(), headerDir)
		if err != nil {
			ctx.ModuleErrorf("filepath.Rel(%q, %q) failed: %s",
				srcDir.String(), headerDir, err)
			continue
		}

		installPaths = append(installPaths, outDir.Join(ctx, relHeaderDir, header.Base()))
	}

	return processHeadersWithVersioner(ctx, srcDir, outDir, srcFiles, installPaths)
}

// link registers actions to link this library, and sets various fields
// on this library to reflect information that should be exported up the build
// tree (for example, exported flags and include paths).
func (library *libraryDecorator) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	if ctx.IsLlndk() {
		if len(library.Properties.Llndk.Export_preprocessed_headers) > 0 {
			// This is the vendor variant of an LLNDK library with preprocessed headers.
			genHeaderOutDir := android.PathForModuleGen(ctx, "include")

			var timestampFiles android.Paths
			for _, dir := range library.Properties.Llndk.Export_preprocessed_headers {
				timestampFiles = append(timestampFiles, processLLNDKHeaders(ctx, dir, genHeaderOutDir))
			}

			if Bool(library.Properties.Llndk.Export_headers_as_system) {
				library.reexportSystemDirs(genHeaderOutDir)
			} else {
				library.reexportDirs(genHeaderOutDir)
			}

			library.reexportDeps(timestampFiles...)
		}

		if Bool(library.Properties.Llndk.Export_headers_as_system) {
			library.flagExporter.Properties.Export_system_include_dirs = append(
				library.flagExporter.Properties.Export_system_include_dirs,
				library.flagExporter.Properties.Export_include_dirs...)
			library.flagExporter.Properties.Export_include_dirs = nil
		}
	}

	// Linking this library consists of linking `deps.Objs` (.o files in dependencies
	// of this library), together with `objs` (.o files created by compiling this
	// library).
	objs = deps.Objs.Copy().Append(objs)
	var out android.Path
	if library.static() || library.header() {
		out = library.linkStatic(ctx, flags, deps, objs)
	} else {
		out = library.linkShared(ctx, flags, deps, objs)
	}

	// Export include paths and flags to be propagated up the tree.
	library.exportIncludes(ctx)
	library.reexportDirs(deps.ReexportedDirs...)
	library.reexportSystemDirs(deps.ReexportedSystemDirs...)
	library.reexportFlags(deps.ReexportedFlags...)
	library.reexportDeps(deps.ReexportedDeps...)
	library.addExportedGeneratedHeaders(deps.ReexportedGeneratedHeaders...)

	// Optionally export aidl headers.
	if Bool(library.Properties.Aidl.Export_aidl_headers) {
		if library.baseCompiler.hasSrcExt(".aidl") {
			dir := android.PathForModuleGen(ctx, "aidl")
			library.reexportDirs(dir)

			// TODO: restrict to aidl deps
			library.reexportDeps(library.baseCompiler.pathDeps...)
			library.addExportedGeneratedHeaders(library.baseCompiler.pathDeps...)
		}
	}

	// Optionally export proto headers.
	if Bool(library.Properties.Proto.Export_proto_headers) {
		if library.baseCompiler.hasSrcExt(".proto") {
			var includes android.Paths
			if flags.proto.CanonicalPathFromRoot {
				includes = append(includes, flags.proto.SubDir)
			}
			includes = append(includes, flags.proto.Dir)
			library.reexportDirs(includes...)

			// TODO: restrict to proto deps
			library.reexportDeps(library.baseCompiler.pathDeps...)
			library.addExportedGeneratedHeaders(library.baseCompiler.pathDeps...)
		}
	}

	// If the library is sysprop_library, expose either public or internal header selectively.
	if library.baseCompiler.hasSrcExt(".sysprop") {
		dir := android.PathForModuleGen(ctx, "sysprop", "include")
		if library.Properties.Sysprop.Platform != nil {
			isOwnerPlatform := Bool(library.Properties.Sysprop.Platform)

			// If the owner is different from the user, expose public header. That is,
			// 1) if the user is product (as owner can only be platform / vendor)
			// 2) if the owner is platform and the client is vendor
			// We don't care Platform -> Vendor dependency as it's already forbidden.
			if ctx.Device() && (ctx.ProductSpecific() || (isOwnerPlatform && ctx.inVendor())) {
				dir = android.PathForModuleGen(ctx, "sysprop/public", "include")
			}
		}

		// Add sysprop-related directories to the exported directories of this library.
		library.reexportDirs(dir)
		library.reexportDeps(library.baseCompiler.pathDeps...)
		library.addExportedGeneratedHeaders(library.baseCompiler.pathDeps...)
	}

	// Add stub-related flags if this library is a stub library.
	library.exportVersioningMacroIfNeeded(ctx)

	// Propagate a Provider containing information about exported flags, deps, and include paths.
	library.flagExporter.setProvider(ctx)

	return out
}

func (library *libraryDecorator) exportVersioningMacroIfNeeded(ctx android.BaseModuleContext) {
	if library.buildStubs() && library.stubsVersion() != "" && !library.skipAPIDefine {
		name := versioningMacroName(ctx.Module().(*Module).ImplementationModuleName(ctx))
		ver := library.stubsVersion()
		library.reexportFlags("-D" + name + "=" + ver)
	}
}

// buildStatic returns true if this library should be built as a static library.
func (library *libraryDecorator) buildStatic() bool {
	return library.MutatedProperties.BuildStatic &&
		BoolDefault(library.StaticProperties.Static.Enabled, true)
}

// buildShared returns true if this library should be built as a shared library.
func (library *libraryDecorator) buildShared() bool {
	return library.MutatedProperties.BuildShared &&
		BoolDefault(library.SharedProperties.Shared.Enabled, true)
}

func (library *libraryDecorator) objs() Objects {
	return library.objects
}

func (library *libraryDecorator) reuseObjs() Objects {
	return library.reuseObjects
}

func (library *libraryDecorator) toc() android.OptionalPath {
	return library.tocFile
}

func (library *libraryDecorator) installSymlinkToRuntimeApex(ctx ModuleContext, file android.Path) {
	dir := library.baseInstaller.installDir(ctx)
	dirOnDevice := android.InstallPathToOnDevicePath(ctx, dir)
	target := "/" + filepath.Join("apex", "com.android.runtime", dir.Base(), "bionic", file.Base())
	ctx.InstallAbsoluteSymlink(dir, file.Base(), target)
	library.postInstallCmds = append(library.postInstallCmds, makeSymlinkCmd(dirOnDevice, file.Base(), target))
}

func (library *libraryDecorator) install(ctx ModuleContext, file android.Path) {
	if library.shared() {
		if ctx.Device() && ctx.useVndk() {
			// set subDir for VNDK extensions
			if ctx.IsVndkExt() {
				if ctx.isVndkSp() {
					library.baseInstaller.subDir = "vndk-sp"
				} else {
					library.baseInstaller.subDir = "vndk"
				}
			}

			// In some cases we want to use core variant for VNDK-Core libs.
			// Skip product variant since VNDKs use only the vendor variant.
			if ctx.isVndk() && !ctx.isVndkSp() && !ctx.IsVndkExt() && !ctx.inProduct() {
				mayUseCoreVariant := true

				if ctx.mustUseVendorVariant() {
					mayUseCoreVariant = false
				}

				if ctx.Config().CFIEnabledForPath(ctx.ModuleDir()) && ctx.Arch().ArchType == android.Arm64 {
					mayUseCoreVariant = false
				}

				if mayUseCoreVariant {
					library.checkSameCoreVariant = true
					if ctx.DeviceConfig().VndkUseCoreVariant() {
						library.useCoreVariant = true
					}
				}
			}

			// do not install vndk libs
			// vndk libs are packaged into VNDK APEX
			if ctx.isVndk() && !ctx.IsVndkExt() {
				return
			}
		} else if len(library.Properties.Stubs.Versions) > 0 && !ctx.Host() && ctx.directlyInAnyApex() {
			// Bionic libraries (e.g. libc.so) is installed to the bootstrap subdirectory.
			// The original path becomes a symlink to the corresponding file in the
			// runtime APEX.
			translatedArch := ctx.Target().NativeBridge == android.NativeBridgeEnabled
			if InstallToBootstrap(ctx.baseModuleName(), ctx.Config()) && !library.buildStubs() &&
				!translatedArch && !ctx.inRamdisk() && !ctx.inVendorRamdisk() && !ctx.inRecovery() {
				if ctx.Device() {
					library.installSymlinkToRuntimeApex(ctx, file)
				}
				library.baseInstaller.subDir = "bootstrap"
			}
		} else if ctx.directlyInAnyApex() && ctx.IsLlndk() && !isBionic(ctx.baseModuleName()) {
			// Skip installing LLNDK (non-bionic) libraries moved to APEX.
			ctx.Module().HideFromMake()
		}

		library.baseInstaller.install(ctx, file)
	}

	if Bool(library.Properties.Static_ndk_lib) && library.static() &&
		!ctx.useVndk() && !ctx.inRamdisk() && !ctx.inVendorRamdisk() && !ctx.inRecovery() && ctx.Device() &&
		library.baseLinker.sanitize.isUnsanitizedVariant() &&
		ctx.isForPlatform() && !ctx.isPreventInstall() {
		installPath := getNdkSysrootBase(ctx).Join(
			ctx, "usr/lib", config.NDKTriple(ctx.toolchain()), file.Base())

		ctx.ModuleBuild(pctx, android.ModuleBuildParams{
			Rule:        android.Cp,
			Description: "install " + installPath.Base(),
			Output:      installPath,
			Input:       file,
		})

		library.ndkSysrootPath = installPath
	}
}

func (library *libraryDecorator) everInstallable() bool {
	// Only shared and static libraries are installed. Header libraries (which are
	// neither static or shared) are not installed.
	return library.shared() || library.static()
}

// static returns true if this library is for a "static' variant.
func (library *libraryDecorator) static() bool {
	return library.MutatedProperties.VariantIsStatic
}

// shared returns true if this library is for a "shared' variant.
func (library *libraryDecorator) shared() bool {
	return library.MutatedProperties.VariantIsShared
}

// header returns true if this library is for a header-only variant.
func (library *libraryDecorator) header() bool {
	// Neither "static" nor "shared" implies this library is header-only.
	return !library.static() && !library.shared()
}

// setStatic marks the library variant as "static".
func (library *libraryDecorator) setStatic() {
	library.MutatedProperties.VariantIsStatic = true
	library.MutatedProperties.VariantIsShared = false
}

// setShared marks the library variant as "shared".
func (library *libraryDecorator) setShared() {
	library.MutatedProperties.VariantIsStatic = false
	library.MutatedProperties.VariantIsShared = true
}

// BuildOnlyStatic disables building this library as a shared library.
func (library *libraryDecorator) BuildOnlyStatic() {
	library.MutatedProperties.BuildShared = false
}

// BuildOnlyShared disables building this library as a static library.
func (library *libraryDecorator) BuildOnlyShared() {
	library.MutatedProperties.BuildStatic = false
}

// HeaderOnly disables building this library as a shared or static library;
// the library only exists to propagate header file dependencies up the build graph.
func (library *libraryDecorator) HeaderOnly() {
	library.MutatedProperties.BuildShared = false
	library.MutatedProperties.BuildStatic = false
}

// hasLLNDKStubs returns true if this cc_library module has a variant that will build LLNDK stubs.
func (library *libraryDecorator) hasLLNDKStubs() bool {
	return String(library.Properties.Llndk_stubs) != ""
}

func (library *libraryDecorator) implementationModuleName(name string) string {
	return name
}

func (library *libraryDecorator) buildStubs() bool {
	return library.MutatedProperties.BuildStubs
}

func (library *libraryDecorator) symbolFileForAbiCheck(ctx ModuleContext) *string {
	if library.Properties.Header_abi_checker.Symbol_file != nil {
		return library.Properties.Header_abi_checker.Symbol_file
	}
	if ctx.Module().(*Module).IsLlndk() {
		return library.Properties.Llndk.Symbol_file
	}
	if library.hasStubsVariants() && library.Properties.Stubs.Symbol_file != nil {
		return library.Properties.Stubs.Symbol_file
	}
	return nil
}

func (library *libraryDecorator) hasStubsVariants() bool {
	return len(library.Properties.Stubs.Versions) > 0
}

func (library *libraryDecorator) stubsVersions(ctx android.BaseMutatorContext) []string {
	return library.Properties.Stubs.Versions
}

func (library *libraryDecorator) setStubsVersion(version string) {
	library.MutatedProperties.StubsVersion = version
}

func (library *libraryDecorator) stubsVersion() string {
	return library.MutatedProperties.StubsVersion
}

func (library *libraryDecorator) setBuildStubs() {
	library.MutatedProperties.BuildStubs = true
}

func (library *libraryDecorator) setAllStubsVersions(versions []string) {
	library.MutatedProperties.AllStubsVersions = versions
}

func (library *libraryDecorator) allStubsVersions() []string {
	return library.MutatedProperties.AllStubsVersions
}

func (library *libraryDecorator) isLatestStubVersion() bool {
	versions := library.Properties.Stubs.Versions
	return versions[len(versions)-1] == library.stubsVersion()
}

func (library *libraryDecorator) availableFor(what string) bool {
	var list []string
	if library.static() {
		list = library.StaticProperties.Static.Apex_available
	} else if library.shared() {
		list = library.SharedProperties.Shared.Apex_available
	}
	if len(list) == 0 {
		return false
	}
	return android.CheckAvailableForApex(what, list)
}

func (library *libraryDecorator) makeUninstallable(mod *Module) {
	if library.static() && library.buildStatic() && !library.buildStubs() {
		// If we're asked to make a static library uninstallable we don't do
		// anything since AndroidMkEntries always sets LOCAL_UNINSTALLABLE_MODULE
		// for these entries. This is done to still get the make targets for NOTICE
		// files from notice_files.mk, which other libraries might depend on.
		return
	}
	mod.ModuleBase.MakeUninstallable()
}

var versioningMacroNamesListKey = android.NewOnceKey("versioningMacroNamesList")

// versioningMacroNamesList returns a singleton map, where keys are "version macro names",
// and values are the module name responsible for registering the version macro name.
//
// Version macros are used when building against stubs, to provide version information about
// the stub. Only stub libraries should have an entry in this list.
//
// For example, when building against libFoo#ver, __LIBFOO_API__ macro is set to ver so
// that headers from libFoo can be conditionally compiled (this may hide APIs
// that are not available for the version).
//
// This map is used to ensure that there aren't conflicts between these version macro names.
func versioningMacroNamesList(config android.Config) *map[string]string {
	return config.Once(versioningMacroNamesListKey, func() interface{} {
		m := make(map[string]string)
		return &m
	}).(*map[string]string)
}

// alphanumeric and _ characters are preserved.
// other characters are all converted to _
var charsNotForMacro = regexp.MustCompile("[^a-zA-Z0-9_]+")

// versioningMacroName returns the canonical version macro name for the given module.
func versioningMacroName(moduleName string) string {
	macroName := charsNotForMacro.ReplaceAllString(moduleName, "_")
	macroName = strings.ToUpper(macroName)
	return "__" + macroName + "_API__"
}

// NewLibrary builds and returns a new Module corresponding to a C++ library.
// Individual module implementations which comprise a C++ library (or something like
// a C++ library) should call this function, set some fields on the result, and
// then call the Init function.
func NewLibrary(hod android.HostOrDeviceSupported) (*Module, *libraryDecorator) {
	module := newModule(hod, android.MultilibBoth)

	library := &libraryDecorator{
		MutatedProperties: LibraryMutatedProperties{
			BuildShared: true,
			BuildStatic: true,
		},
		baseCompiler:  NewBaseCompiler(),
		baseLinker:    NewBaseLinker(module.sanitize),
		baseInstaller: NewBaseInstaller("lib", "lib64", InstallInSystem),
		sabi:          module.sabi,
	}

	module.compiler = library
	module.linker = library
	module.installer = library
	module.library = library

	return module, library
}

// connects a shared library to a static library in order to reuse its .o files to avoid
// compiling source files twice.
func reuseStaticLibrary(mctx android.BottomUpMutatorContext, static, shared *Module) {
	if staticCompiler, ok := static.compiler.(*libraryDecorator); ok {
		sharedCompiler := shared.compiler.(*libraryDecorator)

		// Check libraries in addition to cflags, since libraries may be exporting different
		// include directories.
		if len(staticCompiler.StaticProperties.Static.Cflags) == 0 &&
			len(sharedCompiler.SharedProperties.Shared.Cflags) == 0 &&
			len(staticCompiler.StaticProperties.Static.Whole_static_libs) == 0 &&
			len(sharedCompiler.SharedProperties.Shared.Whole_static_libs) == 0 &&
			len(staticCompiler.StaticProperties.Static.Static_libs) == 0 &&
			len(sharedCompiler.SharedProperties.Shared.Static_libs) == 0 &&
			len(staticCompiler.StaticProperties.Static.Shared_libs) == 0 &&
			len(sharedCompiler.SharedProperties.Shared.Shared_libs) == 0 &&
			// Compare System_shared_libs properties with nil because empty lists are
			// semantically significant for them.
			staticCompiler.StaticProperties.Static.System_shared_libs == nil &&
			sharedCompiler.SharedProperties.Shared.System_shared_libs == nil {

			mctx.AddInterVariantDependency(reuseObjTag, shared, static)
			sharedCompiler.baseCompiler.Properties.OriginalSrcs =
				sharedCompiler.baseCompiler.Properties.Srcs
			sharedCompiler.baseCompiler.Properties.Srcs = nil
			sharedCompiler.baseCompiler.Properties.Generated_sources = nil
		}

		// This dep is just to reference static variant from shared variant
		mctx.AddInterVariantDependency(staticVariantTag, shared, static)
	}
}

// LinkageMutator adds "static" or "shared" variants for modules depending
// on whether the module can be built as a static library or a shared library.
func LinkageMutator(mctx android.BottomUpMutatorContext) {
	ccPrebuilt := false
	if m, ok := mctx.Module().(*Module); ok && m.linker != nil {
		_, ccPrebuilt = m.linker.(prebuiltLibraryInterface)
	}
	if ccPrebuilt {
		library := mctx.Module().(*Module).linker.(prebuiltLibraryInterface)

		// Differentiate between header only and building an actual static/shared library
		buildStatic := library.buildStatic()
		buildShared := library.buildShared()
		if buildStatic || buildShared {
			// Always create both the static and shared variants for prebuilt libraries, and then disable the one
			// that is not being used.  This allows them to share the name of a cc_library module, which requires that
			// all the variants of the cc_library also exist on the prebuilt.
			modules := mctx.CreateLocalVariations("static", "shared")
			static := modules[0].(*Module)
			shared := modules[1].(*Module)

			static.linker.(prebuiltLibraryInterface).setStatic()
			shared.linker.(prebuiltLibraryInterface).setShared()

			if buildShared {
				mctx.AliasVariation("shared")
			} else if buildStatic {
				mctx.AliasVariation("static")
			}

			if !buildStatic {
				static.linker.(prebuiltLibraryInterface).disablePrebuilt()
			}
			if !buildShared {
				shared.linker.(prebuiltLibraryInterface).disablePrebuilt()
			}
		} else {
			// Header only
		}

	} else if library, ok := mctx.Module().(LinkableInterface); ok && library.CcLibraryInterface() {

		// Non-cc.Modules may need an empty variant for their mutators.
		variations := []string{}
		if library.NonCcVariants() {
			variations = append(variations, "")
		}

		isLLNDK := false
		if m, ok := mctx.Module().(*Module); ok {
			// Don't count the vestigial llndk_library module as isLLNDK, it needs a static
			// variant so that a cc_library_prebuilt can depend on it.
			isLLNDK = m.IsLlndk() && !isVestigialLLNDKModule(m)
		}
		buildStatic := library.BuildStaticVariant() && !isLLNDK
		buildShared := library.BuildSharedVariant()
		if buildStatic && buildShared {
			variations := append([]string{"static", "shared"}, variations...)

			modules := mctx.CreateLocalVariations(variations...)
			static := modules[0].(LinkableInterface)
			shared := modules[1].(LinkableInterface)

			static.SetStatic()
			shared.SetShared()

			if _, ok := library.(*Module); ok {
				reuseStaticLibrary(mctx, static.(*Module), shared.(*Module))
			}
			mctx.AliasVariation("shared")
		} else if buildStatic {
			variations := append([]string{"static"}, variations...)

			modules := mctx.CreateLocalVariations(variations...)
			modules[0].(LinkableInterface).SetStatic()
			mctx.AliasVariation("static")
		} else if buildShared {
			variations := append([]string{"shared"}, variations...)

			modules := mctx.CreateLocalVariations(variations...)
			modules[0].(LinkableInterface).SetShared()
			mctx.AliasVariation("shared")
		} else if len(variations) > 0 {
			mctx.CreateLocalVariations(variations...)
			mctx.AliasVariation(variations[0])
		}
	}
}

// normalizeVersions modifies `versions` in place, so that each raw version
// string becomes its normalized canonical form.
// Validates that the versions in `versions` are specified in least to greatest order.
func normalizeVersions(ctx android.BaseModuleContext, versions []string) {
	var previous android.ApiLevel
	for i, v := range versions {
		ver, err := android.ApiLevelFromUser(ctx, v)
		if err != nil {
			ctx.PropertyErrorf("versions", "%s", err.Error())
			return
		}
		if i > 0 && ver.LessThanOrEqualTo(previous) {
			ctx.PropertyErrorf("versions", "not sorted: %v", versions)
		}
		versions[i] = ver.String()
		previous = ver
	}
}

func createVersionVariations(mctx android.BottomUpMutatorContext, versions []string) {
	// "" is for the non-stubs (implementation) variant for system modules, or the LLNDK variant
	// for LLNDK modules.
	variants := append(android.CopyOf(versions), "")

	m := mctx.Module().(*Module)
	isLLNDK := m.IsLlndk()

	modules := mctx.CreateLocalVariations(variants...)
	for i, m := range modules {

		if variants[i] != "" || isLLNDK {
			// A stubs or LLNDK stubs variant.
			c := m.(*Module)
			c.sanitize = nil
			c.stl = nil
			c.Properties.PreventInstall = true
			lib := moduleLibraryInterface(m)
			lib.setBuildStubs()

			if variants[i] != "" {
				// A non-LLNDK stubs module is hidden from make and has a dependency from the
				// implementation module to the stubs module.
				c.Properties.HideFromMake = true
				lib.setStubsVersion(variants[i])
				mctx.AddInterVariantDependency(stubImplDepTag, modules[len(modules)-1], modules[i])
			}
		}
	}
	mctx.AliasVariation("")
	latestVersion := ""
	if len(versions) > 0 {
		latestVersion = versions[len(versions)-1]
	}
	mctx.CreateAliasVariation("latest", latestVersion)
}

func createPerApiVersionVariations(mctx android.BottomUpMutatorContext, minSdkVersion string) {
	from, err := nativeApiLevelFromUser(mctx, minSdkVersion)
	if err != nil {
		mctx.PropertyErrorf("min_sdk_version", err.Error())
		return
	}

	versionStrs := ndkLibraryVersions(mctx, from)
	modules := mctx.CreateLocalVariations(versionStrs...)

	for i, module := range modules {
		module.(*Module).Properties.Sdk_version = StringPtr(versionStrs[i])
	}
}

func CanBeOrLinkAgainstVersionVariants(module interface {
	Host() bool
	InRamdisk() bool
	InVendorRamdisk() bool
}) bool {
	return !module.Host() && !module.InRamdisk() && !module.InVendorRamdisk()
}

func CanBeVersionVariant(module interface {
	Host() bool
	InRamdisk() bool
	InVendorRamdisk() bool
	InRecovery() bool
	CcLibraryInterface() bool
	Shared() bool
}) bool {
	return CanBeOrLinkAgainstVersionVariants(module) &&
		module.CcLibraryInterface() && module.Shared()
}

func moduleLibraryInterface(module blueprint.Module) libraryInterface {
	if m, ok := module.(*Module); ok {
		return m.library
	}
	return nil
}

// versionSelector normalizes the versions in the Stubs.Versions property into MutatedProperties.AllStubsVersions,
// and propagates the value from implementation libraries to llndk libraries with the same name.
func versionSelectorMutator(mctx android.BottomUpMutatorContext) {
	if library := moduleLibraryInterface(mctx.Module()); library != nil && CanBeVersionVariant(mctx.Module().(*Module)) {
		if library.buildShared() {
			versions := library.stubsVersions(mctx)
			if len(versions) > 0 {
				normalizeVersions(mctx, versions)
				if mctx.Failed() {
					return
				}
				// Set the versions on the pre-mutated module so they can be read by any llndk modules that
				// depend on the implementation library and haven't been mutated yet.
				library.setAllStubsVersions(versions)
			}

			if mctx.Module().(*Module).UseVndk() && library.hasLLNDKStubs() {
				// Propagate the version to the llndk stubs module.
				mctx.VisitDirectDepsWithTag(llndkStubDepTag, func(stubs android.Module) {
					if stubsLib := moduleLibraryInterface(stubs); stubsLib != nil {
						stubsLib.setAllStubsVersions(library.allStubsVersions())
					}
				})
			}
		}
	}
}

// versionMutator splits a module into the mandatory non-stubs variant
// (which is unnamed) and zero or more stubs variants.
func versionMutator(mctx android.BottomUpMutatorContext) {
	if library := moduleLibraryInterface(mctx.Module()); library != nil && CanBeVersionVariant(mctx.Module().(*Module)) {
		createVersionVariations(mctx, library.allStubsVersions())
		return
	}

	if m, ok := mctx.Module().(*Module); ok {
		if m.SplitPerApiLevel() && m.IsSdkVariant() {
			if mctx.Os() != android.Android {
				return
			}
			createPerApiVersionVariations(mctx, m.MinSdkVersion())
		}
	}
}

// maybeInjectBoringSSLHash adds a rule to run bssl_inject_hash on the output file if the module has the
// inject_bssl_hash or if any static library dependencies have inject_bssl_hash set.  It returns the output path
// that the linked output file should be written to.
// TODO(b/137267623): Remove this in favor of a cc_genrule when they support operating on shared libraries.
func maybeInjectBoringSSLHash(ctx android.ModuleContext, outputFile android.ModuleOutPath,
	inject *bool, fileName string) android.ModuleOutPath {
	// TODO(b/137267623): Remove this in favor of a cc_genrule when they support operating on shared libraries.
	injectBoringSSLHash := Bool(inject)
	ctx.VisitDirectDeps(func(dep android.Module) {
		if tag, ok := ctx.OtherModuleDependencyTag(dep).(libraryDependencyTag); ok && tag.static() {
			if cc, ok := dep.(*Module); ok {
				if library, ok := cc.linker.(*libraryDecorator); ok {
					if Bool(library.Properties.Inject_bssl_hash) {
						injectBoringSSLHash = true
					}
				}
			}
		}
	})
	if injectBoringSSLHash {
		hashedOutputfile := outputFile
		outputFile = android.PathForModuleOut(ctx, "unhashed", fileName)

		rule := android.NewRuleBuilder(pctx, ctx)
		rule.Command().
			BuiltTool("bssl_inject_hash").
			Flag("-sha256").
			FlagWithInput("-in-object ", outputFile).
			FlagWithOutput("-o ", hashedOutputfile)
		rule.Build("injectCryptoHash", "inject crypto hash")
	}

	return outputFile
}
