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

var neverallowTests = []struct {
	// The name of the test.
	name string

	// Optional test specific rules. If specified then they are used instead of the default rules.
	rules []Rule

	// Additional contents to add to the virtual filesystem used by the tests.
	fs map[string][]byte

	// The expected error patterns. If empty then no errors are expected, otherwise each error
	// reported must be matched by at least one of these patterns. A pattern matches if the error
	// message contains the pattern. A pattern does not have to match the whole error message.
	expectedErrors []string
}{
	// Test General Functionality

	// in direct deps tests
	{
		name: "not_allowed_in_direct_deps",
		rules: []Rule{
			NeverAllow().InDirectDeps("not_allowed_in_direct_deps"),
		},
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				cc_library {
					name: "not_allowed_in_direct_deps",
				}`),
			"other/Android.bp": []byte(`
				cc_library {
					name: "libother",
					static_libs: ["not_allowed_in_direct_deps"],
				}`),
		},
		expectedErrors: []string{
			`module "libother": violates neverallow deps:not_allowed_in_direct_deps`,
		},
	},

	// Test android specific rules

	// include_dir rule tests
	{
		name: "include_dir not allowed to reference art",
		fs: map[string][]byte{
			"other/Android.bp": []byte(`
				cc_library {
					name: "libother",
					include_dirs: ["art/libdexfile/include"],
				}`),
		},
		expectedErrors: []string{
			"all usages of 'art' have been migrated",
		},
	},
	{
		name: "include_dir can reference another location",
		fs: map[string][]byte{
			"other/Android.bp": []byte(`
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
			"vendor/Android.bp": []byte(`
				cc_library {
					name: "libvndk",
					vendor_available: true,
					vndk: {
						enabled: true,
					},
				}`),
		},
		expectedErrors: []string{
			"VNDK can never contain a library that is device dependent",
		},
	},
	{
		name: "no vndk.enabled under device directory",
		fs: map[string][]byte{
			"device/Android.bp": []byte(`
				cc_library {
					name: "libvndk",
					vendor_available: true,
					vndk: {
						enabled: true,
					},
				}`),
		},
		expectedErrors: []string{
			"VNDK can never contain a library that is device dependent",
		},
	},
	{
		name: "vndk-ext under vendor or device directory",
		fs: map[string][]byte{
			"device/Android.bp": []byte(`
				cc_library {
					name: "libvndk1_ext",
					vendor: true,
					vndk: {
						enabled: true,
					},
				}`),
			"vendor/Android.bp": []byte(`
				cc_library {
					name: "libvndk2_ext",
					vendor: true,
					vndk: {
						enabled: true,
					},
				}`),
		},
	},

	{
		name: "no enforce_vintf_manifest.cflags",
		fs: map[string][]byte{
			"Android.bp": []byte(`
				cc_library {
					name: "libexample",
					product_variables: {
						enforce_vintf_manifest: {
							cflags: ["-DSHOULD_NOT_EXIST"],
						},
					},
				}`),
		},
		expectedErrors: []string{
			"manifest enforcement should be independent",
		},
	},

	{
		name: "no treble_linker_namespaces.cflags",
		fs: map[string][]byte{
			"Android.bp": []byte(`
				cc_library {
					name: "libexample",
					product_variables: {
						treble_linker_namespaces: {
							cflags: ["-DSHOULD_NOT_EXIST"],
						},
					},
				}`),
		},
		expectedErrors: []string{
			"nothing should care if linker namespaces are enabled or not",
		},
	},
	{
		name: "libc_bionic_ndk treble_linker_namespaces.cflags",
		fs: map[string][]byte{
			"Android.bp": []byte(`
				cc_library {
					name: "libc_bionic_ndk",
					product_variables: {
						treble_linker_namespaces: {
							cflags: ["-DSHOULD_NOT_EXIST"],
						},
					},
				}`),
		},
	},
	{
		name: "dependency on updatable-media",
		fs: map[string][]byte{
			"Android.bp": []byte(`
				java_library {
					name: "needs_updatable_media",
					libs: ["updatable-media"],
				}`),
		},
		expectedErrors: []string{
			"updatable-media includes private APIs. Use updatable_media_stubs instead.",
		},
	},
	{
		name: "java_device_for_host",
		fs: map[string][]byte{
			"Android.bp": []byte(`
				java_device_for_host {
					name: "device_for_host",
					libs: ["core-libart"],
				}`),
		},
		expectedErrors: []string{
			"java_device_for_host can only be used in whitelisted projects",
		},
	},
	// Libcore rule tests
	{
		name: "sdk_version: \"none\" inside core libraries",
		fs: map[string][]byte{
			"libcore/Android.bp": []byte(`
				java_library {
					name: "inside_core_libraries",
					sdk_version: "none",
				}`),
		},
	},
	{
		name: "sdk_version: \"none\" on android_*stubs_current stub",
		fs: map[string][]byte{
			"frameworks/base/Android.bp": []byte(`
				java_library {
					name: "android_stubs_current",
					sdk_version: "none",
				}`),
		},
	},
	{
		name: "sdk_version: \"none\" outside core libraries",
		fs: map[string][]byte{
			"Android.bp": []byte(`
				java_library {
					name: "outside_core_libraries",
					sdk_version: "none",
				}`),
		},
		expectedErrors: []string{
			"module \"outside_core_libraries\": violates neverallow",
		},
	},
	{
		name: "sdk_version: \"current\"",
		fs: map[string][]byte{
			"Android.bp": []byte(`
				java_library {
					name: "outside_core_libraries",
					sdk_version: "current",
				}`),
		},
	},
}

func TestNeverallow(t *testing.T) {
	for _, test := range neverallowTests {
		// Create a test per config to allow for test specific config, e.g. test rules.
		config := TestConfig(buildDir, nil, "", test.fs)

		t.Run(test.name, func(t *testing.T) {
			// If the test has its own rules then use them instead of the default ones.
			if test.rules != nil {
				setTestNeverallowRules(config, test.rules)
			}
			_, errs := testNeverallow(config)
			CheckErrorsAgainstExpectations(t, errs, test.expectedErrors)
		})
	}
}

func testNeverallow(config Config) (*TestContext, []error) {
	ctx := NewTestContext()
	ctx.RegisterModuleType("cc_library", newMockCcLibraryModule)
	ctx.RegisterModuleType("java_library", newMockJavaLibraryModule)
	ctx.RegisterModuleType("java_library_host", newMockJavaLibraryModule)
	ctx.RegisterModuleType("java_device_for_host", newMockJavaLibraryModule)
	ctx.PostDepsMutators(registerNeverallowMutator)
	ctx.Register(config)

	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
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
