// Copyright 2015 Google Inc. All rights reserved.
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
	"fmt"

	"android/soong/android"
)

func (library *Library) hostDexNeeded() bool {
	return Bool(library.deviceProperties.Hostdex) && !library.Host()
}

func (library *Library) addHostDexAndroidMkInfo(info *android.AndroidMkProviderInfo) {
	if library.hostDexNeeded() {
		var output android.Path
		if library.dexJarFile.IsSet() {
			output = library.dexJarFile.Path()
		} else {
			output = library.implementationAndResourcesJar
		}
		hostDexInfo := android.AndroidMkInfo{
			Class:      "JAVA_LIBRARIES",
			SubName:    "-hostdex",
			OutputFile: android.OptionalPathForPath(output),
			Required:   library.deviceProperties.Target.Hostdex.Required,
			Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		}
		hostDexInfo.SetBool("LOCAL_IS_HOST_MODULE", true)
		if library.dexJarFile.IsSet() {
			hostDexInfo.SetPath("LOCAL_SOONG_DEX_JAR", library.dexJarFile.Path())
		}
		hostDexInfo.SetPath("LOCAL_SOONG_INSTALLED_MODULE", library.hostdexInstallFile)
		hostDexInfo.SetPath("LOCAL_SOONG_HEADER_JAR", library.headerJarFile)
		hostDexInfo.SetPath("LOCAL_SOONG_CLASSES_JAR", library.implementationAndResourcesJar)
		hostDexInfo.SetString("LOCAL_MODULE_STEM", library.Stem()+"-hostdex")

		if info.PrimaryInfo.OutputFile.Valid() {
			info.ExtraInfo = append(info.ExtraInfo, hostDexInfo)
		} else {
			info.PrimaryInfo = hostDexInfo
		}
	}
}

func (library *Library) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := library.prepareAndroidMKProviderInfo(config)
	library.addHostDexAndroidMkInfo(info)
	return info
}

func (library *Library) prepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}

	if !library.ApexModuleBase.AvailableFor(android.AvailableToPlatform) {
		// Platform variant.  If not available for the platform, we don't need Make module,
		// but the hostdex variant below is still needed for this module
	} else {
		info.PrimaryInfo = android.AndroidMkInfo{
			Class:      "JAVA_LIBRARIES",
			OutputFile: android.OptionalPathForPath(library.outputFile),
			Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		}

		if len(library.logtagsSrcs) > 0 {
			info.PrimaryInfo.AddStrings("LOCAL_SOONG_LOGTAGS_FILES", library.logtagsSrcs.Strings()...)
		}

		if library.installFile == nil {
			info.PrimaryInfo.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", true)
		}
		if library.dexJarFile.IsSet() {
			info.PrimaryInfo.SetPath("LOCAL_SOONG_DEX_JAR", library.dexJarFile.Path())
		}
		if len(library.dexpreopter.builtInstalled) > 0 {
			info.PrimaryInfo.SetString("LOCAL_SOONG_BUILT_INSTALLED", library.dexpreopter.builtInstalled)
		}
		info.PrimaryInfo.SetString("LOCAL_SDK_VERSION", library.sdkVersion.String())
		info.PrimaryInfo.SetPath("LOCAL_SOONG_CLASSES_JAR", library.implementationAndResourcesJar)
		info.PrimaryInfo.SetPath("LOCAL_SOONG_HEADER_JAR", library.headerJarFile)

		if library.jacocoInfo.ReportClassesFile != nil {
			info.PrimaryInfo.SetPath("LOCAL_SOONG_JACOCO_REPORT_CLASSES_JAR", library.jacocoInfo.ReportClassesFile)
		}

		requiredUsesLibs, optionalUsesLibs := library.classLoaderContexts.UsesLibs()
		info.PrimaryInfo.AddStrings("LOCAL_EXPORT_SDK_LIBRARIES", append(requiredUsesLibs, optionalUsesLibs...)...)

		// TODO(b470068827): Ensure proguard artifact propagation for APEX-only libraries and apps.
		info.PrimaryInfo.SetOptionalPath("LOCAL_SOONG_PROGUARD_DICT", library.dexer.proguardDictionary)
		info.PrimaryInfo.SetOptionalPath("LOCAL_SOONG_PROGUARD_USAGE_ZIP", library.dexer.proguardUsageZip)
		info.PrimaryInfo.SetString("LOCAL_MODULE_STEM", library.Stem())

		if library.dexpreopter.configPath != nil {
			info.PrimaryInfo.SetPath("LOCAL_SOONG_DEXPREOPT_CONFIG", library.dexpreopter.configPath)
		}
		if library.apiXmlFile != nil {
			info.PrimaryInfo.FooterStrings = append(info.PrimaryInfo.FooterStrings,
				fmt.Sprintf("$(call declare-1p-target,%s,)\n", library.apiXmlFile.String()),
				fmt.Sprintf("$(eval $(call copy-one-file,%s,$(TARGET_OUT_COMMON_INTERMEDIATES)/%s))\n", library.apiXmlFile.String(), library.apiXmlFile.Base()),
			)
		}
	}

	return info
}

