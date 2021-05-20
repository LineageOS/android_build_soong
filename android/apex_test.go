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
	const (
		ForPrebuiltApex    = true
		NotForPrebuiltApex = false
	)
	tests := []struct {
		name        string
		in          []ApexInfo
		wantMerged  []ApexInfo
		wantAliases [][2]string
	}{
		{
			name: "single",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, nil, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex},
			},
			wantMerged: []ApexInfo{
				{"apex10000", FutureApiLevel, false, nil, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex},
			},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
			},
		},
		{
			name: "merge",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, SdkRefs{{"baz", "1"}}, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex},
				{"bar", FutureApiLevel, false, SdkRefs{{"baz", "1"}}, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex},
			},
			wantMerged: []ApexInfo{
				{"apex10000_baz_1", FutureApiLevel, false, SdkRefs{{"baz", "1"}}, []string{"bar", "foo"}, []string{"bar", "foo"}, nil, false}},
			wantAliases: [][2]string{
				{"bar", "apex10000_baz_1"},
				{"foo", "apex10000_baz_1"},
			},
		},
		{
			name: "don't merge version",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, nil, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex},
				{"bar", uncheckedFinalApiLevel(30), false, nil, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex},
			},
			wantMerged: []ApexInfo{
				{"apex30", uncheckedFinalApiLevel(30), false, nil, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex},
				{"apex10000", FutureApiLevel, false, nil, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex},
			},
			wantAliases: [][2]string{
				{"bar", "apex30"},
				{"foo", "apex10000"},
			},
		},
		{
			name: "merge updatable",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, nil, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex},
				{"bar", FutureApiLevel, true, nil, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex},
			},
			wantMerged: []ApexInfo{
				{"apex10000", FutureApiLevel, true, nil, []string{"bar", "foo"}, []string{"bar", "foo"}, nil, NotForPrebuiltApex},
			},
			wantAliases: [][2]string{
				{"bar", "apex10000"},
				{"foo", "apex10000"},
			},
		},
		{
			name: "don't merge sdks",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, SdkRefs{{"baz", "1"}}, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex},
				{"bar", FutureApiLevel, false, SdkRefs{{"baz", "2"}}, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex},
			},
			wantMerged: []ApexInfo{
				{"apex10000_baz_2", FutureApiLevel, false, SdkRefs{{"baz", "2"}}, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex},
				{"apex10000_baz_1", FutureApiLevel, false, SdkRefs{{"baz", "1"}}, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex},
			},
			wantAliases: [][2]string{
				{"bar", "apex10000_baz_2"},
				{"foo", "apex10000_baz_1"},
			},
		},
		{
			name: "don't merge when for prebuilt_apex",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, nil, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex},
				{"bar", FutureApiLevel, true, nil, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex},
				// This one should not be merged in with the others because it is for
				// a prebuilt_apex.
				{"baz", FutureApiLevel, true, nil, []string{"baz"}, []string{"baz"}, nil, ForPrebuiltApex},
			},
			wantMerged: []ApexInfo{
				{"apex10000", FutureApiLevel, true, nil, []string{"bar", "foo"}, []string{"bar", "foo"}, nil, NotForPrebuiltApex},
				{"baz", FutureApiLevel, true, nil, []string{"baz"}, []string{"baz"}, nil, ForPrebuiltApex},
			},
			wantAliases: [][2]string{
				{"bar", "apex10000"},
				{"foo", "apex10000"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := TestConfig(t.TempDir(), nil, "", nil)
			ctx := &configErrorWrapper{config: config}
			gotMerged, gotAliases := mergeApexVariations(ctx, tt.in)
			if !reflect.DeepEqual(gotMerged, tt.wantMerged) {
				t.Errorf("mergeApexVariations() gotMerged = %v, want %v", gotMerged, tt.wantMerged)
			}
			if !reflect.DeepEqual(gotAliases, tt.wantAliases) {
				t.Errorf("mergeApexVariations() gotAliases = %v, want %v", gotAliases, tt.wantAliases)
			}
		})
	}
}
