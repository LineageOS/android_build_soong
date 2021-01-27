// Copyright 2019 Google Inc. All rights reserved.
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

package config

// List of VNDK libraries that have different core variant and vendor variant.
// For these libraries, the vendor variants must be installed even if the device
// has VndkUseCoreVariant set.
// TODO(b/150578172): clean up unstable and non-versioned aidl module
var VndkMustUseVendorVariantList = []string{
	"android.hardware.authsecret-unstable-ndk_platform",
	"android.hardware.authsecret-ndk_platform",
	"android.hardware.authsecret-V1-ndk_platform",
	"android.hardware.automotive.occupant_awareness-ndk_platform",
	"android.hardware.automotive.occupant_awareness-V1-ndk_platform",
	"android.hardware.gnss-unstable-ndk_platform",
	"android.hardware.gnss-ndk_platform",
	"android.hardware.gnss-V1-ndk_platform",
	"android.hardware.health.storage-V1-ndk_platform",
	"android.hardware.health.storage-ndk_platform",
	"android.hardware.health.storage-unstable-ndk_platform",
	"android.hardware.light-V1-ndk_platform",
	"android.hardware.light-ndk_platform",
	"android.hardware.identity-V2-ndk_platform",
	"android.hardware.identity-ndk_platform",
	"android.hardware.nfc@1.2",
	"android.hardware.memtrack-V1-ndk_platform",
	"android.hardware.memtrack-ndk_platform",
	"android.hardware.memtrack-unstable-ndk_platform",
	"android.hardware.oemlock-V1-ndk_platform",
	"android.hardware.oemlock-ndk_platform",
	"android.hardware.oemlock-unstable-ndk_platform",
	"android.hardware.power-V1-ndk_platform",
	"android.hardware.power-ndk_platform",
	"android.hardware.power.stats-V1-ndk_platform",
	"android.hardware.power.stats-ndk_platform",
	"android.hardware.power.stats-unstable-ndk_platform",
	"android.hardware.rebootescrow-V1-ndk_platform",
	"android.hardware.rebootescrow-ndk_platform",
	"android.hardware.security.keymint-V1-ndk_platform",
	"android.hardware.security.keymint-ndk_platform",
	"android.hardware.security.keymint-unstable-ndk_platform",
	"android.hardware.security.secureclock-V1-ndk_platform",
	"android.hardware.security.secureclock-unstable-ndk_platform",
	"android.hardware.security.secureclock-ndk_platform",
	"android.hardware.security.sharedsecret-V1-ndk_platform",
	"android.hardware.security.sharedsecret-ndk_platform",
	"android.hardware.security.sharedsecret-unstable-ndk_platform",
	"android.hardware.vibrator-V1-ndk_platform",
	"android.hardware.vibrator-ndk_platform",
	"android.hardware.weaver-V1-ndk_platform",
	"android.hardware.weaver-ndk_platform",
	"android.hardware.weaver-unstable-ndk_platform",
	"android.system.keystore2-V1-ndk_platform",
	"android.system.keystore2-ndk_platform",
	"android.system.keystore2-unstable-ndk_platform",
	"libbinder",
	"libcrypto",
	"libexpat",
	"libgatekeeper",
	"libgui",
	"libhidlcache",
	"libkeymaster_messages",
	"libkeymaster_portable",
	"libmedia_omx",
	"libpuresoftkeymasterdevice",
	"libselinux",
	"libsoftkeymasterdevice",
	"libsqlite",
	"libssl",
	"libstagefright_bufferpool@2.0",
	"libstagefright_bufferqueue_helper",
	"libstagefright_foundation",
	"libstagefright_omx",
	"libstagefright_omx_utils",
	"libstagefright_xmlparser",
	"libui",
	"libxml2",
}
