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
	"os"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
	"android/soong/rust"

	"github.com/google/blueprint/proptools"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func test(t *testing.T, bp string) *android.TestResult {
	t.Helper()

	bp += `
		cc_library {
			name: "libbase",
			host_supported: true,
		}

		cc_library_headers {
			name: "libbase_headers",
			vendor_available: true,
			product_available: true,
			recovery_available: true,
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

		java_library {
			name: "sysprop-library-stub-product",
			product_specific: true,
			sdk_version: "core_current",
		}

		rust_library {
			name: "librustutils",
			crate_name: "rustutils",
			srcs: ["librustutils/lib.rs"],
			product_available: true,
			vendor_available: true,
			min_sdk_version: "29",
		}
	`

	mockFS := android.MockFS{
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
		"api/sysprop-vendor-on-product-current.txt":   nil,
		"api/sysprop-vendor-on-product-latest.txt":    nil,
		"api/sysprop-odm-current.txt":                 nil,
		"api/sysprop-odm-latest.txt":                  nil,
		"framework/aidl/a.aidl":                       nil,

		// For framework-res, which is an implicit dependency for framework
		"AndroidManifest.xml":                        nil,
		"build/make/target/product/security/testkey": nil,

		"build/soong/scripts/jar-wrapper.sh": nil,

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

		"librustutils/lib.rs": nil,
	}

	result := android.GroupFixturePreparers(
		cc.PrepareForTestWithCcDefaultModules,
		java.PrepareForTestWithJavaDefaultModules,
		rust.PrepareForTestWithRustDefaultModules,
		PrepareForTestWithSyspropBuildComponents,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.DeviceSystemSdkVersions = []string{"28"}
			variables.DeviceVndkVersion = proptools.StringPtr("current")
			variables.Platform_vndk_version = proptools.StringPtr("29")
			variables.DeviceCurrentApiLevelForVendorModules = proptools.StringPtr("28")
		}),
		java.FixtureWithPrebuiltApis(map[string][]string{
			"28": {},
			"29": {},
			"30": {},
		}),
		mockFS.AddToFixture(),
		android.FixtureWithRootAndroidBp(bp),
	).RunTest(t)

	return result
}

func TestSyspropLibrary(t *testing.T) {
	result := test(t, `
		sysprop_library {
			name: "sysprop-platform",
			apex_available: ["//apex_available:platform"],
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
			vendor: true,
		}

		sysprop_library {
			name: "sysprop-vendor-on-product",
			srcs: ["com/android/VendorProperties.sysprop"],
			api_packages: ["com.android"],
			property_owner: "Vendor",
			product_specific: true,
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
			libs: ["sysprop-platform", "sysprop-vendor-on-product"],
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
			static_libs: ["libsysprop-platform"],
		}

		cc_library_static {
			name: "cc-client-platform-static",
			srcs: ["d.cpp"],
			whole_static_libs: ["libsysprop-platform"],
		}

		cc_library {
			name: "cc-client-product",
			srcs: ["d.cpp"],
			product_specific: true,
			static_libs: ["libsysprop-platform-on-product", "libsysprop-vendor-on-product"],
		}

		cc_library {
			name: "cc-client-vendor",
			srcs: ["d.cpp"],
			soc_specific: true,
			static_libs: ["libsysprop-platform", "libsysprop-vendor"],
		}

		cc_binary_host {
			name: "hostbin",
			static_libs: ["libsysprop-platform"],
		}
	`)

	// Check for generated cc_library
	for _, variant := range []string{
		"android_vendor.29_arm_armv7-a-neon_shared",
		"android_vendor.29_arm_armv7-a-neon_static",
		"android_vendor.29_arm64_armv8-a_shared",
		"android_vendor.29_arm64_armv8-a_static",
	} {
		result.ModuleForTests("libsysprop-platform", variant)
		result.ModuleForTests("libsysprop-vendor", variant)
		result.ModuleForTests("libsysprop-odm", variant)
	}

	// product variant of vendor-owned sysprop_library
	for _, variant := range []string{
		"android_product.29_arm_armv7-a-neon_shared",
		"android_product.29_arm_armv7-a-neon_static",
		"android_product.29_arm64_armv8-a_shared",
		"android_product.29_arm64_armv8-a_static",
	} {
		result.ModuleForTests("libsysprop-vendor-on-product", variant)
	}

	for _, variant := range []string{
		"android_arm_armv7-a-neon_shared",
		"android_arm_armv7-a-neon_static",
		"android_arm64_armv8-a_shared",
		"android_arm64_armv8-a_static",
	} {
		library := result.ModuleForTests("libsysprop-platform", variant).Module().(*cc.Module)
		expectedApexAvailableOnLibrary := []string{"//apex_available:platform"}
		android.AssertDeepEquals(t, "apex available property on libsysprop-platform", expectedApexAvailableOnLibrary, library.ApexProperties.Apex_available)
	}

	result.ModuleForTests("sysprop-platform", "android_common")
	result.ModuleForTests("sysprop-platform_public", "android_common")
	result.ModuleForTests("sysprop-vendor", "android_common")
	result.ModuleForTests("sysprop-vendor-on-product", "android_common")

	// Check for exported includes
	coreVariant := "android_arm64_armv8-a_static"
	vendorVariant := "android_vendor.29_arm64_armv8-a_static"
	productVariant := "android_product.29_arm64_armv8-a_static"

	platformInternalPath := "libsysprop-platform/android_arm64_armv8-a_static/gen/sysprop/include"
	platformPublicVendorPath := "libsysprop-platform/android_vendor.29_arm64_armv8-a_static/gen/sysprop/public/include"

	platformOnProductPath := "libsysprop-platform-on-product/android_product.29_arm64_armv8-a_static/gen/sysprop/public/include"

	vendorInternalPath := "libsysprop-vendor/android_vendor.29_arm64_armv8-a_static/gen/sysprop/include"
	vendorOnProductPath := "libsysprop-vendor-on-product/android_product.29_arm64_armv8-a_static/gen/sysprop/public/include"

	platformClient := result.ModuleForTests("cc-client-platform", coreVariant)
	platformFlags := platformClient.Rule("cc").Args["cFlags"]

	// platform should use platform's internal header
	android.AssertStringDoesContain(t, "flags for platform", platformFlags, platformInternalPath)

	platformStaticClient := result.ModuleForTests("cc-client-platform-static", coreVariant)
	platformStaticFlags := platformStaticClient.Rule("cc").Args["cFlags"]

	// platform-static should use platform's internal header
	android.AssertStringDoesContain(t, "flags for platform-static", platformStaticFlags, platformInternalPath)

	productClient := result.ModuleForTests("cc-client-product", productVariant)
	productFlags := productClient.Rule("cc").Args["cFlags"]

	// Product should use platform's and vendor's public headers
	if !strings.Contains(productFlags, platformOnProductPath) ||
		!strings.Contains(productFlags, vendorOnProductPath) {
		t.Errorf("flags for product must contain %#v and %#v, but was %#v.",
			platformOnProductPath, vendorOnProductPath, productFlags)
	}

	vendorClient := result.ModuleForTests("cc-client-vendor", vendorVariant)
	vendorFlags := vendorClient.Rule("cc").Args["cFlags"]

	// Vendor should use platform's public header and vendor's internal header
	if !strings.Contains(vendorFlags, platformPublicVendorPath) ||
		!strings.Contains(vendorFlags, vendorInternalPath) {
		t.Errorf("flags for vendor must contain %#v and %#v, but was %#v.",
			platformPublicVendorPath, vendorInternalPath, vendorFlags)
	}

	// Java modules linking against system API should use public stub
	javaSystemApiClient := result.ModuleForTests("java-platform", "android_common").Rule("javac")
	syspropPlatformPublic := result.ModuleForTests("sysprop-platform_public", "android_common").Description("for turbine")
	if g, w := javaSystemApiClient.Implicits.Strings(), syspropPlatformPublic.Output.String(); !android.InList(w, g) {
		t.Errorf("system api client should use public stub %q, got %q", w, g)
	}
}

