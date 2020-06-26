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
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/dexpreopt"
	"android/soong/genrule"
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

func testConfig(env map[string]string, bp string, fs map[string][]byte) android.Config {
	bp += dexpreopt.BpToolModulesForTest()

	config := TestConfig(buildDir, env, bp, fs)

	// Set up the global Once cache used for dexpreopt.GlobalSoongConfig, so that
	// it doesn't create a real one, which would fail.
	_ = dexpreopt.GlobalSoongConfigForTests(config)

	return config
}

func testContext() *android.TestContext {

	ctx := android.NewTestArchContext()
	RegisterJavaBuildComponents(ctx)
	RegisterAppBuildComponents(ctx)
	RegisterAARBuildComponents(ctx)
	RegisterGenRuleBuildComponents(ctx)
	RegisterSystemModulesBuildComponents(ctx)
	ctx.RegisterModuleType("java_plugin", PluginFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("genrule", genrule.GenRuleFactory)
	RegisterDocsBuildComponents(ctx)
	RegisterStubsBuildComponents(ctx)
	RegisterSdkLibraryBuildComponents(ctx)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)

	RegisterPrebuiltApisBuildComponents(ctx)

	ctx.PostDepsMutators(android.RegisterOverridePostDepsMutators)
	ctx.RegisterPreSingletonType("overlay", android.SingletonFactoryAdaptor(OverlaySingletonFactory))
	ctx.RegisterPreSingletonType("sdk_versions", android.SingletonFactoryAdaptor(sdkPreSingletonFactory))

	// Register module types and mutators from cc needed for JNI testing
	cc.RegisterRequiredBuildComponentsForTest(ctx)

	dexpreopt.RegisterToolModulesForTest(ctx)

	return ctx
}

func run(t *testing.T, ctx *android.TestContext, config android.Config) {
	t.Helper()

	pathCtx := android.PathContextForTesting(config)
	dexpreopt.SetTestGlobalConfig(config, dexpreopt.GlobalConfigForTests(pathCtx))

	ctx.Register(config)
	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)
}

func testJavaError(t *testing.T, pattern string, bp string) (*android.TestContext, android.Config) {
	t.Helper()
	return testJavaErrorWithConfig(t, pattern, testConfig(nil, bp, nil))
}

func testJavaErrorWithConfig(t *testing.T, pattern string, config android.Config) (*android.TestContext, android.Config) {
	t.Helper()
	ctx := testContext()

	pathCtx := android.PathContextForTesting(config)
	dexpreopt.SetTestGlobalConfig(config, dexpreopt.GlobalConfigForTests(pathCtx))

	ctx.Register(config)
	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return ctx, config
	}
	_, errs = ctx.PrepareBuildActions(config)
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return ctx, config
	}

	t.Fatalf("missing expected error %q (0 errors are returned)", pattern)

	return ctx, config
}

func testJavaWithFS(t *testing.T, bp string, fs map[string][]byte) (*android.TestContext, android.Config) {
	t.Helper()
	return testJavaWithConfig(t, testConfig(nil, bp, fs))
}

func testJava(t *testing.T, bp string) (*android.TestContext, android.Config) {
	t.Helper()
	return testJavaWithFS(t, bp, nil)
}

func testJavaWithConfig(t *testing.T, config android.Config) (*android.TestContext, android.Config) {
	t.Helper()
	ctx := testContext()
	run(t, ctx, config)

	return ctx, config
}

func moduleToPath(name string) string {
	switch {
	case name == `""`:
		return name
	case strings.HasSuffix(name, ".jar"):
		return name
	default:
		return filepath.Join(buildDir, ".intermediates", name, "android_common", "turbine-combined", name+".jar")
	}
}

func checkModuleDependencies(t *testing.T, ctx *android.TestContext, name, variant string, expected []string) {
	t.Helper()
	module := ctx.ModuleForTests(name, variant).Module()
	deps := []string{}
	ctx.VisitDirectDeps(module, func(m blueprint.Module) {
		deps = append(deps, m.Name())
	})
	sort.Strings(deps)

	if actual := deps; !reflect.DeepEqual(expected, actual) {
		t.Errorf("expected %#q, found %#q", expected, actual)
	}
}

func TestJavaLinkType(t *testing.T) {
	testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
		}

		java_library {
			name: "bar",
			sdk_version: "current",
			srcs: ["b.java"],
		}

		java_library {
			name: "baz",
			sdk_version: "system_current",
			srcs: ["c.java"],
		}
	`)

	testJavaError(t, "Adjust sdk_version: property of the source or target module so that target module is built with the same or smaller API set than the source.", `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			sdk_version: "current",
			static_libs: ["baz"],
		}

		java_library {
			name: "bar",
			sdk_version: "current",
			srcs: ["b.java"],
		}

		java_library {
			name: "baz",
			sdk_version: "system_current",
			srcs: ["c.java"],
		}
	`)

	testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			sdk_version: "system_current",
			static_libs: ["baz"],
		}

		java_library {
			name: "bar",
			sdk_version: "current",
			srcs: ["b.java"],
		}

		java_library {
			name: "baz",
			sdk_version: "system_current",
			srcs: ["c.java"],
		}
	`)

	testJavaError(t, "Adjust sdk_version: property of the source or target module so that target module is built with the same or smaller API set than the source.", `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			sdk_version: "system_current",
			static_libs: ["baz"],
		}

		java_library {
			name: "bar",
			sdk_version: "current",
			srcs: ["b.java"],
		}

		java_library {
			name: "baz",
			srcs: ["c.java"],
		}
	`)
}

