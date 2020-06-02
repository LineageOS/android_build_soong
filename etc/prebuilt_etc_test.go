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
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"android/soong/android"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_etc_test")
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

func testPrebuiltEtc(t *testing.T, bp string) (*android.TestContext, android.Config) {
	fs := map[string][]byte{
		"foo.conf": nil,
		"bar.conf": nil,
		"baz.conf": nil,
	}

	config := android.TestArchConfig(buildDir, nil, bp, fs)

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("prebuilt_etc", PrebuiltEtcFactory)
	ctx.RegisterModuleType("prebuilt_etc_host", PrebuiltEtcHostFactory)
	ctx.RegisterModuleType("prebuilt_usr_share", PrebuiltUserShareFactory)
	ctx.RegisterModuleType("prebuilt_usr_share_host", PrebuiltUserShareHostFactory)
	ctx.RegisterModuleType("prebuilt_font", PrebuiltFontFactory)
	ctx.RegisterModuleType("prebuilt_firmware", PrebuiltFirmwareFactory)
	ctx.Register(config)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx, config
}

func TestPrebuiltEtcVariants(t *testing.T) {
	ctx, _ := testPrebuiltEtc(t, `
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

	foo_variants := ctx.ModuleVariantsForTests("foo.conf")
	if len(foo_variants) != 1 {
		t.Errorf("expected 1, got %#v", foo_variants)
	}

	bar_variants := ctx.ModuleVariantsForTests("bar.conf")
	if len(bar_variants) != 2 {
		t.Errorf("expected 2, got %#v", bar_variants)
	}

	baz_variants := ctx.ModuleVariantsForTests("baz.conf")
	if len(baz_variants) != 1 {
		t.Errorf("expected 1, got %#v", bar_variants)
	}
}

func TestPrebuiltEtcOutputPath(t *testing.T) {
	ctx, _ := testPrebuiltEtc(t, `
		prebuilt_etc {
			name: "foo.conf",
			src: "foo.conf",
			filename: "foo.installed.conf",
		}
	`)

	p := ctx.ModuleForTests("foo.conf", "android_arm64_armv8-a").Module().(*PrebuiltEtc)
	if p.outputFilePath.Base() != "foo.installed.conf" {
		t.Errorf("expected foo.installed.conf, got %q", p.outputFilePath.Base())
	}
}

func TestPrebuiltEtcGlob(t *testing.T) {
	ctx, _ := testPrebuiltEtc(t, `
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

	p := ctx.ModuleForTests("my_foo", "android_arm64_armv8-a").Module().(*PrebuiltEtc)
	if p.outputFilePath.Base() != "my_foo" {
		t.Errorf("expected my_foo, got %q", p.outputFilePath.Base())
	}

	p = ctx.ModuleForTests("my_bar", "android_arm64_armv8-a").Module().(*PrebuiltEtc)
	if p.outputFilePath.Base() != "bar.conf" {
		t.Errorf("expected bar.conf, got %q", p.outputFilePath.Base())
	}
}

func TestPrebuiltEtcAndroidMk(t *testing.T) {
	ctx, config := testPrebuiltEtc(t, `
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
	}

	mod := ctx.ModuleForTests("foo", "android_arm64_armv8-a").Module().(*PrebuiltEtc)
	entries := android.AndroidMkEntriesForTest(t, config, "", mod)[0]
	for k, expectedValue := range expected {
		if value, ok := entries.EntryMap[k]; ok {
			if !reflect.DeepEqual(value, expectedValue) {
				t.Errorf("Incorrect %s '%s', expected '%s'", k, value, expectedValue)
			}
		} else {
			t.Errorf("No %s defined, saw %q", k, entries.EntryMap)
		}
	}
}

func TestPrebuiltEtcHost(t *testing.T) {
	ctx, _ := testPrebuiltEtc(t, `
		prebuilt_etc_host {
			name: "foo.conf",
			src: "foo.conf",
		}
	`)

	buildOS := android.BuildOs.String()
	p := ctx.ModuleForTests("foo.conf", buildOS+"_common").Module().(*PrebuiltEtc)
	if !p.Host() {
		t.Errorf("host bit is not set for a prebuilt_etc_host module.")
	}
}

func TestPrebuiltUserShareInstallDirPath(t *testing.T) {
	ctx, _ := testPrebuiltEtc(t, `
		prebuilt_usr_share {
			name: "foo.conf",
			src: "foo.conf",
			sub_dir: "bar",
		}
	`)

	p := ctx.ModuleForTests("foo.conf", "android_arm64_armv8-a").Module().(*PrebuiltEtc)
	expected := buildDir + "/target/product/test_device/system/usr/share/bar"
	if p.installDirPath.String() != expected {
		t.Errorf("expected %q, got %q", expected, p.installDirPath.String())
	}
}

func TestPrebuiltUserShareHostInstallDirPath(t *testing.T) {
	ctx, config := testPrebuiltEtc(t, `
		prebuilt_usr_share_host {
			name: "foo.conf",
			src: "foo.conf",
			sub_dir: "bar",
		}
	`)

	buildOS := android.BuildOs.String()
	p := ctx.ModuleForTests("foo.conf", buildOS+"_common").Module().(*PrebuiltEtc)
	expected := filepath.Join(buildDir, "host", config.PrebuiltOS(), "usr", "share", "bar")
	if p.installDirPath.String() != expected {
		t.Errorf("expected %q, got %q", expected, p.installDirPath.String())
	}
}

func TestPrebuiltFontInstallDirPath(t *testing.T) {
	ctx, _ := testPrebuiltEtc(t, `
		prebuilt_font {
			name: "foo.conf",
			src: "foo.conf",
		}
	`)

	p := ctx.ModuleForTests("foo.conf", "android_arm64_armv8-a").Module().(*PrebuiltEtc)
	expected := buildDir + "/target/product/test_device/system/fonts"
	if p.installDirPath.String() != expected {
		t.Errorf("expected %q, got %q", expected, p.installDirPath.String())
	}
}

func TestPrebuiltFirmwareDirPath(t *testing.T) {
	targetPath := buildDir + "/target/product/test_device"
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
			ctx, _ := testPrebuiltEtc(t, tt.config)
			p := ctx.ModuleForTests("foo.conf", "android_arm64_armv8-a").Module().(*PrebuiltEtc)
			if p.installDirPath.String() != tt.expectedPath {
				t.Errorf("expected %q, got %q", tt.expectedPath, p.installDirPath)
			}
		})
	}
}
