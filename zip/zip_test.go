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

package zip

import (
	"reflect"
	"testing"
)

func TestReadRespFile(t *testing.T) {
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
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := ReadRespFile([]byte(testCase.in))
			if !reflect.DeepEqual(got, testCase.out) {
				t.Errorf("expected %q got %q", testCase.out, got)
			}
		})
	}
}
