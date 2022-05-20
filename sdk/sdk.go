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

	registerSdkBuildComponents(android.InitRegistrationContext)
}

func registerSdkBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("sdk", SdkModuleFactory)
	ctx.RegisterModuleType("sdk_snapshot", SnapshotModuleFactory)
	ctx.PreDepsMutators(RegisterPreDepsMutators)
}

type sdk struct {
	android.ModuleBase
	android.DefaultableModuleBase

	// The dynamically generated information about the registered SdkMemberType
	dynamicSdkMemberTypes *dynamicSdkMemberTypes

	// The dynamically created instance of the properties struct containing the sdk member type
	// list properties, e.g. java_libs.
	dynamicMemberTypeListProperties interface{}

	// The dynamically generated information about the registered SdkMemberTrait
	dynamicSdkMemberTraits *dynamicSdkMemberTraits

	// The dynamically created instance of the properties struct containing the sdk member trait
	// list properties.
	dynamicMemberTraitListProperties interface{}

	// Information about the OsType specific member variants depended upon by this variant.
	//
	// Set by OsType specific variants in the collectMembers() method and used by the
	// CommonOS variant when building the snapshot. That work is all done on separate
	// calls to the sdk.GenerateAndroidBuildActions method which is guaranteed to be
	// called for the OsType specific variants before the CommonOS variant (because
	// the latter depends on the former).
	memberVariantDeps []sdkMemberVariantDep

	// The multilib variants that are used by this sdk variant.
	multilibUsages multilibUsage

	properties sdkProperties

	snapshotFile android.OptionalPath

	infoFile android.OptionalPath

	// The builder, preserved for testing.
	builderForTests *snapshotBuilder
}

type sdkProperties struct {
	Snapshot bool `blueprint:"mutated"`

	// True if this is a module_exports (or module_exports_snapshot) module type.
	Module_exports bool `blueprint:"mutated"`

	// The additional visibility to add to the prebuilt modules to allow them to
	// reference each other.
	//
	// This can only be used to widen the visibility of the members:
	//
	// * Specifying //visibility:public here will make all members visible and
	//   essentially ignore their own visibility.
	// * Specifying //visibility:private here is an error.
	// * Specifying any other rule here will add it to the members visibility and
	//   be output to the member prebuilt in the snapshot. Duplicates will be
	//   dropped. Adding a rule to members that have //visibility:private will
	//   cause the //visibility:private to be discarded.
	Prebuilt_visibility []string
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
	sdkMemberTypeKey, sdkMemberTypes := android.RegisteredSdkMemberTypes(moduleExports)
	s.dynamicSdkMemberTypes = getDynamicSdkMemberTypes(sdkMemberTypeKey, sdkMemberTypes)
	// Create an instance of the dynamically created struct that contains all the
	// properties for the member type specific list properties.
	s.dynamicMemberTypeListProperties = s.dynamicSdkMemberTypes.createMemberTypeListProperties()

	sdkMemberTraitsKey, sdkMemberTraits := android.RegisteredSdkMemberTraits()
	s.dynamicSdkMemberTraits = getDynamicSdkMemberTraits(sdkMemberTraitsKey, sdkMemberTraits)
	// Create an instance of the dynamically created struct that contains all the properties for the
	// member trait specific list properties.
	s.dynamicMemberTraitListProperties = s.dynamicSdkMemberTraits.createMemberTraitListProperties()

	// Create a wrapper around the dynamic trait specific properties so that they have to be
	// specified within a traits:{} section in the .bp file.
	traitsWrapper := struct {
		Traits interface{}
	}{s.dynamicMemberTraitListProperties}

	s.AddProperties(&s.properties, s.dynamicMemberTypeListProperties, &traitsWrapper)

	// Make sure that the prebuilt visibility property is verified for errors.
	android.AddVisibilityProperty(s, "prebuilt_visibility", &s.properties.Prebuilt_visibility)
	android.InitCommonOSAndroidMultiTargetsArchModule(s, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(s)
	android.AddLoadHook(s, func(ctx android.LoadHookContext) {
		type props struct {
			Compile_multilib *string
		}
		p := &props{Compile_multilib: proptools.StringPtr("both")}
		ctx.PrependProperties(p)
	})
	return s
}

// sdk_snapshot is a versioned snapshot of an SDK. This is an auto-generated module.
func SnapshotModuleFactory() android.Module {
	s := newSdkModule(false)
	s.properties.Snapshot = true
	return s
}

func (s *sdk) memberTypeListProperties() []*sdkMemberTypeListProperty {
	return s.dynamicSdkMemberTypes.memberTypeListProperties
}

func (s *sdk) memberTypeListProperty(memberType android.SdkMemberType) *sdkMemberTypeListProperty {
	return s.dynamicSdkMemberTypes.memberTypeToProperty[memberType]
}

// memberTraitListProperties returns the list of *sdkMemberTraitListProperty instances for this sdk.
func (s *sdk) memberTraitListProperties() []*sdkMemberTraitListProperty {
	return s.dynamicSdkMemberTraits.memberTraitListProperties
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
		s.buildSnapshot(ctx, sdkVariants)
	}
}

