package sh

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"android/soong/android"
	"android/soong/cc"
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

	cc.RegisterRequiredBuildComponentsForTest(ctx)

	ctx.Register(config)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx, config
}

func TestShTestTestData(t *testing.T) {
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
	expected := []string{":testdata/data1", ":testdata/sub/data2"}
	actual := entries.EntryMap["LOCAL_TEST_DATA"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected test data expected: %q, actual: %q", expected, actual)
	}
}

func TestShTest_dataModules(t *testing.T) {
	ctx, config := testShBinary(t, `
		sh_test {
			name: "foo",
			src: "test.sh",
			host_supported: true,
			data_bins: ["bar"],
			data_libs: ["libbar"],
		}

		cc_binary {
			name: "bar",
			host_supported: true,
			shared_libs: ["libbar"],
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "libbar",
			host_supported: true,
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
		}
	`)

	buildOS := android.BuildOs.String()
	arches := []string{"android_arm64_armv8-a", buildOS + "_x86_64"}
	for _, arch := range arches {
		variant := ctx.ModuleForTests("foo", arch)

		libExt := ".so"
		if arch == "darwin_x86_64" {
			libExt = ".dylib"
		}
		relocated := variant.Output("relocated/lib64/libbar" + libExt)
		expectedInput := filepath.Join(buildDir, ".intermediates/libbar/"+arch+"_shared/libbar"+libExt)
		if relocated.Input.String() != expectedInput {
			t.Errorf("Unexpected relocation input, expected: %q, actual: %q",
				expectedInput, relocated.Input.String())
		}

		mod := variant.Module().(*ShTest)
		entries := android.AndroidMkEntriesForTest(t, config, "", mod)[0]
		expectedData := []string{
			filepath.Join(buildDir, ".intermediates/bar", arch, ":bar"),
			filepath.Join(buildDir, ".intermediates/foo", arch, "relocated/:lib64/libbar"+libExt),
		}
		actualData := entries.EntryMap["LOCAL_TEST_DATA"]
		if !reflect.DeepEqual(expectedData, actualData) {
			t.Errorf("Unexpected test data, expected: %q, actual: %q", expectedData, actualData)
		}
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

func TestShTestHost_dataDeviceModules(t *testing.T) {
	ctx, config := testShBinary(t, `
		sh_test_host {
			name: "foo",
			src: "test.sh",
			data_device_bins: ["bar"],
			data_device_libs: ["libbar"],
		}

		cc_binary {
			name: "bar",
			shared_libs: ["libbar"],
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "libbar",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
		}
	`)

	buildOS := android.BuildOs.String()
	variant := ctx.ModuleForTests("foo", buildOS+"_x86_64")

	relocated := variant.Output("relocated/lib64/libbar.so")
	expectedInput := filepath.Join(buildDir, ".intermediates/libbar/android_arm64_armv8-a_shared/libbar.so")
	if relocated.Input.String() != expectedInput {
		t.Errorf("Unexpected relocation input, expected: %q, actual: %q",
			expectedInput, relocated.Input.String())
	}

	mod := variant.Module().(*ShTest)
	entries := android.AndroidMkEntriesForTest(t, config, "", mod)[0]
	expectedData := []string{
		filepath.Join(buildDir, ".intermediates/bar/android_arm64_armv8-a/:bar"),
		// libbar has been relocated, and so has a variant that matches the host arch.
		filepath.Join(buildDir, ".intermediates/foo/"+buildOS+"_x86_64/relocated/:lib64/libbar.so"),
	}
	actualData := entries.EntryMap["LOCAL_TEST_DATA"]
	if !reflect.DeepEqual(expectedData, actualData) {
		t.Errorf("Unexpected test data, expected: %q, actual: %q", expectedData, actualData)
	}
}
