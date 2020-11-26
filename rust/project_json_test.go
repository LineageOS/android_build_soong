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
	tctx := newTestRustCtx(t, bp)
	tctx.env = map[string]string{"SOONG_GEN_RUST_PROJECT": "1"}
	tctx.generateConfig()
	tctx.parse(t)

	// The JSON file is generated via WriteFileToOutputDir. Therefore, it
	// won't appear in the Output of the TestingSingleton. Manually verify
	// it exists.
	content, err := ioutil.ReadFile(filepath.Join(buildDir, rustProjectJsonFileName))
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

func TestProjectJsonBindGen(t *testing.T) {
	bp := `
	rust_library {
		name: "liba",
		srcs: ["src/lib.rs"],
		rlibs: ["libbindings1"],
		crate_name: "a"
	}
	rust_bindgen {
		name: "libbindings1",
		crate_name: "bindings1",
		source_stem: "bindings1",
		host_supported: true,
		wrapper_src: "src/any.h",
	}
	rust_library_host {
		name: "libb",
		srcs: ["src/lib.rs"],
		rustlibs: ["libbindings2"],
		crate_name: "b"
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
		crate, ok := c.(map[string]interface{})
		if !ok {
			t.Fatalf("Unexpected type for crate: %v", c)
		}
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
			deps, ok := crate["deps"].([]interface{})
			if !ok {
				t.Errorf("Unexpected format for deps: %v", crate["deps"])
			}
			for _, dep := range deps {
				d, ok := dep.(map[string]interface{})
				if !ok {
					t.Errorf("Unexpected format for dep: %v", dep)
				}
				if d["name"] == "bindings1" {
					t.Errorf("libbindings1 depends on itself")
				}
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
	for _, crate := range crates {
		c := crate.(map[string]interface{})
		if c["root_module"] == "b/src/lib.rs" {
			deps, ok := c["deps"].([]interface{})
			if !ok {
				t.Errorf("Unexpected format for deps: %v", c["deps"])
			}
			aCount := 0
			for _, dep := range deps {
				d, ok := dep.(map[string]interface{})
				if !ok {
					t.Errorf("Unexpected format for dep: %v", dep)
				}
				if d["name"] == "a" {
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
