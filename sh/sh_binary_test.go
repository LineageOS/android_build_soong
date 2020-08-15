package sh

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"android/soong/android"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_sh_test")
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

func testShBinary(t *testing.T, bp string) (*android.TestContext, android.Config) {
	fs := map[string][]byte{
		"test.sh":            nil,
		"testdata/data1":     nil,
		"testdata/sub/data2": nil,
	}

	config := android.TestArchConfig(buildDir, nil, bp, fs)

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("sh_test", ShTestFactory)
	ctx.RegisterModuleType("sh_test_host", ShTestHostFactory)
	ctx.Register(config)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx, config
}

func TestShTestSubDir(t *testing.T) {
	ctx, config := testShBinary(t, `
		sh_test {
			name: "foo",
			src: "test.sh",
			sub_dir: "foo_test"
		}
	`)

	mod := ctx.ModuleForTests("foo", "android_arm64_armv8-a").Module().(*ShTest)

	entries := android.AndroidMkEntriesForTest(t, config, "", mod)[0]

	expectedPath := "/tmp/target/product/test_device/data/nativetest64/foo_test"
	actualPath := entries.EntryMap["LOCAL_MODULE_PATH"][0]
	if expectedPath != actualPath {
		t.Errorf("Unexpected LOCAL_MODULE_PATH expected: %q, actual: %q", expectedPath, actualPath)
	}
}

func TestShTest(t *testing.T) {
	ctx, config := testShBinary(t, `
		sh_test {
			name: "foo",
			src: "test.sh",
			filename: "test.sh",
			data: [
				"testdata/data1",
				"testdata/sub/data2",
			],
		}
	`)

	mod := ctx.ModuleForTests("foo", "android_arm64_armv8-a").Module().(*ShTest)

	entries := android.AndroidMkEntriesForTest(t, config, "", mod)[0]

	expectedPath := "/tmp/target/product/test_device/data/nativetest64/foo"
	actualPath := entries.EntryMap["LOCAL_MODULE_PATH"][0]
	if expectedPath != actualPath {
		t.Errorf("Unexpected LOCAL_MODULE_PATH expected: %q, actual: %q", expectedPath, actualPath)
	}

	expectedData := []string{":testdata/data1", ":testdata/sub/data2"}
	actualData := entries.EntryMap["LOCAL_TEST_DATA"]
	if !reflect.DeepEqual(expectedData, actualData) {
		t.Errorf("Unexpected test data expected: %q, actual: %q", expectedData, actualData)
	}
}

func TestShTestHost(t *testing.T) {
	ctx, _ := testShBinary(t, `
		sh_test_host {
			name: "foo",
			src: "test.sh",
			filename: "test.sh",
			data: [
				"testdata/data1",
				"testdata/sub/data2",
			],
		}
	`)

	buildOS := android.BuildOs.String()
	mod := ctx.ModuleForTests("foo", buildOS+"_x86_64").Module().(*ShTest)
	if !mod.Host() {
		t.Errorf("host bit is not set for a sh_test_host module.")
	}
}
