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
	"strconv"
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

func testConfig(env map[string]string) android.Config {
	if env == nil {
		env = make(map[string]string)
	}
	if env["ANDROID_JAVA8_HOME"] == "" {
		env["ANDROID_JAVA8_HOME"] = "jdk8"
	}
	config := android.TestArchConfig(buildDir, env)
	config.TestProductVariables.DeviceSystemSdkVersions = &[]string{"14", "15"}
	return config

}

func testContext(config android.Config, bp string,
	fs map[string][]byte) *android.TestContext {

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("android_app", android.ModuleFactoryAdaptor(AndroidAppFactory))
	ctx.RegisterModuleType("java_binary_host", android.ModuleFactoryAdaptor(BinaryHostFactory))
	ctx.RegisterModuleType("java_library", android.ModuleFactoryAdaptor(LibraryFactory(true)))
	ctx.RegisterModuleType("java_library_host", android.ModuleFactoryAdaptor(LibraryHostFactory))
	ctx.RegisterModuleType("java_import", android.ModuleFactoryAdaptor(ImportFactory))
	ctx.RegisterModuleType("java_defaults", android.ModuleFactoryAdaptor(defaultsFactory))
	ctx.RegisterModuleType("java_system_modules", android.ModuleFactoryAdaptor(SystemModulesFactory))
	ctx.RegisterModuleType("java_genrule", android.ModuleFactoryAdaptor(genRuleFactory))
	ctx.RegisterModuleType("filegroup", android.ModuleFactoryAdaptor(genrule.FileGroupFactory))
	ctx.RegisterModuleType("genrule", android.ModuleFactoryAdaptor(genrule.GenRuleFactory))
	ctx.RegisterModuleType("droiddoc", android.ModuleFactoryAdaptor(DroiddocFactory))
	ctx.RegisterModuleType("droiddoc_host", android.ModuleFactoryAdaptor(DroiddocHostFactory))
	ctx.RegisterModuleType("droiddoc_template", android.ModuleFactoryAdaptor(DroiddocTemplateFactory))
	ctx.PreArchMutators(android.RegisterPrebuiltsPreArchMutators)
	ctx.PreArchMutators(android.RegisterPrebuiltsPostDepsMutators)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.RegisterPreSingletonType("overlay", android.SingletonFactoryAdaptor(OverlaySingletonFactory))
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

	bp += `
		android_app {
			name: "framework-res",
			no_framework_libs: true,
		}
	`

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

	mockFS := map[string][]byte{
		"Android.bp":     []byte(bp),
		"a.java":         nil,
		"b.java":         nil,
		"c.java":         nil,
		"b.kt":           nil,
		"a.jar":          nil,
		"b.jar":          nil,
		"java-res/a":     nil,
		"java-res/b":     nil,
		"java-res2/a":    nil,
		"java-fg/a.java": nil,
		"java-fg/b.java": nil,
		"java-fg/c.java": nil,

		"prebuilts/sdk/14/android.jar":                nil,
		"prebuilts/sdk/14/framework.aidl":             nil,
		"prebuilts/sdk/current/android.jar":           nil,
		"prebuilts/sdk/current/framework.aidl":        nil,
		"prebuilts/sdk/current/core.jar":              nil,
		"prebuilts/sdk/system_current/android.jar":    nil,
		"prebuilts/sdk/system_current/framework.aidl": nil,
		"prebuilts/sdk/system_14/android.jar":         nil,
		"prebuilts/sdk/system_14/framework.aidl":      nil,
		"prebuilts/sdk/test_current/android.jar":      nil,
		"prebuilts/sdk/test_current/framework.aidl":   nil,

		// For framework-res, which is an implicit dependency for framework
		"AndroidManifest.xml":                   nil,
		"build/target/product/security/testkey": nil,

		"build/soong/scripts/jar-wrapper.sh": nil,

		"build/make/core/proguard.flags":             nil,
		"build/make/core/proguard_basic_keeps.flags": nil,

		"jdk8/jre/lib/jce.jar": nil,
		"jdk8/jre/lib/rt.jar":  nil,

		"bar-doc/a.java":                 nil,
		"bar-doc/b.java":                 nil,
		"bar-doc/known_oj_tags.txt":      nil,
		"external/doclava/templates-sdk": nil,
	}

	for k, v := range fs {
		mockFS[k] = v
	}

	ctx.MockFileSystem(mockFS)

	return ctx
}

