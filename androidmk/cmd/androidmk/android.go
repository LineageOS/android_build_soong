package main

import (
	"android/soong/androidmk/parser"
	"fmt"
	"strings"
)

const (
	clear_vars = "__android_mk_clear_vars"
)

var stringProperties = map[string]string{
	"LOCAL_MODULE":          "name",
	"LOCAL_MODULE_STEM":     "stem",
	"LOCAL_MODULE_CLASS":    "class",
	"LOCAL_CXX_STL":         "stl",
	"LOCAL_STRIP_MODULE":    "strip",
	"LOCAL_MULTILIB":        "compile_multilib",
	"LOCAL_ARM_MODE_HACK":   "instruction_set",
	"LOCAL_SDK_VERSION":     "sdk_version",
	"LOCAL_NDK_STL_VARIANT": "stl",
	"LOCAL_JAR_MANIFEST":    "manifest",
}

var listProperties = map[string]string{
	"LOCAL_SRC_FILES":               "srcs",
	"LOCAL_SHARED_LIBRARIES":        "shared_libs",
	"LOCAL_STATIC_LIBRARIES":        "static_libs",
	"LOCAL_WHOLE_STATIC_LIBRARIES":  "whole_static_libs",
	"LOCAL_SYSTEM_SHARED_LIBRARIES": "system_shared_libs",
	"LOCAL_C_INCLUDES":              "include_dirs",
	"LOCAL_EXPORT_C_INCLUDE_DIRS":   "export_include_dirs",
	"LOCAL_ASFLAGS":                 "asflags",
	"LOCAL_CLANG_ASFLAGS":           "clang_asflags",
	"LOCAL_CFLAGS":                  "cflags",
	"LOCAL_CONLYFLAGS":              "conlyflags",
	"LOCAL_CPPFLAGS":                "cppflags",
	"LOCAL_LDFLAGS":                 "ldflags",
	"LOCAL_REQUIRED_MODULES":        "required",
	"LOCAL_MODULE_TAGS":             "tags",
	"LOCAL_LDLIBS":                  "host_ldlibs",
	"LOCAL_CLANG_CFLAGS":            "clang_cflags",

	"LOCAL_JAVA_RESOURCE_DIRS":    "resource_dirs",
	"LOCAL_JAVACFLAGS":            "javacflags",
	"LOCAL_DX_FLAGS":              "dxflags",
	"LOCAL_JAVA_LIBRARIES":        "java_libs",
	"LOCAL_STATIC_JAVA_LIBRARIES": "java_static_libs",
}

var boolProperties = map[string]string{
	"LOCAL_IS_HOST_MODULE":          "host",
	"LOCAL_CLANG":                   "clang",
	"LOCAL_FORCE_STATIC_EXECUTABLE": "static",
	"LOCAL_ADDRESS_SANITIZER":       "asan",
	"LOCAL_NATIVE_COVERAGE":         "native_coverage",
	"LOCAL_NO_CRT":                  "nocrt",
	"LOCAL_ALLOW_UNDEFINED_SYMBOLS": "allow_undefined_symbols",
	"LOCAL_RTTI_FLAG":               "rtti",

	"LOCAL_NO_STANDARD_LIBRARIES": "no_standard_libraries",
}

var deleteProperties = map[string]struct{}{
	"LOCAL_CPP_EXTENSION": struct{}{},
}

var propertySuffixes = []struct {
	suffix string
	class  string
}{
	{"arm", "arch"},
	{"arm64", "arch"},
	{"mips", "arch"},
	{"mips64", "arch"},
	{"x86", "arch"},
	{"x86_64", "arch"},
	{"32", "multilib"},
	{"64", "multilib"},
}

var propertySuffixTranslations = map[string]string{
	"32": "lib32",
	"64": "lib64",
}

var conditionalTranslations = map[string]struct {
	class  string
	suffix string
}{
	"($(HOST_OS),darwin)":   {"target", "darwin"},
	"($(HOST_OS), darwin)":  {"target", "darwin"},
	"($(HOST_OS),windows)":  {"target", "windows"},
	"($(HOST_OS), windows)": {"target", "windows"},
	"($(HOST_OS),linux)":    {"target", "linux"},
	"($(HOST_OS), linux)":   {"target", "linux"},
	"($(BUILD_OS),darwin)":  {"target", "darwin"},
	"($(BUILD_OS), darwin)": {"target", "darwin"},
	"($(BUILD_OS),linux)":   {"target", "linux"},
	"($(BUILD_OS), linux)":  {"target", "linux"},
}

func mydir(args []string) string {
	return "."
}

func allJavaFilesUnder(args []string) string {
	dir := ""
	if len(args) > 0 {
		dir = strings.TrimSpace(args[0])
	}

	return fmt.Sprintf("%s/**/*.java", dir)
}

var moduleTypes = map[string]string{
	"BUILD_SHARED_LIBRARY":      "cc_library_shared",
	"BUILD_STATIC_LIBRARY":      "cc_library_static",
	"BUILD_HOST_SHARED_LIBRARY": "cc_library_host_shared",
	"BUILD_HOST_STATIC_LIBRARY": "cc_library_host_static",
	"BUILD_EXECUTABLE":          "cc_binary",
	"BUILD_HOST_EXECUTABLE":     "cc_binary_host",
	"BUILD_NATIVE_TEST":         "cc_test",
	"BUILD_HOST_NATIVE_TEST":    "cc_test_host",

	"BUILD_JAVA_LIBRARY":             "java_library",
	"BUILD_STATIC_JAVA_LIBRARY":      "java_library_static",
	"BUILD_HOST_JAVA_LIBRARY":        "java_library_host",
	"BUILD_HOST_DALVIK_JAVA_LIBRARY": "java_library_host_dalvik",

	"BUILD_PREBUILT": "prebuilt",
}

var soongModuleTypes = map[string]bool{}

func androidScope() parser.Scope {
	globalScope := parser.NewScope(nil)
	globalScope.Set("CLEAR_VARS", clear_vars)
	globalScope.SetFunc("my-dir", mydir)
	globalScope.SetFunc("all-java-files-under", allJavaFilesUnder)

	for k, v := range moduleTypes {
		globalScope.Set(k, v)
		soongModuleTypes[v] = true
	}

	return globalScope
}
