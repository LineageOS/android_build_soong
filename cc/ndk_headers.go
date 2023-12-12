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
	"path/filepath"

	"android/soong/android"
	"github.com/google/blueprint"
)

var (
	versionBionicHeaders = pctx.AndroidStaticRule("versionBionicHeaders",
		blueprint.RuleParams{
			// The `&& touch $out` isn't really necessary, but Blueprint won't
			// let us have only implicit outputs.
			Command:     "$versionerCmd -o $outDir $srcDir $depsPath && touch $out",
			CommandDeps: []string{"$versionerCmd"},
		},
		"depsPath", "srcDir", "outDir")

	preprocessNdkHeader = pctx.AndroidStaticRule("preprocessNdkHeader",
		blueprint.RuleParams{
			Command:     "$preprocessor -o $out $in",
			CommandDeps: []string{"$preprocessor"},
		},
		"preprocessor")
)

func init() {
	pctx.SourcePathVariable("versionerCmd", "prebuilts/clang-tools/${config.HostPrebuiltTag}/bin/versioner")
}

// Returns the NDK base include path for use with sdk_version current. Usable with -I.
func getCurrentIncludePath(ctx android.ModuleContext) android.InstallPath {
	return getNdkSysrootBase(ctx).Join(ctx, "usr/include")
}

type headerProperties struct {
	// Base directory of the headers being installed. As an example:
	//
	// ndk_headers {
	//     name: "foo",
	//     from: "include",
	//     to: "",
	//     srcs: ["include/foo/bar/baz.h"],
	// }
	//
	// Will install $SYSROOT/usr/include/foo/bar/baz.h. If `from` were instead
	// "include/foo", it would have installed $SYSROOT/usr/include/bar/baz.h.
	From *string

	// Install path within the sysroot. This is relative to usr/include.
	To *string

	// List of headers to install. Glob compatible. Common case is "include/**/*.h".
	Srcs []string `android:"path"`

	// Source paths that should be excluded from the srcs glob.
	Exclude_srcs []string `android:"path"`

	// Path to the NOTICE file associated with the headers.
	License *string `android:"path"`
}

type headerModule struct {
	android.ModuleBase

	properties headerProperties

	srcPaths     android.Paths
	installPaths android.Paths
	licensePath  android.Path
}

func getHeaderInstallDir(ctx android.ModuleContext, header android.Path, from string,
	to string) android.InstallPath {
	// Output path is the sysroot base + "usr/include" + to directory + directory component
	// of the file without the leading from directory stripped.
	//
	// Given:
	// sysroot base = "ndk/sysroot"
	// from = "include/foo"
	// to = "bar"
	// header = "include/foo/woodly/doodly.h"
	// output path = "ndk/sysroot/usr/include/bar/woodly/doodly.h"

	// full/platform/path/to/include/foo
	fullFromPath := android.PathForModuleSrc(ctx, from)

	// full/platform/path/to/include/foo/woodly
	headerDir := filepath.Dir(header.String())

	// woodly
	strippedHeaderDir, err := filepath.Rel(fullFromPath.String(), headerDir)
	if err != nil {
		ctx.ModuleErrorf("filepath.Rel(%q, %q) failed: %s", headerDir,
			fullFromPath.String(), err)
	}

	// full/platform/path/to/sysroot/usr/include/bar/woodly
	installDir := getCurrentIncludePath(ctx).Join(ctx, to, strippedHeaderDir)

	// full/platform/path/to/sysroot/usr/include/bar/woodly/doodly.h
	return installDir
}