func run(t *testing.T, ctx *android.TestContext, config android.Config) {
	t.Helper()
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)
}

func testJava(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	config := testConfig(nil)
	ctx := testContext(config, bp, nil)
	run(t, ctx, config)

	return ctx
}

func moduleToPath(name string) string {
	switch {
	case name == `""`:
		return name
	case strings.HasSuffix(name, ".jar"):
		return name
	case name == "android_stubs_current" || name == "android_system_stubs_current" ||
		name == "android_test_stubs_current":
		return filepath.Join(buildDir, ".intermediates", name, "android_common", "javac", name+".jar")
	default:
		return filepath.Join(buildDir, ".intermediates", name, "android_common", "turbine-combined", name+".jar")
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
	combineJar := ctx.ModuleForTests("foo", "android_common").Description("for javac")

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, javac.Inputs)
	}

	baz := ctx.ModuleForTests("baz", "android_common").Rule("javac").Output.String()
	barTurbine := filepath.Join(buildDir, ".intermediates", "bar", "android_common", "turbine-combined", "bar.jar")
	bazTurbine := filepath.Join(buildDir, ".intermediates", "baz", "android_common", "turbine-combined", "baz.jar")

	if !strings.Contains(javac.Args["classpath"], barTurbine) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], barTurbine)
	}

	if !strings.Contains(javac.Args["classpath"], bazTurbine) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], bazTurbine)
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

