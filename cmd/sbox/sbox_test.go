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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_filesHaveSameContents(t *testing.T) {

	tests := []struct {
		name     string
		a        string
		b        string
		missingA bool
		missingB bool

		equal bool
	}{
		{
			name:  "empty",
			a:     "",
			b:     "",
			equal: true,
		},
		{
			name:  "equal",
			a:     "foo",
			b:     "foo",
			equal: true,
		},
		{
			name:  "unequal",
			a:     "foo",
			b:     "bar",
			equal: false,
		},
		{
			name:  "unequal different sizes",
			a:     "foo",
			b:     "foobar",
			equal: false,
		},
		{
			name:  "equal large",
			a:     strings.Repeat("a", 2*1024*1024),
			b:     strings.Repeat("a", 2*1024*1024),
			equal: true,
		},
		{
			name:  "equal large unaligned",
			a:     strings.Repeat("a", 2*1024*1024+10),
			b:     strings.Repeat("a", 2*1024*1024+10),
			equal: true,
		},
		{
			name:  "unequal large",
			a:     strings.Repeat("a", 2*1024*1024),
			b:     strings.Repeat("a", 2*1024*1024-1) + "b",
			equal: false,
		},
		{
			name:  "unequal large unaligned",
			a:     strings.Repeat("a", 2*1024*1024+10),
			b:     strings.Repeat("a", 2*1024*1024+9) + "b",
			equal: false,
		},
		{
			name:     "missing a",
			missingA: true,
			b:        "foo",
			equal:    false,
		},
		{
			name:     "missing b",
			a:        "foo",
			missingB: true,
			equal:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "testFilesHaveSameContents")
			if err != nil {
				t.Fatalf("failed to create temp dir: %s", err)
			}
			defer os.RemoveAll(tempDir)

			fileA := filepath.Join(tempDir, "a")
			fileB := filepath.Join(tempDir, "b")

			if !tt.missingA {
				err := ioutil.WriteFile(fileA, []byte(tt.a), 0666)
				if err != nil {
					t.Fatalf("failed to write %s: %s", fileA, err)
				}
			}

			if !tt.missingB {
				err := ioutil.WriteFile(fileB, []byte(tt.b), 0666)
				if err != nil {
					t.Fatalf("failed to write %s: %s", fileB, err)
				}
			}

			if got := filesHaveSameContents(fileA, fileB); got != tt.equal {
				t.Errorf("filesHaveSameContents() = %v, want %v", got, tt.equal)
			}
		})
	}
}
