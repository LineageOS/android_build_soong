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

package allowlists

const (
	// Modules with build time of more than half a minute should have high priority.
	DEFAULT_PRIORITIZED_WEIGHT = 1000
	// Modules with build time of more than a few minute should have higher priority.
	HIGH_PRIORITIZED_WEIGHT = 10 * DEFAULT_PRIORITIZED_WEIGHT
	// Modules with inputs greater than the threshold should have high priority.
	// Adjust this threshold if there are lots of wrong predictions.
	INPUT_SIZE_THRESHOLD = 50
)

var (
	// The list of module types which are expected to spend lots of build time.
	// With `--ninja_weight_source=soong`, ninja builds these module types and deps first.
	HugeModuleTypePrefixMap = map[string]int{
		"rust_":       HIGH_PRIORITIZED_WEIGHT,
		"droidstubs":  DEFAULT_PRIORITIZED_WEIGHT,
		"art_":        DEFAULT_PRIORITIZED_WEIGHT,
		"ndk_library": DEFAULT_PRIORITIZED_WEIGHT,
	}
)
