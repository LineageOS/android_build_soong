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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/dexpreopt"
	"android/soong/genrule"
	"android/soong/python"
)

// Legacy preparer used for running tests within the java package.
//
// This includes everything that was needed to run any test in the java package prior to the
// introduction of the test fixtures. Tests that are being converted to use fixtures directly
// rather than through the testJava...() methods should avoid using this and instead use the
// various preparers directly, using android.GroupFixturePreparers(...) to group them when
// necessary.
//
// deprecated
var prepareForJavaTest = android.GroupFixturePreparers(
	genrule.PrepareForTestWithGenRuleBuildComponents,
	// Get the CC build components but not default modules.
	cc.PrepareForTestWithCcBuildComponents,
	// Include all the default java modules.
	PrepareForTestWithJavaDefaultModules,
	PrepareForTestWithOverlayBuildComponents,
	python.PrepareForTestWithPythonBuildComponents,
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		ctx.RegisterPreSingletonType("sdk_versions", sdkPreSingletonFactory)
	}),
	PrepareForTestWithDexpreopt,
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// testJavaError is a legacy way of running tests of java modules that expect errors.
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testJavaError(t *testing.T, pattern string, bp string) (*android.TestContext, android.Config) {
	t.Helper()
	result := android.GroupFixturePreparers(
		prepareForJavaTest, dexpreopt.PrepareForTestByEnablingDexpreopt).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(pattern)).
		RunTestWithBp(t, bp)
	return result.TestContext, result.Config
}

// testJavaWithFS runs tests using the prepareForJavaTest
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testJavaWithFS(t *testing.T, bp string, fs android.MockFS) (*android.TestContext, android.Config) {
	t.Helper()
	result := android.GroupFixturePreparers(
		prepareForJavaTest, fs.AddToFixture()).RunTestWithBp(t, bp)
	return result.TestContext, result.Config
}

// testJava runs tests using the prepareForJavaTest
//
// Do not add any new usages of this, instead use the prepareForJavaTest directly as it makes it
// much easier to customize the test behavior.
//
// If it is necessary to customize the behavior of an existing test that uses this then please first
// convert the test to using prepareForJavaTest first and then in a following change add the
// appropriate fixture preparers. Keeping the conversion change separate makes it easy to verify
// that it did not change the test behavior unexpectedly.
//
// deprecated
func testJava(t *testing.T, bp string) (*android.TestContext, android.Config) {
	t.Helper()
	result := prepareForJavaTest.RunTestWithBp(t, bp)
	return result.TestContext, result.Config
}

// defaultModuleToPath constructs a path to the turbine generate jar for a default test module that
// is defined in PrepareForIntegrationTestWithJava
func defaultModuleToPath(name string) string {
	switch {
	case name == `""`:
		return name
	case strings.HasSuffix(name, ".jar"):
		return name
	default:
		return filepath.Join("out", "soong", ".intermediates", defaultJavaDir, name, "android_common", "turbine-combined", name+".jar")
	}
}

// Test that the PrepareForTestWithJavaDefaultModules provides all the files that it uses by
// running it in a fixture that requires all source files to exist.
func TestPrepareForTestWithJavaDefaultModules(t *testing.T) {
	android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.PrepareForTestDisallowNonExistentPaths,
	).RunTest(t)
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

	testJavaError(t, "consider adjusting sdk_version: OR platform_apis:", `
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

	testJavaError(t, "consider adjusting sdk_version: OR platform_apis:", `
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
	barTurbine := filepath.Join("out", "soong", ".intermediates", "bar", "android_common", "turbine-combined", "bar.jar")
	bazTurbine := filepath.Join("out", "soong", ".intermediates", "baz", "android_common", "turbine-combined", "baz.jar")

	android.AssertStringDoesContain(t, "foo classpath", javac.Args["classpath"], barTurbine)

	android.AssertStringDoesContain(t, "foo classpath", javac.Args["classpath"], bazTurbine)

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != baz {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, baz)
	}
}

