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

package android

import (
	"strings"
	"testing"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// Module to be packaged
type componentTestModule struct {
	ModuleBase
	props struct {
		Deps         []string
		Skip_install *bool
	}
}

// dep tag used in this test. All dependencies are considered as installable.
type installDepTag struct {
	blueprint.BaseDependencyTag
	InstallAlwaysNeededDependencyTag
}

func componentTestModuleFactory() Module {
	m := &componentTestModule{}
	m.AddProperties(&m.props)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibBoth)
	return m
}

func (m *componentTestModule) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddDependency(ctx.Module(), installDepTag{}, m.props.Deps...)
}

func (m *componentTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	builtFile := PathForModuleOut(ctx, m.Name())
	dir := ctx.Target().Arch.ArchType.Multilib
	installDir := PathForModuleInstall(ctx, dir)
	if proptools.Bool(m.props.Skip_install) {
		m.SkipInstall()
	}
	ctx.InstallFile(installDir, m.Name(), builtFile)
}

// Module that itself is a package
type packageTestModule struct {
	ModuleBase
	PackagingBase
	properties struct {
		Install_deps []string `android:`
	}
	entries []string
}

func packageTestModuleFactory(multiTarget bool, depsCollectFirstTargetOnly bool) Module {
	module := &packageTestModule{}
	InitPackageModule(module)
	module.DepsCollectFirstTargetOnly = depsCollectFirstTargetOnly
	if multiTarget {
		InitAndroidMultiTargetsArchModule(module, DeviceSupported, MultilibCommon)
	} else {
		InitAndroidArchModule(module, DeviceSupported, MultilibBoth)
	}
	module.AddProperties(&module.properties)
	return module
}

type packagingDepTag struct {
	blueprint.BaseDependencyTag
	PackagingItemAlwaysDepTag
}

func (m *packageTestModule) DepsMutator(ctx BottomUpMutatorContext) {
	m.AddDeps(ctx, packagingDepTag{})
	ctx.AddDependency(ctx.Module(), installDepTag{}, m.properties.Install_deps...)
}

func (m *packageTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	zipFile := PathForModuleOut(ctx, "myzip.zip")
	m.entries = m.CopyDepsToZip(ctx, m.GatherPackagingSpecs(ctx), zipFile)
}

type testConfig struct {
	multiTarget                bool
	depsCollectFirstTargetOnly bool
	debuggable                 bool
}

func runPackagingTest(t *testing.T, config testConfig, bp string, expected []string) {
	t.Helper()

	var archVariant string
	if config.multiTarget {
		archVariant = "android_common"
	} else {
		archVariant = "android_arm64_armv8-a"
	}

	moduleFactory := func() Module {
		return packageTestModuleFactory(config.multiTarget, config.depsCollectFirstTargetOnly)
	}

	result := GroupFixturePreparers(
		PrepareForTestWithArchMutator,
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterModuleType("component", componentTestModuleFactory)
			ctx.RegisterModuleType("package_module", moduleFactory)
		}),
		FixtureModifyProductVariables(func(variables FixtureProductVariables) {
			variables.Debuggable = proptools.BoolPtr(config.debuggable)
		}),
		FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	p := result.Module("package", archVariant).(*packageTestModule)
	actual := p.entries
	actual = SortedUniqueStrings(actual)
	expected = SortedUniqueStrings(expected)
	AssertDeepEquals(t, "package entries", expected, actual)
}

func TestPackagingBaseMultiTarget(t *testing.T) {
	config := testConfig{
		multiTarget:                true,
		depsCollectFirstTargetOnly: false,
	}
	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		package_module {
			name: "package",
			deps: ["foo"],
		}
		`, []string{"lib64/foo"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
			deps: ["bar"],
		}

		component {
			name: "bar",
		}

		package_module {
			name: "package",
			deps: ["foo"],
		}
		`, []string{"lib64/foo", "lib64/bar"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
			deps: ["bar"],
		}

		component {
			name: "bar",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			compile_multilib: "both",
		}
		`, []string{"lib32/foo", "lib32/bar", "lib64/foo", "lib64/bar"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
			compile_multilib: "32",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			multilib: {
				lib32: {
					deps: ["bar"],
				},
			},
			compile_multilib: "both",
		}
		`, []string{"lib32/foo", "lib32/bar", "lib64/foo"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			multilib: {
				first: {
					deps: ["bar"],
				},
			},
			compile_multilib: "both",
		}
		`, []string{"lib32/foo", "lib64/foo", "lib64/bar"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
		}

		component {
			name: "baz",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			arch: {
				arm64: {
					deps: ["bar"],
				},
				x86_64: {
					deps: ["baz"],
				},
			},
			compile_multilib: "both",
		}
		`, []string{"lib32/foo", "lib64/foo", "lib64/bar"})
}

func TestPackagingBaseSingleTarget(t *testing.T) {
	config := testConfig{
		multiTarget:                false,
		depsCollectFirstTargetOnly: false,
	}
	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		package_module {
			name: "package",
			deps: ["foo"],
		}
		`, []string{"lib64/foo"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
			deps: ["bar"],
		}

		component {
			name: "bar",
		}

		package_module {
			name: "package",
			deps: ["foo"],
		}
		`, []string{"lib64/foo", "lib64/bar"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
			compile_multilib: "32",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			multilib: {
				lib32: {
					deps: ["bar"],
				},
			},
		}
		`, []string{"lib64/foo"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			multilib: {
				lib64: {
					deps: ["bar"],
				},
			},
		}
		`, []string{"lib64/foo", "lib64/bar"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
		}

		component {
			name: "baz",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			arch: {
				arm64: {
					deps: ["bar"],
				},
				x86_64: {
					deps: ["baz"],
				},
			},
		}
		`, []string{"lib64/foo", "lib64/bar"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			install_deps: ["bar"],
		}
		`, []string{"lib64/foo"})
}

