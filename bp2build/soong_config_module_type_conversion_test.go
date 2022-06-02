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

package bp2build

import (
	"android/soong/android"
	"android/soong/cc"
	"testing"
)

func runSoongConfigModuleTypeTest(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, registerSoongConfigModuleTypes, tc)
}

func registerSoongConfigModuleTypes(ctx android.RegistrationContext) {
	cc.RegisterCCBuildComponents(ctx)

	ctx.RegisterModuleType("soong_config_module_type_import", android.SoongConfigModuleTypeImportFactory)
	ctx.RegisterModuleType("soong_config_module_type", android.SoongConfigModuleTypeFactory)
	ctx.RegisterModuleType("soong_config_string_variable", android.SoongConfigStringVariableDummyFactory)
	ctx.RegisterModuleType("soong_config_bool_variable", android.SoongConfigBoolVariableDummyFactory)

	ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
}

func TestSoongConfigModuleType(t *testing.T) {
	bp := `
soong_config_module_type {
	name: "custom_cc_library_static",
	module_type: "cc_library_static",
	config_namespace: "acme",
	bool_variables: ["feature1"],
	properties: ["cflags"],
}

custom_cc_library_static {
	name: "foo",
	bazel_module: { bp2build_available: true },
	host_supported: true,
	soong_config_variables: {
		feature1: {
			conditions_default: {
				cflags: ["-DDEFAULT1"],
			},
			cflags: ["-DFEATURE1"],
		},
	},
}
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - soong_config_module_type is supported in bp2build",
		moduleTypeUnderTest:        "cc_library_static",
		moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
		blueprint:                  bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "foo",
    copts = select({
        "//build/bazel/product_variables:acme__feature1": ["-DFEATURE1"],
        "//conditions:default": ["-DDEFAULT1"],
    }),
    local_includes = ["."],
)`}})
}

func TestSoongConfigModuleTypeImport(t *testing.T) {
	configBp := `
soong_config_module_type {
	name: "custom_cc_library_static",
	module_type: "cc_library_static",
	config_namespace: "acme",
	bool_variables: ["feature1"],
	properties: ["cflags"],
}
`
	bp := `
soong_config_module_type_import {
	from: "foo/bar/SoongConfig.bp",
	module_types: ["custom_cc_library_static"],
}

custom_cc_library_static {
	name: "foo",
	bazel_module: { bp2build_available: true },
	host_supported: true,
	soong_config_variables: {
		feature1: {
			conditions_default: {
				cflags: ["-DDEFAULT1"],
			},
			cflags: ["-DFEATURE1"],
		},
	},
}
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - soong_config_module_type_import is supported in bp2build",
		moduleTypeUnderTest:        "cc_library_static",
		moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
		filesystem: map[string]string{
			"foo/bar/SoongConfig.bp": configBp,
		},
		blueprint: bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "foo",
    copts = select({
        "//build/bazel/product_variables:acme__feature1": ["-DFEATURE1"],
        "//conditions:default": ["-DDEFAULT1"],
    }),
    local_includes = ["."],
)`}})
}

func TestSoongConfigModuleType_StringVar(t *testing.T) {
	bp := `
soong_config_string_variable {
	name: "board",
	values: ["soc_a", "soc_b", "soc_c"],
}

soong_config_module_type {
	name: "custom_cc_library_static",
	module_type: "cc_library_static",
	config_namespace: "acme",
	variables: ["board"],
	properties: ["cflags"],
}

custom_cc_library_static {
	name: "foo",
	bazel_module: { bp2build_available: true },
	host_supported: true,
	soong_config_variables: {
		board: {
			soc_a: {
				cflags: ["-DSOC_A"],
			},
			soc_b: {
				cflags: ["-DSOC_B"],
			},
			soc_c: {},
			conditions_default: {
				cflags: ["-DSOC_DEFAULT"]
			},
		},
	},
}
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - generates selects for string vars",
		moduleTypeUnderTest:        "cc_library_static",
		moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
		blueprint:                  bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "foo",
    copts = select({
        "//build/bazel/product_variables:acme__board__soc_a": ["-DSOC_A"],
        "//build/bazel/product_variables:acme__board__soc_b": ["-DSOC_B"],
        "//conditions:default": ["-DSOC_DEFAULT"],
    }),
    local_includes = ["."],
)`}})
}

