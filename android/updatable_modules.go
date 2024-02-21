// Copyright (C) 2022 The Android Open Source Project
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

package android

// This file contains branch specific constants. They are stored in a separate
// file to minimise the potential of merge conflicts between branches when
// the code from the package is changed.

// The default manifest version for all the modules on this branch.
// This version code will be used only if there is no version field in the
// module's apex_manifest.json. Release branches have their version injected
// into apex_manifest.json by the tooling and will not use the version set
// here. Developers can also set the version field locally in the
// apex_manifest.json to build a module with a specific version.
//
// The value follows the schema from go/mainline-version-codes, and is chosen
// based on the branch such that the builds from testing and development
// branches will have a version higher than the prebuilts.
// Versions per branch:
// * x-dev           - xx0090000 (where xx is the branch SDK level)
// * AOSP            - xx9990000
// * x-mainline-prod - xx9990000
// * master          - 990090000
const DefaultUpdatableModuleVersion = "350090000"
