package tradefed_modules

import (
	"android/soong/android"
	"android/soong/tradefed"
	"encoding/json"
	"fmt"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterTestModuleConfigBuildComponents(android.InitRegistrationContext)
}

// Register the license_kind module type.
func RegisterTestModuleConfigBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("test_module_config", TestModuleConfigFactory)
	ctx.RegisterModuleType("test_module_config_host", TestModuleConfigHostFactory)
}

type testModuleConfigModule struct {
	android.ModuleBase
	android.DefaultableModuleBase
	base android.Module

	tradefedProperties

	// Our updated testConfig.
	testConfig android.OutputPath
	manifest   android.InstallPath
	provider   tradefed.BaseTestProviderData
}

// Host is mostly the same as non-host, just some diffs for AddDependency and
// AndroidMkEntries, but the properties are the same.
type testModuleConfigHostModule struct {
	testModuleConfigModule
}

// Properties to list in Android.bp for this module.
type tradefedProperties struct {
	// Module name of the base test that we will run.
	Base *string `android:"path,arch_variant"`

	// Tradefed Options to add to tradefed xml when not one of the include or exclude filter or property.
	// Sample: [{name: "TestRunnerOptionName", value: "OptionValue" }]
	Options []tradefed.Option

	// List of tradefed include annotations to add to tradefed xml, like "android.platform.test.annotations.Presubmit".
	// Tests will be restricted to those matching an include_annotation or include_filter.
	Include_annotations []string

	// List of tradefed include annotations to add to tradefed xml, like "android.support.test.filters.FlakyTest".
	// Tests matching an exclude annotation or filter will be skipped.
	Exclude_annotations []string

	// List of tradefed include filters to add to tradefed xml, like "fully.qualified.class#method".
	// Tests will be restricted to those matching an include_annotation or include_filter.
	Include_filters []string

	// List of tradefed exclude filters to add to tradefed xml, like "fully.qualified.class#method".
	// Tests matching an exclude annotation or filter will be skipped.
	Exclude_filters []string

	// List of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var (
	testModuleConfigTag     = dependencyTag{name: "TestModuleConfigBase"}
	testModuleConfigHostTag = dependencyTag{name: "TestModuleConfigHostBase"}
	pctx                    = android.NewPackageContext("android/soong/tradefed_modules")
)

func (m *testModuleConfigModule) InstallInTestcases() bool {
	return true
}

func (m *testModuleConfigModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	if m.Base == nil {
		ctx.ModuleErrorf("'base' field must be set to a 'android_test' module.")
		return
	}
	ctx.AddDependency(ctx.Module(), testModuleConfigTag, *m.Base)
}

// Takes base's Tradefed Config xml file and generates a new one with the test properties
// appeneded from this module.
// Rewrite the name of the apk in "test-file-name" to be our module's name, rather than the original one.
func (m *testModuleConfigModule) fixTestConfig(ctx android.ModuleContext, baseTestConfig android.Path) android.OutputPath {
	// Test safe to do when no test_runner_options, but check for that earlier?
	fixedConfig := android.PathForModuleOut(ctx, "test_config_fixer", ctx.ModuleName()+".config")
	rule := android.NewRuleBuilder(pctx, ctx)
	command := rule.Command().BuiltTool("test_config_fixer").Input(baseTestConfig).Output(fixedConfig)
	options := m.composeOptions()
	if len(options) == 0 {
		ctx.ModuleErrorf("Test options must be given when using test_module_config. Set include/exclude filter or annotation.")
	}
	xmlTestModuleConfigSnippet, _ := json.Marshal(options)
	escaped := proptools.NinjaAndShellEscape(string(xmlTestModuleConfigSnippet))
	command.FlagWithArg("--test-file-name=", ctx.ModuleName()+".apk").
		FlagWithArg("--orig-test-file-name=", *m.tradefedProperties.Base+".apk").
		FlagWithArg("--test-runner-options=", escaped)
	rule.Build("fix_test_config", "fix test config")
	return fixedConfig.OutputPath
}

