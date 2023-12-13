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

package android

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// minApiLevelForSdkSnapshot provides access to the min_sdk_version for MinApiLevelForSdkSnapshot
type minApiLevelForSdkSnapshot interface {
	MinSdkVersion(ctx EarlyModuleContext) ApiLevel
}

// MinApiLevelForSdkSnapshot returns the ApiLevel of the min_sdk_version of the supplied module.
//
// If the module does not provide a min_sdk_version then it defaults to 1.
func MinApiLevelForSdkSnapshot(ctx EarlyModuleContext, module Module) ApiLevel {
	minApiLevel := NoneApiLevel
	if m, ok := module.(minApiLevelForSdkSnapshot); ok {
		minApiLevel = m.MinSdkVersion(ctx)
	}
	if minApiLevel == NoneApiLevel {
		// The default min API level is 1.
		minApiLevel = uncheckedFinalApiLevel(1)
	}
	return minApiLevel
}

// SnapshotBuilder provides support for generating the build rules which will build the snapshot.
type SnapshotBuilder interface {
	// CopyToSnapshot generates a rule that will copy the src to the dest (which is a snapshot
	// relative path) and add the dest to the zip.
	CopyToSnapshot(src Path, dest string)

	// EmptyFile returns the path to an empty file.
	//
	// This can be used by sdk member types that need to create an empty file in the snapshot, simply
	// pass the value returned from this to the CopyToSnapshot() method.
	EmptyFile() Path

	// UnzipToSnapshot generates a rule that will unzip the supplied zip into the snapshot relative
	// directory destDir.
	UnzipToSnapshot(zipPath Path, destDir string)

	// AddPrebuiltModule adds a new prebuilt module to the snapshot.
	//
	// It is intended to be called from SdkMemberType.AddPrebuiltModule which can add module type
	// specific properties that are not variant specific. The following properties will be
	// automatically populated before returning.
	//
	// * name
	// * sdk_member_name
	// * prefer
	//
	// Properties that are variant specific will be handled by SdkMemberProperties structure.
	//
	// Each module created by this method can be output to the generated Android.bp file in two
	// different forms, depending on the setting of the SOONG_SDK_SNAPSHOT_VERSION build property.
	// The two forms are:
	// 1. A versioned Soong module that is referenced from a corresponding similarly versioned
	//    snapshot module.
	// 2. An unversioned Soong module that.
	//
	// See sdk/update.go for more information.
	AddPrebuiltModule(member SdkMember, moduleType string) BpModule

	// SdkMemberReferencePropertyTag returns a property tag to use when adding a property to a
	// BpModule that contains references to other sdk members.
	//
	// Using this will ensure that the reference is correctly output for both versioned and
	// unversioned prebuilts in the snapshot.
	//
	// "required: true" means that the property must only contain references to other members of the
	// sdk. Passing a reference to a module that is not a member of the sdk will result in a build
	// error.
	//
	// "required: false" means that the property can contain references to modules that are either
	// members or not members of the sdk. If a reference is to a module that is a non member then the
	// reference is left unchanged, i.e. it is not transformed as references to members are.
	//
	// The handling of the member names is dependent on whether it is an internal or exported member.
	// An exported member is one whose name is specified in one of the member type specific
	// properties. An internal member is one that is added due to being a part of an exported (or
	// other internal) member and is not itself an exported member.
	//
	// Member names are handled as follows:
	// * When creating the unversioned form of the module the name is left unchecked unless the member
	//   is internal in which case it is transformed into an sdk specific name, i.e. by prefixing with
	//   the sdk name.
	//
	// * When creating the versioned form of the module the name is transformed into a versioned sdk
	//   specific name, i.e. by prefixing with the sdk name and suffixing with the version.
	//
	// e.g.
	// bpPropertySet.AddPropertyWithTag("libs", []string{"member1", "member2"}, builder.SdkMemberReferencePropertyTag(true))
	SdkMemberReferencePropertyTag(required bool) BpPropertyTag
}

// BpPropertyTag is a marker interface that can be associated with properties in a BpPropertySet to
// provide additional information which can be used to customize their behavior.
type BpPropertyTag interface{}

