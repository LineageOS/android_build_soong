// Copyright 2017 Google Inc. All rights reserved.
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
	"path/filepath"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
)

var llndkImplDep = struct {
	blueprint.DependencyTag
}{}

var (
	llndkLibrarySuffix = ".llndk"
	llndkHeadersSuffix = ".llndk"
)

// Creates a stub shared library based on the provided version file.
//
// Example:
//
// llndk_library {
//     name: "libfoo",
//     symbol_file: "libfoo.map.txt",
//     export_include_dirs: ["include_vndk"],
// }
//
type llndkLibraryProperties struct {
	// Relative path to the symbol map.
	// An example file can be seen here: TODO(danalbert): Make an example.
	Symbol_file *string

	// Whether to export any headers as -isystem instead of -I. Mainly for use by
	// bionic/libc.
	Export_headers_as_system *bool

	// Which headers to process with versioner. This really only handles
	// bionic/libc/include right now.
	Export_preprocessed_headers []string

	// Whether the system library uses symbol versions.
	Unversioned *bool

	// whether this module can be directly depended upon by libs that are installed to /vendor.
	// When set to false, this module can only be depended on by VNDK libraries, not vendor
	// libraries. This effectively hides this module from vendors. Default value is true.
	Vendor_available *bool

	// list of llndk headers to re-export include directories from.
	Export_llndk_headers []string `android:"arch_variant"`
}

type llndkStubDecorator struct {
	*libraryDecorator

	Properties llndkLibraryProperties

	exportHeadersTimestamp android.OptionalPath
	versionScriptPath      android.ModuleGenPath
}

func (stub *llndkStubDecorator) compilerFlags(ctx ModuleContext, flags Flags, deps PathDeps) Flags {
	flags = stub.baseCompiler.compilerFlags(ctx, flags, deps)
	return addStubLibraryCompilerFlags(flags)
}

func (stub *llndkStubDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	vndkVer := ctx.Module().(*Module).VndkVersion()
	if !inList(vndkVer, ctx.Config().PlatformVersionActiveCodenames()) || vndkVer == "" {
		// For non-enforcing devices, vndkVer is empty. Use "current" in that case, too.
		vndkVer = "current"
	}
	if stub.stubsVersion() != "" {
		vndkVer = stub.stubsVersion()
	}
	objs, versionScript := compileStubLibrary(ctx, flags, String(stub.Properties.Symbol_file), vndkVer, "--llndk")
	stub.versionScriptPath = versionScript
	return objs
}

func (stub *llndkStubDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	headers := addSuffix(stub.Properties.Export_llndk_headers, llndkHeadersSuffix)
	deps.HeaderLibs = append(deps.HeaderLibs, headers...)
	deps.ReexportHeaderLibHeaders = append(deps.ReexportHeaderLibHeaders, headers...)
	return deps
}

func (stub *llndkStubDecorator) Name(name string) string {
	return name + llndkLibrarySuffix
}

func (stub *llndkStubDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	stub.libraryDecorator.libName = strings.TrimSuffix(ctx.ModuleName(),
		llndkLibrarySuffix)
	return stub.libraryDecorator.linkerFlags(ctx, flags)
}

func (stub *llndkStubDecorator) processHeaders(ctx ModuleContext, srcHeaderDir string, outDir android.ModuleGenPath) android.Path {
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

func (stub *llndkStubDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps,
	objs Objects) android.Path {

	if !Bool(stub.Properties.Unversioned) {
		linkerScriptFlag := "-Wl,--version-script," + stub.versionScriptPath.String()
		flags.Local.LdFlags = append(flags.Local.LdFlags, linkerScriptFlag)
		flags.LdFlagsDeps = append(flags.LdFlagsDeps, stub.versionScriptPath)
	}

	if len(stub.Properties.Export_preprocessed_headers) > 0 {
		genHeaderOutDir := android.PathForModuleGen(ctx, "include")

		var timestampFiles android.Paths
		for _, dir := range stub.Properties.Export_preprocessed_headers {
			timestampFiles = append(timestampFiles, stub.processHeaders(ctx, dir, genHeaderOutDir))
		}

		if Bool(stub.Properties.Export_headers_as_system) {
			stub.reexportSystemDirs(genHeaderOutDir)
		} else {
			stub.reexportDirs(genHeaderOutDir)
		}

		stub.reexportDeps(timestampFiles...)
	}

	if Bool(stub.Properties.Export_headers_as_system) {
		stub.exportIncludesAsSystem(ctx)
		stub.libraryDecorator.flagExporter.Properties.Export_include_dirs = []string{}
	}

	if stub.stubsVersion() != "" {
		stub.reexportFlags("-D" + versioningMacroName(ctx.baseModuleName()) + "=" + stub.stubsVersion())
	}

	return stub.libraryDecorator.link(ctx, flags, deps, objs)
}

func (stub *llndkStubDecorator) nativeCoverage() bool {
	return false
}

func NewLLndkStubLibrary() *Module {
	module, library := NewLibrary(android.DeviceSupported)
	library.BuildOnlyShared()
	module.stl = nil
	module.sanitize = nil
	library.StripProperties.Strip.None = BoolPtr(true)

	stub := &llndkStubDecorator{
		libraryDecorator: library,
	}
	stub.Properties.Vendor_available = BoolPtr(true)
	module.compiler = stub
	module.linker = stub
	module.installer = nil

	module.AddProperties(
		&module.Properties,
		&stub.Properties,
		&library.MutatedProperties,
		&library.flagExporter.Properties)

	return module
}

// llndk_library creates a stub llndk shared library based on the provided
// version file. Example:
//
//    llndk_library {
//        name: "libfoo",
//        symbol_file: "libfoo.map.txt",
//        export_include_dirs: ["include_vndk"],
//    }
func LlndkLibraryFactory() android.Module {
	module := NewLLndkStubLibrary()
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibBoth)
	return module
}

type llndkHeadersDecorator struct {
	*libraryDecorator
}

func (headers *llndkHeadersDecorator) Name(name string) string {
	return name + llndkHeadersSuffix
}

// llndk_headers contains a set of c/c++ llndk headers files which are imported
// by other soongs cc modules.
func llndkHeadersFactory() android.Module {
	module, library := NewLibrary(android.DeviceSupported)
	library.HeaderOnly()

	decorator := &llndkHeadersDecorator{
		libraryDecorator: library,
	}

	module.compiler = nil
	module.linker = decorator
	module.installer = nil

	module.AddProperties(
		&module.Properties,
		&library.MutatedProperties,
		&library.flagExporter.Properties)

	module.Init()

	return module
}

func init() {
	android.RegisterModuleType("llndk_library", LlndkLibraryFactory)
	android.RegisterModuleType("llndk_headers", llndkHeadersFactory)
}
