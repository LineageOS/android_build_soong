// Copyright 2020 Google Inc. All rights reserved.
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

package cc

import (
	"fmt"
	"testing"

	"android/soong/android"

	"github.com/google/blueprint"
)

func TestLibraryHeaders(t *testing.T) {
	bp := `
		%s {
			name: "headers",
			export_include_dirs: ["my_include"],
		}
		cc_library_static {
			name: "lib",
			srcs: ["foo.c"],
			header_libs: ["headers"],
		}
	`

	for _, headerModule := range []string{"cc_library_headers", "cc_prebuilt_library_headers"} {
		t.Run(headerModule, func(t *testing.T) {
			ctx := testCc(t, fmt.Sprintf(bp, headerModule))

			// test if header search paths are correctly added
			cc := ctx.ModuleForTests("lib", "android_arm64_armv8-a_static").Rule("cc")
			android.AssertStringDoesContain(t, "cFlags for lib module", cc.Args["cFlags"], " -Imy_include ")

			// Test that there's a valid AndroidMk entry.
			headers := ctx.ModuleForTests("headers", "android_arm64_armv8-a").Module()
			e := android.AndroidMkEntriesForTest(t, ctx, headers)[0]

			// This duplicates the tests done in AndroidMkEntries.write. It would be
			// better to test its output, but there are no test functions that capture that.
			android.AssertBoolEquals(t, "AndroidMkEntries.Disabled", false, e.Disabled)
			android.AssertBoolEquals(t, "AndroidMkEntries.OutputFile.Valid()", true, e.OutputFile.Valid())

			android.AssertStringListContains(t, "LOCAL_EXPORT_CFLAGS for headers module", e.EntryMap["LOCAL_EXPORT_CFLAGS"], "-Imy_include")
		})
	}
}

func TestPrebuiltLibraryHeadersPreferred(t *testing.T) {
	t.Parallel()
	bp := `
		cc_library_headers {
			name: "headers",
			export_include_dirs: ["my_include"],
		}
		cc_prebuilt_library_headers {
			name: "headers",
			prefer: %t,
			export_include_dirs: ["my_include"],
		}
		cc_library_static {
			name: "lib",
			srcs: ["foo.c"],
			header_libs: ["headers"],
		}
	`

	for _, prebuiltPreferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("prebuilt prefer %t", prebuiltPreferred), func(t *testing.T) {
			ctx := testCc(t, fmt.Sprintf(bp, prebuiltPreferred))
			lib := ctx.ModuleForTests("lib", "android_arm64_armv8-a_static")
			sourceDep := ctx.ModuleForTests("headers", "android_arm64_armv8-a")
			prebuiltDep := ctx.ModuleForTests("prebuilt_headers", "android_arm64_armv8-a")
			hasSourceDep := false
			hasPrebuiltDep := false
			ctx.VisitDirectDeps(lib.Module(), func(dep blueprint.Module) {
				if dep == sourceDep.Module() {
					hasSourceDep = true
				}
				if dep == prebuiltDep.Module() {
					hasPrebuiltDep = true
				}
			})
			android.AssertBoolEquals(t, "depends on source headers", !prebuiltPreferred, hasSourceDep)
			android.AssertBoolEquals(t, "depends on prebuilt headers", prebuiltPreferred, hasPrebuiltDep)
		})
	}
}
