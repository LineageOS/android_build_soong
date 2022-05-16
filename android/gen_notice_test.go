package android

import (
	"testing"

	"github.com/google/blueprint"
)

var genNoticeTests = []struct {
	name           string
	fs             MockFS
	expectedErrors []string
}{
	{
		name: "gen_notice must not accept licenses property",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				gen_notice {
					name: "top_license",
					licenses: ["other_license"],
				}`),
		},
		expectedErrors: []string{
			`unrecognized property "licenses"`,
		},
	},
	{
		name: "bad gen_notice",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				gen_notice {
					name: "top_notice",
					for: ["top_rule"],
				}`),
			"other/Android.bp": []byte(`
				mock_genrule {
					name: "other_rule",
					dep: ["top_notice"],
				}`),
		},
		expectedErrors: []string{
			`module "top_notice": for: no "top_rule" module exists`,
		},
	},
	{
		name: "doubly bad gen_notice",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				gen_notice {
					name: "top_notice",
					for: ["top_rule", "other_rule"],
				}`),
		},
		expectedErrors: []string{
			`module "top_notice": for: modules "top_rule", "other_rule" do not exist`,
		},
	},
	{
		name: "good gen_notice",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				gen_notice {
					name: "top_notice",
					for: ["top_rule"],
				}

				mock_genrule {
					name: "top_rule",
					dep: ["top_notice"],
				}`),
			"other/Android.bp": []byte(`
				mock_genrule {
					name: "other_rule",
					dep: ["top_notice"],
				}`),
		},
	},
	{
		name: "multiple license kinds",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				gen_notice {
					name: "top_notice",
					for: ["top_rule"],
				}

				gen_notice {
					name: "top_html_notice",
					html: true,
					for: ["top_rule"],
				}

				gen_notice {
					name: "top_xml_notice",
					xml: true,
					for: ["top_notice"],
				}

				mock_genrule {
					name: "top_rule",
					dep: [
						"top_notice",
						"top_html_notice",
						"top_xml_notice",
					],
				}`),
			"other/Android.bp": []byte(`
				mock_genrule {
					name: "other_rule",
					dep: ["top_xml_notice"],
				}`),
		},
	},
}

func TestGenNotice(t *testing.T) {
	for _, test := range genNoticeTests {
		t.Run(test.name, func(t *testing.T) {
			GroupFixturePreparers(
				PrepareForTestWithGenNotice,
				FixtureRegisterWithContext(func(ctx RegistrationContext) {
					ctx.RegisterModuleType("mock_genrule", newMockGenruleModule)
				}),
				test.fs.AddToFixture(),
			).
				ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern(test.expectedErrors)).
				RunTest(t)
		})
	}
}

type mockGenruleProperties struct {
	Dep []string
}

type mockGenruleModule struct {
	ModuleBase
	DefaultableModuleBase

	properties mockGenruleProperties
}

func newMockGenruleModule() Module {
	m := &mockGenruleModule{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibCommon)
	InitDefaultableModule(m)
	return m
}

type genruleDepTag struct {
	blueprint.BaseDependencyTag
}

func (j *mockGenruleModule) DepsMutator(ctx BottomUpMutatorContext) {
	m, ok := ctx.Module().(Module)
	if !ok {
		return
	}
	ctx.AddDependency(m, genruleDepTag{}, j.properties.Dep...)
}

func (p *mockGenruleModule) GenerateAndroidBuildActions(ModuleContext) {
}
