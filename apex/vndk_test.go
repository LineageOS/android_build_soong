package apex

import (
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func TestVndkApexForVndkLite(t *testing.T) {
	ctx, _ := testApex(t, `
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
	`+vndkLibrariesTxtFiles("current"), func(fs map[string][]byte, config android.Config) {
		config.TestProductVariables.DeviceVndkVersion = proptools.StringPtr("")
	})
	// VNDK-Lite contains only core variants of VNDK-Sp libraries
	ensureExactContents(t, ctx, "com.android.vndk.current", "android_common_image", []string{
		"lib/libvndksp.so",
		"lib/libc++.so",
		"lib64/libvndksp.so",
		"lib64/libc++.so",
		"etc/llndk.libraries.VER.txt",
		"etc/vndkcore.libraries.VER.txt",
		"etc/vndksp.libraries.VER.txt",
		"etc/vndkprivate.libraries.VER.txt",
		"etc/vndkproduct.libraries.VER.txt",
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
			notice: "custom_notice",
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
		ctx, _ := testApex(t, bp)

		// libfoo doesn't have apex variants
		for _, variant := range ctx.ModuleVariantsForTests("libfoo") {
			ensureNotContains(t, variant, "_myapex")
		}

		// VNDK APEX doesn't create apex variant
		files := getFiles(t, ctx, "com.android.vndk.current", "android_common_image")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.VER_arm_armv7-a-neon_shared/libfoo.so")
	})

	t.Run("VNDK APEX gathers only vendor variants even if product variants are available", func(t *testing.T) {
		ctx, _ := testApex(t, bp, func(fs map[string][]byte, config android.Config) {
			// Now product variant is available
			config.TestProductVariables.ProductVndkVersion = proptools.StringPtr("current")
		})

		files := getFiles(t, ctx, "com.android.vndk.current", "android_common_image")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.VER_arm_armv7-a-neon_shared/libfoo.so")
	})

	t.Run("VNDK APEX supports coverage variants", func(t *testing.T) {
		ctx, _ := testApex(t, bp, func(fs map[string][]byte, config android.Config) {
			config.TestProductVariables.GcovCoverage = proptools.BoolPtr(true)
			config.TestProductVariables.Native_coverage = proptools.BoolPtr(true)
		})

		files := getFiles(t, ctx, "com.android.vndk.current", "android_common_image")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.VER_arm_armv7-a-neon_shared/libfoo.so")

		files = getFiles(t, ctx, "com.android.vndk.current", "android_common_cov_image")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.VER_arm_armv7-a-neon_shared_cov/libfoo.so")
	})
}
