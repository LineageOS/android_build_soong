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

	"github.com/google/blueprint/proptools"
)

func (a *apexBundle) AndroidMk() android.AndroidMkData {
	if a.properties.HideFromMake {
		return android.AndroidMkData{
			Disabled: true,
		}
	}
	writers := []android.AndroidMkData{}
	writers = append(writers, a.androidMkForType())
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			for _, data := range writers {
				data.Custom(w, name, prefix, moduleDir, data)
			}
		}}
}

func (a *apexBundle) androidMkForFiles(w io.Writer, apexName, moduleDir string) []string {
	moduleNames := []string{}
	apexType := a.properties.ApexType
	// To avoid creating duplicate build rules, run this function only when primaryApexType is true
	// to install symbol files in $(PRODUCT_OUT}/apex.
	// And if apexType is flattened, run this function to install files in $(PRODUCT_OUT}/system/apex.
	if !a.primaryApexType && apexType != flattenedApex {
		return moduleNames
	}

	for _, fi := range a.filesInfo {
		if cc, ok := fi.module.(*cc.Module); ok && cc.Properties.HideFromMake {
			continue
		}

		if !android.InList(fi.moduleName, moduleNames) {
			moduleNames = append(moduleNames, fi.moduleName)
		}

		fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
		fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
		fmt.Fprintln(w, "LOCAL_MODULE :=", fi.moduleName)
		// /apex/<apex_name>/{lib|framework|...}
		pathWhenActivated := filepath.Join("$(PRODUCT_OUT)", "apex", apexName, fi.installDir)
		if apexType == flattenedApex {
			// /system/apex/<name>/{lib|framework|...}
			fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", filepath.Join(a.installDir.ToMakePath().String(),
				apexName, fi.installDir))
			if a.primaryApexType {
				fmt.Fprintln(w, "LOCAL_SOONG_SYMBOL_PATH :=", pathWhenActivated)
			}
			if len(fi.symlinks) > 0 {
				fmt.Fprintln(w, "LOCAL_MODULE_SYMLINKS :=", strings.Join(fi.symlinks, " "))
			}

			if fi.module != nil && fi.module.NoticeFile().Valid() {
				fmt.Fprintln(w, "LOCAL_NOTICE_FILE :=", fi.module.NoticeFile().Path().String())
			}
		} else {
			fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", pathWhenActivated)
		}
		fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", fi.builtFile.String())
		fmt.Fprintln(w, "LOCAL_MODULE_CLASS :=", fi.class.NameInMake())
		if fi.module != nil {
			archStr := fi.module.Target().Arch.ArchType.String()
			host := false
			switch fi.module.Target().Os.Class {
			case android.Host:
				if fi.module.Target().Arch.ArchType != android.Common {
					fmt.Fprintln(w, "LOCAL_MODULE_HOST_ARCH :=", archStr)
				}
				host = true
			case android.HostCross:
				if fi.module.Target().Arch.ArchType != android.Common {
					fmt.Fprintln(w, "LOCAL_MODULE_HOST_CROSS_ARCH :=", archStr)
				}
				host = true
			case android.Device:
				if fi.module.Target().Arch.ArchType != android.Common {
					fmt.Fprintln(w, "LOCAL_MODULE_TARGET_ARCH :=", archStr)
				}
			}
			if host {
				makeOs := fi.module.Target().Os.String()
				if fi.module.Target().Os == android.Linux || fi.module.Target().Os == android.LinuxBionic {
					makeOs = "linux"
				}
				fmt.Fprintln(w, "LOCAL_MODULE_HOST_OS :=", makeOs)
				fmt.Fprintln(w, "LOCAL_IS_HOST_MODULE := true")
			}
		}
		if fi.class == javaSharedLib {
			javaModule := fi.module.(*java.Library)
			// soong_java_prebuilt.mk sets LOCAL_MODULE_SUFFIX := .jar  Therefore
			// we need to remove the suffix from LOCAL_MODULE_STEM, otherwise
			// we will have foo.jar.jar
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", strings.TrimSuffix(fi.builtFile.Base(), ".jar"))
			fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", javaModule.ImplementationAndResourcesJars()[0].String())
			fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", javaModule.HeaderJars()[0].String())
			fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", fi.builtFile.String())
			fmt.Fprintln(w, "LOCAL_DEX_PREOPT := false")
			fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_java_prebuilt.mk")
		} else if fi.class == nativeSharedLib || fi.class == nativeExecutable || fi.class == nativeTest {
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", fi.builtFile.Base())
			if cc, ok := fi.module.(*cc.Module); ok {
				if cc.UnstrippedOutputFile() != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_UNSTRIPPED_BINARY :=", cc.UnstrippedOutputFile().String())
				}
				cc.AndroidMkWriteAdditionalDependenciesForSourceAbiDiff(w)
				if cc.CoverageOutputFile().Valid() {
					fmt.Fprintln(w, "LOCAL_PREBUILT_COVERAGE_ARCHIVE :=", cc.CoverageOutputFile().String())
				}
			}
			fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_cc_prebuilt.mk")
		} else {
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", fi.builtFile.Base())
			// For flattened apexes, compat symlinks are attached to apex_manifest.json which is guaranteed for every apex
			if a.primaryApexType && fi.builtFile == a.manifestPbOut && len(a.compatSymlinks) > 0 {
				fmt.Fprintln(w, "LOCAL_POST_INSTALL_CMD :=", strings.Join(a.compatSymlinks, " && "))
			}
			fmt.Fprintln(w, "include $(BUILD_PREBUILT)")
		}
	}
	return moduleNames
}

