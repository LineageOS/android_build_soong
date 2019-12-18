package android

import (
	"reflect"
	"testing"
)

func testShBinary(t *testing.T, bp string) (*TestContext, Config) {
	fs := map[string][]byte{
		"test.sh":            nil,
		"testdata/data1":     nil,
		"testdata/sub/data2": nil,
	}

	config := TestArchConfig(buildDir, nil, bp, fs)

	ctx := NewTestArchContext()
	ctx.RegisterModuleType("sh_test", ShTestFactory)
	ctx.RegisterModuleType("sh_test_host", ShTestHostFactory)
	ctx.Register(config)
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

	entries := AndroidMkEntriesForTest(t, config, "", mod)[0]
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
