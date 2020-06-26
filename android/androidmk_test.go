// Copyright 2019 Google Inc. All rights reserved.
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

package android

import (
	"fmt"
	"io"
	"reflect"
	"testing"
)

type customModule struct {
	ModuleBase
	data      AndroidMkData
	distFiles TaggedDistFiles
}

func (m *customModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	m.distFiles = m.GenerateTaggedDistFiles(ctx)
}

func (m *customModule) AndroidMk() AndroidMkData {
	return AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data AndroidMkData) {
			m.data = data
		},
	}
}

func (m *customModule) OutputFiles(tag string) (Paths, error) {
	switch tag {
	case "":
		return PathsForTesting("one.out"), nil
	case ".multiple":
		return PathsForTesting("two.out", "three/four.out"), nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (m *customModule) AndroidMkEntries() []AndroidMkEntries {
	return []AndroidMkEntries{
		{
			Class:     "CUSTOM_MODULE",
			DistFiles: m.distFiles,
		},
	}
}

func customModuleFactory() Module {
	module := &customModule{}
	InitAndroidModule(module)
	return module
}

func TestAndroidMkSingleton_PassesUpdatedAndroidMkDataToCustomCallback(t *testing.T) {
	bp := `
	custom {
		name: "foo",
		required: ["bar"],
		host_required: ["baz"],
		target_required: ["qux"],
	}
	`

	config := TestConfig(buildDir, nil, bp, nil)
	config.inMake = true // Enable androidmk Singleton

	ctx := NewTestContext()
	ctx.RegisterSingletonType("androidmk", AndroidMkSingleton)
	ctx.RegisterModuleType("custom", customModuleFactory)
	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	m := ctx.ModuleForTests("foo", "").Module().(*customModule)

	assertEqual := func(expected interface{}, actual interface{}) {
		if !reflect.DeepEqual(expected, actual) {
			t.Errorf("%q expected, but got %q", expected, actual)
		}
	}
	assertEqual([]string{"bar"}, m.data.Required)
	assertEqual([]string{"baz"}, m.data.Host_required)
	assertEqual([]string{"qux"}, m.data.Target_required)
}

func TestGetDistForGoals(t *testing.T) {
	testCases := []struct {
		bp                     string
		expectedAndroidMkLines []string
	}{
		{
			bp: `
			custom {
				name: "foo",
				dist: {
					targets: ["my_goal"]
				}
			}
			`,
			expectedAndroidMkLines: []string{
				".PHONY: my_goal\n",
				"$(call dist-for-goals,my_goal,one.out:one.out)\n",
			},
		},
		{
			bp: `
			custom {
				name: "foo",
				dists: [
					{
						targets: ["my_goal"],
					},
					{
						targets: ["my_second_goal", "my_third_goal"],
					},
				],
			}
			`,
			expectedAndroidMkLines: []string{
				".PHONY: my_goal\n",
				"$(call dist-for-goals,my_goal,one.out:one.out)\n",
				".PHONY: my_second_goal my_third_goal\n",
				"$(call dist-for-goals,my_second_goal my_third_goal,one.out:one.out)\n",
			},
		},
		{
			bp: `
			custom {
				name: "foo",
				dist: {
					targets: ["my_goal"],
				},
				dists: [
					{
						targets: ["my_second_goal", "my_third_goal"],
					},
				],
			}
			`,
			expectedAndroidMkLines: []string{
				".PHONY: my_second_goal my_third_goal\n",
				"$(call dist-for-goals,my_second_goal my_third_goal,one.out:one.out)\n",
				".PHONY: my_goal\n",
				"$(call dist-for-goals,my_goal,one.out:one.out)\n",
			},
		},
		{
			bp: `
			custom {
				name: "foo",
				dist: {
					targets: ["my_goal", "my_other_goal"],
					tag: ".multiple",
				},
				dists: [
					{
						targets: ["my_second_goal"],
						tag: ".multiple",
					},
					{
						targets: ["my_third_goal"],
						dir: "test/dir",
					},
					{
						targets: ["my_fourth_goal"],
						suffix: ".suffix",
					},
					{
						targets: ["my_fifth_goal"],
						dest: "new-name",
					},
					{
						targets: ["my_sixth_goal"],
						dest: "new-name",
						dir: "some/dir",
						suffix: ".suffix",
					},
				],
			}
			`,
			expectedAndroidMkLines: []string{
				".PHONY: my_second_goal\n",
				"$(call dist-for-goals,my_second_goal,two.out:two.out)\n",
				"$(call dist-for-goals,my_second_goal,three/four.out:four.out)\n",
				".PHONY: my_third_goal\n",
				"$(call dist-for-goals,my_third_goal,one.out:test/dir/one.out)\n",
				".PHONY: my_fourth_goal\n",
				"$(call dist-for-goals,my_fourth_goal,one.out:one.suffix.out)\n",
				".PHONY: my_fifth_goal\n",
				"$(call dist-for-goals,my_fifth_goal,one.out:new-name)\n",
				".PHONY: my_sixth_goal\n",
				"$(call dist-for-goals,my_sixth_goal,one.out:some/dir/new-name.suffix)\n",
				".PHONY: my_goal my_other_goal\n",
				"$(call dist-for-goals,my_goal my_other_goal,two.out:two.out)\n",
				"$(call dist-for-goals,my_goal my_other_goal,three/four.out:four.out)\n",
			},
		},
	}

	for _, testCase := range testCases {
		config := TestConfig(buildDir, nil, testCase.bp, nil)
		config.inMake = true // Enable androidmk Singleton

		ctx := NewTestContext()
		ctx.RegisterSingletonType("androidmk", AndroidMkSingleton)
		ctx.RegisterModuleType("custom", customModuleFactory)
		ctx.Register(config)

		_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
		FailIfErrored(t, errs)
		_, errs = ctx.PrepareBuildActions(config)
		FailIfErrored(t, errs)

		module := ctx.ModuleForTests("foo", "").Module().(*customModule)
		entries := AndroidMkEntriesForTest(t, config, "", module)
		if len(entries) != 1 {
			t.Errorf("Expected a single AndroidMk entry, got %d", len(entries))
		}
		androidMkLines := entries[0].GetDistForGoals(module)

		if len(androidMkLines) != len(testCase.expectedAndroidMkLines) {
			t.Errorf(
				"Expected %d AndroidMk lines, got %d:\n%v",
				len(testCase.expectedAndroidMkLines),
				len(androidMkLines),
				androidMkLines,
			)
		}
		for idx, line := range androidMkLines {
			expectedLine := testCase.expectedAndroidMkLines[idx]
			if line != expectedLine {
				t.Errorf(
					"Expected AndroidMk line to be '%s', got '%s'",
					line,
					expectedLine,
				)
			}
		}
	}
}
