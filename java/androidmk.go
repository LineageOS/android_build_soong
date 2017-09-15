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

func (library *Library) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(library.classpathFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				if library.properties.Installable != nil && *library.properties.Installable == false {
					fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
				}
				if library.dexJarFile != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", library.dexJarFile.String())
				}
				fmt.Fprintln(w, "LOCAL_SDK_VERSION :=", library.deviceProperties.Sdk_version)
			},
		},
	}
}

func (prebuilt *Import) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(prebuilt.combinedClasspathFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
			},
		},
	}
}

func (binary *Binary) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "JAVA_LIBRARIES",
		OutputFile: android.OptionalPathForPath(binary.classpathFile),
		Include:    "$(BUILD_SYSTEM)/soong_java_prebuilt.mk",
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			android.WriteAndroidMkData(w, data)

			fmt.Fprintln(w, "jar_installed_module := $(LOCAL_INSTALLED_MODULE)")
			fmt.Fprintln(w, "include $(CLEAR_VARS)")
			fmt.Fprintln(w, "LOCAL_MODULE := "+name)
			fmt.Fprintln(w, "LOCAL_MODULE_CLASS := EXECUTABLES")
			if strings.Contains(prefix, "HOST_") {
				fmt.Fprintln(w, "LOCAL_IS_HOST_MODULE := true")
			}
			fmt.Fprintln(w, "LOCAL_STRIP_MODULE := false")
			fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", binary.wrapperFile.String())
			fmt.Fprintln(w, "include $(BUILD_PREBUILT)")

			// Ensure that the wrapper script timestamp is always updated when the jar is updated
			fmt.Fprintln(w, "$(LOCAL_INSTALLED_MODULE): $(jar_installed_module)")
			fmt.Fprintln(w, "jar_installed_module :=")
		},
	}
}
