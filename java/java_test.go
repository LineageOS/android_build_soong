// Copyright 2017 Google Inc. All rights reserved.
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
	"android/soong/android"
	"android/soong/genrule"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_java_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}
func testJava(t *testing.T, bp string) *android.TestContext {
	return testJavaWithEnv(t, bp, nil)
}

func testJavaWithEnv(t *testing.T, bp string, env map[string]string) *android.TestContext {
	config := android.TestArchConfig(buildDir, env)

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("android_app", android.ModuleFactoryAdaptor(AndroidAppFactory))
	ctx.RegisterModuleType("java_library", android.ModuleFactoryAdaptor(LibraryFactory(true)))
	ctx.RegisterModuleType("java_library_host", android.ModuleFactoryAdaptor(LibraryHostFactory))
	ctx.RegisterModuleType("java_import", android.ModuleFactoryAdaptor(ImportFactory))
	ctx.RegisterModuleType("java_defaults", android.ModuleFactoryAdaptor(defaultsFactory))
	ctx.RegisterModuleType("java_system_modules", android.ModuleFactoryAdaptor(SystemModulesFactory))
	ctx.RegisterModuleType("filegroup", android.ModuleFactoryAdaptor(genrule.FileGroupFactory))
	ctx.RegisterModuleType("genrule", android.ModuleFactoryAdaptor(genrule.GenRuleFactory))
	ctx.PreArchMutators(android.RegisterPrebuiltsPreArchMutators)
	ctx.PreArchMutators(android.RegisterPrebuiltsPostDepsMutators)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.Register()

	extraModules := []string{
		"core-oj",
		"core-libart",
		"framework",
		"ext",
		"okhttp",
		"android_stubs_current",
		"android_system_stubs_current",
		"android_test_stubs_current",
		"kotlin-stdlib",
	}

	for _, extra := range extraModules {
		bp += fmt.Sprintf(`
			java_library {
				name: "%s",
				srcs: ["a.java"],
				no_standard_libs: true,
				system_modules: "core-system-modules",
			}
		`, extra)
	}

	if config.TargetOpenJDK9() {
		systemModules := []string{
			"core-system-modules",
			"android_stubs_current_system_modules",
			"android_system_stubs_current_system_modules",
			"android_test_stubs_current_system_modules",
		}

		for _, extra := range systemModules {
			bp += fmt.Sprintf(`
			java_system_modules {
				name: "%s",
			}
		`, extra)
		}
	}

	ctx.MockFileSystem(map[string][]byte{
		"Android.bp": []byte(bp),
		"a.java":     nil,
		"b.java":     nil,
		"c.java":     nil,
		"b.kt":       nil,
		"a.jar":      nil,
		"b.jar":      nil,
		"res/a":      nil,
		"res/b":      nil,
		"res2/a":     nil,

		"prebuilts/sdk/14/android.jar":    nil,
		"prebuilts/sdk/14/framework.aidl": nil,
	})

	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	fail(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	fail(t, errs)

	return ctx
}

func moduleToPath(name string) string {
	switch {
	case name == `""`:
		return name
	case strings.HasSuffix(name, ".jar"):
		return name
	default:
		return filepath.Join(buildDir, ".intermediates", name, "android_common", "javac", name+".jar")
	}
}

func TestSimple(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
		}

		java_library {
			name: "baz",
			srcs: ["c.java"],
		}
	`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	combineJar := ctx.ModuleForTests("foo", "android_common").Rule("combineJar")

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, javac.Inputs)
	}

	bar := ctx.ModuleForTests("bar", "android_common").Rule("javac").Output.String()
	baz := ctx.ModuleForTests("baz", "android_common").Rule("javac").Output.String()

	if !strings.Contains(javac.Args["classpath"], bar) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], bar)
	}

	if !strings.Contains(javac.Args["classpath"], baz) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], baz)
	}

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != baz {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, baz)
	}
}

func TestArchSpecific(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			target: {
				android: {
					srcs: ["b.java"],
				},
			},
		}
	`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	if len(javac.Inputs) != 2 || javac.Inputs[0].String() != "a.java" || javac.Inputs[1].String() != "b.java" {
		t.Errorf(`foo inputs %v != ["a.java", "b.java"]`, javac.Inputs)
	}
}

