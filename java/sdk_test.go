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
	"reflect"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/java/config"
)

func TestClasspath(t *testing.T) {
	var classpathTestcases = []struct {
		name          string
		unbundled     bool
		pdk           bool
		moduleType    string
		host          android.OsClass
		properties    string
		bootclasspath []string
		system        string
		classpath     []string
		aidl          string
	}{
		{
			name:          "default",
			bootclasspath: config.DefaultBootclasspathLibraries,
			system:        config.DefaultSystemModules,
			classpath:     config.DefaultLibraries,
			aidl:          "-Iframework/aidl",
		},
		{
			name:          `sdk_version:"core_platform"`,
			properties:    `sdk_version:"core_platform"`,
			bootclasspath: config.DefaultBootclasspathLibraries,
			system:        config.DefaultSystemModules,
			classpath:     []string{},
			aidl:          "",
		},
		{
			name:          "blank sdk version",
			properties:    `sdk_version: "",`,
			bootclasspath: config.DefaultBootclasspathLibraries,
			system:        config.DefaultSystemModules,
			classpath:     config.DefaultLibraries,
			aidl:          "-Iframework/aidl",
		},
		{

			name:          "sdk v25",
			properties:    `sdk_version: "25",`,
			bootclasspath: []string{`""`},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			classpath:     []string{"prebuilts/sdk/25/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:          "-pprebuilts/sdk/25/public/framework.aidl",
		},
		{

			name:          "current",
			properties:    `sdk_version: "current",`,
			bootclasspath: []string{"android_stubs_current", "core-lambda-stubs"},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			aidl:          "-p" + buildDir + "/framework.aidl",
		},
		{

			name:          "system_current",
			properties:    `sdk_version: "system_current",`,
			bootclasspath: []string{"android_system_stubs_current", "core-lambda-stubs"},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			aidl:          "-p" + buildDir + "/framework.aidl",
		},
		{

			name:          "system_25",
			properties:    `sdk_version: "system_25",`,
			bootclasspath: []string{`""`},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			classpath:     []string{"prebuilts/sdk/25/system/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:          "-pprebuilts/sdk/25/public/framework.aidl",
		},
		{

			name:          "test_current",
			properties:    `sdk_version: "test_current",`,
			bootclasspath: []string{"android_test_stubs_current", "core-lambda-stubs"},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			aidl:          "-p" + buildDir + "/framework.aidl",
		},
		{

			name:          "core_current",
			properties:    `sdk_version: "core_current",`,
			bootclasspath: []string{"core.current.stubs", "core-lambda-stubs"},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
		},
		{

			name:          "nostdlib",
			properties:    `sdk_version: "none", system_modules: "none"`,
			system:        "none",
			bootclasspath: []string{`""`},
			classpath:     []string{},
		},
		{

			name:          "nostdlib system_modules",
			properties:    `sdk_version: "none", system_modules: "core-platform-api-stubs-system-modules"`,
			system:        "core-platform-api-stubs-system-modules",
			bootclasspath: []string{"core-platform-api-stubs-system-modules-lib"},
			classpath:     []string{},
		},
		{

			name:          "host default",
			moduleType:    "java_library_host",
			properties:    ``,
			host:          android.Host,
			bootclasspath: []string{"jdk8/jre/lib/jce.jar", "jdk8/jre/lib/rt.jar"},
			classpath:     []string{},
		},
		{

			name:          "host supported default",
			host:          android.Host,
			properties:    `host_supported: true,`,
			classpath:     []string{},
			bootclasspath: []string{"jdk8/jre/lib/jce.jar", "jdk8/jre/lib/rt.jar"},
		},
		{
			name:       "host supported nostdlib",
			host:       android.Host,
			properties: `host_supported: true, sdk_version: "none", system_modules: "none"`,
			classpath:  []string{},
		},
		{

			name:          "unbundled sdk v25",
			unbundled:     true,
			properties:    `sdk_version: "25",`,
			bootclasspath: []string{`""`},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			classpath:     []string{"prebuilts/sdk/25/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:          "-pprebuilts/sdk/25/public/framework.aidl",
		},
		{

			name:          "unbundled current",
			unbundled:     true,
			properties:    `sdk_version: "current",`,
			bootclasspath: []string{`""`},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			classpath:     []string{"prebuilts/sdk/current/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:          "-pprebuilts/sdk/current/public/framework.aidl",
		},

		{
			name:          "pdk default",
			pdk:           true,
			bootclasspath: []string{`""`},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			classpath:     []string{"prebuilts/sdk/25/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:          "-pprebuilts/sdk/25/public/framework.aidl",
		},
		{
			name:          "pdk current",
			pdk:           true,
			properties:    `sdk_version: "current",`,
			bootclasspath: []string{`""`},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			classpath:     []string{"prebuilts/sdk/25/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:          "-pprebuilts/sdk/25/public/framework.aidl",
		},
		{
			name:          "pdk 25",
			pdk:           true,
			properties:    `sdk_version: "25",`,
			bootclasspath: []string{`""`},
			system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
			classpath:     []string{"prebuilts/sdk/25/public/android.jar", "prebuilts/sdk/tools/core-lambda-stubs.jar"},
			aidl:          "-pprebuilts/sdk/25/public/framework.aidl",
		},
	}

	for _, testcase := range classpathTestcases {
		t.Run(testcase.name, func(t *testing.T) {
			moduleType := "java_library"
			if testcase.moduleType != "" {
				moduleType = testcase.moduleType
			}

			bp := moduleType + ` {
				name: "foo",
				srcs: ["a.java"],
				target: {
					android: {
						srcs: ["bar-doc/IFoo.aidl"],
					},
				},
				` + testcase.properties + `
			}`

			variant := "android_common"
			if testcase.host == android.Host {
				variant = android.BuildOs.String() + "_common"
			}

			convertModulesToPaths := func(cp []string) []string {
				ret := make([]string, len(cp))
				for i, e := range cp {
					ret[i] = moduleToPath(e)
				}
				return ret
			}

			bootclasspath := convertModulesToPaths(testcase.bootclasspath)
			classpath := convertModulesToPaths(testcase.classpath)

			bc := strings.Join(bootclasspath, ":")
			if bc != "" {
				bc = "-bootclasspath " + bc
			}

			c := strings.Join(classpath, ":")
			if c != "" {
				c = "-classpath " + c
			}
			system := ""
			if testcase.system == "none" {
				system = "--system=none"
			} else if testcase.system != "" {
				system = "--system=" + filepath.Join(buildDir, ".intermediates", testcase.system, "android_common", "system")
			}

			checkClasspath := func(t *testing.T, ctx *android.TestContext) {
				foo := ctx.ModuleForTests("foo", variant)
				javac := foo.Rule("javac")

				aidl := foo.MaybeRule("aidl")

				got := javac.Args["bootClasspath"]
				if got != bc {
					t.Errorf("bootclasspath expected %q != got %q", bc, got)
				}

				got = javac.Args["classpath"]
				if got != c {
					t.Errorf("classpath expected %q != got %q", c, got)
				}

				var deps []string
				if aidl.Rule != nil {
					deps = append(deps, aidl.Output.String())
				}
				if len(bootclasspath) > 0 && bootclasspath[0] != `""` {
					deps = append(deps, bootclasspath...)
				}
				deps = append(deps, classpath...)

				if !reflect.DeepEqual(javac.Implicits.Strings(), deps) {
					t.Errorf("implicits expected %q != got %q", deps, javac.Implicits.Strings())
				}
			}

			t.Run("Java language level 8", func(t *testing.T) {
				// Test default javac -source 1.8 -target 1.8
				config := testConfig(nil)
				if testcase.unbundled {
					config.TestProductVariables.Unbundled_build = proptools.BoolPtr(true)
				}
				if testcase.pdk {
					config.TestProductVariables.Pdk = proptools.BoolPtr(true)
				}
				ctx := testContext(bp, nil)
				run(t, ctx, config)

				checkClasspath(t, ctx)

				if testcase.host != android.Host {
					aidl := ctx.ModuleForTests("foo", variant).Rule("aidl")

					if g, w := aidl.RuleParams.Command, testcase.aidl+" -I."; !strings.Contains(g, w) {
						t.Errorf("want aidl command to contain %q, got %q", w, g)
					}
				}
			})

			// Test again with javac -source 9 -target 9
			t.Run("Java language level 9", func(t *testing.T) {
				config := testConfig(map[string]string{"EXPERIMENTAL_JAVA_LANGUAGE_LEVEL_9": "true"})
				if testcase.unbundled {
					config.TestProductVariables.Unbundled_build = proptools.BoolPtr(true)
				}
				if testcase.pdk {
					config.TestProductVariables.Pdk = proptools.BoolPtr(true)
				}
				ctx := testContext(bp, nil)
				run(t, ctx, config)

				javac := ctx.ModuleForTests("foo", variant).Rule("javac")
				got := javac.Args["bootClasspath"]
				expected := system
				if testcase.system == "bootclasspath" {
					expected = bc
				}
				if got != expected {
					t.Errorf("bootclasspath expected %q != got %q", expected, got)
				}
			})

			// Test again with PLATFORM_VERSION_CODENAME=REL
			t.Run("REL", func(t *testing.T) {
				config := testConfig(nil)
				config.TestProductVariables.Platform_sdk_codename = proptools.StringPtr("REL")
				config.TestProductVariables.Platform_sdk_final = proptools.BoolPtr(true)

				if testcase.unbundled {
					config.TestProductVariables.Unbundled_build = proptools.BoolPtr(true)
				}
				if testcase.pdk {
					config.TestProductVariables.Pdk = proptools.BoolPtr(true)
				}
				ctx := testContext(bp, nil)
				run(t, ctx, config)

				checkClasspath(t, ctx)
			})
		})
	}

}
