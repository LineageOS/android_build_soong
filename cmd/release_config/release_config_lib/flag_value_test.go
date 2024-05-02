// Copyright 2024 Google Inc. All rights reserved.
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

package release_config_lib

import (
	"os"
	"path/filepath"
	"testing"

	rc_proto "android/soong/cmd/release_config/release_config_proto"

	"google.golang.org/protobuf/proto"
)

type testCaseFlagValueFactory struct {
	protoPath string
	name      string
	data      []byte
	expected  rc_proto.FlagValue
	err       error
}

func (tc testCaseFlagValueFactory) assertProtoEqual(t *testing.T, expected, actual proto.Message) {
	if !proto.Equal(expected, actual) {
		t.Errorf("Expected %q found %q", expected, actual)
	}
}

func TestFlagValueFactory(t *testing.T) {
	testCases := []testCaseFlagValueFactory{
		{
			name:      "stringVal",
			protoPath: "build/release/flag_values/test/RELEASE_FOO.textproto",
			data:      []byte(`name: "RELEASE_FOO" value {string_value: "BAR"}`),
			expected: rc_proto.FlagValue{
				Name:  proto.String("RELEASE_FOO"),
				Value: &rc_proto.Value{Val: &rc_proto.Value_StringValue{"BAR"}},
			},
			err: nil,
		},
	}
	for _, tc := range testCases {
		var err error
		tempdir := t.TempDir()
		path := filepath.Join(tempdir, tc.protoPath)
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, tc.data, 0644); err != nil {
			t.Fatal(err)
		}
		actual := FlagValueFactory(path)
		tc.assertProtoEqual(t, &tc.expected, &actual.proto)
	}
}

type testCaseMarshalValue struct {
	name     string
	value    *rc_proto.Value
	expected string
}

func TestMarshalValue(t *testing.T) {
	testCases := []testCaseMarshalValue{
		{
			name:     "nil",
			value:    nil,
			expected: "",
		},
		{
			name:     "unspecified",
			value:    &rc_proto.Value{},
			expected: "",
		},
		{
			name:     "false",
			value:    &rc_proto.Value{Val: &rc_proto.Value_BoolValue{false}},
			expected: "",
		},
		{
			name:     "true",
			value:    &rc_proto.Value{Val: &rc_proto.Value_BoolValue{true}},
			expected: "true",
		},
		{
			name:     "string",
			value:    &rc_proto.Value{Val: &rc_proto.Value_StringValue{"BAR"}},
			expected: "BAR",
		},
		{
			name:     "obsolete",
			value:    &rc_proto.Value{Val: &rc_proto.Value_Obsolete{true}},
			expected: " #OBSOLETE",
		},
	}
	for _, tc := range testCases {
		actual := MarshalValue(tc.value)
		if actual != tc.expected {
			t.Errorf("Expected %q found %q", tc.expected, actual)
		}
	}
}