func (j *JavaFuzzTest) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := j.Library.prepareAndroidMKProviderInfo(config)

	if info.PrimaryInfo.OutputFile.Valid() {
		info.PrimaryInfo.AddStrings("LOCAL_COMPATIBILITY_SUITE", "null-suite")
		androidMkWriteTestData(android.Paths{j.implementationJarFile}, &info.PrimaryInfo)
		androidMkWriteTestData(j.jniFilePaths, &info.PrimaryInfo)
		if j.fuzzPackagedModule.Corpus != nil {
			androidMkWriteTestData(j.fuzzPackagedModule.Corpus, &info.PrimaryInfo)
		}
		if j.fuzzPackagedModule.Dictionary != nil {
			androidMkWriteTestData(android.Paths{j.fuzzPackagedModule.Dictionary}, &info.PrimaryInfo)
		}
	}

	j.addHostDexAndroidMkInfo(info)

	return info
}

// Called for modules that are a component of a test suite.
func testSuiteComponent(entries *android.AndroidMkInfo, test_suites []string, perTestcaseDirectory bool) {
	entries.SetString("LOCAL_MODULE_TAGS", "tests")
	if len(test_suites) > 0 {
		entries.AddCompatibilityTestSuites(test_suites...)
	} else {
		entries.AddCompatibilityTestSuites("null-suite")
	}
	entries.SetBoolIfTrue("LOCAL_COMPATIBILITY_PER_TESTCASE_DIRECTORY", perTestcaseDirectory)
}

func (j *Test) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := j.Library.prepareAndroidMKProviderInfo(config)

	if info.PrimaryInfo.OutputFile.Valid() {
		testSuiteComponent(&info.PrimaryInfo, j.testProperties.Test_suites, Bool(j.testProperties.Per_testcase_directory))
		if j.testConfig != nil {
			info.PrimaryInfo.SetPath("LOCAL_FULL_TEST_CONFIG", j.testConfig)
		}
		androidMkWriteExtraTestConfigs(j.extraTestConfigs, &info.PrimaryInfo)
		androidMkWriteTestData(j.data, &info.PrimaryInfo)
		if !BoolDefault(j.testProperties.Auto_gen_config, true) {
			info.PrimaryInfo.SetString("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", "true")
		}
		info.PrimaryInfo.AddStrings("LOCAL_TEST_MAINLINE_MODULES", j.testProperties.Test_mainline_modules...)

		j.testProperties.Test_options.CommonTestOptions.SetAndroidMkInfoEntries(&info.PrimaryInfo)
	}

	j.addHostDexAndroidMkInfo(info)

	return info
}

func androidMkWriteExtraTestConfigs(extraTestConfigs android.Paths, entries *android.AndroidMkInfo) {
	if len(extraTestConfigs) > 0 {
		entries.AddStrings("LOCAL_EXTRA_FULL_TEST_CONFIGS", extraTestConfigs.Strings()...)
	}
}

func (j *TestHelperLibrary) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := j.Library.prepareAndroidMKProviderInfo(config)
	if info.PrimaryInfo.OutputFile.Valid() {
		testSuiteComponent(&info.PrimaryInfo, j.testHelperLibraryProperties.Test_suites, Bool(j.testHelperLibraryProperties.Per_testcase_directory))
	}
	j.addHostDexAndroidMkInfo(info)
	return info
}

