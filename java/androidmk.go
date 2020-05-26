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

	"android/soong/android"
)

func (library *Library) AndroidMkEntriesHostDex() android.AndroidMkEntries {
	hostDexNeeded := Bool(library.deviceProperties.Hostdex) && !library.Host()
	if !library.IsForPlatform() {
		// Don't emit hostdex modules from the APEX variants
		hostDexNeeded = false
	}

	if hostDexNeeded {
		var output android.Path
		if library.dexJarFile != nil {
			output = library.dexJarFile
		} else {
			output = library.implementationAndResourcesJar
		}
		return android.AndroidMkEntries{
			Class:      "JAVA_LIBRARIES",
			SubName:    "-hostdex",
			OutputFile: android.OptionalPathForPath(output),
			Required:   library.deviceProperties.Target.Hostdex.Required,
			Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
			ExtraEntries: []android.AndroidMkExtraEntriesFunc{
				func(entries *android.AndroidMkEntries) {
					entries.SetBool("LOCAL_IS_HOST_MODULE", true)
					entries.SetPath("LOCAL_PREBUILT_MODULE_FILE", output)
					if library.dexJarFile != nil {
						entries.SetPath("LOCAL_SOONG_DEX_JAR", library.dexJarFile)
					}
					entries.SetPath("LOCAL_SOONG_HEADER_JAR", library.headerJarFile)
					entries.SetPath("LOCAL_SOONG_CLASSES_JAR", library.implementationAndResourcesJar)
					entries.SetString("LOCAL_MODULE_STEM", library.Stem()+"-hostdex")
				},
			},
		}
	}
	return android.AndroidMkEntries{Disabled: true}
}

func (library *Library) AndroidMkEntries() []android.AndroidMkEntries {
	var entriesList []android.AndroidMkEntries

	mainEntries := android.AndroidMkEntries{Disabled: true}

	// For a java library built for an APEX, we don't need Make module
	hideFromMake := !library.IsForPlatform()
	// If not available for platform, don't emit to make.
	if !library.ApexModuleBase.AvailableFor(android.AvailableToPlatform) {
		hideFromMake = true
	}
	if hideFromMake {
		// May still need to add some additional dependencies. This will be called
		// once for the platform variant (even if it is not being used) and once each
		// for the APEX specific variants. In order to avoid adding the dependency
		// multiple times only add it for the platform variant.
		checkedModulePaths := library.additionalCheckedModules
		if library.IsForPlatform() && len(checkedModulePaths) != 0 {
			mainEntries = android.AndroidMkEntries{
				Class: "FAKE",
				// Need at least one output file in order for this to take effect.
				OutputFile: android.OptionalPathForPath(checkedModulePaths[0]),
				Include:    "$(BUILD_PHONY_PACKAGE)",
				ExtraEntries: []android.AndroidMkExtraEntriesFunc{
					func(entries *android.AndroidMkEntries) {
						entries.AddStrings("LOCAL_ADDITIONAL_CHECKED_MODULE", checkedModulePaths.Strings()...)
					},
				},
			}
		}
	} else {
		mainEntries = android.AndroidMkEntries{
			Class:      "JAVA_LIBRARIES",
			DistFile:   android.OptionalPathForPath(library.distFile),
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
					entries.SetString("LOCAL_SDK_VERSION", library.sdkVersion().raw)
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
					entries.SetString("LOCAL_MODULE_STEM", library.Stem())

					entries.SetOptionalPaths("LOCAL_SOONG_LINT_REPORTS", library.linter.reports)
				},
			},
		}
	}

	hostDexEntries := library.AndroidMkEntriesHostDex()

	entriesList = append(entriesList, mainEntries, hostDexEntries)
	return entriesList
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

func (j *Test) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := j.Library.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, j.testProperties.Test_suites)
		if j.testConfig != nil {
			entries.SetPath("LOCAL_FULL_TEST_CONFIG", j.testConfig)
		}
		androidMkWriteTestData(j.data, entries)
		if !BoolDefault(j.testProperties.Auto_gen_config, true) {
			entries.SetString("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", "true")
		}
	})

	return entriesList
}

func (j *TestHelperLibrary) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := j.Library.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, j.testHelperLibraryProperties.Test_suites)
	})

	return entriesList
}

