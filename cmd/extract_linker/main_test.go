// Copyright 2017 Google Inc. All rights reserved.
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
	"testing"
)

var bytesToAsmTestCases = []struct {
	name string
	in   []byte
	out  string
}{
	{
		name: "empty",
		in:   []byte{},
		out:  "\n",
	},
	{
		name: "short",
		in:   []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01},
		out:  ".byte 127,69,76,70,2,1\n",
	},
	{
		name: "multiline",
		in: []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x30, 0x00, 0x3e, 0x00, 0x01, 0x00, 0x00, 0x00,
			0x50, 0x98, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x88, 0xd1, 0x12, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x38, 0x00,
			0x01, 0x00, 0x00, 0x00, 0x40, 0x00, 0x38, 0x00,
			0x02, 0x00, 0x00, 0x00, 0x40, 0x00, 0x38, 0x00},
		out: ".byte 127,69,76,70,2,1,1,0,0,0,0,0,0,0,0,0,48,0,62,0,1,0,0,0,80,152,2,0,0,0,0,0,64,0,0,0,0,0,0,0,136,209,18,0,0,0,0,0,0,0,0,0,64,0,56,0,1,0,0,0,64,0,56,0\n" +
			".byte 2,0,0,0,64,0,56,0\n",
	},
}

func TestBytesToAsm(t *testing.T) {
	for _, testcase := range bytesToAsmTestCases {
		t.Run(testcase.name, func(t *testing.T) {
			buf := bytes.Buffer{}
			bytesToAsm(&buf, testcase.in)
			if buf.String() != testcase.out {
				t.Errorf("input: %#v\n want: %q\n  got: %q\n", testcase.in, testcase.out, buf.String())
			}
		})
	}
}