// BpPropertySet is a set of properties for use in a .bp file.
type BpPropertySet interface {
	// AddProperty adds a property.
	//
	// The value can be one of the following types:
	// * string
	// * array of the above
	// * bool
	// For these types it is an error if multiple properties with the same name
	// are added.
	//
	// * pointer to a struct
	// * BpPropertySet
	//
	// A pointer to a Blueprint-style property struct is first converted into a
	// BpPropertySet by traversing the fields and adding their values as
	// properties in a BpPropertySet. A field with a struct value is itself
	// converted into a BpPropertySet before adding.
	//
	// Adding a BpPropertySet is done as follows:
	// * If no property with the name exists then the BpPropertySet is added
	//   directly to this property. Care must be taken to ensure that it does not
	//   introduce a cycle.
	// * If a property exists with the name and the current value is a
	//   BpPropertySet then every property of the new BpPropertySet is added to
	//   the existing BpPropertySet.
	// * Otherwise, if a property exists with the name then it is an error.
	AddProperty(name string, value interface{})

	// AddPropertyWithTag adds a property with an associated property tag.
	AddPropertyWithTag(name string, value interface{}, tag BpPropertyTag)

	// AddPropertySet adds a property set with the specified name and returns it so that additional
	// properties can be added to it.
	AddPropertySet(name string) BpPropertySet

	// AddCommentForProperty adds a comment for the named property (or property set).
	AddCommentForProperty(name, text string)
}

// BpModule represents a module definition in a .bp file.
type BpModule interface {
	BpPropertySet

	// ModuleType returns the module type of the module
	ModuleType() string

	// Name returns the name of the module or "" if no name has been specified.
	Name() string
}

// BpPrintable is a marker interface that must be implemented by any struct that is added as a
// property value.
type BpPrintable interface {
	bpPrintable()
}

// BpPrintableBase must be embedded within any struct that is added as a
// property value.
type BpPrintableBase struct {
}

func (b BpPrintableBase) bpPrintable() {
}

var _ BpPrintable = BpPrintableBase{}

// sdkRegisterable defines the interface that must be implemented by objects that can be registered
// in an sdkRegistry.
type sdkRegisterable interface {
	// SdkPropertyName returns the name of the corresponding property on an sdk module.
	SdkPropertyName() string
}

// sdkRegistry provides support for registering and retrieving objects that define properties for
// use by sdk and module_exports module types.
type sdkRegistry struct {
	// The list of registered objects sorted by property name.
	list []sdkRegisterable
}

// copyAndAppend creates a new sdkRegistry that includes all the traits registered in
// this registry plus the supplied trait.
func (r *sdkRegistry) copyAndAppend(registerable sdkRegisterable) *sdkRegistry {
	oldList := r.list

	// Make sure that list does not already contain the property. Uses a simple linear search instead
	// of a binary search even though the list is sorted. That is because the number of items in the
	// list is small and so not worth the overhead of a binary search.
	found := false
	newPropertyName := registerable.SdkPropertyName()
	for _, r := range oldList {
		if r.SdkPropertyName() == newPropertyName {
			found = true
			break
		}
	}
	if found {
		names := []string{}
		for _, r := range oldList {
			names = append(names, r.SdkPropertyName())
		}
		panic(fmt.Errorf("duplicate properties found, %q already exists in %q", newPropertyName, names))
	}

	// Copy the slice just in case this is being read while being modified, e.g. when testing.
	list := make([]sdkRegisterable, 0, len(oldList)+1)
	list = append(list, oldList...)
	list = append(list, registerable)

	// Sort the registered objects by their property name to ensure that registry order has no effect
	// on behavior.
	sort.Slice(list, func(i1, i2 int) bool {
		t1 := list[i1]
		t2 := list[i2]

		return t1.SdkPropertyName() < t2.SdkPropertyName()
	})

	// Create a new registry so the pointer uniquely identifies the set of registered types.
	return &sdkRegistry{
		list: list,
	}
}

