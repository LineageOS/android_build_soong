// Copyright 2019 Google Inc. All rights reserved.
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

package android

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/google/blueprint/proptools"
)

type Named struct {
	A *string `android:"arch_variant"`
	B *string
}

type NamedAllFiltered struct {
	A *string
}

type NamedNoneFiltered struct {
	A *string `android:"arch_variant"`
}

func TestFilterArchStruct(t *testing.T) {
	tests := []struct {
		name     string
		in       interface{}
		out      interface{}
		filtered bool
	}{
		// Property tests
		{
			name: "basic",
			in: &struct {
				A *string `android:"arch_variant"`
				B *string
			}{},
			out: &struct {
				A *string
			}{},
			filtered: true,
		},
		{
			name: "tags",
			in: &struct {
				A *string `android:"arch_variant"`
				B *string `android:"arch_variant,path"`
				C *string `android:"arch_variant,path,variant_prepend"`
				D *string `android:"path,variant_prepend,arch_variant"`
				E *string `android:"path"`
				F *string
			}{},
			out: &struct {
				A *string
				B *string
				C *string
				D *string
			}{},
			filtered: true,
		},
		{
			name: "all filtered",
			in: &struct {
				A *string
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "none filtered",
			in: &struct {
				A *string `android:"arch_variant"`
			}{},
			out: &struct {
				A *string `android:"arch_variant"`
			}{},
			filtered: false,
		},

		// Sub-struct tests
		{
			name: "substruct",
			in: &struct {
				A struct {
					A *string `android:"arch_variant"`
					B *string
				} `android:"arch_variant"`
			}{},
			out: &struct {
				A struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "substruct all filtered",
			in: &struct {
				A struct {
					A *string
				} `android:"arch_variant"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "substruct none filtered",
			in: &struct {
				A struct {
					A *string `android:"arch_variant"`
				} `android:"arch_variant"`
			}{},
			out: &struct {
				A struct {
					A *string `android:"arch_variant"`
				} `android:"arch_variant"`
			}{},
			filtered: false,
		},

		// Named sub-struct tests
		{
			name: "named substruct",
			in: &struct {
				A Named `android:"arch_variant"`
			}{},
			out: &struct {
				A struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "substruct all filtered",
			in: &struct {
				A NamedAllFiltered `android:"arch_variant"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "substruct none filtered",
			in: &struct {
				A NamedNoneFiltered `android:"arch_variant"`
			}{},
			out: &struct {
				A NamedNoneFiltered `android:"arch_variant"`
			}{},
			filtered: false,
		},

		// Pointer to sub-struct tests
		{
			name: "pointer substruct",
			in: &struct {
				A *struct {
					A *string `android:"arch_variant"`
					B *string
				} `android:"arch_variant"`
			}{},
			out: &struct {
				A *struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "pointer substruct all filtered",
			in: &struct {
				A *struct {
					A *string
				} `android:"arch_variant"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "pointer substruct none filtered",
			in: &struct {
				A *struct {
					A *string `android:"arch_variant"`
				} `android:"arch_variant"`
			}{},
			out: &struct {
				A *struct {
					A *string `android:"arch_variant"`
				} `android:"arch_variant"`
			}{},
			filtered: false,
		},

		// Pointer to named sub-struct tests
		{
			name: "pointer named substruct",
			in: &struct {
				A *Named `android:"arch_variant"`
			}{},
			out: &struct {
				A *struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "pointer substruct all filtered",
			in: &struct {
				A *NamedAllFiltered `android:"arch_variant"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "pointer substruct none filtered",
			in: &struct {
				A *NamedNoneFiltered `android:"arch_variant"`
			}{},
			out: &struct {
				A *NamedNoneFiltered `android:"arch_variant"`
			}{},
			filtered: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, filtered := proptools.FilterPropertyStruct(reflect.TypeOf(test.in), filterArchStruct)
			if filtered != test.filtered {
				t.Errorf("expected filtered %v, got %v", test.filtered, filtered)
			}
			expected := reflect.TypeOf(test.out)
			if out != expected {
				t.Errorf("expected type %v, got %v", expected, out)
			}
		})
	}
}

type archTestModule struct {
	ModuleBase
	props struct {
		Deps []string
	}
}

func (m *archTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
}

func (m *archTestModule) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddDependency(ctx.Module(), nil, m.props.Deps...)
}

func archTestModuleFactory() Module {
	m := &archTestModule{}
	m.AddProperties(&m.props)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibBoth)
	return m
}

var prepareForArchTest = GroupFixturePreparers(
	PrepareForTestWithArchMutator,
	FixtureRegisterWithContext(func(ctx RegistrationContext) {
		ctx.RegisterModuleType("module", archTestModuleFactory)
	}),
)

func TestArchMutator(t *testing.T) {
	var buildOSVariants []string
	var buildOS32Variants []string
	switch runtime.GOOS {
	case "linux":
		buildOSVariants = []string{"linux_glibc_x86_64", "linux_glibc_x86"}
		buildOS32Variants = []string{"linux_glibc_x86"}
	case "darwin":
		buildOSVariants = []string{"darwin_x86_64"}
		buildOS32Variants = nil
	}

	bp := `
		module {
			name: "foo",
		}

		module {
			name: "bar",
			host_supported: true,
		}

		module {
			name: "baz",
			device_supported: false,
		}

		module {
			name: "qux",
			host_supported: true,
			compile_multilib: "32",
		}
	`

	testCases := []struct {
		name        string
		preparer    FixturePreparer
		fooVariants []string
		barVariants []string
		bazVariants []string
		quxVariants []string
	}{
		{
			name:        "normal",
			preparer:    nil,
			fooVariants: []string{"android_arm64_armv8-a", "android_arm_armv7-a-neon"},
			barVariants: append(buildOSVariants, "android_arm64_armv8-a", "android_arm_armv7-a-neon"),
			bazVariants: nil,
			quxVariants: append(buildOS32Variants, "android_arm_armv7-a-neon"),
		},
		{
			name: "host-only",
			preparer: FixtureModifyConfig(func(config Config) {
				config.BuildOSTarget = Target{}
				config.BuildOSCommonTarget = Target{}
				config.Targets[Android] = nil
			}),
			fooVariants: nil,
			barVariants: buildOSVariants,
			bazVariants: nil,
			quxVariants: buildOS32Variants,
		},
	}

	enabledVariants := func(ctx *TestContext, name string) []string {
		var ret []string
		variants := ctx.ModuleVariantsForTests(name)
		for _, variant := range variants {
			m := ctx.ModuleForTests(name, variant)
			if m.Module().Enabled() {
				ret = append(ret, variant)
			}
		}
		return ret
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := emptyTestFixtureFactory.RunTest(t,
				prepareForArchTest,
				// Test specific preparer
				OptionalFixturePreparer(tt.preparer),
				FixtureWithRootAndroidBp(bp),
			)
			ctx := result.TestContext

			if g, w := enabledVariants(ctx, "foo"), tt.fooVariants; !reflect.DeepEqual(w, g) {
				t.Errorf("want foo variants:\n%q\ngot:\n%q\n", w, g)
			}

			if g, w := enabledVariants(ctx, "bar"), tt.barVariants; !reflect.DeepEqual(w, g) {
				t.Errorf("want bar variants:\n%q\ngot:\n%q\n", w, g)
			}

			if g, w := enabledVariants(ctx, "baz"), tt.bazVariants; !reflect.DeepEqual(w, g) {
				t.Errorf("want baz variants:\n%q\ngot:\n%q\n", w, g)
			}

			if g, w := enabledVariants(ctx, "qux"), tt.quxVariants; !reflect.DeepEqual(w, g) {
				t.Errorf("want qux variants:\n%q\ngot:\n%q\n", w, g)
			}
		})
	}
}

