// Copyright 2021 Google Inc. All rights reserved.
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
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

func TestJavaSdkLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithPrebuiltApis(map[string][]string{
			"28": {"foo"},
			"29": {"foo"},
			"30": {"bar", "barney", "baz", "betty", "foo", "fred", "quuz", "wilma"},
		}),
		android.FixtureModifyConfig(func(config android.Config) {
			config.SetApiLibraries([]string{"foo"})
		}),
	).RunTestWithBp(t, `
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
			exclude_kotlinc_generated_files: true,
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
					current_api: "api/current.txt",
					removed_api: "api/removed.txt",
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
	foo := result.ModuleForTests("foo", "android_common")
	result.ModuleForTests(apiScopePublic.stubsLibraryModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopeSystem.stubsLibraryModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopeTest.stubsLibraryModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopePublic.stubsSourceModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopeSystem.stubsSourceModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopeTest.stubsSourceModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopePublic.stubsSourceModuleName("foo")+".api.contribution", "")
	result.ModuleForTests(apiScopePublic.apiLibraryModuleName("foo"), "android_common")
	result.ModuleForTests("foo"+sdkXmlFileSuffix, "android_common")
	result.ModuleForTests("foo.api.public.28", "")
	result.ModuleForTests("foo.api.system.28", "")
	result.ModuleForTests("foo.api.test.28", "")

	exportedComponentsInfo := result.ModuleProvider(foo.Module(), android.ExportedComponentsInfoProvider).(android.ExportedComponentsInfo)
	expectedFooExportedComponents := []string{
		"foo-removed.api.public.latest",
		"foo-removed.api.system.latest",
		"foo.api.public.latest",
		"foo.api.system.latest",
		"foo.stubs",
		"foo.stubs.source",
		"foo.stubs.source.system",
		"foo.stubs.source.test",
		"foo.stubs.system",
		"foo.stubs.test",
	}
	android.AssertArrayString(t, "foo exported components", expectedFooExportedComponents, exportedComponentsInfo.Components)

	bazJavac := result.ModuleForTests("baz", "android_common").Rule("javac")
	// tests if baz is actually linked to the stubs lib
	android.AssertStringDoesContain(t, "baz javac classpath", bazJavac.Args["classpath"], "foo.stubs.system.jar")
	// ... and not to the impl lib
	android.AssertStringDoesNotContain(t, "baz javac classpath", bazJavac.Args["classpath"], "foo.jar")
	// test if baz is not linked to the system variant of foo
	android.AssertStringDoesNotContain(t, "baz javac classpath", bazJavac.Args["classpath"], "foo.stubs.jar")

	bazTestJavac := result.ModuleForTests("baz-test", "android_common").Rule("javac")
	// tests if baz-test is actually linked to the test stubs lib
	android.AssertStringDoesContain(t, "baz-test javac classpath", bazTestJavac.Args["classpath"], "foo.stubs.test.jar")

	baz29Javac := result.ModuleForTests("baz-29", "android_common").Rule("javac")
	// tests if baz-29 is actually linked to the system 29 stubs lib
	android.AssertStringDoesContain(t, "baz-29 javac classpath", baz29Javac.Args["classpath"], "prebuilts/sdk/29/system/foo.jar")

	bazModule30Javac := result.ModuleForTests("baz-module-30", "android_common").Rule("javac")
	// tests if "baz-module-30" is actually linked to the module 30 stubs lib
	android.AssertStringDoesContain(t, "baz-module-30 javac classpath", bazModule30Javac.Args["classpath"], "prebuilts/sdk/30/module-lib/foo.jar")

	// test if baz has exported SDK lib names foo and bar to qux
	qux := result.ModuleForTests("qux", "android_common")
	if quxLib, ok := qux.Module().(*Library); ok {
		requiredSdkLibs, optionalSdkLibs := quxLib.ClassLoaderContexts().UsesLibs()
		android.AssertDeepEquals(t, "qux exports (required)", []string{"fred", "quuz", "foo", "bar"}, requiredSdkLibs)
		android.AssertDeepEquals(t, "qux exports (optional)", []string{}, optionalSdkLibs)
	}

	// test if quuz have created the api_contribution module
	result.ModuleForTests(apiScopePublic.stubsSourceModuleName("quuz")+".api.contribution", "")

	fooDexJar := result.ModuleForTests("foo", "android_common").Rule("d8")
	// tests if kotlinc generated files are NOT excluded from output of foo.
	android.AssertStringDoesNotContain(t, "foo dex", fooDexJar.BuildParams.Args["mergeZipsFlags"], "-stripFile META-INF/*.kotlin_module")

	barDexJar := result.ModuleForTests("bar", "android_common").Rule("d8")
	// tests if kotlinc generated files are excluded from output of bar.
	android.AssertStringDoesContain(t, "bar dex", barDexJar.BuildParams.Args["mergeZipsFlags"], "-stripFile META-INF/*.kotlin_module")
}

func TestJavaSdkLibrary_UpdatableLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithPrebuiltApis(map[string][]string{
			"28": {"foo"},
			"29": {"foo"},
			"30": {"foo", "fooUpdatable", "fooUpdatableErr"},
		}),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_version_active_codenames = []string{"Tiramisu", "U", "V", "W", "X"}
		}),
	).RunTestWithBp(t,
		`
		java_sdk_library {
			name: "fooUpdatable",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
			on_bootclasspath_since: "U",
			on_bootclasspath_before: "V",
			min_device_sdk: "W",
			max_device_sdk: "X",
			min_sdk_version: "S",
		}
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
		}
