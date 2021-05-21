// Copyright 2019 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package java

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/dexpreopt"

	"github.com/google/blueprint"
)

const defaultJavaDir = "default/java"

// Test fixture preparer that will register most java build components.
//
// Singletons and mutators should only be added here if they are needed for a majority of java
// module types, otherwise they should be added under a separate preparer to allow them to be
// selected only when needed to reduce test execution time.
//
// Module types do not have much of an overhead unless they are used so this should include as many
// module types as possible. The exceptions are those module types that require mutators and/or
// singletons in order to function in which case they should be kept together in a separate
// preparer.
var PrepareForTestWithJavaBuildComponents = android.GroupFixturePreparers(
	// Make sure that mutators and module types, e.g. prebuilt mutators available.
	android.PrepareForTestWithAndroidBuildComponents,
	// Make java build components available to the test.
	android.FixtureRegisterWithContext(registerRequiredBuildComponentsForTest),
	android.FixtureRegisterWithContext(registerJavaPluginBuildComponents),
	// Additional files needed in tests that disallow non-existent source files.
	// This includes files that are needed by all, or at least most, instances of a java module type.
	android.MockFS{
		// Needed for linter used by java_library.
		"build/soong/java/lint_defaults.txt": nil,
		// Needed for apps that do not provide their own.
		"build/make/target/product/security": nil,
	}.AddToFixture(),
)

// Test fixture preparer that will define all default java modules except the
// fake_tool_binary for dex2oatd.
var PrepareForTestWithJavaDefaultModulesWithoutFakeDex2oatd = android.GroupFixturePreparers(
	// Make sure that all the module types used in the defaults are registered.
	PrepareForTestWithJavaBuildComponents,
	// Additional files needed when test disallows non-existent source.
	android.MockFS{
		// Needed for framework-res
		defaultJavaDir + "/AndroidManifest.xml": nil,
		// Needed for framework
		defaultJavaDir + "/framework/aidl": nil,
		// Needed for various deps defined in GatherRequiredDepsForTest()
		defaultJavaDir + "/a.java": nil,
	}.AddToFixture(),
	// The java default module definitions.
	android.FixtureAddTextFile(defaultJavaDir+"/Android.bp", gatherRequiredDepsForTest()),
	// Add dexpreopt compat libs (android.test.base, etc.) and a fake dex2oatd module.
	dexpreopt.PrepareForTestWithDexpreoptCompatLibs,
)

// Test fixture preparer that will define default java modules, e.g. standard prebuilt modules.
var PrepareForTestWithJavaDefaultModules = android.GroupFixturePreparers(
	PrepareForTestWithJavaDefaultModulesWithoutFakeDex2oatd,
	dexpreopt.PrepareForTestWithFakeDex2oatd,
)

// Provides everything needed by dexpreopt.
var PrepareForTestWithDexpreopt = android.GroupFixturePreparers(
	PrepareForTestWithJavaDefaultModules,
	dexpreopt.PrepareForTestByEnablingDexpreopt,
)

var PrepareForTestWithOverlayBuildComponents = android.FixtureRegisterWithContext(registerOverlayBuildComponents)

// Prepare a fixture to use all java module types, mutators and singletons fully.
//
// This should only be used by tests that want to run with as much of the build enabled as possible.
var PrepareForIntegrationTestWithJava = android.GroupFixturePreparers(
	cc.PrepareForIntegrationTestWithCc,
	PrepareForTestWithJavaDefaultModules,
)

// Prepare a fixture with the standard files required by a java_sdk_library module.
var PrepareForTestWithJavaSdkLibraryFiles = android.FixtureMergeMockFs(android.MockFS{
	"api/current.txt":               nil,
	"api/removed.txt":               nil,
	"api/system-current.txt":        nil,
	"api/system-removed.txt":        nil,
	"api/test-current.txt":          nil,
	"api/test-removed.txt":          nil,
	"api/module-lib-current.txt":    nil,
	"api/module-lib-removed.txt":    nil,
	"api/system-server-current.txt": nil,
	"api/system-server-removed.txt": nil,
})

