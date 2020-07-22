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
	"reflect"
	"strconv"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	// This package doesn't depend on the apex package, but import it to make its mutators to be
	// registered before mutators in this package. See RegisterPostDepsMutators for more details.
	_ "android/soong/apex"
)

func init() {
	pctx.Import("android/soong/android")
	pctx.Import("android/soong/java/config")

	android.RegisterModuleType("sdk", SdkModuleFactory)
	android.RegisterModuleType("sdk_snapshot", SnapshotModuleFactory)
	android.PreDepsMutators(RegisterPreDepsMutators)
	android.PostDepsMutators(RegisterPostDepsMutators)
}

type sdk struct {
	android.ModuleBase
	android.DefaultableModuleBase

	// The dynamically generated information about the registered SdkMemberType
	dynamicSdkMemberTypes *dynamicSdkMemberTypes

	// The dynamically created instance of the properties struct containing the sdk member
	// list properties, e.g. java_libs.
	dynamicMemberTypeListProperties interface{}

	// Information about the OsType specific member variants associated with this variant.
	//
	// Set by OsType specific variants in the collectMembers() method and used by the
	// CommonOS variant when building the snapshot. That work is all done on separate
	// calls to the sdk.GenerateAndroidBuildActions method which is guaranteed to be
	// called for the OsType specific variants before the CommonOS variant (because
	// the latter depends on the former).
	memberRefs []sdkMemberRef

	// The multilib variants that are used by this sdk variant.
	multilibUsages multilibUsage

	properties sdkProperties

	snapshotFile android.OptionalPath

	// The builder, preserved for testing.
	builderForTests *snapshotBuilder
}

type sdkProperties struct {
	Snapshot bool `blueprint:"mutated"`

	// True if this is a module_exports (or module_exports_snapshot) module type.
	Module_exports bool `blueprint:"mutated"`
}

// Contains information about the sdk properties that list sdk members, e.g.
// Java_header_libs.
type sdkMemberListProperty struct {
	// getter for the list of member names
	getter func(properties interface{}) []string

	// the type of member referenced in the list
	memberType android.SdkMemberType

	// the dependency tag used for items in this list that can be used to determine the memberType
	// for a resolved dependency.
	dependencyTag android.SdkMemberTypeDependencyTag
}

func (p *sdkMemberListProperty) propertyName() string {
	return p.memberType.SdkPropertyName()
}

// Cache of dynamically generated dynamicSdkMemberTypes objects. The key is the pointer
// to a slice of SdkMemberType instances held in android.SdkMemberTypes.
var dynamicSdkMemberTypesMap android.OncePer

// A dynamically generated set of member list properties and associated structure type.
type dynamicSdkMemberTypes struct {
	// The dynamically generated structure type.
	//
	// Contains one []string exported field for each android.SdkMemberTypes. The name of the field
	// is the exported form of the value returned by SdkMemberType.SdkPropertyName().
	propertiesStructType reflect.Type

	// Information about each of the member type specific list properties.
	memberListProperties []*sdkMemberListProperty
}

func (d *dynamicSdkMemberTypes) createMemberListProperties() interface{} {
	return reflect.New(d.propertiesStructType).Interface()
}

func getDynamicSdkMemberTypes(registry *android.SdkMemberTypesRegistry) *dynamicSdkMemberTypes {

	// Get a key that uniquely identifies the registry contents.
	key := registry.UniqueOnceKey()

	// Get the registered types.
	registeredTypes := registry.RegisteredTypes()

	// Get the cached value, creating new instance if necessary.
	return dynamicSdkMemberTypesMap.Once(key, func() interface{} {
		return createDynamicSdkMemberTypes(registeredTypes)
	}).(*dynamicSdkMemberTypes)
}

