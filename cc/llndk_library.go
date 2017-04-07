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

	"github.com/google/blueprint"

	"android/soong/android"
)

var (
	llndkLibrarySuffix = ".llndk"
)

// Creates a stub shared library based on the provided version file.
//
// The name of the generated file will be based on the module name by stripping
// the ".llndk" suffix from the module name. Module names must end with ".llndk"
// (as a convention to allow soong to guess the LL-NDK name of a dependency when
// needed). "libfoo.llndk" will generate "libfoo.so".
//
// Example:
//
// llndk_library {
//     name: "libfoo.llndk",
//     symbol_file: "libfoo.map.txt",
//     export_include_dirs: ["include_vndk"],
// }
//
type llndkLibraryProperties struct {
	// Relative path to the symbol map.
	// An example file can be seen here: TODO(danalbert): Make an example.
	Symbol_file string

	// Whether to export any headers as -isystem instead of -I. Mainly for use by
	// bionic/libc.
	Export_headers_as_system bool

	// Which headers to process with versioner. This really only handles
	// bionic/libc/include right now.
	Export_preprocessed_headers []string

	// Whether the system library uses symbol versions.
	Unversioned bool
}

type llndkStubDecorator struct {
	*libraryDecorator

	Properties llndkLibraryProperties

	exportHeadersTimestamp android.OptionalPath
	versionScriptPath      android.ModuleGenPath
}

func (stub *llndkStubDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) Objects {
	if !strings.HasSuffix(ctx.ModuleName(), llndkLibrarySuffix) {
		ctx.ModuleErrorf("llndk_library modules names must be suffixed with %q\n",
			llndkLibrarySuffix)
	}
	objs, versionScript := compileStubLibrary(ctx, flags, stub.Properties.Symbol_file, "current", "--vndk")
	stub.versionScriptPath = versionScript
	return objs
}

func (stub *llndkStubDecorator) linkerDeps(ctx DepsContext, deps Deps) Deps {
	return Deps{}
}

func (stub *llndkStubDecorator) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	stub.libraryDecorator.libName = strings.TrimSuffix(ctx.ModuleName(),
		llndkLibrarySuffix)
	return stub.libraryDecorator.linkerFlags(ctx, flags)
}

func (stub *llndkStubDecorator) processHeaders(ctx ModuleContext, srcHeaderDir string, outDir android.ModuleGenPath) android.Path {
	srcDir := android.PathForModuleSrc(ctx, srcHeaderDir)
	srcFiles := ctx.Glob(filepath.Join(srcDir.String(), "**/*.h"), nil)

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

	if !stub.Properties.Unversioned {
		linkerScriptFlag := "-Wl,--version-script," + stub.versionScriptPath.String()
		flags.LdFlags = append(flags.LdFlags, linkerScriptFlag)
	}

	if len(stub.Properties.Export_preprocessed_headers) > 0 {
		genHeaderOutDir := android.PathForModuleGen(ctx, "include")

		var timestampFiles android.Paths
		for _, dir := range stub.Properties.Export_preprocessed_headers {
			timestampFiles = append(timestampFiles, stub.processHeaders(ctx, dir, genHeaderOutDir))
		}

		includePrefix := "-I "
		if stub.Properties.Export_headers_as_system {
			includePrefix = "-isystem "
		}

		stub.reexportFlags([]string{includePrefix + " " + genHeaderOutDir.String()})
		stub.reexportDeps(timestampFiles)
	}

	if stub.Properties.Export_headers_as_system {
		stub.exportIncludes(ctx, "-isystem")
		stub.libraryDecorator.flagExporter.Properties.Export_include_dirs = []string{}
	}

	return stub.libraryDecorator.link(ctx, flags, deps, objs)
}

func newLLndkStubLibrary() (*Module, []interface{}) {
	module, library := NewLibrary(android.DeviceSupported)
	library.BuildOnlyShared()
	module.stl = nil
	module.sanitize = nil
	library.StripProperties.Strip.None = true

	stub := &llndkStubDecorator{
		libraryDecorator: library,
	}
	module.compiler = stub
	module.linker = stub
	module.installer = nil

	return module, []interface{}{&stub.Properties, &library.MutatedProperties, &library.flagExporter.Properties}
}

func llndkLibraryFactory() (blueprint.Module, []interface{}) {
	module, properties := newLLndkStubLibrary()
	return android.InitAndroidArchModule(module, android.DeviceSupported,
		android.MultilibBoth, properties...)
}

func init() {
	android.RegisterModuleType("llndk_library", llndkLibraryFactory)
}