func (prebuilt *Import) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:        "JAVA_LIBRARIES",
		OverrideName: prebuilt.BaseModuleName(),
		OutputFile:   android.OptionalPathForPath(prebuilt.combinedImplementationFile),
		Include:      "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
	}
	info.PrimaryInfo.SetBool("LOCAL_UNINSTALLABLE_MODULE", !Bool(prebuilt.properties.Installable))
	if prebuilt.dexJarFile.IsSet() {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_DEX_JAR", prebuilt.dexJarFile.Path())
	}
	info.PrimaryInfo.SetPath("LOCAL_SOONG_HEADER_JAR", prebuilt.combinedHeaderFile)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_CLASSES_JAR", prebuilt.combinedImplementationFile)
	info.PrimaryInfo.SetString("LOCAL_SDK_VERSION", prebuilt.sdkVersion.String())
	info.PrimaryInfo.SetString("LOCAL_MODULE_STEM", prebuilt.Stem())
	// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts

	return info
}

func (prebuilt *DexImport) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.dexJarFile.Path()),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
	}
	if prebuilt.dexJarFile.IsSet() {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_DEX_JAR", prebuilt.dexJarFile.Path())
	}
	if len(prebuilt.dexpreopter.builtInstalled) > 0 {
		info.PrimaryInfo.SetString("LOCAL_SOONG_BUILT_INSTALLED", prebuilt.dexpreopter.builtInstalled)
	}
	info.PrimaryInfo.SetString("LOCAL_MODULE_STEM", prebuilt.Stem())
	// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts

	return info
}

func (prebuilt *AARImport) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.implementationJarFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
	}
	info.PrimaryInfo.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_HEADER_JAR", prebuilt.headerJarFile)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_CLASSES_JAR", prebuilt.implementationJarFile)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", prebuilt.exportPackage)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_TRANSITIVE_RES_PACKAGES", prebuilt.transitiveAaptResourcePackagesFile)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_EXPORT_PROGUARD_FLAGS", prebuilt.proguardFlags)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_STATIC_LIBRARY_EXTRA_PACKAGES", prebuilt.extraAaptPackagesFile)
	info.PrimaryInfo.SetPath("LOCAL_FULL_MANIFEST_FILE", prebuilt.manifest)
	info.PrimaryInfo.SetString("LOCAL_SDK_VERSION", prebuilt.sdkVersion.String())
	// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts

	return info
}

func (binary *Binary) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	if binary.Os() == android.Windows {
		// Make does not support Windows Java modules
		return nil
	}

	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(binary.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
	}
	info.PrimaryInfo.SetPath("LOCAL_SOONG_HEADER_JAR", binary.headerJarFile)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_CLASSES_JAR", binary.implementationAndResourcesJar)
	if binary.dexJarFile.IsSet() {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_DEX_JAR", binary.dexJarFile.Path())
	}
	if len(binary.dexpreopter.builtInstalled) > 0 {
		info.PrimaryInfo.SetString("LOCAL_SOONG_BUILT_INSTALLED", binary.dexpreopter.builtInstalled)
	}
	info.PrimaryInfo.AddStrings("LOCAL_REQUIRED_MODULES", binary.androidMkNamesOfJniLibs...)

	return info
}

