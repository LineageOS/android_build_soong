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
	"fmt"
	"path/filepath"
	"testing"
)

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
	props      soongConfigTestModuleProperties
	outputPath ModuleOutPath
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

func (t *soongConfigTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	t.outputPath = PathForModuleOut(ctx, "test")
}

var prepareForSoongConfigTestModule = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.RegisterModuleType("test_defaults", soongConfigTestDefaultsModuleFactory)
	ctx.RegisterModuleType("test", soongConfigTestModuleFactory)
})

func TestSoongConfigModule(t *testing.T) {
	configBp := `
		soong_config_module_type {
			name: "acme_test",
			module_type: "test",
			config_namespace: "acme",
			variables: ["board", "feature1", "FEATURE3", "unused_string_var"],
			bool_variables: ["feature2", "unused_feature", "always_true"],
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

		test_defaults {
			name: "foo_defaults_always_true",
			cflags: ["DEFAULT_ALWAYS_TRUE"],
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
				always_true: {
					defaults: ["foo_defaults_always_true"],
					conditions_default: {
						// verify that conditions_default is skipped if the
						// soong config variable is true by specifying a
						// non-existent module in conditions_default
						defaults: ["//nonexistent:defaults"],
					}
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
						"always_true":       "true",
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
					"DEFAULT_ALWAYS_TRUE",
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
					"acme": {
						"board":       "soc_c",
						"always_true": "true",
					}}),
				fooExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
					"-DF2_CONDITIONS_DEFAULT",
					"-DSIZE=CONDITIONS_DEFAULT",
					"-DF1_CONDITIONS_DEFAULT",
				},
				fooDefaultsExpectedFlags: []string{
					"DEFAULT_ALWAYS_TRUE",
					"DEFAULT",
					"-DGENERIC",
				},
			},
			{
				name: "unused_string_var",
				preparer: fixtureForVendorVars(map[string]map[string]string{
					"acme": {
						"board":       "soc_d",
						"always_true": "true",
					}}),
				fooExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
					"-DF2_CONDITIONS_DEFAULT",
					"-DSIZE=CONDITIONS_DEFAULT",
					"-DSOC_CONDITIONS_DEFAULT", // foo does not contain a prop "soc_d", so we use the default
					"-DF1_CONDITIONS_DEFAULT",
				},
				fooDefaultsExpectedFlags: []string{
					"DEFAULT_ALWAYS_TRUE",
					"DEFAULT",
					"-DGENERIC",
				},
			},

			{
				name: "conditions_default",
				preparer: fixtureForVendorVars(map[string]map[string]string{
					"acme": {
						"always_true": "true",
					}}),
				fooExpectedFlags: []string{
					"DEFAULT",
					"-DGENERIC",
					"-DF2_CONDITIONS_DEFAULT",
					"-DSIZE=CONDITIONS_DEFAULT",
					"-DSOC_CONDITIONS_DEFAULT",
					"-DF1_CONDITIONS_DEFAULT",
				},
				fooDefaultsExpectedFlags: []string{
					"DEFAULT_ALWAYS_TRUE",
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
					PrepareForTestWithSoongConfigModuleBuildComponents,
					prepareForSoongConfigTestModule,
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

func TestNonExistentPropertyInSoongConfigModule(t *testing.T) {
	bp := `
		soong_config_module_type {
			name: "acme_test",
			module_type: "test",
			config_namespace: "acme",
			bool_variables: ["feature1"],
			properties: ["made_up_property"],
		}

		acme_test {
			name: "foo",
			cflags: ["-DGENERIC"],
			soong_config_variables: {
				feature1: {
					made_up_property: true,
				},
			},
		}
    `

	fixtureForVendorVars := func(vars map[string]map[string]string) FixturePreparer {
		return FixtureModifyProductVariables(func(variables FixtureProductVariables) {
			variables.VendorVars = vars
		})
	}

	GroupFixturePreparers(
		fixtureForVendorVars(map[string]map[string]string{"acme": {"feature1": "1"}}),
		PrepareForTestWithDefaults,
		PrepareForTestWithSoongConfigModuleBuildComponents,
		prepareForSoongConfigTestModule,
		FixtureWithRootAndroidBp(bp),
	).ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern([]string{
		// TODO(b/171232169): improve the error message for non-existent properties
		`unrecognized property "soong_config_variables`,
	})).RunTest(t)
}