// Create the dynamicSdkMemberTypes from the list of registered member types.
//
// A struct is created which contains one exported field per member type corresponding to
// the SdkMemberType.SdkPropertyName() value.
//
// A list of sdkMemberListProperty instances is created, one per member type that provides:
// * a reference to the member type.
// * a getter for the corresponding field in the properties struct.
// * a dependency tag that identifies the member type of a resolved dependency.
//
func createDynamicSdkMemberTypes(sdkMemberTypes []android.SdkMemberType) *dynamicSdkMemberTypes {

	var listProperties []*sdkMemberListProperty
	var fields []reflect.StructField

	// Iterate over the member types creating StructField and sdkMemberListProperty objects.
	for f, memberType := range sdkMemberTypes {
		p := memberType.SdkPropertyName()

		// Create a dynamic exported field for the member type's property.
		fields = append(fields, reflect.StructField{
			Name: proptools.FieldNameForProperty(p),
			Type: reflect.TypeOf([]string{}),
			Tag:  `android:"arch_variant"`,
		})

		// Copy the field index for use in the getter func as using the loop variable directly will
		// cause all funcs to use the last value.
		fieldIndex := f

		// Create an sdkMemberListProperty for the member type.
		memberListProperty := &sdkMemberListProperty{
			getter: func(properties interface{}) []string {
				// The properties is expected to be of the following form (where
				// <Module_types> is the name of an SdkMemberType.SdkPropertyName().
				//     properties *struct {<Module_types> []string, ....}
				//
				// Although it accesses the field by index the following reflection code is equivalent to:
				//    *properties.<Module_types>
				//
				list := reflect.ValueOf(properties).Elem().Field(fieldIndex).Interface().([]string)
				return list
			},

			memberType: memberType,

			dependencyTag: android.DependencyTagForSdkMemberType(memberType),
		}

		listProperties = append(listProperties, memberListProperty)
	}

	// Create a dynamic struct from the collated fields.
	propertiesStructType := reflect.StructOf(fields)

	return &dynamicSdkMemberTypes{
		memberListProperties: listProperties,
		propertiesStructType: propertiesStructType,
	}
}

// sdk defines an SDK which is a logical group of modules (e.g. native libs, headers, java libs, etc.)
// which Mainline modules like APEX can choose to build with.
func SdkModuleFactory() android.Module {
	return newSdkModule(false)
}

func newSdkModule(moduleExports bool) *sdk {
	s := &sdk{}
	s.properties.Module_exports = moduleExports
	// Get the dynamic sdk member type data for the currently registered sdk member types.
	var registry *android.SdkMemberTypesRegistry
	if moduleExports {
		registry = android.ModuleExportsMemberTypes
	} else {
		registry = android.SdkMemberTypes
	}
	s.dynamicSdkMemberTypes = getDynamicSdkMemberTypes(registry)
	// Create an instance of the dynamically created struct that contains all the
	// properties for the member type specific list properties.
	s.dynamicMemberTypeListProperties = s.dynamicSdkMemberTypes.createMemberListProperties()
	s.AddProperties(&s.properties, s.dynamicMemberTypeListProperties)
	android.InitCommonOSAndroidMultiTargetsArchModule(s, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(s)
	android.AddLoadHook(s, func(ctx android.LoadHookContext) {
		type props struct {
			Compile_multilib *string
		}
		p := &props{Compile_multilib: proptools.StringPtr("both")}
		ctx.AppendProperties(p)
	})
	return s
}

// sdk_snapshot is a versioned snapshot of an SDK. This is an auto-generated module.
func SnapshotModuleFactory() android.Module {
	s := newSdkModule(false)
	s.properties.Snapshot = true
	return s
}

func (s *sdk) memberListProperties() []*sdkMemberListProperty {
	return s.dynamicSdkMemberTypes.memberListProperties
}

func (s *sdk) getExportedMembers() map[string]struct{} {
	// Collect all the exported members.
	exportedMembers := make(map[string]struct{})

	for _, memberListProperty := range s.memberListProperties() {
		names := memberListProperty.getter(s.dynamicMemberTypeListProperties)

		// Every member specified explicitly in the properties is exported by the sdk.
		for _, name := range names {
			exportedMembers[name] = struct{}{}
		}
	}

	return exportedMembers
}

func (s *sdk) snapshot() bool {
	return s.properties.Snapshot
}

func (s *sdk) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if s.snapshot() {
		// We don't need to create a snapshot out of sdk_snapshot.
		// That doesn't make sense. We need a snapshot to create sdk_snapshot.
		return
	}

	// This method is guaranteed to be called on OsType specific variants before it is called
	// on their corresponding CommonOS variant.
	if !s.IsCommonOSVariant() {
		// Update the OsType specific sdk variant with information about its members.
		s.collectMembers(ctx)
	} else {
		// Get the OsType specific variants on which the CommonOS depends.
		osSpecificVariants := android.GetOsSpecificVariantsOfCommonOSVariant(ctx)
		var sdkVariants []*sdk
		for _, m := range osSpecificVariants {
			if sdkVariant, ok := m.(*sdk); ok {
				sdkVariants = append(sdkVariants, sdkVariant)
			}
		}

		// Generate the snapshot from the member info.
		p := s.buildSnapshot(ctx, sdkVariants)
		s.snapshotFile = android.OptionalPathForPath(p)
		ctx.InstallFile(android.PathForMainlineSdksInstall(ctx), s.Name()+"-current.zip", p)
	}
}

