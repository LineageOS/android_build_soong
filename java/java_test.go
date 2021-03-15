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
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"android/soong/genrule"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/dexpreopt"
	"android/soong/python"
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

// Factory to use to create fixtures for tests in this package.
var javaFixtureFactory = android.NewFixtureFactory(
	&buildDir,
	genrule.PrepareForTestWithGenRuleBuildComponents,
	// Get the CC build components but not default modules.
	cc.PrepareForTestWithCcBuildComponents,
	// Include all the default java modules.
	PrepareForTestWithJavaDefaultModules,
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("java_plugin", PluginFactory)
		ctx.RegisterModuleType("python_binary_host", python.PythonBinaryHostFactory)

		ctx.PreDepsMutators(python.RegisterPythonPreDepsMutators)
		ctx.RegisterPreSingletonType("overlay", OverlaySingletonFactory)
		ctx.RegisterPreSingletonType("sdk_versions", sdkPreSingletonFactory)
	}),
	javaMockFS().AddToFixture(),
	PrepareForTestWithJavaSdkLibraryFiles,
	dexpreopt.PrepareForTestWithDexpreopt,
)

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}

// testConfig is a legacy way of creating a test Config for testing java modules.
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testConfig(env map[string]string, bp string, fs map[string][]byte) android.Config {
	return TestConfig(buildDir, env, bp, fs)
}

// testContext is a legacy way of creating a TestContext for testing java modules.
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testContext(config android.Config) *android.TestContext {

	ctx := android.NewTestArchContext(config)
	RegisterRequiredBuildComponentsForTest(ctx)
	ctx.RegisterModuleType("java_plugin", PluginFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("python_binary_host", python.PythonBinaryHostFactory)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.PreArchMutators(android.RegisterComponentsMutator)

	ctx.PreDepsMutators(python.RegisterPythonPreDepsMutators)
	ctx.PostDepsMutators(android.RegisterOverridePostDepsMutators)
	ctx.RegisterPreSingletonType("overlay", OverlaySingletonFactory)
	ctx.RegisterPreSingletonType("sdk_versions", sdkPreSingletonFactory)

	android.RegisterPrebuiltMutators(ctx)

	genrule.RegisterGenruleBuildComponents(ctx)

	// Register module types and mutators from cc needed for JNI testing
	cc.RegisterRequiredBuildComponentsForTest(ctx)

	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("propagate_rro_enforcement", propagateRROEnforcementMutator).Parallel()
	})

	return ctx
}

// run is a legacy way of running tests of java modules.
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func run(t *testing.T, ctx *android.TestContext, config android.Config) {
	t.Helper()

	pathCtx := android.PathContextForTesting(config)
	dexpreopt.SetTestGlobalConfig(config, dexpreopt.GlobalConfigForTests(pathCtx))

	ctx.Register()
	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)
}

// testJavaError is a legacy way of running tests of java modules that expect errors.
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testJavaError(t *testing.T, pattern string, bp string) (*android.TestContext, android.Config) {
	t.Helper()
	result := javaFixtureFactory.
		Extend(dexpreopt.PrepareForTestWithDexpreopt).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(pattern)).
		RunTestWithBp(t, bp)
	return result.TestContext, result.Config
}

// testJavaErrorWithConfig is a legacy way of running tests of java modules that expect errors.
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testJavaErrorWithConfig(t *testing.T, pattern string, config android.Config) (*android.TestContext, android.Config) {
	t.Helper()
	// This must be done on the supplied config and not as part of the fixture because any changes to
	// the fixture's config will be ignored when RunTestWithConfig replaces it.
	pathCtx := android.PathContextForTesting(config)
	dexpreopt.SetTestGlobalConfig(config, dexpreopt.GlobalConfigForTests(pathCtx))
	result := javaFixtureFactory.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(pattern)).
		RunTestWithConfig(t, config)
	return result.TestContext, result.Config
}