func (prebuilt *Import) AndroidMkEntries() []android.AndroidMkEntries {
	if !prebuilt.IsForPlatform() || !prebuilt.ContainingSdk().Unversioned() {
		return []android.AndroidMkEntries{android.AndroidMkEntries{
			Disabled: true,
		}}
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.combinedClasspathFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", !Bool(prebuilt.properties.Installable))
				entries.SetPath("LOCAL_SOONG_HEADER_JAR", prebuilt.combinedClasspathFile)
				entries.SetPath("LOCAL_SOONG_CLASSES_JAR", prebuilt.combinedClasspathFile)
				entries.SetString("LOCAL_SDK_VERSION", prebuilt.sdkVersion().raw)
				entries.SetString("LOCAL_MODULE_STEM", prebuilt.Stem())
			},
		},
	}}
}

func (prebuilt *DexImport) AndroidMkEntries() []android.AndroidMkEntries {
	if !prebuilt.IsForPlatform() {
		return []android.AndroidMkEntries{android.AndroidMkEntries{
			Disabled: true,
		}}
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
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
				entries.SetString("LOCAL_MODULE_STEM", prebuilt.Stem())
			},
		},
	}}
}

func (prebuilt *AARImport) AndroidMkEntries() []android.AndroidMkEntries {
	if !prebuilt.IsForPlatform() {
		return []android.AndroidMkEntries{{
			Disabled: true,
		}}
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
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
				entries.SetString("LOCAL_SDK_VERSION", prebuilt.sdkVersion().raw)
			},
		},
	}}
}

func (binary *Binary) AndroidMkEntries() []android.AndroidMkEntries {

	if !binary.isWrapperVariant {
		return []android.AndroidMkEntries{android.AndroidMkEntries{
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
		}}
	} else {
		return []android.AndroidMkEntries{android.AndroidMkEntries{
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
		}}
	}
}

func (app *AndroidApp) AndroidMkEntries() []android.AndroidMkEntries {
	if !app.IsForPlatform() || app.appProperties.HideFromMake {
		return []android.AndroidMkEntries{android.AndroidMkEntries{
			Disabled: true,
		}}
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "APPS",
		OutputFile: android.OptionalPathForPath(app.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				// App module names can be overridden.
				entries.SetString("LOCAL_MODULE", app.installApkName)
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", app.appProperties.PreventInstall)
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

				entries.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", app.Privileged())

				entries.SetString("LOCAL_CERTIFICATE", app.certificate.AndroidMkString())
				entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", app.getOverriddenPackages()...)

				if app.embeddedJniLibs {
					jniSymbols := app.JNISymbolsInstalls(app.installPathForJNISymbols.String())
					entries.SetString("LOCAL_SOONG_JNI_LIBS_SYMBOLS", jniSymbols.String())
				} else {
					for _, jniLib := range app.jniLibs {
						entries.AddStrings("LOCAL_SOONG_JNI_LIBS_"+jniLib.target.Arch.ArchType.String(), jniLib.name)
					}
				}

				if len(app.jniCoverageOutputs) > 0 {
					entries.AddStrings("LOCAL_PREBUILT_COVERAGE_ARCHIVE", app.jniCoverageOutputs.Strings()...)
				}
				if len(app.dexpreopter.builtInstalled) > 0 {
					entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", app.dexpreopter.builtInstalled)
				}
				for _, extra := range app.extraOutputFiles {
					install := app.onDeviceDir + "/" + extra.Base()
					entries.AddStrings("LOCAL_SOONG_BUILT_INSTALLED", extra.String()+":"+install)
				}

				entries.SetOptionalPaths("LOCAL_SOONG_LINT_REPORTS", app.linter.reports)
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
	}}
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

func (a *AndroidTest) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := a.AndroidApp.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, a.testProperties.Test_suites)
		if a.testConfig != nil {
			entries.SetPath("LOCAL_FULL_TEST_CONFIG", a.testConfig)
		}
		androidMkWriteTestData(a.data, entries)
	})

	return entriesList
}

func (a *AndroidTestHelperApp) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := a.AndroidApp.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, a.appTestHelperAppProperties.Test_suites)
	})

	return entriesList
}

func (a *AndroidLibrary) AndroidMkEntries() []android.AndroidMkEntries {
	if !a.IsForPlatform() {
		return []android.AndroidMkEntries{{
			Disabled: true,
		}}
	}
	entriesList := a.Library.AndroidMkEntries()
	entries := &entriesList[0]

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

	return entriesList
}

func (jd *Javadoc) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
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
	}}
}

func (ddoc *Droiddoc) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
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

					fmt.Fprintln(w, ".PHONY: checkapi")
					fmt.Fprintln(w, "checkapi:",
						ddoc.checkLastReleasedApiTimestamp.String())

					fmt.Fprintln(w, ".PHONY: droidcore")
					fmt.Fprintln(w, "droidcore: checkapi")
				}
			},
		},
	}}
}

