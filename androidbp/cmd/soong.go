package main

import bpparser "github.com/google/blueprint/parser"

var standardProperties = map[string]struct {
	string
	bpparser.ValueType
}{
	// ==== STRING PROPERTIES ====
	"name":             {"LOCAL_MODULE", bpparser.String},
	"stem":             {"LOCAL_MODULE_STEM", bpparser.String},
	"class":            {"LOCAL_MODULE_CLASS", bpparser.String},
	"stl":              {"LOCAL_CXX_STL", bpparser.String},
	"strip":            {"LOCAL_STRIP_MODULE", bpparser.String},
	"compile_multilib": {"LOCAL_MULTILIB", bpparser.String},
	"instruction_set":  {"LOCAL_ARM_MODE_HACK", bpparser.String},
	"sdk_version":      {"LOCAL_SDK_VERSION", bpparser.String},
	//"stl":              "LOCAL_NDK_STL_VARIANT", TODO
	"manifest":     {"LOCAL_JAR_MANIFEST", bpparser.String},
	"jarjar_rules": {"LOCAL_JARJAR_RULES", bpparser.String},
	"certificate":  {"LOCAL_CERTIFICATE", bpparser.String},
	//"name":             "LOCAL_PACKAGE_NAME", TODO

	// ==== LIST PROPERTIES ====
	"srcs":                {"LOCAL_SRC_FILES", bpparser.List},
	"shared_libs":         {"LOCAL_SHARED_LIBRARIES", bpparser.List},
	"static_libs":         {"LOCAL_STATIC_LIBRARIES", bpparser.List},
	"whole_static_libs":   {"LOCAL_WHOLE_STATIC_LIBRARIES", bpparser.List},
	"system_shared_libs":  {"LOCAL_SYSTEM_SHARED_LIBRARIES", bpparser.List},
	"include_dirs":        {"LOCAL_C_INCLUDES", bpparser.List},
	"export_include_dirs": {"LOCAL_EXPORT_C_INCLUDE_DIRS", bpparser.List},
	"asflags":             {"LOCAL_ASFLAGS", bpparser.List},
	"clang_asflags":       {"LOCAL_CLANG_ASFLAGS", bpparser.List},
	"cflags":              {"LOCAL_CFLAGS", bpparser.List},
	"conlyflags":          {"LOCAL_CONLYFLAGS", bpparser.List},
	"cppflags":            {"LOCAL_CPPFLAGS", bpparser.List},
	"ldflags":             {"LOCAL_LDFLAGS", bpparser.List},
	"required":            {"LOCAL_REQUIRED_MODULES", bpparser.List},
	"tags":                {"LOCAL_MODULE_TAGS", bpparser.List},
	"host_ldlibs":         {"LOCAL_LDLIBS", bpparser.List},
	"clang_cflags":        {"LOCAL_CLANG_CFLAGS", bpparser.List},
	"yaccflags":           {"LOCAL_YACCFLAGS", bpparser.List},
	"java_resource_dirs":  {"LOCAL_JAVA_RESOURCE_DIRS", bpparser.List},
	"javacflags":          {"LOCAL_JAVACFLAGS", bpparser.List},
	"dxflags":             {"LOCAL_DX_FLAGS", bpparser.List},
	"java_libs":           {"LOCAL_JAVA_LIBRARIES", bpparser.List},
	"java_static_libs":    {"LOCAL_STATIC_JAVA_LIBRARIES", bpparser.List},
	"aidl_includes":       {"LOCAL_AIDL_INCLUDES", bpparser.List},
	"aaptflags":           {"LOCAL_AAPT_FLAGS", bpparser.List},
	"package_splits":      {"LOCAL_PACKAGE_SPLITS", bpparser.List},

	// ==== BOOL PROPERTIES ====
	"host":                    {"LOCAL_IS_HOST_MODULE", bpparser.Bool},
	"clang":                   {"LOCAL_CLANG", bpparser.Bool},
	"static":                  {"LOCAL_FORCE_STATIC_EXECUTABLE", bpparser.Bool},
	"asan":                    {"LOCAL_ADDRESS_SANITIZER", bpparser.Bool},
	"native_coverage":         {"LOCAL_NATIVE_COVERAGE", bpparser.Bool},
	"nocrt":                   {"LOCAL_NO_CRT", bpparser.Bool},
	"allow_undefined_symbols": {"LOCAL_ALLOW_UNDEFINED_SYMBOLS", bpparser.Bool},
	"rtti":                     {"LOCAL_RTTI_FLAG", bpparser.Bool},
	"no_standard_libraries":    {"LOCAL_NO_STANDARD_LIBRARIES", bpparser.Bool},
	"export_package_resources": {"LOCAL_EXPORT_PACKAGE_RESOURCES", bpparser.Bool},
}

var moduleTypes = map[string]string{
	"cc_library_shared":        "BUILD_SHARED_LIBRARY",
	"cc_library_static":        "BUILD_STATIC_LIBRARY",
	"cc_library_host_shared":   "BUILD_HOST_SHARED_LIBRARY",
	"cc_library_host_static":   "BUILD_HOST_STATIC_LIBRARY",
	"cc_binary":                "BUILD_EXECUTABLE",
	"cc_binary_host":           "BUILD_HOST_EXECUTABLE",
	"cc_test":                  "BUILD_NATIVE_TEST",
	"cc_test_host":             "BUILD_HOST_NATIVE_TEST",
	"java_library":             "BUILD_JAVA_LIBRARY",
	"java_library_static":      "BUILD_STATIC_JAVA_LIBRARY",
	"java_library_host":        "BUILD_HOST_JAVA_LIBRARY",
	"java_library_host_dalvik": "BUILD_HOST_DALVIK_JAVA_LIBRARY",
	"android_app":              "BUILD_PACKAGE",
	"prebuilt":                 "BUILD_PREBUILT",
}
