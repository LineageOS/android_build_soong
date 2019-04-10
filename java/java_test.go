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
	"strconv"
	"strings"
	"testing"

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

func testConfig(env map[string]string) android.Config {
	return TestConfig(buildDir, env)
}

func testContext(config android.Config, bp string,
	fs map[string][]byte) *android.TestContext {

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("android_app", android.ModuleFactoryAdaptor(AndroidAppFactory))
	ctx.RegisterModuleType("android_app_certificate", android.ModuleFactoryAdaptor(AndroidAppCertificateFactory))
	ctx.RegisterModuleType("android_library", android.ModuleFactoryAdaptor(AndroidLibraryFactory))
	ctx.RegisterModuleType("android_test", android.ModuleFactoryAdaptor(AndroidTestFactory))
	ctx.RegisterModuleType("android_test_helper_app", android.ModuleFactoryAdaptor(AndroidTestHelperAppFactory))
	ctx.RegisterModuleType("java_binary", android.ModuleFactoryAdaptor(BinaryFactory))
	ctx.RegisterModuleType("java_binary_host", android.ModuleFactoryAdaptor(BinaryHostFactory))
	ctx.RegisterModuleType("java_device_for_host", android.ModuleFactoryAdaptor(DeviceForHostFactory))
	ctx.RegisterModuleType("java_host_for_device", android.ModuleFactoryAdaptor(HostForDeviceFactory))
	ctx.RegisterModuleType("java_library", android.ModuleFactoryAdaptor(LibraryFactory))
	ctx.RegisterModuleType("java_library_host", android.ModuleFactoryAdaptor(LibraryHostFactory))
	ctx.RegisterModuleType("java_test", android.ModuleFactoryAdaptor(TestFactory))
	ctx.RegisterModuleType("java_import", android.ModuleFactoryAdaptor(ImportFactory))
	ctx.RegisterModuleType("java_import_host", android.ModuleFactoryAdaptor(ImportFactoryHost))
	ctx.RegisterModuleType("java_defaults", android.ModuleFactoryAdaptor(defaultsFactory))
	ctx.RegisterModuleType("java_system_modules", android.ModuleFactoryAdaptor(SystemModulesFactory))
	ctx.RegisterModuleType("java_genrule", android.ModuleFactoryAdaptor(genRuleFactory))
	ctx.RegisterModuleType("java_plugin", android.ModuleFactoryAdaptor(PluginFactory))
	ctx.RegisterModuleType("dex_import", android.ModuleFactoryAdaptor(DexImportFactory))
	ctx.RegisterModuleType("filegroup", android.ModuleFactoryAdaptor(android.FileGroupFactory))
	ctx.RegisterModuleType("genrule", android.ModuleFactoryAdaptor(genrule.GenRuleFactory))
	ctx.RegisterModuleType("droiddoc", android.ModuleFactoryAdaptor(DroiddocFactory))
	ctx.RegisterModuleType("droiddoc_host", android.ModuleFactoryAdaptor(DroiddocHostFactory))
	ctx.RegisterModuleType("droiddoc_template", android.ModuleFactoryAdaptor(ExportedDroiddocDirFactory))
	ctx.RegisterModuleType("java_sdk_library", android.ModuleFactoryAdaptor(SdkLibraryFactory))
	ctx.RegisterModuleType("override_android_app", android.ModuleFactoryAdaptor(OverrideAndroidAppModuleFactory))
	ctx.RegisterModuleType("prebuilt_apis", android.ModuleFactoryAdaptor(PrebuiltApisFactory))
	ctx.PreArchMutators(android.RegisterPrebuiltsPreArchMutators)
	ctx.PreArchMutators(android.RegisterPrebuiltsPostDepsMutators)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.PreArchMutators(android.RegisterOverridePreArchMutators)
	ctx.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("prebuilt_apis", PrebuiltApisMutator).Parallel()
		ctx.TopDown("java_sdk_library", SdkLibraryMutator).Parallel()
	})
	ctx.RegisterPreSingletonType("overlay", android.SingletonFactoryAdaptor(OverlaySingletonFactory))
	ctx.RegisterPreSingletonType("sdk_versions", android.SingletonFactoryAdaptor(sdkPreSingletonFactory))

	// Register module types and mutators from cc needed for JNI testing
	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(cc.LibraryFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(cc.ObjectFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(cc.ToolchainLibraryFactory))
	ctx.RegisterModuleType("llndk_library", android.ModuleFactoryAdaptor(cc.LlndkLibraryFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("link", cc.LinkageMutator).Parallel()
		ctx.BottomUp("begin", cc.BeginMutator).Parallel()
	})

	bp += GatherRequiredDepsForTest()

	mockFS := map[string][]byte{
		"Android.bp":             []byte(bp),
		"a.java":                 nil,
		"b.java":                 nil,
		"c.java":                 nil,
		"b.kt":                   nil,
		"a.jar":                  nil,
		"b.jar":                  nil,
		"APP_NOTICE":             nil,
		"GENRULE_NOTICE":         nil,
		"LIB_NOTICE":             nil,
		"TOOL_NOTICE":            nil,
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

		"prebuilts/sdk/14/public/android.jar":         nil,
		"prebuilts/sdk/14/public/framework.aidl":      nil,
		"prebuilts/sdk/14/system/android.jar":         nil,
		"prebuilts/sdk/17/public/android.jar":         nil,
		"prebuilts/sdk/17/public/framework.aidl":      nil,
		"prebuilts/sdk/17/system/android.jar":         nil,
		"prebuilts/sdk/25/public/android.jar":         nil,
		"prebuilts/sdk/25/public/framework.aidl":      nil,
		"prebuilts/sdk/25/system/android.jar":         nil,
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

		// For framework-res, which is an implicit dependency for framework
		"AndroidManifest.xml":                        nil,
		"build/make/target/product/security/testkey": nil,

		"build/soong/scripts/jar-wrapper.sh": nil,

		"build/make/core/proguard.flags":             nil,
		"build/make/core/proguard_basic_keeps.flags": nil,

		"jdk8/jre/lib/jce.jar": nil,
		"jdk8/jre/lib/rt.jar":  nil,
		"jdk8/lib/tools.jar":   nil,

		"bar-doc/a.java":                 nil,
		"bar-doc/b.java":                 nil,
		"bar-doc/IFoo.aidl":              nil,
		"bar-doc/known_oj_tags.txt":      nil,
		"external/doclava/templates-sdk": nil,

		"cert/new_cert.x509.pem": nil,
		"cert/new_cert.pk8":      nil,
	}

	for k, v := range fs {
		mockFS[k] = v
	}

	ctx.MockFileSystem(mockFS)

	return ctx
}