func TestSimple(t *testing.T) {
	ctx, _ := testJava(t, `
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

func TestExportedPlugins(t *testing.T) {
	type Result struct {
		library    string
		processors string
	}
	var tests = []struct {
		name    string
		extra   string
		results []Result
	}{
		{
			name:    "Exported plugin is not a direct plugin",
			extra:   `java_library { name: "exports", srcs: ["a.java"], exported_plugins: ["plugin"] }`,
			results: []Result{{library: "exports", processors: "-proc:none"}},
		},
		{
			name: "Exports plugin to dependee",
			extra: `
				java_library{name: "exports", exported_plugins: ["plugin"]}
				java_library{name: "foo", srcs: ["a.java"], libs: ["exports"]}
				java_library{name: "bar", srcs: ["a.java"], static_libs: ["exports"]}
			`,
			results: []Result{
				{library: "foo", processors: "-processor com.android.TestPlugin"},
				{library: "bar", processors: "-processor com.android.TestPlugin"},
			},
		},
		{
			name: "Exports plugin to android_library",
			extra: `
				java_library{name: "exports", exported_plugins: ["plugin"]}
				android_library{name: "foo", srcs: ["a.java"],  libs: ["exports"]}
				android_library{name: "bar", srcs: ["a.java"], static_libs: ["exports"]}
			`,
			results: []Result{
				{library: "foo", processors: "-processor com.android.TestPlugin"},
				{library: "bar", processors: "-processor com.android.TestPlugin"},
			},
		},
		{
			name: "Exports plugin is not propagated via transitive deps",
			extra: `
				java_library{name: "exports", exported_plugins: ["plugin"]}
				java_library{name: "foo", srcs: ["a.java"], libs: ["exports"]}
				java_library{name: "bar", srcs: ["a.java"], static_libs: ["foo"]}
			`,
			results: []Result{
				{library: "foo", processors: "-processor com.android.TestPlugin"},
				{library: "bar", processors: "-proc:none"},
			},
		},
		{
			name: "Exports plugin appends to plugins",
			extra: `
                java_plugin{name: "plugin2", processor_class: "com.android.TestPlugin2"}
				java_library{name: "exports", exported_plugins: ["plugin"]}
				java_library{name: "foo", srcs: ["a.java"], libs: ["exports"], plugins: ["plugin2"]}
			`,
			results: []Result{
				{library: "foo", processors: "-processor com.android.TestPlugin,com.android.TestPlugin2"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := testJava(t, `
				java_plugin {
					name: "plugin",
					processor_class: "com.android.TestPlugin",
				}
			`+test.extra)

			for _, want := range test.results {
				javac := ctx.ModuleForTests(want.library, "android_common").Rule("javac")
				if javac.Args["processor"] != want.processors {
					t.Errorf("For library %v, expected %v, found %v", want.library, want.processors, javac.Args["processor"])
				}
			}
		})
	}
}

func TestSdkVersionByPartition(t *testing.T) {
	testJavaError(t, "sdk_version must have a value when the module is located at vendor or product", `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			vendor: true,
		}
	`)

	testJava(t, `
		java_library {
			name: "bar",
			srcs: ["b.java"],
		}
	`)

	for _, enforce := range []bool{true, false} {
		bp := `
			java_library {
				name: "foo",
				srcs: ["a.java"],
				product_specific: true,
			}
		`

		config := testConfig(nil, bp, nil)
		config.TestProductVariables.EnforceProductPartitionInterface = proptools.BoolPtr(enforce)
		if enforce {
			testJavaErrorWithConfig(t, "sdk_version must have a value when the module is located at vendor or product", config)
		} else {
			testJavaWithConfig(t, config)
		}
	}
}

func TestArchSpecific(t *testing.T) {
	ctx, _ := testJava(t, `
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
	ctx, _ := testJava(t, `
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

func TestPrebuilts(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java", ":stubs-source"],
			libs: ["bar", "sdklib"],
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

		dex_import {
			name: "qux",
			jars: ["b.jar"],
		}

		java_sdk_library_import {
			name: "sdklib",
			public: {
				jars: ["c.jar"],
			},
		}

		prebuilt_stubs_sources {
			name: "stubs-source",
			srcs: ["stubs/sources"],
		}

		java_test_import {
			name: "test",
			jars: ["a.jar"],
			test_suites: ["cts"],
			test_config: "AndroidTest.xml",
		}
		`)

	fooModule := ctx.ModuleForTests("foo", "android_common")
	javac := fooModule.Rule("javac")
	combineJar := ctx.ModuleForTests("foo", "android_common").Description("for javac")
	barJar := ctx.ModuleForTests("bar", "android_common").Rule("combineJar").Output
	bazJar := ctx.ModuleForTests("baz", "android_common").Rule("combineJar").Output
	sdklibStubsJar := ctx.ModuleForTests("sdklib.stubs", "android_common").Rule("combineJar").Output

	fooLibrary := fooModule.Module().(*Library)
	assertDeepEquals(t, "foo java sources incorrect",
		[]string{"a.java"}, fooLibrary.compiledJavaSrcs.Strings())

	assertDeepEquals(t, "foo java source jars incorrect",
		[]string{".intermediates/stubs-source/android_common/stubs-source-stubs.srcjar"},
		android.NormalizePathsForTesting(fooLibrary.compiledSrcJars))

	if !strings.Contains(javac.Args["classpath"], barJar.String()) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], barJar.String())
	}

	if !strings.Contains(javac.Args["classpath"], sdklibStubsJar.String()) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], sdklibStubsJar.String())
	}

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != bazJar.String() {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, bazJar.String())
	}

	ctx.ModuleForTests("qux", "android_common").Rule("Cp")
}