func TestExportedPlugins(t *testing.T) {
	type Result struct {
		library        string
		processors     string
		disableTurbine bool
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
		{
			name: "Exports plugin to with generates_api to dependee",
			extra: `
				java_library{name: "exports", exported_plugins: ["plugin_generates_api"]}
				java_library{name: "foo", srcs: ["a.java"], libs: ["exports"]}
				java_library{name: "bar", srcs: ["a.java"], static_libs: ["exports"]}
			`,
			results: []Result{
				{library: "foo", processors: "-processor com.android.TestPlugin", disableTurbine: true},
				{library: "bar", processors: "-processor com.android.TestPlugin", disableTurbine: true},
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
				java_plugin {
					name: "plugin_generates_api",
					generates_api: true,
					processor_class: "com.android.TestPlugin",
				}
			`+test.extra)

			for _, want := range test.results {
				javac := ctx.ModuleForTests(want.library, "android_common").Rule("javac")
				if javac.Args["processor"] != want.processors {
					t.Errorf("For library %v, expected %v, found %v", want.library, want.processors, javac.Args["processor"])
				}
				turbine := ctx.ModuleForTests(want.library, "android_common").MaybeRule("turbine")
				disableTurbine := turbine.BuildParams.Rule == nil
				if disableTurbine != want.disableTurbine {
					t.Errorf("For library %v, expected disableTurbine %v, found %v", want.library, want.disableTurbine, disableTurbine)
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

		errorHandler := android.FixtureExpectsNoErrors
		if enforce {
			errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern("sdk_version must have a value when the module is located at vendor or product")
		}

		android.GroupFixturePreparers(
			PrepareForTestWithJavaDefaultModules,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.EnforceProductPartitionInterface = proptools.BoolPtr(enforce)
			}),
		).
			ExtendWithErrorHandler(errorHandler).
			RunTestWithBp(t, bp)
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
			jni_libs: ["libjni"],
		}

		cc_library_shared {
			name: "libjni",
			host_supported: true,
			device_supported: false,
			stl: "none",
		}
	`)

	buildOS := ctx.Config().BuildOS.String()

	bar := ctx.ModuleForTests("bar", buildOS+"_common")
	barJar := bar.Output("bar.jar").Output.String()
	barWrapper := ctx.ModuleForTests("bar", buildOS+"_x86_64")
	barWrapperDeps := barWrapper.Output("bar").Implicits.Strings()

	libjni := ctx.ModuleForTests("libjni", buildOS+"_x86_64_shared")
	libjniSO := libjni.Rule("Cp").Output.String()

	// Test that the install binary wrapper depends on the installed jar file
	if g, w := barWrapperDeps, barJar; !android.InList(w, g) {
		t.Errorf("expected binary wrapper implicits to contain %q, got %q", w, g)
	}

	// Test that the install binary wrapper depends on the installed JNI libraries
	if g, w := barWrapperDeps, libjniSO; !android.InList(w, g) {
		t.Errorf("expected binary wrapper implicits to contain %q, got %q", w, g)
	}
}

func TestTest(t *testing.T) {
	ctx, _ := testJava(t, `
		java_test_host {
			name: "foo",
			srcs: ["a.java"],
			jni_libs: ["libjni"],
		}

		cc_library_shared {
			name: "libjni",
			host_supported: true,
			device_supported: false,
			stl: "none",
		}
	`)

	buildOS := ctx.Config().BuildOS.String()

	foo := ctx.ModuleForTests("foo", buildOS+"_common").Module().(*TestHost)

	expected := "lib64/libjni.so"
	if runtime.GOOS == "darwin" {
		expected = "lib64/libjni.dylib"
	}

	fooTestData := foo.data
	if len(fooTestData) != 1 || fooTestData[0].Rel() != expected {
		t.Errorf(`expected foo test data relative path [%q], got %q`,
			expected, fooTestData.Strings())
	}
}

func TestHostBinaryNoJavaDebugInfoOverride(t *testing.T) {
	bp := `
		java_library {
			name: "target_library",
			srcs: ["a.java"],
		}

		java_binary_host {
			name: "host_binary",
			srcs: ["b.java"],
		}
	`

	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.MinimizeJavaDebugInfo = proptools.BoolPtr(true)
		}),
	).RunTestWithBp(t, bp)

	// first, check that the -g flag is added to target modules
	targetLibrary := result.ModuleForTests("target_library", "android_common")
	targetJavaFlags := targetLibrary.Module().VariablesForTests()["javacFlags"]
	if !strings.Contains(targetJavaFlags, "-g:source,lines") {
		t.Errorf("target library javac flags %v should contain "+
			"-g:source,lines override with MinimizeJavaDebugInfo", targetJavaFlags)
	}

	// check that -g is not overridden for host modules
	buildOS := result.Config.BuildOS.String()
	hostBinary := result.ModuleForTests("host_binary", buildOS+"_common")
	hostJavaFlags := hostBinary.Module().VariablesForTests()["javacFlags"]
	if strings.Contains(hostJavaFlags, "-g:source,lines") {
		t.Errorf("java_binary_host javac flags %v should not have "+
			"-g:source,lines override with MinimizeJavaDebugInfo", hostJavaFlags)
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
			sdk_version: "current",
			compile_dex: true,
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
	barModule := ctx.ModuleForTests("bar", "android_common")
	barJar := barModule.Rule("combineJar").Output
	bazModule := ctx.ModuleForTests("baz", "android_common")
	bazJar := bazModule.Rule("combineJar").Output
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

	barDexJar := barModule.Module().(*Import).DexJarBuildPath()
	if barDexJar.IsSet() {
		t.Errorf("bar dex jar build path expected to be set, got %s", barDexJar)
	}

	if !strings.Contains(javac.Args["classpath"], sdklibStubsJar.String()) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], sdklibStubsJar.String())
	}

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != bazJar.String() {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, bazJar.String())
	}

	bazDexJar := bazModule.Module().(*Import).DexJarBuildPath().Path()
	expectedDexJar := "out/soong/.intermediates/baz/android_common/dex/baz.jar"
	android.AssertPathRelativeToTopEquals(t, "baz dex jar build path", expectedDexJar, bazDexJar)

	ctx.ModuleForTests("qux", "android_common").Rule("Cp")
}

