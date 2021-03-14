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
	"android/soong/android"
	"log"
	"os"
	"testing"

	"github.com/google/blueprint/proptools"
)

// Needed in an _test.go file in this package to ensure tests run correctly, particularly in IDE.
func TestMain(m *testing.M) {
	if android.BuildOs != android.Linux {
		// b/145598135 - Generating host snapshots for anything other than linux is not supported.
		log.Printf("Skipping as sdk snapshot generation is only supported on %s not %s", android.Linux, android.BuildOs)
		os.Exit(0)
	}

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
			apex_available: ["myapex"],
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
			prebuilt_visibility: [
				"//prebuilts/mysdk",
			],
			java_header_libs: [
				"myjavalib",
				"mypublicjavalib",
				"mydefaultedjavalib",
				"myprivatejavalib",
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

		java_library {
			name: "myprivatejavalib",
			srcs: ["Test.java"],
			visibility: ["//visibility:private"],
			system_modules: "none",
			sdk_version: "none",
		}
	`

	result := testSdkWithFs(t, ``,
		map[string][]byte{
			"package/Test.java":  nil,
			"package/Android.bp": []byte(packageBp),
		})

	CheckSnapshot(t, result, "mysdk", "package",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "mysdk_myjavalib@current",
    sdk_member_name: "myjavalib",
    visibility: [
        "//other/foo",
        "//package",
        "//prebuilts/mysdk",
    ],
    apex_available: ["//apex_available:platform"],
    jars: ["java/myjavalib.jar"],
}

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: [
        "//other/foo",
        "//package",
        "//prebuilts/mysdk",
    ],
    apex_available: ["//apex_available:platform"],
    jars: ["java/myjavalib.jar"],
}

java_import {
    name: "mysdk_mypublicjavalib@current",
    sdk_member_name: "mypublicjavalib",
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/mypublicjavalib.jar"],
}

java_import {
    name: "mypublicjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/mypublicjavalib.jar"],
}

java_import {
    name: "mysdk_mydefaultedjavalib@current",
    sdk_member_name: "mydefaultedjavalib",
    visibility: [
        "//other/bar",
        "//package",
        "//prebuilts/mysdk",
    ],
    apex_available: ["//apex_available:platform"],
    jars: ["java/mydefaultedjavalib.jar"],
}

java_import {
    name: "mydefaultedjavalib",
    prefer: false,
    visibility: [
        "//other/bar",
        "//package",
        "//prebuilts/mysdk",
    ],
    apex_available: ["//apex_available:platform"],
    jars: ["java/mydefaultedjavalib.jar"],
}

java_import {
    name: "mysdk_myprivatejavalib@current",
    sdk_member_name: "myprivatejavalib",
    visibility: [
        "//package",
        "//prebuilts/mysdk",
    ],
    apex_available: ["//apex_available:platform"],
    jars: ["java/myprivatejavalib.jar"],
}

java_import {
    name: "myprivatejavalib",
    prefer: false,
    visibility: [
        "//package",
        "//prebuilts/mysdk",
    ],
    apex_available: ["//apex_available:platform"],
    jars: ["java/myprivatejavalib.jar"],
}

sdk_snapshot {
    name: "mysdk@current",
    visibility: [
        "//other/foo",
        "//package:__subpackages__",
    ],
    java_header_libs: [
        "mysdk_myjavalib@current",
        "mysdk_mypublicjavalib@current",
        "mysdk_mydefaultedjavalib@current",
        "mysdk_myprivatejavalib@current",
    ],
}
`))
}

func TestPrebuiltVisibilityProperty_IsValidated(t *testing.T) {
	testSdkError(t, `prebuilt_visibility: cannot mix "//visibility:private" with any other visibility rules`, `
		sdk {
			name: "mysdk",
			prebuilt_visibility: [
				"//foo",
				"//visibility:private",
			],
		}
`)
}

func TestPrebuiltVisibilityProperty_AddPrivate(t *testing.T) {
	testSdkError(t, `prebuilt_visibility: "//visibility:private" does not widen the visibility`, `
		sdk {
			name: "mysdk",
			prebuilt_visibility: [
				"//visibility:private",
			],
			java_header_libs: [
				"myjavalib",
			],
		}

		java_library {
			name: "myjavalib",
			// Uses package default visibility
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
		}
`)
}

func TestSdkInstall(t *testing.T) {
	sdk := `
		sdk {
			name: "mysdk",
		}
	`
	result := testSdkWithFs(t, sdk, nil)

	CheckSnapshot(t, result, "mysdk", "",
		checkAllOtherCopyRules(`.intermediates/mysdk/common_os/mysdk-current.zip -> mysdk-current.zip`))
}

