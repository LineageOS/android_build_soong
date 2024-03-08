package cquery

import (
	"encoding/json"
	"fmt"
	"strings"
)

var (
	GetOutputFiles      = &getOutputFilesRequestType{}
	GetCcInfo           = &getCcInfoType{}
	GetApexInfo         = &getApexInfoType{}
	GetCcUnstrippedInfo = &getCcUnstrippedInfoType{}
	GetPrebuiltFileInfo = &getPrebuiltFileInfo{}
)

type CcAndroidMkInfo struct {
	LocalStaticLibs      []string
	LocalWholeStaticLibs []string
	LocalSharedLibs      []string
}

type CcInfo struct {
	CcAndroidMkInfo
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
	TidyFiles            []string
	TocFile              string
	UnstrippedOutput     string
	AbiDiffFiles         []string
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
//   - The arguments are `target` (a configured target) and `id_string` (the label + configuration).
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
//   - The arguments are `target` (a configured target) and `id_string` (the label + configuration).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getCcInfoType) StarlarkFunctionBody() string {
	return `
outputFiles = [f.path for f in target.files.to_list()]
p = providers(target)
cc_info = p.get("CcInfo")
if not cc_info:
  fail("%s did not provide CcInfo" % id_string)

includes = cc_info.compilation_context.includes.to_list()
system_includes = cc_info.compilation_context.system_includes.to_list()
headers = [f.path for f in cc_info.compilation_context.headers.to_list()]

ccObjectFiles = []
staticLibraries = []
rootStaticArchives = []
linker_inputs = cc_info.linking_context.linker_inputs.to_list()

static_info_tag = "//build/bazel/rules/cc:cc_library_static.bzl%CcStaticLibraryInfo"
if static_info_tag in p:
  static_info = p[static_info_tag]
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

shared_info_tag = "//build/bazel/rules/cc:cc_library_shared.bzl%CcSharedLibraryOutputInfo"
stubs_tag = "//build/bazel/rules/cc:cc_stub_library.bzl%CcStubInfo"
unstripped_tag = "//build/bazel/rules/cc:stripped_cc_common.bzl%CcUnstrippedInfo"
unstripped = ""

if shared_info_tag in p:
  shared_info = p[shared_info_tag]
  path = shared_info.output_file.path
  sharedLibraries.append(path)
  rootSharedLibraries += [path]
  unstripped = path
  if unstripped_tag in p:
    unstripped = p[unstripped_tag].unstripped.path
elif stubs_tag in p:
  rootSharedLibraries.extend([f.path for f in target.files.to_list()])
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
if toc_file_tag in p:
  toc_file = p[toc_file_tag].toc.path
else:
  # NOTE: It's OK if there's no ToC, as Soong just uses it for optimization
  pass

tidy_files = []
clang_tidy_info = p.get("//build/bazel/rules/cc:clang_tidy.bzl%ClangTidyInfo")
if clang_tidy_info:
  tidy_files = [v.path for v in clang_tidy_info.transitive_tidy_files.to_list()]

abi_diff_files = []
abi_diff_info = p.get("//build/bazel/rules/abi:abi_dump.bzl%AbiDiffInfo")
if abi_diff_info:
  abi_diff_files = [f.path for f in abi_diff_info.diff_files.to_list()]

local_static_libs = []
local_whole_static_libs = []
local_shared_libs = []
androidmk_tag = "//build/bazel/rules/cc:cc_library_common.bzl%CcAndroidMkInfo"
if androidmk_tag in p:
    androidmk_info = p[androidmk_tag]
    local_static_libs = androidmk_info.local_static_libs
    local_whole_static_libs = androidmk_info.local_whole_static_libs
    local_shared_libs = androidmk_info.local_shared_libs

return json.encode({
    "OutputFiles": outputFiles,
    "CcObjectFiles": ccObjectFiles,
    "CcSharedLibraryFiles": sharedLibraries,
    "CcStaticLibraryFiles": staticLibraries,
    "Includes": includes,
    "SystemIncludes": system_includes,
    "Headers": headers,
    "RootStaticArchives": rootStaticArchives,
    "RootDynamicLibraries": rootSharedLibraries,
    "TidyFiles": [t for t in tidy_files],
    "TocFile": toc_file,
    "UnstrippedOutput": unstripped,
    "AbiDiffFiles": abi_diff_files,
    "LocalStaticLibs": [l for l in local_static_libs],
    "LocalWholeStaticLibs": [l for l in local_whole_static_libs],
    "LocalSharedLibs": [l for l in local_shared_libs],
})`

}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getCcInfoType) ParseResult(rawString string) (CcInfo, error) {
	var ccInfo CcInfo
	if err := parseJson(rawString, &ccInfo); err != nil {
		return ccInfo, err
	}
	return ccInfo, nil
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
//   - The arguments are `target` (a configured target) and `id_string` (the label + configuration).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getApexInfoType) StarlarkFunctionBody() string {
	return `
info = providers(target).get("//build/bazel/rules/apex:apex_info.bzl%ApexInfo")
if not info:
  fail("%s did not provide ApexInfo" % id_string)
bundle_key_info = info.bundle_key_info
container_key_info = info.container_key_info

signed_compressed_output = "" # no .capex if the apex is not compressible, cannot be None as it needs to be json encoded.
if info.signed_compressed_output:
    signed_compressed_output = info.signed_compressed_output.path

mk_info = providers(target).get("//build/bazel/rules/apex:apex_info.bzl%ApexMkInfo")
if not mk_info:
  fail("%s did not provide ApexMkInfo" % id_string)

tidy_files = []
clang_tidy_info = providers(target).get("//build/bazel/rules/cc:clang_tidy.bzl%ClangTidyInfo")
if clang_tidy_info:
    tidy_files = [v.path for v in clang_tidy_info.transitive_tidy_files.to_list()]

return json.encode({
    "signed_output": info.signed_output.path,
    "signed_compressed_output": signed_compressed_output,
    "unsigned_output": info.unsigned_output.path,
    "provides_native_libs": [str(lib) for lib in info.provides_native_libs],
    "requires_native_libs": [str(lib) for lib in info.requires_native_libs],
    "bundle_key_info": [bundle_key_info.public_key.path, bundle_key_info.private_key.path],
    "container_key_info": [container_key_info.pem.path, container_key_info.pk8.path, container_key_info.key_name],
    "package_name": info.package_name,
    "symbols_used_by_apex": info.symbols_used_by_apex.path,
    "java_symbols_used_by_apex": info.java_symbols_used_by_apex.path,
    "backing_libs": info.backing_libs.path,
    "bundle_file": info.base_with_config_zip.path,
    "installed_files": info.installed_files.path,
    "make_modules_to_install": mk_info.make_modules_to_install,
    "files_info": mk_info.files_info,
    "tidy_files": [t for t in tidy_files],
})`
}

