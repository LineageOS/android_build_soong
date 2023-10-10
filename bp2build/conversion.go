package bp2build

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"android/soong/android"
	"android/soong/apex"
	"android/soong/cc"
	cc_config "android/soong/cc/config"
	java_config "android/soong/java/config"
	rust_config "android/soong/rust/config"
	"android/soong/starlark_fmt"

	"github.com/google/blueprint/proptools"
)

type BazelFile struct {
	Dir      string
	Basename string
	Contents string
}

// createSoongInjectionDirFiles returns most of the files to write to the soong_injection directory.
// Some other files also come from CreateProductConfigFiles
func createSoongInjectionDirFiles(ctx *CodegenContext, metrics CodegenMetrics) ([]BazelFile, error) {
	cfg := ctx.Config()
	var files []BazelFile

	files = append(files, newFile("android", GeneratedBuildFileName, "")) // Creates a //cc_toolchain package.
	files = append(files, newFile("android", "constants.bzl", android.BazelCcToolchainVars(cfg)))

	files = append(files, newFile("cc_toolchain", GeneratedBuildFileName, "")) // Creates a //cc_toolchain package.
	files = append(files, newFile("cc_toolchain", "config_constants.bzl", cc_config.BazelCcToolchainVars(cfg)))
	files = append(files, newFile("cc_toolchain", "sanitizer_constants.bzl", cc.BazelCcSanitizerToolchainVars(cfg)))

	files = append(files, newFile("java_toolchain", GeneratedBuildFileName, "")) // Creates a //java_toolchain package.
	files = append(files, newFile("java_toolchain", "constants.bzl", java_config.BazelJavaToolchainVars(cfg)))

	files = append(files, newFile("rust_toolchain", GeneratedBuildFileName, "")) // Creates a //rust_toolchain package.
	files = append(files, newFile("rust_toolchain", "constants.bzl", rust_config.BazelRustToolchainVars(cfg)))

	files = append(files, newFile("apex_toolchain", GeneratedBuildFileName, "")) // Creates a //apex_toolchain package.
	apexToolchainVars, err := apex.BazelApexToolchainVars()
	if err != nil {
		return nil, err
	}
	files = append(files, newFile("apex_toolchain", "constants.bzl", apexToolchainVars))

	if buf, err := json.MarshalIndent(metrics.convertedModuleWithType, "", "  "); err != nil {
		return []BazelFile{}, err
	} else {
		files = append(files, newFile("metrics", "converted_modules.json", string(buf)))
	}

	convertedModulePathMap, err := json.MarshalIndent(metrics.convertedModulePathMap, "", "\t")
	if err != nil {
		panic(err)
	}
	files = append(files, newFile("metrics", GeneratedBuildFileName, "")) // Creates a //metrics package.
	files = append(files, newFile("metrics", "converted_modules_path_map.json", string(convertedModulePathMap)))
	files = append(files, newFile("metrics", "converted_modules_path_map.bzl", "modules = "+strings.ReplaceAll(string(convertedModulePathMap), "\\", "\\\\")))

	files = append(files, newFile("product_config", "soong_config_variables.bzl", cfg.Bp2buildSoongConfigDefinitions.String()))

	files = append(files, newFile("product_config", "arch_configuration.bzl", android.StarlarkArchConfigurations()))

	apiLevelsMap, err := android.GetApiLevelsMap(cfg)
	if err != nil {
		return nil, err
	}
	apiLevelsContent, err := json.Marshal(apiLevelsMap)
	if err != nil {
		return nil, err
	}
	files = append(files, newFile("api_levels", GeneratedBuildFileName, `exports_files(["api_levels.json"])`))
	// TODO(b/269691302)  value of apiLevelsContent is product variable dependent and should be avoided for soong injection
	files = append(files, newFile("api_levels", "api_levels.json", string(apiLevelsContent)))
	files = append(files, newFile("api_levels", "platform_versions.bzl", platformVersionContents(cfg)))

	files = append(files, newFile("allowlists", GeneratedBuildFileName, ""))
	// TODO(b/262781701): Create an alternate soong_build entrypoint for writing out these files only when requested
	files = append(files, newFile("allowlists", "mixed_build_prod_allowlist.txt", strings.Join(android.GetBazelEnabledModules(android.BazelProdMode), "\n")+"\n"))
	files = append(files, newFile("allowlists", "mixed_build_staging_allowlist.txt", strings.Join(android.GetBazelEnabledModules(android.BazelStagingMode), "\n")+"\n"))

	return files, nil
}

func platformVersionContents(cfg android.Config) string {
	// Despite these coming from cfg.productVariables, they are actually hardcoded in global
	// makefiles, not set in individual product config makesfiles, so they're safe to just export
	// and load() directly.

	platformVersionActiveCodenames := make([]string, 0, len(cfg.PlatformVersionActiveCodenames()))
	for _, codename := range cfg.PlatformVersionActiveCodenames() {
		platformVersionActiveCodenames = append(platformVersionActiveCodenames, fmt.Sprintf("%q", codename))
	}

	platformSdkVersion := "None"
	if cfg.RawPlatformSdkVersion() != nil {
		platformSdkVersion = strconv.Itoa(*cfg.RawPlatformSdkVersion())
	}

	return fmt.Sprintf(`
platform_versions = struct(
    platform_sdk_final = %s,
    platform_sdk_version = %s,
    platform_sdk_codename = %q,
    platform_version_active_codenames = [%s],
)
`, starlark_fmt.PrintBool(cfg.PlatformSdkFinal()), platformSdkVersion, cfg.PlatformSdkCodename(), strings.Join(platformVersionActiveCodenames, ", "))
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
		if mode == Bp2Build {
			content = `# READ THIS FIRST:
# This file was automatically generated by bp2build for the Bazel migration project.
# Feel free to edit or test it, but do *not* check it into your version control system.
`
			content += targets.LoadStatements()
			content += "\n\n"
			// Get package rule from the handcrafted BUILD file, otherwise emit the default one.
			prText := "package(default_visibility = [\"//visibility:public\"])\n"
			if pr := targets.packageRule(); pr != nil {
				prText = pr.content
			}
			content += prText
		} else if mode == QueryView {
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

	// additional files:
	//  * workspace file
	//  * base BUILD file
	//  * rules BUILD file
	//  * rules providers.bzl file
	//  * rules soong_module.bzl file
	numAdditionalFiles = 5
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
