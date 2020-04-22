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

	"github.com/google/blueprint"
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

func testContext(config android.Config) *android.TestContext {

	ctx := android.NewTestArchContext()
	java.RegisterJavaBuildComponents(ctx)
	java.RegisterAppBuildComponents(ctx)
	java.RegisterSystemModulesBuildComponents(ctx)

	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("sysprop_deps", syspropDepsMutator).Parallel()
	})

	cc.RegisterRequiredBuildComponentsForTest(ctx)
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("sysprop_java", java.SyspropMutator).Parallel()
	})

	ctx.RegisterModuleType("sysprop_library", syspropLibraryFactory)

	ctx.Register(config)

	return ctx
}

func run(t *testing.T, ctx *android.TestContext, config android.Config) {
	t.Helper()
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)
}

func testConfig(env map[string]string, bp string, fs map[string][]byte) android.Config {
	bp += cc.GatherRequiredDepsForTest(android.Android)

	mockFS := map[string][]byte{
		"a.java":                           nil,
		"b.java":                           nil,
		"c.java":                           nil,
		"d.cpp":                            nil,
		"api/sysprop-platform-current.txt": nil,
		"api/sysprop-platform-latest.txt":  nil,
		"api/sysprop-platform-on-product-current.txt": nil,
		"api/sysprop-platform-on-product-latest.txt":  nil,
		"api/sysprop-vendor-current.txt":              nil,
		"api/sysprop-vendor-latest.txt":               nil,
		"api/sysprop-odm-current.txt":                 nil,
		"api/sysprop-odm-latest.txt":                  nil,
		"framework/aidl/a.aidl":                       nil,

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
		"com/android2/OdmProperties.sysprop":         nil,
	}

	for k, v := range fs {
		mockFS[k] = v
	}

	config := java.TestConfig(buildDir, env, bp, mockFS)

	config.TestProductVariables.DeviceSystemSdkVersions = []string{"28"}
	config.TestProductVariables.DeviceVndkVersion = proptools.StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = proptools.StringPtr("VER")

	return config

}

func test(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	config := testConfig(nil, bp, nil)
	ctx := testContext(config)
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
			host_supported: true,
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

		sysprop_library {
			name: "sysprop-odm",
			srcs: ["com/android2/OdmProperties.sysprop"],
			api_packages: ["com.android2"],
			property_owner: "Odm",
			device_specific: true,
		}

		java_library {
			name: "java-platform",
			srcs: ["c.java"],
			sdk_version: "system_current",
			libs: ["sysprop-platform"],
		}

		java_library {
			name: "java-platform-private",
			srcs: ["c.java"],
			platform_apis: true,
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

		cc_library {
			name: "libbase",
			host_supported: true,
		}

		cc_library_headers {
			name: "libbase_headers",
			vendor_available: true,
			recovery_available: true,
		}

		cc_library {
			name: "liblog",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			recovery_available: true,
			host_supported: true,
		}

		cc_binary_host {
			name: "hostbin",
			static_libs: ["sysprop-platform"],
		}

		llndk_library {
			name: "liblog",
			symbol_file: "",
		}

		java_library {
			name: "sysprop-library-stub-platform",
			sdk_version: "core_current",
		}

		java_library {
			name: "sysprop-library-stub-vendor",
			soc_specific: true,
			sdk_version: "core_current",
		}
		`)

	// Check for generated cc_library
	for _, variant := range []string{
		"android_vendor.VER_arm_armv7-a-neon_shared",
		"android_vendor.VER_arm_armv7-a-neon_static",
		"android_vendor.VER_arm64_armv8-a_shared",
		"android_vendor.VER_arm64_armv8-a_static",
	} {
		ctx.ModuleForTests("libsysprop-platform", variant)
		ctx.ModuleForTests("libsysprop-vendor", variant)
		ctx.ModuleForTests("libsysprop-odm", variant)
	}

	for _, variant := range []string{
		"android_arm_armv7-a-neon_shared",
		"android_arm_armv7-a-neon_static",
		"android_arm64_armv8-a_shared",
		"android_arm64_armv8-a_static",
	} {
		ctx.ModuleForTests("libsysprop-platform", variant)

		// core variant of vendor-owned sysprop_library is for product
		ctx.ModuleForTests("libsysprop-vendor", variant)
	}

	ctx.ModuleForTests("sysprop-platform", "android_common")
	ctx.ModuleForTests("sysprop-platform_public", "android_common")
	ctx.ModuleForTests("sysprop-vendor", "android_common")

	// Check for exported includes
	coreVariant := "android_arm64_armv8-a_static"
	vendorVariant := "android_vendor.VER_arm64_armv8-a_static"

	platformInternalPath := "libsysprop-platform/android_arm64_armv8-a_static/gen/sysprop/include"
	platformPublicCorePath := "libsysprop-platform/android_arm64_armv8-a_static/gen/sysprop/public/include"
	platformPublicVendorPath := "libsysprop-platform/android_vendor.VER_arm64_armv8-a_static/gen/sysprop/public/include"

	platformOnProductPath := "libsysprop-platform-on-product/android_arm64_armv8-a_static/gen/sysprop/public/include"

	vendorInternalPath := "libsysprop-vendor/android_vendor.VER_arm64_armv8-a_static/gen/sysprop/include"
	vendorPublicPath := "libsysprop-vendor/android_arm64_armv8-a_static/gen/sysprop/public/include"

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

	// Product should use platform's and vendor's public headers
	if !strings.Contains(productFlags, platformOnProductPath) ||
		!strings.Contains(productFlags, vendorPublicPath) {
		t.Errorf("flags for product must contain %#v and %#v, but was %#v.",
			platformPublicCorePath, vendorPublicPath, productFlags)
	}

	vendorClient := ctx.ModuleForTests("cc-client-vendor", vendorVariant)
	vendorFlags := vendorClient.Rule("cc").Args["cFlags"]

	// Vendor should use platform's public header and vendor's internal header
	if !strings.Contains(vendorFlags, platformPublicVendorPath) ||
		!strings.Contains(vendorFlags, vendorInternalPath) {
		t.Errorf("flags for vendor must contain %#v and %#v, but was %#v.",
			platformPublicVendorPath, vendorInternalPath, vendorFlags)
	}

	// Java modules linking against system API should use public stub
	javaSystemApiClient := ctx.ModuleForTests("java-platform", "android_common")
	publicStubFound := false
	ctx.VisitDirectDeps(javaSystemApiClient.Module(), func(dep blueprint.Module) {
		if dep.Name() == "sysprop-platform_public" {
			publicStubFound = true
		}
	})
	if !publicStubFound {
		t.Errorf("system api client should use public stub")
	}

}
