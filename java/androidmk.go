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
	"io"
	"strings"

	"android/soong/android"
)

func (library *Library) AndroidMkHostDex(w io.Writer, name string, data android.AndroidMkData) {
	if Bool(library.deviceProperties.Hostdex) && !library.Host() {
		fmt.Fprintln(w, "include $(CLEAR_VARS)")
		fmt.Fprintln(w, "LOCAL_MODULE := "+name+"-hostdex")
		fmt.Fprintln(w, "LOCAL_IS_HOST_MODULE := true")
		fmt.Fprintln(w, "LOCAL_MODULE_CLASS := JAVA_LIBRARIES")
		if library.dexJarFile != nil {
			fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", library.dexJarFile.String())
		} else {
			fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", library.implementationAndResourcesJar.String())
		}
		if library.dexJarFile != nil {
			fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", library.dexJarFile.String())
		}
		fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", library.headerJarFile.String())
		fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", library.implementationAndResourcesJar.String())
		fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES := "+strings.Join(data.Required, " "))
		if r := library.deviceProperties.Target.Hostdex.Required; len(r) > 0 {
			fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES +=", strings.Join(r, " "))
		}
		fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_java_prebuilt.mk")
	}
}

func (library *Library) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(library.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				if len(library.logtagsSrcs) > 0 {
					var logtags []string
					for _, l := range library.logtagsSrcs {
						logtags = append(logtags, l.Rel())
					}
					fmt.Fprintln(w, "LOCAL_LOGTAGS_FILES :=", strings.Join(logtags, " "))
				}

				if library.installFile == nil {
					fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
				}
				if library.dexJarFile != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", library.dexJarFile.String())
				}
				if len(library.dexpreopter.builtInstalled) > 0 {
					fmt.Fprintln(w, "LOCAL_SOONG_BUILT_INSTALLED :=", library.dexpreopter.builtInstalled)
				}
				fmt.Fprintln(w, "LOCAL_SDK_VERSION :=", library.sdkVersion())
				fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", library.implementationAndResourcesJar.String())
				fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", library.headerJarFile.String())

				if library.jacocoReportClassesFile != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_JACOCO_REPORT_CLASSES_JAR :=", library.jacocoReportClassesFile.String())
				}

				if len(library.exportedSdkLibs) != 0 {
					fmt.Fprintln(w, "LOCAL_EXPORT_SDK_LIBRARIES :=", strings.Join(library.exportedSdkLibs, " "))
				}

				if len(library.additionalCheckedModules) != 0 {
					fmt.Fprintln(w, "LOCAL_ADDITIONAL_CHECKED_MODULE +=", strings.Join(library.additionalCheckedModules.Strings(), " "))
				}

				// Temporary hack: export sources used to compile framework.jar to Make
				// to be used for droiddoc
				// TODO(ccross): remove this once droiddoc is in soong
				if (library.Name() == "framework") || (library.Name() == "framework-annotation-proc") {
					fmt.Fprintln(w, "SOONG_FRAMEWORK_SRCS :=", strings.Join(library.compiledJavaSrcs.Strings(), " "))
					fmt.Fprintln(w, "SOONG_FRAMEWORK_SRCJARS :=", strings.Join(library.compiledSrcJars.Strings(), " "))
				}
			},
		},
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			android.WriteAndroidMkData(w, data)
			library.AndroidMkHostDex(w, name, data)
		},
	}
}

// Called for modules that are a component of a test suite.
func testSuiteComponent(w io.Writer, test_suites []string) {
	fmt.Fprintln(w, "LOCAL_MODULE_TAGS := tests")
	if len(test_suites) > 0 {
		fmt.Fprintln(w, "LOCAL_COMPATIBILITY_SUITE :=",
			strings.Join(test_suites, " "))
	} else {
		fmt.Fprintln(w, "LOCAL_COMPATIBILITY_SUITE := null-suite")
	}
}

