package bp2build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"android/soong/android"
	"android/soong/android/soongconfig"
	"android/soong/starlark_import"

	"github.com/google/blueprint/proptools"
	"go.starlark.net/starlark"
)

type createProductConfigFilesResult struct {
	injectionFiles  []BazelFile
	bp2buildFiles   []BazelFile
	bp2buildTargets map[string]BazelTargets
}

type bazelLabel struct {
	repo   string
	pkg    string
	target string
}

func (l *bazelLabel) Less(other *bazelLabel) bool {
	if l.repo < other.repo {
		return true
	}
	if l.repo > other.repo {
		return false
	}
	if l.pkg < other.pkg {
		return true
	}
	if l.pkg > other.pkg {
		return false
	}
	return l.target < other.target
}

func (l *bazelLabel) String() string {
	return fmt.Sprintf("@%s//%s:%s", l.repo, l.pkg, l.target)
}

func createProductConfigFiles(
	ctx *CodegenContext,
	metrics CodegenMetrics) (createProductConfigFilesResult, error) {
	cfg := &ctx.config
	targetProduct := "unknown"
	if cfg.HasDeviceProduct() {
		targetProduct = cfg.DeviceProduct()
	}
	targetBuildVariant := "user"
	if cfg.Eng() {
		targetBuildVariant = "eng"
	} else if cfg.Debuggable() {
		targetBuildVariant = "userdebug"
	}

	var res createProductConfigFilesResult

	productVariablesFileName := cfg.ProductVariablesFileName
	if !strings.HasPrefix(productVariablesFileName, "/") {
		productVariablesFileName = filepath.Join(ctx.topDir, productVariablesFileName)
	}
	productVariablesBytes, err := os.ReadFile(productVariablesFileName)
	if err != nil {
		return res, err
	}
	productVariables := android.ProductVariables{}
	err = json.Unmarshal(productVariablesBytes, &productVariables)
	if err != nil {
		return res, err
	}

	currentProductFolder := fmt.Sprintf("build/bazel/products/%s", targetProduct)
	if len(productVariables.PartitionVarsForBazelMigrationOnlyDoNotUse.ProductDirectory) > 0 {
		currentProductFolder = fmt.Sprintf("%s%s", productVariables.PartitionVarsForBazelMigrationOnlyDoNotUse.ProductDirectory, targetProduct)
	}

	productReplacer := strings.NewReplacer(
		"{PRODUCT}", targetProduct,
		"{VARIANT}", targetBuildVariant,
		"{PRODUCT_FOLDER}", currentProductFolder)

	productsForTestingMap, err := starlark_import.GetStarlarkValue[map[string]map[string]starlark.Value]("products_for_testing")
	if err != nil {
		return res, err
	}
	productsForTesting := android.SortedKeys(productsForTestingMap)
	for i := range productsForTesting {
		productsForTesting[i] = fmt.Sprintf("  \"@//build/bazel/tests/products:%s\",", productsForTesting[i])
	}

	productLabelsToVariables := make(map[bazelLabel]*android.ProductVariables)
	productLabelsToVariables[bazelLabel{
		repo:   "",
		pkg:    currentProductFolder,
		target: targetProduct,
	}] = &productVariables
	for product, productVariablesStarlark := range productsForTestingMap {
		productVariables, err := starlarkMapToProductVariables(productVariablesStarlark)
		if err != nil {
			return res, err
		}
		productLabelsToVariables[bazelLabel{
			repo:   "",
			pkg:    "build/bazel/tests/products",
			target: product,
		}] = &productVariables
	}

	res.bp2buildTargets = make(map[string]BazelTargets)
	res.bp2buildTargets[currentProductFolder] = append(res.bp2buildTargets[currentProductFolder], BazelTarget{
		name:        productReplacer.Replace("{PRODUCT}"),
		packageName: currentProductFolder,
		content: productReplacer.Replace(`android_product(
    name = "{PRODUCT}",
    soong_variables = _soong_variables,
)`),
		ruleClass: "android_product",
		loads: []BazelLoad{
			{
				file: ":soong.variables.bzl",
				symbols: []BazelLoadSymbol{{
					symbol: "variables",
					alias:  "_soong_variables",
				}},
			},
			{
				file:    "//build/bazel/product_config:android_product.bzl",
				symbols: []BazelLoadSymbol{{symbol: "android_product"}},
			},
		},
	})
	createTargets(productLabelsToVariables, res.bp2buildTargets)

	platformMappingContent, err := platformMappingContent(
		productLabelsToVariables,
		ctx.Config().Bp2buildSoongConfigDefinitions,
		metrics.convertedModulePathMap)
	if err != nil {
		return res, err
	}

	res.injectionFiles = []BazelFile{
		newFile(
			"product_config_platforms",
			"BUILD.bazel",
			productReplacer.Replace(`
package(default_visibility = [
	"@//build/bazel/product_config:__subpackages__",
	"@soong_injection//product_config_platforms:__subpackages__",
])

load("@//{PRODUCT_FOLDER}:soong.variables.bzl", _soong_variables = "variables")
load("@//build/bazel/product_config:android_product.bzl", "android_product")

# Bazel will qualify its outputs by the platform name. When switching between products, this
# means that soong-built files that depend on bazel-built files will suddenly get different
# dependency files, because the path changes, and they will be rebuilt. In order to avoid this
# extra rebuilding, make mixed builds always use a single platform so that the bazel artifacts
# are always under the same path.
android_product(
    name = "mixed_builds_product",
    soong_variables = _soong_variables,
    extra_constraints = ["@//build/bazel/platforms:mixed_builds"],
)
`)),
		newFile(
			"product_config_platforms",
			"product_labels.bzl",
			productReplacer.Replace(`
# This file keeps a list of all the products in the android source tree, because they're
# discovered as part of a preprocessing step before bazel runs.
# TODO: When we start generating the platforms for more than just the
# currently lunched product, they should all be listed here
product_labels = [
  "@soong_injection//product_config_platforms:mixed_builds_product",
  "@//{PRODUCT_FOLDER}:{PRODUCT}",
`)+strings.Join(productsForTesting, "\n")+"\n]\n"),
		newFile(
			"product_config_platforms",
			"common.bazelrc",
			productReplacer.Replace(`
build --platform_mappings=platform_mappings
build --platforms @//{PRODUCT_FOLDER}:{PRODUCT}_linux_x86_64
build --//build/bazel/product_config:target_build_variant={VARIANT}

build:android --platforms=@//{PRODUCT_FOLDER}:{PRODUCT}
build:linux_x86 --platforms=@//{PRODUCT_FOLDER}:{PRODUCT}_linux_x86
build:linux_x86_64 --platforms=@//{PRODUCT_FOLDER}:{PRODUCT}_linux_x86_64
build:linux_bionic_x86_64 --platforms=@//{PRODUCT_FOLDER}:{PRODUCT}_linux_bionic_x86_64
build:linux_musl_x86 --platforms=@//{PRODUCT_FOLDER}:{PRODUCT}_linux_musl_x86
build:linux_musl_x86_64 --platforms=@//{PRODUCT_FOLDER}:{PRODUCT}_linux_musl_x86_64
`)),
		newFile(
			"product_config_platforms",
			"linux.bazelrc",
			productReplacer.Replace(`
build --host_platform @//{PRODUCT_FOLDER}:{PRODUCT}_linux_x86_64
`)),
		newFile(
			"product_config_platforms",
			"darwin.bazelrc",
			productReplacer.Replace(`
build --host_platform @//{PRODUCT_FOLDER}:{PRODUCT}_darwin_x86_64
`)),
	}
	res.bp2buildFiles = []BazelFile{
		newFile(
			"",
			"platform_mappings",
			platformMappingContent),
		newFile(
			currentProductFolder,
			"soong.variables.bzl",
			`variables = json.decode("""`+strings.ReplaceAll(string(productVariablesBytes), "\\", "\\\\")+`""")`),
	}

	return res, nil
}

