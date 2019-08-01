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

package build

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"android/soong/ui/logger"
)

func testContext() Context {
	return Context{&ContextImpl{
		Context: context.Background(),
		Logger:  logger.New(&bytes.Buffer{}),
		Writer:  &bytes.Buffer{},
	}}
}

func TestConfigParseArgsJK(t *testing.T) {
	ctx := testContext()

	testCases := []struct {
		args []string

		parallel  int
		keepGoing int
		remaining []string
	}{
		{nil, -1, -1, nil},

		{[]string{"-j"}, -1, -1, nil},
		{[]string{"-j1"}, 1, -1, nil},
		{[]string{"-j1234"}, 1234, -1, nil},

		{[]string{"-j", "1"}, 1, -1, nil},
		{[]string{"-j", "1234"}, 1234, -1, nil},
		{[]string{"-j", "1234", "abc"}, 1234, -1, []string{"abc"}},
		{[]string{"-j", "abc"}, -1, -1, []string{"abc"}},
		{[]string{"-j", "1abc"}, -1, -1, []string{"1abc"}},

		{[]string{"-k"}, -1, 0, nil},
		{[]string{"-k0"}, -1, 0, nil},
		{[]string{"-k1"}, -1, 1, nil},
		{[]string{"-k1234"}, -1, 1234, nil},

		{[]string{"-k", "0"}, -1, 0, nil},
		{[]string{"-k", "1"}, -1, 1, nil},
		{[]string{"-k", "1234"}, -1, 1234, nil},
		{[]string{"-k", "1234", "abc"}, -1, 1234, []string{"abc"}},
		{[]string{"-k", "abc"}, -1, 0, []string{"abc"}},
		{[]string{"-k", "1abc"}, -1, 0, []string{"1abc"}},

		// TODO: These are supported in Make, should we support them?
		//{[]string{"-kj"}, -1, 0},
		//{[]string{"-kj8"}, 8, 0},

		// -jk is not valid in Make
	}

	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			defer logger.Recover(func(err error) {
				t.Fatal(err)
			})

			c := &configImpl{
				parallel:  -1,
				keepGoing: -1,
			}
			c.parseArgs(ctx, tc.args)

			if c.parallel != tc.parallel {
				t.Errorf("for %q, parallel:\nwant: %d\n got: %d\n",
					strings.Join(tc.args, " "),
					tc.parallel, c.parallel)
			}
			if c.keepGoing != tc.keepGoing {
				t.Errorf("for %q, keep going:\nwant: %d\n got: %d\n",
					strings.Join(tc.args, " "),
					tc.keepGoing, c.keepGoing)
			}
			if !reflect.DeepEqual(c.arguments, tc.remaining) {
				t.Errorf("for %q, remaining arguments:\nwant: %q\n got: %q\n",
					strings.Join(tc.args, " "),
					tc.remaining, c.arguments)
			}
		})
	}
}

func TestConfigParseArgsVars(t *testing.T) {
	ctx := testContext()

	testCases := []struct {
		env  []string
		args []string

		expectedEnv []string
		remaining   []string
	}{
		{},
		{
			env: []string{"A=bc"},

			expectedEnv: []string{"A=bc"},
		},
		{
			args: []string{"abc"},

			remaining: []string{"abc"},
		},

		{
			args: []string{"A=bc"},

			expectedEnv: []string{"A=bc"},
		},
		{
			env:  []string{"A=a"},
			args: []string{"A=bc"},

			expectedEnv: []string{"A=bc"},
		},

		{
			env:  []string{"A=a"},
			args: []string{"A=", "=b"},

			expectedEnv: []string{"A="},
			remaining:   []string{"=b"},
		},
	}

	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			defer logger.Recover(func(err error) {
				t.Fatal(err)
			})

			e := Environment(tc.env)
			c := &configImpl{
				environ: &e,
			}
			c.parseArgs(ctx, tc.args)

			if !reflect.DeepEqual([]string(*c.environ), tc.expectedEnv) {
				t.Errorf("for env=%q args=%q, environment:\nwant: %q\n got: %q\n",
					tc.env, tc.args,
					tc.expectedEnv, []string(*c.environ))
			}
			if !reflect.DeepEqual(c.arguments, tc.remaining) {
				t.Errorf("for env=%q args=%q, remaining arguments:\nwant: %q\n got: %q\n",
					tc.env, tc.args,
					tc.remaining, c.arguments)
			}
		})
	}
}

