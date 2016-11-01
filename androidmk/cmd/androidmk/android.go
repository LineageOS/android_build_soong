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
	bpparser.Type
}{
	// String properties
	"LOCAL_MODULE":               {"name", bpparser.StringType},
	"LOCAL_MODULE_CLASS":         {"class", bpparser.StringType},
	"LOCAL_CXX_STL":              {"stl", bpparser.StringType},
	"LOCAL_STRIP_MODULE":         {"strip", bpparser.StringType},
	"LOCAL_MULTILIB":             {"compile_multilib", bpparser.StringType},
	"LOCAL_ARM_MODE_HACK":        {"instruction_set", bpparser.StringType},
	"LOCAL_SDK_VERSION":          {"sdk_version", bpparser.StringType},
	"LOCAL_NDK_STL_VARIANT":      {"stl", bpparser.StringType},
	"LOCAL_JAR_MANIFEST":         {"manifest", bpparser.StringType},
	"LOCAL_JARJAR_RULES":         {"jarjar_rules", bpparser.StringType},
	"LOCAL_CERTIFICATE":          {"certificate", bpparser.StringType},
	"LOCAL_PACKAGE_NAME":         {"name", bpparser.StringType},
	"LOCAL_MODULE_RELATIVE_PATH": {"relative_install_path", bpparser.StringType},
	"LOCAL_PROTOC_OPTIMIZE_TYPE": {"proto.type", bpparser.StringType},

	// List properties
	"LOCAL_SRC_FILES_EXCLUDE":             {"exclude_srcs", bpparser.ListType},
	"LOCAL_SHARED_LIBRARIES":              {"shared_libs", bpparser.ListType},
	"LOCAL_STATIC_LIBRARIES":              {"static_libs", bpparser.ListType},
	"LOCAL_WHOLE_STATIC_LIBRARIES":        {"whole_static_libs", bpparser.ListType},
	"LOCAL_SYSTEM_SHARED_LIBRARIES":       {"system_shared_libs", bpparser.ListType},
	"LOCAL_ASFLAGS":                       {"asflags", bpparser.ListType},
	"LOCAL_CLANG_ASFLAGS":                 {"clang_asflags", bpparser.ListType},
	"LOCAL_CFLAGS":                        {"cflags", bpparser.ListType},
	"LOCAL_CONLYFLAGS":                    {"conlyflags", bpparser.ListType},
	"LOCAL_CPPFLAGS":                      {"cppflags", bpparser.ListType},
	"LOCAL_REQUIRED_MODULES":              {"required", bpparser.ListType},
	"LOCAL_MODULE_TAGS":                   {"tags", bpparser.ListType},
	"LOCAL_LDLIBS":                        {"host_ldlibs", bpparser.ListType},
	"LOCAL_CLANG_CFLAGS":                  {"clang_cflags", bpparser.ListType},
	"LOCAL_YACCFLAGS":                     {"yaccflags", bpparser.ListType},
	"LOCAL_SANITIZE_RECOVER":              {"sanitize.recover", bpparser.ListType},
	"LOCAL_LOGTAGS_FILES":                 {"logtags", bpparser.ListType},
	"LOCAL_EXPORT_SHARED_LIBRARY_HEADERS": {"export_shared_lib_headers", bpparser.ListType},
	"LOCAL_EXPORT_STATIC_LIBRARY_HEADERS": {"export_static_lib_headers", bpparser.ListType},
	"LOCAL_INIT_RC":                       {"init_rc", bpparser.ListType},
	"LOCAL_TIDY_FLAGS":                    {"tidy_flags", bpparser.ListType},
	// TODO: This is comma-seperated, not space-separated
	"LOCAL_TIDY_CHECKS": {"tidy_checks", bpparser.ListType},

	"LOCAL_JAVA_RESOURCE_DIRS":    {"java_resource_dirs", bpparser.ListType},
	"LOCAL_JAVACFLAGS":            {"javacflags", bpparser.ListType},
	"LOCAL_DX_FLAGS":              {"dxflags", bpparser.ListType},
	"LOCAL_JAVA_LIBRARIES":        {"java_libs", bpparser.ListType},
	"LOCAL_STATIC_JAVA_LIBRARIES": {"java_static_libs", bpparser.ListType},
	"LOCAL_AIDL_INCLUDES":         {"aidl_includes", bpparser.ListType},
	"LOCAL_AAPT_FLAGS":            {"aaptflags", bpparser.ListType},
	"LOCAL_PACKAGE_SPLITS":        {"package_splits", bpparser.ListType},

	// Bool properties
	"LOCAL_IS_HOST_MODULE":          {"host", bpparser.BoolType},
	"LOCAL_CLANG":                   {"clang", bpparser.BoolType},
	"LOCAL_FORCE_STATIC_EXECUTABLE": {"static_executable", bpparser.BoolType},
	"LOCAL_NATIVE_COVERAGE":         {"native_coverage", bpparser.BoolType},
	"LOCAL_NO_CRT":                  {"nocrt", bpparser.BoolType},
	"LOCAL_ALLOW_UNDEFINED_SYMBOLS": {"allow_undefined_symbols", bpparser.BoolType},
	"LOCAL_RTTI_FLAG":               {"rtti", bpparser.BoolType},
	"LOCAL_NO_STANDARD_LIBRARIES":   {"no_standard_libraries", bpparser.BoolType},
	"LOCAL_PACK_MODULE_RELOCATIONS": {"pack_relocations", bpparser.BoolType},
	"LOCAL_TIDY":                    {"tidy", bpparser.BoolType},

	"LOCAL_EXPORT_PACKAGE_RESOURCES": {"export_package_resources", bpparser.BoolType},
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

type listSplitFunc func(bpparser.Expression) (string, bpparser.Expression, error)

func emptyList(value bpparser.Expression) bool {
	if list, ok := value.(*bpparser.List); ok {
		return len(list.Values) == 0
	}
	return false
}

func splitBpList(val bpparser.Expression, keyFunc listSplitFunc) (lists map[string]bpparser.Expression, err error) {
	lists = make(map[string]bpparser.Expression)

	switch val := val.(type) {
	case *bpparser.Operator:
		listsA, err := splitBpList(val.Args[0], keyFunc)
		if err != nil {
			return nil, err
		}

		listsB, err := splitBpList(val.Args[1], keyFunc)
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
				expression := val.Copy().(*bpparser.Operator)
				expression.Args = [2]bpparser.Expression{vA, vB}
				lists[k] = expression
			} else {
				lists[k] = vB
			}
		}
	case *bpparser.Variable:
		key, value, err := keyFunc(val)
		if err != nil {
			return nil, err
		}
		if value.Type() == bpparser.ListType {
			lists[key] = value
		} else {
			lists[key] = &bpparser.List{
				Values: []bpparser.Expression{value},
			}
		}
	case *bpparser.List:
		for _, v := range val.Values {
			key, value, err := keyFunc(v)
			if err != nil {
				return nil, err
			}
			l := lists[key]
			if l == nil {
				l = &bpparser.List{}
			}
			l.(*bpparser.List).Values = append(l.(*bpparser.List).Values, value)
			lists[key] = l
		}
	default:
		panic(fmt.Errorf("unexpected type %t", val))
	}

	return lists, nil
}

