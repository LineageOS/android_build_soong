// Copyright 2023 Google Inc. All rights reserved.
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

package starlark_import

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

func TestBasic(t *testing.T) {
	globals, _, err := runStarlarkFileWithFilesystem("a.bzl", "", map[string]string{
		"a.bzl": `
my_string = "hello, world!"
`})
	if err != nil {
		t.Error(err)
		return
	}

	if globals["my_string"].(starlark.String) != "hello, world!" {
		t.Errorf("Expected %q, got %q", "hello, world!", globals["my_string"].String())
	}
}

func TestLoad(t *testing.T) {
	globals, _, err := runStarlarkFileWithFilesystem("a.bzl", "", map[string]string{
		"a.bzl": `
load("//b.bzl", _b_string = "my_string")
my_string = "hello, " + _b_string
`,
		"b.bzl": `
my_string = "world!"
`})
	if err != nil {
		t.Error(err)
		return
	}

	if globals["my_string"].(starlark.String) != "hello, world!" {
		t.Errorf("Expected %q, got %q", "hello, world!", globals["my_string"].String())
	}
}

func TestLoadRelative(t *testing.T) {
	globals, ninjaDeps, err := runStarlarkFileWithFilesystem("a.bzl", "", map[string]string{
		"a.bzl": `
load(":b.bzl", _b_string = "my_string")
load("//foo/c.bzl", _c_string = "my_string")
my_string = "hello, " + _b_string
c_string = _c_string
`,
		"b.bzl": `
my_string = "world!"
`,
		"foo/c.bzl": `
load(":d.bzl", _d_string = "my_string")
my_string = "hello, " + _d_string
`,
		"foo/d.bzl": `
my_string = "world!"
`})
	if err != nil {
		t.Error(err)
		return
	}

	if globals["my_string"].(starlark.String) != "hello, world!" {
		t.Errorf("Expected %q, got %q", "hello, world!", globals["my_string"].String())
	}

	expectedNinjaDeps := []string{
		"a.bzl",
		"b.bzl",
		"foo/c.bzl",
		"foo/d.bzl",
	}
	if !slicesEqual(ninjaDeps, expectedNinjaDeps) {
		t.Errorf("Expected %v ninja deps, got %v", expectedNinjaDeps, ninjaDeps)
	}
}

func TestLoadCycle(t *testing.T) {
	_, _, err := runStarlarkFileWithFilesystem("a.bzl", "", map[string]string{
		"a.bzl": `
load(":b.bzl", _b_string = "my_string")
my_string = "hello, " + _b_string
`,
		"b.bzl": `
load(":a.bzl", _a_string = "my_string")
my_string = "hello, " + _a_string
`})
	if err == nil || !strings.Contains(err.Error(), "cycle in load graph") {
		t.Errorf("Expected cycle in load graph, got: %v", err)
		return
	}
}

func slicesEqual[T comparable](a []T, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