// Convert --exclude_filters: ["filter1", "filter2"] ->
// [ Option{Name: "exclude-filters", Value: "filter1"}, Option{Name: "exclude-filters", Value: "filter2"},
// ... + include + annotations ]
func (m *testModuleConfigModule) composeOptions() []tradefed.Option {
	options := m.Options
	for _, e := range m.Exclude_filters {
		options = append(options, tradefed.Option{Name: "exclude-filter", Value: e})
	}
	for _, i := range m.Include_filters {
		options = append(options, tradefed.Option{Name: "include-filter", Value: i})
	}
	for _, e := range m.Exclude_annotations {
		options = append(options, tradefed.Option{Name: "exclude-annotation", Value: e})
	}
	for _, i := range m.Include_annotations {
		options = append(options, tradefed.Option{Name: "include-annotation", Value: i})
	}
	return options
}

// Files to write and where they come from:
// 1) test_module_config.manifest
//   - Leave a trail of where we got files from in case other tools need it.
//
// 2) $Module.config
//   - comes from base's module.config (AndroidTest.xml), and then we add our test_options.
//     provider.TestConfig
//     [rules via soong_app_prebuilt]
//
// 3) $ARCH/$Module.apk
//   - comes from base
//     provider.OutputFile
//     [rules via soong_app_prebuilt]
//
// 4) [bases data]
//   - We copy all of bases data (like helper apks) to our install directory too.
//     Since we call AndroidMkEntries on base, it will write out LOCAL_COMPATIBILITY_SUPPORT_FILES
//     with this data and app_prebuilt.mk will generate the rules to copy it from base.
//     We have no direct rules here to add to ninja.
//
// If we change to symlinks, this all needs to change.
func (m *testModuleConfigModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	m.validateBase(ctx, &testModuleConfigTag, "android_test", false)
	m.generateManifestAndConfig(ctx)

}

// Ensure at least one test_suite is listed.  Ideally it should be general-tests
// or device-tests, whichever is listed in base and prefer general-tests if both are listed.
// However this is not enforced yet.
//
// Returns true if okay and reports errors via ModuleErrorf.
func (m *testModuleConfigModule) validateTestSuites(ctx android.ModuleContext) bool {
	if len(m.tradefedProperties.Test_suites) == 0 {
		ctx.ModuleErrorf("At least one test-suite must be set or this won't run. Use \"general-tests\" or \"device-tests\"")
		return false
	}

	return true
}

func TestModuleConfigFactory() android.Module {
	module := &testModuleConfigModule{}

	module.AddProperties(&module.tradefedProperties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)

	return module
}

func TestModuleConfigHostFactory() android.Module {
	module := &testModuleConfigHostModule{}

	module.AddProperties(&module.tradefedProperties)
	android.InitAndroidMultiTargetsArchModule(module, android.HostSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)

	return module
}

// Implements android.AndroidMkEntriesProvider
var _ android.AndroidMkEntriesProvider = (*testModuleConfigModule)(nil)

func (m *testModuleConfigModule) AndroidMkEntries() []android.AndroidMkEntries {
	// We rely on base writing LOCAL_COMPATIBILITY_SUPPORT_FILES for its data files
	entriesList := m.base.(android.AndroidMkEntriesProvider).AndroidMkEntries()
	entries := &entriesList[0]
	entries.OutputFile = android.OptionalPathForPath(m.provider.OutputFile)
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		entries.SetString("LOCAL_MODULE", m.Name()) //  out module name, not base's

		// Out update config file with extra options.
		entries.SetPath("LOCAL_FULL_TEST_CONFIG", m.testConfig)
		entries.SetString("LOCAL_MODULE_TAGS", "tests")

		// Don't append to base's test-suites, only use the ones we define, so clear it before
		// appending to it.
		entries.SetString("LOCAL_COMPATIBILITY_SUITE", "")
		entries.AddCompatibilityTestSuites(m.tradefedProperties.Test_suites...)

		if len(m.provider.HostRequiredModuleNames) > 0 {
			entries.AddStrings("LOCAL_HOST_REQUIRED_MODULES", m.provider.HostRequiredModuleNames...)
		}
		if len(m.provider.RequiredModuleNames) > 0 {
			entries.AddStrings("LOCAL_REQUIRED_MODULES", m.provider.RequiredModuleNames...)
		}

		if m.provider.IsHost == false {
			// Not needed for jar_host_test
			//
			// Clear the JNI symbols because they belong to base not us. Either transform the names in the string
			// or clear the variable because we don't need it, we are copying bases libraries not generating
			// new ones.
			entries.SetString("LOCAL_SOONG_JNI_LIBS_SYMBOLS", "")
		}
	})
	return entriesList
}