func splitLocalGlobalPath(value bpparser.Expression) (string, bpparser.Expression, error) {
	switch v := value.(type) {
	case *bpparser.Variable:
		if v.Name == "LOCAL_PATH" {
			return "local", &bpparser.String{
				Value: ".",
			}, nil
		} else {
			// TODO: Should we split variables?
			return "global", value, nil
		}
	case *bpparser.Operator:
		if v.Type() != bpparser.StringType {
			return "", nil, fmt.Errorf("splitLocalGlobalPath expected a string, got %s", value.Type)
		}

		if v.Operator != '+' {
			return "global", value, nil
		}

		firstOperand := v.Args[0]
		secondOperand := v.Args[1]
		if firstOperand.Type() != bpparser.StringType {
			return "global", value, nil
		}

		if _, ok := firstOperand.(*bpparser.Operator); ok {
			return "global", value, nil
		}

		if variable, ok := firstOperand.(*bpparser.Variable); !ok || variable.Name != "LOCAL_PATH" {
			return "global", value, nil
		}

		local := secondOperand
		if s, ok := secondOperand.(*bpparser.String); ok {
			if strings.HasPrefix(s.Value, "/") {
				s.Value = s.Value[1:]
			}
		}
		return "local", local, nil
	case *bpparser.String:
		return "global", value, nil
	default:
		return "", nil, fmt.Errorf("splitLocalGlobalPath expected a string, got %s", value.Type)

	}
}