func (m *headerModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if String(m.properties.License) == "" {
		ctx.PropertyErrorf("license", "field is required")
	}

	m.licensePath = android.PathForModuleSrc(ctx, String(m.properties.License))

	m.srcPaths = android.PathsForModuleSrcExcludes(ctx, m.properties.Srcs, m.properties.Exclude_srcs)
	for _, header := range m.srcPaths {
		installDir := getHeaderInstallDir(ctx, header, String(m.properties.From),
			String(m.properties.To))
		installedPath := ctx.InstallFile(installDir, header.Base(), header)
		installPath := installDir.Join(ctx, header.Base())
		if installPath != installedPath {
			panic(fmt.Sprintf(
				"expected header install path (%q) not equal to actual install path %q",
				installPath, installedPath))
		}
		m.installPaths = append(m.installPaths, installPath)
	}

	if len(m.installPaths) == 0 {
		ctx.ModuleErrorf("srcs %q matched zero files", m.properties.Srcs)
	}
}

// ndk_headers installs the sets of ndk headers defined in the srcs property
// to the sysroot base + "usr/include" + to directory + directory component.
// ndk_headers requires the license file to be specified. Example:
//
//	Given:
//	sysroot base = "ndk/sysroot"
//	from = "include/foo"
//	to = "bar"
//	header = "include/foo/woodly/doodly.h"
//	output path = "ndk/sysroot/usr/include/bar/woodly/doodly.h"
func NdkHeadersFactory() android.Module {
	module := &headerModule{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

type versionedHeaderProperties struct {
	// Base directory of the headers being installed. As an example:
	//
	// versioned_ndk_headers {
	//     name: "foo",
	//     from: "include",
	//     to: "",
	// }
	//
	// Will install $SYSROOT/usr/include/foo/bar/baz.h. If `from` were instead
	// "include/foo", it would have installed $SYSROOT/usr/include/bar/baz.h.
	From *string

	// Install path within the sysroot. This is relative to usr/include.
	To *string

	// Path to the NOTICE file associated with the headers.
	License *string
}

// Like ndk_headers, but preprocesses the headers with the bionic versioner:
// https://android.googlesource.com/platform/bionic/+/master/tools/versioner/README.md.
//
// Unlike ndk_headers, we don't operate on a list of sources but rather a whole directory, the
// module does not have the srcs property, and operates on a full directory (the `from` property).
//
// Note that this is really only built to handle bionic/libc/include.
type versionedHeaderModule struct {
	android.ModuleBase

	properties versionedHeaderProperties

	srcPaths     android.Paths
	installPaths android.Paths
	licensePath  android.Path
}

// Return the glob pattern to find all .h files beneath `dir`
func headerGlobPattern(dir string) string {
	return filepath.Join(dir, "**", "*.h")
}

func (m *versionedHeaderModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if String(m.properties.License) == "" {
		ctx.PropertyErrorf("license", "field is required")
	}

	m.licensePath = android.PathForModuleSrc(ctx, String(m.properties.License))

	fromSrcPath := android.PathForModuleSrc(ctx, String(m.properties.From))
	toOutputPath := getCurrentIncludePath(ctx).Join(ctx, String(m.properties.To))
	m.srcPaths = ctx.GlobFiles(headerGlobPattern(fromSrcPath.String()), nil)
	var installPaths []android.WritablePath
	for _, header := range m.srcPaths {
		installDir := getHeaderInstallDir(ctx, header, String(m.properties.From), String(m.properties.To))
		installPath := installDir.Join(ctx, header.Base())
		installPaths = append(installPaths, installPath)
		m.installPaths = append(m.installPaths, installPath)
	}

	if len(m.installPaths) == 0 {
		ctx.ModuleErrorf("glob %q matched zero files", String(m.properties.From))
	}

	processHeadersWithVersioner(ctx, fromSrcPath, toOutputPath, m.srcPaths, installPaths)
}

func processHeadersWithVersioner(ctx android.ModuleContext, srcDir, outDir android.Path,
	srcPaths android.Paths, installPaths []android.WritablePath) android.Path {
	// The versioner depends on a dependencies directory to simplify determining include paths
	// when parsing headers. This directory contains architecture specific directories as well
	// as a common directory, each of which contains symlinks to the actually directories to
	// be included.
	//
	// ctx.Glob doesn't follow symlinks, so we need to do this ourselves so we correctly
	// depend on these headers.
	// TODO(http://b/35673191): Update the versioner to use a --sysroot.
	depsPath := android.PathForSource(ctx, "bionic/libc/versioner-dependencies")
	depsGlob := ctx.Glob(filepath.Join(depsPath.String(), "**/*"), nil)
	for i, path := range depsGlob {
		if ctx.IsSymlink(path) {
			dest := ctx.Readlink(path)
			// Additional .. to account for the symlink itself.
			depsGlob[i] = android.PathForSource(
				ctx, filepath.Clean(filepath.Join(path.String(), "..", dest)))
		}
	}

	timestampFile := android.PathForModuleOut(ctx, "versioner.timestamp")
	ctx.Build(pctx, android.BuildParams{
		Rule:            versionBionicHeaders,
		Description:     "versioner preprocess " + srcDir.Rel(),
		Output:          timestampFile,
		Implicits:       append(srcPaths, depsGlob...),
		ImplicitOutputs: installPaths,
		Args: map[string]string{
			"depsPath": depsPath.String(),
			"srcDir":   srcDir.String(),
			"outDir":   outDir.String(),
		},
	})

	return timestampFile
}

// versioned_ndk_headers preprocesses the headers with the bionic versioner:
// https://android.googlesource.com/platform/bionic/+/master/tools/versioner/README.md.
// Unlike the ndk_headers soong module, versioned_ndk_headers operates on a
// directory level specified in `from` property. This is only used to process
// the bionic/libc/include directory.
func VersionedNdkHeadersFactory() android.Module {
	module := &versionedHeaderModule{}

	module.AddProperties(&module.properties)

	android.InitAndroidModule(module)

	return module
}

// preprocessed_ndk_header {
//
//	name: "foo",
//	preprocessor: "foo.sh",
//	srcs: [...],
//	to: "android",
//
// }
//
// Will invoke the preprocessor as:
//
//	$preprocessor -o $SYSROOT/usr/include/android/needs_preproc.h $src
//
// For each src in srcs.
type preprocessedHeadersProperties struct {
	// The preprocessor to run. Must be a program inside the source directory
	// with no dependencies.
	Preprocessor *string

	// Source path to the files to be preprocessed.
	Srcs []string

	// Source paths that should be excluded from the srcs glob.
	Exclude_srcs []string

	// Install path within the sysroot. This is relative to usr/include.
	To *string

	// Path to the NOTICE file associated with the headers.
	License *string
}

type preprocessedHeadersModule struct {
	android.ModuleBase

	properties preprocessedHeadersProperties

	srcPaths     android.Paths
	installPaths android.Paths
	licensePath  android.Path
}

func (m *preprocessedHeadersModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if String(m.properties.License) == "" {
		ctx.PropertyErrorf("license", "field is required")
	}

	preprocessor := android.PathForModuleSrc(ctx, String(m.properties.Preprocessor))
	m.licensePath = android.PathForModuleSrc(ctx, String(m.properties.License))

	m.srcPaths = android.PathsForModuleSrcExcludes(ctx, m.properties.Srcs, m.properties.Exclude_srcs)
	installDir := getCurrentIncludePath(ctx).Join(ctx, String(m.properties.To))
	for _, src := range m.srcPaths {
		installPath := installDir.Join(ctx, src.Base())
		m.installPaths = append(m.installPaths, installPath)

		ctx.Build(pctx, android.BuildParams{
			Rule:        preprocessNdkHeader,
			Description: "preprocess " + src.Rel(),
			Input:       src,
			Output:      installPath,
			Args: map[string]string{
				"preprocessor": preprocessor.String(),
			},
		})
	}

	if len(m.installPaths) == 0 {
		ctx.ModuleErrorf("srcs %q matched zero files", m.properties.Srcs)
	}
}

// preprocessed_ndk_headers preprocesses all the ndk headers listed in the srcs
// property by executing the command defined in the preprocessor property.
func preprocessedNdkHeadersFactory() android.Module {
	module := &preprocessedHeadersModule{}

	module.AddProperties(&module.properties)

	android.InitAndroidModule(module)

	return module
}
