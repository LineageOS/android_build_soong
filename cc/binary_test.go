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
)

func TestCcBinaryWithBazel(t *testing.T) {
	bp := `
cc_binary {
	name: "foo",
	srcs: ["foo.cc"],
	bazel_module: { label: "//foo/bar:bar" },
}`
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.BazelContext = android.MockBazelContext{
		OutputBaseDir: "outputbase",
		LabelToOutputFiles: map[string][]string{
			"//foo/bar:bar": []string{"foo"},
		},
	}
	ctx := testCcWithConfig(t, config)

	binMod := ctx.ModuleForTests("foo", "android_arm64_armv8-a").Module()
	producer := binMod.(android.OutputFileProducer)
	outputFiles, err := producer.OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_binary outputfiles %s", err)
	}
	expectedOutputFiles := []string{"outputbase/execroot/__main__/foo"}
	android.AssertDeepEquals(t, "output files", expectedOutputFiles, outputFiles.Strings())

	unStrippedFilePath := binMod.(*Module).UnstrippedOutputFile()
	expectedUnStrippedFile := "outputbase/execroot/__main__/foo"
	android.AssertStringEquals(t, "Unstripped output file", expectedUnStrippedFile, unStrippedFilePath.String())
}

func TestBinaryLinkerScripts(t *testing.T) {
	result := PrepareForIntegrationTestWithCc.RunTestWithBp(t, `
		cc_binary {
			name: "foo",
			srcs: ["foo.cc"],
			linker_scripts: ["foo.ld", "bar.ld"],
		}`)

	binFoo := result.ModuleForTests("foo", "android_arm64_armv8-a").Rule("ld")

	android.AssertStringListContains(t, "missing dependency on linker_scripts",
		binFoo.Implicits.Strings(), "foo.ld")
	android.AssertStringListContains(t, "missing dependency on linker_scripts",
		binFoo.Implicits.Strings(), "bar.ld")
	android.AssertStringDoesContain(t, "missing flag for linker_scripts",
		libfoo.Args["ldFlags"], "-Wl,--script,foo.ld")
	android.AssertStringDoesContain(t, "missing flag for linker_scripts",
		libfoo.Args["ldFlags"], "-Wl,--script,bar.ld")
}
