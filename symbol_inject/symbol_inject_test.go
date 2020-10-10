// Copyright 2018 Google Inc. All rights reserved.
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

package symbol_inject

import (
	"bytes"
	"strconv"
	"testing"
)

func TestCopyAndInject(t *testing.T) {
	s := "abcdefghijklmnopqrstuvwxyz"
	testCases := []struct {
		offset   uint64
		buf      string
		expected string
	}{
		{
			offset:   0,
			buf:      "A",
			expected: "Abcdefghijklmnopqrstuvwxyz",
		},
		{
			offset:   1,
			buf:      "B",
			expected: "aBcdefghijklmnopqrstuvwxyz",
		},
		{
			offset:   25,
			buf:      "Z",
			expected: "abcdefghijklmnopqrstuvwxyZ",
		},
	}

	for i, testCase := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			in := bytes.NewReader([]byte(s))
			out := &bytes.Buffer{}
			copyAndInject(in, out, testCase.offset, []byte(testCase.buf))

			if out.String() != testCase.expected {
				t.Errorf("expected %s, got %s", testCase.expected, out.String())
			}
		})
	}
}