func TestConfigCheckTopDir(t *testing.T) {
	ctx := testContext()
	buildRootDir := filepath.Dir(srcDirFileCheck)
	expectedErrStr := fmt.Sprintf("Current working directory must be the source tree. %q not found.", srcDirFileCheck)

	tests := []struct {
		// ********* Setup *********
		// Test description.
		description string

		// ********* Action *********
		// If set to true, the build root file is created.
		rootBuildFile bool

		// The current path where Soong is being executed.
		path string

		// ********* Validation *********
		// Expecting error and validate the error string against expectedErrStr.
		wantErr bool
	}{{
		description:   "current directory is the root source tree",
		rootBuildFile: true,
		path:          ".",
		wantErr:       false,
	}, {
		description:   "one level deep in the source tree",
		rootBuildFile: true,
		path:          "1",
		wantErr:       true,
	}, {
		description:   "very deep in the source tree",
		rootBuildFile: true,
		path:          "1/2/3/4/5/6/7/8/9/1/2/3/4/5/6/7/8/9/1/2/3/4/5/6/7/8/9/1/2/3/4/5/6/7",
		wantErr:       true,
	}, {
		description:   "outside of source tree",
		rootBuildFile: false,
		path:          "1/2/3/4/5",
		wantErr:       true,
	}}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			defer logger.Recover(func(err error) {
				if !tt.wantErr {
					t.Fatalf("Got unexpected error: %v", err)
				}
				if expectedErrStr != err.Error() {
					t.Fatalf("expected %s, got %s", expectedErrStr, err.Error())
				}
			})

			// Create the root source tree.
			rootDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(rootDir)

			// Create the build root file. This is to test if topDir returns an error if the build root
			// file does not exist.
			if tt.rootBuildFile {
				dir := filepath.Join(rootDir, buildRootDir)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Errorf("failed to create %s directory: %v", dir, err)
				}
				f := filepath.Join(rootDir, srcDirFileCheck)
				if err := ioutil.WriteFile(f, []byte{}, 0644); err != nil {
					t.Errorf("failed to create file %s: %v", f, err)
				}
			}

			// Next block of code is to set the current directory.
			dir := rootDir
			if tt.path != "" {
				dir = filepath.Join(dir, tt.path)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Errorf("failed to create %s directory: %v", dir, err)
				}
			}
			curDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get the current directory: %v", err)
			}
			defer func() { os.Chdir(curDir) }()

			if err := os.Chdir(dir); err != nil {
				t.Fatalf("failed to change directory to %s: %v", dir, err)
			}

			checkTopDir(ctx)
		})
	}
}

func TestConfigConvertToTarget(t *testing.T) {
	tests := []struct {
		// ********* Setup *********
		// Test description.
		description string

		// ********* Action *********
		// The current directory where Soong is being executed.
		dir string

		// The current prefix string to be pre-appended to the target.
		prefix string

		// ********* Validation *********
		// The expected target to be invoked in ninja.
		expectedTarget string
	}{{
		description:    "one level directory in source tree",
		dir:            "test1",
		prefix:         "MODULES-IN-",
		expectedTarget: "MODULES-IN-test1",
	}, {
		description:    "multiple level directories in source tree",
		dir:            "test1/test2/test3/test4",
		prefix:         "GET-INSTALL-PATH-IN-",
		expectedTarget: "GET-INSTALL-PATH-IN-test1-test2-test3-test4",
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			target := convertToTarget(tt.dir, tt.prefix)
			if target != tt.expectedTarget {
				t.Errorf("expected %s, got %s for target", tt.expectedTarget, target)
			}
		})
	}
}

func setTop(t *testing.T, dir string) func() {
	curDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change directory to top dir %s: %v", dir, err)
	}
	return func() { os.Chdir(curDir) }
}

func createBuildFiles(t *testing.T, topDir string, buildFiles []string) {
	for _, buildFile := range buildFiles {
		buildFile = filepath.Join(topDir, buildFile)
		if err := ioutil.WriteFile(buildFile, []byte{}, 0644); err != nil {
			t.Errorf("failed to create file %s: %v", buildFile, err)
		}
	}
}

func createDirectories(t *testing.T, topDir string, dirs []string) {
	for _, dir := range dirs {
		dir = filepath.Join(topDir, dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Errorf("failed to create %s directory: %v", dir, err)
		}
	}
}