var classpathTestcases = []struct {
	name          string
	moduleType    string
	host          android.OsClass
	properties    string
	bootclasspath []string
	system        string
	classpath     []string
}{
	{
		name:          "default",
		bootclasspath: []string{"core-oj", "core-libart"},
		system:        "core-system-modules",
		classpath:     []string{"ext", "framework", "okhttp"},
	},
	{
		name:          "blank sdk version",
		properties:    `sdk_version: "",`,
		bootclasspath: []string{"core-oj", "core-libart"},
		system:        "core-system-modules",
		classpath:     []string{"ext", "framework", "okhttp"},
	},
	{

		name:          "sdk v14",
		properties:    `sdk_version: "14",`,
		bootclasspath: []string{`""`},
		system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
		classpath:     []string{"prebuilts/sdk/14/android.jar"},
	},
	{

		name:          "current",
		properties:    `sdk_version: "current",`,
		bootclasspath: []string{"android_stubs_current"},
		system:        "android_stubs_current_system_modules",
		classpath:     []string{},
	},
	{

		name:          "system_current",
		properties:    `sdk_version: "system_current",`,
		bootclasspath: []string{"android_system_stubs_current"},
		system:        "android_system_stubs_current_system_modules",
		classpath:     []string{},
	},
	{

		name:          "test_current",
		properties:    `sdk_version: "test_current",`,
		bootclasspath: []string{"android_test_stubs_current"},
		system:        "android_test_stubs_current_system_modules",
		classpath:     []string{},
	},
	{

		name:          "nostdlib",
		properties:    `no_standard_libs: true, system_modules: "none"`,
		system:        "none",
		bootclasspath: []string{`""`},
		classpath:     []string{},
	},
	{

		name:          "nostdlib system_modules",
		properties:    `no_standard_libs: true, system_modules: "core-system-modules"`,
		system:        "core-system-modules",
		bootclasspath: []string{`""`},
		classpath:     []string{},
	},
	{

		name:       "host default",
		moduleType: "java_library_host",
		properties: ``,
		host:       android.Host,
		classpath:  []string{},
	},
	{
		name:       "host nostdlib",
		moduleType: "java_library_host",
		host:       android.Host,
		properties: `no_standard_libs: true`,
		classpath:  []string{},
	},
	{

		name:       "host supported default",
		host:       android.Host,
		properties: `host_supported: true,`,
		classpath:  []string{},
	},
	{
		name:       "host supported nostdlib",
		host:       android.Host,
		properties: `host_supported: true, no_standard_libs: true, system_modules: "none"`,
		classpath:  []string{},
	},
}

func TestClasspath(t *testing.T) {
	for _, testcase := range classpathTestcases {
		t.Run(testcase.name, func(t *testing.T) {
			moduleType := "java_library"
			if testcase.moduleType != "" {
				moduleType = testcase.moduleType
			}

			bp := moduleType + ` {
				name: "foo",
				srcs: ["a.java"],
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
				system = "--system=" + filepath.Join(buildDir, ".intermediates", testcase.system, "android_common", "system") + "/"
			}

			t.Run("1.8", func(t *testing.T) {
				// Test default javac 1.8
				ctx := testJava(t, bp)

				javac := ctx.ModuleForTests("foo", variant).Rule("javac")

				got := javac.Args["bootClasspath"]
				if got != bc {
					t.Errorf("bootclasspath expected %q != got %q", bc, got)
				}

				got = javac.Args["classpath"]
				if got != c {
					t.Errorf("classpath expected %q != got %q", c, got)
				}

				var deps []string
				if len(bootclasspath) > 0 && bootclasspath[0] != `""` {
					deps = append(deps, bootclasspath...)
				}
				deps = append(deps, classpath...)

				if !reflect.DeepEqual(javac.Implicits.Strings(), deps) {
					t.Errorf("implicits expected %q != got %q", deps, javac.Implicits.Strings())
				}
			})

			// Test again with javac 1.9
			t.Run("1.9", func(t *testing.T) {
				ctx := testJavaWithEnv(t, bp, map[string]string{"EXPERIMENTAL_USE_OPENJDK9": "true"})

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
		})
	}

}

func TestPrebuilts(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
		}

		java_import {
			name: "bar",
			jars: ["a.jar"],
		}

		java_import {
			name: "baz",
			jars: ["b.jar"],
		}
		`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	combineJar := ctx.ModuleForTests("foo", "android_common").Rule("combineJar")

	bar := "a.jar"
	if !strings.Contains(javac.Args["classpath"], bar) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], bar)
	}

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != "b.jar" {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, "b.jar")
	}
}

func TestDefaults(t *testing.T) {
	ctx := testJava(t, `
		java_defaults {
			name: "defaults",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
		}

		java_library {
			name: "foo",
			defaults: ["defaults"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
		}

		java_library {
			name: "baz",
			srcs: ["c.java"],
		}
		`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	combineJar := ctx.ModuleForTests("foo", "android_common").Rule("combineJar")

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, javac.Inputs)
	}

	bar := ctx.ModuleForTests("bar", "android_common").Rule("javac").Output.String()
	if !strings.Contains(javac.Args["classpath"], bar) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], bar)
	}

	baz := ctx.ModuleForTests("baz", "android_common").Rule("javac").Output.String()
	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != baz {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, baz)
	}
}

