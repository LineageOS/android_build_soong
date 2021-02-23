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
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"
)

type customModule struct {
	ModuleBase

	properties struct {
		Default_dist_files *string
		Dist_output_file   *bool
	}

	data       AndroidMkData
	distFiles  TaggedDistFiles
	outputFile OptionalPath

	// The paths that will be used as the default dist paths if no tag is
	// specified.
	defaultDistPaths Paths
}

const (
	defaultDistFiles_None    = "none"
	defaultDistFiles_Default = "default"
	defaultDistFiles_Tagged  = "tagged"
)

func (m *customModule) GenerateAndroidBuildActions(ctx ModuleContext) {

	// If the dist_output_file: true then create an output file that is stored in
	// the OutputFile property of the AndroidMkEntry.
	if proptools.BoolDefault(m.properties.Dist_output_file, true) {
		path := PathForTesting("dist-output-file.out")
		m.outputFile = OptionalPathForPath(path)

		// Previous code would prioritize the DistFiles property over the OutputFile
		// property in AndroidMkEntry when determining the default dist paths.
		// Setting this first allows it to be overridden based on the
		// default_dist_files setting replicating that previous behavior.
		m.defaultDistPaths = Paths{path}
	}

	// Based on the setting of the default_dist_files property possibly create a
	// TaggedDistFiles structure that will be stored in the DistFiles property of
	// the AndroidMkEntry.
	defaultDistFiles := proptools.StringDefault(m.properties.Default_dist_files, defaultDistFiles_Tagged)
	switch defaultDistFiles {
	case defaultDistFiles_None:
		// Do nothing

	case defaultDistFiles_Default:
		path := PathForTesting("default-dist.out")
		m.defaultDistPaths = Paths{path}
		m.distFiles = MakeDefaultDistFiles(path)

	case defaultDistFiles_Tagged:
		// Module types that set AndroidMkEntry.DistFiles to the result of calling
		// GenerateTaggedDistFiles(ctx) relied on no tag being treated as "" which
		// meant that the default dist paths would be whatever was returned by
		// OutputFiles(""). In order to preserve that behavior when treating no tag
		// as being equal to DefaultDistTag this ensures that
		// OutputFiles(DefaultDistTag) will return the same as OutputFiles("").
		m.defaultDistPaths = PathsForTesting("one.out")

		// This must be called after setting defaultDistPaths/outputFile as
		// GenerateTaggedDistFiles calls into OutputFiles(tag) which may use those
		// fields.
		m.distFiles = m.GenerateTaggedDistFiles(ctx)
	}
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
	case DefaultDistTag:
		if m.defaultDistPaths != nil {
			return m.defaultDistPaths, nil
		} else {
			return nil, fmt.Errorf("default dist tag is not available")
		}
	case "":
		return PathsForTesting("one.out"), nil
	case ".multiple":
		return PathsForTesting("two.out", "three/four.out"), nil
	case ".another-tag":
		return PathsForTesting("another.out"), nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (m *customModule) AndroidMkEntries() []AndroidMkEntries {
	return []AndroidMkEntries{
		{
			Class:      "CUSTOM_MODULE",
			DistFiles:  m.distFiles,
			OutputFile: m.outputFile,
		},
	}
}

func customModuleFactory() Module {
	module := &customModule{}

	module.AddProperties(&module.properties)

	InitAndroidModule(module)
	return module
}

// buildContextAndCustomModuleFoo creates a config object, processes the supplied
// bp module and then returns the config and the custom module called "foo".
func buildContextAndCustomModuleFoo(t *testing.T, bp string) (*TestContext, *customModule) {
	t.Helper()
	config := TestConfig(buildDir, nil, bp, nil)
	config.katiEnabled = true // Enable androidmk Singleton

	ctx := NewTestContext(config)
	ctx.RegisterSingletonType("androidmk", AndroidMkSingleton)
	ctx.RegisterModuleType("custom", customModuleFactory)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	module := ctx.ModuleForTests("foo", "").Module().(*customModule)
	return ctx, module
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

	_, m := buildContextAndCustomModuleFoo(t, bp)

	assertEqual := func(expected interface{}, actual interface{}) {
		if !reflect.DeepEqual(expected, actual) {
			t.Errorf("%q expected, but got %q", expected, actual)
		}
	}
	assertEqual([]string{"bar"}, m.data.Required)
	assertEqual([]string{"baz"}, m.data.Host_required)
	assertEqual([]string{"qux"}, m.data.Target_required)
}

func TestGenerateDistContributionsForMake(t *testing.T) {
	dc := &distContributions{
		copiesForGoals: []*copiesForGoals{
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("one.out", "one.out"),
					distCopyForTest("two.out", "other.out"),
				},
			},
		},
	}

	makeOutput := generateDistContributionsForMake(dc)

	assertStringEquals(t, `.PHONY: my_goal
$(call dist-for-goals,my_goal,one.out:one.out)
$(call dist-for-goals,my_goal,two.out:other.out)
`, strings.Join(makeOutput, ""))
}

