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

func (mod *Module) ExcludeFromVendorSnapshot() bool {
	// TODO Rust does not yet support snapshotting
	return false
}

func (mod *Module) ExcludeFromRecoverySnapshot() bool {
	// TODO Rust does not yet support snapshotting
	return false
}

func (mod *Module) IsSnapshotLibrary() bool {
	// TODO Rust does not yet support snapshotting
	return false
}

func (mod *Module) SnapshotRuntimeLibs() []string {
	// TODO Rust does not yet support a runtime libs notion similar to CC
	return []string{}
}

func (mod *Module) SnapshotSharedLibs() []string {
	// TODO Rust does not yet support snapshotting
	return []string{}
}

func (mod *Module) Symlinks() []string {
	// TODO update this to return the list of symlinks when Rust supports defining symlinks
	return nil
}

func (m *Module) SnapshotHeaders() android.Paths {
	// TODO Rust does not yet support snapshotting
	return android.Paths{}
}
