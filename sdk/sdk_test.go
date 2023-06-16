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
	"log"
	"os"
	"runtime"
	"testing"

	"android/soong/android"
	"android/soong/java"

	"github.com/google/blueprint/proptools"
)

// Needed in an _test.go file in this package to ensure tests run correctly, particularly in IDE.
func TestMain(m *testing.M) {
	if runtime.GOOS != "linux" {
		// b/145598135 - Generating host snapshots for anything other than linux is not supported.
		log.Printf("Skipping as sdk snapshot generation is only supported on linux not %s", runtime.GOOS)
		os.Exit(0)
	}

	os.Exit(m.Run())
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
    name: "mypublicjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/mypublicjavalib.jar"],
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
    name: "myprivatejavalib",
    prefer: false,
    visibility: [
        "//package",
        "//prebuilts/mysdk",
    ],
    apex_available: ["//apex_available:platform"],
    jars: ["java/myprivatejavalib.jar"],
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
		checkAllOtherCopyRules(`
.intermediates/mysdk/common_os/mysdk-current.info -> mysdk-current.info
.intermediates/mysdk/common_os/mysdk-current.zip -> mysdk-current.zip
`))
}

type EmbeddedPropertiesStruct struct {
	S_Embedded_Common    string `android:"arch_variant"`
	S_Embedded_Different string `android:"arch_variant"`
}

