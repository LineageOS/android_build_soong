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

// This variable is effectively unused in pre-master branches, and is
// included (with the same value as it has in AOSP) only to ease
// merges between branches (see the comment in the
// useLegacyCorePlatformApi() function):
var legacyCorePlatformApiModules = []string{
	"ArcSettings",
	"ahat-test-dump",
	"android.car",
	"android.test.mock",
	"android.test.mock.impl",
	"AoapTestDeviceApp",
	"AoapTestHostApp",
	"api-stubs-docs",
	"art_cts_jvmti_test_library",
	"art-gtest-jars-MyClassNatives",
	"BackupFrameworksServicesRoboTests",
	"BandwidthEnforcementTest",
	"BlockedNumberProvider",
	"BluetoothInstrumentationTests",
	"BluetoothMidiService",
	"car-apps-common",
	"CertInstaller",
	"ConnectivityManagerTest",
	"ContactsProvider",
	"core-tests-support",
	"CtsContentTestCases",
	"CtsIkeTestCases",
	"CtsLibcoreWycheproofBCTestCases",
	"CtsMediaTestCases",
	"CtsNetTestCases",
	"CtsNetTestCasesLatestSdk",
	"CtsSecurityTestCases",
	"CtsUsageStatsTestCases",
	"DisplayCutoutEmulationEmu01Overlay",
	"DocumentsUIPerfTests",
	"DocumentsUITests",
	"DownloadProvider",
	"DownloadProviderTests",
	"DownloadProviderUi",
	"DynamicSystemInstallationService",
	"EmergencyInfo-lib",
	"ethernet-service",
	"EthernetServiceTests",
	"ExternalStorageProvider",
	"ExtServices",
	"ExtServices-core",
	"framework-all",
	"framework-minus-apex",
	"FrameworksCoreTests",
	"FrameworksIkeTests",
	"FrameworksNetCommonTests",
	"FrameworksNetTests",
	"FrameworksServicesRoboTests",
	"FrameworksServicesTests",
	"FrameworksUtilTests",
	"hid",
	"hidl_test_java_java",
	"hwbinder",
	"ims",
	"KeyChain",
	"ksoap2",
	"LocalTransport",
	"lockagent",
	"mediaframeworktest",
	"MediaProvider",
	"MmsService",
	"MtpDocumentsProvider",
	"MultiDisplayProvider",
	"NetworkStackIntegrationTestsLib",
	"NetworkStackNextIntegrationTests",
	"NetworkStackNextTests",
	"NetworkStackTests",
	"NetworkStackTestsLib",
	"NfcNci",
	"platform_library-docs",
	"PrintSpooler",
	"RollbackTest",
	"services",
	"services.accessibility",
	"services.backup",
	"services.core.unboosted",
	"services.devicepolicy",
	"services.print",
	"services.usage",
	"services.usb",
	"Settings-core",
	"SettingsGoogle",
	"SettingsLib",
	"SettingsProvider",
	"SettingsProviderTest",
	"SettingsRoboTests",
	"Shell",
	"ShellTests",
	"sl4a.Common",
	"StatementService",
	"SystemUI-core",
	"SystemUISharedLib",
	"SystemUI-tests",
	"Telecom",
	"TelecomUnitTests",
	"telephony-common",
	"TelephonyProvider",
	"TelephonyProviderTests",
	"TeleService",
	"testables",
	"TetheringTests",
	"TetheringTestsLib",
	"time_zone_distro_installer",
	"time_zone_distro_installer-tests",
	"time_zone_distro-tests",
	"time_zone_updater",
	"TvProvider",
	"uiautomator-stubs-docs",
	"UsbHostExternalManagementTestApp",
	"UserDictionaryProvider",
	"WallpaperBackup",
	"wifi-service",
}

// This variable is effectively unused in pre-master branches, and is
// included (with the same value as it has in AOSP) only to ease
// merges between branches (see the comment in the
// useLegacyCorePlatformApi() function):
var legacyCorePlatformApiLookup = make(map[string]struct{})

func init() {
	for _, module := range legacyCorePlatformApiModules {
		legacyCorePlatformApiLookup[module] = struct{}{}
	}
}

func useLegacyCorePlatformApi(ctx android.EarlyModuleContext) bool {
	// In pre-master branches, we don't attempt to force usage of the stable
	// version of the core/platform API. Instead, we always use the legacy
	// version --- except in tests, where we always use stable, so that we
	// can make the test assertions the same as other branches.
	// This should be false in tests and true otherwise:
	return ctx.Config().TestProductVariables == nil
}

func corePlatformSystemModules(ctx android.EarlyModuleContext) string {
	if useLegacyCorePlatformApi(ctx) {
		return config.LegacyCorePlatformSystemModules
	} else {
		return config.StableCorePlatformSystemModules
	}
}

func corePlatformBootclasspathLibraries(ctx android.EarlyModuleContext) []string {
	if useLegacyCorePlatformApi(ctx) {
		return config.LegacyCorePlatformBootclasspathLibraries
	} else {
		return config.StableCorePlatformBootclasspathLibraries
	}
}
