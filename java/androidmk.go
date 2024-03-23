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

	"github.com/google/blueprint/proptools"
)

func (library *Library) AndroidMkEntriesHostDex() android.AndroidMkEntries {
	hostDexNeeded := Bool(library.deviceProperties.Hostdex) && !library.Host()
	if library.hideApexVariantFromMake {
		hostDexNeeded = false
	}

	if hostDexNeeded {
		var output android.Path
		if library.dexJarFile.IsSet() {
			output = library.dexJarFile.Path()
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
				func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
					entries.SetBool("LOCAL_IS_HOST_MODULE", true)
					entries.SetPath("LOCAL_PREBUILT_MODULE_FILE", output)
					if library.dexJarFile.IsSet() {
						entries.SetPath("LOCAL_SOONG_DEX_JAR", library.dexJarFile.Path())
					}
					entries.SetPath("LOCAL_SOONG_INSTALLED_MODULE", library.hostdexInstallFile)
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

	if library.Os() == android.Windows {
		// Make does not support Windows Java modules
		return nil
	}

	if library.hideApexVariantFromMake {
		// For a java library built for an APEX, we don't need a Make module for itself. Otherwise, it
		// will conflict with the platform variant because they have the same module name in the
		// makefile. However, we need to add its dexpreopt outputs as sub-modules, if it is preopted.
		dexpreoptEntries := library.dexpreopter.AndroidMkEntriesForApex()
		if len(dexpreoptEntries) > 0 {
			entriesList = append(entriesList, dexpreoptEntries...)
		}
		entriesList = append(entriesList, android.AndroidMkEntries{Disabled: true})
	} else if !library.ApexModuleBase.AvailableFor(android.AvailableToPlatform) {
		// Platform variant.  If not available for the platform, we don't need Make module.
		entriesList = append(entriesList, android.AndroidMkEntries{Disabled: true})
	} else if proptools.Bool(library.properties.Headers_only) {
		// If generating headers only then don't expose to Make.
		entriesList = append(entriesList, android.AndroidMkEntries{Disabled: true})
	} else {
		entriesList = append(entriesList, android.AndroidMkEntries{
			Class:      "JAVA_LIBRARIES",
			OutputFile: android.OptionalPathForPath(library.outputFile),
			Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
			ExtraEntries: []android.AndroidMkExtraEntriesFunc{
				func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
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
					if library.dexJarFile.IsSet() {
						entries.SetPath("LOCAL_SOONG_DEX_JAR", library.dexJarFile.Path())
					}
					if len(library.dexpreopter.builtInstalled) > 0 {
						entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", library.dexpreopter.builtInstalled)
					}
					entries.SetString("LOCAL_SDK_VERSION", library.sdkVersion.String())
					entries.SetPath("LOCAL_SOONG_CLASSES_JAR", library.implementationAndResourcesJar)
					entries.SetPath("LOCAL_SOONG_HEADER_JAR", library.headerJarFile)

					if library.jacocoReportClassesFile != nil {
						entries.SetPath("LOCAL_SOONG_JACOCO_REPORT_CLASSES_JAR", library.jacocoReportClassesFile)
					}

					requiredUsesLibs, optionalUsesLibs := library.classLoaderContexts.UsesLibs()
					entries.AddStrings("LOCAL_EXPORT_SDK_LIBRARIES", append(requiredUsesLibs, optionalUsesLibs...)...)

					entries.SetOptionalPath("LOCAL_SOONG_PROGUARD_DICT", library.dexer.proguardDictionary)
					entries.SetOptionalPath("LOCAL_SOONG_PROGUARD_USAGE_ZIP", library.dexer.proguardUsageZip)
					entries.SetString("LOCAL_MODULE_STEM", library.Stem())

					entries.SetOptionalPaths("LOCAL_SOONG_LINT_REPORTS", library.linter.reports)

					if library.dexpreopter.configPath != nil {
						entries.SetPath("LOCAL_SOONG_DEXPREOPT_CONFIG", library.dexpreopter.configPath)
					}
					android.SetAconfigFileMkEntries(&library.ModuleBase, entries, library.mergedAconfigFiles)
				},
			},
		})
	}

	entriesList = append(entriesList, library.AndroidMkEntriesHostDex())

	return entriesList
}

func (j *JavaFuzzTest) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := j.Library.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		entries.AddStrings("LOCAL_COMPATIBILITY_SUITE", "null-suite")
		androidMkWriteTestData(android.Paths{j.implementationJarFile}, entries)
		androidMkWriteTestData(j.jniFilePaths, entries)
		if j.fuzzPackagedModule.Corpus != nil {
			androidMkWriteTestData(j.fuzzPackagedModule.Corpus, entries)
		}
		if j.fuzzPackagedModule.Dictionary != nil {
			androidMkWriteTestData(android.Paths{j.fuzzPackagedModule.Dictionary}, entries)
		}
	})
	return entriesList
}

