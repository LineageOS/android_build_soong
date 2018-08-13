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

package cc

import (
	"testing"
)

func TestSplitFileExt(t *testing.T) {
	t.Run("soname with version", func(t *testing.T) {
		root, suffix, ext := splitFileExt("libtest.so.1.0.30")
		expected := "libtest"
		if root != expected {
			t.Errorf("root should be %q but got %q", expected, root)
		}
		expected = ".so.1.0.30"
		if suffix != expected {
			t.Errorf("suffix should be %q but got %q", expected, suffix)
		}
		expected = ".so"
		if ext != expected {
			t.Errorf("ext should be %q but got %q", expected, ext)
		}
	})

	t.Run("version numbers in the middle should be ignored", func(t *testing.T) {
		root, suffix, ext := splitFileExt("libtest.1.0.30.so")
		expected := "libtest.1.0.30"
		if root != expected {
			t.Errorf("root should be %q but got %q", expected, root)
		}
		expected = ".so"
		if suffix != expected {
			t.Errorf("suffix should be %q but got %q", expected, suffix)
		}
		expected = ".so"
		if ext != expected {
			t.Errorf("ext should be %q but got %q", expected, ext)
		}
	})

	t.Run("no known file extension", func(t *testing.T) {
		root, suffix, ext := splitFileExt("test.exe")
		expected := "test"
		if root != expected {
			t.Errorf("root should be %q but got %q", expected, root)
		}
		expected = ".exe"
		if suffix != expected {
			t.Errorf("suffix should be %q but got %q", expected, suffix)
		}
		if ext != expected {
			t.Errorf("ext should be %q but got %q", expected, ext)
		}
	})
}
