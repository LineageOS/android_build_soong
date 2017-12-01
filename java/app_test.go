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

package java

import (
	"android/soong/android"
	"reflect"
	"testing"
)

var (
	resourceFiles = []string{
		"res/layout/layout.xml",
		"res/values/strings.xml",
		"res/values-en-rUS/strings.xml",
	}

	compiledResourceFiles = []string{
		"aapt2/res/layout_layout.xml.flat",
		"aapt2/res/values_strings.arsc.flat",
		"aapt2/res/values-en-rUS_strings.arsc.flat",
	}
)

func testAppContext(config android.Config, bp string, fs map[string][]byte) *android.TestContext {
	appFS := map[string][]byte{}
	for k, v := range fs {
		appFS[k] = v
	}

	for _, file := range resourceFiles {
		appFS[file] = nil
	}

	return testContext(config, bp, appFS)
}

func testApp(t *testing.T, bp string) *android.TestContext {
	config := testConfig(nil)

	ctx := testAppContext(config, bp, nil)

	run(t, ctx, config)

	return ctx
}

func TestApp(t *testing.T) {
	ctx := testApp(t, `
		android_app {
			name: "foo",
			srcs: ["a.java"],
		}
	`)

	foo := ctx.ModuleForTests("foo", "android_common")

	expectedLinkImplicits := []string{"AndroidManifest.xml"}

	frameworkRes := ctx.ModuleForTests("framework-res", "android_common")
	expectedLinkImplicits = append(expectedLinkImplicits,
		frameworkRes.Output("package-res.apk").Output.String())

	// Test the mapping from input files to compiled output file names
	compile := foo.Output(compiledResourceFiles[0])
	if !reflect.DeepEqual(resourceFiles, compile.Inputs.Strings()) {
		t.Errorf("expected aapt2 compile inputs expected:\n  %#v\n got:\n  %#v",
			resourceFiles, compile.Inputs.Strings())
	}
	expectedLinkImplicits = append(expectedLinkImplicits, compile.Outputs.Strings()...)

	list := foo.Output("aapt2/res.list")
	expectedLinkImplicits = append(expectedLinkImplicits, list.Output.String())

	// Check that the link rule uses
	res := ctx.ModuleForTests("foo", "android_common").Output("package-res.apk")
	if !reflect.DeepEqual(expectedLinkImplicits, res.Implicits.Strings()) {
		t.Errorf("expected aapt2 link implicits expected:\n  %#v\n got:\n  %#v",
			expectedLinkImplicits, res.Implicits.Strings())
	}
}
