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

// TODO(jungjw): We'll probably want AndroidMkEntriesProvider.AndroidMkEntries to return multiple
// entries so that this can be more error-proof.
func (library *Library) AndroidMkHostDex(w io.Writer, name string, entries *android.AndroidMkEntries) {
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
		if len(entries.Required) > 0 {
			fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES :=", strings.Join(entries.Required, " "))
		}
		if len(entries.Host_required) > 0 {
			fmt.Fprintln(w, "LOCAL_HOST_REQUIRED_MODULES :=", strings.Join(entries.Host_required, " "))
		}
		if len(entries.Target_required) > 0 {
			fmt.Fprintln(w, "LOCAL_TARGET_REQUIRED_MODULES :=", strings.Join(entries.Target_required, " "))
		}
		if r := library.deviceProperties.Target.Hostdex.Required; len(r) > 0 {
			fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES +=", strings.Join(r, " "))
		}
		fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_java_prebuilt.mk")
	}
}

func (library *Library) AndroidMkEntries() android.AndroidMkEntries {
	if !library.IsForPlatform() {
		return android.AndroidMkEntries{
			Disabled: true,
		}
	}
	return android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(library.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				if len(library.logtagsSrcs) > 0 {
					var logtags []string
					for _, l := range library.logtagsSrcs {
						logtags = append(logtags, l.Rel())
					}
					entries.AddStrings("LOCAL_LOGTAGS_FILES", logtags...)
				}

				if library.installFile == nil {
					entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", true)
				}
				if library.dexJarFile != nil {
					entries.SetPath("LOCAL_SOONG_DEX_JAR", library.dexJarFile)
				}
				if len(library.dexpreopter.builtInstalled) > 0 {
					entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", library.dexpreopter.builtInstalled)
				}
				entries.SetString("LOCAL_SDK_VERSION", library.sdkVersion())
				entries.SetPath("LOCAL_SOONG_CLASSES_JAR", library.implementationAndResourcesJar)
				entries.SetPath("LOCAL_SOONG_HEADER_JAR", library.headerJarFile)

				if library.jacocoReportClassesFile != nil {
					entries.SetPath("LOCAL_SOONG_JACOCO_REPORT_CLASSES_JAR", library.jacocoReportClassesFile)
				}

				entries.AddStrings("LOCAL_EXPORT_SDK_LIBRARIES", library.exportedSdkLibs...)

				if len(library.additionalCheckedModules) != 0 {
					entries.AddStrings("LOCAL_ADDITIONAL_CHECKED_MODULE", library.additionalCheckedModules.Strings()...)
				}

				if library.proguardDictionary != nil {
					entries.SetPath("LOCAL_SOONG_PROGUARD_DICT", library.proguardDictionary)
				}
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
				library.AndroidMkHostDex(w, name, entries)
			},
		},
	}
}

// Called for modules that are a component of a test suite.
func testSuiteComponent(entries *android.AndroidMkEntries, test_suites []string) {
	entries.SetString("LOCAL_MODULE_TAGS", "tests")
	if len(test_suites) > 0 {
		entries.AddStrings("LOCAL_COMPATIBILITY_SUITE", test_suites...)
	} else {
		entries.SetString("LOCAL_COMPATIBILITY_SUITE", "null-suite")
	}
}

func (j *Test) AndroidMkEntries() android.AndroidMkEntries {
	entries := j.Library.AndroidMkEntries()
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, j.testProperties.Test_suites)
		if j.testConfig != nil {
			entries.SetPath("LOCAL_FULL_TEST_CONFIG", j.testConfig)
		}
		androidMkWriteTestData(j.data, entries)
	})

	return entries
}

func (j *TestHelperLibrary) AndroidMkEntries() android.AndroidMkEntries {
	entries := j.Library.AndroidMkEntries()
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, j.testHelperLibraryProperties.Test_suites)
	})

	return entries
}

func (prebuilt *Import) AndroidMkEntries() android.AndroidMkEntries {
	if !prebuilt.IsForPlatform() || !prebuilt.ContainingSdk().Unversioned() {
		return android.AndroidMkEntries{
			Disabled: true,
		}
	}
	return android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.combinedClasspathFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", !Bool(prebuilt.properties.Installable))
				entries.SetPath("LOCAL_SOONG_HEADER_JAR", prebuilt.combinedClasspathFile)
				entries.SetPath("LOCAL_SOONG_CLASSES_JAR", prebuilt.combinedClasspathFile)
				entries.SetString("LOCAL_SDK_VERSION", prebuilt.sdkVersion())
			},
		},
	}
}