func assertDeepEquals(t *testing.T, message string, expected interface{}, actual interface{}) {
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("%s: expected %q, found %q", message, expected, actual)
	}
}

func TestJavaSdkLibraryImport(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			libs: ["sdklib"],
			sdk_version: "current",
		}

		java_library {
			name: "foo.system",
			srcs: ["a.java"],
			libs: ["sdklib"],
			sdk_version: "system_current",
		}

		java_library {
			name: "foo.test",
			srcs: ["a.java"],
			libs: ["sdklib"],
			sdk_version: "test_current",
		}

		java_sdk_library_import {
			name: "sdklib",
			public: {
				jars: ["a.jar"],
			},
			system: {
				jars: ["b.jar"],
			},
			test: {
				jars: ["c.jar"],
				stub_srcs: ["c.java"],
			},
		}
		`)

	for _, scope := range []string{"", ".system", ".test"} {
		fooModule := ctx.ModuleForTests("foo"+scope, "android_common")
		javac := fooModule.Rule("javac")

		sdklibStubsJar := ctx.ModuleForTests("sdklib.stubs"+scope, "android_common").Rule("combineJar").Output
		if !strings.Contains(javac.Args["classpath"], sdklibStubsJar.String()) {
			t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], sdklibStubsJar.String())
		}
	}

	checkModuleDependencies(t, ctx, "sdklib", "android_common", []string{
		`prebuilt_sdklib.stubs`,
		`prebuilt_sdklib.stubs.source.test`,
		`prebuilt_sdklib.stubs.system`,
		`prebuilt_sdklib.stubs.test`,
	})
}

func TestJavaSdkLibraryImport_WithSource(t *testing.T) {
	ctx, _ := testJava(t, `
		java_sdk_library {
			name: "sdklib",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "none",
			public: {
				enabled: true,
			},
		}

		java_sdk_library_import {
			name: "sdklib",
			public: {
				jars: ["a.jar"],
			},
		}
		`)

	checkModuleDependencies(t, ctx, "sdklib", "android_common", []string{
		`dex2oatd`,
		`prebuilt_sdklib`,
		`sdklib.impl`,
		`sdklib.stubs`,
		`sdklib.stubs.source`,
		`sdklib.xml`,
	})

	checkModuleDependencies(t, ctx, "prebuilt_sdklib", "android_common", []string{
		`sdklib.impl`,
		// This should be prebuilt_sdklib.stubs but is set to sdklib.stubs because the
		// dependency is added after prebuilts may have been renamed and so has to use
		// the renamed name.
		`sdklib.stubs`,
		`sdklib.xml`,
	})
}

func TestJavaSdkLibraryImport_Preferred(t *testing.T) {
	ctx, _ := testJava(t, `
		java_sdk_library {
			name: "sdklib",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "none",
			public: {
				enabled: true,
			},
		}

		java_sdk_library_import {
			name: "sdklib",
			prefer: true,
			public: {
				jars: ["a.jar"],
			},
		}
		`)

	checkModuleDependencies(t, ctx, "sdklib", "android_common", []string{
		`dex2oatd`,
		`prebuilt_sdklib`,
		// This should be sdklib.stubs but is switched to the prebuilt because it is preferred.
		`prebuilt_sdklib.stubs`,
		`sdklib.impl`,
		`sdklib.stubs.source`,
		`sdklib.xml`,
	})

	checkModuleDependencies(t, ctx, "prebuilt_sdklib", "android_common", []string{
		`prebuilt_sdklib.stubs`,
		`sdklib.impl`,
		`sdklib.xml`,
	})
}

func TestDefaults(t *testing.T) {
	ctx, _ := testJava(t, `
		java_defaults {
			name: "defaults",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
			optimize: {enabled: false},
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

		android_test {
			name: "atestOptimize",
			defaults: ["defaults"],
			optimize: {enabled: true},
		}

		android_test {
			name: "atestNoOptimize",
			defaults: ["defaults"],
		}

		android_test {
			name: "atestDefault",
			srcs: ["a.java"],
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

	atestOptimize := ctx.ModuleForTests("atestOptimize", "android_common").MaybeRule("r8")
	if atestOptimize.Output == nil {
		t.Errorf("atestOptimize should optimize APK")
	}

	atestNoOptimize := ctx.ModuleForTests("atestNoOptimize", "android_common").MaybeRule("d8")
	if atestNoOptimize.Output == nil {
		t.Errorf("atestNoOptimize should not optimize APK")
	}

	atestDefault := ctx.ModuleForTests("atestDefault", "android_common").MaybeRule("r8")
	if atestDefault.Output == nil {
		t.Errorf("atestDefault should optimize APK")
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
			args: "-C java-res -f java-res/a/a -f java-res/b/b",
		},
		{
			// Test that a module with java_resources includes the files
			name: "resource files",
			prop: `java_resources: ["java-res/a/a", "java-res/b/b"]`,
			args: "-C . -f java-res/a/a -f java-res/b/b",
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
					srcs: ["java-res/a/a", "java-res/b/b"],
				}`,
			args: "-C java-res -f java-res/a/a -f java-res/b/b",
		},
		{
			// Test that a module with wildcards in java_resource_dirs has the correct path prefixes
			name: "wildcard dirs",
			prop: `java_resource_dirs: ["java-res/*"]`,
			args: "-C java-res/a -f java-res/a/a -C java-res/b -f java-res/b/b",
		},
		{
			// Test that a module exclude_java_resource_dirs excludes the files
			name: "wildcard dirs",
			prop: `java_resource_dirs: ["java-res/*"], exclude_java_resource_dirs: ["java-res/b"]`,
			args: "-C java-res/a -f java-res/a/a",
		},
		{
			// Test wildcards in java_resources
			name: "wildcard files",
			prop: `java_resources: ["java-res/**/*"]`,
			args: "-C . -f java-res/a/a -f java-res/b/b",
		},
		{
			// Test exclude_java_resources with java_resources
			name: "wildcard files with exclude",
			prop: `java_resources: ["java-res/**/*"], exclude_java_resources: ["java-res/b/*"]`,
			args: "-C . -f java-res/a/a",
		},
		{
			// Test exclude_java_resources with java_resource_dirs
			name: "resource dirs with exclude files",
			prop: `java_resource_dirs: ["java-res"], exclude_java_resources: ["java-res/b/b"]`,
			args: "-C java-res -f java-res/a/a",
		},
		{
			// Test exclude_java_resource_dirs with java_resource_dirs
			name: "resource dirs with exclude files",
			prop: `java_resource_dirs: ["java-res", "java-res2"], exclude_java_resource_dirs: ["java-res2"]`,
			args: "-C java-res -f java-res/a/a -f java-res/b/b",
		},
	}

	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := testJavaWithFS(t, `
				java_library {
					name: "foo",
					srcs: [
						"a.java",
						"b.java",
						"c.java",
					],
					`+test.prop+`,
				}
			`+test.extra,
				map[string][]byte{
					"java-res/a/a": nil,
					"java-res/b/b": nil,
					"java-res2/a":  nil,
				},
			)

			foo := ctx.ModuleForTests("foo", "android_common").Output("withres/foo.jar")
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

func TestIncludeSrcs(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		java_library {
			name: "foo",
			srcs: [
				"a.java",
				"b.java",
				"c.java",
			],
			include_srcs: true,
		}

		java_library {
			name: "bar",
			srcs: [
				"a.java",
				"b.java",
				"c.java",
			],
			java_resource_dirs: ["java-res"],
			include_srcs: true,
		}
	`, map[string][]byte{
		"java-res/a/a": nil,
		"java-res/b/b": nil,
		"java-res2/a":  nil,
	})

	// Test a library with include_srcs: true
	foo := ctx.ModuleForTests("foo", "android_common").Output("withres/foo.jar")
	fooSrcJar := ctx.ModuleForTests("foo", "android_common").Output("foo.srcjar")

	if g, w := fooSrcJar.Output.String(), foo.Inputs.Strings(); !inList(g, w) {
		t.Errorf("foo combined jars %v does not contain %q", w, g)
	}

	if g, w := fooSrcJar.Args["jarArgs"], "-C . -f a.java -f b.java -f c.java"; g != w {
		t.Errorf("foo source jar args %q is not %q", w, g)
	}

	// Test a library with include_srcs: true and resources
	bar := ctx.ModuleForTests("bar", "android_common").Output("withres/bar.jar")
	barResCombined := ctx.ModuleForTests("bar", "android_common").Output("res-combined/bar.jar")
	barRes := ctx.ModuleForTests("bar", "android_common").Output("res/bar.jar")
	barSrcJar := ctx.ModuleForTests("bar", "android_common").Output("bar.srcjar")

	if g, w := barSrcJar.Output.String(), barResCombined.Inputs.Strings(); !inList(g, w) {
		t.Errorf("bar combined resource jars %v does not contain %q", w, g)
	}

	if g, w := barRes.Output.String(), barResCombined.Inputs.Strings(); !inList(g, w) {
		t.Errorf("bar combined resource jars %v does not contain %q", w, g)
	}

	if g, w := barResCombined.Output.String(), bar.Inputs.Strings(); !inList(g, w) {
		t.Errorf("bar combined jars %v does not contain %q", w, g)
	}

	if g, w := barSrcJar.Args["jarArgs"], "-C . -f a.java -f b.java -f c.java"; g != w {
		t.Errorf("bar source jar args %q is not %q", w, g)
	}

	if g, w := barRes.Args["jarArgs"], "-C java-res -f java-res/a/a -f java-res/b/b"; g != w {
		t.Errorf("bar resource jar args %q is not %q", w, g)
	}
}

