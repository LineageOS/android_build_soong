// Copyright 2024 Google Inc. All rights reserved.
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

package build

import (
	"os"
	"slices"
	"strings"
)

var androidmk_denylist []string = []string{
	"art/",
	"bionic/",
	"bootable/",
	"build/",
	"cts/",
	"dalvik/",
	"developers/",
	"development/",
	"device/common/",
	"device/generic/",
	"device/google/",
	"device/google_car/",
	"device/sample/",
	"external/",
	"frameworks/",
	"hardware/google/",
	"hardware/interfaces/",
	"hardware/libhardware/",
	"hardware/libhardware_legacy/",
	"hardware/ril/",
	// Do not block other directories in kernel/, see b/319658303.
	"kernel/configs/",
	"kernel/prebuilts/",
	"kernel/tests/",
	"libcore/",
	"libnativehelper/",
	"packages/",
	"pdk/",
	"platform_testing/",
	"prebuilts/",
	"sdk/",
	"system/",
	"test/",
	"tools/",
	"trusty/",
	"toolchain/",
}

var androidmk_allowlist []string = []string{
	"bootable/deprecated-ota/updater/Android.mk",
	"external/mesa/android/Android.mk",
}

func getAllLines(ctx Context, filename string) []string {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}
		} else {
			ctx.Fatalf("Could not read %s: %v", filename, err)
		}
	}
	return strings.Split(strings.Trim(string(bytes), " \n"), "\n")
}

func blockAndroidMks(ctx Context, androidMks []string) {
	allowlist_files := []string{
		"vendor/google/build/androidmk/allowlist.txt",
		"device/google/harriet/androidmk/allowlist.txt",
	}
	for _, allowlist_file := range allowlist_files {
		allowlist := getAllLines(ctx, allowlist_file)
		androidmk_allowlist = append(androidmk_allowlist, allowlist...)
	}
	slices.Sort(androidmk_allowlist)
	androidmk_allowlist = slices.Compact(androidmk_allowlist)

	denylist := getAllLines(ctx, "vendor/google/build/androidmk/denylist.txt")
	androidmk_denylist = append(androidmk_denylist, denylist...)

	for _, mkFile := range androidMks {
		for _, d := range androidmk_denylist {
			if strings.HasPrefix(mkFile, d) && !slices.Contains(androidmk_allowlist, mkFile) {
				ctx.Fatalf("Found blocked Android.mk file: %s. "+
					"Please see androidmk_denylist.go for the blocked directories and contact build system team if the file should not be blocked.", mkFile)
			}
		}
	}
}

var ignore_androidmks []string = []string{
	// The Android.mk files in these directories are for NDK build system.
	"external/fmtlib/",
	"external/google-breakpad/",
	"external/googletest/",
	"external/libaom/",
	"external/libusb/",
	"external/libvpx/",
	"external/libwebm/",
	"external/libwebsockets/",
	"external/vulkan-validation-layers/",
	"external/walt/",
	"external/webp/",
	// These directories hold the published Android SDK, used in Unbundled Gradle builds.
	"prebuilts/fullsdk-darwin",
	"prebuilts/fullsdk-linux",
	// wpa_supplicant_8 has been converted to Android.bp and Android.mk files are kept for troubleshooting.
	"external/wpa_supplicant_8/",
	// Empty Android.mk in package's top directory
	"external/proguard/",
	"external/swig/",
	"toolchain/",
	"vendor/google/graphics/",
}

func ignoreSomeAndroidMks(androidMks []string) (filtered []string) {
	shouldKeep := func(androidmk string) bool {
		for _, prefix := range ignore_androidmks {
			if strings.HasPrefix(androidmk, prefix) {
				return false
			}
		}
		return true
	}

	for _, l := range androidMks {
		if shouldKeep(l) {
			filtered = append(filtered, l)
		}
	}
	return
}
