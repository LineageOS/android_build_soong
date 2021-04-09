package cquery

import (
	"strings"
)

var (
	GetOutputFiles                  = &getOutputFilesRequestType{}
	GetOutputFilesAndCcObjectFiles  = &getOutputFilesAndCcObjectFilesType{}
	GetPrebuiltCcStaticLibraryFiles = &getPrebuiltCcStaticLibraryFiles{}
)

type GetOutputFilesAndCcObjectFiles_Result struct {
	OutputFiles   []string
	CcObjectFiles []string
}

type getOutputFilesRequestType struct{}

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
	return strings.Split(rawString, ", ")
}

type getOutputFilesAndCcObjectFilesType struct{}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getOutputFilesAndCcObjectFilesType) Name() string {
	return "getOutputFilesAndCcObjectFiles"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information.
// The function should have the following properties:
//   - `target` is the only parameter to this function (a configured target).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getOutputFilesAndCcObjectFilesType) StarlarkFunctionBody() string {
	return `
outputFiles = [f.path for f in target.files.to_list()]

ccObjectFiles = []
linker_inputs = providers(target)["CcInfo"].linking_context.linker_inputs.to_list()

for linker_input in linker_inputs:
  for library in linker_input.libraries:
    for object in library.objects:
      ccObjectFiles += [object.path]
return ', '.join(outputFiles) + "|" + ', '.join(ccObjectFiles)`
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getOutputFilesAndCcObjectFilesType) ParseResult(rawString string) GetOutputFilesAndCcObjectFiles_Result {
	var outputFiles []string
	var ccObjects []string

	splitString := strings.Split(rawString, "|")
	outputFilesString := splitString[0]
	ccObjectsString := splitString[1]
	outputFiles = splitOrEmpty(outputFilesString, ", ")
	ccObjects = splitOrEmpty(ccObjectsString, ", ")
	return GetOutputFilesAndCcObjectFiles_Result{outputFiles, ccObjects}
}

type getPrebuiltCcStaticLibraryFiles struct{}

// Name returns the name of the starlark function to get prebuilt cc static library files
func (g getPrebuiltCcStaticLibraryFiles) Name() string {
	return "getPrebuiltCcStaticLibraryFiles"
}

// StarlarkFunctionBody returns the unindented body of a starlark function for extracting the static
// library paths from a cc_import module.
func (g getPrebuiltCcStaticLibraryFiles) StarlarkFunctionBody() string {
	return `
linker_inputs = providers(target)["CcInfo"].linking_context.linker_inputs.to_list()

static_libraries = []
for linker_input in linker_inputs:
  for library in linker_input.libraries:
    static_libraries.append(library.static_library.path)

return ', '.join(static_libraries)`
}

// ParseResult returns a slice of bazel output paths to static libraries if any exist for the given
// rawString corresponding to the string output which was created by evaluating the
// StarlarkFunctionBody.
func (g getPrebuiltCcStaticLibraryFiles) ParseResult(rawString string) []string {
	return strings.Split(rawString, ", ")
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
