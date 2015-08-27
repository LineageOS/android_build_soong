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
	"srcs":               {"LOCAL_SRC_FILES", bpparser.List},
	"exclude_srcs":       {"LOCAL_SRC_FILES_EXCLUDE", bpparser.List},
	"shared_libs":        {"LOCAL_SHARED_LIBRARIES", bpparser.List},
	"static_libs":        {"LOCAL_STATIC_LIBRARIES", bpparser.List},
	"whole_static_libs":  {"LOCAL_WHOLE_STATIC_LIBRARIES", bpparser.List},
	"system_shared_libs": {"LOCAL_SYSTEM_SHARED_LIBRARIES", bpparser.List},
	"asflags":            {"LOCAL_ASFLAGS", bpparser.List},
	"clang_asflags":      {"LOCAL_CLANG_ASFLAGS", bpparser.List},
	"cflags":             {"LOCAL_CFLAGS", bpparser.List},
	"conlyflags":         {"LOCAL_CONLYFLAGS", bpparser.List},
	"cppflags":           {"LOCAL_CPPFLAGS", bpparser.List},
	"ldflags":            {"LOCAL_LDFLAGS", bpparser.List},
	"required":           {"LOCAL_REQUIRED_MODULES", bpparser.List},
	"tags":               {"LOCAL_MODULE_TAGS", bpparser.List},
	"host_ldlibs":        {"LOCAL_LDLIBS", bpparser.List},
	"clang_cflags":       {"LOCAL_CLANG_CFLAGS", bpparser.List},
	"yaccflags":          {"LOCAL_YACCFLAGS", bpparser.List},
	"java_resource_dirs": {"LOCAL_JAVA_RESOURCE_DIRS", bpparser.List},
	"javacflags":         {"LOCAL_JAVACFLAGS", bpparser.List},
	"dxflags":            {"LOCAL_DX_FLAGS", bpparser.List},
	"java_libs":          {"LOCAL_JAVA_LIBRARIES", bpparser.List},
	"java_static_libs":   {"LOCAL_STATIC_JAVA_LIBRARIES", bpparser.List},
	"aidl_includes":      {"LOCAL_AIDL_INCLUDES", bpparser.List},
	"aaptflags":          {"LOCAL_AAPT_FLAGS", bpparser.List},
	"package_splits":     {"LOCAL_PACKAGE_SPLITS", bpparser.List},

	// ==== BOOL PROPERTIES ====
	"host":                    {"LOCAL_IS_HOST_MODULE", bpparser.Bool},
	"clang":                   {"LOCAL_CLANG", bpparser.Bool},
	"static_executable":       {"LOCAL_FORCE_STATIC_EXECUTABLE", bpparser.Bool},
	"asan":                    {"LOCAL_ADDRESS_SANITIZER", bpparser.Bool},
	"native_coverage":         {"LOCAL_NATIVE_COVERAGE", bpparser.Bool},
	"nocrt":                   {"LOCAL_NO_CRT", bpparser.Bool},
	"allow_undefined_symbols": {"LOCAL_ALLOW_UNDEFINED_SYMBOLS", bpparser.Bool},
	"rtti":                      {"LOCAL_RTTI_FLAG", bpparser.Bool},
	"no_standard_libraries":     {"LOCAL_NO_STANDARD_LIBRARIES", bpparser.Bool},
	"export_package_resources":  {"LOCAL_EXPORT_PACKAGE_RESOURCES", bpparser.Bool},
	"no_default_compiler_flags": {"LOCAL_NO_DEFAULT_COMPILER_FLAGS", bpparser.Bool},
}

var rewriteProperties = map[string]struct {
	string
	f func(name string, prop *bpparser.Property, val string) (propAssignment, error)
}{
	"include_dirs":        {"LOCAL_C_INCLUDES", appendAssign},
	"local_include_dirs":  {"LOCAL_C_INCLUDES", prependLocalPath},
	"export_include_dirs": {"LOCAL_EXPORT_C_INCLUDE_DIRS", prependLocalPath},
	"suffix":              {"LOCAL_MODULE_STEM", prependLocalModule},
	"version_script":      {"LOCAL_LDFLAGS", versionScript},
}

var ignoredProperties = map[string]bool{
	"host_supported": true,
}

var moduleTypeToRule = map[string]string{
	"cc_library_shared":        "BUILD_SHARED_LIBRARY",
	"cc_library_static":        "BUILD_STATIC_LIBRARY",
	"cc_library_host_shared":   "BUILD_HOST_SHARED_LIBRARY",
	"cc_library_host_static":   "BUILD_HOST_STATIC_LIBRARY",
	"cc_binary":                "BUILD_EXECUTABLE",
	"cc_binary_host":           "BUILD_HOST_EXECUTABLE",
	"cc_test":                  "BUILD_NATIVE_TEST",
	"cc_test_host":             "BUILD_HOST_NATIVE_TEST",
	"cc_benchmark":             "BUILD_NATIVE_BENCHMARK",
	"cc_benchmark_host":        "BUILD_HOST_NATIVE_BENCHMARK",
	"java_library":             "BUILD_JAVA_LIBRARY",
	"java_library_static":      "BUILD_STATIC_JAVA_LIBRARY",
	"java_library_host":        "BUILD_HOST_JAVA_LIBRARY",
	"java_library_host_dalvik": "BUILD_HOST_DALVIK_JAVA_LIBRARY",
	"android_app":              "BUILD_PACKAGE",
	"prebuilt":                 "BUILD_PREBUILT",
}

