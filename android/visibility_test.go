package android

import (
	"reflect"
	"testing"

	"github.com/google/blueprint"
)

var visibilityTests = []struct {
	name                string
	fs                  MockFS
	expectedErrors      []string
	effectiveVisibility map[qualifiedModuleName][]string
}{
	{
		name: "invalid visibility: empty list",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: [],
				}`),
		},
		expectedErrors: []string{`visibility: must contain at least one visibility rule`},
	},
	{
		name: "invalid visibility: empty rule",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: [""],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern ""`},
	},
	{
		name: "invalid visibility: unqualified",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["target"],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern "target"`},
	},
	{
		name: "invalid visibility: empty namespace",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//"],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern "//"`},
	},
	{
		name: "invalid visibility: empty module",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: [":"],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern ":"`},
	},
	{
		name: "invalid visibility: empty namespace and module",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//:"],
				}`),
		},
		expectedErrors: []string{`visibility: invalid visibility pattern "//:"`},
	},
	{
		name: "//visibility:unknown",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//visibility:unknown"],
				}`),
		},
		expectedErrors: []string{`unrecognized visibility rule "//visibility:unknown"`},
	},
	{
		name: "//visibility:xxx mixed",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
		fs: MockFS{
			"top/Android.bp": []byte(`
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
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//visibility:public"],
				}

				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}

				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}

				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
		},
	},
	{
		// Verify that //visibility:private allows the module to be referenced from the current
		// directory only.
		name: "//visibility:private",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//visibility:private"],
				}

				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
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
		// Verify that //visibility:private allows the module to be referenced from the current
		// directory only.
		name: "//visibility:private (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//visibility:private"],
				}

				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "nested-notice" references "//top:libexample" which is not visible to this` +
				` module\nYou may need to add "//top/nested" to its visibility`,
			`module "other-notice" references "//top:libexample" which is not visible to this module\n` +
				`You may need to add "//other" to its visibility`,
		},
	},
	{
		// Verify that :__pkg__ allows the module to be referenced from the current directory only.
		name: ":__pkg__",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: [":__pkg__"],
				}

				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
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
		name: ":__pkg__ (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: [":__pkg__"],
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "nested-notice" references "//top:libexample" which is not visible to this module`,
			`module "other-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		// Verify that //top/nested allows the module to be referenced from the current directory and
		// the top/nested directory only, not a subdirectory of top/nested and not peak directory.
		name: "//top/nested",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//top/nested"],
				}

				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"top/nested/again/Android.bp": []byte(`
				mock_library {
					name: "libnestedagain",
					deps: ["libexample"],
				}`),
			"peak/Android.bp": []byte(`
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
		// Verify that //top/nested allows the module to be referenced from the current directory and
		// the top/nested directory only, not a subdirectory of top/nested and not peak directory.
		name: "//top/nested (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//top/nested"],
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"top/nested/again/Android.bp": []byte(`
				gen_notice {
					name: "nestedagain-notice",
					for: ["libexample"],
				}`),
			"peak/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "other-notice" references "//top:libexample" which is not visible to this module`,
			`module "nestedagain-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		// Verify that :__subpackages__ allows the module to be referenced from the current directory
		// and sub directories but nowhere else.
		name: ":__subpackages__",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: [":__subpackages__"],
				}

				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"peak/other/Android.bp": []byte(`
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
		// Verify that :__subpackages__ allows the module to be referenced from the current directory
		// and sub directories but nowhere else.
		name: ":__subpackages__ (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: [":__subpackages__"],
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"peak/other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "other-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		// Verify that //top/nested:__subpackages__ allows the module to be referenced from the current
		// directory and sub directories but nowhere else.
		name: "//top/nested:__subpackages__",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//top/nested:__subpackages__", "//other"],
				}

				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"top/other/Android.bp": []byte(`
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
		name: "//top/nested:__subpackages__ (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//top/nested:__subpackages__", "//other"],
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"top/other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "other-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		// Verify that ["//top/nested", "//peak:__subpackages"] allows the module to be referenced from
		// the current directory, top/nested and peak and all its subpackages.
		name: `["//top/nested", "//peak:__subpackages__"]`,
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//top/nested", "//peak:__subpackages__"],
				}

				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"peak/other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
	},
	{
		// Verify that ["//top/nested", "//peak:__subpackages"] allows the module to be referenced from
		// the current directory, top/nested and peak and all its subpackages.
		name: `["//top/nested", "//peak:__subpackages__ (notices)"]`,
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//top/nested", "//peak:__subpackages__"],
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"peak/other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
		},
	},
	{
		// Verify that //vendor... cannot be used outside vendor apart from //vendor:__subpackages__
		name: `//vendor`,
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//vendor:__subpackages__"],
				}

				mock_library {
					name: "libsamepackage",
					visibility: ["//vendor/apps/AcmeSettings"],
				}`),
			"vendor/Android.bp": []byte(`
				mock_library {
					name: "libvendorexample",
					deps: ["libexample"],
					visibility: ["//vendor/nested"],
				}`),
			"vendor/nested/Android.bp": []byte(`
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
		fs: MockFS{
			"top/Android.bp": []byte(`
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
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
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
		// Check that visibility is the union of the defaults modules.
		name: "defaults union, basic (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//other"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//top/nested"],
					defaults: ["libexample_defaults"],
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "outsider-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		name: "defaults union, multiple defaults",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
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
		name: "defaults union, multiple defaults (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
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

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "outsider-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		name: "//visibility:public mixed with other in defaults",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//namespace"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:public"],
					defaults: ["libexample_defaults"],
				}`),
			"outsider/Android.bp": []byte(`
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
		name: "//visibility:public overriding defaults (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//namespace"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:public"],
					defaults: ["libexample_defaults"],
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
		effectiveVisibility: map[qualifiedModuleName][]string{
			qualifiedModuleName{pkg: "top", name: "libexample"}: {"//visibility:public"},
		},
	},
	{
		name: "//visibility:public mixed with other from different defaults 1",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
			"outsider/Android.bp": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:public mixed with other from different defaults 1",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:public mixed with other from different defaults 2",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
			"outsider/Android.bp": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:public mixed with other from different defaults 2 (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:private in defaults",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
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
		name: "//visibility:private in defaults (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:private"],
				}
				mock_library {
					name: "libexample",
					defaults: ["libexample_defaults"],
				}

				gen_notice {
					name: "libexample-notice",
					for: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "nested-notice" references "//top:libexample" which is not visible to this module`,
			`module "other-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		name: "//visibility:private mixed with other in defaults",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
		fs: MockFS{
			"top/Android.bp": []byte(`
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
		fs: MockFS{
			"top/Android.bp": []byte(`
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
		name: "//visibility:private override //visibility:public",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:public"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:private"],
					defaults: ["libexample_defaults"],
				}`),
		},
		expectedErrors: []string{
			`module "libexample": visibility: cannot mix "//visibility:private" with any other visibility rules`,
		},
	},
	{
		name: "//visibility:public override //visibility:private",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:private"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:public"],
					defaults: ["libexample_defaults"],
				}`),
		},
		expectedErrors: []string{
			`module "libexample": visibility: cannot mix "//visibility:private" with any other visibility rules`,
		},
	},
	{
		name: "//visibility:override must be first in the list",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					visibility: ["//other", "//visibility:override", "//namespace"],
				}`),
		},
		expectedErrors: []string{
			`module "libexample": visibility: "//visibility:override" may only be used at the start of the visibility rules`,
		},
	},
	{
		name: "//visibility:override discards //visibility:private",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:private"],
				}
				mock_library {
					name: "libexample",
					// Make this visibility to //other but not //visibility:private
					visibility: ["//visibility:override", "//other"],
					defaults: ["libexample_defaults"],
				}`),
			"other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:override discards //visibility:private (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:private"],
				}
				mock_library {
					name: "libexample",
					// Make this visibility to //other but not //visibility:private
					visibility: ["//visibility:override", "//other"],
					defaults: ["libexample_defaults"],
				}`),
			"other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:override discards //visibility:public",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:public"],
				}
				mock_library {
					name: "libexample",
					// Make this visibility to //other but not //visibility:public
					visibility: ["//visibility:override", "//other"],
					defaults: ["libexample_defaults"],
				}`),
			"other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
			"namespace/Android.bp": []byte(`
				mock_library {
					name: "libnamespace",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libnamespace" variant "android_common": depends on //top:libexample which is not visible to this module\nYou may need to add "//namespace" to its visibility`,
		},
	},
	{
		name: "//visibility:override discards //visibility:public (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:public"],
				}
				mock_library {
					name: "libexample",
					// Make this visibility to //other but not //visibility:public
					visibility: ["//visibility:override", "//other"],
					defaults: ["libexample_defaults"],
				}`),
			"other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
			"namespace/Android.bp": []byte(`
				gen_notice {
					name: "namespace-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "namespace-notice" references "//top:libexample" which is not visible to this module\nYou may need to add "//namespace" to its visibility`,
		},
	},
	{
		name: "//visibility:override discards defaults supplied rules",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//namespace"],
				}
				mock_library {
					name: "libexample",
					// Make this visibility to //other but not //namespace
					visibility: ["//visibility:override", "//other"],
					defaults: ["libexample_defaults"],
				}`),
			"other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
			"namespace/Android.bp": []byte(`
				mock_library {
					name: "libnamespace",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libnamespace" variant "android_common": depends on //top:libexample which is not visible to this module\nYou may need to add "//namespace" to its visibility`,
		},
	},
	{
		name: "//visibility:override discards defaults supplied rules (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//namespace"],
				}
				mock_library {
					name: "libexample",
					// Make this visibility to //other but not //namespace
					visibility: ["//visibility:override", "//other"],
					defaults: ["libexample_defaults"],
				}`),
			"other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libexample"],
				}`),
			"namespace/Android.bp": []byte(`
				gen_notice {
					name: "namespace-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "namespace-notice" references "//top:libexample" which is not visible to this module\nYou may need to add "//namespace" to its visibility`,
		},
	},
	{
		name: "//visibility:override can override //visibility:public with //visibility:private",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:public"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:override", "//visibility:private"],
					defaults: ["libexample_defaults"],
				}`),
			"namespace/Android.bp": []byte(`
				mock_library {
					name: "libnamespace",
					deps: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "libnamespace" variant "android_common": depends on //top:libexample which is not visible to this module`,
		},
	},
	{
		name: "//visibility:override can override //visibility:public with //visibility:private (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:public"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:override", "//visibility:private"],
					defaults: ["libexample_defaults"],
				}`),
			"namespace/Android.bp": []byte(`
				gen_notice {
					name: "namespace-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "namespace-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		name: "//visibility:override can override //visibility:private with //visibility:public",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:private"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:override", "//visibility:public"],
					defaults: ["libexample_defaults"],
				}`),
			"namespace/Android.bp": []byte(`
				mock_library {
					name: "libnamespace",
					deps: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:override can override //visibility:private with //visibility:public (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "libexample_defaults",
					visibility: ["//visibility:private"],
				}
				mock_library {
					name: "libexample",
					visibility: ["//visibility:override", "//visibility:public"],
					defaults: ["libexample_defaults"],
				}`),
			"namespace/Android.bp": []byte(`
				gen_notice {
					name: "namespace-notice",
					for: ["libexample"],
				}`),
		},
	},
	{
		name: "//visibility:private mixed with itself",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
			"outsider/Android.bp": []byte(`
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
		name: "//visibility:private mixed with itself (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "outsider-notice" references "//top:libexample" which is not visible to this module`,
		},
	},

	// Defaults module's defaults_visibility tests
	{
		name: "defaults_visibility invalid",
		fs: MockFS{
			"top/Android.bp": []byte(`
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
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}
				mock_defaults {
					name: "top_defaults",
					defaults_visibility: ["//visibility:public"],
				}`),
			"outsider/Android.bp": []byte(`
				mock_library {
					name: "liboutsider",
					defaults: ["top_defaults"],
				}`),
		},
	},

	// Package default_visibility tests
	{
		name: "package default_visibility property is checked",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:invalid"],
				}`),
		},
		expectedErrors: []string{`default_visibility: unrecognized visibility rule "//visibility:invalid"`},
	},
	{
		// This test relies on the default visibility being legacy_public.
		name: "package default_visibility property used when no visibility specified",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}

				mock_library {
					name: "libexample",
				}`),
			"outsider/Android.bp": []byte(`
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
		// This test relies on the default visibility being legacy_public.
		name: "package default_visibility property used when no visibility specified (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}

				mock_library {
					name: "libexample",
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "outsider-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		name: "package default_visibility public does not override visibility private",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:public"],
				}

				mock_library {
					name: "libexample",
					visibility: ["//visibility:private"],
				}`),
			"outsider/Android.bp": []byte(`
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
		name: "package default_visibility public does not override visibility private (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:public"],
				}

				mock_library {
					name: "libexample",
					visibility: ["//visibility:private"],
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "outsider-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		name: "package default_visibility private does not override visibility public",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}

				mock_library {
					name: "libexample",
					visibility: ["//visibility:public"],
				}`),
			"outsider/Android.bp": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
	},
	{
		name: "package default_visibility private does not override visibility public (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}

				mock_library {
					name: "libexample",
					visibility: ["//visibility:public"],
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
	},
	{
		name: "package default_visibility :__subpackages__",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: [":__subpackages__"],
				}

				mock_library {
					name: "libexample",
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
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
		name: "package default_visibility :__subpackages__ (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: [":__subpackages__"],
				}

				mock_library {
					name: "libexample",
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample"],
				}`),
		},
		expectedErrors: []string{
			`module "outsider-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		name: "package default_visibility inherited to subpackages",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//outsider"],
				}

				mock_library {
					name: "libexample",
					visibility: [":__subpackages__"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
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
		name: "package default_visibility inherited to subpackages (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//outsider"],
				}

				mock_library {
					name: "libexample",
					visibility: [":__subpackages__"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}

				gen_notice {
					name: "nested-notice",
					for: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libexample", "libnested"],
				}`),
		},
		expectedErrors: []string{
			`module "outsider-notice" references "//top:libexample" which is not visible to this module`,
		},
	},
	{
		name: "package default_visibility inherited to subpackages",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}`),
			"top/nested/Android.bp": []byte(`
				package {
					default_visibility: ["//outsider"],
				}

				mock_library {
					name: "libnested",
				}`),
			"top/other/Android.bp": []byte(`
				mock_library {
					name: "libother",
				}`),
			"outsider/Android.bp": []byte(`
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
		name: "package default_visibility inherited to subpackages (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				package {
					default_visibility: ["//visibility:private"],
				}`),
			"top/nested/Android.bp": []byte(`
				package {
					default_visibility: ["//outsider"],
				}

				mock_library {
					name: "libnested",
				}`),
			"top/other/Android.bp": []byte(`
				mock_library {
					name: "libother",
				}

				gen_notice {
					name: "other-notice",
					for: ["libother"],
				}`),
			"outsider/Android.bp": []byte(`
				gen_notice {
					name: "outsider-notice",
					for: ["libother", "libnested"],
				}`),
		},
		expectedErrors: []string{
			`module "outsider-notice" references "//top/other:libother" which is not visible to this` +
				` module\nYou may need to add "//outsider" to its visibility`,
		},
	},
	{
		name: "verify that prebuilt dependencies are ignored for visibility reasons (not preferred)",
		fs: MockFS{
			"prebuilts/Android.bp": []byte(`
				prebuilt {
					name: "module",
					visibility: ["//top/other"],
				}`),
			"top/sources/source_file": nil,
			"top/sources/Android.bp": []byte(`
				source {
					name: "module",
					visibility: ["//top/other"],
				}`),
			"top/other/source_file": nil,
			"top/other/Android.bp": []byte(`
				source {
					name: "other",
					deps: [":module"],
				}`),
		},
	},
	{
		name: "verify that prebuilt dependencies are ignored for visibility reasons (not preferred) (notices)",
		fs: MockFS{
			"prebuilts/Android.bp": []byte(`
				prebuilt {
					name: "module",
					visibility: ["//top/other"],
				}`),
			"top/sources/source_file": nil,
			"top/sources/Android.bp": []byte(`
				source {
					name: "module",
					visibility: ["//top/other"],
				}`),
			"top/other/source_file": nil,
			"top/other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["module"],
				}`),
		},
	},
	{
		name: "verify that prebuilt dependencies are ignored for visibility reasons (preferred)",
		fs: MockFS{
			"prebuilts/Android.bp": []byte(`
				prebuilt {
					name: "module",
					visibility: ["//top/other"],
					prefer: true,
				}`),
			"top/sources/source_file": nil,
			"top/sources/Android.bp": []byte(`
				source {
					name: "module",
					visibility: ["//top/other"],
				}`),
			"top/other/source_file": nil,
			"top/other/Android.bp": []byte(`
				source {
					name: "other",
					deps: [":module"],
				}`),
		},
	},
	{
		name: "verify that prebuilt dependencies are ignored for visibility reasons (preferred) (notices)",
		fs: MockFS{
			"prebuilts/Android.bp": []byte(`
				prebuilt {
					name: "module",
					visibility: ["//top/other"],
					prefer: true,
				}`),
			"top/sources/source_file": nil,
			"top/sources/Android.bp": []byte(`
				source {
					name: "module",
					visibility: ["//top/other"],
				}`),
			"top/other/source_file": nil,
			"top/other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["module"],
				}`),
		},
	},
	{
		name: "ensure visibility properties are checked for correctness",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_parent {
					name: "parent",
					visibility: ["//top/nested"],
					child: {
						name: "libchild",
						visibility: ["top/other"],
					},
				}`),
		},
		expectedErrors: []string{
			`module "parent": child.visibility: invalid visibility pattern "top/other"`,
		},
	},
	{
		name: "invalid visibility added to child detected during gather phase",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_parent {
					name: "parent",
					visibility: ["//top/nested"],
					child: {
						name: "libchild",
						invalid_visibility: ["top/other"],
					},
				}`),
		},
		expectedErrors: []string{
			// That this error is reported against the child not the parent shows it was
			// not being detected in the parent which is correct as invalid_visibility is
			// purposely not added to the list of visibility properties to check, and was
			// in fact detected in the child in the gather phase. Contrast this error message
			// with the preceding one.
			`module "libchild" \(created by module "parent"\): visibility: invalid visibility pattern "top/other"`,
		},
	},
	{
		name: "automatic visibility inheritance enabled",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_parent {
					name: "parent",
					visibility: ["//top/nested"],
					child: {
						name: "libchild",
						visibility: ["//top/other"],
					},
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					deps: ["libchild"],
				}`),
			"top/other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					deps: ["libchild"],
				}`),
		},
	},
	{
		name: "automatic visibility inheritance enabled (notices)",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_parent {
					name: "parent",
					visibility: ["//top/nested"],
					child: {
						name: "libchild",
						visibility: ["//top/other"],
					},
				}`),
			"top/nested/Android.bp": []byte(`
				gen_notice {
					name: "nested-notice",
					for: ["libchild"],
				}`),
			"top/other/Android.bp": []byte(`
				gen_notice {
					name: "other-notice",
					for: ["libchild"],
				}`),
		},
	},
	{
		name: "any_partition visibility works",
		fs: MockFS{
			"top/Android.bp": []byte(`
				android_filesystem {
					name: "foo",
					deps: ["bar"],
				}`),
			"top/nested/Android.bp": []byte(`
				package(default_visibility=["//visibility:private"])
				mock_library {
					name: "bar",
					visibility: ["//visibility:any_partition"],
				}`),
		},
	},
	{
		name: "any_partition visibility doesn't work for non-partitions",
		fs: MockFS{
			"top/Android.bp": []byte(`
				mock_library {
					name: "foo",
					deps: ["bar"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "bar",
					visibility: ["//visibility:any_partition"],
				}`),
		},
		expectedErrors: []string{`module "foo" variant "android_common": depends on //top/nested:bar which is not visible to this module`},
	},
}

