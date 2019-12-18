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

package bpf

import (
	"io/ioutil"
	"os"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "genrule_test")
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

func testConfig(buildDir string, env map[string]string, bp string) android.Config {
	mockFS := map[string][]byte{
		"bpf.c":       nil,
		"BpfTest.cpp": nil,
	}

	return cc.TestConfig(buildDir, android.Android, env, bp, mockFS)
}

func testContext(config android.Config) *android.TestContext {
	ctx := cc.CreateTestContext()
	ctx.RegisterModuleType("bpf", bpfFactory)
	ctx.Register(config)

	return ctx
}

func TestBpfDataDependency(t *testing.T) {
	bp := `
		bpf {
			name: "bpf.o",
			srcs: ["bpf.c"],
		}

		cc_test {
			name: "vts_test_binary_bpf_module",
			srcs: ["BpfTest.cpp"],
			data: [":bpf.o"],
			gtest: false,
		}
	`

	config := testConfig(buildDir, nil, bp)
	ctx := testContext(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if errs == nil {
		_, errs = ctx.PrepareBuildActions(config)
	}
	if errs != nil {
		t.Fatal(errs)
	}

	// We only verify the above BP configuration is processed successfully since the data property
	// value is not available for testing from this package.
	// TODO(jungjw): Add a check for data or move this test to the cc package.
}