func (prebuilt *DexImport) AndroidMkEntries() android.AndroidMkEntries {
	if !prebuilt.IsForPlatform() {
		return android.AndroidMkEntries{
			Disabled: true,
		}
	}
	return android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.maybeStrippedDexJarFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				if prebuilt.dexJarFile != nil {
					entries.SetPath("LOCAL_SOONG_DEX_JAR", prebuilt.dexJarFile)
					// TODO(b/125517186): export the dex jar as a classes jar to match some mis-uses in Make until
					// boot_jars_package_check.mk can check dex jars.
					entries.SetPath("LOCAL_SOONG_HEADER_JAR", prebuilt.dexJarFile)
					entries.SetPath("LOCAL_SOONG_CLASSES_JAR", prebuilt.dexJarFile)
				}
				if len(prebuilt.dexpreopter.builtInstalled) > 0 {
					entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", prebuilt.dexpreopter.builtInstalled)
				}
			},
		},
	}
}

func (prebuilt *AARImport) AndroidMkEntries() android.AndroidMkEntries {
	return android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.classpathFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
				entries.SetPath("LOCAL_SOONG_HEADER_JAR", prebuilt.classpathFile)
				entries.SetPath("LOCAL_SOONG_CLASSES_JAR", prebuilt.classpathFile)
				entries.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", prebuilt.exportPackage)
				entries.SetPath("LOCAL_SOONG_EXPORT_PROGUARD_FLAGS", prebuilt.proguardFlags)
				entries.SetPath("LOCAL_SOONG_STATIC_LIBRARY_EXTRA_PACKAGES", prebuilt.extraAaptPackagesFile)
				entries.SetPath("LOCAL_FULL_MANIFEST_FILE", prebuilt.manifest)
				entries.SetString("LOCAL_SDK_VERSION", prebuilt.sdkVersion())
			},
		},
	}
}

func (binary *Binary) AndroidMkEntries() android.AndroidMkEntries {

	if !binary.isWrapperVariant {
		return android.AndroidMkEntries{
			Class:      "JAVA_LIBRARIES",
			OutputFile: android.OptionalPathForPath(binary.outputFile),
			Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
			ExtraEntries: []android.AndroidMkExtraEntriesFunc{
				func(entries *android.AndroidMkEntries) {
					entries.SetPath("LOCAL_SOONG_HEADER_JAR", binary.headerJarFile)
					entries.SetPath("LOCAL_SOONG_CLASSES_JAR", binary.implementationAndResourcesJar)
					if binary.dexJarFile != nil {
						entries.SetPath("LOCAL_SOONG_DEX_JAR", binary.dexJarFile)
					}
					if len(binary.dexpreopter.builtInstalled) > 0 {
						entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", binary.dexpreopter.builtInstalled)
					}
				},
			},
			ExtraFooters: []android.AndroidMkExtraFootersFunc{
				func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
					fmt.Fprintln(w, "jar_installed_module := $(LOCAL_INSTALLED_MODULE)")
				},
			},
		}
	} else {
		return android.AndroidMkEntries{
			Class:      "EXECUTABLES",
			OutputFile: android.OptionalPathForPath(binary.wrapperFile),
			ExtraEntries: []android.AndroidMkExtraEntriesFunc{
				func(entries *android.AndroidMkEntries) {
					entries.SetBool("LOCAL_STRIP_MODULE", false)
				},
			},
			ExtraFooters: []android.AndroidMkExtraFootersFunc{
				func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
					// Ensure that the wrapper script timestamp is always updated when the jar is updated
					fmt.Fprintln(w, "$(LOCAL_INSTALLED_MODULE): $(jar_installed_module)")
					fmt.Fprintln(w, "jar_installed_module :=")
				},
			},
		}
	}
}