// Called for modules that are a component of a test suite.
func testSuiteComponent(entries *android.AndroidMkEntries, test_suites []string, perTestcaseDirectory bool) {
	entries.SetString("LOCAL_MODULE_TAGS", "tests")
	if len(test_suites) > 0 {
		entries.AddCompatibilityTestSuites(test_suites...)
	} else {
		entries.AddCompatibilityTestSuites("null-suite")
	}
	entries.SetBoolIfTrue("LOCAL_COMPATIBILITY_PER_TESTCASE_DIRECTORY", perTestcaseDirectory)
}

func (j *Test) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := j.Library.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, j.testProperties.Test_suites, Bool(j.testProperties.Per_testcase_directory))
		if j.testConfig != nil {
			entries.SetPath("LOCAL_FULL_TEST_CONFIG", j.testConfig)
		}
		androidMkWriteExtraTestConfigs(j.extraTestConfigs, entries)
		androidMkWriteTestData(j.data, entries)
		if !BoolDefault(j.testProperties.Auto_gen_config, true) {
			entries.SetString("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", "true")
		}
		entries.AddStrings("LOCAL_TEST_MAINLINE_MODULES", j.testProperties.Test_mainline_modules...)

		j.testProperties.Test_options.CommonTestOptions.SetAndroidMkEntries(entries)
	})

	return entriesList
}

func androidMkWriteExtraTestConfigs(extraTestConfigs android.Paths, entries *android.AndroidMkEntries) {
	if len(extraTestConfigs) > 0 {
		entries.AddStrings("LOCAL_EXTRA_FULL_TEST_CONFIGS", extraTestConfigs.Strings()...)
	}
}

func (j *TestHelperLibrary) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := j.Library.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, j.testHelperLibraryProperties.Test_suites, Bool(j.testHelperLibraryProperties.Per_testcase_directory))
	})

	return entriesList
}

func (prebuilt *Import) AndroidMkEntries() []android.AndroidMkEntries {
	if prebuilt.hideApexVariantFromMake {
		return []android.AndroidMkEntries{}
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:        "JAVA_LIBRARIES",
		OverrideName: prebuilt.BaseModuleName(),
		OutputFile:   android.OptionalPathForPath(prebuilt.combinedClasspathFile),
		Include:      "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", !Bool(prebuilt.properties.Installable))
				if prebuilt.dexJarFile.IsSet() {
					entries.SetPath("LOCAL_SOONG_DEX_JAR", prebuilt.dexJarFile.Path())
				}
				entries.SetPath("LOCAL_SOONG_HEADER_JAR", prebuilt.combinedClasspathFile)
				entries.SetPath("LOCAL_SOONG_CLASSES_JAR", prebuilt.combinedClasspathFile)
				entries.SetString("LOCAL_SDK_VERSION", prebuilt.sdkVersion.String())
				entries.SetString("LOCAL_MODULE_STEM", prebuilt.Stem())
				// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts
			},
		},
	}}
}

func (prebuilt *DexImport) AndroidMkEntries() []android.AndroidMkEntries {
	if prebuilt.hideApexVariantFromMake {
		return []android.AndroidMkEntries{android.AndroidMkEntries{
			Disabled: true,
		}}
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.dexJarFile.Path()),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				if prebuilt.dexJarFile.IsSet() {
					entries.SetPath("LOCAL_SOONG_DEX_JAR", prebuilt.dexJarFile.Path())
				}
				if len(prebuilt.dexpreopter.builtInstalled) > 0 {
					entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", prebuilt.dexpreopter.builtInstalled)
				}
				entries.SetString("LOCAL_MODULE_STEM", prebuilt.Stem())
				// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts
			},
		},
	}}
}

func (prebuilt *AARImport) AndroidMkEntries() []android.AndroidMkEntries {
	if prebuilt.hideApexVariantFromMake {
		return []android.AndroidMkEntries{{
			Disabled: true,
		}}
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.classpathFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
				entries.SetPath("LOCAL_SOONG_HEADER_JAR", prebuilt.classpathFile)
				entries.SetPath("LOCAL_SOONG_CLASSES_JAR", prebuilt.classpathFile)
				entries.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", prebuilt.exportPackage)
				entries.SetPath("LOCAL_SOONG_TRANSITIVE_RES_PACKAGES", prebuilt.transitiveAaptResourcePackagesFile)
				entries.SetPath("LOCAL_SOONG_EXPORT_PROGUARD_FLAGS", prebuilt.proguardFlags)
				entries.SetPath("LOCAL_SOONG_STATIC_LIBRARY_EXTRA_PACKAGES", prebuilt.extraAaptPackagesFile)
				entries.SetPath("LOCAL_FULL_MANIFEST_FILE", prebuilt.manifest)
				entries.SetString("LOCAL_SDK_VERSION", prebuilt.sdkVersion.String())
				// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts
			},
		},
	}}
}