func (dstubs *Droidstubs) AndroidMkEntries() []android.AndroidMkEntries {
	// If the stubsSrcJar is not generated (because generate_stubs is false) then
	// use the api file as the output file to ensure the relevant phony targets
	// are created in make if only the api txt file is being generated. This is
	// needed because an invalid output file would prevent the make entries from
	// being written.
	// TODO(b/146727827): Revert when we do not need to generate stubs and API separately.
	distFile := android.OptionalPathForPath(dstubs.apiFile)
	outputFile := android.OptionalPathForPath(dstubs.stubsSrcJar)
	if !outputFile.Valid() {
		outputFile = distFile
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		DistFile:   distFile,
		OutputFile: outputFile,
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
				if dstubs.metadataZip != nil {
					entries.SetPath("LOCAL_DROIDDOC_METADATA_ZIP", dstubs.metadataZip)
				}
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
				if dstubs.apiFile != nil {
					fmt.Fprintf(w, ".PHONY: %s %s.txt\n", dstubs.Name(), dstubs.Name())
					fmt.Fprintf(w, "%s %s.txt: %s\n", dstubs.Name(), dstubs.Name(), dstubs.apiFile)
				}
				if dstubs.removedApiFile != nil {
					fmt.Fprintf(w, ".PHONY: %s %s.txt\n", dstubs.Name(), dstubs.Name())
					fmt.Fprintf(w, "%s %s.txt: %s\n", dstubs.Name(), dstubs.Name(), dstubs.removedApiFile)
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

					fmt.Fprintln(w, ".PHONY: checkapi")
					fmt.Fprintln(w, "checkapi:",
						dstubs.checkLastReleasedApiTimestamp.String())

					fmt.Fprintln(w, ".PHONY: droidcore")
					fmt.Fprintln(w, "droidcore: checkapi")
				}
				if dstubs.apiLintTimestamp != nil {
					fmt.Fprintln(w, ".PHONY:", dstubs.Name()+"-api-lint")
					fmt.Fprintln(w, dstubs.Name()+"-api-lint:",
						dstubs.apiLintTimestamp.String())

					fmt.Fprintln(w, ".PHONY: checkapi")
					fmt.Fprintln(w, "checkapi:",
						dstubs.Name()+"-api-lint")

					fmt.Fprintln(w, ".PHONY: droidcore")
					fmt.Fprintln(w, "droidcore: checkapi")

					if dstubs.apiLintReport != nil {
						fmt.Fprintf(w, "$(call dist-for-goals,%s,%s:%s)\n", dstubs.Name()+"-api-lint",
							dstubs.apiLintReport.String(), "apilint/"+dstubs.Name()+"-lint-report.txt")
					}
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
	}}
}

func (a *AndroidAppImport) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "APPS",
		OutputFile: android.OptionalPathForPath(a.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", a.Privileged())
				entries.SetString("LOCAL_CERTIFICATE", a.certificate.AndroidMkString())
				entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", a.properties.Overrides...)
				if len(a.dexpreopter.builtInstalled) > 0 {
					entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", a.dexpreopter.builtInstalled)
				}
				entries.AddStrings("LOCAL_INSTALLED_MODULE_STEM", a.installPath.Rel())
			},
		},
	}}
}

func (a *AndroidTestImport) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := a.AndroidAppImport.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, a.testProperties.Test_suites)
		androidMkWriteTestData(a.data, entries)
	})
	return entriesList
}

func androidMkWriteTestData(data android.Paths, entries *android.AndroidMkEntries) {
	var testFiles []string
	for _, d := range data {
		testFiles = append(testFiles, d.String()+":"+d.Rel())
	}
	entries.AddStrings("LOCAL_COMPATIBILITY_SUPPORT_FILES", testFiles...)
}

func (r *RuntimeResourceOverlay) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(r.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_CERTIFICATE", r.certificate.AndroidMkString())
				entries.SetPath("LOCAL_MODULE_PATH", r.installDir.ToMakePath())
				entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", r.properties.Overrides...)
			},
		},
	}}
}

func (apkSet *AndroidAppSet) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{
		android.AndroidMkEntries{
			Class:      "APPS",
			OutputFile: android.OptionalPathForPath(apkSet.packedOutput),
			Include:    "$(BUILD_SYSTEM)/soong_android_app_set.mk",
			ExtraEntries: []android.AndroidMkExtraEntriesFunc{
				func(entries *android.AndroidMkEntries) {
					entries.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", apkSet.Privileged())
					entries.SetString("LOCAL_APK_SET_MASTER_FILE", apkSet.masterFile)
					entries.SetPath("LOCAL_APKCERTS_FILE", apkSet.apkcertsFile)
					entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", apkSet.properties.Overrides...)
				},
			},
		},
	}
}
