// Copyright 2020 The Android Open Source Project
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

package rust

import (
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

func TestClippy(t *testing.T) {

	bp := `
		// foo uses the default value of clippy_lints
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
		// bar forces the use of the "android" lint set
		rust_library {
			name: "libbar",
			srcs: ["foo.rs"],
			crate_name: "bar",
			clippy_lints: "android",
		}
		// foobar explicitly disable clippy
		rust_library {
			name: "libfoobar",
			srcs: ["foo.rs"],
			crate_name: "foobar",
			clippy_lints: "none",
		}`

	bp = bp + GatherRequiredDepsForTest()
	bp = bp + cc.GatherRequiredDepsForTest(android.NoOsType)

	fs := map[string][]byte{
		// Reuse the same blueprint file for subdirectories.
		"external/Android.bp": []byte(bp),
		"hardware/Android.bp": []byte(bp),
	}

	var clippyLintTests = []struct {
		modulePath string
		fooFlags   string
	}{
		{"", "${config.ClippyDefaultLints}"},
		{"external/", ""},
		{"hardware/", "${config.ClippyVendorLints}"},
	}

	for _, tc := range clippyLintTests {
		t.Run("path="+tc.modulePath, func(t *testing.T) {

			config := android.TestArchConfig(t.TempDir(), nil, bp, fs)
			ctx := CreateTestContext(config)
			ctx.Register()
			_, errs := ctx.ParseFileList(".", []string{tc.modulePath + "Android.bp"})
			android.FailIfErrored(t, errs)
			_, errs = ctx.PrepareBuildActions(config)
			android.FailIfErrored(t, errs)

			r := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_dylib").MaybeRule("clippy")
			if r.Args["clippyFlags"] != tc.fooFlags {
				t.Errorf("Incorrect flags for libfoo: %q, want %q", r.Args["clippyFlags"], tc.fooFlags)
			}

			r = ctx.ModuleForTests("libbar", "android_arm64_armv8-a_dylib").MaybeRule("clippy")
			if r.Args["clippyFlags"] != "${config.ClippyDefaultLints}" {
				t.Errorf("Incorrect flags for libbar: %q, want %q", r.Args["clippyFlags"], "${config.ClippyDefaultLints}")
			}

			r = ctx.ModuleForTests("libfoobar", "android_arm64_armv8-a_dylib").MaybeRule("clippy")
			if r.Rule != nil {
				t.Errorf("libfoobar is setup to use clippy when explicitly disabled: clippyFlags=%q", r.Args["clippyFlags"])
			}

		})
	}
}
