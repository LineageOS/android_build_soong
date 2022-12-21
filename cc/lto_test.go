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
	"android/soong/android"
	"strings"
	"testing"

	"github.com/google/blueprint"
)

func TestThinLtoDeps(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "lto_enabled",
		srcs: ["src.c"],
		static_libs: ["foo", "lib_never_lto"],
		shared_libs: ["bar"],
		lto: {
			thin: true,
		}
	}
	cc_library_static {
		name: "foo",
		static_libs: ["baz"],
	}
	cc_library_shared {
		name: "bar",
		static_libs: ["qux"],
	}
	cc_library_static {
		name: "baz",
	}
	cc_library_static {
		name: "qux",
	}
	cc_library_static {
		name: "lib_never_lto",
		lto: {
			never: true,
		},
	}
`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	libLto := result.ModuleForTests("lto_enabled", "android_arm64_armv8-a_shared").Module()

	hasDep := func(m android.Module, wantDep android.Module) bool {
		var found bool
		result.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	libFoo := result.ModuleForTests("foo", "android_arm64_armv8-a_static_lto-thin").Module()
	if !hasDep(libLto, libFoo) {
		t.Errorf("'lto_enabled' missing dependency on thin lto variant of 'foo'")
	}

	libBaz := result.ModuleForTests("baz", "android_arm64_armv8-a_static_lto-thin").Module()
	if !hasDep(libFoo, libBaz) {
		t.Errorf("'foo' missing dependency on thin lto variant of transitive dep 'baz'")
	}

	libNeverLto := result.ModuleForTests("lib_never_lto", "android_arm64_armv8-a_static_lto-thin").Module()
	if !hasDep(libLto, libNeverLto) {
		t.Errorf("'lto_enabled' missing dependency on NO-thin lto variant of 'lib_never_lto'")
	}

	libBar := result.ModuleForTests("bar", "android_arm64_armv8-a_shared").Module()
	if !hasDep(libLto, libBar) {
		t.Errorf("'lto_enabled' missing dependency on non-thin lto variant of 'bar'")
	}

	barVariants := result.ModuleVariantsForTests("bar")
	for _, v := range barVariants {
		if strings.Contains(v, "lto-thin") {
			t.Errorf("Expected variants for 'bar' to not contain 'lto-thin', but found %q", v)
		}
	}
	quxVariants := result.ModuleVariantsForTests("qux")
	for _, v := range quxVariants {
		if strings.Contains(v, "lto-thin") {
			t.Errorf("Expected variants for 'qux' to not contain 'lto-thin', but found %q", v)
		}
	}
}

func TestThinLtoOnlyOnStaticDep(t *testing.T) {
	t.Parallel()
	bp := `
	cc_library_shared {
		name: "root",
		srcs: ["src.c"],
		static_libs: ["foo"],
	}
	cc_library_shared {
		name: "root_no_lto",
		srcs: ["src.c"],
		static_libs: ["foo"],
		lto: {
			never: true,
		}
	}
	cc_library_static {
		name: "foo",
		srcs: ["foo.c"],
		static_libs: ["baz"],
		lto: {
			thin: true,
		}
	}
	cc_library_static {
		name: "baz",
		srcs: ["baz.c"],
	}
`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	libRoot := result.ModuleForTests("root", "android_arm64_armv8-a_shared").Module()
	libRootLtoNever := result.ModuleForTests("root_no_lto", "android_arm64_armv8-a_shared").Module()

	hasDep := func(m android.Module, wantDep android.Module) bool {
		var found bool
		result.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	libFoo := result.ModuleForTests("foo", "android_arm64_armv8-a_static")
	if !hasDep(libRoot, libFoo.Module()) {
		t.Errorf("'root' missing dependency on thin lto variant of 'foo'")
	}

	if !hasDep(libRootLtoNever, libFoo.Module()) {
		t.Errorf("'root_no_lto' missing dependency on thin lto variant of 'foo'")
	}

	libFooCFlags := libFoo.Rule("cc").Args["cFlags"]
	if w := "-flto=thin -fsplit-lto-unit"; !strings.Contains(libFooCFlags, w) {
		t.Errorf("'foo' expected to have flags %q, but got %q", w, libFooCFlags)
	}

	libBaz := result.ModuleForTests("baz", "android_arm64_armv8-a_static_lto-thin")
	if !hasDep(libFoo.Module(), libBaz.Module()) {
		t.Errorf("'foo' missing dependency on thin lto variant of transitive dep 'baz'")
	}

	libBazCFlags := libFoo.Rule("cc").Args["cFlags"]
	if w := "-flto=thin -fsplit-lto-unit"; !strings.Contains(libBazCFlags, w) {
		t.Errorf("'baz' expected to have flags %q, but got %q", w, libFooCFlags)
	}
}
