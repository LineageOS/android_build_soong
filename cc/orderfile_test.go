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

package cc

import (
	"strings"
	"testing"

	"android/soong/android"
)

func TestOrderfileProfileSharedLibrary(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "libTest",
		srcs: ["test.c"],
		orderfile : {
			instrumentation: true,
			load_order_file: false,
			order_file_path: "",
		},
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	expectedCFlag := "-forder-file-instrumentation"

	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared")

	// Check cFlags of orderfile-enabled module
	cFlags := libTest.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libTest' to enable orderfile, but did not find %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check ldFlags of orderfile-enabled module
	ldFlags := libTest.Rule("ld").Args["ldFlags"]
	if !strings.Contains(ldFlags, expectedCFlag) {
		t.Errorf("Expected 'libTest' to enable orderfile, but did not find %q in ldflags %q", expectedCFlag, ldFlags)
	}
}

func TestOrderfileLoadSharedLibrary(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "libTest",
		srcs: ["test.c"],
		orderfile : {
			instrumentation: true,
			load_order_file: true,
			order_file_path: "libTest.orderfile",
		},
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		android.FixtureAddTextFile("toolchain/pgo-profiles/orderfiles/libTest.orderfile", "TEST"),
	).RunTestWithBp(t, bp)

	expectedCFlag := "-Wl,--symbol-ordering-file=toolchain/pgo-profiles/orderfiles/libTest.orderfile"

	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared")

	// Check ldFlags of orderfile-enabled module
	ldFlags := libTest.Rule("ld").Args["ldFlags"]
	if !strings.Contains(ldFlags, expectedCFlag) {
		t.Errorf("Expected 'libTest' to load orderfile, but did not find %q in ldflags %q", expectedCFlag, ldFlags)
	}
}

func TestOrderfileProfileBinary(t *testing.T) {
	t.Parallel()
	bp := `
	cc_binary {
		name: "test",
		srcs: ["test.c"],
		orderfile : {
			instrumentation: true,
			load_order_file: false,
			order_file_path: "",
		},
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	expectedCFlag := "-forder-file-instrumentation"

	test := result.ModuleForTests("test", "android_arm64_armv8-a")

	// Check cFlags of orderfile-enabled module
	cFlags := test.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'test' to enable orderfile, but did not find %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check ldFlags of orderfile-enabled module
	ldFlags := test.Rule("ld").Args["ldFlags"]
	if !strings.Contains(ldFlags, expectedCFlag) {
		t.Errorf("Expected 'test' to enable orderfile, but did not find %q in ldflags %q", expectedCFlag, ldFlags)
	}
}

func TestOrderfileLoadBinary(t *testing.T) {
	t.Parallel()
	bp := `
	cc_binary {
		name: "test",
		srcs: ["test.c"],
		orderfile : {
			instrumentation: true,
			load_order_file: true,
			order_file_path: "test.orderfile",
		},
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		android.FixtureAddTextFile("toolchain/pgo-profiles/orderfiles/test.orderfile", "TEST"),
	).RunTestWithBp(t, bp)

	expectedCFlag := "-Wl,--symbol-ordering-file=toolchain/pgo-profiles/orderfiles/test.orderfile"

	test := result.ModuleForTests("test", "android_arm64_armv8-a")

	// Check ldFlags of orderfile-enabled module
	ldFlags := test.Rule("ld").Args["ldFlags"]
	if !strings.Contains(ldFlags, expectedCFlag) {
		t.Errorf("Expected 'test' to load orderfile, but did not find %q in ldflags %q", expectedCFlag, ldFlags)
	}
}

