package android

import (
	"testing"
)

var packageTests = []struct {
	name           string
	fs             MockFS
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
					licenses: ["license"],
				}`),
		},
		expectedErrors: []string{
			`top/Blueprints:5:14: unrecognized property "licenses"`,
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
			GroupFixturePreparers(
				PrepareForTestWithArchMutator,
				PrepareForTestWithPackageModule,
				test.fs.AddToFixture(),
			).
				ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern(test.expectedErrors)).
				RunTest(t)
		})
	}
}
