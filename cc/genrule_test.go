// Copyright 2018 Google Inc. All rights reserved.
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
	"reflect"
	"testing"

	"android/soong/android"
)

func testGenruleContext(config android.Config) *android.TestContext {
	ctx := android.NewTestArchContext(config)
	ctx.RegisterModuleType("cc_genrule", genRuleFactory)
	ctx.Register()

	return ctx
}

func TestArchGenruleCmd(t *testing.T) {
	fs := map[string][]byte{
		"tool": nil,
		"foo":  nil,
		"bar":  nil,
	}
	bp := `
				cc_genrule {
					name: "gen",
					tool_files: ["tool"],
					cmd: "$(location tool) $(in) $(out)",
					arch: {
						arm: {
							srcs: ["foo"],
							out: ["out_arm"],
						},
						arm64: {
							srcs: ["bar"],
							out: ["out_arm64"],
						},
					},
				}
			`
	config := android.TestArchConfig(t.TempDir(), nil, bp, fs)

	ctx := testGenruleContext(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if errs == nil {
		_, errs = ctx.PrepareBuildActions(config)
	}
	if errs != nil {
		t.Fatal(errs)
	}

	gen := ctx.ModuleForTests("gen", "android_arm_armv7-a-neon").Output("out_arm")
	expected := []string{"foo"}
	if !reflect.DeepEqual(expected, gen.Implicits.Strings()[:len(expected)]) {
		t.Errorf(`want arm inputs %v, got %v`, expected, gen.Implicits.Strings())
	}

	gen = ctx.ModuleForTests("gen", "android_arm64_armv8-a").Output("out_arm64")
	expected = []string{"bar"}
	if !reflect.DeepEqual(expected, gen.Implicits.Strings()[:len(expected)]) {
		t.Errorf(`want arm64 inputs %v, got %v`, expected, gen.Implicits.Strings())
	}
}

func TestLibraryGenruleCmd(t *testing.T) {
	bp := `
		cc_library {
			name: "libboth",
		}

		cc_library_shared {
			name: "libshared",
		}

		cc_library_static {
			name: "libstatic",
		}

		cc_genrule {
			name: "gen",
			tool_files: ["tool"],
			srcs: [
				":libboth",
				":libshared",
				":libstatic",
			],
			cmd: "$(location tool) $(in) $(out)",
			out: ["out"],
		}
		`
	ctx := testCc(t, bp)

	gen := ctx.ModuleForTests("gen", "android_arm_armv7-a-neon").Output("out")
	expected := []string{"libboth.so", "libshared.so", "libstatic.a"}
	var got []string
	for _, input := range gen.Implicits {
		got = append(got, input.Base())
	}
	if !reflect.DeepEqual(expected, got[:len(expected)]) {
		t.Errorf(`want inputs %v, got %v`, expected, got)
	}
}