func TestSoongConfigModuleType_StringAndBoolVar(t *testing.T) {
	bp := `
soong_config_bool_variable {
	name: "feature1",
}

soong_config_bool_variable {
	name: "feature2",
}

soong_config_string_variable {
	name: "board",
	values: ["soc_a", "soc_b", "soc_c"],
}

soong_config_module_type {
	name: "custom_cc_library_static",
	module_type: "cc_library_static",
	config_namespace: "acme",
	variables: ["feature1", "feature2", "board"],
	properties: ["cflags"],
}

custom_cc_library_static {
	name: "foo",
	bazel_module: { bp2build_available: true },
	host_supported: true,
	soong_config_variables: {
		feature1: {
			conditions_default: {
				cflags: ["-DDEFAULT1"],
			},
			cflags: ["-DFEATURE1"],
		},
		feature2: {
			cflags: ["-DFEATURE2"],
			conditions_default: {
				cflags: ["-DDEFAULT2"],
			},
		},
		board: {
			soc_a: {
				cflags: ["-DSOC_A"],
			},
			soc_b: {
				cflags: ["-DSOC_B"],
			},
			soc_c: {},
			conditions_default: {
				cflags: ["-DSOC_DEFAULT"]
			},
		},
	},
}`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - generates selects for multiple variable types",
		moduleTypeUnderTest:        "cc_library_static",
		moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
		blueprint:                  bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "foo",
    copts = select({
        "//build/bazel/product_variables:acme__board__soc_a": ["-DSOC_A"],
        "//build/bazel/product_variables:acme__board__soc_b": ["-DSOC_B"],
        "//conditions:default": ["-DSOC_DEFAULT"],
    }) + select({
        "//build/bazel/product_variables:acme__feature1": ["-DFEATURE1"],
        "//conditions:default": ["-DDEFAULT1"],
    }) + select({
        "//build/bazel/product_variables:acme__feature2": ["-DFEATURE2"],
        "//conditions:default": ["-DDEFAULT2"],
    }),
    local_includes = ["."],
)`}})
}

func TestSoongConfigModuleType_StringVar_LabelListDeps(t *testing.T) {
	bp := `
soong_config_string_variable {
	name: "board",
	values: ["soc_a", "soc_b", "soc_c"],
}

soong_config_module_type {
	name: "custom_cc_library_static",
	module_type: "cc_library_static",
	config_namespace: "acme",
	variables: ["board"],
	properties: ["cflags", "static_libs"],
}

custom_cc_library_static {
	name: "foo",
	bazel_module: { bp2build_available: true },
	host_supported: true,
	soong_config_variables: {
		board: {
			soc_a: {
				cflags: ["-DSOC_A"],
				static_libs: ["soc_a_dep"],
			},
			soc_b: {
				cflags: ["-DSOC_B"],
				static_libs: ["soc_b_dep"],
			},
			soc_c: {},
			conditions_default: {
				cflags: ["-DSOC_DEFAULT"],
				static_libs: ["soc_default_static_dep"],
			},
		},
	},
}`

	otherDeps := `
