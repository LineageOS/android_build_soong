package android

import (
	"testing"
)

var packageTests = []struct {
	name           string
	fs             map[string][]byte
	expectedErrors []string
}{
	// Package default_visibility handling is tested in visibility_test.go
	{
		name: "package must not accept visibility and name properties",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					name: "package",
					visibility: ["//visibility:private"],
				}`),
		},
		expectedErrors: []string{
			`top/Blueprints:3:10: unrecognized property "name"`,
			`top/Blueprints:4:16: unrecognized property "visibility"`,
		},
	},
	{
		name: "multiple packages in separate directories",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
				}`),
			"other/Blueprints": []byte(`
				package {
				}`),
			"other/nested/Blueprints": []byte(`
				package {
				}`),
		},
	},
	{
		name: "package must not be specified more than once per package",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_visibility: ["//visibility:private"],
					default_applicable_licenses: ["license"],
				}

        package {
				}`),
		},
		expectedErrors: []string{
			`module "//top" already defined`,
		},
	},
}

func TestPackage(t *testing.T) {
	for _, test := range packageTests {
		t.Run(test.name, func(t *testing.T) {
			_, errs := testPackage(test.fs)

			expectedErrors := test.expectedErrors
			if expectedErrors == nil {
				FailIfErrored(t, errs)
			} else {
				for _, expectedError := range expectedErrors {
					FailIfNoMatchingErrors(t, expectedError, errs)
				}
				if len(errs) > len(expectedErrors) {
					t.Errorf("additional errors found, expected %d, found %d", len(expectedErrors), len(errs))
					for i, expectedError := range expectedErrors {
						t.Errorf("expectedErrors[%d] = %s", i, expectedError)
					}
					for i, err := range errs {
						t.Errorf("errs[%d] = %s", i, err)
					}
				}
			}
		})
	}
}

func testPackage(fs map[string][]byte) (*TestContext, []error) {

	// Create a new config per test as visibility information is stored in the config.
	config := TestArchConfig(buildDir, nil, "", fs)

	ctx := NewTestArchContext()
	RegisterPackageBuildComponents(ctx)
	ctx.Register(config)

	_, errs := ctx.ParseBlueprintsFiles(".")
	if len(errs) > 0 {
		return ctx, errs
	}

	_, errs = ctx.PrepareBuildActions(config)
	return ctx, errs
}
