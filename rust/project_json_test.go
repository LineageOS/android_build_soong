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
	"io/ioutil"
	"path/filepath"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

func TestProjectJson(t *testing.T) {
	bp := `rust_library {
		  name: "liba",
		  srcs: ["src/lib.rs"],
		  crate_name: "a"
		}` + GatherRequiredDepsForTest()
	env := map[string]string{"SOONG_GEN_RUST_PROJECT": "1"}
	fs := map[string][]byte{
		"foo.rs":     nil,
		"src/lib.rs": nil,
	}

	cc.GatherRequiredFilesForTest(fs)

	config := android.TestArchConfig(buildDir, env, bp, fs)
	ctx := CreateTestContext()
	ctx.Register(config)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	// The JSON file is generated via WriteFileToOutputDir. Therefore, it
	// won't appear in the Output of the TestingSingleton. Manually verify
	// it exists.
	_, err := ioutil.ReadFile(filepath.Join(buildDir, "rust-project.json"))
	if err != nil {
		t.Errorf("rust-project.json has not been generated")
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
	` + GatherRequiredDepsForTest()
	fs := map[string][]byte{
		"a1/src/lib.rs": nil,
		"a2/src/lib.rs": nil,
		"b/src/lib.rs":  nil,
	}
	jsonContent := testProjectJson(t, bp, fs)
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
