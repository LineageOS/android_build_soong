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

func (gf *generatedFile) indent() {
	gf.indentLevel++
}

func (gf *generatedFile) dedent() {
	gf.indentLevel--
}

func (gf *generatedFile) printfln(format string, args ...interface{}) {
	// ninja consumes newline characters in rspfile_content. Prevent it by
	// escaping the backslash in the newline character. The extra backshash
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

func (s *sdk) javaLibs(ctx android.ModuleContext) []*java.Library {
	result := []*java.Library{}
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
	aidlIncludeDir            = "aidl"
	javaStubDir               = "java"
	javaStubFileSuffix        = ".jar"
	nativeIncludeDir          = "include"
	nativeGeneratedIncludeDir = "include_gen"
	nativeStubDir             = "lib"
	nativeStubFileSuffix      = ".so"
)

// path to the stub file of a java library. Relative to <sdk_root>/<api_dir>
func javaStubFilePathFor(javaLib *java.Library) string {
	return filepath.Join(javaStubDir, javaLib.Name()+javaStubFileSuffix)
}

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

// buildAndroidBp creates the blueprint file that defines prebuilt modules for each of
// the SDK members, and the entire sdk_snapshot module for the specified version
// TODO(jiyong): create a meta info file (e.g. json, protobuf, etc.) instead, and convert it to
// Android.bp in the (presumably old) branch where the snapshots will be used. This will give us
// some flexibility to introduce backwards incompatible changes in soong.
func (s *sdk) buildAndroidBp(ctx android.ModuleContext, version string) android.OutputPath {
	bp := newGeneratedFile(ctx, "snapshot", "Android.bp")
	bp.printfln("// This is auto-generated. DO NOT EDIT.")
	bp.printfln("")

	javaLibModules := s.javaLibs(ctx)
	for _, m := range javaLibModules {
		name := m.Name()
		bp.printfln("java_import {")
		bp.indent()
		bp.printfln("name: %q,", versionedSdkMemberName(ctx, name, version))
		bp.printfln("sdk_member_name: %q,", name)
		bp.printfln("jars: [%q],", javaStubFilePathFor(m))
		bp.dedent()
		bp.printfln("}")
		bp.printfln("")

		// This module is for the case when the source tree for the unversioned module
		// doesn't exist (i.e. building in an unbundled tree). "prefer:" is set to false
		// so that this module does not eclipse the unversioned module if it exists.
		bp.printfln("java_import {")
		bp.indent()
		bp.printfln("name: %q,", name)
		bp.printfln("jars: [%q],", javaStubFilePathFor(m))
		bp.printfln("prefer: false,")
		bp.dedent()
		bp.printfln("}")
		bp.printfln("")
	}

	nativeLibInfos := s.nativeMemberInfos(ctx)
	for _, info := range nativeLibInfos {
		bp.printfln("cc_prebuilt_library_shared {")
		bp.indent()
		bp.printfln("name: %q,", versionedSdkMemberName(ctx, info.name, version))
		bp.printfln("sdk_member_name: %q,", info.name)

		// a function for emitting include dirs
		printExportedDirsForNativeLibs := func(lib archSpecificNativeLibInfo, systemInclude bool) {
			includeDirs := nativeIncludeDirPathsFor(ctx, lib, systemInclude, info.hasArchSpecificFlags)
			if len(includeDirs) == 0 {
				return
			}
			if !systemInclude {
				bp.printfln("export_include_dirs: [")
			} else {
				bp.printfln("export_system_include_dirs: [")
			}
			bp.indent()
			for _, dir := range includeDirs {
				bp.printfln("%q,", dir)
			}
			bp.dedent()
			bp.printfln("],")
		}

		if !info.hasArchSpecificFlags {
			printExportedDirsForNativeLibs(info.archVariants[0], false /*systemInclude*/)
			printExportedDirsForNativeLibs(info.archVariants[0], true /*systemInclude*/)
		}

		bp.printfln("arch: {")
		bp.indent()
		for _, av := range info.archVariants {
			bp.printfln("%s: {", av.archType)
			bp.indent()
			bp.printfln("srcs: [%q],", nativeStubFilePathFor(av))
			if info.hasArchSpecificFlags {
				// export_* properties are added inside the arch: {<arch>: {...}} block
				printExportedDirsForNativeLibs(av, false /*systemInclude*/)
				printExportedDirsForNativeLibs(av, true /*systemInclude*/)
			}
			bp.dedent()
			bp.printfln("},") // <arch>
		}
		bp.dedent()
		bp.printfln("},") // arch
		bp.printfln("stl: \"none\",")
		bp.printfln("system_shared_libs: [],")
		bp.dedent()
		bp.printfln("}") // cc_prebuilt_library_shared
		bp.printfln("")
	}

	bp.printfln("sdk_snapshot {")
	bp.indent()
	bp.printfln("name: %q,", ctx.ModuleName()+string(android.SdkVersionSeparator)+version)
	if len(javaLibModules) > 0 {
		bp.printfln("java_libs: [")
		bp.indent()
		for _, m := range javaLibModules {
			bp.printfln("%q,", versionedSdkMemberName(ctx, m.Name(), version))
		}
		bp.dedent()
		bp.printfln("],") // java_libs
	}
	if len(nativeLibInfos) > 0 {
		bp.printfln("native_shared_libs: [")
		bp.indent()
		for _, info := range nativeLibInfos {
			bp.printfln("%q,", versionedSdkMemberName(ctx, info.name, version))
		}
		bp.dedent()
		bp.printfln("],") // native_shared_libs
	}
	bp.dedent()
	bp.printfln("}") // sdk_snapshot
	bp.printfln("")

	bp.build(pctx, ctx, nil)
	return bp.path
}

// buildSnapshot is the main function in this source file. It creates rules to copy
// the contents (header files, stub libraries, etc) into the zip file.
func (s *sdk) buildSnapshot(ctx android.ModuleContext) android.OutputPath {
	snapshotPath := func(paths ...string) android.OutputPath {
		return android.PathForModuleOut(ctx, "snapshot").Join(ctx, paths...)
	}

	var filesToZip android.Paths
	// copy src to dest and add the dest to the zip
	copy := func(src android.Path, dest android.OutputPath) {
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.Cp,
			Input:  src,
			Output: dest,
		})
		filesToZip = append(filesToZip, dest)
	}

	// copy exported AIDL files and stub jar files
	for _, m := range s.javaLibs(ctx) {
		headerJars := m.HeaderJars()
		if len(headerJars) != 1 {
			panic(fmt.Errorf("there must be only one header jar from %q", m.Name()))
		}
		copy(headerJars[0], snapshotPath(javaStubFilePathFor(m)))

		for _, dir := range m.AidlIncludeDirs() {
			// TODO(jiyong): copy parcelable declarations only
			aidlFiles, _ := ctx.GlobWithDeps(dir.String()+"/**/*.aidl", nil)
			for _, file := range aidlFiles {
				copy(android.PathForSource(ctx, file), snapshotPath(aidlIncludeDir, file))
			}
		}
	}

	// copy exported header files and stub *.so files
	nativeLibInfos := s.nativeMemberInfos(ctx)
	for _, info := range nativeLibInfos {

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
					dest := snapshotPath(targetDir, file)
					copy(src, dest)
				}
			}

			genHeaders := lib.exportedDeps
			for _, file := range genHeaders {
				targetDir := nativeGeneratedIncludeDir
				if info.hasArchSpecificFlags {
					targetDir = filepath.Join(lib.archType, targetDir)
				}
				dest := snapshotPath(targetDir, lib.name, file.Rel())
				copy(file, dest)
			}
		}

		if !info.hasArchSpecificFlags {
			printExportedDirCopyCommandsForNativeLibs(info.archVariants[0])
		}

		// for each architecture
		for _, av := range info.archVariants {
			copy(av.outputFile, snapshotPath(nativeStubFilePathFor(av)))

			if info.hasArchSpecificFlags {
				printExportedDirCopyCommandsForNativeLibs(av)
			}
		}
	}

	// generate Android.bp
	bp := s.buildAndroidBp(ctx, "current")
	filesToZip = append(filesToZip, bp)

	// zip them all
	zipFile := android.PathForModuleOut(ctx, ctx.ModuleName()+"-current.zip").OutputPath
	rb := android.NewRuleBuilder()
	rb.Command().
		BuiltTool(ctx, "soong_zip").
		FlagWithArg("-C ", snapshotPath().String()).
		FlagWithRspFileInputList("-l ", filesToZip).
		FlagWithOutput("-o ", zipFile)
	rb.Build(pctx, ctx, "snapshot", "Building snapshot for "+ctx.ModuleName())

	return zipFile
}