func platformMappingContent(
	productLabelToVariables map[bazelLabel]*android.ProductVariables,
	soongConfigDefinitions soongconfig.Bp2BuildSoongConfigDefinitions,
	convertedModulePathMap map[string]string) (string, error) {
	var result strings.Builder

	mergedConvertedModulePathMap := make(map[string]string)
	for k, v := range convertedModulePathMap {
		mergedConvertedModulePathMap[k] = v
	}
	additionalModuleNamesToPackages, err := starlark_import.GetStarlarkValue[map[string]string]("additional_module_names_to_packages")
	if err != nil {
		return "", err
	}
	for k, v := range additionalModuleNamesToPackages {
		mergedConvertedModulePathMap[k] = v
	}

	productLabels := make([]bazelLabel, 0, len(productLabelToVariables))
	for k := range productLabelToVariables {
		productLabels = append(productLabels, k)
	}
	sort.Slice(productLabels, func(i, j int) bool {
		return productLabels[i].Less(&productLabels[j])
	})
	result.WriteString("platforms:\n")
	for _, productLabel := range productLabels {
		platformMappingSingleProduct(productLabel, productLabelToVariables[productLabel], soongConfigDefinitions, mergedConvertedModulePathMap, &result)
	}
	return result.String(), nil
}

var bazelPlatformSuffixes = []string{
	"",
	"_darwin_arm64",
	"_darwin_x86_64",
	"_linux_bionic_arm64",
	"_linux_bionic_x86_64",
	"_linux_musl_x86",
	"_linux_musl_x86_64",
	"_linux_x86",
	"_linux_x86_64",
	"_windows_x86",
	"_windows_x86_64",
}

