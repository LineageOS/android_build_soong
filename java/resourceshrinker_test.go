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

func TestShrinkResourcesArgs(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
	).RunTestWithBp(t, `
		android_app {
			name: "app_shrink",
			platform_apis: true,
			optimize: {
				shrink_resources: true,
			}
		}

		android_app {
			name: "app_no_shrink",
			platform_apis: true,
			optimize: {
				shrink_resources: false,
			}
		}
	`)

	appShrink := result.ModuleForTests("app_shrink", "android_common")
	appShrinkResources := appShrink.Rule("shrinkResources")
	android.AssertStringDoesContain(t, "expected shrinker.xml in app_shrink resource shrinker flags",
		appShrinkResources.Args["raw_resources"], "shrinker.xml")

	appNoShrink := result.ModuleForTests("app_no_shrink", "android_common")
	if appNoShrink.MaybeRule("shrinkResources").Rule != nil {
		t.Errorf("unexpected shrinkResources rule for app_no_shrink")
	}
}