// registeredObjects returns the list of registered instances.
func (r *sdkRegistry) registeredObjects() []sdkRegisterable {
	return r.list
}

// uniqueOnceKey returns a key that uniquely identifies this instance and can be used with
// OncePer.Once
func (r *sdkRegistry) uniqueOnceKey() OnceKey {
	// Use the pointer to the registry as the unique key. The pointer is used because it is guaranteed
	// to uniquely identify the contained list. The list itself cannot be used as slices are not
	// comparable. Using the pointer does mean that two separate registries with identical lists would
	// have different keys and so cause whatever information is cached to be created multiple times.
	// However, that is not an issue in practice as it should not occur outside tests. Constructing a
	// string representation of the list to use instead would avoid that but is an unnecessary
	// complication that provides no significant benefit.
	return NewCustomOnceKey(r)
}

// SdkMemberTrait represents a trait that members of an sdk module can contribute to the sdk
// snapshot.
//
// A trait is simply a characteristic of sdk member that is not required by default which may be
// required for some members but not others. Traits can cause additional information to be output
// to the sdk snapshot or replace the default information exported for a member with something else.
// e.g.
//   - By default cc libraries only export the default image variants to the SDK. However, for some
//     members it may be necessary to export specific image variants, e.g. vendor, or recovery.
//   - By default cc libraries export all the configured architecture variants except for the native
//     bridge architecture variants. However, for some members it may be necessary to export the
//     native bridge architecture variants as well.
//   - By default cc libraries export the platform variant (i.e. sdk:). However, for some members it
//     may be necessary to export the sdk variant (i.e. sdk:sdk).
//
// A sdk can request a module to provide no traits, one trait or a collection of traits. The exact
// behavior of a trait is determined by how SdkMemberType implementations handle the traits. A trait
// could be specific to one SdkMemberType or many. Some trait combinations could be incompatible.
//
// The sdk module type will create a special traits structure that contains a property for each
// trait registered with RegisterSdkMemberTrait(). The property names are those returned from
// SdkPropertyName(). Each property contains a list of modules that are required to have that trait.
// e.g. something like this:
//
//	sdk {
//	  name: "sdk",
//	  ...
//	  traits: {
//	    recovery_image: ["module1", "module4", "module5"],
//	    native_bridge: ["module1", "module2"],
//	    native_sdk: ["module1", "module3"],
//	    ...
//	  },
//	  ...
//	}
type SdkMemberTrait interface {
	// SdkPropertyName returns the name of the traits property on an sdk module.
	SdkPropertyName() string
}

var _ sdkRegisterable = (SdkMemberTrait)(nil)

// SdkMemberTraitBase is the base struct that must be embedded within any type that implements
// SdkMemberTrait.
type SdkMemberTraitBase struct {
	// PropertyName is the name of the property
	PropertyName string
}

func (b *SdkMemberTraitBase) SdkPropertyName() string {
	return b.PropertyName
}

// SdkMemberTraitSet is a set of SdkMemberTrait instances.
type SdkMemberTraitSet interface {
	// Empty returns true if this set is empty.
	Empty() bool

	// Contains returns true if this set contains the specified trait.
	Contains(trait SdkMemberTrait) bool

	// Subtract returns a new set containing all elements of this set except for those in the
	// other set.
	Subtract(other SdkMemberTraitSet) SdkMemberTraitSet

	// String returns a string representation of the set and its contents.
	String() string
}

func NewSdkMemberTraitSet(traits []SdkMemberTrait) SdkMemberTraitSet {
	if len(traits) == 0 {
		return EmptySdkMemberTraitSet()
	}

	m := sdkMemberTraitSet{}
	for _, trait := range traits {
		m[trait] = true
	}
	return m
}

func EmptySdkMemberTraitSet() SdkMemberTraitSet {
	return (sdkMemberTraitSet)(nil)
}

type sdkMemberTraitSet map[SdkMemberTrait]bool

var _ SdkMemberTraitSet = (sdkMemberTraitSet{})

func (s sdkMemberTraitSet) Empty() bool {
	return len(s) == 0
}

