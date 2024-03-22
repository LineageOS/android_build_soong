// Copyright 2023 Google Inc. All rights reserved.
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
	"testing"

	"android/soong/android"
)

func TestManifestMerger(t *testing.T) {
	bp := `
		android_app {
			name: "app",
			sdk_version: "current",
			srcs: ["app/app.java"],
			resource_dirs: ["app/res"],
			manifest: "app/AndroidManifest.xml",
			additional_manifests: ["app/AndroidManifest2.xml"],
			static_libs: ["direct", "direct_import"],
		}

		android_library {
			name: "direct",
			sdk_version: "current",
			srcs: ["direct/direct.java"],
			resource_dirs: ["direct/res"],
			manifest: "direct/AndroidManifest.xml",
			additional_manifests: ["direct/AndroidManifest2.xml"],
			static_libs: ["transitive", "transitive_import"],
		}

		android_library {
			name: "transitive",
			sdk_version: "current",
			srcs: ["transitive/transitive.java"],
			resource_dirs: ["transitive/res"],
			manifest: "transitive/AndroidManifest.xml",
			additional_manifests: ["transitive/AndroidManifest2.xml"],
		}

		android_library_import {
			name: "direct_import",
			sdk_version: "current",
			aars: ["direct_import.aar"],
			static_libs: ["direct_import_dep"],
		}

		android_library_import {
			name: "direct_import_dep",
			sdk_version: "current",
			aars: ["direct_import_dep.aar"],
		}

		android_library_import {
			name: "transitive_import",
			sdk_version: "current",
			aars: ["transitive_import.aar"],
			static_libs: ["transitive_import_dep"],
		}

		android_library_import {
			name: "transitive_import_dep",
			sdk_version: "current",
			aars: ["transitive_import_dep.aar"],
		}
	`

	result := PrepareForTestWithJavaDefaultModules.RunTestWithBp(t, bp)

	manifestMergerRule := result.ModuleForTests("app", "android_common").Rule("manifestMerger")
	android.AssertPathRelativeToTopEquals(t, "main manifest",
		"out/soong/.intermediates/app/android_common/manifest_fixer/AndroidManifest.xml",
		manifestMergerRule.Input)
	android.AssertPathsRelativeToTopEquals(t, "lib manifests",
		[]string{
			"app/AndroidManifest2.xml",
			"out/soong/.intermediates/direct/android_common/manifest_fixer/AndroidManifest.xml",
			"direct/AndroidManifest2.xml",
			"out/soong/.intermediates/transitive/android_common/manifest_fixer/AndroidManifest.xml",
			"transitive/AndroidManifest2.xml",
			"out/soong/.intermediates/transitive_import/android_common/aar/AndroidManifest.xml",
			"out/soong/.intermediates/direct_import/android_common/aar/AndroidManifest.xml",
			// TODO(b/288358614): Soong has historically not merged manifests from dependencies of
			// android_library_import modules.

		},
		manifestMergerRule.Implicits)
}

func TestManifestValuesApplicationIdSetsPackageName(t *testing.T) {
	bp := `
		android_test {
			name: "test",
			sdk_version: "current",
			srcs: ["app/app.java"],
			manifest: "test/AndroidManifest.xml",
			additional_manifests: ["test/AndroidManifest2.xml"],
			static_libs: ["direct"],
      test_suites: ["device-tests"],
      manifest_values:  {
        applicationId: "new_package_name"
      },
		}

		android_library {
			name: "direct",
			sdk_version: "current",
			srcs: ["direct/direct.java"],
			resource_dirs: ["direct/res"],
			manifest: "direct/AndroidManifest.xml",
			additional_manifests: ["direct/AndroidManifest2.xml"],
		}

	`

	result := PrepareForTestWithJavaDefaultModules.RunTestWithBp(t, bp)

	manifestMergerRule := result.ModuleForTests("test", "android_common").Rule("manifestMerger")
	android.AssertStringMatches(t,
		"manifest merger args",
		manifestMergerRule.Args["args"],
		"--property PACKAGE=new_package_name")
}
