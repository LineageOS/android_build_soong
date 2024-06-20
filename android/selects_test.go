// Copyright 2024 Google Inc. All rights reserved.
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
	"reflect"
	"testing"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func TestSelects(t *testing.T) {
	testCases := []struct {
		name           string
		bp             string
		provider       selectsTestProvider
		providers      map[string]selectsTestProvider
		vendorVars     map[string]map[string]string
		vendorVarTypes map[string]map[string]string
		expectedError  string
	}{
		{
			name: "basic string list",
			bp: `
			my_module_type {
				name: "foo",
				my_string_list: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a.cpp"],
					"b": ["b.cpp"],
					default: ["c.cpp"],
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string_list: &[]string{"c.cpp"},
			},
		},
		{
			name: "basic string",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": "a.cpp",
					"b": "b.cpp",
					default: "c.cpp",
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("c.cpp"),
			},
		},
		{
			name: "basic bool",
			bp: `
			my_module_type {
				name: "foo",
				my_bool: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": true,
					"b": false,
					default: true,
				}),
			}
			`,
			provider: selectsTestProvider{
				my_bool: proptools.BoolPtr(true),
			},
		},
		{
			name: "basic paths",
			bp: `
			my_module_type {
				name: "foo",
				my_paths: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["foo.txt"],
					"b": ["bar.txt"],
					default: ["baz.txt"],
				}),
			}
			`,
			provider: selectsTestProvider{
				my_paths: &[]string{"baz.txt"},
			},
		},
		{
			name: "paths with module references",
			bp: `
			my_module_type {
				name: "foo",
				my_paths: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": [":a"],
					"b": [":b"],
					default: [":c"],
				}),
			}
			`,
			expectedError: `"foo" depends on undefined module "c"`,
		},
		{
			name: "Differing types",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": "a.cpp",
					"b": true,
					default: "c.cpp",
				}),
			}
			`,
			expectedError: `Android.bp:8:5: Found select statement with differing types "string" and "bool" in its cases`,
		},
		{
			name: "Select type doesn't match property type",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": false,
					"b": true,
					default: true,
				}),
			}
			`,
			expectedError: `can't assign bool value to string property "my_string\[0\]"`,
		},
		{
			name: "String list non-default",
			bp: `
			my_module_type {
				name: "foo",
				my_string_list: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a.cpp"],
					"b": ["b.cpp"],
					default: ["c.cpp"],
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string_list: &[]string{"a.cpp"},
			},
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable": "a",
				},
			},
		},
		{
			name: "String list append",
			bp: `
			my_module_type {
				name: "foo",
				my_string_list: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a.cpp"],
					"b": ["b.cpp"],
					default: ["c.cpp"],
				}) + select(soong_config_variable("my_namespace", "my_variable_2"), {
					"a2": ["a2.cpp"],
					"b2": ["b2.cpp"],
					default: ["c2.cpp"],
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string_list: &[]string{"a.cpp", "c2.cpp"},
			},
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable": "a",
				},
			},
		},
		{
			name: "String list prepend literal",
			bp: `
			my_module_type {
				name: "foo",
				my_string_list: ["literal.cpp"] + select(soong_config_variable("my_namespace", "my_variable"), {
					"a2": ["a2.cpp"],
					"b2": ["b2.cpp"],
					default: ["c2.cpp"],
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string_list: &[]string{"literal.cpp", "c2.cpp"},
			},
		},
		{
			name: "String list append literal",
			bp: `
			my_module_type {
				name: "foo",
				my_string_list: select(soong_config_variable("my_namespace", "my_variable"), {
					"a2": ["a2.cpp"],
					"b2": ["b2.cpp"],
					default: ["c2.cpp"],
				}) + ["literal.cpp"],
			}
			`,
			provider: selectsTestProvider{
				my_string_list: &[]string{"c2.cpp", "literal.cpp"},
			},
		},
		{
			name: "true + false = true",
			bp: `
			my_module_type {
				name: "foo",
				my_bool: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": true,
					"b": false,
					default: true,
				}) + false,
			}
			`,
			provider: selectsTestProvider{
				my_bool: proptools.BoolPtr(true),
			},
		},
		{
			name: "false + false = false",
			bp: `
			my_module_type {
				name: "foo",
				my_bool: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": true,
					"b": false,
					default: true,
				}) + false,
			}
			`,
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable": "b",
				},
			},
			provider: selectsTestProvider{
				my_bool: proptools.BoolPtr(false),
			},
		},
		{
			name: "Append string",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": "a",
					"b": "b",
					default: "c",
				}) + ".cpp",
			}
			`,
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("c.cpp"),
			},
		},
		{
			name: "Select on arch",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(arch(), {
					"x86": "my_x86",
					"x86_64": "my_x86_64",
					"arm": "my_arm",
					"arm64": "my_arm64",
					default: "my_default",
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("my_arm64"),
			},
		},
		{
			name: "Select on os",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(os(), {
					"android": "my_android",
					"linux": "my_linux",
					default: "my_default",
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("my_android"),
			},
		},
		{
			name: "Unset value",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": unset,
					"b": "b",
					default: "c",
				})
			}
			`,
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable": "a",
				},
			},
			provider: selectsTestProvider{},
		},
		{
			name: "Unset value on different branch",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": unset,
					"b": "b",
					default: "c",
				})
			}
			`,
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("c"),
			},
		},
		{
			name: "unset + unset = unset",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"foo": "bar",
					default: unset,
				}) + select(soong_config_variable("my_namespace", "my_variable2"), {
					"baz": "qux",
					default: unset,
				})
			}
			`,
			provider: selectsTestProvider{},
		},
		{
			name: "unset + string = string",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"foo": "bar",
					default: unset,
				}) + select(soong_config_variable("my_namespace", "my_variable2"), {
					default: "a",
				})
			}
			`,
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("a"),
			},
		},
		{
			name: "unset + bool = bool",
			bp: `
			my_module_type {
				name: "foo",
				my_bool: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": true,
					default: unset,
				}) + select(soong_config_variable("my_namespace", "my_variable2"), {
					default: true,
				})
			}
			`,
			provider: selectsTestProvider{
				my_bool: proptools.BoolPtr(true),
			},
		},
		{
			name: "defaults with lists are appended",
			bp: `
			my_module_type {
				name: "foo",
				defaults: ["bar"],
				my_string_list: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a1"],
					default: ["b1"],
				}),
			}
			my_defaults {
				name: "bar",
				my_string_list: select(soong_config_variable("my_namespace", "my_variable2"), {
					"a": ["a2"],
					default: ["b2"],
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string_list: &[]string{"b2", "b1"},
			},
		},
		{
			name: "defaults applied to multiple modules",
			bp: `
			my_module_type {
				name: "foo2",
				defaults: ["bar"],
				my_string_list: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a1"],
					default: ["b1"],
				}),
			}
			my_module_type {
				name: "foo",
				defaults: ["bar"],
				my_string_list: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a1"],
					default: ["b1"],
				}),
			}
			my_defaults {
				name: "bar",
				my_string_list: select(soong_config_variable("my_namespace", "my_variable2"), {
					"a": ["a2"],
					default: ["b2"],
				}),
			}
			`,
			providers: map[string]selectsTestProvider{
				"foo": {
					my_string_list: &[]string{"b2", "b1"},
				},
				"foo2": {
					my_string_list: &[]string{"b2", "b1"},
				},
			},
		},
		{
			name: "Replacing string list",
			bp: `
			my_module_type {
				name: "foo",
				defaults: ["bar"],
				replacing_string_list: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a1"],
					default: ["b1"],
				}),
			}
			my_defaults {
				name: "bar",
				replacing_string_list: select(soong_config_variable("my_namespace", "my_variable2"), {
					"a": ["a2"],
					default: ["b2"],
				}),
			}
			`,
			provider: selectsTestProvider{
				replacing_string_list: &[]string{"b1"},
			},
		},
		{
			name: "Multi-condition string 1",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select((
					soong_config_variable("my_namespace", "my_variable"),
					soong_config_variable("my_namespace", "my_variable2"),
				), {
					("a", "b"): "a+b",
					("a", default): "a+default",
					(default, default): "default",
				}),
			}
			`,
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable":  "a",
					"my_variable2": "b",
				},
			},
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("a+b"),
			},
		},
		{
			name: "Multi-condition string 2",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select((
					soong_config_variable("my_namespace", "my_variable"),
					soong_config_variable("my_namespace", "my_variable2"),
				), {
					("a", "b"): "a+b",
					("a", default): "a+default",
					(default, default): "default",
				}),
			}
			`,
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable":  "a",
					"my_variable2": "c",
				},
			},
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("a+default"),
			},
		},
		{
			name: "Multi-condition string 3",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select((
					soong_config_variable("my_namespace", "my_variable"),
					soong_config_variable("my_namespace", "my_variable2"),
				), {
					("a", "b"): "a+b",
					("a", default): "a+default",
					(default, default): "default",
				}),
			}
			`,
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable":  "c",
					"my_variable2": "b",
				},
			},
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("default"),
			},
		},
		{
			name: "Unhandled string value",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"foo": "a",
					"bar": "b",
				}),
			}
			`,
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable": "baz",
				},
			},
			expectedError: `my_string: soong_config_variable\("my_namespace", "my_variable"\) had value "baz", which was not handled by the select statement`,
		},
		{
			name: "Select on boolean",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(boolean_var_for_testing(), {
					true: "t",
					false: "f",
				}),
			}
			`,
			vendorVars: map[string]map[string]string{
				"boolean_var": {
					"for_testing": "true",
				},
			},
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("t"),
			},
		},
		{
			name: "Select on boolean soong config variable",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					true: "t",
					false: "f",
				}),
			}
			`,
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable": "true",
				},
			},
			vendorVarTypes: map[string]map[string]string{
				"my_namespace": {
					"my_variable": "bool",
				},
			},
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("t"),
			},
		},
		{
			name: "Select on boolean false",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(boolean_var_for_testing(), {
					true: "t",
					false: "f",
				}),
			}
			`,
			vendorVars: map[string]map[string]string{
				"boolean_var": {
					"for_testing": "false",
				},
			},
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("f"),
			},
		},
		{
			name: "Select on boolean undefined",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(boolean_var_for_testing(), {
					true: "t",
					false: "f",
				}),
			}
			`,
			expectedError: `my_string: boolean_var_for_testing\(\) had value undefined, which was not handled by the select statement`,
		},
		{
			name: "Select on boolean undefined with default",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(boolean_var_for_testing(), {
					true: "t",
					false: "f",
					default: "default",
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("default"),
			},
		},
		{
			name: "Mismatched condition types",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(boolean_var_for_testing(), {
					"true": "t",
					"false": "f",
					default: "default",
				}),
			}
			`,
			vendorVars: map[string]map[string]string{
				"boolean_var": {
					"for_testing": "false",
				},
			},
			expectedError: "Expected all branches of a select on condition boolean_var_for_testing\\(\\) to have type bool, found string",
		},
		{
			name: "Assigning select to nonconfigurable bool",
			bp: `
			my_module_type {
				name: "foo",
				my_nonconfigurable_bool: select(arch(), {
					"x86_64": true,
					default: false,
				}),
			}
			`,
			expectedError: `can't assign select statement to non-configurable property "my_nonconfigurable_bool"`,
		},
		{
			name: "Assigning select to nonconfigurable string",
			bp: `
			my_module_type {
				name: "foo",
				my_nonconfigurable_string: select(arch(), {
					"x86_64": "x86!",
					default: "unknown!",
				}),
			}
			`,
			expectedError: `can't assign select statement to non-configurable property "my_nonconfigurable_string"`,
		},
		{
			name: "Assigning appended selects to nonconfigurable string",
			bp: `
			my_module_type {
				name: "foo",
				my_nonconfigurable_string: select(arch(), {
					"x86_64": "x86!",
					default: "unknown!",
				}) + select(os(), {
					"darwin": "_darwin!",
					default: "unknown!",
				}),
			}
			`,
			expectedError: `can't assign select statement to non-configurable property "my_nonconfigurable_string"`,
		},
		{
			name: "Assigning select to nonconfigurable string list",
			bp: `
			my_module_type {
				name: "foo",
				my_nonconfigurable_string_list: select(arch(), {
					"x86_64": ["foo", "bar"],
					default: ["baz", "qux"],
				}),
			}
			`,
			expectedError: `can't assign select statement to non-configurable property "my_nonconfigurable_string_list"`,
		},
		{
			name: "Select in variable",
			bp: `
			my_second_variable = ["after.cpp"]
			my_variable = select(soong_config_variable("my_namespace", "my_variable"), {
				"a": ["a.cpp"],
				"b": ["b.cpp"],
				default: ["c.cpp"],
			}) + my_second_variable
			my_module_type {
				name: "foo",
				my_string_list: ["before.cpp"] + my_variable,
			}
			`,
			provider: selectsTestProvider{
				my_string_list: &[]string{"before.cpp", "a.cpp", "after.cpp"},
			},
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable": "a",
				},
			},
		},
		{
			name: "Soong config value variable on configurable property",
			bp: `
			soong_config_module_type {
				name: "soong_config_my_module_type",
				module_type: "my_module_type",
				config_namespace: "my_namespace",
				value_variables: ["my_variable"],
				properties: ["my_string", "my_string_list"],
			}

			soong_config_my_module_type {
				name: "foo",
				my_string_list: ["before.cpp"],
				soong_config_variables: {
					my_variable: {
						my_string_list: ["after_%s.cpp"],
						my_string: "%s.cpp",
					},
				},
			}
			`,
			provider: selectsTestProvider{
				my_string:      proptools.StringPtr("foo.cpp"),
				my_string_list: &[]string{"before.cpp", "after_foo.cpp"},
			},
			vendorVars: map[string]map[string]string{
				"my_namespace": {
					"my_variable": "foo",
				},
			},
		},
		{
			name: "Property appending with variable",
			bp: `
			my_variable = ["b.cpp"]
			my_module_type {
				name: "foo",
				my_string_list: ["a.cpp"] + my_variable + select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a.cpp"],
					"b": ["b.cpp"],
					default: ["c.cpp"],
				}),
			}
			`,
			provider: selectsTestProvider{
				my_string_list: &[]string{"a.cpp", "b.cpp", "c.cpp"},
			},
		},
		{
			name: "Test AppendSimpleValue",
			bp: `
			my_module_type {
				name: "foo",
				my_string_list: ["a.cpp"] + select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a.cpp"],
					"b": ["b.cpp"],
					default: ["c.cpp"],
				}),
			}
			`,
			vendorVars: map[string]map[string]string{
				"selects_test": {
					"append_to_string_list": "foo.cpp",
				},
			},
			provider: selectsTestProvider{
				my_string_list: &[]string{"a.cpp", "c.cpp", "foo.cpp"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixtures := GroupFixturePreparers(
				PrepareForTestWithDefaults,
				PrepareForTestWithArchMutator,
				PrepareForTestWithSoongConfigModuleBuildComponents,
				FixtureRegisterWithContext(func(ctx RegistrationContext) {
					ctx.RegisterModuleType("my_module_type", newSelectsMockModule)
					ctx.RegisterModuleType("my_defaults", newSelectsMockModuleDefaults)
				}),
				FixtureModifyProductVariables(func(variables FixtureProductVariables) {
					variables.VendorVars = tc.vendorVars
					variables.VendorVarTypes = tc.vendorVarTypes
				}),
			)
			if tc.expectedError != "" {
				fixtures = fixtures.ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(tc.expectedError))
			}
			result := fixtures.RunTestWithBp(t, tc.bp)

			if tc.expectedError == "" {
				if len(tc.providers) == 0 {
					tc.providers = map[string]selectsTestProvider{
						"foo": tc.provider,
					}
				}

				for moduleName := range tc.providers {
					expected := tc.providers[moduleName]
					m := result.ModuleForTests(moduleName, "android_arm64_armv8-a")
					p, _ := OtherModuleProvider(result.testContext.OtherModuleProviderAdaptor(), m.Module(), selectsTestProviderKey)
					if !reflect.DeepEqual(p, expected) {
						t.Errorf("Expected:\n  %q\ngot:\n  %q", expected.String(), p.String())
					}
				}
			}
		})
	}
}