func (s sdkMemberTraitSet) Contains(trait SdkMemberTrait) bool {
	return s[trait]
}

func (s sdkMemberTraitSet) Subtract(other SdkMemberTraitSet) SdkMemberTraitSet {
	if other.Empty() {
		return s
	}

	var remainder []SdkMemberTrait
	for trait, _ := range s {
		if !other.Contains(trait) {
			remainder = append(remainder, trait)
		}
	}

	return NewSdkMemberTraitSet(remainder)
}

func (s sdkMemberTraitSet) String() string {
	list := []string{}
	for trait, _ := range s {
		list = append(list, trait.SdkPropertyName())
	}
	sort.Strings(list)
	return fmt.Sprintf("[%s]", strings.Join(list, ","))
}

var registeredSdkMemberTraits = &sdkRegistry{}

// RegisteredSdkMemberTraits returns a OnceKey and a sorted list of registered traits.
//
// The key uniquely identifies the array of traits and can be used with OncePer.Once() to cache
// information derived from the array of traits.
func RegisteredSdkMemberTraits() (OnceKey, []SdkMemberTrait) {
	registerables := registeredSdkMemberTraits.registeredObjects()
	traits := make([]SdkMemberTrait, len(registerables))
	for i, registerable := range registerables {
		traits[i] = registerable.(SdkMemberTrait)
	}
	return registeredSdkMemberTraits.uniqueOnceKey(), traits
}

// RegisterSdkMemberTrait registers an SdkMemberTrait object to allow them to be used in the
// module_exports, module_exports_snapshot, sdk and sdk_snapshot module types.
func RegisterSdkMemberTrait(trait SdkMemberTrait) {
	registeredSdkMemberTraits = registeredSdkMemberTraits.copyAndAppend(trait)
}

// SdkMember is an individual member of the SDK.
//
// It includes all of the variants that the SDK depends upon.
type SdkMember interface {
	// Name returns the name of the member.
	Name() string

	// Variants returns all the variants of this module depended upon by the SDK.
	Variants() []Module
}

// SdkMemberDependencyTag is the interface that a tag must implement in order to allow the
// dependent module to be automatically added to the sdk.
type SdkMemberDependencyTag interface {
	blueprint.DependencyTag

	// SdkMemberType returns the SdkMemberType that will be used to automatically add the child module
	// to the sdk.
	//
	// Returning nil will prevent the module being added to the sdk.
	SdkMemberType(child Module) SdkMemberType

	// ExportMember determines whether a module added to the sdk through this tag will be exported
	// from the sdk or not.
	//
	// An exported member is added to the sdk using its own name, e.g. if "foo" was exported from sdk
	// "bar" then its prebuilt would be simply called "foo". A member can be added to the sdk via
	// multiple tags and if any of those tags returns true from this method then the membe will be
	// exported. Every module added directly to the sdk via one of the member type specific
	// properties, e.g. java_libs, will automatically be exported.
	//
	// If a member is not exported then it is treated as an internal implementation detail of the
	// sdk and so will be added with an sdk specific name. e.g. if "foo" was an internal member of sdk
	// "bar" then its prebuilt would be called "bar_foo". Additionally its visibility will be set to
	// "//visibility:private" so it will not be accessible from outside its Android.bp file.
	ExportMember() bool
}

var _ SdkMemberDependencyTag = (*sdkMemberDependencyTag)(nil)
var _ ReplaceSourceWithPrebuilt = (*sdkMemberDependencyTag)(nil)

type sdkMemberDependencyTag struct {
	blueprint.BaseDependencyTag
	memberType SdkMemberType
	export     bool
}

func (t *sdkMemberDependencyTag) SdkMemberType(_ Module) SdkMemberType {
	return t.memberType
}

func (t *sdkMemberDependencyTag) ExportMember() bool {
	return t.export
}

// ReplaceSourceWithPrebuilt prevents dependencies from the sdk/module_exports onto their members
// from being replaced with a preferred prebuilt.
func (t *sdkMemberDependencyTag) ReplaceSourceWithPrebuilt() bool {
	return false
}