func TestGeneratedSources(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
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
	`, map[string][]byte{
		"a.java": nil,
		"b.java": nil,
	})

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

func TestTurbine(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "14",
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			static_libs: ["foo"],
			sdk_version: "14",
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
	if !strings.Contains(bazJavac.Args["classpath"], "prebuilts/sdk/14/public/android.jar") {
		t.Errorf("baz javac classpath %v does not contain %q", bazJavac.Args["classpath"],
			"prebuilts/sdk/14/public/android.jar")
	}
}

func TestSharding(t *testing.T) {
	ctx, _ := testJava(t, `
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
	ctx, _ := testJavaWithFS(t, `
		droiddoc_exported_dir {
		    name: "droiddoc-templates-sdk",
		    path: ".",
		}
		filegroup {
		    name: "bar-doc-aidl-srcs",
		    srcs: ["bar-doc/IBar.aidl"],
		    path: "bar-doc",
		}
		droiddoc {
		    name: "bar-doc",
		    srcs: [
		        "bar-doc/a.java",
		        "bar-doc/IFoo.aidl",
		        ":bar-doc-aidl-srcs",
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
		`,
		map[string][]byte{
			"bar-doc/a.java": nil,
			"bar-doc/b.java": nil,
		})

	barDoc := ctx.ModuleForTests("bar-doc", "android_common").Rule("javadoc")
	var javaSrcs []string
	for _, i := range barDoc.Inputs {
		javaSrcs = append(javaSrcs, i.Base())
	}
	if len(javaSrcs) != 1 || javaSrcs[0] != "a.java" {
		t.Errorf("inputs of bar-doc must be []string{\"a.java\"}, but was %#v.", javaSrcs)
	}

	aidl := ctx.ModuleForTests("bar-doc", "android_common").Rule("aidl")
	if g, w := barDoc.Implicits.Strings(), aidl.Output.String(); !inList(w, g) {
		t.Errorf("implicits of bar-doc must contain %q, but was %q.", w, g)
	}

	if g, w := aidl.Implicits.Strings(), []string{"bar-doc/IBar.aidl", "bar-doc/IFoo.aidl"}; !reflect.DeepEqual(w, g) {
		t.Errorf("aidl inputs must be %q, but was %q", w, g)
	}
}

