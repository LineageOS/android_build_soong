// Copyright 2021 Google Inc. All rights reserved.
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

import "android/soong/android"

// This file contains support for the image variant sdk traits.

func init() {
	android.RegisterSdkMemberTrait(ramdiskImageRequiredSdkTrait)
	android.RegisterSdkMemberTrait(recoveryImageRequiredSdkTrait)
}

type imageSdkTraitStruct struct {
	android.SdkMemberTraitBase
}

var ramdiskImageRequiredSdkTrait android.SdkMemberTrait = &imageSdkTraitStruct{
	SdkMemberTraitBase: android.SdkMemberTraitBase{
		PropertyName: "ramdisk_image_required",
	},
}

var recoveryImageRequiredSdkTrait android.SdkMemberTrait = &imageSdkTraitStruct{
	SdkMemberTraitBase: android.SdkMemberTraitBase{
		PropertyName: "recovery_image_required",
	},
}
