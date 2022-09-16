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
	bp := `
	cc_library {
		name: "lto_enabled",
		srcs: ["src.c"],
		static_libs: ["foo"],
		shared_libs: ["bar"],
		lto: {
			thin: true,
		}
	}
	cc_library {
		name: "foo",
		static_libs: ["baz"],
	}
	cc_library {
		name: "bar",
		static_libs: ["qux"],
	}
	cc_library {
		name: "baz",
	}
	cc_library {
		name: "qux",
	}
`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
	).RunTestWithBp(t, bp)

	libLto := result.ModuleForTests("lto_enabled", "android_arm64_armv8-a_shared").Module()
	libFoo := result.ModuleForTests("foo", "android_arm64_armv8-a_static_lto-thin").Module()
	libBaz := result.ModuleForTests("baz", "android_arm64_armv8-a_static_lto-thin").Module()

	hasDep := func(m android.Module, wantDep android.Module) bool {
		var found bool
		result.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	if !hasDep(libLto, libFoo) {
		t.Errorf("'lto_enabled' missing dependency on thin lto variant of 'foo'")
	}

	if !hasDep(libFoo, libBaz) {
		t.Errorf("'lto_enabled' missing dependency on thin lto variant of transitive dep 'baz'")
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
