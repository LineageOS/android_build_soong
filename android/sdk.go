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

// SdkAware is the interface that must be supported by any module to become a member of SDK or to be
// built with SDK
type SdkAware interface {
	Module
	sdkBase() *SdkBase
	MakeMemberOf(sdk SdkRef)
	IsInAnySdk() bool
	ContainingSdk() SdkRef
	MemberName() string
	BuildWithSdks(sdks SdkRefs)
	RequiredSdks() SdkRefs
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

func (s *SdkBase) BuildSnapshot(sdkModuleContext ModuleContext, builder SnapshotBuilder) {
	sdkModuleContext.ModuleErrorf("module type " + sdkModuleContext.OtherModuleType(s.module) + " cannot be used in an sdk")
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
	// e.g.
	// bpPropertySet.AddPropertyWithTag("libs", []string{"member1", "member2"}, builder.SdkMemberReferencePropertyTag())
	SdkMemberReferencePropertyTag() BpPropertyTag
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

	// Build the snapshot for the SDK member
	//
	// The ModuleContext provided is for the SDK module, so information for
	// variants in the supplied member can be accessed using the Other... methods.
	//
	// The SdkMember is guaranteed to contain variants for which the
	// IsInstance(Module) method returned true.
	BuildSnapshot(sdkModuleContext ModuleContext, builder SnapshotBuilder, member SdkMember)
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
