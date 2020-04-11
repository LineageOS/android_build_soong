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
	"reflect"
	"testing"
)

type soongConfigTestModule struct {
	ModuleBase
	props soongConfigTestModuleProperties
}

type soongConfigTestModuleProperties struct {
	Cflags []string
}

func soongConfigTestModuleFactory() Module {
	m := &soongConfigTestModule{}
	m.AddProperties(&m.props)
	InitAndroidModule(m)
	return m
}

func (t soongConfigTestModule) GenerateAndroidBuildActions(ModuleContext) {}

func TestSoongConfigModule(t *testing.T) {
	configBp := `
		soong_config_module_type {
			name: "acme_test_defaults",
			module_type: "test_defaults",
			config_namespace: "acme",
			variables: ["board", "feature1", "FEATURE3"],
			bool_variables: ["feature2"],
			value_variables: ["size"],
			properties: ["cflags", "srcs"],
		}

		soong_config_string_variable {
			name: "board",
			values: ["soc_a", "soc_b"],
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
			module_types: ["acme_test_defaults"],
		}
	`

	bp := `
		acme_test_defaults {
			name: "foo",
			cflags: ["-DGENERIC"],
			soong_config_variables: {
				board: {
					soc_a: {
						cflags: ["-DSOC_A"],
					},
					soc_b: {
						cflags: ["-DSOC_B"],
					},
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

	run := func(t *testing.T, bp string, fs map[string][]byte) {
		config := TestConfig(buildDir, nil, bp, fs)

		config.TestProductVariables.VendorVars = map[string]map[string]string{
			"acme": map[string]string{
				"board":    "soc_a",
				"size":     "42",
				"feature1": "true",
				"feature2": "false",
				// FEATURE3 unset
			},
		}

		ctx := NewTestContext()
		ctx.RegisterModuleType("soong_config_module_type_import", soongConfigModuleTypeImportFactory)
		ctx.RegisterModuleType("soong_config_module_type", soongConfigModuleTypeFactory)
		ctx.RegisterModuleType("soong_config_string_variable", soongConfigStringVariableDummyFactory)
		ctx.RegisterModuleType("soong_config_bool_variable", soongConfigBoolVariableDummyFactory)
		ctx.RegisterModuleType("test_defaults", soongConfigTestModuleFactory)
		ctx.Register(config)

		_, errs := ctx.ParseBlueprintsFiles("Android.bp")
		FailIfErrored(t, errs)
		_, errs = ctx.PrepareBuildActions(config)
		FailIfErrored(t, errs)

		foo := ctx.ModuleForTests("foo", "").Module().(*soongConfigTestModule)
		if g, w := foo.props.Cflags, []string{"-DGENERIC", "-DSIZE=42", "-DSOC_A", "-DFEATURE1"}; !reflect.DeepEqual(g, w) {
			t.Errorf("wanted foo cflags %q, got %q", w, g)
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