func TestGetDistForGoals(t *testing.T) {
	bp := `
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
			`

	expectedAndroidMkLines := []string{
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
	}

	ctx, module := buildContextAndCustomModuleFoo(t, bp)
	entries := AndroidMkEntriesForTest(t, ctx, module)
	if len(entries) != 1 {
		t.Errorf("Expected a single AndroidMk entry, got %d", len(entries))
	}
	androidMkLines := entries[0].GetDistForGoals(module)

	if len(androidMkLines) != len(expectedAndroidMkLines) {
		t.Errorf(
			"Expected %d AndroidMk lines, got %d:\n%v",
			len(expectedAndroidMkLines),
			len(androidMkLines),
			androidMkLines,
		)
	}
	for idx, line := range androidMkLines {
		expectedLine := expectedAndroidMkLines[idx]
		if line != expectedLine {
			t.Errorf(
				"Expected AndroidMk line to be '%s', got '%s'",
				expectedLine,
				line,
			)
		}
	}
}

func distCopyForTest(from, to string) distCopy {
	return distCopy{PathForTesting(from), to}
}

func TestGetDistContributions(t *testing.T) {
	compareContributions := func(d1 *distContributions, d2 *distContributions) error {
		if d1 == nil || d2 == nil {
			if d1 != d2 {
				return fmt.Errorf("pointer mismatch, expected both to be nil but they were %p and %p", d1, d2)
			} else {
				return nil
			}
		}
		if expected, actual := len(d1.copiesForGoals), len(d2.copiesForGoals); expected != actual {
			return fmt.Errorf("length mismatch, expected %d found %d", expected, actual)
		}

		for i, copies1 := range d1.copiesForGoals {
			copies2 := d2.copiesForGoals[i]
			if expected, actual := copies1.goals, copies2.goals; expected != actual {
				return fmt.Errorf("goals mismatch at position %d: expected %q found %q", i, expected, actual)
			}

			if expected, actual := len(copies1.copies), len(copies2.copies); expected != actual {
				return fmt.Errorf("length mismatch in copy instructions at position %d, expected %d found %d", i, expected, actual)
			}

			for j, c1 := range copies1.copies {
				c2 := copies2.copies[j]
				if expected, actual := NormalizePathForTesting(c1.from), NormalizePathForTesting(c2.from); expected != actual {
					return fmt.Errorf("paths mismatch at position %d.%d: expected %q found %q", i, j, expected, actual)
				}

				if expected, actual := c1.dest, c2.dest; expected != actual {
					return fmt.Errorf("dest mismatch at position %d.%d: expected %q found %q", i, j, expected, actual)
				}
			}
		}

		return nil
	}

	formatContributions := func(d *distContributions) string {
		buf := &strings.Builder{}
		if d == nil {
			fmt.Fprint(buf, "nil")
		} else {
			for _, copiesForGoals := range d.copiesForGoals {
				fmt.Fprintf(buf, "    Goals: %q {\n", copiesForGoals.goals)
				for _, c := range copiesForGoals.copies {
					fmt.Fprintf(buf, "        %s -> %s\n", NormalizePathForTesting(c.from), c.dest)
				}
				fmt.Fprint(buf, "    }\n")
			}
		}
		return buf.String()
	}

	testHelper := func(t *testing.T, name, bp string, expectedContributions *distContributions) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()

			ctx, module := buildContextAndCustomModuleFoo(t, bp)
			entries := AndroidMkEntriesForTest(t, ctx, module)
			if len(entries) != 1 {
				t.Errorf("Expected a single AndroidMk entry, got %d", len(entries))
			}
			distContributions := entries[0].getDistContributions(module)

			if err := compareContributions(expectedContributions, distContributions); err != nil {
				t.Errorf("%s\nExpected Contributions\n%sActualContributions\n%s",
					err,
					formatContributions(expectedContributions),
					formatContributions(distContributions))
			}
		})
	}

	testHelper(t, "dist-without-tag", `
			custom {
				name: "foo",
				dist: {
					targets: ["my_goal"]
				}
			}
`,
		&distContributions{
			copiesForGoals: []*copiesForGoals{
				{
					goals: "my_goal",
					copies: []distCopy{
						distCopyForTest("one.out", "one.out"),
					},
				},
			},
		})

	testHelper(t, "dist-with-tag", `
			custom {
				name: "foo",
				dist: {
					targets: ["my_goal"],
					tag: ".another-tag",
				}
			}
`,
		&distContributions{
			copiesForGoals: []*copiesForGoals{
				{
					goals: "my_goal",
					copies: []distCopy{
						distCopyForTest("another.out", "another.out"),
					},
				},
			},
		})

	testHelper(t, "dists-with-tag", `
			custom {
				name: "foo",
				dists: [
					{
						targets: ["my_goal"],
						tag: ".another-tag",
					},
				],
			}
`,
		&distContributions{
			copiesForGoals: []*copiesForGoals{
				{
					goals: "my_goal",
					copies: []distCopy{
						distCopyForTest("another.out", "another.out"),
					},
				},
			},
		})

	testHelper(t, "multiple-dists-with-and-without-tag", `
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
		&distContributions{
			copiesForGoals: []*copiesForGoals{
				{
					goals: "my_goal",
					copies: []distCopy{
						distCopyForTest("one.out", "one.out"),
					},
				},
				{
					goals: "my_second_goal my_third_goal",
					copies: []distCopy{
						distCopyForTest("one.out", "one.out"),
					},
				},
			},
		})

	testHelper(t, "dist-plus-dists-without-tags", `
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
		&distContributions{
			copiesForGoals: []*copiesForGoals{
				{
					goals: "my_second_goal my_third_goal",
					copies: []distCopy{
						distCopyForTest("one.out", "one.out"),
					},
				},
				{
					goals: "my_goal",
					copies: []distCopy{
						distCopyForTest("one.out", "one.out"),
					},
				},
			},
		})

	testHelper(t, "dist-plus-dists-with-tags", `
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
		&distContributions{
			copiesForGoals: []*copiesForGoals{
				{
					goals: "my_second_goal",
					copies: []distCopy{
						distCopyForTest("two.out", "two.out"),
						distCopyForTest("three/four.out", "four.out"),
					},
				},
				{
					goals: "my_third_goal",
					copies: []distCopy{
						distCopyForTest("one.out", "test/dir/one.out"),
					},
				},
				{
					goals: "my_fourth_goal",
					copies: []distCopy{
						distCopyForTest("one.out", "one.suffix.out"),
					},
				},
				{
					goals: "my_fifth_goal",
					copies: []distCopy{
						distCopyForTest("one.out", "new-name"),
					},
				},
				{
					goals: "my_sixth_goal",
					copies: []distCopy{
						distCopyForTest("one.out", "some/dir/new-name.suffix"),
					},
				},
				{
					goals: "my_goal my_other_goal",
					copies: []distCopy{
						distCopyForTest("two.out", "two.out"),
						distCopyForTest("three/four.out", "four.out"),
					},
				},
			},
		})

	// The above test the default values of default_dist_files and use_output_file.

	// The following tests explicitly test the different combinations of those settings.
	testHelper(t, "tagged-dist-files-no-output", `
			custom {
				name: "foo",
				default_dist_files: "tagged",
				dist_output_file: false,
				dists: [
					{
						targets: ["my_goal"],
					},
					{
						targets: ["my_goal"],
						tag: ".multiple",
					},
				],
			}
`, &distContributions{
		copiesForGoals: []*copiesForGoals{
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("one.out", "one.out"),
				},
			},
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("two.out", "two.out"),
					distCopyForTest("three/four.out", "four.out"),
				},
			},
		},
	})

	testHelper(t, "default-dist-files-no-output", `
			custom {
				name: "foo",
				default_dist_files: "default",
				dist_output_file: false,
				dists: [
					{
						targets: ["my_goal"],
					},
					{
						targets: ["my_goal"],
						tag: ".multiple",
					},
				],
			}
`, &distContributions{
		copiesForGoals: []*copiesForGoals{
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("default-dist.out", "default-dist.out"),
				},
			},
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("two.out", "two.out"),
					distCopyForTest("three/four.out", "four.out"),
				},
			},
		},
	})

	testHelper(t, "no-dist-files-no-output", `
			custom {
				name: "foo",
				default_dist_files: "none",
				dist_output_file: false,
				dists: [
					// The following is silently ignored because there is not default file
					// in either the dist files or the output file.
					{
						targets: ["my_goal"],
					},
					{
						targets: ["my_goal"],
						tag: ".multiple",
					},
				],
			}
`, &distContributions{
		copiesForGoals: []*copiesForGoals{
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("two.out", "two.out"),
					distCopyForTest("three/four.out", "four.out"),
				},
			},
		},
	})

	testHelper(t, "tagged-dist-files-default-output", `
			custom {
				name: "foo",
				default_dist_files: "tagged",
				dist_output_file: true,
				dists: [
					{
						targets: ["my_goal"],
					},
					{
						targets: ["my_goal"],
						tag: ".multiple",
					},
				],
			}
`, &distContributions{
		copiesForGoals: []*copiesForGoals{
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("one.out", "one.out"),
				},
			},
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("two.out", "two.out"),
					distCopyForTest("three/four.out", "four.out"),
				},
			},
		},
	})

	testHelper(t, "default-dist-files-default-output", `
			custom {
				name: "foo",
				default_dist_files: "default",
				dist_output_file: true,
				dists: [
					{
						targets: ["my_goal"],
					},
					{
						targets: ["my_goal"],
						tag: ".multiple",
					},
				],
			}
`, &distContributions{
		copiesForGoals: []*copiesForGoals{
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("default-dist.out", "default-dist.out"),
				},
			},
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("two.out", "two.out"),
					distCopyForTest("three/four.out", "four.out"),
				},
			},
		},
	})

	testHelper(t, "no-dist-files-default-output", `
			custom {
				name: "foo",
				default_dist_files: "none",
				dist_output_file: true,
				dists: [
					{
						targets: ["my_goal"],
					},
					{
						targets: ["my_goal"],
						tag: ".multiple",
					},
				],
			}
`, &distContributions{
		copiesForGoals: []*copiesForGoals{
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("dist-output-file.out", "dist-output-file.out"),
				},
			},
			{
				goals: "my_goal",
				copies: []distCopy{
					distCopyForTest("two.out", "two.out"),
					distCopyForTest("three/four.out", "four.out"),
				},
			},
		},
	})
}