// DependencyTagForSdkMemberType creates an SdkMemberDependencyTag that will cause any
// dependencies added by the tag to be added to the sdk as the specified SdkMemberType and exported
// (or not) as specified by the export parameter.
func DependencyTagForSdkMemberType(memberType SdkMemberType, export bool) SdkMemberDependencyTag {
	return &sdkMemberDependencyTag{memberType: memberType, export: export}
}

// SdkMemberType is the interface that must be implemented for every type that can be a member of an
// sdk.
//
// The basic implementation should look something like this, where ModuleType is
// the name of the module type being supported.
//
//	type moduleTypeSdkMemberType struct {
//	    android.SdkMemberTypeBase
//	}
//
//	func init() {
//	    android.RegisterSdkMemberType(&moduleTypeSdkMemberType{
//	        SdkMemberTypeBase: android.SdkMemberTypeBase{
//	            PropertyName: "module_types",
//	        },
//	    }
//	}
//
//	...methods...
type SdkMemberType interface {
	// SdkPropertyName returns the name of the member type property on an sdk module.
	SdkPropertyName() string

	// RequiresBpProperty returns true if this member type requires its property to be usable within
	// an Android.bp file.
	RequiresBpProperty() bool

	// SupportedBuildReleases returns the string representation of a set of target build releases that
	// support this member type.
	SupportedBuildReleases() string

	// UsableWithSdkAndSdkSnapshot returns true if the member type supports the sdk/sdk_snapshot,
	// false otherwise.
	UsableWithSdkAndSdkSnapshot() bool

	// IsHostOsDependent returns true if prebuilt host artifacts may be specific to the host OS. Only
	// applicable to modules where HostSupported() is true. If this is true, snapshots will list each
	// host OS variant explicitly and disable all other host OS'es.
	IsHostOsDependent() bool

	// SupportedLinkages returns the names of the linkage variants supported by this module.
	SupportedLinkages() []string

	// ArePrebuiltsRequired returns true if prebuilts are required in the sdk snapshot, false
	// otherwise.
	ArePrebuiltsRequired() bool

	// AddDependencies adds dependencies from the SDK module to all the module variants the member
	// type contributes to the SDK. `names` is the list of module names given in the member type
	// property (as returned by SdkPropertyName()) in the SDK module. The exact set of variants
	// required is determined by the SDK and its properties. The dependencies must be added with the
	// supplied tag.
	//
	// The BottomUpMutatorContext provided is for the SDK module.
	AddDependencies(ctx SdkDependencyContext, dependencyTag blueprint.DependencyTag, names []string)

	// IsInstance returns true if the supplied module is an instance of this member type.
	//
	// This is used to check the type of each variant before added to the SdkMember. Returning false
	// will cause an error to be logged explaining that the module is not allowed in whichever sdk
	// property it was added.
	IsInstance(module Module) bool

	// UsesSourceModuleTypeInSnapshot returns true when the AddPrebuiltModule() method returns a
	// source module type.
	UsesSourceModuleTypeInSnapshot() bool

	// AddPrebuiltModule is called to add a prebuilt module that the sdk will populate.
	//
	// The sdk module code generates the snapshot as follows:
	//
	// * A properties struct of type SdkMemberProperties is created for each variant and
	//   populated with information from the variant by calling PopulateFromVariant(Module)
	//   on the struct.
	//
	// * An additional properties struct is created into which the common properties will be
	//   added.
	//
	// * The variant property structs are analysed to find exported (capitalized) fields which
	//   have common values. Those fields are cleared and the common value added to the common
	//   properties.
	//
	//   A field annotated with a tag of `sdk:"ignore"` will be treated as if it
	//   was not capitalized, i.e. ignored and not optimized for common values.
	//
	//   A field annotated with a tag of `sdk:"keep"` will not be cleared even if the value is common
	//   across multiple structs. Common values will still be copied into the common property struct.
	//   So, if the same value is placed in all structs populated from variants that value would be
	//   copied into all common property structs and so be available in every instance.
	//
	//   A field annotated with a tag of `android:"arch_variant"` will be allowed to have
	//   values that differ by arch, fields not tagged as such must have common values across
	//   all variants.
	//
	// * Additional field tags can be specified on a field that will ignore certain values
	//   for the purpose of common value optimization. A value that is ignored must have the
	//   default value for the property type. This is to ensure that significant value are not
	//   ignored by accident. The purpose of this is to allow the snapshot generation to reflect
	//   the behavior of the runtime. e.g. if a property is ignored on the host then a property
	//   that is common for android can be treated as if it was common for android and host as
	//   the setting for host is ignored anyway.
	//   * `sdk:"ignored-on-host" - this indicates the property is ignored on the host variant.
	//
	// * The sdk module type populates the BpModule structure, creating the arch specific
	//   structure and calls AddToPropertySet(...) on the properties struct to add the member
	//   specific properties in the correct place in the structure.
	//
	AddPrebuiltModule(ctx SdkMemberContext, member SdkMember) BpModule

	// CreateVariantPropertiesStruct creates a structure into which variant specific properties can be
	// added.
	CreateVariantPropertiesStruct() SdkMemberProperties

	// SupportedTraits returns the set of traits supported by this member type.
	SupportedTraits() SdkMemberTraitSet

	// Overrides returns whether type overrides other SdkMemberType
	Overrides(SdkMemberType) bool
}

