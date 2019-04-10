// Copyright (C) 2019 The Android Open Source Project
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

package sysprop

import (
	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"

	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_sysprop_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}

func testContext(config android.Config, bp string,
	fs map[string][]byte) *android.TestContext {

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("android_app", android.ModuleFactoryAdaptor(java.AndroidAppFactory))
	ctx.RegisterModuleType("droiddoc_template", android.ModuleFactoryAdaptor(java.ExportedDroiddocDirFactory))
	ctx.RegisterModuleType("java_library", android.ModuleFactoryAdaptor(java.LibraryFactory))
	ctx.RegisterModuleType("java_system_modules", android.ModuleFactoryAdaptor(java.SystemModulesFactory))
	ctx.RegisterModuleType("prebuilt_apis", android.ModuleFactoryAdaptor(java.PrebuiltApisFactory))
	ctx.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("load_hooks", android.LoadHookMutator).Parallel()
	})
	ctx.PreArchMutators(android.RegisterPrebuiltsPreArchMutators)
	ctx.PreArchMutators(android.RegisterPrebuiltsPostDepsMutators)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("prebuilt_apis", java.PrebuiltApisMutator).Parallel()
		ctx.TopDown("java_sdk_library", java.SdkLibraryMutator).Parallel()
	})

	ctx.RegisterModuleType("cc_library", android.ModuleFactoryAdaptor(cc.LibraryFactory))
	ctx.RegisterModuleType("cc_library_headers", android.ModuleFactoryAdaptor(cc.LibraryHeaderFactory))
	ctx.RegisterModuleType("cc_library_static", android.ModuleFactoryAdaptor(cc.LibraryFactory))
	ctx.RegisterModuleType("cc_object", android.ModuleFactoryAdaptor(cc.ObjectFactory))
	ctx.RegisterModuleType("llndk_library", android.ModuleFactoryAdaptor(cc.LlndkLibraryFactory))
	ctx.RegisterModuleType("toolchain_library", android.ModuleFactoryAdaptor(cc.ToolchainLibraryFactory))
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("image", cc.ImageMutator).Parallel()
		ctx.BottomUp("link", cc.LinkageMutator).Parallel()
		ctx.BottomUp("vndk", cc.VndkMutator).Parallel()
		ctx.BottomUp("version", cc.VersionMutator).Parallel()
		ctx.BottomUp("begin", cc.BeginMutator).Parallel()
		ctx.BottomUp("sysprop", cc.SyspropMutator).Parallel()
	})

	ctx.RegisterModuleType("sysprop_library", android.ModuleFactoryAdaptor(syspropLibraryFactory))

	ctx.Register()

	bp += java.GatherRequiredDepsForTest()
	bp += cc.GatherRequiredDepsForTest(android.Android)

	mockFS := map[string][]byte{
		"Android.bp":             []byte(bp),
		"a.java":                 nil,
		"b.java":                 nil,
		"c.java":                 nil,
		"d.cpp":                  nil,
		"api/current.txt":        nil,
		"api/removed.txt":        nil,
		"api/system-current.txt": nil,
		"api/system-removed.txt": nil,
		"api/test-current.txt":   nil,
		"api/test-removed.txt":   nil,
		"framework/aidl/a.aidl":  nil,

		"prebuilts/sdk/current/core/android.jar":                              nil,
		"prebuilts/sdk/current/public/android.jar":                            nil,
		"prebuilts/sdk/current/public/framework.aidl":                         nil,
		"prebuilts/sdk/current/public/core.jar":                               nil,
		"prebuilts/sdk/current/system/android.jar":                            nil,
		"prebuilts/sdk/current/test/android.jar":                              nil,
		"prebuilts/sdk/28/public/api/sysprop-platform.txt":                    nil,
		"prebuilts/sdk/28/system/api/sysprop-platform.txt":                    nil,
		"prebuilts/sdk/28/test/api/sysprop-platform.txt":                      nil,
		"prebuilts/sdk/28/public/api/sysprop-platform-removed.txt":            nil,
		"prebuilts/sdk/28/system/api/sysprop-platform-removed.txt":            nil,
		"prebuilts/sdk/28/test/api/sysprop-platform-removed.txt":              nil,
		"prebuilts/sdk/28/public/api/sysprop-platform-on-product.txt":         nil,
		"prebuilts/sdk/28/system/api/sysprop-platform-on-product.txt":         nil,
		"prebuilts/sdk/28/test/api/sysprop-platform-on-product.txt":           nil,
		"prebuilts/sdk/28/public/api/sysprop-platform-on-product-removed.txt": nil,
		"prebuilts/sdk/28/system/api/sysprop-platform-on-product-removed.txt": nil,
		"prebuilts/sdk/28/test/api/sysprop-platform-on-product-removed.txt":   nil,
		"prebuilts/sdk/28/public/api/sysprop-vendor.txt":                      nil,
		"prebuilts/sdk/28/system/api/sysprop-vendor.txt":                      nil,
		"prebuilts/sdk/28/test/api/sysprop-vendor.txt":                        nil,
		"prebuilts/sdk/28/public/api/sysprop-vendor-removed.txt":              nil,
		"prebuilts/sdk/28/system/api/sysprop-vendor-removed.txt":              nil,
		"prebuilts/sdk/28/test/api/sysprop-vendor-removed.txt":                nil,
		"prebuilts/sdk/tools/core-lambda-stubs.jar":                           nil,
		"prebuilts/sdk/Android.bp":                                            []byte(`prebuilt_apis { name: "sdk", api_dirs: ["28", "current"],}`),

		// For framework-res, which is an implicit dependency for framework
		"AndroidManifest.xml":                        nil,
		"build/make/target/product/security/testkey": nil,

		"build/soong/scripts/jar-wrapper.sh": nil,

		"build/make/core/proguard.flags":             nil,
		"build/make/core/proguard_basic_keeps.flags": nil,

		"jdk8/jre/lib/jce.jar": nil,
		"jdk8/jre/lib/rt.jar":  nil,
		"jdk8/lib/tools.jar":   nil,

		"bar-doc/a.java":                 nil,
		"bar-doc/b.java":                 nil,
		"bar-doc/IFoo.aidl":              nil,
		"bar-doc/known_oj_tags.txt":      nil,
		"external/doclava/templates-sdk": nil,

		"cert/new_cert.x509.pem": nil,
		"cert/new_cert.pk8":      nil,

		"android/sysprop/PlatformProperties.sysprop": nil,
		"com/android/VendorProperties.sysprop":       nil,
	}

	for k, v := range fs {
		mockFS[k] = v
	}

	ctx.MockFileSystem(mockFS)

	return ctx
}