func (s *sdk) AndroidMkEntries() []android.AndroidMkEntries {
	if !s.snapshotFile.Valid() != !s.infoFile.Valid() {
		panic("Snapshot (%q) and info file (%q) should both be set or neither should be set.")
	} else if !s.snapshotFile.Valid() {
		return []android.AndroidMkEntries{}
	}

	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "FAKE",
		OutputFile: s.snapshotFile,
		DistFiles:  android.MakeDefaultDistFiles(s.snapshotFile.Path(), s.infoFile.Path()),
		Include:    "$(BUILD_PHONY_PACKAGE)",
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string) {
				// Allow the sdk to be built by simply passing its name on the command line.
				fmt.Fprintln(w, ".PHONY:", s.Name())
				fmt.Fprintln(w, s.Name()+":", s.snapshotFile.String())

				// Allow the sdk info to be built by simply passing its name on the command line.
				infoTarget := s.Name() + ".info"
				fmt.Fprintln(w, ".PHONY:", infoTarget)
				fmt.Fprintln(w, infoTarget+":", s.infoFile.String())
			},
		},
	}}
}

// gatherTraits gathers the traits from the dynamically generated trait specific properties.
//
// Returns a map from member name to the set of required traits.
func (s *sdk) gatherTraits() map[string]android.SdkMemberTraitSet {
	traitListByMember := map[string][]android.SdkMemberTrait{}
	for _, memberListProperty := range s.memberTraitListProperties() {
		names := memberListProperty.getter(s.dynamicMemberTraitListProperties)
		for _, name := range names {
			traitListByMember[name] = append(traitListByMember[name], memberListProperty.memberTrait)
		}
	}

	traitSetByMember := map[string]android.SdkMemberTraitSet{}
	for name, list := range traitListByMember {
		traitSetByMember[name] = android.NewSdkMemberTraitSet(list)
	}

	return traitSetByMember
}

// newDependencyContext creates a new SdkDependencyContext for this sdk.
func (s *sdk) newDependencyContext(mctx android.BottomUpMutatorContext) android.SdkDependencyContext {
	traits := s.gatherTraits()

	return &dependencyContext{
		BottomUpMutatorContext: mctx,
		requiredTraits:         traits,
	}
}

type dependencyContext struct {
	android.BottomUpMutatorContext

	// Map from member name to the set of traits that the sdk requires the member provides.
	requiredTraits map[string]android.SdkMemberTraitSet
}

func (d *dependencyContext) RequiredTraits(name string) android.SdkMemberTraitSet {
	if s, ok := d.requiredTraits[name]; ok {
		return s
	} else {
		return android.EmptySdkMemberTraitSet()
	}
}

