package cquery

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestGetOutputFilesParseResults(t *testing.T) {
	testCases := []struct {
		description    string
		input          string
		expectedOutput []string
	}{
		{
			description:    "no result",
			input:          "",
			expectedOutput: []string{},
		},
		{
			description:    "one result",
			input:          "test",
			expectedOutput: []string{"test"},
		},
		{
			description:    "splits on comma with space",
			input:          "foo, bar",
			expectedOutput: []string{"foo", "bar"},
		},
	}
	for _, tc := range testCases {
		actualOutput := GetOutputFiles.ParseResult(tc.input)
		if !reflect.DeepEqual(tc.expectedOutput, actualOutput) {
			t.Errorf("%q: expected %#v != actual %#v", tc.description, tc.expectedOutput, actualOutput)
		}
	}
}

func TestGetPythonBinaryParseResults(t *testing.T) {
	testCases := []struct {
		description    string
		input          string
		expectedOutput string
	}{
		{
			description:    "no result",
			input:          "",
			expectedOutput: "",
		},
		{
			description:    "one result",
			input:          "test",
			expectedOutput: "test",
		},
	}
	for _, tc := range testCases {
		actualOutput := GetPythonBinary.ParseResult(tc.input)
		if !reflect.DeepEqual(tc.expectedOutput, actualOutput) {
			t.Errorf("%q: expected %#v != actual %#v", tc.description, tc.expectedOutput, actualOutput)
		}
	}
}

func TestGetCcInfoParseResults(t *testing.T) {
	testCases := []struct {
		description          string
		input                string
		expectedOutput       CcInfo
		expectedErrorMessage string
	}{
		{
			description: "no result",
			input:       "||||||||",
			expectedOutput: CcInfo{
				OutputFiles:          []string{},
				CcObjectFiles:        []string{},
				CcStaticLibraryFiles: []string{},
				Includes:             []string{},
				SystemIncludes:       []string{},
				Headers:              []string{},
				RootStaticArchives:   []string{},
				RootDynamicLibraries: []string{},
				TocFile:              "",
			},
		},
		{
			description: "only output",
			input:       "test||||||||",
			expectedOutput: CcInfo{
				OutputFiles:          []string{"test"},
				CcObjectFiles:        []string{},
				CcStaticLibraryFiles: []string{},
				Includes:             []string{},
				SystemIncludes:       []string{},
				Headers:              []string{},
				RootStaticArchives:   []string{},
				RootDynamicLibraries: []string{},
				TocFile:              "",
			},
		},
		{
			description: "all items set",
			input:       "out1, out2|static_lib1, static_lib2|object1, object2|., dir/subdir|system/dir, system/other/dir|dir/subdir/hdr.h|rootstaticarchive1|rootdynamiclibrary1|lib.so.toc",
			expectedOutput: CcInfo{
				OutputFiles:          []string{"out1", "out2"},
				CcObjectFiles:        []string{"object1", "object2"},
				CcStaticLibraryFiles: []string{"static_lib1", "static_lib2"},
				Includes:             []string{".", "dir/subdir"},
				SystemIncludes:       []string{"system/dir", "system/other/dir"},
				Headers:              []string{"dir/subdir/hdr.h"},
				RootStaticArchives:   []string{"rootstaticarchive1"},
				RootDynamicLibraries: []string{"rootdynamiclibrary1"},
				TocFile:              "lib.so.toc",
			},
		},
		{
			description:          "too few result splits",
			input:                "|",
			expectedOutput:       CcInfo{},
			expectedErrorMessage: fmt.Sprintf("Expected %d items, got %q", 9, []string{"", ""}),
		},
		{
			description:          "too many result splits",
			input:                strings.Repeat("|", 50),
			expectedOutput:       CcInfo{},
			expectedErrorMessage: fmt.Sprintf("Expected %d items, got %q", 9, make([]string, 51)),
		},
	}
	for _, tc := range testCases {
		actualOutput, err := GetCcInfo.ParseResult(tc.input)
		if (err == nil && tc.expectedErrorMessage != "") ||
			(err != nil && err.Error() != tc.expectedErrorMessage) {
			t.Errorf("%q: expected Error %s, got %s", tc.description, tc.expectedErrorMessage, err)
		} else if err == nil && !reflect.DeepEqual(tc.expectedOutput, actualOutput) {
			t.Errorf("%q: expected %#v != actual %#v", tc.description, tc.expectedOutput, actualOutput)
		}
	}
}
