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
	"reflect"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
)

var pctx = android.NewPackageContext("android/soong/sdk")

type generatedContents struct {
	content     strings.Builder
	indentLevel int
}

// generatedFile abstracts operations for writing contents into a file and emit a build rule
// for the file.
type generatedFile struct {
	generatedContents
	path android.OutputPath
}

func newGeneratedFile(ctx android.ModuleContext, path ...string) *generatedFile {
	return &generatedFile{
		path: android.PathForModuleOut(ctx, path...).OutputPath,
	}
}

func (gc *generatedContents) Indent() {
	gc.indentLevel++
}

func (gc *generatedContents) Dedent() {
	gc.indentLevel--
}

func (gc *generatedContents) Printfln(format string, args ...interface{}) {
	// ninja consumes newline characters in rspfile_content. Prevent it by
	// escaping the backslash in the newline character. The extra backslash
	// is removed when the rspfile is written to the actual script file
	fmt.Fprintf(&(gc.content), strings.Repeat("    ", gc.indentLevel)+format+"\\n", args...)
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

func (s *sdk) stubsSources(ctx android.ModuleContext) []android.SdkAware {
	result := []android.SdkAware{}
	ctx.VisitDirectDeps(func(m android.Module) {
		if j, ok := m.(*java.Droidstubs); ok {
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

	bpFile := &bpFile{
		modules: make(map[string]*bpModule),
	}

	builder := &snapshotBuilder{
		ctx:             ctx,
		version:         "current",
		snapshotDir:     snapshotDir.OutputPath,
		filesToZip:      []android.Path{bp.path},
		bpFile:          bpFile,
		prebuiltModules: make(map[string]*bpModule),
	}
	s.builderForTests = builder

	// copy exported AIDL files and stub jar files
	javaLibs := s.javaLibs(ctx)
	for _, m := range javaLibs {
		m.BuildSnapshot(ctx, builder)
	}

	// copy stubs sources
	stubsSources := s.stubsSources(ctx)
	for _, m := range stubsSources {
		m.BuildSnapshot(ctx, builder)
	}

	// copy exported header files and stub *.so files
	nativeLibInfos := s.nativeMemberInfos(ctx)
	for _, info := range nativeLibInfos {
		buildSharedNativeLibSnapshot(ctx, info, builder)
	}

	for _, unversioned := range builder.prebuiltOrder {
		// Copy the unversioned module so it can be modified to make it versioned.
		versioned := unversioned.copy()
		name := versioned.properties["name"].(string)
		versioned.setProperty("name", builder.versionedSdkMemberName(name))
		versioned.insertAfter("name", "sdk_member_name", name)
		bpFile.AddModule(versioned)

		// Set prefer: false - this is not strictly required as that is the default.
		unversioned.insertAfter("name", "prefer", false)
		bpFile.AddModule(unversioned)
	}

	// Create the snapshot module.
	snapshotName := ctx.ModuleName() + string(android.SdkVersionSeparator) + builder.version
	snapshotModule := bpFile.newModule("sdk_snapshot")
	snapshotModule.AddProperty("name", snapshotName)
	if len(s.properties.Java_libs) > 0 {
		snapshotModule.AddProperty("java_libs", builder.versionedSdkMemberNames(s.properties.Java_libs))
	}
	if len(s.properties.Stubs_sources) > 0 {
		snapshotModule.AddProperty("stubs_sources", builder.versionedSdkMemberNames(s.properties.Stubs_sources))
	}
	if len(s.properties.Native_shared_libs) > 0 {
		snapshotModule.AddProperty("native_shared_libs", builder.versionedSdkMemberNames(s.properties.Native_shared_libs))
	}
	bpFile.AddModule(snapshotModule)

	// generate Android.bp
	bp = newGeneratedFile(ctx, "snapshot", "Android.bp")
	generateBpContents(&bp.generatedContents, bpFile)

	bp.build(pctx, ctx, nil)

	filesToZip := builder.filesToZip

	// zip them all
	outputZipFile := android.PathForModuleOut(ctx, ctx.ModuleName()+"-current.zip").OutputPath
	outputRuleName := "snapshot"
	outputDesc := "Building snapshot for " + ctx.ModuleName()

	// If there are no zips to merge then generate the output zip directly.
	// Otherwise, generate an intermediate zip file into which other zips can be
	// merged.
	var zipFile android.OutputPath
	var ruleName string
	var desc string
	if len(builder.zipsToMerge) == 0 {
		zipFile = outputZipFile
		ruleName = outputRuleName
		desc = outputDesc
	} else {
		zipFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"-current.unmerged.zip").OutputPath
		ruleName = "intermediate snapshot"
		desc = "Building intermediate snapshot for " + ctx.ModuleName()
	}

	rb := android.NewRuleBuilder()
	rb.Command().
		BuiltTool(ctx, "soong_zip").
		FlagWithArg("-C ", builder.snapshotDir.String()).
		FlagWithRspFileInputList("-l ", filesToZip).
		FlagWithOutput("-o ", zipFile)
	rb.Build(pctx, ctx, ruleName, desc)

	if len(builder.zipsToMerge) != 0 {
		rb := android.NewRuleBuilder()
		rb.Command().
			BuiltTool(ctx, "merge_zips").
			Output(outputZipFile).
			Input(zipFile).
			Inputs(builder.zipsToMerge)
		rb.Build(pctx, ctx, outputRuleName, outputDesc)
	}

	return outputZipFile
}

func generateBpContents(contents *generatedContents, bpFile *bpFile) {
	contents.Printfln("// This is auto-generated. DO NOT EDIT.")
	for _, bpModule := range bpFile.order {
		contents.Printfln("")
		contents.Printfln("%s {", bpModule.moduleType)
		outputPropertySet(contents, &bpModule.bpPropertySet)
		contents.Printfln("}")
	}
	contents.Printfln("")
}

func outputPropertySet(contents *generatedContents, set *bpPropertySet) {
	contents.Indent()
	for _, name := range set.order {
		value := set.properties[name]

		reflectedValue := reflect.ValueOf(value)
		t := reflectedValue.Type()

		kind := t.Kind()
		switch kind {
		case reflect.Slice:
			length := reflectedValue.Len()
			if length > 1 {
				contents.Printfln("%s: [", name)
				contents.Indent()
				for i := 0; i < length; i = i + 1 {
					contents.Printfln("%q,", reflectedValue.Index(i).Interface())
				}
				contents.Dedent()
				contents.Printfln("],")
			} else if length == 0 {
				contents.Printfln("%s: [],", name)
			} else {
				contents.Printfln("%s: [%q],", name, reflectedValue.Index(0).Interface())
			}
		case reflect.Bool:
			contents.Printfln("%s: %t,", name, reflectedValue.Bool())

		case reflect.Ptr:
			contents.Printfln("%s: {", name)
			outputPropertySet(contents, reflectedValue.Interface().(*bpPropertySet))
			contents.Printfln("},")

		default:
			contents.Printfln("%s: %q,", name, value)
		}
	}
	contents.Dedent()
}

func (s *sdk) GetAndroidBpContentsForTests() string {
	contents := &generatedContents{}
	generateBpContents(contents, s.builderForTests.bpFile)
	return contents.content.String()
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

	info.generatePrebuiltLibrary(ctx, builder)
}

func (info *nativeLibInfo) generatePrebuiltLibrary(ctx android.ModuleContext, builder android.SnapshotBuilder) {

	// a function for emitting include dirs
	addExportedDirsForNativeLibs := func(lib archSpecificNativeLibInfo, properties android.BpPropertySet, systemInclude bool) {
		includeDirs := nativeIncludeDirPathsFor(ctx, lib, systemInclude, info.hasArchSpecificFlags)
		if len(includeDirs) == 0 {
			return
		}
		var propertyName string
		if !systemInclude {
			propertyName = "export_include_dirs"
		} else {
			propertyName = "export_system_include_dirs"
		}
		properties.AddProperty(propertyName, includeDirs)
	}

	pbm := builder.AddPrebuiltModule(info.name, "cc_prebuilt_library_shared")

	if !info.hasArchSpecificFlags {
		addExportedDirsForNativeLibs(info.archVariants[0], pbm, false /*systemInclude*/)
		addExportedDirsForNativeLibs(info.archVariants[0], pbm, true /*systemInclude*/)
	}

	archProperties := pbm.AddPropertySet("arch")
	for _, av := range info.archVariants {
		archTypeProperties := archProperties.AddPropertySet(av.archType)
		archTypeProperties.AddProperty("srcs", []string{nativeStubFilePathFor(av)})
		if info.hasArchSpecificFlags {
			// export_* properties are added inside the arch: {<arch>: {...}} block
			addExportedDirsForNativeLibs(av, archTypeProperties, false /*systemInclude*/)
			addExportedDirsForNativeLibs(av, archTypeProperties, true /*systemInclude*/)
		}
	}
	pbm.AddProperty("stl", "none")
	pbm.AddProperty("system_shared_libs", []string{})
}

type snapshotBuilder struct {
	ctx         android.ModuleContext
	version     string
	snapshotDir android.OutputPath
	bpFile      *bpFile
	filesToZip  android.Paths
	zipsToMerge android.Paths

	prebuiltModules map[string]*bpModule
	prebuiltOrder   []*bpModule
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

func (s *snapshotBuilder) UnzipToSnapshot(zipPath android.Path, destDir string) {
	ctx := s.ctx

	// Repackage the zip file so that the entries are in the destDir directory.
	// This will allow the zip file to be merged into the snapshot.
	tmpZipPath := android.PathForModuleOut(ctx, "tmp", destDir+".zip").OutputPath
	rb := android.NewRuleBuilder()
	rb.Command().
		BuiltTool(ctx, "zip2zip").
		FlagWithInput("-i ", zipPath).
		FlagWithOutput("-o ", tmpZipPath).
		Flag("**/*:" + destDir)
	rb.Build(pctx, ctx, "repackaging "+destDir,
		"Repackaging zip file "+destDir+" for snapshot "+ctx.ModuleName())

	// Add the repackaged zip file to the files to merge.
	s.zipsToMerge = append(s.zipsToMerge, tmpZipPath)
}

func (s *snapshotBuilder) AddPrebuiltModule(name string, moduleType string) android.BpModule {
	if s.prebuiltModules[name] != nil {
		panic(fmt.Sprintf("Duplicate module detected, module %s has already been added", name))
	}

	m := s.bpFile.newModule(moduleType)
	m.AddProperty("name", name)

	s.prebuiltModules[name] = m
	s.prebuiltOrder = append(s.prebuiltOrder, m)
	return m
}

// Get a versioned name appropriate for the SDK snapshot version being taken.
func (s *snapshotBuilder) versionedSdkMemberName(unversionedName string) string {
	return versionedSdkMemberName(s.ctx, unversionedName, s.version)
}

func (s *snapshotBuilder) versionedSdkMemberNames(members []string) []string {
	var references []string = nil
	for _, m := range members {
		references = append(references, s.versionedSdkMemberName(m))
	}
	return references
}
