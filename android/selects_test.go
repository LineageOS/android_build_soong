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
		name          string
		bp            string
		provider      selectsTestProvider
		vendorVars    map[string]map[string]string
		expectedError string
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
			name: "Select on variant",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(variant("arch"), {
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
					default: unset,
				}) + select(soong_config_variable("my_namespace", "my_variable2"), {
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixtures := GroupFixturePreparers(
				PrepareForTestWithDefaults,
				PrepareForTestWithArchMutator,
				FixtureRegisterWithContext(func(ctx RegistrationContext) {
					ctx.RegisterModuleType("my_module_type", newSelectsMockModule)
					ctx.RegisterModuleType("my_defaults", newSelectsMockModuleDefaults)
				}),
				FixtureModifyProductVariables(func(variables FixtureProductVariables) {
					variables.VendorVars = tc.vendorVars
				}),
			)
			if tc.expectedError != "" {
				fixtures = fixtures.ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(tc.expectedError))
			}
			result := fixtures.RunTestWithBp(t, tc.bp)

			if tc.expectedError == "" {
				m := result.ModuleForTests("foo", "android_arm64_armv8-a")
				p, _ := OtherModuleProvider(result.testContext.OtherModuleProviderAdaptor(), m.Module(), selectsTestProviderKey)
				if !reflect.DeepEqual(p, tc.provider) {
					t.Errorf("Expected:\n  %q\ngot:\n  %q", tc.provider.String(), p.String())
				}
			}
		})
	}
}

type selectsTestProvider struct {
	my_bool               *bool
	my_string             *string
	my_string_list        *[]string
	my_paths              *[]string
	replacing_string_list *[]string
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
	return fmt.Sprintf(`selectsTestProvider {
	my_bool: %v,
	my_string: %s,
    my_string_list: %s,
    my_paths: %s,
	replacing_string_list %s,
}`, myBoolStr, myStringStr, p.my_string_list, p.my_paths, p.replacing_string_list)
}

var selectsTestProviderKey = blueprint.NewProvider[selectsTestProvider]()

type selectsMockModuleProperties struct {
	My_bool               proptools.Configurable[bool]
	My_string             proptools.Configurable[string]
	My_string_list        proptools.Configurable[[]string]
	My_paths              proptools.Configurable[[]string] `android:"path"`
	Replacing_string_list proptools.Configurable[[]string] `android:"replace_instead_of_append,arch_variant"`
}

type selectsMockModule struct {
	ModuleBase
	DefaultableModuleBase
	properties selectsMockModuleProperties
}

func (p *selectsMockModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	SetProvider(ctx, selectsTestProviderKey, selectsTestProvider{
		my_bool:               p.properties.My_bool.Get(ctx),
		my_string:             p.properties.My_string.Get(ctx),
		my_string_list:        p.properties.My_string_list.Get(ctx),
		my_paths:              p.properties.My_paths.Get(ctx),
		replacing_string_list: p.properties.Replacing_string_list.Get(ctx),
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