func (app *AndroidApp) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "APPS",
		OutputFile: android.OptionalPathForPath(app.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		Required:   app.requiredModuleNames,
	}
	// App module names can be overridden.
	info.PrimaryInfo.OverrideName = app.installApkName
	if app.headerJarFile != nil {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_HEADER_JAR", app.headerJarFile)
	}
	info.PrimaryInfo.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", app.exportPackage)
	if app.dexJarFile.IsSet() {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_DEX_JAR", app.dexJarFile.Path())
	}
	if app.implementationAndResourcesJar != nil {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_CLASSES_JAR", app.implementationAndResourcesJar)
	}
	if app.headerJarFile != nil {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_HEADER_JAR", app.headerJarFile)
	}
	if app.bundleFile != nil {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_BUNDLE", app.bundleFile)
	}
	if app.jacocoInfo.ReportClassesFile != nil {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_JACOCO_REPORT_CLASSES_JAR", app.jacocoInfo.ReportClassesFile)
	}
	info.PrimaryInfo.SetOptionalPath("LOCAL_SOONG_PROGUARD_DICT", app.dexer.proguardDictionary)
	info.PrimaryInfo.SetOptionalPath("LOCAL_SOONG_PROGUARD_USAGE_ZIP", app.dexer.proguardUsageZip)

	if app.Name() == "framework-res" || app.Name() == "org.lineageos.platform-res" {
		info.PrimaryInfo.SetString("LOCAL_MODULE_PATH", "$(TARGET_OUT_JAVA_LIBRARIES)")
		// Make base_rules.mk not put framework-res in a subdirectory called
		// framework_res.
		info.PrimaryInfo.SetBoolIfTrue("LOCAL_NO_STANDARD_LIBRARIES", true)
	}

	info.PrimaryInfo.SetBoolIfTrue("LOCAL_EXPORT_PACKAGE_RESOURCES", Bool(app.appProperties.Export_package_resources))

	info.PrimaryInfo.SetPath("LOCAL_FULL_MANIFEST_FILE", app.manifestPath)

	info.PrimaryInfo.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", app.Privileged())

	info.PrimaryInfo.SetString("LOCAL_CERTIFICATE", app.certificate.AndroidMkString())
	info.PrimaryInfo.AddStrings("LOCAL_OVERRIDES_PACKAGES", app.getOverriddenPackages()...)

	if app.embeddedJniLibs {
		jniSymbols := JNISymbolsInstalls(app.jniLibs, app.installPathForJNISymbols.String())
		info.PrimaryInfo.SetString("LOCAL_SOONG_JNI_LIBS_SYMBOLS", jniSymbols.String())
	} else {
		var names []string
		for _, jniLib := range app.jniLibs {
			names = append(names, jniLib.name+":"+jniLib.target.Arch.ArchType.Bitness())
		}
		info.PrimaryInfo.AddStrings("LOCAL_REQUIRED_MODULES", names...)
	}

	if len(app.jniCoverageOutputs) > 0 {
		info.PrimaryInfo.AddStrings("LOCAL_PREBUILT_COVERAGE_ARCHIVE", app.jniCoverageOutputs.Strings()...)
	}
	if len(app.dexpreopter.builtInstalled) > 0 {
		info.PrimaryInfo.SetString("LOCAL_SOONG_BUILT_INSTALLED", app.dexpreopter.builtInstalled)
	}
	if app.dexpreopter.configPath != nil {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_DEXPREOPT_CONFIG", app.dexpreopter.configPath)
	}
	for _, extra := range app.extraOutputFiles {
		install := app.onDeviceDir + "/" + extra.Base()
		info.PrimaryInfo.AddStrings("LOCAL_SOONG_BUILT_INSTALLED", extra.String()+":"+install)
	}

	info.PrimaryInfo.AddStrings("LOCAL_SOONG_LOGTAGS_FILES", app.logtagsSrcs.Strings()...)

	return info
}

func (a *AutogenRuntimeResourceOverlay) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	if a.outputFile == nil {
		return nil
	}
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "APPS",
		OutputFile: android.OptionalPathForPath(a.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
	}
	info.PrimaryInfo.SetString("LOCAL_CERTIFICATE", a.certificate.AndroidMkString())

	return info
}

func (a *AndroidApp) getOverriddenPackages() []string {
	var overridden []string
	if len(a.overridableAppProperties.Overrides) > 0 {
		overridden = append(overridden, a.overridableAppProperties.Overrides...)
	}
	return overridden
}

func (a *AndroidTest) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := a.AndroidApp.PrepareAndroidMKProviderInfo(config)
	testSuiteComponent(&info.PrimaryInfo, a.testProperties.Test_suites, Bool(a.testProperties.Per_testcase_directory))
	if a.testConfig != nil {
		info.PrimaryInfo.SetPath("LOCAL_FULL_TEST_CONFIG", a.testConfig)
	}
	androidMkWriteExtraTestConfigs(a.extraTestConfigs, &info.PrimaryInfo)
	androidMkWriteTestData(a.data, &info.PrimaryInfo)
	info.PrimaryInfo.AddStrings("LOCAL_TEST_MAINLINE_MODULES", a.testProperties.Test_mainline_modules...)

	return info
}

