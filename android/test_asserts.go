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
	"reflect"
	"strings"
	"testing"
)

// This file contains general purpose test assert functions.

// AssertBoolEquals checks if the expected and actual values are equal and if they are not then it
// reports an error prefixed with the supplied message and including a reason for why it failed.
func AssertBoolEquals(t *testing.T, message string, expected bool, actual bool) {
	t.Helper()
	if actual != expected {
		t.Errorf("%s: expected %t, actual %t", message, expected, actual)
	}
}

// AssertStringEquals checks if the expected and actual values are equal and if they are not then
// it reports an error prefixed with the supplied message and including a reason for why it failed.
func AssertStringEquals(t *testing.T, message string, expected string, actual string) {
	t.Helper()
	if actual != expected {
		t.Errorf("%s: expected %s, actual %s", message, expected, actual)
	}
}

// AssertPathRelativeToTopEquals checks if the expected value is equal to the result of calling
// PathRelativeToTop on the actual Path.
func AssertPathRelativeToTopEquals(t *testing.T, message string, expected string, actual Path) {
	t.Helper()
	AssertStringEquals(t, message, expected, PathRelativeToTop(actual))
}

// AssertPathsRelativeToTopEquals checks if the expected value is equal to the result of calling
// PathsRelativeToTop on the actual Paths.
func AssertPathsRelativeToTopEquals(t *testing.T, message string, expected []string, actual Paths) {
	t.Helper()
	AssertDeepEquals(t, message, expected, PathsRelativeToTop(actual))
}

// AssertStringPathRelativeToTopEquals checks if the expected value is equal to the result of calling
// StringPathRelativeToTop on the actual string path.
func AssertStringPathRelativeToTopEquals(t *testing.T, message string, config Config, expected string, actual string) {
	t.Helper()
	AssertStringEquals(t, message, expected, StringPathRelativeToTop(config.buildDir, actual))
}

// AssertStringPathsRelativeToTopEquals checks if the expected value is equal to the result of
// calling StringPathsRelativeToTop on the actual string paths.
func AssertStringPathsRelativeToTopEquals(t *testing.T, message string, config Config, expected []string, actual []string) {
	t.Helper()
	AssertDeepEquals(t, message, expected, StringPathsRelativeToTop(config.buildDir, actual))
}

// AssertErrorMessageEquals checks if the error is not nil and has the expected message. If it does
// not then this reports an error prefixed with the supplied message and including a reason for why
// it failed.
func AssertErrorMessageEquals(t *testing.T, message string, expected string, actual error) {
	t.Helper()
	if actual == nil {
		t.Errorf("Expected error but was nil")
	} else if actual.Error() != expected {
		t.Errorf("%s: expected %s, actual %s", message, expected, actual.Error())
	}
}

// AssertTrimmedStringEquals checks if the expected and actual values are the same after trimming
// leading and trailing spaces from them both. If they are not then it reports an error prefixed
// with the supplied message and including a reason for why it failed.
func AssertTrimmedStringEquals(t *testing.T, message string, expected string, actual string) {
	t.Helper()
	AssertStringEquals(t, message, strings.TrimSpace(expected), strings.TrimSpace(actual))
}

// AssertStringDoesContain checks if the string contains the expected substring. If it does not
// then it reports an error prefixed with the supplied message and including a reason for why it
// failed.
func AssertStringDoesContain(t *testing.T, message string, s string, expectedSubstring string) {
	t.Helper()
	if !strings.Contains(s, expectedSubstring) {
		t.Errorf("%s: could not find %q within %q", message, expectedSubstring, s)
	}
}

// AssertStringDoesNotContain checks if the string contains the expected substring. If it does then
// it reports an error prefixed with the supplied message and including a reason for why it failed.
func AssertStringDoesNotContain(t *testing.T, message string, s string, unexpectedSubstring string) {
	t.Helper()
	if strings.Contains(s, unexpectedSubstring) {
		t.Errorf("%s: unexpectedly found %q within %q", message, unexpectedSubstring, s)
	}
}

// AssertStringListContains checks if the list of strings contains the expected string. If it does
// not then it reports an error prefixed with the supplied message and including a reason for why it
// failed.
func AssertStringListContains(t *testing.T, message string, list []string, expected string) {
	t.Helper()
	if !InList(expected, list) {
		t.Errorf("%s: could not find %q within %q", message, expected, list)
	}
}

// AssertArrayString checks if the expected and actual values are equal and if they are not then it
// reports an error prefixed with the supplied message and including a reason for why it failed.
func AssertArrayString(t *testing.T, message string, expected, actual []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("%s: expected %d (%q), actual (%d) %q", message, len(expected), expected, len(actual), actual)
		return
	}
	for i := range actual {
		if actual[i] != expected[i] {
			t.Errorf("%s: expected %d-th, %q (%q), actual %q (%q)",
				message, i, expected[i], expected, actual[i], actual)
			return
		}
	}
}

// AssertDeepEquals checks if the expected and actual values are equal using reflect.DeepEqual and
// if they are not then it reports an error prefixed with the supplied message and including a
// reason for why it failed.
func AssertDeepEquals(t *testing.T, message string, expected interface{}, actual interface{}) {
	t.Helper()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("%s: expected:\n  %#v\n got:\n  %#v", message, expected, actual)
	}
}

// AssertPanic checks that the supplied function panics as expected.
func AssertPanic(t *testing.T, message string, funcThatShouldPanic func()) {
	t.Helper()
	panicked := false
	func() {
		defer func() {
			if x := recover(); x != nil {
				panicked = true
			}
		}()
		funcThatShouldPanic()
	}()
	if !panicked {
		t.Error(message)
	}
}