func platformMappingSingleProduct(
	label bazelLabel,
	productVariables *android.ProductVariables,
	soongConfigDefinitions soongconfig.Bp2BuildSoongConfigDefinitions,
	convertedModulePathMap map[string]string,
	result *strings.Builder) {

	platform_sdk_version := -1
	if productVariables.Platform_sdk_version != nil {
		platform_sdk_version = *productVariables.Platform_sdk_version
	}

	defaultAppCertificateFilegroup := "//build/bazel/utils:empty_filegroup"
	if proptools.String(productVariables.DefaultAppCertificate) != "" {
		defaultAppCertificateFilegroup = "@//" + filepath.Dir(proptools.String(productVariables.DefaultAppCertificate)) + ":generated_android_certificate_directory"
	}

	// TODO: b/301598690 - commas can't be escaped in a string-list passed in a platform mapping,
	// so commas are switched for ":" here, and must be back-substituted into commas
	// wherever the AAPTCharacteristics product config variable is used.
	AAPTConfig := []string{}
	for _, conf := range productVariables.AAPTConfig {
		AAPTConfig = append(AAPTConfig, strings.Replace(conf, ",", ":", -1))
	}

	for _, suffix := range bazelPlatformSuffixes {
		result.WriteString("  ")
		result.WriteString(label.String())
		result.WriteString(suffix)
		result.WriteString("\n")
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:aapt_characteristics=%s\n", proptools.String(productVariables.AAPTCharacteristics)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:aapt_config=%s\n", strings.Join(AAPTConfig, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:aapt_preferred_config=%s\n", proptools.String(productVariables.AAPTPreferredConfig)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:always_use_prebuilt_sdks=%t\n", proptools.Bool(productVariables.Always_use_prebuilt_sdks)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:arc=%t\n", proptools.Bool(productVariables.Arc)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:apex_global_min_sdk_version_override=%s\n", proptools.String(productVariables.ApexGlobalMinSdkVersionOverride)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:binder32bit=%t\n", proptools.Bool(productVariables.Binder32bit)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:build_from_text_stub=%t\n", proptools.Bool(productVariables.Build_from_text_stub)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:build_broken_incorrect_partition_images=%t\n", productVariables.BuildBrokenIncorrectPartitionImages))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:build_id=%s\n", proptools.String(productVariables.BuildId)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:build_version_tags=%s\n", strings.Join(productVariables.BuildVersionTags, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:cfi_exclude_paths=%s\n", strings.Join(productVariables.CFIExcludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:cfi_include_paths=%s\n", strings.Join(productVariables.CFIIncludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:compressed_apex=%t\n", proptools.Bool(productVariables.CompressedApex)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:default_app_certificate=%s\n", proptools.String(productVariables.DefaultAppCertificate)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:default_app_certificate_filegroup=%s\n", defaultAppCertificateFilegroup))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_abi=%s\n", strings.Join(productVariables.DeviceAbi, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_max_page_size_supported=%s\n", proptools.String(productVariables.DeviceMaxPageSizeSupported)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_name=%s\n", proptools.String(productVariables.DeviceName)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_page_size_agnostic=%t\n", proptools.Bool(productVariables.DevicePageSizeAgnostic)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_product=%s\n", proptools.String(productVariables.DeviceProduct)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_platform=%s\n", label.String()))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:enable_cfi=%t\n", proptools.BoolDefault(productVariables.EnableCFI, true)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:enforce_vintf_manifest=%t\n", proptools.Bool(productVariables.Enforce_vintf_manifest)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:malloc_not_svelte=%t\n", proptools.Bool(productVariables.Malloc_not_svelte)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:malloc_pattern_fill_contents=%t\n", proptools.Bool(productVariables.Malloc_pattern_fill_contents)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:malloc_zero_contents=%t\n", proptools.Bool(productVariables.Malloc_zero_contents)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:memtag_heap_exclude_paths=%s\n", strings.Join(productVariables.MemtagHeapExcludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:memtag_heap_async_include_paths=%s\n", strings.Join(productVariables.MemtagHeapAsyncIncludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:memtag_heap_sync_include_paths=%s\n", strings.Join(productVariables.MemtagHeapSyncIncludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:manifest_package_name_overrides=%s\n", strings.Join(productVariables.ManifestPackageNameOverrides, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:native_coverage=%t\n", proptools.Bool(productVariables.Native_coverage)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:platform_sdk_final=%t\n", proptools.Bool(productVariables.Platform_sdk_final)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:platform_security_patch=%s\n", proptools.String(productVariables.Platform_security_patch)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:platform_version_last_stable=%s\n", proptools.String(productVariables.Platform_version_last_stable)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:platform_version_name=%s\n", proptools.String(productVariables.Platform_version_name)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:product_brand=%s\n", productVariables.ProductBrand))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:product_manufacturer=%s\n", productVariables.ProductManufacturer))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:release_aconfig_flag_default_permission=%s\n", productVariables.ReleaseAconfigFlagDefaultPermission))
		// Empty string can't be used as label_flag on the bazel side
		if len(productVariables.ReleaseAconfigValueSets) > 0 {
			result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:release_aconfig_value_sets=%s\n", productVariables.ReleaseAconfigValueSets))
		}
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:release_version=%s\n", productVariables.ReleaseVersion))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:platform_sdk_version=%d\n", platform_sdk_version))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:safestack=%t\n", proptools.Bool(productVariables.Safestack)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:treble_linker_namespaces=%t\n", proptools.Bool(productVariables.Treble_linker_namespaces)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:tidy_checks=%s\n", proptools.String(productVariables.TidyChecks)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:uml=%t\n", proptools.Bool(productVariables.Uml)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:unbundled_build=%t\n", proptools.Bool(productVariables.Unbundled_build)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:unbundled_build_apps=%s\n", strings.Join(productVariables.Unbundled_build_apps, ",")))

		for _, override := range productVariables.CertificateOverrides {
			parts := strings.SplitN(override, ":", 2)
			if apexPath, ok := convertedModulePathMap[parts[0]]; ok {
				if overrideCertPath, ok := convertedModulePathMap[parts[1]]; ok {
					result.WriteString(fmt.Sprintf("    --%s:%s_certificate_override=%s:%s\n", apexPath, parts[0], overrideCertPath, parts[1]))
				}
			}
		}

		for _, namespace := range android.SortedKeys(productVariables.VendorVars) {
			for _, variable := range android.SortedKeys(productVariables.VendorVars[namespace]) {
				value := productVariables.VendorVars[namespace][variable]
				key := namespace + "__" + variable
				_, hasBool := soongConfigDefinitions.BoolVars[key]
				_, hasString := soongConfigDefinitions.StringVars[key]
				_, hasValue := soongConfigDefinitions.ValueVars[key]
				if !hasBool && !hasString && !hasValue {
					// Not all soong config variables are defined in Android.bp files. For example,
					// prebuilt_bootclasspath_fragment uses soong config variables in a nonstandard
					// way, that causes them to be present in the soong.variables file but not
					// defined in an Android.bp file. There's also nothing stopping you from setting
					// a variable in make that doesn't exist in soong. We only generate build
					// settings for the ones that exist in soong, so skip all others.
					continue
				}
				if hasBool && hasString || hasBool && hasValue || hasString && hasValue {
					panic(fmt.Sprintf("Soong config variable %s:%s appears to be of multiple types. bool? %t, string? %t, value? %t", namespace, variable, hasBool, hasString, hasValue))
				}
				if hasBool {
					// Logic copied from soongConfig.Bool()
					value = strings.ToLower(value)
					if value == "1" || value == "y" || value == "yes" || value == "on" || value == "true" {
						value = "true"
					} else {
						value = "false"
					}
				}
				result.WriteString(fmt.Sprintf("    --//build/bazel/product_config/soong_config_variables:%s=%s\n", strings.ToLower(key), value))
			}
		}
	}
}

