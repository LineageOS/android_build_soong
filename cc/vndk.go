// Copyright 2017 Google Inc. All rights reserved.
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

package cc

import (
	"strings"

	"android/soong/android"
)

const (
	llndkLibrariesTxt       = "llndk.libraries.txt"
	vndkCoreLibrariesTxt    = "vndkcore.libraries.txt"
	vndkSpLibrariesTxt      = "vndksp.libraries.txt"
	vndkPrivateLibrariesTxt = "vndkprivate.libraries.txt"
	vndkProductLibrariesTxt = "vndkproduct.libraries.txt"
)

func VndkLibrariesTxtModules(vndkVersion string, ctx android.BaseModuleContext) []string {
	// Snapshot vndks have their own *.libraries.VER.txt files.
	// Note that snapshots don't have "vndkcorevariant.libraries.VER.txt"
	result := []string{
		insertVndkVersion(vndkCoreLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkSpLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkPrivateLibrariesTxt, vndkVersion),
		insertVndkVersion(vndkProductLibrariesTxt, vndkVersion),
		insertVndkVersion(llndkLibrariesTxt, vndkVersion),
	}

	return result
}

type VndkProperties struct {
	Vndk struct {
		// declared as a VNDK or VNDK-SP module. The vendor variant
		// will be installed in /system instead of /vendor partition.
		//
		// `vendor_available` and `product_available` must be explicitly
		// set to either true or false together with `vndk: {enabled: true}`.
		Enabled *bool

		// declared as a VNDK-SP module, which is a subset of VNDK.
		//
		// `vndk: { enabled: true }` must set together.
		//
		// All these modules are allowed to link to VNDK-SP or LL-NDK
		// modules only. Other dependency will cause link-type errors.
		//
		// If `support_system_process` is not set or set to false,
		// the module is VNDK-core and can link to other VNDK-core,
		// VNDK-SP or LL-NDK modules only.
		Support_system_process *bool

		// declared as a VNDK-private module.
		// This module still creates the vendor and product variants refering
		// to the `vendor_available: true` and `product_available: true`
		// properties. However, it is only available to the other VNDK modules
		// but not to the non-VNDK vendor or product modules.
		Private *bool

		// Extending another module
		Extends *string
	}
}

func insertVndkVersion(filename string, vndkVersion string) string {
	if index := strings.LastIndex(filename, "."); index != -1 {
		return filename[:index] + "." + vndkVersion + filename[index:]
	}
	return filename
}
