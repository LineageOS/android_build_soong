package bp2build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

	// TODO(b/249685973): the name is product_config_platforms because product_config
	// was already used for other files. Deduplicate them.
	currentProductFolder := fmt.Sprintf("product_config_platforms/products/%s-%s", targetProduct, targetBuildVariant)

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

	productLabelsToVariables := make(map[string]*android.ProductVariables)
	productLabelsToVariables[productReplacer.Replace("@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}")] = &productVariables
	for product, productVariablesStarlark := range productsForTestingMap {
		productVariables, err := starlarkMapToProductVariables(productVariablesStarlark)
		if err != nil {
			return res, err
		}
		productLabelsToVariables["@//build/bazel/tests/products:"+product] = &productVariables
	}

	res.bp2buildTargets = createTargets(productLabelsToVariables)

	platformMappingContent, err := platformMappingContent(
		productLabelsToVariables,
		ctx.Config().Bp2buildSoongConfigDefinitions,
		metrics.convertedModulePathMap)
	if err != nil {
		return res, err
	}

	res.injectionFiles = []BazelFile{
		newFile(
			currentProductFolder,
			"soong.variables.bzl",
			`variables = json.decode("""`+strings.ReplaceAll(string(productVariablesBytes), "\\", "\\\\")+`""")`),
		newFile(
			currentProductFolder,
			"BUILD",
			productReplacer.Replace(`
package(default_visibility=[
    "@soong_injection//product_config_platforms:__subpackages__",
    "@//build/bazel/product_config:__subpackages__",
])
load(":soong.variables.bzl", _soong_variables = "variables")
load("@//build/bazel/product_config:android_product.bzl", "android_product")

android_product(
    name = "{PRODUCT}-{VARIANT}",
    soong_variables = _soong_variables,
)
`)),
		newFile(
			"product_config_platforms",
			"BUILD.bazel",
			productReplacer.Replace(`
package(default_visibility = [
	"@//build/bazel/product_config:__subpackages__",
	"@soong_injection//product_config_platforms:__subpackages__",
])

load("//{PRODUCT_FOLDER}:soong.variables.bzl", _soong_variables = "variables")
load("@//build/bazel/product_config:android_product.bzl", "android_product")

# Bazel will qualify its outputs by the platform name. When switching between products, this
# means that soong-built files that depend on bazel-built files will suddenly get different
# dependency files, because the path changes, and they will be rebuilt. In order to avoid this
# extra rebuilding, make mixed builds always use a single platform so that the bazel artifacts
# are always under the same path.
android_product(
    name = "mixed_builds_product-{VARIANT}",
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
  "@soong_injection//product_config_platforms:mixed_builds_product-{VARIANT}",
  "@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}",
`)+strings.Join(productsForTesting, "\n")+"\n]\n"),
		newFile(
			"product_config_platforms",
			"common.bazelrc",
			productReplacer.Replace(`
build --platform_mappings=platform_mappings
build --platforms @soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_x86_64

build:android --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}
build:linux_x86 --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_x86
build:linux_x86_64 --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_x86_64
build:linux_bionic_x86_64 --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_bionic_x86_64
build:linux_musl_x86 --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_musl_x86
build:linux_musl_x86_64 --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_musl_x86_64
`)),
		newFile(
			"product_config_platforms",
			"linux.bazelrc",
			productReplacer.Replace(`
build --host_platform @soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_x86_64
`)),
		newFile(
			"product_config_platforms",
			"darwin.bazelrc",
			productReplacer.Replace(`
build --host_platform @soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_darwin_x86_64
`)),
	}
	res.bp2buildFiles = []BazelFile{
		newFile(
			"",
			"platform_mappings",
			platformMappingContent),
	}

	return res, nil
}