`)

	// test that updatability attributes are passed on correctly
	fooUpdatable := result.ModuleForTests("fooUpdatable.xml", "android_common").Rule("java_sdk_xml")
	android.AssertStringDoesContain(t, "fooUpdatable.xml java_sdk_xml command", fooUpdatable.RuleParams.Command, `on-bootclasspath-since=\"U\"`)
	android.AssertStringDoesContain(t, "fooUpdatable.xml java_sdk_xml command", fooUpdatable.RuleParams.Command, `on-bootclasspath-before=\"V\"`)
	android.AssertStringDoesContain(t, "fooUpdatable.xml java_sdk_xml command", fooUpdatable.RuleParams.Command, `min-device-sdk=\"W\"`)
	android.AssertStringDoesContain(t, "fooUpdatable.xml java_sdk_xml command", fooUpdatable.RuleParams.Command, `max-device-sdk=\"X\"`)

	// double check that updatability attributes are not written if they don't exist in the bp file
	// the permissions file for the foo library defined above
	fooPermissions := result.ModuleForTests("foo.xml", "android_common").Rule("java_sdk_xml")
	android.AssertStringDoesNotContain(t, "foo.xml java_sdk_xml command", fooPermissions.RuleParams.Command, `on-bootclasspath-since`)
	android.AssertStringDoesNotContain(t, "foo.xml java_sdk_xml command", fooPermissions.RuleParams.Command, `on-bootclasspath-before`)
	android.AssertStringDoesNotContain(t, "foo.xml java_sdk_xml command", fooPermissions.RuleParams.Command, `min-device-sdk`)
	android.AssertStringDoesNotContain(t, "foo.xml java_sdk_xml command", fooPermissions.RuleParams.Command, `max-device-sdk`)
}

func TestJavaSdkLibrary_UpdatableLibrary_Validation_ValidVersion(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithPrebuiltApis(map[string][]string{
			"30": {"fooUpdatable", "fooUpdatableErr"},
		}),
	).ExtendWithErrorHandler(android.FixtureExpectsAllErrorsToMatchAPattern(
		[]string{
			`on_bootclasspath_since: "aaa" could not be parsed as an integer and is not a recognized codename`,
			`on_bootclasspath_before: "bbc" could not be parsed as an integer and is not a recognized codename`,
			`min_device_sdk: "ccc" could not be parsed as an integer and is not a recognized codename`,
			`max_device_sdk: "current" is not an allowed value for this attribute`,
		})).RunTestWithBp(t,
		`
	java_sdk_library {
			name: "fooUpdatableErr",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
			on_bootclasspath_since: "aaa",
			on_bootclasspath_before: "bbc",
			min_device_sdk: "ccc",
			max_device_sdk: "current",
		}
`)
}

func TestJavaSdkLibrary_UpdatableLibrary_Validation_AtLeastTAttributes(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithPrebuiltApis(map[string][]string{
			"28": {"foo"},
		}),
	).ExtendWithErrorHandler(android.FixtureExpectsAllErrorsToMatchAPattern(
		[]string{
			"on_bootclasspath_since: Attribute value needs to be at least T",
			"on_bootclasspath_before: Attribute value needs to be at least T",
			"min_device_sdk: Attribute value needs to be at least T",
			"max_device_sdk: Attribute value needs to be at least T",
		},
	)).RunTestWithBp(t,
		`
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
			on_bootclasspath_since: "S",
			on_bootclasspath_before: "S",
			min_device_sdk: "S",
			max_device_sdk: "S",
			min_sdk_version: "S",
		}
`)
}

func TestJavaSdkLibrary_UpdatableLibrary_Validation_MinAndMaxDeviceSdk(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithPrebuiltApis(map[string][]string{
			"28": {"foo"},
		}),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_version_active_codenames = []string{"Tiramisu", "U", "V"}
		}),
	).ExtendWithErrorHandler(android.FixtureExpectsAllErrorsToMatchAPattern(
		[]string{
			"min_device_sdk can't be greater than max_device_sdk",
		},
	)).RunTestWithBp(t,
		`
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
			min_device_sdk: "V",
			max_device_sdk: "U",
			min_sdk_version: "S",
		}
`)
}

