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

package etc

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var prepareForPrebuiltEtcTest = android.GroupFixturePreparers(
	android.PrepareForTestWithArchMutator,
	PrepareForTestWithPrebuiltEtc,
	android.FixtureMergeMockFs(android.MockFS{
		"foo.conf": nil,
		"bar.conf": nil,
		"baz.conf": nil,
	}),
)

func TestPrebuiltEtcVariants(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc {
			name: "foo.conf",
			src: "foo.conf",
		}
		prebuilt_etc {
			name: "bar.conf",
			src: "bar.conf",
			recovery_available: true,
			split_all_variants: true,
		}
		prebuilt_etc {
			name: "baz.conf",
			src: "baz.conf",
			recovery: true,
		}
		prebuilt_etc {
			name: "vendor.conf",
			src: "vendor.conf",
			vendor: true,
		}
		prebuilt_etc {
			name: "odm.conf",
			src: "odm.conf",
			device_specific: true,
		}
	`)

	fooVariants := result.ModuleVariantsForTests("foo.conf")
	if g, w := fooVariants, []string{"android_arm64_armv8-a"}; !slices.Equal(g, w) {
		t.Errorf("expected foo.conf variants %q, got %q", w, g)
	}

	barVariants := result.ModuleVariantsForTests("bar.conf")
	if g, w := barVariants, []string{"android_arm64_armv8-a", "android_recovery_arm64_armv8-a"}; !slices.Equal(g, w) {
		t.Errorf("expected bar.conf variants %q, got %q", w, g)
	}

	bazVariants := result.ModuleVariantsForTests("baz.conf")
	if g, w := bazVariants, []string{"android_recovery_arm64_armv8-a"}; !slices.Equal(g, w) {
		t.Errorf("expected baz.conf variants %q, got %q", w, g)
	}

	vendorVariants := result.ModuleVariantsForTests("vendor.conf")
	if g, w := vendorVariants, []string{"android_vendor_arm64_armv8-a"}; !slices.Equal(g, w) {
		t.Errorf("expected vendor.conf variants %q, got %q", w, g)
	}

	odmVariants := result.ModuleVariantsForTests("odm.conf")
	if g, w := odmVariants, []string{"android_vendor_arm64_armv8-a"}; !slices.Equal(g, w) {
		t.Errorf("expected odm.conf variants %q, got %q", w, g)
	}
}

func TestPrebuiltEtcOutputPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc {
			name: "foo.conf",
			src: "foo.conf",
			filename: "foo.installed.conf",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	android.AssertStringEquals(t, "output file path", "foo.installed.conf", p.outputFilePaths[0].Base())
}

func TestPrebuiltEtcGlob(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc {
			name: "my_foo",
			src: "foo.*",
		}
		prebuilt_etc {
			name: "my_bar",
			src: "bar.*",
			filename_from_src: true,
		}
	`)

	p := result.Module("my_foo", "android_arm64_armv8-a").(*PrebuiltEtc)
	android.AssertStringEquals(t, "my_foo output file path", "my_foo", p.outputFilePaths[0].Base())

	p = result.Module("my_bar", "android_arm64_armv8-a").(*PrebuiltEtc)
	android.AssertStringEquals(t, "my_bar output file path", "bar.conf", p.outputFilePaths[0].Base())
}

func TestPrebuiltEtcMultipleSrcs(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc {
			name: "foo",
			srcs: ["*.conf"],
		}
	`)

	p := result.Module("foo", "android_arm64_armv8-a").(*PrebuiltEtc)
	android.AssertStringEquals(t, "output file path", "bar.conf", p.outputFilePaths[0].Base())
	android.AssertStringEquals(t, "output file path", "baz.conf", p.outputFilePaths[1].Base())
	android.AssertStringEquals(t, "output file path", "foo.conf", p.outputFilePaths[2].Base())
}

func TestPrebuiltEtcDsts(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc {
			name: "foo",
			srcs: ["foo.conf", "bar.conf"],
			dsts: ["foodir/foo.conf", "bardir/extradir/different.name"],
		}
	`)

	p := result.Module("foo", "android_arm64_armv8-a").(*PrebuiltEtc)
	android.AssertStringEquals(t, "output file path", "foo.conf", p.outputFilePaths[0].Base())
	android.AssertStringEquals(t, "output file path", "different.name", p.outputFilePaths[1].Base())

	expectedPaths := [...]string{
		"out/target/product/test_device/system/etc/foodir",
		"out/target/product/test_device/system/etc/bardir/extradir",
	}
	android.AssertPathRelativeToTopEquals(t, "install dir", expectedPaths[0], p.installDirPaths[0])
	android.AssertPathRelativeToTopEquals(t, "install dir", expectedPaths[1], p.installDirPaths[1])
}

