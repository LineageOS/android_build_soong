package android

import (
	"testing"
)

var licenseTests = []struct {
	name           string
	fs             map[string][]byte
	expectedErrors []string
}{
	{
		name: "license must not accept licenses property",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license {
					name: "top_license",
					visibility: ["//visibility:private"],
					licenses: ["other_license"],
				}`),
		},
		expectedErrors: []string{
			`top/Blueprints:5:14: unrecognized property "licenses"`,
		},
	},
	{
		name: "private license",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license_kind {
					name: "top_notice",
					conditions: ["notice"],
					visibility: ["//visibility:private"],
				}

				license {
					name: "top_allowed_as_notice",
					license_kinds: ["top_notice"],
					visibility: ["//visibility:private"],
				}`),
			"other/Blueprints": []byte(`
				rule {
					name: "arule",
					licenses: ["top_allowed_as_notice"],
				}`),
			"yetmore/Blueprints": []byte(`
				package {
					default_applicable_licenses: ["top_allowed_as_notice"],
				}`),
		},
		expectedErrors: []string{
			`other/Blueprints:2:5: module "arule": depends on //top:top_allowed_as_notice ` +
				`which is not visible to this module`,
			`yetmore/Blueprints:2:5: module "//yetmore": depends on //top:top_allowed_as_notice ` +
				`which is not visible to this module`,
		},
	},
	{
		name: "must reference license_kind module",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				rule {
					name: "top_by_exception_only",
				}

				license {
					name: "top_proprietary",
					license_kinds: ["top_by_exception_only"],
					visibility: ["//visibility:public"],
				}`),
		},
		expectedErrors: []string{
			`top/Blueprints:6:5: module "top_proprietary": license_kinds property ` +
				`"top_by_exception_only" is not a license_kind module`,
		},
	},
	{
		name: "license_kind module must exist",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license {
					name: "top_notice_allowed",
					license_kinds: ["top_notice"],
					visibility: ["//visibility:public"],
				}`),
		},
		expectedErrors: []string{
			`top/Blueprints:2:5: "top_notice_allowed" depends on undefined module "top_notice"`,
		},
	},
	{
		name: "public license",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license_kind {
					name: "top_by_exception_only",
					conditions: ["by_exception_only"],
					visibility: ["//visibility:private"],
				}

				license {
					name: "top_proprietary",
					license_kinds: ["top_by_exception_only"],
					visibility: ["//visibility:public"],
				}`),
			"other/Blueprints": []byte(`
				rule {
					name: "arule",
					licenses: ["top_proprietary"],
				}`),
			"yetmore/Blueprints": []byte(`
				package {
					default_applicable_licenses: ["top_proprietary"],
				}`),
		},
	},
	{
		name: "multiple licenses",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_applicable_licenses: ["top_proprietary"],
				}

				license_kind {
					name: "top_notice",
					conditions: ["notice"],
				}

				license_kind {
					name: "top_by_exception_only",
					conditions: ["by_exception_only"],
					visibility: ["//visibility:public"],
				}

				license {
					name: "top_allowed_as_notice",
					license_kinds: ["top_notice"],
				}

				license {
					name: "top_proprietary",
					license_kinds: ["top_by_exception_only"],
					visibility: ["//visibility:public"],
				}
				rule {
					name: "myrule",
					licenses: ["top_allowed_as_notice", "top_proprietary"]
				}`),
			"other/Blueprints": []byte(`
				rule {
					name: "arule",
					licenses: ["top_proprietary"],
				}`),
			"yetmore/Blueprints": []byte(`
				package {
					default_applicable_licenses: ["top_proprietary"],
				}`),
		},
	},
}

func TestLicense(t *testing.T) {
	for _, test := range licenseTests {
		t.Run(test.name, func(t *testing.T) {
			_, errs := testLicense(test.fs)

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

func testLicense(fs map[string][]byte) (*TestContext, []error) {

	// Create a new config per test as visibility information is stored in the config.
	env := make(map[string]string)
	env["ANDROID_REQUIRE_LICENSES"] = "1"
	config := TestArchConfig(buildDir, env, "", fs)

	ctx := NewTestArchContext(config)
	RegisterPackageBuildComponents(ctx)
	registerTestPrebuiltBuildComponents(ctx)
	RegisterLicenseKindBuildComponents(ctx)
	RegisterLicenseBuildComponents(ctx)
	ctx.RegisterModuleType("rule", newMockRuleModule)
	ctx.PreArchMutators(RegisterVisibilityRuleChecker)
	ctx.PreArchMutators(RegisterLicensesPackageMapper)
	ctx.PreArchMutators(RegisterDefaultsPreArchMutators)
	ctx.PreArchMutators(RegisterLicensesPropertyGatherer)
	ctx.PreArchMutators(RegisterVisibilityRuleGatherer)
	ctx.PostDepsMutators(RegisterVisibilityRuleEnforcer)
	ctx.PostDepsMutators(RegisterLicensesDependencyChecker)
	ctx.Register()

	_, errs := ctx.ParseBlueprintsFiles(".")
	if len(errs) > 0 {
		return ctx, errs
	}

	_, errs = ctx.PrepareBuildActions(config)
	return ctx, errs
}

type mockRuleModule struct {
	ModuleBase
	DefaultableModuleBase
}

func newMockRuleModule() Module {
	m := &mockRuleModule{}
	InitAndroidModule(m)
	InitDefaultableModule(m)
	return m
}

func (p *mockRuleModule) GenerateAndroidBuildActions(ModuleContext) {
}
