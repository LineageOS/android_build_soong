package android

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

func testShBinary(t *testing.T, bp string) (*TestContext, Config) {
	buildDir, err := ioutil.TempDir("", "soong_sh_binary_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

	config := TestArchConfig(buildDir, nil)

	ctx := NewTestArchContext()
	ctx.RegisterModuleType("sh_test", ModuleFactoryAdaptor(ShTestFactory))
	ctx.RegisterModuleType("sh_test_host", ModuleFactoryAdaptor(ShTestHostFactory))
	ctx.Register()
	mockFiles := map[string][]byte{
		"Android.bp":         []byte(bp),
		"test.sh":            nil,
		"testdata/data1":     nil,
		"testdata/sub/data2": nil,
	}
	ctx.MockFileSystem(mockFiles)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

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

	entries := AndroidMkEntriesForTest(t, config, "", mod)
	expected := []string{":testdata/data1", ":testdata/sub/data2"}
	actual := entries.EntryMap["LOCAL_TEST_DATA"]
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Unexpected test data expected: %q, actual: %q", expected, actual)
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

	buildOS := BuildOs.String()
	mod := ctx.ModuleForTests("foo", buildOS+"_x86_64").Module().(*ShTest)
	if !mod.Host() {
		t.Errorf("host bit is not set for a sh_test_host module.")
	}
}