func (a *AndroidTestHelperApp) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := a.AndroidApp.PrepareAndroidMKProviderInfo(config)
	testSuiteComponent(&info.PrimaryInfo, a.appTestHelperAppProperties.Test_suites, Bool(a.appTestHelperAppProperties.Per_testcase_directory))
	// introduce a flag variable to control the generation of the .config file
	info.PrimaryInfo.SetString("LOCAL_DISABLE_TEST_CONFIG", "true")

	return info
}

func (a *AndroidLibrary) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := a.Library.prepareAndroidMKProviderInfo(config)

	if info.PrimaryInfo.OutputFile.Valid() {
		if a.aarFile != nil {
			info.PrimaryInfo.SetPath("LOCAL_SOONG_AAR", a.aarFile)
		}

		if a.Name() == "framework-res" || a.Name() == "org.lineageos.platform-res" {
			info.PrimaryInfo.SetString("LOCAL_MODULE_PATH", "$(TARGET_OUT_JAVA_LIBRARIES)")
			// Make base_rules.mk not put framework-res in a subdirectory called
			// framework_res.
			info.PrimaryInfo.SetBoolIfTrue("LOCAL_NO_STANDARD_LIBRARIES", true)
		}

		info.PrimaryInfo.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", a.exportPackage)
		info.PrimaryInfo.SetPath("LOCAL_SOONG_TRANSITIVE_RES_PACKAGES", a.transitiveAaptResourcePackagesFile)
		info.PrimaryInfo.SetPath("LOCAL_SOONG_STATIC_LIBRARY_EXTRA_PACKAGES", a.extraAaptPackagesFile)
		info.PrimaryInfo.SetPath("LOCAL_FULL_MANIFEST_FILE", a.mergedManifestFile)
		info.PrimaryInfo.SetPath("LOCAL_SOONG_EXPORT_PROGUARD_FLAGS", a.combinedExportedProguardFlagsFile)
		info.PrimaryInfo.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", true)
	}

	a.addHostDexAndroidMkInfo(info)

	return info
}

func (jd *Javadoc) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(jd.stubsSrcJar),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
	}
	if BoolDefault(jd.properties.Installable, true) {
		info.PrimaryInfo.SetPath("LOCAL_DROIDDOC_DOC_ZIP", jd.docZip)
	}
	if jd.exportableStubsSrcJar != nil {
		info.PrimaryInfo.SetPath("LOCAL_DROIDDOC_STUBS_SRCJAR", jd.exportableStubsSrcJar)
	}

	return info
}

func (ddoc *Droiddoc) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(ddoc.Javadoc.docZip),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
	}
	if ddoc.Javadoc.docZip != nil {
		info.PrimaryInfo.SetPath("LOCAL_DROIDDOC_DOC_ZIP", ddoc.Javadoc.docZip)
	}
	info.PrimaryInfo.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !BoolDefault(ddoc.Javadoc.properties.Installable, true))

	return info
}

func (dstubs *Droidstubs) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	// If the stubsSrcJar is not generated (because generate_stubs is false) then
	// use the api file as the output file to ensure the relevant phony targets
	// are created in make if only the api txt file is being generated. This is
	// needed because an invalid output file would prevent the make entries from
	// being written.
	//
	// Note that dstubs.apiFile can be also be nil if WITHOUT_CHECKS_API is true.
	// TODO(b/146727827): Revert when we do not need to generate stubs and API separately.

	outputFile := android.OptionalPathForPath(dstubs.stubsSrcJar)
	if !outputFile.Valid() {
		outputFile = android.OptionalPathForPath(dstubs.apiFile)
	}
	if !outputFile.Valid() {
		outputFile = android.OptionalPathForPath(dstubs.everythingArtifacts.apiVersionsXml)
	}
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "JAVA_LIBRARIES",
		OutputFile: outputFile,
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
	}
	if dstubs.Javadoc.exportableStubsSrcJar != nil {
		info.PrimaryInfo.SetPath("LOCAL_DROIDDOC_STUBS_SRCJAR", dstubs.Javadoc.exportableStubsSrcJar)
	}
	if dstubs.everythingArtifacts.apiVersionsXml != nil {
		info.PrimaryInfo.SetPath("LOCAL_DROIDDOC_API_VERSIONS_XML", dstubs.exportableArtifacts.apiVersionsXml)
	}
	if dstubs.everythingArtifacts.annotationsZip != nil {
		info.PrimaryInfo.SetPath("LOCAL_DROIDDOC_ANNOTATIONS_ZIP", dstubs.exportableArtifacts.annotationsZip)
	}
	if dstubs.everythingArtifacts.metadataZip != nil {
		info.PrimaryInfo.SetPath("LOCAL_DROIDDOC_METADATA_ZIP", dstubs.exportableArtifacts.metadataZip)
	}
	if dstubs.apiLintTimestamp != nil {
		if dstubs.apiLintReport != nil {
			info.PrimaryInfo.FooterStrings = append(info.PrimaryInfo.FooterStrings,
				fmt.Sprintf("$(call declare-0p-target,%s)\n", dstubs.apiLintReport.String()))
		}
	}

	return info
}