func (m *testModuleConfigHostModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	if m.Base == nil {
		ctx.ModuleErrorf("'base' field must be set to a 'java_test_host' module")
		return
	}
	ctx.AddVariationDependencies(ctx.Config().BuildOSCommonTarget.Variations(), testModuleConfigHostTag, *m.Base)
}

// File to write:
// 1) out/host/linux-x86/testcases/derived-module/test_module_config.manifest # contains base's name.
// 2) out/host/linux-x86/testcases/derived-module/derived-module.config  # Update AnroidTest.xml
// 3) out/host/linux-x86/testcases/derived-module/base.jar
//   - written via soong_java_prebuilt.mk
//
// 4) out/host/linux-x86/testcases/derived-module/* # data dependencies from base.
//   - written via soong_java_prebuilt.mk
func (m *testModuleConfigHostModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	m.validateBase(ctx, &testModuleConfigHostTag, "java_test_host", true)
	m.generateManifestAndConfig(ctx)
}

// Ensure the base listed is the right type by checking that we get the expected provider data.
// Returns false on errors and the context is updated with an error indicating the baseType expected.
func (m *testModuleConfigModule) validateBase(ctx android.ModuleContext, depTag *dependencyTag, baseType string, baseShouldBeHost bool) {
	ctx.VisitDirectDepsWithTag(*depTag, func(dep android.Module) {
		if provider, ok := android.OtherModuleProvider(ctx, dep, tradefed.BaseTestProviderKey); ok {
			if baseShouldBeHost == provider.IsHost {
				m.base = dep
				m.provider = provider
			} else {
				if baseShouldBeHost {
					ctx.ModuleErrorf("'android_test' module used as base, but 'java_test_host' expected.")
				} else {
					ctx.ModuleErrorf("'java_test_host' module used as base, but 'android_test' expected.")
				}
			}
		} else {
			ctx.ModuleErrorf("'%s' module used as base but it is not a '%s' module.", *m.Base, baseType)
		}
	})
}

// Actions to write:
//  1. manifest file to testcases dir
//  2. New Module.config / AndroidTest.xml file with our options.
func (m *testModuleConfigModule) generateManifestAndConfig(ctx android.ModuleContext) {
	// Keep before early returns.
	android.SetProvider(ctx, android.TestOnlyProviderKey, android.TestModuleInformation{
		TestOnly:       true,
		TopLevelTarget: true,
	})

	if !m.validateTestSuites(ctx) {
		return
	}
	// Ensure the base provider is accurate
	if m.provider.TestConfig == nil {
		return
	}

	// 1) A manifest file listing the base, write text to a tiny file.
	installDir := android.PathForModuleInstall(ctx, ctx.ModuleName())
	manifest := android.PathForModuleOut(ctx, "test_module_config.manifest")
	android.WriteFileRule(ctx, manifest, fmt.Sprintf("{%q: %q}", "base", *m.tradefedProperties.Base))
	// build/soong/android/androidmk.go has this comment:
	//    Assume the primary install file is last
	// so we need to Install our file last.
	ctx.InstallFile(installDir, manifest.Base(), manifest)

	// 2) Module.config / AndroidTest.xml
	m.testConfig = m.fixTestConfig(ctx, m.provider.TestConfig)
}

var _ android.AndroidMkEntriesProvider = (*testModuleConfigHostModule)(nil)
