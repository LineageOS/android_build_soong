package apex

import (
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func TestVndkApexForVndkLite(t *testing.T) {
	ctx, _ := testApex(t, `
		apex_vndk {
			name: "myapex",
			key: "myapex.key",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libvndk",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libvndksp",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}
	`+vndkLibrariesTxtFiles("current"), func(fs map[string][]byte, config android.Config) {
		config.TestProductVariables.DeviceVndkVersion = proptools.StringPtr("")
	})
	// VNDK-Lite contains only core variants of VNDK-Sp libraries
	ensureExactContents(t, ctx, "myapex", "android_common_image", []string{
		"lib/libvndksp.so",
		"lib/libc++.so",
		"lib64/libvndksp.so",
		"lib64/libc++.so",
		"etc/llndk.libraries.VER.txt",
		"etc/vndkcore.libraries.VER.txt",
		"etc/vndksp.libraries.VER.txt",
		"etc/vndkprivate.libraries.VER.txt",
	})
}

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
