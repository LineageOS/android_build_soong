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

type Named struct {
	A *string `android:"arch_variant"`
	B *string
}

type NamedAllFiltered struct {
	A *string
}

type NamedNoneFiltered struct {
	A *string `android:"arch_variant"`
}

func TestFilterArchStruct(t *testing.T) {
	tests := []struct {
		name     string
		in       interface{}
		out      interface{}
		filtered bool
	}{
		// Property tests
		{
			name: "basic",
			in: &struct {
				A *string `android:"arch_variant"`
				B *string
			}{},
			out: &struct {
				A *string
			}{},
			filtered: true,
		},
		{
			name: "all filtered",
			in: &struct {
				A *string
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "none filtered",
			in: &struct {
				A *string `android:"arch_variant"`
			}{},
			out: &struct {
				A *string `android:"arch_variant"`
			}{},
			filtered: false,
		},

		// Sub-struct tests
		{
			name: "substruct",
			in: &struct {
				A struct {
					A *string `android:"arch_variant"`
					B *string
				} `android:"arch_variant"`
			}{},
			out: &struct {
				A struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "substruct all filtered",
			in: &struct {
				A struct {
					A *string
				} `android:"arch_variant"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "substruct none filtered",
			in: &struct {
				A struct {
					A *string `android:"arch_variant"`
				} `android:"arch_variant"`
			}{},
			out: &struct {
				A struct {
					A *string `android:"arch_variant"`
				} `android:"arch_variant"`
			}{},
			filtered: false,
		},

		// Named sub-struct tests
		{
			name: "named substruct",
			in: &struct {
				A Named `android:"arch_variant"`
			}{},
			out: &struct {
				A struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "substruct all filtered",
			in: &struct {
				A NamedAllFiltered `android:"arch_variant"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "substruct none filtered",
			in: &struct {
				A NamedNoneFiltered `android:"arch_variant"`
			}{},
			out: &struct {
				A NamedNoneFiltered `android:"arch_variant"`
			}{},
			filtered: false,
		},

		// Pointer to sub-struct tests
		{
			name: "pointer substruct",
			in: &struct {
				A *struct {
					A *string `android:"arch_variant"`
					B *string
				} `android:"arch_variant"`
			}{},
			out: &struct {
				A *struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "pointer substruct all filtered",
			in: &struct {
				A *struct {
					A *string
				} `android:"arch_variant"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "pointer substruct none filtered",
			in: &struct {
				A *struct {
					A *string `android:"arch_variant"`
				} `android:"arch_variant"`
			}{},
			out: &struct {
				A *struct {
					A *string `android:"arch_variant"`
				} `android:"arch_variant"`
			}{},
			filtered: false,
		},

		// Pointer to named sub-struct tests
		{
			name: "pointer named substruct",
			in: &struct {
				A *Named `android:"arch_variant"`
			}{},
			out: &struct {
				A *struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "pointer substruct all filtered",
			in: &struct {
				A *NamedAllFiltered `android:"arch_variant"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "pointer substruct none filtered",
			in: &struct {
				A *NamedNoneFiltered `android:"arch_variant"`
			}{},
			out: &struct {
				A *NamedNoneFiltered `android:"arch_variant"`
			}{},
			filtered: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, filtered := filterArchStruct(reflect.TypeOf(test.in))
			if filtered != test.filtered {
				t.Errorf("expected filtered %v, got %v", test.filtered, filtered)
			}
			expected := reflect.TypeOf(test.out)
			if out != expected {
				t.Errorf("expected type %v, got %v", expected, out)
			}
		})
	}
}