// runWithErrors is a legacy way of running tests of java modules that expect errors.
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func runWithErrors(t *testing.T, ctx *android.TestContext, config android.Config, pattern string) {
	ctx.Register()
	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}
	_, errs = ctx.PrepareBuildActions(config)
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}

	t.Fatalf("missing expected error %q (0 errors are returned)", pattern)
	return
}

// testJavaWithFS runs tests using the javaFixtureFactory
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testJavaWithFS(t *testing.T, bp string, fs android.MockFS) (*android.TestContext, android.Config) {
	t.Helper()
	result := javaFixtureFactory.Extend(fs.AddToFixture()).RunTestWithBp(t, bp)
	return result.TestContext, result.Config
}

// testJava runs tests using the javaFixtureFactory
//
// Do not add any new usages of this, instead use the javaFixtureFactory directly as it makes it
// much easier to customize the test behavior.
//
// If it is necessary to customize the behavior of an existing test that uses this then please first
// convert the test to using javaFixtureFactory first and then in a following change add the
// appropriate fixture preparers. Keeping the conversion change separate makes it easy to verify
// that it did not change the test behavior unexpectedly.
//
// deprecated
func testJava(t *testing.T, bp string) (*android.TestContext, android.Config) {
	t.Helper()
	result := javaFixtureFactory.RunTestWithBp(t, bp)
	return result.TestContext, result.Config
}

// testJavaWithConfig runs tests using the javaFixtureFactory
//
// See testJava for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testJavaWithConfig(t *testing.T, config android.Config) (*android.TestContext, android.Config) {
	t.Helper()
	result := javaFixtureFactory.RunTestWithConfig(t, config)
	return result.TestContext, result.Config
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