func TestJavaSdkLibrary_UpdatableLibrary_Validation_MinAndMaxDeviceSdkAndModuleMinSdk(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithPrebuiltApis(map[string][]string{
			"28": {"foo"},
		}),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_version_active_codenames = []string{"Tiramisu", "U", "V"}
		}),
	).ExtendWithErrorHandler(android.FixtureExpectsAllErrorsToMatchAPattern(
		[]string{
			regexp.QuoteMeta("min_device_sdk: Can't be less than module's min sdk (V)"),
			regexp.QuoteMeta("max_device_sdk: Can't be less than module's min sdk (V)"),
		},
	)).RunTestWithBp(t,
		`
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
			min_device_sdk: "U",
			max_device_sdk: "U",
			min_sdk_version: "V",
		}
`)
}

func TestJavaSdkLibrary_UpdatableLibrary_usesNewTag(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithPrebuiltApis(map[string][]string{
			"30": {"foo"},
		}),
	).RunTestWithBp(t,
		`
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			min_device_sdk: "Tiramisu",
			min_sdk_version: "S",
		}
`)
	// test that updatability attributes are passed on correctly
	fooUpdatable := result.ModuleForTests("foo.xml", "android_common").Rule("java_sdk_xml")
	android.AssertStringDoesContain(t, "foo.xml java_sdk_xml command", fooUpdatable.RuleParams.Command, `<apex-library`)
	android.AssertStringDoesNotContain(t, "foo.xml java_sdk_xml command", fooUpdatable.RuleParams.Command, `<library`)
}

func TestJavaSdkLibrary_StubOrImplOnlyLibs(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("sdklib"),
	).RunTestWithBp(t, `
		java_sdk_library {
			name: "sdklib",
			srcs: ["a.java"],
			libs: ["lib"],
			static_libs: ["static-lib"],
			impl_only_libs: ["impl-only-lib"],
			stub_only_libs: ["stub-only-lib"],
			stub_only_static_libs: ["stub-only-static-lib"],
		}
		java_defaults {
			name: "defaults",
			srcs: ["a.java"],
			sdk_version: "current",
		}
		java_library { name: "lib", defaults: ["defaults"] }
		java_library { name: "static-lib", defaults: ["defaults"] }
		java_library { name: "impl-only-lib", defaults: ["defaults"] }
		java_library { name: "stub-only-lib", defaults: ["defaults"] }
		java_library { name: "stub-only-static-lib", defaults: ["defaults"] }
		`)
	var expectations = []struct {
		lib               string
		on_impl_classpath bool
		on_stub_classpath bool
		in_impl_combined  bool
		in_stub_combined  bool
	}{
		{lib: "lib", on_impl_classpath: true},
		{lib: "static-lib", in_impl_combined: true},
		{lib: "impl-only-lib", on_impl_classpath: true},
		{lib: "stub-only-lib", on_stub_classpath: true},
		{lib: "stub-only-static-lib", in_stub_combined: true},
	}
	verify := func(sdklib, dep string, cp, combined bool) {
		sdklibCp := result.ModuleForTests(sdklib, "android_common").Rule("javac").Args["classpath"]
		expected := cp || combined // Every combined jar is also on the classpath.
		android.AssertStringContainsEquals(t, "bad classpath for "+sdklib, sdklibCp, "/"+dep+".jar", expected)

		combineJarInputs := result.ModuleForTests(sdklib, "android_common").Rule("combineJar").Inputs.Strings()
		depPath := filepath.Join("out", "soong", ".intermediates", dep, "android_common", "turbine-combined", dep+".jar")
		android.AssertStringListContainsEquals(t, "bad combined inputs for "+sdklib, combineJarInputs, depPath, combined)
	}
	for _, expectation := range expectations {
		verify("sdklib.impl", expectation.lib, expectation.on_impl_classpath, expectation.in_impl_combined)

		stubName := apiScopePublic.sourceStubLibraryModuleName("sdklib")
		verify(stubName, expectation.lib, expectation.on_stub_classpath, expectation.in_stub_combined)
	}
}

