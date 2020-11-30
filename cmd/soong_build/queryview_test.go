// Copyright 2020 Google Inc. All rights reserved.
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

package main

import (
	"android/soong/android"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/google/blueprint/bootstrap/bpdoc"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "bazel_queryview_test")
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

type customModule struct {
	android.ModuleBase
}

// OutputFiles is needed because some instances of this module use dist with a
// tag property which requires the module implements OutputFileProducer.
func (m *customModule) OutputFiles(tag string) (android.Paths, error) {
	return android.PathsForTesting("path" + tag), nil
}

func (m *customModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// nothing for now.
}

func customModuleFactory() android.Module {
	module := &customModule{}
	android.InitAndroidModule(module)
	return module
}

func TestGenerateBazelQueryViewFromBlueprint(t *testing.T) {
	testCases := []struct {
		bp                  string
		expectedBazelTarget string
	}{
		{
			bp: `custom {
	name: "foo",
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    module_name = "foo",
    module_type = "custom",
    module_variant = "",
    module_deps = [
    ],
)`,
		},
		{
			bp: `custom {
	name: "foo",
	ramdisk: true,
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    module_name = "foo",
    module_type = "custom",
    module_variant = "",
    module_deps = [
    ],
    ramdisk = True,
)`,
		},
		{
			bp: `custom {
	name: "foo",
	owner: "a_string_with\"quotes\"_and_\\backslashes\\\\",
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    module_name = "foo",
    module_type = "custom",
    module_variant = "",
    module_deps = [
    ],
    owner = "a_string_with\"quotes\"_and_\\backslashes\\\\",
)`,
		},
		{
			bp: `custom {
	name: "foo",
	required: ["bar"],
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    module_name = "foo",
    module_type = "custom",
    module_variant = "",
    module_deps = [
    ],
    required = [
        "bar",
    ],
)`,
		},
		{
			bp: `custom {
	name: "foo",
	target_required: ["qux", "bazqux"],
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    module_name = "foo",
    module_type = "custom",
    module_variant = "",
    module_deps = [
    ],
    target_required = [
        "qux",
        "bazqux",
    ],
)`,
		},
		{
			bp: `custom {
	name: "foo",
	dist: {
		targets: ["goal_foo"],
		tag: ".foo",
	},
	dists: [
		{
			targets: ["goal_bar"],
			tag: ".bar",
		},
	],
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    module_name = "foo",
    module_type = "custom",
    module_variant = "",
    module_deps = [
    ],
    dist = {
        "tag": ".foo",
        "targets": [
            "goal_foo",
        ],
    },
    dists = [
        {
            "tag": ".bar",
            "targets": [
                "goal_bar",
            ],
        },
    ],
)`,
		},
		{
			bp: `custom {
	name: "foo",
	required: ["bar"],
	target_required: ["qux", "bazqux"],
	ramdisk: true,
	owner: "custom_owner",
	dists: [
		{
			tag: ".tag",
			targets: ["my_goal"],
		},
	],
}
		`,
			expectedBazelTarget: `soong_module(
    name = "foo",
    module_name = "foo",
    module_type = "custom",
    module_variant = "",
    module_deps = [
    ],
    dists = [
        {
            "tag": ".tag",
            "targets": [
                "my_goal",
            ],
        },
    ],
    owner = "custom_owner",
    ramdisk = True,
    required = [
        "bar",
    ],
    target_required = [
        "qux",
        "bazqux",
    ],
)`,
		},
	}

	for _, testCase := range testCases {
		config := android.TestConfig(buildDir, nil, testCase.bp, nil)
		ctx := android.NewTestContext(config)
		ctx.RegisterModuleType("custom", customModuleFactory)
		ctx.Register()

		_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
		android.FailIfErrored(t, errs)
		_, errs = ctx.PrepareBuildActions(config)
		android.FailIfErrored(t, errs)

		module := ctx.ModuleForTests("foo", "").Module().(*customModule)
		blueprintCtx := ctx.Context.Context

		actualBazelTarget := generateSoongModuleTarget(blueprintCtx, module)
		if actualBazelTarget != testCase.expectedBazelTarget {
			t.Errorf(
				"Expected generated Bazel target to be '%s', got '%s'",
				testCase.expectedBazelTarget,
				actualBazelTarget,
			)
		}
	}
}

func createPackageFixtures() []*bpdoc.Package {
	properties := []bpdoc.Property{
		bpdoc.Property{
			Name: "int64_prop",
			Type: "int64",
		},
		bpdoc.Property{
			Name: "int_prop",
			Type: "int",
		},
		bpdoc.Property{
			Name: "bool_prop",
			Type: "bool",
		},
		bpdoc.Property{
			Name: "string_prop",
			Type: "string",
		},
		bpdoc.Property{
			Name: "string_list_prop",
			Type: "list of string",
		},
		bpdoc.Property{
			Name: "nested_prop",
			Type: "",
			Properties: []bpdoc.Property{
				bpdoc.Property{
					Name: "int_prop",
					Type: "int",
				},
				bpdoc.Property{
					Name: "bool_prop",
					Type: "bool",
				},
				bpdoc.Property{
					Name: "string_prop",
					Type: "string",
				},
			},
		},
		bpdoc.Property{
			Name: "unknown_type",
			Type: "unknown",
		},
	}

	fooPropertyStruct := &bpdoc.PropertyStruct{
		Name:       "FooProperties",
		Properties: properties,
	}

	moduleTypes := []*bpdoc.ModuleType{
		&bpdoc.ModuleType{
			Name: "foo_library",
			PropertyStructs: []*bpdoc.PropertyStruct{
				fooPropertyStruct,
			},
		},

		&bpdoc.ModuleType{
			Name: "foo_binary",
			PropertyStructs: []*bpdoc.PropertyStruct{
				fooPropertyStruct,
			},
		},
		&bpdoc.ModuleType{
			Name: "foo_test",
			PropertyStructs: []*bpdoc.PropertyStruct{
				fooPropertyStruct,
			},
		},
	}

	return [](*bpdoc.Package){
		&bpdoc.Package{
			Name:        "foo_language",
			Path:        "android/soong/foo",
			ModuleTypes: moduleTypes,
		},
	}
}

func TestGenerateModuleRuleShims(t *testing.T) {
	ruleShims, err := createRuleShims(createPackageFixtures())
	if err != nil {
		panic(err)
	}

	if len(ruleShims) != 1 {
		t.Errorf("Expected to generate 1 rule shim, but got %d", len(ruleShims))
	}

	fooRuleShim := ruleShims["foo"]
	expectedRules := []string{"foo_binary", "foo_library", "foo_test_"}

	if len(fooRuleShim.rules) != 3 {
		t.Errorf("Expected 3 rules, but got %d", len(fooRuleShim.rules))
	}

	for i, rule := range fooRuleShim.rules {
		if rule != expectedRules[i] {
			t.Errorf("Expected rule shim to contain %s, but got %s", expectedRules[i], rule)
		}
	}

	expectedBzl := `load("//build/bazel/queryview_rules:providers.bzl", "SoongModuleInfo")

def _foo_binary_impl(ctx):
    return [SoongModuleInfo()]

foo_binary = rule(
    implementation = _foo_binary_impl,
    attrs = {
        "module_name": attr.string(mandatory = True),
        "module_variant": attr.string(),
        "module_deps": attr.label_list(providers = [SoongModuleInfo]),
        "bool_prop": attr.bool(),
        "int64_prop": attr.int(),
        "int_prop": attr.int(),
#         "nested_prop__int_prop": attr.int(),
#         "nested_prop__bool_prop": attr.bool(),
#         "nested_prop__string_prop": attr.string(),
        "string_list_prop": attr.string_list(),
        "string_prop": attr.string(),
    },
)

def _foo_library_impl(ctx):
    return [SoongModuleInfo()]

foo_library = rule(
    implementation = _foo_library_impl,
    attrs = {
        "module_name": attr.string(mandatory = True),
        "module_variant": attr.string(),
        "module_deps": attr.label_list(providers = [SoongModuleInfo]),
        "bool_prop": attr.bool(),
        "int64_prop": attr.int(),
        "int_prop": attr.int(),
#         "nested_prop__int_prop": attr.int(),
#         "nested_prop__bool_prop": attr.bool(),
#         "nested_prop__string_prop": attr.string(),
        "string_list_prop": attr.string_list(),
        "string_prop": attr.string(),
    },
)

def _foo_test__impl(ctx):
    return [SoongModuleInfo()]

foo_test_ = rule(
    implementation = _foo_test__impl,
    attrs = {
        "module_name": attr.string(mandatory = True),
        "module_variant": attr.string(),
        "module_deps": attr.label_list(providers = [SoongModuleInfo]),
        "bool_prop": attr.bool(),
        "int64_prop": attr.int(),
        "int_prop": attr.int(),
#         "nested_prop__int_prop": attr.int(),
#         "nested_prop__bool_prop": attr.bool(),
#         "nested_prop__string_prop": attr.string(),
        "string_list_prop": attr.string_list(),
        "string_prop": attr.string(),
    },
)
`

	if fooRuleShim.content != expectedBzl {
		t.Errorf(
			"Expected the generated rule shim bzl to be:\n%s\nbut got:\n%s",
			expectedBzl,
			fooRuleShim.content)
	}
}

func TestGenerateSoongModuleBzl(t *testing.T) {
	ruleShims, err := createRuleShims(createPackageFixtures())
	if err != nil {
		panic(err)
	}
	actualSoongModuleBzl := generateSoongModuleBzl(ruleShims)

	expectedLoad := "load(\"//build/bazel/queryview_rules:foo.bzl\", \"foo_binary\", \"foo_library\", \"foo_test_\")"
	expectedRuleMap := `soong_module_rule_map = {
    "foo_binary": foo_binary,
    "foo_library": foo_library,
    "foo_test_": foo_test_,
}`
	if !strings.Contains(actualSoongModuleBzl, expectedLoad) {
		t.Errorf(
			"Generated soong_module.bzl:\n\n%s\n\n"+
				"Could not find the load statement in the generated soong_module.bzl:\n%s",
			actualSoongModuleBzl,
			expectedLoad)
	}

	if !strings.Contains(actualSoongModuleBzl, expectedRuleMap) {
		t.Errorf(
			"Generated soong_module.bzl:\n\n%s\n\n"+
				"Could not find the module -> rule map in the generated soong_module.bzl:\n%s",
			actualSoongModuleBzl,
			expectedRuleMap)
	}
}
