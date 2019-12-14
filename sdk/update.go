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
	"reflect"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

var pctx = android.NewPackageContext("android/soong/sdk")

var (
	repackageZip = pctx.AndroidStaticRule("SnapshotRepackageZip",
		blueprint.RuleParams{
			Command: `${config.Zip2ZipCmd} -i $in -o $out -x META-INF/**/* "**/*:$destdir"`,
			CommandDeps: []string{
				"${config.Zip2ZipCmd}",
			},
		},
		"destdir")

	zipFiles = pctx.AndroidStaticRule("SnapshotZipFiles",
		blueprint.RuleParams{
			Command: `${config.SoongZipCmd} -C $basedir -l $out.rsp -o $out`,
			CommandDeps: []string{
				"${config.SoongZipCmd}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		},
		"basedir")

	mergeZips = pctx.AndroidStaticRule("SnapshotMergeZips",
		blueprint.RuleParams{
			Command: `${config.MergeZipsCmd} $out $in`,
			CommandDeps: []string{
				"${config.MergeZipsCmd}",
			},
		})
)

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

// Collect all the members.
//
// The members are first grouped by type and then grouped by name. The order of
// the types is the order they are referenced in android.SdkMemberTypes. The
// names are in order in which the dependencies were added.
func (s *sdk) collectMembers(ctx android.ModuleContext) []*sdkMember {
	byType := make(map[android.SdkMemberType][]*sdkMember)
	byName := make(map[string]*sdkMember)

	ctx.VisitDirectDeps(func(m android.Module) {
		tag := ctx.OtherModuleDependencyTag(m)
		if memberTag, ok := tag.(*sdkMemberDependencyTag); ok {
			memberType := memberTag.memberType

			// Make sure that the resolved module is allowed in the member list property.
			if !memberType.IsInstance(m) {
				ctx.ModuleErrorf("module %q is not valid in property %s", ctx.OtherModuleName(m), memberType.SdkPropertyName())
			}

			name := ctx.OtherModuleName(m)

			member := byName[name]
			if member == nil {
				member = &sdkMember{memberType: memberType, name: name}
				byName[name] = member
				byType[memberType] = append(byType[memberType], member)
			}

			member.variants = append(member.variants, m.(android.SdkAware))
		}
	})

	var members []*sdkMember
	for _, memberListProperty := range s.dynamicSdkMemberTypes.memberListProperties {
		membersOfType := byType[memberListProperty.memberType]
		members = append(members, membersOfType...)
	}

	return members
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
		sdk:             s,
		version:         "current",
		snapshotDir:     snapshotDir.OutputPath,
		copies:          make(map[string]string),
		filesToZip:      []android.Path{bp.path},
		bpFile:          bpFile,
		prebuiltModules: make(map[string]*bpModule),
	}
	s.builderForTests = builder

	for _, member := range s.collectMembers(ctx) {
		member.memberType.BuildSnapshot(ctx, builder, member)
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

	// Make sure that the snapshot has the same visibility as the sdk.
	visibility := android.EffectiveVisibilityRules(ctx, s)
	if len(visibility) != 0 {
		snapshotModule.AddProperty("visibility", visibility)
	}

	addHostDeviceSupportedProperties(&s.ModuleBase, snapshotModule)
	for _, memberListProperty := range s.dynamicSdkMemberTypes.memberListProperties {
		names := memberListProperty.getter(s.dynamicMemberTypeListProperties)
		if len(names) > 0 {
			snapshotModule.AddProperty(memberListProperty.propertyName(), builder.versionedSdkMemberNames(names))
		}
	}
	bpFile.AddModule(snapshotModule)

	// generate Android.bp
	bp = newGeneratedFile(ctx, "snapshot", "Android.bp")
	generateBpContents(&bp.generatedContents, bpFile)

	bp.build(pctx, ctx, nil)

	filesToZip := builder.filesToZip

	// zip them all
	outputZipFile := android.PathForModuleOut(ctx, ctx.ModuleName()+"-current.zip").OutputPath
	outputDesc := "Building snapshot for " + ctx.ModuleName()

	// If there are no zips to merge then generate the output zip directly.
	// Otherwise, generate an intermediate zip file into which other zips can be
	// merged.
	var zipFile android.OutputPath
	var desc string
	if len(builder.zipsToMerge) == 0 {
		zipFile = outputZipFile
		desc = outputDesc
	} else {
		zipFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"-current.unmerged.zip").OutputPath
		desc = "Building intermediate snapshot for " + ctx.ModuleName()
	}

	ctx.Build(pctx, android.BuildParams{
		Description: desc,
		Rule:        zipFiles,
		Inputs:      filesToZip,
		Output:      zipFile,
		Args: map[string]string{
			"basedir": builder.snapshotDir.String(),
		},
	})

	if len(builder.zipsToMerge) != 0 {
		ctx.Build(pctx, android.BuildParams{
			Description: outputDesc,
			Rule:        mergeZips,
			Input:       zipFile,
			Inputs:      builder.zipsToMerge,
			Output:      outputZipFile,
		})
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

type snapshotBuilder struct {
	ctx         android.ModuleContext
	sdk         *sdk
	version     string
	snapshotDir android.OutputPath
	bpFile      *bpFile

	// Map from destination to source of each copy - used to eliminate duplicates and
	// detect conflicts.
	copies map[string]string

	filesToZip  android.Paths
	zipsToMerge android.Paths

	prebuiltModules map[string]*bpModule
	prebuiltOrder   []*bpModule
}

func (s *snapshotBuilder) CopyToSnapshot(src android.Path, dest string) {
	if existing, ok := s.copies[dest]; ok {
		if existing != src.String() {
			s.ctx.ModuleErrorf("conflicting copy, %s copied from both %s and %s", dest, existing, src)
			return
		}
	} else {
		path := s.snapshotDir.Join(s.ctx, dest)
		s.ctx.Build(pctx, android.BuildParams{
			Rule:   android.Cp,
			Input:  src,
			Output: path,
		})
		s.filesToZip = append(s.filesToZip, path)

		s.copies[dest] = src.String()
	}
}

func (s *snapshotBuilder) UnzipToSnapshot(zipPath android.Path, destDir string) {
	ctx := s.ctx

	// Repackage the zip file so that the entries are in the destDir directory.
	// This will allow the zip file to be merged into the snapshot.
	tmpZipPath := android.PathForModuleOut(ctx, "tmp", destDir+".zip").OutputPath

	ctx.Build(pctx, android.BuildParams{
		Description: "Repackaging zip file " + destDir + " for snapshot " + ctx.ModuleName(),
		Rule:        repackageZip,
		Input:       zipPath,
		Output:      tmpZipPath,
		Args: map[string]string{
			"destdir": destDir,
		},
	})

	// Add the repackaged zip file to the files to merge.
	s.zipsToMerge = append(s.zipsToMerge, tmpZipPath)
}

func (s *snapshotBuilder) AddPrebuiltModule(member android.SdkMember, moduleType string) android.BpModule {
	name := member.Name()
	if s.prebuiltModules[name] != nil {
		panic(fmt.Sprintf("Duplicate module detected, module %s has already been added", name))
	}

	m := s.bpFile.newModule(moduleType)
	m.AddProperty("name", name)

	// Extract visibility information from a member variant. All variants have the same
	// visibility so it doesn't matter which one is used.
	visibility := android.EffectiveVisibilityRules(s.ctx, member.Variants()[0])
	if len(visibility) != 0 {
		m.AddProperty("visibility", visibility)
	}

	addHostDeviceSupportedProperties(&s.sdk.ModuleBase, m)

	s.prebuiltModules[name] = m
	s.prebuiltOrder = append(s.prebuiltOrder, m)
	return m
}

func addHostDeviceSupportedProperties(module *android.ModuleBase, bpModule *bpModule) {
	if !module.DeviceSupported() {
		bpModule.AddProperty("device_supported", false)
	}
	if module.HostSupported() {
		bpModule.AddProperty("host_supported", true)
	}
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

var _ android.SdkMember = (*sdkMember)(nil)

type sdkMember struct {
	memberType android.SdkMemberType
	name       string
	variants   []android.SdkAware
}

func (m *sdkMember) Name() string {
	return m.name
}

func (m *sdkMember) Variants() []android.SdkAware {
	return m.variants
}
