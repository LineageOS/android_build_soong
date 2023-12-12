package bp2build

import (
	"reflect"
	"strings"

	"android/soong/android"
	"github.com/google/blueprint/proptools"
)

type BazelFile struct {
	Dir      string
	Basename string
	Contents string
}

func CreateBazelFiles(ruleShims map[string]RuleShim, buildToTargets map[string]BazelTargets, mode CodegenMode) []BazelFile {
	var files []BazelFile

	if mode == QueryView {
		// Write top level WORKSPACE.
		files = append(files, newFile("", "WORKSPACE", ""))

		// Used to denote that the top level directory is a package.
		files = append(files, newFile("", GeneratedBuildFileName, ""))

		files = append(files, newFile(bazelRulesSubDir, GeneratedBuildFileName, ""))

		// These files are only used for queryview.
		files = append(files, newFile(bazelRulesSubDir, "providers.bzl", providersBzl))

		for bzlFileName, ruleShim := range ruleShims {
			files = append(files, newFile(bazelRulesSubDir, bzlFileName+".bzl", ruleShim.content))
		}
		files = append(files, newFile(bazelRulesSubDir, "soong_module.bzl", generateSoongModuleBzl(ruleShims)))
	}

	files = append(files, createBuildFiles(buildToTargets, mode)...)

	return files
}

func createBuildFiles(buildToTargets map[string]BazelTargets, mode CodegenMode) []BazelFile {
	files := make([]BazelFile, 0, len(buildToTargets))
	for _, dir := range android.SortedKeys(buildToTargets) {
		targets := buildToTargets[dir]
		targets.sort()

		var content string
		if mode == QueryView {
			content = soongModuleLoad
		}
		if content != "" {
			// If there are load statements, add a couple of newlines.
			content += "\n\n"
		}
		content += targets.String()
		files = append(files, newFile(dir, GeneratedBuildFileName, content))
	}
	return files
}

func newFile(dir, basename, content string) BazelFile {
	return BazelFile{
		Dir:      dir,
		Basename: basename,
		Contents: content,
	}
}

const (
	bazelRulesSubDir = "build/bazel/queryview_rules"
)

var (
	// Certain module property names are blocklisted/ignored here, for the reasons commented.
	ignoredPropNames = map[string]bool{
		"name":               true, // redundant, since this is explicitly generated for every target
		"from":               true, // reserved keyword
		"in":                 true, // reserved keyword
		"size":               true, // reserved for tests
		"arch":               true, // interface prop type is not supported yet.
		"multilib":           true, // interface prop type is not supported yet.
		"target":             true, // interface prop type is not supported yet.
		"visibility":         true, // Bazel has native visibility semantics. Handle later.
		"features":           true, // There is already a built-in attribute 'features' which cannot be overridden.
		"for":                true, // reserved keyword, b/233579439
		"versions_with_info": true, // TODO(b/245730552) struct properties not fully supported
	}
)

func shouldGenerateAttribute(prop string) bool {
	return !ignoredPropNames[prop]
}

func shouldSkipStructField(field reflect.StructField) bool {
	if field.PkgPath != "" && !field.Anonymous {
		// Skip unexported fields. Some properties are
		// internal to Soong only, and these fields do not have PkgPath.
		return true
	}
	// fields with tag `blueprint:"mutated"` are exported to enable modification in mutators, etc.
	// but cannot be set in a .bp file
	if proptools.HasTag(field, "blueprint", "mutated") {
		return true
	}
	return false
}

// FIXME(b/168089390): In Bazel, rules ending with "_test" needs to be marked as
// testonly = True, forcing other rules that depend on _test rules to also be
// marked as testonly = True. This semantic constraint is not present in Soong.
// To work around, rename "*_test" rules to "*_test_".
func canonicalizeModuleType(moduleName string) string {
	if strings.HasSuffix(moduleName, "_test") {
		return moduleName + "_"
	}

	return moduleName
}
