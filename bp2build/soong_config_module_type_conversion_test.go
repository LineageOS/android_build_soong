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
}

func TestSoongConfigModuleType(t *testing.T) {
	bp := `
soong_config_module_type {
	name: "custom_cc_library_static",
	module_type: "cc_library_static",
	config_namespace: "acme",
	bool_variables: ["feature1"],
	properties: ["cflags"],
	bazel_module: { bp2build_available: true },
}

custom_cc_library_static {
	name: "foo",
	bazel_module: { bp2build_available: true },
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
		description:                        "soong config variables - soong_config_module_type is supported in bp2build",
		moduleTypeUnderTest:                "cc_library_static",
		moduleTypeUnderTestFactory:         cc.LibraryStaticFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.CcLibraryStaticBp2Build,
		blueprint:                          bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "foo",
    copts = select({
        "//build/bazel/product_variables:acme__feature1__enabled": ["-DFEATURE1"],
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
	bazel_module: { bp2build_available: true },
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
		description:                        "soong config variables - soong_config_module_type_import is supported in bp2build",
		moduleTypeUnderTest:                "cc_library_static",
		moduleTypeUnderTestFactory:         cc.LibraryStaticFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.CcLibraryStaticBp2Build,
		filesystem: map[string]string{
			"foo/bar/SoongConfig.bp": configBp,
		},
		blueprint: bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "foo",
    copts = select({
        "//build/bazel/product_variables:acme__feature1__enabled": ["-DFEATURE1"],
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
	bazel_module: { bp2build_available: true },
}

custom_cc_library_static {
	name: "foo",
	bazel_module: { bp2build_available: true },
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
		description:                        "soong config variables - generates selects for string vars",
		moduleTypeUnderTest:                "cc_library_static",
		moduleTypeUnderTestFactory:         cc.LibraryStaticFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.CcLibraryStaticBp2Build,
		blueprint:                          bp,
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
	bazel_module: { bp2build_available: true },
}

custom_cc_library_static {
	name: "foo",
	bazel_module: { bp2build_available: true },
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
		description:                        "soong config variables - generates selects for multiple variable types",
		moduleTypeUnderTest:                "cc_library_static",
		moduleTypeUnderTestFactory:         cc.LibraryStaticFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.CcLibraryStaticBp2Build,
		blueprint:                          bp,
		expectedBazelTargets: []string{`cc_library_static(
    name = "foo",
    copts = select({
        "//build/bazel/product_variables:acme__board__soc_a": ["-DSOC_A"],
        "//build/bazel/product_variables:acme__board__soc_b": ["-DSOC_B"],
        "//conditions:default": ["-DSOC_DEFAULT"],
    }) + select({
        "//build/bazel/product_variables:acme__feature1__enabled": ["-DFEATURE1"],
        "//conditions:default": ["-DDEFAULT1"],
    }) + select({
        "//build/bazel/product_variables:acme__feature2__enabled": ["-DFEATURE2"],
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
	bazel_module: { bp2build_available: true },
}

custom_cc_library_static {
	name: "foo",
	bazel_module: { bp2build_available: true },
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
		description:                        "soong config variables - generates selects for label list attributes",
		moduleTypeUnderTest:                "cc_library_static",
		moduleTypeUnderTestFactory:         cc.LibraryStaticFactory,
		moduleTypeUnderTestBp2BuildMutator: cc.CcLibraryStaticBp2Build,
		blueprint:                          bp,
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