func TestVisibility(t *testing.T) {
	for _, test := range visibilityTests {
		t.Run(test.name, func(t *testing.T) {
			result := GroupFixturePreparers(
				// General preparers in alphabetical order as test infrastructure will enforce correct
				// registration order.
				PrepareForTestWithArchMutator,
				PrepareForTestWithDefaults,
				PrepareForTestWithGenNotice,
				PrepareForTestWithOverrides,
				PrepareForTestWithPackageModule,
				PrepareForTestWithPrebuilts,
				PrepareForTestWithVisibility,

				// Additional test specific preparers.
				FixtureRegisterWithContext(func(ctx RegistrationContext) {
					ctx.RegisterModuleType("mock_library", newMockLibraryModule)
					ctx.RegisterModuleType("mock_parent", newMockParentFactory)
					ctx.RegisterModuleType("mock_defaults", defaultsFactory)
					// For testing //visibility:any_partition. The module type doesn't matter, just that it's registered under the name "android_filesystem"
					ctx.RegisterModuleType("android_filesystem", newMockLibraryModule)
				}),
				prepareForTestWithFakePrebuiltModules,
				// Add additional files to the mock filesystem
				test.fs.AddToFixture(),
			).
				ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern(test.expectedErrors)).
				RunTest(t)

			if test.effectiveVisibility != nil {
				checkEffectiveVisibility(t, result, test.effectiveVisibility)
			}
		})
	}
}

