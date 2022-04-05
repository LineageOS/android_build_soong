// Copyright 2022 Google Inc. All rights reserved.
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
	"encoding/binary"
	"reflect"
	"testing"
)

func Test_readNote(t *testing.T) {
	note := []byte{
		0x04, 0x00, 0x00, 0x00,
		0x10, 0x00, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0x47, 0x4e, 0x55, 0x00,
		0xca, 0xaf, 0x44, 0xd2, 0x82, 0x78, 0x68, 0xfe, 0xc0, 0x90, 0xa3, 0x43, 0x85, 0x36, 0x6c, 0xc7,
	}

	descs, err := readNote(bytes.NewBuffer(note), binary.LittleEndian)
	if err != nil {
		t.Fatalf("unexpected error in readNote: %s", err)
	}

	expectedDescs := map[string][]byte{
		"GNU\x00": []byte{0xca, 0xaf, 0x44, 0xd2, 0x82, 0x78, 0x68, 0xfe, 0xc0, 0x90, 0xa3, 0x43, 0x85, 0x36, 0x6c, 0xc7},
	}

	if !reflect.DeepEqual(descs, expectedDescs) {
		t.Errorf("incorrect return, want %#v got %#v", expectedDescs, descs)
	}
}
