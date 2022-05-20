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
	const expectedSplits = 10
	noResult := strings.Repeat("|", expectedSplits-1)
	testCases := []struct {
		description          string
		input                string
		expectedOutput       CcInfo
		expectedErrorMessage string
	}{
		{
			description: "no result",
			input:       noResult,
			expectedOutput: CcInfo{
				OutputFiles:          []string{},
				CcObjectFiles:        []string{},
				CcSharedLibraryFiles: []string{},
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
			input:       "test" + noResult,
			expectedOutput: CcInfo{
				OutputFiles:          []string{"test"},
				CcObjectFiles:        []string{},
				CcSharedLibraryFiles: []string{},
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
			description: "only ToC",
			input:       noResult + "test",
			expectedOutput: CcInfo{
				OutputFiles:          []string{},
				CcObjectFiles:        []string{},
				CcSharedLibraryFiles: []string{},
				CcStaticLibraryFiles: []string{},
				Includes:             []string{},
				SystemIncludes:       []string{},
				Headers:              []string{},
				RootStaticArchives:   []string{},
				RootDynamicLibraries: []string{},
				TocFile:              "test",
			},
		},
		{
			description: "all items set",
			input: "out1, out2" +
				"|object1, object2" +
				"|shared_lib1, shared_lib2" +
				"|static_lib1, static_lib2" +
				"|., dir/subdir" +
				"|system/dir, system/other/dir" +
				"|dir/subdir/hdr.h" +
				"|rootstaticarchive1" +
				"|rootdynamiclibrary1" +
				"|lib.so.toc",
			expectedOutput: CcInfo{
				OutputFiles:          []string{"out1", "out2"},
				CcObjectFiles:        []string{"object1", "object2"},
				CcSharedLibraryFiles: []string{"shared_lib1", "shared_lib2"},
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
			expectedErrorMessage: fmt.Sprintf("expected %d items, got %q", expectedSplits, []string{"", ""}),
		},
		{
			description:          "too many result splits",
			input:                strings.Repeat("|", expectedSplits+1), // 2 too many
			expectedOutput:       CcInfo{},
			expectedErrorMessage: fmt.Sprintf("expected %d items, got %q", expectedSplits, make([]string, expectedSplits+2)),
		},
	}
	for _, tc := range testCases {
		actualOutput, err := GetCcInfo.ParseResult(tc.input)
		if (err == nil && tc.expectedErrorMessage != "") ||
			(err != nil && err.Error() != tc.expectedErrorMessage) {
			t.Errorf("%q:\nexpected Error %s\n, got %s", tc.description, tc.expectedErrorMessage, err)
		} else if err == nil && !reflect.DeepEqual(tc.expectedOutput, actualOutput) {
			t.Errorf("%q:\n expected %#v\n!= actual %#v", tc.description, tc.expectedOutput, actualOutput)
		}
	}
}

func TestGetApexInfoParseResults(t *testing.T) {
	testCases := []struct {
		description    string
		input          string
		expectedOutput ApexCqueryInfo
	}{
		{
			description:    "no result",
			input:          "{}",
			expectedOutput: ApexCqueryInfo{},
		},
		{
			description: "one result",
			input: `{"signed_output":"my.apex",` +
				`"unsigned_output":"my.apex.unsigned",` +
				`"requires_native_libs":["//bionic/libc:libc","//bionic/libdl:libdl"],` +
				`"bundle_key_pair":["foo.pem","foo.privkey"],` +
				`"container_key_pair":["foo.x509.pem", "foo.pk8"],` +
				`"provides_native_libs":[]}`,
			expectedOutput: ApexCqueryInfo{
				SignedOutput:     "my.apex",
				UnsignedOutput:   "my.apex.unsigned",
				RequiresLibs:     []string{"//bionic/libc:libc", "//bionic/libdl:libdl"},
				ProvidesLibs:     []string{},
				BundleKeyPair:    []string{"foo.pem", "foo.privkey"},
				ContainerKeyPair: []string{"foo.x509.pem", "foo.pk8"},
			},
		},
	}
	for _, tc := range testCases {
		actualOutput := GetApexInfo.ParseResult(tc.input)
		if !reflect.DeepEqual(tc.expectedOutput, actualOutput) {
			t.Errorf("%q: expected %#v != actual %#v", tc.description, tc.expectedOutput, actualOutput)
		}
	}
}
