// Copyright 2021 Google Inc. All rights reserved.
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

package response

import (
	"bytes"
	"reflect"
	"testing"
)

func TestReadRspFile(t *testing.T) {
	testCases := []struct {
		name, in string
		out      []string
	}{
		{
			name: "single quoting test case 1",
			in:   `./cmd '"'-C`,
			out:  []string{"./cmd", `"-C`},
		},
		{
			name: "single quoting test case 2",
			in:   `./cmd '-C`,
			out:  []string{"./cmd", `-C`},
		},
		{
			name: "single quoting test case 3",
			in:   `./cmd '\"'-C`,
			out:  []string{"./cmd", `\"-C`},
		},
		{
			name: "single quoting test case 4",
			in:   `./cmd '\\'-C`,
			out:  []string{"./cmd", `\\-C`},
		},
		{
			name: "none quoting test case 1",
			in:   `./cmd \'-C`,
			out:  []string{"./cmd", `'-C`},
		},
		{
			name: "none quoting test case 2",
			in:   `./cmd \\-C`,
			out:  []string{"./cmd", `\-C`},
		},
		{
			name: "none quoting test case 3",
			in:   `./cmd \"-C`,
			out:  []string{"./cmd", `"-C`},
		},
		{
			name: "double quoting test case 1",
			in:   `./cmd "'"-C`,
			out:  []string{"./cmd", `'-C`},
		},
		{
			name: "double quoting test case 2",
			in:   `./cmd "\\"-C`,
			out:  []string{"./cmd", `\-C`},
		},
		{
			name: "double quoting test case 3",
			in:   `./cmd "\""-C`,
			out:  []string{"./cmd", `"-C`},
		},
		{
			name: "ninja rsp file",
			in:   "'a'\nb\n'@'\n'foo'\\''bar'\n'foo\"bar'",
			out:  []string{"a", "b", "@", "foo'bar", `foo"bar`},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := ReadRspFile(bytes.NewBuffer([]byte(testCase.in)))
			if err != nil {
				t.Errorf("unexpected error: %q", err)
			}
			if !reflect.DeepEqual(got, testCase.out) {
				t.Errorf("expected %q got %q", testCase.out, got)
			}
		})
	}
}

func TestWriteRspFile(t *testing.T) {
	testCases := []struct {
		name string
		in   []string
		out  string
	}{
		{
			name: "ninja rsp file",
			in:   []string{"a", "b", "@", "foo'bar", `foo"bar`},
			out:  "a b '@' 'foo'\\''bar' 'foo\"bar'",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := WriteRspFile(buf, testCase.in)
			if err != nil {
				t.Errorf("unexpected error: %q", err)
			}
			if buf.String() != testCase.out {
				t.Errorf("expected %q got %q", testCase.out, buf.String())
			}
		})
	}
}
