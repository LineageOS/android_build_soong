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
	"LOCAL_SRC_FILES_EXCLUDE":             {"exclude_srcs", bpparser.List},
	"LOCAL_SHARED_LIBRARIES":              {"shared_libs", bpparser.List},
	"LOCAL_STATIC_LIBRARIES":              {"static_libs", bpparser.List},
	"LOCAL_WHOLE_STATIC_LIBRARIES":        {"whole_static_libs", bpparser.List},
	"LOCAL_SYSTEM_SHARED_LIBRARIES":       {"system_shared_libs", bpparser.List},
	"LOCAL_ASFLAGS":                       {"asflags", bpparser.List},
	"LOCAL_CLANG_ASFLAGS":                 {"clang_asflags", bpparser.List},
	"LOCAL_CFLAGS":                        {"cflags", bpparser.List},
	"LOCAL_CONLYFLAGS":                    {"conlyflags", bpparser.List},
	"LOCAL_CPPFLAGS":                      {"cppflags", bpparser.List},
	"LOCAL_REQUIRED_MODULES":              {"required", bpparser.List},
	"LOCAL_MODULE_TAGS":                   {"tags", bpparser.List},
	"LOCAL_LDLIBS":                        {"host_ldlibs", bpparser.List},
	"LOCAL_CLANG_CFLAGS":                  {"clang_cflags", bpparser.List},
	"LOCAL_YACCFLAGS":                     {"yaccflags", bpparser.List},
	"LOCAL_SANITIZE_RECOVER":              {"sanitize.recover", bpparser.List},
	"LOCAL_LOGTAGS_FILES":                 {"logtags", bpparser.List},
	"LOCAL_EXPORT_SHARED_LIBRARY_HEADERS": {"export_shared_lib_headers", bpparser.List},
	"LOCAL_EXPORT_STATIC_LIBRARY_HEADERS": {"export_static_lib_headers", bpparser.List},

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
	"LOCAL_NATIVE_COVERAGE":         {"native_coverage", bpparser.Bool},
	"LOCAL_NO_CRT":                  {"nocrt", bpparser.Bool},
	"LOCAL_ALLOW_UNDEFINED_SYMBOLS": {"allow_undefined_symbols", bpparser.Bool},
	"LOCAL_RTTI_FLAG":               {"rtti", bpparser.Bool},

	"LOCAL_NO_STANDARD_LIBRARIES": {"no_standard_libraries", bpparser.Bool},

	"LOCAL_EXPORT_PACKAGE_RESOURCES": {"export_package_resources", bpparser.Bool},
}

var rewriteProperties = map[string]struct {
	f func(file *bpFile, prefix string, value *mkparser.MakeString, append bool) error
}{
	"LOCAL_C_INCLUDES":            {localIncludeDirs},
	"LOCAL_EXPORT_C_INCLUDE_DIRS": {exportIncludeDirs},
	"LOCAL_MODULE_STEM":           {stem},
	"LOCAL_MODULE_HOST_OS":        {hostOs},
	"LOCAL_SRC_FILES":             {srcFiles},
	"LOCAL_SANITIZE":              {sanitize},
	"LOCAL_LDFLAGS":               {ldflags},
}

type listSplitFunc func(bpparser.Value) (string, *bpparser.Value, error)

func emptyList(value *bpparser.Value) bool {
	return value.Type == bpparser.List && value.Expression == nil && value.Variable == "" &&
		len(value.ListValue) == 0
}

func splitBpList(val *bpparser.Value, keyFunc listSplitFunc) (lists map[string]*bpparser.Value, err error) {
	lists = make(map[string]*bpparser.Value)

	if val.Expression != nil {
		listsA, err := splitBpList(&val.Expression.Args[0], keyFunc)
		if err != nil {
			return nil, err
		}

		listsB, err := splitBpList(&val.Expression.Args[1], keyFunc)
		if err != nil {
			return nil, err
		}

		for k, v := range listsA {
			if !emptyList(v) {
				lists[k] = v
			}
		}

		for k, vB := range listsB {
			if emptyList(vB) {
				continue
			}

			if vA, ok := lists[k]; ok {
				expression := *val.Expression
				lists[k] = &bpparser.Value{
					Type:       bpparser.List,
					Expression: &expression,
				}
				lists[k].Expression.Args = [2]bpparser.Value{*vA, *vB}
			} else {
				lists[k] = vB
			}
		}
	} else if val.Variable != "" {
		key, value, err := keyFunc(*val)
		if err != nil {
			return nil, err
		}
		if value.Type == bpparser.List {
			lists[key] = value
		} else {
			lists[key] = &bpparser.Value{
				Type:      bpparser.List,
				ListValue: []bpparser.Value{*value},
			}
		}
	} else {
		for _, v := range val.ListValue {
			key, value, err := keyFunc(v)
			if err != nil {
				return nil, err
			}
			if _, ok := lists[key]; !ok {
				lists[key] = &bpparser.Value{
					Type: bpparser.List,
				}
			}
			lists[key].ListValue = append(lists[key].ListValue, *value)
		}
	}

	return lists, nil
}

