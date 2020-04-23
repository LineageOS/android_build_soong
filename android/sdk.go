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
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// Extracted from SdkAware to make it easier to define custom subsets of the
// SdkAware interface and improve code navigation within the IDE.
//
// In addition to its use in SdkAware this interface must also be implemented by
// APEX to specify the SDKs required by that module and its contents. e.g. APEX
// is expected to implement RequiredSdks() by reading its own properties like
// `uses_sdks`.
type RequiredSdks interface {
	// The set of SDKs required by an APEX and its contents.
	RequiredSdks() SdkRefs
}

// SdkAware is the interface that must be supported by any module to become a member of SDK or to be
// built with SDK
type SdkAware interface {
	Module
	RequiredSdks

	sdkBase() *SdkBase
	MakeMemberOf(sdk SdkRef)
	IsInAnySdk() bool
	ContainingSdk() SdkRef
	MemberName() string
	BuildWithSdks(sdks SdkRefs)
}

// SdkRef refers to a version of an SDK
type SdkRef struct {
	Name    string
	Version string
}

// Unversioned determines if the SdkRef is referencing to the unversioned SDK module
func (s SdkRef) Unversioned() bool {
	return s.Version == ""
}

// String returns string representation of this SdkRef for debugging purpose
func (s SdkRef) String() string {
	if s.Name == "" {
		return "(No Sdk)"
	}
	if s.Unversioned() {
		return s.Name
	}
	return s.Name + string(SdkVersionSeparator) + s.Version
}

// SdkVersionSeparator is a character used to separate an sdk name and its version
const SdkVersionSeparator = '@'

// ParseSdkRef parses a `name@version` style string into a corresponding SdkRef struct
func ParseSdkRef(ctx BaseModuleContext, str string, property string) SdkRef {
	tokens := strings.Split(str, string(SdkVersionSeparator))
	if len(tokens) < 1 || len(tokens) > 2 {
		ctx.PropertyErrorf(property, "%q does not follow name#version syntax", str)
		return SdkRef{Name: "invalid sdk name", Version: "invalid sdk version"}
	}

	name := tokens[0]

	var version string
	if len(tokens) == 2 {
		version = tokens[1]
	}

	return SdkRef{Name: name, Version: version}
}

type SdkRefs []SdkRef

// Contains tells if the given SdkRef is in this list of SdkRef's
func (refs SdkRefs) Contains(s SdkRef) bool {
	for _, r := range refs {
		if r == s {
			return true
		}
	}
	return false
}

type sdkProperties struct {
	// The SDK that this module is a member of. nil if it is not a member of any SDK
	ContainingSdk *SdkRef `blueprint:"mutated"`

	// The list of SDK names and versions that are used to build this module
	RequiredSdks SdkRefs `blueprint:"mutated"`

	// Name of the module that this sdk member is representing
	Sdk_member_name *string
}

// SdkBase is a struct that is expected to be included in module types to implement the SdkAware
// interface. InitSdkAwareModule should be called to initialize this struct.
type SdkBase struct {
	properties sdkProperties
	module     SdkAware
}

func (s *SdkBase) sdkBase() *SdkBase {
	return s
}

// MakeMemberOf sets this module to be a member of a specific SDK
func (s *SdkBase) MakeMemberOf(sdk SdkRef) {
	s.properties.ContainingSdk = &sdk
}

// IsInAnySdk returns true if this module is a member of any SDK
func (s *SdkBase) IsInAnySdk() bool {
	return s.properties.ContainingSdk != nil
}

// ContainingSdk returns the SDK that this module is a member of
func (s *SdkBase) ContainingSdk() SdkRef {
	if s.properties.ContainingSdk != nil {
		return *s.properties.ContainingSdk
	}
	return SdkRef{Name: "", Version: ""}
}

// MemberName returns the name of the module that this SDK member is overriding
func (s *SdkBase) MemberName() string {
	return proptools.String(s.properties.Sdk_member_name)
}

// BuildWithSdks is used to mark that this module has to be built with the given SDK(s).
func (s *SdkBase) BuildWithSdks(sdks SdkRefs) {
	s.properties.RequiredSdks = sdks
}

