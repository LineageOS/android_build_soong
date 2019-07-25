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

package android

import (
	"testing"

	"github.com/google/blueprint"
)

func init() {
	// Add extra rules needed for testing.
	AddNeverAllowRules(
		NeverAllow().InDirectDeps("not_allowed_in_direct_deps"),
	)
}

var neverallowTests = []struct {
	name          string
	fs            map[string][]byte
	expectedError string
}{
	// Test General Functionality

	// in direct deps tests
	{
		name: "not_allowed_in_direct_deps",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				cc_library {
					name: "not_allowed_in_direct_deps",
				}`),
			"other/Blueprints": []byte(`
				cc_library {
					name: "libother",
					static_libs: ["not_allowed_in_direct_deps"],
				}`),
		},
		expectedError: `module "libother": violates neverallow deps:not_allowed_in_direct_deps`,
	},

	// Test specific rules

	// include_dir rule tests
	{
		name: "include_dir not allowed to reference art",
		fs: map[string][]byte{
			"other/Blueprints": []byte(`
				cc_library {
					name: "libother",
					include_dirs: ["art/libdexfile/include"],
				}`),
		},
		expectedError: "all usages of 'art' have been migrated",
	},
	{
		name: "include_dir can reference another location",
		fs: map[string][]byte{
			"other/Blueprints": []byte(`
				cc_library {
					name: "libother",
					include_dirs: ["another/include"],
				}`),
		},
	},
	// Treble rule tests
	{
		name: "no vndk.enabled under vendor directory",
		fs: map[string][]byte{
			"vendor/Blueprints": []byte(`
				cc_library {
					name: "libvndk",
					vendor_available: true,
					vndk: {
						enabled: true,
					},
				}`),
		},
		expectedError: "VNDK can never contain a library that is device dependent",
	},
	{
		name: "no vndk.enabled under device directory",
		fs: map[string][]byte{
			"device/Blueprints": []byte(`
				cc_library {
					name: "libvndk",
					vendor_available: true,
					vndk: {
						enabled: true,
					},
				}`),
		},
		expectedError: "VNDK can never contain a library that is device dependent",
	},
	{
		name: "vndk-ext under vendor or device directory",
		fs: map[string][]byte{
			"device/Blueprints": []byte(`
				cc_library {
					name: "libvndk1_ext",
					vendor: true,
					vndk: {
						enabled: true,
					},
				}`),
			"vendor/Blueprints": []byte(`
				cc_library {
					name: "libvndk2_ext",
					vendor: true,
					vndk: {
						enabled: true,
					},
				}`),
		},
		expectedError: "",
	},

	{
		name: "no enforce_vintf_manifest.cflags",
		fs: map[string][]byte{
			"Blueprints": []byte(`
				cc_library {
					name: "libexample",
					product_variables: {
						enforce_vintf_manifest: {
							cflags: ["-DSHOULD_NOT_EXIST"],
						},
					},
				}`),
		},
		expectedError: "manifest enforcement should be independent",
	},
	{
		name: "libhidltransport enforce_vintf_manifest.cflags",
		fs: map[string][]byte{
			"Blueprints": []byte(`
				cc_library {
					name: "libhidltransport",
					product_variables: {
						enforce_vintf_manifest: {
							cflags: ["-DSHOULD_NOT_EXIST"],
						},
					},
				}`),
		},
		expectedError: "",
	},

	{
		name: "no treble_linker_namespaces.cflags",
		fs: map[string][]byte{
			"Blueprints": []byte(`
				cc_library {
					name: "libexample",
					product_variables: {
						treble_linker_namespaces: {
							cflags: ["-DSHOULD_NOT_EXIST"],
						},
					},
				}`),
		},
		expectedError: "nothing should care if linker namespaces are enabled or not",
	},
	{
		name: "libc_bionic_ndk treble_linker_namespaces.cflags",
		fs: map[string][]byte{
			"Blueprints": []byte(`
				cc_library {
					name: "libc_bionic_ndk",
					product_variables: {
						treble_linker_namespaces: {
							cflags: ["-DSHOULD_NOT_EXIST"],
						},
					},
				}`),
		},
		expectedError: "",
	},
	{
		name: "java_device_for_host",
		fs: map[string][]byte{
			"Blueprints": []byte(`
				java_device_for_host {
					name: "device_for_host",
					libs: ["core-libart"],
				}`),
		},
		expectedError: "java_device_for_host can only be used in whitelisted projects",
	},
	// Libcore rule tests
	{
		name: "sdk_version: \"none\" inside core libraries",
		fs: map[string][]byte{
			"libcore/Blueprints": []byte(`
				java_library {
					name: "inside_core_libraries",
					sdk_version: "none",
				}`),
		},
	},
	{
		name: "sdk_version: \"none\" outside core libraries",
		fs: map[string][]byte{
			"Blueprints": []byte(`
				java_library {
					name: "outside_core_libraries",
					sdk_version: "none",
				}`),
		},
		expectedError: "module \"outside_core_libraries\": violates neverallow",
	},
	{
		name: "sdk_version: \"current\"",
		fs: map[string][]byte{
			"Blueprints": []byte(`
				java_library {
					name: "outside_core_libraries",
					sdk_version: "current",
				}`),
		},
	},
}

func TestNeverallow(t *testing.T) {
	config := TestConfig(buildDir, nil)

	for _, test := range neverallowTests {
		t.Run(test.name, func(t *testing.T) {
			_, errs := testNeverallow(t, config, test.fs)

			if test.expectedError == "" {
				FailIfErrored(t, errs)
			} else {
				FailIfNoMatchingErrors(t, test.expectedError, errs)
			}
		})
	}
}

func testNeverallow(t *testing.T, config Config, fs map[string][]byte) (*TestContext, []error) {
	ctx := NewTestContext()
	ctx.RegisterModuleType("cc_library", ModuleFactoryAdaptor(newMockCcLibraryModule))
	ctx.RegisterModuleType("java_library", ModuleFactoryAdaptor(newMockJavaLibraryModule))
	ctx.RegisterModuleType("java_library_host", ModuleFactoryAdaptor(newMockJavaLibraryModule))
	ctx.RegisterModuleType("java_device_for_host", ModuleFactoryAdaptor(newMockJavaLibraryModule))
	ctx.PostDepsMutators(registerNeverallowMutator)
	ctx.Register()

	ctx.MockFileSystem(fs)

	_, errs := ctx.ParseBlueprintsFiles("Blueprints")
	if len(errs) > 0 {
		return ctx, errs
	}

	_, errs = ctx.PrepareBuildActions(config)
	return ctx, errs
}

type mockCcLibraryProperties struct {
	Include_dirs     []string
	Vendor_available *bool
	Static_libs      []string

	Vndk struct {
		Enabled                *bool
		Support_system_process *bool
		Extends                *string
	}

	Product_variables struct {
		Enforce_vintf_manifest struct {
			Cflags []string
		}

		Treble_linker_namespaces struct {
			Cflags []string
		}
	}
}

type mockCcLibraryModule struct {
	ModuleBase
	properties mockCcLibraryProperties
}

func newMockCcLibraryModule() Module {
	m := &mockCcLibraryModule{}
	m.AddProperties(&m.properties)
	InitAndroidModule(m)
	return m
}

type neverallowTestDependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var staticDepTag = neverallowTestDependencyTag{name: "static"}

func (c *mockCcLibraryModule) DepsMutator(ctx BottomUpMutatorContext) {
	for _, lib := range c.properties.Static_libs {
		ctx.AddDependency(ctx.Module(), staticDepTag, lib)
	}
}

func (p *mockCcLibraryModule) GenerateAndroidBuildActions(ModuleContext) {
}

type mockJavaLibraryProperties struct {
	Libs        []string
	Sdk_version *string
}

type mockJavaLibraryModule struct {
	ModuleBase
	properties mockJavaLibraryProperties
}

func newMockJavaLibraryModule() Module {
	m := &mockJavaLibraryModule{}
	m.AddProperties(&m.properties)
	InitAndroidModule(m)
	return m
}

func (p *mockJavaLibraryModule) GenerateAndroidBuildActions(ModuleContext) {
}
