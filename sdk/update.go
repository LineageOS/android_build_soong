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
	"android/soong/cc"
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
// Returns a list containing type (extracted from the dependency tag) and the variant
// plus the multilib usages.
func (s *sdk) collectMembers(ctx android.ModuleContext) {
	s.multilibUsages = multilibNone
	ctx.WalkDeps(func(child android.Module, parent android.Module) bool {
		tag := ctx.OtherModuleDependencyTag(child)
		if memberTag, ok := tag.(android.SdkMemberTypeDependencyTag); ok {
			memberType := memberTag.SdkMemberType()

			// Make sure that the resolved module is allowed in the member list property.
			if !memberType.IsInstance(child) {
				ctx.ModuleErrorf("module %q is not valid in property %s", ctx.OtherModuleName(child), memberType.SdkPropertyName())
			}

			// Keep track of which multilib variants are used by the sdk.
			s.multilibUsages = s.multilibUsages.addArchType(child.Target().Arch.ArchType)

			s.memberRefs = append(s.memberRefs, sdkMemberRef{memberType, child.(android.SdkAware)})

			// If the member type supports transitive sdk members then recurse down into
			// its dependencies, otherwise exit traversal.
			return memberType.HasTransitiveSdkMembers()
		}

		return false
	})
}

// Organize the members.
//
// The members are first grouped by type and then grouped by name. The order of
// the types is the order they are referenced in android.SdkMemberTypesRegistry.
// The names are in the order in which the dependencies were added.
//
// Returns the members as well as the multilib setting to use.
func (s *sdk) organizeMembers(ctx android.ModuleContext, memberRefs []sdkMemberRef) []*sdkMember {
	byType := make(map[android.SdkMemberType][]*sdkMember)
	byName := make(map[string]*sdkMember)

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

		// Only append new variants to the list. This is needed because a member can be both
		// exported by the sdk and also be a transitive sdk member.
		member.variants = appendUniqueVariants(member.variants, variant)
	}

	var members []*sdkMember
	for _, memberListProperty := range s.memberListProperties() {
		membersOfType := byType[memberListProperty.memberType]
		members = append(members, membersOfType...)
	}

	return members
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

	allMembersByName := make(map[string]struct{})
	exportedMembersByName := make(map[string]struct{})
	var memberRefs []sdkMemberRef
	for _, sdkVariant := range sdkVariants {
		memberRefs = append(memberRefs, sdkVariant.memberRefs...)

		// Record the names of all the members, both explicitly specified and implicitly
		// included.
		for _, memberRef := range sdkVariant.memberRefs {
			allMembersByName[memberRef.variant.Name()] = struct{}{}
		}

		// Merge the exported member sets from all sdk variants.
		for key, _ := range sdkVariant.getExportedMembers() {
			exportedMembersByName[key] = struct{}{}
		}
	}

	snapshotDir := android.PathForModuleOut(ctx, "snapshot")

	bp := newGeneratedFile(ctx, "snapshot", "Android.bp")

	bpFile := &bpFile{
		modules: make(map[string]*bpModule),
	}

	builder := &snapshotBuilder{
		ctx:                   ctx,
		sdk:                   s,
		version:               "current",
		snapshotDir:           snapshotDir.OutputPath,
		copies:                make(map[string]string),
		filesToZip:            []android.Path{bp.path},
		bpFile:                bpFile,
		prebuiltModules:       make(map[string]*bpModule),
		allMembersByName:      allMembersByName,
		exportedMembersByName: exportedMembersByName,
	}
	s.builderForTests = builder

	members := s.organizeMembers(ctx, memberRefs)
	for _, member := range members {
		memberType := member.memberType

		memberCtx := &memberContext{ctx, builder, memberType, member.name}

		prebuiltModule := memberType.AddPrebuiltModule(memberCtx, member)
		s.createMemberSnapshot(memberCtx, member, prebuiltModule)
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

	var dynamicMemberPropertiesList []interface{}
	osTypeToMemberProperties := make(map[android.OsType]*sdk)
	for _, sdkVariant := range sdkVariants {
		properties := sdkVariant.dynamicMemberTypeListProperties
		osTypeToMemberProperties[sdkVariant.Target().Os] = sdkVariant
		dynamicMemberPropertiesList = append(dynamicMemberPropertiesList, properties)
	}

	// Extract the common lists of members into a separate struct.
	commonDynamicMemberProperties := s.dynamicSdkMemberTypes.createMemberListProperties()
	extractor := newCommonValueExtractor(commonDynamicMemberProperties)
	extractor.extractCommonProperties(commonDynamicMemberProperties, dynamicMemberPropertiesList)

	// Add properties common to all os types.
	s.addMemberPropertiesToPropertySet(builder, snapshotModule, commonDynamicMemberProperties)

	// Iterate over the os types in a fixed order.
	targetPropertySet := snapshotModule.AddPropertySet("target")
	for _, osType := range s.getPossibleOsTypes() {
		if sdkVariant, ok := osTypeToMemberProperties[osType]; ok {
			osPropertySet := targetPropertySet.AddPropertySet(sdkVariant.Target().Os.Name)

			// Compile_multilib defaults to both and must always be set to both on the
			// device and so only needs to be set when targeted at the host and is neither
			// unspecified or both.
			multilib := sdkVariant.multilibUsages
			if (osType.Class == android.Host || osType.Class == android.HostCross) &&
				multilib != multilibNone && multilib != multilibBoth {
				osPropertySet.AddProperty("compile_multilib", multilib.String())
			}

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
			propertySet.AddProperty(memberListProperty.propertyName(), builder.versionedSdkMemberNames(names, false))
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
var requiredSdkMemberReferencePropertyTag = propertyTag{"requiredSdkMemberReferencePropertyTag"}
var optionalSdkMemberReferencePropertyTag = propertyTag{"optionalSdkMemberReferencePropertyTag"}

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
	module.setProperty("name", t.builder.versionedSdkMemberName(name, true))
	module.insertAfter("name", "sdk_member_name", name)
	return module
}

func (t unversionedToVersionedTransformation) transformProperty(name string, value interface{}, tag android.BpPropertyTag) (interface{}, android.BpPropertyTag) {
	if tag == requiredSdkMemberReferencePropertyTag || tag == optionalSdkMemberReferencePropertyTag {
		required := tag == requiredSdkMemberReferencePropertyTag
		return t.builder.versionedSdkMemberNames(value.([]string), required), tag
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
	module.setProperty("name", t.builder.unversionedSdkMemberName(name, true))

	// Set prefer: false - this is not strictly required as that is the default.
	module.insertAfter("name", "prefer", false)

	return module
}

func (t unversionedTransformation) transformProperty(name string, value interface{}, tag android.BpPropertyTag) (interface{}, android.BpPropertyTag) {
	if tag == requiredSdkMemberReferencePropertyTag || tag == optionalSdkMemberReferencePropertyTag {
		required := tag == requiredSdkMemberReferencePropertyTag
		return t.builder.unversionedSdkMemberNames(value.([]string), required), tag
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

	// Output the properties first, followed by the nested sets. This ensures a
	// consistent output irrespective of whether property sets are created before
	// or after the properties. This simplifies the creation of the module.
	for _, name := range set.order {
		value := set.getValue(name)

		switch v := value.(type) {
		case []string:
			length := len(v)
			if length > 1 {
				contents.Printfln("%s: [", name)
				contents.Indent()
				for i := 0; i < length; i = i + 1 {
					contents.Printfln("%q,", v[i])
				}
				contents.Dedent()
				contents.Printfln("],")
			} else if length == 0 {
				contents.Printfln("%s: [],", name)
			} else {
				contents.Printfln("%s: [%q],", name, v[0])
			}

		case bool:
			contents.Printfln("%s: %t,", name, v)

		case *bpPropertySet:
			// Do not write property sets in the properties phase.

		default:
			contents.Printfln("%s: %q,", name, value)
		}
	}

	for _, name := range set.order {
		value := set.getValue(name)

		// Only write property sets in the sets phase.
		switch v := value.(type) {
		case *bpPropertySet:
			contents.Printfln("%s: {", name)
			outputPropertySet(contents, v)
			contents.Printfln("},")
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

	// The set of all members by name.
	allMembersByName map[string]struct{}

	// The set of exported members by name.
	exportedMembersByName map[string]struct{}
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

	if s.isInternalMember(name) {
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

func (s *snapshotBuilder) SdkMemberReferencePropertyTag(required bool) android.BpPropertyTag {
	if required {
		return requiredSdkMemberReferencePropertyTag
	} else {
		return optionalSdkMemberReferencePropertyTag
	}
}

func (s *snapshotBuilder) OptionalSdkMemberReferencePropertyTag() android.BpPropertyTag {
	return optionalSdkMemberReferencePropertyTag
}

// Get a versioned name appropriate for the SDK snapshot version being taken.
func (s *snapshotBuilder) versionedSdkMemberName(unversionedName string, required bool) string {
	if _, ok := s.allMembersByName[unversionedName]; !ok {
		if required {
			s.ctx.ModuleErrorf("Required member reference %s is not a member of the sdk", unversionedName)
		}
		return unversionedName
	}
	return versionedSdkMemberName(s.ctx, unversionedName, s.version)
}

func (s *snapshotBuilder) versionedSdkMemberNames(members []string, required bool) []string {
	var references []string = nil
	for _, m := range members {
		references = append(references, s.versionedSdkMemberName(m, required))
	}
	return references
}

// Get an internal name unique to the sdk.
func (s *snapshotBuilder) unversionedSdkMemberName(unversionedName string, required bool) string {
	if _, ok := s.allMembersByName[unversionedName]; !ok {
		if required {
			s.ctx.ModuleErrorf("Required member reference %s is not a member of the sdk", unversionedName)
		}
		return unversionedName
	}

	if s.isInternalMember(unversionedName) {
		return s.ctx.ModuleName() + "_" + unversionedName
	} else {
		return unversionedName
	}
}

func (s *snapshotBuilder) unversionedSdkMemberNames(members []string, required bool) []string {
	var references []string = nil
	for _, m := range members {
		references = append(references, s.unversionedSdkMemberName(m, required))
	}
	return references
}

func (s *snapshotBuilder) isInternalMember(memberName string) bool {
	_, ok := s.exportedMembersByName[memberName]
	return !ok
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

// Track usages of multilib variants.
type multilibUsage int

const (
	multilibNone multilibUsage = 0
	multilib32   multilibUsage = 1
	multilib64   multilibUsage = 2
	multilibBoth               = multilib32 | multilib64
)

// Add the multilib that is used in the arch type.
func (m multilibUsage) addArchType(archType android.ArchType) multilibUsage {
	multilib := archType.Multilib
	switch multilib {
	case "":
		return m
	case "lib32":
		return m | multilib32
	case "lib64":
		return m | multilib64
	default:
		panic(fmt.Errorf("Unknown Multilib field in ArchType, expected 'lib32' or 'lib64', found %q", multilib))
	}
}

func (m multilibUsage) String() string {
	switch m {
	case multilibNone:
		return ""
	case multilib32:
		return "32"
	case multilib64:
		return "64"
	case multilibBoth:
		return "both"
	default:
		panic(fmt.Errorf("Unknown multilib value, found %b, expected one of %b, %b, %b or %b",
			m, multilibNone, multilib32, multilib64, multilibBoth))
	}
}

type baseInfo struct {
	Properties android.SdkMemberProperties
}

type osTypeSpecificInfo struct {
	baseInfo

	osType android.OsType

	// The list of arch type specific info for this os type.
	//
	// Nil if there is one variant whose arch type is common
	archInfos []*archTypeSpecificInfo
}

type variantPropertiesFactoryFunc func() android.SdkMemberProperties

// Create a new osTypeSpecificInfo for the specified os type and its properties
// structures populated with information from the variants.
func newOsTypeSpecificInfo(ctx android.SdkMemberContext, osType android.OsType, variantPropertiesFactory variantPropertiesFactoryFunc, osTypeVariants []android.Module) *osTypeSpecificInfo {
	osInfo := &osTypeSpecificInfo{
		osType: osType,
	}

	osSpecificVariantPropertiesFactory := func() android.SdkMemberProperties {
		properties := variantPropertiesFactory()
		properties.Base().Os = osType
		return properties
	}

	// Create a structure into which properties common across the architectures in
	// this os type will be stored.
	osInfo.Properties = osSpecificVariantPropertiesFactory()

	// Group the variants by arch type.
	var variantsByArchName = make(map[string][]android.Module)
	var archTypes []android.ArchType
	for _, variant := range osTypeVariants {
		archType := variant.Target().Arch.ArchType
		archTypeName := archType.Name
		if _, ok := variantsByArchName[archTypeName]; !ok {
			archTypes = append(archTypes, archType)
		}

		variantsByArchName[archTypeName] = append(variantsByArchName[archTypeName], variant)
	}

	if commonVariants, ok := variantsByArchName["common"]; ok {
		if len(osTypeVariants) != 1 {
			panic("Expected to only have 1 variant when arch type is common but found " + string(len(osTypeVariants)))
		}

		// A common arch type only has one variant and its properties should be treated
		// as common to the os type.
		osInfo.Properties.PopulateFromVariant(ctx, commonVariants[0])
	} else {
		// Create an arch specific info for each supported architecture type.
		for _, archType := range archTypes {
			archTypeName := archType.Name

			archVariants := variantsByArchName[archTypeName]
			archInfo := newArchSpecificInfo(ctx, archType, osSpecificVariantPropertiesFactory, archVariants)

			osInfo.archInfos = append(osInfo.archInfos, archInfo)
		}
	}

	return osInfo
}

// Optimize the properties by extracting common properties from arch type specific
// properties into os type specific properties.
func (osInfo *osTypeSpecificInfo) optimizeProperties(commonValueExtractor *commonValueExtractor) {
	// Nothing to do if there is only a single common architecture.
	if len(osInfo.archInfos) == 0 {
		return
	}

	multilib := multilibNone
	var archPropertiesList []android.SdkMemberProperties
	for _, archInfo := range osInfo.archInfos {
		multilib = multilib.addArchType(archInfo.archType)

		// Optimize the arch properties first.
		archInfo.optimizeProperties(commonValueExtractor)

		archPropertiesList = append(archPropertiesList, archInfo.Properties)
	}

	commonValueExtractor.extractCommonProperties(osInfo.Properties, archPropertiesList)

	// Choose setting for compile_multilib that is appropriate for the arch variants supplied.
	osInfo.Properties.Base().Compile_multilib = multilib.String()
}

// Add the properties for an os to a property set.
//
// Maps the properties related to the os variants through to an appropriate
// module structure that will produce equivalent set of variants when it is
// processed in a build.
func (osInfo *osTypeSpecificInfo) addToPropertySet(ctx *memberContext, bpModule android.BpModule, targetPropertySet android.BpPropertySet) {

	var osPropertySet android.BpPropertySet
	var archPropertySet android.BpPropertySet
	var archOsPrefix string
	if osInfo.Properties.Base().Os_count == 1 {
		// There is only one os type present in the variants so don't bother
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
		archPropertySet = osPropertySet.AddPropertySet("arch")

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
		osType := osInfo.osType
		osPropertySet = targetPropertySet.AddPropertySet(osType.Name)
		archPropertySet = targetPropertySet

		// Arch specific properties need to be added to an os and arch specific
		// section prefixed with <os>_.
		archOsPrefix = osType.Name + "_"
	}

	// Add the os specific but arch independent properties to the module.
	osInfo.Properties.AddToPropertySet(ctx, osPropertySet)

	// Add arch (and possibly os) specific sections for each set of arch (and possibly
	// os) specific properties.
	//
	// The archInfos list will be empty if the os contains variants for the common
	// architecture.
	for _, archInfo := range osInfo.archInfos {
		archInfo.addToPropertySet(ctx, archPropertySet, archOsPrefix)
	}
}

type archTypeSpecificInfo struct {
	baseInfo

	archType android.ArchType

	linkInfos []*linkTypeSpecificInfo
}

// Create a new archTypeSpecificInfo for the specified arch type and its properties
// structures populated with information from the variants.
func newArchSpecificInfo(ctx android.SdkMemberContext, archType android.ArchType, variantPropertiesFactory variantPropertiesFactoryFunc, archVariants []android.Module) *archTypeSpecificInfo {

	// Create an arch specific info into which the variant properties can be copied.
	archInfo := &archTypeSpecificInfo{archType: archType}

	// Create the properties into which the arch type specific properties will be
	// added.
	archInfo.Properties = variantPropertiesFactory()

	if len(archVariants) == 1 {
		archInfo.Properties.PopulateFromVariant(ctx, archVariants[0])
	} else {
		// There is more than one variant for this arch type which must be differentiated
		// by link type.
		for _, linkVariant := range archVariants {
			linkType := getLinkType(linkVariant)
			if linkType == "" {
				panic(fmt.Errorf("expected one arch specific variant as it is not identified by link type but found %d", len(archVariants)))
			} else {
				linkInfo := newLinkSpecificInfo(ctx, linkType, variantPropertiesFactory, linkVariant)

				archInfo.linkInfos = append(archInfo.linkInfos, linkInfo)
			}
		}
	}

	return archInfo
}

// Get the link type of the variant
//
// If the variant is not differentiated by link type then it returns "",
// otherwise it returns one of "static" or "shared".
func getLinkType(variant android.Module) string {
	linkType := ""
	if linkable, ok := variant.(cc.LinkableInterface); ok {
		if linkable.Shared() && linkable.Static() {
			panic(fmt.Errorf("expected variant %q to be either static or shared but was both", variant.String()))
		} else if linkable.Shared() {
			linkType = "shared"
		} else if linkable.Static() {
			linkType = "static"
		} else {
			panic(fmt.Errorf("expected variant %q to be either static or shared but was neither", variant.String()))
		}
	}
	return linkType
}

// Optimize the properties by extracting common properties from link type specific
// properties into arch type specific properties.
func (archInfo *archTypeSpecificInfo) optimizeProperties(commonValueExtractor *commonValueExtractor) {
	if len(archInfo.linkInfos) == 0 {
		return
	}

	var propertiesList []android.SdkMemberProperties
	for _, linkInfo := range archInfo.linkInfos {
		propertiesList = append(propertiesList, linkInfo.Properties)
	}

	commonValueExtractor.extractCommonProperties(archInfo.Properties, propertiesList)
}

// Add the properties for an arch type to a property set.
func (archInfo *archTypeSpecificInfo) addToPropertySet(ctx *memberContext, archPropertySet android.BpPropertySet, archOsPrefix string) {
	archTypeName := archInfo.archType.Name
	archTypePropertySet := archPropertySet.AddPropertySet(archOsPrefix + archTypeName)
	archInfo.Properties.AddToPropertySet(ctx, archTypePropertySet)

	for _, linkInfo := range archInfo.linkInfos {
		linkPropertySet := archTypePropertySet.AddPropertySet(linkInfo.linkType)
		linkInfo.Properties.AddToPropertySet(ctx, linkPropertySet)
	}
}

type linkTypeSpecificInfo struct {
	baseInfo

	linkType string
}

// Create a new linkTypeSpecificInfo for the specified link type and its properties
// structures populated with information from the variant.
func newLinkSpecificInfo(ctx android.SdkMemberContext, linkType string, variantPropertiesFactory variantPropertiesFactoryFunc, linkVariant android.Module) *linkTypeSpecificInfo {
	linkInfo := &linkTypeSpecificInfo{
		baseInfo: baseInfo{
			// Create the properties into which the link type specific properties will be
			// added.
			Properties: variantPropertiesFactory(),
		},
		linkType: linkType,
	}
	linkInfo.Properties.PopulateFromVariant(ctx, linkVariant)
	return linkInfo
}

type memberContext struct {
	sdkMemberContext android.ModuleContext
	builder          *snapshotBuilder
	memberType       android.SdkMemberType
	name             string
}

func (m *memberContext) SdkModuleContext() android.ModuleContext {
	return m.sdkMemberContext
}

func (m *memberContext) SnapshotBuilder() android.SnapshotBuilder {
	return m.builder
}

func (m *memberContext) MemberType() android.SdkMemberType {
	return m.memberType
}

func (m *memberContext) Name() string {
	return m.name
}

func (s *sdk) createMemberSnapshot(ctx *memberContext, member *sdkMember, bpModule android.BpModule) {

	memberType := member.memberType

	// Group the variants by os type.
	variantsByOsType := make(map[android.OsType][]android.Module)
	variants := member.Variants()
	for _, variant := range variants {
		osType := variant.Target().Os
		variantsByOsType[osType] = append(variantsByOsType[osType], variant)
	}

	osCount := len(variantsByOsType)
	variantPropertiesFactory := func() android.SdkMemberProperties {
		properties := memberType.CreateVariantPropertiesStruct()
		base := properties.Base()
		base.Os_count = osCount
		return properties
	}

	osTypeToInfo := make(map[android.OsType]*osTypeSpecificInfo)

	// The set of properties that are common across all architectures and os types.
	commonProperties := variantPropertiesFactory()
	commonProperties.Base().Os = android.CommonOS

	// Create common value extractor that can be used to optimize the properties.
	commonValueExtractor := newCommonValueExtractor(commonProperties)

	// The list of property structures which are os type specific but common across
	// architectures within that os type.
	var osSpecificPropertiesList []android.SdkMemberProperties

	for osType, osTypeVariants := range variantsByOsType {
		osInfo := newOsTypeSpecificInfo(ctx, osType, variantPropertiesFactory, osTypeVariants)
		osTypeToInfo[osType] = osInfo
		// Add the os specific properties to a list of os type specific yet architecture
		// independent properties structs.
		osSpecificPropertiesList = append(osSpecificPropertiesList, osInfo.Properties)

		// Optimize the properties across all the variants for a specific os type.
		osInfo.optimizeProperties(commonValueExtractor)
	}

	// Extract properties which are common across all architectures and os types.
	commonValueExtractor.extractCommonProperties(commonProperties, osSpecificPropertiesList)

	// Add the common properties to the module.
	commonProperties.AddToPropertySet(ctx, bpModule)

	// Create a target property set into which target specific properties can be
	// added.
	targetPropertySet := bpModule.AddPropertySet("target")

	// Iterate over the os types in a fixed order.
	for _, osType := range s.getPossibleOsTypes() {
		osInfo := osTypeToInfo[osType]
		if osInfo == nil {
			continue
		}

		osInfo.addToPropertySet(ctx, bpModule, targetPropertySet)
	}
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

// Given a struct value, access a field within that struct (or one of its embedded
// structs).
type fieldAccessorFunc func(structValue reflect.Value) reflect.Value

// Supports extracting common values from a number of instances of a properties
// structure into a separate common set of properties.
type commonValueExtractor struct {
	// The getters for every field from which common values can be extracted.
	fieldGetters []fieldAccessorFunc
}

// Create a new common value extractor for the structure type for the supplied
// properties struct.
//
// The returned extractor can be used on any properties structure of the same type
// as the supplied set of properties.
func newCommonValueExtractor(propertiesStruct interface{}) *commonValueExtractor {
	structType := getStructValue(reflect.ValueOf(propertiesStruct)).Type()
	extractor := &commonValueExtractor{}
	extractor.gatherFields(structType, nil)
	return extractor
}

// Gather the fields from the supplied structure type from which common values will
// be extracted.
//
// This is recursive function. If it encounters an embedded field (no field name)
// that is a struct then it will recurse into that struct passing in the accessor
// for the field. That will then be used in the accessors for the fields in the
// embedded struct.
func (e *commonValueExtractor) gatherFields(structType reflect.Type, containingStructAccessor fieldAccessorFunc) {
	for f := 0; f < structType.NumField(); f++ {
		field := structType.Field(f)
		if field.PkgPath != "" {
			// Ignore unexported fields.
			continue
		}

		// Ignore fields whose value should be kept.
		if proptools.HasTag(field, "sdk", "keep") {
			continue
		}

		// Save a copy of the field index for use in the function.
		fieldIndex := f
		fieldGetter := func(value reflect.Value) reflect.Value {
			if containingStructAccessor != nil {
				// This is an embedded structure so first access the field for the embedded
				// structure.
				value = containingStructAccessor(value)
			}

			// Skip through interface and pointer values to find the structure.
			value = getStructValue(value)

			// Return the field.
			return value.Field(fieldIndex)
		}

		if field.Type.Kind() == reflect.Struct && field.Anonymous {
			// Gather fields from the embedded structure.
			e.gatherFields(field.Type, fieldGetter)
		} else {
			e.fieldGetters = append(e.fieldGetters, fieldGetter)
		}
	}
}

func getStructValue(value reflect.Value) reflect.Value {
foundStruct:
	for {
		kind := value.Kind()
		switch kind {
		case reflect.Interface, reflect.Ptr:
			value = value.Elem()
		case reflect.Struct:
			break foundStruct
		default:
			panic(fmt.Errorf("expecting struct, interface or pointer, found %v of kind %s", value, kind))
		}
	}
	return value
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
func (e *commonValueExtractor) extractCommonProperties(commonProperties interface{}, inputPropertiesSlice interface{}) {
	commonPropertiesValue := reflect.ValueOf(commonProperties)
	commonStructValue := commonPropertiesValue.Elem()

	for _, fieldGetter := range e.fieldGetters {
		// Check to see if all the structures have the same value for the field. The commonValue
		// is nil on entry to the loop and if it is nil on exit then there is no common value,
		// otherwise it points to the common value.
		var commonValue *reflect.Value
		sliceValue := reflect.ValueOf(inputPropertiesSlice)

		for i := 0; i < sliceValue.Len(); i++ {
			itemValue := sliceValue.Index(i)
			fieldValue := fieldGetter(itemValue)

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
			emptyValue := reflect.Zero(commonValue.Type())
			fieldGetter(commonStructValue).Set(*commonValue)
			for i := 0; i < sliceValue.Len(); i++ {
				itemValue := sliceValue.Index(i)
				fieldValue := fieldGetter(itemValue)
				fieldValue.Set(emptyValue)
			}
		}
	}
}