// RequiredSdks returns the SDK(s) that this module has to be built with
func (s *SdkBase) RequiredSdks() SdkRefs {
	return s.properties.RequiredSdks
}

// InitSdkAwareModule initializes the SdkBase struct. This must be called by all modules including
// SdkBase.
func InitSdkAwareModule(m SdkAware) {
	base := m.sdkBase()
	base.module = m
	m.AddProperties(&base.properties)
}

// Provide support for generating the build rules which will build the snapshot.
type SnapshotBuilder interface {
	// Copy src to the dest (which is a snapshot relative path) and add the dest
	// to the zip
	CopyToSnapshot(src Path, dest string)

	// Unzip the supplied zip into the snapshot relative directory destDir.
	UnzipToSnapshot(zipPath Path, destDir string)

	// Add a new prebuilt module to the snapshot. The returned module
	// must be populated with the module type specific properties. The following
	// properties will be automatically populated.
	//
	// * name
	// * sdk_member_name
	// * prefer
	//
	// This will result in two Soong modules being generated in the Android. One
	// that is versioned, coupled to the snapshot version and marked as
	// prefer=true. And one that is not versioned, not marked as prefer=true and
	// will only be used if the equivalently named non-prebuilt module is not
	// present.
	AddPrebuiltModule(member SdkMember, moduleType string) BpModule

	// The property tag to use when adding a property to a BpModule that contains
	// references to other sdk members. Using this will ensure that the reference
	// is correctly output for both versioned and unversioned prebuilts in the
	// snapshot.
	//
	// "required: true" means that the property must only contain references
	// to other members of the sdk. Passing a reference to a module that is not a
	// member of the sdk will result in a build error.
	//
	// "required: false" means that the property can contain references to modules
	// that are either members or not members of the sdk. If a reference is to a
	// module that is a non member then the reference is left unchanged, i.e. it
	// is not transformed as references to members are.
	//
	// The handling of the member names is dependent on whether it is an internal or
	// exported member. An exported member is one whose name is specified in one of
	// the member type specific properties. An internal member is one that is added
	// due to being a part of an exported (or other internal) member and is not itself
	// an exported member.
	//
	// Member names are handled as follows:
	// * When creating the unversioned form of the module the name is left unchecked
	//   unless the member is internal in which case it is transformed into an sdk
	//   specific name, i.e. by prefixing with the sdk name.
	//
	// * When creating the versioned form of the module the name is transformed into
	//   a versioned sdk specific name, i.e. by prefixing with the sdk name and
	//   suffixing with the version.
	//
	// e.g.
	// bpPropertySet.AddPropertyWithTag("libs", []string{"member1", "member2"}, builder.SdkMemberReferencePropertyTag(true))
	SdkMemberReferencePropertyTag(required bool) BpPropertyTag
}

type BpPropertyTag interface{}

// A set of properties for use in a .bp file.
type BpPropertySet interface {
	// Add a property, the value can be one of the following types:
	// * string
	// * array of the above
	// * bool
	// * BpPropertySet
	//
	// It is an error if multiple properties with the same name are added.
	AddProperty(name string, value interface{})

	// Add a property with an associated tag
	AddPropertyWithTag(name string, value interface{}, tag BpPropertyTag)

	// Add a property set with the specified name and return so that additional
	// properties can be added.
	AddPropertySet(name string) BpPropertySet
}

// A .bp module definition.
type BpModule interface {
	BpPropertySet
}

// An individual member of the SDK, includes all of the variants that the SDK
// requires.
type SdkMember interface {
	// The name of the member.
	Name() string

	// All the variants required by the SDK.
	Variants() []SdkAware
}

type SdkMemberTypeDependencyTag interface {
	blueprint.DependencyTag

	SdkMemberType() SdkMemberType
}

type sdkMemberDependencyTag struct {
	blueprint.BaseDependencyTag
	memberType SdkMemberType
}

func (t *sdkMemberDependencyTag) SdkMemberType() SdkMemberType {
	return t.memberType
}