func (s *sdk) AndroidMkEntries() []android.AndroidMkEntries {
	if !s.snapshotFile.Valid() {
		return []android.AndroidMkEntries{}
	}

	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "FAKE",
		OutputFile: s.snapshotFile,
		DistFile:   s.snapshotFile,
		Include:    "$(BUILD_PHONY_PACKAGE)",
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
				// Allow the sdk to be built by simply passing its name on the command line.
				fmt.Fprintln(w, ".PHONY:", s.Name())
				fmt.Fprintln(w, s.Name()+":", s.snapshotFile.String())
			},
		},
	}}
}

// RegisterPreDepsMutators registers pre-deps mutators to support modules implementing SdkAware
// interface and the sdk module type. This function has been made public to be called by tests
// outside of the sdk package
func RegisterPreDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.BottomUp("SdkMember", memberMutator).Parallel()
	ctx.TopDown("SdkMember_deps", memberDepsMutator).Parallel()
	ctx.BottomUp("SdkMemberInterVersion", memberInterVersionMutator).Parallel()
}

// RegisterPostDepsMutators registers post-deps mutators to support modules implementing SdkAware
// interface and the sdk module type. This function has been made public to be called by tests
// outside of the sdk package
func RegisterPostDepsMutators(ctx android.RegisterMutatorsContext) {
	// These must run AFTER apexMutator. Note that the apex package is imported even though there is
	// no direct dependency to the package here. sdkDepsMutator sets the SDK requirements from an
	// APEX to its dependents. Since different versions of the same SDK can be used by different
	// APEXes, the apex and its dependents (which includes the dependencies to the sdk members)
	// should have been mutated for the apex before the SDK requirements are set.
	ctx.TopDown("SdkDepsMutator", sdkDepsMutator).Parallel()
	ctx.BottomUp("SdkDepsReplaceMutator", sdkDepsReplaceMutator).Parallel()
	ctx.TopDown("SdkRequirementCheck", sdkRequirementsMutator).Parallel()
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
}

// For dependencies from an in-development version of an SDK member to frozen versions of the same member
// e.g. libfoo -> libfoo.mysdk.11 and libfoo.mysdk.12
type sdkMemberVersionedDepTag struct {
	dependencyTag
	member  string
	version string
}

// Mark this tag so dependencies that use it are excluded from visibility enforcement.
func (t sdkMemberVersionedDepTag) ExcludeFromVisibilityEnforcement() {}

// Step 1: create dependencies from an SDK module to its members.
func memberMutator(mctx android.BottomUpMutatorContext) {
	if s, ok := mctx.Module().(*sdk); ok {
		// Add dependencies from enabled and non CommonOS variants to the sdk member variants.
		if s.Enabled() && !s.IsCommonOSVariant() {
			for _, memberListProperty := range s.memberListProperties() {
				names := memberListProperty.getter(s.dynamicMemberTypeListProperties)
				if len(names) > 0 {
					tag := memberListProperty.dependencyTag
					memberListProperty.memberType.AddDependencies(mctx, tag, names)
				}
			}
		}
	}
}

// Step 2: record that dependencies of SDK modules are members of the SDK modules
func memberDepsMutator(mctx android.TopDownMutatorContext) {
	if s, ok := mctx.Module().(*sdk); ok {
		mySdkRef := android.ParseSdkRef(mctx, mctx.ModuleName(), "name")
		if s.snapshot() && mySdkRef.Unversioned() {
			mctx.PropertyErrorf("name", "sdk_snapshot should be named as <name>@<version>. "+
				"Did you manually modify Android.bp?")
		}
		if !s.snapshot() && !mySdkRef.Unversioned() {
			mctx.PropertyErrorf("name", "sdk shouldn't be named as <name>@<version>.")
		}
		if mySdkRef.Version != "" && mySdkRef.Version != "current" {
			if _, err := strconv.Atoi(mySdkRef.Version); err != nil {
				mctx.PropertyErrorf("name", "version %q is neither a number nor \"current\"", mySdkRef.Version)
			}
		}

		mctx.VisitDirectDeps(func(child android.Module) {
			if member, ok := child.(android.SdkAware); ok {
				member.MakeMemberOf(mySdkRef)
			}
		})
	}
}

