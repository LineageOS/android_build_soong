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
	// Archives owned by the current target (not by its dependencies). These will
	// be a subset of OutputFiles. (or static libraries, this will be equal to OutputFiles,
	// but general cc_library will also have dynamic libraries in output files).
	RootStaticArchives []string
	// Dynamic libraries (.so files) created by the current target. These will
	// be a subset of OutputFiles. (or shared libraries, this will be equal to OutputFiles,
	// but general cc_library will also have dynamic libraries in output files).
	RootDynamicLibraries []string
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

includes = providers(target)["CcInfo"].compilation_context.includes.to_list()
system_includes = providers(target)["CcInfo"].compilation_context.system_includes.to_list()

ccObjectFiles = []
staticLibraries = []
rootStaticArchives = []
linker_inputs = providers(target)["CcInfo"].linking_context.linker_inputs.to_list()

for linker_input in linker_inputs:
  for library in linker_input.libraries:
    for object in library.objects:
      ccObjectFiles += [object.path]
    if library.static_library:
      staticLibraries.append(library.static_library.path)
      if linker_input.owner == target.label:
        rootStaticArchives.append(library.static_library.path)

rootDynamicLibraries = []

if "@rules_cc//examples:experimental_cc_shared_library.bzl%CcSharedLibraryInfo" in providers(target):
  shared_info = providers(target)["@rules_cc//examples:experimental_cc_shared_library.bzl%CcSharedLibraryInfo"]
  for lib in shared_info.linker_input.libraries:
    rootDynamicLibraries += [lib.dynamic_library.path]

returns = [
  outputFiles,
  staticLibraries,
  ccObjectFiles,
  includes,
  system_includes,
  rootStaticArchives,
  rootDynamicLibraries
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
	if expectedLen := 7; len(splitString) != expectedLen {
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
	rootStaticArchives := splitOrEmpty(splitString[5], ", ")
	rootDynamicLibraries := splitOrEmpty(splitString[6], ", ")
	return CcInfo{
		OutputFiles:          outputFiles,
		CcObjectFiles:        ccObjects,
		CcStaticLibraryFiles: ccStaticLibraries,
		Includes:             includes,
		SystemIncludes:       systemIncludes,
		RootStaticArchives:   rootStaticArchives,
		RootDynamicLibraries: rootDynamicLibraries,
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