func DependencyTagForSdkMemberType(memberType SdkMemberType) SdkMemberTypeDependencyTag {
	return &sdkMemberDependencyTag{memberType: memberType}
}

// Interface that must be implemented for every type that can be a member of an
// sdk.
//
// The basic implementation should look something like this, where ModuleType is
// the name of the module type being supported.
//
//    type moduleTypeSdkMemberType struct {
//        android.SdkMemberTypeBase
//    }
//
//    func init() {
//        android.RegisterSdkMemberType(&moduleTypeSdkMemberType{
//            SdkMemberTypeBase: android.SdkMemberTypeBase{
//                PropertyName: "module_types",
//            },
//        }
//    }
//
//    ...methods...
//
type SdkMemberType interface {
	// The name of the member type property on an sdk module.
	SdkPropertyName() string

	// True if the member type supports the sdk/sdk_snapshot, false otherwise.
	UsableWithSdkAndSdkSnapshot() bool

	// Return true if modules of this type can have dependencies which should be
	// treated as if they are sdk members.
	//
	// Any dependency that is to be treated as a member of the sdk needs to implement
	// SdkAware and be added with an SdkMemberTypeDependencyTag tag.
	HasTransitiveSdkMembers() bool

	// Add dependencies from the SDK module to all the variants the member
	// contributes to the SDK. The exact set of variants required is determined
	// by the SDK and its properties. The dependencies must be added with the
	// supplied tag.
	//
	// The BottomUpMutatorContext provided is for the SDK module.
	AddDependencies(mctx BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string)

	// Return true if the supplied module is an instance of this member type.
	//
	// This is used to check the type of each variant before added to the
	// SdkMember. Returning false will cause an error to be logged expaining that
	// the module is not allowed in whichever sdk property it was added.
	IsInstance(module Module) bool

	// Add a prebuilt module that the sdk will populate.
	//
	// Returning nil from this will cause the sdk module type to use the deprecated BuildSnapshot
	// method to build the snapshot. That method is deprecated because it requires the SdkMemberType
	// implementation to do all the word.
	//
	// Otherwise, returning a non-nil value from this will cause the sdk module type to do the
	// majority of the work to generate the snapshot. The sdk module code generates the snapshot
	// as follows:
	//
	// * A properties struct of type SdkMemberProperties is created for each variant and
	//   populated with information from the variant by calling PopulateFromVariant(SdkAware)
	//   on the struct.
	//
	// * An additional properties struct is created into which the common properties will be
	//   added.
	//
	// * The variant property structs are analysed to find exported (capitalized) fields which
	//   have common values. Those fields are cleared and the common value added to the common
	//   properties. A field annotated with a tag of `sdk:"keep"` will be treated as if it
	//   was not capitalized, i.e. not optimized for common values.
	//
	// * The sdk module type populates the BpModule structure, creating the arch specific
	//   structure and calls AddToPropertySet(...) on the properties struct to add the member
	//   specific properties in the correct place in the structure.
	//
	AddPrebuiltModule(ctx SdkMemberContext, member SdkMember) BpModule

	// Create a structure into which variant specific properties can be added.
	CreateVariantPropertiesStruct() SdkMemberProperties
}

// Base type for SdkMemberType implementations.
type SdkMemberTypeBase struct {
	PropertyName         string
	SupportsSdk          bool
	TransitiveSdkMembers bool
}

func (b *SdkMemberTypeBase) SdkPropertyName() string {
	return b.PropertyName
}

func (b *SdkMemberTypeBase) UsableWithSdkAndSdkSnapshot() bool {
	return b.SupportsSdk
}

func (b *SdkMemberTypeBase) HasTransitiveSdkMembers() bool {
	return b.TransitiveSdkMembers
}

// Encapsulates the information about registered SdkMemberTypes.
type SdkMemberTypesRegistry struct {
	// The list of types sorted by property name.
	list []SdkMemberType

	// The key that uniquely identifies this registry instance.
	key OnceKey
}