func assertDeepEquals(t *testing.T, message string, expected interface{}, actual interface{}) {
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("%s: expected %q, found %q", message, expected, actual)
	}
}

func TestPrebuiltStubsSources(t *testing.T) {
	test := func(t *testing.T, sourcesPath string, expectedInputs []string) {
		ctx, _ := testJavaWithFS(t, fmt.Sprintf(`
prebuilt_stubs_sources {
  name: "stubs-source",
	srcs: ["%s"],
}`, sourcesPath), map[string][]byte{
			"stubs/sources/pkg/A.java": nil,
			"stubs/sources/pkg/B.java": nil,
		})

		zipSrc := ctx.ModuleForTests("stubs-source", "android_common").Rule("zip_src")
		if expected, actual := expectedInputs, zipSrc.Inputs.Strings(); !reflect.DeepEqual(expected, actual) {
			t.Errorf("mismatch of inputs to soong_zip: expected %q, actual %q", expected, actual)
		}
	}

	t.Run("empty/missing directory", func(t *testing.T) {
		test(t, "empty-directory", nil)
	})

	t.Run("non-empty set of sources", func(t *testing.T) {
		test(t, "stubs/sources", []string{
			"stubs/sources/pkg/A.java",
			"stubs/sources/pkg/B.java",
		})
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

	barTurbine := filepath.Join("out", "soong", ".intermediates", "bar", "android_common", "turbine-combined", "bar.jar")
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
	result := android.GroupFixturePreparers(
		prepareForJavaTest, FixtureWithPrebuiltApis(map[string][]string{"14": {"foo"}})).
		RunTestWithBp(t, `
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

	fooTurbine := result.ModuleForTests("foo", "android_common").Rule("turbine")
	barTurbine := result.ModuleForTests("bar", "android_common").Rule("turbine")
	barJavac := result.ModuleForTests("bar", "android_common").Rule("javac")
	barTurbineCombined := result.ModuleForTests("bar", "android_common").Description("for turbine")
	bazJavac := result.ModuleForTests("baz", "android_common").Rule("javac")

	android.AssertPathsRelativeToTopEquals(t, "foo inputs", []string{"a.java"}, fooTurbine.Inputs)

	fooHeaderJar := filepath.Join("out", "soong", ".intermediates", "foo", "android_common", "turbine-combined", "foo.jar")
	barTurbineJar := filepath.Join("out", "soong", ".intermediates", "bar", "android_common", "turbine", "bar.jar")
	android.AssertStringDoesContain(t, "bar turbine classpath", barTurbine.Args["classpath"], fooHeaderJar)
	android.AssertStringDoesContain(t, "bar javac classpath", barJavac.Args["classpath"], fooHeaderJar)
	android.AssertPathsRelativeToTopEquals(t, "bar turbine combineJar", []string{barTurbineJar, fooHeaderJar}, barTurbineCombined.Inputs)
	android.AssertStringDoesContain(t, "baz javac classpath", bazJavac.Args["classpath"], "prebuilts/sdk/14/public/android.jar")
}

func TestSharding(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "bar",
			srcs: ["a.java","b.java","c.java"],
			javac_shard_size: 1
		}
		`)

	barHeaderJar := filepath.Join("out", "soong", ".intermediates", "bar", "android_common", "turbine", "bar.jar")
	for i := 0; i < 3; i++ {
		barJavac := ctx.ModuleForTests("bar", "android_common").Description("javac" + strconv.Itoa(i))
		if !strings.HasPrefix(barJavac.Args["classpath"], "-classpath "+barHeaderJar+":") {
			t.Errorf("bar javac classpath %v does start with %q", barJavac.Args["classpath"], barHeaderJar)
		}
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
	testJavaWithFS(t, "", map[string][]byte{
		"libcore/Android.bp": []byte(`
				java_library {
						name: "core",
						sdk_version: "none",
						system_modules: "none",
				}

				filegroup {
					name: "core-jar",
					srcs: [":core{.jar}"],
				}
		`),
	})
}

func TestJavaImport(t *testing.T) {
	testJavaWithFS(t, "", map[string][]byte{
		"libcore/Android.bp": []byte(`
				java_import {
						name: "core",
						sdk_version: "none",
				}

				filegroup {
					name: "core-jar",
					srcs: [":core{.jar}"],
				}
		`),
	})
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
	variables := ctx.ModuleForTests(moduleName, "android_common").VariablesForTestsRelativeToTop()
	flags := strings.Split(variables["javacFlags"], " ")
	got := ""
	for _, flag := range flags {
		keyEnd := strings.Index(flag, "=")
		if keyEnd > -1 && flag[:keyEnd] == "--patch-module" {
			got = flag[keyEnd+1:]
			break
		}
	}
	if expected != android.StringPathRelativeToTop(ctx.Config().SoongOutDir(), got) {
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
				srcs: [
					"c.java",
					// Tests for b/150878007
					"dir/d.java",
					"dir2/e.java",
					"dir2/f.java",
					"nested/dir/g.java"
				],
				patch_module: "java.base",
			}
		`
		ctx, _ := testJava(t, bp)

		checkPatchModuleFlag(t, ctx, "foo", "")
		expected := "java.base=.:out/soong"
		checkPatchModuleFlag(t, ctx, "bar", expected)
		expected = "java.base=" + strings.Join([]string{
			".", "out/soong", "dir", "dir2", "nested", defaultModuleToPath("ext"), defaultModuleToPath("framework")}, ":")
		checkPatchModuleFlag(t, ctx, "baz", expected)
	})
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

func TestAidlExportIncludeDirsFromImports(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["aidl/foo/IFoo.aidl"],
			libs: ["bar"],
		}

		java_import {
			name: "bar",
			jars: ["a.jar"],
			aidl: {
				export_include_dirs: ["aidl/bar"],
			},
		}
	`)

	aidlCommand := ctx.ModuleForTests("foo", "android_common").Rule("aidl").RuleParams.Command
	expectedAidlFlag := "-Iaidl/bar"
	if !strings.Contains(aidlCommand, expectedAidlFlag) {
		t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlag)
	}
}

func TestAidlFlagsArePassedToTheAidlCompiler(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["aidl/foo/IFoo.aidl"],
			aidl: { flags: ["-Werror"], },
		}
	`)

	aidlCommand := ctx.ModuleForTests("foo", "android_common").Rule("aidl").RuleParams.Command
	expectedAidlFlag := "-Werror"
	if !strings.Contains(aidlCommand, expectedAidlFlag) {
		t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlag)
	}
}

func TestAidlFlagsWithMinSdkVersion(t *testing.T) {
	fixture := android.GroupFixturePreparers(
		prepareForJavaTest, FixtureWithPrebuiltApis(map[string][]string{"14": {"foo"}}))

	for _, tc := range []struct {
		name       string
		sdkVersion string
		expected   string
	}{
		{"default is current", "", "current"},
		{"use sdk_version", `sdk_version: "14"`, "14"},
		{"system_current", `sdk_version: "system_current"`, "current"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := fixture.RunTestWithBp(t, `
				java_library {
					name: "foo",
					srcs: ["aidl/foo/IFoo.aidl"],
					`+tc.sdkVersion+`
				}
			`)
			aidlCommand := ctx.ModuleForTests("foo", "android_common").Rule("aidl").RuleParams.Command
			expectedAidlFlag := "--min_sdk_version=" + tc.expected
			if !strings.Contains(aidlCommand, expectedAidlFlag) {
				t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlag)
			}
		})
	}
}

func TestAidlEnforcePermissions(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["aidl/foo/IFoo.aidl"],
			aidl: { enforce_permissions: true },
		}
	`)

	aidlCommand := ctx.ModuleForTests("foo", "android_common").Rule("aidl").RuleParams.Command
	expectedAidlFlag := "-Wmissing-permission-annotation -Werror"
	if !strings.Contains(aidlCommand, expectedAidlFlag) {
		t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlag)
	}
}