func (j *Test) AndroidMk() android.AndroidMkData {
	data := j.Library.AndroidMk()
	data.Extra = append(data.Extra, func(w io.Writer, outputFile android.Path) {
		testSuiteComponent(w, j.testProperties.Test_suites)
		if j.testConfig != nil {
			fmt.Fprintln(w, "LOCAL_FULL_TEST_CONFIG :=", j.testConfig.String())
		}
	})

	androidMkWriteTestData(j.data, &data)

	return data
}

func (j *TestHelperLibrary) AndroidMk() android.AndroidMkData {
	data := j.Library.AndroidMk()
	data.Extra = append(data.Extra, func(w io.Writer, outputFile android.Path) {
		testSuiteComponent(w, j.testHelperLibraryProperties.Test_suites)
	})

	return data
}

func (prebuilt *Import) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.combinedClasspathFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := ", !Bool(prebuilt.properties.Installable))
				fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", prebuilt.combinedClasspathFile.String())
				fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", prebuilt.combinedClasspathFile.String())
				fmt.Fprintln(w, "LOCAL_SDK_VERSION :=", prebuilt.sdkVersion())
			},
		},
	}
}

func (prebuilt *DexImport) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.maybeStrippedDexJarFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				if prebuilt.dexJarFile != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", prebuilt.dexJarFile.String())
					// TODO(b/125517186): export the dex jar as a classes jar to match some mis-uses in Make until
					// boot_jars_package_check.mk can check dex jars.
					fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", prebuilt.dexJarFile.String())
					fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", prebuilt.dexJarFile.String())
				}
				if len(prebuilt.dexpreopter.builtInstalled) > 0 {
					fmt.Fprintln(w, "LOCAL_SOONG_BUILT_INSTALLED :=", prebuilt.dexpreopter.builtInstalled)
				}
			},
		},
	}
}

func (prebuilt *AARImport) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.classpathFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
				fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", prebuilt.classpathFile.String())
				fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", prebuilt.classpathFile.String())
				fmt.Fprintln(w, "LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE :=", prebuilt.exportPackage.String())
				fmt.Fprintln(w, "LOCAL_SOONG_EXPORT_PROGUARD_FLAGS :=", prebuilt.proguardFlags.String())
				fmt.Fprintln(w, "LOCAL_SOONG_STATIC_LIBRARY_EXTRA_PACKAGES :=", prebuilt.extraAaptPackagesFile.String())
				fmt.Fprintln(w, "LOCAL_FULL_MANIFEST_FILE :=", prebuilt.manifest.String())
				fmt.Fprintln(w, "LOCAL_SDK_VERSION :=", prebuilt.sdkVersion())
			},
		},
	}
}

func (binary *Binary) AndroidMk() android.AndroidMkData {

	if !binary.isWrapperVariant {
		return android.AndroidMkData{
			Class:      "JAVA_LIBRARIES",
			OutputFile: android.OptionalPathForPath(binary.outputFile),
			Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
			Extra: []android.AndroidMkExtraFunc{
				func(w io.Writer, outputFile android.Path) {
					fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", binary.headerJarFile.String())
					fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", binary.implementationAndResourcesJar.String())
					if binary.dexJarFile != nil {
						fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", binary.dexJarFile.String())
					}
					if len(binary.dexpreopter.builtInstalled) > 0 {
						fmt.Fprintln(w, "LOCAL_SOONG_BUILT_INSTALLED :=", binary.dexpreopter.builtInstalled)
					}
				},
			},
			Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
				android.WriteAndroidMkData(w, data)

				fmt.Fprintln(w, "jar_installed_module := $(LOCAL_INSTALLED_MODULE)")
			},
		}
	} else {
		return android.AndroidMkData{
			Class:      "EXECUTABLES",
			OutputFile: android.OptionalPathForPath(binary.wrapperFile),
			Extra: []android.AndroidMkExtraFunc{
				func(w io.Writer, outputFile android.Path) {
					fmt.Fprintln(w, "LOCAL_STRIP_MODULE := false")
				},
			},
			Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
				android.WriteAndroidMkData(w, data)

				// Ensure that the wrapper script timestamp is always updated when the jar is updated
				fmt.Fprintln(w, "$(LOCAL_INSTALLED_MODULE): $(jar_installed_module)")
				fmt.Fprintln(w, "jar_installed_module :=")
			},
		}
	}
}

