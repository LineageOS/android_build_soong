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

package android

import (
	"testing"
)

type soongConfigTestDefaultsModuleProperties struct {
}

type soongConfigTestDefaultsModule struct {
	ModuleBase
	DefaultsModuleBase
}

func soongConfigTestDefaultsModuleFactory() Module {
	m := &soongConfigTestDefaultsModule{}
	m.AddProperties(&soongConfigTestModuleProperties{})
	InitDefaultsModule(m)
	return m
}

type soongConfigTestModule struct {
	ModuleBase
	DefaultableModuleBase
	props soongConfigTestModuleProperties
}

type soongConfigTestModuleProperties struct {
	Cflags []string
}

func soongConfigTestModuleFactory() Module {
	m := &soongConfigTestModule{}
	m.AddProperties(&m.props)
	InitAndroidModule(m)
	InitDefaultableModule(m)
	return m
}

func (t soongConfigTestModule) GenerateAndroidBuildActions(ModuleContext) {}

func TestSoongConfigModule(t *testing.T) {
	configBp := `
		soong_config_module_type {
			name: "acme_test",
			module_type: "test",
			config_namespace: "acme",
			variables: ["board", "feature1", "FEATURE3", "unused_string_var"],
			bool_variables: ["feature2", "unused_feature"],
			value_variables: ["size", "unused_size"],
			properties: ["cflags", "srcs", "defaults"],
		}

		soong_config_string_variable {
			name: "board",
			values: ["soc_a", "soc_b", "soc_c", "soc_d"],
		}

		soong_config_string_variable {
			name: "unused_string_var",
			values: ["a", "b"],
		}

		soong_config_bool_variable {
			name: "feature1",
		}

		soong_config_bool_variable {
			name: "FEATURE3",
		}
	`

	importBp := `
		soong_config_module_type_import {
			from: "SoongConfig.bp",
			module_types: ["acme_test"],
		}
	`

	bp := `
		test_defaults {
			name: "foo_defaults",
			cflags: ["DEFAULT"],
		}

		acme_test {
			name: "foo",
			cflags: ["-DGENERIC"],
			defaults: ["foo_defaults"],
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
						cflags: ["-DSOC_CONDITIONS_DEFAULT"],
					},
				},
				size: {
					cflags: ["-DSIZE=%s"],
					conditions_default: {
						cflags: ["-DSIZE=CONDITIONS_DEFAULT"],
					},
				},
				feature1: {
					  conditions_default: {
						  cflags: ["-DF1_CONDITIONS_DEFAULT"],
					  },
					cflags: ["-DFEATURE1"],
				},
				feature2: {
					cflags: ["-DFEATURE2"],
					 conditions_default: {
						 cflags: ["-DF2_CONDITIONS_DEFAULT"],
					 },
				},
				FEATURE3: {
					cflags: ["-DFEATURE3"],
				},
			},
		}

		test_defaults {
			name: "foo_defaults_a",
			cflags: ["DEFAULT_A"],
		}

		test_defaults {
			name: "foo_defaults_b",
			cflags: ["DEFAULT_B"],
		}

		acme_test {
			name: "foo_with_defaults",
			cflags: ["-DGENERIC"],
			defaults: ["foo_defaults"],
			soong_config_variables: {
				board: {
					soc_a: {
						cflags: ["-DSOC_A"],
						defaults: ["foo_defaults_a"],
					},
					soc_b: {
						cflags: ["-DSOC_B"],
						defaults: ["foo_defaults_b"],
					},
					soc_c: {},
				},
				size: {
					cflags: ["-DSIZE=%s"],
				},
				feature1: {
					cflags: ["-DFEATURE1"],
				},
				feature2: {
					cflags: ["-DFEATURE2"],
				},
				FEATURE3: {
					cflags: ["-DFEATURE3"],
				},
			},
		}
    `

	fixtureForVendorVars := func(vars map[string]map[string]string) FixturePreparer {
		return FixtureModifyProductVariables(func(variables FixtureProductVariables) {
			variables.VendorVars = vars
		})
	}

	run := func(t *testing.T, bp string, fs MockFS) {
		testCases := []struct {
			name                     string
			preparer                 FixturePreparer
			fooExpectedFlags         []string
			fooDefaultsExpectedFlags []string
		}{
			{
				name: "withValues",
				preparer: fixtureForVendorVars(map[string]map[string]string{
					"acme": {
						"board":    "soc_a",
						"size":     "42",
						"feature1": "true",
						"feature2": "false",
						// FEATURE3 unset
						"unused_feature":    "true", // unused
						"unused_size":       "1",    // unused
						"unused_string_var": "a",    // unused
					},
				}),
				fooExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
					"-DF2_CONDITIONS_DEFAULT",
					"-DSIZE=42",
					"-DSOC_A",
					"-DFEATURE1",
				},
				fooDefaultsExpectedFlags: []string{
					"DEFAULT_A",
					"DEFAULT",
					"-DGENERIC",
					"-DSIZE=42",
					"-DSOC_A",
					"-DFEATURE1",
				},
			},
			{
				name: "empty_prop_for_string_var",
				preparer: fixtureForVendorVars(map[string]map[string]string{
					"acme": {"board": "soc_c"}}),
				fooExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
					"-DF2_CONDITIONS_DEFAULT",
					"-DSIZE=CONDITIONS_DEFAULT",
					"-DF1_CONDITIONS_DEFAULT",
				},
				fooDefaultsExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
				},
			},
			{
				name: "unused_string_var",
				preparer: fixtureForVendorVars(map[string]map[string]string{
					"acme": {"board": "soc_d"}}),
				fooExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
					"-DF2_CONDITIONS_DEFAULT",
					"-DSIZE=CONDITIONS_DEFAULT",
					"-DSOC_CONDITIONS_DEFAULT", // foo does not contain a prop "soc_d", so we use the default
					"-DF1_CONDITIONS_DEFAULT",
				},
				fooDefaultsExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
				},
			},

			{
				name:     "conditions_default",
				preparer: fixtureForVendorVars(map[string]map[string]string{}),
				fooExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
					"-DF2_CONDITIONS_DEFAULT",
					"-DSIZE=CONDITIONS_DEFAULT",
					"-DSOC_CONDITIONS_DEFAULT",
					"-DF1_CONDITIONS_DEFAULT",
				},
				fooDefaultsExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := GroupFixturePreparers(
					tc.preparer,
					PrepareForTestWithDefaults,
					FixtureRegisterWithContext(func(ctx RegistrationContext) {
						ctx.RegisterModuleType("soong_config_module_type_import", soongConfigModuleTypeImportFactory)
						ctx.RegisterModuleType("soong_config_module_type", soongConfigModuleTypeFactory)
						ctx.RegisterModuleType("soong_config_string_variable", soongConfigStringVariableDummyFactory)
						ctx.RegisterModuleType("soong_config_bool_variable", soongConfigBoolVariableDummyFactory)
						ctx.RegisterModuleType("test_defaults", soongConfigTestDefaultsModuleFactory)
						ctx.RegisterModuleType("test", soongConfigTestModuleFactory)
					}),
					fs.AddToFixture(),
					FixtureWithRootAndroidBp(bp),
				).RunTest(t)

				foo := result.ModuleForTests("foo", "").Module().(*soongConfigTestModule)
				AssertDeepEquals(t, "foo cflags", tc.fooExpectedFlags, foo.props.Cflags)

				fooDefaults := result.ModuleForTests("foo_with_defaults", "").Module().(*soongConfigTestModule)
				AssertDeepEquals(t, "foo_with_defaults cflags", tc.fooDefaultsExpectedFlags, fooDefaults.props.Cflags)
			})
		}
	}

	t.Run("single file", func(t *testing.T) {
		run(t, configBp+bp, nil)
	})

	t.Run("import", func(t *testing.T) {
		run(t, importBp+bp, map[string][]byte{
			"SoongConfig.bp": []byte(configBp),
		})
	})
}

func testConfigWithVendorVars(buildDir, bp string, fs map[string][]byte, vendorVars map[string]map[string]string) Config {
	config := TestConfig(buildDir, nil, bp, fs)

	config.TestProductVariables.VendorVars = vendorVars

	return config
}
