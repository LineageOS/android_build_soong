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

package xml

import (
	"android/soong/android"
	"io/ioutil"
	"os"
	"testing"
)

func testXml(t *testing.T, bp string) *android.TestContext {
	config, buildDir := setup(t)
	defer teardown(buildDir)
	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("prebuilt_etc", android.ModuleFactoryAdaptor(android.PrebuiltEtcFactory))
	ctx.RegisterModuleType("prebuilt_etc_xml", android.ModuleFactoryAdaptor(PrebuiltEtcXmlFactory))
	ctx.Register()
	mockFiles := map[string][]byte{
		"Android.bp": []byte(bp),
		"foo.xml":    nil,
		"foo.dtd":    nil,
		"bar.xml":    nil,
		"bar.xsd":    nil,
	}
	ctx.MockFileSystem(mockFiles)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx
}

func setup(t *testing.T) (config android.Config, buildDir string) {
	buildDir, err := ioutil.TempDir("", "soong_xml_test")
	if err != nil {
		t.Fatal(err)
	}

	config = android.TestArchConfig(buildDir, nil)

	return
}

func teardown(buildDir string) {
	os.RemoveAll(buildDir)
}

// Minimal test
func TestPrebuiltEtcXml(t *testing.T) {
	ctx := testXml(t, `
		prebuilt_etc_xml {
			name: "foo.xml",
			src: "foo.xml",
			schema: "foo.dtd",
		}
		prebuilt_etc_xml {
			name: "bar.xml",
			src: "bar.xml",
			schema: "bar.xsd",
		}
	`)

	xmllint := ctx.ModuleForTests("foo.xml", "android_common").Rule("xmllint")
	input := xmllint.Input.String()
	if input != "foo.xml" {
		t.Errorf("input expected %q != got %q", "foo.xml", input)
	}
	schema := xmllint.Args["dtd"]
	if schema != "foo.dtd" {
		t.Errorf("dtd expected %q != got %q", "foo.dtdl", schema)
	}
}
