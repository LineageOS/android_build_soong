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
				{
					ApexVariationName: "foo",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo"},
					InApexModules:     []string{"foo"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantMerged: []ApexInfo{
				{
					ApexVariationName: "apex10000",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo"},
					InApexModules:     []string{"foo"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
			},
		},
		{
			name: "merge",
			in: []ApexInfo{
				{
					ApexVariationName: "foo",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo"},
					InApexModules:     []string{"foo"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
				{
					ApexVariationName: "bar",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"bar"},
					InApexModules:     []string{"bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantMerged: []ApexInfo{
				{
					ApexVariationName: "apex10000",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo", "bar"},
					InApexModules:     []string{"foo", "bar"},
				}},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
				{"bar", "apex10000"},
			},
		},
		{
			name: "don't merge version",
			in: []ApexInfo{
				{
					ApexVariationName: "foo",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo"},
					InApexModules:     []string{"foo"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
				{
					ApexVariationName: "bar",
					MinSdkVersion:     uncheckedFinalApiLevel(30),
					InApexVariants:    []string{"bar"},
					InApexModules:     []string{"bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantMerged: []ApexInfo{
				{
					ApexVariationName: "apex10000",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo"},
					InApexModules:     []string{"foo"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
				{
					ApexVariationName: "apex30",
					MinSdkVersion:     uncheckedFinalApiLevel(30),
					InApexVariants:    []string{"bar"},
					InApexModules:     []string{"bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
				{"bar", "apex30"},
			},
		},
		{
			name: "merge updatable",
			in: []ApexInfo{
				{
					ApexVariationName: "foo",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo"},
					InApexModules:     []string{"foo"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
				{
					ApexVariationName: "bar",
					MinSdkVersion:     FutureApiLevel,
					Updatable:         true,
					InApexVariants:    []string{"bar"},
					InApexModules:     []string{"bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantMerged: []ApexInfo{
				{
					ApexVariationName: "apex10000",
					MinSdkVersion:     FutureApiLevel,
					Updatable:         true,
					InApexVariants:    []string{"foo", "bar"},
					InApexModules:     []string{"foo", "bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
				{"bar", "apex10000"},
			},
		},
		{
			name: "don't merge when for prebuilt_apex",
			in: []ApexInfo{
				{
					ApexVariationName: "foo",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo"},
					InApexModules:     []string{"foo"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
				{
					ApexVariationName: "bar",
					MinSdkVersion:     FutureApiLevel,
					Updatable:         true,
					InApexVariants:    []string{"bar"},
					InApexModules:     []string{"bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
				// This one should not be merged in with the others because it is for
				// a prebuilt_apex.
				{
					ApexVariationName: "baz",
					MinSdkVersion:     FutureApiLevel,
					Updatable:         true,
					InApexVariants:    []string{"baz"},
					InApexModules:     []string{"baz"},
					ForPrebuiltApex:   ForPrebuiltApex,
				},
			},
			wantMerged: []ApexInfo{
				{
					ApexVariationName: "apex10000",
					MinSdkVersion:     FutureApiLevel,
					Updatable:         true,
					InApexVariants:    []string{"foo", "bar"},
					InApexModules:     []string{"foo", "bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
				{
					ApexVariationName: "baz",
					MinSdkVersion:     FutureApiLevel,
					Updatable:         true,
					InApexVariants:    []string{"baz"},
					InApexModules:     []string{"baz"},
					ForPrebuiltApex:   ForPrebuiltApex,
				},
			},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
				{"bar", "apex10000"},
			},
		},
		{
			name: "merge different UsePlatformApis but don't allow using platform api",
			in: []ApexInfo{
				{
					ApexVariationName: "foo",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo"},
					InApexModules:     []string{"foo"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
				{
					ApexVariationName: "bar",
					MinSdkVersion:     FutureApiLevel,
					UsePlatformApis:   true,
					InApexVariants:    []string{"bar"},
					InApexModules:     []string{"bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantMerged: []ApexInfo{
				{
					ApexVariationName: "apex10000",
					MinSdkVersion:     FutureApiLevel,
					InApexVariants:    []string{"foo", "bar"},
					InApexModules:     []string{"foo", "bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
				{"bar", "apex10000"},
			},
		},
		{
			name: "merge same UsePlatformApis and allow using platform api",
			in: []ApexInfo{
				{
					ApexVariationName: "foo",
					MinSdkVersion:     FutureApiLevel,
					UsePlatformApis:   true,
					InApexVariants:    []string{"foo"},
					InApexModules:     []string{"foo"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
				{
					ApexVariationName: "bar",
					MinSdkVersion:     FutureApiLevel,
					UsePlatformApis:   true,
					InApexVariants:    []string{"bar"},
					InApexModules:     []string{"bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantMerged: []ApexInfo{
				{
					ApexVariationName: "apex10000",
					MinSdkVersion:     FutureApiLevel,
					UsePlatformApis:   true,
					InApexVariants:    []string{"foo", "bar"},
					InApexModules:     []string{"foo", "bar"},
					ForPrebuiltApex:   NotForPrebuiltApex,
				},
			},
			wantAliases: [][2]string{
				{"foo", "apex10000"},
				{"bar", "apex10000"},
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
