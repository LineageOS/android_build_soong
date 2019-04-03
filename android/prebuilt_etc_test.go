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

package android

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func testPrebuiltEtc(t *testing.T, bp string) (*TestContext, Config) {
	config, buildDir := setUp(t)
	defer tearDown(buildDir)
	ctx := NewTestArchContext()
	ctx.RegisterModuleType("prebuilt_etc", ModuleFactoryAdaptor(PrebuiltEtcFactory))
	ctx.RegisterModuleType("prebuilt_etc_host", ModuleFactoryAdaptor(PrebuiltEtcHostFactory))
	ctx.RegisterModuleType("prebuilt_usr_share", ModuleFactoryAdaptor(PrebuiltUserShareFactory))
	ctx.RegisterModuleType("prebuilt_usr_share_host", ModuleFactoryAdaptor(PrebuiltUserShareHostFactory))
	ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("prebuilt_etc", prebuiltEtcMutator).Parallel()
	})
	ctx.Register()
	mockFiles := map[string][]byte{
		"Android.bp": []byte(bp),
		"foo.conf":   nil,
		"bar.conf":   nil,
		"baz.conf":   nil,
	}
	ctx.MockFileSystem(mockFiles)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	return ctx, config
}

func setUp(t *testing.T) (config Config, buildDir string) {
	buildDir, err := ioutil.TempDir("", "soong_prebuilt_etc_test")
	if err != nil {
		t.Fatal(err)
	}

	config = TestArchConfig(buildDir, nil)
	return
}

func tearDown(buildDir string) {
	os.RemoveAll(buildDir)
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

	p := ctx.ModuleForTests("foo.conf", "android_arm64_armv8-a_core").Module().(*PrebuiltEtc)
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

	p := ctx.ModuleForTests("my_foo", "android_arm64_armv8-a_core").Module().(*PrebuiltEtc)
	if p.outputFilePath.Base() != "my_foo" {
		t.Errorf("expected my_foo, got %q", p.outputFilePath.Base())
	}

	p = ctx.ModuleForTests("my_bar", "android_arm64_armv8-a_core").Module().(*PrebuiltEtc)
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
		}
	`)

	expected := map[string][]string{
		"LOCAL_MODULE":                  {"foo"},
		"LOCAL_MODULE_CLASS":            {"ETC"},
		"LOCAL_MODULE_OWNER":            {"abc"},
		"LOCAL_INSTALLED_MODULE_STEM":   {"foo.conf"},
		"LOCAL_REQUIRED_MODULES":        {"modA", "moduleB"},
	}

	mod := ctx.ModuleForTests("foo", "android_arm64_armv8-a_core").Module().(*PrebuiltEtc)
	entries := AndroidMkEntriesForTest(t, config, "", mod)
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

	buildOS := BuildOs.String()
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

	p := ctx.ModuleForTests("foo.conf", "android_arm64_armv8-a_core").Module().(*PrebuiltEtc)
	expected := "target/product/test_device/system/usr/share/bar"
	if p.installDirPath.RelPathString() != expected {
		t.Errorf("expected %q, got %q", expected, p.installDirPath.RelPathString())
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

	buildOS := BuildOs.String()
	p := ctx.ModuleForTests("foo.conf", buildOS+"_common").Module().(*PrebuiltEtc)
	expected := filepath.Join("host", config.PrebuiltOS(), "usr", "share", "bar")
	if p.installDirPath.RelPathString() != expected {
		t.Errorf("expected %q, got %q", expected, p.installDirPath.RelPathString())
	}
}
