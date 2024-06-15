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

	// The paths to the generated annotation-flags.csv files.
	AnnotationFlagsPaths android.Paths

	// The paths to the generated metadata.csv files.
	MetadataPaths android.Paths

	// The paths to the generated index.csv files.
	IndexPaths android.Paths

	// The subsets of the monolithic hiddenapi-stubs-flags.txt file that are provided by each
	// bootclasspath_fragment modules.
	StubFlagSubsets SignatureCsvSubsets

	// The subsets of the monolithic hiddenapi-flags.csv file that are provided by each
	// bootclasspath_fragment modules.
	FlagSubsets SignatureCsvSubsets

	// The classes jars from the libraries on the platform bootclasspath.
	ClassesJars android.Paths
}

// newMonolithicHiddenAPIInfo creates a new MonolithicHiddenAPIInfo from the flagFilesByCategory
// plus information provided by each of the fragments.
func newMonolithicHiddenAPIInfo(ctx android.ModuleContext, flagFilesByCategory FlagFilesByCategory, classpathElements ClasspathElements) MonolithicHiddenAPIInfo {
	monolithicInfo := MonolithicHiddenAPIInfo{}

	monolithicInfo.FlagsFilesByCategory = flagFilesByCategory

	// Merge all the information from the classpathElements. The fragments form a DAG so it is possible that
	// this will introduce duplicates so they will be resolved after processing all the classpathElements.
	for _, element := range classpathElements {
		switch e := element.(type) {
		case *ClasspathLibraryElement:
			classesJars := retrieveClassesJarsFromModule(e.Module())
			monolithicInfo.ClassesJars = append(monolithicInfo.ClassesJars, classesJars...)

		case *ClasspathFragmentElement:
			fragment := e.Module()
			if info, ok := android.OtherModuleProvider(ctx, fragment, HiddenAPIInfoProvider); ok {
				monolithicInfo.append(ctx, fragment, &info)
			} else {
				ctx.ModuleErrorf("%s does not provide hidden API information", fragment)
			}
		}
	}

	return monolithicInfo
}

// append appends all the files from the supplied info to the corresponding files in this struct.
func (i *MonolithicHiddenAPIInfo) append(ctx android.ModuleContext, otherModule android.Module, other *HiddenAPIInfo) {
	i.FlagsFilesByCategory.append(other.FlagFilesByCategory)
	i.AnnotationFlagsPaths = append(i.AnnotationFlagsPaths, other.AnnotationFlagsPath)
	i.MetadataPaths = append(i.MetadataPaths, other.MetadataPath)
	i.IndexPaths = append(i.IndexPaths, other.IndexPath)

	apexInfo, ok := android.OtherModuleProvider(ctx, otherModule, android.ApexInfoProvider)
	if !ok {
		ctx.ModuleErrorf("Could not determine min_version_version of %s\n", otherModule.Name())
		return
	}
	if apexInfo.MinSdkVersion.LessThanOrEqualTo(android.ApiLevelR) {
		// Restrict verify_overlaps to R and older modules.
		// The runtime in S does not have the same restriction that
		// requires the hiddenapi flags to be generated in a monolithic
		// invocation.
		i.StubFlagSubsets = append(i.StubFlagSubsets, other.StubFlagSubset())
		i.FlagSubsets = append(i.FlagSubsets, other.FlagSubset())
	}
}

var MonolithicHiddenAPIInfoProvider = blueprint.NewProvider[MonolithicHiddenAPIInfo]()