func (binary *Binary) AndroidMkEntries() []android.AndroidMkEntries {
	if binary.Os() == android.Windows {
		// Make does not support Windows Java modules
		return nil
	}

	if !binary.isWrapperVariant {
		return []android.AndroidMkEntries{android.AndroidMkEntries{
			Class:      "JAVA_LIBRARIES",
			OutputFile: android.OptionalPathForPath(binary.outputFile),
			Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
			ExtraEntries: []android.AndroidMkExtraEntriesFunc{
				func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
					entries.SetPath("LOCAL_SOONG_HEADER_JAR", binary.headerJarFile)
					entries.SetPath("LOCAL_SOONG_CLASSES_JAR", binary.implementationAndResourcesJar)
					if binary.dexJarFile.IsSet() {
						entries.SetPath("LOCAL_SOONG_DEX_JAR", binary.dexJarFile.Path())
					}
					if len(binary.dexpreopter.builtInstalled) > 0 {
						entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", binary.dexpreopter.builtInstalled)
					}
					android.SetAconfigFileMkEntries(&binary.ModuleBase, entries, binary.mergedAconfigFiles)
				},
			},
			ExtraFooters: []android.AndroidMkExtraFootersFunc{
				func(w io.Writer, name, prefix, moduleDir string) {
					fmt.Fprintln(w, "jar_installed_module := $(LOCAL_INSTALLED_MODULE)")
				},
			},
		}}
	} else {
		outputFile := binary.wrapperFile

		return []android.AndroidMkEntries{android.AndroidMkEntries{
			Class:      "EXECUTABLES",
			OutputFile: android.OptionalPathForPath(outputFile),
			ExtraEntries: []android.AndroidMkExtraEntriesFunc{
				func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
					entries.SetBool("LOCAL_STRIP_MODULE", false)
				},
			},
			ExtraFooters: []android.AndroidMkExtraFootersFunc{
				func(w io.Writer, name, prefix, moduleDir string) {
					// Ensure that the wrapper script timestamp is always updated when the jar is updated
					fmt.Fprintln(w, "$(LOCAL_INSTALLED_MODULE): $(jar_installed_module)")
					fmt.Fprintln(w, "jar_installed_module :=")
				},
			},
		}}
	}
}

