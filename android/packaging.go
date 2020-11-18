// Copyright 2020 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License")
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

// PackagingSpec abstracts a request to place a built artifact at a certain path in a package.
// A package can be the traditional <partition>.img, but isn't limited to those. Other examples could
// be a new filesystem image that is a subset of system.img (e.g. for an Android-like mini OS running
// on a VM), or a zip archive for some of the host tools.
type PackagingSpec struct {
	// Path relative to the root of the package
	relPathInPackage string

	// The path to the built artifact
	srcPath Path

	// If this is not empty, then relPathInPackage should be a symlink to this target. (Then
	// srcPath is of course ignored.)
	symlinkTarget string

	// Whether relPathInPackage should be marked as executable or not
	executable bool
}
