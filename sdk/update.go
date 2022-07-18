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

// Environment variables that affect the generated snapshot
// ========================================================
//
// SOONG_SDK_SNAPSHOT_PREFER
//     By default every unversioned module in the generated snapshot has prefer: false. Building it
//     with SOONG_SDK_SNAPSHOT_PREFER=true will force them to use prefer: true.
//
// SOONG_SDK_SNAPSHOT_USE_SOURCE_CONFIG_VAR
//     If set this specifies the Soong config var that can be used to control whether the prebuilt
//     modules from the generated snapshot or the original source modules. Values must be a colon
//     separated pair of strings, the first of which is the Soong config namespace, and the second
//     is the name of the variable within that namespace.
//
//     The config namespace and var name are used to set the `use_source_config_var` property. That
//     in turn will cause the generated prebuilts to use the soong config variable to select whether
//     source or the prebuilt is used.
//     e.g. If an sdk snapshot is built using:
//       m SOONG_SDK_SNAPSHOT_USE_SOURCE_CONFIG_VAR=acme:build_from_source sdkextensions-sdk
//     Then the resulting snapshot will include:
//       use_source_config_var: {
//         config_namespace: "acme",
//         var_name: "build_from_source",
//       }
//
//     Assuming that the config variable is defined in .mk using something like:
//       $(call add_soong_config_namespace,acme)
//       $(call add_soong_config_var_value,acme,build_from_source,true)
//
//     Then when the snapshot is unpacked in the repository it will have the following behavior:
//       m droid - will use the sdkextensions-sdk prebuilts if present. Otherwise, it will use the
//           sources.
//       m SOONG_CONFIG_acme_build_from_source=true droid - will use the sdkextensions-sdk
//            sources, if present. Otherwise, it will use the prebuilts.
//
//     This is a temporary mechanism to control the prefer flags and will be removed once a more
//     maintainable solution has been implemented.
//     TODO(b/174997203): Remove when no longer necessary.
//
// SOONG_SDK_SNAPSHOT_VERSION
//     This provides control over the version of the generated snapshot.
//
//     SOONG_SDK_SNAPSHOT_VERSION=current will generate unversioned and versioned prebuilts and a
//     versioned snapshot module. This is the default behavior. The zip file containing the
//     generated snapshot will be <sdk-name>-current.zip.
//
//     SOONG_SDK_SNAPSHOT_VERSION=unversioned will generate unversioned prebuilts only and the zip
//     file containing the generated snapshot will be <sdk-name>.zip.
//
//     SOONG_SDK_SNAPSHOT_VERSION=<number> will generate versioned prebuilts and a versioned
//     snapshot module only. The zip file containing the generated snapshot will be
//     <sdk-name>-<number>.zip.
//
// SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE
//     This allows the target build release (i.e. the release version of the build within which
//     the snapshot will be used) of the snapshot to be specified. If unspecified then it defaults
//     to the current build release version. Otherwise, it must be the name of one of the build
//     releases defined in nameToBuildRelease, e.g. S, T, etc..
//
//     The generated snapshot must only be used in the specified target release. If the target
//     build release is not the current build release then the generated Android.bp file not be
//     checked for compatibility.
//
//     e.g. if setting SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE=S will cause the generated snapshot
//     to be compatible with S.
//

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