func TestAidlEnforcePermissionsException(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["aidl/foo/IFoo.aidl", "aidl/foo/IFoo2.aidl"],
			aidl: { enforce_permissions: true, enforce_permissions_exceptions: ["aidl/foo/IFoo2.aidl"] },
		}
	`)

	aidlCommand := ctx.ModuleForTests("foo", "android_common").Rule("aidl").RuleParams.Command
	expectedAidlFlag := "$$FLAGS -Wmissing-permission-annotation -Werror aidl/foo/IFoo.aidl"
	if !strings.Contains(aidlCommand, expectedAidlFlag) {
		t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlag)
	}
	expectedAidlFlag = "$$FLAGS  aidl/foo/IFoo2.aidl"
	if !strings.Contains(aidlCommand, expectedAidlFlag) {
		t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlag)
	}
}

func TestDataNativeBinaries(t *testing.T) {
	ctx, _ := testJava(t, `
		java_test_host {
			name: "foo",
			srcs: ["a.java"],
			data_native_bins: ["bin"]
		}

		python_binary_host {
			name: "bin",
			srcs: ["bin.py"],
		}
	`)

	buildOS := ctx.Config().BuildOS.String()

	test := ctx.ModuleForTests("foo", buildOS+"_common").Module().(*TestHost)
	entries := android.AndroidMkEntriesForTest(t, ctx, test)[0]
	expected := []string{"out/soong/.intermediates/bin/" + buildOS + "_x86_64_PY3/bin:bin"}
	actual := entries.EntryMap["LOCAL_COMPATIBILITY_SUPPORT_FILES"]
	android.AssertStringPathsRelativeToTopEquals(t, "LOCAL_COMPATIBILITY_SUPPORT_FILES", ctx.Config(), expected, actual)
}

func TestDefaultInstallable(t *testing.T) {
	ctx, _ := testJava(t, `
		java_test_host {
			name: "foo"
		}
	`)

	buildOS := ctx.Config().BuildOS.String()
	module := ctx.ModuleForTests("foo", buildOS+"_common").Module().(*TestHost)
	assertDeepEquals(t, "Default installable value should be true.", proptools.BoolPtr(true),
		module.properties.Installable)
}

func TestErrorproneEnabled(t *testing.T) {
	ctx, _ := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			errorprone: {
				enabled: true,
			},
		}
	`)

	javac := ctx.ModuleForTests("foo", "android_common").Description("javac")

	// Test that the errorprone plugins are passed to javac
	expectedSubstring := "-Xplugin:ErrorProne"
	if !strings.Contains(javac.Args["javacFlags"], expectedSubstring) {
		t.Errorf("expected javacFlags to contain %q, got %q", expectedSubstring, javac.Args["javacFlags"])
	}

	// Modules with errorprone { enabled: true } will include errorprone checks
	// in the main javac build rule. Only when RUN_ERROR_PRONE is true will
	// the explicit errorprone build rule be created.
	errorprone := ctx.ModuleForTests("foo", "android_common").MaybeDescription("errorprone")
	if errorprone.RuleParams.Description != "" {
		t.Errorf("expected errorprone build rule to not exist, but it did")
	}
}

