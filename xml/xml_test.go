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
	"io/ioutil"
	"os"
	"testing"

	"android/soong/android"
	"android/soong/etc"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_xml_test")
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

func testXml(t *testing.T, bp string) *android.TestContext {
	fs := map[string][]byte{
		"foo.xml": nil,
		"foo.dtd": nil,
		"bar.xml": nil,
		"bar.xsd": nil,
		"baz.xml": nil,
	}
	config := android.TestArchConfig(buildDir, nil, bp, fs)
	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("prebuilt_etc", etc.PrebuiltEtcFactory)
	ctx.RegisterModuleType("prebuilt_etc_xml", PrebuiltEtcXmlFactory)
	ctx.Register(config)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx
}

func assertEqual(t *testing.T, name, expected, actual string) {
	t.Helper()
	if expected != actual {
		t.Errorf(name+" expected %q != got %q", expected, actual)
	}
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
		prebuilt_etc_xml {
			name: "baz.xml",
			src: "baz.xml",
		}
	`)

	for _, tc := range []struct {
		rule, input, schemaType, schema string
	}{
		{rule: "xmllint-dtd", input: "foo.xml", schemaType: "dtd", schema: "foo.dtd"},
		{rule: "xmllint-xsd", input: "bar.xml", schemaType: "xsd", schema: "bar.xsd"},
		{rule: "xmllint-minimal", input: "baz.xml"},
	} {
		t.Run(tc.schemaType, func(t *testing.T) {
			rule := ctx.ModuleForTests(tc.input, "android_arm64_armv8-a").Rule(tc.rule)
			assertEqual(t, "input", tc.input, rule.Input.String())
			if tc.schemaType != "" {
				assertEqual(t, "schema", tc.schema, rule.Args[tc.schemaType])
			}
		})
	}

	m := ctx.ModuleForTests("foo.xml", "android_arm64_armv8-a").Module().(*prebuiltEtcXml)
	assertEqual(t, "installDir", buildDir+"/target/product/test_device/system/etc", m.InstallDirPath().String())
}
