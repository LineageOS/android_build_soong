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
	"android/soong/multitree"
)

func TestCcApiStubLibraryOutputFiles(t *testing.T) {
	bp := `
		cc_api_stub_library {
			name: "foo",
			symbol_file: "foo.map.txt",
			first_version: "29",
		}
	`
	result := prepareForCcTest.RunTestWithBp(t, bp)
	outputs := result.ModuleForTests("foo", "android_arm64_armv8-a_shared").AllOutputs()
	expected_file_suffixes := []string{".c", "stub.map", ".o", ".so"}
	for _, expected_file_suffix := range expected_file_suffixes {
		android.AssertBoolEquals(t, expected_file_suffix+" file not found in output", true, android.SuffixInList(outputs, expected_file_suffix))
	}
}

func TestCcApiStubLibraryVariants(t *testing.T) {
	bp := `
		cc_api_stub_library {
			name: "foo",
			symbol_file: "foo.map.txt",
			first_version: "29",
		}
	`
	result := prepareForCcTest.RunTestWithBp(t, bp)
	variants := result.ModuleVariantsForTests("foo")
	expected_variants := []string{"29", "30", "S", "Tiramisu"} //TODO: make this test deterministic by using fixtures
	for _, expected_variant := range expected_variants {
		android.AssertBoolEquals(t, expected_variant+" variant not found in foo", true, android.SubstringInList(variants, expected_variant))
	}
}

func TestCcLibraryUsesCcApiStubLibrary(t *testing.T) {
	bp := `
		cc_api_stub_library {
			name: "foo",
			symbol_file: "foo.map.txt",
			first_version: "29",
		}
		cc_library {
			name: "foo_user",
			shared_libs: [
				"foo#29",
			],
		}

	`
	prepareForCcTest.RunTestWithBp(t, bp)
}

func TestApiSurfaceOutputs(t *testing.T) {
	bp := `
		api_surface {
			name: "mysdk",
			contributions: [
				"foo",
			],
		}

		cc_api_contribution {
			name: "foo",
			symbol_file: "foo.map.txt",
			first_version: "29",
		}
	`
	result := android.GroupFixturePreparers(
		prepareForCcTest,
		multitree.PrepareForTestWithApiSurface,
	).RunTestWithBp(t, bp)
	mysdk := result.ModuleForTests("mysdk", "")

	actual_surface_inputs := mysdk.Rule("phony").BuildParams.Inputs.Strings()
	expected_file_suffixes := []string{"mysdk/foo/foo.map.txt"}
	for _, expected_file_suffix := range expected_file_suffixes {
		android.AssertBoolEquals(t, expected_file_suffix+" file not found in input", true, android.SuffixInList(actual_surface_inputs, expected_file_suffix))
	}

	// check args/inputs to rule
	/*api_surface_gen_rule_args := result.ModuleForTests("mysdk", "").Rule("genApiSurfaceBuildFiles").Args
	android.AssertStringEquals(t, "name", "foo.mysdk", api_surface_gen_rule_args["name"])
	android.AssertStringEquals(t, "symbol_file", "foo.map.txt", api_surface_gen_rule_args["symbol_file"])*/
}
