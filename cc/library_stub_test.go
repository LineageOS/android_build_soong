// Copyright 2021 Google Inc. All rights reserved.
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
	_ "fmt"
	_ "sort"

	"testing"

	"android/soong/android"

	"github.com/google/blueprint"
)

func hasDirectDependency(t *testing.T, ctx *android.TestResult, from android.Module, to android.Module) bool {
	t.Helper()
	var found bool
	ctx.VisitDirectDeps(from, func(dep blueprint.Module) {
		if dep == to {
			found = true
		}
	})
	return found
}

func TestApiLibraryReplacesExistingModule(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
		}

		cc_library {
			name: "libbar",
		}

		cc_api_library {
			name: "libbar",
			src: "libbar.so",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [
				"libbar",
			],
			header_libs: [],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libbar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_shared").Module()
	libbarApiImport := ctx.ModuleForTests("libbar.apiimport", "android_arm64_armv8-a_shared").Module()

	android.AssertBoolEquals(t, "original library should not be linked", false, hasDirectDependency(t, ctx, libfoo, libbar))
	android.AssertBoolEquals(t, "Stub library from API surface should be linked", true, hasDirectDependency(t, ctx, libfoo, libbarApiImport))
}

func TestApiLibraryDoNotRequireOriginalModule(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
		}

		cc_api_library {
			name: "libbar",
			src: "libbar.so",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [
				"libbar",
			],
			header_libs: [],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libbarApiImport := ctx.ModuleForTests("libbar.apiimport", "android_arm64_armv8-a_shared").Module()

	android.AssertBoolEquals(t, "Stub library from API surface should be linked", true, hasDirectDependency(t, ctx, libfoo, libbarApiImport))
}

func TestApiLibraryShouldNotReplaceWithoutApiImport(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
		}

		cc_library {
			name: "libbar",
		}

		cc_api_library {
			name: "libbar",
			src: "libbar.so",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [],
			header_libs: [],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libbar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_shared").Module()
	libbarApiImport := ctx.ModuleForTests("libbar.apiimport", "android_arm64_armv8-a_shared").Module()

	android.AssertBoolEquals(t, "original library should be linked", true, hasDirectDependency(t, ctx, libfoo, libbar))
	android.AssertBoolEquals(t, "Stub library from API surface should not be linked", false, hasDirectDependency(t, ctx, libfoo, libbarApiImport))
}

func TestApiHeaderReplacesExistingModule(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			header_libs: ["libfoo_headers"],
		}

		cc_api_library {
			name: "libfoo",
			header_libs: ["libfoo_headers"],
			src: "libfoo.so",
		}

		cc_library_headers {
			name: "libfoo_headers",
		}

		cc_api_headers {
			name: "libfoo_headers",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [
				"libfoo",
			],
			header_libs: [
				"libfoo_headers",
			],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libfooApiImport := ctx.ModuleForTests("libfoo.apiimport", "android_arm64_armv8-a_shared").Module()
	libfooHeader := ctx.ModuleForTests("libfoo_headers", "android_arm64_armv8-a").Module()
	libfooHeaderApiImport := ctx.ModuleForTests("libfoo_headers.apiimport", "android_arm64_armv8-a").Module()

	android.AssertBoolEquals(t, "original header should not be used for original library", false, hasDirectDependency(t, ctx, libfoo, libfooHeader))
	android.AssertBoolEquals(t, "Header from API surface should be used for original library", true, hasDirectDependency(t, ctx, libfoo, libfooHeaderApiImport))
	android.AssertBoolEquals(t, "original header should not be used for library imported from API surface", false, hasDirectDependency(t, ctx, libfooApiImport, libfooHeader))
	android.AssertBoolEquals(t, "Header from API surface should be used for library imported from API surface", true, hasDirectDependency(t, ctx, libfooApiImport, libfooHeaderApiImport))
}

func TestApiHeadersDoNotRequireOriginalModule(t *testing.T) {
	bp := `
	cc_library {
		name: "libfoo",
		header_libs: ["libfoo_headers"],
	}

	cc_api_headers {
		name: "libfoo_headers",
	}

	api_imports {
		name: "api_imports",
		shared_libs: [
			"libfoo",
		],
		header_libs: [
			"libfoo_headers",
		],
	}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libfooHeaderApiImport := ctx.ModuleForTests("libfoo_headers.apiimport", "android_arm64_armv8-a").Module()

	android.AssertBoolEquals(t, "Header from API surface should be used for original library", true, hasDirectDependency(t, ctx, libfoo, libfooHeaderApiImport))
}

func TestApiHeadersShouldNotReplaceWithoutApiImport(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			header_libs: ["libfoo_headers"],
		}

		cc_library_headers {
			name: "libfoo_headers",
		}

		cc_api_headers {
			name: "libfoo_headers",
		}

		api_imports {
			name: "api_imports",
			shared_libs: [
				"libfoo",
			],
			header_libs: [],
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
	libfooHeader := ctx.ModuleForTests("libfoo_headers", "android_arm64_armv8-a").Module()
	libfooHeaderApiImport := ctx.ModuleForTests("libfoo_headers.apiimport", "android_arm64_armv8-a").Module()

	android.AssertBoolEquals(t, "original header should be used for original library", true, hasDirectDependency(t, ctx, libfoo, libfooHeader))
	android.AssertBoolEquals(t, "Header from API surface should not be used for original library", false, hasDirectDependency(t, ctx, libfoo, libfooHeaderApiImport))
}