func TestJavaSdkLibrary_DoNotAccessImplWhenItIsNotBuilt(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
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
	barLibrary := result.ModuleForTests("bar", "android_common").Rule("javac")
	if expected, actual := `^-classpath .*:out/soong/[^:]*/turbine-combined/foo\.stubs\.jar$`, barLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

func TestJavaSdkLibrary_AccessOutputFiles(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			annotations_enabled: true,
			public: {
				enabled: true,
			},
		}
		java_library {
			name: "bar",
			srcs: ["b.java", ":foo{.public.stubs.source}"],
			java_resources: [":foo{.public.annotations.zip}"],
		}
		`)
}

func TestJavaSdkLibrary_AccessOutputFiles_NoAnnotations(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`module "bar" variant "android_common": path dependency ":foo{.public.annotations.zip}": annotations.zip not available for api scope public`)).
		RunTestWithBp(t, `
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
			java_resources: [":foo{.public.annotations.zip}"],
		}
		`)
}

func TestJavaSdkLibrary_AccessOutputFiles_MissingScope(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`"foo" does not provide api scope system`)).
		RunTestWithBp(t, `
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
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("sdklib"),
	).RunTestWithBp(t, `
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

	CheckModuleDependencies(t, result.TestContext, "sdklib", "android_common", []string{
		`dex2oatd`,
		`sdklib-removed.api.public.latest`,
		`sdklib.api.public.latest`,
		`sdklib.impl`,
		`sdklib.stubs`,
		`sdklib.stubs.source`,
		`sdklib.xml`,
	})
}

func TestJavaSdkLibraryImport_AccessOutputFiles(t *testing.T) {
	prepareForJavaTest.RunTestWithBp(t, `
		java_sdk_library_import {
			name: "foo",
			public: {
				jars: ["a.jar"],
				stub_srcs: ["a.java"],
				current_api: "api/current.txt",
				removed_api: "api/removed.txt",
				annotations: "x/annotations.zip",
			},
		}

		java_library {
			name: "bar",
			srcs: [":foo{.public.stubs.source}"],
			java_resources: [
				":foo{.public.api.txt}",
				":foo{.public.removed-api.txt}",
				":foo{.public.annotations.zip}",
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
		prepareForJavaTest.
			ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`stubs.source not available for api scope public`)).
			RunTestWithBp(t, bp+`
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
		prepareForJavaTest.
			ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`api.txt not available for api scope public`)).
			RunTestWithBp(t, bp+`
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
		prepareForJavaTest.
			ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`removed-api.txt not available for api scope public`)).
			RunTestWithBp(t, bp+`
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
	prepareForJavaTest.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`module "foo": enabled api scope "system" depends on disabled scope "public"`)).
		RunTestWithBp(t, `
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
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
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
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
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
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
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

func TestJavaSdkLibrary_SystemServer_AccessToStubScopeLibs(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo-public", "foo-system", "foo-module-lib", "foo-system-server"),
	).RunTestWithBp(t, `
		java_sdk_library {
			name: "foo-public",
			srcs: ["a.java"],
			api_packages: ["foo"],
			public: {
				enabled: true,
			},
		}

		java_sdk_library {
			name: "foo-system",
			srcs: ["a.java"],
			api_packages: ["foo"],
			system: {
				enabled: true,
			},
		}

		java_sdk_library {
			name: "foo-module-lib",
			srcs: ["a.java"],
			api_packages: ["foo"],
			system: {
				enabled: true,
			},
			module_lib: {
				enabled: true,
			},
		}

		java_sdk_library {
			name: "foo-system-server",
			srcs: ["a.java"],
			api_packages: ["foo"],
			system_server: {
				enabled: true,
			},
		}

		java_library {
			name: "bar",
			srcs: ["a.java"],
			libs: ["foo-public", "foo-system", "foo-module-lib", "foo-system-server"],
			sdk_version: "system_server_current",
		}
		`)

	stubsPath := func(name string, scope *apiScope) string {
		name = scope.stubsLibraryModuleName(name)
		return fmt.Sprintf("out/soong/.intermediates/%[1]s/android_common/turbine-combined/%[1]s.jar", name)
	}

	// The bar library should depend on the highest (where system server is highest and public is
	// lowest) API scopes provided by each of the foo-* modules. The highest API scope provided by the
	// foo-<x> module is <x>.
	barLibrary := result.ModuleForTests("bar", "android_common").Rule("javac")
	stubLibraries := []string{
		stubsPath("foo-public", apiScopePublic),
		stubsPath("foo-system", apiScopeSystem),
		stubsPath("foo-module-lib", apiScopeModuleLib),
		stubsPath("foo-system-server", apiScopeSystemServer),
	}
	expectedPattern := fmt.Sprintf(`^-classpath .*:\Q%s\E$`, strings.Join(stubLibraries, ":"))
	if expected, actual := expectedPattern, barLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected pattern %q to match %#q", expected, actual)
	}
}

func TestJavaSdkLibrary_MissingScope(t *testing.T) {
	prepareForJavaTest.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`requires api scope module-lib from foo but it only has \[\] available`)).
		RunTestWithBp(t, `
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
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
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
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
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
	bazLibrary := result.ModuleForTests("baz", "android_common").Rule("javac")
	if expected, actual := `^-classpath .*:out/soong/[^:]*/turbine-combined/foo\.stubs.system\.jar$`, bazLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

func TestJavaSdkLibraryImport(t *testing.T) {
	result := prepareForJavaTest.RunTestWithBp(t, `
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
		fooModule := result.ModuleForTests("foo"+scope, "android_common")
		javac := fooModule.Rule("javac")

		sdklibStubsJar := result.ModuleForTests("sdklib.stubs"+scope, "android_common").Rule("combineJar").Output
		android.AssertStringDoesContain(t, "foo classpath", javac.Args["classpath"], sdklibStubsJar.String())
	}

	CheckModuleDependencies(t, result.TestContext, "sdklib", "android_common", []string{
		`dex2oatd`,
		`prebuilt_sdklib.stubs`,
		`prebuilt_sdklib.stubs.source.test`,
		`prebuilt_sdklib.stubs.system`,
		`prebuilt_sdklib.stubs.test`,
	})
}