func checkEffectiveVisibility(t *testing.T, result *TestResult, effectiveVisibility map[qualifiedModuleName][]string) {
	for moduleName, expectedRules := range effectiveVisibility {
		rule := effectiveVisibilityRules(result.Config, moduleName)
		stringRules := rule.Strings()
		AssertDeepEquals(t, "effective rules mismatch", expectedRules, stringRules)
	}
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

type mockParentProperties struct {
	Child struct {
		Name *string

		// Visibility to pass to the child module.
		Visibility []string

		// Purposely not validated visibility to pass to the child.
		Invalid_visibility []string
	}
}

type mockParent struct {
	ModuleBase
	DefaultableModuleBase
	properties mockParentProperties
}

func (p *mockParent) GenerateAndroidBuildActions(ModuleContext) {
}

func newMockParentFactory() Module {
	m := &mockParent{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibCommon)
	InitDefaultableModule(m)
	AddVisibilityProperty(m, "child.visibility", &m.properties.Child.Visibility)

	m.SetDefaultableHook(func(ctx DefaultableHookContext) {
		visibility := m.properties.Child.Visibility
		visibility = append(visibility, m.properties.Child.Invalid_visibility...)
		ctx.CreateModule(newMockLibraryModule, &struct {
			Name       *string
			Visibility []string
		}{m.properties.Child.Name, visibility})
	})
	return m
}

func testVisibilityRuleSet(t *testing.T, rules, extra, expected []string) {
	t.Helper()
	set := &visibilityRuleSet{rules}
	err := set.Widen(extra)
	if err != nil {
		t.Error(err)
		return
	}
	actual := set.Strings()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("mismatching rules after extend: expected %#v, actual %#v", expected, actual)
	}
}