func TestPrebuiltEtcDstsPlusRelativeInstallPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc {
			name: "foo",
			srcs: ["foo.conf", "bar.conf"],
			dsts: ["foodir/foo.conf", "bardir/extradir/different.name"],
			relative_install_path: "somewhere",
		}
	`)

	p := result.Module("foo", "android_arm64_armv8-a").(*PrebuiltEtc)
	android.AssertStringEquals(t, "output file path", "foo.conf", p.outputFilePaths[0].Base())
	android.AssertStringEquals(t, "output file path", "different.name", p.outputFilePaths[1].Base())

	expectedPaths := [...]string{
		"out/target/product/test_device/system/etc/somewhere/foodir",
		"out/target/product/test_device/system/etc/somewhere/bardir/extradir",
	}
	android.AssertPathRelativeToTopEquals(t, "install dir", expectedPaths[0], p.installDirPaths[0])
	android.AssertPathRelativeToTopEquals(t, "install dir", expectedPaths[1], p.installDirPaths[1])
}

func TestPrebuiltEtcDstsSrcGlob(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc {
			name: "foo",
			srcs: ["*.conf"],
			dsts: ["a.conf", "b.conf", "c.conf"],
		}
	`)

	p := result.Module("foo", "android_arm64_armv8-a").(*PrebuiltEtc)
	android.AssertStringEquals(t, "output file path", "a.conf", p.outputFilePaths[0].Base())
	android.AssertStringEquals(t, "output file path", "b.conf", p.outputFilePaths[1].Base())
	android.AssertStringEquals(t, "output file path", "c.conf", p.outputFilePaths[2].Base())
}

func TestPrebuiltEtcDstsSrcGlobDstsTooShort(t *testing.T) {
	prepareForPrebuiltEtcTest.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern("Must have one entry in dsts per source file")).
		RunTestWithBp(t, `
			prebuilt_etc {
				name: "foo",
				srcs: ["*.conf"],
				dsts: ["a.conf", "b.conf"],
			}
		`)
}

func TestPrebuiltEtcDstsSrcGlobDstsTooLong(t *testing.T) {
	prepareForPrebuiltEtcTest.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern("Must have one entry in dsts per source file")).
		RunTestWithBp(t, `
			prebuilt_etc {
				name: "foo",
				srcs: ["*.conf"],
				dsts: ["a.conf", "b.conf", "c.conf", "d.conf"],
			}
		`)
}

func TestPrebuiltEtcCannotDstsWithSrc(t *testing.T) {
	prepareForPrebuiltEtcTest.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern("dsts is set. Must use srcs")).
		RunTestWithBp(t, `
			prebuilt_etc {
				name: "foo.conf",
				src: "foo.conf",
				dsts: ["a.conf"],
			}
		`)
}

func TestPrebuiltEtcCannotDstsWithFilenameFromSrc(t *testing.T) {
	prepareForPrebuiltEtcTest.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern("dsts is set. Cannot set filename_from_src")).
		RunTestWithBp(t, `
			prebuilt_etc {
				name: "foo.conf",
				srcs: ["foo.conf"],
				dsts: ["a.conf"],
				filename_from_src: true,
			}
		`)
}

func TestPrebuiltEtcAndroidMk(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc {
			name: "foo",
			src: "foo.conf",
			owner: "abc",
			filename_from_src: true,
			required: ["modA", "moduleB"],
			host_required: ["hostModA", "hostModB"],
			target_required: ["targetModA"],
		}
	`)

	expected := map[string][]string{
		"LOCAL_MODULE":                  {"foo"},
		"LOCAL_MODULE_CLASS":            {"ETC"},
		"LOCAL_MODULE_OWNER":            {"abc"},
		"LOCAL_INSTALLED_MODULE_STEM":   {"foo.conf"},
		"LOCAL_REQUIRED_MODULES":        {"modA", "moduleB"},
		"LOCAL_HOST_REQUIRED_MODULES":   {"hostModA", "hostModB"},
		"LOCAL_TARGET_REQUIRED_MODULES": {"targetModA"},
		"LOCAL_SOONG_MODULE_TYPE":       {"prebuilt_etc"},
	}

	mod := result.Module("foo", "android_arm64_armv8-a").(*PrebuiltEtc)
	entries := android.AndroidMkEntriesForTest(t, result.TestContext, mod)[0]
	for k, expectedValue := range expected {
		if value, ok := entries.EntryMap[k]; ok {
			android.AssertDeepEquals(t, k, expectedValue, value)
		} else {
			t.Errorf("No %s defined, saw %q", k, entries.EntryMap)
		}
	}
}

func TestPrebuiltEtcRelativeInstallPathInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc {
			name: "foo.conf",
			src: "foo.conf",
			relative_install_path: "bar",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/target/product/test_device/system/etc/bar"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltEtcCannotSetRelativeInstallPathAndSubDir(t *testing.T) {
	prepareForPrebuiltEtcTest.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern("relative_install_path is set. Cannot set sub_dir")).
		RunTestWithBp(t, `
			prebuilt_etc {
				name: "foo.conf",
				src: "foo.conf",
				sub_dir: "bar",
				relative_install_path: "bar",
			}
		`)
}

func TestPrebuiltEtcHost(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_etc_host {
			name: "foo.conf",
			src: "foo.conf",
		}
	`)

	buildOS := result.Config.BuildOS.String()
	p := result.Module("foo.conf", buildOS+"_common").(*PrebuiltEtc)
	if !p.Host() {
		t.Errorf("host bit is not set for a prebuilt_etc_host module.")
	}
}