func localIncludeDirs(file *bpFile, prefix string, value *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, value, bpparser.ListType)
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
	val, err := makeVariableToBlueprint(file, value, bpparser.ListType)
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
	val, err := makeVariableToBlueprint(file, value, bpparser.StringType)
	if err != nil {
		return err
	}
	varName := "stem"

	if exp, ok := val.(*bpparser.Operator); ok && exp.Operator == '+' {
		if variable, ok := exp.Args[0].(*bpparser.Variable); ok && variable.Name == "LOCAL_MODULE" {
			varName = "suffix"
			val = exp.Args[1]
		}
	}

	return setVariable(file, appendVariable, prefix, varName, val, true)
}

func hostOs(file *bpFile, prefix string, value *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, value, bpparser.ListType)
	if err != nil {
		return err
	}

	inList := func(s string) bool {
		for _, v := range val.(*bpparser.List).Values {
			if v.(*bpparser.String).Value == s {
				return true
			}
		}
		return false
	}

	falseValue := &bpparser.Bool{
		Value: false,
	}

	trueValue := &bpparser.Bool{
		Value: true,
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

func splitSrcsLogtags(value bpparser.Expression) (string, bpparser.Expression, error) {
	switch v := value.(type) {
	case *bpparser.Variable:
		// TODO: attempt to split variables?
		return "srcs", value, nil
	case *bpparser.Operator:
		// TODO: attempt to handle expressions?
		return "srcs", value, nil
	case *bpparser.String:
		if strings.HasSuffix(v.Value, ".logtags") {
			return "logtags", value, nil
		}
		return "srcs", value, nil
	default:
		return "", nil, fmt.Errorf("splitSrcsLogtags expected a string, got %s", value.Type())
	}

}

func srcFiles(file *bpFile, prefix string, value *mkparser.MakeString, appendVariable bool) error {
	val, err := makeVariableToBlueprint(file, value, bpparser.ListType)
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
	val, err := makeVariableToBlueprint(file, mkvalue, bpparser.ListType)
	if err != nil {
		return err
	}

	lists, err := splitBpList(val, func(value bpparser.Expression) (string, bpparser.Expression, error) {
		switch v := value.(type) {
		case *bpparser.Variable:
			return "vars", value, nil
		case *bpparser.Operator:
			file.errorf(mkvalue, "unknown sanitize expression")
			return "unknown", value, nil
		case *bpparser.String:
			switch v.Value {
			case "never", "address", "coverage", "integer", "thread", "undefined":
				bpTrue := &bpparser.Bool{
					Value: true,
				}
				return v.Value, bpTrue, nil
			default:
				file.errorf(mkvalue, "unknown sanitize argument: %s", v.Value)
				return "unknown", value, nil
			}
		default:
			return "", nil, fmt.Errorf("sanitize expected a string, got %s", value.Type())
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
			err = setVariable(file, false, prefix, "sanitize."+k, lists[k].(*bpparser.List).Values[0], true)
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
	val, err := makeVariableToBlueprint(file, mkvalue, bpparser.ListType)
	if err != nil {
		return err
	}

	lists, err := splitBpList(val, func(value bpparser.Expression) (string, bpparser.Expression, error) {
		// Anything other than "-Wl,--version_script," + LOCAL_PATH + "<path>" matches ldflags
		exp1, ok := value.(*bpparser.Operator)
		if !ok {
			return "ldflags", value, nil
		}

		exp2, ok := exp1.Args[0].(*bpparser.Operator)
		if !ok {
			return "ldflags", value, nil
		}

		if s, ok := exp2.Args[0].(*bpparser.String); !ok || s.Value != "-Wl,--version-script," {
			return "ldflags", value, nil
		}

		if v, ok := exp2.Args[1].(*bpparser.Variable); !ok || v.Name != "LOCAL_PATH" {
			file.errorf(mkvalue, "Unrecognized version-script")
			return "ldflags", value, nil
		}

		s, ok := exp1.Args[1].(*bpparser.String)
		if !ok {
			file.errorf(mkvalue, "Unrecognized version-script")
			return "ldflags", value, nil
		}

		s.Value = strings.TrimPrefix(s.Value, "/")

		return "version", s, nil
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
		if len(version_script.(*bpparser.List).Values) > 1 {
			file.errorf(mkvalue, "multiple version scripts found?")
		}
		err = setVariable(file, false, prefix, "version_script", version_script.(*bpparser.List).Values[0], true)
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
