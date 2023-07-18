// Copyright 2020 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	HeaderExts = []string{".h", ".hh", ".hpp", ".hxx", ".h++", ".inl", ".inc", ".ipp", ".h.generic"}
)

func (m *Module) IsSnapshotLibrary() bool {
	if _, ok := m.linker.(snapshotLibraryInterface); ok {
		return true
	}
	return false
}

func (m *Module) SnapshotHeaders() android.Paths {
	if m.IsSnapshotLibrary() {
		return m.linker.(snapshotLibraryInterface).snapshotHeaders()
	}
	return android.Paths{}
}

func (m *Module) Dylib() bool {
	return false
}

func (m *Module) Rlib() bool {
	return false
}

func (m *Module) SnapshotRuntimeLibs() []string {
	return m.Properties.SnapshotRuntimeLibs
}

func (m *Module) SnapshotSharedLibs() []string {
	return m.Properties.SnapshotSharedLibs
}

func (m *Module) SnapshotStaticLibs() []string {
	return m.Properties.SnapshotStaticLibs
}

func (m *Module) SnapshotRlibs() []string {
	return []string{}
}

func (m *Module) SnapshotDylibs() []string {
	return []string{}
}

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

// ShouldCollectHeadersForSnapshot determines if the module is a possible candidate for snapshot.
// If it's true, collectHeadersForSnapshot will be called in GenerateAndroidBuildActions.
func ShouldCollectHeadersForSnapshot(ctx android.ModuleContext, m LinkableInterface, apexInfo android.ApexInfo) bool {
	if ctx.DeviceConfig().VndkVersion() != "current" &&
		ctx.DeviceConfig().RecoverySnapshotVersion() != "current" {
		return false
	}
	if _, ok := isVndkSnapshotAware(ctx.DeviceConfig(), m, apexInfo); ok {
		return ctx.Config().VndkSnapshotBuildArtifacts()
	}

	for _, image := range []SnapshotImage{VendorSnapshotImageSingleton, RecoverySnapshotImageSingleton} {
		if isSnapshotAware(ctx.DeviceConfig(), m, image.IsProprietaryPath(ctx.ModuleDir(), ctx.DeviceConfig()), apexInfo, image) {
			return true
		}
	}
	return false
}