func TestConfigGetTargets(t *testing.T) {
	ctx := testContext()
	tests := []struct {
		// ********* Setup *********
		// Test description.
		description string

		// Directories that exist in the source tree.
		dirsInTrees []string

		// Build files that exists in the source tree.
		buildFiles []string

		// ********* Action *********
		// Directories passed in to soong_ui.
		dirs []string

		// Current directory that the user executed the build action command.
		curDir string

		// ********* Validation *********
		// Expected targets from the function.
		expectedTargets []string

		// Expecting error from running test case.
		errStr string
	}{{
		description:     "one target dir specified",
		dirsInTrees:     []string{"0/1/2/3"},
		buildFiles:      []string{"0/1/2/3/Android.bp"},
		dirs:            []string{"1/2/3"},
		curDir:          "0",
		expectedTargets: []string{"MODULES-IN-0-1-2-3"},
	}, {
		description: "one target dir specified, build file does not exist",
		dirsInTrees: []string{"0/1/2/3"},
		buildFiles:  []string{},
		dirs:        []string{"1/2/3"},
		curDir:      "0",
		errStr:      "Build file not found for 0/1/2/3 directory",
	}, {
		description: "one target dir specified, invalid targets specified",
		dirsInTrees: []string{"0/1/2/3"},
		buildFiles:  []string{},
		dirs:        []string{"1/2/3:t1:t2"},
		curDir:      "0",
		errStr:      "1/2/3:t1:t2 not in proper directory:target1,target2,... format (\":\" was specified more than once)",
	}, {
		description:     "one target dir specified, no targets specified but has colon",
		dirsInTrees:     []string{"0/1/2/3"},
		buildFiles:      []string{"0/1/2/3/Android.bp"},
		dirs:            []string{"1/2/3:"},
		curDir:          "0",
		expectedTargets: []string{"MODULES-IN-0-1-2-3"},
	}, {
		description:     "one target dir specified, two targets specified",
		dirsInTrees:     []string{"0/1/2/3"},
		buildFiles:      []string{"0/1/2/3/Android.bp"},
		dirs:            []string{"1/2/3:t1,t2"},
		curDir:          "0",
		expectedTargets: []string{"t1", "t2"},
	}, {
		description: "one target dir specified, no targets and has a comma",
		dirsInTrees: []string{"0/1/2/3"},
		buildFiles:  []string{"0/1/2/3/Android.bp"},
		dirs:        []string{"1/2/3:,"},
		curDir:      "0",
		errStr:      "0/1/2/3 not in proper directory:target1,target2,... format",
	}, {
		description: "one target dir specified, improper targets defined",
		dirsInTrees: []string{"0/1/2/3"},
		buildFiles:  []string{"0/1/2/3/Android.bp"},
		dirs:        []string{"1/2/3:,t1"},
		curDir:      "0",
		errStr:      "0/1/2/3 not in proper directory:target1,target2,... format",
	}, {
		description: "one target dir specified, blank target",
		dirsInTrees: []string{"0/1/2/3"},
		buildFiles:  []string{"0/1/2/3/Android.bp"},
		dirs:        []string{"1/2/3:t1,"},
		curDir:      "0",
		errStr:      "0/1/2/3 not in proper directory:target1,target2,... format",
	}, {
		description:     "one target dir specified, many targets specified",
		dirsInTrees:     []string{"0/1/2/3"},
		buildFiles:      []string{"0/1/2/3/Android.bp"},
		dirs:            []string{"1/2/3:t1,t2,t3,t4,t5,t6,t7,t8,t9,t10"},
		curDir:          "0",
		expectedTargets: []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7", "t8", "t9", "t10"},
	}, {
		description: "one target dir specified, one target specified, build file does not exist",
		dirsInTrees: []string{"0/1/2/3"},
		buildFiles:  []string{},
		dirs:        []string{"1/2/3:t1"},
		curDir:      "0",
		errStr:      "Couldn't locate a build file from 0/1/2/3 directory",
	}, {
		description: "one target dir specified, one target specified, build file not in target dir",
		dirsInTrees: []string{"0/1/2/3"},
		buildFiles:  []string{"0/1/2/Android.mk"},
		dirs:        []string{"1/2/3:t1"},
		curDir:      "0",
		errStr:      "Couldn't locate a build file from 0/1/2/3 directory",
	}, {
		description:     "one target dir specified, build file not in target dir",
		dirsInTrees:     []string{"0/1/2/3"},
		buildFiles:      []string{"0/1/2/Android.mk"},
		dirs:            []string{"1/2/3"},
		curDir:          "0",
		expectedTargets: []string{"MODULES-IN-0-1-2"},
	}, {
		description:     "multiple targets dir specified, targets specified",
		dirsInTrees:     []string{"0/1/2/3", "0/3/4"},
		buildFiles:      []string{"0/1/2/3/Android.bp", "0/3/4/Android.mk"},
		dirs:            []string{"1/2/3:t1,t2", "3/4:t3,t4,t5"},
		curDir:          "0",
		expectedTargets: []string{"t1", "t2", "t3", "t4", "t5"},
	}, {
		description:     "multiple targets dir specified, one directory has targets specified",
		dirsInTrees:     []string{"0/1/2/3", "0/3/4"},
		buildFiles:      []string{"0/1/2/3/Android.bp", "0/3/4/Android.mk"},
		dirs:            []string{"1/2/3:t1,t2", "3/4"},
		curDir:          "0",
		expectedTargets: []string{"t1", "t2", "MODULES-IN-0-3-4"},
	}, {
		description: "two dirs specified, only one dir exist",
		dirsInTrees: []string{"0/1/2/3"},
		buildFiles:  []string{"0/1/2/3/Android.mk"},
		dirs:        []string{"1/2/3:t1", "3/4"},
		curDir:      "0",
		errStr:      "couldn't find directory 0/3/4",
	}, {
		description:     "multiple targets dirs specified at root source tree",
		dirsInTrees:     []string{"0/1/2/3", "0/3/4"},
		buildFiles:      []string{"0/1/2/3/Android.bp", "0/3/4/Android.mk"},
		dirs:            []string{"0/1/2/3:t1,t2", "0/3/4"},
		curDir:          ".",
		expectedTargets: []string{"t1", "t2", "MODULES-IN-0-3-4"},
	}, {
		description: "no directories specified",
		dirsInTrees: []string{"0/1/2/3", "0/3/4"},
		buildFiles:  []string{"0/1/2/3/Android.bp", "0/3/4/Android.mk"},
		dirs:        []string{},
		curDir:      ".",
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			defer logger.Recover(func(err error) {
				if tt.errStr == "" {
					t.Fatalf("Got unexpected error: %v", err)
				}
				if tt.errStr != err.Error() {
					t.Errorf("expected %s, got %s", tt.errStr, err.Error())
				}
			})

			// Create the root source tree.
			topDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(topDir)

			createDirectories(t, topDir, tt.dirsInTrees)
			createBuildFiles(t, topDir, tt.buildFiles)
			r := setTop(t, topDir)
			defer r()

			targets := getTargetsFromDirs(ctx, tt.curDir, tt.dirs, "MODULES-IN-")
			if !reflect.DeepEqual(targets, tt.expectedTargets) {
				t.Errorf("expected %v, got %v for targets", tt.expectedTargets, targets)
			}

			// If the execution reached here and there was an expected error code, the unit test case failed.
			if tt.errStr != "" {
				t.Errorf("expecting error %s", tt.errStr)
			}
		})
	}
}

