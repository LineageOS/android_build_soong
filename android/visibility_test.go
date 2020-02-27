package android

import (
	"reflect"
	"testing"

	"github.com/google/blueprint"
)

var visibilityTests = []struct {
	name                string
	fs                  map[string][]byte
	expectedErrors      []string
	effectiveVisibility map[qualifiedModuleName][]string
}{
	{
		name: "invalid visibility: empty list",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: [],
				}`),
		},
		expectedErrors: []string{`visibility: must contain at least one visibility rule`},
	},
	{
		name: "invalid visibility: empty rule",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: [""],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern ""`},
	},
	{
		name: "invalid visibility: unqualified",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["target"],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern "target"`},
	},
	{
		name: "invalid visibility: empty namespace",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//"],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern "//"`},
	},
	{
		name: "invalid visibility: empty module",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: [":"],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern ":"`},
	},
	{
		name: "invalid visibility: empty namespace and module",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//:"],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern "//:"`},
	},
	{
		name: "//visibility:unknown",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//visibility:unknown"],
				}`),
		},
		expectedErrors: []string{`unrecognized visibility rule "//visibility:unknown"`},
	},
	{
		name: "//visibility:xxx mixed",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//visibility:public", "//namespace"],
				}

				mock_library {
					name: "libother",
					visibility: ["//visibility:private", "//namespace"],
				}`),
		},
		expectedErrors: []string{
			`module "libother": visibility: cannot mix "//visibility:private"` +
				` with any other visibility rules`,
			`module "libexample": visibility: cannot mix "//visibility:public"` +
				` with any other visibility rules`,
		},
	},
	{
		name: "//visibility:legacy_public",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//visibility:legacy_public"],
				}`),
		},
		expectedErrors: []string{
			`module "libexample": visibility: //visibility:legacy_public must` +
				` not be used`,
		},
	},
	{
		// Verify that //visibility:public will allow the module to be referenced from anywhere, e.g.
		// the current directory, a nested directory and a directory in a separate tree.
		name: "//visibility:public",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//visibility:public"],
				}
	
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
	},
	{
		// Verify that //visibility:private allows the module to be referenced from the current
		// directory only.
		name: "//visibility:private",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//visibility:private"],
				}
	
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libnested" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
			`module "libother" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		// Verify that :__pkg__ allows the module to be referenced from the current directory only.
		name: ":__pkg__",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: [":__pkg__"],
				}
	
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libnested" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
			`module "libother" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		// Verify that //top/nested allows the module to be referenced from the current directory and
		// the top/nested directory only, not a subdirectory of top/nested and not peak directory.
		name: "//top/nested",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//top/nested"],
				}
	
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"top/nested/again/Blueprints": []byte(`
				mock_library {
					name: "libnestedagain",
					deps: ["libexample"],
				}`),
			"peak/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libother" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
			`module "libnestedagain" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		// Verify that :__subpackages__ allows the module to be referenced from the current directory
		// and sub directories but nowhere else.
		name: ":__subpackages__",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: [":__subpackages__"],
				}
	
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"peak/other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libother" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		// Verify that //top/nested:__subpackages__ allows the module to be referenced from the current
		// directory and sub directories but nowhere else.
		name: "//top/nested:__subpackages__",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//top/nested:__subpackages__", "//other"],
				}
	
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"top/other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libother" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		// Verify that ["//top/nested", "//peak:__subpackages"] allows the module to be referenced from
		// the current directory, top/nested and peak and all its subpackages.
		name: `["//top/nested", "//peak:__subpackages__"]`,
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//top/nested", "//peak:__subpackages__"],
				}
	
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"peak/other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
	},
	{
		// Verify that //vendor... cannot be used outside vendor apart from //vendor:__subpackages__
		name: `//vendor`,
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//vendor:__subpackages__"],
				}
	
				mock_library {
					name: "libsamepackage",
					visibility: ["//vendor/apps/AcmeSettings"],
				}`),
			"vendor/Blueprints": []byte(`
				mock_library {
					name: "libvendorexample",
					deps: ["libexample"],
					visibility: ["//vendor/nested"],
				}`),
			"vendor/nested/Blueprints": []byte(`
				mock_library {
					name: "libvendornested",
					deps: ["libexample", "libvendorexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libsamepackage": visibility: "//vendor/apps/AcmeSettings"` +
				` is not allowed. Packages outside //vendor cannot make themselves visible to specific` +
				` targets within //vendor, they can only use //vendor:__subpackages__.`,
		},
	},

	// Defaults propagation tests
	{
		// Check that visibility is the union of the defaults modules.
		name: "defaults union, basic",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//other"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//top/nested"],
					defaults: ["libexample_defaults"],
				}
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "liboutsider" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		name: "defaults union, multiple defaults",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults_1",
					visibility: ["//other"],
				}
				mock_defaults {
					name: "libexample_defaults_2",
					visibility: ["//top/nested"],
				}
				mock_library {
					name: "libexample",
					defaults: ["libexample_defaults_1", "libexample_defaults_2"],
				}
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "liboutsider" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		name: "//visibility:public mixed with other in defaults",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:public", "//namespace"],
				}
				mock_library {
					name: "libexample",
					defaults: ["libexample_defaults"],
				}`),
		},
		expectedErrors: []string{
			`module "libexample_defaults": visibility: cannot mix "//visibility:public"` +
				` with any other visibility rules`,
		},
	},
	{
		name: "//visibility:public overriding defaults",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//namespace"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:public"],
					defaults: ["libexample_defaults"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
		effectiveVisibility: map[qualifiedModuleName][]string{
			qualifiedModuleName{pkg: "top", name: "libexample"}: {"//visibility:public"},
		},
	},
	{
		name: "//visibility:public mixed with other from different defaults 1",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults_1",
					visibility: ["//namespace"],
				}
				mock_defaults {
					name: "libexample_defaults_2",
					visibility: ["//visibility:public"],
				}
				mock_library {
					name: "libexample",
					defaults: ["libexample_defaults_1", "libexample_defaults_2"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:public mixed with other from different defaults 2",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults_1",
					visibility: ["//visibility:public"],
				}
				mock_defaults {
					name: "libexample_defaults_2",
					visibility: ["//namespace"],
				}
				mock_library {
					name: "libexample",
					defaults: ["libexample_defaults_1", "libexample_defaults_2"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:private in defaults",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:private"],
				}
				mock_library {
					name: "libexample",
					defaults: ["libexample_defaults"],
				}
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libnested" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
			`module "libother" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		name: "//visibility:private mixed with other in defaults",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:private", "//namespace"],
				}
				mock_library {
					name: "libexample",
					defaults: ["libexample_defaults"],
				}`),
		},
		expectedErrors: []string{
			`module "libexample_defaults": visibility: cannot mix "//visibility:private"` +
				` with any other visibility rules`,
		},
	},
	{
		name: "//visibility:private overriding defaults",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//namespace"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:private"],
					defaults: ["libexample_defaults"],
				}`),
		},
		expectedErrors: []string{
			`module "libexample": visibility: cannot mix "//visibility:private"` +
				` with any other visibility rules`,
		},
	},
	{
		name: "//visibility:private in defaults overridden",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:private"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//namespace"],
					defaults: ["libexample_defaults"],
				}`),
		},
		expectedErrors: []string{
			`module "libexample": visibility: cannot mix "//visibility:private"` +
				` with any other visibility rules`,
		},
	},
	{
		name: "//visibility:private mixed with itself",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "libexample_defaults_1",
					visibility: ["//visibility:private"],
				}
				mock_defaults {
					name: "libexample_defaults_2",
					visibility: ["//visibility:private"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:private"],
					defaults: ["libexample_defaults_1", "libexample_defaults_2"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "liboutsider" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},

	// Defaults module's defaults_visibility tests
	{
		name: "defaults_visibility invalid",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "top_defaults",
					defaults_visibility: ["//visibility:invalid"],
				}`),
		},
		expectedErrors: []string{
			`defaults_visibility: unrecognized visibility rule "//visibility:invalid"`,
		},
	},
	{
		name: "defaults_visibility overrides package default",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}
				mock_defaults {
					name: "top_defaults",
					defaults_visibility: ["//visibility:public"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					defaults: ["top_defaults"],
				}`),
		},
	},

	// Package default_visibility tests
	{
		name: "package default_visibility property is checked",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_visibility: ["//visibility:invalid"],
				}`),
		},
		expectedErrors: []string{`default_visibility: unrecognized visibility rule "//visibility:invalid"`},
	},
	{
		// This test relies on the default visibility being legacy_public.
		name: "package default_visibility property used when no visibility specified",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}

				mock_library {
					name: "libexample",
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "liboutsider" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		name: "package default_visibility public does not override visibility private",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_visibility: ["//visibility:public"],
				}

				mock_library {
					name: "libexample",
					visibility: ["//visibility:private"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "liboutsider" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		name: "package default_visibility private does not override visibility public",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}

				mock_library {
					name: "libexample",
					visibility: ["//visibility:public"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
	},
	{
		name: "package default_visibility :__subpackages__",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_visibility: [":__subpackages__"],
				}

				mock_library {
					name: "libexample",
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "liboutsider" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		name: "package default_visibility inherited to subpackages",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_visibility: ["//outsider"],
				}

				mock_library {
					name: "libexample",
          visibility: [":__subpackages__"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample", "libnested"],
				}`),
		},
		expectedErrors: []string{
			`module "liboutsider" variant "android_common": depends on //top:libexample which is not` +
				` visible to this module`,
		},
	},
	{
		name: "package default_visibility inherited to subpackages",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}`),
			"top/nested/Blueprints": []byte(`
				package {
					default_visibility: ["//outsider"],
				}

				mock_library {
					name: "libnested",
				}`),
			"top/other/Blueprints": []byte(`
				mock_library {
					name: "libother",
				}`),
			"outsider/Blueprints": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libother", "libnested"],
				}`),
		},
		expectedErrors: []string{
			`module "liboutsider" variant "android_common": depends on //top/other:libother which is` +
				` not visible to this module`,
		},
	},
	{
		name: "verify that prebuilt dependencies are ignored for visibility reasons (not preferred)",
		fs: map[string][]byte{
			"prebuilts/Blueprints": []byte(`
				prebuilt {
					name: "module",
					visibility: ["//top/other"],
				}`),
			"top/sources/source_file": nil,
			"top/sources/Blueprints": []byte(`
				source {
					name: "module",
					visibility: ["//top/other"],
				}`),
			"top/other/source_file": nil,
			"top/other/Blueprints": []byte(`
				source {
					name: "other",
					deps: [":module"],
				}`),
		},
	},
	{
		name: "verify that prebuilt dependencies are ignored for visibility reasons (preferred)",
		fs: map[string][]byte{
			"prebuilts/Blueprints": []byte(`
				prebuilt {
					name: "module",
					visibility: ["//top/other"],
					prefer: true,
				}`),
			"top/sources/source_file": nil,
			"top/sources/Blueprints": []byte(`
				source {
					name: "module",
					visibility: ["//top/other"],
				}`),
			"top/other/source_file": nil,
			"top/other/Blueprints": []byte(`
				source {
					name: "other",
					deps: [":module"],
				}`),
		},
	},
}

