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
	"android/soong/android"
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

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
	result.ModuleForTests("foo", "android_common")
	result.ModuleForTests(apiScopePublic.stubsLibraryModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopeSystem.stubsLibraryModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopeTest.stubsLibraryModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopePublic.stubsSourceModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopeSystem.stubsSourceModuleName("foo"), "android_common")
	result.ModuleForTests(apiScopeTest.stubsSourceModuleName("foo"), "android_common")
	result.ModuleForTests("foo"+sdkXmlFileSuffix, "android_common")
	result.ModuleForTests("foo.api.public.28", "")
	result.ModuleForTests("foo.api.system.28", "")
	result.ModuleForTests("foo.api.test.28", "")

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
		sdkLibs := quxLib.ClassLoaderContexts().UsesLibs()
		android.AssertDeepEquals(t, "qux exports", []string{"foo", "bar", "fred", "quuz"}, sdkLibs)
	}
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
		verify("sdklib", expectation.lib, expectation.on_impl_classpath, expectation.in_impl_combined)
		verify("sdklib.impl", expectation.lib, expectation.on_impl_classpath, expectation.in_impl_combined)

		stubName := apiScopePublic.stubsLibraryModuleName("sdklib")
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

func TestJavaSdkLibrary_UseSourcesFromAnotherSdkLibrary(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles,
		FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
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

func TestJavaSdkLibraryImport_Preferred(t *testing.T) {
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
			prefer: true,
			public: {
				jars: ["a.jar"],
			},
		}
		`)

	CheckModuleDependencies(t, result.TestContext, "sdklib", "android_common", []string{
		`dex2oatd`,
		`prebuilt_sdklib`,
		`sdklib.impl`,
		`sdklib.stubs`,
		`sdklib.stubs.source`,
		`sdklib.xml`,
	})

	CheckModuleDependencies(t, result.TestContext, "prebuilt_sdklib", "android_common", []string{
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