// Profile flags should propagate through static libraries
func TestOrderfileProfilePropagateStaticDeps(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "libTest",
		srcs: ["test.c"],
		static_libs: ["libFoo"],
		orderfile : {
			instrumentation: true,
			load_order_file: false,
			order_file_path: "",
		},
	}

	cc_library_static {
		name: "libFoo",
		srcs: ["foo.c"],
		static_libs: ["libBar"],
	}

	cc_library_static {
		name: "libBar",
		srcs: ["bar.c"],
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	expectedCFlag := "-forder-file-instrumentation"

	// Check cFlags of orderfile-enabled module
	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared")

	cFlags := libTest.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libTest' to enable orderfile, but did not find %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check cFlags of orderfile variant static libraries
	libFooOfVariant := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static_orderfile")
	libBarOfVariant := result.ModuleForTests("libBar", "android_arm64_armv8-a_static_orderfile")

	cFlags = libFooOfVariant.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libFooOfVariant' to enable orderfile, but did not find %q in cflags %q", expectedCFlag, cFlags)
	}

	cFlags = libBarOfVariant.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libBarOfVariant' to enable orderfile, but did not find %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check dependency edge from orderfile-enabled module to orderfile variant static libraries
	if !hasDirectDep(result, libTest.Module(), libFooOfVariant.Module()) {
		t.Errorf("libTest missing dependency on orderfile variant of libFoo")
	}

	if !hasDirectDep(result, libFooOfVariant.Module(), libBarOfVariant.Module()) {
		t.Errorf("libTest missing dependency on orderfile variant of libBar")
	}

	// Check cFlags of the non-orderfile variant static libraries
	libFoo := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static")
	libBar := result.ModuleForTests("libBar", "android_arm64_armv8-a_static")

	cFlags = libFoo.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libFoo' to not enable orderfile, but did find %q in cflags %q", expectedCFlag, cFlags)
	}

	cFlags = libBar.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libBar' to not enable orderfile, but did find %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check no dependency edge from orderfile-enabled module to non-orderfile variant static libraries
	if hasDirectDep(result, libTest.Module(), libFoo.Module()) {
		t.Errorf("libTest has dependency on non-orderfile variant of libFoo")
	}

	if !hasDirectDep(result, libFoo.Module(), libBar.Module()) {
		t.Errorf("libTest has dependency on non-orderfile variant of libBar")
	}
}

// Load flags should never propagate
func TestOrderfileLoadPropagateStaticDeps(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "libTest",
		srcs: ["test.c"],
		static_libs: ["libFoo"],
		orderfile : {
			instrumentation: true,
			load_order_file: true,
			order_file_path: "test.orderfile",
		},
	}

	cc_library_static {
		name: "libFoo",
		srcs: ["foo.c"],
		static_libs: ["libBar"],
	}

	cc_library_static {
		name: "libBar",
		srcs: ["bar.c"],
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		android.FixtureAddTextFile("toolchain/pgo-profiles/orderfiles/test.orderfile", "TEST"),
	).RunTestWithBp(t, bp)

	expectedCFlag := "-Wl,--symbol-ordering-file=toolchain/pgo-profiles/orderfiles/test.orderfile"

	// Check ldFlags of orderfile-enabled module
	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared")

	ldFlags := libTest.Rule("ld").Args["ldFlags"]
	if !strings.Contains(ldFlags, expectedCFlag) {
		t.Errorf("Expected 'libTest' to load orderfile, but did not find %q in ldFlags %q", expectedCFlag, ldFlags)
	}

	libFoo := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static")
	libBar := result.ModuleForTests("libBar", "android_arm64_armv8-a_static")

	// Check dependency edge from orderfile-enabled module to non-orderfile variant static libraries
	if !hasDirectDep(result, libTest.Module(), libFoo.Module()) {
		t.Errorf("libTest missing dependency on non-orderfile variant of libFoo")
	}

	if !hasDirectDep(result, libFoo.Module(), libBar.Module()) {
		t.Errorf("libTest missing dependency on non-orderfile variant of libBar")
	}

	// Make sure no orderfile variants are created for static libraries because the flags were not propagated
	libFooVariants := result.ModuleVariantsForTests("libFoo")
	for _, v := range libFooVariants {
		if strings.Contains(v, "orderfile") {
			t.Errorf("Expected variants for 'libFoo' to not contain 'orderfile', but found %q", v)
		}
	}

	libBarVariants := result.ModuleVariantsForTests("libBar")
	for _, v := range libBarVariants {
		if strings.Contains(v, "orderfile") {
			t.Errorf("Expected variants for 'libBar' to not contain 'orderfile', but found %q", v)
		}
	}
}

