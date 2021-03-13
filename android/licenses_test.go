package android

import (
	"testing"

	"github.com/google/blueprint"
)

var prepareForTestWithLicenses = GroupFixturePreparers(
	FixtureRegisterWithContext(RegisterLicenseKindBuildComponents),
	FixtureRegisterWithContext(RegisterLicenseBuildComponents),
	FixtureRegisterWithContext(registerLicenseMutators),
)

func registerLicenseMutators(ctx RegistrationContext) {
	ctx.PreArchMutators(RegisterLicensesPackageMapper)
	ctx.PreArchMutators(RegisterLicensesPropertyGatherer)
	ctx.PostDepsMutators(RegisterLicensesDependencyChecker)
}

var licensesTests = []struct {
	name                       string
	fs                         MockFS
	expectedErrors             []string
	effectiveLicenses          map[string][]string
	effectiveInheritedLicenses map[string][]string
	effectivePackage           map[string]string
	effectiveNotices           map[string][]string
	effectiveKinds             map[string][]string
	effectiveConditions        map[string][]string
}{
	{
		name: "invalid module type without licenses property",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_bad_module {
					name: "libexample",
				}`),
		},
		expectedErrors: []string{`module type "mock_bad_module" must have an applicable licenses property`},
	},
	{
		name: "license must exist",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_library {
					name: "libexample",
					licenses: ["notice"],
				}`),
		},
		expectedErrors: []string{`"libexample" depends on undefined module "notice"`},
	},
	{
		name: "all good",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license_kind {
					name: "notice",
					conditions: ["shownotice"],
				}

				license {
					name: "top_Apache2",
					license_kinds: ["notice"],
					package_name: "topDog",
					license_text: ["LICENSE", "NOTICE"],
				}

				mock_library {
					name: "libexample1",
					licenses: ["top_Apache2"],
				}`),
			"top/nested/Blueprints": []byte(`
				mock_library {
					name: "libnested",
					licenses: ["top_Apache2"],
				}`),
			"other/Blueprints": []byte(`
				mock_library {
					name: "libother",
					licenses: ["top_Apache2"],
				}`),
		},
		effectiveLicenses: map[string][]string{
			"libexample1": []string{"top_Apache2"},
			"libnested":   []string{"top_Apache2"},
			"libother":    []string{"top_Apache2"},
		},
		effectiveKinds: map[string][]string{
			"libexample1": []string{"notice"},
			"libnested":   []string{"notice"},
			"libother":    []string{"notice"},
		},
		effectivePackage: map[string]string{
			"libexample1": "topDog",
			"libnested":   "topDog",
			"libother":    "topDog",
		},
		effectiveConditions: map[string][]string{
			"libexample1": []string{"shownotice"},
			"libnested":   []string{"shownotice"},
			"libother":    []string{"shownotice"},
		},
		effectiveNotices: map[string][]string{
			"libexample1": []string{"top/LICENSE", "top/NOTICE"},
			"libnested":   []string{"top/LICENSE", "top/NOTICE"},
			"libother":    []string{"top/LICENSE", "top/NOTICE"},
		},
	},

	// Defaults propagation tests
	{
		// Check that licenses is the union of the defaults modules.
		name: "defaults union, basic",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license_kind {
					name: "top_notice",
					conditions: ["notice"],
				}

				license {
					name: "top_other",
					license_kinds: ["top_notice"],
				}

				mock_defaults {
					name: "libexample_defaults",
					licenses: ["top_other"],
				}
				mock_library {
					name: "libexample",
					licenses: ["nested_other"],
					defaults: ["libexample_defaults"],
				}
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Blueprints": []byte(`
				license_kind {
					name: "nested_notice",
					conditions: ["notice"],
				}

				license {
					name: "nested_other",
					license_kinds: ["nested_notice"],
				}

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
		effectiveLicenses: map[string][]string{
			"libexample":     []string{"nested_other", "top_other"},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
		},
		effectiveInheritedLicenses: map[string][]string{
			"libexample":     []string{"nested_other", "top_other"},
			"libsamepackage": []string{"nested_other", "top_other"},
			"libnested":      []string{"nested_other", "top_other"},
			"libother":       []string{"nested_other", "top_other"},
		},
		effectiveKinds: map[string][]string{
			"libexample":     []string{"nested_notice", "top_notice"},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
		},
		effectiveConditions: map[string][]string{
			"libexample":     []string{"notice"},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
		},
	},
	{
		name: "defaults union, multiple defaults",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				license {
					name: "top",
				}
				mock_defaults {
					name: "libexample_defaults_1",
					licenses: ["other"],
				}
				mock_defaults {
					name: "libexample_defaults_2",
					licenses: ["top_nested"],
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
				license {
					name: "top_nested",
					license_text: ["LICENSE.txt"],
				}
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Blueprints": []byte(`
				license {
					name: "other",
				}
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
		effectiveLicenses: map[string][]string{
			"libexample":     []string{"other", "top_nested"},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
			"liboutsider":    []string{},
		},
		effectiveInheritedLicenses: map[string][]string{
			"libexample":     []string{"other", "top_nested"},
			"libsamepackage": []string{"other", "top_nested"},
			"libnested":      []string{"other", "top_nested"},
			"libother":       []string{"other", "top_nested"},
			"liboutsider":    []string{"other", "top_nested"},
		},
		effectiveKinds: map[string][]string{
			"libexample":     []string{},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
			"liboutsider":    []string{},
		},
		effectiveNotices: map[string][]string{
			"libexample":     []string{"top/nested/LICENSE.txt"},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
			"liboutsider":    []string{},
		},
	},

	// Defaults module's defaults_licenses tests
	{
		name: "defaults_licenses invalid",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				mock_defaults {
					name: "top_defaults",
					licenses: ["notice"],
				}`),
		},
		expectedErrors: []string{`"top_defaults" depends on undefined module "notice"`},
	},
	{
		name: "defaults_licenses overrides package default",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_applicable_licenses: ["by_exception_only"],
				}
				license {
					name: "by_exception_only",
				}
				license {
					name: "notice",
				}
				mock_defaults {
					name: "top_defaults",
					licenses: ["notice"],
				}
				mock_library {
					name: "libexample",
				}
				mock_library {
					name: "libdefaults",
					defaults: ["top_defaults"],
				}`),
		},
		effectiveLicenses: map[string][]string{
			"libexample":  []string{"by_exception_only"},
			"libdefaults": []string{"notice"},
		},
		effectiveInheritedLicenses: map[string][]string{
			"libexample":  []string{"by_exception_only"},
			"libdefaults": []string{"notice"},
		},
	},

	// Package default_applicable_licenses tests
	{
		name: "package default_applicable_licenses must exist",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_applicable_licenses: ["notice"],
				}`),
		},
		expectedErrors: []string{`"//top" depends on undefined module "notice"`},
	},
	{
		// This test relies on the default licenses being legacy_public.
		name: "package default_applicable_licenses property used when no licenses specified",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_applicable_licenses: ["top_notice"],
				}

				license {
					name: "top_notice",
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
		effectiveLicenses: map[string][]string{
			"libexample":  []string{"top_notice"},
			"liboutsider": []string{},
		},
		effectiveInheritedLicenses: map[string][]string{
			"libexample":  []string{"top_notice"},
			"liboutsider": []string{"top_notice"},
		},
	},
	{
		name: "package default_applicable_licenses not inherited to subpackages",
		fs: map[string][]byte{
			"top/Blueprints": []byte(`
				package {
					default_applicable_licenses: ["top_notice"],
				}
				license {
					name: "top_notice",
				}
				mock_library {
					name: "libexample",
				}`),
			"top/nested/Blueprints": []byte(`
				package {
					default_applicable_licenses: ["outsider"],
				}

				mock_library {
					name: "libnested",
				}`),
			"top/other/Blueprints": []byte(`
				mock_library {
					name: "libother",
				}`),
			"outsider/Blueprints": []byte(`
				license {
					name: "outsider",
				}
				mock_library {
					name: "liboutsider",
					deps: ["libexample", "libother", "libnested"],
				}`),
		},
		effectiveLicenses: map[string][]string{
			"libexample":  []string{"top_notice"},
			"libnested":   []string{"outsider"},
			"libother":    []string{},
			"liboutsider": []string{},
		},
		effectiveInheritedLicenses: map[string][]string{
			"libexample":  []string{"top_notice"},
			"libnested":   []string{"outsider"},
			"libother":    []string{},
			"liboutsider": []string{"top_notice", "outsider"},
		},
	},
	{
		name: "verify that prebuilt dependencies are included",
		fs: map[string][]byte{
			"prebuilts/Blueprints": []byte(`
				license {
					name: "prebuilt"
				}
				prebuilt {
					name: "module",
					licenses: ["prebuilt"],
				}`),
			"top/sources/source_file": nil,
			"top/sources/Blueprints": []byte(`
				license {
					name: "top_sources"
				}
				source {
					name: "module",
					licenses: ["top_sources"],
				}`),
			"top/other/source_file": nil,
			"top/other/Blueprints": []byte(`
				source {
					name: "other",
					deps: [":module"],
				}`),
		},
		effectiveLicenses: map[string][]string{
			"other": []string{},
		},
		effectiveInheritedLicenses: map[string][]string{
			"other": []string{"prebuilt", "top_sources"},
		},
	},
	{
		name: "verify that prebuilt dependencies are ignored for licenses reasons (preferred)",
		fs: map[string][]byte{
			"prebuilts/Blueprints": []byte(`
				license {
					name: "prebuilt"
				}
				prebuilt {
					name: "module",
					licenses: ["prebuilt"],
					prefer: true,
				}`),
			"top/sources/source_file": nil,
			"top/sources/Blueprints": []byte(`
				license {
					name: "top_sources"
				}
				source {
					name: "module",
					licenses: ["top_sources"],
				}`),
			"top/other/source_file": nil,
			"top/other/Blueprints": []byte(`
				source {
					name: "other",
					deps: [":module"],
				}`),
		},
		effectiveLicenses: map[string][]string{
			"other": []string{},
		},
		effectiveInheritedLicenses: map[string][]string{
			"module": []string{"prebuilt", "top_sources"},
			"other":  []string{"prebuilt", "top_sources"},
		},
	},
}

func TestLicenses(t *testing.T) {
	for _, test := range licensesTests {
		t.Run(test.name, func(t *testing.T) {
			// Customize the common license text fixture factory.
			result := licenseTestFixtureFactory.Extend(
				FixtureRegisterWithContext(func(ctx RegistrationContext) {
					ctx.RegisterModuleType("mock_bad_module", newMockLicensesBadModule)
					ctx.RegisterModuleType("mock_library", newMockLicensesLibraryModule)
					ctx.RegisterModuleType("mock_defaults", defaultsLicensesFactory)
				}),
				test.fs.AddToFixture(),
			).
				ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern(test.expectedErrors)).
				RunTest(t)

			if test.effectiveLicenses != nil {
				checkEffectiveLicenses(t, result, test.effectiveLicenses)
			}

			if test.effectivePackage != nil {
				checkEffectivePackage(t, result, test.effectivePackage)
			}

			if test.effectiveNotices != nil {
				checkEffectiveNotices(t, result, test.effectiveNotices)
			}

			if test.effectiveKinds != nil {
				checkEffectiveKinds(t, result, test.effectiveKinds)
			}

			if test.effectiveConditions != nil {
				checkEffectiveConditions(t, result, test.effectiveConditions)
			}

			if test.effectiveInheritedLicenses != nil {
				checkEffectiveInheritedLicenses(t, result, test.effectiveInheritedLicenses)
			}
		})
	}
}

func checkEffectiveLicenses(t *testing.T, result *TestResult, effectiveLicenses map[string][]string) {
	actualLicenses := make(map[string][]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}
		actualLicenses[m.Name()] = base.commonProperties.Effective_licenses
	})

	for moduleName, expectedLicenses := range effectiveLicenses {
		licenses, ok := actualLicenses[moduleName]
		if !ok {
			licenses = []string{}
		}
		if !compareUnorderedStringArrays(expectedLicenses, licenses) {
			t.Errorf("effective licenses mismatch for module %q: expected %q, found %q", moduleName, expectedLicenses, licenses)
		}
	}
}

func checkEffectiveInheritedLicenses(t *testing.T, result *TestResult, effectiveInheritedLicenses map[string][]string) {
	actualLicenses := make(map[string][]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}
		inherited := make(map[string]bool)
		for _, l := range base.commonProperties.Effective_licenses {
			inherited[l] = true
		}
		result.Context.Context.VisitDepsDepthFirst(m, func(c blueprint.Module) {
			if _, ok := c.(*licenseModule); ok {
				return
			}
			if _, ok := c.(*licenseKindModule); ok {
				return
			}
			if _, ok := c.(*packageModule); ok {
				return
			}
			cmodule, ok := c.(Module)
			if !ok {
				t.Errorf("%q not a module", c.Name())
				return
			}
			cbase := cmodule.base()
			if cbase == nil {
				return
			}
			for _, l := range cbase.commonProperties.Effective_licenses {
				inherited[l] = true
			}
		})
		actualLicenses[m.Name()] = []string{}
		for l := range inherited {
			actualLicenses[m.Name()] = append(actualLicenses[m.Name()], l)
		}
	})

	for moduleName, expectedInheritedLicenses := range effectiveInheritedLicenses {
		licenses, ok := actualLicenses[moduleName]
		if !ok {
			licenses = []string{}
		}
		if !compareUnorderedStringArrays(expectedInheritedLicenses, licenses) {
			t.Errorf("effective inherited licenses mismatch for module %q: expected %q, found %q", moduleName, expectedInheritedLicenses, licenses)
		}
	}
}

func checkEffectivePackage(t *testing.T, result *TestResult, effectivePackage map[string]string) {
	actualPackage := make(map[string]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}

		if base.commonProperties.Effective_package_name == nil {
			actualPackage[m.Name()] = ""
		} else {
			actualPackage[m.Name()] = *base.commonProperties.Effective_package_name
		}
	})

	for moduleName, expectedPackage := range effectivePackage {
		packageName, ok := actualPackage[moduleName]
		if !ok {
			packageName = ""
		}
		if expectedPackage != packageName {
			t.Errorf("effective package mismatch for module %q: expected %q, found %q", moduleName, expectedPackage, packageName)
		}
	}
}

func checkEffectiveNotices(t *testing.T, result *TestResult, effectiveNotices map[string][]string) {
	actualNotices := make(map[string][]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}
		actualNotices[m.Name()] = base.commonProperties.Effective_license_text
	})

	for moduleName, expectedNotices := range effectiveNotices {
		notices, ok := actualNotices[moduleName]
		if !ok {
			notices = []string{}
		}
		if !compareUnorderedStringArrays(expectedNotices, notices) {
			t.Errorf("effective notice files mismatch for module %q: expected %q, found %q", moduleName, expectedNotices, notices)
		}
	}
}

func checkEffectiveKinds(t *testing.T, result *TestResult, effectiveKinds map[string][]string) {
	actualKinds := make(map[string][]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}
		actualKinds[m.Name()] = base.commonProperties.Effective_license_kinds
	})

	for moduleName, expectedKinds := range effectiveKinds {
		kinds, ok := actualKinds[moduleName]
		if !ok {
			kinds = []string{}
		}
		if !compareUnorderedStringArrays(expectedKinds, kinds) {
			t.Errorf("effective license kinds mismatch for module %q: expected %q, found %q", moduleName, expectedKinds, kinds)
		}
	}
}

func checkEffectiveConditions(t *testing.T, result *TestResult, effectiveConditions map[string][]string) {
	actualConditions := make(map[string][]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}
		actualConditions[m.Name()] = base.commonProperties.Effective_license_conditions
	})

	for moduleName, expectedConditions := range effectiveConditions {
		conditions, ok := actualConditions[moduleName]
		if !ok {
			conditions = []string{}
		}
		if !compareUnorderedStringArrays(expectedConditions, conditions) {
			t.Errorf("effective license conditions mismatch for module %q: expected %q, found %q", moduleName, expectedConditions, conditions)
		}
	}
}

func compareUnorderedStringArrays(expected, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}
	s := make(map[string]int)
	for _, v := range expected {
		s[v] += 1
	}
	for _, v := range actual {
		c, ok := s[v]
		if !ok {
			return false
		}
		if c < 1 {
			return false
		}
		s[v] -= 1
	}
	return true
}

type mockLicensesBadProperties struct {
	Visibility []string
}

type mockLicensesBadModule struct {
	ModuleBase
	DefaultableModuleBase
	properties mockLicensesBadProperties
}

func newMockLicensesBadModule() Module {
	m := &mockLicensesBadModule{}

	base := m.base()
	m.AddProperties(&base.nameProperties, &m.properties)

	base.generalProperties = m.GetProperties()
	base.customizableProperties = m.GetProperties()

	// The default_visibility property needs to be checked and parsed by the visibility module during
	// its checking and parsing phases so make it the primary visibility property.
	setPrimaryVisibilityProperty(m, "visibility", &m.properties.Visibility)

	initAndroidModuleBase(m)
	InitDefaultableModule(m)

	return m
}

func (m *mockLicensesBadModule) GenerateAndroidBuildActions(ModuleContext) {
}

type mockLicensesLibraryProperties struct {
	Deps []string
}

type mockLicensesLibraryModule struct {
	ModuleBase
	DefaultableModuleBase
	properties mockLicensesLibraryProperties
}

func newMockLicensesLibraryModule() Module {
	m := &mockLicensesLibraryModule{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibCommon)
	InitDefaultableModule(m)
	return m
}

type dependencyLicensesTag struct {
	blueprint.BaseDependencyTag
	name string
}

func (j *mockLicensesLibraryModule) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddVariationDependencies(nil, dependencyLicensesTag{name: "mockdeps"}, j.properties.Deps...)
}

func (p *mockLicensesLibraryModule) GenerateAndroidBuildActions(ModuleContext) {
}

type mockLicensesDefaults struct {
	ModuleBase
	DefaultsModuleBase
}

func defaultsLicensesFactory() Module {
	m := &mockLicensesDefaults{}
	InitDefaultsModule(m)
	return m
}
