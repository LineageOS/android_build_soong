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

package cc

import (
	"testing"

	"github.com/google/blueprint"

	"android/soong/android"
)

func TestNdkHeaderDependency(t *testing.T) {
	isDep := func(ctx *android.TestResult, from, toExpected android.Module) bool {
		foundDep := false
		ctx.VisitDirectDeps(from, func(toActual blueprint.Module) {
			if toExpected.Name() == toActual.Name() {
				foundDep = true
			}
		})
		return foundDep
	}
	bp := `
	ndk_library {
		name: "libfoo",
		first_version: "29",
		symbol_file: "libfoo.map.txt",
		export_header_libs: ["libfoo_headers"],
	}
	ndk_headers {
		name: "libfoo_headers",
		srcs: ["foo.h"],
		license: "NOTICE",
	}
	//This module is needed since Soong creates a dep edge on source
	cc_library {
		name: "libfoo",
	}
	`
	ctx := prepareForCcTest.RunTestWithBp(t, bp)
	libfoo := ctx.ModuleForTests("libfoo.ndk", "android_arm64_armv8-a_sdk_shared")
	libfoo_headers := ctx.ModuleForTests("libfoo_headers", "")
	android.AssertBoolEquals(t, "Could not find headers of ndk_library", true, isDep(ctx, libfoo.Module(), libfoo_headers.Module()))
}