func TestErrorproneDisabled(t *testing.T) {
	bp := `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			errorprone: {
				enabled: false,
			},
		}
	`
	ctx := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.FixtureMergeEnv(map[string]string{
			"RUN_ERROR_PRONE": "true",
		}),
	).RunTestWithBp(t, bp)

	javac := ctx.ModuleForTests("foo", "android_common").Description("javac")

	// Test that the errorprone plugins are not passed to javac, like they would
	// be if enabled was true.
	expectedSubstring := "-Xplugin:ErrorProne"
	if strings.Contains(javac.Args["javacFlags"], expectedSubstring) {
		t.Errorf("expected javacFlags to not contain %q, got %q", expectedSubstring, javac.Args["javacFlags"])
	}

	// Check that no errorprone build rule is created, like there would be
	// if enabled was unset and RUN_ERROR_PRONE was true.
	errorprone := ctx.ModuleForTests("foo", "android_common").MaybeDescription("errorprone")
	if errorprone.RuleParams.Description != "" {
		t.Errorf("expected errorprone build rule to not exist, but it did")
	}
}

func TestErrorproneEnabledOnlyByEnvironmentVariable(t *testing.T) {
	bp := `
		java_library {
			name: "foo",
			srcs: ["a.java"],
		}
	`
	ctx := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		android.FixtureMergeEnv(map[string]string{
			"RUN_ERROR_PRONE": "true",
		}),
	).RunTestWithBp(t, bp)

	javac := ctx.ModuleForTests("foo", "android_common").Description("javac")
	errorprone := ctx.ModuleForTests("foo", "android_common").Description("errorprone")

	// Check that the errorprone plugins are not passed to javac, because they
	// will instead be passed to the separate errorprone compilation
	expectedSubstring := "-Xplugin:ErrorProne"
	if strings.Contains(javac.Args["javacFlags"], expectedSubstring) {
		t.Errorf("expected javacFlags to not contain %q, got %q", expectedSubstring, javac.Args["javacFlags"])
	}

	// Check that the errorprone plugin is enabled
	if !strings.Contains(errorprone.Args["javacFlags"], expectedSubstring) {
		t.Errorf("expected errorprone to contain %q, got %q", expectedSubstring, javac.Args["javacFlags"])
	}
}

