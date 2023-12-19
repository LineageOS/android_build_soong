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
	"android/soong/android"
	"testing"
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