func TestConfigFindBuildFile(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		// ********* Setup *********
		// Test description.
		description string

		// Array of build files to create in dir.
		buildFiles []string

		// Directories that exist in the source tree.
		dirsInTrees []string

		// ********* Action *********
		// The base directory is where findBuildFile is invoked.
		dir string

		// ********* Validation *********
		// Expected build file path to find.
		expectedBuildFile string
	}{{
		description:       "build file exists at leaf directory",
		buildFiles:        []string{"1/2/3/Android.bp"},
		dirsInTrees:       []string{"1/2/3"},
		dir:               "1/2/3",
		expectedBuildFile: "1/2/3/Android.mk",
	}, {
		description:       "build file exists in all directory paths",
		buildFiles:        []string{"1/Android.mk", "1/2/Android.mk", "1/2/3/Android.mk"},
		dirsInTrees:       []string{"1/2/3"},
		dir:               "1/2/3",
		expectedBuildFile: "1/2/3/Android.mk",
	}, {
		description:       "build file does not exist in all directory paths",
		buildFiles:        []string{},
		dirsInTrees:       []string{"1/2/3"},
		dir:               "1/2/3",
		expectedBuildFile: "",
	}, {
		description:       "build file exists only at top directory",
		buildFiles:        []string{"Android.bp"},
		dirsInTrees:       []string{"1/2/3"},
		dir:               "1/2/3",
		expectedBuildFile: "",
	}, {
		description:       "build file exist in a subdirectory",
		buildFiles:        []string{"1/2/Android.bp"},
		dirsInTrees:       []string{"1/2/3"},
		dir:               "1/2/3",
		expectedBuildFile: "1/2/Android.mk",
	}, {
		description:       "build file exists in a subdirectory",
		buildFiles:        []string{"1/Android.mk"},
		dirsInTrees:       []string{"1/2/3"},
		dir:               "1/2/3",
		expectedBuildFile: "1/Android.mk",
	}, {
		description:       "top directory",
		buildFiles:        []string{"Android.bp"},
		dirsInTrees:       []string{},
		dir:               ".",
		expectedBuildFile: "",
	}, {
		description:       "build file exists in subdirectory",
		buildFiles:        []string{"1/2/3/Android.bp", "1/2/4/Android.bp"},
		dirsInTrees:       []string{"1/2/3", "1/2/4"},
		dir:               "1/2",
		expectedBuildFile: "1/2/Android.mk",
	}, {
		description:       "build file exists in parent subdirectory",
		buildFiles:        []string{"1/5/Android.bp"},
		dirsInTrees:       []string{"1/2/3", "1/2/4", "1/5"},
		dir:               "1/2",
		expectedBuildFile: "1/Android.mk",
	}, {
		description:       "build file exists in deep parent's subdirectory.",
		buildFiles:        []string{"1/5/6/Android.bp"},
		dirsInTrees:       []string{"1/2/3", "1/2/4", "1/5/6", "1/5/7"},
		dir:               "1/2",
		expectedBuildFile: "1/Android.mk",
	}}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			defer logger.Recover(func(err error) {
				t.Fatalf("Got unexpected error: %v", err)
			})

			topDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(topDir)

			createDirectories(t, topDir, tt.dirsInTrees)
			createBuildFiles(t, topDir, tt.buildFiles)

			curDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("Could not get working directory: %v", err)
			}
			defer func() { os.Chdir(curDir) }()
			if err := os.Chdir(topDir); err != nil {
				t.Fatalf("Could not change top dir to %s: %v", topDir, err)
			}

			buildFile := findBuildFile(ctx, tt.dir)
			if buildFile != tt.expectedBuildFile {
				t.Errorf("expected %q, got %q for build file", tt.expectedBuildFile, buildFile)
			}
		})
	}
}

