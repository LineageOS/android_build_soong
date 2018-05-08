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

package parser

import (
	"bytes"
	"testing"
)

var parserTestCases = []struct {
	name string
	in   string
	out  []Node
}{
	{
		name: "Escaped $",
		in:   `a$$ b: c`,
		out: []Node{
			&Rule{
				Target:        SimpleMakeString("a$ b", NoPos),
				Prerequisites: SimpleMakeString("c", NoPos),
			},
		},
	},
}

func TestParse(t *testing.T) {
	for _, test := range parserTestCases {
		t.Run(test.name, func(t *testing.T) {
			p := NewParser(test.name, bytes.NewBufferString(test.in))
			got, errs := p.Parse()

			if len(errs) != 0 {
				t.Fatalf("Unexpected errors while parsing: %v", errs)
			}

			if len(got) != len(test.out) {
				t.Fatalf("length mismatch, expected %d nodes, got %d", len(test.out), len(got))
			}

			for i := range got {
				if got[i].Dump() != test.out[i].Dump() {
					t.Errorf("incorrect node %d:\nexpected: %#v (%s)\n     got: %#v (%s)",
						i, test.out[i], test.out[i].Dump(), got[i], got[i].Dump())
				}
			}
		})
	}
}
