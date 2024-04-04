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

// This file defines snapshot prebuilt modules, e.g. vendor snapshot and recovery snapshot. Such
// snapshot modules will override original source modules with setting BOARD_VNDK_VERSION, with
// snapshot mutators and snapshot information maps which are also defined in this file.

import (
	"android/soong/android"
)

type SnapshotInterface interface {
	MatchesWithDevice(config android.DeviceConfig) bool
	IsSnapshotPrebuilt() bool
	Version() string
	SnapshotAndroidMkSuffix() string
}

var _ SnapshotInterface = (*vndkPrebuiltLibraryDecorator)(nil)