func (app *AndroidApp) AndroidMkEntries() []android.AndroidMkEntries {
	if app.hideApexVariantFromMake || app.IsHideFromMake() {
		return []android.AndroidMkEntries{android.AndroidMkEntries{
			Disabled: true,
		}}
	}
	var required []string
	if proptools.Bool(app.appProperties.Generate_product_characteristics_rro) {
		required = []string{app.productCharacteristicsRROPackageName()}
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "APPS",
		OutputFile: android.OptionalPathForPath(app.outputFile),
		Include:    "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		Required:   required,
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				// App module names can be overridden.
				entries.SetString("LOCAL_MODULE", app.installApkName)
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", app.appProperties.PreventInstall)
				if app.headerJarFile != nil {
					entries.SetPath("LOCAL_SOONG_HEADER_JAR", app.headerJarFile)
				}
				entries.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", app.exportPackage)
				if app.dexJarFile.IsSet() {
					entries.SetPath("LOCAL_SOONG_DEX_JAR", app.dexJarFile.Path())
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
				entries.SetOptionalPath("LOCAL_SOONG_PROGUARD_DICT", app.dexer.proguardDictionary)
				entries.SetOptionalPath("LOCAL_SOONG_PROGUARD_USAGE_ZIP", app.dexer.proguardUsageZip)

				if app.Name() == "framework-res" {
					entries.SetString("LOCAL_MODULE_PATH", "$(TARGET_OUT_JAVA_LIBRARIES)")
					// Make base_rules.mk not put framework-res in a subdirectory called
					// framework_res.
					entries.SetBoolIfTrue("LOCAL_NO_STANDARD_LIBRARIES", true)
				}

				filterRRO := func(filter overlayType) android.Paths {
					var paths android.Paths
					seen := make(map[android.Path]bool)
					for _, d := range app.rroDirsDepSet.ToList() {
						if d.overlayType == filter {
							if seen[d.path] {
								continue
							}
							seen[d.path] = true
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
						var partitionTag string

						// Mimic the creation of partition_tag in build/make,
						// which defaults to an empty string when the partition is system.
						// Otherwise, capitalize with a leading _
						if jniLib.partition == "system" {
							partitionTag = ""
						} else {
							split := strings.Split(jniLib.partition, "/")
							partitionTag = "_" + strings.ToUpper(split[len(split)-1])
						}
						entries.AddStrings("LOCAL_SOONG_JNI_LIBS_PARTITION_"+jniLib.target.Arch.ArchType.String(),
							jniLib.name+":"+partitionTag)
					}
				}

				if len(app.jniCoverageOutputs) > 0 {
					entries.AddStrings("LOCAL_PREBUILT_COVERAGE_ARCHIVE", app.jniCoverageOutputs.Strings()...)
				}
				if len(app.dexpreopter.builtInstalled) > 0 {
					entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", app.dexpreopter.builtInstalled)
				}
				if app.dexpreopter.configPath != nil {
					entries.SetPath("LOCAL_SOONG_DEXPREOPT_CONFIG", app.dexpreopter.configPath)
				}
				for _, extra := range app.extraOutputFiles {
					install := app.onDeviceDir + "/" + extra.Base()
					entries.AddStrings("LOCAL_SOONG_BUILT_INSTALLED", extra.String()+":"+install)
				}

				entries.SetOptionalPaths("LOCAL_SOONG_LINT_REPORTS", app.linter.reports)

				if app.Name() != "framework-res" {
					android.SetAconfigFileMkEntries(&app.ModuleBase, entries, app.mergedAconfigFiles)
				}
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string) {
				if app.javaApiUsedByOutputFile.String() != "" {
					fmt.Fprintf(w, "$(call dist-for-goals,%s,%s:%s/$(notdir %s))\n",
						app.installApkName, app.javaApiUsedByOutputFile.String(), "java_apis_used_by_apex", app.javaApiUsedByOutputFile.String())
				}
			},
		}},
	}
}

func (a *AndroidApp) getOverriddenPackages() []string {
	var overridden []string
	if len(a.overridableAppProperties.Overrides) > 0 {
		overridden = append(overridden, a.overridableAppProperties.Overrides...)
	}
	return overridden
}

func (a *AndroidTest) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := a.AndroidApp.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, a.testProperties.Test_suites, Bool(a.testProperties.Per_testcase_directory))
		if a.testConfig != nil {
			entries.SetPath("LOCAL_FULL_TEST_CONFIG", a.testConfig)
		}
		androidMkWriteExtraTestConfigs(a.extraTestConfigs, entries)
		androidMkWriteTestData(a.data, entries)
		entries.AddStrings("LOCAL_TEST_MAINLINE_MODULES", a.testProperties.Test_mainline_modules...)
	})

	return entriesList
}

func (a *AndroidTestHelperApp) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := a.AndroidApp.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, a.appTestHelperAppProperties.Test_suites, Bool(a.appTestHelperAppProperties.Per_testcase_directory))
		// introduce a flag variable to control the generation of the .config file
		entries.SetString("LOCAL_DISABLE_TEST_CONFIG", "true")
	})

	return entriesList
}

func (a *AndroidLibrary) AndroidMkEntries() []android.AndroidMkEntries {
	if a.hideApexVariantFromMake {
		return []android.AndroidMkEntries{{
			Disabled: true,
		}}
	}
	entriesList := a.Library.AndroidMkEntries()
	entries := &entriesList[0]

	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
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
		entries.SetPath("LOCAL_SOONG_TRANSITIVE_RES_PACKAGES", a.transitiveAaptResourcePackagesFile)
		entries.SetPath("LOCAL_SOONG_STATIC_LIBRARY_EXTRA_PACKAGES", a.extraAaptPackagesFile)
		entries.SetPath("LOCAL_FULL_MANIFEST_FILE", a.mergedManifestFile)
		entries.SetPath("LOCAL_SOONG_EXPORT_PROGUARD_FLAGS", a.combinedExportedProguardFlagsFile)
		entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", true)
		android.SetAconfigFileMkEntries(&a.ModuleBase, entries, a.mergedAconfigFiles)
	})

	return entriesList
}

func (jd *Javadoc) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(jd.stubsSrcJar),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				if BoolDefault(jd.properties.Installable, true) {
					entries.SetPath("LOCAL_DROIDDOC_DOC_ZIP", jd.docZip)
				}
				if jd.exportableStubsSrcJar != nil {
					entries.SetPath("LOCAL_DROIDDOC_STUBS_SRCJAR", jd.exportableStubsSrcJar)
				}
			},
		},
	}}
}

