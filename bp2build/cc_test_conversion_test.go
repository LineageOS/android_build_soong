// Copyright 2022 Google Inc. All rights reserved.
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

package bp2build

import (
	"fmt"
	"testing"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/genrule"
)

type ccTestBp2buildTestCase struct {
	description             string
	blueprint               string
	filesystem              map[string]string
	targets                 []testBazelTarget
	stubbedBuildDefinitions []string
}

func registerCcTestModuleTypes(ctx android.RegistrationContext) {
	cc.RegisterCCBuildComponents(ctx)
	ctx.RegisterModuleType("cc_binary", cc.BinaryFactory)
	ctx.RegisterModuleType("cc_library_static", cc.LibraryStaticFactory)
	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	ctx.RegisterModuleType("cc_test_library", cc.TestLibraryFactory)
	ctx.RegisterModuleType("genrule", genrule.GenRuleFactory)
}

func runCcTestTestCase(t *testing.T, testCase ccTestBp2buildTestCase) {
	t.Helper()
	moduleTypeUnderTest := "cc_test"
	description := fmt.Sprintf("%s %s", moduleTypeUnderTest, testCase.description)
	t.Run(description, func(t *testing.T) {
		t.Helper()
		RunBp2BuildTestCase(t, registerCcTestModuleTypes, Bp2buildTestCase{
			ExpectedBazelTargets:       generateBazelTargetsForTest(testCase.targets, android.HostAndDeviceSupported),
			Filesystem:                 testCase.filesystem,
			ModuleTypeUnderTest:        moduleTypeUnderTest,
			ModuleTypeUnderTestFactory: cc.TestFactory,
			Description:                description,
			Blueprint:                  testCase.blueprint,
			StubbedBuildDefinitions:    testCase.stubbedBuildDefinitions,
		})
	})
}

