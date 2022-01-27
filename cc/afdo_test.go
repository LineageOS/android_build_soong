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

	"android/soong/android"
	"github.com/google/blueprint"
)

func TestAfdoDeps(t *testing.T) {
	bp := `
	cc_library {
		name: "libTest",
		srcs: ["foo.c"],
		static_libs: ["libFoo"],
		afdo: true,
	}

	cc_library {
		name: "libFoo",
		static_libs: ["libBar"],
	}

	cc_library {
		name: "libBar",
	}
	`
	prepareForAfdoTest := android.FixtureAddTextFile("toolchain/pgo-profiles/sampling/libTest.afdo", "TEST")

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		prepareForAfdoTest,
	).RunTestWithBp(t, bp)

	libTest := result.ModuleForTests("libTest", "android_arm64_armv8-a_shared").Module()
	libFoo := result.ModuleForTests("libFoo", "android_arm64_armv8-a_static_afdo-libTest").Module()
	libBar := result.ModuleForTests("libBar", "android_arm64_armv8-a_static_afdo-libTest").Module()

	hasDep := func(m android.Module, wantDep android.Module) bool {
		var found bool
		result.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	if !hasDep(libTest, libFoo) {
		t.Errorf("libTest missing dependency on afdo variant of libFoo")
	}

	if !hasDep(libFoo, libBar) {
		t.Errorf("libTest missing dependency on afdo variant of libBar")
	}
}