var _ sdkRegisterable = (SdkMemberType)(nil)

// SdkDependencyContext provides access to information needed by the SdkMemberType.AddDependencies()
// implementations.
type SdkDependencyContext interface {
	BottomUpMutatorContext

	// RequiredTraits returns the set of SdkMemberTrait instances that the sdk requires the named
	// member to provide.
	RequiredTraits(name string) SdkMemberTraitSet

	// RequiresTrait returns true if the sdk requires the member with the supplied name to provide the
	// supplied trait.
	RequiresTrait(name string, trait SdkMemberTrait) bool
}

// SdkMemberTypeBase is the base type for SdkMemberType implementations and must be embedded in any
// struct that implements SdkMemberType.
type SdkMemberTypeBase struct {
	PropertyName string

	// Property names that this SdkMemberTypeBase can override, this is useful when a module type is a
	// superset of another module type.
	OverridesPropertyNames map[string]bool

	// The names of linkage variants supported by this module.
	SupportedLinkageNames []string

	// When set to true BpPropertyNotRequired indicates that the member type does not require the
	// property to be specifiable in an Android.bp file.
	BpPropertyNotRequired bool

	// The name of the first targeted build release.
	//
	// If not specified then it is assumed to be available on all targeted build releases.
	SupportedBuildReleaseSpecification string

	// Set to true if this must be usable with the sdk/sdk_snapshot module types. Otherwise, it will
	// only be usable with module_exports/module_exports_snapshots module types.
	SupportsSdk bool

	// Set to true if prebuilt host artifacts of this member may be specific to the host OS. Only
	// applicable to modules where HostSupported() is true.
	HostOsDependent bool

	// When set to true UseSourceModuleTypeInSnapshot indicates that the member type creates a source
	// module type in its SdkMemberType.AddPrebuiltModule() method. That prevents the sdk snapshot
	// code from automatically adding a prefer: true flag.
	UseSourceModuleTypeInSnapshot bool

	// Set to proptools.BoolPtr(false) if this member does not generate prebuilts but is only provided
	// to allow the sdk to gather members from this member's dependencies. If not specified then
	// defaults to true.
	PrebuiltsRequired *bool

	// The list of supported traits.
	Traits []SdkMemberTrait
}

func (b *SdkMemberTypeBase) SdkPropertyName() string {
	return b.PropertyName
}

func (b *SdkMemberTypeBase) RequiresBpProperty() bool {
	return !b.BpPropertyNotRequired
}

func (b *SdkMemberTypeBase) SupportedBuildReleases() string {
	return b.SupportedBuildReleaseSpecification
}

func (b *SdkMemberTypeBase) UsableWithSdkAndSdkSnapshot() bool {
	return b.SupportsSdk
}

