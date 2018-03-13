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
	"sort"
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

	compiledResourceOutputs := compile.Outputs.Strings()
	sort.Strings(compiledResourceOutputs)

	expectedLinkImplicits = append(expectedLinkImplicits, compiledResourceOutputs...)

	list := foo.Output("aapt2/res.list")
	expectedLinkImplicits = append(expectedLinkImplicits, list.Output.String())

	// Check that the link rule uses
	res := ctx.ModuleForTests("foo", "android_common").Output("package-res.apk")
	if !reflect.DeepEqual(expectedLinkImplicits, res.Implicits.Strings()) {
		t.Errorf("expected aapt2 link implicits expected:\n  %#v\n got:\n  %#v",
			expectedLinkImplicits, res.Implicits.Strings())
	}
}

var testEnforceRROTests = []struct {
	name                       string
	enforceRROTargets          []string
	enforceRROExcludedOverlays []string
	fooOverlayFiles            []string
	fooRRODirs                 []string
	barOverlayFiles            []string
	barRRODirs                 []string
}{
	{
		name:                       "no RRO",
		enforceRROTargets:          nil,
		enforceRROExcludedOverlays: nil,
		fooOverlayFiles: []string{
			"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
			"device/vendor/blah/overlay/foo/res/values/strings.xml",
		},
		fooRRODirs: nil,
		barOverlayFiles: []string{
			"device/vendor/blah/static_overlay/bar/res/values/strings.xml",
			"device/vendor/blah/overlay/bar/res/values/strings.xml",
		},
		barRRODirs: nil,
	},
	{
		name:                       "enforce RRO on foo",
		enforceRROTargets:          []string{"foo"},
		enforceRROExcludedOverlays: []string{"device/vendor/blah/static_overlay"},
		fooOverlayFiles: []string{
			"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
		},
		fooRRODirs: []string{
			"device/vendor/blah/overlay/foo/res",
		},
		barOverlayFiles: []string{
			"device/vendor/blah/static_overlay/bar/res/values/strings.xml",
			"device/vendor/blah/overlay/bar/res/values/strings.xml",
		},
		barRRODirs: nil,
	},
	{
		name:                       "enforce RRO on all",
		enforceRROTargets:          []string{"*"},
		enforceRROExcludedOverlays: []string{"device/vendor/blah/static_overlay"},
		fooOverlayFiles: []string{
			"device/vendor/blah/static_overlay/foo/res/values/strings.xml",
		},
		fooRRODirs: []string{
			"device/vendor/blah/overlay/foo/res",
		},
		barOverlayFiles: []string{
			"device/vendor/blah/static_overlay/bar/res/values/strings.xml",
		},
		barRRODirs: []string{
			"device/vendor/blah/overlay/bar/res",
		},
	},
}

func TestEnforceRRO(t *testing.T) {
	resourceOverlays := []string{
		"device/vendor/blah/overlay",
		"device/vendor/blah/overlay2",
		"device/vendor/blah/static_overlay",
	}

	fs := map[string][]byte{
		"foo/res/res/values/strings.xml":                               nil,
		"bar/res/res/values/strings.xml":                               nil,
		"device/vendor/blah/overlay/foo/res/values/strings.xml":        nil,
		"device/vendor/blah/overlay/bar/res/values/strings.xml":        nil,
		"device/vendor/blah/static_overlay/foo/res/values/strings.xml": nil,
		"device/vendor/blah/static_overlay/bar/res/values/strings.xml": nil,
		"device/vendor/blah/overlay2/res/values/strings.xml":           nil,
	}

	bp := `
			android_app {
				name: "foo",
				resource_dirs: ["foo/res"],
			}

			android_app {
				name: "bar",
				resource_dirs: ["bar/res"],
			}
		`

	for _, testCase := range testEnforceRROTests {
		t.Run(testCase.name, func(t *testing.T) {
			config := testConfig(nil)
			config.TestProductVariables.ResourceOverlays = &resourceOverlays
			if testCase.enforceRROTargets != nil {
				config.TestProductVariables.EnforceRROTargets = &testCase.enforceRROTargets
			}
			if testCase.enforceRROExcludedOverlays != nil {
				config.TestProductVariables.EnforceRROExcludedOverlays = &testCase.enforceRROExcludedOverlays
			}

			ctx := testAppContext(config, bp, fs)
			run(t, ctx, config)

			getOverlays := func(moduleName string) ([]string, []string) {
				module := ctx.ModuleForTests(moduleName, "android_common")
				overlayCompiledPaths := module.Output("aapt2/overlay.list").Inputs.Strings()

				var overlayFiles []string
				for _, o := range overlayCompiledPaths {
					overlayFiles = append(overlayFiles, module.Output(o).Inputs.Strings()...)
				}

				rroDirs := module.Module().(*AndroidApp).rroDirs.Strings()

				return overlayFiles, rroDirs
			}

			fooOverlayFiles, fooRRODirs := getOverlays("foo")
			barOverlayFiles, barRRODirs := getOverlays("bar")

			if !reflect.DeepEqual(fooOverlayFiles, testCase.fooOverlayFiles) {
				t.Errorf("expected foo overlay files:\n  %#v\n got:\n  %#v",
					testCase.fooOverlayFiles, fooOverlayFiles)
			}
			if !reflect.DeepEqual(fooRRODirs, testCase.fooRRODirs) {
				t.Errorf("expected foo rroDirs:  %#v\n got:\n  %#v",
					testCase.fooRRODirs, fooRRODirs)
			}

			if !reflect.DeepEqual(barOverlayFiles, testCase.barOverlayFiles) {
				t.Errorf("expected bar overlay files:\n  %#v\n got:\n  %#v",
					testCase.barOverlayFiles, barOverlayFiles)
			}
			if !reflect.DeepEqual(barRRODirs, testCase.barRRODirs) {
				t.Errorf("expected bar rroDirs:  %#v\n got:\n  %#v",
					testCase.barRRODirs, barRRODirs)
			}

		})
	}
}
