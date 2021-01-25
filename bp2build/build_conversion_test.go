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

package bp2build

import (
	"android/soong/android"
	"testing"
)

func TestGenerateSoongModuleTargets(t *testing.T) {
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

	dir := "."
	for _, testCase := range testCases {
		config := android.TestConfig(buildDir, nil, testCase.bp, nil)
		ctx := android.NewTestContext(config)
		ctx.RegisterModuleType("custom", customModuleFactory)
		ctx.Register()

		_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
		android.FailIfErrored(t, errs)
		_, errs = ctx.PrepareBuildActions(config)
		android.FailIfErrored(t, errs)

		bazelTargets := GenerateSoongModuleTargets(ctx.Context.Context, false)[dir]
		if actualCount, expectedCount := len(bazelTargets), 1; actualCount != expectedCount {
			t.Fatalf("Expected %d bazel target, got %d", expectedCount, actualCount)
		}

		actualBazelTarget := bazelTargets[0]
		if actualBazelTarget.content != testCase.expectedBazelTarget {
			t.Errorf(
				"Expected generated Bazel target to be '%s', got '%s'",
				testCase.expectedBazelTarget,
				actualBazelTarget.content,
			)
		}
	}
}

func TestGenerateBazelTargetModules(t *testing.T) {
	testCases := []struct {
		bp                  string
		expectedBazelTarget string
	}{
		{
			bp: `custom {
	name: "foo",
    string_list_prop: ["a", "b"],
    string_prop: "a",
}`,
			expectedBazelTarget: `custom(
    name = "foo",
    string_list_prop = [
        "a",
        "b",
    ],
    string_prop = "a",
)`,
		},
	}

	dir := "."
	for _, testCase := range testCases {
		config := android.TestConfig(buildDir, nil, testCase.bp, nil)
		ctx := android.NewTestContext(config)
		ctx.RegisterModuleType("custom", customModuleFactory)
		ctx.RegisterBp2BuildMutator("custom", customBp2BuildMutator)
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
		android.FailIfErrored(t, errs)
		_, errs = ctx.ResolveDependencies(config)
		android.FailIfErrored(t, errs)

		bazelTargets := GenerateSoongModuleTargets(ctx.Context.Context, true)[dir]
		if actualCount, expectedCount := len(bazelTargets), 1; actualCount != expectedCount {
			t.Fatalf("Expected %d bazel target, got %d", expectedCount, actualCount)
		}

		actualBazelTarget := bazelTargets[0]
		if actualBazelTarget.content != testCase.expectedBazelTarget {
			t.Errorf(
				"Expected generated Bazel target to be '%s', got '%s'",
				testCase.expectedBazelTarget,
				actualBazelTarget.content,
			)
		}
	}
}

func TestModuleTypeBp2Build(t *testing.T) {
	testCases := []struct {
		moduleTypeUnderTest        string
		moduleTypeUnderTestFactory android.ModuleFactory
		bp                         string
		expectedBazelTarget        string
	}{
		{
			moduleTypeUnderTest:        "filegroup",
			moduleTypeUnderTestFactory: android.FileGroupFactory,
			bp: `filegroup {
	name: "foo",
	srcs: [],
}`,
			expectedBazelTarget: `filegroup(
    name = "foo",
    srcs = [
    ],
)`,
		},
		{
			moduleTypeUnderTest:        "filegroup",
			moduleTypeUnderTestFactory: android.FileGroupFactory,
			bp: `filegroup {
	name: "foo",
	srcs: ["a", "b"],
}`,
			expectedBazelTarget: `filegroup(
    name = "foo",
    srcs = [
        "a",
        "b",
    ],
)`,
		},
	}

	dir := "."
	for _, testCase := range testCases {
		config := android.TestConfig(buildDir, nil, testCase.bp, nil)
		ctx := android.NewTestContext(config)
		ctx.RegisterModuleType(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestFactory)
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
		android.FailIfErrored(t, errs)
		_, errs = ctx.ResolveDependencies(config)
		android.FailIfErrored(t, errs)

		bazelTargets := GenerateSoongModuleTargets(ctx.Context.Context, true)[dir]
		if actualCount, expectedCount := len(bazelTargets), 1; actualCount != expectedCount {
			t.Fatalf("Expected %d bazel target, got %d", expectedCount, actualCount)
		}

		actualBazelTarget := bazelTargets[0]
		if actualBazelTarget.content != testCase.expectedBazelTarget {
			t.Errorf(
				"Expected generated Bazel target to be '%s', got '%s'",
				testCase.expectedBazelTarget,
				actualBazelTarget.content,
			)
		}
	}
}