func TestConfigSplitArgs(t *testing.T) {
	tests := []struct {
		// ********* Setup *********
		// Test description.
		description string

		// ********* Action *********
		// Arguments passed in to soong_ui.
		args []string

		// ********* Validation *********
		// Expected newArgs list after extracting the directories.
		expectedNewArgs []string

		// Expected directories
		expectedDirs []string
	}{{
		description:     "flags but no directories specified",
		args:            []string{"showcommands", "-j", "-k"},
		expectedNewArgs: []string{"showcommands", "-j", "-k"},
		expectedDirs:    []string{},
	}, {
		description:     "flags and one directory specified",
		args:            []string{"snod", "-j", "dir:target1,target2"},
		expectedNewArgs: []string{"snod", "-j"},
		expectedDirs:    []string{"dir:target1,target2"},
	}, {
		description:     "flags and directories specified",
		args:            []string{"dist", "-k", "dir1", "dir2:target1,target2"},
		expectedNewArgs: []string{"dist", "-k"},
		expectedDirs:    []string{"dir1", "dir2:target1,target2"},
	}, {
		description:     "only directories specified",
		args:            []string{"dir1", "dir2", "dir3:target1,target2"},
		expectedNewArgs: []string{},
		expectedDirs:    []string{"dir1", "dir2", "dir3:target1,target2"},
	}}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			args, dirs := splitArgs(tt.args)
			if !reflect.DeepEqual(tt.expectedNewArgs, args) {
				t.Errorf("expected %v, got %v for arguments", tt.expectedNewArgs, args)
			}
			if !reflect.DeepEqual(tt.expectedDirs, dirs) {
				t.Errorf("expected %v, got %v for directories", tt.expectedDirs, dirs)
			}
		})
	}
}

type envVar struct {
	name  string
	value string
}

