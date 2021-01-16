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
			Command: `${config.SoongZipCmd} -C $basedir -r $out.rsp -o $out`,
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
	fmt.Fprintf(&(gc.content), strings.Repeat("    ", gc.indentLevel)+format+"\n", args...)
}

func (gf *generatedFile) build(pctx android.PackageContext, ctx android.BuilderContext, implicits android.Paths) {
	rb := android.NewRuleBuilder(pctx, ctx)

	content := gf.content.String()

	// ninja consumes newline characters in rspfile_content. Prevent it by
	// escaping the backslash in the newline character. The extra backslash
	// is removed when the rspfile is written to the actual script file
	content = strings.ReplaceAll(content, "\n", "\\n")

	rb.Command().
		Implicits(implicits).
		Text("echo").Text(proptools.ShellEscape(content)).
		// convert \\n to \n
		Text("| sed 's/\\\\n/\\n/g' >").Output(gf.path)
	rb.Command().
		Text("chmod a+x").Output(gf.path)
	rb.Build(gf.path.Base(), "Build "+gf.path.Base())
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
		s.createMemberSnapshot(memberCtx, member, prebuiltModule.(*bpModule))
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
	visibility := android.EffectiveVisibilityRules(ctx, s).Strings()
	if len(visibility) != 0 {
		snapshotModule.AddProperty("visibility", visibility)
	}

	addHostDeviceSupportedProperties(s.ModuleBase.DeviceSupported(), s.ModuleBase.HostSupported(), snapshotModule)

	var dynamicMemberPropertiesContainers []propertiesContainer
	osTypeToMemberProperties := make(map[android.OsType]*sdk)
	for _, sdkVariant := range sdkVariants {
		properties := sdkVariant.dynamicMemberTypeListProperties
		osTypeToMemberProperties[sdkVariant.Target().Os] = sdkVariant
		dynamicMemberPropertiesContainers = append(dynamicMemberPropertiesContainers, &dynamicMemberPropertiesContainer{sdkVariant, properties})
	}

	// Extract the common lists of members into a separate struct.
	commonDynamicMemberProperties := s.dynamicSdkMemberTypes.createMemberListProperties()
	extractor := newCommonValueExtractor(commonDynamicMemberProperties)
	extractCommonProperties(ctx, extractor, commonDynamicMemberProperties, dynamicMemberPropertiesContainers)

	// Add properties common to all os types.
	s.addMemberPropertiesToPropertySet(builder, snapshotModule, commonDynamicMemberProperties)

	// Optimize other per-variant properties, besides the dynamic member lists.
	type variantProperties struct {
		Compile_multilib string `android:"arch_variant"`
	}
	var variantPropertiesContainers []propertiesContainer
	variantToProperties := make(map[*sdk]*variantProperties)
	for _, sdkVariant := range sdkVariants {
		props := &variantProperties{
			Compile_multilib: sdkVariant.multilibUsages.String(),
		}
		variantPropertiesContainers = append(variantPropertiesContainers, &dynamicMemberPropertiesContainer{sdkVariant, props})
		variantToProperties[sdkVariant] = props
	}
	commonVariantProperties := variantProperties{}
	extractor = newCommonValueExtractor(commonVariantProperties)
	extractCommonProperties(ctx, extractor, &commonVariantProperties, variantPropertiesContainers)
	if commonVariantProperties.Compile_multilib != "" && commonVariantProperties.Compile_multilib != "both" {
		// Compile_multilib defaults to both so only needs to be set when it's
		// specified and not both.
		snapshotModule.AddProperty("compile_multilib", commonVariantProperties.Compile_multilib)
	}

	targetPropertySet := snapshotModule.AddPropertySet("target")

	// Iterate over the os types in a fixed order.
	for _, osType := range s.getPossibleOsTypes() {
		if sdkVariant, ok := osTypeToMemberProperties[osType]; ok {
			osPropertySet := targetPropertySet.AddPropertySet(sdkVariant.Target().Os.Name)

			variantProps := variantToProperties[sdkVariant]
			if variantProps.Compile_multilib != "" && variantProps.Compile_multilib != "both" {
				osPropertySet.AddProperty("compile_multilib", variantProps.Compile_multilib)
			}

			s.addMemberPropertiesToPropertySet(builder, osPropertySet, sdkVariant.dynamicMemberTypeListProperties)
		}
	}

	// If host is supported and any member is host OS dependent then disable host
	// by default, so that we can enable each host OS variant explicitly. This
	// avoids problems with implicitly enabled OS variants when the snapshot is
	// used, which might be different from this run (e.g. different build OS).
	if s.HostSupported() {
		var supportedHostTargets []string
		for _, memberRef := range memberRefs {
			if memberRef.memberType.IsHostOsDependent() && memberRef.variant.Target().Os.Class == android.Host {
				targetString := memberRef.variant.Target().Os.String() + "_" + memberRef.variant.Target().Arch.ArchType.String()
				if !android.InList(targetString, supportedHostTargets) {
					supportedHostTargets = append(supportedHostTargets, targetString)
				}
			}
		}
		if len(supportedHostTargets) > 0 {
			hostPropertySet := targetPropertySet.AddPropertySet("host")
			hostPropertySet.AddProperty("enabled", false)
		}
		// Enable the <os>_<arch> variant explicitly when we've disabled it by default on host.
		for _, hostTarget := range supportedHostTargets {
			propertySet := targetPropertySet.AddPropertySet(hostTarget)
			propertySet.AddProperty("enabled", true)
		}
	}

	// Prune any empty property sets.
	snapshotModule.transform(pruneEmptySetTransformer{})

	bpFile.AddModule(snapshotModule)

	// generate Android.bp
	bp = newGeneratedFile(ctx, "snapshot", "Android.bp")
	generateBpContents(&bp.generatedContents, bpFile)

	contents := bp.content.String()
	syntaxCheckSnapshotBpFile(ctx, contents)

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

// Check the syntax of the generated Android.bp file contents and if they are
// invalid then log an error with the contents (tagged with line numbers) and the
// errors that were found so that it is easy to see where the problem lies.
func syntaxCheckSnapshotBpFile(ctx android.ModuleContext, contents string) {
	errs := android.CheckBlueprintSyntax(ctx, "Android.bp", contents)
	if len(errs) != 0 {
		message := &strings.Builder{}
		_, _ = fmt.Fprint(message, `errors in generated Android.bp snapshot:

Generated Android.bp contents
========================================================================
`)
		for i, line := range strings.Split(contents, "\n") {
			_, _ = fmt.Fprintf(message, "%6d:    %s\n", i+1, line)
		}

		_, _ = fmt.Fprint(message, `
========================================================================

Errors found:
`)

		for _, err := range errs {
			_, _ = fmt.Fprintf(message, "%s\n", err.Error())
		}

		ctx.ModuleErrorf("%s", message.String())
	}
}

func extractCommonProperties(ctx android.ModuleContext, extractor *commonValueExtractor, commonProperties interface{}, inputPropertiesSlice interface{}) {
	err := extractor.extractCommonProperties(commonProperties, inputPropertiesSlice)
	if err != nil {
		ctx.ModuleErrorf("error extracting common properties: %s", err)
	}
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

	// The path to an empty file.
	emptyFile android.WritablePath

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

func (s *snapshotBuilder) EmptyFile() android.Path {
	if s.emptyFile == nil {
		ctx := s.ctx
		s.emptyFile = android.PathForModuleOut(ctx, "empty")
		s.ctx.Build(pctx, android.BuildParams{
			Rule:   android.Touch,
			Output: s.emptyFile,
		})
	}

	return s.emptyFile
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
		visibilityRules := android.EffectiveVisibilityRules(s.ctx, variant)

		// Add any additional visibility rules needed for the prebuilts to reference each other.
		err := visibilityRules.Widen(s.sdk.properties.Prebuilt_visibility)
		if err != nil {
			s.ctx.PropertyErrorf("prebuilt_visibility", "%s", err)
		}

		visibility := visibilityRules.Strings()
		if len(visibility) != 0 {
			m.AddProperty("visibility", visibility)
		}
	}

	// Where available copy apex_available properties from the member.
	if apexAware, ok := variant.(interface{ ApexAvailable() []string }); ok {
		apexAvailable := apexAware.ApexAvailable()
		if len(apexAvailable) == 0 {
			// //apex_available:platform is the default.
			apexAvailable = []string{android.AvailableToPlatform}
		}

		// Add in any baseline apex available settings.
		apexAvailable = append(apexAvailable, apex.BaselineApexAvailable(member.Name())...)

		// Remove duplicates and sort.
		apexAvailable = android.FirstUniqueStrings(apexAvailable)
		sort.Strings(apexAvailable)

		m.AddProperty("apex_available", apexAvailable)
	}

	deviceSupported := false
	hostSupported := false

	for _, variant := range member.Variants() {
		osClass := variant.Target().Os.Class
		if osClass == android.Host {
			hostSupported = true
		} else if osClass == android.Device {
			deviceSupported = true
		}
	}

	addHostDeviceSupportedProperties(deviceSupported, hostSupported, m)

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

// Add the properties from the given SdkMemberProperties to the blueprint
// property set. This handles common properties in SdkMemberPropertiesBase and
// calls the member-specific AddToPropertySet for the rest.
func addSdkMemberPropertiesToSet(ctx *memberContext, memberProperties android.SdkMemberProperties, targetPropertySet android.BpPropertySet) {
	if memberProperties.Base().Compile_multilib != "" {
		targetPropertySet.AddProperty("compile_multilib", memberProperties.Base().Compile_multilib)
	}

	memberProperties.AddToPropertySet(ctx, targetPropertySet)
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

func (b *baseInfo) optimizableProperties() interface{} {
	return b.Properties
}

type osTypeSpecificInfo struct {
	baseInfo

	osType android.OsType

	// The list of arch type specific info for this os type.
	//
	// Nil if there is one variant whose arch type is common
	archInfos []*archTypeSpecificInfo
}

var _ propertiesContainer = (*osTypeSpecificInfo)(nil)

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
			panic(fmt.Errorf("Expected to only have 1 variant when arch type is common but found %d", len(osTypeVariants)))
		}

		// A common arch type only has one variant and its properties should be treated
		// as common to the os type.
		osInfo.Properties.PopulateFromVariant(ctx, commonVariants[0])
	} else {
		// Create an arch specific info for each supported architecture type.
		for _, archType := range archTypes {
			archTypeName := archType.Name

			archVariants := variantsByArchName[archTypeName]
			archInfo := newArchSpecificInfo(ctx, archType, osType, osSpecificVariantPropertiesFactory, archVariants)

			osInfo.archInfos = append(osInfo.archInfos, archInfo)
		}
	}

	return osInfo
}

