// Copyright 2020 Google Inc. All rights reserved.
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

package bazel

type bazelModuleProperties struct {
	// The label of the Bazel target replacing this Soong module.
	Label string
}

// Properties contains common module properties for Bazel migration purposes.
type Properties struct {
	// In USE_BAZEL_ANALYSIS=1 mode, this represents the Bazel target replacing
	// this Soong module.
	Bazel_module bazelModuleProperties
}

// BazelTargetModuleProperties contain properties and metadata used for
// Blueprint to BUILD file conversion.
type BazelTargetModuleProperties struct {
	// The Bazel rule class for this target.
	Rule_class string

	// The target label for the bzl file containing the definition of the rule class.
	Bzl_load_location string
}

// Label is used to represent a Bazel compatible Label. Also stores the original bp text to support
// string replacement.
type Label struct {
	Bp_text string
	Label   string
}

// LabelList is used to represent a list of Bazel labels.
type LabelList struct {
	Includes []Label
	Excludes []Label
}

// Append appends the fields of other labelList to the corresponding fields of ll.
func (ll *LabelList) Append(other LabelList) {
	if len(ll.Includes) > 0 || len(other.Includes) > 0 {
		ll.Includes = append(ll.Includes, other.Includes...)
	}
	if len(ll.Excludes) > 0 || len(other.Excludes) > 0 {
		ll.Excludes = append(other.Excludes, other.Excludes...)
	}
}
