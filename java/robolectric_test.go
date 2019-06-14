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

package java

import (
	"reflect"
	"testing"
)

func Test_shardTests(t *testing.T) {
	type args struct {
		paths  []string
		shards int
	}
	tests := []struct {
		name string
		args args
		want [][]string
	}{
		{
			name: "empty",
			args: args{
				paths:  nil,
				shards: 1,
			},
			want: [][]string(nil),
		},
		{
			name: "too many shards",
			args: args{
				paths:  []string{"a", "b"},
				shards: 3,
			},
			want: [][]string{{"a"}, {"b"}},
		},
		{
			name: "single shard",
			args: args{
				paths:  []string{"a", "b"},
				shards: 1,
			},
			want: [][]string{{"a", "b"}},
		},
		{
			name: "shard per input",
			args: args{
				paths:  []string{"a", "b", "c"},
				shards: 3,
			},
			want: [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name: "balanced shards",
			args: args{
				paths:  []string{"a", "b", "c", "d"},
				shards: 2,
			},
			want: [][]string{{"a", "b"}, {"c", "d"}},
		},
		{
			name: "unbalanced shards",
			args: args{
				paths:  []string{"a", "b", "c"},
				shards: 2,
			},
			want: [][]string{{"a", "b"}, {"c"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shardTests(tt.args.paths, tt.args.shards); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("shardTests() = %v, want %v", got, tt.want)
			}
		})
	}
}