func TestDroidstubs(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		droiddoc_exported_dir {
		    name: "droiddoc-templates-sdk",
		    path: ".",
		}

		droidstubs {
		    name: "bar-stubs",
		    srcs: [
		        "bar-doc/a.java",
				],
				api_levels_annotations_dirs: [
					"droiddoc-templates-sdk",
				],
				api_levels_annotations_enabled: true,
		}

		droidstubs {
		    name: "bar-stubs-other",
		    srcs: [
		        "bar-doc/a.java",
				],
				api_levels_annotations_dirs: [
					"droiddoc-templates-sdk",
				],
				api_levels_annotations_enabled: true,
				api_levels_jar_filename: "android.other.jar",
		}
		`,
		map[string][]byte{
			"bar-doc/a.java": nil,
		})
	testcases := []struct {
		moduleName          string
		expectedJarFilename string
	}{
		{
			moduleName:          "bar-stubs",
			expectedJarFilename: "android.jar",
		},
		{
			moduleName:          "bar-stubs-other",
			expectedJarFilename: "android.other.jar",
		},
	}
	for _, c := range testcases {
		m := ctx.ModuleForTests(c.moduleName, "android_common")
		metalava := m.Rule("metalava")
		expected := "--android-jar-pattern ./%/public/" + c.expectedJarFilename
		if actual := metalava.RuleParams.Command; !strings.Contains(actual, expected) {
			t.Errorf("For %q, expected metalava argument %q, but was not found %q", c.moduleName, expected, actual)
		}
	}
}

func TestDroidstubsWithSystemModules(t *testing.T) {
	ctx, _ := testJava(t, `
		droidstubs {
		    name: "stubs-source-system-modules",
		    srcs: [
		        "bar-doc/a.java",
		    ],
				sdk_version: "none",
				system_modules: "source-system-modules",
		}

		java_library {
				name: "source-jar",
		    srcs: [
		        "a.java",
		    ],
		}

		java_system_modules {
				name: "source-system-modules",
				libs: ["source-jar"],
		}

		droidstubs {
		    name: "stubs-prebuilt-system-modules",
		    srcs: [
		        "bar-doc/a.java",
		    ],
				sdk_version: "none",
				system_modules: "prebuilt-system-modules",
		}

		java_import {
				name: "prebuilt-jar",
				jars: ["a.jar"],
		}

		java_system_modules_import {
				name: "prebuilt-system-modules",
				libs: ["prebuilt-jar"],
		}
		`)

	checkSystemModulesUseByDroidstubs(t, ctx, "stubs-source-system-modules", "source-jar.jar")

	checkSystemModulesUseByDroidstubs(t, ctx, "stubs-prebuilt-system-modules", "prebuilt-jar.jar")
}

func checkSystemModulesUseByDroidstubs(t *testing.T, ctx *android.TestContext, moduleName string, systemJar string) {
	metalavaRule := ctx.ModuleForTests(moduleName, "android_common").Rule("metalava")
	var systemJars []string
	for _, i := range metalavaRule.Implicits {
		systemJars = append(systemJars, i.Base())
	}
	if len(systemJars) < 1 || systemJars[0] != systemJar {
		t.Errorf("inputs of %q must be []string{%q}, but was %#v.", moduleName, systemJar, systemJars)
	}
}

func TestJarGenrules(t *testing.T) {
	ctx, _ := testJava(t, `
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
	ctx, _ := testJava(t, `
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

func TestJavaLibrary(t *testing.T) {
	config := testConfig(nil, "", map[string][]byte{
		"libcore/Android.bp": []byte(`
				java_library {
						name: "core",
						sdk_version: "none",
						system_modules: "none",
				}`),
	})
	ctx := testContext()
	run(t, ctx, config)
}

func TestJavaSdkLibrary(t *testing.T) {
	ctx, _ := testJava(t, `
		droiddoc_exported_dir {
			name: "droiddoc-templates-sdk",
			path: ".",
		}
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
		}
		java_sdk_library {
			name: "bar",
			srcs: ["a.java", "b.java"],
			api_packages: ["bar"],
		}
		java_library {
			name: "baz",
			srcs: ["c.java"],
			libs: ["foo", "bar.stubs"],
			sdk_version: "system_current",
		}
		java_sdk_library {
			name: "barney",
			srcs: ["c.java"],
			api_only: true,
		}
		java_sdk_library {
			name: "betty",
			srcs: ["c.java"],
			shared_library: false,
		}
		java_sdk_library_import {
		    name: "quuz",
				public: {
					jars: ["c.jar"],
				},
		}
		java_sdk_library_import {
		    name: "fred",
				public: {
					jars: ["b.jar"],
				},
		}
		java_sdk_library_import {
		    name: "wilma",
				public: {
					jars: ["b.jar"],
				},
				shared_library: false,
		}
		java_library {
		    name: "qux",
		    srcs: ["c.java"],
		    libs: ["baz", "fred", "quuz.stubs", "wilma", "barney", "betty"],
		    sdk_version: "system_current",
		}
		java_library {
			name: "baz-test",
			srcs: ["c.java"],
			libs: ["foo"],
			sdk_version: "test_current",
		}
		java_library {
			name: "baz-29",
			srcs: ["c.java"],
			libs: ["foo"],
			sdk_version: "system_29",
		}
		`)

	// check the existence of the internal modules
	ctx.ModuleForTests("foo", "android_common")
	ctx.ModuleForTests(apiScopePublic.stubsLibraryModuleName("foo"), "android_common")
	ctx.ModuleForTests(apiScopeSystem.stubsLibraryModuleName("foo"), "android_common")
	ctx.ModuleForTests(apiScopeTest.stubsLibraryModuleName("foo"), "android_common")
	ctx.ModuleForTests(apiScopePublic.stubsSourceModuleName("foo"), "android_common")
	ctx.ModuleForTests(apiScopeSystem.stubsSourceModuleName("foo"), "android_common")
	ctx.ModuleForTests(apiScopeTest.stubsSourceModuleName("foo"), "android_common")
	ctx.ModuleForTests("foo"+sdkXmlFileSuffix, "android_common")
	ctx.ModuleForTests("foo.api.public.28", "")
	ctx.ModuleForTests("foo.api.system.28", "")
	ctx.ModuleForTests("foo.api.test.28", "")

	bazJavac := ctx.ModuleForTests("baz", "android_common").Rule("javac")
	// tests if baz is actually linked to the stubs lib
	if !strings.Contains(bazJavac.Args["classpath"], "foo.stubs.system.jar") {
		t.Errorf("baz javac classpath %v does not contain %q", bazJavac.Args["classpath"],
			"foo.stubs.system.jar")
	}
	// ... and not to the impl lib
	if strings.Contains(bazJavac.Args["classpath"], "foo.jar") {
		t.Errorf("baz javac classpath %v should not contain %q", bazJavac.Args["classpath"],
			"foo.jar")
	}
	// test if baz is not linked to the system variant of foo
	if strings.Contains(bazJavac.Args["classpath"], "foo.stubs.jar") {
		t.Errorf("baz javac classpath %v should not contain %q", bazJavac.Args["classpath"],
			"foo.stubs.jar")
	}

	bazTestJavac := ctx.ModuleForTests("baz-test", "android_common").Rule("javac")
	// tests if baz-test is actually linked to the test stubs lib
	if !strings.Contains(bazTestJavac.Args["classpath"], "foo.stubs.test.jar") {
		t.Errorf("baz-test javac classpath %v does not contain %q", bazTestJavac.Args["classpath"],
			"foo.stubs.test.jar")
	}

	baz29Javac := ctx.ModuleForTests("baz-29", "android_common").Rule("javac")
	// tests if baz-29 is actually linked to the system 29 stubs lib
	if !strings.Contains(baz29Javac.Args["classpath"], "prebuilts/sdk/29/system/foo.jar") {
		t.Errorf("baz-29 javac classpath %v does not contain %q", baz29Javac.Args["classpath"],
			"prebuilts/sdk/29/system/foo.jar")
	}

	// test if baz has exported SDK lib names foo and bar to qux
	qux := ctx.ModuleForTests("qux", "android_common")
	if quxLib, ok := qux.Module().(*Library); ok {
		sdkLibs := quxLib.ExportedSdkLibs()
		sort.Strings(sdkLibs)
		if w := []string{"bar", "foo", "fred", "quuz"}; !reflect.DeepEqual(w, sdkLibs) {
			t.Errorf("qux should export %q but exports %q", w, sdkLibs)
		}
	}
}

func TestJavaSdkLibrary_DoNotAccessImplWhenItIsNotBuilt(t *testing.T) {
	ctx, _ := testJava(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_only: true,
			public: {
				enabled: true,
			},
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			libs: ["foo"],
		}
		`)

	// The bar library should depend on the stubs jar.
	barLibrary := ctx.ModuleForTests("bar", "android_common").Rule("javac")
	if expected, actual := `^-classpath .*:/[^:]*/turbine-combined/foo\.stubs\.jar$`, barLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

