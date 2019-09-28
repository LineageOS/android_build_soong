// Copyright 2019 Google Inc. All rights reserved.
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

func TestConfig(buildDir string, env map[string]string) android.Config {
	if env == nil {
		env = make(map[string]string)
	}
	if env["ANDROID_JAVA8_HOME"] == "" {
		env["ANDROID_JAVA8_HOME"] = "jdk8"
	}
	config := android.TestArchConfig(buildDir, env)

	return config
}

func GatherRequiredDepsForTest() string {
	var bp string

	extraModules := []string{
		"core-lambda-stubs",
		"ext",
		"updatable_media_stubs",
		"android_stubs_current",
		"android_system_stubs_current",
		"android_test_stubs_current",
		"core.current.stubs",
		"core.platform.api.stubs",
		"kotlin-stdlib",
		"kotlin-stdlib-jdk7",
		"kotlin-stdlib-jdk8",
		"kotlin-annotations",
	}

	for _, extra := range extraModules {
		bp += fmt.Sprintf(`
			java_library {
				name: "%s",
				srcs: ["a.java"],
				sdk_version: "none",
				system_modules: "core-platform-api-stubs-system-modules",
			}
		`, extra)
	}

	bp += `
		java_library {
			name: "framework",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "core-platform-api-stubs-system-modules",
			aidl: {
				export_include_dirs: ["framework/aidl"],
			},
		}

		android_app {
			name: "framework-res",
			sdk_version: "core_platform",
		}

		java_library {
			name: "android.hidl.base-V1.0-java",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "core-platform-api-stubs-system-modules",
			installable: true,
		}

		java_library {
			name: "android.hidl.manager-V1.0-java",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "core-platform-api-stubs-system-modules",
			installable: true,
		}

		java_library {
			name: "org.apache.http.legacy",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "core-platform-api-stubs-system-modules",
			installable: true,
		}
	`

	systemModules := []string{
		"core-current-stubs-system-modules",
		"core-platform-api-stubs-system-modules",
		"android_stubs_current_system_modules",
		"android_system_stubs_current_system_modules",
		"android_test_stubs_current_system_modules",
	}

	for _, extra := range systemModules {
		bp += fmt.Sprintf(`
			java_system_modules {
				name: "%[1]s",
				libs: ["%[1]s-lib"],
			}
			java_library {
				name: "%[1]s-lib",
				sdk_version: "none",
				system_modules: "none",
			}
		`, extra)
	}

	return bp
}
