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

package shared

import (
	"testing"
)

func assertEqual(t *testing.T, expected, actual string) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected %q != got %q", expected, actual)
	}
}

func TestJoinPath(t *testing.T) {
	assertEqual(t, "/a/b", JoinPath("c/d", "/a/b"))
	assertEqual(t, "a/b", JoinPath("a", "b"))
	assertEqual(t, "/a/b", JoinPath("x", "/a", "b"))
}
