// Copyright 2019 Google Inc. All rights reserved.
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
)

// Needed in an _test.go file in this package to ensure tests run correctly, particularly in IDE.
func TestMain(m *testing.M) {
	runTestWithBuildDir(m)
}

func TestDepNotInRequiredSdks(t *testing.T) {
	testSdkError(t, `module "myjavalib".*depends on "otherlib".*that isn't part of the required SDKs:.*`, `
		sdk {
			name: "mysdk",
			java_header_libs: ["sdkmember"],
		}

		sdk_snapshot {
			name: "mysdk@1",
			java_header_libs: ["sdkmember_mysdk_1"],
		}

		java_import {
			name: "sdkmember",
			prefer: false,
			host_supported: true,
		}

		java_import {
			name: "sdkmember_mysdk_1",
			sdk_member_name: "sdkmember",
			host_supported: true,
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			libs: [
				"sdkmember",
				"otherlib",
			],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}

		// this lib is no in mysdk
		java_library {
			name: "otherlib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}

		apex {
			name: "myapex",
			java_libs: ["myjavalib"],
			uses_sdks: ["mysdk@1"],
			key: "myapex.key",
			certificate: ":myapex.cert",
		}
	`)
}

// Ensure that prebuilt modules have the same effective visibility as the source
// modules.
func TestSnapshotVisibility(t *testing.T) {
	packageBp := `
		package {
			default_visibility: ["//other/foo"],
		}

		sdk {
			name: "mysdk",
			visibility: [
				"//other/foo",
				// This short form will be replaced with //package:__subpackages__ in the
				// generated sdk_snapshot.
				":__subpackages__",
			],
			java_header_libs: [
				"myjavalib",
				"mypublicjavalib",
				"mydefaultedjavalib",
			],
		}

		java_library {
			name: "myjavalib",
			// Uses package default visibility
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
		}

		java_defaults {
			name: "java-defaults",
			visibility: ["//other/bar"], 
		}

		java_library {
			name: "mypublicjavalib",
			defaults: ["java-defaults"],
      visibility: ["//visibility:public"],
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
		}

		java_defaults {
			name: "myjavadefaults",
			visibility: ["//other/bar"],
		}

		java_library {
			name: "mydefaultedjavalib",
			defaults: ["myjavadefaults"],
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
		}
	`

	result := testSdkWithFs(t, ``,
		map[string][]byte{
			"package/Test.java":  nil,
			"package/Android.bp": []byte(packageBp),
		})

	result.CheckSnapshot("mysdk", "android_common", "package",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    visibility: ["//other/foo:__pkg__"],
    jars: ["java/myjavalib.jar"],
}

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//other/foo:__pkg__"],
    jars: ["java/myjavalib.jar"],
}

java_import {
    name: "mysdk_mypublicjavalib@current",
    sdk_member_name: "mypublicjavalib",
    visibility: ["//visibility:public"],
    jars: ["java/mypublicjavalib.jar"],
}

java_import {
    name: "mypublicjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    jars: ["java/mypublicjavalib.jar"],
}

java_import {
    name: "mysdk_mydefaultedjavalib@current",
    sdk_member_name: "mydefaultedjavalib",
    visibility: ["//other/bar:__pkg__"],
    jars: ["java/mydefaultedjavalib.jar"],
}

java_import {
    name: "mydefaultedjavalib",
    prefer: false,
    visibility: ["//other/bar:__pkg__"],
    jars: ["java/mydefaultedjavalib.jar"],
}

sdk_snapshot {
    name: "mysdk@current",
    visibility: [
        "//other/foo:__pkg__",
        "//package:__subpackages__",
    ],
    java_header_libs: [
        "mysdk_myjavalib@current",
        "mysdk_mypublicjavalib@current",
        "mysdk_mydefaultedjavalib@current",
    ],
}
`))
}
