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

package main

import (
	"reflect"
	"testing"
)

func Test_combine(t *testing.T) {
	tests := []struct {
		name string
		old  any
		new  any
		want any
	}{
		{
			name: "objects",
			old: map[string]any{
				"foo": "bar",
				"baz": []any{"a"},
			},
			new: map[string]any{
				"foo": "bar",
				"baz": []any{"b"},
			},
			want: map[string]any{
				"foo": "bar",
				"baz": []any{"a", "b"},
			},
		},
		{
			name: "arrays",
			old:  []any{"foo", "bar"},
			new:  []any{"foo", "baz"},
			want: []any{"bar", "baz", "foo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := combine(tt.old, tt.new); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("combine() = %v, want %v", got, tt.want)
			}
		})
	}
}