func (ddoc *Droiddoc) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(ddoc.Javadoc.docZip),
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				if ddoc.Javadoc.docZip != nil {
					entries.SetPath("LOCAL_DROIDDOC_DOC_ZIP", ddoc.Javadoc.docZip)
				}
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !BoolDefault(ddoc.Javadoc.properties.Installable, true))
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
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: outputFile,
		Include:    "$(BUILD_SYSTEM)/soong_droiddoc_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				if dstubs.Javadoc.exportableStubsSrcJar != nil {
					entries.SetPath("LOCAL_DROIDDOC_STUBS_SRCJAR", dstubs.Javadoc.exportableStubsSrcJar)
				}
				if dstubs.everythingArtifacts.apiVersionsXml != nil {
					entries.SetPath("LOCAL_DROIDDOC_API_VERSIONS_XML", dstubs.exportableArtifacts.apiVersionsXml)
				}
				if dstubs.everythingArtifacts.annotationsZip != nil {
					entries.SetPath("LOCAL_DROIDDOC_ANNOTATIONS_ZIP", dstubs.exportableArtifacts.annotationsZip)
				}
				if dstubs.everythingArtifacts.metadataZip != nil {
					entries.SetPath("LOCAL_DROIDDOC_METADATA_ZIP", dstubs.exportableArtifacts.metadataZip)
				}
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string) {
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
						fmt.Fprintf(w, "$(call declare-0p-target,%s)\n", dstubs.apiLintReport.String())
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
	if a.hideApexVariantFromMake {
		// The non-platform variant is placed inside APEX. No reason to
		// make it available to Make.
		return nil
	}
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:        "APPS",
		OutputFile:   android.OptionalPathForPath(a.outputFile),
		OverrideName: a.BaseModuleName(), // TODO (spandandas): Add a test
		Include:      "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", a.Privileged())
				entries.SetString("LOCAL_CERTIFICATE", a.certificate.AndroidMkString())
				entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", a.properties.Overrides...)
				if len(a.dexpreopter.builtInstalled) > 0 {
					entries.SetString("LOCAL_SOONG_BUILT_INSTALLED", a.dexpreopter.builtInstalled)
				}
				entries.AddStrings("LOCAL_INSTALLED_MODULE_STEM", a.installPath.Rel())
				if Bool(a.properties.Export_package_resources) {
					entries.SetPath("LOCAL_SOONG_RESOURCE_EXPORT_PACKAGE", a.outputFile)
				}
				// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts
			},
		},
	}}
}

func (a *AndroidTestImport) AndroidMkEntries() []android.AndroidMkEntries {
	entriesList := a.AndroidAppImport.AndroidMkEntries()
	entries := &entriesList[0]
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		testSuiteComponent(entries, a.testProperties.Test_suites, Bool(a.testProperties.Per_testcase_directory))
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
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_CERTIFICATE", r.certificate.AndroidMkString())
				entries.SetPath("LOCAL_MODULE_PATH", r.installDir)
				entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", r.properties.Overrides...)
				// TODO: LOCAL_ACONFIG_FILES -- Might eventually need aconfig flags?
			},
		},
	}}
}

func (apkSet *AndroidAppSet) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{
		android.AndroidMkEntries{
			Class:      "APPS",
			OutputFile: android.OptionalPathForPath(apkSet.primaryOutput),
			Include:    "$(BUILD_SYSTEM)/soong_android_app_set.mk",
			ExtraEntries: []android.AndroidMkExtraEntriesFunc{
				func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
					entries.SetBoolIfTrue("LOCAL_PRIVILEGED_MODULE", apkSet.Privileged())
					entries.SetPath("LOCAL_APK_SET_INSTALL_FILE", apkSet.PackedAdditionalOutputs())
					entries.SetPath("LOCAL_APKCERTS_FILE", apkSet.apkcertsFile)
					entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", apkSet.properties.Overrides...)
					// TODO(b/289117800): LOCAL_ACONFIG_FILES for prebuilts -- Both declarations and values
				},
			},
		},
	}
}

func (al *ApiLibrary) AndroidMkEntries() []android.AndroidMkEntries {
	var entriesList []android.AndroidMkEntries

	entriesList = append(entriesList, android.AndroidMkEntries{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(al.stubsJar),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", true)
				entries.SetPath("LOCAL_SOONG_CLASSES_JAR", al.stubsJar)
				entries.SetPath("LOCAL_SOONG_HEADER_JAR", al.stubsJar)
			},
		},
	})

	return entriesList
}