// defaultModuleToPath constructs a path to the turbine generate jar for a default test module that
// is defined in PrepareForIntegrationTestWithJava
func defaultModuleToPath(name string) string {
	return filepath.Join(buildDir, ".intermediates", defaultJavaDir, name, "android_common", "turbine-combined", name+".jar")
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
			jni_libs: ["libjni"],
		}

		cc_library_shared {
			name: "libjni",
			host_supported: true,
			device_supported: false,
			stl: "none",
		}
	`)

	buildOS := android.BuildOs.String()

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
	config := testConfig(nil, bp, nil)
	config.TestProductVariables.MinimizeJavaDebugInfo = proptools.BoolPtr(true)

	ctx, _ := testJavaWithConfig(t, config)

	// first, check that the -g flag is added to target modules
	targetLibrary := ctx.ModuleForTests("target_library", "android_common")
	targetJavaFlags := targetLibrary.Module().VariablesForTests()["javacFlags"]
	if !strings.Contains(targetJavaFlags, "-g:source,lines") {
		t.Errorf("target library javac flags %v should contain "+
			"-g:source,lines override with MinimizeJavaDebugInfo", targetJavaFlags)
	}

	// check that -g is not overridden for host modules
	buildOS := android.BuildOs.String()
	hostBinary := ctx.ModuleForTests("host_binary", buildOS+"_common")
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
	if barDexJar != nil {
		t.Errorf("bar dex jar build path expected to be nil, got %q", barDexJar)
	}

	if !strings.Contains(javac.Args["classpath"], sdklibStubsJar.String()) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], sdklibStubsJar.String())
	}

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != bazJar.String() {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, bazJar.String())
	}

	bazDexJar := bazModule.Module().(*Import).DexJarBuildPath().String()
	expectedDexJar := buildDir + "/.intermediates/baz/android_common/dex/baz.jar"
	if bazDexJar != expectedDexJar {
		t.Errorf("baz dex jar build path expected %q, got %q", expectedDexJar, bazDexJar)
	}

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
		test(t, "empty-directory", []string{})
	})

	t.Run("non-empty set of sources", func(t *testing.T) {
		test(t, "stubs/sources", []string{
			"stubs/sources/pkg/A.java",
			"stubs/sources/pkg/B.java",
		})
	})
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

	CheckModuleDependencies(t, ctx, "sdklib", "android_common", []string{
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

	CheckModuleDependencies(t, ctx, "sdklib", "android_common", []string{
		`dex2oatd`,
		`prebuilt_sdklib`,
		`sdklib.impl`,
		`sdklib.stubs`,
		`sdklib.stubs.source`,
		`sdklib.xml`,
	})

	CheckModuleDependencies(t, ctx, "prebuilt_sdklib", "android_common", []string{
		`prebuilt_sdklib.stubs`,
		`sdklib.impl`,
		// This should be prebuilt_sdklib.stubs but is set to sdklib.stubs because the
		// dependency is added after prebuilts may have been renamed and so has to use
		// the renamed name.
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

	CheckModuleDependencies(t, ctx, "sdklib", "android_common", []string{
		`dex2oatd`,
		`prebuilt_sdklib`,
		`sdklib.impl`,
		`sdklib.stubs`,
		`sdklib.stubs.source`,
		`sdklib.xml`,
	})

	CheckModuleDependencies(t, ctx, "prebuilt_sdklib", "android_common", []string{
		`prebuilt_sdklib.stubs`,
		`sdklib.impl`,
		`sdklib.xml`,
	})
}

func TestJavaSdkLibraryEnforce(t *testing.T) {
	partitionToBpOption := func(partition string) string {
		switch partition {
		case "system":
			return ""
		case "vendor":
			return "soc_specific: true,"
		case "product":
			return "product_specific: true,"
		default:
			panic("Invalid partition group name: " + partition)
		}
	}

	type testConfigInfo struct {
		libraryType                string
		fromPartition              string
		toPartition                string
		enforceVendorInterface     bool
		enforceProductInterface    bool
		enforceJavaSdkLibraryCheck bool
		allowList                  []string
	}

	createTestConfig := func(info testConfigInfo) android.Config {
		bpFileTemplate := `
			java_library {
				name: "foo",
				srcs: ["foo.java"],
				libs: ["bar"],
				sdk_version: "current",
				%s
			}

			%s {
				name: "bar",
				srcs: ["bar.java"],
				sdk_version: "current",
				%s
			}
		`

		bpFile := fmt.Sprintf(bpFileTemplate,
			partitionToBpOption(info.fromPartition),
			info.libraryType,
			partitionToBpOption(info.toPartition))

		config := testConfig(nil, bpFile, nil)
		configVariables := config.TestProductVariables

		configVariables.EnforceProductPartitionInterface = proptools.BoolPtr(info.enforceProductInterface)
		if info.enforceVendorInterface {
			configVariables.DeviceVndkVersion = proptools.StringPtr("current")
		}
		configVariables.EnforceInterPartitionJavaSdkLibrary = proptools.BoolPtr(info.enforceJavaSdkLibraryCheck)
		configVariables.InterPartitionJavaLibraryAllowList = info.allowList

		return config
	}

	errorMessage := "is not allowed across the partitions"

	testJavaWithConfig(t, createTestConfig(testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "product",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: false,
	}))

	testJavaWithConfig(t, createTestConfig(testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "product",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    false,
		enforceJavaSdkLibraryCheck: true,
	}))

	testJavaErrorWithConfig(t, errorMessage, createTestConfig(testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "product",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}))

	testJavaErrorWithConfig(t, errorMessage, createTestConfig(testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "vendor",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}))

	testJavaWithConfig(t, createTestConfig(testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "vendor",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
		allowList:                  []string{"bar"},
	}))

	testJavaErrorWithConfig(t, errorMessage, createTestConfig(testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "vendor",
		toPartition:                "product",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}))

	testJavaWithConfig(t, createTestConfig(testConfigInfo{
		libraryType:                "java_sdk_library",
		fromPartition:              "product",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}))

	testJavaWithConfig(t, createTestConfig(testConfigInfo{
		libraryType:                "java_sdk_library",
		fromPartition:              "vendor",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}))

	testJavaWithConfig(t, createTestConfig(testConfigInfo{
		libraryType:                "java_sdk_library",
		fromPartition:              "vendor",
		toPartition:                "product",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}))
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

func TestJavaLint(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		java_library {
			name: "foo",
			srcs: [
				"a.java",
				"b.java",
				"c.java",
			],
			min_sdk_version: "29",
			sdk_version: "system_current",
		}
       `, map[string][]byte{
		"lint-baseline.xml": nil,
	})

	foo := ctx.ModuleForTests("foo", "android_common")
	rule := foo.Rule("lint")

	if !strings.Contains(rule.RuleParams.Command, "--baseline lint-baseline.xml") {
		t.Error("did not pass --baseline flag")
	}
}

