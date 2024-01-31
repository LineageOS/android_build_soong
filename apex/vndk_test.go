package apex

import (
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

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
