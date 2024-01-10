// Copyright (C) 2019 The Android Open Source Project
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

package apex

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
	"android/soong/rust"
)

func (a *apexBundle) AndroidMk() android.AndroidMkData {
	if a.properties.HideFromMake {
		return android.AndroidMkData{
			Disabled: true,
		}
	}
	return a.androidMkForType()
}

// nameInMake converts apexFileClass into the corresponding class name in Make.
func (class apexFileClass) nameInMake() string {
	switch class {
	case etc:
		return "ETC"
	case nativeSharedLib:
		return "SHARED_LIBRARIES"
	case nativeExecutable, shBinary:
		return "EXECUTABLES"
	case javaSharedLib:
		return "JAVA_LIBRARIES"
	case nativeTest:
		return "NATIVE_TESTS"
	case app, appSet:
		// b/142537672 Why isn't this APP? We want to have full control over
		// the paths and file names of the apk file under the flattend APEX.
		// If this is set to APP, then the paths and file names are modified
		// by the Make build system. For example, it is installed to
		// /system/apex/<apexname>/app/<Appname>/<apexname>.<Appname>/ instead of
		// /system/apex/<apexname>/app/<Appname> because the build system automatically
		// appends module name (which is <apexname>.<Appname> to the path.
		return "ETC"
	default:
		panic(fmt.Errorf("unknown class %d", class))
	}
}

// Return the full module name for a dependency module, which appends the apex module name unless re-using a system lib.
func (a *apexBundle) fullModuleName(apexBundleName string, linkToSystemLib bool, fi *apexFile) string {
	if linkToSystemLib {
		return fi.androidMkModuleName
	}
	return fi.androidMkModuleName + "." + apexBundleName
}

