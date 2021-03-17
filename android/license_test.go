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
		name: "public license",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
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
	ctx := NewTestArchContext()
	RegisterPackageBuildComponents(ctx)
	registerTestPrebuiltBuildComponents(ctx)
	RegisterLicenseBuildComponents(ctx)
	ctx.RegisterModuleType("rule", newMockRuleModule)
	ctx.PreArchMutators(RegisterVisibilityRuleChecker)
	ctx.PreArchMutators(RegisterDefaultsPreArchMutators)
	ctx.PreArchMutators(RegisterVisibilityRuleGatherer)
	ctx.PostDepsMutators(RegisterVisibilityRuleEnforcer)
	ctx.Register(config)
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
