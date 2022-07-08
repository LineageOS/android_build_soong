// Copyright (C) 2022 The Android Open Source Project
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

package apex

import (
	"android/soong/android"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// This file contains support for using apex modules within an sdk.

func init() {
	// Register sdk member types.
	android.RegisterSdkMemberType(&apexSdkMemberType{
		SdkMemberTypeBase: android.SdkMemberTypeBase{
			PropertyName: "apexes",
			SupportsSdk:  true,

			// The apexes property does not need to be included in the snapshot as adding an apex to an
			// sdk does not produce any prebuilts of the apex.
			PrebuiltsRequired: proptools.BoolPtr(false),
		},
	})
}

type apexSdkMemberType struct {
	android.SdkMemberTypeBase
}

func (mt *apexSdkMemberType) AddDependencies(ctx android.SdkDependencyContext, dependencyTag blueprint.DependencyTag, names []string) {
	ctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (mt *apexSdkMemberType) IsInstance(module android.Module) bool {
	_, ok := module.(*apexBundle)
	return ok
}

func (mt *apexSdkMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	panic("Sdk does not create prebuilts of the apexes in its snapshot")
}

func (mt *apexSdkMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	panic("Sdk does not create prebuilts of the apexes in its snapshot")
}