// FixtureWithLastReleaseApis creates a preparer that creates prebuilt versions of the specified
// modules for the `last` API release. By `last` it just means last in the list of supplied versions
// and as this only provides one version it can be any value.
//
// This uses FixtureWithPrebuiltApis under the covers so the limitations of that apply to this.
func FixtureWithLastReleaseApis(moduleNames ...string) android.FixturePreparer {
	return FixtureWithPrebuiltApis(map[string][]string{
		"30": moduleNames,
	})
}

// PrepareForTestWithPrebuiltsOfCurrentApi is a preparer that creates prebuilt versions of the
// standard modules for the current version.
//
// This uses FixtureWithPrebuiltApis under the covers so the limitations of that apply to this.
var PrepareForTestWithPrebuiltsOfCurrentApi = FixtureWithPrebuiltApis(map[string][]string{
	"current": {},
	// Can't have current on its own as it adds a prebuilt_apis module but doesn't add any
	// .txt files which causes the prebuilt_apis module to fail.
	"30": {},
})

// FixtureWithPrebuiltApis creates a preparer that will define prebuilt api modules for the
// specified releases and modules.
//
// The supplied map keys are the releases, e.g. current, 29, 30, etc. The values are a list of
// modules for that release. Due to limitations in the prebuilt_apis module which this preparer
// uses the set of releases must include at least one numbered release, i.e. it cannot just include
// "current".
//
// This defines a file in the mock file system in a predefined location (prebuilts/sdk/Android.bp)
// and so only one instance of this can be used in each fixture.
func FixtureWithPrebuiltApis(release2Modules map[string][]string) android.FixturePreparer {
	mockFS := android.MockFS{}
	path := "prebuilts/sdk/Android.bp"

	bp := fmt.Sprintf(`
			prebuilt_apis {
				name: "sdk",
				api_dirs: ["%s"],
				imports_sdk_version: "none",
				imports_compile_dex: true,
			}
		`, strings.Join(android.SortedStringKeys(release2Modules), `", "`))

	for release, modules := range release2Modules {
		libs := append([]string{"android", "core-for-system-modules"}, modules...)
		mockFS.Merge(prebuiltApisFilesForLibs([]string{release}, libs))
	}
	return android.GroupFixturePreparers(
		android.FixtureAddTextFile(path, bp),
		android.FixtureMergeMockFs(mockFS),
	)
}

func prebuiltApisFilesForLibs(apiLevels []string, sdkLibs []string) map[string][]byte {
	fs := make(map[string][]byte)
	for _, level := range apiLevels {
		for _, lib := range sdkLibs {
			for _, scope := range []string{"public", "system", "module-lib", "system-server", "test"} {
				fs[fmt.Sprintf("prebuilts/sdk/%s/%s/%s.jar", level, scope, lib)] = nil
				// No finalized API files for "current"
				if level != "current" {
					fs[fmt.Sprintf("prebuilts/sdk/%s/%s/api/%s.txt", level, scope, lib)] = nil
					fs[fmt.Sprintf("prebuilts/sdk/%s/%s/api/%s-removed.txt", level, scope, lib)] = nil
				}
			}
		}
		fs[fmt.Sprintf("prebuilts/sdk/%s/public/framework.aidl", level)] = nil
	}
	return fs
}

// FixtureConfigureBootJars configures the boot jars in both the dexpreopt.GlobalConfig and
// Config.productVariables structs. As a side effect that enables dexpreopt.
func FixtureConfigureBootJars(bootJars ...string) android.FixturePreparer {
	artBootJars := []string{}
	for _, j := range bootJars {
		artApex := false
		for _, artApexName := range artApexNames {
			if strings.HasPrefix(j, artApexName+":") {
				artApex = true
				break
			}
		}
		if artApex {
			artBootJars = append(artBootJars, j)
		}
	}
	return android.GroupFixturePreparers(
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.BootJars = android.CreateTestConfiguredJarList(bootJars)
		}),
		dexpreopt.FixtureSetBootJars(bootJars...),
		dexpreopt.FixtureSetArtBootJars(artBootJars...),

		// Add a fake dex2oatd module.
		dexpreopt.PrepareForTestWithFakeDex2oatd,
	)
}

