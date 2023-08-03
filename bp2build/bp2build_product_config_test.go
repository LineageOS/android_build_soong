package bp2build

import (
	"android/soong/android"
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
