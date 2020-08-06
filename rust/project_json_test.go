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