func starlarkMapToProductVariables(in map[string]starlark.Value) (android.ProductVariables, error) {
	result := android.ProductVariables{}
	productVarsReflect := reflect.ValueOf(&result).Elem()
	for i := 0; i < productVarsReflect.NumField(); i++ {
		field := productVarsReflect.Field(i)
		fieldType := productVarsReflect.Type().Field(i)
		name := fieldType.Name
		if name == "BootJars" || name == "ApexBootJars" || name == "VendorSnapshotModules" ||
			name == "RecoverySnapshotModules" {
			// These variables have more complicated types, and we don't need them right now
			continue
		}
		if _, ok := in[name]; ok {
			if name == "VendorVars" {
				vendorVars, err := starlark_import.Unmarshal[map[string]map[string]string](in[name])
				if err != nil {
					return result, err
				}
				field.Set(reflect.ValueOf(vendorVars))
				continue
			}
			switch field.Type().Kind() {
			case reflect.Bool:
				val, err := starlark_import.Unmarshal[bool](in[name])
				if err != nil {
					return result, err
				}
				field.SetBool(val)
			case reflect.String:
				val, err := starlark_import.Unmarshal[string](in[name])
				if err != nil {
					return result, err
				}
				field.SetString(val)
			case reflect.Slice:
				if field.Type().Elem().Kind() != reflect.String {
					return result, fmt.Errorf("slices of types other than strings are unimplemented")
				}
				val, err := starlark_import.UnmarshalReflect(in[name], field.Type())
				if err != nil {
					return result, err
				}
				field.Set(val)
			case reflect.Pointer:
				switch field.Type().Elem().Kind() {
				case reflect.Bool:
					val, err := starlark_import.UnmarshalNoneable[bool](in[name])
					if err != nil {
						return result, err
					}
					field.Set(reflect.ValueOf(val))
				case reflect.String:
					val, err := starlark_import.UnmarshalNoneable[string](in[name])
					if err != nil {
						return result, err
					}
					field.Set(reflect.ValueOf(val))
				case reflect.Int:
					val, err := starlark_import.UnmarshalNoneable[int](in[name])
					if err != nil {
						return result, err
					}
					field.Set(reflect.ValueOf(val))
				default:
					return result, fmt.Errorf("pointers of types other than strings/bools are unimplemented: %s", field.Type().Elem().Kind().String())
				}
			default:
				return result, fmt.Errorf("unimplemented type: %s", field.Type().String())
			}
		}
	}

	result.Native_coverage = proptools.BoolPtr(
		proptools.Bool(result.GcovCoverage) ||
			proptools.Bool(result.ClangCoverage))

	return result, nil
}