type buildActionTestCase struct {
	// ********* Setup *********
	// Test description.
	description string

	// Directories that exist in the source tree.
	dirsInTrees []string

	// Build files that exists in the source tree.
	buildFiles []string

	// Create root symlink that points to topDir.
	rootSymlink bool

	// ********* Action *********
	// Arguments passed in to soong_ui.
	args []string

	// Directory where the build action was invoked.
	curDir string

	// WITH_TIDY_ONLY environment variable specified.
	tidyOnly string

	// ********* Validation *********
	// Expected arguments to be in Config instance.
	expectedArgs []string

	// Expecting error from running test case.
	expectedErrStr string
}

func testGetConfigArgs(t *testing.T, tt buildActionTestCase, action BuildAction) {
	ctx := testContext()

	defer logger.Recover(func(err error) {
		if tt.expectedErrStr == "" {
			t.Fatalf("Got unexpected error: %v", err)
		}
		if tt.expectedErrStr != err.Error() {
			t.Errorf("expected %s, got %s", tt.expectedErrStr, err.Error())
		}
	})

	// Environment variables to set it to blank on every test case run.
	resetEnvVars := []string{
		"WITH_TIDY_ONLY",
	}

	for _, name := range resetEnvVars {
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("failed to unset environment variable %s: %v", name, err)
		}
	}
	if tt.tidyOnly != "" {
		if err := os.Setenv("WITH_TIDY_ONLY", tt.tidyOnly); err != nil {
			t.Errorf("failed to set WITH_TIDY_ONLY to %s: %v", tt.tidyOnly, err)
		}
	}

	// Create the root source tree.
	topDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(topDir)

	createDirectories(t, topDir, tt.dirsInTrees)
	createBuildFiles(t, topDir, tt.buildFiles)

	if tt.rootSymlink {
		// Create a secondary root source tree which points to the true root source tree.
		symlinkTopDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("failed to create symlink temp dir: %v", err)
		}
		defer os.RemoveAll(symlinkTopDir)

		symlinkTopDir = filepath.Join(symlinkTopDir, "root")
		err = os.Symlink(topDir, symlinkTopDir)
		if err != nil {
			t.Fatalf("failed to create symlink: %v", err)
		}
		topDir = symlinkTopDir
	}

	r := setTop(t, topDir)
	defer r()

	// The next block is to create the root build file.
	rootBuildFileDir := filepath.Dir(srcDirFileCheck)
	if err := os.MkdirAll(rootBuildFileDir, 0755); err != nil {
		t.Fatalf("Failed to create %s directory: %v", rootBuildFileDir, err)
	}

	if err := ioutil.WriteFile(srcDirFileCheck, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create %s file: %v", srcDirFileCheck, err)
	}

	args := getConfigArgs(action, tt.curDir, ctx, tt.args)
	if !reflect.DeepEqual(tt.expectedArgs, args) {
		t.Fatalf("expected %v, got %v for config arguments", tt.expectedArgs, args)
	}

	// If the execution reached here and there was an expected error code, the unit test case failed.
	if tt.expectedErrStr != "" {
		t.Errorf("expecting error %s", tt.expectedErrStr)
	}
}

func TestGetConfigArgsBuildModules(t *testing.T) {
	tests := []buildActionTestCase{{
		description:  "normal execution from the root source tree directory",
		dirsInTrees:  []string{"0/1/2", "0/2", "0/3"},
		buildFiles:   []string{"0/1/2/Android.mk", "0/2/Android.bp", "0/3/Android.mk"},
		args:         []string{"-j", "fake_module", "fake_module2"},
		curDir:       ".",
		tidyOnly:     "",
		expectedArgs: []string{"-j", "fake_module", "fake_module2"},
	}, {
		description:  "normal execution in deep directory",
		dirsInTrees:  []string{"0/1/2", "0/2", "0/3", "1/2/3/4/5/6/7/8/9/1/2/3/4/5/6"},
		buildFiles:   []string{"0/1/2/Android.mk", "0/2/Android.bp", "1/2/3/4/5/6/7/8/9/1/2/3/4/5/6/Android.mk"},
		args:         []string{"-j", "fake_module", "fake_module2", "-k"},
		curDir:       "1/2/3/4/5/6/7/8/9",
		tidyOnly:     "",
		expectedArgs: []string{"-j", "fake_module", "fake_module2", "-k"},
	}, {
		description:  "normal execution in deep directory, no targets",
		dirsInTrees:  []string{"0/1/2", "0/2", "0/3", "1/2/3/4/5/6/7/8/9/1/2/3/4/5/6"},
		buildFiles:   []string{"0/1/2/Android.mk", "0/2/Android.bp", "1/2/3/4/5/6/7/8/9/1/2/3/4/5/6/Android.mk"},
		args:         []string{"-j", "-k"},
		curDir:       "1/2/3/4/5/6/7/8/9",
		tidyOnly:     "",
		expectedArgs: []string{"-j", "-k"},
	}, {
		description:  "normal execution in root source tree, no args",
		dirsInTrees:  []string{"0/1/2", "0/2", "0/3"},
		buildFiles:   []string{"0/1/2/Android.mk", "0/2/Android.bp"},
		args:         []string{},
		curDir:       "0/2",
		tidyOnly:     "",
		expectedArgs: []string{},
	}, {
		description:  "normal execution in symlink root source tree, no args",
		dirsInTrees:  []string{"0/1/2", "0/2", "0/3"},
		buildFiles:   []string{"0/1/2/Android.mk", "0/2/Android.bp"},
		rootSymlink:  true,
		args:         []string{},
		curDir:       "0/2",
		tidyOnly:     "",
		expectedArgs: []string{},
	}}
	for _, tt := range tests {
		t.Run("build action BUILD_MODULES with dependencies, "+tt.description, func(t *testing.T) {
			testGetConfigArgs(t, tt, BUILD_MODULES)
		})
	}
}

