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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"android/soong/ui/build/paths"
)

var tmpDir string
var origPATH string

func TestMain(m *testing.M) {
	os.Exit(func() int {
		var err error
		tmpDir, err = ioutil.TempDir("", "interposer_test")
		if err != nil {
			panic(err)
		}
		defer os.RemoveAll(tmpDir)

		origPATH = os.Getenv("PATH")
		err = os.Setenv("PATH", "")
		if err != nil {
			panic(err)
		}

		return m.Run()
	}())
}

func setup(t *testing.T) string {
	f, err := ioutil.TempFile(tmpDir, "interposer")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	err = ioutil.WriteFile(f.Name()+"_origpath", []byte(origPATH), 0666)
	if err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestInterposer(t *testing.T) {
	interposer := setup(t)

	logConfig := func(name string) paths.PathConfig {
		if name == "true" {
			return paths.PathConfig{
				Log:   false,
				Error: false,
			}
		} else if name == "path_interposer_test_not_allowed" {
			return paths.PathConfig{
				Log:   false,
				Error: true,
			}
		}
		return paths.PathConfig{
			Log:   true,
			Error: false,
		}
	}

	testCases := []struct {
		name string
		args []string

		exitCode int
		err      error
		logEntry string
	}{
		{
			name: "direct call",
			args: []string{interposer},

			exitCode: 1,
			err:      usage,
		},
		{
			name: "relative call",
			args: []string{filepath.Base(interposer)},

			exitCode: 1,
			err:      usage,
		},
		{
			name: "true",
			args: []string{"/my/path/true"},
		},
		{
			name: "relative true",
			args: []string{"true"},
		},
		{
			name: "exit code",
			args: []string{"bash", "-c", "exit 42"},

			exitCode: 42,
			logEntry: "bash",
		},
		{
			name: "signal",
			args: []string{"bash", "-c", "kill -9 $$"},

			exitCode: 137,
			logEntry: "bash",
		},
		{
			name: "does not exist",
			args: []string{"path_interposer_test_does_not_exist"},

			exitCode: 1,
			err:      fmt.Errorf(`exec: "path_interposer_test_does_not_exist": executable file not found in $PATH`),
			logEntry: "path_interposer_test_does_not_exist",
		},
		{
			name: "not allowed",
			args: []string{"path_interposer_test_not_allowed"},

			exitCode: 1,
			err:      fmt.Errorf(`"path_interposer_test_not_allowed" is not allowed to be used. See https://android.googlesource.com/platform/build/+/main/Changes.md#PATH_Tools for more information.`),
			logEntry: "path_interposer_test_not_allowed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			logged := false
			logFunc := func(logSocket string, entry *paths.LogEntry, done chan interface{}) {
				defer close(done)

				logged = true
				if entry.Basename != testCase.logEntry {
					t.Errorf("unexpected log entry:\nwant: %q\n got: %q", testCase.logEntry, entry.Basename)
				}
			}

			exitCode, err := Main(ioutil.Discard, ioutil.Discard, interposer, testCase.args, mainOpts{
				sendLog: logFunc,
				config:  logConfig,
			})

			errstr := func(err error) string {
				if err == nil {
					return ""
				}
				return err.Error()
			}
			if errstr(testCase.err) != errstr(err) {
				t.Errorf("unexpected error:\nwant: %v\n got: %v", testCase.err, err)
			}
			if testCase.exitCode != exitCode {
				t.Errorf("expected exit code %d, got %d", testCase.exitCode, exitCode)
			}
			if !logged && testCase.logEntry != "" {
				t.Errorf("no log entry, but expected %q", testCase.logEntry)
			}
		})
	}
}

func TestMissingPath(t *testing.T) {
	interposer := setup(t)
	err := os.Remove(interposer + "_origpath")
	if err != nil {
		t.Fatal("Failed to remove:", err)
	}

	exitCode, err := Main(ioutil.Discard, ioutil.Discard, interposer, []string{"true"}, mainOpts{})
	if err != usage {
		t.Errorf("Unexpected error:\n got: %v\nwant: %v", err, usage)
	}
	if exitCode != 1 {
		t.Errorf("expected exit code %d, got %d", 1, exitCode)
	}
}
