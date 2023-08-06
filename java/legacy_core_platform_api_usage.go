// Copyright 2020 Google Inc. All rights reserved.
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

package java

import (
	"android/soong/android"
	"android/soong/java/config"
)

var legacyCorePlatformApiModules = []string{
	"ArcSettings",
	"BTTestApp",
	"CapCtrlInterface",
	"com.qti.location.sdk",
	"face-V1-0-javalib",
	"FloralClocks",
	"framework-jobscheduler",
	"framework-minus-apex",
	"framework-minus-apex-headers",
	"framework-minus-apex-intdefs",
	"FrameworksCoreTests",
	"HelloOslo",
	"izat.lib.glue",
	"mediatek-ims-base",
	"ModemTestMode",
	"MtkCapCtrl",
	"my.tests.snapdragonsdktest",
	"NetworkSetting",
	"PerformanceMode",
	"pxp-monitor",
	"QColor",
	"qcom.fmradio",
	"Qmmi",
	"QPerformance",
	"sam",
	"saminterfacelibrary",
	"sammanagerlibrary",
	"services",
	"services.core.unboosted",
	"Settings-core",
	"SettingsGoogle",
	"SettingsGoogleOverlayCoral",
	"SettingsGoogleOverlayFlame",
	"SettingsLib",
	"SettingsRoboTests",
	"SimContact",
	"SimContacts",
	"SimSettings",
	"tcmiface",
	"telephony-common",
	"TeleService",
	"UxPerformance",
	"WfdCommon",
}

var legacyCorePlatformApiLookup = make(map[string]struct{})

func init() {
	for _, module := range legacyCorePlatformApiModules {
		legacyCorePlatformApiLookup[module] = struct{}{}
	}
}

var legacyCorePlatformApiLookupKey = android.NewOnceKey("legacyCorePlatformApiLookup")

func getLegacyCorePlatformApiLookup(config android.Config) map[string]struct{} {
	return config.Once(legacyCorePlatformApiLookupKey, func() interface{} {
		return legacyCorePlatformApiLookup
	}).(map[string]struct{})
}

// useLegacyCorePlatformApi checks to see whether the supplied module name is in the list of modules
// that are able to use the legacy core platform API and returns true if it does, false otherwise.
//
// This method takes the module name separately from the context as this may be being called for a
// module that is not the target of the supplied context.
func useLegacyCorePlatformApi(ctx android.EarlyModuleContext, moduleName string) bool {
	lookup := getLegacyCorePlatformApiLookup(ctx.Config())
	_, found := lookup[moduleName]
	return found
}

func corePlatformSystemModules(ctx android.EarlyModuleContext) string {
	if useLegacyCorePlatformApi(ctx, ctx.ModuleName()) {
		return config.LegacyCorePlatformSystemModules
	} else {
		return config.StableCorePlatformSystemModules
	}
}

func corePlatformBootclasspathLibraries(ctx android.EarlyModuleContext) []string {
	if useLegacyCorePlatformApi(ctx, ctx.ModuleName()) {
		return config.LegacyCorePlatformBootclasspathLibraries
	} else {
		return config.StableCorePlatformBootclasspathLibraries
	}
}
