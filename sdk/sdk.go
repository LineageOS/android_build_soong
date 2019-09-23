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
	"github.com/google/blueprint"

	"android/soong/android"
	// This package doesn't depend on the apex package, but import it to make its mutators to be
	// registered before mutators in this package. See RegisterPostDepsMutators for more details.
	_ "android/soong/apex"
)

func init() {
	android.RegisterModuleType("sdk", ModuleFactory)
	android.PreDepsMutators(RegisterPreDepsMutators)
	android.PostDepsMutators(RegisterPostDepsMutators)
}

type sdk struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties sdkProperties
}

type sdkProperties struct {
	// The list of java_import modules that provide Java stubs for this SDK
	Java_libs          []string
	Native_shared_libs []string
}

// sdk defines an SDK which is a logical group of modules (e.g. native libs, headers, java libs, etc.)
// which Mainline modules like APEX can choose to build with.
func ModuleFactory() android.Module {
	s := &sdk{}
	s.AddProperties(&s.properties)
	android.InitAndroidMultiTargetsArchModule(s, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(s)
	return s
}

func (s *sdk) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// TODO(jiyong): add build rules for creating stubs from members of this SDK
}

// RegisterPreDepsMutators registers pre-deps mutators to support modules implementing SdkAware
// interface and the sdk module type. This function has been made public to be called by tests
// outside of the sdk package
func RegisterPreDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.BottomUp("SdkMember", memberMutator).Parallel()
	ctx.TopDown("SdkMember_deps", memberDepsMutator).Parallel()
	ctx.BottomUp("SdkMemberInterVersion", memberInterVersionMutator).Parallel()
}

// RegisterPostDepshMutators registers post-deps mutators to support modules implementing SdkAware
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
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
}

// For dependencies from an SDK module to its members
// e.g. mysdk -> libfoo and libbar
var sdkMemberDepTag dependencyTag

// For dependencies from an in-development version of an SDK member to frozen versions of the same member
// e.g. libfoo -> libfoo.mysdk.11 and libfoo.mysdk.12
type sdkMemberVesionedDepTag struct {
	dependencyTag
	member  string
	version string
}

// Step 1: create dependencies from an SDK module to its members.
func memberMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*sdk); ok {
		mctx.AddVariationDependencies(nil, sdkMemberDepTag, m.properties.Java_libs...)

		targets := mctx.MultiTargets()
		for _, target := range targets {
			mctx.AddFarVariationDependencies([]blueprint.Variation{
				{Mutator: "arch", Variation: target.String()},
				{Mutator: "image", Variation: "core"},
				{Mutator: "link", Variation: "shared"},
			}, sdkMemberDepTag, m.properties.Native_shared_libs...)
		}
	}
}

// Step 2: record that dependencies of SDK modules are members of the SDK modules
func memberDepsMutator(mctx android.TopDownMutatorContext) {
	if _, ok := mctx.Module().(*sdk); ok {
		mySdkRef := android.ParseSdkRef(mctx, mctx.ModuleName(), "name")
		mctx.VisitDirectDeps(func(child android.Module) {
			if member, ok := child.(android.SdkAware); ok {
				member.MakeMemberOf(mySdkRef)
			}
		})
	}
}

// Step 3: create dependencies from the in-development version of an SDK member to frozen versions
// of the same member. By having these dependencies, they are mutated for multiple Mainline modules
// (apex and apk), each of which might want different sdks to be built with. For example, if both
// apex A and B are referencing libfoo which is a member of sdk 'mysdk', the two APEXes can be
// built with libfoo.mysdk.11 and libfoo.mysdk.12, respectively depending on which sdk they are
// using.
func memberInterVersionMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(android.SdkAware); ok && m.IsInAnySdk() {
		if !m.ContainingSdk().IsCurrentVersion() {
			memberName := m.MemberName()
			tag := sdkMemberVesionedDepTag{member: memberName, version: m.ContainingSdk().Version}
			mctx.AddReverseDependency(mctx.Module(), tag, memberName)
		}
	}
}

// Step 4: transitively ripple down the SDK requirements from the root modules like APEX to its
// descendants
func sdkDepsMutator(mctx android.TopDownMutatorContext) {
	if m, ok := mctx.Module().(android.SdkAware); ok {
		// Module types for Mainline modules (e.g. APEX) are expected to implement RequiredSdks()
		// by reading its own properties like `uses_sdks`.
		requiredSdks := m.RequiredSdks()
		if len(requiredSdks) > 0 {
			mctx.VisitDirectDeps(func(m android.Module) {
				if dep, ok := m.(android.SdkAware); ok {
					dep.BuildWithSdks(requiredSdks)
				}
			})
		}
	}
}

// Step 5: if libfoo.mysdk.11 is in the context where version 11 of mysdk is requested, the
// versioned module is used instead of the un-versioned (in-development) module libfoo
func sdkDepsReplaceMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(android.SdkAware); ok && m.IsInAnySdk() {
		if sdk := m.ContainingSdk(); !sdk.IsCurrentVersion() {
			if m.RequiredSdks().Contains(sdk) {
				// Note that this replacement is done only for the modules that have the same
				// variations as the current module. Since current module is already mutated for
				// apex references in other APEXes are not affected by this replacement.
				memberName := m.MemberName()
				mctx.ReplaceDependencies(memberName)
			}
		}
	}
}
