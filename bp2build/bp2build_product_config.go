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
load("@//build/bazel/product_config:utils.bzl", "android_product")

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

# TODO(b/249685973): Remove this. It was only added for a platform_mappings file,
# which can possibly be replaced with autogenerating the platform_mappings file,
# or removing that file entirely.
alias(
	name = "current_android_platform",
	# TODO: When we start generating the platforms for more than just the
	# currently lunched, product, turn this into a select with an arm for each product.
	actual = "@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}",
)

alias(
	name = "product_vars",
	actual = select({
		# TODO: When we start generating the platforms for more than just the
		# currently lunched, product, this select should have an arm for each product.
		"@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_constraint_value": "@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_product_vars",
	}),
)
`)),
		newFile(
			"product_config_platforms",
			"common.bazelrc",
			productReplacer.Replace(`
build --platforms @soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_x86_64

build:android --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}
build:linux_x86_64 --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_x86_64
build:linux_bionic_x86_64 --platforms=@soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}_linux_bionic_x86_64
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
		newFile(
			"product_config_platforms",
			"platform_mappings",
			productReplacer.Replace(`
flags:
  --cpu=k8
    @soong_injection//{PRODUCT_FOLDER}:{PRODUCT}-{VARIANT}
`)),
	}

	return result, nil
}
