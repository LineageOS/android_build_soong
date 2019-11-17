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
	"strings"

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

	// Build a snapshot of the module.
	BuildSnapshot(sdkModuleContext ModuleContext, builder SnapshotBuilder)
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

	// Get the AndroidBpFile for the snapshot.
	AndroidBpFile() GeneratedSnapshotFile

	// Get a versioned name appropriate for the SDK snapshot version being taken.
	VersionedSdkMemberName(unversionedName string) interface{}
}

// Provides support for generating a file, e.g. the Android.bp file.
type GeneratedSnapshotFile interface {
	Printfln(format string, args ...interface{})
	Indent()
	Dedent()
}