func createTargets(productLabelsToVariables map[bazelLabel]*android.ProductVariables, res map[string]BazelTargets) {
	createGeneratedAndroidCertificateDirectories(productLabelsToVariables, res)
	createAvbKeyFilegroups(productLabelsToVariables, res)
	for label, variables := range productLabelsToVariables {
		createSystemPartition(label, &variables.PartitionVarsForBazelMigrationOnlyDoNotUse, res)
	}
}

func createGeneratedAndroidCertificateDirectories(productLabelsToVariables map[bazelLabel]*android.ProductVariables, targets map[string]BazelTargets) {
	var allDefaultAppCertificateDirs []string
	for _, productVariables := range productLabelsToVariables {
		if proptools.String(productVariables.DefaultAppCertificate) != "" {
			d := filepath.Dir(proptools.String(productVariables.DefaultAppCertificate))
			if !android.InList(d, allDefaultAppCertificateDirs) {
				allDefaultAppCertificateDirs = append(allDefaultAppCertificateDirs, d)
			}
		}
	}
	for _, dir := range allDefaultAppCertificateDirs {
		content := `filegroup(
    name = "generated_android_certificate_directory",
    srcs = glob([
        "*.pk8",
        "*.pem",
        "*.avbpubkey",
    ]),
    visibility = ["//visibility:public"],
)`
		targets[dir] = append(targets[dir], BazelTarget{
			name:        "generated_android_certificate_directory",
			packageName: dir,
			content:     content,
			ruleClass:   "filegroup",
		})
	}
}

func createAvbKeyFilegroups(productLabelsToVariables map[bazelLabel]*android.ProductVariables, targets map[string]BazelTargets) {
	var allAvbKeys []string
	for _, productVariables := range productLabelsToVariables {
		for _, partitionVariables := range productVariables.PartitionVarsForBazelMigrationOnlyDoNotUse.PartitionQualifiedVariables {
			if partitionVariables.BoardAvbKeyPath != "" {
				if !android.InList(partitionVariables.BoardAvbKeyPath, allAvbKeys) {
					allAvbKeys = append(allAvbKeys, partitionVariables.BoardAvbKeyPath)
				}
			}
		}
	}
	for _, key := range allAvbKeys {
		dir := filepath.Dir(key)
		name := filepath.Base(key)
		content := fmt.Sprintf(`filegroup(
    name = "%s_filegroup",
    srcs = ["%s"],
    visibility = ["//visibility:public"],
)`, name, name)
		targets[dir] = append(targets[dir], BazelTarget{
			name:        name + "_filegroup",
			packageName: dir,
			content:     content,
			ruleClass:   "filegroup",
		})
	}
}

