package bp2build

import (
	"android/soong/android"
	"android/soong/starlark_import"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/google/blueprint/proptools"
	"go.starlark.net/starlark"
)

func CreateProductConfigFiles(
	ctx *CodegenContext) ([]BazelFile, []BazelFile, error) {
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

	productVariablesFileName := cfg.ProductVariablesFileName
	if !strings.HasPrefix(productVariablesFileName, "/") {
		productVariablesFileName = filepath.Join(ctx.topDir, productVariablesFileName)
	}
	productVariablesBytes, err := os.ReadFile(productVariablesFileName)
	if err != nil {
		return nil, nil, err
	}
	productVariables := android.ProductVariables{}
	err = json.Unmarshal(productVariablesBytes, &productVariables)
	if err != nil {
		return nil, nil, err
	}

	// TODO(b/249685973): the name is product_config_platforms because product_config
	// was already used for other files. Deduplicate them.
	currentProductFolder := fmt.Sprintf("product_config_platforms/products/%s-%s", targetProduct, targetBuildVariant)

	productReplacer := strings.NewReplacer(
		"{PRODUCT}", targetProduct,
		"{VARIANT}", targetBuildVariant,
		"{PRODUCT_FOLDER}", currentProductFolder)

	platformMappingContent, err := platformMappingContent(productReplacer.Replace("@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}"), &productVariables)
	if err != nil {
		return nil, nil, err
	}

	injectionDirFiles := []BazelFile{
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
  "@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}"
]
`)),
		newFile(
			"product_config_platforms",
			"common.bazelrc",
			productReplacer.Replace(`
build --platform_mappings=platform_mappings
build --platforms @soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_x86_64

build:android --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}
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
	bp2buildDirFiles := []BazelFile{
		newFile(
			"",
			"platform_mappings",
			platformMappingContent),
	}
	return injectionDirFiles, bp2buildDirFiles, nil
}

func platformMappingContent(mainProductLabel string, mainProductVariables *android.ProductVariables) (string, error) {
	productsForTesting, err := starlark_import.GetStarlarkValue[map[string]map[string]starlark.Value]("products_for_testing")
	if err != nil {
		return "", err
	}
	var result strings.Builder
	result.WriteString("platforms:\n")
	platformMappingSingleProduct(mainProductLabel, mainProductVariables, &result)
	for product, productVariablesStarlark := range productsForTesting {
		productVariables, err := starlarkMapToProductVariables(productVariablesStarlark)
		if err != nil {
			return "", err
		}
		platformMappingSingleProduct("@//build/bazel/tests/products:"+product, &productVariables, &result)
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

func platformMappingSingleProduct(label string, productVariables *android.ProductVariables, result *strings.Builder) {
	targetBuildVariant := "user"
	if proptools.Bool(productVariables.Eng) {
		targetBuildVariant = "eng"
	} else if proptools.Bool(productVariables.Debuggable) {
		targetBuildVariant = "userdebug"
	}

	for _, suffix := range bazelPlatformSuffixes {
		result.WriteString("  ")
		result.WriteString(label)
		result.WriteString(suffix)
		result.WriteString("\n")
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:always_use_prebuilt_sdks=%t\n", proptools.Bool(productVariables.Always_use_prebuilt_sdks)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:apex_global_min_sdk_version_override=%s\n", proptools.String(productVariables.ApexGlobalMinSdkVersionOverride)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:build_id=%s\n", proptools.String(productVariables.BuildId)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:build_version_tags=%s\n", strings.Join(productVariables.BuildVersionTags, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:certificate_overrides=%s\n", strings.Join(productVariables.CertificateOverrides, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:cfi_exclude_paths=%s\n", strings.Join(productVariables.CFIExcludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:cfi_include_paths=%s\n", strings.Join(productVariables.CFIIncludePaths, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:compressed_apex=%t\n", proptools.Bool(productVariables.CompressedApex)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:default_app_certificate=%s\n", proptools.String(productVariables.DefaultAppCertificate)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_abi=%s\n", strings.Join(productVariables.DeviceAbi, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_max_page_size_supported=%s\n", proptools.String(productVariables.DeviceMaxPageSizeSupported)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_name=%s\n", proptools.String(productVariables.DeviceName)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:device_product=%s\n", proptools.String(productVariables.DeviceProduct)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:enable_cfi=%t\n", proptools.BoolDefault(productVariables.EnableCFI, true)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:manifest_package_name_overrides=%s\n", strings.Join(productVariables.ManifestPackageNameOverrides, ",")))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:platform_version_name=%s\n", proptools.String(productVariables.Platform_version_name)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:product_brand=%s\n", productVariables.ProductBrand))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:product_manufacturer=%s\n", productVariables.ProductManufacturer))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:target_build_variant=%s\n", targetBuildVariant))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:tidy_checks=%s\n", proptools.String(productVariables.TidyChecks)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:unbundled_build=%t\n", proptools.Bool(productVariables.Unbundled_build)))
		result.WriteString(fmt.Sprintf("    --//build/bazel/product_config:unbundled_build_apps=%s\n", strings.Join(productVariables.Unbundled_build_apps, ",")))
	}
}

func starlarkMapToProductVariables(in map[string]starlark.Value) (android.ProductVariables, error) {
	result := android.ProductVariables{}
	productVarsReflect := reflect.ValueOf(&result).Elem()
	for i := 0; i < productVarsReflect.NumField(); i++ {
		field := productVarsReflect.Field(i)
		fieldType := productVarsReflect.Type().Field(i)
		name := fieldType.Name
		if name == "BootJars" || name == "ApexBootJars" || name == "VendorVars" ||
			name == "VendorSnapshotModules" || name == "RecoverySnapshotModules" {
			// These variables have more complicated types, and we don't need them right now
			continue
		}
		if _, ok := in[name]; ok {
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

	return result, nil
}
