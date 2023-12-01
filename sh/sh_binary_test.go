package sh

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var prepareForShTest = android.GroupFixturePreparers(
	cc.PrepareForTestWithCcBuildComponents,
	java.PrepareForTestWithJavaDefaultModules,
	PrepareForTestWithShBuildComponents,
	android.FixtureMergeMockFs(android.MockFS{
		"test.sh":            nil,
		"testdata/data1":     nil,
		"testdata/sub/data2": nil,
	}),
)

// testShBinary runs tests using the prepareForShTest
//
// Do not add any new usages of this, instead use the prepareForShTest directly as it makes it much
// easier to customize the test behavior.
//
// If it is necessary to customize the behavior of an existing test that uses this then please first
// convert the test to using prepareForShTest first and then in a following change add the
// appropriate fixture preparers. Keeping the conversion change separate makes it easy to verify
// that it did not change the test behavior unexpectedly.
//
// deprecated
func testShBinary(t *testing.T, bp string) (*android.TestContext, android.Config) {
	bp = bp + cc.GatherRequiredDepsForTest(android.Android)

	result := prepareForShTest.RunTestWithBp(t, bp)

	return result.TestContext, result.Config
}

func TestShTestSubDir(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForShTest,
		android.FixtureModifyConfig(android.SetKatiEnabledForTests),
	).RunTestWithBp(t, `
		sh_test {
			name: "foo",
			src: "test.sh",
			sub_dir: "foo_test"
		}
	`)

	mod := result.ModuleForTests("foo", "android_arm64_armv8-a").Module().(*ShTest)

	entries := android.AndroidMkEntriesForTest(t, result.TestContext, mod)[0]

	expectedPath := "out/target/product/test_device/data/nativetest64/foo_test"
	actualPath := entries.EntryMap["LOCAL_MODULE_PATH"][0]
	android.AssertStringPathRelativeToTopEquals(t, "LOCAL_MODULE_PATH[0]", result.Config, expectedPath, actualPath)
}

func TestShTest(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForShTest,
		android.FixtureModifyConfig(android.SetKatiEnabledForTests),
	).RunTestWithBp(t, `
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

	mod := result.ModuleForTests("foo", "android_arm64_armv8-a").Module().(*ShTest)

	entries := android.AndroidMkEntriesForTest(t, result.TestContext, mod)[0]

	expectedPath := "out/target/product/test_device/data/nativetest64/foo"
	actualPath := entries.EntryMap["LOCAL_MODULE_PATH"][0]
	android.AssertStringPathRelativeToTopEquals(t, "LOCAL_MODULE_PATH[0]", result.Config, expectedPath, actualPath)

	expectedData := []string{":testdata/data1", ":testdata/sub/data2"}
	actualData := entries.EntryMap["LOCAL_TEST_DATA"]
	android.AssertDeepEquals(t, "LOCAL_TEST_DATA", expectedData, actualData)
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

	buildOS := config.BuildOS.String()
	arches := []string{"android_arm64_armv8-a", buildOS + "_x86_64"}
	for _, arch := range arches {
		variant := ctx.ModuleForTests("foo", arch)

		libExt := ".so"
		if arch == "darwin_x86_64" {
			libExt = ".dylib"
		}
		relocated := variant.Output(filepath.Join("out/soong/.intermediates/foo", arch, "relocated/lib64/libbar"+libExt))
		expectedInput := "out/soong/.intermediates/libbar/" + arch + "_shared/libbar" + libExt
		android.AssertPathRelativeToTopEquals(t, "relocation input", expectedInput, relocated.Input)

		mod := variant.Module().(*ShTest)
		entries := android.AndroidMkEntriesForTest(t, ctx, mod)[0]
		expectedData := []string{
			filepath.Join("out/soong/.intermediates/bar", arch, ":bar"),
			filepath.Join("out/soong/.intermediates/foo", arch, "relocated/:lib64/libbar"+libExt),
		}
		actualData := entries.EntryMap["LOCAL_TEST_DATA"]
		android.AssertStringPathsRelativeToTopEquals(t, "LOCAL_TEST_DATA", config, expectedData, actualData)
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
			test_options: {
				unit_test: true,
			},
		}
	`)

	buildOS := ctx.Config().BuildOS.String()
	mod := ctx.ModuleForTests("foo", buildOS+"_x86_64").Module().(*ShTest)
	if !mod.Host() {
		t.Errorf("host bit is not set for a sh_test_host module.")
	}
	entries := android.AndroidMkEntriesForTest(t, ctx, mod)[0]
	actualData, _ := strconv.ParseBool(entries.EntryMap["LOCAL_IS_UNIT_TEST"][0])
	android.AssertBoolEquals(t, "LOCAL_IS_UNIT_TEST", true, actualData)
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

	buildOS := config.BuildOS.String()
	variant := buildOS + "_x86_64"
	foo := ctx.ModuleForTests("foo", variant)

	relocated := foo.Output(filepath.Join("out/soong/.intermediates/foo", variant, "relocated/lib64/libbar.so"))
	expectedInput := "out/soong/.intermediates/libbar/android_arm64_armv8-a_shared/libbar.so"
	android.AssertPathRelativeToTopEquals(t, "relocation input", expectedInput, relocated.Input)

	mod := foo.Module().(*ShTest)
	entries := android.AndroidMkEntriesForTest(t, ctx, mod)[0]
	expectedData := []string{
		"out/soong/.intermediates/bar/android_arm64_armv8-a/:bar",
		// libbar has been relocated, and so has a variant that matches the host arch.
		"out/soong/.intermediates/foo/" + variant + "/relocated/:lib64/libbar.so",
	}
	actualData := entries.EntryMap["LOCAL_TEST_DATA"]
	android.AssertStringPathsRelativeToTopEquals(t, "LOCAL_TEST_DATA", config, expectedData, actualData)
}

func TestShTestHost_dataDeviceModulesAutogenTradefedConfig(t *testing.T) {
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

	buildOS := config.BuildOS.String()
	fooModule := ctx.ModuleForTests("foo", buildOS+"_x86_64")

	expectedBinAutogenConfig := `<option name="push-file" key="bar" value="/data/local/tests/unrestricted/foo/bar" />`
	autogen := fooModule.Rule("autogen")
	if !strings.Contains(autogen.Args["extraConfigs"], expectedBinAutogenConfig) {
		t.Errorf("foo extraConfings %v does not contain %q", autogen.Args["extraConfigs"], expectedBinAutogenConfig)
	}
}

func TestShTestHost_javaData(t *testing.T) {
	ctx, config := testShBinary(t, `
		sh_test_host {
			name: "foo",
			src: "test.sh",
			filename: "test.sh",
			data: [
				"testdata/data1",
				"testdata/sub/data2",
			],
			java_data: [
				"javalib",
			],
		}

		java_library_host {
			name: "javalib",
			srcs: [],
		}
	`)
	buildOS := ctx.Config().BuildOS.String()
	mod := ctx.ModuleForTests("foo", buildOS+"_x86_64").Module().(*ShTest)
	if !mod.Host() {
		t.Errorf("host bit is not set for a sh_test_host module.")
	}
	expectedData := []string{
		":testdata/data1",
		":testdata/sub/data2",
		"out/soong/.intermediates/javalib/" + buildOS + "_common/combined/:javalib.jar",
	}

	entries := android.AndroidMkEntriesForTest(t, ctx, mod)[0]
	actualData := entries.EntryMap["LOCAL_TEST_DATA"]
	android.AssertStringPathsRelativeToTopEquals(t, "LOCAL_TEST_DATA", config, expectedData, actualData)
}