func createSystemPartition(platformLabel bazelLabel, variables *android.PartitionVariables, targets map[string]BazelTargets) {
	if !variables.PartitionQualifiedVariables["system"].BuildingImage {
		return
	}
	qualifiedVariables := variables.PartitionQualifiedVariables["system"]

	imageProps := generateImagePropDictionary(variables, "system")
	imageProps["skip_fsck"] = "true"

	var properties strings.Builder
	for _, prop := range android.SortedKeys(imageProps) {
		properties.WriteString(prop)
		properties.WriteRune('=')
		properties.WriteString(imageProps[prop])
		properties.WriteRune('\n')
	}

	var extraProperties strings.Builder
	if variables.BoardAvbEnable {
		extraProperties.WriteString("    avb_enable = True,\n")
		extraProperties.WriteString(fmt.Sprintf("    avb_add_hashtree_footer_args = %q,\n", qualifiedVariables.BoardAvbAddHashtreeFooterArgs))
		keypath := qualifiedVariables.BoardAvbKeyPath
		if keypath != "" {
			extraProperties.WriteString(fmt.Sprintf("    avb_key = \"//%s:%s\",\n", filepath.Dir(keypath), filepath.Base(keypath)+"_filegroup"))
			extraProperties.WriteString(fmt.Sprintf("    avb_algorithm = %q,\n", qualifiedVariables.BoardAvbAlgorithm))
			extraProperties.WriteString(fmt.Sprintf("    avb_rollback_index = %s,\n", qualifiedVariables.BoardAvbRollbackIndex))
			extraProperties.WriteString(fmt.Sprintf("    avb_rollback_index_location = %s,\n", qualifiedVariables.BoardAvbRollbackIndexLocation))
		}
	}

	targets[platformLabel.pkg] = append(targets[platformLabel.pkg], BazelTarget{
		name:        "system_image",
		packageName: platformLabel.pkg,
		content: fmt.Sprintf(`partition(
    name = "system_image",
    base_staging_dir = "//build/bazel/bazel_sandwich:system_staging_dir",
    base_staging_dir_file_list = "//build/bazel/bazel_sandwich:system_staging_dir_file_list",
    root_dir = "//build/bazel/bazel_sandwich:root_staging_dir",
    selinux_file_contexts = "//build/bazel/bazel_sandwich:selinux_file_contexts",
    image_properties = """
%s
""",
%s
    type = "system",
)`, properties.String(), extraProperties.String()),
		ruleClass: "partition",
		loads: []BazelLoad{{
			file: "//build/bazel/rules/partitions:partition.bzl",
			symbols: []BazelLoadSymbol{{
				symbol: "partition",
			}},
		}},
	}, BazelTarget{
		name:        "system_image_test",
		packageName: platformLabel.pkg,
		content: `partition_diff_test(
    name = "system_image_test",
    partition1 = "//build/bazel/bazel_sandwich:make_system_image",
    partition2 = ":system_image",
)`,
		ruleClass: "partition_diff_test",
		loads: []BazelLoad{{
			file: "//build/bazel/rules/partitions/diff:partition_diff.bzl",
			symbols: []BazelLoadSymbol{{
				symbol: "partition_diff_test",
			}},
		}},
	}, BazelTarget{
		name:        "run_system_image_test",
		packageName: platformLabel.pkg,
		content: `run_test_in_build(
    name = "run_system_image_test",
    test = ":system_image_test",
)`,
		ruleClass: "run_test_in_build",
		loads: []BazelLoad{{
			file: "//build/bazel/bazel_sandwich:run_test_in_build.bzl",
			symbols: []BazelLoadSymbol{{
				symbol: "run_test_in_build",
			}},
		}},
	})
}

var allPartitionTypes = []string{
	"system",
	"vendor",
	"cache",
	"userdata",
	"product",
	"system_ext",
	"oem",
	"odm",
	"vendor_dlkm",
	"odm_dlkm",
	"system_dlkm",
}