func splitLocalGlobalPath(value bpparser.Value) (string, *bpparser.Value, error) {
	if value.Variable == "LOCAL_PATH" {
		return "local", &bpparser.Value{
			Type:        bpparser.String,
			StringValue: ".",
		}, nil
	} else if value.Variable != "" {
		// TODO: Should we split variables?
		return "global", &value, nil
	}

	if value.Type != bpparser.String {
		return "", nil, fmt.Errorf("splitLocalGlobalPath expected a string, got %s", value.Type)
	}

	if value.Expression == nil {
		return "global", &value, nil
	}

	if value.Expression.Operator != '+' {
		return "global", &value, nil
	}

	firstOperand := value.Expression.Args[0]
	secondOperand := value.Expression.Args[1]
	if firstOperand.Type != bpparser.String {
		return "global", &value, nil
	}

	if firstOperand.Expression != nil {
		return "global", &value, nil
	}

	if firstOperand.Variable != "LOCAL_PATH" {
		return "global", &value, nil
	}

	if secondOperand.Expression == nil && secondOperand.Variable == "" {
		if strings.HasPrefix(secondOperand.StringValue, "/") {
			secondOperand.StringValue = secondOperand.StringValue[1:]
		}
	}
	return "local", &secondOperand, nil
}

func localIncludeDirs(file *bpFile, prefix string, value *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, value, bpparser.List)
	if err != nil {
		return err
	}

	lists, err := splitBpList(val, splitLocalGlobalPath)
	if err != nil {
		return err
	}

	if global, ok := lists["global"]; ok && !emptyList(global) {
		err = setVariable(file, appendVariable, prefix, "include_dirs", global, true)
		if err != nil {
			return err
		}
	}

	if local, ok := lists["local"]; ok && !emptyList(local) {
		err = setVariable(file, appendVariable, prefix, "local_include_dirs", local, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func exportIncludeDirs(file *bpFile, prefix string, value *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, value, bpparser.List)
	if err != nil {
		return err
	}

	lists, err := splitBpList(val, splitLocalGlobalPath)
	if err != nil {
		return err
	}

	if local, ok := lists["local"]; ok && !emptyList(local) {
		err = setVariable(file, appendVariable, prefix, "export_include_dirs", local, true)
		if err != nil {
			return err
		}
		appendVariable = true
	}

	// Add any paths that could not be converted to local relative paths to export_include_dirs
	// anyways, they will cause an error if they don't exist and can be fixed manually.
	if global, ok := lists["global"]; ok && !emptyList(global) {
		err = setVariable(file, appendVariable, prefix, "export_include_dirs", global, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func stem(file *bpFile, prefix string, value *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, value, bpparser.String)
	if err != nil {
		return err
	}
	varName := "stem"

	if val.Expression != nil && val.Expression.Operator == '+' &&
		val.Expression.Args[0].Variable == "LOCAL_MODULE" {
		varName = "suffix"
		val = &val.Expression.Args[1]
	}

	return setVariable(file, appendVariable, prefix, varName, val, true)
}

func hostOs(file *bpFile, prefix string, value *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, value, bpparser.List)
	if err != nil {
		return err
	}

	inList := func(s string) bool {
		for _, v := range val.ListValue {
			if v.StringValue == s {
				return true
			}
		}
		return false
	}

	falseValue := &bpparser.Value{
		Type:      bpparser.Bool,
		BoolValue: false,
	}

	trueValue := &bpparser.Value{
		Type:      bpparser.Bool,
		BoolValue: true,
	}

	if inList("windows") {
		err = setVariable(file, appendVariable, "target.windows", "enabled", trueValue, true)
	}

	if !inList("linux") && err == nil {
		err = setVariable(file, appendVariable, "target.linux", "enabled", falseValue, true)
	}

	if !inList("darwin") && err == nil {
		err = setVariable(file, appendVariable, "target.darwin", "enabled", falseValue, true)
	}

	return err
}

func splitSrcsLogtags(value bpparser.Value) (string, *bpparser.Value, error) {
	if value.Variable != "" {
		// TODO: attempt to split variables?
		return "srcs", &value, nil
	}

	if value.Type != bpparser.String {
		return "", nil, fmt.Errorf("splitSrcsLogtags expected a string, got %s", value.Type)
	}

	if value.Expression != nil {
		// TODO: attempt to handle expressions?
		return "srcs", &value, nil
	}

	if strings.HasSuffix(value.StringValue, ".logtags") {
		return "logtags", &value, nil
	}

	return "srcs", &value, nil
}

func srcFiles(file *bpFile, prefix string, value *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, value, bpparser.List)
	if err != nil {
		return err
	}

	lists, err := splitBpList(val, splitSrcsLogtags)

	if srcs, ok := lists["srcs"]; ok && !emptyList(srcs) {
		err = setVariable(file, appendVariable, prefix, "srcs", srcs, true)
		if err != nil {
			return err
		}
	}

	if logtags, ok := lists["logtags"]; ok && !emptyList(logtags) {
		err = setVariable(file, true, prefix, "logtags", logtags, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func sanitize(file *bpFile, prefix string, mkvalue *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, mkvalue, bpparser.List)
	if err != nil {
		return err
	}

	lists, err := splitBpList(val, func(value bpparser.Value) (string, *bpparser.Value, error) {
		if value.Variable != "" {
			return "vars", &value, nil
		}

		if value.Expression != nil {
			file.errorf(mkvalue, "unknown sanitize expression")
			return "unknown", &value, nil
		}

		switch value.StringValue {
		case "never", "address", "coverage", "integer", "thread", "undefined":
			bpTrue := bpparser.Value{
				Type:      bpparser.Bool,
				BoolValue: true,
			}
			return value.StringValue, &bpTrue, nil
		default:
			file.errorf(mkvalue, "unknown sanitize argument: %s", value.StringValue)
			return "unknown", &value, nil
		}
	})
	if err != nil {
		return err
	}

	for k, v := range lists {
		if emptyList(v) {
			continue
		}

		switch k {
		case "never", "address", "coverage", "integer", "thread", "undefined":
			err = setVariable(file, false, prefix, "sanitize."+k, &lists[k].ListValue[0], true)
		case "unknown":
			// Nothing, we already added the error above
		case "vars":
			fallthrough
		default:
			err = setVariable(file, true, prefix, "sanitize", v, true)
		}

		if err != nil {
			return err
		}
	}

	return err
}

func ldflags(file *bpFile, prefix string, mkvalue *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, mkvalue, bpparser.List)
	if err != nil {
		return err
	}

	lists, err := splitBpList(val, func(value bpparser.Value) (string, *bpparser.Value, error) {
		// Anything other than "-Wl,--version_script," + LOCAL_PATH + "<path>" matches ldflags
		if value.Variable != "" || value.Expression == nil {
			return "ldflags", &value, nil
		}

		exp := value.Expression.Args[0]
		if exp.Variable != "" || exp.Expression == nil || exp.Expression.Args[0].StringValue != "-Wl,--version-script," {
			return "ldflags", &value, nil
		}

		if exp.Expression.Args[1].Variable != "LOCAL_PATH" {
			file.errorf(mkvalue, "Unrecognized version-script")
			return "ldflags", &value, nil
		}

		value.Expression.Args[1].StringValue = strings.TrimPrefix(value.Expression.Args[1].StringValue, "/")

		return "version", &value.Expression.Args[1], nil
	})
	if err != nil {
		return err
	}

	if ldflags, ok := lists["ldflags"]; ok && !emptyList(ldflags) {
		err = setVariable(file, appendVariable, prefix, "ldflags", ldflags, true)
		if err != nil {
			return err
		}
	}

	if version_script, ok := lists["version"]; ok && !emptyList(version_script) {
		if len(version_script.ListValue) > 1 {
			file.errorf(mkvalue, "multiple version scripts found?")
		}
		err = setVariable(file, false, prefix, "version_script", &version_script.ListValue[0], true)
		if err != nil {
			return err
		}
	}

	return nil
}

var deleteProperties = map[string]struct{}{
	"LOCAL_CPP_EXTENSION": struct{}{},
}

// Shorter suffixes of other suffixes must be at the end of the list
var propertyPrefixes = []struct{ mk, bp string }{
	{"arm", "arch.arm"},
	{"arm64", "arch.arm64"},
	{"mips", "arch.mips"},
	{"mips64", "arch.mips64"},
	{"x86", "arch.x86"},
	{"x86_64", "arch.x86_64"},
	{"32", "multilib.lib32"},
	// 64 must be after x86_64
	{"64", "multilib.lib64"},
	{"darwin", "target.darwin"},
	{"linux", "target.linux"},
	{"windows", "target.windows"},
}

var conditionalTranslations = map[string]map[bool]string{
	"($(HOST_OS),darwin)": {
		true:  "target.darwin",
		false: "target.not_darwin"},
	"($(HOST_OS), darwin)": {
		true:  "target.darwin",
		false: "target.not_darwin"},
	"($(HOST_OS),windows)": {
		true:  "target.windows",
		false: "target.not_windows"},
	"($(HOST_OS), windows)": {
		true:  "target.windows",
		false: "target.not_windows"},
	"($(HOST_OS),linux)": {
		true:  "target.linux",
		false: "target.not_linux"},
	"($(HOST_OS), linux)": {
		true:  "target.linux",
		false: "target.not_linux"},
	"($(BUILD_OS),darwin)": {
		true:  "target.darwin",
		false: "target.not_darwin"},
	"($(BUILD_OS), darwin)": {
		true:  "target.darwin",
		false: "target.not_darwin"},
	"($(BUILD_OS),linux)": {
		true:  "target.linux",
		false: "target.not_linux"},
	"($(BUILD_OS), linux)": {
		true:  "target.linux",
		false: "target.not_linux"},
	"(,$(TARGET_BUILD_APPS))": {
		false: "product_variables.unbundled_build",
	},
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