type EmbeddedPropertiesStruct struct {
	S_Embedded_Common    string `android:"arch_variant"`
	S_Embedded_Different string `android:"arch_variant"`
}

type testPropertiesStruct struct {
	name        string
	private     string
	Public_Kept string `sdk:"keep"`
	S_Common    string
	S_Different string `android:"arch_variant"`
	A_Common    []string
	A_Different []string `android:"arch_variant"`
	F_Common    *bool
	F_Different *bool `android:"arch_variant"`
	EmbeddedPropertiesStruct
}

func (p *testPropertiesStruct) optimizableProperties() interface{} {
	return p
}

func (p *testPropertiesStruct) String() string {
	return p.name
}

var _ propertiesContainer = (*testPropertiesStruct)(nil)

func TestCommonValueOptimization(t *testing.T) {
	common := &testPropertiesStruct{name: "common"}
	structs := []propertiesContainer{
		&testPropertiesStruct{
			name:        "struct-0",
			private:     "common",
			Public_Kept: "common",
			S_Common:    "common",
			S_Different: "upper",
			A_Common:    []string{"first", "second"},
			A_Different: []string{"alpha", "beta"},
			F_Common:    proptools.BoolPtr(false),
			F_Different: proptools.BoolPtr(false),
			EmbeddedPropertiesStruct: EmbeddedPropertiesStruct{
				S_Embedded_Common:    "embedded_common",
				S_Embedded_Different: "embedded_upper",
			},
		},
		&testPropertiesStruct{
			name:        "struct-1",
			private:     "common",
			Public_Kept: "common",
			S_Common:    "common",
			S_Different: "lower",
			A_Common:    []string{"first", "second"},
			A_Different: []string{"alpha", "delta"},
			F_Common:    proptools.BoolPtr(false),
			F_Different: proptools.BoolPtr(true),
			EmbeddedPropertiesStruct: EmbeddedPropertiesStruct{
				S_Embedded_Common:    "embedded_common",
				S_Embedded_Different: "embedded_lower",
			},
		},
	}

	extractor := newCommonValueExtractor(common)

	err := extractor.extractCommonProperties(common, structs)
	android.AssertDeepEquals(t, "unexpected error", nil, err)

	android.AssertDeepEquals(t, "common properties not correct",
		&testPropertiesStruct{
			name:        "common",
			private:     "",
			Public_Kept: "",
			S_Common:    "common",
			S_Different: "",
			A_Common:    []string{"first", "second"},
			A_Different: []string(nil),
			F_Common:    proptools.BoolPtr(false),
			F_Different: nil,
			EmbeddedPropertiesStruct: EmbeddedPropertiesStruct{
				S_Embedded_Common:    "embedded_common",
				S_Embedded_Different: "",
			},
		},
		common)

	android.AssertDeepEquals(t, "updated properties[0] not correct",
		&testPropertiesStruct{
			name:        "struct-0",
			private:     "common",
			Public_Kept: "common",
			S_Common:    "",
			S_Different: "upper",
			A_Common:    nil,
			A_Different: []string{"alpha", "beta"},
			F_Common:    nil,
			F_Different: proptools.BoolPtr(false),
			EmbeddedPropertiesStruct: EmbeddedPropertiesStruct{
				S_Embedded_Common:    "",
				S_Embedded_Different: "embedded_upper",
			},
		},
		structs[0])

	android.AssertDeepEquals(t, "updated properties[1] not correct",
		&testPropertiesStruct{
			name:        "struct-1",
			private:     "common",
			Public_Kept: "common",
			S_Common:    "",
			S_Different: "lower",
			A_Common:    nil,
			A_Different: []string{"alpha", "delta"},
			F_Common:    nil,
			F_Different: proptools.BoolPtr(true),
			EmbeddedPropertiesStruct: EmbeddedPropertiesStruct{
				S_Embedded_Common:    "",
				S_Embedded_Different: "embedded_lower",
			},
		},
		structs[1])
}

func TestCommonValueOptimization_InvalidArchSpecificVariants(t *testing.T) {
	common := &testPropertiesStruct{name: "common"}
	structs := []propertiesContainer{
		&testPropertiesStruct{
			name:     "struct-0",
			S_Common: "should-be-but-is-not-common0",
		},
		&testPropertiesStruct{
			name:     "struct-1",
			S_Common: "should-be-but-is-not-common1",
		},
	}

	extractor := newCommonValueExtractor(common)

	err := extractor.extractCommonProperties(common, structs)
	android.AssertErrorMessageEquals(t, "unexpected error", `field "S_Common" is not tagged as "arch_variant" but has arch specific properties:
    "struct-0" has value "should-be-but-is-not-common0"
    "struct-1" has value "should-be-but-is-not-common1"`, err)
}
