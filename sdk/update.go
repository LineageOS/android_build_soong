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
	"sort"
	"strings"

	"android/soong/apex"
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
// Returns a list containing type (extracted from the dependency tag) and the variant.
func (s *sdk) collectMembers(ctx android.ModuleContext) []sdkMemberRef {
	var memberRefs []sdkMemberRef
	ctx.WalkDeps(func(child android.Module, parent android.Module) bool {
		tag := ctx.OtherModuleDependencyTag(child)
		if memberTag, ok := tag.(android.SdkMemberTypeDependencyTag); ok {
			memberType := memberTag.SdkMemberType()

			// Make sure that the resolved module is allowed in the member list property.
			if !memberType.IsInstance(child) {
				ctx.ModuleErrorf("module %q is not valid in property %s", ctx.OtherModuleName(child), memberType.SdkPropertyName())
			}

			memberRefs = append(memberRefs, sdkMemberRef{memberType, child.(android.SdkAware)})

			// If the member type supports transitive sdk members then recurse down into
			// its dependencies, otherwise exit traversal.
			return memberType.HasTransitiveSdkMembers()
		}

		return false
	})

	return memberRefs
}

// Organize the members.
//
// The members are first grouped by type and then grouped by name. The order of
// the types is the order they are referenced in android.SdkMemberTypesRegistry.
// The names are in the order in which the dependencies were added.
//
// Returns the members as well as the multilib setting to use.
func (s *sdk) organizeMembers(ctx android.ModuleContext, memberRefs []sdkMemberRef) ([]*sdkMember, string) {
	byType := make(map[android.SdkMemberType][]*sdkMember)
	byName := make(map[string]*sdkMember)

	lib32 := false // True if any of the members have 32 bit version.
	lib64 := false // True if any of the members have 64 bit version.

	for _, memberRef := range memberRefs {
		memberType := memberRef.memberType
		variant := memberRef.variant

		name := ctx.OtherModuleName(variant)
		member := byName[name]
		if member == nil {
			member = &sdkMember{memberType: memberType, name: name}
			byName[name] = member
			byType[memberType] = append(byType[memberType], member)
		}

		multilib := variant.Target().Arch.ArchType.Multilib
		if multilib == "lib32" {
			lib32 = true
		} else if multilib == "lib64" {
			lib64 = true
		}

		// Only append new variants to the list. This is needed because a member can be both
		// exported by the sdk and also be a transitive sdk member.
		member.variants = appendUniqueVariants(member.variants, variant)
	}

	var members []*sdkMember
	for _, memberListProperty := range s.memberListProperties() {
		membersOfType := byType[memberListProperty.memberType]
		members = append(members, membersOfType...)
	}

	// Compute the setting of multilib.
	var multilib string
	if lib32 && lib64 {
		multilib = "both"
	} else if lib32 {
		multilib = "32"
	} else if lib64 {
		multilib = "64"
	}

	return members, multilib
}