func (r *SdkMemberTypesRegistry) copyAndAppend(memberType SdkMemberType) *SdkMemberTypesRegistry {
	oldList := r.list

	// Copy the slice just in case this is being read while being modified, e.g. when testing.
	list := make([]SdkMemberType, 0, len(oldList)+1)
	list = append(list, oldList...)
	list = append(list, memberType)

	// Sort the member types by their property name to ensure that registry order has no effect
	// on behavior.
	sort.Slice(list, func(i1, i2 int) bool {
		t1 := list[i1]
		t2 := list[i2]

		return t1.SdkPropertyName() < t2.SdkPropertyName()
	})

	// Generate a key that identifies the slice of SdkMemberTypes by joining the property names
	// from all the SdkMemberType .
	var properties []string
	for _, t := range list {
		properties = append(properties, t.SdkPropertyName())
	}
	key := NewOnceKey(strings.Join(properties, "|"))

	// Create a new registry so the pointer uniquely identifies the set of registered types.
	return &SdkMemberTypesRegistry{
		list: list,
		key:  key,
	}
}

func (r *SdkMemberTypesRegistry) RegisteredTypes() []SdkMemberType {
	return r.list
}

func (r *SdkMemberTypesRegistry) UniqueOnceKey() OnceKey {
	// Use the pointer to the registry as the unique key.
	return NewCustomOnceKey(r)
}

// The set of registered SdkMemberTypes, one for sdk module and one for module_exports.
var ModuleExportsMemberTypes = &SdkMemberTypesRegistry{}
var SdkMemberTypes = &SdkMemberTypesRegistry{}

// Register an SdkMemberType object to allow them to be used in the sdk and sdk_snapshot module
// types.
func RegisterSdkMemberType(memberType SdkMemberType) {
	// All member types are usable with module_exports.
	ModuleExportsMemberTypes = ModuleExportsMemberTypes.copyAndAppend(memberType)

	// Only those that explicitly indicate it are usable with sdk.
	if memberType.UsableWithSdkAndSdkSnapshot() {
		SdkMemberTypes = SdkMemberTypes.copyAndAppend(memberType)
	}
}

// Base structure for all implementations of SdkMemberProperties.
//
// Contains common properties that apply across many different member types. These
// are not affected by the optimization to extract common values.
type SdkMemberPropertiesBase struct {
	// The number of unique os types supported by the member variants.
	//
	// If a member has a variant with more than one os type then it will need to differentiate
	// the locations of any of their prebuilt files in the snapshot by os type to prevent them
	// from colliding. See OsPrefix().
	//
	// This property is the same for all variants of a member and so would be optimized away
	// if it was not explicitly kept.
	Os_count int `sdk:"keep"`

	// The os type for which these properties refer.
	//
	// Provided to allow a member to differentiate between os types in the locations of their
	// prebuilt files when it supports more than one os type.
	//
	// This property is the same for all os type specific variants of a member and so would be
	// optimized away if it was not explicitly kept.
	Os OsType `sdk:"keep"`

	// The setting to use for the compile_multilib property.
	//
	// This property is set after optimization so there is no point in trying to optimize it.
	Compile_multilib string `sdk:"keep"`
}

// The os prefix to use for any file paths in the sdk.
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

// Interface to be implemented on top of a structure that contains variant specific
// information.
//
// Struct fields that are capitalized are examined for common values to extract. Fields
// that are not capitalized are assumed to be arch specific.
type SdkMemberProperties interface {
	// Access the base structure.
	Base() *SdkMemberPropertiesBase

	// Populate this structure with information from the variant.
	PopulateFromVariant(ctx SdkMemberContext, variant Module)

	// Add the information from this structure to the property set.
	AddToPropertySet(ctx SdkMemberContext, propertySet BpPropertySet)
}

// Provides access to information common to a specific member.
type SdkMemberContext interface {

	// The module context of the sdk common os variant which is creating the snapshot.
	SdkModuleContext() ModuleContext

	// The builder of the snapshot.
	SnapshotBuilder() SnapshotBuilder

	// The type of the member.
	MemberType() SdkMemberType

	// The name of the member.
	//
	// Provided for use by sdk members to create a member specific location within the snapshot
	// into which to copy the prebuilt files.
	Name() string
}