func TestPrebuiltEtcAllowMissingDependencies(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForPrebuiltEtcTest,
		android.PrepareForTestDisallowNonExistentPaths,
		android.FixtureModifyConfig(
			func(config android.Config) {
				config.TestProductVariables.Allow_missing_dependencies = proptools.BoolPtr(true)
			}),
	).RunTestWithBp(t, `
		prebuilt_etc {
			name: "foo.conf",
			filename_from_src: true,
			arch: {
				x86: {
					src: "x86.conf",
				},
			},
		}
	`)

	android.AssertStringEquals(t, "expected error rule", "android/soong/android.Error",
		result.ModuleForTests(t, "foo.conf", "android_arm64_armv8-a").Output("foo.conf").Rule.String())
}

func TestPrebuiltRootInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_root {
			name: "foo.conf",
			src: "foo.conf",
			filename: "foo.conf",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/target/product/test_device/system"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltRootInstallDirPathValidate(t *testing.T) {
	prepareForPrebuiltEtcTest.ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern("filename cannot contain separator")).RunTestWithBp(t, `
		prebuilt_root {
			name: "foo.conf",
			src: "foo.conf",
			filename: "foo/bar.conf",
		}
	`)
}

func TestPrebuiltAvbInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_avb {
			name: "foo.conf",
			src: "foo.conf",
			filename: "foo.conf",
			//recovery: true,
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/target/product/test_device/root/avb"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltAvdInstallDirPathValidate(t *testing.T) {
	prepareForPrebuiltEtcTest.ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern("filename cannot contain separator")).RunTestWithBp(t, `
		prebuilt_avb {
			name: "foo.conf",
			src: "foo.conf",
			filename: "foo/bar.conf",
		}
	`)
}

func TestPrebuiltUserShareInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_usr_share {
			name: "foo.conf",
			src: "foo.conf",
			sub_dir: "bar",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/target/product/test_device/system/usr/share/bar"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltUserShareHostInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_usr_share_host {
			name: "foo.conf",
			src: "foo.conf",
			sub_dir: "bar",
		}
	`)

	buildOS := result.Config.BuildOS.String()
	p := result.Module("foo.conf", buildOS+"_common").(*PrebuiltEtc)
	expected := filepath.Join("out/host", result.Config.PrebuiltOS(), "usr", "share", "bar")
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltPrebuiltUserHyphenDataInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
	prebuilt_usr_hyphendata {
			name: "foo.conf",
			src: "foo.conf",
			sub_dir: "bar",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/target/product/test_device/system/usr/hyphen-data/bar"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltPrebuiltUserKeyLayoutInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
	prebuilt_usr_keylayout {
			name: "foo.conf",
			src: "foo.conf",
			sub_dir: "bar",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/target/product/test_device/system/usr/keylayout/bar"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltPrebuiltUserKeyCharsInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
	prebuilt_usr_keychars {
			name: "foo.conf",
			src: "foo.conf",
			sub_dir: "bar",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/target/product/test_device/system/usr/keychars/bar"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltPrebuiltUserIdcInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
	prebuilt_usr_idc {
			name: "foo.conf",
			src: "foo.conf",
			sub_dir: "bar",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/target/product/test_device/system/usr/idc/bar"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltFontInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_font {
			name: "foo.conf",
			src: "foo.conf",
		}
	`)

	p := result.Module("foo.conf", "android_common").(*PrebuiltEtc)
	expected := "out/target/product/test_device/system/fonts"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltOverlayInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_overlay {
			name: "foo.conf",
			src: "foo.conf",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/target/product/test_device/system/overlay"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltFirmwareDirPath(t *testing.T) {
	targetPath := "out/target/product/test_device"
	tests := []struct {
		description  string
		config       string
		variant      string
		expectedPath string
	}{{
		description: "prebuilt: system firmware",
		config: `
			prebuilt_firmware {
				name: "foo.conf",
				src: "foo.conf",
			}`,
		variant:      "android_arm64_armv8-a",
		expectedPath: filepath.Join(targetPath, "system/etc/firmware"),
	}, {
		description: "prebuilt: vendor firmware",
		config: `
			prebuilt_firmware {
				name: "foo.conf",
				src: "foo.conf",
				soc_specific: true,
				sub_dir: "sub_dir",
			}`,
		variant:      "android_vendor_arm64_armv8-a",
		expectedPath: filepath.Join(targetPath, "vendor/firmware/sub_dir"),
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := prepareForPrebuiltEtcTest.RunTestWithBp(t, tt.config)
			p := result.Module("foo.conf", tt.variant).(*PrebuiltEtc)
			android.AssertPathRelativeToTopEquals(t, "install dir", tt.expectedPath, p.installDirPaths[0])
		})
	}
}