// androidMkForFiles generates Make definitions for the contents of an
// apexBundle (apexBundle#filesInfo).  The filesInfo structure can either be
// populated by Soong for unconverted APEXes, or Bazel in mixed mode. Use
// apexFile#isBazelPrebuilt to differentiate.
func (a *apexBundle) androidMkForFiles(w io.Writer, apexBundleName, moduleDir string,
	apexAndroidMkData android.AndroidMkData) []string {

	// apexBundleName comes from the 'name' property or soong module.
	// apexName comes from 'name' property of apex_manifest.
	// An apex is installed to /system/apex/<apexBundleName> and is activated at /apex/<apexName>
	// In many cases, the two names are the same, but could be different in general.
	// However, symbol files for apex files are installed under /apex/<apexBundleName> to avoid
	// conflicts between two apexes with the same apexName.

	moduleNames := []string{}

	for _, fi := range a.filesInfo {
		linkToSystemLib := a.linkToSystemLib && fi.transitiveDep && fi.availableToPlatform()
		moduleName := a.fullModuleName(apexBundleName, linkToSystemLib, &fi)

		// This name will be added to LOCAL_REQUIRED_MODULES of the APEX. We need to be
		// arch-specific otherwise we will end up installing both ABIs even when only
		// either of the ABI is requested.
		aName := moduleName
		switch fi.multilib {
		case "lib32":
			aName = aName + ":32"
		case "lib64":
			aName = aName + ":64"
		}
		if !android.InList(aName, moduleNames) {
			moduleNames = append(moduleNames, aName)
		}

		if linkToSystemLib {
			// No need to copy the file since it's linked to the system file
			continue
		}

		fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)  # apex.apexBundle.files")
		if fi.moduleDir != "" {
			fmt.Fprintln(w, "LOCAL_PATH :=", fi.moduleDir)
		} else {
			fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
		}
		fmt.Fprintln(w, "LOCAL_MODULE :=", moduleName)

		if fi.module != nil && fi.module.Owner() != "" {
			fmt.Fprintln(w, "LOCAL_MODULE_OWNER :=", fi.module.Owner())
		}
		// /apex/<apexBundleName>/{lib|framework|...}
		pathForSymbol := filepath.Join("$(PRODUCT_OUT)", "apex", apexBundleName, fi.installDir)
		modulePath := pathForSymbol
		fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", modulePath)
		// AconfigUpdateAndroidMkData may have added elements to Extra.  Process them here.
		for _, extra := range apexAndroidMkData.Extra {
			extra(w, fi.builtFile)
		}

		// For non-flattend APEXes, the merged notice file is attached to the APEX itself.
		// We don't need to have notice file for the individual modules in it. Otherwise,
		// we will have duplicated notice entries.
		fmt.Fprintln(w, "LOCAL_NO_NOTICE_FILE := true")
		fmt.Fprintln(w, "LOCAL_SOONG_INSTALLED_MODULE :=", filepath.Join(modulePath, fi.stem()))
		fmt.Fprintln(w, "LOCAL_SOONG_INSTALL_PAIRS :=", fi.builtFile.String()+":"+filepath.Join(modulePath, fi.stem()))
		fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", fi.builtFile.String())
		fmt.Fprintln(w, "LOCAL_MODULE_CLASS :=", fi.class.nameInMake())
		if fi.module != nil {
			// This apexFile's module comes from Soong
			if fi.module.Target().Arch.ArchType != android.Common {
				archStr := fi.module.Target().Arch.ArchType.String()
				fmt.Fprintln(w, "LOCAL_MODULE_TARGET_ARCH :=", archStr)
			}
		}
		if fi.jacocoReportClassesFile != nil {
			fmt.Fprintln(w, "LOCAL_SOONG_JACOCO_REPORT_CLASSES_JAR :=", fi.jacocoReportClassesFile.String())
		}
		switch fi.class {
		case javaSharedLib:
			// soong_java_prebuilt.mk sets LOCAL_MODULE_SUFFIX := .jar  Therefore
			// we need to remove the suffix from LOCAL_MODULE_STEM, otherwise
			// we will have foo.jar.jar
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", strings.TrimSuffix(fi.stem(), ".jar"))
			if javaModule, ok := fi.module.(java.ApexDependency); ok {
				fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", javaModule.ImplementationAndResourcesJars()[0].String())
				fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", javaModule.HeaderJars()[0].String())
			} else {
				fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", fi.builtFile.String())
				fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", fi.builtFile.String())
			}
			fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", fi.builtFile.String())
			fmt.Fprintln(w, "LOCAL_DEX_PREOPT := false")
			fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_java_prebuilt.mk")
		case app:
			fmt.Fprintln(w, "LOCAL_CERTIFICATE :=", fi.certificate.AndroidMkString())
			// soong_app_prebuilt.mk sets LOCAL_MODULE_SUFFIX := .apk  Therefore
			// we need to remove the suffix from LOCAL_MODULE_STEM, otherwise
			// we will have foo.apk.apk
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", strings.TrimSuffix(fi.stem(), ".apk"))
			if app, ok := fi.module.(*java.AndroidApp); ok {
				android.AndroidMkEmitAssignList(w, "LOCAL_PREBUILT_COVERAGE_ARCHIVE", app.JniCoverageOutputs().Strings())
				if jniLibSymbols := app.JNISymbolsInstalls(modulePath); len(jniLibSymbols) > 0 {
					fmt.Fprintln(w, "LOCAL_SOONG_JNI_LIBS_SYMBOLS :=", jniLibSymbols.String())
				}
			}
			fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_app_prebuilt.mk")
		case appSet:
			as, ok := fi.module.(*java.AndroidAppSet)
			if !ok {
				panic(fmt.Sprintf("Expected %s to be AndroidAppSet", fi.module))
			}
			fmt.Fprintln(w, "LOCAL_APK_SET_INSTALL_FILE :=", as.PackedAdditionalOutputs().String())
			fmt.Fprintln(w, "LOCAL_APKCERTS_FILE :=", as.APKCertsFile().String())
			fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_android_app_set.mk")
		case nativeSharedLib, nativeExecutable, nativeTest:
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", fi.stem())
			if ccMod, ok := fi.module.(*cc.Module); ok {
				if ccMod.UnstrippedOutputFile() != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_UNSTRIPPED_BINARY :=", ccMod.UnstrippedOutputFile().String())
				}
				ccMod.AndroidMkWriteAdditionalDependenciesForSourceAbiDiff(w)
				if ccMod.CoverageOutputFile().Valid() {
					fmt.Fprintln(w, "LOCAL_PREBUILT_COVERAGE_ARCHIVE :=", ccMod.CoverageOutputFile().String())
				}
			} else if rustMod, ok := fi.module.(*rust.Module); ok {
				if rustMod.UnstrippedOutputFile() != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_UNSTRIPPED_BINARY :=", rustMod.UnstrippedOutputFile().String())
				}
			}
			fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_cc_rust_prebuilt.mk")
		default:
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", fi.stem())
			fmt.Fprintln(w, "include $(BUILD_PREBUILT)")
		}

		// m <module_name> will build <module_name>.<apex_name> as well.
		if fi.androidMkModuleName != moduleName {
			fmt.Fprintf(w, ".PHONY: %s\n", fi.androidMkModuleName)
			fmt.Fprintf(w, "%s: %s\n", fi.androidMkModuleName, moduleName)
		}
	}
	return moduleNames
}