func run(t *testing.T, ctx *android.TestContext, config android.Config) {
	t.Helper()
	_, errs := ctx.ParseFileList(".", []string{"Android.bp", "prebuilts/sdk/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)
}

func testConfig(env map[string]string) android.Config {
	config := java.TestConfig(buildDir, env)

	config.TestProductVariables.DeviceSystemSdkVersions = []string{"28"}
	config.TestProductVariables.DeviceVndkVersion = proptools.StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = proptools.StringPtr("VER")

	return config

}

func test(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	config := testConfig(nil)
	ctx := testContext(config, bp, nil)
	run(t, ctx, config)

	return ctx
}

func TestSyspropLibrary(t *testing.T) {
	ctx := test(t, `
		sysprop_library {
			name: "sysprop-platform",
			srcs: ["android/sysprop/PlatformProperties.sysprop"],
			api_packages: ["android.sysprop"],
			property_owner: "Platform",
			vendor_available: true,
		}

		sysprop_library {
			name: "sysprop-platform-on-product",
			srcs: ["android/sysprop/PlatformProperties.sysprop"],
			api_packages: ["android.sysprop"],
			property_owner: "Platform",
			product_specific: true,
		}

		sysprop_library {
			name: "sysprop-vendor",
			srcs: ["com/android/VendorProperties.sysprop"],
			api_packages: ["com.android"],
			property_owner: "Vendor",
			product_specific: true,
			vendor_available: true,
		}

		java_library {
			name: "java-platform",
			srcs: ["c.java"],
			sdk_version: "system_current",
			libs: ["sysprop-platform"],
		}

		java_library {
			name: "java-product",
			srcs: ["c.java"],
			sdk_version: "system_current",
			product_specific: true,
			libs: ["sysprop-platform", "sysprop-vendor"],
		}

		java_library {
			name: "java-vendor",
			srcs: ["c.java"],
			sdk_version: "system_current",
			soc_specific: true,
			libs: ["sysprop-platform", "sysprop-vendor"],
		}

		cc_library {
			name: "cc-client-platform",
			srcs: ["d.cpp"],
			static_libs: ["sysprop-platform"],
		}

		cc_library_static {
			name: "cc-client-platform-static",
			srcs: ["d.cpp"],
			whole_static_libs: ["sysprop-platform"],
		}

		cc_library {
			name: "cc-client-product",
			srcs: ["d.cpp"],
			product_specific: true,
			static_libs: ["sysprop-platform-on-product", "sysprop-vendor"],
		}

		cc_library {
			name: "cc-client-vendor",
			srcs: ["d.cpp"],
			soc_specific: true,
			static_libs: ["sysprop-platform", "sysprop-vendor"],
		}

		cc_library_headers {
			name: "libbase_headers",
			vendor_available: true,
			recovery_available: true,
		}

		cc_library {
			name: "liblog",
			no_libgcc: true,
			nocrt: true,
			system_shared_libs: [],
			recovery_available: true,
		}

		llndk_library {
			name: "liblog",
			symbol_file: "",
		}
		`)

	for _, variant := range []string{
		"android_arm_armv7-a-neon_core_shared",
		"android_arm_armv7-a-neon_core_static",
		"android_arm_armv7-a-neon_vendor_shared",
		"android_arm_armv7-a-neon_vendor_static",
		"android_arm64_armv8-a_core_shared",
		"android_arm64_armv8-a_core_static",
		"android_arm64_armv8-a_vendor_shared",
		"android_arm64_armv8-a_vendor_static",
	} {
		// Check for generated cc_library
		ctx.ModuleForTests("libsysprop-platform", variant)
		ctx.ModuleForTests("libsysprop-vendor", variant)
	}

	ctx.ModuleForTests("sysprop-platform", "android_common")
	ctx.ModuleForTests("sysprop-vendor", "android_common")

	// Check for exported includes
	coreVariant := "android_arm64_armv8-a_core_static"
	vendorVariant := "android_arm64_armv8-a_vendor_static"

	platformInternalPath := "libsysprop-platform/android_arm64_armv8-a_core_static/gen/sysprop/include"
	platformSystemCorePath := "libsysprop-platform/android_arm64_armv8-a_core_static/gen/sysprop/system/include"
	platformSystemVendorPath := "libsysprop-platform/android_arm64_armv8-a_vendor_static/gen/sysprop/system/include"

	platformOnProductPath := "libsysprop-platform-on-product/android_arm64_armv8-a_core_static/gen/sysprop/system/include"

	vendorInternalPath := "libsysprop-vendor/android_arm64_armv8-a_vendor_static/gen/sysprop/include"
	vendorSystemPath := "libsysprop-vendor/android_arm64_armv8-a_core_static/gen/sysprop/system/include"

	platformClient := ctx.ModuleForTests("cc-client-platform", coreVariant)
	platformFlags := platformClient.Rule("cc").Args["cFlags"]

	// platform should use platform's internal header
	if !strings.Contains(platformFlags, platformInternalPath) {
		t.Errorf("flags for platform must contain %#v, but was %#v.",
			platformInternalPath, platformFlags)
	}

	platformStaticClient := ctx.ModuleForTests("cc-client-platform-static", coreVariant)
	platformStaticFlags := platformStaticClient.Rule("cc").Args["cFlags"]

	// platform-static should use platform's internal header
	if !strings.Contains(platformStaticFlags, platformInternalPath) {
		t.Errorf("flags for platform-static must contain %#v, but was %#v.",
			platformInternalPath, platformStaticFlags)
	}

	productClient := ctx.ModuleForTests("cc-client-product", coreVariant)
	productFlags := productClient.Rule("cc").Args["cFlags"]

	// Product should use platform's and vendor's system headers
	if !strings.Contains(productFlags, platformOnProductPath) ||
		!strings.Contains(productFlags, vendorSystemPath) {
		t.Errorf("flags for product must contain %#v and %#v, but was %#v.",
			platformSystemCorePath, vendorSystemPath, productFlags)
	}

	vendorClient := ctx.ModuleForTests("cc-client-vendor", vendorVariant)
	vendorFlags := vendorClient.Rule("cc").Args["cFlags"]

	// Vendor should use platform's system header and vendor's internal header
	if !strings.Contains(vendorFlags, platformSystemVendorPath) ||
		!strings.Contains(vendorFlags, vendorInternalPath) {
		t.Errorf("flags for vendor must contain %#v and %#v, but was %#v.",
			platformSystemVendorPath, vendorInternalPath, vendorFlags)
	}
}
