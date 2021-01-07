package android

import (
	"testing"

	"github.com/google/blueprint"
)

var licenseKindTests = []struct {
	name           string
	fs             map[string][]byte
	expectedErrors []string
}{
	{
		name: "license_kind must not accept licenses property",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license_kind {
					name: "top_license",
					licenses: ["other_license"],
				}`),
		},
		expectedErrors: []string{
			`top/Blueprints:4:14: unrecognized property "licenses"`,
		},
	},
	{
		name: "bad license_kind",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license_kind {
					name: "top_notice",
					conditions: ["notice"],
				}`),
			"other/Blueprints": []byte(`
				mock_license {
					name: "other_notice",
					license_kinds: ["notice"],
				}`),
		},
		expectedErrors: []string{
			`other/Blueprints:2:5: "other_notice" depends on undefined module "notice"`,
		},
	},
	{
		name: "good license kind",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license_kind {
					name: "top_by_exception_only",
					conditions: ["by_exception_only"],
				}

				mock_license {
					name: "top_proprietary",
					license_kinds: ["top_by_exception_only"],
				}`),
			"other/Blueprints": []byte(`
				mock_license {
					name: "other_proprietary",
					license_kinds: ["top_proprietary"],
				}`),
		},
	},
	{
		name: "multiple license kinds",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license_kind {
					name: "top_notice",
					conditions: ["notice"],
				}

				license_kind {
					name: "top_by_exception_only",
					conditions: ["by_exception_only"],
				}

				mock_license {
					name: "top_allowed_as_notice",
					license_kinds: ["top_notice"],
				}

				mock_license {
					name: "top_proprietary",
					license_kinds: ["top_by_exception_only"],
				}`),
			"other/Blueprints": []byte(`
				mock_license {
					name: "other_rule",
					license_kinds: ["top_by_exception_only"],
				}`),
		},
	},
}

func TestLicenseKind(t *testing.T) {
	for _, test := range licenseKindTests {
		t.Run(test.name, func(t *testing.T) {
			_, errs := testLicenseKind(test.fs)

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

func testLicenseKind(fs map[string][]byte) (*TestContext, []error) {

	// Create a new config per test as license_kind information is stored in the config.
	config := TestArchConfig(buildDir, nil, "", fs)

	ctx := NewTestArchContext(config)
	RegisterLicenseKindBuildComponents(ctx)
	ctx.RegisterModuleType("mock_license", newMockLicenseModule)
	ctx.Register()

	_, errs := ctx.ParseBlueprintsFiles(".")
	if len(errs) > 0 {
		return ctx, errs
	}

	_, errs = ctx.PrepareBuildActions(config)
	return ctx, errs
}

type mockLicenseProperties struct {
	License_kinds []string
}

type mockLicenseModule struct {
	ModuleBase
	DefaultableModuleBase

	properties mockLicenseProperties
}

func newMockLicenseModule() Module {
	m := &mockLicenseModule{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibCommon)
	InitDefaultableModule(m)
	return m
}

type licensekindTag struct {
	blueprint.BaseDependencyTag
}

func (j *mockLicenseModule) DepsMutator(ctx BottomUpMutatorContext) {
	m, ok := ctx.Module().(Module)
	if !ok {
		return
	}
	ctx.AddDependency(m, licensekindTag{}, j.properties.License_kinds...)
}

func (p *mockLicenseModule) GenerateAndroidBuildActions(ModuleContext) {
}
