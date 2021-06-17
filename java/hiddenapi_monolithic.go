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
	"github.com/google/blueprint"
)

// MonolithicHiddenAPIInfo contains information needed/provided by the hidden API generation of the
// monolithic hidden API files.
//
// Each list of paths includes all the equivalent paths from each of the bootclasspath_fragment
// modules that contribute to the platform-bootclasspath.
type MonolithicHiddenAPIInfo struct {
	// FlagsFilesByCategory maps from the flag file category to the paths containing information for
	// that category.
	FlagsFilesByCategory FlagFilesByCategory

	// The paths to the generated stub-flags.csv files.
	StubFlagsPaths android.Paths

	// The paths to the generated annotation-flags.csv files.
	AnnotationFlagsPaths android.Paths

	// The paths to the generated metadata.csv files.
	MetadataPaths android.Paths

	// The paths to the generated index.csv files.
	IndexPaths android.Paths

	// The paths to the generated all-flags.csv files.
	AllFlagsPaths android.Paths
}

// newMonolithicHiddenAPIInfo creates a new MonolithicHiddenAPIInfo from the flagFilesByCategory
// plus information provided by each of the fragments.
func newMonolithicHiddenAPIInfo(ctx android.ModuleContext, flagFilesByCategory FlagFilesByCategory, fragments []android.Module) MonolithicHiddenAPIInfo {
	monolithicInfo := MonolithicHiddenAPIInfo{}

	monolithicInfo.FlagsFilesByCategory = flagFilesByCategory

	// Merge all the information from the fragments. The fragments form a DAG so it is possible that
	// this will introduce duplicates so they will be resolved after processing all the fragments.
	for _, fragment := range fragments {
		if ctx.OtherModuleHasProvider(fragment, HiddenAPIInfoProvider) {
			info := ctx.OtherModuleProvider(fragment, HiddenAPIInfoProvider).(HiddenAPIInfo)
			monolithicInfo.append(&info)
		}
	}

	// Dedup paths.
	monolithicInfo.dedup()

	return monolithicInfo
}

// append appends all the files from the supplied info to the corresponding files in this struct.
func (i *MonolithicHiddenAPIInfo) append(other *HiddenAPIInfo) {
	i.FlagsFilesByCategory.append(other.FlagFilesByCategory)

	// The output may not be set if the bootclasspath_fragment has not yet been updated to support
	// hidden API processing.
	// TODO(b/179354495): Switch back to append once all bootclasspath_fragment modules have been
	//  updated to support hidden API processing properly.
	appendIfNotNil := func(paths android.Paths, path android.Path) android.Paths {
		if path == nil {
			return paths
		}
		return append(paths, path)
	}
	i.StubFlagsPaths = appendIfNotNil(i.StubFlagsPaths, other.StubFlagsPath)
	i.AnnotationFlagsPaths = appendIfNotNil(i.AnnotationFlagsPaths, other.AnnotationFlagsPath)
	i.MetadataPaths = appendIfNotNil(i.MetadataPaths, other.MetadataPath)
	i.IndexPaths = appendIfNotNil(i.IndexPaths, other.IndexPath)
	i.AllFlagsPaths = appendIfNotNil(i.AllFlagsPaths, other.AllFlagsPath)
}

// dedup removes duplicates in all the paths, while maintaining the order in which they were
// appended.
func (i *MonolithicHiddenAPIInfo) dedup() {
	i.FlagsFilesByCategory.dedup()
	i.StubFlagsPaths = android.FirstUniquePaths(i.StubFlagsPaths)
	i.AnnotationFlagsPaths = android.FirstUniquePaths(i.AnnotationFlagsPaths)
	i.MetadataPaths = android.FirstUniquePaths(i.MetadataPaths)
	i.IndexPaths = android.FirstUniquePaths(i.IndexPaths)
	i.AllFlagsPaths = android.FirstUniquePaths(i.AllFlagsPaths)
}

var MonolithicHiddenAPIInfoProvider = blueprint.NewProvider(MonolithicHiddenAPIInfo{})
