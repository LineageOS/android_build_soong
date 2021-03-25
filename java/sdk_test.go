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
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/java/config"
)

func TestClasspath(t *testing.T) {
	const frameworkAidl = "-I" + defaultJavaDir + "/framework/aidl"
	var classpathTestcases = []struct {
		name       string
		unbundled  bool
		moduleType string
		host       android.OsClass
		properties string

		// for java 8
		bootclasspath  []string
		java8classpath []string

		// for java 9
		system         string
		java9classpath []string

		forces8 bool // if set, javac will always be called with java 8 arguments

		aidl string
	}{
		{
			name:           "default",
			bootclasspath:  config.StableCorePlatformBootclasspathLibraries,
			system:         config.StableCorePlatformSystemModules,
			java8classpath: config.FrameworkLibraries,
			java9classpath: config.FrameworkLibraries,
			aidl:           frameworkAidl,
		},
		{
			name:           `sdk_version:"core_platform"`,
			properties:     `sdk_version:"core_platform"`,
			bootclasspath:  config.StableCorePlatformBootclasspathLibraries,
			system:         config.StableCorePlatformSystemModules,
			java8classpath: []string{},
			aidl:           "",
		},
		{
			name:           "blank sdk version",
			properties:     `sdk_version: "",`,
			bootclasspath:  config.StableCorePlatformBootclasspathLibraries,
			system:         config.StableCorePlatformSystemModules,
			java8classpath: config.FrameworkLibraries,
			java9classpath: config.FrameworkLibraries,
			aidl:           frameworkAidl,
		},
		{

			name:           "sdk v29",
			properties:     `sdk_version: "29",`,
			bootclasspath:  []string{`""`},
			forces8:        true,
			java8classpath: []string{"prebuilts/sdk/29/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:           "-pprebuilts/sdk/29/public/framework.aidl",
		},
		{

			name:           "sdk v30",
			properties:     `sdk_version: "30",`,
			bootclasspath:  []string{`""`},
			system:         "sdk_public_30_system_modules",
			java8classpath: []string{"prebuilts/sdk/30/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			java9classpath: []string{"prebuilts/sdk/30/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:           "-pprebuilts/sdk/30/public/framework.aidl",
		},
		{

			name:           "current",
			properties:     `sdk_version: "current",`,
			bootclasspath:  []string{"android_stubs_current", "core-lambda-stubs"},
			system:         "core-current-stubs-system-modules",
			java9classpath: []string{"android_stubs_current"},
			aidl:           "-pout/soong/framework.aidl",
		},
		{

			name:           "system_current",
			properties:     `sdk_version: "system_current",`,
			bootclasspath:  []string{"android_system_stubs_current", "core-lambda-stubs"},
			system:         "core-current-stubs-system-modules",
			java9classpath: []string{"android_system_stubs_current"},
			aidl:           "-pout/soong/framework.aidl",
		},
		{

			name:           "system_29",
			properties:     `sdk_version: "system_29",`,
			bootclasspath:  []string{`""`},
			forces8:        true,
			java8classpath: []string{"prebuilts/sdk/29/system/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:           "-pprebuilts/sdk/29/public/framework.aidl",
		},
		{

			name:           "system_30",
			properties:     `sdk_version: "system_30",`,
			bootclasspath:  []string{`""`},
			system:         "sdk_public_30_system_modules",
			java8classpath: []string{"prebuilts/sdk/30/system/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			java9classpath: []string{"prebuilts/sdk/30/system/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:           "-pprebuilts/sdk/30/public/framework.aidl",
		},
		{

			name:           "test_current",
			properties:     `sdk_version: "test_current",`,
			bootclasspath:  []string{"android_test_stubs_current", "core-lambda-stubs"},
			system:         "core-current-stubs-system-modules",
			java9classpath: []string{"android_test_stubs_current"},
			aidl:           "-pout/soong/framework.aidl",
		},
		{

			name:          "core_current",
			properties:    `sdk_version: "core_current",`,
			bootclasspath: []string{"core.current.stubs", "core-lambda-stubs"},
			system:        "core-current-stubs-system-modules",
		},
		{

			name:           "nostdlib",
			properties:     `sdk_version: "none", system_modules: "none"`,
			system:         "none",
			bootclasspath:  []string{`""`},
			java8classpath: []string{},
		},
		{

			name:           "nostdlib system_modules",
			properties:     `sdk_version: "none", system_modules: "stable-core-platform-api-stubs-system-modules"`,
			system:         "stable-core-platform-api-stubs-system-modules",
			bootclasspath:  []string{"stable-core-platform-api-stubs-system-modules-lib"},
			java8classpath: []string{},
		},
		{

			name:           "host default",
			moduleType:     "java_library_host",
			properties:     ``,
			host:           android.Host,
			bootclasspath:  []string{"jdk8/jre/lib/jce.jar", "jdk8/jre/lib/rt.jar"},
			java8classpath: []string{},
		},
		{

			name:           "host supported default",
			host:           android.Host,
			properties:     `host_supported: true,`,
			java8classpath: []string{},
			bootclasspath:  []string{"jdk8/jre/lib/jce.jar", "jdk8/jre/lib/rt.jar"},
		},
		{
			name:           "host supported nostdlib",
			host:           android.Host,
			properties:     `host_supported: true, sdk_version: "none", system_modules: "none"`,
			java8classpath: []string{},
		},
		{

			name:           "unbundled sdk v29",
			unbundled:      true,
			properties:     `sdk_version: "29",`,
			bootclasspath:  []string{`""`},
			forces8:        true,
			java8classpath: []string{"prebuilts/sdk/29/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:           "-pprebuilts/sdk/29/public/framework.aidl",
		},
		{

			name:           "unbundled sdk v30",
			unbundled:      true,
			properties:     `sdk_version: "30",`,
			bootclasspath:  []string{`""`},
			system:         "sdk_public_30_system_modules",
			java8classpath: []string{"prebuilts/sdk/30/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			java9classpath: []string{"prebuilts/sdk/30/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:           "-pprebuilts/sdk/30/public/framework.aidl",
		},
		{

			name:           "unbundled current",
			unbundled:      true,
			properties:     `sdk_version: "current",`,
			bootclasspath:  []string{`""`},
			system:         "sdk_public_current_system_modules",
			java8classpath: []string{"prebuilts/sdk/current/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			java9classpath: []string{"prebuilts/sdk/current/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:           "-pprebuilts/sdk/current/public/framework.aidl",
		},

		{
			name:           "module_current",
			properties:     `sdk_version: "module_current",`,
			bootclasspath:  []string{"android_module_lib_stubs_current", "core-lambda-stubs"},
			system:         "core-current-stubs-system-modules",
			java9classpath: []string{"android_module_lib_stubs_current"},
			aidl:           "-pout/soong/framework_non_updatable.aidl",
		},
		{
			name:           "system_server_current",
			properties:     `sdk_version: "system_server_current",`,
			bootclasspath:  []string{"android_system_server_stubs_current", "core-lambda-stubs"},
			system:         "core-current-stubs-system-modules",
			java9classpath: []string{"android_system_server_stubs_current"},
			aidl:           "-pout/soong/framework.aidl",
		},
	}

	for _, testcase := range classpathTestcases {
		t.Run(testcase.name, func(t *testing.T) {
			moduleType := "java_library"
			if testcase.moduleType != "" {
				moduleType = testcase.moduleType
			}

			props := `
				name: "foo",
				srcs: ["a.java"],
				target: {
					android: {
						srcs: ["bar-doc/IFoo.aidl"],
					},
				},
				`
			bp := moduleType + " {" + props + testcase.properties + `
			}`
			bpJava8 := moduleType + " {" + props + `java_version: "1.8",
				` + testcase.properties + `
			}`

			variant := "android_common"
			if testcase.host == android.Host {
				variant = android.BuildOs.String() + "_common"
			}

			convertModulesToPaths := func(cp []string) []string {
				ret := make([]string, len(cp))
				for i, e := range cp {
					ret[i] = defaultModuleToPath(e)
				}
				return ret
			}

			bootclasspath := convertModulesToPaths(testcase.bootclasspath)
			java8classpath := convertModulesToPaths(testcase.java8classpath)
			java9classpath := convertModulesToPaths(testcase.java9classpath)

			bc := ""
			var bcDeps []string
			if len(bootclasspath) > 0 {
				bc = "-bootclasspath " + strings.Join(bootclasspath, ":")
				if bootclasspath[0] != `""` {
					bcDeps = bootclasspath
				}
			}

			j8c := ""
			if len(java8classpath) > 0 {
				j8c = "-classpath " + strings.Join(java8classpath, ":")
			}

			j9c := ""
			if len(java9classpath) > 0 {
				j9c = "-classpath " + strings.Join(java9classpath, ":")
			}

			system := ""
			var systemDeps []string
			if testcase.system == "none" {
				system = "--system=none"
			} else if testcase.system != "" {
				dir := ""
				if strings.HasPrefix(testcase.system, "sdk_public_") {
					dir = "prebuilts/sdk"
				} else {
					dir = defaultJavaDir
				}
				system = "--system=" + filepath.Join("out", "soong", ".intermediates", dir, testcase.system, "android_common", "system")
				// The module-relative parts of these paths are hardcoded in system_modules.go:
				systemDeps = []string{
					filepath.Join("out", "soong", ".intermediates", dir, testcase.system, "android_common", "system", "lib", "modules"),
					filepath.Join("out", "soong", ".intermediates", dir, testcase.system, "android_common", "system", "lib", "jrt-fs.jar"),
					filepath.Join("out", "soong", ".intermediates", dir, testcase.system, "android_common", "system", "release"),
				}
			}

			checkClasspath := func(t *testing.T, result *android.TestResult, isJava8 bool) {
				foo := result.ModuleForTests("foo", variant)
				javac := foo.Rule("javac").RelativeToTop()
				var deps []string

				aidl := foo.MaybeRule("aidl").RelativeToTop()
				if aidl.Rule != nil {
					deps = append(deps, android.PathRelativeToTop(aidl.Output))
				}

				got := javac.Args["bootClasspath"]
				expected := ""
				if isJava8 || testcase.forces8 {
					expected = bc
					deps = append(deps, bcDeps...)
				} else {
					expected = system
					deps = append(deps, systemDeps...)
				}
				if got != expected {
					t.Errorf("bootclasspath expected %q != got %q", expected, got)
				}

				if isJava8 || testcase.forces8 {
					expected = j8c
					deps = append(deps, java8classpath...)
				} else {
					expected = j9c
					deps = append(deps, java9classpath...)
				}
				got = javac.Args["classpath"]
				if got != expected {
					t.Errorf("classpath expected %q != got %q", expected, got)
				}

				android.AssertPathsRelativeToTopEquals(t, "implicits", deps, javac.Implicits)
			}

			fixtureFactory := android.GroupFixturePreparers(
				prepareForJavaTest,
				FixtureWithPrebuiltApis(map[string][]string{
					"29":      {},
					"30":      {},
					"current": {},
				}),
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					if testcase.unbundled {
						variables.Unbundled_build = proptools.BoolPtr(true)
						variables.Always_use_prebuilt_sdks = proptools.BoolPtr(true)
					}
				}),
				android.FixtureModifyEnv(func(env map[string]string) {
					if env["ANDROID_JAVA8_HOME"] == "" {
						env["ANDROID_JAVA8_HOME"] = "jdk8"
					}
				}),
			)

			// Test with legacy javac -source 1.8 -target 1.8
			t.Run("Java language level 8", func(t *testing.T) {
				result := fixtureFactory.RunTestWithBp(t, bpJava8)

				checkClasspath(t, result, true /* isJava8 */)

				if testcase.host != android.Host {
					aidl := result.ModuleForTests("foo", variant).Rule("aidl").RelativeToTop()

					android.AssertStringDoesContain(t, "aidl command", aidl.RuleParams.Command, testcase.aidl+" -I.")
				}
			})

			// Test with default javac -source 9 -target 9
			t.Run("Java language level 9", func(t *testing.T) {
				result := fixtureFactory.RunTestWithBp(t, bp)

				checkClasspath(t, result, false /* isJava8 */)

				if testcase.host != android.Host {
					aidl := result.ModuleForTests("foo", variant).Rule("aidl").RelativeToTop()

					android.AssertStringDoesContain(t, "aidl command", aidl.RuleParams.Command, testcase.aidl+" -I.")
				}
			})

			prepareWithPlatformVersionRel := android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.Platform_sdk_codename = proptools.StringPtr("REL")
				variables.Platform_sdk_final = proptools.BoolPtr(true)
			})

			// Test again with PLATFORM_VERSION_CODENAME=REL, javac -source 8 -target 8
			t.Run("REL + Java language level 8", func(t *testing.T) {
				result := fixtureFactory.Extend(prepareWithPlatformVersionRel).RunTestWithBp(t, bpJava8)

				checkClasspath(t, result, true /* isJava8 */)
			})

			// Test again with PLATFORM_VERSION_CODENAME=REL, javac -source 9 -target 9
			t.Run("REL + Java language level 9", func(t *testing.T) {
				result := fixtureFactory.Extend(prepareWithPlatformVersionRel).RunTestWithBp(t, bp)

				checkClasspath(t, result, false /* isJava8 */)
			})
		})
	}
}
