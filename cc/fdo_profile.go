// Copyright 2023 Google Inc. All rights reserved.
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

package cc

import (
	"android/soong/android"
	"android/soong/bazel"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterFdoProfileBuildComponents(android.InitRegistrationContext)
}

func RegisterFdoProfileBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("fdo_profile", FdoProfileFactory)
}

type fdoProfile struct {
	android.ModuleBase
	android.BazelModuleBase

	properties fdoProfileProperties
}

type fdoProfileProperties struct {
	Profile *string `android:"arch_variant"`
}

type bazelFdoProfileAttributes struct {
	Profile bazel.StringAttribute
}

func (fp *fdoProfile) ConvertWithBp2build(ctx android.TopDownMutatorContext) {
	var profileAttr bazel.StringAttribute

	archVariantProps := fp.GetArchVariantProperties(ctx, &fdoProfileProperties{})
	for axis, configToProps := range archVariantProps {
		for config, _props := range configToProps {
			if archProps, ok := _props.(*fdoProfileProperties); ok {
				if axis.String() == "arch" || axis.String() == "no_config" {
					if archProps.Profile != nil {
						profileAttr.SetSelectValue(axis, config, archProps.Profile)
					}
				}
			}
		}
	}

	// Ideally, cc_library_shared's fdo_profile attr can be a select statement so that we
	// don't lift the restriction here. However, in cc_library_shared macro, fdo_profile
	// is used as a string, we need to temporarily lift the host restriction until we can
	// pass use fdo_profile attr with select statement
	// https://cs.android.com/android/platform/superproject/+/master:build/bazel/rules/cc/cc_library_shared.bzl;l=127;drc=cc01bdfd39857eddbab04ef69ab6db22dcb1858a
	// TODO(b/276287371): Drop the restriction override after fdo_profile path is handled properly
	var noRestriction bazel.BoolAttribute
	noRestriction.SetSelectValue(bazel.NoConfigAxis, "", proptools.BoolPtr(true))

	ctx.CreateBazelTargetModuleWithRestrictions(
		bazel.BazelTargetModuleProperties{
			Rule_class: "fdo_profile",
		},
		android.CommonAttributes{
			Name: fp.Name(),
		},
		&bazelFdoProfileAttributes{
			Profile: profileAttr,
		},
		noRestriction,
	)
}

// FdoProfileInfo is provided by FdoProfileProvider
type FdoProfileInfo struct {
	Path android.Path
}

// FdoProfileProvider is used to provide path to an fdo profile
var FdoProfileProvider = blueprint.NewMutatorProvider(FdoProfileInfo{}, "fdo_profile")

// FdoProfileMutatorInterface is the interface implemented by fdo_profile module type
// module types that can depend on an fdo_profile module
type FdoProfileMutatorInterface interface {
	// FdoProfileMutator eithers set or get FdoProfileProvider
	fdoProfileMutator(ctx android.BottomUpMutatorContext)
}

var _ FdoProfileMutatorInterface = (*fdoProfile)(nil)

// GenerateAndroidBuildActions of fdo_profile does not have any build actions
func (fp *fdoProfile) GenerateAndroidBuildActions(ctx android.ModuleContext) {}

// FdoProfileMutator sets FdoProfileProvider to fdo_profile module
// or sets afdo.Properties.FdoProfilePath to path in FdoProfileProvider of the depended fdo_profile
func (fp *fdoProfile) fdoProfileMutator(ctx android.BottomUpMutatorContext) {
	if fp.properties.Profile != nil {
		path := android.PathForModuleSrc(ctx, *fp.properties.Profile)
		ctx.SetProvider(FdoProfileProvider, FdoProfileInfo{
			Path: path,
		})
	}
}

// fdoProfileMutator calls the generic fdoProfileMutator function of fdoProfileMutator
// which is implemented by cc and cc.FdoProfile
func fdoProfileMutator(ctx android.BottomUpMutatorContext) {
	if f, ok := ctx.Module().(FdoProfileMutatorInterface); ok {
		f.fdoProfileMutator(ctx)
	}
}

func FdoProfileFactory() android.Module {
	m := &fdoProfile{}
	m.AddProperties(&m.properties)
	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, android.MultilibBoth)
	android.InitBazelModule(m)
	return m
}
