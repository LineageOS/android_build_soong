// Copyright 2020 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package cc

// This file contains utility types and functions for VNDK / vendor snapshot.

import (
	"android/soong/android"
)

var (
	headerExts = []string{".h", ".hh", ".hpp", ".hxx", ".h++", ".inl", ".inc", ".ipp", ".h.generic"}
)

// snapshotLibraryInterface is an interface for libraries captured to VNDK / vendor snapshots.
type snapshotLibraryInterface interface {
	libraryInterface

	// collectHeadersForSnapshot is called in GenerateAndroidBuildActions for snapshot aware
	// modules (See isSnapshotAware below).
	// This function should gather all headers needed for snapshot.
	collectHeadersForSnapshot(ctx android.ModuleContext)

	// snapshotHeaders should return collected headers by collectHeadersForSnapshot.
	// Calling snapshotHeaders before collectHeadersForSnapshot is an error.
	snapshotHeaders() android.Paths
}

var _ snapshotLibraryInterface = (*prebuiltLibraryLinker)(nil)
var _ snapshotLibraryInterface = (*libraryDecorator)(nil)

// snapshotMap is a helper wrapper to a map from base module name to snapshot module name.
type snapshotMap struct {
	snapshots map[string]string
}

func newSnapshotMap() *snapshotMap {
	return &snapshotMap{
		snapshots: make(map[string]string),
	}
}

func snapshotMapKey(name string, arch android.ArchType) string {
	return name + ":" + arch.String()
}

// Adds a snapshot name for given module name and architecture.
// e.g. add("libbase", X86, "libbase.vndk.29.x86")
func (s *snapshotMap) add(name string, arch android.ArchType, snapshot string) {
	s.snapshots[snapshotMapKey(name, arch)] = snapshot
}

// Returns snapshot name for given module name and architecture, if found.
// e.g. get("libcutils", X86) => "libcutils.vndk.29.x86", true
func (s *snapshotMap) get(name string, arch android.ArchType) (snapshot string, found bool) {
	snapshot, found = s.snapshots[snapshotMapKey(name, arch)]
	return snapshot, found
}

// shouldCollectHeadersForSnapshot determines if the module is a possible candidate for snapshot.
// If it's true, collectHeadersForSnapshot will be called in GenerateAndroidBuildActions.
func shouldCollectHeadersForSnapshot(ctx android.ModuleContext, m *Module, apexInfo android.ApexInfo) bool {
	if ctx.DeviceConfig().VndkVersion() != "current" &&
		ctx.DeviceConfig().RecoverySnapshotVersion() != "current" {
		return false
	}
	if _, _, ok := isVndkSnapshotAware(ctx.DeviceConfig(), m, apexInfo); ok {
		return ctx.Config().VndkSnapshotBuildArtifacts()
	} else if isVendorSnapshotAware(m, isVendorProprietaryPath(ctx.ModuleDir()), apexInfo) ||
		isRecoverySnapshotAware(m, isRecoveryProprietaryPath(ctx.ModuleDir()), apexInfo) {
		return true
	}
	return false
}