func TestGetConfigArgsBuildModulesInDirectory(t *testing.T) {
	tests := []buildActionTestCase{{
		description:  "normal execution in a directory",
		dirsInTrees:  []string{"0/1/2"},
		buildFiles:   []string{"0/1/2/Android.mk"},
		args:         []string{"fake-module"},
		curDir:       "0/1/2",
		tidyOnly:     "",
		expectedArgs: []string{"fake-module", "MODULES-IN-0-1-2"},
	}, {
		description:  "build file in parent directory",
		dirsInTrees:  []string{"0/1/2"},
		buildFiles:   []string{"0/1/Android.mk"},
		args:         []string{},
		curDir:       "0/1/2",
		tidyOnly:     "",
		expectedArgs: []string{"MODULES-IN-0-1"},
	},
		{
			description:  "build file in parent directory, multiple module names passed in",
			dirsInTrees:  []string{"0/1/2"},
			buildFiles:   []string{"0/1/Android.mk"},
			args:         []string{"fake-module1", "fake-module2", "fake-module3"},
			curDir:       "0/1/2",
			tidyOnly:     "",
			expectedArgs: []string{"fake-module1", "fake-module2", "fake-module3", "MODULES-IN-0-1"},
		}, {
			description:  "build file in 2nd level parent directory",
			dirsInTrees:  []string{"0/1/2"},
			buildFiles:   []string{"0/Android.bp"},
			args:         []string{},
			curDir:       "0/1/2",
			tidyOnly:     "",
			expectedArgs: []string{"MODULES-IN-0"},
		}, {
			description:  "build action executed at root directory",
			dirsInTrees:  []string{},
			buildFiles:   []string{},
			rootSymlink:  false,
			args:         []string{},
			curDir:       ".",
			tidyOnly:     "",
			expectedArgs: []string{},
		}, {
			description:  "build action executed at root directory in symlink",
			dirsInTrees:  []string{},
			buildFiles:   []string{},
			rootSymlink:  true,
			args:         []string{},
			curDir:       ".",
			tidyOnly:     "",
			expectedArgs: []string{},
		}, {
			description:    "build file not found",
			dirsInTrees:    []string{"0/1/2"},
			buildFiles:     []string{},
			args:           []string{},
			curDir:         "0/1/2",
			tidyOnly:       "",
			expectedArgs:   []string{"MODULES-IN-0-1-2"},
			expectedErrStr: "Build file not found for 0/1/2 directory",
		}, {
			description:  "GET-INSTALL-PATH specified,",
			dirsInTrees:  []string{"0/1/2"},
			buildFiles:   []string{"0/1/Android.mk"},
			args:         []string{"GET-INSTALL-PATH", "-j", "-k", "GET-INSTALL-PATH"},
			curDir:       "0/1/2",
			tidyOnly:     "",
			expectedArgs: []string{"-j", "-k", "GET-INSTALL-PATH-IN-0-1"},
		}, {
			description:  "tidy only environment variable specified,",
			dirsInTrees:  []string{"0/1/2"},
			buildFiles:   []string{"0/1/Android.mk"},
			args:         []string{"GET-INSTALL-PATH"},
			curDir:       "0/1/2",
			tidyOnly:     "true",
			expectedArgs: []string{"tidy_only"},
		}, {
			description:  "normal execution in root directory with args",
			dirsInTrees:  []string{},
			buildFiles:   []string{},
			args:         []string{"-j", "-k", "fake_module"},
			curDir:       "",
			tidyOnly:     "",
			expectedArgs: []string{"-j", "-k", "fake_module"},
		}}
	for _, tt := range tests {
		t.Run("build action BUILD_MODULES_IN_DIR, "+tt.description, func(t *testing.T) {
			testGetConfigArgs(t, tt, BUILD_MODULES_IN_A_DIRECTORY)
		})
	}
}