// An equivalent of make's generate-image-prop-dictionary function
func generateImagePropDictionary(variables *android.PartitionVariables, partitionType string) map[string]string {
	partitionQualifiedVariables, ok := variables.PartitionQualifiedVariables[partitionType]
	if !ok {
		panic("Unknown partitionType: " + partitionType)
	}
	ret := map[string]string{}
	if partitionType == "system" {
		if len(variables.PartitionQualifiedVariables["system_other"].BoardPartitionSize) > 0 {
			ret["system_other_size"] = variables.PartitionQualifiedVariables["system_other"].BoardPartitionSize
		}
		if len(partitionQualifiedVariables.ProductHeadroom) > 0 {
			ret["system_headroom"] = partitionQualifiedVariables.ProductHeadroom
		}
		addCommonRoFlagsToImageProps(variables, partitionType, ret)
	}
	// TODO: other partition-specific logic
	if variables.TargetUserimagesUseExt2 {
		ret["fs_type"] = "ext2"
	} else if variables.TargetUserimagesUseExt3 {
		ret["fs_type"] = "ext3"
	} else if variables.TargetUserimagesUseExt4 {
		ret["fs_type"] = "ext4"
	}

	if !variables.TargetUserimagesSparseExtDisabled {
		ret["extfs_sparse_flag"] = "-s"
	}
	if !variables.TargetUserimagesSparseErofsDisabled {
		ret["erofs_sparse_flag"] = "-s"
	}
	if !variables.TargetUserimagesSparseSquashfsDisabled {
		ret["squashfs_sparse_flag"] = "-s"
	}
	if !variables.TargetUserimagesSparseF2fsDisabled {
		ret["f2fs_sparse_flag"] = "-S"
	}
	erofsCompressor := variables.BoardErofsCompressor
	if len(erofsCompressor) == 0 && hasErofsPartition(variables) {
		if len(variables.BoardErofsUseLegacyCompression) > 0 {
			erofsCompressor = "lz4"
		} else {
			erofsCompressor = "lz4hc,9"
		}
	}
	if len(erofsCompressor) > 0 {
		ret["erofs_default_compressor"] = erofsCompressor
	}
	if len(variables.BoardErofsCompressorHints) > 0 {
		ret["erofs_default_compress_hints"] = variables.BoardErofsCompressorHints
	}
	if len(variables.BoardErofsCompressorHints) > 0 {
		ret["erofs_default_compress_hints"] = variables.BoardErofsCompressorHints
	}
	if len(variables.BoardErofsPclusterSize) > 0 {
		ret["erofs_pcluster_size"] = variables.BoardErofsPclusterSize
	}
	if len(variables.BoardErofsShareDupBlocks) > 0 {
		ret["erofs_share_dup_blocks"] = variables.BoardErofsShareDupBlocks
	}
	if len(variables.BoardErofsUseLegacyCompression) > 0 {
		ret["erofs_use_legacy_compression"] = variables.BoardErofsUseLegacyCompression
	}
	if len(variables.BoardExt4ShareDupBlocks) > 0 {
		ret["ext4_share_dup_blocks"] = variables.BoardExt4ShareDupBlocks
	}
	if len(variables.BoardFlashLogicalBlockSize) > 0 {
		ret["flash_logical_block_size"] = variables.BoardFlashLogicalBlockSize
	}
	if len(variables.BoardFlashEraseBlockSize) > 0 {
		ret["flash_erase_block_size"] = variables.BoardFlashEraseBlockSize
	}
	if len(variables.BoardExt4ShareDupBlocks) > 0 {
		ret["ext4_share_dup_blocks"] = variables.BoardExt4ShareDupBlocks
	}
	if len(variables.BoardExt4ShareDupBlocks) > 0 {
		ret["ext4_share_dup_blocks"] = variables.BoardExt4ShareDupBlocks
	}
	for _, partitionType := range allPartitionTypes {
		if qualifiedVariables, ok := variables.PartitionQualifiedVariables[partitionType]; ok && len(qualifiedVariables.ProductVerityPartition) > 0 {
			ret[partitionType+"_verity_block_device"] = qualifiedVariables.ProductVerityPartition
		}
	}
	// TODO: Vboot
	// TODO: AVB
	if variables.BoardUsesRecoveryAsBoot {
		ret["recovery_as_boot"] = "true"
	}
	if variables.BoardBuildGkiBootImageWithoutRamdisk {
		ret["gki_boot_image_without_ramdisk"] = "true"
	}
	if variables.ProductUseDynamicPartitionSize {
		ret["use_dynamic_partition_size"] = "true"
	}
	if variables.CopyImagesForTargetFilesZip {
		ret["use_fixed_timestamp"] = "true"
	}
	return ret
}