func (app *AndroidApp) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "APPS",
		OutputFile: android.OptionalPathForPath(app.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				// TODO(jungjw): This, outputting two LOCAL_MODULE lines, works, but is not ideal. Find a better solution.
				if app.Name() != app.installApkName {
					fmt.Fprintln(w, "# Overridden by PRODUCT_PACKAGE_NAME_OVERRIDES")
					fmt.Fprintln(w, "LOCAL_MODULE :=", app.installApkName)
				}
				fmt.Fprintln(w, "LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE :=", app.exportPackage.String())
				if app.dexJarFile != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", app.dexJarFile.String())
				}
				if app.implementationAndResourcesJar != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", app.implementationAndResourcesJar.String())
				}
				if app.headerJarFile != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", app.headerJarFile.String())
				}
				if app.bundleFile != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_BUNDLE :=", app.bundleFile.String())
				}
				if app.jacocoReportClassesFile != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_JACOCO_REPORT_CLASSES_JAR :=", app.jacocoReportClassesFile.String())
				}
				if app.proguardDictionary != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_PROGUARD_DICT :=", app.proguardDictionary.String())
				}

				if app.Name() == "framework-res" {
					fmt.Fprintln(w, "LOCAL_MODULE_PATH := $(TARGET_OUT_JAVA_LIBRARIES)")
					// Make base_rules.mk not put framework-res in a subdirectory called
					// framework_res.
					fmt.Fprintln(w, "LOCAL_NO_STANDARD_LIBRARIES := true")
				}

				filterRRO := func(filter overlayType) android.Paths {
					var paths android.Paths
					for _, d := range app.rroDirs {
						if d.overlayType == filter {
							paths = append(paths, d.path)
						}
					}
					// Reverse the order, Soong stores rroDirs in aapt2 order (low to high priority), but Make
					// expects it in LOCAL_RESOURCE_DIRS order (high to low priority).
					return android.ReversePaths(paths)
				}
				deviceRRODirs := filterRRO(device)
				if len(deviceRRODirs) > 0 {
					fmt.Fprintln(w, "LOCAL_SOONG_DEVICE_RRO_DIRS :=", strings.Join(deviceRRODirs.Strings(), " "))
				}
				productRRODirs := filterRRO(product)
				if len(productRRODirs) > 0 {
					fmt.Fprintln(w, "LOCAL_SOONG_PRODUCT_RRO_DIRS :=", strings.Join(productRRODirs.Strings(), " "))
				}

				if Bool(app.appProperties.Export_package_resources) {
					fmt.Fprintln(w, "LOCAL_EXPORT_PACKAGE_RESOURCES := true")
				}

				fmt.Fprintln(w, "LOCAL_FULL_MANIFEST_FILE :=", app.manifestPath.String())

				if Bool(app.appProperties.Privileged) {
					fmt.Fprintln(w, "LOCAL_PRIVILEGED_MODULE := true")
				}

				fmt.Fprintln(w, "LOCAL_CERTIFICATE :=", app.certificate.Pem.String())
				if overriddenPkgs := app.getOverriddenPackages(); len(overriddenPkgs) > 0 {
					fmt.Fprintln(w, "LOCAL_OVERRIDES_PACKAGES :=", strings.Join(overriddenPkgs, " "))
				}

				for _, jniLib := range app.installJniLibs {
					fmt.Fprintln(w, "LOCAL_SOONG_JNI_LIBS_"+jniLib.target.Arch.ArchType.String(), "+=", jniLib.name)
				}
				if len(app.dexpreopter.builtInstalled) > 0 {
					fmt.Fprintln(w, "LOCAL_SOONG_BUILT_INSTALLED :=", app.dexpreopter.builtInstalled)
				}
				for _, split := range app.aapt.splits {
					install := "$(LOCAL_MODULE_PATH)/" + strings.TrimSuffix(app.installApkName, ".apk") + split.suffix + ".apk"
					fmt.Fprintln(w, "LOCAL_SOONG_BUILT_INSTALLED +=", split.path.String()+":"+install)
				}
			},
		},
	}
}