// Optimize the properties by extracting common properties from arch type specific
// properties into os type specific properties.
func (osInfo *osTypeSpecificInfo) optimizeProperties(ctx *memberContext, commonValueExtractor *commonValueExtractor) {
	// Nothing to do if there is only a single common architecture.
	if len(osInfo.archInfos) == 0 {
		return
	}

	multilib := multilibNone
	for _, archInfo := range osInfo.archInfos {
		multilib = multilib.addArchType(archInfo.archType)

		// Optimize the arch properties first.
		archInfo.optimizeProperties(ctx, commonValueExtractor)
	}

	extractCommonProperties(ctx.sdkMemberContext, commonValueExtractor, osInfo.Properties, osInfo.archInfos)

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
	if osInfo.Properties.Base().Os_count == 1 &&
		(osInfo.osType.Class == android.Device || !ctx.memberType.IsHostOsDependent()) {
		// There is only one OS type present in the variants and it shouldn't have a
		// variant-specific target. The latter is the case if it's either for device
		// where there is only one OS (android), or for host and the member type
		// isn't host OS dependent.

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
	addSdkMemberPropertiesToSet(ctx, osInfo.Properties, osPropertySet)

	// Add arch (and possibly os) specific sections for each set of arch (and possibly
	// os) specific properties.
	//
	// The archInfos list will be empty if the os contains variants for the common
	// architecture.
	for _, archInfo := range osInfo.archInfos {
		archInfo.addToPropertySet(ctx, archPropertySet, archOsPrefix)
	}
}

func (osInfo *osTypeSpecificInfo) isHostVariant() bool {
	osClass := osInfo.osType.Class
	return osClass == android.Host
}

var _ isHostVariant = (*osTypeSpecificInfo)(nil)

func (osInfo *osTypeSpecificInfo) String() string {
	return fmt.Sprintf("OsType{%s}", osInfo.osType)
}

type archTypeSpecificInfo struct {
	baseInfo

	archType android.ArchType
	osType   android.OsType

	linkInfos []*linkTypeSpecificInfo
}

var _ propertiesContainer = (*archTypeSpecificInfo)(nil)

// Create a new archTypeSpecificInfo for the specified arch type and its properties
// structures populated with information from the variants.
func newArchSpecificInfo(ctx android.SdkMemberContext, archType android.ArchType, osType android.OsType, variantPropertiesFactory variantPropertiesFactoryFunc, archVariants []android.Module) *archTypeSpecificInfo {

	// Create an arch specific info into which the variant properties can be copied.
	archInfo := &archTypeSpecificInfo{archType: archType, osType: osType}

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

func (archInfo *archTypeSpecificInfo) optimizableProperties() interface{} {
	return archInfo.Properties
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
func (archInfo *archTypeSpecificInfo) optimizeProperties(ctx *memberContext, commonValueExtractor *commonValueExtractor) {
	if len(archInfo.linkInfos) == 0 {
		return
	}

	extractCommonProperties(ctx.sdkMemberContext, commonValueExtractor, archInfo.Properties, archInfo.linkInfos)
}

// Add the properties for an arch type to a property set.
func (archInfo *archTypeSpecificInfo) addToPropertySet(ctx *memberContext, archPropertySet android.BpPropertySet, archOsPrefix string) {
	archTypeName := archInfo.archType.Name
	archTypePropertySet := archPropertySet.AddPropertySet(archOsPrefix + archTypeName)
	// Enable the <os>_<arch> variant explicitly when we've disabled it by default on host.
	if ctx.memberType.IsHostOsDependent() && archInfo.osType.Class == android.Host {
		archTypePropertySet.AddProperty("enabled", true)
	}
	addSdkMemberPropertiesToSet(ctx, archInfo.Properties, archTypePropertySet)

	for _, linkInfo := range archInfo.linkInfos {
		linkPropertySet := archTypePropertySet.AddPropertySet(linkInfo.linkType)
		addSdkMemberPropertiesToSet(ctx, linkInfo.Properties, linkPropertySet)
	}
}

func (archInfo *archTypeSpecificInfo) String() string {
	return fmt.Sprintf("ArchType{%s}", archInfo.archType)
}

type linkTypeSpecificInfo struct {
	baseInfo

	linkType string
}

var _ propertiesContainer = (*linkTypeSpecificInfo)(nil)

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

func (l *linkTypeSpecificInfo) String() string {
	return fmt.Sprintf("LinkType{%s}", l.linkType)
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

func (s *sdk) createMemberSnapshot(ctx *memberContext, member *sdkMember, bpModule *bpModule) {

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
	var osSpecificPropertiesContainers []*osTypeSpecificInfo

	for osType, osTypeVariants := range variantsByOsType {
		osInfo := newOsTypeSpecificInfo(ctx, osType, variantPropertiesFactory, osTypeVariants)
		osTypeToInfo[osType] = osInfo
		// Add the os specific properties to a list of os type specific yet architecture
		// independent properties structs.
		osSpecificPropertiesContainers = append(osSpecificPropertiesContainers, osInfo)

		// Optimize the properties across all the variants for a specific os type.
		osInfo.optimizeProperties(ctx, commonValueExtractor)
	}

	// Extract properties which are common across all architectures and os types.
	extractCommonProperties(ctx.sdkMemberContext, commonValueExtractor, commonProperties, osSpecificPropertiesContainers)

	// Add the common properties to the module.
	addSdkMemberPropertiesToSet(ctx, commonProperties, bpModule)

	// Create a target property set into which target specific properties can be
	// added.
	targetPropertySet := bpModule.AddPropertySet("target")

	// If the member is host OS dependent and has host_supported then disable by
	// default and enable each host OS variant explicitly. This avoids problems
	// with implicitly enabled OS variants when the snapshot is used, which might
	// be different from this run (e.g. different build OS).
	if ctx.memberType.IsHostOsDependent() {
		hostSupported := bpModule.getValue("host_supported") == true // Missing means false.
		if hostSupported {
			hostPropertySet := targetPropertySet.AddPropertySet("host")
			hostPropertySet.AddProperty("enabled", false)
		}
	}

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
			if osType.Class == android.Host {
				osTypes = append(osTypes, osType)
			}
		}
	}
	sort.SliceStable(osTypes, func(i, j int) bool { return osTypes[i].Name < osTypes[j].Name })
	return osTypes
}

// Given a set of properties (struct value), return the value of the field within that
// struct (or one of its embedded structs).
type fieldAccessorFunc func(structValue reflect.Value) reflect.Value

// Checks the metadata to determine whether the property should be ignored for the
// purposes of common value extraction or not.
type extractorMetadataPredicate func(metadata propertiesContainer) bool

// Indicates whether optimizable properties are provided by a host variant or
// not.
type isHostVariant interface {
	isHostVariant() bool
}

// A property that can be optimized by the commonValueExtractor.
type extractorProperty struct {
	// The name of the field for this property. It is a "."-separated path for
	// fields in non-anonymous substructs.
	name string

	// Filter that can use metadata associated with the properties being optimized
	// to determine whether the field should be ignored during common value
	// optimization.
	filter extractorMetadataPredicate

	// Retrieves the value on which common value optimization will be performed.
	getter fieldAccessorFunc

	// The empty value for the field.
	emptyValue reflect.Value

	// True if the property can support arch variants false otherwise.
	archVariant bool
}

func (p extractorProperty) String() string {
	return p.name
}

// Supports extracting common values from a number of instances of a properties
// structure into a separate common set of properties.
type commonValueExtractor struct {
	// The properties that the extractor can optimize.
	properties []extractorProperty
}

// Create a new common value extractor for the structure type for the supplied
// properties struct.
//
// The returned extractor can be used on any properties structure of the same type
// as the supplied set of properties.
func newCommonValueExtractor(propertiesStruct interface{}) *commonValueExtractor {
	structType := getStructValue(reflect.ValueOf(propertiesStruct)).Type()
	extractor := &commonValueExtractor{}
	extractor.gatherFields(structType, nil, "")
	return extractor
}

// Gather the fields from the supplied structure type from which common values will
// be extracted.
//
// This is recursive function. If it encounters a struct then it will recurse
// into it, passing in the accessor for the field and the struct name as prefix
// for the nested fields. That will then be used in the accessors for the fields
// in the embedded struct.
func (e *commonValueExtractor) gatherFields(structType reflect.Type, containingStructAccessor fieldAccessorFunc, namePrefix string) {
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

		var filter extractorMetadataPredicate

		// Add a filter
		if proptools.HasTag(field, "sdk", "ignored-on-host") {
			filter = func(metadata propertiesContainer) bool {
				if m, ok := metadata.(isHostVariant); ok {
					if m.isHostVariant() {
						return false
					}
				}
				return true
			}
		}

		// Save a copy of the field index for use in the function.
		fieldIndex := f

		name := namePrefix + field.Name

		fieldGetter := func(value reflect.Value) reflect.Value {
			if containingStructAccessor != nil {
				// This is an embedded structure so first access the field for the embedded
				// structure.
				value = containingStructAccessor(value)
			}

			// Skip through interface and pointer values to find the structure.
			value = getStructValue(value)

			defer func() {
				if r := recover(); r != nil {
					panic(fmt.Errorf("%s for fieldIndex %d of field %s of value %#v", r, fieldIndex, name, value.Interface()))
				}
			}()

			// Return the field.
			return value.Field(fieldIndex)
		}

		if field.Type.Kind() == reflect.Struct {
			// Gather fields from the nested or embedded structure.
			var subNamePrefix string
			if field.Anonymous {
				subNamePrefix = namePrefix
			} else {
				subNamePrefix = name + "."
			}
			e.gatherFields(field.Type, fieldGetter, subNamePrefix)
		} else {
			property := extractorProperty{
				name,
				filter,
				fieldGetter,
				reflect.Zero(field.Type),
				proptools.HasTag(field, "android", "arch_variant"),
			}
			e.properties = append(e.properties, property)
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

// A container of properties to be optimized.
//
// Allows additional information to be associated with the properties, e.g. for
// filtering.
type propertiesContainer interface {
	fmt.Stringer

	// Get the properties that need optimizing.
	optimizableProperties() interface{}
}

// A wrapper for dynamic member properties to allow them to be optimized.
type dynamicMemberPropertiesContainer struct {
	sdkVariant              *sdk
	dynamicMemberProperties interface{}
}

func (c dynamicMemberPropertiesContainer) optimizableProperties() interface{} {
	return c.dynamicMemberProperties
}

func (c dynamicMemberPropertiesContainer) String() string {
	return c.sdkVariant.String()
}

// Extract common properties from a slice of property structures of the same type.
//
// All the property structures must be of the same type.
// commonProperties - must be a pointer to the structure into which common properties will be added.
// inputPropertiesSlice - must be a slice of propertiesContainer interfaces.
//
// Iterates over each exported field (capitalized name) and checks to see whether they
// have the same value (using DeepEquals) across all the input properties. If it does not then no
// change is made. Otherwise, the common value is stored in the field in the commonProperties
// and the field in each of the input properties structure is set to its default value. Nested
// structs are visited recursively and their non-struct fields are compared.
func (e *commonValueExtractor) extractCommonProperties(commonProperties interface{}, inputPropertiesSlice interface{}) error {
	commonPropertiesValue := reflect.ValueOf(commonProperties)
	commonStructValue := commonPropertiesValue.Elem()

	sliceValue := reflect.ValueOf(inputPropertiesSlice)

	for _, property := range e.properties {
		fieldGetter := property.getter
		filter := property.filter
		if filter == nil {
			filter = func(metadata propertiesContainer) bool {
				return true
			}
		}

		// Check to see if all the structures have the same value for the field. The commonValue
		// is nil on entry to the loop and if it is nil on exit then there is no common value or
		// all the values have been filtered out, otherwise it points to the common value.
		var commonValue *reflect.Value

		// Assume that all the values will be the same.
		//
		// While similar to this is not quite the same as commonValue == nil. If all the values
		// have been filtered out then this will be false but commonValue == nil will be true.
		valuesDiffer := false

		for i := 0; i < sliceValue.Len(); i++ {
			container := sliceValue.Index(i).Interface().(propertiesContainer)
			itemValue := reflect.ValueOf(container.optimizableProperties())
			fieldValue := fieldGetter(itemValue)

			if !filter(container) {
				expectedValue := property.emptyValue.Interface()
				actualValue := fieldValue.Interface()
				if !reflect.DeepEqual(expectedValue, actualValue) {
					return fmt.Errorf("field %q is supposed to be ignored for %q but is set to %#v instead of %#v", property, container, actualValue, expectedValue)
				}
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
					valuesDiffer = true
					break
				}
			}
		}

		// If the fields all have common value then store it in the common struct field
		// and set the input struct's field to the empty value.
		if commonValue != nil {
			emptyValue := property.emptyValue
			fieldGetter(commonStructValue).Set(*commonValue)
			for i := 0; i < sliceValue.Len(); i++ {
				container := sliceValue.Index(i).Interface().(propertiesContainer)
				itemValue := reflect.ValueOf(container.optimizableProperties())
				fieldValue := fieldGetter(itemValue)
				fieldValue.Set(emptyValue)
			}
		}

		if valuesDiffer && !property.archVariant {
			// The values differ but the property does not support arch variants so it
			// is an error.
			var details strings.Builder
			for i := 0; i < sliceValue.Len(); i++ {
				container := sliceValue.Index(i).Interface().(propertiesContainer)
				itemValue := reflect.ValueOf(container.optimizableProperties())
				fieldValue := fieldGetter(itemValue)

				_, _ = fmt.Fprintf(&details, "\n    %q has value %q", container.String(), fieldValue.Interface())
			}

			return fmt.Errorf("field %q is not tagged as \"arch_variant\" but has arch specific properties:%s", property.String(), details.String())
		}
	}

	return nil
}
