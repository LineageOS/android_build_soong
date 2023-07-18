// Copyright 2015 Google Inc. All rights reserved.
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
	"testing"
)

func TestSdkSpecFrom(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "",
			expected: "private_current",
		},
		{
			input:    "none",
			expected: "none_(no version)",
		},
		{
			input:    "core_platform",
			expected: "core_platform_current",
		},
		{
			input:    "_",
			expected: "invalid__",
		},
		{
			input:    "_31",
			expected: "invalid__31",
		},
		{
			input:    "system_R",
			expected: "system_30",
		},
		{
			input:    "test_31",
			expected: "test_31",
		},
		{
			input:    "module_current",
			expected: "module-lib_current",
		},
		{
			input:    "31",
			expected: "public_31",
		},
		{
			input:    "S",
			expected: "public_31",
		},
		{
			input:    "current",
			expected: "public_current",
		},
		{
			input:    "Tiramisu",
			expected: "public_Tiramisu",
		},
	}

	config := NullConfig("", "")

	config.productVariables = ProductVariables{
		Platform_sdk_version:              intPtr(31),
		Platform_sdk_codename:             stringPtr("Tiramisu"),
		Platform_version_active_codenames: []string{"Tiramisu"},
	}

	for _, tc := range testCases {
		if got := SdkSpecFromWithConfig(config, tc.input).String(); tc.expected != got {
			t.Errorf("Expected %v, got %v", tc.expected, got)
		}
	}
}
