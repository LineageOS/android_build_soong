// Copyright 2024 Google Inc. All rights reserved.
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

// For storing user-supplied properties about source code on a module to be queried later.
type SourceProperties struct {
	// Indicates that the module and its source code are only used in tests, not
	// production code. Used by coverage reports and potentially other tools.
	Test_only *bool
	// Used internally to write if this is a top level test target.
	// i.e. something that can be run directly or through tradefed as a test.
	// `java_library` would be false, `java_test` would be true.
	Top_level_test_target bool `blueprint:"mutated"`
}
