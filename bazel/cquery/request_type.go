package cquery

import (
	"encoding/json"
	"fmt"
	"strings"
)

var (
	GetOutputFiles  = &getOutputFilesRequestType{}
	GetPythonBinary = &getPythonBinaryRequestType{}
	GetCcInfo       = &getCcInfoType{}
	GetApexInfo     = &getApexInfoType{}
)

type CcInfo struct {
	OutputFiles          []string
	CcObjectFiles        []string
	CcSharedLibraryFiles []string
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

static_info_tag = "//build/bazel/rules/cc:cc_library_static.bzl%CcStaticLibraryInfo"
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

sharedLibraries = []
rootSharedLibraries = []

shared_info_tag = "@_builtins//:common/cc/experimental_cc_shared_library.bzl%CcSharedLibraryInfo"
if shared_info_tag in providers(target):
  shared_info = providers(target)[shared_info_tag]
  for lib in shared_info.linker_input.libraries:
    path = lib.dynamic_library.path
    rootSharedLibraries += [path]
    sharedLibraries.append(path)
else:
  for linker_input in linker_inputs:
    for library in linker_input.libraries:
      if library.dynamic_library:
        path = library.dynamic_library.path
        sharedLibraries.append(path)
        if linker_input.owner == target.label:
          rootSharedLibraries.append(path)

toc_file = ""
toc_file_tag = "//build/bazel/rules/cc:generate_toc.bzl%CcTocInfo"
if toc_file_tag in providers(target):
  toc_file = providers(target)[toc_file_tag].toc.path
else:
  # NOTE: It's OK if there's no ToC, as Soong just uses it for optimization
  pass

returns = [
  outputFiles,
  ccObjectFiles,
  sharedLibraries,
  staticLibraries,
  includes,
  system_includes,
  headers,
  rootStaticArchives,
  rootSharedLibraries,
  [toc_file]
]

return "|".join([", ".join(r) for r in returns])`
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getCcInfoType) ParseResult(rawString string) (CcInfo, error) {
	const expectedLen = 10
	splitString := strings.Split(rawString, "|")
	if len(splitString) != expectedLen {
		return CcInfo{}, fmt.Errorf("expected %d items, got %q", expectedLen, splitString)
	}
	outputFilesString := splitString[0]
	ccObjectsString := splitString[1]
	ccSharedLibrariesString := splitString[2]
	ccStaticLibrariesString := splitString[3]
	includesString := splitString[4]
	systemIncludesString := splitString[5]
	headersString := splitString[6]
	rootStaticArchivesString := splitString[7]
	rootDynamicLibrariesString := splitString[8]
	tocFile := splitString[9] // NOTE: Will be the empty string if there wasn't

	outputFiles := splitOrEmpty(outputFilesString, ", ")
	ccObjects := splitOrEmpty(ccObjectsString, ", ")
	ccSharedLibraries := splitOrEmpty(ccSharedLibrariesString, ", ")
	ccStaticLibraries := splitOrEmpty(ccStaticLibrariesString, ", ")
	includes := splitOrEmpty(includesString, ", ")
	systemIncludes := splitOrEmpty(systemIncludesString, ", ")
	headers := splitOrEmpty(headersString, ", ")
	rootStaticArchives := splitOrEmpty(rootStaticArchivesString, ", ")
	rootDynamicLibraries := splitOrEmpty(rootDynamicLibrariesString, ", ")
	return CcInfo{
		OutputFiles:          outputFiles,
		CcObjectFiles:        ccObjects,
		CcSharedLibraryFiles: ccSharedLibraries,
		CcStaticLibraryFiles: ccStaticLibraries,
		Includes:             includes,
		SystemIncludes:       systemIncludes,
		Headers:              headers,
		RootStaticArchives:   rootStaticArchives,
		RootDynamicLibraries: rootDynamicLibraries,
		TocFile:              tocFile,
	}, nil
}

// Query Bazel for the artifacts generated by the apex modules.
type getApexInfoType struct{}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getApexInfoType) Name() string {
	return "getApexInfo"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information. The function should have the following properties:
//   - `target` is the only parameter to this function (a configured target).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getApexInfoType) StarlarkFunctionBody() string {
	return `info = providers(target)["//build/bazel/rules/apex:apex.bzl%ApexInfo"]
return "{%s}" % ",".join([
    json_for_file("signed_output", info.signed_output),
    json_for_file("unsigned_output", info.unsigned_output),
    json_for_labels("provides_native_libs", info.provides_native_libs),
    json_for_labels("requires_native_libs", info.requires_native_libs),
    json_for_files("bundle_key_pair", info.bundle_key_pair),
    json_for_files("container_key_pair", info.container_key_pair)
    ])`
}

type ApexCqueryInfo struct {
	SignedOutput     string   `json:"signed_output"`
	UnsignedOutput   string   `json:"unsigned_output"`
	ProvidesLibs     []string `json:"provides_native_libs"`
	RequiresLibs     []string `json:"requires_native_libs"`
	BundleKeyPair    []string `json:"bundle_key_pair"`
	ContainerKeyPair []string `json:"container_key_pair"`
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getApexInfoType) ParseResult(rawString string) ApexCqueryInfo {
	var info ApexCqueryInfo
	if err := json.Unmarshal([]byte(rawString), &info); err != nil {
		panic(fmt.Errorf("cannot parse cquery result '%s': %s", rawString, err))
	}
	return info
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
