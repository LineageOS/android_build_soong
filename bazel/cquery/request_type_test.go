package cquery

import (
	"encoding/json"
	"reflect"
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
		description    string
		inputCcInfo    CcInfo
		expectedOutput CcInfo
	}{
		{
			description:    "no result",
			inputCcInfo:    CcInfo{},
			expectedOutput: CcInfo{},
		},
		{
			description: "only output",
			inputCcInfo: CcInfo{
				OutputFiles: []string{"test", "test3"},
			},
			expectedOutput: CcInfo{
				OutputFiles: []string{"test", "test3"},
			},
		},
		{
			description: "only ToC",
			inputCcInfo: CcInfo{
				TocFile: "test",
			},
			expectedOutput: CcInfo{
				TocFile: "test",
			},
		},
		{
			description: "all items set",
			inputCcInfo: CcInfo{
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
	}
	for _, tc := range testCases {
		jsonInput, _ := json.Marshal(tc.inputCcInfo)
		actualOutput, err := GetCcInfo.ParseResult(string(jsonInput))
		if err != nil {
			t.Errorf("%q:\n test case get error: %q", tc.description, err)
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
