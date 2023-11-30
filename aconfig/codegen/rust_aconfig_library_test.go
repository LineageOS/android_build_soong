package codegen

import (
	"fmt"
	"testing"

	"android/soong/android"
	"android/soong/rust"
)

func TestRustAconfigLibrary(t *testing.T) {
	result := android.GroupFixturePreparers(
		PrepareForTestWithAconfigBuildComponents,
		rust.PrepareForTestWithRustIncludeVndk,
		android.PrepareForTestWithArchMutator,
		android.PrepareForTestWithDefaults,
		android.PrepareForTestWithPrebuilts,
	).
		ExtendWithErrorHandler(android.FixtureExpectsNoErrors).
		RunTestWithBp(t, fmt.Sprintf(`
			rust_library {
				name: "libflags_rust", // test mock
				crate_name: "flags_rust",
				srcs: ["lib.rs"],
			}
			rust_library {
				name: "liblazy_static", // test mock
				crate_name: "lazy_static",
				srcs: ["src/lib.rs"],
			}
			aconfig_declarations {
				name: "my_aconfig_declarations",
				package: "com.example.package",
				srcs: ["foo.aconfig"],
			}

			rust_aconfig_library {
				name: "libmy_rust_aconfig_library",
				crate_name: "my_rust_aconfig_library",
				aconfig_declarations: "my_aconfig_declarations",
			}
		`))

	sourceVariant := result.ModuleForTests("libmy_rust_aconfig_library", "android_arm64_armv8-a_source")
	rule := sourceVariant.Rule("rust_aconfig_library")
	android.AssertStringEquals(t, "rule must contain production mode", rule.Args["mode"], "production")

	dylibVariant := result.ModuleForTests("libmy_rust_aconfig_library", "android_arm64_armv8-a_dylib")
	rlibRlibStdVariant := result.ModuleForTests("libmy_rust_aconfig_library", "android_arm64_armv8-a_rlib_rlib-std")
	rlibDylibStdVariant := result.ModuleForTests("libmy_rust_aconfig_library", "android_arm64_armv8-a_rlib_dylib-std")

	variants := []android.TestingModule{
		dylibVariant,
		rlibDylibStdVariant,
		rlibRlibStdVariant,
	}

	for _, variant := range variants {
		android.AssertStringEquals(
			t,
			"dylib variant builds from generated rust code",
			"out/soong/.intermediates/libmy_rust_aconfig_library/android_arm64_armv8-a_source/gen/src/lib.rs",
			variant.Rule("rustc").Inputs[0].RelativeToTop().String(),
		)
	}
}

var rustCodegenModeTestData = []struct {
	setting, expected string
}{
	{"", "production"},
	{"mode: `production`,", "production"},
	{"mode: `test`,", "test"},
	{"mode: `exported`,", "exported"},
}

func TestRustCodegenMode(t *testing.T) {
	for _, testData := range rustCodegenModeTestData {
		testRustCodegenModeHelper(t, testData.setting, testData.expected)
	}
}

func testRustCodegenModeHelper(t *testing.T, bpMode string, ruleMode string) {
	t.Helper()
	result := android.GroupFixturePreparers(
		PrepareForTestWithAconfigBuildComponents,
		rust.PrepareForTestWithRustIncludeVndk).
		ExtendWithErrorHandler(android.FixtureExpectsNoErrors).
		RunTestWithBp(t, fmt.Sprintf(`
			rust_library {
				name: "libflags_rust", // test mock
				crate_name: "flags_rust",
				srcs: ["lib.rs"],
			}
			rust_library {
				name: "liblazy_static", // test mock
				crate_name: "lazy_static",
				srcs: ["src/lib.rs"],
			}
			aconfig_declarations {
				name: "my_aconfig_declarations",
				package: "com.example.package",
				srcs: ["foo.aconfig"],
			}
			rust_aconfig_library {
				name: "libmy_rust_aconfig_library",
				crate_name: "my_rust_aconfig_library",
				aconfig_declarations: "my_aconfig_declarations",
				%s
			}
		`, bpMode))

	module := result.ModuleForTests("libmy_rust_aconfig_library", "android_arm64_armv8-a_source")
	rule := module.Rule("rust_aconfig_library")
	android.AssertStringEquals(t, "rule must contain test mode", rule.Args["mode"], ruleMode)
}

var incorrectRustCodegenModeTestData = []struct {
	setting, expectedErr string
}{
	{"mode: `unsupported`,", "mode: \"unsupported\" is not a supported mode"},
}

func TestIncorrectRustCodegenMode(t *testing.T) {
	for _, testData := range incorrectRustCodegenModeTestData {
		testIncorrectRustCodegenModeHelper(t, testData.setting, testData.expectedErr)
	}
}

func testIncorrectRustCodegenModeHelper(t *testing.T, bpMode string, err string) {
	t.Helper()
	android.GroupFixturePreparers(
		PrepareForTestWithAconfigBuildComponents,
		rust.PrepareForTestWithRustIncludeVndk).
		ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern(err)).
		RunTestWithBp(t, fmt.Sprintf(`
			rust_library {
				name: "libflags_rust", // test mock
				crate_name: "flags_rust",
				srcs: ["lib.rs"],
			}
			rust_library {
				name: "liblazy_static", // test mock
				crate_name: "lazy_static",
				srcs: ["src/lib.rs"],
			}
			aconfig_declarations {
				name: "my_aconfig_declarations",
				package: "com.example.package",
				srcs: ["foo.aconfig"],
			}
			rust_aconfig_library {
				name: "libmy_rust_aconfig_library",
				crate_name: "my_rust_aconfig_library",
				aconfig_declarations: "my_aconfig_declarations",
				%s
			}
		`, bpMode))
}
