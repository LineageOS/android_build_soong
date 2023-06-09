package bp2build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CreateProductConfigFiles(
	ctx *CodegenContext) ([]BazelFile, error) {
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
	bytes, err := os.ReadFile(productVariablesFileName)
	if err != nil {
		return nil, err
	}

	// TODO(b/249685973): the name is product_config_platforms because product_config
	// was already used for other files. Deduplicate them.
	currentProductFolder := fmt.Sprintf("product_config_platforms/products/%s-%s", targetProduct, targetBuildVariant)

	productReplacer := strings.NewReplacer(
		"{PRODUCT}", targetProduct,
		"{VARIANT}", targetBuildVariant,
		"{PRODUCT_FOLDER}", currentProductFolder)

	result := []BazelFile{
		newFile(
			currentProductFolder,
			"soong.variables.bzl",
			`variables = json.decode("""`+strings.ReplaceAll(string(bytes), "\\", "\\\\")+`""")`),
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

	return result, nil
}
