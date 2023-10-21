package bp2build

import (
	"android/soong/android"
	"android/soong/cc"
	"android/soong/starlark_import"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/blueprint/proptools"
	"go.starlark.net/starlark"
)

func createStarlarkValue(t *testing.T, code string) starlark.Value {
	t.Helper()
	result, err := starlark.ExecFile(&starlark.Thread{}, "main.bzl", "x = "+code, nil)
	if err != nil {
		t.Error(err)
	}
	return result["x"]
}

func createStarlarkProductVariablesMap(t *testing.T, code string) map[string]starlark.Value {
	t.Helper()
	rawValue := createStarlarkValue(t, code)
	value, err := starlark_import.Unmarshal[map[string]starlark.Value](rawValue)
	if err != nil {
		t.Error(err)
	}
	return value
}

func TestStarlarkMapToProductVariables(t *testing.T) {
	thirty := 30
	cases := []struct {
		starlark string
		result   android.ProductVariables
	}{
		{
			starlark: `{"CompressedApex": True}`,
			result:   android.ProductVariables{CompressedApex: proptools.BoolPtr(true)},
		},
		{
			starlark: `{"ApexGlobalMinSdkVersionOverride": "Tiramisu"}`,
			result:   android.ProductVariables{ApexGlobalMinSdkVersionOverride: proptools.StringPtr("Tiramisu")},
		},
		{
			starlark: `{"ProductManufacturer": "Google"}`,
			result:   android.ProductVariables{ProductManufacturer: "Google"},
		},
		{
			starlark: `{"Unbundled_build_apps": ["app1", "app2"]}`,
			result:   android.ProductVariables{Unbundled_build_apps: []string{"app1", "app2"}},
		},
		{
			starlark: `{"Platform_sdk_version": 30}`,
			result:   android.ProductVariables{Platform_sdk_version: &thirty},
		},
		{
			starlark: `{"HostFakeSnapshotEnabled": True}`,
			result:   android.ProductVariables{HostFakeSnapshotEnabled: true},
		},
	}

	for _, testCase := range cases {
		productVariables, err := starlarkMapToProductVariables(createStarlarkProductVariablesMap(t,
			testCase.starlark))
		if err != nil {
			t.Error(err)
			continue
		}
		testCase.result.Native_coverage = proptools.BoolPtr(false)
		if !reflect.DeepEqual(testCase.result, productVariables) {
			expected, err := json.Marshal(testCase.result)
			if err != nil {
				t.Error(err)
				continue
			}
			actual, err := json.Marshal(productVariables)
			if err != nil {
				t.Error(err)
				continue
			}
			expectedStr := string(expected)
			actualStr := string(actual)
			t.Errorf("expected %q, but got %q", expectedStr, actualStr)
		}
	}
}

func TestSystemPartitionDeps(t *testing.T) {
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("cc_library", cc.LibraryFactory)
	}, Bp2buildTestCase{
		ExtraFixturePreparer: android.GroupFixturePreparers(
			android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				deviceProduct := "aosp_arm64"
				variables.DeviceProduct = &deviceProduct
				partitionVars := &variables.PartitionVarsForBazelMigrationOnlyDoNotUse
				partitionVars.ProductDirectory = "build/make/target/product/"
				partitionVars.ProductPackages = []string{"foo"}
				var systemVars android.PartitionQualifiedVariablesType
				systemVars.BuildingImage = true
				partitionVars.PartitionQualifiedVariables = map[string]android.PartitionQualifiedVariablesType{
					"system": systemVars,
				}
			}),
			android.FixtureModifyConfig(func(config android.Config) {
				// MockBazelContext will pretend everything is mixed-builds allowlisted.
				// The default is noopBazelContext, which does the opposite.
				config.BazelContext = android.MockBazelContext{}
			}),
		),
		Blueprint: `
cc_library {
  name: "foo",
}`,
		ExpectedBazelTargets: []string{`android_product(
    name = "aosp_arm64",
    soong_variables = _soong_variables,
)`, `partition(
    name = "system_image",
    base_staging_dir = "//build/bazel/bazel_sandwich:system_staging_dir",
    base_staging_dir_file_list = "//build/bazel/bazel_sandwich:system_staging_dir_file_list",
    root_dir = "//build/bazel/bazel_sandwich:root_staging_dir",
    selinux_file_contexts = "//build/bazel/bazel_sandwich:selinux_file_contexts",
    image_properties = """
building_system_image=true
erofs_sparse_flag=-s
extfs_sparse_flag=-s
f2fs_sparse_flag=-S
skip_fsck=true
squashfs_sparse_flag=-s
system_disable_sparse=true

""",
    deps = [
        "//:foo",
    ],

    type = "system",
)`, `partition_diff_test(
    name = "system_image_test",
    partition1 = "//build/bazel/bazel_sandwich:make_system_image",
    partition2 = ":system_image",
)`, `run_test_in_build(
    name = "run_system_image_test",
    test = ":system_image_test",
)`},
		Dir:                      "build/make/target/product/aosp_arm64",
		RunBp2buildProductConfig: true,
	})
}