func TestJavaLintWithoutBaseline(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		java_library {
			name: "foo",
			srcs: [
				"a.java",
				"b.java",
				"c.java",
			],
			min_sdk_version: "29",
			sdk_version: "system_current",
		}
       `, map[string][]byte{})

	foo := ctx.ModuleForTests("foo", "android_common")
	rule := foo.Rule("lint")

	if strings.Contains(rule.RuleParams.Command, "--baseline") {
		t.Error("passed --baseline flag for non existent file")
	}
}

func TestJavaLintRequiresCustomLintFileToExist(t *testing.T) {
	config := testConfig(
		nil,
		`
		java_library {
			name: "foo",
			srcs: [
			],
			min_sdk_version: "29",
			sdk_version: "system_current",
			lint: {
				baseline_filename: "mybaseline.xml",
			},
		}
     `, map[string][]byte{
			"build/soong/java/lint_defaults.txt":                   nil,
			"prebuilts/cmdline-tools/tools/bin/lint":               nil,
			"prebuilts/cmdline-tools/tools/lib/lint-classpath.jar": nil,
			"framework/aidl":                     nil,
			"a.java":                             nil,
			"AndroidManifest.xml":                nil,
			"build/make/target/product/security": nil,
		})
	config.TestAllowNonExistentPaths = false
	testJavaErrorWithConfig(t,
		"source path \"mybaseline.xml\" does not exist",
		config,
	)
}

func TestJavaLintUsesCorrectBpConfig(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		java_library {
			name: "foo",
			srcs: [
				"a.java",
				"b.java",
				"c.java",
			],
			min_sdk_version: "29",
			sdk_version: "system_current",
			lint: {
				error_checks: ["SomeCheck"],
				baseline_filename: "mybaseline.xml",
			},
		}
       `, map[string][]byte{
		"mybaseline.xml": nil,
	})

	foo := ctx.ModuleForTests("foo", "android_common")
	rule := foo.Rule("lint")

	if !strings.Contains(rule.RuleParams.Command, "--baseline mybaseline.xml") {
		t.Error("did not use the correct file for baseline")
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
		droidstubs {
		    name: "bar-stubs",
		    srcs: [
		        "bar-doc/a.java",
		    ],
		    exclude_srcs: [
		        "bar-doc/b.java"
		    ],
		    api_levels_annotations_dirs: [
		      "droiddoc-templates-sdk",
		    ],
		    api_levels_annotations_enabled: true,
		}
		droiddoc {
		    name: "bar-doc",
		    srcs: [
		        ":bar-stubs",
		        "bar-doc/IFoo.aidl",
		        ":bar-doc-aidl-srcs",
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
		    flags: ["-offlinemode -title \"libcore\""],
		}
		`,
		map[string][]byte{
			"bar-doc/a.java": nil,
			"bar-doc/b.java": nil,
		})
	barStubs := ctx.ModuleForTests("bar-stubs", "android_common")
	barStubsOutputs, err := barStubs.Module().(*Droidstubs).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error %q retrieving \"bar-stubs\" output file", err)
	}
	if len(barStubsOutputs) != 1 {
		t.Errorf("Expected one output from \"bar-stubs\" got %s", barStubsOutputs)
	}

	barStubsOutput := barStubsOutputs[0]
	barDoc := ctx.ModuleForTests("bar-doc", "android_common")
	javaDoc := barDoc.Rule("javadoc")
	if g, w := javaDoc.Implicits.Strings(), barStubsOutput.String(); !inList(w, g) {
		t.Errorf("implicits of bar-doc must contain %q, but was %q.", w, g)
	}

	expected := "-sourcepath " + buildDir + "/.intermediates/bar-doc/android_common/srcjars "
	if !strings.Contains(javaDoc.RuleParams.Command, expected) {
		t.Errorf("bar-doc command does not contain flag %q, but should\n%q", expected, javaDoc.RuleParams.Command)
	}

	aidl := barDoc.Rule("aidl")
	if g, w := javaDoc.Implicits.Strings(), aidl.Output.String(); !inList(w, g) {
		t.Errorf("implicits of bar-doc must contain %q, but was %q.", w, g)
	}

	if g, w := aidl.Implicits.Strings(), []string{"bar-doc/IBar.aidl", "bar-doc/IFoo.aidl"}; !reflect.DeepEqual(w, g) {
		t.Errorf("aidl inputs must be %q, but was %q", w, g)
	}
}

func TestDroiddocArgsAndFlagsCausesError(t *testing.T) {
	testJavaError(t, "flags is set. Cannot set args", `
		droiddoc_exported_dir {
		    name: "droiddoc-templates-sdk",
		    path: ".",
		}
		filegroup {
		    name: "bar-doc-aidl-srcs",
		    srcs: ["bar-doc/IBar.aidl"],
		    path: "bar-doc",
		}
		droidstubs {
		    name: "bar-stubs",
		    srcs: [
		        "bar-doc/a.java",
		    ],
		    exclude_srcs: [
		        "bar-doc/b.java"
		    ],
		    api_levels_annotations_dirs: [
		      "droiddoc-templates-sdk",
		    ],
		    api_levels_annotations_enabled: true,
		}
		droiddoc {
		    name: "bar-doc",
		    srcs: [
		        ":bar-stubs",
		        "bar-doc/IFoo.aidl",
		        ":bar-doc-aidl-srcs",
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
		    flags: ["-offlinemode -title \"libcore\""],
		    args: "-offlinemode -title \"libcore\"",
		}
		`)
}

func TestDroidstubs(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		droiddoc_exported_dir {
			name: "droiddoc-templates-sdk",
			path: ".",
		}

		droidstubs {
			name: "bar-stubs",
			srcs: ["bar-doc/a.java"],
			api_levels_annotations_dirs: ["droiddoc-templates-sdk"],
			api_levels_annotations_enabled: true,
		}

		droidstubs {
			name: "bar-stubs-other",
			srcs: ["bar-doc/a.java"],
			high_mem: true,
			api_levels_annotations_dirs: ["droiddoc-templates-sdk"],
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
		high_mem            bool
	}{
		{
			moduleName:          "bar-stubs",
			expectedJarFilename: "android.jar",
			high_mem:            false,
		},
		{
			moduleName:          "bar-stubs-other",
			expectedJarFilename: "android.other.jar",
			high_mem:            true,
		},
	}
	for _, c := range testcases {
		m := ctx.ModuleForTests(c.moduleName, "android_common")
		metalava := m.Rule("metalava")
		rp := metalava.RuleParams
		expected := "--android-jar-pattern ./%/public/" + c.expectedJarFilename
		if actual := rp.Command; !strings.Contains(actual, expected) {
			t.Errorf("For %q, expected metalava argument %q, but was not found %q", c.moduleName, expected, actual)
		}

		if actual := rp.Pool != nil && strings.Contains(rp.Pool.String(), "highmem"); actual != c.high_mem {
			t.Errorf("Expected %q high_mem to be %v, was %v", c.moduleName, c.high_mem, actual)
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

	if g, w := jargen.Implicits.Strings(), foo.Output.String(); !android.InList(w, g) {
		t.Errorf("expected jargen inputs [%q], got %q", w, g)
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
				}

				filegroup {
					name: "core-jar",
					srcs: [":core{.jar}"],
				}
`),
	})
	ctx := testContext(config)
	run(t, ctx, config)
}