func TestDataDeviceBinsBuildsDeviceBinary(t *testing.T) {
	testCases := []struct {
		dataDeviceBinType  string
		depCompileMultilib string
		variants           []string
		expectedError      string
	}{
		{
			dataDeviceBinType:  "first",
			depCompileMultilib: "first",
			variants:           []string{"android_arm64_armv8-a"},
		},
		{
			dataDeviceBinType:  "first",
			depCompileMultilib: "both",
			variants:           []string{"android_arm64_armv8-a"},
		},
		{
			// this is true because our testing framework is set up with
			// Targets ~ [<64bit target>, <32bit target>], where 64bit is "first"
			dataDeviceBinType:  "first",
			depCompileMultilib: "32",
			expectedError:      `Android.bp:2:3: dependency "bar" of "foo" missing variant`,
		},
		{
			dataDeviceBinType:  "first",
			depCompileMultilib: "64",
			variants:           []string{"android_arm64_armv8-a"},
		},
		{
			dataDeviceBinType:  "both",
			depCompileMultilib: "both",
			variants: []string{
				"android_arm_armv7-a-neon",
				"android_arm64_armv8-a",
			},
		},
		{
			dataDeviceBinType:  "both",
			depCompileMultilib: "32",
			expectedError:      `Android.bp:2:3: dependency "bar" of "foo" missing variant`,
		},
		{
			dataDeviceBinType:  "both",
			depCompileMultilib: "64",
			expectedError:      `Android.bp:2:3: dependency "bar" of "foo" missing variant`,
		},
		{
			dataDeviceBinType:  "both",
			depCompileMultilib: "first",
			expectedError:      `Android.bp:2:3: dependency "bar" of "foo" missing variant`,
		},
		{
			dataDeviceBinType:  "32",
			depCompileMultilib: "32",
			variants:           []string{"android_arm_armv7-a-neon"},
		},
		{
			dataDeviceBinType:  "32",
			depCompileMultilib: "first",
			expectedError:      `Android.bp:2:3: dependency "bar" of "foo" missing variant`,
		},
		{
			dataDeviceBinType:  "32",
			depCompileMultilib: "both",
			variants:           []string{"android_arm_armv7-a-neon"},
		},
		{
			dataDeviceBinType:  "32",
			depCompileMultilib: "64",
			expectedError:      `Android.bp:2:3: dependency "bar" of "foo" missing variant`,
		},
		{
			dataDeviceBinType:  "64",
			depCompileMultilib: "64",
			variants:           []string{"android_arm64_armv8-a"},
		},
		{
			dataDeviceBinType:  "64",
			depCompileMultilib: "both",
			variants:           []string{"android_arm64_armv8-a"},
		},
		{
			dataDeviceBinType:  "64",
			depCompileMultilib: "first",
			variants:           []string{"android_arm64_armv8-a"},
		},
		{
			dataDeviceBinType:  "64",
			depCompileMultilib: "32",
			expectedError:      `Android.bp:2:3: dependency "bar" of "foo" missing variant`,
		},
		{
			dataDeviceBinType:  "prefer32",
			depCompileMultilib: "32",
			variants:           []string{"android_arm_armv7-a-neon"},
		},
		{
			dataDeviceBinType:  "prefer32",
			depCompileMultilib: "both",
			variants:           []string{"android_arm_armv7-a-neon"},
		},
		{
			dataDeviceBinType:  "prefer32",
			depCompileMultilib: "first",
			expectedError:      `Android.bp:2:3: dependency "bar" of "foo" missing variant`,
		},
		{
			dataDeviceBinType:  "prefer32",
			depCompileMultilib: "64",
			expectedError:      `Android.bp:2:3: dependency "bar" of "foo" missing variant`,
		},
	}

	bpTemplate := `
		java_test_host {
			name: "foo",
			srcs: ["test.java"],
			data_device_bins_%s: ["bar"],
		}

		cc_binary {
			name: "bar",
			compile_multilib: "%s",
		}
	`

	for _, tc := range testCases {
		bp := fmt.Sprintf(bpTemplate, tc.dataDeviceBinType, tc.depCompileMultilib)

		errorHandler := android.FixtureExpectsNoErrors
		if tc.expectedError != "" {
			errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(tc.expectedError)
		}

		testName := fmt.Sprintf(`data_device_bins_%s with compile_multilib:"%s"`, tc.dataDeviceBinType, tc.depCompileMultilib)
		t.Run(testName, func(t *testing.T) {
			ctx := android.GroupFixturePreparers(PrepareForIntegrationTestWithJava).
				ExtendWithErrorHandler(errorHandler).
				RunTestWithBp(t, bp)
			if tc.expectedError != "" {
				return
			}

			buildOS := ctx.Config.BuildOS.String()
			fooVariant := ctx.ModuleForTests("foo", buildOS+"_common")
			fooMod := fooVariant.Module().(*TestHost)
			entries := android.AndroidMkEntriesForTest(t, ctx.TestContext, fooMod)[0]

			expectedAutogenConfig := `<option name="push-file" key="bar" value="/data/local/tests/unrestricted/foo/bar" />`
			autogen := fooVariant.Rule("autogen")
			if !strings.Contains(autogen.Args["extraConfigs"], expectedAutogenConfig) {
				t.Errorf("foo extraConfigs %v does not contain %q", autogen.Args["extraConfigs"], expectedAutogenConfig)
			}

			expectedData := []string{}
			for _, variant := range tc.variants {
				barVariant := ctx.ModuleForTests("bar", variant)
				relocated := barVariant.Output("bar")
				expectedInput := fmt.Sprintf("out/soong/.intermediates/bar/%s/unstripped/bar", variant)
				android.AssertPathRelativeToTopEquals(t, "relocation input", expectedInput, relocated.Input)

				expectedData = append(expectedData, fmt.Sprintf("out/soong/.intermediates/bar/%s/bar:bar", variant))
			}

			actualData := entries.EntryMap["LOCAL_COMPATIBILITY_SUPPORT_FILES"]
			android.AssertStringPathsRelativeToTopEquals(t, "LOCAL_TEST_DATA", ctx.Config, expectedData, actualData)
		})
	}
}