func (app *AndroidApp) AndroidMkEntries() android.AndroidMkEntries {
	return android.AndroidMkEntries{
		Class:      "APPS",
		OutputFile: android.OptionalPathForPath(app.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				// App module names can be overridden.
				entries.SetString("LOCAL_MODULE", app.installApkName)
				entries.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", app.exportPackage)
				if app.dexJarFile != nil {
					entries.SetPath("LOCAL_SOONG_DEX_JAR", app.dexJarFile)
				}
				if app.implementationAndResourcesJar != nil {
					entries.SetPath("LOCAL_SOONG_CLASSES_JAR", app.implementationAndResourcesJar)
				}
				if app.headerJarFile != nil {
					entries.SetPath("LOCAL_SOONG_HEADER_JAR", app.headerJarFile)
				}
				if app.bundleFile != nil {
					entries.SetPath("LOCAL_SOONG_BUNDLE", app.bundleFile)
				}
				if app.jacocoReportClassesFile != nil {
					entries.SetPath("LOCAL_SOONG_JACOCO_REPORT_CLASSES_JAR", app.jacocoReportClassesFile)
				}
				if app.proguardDictionary != nil {
					entries.SetPath("LOCAL_SOONG_PROGUARD_DICT", app.proguardDictionary)
				}

				if app.Name() == "framework-res" {
					entries.SetString("LOCAL_MODULE_PATH", "$(TARGET_OUT_JAVA_LIBRARIES)")
					// Make base_rules.mk not put framework-res in a subdirectory called
					// framework_res.
					entries.SetBoolIfTrue("LOCAL_NO_STANDARD_LIBRARIES", true)
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
					entries.AddStrings("LOCAL_SOONG_DEVICE_RRO_DIRS", deviceRRODirs.Strings()...)
				}
				productRRODirs := filterRRO(product)
				if len(productRRODirs) > 0 {
					entries.AddStrings("LOCAL_SOONG_PRODUCT_RRO_DIRS", productRRODirs.Strings()...)
				}

				entries.SetBoolIfTrue("LOCAL_EXPORT_PACKAGE_RESOURCES", Bool(app.appProperties.Export_package_resources))

				entries.SetPath("LOCAL_FULL_MANIFEST_FILE", app.manifestPath)

				entries.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", Bool(app.appProperties.Privileged))

				entries.SetPath("LOCAL_CERTIFICATE", app.certificate.Pem)
				entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", app.getOverriddenPackages()...)

				for _, jniLib := range app.installJniLibs {
					entries.AddStrings("LOCAL_SOONG_JNI_LIBS_"+jniLib.target.Arch.ArchType.String(), jniLib.name)
				}
				if len(app.dexpreopter.builtInstalled) > 0 {
					entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", app.dexpreopter.builtInstalled)
				}
				for _, split := range app.aapt.splits {
					install := app.onDeviceDir + "/" +
						strings.TrimSuffix(app.installApkName, ".apk") + "_" + split.suffix + ".apk"
					entries.AddStrings("LOCAL_SOONG_BUILT_INSTALLED", split.path.String()+":"+install)
				}
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
				if app.noticeOutputs.Merged.Valid() {
					fmt.Fprintf(w, "$(call dist-for-goals,%s,%s:%s)\n",
						app.installApkName, app.noticeOutputs.Merged.String(), app.installApkName+"_NOTICE")
				}
				if app.noticeOutputs.TxtOutput.Valid() {
					fmt.Fprintf(w, "$(call dist-for-goals,%s,%s:%s)\n",
						app.installApkName, app.noticeOutputs.TxtOutput.String(), app.installApkName+"_NOTICE.txt")
				}
				if app.noticeOutputs.HtmlOutput.Valid() {
					fmt.Fprintf(w, "$(call dist-for-goals,%s,%s:%s)\n",
						app.installApkName, app.noticeOutputs.HtmlOutput.String(), app.installApkName+"_NOTICE.html")
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

func (a *AndroidTest) AndroidMkEntries() android.AndroidMkEntries {
	entries := a.AndroidApp.AndroidMkEntries()
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, a.testProperties.Test_suites)
		if a.testConfig != nil {
			entries.SetPath("LOCAL_FULL_TEST_CONFIG", a.testConfig)
		}
		androidMkWriteTestData(a.data, entries)
	})

	return entries
}

func (a *AndroidTestHelperApp) AndroidMkEntries() android.AndroidMkEntries {
	entries := a.AndroidApp.AndroidMkEntries()
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, a.appTestHelperAppProperties.Test_suites)
	})

	return entries
}