func TestJavaImport(t *testing.T) {
	config := testConfig(nil, "", map[string][]byte{
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
	ctx := testContext(config)
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
		java_library {
			name: "baz-module-30",
			srcs: ["c.java"],
			libs: ["foo"],
			sdk_version: "module_30",
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

	bazModule30Javac := ctx.ModuleForTests("baz-module-30", "android_common").Rule("javac")
	// tests if "baz-module-30" is actually linked to the module 30 stubs lib
	if !strings.Contains(bazModule30Javac.Args["classpath"], "prebuilts/sdk/30/module-lib/foo.jar") {
		t.Errorf("baz-module-30 javac classpath %v does not contain %q", bazModule30Javac.Args["classpath"],
			"prebuilts/sdk/30/module-lib/foo.jar")
	}

	// test if baz has exported SDK lib names foo and bar to qux
	qux := ctx.ModuleForTests("qux", "android_common")
	if quxLib, ok := qux.Module().(*Library); ok {
		sdkLibs := quxLib.ClassLoaderContexts().UsesLibs()
		if w := []string{"foo", "bar", "fred", "quuz"}; !reflect.DeepEqual(w, sdkLibs) {
			t.Errorf("qux should export %q but exports %q", w, sdkLibs)
		}
	}
}

func TestJavaSdkLibrary_StubOrImplOnlyLibs(t *testing.T) {
	ctx, _ := testJava(t, `
		java_sdk_library {
			name: "sdklib",
			srcs: ["a.java"],
			impl_only_libs: ["foo"],
			stub_only_libs: ["bar"],
		}
		java_library {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
		}
		java_library {
			name: "bar",
			srcs: ["a.java"],
			sdk_version: "current",
		}
		`)

	for _, implName := range []string{"sdklib", "sdklib.impl"} {
		implJavacCp := ctx.ModuleForTests(implName, "android_common").Rule("javac").Args["classpath"]
		if !strings.Contains(implJavacCp, "/foo.jar") || strings.Contains(implJavacCp, "/bar.jar") {
			t.Errorf("%v javac classpath %v does not contain foo and not bar", implName, implJavacCp)
		}
	}
	stubName := apiScopePublic.stubsLibraryModuleName("sdklib")
	stubsJavacCp := ctx.ModuleForTests(stubName, "android_common").Rule("javac").Args["classpath"]
	if strings.Contains(stubsJavacCp, "/foo.jar") || !strings.Contains(stubsJavacCp, "/bar.jar") {
		t.Errorf("stubs javac classpath %v does not contain bar and not foo", stubsJavacCp)
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

	CheckModuleDependencies(t, ctx, "sdklib", "android_common", []string{
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
		expected := "java.base=.:" + buildDir
		checkPatchModuleFlag(t, ctx, "bar", expected)
		expected = "java.base=" + strings.Join([]string{
			".", buildDir, "dir", "dir2", "nested", defaultModuleToPath("ext"), defaultModuleToPath("framework")}, ":")
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

	buildOS := android.BuildOs.String()

	test := ctx.ModuleForTests("foo", buildOS+"_common").Module().(*TestHost)
	entries := android.AndroidMkEntriesForTest(t, ctx, test)[0]
	expected := []string{buildDir + "/.intermediates/bin/" + buildOS + "_x86_64_PY3/bin:bin"}
	actual := entries.EntryMap["LOCAL_COMPATIBILITY_SUPPORT_FILES"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected test data - expected: %q, actual: %q", expected, actual)
	}
}