func TestJavaSdkLibrary_UseSourcesFromAnotherSdkLibrary(t *testing.T) {
	testJava(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			public: {
				enabled: true,
			},
		}

		java_library {
			name: "bar",
			srcs: ["b.java", ":foo{.public.stubs.source}"],
		}
		`)
}

func TestJavaSdkLibrary_AccessOutputFiles_MissingScope(t *testing.T) {
	testJavaError(t, `"foo" does not provide api scope system`, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			public: {
				enabled: true,
			},
		}

		java_library {
			name: "bar",
			srcs: ["b.java", ":foo{.system.stubs.source}"],
		}
		`)
}

func TestJavaSdkLibrary_Deps(t *testing.T) {
	ctx, _ := testJava(t, `
		java_sdk_library {
			name: "sdklib",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "none",
			public: {
				enabled: true,
			},
		}
		`)

	checkModuleDependencies(t, ctx, "sdklib", "android_common", []string{
		`dex2oatd`,
		`sdklib.impl`,
		`sdklib.stubs`,
		`sdklib.stubs.source`,
		`sdklib.xml`,
	})
}

func TestJavaSdkLibraryImport_AccessOutputFiles(t *testing.T) {
	testJava(t, `
		java_sdk_library_import {
			name: "foo",
			public: {
				jars: ["a.jar"],
				stub_srcs: ["a.java"],
				current_api: "api/current.txt",
				removed_api: "api/removed.txt",
			},
		}

		java_library {
			name: "bar",
			srcs: [":foo{.public.stubs.source}"],
			java_resources: [
				":foo{.public.api.txt}",
				":foo{.public.removed-api.txt}",
			],
		}
		`)
}

