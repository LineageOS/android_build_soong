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
					ret[i] = moduleToPath(e)
				}
				return ret
			}

			bootclasspath := convertModulesToPaths(testcase.bootclasspath)
			classpath := convertModulesToPaths(testcase.classpath)

			bc := ""
			var bcDeps []string
			if len(bootclasspath) > 0 {
				bc = "-bootclasspath " + strings.Join(bootclasspath, ":")
				if bootclasspath[0] != `""` {
					bcDeps = bootclasspath
				}
			}

			c := ""
			if len(classpath) > 0 {
				c = "-classpath " + strings.Join(classpath, ":")
			}

			system := ""
			var systemDeps []string
			if testcase.system == "none" {
				system = "--system=none"
			} else if testcase.system == "bootclasspath" {
				system = bc
				systemDeps = bcDeps
			} else if testcase.system != "" {
				system = "--system=" + filepath.Join(buildDir, ".intermediates", testcase.system, "android_common", "system")
				// The module-relative parts of these paths are hardcoded in system_modules.go:
				systemDeps = []string{
					filepath.Join(buildDir, ".intermediates", testcase.system, "android_common", "system", "lib", "modules"),
					filepath.Join(buildDir, ".intermediates", testcase.system, "android_common", "system", "lib", "jrt-fs.jar"),
					filepath.Join(buildDir, ".intermediates", testcase.system, "android_common", "system", "release"),
				}
			}

			checkClasspath := func(t *testing.T, ctx *android.TestContext, isJava8 bool) {
				foo := ctx.ModuleForTests("foo", variant)
				javac := foo.Rule("javac")
				var deps []string

				aidl := foo.MaybeRule("aidl")
				if aidl.Rule != nil {
					deps = append(deps, aidl.Output.String())
				}

				got := javac.Args["bootClasspath"]
				expected := ""
				if isJava8 {
					expected = bc
					deps = append(deps, bcDeps...)
				} else {
					expected = system
					deps = append(deps, systemDeps...)
				}
				if got != expected {
					t.Errorf("bootclasspath expected %q != got %q", expected, got)
				}

				got = javac.Args["classpath"]
				if got != c {
					t.Errorf("classpath expected %q != got %q", c, got)
				}
				deps = append(deps, classpath...)

				if !reflect.DeepEqual(javac.Implicits.Strings(), deps) {
					t.Errorf("implicits expected %q != got %q", deps, javac.Implicits.Strings())
				}
			}

			// Test with legacy javac -source 1.8 -target 1.8
			t.Run("Java language level 8", func(t *testing.T) {
				config := testConfig(nil)
				if testcase.unbundled {
					config.TestProductVariables.Unbundled_build = proptools.BoolPtr(true)
				}
				if testcase.pdk {
					config.TestProductVariables.Pdk = proptools.BoolPtr(true)
				}
				ctx := testContext(bpJava8, nil)
				run(t, ctx, config)

				checkClasspath(t, ctx, true /* isJava8 */)

				if testcase.host != android.Host {
					aidl := ctx.ModuleForTests("foo", variant).Rule("aidl")

					if g, w := aidl.RuleParams.Command, testcase.aidl+" -I."; !strings.Contains(g, w) {
						t.Errorf("want aidl command to contain %q, got %q", w, g)
					}
				}
			})

			// Test with default javac -source 9 -target 9
			t.Run("Java language level 9", func(t *testing.T) {
				config := testConfig(nil)
				if testcase.unbundled {
					config.TestProductVariables.Unbundled_build = proptools.BoolPtr(true)
				}
				if testcase.pdk {
					config.TestProductVariables.Pdk = proptools.BoolPtr(true)
				}
				ctx := testContext(bp, nil)
				run(t, ctx, config)

				checkClasspath(t, ctx, false /* isJava8 */)

				if testcase.host != android.Host {
					aidl := ctx.ModuleForTests("foo", variant).Rule("aidl")

					if g, w := aidl.RuleParams.Command, testcase.aidl+" -I."; !strings.Contains(g, w) {
						t.Errorf("want aidl command to contain %q, got %q", w, g)
					}
				}
			})

			// Test again with PLATFORM_VERSION_CODENAME=REL, javac -source 8 -target 8
			t.Run("REL + Java language level 8", func(t *testing.T) {
				config := testConfig(nil)
				config.TestProductVariables.Platform_sdk_codename = proptools.StringPtr("REL")
				config.TestProductVariables.Platform_sdk_final = proptools.BoolPtr(true)

				if testcase.unbundled {
					config.TestProductVariables.Unbundled_build = proptools.BoolPtr(true)
				}
				if testcase.pdk {
					config.TestProductVariables.Pdk = proptools.BoolPtr(true)
				}
				ctx := testContext(bpJava8, nil)
				run(t, ctx, config)

				checkClasspath(t, ctx, true /* isJava8 */)
			})

			// TODO(b/142896162): Add a with PLATFORM_VERSION_CODENAME=REL, javac -source 9 -target 9, when that all works.
		})
	}

}