func TestJavaSdkLibraryImport_WithSource(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("sdklib"),
	).RunTestWithBp(t, `
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

	CheckModuleDependencies(t, result.TestContext, "sdklib", "android_common", []string{
		`dex2oatd`,
		`prebuilt_sdklib`,
		`sdklib-removed.api.public.latest`,
		`sdklib.api.public.latest`,
		`sdklib.impl`,
		`sdklib.stubs`,
		`sdklib.stubs.source`,
		`sdklib.xml`,
	})

	CheckModuleDependencies(t, result.TestContext, "prebuilt_sdklib", "android_common", []string{
		`prebuilt_sdklib.stubs`,
		`sdklib.impl`,
		// This should be prebuilt_sdklib.stubs but is set to sdklib.stubs because the
		// dependency is added after prebuilts may have been renamed and so has to use
		// the renamed name.
		`sdklib.xml`,
	})
}

func testJavaSdkLibraryImport_Preferred(t *testing.T, prefer string, preparer android.FixturePreparer) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("sdklib"),
		preparer,
	).RunTestWithBp(t, `
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
			`+prefer+`
			public: {
				jars: ["a.jar"],
				stub_srcs: ["a.java"],
				current_api: "current.txt",
				removed_api: "removed.txt",
				annotations: "annotations.zip",
			},
		}

		java_library {
			name: "combined",
			static_libs: [
				"sdklib.stubs",
			],
			java_resources: [
				":sdklib.stubs.source",
				":sdklib{.public.api.txt}",
				":sdklib{.public.removed-api.txt}",
				":sdklib{.public.annotations.zip}",
			],
			sdk_version: "none",
			system_modules: "none",
		}

		java_library {
			name: "public",
			srcs: ["a.java"],
			libs: ["sdklib"],
			sdk_version: "current",
		}
		`)

	CheckModuleDependencies(t, result.TestContext, "sdklib", "android_common", []string{
		`prebuilt_sdklib`,
		`sdklib-removed.api.public.latest`,
		`sdklib.api.public.latest`,
		`sdklib.impl`,
		`sdklib.stubs`,
		`sdklib.stubs.source`,
		`sdklib.xml`,
	})

	CheckModuleDependencies(t, result.TestContext, "prebuilt_sdklib", "android_common", []string{
		`dex2oatd`,
		`prebuilt_sdklib.stubs`,
		`prebuilt_sdklib.stubs.source`,
		`sdklib.impl`,
		`sdklib.xml`,
	})

	// Make sure that dependencies on child modules use the prebuilt when preferred.
	CheckModuleDependencies(t, result.TestContext, "combined", "android_common", []string{
		// Each use of :sdklib{...} adds a dependency onto prebuilt_sdklib.
		`prebuilt_sdklib`,
		`prebuilt_sdklib`,
		`prebuilt_sdklib`,
		`prebuilt_sdklib.stubs`,
		`prebuilt_sdklib.stubs.source`,
	})

	// Make sure that dependencies on sdklib that resolve to one of the child libraries use the
	// prebuilt library.
	public := result.ModuleForTests("public", "android_common")
	rule := public.Output("javac/public.jar")
	inputs := rule.Implicits.Strings()
	expected := "out/soong/.intermediates/prebuilt_sdklib.stubs/android_common/combined/sdklib.stubs.jar"
	if !android.InList(expected, inputs) {
		t.Errorf("expected %q to contain %q", inputs, expected)
	}
}

func TestJavaSdkLibraryImport_Preferred(t *testing.T) {
	t.Run("prefer", func(t *testing.T) {
		testJavaSdkLibraryImport_Preferred(t, "prefer: true,", android.NullFixturePreparer)
	})

	t.Run("use_source_config_var", func(t *testing.T) {
		testJavaSdkLibraryImport_Preferred(t,
			"use_source_config_var: {config_namespace: \"acme\", var_name: \"use_source\"},",
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.VendorVars = map[string]map[string]string{
					"acme": {
						"use_source": "false",
					},
				}
			}))
	})
}

// If a module is listed in `mainline_module_contributions, it should be used
// It will supersede any other source vs prebuilt selection mechanism like `prefer` attribute
func TestSdkLibraryImport_MetadataModuleSupersedesPreferred(t *testing.T) {
	bp := `
		apex_contributions {
			name: "my_mainline_module_contributions",
			api_domain: "my_mainline_module",
			contents: [
				// legacy mechanism prefers the prebuilt
				// mainline_module_contributions supersedes this since source is listed explicitly
				"sdklib.prebuilt_preferred_using_legacy_flags",

				// legacy mechanism prefers the source
				// mainline_module_contributions supersedes this since prebuilt is listed explicitly
				"prebuilt_sdklib.source_preferred_using_legacy_flags",
			],
		}
		all_apex_contributions {
			name: "all_apex_contributions",
		}
		java_sdk_library {
			name: "sdklib.prebuilt_preferred_using_legacy_flags",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "none",
			public: {
				enabled: true,
			},
			system: {
				enabled: true,
			}
		}
		java_sdk_library_import {
			name: "sdklib.prebuilt_preferred_using_legacy_flags",
			prefer: true, // prebuilt is preferred using legacy mechanism
			public: {
				jars: ["a.jar"],
				stub_srcs: ["a.java"],
				current_api: "current.txt",
				removed_api: "removed.txt",
				annotations: "annotations.zip",
			},
			system: {
				jars: ["a.jar"],
				stub_srcs: ["a.java"],
				current_api: "current.txt",
				removed_api: "removed.txt",
				annotations: "annotations.zip",
			},
		}
		java_sdk_library {
			name: "sdklib.source_preferred_using_legacy_flags",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "none",
			public: {
				enabled: true,
			},
			system: {
				enabled: true,
			}
		}
		java_sdk_library_import {
			name: "sdklib.source_preferred_using_legacy_flags",
			prefer: false, // source is preferred using legacy mechanism
			public: {
				jars: ["a.jar"],
				stub_srcs: ["a.java"],
				current_api: "current.txt",
				removed_api: "removed.txt",
				annotations: "annotations.zip",
			},
			system: {
				jars: ["a.jar"],
				stub_srcs: ["a.java"],
				current_api: "current.txt",
				removed_api: "removed.txt",
				annotations: "annotations.zip",
			},
		}

		// rdeps
		java_library {
			name: "public",
			srcs: ["a.java"],
			libs: [
				// this should get source since source is listed in my_mainline_module_contributions
				"sdklib.prebuilt_preferred_using_legacy_flags.stubs",
				"sdklib.prebuilt_preferred_using_legacy_flags.stubs.system",

				// this should get prebuilt since source is listed in my_mainline_module_contributions
				"sdklib.source_preferred_using_legacy_flags.stubs",
				"sdklib.source_preferred_using_legacy_flags.stubs.system",

			],
		}
	`
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("sdklib.source_preferred_using_legacy_flags", "sdklib.prebuilt_preferred_using_legacy_flags"),
		android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
			android.RegisterApexContributionsBuildComponents(ctx)
		}),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.BuildFlags = map[string]string{
				"RELEASE_APEX_CONTRIBUTIONS_ADSERVICES": "my_mainline_module_contributions",
			}
		}),
	).RunTestWithBp(t, bp)

	// Make sure that rdeps get the correct source vs prebuilt based on mainline_module_contributions
	public := result.ModuleForTests("public", "android_common")
	rule := public.Output("javac/public.jar")
	inputs := rule.Implicits.Strings()
	expectedInputs := []string{
		// source
		"out/soong/.intermediates/sdklib.prebuilt_preferred_using_legacy_flags.stubs/android_common/turbine-combined/sdklib.prebuilt_preferred_using_legacy_flags.stubs.jar",
		"out/soong/.intermediates/sdklib.prebuilt_preferred_using_legacy_flags.stubs.system/android_common/turbine-combined/sdklib.prebuilt_preferred_using_legacy_flags.stubs.system.jar",

		// prebuilt
		"out/soong/.intermediates/prebuilt_sdklib.source_preferred_using_legacy_flags.stubs/android_common/combined/sdklib.source_preferred_using_legacy_flags.stubs.jar",
		"out/soong/.intermediates/prebuilt_sdklib.source_preferred_using_legacy_flags.stubs.system/android_common/combined/sdklib.source_preferred_using_legacy_flags.stubs.system.jar",
	}
	for _, expected := range expectedInputs {
		if !android.InList(expected, inputs) {
			t.Errorf("expected %q to contain %q", inputs, expected)
		}
	}
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

	createPreparer := func(info testConfigInfo) android.FixturePreparer {
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

		return android.GroupFixturePreparers(
			PrepareForTestWithJavaSdkLibraryFiles,
			FixtureWithLastReleaseApis("bar"),
			android.FixtureWithRootAndroidBp(bpFile),
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.EnforceProductPartitionInterface = proptools.BoolPtr(info.enforceProductInterface)
				if info.enforceVendorInterface {
					variables.DeviceVndkVersion = proptools.StringPtr("current")
				}
				variables.EnforceInterPartitionJavaSdkLibrary = proptools.BoolPtr(info.enforceJavaSdkLibraryCheck)
				variables.InterPartitionJavaLibraryAllowList = info.allowList
			}),
		)
	}

	runTest := func(t *testing.T, info testConfigInfo, expectedErrorPattern string) {
		t.Run(fmt.Sprintf("%v", info), func(t *testing.T) {
			errorHandler := android.FixtureExpectsNoErrors
			if expectedErrorPattern != "" {
				errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(expectedErrorPattern)
			}
			android.GroupFixturePreparers(
				prepareForJavaTest,
				createPreparer(info),
			).
				ExtendWithErrorHandler(errorHandler).
				RunTest(t)
		})
	}

	errorMessage := "is not allowed across the partitions"

	runTest(t, testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "product",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: false,
	}, "")

	runTest(t, testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "product",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    false,
		enforceJavaSdkLibraryCheck: true,
	}, "")

	runTest(t, testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "product",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}, errorMessage)

	runTest(t, testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "vendor",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}, errorMessage)

	runTest(t, testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "vendor",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
		allowList:                  []string{"bar"},
	}, "")

	runTest(t, testConfigInfo{
		libraryType:                "java_library",
		fromPartition:              "vendor",
		toPartition:                "product",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}, errorMessage)

	runTest(t, testConfigInfo{
		libraryType:                "java_sdk_library",
		fromPartition:              "product",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}, "")

	runTest(t, testConfigInfo{
		libraryType:                "java_sdk_library",
		fromPartition:              "vendor",
		toPartition:                "system",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}, "")

	runTest(t, testConfigInfo{
		libraryType:                "java_sdk_library",
		fromPartition:              "vendor",
		toPartition:                "product",
		enforceVendorInterface:     true,
		enforceProductInterface:    true,
		enforceJavaSdkLibraryCheck: true,
	}, "")
}

