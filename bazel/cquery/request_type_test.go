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

func TestGetCcInfoParseResults(t *testing.T) {
	testCases := []struct {
		description          string
		input                string
		expectedOutput       CcInfo
		expectedErrorMessage string
	}{
		{
			description: "no result",
			input:       "||||",
			expectedOutput: CcInfo{
				OutputFiles:          []string{},
				CcObjectFiles:        []string{},
				CcStaticLibraryFiles: []string{},
				Includes:             []string{},
				SystemIncludes:       []string{},
			},
		},
		{
			description: "only output",
			input:       "test||||",
			expectedOutput: CcInfo{
				OutputFiles:          []string{"test"},
				CcObjectFiles:        []string{},
				CcStaticLibraryFiles: []string{},
				Includes:             []string{},
				SystemIncludes:       []string{},
			},
		},
		{
			description: "all items set",
			input:       "out1, out2|static_lib1, static_lib2|object1, object2|., dir/subdir|system/dir, system/other/dir",
			expectedOutput: CcInfo{
				OutputFiles:          []string{"out1", "out2"},
				CcObjectFiles:        []string{"object1", "object2"},
				CcStaticLibraryFiles: []string{"static_lib1", "static_lib2"},
				Includes:             []string{".", "dir/subdir"},
				SystemIncludes:       []string{"system/dir", "system/other/dir"},
			},
		},
		{
			description:          "too few result splits",
			input:                "|",
			expectedOutput:       CcInfo{},
			expectedErrorMessage: fmt.Sprintf("Expected %d items, got %q", 5, []string{"", ""}),
		},
		{
			description:          "too many result splits",
			input:                strings.Repeat("|", 8),
			expectedOutput:       CcInfo{},
			expectedErrorMessage: fmt.Sprintf("Expected %d items, got %q", 5, make([]string, 9)),
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