const (
	soongSdkSnapshotVersionUnversioned = "unversioned"
	soongSdkSnapshotVersionCurrent     = "current"
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

// IndentedPrintf will add spaces to indent the line to the appropriate level before printing the
// arguments.
func (gc *generatedContents) IndentedPrintf(format string, args ...interface{}) {
	fmt.Fprintf(&(gc.content), strings.Repeat("    ", gc.indentLevel)+format, args...)
}

// UnindentedPrintf does not add spaces to indent the line to the appropriate level before printing
// the arguments.
func (gc *generatedContents) UnindentedPrintf(format string, args ...interface{}) {
	fmt.Fprintf(&(gc.content), format, args...)
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
		Text("echo -n").Text(proptools.ShellEscape(content)).
		// convert \\n to \n
		Text("| sed 's/\\\\n/\\n/g' >").Output(gf.path)
	rb.Command().
		Text("chmod a+x").Output(gf.path)
	rb.Build(gf.path.Base(), "Build "+gf.path.Base())
}

// Collect all the members.
//
// Updates the sdk module with a list of sdkMemberVariantDep instances and details as to which
// multilibs (32/64/both) are used by this sdk variant.
func (s *sdk) collectMembers(ctx android.ModuleContext) {
	s.multilibUsages = multilibNone
	ctx.WalkDeps(func(child android.Module, parent android.Module) bool {
		tag := ctx.OtherModuleDependencyTag(child)
		if memberTag, ok := tag.(android.SdkMemberDependencyTag); ok {
			memberType := memberTag.SdkMemberType(child)

			// If a nil SdkMemberType was returned then this module should not be added to the sdk.
			if memberType == nil {
				return false
			}

			// Make sure that the resolved module is allowed in the member list property.
			if !memberType.IsInstance(child) {
				ctx.ModuleErrorf("module %q is not valid in property %s", ctx.OtherModuleName(child), memberType.SdkPropertyName())
			}

			// Keep track of which multilib variants are used by the sdk.
			s.multilibUsages = s.multilibUsages.addArchType(child.Target().Arch.ArchType)

			var exportedComponentsInfo android.ExportedComponentsInfo
			if ctx.OtherModuleHasProvider(child, android.ExportedComponentsInfoProvider) {
				exportedComponentsInfo = ctx.OtherModuleProvider(child, android.ExportedComponentsInfoProvider).(android.ExportedComponentsInfo)
			}

			export := memberTag.ExportMember()
			s.memberVariantDeps = append(s.memberVariantDeps, sdkMemberVariantDep{
				s, memberType, child.(android.SdkAware), export, exportedComponentsInfo,
			})

			// Recurse down into the member's dependencies as it may have dependencies that need to be
			// automatically added to the sdk.
			return true
		}

		return false
	})
}

// groupMemberVariantsByMemberThenType groups the member variant dependencies so that all the
// variants of each member are grouped together within an sdkMember instance.
//
// The sdkMember instances are then grouped into slices by member type. Within each such slice the
// sdkMember instances appear in the order they were added as dependencies.
//
// Finally, the member type slices are concatenated together to form a single slice. The order in
// which they are concatenated is the order in which the member types were registered in the
// android.SdkMemberTypesRegistry.
func (s *sdk) groupMemberVariantsByMemberThenType(ctx android.ModuleContext, targetBuildRelease *buildRelease, memberVariantDeps []sdkMemberVariantDep) []*sdkMember {
	byType := make(map[android.SdkMemberType][]*sdkMember)
	byName := make(map[string]*sdkMember)

	for _, memberVariantDep := range memberVariantDeps {
		memberType := memberVariantDep.memberType
		variant := memberVariantDep.variant

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
	for _, memberListProperty := range s.memberTypeListProperties() {
		memberType := memberListProperty.memberType

		if !isMemberTypeSupportedByTargetBuildRelease(memberType, targetBuildRelease) {
			continue
		}

		membersOfType := byType[memberType]
		members = append(members, membersOfType...)
	}

	return members
}

// isMemberTypeSupportedByTargetBuildRelease returns true if the member type is supported by the
// target build release.
func isMemberTypeSupportedByTargetBuildRelease(memberType android.SdkMemberType, targetBuildRelease *buildRelease) bool {
	supportedByTargetBuildRelease := true
	supportedBuildReleases := memberType.SupportedBuildReleases()
	if supportedBuildReleases == "" {
		supportedBuildReleases = "S+"
	}

	set, err := parseBuildReleaseSet(supportedBuildReleases)
	if err != nil {
		panic(fmt.Errorf("member type %s has invalid supported build releases %q: %s",
			memberType.SdkPropertyName(), supportedBuildReleases, err))
	}
	if !set.contains(targetBuildRelease) {
		supportedByTargetBuildRelease = false
	}
	return supportedByTargetBuildRelease
}

func appendUniqueVariants(variants []android.SdkAware, newVariant android.SdkAware) []android.SdkAware {
	for _, v := range variants {
		if v == newVariant {
			return variants
		}
	}
	return append(variants, newVariant)
}

// BUILD_NUMBER_FILE is the name of the file in the snapshot zip that will contain the number of
// the build from which the snapshot was produced.
const BUILD_NUMBER_FILE = "snapshot-creation-build-number.txt"

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

	// Aggregate all the sdkMemberVariantDep instances from all the sdk variants.
	hasLicenses := false
	var memberVariantDeps []sdkMemberVariantDep
	for _, sdkVariant := range sdkVariants {
		memberVariantDeps = append(memberVariantDeps, sdkVariant.memberVariantDeps...)
	}

	// Filter out any sdkMemberVariantDep that is a component of another.
	memberVariantDeps = filterOutComponents(ctx, memberVariantDeps)

	// Record the names of all the members, both explicitly specified and implicitly
	// included.
	allMembersByName := make(map[string]struct{})
	exportedMembersByName := make(map[string]struct{})

	addMember := func(name string, export bool) {
		allMembersByName[name] = struct{}{}
		if export {
			exportedMembersByName[name] = struct{}{}
		}
	}

	for _, memberVariantDep := range memberVariantDeps {
		name := memberVariantDep.variant.Name()
		export := memberVariantDep.export

		addMember(name, export)

		// Add any components provided by the module.
		for _, component := range memberVariantDep.exportedComponentsInfo.Components {
			addMember(component, export)
		}

		if memberVariantDep.memberType == android.LicenseModuleSdkMemberType {
			hasLicenses = true
		}
	}

	snapshotDir := android.PathForModuleOut(ctx, "snapshot")

	bp := newGeneratedFile(ctx, "snapshot", "Android.bp")

	bpFile := &bpFile{
		modules: make(map[string]*bpModule),
	}

	config := ctx.Config()
	version := config.GetenvWithDefault("SOONG_SDK_SNAPSHOT_VERSION", "current")

	// Generate versioned modules in the snapshot unless an unversioned snapshot has been requested.
	generateVersioned := version != soongSdkSnapshotVersionUnversioned

	// Generate unversioned modules in the snapshot unless a numbered snapshot has been requested.
	//
	// Unversioned modules are not required in that case because the numbered version will be a
	// finalized version of the snapshot that is intended to be kept separate from the
	generateUnversioned := version == soongSdkSnapshotVersionUnversioned || version == soongSdkSnapshotVersionCurrent
	snapshotZipFileSuffix := ""
	if generateVersioned {
		snapshotZipFileSuffix = "-" + version
	}

	currentBuildRelease := latestBuildRelease()
	targetBuildReleaseEnv := config.GetenvWithDefault("SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE", currentBuildRelease.name)
	targetBuildRelease, err := nameToRelease(targetBuildReleaseEnv)
	if err != nil {
		ctx.ModuleErrorf("invalid SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE: %s", err)
		targetBuildRelease = currentBuildRelease
	}

	builder := &snapshotBuilder{
		ctx:                   ctx,
		sdk:                   s,
		version:               version,
		snapshotDir:           snapshotDir.OutputPath,
		copies:                make(map[string]string),
		filesToZip:            []android.Path{bp.path},
		bpFile:                bpFile,
		prebuiltModules:       make(map[string]*bpModule),
		allMembersByName:      allMembersByName,
		exportedMembersByName: exportedMembersByName,
		targetBuildRelease:    targetBuildRelease,
	}
	s.builderForTests = builder

	// If the sdk snapshot includes any license modules then add a package module which has a
	// default_applicable_licenses property. That will prevent the LSC license process from updating
	// the generated Android.bp file to add a package module that includes all licenses used by all
	// the modules in that package. That would be unnecessary as every module in the sdk should have
	// their own licenses property specified.
	if hasLicenses {
		pkg := bpFile.newModule("package")
		property := "default_applicable_licenses"
		pkg.AddCommentForProperty(property, `
A default list here prevents the license LSC from adding its own list which would
be unnecessary as every module in the sdk already has its own licenses property.
`)
		pkg.AddProperty(property, []string{"Android-Apache-2.0"})
		bpFile.AddModule(pkg)
	}

	// Group the variants for each member module together and then group the members of each member
	// type together.
	members := s.groupMemberVariantsByMemberThenType(ctx, targetBuildRelease, memberVariantDeps)

	// Create the prebuilt modules for each of the member modules.
	traits := s.gatherTraits()
	for _, member := range members {
		memberType := member.memberType

		name := member.name
		requiredTraits := traits[name]
		if requiredTraits == nil {
			requiredTraits = android.EmptySdkMemberTraitSet()
		}

		// Create the snapshot for the member.
		memberCtx := &memberContext{ctx, builder, memberType, name, requiredTraits}

		prebuiltModule := memberType.AddPrebuiltModule(memberCtx, member)
		s.createMemberSnapshot(memberCtx, member, prebuiltModule.(*bpModule))
	}

	// Create a transformer that will transform an unversioned module into a versioned module.
	unversionedToVersionedTransformer := unversionedToVersionedTransformation{builder: builder}

	// Create a transformer that will transform an unversioned module by replacing any references
	// to internal members with a unique module name and setting prefer: false.
	unversionedTransformer := unversionedTransformation{
		builder: builder,
	}

	for _, unversioned := range builder.prebuiltOrder {
		// Prune any empty property sets.
		unversioned = unversioned.transform(pruneEmptySetTransformer{})

		if generateVersioned {
			// Copy the unversioned module so it can be modified to make it versioned.
			versioned := unversioned.deepCopy()

			// Transform the unversioned module into a versioned one.
			versioned.transform(unversionedToVersionedTransformer)
			bpFile.AddModule(versioned)
		}

		if generateUnversioned {
			// Transform the unversioned module to make it suitable for use in the snapshot.
			unversioned.transform(unversionedTransformer)
			bpFile.AddModule(unversioned)
		}
	}

	if generateVersioned {
		// Add the sdk/module_exports_snapshot module to the bp file.
		s.addSnapshotModule(ctx, builder, sdkVariants, memberVariantDeps)
	}

	// generate Android.bp
	bp = newGeneratedFile(ctx, "snapshot", "Android.bp")
	generateBpContents(&bp.generatedContents, bpFile)

	contents := bp.content.String()
	// If the snapshot is being generated for the current build release then check the syntax to make
	// sure that it is compatible.
	if targetBuildRelease == currentBuildRelease {
		syntaxCheckSnapshotBpFile(ctx, contents)
	}

	bp.build(pctx, ctx, nil)

	// Copy the build number file into the snapshot.
	builder.CopyToSnapshot(ctx.Config().BuildNumberFile(ctx), BUILD_NUMBER_FILE)

	filesToZip := builder.filesToZip

	// zip them all
	zipPath := fmt.Sprintf("%s%s.zip", ctx.ModuleName(), snapshotZipFileSuffix)
	outputZipFile := android.PathForModuleOut(ctx, zipPath).OutputPath
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
		intermediatePath := fmt.Sprintf("%s%s.unmerged.zip", ctx.ModuleName(), snapshotZipFileSuffix)
		zipFile = android.PathForModuleOut(ctx, intermediatePath).OutputPath
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

// filterOutComponents removes any item from the deps list that is a component of another item in
// the deps list, e.g. if the deps list contains "foo" and "foo.stubs" which is component of "foo"
// then it will remove "foo.stubs" from the deps.
func filterOutComponents(ctx android.ModuleContext, deps []sdkMemberVariantDep) []sdkMemberVariantDep {
	// Collate the set of components that all the modules added to the sdk provide.
	components := map[string]*sdkMemberVariantDep{}
	for i, _ := range deps {
		dep := &deps[i]
		for _, c := range dep.exportedComponentsInfo.Components {
			components[c] = dep
		}
	}

	// If no module provides components then return the input deps unfiltered.
	if len(components) == 0 {
		return deps
	}

	filtered := make([]sdkMemberVariantDep, 0, len(deps))
	for _, dep := range deps {
		name := android.RemoveOptionalPrebuiltPrefix(ctx.OtherModuleName(dep.variant))
		if owner, ok := components[name]; ok {
			// This is a component of another module that is a member of the sdk.

			// If the component is exported but the owning module is not then the configuration is not
			// supported.
			if dep.export && !owner.export {
				ctx.ModuleErrorf("Module %s is internal to the SDK but provides component %s which is used outside the SDK")
				continue
			}

			// This module must not be added to the list of members of the sdk as that would result in a
			// duplicate module in the sdk snapshot.
			continue
		}

		filtered = append(filtered, dep)
	}
	return filtered
}

// addSnapshotModule adds the sdk_snapshot/module_exports_snapshot module to the builder.
func (s *sdk) addSnapshotModule(ctx android.ModuleContext, builder *snapshotBuilder, sdkVariants []*sdk, memberVariantDeps []sdkMemberVariantDep) {
	bpFile := builder.bpFile

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

	combinedPropertiesList := s.collateSnapshotModuleInfo(ctx, sdkVariants, memberVariantDeps)
	commonCombinedProperties := s.optimizeSnapshotModuleProperties(ctx, combinedPropertiesList)

	s.addSnapshotPropertiesToPropertySet(builder, snapshotModule, commonCombinedProperties)

	targetPropertySet := snapshotModule.AddPropertySet("target")

	// Create a mapping from osType to combined properties.
	osTypeToCombinedProperties := map[android.OsType]*combinedSnapshotModuleProperties{}
	for _, combined := range combinedPropertiesList {
		osTypeToCombinedProperties[combined.sdkVariant.Os()] = combined
	}

	// Iterate over the os types in a fixed order.
	for _, osType := range s.getPossibleOsTypes() {
		if combined, ok := osTypeToCombinedProperties[osType]; ok {
			osPropertySet := targetPropertySet.AddPropertySet(osType.Name)

			s.addSnapshotPropertiesToPropertySet(builder, osPropertySet, combined)
		}
	}

	// If host is supported and any member is host OS dependent then disable host
	// by default, so that we can enable each host OS variant explicitly. This
	// avoids problems with implicitly enabled OS variants when the snapshot is
	// used, which might be different from this run (e.g. different build OS).
	if s.HostSupported() {
		var supportedHostTargets []string
		for _, memberVariantDep := range memberVariantDeps {
			if memberVariantDep.memberType.IsHostOsDependent() && memberVariantDep.variant.Target().Os.Class == android.Host {
				targetString := memberVariantDep.variant.Target().Os.String() + "_" + memberVariantDep.variant.Target().Arch.ArchType.String()
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

// snapshotModuleStaticProperties contains snapshot static (i.e. not dynamically generated) properties.
type snapshotModuleStaticProperties struct {
	Compile_multilib string `android:"arch_variant"`
}

// combinedSnapshotModuleProperties are the properties that are associated with the snapshot module.
type combinedSnapshotModuleProperties struct {
	// The sdk variant from which this information was collected.
	sdkVariant *sdk

	// Static snapshot module properties.
	staticProperties *snapshotModuleStaticProperties

	// The dynamically generated member list properties.
	dynamicProperties interface{}
}

// collateSnapshotModuleInfo collates all the snapshot module info from supplied sdk variants.
func (s *sdk) collateSnapshotModuleInfo(ctx android.BaseModuleContext, sdkVariants []*sdk, memberVariantDeps []sdkMemberVariantDep) []*combinedSnapshotModuleProperties {
	sdkVariantToCombinedProperties := map[*sdk]*combinedSnapshotModuleProperties{}
	var list []*combinedSnapshotModuleProperties
	for _, sdkVariant := range sdkVariants {
		staticProperties := &snapshotModuleStaticProperties{
			Compile_multilib: sdkVariant.multilibUsages.String(),
		}
		dynamicProperties := s.dynamicSdkMemberTypes.createMemberTypeListProperties()

		combinedProperties := &combinedSnapshotModuleProperties{
			sdkVariant:        sdkVariant,
			staticProperties:  staticProperties,
			dynamicProperties: dynamicProperties,
		}
		sdkVariantToCombinedProperties[sdkVariant] = combinedProperties

		list = append(list, combinedProperties)
	}

	for _, memberVariantDep := range memberVariantDeps {
		// If the member dependency is internal then do not add the dependency to the snapshot member
		// list properties.
		if !memberVariantDep.export {
			continue
		}

		combined := sdkVariantToCombinedProperties[memberVariantDep.sdkVariant]
		memberListProperty := s.memberTypeListProperty(memberVariantDep.memberType)
		memberName := ctx.OtherModuleName(memberVariantDep.variant)

		if memberListProperty.getter == nil {
			continue
		}

		// Append the member to the appropriate list, if it is not already present in the list.
		memberList := memberListProperty.getter(combined.dynamicProperties)
		if !android.InList(memberName, memberList) {
			memberList = append(memberList, memberName)
		}
		memberListProperty.setter(combined.dynamicProperties, memberList)
	}

	return list
}

func (s *sdk) optimizeSnapshotModuleProperties(ctx android.ModuleContext, list []*combinedSnapshotModuleProperties) *combinedSnapshotModuleProperties {

	// Extract the dynamic properties and add them to a list of propertiesContainer.
	propertyContainers := []propertiesContainer{}
	for _, i := range list {
		propertyContainers = append(propertyContainers, sdkVariantPropertiesContainer{
			sdkVariant: i.sdkVariant,
			properties: i.dynamicProperties,
		})
	}

	// Extract the common members, removing them from the original properties.
	commonDynamicProperties := s.dynamicSdkMemberTypes.createMemberTypeListProperties()
	extractor := newCommonValueExtractor(commonDynamicProperties)
	extractCommonProperties(ctx, extractor, commonDynamicProperties, propertyContainers)

	// Extract the static properties and add them to a list of propertiesContainer.
	propertyContainers = []propertiesContainer{}
	for _, i := range list {
		propertyContainers = append(propertyContainers, sdkVariantPropertiesContainer{
			sdkVariant: i.sdkVariant,
			properties: i.staticProperties,
		})
	}

	commonStaticProperties := &snapshotModuleStaticProperties{}
	extractor = newCommonValueExtractor(commonStaticProperties)
	extractCommonProperties(ctx, extractor, &commonStaticProperties, propertyContainers)

	return &combinedSnapshotModuleProperties{
		sdkVariant:        nil,
		staticProperties:  commonStaticProperties,
		dynamicProperties: commonDynamicProperties,
	}
}

func (s *sdk) addSnapshotPropertiesToPropertySet(builder *snapshotBuilder, propertySet android.BpPropertySet, combined *combinedSnapshotModuleProperties) {
	staticProperties := combined.staticProperties
	multilib := staticProperties.Compile_multilib
	if multilib != "" && multilib != "both" {
		// Compile_multilib defaults to both so only needs to be set when it's specified and not both.
		propertySet.AddProperty("compile_multilib", multilib)
	}

	dynamicMemberTypeListProperties := combined.dynamicProperties
	for _, memberListProperty := range s.memberTypeListProperties() {
		if memberListProperty.getter == nil {
			continue
		}
		if !isMemberTypeSupportedByTargetBuildRelease(memberListProperty.memberType, builder.targetBuildRelease) {
			continue
		}
		names := memberListProperty.getter(dynamicMemberTypeListProperties)
		if len(names) > 0 {
			propertySet.AddProperty(memberListProperty.propertyName(), builder.versionedSdkMemberNames(names, false))
		}
	}
}

type propertyTag struct {
	name string
}

var _ android.BpPropertyTag = propertyTag{}

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
	name := module.Name()
	module.setProperty("name", t.builder.versionedSdkMemberName(name, true))
	module.insertAfter("name", "sdk_member_name", name)
	// Remove the prefer property if present as versioned modules never need marking with prefer.
	module.removeProperty("prefer")
	// Ditto for use_source_config_var
	module.removeProperty("use_source_config_var")
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
	name := module.Name()
	module.setProperty("name", t.builder.unversionedSdkMemberName(name, true))
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
	generateFilteredBpContents(contents, bpFile, func(*bpModule) bool {
		return true
	})
}

func generateFilteredBpContents(contents *generatedContents, bpFile *bpFile, moduleFilter func(module *bpModule) bool) {
	contents.IndentedPrintf("// This is auto-generated. DO NOT EDIT.\n")
	for _, bpModule := range bpFile.order {
		if moduleFilter(bpModule) {
			contents.IndentedPrintf("\n")
			contents.IndentedPrintf("%s {\n", bpModule.moduleType)
			outputPropertySet(contents, bpModule.bpPropertySet)
			contents.IndentedPrintf("}\n")
		}
	}
}

func outputPropertySet(contents *generatedContents, set *bpPropertySet) {
	contents.Indent()

	addComment := func(name string) {
		if text, ok := set.comments[name]; ok {
			for _, line := range strings.Split(text, "\n") {
				contents.IndentedPrintf("// %s\n", line)
			}
		}
	}

	// Output the properties first, followed by the nested sets. This ensures a
	// consistent output irrespective of whether property sets are created before
	// or after the properties. This simplifies the creation of the module.
	for _, name := range set.order {
		value := set.getValue(name)

		// Do not write property sets in the properties phase.
		if _, ok := value.(*bpPropertySet); ok {
			continue
		}

		addComment(name)
		reflectValue := reflect.ValueOf(value)
		outputNamedValue(contents, name, reflectValue)
	}

	for _, name := range set.order {
		value := set.getValue(name)

		// Only write property sets in the sets phase.
		switch v := value.(type) {
		case *bpPropertySet:
			addComment(name)
			contents.IndentedPrintf("%s: {\n", name)
			outputPropertySet(contents, v)
			contents.IndentedPrintf("},\n")
		}
	}

	contents.Dedent()
}

// outputNamedValue outputs a value that has an associated name. The name will be indented, followed
// by the value and then followed by a , and a newline.
func outputNamedValue(contents *generatedContents, name string, value reflect.Value) {
	contents.IndentedPrintf("%s: ", name)
	outputUnnamedValue(contents, value)
	contents.UnindentedPrintf(",\n")
}

// outputUnnamedValue outputs a single value. The value is not indented and is not followed by
// either a , or a newline. With multi-line values, e.g. slices, all but the first line will be
// indented and all but the last line will end with a newline.
func outputUnnamedValue(contents *generatedContents, value reflect.Value) {
	valueType := value.Type()
	switch valueType.Kind() {
	case reflect.Bool:
		contents.UnindentedPrintf("%t", value.Bool())

	case reflect.String:
		contents.UnindentedPrintf("%q", value)

	case reflect.Ptr:
		outputUnnamedValue(contents, value.Elem())

	case reflect.Slice:
		length := value.Len()
		if length == 0 {
			contents.UnindentedPrintf("[]")
		} else {
			firstValue := value.Index(0)
			if length == 1 && !multiLineValue(firstValue) {
				contents.UnindentedPrintf("[")
				outputUnnamedValue(contents, firstValue)
				contents.UnindentedPrintf("]")
			} else {
				contents.UnindentedPrintf("[\n")
				contents.Indent()
				for i := 0; i < length; i++ {
					itemValue := value.Index(i)
					contents.IndentedPrintf("")
					outputUnnamedValue(contents, itemValue)
					contents.UnindentedPrintf(",\n")
				}
				contents.Dedent()
				contents.IndentedPrintf("]")
			}
		}

	case reflect.Struct:
		// Avoid unlimited recursion by requiring every structure to implement android.BpPrintable.
		v := value.Interface()
		if _, ok := v.(android.BpPrintable); !ok {
			panic(fmt.Errorf("property value %#v of type %T does not implement android.BpPrintable", v, v))
		}
		contents.UnindentedPrintf("{\n")
		contents.Indent()
		for f := 0; f < valueType.NumField(); f++ {
			fieldType := valueType.Field(f)
			if fieldType.Anonymous {
				continue
			}
			fieldValue := value.Field(f)
			fieldName := fieldType.Name
			propertyName := proptools.PropertyNameForField(fieldName)
			outputNamedValue(contents, propertyName, fieldValue)
		}
		contents.Dedent()
		contents.IndentedPrintf("}")

	default:
		panic(fmt.Errorf("Unknown type: %T of value %#v", value, value))
	}
}

// multiLineValue returns true if the supplied value may require multiple lines in the output.
func multiLineValue(value reflect.Value) bool {
	kind := value.Kind()
	return kind == reflect.Slice || kind == reflect.Struct
}

func (s *sdk) GetAndroidBpContentsForTests() string {
	contents := &generatedContents{}
	generateBpContents(contents, s.builderForTests.bpFile)
	return contents.content.String()
}

func (s *sdk) GetUnversionedAndroidBpContentsForTests() string {
	contents := &generatedContents{}
	generateFilteredBpContents(contents, s.builderForTests.bpFile, func(module *bpModule) bool {
		name := module.Name()
		// Include modules that are either unversioned or have no name.
		return !strings.Contains(name, "@")
	})
	return contents.content.String()
}

func (s *sdk) GetVersionedAndroidBpContentsForTests() string {
	contents := &generatedContents{}
	generateFilteredBpContents(contents, s.builderForTests.bpFile, func(module *bpModule) bool {
		name := module.Name()
		// Include modules that are either versioned or have no name.
		return name == "" || strings.Contains(name, "@")
	})
	return contents.content.String()
}

type snapshotBuilder struct {
	ctx android.ModuleContext
	sdk *sdk

	// The version of the generated snapshot.
	//
	// See the documentation of SOONG_SDK_SNAPSHOT_VERSION above for details of the valid values of
	// this field.
	version string

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

	// The target build release for which the snapshot is to be generated.
	targetBuildRelease *buildRelease
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

	// The licenses are the same for all variants.
	mctx := s.ctx
	licenseInfo := mctx.OtherModuleProvider(variant, android.LicenseInfoProvider).(android.LicenseInfo)
	if len(licenseInfo.Licenses) > 0 {
		m.AddPropertyWithTag("licenses", licenseInfo.Licenses, s.OptionalSdkMemberReferencePropertyTag())
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
	// If neither device or host is supported then this module does not support either so will not
	// recognize the properties.
	if !deviceSupported && !hostSupported {
		return
	}

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

// sdkMemberVariantDep represents a dependency from an sdk variant onto a member variant.
type sdkMemberVariantDep struct {
	// The sdk variant that depends (possibly indirectly) on the member variant.
	sdkVariant *sdk

	// The type of sdk member the variant is to be treated as.
	memberType android.SdkMemberType

	// The variant that is added to the sdk.
	variant android.SdkAware

	// True if the member should be exported, i.e. accessible, from outside the sdk.
	export bool

	// The names of additional component modules provided by the variant.
	exportedComponentsInfo android.ExportedComponentsInfo
}

var _ android.SdkMember = (*sdkMember)(nil)

// sdkMember groups all the variants of a specific member module together along with the name of the
// module and the member type. This is used to generate the prebuilt modules for a specific member.
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
	var variantsByArchId = make(map[archId][]android.Module)
	var archIds []archId
	for _, variant := range osTypeVariants {
		target := variant.Target()
		id := archIdFromTarget(target)
		if _, ok := variantsByArchId[id]; !ok {
			archIds = append(archIds, id)
		}

		variantsByArchId[id] = append(variantsByArchId[id], variant)
	}

	if commonVariants, ok := variantsByArchId[commonArchId]; ok {
		if len(osTypeVariants) != 1 {
			panic(fmt.Errorf("Expected to only have 1 variant when arch type is common but found %d", len(osTypeVariants)))
		}

		// A common arch type only has one variant and its properties should be treated
		// as common to the os type.
		osInfo.Properties.PopulateFromVariant(ctx, commonVariants[0])
	} else {
		// Create an arch specific info for each supported architecture type.
		for _, id := range archIds {
			archVariants := variantsByArchId[id]
			archInfo := newArchSpecificInfo(ctx, id, osType, osSpecificVariantPropertiesFactory, archVariants)

			osInfo.archInfos = append(osInfo.archInfos, archInfo)
		}
	}

	return osInfo
}

func (osInfo *osTypeSpecificInfo) pruneUnsupportedProperties(pruner *propertyPruner) {
	if len(osInfo.archInfos) == 0 {
		pruner.pruneProperties(osInfo.Properties)
	} else {
		for _, archInfo := range osInfo.archInfos {
			archInfo.pruneUnsupportedProperties(pruner)
		}
	}
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
		multilib = multilib.addArchType(archInfo.archId.archType)

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

// archId encapsulates the information needed to identify a combination of arch type and native
// bridge support.
//
// Conceptually, native bridge support is a facet of an android.Target, not an android.Arch as it is
// essentially using one android.Arch to implement another. However, in terms of the handling of
// the variants native bridge is treated as part of the arch variation. See the ArchVariation method
// on android.Target.
//
// So, it makes sense when optimizing the variants to combine native bridge with the arch type.
type archId struct {
	// The arch type of the variant's target.
	archType android.ArchType

	// True if the variants is for the native bridge, false otherwise.
	nativeBridge bool
}

// propertyName returns the name of the property corresponding to use for this arch id.
func (i *archId) propertyName() string {
	name := i.archType.Name
	if i.nativeBridge {
		// Note: This does not result in a valid property because there is no architecture specific
		// native bridge property, only a generic "native_bridge" property. However, this will be used
		// in error messages if there is an attempt to use this in a generated bp file.
		name += "_native_bridge"
	}
	return name
}

func (i *archId) String() string {
	return fmt.Sprintf("ArchType{%s}, NativeBridge{%t}", i.archType, i.nativeBridge)
}

// archIdFromTarget returns an archId initialized from information in the supplied target.
func archIdFromTarget(target android.Target) archId {
	return archId{
		archType:     target.Arch.ArchType,
		nativeBridge: target.NativeBridge == android.NativeBridgeEnabled,
	}
}

// commonArchId is the archId for the common architecture.
var commonArchId = archId{archType: android.Common}

type archTypeSpecificInfo struct {
	baseInfo

	archId archId
	osType android.OsType

	imageVariantInfos []*imageVariantSpecificInfo
}

var _ propertiesContainer = (*archTypeSpecificInfo)(nil)

// Create a new archTypeSpecificInfo for the specified arch type and its properties
// structures populated with information from the variants.
func newArchSpecificInfo(ctx android.SdkMemberContext, archId archId, osType android.OsType, variantPropertiesFactory variantPropertiesFactoryFunc, archVariants []android.Module) *archTypeSpecificInfo {

	// Create an arch specific info into which the variant properties can be copied.
	archInfo := &archTypeSpecificInfo{archId: archId, osType: osType}

	// Create the properties into which the arch type specific properties will be
	// added.
	archInfo.Properties = variantPropertiesFactory()

	if len(archVariants) == 1 {
		archInfo.Properties.PopulateFromVariant(ctx, archVariants[0])
	} else {
		// Group the variants by image type.
		variantsByImage := make(map[string][]android.Module)
		for _, variant := range archVariants {
			image := variant.ImageVariation().Variation
			variantsByImage[image] = append(variantsByImage[image], variant)
		}

		// Create the image variant info in a fixed order.
		for _, imageVariantName := range android.SortedStringKeys(variantsByImage) {
			variants := variantsByImage[imageVariantName]
			archInfo.imageVariantInfos = append(archInfo.imageVariantInfos, newImageVariantSpecificInfo(ctx, imageVariantName, variantPropertiesFactory, variants))
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

func (archInfo *archTypeSpecificInfo) pruneUnsupportedProperties(pruner *propertyPruner) {
	if len(archInfo.imageVariantInfos) == 0 {
		pruner.pruneProperties(archInfo.Properties)
	} else {
		for _, imageVariantInfo := range archInfo.imageVariantInfos {
			imageVariantInfo.pruneUnsupportedProperties(pruner)
		}
	}
}

// Optimize the properties by extracting common properties from link type specific
// properties into arch type specific properties.
func (archInfo *archTypeSpecificInfo) optimizeProperties(ctx *memberContext, commonValueExtractor *commonValueExtractor) {
	if len(archInfo.imageVariantInfos) == 0 {
		return
	}

	// Optimize the image variant properties first.
	for _, imageVariantInfo := range archInfo.imageVariantInfos {
		imageVariantInfo.optimizeProperties(ctx, commonValueExtractor)
	}

	extractCommonProperties(ctx.sdkMemberContext, commonValueExtractor, archInfo.Properties, archInfo.imageVariantInfos)
}

// Add the properties for an arch type to a property set.
func (archInfo *archTypeSpecificInfo) addToPropertySet(ctx *memberContext, archPropertySet android.BpPropertySet, archOsPrefix string) {
	archPropertySuffix := archInfo.archId.propertyName()
	propertySetName := archOsPrefix + archPropertySuffix
	archTypePropertySet := archPropertySet.AddPropertySet(propertySetName)
	// Enable the <os>_<arch> variant explicitly when we've disabled it by default on host.
	if ctx.memberType.IsHostOsDependent() && archInfo.osType.Class == android.Host {
		archTypePropertySet.AddProperty("enabled", true)
	}
	addSdkMemberPropertiesToSet(ctx, archInfo.Properties, archTypePropertySet)

	for _, imageVariantInfo := range archInfo.imageVariantInfos {
		imageVariantInfo.addToPropertySet(ctx, archTypePropertySet)
	}

	// If this is for a native bridge architecture then make sure that the property set does not
	// contain any properties as providing native bridge specific properties is not currently
	// supported.
	if archInfo.archId.nativeBridge {
		propertySetContents := getPropertySetContents(archTypePropertySet)
		if propertySetContents != "" {
			ctx.SdkModuleContext().ModuleErrorf("Architecture variant %q of sdk member %q has properties distinct from other variants; this is not yet supported. The properties are:\n%s",
				propertySetName, ctx.name, propertySetContents)
		}
	}
}

// getPropertySetContents returns the string representation of the contents of a property set, after
// recursively pruning any empty nested property sets.
func getPropertySetContents(propertySet android.BpPropertySet) string {
	set := propertySet.(*bpPropertySet)
	set.transformContents(pruneEmptySetTransformer{})
	if len(set.properties) != 0 {
		contents := &generatedContents{}
		contents.Indent()
		outputPropertySet(contents, set)
		setAsString := contents.content.String()
		return setAsString
	}
	return ""
}

func (archInfo *archTypeSpecificInfo) String() string {
	return archInfo.archId.String()
}

type imageVariantSpecificInfo struct {
	baseInfo

	imageVariant string

	linkInfos []*linkTypeSpecificInfo
}

func newImageVariantSpecificInfo(ctx android.SdkMemberContext, imageVariant string, variantPropertiesFactory variantPropertiesFactoryFunc, imageVariants []android.Module) *imageVariantSpecificInfo {

	// Create an image variant specific info into which the variant properties can be copied.
	imageInfo := &imageVariantSpecificInfo{imageVariant: imageVariant}

	// Create the properties into which the image variant specific properties will be added.
	imageInfo.Properties = variantPropertiesFactory()

	if len(imageVariants) == 1 {
		imageInfo.Properties.PopulateFromVariant(ctx, imageVariants[0])
	} else {
		// There is more than one variant for this image variant which must be differentiated by link
		// type.
		for _, linkVariant := range imageVariants {
			linkType := getLinkType(linkVariant)
			if linkType == "" {
				panic(fmt.Errorf("expected one arch specific variant as it is not identified by link type but found %d", len(imageVariants)))
			} else {
				linkInfo := newLinkSpecificInfo(ctx, linkType, variantPropertiesFactory, linkVariant)

				imageInfo.linkInfos = append(imageInfo.linkInfos, linkInfo)
			}
		}
	}

	return imageInfo
}

func (imageInfo *imageVariantSpecificInfo) pruneUnsupportedProperties(pruner *propertyPruner) {
	if len(imageInfo.linkInfos) == 0 {
		pruner.pruneProperties(imageInfo.Properties)
	} else {
		for _, linkInfo := range imageInfo.linkInfos {
			linkInfo.pruneUnsupportedProperties(pruner)
		}
	}
}

// Optimize the properties by extracting common properties from link type specific
// properties into arch type specific properties.
func (imageInfo *imageVariantSpecificInfo) optimizeProperties(ctx *memberContext, commonValueExtractor *commonValueExtractor) {
	if len(imageInfo.linkInfos) == 0 {
		return
	}

	extractCommonProperties(ctx.sdkMemberContext, commonValueExtractor, imageInfo.Properties, imageInfo.linkInfos)
}

// Add the properties for an arch type to a property set.
func (imageInfo *imageVariantSpecificInfo) addToPropertySet(ctx *memberContext, propertySet android.BpPropertySet) {
	if imageInfo.imageVariant != android.CoreVariation {
		propertySet = propertySet.AddPropertySet(imageInfo.imageVariant)
	}

	addSdkMemberPropertiesToSet(ctx, imageInfo.Properties, propertySet)

	for _, linkInfo := range imageInfo.linkInfos {
		linkInfo.addToPropertySet(ctx, propertySet)
	}

	// If this is for a non-core image variant then make sure that the property set does not contain
	// any properties as providing non-core image variant specific properties for prebuilts is not
	// currently supported.
	if imageInfo.imageVariant != android.CoreVariation {
		propertySetContents := getPropertySetContents(propertySet)
		if propertySetContents != "" {
			ctx.SdkModuleContext().ModuleErrorf("Image variant %q of sdk member %q has properties distinct from other variants; this is not yet supported. The properties are:\n%s",
				imageInfo.imageVariant, ctx.name, propertySetContents)
		}
	}
}

func (imageInfo *imageVariantSpecificInfo) String() string {
	return imageInfo.imageVariant
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

func (l *linkTypeSpecificInfo) addToPropertySet(ctx *memberContext, propertySet android.BpPropertySet) {
	linkPropertySet := propertySet.AddPropertySet(l.linkType)
	addSdkMemberPropertiesToSet(ctx, l.Properties, linkPropertySet)
}

func (l *linkTypeSpecificInfo) pruneUnsupportedProperties(pruner *propertyPruner) {
	pruner.pruneProperties(l.Properties)
}

func (l *linkTypeSpecificInfo) String() string {
	return fmt.Sprintf("LinkType{%s}", l.linkType)
}

type memberContext struct {
	sdkMemberContext android.ModuleContext
	builder          *snapshotBuilder
	memberType       android.SdkMemberType
	name             string

	// The set of traits required of this member.
	requiredTraits android.SdkMemberTraitSet
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

func (m *memberContext) RequiresTrait(trait android.SdkMemberTrait) bool {
	return m.requiredTraits.Contains(trait)
}

func (m *memberContext) IsTargetBuildBeforeTiramisu() bool {
	return m.builder.targetBuildRelease.EarlierThan(buildReleaseT)
}

var _ android.SdkMemberContext = (*memberContext)(nil)

func (s *sdk) createMemberSnapshot(ctx *memberContext, member *sdkMember, bpModule *bpModule) {

	memberType := member.memberType

	// Do not add the prefer property if the member snapshot module is a source module type.
	config := ctx.sdkMemberContext.Config()
	if !memberType.UsesSourceModuleTypeInSnapshot() {
		// Set the prefer based on the environment variable. This is a temporary work around to allow a
		// snapshot to be created that sets prefer: true.
		// TODO(b/174997203): Remove once the ability to select the modules to prefer can be done
		//  dynamically at build time not at snapshot generation time.
		prefer := config.IsEnvTrue("SOONG_SDK_SNAPSHOT_PREFER")

		// Set prefer. Setting this to false is not strictly required as that is the default but it does
		// provide a convenient hook to post-process the generated Android.bp file, e.g. in tests to
		// check the behavior when a prebuilt is preferred. It also makes it explicit what the default
		// behavior is for the module.
		bpModule.insertAfter("name", "prefer", prefer)

		configVar := config.Getenv("SOONG_SDK_SNAPSHOT_USE_SOURCE_CONFIG_VAR")
		if configVar != "" {
			parts := strings.Split(configVar, ":")
			cfp := android.ConfigVarProperties{
				Config_namespace: proptools.StringPtr(parts[0]),
				Var_name:         proptools.StringPtr(parts[1]),
			}
			bpModule.insertAfter("prefer", "use_source_config_var", cfp)
		}
	}

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

	// Create a property pruner that will prune any properties unsupported by the target build
	// release.
	targetBuildRelease := ctx.builder.targetBuildRelease
	unsupportedPropertyPruner := newPropertyPrunerByBuildRelease(commonProperties, targetBuildRelease)

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

		osInfo.pruneUnsupportedProperties(unsupportedPropertyPruner)

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
	for _, osType := range android.OsTypeList() {
		if s.DeviceSupported() {
			if osType.Class == android.Device {
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

// A wrapper for sdk variant related properties to allow them to be optimized.
type sdkVariantPropertiesContainer struct {
	sdkVariant *sdk
	properties interface{}
}

func (c sdkVariantPropertiesContainer) optimizableProperties() interface{} {
	return c.properties
}

func (c sdkVariantPropertiesContainer) String() string {
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