func TestJavaSdkLibraryImport_AccessOutputFiles_Invalid(t *testing.T) {
	bp := `
		java_sdk_library_import {
			name: "foo",
			public: {
				jars: ["a.jar"],
			},
		}
		`

	t.Run("stubs.source", func(t *testing.T) {
		testJavaError(t, `stubs.source not available for api scope public`, bp+`
		java_library {
			name: "bar",
			srcs: [":foo{.public.stubs.source}"],
			java_resources: [
				":foo{.public.api.txt}",
				":foo{.public.removed-api.txt}",
			],
		}
		`)
	})

	t.Run("api.txt", func(t *testing.T) {
		testJavaError(t, `api.txt not available for api scope public`, bp+`
		java_library {
			name: "bar",
			srcs: ["a.java"],
			java_resources: [
				":foo{.public.api.txt}",
			],
		}
		`)
	})

	t.Run("removed-api.txt", func(t *testing.T) {
		testJavaError(t, `removed-api.txt not available for api scope public`, bp+`
		java_library {
			name: "bar",
			srcs: ["a.java"],
			java_resources: [
				":foo{.public.removed-api.txt}",
			],
		}
		`)
	})
}

func TestJavaSdkLibrary_InvalidScopes(t *testing.T) {
	testJavaError(t, `module "foo": enabled api scope "system" depends on disabled scope "public"`, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
			// Explicitly disable public to test the check that ensures the set of enabled
			// scopes is consistent.
			public: {
				enabled: false,
			},
			system: {
				enabled: true,
			},
		}
		`)
}

func TestJavaSdkLibrary_SdkVersion_ForScope(t *testing.T) {
	testJava(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
			system: {
				enabled: true,
				sdk_version: "module_current",
			},
		}
		`)
}

func TestJavaSdkLibrary_ModuleLib(t *testing.T) {
	testJava(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
			system: {
				enabled: true,
			},
			module_lib: {
				enabled: true,
			},
		}
		`)
}

func TestJavaSdkLibrary_SystemServer(t *testing.T) {
	testJava(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
			system: {
				enabled: true,
			},
			system_server: {
				enabled: true,
			},
		}
		`)
}

func TestJavaSdkLibrary_MissingScope(t *testing.T) {
	testJavaError(t, `requires api scope module-lib from foo but it only has \[\] available`, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			public: {
				enabled: false,
			},
		}

		java_library {
			name: "baz",
			srcs: ["a.java"],
			libs: ["foo"],
			sdk_version: "module_current",
		}
		`)
}

func TestJavaSdkLibrary_FallbackScope(t *testing.T) {
	testJava(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			system: {
				enabled: true,
			},
		}

		java_library {
			name: "baz",
			srcs: ["a.java"],
			libs: ["foo"],
			// foo does not have module-lib scope so it should fallback to system
			sdk_version: "module_current",
		}
		`)
}