// Soong equivalent of make's add-common-ro-flags-to-image-props
func addCommonRoFlagsToImageProps(variables *android.PartitionVariables, partitionType string, ret map[string]string) {
	partitionQualifiedVariables, ok := variables.PartitionQualifiedVariables[partitionType]
	if !ok {
		panic("Unknown partitionType: " + partitionType)
	}
	if len(partitionQualifiedVariables.BoardErofsCompressor) > 0 {
		ret[partitionType+"_erofs_compressor"] = partitionQualifiedVariables.BoardErofsCompressor
	}
	if len(partitionQualifiedVariables.BoardErofsCompressHints) > 0 {
		ret[partitionType+"_erofs_compress_hints"] = partitionQualifiedVariables.BoardErofsCompressHints
	}
	if len(partitionQualifiedVariables.BoardErofsPclusterSize) > 0 {
		ret[partitionType+"_erofs_pcluster_size"] = partitionQualifiedVariables.BoardErofsPclusterSize
	}
	if len(partitionQualifiedVariables.BoardExtfsRsvPct) > 0 {
		ret[partitionType+"_extfs_rsv_pct"] = partitionQualifiedVariables.BoardExtfsRsvPct
	}
	if len(partitionQualifiedVariables.BoardF2fsSloadCompressFlags) > 0 {
		ret[partitionType+"_f2fs_sldc_flags"] = partitionQualifiedVariables.BoardF2fsSloadCompressFlags
	}
	if len(partitionQualifiedVariables.BoardFileSystemCompress) > 0 {
		ret[partitionType+"_f2fs_compress"] = partitionQualifiedVariables.BoardFileSystemCompress
	}
	if len(partitionQualifiedVariables.BoardFileSystemType) > 0 {
		ret[partitionType+"_fs_type"] = partitionQualifiedVariables.BoardFileSystemType
	}
	if len(partitionQualifiedVariables.BoardJournalSize) > 0 {
		ret[partitionType+"_journal_size"] = partitionQualifiedVariables.BoardJournalSize
	}
	if len(partitionQualifiedVariables.BoardPartitionReservedSize) > 0 {
		ret[partitionType+"_reserved_size"] = partitionQualifiedVariables.BoardPartitionReservedSize
	}
	if len(partitionQualifiedVariables.BoardPartitionSize) > 0 {
		ret[partitionType+"_size"] = partitionQualifiedVariables.BoardPartitionSize
	}
	if len(partitionQualifiedVariables.BoardSquashfsBlockSize) > 0 {
		ret[partitionType+"_squashfs_block_size"] = partitionQualifiedVariables.BoardSquashfsBlockSize
	}
	if len(partitionQualifiedVariables.BoardSquashfsCompressor) > 0 {
		ret[partitionType+"_squashfs_compressor"] = partitionQualifiedVariables.BoardSquashfsCompressor
	}
	if len(partitionQualifiedVariables.BoardSquashfsCompressorOpt) > 0 {
		ret[partitionType+"_squashfs_compressor_opt"] = partitionQualifiedVariables.BoardSquashfsCompressorOpt
	}
	if len(partitionQualifiedVariables.BoardSquashfsDisable4kAlign) > 0 {
		ret[partitionType+"_squashfs_disable_4k_align"] = partitionQualifiedVariables.BoardSquashfsDisable4kAlign
	}
	if len(partitionQualifiedVariables.BoardPartitionSize) == 0 && len(partitionQualifiedVariables.BoardPartitionReservedSize) == 0 && len(partitionQualifiedVariables.ProductHeadroom) == 0 {
		ret[partitionType+"_disable_sparse"] = "true"
	}
	addCommonFlagsToImageProps(variables, partitionType, ret)
}

func hasErofsPartition(variables *android.PartitionVariables) bool {
	return variables.PartitionQualifiedVariables["product"].BoardFileSystemType == "erofs" ||
		variables.PartitionQualifiedVariables["system_ext"].BoardFileSystemType == "erofs" ||
		variables.PartitionQualifiedVariables["odm"].BoardFileSystemType == "erofs" ||
		variables.PartitionQualifiedVariables["vendor"].BoardFileSystemType == "erofs" ||
		variables.PartitionQualifiedVariables["system"].BoardFileSystemType == "erofs" ||
		variables.PartitionQualifiedVariables["vendor_dlkm"].BoardFileSystemType == "erofs" ||
		variables.PartitionQualifiedVariables["odm_dlkm"].BoardFileSystemType == "erofs" ||
		variables.PartitionQualifiedVariables["system_dlkm"].BoardFileSystemType == "erofs"
}

// Soong equivalent of make's add-common-flags-to-image-props
func addCommonFlagsToImageProps(variables *android.PartitionVariables, partitionType string, ret map[string]string) {
	// The selinux_fc will be handled separately
	partitionQualifiedVariables, ok := variables.PartitionQualifiedVariables[partitionType]
	if !ok {
		panic("Unknown partitionType: " + partitionType)
	}
	ret["building_"+partitionType+"_image"] = boolToMakeString(partitionQualifiedVariables.BuildingImage)
}

func boolToMakeString(b bool) string {
	if b {
		return "true"
	}
	return ""
}