cc_library_static { name: "soc_a_dep", bazel_module: { bp2build_available: false } }
cc_library_static { name: "soc_b_dep", bazel_module: { bp2build_available: false } }
cc_library_static { name: "soc_default_static_dep", bazel_module: { bp2build_available: false } }
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - generates selects for label list attributes",
		moduleTypeUnderTest:        "cc_library_static",
		moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
		blueprint:                  bp,
		filesystem: map[string]string{
			"foo/bar/Android.bp": otherDeps,
		},
		expectedBazelTargets: []string{`cc_library_static(
    name = "foo",
    copts = select({
        "//build/bazel/product_variables:acme__board__soc_a": ["-DSOC_A"],
        "//build/bazel/product_variables:acme__board__soc_b": ["-DSOC_B"],
        "//conditions:default": ["-DSOC_DEFAULT"],
    }),
    implementation_deps = select({
        "//build/bazel/product_variables:acme__board__soc_a": ["//foo/bar:soc_a_dep"],
        "//build/bazel/product_variables:acme__board__soc_b": ["//foo/bar:soc_b_dep"],
        "//conditions:default": ["//foo/bar:soc_default_static_dep"],
    }),
    local_includes = ["."],
)`}})
}

func TestSoongConfigModuleType_Defaults_SingleNamespace(t *testing.T) {
	bp := `
soong_config_module_type {
	name: "vendor_foo_cc_defaults",
	module_type: "cc_defaults",
	config_namespace: "vendor_foo",
	bool_variables: ["feature"],
	properties: ["cflags", "cppflags"],
}

vendor_foo_cc_defaults {
	name: "foo_defaults_1",
	soong_config_variables: {
		feature: {
			cflags: ["-cflag_feature_1"],
			conditions_default: {
				cflags: ["-cflag_default_1"],
			},
		},
	},
}

vendor_foo_cc_defaults {
	name: "foo_defaults_2",
	defaults: ["foo_defaults_1"],
	soong_config_variables: {
		feature: {
			cflags: ["-cflag_feature_2"],
			conditions_default: {
				cflags: ["-cflag_default_2"],
			},
		},
	},
}

cc_library_static {
	name: "lib",
	defaults: ["foo_defaults_2"],
	bazel_module: { bp2build_available: true },
	host_supported: true,
}
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - defaults with a single namespace",
		moduleTypeUnderTest:        "cc_library_static",
		moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
		blueprint:                  bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "lib",
    copts = select({
        "//build/bazel/product_variables:vendor_foo__feature": [
            "-cflag_feature_2",
            "-cflag_feature_1",
        ],
        "//conditions:default": [
            "-cflag_default_2",
            "-cflag_default_1",
        ],
    }),
    local_includes = ["."],
)`}})
}

func TestSoongConfigModuleType_MultipleDefaults_SingleNamespace(t *testing.T) {
	bp := `
soong_config_module_type {
	name: "foo_cc_defaults",
	module_type: "cc_defaults",
	config_namespace: "acme",
	bool_variables: ["feature"],
	properties: ["cflags"],
}

soong_config_module_type {
	name: "bar_cc_defaults",
	module_type: "cc_defaults",
	config_namespace: "acme",
	bool_variables: ["feature"],
	properties: ["cflags", "asflags"],
}

foo_cc_defaults {
	name: "foo_defaults",
	soong_config_variables: {
		feature: {
			cflags: ["-cflag_foo"],
			conditions_default: {
				cflags: ["-cflag_default_foo"],
			},
		},
	},
}

bar_cc_defaults {
	name: "bar_defaults",
	srcs: ["file.S"],
	soong_config_variables: {
		feature: {
			cflags: ["-cflag_bar"],
			asflags: ["-asflag_bar"],
			conditions_default: {
				asflags: ["-asflag_default_bar"],
				cflags: ["-cflag_default_bar"],
			},
		},
	},
}

cc_library_static {
	name: "lib",
	defaults: ["foo_defaults", "bar_defaults"],
	bazel_module: { bp2build_available: true },
	host_supported: true,
}

cc_library_static {
	name: "lib2",
	defaults: ["bar_defaults", "foo_defaults"],
	bazel_module: { bp2build_available: true },
	host_supported: true,
}
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - multiple defaults with a single namespace",
		moduleTypeUnderTest:        "cc_library_static",
		moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
		blueprint:                  bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "lib",
    asflags = select({
        "//build/bazel/product_variables:acme__feature": ["-asflag_bar"],
        "//conditions:default": ["-asflag_default_bar"],
    }),
    copts = select({
        "//build/bazel/product_variables:acme__feature": [
            "-cflag_foo",
            "-cflag_bar",
        ],
        "//conditions:default": [
            "-cflag_default_foo",
            "-cflag_default_bar",
        ],
    }),
    local_includes = ["."],
    srcs_as = ["file.S"],
)`,
			`cc_library_static(
    name = "lib2",
    asflags = select({
        "//build/bazel/product_variables:acme__feature": ["-asflag_bar"],
        "//conditions:default": ["-asflag_default_bar"],
    }),
    copts = select({
        "//build/bazel/product_variables:acme__feature": [
            "-cflag_bar",
            "-cflag_foo",
        ],
        "//conditions:default": [
            "-cflag_default_bar",
            "-cflag_default_foo",
        ],
    }),
    local_includes = ["."],
    srcs_as = ["file.S"],
)`}})
}

