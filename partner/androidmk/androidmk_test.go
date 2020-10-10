// Copyright 2016 Google Inc. All rights reserved.
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
	"bytes"
	"fmt"
	"strings"
	"testing"

	"android/soong/androidmk/androidmk"
	"android/soong/bpfix/bpfix"

	_ "android/soong/partner/bpfix/extensions"
)

var testCases = []struct {
	desc     string
	in       string
	expected string
}{
	{
		desc: "headers replacement",
		in: `
include $(CLEAR_VARS)
LOCAL_MODULE := test
LOCAL_SRC_FILES := a.c
LOCAL_C_INCLUDES := test1 $(TARGET_OUT_HEADERS)/my_headers test2
include $(BUILD_SHARED_LIBRARY)`,
		expected: `
cc_library_shared {
    name: "test",
	srcs: ["a.c"],
	include_dirs: [
		"test1",

		"test2",
	],
	header_libs: ["my_header_lib"]
}`,
	},
}

func TestEndToEnd(t *testing.T) {
	for i, test := range testCases {
		expected, err := bpfix.Reformat(test.expected)
		if err != nil {
			t.Error(err)
		}

		got, errs := androidmk.ConvertFile(fmt.Sprintf("<testcase %d>", i), bytes.NewBufferString(test.in))
		if len(errs) > 0 {
			t.Errorf("Unexpected errors: %q", errs)
			continue
		}

		if got != expected {
			t.Errorf("failed testcase '%s'\ninput:\n%s\n\nexpected:\n%s\ngot:\n%s\n", test.desc, strings.TrimSpace(test.in), expected, got)
		}
	}
}
