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
	"github.com/google/blueprint"
)

func init() {
	RegisterFdoProfileBuildComponents(android.InitRegistrationContext)
}

func RegisterFdoProfileBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("fdo_profile", FdoProfileFactory)
}

type fdoProfile struct {
	android.ModuleBase

	properties fdoProfileProperties
}

type fdoProfileProperties struct {
	Profile *string `android:"arch_variant"`
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
	return m
}