// FixtureConfigureUpdatableBootJars configures the updatable boot jars in both the
// dexpreopt.GlobalConfig and Config.productVariables structs. As a side effect that enables
// dexpreopt.
func FixtureConfigureUpdatableBootJars(bootJars ...string) android.FixturePreparer {
	return android.GroupFixturePreparers(
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.UpdatableBootJars = android.CreateTestConfiguredJarList(bootJars)
		}),
		dexpreopt.FixtureSetUpdatableBootJars(bootJars...),

		// Add a fake dex2oatd module.
		dexpreopt.PrepareForTestWithFakeDex2oatd,
	)
}

// registerRequiredBuildComponentsForTest registers the build components used by
// PrepareForTestWithJavaDefaultModules.
//
// As functionality is moved out of here into separate FixturePreparer instances they should also
// be moved into GatherRequiredDepsForTest for use by tests that have not yet switched to use test
// fixtures.
func registerRequiredBuildComponentsForTest(ctx android.RegistrationContext) {
	RegisterAARBuildComponents(ctx)
	RegisterAppBuildComponents(ctx)
	RegisterAppImportBuildComponents(ctx)
	RegisterAppSetBuildComponents(ctx)
	registerBootclasspathBuildComponents(ctx)
	registerBootclasspathFragmentBuildComponents(ctx)
	RegisterDexpreoptBootJarsComponents(ctx)
	RegisterDocsBuildComponents(ctx)
	RegisterGenRuleBuildComponents(ctx)
	registerJavaBuildComponents(ctx)
	registerPlatformBootclasspathBuildComponents(ctx)
	RegisterPrebuiltApisBuildComponents(ctx)
	RegisterRuntimeResourceOverlayBuildComponents(ctx)
	RegisterSdkLibraryBuildComponents(ctx)
	RegisterStubsBuildComponents(ctx)
	RegisterSystemModulesBuildComponents(ctx)
	registerSystemserverClasspathBuildComponents(ctx)
	registerLintBuildComponents(ctx)
}

// gatherRequiredDepsForTest gathers the module definitions used by
// PrepareForTestWithJavaDefaultModules.
//
// As functionality is moved out of here into separate FixturePreparer instances they should also
// be moved into GatherRequiredDepsForTest for use by tests that have not yet switched to use test
// fixtures.
func gatherRequiredDepsForTest() string {
	var bp string

	extraModules := []string{
		"core-lambda-stubs",
		"ext",
		"android_stubs_current",
		"android_system_stubs_current",
		"android_test_stubs_current",
		"android_module_lib_stubs_current",
		"android_system_server_stubs_current",
		"core.current.stubs",
		"legacy.core.platform.api.stubs",
		"stable.core.platform.api.stubs",
		"kotlin-stdlib",
		"kotlin-stdlib-jdk7",
		"kotlin-stdlib-jdk8",
		"kotlin-annotations",
	}

	for _, extra := range extraModules {
		bp += fmt.Sprintf(`
			java_library {
				name: "%s",
				srcs: ["a.java"],
				sdk_version: "none",
				system_modules: "stable-core-platform-api-stubs-system-modules",
				compile_dex: true,
			}
		`, extra)
	}

	bp += `
		java_library {
			name: "framework",
			srcs: ["a.java"],
			sdk_version: "none",
			system_modules: "stable-core-platform-api-stubs-system-modules",
			aidl: {
				export_include_dirs: ["framework/aidl"],
			},
		}

		android_app {
			name: "framework-res",
			sdk_version: "core_platform",
		}`

	systemModules := []string{
		"core-current-stubs-system-modules",
		"legacy-core-platform-api-stubs-system-modules",
		"stable-core-platform-api-stubs-system-modules",
	}

	for _, extra := range systemModules {
		bp += fmt.Sprintf(`
			java_system_modules {
				name: "%[1]s",
				libs: ["%[1]s-lib"],
			}
			java_library {
				name: "%[1]s-lib",
				sdk_version: "none",
				system_modules: "none",
			}
		`, extra)
	}

	// Make sure that the dex_bootjars singleton module is instantiated for the tests.
	bp += `
		dex_bootjars {
			name: "dex_bootjars",
		}
`

	return bp
}

