// Copyright 2017 Google Inc. All rights reserved.
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

package cc

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"android/soong/android"
)

type dataFile struct {
	path string
	file string
}

var testDataTests = []struct {
	name    string
	modules string
	data    []dataFile
}{
	{
		name: "data files",
		modules: `
			test {
				name: "foo",
				data: [
					"baz",
					"bar/baz",
				],
			}`,
		data: []dataFile{
			{"dir", "baz"},
			{"dir", "bar/baz"},
		},
	},
	{
		name: "filegroup",
		modules: `
			filegroup {
				name: "fg",
				srcs: [
					"baz",
					"bar/baz",
				],
			}

			test {
				name: "foo",
				data: [":fg"],
			}`,
		data: []dataFile{
			{"dir", "baz"},
			{"dir", "bar/baz"},
		},
	},
	{
		name: "relative filegroup",
		modules: `
			filegroup {
				name: "fg",
				srcs: [
					"bar/baz",
				],
				path: "bar",
			}

			test {
				name: "foo",
				data: [":fg"],
			}`,
		data: []dataFile{
			{"dir/bar", "baz"},
		},
	},
	{
		name: "relative filegroup trailing slash",
		modules: `
			filegroup {
				name: "fg",
				srcs: [
					"bar/baz",
				],
				path: "bar/",
			}

			test {
				name: "foo",
				data: [":fg"],
			}`,
		data: []dataFile{
			{"dir/bar", "baz"},
		},
	},
}

func TestDataTests(t *testing.T) {
	buildDir, err := ioutil.TempDir("", "soong_test_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

	for _, test := range testDataTests {
		t.Run(test.name, func(t *testing.T) {
			config := android.TestConfig(buildDir, nil, "", map[string][]byte{
				"dir/Android.bp": []byte(test.modules),
				"dir/baz":        nil,
				"dir/bar/baz":    nil,
			})
			ctx := android.NewTestContext()
			ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
			ctx.RegisterModuleType("test", newTest)
			ctx.Register(config)

			_, errs := ctx.ParseBlueprintsFiles("Blueprints")
			android.FailIfErrored(t, errs)
			_, errs = ctx.PrepareBuildActions(config)
			android.FailIfErrored(t, errs)

			foo := ctx.ModuleForTests("foo", "")

			got := foo.Module().(*testDataTest).data
			if len(got) != len(test.data) {
				t.Errorf("expected %d data files, got %d",
					len(test.data), len(got))
			}

			for i := range got {
				if i >= len(test.data) {
					break
				}

				path := filepath.Join(test.data[i].path, test.data[i].file)
				if test.data[i].file != got[i].Rel() ||
					path != got[i].String() {
					t.Errorf("expected %s:%s got %s:%s",
						path, test.data[i].file,
						got[i].String(), got[i].Rel())
				}
			}
		})
	}
}

type testDataTest struct {
	android.ModuleBase
	data       android.Paths
	Properties struct {
		Data []string `android:"path"`
	}
}

func newTest() android.Module {
	m := &testDataTest{}
	m.AddProperties(&m.Properties)
	android.InitAndroidModule(m)
	return m
}

func (test *testDataTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	test.data = android.PathsForModuleSrc(ctx, test.Properties.Data)
}