func (a *AndroidApp) getOverriddenPackages() []string {
	var overridden []string
	if len(a.appProperties.Overrides) > 0 {
		overridden = append(overridden, a.appProperties.Overrides...)
	}
	if a.Name() != a.installApkName {
		overridden = append(overridden, a.Name())
	}
	return overridden
}

func (a *AndroidTest) AndroidMk() android.AndroidMkData {
	data := a.AndroidApp.AndroidMk()
	data.Extra = append(data.Extra, func(w io.Writer, outputFile android.Path) {
		testSuiteComponent(w, a.testProperties.Test_suites)
		if a.testConfig != nil {
			fmt.Fprintln(w, "LOCAL_FULL_TEST_CONFIG :=", a.testConfig.String())
		}
	})
	androidMkWriteTestData(a.data, &data)

	return data
}

func (a *AndroidTestHelperApp) AndroidMk() android.AndroidMkData {
	data := a.AndroidApp.AndroidMk()
	data.Extra = append(data.Extra, func(w io.Writer, outputFile android.Path) {
		testSuiteComponent(w, a.appTestHelperAppProperties.Test_suites)
	})

	return data
}

func (a *AndroidLibrary) AndroidMk() android.AndroidMkData {
	data := a.Library.AndroidMk()

	data.Extra = append(data.Extra, func(w io.Writer, outputFile android.Path) {
		if a.aarFile != nil {
			fmt.Fprintln(w, "LOCAL_SOONG_AAR :=", a.aarFile.String())
		}
		if a.proguardDictionary != nil {
			fmt.Fprintln(w, "LOCAL_SOONG_PROGUARD_DICT :=", a.proguardDictionary.String())
		}

		if a.Name() == "framework-res" {
			fmt.Fprintln(w, "LOCAL_MODULE_PATH := $(TARGET_OUT_JAVA_LIBRARIES)")
			// Make base_rules.mk not put framework-res in a subdirectory called
			// framework_res.
			fmt.Fprintln(w, "LOCAL_NO_STANDARD_LIBRARIES := true")
		}

		fmt.Fprintln(w, "LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE :=", a.exportPackage.String())
		fmt.Fprintln(w, "LOCAL_SOONG_STATIC_LIBRARY_EXTRA_PACKAGES :=", a.extraAaptPackagesFile.String())
		fmt.Fprintln(w, "LOCAL_FULL_MANIFEST_FILE :=", a.mergedManifestFile.String())
		fmt.Fprintln(w, "LOCAL_SOONG_EXPORT_PROGUARD_FLAGS :=",
			strings.Join(a.exportedProguardFlagFiles.Strings(), " "))
		fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
	})

	return data
}

func (jd *Javadoc) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(jd.stubsSrcJar),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				if BoolDefault(jd.properties.Installable, true) {
					fmt.Fprintln(w, "LOCAL_DROIDDOC_DOC_ZIP := ", jd.docZip.String())
				}
				if jd.stubsSrcJar != nil {
					fmt.Fprintln(w, "LOCAL_DROIDDOC_STUBS_SRCJAR := ", jd.stubsSrcJar.String())
				}
			},
		},
	}
}