func TestApexAvailabilityIsForwarded(t *testing.T) {
	result := test(t, `
		sysprop_library {
			name: "sysprop-platform",
			apex_available: ["//apex_available:platform"],
			srcs: ["android/sysprop/PlatformProperties.sysprop"],
			api_packages: ["android.sysprop"],
			property_owner: "Platform",
		}
	`)

	expected := []string{"//apex_available:platform"}

	ccModule := result.ModuleForTests("libsysprop-platform", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	propFromCc := ccModule.ApexProperties.Apex_available
	android.AssertDeepEquals(t, "apex_available forwarding to cc module", expected, propFromCc)

	javaModule := result.ModuleForTests("sysprop-platform", "android_common").Module().(*java.Library)
	propFromJava := javaModule.ApexProperties.Apex_available
	android.AssertDeepEquals(t, "apex_available forwarding to java module", expected, propFromJava)

	rustModule := result.ModuleForTests("libsysprop_platform_rust", "android_arm64_armv8-a_rlib_rlib-std").Module().(*rust.Module)
	propFromRust := rustModule.ApexProperties.Apex_available
	android.AssertDeepEquals(t, "apex_available forwarding to rust module", expected, propFromRust)
}

func TestMinSdkVersionIsForwarded(t *testing.T) {
	result := test(t, `
		sysprop_library {
			name: "sysprop-platform",
			srcs: ["android/sysprop/PlatformProperties.sysprop"],
			api_packages: ["android.sysprop"],
			property_owner: "Platform",
			cpp: {
				min_sdk_version: "29",
			},
			java: {
				min_sdk_version: "30",
			},
			rust: {
				min_sdk_version: "29",
			}
		}
	`)

	ccModule := result.ModuleForTests("libsysprop-platform", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	propFromCc := proptools.String(ccModule.Properties.Min_sdk_version)
	android.AssertStringEquals(t, "min_sdk_version forwarding to cc module", "29", propFromCc)

	javaModule := result.ModuleForTests("sysprop-platform", "android_common").Module().(*java.Library)
	propFromJava := javaModule.MinSdkVersionString()
	android.AssertStringEquals(t, "min_sdk_version forwarding to java module", "30", propFromJava)

	rustModule := result.ModuleForTests("libsysprop_platform_rust", "android_arm64_armv8-a_rlib_rlib-std").Module().(*rust.Module)
	propFromRust := proptools.String(rustModule.Properties.Min_sdk_version)
	android.AssertStringEquals(t, "min_sdk_version forwarding to rust module", "29", propFromRust)
}