func TestBinary(t *testing.T) {
	ctx := testJava(t, `
		java_library_host {
			name: "foo",
			srcs: ["a.java"],
		}

		java_binary_host {
			name: "bar",
			srcs: ["b.java"],
			static_libs: ["foo"],
		}
	`)

	buildOS := android.BuildOs.String()

	bar := ctx.ModuleForTests("bar", buildOS+"_common")
	barJar := bar.Output("bar.jar").Output.String()
	barWrapper := ctx.ModuleForTests("bar", buildOS+"_x86_64")
	barWrapperDeps := barWrapper.Output("bar").Implicits.Strings()

	// Test that the install binary wrapper depends on the installed jar file
	if len(barWrapperDeps) != 1 || barWrapperDeps[0] != barJar {
		t.Errorf("expected binary wrapper implicits [%q], got %v",
			barJar, barWrapperDeps)
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
		bootclasspath: []string{`""`},
		system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
		classpath:     []string{"prebuilts/sdk/current/android.jar"},
	},
	{

		name:          "system_current",
		properties:    `sdk_version: "system_current",`,
		bootclasspath: []string{`""`},
		system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
		classpath:     []string{"prebuilts/sdk/system_current/android.jar"},
	},
	{

		name:          "system_14",
		properties:    `sdk_version: "system_14",`,
		bootclasspath: []string{`""`},
		system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
		classpath:     []string{"prebuilts/sdk/system_14/android.jar"},
	},
	{

		name:          "test_current",
		properties:    `sdk_version: "test_current",`,
		bootclasspath: []string{`""`},
		system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
		classpath:     []string{"prebuilts/sdk/test_current/android.jar"},
	},
	{

		name:          "core_current",
		properties:    `sdk_version: "core_current",`,
		bootclasspath: []string{`""`},
		system:        "bootclasspath", // special value to tell 1.9 test to expect bootclasspath
		classpath:     []string{"prebuilts/sdk/current/core.jar"},
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

		name:          "host default",
		moduleType:    "java_library_host",
		properties:    ``,
		host:          android.Host,
		bootclasspath: []string{"jdk8/jre/lib/jce.jar", "jdk8/jre/lib/rt.jar"},
		classpath:     []string{},
	},
	{
		name:       "host nostdlib",
		moduleType: "java_library_host",
		host:       android.Host,
		properties: `no_standard_libs: true`,
		classpath:  []string{},
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
				config := testConfig(map[string]string{"EXPERIMENTAL_USE_OPENJDK9": "true"})
				ctx := testContext(config, bp, nil)
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
	combineJar := ctx.ModuleForTests("foo", "android_common").Description("for javac")

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
	combineJar := ctx.ModuleForTests("foo", "android_common").Description("for javac")

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, javac.Inputs)
	}

	barTurbine := filepath.Join(buildDir, ".intermediates", "bar", "android_common", "turbine-combined", "bar.jar")
	if !strings.Contains(javac.Args["classpath"], barTurbine) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], barTurbine)
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
			prop: `java_resource_dirs: ["java-res"]`,
			args: "-C java-res -f java-res/a -f java-res/b",
		},
		{
			// Test that a module with java_resources includes the files
			name: "resource files",
			prop: `java_resources: ["java-res/a", "java-res/b"]`,
			args: "-C . -f java-res/a -f java-res/b",
		},
		{
			// Test that a module with a filegroup in java_resources includes the files with the
			// path prefix
			name: "resource filegroup",
			prop: `java_resources: [":foo-res"]`,
			extra: `
				filegroup {
					name: "foo-res",
					path: "java-res",
					srcs: ["java-res/a", "java-res/b"],
				}`,
			args: "-C java-res -f java-res/a -f java-res/b",
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
			java_resource_dirs: ["java-res", "java-res2"],
			exclude_java_resource_dirs: ["java-res2"],
		}

		java_library {
			name: "bar",
			srcs: ["a.java"],
			java_resources: ["java-res/*"],
			exclude_java_resources: ["java-res/b"],
		}
	`)

	fooRes := ctx.ModuleForTests("foo", "android_common").Output("res/foo.jar")

	expected := "-C java-res -f java-res/a -f java-res/b"
	if fooRes.Args["jarArgs"] != expected {
		t.Errorf("foo resource jar args %q is not %q",
			fooRes.Args["jarArgs"], expected)

	}

	barRes := ctx.ModuleForTests("bar", "android_common").Output("res/bar.jar")

	expected = "-C . -f java-res/a"
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
			tool_files: ["java-res/a"],
			out: ["gen.java"],
		}
	`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	genrule := ctx.ModuleForTests("gen", "").Rule("generator")

	if filepath.Base(genrule.Output.String()) != "gen.java" {
		t.Fatalf(`gen output file %v is not ".../gen.java"`, genrule.Output.String())
	}

	if len(javac.Inputs) != 3 ||
		javac.Inputs[0].String() != "a.java" ||
		javac.Inputs[1].String() != genrule.Output.String() ||
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

		java_library {
			name: "bar",
			srcs: ["b.kt"],
			libs: ["foo"],
			static_libs: ["baz"],
		}

		java_library {
			name: "baz",
			srcs: ["c.java"],
		}
		`)

	fooKotlinc := ctx.ModuleForTests("foo", "android_common").Rule("kotlinc")
	fooJavac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	fooJar := ctx.ModuleForTests("foo", "android_common").Output("combined/foo.jar")

	if len(fooKotlinc.Inputs) != 2 || fooKotlinc.Inputs[0].String() != "a.java" ||
		fooKotlinc.Inputs[1].String() != "b.kt" {
		t.Errorf(`foo kotlinc inputs %v != ["a.java", "b.kt"]`, fooKotlinc.Inputs)
	}

	if len(fooJavac.Inputs) != 1 || fooJavac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, fooJavac.Inputs)
	}

	if !strings.Contains(fooJavac.Args["classpath"], fooKotlinc.Output.String()) {
		t.Errorf("foo classpath %v does not contain %q",
			fooJavac.Args["classpath"], fooKotlinc.Output.String())
	}

	if !inList(fooKotlinc.Output.String(), fooJar.Inputs.Strings()) {
		t.Errorf("foo jar inputs %v does not contain %q",
			fooJar.Inputs.Strings(), fooKotlinc.Output.String())
	}

	fooHeaderJar := ctx.ModuleForTests("foo", "android_common").Output("turbine-combined/foo.jar")
	bazHeaderJar := ctx.ModuleForTests("baz", "android_common").Output("turbine-combined/baz.jar")
	barKotlinc := ctx.ModuleForTests("bar", "android_common").Rule("kotlinc")

	if len(barKotlinc.Inputs) != 1 || barKotlinc.Inputs[0].String() != "b.kt" {
		t.Errorf(`bar kotlinc inputs %v != ["b.kt"]`, barKotlinc.Inputs)
	}

	if !inList(fooHeaderJar.Output.String(), barKotlinc.Implicits.Strings()) {
		t.Errorf(`expected %q in bar implicits %v`,
			fooHeaderJar.Output.String(), barKotlinc.Implicits.Strings())
	}

	if !inList(bazHeaderJar.Output.String(), barKotlinc.Implicits.Strings()) {
		t.Errorf(`expected %q in bar implicits %v`,
			bazHeaderJar.Output.String(), barKotlinc.Implicits.Strings())
	}
}

func TestTurbine(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			static_libs: ["foo"],
		}

		java_library {
			name: "baz",
			srcs: ["c.java"],
			libs: ["bar"],
			sdk_version: "14",
		}
		`)

	fooTurbine := ctx.ModuleForTests("foo", "android_common").Rule("turbine")
	barTurbine := ctx.ModuleForTests("bar", "android_common").Rule("turbine")
	barJavac := ctx.ModuleForTests("bar", "android_common").Rule("javac")
	barTurbineCombined := ctx.ModuleForTests("bar", "android_common").Description("for turbine")
	bazJavac := ctx.ModuleForTests("baz", "android_common").Rule("javac")

	if len(fooTurbine.Inputs) != 1 || fooTurbine.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, fooTurbine.Inputs)
	}

	fooHeaderJar := filepath.Join(buildDir, ".intermediates", "foo", "android_common", "turbine-combined", "foo.jar")
	if !strings.Contains(barTurbine.Args["classpath"], fooHeaderJar) {
		t.Errorf("bar turbine classpath %v does not contain %q", barTurbine.Args["classpath"], fooHeaderJar)
	}
	if !strings.Contains(barJavac.Args["classpath"], fooHeaderJar) {
		t.Errorf("bar javac classpath %v does not contain %q", barJavac.Args["classpath"], fooHeaderJar)
	}
	if len(barTurbineCombined.Inputs) != 2 || barTurbineCombined.Inputs[1].String() != fooHeaderJar {
		t.Errorf("bar turbine combineJar inputs %v does not contain %q", barTurbineCombined.Inputs, fooHeaderJar)
	}
	if !strings.Contains(bazJavac.Args["classpath"], "prebuilts/sdk/14/android.jar") {
		t.Errorf("baz javac classpath %v does not contain %q", bazJavac.Args["classpath"],
			"prebuilts/sdk/14/android.jar")
	}
}