func TestJavaSdkLibrary_DefaultToStubs(t *testing.T) {
	ctx, _ := testJava(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			system: {
				enabled: true,
			},
			default_to_stubs: true,
		}

		java_library {
			name: "baz",
			srcs: ["a.java"],
			libs: ["foo"],
			// does not have sdk_version set, should fallback to module,
			// which will then fallback to system because the module scope
			// is not enabled.
		}
		`)
	// The baz library should depend on the system stubs jar.
	bazLibrary := ctx.ModuleForTests("baz", "android_common").Rule("javac")
	if expected, actual := `^-classpath .*:/[^:]*/turbine-combined/foo\.stubs.system\.jar$`, bazLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

var compilerFlagsTestCases = []struct {
	in  string
	out bool
}{
	{
		in:  "a",
		out: false,
	},
	{
		in:  "-a",
		out: true,
	},
	{
		in:  "-no-jdk",
		out: false,
	},
	{
		in:  "-no-stdlib",
		out: false,
	},
	{
		in:  "-kotlin-home",
		out: false,
	},
	{
		in:  "-kotlin-home /some/path",
		out: false,
	},
	{
		in:  "-include-runtime",
		out: false,
	},
	{
		in:  "-Xintellij-plugin-root",
		out: false,
	},
}

type mockContext struct {
	android.ModuleContext
	result bool
}

func (ctx *mockContext) PropertyErrorf(property, format string, args ...interface{}) {
	// CheckBadCompilerFlags calls this function when the flag should be rejected
	ctx.result = false
}

func TestCompilerFlags(t *testing.T) {
	for _, testCase := range compilerFlagsTestCases {
		ctx := &mockContext{result: true}
		CheckKotlincFlags(ctx, []string{testCase.in})
		if ctx.result != testCase.out {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", ctx.result)
		}
	}
}

// TODO(jungjw): Consider making this more robust by ignoring path order.
func checkPatchModuleFlag(t *testing.T, ctx *android.TestContext, moduleName string, expected string) {
	variables := ctx.ModuleForTests(moduleName, "android_common").Module().VariablesForTests()
	flags := strings.Split(variables["javacFlags"], " ")
	got := ""
	for _, flag := range flags {
		keyEnd := strings.Index(flag, "=")
		if keyEnd > -1 && flag[:keyEnd] == "--patch-module" {
			got = flag[keyEnd+1:]
			break
		}
	}
	if expected != got {
		t.Errorf("Unexpected patch-module flag for module %q - expected %q, but got %q", moduleName, expected, got)
	}
}

func TestPatchModule(t *testing.T) {
	t.Run("Java language level 8", func(t *testing.T) {
		// Test with legacy javac -source 1.8 -target 1.8
		bp := `
			java_library {
				name: "foo",
				srcs: ["a.java"],
				java_version: "1.8",
			}

			java_library {
				name: "bar",
				srcs: ["b.java"],
				sdk_version: "none",
				system_modules: "none",
				patch_module: "java.base",
				java_version: "1.8",
			}

			java_library {
				name: "baz",
				srcs: ["c.java"],
				patch_module: "java.base",
				java_version: "1.8",
			}
		`
		ctx, _ := testJava(t, bp)

		checkPatchModuleFlag(t, ctx, "foo", "")
		checkPatchModuleFlag(t, ctx, "bar", "")
		checkPatchModuleFlag(t, ctx, "baz", "")
	})

	t.Run("Java language level 9", func(t *testing.T) {
		// Test with default javac -source 9 -target 9
		bp := `
			java_library {
				name: "foo",
				srcs: ["a.java"],
			}

			java_library {
				name: "bar",
				srcs: ["b.java"],
				sdk_version: "none",
				system_modules: "none",
				patch_module: "java.base",
			}

			java_library {
				name: "baz",
				srcs: ["c.java"],
				patch_module: "java.base",
			}
		`
		ctx, _ := testJava(t, bp)

		checkPatchModuleFlag(t, ctx, "foo", "")
		expected := "java.base=.:" + buildDir
		checkPatchModuleFlag(t, ctx, "bar", expected)
		expected = "java.base=" + strings.Join([]string{".", buildDir, moduleToPath("ext"), moduleToPath("framework")}, ":")
		checkPatchModuleFlag(t, ctx, "baz", expected)
	})
}

func TestJavaSystemModules(t *testing.T) {
	ctx, _ := testJava(t, `
		java_system_modules {
			name: "system-modules",
			libs: ["system-module1", "system-module2"],
		}
		java_library {
			name: "system-module1",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "none",
		}
		java_library {
			name: "system-module2",
			srcs: ["b.java"],
			sdk_version: "none",
			system_modules: "none",
		}
		`)

	// check the existence of the module
	systemModules := ctx.ModuleForTests("system-modules", "android_common")

	cmd := systemModules.Rule("jarsTosystemModules")

	// make sure the command compiles against the supplied modules.
	for _, module := range []string{"system-module1.jar", "system-module2.jar"} {
		if !strings.Contains(cmd.Args["classpath"], module) {
			t.Errorf("system modules classpath %v does not contain %q", cmd.Args["classpath"],
				module)
		}
	}
}

func TestJavaSystemModulesImport(t *testing.T) {
	ctx, _ := testJava(t, `
		java_system_modules_import {
			name: "system-modules",
			libs: ["system-module1", "system-module2"],
		}
		java_import {
			name: "system-module1",
			jars: ["a.jar"],
		}
		java_import {
			name: "system-module2",
			jars: ["b.jar"],
		}
		`)

	// check the existence of the module
	systemModules := ctx.ModuleForTests("system-modules", "android_common")

	cmd := systemModules.Rule("jarsTosystemModules")

	// make sure the command compiles against the supplied modules.
	for _, module := range []string{"system-module1.jar", "system-module2.jar"} {
		if !strings.Contains(cmd.Args["classpath"], module) {
			t.Errorf("system modules classpath %v does not contain %q", cmd.Args["classpath"],
				module)
		}
	}
}

func TestJavaLibraryWithSystemModules(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
		    name: "lib-with-source-system-modules",
		    srcs: [
		        "a.java",
		    ],
				sdk_version: "none",
				system_modules: "source-system-modules",
		}

		java_library {
				name: "source-jar",
		    srcs: [
		        "a.java",
		    ],
		}

		java_system_modules {
				name: "source-system-modules",
				libs: ["source-jar"],
		}

		java_library {
		    name: "lib-with-prebuilt-system-modules",
		    srcs: [
		        "a.java",
		    ],
				sdk_version: "none",
				system_modules: "prebuilt-system-modules",
		}

		java_import {
				name: "prebuilt-jar",
				jars: ["a.jar"],
		}

		java_system_modules_import {
				name: "prebuilt-system-modules",
				libs: ["prebuilt-jar"],
		}
		`)

	checkBootClasspathForSystemModule(t, ctx, "lib-with-source-system-modules", "/source-jar.jar")

	checkBootClasspathForSystemModule(t, ctx, "lib-with-prebuilt-system-modules", "/prebuilt-jar.jar")
}

func checkBootClasspathForSystemModule(t *testing.T, ctx *android.TestContext, moduleName string, expectedSuffix string) {
	javacRule := ctx.ModuleForTests(moduleName, "android_common").Rule("javac")
	bootClasspath := javacRule.Args["bootClasspath"]
	if strings.HasPrefix(bootClasspath, "--system ") && strings.HasSuffix(bootClasspath, expectedSuffix) {
		t.Errorf("bootclasspath of %q must start with --system and end with %q, but was %#v.", moduleName, expectedSuffix, bootClasspath)
	}
}
