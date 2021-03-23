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

package sdk

import (
	"testing"

	"android/soong/android"
	"android/soong/java"
)

func TestSnapshotWithCompatConfig(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		java.PrepareForTestWithPlatformCompatConfig,
	).RunTestWithBp(t, `
		sdk {
			name: "mysdk",
			compat_configs: ["myconfig"],
		}

		platform_compat_config {
			name: "myconfig",
		}
	`)

	CheckSnapshot(t, result, "mysdk", "",
		checkVersionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_platform_compat_config {
    name: "mysdk_myconfig@current",
    sdk_member_name: "myconfig",
    visibility: ["//visibility:public"],
    metadata: "compat_configs/myconfig/myconfig_meta.xml",
}

sdk_snapshot {
    name: "mysdk@current",
    visibility: ["//visibility:public"],
    compat_configs: ["mysdk_myconfig@current"],
}
`),
		checkUnversionedAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_platform_compat_config {
    name: "myconfig",
    prefer: false,
    visibility: ["//visibility:public"],
    metadata: "compat_configs/myconfig/myconfig_meta.xml",
}
`),
		checkAllCopyRules(`
.intermediates/myconfig/android_common/myconfig_meta.xml -> compat_configs/myconfig/myconfig_meta.xml
`),
	)
}