func TestSoongConfigModuleType_Defaults_MultipleNamespaces(t *testing.T) {
	bp := `
soong_config_module_type {
	name: "vendor_foo_cc_defaults",
	module_type: "cc_defaults",
	config_namespace: "vendor_foo",
	bool_variables: ["feature"],
	properties: ["cflags"],
}

soong_config_module_type {
	name: "vendor_bar_cc_defaults",
	module_type: "cc_defaults",
	config_namespace: "vendor_bar",
	bool_variables: ["feature"],
	properties: ["cflags"],
}

soong_config_module_type {
	name: "vendor_qux_cc_defaults",
	module_type: "cc_defaults",
	config_namespace: "vendor_qux",
	bool_variables: ["feature"],
	properties: ["cflags"],
}

vendor_foo_cc_defaults {
	name: "foo_defaults",
	soong_config_variables: {
		feature: {
			cflags: ["-DVENDOR_FOO_FEATURE"],
			conditions_default: {
				cflags: ["-DVENDOR_FOO_DEFAULT"],
			},
		},
	},
}

vendor_bar_cc_defaults {
	name: "bar_defaults",
	soong_config_variables: {
		feature: {
			cflags: ["-DVENDOR_BAR_FEATURE"],
			conditions_default: {
				cflags: ["-DVENDOR_BAR_DEFAULT"],
			},
		},
	},
}

vendor_qux_cc_defaults {
	name: "qux_defaults",
	defaults: ["bar_defaults"],
	soong_config_variables: {
		feature: {
			cflags: ["-DVENDOR_QUX_FEATURE"],
			conditions_default: {
				cflags: ["-DVENDOR_QUX_DEFAULT"],
			},
		},
	},
}

cc_library_static {
	name: "lib",
	defaults: ["foo_defaults", "qux_defaults"],
	bazel_module: { bp2build_available: true },
	host_supported: true,
}
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - defaults with multiple namespaces",
		moduleTypeUnderTest:        "cc_library_static",
		moduleTypeUnderTestFactory: cc.LibraryStaticFactory,
		blueprint:                  bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "lib",
    copts = select({
        "//build/bazel/product_variables:vendor_bar__feature": ["-DVENDOR_BAR_FEATURE"],
        "//conditions:default": ["-DVENDOR_BAR_DEFAULT"],
    }) + select({
        "//build/bazel/product_variables:vendor_foo__feature": ["-DVENDOR_FOO_FEATURE"],
        "//conditions:default": ["-DVENDOR_FOO_DEFAULT"],
    }) + select({
        "//build/bazel/product_variables:vendor_qux__feature": ["-DVENDOR_QUX_FEATURE"],
        "//conditions:default": ["-DVENDOR_QUX_DEFAULT"],
    }),
    local_includes = ["."],
)`}})
}