func TestJavaSdkLibraryDist(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaBuildComponents,
		PrepareForTestWithJavaDefaultModules,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis(
			"sdklib_no_group",
			"sdklib_group_foo",
			"sdklib_owner_foo",
			"foo"),
	).RunTestWithBp(t, `
		java_sdk_library {
			name: "sdklib_no_group",
			srcs: ["foo.java"],
		}

		java_sdk_library {
			name: "sdklib_group_foo",
			srcs: ["foo.java"],
			dist_group: "foo",
		}

		java_sdk_library {
			name: "sdklib_owner_foo",
			srcs: ["foo.java"],
			owner: "foo",
		}

		java_sdk_library {
			name: "sdklib_stem_foo",
			srcs: ["foo.java"],
			dist_stem: "foo",
		}
	`)

	type testCase struct {
		module   string
		distDir  string
		distStem string
	}
	testCases := []testCase{
		{
			module:   "sdklib_no_group",
			distDir:  "apistubs/unknown/public",
			distStem: "sdklib_no_group.jar",
		},
		{
			module:   "sdklib_group_foo",
			distDir:  "apistubs/foo/public",
			distStem: "sdklib_group_foo.jar",
		},
		{
			// Owner doesn't affect distDir after b/186723288.
			module:   "sdklib_owner_foo",
			distDir:  "apistubs/unknown/public",
			distStem: "sdklib_owner_foo.jar",
		},
		{
			module:   "sdklib_stem_foo",
			distDir:  "apistubs/unknown/public",
			distStem: "foo.jar",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.module, func(t *testing.T) {
			m := result.ModuleForTests(tt.module+".stubs", "android_common").Module().(*Library)
			dists := m.Dists()
			if len(dists) != 1 {
				t.Fatalf("expected exactly 1 dist entry, got %d", len(dists))
			}
			if g, w := String(dists[0].Dir), tt.distDir; g != w {
				t.Errorf("expected dist dir %q, got %q", w, g)
			}
			if g, w := String(dists[0].Dest), tt.distStem; g != w {
				t.Errorf("expected dist stem %q, got %q", w, g)
			}
		})
	}
}

