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

package main

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSplitList(t *testing.T) {
	testcases := []struct {
		inputCount int
		shardCount int
		want       [][]string
	}{
		{
			inputCount: 1,
			shardCount: 1,
			want:       [][]string{{"1"}},
		},
		{
			inputCount: 1,
			shardCount: 2,
			want:       [][]string{{"1"}, {}},
		},
		{
			inputCount: 4,
			shardCount: 2,
			want:       [][]string{{"1", "2"}, {"3", "4"}},
		},
		{
			inputCount: 19,
			shardCount: 10,
			want: [][]string{
				{"1", "2"},
				{"3", "4"},
				{"5", "6"},
				{"7", "8"},
				{"9", "10"},
				{"11", "12"},
				{"13", "14"},
				{"15", "16"},
				{"17", "18"},
				{"19"},
			},
		},
		{
			inputCount: 15,
			shardCount: 10,
			want: [][]string{
				{"1", "2"},
				{"3", "4"},
				{"5", "6"},
				{"7", "8"},
				{"9", "10"},
				{"11"},
				{"12"},
				{"13"},
				{"14"},
				{"15"},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("%d/%d", tc.inputCount, tc.shardCount), func(t *testing.T) {
			input := []string{}
			for i := 1; i <= tc.inputCount; i++ {
				input = append(input, fmt.Sprintf("%d", i))
			}

			got := splitList(input, tc.shardCount)

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("unexpected result for splitList([]string{...%d...}, %d):\nwant: %v\n got: %v\n",
					tc.inputCount, tc.shardCount, tc.want, got)
			}
		})
	}
}