func TestDuplicateStringValueInSoongConfigStringVariable(t *testing.T) {
	bp := `
		soong_config_string_variable {
			name: "board",
			values: ["soc_a", "soc_b", "soc_c", "soc_a"],
		}

		soong_config_module_type {
			name: "acme_test",
			module_type: "test",
			config_namespace: "acme",
			variables: ["board"],
			properties: ["cflags", "srcs", "defaults"],
		}
    `

	fixtureForVendorVars := func(vars map[string]map[string]string) FixturePreparer {
		return FixtureModifyProductVariables(func(variables FixtureProductVariables) {
			variables.VendorVars = vars
		})
	}

	GroupFixturePreparers(
		fixtureForVendorVars(map[string]map[string]string{"acme": {"feature1": "1"}}),
		PrepareForTestWithDefaults,
		PrepareForTestWithSoongConfigModuleBuildComponents,
		prepareForSoongConfigTestModule,
		FixtureWithRootAndroidBp(bp),
	).ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern([]string{
		// TODO(b/171232169): improve the error message for non-existent properties
		`Android.bp: soong_config_string_variable: values property error: duplicate value: "soc_a"`,
	})).RunTest(t)
}

type soongConfigTestSingletonModule struct {
	SingletonModuleBase
	props soongConfigTestSingletonModuleProperties
}

type soongConfigTestSingletonModuleProperties struct {
	Fragments []struct {
		Apex   string
		Module string
	}
}

func soongConfigTestSingletonModuleFactory() SingletonModule {
	m := &soongConfigTestSingletonModule{}
	m.AddProperties(&m.props)
	InitAndroidModule(m)
	return m
}

func (t *soongConfigTestSingletonModule) GenerateAndroidBuildActions(ModuleContext) {}

func (t *soongConfigTestSingletonModule) GenerateSingletonBuildActions(SingletonContext) {}

var prepareForSoongConfigTestSingletonModule = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.RegisterSingletonModuleType("test_singleton", soongConfigTestSingletonModuleFactory)
})

func TestSoongConfigModuleSingletonModule(t *testing.T) {
	bp := `
		soong_config_module_type {
			name: "acme_test_singleton",
			module_type: "test_singleton",
			config_namespace: "acme",
			bool_variables: ["coyote"],
			properties: ["fragments"],
		}

		acme_test_singleton {
			name: "wiley",
			fragments: [
				{
					apex: "com.android.acme",
					module: "road-runner",
				},
			],
			soong_config_variables: {
				coyote: {
					fragments: [
						{
							apex: "com.android.acme",
							module: "wiley",
						},
					],
				},
			},
		}
	`

	for _, test := range []struct {
		coyote            bool
		expectedFragments string
	}{
		{
			coyote:            false,
			expectedFragments: "[{Apex:com.android.acme Module:road-runner}]",
		},
		{
			coyote:            true,
			expectedFragments: "[{Apex:com.android.acme Module:road-runner} {Apex:com.android.acme Module:wiley}]",
		},
	} {
		t.Run(fmt.Sprintf("coyote:%t", test.coyote), func(t *testing.T) {
			result := GroupFixturePreparers(
				PrepareForTestWithSoongConfigModuleBuildComponents,
				prepareForSoongConfigTestSingletonModule,
				FixtureWithRootAndroidBp(bp),
				FixtureModifyProductVariables(func(variables FixtureProductVariables) {
					variables.VendorVars = map[string]map[string]string{
						"acme": {
							"coyote": fmt.Sprintf("%t", test.coyote),
						},
					}
				}),
			).RunTest(t)

			// Make sure that the singleton was created.
			result.SingletonForTests("test_singleton")
			m := result.ModuleForTests("wiley", "").module.(*soongConfigTestSingletonModule)
			AssertStringEquals(t, "fragments", test.expectedFragments, fmt.Sprintf("%+v", m.props.Fragments))
		})
	}
}