func (ddoc *Droiddoc) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(ddoc.stubsSrcJar),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				if BoolDefault(ddoc.Javadoc.properties.Installable, true) && ddoc.Javadoc.docZip != nil {
					fmt.Fprintln(w, "LOCAL_DROIDDOC_DOC_ZIP := ", ddoc.Javadoc.docZip.String())
				}
				if ddoc.Javadoc.stubsSrcJar != nil {
					fmt.Fprintln(w, "LOCAL_DROIDDOC_STUBS_SRCJAR := ", ddoc.Javadoc.stubsSrcJar.String())
				}
				if ddoc.checkCurrentApiTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", ddoc.Name()+"-check-current-api")
					fmt.Fprintln(w, ddoc.Name()+"-check-current-api:",
						ddoc.checkCurrentApiTimestamp.String())

					fmt.Fprintln(w, ".PHONY: checkapi")
					fmt.Fprintln(w, "checkapi:",
						ddoc.checkCurrentApiTimestamp.String())

					fmt.Fprintln(w, ".PHONY: droidcore")
					fmt.Fprintln(w, "droidcore: checkapi")
				}
				if ddoc.updateCurrentApiTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", ddoc.Name()+"-update-current-api")
					fmt.Fprintln(w, ddoc.Name()+"-update-current-api:",
						ddoc.updateCurrentApiTimestamp.String())

					fmt.Fprintln(w, ".PHONY: update-api")
					fmt.Fprintln(w, "update-api:",
						ddoc.updateCurrentApiTimestamp.String())
				}
				if ddoc.checkLastReleasedApiTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", ddoc.Name()+"-check-last-released-api")
					fmt.Fprintln(w, ddoc.Name()+"-check-last-released-api:",
						ddoc.checkLastReleasedApiTimestamp.String())

					if ddoc.Name() == "api-stubs-docs" || ddoc.Name() == "system-api-stubs-docs" {
						fmt.Fprintln(w, ".PHONY: checkapi")
						fmt.Fprintln(w, "checkapi:",
							ddoc.checkLastReleasedApiTimestamp.String())

						fmt.Fprintln(w, ".PHONY: droidcore")
						fmt.Fprintln(w, "droidcore: checkapi")
					}
				}
				apiFilePrefix := "INTERNAL_PLATFORM_"
				if String(ddoc.properties.Api_tag_name) != "" {
					apiFilePrefix += String(ddoc.properties.Api_tag_name) + "_"
				}
				if ddoc.apiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"API_FILE := ", ddoc.apiFile.String())
				}
				if ddoc.dexApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"DEX_API_FILE := ", ddoc.dexApiFile.String())
				}
				if ddoc.privateApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"PRIVATE_API_FILE := ", ddoc.privateApiFile.String())
				}
				if ddoc.privateDexApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"PRIVATE_DEX_API_FILE := ", ddoc.privateDexApiFile.String())
				}
				if ddoc.removedApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"REMOVED_API_FILE := ", ddoc.removedApiFile.String())
				}
				if ddoc.removedDexApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"REMOVED_DEX_API_FILE := ", ddoc.removedDexApiFile.String())
				}
				if ddoc.exactApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"EXACT_API_FILE := ", ddoc.exactApiFile.String())
				}
				if ddoc.proguardFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"PROGUARD_FILE := ", ddoc.proguardFile.String())
				}
			},
		},
	}
}

