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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/snapshot"
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

var prepareForPrebuiltEtcSnapshotTest = android.GroupFixturePreparers(
	prepareForPrebuiltEtcTest,
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		snapshot.VendorSnapshotImageSingleton.Init(ctx)
		snapshot.RecoverySnapshotImageSingleton.Init(ctx)
	}),
	android.FixtureModifyConfig(func(config android.Config) {
		config.TestProductVariables.DeviceVndkVersion = proptools.StringPtr("current")
		config.TestProductVariables.RecoverySnapshotVersion = proptools.StringPtr("current")
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
		}
		prebuilt_etc {
			name: "baz.conf",
			src: "baz.conf",
			recovery: true,
		}
	`)

	foo_variants := result.ModuleVariantsForTests("foo.conf")
	if len(foo_variants) != 1 {
		t.Errorf("expected 1, got %#v", foo_variants)
	}

	bar_variants := result.ModuleVariantsForTests("bar.conf")
	if len(bar_variants) != 2 {
		t.Errorf("expected 2, got %#v", bar_variants)
	}

	baz_variants := result.ModuleVariantsForTests("baz.conf")
	if len(baz_variants) != 1 {
		t.Errorf("expected 1, got %#v", baz_variants)
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
	expected := "out/soong/target/product/test_device/system/etc/bar"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPath)
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
		result.ModuleForTests("foo.conf", "android_arm64_armv8-a").Output("foo.conf").Rule.String())
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
	expected := "out/soong/target/product/test_device/system"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPath)
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

func TestPrebuiltUserShareInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_usr_share {
			name: "foo.conf",
			src: "foo.conf",
			sub_dir: "bar",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/soong/target/product/test_device/system/usr/share/bar"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPath)
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
	expected := filepath.Join("out/soong/host", result.Config.PrebuiltOS(), "usr", "share", "bar")
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPath)
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
	expected := "out/soong/target/product/test_device/system/usr/hyphen-data/bar"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPath)
}

func TestPrebuiltFontInstallDirPath(t *testing.T) {
	result := prepareForPrebuiltEtcTest.RunTestWithBp(t, `
		prebuilt_font {
			name: "foo.conf",
			src: "foo.conf",
		}
	`)

	p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
	expected := "out/soong/target/product/test_device/system/fonts"
	android.AssertPathRelativeToTopEquals(t, "install dir", expected, p.installDirPath)
}

func TestPrebuiltFirmwareDirPath(t *testing.T) {
	targetPath := "out/soong/target/product/test_device"
	tests := []struct {
		description  string
		config       string
		expectedPath string
	}{{
		description: "prebuilt: system firmware",
		config: `
			prebuilt_firmware {
				name: "foo.conf",
				src: "foo.conf",
			}`,
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
		expectedPath: filepath.Join(targetPath, "vendor/firmware/sub_dir"),
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := prepareForPrebuiltEtcTest.RunTestWithBp(t, tt.config)
			p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
			android.AssertPathRelativeToTopEquals(t, "install dir", tt.expectedPath, p.installDirPath)
		})
	}
}

func TestPrebuiltDSPDirPath(t *testing.T) {
	targetPath := "out/soong/target/product/test_device"
	tests := []struct {
		description  string
		config       string
		expectedPath string
	}{{
		description: "prebuilt: system dsp",
		config: `
			prebuilt_dsp {
				name: "foo.conf",
				src: "foo.conf",
			}`,
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
		expectedPath: filepath.Join(targetPath, "vendor/dsp/sub_dir"),
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := prepareForPrebuiltEtcTest.RunTestWithBp(t, tt.config)
			p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
			android.AssertPathRelativeToTopEquals(t, "install dir", tt.expectedPath, p.installDirPath)
		})
	}
}

func TestPrebuiltRFSADirPath(t *testing.T) {
	targetPath := "out/soong/target/product/test_device"
	tests := []struct {
		description  string
		config       string
		expectedPath string
	}{{
		description: "prebuilt: system rfsa",
		config: `
			prebuilt_rfsa {
				name: "foo.conf",
				src: "foo.conf",
			}`,
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
		expectedPath: filepath.Join(targetPath, "vendor/lib/rfsa/sub_dir"),
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := prepareForPrebuiltEtcTest.RunTestWithBp(t, tt.config)
			p := result.Module("foo.conf", "android_arm64_armv8-a").(*PrebuiltEtc)
			android.AssertPathRelativeToTopEquals(t, "install dir", tt.expectedPath, p.installDirPath)
		})
	}
}

func checkIfSnapshotTaken(t *testing.T, result *android.TestResult, image string, moduleName string) {
	checkIfSnapshotExistAsExpected(t, result, image, moduleName, true)
}

func checkIfSnapshotNotTaken(t *testing.T, result *android.TestResult, image string, moduleName string) {
	checkIfSnapshotExistAsExpected(t, result, image, moduleName, false)
}

func checkIfSnapshotExistAsExpected(t *testing.T, result *android.TestResult, image string, moduleName string, expectToExist bool) {
	snapshotSingleton := result.SingletonForTests(image + "-snapshot")
	archType := "arm64"
	archVariant := "armv8-a"
	archDir := fmt.Sprintf("arch-%s", archType)

	snapshotDir := fmt.Sprintf("%s-snapshot", image)
	snapshotVariantPath := filepath.Join(snapshotDir, archType)
	outputDir := filepath.Join(snapshotVariantPath, archDir, "etc")
	imageVariant := ""
	if image == "recovery" {
		imageVariant = "recovery_"
	}
	mod := result.ModuleForTests(moduleName, fmt.Sprintf("android_%s%s_%s", imageVariant, archType, archVariant))
	outputFiles := mod.OutputFiles(t, "")
	if len(outputFiles) != 1 {
		t.Errorf("%q must have single output\n", moduleName)
		return
	}
	snapshotPath := filepath.Join(outputDir, moduleName)

	if expectToExist {
		out := snapshotSingleton.Output(snapshotPath)

		if out.Input.String() != outputFiles[0].String() {
			t.Errorf("The input of snapshot %q must be %q, but %q", "prebuilt_vendor", out.Input.String(), outputFiles[0])
		}

		snapshotJsonPath := snapshotPath + ".json"

		if snapshotSingleton.MaybeOutput(snapshotJsonPath).Rule == nil {
			t.Errorf("%q expected but not found", snapshotJsonPath)
		}
	} else {
		out := snapshotSingleton.MaybeOutput(snapshotPath)
		if out.Rule != nil {
			t.Errorf("There must be no rule for module %q output file %q", moduleName, outputFiles[0])
		}
	}
}

func TestPrebuiltTakeSnapshot(t *testing.T) {
	var testBp = `
	prebuilt_etc {
		name: "prebuilt_vendor",
		src: "foo.conf",
		vendor: true,
	}

	prebuilt_etc {
		name: "prebuilt_vendor_indirect",
		src: "foo.conf",
		vendor: true,
	}

	prebuilt_etc {
		name: "prebuilt_recovery",
		src: "bar.conf",
		recovery: true,
	}

	prebuilt_etc {
		name: "prebuilt_recovery_indirect",
		src: "bar.conf",
		recovery: true,
	}
	`

	t.Run("prebuilt: vendor and recovery snapshot", func(t *testing.T) {
		result := prepareForPrebuiltEtcSnapshotTest.RunTestWithBp(t, testBp)

		checkIfSnapshotTaken(t, result, "vendor", "prebuilt_vendor")
		checkIfSnapshotTaken(t, result, "vendor", "prebuilt_vendor_indirect")
		checkIfSnapshotTaken(t, result, "recovery", "prebuilt_recovery")
		checkIfSnapshotTaken(t, result, "recovery", "prebuilt_recovery_indirect")
	})

	t.Run("prebuilt: directed snapshot", func(t *testing.T) {
		prepareForPrebuiltEtcDirectedSnapshotTest := android.GroupFixturePreparers(
			prepareForPrebuiltEtcSnapshotTest,
			android.FixtureModifyConfig(func(config android.Config) {
				config.TestProductVariables.DirectedVendorSnapshot = true
				config.TestProductVariables.VendorSnapshotModules = make(map[string]bool)
				config.TestProductVariables.VendorSnapshotModules["prebuilt_vendor"] = true
				config.TestProductVariables.DirectedRecoverySnapshot = true
				config.TestProductVariables.RecoverySnapshotModules = make(map[string]bool)
				config.TestProductVariables.RecoverySnapshotModules["prebuilt_recovery"] = true
			}),
		)

		result := prepareForPrebuiltEtcDirectedSnapshotTest.RunTestWithBp(t, testBp)

		checkIfSnapshotTaken(t, result, "vendor", "prebuilt_vendor")
		checkIfSnapshotNotTaken(t, result, "vendor", "prebuilt_vendor_indirect")
		checkIfSnapshotTaken(t, result, "recovery", "prebuilt_recovery")
		checkIfSnapshotNotTaken(t, result, "recovery", "prebuilt_recovery_indirect")
	})
}
