// Copyright 2021 The Android Open Source Project
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
package snapshot

import (
	"android/soong/android"
)

var pctx = android.NewPackageContext("android/soong/snapshot")

func init() {
	pctx.Import("android/soong/android")
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
	Required  []string `json:",omitempty"`
	Overrides []string `json:",omitempty"`

	// license information
	LicenseKinds []string `json:",omitempty"`
	LicenseTexts []string `json:",omitempty"`
}

func (prop *SnapshotJsonFlags) InitBaseSnapshotPropsWithName(m android.Module, name string) {
	prop.ModuleName = name

	prop.LicenseKinds = m.EffectiveLicenseKinds()
	prop.LicenseTexts = m.EffectiveLicenseFiles().Strings()
}

func (prop *SnapshotJsonFlags) InitBaseSnapshotProps(m android.Module) {
	prop.InitBaseSnapshotPropsWithName(m, m.Name())
}
