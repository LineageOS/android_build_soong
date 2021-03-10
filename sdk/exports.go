// Copyright (C) 2019 The Android Open Source Project
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

package sdk

import "android/soong/android"

func init() {
	registerModuleExportsBuildComponents(android.InitRegistrationContext)
}

func registerModuleExportsBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("module_exports", ModuleExportsFactory)
	ctx.RegisterModuleType("module_exports_snapshot", ModuleExportsSnapshotsFactory)
}

// module_exports defines the exports of a mainline module. The exports are Soong modules
// which are required by Soong modules that are not part of the mainline module.
func ModuleExportsFactory() android.Module {
	return newSdkModule(true)
}

// module_exports_snapshot is a versioned snapshot of prebuilt versions of all the exports
// of a mainline module.
func ModuleExportsSnapshotsFactory() android.Module {
	s := newSdkModule(true)
	s.properties.Snapshot = true
	return s
}