func TestResources(t *testing.T) {
	var table = []struct {
		name  string
		prop  string
		extra string
		args  string
	}{
		{
			// Test that a module with java_resource_dirs includes the files
			name: "resource dirs",
			prop: `java_resource_dirs: ["res"]`,
			args: "-C res -f res/a -f res/b",
		},
		{
			// Test that a module with java_resources includes the files
			name: "resource files",
			prop: `java_resources: ["res/a", "res/b"]`,
			args: "-C . -f res/a -f res/b",
		},
		{
			// Test that a module with a filegroup in java_resources includes the files with the
			// path prefix
			name: "resource filegroup",
			prop: `java_resources: [":foo-res"]`,
			extra: `
				filegroup {
					name: "foo-res",
					path: "res",
					srcs: ["res/a", "res/b"],
				}`,
			args: "-C res -f res/a -f res/b",
		},
		{
			// Test that a module with "include_srcs: true" includes its source files in the resources jar
			name: "include sources",
			prop: `include_srcs: true`,
			args: "-C . -f a.java -f b.java -f c.java",
		},
	}

	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			ctx := testJava(t, `
				java_library {
					name: "foo",
					srcs: [
						"a.java",
						"b.java",
						"c.java",
					],
					`+test.prop+`,
				}
			`+test.extra)

			foo := ctx.ModuleForTests("foo", "android_common").Output("combined/foo.jar")
			fooRes := ctx.ModuleForTests("foo", "android_common").Output("res/foo.jar")

			if !inList(fooRes.Output.String(), foo.Inputs.Strings()) {
				t.Errorf("foo combined jars %v does not contain %q",
					foo.Inputs.Strings(), fooRes.Output.String())
			}

			if fooRes.Args["jarArgs"] != test.args {
				t.Errorf("foo resource jar args %q is not %q",
					fooRes.Args["jarArgs"], test.args)
			}
		})
	}
}

func TestExcludeResources(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			java_resource_dirs: ["res", "res2"],
			exclude_java_resource_dirs: ["res2"],
		}

		java_library {
			name: "bar",
			srcs: ["a.java"],
			java_resources: ["res/*"],
			exclude_java_resources: ["res/b"],
		}
	`)

	fooRes := ctx.ModuleForTests("foo", "android_common").Output("res/foo.jar")

	expected := "-C res -f res/a -f res/b"
	if fooRes.Args["jarArgs"] != expected {
		t.Errorf("foo resource jar args %q is not %q",
			fooRes.Args["jarArgs"], expected)

	}

	barRes := ctx.ModuleForTests("bar", "android_common").Output("res/bar.jar")

	expected = "-C . -f res/a"
	if barRes.Args["jarArgs"] != expected {
		t.Errorf("bar resource jar args %q is not %q",
			barRes.Args["jarArgs"], expected)

	}
}

func TestGeneratedSources(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: [
				"a*.java",
				":gen",
				"b*.java",
			],
		}

		genrule {
			name: "gen",
			tool_files: ["res/a"],
			out: ["gen.java"],
		}
	`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	genrule := ctx.ModuleForTests("gen", "").Rule("generator")

	if len(genrule.Outputs) != 1 || filepath.Base(genrule.Outputs[0].String()) != "gen.java" {
		t.Fatalf(`gen output file %v is not [".../gen.java"]`, genrule.Outputs.Strings())
	}

	if len(javac.Inputs) != 3 ||
		javac.Inputs[0].String() != "a.java" ||
		javac.Inputs[1].String() != genrule.Outputs[0].String() ||
		javac.Inputs[2].String() != "b.java" {
		t.Errorf(`foo inputs %v != ["a.java", ".../gen.java", "b.java"]`, javac.Inputs)
	}
}

func TestKotlin(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
                        srcs: ["a.java", "b.kt"],
		}
		`)

	kotlinc := ctx.ModuleForTests("foo", "android_common").Rule("kotlinc")
	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	jar := ctx.ModuleForTests("foo", "android_common").Output("combined/foo.jar")

	if len(kotlinc.Inputs) != 2 || kotlinc.Inputs[0].String() != "a.java" ||
		kotlinc.Inputs[1].String() != "b.kt" {
		t.Errorf(`foo kotlinc inputs %v != ["a.java", "b.kt"]`, kotlinc.Inputs)
	}

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, javac.Inputs)
	}

	if !strings.Contains(javac.Args["classpath"], kotlinc.Output.String()) {
		t.Errorf("foo classpath %v does not contain %q",
			javac.Args["classpath"], kotlinc.Output.String())
	}

	if !inList(kotlinc.Output.String(), jar.Inputs.Strings()) {
		t.Errorf("foo jar inputs %v does not contain %q",
			jar.Inputs.Strings(), kotlinc.Output.String())
	}
}

func fail(t *testing.T, errs []error) {
	if len(errs) > 0 {
		for _, err := range errs {
			t.Error(err)
		}
		t.FailNow()
	}
}