func TestArchMutatorNativeBridge(t *testing.T) {
	bp := `
		// This module is only enabled for x86.
		module {
			name: "foo",
		}

		// This module is enabled for x86 and arm (via native bridge).
		module {
			name: "bar",
			native_bridge_supported: true,
		}

		// This module is enabled for arm (native_bridge) only.
		module {
			name: "baz",
			native_bridge_supported: true,
			enabled: false,
			target: {
				native_bridge: {
					enabled: true,
				}
			}
		}
	`

	testCases := []struct {
		name        string
		preparer    FixturePreparer
		fooVariants []string
		barVariants []string
		bazVariants []string
	}{
		{
			name:        "normal",
			preparer:    nil,
			fooVariants: []string{"android_x86_64_silvermont", "android_x86_silvermont"},
			barVariants: []string{"android_x86_64_silvermont", "android_native_bridge_arm64_armv8-a", "android_x86_silvermont", "android_native_bridge_arm_armv7-a-neon"},
			bazVariants: []string{"android_native_bridge_arm64_armv8-a", "android_native_bridge_arm_armv7-a-neon"},
		},
	}

	enabledVariants := func(ctx *TestContext, name string) []string {
		var ret []string
		variants := ctx.ModuleVariantsForTests(name)
		for _, variant := range variants {
			m := ctx.ModuleForTests(name, variant)
			if m.Module().Enabled() {
				ret = append(ret, variant)
			}
		}
		return ret
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := emptyTestFixtureFactory.RunTest(t,
				prepareForArchTest,
				// Test specific preparer
				OptionalFixturePreparer(tt.preparer),
				// Prepare for native bridge test
				FixtureModifyConfig(func(config Config) {
					config.Targets[Android] = []Target{
						{Android, Arch{ArchType: X86_64, ArchVariant: "silvermont", Abi: []string{"arm64-v8a"}}, NativeBridgeDisabled, "", "", false},
						{Android, Arch{ArchType: X86, ArchVariant: "silvermont", Abi: []string{"armeabi-v7a"}}, NativeBridgeDisabled, "", "", false},
						{Android, Arch{ArchType: Arm64, ArchVariant: "armv8-a", Abi: []string{"arm64-v8a"}}, NativeBridgeEnabled, "x86_64", "arm64", false},
						{Android, Arch{ArchType: Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}}, NativeBridgeEnabled, "x86", "arm", false},
					}
				}),
				FixtureWithRootAndroidBp(bp),
			)

			ctx := result.TestContext

			if g, w := enabledVariants(ctx, "foo"), tt.fooVariants; !reflect.DeepEqual(w, g) {
				t.Errorf("want foo variants:\n%q\ngot:\n%q\n", w, g)
			}

			if g, w := enabledVariants(ctx, "bar"), tt.barVariants; !reflect.DeepEqual(w, g) {
				t.Errorf("want bar variants:\n%q\ngot:\n%q\n", w, g)
			}

			if g, w := enabledVariants(ctx, "baz"), tt.bazVariants; !reflect.DeepEqual(w, g) {
				t.Errorf("want qux variants:\n%q\ngot:\n%q\n", w, g)
			}
		})
	}
}
