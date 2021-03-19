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

import "testing"

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

	buildDir := "build"
	factory := NewFixtureFactory(&buildDir, preparer1, preparer2, preparer1, preparer1Then2)

	extension := factory.Extend(preparer4, preparer2)

	extension.Fixture(t, preparer1, preparer2, preparer2Then1, preparer3)

	AssertDeepEquals(t, "preparers called in wrong order",
		[]string{"preparer1", "preparer2", "preparer4", "preparer3"}, list)
}
