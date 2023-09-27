package apex

import (
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func TestVndkApexForVndkLite(t *testing.T) {
	ctx := testApex(t, `
		apex_vndk {
			name: "com.android.vndk.current",
			key: "com.android.vndk.current.key",
			updatable: false,
		}

		apex_key {
			name: "com.android.vndk.current.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libvndk",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "com.android.vndk.current" ],
		}

		cc_library {
			name: "libvndksp",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "com.android.vndk.current" ],
		}
	`+vndkLibrariesTxtFiles("current"),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.DeviceVndkVersion = proptools.StringPtr("")
			variables.KeepVndk = proptools.BoolPtr(true)
		}),
	)
	// VNDK-Lite contains only core variants of VNDK-Sp libraries
	ensureExactContents(t, ctx, "com.android.vndk.current", "android_common", []string{
		"lib/libvndksp.so",
		"lib/libc++.so",
		"lib64/libvndksp.so",
		"lib64/libc++.so",
		"etc/llndk.libraries.29.txt",
		"etc/vndkcore.libraries.29.txt",
		"etc/vndksp.libraries.29.txt",
		"etc/vndkprivate.libraries.29.txt",
		"etc/vndkproduct.libraries.29.txt",
	})
}

func TestVndkApexUsesVendorVariant(t *testing.T) {
	bp := `
		apex_vndk {
			name: "com.android.vndk.current",
			key: "mykey",
			updatable: false,
		}
		apex_key {
			name: "mykey",
		}
		cc_library {
			name: "libfoo",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			system_shared_libs: [],
			stl: "none",
		}
		` + vndkLibrariesTxtFiles("current")

	ensureFileSrc := func(t *testing.T, files []fileInApex, path, src string) {
		t.Helper()
		for _, f := range files {
			if f.path == path {
				ensureContains(t, f.src, src)
				return
			}
		}
		t.Errorf("expected path %q not found", path)
	}

	t.Run("VNDK lib doesn't have an apex variant", func(t *testing.T) {
		ctx := testApex(t, bp)

		// libfoo doesn't have apex variants
		for _, variant := range ctx.ModuleVariantsForTests("libfoo") {
			ensureNotContains(t, variant, "_myapex")
		}

		// VNDK APEX doesn't create apex variant
		files := getFiles(t, ctx, "com.android.vndk.current", "android_common")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.29_arm_armv7-a-neon_shared/libfoo.so")
	})

	t.Run("VNDK APEX gathers only vendor variants even if product variants are available", func(t *testing.T) {
		ctx := testApex(t, bp)

		files := getFiles(t, ctx, "com.android.vndk.current", "android_common")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.29_arm_armv7-a-neon_shared/libfoo.so")
	})

	t.Run("VNDK APEX supports coverage variants", func(t *testing.T) {
		ctx := testApex(t, bp,
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.GcovCoverage = proptools.BoolPtr(true)
				variables.Native_coverage = proptools.BoolPtr(true)
			}),
		)

		files := getFiles(t, ctx, "com.android.vndk.current", "android_common")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.29_arm_armv7-a-neon_shared/libfoo.so")

		files = getFiles(t, ctx, "com.android.vndk.current", "android_common_cov")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.29_arm_armv7-a-neon_shared_cov/libfoo.so")
	})
}