func TestVisibility(t *testing.T) {
	for _, test := range visibilityTests {
		t.Run(test.name, func(t *testing.T) {
			ctx, errs := testVisibility(buildDir, test.fs)

			CheckErrorsAgainstExpectations(t, errs, test.expectedErrors)

			if test.effectiveVisibility != nil {
				checkEffectiveVisibility(t, ctx, test.effectiveVisibility)
			}
		})
	}
}

func checkEffectiveVisibility(t *testing.T, ctx *TestContext, effectiveVisibility map[qualifiedModuleName][]string) {
	for moduleName, expectedRules := range effectiveVisibility {
		rule := effectiveVisibilityRules(ctx.config, moduleName)
		stringRules := rule.Strings()
		if !reflect.DeepEqual(expectedRules, stringRules) {
			t.Errorf("effective rules mismatch: expected %q, found %q", expectedRules, stringRules)
		}
	}
}

func testVisibility(buildDir string, fs map[string][]byte) (*TestContext, []error) {

	// Create a new config per test as visibility information is stored in the config.
	config := TestArchConfig(buildDir, nil, "", fs)

	ctx := NewTestArchContext()
	ctx.RegisterModuleType("mock_library", newMockLibraryModule)
	ctx.RegisterModuleType("mock_defaults", defaultsFactory)

	// Order of the following method calls is significant.
	RegisterPackageBuildComponents(ctx)
	registerTestPrebuiltBuildComponents(ctx)
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

type mockLibraryProperties struct {
	Deps []string
}

type mockLibraryModule struct {
	ModuleBase
	DefaultableModuleBase
	properties mockLibraryProperties
}

func newMockLibraryModule() Module {
	m := &mockLibraryModule{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibCommon)
	InitDefaultableModule(m)
	return m
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

func (j *mockLibraryModule) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddVariationDependencies(nil, dependencyTag{name: "mockdeps"}, j.properties.Deps...)
}

func (p *mockLibraryModule) GenerateAndroidBuildActions(ModuleContext) {
}

type mockDefaults struct {
	ModuleBase
	DefaultsModuleBase
}

func defaultsFactory() Module {
	m := &mockDefaults{}
	InitDefaultsModule(m)
	return m
}