// Profile flags should not propagate through shared libraries
func TestOrderfileProfilePropagateSharedDeps(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "libTest",
		srcs: ["test.c"],
		shared_libs: ["libFoo"],
		orderfile : {
			instrumentation: true,
			load_order_file: false,
			order_file_path: "",
		},
	}

	cc_library_shared {
		name: "libFoo",
		srcs: ["foo.c"],
		static_libs: ["libBar"],
	}

	cc_library_static {
		name: "libBar",
		srcs: ["bar.c"],
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	expectedCFlag := "-forder-file-instrumentation"

	// Check cFlags of orderfile-enabled module
	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared")

	cFlags := libTest.Rule("cc").Args["cFlags"]
	if !strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libTest' to enable orderfile, but did not find %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check cFlags of the static and shared libraries
	libFoo := result.ModuleForTests("libFoo", "android_arm64_armv8-a_shared")
	libBar := result.ModuleForTests("libBar", "android_arm64_armv8-a_static")

	cFlags = libFoo.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libFoo' to not enable orderfile, but did find %q in cflags %q", expectedCFlag, cFlags)
	}

	cFlags = libBar.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libBar' to not enable orderfile, but did find %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check dependency edge from orderfile-enabled module to non-orderfile variant static libraries
	if !hasDirectDep(result, libTest.Module(), libFoo.Module()) {
		t.Errorf("libTest missing dependency on non-orderfile variant of libFoo")
	}

	if !hasDirectDep(result, libFoo.Module(), libBar.Module()) {
		t.Errorf("libTest missing dependency on non-orderfile variant of libBar")
	}

	// Make sure no orderfile variants are created for libraries because the flags were not propagated
	libFooVariants := result.ModuleVariantsForTests("libFoo")
	for _, v := range libFooVariants {
		if strings.Contains(v, "orderfile") {
			t.Errorf("Expected variants for 'libFoo' to not contain 'orderfile', but found %q", v)
		}
	}

	libBarVariants := result.ModuleVariantsForTests("libBar")
	for _, v := range libBarVariants {
		if strings.Contains(v, "orderfile") {
			t.Errorf("Expected variants for 'libBar' to not contain 'orderfile', but found %q", v)
		}
	}
}

// Profile flags should not work or be propagated if orderfile flags start at a static library
func TestOrderfileProfileStaticLibrary(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_static {
		name: "libTest",
		srcs: ["test.c"],
		static_libs: ["libFoo"],
		orderfile : {
			instrumentation: true,
			load_order_file: false,
			order_file_path: "",
		},
	}

	cc_library_static {
		name: "libFoo",
		srcs: ["foo.c"],
		static_libs: ["libBar"],
	}

	cc_library_static {
		name: "libBar",
		srcs: ["bar.c"],
	}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	expectedCFlag := "-forder-file-instrumentation"

	// Check cFlags of module
	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_static")

	cFlags := libTest.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libTest' to not enable orderfile, but did find %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check cFlags of the static libraries
	libFoo := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static")
	libBar := result.ModuleForTests("libBar", "android_arm64_armv8-a_static")

	cFlags = libFoo.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libFoo' to not enable orderfile, but did find %q in cflags %q", expectedCFlag, cFlags)
	}

	cFlags = libBar.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags, expectedCFlag) {
		t.Errorf("Expected 'libBar' to not enable orderfile, but did find %q in cflags %q", expectedCFlag, cFlags)
	}

	// Check dependency edge from orderfile-enabled module to non-orderfile variant libraries
	if !hasDirectDep(result, libTest.Module(), libFoo.Module()) {
		t.Errorf("libTest missing dependency on non-orderfile variant of libFoo")
	}

	if !hasDirectDep(result, libFoo.Module(), libBar.Module()) {
		t.Errorf("libTest missing dependency on non-orderfile variant of libBar")
	}

	// Make sure no orderfile variants are created for static libraries because the flags were not propagated
	libFooVariants := result.ModuleVariantsForTests("libFoo")
	for _, v := range libFooVariants {
		if strings.Contains(v, "orderfile") {
			t.Errorf("Expected variants for 'libFoo' to not contain 'orderfile', but found %q", v)
		}
	}

	libBarVariants := result.ModuleVariantsForTests("libBar")
	for _, v := range libBarVariants {
		if strings.Contains(v, "orderfile") {
			t.Errorf("Expected variants for 'libBar' to not contain 'orderfile', but found %q", v)
		}
	}
}
