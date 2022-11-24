package android

import (
	"testing"
)

// Common test set up for license tests.
var prepareForLicenseTest = GroupFixturePreparers(
	// General preparers in alphabetical order.
	PrepareForTestWithDefaults,
	PrepareForTestWithLicenses,
	PrepareForTestWithOverrides,
	PrepareForTestWithPackageModule,
	PrepareForTestWithPrebuilts,
	PrepareForTestWithVisibility,

	// Additional test specific stuff
	prepareForTestWithFakePrebuiltModules,
	FixtureMergeEnv(map[string]string{"ANDROID_REQUIRE_LICENSES": "1"}),
)

var licenseTests = []struct {
	name           string
	fs             MockFS
	expectedErrors []string
}{
	{
		name: "license must not accept licenses property",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				license {
					name: "top_license",
					visibility: ["//visibility:private"],
					licenses: ["other_license"],
				}`),
		},
		expectedErrors: []string{
			`top/Android.bp:5:14: unrecognized property "licenses"`,
		},
	},
	{
		name: "private license",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
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
			"other/Android.bp": []byte(`
				rule {
					name: "arule",
					licenses: ["top_allowed_as_notice"],
				}`),
			"yetmore/Android.bp": []byte(`
				package {
					default_applicable_licenses: ["top_allowed_as_notice"],
				}`),
		},
		expectedErrors: []string{
			`other/Android.bp:2:5: module "arule": depends on //top:top_allowed_as_notice ` +
				`which is not visible to this module`,
			`yetmore/Android.bp:2:5: module "//yetmore": depends on //top:top_allowed_as_notice ` +
				`which is not visible to this module`,
		},
	},
	{
		name: "must reference license_kind module",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
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
			`top/Android.bp:6:5: module "top_proprietary": license_kinds property ` +
				`"top_by_exception_only" is not a license_kind module`,
		},
	},
	{
		name: "must not duplicate license_kind",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				license_kind {
					name: "top_by_exception_only",
					conditions: ["by_exception_only"],
					visibility: ["//visibility:private"],
				}

				license_kind {
					name: "top_by_exception_only_2",
					conditions: ["by_exception_only"],
					visibility: ["//visibility:private"],
				}

				license {
					name: "top_proprietary",
					license_kinds: [
						"top_by_exception_only",
						"top_by_exception_only_2",
						"top_by_exception_only"
					],
					visibility: ["//visibility:public"],
				}`),
		},
		expectedErrors: []string{
			`top/Android.bp:14:5: module "top_proprietary": Duplicated license kind: "top_by_exception_only"`,
		},
	},
	{
		name: "license_kind module must exist",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				license {
					name: "top_notice_allowed",
					license_kinds: ["top_notice"],
					visibility: ["//visibility:public"],
				}`),
		},
		expectedErrors: []string{
			`top/Android.bp:2:5: "top_notice_allowed" depends on undefined module "top_notice"`,
		},
	},
	{
		name: "public license",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
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
			"other/Android.bp": []byte(`
				rule {
					name: "arule",
					licenses: ["top_proprietary"],
				}`),
			"yetmore/Android.bp": []byte(`
				package {
					default_applicable_licenses: ["top_proprietary"],
				}`),
		},
	},
	{
		name: "multiple licenses",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
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
			"other/Android.bp": []byte(`
				rule {
					name: "arule",
					licenses: ["top_proprietary"],
				}`),
			"yetmore/Android.bp": []byte(`
				package {
					default_applicable_licenses: ["top_proprietary"],
				}`),
		},
	},
}

func TestLicense(t *testing.T) {
	for _, test := range licenseTests {
		t.Run(test.name, func(t *testing.T) {
			// Customize the common license text fixture factory.
			GroupFixturePreparers(
				prepareForLicenseTest,
				FixtureRegisterWithContext(func(ctx RegistrationContext) {
					ctx.RegisterModuleType("rule", newMockRuleModule)
				}),
				test.fs.AddToFixture(),
			).
				ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern(test.expectedErrors)).
				RunTest(t)
		})
	}
}

func testLicense(t *testing.T, fs MockFS, expectedErrors []string) {
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
