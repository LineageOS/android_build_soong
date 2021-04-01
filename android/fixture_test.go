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

package android

import (
	"testing"
)

// Make sure that FixturePreparer instances are only called once per fixture and in the order in
// which they were added.
func TestFixtureDedup(t *testing.T) {
	list := []string{}

	appendToList := func(s string) FixturePreparer {
		return FixtureModifyConfig(func(_ Config) {
			list = append(list, s)
		})
	}

	preparer1 := appendToList("preparer1")
	preparer2 := appendToList("preparer2")
	preparer3 := appendToList("preparer3")
	preparer4 := OptionalFixturePreparer(appendToList("preparer4"))
	nilPreparer := OptionalFixturePreparer(nil)

	preparer1Then2 := GroupFixturePreparers(preparer1, preparer2, nilPreparer)

	preparer2Then1 := GroupFixturePreparers(preparer2, preparer1)

	group := GroupFixturePreparers(preparer1, preparer2, preparer1, preparer1Then2)

	extension := GroupFixturePreparers(group, preparer4, preparer2)

	GroupFixturePreparers(extension, preparer1, preparer2, preparer2Then1, preparer3).Fixture(t)

	AssertDeepEquals(t, "preparers called in wrong order",
		[]string{"preparer1", "preparer2", "preparer4", "preparer3"}, list)
}

func TestFixtureValidateMockFS(t *testing.T) {
	t.Run("absolute path", func(t *testing.T) {
		AssertPanicMessageContains(t, "source path validation failed", "Path is outside directory: /abs/path/Android.bp", func() {
			FixtureAddFile("/abs/path/Android.bp", nil).Fixture(t)
		})
	})
	t.Run("not canonical", func(t *testing.T) {
		AssertPanicMessageContains(t, "source path validation failed", `path "path/with/../in/it/Android.bp" is not a canonical path, use "path/in/it/Android.bp" instead`, func() {
			FixtureAddFile("path/with/../in/it/Android.bp", nil).Fixture(t)
		})
	})
	t.Run("FixtureAddFile", func(t *testing.T) {
		AssertPanicMessageContains(t, "source path validation failed", `cannot add output path "out/Android.bp" to the mock file system`, func() {
			FixtureAddFile("out/Android.bp", nil).Fixture(t)
		})
	})
	t.Run("FixtureMergeMockFs", func(t *testing.T) {
		AssertPanicMessageContains(t, "source path validation failed", `cannot add output path "out/Android.bp" to the mock file system`, func() {
			FixtureMergeMockFs(MockFS{
				"out/Android.bp": nil,
			}).Fixture(t)
		})
	})
	t.Run("FixtureModifyMockFS", func(t *testing.T) {
		AssertPanicMessageContains(t, "source path validation failed", `cannot add output path "out/Android.bp" to the mock file system`, func() {
			FixtureModifyMockFS(func(fs MockFS) {
				fs["out/Android.bp"] = nil
			}).Fixture(t)
		})
	})
}