func TestSoongConfigModuleType_Defaults(t *testing.T) {
	bp := `
soong_config_string_variable {
    name: "library_linking_strategy",
    values: [
        "prefer_static",
    ],
}

soong_config_module_type {
    name: "library_linking_strategy_cc_defaults",
    module_type: "cc_defaults",
    config_namespace: "ANDROID",
    variables: ["library_linking_strategy"],
    properties: [
        "shared_libs",
        "static_libs",
    ],
}

library_linking_strategy_cc_defaults {
    name: "library_linking_strategy_lib_a_defaults",
    soong_config_variables: {
        library_linking_strategy: {
            prefer_static: {
                static_libs: [
                    "lib_a",
                ],
            },
            conditions_default: {
                shared_libs: [
                    "lib_a",
                ],
            },
        },
    },
}

library_linking_strategy_cc_defaults {
    name: "library_linking_strategy_merged_defaults",
    defaults: ["library_linking_strategy_lib_a_defaults"],
    host_supported: true,
    soong_config_variables: {
        library_linking_strategy: {
            prefer_static: {
                static_libs: [
                    "lib_b",
                ],
            },
            conditions_default: {
                shared_libs: [
                    "lib_b",
                ],
            },
        },
    },
}

cc_binary {
    name: "library_linking_strategy_sample_binary",
    srcs: ["library_linking_strategy.cc"],
    defaults: ["library_linking_strategy_merged_defaults"],
}`

	otherDeps := `
cc_library { name: "lib_a", bazel_module: { bp2build_available: false } }
cc_library { name: "lib_b", bazel_module: { bp2build_available: false } }
cc_library { name: "lib_default", bazel_module: { bp2build_available: false } }
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - generates selects for library_linking_strategy",
		moduleTypeUnderTest:        "cc_binary",
		moduleTypeUnderTestFactory: cc.BinaryFactory,
		blueprint:                  bp,
		filesystem: map[string]string{
			"foo/bar/Android.bp": otherDeps,
		},
		expectedBazelTargets: []string{`cc_binary(
    name = "library_linking_strategy_sample_binary",
    deps = select({
        "//build/bazel/product_variables:android__library_linking_strategy__prefer_static": [
            "//foo/bar:lib_b_bp2build_cc_library_static",
            "//foo/bar:lib_a_bp2build_cc_library_static",
        ],
        "//conditions:default": [],
    }),
    dynamic_deps = select({
        "//build/bazel/product_variables:android__library_linking_strategy__prefer_static": [],
        "//conditions:default": [
            "//foo/bar:lib_b",
            "//foo/bar:lib_a",
        ],
    }),
    local_includes = ["."],
    srcs = ["library_linking_strategy.cc"],
)`}})
}

func TestSoongConfigModuleType_Defaults_Another(t *testing.T) {
	bp := `
soong_config_string_variable {
    name: "library_linking_strategy",
    values: [
        "prefer_static",
    ],
}

soong_config_module_type {
    name: "library_linking_strategy_cc_defaults",
    module_type: "cc_defaults",
    config_namespace: "ANDROID",
    variables: ["library_linking_strategy"],
    properties: [
        "shared_libs",
        "static_libs",
    ],
}

library_linking_strategy_cc_defaults {
    name: "library_linking_strategy_sample_defaults",
    soong_config_variables: {
        library_linking_strategy: {
            prefer_static: {
                static_libs: [
                    "lib_a",
                    "lib_b",
                ],
            },
            conditions_default: {
                shared_libs: [
                    "lib_a",
                    "lib_b",
                ],
            },
        },
    },
}

cc_binary {
    name: "library_linking_strategy_sample_binary",
    host_supported: true,
    srcs: ["library_linking_strategy.cc"],
    defaults: ["library_linking_strategy_sample_defaults"],
}`

	otherDeps := `
cc_library { name: "lib_a", bazel_module: { bp2build_available: false } }
cc_library { name: "lib_b", bazel_module: { bp2build_available: false } }
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - generates selects for library_linking_strategy",
		moduleTypeUnderTest:        "cc_binary",
		moduleTypeUnderTestFactory: cc.BinaryFactory,
		blueprint:                  bp,
		filesystem: map[string]string{
			"foo/bar/Android.bp": otherDeps,
		},
		expectedBazelTargets: []string{`cc_binary(
    name = "library_linking_strategy_sample_binary",
    deps = select({
        "//build/bazel/product_variables:android__library_linking_strategy__prefer_static": [
            "//foo/bar:lib_a_bp2build_cc_library_static",
            "//foo/bar:lib_b_bp2build_cc_library_static",
        ],
        "//conditions:default": [],
    }),
    dynamic_deps = select({
        "//build/bazel/product_variables:android__library_linking_strategy__prefer_static": [],
        "//conditions:default": [
            "//foo/bar:lib_a",
            "//foo/bar:lib_b",
        ],
    }),
    local_includes = ["."],
    srcs = ["library_linking_strategy.cc"],
)`}})
}

