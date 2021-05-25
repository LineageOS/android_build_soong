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
)

var prepareForTestWithSystemserverclasspathFragment = android.GroupFixturePreparers(
	java.PrepareForTestWithDexpreopt,
	PrepareForTestWithApexBuildComponents,
)

func TestSystemserverclasspathFragmentContents(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForTestWithSystemserverclasspathFragment,
		prepareForTestWithMyapex,
	).RunTestWithBp(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			systemserverclasspath_fragments: [
				"mysystemserverclasspathfragment",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "foo",
			srcs: ["b.java"],
			installable: true,
			apex_available: [
				"myapex",
			],
		}

		systemserverclasspath_fragment {
			name: "mysystemserverclasspathfragment",
			contents: [
				"foo",
			],
			apex_available: [
				"myapex",
			],
		}
	`)

	ensureExactContents(t, result.TestContext, "myapex", "android_common_myapex_image", []string{
		"etc/classpaths/systemserverclasspath.pb",
		"javalib/foo.jar",
	})

	java.CheckModuleDependencies(t, result.TestContext, "myapex", "android_common_myapex_image", []string{
		`myapex.key`,
		`mysystemserverclasspathfragment`,
	})
}
