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
	"regexp"
	"testing"

	"github.com/google/blueprint"
)

var neverallowTests = []struct {
	// The name of the test.
	name string

	// Optional test specific rules. If specified then they are used instead of the default rules.
	rules []Rule

	// Additional contents to add to the virtual filesystem used by the tests.
	fs MockFS

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
			regexp.QuoteMeta("module \"libother\": violates neverallow requirements. Not allowed:\n\tdep(s): [\"not_allowed_in_direct_deps\"]"),
		},
	},
	{
		name: "multiple constraints",
		rules: []Rule{
			NeverAllow().
				InDirectDeps("not_allowed_in_direct_deps").
				In("other").
				ModuleType("cc_library").
				NotIn("top").
				NotModuleType("cc_binary"),
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
			regexp.QuoteMeta(`module "libother": violates neverallow requirements. Not allowed:
	in dirs: ["other/"]
	module types: ["cc_library"]
	dep(s): ["not_allowed_in_direct_deps"]
	EXCEPT in dirs: ["top/"]
	EXCEPT module types: ["cc_binary"]`),
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
		name: "include_dir not allowed to reference art",
		fs: map[string][]byte{
			"system/libfmq/Android.bp": []byte(`
				cc_library {
					name: "libother",
					include_dirs: ["any/random/file"],
				}`),
		},
		expectedErrors: []string{
			"all usages of them in 'system/libfmq' have been migrated",
		},
	},
	{
		name: "include_dir can work",
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
		name: "java_device_for_host",
		fs: map[string][]byte{
			"Android.bp": []byte(`
				java_device_for_host {
					name: "device_for_host",
					libs: ["core-libart"],
				}`),
		},
		expectedErrors: []string{
			"java_device_for_host can only be used in allowed projects",
		},
	},
	// CC sdk rule tests
	{
		name: `"sdk_variant_only" outside allowed list`,
		fs: map[string][]byte{
			"Android.bp": []byte(`
				cc_library {
					name: "outside_allowed_list",
					sdk_version: "current",
					sdk_variant_only: true,
				}`),
		},
		expectedErrors: []string{
			`module "outside_allowed_list": violates neverallow`,
		},
	},
	{
		name: `"sdk_variant_only: false" outside allowed list`,
		fs: map[string][]byte{
			"Android.bp": []byte(`
				cc_library {
					name: "outside_allowed_list",
					sdk_version: "current",
					sdk_variant_only: false,
				}`),
		},
		expectedErrors: []string{
			`module "outside_allowed_list": violates neverallow`,
		},
	},
	{
		name: `"platform" outside allowed list`,
		fs: map[string][]byte{
			"Android.bp": []byte(`
				cc_library {
					name: "outside_allowed_list",
					platform: {
						shared_libs: ["libfoo"],
					},
				}`),
		},
		expectedErrors: []string{
			`module "outside_allowed_list": violates neverallow`,
		},
	},
	{
		name: "uncompress_dex inside art",
		fs: map[string][]byte{
			"art/Android.bp": []byte(`
				java_library {
					name: "inside_art_libraries",
					uncompress_dex: true,
				}`),
		},
	},
	{
		name: "uncompress_dex outside art",
		fs: map[string][]byte{
			"other/Android.bp": []byte(`
				java_library {
					name: "outside_art_libraries",
					uncompress_dex: true,
				}`),
		},
		expectedErrors: []string{
			"module \"outside_art_libraries\": violates neverallow",
		},
	},
	// Tests for the rule prohibiting the use of framework
	{
		name: "prohibit framework",
		fs: map[string][]byte{
			"Android.bp": []byte(`
				java_library {
					name: "foo",
					libs: ["framework"],
					sdk_version: "current",
				}`),
		},
		expectedErrors: []string{
			"framework can't be used when building against SDK",
		},
	},
	// Test for the rule restricting use of implementation_installable
	{
		name: `"implementation_installable" outside allowed list`,
		fs: map[string][]byte{
			"Android.bp": []byte(`
				cc_library {
					name: "outside_allowed_list",
					stubs: {
                                                implementation_installable: true,
					},
				}`),
		},
		expectedErrors: []string{
			`module "outside_allowed_list": violates neverallow`,
		},
	},
	// Test for the rule restricting use of exclude_static_libs
	{
		name: `"exclude_static_libs" outside allowed directory`,
		fs: map[string][]byte{
			"a/b/Android.bp": []byte(`
				java_library {
					name: "baz",
					exclude_static_libs: [
						"bar",
					],
				}
			`),
		},
		expectedErrors: []string{
			`exclude_static_libs property is only allowed for java modules defined in build/soong, libcore, and frameworks/base/api`,
		},
	},
	// Test for only allowing headers_only for framework-minus-apex-headers
	{
		name: `"headers_only" outside framework-minus-apex-headers modules`,
		fs: map[string][]byte{
			"a/b/Android.bp": []byte(`
				java_library {
					name: "baz",
					headers_only: true,
				}
			`),
		},
		expectedErrors: []string{
			`headers_only can only be used for generating framework-minus-apex headers for non-updatable modules`,
		},
	},
}

var prepareForNeverAllowTest = GroupFixturePreparers(
	FixtureRegisterWithContext(func(ctx RegistrationContext) {
		ctx.RegisterModuleType("cc_library", newMockCcLibraryModule)
		ctx.RegisterModuleType("java_library", newMockJavaLibraryModule)
		ctx.RegisterModuleType("java_library_host", newMockJavaLibraryModule)
		ctx.RegisterModuleType("java_device_for_host", newMockJavaLibraryModule)
	}),
)

func TestNeverallow(t *testing.T) {
	for _, test := range neverallowTests {
		t.Run(test.name, func(t *testing.T) {
			GroupFixturePreparers(
				prepareForNeverAllowTest,
				PrepareForTestWithNeverallowRules(test.rules),
				test.fs.AddToFixture(),
			).
				ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern(test.expectedErrors)).
				RunTest(t)
		})
	}
}

type mockCcLibraryProperties struct {
	Include_dirs     []string
	Vendor_available *bool
	Static_libs      []string
	Sdk_version      *string
	Sdk_variant_only *bool

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

	Platform struct {
		Shared_libs []string
	}

	Stubs struct {
		Implementation_installable *bool
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
	Libs                []string
	Sdk_version         *string
	Uncompress_dex      *bool
	Exclude_static_libs []string
	Headers_only        *bool
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
