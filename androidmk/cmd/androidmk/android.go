package main

import (
	"android/soong/androidmk/parser"
)

const (
	clear_vars                = "__android_mk_clear_vars"
	build_shared_library      = "cc_library_shared"
	build_static_library      = "cc_library_static"
	build_host_static_library = "cc_library_host_static"
	build_host_shared_library = "cc_library_host_shared"
	build_executable          = "cc_binary"
	build_host_executable     = "cc_binary_host"
	build_native_test         = "cc_test"
	build_host_native_test    = "cc_test_host"
	build_prebuilt            = "prebuilt"
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
}

func mydir(args []string) string {
	return "."
}

func androidScope() parser.Scope {
	globalScope := parser.NewScope(nil)
	globalScope.Set("CLEAR_VARS", clear_vars)
	globalScope.Set("BUILD_HOST_EXECUTABLE", build_host_executable)
	globalScope.Set("BUILD_SHARED_LIBRARY", build_shared_library)
	globalScope.Set("BUILD_STATIC_LIBRARY", build_static_library)
	globalScope.Set("BUILD_HOST_STATIC_LIBRARY", build_host_static_library)
	globalScope.Set("BUILD_HOST_SHARED_LIBRARY", build_host_shared_library)
	globalScope.Set("BUILD_NATIVE_TEST", build_native_test)
	globalScope.Set("BUILD_HOST_NATIVE_TEST", build_host_native_test)
	globalScope.Set("BUILD_EXECUTABLE", build_executable)
	globalScope.Set("BUILD_PREBUILT", build_prebuilt)
	globalScope.SetFunc("my-dir", mydir)

	globalScope.Set("lib32", "lib32")
	globalScope.Set("lib64", "lib64")
	globalScope.Set("arm", "arm")
	globalScope.Set("arm64", "arm64")
	globalScope.Set("mips", "mips")
	globalScope.Set("mips64", "mips64")
	globalScope.Set("x86", "x86")
	globalScope.Set("x86_64", "x86_64")

	return globalScope
}