type testPropertiesStruct struct {
	name          string
	private       string
	Public_Ignore string `sdk:"ignore"`
	Public_Keep   string `sdk:"keep"`
	S_Common      string
	S_Different   string `android:"arch_variant"`
	A_Common      []string
	A_Different   []string `android:"arch_variant"`
	F_Common      *bool
	F_Different   *bool `android:"arch_variant"`
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
			name:          "struct-0",
			private:       "common",
			Public_Ignore: "common",
			Public_Keep:   "keep",
			S_Common:      "common",
			S_Different:   "upper",
			A_Common:      []string{"first", "second"},
			A_Different:   []string{"alpha", "beta"},
			F_Common:      proptools.BoolPtr(false),
			F_Different:   proptools.BoolPtr(false),
			EmbeddedPropertiesStruct: EmbeddedPropertiesStruct{
				S_Embedded_Common:    "embedded_common",
				S_Embedded_Different: "embedded_upper",
			},
		},
		&testPropertiesStruct{
			name:          "struct-1",
			private:       "common",
			Public_Ignore: "common",
			Public_Keep:   "keep",
			S_Common:      "common",
			S_Different:   "lower",
			A_Common:      []string{"first", "second"},
			A_Different:   []string{"alpha", "delta"},
			F_Common:      proptools.BoolPtr(false),
			F_Different:   proptools.BoolPtr(true),
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
			name:          "common",
			private:       "",
			Public_Ignore: "",
			Public_Keep:   "keep",
			S_Common:      "common",
			S_Different:   "",
			A_Common:      []string{"first", "second"},
			A_Different:   []string(nil),
			F_Common:      proptools.BoolPtr(false),
			F_Different:   nil,
			EmbeddedPropertiesStruct: EmbeddedPropertiesStruct{
				S_Embedded_Common:    "embedded_common",
				S_Embedded_Different: "",
			},
		},
		common)

	android.AssertDeepEquals(t, "updated properties[0] not correct",
		&testPropertiesStruct{
			name:          "struct-0",
			private:       "common",
			Public_Ignore: "common",
			Public_Keep:   "keep",
			S_Common:      "",
			S_Different:   "upper",
			A_Common:      nil,
			A_Different:   []string{"alpha", "beta"},
			F_Common:      nil,
			F_Different:   proptools.BoolPtr(false),
			EmbeddedPropertiesStruct: EmbeddedPropertiesStruct{
				S_Embedded_Common:    "",
				S_Embedded_Different: "embedded_upper",
			},
		},
		structs[0])

	android.AssertDeepEquals(t, "updated properties[1] not correct",
		&testPropertiesStruct{
			name:          "struct-1",
			private:       "common",
			Public_Ignore: "common",
			Public_Keep:   "keep",
			S_Common:      "",
			S_Different:   "lower",
			A_Common:      nil,
			A_Different:   []string{"alpha", "delta"},
			F_Common:      nil,
			F_Different:   proptools.BoolPtr(true),
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

// Ensure that sdk snapshot related environment variables work correctly.
func TestSnapshot_EnvConfiguration(t *testing.T) {
	bp := `
		sdk {
			name: "mysdk",
			java_header_libs: ["myjavalib"],
		}

		java_library {
			name: "myjavalib",
			srcs: ["Test.java"],
			system_modules: "none",
			sdk_version: "none",
			compile_dex: true,
			host_supported: true,
		}
	`
	preparer := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureWithRootAndroidBp(bp),
	)

	checkZipFile := func(t *testing.T, result *android.TestResult, expected string) {
		zipRule := result.ModuleForTests("mysdk", "common_os").Rule("SnapshotZipFiles")
		android.AssertStringEquals(t, "snapshot zip file", expected, zipRule.Output.String())
	}

	t.Run("no env variables", func(t *testing.T) {
		result := preparer.RunTest(t)

		checkZipFile(t, result, "out/soong/.intermediates/mysdk/common_os/mysdk-current.zip")

		CheckSnapshot(t, result, "mysdk", "",
			checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["java/myjavalib.jar"],
}
			`),
		)
	})

	t.Run("SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE=S", func(t *testing.T) {
		result := android.GroupFixturePreparers(
			prepareForSdkTestWithJava,
			java.PrepareForTestWithJavaDefaultModules,
			java.PrepareForTestWithJavaSdkLibraryFiles,
			java.FixtureWithLastReleaseApis("mysdklibrary"),
			android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				bootclasspath_fragments: ["mybootclasspathfragment"],
			}

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				apex_available: ["myapex"],
				contents: ["mysdklibrary"],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			java_sdk_library {
				name: "mysdklibrary",
				srcs: ["Test.java"],
				compile_dex: true,
				sdk_version: "S",
				public: {enabled: true},
				permitted_packages: ["mysdklibrary"],
			}
		`),
			android.FixtureMergeEnv(map[string]string{
				"SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE": "S",
			}),
		).RunTest(t)

		CheckSnapshot(t, result, "mysdk", "",
			checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

prebuilt_bootclasspath_fragment {
    name: "mybootclasspathfragment",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["myapex"],
    contents: ["mysdklibrary"],
    hidden_api: {
        annotation_flags: "hiddenapi/annotation-flags.csv",
        metadata: "hiddenapi/metadata.csv",
        index: "hiddenapi/index.csv",
        stub_flags: "hiddenapi/stub-flags.csv",
        all_flags: "hiddenapi/all-flags.csv",
    },
}

java_sdk_library_import {
    name: "mysdklibrary",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    shared_library: true,
    compile_dex: true,
    permitted_packages: ["mysdklibrary"],
    public: {
        jars: ["sdk_library/public/mysdklibrary-stubs.jar"],
        stub_srcs: ["sdk_library/public/mysdklibrary_stub_sources"],
        current_api: "sdk_library/public/mysdklibrary.txt",
        removed_api: "sdk_library/public/mysdklibrary-removed.txt",
        sdk_version: "current",
    },
}
`),

			checkAllCopyRules(`
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/annotation-flags.csv -> hiddenapi/annotation-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/metadata.csv -> hiddenapi/metadata.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/index.csv -> hiddenapi/index.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/stub-flags.csv -> hiddenapi/stub-flags.csv
.intermediates/mybootclasspathfragment/android_common/modular-hiddenapi/all-flags.csv -> hiddenapi/all-flags.csv
.intermediates/mysdklibrary.stubs/android_common/combined/mysdklibrary.stubs.jar -> sdk_library/public/mysdklibrary-stubs.jar
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_api.txt -> sdk_library/public/mysdklibrary.txt
.intermediates/mysdklibrary.stubs.source/android_common/metalava/mysdklibrary.stubs.source_removed.txt -> sdk_library/public/mysdklibrary-removed.txt
`),
		)
	})

}