func (a *apexBundle) androidMkForType() android.AndroidMkData {
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			moduleNames := []string{}
			apexType := a.properties.ApexType
			if a.installable() {
				apexName := proptools.StringDefault(a.properties.Apex_name, name)
				moduleNames = a.androidMkForFiles(w, apexName, moduleDir)
			}

			if apexType == flattenedApex {
				// Only image APEXes can be flattened.
				fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
				fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
				fmt.Fprintln(w, "LOCAL_MODULE :=", name+a.suffix)
				if len(moduleNames) > 0 {
					fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES :=", strings.Join(moduleNames, " "))
				}
				fmt.Fprintln(w, "include $(BUILD_PHONY_PACKAGE)")
				fmt.Fprintln(w, "$(LOCAL_INSTALLED_MODULE): .KATI_IMPLICIT_OUTPUTS :=", a.outputFile.String())

			} else {
				fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
				fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
				fmt.Fprintln(w, "LOCAL_MODULE :=", name+a.suffix)
				fmt.Fprintln(w, "LOCAL_MODULE_CLASS := ETC") // do we need a new class?
				fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", a.outputFile.String())
				fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", a.installDir.ToMakePath().String())
				fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", name+apexType.suffix())
				fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE :=", !a.installable())
				fmt.Fprintln(w, "LOCAL_OVERRIDES_MODULES :=", strings.Join(a.properties.Overrides, " "))
				if len(moduleNames) > 0 {
					fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES +=", strings.Join(moduleNames, " "))
				}
				if len(a.externalDeps) > 0 {
					fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES +=", strings.Join(a.externalDeps, " "))
				}
				var postInstallCommands []string
				if a.prebuiltFileToDelete != "" {
					postInstallCommands = append(postInstallCommands, "rm -rf "+
						filepath.Join(a.installDir.ToMakePath().String(), a.prebuiltFileToDelete))
				}
				// For unflattened apexes, compat symlinks are attached to apex package itself as LOCAL_POST_INSTALL_CMD
				postInstallCommands = append(postInstallCommands, a.compatSymlinks...)
				if len(postInstallCommands) > 0 {
					fmt.Fprintln(w, "LOCAL_POST_INSTALL_CMD :=", strings.Join(postInstallCommands, " && "))
				}
				fmt.Fprintln(w, "include $(BUILD_PREBUILT)")

				if apexType == imageApex {
					fmt.Fprintln(w, "ALL_MODULES.$(LOCAL_MODULE).BUNDLE :=", a.bundleModuleFile.String())
				}
			}
		}}
}
