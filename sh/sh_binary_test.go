package sh

import (
	"io/ioutil"
	"os"
	"path"
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

var shFixtureFactory = android.NewFixtureFactory(
	&buildDir,
	cc.PrepareForTestWithCcBuildComponents,
	PrepareForTestWithShBuildComponents,
	android.FixtureMergeMockFs(android.MockFS{
		"test.sh":            nil,
		"testdata/data1":     nil,
		"testdata/sub/data2": nil,
	}),
)

// testShBinary runs tests using the shFixtureFactory
//
// Do not add any new usages of this, instead use the shFixtureFactory directly as it makes it much
// easier to customize the test behavior.
//
// If it is necessary to customize the behavior of an existing test that uses this then please first
// convert the test to using shFixtureFactory first and then in a following change add the
// appropriate fixture preparers. Keeping the conversion change separate makes it easy to verify
// that it did not change the test behavior unexpectedly.
//
// deprecated
func testShBinary(t *testing.T, bp string) (*android.TestContext, android.Config) {
	result := shFixtureFactory.RunTestWithBp(t, bp)

	return result.TestContext, result.Config
}

func TestShTestSubDir(t *testing.T) {
	ctx, _ := testShBinary(t, `
		sh_test {
			name: "foo",
			src: "test.sh",
			sub_dir: "foo_test"
		}
	`)

	mod := ctx.ModuleForTests("foo", "android_arm64_armv8-a").Module().(*ShTest)

	entries := android.AndroidMkEntriesForTest(t, ctx, mod)[0]

	expectedPath := path.Join(buildDir,
		"../target/product/test_device/data/nativetest64/foo_test")
	actualPath := entries.EntryMap["LOCAL_MODULE_PATH"][0]
	if expectedPath != actualPath {
		t.Errorf("Unexpected LOCAL_MODULE_PATH expected: %q, actual: %q", expectedPath, actualPath)
	}
}

func TestShTest(t *testing.T) {
	ctx, _ := testShBinary(t, `
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

	entries := android.AndroidMkEntriesForTest(t, ctx, mod)[0]

	expectedPath := path.Join(buildDir,
		"../target/product/test_device/data/nativetest64/foo")
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

func TestShTest_dataModules(t *testing.T) {
	ctx, _ := testShBinary(t, `
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
		entries := android.AndroidMkEntriesForTest(t, ctx, mod)[0]
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
	ctx, _ := testShBinary(t, `
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
	entries := android.AndroidMkEntriesForTest(t, ctx, mod)[0]
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