func TestSdkLibrary_CheckMinSdkVersion(t *testing.T) {
	preparer := android.GroupFixturePreparers(
		PrepareForTestWithJavaBuildComponents,
		PrepareForTestWithJavaDefaultModules,
		PrepareForTestWithJavaSdkLibraryFiles,
	)

	preparer.RunTestWithBp(t, `
		java_sdk_library {
			name: "sdklib",
            srcs: ["a.java"],
            static_libs: ["util"],
            min_sdk_version: "30",
			unsafe_ignore_missing_latest_api: true,
        }

		java_library {
			name: "util",
			srcs: ["a.java"],
			min_sdk_version: "30",
		}
	`)

	preparer.
		RunTestWithBp(t, `
			java_sdk_library {
				name: "sdklib",
				srcs: ["a.java"],
				libs: ["util"],
				impl_only_libs: ["util"],
				stub_only_libs: ["util"],
				stub_only_static_libs: ["util"],
				min_sdk_version: "30",
				unsafe_ignore_missing_latest_api: true,
			}

			java_library {
				name: "util",
				srcs: ["a.java"],
			}
		`)

	preparer.ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`module "util".*should support min_sdk_version\(30\)`)).
		RunTestWithBp(t, `
			java_sdk_library {
				name: "sdklib",
				srcs: ["a.java"],
				static_libs: ["util"],
				min_sdk_version: "30",
				unsafe_ignore_missing_latest_api: true,
			}

			java_library {
				name: "util",
				srcs: ["a.java"],
				min_sdk_version: "31",
			}
		`)

	preparer.ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`module "another_util".*should support min_sdk_version\(30\)`)).
		RunTestWithBp(t, `
			java_sdk_library {
				name: "sdklib",
				srcs: ["a.java"],
				static_libs: ["util"],
				min_sdk_version: "30",
				unsafe_ignore_missing_latest_api: true,
			}

			java_library {
				name: "util",
				srcs: ["a.java"],
				static_libs: ["another_util"],
				min_sdk_version: "30",
			}

			java_library {
				name: "another_util",
				srcs: ["a.java"],
				min_sdk_version: "31",
			}
		`)
}

