// Copyright (C) 2021 The Android Open Source Project
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

package apex

import (
	"fmt"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/java"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// Contains tests for platform_bootclasspath logic from java/platform_bootclasspath.go that requires
// apexes.

var prepareForTestWithPlatformBootclasspath = android.GroupFixturePreparers(
	java.PrepareForTestWithDexpreopt,
	PrepareForTestWithApexBuildComponents,
)

func TestPlatformBootclasspath_Fragments(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		prepareForTestWithMyapex,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("foo"),
		java.FixtureConfigureBootJars("myapex:bar"),
		android.FixtureWithRootAndroidBp(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
				fragments: [
					{
						apex: "myapex",
						module:"bar-fragment",
					},
				],
				hidden_api: {
					unsupported: [
							"unsupported.txt",
					],
					removed: [
							"removed.txt",
					],
					max_target_r_low_priority: [
							"max-target-r-low-priority.txt",
					],
					max_target_q: [
							"max-target-q.txt",
					],
					max_target_p: [
							"max-target-p.txt",
					],
					max_target_o_low_priority: [
							"max-target-o-low-priority.txt",
					],
					blocked: [
							"blocked.txt",
					],
					unsupported_packages: [
							"unsupported-packages.txt",
					],
				},
			}

			apex {
				name: "myapex",
				key: "myapex.key",
				bootclasspath_fragments: [
					"bar-fragment",
				],
				updatable: false,
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}

			bootclasspath_fragment {
				name: "bar-fragment",
				contents: ["bar"],
				apex_available: ["myapex"],
				api: {
					stub_libs: ["foo"],
				},
				hidden_api: {
					unsupported: [
							"bar-unsupported.txt",
					],
					removed: [
							"bar-removed.txt",
					],
					max_target_r_low_priority: [
							"bar-max-target-r-low-priority.txt",
					],
					max_target_q: [
							"bar-max-target-q.txt",
					],
					max_target_p: [
							"bar-max-target-p.txt",
					],
					max_target_o_low_priority: [
							"bar-max-target-o-low-priority.txt",
					],
					blocked: [
							"bar-blocked.txt",
					],
					unsupported_packages: [
							"bar-unsupported-packages.txt",
					],
				},
			}

			java_library {
				name: "bar",
				apex_available: ["myapex"],
				srcs: ["a.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
				permitted_packages: ["bar"],
			}

			java_sdk_library {
				name: "foo",
				srcs: ["a.java"],
				public: {
					enabled: true,
				},
				compile_dex: true,
			}
		`),
	).RunTest(t)

	pbcp := result.Module("platform-bootclasspath", "android_common")
	info := result.ModuleProvider(pbcp, java.MonolithicHiddenAPIInfoProvider).(java.MonolithicHiddenAPIInfo)

	for _, category := range java.HiddenAPIFlagFileCategories {
		name := category.PropertyName
		message := fmt.Sprintf("category %s", name)
		filename := strings.ReplaceAll(name, "_", "-")
		expected := []string{fmt.Sprintf("%s.txt", filename), fmt.Sprintf("bar-%s.txt", filename)}
		android.AssertPathsRelativeToTopEquals(t, message, expected, info.FlagsFilesByCategory[category])
	}

	android.AssertPathsRelativeToTopEquals(t, "stub flags", []string{"out/soong/.intermediates/bar-fragment/android_common_apex10000/modular-hiddenapi/stub-flags.csv"}, info.StubFlagsPaths)
	android.AssertPathsRelativeToTopEquals(t, "annotation flags", []string{"out/soong/.intermediates/bar-fragment/android_common_apex10000/modular-hiddenapi/annotation-flags.csv"}, info.AnnotationFlagsPaths)
	android.AssertPathsRelativeToTopEquals(t, "metadata flags", []string{"out/soong/.intermediates/bar-fragment/android_common_apex10000/modular-hiddenapi/metadata.csv"}, info.MetadataPaths)
	android.AssertPathsRelativeToTopEquals(t, "index flags", []string{"out/soong/.intermediates/bar-fragment/android_common_apex10000/modular-hiddenapi/index.csv"}, info.IndexPaths)
	android.AssertPathsRelativeToTopEquals(t, "all flags", []string{"out/soong/.intermediates/bar-fragment/android_common_apex10000/modular-hiddenapi/all-flags.csv"}, info.AllFlagsPaths)
}

func TestPlatformBootclasspathDependencies(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		prepareForTestWithArtApex,
		prepareForTestWithMyapex,
		// Configure some libraries in the art and framework boot images.
		java.FixtureConfigureBootJars("com.android.art:baz", "com.android.art:quuz", "platform:foo"),
		java.FixtureConfigureUpdatableBootJars("myapex:bar"),
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
		apex {
			name: "com.android.art",
			key: "com.android.art.key",
 			bootclasspath_fragments: [
				"art-bootclasspath-fragment",
			],
			updatable: false,
		}

		apex_key {
			name: "com.android.art.key",
			public_key: "com.android.art.avbpubkey",
			private_key: "com.android.art.pem",
		}

		bootclasspath_fragment {
			name: "art-bootclasspath-fragment",
			apex_available: [
				"com.android.art",
			],
			contents: [
				"baz",
				"quuz",
			],
		}

		java_library {
			name: "baz",
			apex_available: [
				"com.android.art",
			],
			srcs: ["b.java"],
			installable: true,
		}

		// Add a java_import that is not preferred and so won't have an appropriate apex variant created
		// for it to make sure that the platform_bootclasspath doesn't try and add a dependency onto it.
		java_import {
			name: "baz",
			apex_available: [
				"com.android.art",
			],
			jars: ["b.jar"],
		}

		java_library {
			name: "quuz",
			apex_available: [
				"com.android.art",
			],
			srcs: ["b.java"],
			installable: true,
		}

		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: [
				"bar",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library {
			name: "foo",
			srcs: ["b.java"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
			apex_available: ["myapex"],
			permitted_packages: ["bar"],
		}

		platform_bootclasspath {
			name: "myplatform-bootclasspath",

			fragments: [
				{
					apex: "com.android.art",
					module: "art-bootclasspath-fragment",
				},
			],
		}
`,
	)

	java.CheckPlatformBootclasspathModules(t, result, "myplatform-bootclasspath", []string{
		// The configured contents of BootJars.
		"com.android.art:baz",
		"com.android.art:quuz",
		"platform:foo",

		// The configured contents of UpdatableBootJars.
		"myapex:bar",
	})

	java.CheckPlatformBootclasspathFragments(t, result, "myplatform-bootclasspath", []string{
		`com.android.art:art-bootclasspath-fragment`,
	})

	// Make sure that the myplatform-bootclasspath has the correct dependencies.
	CheckModuleDependencies(t, result.TestContext, "myplatform-bootclasspath", "android_common", []string{
		// The following are stubs.
		`platform:android_stubs_current`,
		`platform:android_system_stubs_current`,
		`platform:android_test_stubs_current`,
		`platform:legacy.core.platform.api.stubs`,

		// Needed for generating the boot image.
		`platform:dex2oatd`,

		// The configured contents of BootJars.
		`com.android.art:baz`,
		`com.android.art:quuz`,
		`platform:foo`,

		// The configured contents of UpdatableBootJars.
		`myapex:bar`,

		// The fragments.
		`com.android.art:art-bootclasspath-fragment`,
	})
}

// TestPlatformBootclasspath_AlwaysUsePrebuiltSdks verifies that the build does not fail when
// AlwaysUsePrebuiltSdk() returns true. The structure of the modules in this test matches what
// currently exists in some places in the Android build but it is not the intended structure. It is
// in fact an invalid structure that should cause build failures. However, fixing that structure
// will take too long so in the meantime this tests the workarounds to avoid build breakages.
//
// The main issues with this structure are:
// 1. There is no prebuilt_bootclasspath_fragment referencing the "foo" java_sdk_library_import.
// 2. There is no prebuilt_apex/apex_set which makes the dex implementation jar available to the
//    prebuilt_bootclasspath_fragment and the "foo" java_sdk_library_import.
//
// Together these cause the following symptoms:
// 1. The "foo" java_sdk_library_import does not have a dex implementation jar.
// 2. The "foo" java_sdk_library_import does not have a myapex variant.
//
// TODO(b/179354495): Fix the structure in this test once the main Android build has been fixed.
func TestPlatformBootclasspath_AlwaysUsePrebuiltSdks(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		prepareForTestWithMyapex,
		// Configure two libraries, the first is a java_sdk_library whose prebuilt will be used because
		// of AlwaysUsePrebuiltsSdk() but does not have an appropriate apex variant and does not provide
		// a boot dex jar. The second is a normal library that is unaffected. The order matters because
		// if the dependency on myapex:foo is filtered out because of either of those conditions then
		// the dependencies resolved by the platform_bootclasspath will not match the configured list
		// and so will fail the test.
		java.FixtureConfigureUpdatableBootJars("myapex:foo", "myapex:bar"),
		java.PrepareForTestWithJavaSdkLibraryFiles,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Always_use_prebuilt_sdks = proptools.BoolPtr(true)
		}),
		java.FixtureWithPrebuiltApis(map[string][]string{
			"current": {},
			"30":      {"foo"},
		}),
	).RunTestWithBp(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			bootclasspath_fragments: [
				"mybootclasspath-fragment",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
			apex_available: ["myapex"],
			permitted_packages: ["bar"],
		}

		java_sdk_library {
			name: "foo",
			srcs: ["b.java"],
			shared_library: false,
			public: {
				enabled: true,
			},
			apex_available: ["myapex"],
			permitted_packages: ["foo"],
		}

		// A prebuilt java_sdk_library_import that is not preferred by default but will be preferred
		// because AlwaysUsePrebuiltSdks() is true.
		java_sdk_library_import {
			name: "foo",
			prefer: false,
			shared_library: false,
			public: {
				jars: ["sdk_library/public/foo-stubs.jar"],
				stub_srcs: ["sdk_library/public/foo_stub_sources"],
				current_api: "sdk_library/public/foo.txt",
				removed_api: "sdk_library/public/foo-removed.txt",
				sdk_version: "current",
			},
			apex_available: ["myapex"],
		}

		// This always depends on the source foo module, its dependencies are not affected by the
		// AlwaysUsePrebuiltSdks().
		bootclasspath_fragment {
			name: "mybootclasspath-fragment",
			apex_available: [
				"myapex",
			],
			contents: [
				"foo", "bar",
			],
		}

		platform_bootclasspath {
			name: "myplatform-bootclasspath",
		}
`,
	)

	java.CheckPlatformBootclasspathModules(t, result, "myplatform-bootclasspath", []string{
		// The configured contents of BootJars.
		"platform:prebuilt_foo", // Note: This is the platform not myapex variant.
		"myapex:bar",
	})

	// Make sure that the myplatform-bootclasspath has the correct dependencies.
	CheckModuleDependencies(t, result.TestContext, "myplatform-bootclasspath", "android_common", []string{
		// The following are stubs.
		"platform:prebuilt_sdk_public_current_android",
		"platform:prebuilt_sdk_system_current_android",
		"platform:prebuilt_sdk_test_current_android",

		// Not a prebuilt as no prebuilt existed when it was added.
		"platform:legacy.core.platform.api.stubs",

		// Needed for generating the boot image.
		`platform:dex2oatd`,

		// The platform_bootclasspath intentionally adds dependencies on both source and prebuilt
		// modules when available as it does not know which one will be preferred.
		//
		// The source module has an APEX variant but the prebuilt does not.
		"myapex:foo",
		"platform:prebuilt_foo",

		// Only a source module exists.
		"myapex:bar",
	})
}

// CheckModuleDependencies checks the dependencies of the selected module against the expected list.
//
// The expected list must be a list of strings of the form "<apex>:<module>", where <apex> is the
// name of the apex, or platform is it is not part of an apex and <module> is the module name.
func CheckModuleDependencies(t *testing.T, ctx *android.TestContext, name, variant string, expected []string) {
	t.Helper()
	module := ctx.ModuleForTests(name, variant).Module()
	modules := []android.Module{}
	ctx.VisitDirectDeps(module, func(m blueprint.Module) {
		modules = append(modules, m.(android.Module))
	})

	pairs := java.ApexNamePairsFromModules(ctx, modules)
	android.AssertDeepEquals(t, "module dependencies", expected, pairs)
}
