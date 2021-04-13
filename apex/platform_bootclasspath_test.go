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
	"testing"

	"android/soong/android"
	"android/soong/java"
	"github.com/google/blueprint"
)

// Contains tests for platform_bootclasspath logic from java/platform_bootclasspath.go that requires
// apexes.

var prepareForTestWithPlatformBootclasspath = android.GroupFixturePreparers(
	java.PrepareForTestWithDexpreopt,
	PrepareForTestWithApexBuildComponents,
)

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
		"com.android.art:baz",
		"com.android.art:quuz",
		"platform:foo",
		"myapex:bar",
	})

	java.CheckPlatformBootclasspathFragments(t, result, "myplatform-bootclasspath", []string{
		`com.android.art:art-bootclasspath-fragment`,
	})

	// Make sure that the myplatform-bootclasspath has the correct dependencies.
	CheckModuleDependencies(t, result.TestContext, "myplatform-bootclasspath", "android_common", []string{
		`platform:dex2oatd`,
		`com.android.art:baz`,
		`com.android.art:quuz`,
		`platform:foo`,
		`myapex:bar`,
		`com.android.art:art-bootclasspath-fragment`,
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
