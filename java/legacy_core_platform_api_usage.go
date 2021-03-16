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
	"backuplib",
	"BandwidthEnforcementTest",
	"BlockedNumberProvider",
	"BluetoothInstrumentationTests",
	"BluetoothMidiService",
	"CarDeveloperOptions",
	"CarService",
	"CarServiceTest",
	"car-apps-common",
	"car-service-test-lib",
	"car-service-test-static-lib",
	"CertInstaller",
	"ConnectivityManagerTest",
	"ContactsProvider",
	"CorePerfTests",
	"core-tests-support",
	"CtsAppExitTestCases",
	"CtsContentTestCases",
	"CtsIkeTestCases",
	"CtsAppExitTestCases",
	"CtsLibcoreWycheproofBCTestCases",
	"CtsMediaTestCases",
	"CtsNetTestCases",
	"CtsNetTestCasesLatestSdk",
	"CtsSecurityTestCases",
	"CtsSuspendAppsTestCases",
	"CtsUsageStatsTestCases",
	"DeadpoolService",
	"DeadpoolServiceBtServices",
	"DeviceInfo",
	"DiagnosticTools",
	"DisplayCutoutEmulationEmu01Overlay",
	"DocumentsUIPerfTests",
	"DocumentsUITests",
	"DownloadProvider",
	"DownloadProviderTests",
	"DownloadProviderUi",
	"ds-car-docs", // for AAOS API documentation only
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
	"FrameworksMockingServicesTests",
	"FrameworksUtilTests",
	"FrameworksWifiTests",
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
	"service-blobstore",
	"service-connectivity-pre-jarjar",
	"service-jobscheduler",
	"services",
	"services.accessibility",
	"services.backup",
	"services.core.unboosted",
	"services.devicepolicy",
	"services.print",
	"services.usage",
	"services.usb",
	"Settings-core",
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

var legacyCorePlatformApiLookup = make(map[string]struct{})

func init() {
	for _, module := range legacyCorePlatformApiModules {
		legacyCorePlatformApiLookup[module] = struct{}{}
	}
}

func useLegacyCorePlatformApi(ctx android.EarlyModuleContext) bool {
	return useLegacyCorePlatformApiByName(ctx.ModuleName())
}

func useLegacyCorePlatformApiByName(name string) bool {
	_, found := legacyCorePlatformApiLookup[name]
	return found
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
