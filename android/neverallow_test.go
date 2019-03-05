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
	"io/ioutil"
	"os"
	"testing"
)

var neverallowTests = []struct {
	name          string
	fs            map[string][]byte
	expectedError string
}{
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
		name: "dependency on core-libart",
		fs: map[string][]byte{
			"Blueprints": []byte(`
				java_library {
					name: "needs_core_libart",
					libs: ["core-libart"],
				}`),
		},
		expectedError: "Only core libraries projects can depend on core-libart",
	},
	{
		name: "dependency on updatable-media",
		fs: map[string][]byte{
			"Blueprints": []byte(`
				java_library {
					name: "needs_updatable_media",
					libs: ["updatable-media"],
				}`),
		},
		expectedError: "updatable-media includes private APIs. Use updatable_media_stubs instead.",
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
}

func TestNeverallow(t *testing.T) {
	buildDir, err := ioutil.TempDir("", "soong_neverallow_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

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
	Vendor_available *bool

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

func (p *mockCcLibraryModule) GenerateAndroidBuildActions(ModuleContext) {
}

type mockJavaLibraryProperties struct {
	Libs []string
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