func TestVisibilityRuleSet(t *testing.T) {
	t.Run("extend empty", func(t *testing.T) {
		testVisibilityRuleSet(t, nil, []string{"//foo"}, []string{"//foo"})
	})
	t.Run("extend", func(t *testing.T) {
		testVisibilityRuleSet(t, []string{"//foo"}, []string{"//bar"}, []string{"//bar", "//foo"})
	})
	t.Run("extend duplicate", func(t *testing.T) {
		testVisibilityRuleSet(t, []string{"//foo"}, []string{"//bar", "//foo"}, []string{"//bar", "//foo"})
	})
	t.Run("extend public", func(t *testing.T) {
		testVisibilityRuleSet(t, []string{"//visibility:public"}, []string{"//foo"}, []string{"//visibility:public"})
	})
	t.Run("extend private", func(t *testing.T) {
		testVisibilityRuleSet(t, []string{"//visibility:private"}, []string{"//foo"}, []string{"//foo"})
	})
	t.Run("extend with public", func(t *testing.T) {
		testVisibilityRuleSet(t, []string{"//foo"}, []string{"//visibility:public"}, []string{"//visibility:public"})
	})
	t.Run("extend with private", func(t *testing.T) {
		t.Helper()
		set := &visibilityRuleSet{[]string{"//foo"}}
		err := set.Widen([]string{"//visibility:private"})
		expectedError := `"//visibility:private" does not widen the visibility`
		if err == nil {
			t.Errorf("missing error")
		} else if err.Error() != expectedError {
			t.Errorf("expected error %q found error %q", expectedError, err)
		}
	})
}
