package apex

import (
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func TestVndkApexUsesVendorVariant(t *testing.T) {
	bp := `
		apex_vndk {
			name: "myapex",
			key: "mykey",
		}
		apex_key {
			name: "mykey",
		}
		cc_library {
			name: "libfoo",
			vendor_available: true,
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
		t.Fail()
	}

	t.Run("VNDK lib doesn't have an apex variant", func(t *testing.T) {
		ctx, _ := testApex(t, bp)

		// libfoo doesn't have apex variants
		for _, variant := range ctx.ModuleVariantsForTests("libfoo") {
			ensureNotContains(t, variant, "_myapex")
		}

		// VNDK APEX doesn't create apex variant
		files := getFiles(t, ctx, "myapex", "android_common_image")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.VER_arm_armv7-a-neon_shared/libfoo.so")
	})

	t.Run("VNDK APEX gathers only vendor variants even if product variants are available", func(t *testing.T) {
		ctx, _ := testApex(t, bp, func(fs map[string][]byte, config android.Config) {
			// Now product variant is available
			config.TestProductVariables.ProductVndkVersion = proptools.StringPtr("current")
		})

		files := getFiles(t, ctx, "myapex", "android_common_image")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.VER_arm_armv7-a-neon_shared/libfoo.so")
	})

	t.Run("VNDK APEX supports coverage variants", func(t *testing.T) {
		ctx, _ := testApex(t, bp+`
			cc_library {
				name: "libprofile-extras",
				vendor_available: true,
				recovery_available: true,
				native_coverage: false,
				system_shared_libs: [],
				stl: "none",
				notice: "custom_notice",
			}
			cc_library {
				name: "libprofile-clang-extras",
				vendor_available: true,
				recovery_available: true,
				native_coverage: false,
				system_shared_libs: [],
				stl: "none",
				notice: "custom_notice",
			}
			cc_library {
				name: "libprofile-extras_ndk",
				vendor_available: true,
				native_coverage: false,
				system_shared_libs: [],
				stl: "none",
				notice: "custom_notice",
			}
			cc_library {
				name: "libprofile-clang-extras_ndk",
				vendor_available: true,
				native_coverage: false,
				system_shared_libs: [],
				stl: "none",
				notice: "custom_notice",
			}
		`, func(fs map[string][]byte, config android.Config) {
			config.TestProductVariables.Native_coverage = proptools.BoolPtr(true)
		})

		files := getFiles(t, ctx, "myapex", "android_common_image")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.VER_arm_armv7-a-neon_shared/libfoo.so")

		files = getFiles(t, ctx, "myapex", "android_common_cov_image")
		ensureFileSrc(t, files, "lib/libfoo.so", "libfoo/android_vendor.VER_arm_armv7-a-neon_shared_cov/libfoo.so")
	})
}