func TestPrebuiltDSPDirPath(t *testing.T) {
	targetPath := "out/target/product/test_device"
	tests := []struct {
		description  string
		config       string
		variant      string
		expectedPath string
	}{{
		description: "prebuilt: system dsp",
		config: `
			prebuilt_dsp {
				name: "foo.conf",
				src: "foo.conf",
			}`,
		variant:      "android_arm64_armv8-a",
		expectedPath: filepath.Join(targetPath, "system/etc/dsp"),
	}, {
		description: "prebuilt: vendor dsp",
		config: `
			prebuilt_dsp {
				name: "foo.conf",
				src: "foo.conf",
				soc_specific: true,
				sub_dir: "sub_dir",
			}`,
		variant:      "android_vendor_arm64_armv8-a",
		expectedPath: filepath.Join(targetPath, "vendor/dsp/sub_dir"),
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := prepareForPrebuiltEtcTest.RunTestWithBp(t, tt.config)
			p := result.Module("foo.conf", tt.variant).(*PrebuiltEtc)
			android.AssertPathRelativeToTopEquals(t, "install dir", tt.expectedPath, p.installDirPaths[0])
		})
	}
}

func TestPrebuiltRFSADirPath(t *testing.T) {
	targetPath := "out/target/product/test_device"
	tests := []struct {
		description  string
		config       string
		variant      string
		expectedPath string
	}{{
		description: "prebuilt: system rfsa",
		config: `
			prebuilt_rfsa {
				name: "foo.conf",
				src: "foo.conf",
			}`,
		variant:      "android_arm64_armv8-a",
		expectedPath: filepath.Join(targetPath, "system/lib/rfsa"),
	}, {
		description: "prebuilt: vendor rfsa",
		config: `
			prebuilt_rfsa {
				name: "foo.conf",
				src: "foo.conf",
				soc_specific: true,
				sub_dir: "sub_dir",
			}`,
		variant:      "android_vendor_arm64_armv8-a",
		expectedPath: filepath.Join(targetPath, "vendor/lib/rfsa/sub_dir"),
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := prepareForPrebuiltEtcTest.RunTestWithBp(t, tt.config)
			p := result.Module("foo.conf", tt.variant).(*PrebuiltEtc)
			android.AssertPathRelativeToTopEquals(t, "install dir", tt.expectedPath, p.installDirPaths[0])
		})
	}
}

func TestPrebuiltMediaAutoDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_media {
			name: "foo",
			src: "Alarm_Beep_01.ogg",
			product_specific: true,
			relative_install_path: "alarms"
		}
	`)

	p := result.Module("foo", "android_common").(*PrebuiltEtc)
	expected := "out/target/product/test_device/product/media/alarms"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPaths[0])
}

func TestPrebuiltAddonDDirPath(t *testing.T) {
	targetPath := "out/target/product/test_device"
	tests := []struct {
		description  string
		config       string
		variant      string
		expectedPath string
	}{{
		description: "prebuilt: system addon.d",
		config: `
			prebuilt_addon_d {
				name: "foo",
				src: "foo.conf",
			}`,
		variant:      "android_common",
		expectedPath: filepath.Join(targetPath, "system/addon.d"),
	}, {
		description: "prebuilt: system_ext addon.d",
		config: `
			prebuilt_addon_d {
				name: "foo",
				src: "foo.conf",
				system_ext_specific: true,
			}`,
		variant:      "android_common",
		expectedPath: filepath.Join(targetPath, "system_ext/addon.d"),
	}, {
		description: "prebuilt: vendor addon.d",
		config: `
			prebuilt_addon_d {
				name: "foo",
				src: "foo.conf",
				soc_specific: true,
			}`,
		variant:      "android_vendor_common",
		expectedPath: filepath.Join(targetPath, "vendor/addon.d"),
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := prepareForPrebuiltEtcTest.RunTestWithBp(t, tt.config)
			p := result.Module("foo", tt.variant).(*PrebuiltEtc)
			android.AssertPathRelativeToTopEquals(t, "install dir", tt.expectedPath, p.installDirPaths[0])
		})
	}
}