func (dstubs *Droidstubs) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(dstubs.stubsSrcJar),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				if dstubs.Javadoc.stubsSrcJar != nil {
					fmt.Fprintln(w, "LOCAL_DROIDDOC_STUBS_SRCJAR := ", dstubs.Javadoc.stubsSrcJar.String())
				}
				if dstubs.apiVersionsXml != nil {
					fmt.Fprintln(w, "LOCAL_DROIDDOC_API_VERSIONS_XML := ", dstubs.apiVersionsXml.String())
				}
				if dstubs.annotationsZip != nil {
					fmt.Fprintln(w, "LOCAL_DROIDDOC_ANNOTATIONS_ZIP := ", dstubs.annotationsZip.String())
				}
				if dstubs.jdiffDocZip != nil {
					fmt.Fprintln(w, "LOCAL_DROIDDOC_JDIFF_DOC_ZIP := ", dstubs.jdiffDocZip.String())
				}
				if dstubs.checkCurrentApiTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", dstubs.Name()+"-check-current-api")
					fmt.Fprintln(w, dstubs.Name()+"-check-current-api:",
						dstubs.checkCurrentApiTimestamp.String())

					fmt.Fprintln(w, ".PHONY: checkapi")
					fmt.Fprintln(w, "checkapi:",
						dstubs.checkCurrentApiTimestamp.String())

					fmt.Fprintln(w, ".PHONY: droidcore")
					fmt.Fprintln(w, "droidcore: checkapi")
				}
				if dstubs.updateCurrentApiTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", dstubs.Name()+"-update-current-api")
					fmt.Fprintln(w, dstubs.Name()+"-update-current-api:",
						dstubs.updateCurrentApiTimestamp.String())

					fmt.Fprintln(w, ".PHONY: update-api")
					fmt.Fprintln(w, "update-api:",
						dstubs.updateCurrentApiTimestamp.String())
				}
				if dstubs.checkLastReleasedApiTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", dstubs.Name()+"-check-last-released-api")
					fmt.Fprintln(w, dstubs.Name()+"-check-last-released-api:",
						dstubs.checkLastReleasedApiTimestamp.String())

					if dstubs.Name() == "api-stubs-docs" || dstubs.Name() == "system-api-stubs-docs" {
						fmt.Fprintln(w, ".PHONY: checkapi")
						fmt.Fprintln(w, "checkapi:",
							dstubs.checkLastReleasedApiTimestamp.String())

						fmt.Fprintln(w, ".PHONY: droidcore")
						fmt.Fprintln(w, "droidcore: checkapi")
					}
				}
				if dstubs.checkNullabilityWarningsTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", dstubs.Name()+"-check-nullability-warnings")
					fmt.Fprintln(w, dstubs.Name()+"-check-nullability-warnings:",
						dstubs.checkNullabilityWarningsTimestamp.String())

					fmt.Fprintln(w, ".PHONY:", "droidcore")
					fmt.Fprintln(w, "droidcore: ", dstubs.Name()+"-check-nullability-warnings")
				}
				apiFilePrefix := "INTERNAL_PLATFORM_"
				if String(dstubs.properties.Api_tag_name) != "" {
					apiFilePrefix += String(dstubs.properties.Api_tag_name) + "_"
				}
				if dstubs.apiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"API_FILE := ", dstubs.apiFile.String())
				}
				if dstubs.dexApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"DEX_API_FILE := ", dstubs.dexApiFile.String())
				}
				if dstubs.privateApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"PRIVATE_API_FILE := ", dstubs.privateApiFile.String())
				}
				if dstubs.privateDexApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"PRIVATE_DEX_API_FILE := ", dstubs.privateDexApiFile.String())
				}
				if dstubs.removedApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"REMOVED_API_FILE := ", dstubs.removedApiFile.String())
				}
				if dstubs.removedDexApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"REMOVED_DEX_API_FILE := ", dstubs.removedDexApiFile.String())
				}
				if dstubs.exactApiFile != nil {
					fmt.Fprintln(w, apiFilePrefix+"EXACT_API_FILE := ", dstubs.exactApiFile.String())
				}
			},
		},
	}
}

func androidMkWriteTestData(data android.Paths, ret *android.AndroidMkData) {
	var testFiles []string
	for _, d := range data {
		testFiles = append(testFiles, d.String()+":"+d.Rel())
	}
	if len(testFiles) > 0 {
		ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
			fmt.Fprintln(w, "LOCAL_COMPATIBILITY_SUPPORT_FILES := "+strings.Join(testFiles, " "))
		})
	}
}
