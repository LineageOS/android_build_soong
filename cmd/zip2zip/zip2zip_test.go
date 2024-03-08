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

	inputFiles   []string
	sortGlobs    bool
	sortJava     bool
	args         []string
	excludes     []string
	includes     []string
	uncompresses []string

	outputFiles []string
	storedFiles []string
	err         error
}{
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
		name: "excludes with args",

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
		name: "excludes over args",

		inputFiles: []string{
			"a/a",
			"a/b",
		},
		args:     []string{"a/a"},
		excludes: []string{"a/*"},

		outputFiles: nil,
	},
	{
		name: "excludes with includes",

		inputFiles: []string{
			"a/a",
			"a/b",
		},
		args:     nil,
		excludes: []string{"a/*"},
		includes: []string{"a/b"},

		outputFiles: []string{"a/b"},
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
	{
		name: "uncompress one",

		inputFiles: []string{
			"a/a",
			"a/b",
		},
		uncompresses: []string{"a/a"},

		outputFiles: []string{
			"a/a",
			"a/b",
		},
		storedFiles: []string{
			"a/a",
		},
	},
	{
		name: "uncompress two",

		inputFiles: []string{
			"a/a",
			"a/b",
		},
		uncompresses: []string{"a/a", "a/b"},

		outputFiles: []string{
			"a/a",
			"a/b",
		},
		storedFiles: []string{
			"a/a",
			"a/b",
		},
	},
	{
		name: "uncompress glob",

		inputFiles: []string{
			"a/a",
			"a/b",
			"a/c.so",
			"a/d.so",
		},
		uncompresses: []string{"a/*.so"},

		outputFiles: []string{
			"a/a",
			"a/b",
			"a/c.so",
			"a/d.so",
		},
		storedFiles: []string{
			"a/c.so",
			"a/d.so",
		},
	},
	{
		name: "uncompress rename",

		inputFiles: []string{
			"a/a",
		},
		args:         []string{"a/a:a/b"},
		uncompresses: []string{"a/b"},

		outputFiles: []string{
			"a/b",
		},
		storedFiles: []string{
			"a/b",
		},
	},
	{
		name: "recursive glob",

		inputFiles: []string{
			"a/a/a",
			"a/a/b",
		},
		args: []string{"a/**/*:b"},
		outputFiles: []string{
			"b/a/a",
			"b/a/b",
		},
	},
	{
		name: "glob",

		inputFiles: []string{
			"a/a/a",
			"a/a/b",
			"a/b",
			"a/c",
		},
		args: []string{"a/*:b"},
		outputFiles: []string{
			"b/b",
			"b/c",
		},
	},
	{
		name: "top level glob",

		inputFiles: []string{
			"a",
			"b",
		},
		args: []string{"*:b"},
		outputFiles: []string{
			"b/a",
			"b/b",
		},
	},
	{
		name: "multilple glob",

		inputFiles: []string{
			"a/a/a",
			"a/a/b",
		},
		args: []string{"a/*/*:b"},
		outputFiles: []string{
			"b/a/a",
			"b/a/b",
		},
	},
	{
		name: "escaping",

		inputFiles:  []string{"a"},
		args:        []string{"\\a"},
		outputFiles: []string{"a"},
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
			err = zip2zip(inputReader, outputWriter, testCase.sortGlobs, testCase.sortJava, false,
				testCase.args, testCase.excludes, testCase.includes, testCase.uncompresses)
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
			var storedFiles []string
			if len(outputReader.File) > 0 {
				outputFiles = make([]string, len(outputReader.File))
				for i, file := range outputReader.File {
					outputFiles[i] = file.Name
					if file.Method == zip.Store {
						storedFiles = append(storedFiles, file.Name)
					}
				}
			}

			if !reflect.DeepEqual(testCase.outputFiles, outputFiles) {
				t.Fatalf("Output file list does not match:\nwant: %v\n got: %v", testCase.outputFiles, outputFiles)
			}
			if !reflect.DeepEqual(testCase.storedFiles, storedFiles) {
				t.Fatalf("Stored file list does not match:\nwant: %v\n got: %v", testCase.storedFiles, storedFiles)
			}
		})
	}
}

// TestZip2Zip64 tests that zip2zip on zip file larger than 4GB produces a valid zip file.
func TestZip2Zip64(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}
	inputBuf := &bytes.Buffer{}
	outputBuf := &bytes.Buffer{}

	inputWriter := zip.NewWriter(inputBuf)
	w, err := inputWriter.CreateHeaderAndroid(&zip.FileHeader{
		Name:   "a",
		Method: zip.Store,
	})
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4*1024*1024)
	for i := 0; i < 1025; i++ {
		w.Write(buf)
	}
	w, err = inputWriter.CreateHeaderAndroid(&zip.FileHeader{
		Name:   "b",
		Method: zip.Store,
	})
	for i := 0; i < 1025; i++ {
		w.Write(buf)
	}
	inputWriter.Close()
	inputBytes := inputBuf.Bytes()

	inputReader, err := zip.NewReader(bytes.NewReader(inputBytes), int64(len(inputBytes)))
	if err != nil {
		t.Fatal(err)
	}

	outputWriter := zip.NewWriter(outputBuf)
	err = zip2zip(inputReader, outputWriter, false, false, false,
		nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	outputWriter.Close()
	outputBytes := outputBuf.Bytes()
	_, err = zip.NewReader(bytes.NewReader(outputBytes), int64(len(outputBytes)))
	if err != nil {
		t.Fatal(err)
	}
}

func TestConstantPartOfPattern(t *testing.T) {
	testCases := []struct{ in, out string }{
		{
			in:  "",
			out: "",
		},
		{
			in:  "a",
			out: "a",
		},
		{
			in:  "*",
			out: "",
		},
		{
			in:  "a/a",
			out: "a/a",
		},
		{
			in:  "a/*",
			out: "a",
		},
		{
			in:  "a/*/a",
			out: "a",
		},
		{
			in:  "a/**/*",
			out: "a",
		},
	}

	for _, test := range testCases {
		t.Run(test.in, func(t *testing.T) {
			got := constantPartOfPattern(test.in)
			if got != test.out {
				t.Errorf("want %q, got %q", test.out, got)
			}
		})
	}
}