func TestSharding(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "bar",
			srcs: ["a.java","b.java","c.java"],
			javac_shard_size: 1
		}
		`)

	barHeaderJar := filepath.Join(buildDir, ".intermediates", "bar", "android_common", "turbine-combined", "bar.jar")
	for i := 0; i < 3; i++ {
		barJavac := ctx.ModuleForTests("bar", "android_common").Description("javac" + strconv.Itoa(i))
		if !strings.Contains(barJavac.Args["classpath"], barHeaderJar) {
			t.Errorf("bar javac classpath %v does not contain %q", barJavac.Args["classpath"], barHeaderJar)
		}
	}
}

func TestDroiddoc(t *testing.T) {
	ctx := testJava(t, `
		droiddoc_template {
		    name: "droiddoc-templates-sdk",
		    path: ".",
		}
		droiddoc {
		    name: "bar-doc",
		    srcs: [
		        "bar-doc/*.java",
		    ],
		    exclude_srcs: [
		        "bar-doc/b.java"
		    ],
		    custom_template: "droiddoc-templates-sdk",
		    hdf: [
		        "android.whichdoc offline",
		    ],
		    knowntags: [
		        "bar-doc/known_oj_tags.txt",
		    ],
		    proofread_file: "libcore-proofread.txt",
		    todo_file: "libcore-docs-todo.html",
		    args: "-offlinemode -title \"libcore\"",
		}
		`)

	stubsJar := filepath.Join(buildDir, ".intermediates", "bar-doc", "android_common", "bar-doc-stubs.srcjar")
	barDoc := ctx.ModuleForTests("bar-doc", "android_common").Output("bar-doc-stubs.srcjar")
	if stubsJar != barDoc.Output.String() {
		t.Errorf("expected stubs Jar [%q], got %q", stubsJar, barDoc.Output.String())
	}
}

func TestJarGenrules(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
		}

		java_genrule {
			name: "jargen",
			tool_files: ["b.java"],
			cmd: "$(location b.java) $(in) $(out)",
			out: ["jargen.jar"],
			srcs: [":foo"],
		}

		java_library {
			name: "bar",
			static_libs: ["jargen"],
			srcs: ["c.java"],
		}

		java_library {
			name: "baz",
			libs: ["jargen"],
			srcs: ["c.java"],
		}
	`)

	foo := ctx.ModuleForTests("foo", "android_common").Output("javac/foo.jar")
	jargen := ctx.ModuleForTests("jargen", "android_common").Output("jargen.jar")
	bar := ctx.ModuleForTests("bar", "android_common").Output("javac/bar.jar")
	baz := ctx.ModuleForTests("baz", "android_common").Output("javac/baz.jar")
	barCombined := ctx.ModuleForTests("bar", "android_common").Output("combined/bar.jar")

	if len(jargen.Inputs) != 1 || jargen.Inputs[0].String() != foo.Output.String() {
		t.Errorf("expected jargen inputs [%q], got %q", foo.Output.String(), jargen.Inputs.Strings())
	}

	if !strings.Contains(bar.Args["classpath"], jargen.Output.String()) {
		t.Errorf("bar classpath %v does not contain %q", bar.Args["classpath"], jargen.Output.String())
	}

	if !strings.Contains(baz.Args["classpath"], jargen.Output.String()) {
		t.Errorf("baz classpath %v does not contain %q", baz.Args["classpath"], jargen.Output.String())
	}

	if len(barCombined.Inputs) != 2 ||
		barCombined.Inputs[0].String() != bar.Output.String() ||
		barCombined.Inputs[1].String() != jargen.Output.String() {
		t.Errorf("bar combined jar inputs %v is not [%q, %q]",
			barCombined.Inputs.Strings(), bar.Output.String(), jargen.Output.String())
	}
}

func TestExcludeFileGroupInSrcs(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java", ":foo-srcs"],
			exclude_srcs: ["a.java", ":foo-excludes"],
		}

		filegroup {
			name: "foo-srcs",
			srcs: ["java-fg/a.java", "java-fg/b.java", "java-fg/c.java"],
		}

		filegroup {
			name: "foo-excludes",
			srcs: ["java-fg/a.java", "java-fg/b.java"],
		}
	`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "java-fg/c.java" {
		t.Errorf(`foo inputs %v != ["java-fg/c.java"]`, javac.Inputs)
	}
}
