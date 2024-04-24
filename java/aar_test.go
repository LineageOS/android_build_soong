// Copyright 2022 Google Inc. All rights reserved.
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

func TestAarImportProducesJniPackages(t *testing.T) {
	ctx := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
	).RunTestWithBp(t, `
		android_library_import {
			name: "aar-no-jni",
			aars: ["aary.aar"],
		}
		android_library_import {
			name: "aar-jni",
			aars: ["aary.aar"],
			extract_jni: true,
		}`)

	testCases := []struct {
		name       string
		hasPackage bool
	}{
		{
			name:       "aar-no-jni",
			hasPackage: false,
		},
		{
			name:       "aar-jni",
			hasPackage: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			appMod := ctx.Module(tc.name, "android_common")
			appTestMod := ctx.ModuleForTests(tc.name, "android_common")

			info, ok := android.SingletonModuleProvider(ctx, appMod, JniPackageProvider)
			if !ok {
				t.Errorf("expected android_library_import to have JniPackageProvider")
			}

			if !tc.hasPackage {
				if len(info.JniPackages) != 0 {
					t.Errorf("expected JniPackages to be empty, but got %v", info.JniPackages)
				}
				outputFile := "arm64-v8a_jni.zip"
				jniOutputLibZip := appTestMod.MaybeOutput(outputFile)
				if jniOutputLibZip.Rule != nil {
					t.Errorf("did not expect an output file, but found %v", outputFile)
				}
				return
			}

			if len(info.JniPackages) != 1 {
				t.Errorf("expected a single JniPackage, but got %v", info.JniPackages)
			}

			outputFile := info.JniPackages[0].String()
			jniOutputLibZip := appTestMod.Output(outputFile)
			if jniOutputLibZip.Rule == nil {
				t.Errorf("did not find output file %v", outputFile)
			}
		})
	}
}

func TestLibraryFlagsPackages(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForJavaTest,
	).RunTestWithBp(t, `
		android_library {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
			flags_packages: [
				"bar",
				"baz",
			],
		}
		aconfig_declarations {
			name: "bar",
			package: "com.example.package.bar",
			container: "com.android.foo",
			srcs: [
				"bar.aconfig",
			],
		}
		aconfig_declarations {
			name: "baz",
			package: "com.example.package.baz",
			container: "com.android.foo",
			srcs: [
				"baz.aconfig",
			],
		}
	`)

	foo := result.ModuleForTests("foo", "android_common")

	// android_library module depends on aconfig_declarations listed in flags_packages
	android.AssertBoolEquals(t, "foo expected to depend on bar", true,
		CheckModuleHasDependency(t, result.TestContext, "foo", "android_common", "bar"))

	android.AssertBoolEquals(t, "foo expected to depend on baz", true,
		CheckModuleHasDependency(t, result.TestContext, "foo", "android_common", "baz"))

	aapt2LinkRule := foo.Rule("android/soong/java.aapt2Link")
	linkInFlags := aapt2LinkRule.Args["inFlags"]
	android.AssertStringDoesContain(t,
		"aapt2 link command expected to pass feature flags arguments",
		linkInFlags,
		"--feature-flags @out/soong/.intermediates/bar/intermediate.txt --feature-flags @out/soong/.intermediates/baz/intermediate.txt",
	)
}

func TestAndroidLibraryOutputFilesRel(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
	).RunTestWithBp(t, `
		android_library {
			name: "foo",
			srcs: ["a.java"],
			java_resources: ["foo.txt"],
		}

		android_library_import {
			name: "bar",
			aars: ["bar_prebuilt.aar"],

		}

		android_library_import {
			name: "baz",
			aars: ["baz_prebuilt.aar"],
			static_libs: ["foo", "bar"],
		}
	`)

	foo := result.ModuleForTests("foo", "android_common")
	bar := result.ModuleForTests("bar", "android_common")
	baz := result.ModuleForTests("baz", "android_common")

	fooOutputPath := android.OutputFileForModule(android.PathContext(nil), foo.Module(), "")
	barOutputPath := android.OutputFileForModule(android.PathContext(nil), bar.Module(), "")
	bazOutputPath := android.OutputFileForModule(android.PathContext(nil), baz.Module(), "")

	android.AssertPathRelativeToTopEquals(t, "foo output path",
		"out/soong/.intermediates/foo/android_common/withres/foo.jar", fooOutputPath)
	android.AssertPathRelativeToTopEquals(t, "bar output path",
		"out/soong/.intermediates/bar/android_common/aar/bar.jar", barOutputPath)
	android.AssertPathRelativeToTopEquals(t, "baz output path",
		"out/soong/.intermediates/baz/android_common/withres/baz.jar", bazOutputPath)

	android.AssertStringEquals(t, "foo relative output path",
		"foo.jar", fooOutputPath.Rel())
	android.AssertStringEquals(t, "bar relative output path",
		"bar.jar", barOutputPath.Rel())
	android.AssertStringEquals(t, "baz relative output path",
		"baz.jar", bazOutputPath.Rel())
}
