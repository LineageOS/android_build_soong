// Copyright 2020 Google Inc. All rights reserved.
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

package soongconfig

import (
	"reflect"
	"testing"

	"github.com/google/blueprint/proptools"
)

func Test_CanonicalizeToProperty(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "lowercase",
			arg:  "board",
			want: "board",
		},
		{
			name: "uppercase",
			arg:  "BOARD",
			want: "BOARD",
		},
		{
			name: "numbers",
			arg:  "BOARD123",
			want: "BOARD123",
		},
		{
			name: "underscore",
			arg:  "TARGET_BOARD",
			want: "TARGET_BOARD",
		},
		{
			name: "dash",
			arg:  "TARGET-BOARD",
			want: "TARGET_BOARD",
		},
		{
			name: "unicode",
			arg:  "boardλ",
			want: "board_",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalizeToProperty(tt.arg); got != tt.want {
				t.Errorf("canonicalizeToProperty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_typeForPropertyFromPropertyStruct(t *testing.T) {
	tests := []struct {
		name     string
		ps       interface{}
		property string
		want     string
	}{
		{
			name: "string",
			ps: struct {
				A string
			}{},
			property: "a",
			want:     "string",
		},
		{
			name: "list",
			ps: struct {
				A []string
			}{},
			property: "a",
			want:     "[]string",
		},
		{
			name: "missing",
			ps: struct {
				A []string
			}{},
			property: "b",
			want:     "",
		},
		{
			name: "nested",
			ps: struct {
				A struct {
					B string
				}
			}{},
			property: "a.b",
			want:     "string",
		},
		{
			name: "missing nested",
			ps: struct {
				A struct {
					B string
				}
			}{},
			property: "a.c",
			want:     "",
		},
		{
			name: "not a struct",
			ps: struct {
				A string
			}{},
			property: "a.b",
			want:     "",
		},
		{
			name: "nested pointer",
			ps: struct {
				A *struct {
					B string
				}
			}{},
			property: "a.b",
			want:     "string",
		},
		{
			name: "nested interface",
			ps: struct {
				A interface{}
			}{
				A: struct {
					B string
				}{},
			},
			property: "a.b",
			want:     "string",
		},
		{
			name: "nested interface pointer",
			ps: struct {
				A interface{}
			}{
				A: &struct {
					B string
				}{},
			},
			property: "a.b",
			want:     "string",
		},
		{
			name: "nested interface nil pointer",
			ps: struct {
				A interface{}
			}{
				A: (*struct {
					B string
				})(nil),
			},
			property: "a.b",
			want:     "string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := typeForPropertyFromPropertyStruct(tt.ps, tt.property)
			got := ""
			if typ != nil {
				got = typ.String()
			}
			if got != tt.want {
				t.Errorf("typeForPropertyFromPropertyStruct() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_createAffectablePropertiesType(t *testing.T) {
	tests := []struct {
		name                 string
		affectableProperties []string
		factoryProps         interface{}
		want                 string
	}{
		{
			name:                 "string",
			affectableProperties: []string{"cflags"},
			factoryProps: struct {
				Cflags string
			}{},
			want: "*struct { Cflags string }",
		},
		{
			name:                 "list",
			affectableProperties: []string{"cflags"},
			factoryProps: struct {
				Cflags []string
			}{},
			want: "*struct { Cflags []string }",
		},
		{
			name:                 "string pointer",
			affectableProperties: []string{"cflags"},
			factoryProps: struct {
				Cflags *string
			}{},
			want: "*struct { Cflags *string }",
		},
		{
			name:                 "subset",
			affectableProperties: []string{"cflags"},
			factoryProps: struct {
				Cflags  string
				Ldflags string
			}{},
			want: "*struct { Cflags string }",
		},
		{
			name:                 "none",
			affectableProperties: []string{"cflags"},
			factoryProps: struct {
				Ldflags string
			}{},
			want: "",
		},
		{
			name:                 "nested",
			affectableProperties: []string{"multilib.lib32.cflags"},
			factoryProps: struct {
				Multilib struct {
					Lib32 struct {
						Cflags string
					}
				}
			}{},
			want: "*struct { Multilib struct { Lib32 struct { Cflags string } } }",
		},
		{
			name: "complex",
			affectableProperties: []string{
				"cflags",
				"multilib.lib32.cflags",
				"multilib.lib32.ldflags",
				"multilib.lib64.cflags",
				"multilib.lib64.ldflags",
				"zflags",
			},
			factoryProps: struct {
				Cflags   string
				Multilib struct {
					Lib32 struct {
						Cflags  string
						Ldflags string
					}
					Lib64 struct {
						Cflags  string
						Ldflags string
					}
				}
				Zflags string
			}{},
			want: "*struct { Cflags string; Multilib struct { Lib32 struct { Cflags string; Ldflags string }; Lib64 struct { Cflags string; Ldflags string } }; Zflags string }",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := createAffectablePropertiesType(tt.affectableProperties, []interface{}{tt.factoryProps})
			got := ""
			if typ != nil {
				got = typ.String()
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createAffectablePropertiesType() = %v, want %v", got, tt.want)
			}
		})
	}
}

type properties struct {
	A *string
	B bool
}

type boolVarProps struct {
	A                  *string
	B                  bool
	Conditions_default *properties
}

type boolSoongConfigVars struct {
	Bool_var interface{}
}

type stringSoongConfigVars struct {
	String_var interface{}
}

type valueSoongConfigVars struct {
	My_value_var interface{}
}

func Test_PropertiesToApply_Bool(t *testing.T) {
	mt, _ := newModuleType(&ModuleTypeProperties{
		Module_type:      "foo",
		Config_namespace: "bar",
		Bool_variables:   []string{"bool_var"},
		Properties:       []string{"a", "b"},
	})
	boolVarPositive := &properties{
		A: proptools.StringPtr("A"),
		B: true,
	}
	conditionsDefault := &properties{
		A: proptools.StringPtr("default"),
		B: false,
	}
	actualProps := &struct {
		Soong_config_variables boolSoongConfigVars
	}{
		Soong_config_variables: boolSoongConfigVars{
			Bool_var: &boolVarProps{
				A:                  boolVarPositive.A,
				B:                  boolVarPositive.B,
				Conditions_default: conditionsDefault,
			},
		},
	}
	props := reflect.ValueOf(actualProps)

	testCases := []struct {
		name      string
		config    SoongConfig
		wantProps []interface{}
	}{
		{
			name:      "no_vendor_config",
			config:    Config(map[string]string{}),
			wantProps: []interface{}{conditionsDefault},
		},
		{
			name:      "vendor_config_false",
			config:    Config(map[string]string{"bool_var": "n"}),
			wantProps: []interface{}{conditionsDefault},
		},
		{
			name:      "bool_var_true",
			config:    Config(map[string]string{"bool_var": "y"}),
			wantProps: []interface{}{boolVarPositive},
		},
	}

	for _, tc := range testCases {
		gotProps, err := PropertiesToApply(mt, props, tc.config)
		if err != nil {
			t.Errorf("%s: Unexpected error in PropertiesToApply: %s", tc.name, err)
		}

		if !reflect.DeepEqual(gotProps, tc.wantProps) {
			t.Errorf("%s: Expected %s, got %s", tc.name, tc.wantProps, gotProps)
		}
	}
}

func Test_PropertiesToApply_Value(t *testing.T) {
	mt, _ := newModuleType(&ModuleTypeProperties{
		Module_type:      "foo",
		Config_namespace: "bar",
		Value_variables:  []string{"my_value_var"},
		Properties:       []string{"a", "b"},
	})
	conditionsDefault := &properties{
		A: proptools.StringPtr("default"),
		B: false,
	}
	actualProps := &struct {
		Soong_config_variables valueSoongConfigVars
	}{
		Soong_config_variables: valueSoongConfigVars{
			My_value_var: &boolVarProps{
				A:                  proptools.StringPtr("A=%s"),
				B:                  true,
				Conditions_default: conditionsDefault,
			},
		},
	}
	props := reflect.ValueOf(actualProps)

	testCases := []struct {
		name      string
		config    SoongConfig
		wantProps []interface{}
	}{
		{
			name:      "no_vendor_config",
			config:    Config(map[string]string{}),
			wantProps: []interface{}{conditionsDefault},
		},
		{
			name:   "value_var_set",
			config: Config(map[string]string{"my_value_var": "Hello"}),
			wantProps: []interface{}{&properties{
				A: proptools.StringPtr("A=Hello"),
				B: true,
			}},
		},
	}

	for _, tc := range testCases {
		gotProps, err := PropertiesToApply(mt, props, tc.config)
		if err != nil {
			t.Errorf("%s: Unexpected error in PropertiesToApply: %s", tc.name, err)
		}

		if !reflect.DeepEqual(gotProps, tc.wantProps) {
			t.Errorf("%s: Expected %s, got %s", tc.name, tc.wantProps, gotProps)
		}
	}
}

func Test_PropertiesToApply_Value_Nested(t *testing.T) {
	mt, _ := newModuleType(&ModuleTypeProperties{
		Module_type:      "foo",
		Config_namespace: "bar",
		Value_variables:  []string{"my_value_var"},
		Properties:       []string{"a.b"},
	})
	type properties struct {
		A struct {
			B string
		}
	}
	conditionsDefault := &properties{
		A: struct{ B string }{
			B: "default",
		},
	}
	type valueVarProps struct {
		A struct {
			B string
		}
		Conditions_default *properties
	}
	actualProps := &struct {
		Soong_config_variables valueSoongConfigVars
	}{
		Soong_config_variables: valueSoongConfigVars{
			My_value_var: &valueVarProps{
				A: struct{ B string }{
					B: "A.B=%s",
				},
				Conditions_default: conditionsDefault,
			},
		},
	}
	props := reflect.ValueOf(actualProps)

	testCases := []struct {
		name      string
		config    SoongConfig
		wantProps []interface{}
	}{
		{
			name:      "no_vendor_config",
			config:    Config(map[string]string{}),
			wantProps: []interface{}{conditionsDefault},
		},
		{
			name:   "value_var_set",
			config: Config(map[string]string{"my_value_var": "Hello"}),
			wantProps: []interface{}{&properties{
				A: struct{ B string }{
					B: "A.B=Hello",
				},
			}},
		},
	}

	for _, tc := range testCases {
		gotProps, err := PropertiesToApply(mt, props, tc.config)
		if err != nil {
			t.Errorf("%s: Unexpected error in PropertiesToApply: %s", tc.name, err)
		}

		if !reflect.DeepEqual(gotProps, tc.wantProps) {
			t.Errorf("%s: Expected %s, got %s", tc.name, tc.wantProps, gotProps)
		}
	}
}

func Test_PropertiesToApply_String_Error(t *testing.T) {
	mt, _ := newModuleType(&ModuleTypeProperties{
		Module_type:      "foo",
		Config_namespace: "bar",
		Variables:        []string{"string_var"},
		Properties:       []string{"a", "b"},
	})
	mt.Variables = append(mt.Variables, &stringVariable{
		baseVariable: baseVariable{
			variable: "string_var",
		},
		values: []string{"a", "b", "c"},
	})
	stringVarPositive := &properties{
		A: proptools.StringPtr("A"),
		B: true,
	}
	conditionsDefault := &properties{
		A: proptools.StringPtr("default"),
		B: false,
	}
	actualProps := &struct {
		Soong_config_variables stringSoongConfigVars
	}{
		Soong_config_variables: stringSoongConfigVars{
			String_var: &boolVarProps{
				A:                  stringVarPositive.A,
				B:                  stringVarPositive.B,
				Conditions_default: conditionsDefault,
			},
		},
	}
	props := reflect.ValueOf(actualProps)

	_, err := PropertiesToApply(mt, props, Config(map[string]string{
		"string_var": "x",
	}))
	expected := `Soong config property "string_var" must be one of [a b c], found "x"`
	if err == nil {
		t.Fatalf("Expected an error, got nil")
	} else if err.Error() != expected {
		t.Fatalf("Error message was not correct, expected %q, got %q", expected, err.Error())
	}
}