func appendUniqueVariants(variants []android.SdkAware, newVariant android.SdkAware) []android.SdkAware {
	for _, v := range variants {
		if v == newVariant {
			return variants
		}
	}
	return append(variants, newVariant)
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
func (s *sdk) buildSnapshot(ctx android.ModuleContext, sdkVariants []*sdk) android.OutputPath {

	exportedMembers := make(map[string]struct{})
	var memberRefs []sdkMemberRef
	for _, sdkVariant := range sdkVariants {
		memberRefs = append(memberRefs, sdkVariant.memberRefs...)

		// Merge the exported member sets from all sdk variants.
		for key, _ := range sdkVariant.getExportedMembers() {
			exportedMembers[key] = struct{}{}
		}
	}
	s.exportedMembers = exportedMembers

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

	members, multilib := s.organizeMembers(ctx, memberRefs)
	for _, member := range members {
		memberType := member.memberType
		prebuiltModule := memberType.AddPrebuiltModule(ctx, builder, member)
		if prebuiltModule == nil {
			// Fall back to legacy method of building a snapshot
			memberType.BuildSnapshot(ctx, builder, member)
		} else {
			s.createMemberSnapshot(ctx, builder, member, prebuiltModule)
		}
	}

	// Create a transformer that will transform an unversioned module into a versioned module.
	unversionedToVersionedTransformer := unversionedToVersionedTransformation{builder: builder}

	// Create a transformer that will transform an unversioned module by replacing any references
	// to internal members with a unique module name and setting prefer: false.
	unversionedTransformer := unversionedTransformation{builder: builder}

	for _, unversioned := range builder.prebuiltOrder {
		// Prune any empty property sets.
		unversioned = unversioned.transform(pruneEmptySetTransformer{})

		// Copy the unversioned module so it can be modified to make it versioned.
		versioned := unversioned.deepCopy()

		// Transform the unversioned module into a versioned one.
		versioned.transform(unversionedToVersionedTransformer)
		bpFile.AddModule(versioned)

		// Transform the unversioned module to make it suitable for use in the snapshot.
		unversioned.transform(unversionedTransformer)
		bpFile.AddModule(unversioned)
	}

	// Create the snapshot module.
	snapshotName := ctx.ModuleName() + string(android.SdkVersionSeparator) + builder.version
	var snapshotModuleType string
	if s.properties.Module_exports {
		snapshotModuleType = "module_exports_snapshot"
	} else {
		snapshotModuleType = "sdk_snapshot"
	}
	snapshotModule := bpFile.newModule(snapshotModuleType)
	snapshotModule.AddProperty("name", snapshotName)

	// Make sure that the snapshot has the same visibility as the sdk.
	visibility := android.EffectiveVisibilityRules(ctx, s)
	if len(visibility) != 0 {
		snapshotModule.AddProperty("visibility", visibility)
	}

	addHostDeviceSupportedProperties(s.ModuleBase.DeviceSupported(), s.ModuleBase.HostSupported(), snapshotModule)

	// Compile_multilib defaults to both and must always be set to both on the
	// device and so only needs to be set when targeted at the host and is neither
	// unspecified or both.
	targetPropertySet := snapshotModule.AddPropertySet("target")
	if s.HostSupported() && multilib != "" && multilib != "both" {
		hostSet := targetPropertySet.AddPropertySet("host")
		hostSet.AddProperty("compile_multilib", multilib)
	}

	var dynamicMemberPropertiesList []interface{}
	osTypeToMemberProperties := make(map[android.OsType]*sdk)
	for _, sdkVariant := range sdkVariants {
		properties := sdkVariant.dynamicMemberTypeListProperties
		osTypeToMemberProperties[sdkVariant.Target().Os] = sdkVariant
		dynamicMemberPropertiesList = append(dynamicMemberPropertiesList, properties)
	}

	// Extract the common lists of members into a separate struct.
	commonDynamicMemberProperties := s.dynamicSdkMemberTypes.createMemberListProperties()
	extractCommonProperties(commonDynamicMemberProperties, dynamicMemberPropertiesList)

	// Add properties common to all os types.
	s.addMemberPropertiesToPropertySet(builder, snapshotModule, commonDynamicMemberProperties)

	// Iterate over the os types in a fixed order.
	for _, osType := range s.getPossibleOsTypes() {
		if sdkVariant, ok := osTypeToMemberProperties[osType]; ok {
			osPropertySet := targetPropertySet.AddPropertySet(sdkVariant.Target().Os.Name)
			s.addMemberPropertiesToPropertySet(builder, osPropertySet, sdkVariant.dynamicMemberTypeListProperties)
		}
	}

	// Prune any empty property sets.
	snapshotModule.transform(pruneEmptySetTransformer{})

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

func (s *sdk) addMemberPropertiesToPropertySet(builder *snapshotBuilder, propertySet android.BpPropertySet, dynamicMemberTypeListProperties interface{}) {
	for _, memberListProperty := range s.memberListProperties() {
		names := memberListProperty.getter(dynamicMemberTypeListProperties)
		if len(names) > 0 {
			propertySet.AddProperty(memberListProperty.propertyName(), builder.versionedSdkMemberNames(names))
		}
	}
}

type propertyTag struct {
	name string
}

// A BpPropertyTag to add to a property that contains references to other sdk members.
//
// This will cause the references to be rewritten to a versioned reference in the version
// specific instance of a snapshot module.
var sdkMemberReferencePropertyTag = propertyTag{"sdkMemberReferencePropertyTag"}

// A BpPropertyTag that indicates the property should only be present in the versioned
// module.
//
// This will cause the property to be removed from the unversioned instance of a
// snapshot module.
var sdkVersionedOnlyPropertyTag = propertyTag{"sdkVersionedOnlyPropertyTag"}

type unversionedToVersionedTransformation struct {
	identityTransformation
	builder *snapshotBuilder
}

func (t unversionedToVersionedTransformation) transformModule(module *bpModule) *bpModule {
	// Use a versioned name for the module but remember the original name for the
	// snapshot.
	name := module.getValue("name").(string)
	module.setProperty("name", t.builder.versionedSdkMemberName(name))
	module.insertAfter("name", "sdk_member_name", name)
	return module
}

func (t unversionedToVersionedTransformation) transformProperty(name string, value interface{}, tag android.BpPropertyTag) (interface{}, android.BpPropertyTag) {
	if tag == sdkMemberReferencePropertyTag {
		return t.builder.versionedSdkMemberNames(value.([]string)), tag
	} else {
		return value, tag
	}
}

type unversionedTransformation struct {
	identityTransformation
	builder *snapshotBuilder
}

func (t unversionedTransformation) transformModule(module *bpModule) *bpModule {
	// If the module is an internal member then use a unique name for it.
	name := module.getValue("name").(string)
	module.setProperty("name", t.builder.unversionedSdkMemberName(name))

	// Set prefer: false - this is not strictly required as that is the default.
	module.insertAfter("name", "prefer", false)

	return module
}

func (t unversionedTransformation) transformProperty(name string, value interface{}, tag android.BpPropertyTag) (interface{}, android.BpPropertyTag) {
	if tag == sdkMemberReferencePropertyTag {
		return t.builder.unversionedSdkMemberNames(value.([]string)), tag
	} else if tag == sdkVersionedOnlyPropertyTag {
		// The property is not allowed in the unversioned module so remove it.
		return nil, nil
	} else {
		return value, tag
	}
}

type pruneEmptySetTransformer struct {
	identityTransformation
}

var _ bpTransformer = (*pruneEmptySetTransformer)(nil)

func (t pruneEmptySetTransformer) transformPropertySetAfterContents(name string, propertySet *bpPropertySet, tag android.BpPropertyTag) (*bpPropertySet, android.BpPropertyTag) {
	if len(propertySet.properties) == 0 {
		return nil, nil
	} else {
		return propertySet, tag
	}
}

func generateBpContents(contents *generatedContents, bpFile *bpFile) {
	contents.Printfln("// This is auto-generated. DO NOT EDIT.")
	for _, bpModule := range bpFile.order {
		contents.Printfln("")
		contents.Printfln("%s {", bpModule.moduleType)
		outputPropertySet(contents, bpModule.bpPropertySet)
		contents.Printfln("}")
	}
}

func outputPropertySet(contents *generatedContents, set *bpPropertySet) {
	contents.Indent()
	for _, name := range set.order {
		value := set.getValue(name)

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

	variant := member.Variants()[0]

	if s.sdk.isInternalMember(name) {
		// An internal member is only referenced from the sdk snapshot which is in the
		// same package so can be marked as private.
		m.AddProperty("visibility", []string{"//visibility:private"})
	} else {
		// Extract visibility information from a member variant. All variants have the same
		// visibility so it doesn't matter which one is used.
		visibility := android.EffectiveVisibilityRules(s.ctx, variant)
		if len(visibility) != 0 {
			m.AddProperty("visibility", visibility)
		}
	}

	deviceSupported := false
	hostSupported := false

	for _, variant := range member.Variants() {
		osClass := variant.Target().Os.Class
		if osClass == android.Host || osClass == android.HostCross {
			hostSupported = true
		} else if osClass == android.Device {
			deviceSupported = true
		}
	}

	addHostDeviceSupportedProperties(deviceSupported, hostSupported, m)

	// Where available copy apex_available properties from the member.
	if apexAware, ok := variant.(interface{ ApexAvailable() []string }); ok {
		apexAvailable := apexAware.ApexAvailable()

		// Add in any white listed apex available settings.
		apexAvailable = append(apexAvailable, apex.WhitelistedApexAvailable(member.Name())...)

		if len(apexAvailable) > 0 {
			// Remove duplicates and sort.
			apexAvailable = android.FirstUniqueStrings(apexAvailable)
			sort.Strings(apexAvailable)

			m.AddProperty("apex_available", apexAvailable)
		}
	}

	// Disable installation in the versioned module of those modules that are ever installable.
	if installable, ok := variant.(interface{ EverInstallable() bool }); ok {
		if installable.EverInstallable() {
			m.AddPropertyWithTag("installable", false, sdkVersionedOnlyPropertyTag)
		}
	}

	s.prebuiltModules[name] = m
	s.prebuiltOrder = append(s.prebuiltOrder, m)
	return m
}

func addHostDeviceSupportedProperties(deviceSupported bool, hostSupported bool, bpModule *bpModule) {
	if !deviceSupported {
		bpModule.AddProperty("device_supported", false)
	}
	if hostSupported {
		bpModule.AddProperty("host_supported", true)
	}
}

func (s *snapshotBuilder) SdkMemberReferencePropertyTag() android.BpPropertyTag {
	return sdkMemberReferencePropertyTag
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

// Get an internal name unique to the sdk.
func (s *snapshotBuilder) unversionedSdkMemberName(unversionedName string) string {
	if s.sdk.isInternalMember(unversionedName) {
		return s.ctx.ModuleName() + "_" + unversionedName
	} else {
		return unversionedName
	}
}

func (s *snapshotBuilder) unversionedSdkMemberNames(members []string) []string {
	var references []string = nil
	for _, m := range members {
		references = append(references, s.unversionedSdkMemberName(m))
	}
	return references
}

type sdkMemberRef struct {
	memberType android.SdkMemberType
	variant    android.SdkAware
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

type baseInfo struct {
	Properties android.SdkMemberProperties
}

type osTypeSpecificInfo struct {
	baseInfo

	// The list of arch type specific info for this os type.
	archTypes []*archTypeSpecificInfo

	// True if the member has common arch variants for this os type.
	commonArch bool
}

type archTypeSpecificInfo struct {
	baseInfo

	archType android.ArchType
}

func (s *sdk) createMemberSnapshot(sdkModuleContext android.ModuleContext, builder *snapshotBuilder, member *sdkMember, bpModule android.BpModule) {

	memberType := member.memberType

	// Group the variants by os type.
	variantsByOsType := make(map[android.OsType][]android.SdkAware)
	variants := member.Variants()
	for _, variant := range variants {
		osType := variant.Target().Os
		variantsByOsType[osType] = append(variantsByOsType[osType], variant)
	}

	osCount := len(variantsByOsType)
	createVariantPropertiesStruct := func(os android.OsType) android.SdkMemberProperties {
		properties := memberType.CreateVariantPropertiesStruct()
		base := properties.Base()
		base.Os_count = osCount
		base.Os = os
		return properties
	}

	osTypeToInfo := make(map[android.OsType]*osTypeSpecificInfo)

	// The set of properties that are common across all architectures and os types.
	commonProperties := createVariantPropertiesStruct(android.CommonOS)

	// The list of property structures which are os type specific but common across
	// architectures within that os type.
	var osSpecificPropertiesList []android.SdkMemberProperties

	for osType, osTypeVariants := range variantsByOsType {
		// Group the properties for each variant by arch type within the os.
		osInfo := &osTypeSpecificInfo{}
		osTypeToInfo[osType] = osInfo

		// Create a structure into which properties common across the architectures in
		// this os type will be stored. Add it to the list of os type specific yet
		// architecture independent properties structs.
		osInfo.Properties = createVariantPropertiesStruct(osType)
		osSpecificPropertiesList = append(osSpecificPropertiesList, osInfo.Properties)

		commonArch := false
		for _, variant := range osTypeVariants {
			var properties android.SdkMemberProperties

			// Get the info associated with the arch type inside the os info.
			archType := variant.Target().Arch.ArchType

			if archType.Name == "common" {
				// The arch type is common so populate the common properties directly.
				properties = osInfo.Properties

				commonArch = true
			} else {
				archInfo := &archTypeSpecificInfo{archType: archType}
				properties = createVariantPropertiesStruct(osType)
				archInfo.Properties = properties

				osInfo.archTypes = append(osInfo.archTypes, archInfo)
			}

			properties.PopulateFromVariant(variant)
		}

		if commonArch {
			if len(osTypeVariants) != 1 {
				panic("Expected to only have 1 variant when arch type is common but found " + string(len(variants)))
			}
		} else {
			var archPropertiesList []android.SdkMemberProperties
			for _, archInfo := range osInfo.archTypes {
				archPropertiesList = append(archPropertiesList, archInfo.Properties)
			}

			extractCommonProperties(osInfo.Properties, archPropertiesList)

			// Choose setting for compile_multilib that is appropriate for the arch variants supplied.
			var multilib string
			archVariantCount := len(osInfo.archTypes)
			if archVariantCount == 2 {
				multilib = "both"
			} else if archVariantCount == 1 {
				if strings.HasSuffix(osInfo.archTypes[0].archType.Name, "64") {
					multilib = "64"
				} else {
					multilib = "32"
				}
			}

			osInfo.commonArch = commonArch
			osInfo.Properties.Base().Compile_multilib = multilib
		}
	}

	// Extract properties which are common across all architectures and os types.
	extractCommonProperties(commonProperties, osSpecificPropertiesList)

	// Add the common properties to the module.
	commonProperties.AddToPropertySet(sdkModuleContext, builder, bpModule)

	// Create a target property set into which target specific properties can be
	// added.
	targetPropertySet := bpModule.AddPropertySet("target")

	// Iterate over the os types in a fixed order.
	for _, osType := range s.getPossibleOsTypes() {
		osInfo := osTypeToInfo[osType]
		if osInfo == nil {
			continue
		}

		var osPropertySet android.BpPropertySet
		var archOsPrefix string
		if len(osTypeToInfo) == 1 {
			// There is only one os type present in the variants sp don't bother
			// with adding target specific properties.

			// Create a structure that looks like:
			// module_type {
			//   name: "...",
			//   ...
			//   <common properties>
			//   ...
			//   <single os type specific properties>
			//
			//   arch: {
			//     <arch specific sections>
			//   }
			//
			osPropertySet = bpModule

			// Arch specific properties need to be added to an arch specific section
			// within arch.
			archOsPrefix = ""
		} else {
			// Create a structure that looks like:
			// module_type {
			//   name: "...",
			//   ...
			//   <common properties>
			//   ...
			//   target: {
			//     <arch independent os specific sections, e.g. android>
			//     ...
			//     <arch and os specific sections, e.g. android_x86>
			//   }
			//
			osPropertySet = targetPropertySet.AddPropertySet(osType.Name)

			// Arch specific properties need to be added to an os and arch specific
			// section prefixed with <os>_.
			archOsPrefix = osType.Name + "_"
		}

		osInfo.Properties.AddToPropertySet(sdkModuleContext, builder, osPropertySet)
		if !osInfo.commonArch {
			// Either add the arch specific sections into the target or arch sections
			// depending on whether they will also be os specific.
			var archPropertySet android.BpPropertySet
			if archOsPrefix == "" {
				archPropertySet = osPropertySet.AddPropertySet("arch")
			} else {
				archPropertySet = targetPropertySet
			}

			// Add arch (and possibly os) specific sections for each set of
			// arch (and possibly os) specific properties.
			for _, av := range osInfo.archTypes {
				archTypePropertySet := archPropertySet.AddPropertySet(archOsPrefix + av.archType.Name)

				av.Properties.AddToPropertySet(sdkModuleContext, builder, archTypePropertySet)
			}
		}
	}

	memberType.FinalizeModule(sdkModuleContext, builder, member, bpModule)
}

// Compute the list of possible os types that this sdk could support.
func (s *sdk) getPossibleOsTypes() []android.OsType {
	var osTypes []android.OsType
	for _, osType := range android.OsTypeList {
		if s.DeviceSupported() {
			if osType.Class == android.Device && osType != android.Fuchsia {
				osTypes = append(osTypes, osType)
			}
		}
		if s.HostSupported() {
			if osType.Class == android.Host || osType.Class == android.HostCross {
				osTypes = append(osTypes, osType)
			}
		}
	}
	sort.SliceStable(osTypes, func(i, j int) bool { return osTypes[i].Name < osTypes[j].Name })
	return osTypes
}

// Extract common properties from a slice of property structures of the same type.
//
// All the property structures must be of the same type.
// commonProperties - must be a pointer to the structure into which common properties will be added.
// inputPropertiesSlice - must be a slice of input properties structures.
//
// Iterates over each exported field (capitalized name) and checks to see whether they
// have the same value (using DeepEquals) across all the input properties. If it does not then no
// change is made. Otherwise, the common value is stored in the field in the commonProperties
// and the field in each of the input properties structure is set to its default value.
func extractCommonProperties(commonProperties interface{}, inputPropertiesSlice interface{}) {
	commonPropertiesValue := reflect.ValueOf(commonProperties)
	commonStructValue := commonPropertiesValue.Elem()
	propertiesStructType := commonStructValue.Type()

	// Create an empty structure from which default values for the field can be copied.
	emptyStructValue := reflect.New(propertiesStructType).Elem()

	for f := 0; f < propertiesStructType.NumField(); f++ {
		// Check to see if all the structures have the same value for the field. The commonValue
		// is nil on entry to the loop and if it is nil on exit then there is no common value,
		// otherwise it points to the common value.
		var commonValue *reflect.Value
		sliceValue := reflect.ValueOf(inputPropertiesSlice)

		field := propertiesStructType.Field(f)
		if field.Name == "SdkMemberPropertiesBase" {
			continue
		}

		for i := 0; i < sliceValue.Len(); i++ {
			structValue := sliceValue.Index(i).Elem().Elem()
			fieldValue := structValue.Field(f)
			if !fieldValue.CanInterface() {
				// The field is not exported so ignore it.
				continue
			}

			if commonValue == nil {
				// Use the first value as the commonProperties value.
				commonValue = &fieldValue
			} else {
				// If the value does not match the current common value then there is
				// no value in common so break out.
				if !reflect.DeepEqual(fieldValue.Interface(), commonValue.Interface()) {
					commonValue = nil
					break
				}
			}
		}

		// If the fields all have a common value then store it in the common struct field
		// and set the input struct's field to the empty value.
		if commonValue != nil {
			emptyValue := emptyStructValue.Field(f)
			commonStructValue.Field(f).Set(*commonValue)
			for i := 0; i < sliceValue.Len(); i++ {
				structValue := sliceValue.Index(i).Elem().Elem()
				fieldValue := structValue.Field(f)
				fieldValue.Set(emptyValue)
			}
		}
	}
}
