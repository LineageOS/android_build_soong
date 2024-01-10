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
	"slices"
	"testing"

	"android/soong/android"
)

func testGenruleContext(config android.Config) *android.TestContext {
	ctx := android.NewTestArchContext(config)
	ctx.RegisterModuleType("cc_genrule", GenRuleFactory)
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
					out: ["out_arm"],
					arch: {
						arm: {
							srcs: ["foo"],
						},
						arm64: {
							srcs: ["bar"],
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

	gen = ctx.ModuleForTests("gen", "android_arm64_armv8-a").Output("out_arm")
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

func TestCmdPrefix(t *testing.T) {
	bp := `
		cc_genrule {
			name: "gen",
			cmd: "echo foo",
			out: ["out"],
			native_bridge_supported: true,
		}
		`

	testCases := []struct {
		name     string
		variant  string
		preparer android.FixturePreparer

		arch         string
		nativeBridge string
		multilib     string
	}{
		{
			name:     "arm",
			variant:  "android_arm_armv7-a-neon",
			arch:     "arm",
			multilib: "lib32",
		},
		{
			name:     "arm64",
			variant:  "android_arm64_armv8-a",
			arch:     "arm64",
			multilib: "lib64",
		},
		{
			name:    "nativebridge",
			variant: "android_native_bridge_arm_armv7-a-neon",
			preparer: android.FixtureModifyConfig(func(config android.Config) {
				config.Targets[android.Android] = []android.Target{
					{
						Os:           android.Android,
						Arch:         android.Arch{ArchType: android.X86, ArchVariant: "silvermont", Abi: []string{"armeabi-v7a"}},
						NativeBridge: android.NativeBridgeDisabled,
					},
					{
						Os:                       android.Android,
						Arch:                     android.Arch{ArchType: android.Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}},
						NativeBridge:             android.NativeBridgeEnabled,
						NativeBridgeHostArchName: "x86",
						NativeBridgeRelativePath: "arm",
					},
				}
			}),
			arch:         "arm",
			multilib:     "lib32",
			nativeBridge: "arm",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := android.GroupFixturePreparers(
				PrepareForIntegrationTestWithCc,
				android.OptionalFixturePreparer(tt.preparer),
			).RunTestWithBp(t, bp)
			gen := result.ModuleForTests("gen", tt.variant)
			sboxProto := android.RuleBuilderSboxProtoForTests(t, result.TestContext, gen.Output("genrule.sbox.textproto"))
			cmd := *sboxProto.Commands[0].Command
			android.AssertStringDoesContain(t, "incorrect CC_ARCH", cmd, "CC_ARCH="+tt.arch+" ")
			android.AssertStringDoesContain(t, "incorrect CC_NATIVE_BRIDGE", cmd, "CC_NATIVE_BRIDGE="+tt.nativeBridge+" ")
			android.AssertStringDoesContain(t, "incorrect CC_MULTILIB", cmd, "CC_MULTILIB="+tt.multilib+" ")
		})
	}
}

func TestVendorProductVariantGenrule(t *testing.T) {
	bp := `
	cc_genrule {
		name: "gen",
		tool_files: ["tool"],
		cmd: "$(location tool) $(in) $(out)",
		out: ["out"],
		vendor_available: true,
		product_available: true,
	}
	`
	t.Helper()
	ctx := PrepareForTestWithCcIncludeVndk.RunTestWithBp(t, bp)

	variants := ctx.ModuleVariantsForTests("gen")
	if !slices.Contains(variants, "android_vendor_arm64_armv8-a") {
		t.Errorf(`expected vendor variant, but does not exist in %v`, variants)
	}
	if !slices.Contains(variants, "android_product_arm64_armv8-a") {
		t.Errorf(`expected product variant, but does not exist in %v`, variants)
	}
}