func TestSoongConfigModuleType_Defaults_UnusedProps(t *testing.T) {
	bp := `
soong_config_string_variable {
    name: "alphabet",
    values: [
        "a",
        "b",
        "c", // unused
    ],
}

soong_config_module_type {
    name: "alphabet_cc_defaults",
    module_type: "cc_defaults",
    config_namespace: "ANDROID",
    variables: ["alphabet"],
    properties: [
        "cflags", // unused
        "shared_libs",
        "static_libs",
    ],
}

alphabet_cc_defaults {
    name: "alphabet_sample_cc_defaults",
    soong_config_variables: {
        alphabet: {
            a: {
                shared_libs: [
                    "lib_a",
                ],
            },
            b: {
                shared_libs: [
                    "lib_b",
                ],
            },
            conditions_default: {
                static_libs: [
                    "lib_default",
                ],
            },
        },
    },
}

cc_binary {
    name: "alphabet_binary",
    host_supported: true,
    srcs: ["main.cc"],
    defaults: ["alphabet_sample_cc_defaults"],
}`

	otherDeps := `
cc_library { name: "lib_a", bazel_module: { bp2build_available: false } }
cc_library { name: "lib_b", bazel_module: { bp2build_available: false } }
cc_library { name: "lib_default", bazel_module: { bp2build_available: false } }
`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - generates selects for library_linking_strategy",
		moduleTypeUnderTest:        "cc_binary",
		moduleTypeUnderTestFactory: cc.BinaryFactory,
		blueprint:                  bp,
		filesystem: map[string]string{
			"foo/bar/Android.bp": otherDeps,
		},
		expectedBazelTargets: []string{`cc_binary(
    name = "alphabet_binary",
    deps = select({
        "//build/bazel/product_variables:android__alphabet__a": [],
        "//build/bazel/product_variables:android__alphabet__b": [],
        "//conditions:default": ["//foo/bar:lib_default_bp2build_cc_library_static"],
    }),
    dynamic_deps = select({
        "//build/bazel/product_variables:android__alphabet__a": ["//foo/bar:lib_a"],
        "//build/bazel/product_variables:android__alphabet__b": ["//foo/bar:lib_b"],
        "//conditions:default": [],
    }),
    local_includes = ["."],
    srcs = ["main.cc"],
)`}})
}