type ApexInfo struct {
	// From the ApexInfo provider
	SignedOutput           string   `json:"signed_output"`
	SignedCompressedOutput string   `json:"signed_compressed_output"`
	UnsignedOutput         string   `json:"unsigned_output"`
	ProvidesLibs           []string `json:"provides_native_libs"`
	RequiresLibs           []string `json:"requires_native_libs"`
	BundleKeyInfo          []string `json:"bundle_key_info"`
	ContainerKeyInfo       []string `json:"container_key_info"`
	PackageName            string   `json:"package_name"`
	SymbolsUsedByApex      string   `json:"symbols_used_by_apex"`
	JavaSymbolsUsedByApex  string   `json:"java_symbols_used_by_apex"`
	BackingLibs            string   `json:"backing_libs"`
	BundleFile             string   `json:"bundle_file"`
	InstalledFiles         string   `json:"installed_files"`
	TidyFiles              []string `json:"tidy_files"`

	// From the ApexMkInfo provider
	MakeModulesToInstall []string            `json:"make_modules_to_install"`
	PayloadFilesInfo     []map[string]string `json:"files_info"`
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getApexInfoType) ParseResult(rawString string) (ApexInfo, error) {
	var info ApexInfo
	err := parseJson(rawString, &info)
	return info, err
}

