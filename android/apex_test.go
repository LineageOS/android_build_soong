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
				{"foo", FutureApiLevel, false, false, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex, nil},
			},
			wantMerged: []ApexInfo{
				{"apex10000", FutureApiLevel, false, false, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex, nil},
			},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
			},
		},
		{
			name: "merge",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, false, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex, nil},
				{"bar", FutureApiLevel, false, false, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex, nil},
			},
			wantMerged: []ApexInfo{
				{"apex10000", FutureApiLevel, false, false, []string{"bar", "foo"}, []string{"bar", "foo"}, nil, false, nil}},
			wantAliases: [][2]string{
				{"bar", "apex10000"},
				{"foo", "apex10000"},
			},
		},
		{
			name: "don't merge version",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, false, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex, nil},
				{"bar", uncheckedFinalApiLevel(30), false, false, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex, nil},
			},
			wantMerged: []ApexInfo{
				{"apex30", uncheckedFinalApiLevel(30), false, false, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex, nil},
				{"apex10000", FutureApiLevel, false, false, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex, nil},
			},
			wantAliases: [][2]string{
				{"bar", "apex30"},
				{"foo", "apex10000"},
			},
		},
		{
			name: "merge updatable",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, false, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex, nil},
				{"bar", FutureApiLevel, true, false, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex, nil},
			},
			wantMerged: []ApexInfo{
				{"apex10000", FutureApiLevel, true, false, []string{"bar", "foo"}, []string{"bar", "foo"}, nil, NotForPrebuiltApex, nil},
			},
			wantAliases: [][2]string{
				{"bar", "apex10000"},
				{"foo", "apex10000"},
			},
		},
		{
			name: "don't merge when for prebuilt_apex",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, false, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex, nil},
				{"bar", FutureApiLevel, true, false, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex, nil},
				// This one should not be merged in with the others because it is for
				// a prebuilt_apex.
				{"baz", FutureApiLevel, true, false, []string{"baz"}, []string{"baz"}, nil, ForPrebuiltApex, nil},
			},
			wantMerged: []ApexInfo{
				{"apex10000", FutureApiLevel, true, false, []string{"bar", "foo"}, []string{"bar", "foo"}, nil, NotForPrebuiltApex, nil},
				{"baz", FutureApiLevel, true, false, []string{"baz"}, []string{"baz"}, nil, ForPrebuiltApex, nil},
			},
			wantAliases: [][2]string{
				{"bar", "apex10000"},
				{"foo", "apex10000"},
			},
		},
		{
			name: "merge different UsePlatformApis but don't allow using platform api",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, false, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex, nil},
				{"bar", FutureApiLevel, false, true, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex, nil},
			},
			wantMerged: []ApexInfo{
				{"apex10000", FutureApiLevel, false, false, []string{"bar", "foo"}, []string{"bar", "foo"}, nil, NotForPrebuiltApex, nil},
			},
			wantAliases: [][2]string{
				{"bar", "apex10000"},
				{"foo", "apex10000"},
			},
		},
		{
			name: "merge same UsePlatformApis and allow using platform api",
			in: []ApexInfo{
				{"foo", FutureApiLevel, false, true, []string{"foo"}, []string{"foo"}, nil, NotForPrebuiltApex, nil},
				{"bar", FutureApiLevel, false, true, []string{"bar"}, []string{"bar"}, nil, NotForPrebuiltApex, nil},
			},
			wantMerged: []ApexInfo{
				{"apex10000", FutureApiLevel, false, true, []string{"bar", "foo"}, []string{"bar", "foo"}, nil, NotForPrebuiltApex, nil},
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