func platformMappingContent(
	productLabelToVariables map[string]*android.ProductVariables,
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

	result.WriteString("platforms:\n")
	for productLabel, productVariables := range productLabelToVariables {
		platformMappingSingleProduct(productLabel, productVariables, soongConfigDefinitions, mergedConvertedModulePathMap, &result)
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
	label string,
	productVariables *android.ProductVariables,
	soongConfigDefinitions soongconfig.Bp2BuildSoongConfigDefinitions,
	convertedModulePathMap map[string]string,
	result *strings.Builder) {
	targetBuildVariant := "user"
	if proptools.Bool(productVariables.Eng) {
		targetBuildVariant = "eng"
	} else if proptools.Bool(productVariables.Debuggable) {
		targetBuildVariant = "userdebug"
	}

	platform_sdk_version := -1
	if productVariables.Platform_sdk_version != nil {
		platform_sdk_version = *productVariables.Platform_sdk_version
	}

	defaultAppCertificateFilegroup := "//build/bazel/utils:empty_filegroup"
	if proptools.String(productVariables.DefaultAppCertificate) != "" {
		defaultAppCertificateFilegroup = "@//" + filepath.Dir(proptools.String(productVariables.DefaultAppCertificate)) + ":generated_android_certificate_directory"
	}

	for _, suffix := range bazelPlatformSuffixes {
		result.WriteString("  ")
		result.WriteString(label)
		result.WriteString(suffix)
		result.WriteString("\n")
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
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:debuggable=%t\n", proptools.Bool(productVariables.Debuggable)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:default_app_certificate=%s\n", proptools.String(productVariables.DefaultAppCertificate)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:default_app_certificate_filegroup=%s\n", defaultAppCertificateFilegroup))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_abi=%s\n", strings.Join(productVariables.DeviceAbi, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_max_page_size_supported=%s\n", proptools.String(productVariables.DeviceMaxPageSizeSupported)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_name=%s\n", proptools.String(productVariables.DeviceName)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_page_size_agnostic=%t\n", proptools.Bool(productVariables.DevicePageSizeAgnostic)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_product=%s\n", proptools.String(productVariables.DeviceProduct)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_platform=%s\n", label))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:enable_cfi=%t\n", proptools.BoolDefault(productVariables.EnableCFI, true)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:enforce_vintf_manifest=%t\n", proptools.Bool(productVariables.Enforce_vintf_manifest)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:eng=%t\n", proptools.Bool(productVariables.Eng)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:malloc_not_svelte=%t\n", proptools.Bool(productVariables.Malloc_not_svelte)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:malloc_pattern_fill_contents=%t\n", proptools.Bool(productVariables.Malloc_pattern_fill_contents)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:malloc_zero_contents=%t\n", proptools.Bool(productVariables.Malloc_zero_contents)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:memtag_heap_exclude_paths=%s\n", strings.Join(productVariables.MemtagHeapExcludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:memtag_heap_async_include_paths=%s\n", strings.Join(productVariables.MemtagHeapAsyncIncludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:memtag_heap_sync_include_paths=%s\n", strings.Join(productVariables.MemtagHeapSyncIncludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:manifest_package_name_overrides=%s\n", strings.Join(productVariables.ManifestPackageNameOverrides, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:native_coverage=%t\n", proptools.Bool(productVariables.Native_coverage)))
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
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:target_build_variant=%s\n", targetBuildVariant))
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

		for namespace, namespaceContents := range productVariables.VendorVars {
			for variable, value := range namespaceContents {
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

func createTargets(productLabelsToVariables map[string]*android.ProductVariables) map[string]BazelTargets {
	res := make(map[string]BazelTargets)
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
		content := fmt.Sprintf(ruleTargetTemplate, "filegroup", "generated_android_certificate_directory", propsToAttributes(map[string]string{
			"srcs": `glob([
        "*.pk8",
        "*.pem",
        "*.avbpubkey",
    ])`,
			"visibility": `["//visibility:public"]`,
		}))
		res[dir] = append(res[dir], BazelTarget{
			name:        "generated_android_certificate_directory",
			packageName: dir,
			content:     content,
			ruleClass:   "filegroup",
		})
	}
	return res
}