func CheckModuleDependencies(t *testing.T, ctx *android.TestContext, name, variant string, expected []string) {
	t.Helper()
	module := ctx.ModuleForTests(name, variant).Module()
	deps := []string{}
	ctx.VisitDirectDeps(module, func(m blueprint.Module) {
		deps = append(deps, m.Name())
	})
	sort.Strings(deps)

	if actual := deps; !reflect.DeepEqual(expected, actual) {
		t.Errorf("expected %#q, found %#q", expected, actual)
	}
}

// CheckPlatformBootclasspathModules returns the apex:module pair for the modules depended upon by
// the platform-bootclasspath module.
func CheckPlatformBootclasspathModules(t *testing.T, result *android.TestResult, name string, expected []string) {
	t.Helper()
	platformBootclasspath := result.Module(name, "android_common").(*platformBootclasspathModule)
	pairs := ApexNamePairsFromModules(result.TestContext, platformBootclasspath.configuredModules)
	android.AssertDeepEquals(t, fmt.Sprintf("%s modules", "platform-bootclasspath"), expected, pairs)
}

// ApexNamePairsFromModules returns the apex:module pair for the supplied modules.
func ApexNamePairsFromModules(ctx *android.TestContext, modules []android.Module) []string {
	pairs := []string{}
	for _, module := range modules {
		pairs = append(pairs, apexNamePairFromModule(ctx, module))
	}
	return pairs
}

func apexNamePairFromModule(ctx *android.TestContext, module android.Module) string {
	name := module.Name()
	var apex string
	apexInfo := ctx.ModuleProvider(module, android.ApexInfoProvider).(android.ApexInfo)
	if apexInfo.IsForPlatform() {
		apex = "platform"
	} else {
		apex = apexInfo.InApexVariants[0]
	}

	return fmt.Sprintf("%s:%s", apex, name)
}

// CheckPlatformBootclasspathFragments returns the apex:module pair for the fragments depended upon
// by the platform-bootclasspath module.
func CheckPlatformBootclasspathFragments(t *testing.T, result *android.TestResult, name string, expected []string) {
	t.Helper()
	platformBootclasspath := result.Module(name, "android_common").(*platformBootclasspathModule)
	pairs := ApexNamePairsFromModules(result.TestContext, platformBootclasspath.fragments)
	android.AssertDeepEquals(t, fmt.Sprintf("%s fragments", "platform-bootclasspath"), expected, pairs)
}

func CheckHiddenAPIRuleInputs(t *testing.T, expected string, hiddenAPIRule android.TestingBuildParams) {
	t.Helper()
	actual := strings.TrimSpace(strings.Join(android.NormalizePathsForTesting(hiddenAPIRule.Implicits), "\n"))
	expected = strings.TrimSpace(expected)
	if actual != expected {
		t.Errorf("Expected hiddenapi rule inputs:\n%s\nactual inputs:\n%s", expected, actual)
	}
}

// Check that the merged file create by platform_compat_config_singleton has the correct inputs.
func CheckMergedCompatConfigInputs(t *testing.T, result *android.TestResult, message string, expectedPaths ...string) {
	sourceGlobalCompatConfig := result.SingletonForTests("platform_compat_config_singleton")
	allOutputs := sourceGlobalCompatConfig.AllOutputs()
	android.AssertIntEquals(t, message+": output len", 1, len(allOutputs))
	output := sourceGlobalCompatConfig.Output(allOutputs[0])
	android.AssertPathsRelativeToTopEquals(t, message+": inputs", expectedPaths, output.Implicits)
}