func (a *apexBundle) writeRequiredModules(w io.Writer, moduleNames []string) {
	var required []string
	var targetRequired []string
	var hostRequired []string
	required = append(required, a.RequiredModuleNames()...)
	targetRequired = append(targetRequired, a.TargetRequiredModuleNames()...)
	hostRequired = append(hostRequired, a.HostRequiredModuleNames()...)
	for _, fi := range a.filesInfo {
		required = append(required, fi.requiredModuleNames...)
		targetRequired = append(targetRequired, fi.targetRequiredModuleNames...)
		hostRequired = append(hostRequired, fi.hostRequiredModuleNames...)
	}
	android.AndroidMkEmitAssignList(w, "LOCAL_REQUIRED_MODULES", moduleNames, a.makeModulesToInstall, required)
	android.AndroidMkEmitAssignList(w, "LOCAL_TARGET_REQUIRED_MODULES", targetRequired)
	android.AndroidMkEmitAssignList(w, "LOCAL_HOST_REQUIRED_MODULES", hostRequired)
}

func (a *apexBundle) androidMkForType() android.AndroidMkData {
	return android.AndroidMkData{
		// While we do not provide a value for `Extra`, AconfigUpdateAndroidMkData may add some, which we must honor.
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			moduleNames := []string{}
			if a.installable() {
				moduleNames = a.androidMkForFiles(w, name, moduleDir, data)
			}

			fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)  # apex.apexBundle")
			fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
			fmt.Fprintln(w, "LOCAL_MODULE :=", name)
			fmt.Fprintln(w, "LOCAL_MODULE_CLASS := ETC") // do we need a new class?
			fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", a.outputFile.String())
			fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", a.installDir.String())
			stemSuffix := imageApexSuffix
			if a.isCompressed {
				stemSuffix = imageCapexSuffix
			}
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", name+stemSuffix)
			fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE :=", !a.installable())
			if a.installable() {
				fmt.Fprintln(w, "LOCAL_SOONG_INSTALLED_MODULE :=", a.installedFile.String())
				fmt.Fprintln(w, "LOCAL_SOONG_INSTALL_PAIRS :=", a.outputFile.String()+":"+a.installedFile.String())
				fmt.Fprintln(w, "LOCAL_SOONG_INSTALL_SYMLINKS := ", strings.Join(a.compatSymlinks.Strings(), " "))
			}
			fmt.Fprintln(w, "LOCAL_APEX_KEY_PATH := ", a.apexKeysPath.String())

			// Because apex writes .mk with Custom(), we need to write manually some common properties
			// which are available via data.Entries
			commonProperties := []string{
				"LOCAL_FULL_INIT_RC", "LOCAL_FULL_VINTF_FRAGMENTS",
				"LOCAL_PROPRIETARY_MODULE", "LOCAL_VENDOR_MODULE", "LOCAL_ODM_MODULE", "LOCAL_PRODUCT_MODULE", "LOCAL_SYSTEM_EXT_MODULE",
				"LOCAL_MODULE_OWNER",
			}
			for _, name := range commonProperties {
				if value, ok := data.Entries.EntryMap[name]; ok {
					android.AndroidMkEmitAssignList(w, name, value)
				}
			}

			android.AndroidMkEmitAssignList(w, "LOCAL_OVERRIDES_MODULES", a.overridableProperties.Overrides)
			a.writeRequiredModules(w, moduleNames)
			// AconfigUpdateAndroidMkData may have added elements to Extra.  Process them here.
			for _, extra := range data.Extra {
				extra(w, a.outputFile)
			}

			fmt.Fprintln(w, "include $(BUILD_PREBUILT)")
			fmt.Fprintln(w, "ALL_MODULES.$(my_register_name).BUNDLE :=", a.bundleModuleFile.String())
			android.AndroidMkEmitAssignList(w, "ALL_MODULES.$(my_register_name).LINT_REPORTS", a.lintReports.Strings())

			if a.installedFilesFile != nil {
				goal := "checkbuild"
				distFile := name + "-installed-files.txt"
				fmt.Fprintln(w, ".PHONY:", goal)
				fmt.Fprintf(w, "$(call dist-for-goals,%s,%s:%s)\n",
					goal, a.installedFilesFile.String(), distFile)
				fmt.Fprintf(w, "$(call declare-0p-target,%s)\n", a.installedFilesFile.String())
			}
			for _, dist := range data.Entries.GetDistForGoals(a) {
				fmt.Fprintf(w, dist)
			}

			distCoverageFiles(w, "ndk_apis_usedby_apex", a.nativeApisUsedByModuleFile.String())
			distCoverageFiles(w, "ndk_apis_backedby_apex", a.nativeApisBackedByModuleFile.String())
			distCoverageFiles(w, "java_apis_used_by_apex", a.javaApisUsedByModuleFile.String())
		}}
}

func distCoverageFiles(w io.Writer, dir string, distfile string) {
	if distfile != "" {
		goal := "apps_only"
		fmt.Fprintf(w, "ifneq (,$(filter $(my_register_name),$(TARGET_BUILD_APPS)))\n"+
			" $(call dist-for-goals,%s,%s:%s/$(notdir %s))\n"+
			"endif\n", goal, distfile, dir, distfile)
	}
}