func TestBasicCcTest(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description: "basic cc_test with commonly used attributes",
		stubbedBuildDefinitions: []string{"libbuildversion", "libprotobuf-cpp-lite", "libprotobuf-cpp-full",
			"foolib", "hostlib", "data_mod", "cc_bin", "cc_lib", "cc_test_lib2", "libgtest_main", "libgtest"},
		blueprint: `
cc_test {
    name: "mytest",
    host_supported: true,
    srcs: ["test.cpp"],
    target: {
        android: {
            srcs: ["android.cpp"],
            shared_libs: ["foolib"],
        },
        linux: {
            srcs: ["linux.cpp"],
        },
        host: {
            static_libs: ["hostlib"],
        },
    },
    static_libs: ["cc_test_lib1"],
    shared_libs: ["cc_test_lib2"],
    data: [":data_mod", "file.txt"],
    data_bins: [":cc_bin"],
    data_libs: [":cc_lib"],
    cflags: ["-Wall"],
}

cc_test_library {
    name: "cc_test_lib1",
    host_supported: true,
    include_build_directory: false,
}
` + simpleModule("cc_library", "foolib") +
			simpleModule("cc_library_static", "hostlib") +
			simpleModule("genrule", "data_mod") +
			simpleModule("cc_binary", "cc_bin") +
			simpleModule("cc_library", "cc_lib") +
			simpleModule("cc_test_library", "cc_test_lib2") +
			simpleModule("cc_library_static", "libgtest_main") +
			simpleModule("cc_library_static", "libgtest"),
		targets: []testBazelTarget{
			{"cc_library_shared", "cc_test_lib1", AttrNameToString{}},
			{"cc_library_static", "cc_test_lib1_bp2build_cc_library_static", AttrNameToString{}},
			{"cc_test", "mytest", AttrNameToString{
				"copts": `["-Wall"]`,
				"data": `[
        ":data_mod",
        "file.txt",
        ":cc_bin",
        ":cc_lib",
    ]`,
				"deps": `[
        ":cc_test_lib1_bp2build_cc_library_static",
        ":libgtest_main",
        ":libgtest",
    ] + select({
        "//build/bazel/platforms/os:darwin": [":hostlib"],
        "//build/bazel/platforms/os:linux_bionic": [":hostlib"],
        "//build/bazel/platforms/os:linux_glibc": [":hostlib"],
        "//build/bazel/platforms/os:linux_musl": [":hostlib"],
        "//build/bazel/platforms/os:windows": [":hostlib"],
        "//conditions:default": [],
    })`,
				"local_includes": `["."]`,
				"dynamic_deps": `[":cc_test_lib2"] + select({
        "//build/bazel/platforms/os:android": [":foolib"],
        "//conditions:default": [],
    })`,
				"srcs": `["test.cpp"] + select({
        "//build/bazel/platforms/os:android": [
            "linux.cpp",
            "android.cpp",
        ],
        "//build/bazel/platforms/os:linux_bionic": ["linux.cpp"],
        "//build/bazel/platforms/os:linux_glibc": ["linux.cpp"],
        "//build/bazel/platforms/os:linux_musl": ["linux.cpp"],
        "//conditions:default": [],
    })`,
				"runs_on": `[
        "host_without_device",
        "device",
    ]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestBasicCcTestGtestIsolatedDisabled(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description: "cc test with disabled gtest and isolated props",
		blueprint: `
cc_test {
    name: "mytest",
    host_supported: true,
    srcs: ["test.cpp"],
    gtest: false,
    isolated: false,
}
`,
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"gtest":          "False",
				"local_includes": `["."]`,
				"srcs":           `["test.cpp"]`,
				"runs_on": `[
        "host_without_device",
        "device",
    ]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_TestOptions_Tags(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description:             "cc test with test_options.tags converted to tags",
		stubbedBuildDefinitions: []string{"libgtest_main", "libgtest"},
		blueprint: `
cc_test {
    name: "mytest",
    host_supported: true,
    srcs: ["test.cpp"],
    test_options: { tags: ["no-remote"] },
}
` + simpleModule("cc_library_static", "libgtest_main") +
			simpleModule("cc_library_static", "libgtest"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"tags":           `["no-remote"]`,
				"local_includes": `["."]`,
				"srcs":           `["test.cpp"]`,
				"deps": `[
        ":libgtest_main",
        ":libgtest",
    ]`,
				"runs_on": `[
        "host_without_device",
        "device",
    ]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_TestConfig(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description: "cc test that sets a test_config",
		filesystem: map[string]string{
			"test_config.xml": "",
		},
		stubbedBuildDefinitions: []string{"libgtest_main", "libgtest"},
		blueprint: `
cc_test {
	name: "mytest",
	srcs: ["test.cpp"],
	test_config: "test_config.xml",
}
` + simpleModule("cc_library_static", "libgtest_main") +
			simpleModule("cc_library_static", "libgtest"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["test.cpp"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"test_config":            `"test_config.xml"`,
				"deps": `[
        ":libgtest_main",
        ":libgtest",
    ]`,
				"runs_on": `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_TestConfigAndroidTestXML(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description: "cc test that defaults to test config AndroidTest.xml",
		filesystem: map[string]string{
			"AndroidTest.xml":   "",
			"DynamicConfig.xml": "",
		},
		stubbedBuildDefinitions: []string{"libgtest_main", "libgtest"},
		blueprint: `
cc_test {
	name: "mytest",
	srcs: ["test.cpp"],
}
` + simpleModule("cc_library_static", "libgtest_main") +
			simpleModule("cc_library_static", "libgtest"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["test.cpp"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"test_config":            `"AndroidTest.xml"`,
				"dynamic_config":         `"DynamicConfig.xml"`,
				"deps": `[
        ":libgtest_main",
        ":libgtest",
    ]`,
				"runs_on": `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_TestConfigTemplateOptions(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description: "cc test that sets test config template attributes",
		filesystem: map[string]string{
			"test_config_template.xml": "",
		},
		stubbedBuildDefinitions: []string{"libgtest_isolated_main", "liblog"},
		blueprint: `
cc_test {
	name: "mytest",
	srcs: ["test.cpp"],
	test_config_template: "test_config_template.xml",
	auto_gen_config: true,
	isolated: true,
}
` + simpleModule("cc_library_static", "libgtest_isolated_main") +
			simpleModule("cc_library", "liblog"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"auto_generate_test_config": "True",
				"local_includes":            `["."]`,
				"srcs":                      `["test.cpp"]`,
				"target_compatible_with":    `["//build/bazel/platforms/os:android"]`,
				"template_configs": `[
        "'<target_preparer class=\"com.android.tradefed.targetprep.RootTargetPreparer\">\\n        <option name=\"force-root\" value=\"false\" />\\n    </target_preparer>'",
        "'<option name=\"not-shardable\" value=\"true\" />'",
    ]`,
				"template_install_base": `"/data/local/tmp"`,
				"template_test_config":  `"test_config_template.xml"`,
				"deps":                  `[":libgtest_isolated_main"]`,
				"dynamic_deps":          `[":liblog"]`,
				"runs_on":               `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_WithExplicitGTestDepInAndroidBp(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description:             "cc test that lists libgtest in Android.bp should not have dups of libgtest in BUILD file",
		stubbedBuildDefinitions: []string{"libgtest_main", "libgtest"},
		blueprint: `
cc_test {
	name: "mytest",
	srcs: ["test.cpp"],
	static_libs: ["libgtest"],
}
` + simpleModule("cc_library_static", "libgtest_main") +
			simpleModule("cc_library_static", "libgtest"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["test.cpp"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"deps": `[
        ":libgtest",
        ":libgtest_main",
    ]`,
				"runs_on": `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})

}

func TestCcTest_WithIsolatedTurnedOn(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description:             "cc test that sets `isolated: true` should run with ligtest_isolated_main instead of libgtest_main",
		stubbedBuildDefinitions: []string{"libgtest_isolated_main", "liblog"},
		blueprint: `
cc_test {
	name: "mytest",
	srcs: ["test.cpp"],
	isolated: true,
}
` + simpleModule("cc_library_static", "libgtest_isolated_main") +
			simpleModule("cc_library", "liblog"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["test.cpp"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"deps":                   `[":libgtest_isolated_main"]`,
				"dynamic_deps":           `[":liblog"]`,
				"runs_on":                `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})

}

func TestCcTest_GtestExplicitlySpecifiedInAndroidBp(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description:             "If `gtest` is explicit in Android.bp, it should be explicit in BUILD files as well",
		stubbedBuildDefinitions: []string{"libgtest_main", "libgtest"},
		blueprint: `
cc_test {
	name: "mytest_with_gtest",
	gtest: true,
}
cc_test {
	name: "mytest_with_no_gtest",
	gtest: false,
}
` + simpleModule("cc_library_static", "libgtest_main") +
			simpleModule("cc_library_static", "libgtest"),
		targets: []testBazelTarget{
			{"cc_test", "mytest_with_gtest", AttrNameToString{
				"local_includes": `["."]`,
				"deps": `[
        ":libgtest_main",
        ":libgtest",
    ]`,
				"gtest":                  "True",
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"runs_on":                `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
			{"cc_test", "mytest_with_no_gtest", AttrNameToString{
				"local_includes":         `["."]`,
				"gtest":                  "False",
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"runs_on":                `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_DisableMemtagHeap(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description:             "cc test that disable memtag_heap",
		stubbedBuildDefinitions: []string{"libgtest_isolated_main", "liblog"},
		blueprint: `
cc_test {
	name: "mytest",
	srcs: ["test.cpp"],
	isolated: true,
	sanitize: {
		cfi: true,
		memtag_heap: false,
	},
}
` + simpleModule("cc_library_static", "libgtest_isolated_main") +
			simpleModule("cc_library", "liblog"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["test.cpp"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"deps":                   `[":libgtest_isolated_main"]`,
				"dynamic_deps":           `[":liblog"]`,
				"runs_on":                `["device"]`,
				"features": `["android_cfi"] + select({
        "//build/bazel/platforms/os_arch:android_arm64": ["-memtag_heap"],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_RespectArm64MemtagHeap(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description:             "cc test that disable memtag_heap",
		stubbedBuildDefinitions: []string{"libgtest_isolated_main", "liblog"},
		blueprint: `
cc_test {
	name: "mytest",
	srcs: ["test.cpp"],
	isolated: true,
	target: {
		android_arm64: {
			sanitize: {
				memtag_heap: false,
			}
		}
	},
}
` + simpleModule("cc_library_static", "libgtest_isolated_main") +
			simpleModule("cc_library", "liblog"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["test.cpp"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"deps":                   `[":libgtest_isolated_main"]`,
				"dynamic_deps":           `[":liblog"]`,
				"runs_on":                `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": ["-memtag_heap"],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_IgnoreNoneArm64MemtagHeap(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description:             "cc test that disable memtag_heap",
		stubbedBuildDefinitions: []string{"libgtest_isolated_main", "liblog"},
		blueprint: `
cc_test {
	name: "mytest",
	srcs: ["test.cpp"],
	isolated: true,
	arch: {
		x86: {
			sanitize: {
				memtag_heap: false,
			}
		}
	},
}
` + simpleModule("cc_library_static", "libgtest_isolated_main") +
			simpleModule("cc_library", "liblog"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["test.cpp"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"deps":                   `[":libgtest_isolated_main"]`,
				"dynamic_deps":           `[":liblog"]`,
				"runs_on":                `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "memtag_heap",
            "diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_Arm64MemtagHeapOverrideNoConfigOne(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description:             "cc test that disable memtag_heap",
		stubbedBuildDefinitions: []string{"libgtest_isolated_main", "liblog"},
		blueprint: `
cc_test {
	name: "mytest",
	srcs: ["test.cpp"],
	isolated: true,
	sanitize: {
		memtag_heap: true,
	},
	target: {
		android_arm64: {
			sanitize: {
				memtag_heap: false,
				diag: {
					memtag_heap: false,
				},
			}
		}
	},
}
` + simpleModule("cc_library_static", "libgtest_isolated_main") +
			simpleModule("cc_library", "liblog"),
		targets: []testBazelTarget{
			{"cc_test", "mytest", AttrNameToString{
				"local_includes":         `["."]`,
				"srcs":                   `["test.cpp"]`,
				"target_compatible_with": `["//build/bazel/platforms/os:android"]`,
				"deps":                   `[":libgtest_isolated_main"]`,
				"dynamic_deps":           `[":liblog"]`,
				"runs_on":                `["device"]`,
				"features": `select({
        "//build/bazel/platforms/os_arch:android_arm64": [
            "-memtag_heap",
            "-diag_memtag_heap",
        ],
        "//conditions:default": [],
    })`,
			},
			},
		},
	})
}

func TestCcTest_UnitTestFalse(t *testing.T) {
	runCcTestTestCase(t, ccTestBp2buildTestCase{
		description: "cc test with test_options.tags converted to tags",
		blueprint: `
cc_test {
    name: "mytest",
    host_supported: true,
    srcs: ["test.cpp"],
    test_options: { unit_test: false },
}
` + simpleModule("cc_library_static", "libgtest_main") +
			simpleModule("cc_library_static", "libgtest"),
		stubbedBuildDefinitions: []string{
			"libgtest_main",
			"libgtest",
		},
		targets: []testBazelTarget{},
	})
}
