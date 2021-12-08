package cquery

import (
	"fmt"
	"strings"
)

var (
	GetOutputFiles  = &getOutputFilesRequestType{}
	GetPythonBinary = &getPythonBinaryRequestType{}
	GetCcInfo       = &getCcInfoType{}
)

type CcInfo struct {
	OutputFiles          []string
	CcObjectFiles        []string
	CcStaticLibraryFiles []string
	Includes             []string
	SystemIncludes       []string
	Headers              []string
	// Archives owned by the current target (not by its dependencies). These will
	// be a subset of OutputFiles. (or static libraries, this will be equal to OutputFiles,
	// but general cc_library will also have dynamic libraries in output files).
	RootStaticArchives []string
	// Dynamic libraries (.so files) created by the current target. These will
	// be a subset of OutputFiles. (or shared libraries, this will be equal to OutputFiles,
	// but general cc_library will also have dynamic libraries in output files).
	RootDynamicLibraries []string
	TocFile              string
}

type getOutputFilesRequestType struct{}

type getPythonBinaryRequestType struct{}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getOutputFilesRequestType) Name() string {
	return "getOutputFiles"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information.
// The function should have the following properties:
//   - `target` is the only parameter to this function (a configured target).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getOutputFilesRequestType) StarlarkFunctionBody() string {
	return "return ', '.join([f.path for f in target.files.to_list()])"
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getOutputFilesRequestType) ParseResult(rawString string) []string {
	return splitOrEmpty(rawString, ", ")
}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getPythonBinaryRequestType) Name() string {
	return "getPythonBinary"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information.
// The function should have the following properties:
//   - `target` is the only parameter to this function (a configured target).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getPythonBinaryRequestType) StarlarkFunctionBody() string {
	return "return providers(target)['FilesToRunProvider'].executable.path"
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getPythonBinaryRequestType) ParseResult(rawString string) string {
	return rawString
}

type getCcInfoType struct{}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getCcInfoType) Name() string {
	return "getCcInfo"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information.
// The function should have the following properties:
//   - `target` is the only parameter to this function (a configured target).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getCcInfoType) StarlarkFunctionBody() string {
	return `
outputFiles = [f.path for f in target.files.to_list()]
cc_info = providers(target)["CcInfo"]

includes = cc_info.compilation_context.includes.to_list()
system_includes = cc_info.compilation_context.system_includes.to_list()
headers = [f.path for f in cc_info.compilation_context.headers.to_list()]

ccObjectFiles = []
staticLibraries = []
rootStaticArchives = []
linker_inputs = cc_info.linking_context.linker_inputs.to_list()

static_info_tag = "//build/bazel/rules:cc_library_static.bzl%CcStaticLibraryInfo"
if static_info_tag in providers(target):
  static_info = providers(target)[static_info_tag]
  ccObjectFiles = [f.path for f in static_info.objects]
  rootStaticArchives = [static_info.root_static_archive.path]
else:
  for linker_input in linker_inputs:
    for library in linker_input.libraries:
      for object in library.objects:
        ccObjectFiles += [object.path]
      if library.static_library:
        staticLibraries.append(library.static_library.path)
        if linker_input.owner == target.label:
          rootStaticArchives.append(library.static_library.path)

rootDynamicLibraries = []

shared_info_tag = "@rules_cc//examples:experimental_cc_shared_library.bzl%CcSharedLibraryInfo"
if shared_info_tag in providers(target):
  shared_info = providers(target)[shared_info_tag]
  for lib in shared_info.linker_input.libraries:
    rootDynamicLibraries += [lib.dynamic_library.path]

toc_file = ""
toc_file_tag = "//build/bazel/rules:generate_toc.bzl%CcTocInfo"
if toc_file_tag in providers(target):
  toc_file = providers(target)[toc_file_tag].toc.path

returns = [
  outputFiles,
  staticLibraries,
  ccObjectFiles,
  includes,
  system_includes,
  headers,
  rootStaticArchives,
  rootDynamicLibraries,
  [toc_file]
]

return "|".join([", ".join(r) for r in returns])`
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getCcInfoType) ParseResult(rawString string) (CcInfo, error) {
	var outputFiles []string
	var ccObjects []string

	splitString := strings.Split(rawString, "|")
	if expectedLen := 9; len(splitString) != expectedLen {
		return CcInfo{}, fmt.Errorf("Expected %d items, got %q", expectedLen, splitString)
	}
	outputFilesString := splitString[0]
	ccStaticLibrariesString := splitString[1]
	ccObjectsString := splitString[2]
	outputFiles = splitOrEmpty(outputFilesString, ", ")
	ccStaticLibraries := splitOrEmpty(ccStaticLibrariesString, ", ")
	ccObjects = splitOrEmpty(ccObjectsString, ", ")
	includes := splitOrEmpty(splitString[3], ", ")
	systemIncludes := splitOrEmpty(splitString[4], ", ")
	headers := splitOrEmpty(splitString[5], ", ")
	rootStaticArchives := splitOrEmpty(splitString[6], ", ")
	rootDynamicLibraries := splitOrEmpty(splitString[7], ", ")
	tocFile := splitString[8] // NOTE: Will be the empty string if there wasn't
	return CcInfo{
		OutputFiles:          outputFiles,
		CcObjectFiles:        ccObjects,
		CcStaticLibraryFiles: ccStaticLibraries,
		Includes:             includes,
		SystemIncludes:       systemIncludes,
		Headers:              headers,
		RootStaticArchives:   rootStaticArchives,
		RootDynamicLibraries: rootDynamicLibraries,
		TocFile:              tocFile,
	}, nil
}

// splitOrEmpty is a modification of strings.Split() that returns an empty list
// if the given string is empty.
func splitOrEmpty(s string, sep string) []string {
	if len(s) < 1 {
		return []string{}
	} else {
		return strings.Split(s, sep)
	}
}
