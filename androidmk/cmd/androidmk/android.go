package main

import (
	mkparser "android/soong/androidmk/parser"
	"fmt"
	"strings"

	bpparser "github.com/google/blueprint/parser"
)

const (
	clear_vars = "__android_mk_clear_vars"
)

var standardProperties = map[string]struct {
	string
	bpparser.ValueType
}{
	// String properties
	"LOCAL_MODULE":               {"name", bpparser.String},
	"LOCAL_MODULE_STEM":          {"stem", bpparser.String},
	"LOCAL_MODULE_CLASS":         {"class", bpparser.String},
	"LOCAL_CXX_STL":              {"stl", bpparser.String},
	"LOCAL_STRIP_MODULE":         {"strip", bpparser.String},
	"LOCAL_MULTILIB":             {"compile_multilib", bpparser.String},
	"LOCAL_ARM_MODE_HACK":        {"instruction_set", bpparser.String},
	"LOCAL_SDK_VERSION":          {"sdk_version", bpparser.String},
	"LOCAL_NDK_STL_VARIANT":      {"stl", bpparser.String},
	"LOCAL_JAR_MANIFEST":         {"manifest", bpparser.String},
	"LOCAL_JARJAR_RULES":         {"jarjar_rules", bpparser.String},
	"LOCAL_CERTIFICATE":          {"certificate", bpparser.String},
	"LOCAL_PACKAGE_NAME":         {"name", bpparser.String},
	"LOCAL_MODULE_RELATIVE_PATH": {"relative_install_path", bpparser.String},

	// List properties
	"LOCAL_SRC_FILES":               {"srcs", bpparser.List},
	"LOCAL_SHARED_LIBRARIES":        {"shared_libs", bpparser.List},
	"LOCAL_STATIC_LIBRARIES":        {"static_libs", bpparser.List},
	"LOCAL_WHOLE_STATIC_LIBRARIES":  {"whole_static_libs", bpparser.List},
	"LOCAL_SYSTEM_SHARED_LIBRARIES": {"system_shared_libs", bpparser.List},
	"LOCAL_C_INCLUDES":              {"include_dirs", bpparser.List},
	"LOCAL_EXPORT_C_INCLUDE_DIRS":   {"export_include_dirs", bpparser.List},
	"LOCAL_ASFLAGS":                 {"asflags", bpparser.List},
	"LOCAL_CLANG_ASFLAGS":           {"clang_asflags", bpparser.List},
	"LOCAL_CFLAGS":                  {"cflags", bpparser.List},
	"LOCAL_CONLYFLAGS":              {"conlyflags", bpparser.List},
	"LOCAL_CPPFLAGS":                {"cppflags", bpparser.List},
	"LOCAL_LDFLAGS":                 {"ldflags", bpparser.List},
	"LOCAL_REQUIRED_MODULES":        {"required", bpparser.List},
	"LOCAL_MODULE_TAGS":             {"tags", bpparser.List},
	"LOCAL_LDLIBS":                  {"host_ldlibs", bpparser.List},
	"LOCAL_CLANG_CFLAGS":            {"clang_cflags", bpparser.List},
	"LOCAL_YACCFLAGS":               {"yaccflags", bpparser.List},

	"LOCAL_JAVA_RESOURCE_DIRS":    {"java_resource_dirs", bpparser.List},
	"LOCAL_JAVACFLAGS":            {"javacflags", bpparser.List},
	"LOCAL_DX_FLAGS":              {"dxflags", bpparser.List},
	"LOCAL_JAVA_LIBRARIES":        {"java_libs", bpparser.List},
	"LOCAL_STATIC_JAVA_LIBRARIES": {"java_static_libs", bpparser.List},
	"LOCAL_AIDL_INCLUDES":         {"aidl_includes", bpparser.List},
	"LOCAL_AAPT_FLAGS":            {"aaptflags", bpparser.List},
	"LOCAL_PACKAGE_SPLITS":        {"package_splits", bpparser.List},

	// Bool properties
	"LOCAL_IS_HOST_MODULE":          {"host", bpparser.Bool},
	"LOCAL_CLANG":                   {"clang", bpparser.Bool},
	"LOCAL_FORCE_STATIC_EXECUTABLE": {"static", bpparser.Bool},
	"LOCAL_ADDRESS_SANITIZER":       {"asan", bpparser.Bool},
	"LOCAL_NATIVE_COVERAGE":         {"native_coverage", bpparser.Bool},
	"LOCAL_NO_CRT":                  {"nocrt", bpparser.Bool},
	"LOCAL_ALLOW_UNDEFINED_SYMBOLS": {"allow_undefined_symbols", bpparser.Bool},
	"LOCAL_RTTI_FLAG":               {"rtti", bpparser.Bool},

	"LOCAL_NO_STANDARD_LIBRARIES": {"no_standard_libraries", bpparser.Bool},

	"LOCAL_EXPORT_PACKAGE_RESOURCES": {"export_package_resources", bpparser.Bool},
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
	"USE_MINGW":             {"target", "windows"},
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

func allSubdirJavaFiles(args []string) string {
	return "**/*.java"
}

var moduleTypes = map[string]string{
	"BUILD_SHARED_LIBRARY":        "cc_library_shared",
	"BUILD_STATIC_LIBRARY":        "cc_library_static",
	"BUILD_HOST_SHARED_LIBRARY":   "cc_library_host_shared",
	"BUILD_HOST_STATIC_LIBRARY":   "cc_library_host_static",
	"BUILD_EXECUTABLE":            "cc_binary",
	"BUILD_HOST_EXECUTABLE":       "cc_binary_host",
	"BUILD_NATIVE_TEST":           "cc_test",
	"BUILD_HOST_NATIVE_TEST":      "cc_test_host",
	"BUILD_NATIVE_BENCHMARK":      "cc_benchmark",
	"BUILD_HOST_NATIVE_BENCHMARK": "cc_benchmark_host",

	"BUILD_JAVA_LIBRARY":             "java_library",
	"BUILD_STATIC_JAVA_LIBRARY":      "java_library_static",
	"BUILD_HOST_JAVA_LIBRARY":        "java_library_host",
	"BUILD_HOST_DALVIK_JAVA_LIBRARY": "java_library_host_dalvik",
	"BUILD_PACKAGE":                  "android_app",

	"BUILD_PREBUILT": "prebuilt",
}

var soongModuleTypes = map[string]bool{}

func androidScope() mkparser.Scope {
	globalScope := mkparser.NewScope(nil)
	globalScope.Set("CLEAR_VARS", clear_vars)
	globalScope.SetFunc("my-dir", mydir)
	globalScope.SetFunc("all-java-files-under", allJavaFilesUnder)
	globalScope.SetFunc("all-subdir-java-files", allSubdirJavaFiles)

	for k, v := range moduleTypes {
		globalScope.Set(k, v)
		soongModuleTypes[v] = true
	}

	return globalScope
}
