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

package android

import (
	"reflect"
	"testing"
)

func Test_mergeApexVariations(t *testing.T) {
	tests := []struct {
		name        string
		in          []ApexInfo
		wantMerged  []ApexInfo
		wantAliases [][2]string
	}{
		{
			name: "single",
			in: []ApexInfo{
				{"foo", 10000, false, nil, []string{"foo"}},
			},
			wantMerged: []ApexInfo{
				{"apex10000", 10000, false, nil, []string{"foo"}},
			},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
			},
		},
		{
			name: "merge",
			in: []ApexInfo{
				{"foo", 10000, false, SdkRefs{{"baz", "1"}}, []string{"foo"}},
				{"bar", 10000, false, SdkRefs{{"baz", "1"}}, []string{"bar"}},
			},
			wantMerged: []ApexInfo{
				{"apex10000_baz_1", 10000, false, SdkRefs{{"baz", "1"}}, []string{"bar", "foo"}},
			},
			wantAliases: [][2]string{
				{"bar", "apex10000_baz_1"},
				{"foo", "apex10000_baz_1"},
			},
		},
		{
			name: "don't merge version",
			in: []ApexInfo{
				{"foo", 10000, false, nil, []string{"foo"}},
				{"bar", 30, false, nil, []string{"bar"}},
			},
			wantMerged: []ApexInfo{
				{"apex30", 30, false, nil, []string{"bar"}},
				{"apex10000", 10000, false, nil, []string{"foo"}},
			},
			wantAliases: [][2]string{
				{"bar", "apex30"},
				{"foo", "apex10000"},
			},
		},
		{
			name: "merge updatable",
			in: []ApexInfo{
				{"foo", 10000, false, nil, []string{"foo"}},
				{"bar", 10000, true, nil, []string{"bar"}},
			},
			wantMerged: []ApexInfo{
				{"apex10000", 10000, true, nil, []string{"bar", "foo"}},
			},
			wantAliases: [][2]string{
				{"bar", "apex10000"},
				{"foo", "apex10000"},
			},
		},
		{
			name: "don't merge sdks",
			in: []ApexInfo{
				{"foo", 10000, false, SdkRefs{{"baz", "1"}}, []string{"foo"}},
				{"bar", 10000, false, SdkRefs{{"baz", "2"}}, []string{"bar"}},
			},
			wantMerged: []ApexInfo{
				{"apex10000_baz_2", 10000, false, SdkRefs{{"baz", "2"}}, []string{"bar"}},
				{"apex10000_baz_1", 10000, false, SdkRefs{{"baz", "1"}}, []string{"foo"}},
			},
			wantAliases: [][2]string{
				{"bar", "apex10000_baz_2"},
				{"foo", "apex10000_baz_1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMerged, gotAliases := mergeApexVariations(tt.in)
			if !reflect.DeepEqual(gotMerged, tt.wantMerged) {
				t.Errorf("mergeApexVariations() gotMerged = %v, want %v", gotMerged, tt.wantMerged)
			}
			if !reflect.DeepEqual(gotAliases, tt.wantAliases) {
				t.Errorf("mergeApexVariations() gotAliases = %v, want %v", gotAliases, tt.wantAliases)
			}
		})
	}
}