func (b *SdkMemberTypeBase) IsHostOsDependent() bool {
	return b.HostOsDependent
}

func (b *SdkMemberTypeBase) ArePrebuiltsRequired() bool {
	return proptools.BoolDefault(b.PrebuiltsRequired, true)
}

func (b *SdkMemberTypeBase) UsesSourceModuleTypeInSnapshot() bool {
	return b.UseSourceModuleTypeInSnapshot
}

func (b *SdkMemberTypeBase) SupportedTraits() SdkMemberTraitSet {
	return NewSdkMemberTraitSet(b.Traits)
}

func (b *SdkMemberTypeBase) Overrides(other SdkMemberType) bool {
	return b.OverridesPropertyNames[other.SdkPropertyName()]
}

func (b *SdkMemberTypeBase) SupportedLinkages() []string {
	return b.SupportedLinkageNames
}

// registeredModuleExportsMemberTypes is the set of registered SdkMemberTypes for module_exports
// modules.
var registeredModuleExportsMemberTypes = &sdkRegistry{}

// registeredSdkMemberTypes is the set of registered registeredSdkMemberTypes for sdk modules.
var registeredSdkMemberTypes = &sdkRegistry{}

// RegisteredSdkMemberTypes returns a OnceKey and a sorted list of registered types.
//
// If moduleExports is true then the slice of types includes all registered types that can be used
// with the module_exports and module_exports_snapshot module types. Otherwise, the slice of types
// only includes those registered types that can be used with the sdk and sdk_snapshot module
// types.
//
// The key uniquely identifies the array of types and can be used with OncePer.Once() to cache
// information derived from the array of types.
func RegisteredSdkMemberTypes(moduleExports bool) (OnceKey, []SdkMemberType) {
	var registry *sdkRegistry
	if moduleExports {
		registry = registeredModuleExportsMemberTypes
	} else {
		registry = registeredSdkMemberTypes
	}

	registerables := registry.registeredObjects()
	types := make([]SdkMemberType, len(registerables))
	for i, registerable := range registerables {
		types[i] = registerable.(SdkMemberType)
	}
	return registry.uniqueOnceKey(), types
}

// RegisterSdkMemberType registers an SdkMemberType object to allow them to be used in the
// module_exports, module_exports_snapshot and (depending on the value returned from
// SdkMemberType.UsableWithSdkAndSdkSnapshot) the sdk and sdk_snapshot module types.
func RegisterSdkMemberType(memberType SdkMemberType) {
	// All member types are usable with module_exports.
	registeredModuleExportsMemberTypes = registeredModuleExportsMemberTypes.copyAndAppend(memberType)

	// Only those that explicitly indicate it are usable with sdk.
	if memberType.UsableWithSdkAndSdkSnapshot() {
		registeredSdkMemberTypes = registeredSdkMemberTypes.copyAndAppend(memberType)
	}
}

// SdkMemberPropertiesBase is the base structure for all implementations of SdkMemberProperties and
// must be embedded in any struct that implements SdkMemberProperties.
//
// Contains common properties that apply across many different member types.
type SdkMemberPropertiesBase struct {
	// The number of unique os types supported by the member variants.
	//
	// If a member has a variant with more than one os type then it will need to differentiate
	// the locations of any of their prebuilt files in the snapshot by os type to prevent them
	// from colliding. See OsPrefix().
	//
	// Ignore this property during optimization. This is needed because this property is the same for
	// all variants of a member and so would be optimized away if it was not ignored.
	Os_count int `sdk:"ignore"`

	// The os type for which these properties refer.
	//
	// Provided to allow a member to differentiate between os types in the locations of their
	// prebuilt files when it supports more than one os type.
	//
	// Ignore this property during optimization. This is needed because this property is the same for
	// all variants of a member and so would be optimized away if it was not ignored.
	Os OsType `sdk:"ignore"`

	// The setting to use for the compile_multilib property.
	Compile_multilib string `android:"arch_variant"`
}

