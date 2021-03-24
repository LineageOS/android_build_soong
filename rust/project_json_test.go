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

package rust

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"android/soong/android"
)

// testProjectJson run the generation of rust-project.json. It returns the raw
// content of the generated file.
func testProjectJson(t *testing.T, bp string) []byte {
	result := prepareForRustTest.
		Extend(android.FixtureMergeEnv(map[string]string{"SOONG_GEN_RUST_PROJECT": "1"})).
		RunTestWithBp(t, bp)

	// The JSON file is generated via WriteFileToOutputDir. Therefore, it
	// won't appear in the Output of the TestingSingleton. Manually verify
	// it exists.
	content, err := ioutil.ReadFile(filepath.Join(result.Config.BuildDir(), rustProjectJsonFileName))
	if err != nil {
		t.Errorf("rust-project.json has not been generated")
	}
	return content
}

// validateJsonCrates validates that content follows the basic structure of
// rust-project.json. It returns the crates attribute if the validation
// succeeded.
// It uses an empty interface instead of relying on a defined structure to
// avoid a strong dependency on our implementation.
func validateJsonCrates(t *testing.T, rawContent []byte) []interface{} {
	var content interface{}
	err := json.Unmarshal(rawContent, &content)
	if err != nil {
		t.Errorf("Unable to parse the rust-project.json as JSON: %v", err)
	}
	root, ok := content.(map[string]interface{})
	if !ok {
		t.Errorf("Unexpected JSON format: %v", content)
	}
	if _, ok = root["crates"]; !ok {
		t.Errorf("No crates attribute in rust-project.json: %v", root)
	}
	crates, ok := root["crates"].([]interface{})
	if !ok {
		t.Errorf("Unexpected crates format: %v", root["crates"])
	}
	return crates
}

// validateCrate ensures that a crate can be parsed as a map.
func validateCrate(t *testing.T, crate interface{}) map[string]interface{} {
	c, ok := crate.(map[string]interface{})
	if !ok {
		t.Fatalf("Unexpected type for crate: %v", c)
	}
	return c
}

// validateDependencies parses the dependencies for a crate. It returns a list
// of the dependencies name.
func validateDependencies(t *testing.T, crate map[string]interface{}) []string {
	var dependencies []string
	deps, ok := crate["deps"].([]interface{})
	if !ok {
		t.Errorf("Unexpected format for deps: %v", crate["deps"])
	}
	for _, dep := range deps {
		d, ok := dep.(map[string]interface{})
		if !ok {
			t.Errorf("Unexpected format for dependency: %v", dep)
		}
		name, ok := d["name"].(string)
		if !ok {
			t.Errorf("Dependency is missing the name key: %v", d)
		}
		dependencies = append(dependencies, name)
	}
	return dependencies
}

func TestProjectJsonDep(t *testing.T) {
	bp := `
	rust_library {
		name: "liba",
		srcs: ["a/src/lib.rs"],
		crate_name: "a"
	}
	rust_library {
		name: "libb",
		srcs: ["b/src/lib.rs"],
		crate_name: "b",
		rlibs: ["liba"],
	}
	`
	jsonContent := testProjectJson(t, bp)
	validateJsonCrates(t, jsonContent)
}

func TestProjectJsonBinary(t *testing.T) {
	bp := `
	rust_binary {
		name: "libz",
		srcs: ["z/src/lib.rs"],
		crate_name: "z"
	}
	`
	jsonContent := testProjectJson(t, bp)
	crates := validateJsonCrates(t, jsonContent)
	for _, c := range crates {
		crate := validateCrate(t, c)
		rootModule, ok := crate["root_module"].(string)
		if !ok {
			t.Fatalf("Unexpected type for root_module: %v", crate["root_module"])
		}
		if rootModule == "z/src/lib.rs" {
			return
		}
	}
	t.Errorf("Entry for binary %q not found: %s", "a", jsonContent)
}

func TestProjectJsonBindGen(t *testing.T) {
	bp := `
	rust_library {
		name: "libd",
		srcs: ["d/src/lib.rs"],
		rlibs: ["libbindings1"],
		crate_name: "d"
	}
	rust_bindgen {
		name: "libbindings1",
		crate_name: "bindings1",
		source_stem: "bindings1",
		host_supported: true,
		wrapper_src: "src/any.h",
	}
	rust_library_host {
		name: "libe",
		srcs: ["e/src/lib.rs"],
		rustlibs: ["libbindings2"],
		crate_name: "e"
	}
	rust_bindgen_host {
		name: "libbindings2",
		crate_name: "bindings2",
		source_stem: "bindings2",
		wrapper_src: "src/any.h",
	}
	`
	jsonContent := testProjectJson(t, bp)
	crates := validateJsonCrates(t, jsonContent)
	for _, c := range crates {
		crate := validateCrate(t, c)
		rootModule, ok := crate["root_module"].(string)
		if !ok {
			t.Fatalf("Unexpected type for root_module: %v", crate["root_module"])
		}
		if strings.Contains(rootModule, "libbindings1") && !strings.Contains(rootModule, "android_arm64") {
			t.Errorf("The source path for libbindings1 does not contain android_arm64, got %v", rootModule)
		}
		if strings.Contains(rootModule, "libbindings2") && !strings.Contains(rootModule, android.BuildOs.String()) {
			t.Errorf("The source path for libbindings2 does not contain the BuildOs, got %v; want %v",
				rootModule, android.BuildOs.String())
		}
		// Check that libbindings1 does not depend on itself.
		if strings.Contains(rootModule, "libbindings1") {
			for _, depName := range validateDependencies(t, crate) {
				if depName == "bindings1" {
					t.Errorf("libbindings1 depends on itself")
				}
			}
		}
		if strings.Contains(rootModule, "d/src/lib.rs") {
			// Check that libd depends on libbindings1
			found := false
			for _, depName := range validateDependencies(t, crate) {
				if depName == "bindings1" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("libd does not depend on libbindings1: %v", crate)
			}
			// Check that OUT_DIR is populated.
			env, ok := crate["env"].(map[string]interface{})
			if !ok {
				t.Errorf("libd does not have its environment variables set: %v", crate)
			}
			if _, ok = env["OUT_DIR"]; !ok {
				t.Errorf("libd does not have its OUT_DIR set: %v", env)
			}

		}
	}
}

func TestProjectJsonMultiVersion(t *testing.T) {
	bp := `
	rust_library {
		name: "liba1",
		srcs: ["a1/src/lib.rs"],
		crate_name: "a"
	}
	rust_library {
		name: "liba2",
		srcs: ["a2/src/lib.rs"],
		crate_name: "a",
	}
	rust_library {
		name: "libb",
		srcs: ["b/src/lib.rs"],
		crate_name: "b",
		rustlibs: ["liba1", "liba2"],
	}
	`
	jsonContent := testProjectJson(t, bp)
	crates := validateJsonCrates(t, jsonContent)
	for _, c := range crates {
		crate := validateCrate(t, c)
		rootModule, ok := crate["root_module"].(string)
		if !ok {
			t.Fatalf("Unexpected type for root_module: %v", crate["root_module"])
		}
		// Make sure that b has 2 different dependencies.
		if rootModule == "b/src/lib.rs" {
			aCount := 0
			deps := validateDependencies(t, crate)
			for _, depName := range deps {
				if depName == "a" {
					aCount++
				}
			}
			if aCount != 2 {
				t.Errorf("Unexpected number of liba dependencies want %v, got %v: %v", 2, aCount, deps)
			}
			return
		}
	}
	t.Errorf("libb crate has not been found: %v", crates)
}