func (d *dependencyContext) RequiresTrait(name string, trait android.SdkMemberTrait) bool {
	return d.RequiredTraits(name).Contains(trait)
}

var _ android.SdkDependencyContext = (*dependencyContext)(nil)

// RegisterPreDepsMutators registers pre-deps mutators to support modules implementing SdkAware
// interface and the sdk module type. This function has been made public to be called by tests
// outside of the sdk package
func RegisterPreDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.BottomUp("SdkMember", memberMutator).Parallel()
	ctx.TopDown("SdkMember_deps", memberDepsMutator).Parallel()
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
}

// Mark this tag so dependencies that use it are excluded from APEX contents.
func (t dependencyTag) ExcludeFromApexContents() {}

var _ android.ExcludeFromApexContentsTag = dependencyTag{}

// For dependencies from an in-development version of an SDK member to frozen versions of the same member
// e.g. libfoo -> libfoo.mysdk.11 and libfoo.mysdk.12
//
// The dependency represented by this tag requires that for every APEX variant created for the
// `from` module that an equivalent APEX variant is created for the 'to' module. This is because an
// APEX that requires a specific version of an sdk (via the `uses_sdks` property will replace
// dependencies on the unversioned sdk member with a dependency on the appropriate versioned sdk
// member. In order for that to work the versioned sdk member needs to have a variant for that APEX.
// As it is not known at the time that the APEX variants are created which specific APEX variants of
// a versioned sdk members will be required it is necessary for the versioned sdk members to have
// variants for any APEX that it could be used within.
//
// If the APEX selects a versioned sdk member then it will not have a dependency on the `from`
// module at all so any dependencies of that module will not affect the APEX. However, if the APEX
// selects the unversioned sdk member then it must exclude all the versioned sdk members. In no
// situation would this dependency cause the `to` module to be added to the APEX hence why this tag
// also excludes the `to` module from being added to the APEX contents.
type sdkMemberVersionedDepTag struct {
	dependencyTag
	member  string
	version string
}

func (t sdkMemberVersionedDepTag) AlwaysRequireApexVariant() bool {
	return true
}

// Mark this tag so dependencies that use it are excluded from visibility enforcement.
func (t sdkMemberVersionedDepTag) ExcludeFromVisibilityEnforcement() {}

var _ android.AlwaysRequireApexVariantTag = sdkMemberVersionedDepTag{}

// Step 1: create dependencies from an SDK module to its members.
func memberMutator(mctx android.BottomUpMutatorContext) {
	if s, ok := mctx.Module().(*sdk); ok {
		// Add dependencies from enabled and non CommonOS variants to the sdk member variants.
		if s.Enabled() && !s.IsCommonOSVariant() {
			ctx := s.newDependencyContext(mctx)
			for _, memberListProperty := range s.memberTypeListProperties() {
				if memberListProperty.getter == nil {
					continue
				}
				names := memberListProperty.getter(s.dynamicMemberTypeListProperties)
				if len(names) > 0 {
					memberType := memberListProperty.memberType

					// Verify that the member type supports the specified traits.
					supportedTraits := memberType.SupportedTraits()
					for _, name := range names {
						requiredTraits := ctx.RequiredTraits(name)
						unsupportedTraits := requiredTraits.Subtract(supportedTraits)
						if !unsupportedTraits.Empty() {
							ctx.ModuleErrorf("sdk member %q has traits %s that are unsupported by its member type %q", name, unsupportedTraits, memberType.SdkPropertyName())
						}
					}

					// Add dependencies using the appropriate tag.
					tag := memberListProperty.dependencyTag
					memberType.AddDependencies(ctx, tag, names)
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

// An interface that encapsulates all the functionality needed to manage the sdk dependencies.
//
// It is a mixture of apex and sdk module functionality.
type sdkAndApexModule interface {
	android.Module
	android.DepIsInSameApex
}