// OsPrefix returns the os prefix to use for any file paths in the sdk.
//
// Is an empty string if the member only provides variants for a single os type, otherwise
// is the OsType.Name.
func (b *SdkMemberPropertiesBase) OsPrefix() string {
	if b.Os_count == 1 {
		return ""
	} else {
		return b.Os.Name
	}
}

func (b *SdkMemberPropertiesBase) Base() *SdkMemberPropertiesBase {
	return b
}

// SdkMemberProperties is the interface to be implemented on top of a structure that contains
// variant specific information.
//
// Struct fields that are capitalized are examined for common values to extract. Fields that are not
// capitalized are assumed to be arch specific.
type SdkMemberProperties interface {
	// Base returns the base structure.
	Base() *SdkMemberPropertiesBase

	// PopulateFromVariant populates this structure with information from a module variant.
	//
	// It will typically be called once for each variant of a member module that the SDK depends upon.
	PopulateFromVariant(ctx SdkMemberContext, variant Module)

	// AddToPropertySet adds the information from this structure to the property set.
	//
	// This will be called for each instance of this structure on which the PopulateFromVariant method
	// was called and also on a number of different instances of this structure into which properties
	// common to one or more variants have been copied. Therefore, implementations of this must handle
	// the case when this structure is only partially populated.
	AddToPropertySet(ctx SdkMemberContext, propertySet BpPropertySet)
}

// SdkMemberContext provides access to information common to a specific member.
type SdkMemberContext interface {

	// SdkModuleContext returns the module context of the sdk common os variant which is creating the
	// snapshot.
	//
	// This is common to all members of the sdk and is not specific to the member being processed.
	// If information about the member being processed needs to be obtained from this ModuleContext it
	// must be obtained using one of the OtherModule... methods not the Module... methods.
	SdkModuleContext() ModuleContext

	// SnapshotBuilder the builder of the snapshot.
	SnapshotBuilder() SnapshotBuilder

	// MemberType returns the type of the member currently being processed.
	MemberType() SdkMemberType

	// Name returns the name of the member currently being processed.
	//
	// Provided for use by sdk members to create a member specific location within the snapshot
	// into which to copy the prebuilt files.
	Name() string

	// RequiresTrait returns true if this member is expected to provide the specified trait.
	RequiresTrait(trait SdkMemberTrait) bool

	// IsTargetBuildBeforeTiramisu return true if the target build release for which this snapshot is
	// being generated is before Tiramisu, i.e. S.
	IsTargetBuildBeforeTiramisu() bool

	// ModuleErrorf reports an error at the line number of the module type in the module definition.
	ModuleErrorf(fmt string, args ...interface{})
}

// ExportedComponentsInfo contains information about the components that this module exports to an
// sdk snapshot.
//
// A component of a module is a child module that the module creates and which forms an integral
// part of the functionality that the creating module provides. A component module is essentially
// owned by its creator and is tightly coupled to the creator and other components.
//
// e.g. the child modules created by prebuilt_apis are not components because they are not tightly
// coupled to the prebuilt_apis module. Once they are created the prebuilt_apis ignores them. The
// child impl and stub library created by java_sdk_library (and corresponding import) are components
// because the creating module depends upon them in order to provide some of its own functionality.
//
// A component is exported if it is part of an sdk snapshot. e.g. The xml and impl child modules are
// components but they are not exported as they are not part of an sdk snapshot.
//
// This information is used by the sdk snapshot generation code to ensure that it does not create
// an sdk snapshot that contains a declaration of the component module and the module that creates
// it as that would result in duplicate modules when attempting to use the snapshot. e.g. a snapshot
// that included the java_sdk_library_import "foo" and also a java_import "foo.stubs" would fail
// as there would be two modules called "foo.stubs".
type ExportedComponentsInfo struct {
	// The names of the exported components.
	Components []string
}

var ExportedComponentsInfoProvider = blueprint.NewProvider[ExportedComponentsInfo]()

// AdditionalSdkInfo contains additional properties to add to the generated SDK info file.
type AdditionalSdkInfo struct {
	Properties map[string]interface{}
}

var AdditionalSdkInfoProvider = blueprint.NewProvider[AdditionalSdkInfo]()