// Step 3: create dependencies from the unversioned SDK member to snapshot versions
// of the same member. By having these dependencies, they are mutated for multiple Mainline modules
// (apex and apk), each of which might want different sdks to be built with. For example, if both
// apex A and B are referencing libfoo which is a member of sdk 'mysdk', the two APEXes can be
// built with libfoo.mysdk.11 and libfoo.mysdk.12, respectively depending on which sdk they are
// using.
func memberInterVersionMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(android.SdkAware); ok && m.IsInAnySdk() {
		if !m.ContainingSdk().Unversioned() {
			memberName := m.MemberName()
			tag := sdkMemberVersionedDepTag{member: memberName, version: m.ContainingSdk().Version}
			mctx.AddReverseDependency(mctx.Module(), tag, memberName)
		}
	}
}

// Step 4: transitively ripple down the SDK requirements from the root modules like APEX to its
// descendants
func sdkDepsMutator(mctx android.TopDownMutatorContext) {
	if parent, ok := mctx.Module().(interface {
		android.DepIsInSameApex
		android.RequiredSdks
	}); ok {
		// Module types for Mainline modules (e.g. APEX) are expected to implement RequiredSdks()
		// by reading its own properties like `uses_sdks`.
		requiredSdks := parent.RequiredSdks()
		if len(requiredSdks) > 0 {
			mctx.VisitDirectDeps(func(m android.Module) {
				// Only propagate required sdks from the apex onto its contents.
				if dep, ok := m.(android.SdkAware); ok && parent.DepIsInSameApex(mctx, dep) {
					dep.BuildWithSdks(requiredSdks)
				}
			})
		}
	}
}

// Step 5: if libfoo.mysdk.11 is in the context where version 11 of mysdk is requested, the
// versioned module is used instead of the un-versioned (in-development) module libfoo
func sdkDepsReplaceMutator(mctx android.BottomUpMutatorContext) {
	if versionedSdkMember, ok := mctx.Module().(android.SdkAware); ok && versionedSdkMember.IsInAnySdk() {
		if sdk := versionedSdkMember.ContainingSdk(); !sdk.Unversioned() {
			// Only replace dependencies to <sdkmember> with <sdkmember@required-version>
			// if the depending module requires it. e.g.
			//      foo -> sdkmember
			// will be transformed to:
			//      foo -> sdkmember@1
			// if and only if foo is a member of an APEX that requires version 1 of the
			// sdk containing sdkmember.
			memberName := versionedSdkMember.MemberName()

			// Replace dependencies on sdkmember with a dependency on the current module which
			// is a versioned prebuilt of the sdkmember if required.
			mctx.ReplaceDependenciesIf(memberName, func(from blueprint.Module, tag blueprint.DependencyTag, to blueprint.Module) bool {
				// from - foo
				// to - sdkmember
				replace := false
				if parent, ok := from.(android.RequiredSdks); ok {
					replace = parent.RequiredSdks().Contains(sdk)
				}
				return replace
			})
		}
	}
}

// Step 6: ensure that the dependencies outside of the APEX are all from the required SDKs
func sdkRequirementsMutator(mctx android.TopDownMutatorContext) {
	if m, ok := mctx.Module().(interface {
		android.DepIsInSameApex
		android.RequiredSdks
	}); ok {
		requiredSdks := m.RequiredSdks()
		if len(requiredSdks) == 0 {
			return
		}
		mctx.VisitDirectDeps(func(dep android.Module) {
			tag := mctx.OtherModuleDependencyTag(dep)
			if tag == android.DefaultsDepTag {
				// dependency to defaults is always okay
				return
			}

			// Ignore the dependency from the unversioned member to any versioned members as an
			// apex that depends on the unversioned member will not also be depending on a versioned
			// member.
			if _, ok := tag.(sdkMemberVersionedDepTag); ok {
				return
			}

			// If the dep is outside of the APEX, but is not in any of the
			// required SDKs, we know that the dep is a violation.
			if sa, ok := dep.(android.SdkAware); ok {
				if !m.DepIsInSameApex(mctx, dep) && !requiredSdks.Contains(sa.ContainingSdk()) {
					mctx.ModuleErrorf("depends on %q (in SDK %q) that isn't part of the required SDKs: %v",
						sa.Name(), sa.ContainingSdk(), requiredSdks)
				}
			}
		})
	}
}
