// Copyright 2021 The Android Open Source Project
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

package rust

import (
	"android/soong/android"
)

// snapshotLibraryInterface is an interface for libraries captured to VNDK / vendor snapshots.
type snapshotLibraryInterface interface {
	libraryInterface

	// collectHeadersForSnapshot is called in GenerateAndroidBuildActions for snapshot aware
	// modules (See isSnapshotAware below).
	// This function should gather all headers needed for snapshot.
	collectHeadersForSnapshot(ctx android.ModuleContext, deps PathDeps)

	// snapshotHeaders should return collected headers by collectHeadersForSnapshot.
	// Calling snapshotHeaders before collectHeadersForSnapshot is an error.
	snapshotHeaders() android.Paths
}

func (mod *Module) ExcludeFromVendorSnapshot() bool {
	return Bool(mod.Properties.Exclude_from_vendor_snapshot)
}

func (mod *Module) ExcludeFromRecoverySnapshot() bool {
	return Bool(mod.Properties.Exclude_from_recovery_snapshot)
}

func (mod *Module) IsSnapshotLibrary() bool {
	if lib, ok := mod.compiler.(libraryInterface); ok {
		return lib.shared() || lib.static() || lib.rlib() || lib.dylib()
	}
	return false
}

func (mod *Module) SnapshotRuntimeLibs() []string {
	// TODO Rust does not yet support a runtime libs notion similar to CC
	return []string{}
}

func (mod *Module) SnapshotSharedLibs() []string {
	return mod.Properties.SnapshotSharedLibs
}

func (mod *Module) SnapshotStaticLibs() []string {
	return mod.Properties.SnapshotStaticLibs
}

func (mod *Module) SnapshotRlibs() []string {
	return mod.Properties.SnapshotRlibs
}

func (mod *Module) SnapshotDylibs() []string {
	return mod.Properties.SnapshotDylibs
}

func (mod *Module) Symlinks() []string {
	// TODO update this to return the list of symlinks when Rust supports defining symlinks
	return nil
}

func (m *Module) SnapshotHeaders() android.Paths {
	if l, ok := m.compiler.(snapshotLibraryInterface); ok {
		return l.snapshotHeaders()
	}
	return android.Paths{}
}
