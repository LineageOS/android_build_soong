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

func TestSnapshotWithPackageDefaultLicense(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.PrepareForTestWithLicenses,
		android.PrepareForTestWithLicenseDefaultModules,
		android.MockFS{
			"NOTICE1": nil,
			"NOTICE2": nil,
		}.AddToFixture(),
	).RunTestWithBp(t, `
		package {
			default_applicable_licenses: ["mylicense"],
		}

		license {
			name: "mylicense",
			license_kinds: [
				"SPDX-license-identifier-Apache-2.0",
				"legacy_unencumbered",
			],
			license_text: [
				"NOTICE1",
				"NOTICE2",
			],
		}

		sdk {
			name: "mysdk",
			java_header_libs: ["myjavalib"],
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkUnversionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

package {
    // A default list here prevents the license LSC from adding its own list which would
    // be unnecessary as every module in the sdk already has its own licenses property.
    default_applicable_licenses: ["Android-Apache-2.0"],
}

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    licenses: ["mysdk_mylicense"],
    jars: ["java/myjavalib.jar"],
}

license {
    name: "mysdk_mylicense",
    visibility: ["//visibility:private"],
    license_kinds: [
        "SPDX-license-identifier-Apache-2.0",
        "legacy_unencumbered",
    ],
    license_text: [
        "licenses/NOTICE1",
        "licenses/NOTICE2",
    ],
}
		`),
		checkVersionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

package {
    // A default list here prevents the license LSC from adding its own list which would
    // be unnecessary as every module in the sdk already has its own licenses property.
    default_applicable_licenses: ["Android-Apache-2.0"],
}

java_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    licenses: ["mysdk_mylicense@current"],
    jars: ["java/myjavalib.jar"],
}

license {
    name: "mysdk_mylicense@current",
    sdk_member_name: "mylicense",
    visibility: ["//visibility:private"],
    license_kinds: [
        "SPDX-license-identifier-Apache-2.0",
        "legacy_unencumbered",
    ],
    license_text: [
        "licenses/NOTICE1",
        "licenses/NOTICE2",
    ],
}

sdk_snapshot {
    name: "mysdk@current",
    visibility: ["//visibility:public"],
    java_header_libs: ["mysdk_myjavalib@current"],
}
		`),
		checkAllCopyRules(`
.intermediates/myjavalib/android_common/turbine-combined/myjavalib.jar -> java/myjavalib.jar
NOTICE1 -> licenses/NOTICE1
NOTICE2 -> licenses/NOTICE2
`),
	)
}