func TestGetConfigArgsBuildModulesInDirectories(t *testing.T) {
	tests := []buildActionTestCase{{
		description:  "normal execution in a directory",
		dirsInTrees:  []string{"0/1/2/3.1", "0/1/2/3.2", "0/1/2/3.3"},
		buildFiles:   []string{"0/1/2/3.1/Android.bp", "0/1/2/3.2/Android.bp", "0/1/2/3.3/Android.bp"},
		args:         []string{"3.1/", "3.2/", "3.3/"},
		curDir:       "0/1/2",
		tidyOnly:     "",
		expectedArgs: []string{"MODULES-IN-0-1-2-3.1", "MODULES-IN-0-1-2-3.2", "MODULES-IN-0-1-2-3.3"},
	}, {
		description:  "GET-INSTALL-PATH specified",
		dirsInTrees:  []string{"0/1/2/3.1", "0/1/2/3.2", "0/1/3"},
		buildFiles:   []string{"0/1/2/3.1/Android.bp", "0/1/2/3.2/Android.bp", "0/1/Android.bp"},
		args:         []string{"GET-INSTALL-PATH", "2/3.1/", "2/3.2", "3"},
		curDir:       "0/1",
		tidyOnly:     "",
		expectedArgs: []string{"GET-INSTALL-PATH-IN-0-1-2-3.1", "GET-INSTALL-PATH-IN-0-1-2-3.2", "GET-INSTALL-PATH-IN-0-1"},
	}, {
		description:  "tidy only environment variable specified",
		dirsInTrees:  []string{"0/1/2/3.1", "0/1/2/3.2", "0/1/2/3.3"},
		buildFiles:   []string{"0/1/2/3.1/Android.bp", "0/1/2/3.2/Android.bp", "0/1/2/3.3/Android.bp"},
		args:         []string{"GET-INSTALL-PATH", "3.1/", "3.2/", "3.3"},
		curDir:       "0/1/2",
		tidyOnly:     "1",
		expectedArgs: []string{"tidy_only"},
	}, {
		description:  "normal execution from top dir directory",
		dirsInTrees:  []string{"0/1/2/3.1", "0/1/2/3.2", "0/1/3", "0/2"},
		buildFiles:   []string{"0/1/2/3.1/Android.bp", "0/1/2/3.2/Android.bp", "0/1/3/Android.bp", "0/2/Android.bp"},
		rootSymlink:  false,
		args:         []string{"0/1/2/3.1", "0/1/2/3.2", "0/1/3", "0/2"},
		curDir:       ".",
		tidyOnly:     "",
		expectedArgs: []string{"MODULES-IN-0-1-2-3.1", "MODULES-IN-0-1-2-3.2", "MODULES-IN-0-1-3", "MODULES-IN-0-2"},
	}, {
		description:  "normal execution from top dir directory in symlink",
		dirsInTrees:  []string{"0/1/2/3.1", "0/1/2/3.2", "0/1/3", "0/2"},
		buildFiles:   []string{"0/1/2/3.1/Android.bp", "0/1/2/3.2/Android.bp", "0/1/3/Android.bp", "0/2/Android.bp"},
		rootSymlink:  true,
		args:         []string{"0/1/2/3.1", "0/1/2/3.2", "0/1/3", "0/2"},
		curDir:       ".",
		tidyOnly:     "",
		expectedArgs: []string{"MODULES-IN-0-1-2-3.1", "MODULES-IN-0-1-2-3.2", "MODULES-IN-0-1-3", "MODULES-IN-0-2"},
	}}
	for _, tt := range tests {
		t.Run("build action BUILD_MODULES_IN_DIRS, "+tt.description, func(t *testing.T) {
			testGetConfigArgs(t, tt, BUILD_MODULES_IN_DIRECTORIES)
		})
	}
}
