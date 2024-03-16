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
					_: ["c.cpp"],
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
					_: "c.cpp",
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
					_: true,
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
					_: ["baz.txt"],
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
					_: [":c"],
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
					_: "c.cpp",
				}),
			}
			`,
			expectedError: `can't assign bool value to string property "my_string\[1\]"`,
		},
		{
			name: "String list non-default",
			bp: `
			my_module_type {
				name: "foo",
				my_string_list: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": ["a.cpp"],
					"b": ["b.cpp"],
					_: ["c.cpp"],
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
					_: ["c.cpp"],
				}) + select(soong_config_variable("my_namespace", "my_variable_2"), {
					"a2": ["a2.cpp"],
					"b2": ["b2.cpp"],
					_: ["c2.cpp"],
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
					_: ["c2.cpp"],
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
					_: ["c2.cpp"],
				}) + ["literal.cpp"],
			}
			`,
			provider: selectsTestProvider{
				my_string_list: &[]string{"c2.cpp", "literal.cpp"},
			},
		},
		{
			name: "Can't append bools",
			bp: `
			my_module_type {
				name: "foo",
				my_bool: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": true,
					"b": false,
					_: true,
				}) + false,
			}
			`,
			expectedError: "my_bool: Cannot append bools",
		},
		{
			name: "Append string",
			bp: `
			my_module_type {
				name: "foo",
				my_string: select(soong_config_variable("my_namespace", "my_variable"), {
					"a": "a",
					"b": "b",
					_: "c",
				}) + ".cpp",
			}
			`,
			provider: selectsTestProvider{
				my_string: proptools.StringPtr("c.cpp"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixtures := GroupFixturePreparers(
				FixtureRegisterWithContext(func(ctx RegistrationContext) {
					ctx.RegisterModuleType("my_module_type", newSelectsMockModule)
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
				m := result.ModuleForTests("foo", "")
				p, _ := OtherModuleProvider[selectsTestProvider](result.testContext.OtherModuleProviderAdaptor(), m.Module(), selectsTestProviderKey)
				if !reflect.DeepEqual(p, tc.provider) {
					t.Errorf("Expected:\n  %q\ngot:\n  %q", tc.provider.String(), p.String())
				}
			}
		})
	}
}

type selectsTestProvider struct {
	my_bool        *bool
	my_string      *string
	my_string_list *[]string
	my_paths       *[]string
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
}`, myBoolStr, myStringStr, p.my_string_list, p.my_paths)
}

var selectsTestProviderKey = blueprint.NewProvider[selectsTestProvider]()

type selectsMockModuleProperties struct {
	My_bool        proptools.Configurable[bool]
	My_string      proptools.Configurable[string]
	My_string_list proptools.Configurable[[]string]
	My_paths       proptools.Configurable[[]string] `android:"path"`
}

type selectsMockModule struct {
	ModuleBase
	DefaultableModuleBase
	properties selectsMockModuleProperties
}

func (p *selectsMockModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	SetProvider(ctx, selectsTestProviderKey, selectsTestProvider{
		my_bool:        p.properties.My_bool.Evaluate(ctx),
		my_string:      p.properties.My_string.Evaluate(ctx),
		my_string_list: p.properties.My_string_list.Evaluate(ctx),
		my_paths:       p.properties.My_paths.Evaluate(ctx),
	})
}

func newSelectsMockModule() Module {
	m := &selectsMockModule{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibCommon)
	InitDefaultableModule(m)
	return m
}
