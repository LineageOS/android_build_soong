// Copyright (C) 2019 The Android Open Source Project
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

package sdk

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
)

var pctx = android.NewPackageContext("android/soong/sdk")

// generatedFile abstracts operations for writing contents into a file and emit a build rule
// for the file.
type generatedFile struct {
	path        android.OutputPath
	content     strings.Builder
	indentLevel int
}

func newGeneratedFile(ctx android.ModuleContext, path ...string) *generatedFile {
	return &generatedFile{
		path:        android.PathForModuleOut(ctx, path...).OutputPath,
		indentLevel: 0,
	}
}

func (gf *generatedFile) Indent() {
	gf.indentLevel++
}

func (gf *generatedFile) Dedent() {
	gf.indentLevel--
}

func (gf *generatedFile) Printfln(format string, args ...interface{}) {
	// ninja consumes newline characters in rspfile_content. Prevent it by
	// escaping the backslash in the newline character. The extra backslash
	// is removed when the rspfile is written to the actual script file
	fmt.Fprintf(&(gf.content), strings.Repeat("    ", gf.indentLevel)+format+"\\n", args...)
}

func (gf *generatedFile) build(pctx android.PackageContext, ctx android.BuilderContext, implicits android.Paths) {
	rb := android.NewRuleBuilder()
	// convert \\n to \n
	rb.Command().
		Implicits(implicits).
		Text("echo").Text(proptools.ShellEscape(gf.content.String())).
		Text("| sed 's/\\\\n/\\n/g' >").Output(gf.path)
	rb.Command().
		Text("chmod a+x").Output(gf.path)
	rb.Build(pctx, ctx, gf.path.Base(), "Build "+gf.path.Base())
}

func (s *sdk) javaLibs(ctx android.ModuleContext) []android.SdkAware {
	result := []android.SdkAware{}
	ctx.VisitDirectDeps(func(m android.Module) {
		if j, ok := m.(*java.Library); ok {
			result = append(result, j)
		}
	})
	return result
}

// archSpecificNativeLibInfo represents an arch-specific variant of a native lib
type archSpecificNativeLibInfo struct {
	name                      string
	archType                  string
	exportedIncludeDirs       android.Paths
	exportedSystemIncludeDirs android.Paths
	exportedFlags             []string
	exportedDeps              android.Paths
	outputFile                android.Path
}

func (lib *archSpecificNativeLibInfo) signature() string {
	return fmt.Sprintf("%v %v %v %v",
		lib.name,
		lib.exportedIncludeDirs.Strings(),
		lib.exportedSystemIncludeDirs.Strings(),
		lib.exportedFlags)
}

// nativeLibInfo represents a collection of arch-specific modules having the same name
type nativeLibInfo struct {
	name         string
	archVariants []archSpecificNativeLibInfo
	// hasArchSpecificFlags is set to true if modules for each architecture all have the same
	// include dirs, flags, etc, in which case only those of the first arch is selected.
	hasArchSpecificFlags bool
}

// nativeMemberInfos collects all cc.Modules that are member of an SDK.
func (s *sdk) nativeMemberInfos(ctx android.ModuleContext) []*nativeLibInfo {
	infoMap := make(map[string]*nativeLibInfo)

	// Collect cc.Modules
	ctx.VisitDirectDeps(func(m android.Module) {
		ccModule, ok := m.(*cc.Module)
		if !ok {
			return
		}
		depName := ctx.OtherModuleName(m)

		if _, ok := infoMap[depName]; !ok {
			infoMap[depName] = &nativeLibInfo{name: depName}
		}

		info := infoMap[depName]
		info.archVariants = append(info.archVariants, archSpecificNativeLibInfo{
			name:                      ccModule.BaseModuleName(),
			archType:                  ccModule.Target().Arch.ArchType.String(),
			exportedIncludeDirs:       ccModule.ExportedIncludeDirs(),
			exportedSystemIncludeDirs: ccModule.ExportedSystemIncludeDirs(),
			exportedFlags:             ccModule.ExportedFlags(),
			exportedDeps:              ccModule.ExportedDeps(),
			outputFile:                ccModule.OutputFile().Path(),
		})
	})

	// Determine if include dirs and flags for each module are different across arch-specific
	// modules or not. And set hasArchSpecificFlags accordingly
	for _, info := range infoMap {
		// by default, include paths and flags are assumed to be the same across arches
		info.hasArchSpecificFlags = false
		oldSignature := ""
		for _, av := range info.archVariants {
			newSignature := av.signature()
			if oldSignature == "" {
				oldSignature = newSignature
			}
			if oldSignature != newSignature {
				info.hasArchSpecificFlags = true
				break
			}
		}
	}

	var list []*nativeLibInfo
	for _, v := range infoMap {
		list = append(list, v)
	}
	return list
}