type selectsTestProvider struct {
	my_bool                        *bool
	my_string                      *string
	my_string_list                 *[]string
	my_paths                       *[]string
	replacing_string_list          *[]string
	my_nonconfigurable_bool        *bool
	my_nonconfigurable_string      *string
	my_nonconfigurable_string_list []string
}

func (p *selectsTestProvider) String() string {
	myBoolStr := "nil"
	if p.my_bool != nil {
		myBoolStr = fmt.Sprintf("%t", *p.my_bool)
	}
	myStringStr := "nil"
	if p.my_string != nil {
		myStringStr = *p.my_string
	}
	myNonconfigurableStringStr := "nil"
	if p.my_nonconfigurable_string != nil {
		myNonconfigurableStringStr = *p.my_nonconfigurable_string
	}
	return fmt.Sprintf(`selectsTestProvider {
	my_bool: %v,
	my_string: %s,
    my_string_list: %s,
    my_paths: %s,
	replacing_string_list %s,
	my_nonconfigurable_bool: %v,
	my_nonconfigurable_string: %s,
	my_nonconfigurable_string_list: %s,
}`,
		myBoolStr,
		myStringStr,
		p.my_string_list,
		p.my_paths,
		p.replacing_string_list,
		p.my_nonconfigurable_bool,
		myNonconfigurableStringStr,
		p.my_nonconfigurable_string_list,
	)
}

