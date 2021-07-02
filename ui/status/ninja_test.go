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

package status

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"android/soong/ui/logger"
)

// Tests that closing the ninja reader when nothing has opened the other end of the fifo is fast.
func TestNinjaReader_Close(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "ninja_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	stat := &Status{}
	nr := NewNinjaReader(logger.New(ioutil.Discard), stat.StartTool(), filepath.Join(tempDir, "fifo"))

	start := time.Now()

	nr.Close()

	if g, w := time.Since(start), NINJA_READER_CLOSE_TIMEOUT; g >= w {
		t.Errorf("nr.Close timed out, %s > %s", g, w)
	}
}

// Test that error hint is added to output if available
func TestNinjaReader_CorrectErrorHint(t *testing.T) {
	errorPattern1 := "pattern-1 in input"
	errorHint1 := "\n Fix by doing task 1"
	errorPattern2 := "pattern-2 in input"
	errorHint2 := "\n Fix by doing task 2"
	mockErrorHints := make(map[string]string)
	mockErrorHints[errorPattern1] = errorHint1
	mockErrorHints[errorPattern2] = errorHint2

	errorHintGenerator := *newErrorHintGenerator(mockErrorHints)
	testCases := []struct {
		rawOutput            string
		buildExitCode        int
		expectedFinalOutput  string
		testCaseErrorMessage string
	}{
		{
			rawOutput:            "ninja build was successful",
			buildExitCode:        0,
			expectedFinalOutput:  "ninja build was successful",
			testCaseErrorMessage: "raw output changed when build was successful",
		},
		{
			rawOutput:            "ninja build failed",
			buildExitCode:        1,
			expectedFinalOutput:  "ninja build failed",
			testCaseErrorMessage: "raw output changed even when no error hint pattern was found",
		},
		{
			rawOutput:            "ninja build failed: " + errorPattern1 + "some footnotes",
			buildExitCode:        1,
			expectedFinalOutput:  "ninja build failed: " + errorPattern1 + "some footnotes" + errorHint1,
			testCaseErrorMessage: "error hint not added despite pattern match",
		},
		{
			rawOutput:            "ninja build failed: " + errorPattern2 + errorPattern1,
			buildExitCode:        1,
			expectedFinalOutput:  "ninja build failed: " + errorPattern2 + errorPattern1 + errorHint2,
			testCaseErrorMessage: "error hint should be added for first pattern match in raw output",
		},
	}
	for _, testCase := range testCases {
		actualFinalOutput := errorHintGenerator.GetOutputWithErrorHint(testCase.rawOutput, testCase.buildExitCode)
		if actualFinalOutput != testCase.expectedFinalOutput {
			t.Errorf(testCase.testCaseErrorMessage+"\nexpected: %s\ngot: %s", testCase.expectedFinalOutput, actualFinalOutput)
		}
	}
}
