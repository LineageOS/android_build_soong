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

package main

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"android/soong/third_party/zip"
)

var testCases = []struct {
	name string

	inputFiles []string
	sortGlobs  bool
	sortJava   bool
	args       []string
	excludes   []string

	outputFiles []string
	err         error
}{
	{
		name: "unsupported \\",

		args: []string{"a\\b:b"},

		err: fmt.Errorf("\\ characters are not currently supported"),
	},
	{ // This is modelled after the update package build rules in build/make/core/Makefile
		name: "filter globs",

		inputFiles: []string{
			"RADIO/a",
			"IMAGES/system.img",
			"IMAGES/b.txt",
			"IMAGES/recovery.img",
			"IMAGES/vendor.img",
			"OTA/android-info.txt",
			"OTA/b",
		},
		args: []string{"OTA/android-info.txt:android-info.txt", "IMAGES/*.img:."},

		outputFiles: []string{
			"android-info.txt",
			"system.img",
			"recovery.img",
			"vendor.img",
		},
	},
	{
		name: "sorted filter globs",

		inputFiles: []string{
			"RADIO/a",
			"IMAGES/system.img",
			"IMAGES/b.txt",
			"IMAGES/recovery.img",
			"IMAGES/vendor.img",
			"OTA/android-info.txt",
			"OTA/b",
		},
		sortGlobs: true,
		args:      []string{"IMAGES/*.img:.", "OTA/android-info.txt:android-info.txt"},

		outputFiles: []string{
			"recovery.img",
			"system.img",
			"vendor.img",
			"android-info.txt",
		},
	},
	{
		name: "sort all",

		inputFiles: []string{
			"RADIO/",
			"RADIO/a",
			"IMAGES/",
			"IMAGES/system.img",
			"IMAGES/b.txt",
			"IMAGES/recovery.img",
			"IMAGES/vendor.img",
			"OTA/",
			"OTA/b",
			"OTA/android-info.txt",
		},
		sortGlobs: true,
		args:      []string{"**/*"},

		outputFiles: []string{
			"IMAGES/b.txt",
			"IMAGES/recovery.img",
			"IMAGES/system.img",
			"IMAGES/vendor.img",
			"OTA/android-info.txt",
			"OTA/b",
			"RADIO/a",
		},
	},
	{
		name: "sort all implicit",

		inputFiles: []string{
			"RADIO/",
			"RADIO/a",
			"IMAGES/",
			"IMAGES/system.img",
			"IMAGES/b.txt",
			"IMAGES/recovery.img",
			"IMAGES/vendor.img",
			"OTA/",
			"OTA/b",
			"OTA/android-info.txt",
		},
		sortGlobs: true,
		args:      nil,

		outputFiles: []string{
			"IMAGES/",
			"IMAGES/b.txt",
			"IMAGES/recovery.img",
			"IMAGES/system.img",
			"IMAGES/vendor.img",
			"OTA/",
			"OTA/android-info.txt",
			"OTA/b",
			"RADIO/",
			"RADIO/a",
		},
	},
	{
		name: "sort jar",

		inputFiles: []string{
			"MANIFEST.MF",
			"META-INF/MANIFEST.MF",
			"META-INF/aaa/",
			"META-INF/aaa/aaa",
			"META-INF/AAA",
			"META-INF.txt",
			"META-INF/",
			"AAA",
			"aaa",
		},
		sortJava: true,
		args:     nil,

		outputFiles: []string{
			"META-INF/",
			"META-INF/MANIFEST.MF",
			"META-INF/AAA",
			"META-INF/aaa/",
			"META-INF/aaa/aaa",
			"AAA",
			"MANIFEST.MF",
			"META-INF.txt",
			"aaa",
		},
	},
	{
		name: "double input",

		inputFiles: []string{
			"b",
			"a",
		},
		args: []string{"a:a2", "**/*"},

		outputFiles: []string{
			"a2",
			"b",
			"a",
		},
	},
	{
		name: "multiple matches",

		inputFiles: []string{
			"a/a",
		},
		args: []string{"a/a", "a/*"},

		outputFiles: []string{
			"a/a",
		},
	},
	{
		name: "multiple conflicting matches",

		inputFiles: []string{
			"a/a",
			"a/b",
		},
		args: []string{"a/b:a/a", "a/*"},

		err: fmt.Errorf(`multiple entries for "a/a" with different contents`),
	},
	{
		name: "excludes",

		inputFiles: []string{
			"a/a",
			"a/b",
		},
		args:     nil,
		excludes: []string{"a/a"},

		outputFiles: []string{
			"a/b",
		},
	},
	{
		name: "excludes with include",

		inputFiles: []string{
			"a/a",
			"a/b",
		},
		args:     []string{"a/*"},
		excludes: []string{"a/a"},

		outputFiles: []string{
			"a/b",
		},
	},
	{
		name: "excludes with glob",

		inputFiles: []string{
			"a/a",
			"a/b",
		},
		args:     []string{"a/*"},
		excludes: []string{"a/*"},

		outputFiles: nil,
	},
}

func errorString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

func TestZip2Zip(t *testing.T) {
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			inputBuf := &bytes.Buffer{}
			outputBuf := &bytes.Buffer{}

			inputWriter := zip.NewWriter(inputBuf)
			for _, file := range testCase.inputFiles {
				w, err := inputWriter.Create(file)
				if err != nil {
					t.Fatal(err)
				}
				fmt.Fprintln(w, "test")
			}
			inputWriter.Close()
			inputBytes := inputBuf.Bytes()
			inputReader, err := zip.NewReader(bytes.NewReader(inputBytes), int64(len(inputBytes)))
			if err != nil {
				t.Fatal(err)
			}

			outputWriter := zip.NewWriter(outputBuf)
			err = zip2zip(inputReader, outputWriter, testCase.sortGlobs, testCase.sortJava, false, testCase.args, testCase.excludes)
			if errorString(testCase.err) != errorString(err) {
				t.Fatalf("Unexpected error:\n got: %q\nwant: %q", errorString(err), errorString(testCase.err))
			}

			outputWriter.Close()
			outputBytes := outputBuf.Bytes()
			outputReader, err := zip.NewReader(bytes.NewReader(outputBytes), int64(len(outputBytes)))
			if err != nil {
				t.Fatal(err)
			}
			var outputFiles []string
			if len(outputReader.File) > 0 {
				outputFiles = make([]string, len(outputReader.File))
				for i, file := range outputReader.File {
					outputFiles[i] = file.Name
				}
			}

			if !reflect.DeepEqual(testCase.outputFiles, outputFiles) {
				t.Fatalf("Output file list does not match:\n got: %v\nwant: %v", outputFiles, testCase.outputFiles)
			}
		})
	}
}
