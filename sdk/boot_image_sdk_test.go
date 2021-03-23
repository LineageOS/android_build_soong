// Copyright (C) 2021 The Android Open Source Project
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
	"testing"

	"android/soong/android"
)

func TestSnapshotWithBootImage(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				boot_images: ["mybootimage"],
			}

			boot_image {
				name: "mybootimage",
				image_name: "art",
			}
		`),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "",
		checkUnversionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_boot_image {
    name: "mybootimage",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    image_name: "art",
}
`),
		checkVersionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_boot_image {
    name: "mysdk_mybootimage@current",
    sdk_member_name: "mybootimage",
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    image_name: "art",
}

sdk_snapshot {
    name: "mysdk@current",
    visibility: ["//visibility:public"],
    boot_images: ["mysdk_mybootimage@current"],
}
`),
		checkAllCopyRules(""))
}

// Test that boot_image works with sdk.
func TestBasicSdkWithBootImage(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForSdkTestWithApex,
		prepareForSdkTestWithJava,
		android.FixtureWithRootAndroidBp(`
		sdk {
			name: "mysdk",
			boot_images: ["mybootimage"],
		}

		boot_image {
			name: "mybootimage",
			image_name: "art",
			apex_available: ["myapex"],
		}

		sdk_snapshot {
			name: "mysdk@1",
			boot_images: ["mybootimage_mysdk_1"],
		}

		prebuilt_boot_image {
			name: "mybootimage_mysdk_1",
			sdk_member_name: "mybootimage",
			prefer: false,
			visibility: ["//visibility:public"],
			apex_available: [
				"myapex",
			],
			image_name: "art",
		}
	`),
	).RunTest(t)
}