func TestJavaSdkLibrary_StubOnlyLibs_PassedToDroidstubs(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			public: {
				enabled: true,
			},
			stub_only_libs: ["bar-lib"],
		}

		java_library {
			name: "bar-lib",
			srcs: ["b.java"],
		}
		`)

	// The foo.stubs.source should depend on bar-lib
	fooStubsSources := result.ModuleForTests("foo.stubs.source", "android_common").Module().(*Droidstubs)
	android.AssertStringListContains(t, "foo stubs should depend on bar-lib", fooStubsSources.Javadoc.properties.Libs, "bar-lib")
}

func TestJavaSdkLibrary_Scope_Libs_PassedToDroidstubs(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			public: {
				enabled: true,
				libs: ["bar-lib"],
			},
		}

		java_library {
			name: "bar-lib",
			srcs: ["b.java"],
		}
		`)

	// The foo.stubs.source should depend on bar-lib
	fooStubsSources := result.ModuleForTests("foo.stubs.source", "android_common").Module().(*Droidstubs)
	android.AssertStringListContains(t, "foo stubs should depend on bar-lib", fooStubsSources.Javadoc.properties.Libs, "bar-lib")
}

func TestJavaSdkLibrary_ApiLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
		android.FixtureModifyConfig(func(config android.Config) {
			config.SetApiLibraries([]string{"foo"})
		}),
	).RunTestWithBp(t, `
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
			test: {
				enabled: true,
			},
		}
	`)

	testCases := []struct {
		scope              *apiScope
		apiContributions   []string
		fullApiSurfaceStub string
	}{
		{
			scope:              apiScopePublic,
			apiContributions:   []string{"foo.stubs.source.api.contribution"},
			fullApiSurfaceStub: "android_stubs_current",
		},
		{
			scope:              apiScopeSystem,
			apiContributions:   []string{"foo.stubs.source.system.api.contribution", "foo.stubs.source.api.contribution"},
			fullApiSurfaceStub: "android_system_stubs_current",
		},
		{
			scope:              apiScopeTest,
			apiContributions:   []string{"foo.stubs.source.test.api.contribution", "foo.stubs.source.system.api.contribution", "foo.stubs.source.api.contribution"},
			fullApiSurfaceStub: "android_test_stubs_current",
		},
		{
			scope:              apiScopeModuleLib,
			apiContributions:   []string{"foo.stubs.source.module_lib.api.contribution", "foo.stubs.source.system.api.contribution", "foo.stubs.source.api.contribution"},
			fullApiSurfaceStub: "android_module_lib_stubs_current_full.from-text",
		},
	}

	for _, c := range testCases {
		m := result.ModuleForTests(c.scope.apiLibraryModuleName("foo"), "android_common").Module().(*ApiLibrary)
		android.AssertArrayString(t, "Module expected to contain api contributions", c.apiContributions, m.properties.Api_contributions)
		android.AssertStringEquals(t, "Module expected to contain full api surface api library", c.fullApiSurfaceStub, *m.properties.Full_api_surface_stub)
	}
}

func TestStaticDepStubLibrariesVisibility(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
		android.FixtureMergeMockFs(
			map[string][]byte{
				"A.java": nil,
				"dir/Android.bp": []byte(
					`
					java_library {
						name: "bar",
						srcs: ["A.java"],
						libs: ["foo.stubs.from-source"],
					}
					`),
				"dir/A.java": nil,
			},
		).ExtendWithErrorHandler(
			android.FixtureExpectsAtLeastOneErrorMatchingPattern(
				`module "bar" variant "android_common": depends on //.:foo.stubs.from-source which is not visible to this module`)),
	).RunTestWithBp(t, `
		java_sdk_library {
			name: "foo",
			srcs: ["A.java"],
		}
	`)
}

func TestSdkLibraryDependency(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithPrebuiltApis(map[string][]string{
			"30": {"bar", "foo"},
		}),
	).RunTestWithBp(t,
		`
		java_sdk_library {
			name: "foo",
			srcs: ["a.java", "b.java"],
			api_packages: ["foo"],
		}

		java_sdk_library {
			name: "bar",
			srcs: ["c.java", "b.java"],
			libs: [
				"foo",
			],
			uses_libs: [
				"foo",
			],
		}
`)

	barPermissions := result.ModuleForTests("bar.xml", "android_common").Rule("java_sdk_xml")

	android.AssertStringDoesContain(t, "bar.xml java_sdk_xml command", barPermissions.RuleParams.Command, `dependency=\"foo\"`)
}
