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
	"os"
	"testing"

	"android/soong/android"
	"android/soong/etc"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var emptyFixtureFactory = android.NewFixtureFactory(nil)

func testXml(t *testing.T, bp string) *android.TestResult {
	fs := android.MockFS{
		"foo.xml": nil,
		"foo.dtd": nil,
		"bar.xml": nil,
		"bar.xsd": nil,
		"baz.xml": nil,
	}

	return emptyFixtureFactory.RunTest(t,
		android.PrepareForTestWithArchMutator,
		etc.PrepareForTestWithPrebuiltEtc,
		PreparerForTestWithXmlBuildComponents,
		fs.AddToFixture(),
		android.FixtureWithRootAndroidBp(bp),
	)
}

// Minimal test
func TestPrebuiltEtcXml(t *testing.T) {
	result := testXml(t, `
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
			rule := result.ModuleForTests(tc.input, "android_arm64_armv8-a").Rule(tc.rule)
			android.AssertStringEquals(t, "input", tc.input, rule.Input.String())
			if tc.schemaType != "" {
				android.AssertStringEquals(t, "schema", tc.schema, rule.Args[tc.schemaType])
			}
		})
	}

	m := result.ModuleForTests("foo.xml", "android_arm64_armv8-a").Module().(*prebuiltEtcXml)
	android.AssertPathRelativeToTopEquals(t, "installDir", "out/soong/target/product/test_device/system/etc", m.InstallDirPath())
}
