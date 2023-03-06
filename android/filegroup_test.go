package android

import (
	"testing"
)

func TestFilegroupDefaults(t *testing.T) {
	bp := FixtureAddTextFile("p/Android.bp", `
		filegroup_defaults {
			name: "defaults",
			visibility: ["//x"],
		}
		filegroup {
			name: "foo",
			defaults: ["defaults"],
			visibility: ["//y"],
		}
	`)
	result := GroupFixturePreparers(
		PrepareForTestWithFilegroup,
		PrepareForTestWithDefaults,
		PrepareForTestWithVisibility,
		bp).RunTest(t)
	rules := effectiveVisibilityRules(result.Config, qualifiedModuleName{pkg: "p", name: "foo"})
	AssertDeepEquals(t, "visibility", []string{"//x", "//y"}, rules.Strings())
}