func (a *AndroidAppImport) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:        "APPS",
		OutputFile:   android.OptionalPathForPath(a.outputFile),
		OverrideName: a.BaseModuleName(), // TODO (spandandas): Add a test
		Include:      "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
	}
	info.PrimaryInfo.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", a.Privileged())
	info.PrimaryInfo.SetString("LOCAL_CERTIFICATE", a.certificate.AndroidMkString())
	info.PrimaryInfo.AddStrings("LOCAL_OVERRIDES_PACKAGES", a.properties.Overrides...)
	if len(a.dexpreopter.builtInstalled) > 0 {
		info.PrimaryInfo.SetString("LOCAL_SOONG_BUILT_INSTALLED", a.dexpreopter.builtInstalled)
	}
	info.PrimaryInfo.AddStrings("LOCAL_INSTALLED_MODULE_STEM", a.installPath.Rel())
	if Bool(a.properties.Export_package_resources) {
		info.PrimaryInfo.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", a.outputFile)
	}
	// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts

	return info
}

func (a *AndroidTestImport) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := a.AndroidAppImport.PrepareAndroidMKProviderInfo(config)
	testSuiteComponent(&info.PrimaryInfo, a.testProperties.Test_suites, Bool(a.testProperties.Per_testcase_directory))
	androidMkWriteTestData(a.data, &info.PrimaryInfo)

	return info
}

func androidMkWriteTestData(data android.Paths, entries *android.AndroidMkInfo) {
	var testFiles []string
	for _, d := range data {
		testFiles = append(testFiles, d.String()+":"+d.Rel())
	}
	entries.AddStrings("LOCAL_COMPATIBILITY_SUPPORT_FILES", testFiles...)
}

func (r *RuntimeResourceOverlay) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(r.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
	}
	info.PrimaryInfo.SetString("LOCAL_CERTIFICATE", r.certificate.AndroidMkString())
	info.PrimaryInfo.SetPath("LOCAL_MODULE_PATH", r.installDir)
	info.PrimaryInfo.AddStrings("LOCAL_OVERRIDES_PACKAGES", r.properties.Overrides...)
	// TODO: LOCAL_ACONFIG_FILES -- Might eventually need aconfig flags?

	return info
}

func (apkSet *AndroidAppSet) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "APPS",
		OutputFile: android.OptionalPathForPath(apkSet.primaryOutput),
		Include:    "$(BUILD_SYSTEM)/soong_android_app_set.mk",
	}
	info.PrimaryInfo.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", apkSet.Privileged())
	info.PrimaryInfo.SetPath("LOCAL_APK_SET_INSTALL_FILE", apkSet.PackedAdditionalOutputs())
	info.PrimaryInfo.SetPath("LOCAL_APKCERTS_FILE", apkSet.apkcertsFile)
	info.PrimaryInfo.AddStrings("LOCAL_OVERRIDES_PACKAGES", apkSet.properties.Overrides...)
	// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts -- Both declarations and values

	return info
}

func (al *ApiLibrary) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	info := &android.AndroidMkProviderInfo{}
	info.PrimaryInfo = android.AndroidMkInfo{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(al.stubsJar),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
	}
	info.PrimaryInfo.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", true)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_CLASSES_JAR", al.stubsJar)
	info.PrimaryInfo.SetPath("LOCAL_SOONG_HEADER_JAR", al.stubsJar)

	return info
}