func (a *AndroidLibrary) AndroidMkEntries() android.AndroidMkEntries {
	entries := a.Library.AndroidMkEntries()

	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		if a.aarFile != nil {
			entries.SetPath("LOCAL_SOONG_AAR", a.aarFile)
		}

		if a.Name() == "framework-res" {
			entries.SetString("LOCAL_MODULE_PATH", "$(TARGET_OUT_JAVA_LIBRARIES)")
			// Make base_rules.mk not put framework-res in a subdirectory called
			// framework_res.
			entries.SetBoolIfTrue("LOCAL_NO_STANDARD_LIBRARIES", true)
		}

		entries.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", a.exportPackage)
		entries.SetPath("LOCAL_SOONG_STATIC_LIBRARY_EXTRA_PACKAGES", a.extraAaptPackagesFile)
		entries.SetPath("LOCAL_FULL_MANIFEST_FILE", a.mergedManifestFile)
		entries.AddStrings("LOCAL_SOONG_EXPORT_PROGUARD_FLAGS", a.exportedProguardFlagFiles.Strings()...)
		entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", true)
	})

	return entries
}

func (jd *Javadoc) AndroidMkEntries() android.AndroidMkEntries {
	return android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(jd.stubsSrcJar),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				if BoolDefault(jd.properties.Installable, true) {
					entries.SetPath("LOCAL_DROIDDOC_DOC_ZIP", jd.docZip)
				}
				if jd.stubsSrcJar != nil {
					entries.SetPath("LOCAL_DROIDDOC_STUBS_SRCJAR", jd.stubsSrcJar)
				}
			},
		},
	}
}

func (ddoc *Droiddoc) AndroidMkEntries() android.AndroidMkEntries {
	return android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(ddoc.stubsSrcJar),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				if BoolDefault(ddoc.Javadoc.properties.Installable, true) && ddoc.Javadoc.docZip != nil {
					entries.SetPath("LOCAL_DROIDDOC_DOC_ZIP", ddoc.Javadoc.docZip)
				}
				if ddoc.Javadoc.stubsSrcJar != nil {
					entries.SetPath("LOCAL_DROIDDOC_STUBS_SRCJAR", ddoc.Javadoc.stubsSrcJar)
				}
				apiFilePrefix := "INTERNAL_PLATFORM_"
				if String(ddoc.properties.Api_tag_name) != "" {
					apiFilePrefix += String(ddoc.properties.Api_tag_name) + "_"
				}
				if ddoc.apiFile != nil {
					entries.SetPath(apiFilePrefix+"API_FILE", ddoc.apiFile)
				}
				if ddoc.dexApiFile != nil {
					entries.SetPath(apiFilePrefix+"DEX_API_FILE", ddoc.dexApiFile)
				}
				if ddoc.privateApiFile != nil {
					entries.SetPath(apiFilePrefix+"PRIVATE_API_FILE", ddoc.privateApiFile)
				}
				if ddoc.privateDexApiFile != nil {
					entries.SetPath(apiFilePrefix+"PRIVATE_DEX_API_FILE", ddoc.privateDexApiFile)
				}
				if ddoc.removedApiFile != nil {
					entries.SetPath(apiFilePrefix+"REMOVED_API_FILE", ddoc.removedApiFile)
				}
				if ddoc.removedDexApiFile != nil {
					entries.SetPath(apiFilePrefix+"REMOVED_DEX_API_FILE", ddoc.removedDexApiFile)
				}
				if ddoc.exactApiFile != nil {
					entries.SetPath(apiFilePrefix+"EXACT_API_FILE", ddoc.exactApiFile)
				}
				if ddoc.proguardFile != nil {
					entries.SetPath(apiFilePrefix+"PROGUARD_FILE", ddoc.proguardFile)
				}
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
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
			},
		},
	}
}

