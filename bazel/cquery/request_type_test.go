package cquery

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestGetOutputFilesParseResults(t *testing.T) {
	t.Parallel()
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
		t.Run(tc.description, func(t *testing.T) {
			actualOutput := GetOutputFiles.ParseResult(tc.input)
			if !reflect.DeepEqual(tc.expectedOutput, actualOutput) {
				t.Errorf("expected %#v != actual %#v", tc.expectedOutput, actualOutput)
			}
		})
	}
}

func TestGetCcInfoParseResults(t *testing.T) {
	t.Parallel()
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
		t.Run(tc.description, func(t *testing.T) {
			jsonInput, _ := json.Marshal(tc.inputCcInfo)
			actualOutput, err := GetCcInfo.ParseResult(string(jsonInput))
			if err != nil {
				t.Errorf("error parsing result: %q", err)
			} else if err == nil && !reflect.DeepEqual(tc.expectedOutput, actualOutput) {
				t.Errorf("expected %#v\n!= actual %#v", tc.expectedOutput, actualOutput)
			}
		})
	}
}

func TestGetCcInfoParseResultsError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description   string
		input         string
		expectedError string
	}{
		{
			description:   "not json",
			input:         ``,
			expectedError: `cannot parse cquery result '': EOF`,
		},
		{
			description: "invalid field",
			input: `{
	"toc_file": "dir/file.so.toc"
}`,
			expectedError: `json: unknown field "toc_file"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			_, err := GetCcInfo.ParseResult(tc.input)
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("expected string %q in error message, got %q", tc.expectedError, err)
			}
		})
	}
}

func TestGetApexInfoParseResults(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description    string
		input          string
		expectedOutput ApexInfo
	}{
		{
			description:    "no result",
			input:          "{}",
			expectedOutput: ApexInfo{},
		},
		{
			description: "one result",
			input: `{
	"signed_output":"my.apex",
	"unsigned_output":"my.apex.unsigned",
	"requires_native_libs":["//bionic/libc:libc","//bionic/libdl:libdl"],
	"bundle_key_info":["foo.pem", "foo.privkey"],
	"container_key_info":["foo.x509.pem", "foo.pk8", "foo"],
	"package_name":"package.name",
	"symbols_used_by_apex": "path/to/my.apex_using.txt",
	"backing_libs":"path/to/backing.txt",
	"bundle_file": "dir/bundlefile.zip",
	"installed_files":"path/to/installed-files.txt",
	"provides_native_libs":[],
	"make_modules_to_install": ["foo","bar"]
}`,
			expectedOutput: ApexInfo{
				// ApexInfo
				SignedOutput:      "my.apex",
				UnsignedOutput:    "my.apex.unsigned",
				RequiresLibs:      []string{"//bionic/libc:libc", "//bionic/libdl:libdl"},
				ProvidesLibs:      []string{},
				BundleKeyInfo:     []string{"foo.pem", "foo.privkey"},
				ContainerKeyInfo:  []string{"foo.x509.pem", "foo.pk8", "foo"},
				PackageName:       "package.name",
				SymbolsUsedByApex: "path/to/my.apex_using.txt",
				BackingLibs:       "path/to/backing.txt",
				BundleFile:        "dir/bundlefile.zip",
				InstalledFiles:    "path/to/installed-files.txt",

				// ApexMkInfo
				MakeModulesToInstall: []string{"foo", "bar"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actualOutput, err := GetApexInfo.ParseResult(tc.input)
			if err != nil {
				t.Errorf("Unexpected error %q", err)
			}
			if !reflect.DeepEqual(tc.expectedOutput, actualOutput) {
				t.Errorf("expected %#v != actual %#v", tc.expectedOutput, actualOutput)
			}
		})
	}
}

func TestGetApexInfoParseResultsError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description   string
		input         string
		expectedError string
	}{
		{
			description:   "not json",
			input:         ``,
			expectedError: `cannot parse cquery result '': EOF`,
		},
		{
			description: "invalid field",
			input: `{
	"fake_field": "path/to/file"
}`,
			expectedError: `json: unknown field "fake_field"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			_, err := GetApexInfo.ParseResult(tc.input)
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("expected string %q in error message, got %q", tc.expectedError, err)
			}
		})
	}
}

func TestGetCcUnstrippedParseResults(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description    string
		input          string
		expectedOutput CcUnstrippedInfo
	}{
		{
			description:    "no result",
			input:          "{}",
			expectedOutput: CcUnstrippedInfo{},
		},
		{
			description: "one result",
			input:       `{"OutputFile":"myapp", "UnstrippedOutput":"myapp_unstripped"}`,
			expectedOutput: CcUnstrippedInfo{
				OutputFile:       "myapp",
				UnstrippedOutput: "myapp_unstripped",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actualOutput, err := GetCcUnstrippedInfo.ParseResult(tc.input)
			if err != nil {
				t.Errorf("Unexpected error %q", err)
			}
			if !reflect.DeepEqual(tc.expectedOutput, actualOutput) {
				t.Errorf("expected %#v != actual %#v", tc.expectedOutput, actualOutput)
			}
		})
	}
}

func TestGetCcUnstrippedParseResultsErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description   string
		input         string
		expectedError string
	}{
		{
			description:   "not json",
			input:         ``,
			expectedError: `cannot parse cquery result '': EOF`,
		},
		{
			description: "invalid field",
			input: `{
	"fake_field": "path/to/file"
}`,
			expectedError: `json: unknown field "fake_field"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			_, err := GetCcUnstrippedInfo.ParseResult(tc.input)
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("expected string %q in error message, got %q", tc.expectedError, err)
			}
		})
	}
}