// getCcUnstrippedInfoType implements cqueryRequest interface. It handles the
// interaction with `bazel cquery` to retrieve CcUnstrippedInfo provided
// by the` cc_binary` and `cc_shared_library` rules.
type getCcUnstrippedInfoType struct{}

func (g getCcUnstrippedInfoType) Name() string {
	return "getCcUnstrippedInfo"
}

func (g getCcUnstrippedInfoType) StarlarkFunctionBody() string {
	return `
p = providers(target)
output_path = target.files.to_list()[0].path

unstripped = output_path
unstripped_tag = "//build/bazel/rules/cc:stripped_cc_common.bzl%CcUnstrippedInfo"
if unstripped_tag in p:
    unstripped_info = p[unstripped_tag]
    unstripped = unstripped_info.unstripped[0].files.to_list()[0].path

local_static_libs = []
local_whole_static_libs = []
local_shared_libs = []
androidmk_tag = "//build/bazel/rules/cc:cc_library_common.bzl%CcAndroidMkInfo"
if androidmk_tag in p:
    androidmk_info = p[androidmk_tag]
    local_static_libs = androidmk_info.local_static_libs
    local_whole_static_libs = androidmk_info.local_whole_static_libs
    local_shared_libs = androidmk_info.local_shared_libs

tidy_files = []
clang_tidy_info = p.get("//build/bazel/rules/cc:clang_tidy.bzl%ClangTidyInfo")
if clang_tidy_info:
    tidy_files = [v.path for v in clang_tidy_info.transitive_tidy_files.to_list()]

return json.encode({
    "OutputFile":  output_path,
    "UnstrippedOutput": unstripped,
    "LocalStaticLibs": [l for l in local_static_libs],
    "LocalWholeStaticLibs": [l for l in local_whole_static_libs],
    "LocalSharedLibs": [l for l in local_shared_libs],
    "TidyFiles": [t for t in tidy_files],
})
`
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getCcUnstrippedInfoType) ParseResult(rawString string) (CcUnstrippedInfo, error) {
	var info CcUnstrippedInfo
	err := parseJson(rawString, &info)
	return info, err
}

type CcUnstrippedInfo struct {
	CcAndroidMkInfo
	OutputFile       string
	UnstrippedOutput string
	TidyFiles        []string
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

// parseJson decodes json string into the fields of the receiver.
// Unknown attribute name causes panic.
func parseJson(jsonString string, info interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(jsonString))
	decoder.DisallowUnknownFields() //useful to detect typos, e.g. in unit tests
	err := decoder.Decode(info)
	if err != nil {
		return fmt.Errorf("cannot parse cquery result '%s': %s", jsonString, err)
	}
	return nil
}

type getPrebuiltFileInfo struct{}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getPrebuiltFileInfo) Name() string {
	return "getPrebuiltFileInfo"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information.
// The function should have the following properties:
//   - The arguments are `target` (a configured target) and `id_string` (the label + configuration).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getPrebuiltFileInfo) StarlarkFunctionBody() string {
	return `
p = providers(target)
prebuilt_file_info = p.get("//build/bazel/rules:prebuilt_file.bzl%PrebuiltFileInfo")
if not prebuilt_file_info:
  fail("%s did not provide PrebuiltFileInfo" % id_string)

return json.encode({
	"Src": prebuilt_file_info.src.path,
	"Dir": prebuilt_file_info.dir,
	"Filename": prebuilt_file_info.filename,
	"Installable": prebuilt_file_info.installable,
})`
}

type PrebuiltFileInfo struct {
	// TODO: b/207489266 - Fully support all properties in prebuilt_file
	Src         string
	Dir         string
	Filename    string
	Installable bool
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getPrebuiltFileInfo) ParseResult(rawString string) (PrebuiltFileInfo, error) {
	var info PrebuiltFileInfo
	err := parseJson(rawString, &info)
	return info, err
}
