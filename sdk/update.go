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
	"io"
	"path/filepath"
	"strconv"
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

func newGeneratedFile(ctx android.ModuleContext, name string) *generatedFile {
	return &generatedFile{
		path:        android.PathForModuleOut(ctx, name).OutputPath,
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
//            java/<module_name>/stub.jar    : a stub jar for a java library 'module_name'
//         include/
//            bionic/libc/include/stdlib.h   : an exported header file
//         include_gen/
//            com/android/.../IFoo.h : a generated header file
//         <arch>/include/   : arch-specific exported headers
//         <arch>/include_gen/   : arch-specific generated headers
//         <arch>/lib/
//            libFoo.so   : a stub library

const (
	aidlIncludeDir            = "aidl"
	javaStubDir               = "java"
	javaStubFile              = "stub.jar"
	nativeIncludeDir          = "include"
	nativeGeneratedIncludeDir = "include_gen"
	nativeStubDir             = "lib"
	nativeStubFileSuffix      = ".so"
)

// path to the stub file of a java library. Relative to <sdk_root>/<api_dir>
func javaStubFilePathFor(javaLib *java.Library) string {
	return filepath.Join(javaStubDir, javaLib.Name(), javaStubFile)
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
	buildDir := ctx.Config().BuildDir()
	var includeDirs []android.Path
	if !systemInclude {
		includeDirs = lib.exportedIncludeDirs
	} else {
		includeDirs = lib.exportedSystemIncludeDirs
	}
	for _, dir := range includeDirs {
		var path string
		if gen := strings.HasPrefix(dir.String(), buildDir); gen {
			path = filepath.Join(nativeGeneratedIncludeDir, dir.Rel())
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

// A name that uniquely identifies an prebuilt SDK member for a version of SDK snapshot
// This isn't visible to users, so could be changed in future.
func versionedSdkMemberName(ctx android.ModuleContext, memberName string, version string) string {
	return ctx.ModuleName() + "_" + memberName + string(android.SdkVersionSeparator) + version
}

// arm64, arm, x86, x86_64, etc.
func archTypeOf(module android.Module) string {
	return module.Target().Arch.ArchType.String()
}

// buildAndroidBp creates the blueprint file that defines prebuilt modules for each of
// the SDK members, and the entire sdk_snapshot module for the specified version
func (s *sdk) buildAndroidBp(ctx android.ModuleContext, version string) android.OutputPath {
	bp := newGeneratedFile(ctx, "blueprint-"+version+".bp")
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

func (s *sdk) buildScript(ctx android.ModuleContext, version string) android.OutputPath {
	sh := newGeneratedFile(ctx, "update_prebuilt-"+version+".sh")
	buildDir := ctx.Config().BuildDir()

	snapshotPath := func(paths ...string) string {
		return filepath.Join(ctx.ModuleDir(), version, filepath.Join(paths...))
	}

	// TODO(jiyong) instead of creating script, create a zip file having the Android.bp, the headers,
	// and the stubs and put it to the dist directory. The dist'ed zip file then would be downloaded,
	// unzipped and then uploaded to gerrit again.
	sh.printfln("#!/bin/bash")
	sh.printfln("echo Updating snapshot of %s in %s", ctx.ModuleName(), snapshotPath())
	sh.printfln("pushd $ANDROID_BUILD_TOP > /dev/null")
	sh.printfln("mkdir -p %s", snapshotPath(aidlIncludeDir))
	sh.printfln("mkdir -p %s", snapshotPath(javaStubDir))
	sh.printfln("mkdir -p %s", snapshotPath(nativeIncludeDir))
	sh.printfln("mkdir -p %s", snapshotPath(nativeGeneratedIncludeDir))
	for _, target := range ctx.MultiTargets() {
		arch := target.Arch.ArchType.String()
		sh.printfln("mkdir -p %s", snapshotPath(arch, nativeStubDir))
		sh.printfln("mkdir -p %s", snapshotPath(arch, nativeIncludeDir))
		sh.printfln("mkdir -p %s", snapshotPath(arch, nativeGeneratedIncludeDir))
	}

	var implicits android.Paths
	for _, m := range s.javaLibs(ctx) {
		headerJars := m.HeaderJars()
		if len(headerJars) != 1 {
			panic(fmt.Errorf("there must be only one header jar from %q", m.Name()))
		}
		implicits = append(implicits, headerJars...)

		exportedAidlIncludeDirs := m.AidlIncludeDirs()
		for _, dir := range exportedAidlIncludeDirs {
			// Using tar to copy with the directory structure
			// TODO(jiyong): copy parcelable declarations only
			sh.printfln("find %s -name \"*.aidl\" | tar cf - -T - | (cd %s; tar xf -)",
				dir.String(), snapshotPath(aidlIncludeDir))
		}

		copyTarget := snapshotPath(javaStubFilePathFor(m))
		sh.printfln("mkdir -p %s && cp %s %s",
			filepath.Dir(copyTarget), headerJars[0].String(), copyTarget)
	}

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
				gen := strings.HasPrefix(dir.String(), buildDir)
				targetDir := nativeIncludeDir
				if gen {
					targetDir = nativeGeneratedIncludeDir
				}
				if info.hasArchSpecificFlags {
					targetDir = filepath.Join(lib.archType, targetDir)
				}
				targetDir = snapshotPath(targetDir)

				sourceDirRoot := "."
				sourceDirRel := dir.String()
				if gen {
					// ex) out/soong/.intermediate/foo/bar/gen/aidl
					sourceDirRoot = strings.TrimSuffix(dir.String(), dir.Rel())
					sourceDirRel = dir.Rel()
				}
				// TODO(jiyong) copy headers having other suffixes
				sh.printfln("(cd %s; find %s -name \"*.h\" | tar cf - -T - ) | (cd %s; tar xf -)",
					sourceDirRoot, sourceDirRel, targetDir)
			}
		}

		if !info.hasArchSpecificFlags {
			printExportedDirCopyCommandsForNativeLibs(info.archVariants[0])
		}

		// for each architecture
		for _, av := range info.archVariants {
			stub := av.outputFile
			implicits = append(implicits, stub)
			copiedStub := snapshotPath(nativeStubFilePathFor(av))
			sh.printfln("cp %s %s", stub.String(), copiedStub)

			if info.hasArchSpecificFlags {
				printExportedDirCopyCommandsForNativeLibs(av)
			}
		}
	}

	bp := s.buildAndroidBp(ctx, version)
	implicits = append(implicits, bp)
	sh.printfln("cp %s %s", bp.String(), snapshotPath("Android.bp"))

	sh.printfln("popd > /dev/null")
	sh.printfln("rm -- \"$0\"") // self deleting so that stale script is not used
	sh.printfln("echo Done")

	sh.build(pctx, ctx, implicits)
	return sh.path
}

func (s *sdk) buildSnapshotGenerationScripts(ctx android.ModuleContext) {
	if s.snapshot() {
		// we don't need a script for sdk_snapshot.. as they are frozen
		return
	}

	// script to update the 'current' snapshot
	s.updateScript = s.buildScript(ctx, "current")

	versions := s.frozenVersions(ctx)
	newVersion := "1"
	if len(versions) >= 1 {
		lastVersion := versions[len(versions)-1]
		lastVersionNum, err := strconv.Atoi(lastVersion)
		if err != nil {
			panic(err)
			return
		}
		newVersion = strconv.Itoa(lastVersionNum + 1)
	}
	// script to create a new frozen version of snapshot
	s.freezeScript = s.buildScript(ctx, newVersion)
}

func (s *sdk) androidMkEntriesForScript() android.AndroidMkEntries {
	if s.snapshot() {
		// we don't need a script for sdk_snapshot.. as they are frozen
		return android.AndroidMkEntries{}
	}

	entries := android.AndroidMkEntries{
		Class: "FAKE",
		// TODO(jiyong): remove this? but androidmk.go expects OutputFile to be specified anyway
		OutputFile: android.OptionalPathForPath(s.updateScript),
		Include:    "$(BUILD_SYSTEM)/base_rules.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.AddStrings("LOCAL_ADDITIONAL_DEPENDENCIES",
					s.updateScript.String(), s.freezeScript.String())
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
				fmt.Fprintln(w, "$(LOCAL_BUILT_MODULE): $(LOCAL_ADDITIONAL_DEPENDENCIES)")
				fmt.Fprintln(w, "	touch $@")
				fmt.Fprintln(w, "	echo ##################################################")
				fmt.Fprintln(w, "	echo To update current SDK: execute", filepath.Join("\\$$ANDROID_BUILD_TOP", s.updateScript.String()))
				fmt.Fprintln(w, "	echo To freeze current SDK: execute", filepath.Join("\\$$ANDROID_BUILD_TOP", s.freezeScript.String()))
				fmt.Fprintln(w, "	echo ##################################################")
			},
		},
	}
	return entries
}
