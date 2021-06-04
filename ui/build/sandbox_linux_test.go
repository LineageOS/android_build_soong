// Copyright 2021 Google Inc. All rights reserved.
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
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// set src dir of sandbox
	sandboxConfig.srcDir = "/my/src/dir"
	os.Exit(m.Run())
}

func TestMountFlagsSrcDir(t *testing.T) {
	testCases := []struct {
		srcDirIsRO         bool
		expectedSrcDirFlag string
	}{
		{
			srcDirIsRO:         false,
			expectedSrcDirFlag: "-B",
		},
		{
			srcDirIsRO:         true,
			expectedSrcDirFlag: "-R",
		},
	}
	for _, testCase := range testCases {
		c := testCmd()
		c.config.sandboxConfig.SetSrcDirIsRO(testCase.srcDirIsRO)
		c.wrapSandbox()
		if !isExpectedMountFlag(c.Args, sandboxConfig.srcDir, testCase.expectedSrcDirFlag) {
			t.Error("Mount flag of srcDir is not correct")
		}
	}
}

func TestMountFlagsSrcDirRWAllowlist(t *testing.T) {
	testCases := []struct {
		srcDirRWAllowlist []string
	}{
		{
			srcDirRWAllowlist: []string{},
		},
		{
			srcDirRWAllowlist: []string{"my/path"},
		},
		{
			srcDirRWAllowlist: []string{"my/path1", "my/path2"},
		},
	}
	for _, testCase := range testCases {
		c := testCmd()
		c.config.sandboxConfig.SetSrcDirIsRO(true)
		c.config.sandboxConfig.SetSrcDirRWAllowlist(testCase.srcDirRWAllowlist)
		c.wrapSandbox()
		for _, allowlistPath := range testCase.srcDirRWAllowlist {
			if !isExpectedMountFlag(c.Args, allowlistPath, "-B") {
				t.Error("Mount flag of srcDirRWAllowlist is not correct, expect -B")
			}
		}
	}
}

// utils for setting up test
func testConfig() Config {
	// create a minimal testConfig
	env := Environment([]string{})
	sandboxConfig := SandboxConfig{}
	return Config{&configImpl{environ: &env,
		sandboxConfig: &sandboxConfig}}
}

func testCmd() *Cmd {
	return Command(testContext(), testConfig(), "sandbox_test", "path/to/nsjail")
}

func isExpectedMountFlag(cmdArgs []string, dirName string, expectedFlag string) bool {
	indexOfSrcDir := index(cmdArgs, dirName)
	return cmdArgs[indexOfSrcDir-1] == expectedFlag
}

func index(arr []string, target string) int {
	for idx, element := range arr {
		if element == target {
			return idx
		}
	}
	panic("element could not be located in input array")
}