// SDK directory structure
// <sdk_root>/
//     Android.bp   : definition of a 'sdk' module is here. This is a hand-made one.
//     <api_ver>/   : below this directory are all auto-generated
//         Android.bp   : definition of 'sdk_snapshot' module is here
//         aidl/
//            frameworks/base/core/..../IFoo.aidl   : an exported AIDL file
//         java/
//            <module_name>.jar    : the stub jar for a java library 'module_name'
//         include/
//            bionic/libc/include/stdlib.h   : an exported header file
//         include_gen/
//            <module_name>/com/android/.../IFoo.h : a generated header file
//         <arch>/include/   : arch-specific exported headers
//         <arch>/include_gen/   : arch-specific generated headers
//         <arch>/lib/
//            libFoo.so   : a stub library

const (
	nativeIncludeDir          = "include"
	nativeGeneratedIncludeDir = "include_gen"
	nativeStubDir             = "lib"
	nativeStubFileSuffix      = ".so"
)

// path to the stub file of a native shared library. Relative to <sdk_root>/<api_dir>
func nativeStubFilePathFor(lib archSpecificNativeLibInfo) string {
	return filepath.Join(lib.archType,
		nativeStubDir, lib.name+nativeStubFileSuffix)
}

// paths to the include dirs of a native shared library. Relative to <sdk_root>/<api_dir>
func nativeIncludeDirPathsFor(ctx android.ModuleContext, lib archSpecificNativeLibInfo,
	systemInclude bool, archSpecific bool) []string {
	var result []string
	var includeDirs []android.Path
	if !systemInclude {
		includeDirs = lib.exportedIncludeDirs
	} else {
		includeDirs = lib.exportedSystemIncludeDirs
	}
	for _, dir := range includeDirs {
		var path string
		if _, gen := dir.(android.WritablePath); gen {
			path = filepath.Join(nativeGeneratedIncludeDir, lib.name)
		} else {
			path = filepath.Join(nativeIncludeDir, dir.String())
		}
		if archSpecific {
			path = filepath.Join(lib.archType, path)
		}
		result = append(result, path)
	}
	return result
}

// A name that uniquely identifies a prebuilt SDK member for a version of SDK snapshot
// This isn't visible to users, so could be changed in future.
func versionedSdkMemberName(ctx android.ModuleContext, memberName string, version string) string {
	return ctx.ModuleName() + "_" + memberName + string(android.SdkVersionSeparator) + version
}

// buildSnapshot is the main function in this source file. It creates rules to copy
// the contents (header files, stub libraries, etc) into the zip file.
func (s *sdk) buildSnapshot(ctx android.ModuleContext) android.OutputPath {
	snapshotDir := android.PathForModuleOut(ctx, "snapshot")

	bp := newGeneratedFile(ctx, "snapshot", "Android.bp")
	bp.Printfln("// This is auto-generated. DO NOT EDIT.")
	bp.Printfln("")

	builder := &snapshotBuilder{
		ctx:           ctx,
		version:       "current",
		snapshotDir:   snapshotDir.OutputPath,
		androidBpFile: bp,
		filesToZip:    []android.Path{bp.path},
	}

	// copy exported AIDL files and stub jar files
	javaLibs := s.javaLibs(ctx)
	for _, m := range javaLibs {
		m.BuildSnapshot(ctx, builder)
	}

	// copy exported header files and stub *.so files
	nativeLibInfos := s.nativeMemberInfos(ctx)
	for _, info := range nativeLibInfos {
		buildSharedNativeLibSnapshot(ctx, info, builder)
	}

	// generate Android.bp

	bp.Printfln("sdk_snapshot {")
	bp.Indent()
	bp.Printfln("name: %q,", ctx.ModuleName()+string(android.SdkVersionSeparator)+builder.version)
	if len(javaLibs) > 0 {
		bp.Printfln("java_libs: [")
		bp.Indent()
		for _, m := range javaLibs {
			bp.Printfln("%q,", builder.VersionedSdkMemberName(m.Name()))
		}
		bp.Dedent()
		bp.Printfln("],") // java_libs
	}
	if len(nativeLibInfos) > 0 {
		bp.Printfln("native_shared_libs: [")
		bp.Indent()
		for _, info := range nativeLibInfos {
			bp.Printfln("%q,", builder.VersionedSdkMemberName(info.name))
		}
		bp.Dedent()
		bp.Printfln("],") // native_shared_libs
	}
	bp.Dedent()
	bp.Printfln("}") // sdk_snapshot
	bp.Printfln("")

	bp.build(pctx, ctx, nil)

	filesToZip := builder.filesToZip

	// zip them all
	zipFile := android.PathForModuleOut(ctx, ctx.ModuleName()+"-current.zip").OutputPath
	rb := android.NewRuleBuilder()
	rb.Command().
		BuiltTool(ctx, "soong_zip").
		FlagWithArg("-C ", builder.snapshotDir.String()).
		FlagWithRspFileInputList("-l ", filesToZip).
		FlagWithOutput("-o ", zipFile)
	rb.Build(pctx, ctx, "snapshot", "Building snapshot for "+ctx.ModuleName())

	return zipFile
}