func TestSoongConfigModuleTrace(t *testing.T) {
	bp := `
		soong_config_module_type {
			name: "acme_test",
			module_type: "test",
			config_namespace: "acme",
			variables: ["board", "feature1", "FEATURE3", "unused_string_var"],
			bool_variables: ["feature2", "unused_feature", "always_true"],
			value_variables: ["size", "unused_size"],
			properties: ["cflags", "srcs", "defaults"],
		}

		soong_config_module_type {
			name: "acme_test_defaults",
			module_type: "test_defaults",
			config_namespace: "acme",
			variables: ["board", "feature1", "FEATURE3", "unused_string_var"],
			bool_variables: ["feature2", "unused_feature", "always_true"],
			value_variables: ["size", "unused_size"],
			properties: ["cflags", "srcs", "defaults"],
		}

		soong_config_string_variable {
			name: "board",
			values: ["soc_a", "soc_b", "soc_c"],
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

		test_defaults {
			name: "test_defaults",
			cflags: ["DEFAULT"],
		}

		test {
			name: "normal",
			defaults: ["test_defaults"],
		}

		acme_test {
			name: "board_1",
			defaults: ["test_defaults"],
			soong_config_variables: {
				board: {
					soc_a: {
						cflags: ["-DSOC_A"],
					},
				},
			},
		}

		acme_test {
			name: "board_2",
			defaults: ["test_defaults"],
			soong_config_variables: {
				board: {
					soc_a: {
						cflags: ["-DSOC_A"],
					},
				},
			},
		}

		acme_test {
			name: "size",
			defaults: ["test_defaults"],
			soong_config_variables: {
				size: {
					cflags: ["-DSIZE=%s"],
				},
			},
		}

		acme_test {
			name: "board_and_size",
			defaults: ["test_defaults"],
			soong_config_variables: {
				board: {
					soc_a: {
						cflags: ["-DSOC_A"],
					},
				},
				size: {
					cflags: ["-DSIZE=%s"],
				},
			},
		}

		acme_test_defaults {
			name: "board_defaults",
			soong_config_variables: {
				board: {
					soc_a: {
						cflags: ["-DSOC_A"],
					},
				},
			},
		}

		acme_test_defaults {
			name: "size_defaults",
			soong_config_variables: {
				size: {
					cflags: ["-DSIZE=%s"],
				},
			},
		}

		test {
			name: "board_and_size_with_defaults",
			defaults: ["board_defaults", "size_defaults"],
		}
    `

	fixtureForVendorVars := func(vars map[string]map[string]string) FixturePreparer {
		return FixtureModifyProductVariables(func(variables FixtureProductVariables) {
			variables.VendorVars = vars
		})
	}

	preparer := fixtureForVendorVars(map[string]map[string]string{
		"acme": {
			"board":    "soc_a",
			"size":     "42",
			"feature1": "true",
			"feature2": "false",
			// FEATURE3 unset
			"unused_feature":    "true", // unused
			"unused_size":       "1",    // unused
			"unused_string_var": "a",    // unused
			"always_true":       "true",
		},
	})

	t.Run("soong config trace hash", func(t *testing.T) {
		result := GroupFixturePreparers(
			preparer,
			PrepareForTestWithDefaults,
			PrepareForTestWithSoongConfigModuleBuildComponents,
			prepareForSoongConfigTestModule,
			FixtureRegisterWithContext(func(ctx RegistrationContext) {
				ctx.FinalDepsMutators(registerSoongConfigTraceMutator)
			}),
			FixtureWithRootAndroidBp(bp),
		).RunTest(t)

		// Hashes of modules not using soong config should be empty
		normal := result.ModuleForTests("normal", "").Module().(*soongConfigTestModule)
		AssertDeepEquals(t, "normal hash", normal.base().commonProperties.SoongConfigTraceHash, "")
		AssertDeepEquals(t, "normal hash out", normal.outputPath.RelativeToTop().String(), "out/soong/.intermediates/normal/test")

		board1 := result.ModuleForTests("board_1", "").Module().(*soongConfigTestModule)
		board2 := result.ModuleForTests("board_2", "").Module().(*soongConfigTestModule)
		size := result.ModuleForTests("size", "").Module().(*soongConfigTestModule)

		// Trace mutator sets soong config trace hash correctly
		board1Hash := board1.base().commonProperties.SoongConfigTrace.hash()
		board1Output := board1.outputPath.RelativeToTop().String()
		AssertDeepEquals(t, "board hash calc", board1Hash, board1.base().commonProperties.SoongConfigTraceHash)
		AssertDeepEquals(t, "board hash path", board1Output, filepath.Join("out/soong/.intermediates/board_1", board1Hash, "test"))

		sizeHash := size.base().commonProperties.SoongConfigTrace.hash()
		sizeOutput := size.outputPath.RelativeToTop().String()
		AssertDeepEquals(t, "size hash calc", sizeHash, size.base().commonProperties.SoongConfigTraceHash)
		AssertDeepEquals(t, "size hash path", sizeOutput, filepath.Join("out/soong/.intermediates/size", sizeHash, "test"))

		// Trace should be identical for modules using the same set of variables
		AssertDeepEquals(t, "board trace", board1.base().commonProperties.SoongConfigTrace, board2.base().commonProperties.SoongConfigTrace)
		AssertDeepEquals(t, "board hash", board1.base().commonProperties.SoongConfigTraceHash, board2.base().commonProperties.SoongConfigTraceHash)

		// Trace hash should be different for different sets of soong variables
		AssertBoolEquals(t, "board hash not equal to size hash", board1.base().commonProperties.SoongConfigTraceHash == size.commonProperties.SoongConfigTraceHash, false)

		boardSize := result.ModuleForTests("board_and_size", "").Module().(*soongConfigTestModule)
		boardSizeDefaults := result.ModuleForTests("board_and_size_with_defaults", "").Module()

		// Trace should propagate
		AssertDeepEquals(t, "board_size hash calc", boardSize.base().commonProperties.SoongConfigTrace.hash(), boardSize.base().commonProperties.SoongConfigTraceHash)
		AssertDeepEquals(t, "board_size trace", boardSize.base().commonProperties.SoongConfigTrace, boardSizeDefaults.base().commonProperties.SoongConfigTrace)
		AssertDeepEquals(t, "board_size hash", boardSize.base().commonProperties.SoongConfigTraceHash, boardSizeDefaults.base().commonProperties.SoongConfigTraceHash)
	})
}