var ignoredModuleType = map[string]bool{
	"bootstrap_go_binary":  true,
	"bootstrap_go_package": true,
	"toolchain_library":    true,
}

var suffixProperties = map[string]map[string]string{
	"multilib": {"lib32": "32", "lib64": "64"},
	"arch": {"arm": "arm", "arm64": "arm64", "mips": "mips", "mips64": "mips64",
		"x86": "x86", "x86_64": "x86_64"},
}

var cpuVariantConditionals = map[string]struct {
	conditional string
	suffix      string
	secondArch  bool
}{
	"armv5te":      {"ifeq ($(TARGET_ARCH_VARIANT),armv5te)", "$(TARGET_ARCH)", true},
	"armv7_a":      {"ifeq ($(TARGET_ARCH_VARIANT),armv7-a)", "$(TARGET_ARCH)", true},
	"armv7_a_neon": {"ifeq ($(TARGET_ARCH_VARIANT),armv7-a-neon)", "$(TARGET_ARCH)", true},
	"cortex_a7":    {"ifeq ($(TARGET_CPU_VARIANT),cortex-a7)", "$(TARGET_ARCH)", true},
	"cortex_a8":    {"ifeq ($(TARGET_CPU_VARIANT),cortex-a8)", "$(TARGET_ARCH)", true},
	"cortex_a9":    {"ifeq ($(TARGET_CPU_VARIANT),cortex-a9)", "$(TARGET_ARCH)", true},
	"cortex_a15":   {"ifeq ($(TARGET_CPU_VARIANT),cortex-a15)", "$(TARGET_ARCH)", true},
	"krait":        {"ifeq ($(TARGET_CPU_VARIANT),krait)", "$(TARGET_ARCH)", true},
	"denver":       {"ifeq ($(TARGET_CPU_VARIANT),denver)", "$(TARGET_ARCH)", true},
	"denver64":     {"ifeq ($(TARGET_CPU_VARIANT),denver64)", "$(TARGET_ARCH)", true},
	"mips_rev6":    {"ifdef ARCH_MIPS_REV6", "mips", false},
	"atom":         {"ifeq ($(TARGET_ARCH_VARIANT),atom)", "$(TARGET_ARCH)", true},
	"silvermont":   {"ifeq ($(TARGET_ARCH_VARIANT),silvermont)", "$(TARGET_ARCH)", true},
	"x86_sse3":     {"ifeq ($(ARCH_X86_HAVE_SSE3),true)", "x86", false},
	"x86_sse4":     {"ifeq ($(ARCH_X86_HAVE_SSE4),true)", "x86", false},
}

var hostScopedPropertyConditionals = map[string]string{
	"host":        "",
	"darwin":      "ifeq ($(HOST_OS), darwin)",
	"not_darwin":  "ifneq ($(HOST_OS), darwin)",
	"windows":     "ifeq ($(HOST_OS), windows)",
	"not_windows": "ifneq ($(HOST_OS), windows)",
	"linux":       "ifeq ($(HOST_OS), linux)",
	"not_linux":   "ifneq ($(HOST_OS), linux)",
}

// TODO: host target?
var targetScopedPropertyConditionals = map[string]string{
	"android":       "",
	"android32":     "ifneq ($(TARGET_IS_64_BIT), true)",
	"not_android32": "ifeq ($(TARGET_IS_64_BIT), true)",
	"android64":     "ifeq ($(TARGET_IS_64_BIT), true)",
	"not_android64": "ifneq ($(TARGET_IS_64_BIT), true)",
}

var disabledHostConditionals = map[string]string{
	"darwin":      "ifneq ($(HOST_OS), darwin)",
	"not_darwin":  "ifeq ($(HOST_OS), darwin)",
	"windows":     "ifneq ($(HOST_OS), windows)",
	"not_windows": "ifeq ($(HOST_OS), windows)",
	"linux":       "ifneq ($(HOST_OS), linux)",
	"not_linux":   "ifeq ($(HOST_OS), linux)",
}

var disabledTargetConditionals = map[string]string{
	"android32":     "ifeq ($(TARGET_IS_64_BIT), true)",
	"not_android32": "ifeq ($(TARGET_IS_64_BIT), false)",
	"android64":     "ifeq ($(TARGET_IS_64_BIT), false)",
	"not_android64": "ifeq ($(TARGET_IS_64_BIT), true)",
}

var targetToHostModuleRule = map[string]string{
	"BUILD_SHARED_LIBRARY": "BUILD_HOST_SHARED_LIBRARY",
	"BUILD_STATIC_LIBRARY": "BUILD_HOST_STATIC_LIBRARY",
	"BUILD_EXECUTABLE":     "BUILD_HOST_EXECUTABLE",
	"BUILD_NATIVE_TEST":    "BUILD_HOST_NATIVE_TEST",
	"BUILD_JAVA_LIBRARY":   "BUILD_HOST_JAVA_LIBRARY",
}

var productVariableConditionals = map[string]struct{conditional, value string}{
	"device_uses_jemalloc": {"ifneq ($(MALLOC_IMPL),dlmalloc)", ""},
	"device_uses_dlmalloc": {"ifeq ($(MALLOC_IMPL),dlmalloc)", ""},
	"device_uses_logd":     {"ifneq ($(TARGET_USES_LOGD),false)", ""},
	"dlmalloc_alignment":   {"ifdef DLMALLOC_ALIGNMENT", "$(DLMALLOC_ALIGNMENT)"},
}
