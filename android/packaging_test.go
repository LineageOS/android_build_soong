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
	"reflect"
	"testing"

	"github.com/google/blueprint"
)

// Module to be packaged
type componentTestModule struct {
	ModuleBase
	props struct {
		Deps []string
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
	ctx.InstallFile(installDir, m.Name(), builtFile)
}

// Module that itself is a package
type packageTestModule struct {
	ModuleBase
	PackagingBase

	entries []string
}

func packageMultiTargetTestModuleFactory() Module {
	module := &packageTestModule{}
	InitPackageModule(module)
	InitAndroidMultiTargetsArchModule(module, DeviceSupported, MultilibCommon)
	return module
}

func packageTestModuleFactory() Module {
	module := &packageTestModule{}
	InitPackageModule(module)
	InitAndroidArchModule(module, DeviceSupported, MultilibBoth)
	return module
}

func (m *packageTestModule) DepsMutator(ctx BottomUpMutatorContext) {
	m.AddDeps(ctx, installDepTag{})
}

func (m *packageTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	zipFile := PathForModuleOut(ctx, "myzip.zip")
	m.entries = m.CopyDepsToZip(ctx, zipFile)
}

func runPackagingTest(t *testing.T, multitarget bool, bp string, expected []string) {
	t.Helper()

	config := TestArchConfig(buildDir, nil, bp, nil)

	ctx := NewTestArchContext(config)
	ctx.RegisterModuleType("component", componentTestModuleFactory)

	var archVariant string
	if multitarget {
		archVariant = "android_common"
		ctx.RegisterModuleType("package_module", packageMultiTargetTestModuleFactory)
	} else {
		archVariant = "android_arm64_armv8-a"
		ctx.RegisterModuleType("package_module", packageTestModuleFactory)
	}
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	p := ctx.ModuleForTests("package", archVariant).Module().(*packageTestModule)
	actual := p.entries
	actual = SortedUniqueStrings(actual)
	expected = SortedUniqueStrings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("\ngot: %v\nexpected: %v\n", actual, expected)
	}
}

func TestPackagingBaseMultiTarget(t *testing.T) {
	multiTarget := true
	runPackagingTest(t, multiTarget,
		`
		component {
			name: "foo",
		}

		package_module {
			name: "package",
			deps: ["foo"],
		}
		`, []string{"lib64/foo"})

	runPackagingTest(t, multiTarget,
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

	runPackagingTest(t, multiTarget,
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

	runPackagingTest(t, multiTarget,
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

	runPackagingTest(t, multiTarget,
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

	runPackagingTest(t, multiTarget,
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
	multiTarget := false
	runPackagingTest(t, multiTarget,
		`
		component {
			name: "foo",
		}

		package_module {
			name: "package",
			deps: ["foo"],
		}
		`, []string{"lib64/foo"})

	runPackagingTest(t, multiTarget,
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

	runPackagingTest(t, multiTarget,
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

	runPackagingTest(t, multiTarget,
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

	runPackagingTest(t, multiTarget,
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
}
