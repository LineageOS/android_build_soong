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
type soongConfigVariables struct {
	Bool_var       properties
	Other_bool_var properties
}

type soongConfigProps struct {
	Soong_config_variables soongConfigVariables
}

func Test_PropertiesToApply(t *testing.T) {

	mt := &ModuleType{
		BaseModuleType:  "foo",
		ConfigNamespace: "bar",
		Variables: []soongConfigVariable{
			newBoolVariable("bool_var"),
			newBoolVariable("other_bool_var"),
		},
		affectableProperties: []string{
			"a",
			"b",
		},
	}
	props := soongConfigProps{
		Soong_config_variables: soongConfigVariables{
			Bool_var: properties{
				A: proptools.StringPtr("a"),
				B: true,
			},
			Other_bool_var: properties{
				A: proptools.StringPtr("other"),
				B: false,
			},
		},
	}

	testCases := []struct {
		config    SoongConfig
		wantProps []interface{}
	}{
		{
			config: Config(map[string]string{}),
		},
		{
			config:    Config(map[string]string{"bool_var": "y"}),
			wantProps: []interface{}{props.Soong_config_variables.Bool_var},
		},
		{
			config:    Config(map[string]string{"other_bool_var": "y"}),
			wantProps: []interface{}{props.Soong_config_variables.Other_bool_var},
		},
	}

	for _, tc := range testCases {
		gotProps, err := PropertiesToApply(mt, reflect.ValueOf(&props), tc.config)
		if err != nil {
			t.Errorf("Unexpected error in PropertiesToApply: %s", err)
		}

		if !reflect.DeepEqual(gotProps, tc.wantProps) {
			t.Errorf("Expected %s, got %s", tc.wantProps, gotProps)
		}
	}
}