func (dstubs *Droidstubs) AndroidMkEntries() android.AndroidMkEntries {
	return android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(dstubs.stubsSrcJar),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				if dstubs.Javadoc.stubsSrcJar != nil {
					entries.SetPath("LOCAL_DROIDDOC_STUBS_SRCJAR", dstubs.Javadoc.stubsSrcJar)
				}
				if dstubs.apiVersionsXml != nil {
					entries.SetPath("LOCAL_DROIDDOC_API_VERSIONS_XML", dstubs.apiVersionsXml)
				}
				if dstubs.annotationsZip != nil {
					entries.SetPath("LOCAL_DROIDDOC_ANNOTATIONS_ZIP", dstubs.annotationsZip)
				}
				if dstubs.jdiffDocZip != nil {
					entries.SetPath("LOCAL_DROIDDOC_JDIFF_DOC_ZIP", dstubs.jdiffDocZip)
				}
				apiFilePrefix := "INTERNAL_PLATFORM_"
				if String(dstubs.properties.Api_tag_name) != "" {
					apiFilePrefix += String(dstubs.properties.Api_tag_name) + "_"
				}
				if dstubs.apiFile != nil {
					entries.SetPath(apiFilePrefix+"API_FILE", dstubs.apiFile)
				}
				if dstubs.dexApiFile != nil {
					entries.SetPath(apiFilePrefix+"DEX_API_FILE", dstubs.dexApiFile)
				}
				if dstubs.privateApiFile != nil {
					entries.SetPath(apiFilePrefix+"PRIVATE_API_FILE", dstubs.privateApiFile)
				}
				if dstubs.privateDexApiFile != nil {
					entries.SetPath(apiFilePrefix+"PRIVATE_DEX_API_FILE", dstubs.privateDexApiFile)
				}
				if dstubs.removedApiFile != nil {
					entries.SetPath(apiFilePrefix+"REMOVED_API_FILE", dstubs.removedApiFile)
				}
				if dstubs.removedDexApiFile != nil {
					entries.SetPath(apiFilePrefix+"REMOVED_DEX_API_FILE", dstubs.removedDexApiFile)
				}
				if dstubs.exactApiFile != nil {
					entries.SetPath(apiFilePrefix+"EXACT_API_FILE", dstubs.exactApiFile)
				}
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
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
				if dstubs.apiLintTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", dstubs.Name()+"-api-lint")
					fmt.Fprintln(w, dstubs.Name()+"-api-lint:",
						dstubs.apiLintTimestamp.String())

					fmt.Fprintln(w, ".PHONY: checkapi")
					fmt.Fprintln(w, "checkapi:",
						dstubs.apiLintTimestamp.String())

					fmt.Fprintln(w, ".PHONY: droidcore")
					fmt.Fprintln(w, "droidcore: checkapi")
				}
				if dstubs.checkNullabilityWarningsTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", dstubs.Name()+"-check-nullability-warnings")
					fmt.Fprintln(w, dstubs.Name()+"-check-nullability-warnings:",
						dstubs.checkNullabilityWarningsTimestamp.String())

					fmt.Fprintln(w, ".PHONY:", "droidcore")
					fmt.Fprintln(w, "droidcore: ", dstubs.Name()+"-check-nullability-warnings")
				}
			},
		},
	}
}

func (a *AndroidAppImport) AndroidMkEntries() android.AndroidMkEntries {
	return android.AndroidMkEntries{
		Class:      "APPS",
		OutputFile: android.OptionalPathForPath(a.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", Bool(a.properties.Privileged))
				if a.certificate != nil {
					entries.SetPath("LOCAL_CERTIFICATE", a.certificate.Pem)
				} else {
					entries.SetString("LOCAL_CERTIFICATE", "PRESIGNED")
				}
				entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", a.properties.Overrides...)
				if len(a.dexpreopter.builtInstalled) > 0 {
					entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", a.dexpreopter.builtInstalled)
				}
				entries.AddStrings("LOCAL_INSTALLED_MODULE_STEM", a.installPath.Rel())
			},
		},
	}
}

func (a *AndroidTestImport) AndroidMkEntries() android.AndroidMkEntries {
	entries := a.AndroidAppImport.AndroidMkEntries()
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, a.testProperties.Test_suites)
		androidMkWriteTestData(a.data, entries)
	})
	return entries
}

func androidMkWriteTestData(data android.Paths, entries *android.AndroidMkEntries) {
	var testFiles []string
	for _, d := range data {
		testFiles = append(testFiles, d.String()+":"+d.Rel())
	}
	entries.AddStrings("LOCAL_COMPATIBILITY_SUPPORT_FILES", testFiles...)
}