func buildSharedNativeLibSnapshot(ctx android.ModuleContext, info *nativeLibInfo, builder android.SnapshotBuilder) {
	// a function for emitting include dirs
	printExportedDirCopyCommandsForNativeLibs := func(lib archSpecificNativeLibInfo) {
		includeDirs := lib.exportedIncludeDirs
		includeDirs = append(includeDirs, lib.exportedSystemIncludeDirs...)
		if len(includeDirs) == 0 {
			return
		}
		for _, dir := range includeDirs {
			if _, gen := dir.(android.WritablePath); gen {
				// generated headers are copied via exportedDeps. See below.
				continue
			}
			targetDir := nativeIncludeDir
			if info.hasArchSpecificFlags {
				targetDir = filepath.Join(lib.archType, targetDir)
			}

			// TODO(jiyong) copy headers having other suffixes
			headers, _ := ctx.GlobWithDeps(dir.String()+"/**/*.h", nil)
			for _, file := range headers {
				src := android.PathForSource(ctx, file)
				dest := filepath.Join(targetDir, file)
				builder.CopyToSnapshot(src, dest)
			}
		}

		genHeaders := lib.exportedDeps
		for _, file := range genHeaders {
			targetDir := nativeGeneratedIncludeDir
			if info.hasArchSpecificFlags {
				targetDir = filepath.Join(lib.archType, targetDir)
			}
			dest := filepath.Join(targetDir, lib.name, file.Rel())
			builder.CopyToSnapshot(file, dest)
		}
	}

	if !info.hasArchSpecificFlags {
		printExportedDirCopyCommandsForNativeLibs(info.archVariants[0])
	}

	// for each architecture
	for _, av := range info.archVariants {
		builder.CopyToSnapshot(av.outputFile, nativeStubFilePathFor(av))

		if info.hasArchSpecificFlags {
			printExportedDirCopyCommandsForNativeLibs(av)
		}
	}

	bp := builder.AndroidBpFile()
	bp.Printfln("cc_prebuilt_library_shared {")
	bp.Indent()
	bp.Printfln("name: %q,", builder.VersionedSdkMemberName(info.name))
	bp.Printfln("sdk_member_name: %q,", info.name)

	// a function for emitting include dirs
	printExportedDirsForNativeLibs := func(lib archSpecificNativeLibInfo, systemInclude bool) {
		includeDirs := nativeIncludeDirPathsFor(ctx, lib, systemInclude, info.hasArchSpecificFlags)
		if len(includeDirs) == 0 {
			return
		}
		if !systemInclude {
			bp.Printfln("export_include_dirs: [")
		} else {
			bp.Printfln("export_system_include_dirs: [")
		}
		bp.Indent()
		for _, dir := range includeDirs {
			bp.Printfln("%q,", dir)
		}
		bp.Dedent()
		bp.Printfln("],")
	}

	if !info.hasArchSpecificFlags {
		printExportedDirsForNativeLibs(info.archVariants[0], false /*systemInclude*/)
		printExportedDirsForNativeLibs(info.archVariants[0], true /*systemInclude*/)
	}

	bp.Printfln("arch: {")
	bp.Indent()
	for _, av := range info.archVariants {
		bp.Printfln("%s: {", av.archType)
		bp.Indent()
		bp.Printfln("srcs: [%q],", nativeStubFilePathFor(av))
		if info.hasArchSpecificFlags {
			// export_* properties are added inside the arch: {<arch>: {...}} block
			printExportedDirsForNativeLibs(av, false /*systemInclude*/)
			printExportedDirsForNativeLibs(av, true /*systemInclude*/)
		}
		bp.Dedent()
		bp.Printfln("},") // <arch>
	}
	bp.Dedent()
	bp.Printfln("},") // arch
	bp.Printfln("stl: \"none\",")
	bp.Printfln("system_shared_libs: [],")
	bp.Dedent()
	bp.Printfln("}") // cc_prebuilt_library_shared
	bp.Printfln("")
}

type snapshotBuilder struct {
	ctx           android.ModuleContext
	version       string
	snapshotDir   android.OutputPath
	filesToZip    android.Paths
	androidBpFile *generatedFile
}

func (s *snapshotBuilder) CopyToSnapshot(src android.Path, dest string) {
	path := s.snapshotDir.Join(s.ctx, dest)
	s.ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Input:  src,
		Output: path,
	})
	s.filesToZip = append(s.filesToZip, path)
}

func (s *snapshotBuilder) AndroidBpFile() android.GeneratedSnapshotFile {
	return s.androidBpFile
}

func (s *snapshotBuilder) VersionedSdkMemberName(unversionedName string) interface{} {
	return versionedSdkMemberName(s.ctx, unversionedName, s.version)
}
