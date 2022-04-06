// Copyright 2021 The Android Open Source Project
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
package snapshot

import (
	"android/soong/android"
	"path/filepath"
)

// Interface for modules which can be captured in the snapshot.
type SnapshotModuleInterfaceBase interface{}

// Defines the specifics of different images to which the snapshot process is applicable, e.g.,
// vendor, recovery, ramdisk.
type SnapshotImage interface {
	// Returns true if a snapshot should be generated for this image.
	shouldGenerateSnapshot(ctx android.SingletonContext) bool

	// Function that returns true if the module is included in this image.
	// Using a function return instead of a value to prevent early
	// evalution of a function that may be not be defined.
	InImage(m SnapshotModuleInterfaceBase) func() bool

	// Returns true if a dir under source tree is an SoC-owned proprietary
	// directory, such as device/, vendor/, etc.
	//
	// For a given snapshot (e.g., vendor, recovery, etc.) if
	// isProprietaryPath(dir, deviceConfig) returns true, then the module in dir
	// will be built from sources.
	IsProprietaryPath(dir string, deviceConfig android.DeviceConfig) bool

	// Whether a given module has been explicitly excluded from the
	// snapshot, e.g., using the exclude_from_vendor_snapshot or
	// exclude_from_recovery_snapshot properties.
	ExcludeFromSnapshot(m SnapshotModuleInterfaceBase) bool

	// Returns true if the build is using a snapshot for this image.
	IsUsingSnapshot(cfg android.DeviceConfig) bool

	// Returns a version of which the snapshot should be used in this target.
	// This will only be meaningful when isUsingSnapshot is true.
	TargetSnapshotVersion(cfg android.DeviceConfig) string

	// Whether to exclude a given module from the directed snapshot or not.
	// If the makefile variable DIRECTED_{IMAGE}_SNAPSHOT is true, directed snapshot is turned on,
	// and only modules listed in {IMAGE}_SNAPSHOT_MODULES will be captured.
	ExcludeFromDirectedSnapshot(cfg android.DeviceConfig, name string) bool

	// Returns target image name
	ImageName() string
}

type directoryMap map[string]bool

var (
	// Modules under following directories are ignored. They are OEM's and vendor's
	// proprietary modules(device/, kernel/, vendor/, and hardware/).
	defaultDirectoryExcludedMap = directoryMap{
		"device":   true,
		"hardware": true,
		"kernel":   true,
		"vendor":   true,
	}

	// Modules under following directories are included as they are in AOSP,
	// although hardware/ and kernel/ are normally for vendor's own.
	defaultDirectoryIncludedMap = directoryMap{
		"kernel/configs":              true,
		"kernel/prebuilts":            true,
		"kernel/tests":                true,
		"hardware/interfaces":         true,
		"hardware/libhardware":        true,
		"hardware/libhardware_legacy": true,
		"hardware/ril":                true,
	}
)

func isDirectoryExcluded(dir string, excludedMap directoryMap, includedMap directoryMap) bool {
	if dir == "." || dir == "/" {
		return false
	}
	if includedMap[dir] {
		return false
	} else if excludedMap[dir] {
		return true
	} else if defaultDirectoryIncludedMap[dir] {
		return false
	} else if defaultDirectoryExcludedMap[dir] {
		return true
	} else {
		return isDirectoryExcluded(filepath.Dir(dir), excludedMap, includedMap)
	}
}

// This is to be saved as .json files, which is for development/vendor_snapshot/update.py.
// These flags become Android.bp snapshot module properties.
//
// Attributes are optional and will be populated based on each module's need.
// Common attributes are defined here, languages may extend this struct to add
// additional attributes.
type SnapshotJsonFlags struct {
	ModuleName          string `json:",omitempty"`
	RelativeInstallPath string `json:",omitempty"`
	Filename            string `json:",omitempty"`
	ModuleStemName      string `json:",omitempty"`
	RustProcMacro       bool   `json:",omitempty"`
	CrateName           string `json:",omitempty"`

	// dependencies
	Required []string `json:",omitempty"`
}
