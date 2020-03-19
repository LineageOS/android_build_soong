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
	"android/soong/cc"
)

func TestConfig(buildDir string, env map[string]string, bp string, fs map[string][]byte) android.Config {
	bp += GatherRequiredDepsForTest()

	mockFS := map[string][]byte{
		"a.java":                 nil,
		"b.java":                 nil,
		"c.java":                 nil,
		"b.kt":                   nil,
		"a.jar":                  nil,
		"b.jar":                  nil,
		"c.jar":                  nil,
		"APP_NOTICE":             nil,
		"GENRULE_NOTICE":         nil,
		"LIB_NOTICE":             nil,
		"TOOL_NOTICE":            nil,
		"AndroidTest.xml":        nil,
		"java-res/a/a":           nil,
		"java-res/b/b":           nil,
		"java-res2/a":            nil,
		"java-fg/a.java":         nil,
		"java-fg/b.java":         nil,
		"java-fg/c.java":         nil,
		"api/current.txt":        nil,
		"api/removed.txt":        nil,
		"api/system-current.txt": nil,
		"api/system-removed.txt": nil,
		"api/test-current.txt":   nil,
		"api/test-removed.txt":   nil,
		"framework/aidl/a.aidl":  nil,
		"assets_a/a":             nil,
		"assets_b/b":             nil,

		"prebuilts/sdk/14/public/android.jar":         nil,
		"prebuilts/sdk/14/public/framework.aidl":      nil,
		"prebuilts/sdk/14/system/android.jar":         nil,
		"prebuilts/sdk/17/public/android.jar":         nil,
		"prebuilts/sdk/17/public/framework.aidl":      nil,
		"prebuilts/sdk/17/system/android.jar":         nil,
		"prebuilts/sdk/29/public/android.jar":         nil,
		"prebuilts/sdk/29/public/framework.aidl":      nil,
		"prebuilts/sdk/29/system/android.jar":         nil,
		"prebuilts/sdk/29/system/foo.jar":             nil,
		"prebuilts/sdk/current/core/android.jar":      nil,
		"prebuilts/sdk/current/public/android.jar":    nil,
		"prebuilts/sdk/current/public/framework.aidl": nil,
		"prebuilts/sdk/current/public/core.jar":       nil,
		"prebuilts/sdk/current/system/android.jar":    nil,
		"prebuilts/sdk/current/test/android.jar":      nil,
		"prebuilts/sdk/28/public/api/foo.txt":         nil,
		"prebuilts/sdk/28/system/api/foo.txt":         nil,
		"prebuilts/sdk/28/test/api/foo.txt":           nil,
		"prebuilts/sdk/28/public/api/foo-removed.txt": nil,
		"prebuilts/sdk/28/system/api/foo-removed.txt": nil,
		"prebuilts/sdk/28/test/api/foo-removed.txt":   nil,
		"prebuilts/sdk/28/public/api/bar.txt":         nil,
		"prebuilts/sdk/28/system/api/bar.txt":         nil,
		"prebuilts/sdk/28/test/api/bar.txt":           nil,
		"prebuilts/sdk/28/public/api/bar-removed.txt": nil,
		"prebuilts/sdk/28/system/api/bar-removed.txt": nil,
		"prebuilts/sdk/28/test/api/bar-removed.txt":   nil,
		"prebuilts/sdk/tools/core-lambda-stubs.jar":   nil,
		"prebuilts/sdk/Android.bp":                    []byte(`prebuilt_apis { name: "sdk", api_dirs: ["14", "28", "current"],}`),

		"prebuilts/apk/app.apk":        nil,
		"prebuilts/apk/app_arm.apk":    nil,
		"prebuilts/apk/app_arm64.apk":  nil,
		"prebuilts/apk/app_xhdpi.apk":  nil,
		"prebuilts/apk/app_xxhdpi.apk": nil,

		// For framework-res, which is an implicit dependency for framework
		"AndroidManifest.xml":                        nil,
		"build/make/target/product/security/testkey": nil,

		"build/soong/scripts/jar-wrapper.sh": nil,

		"build/make/core/verify_uses_libraries.sh": nil,

		"build/make/core/proguard.flags":             nil,
		"build/make/core/proguard_basic_keeps.flags": nil,

		"jdk8/jre/lib/jce.jar": nil,
		"jdk8/jre/lib/rt.jar":  nil,
		"jdk8/lib/tools.jar":   nil,

		"bar-doc/a.java":                 nil,
		"bar-doc/b.java":                 nil,
		"bar-doc/IFoo.aidl":              nil,
		"bar-doc/IBar.aidl":              nil,
		"bar-doc/known_oj_tags.txt":      nil,
		"external/doclava/templates-sdk": nil,

		"cert/new_cert.x509.pem": nil,
		"cert/new_cert.pk8":      nil,

		"testdata/data": nil,

		"stubs-sources/foo/Foo.java": nil,
		"stubs/sources/foo/Foo.java": nil,
	}

	cc.GatherRequiredFilesForTest(mockFS)

	for k, v := range fs {
		mockFS[k] = v
	}

	if env == nil {
		env = make(map[string]string)
	}
	if env["ANDROID_JAVA8_HOME"] == "" {
		env["ANDROID_JAVA8_HOME"] = "jdk8"
	}
	config := android.TestArchConfig(buildDir, env, bp, mockFS)

	return config
}

func GatherRequiredDepsForTest() string {
	var bp string

	extraModules := []string{
		"core-lambda-stubs",
		"ext",
		"android_stubs_current",
		"android_system_stubs_current",
		"android_test_stubs_current",
		"android_module_lib_stubs_current",
		"android_system_server_stubs_current",
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
