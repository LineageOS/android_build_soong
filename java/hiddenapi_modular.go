// Copyright (C) 2021 The Android Open Source Project
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

package java

import (
	"android/soong/android"
)

// Contains support for processing hiddenAPI in a modular fashion.

// hiddenAPIAugmentationInfo contains paths to the files that can be used to augment the information
// obtained from annotations within the source code in order to create the complete set of flags
// that should be applied to the dex implementation jars on the bootclasspath.
//
// Each property contains a list of paths. With the exception of the Unsupported_packages the paths
// of each property reference a plain text file that contains a java signature per line. The flags
// for each of those signatures will be updated in a property specific way.
//
// The Unsupported_packages property contains a list of paths, each of which is a plain text file
// with one Java package per line. All members of all classes within that package (but not nested
// packages) will be updated in a property specific way.
type hiddenAPIAugmentationInfo struct {
	// Marks each signature in the referenced files as being unsupported.
	Unsupported android.Paths

	// Marks each signature in the referenced files as being unsupported because it has been removed.
	// Any conflicts with other flags are ignored.
	Removed android.Paths

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= R
	// and low priority.
	Max_target_r_low_priority android.Paths

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= Q.
	Max_target_q android.Paths

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= P.
	Max_target_p android.Paths

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= O
	// and low priority. Any conflicts with other flags are ignored.
	Max_target_o_low_priority android.Paths

	// Marks each signature in the referenced files as being blocked.
	Blocked android.Paths

	// Marks each signature in every package in the referenced files as being unsupported.
	Unsupported_packages android.Paths
}