var selectsTestProviderKey = blueprint.NewProvider[selectsTestProvider]()

type selectsMockModuleProperties struct {
	My_bool                        proptools.Configurable[bool]
	My_string                      proptools.Configurable[string]
	My_string_list                 proptools.Configurable[[]string]
	My_paths                       proptools.Configurable[[]string] `android:"path"`
	Replacing_string_list          proptools.Configurable[[]string] `android:"replace_instead_of_append,arch_variant"`
	My_nonconfigurable_bool        *bool
	My_nonconfigurable_string      *string
	My_nonconfigurable_string_list []string
}

type selectsMockModule struct {
	ModuleBase
	DefaultableModuleBase
	properties selectsMockModuleProperties
}

func optionalToPtr[T any](o proptools.ConfigurableOptional[T]) *T {
	if o.IsEmpty() {
		return nil
	}
	x := o.Get()
	return &x
}

func (p *selectsMockModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	toAppend := ctx.Config().VendorConfig("selects_test").String("append_to_string_list")
	if toAppend != "" {
		p.properties.My_string_list.AppendSimpleValue([]string{toAppend})
	}
	SetProvider(ctx, selectsTestProviderKey, selectsTestProvider{
		my_bool:                        optionalToPtr(p.properties.My_bool.Get(ctx)),
		my_string:                      optionalToPtr(p.properties.My_string.Get(ctx)),
		my_string_list:                 optionalToPtr(p.properties.My_string_list.Get(ctx)),
		my_paths:                       optionalToPtr(p.properties.My_paths.Get(ctx)),
		replacing_string_list:          optionalToPtr(p.properties.Replacing_string_list.Get(ctx)),
		my_nonconfigurable_bool:        p.properties.My_nonconfigurable_bool,
		my_nonconfigurable_string:      p.properties.My_nonconfigurable_string,
		my_nonconfigurable_string_list: p.properties.My_nonconfigurable_string_list,
	})
}

func newSelectsMockModule() Module {
	m := &selectsMockModule{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibFirst)
	InitDefaultableModule(m)
	return m
}

type selectsMockModuleDefaults struct {
	ModuleBase
	DefaultsModuleBase
}

func (d *selectsMockModuleDefaults) GenerateAndroidBuildActions(ctx ModuleContext) {
}

func newSelectsMockModuleDefaults() Module {
	module := &selectsMockModuleDefaults{}

	module.AddProperties(
		&selectsMockModuleProperties{},
	)

	InitDefaultsModule(module)

	return module
}