func run(t *testing.T, ctx *android.TestContext, config android.Config) {
	t.Helper()

	pathCtx := android.PathContextForTesting(config, nil)
	setDexpreoptTestGlobalConfig(config, dexpreopt.GlobalConfigForTests(pathCtx))

	ctx.Register()
	_, errs := ctx.ParseFileList(".", []string{"Android.bp", "prebuilts/sdk/Android.bp"})
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

		dex_import {
			name: "qux",
			jars: ["b.jar"],
		}
		`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	combineJar := ctx.ModuleForTests("foo", "android_common").Description("for javac")
	barJar := ctx.ModuleForTests("bar", "android_common").Rule("combineJar").Output
	bazJar := ctx.ModuleForTests("baz", "android_common").Rule("combineJar").Output

	if !strings.Contains(javac.Args["classpath"], barJar.String()) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], barJar.String())
	}

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != bazJar.String() {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, bazJar.String())
	}

	ctx.ModuleForTests("qux", "android_common").Rule("Cp")
}

func TestDefaults(t *testing.T) {
	ctx := testJava(t, `
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
			// Test that a module with "include_srcs: true" includes its source files in the resources jar
			name: "include sources",
			prop: `include_srcs: true`,
			args: "-C . -f a.java -f b.java -f c.java",
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

func TestTurbine(t *testing.T) {
	ctx := testJava(t, `
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
		        "bar-doc/IFoo.aidl",
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
	inputs := ctx.ModuleForTests("bar-doc", "android_common").Rule("javadoc").Inputs
	var javaSrcs []string
	for _, i := range inputs {
		javaSrcs = append(javaSrcs, i.Base())
	}
	if len(javaSrcs) != 2 || javaSrcs[0] != "a.java" || javaSrcs[1] != "IFoo.java" {
		t.Errorf("inputs of bar-doc must be []string{\"a.java\", \"IFoo.java\", but was %#v.", javaSrcs)
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

func TestJavaSdkLibrary(t *testing.T) {
	ctx := testJava(t, `
		droiddoc_template {
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
			libs: ["foo", "bar"],
			sdk_version: "system_current",
		}
		java_library {
		    name: "qux",
		    srcs: ["c.java"],
		    libs: ["baz"],
		    sdk_version: "system_current",
		}
		`)

	// check the existence of the internal modules
	ctx.ModuleForTests("foo", "android_common")
	ctx.ModuleForTests("foo"+sdkStubsLibrarySuffix, "android_common")
	ctx.ModuleForTests("foo"+sdkStubsLibrarySuffix+sdkSystemApiSuffix, "android_common")
	ctx.ModuleForTests("foo"+sdkStubsLibrarySuffix+sdkTestApiSuffix, "android_common")
	ctx.ModuleForTests("foo"+sdkDocsSuffix, "android_common")
	ctx.ModuleForTests("foo"+sdkDocsSuffix+sdkSystemApiSuffix, "android_common")
	ctx.ModuleForTests("foo"+sdkDocsSuffix+sdkTestApiSuffix, "android_common")
	ctx.ModuleForTests("foo"+sdkXmlFileSuffix, "android_arm64_armv8-a")
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

	// test if baz has exported SDK lib names foo and bar to qux
	qux := ctx.ModuleForTests("qux", "android_common")
	if quxLib, ok := qux.Module().(*Library); ok {
		sdkLibs := quxLib.ExportedSdkLibs()
		if len(sdkLibs) != 2 || !android.InList("foo", sdkLibs) || !android.InList("bar", sdkLibs) {
			t.Errorf("qux should export \"foo\" and \"bar\" but exports %v", sdkLibs)
		}
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
	bp := `
		java_library {
			name: "foo",
			srcs: ["a.java"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			no_standard_libs: true,
			system_modules: "none",
			patch_module: "java.base",
		}

		java_library {
			name: "baz",
			srcs: ["c.java"],
			patch_module: "java.base",
		}
	`

	t.Run("1.8", func(t *testing.T) {
		// Test default javac 1.8
		ctx := testJava(t, bp)

		checkPatchModuleFlag(t, ctx, "foo", "")
		checkPatchModuleFlag(t, ctx, "bar", "")
		checkPatchModuleFlag(t, ctx, "baz", "")
	})

	t.Run("1.9", func(t *testing.T) {
		// Test again with javac 1.9
		config := testConfig(map[string]string{"EXPERIMENTAL_USE_OPENJDK9": "true"})
		ctx := testContext(config, bp, nil)
		run(t, ctx, config)

		checkPatchModuleFlag(t, ctx, "foo", "")
		expected := "java.base=.:" + buildDir
		checkPatchModuleFlag(t, ctx, "bar", expected)
		expected = "java.base=" + strings.Join([]string{".", buildDir, moduleToPath("ext"), moduleToPath("framework"), moduleToPath("updatable_media_stubs")}, ":")
		checkPatchModuleFlag(t, ctx, "baz", expected)
	})
}