func TestPackagingWithSkipInstallDeps(t *testing.T) {
	// package -[dep]-> foo -[dep]-> bar      -[dep]-> baz
	// Packaging should continue transitively through modules that are not installed.
	config := testConfig{
		multiTarget:                false,
		depsCollectFirstTargetOnly: false,
	}
	runPackagingTest(t, config,
		`
		component {
			name: "foo",
			deps: ["bar"],
		}

		component {
			name: "bar",
			deps: ["baz"],
			skip_install: true,
		}

		component {
			name: "baz",
		}

		package_module {
			name: "package",
			deps: ["foo"],
		}
		`, []string{"lib64/foo", "lib64/bar", "lib64/baz"})
}

func TestPackagingWithDepsCollectFirstTargetOnly(t *testing.T) {
	config := testConfig{
		multiTarget:                true,
		depsCollectFirstTargetOnly: true,
	}
	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		package_module {
			name: "package",
			deps: ["foo"],
		}
		`, []string{"lib64/foo"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
			deps: ["bar"],
		}

		component {
			name: "bar",
		}

		package_module {
			name: "package",
			deps: ["foo"],
		}
		`, []string{"lib64/foo", "lib64/bar"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
			deps: ["bar"],
		}

		component {
			name: "bar",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			compile_multilib: "both",
		}
		`, []string{"lib64/foo", "lib64/bar"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
			compile_multilib: "32",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			multilib: {
				lib32: {
					deps: ["bar"],
				},
			},
			compile_multilib: "both",
		}
		`, []string{"lib32/bar", "lib64/foo"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			multilib: {
				both: {
					deps: ["bar"],
				},
			},
			compile_multilib: "both",
		}
		`, []string{"lib64/foo", "lib32/bar", "lib64/bar"})

	runPackagingTest(t, config,
		`
		component {
			name: "foo",
		}

		component {
			name: "bar",
		}

		component {
			name: "baz",
		}

		package_module {
			name: "package",
			deps: ["foo"],
			arch: {
				arm64: {
					deps: ["bar"],
				},
				x86_64: {
					deps: ["baz"],
				},
			},
			compile_multilib: "both",
		}
		`, []string{"lib64/foo", "lib64/bar"})
}

func TestDebuggableDeps(t *testing.T) {
	bp := `
		component {
			name: "foo",
		}

		component {
			name: "bar",
			deps: ["baz"],
		}

		component {
			name: "baz",
		}

		package_module {
			name: "package",
			deps: ["foo"] + select(product_variable("debuggable"), {
				true: ["bar"],
				default: [],
			}),
		}`
	testcases := []struct {
		debuggable bool
		expected   []string
	}{
		{
			debuggable: true,
			expected:   []string{"lib64/foo", "lib64/bar", "lib64/baz"},
		},
		{
			debuggable: false,
			expected:   []string{"lib64/foo"},
		},
	}
	for _, tc := range testcases {
		config := testConfig{
			debuggable: tc.debuggable,
		}
		runPackagingTest(t, config, bp, tc.expected)
	}
}

func TestPrefer32Deps(t *testing.T) {
	bpTemplate := `
		component {
			name: "foo",
			compile_multilib: "both", // not needed but for clarity
		}

		component {
			name: "foo_32only",
			compile_multilib: "prefer32",
		}

		component {
			name: "foo_64only",
			compile_multilib: "64",
		}

		package_module {
			name: "package",
			compile_multilib: "%COMPILE_MULTILIB%",
			multilib: {
				prefer32: {
					deps: %DEPS%,
				},
			},
		}
	`

	testcases := []struct {
		compileMultilib string
		deps            []string
		expected        []string
	}{
		{
			compileMultilib: "first",
			deps:            []string{"foo", "foo_64only"},
			expected:        []string{"lib64/foo", "lib64/foo_64only"},
		},
		{
			compileMultilib: "64",
			deps:            []string{"foo", "foo_64only"},
			expected:        []string{"lib64/foo", "lib64/foo_64only"},
		},
		{
			compileMultilib: "32",
			deps:            []string{"foo", "foo_32only"},
			expected:        []string{"lib32/foo", "lib32/foo_32only"},
		},
		{
			compileMultilib: "both",
			deps:            []string{"foo", "foo_32only", "foo_64only"},
			expected:        []string{"lib32/foo", "lib32/foo_32only", "lib64/foo_64only"},
		},
	}
	for _, tc := range testcases {
		config := testConfig{
			multiTarget:                true,
			depsCollectFirstTargetOnly: true,
		}
		bp := strings.Replace(bpTemplate, "%COMPILE_MULTILIB%", tc.compileMultilib, -1)
		bp = strings.Replace(bp, "%DEPS%", `["`+strings.Join(tc.deps, `", "`)+`"]`, -1)
		runPackagingTest(t, config, bp, tc.expected)
	}
}