func TestSoongConfigModuleType_ProductVariableConfigWithPlatformConfig(t *testing.T) {
	bp := `
soong_config_bool_variable {
    name: "special_build",
}

soong_config_module_type {
    name: "alphabet_cc_defaults",
    module_type: "cc_defaults",
    config_namespace: "alphabet_module",
    bool_variables: ["special_build"],
    properties: ["enabled"],
}

alphabet_cc_defaults {
    name: "alphabet_sample_cc_defaults",
    soong_config_variables: {
        special_build: {
            enabled: true,
        },
    },
}

cc_binary {
    name: "alphabet_binary",
    srcs: ["main.cc"],
    host_supported: true,
    defaults: ["alphabet_sample_cc_defaults"],
    enabled: false,
    arch: {
        x86_64: {
            enabled: false,
        },
    },
    target: {
        darwin: {
            enabled: false,
        },
    },
}`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - generates selects for library_linking_strategy",
		moduleTypeUnderTest:        "cc_binary",
		moduleTypeUnderTestFactory: cc.BinaryFactory,
		blueprint:                  bp,
		filesystem:                 map[string]string{},
		expectedBazelTargets: []string{`cc_binary(
    name = "alphabet_binary",
    local_includes = ["."],
    srcs = ["main.cc"],
    target_compatible_with = ["//build/bazel/product_variables:alphabet_module__special_build"] + select({
        "//build/bazel/platforms/os_arch:android_x86_64": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:darwin_arm64": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:darwin_x86_64": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:linux_bionic_x86_64": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:linux_glibc_x86_64": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:linux_musl_x86_64": ["@platforms//:incompatible"],
        "//build/bazel/platforms/os_arch:windows_x86_64": ["@platforms//:incompatible"],
        "//conditions:default": [],
    }),
)`}})
}

func TestSoongConfigModuleType_ProductVariableConfigOverridesEnable(t *testing.T) {
	bp := `
soong_config_bool_variable {
    name: "special_build",
}

soong_config_module_type {
    name: "alphabet_cc_defaults",
    module_type: "cc_defaults",
    config_namespace: "alphabet_module",
    bool_variables: ["special_build"],
    properties: ["enabled"],
}

alphabet_cc_defaults {
    name: "alphabet_sample_cc_defaults",
    soong_config_variables: {
        special_build: {
            enabled: true,
        },
    },
}

cc_binary {
    name: "alphabet_binary",
    srcs: ["main.cc"],
    defaults: ["alphabet_sample_cc_defaults"],
    enabled: false,
}`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - generates selects for library_linking_strategy",
		moduleTypeUnderTest:        "cc_binary",
		moduleTypeUnderTestFactory: cc.BinaryFactory,
		blueprint:                  bp,
		filesystem:                 map[string]string{},
		expectedBazelTargets: []string{`cc_binary(
    name = "alphabet_binary",
    local_includes = ["."],
    srcs = ["main.cc"],
    target_compatible_with = ["//build/bazel/product_variables:alphabet_module__special_build"],
)`}})
}

func TestSoongConfigModuleType_ProductVariableIgnoredIfEnabledByDefault(t *testing.T) {
	bp := `
soong_config_bool_variable {
    name: "special_build",
}

soong_config_module_type {
    name: "alphabet_cc_defaults",
    module_type: "cc_defaults",
    config_namespace: "alphabet_module",
    bool_variables: ["special_build"],
    properties: ["enabled"],
}

alphabet_cc_defaults {
    name: "alphabet_sample_cc_defaults",
    host_supported: true,
    soong_config_variables: {
        special_build: {
            enabled: true,
        },
    },
}

cc_binary {
    name: "alphabet_binary",
    srcs: ["main.cc"],
    defaults: ["alphabet_sample_cc_defaults"],
}`

	runSoongConfigModuleTypeTest(t, bp2buildTestCase{
		description:                "soong config variables - generates selects for library_linking_strategy",
		moduleTypeUnderTest:        "cc_binary",
		moduleTypeUnderTestFactory: cc.BinaryFactory,
		blueprint:                  bp,
		filesystem:                 map[string]string{},
		expectedBazelTargets: []string{`cc_binary(
    name = "alphabet_binary",
    local_includes = ["."],
    srcs = ["main.cc"],
)`}})
}
