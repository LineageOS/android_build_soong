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
	"testing"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/dexpreopt"
	"android/soong/python"

	"github.com/google/blueprint"
)

func TestConfig(buildDir string, env map[string]string, bp string, fs map[string][]byte) android.Config {
	bp += GatherRequiredDepsForTest()

	mockFS := map[string][]byte{
		"api/current.txt":        nil,
		"api/removed.txt":        nil,
		"api/system-current.txt": nil,
		"api/system-removed.txt": nil,
		"api/test-current.txt":   nil,
		"api/test-removed.txt":   nil,

		"prebuilts/sdk/tools/core-lambda-stubs.jar": nil,
		"prebuilts/sdk/Android.bp":                  []byte(`prebuilt_apis { name: "sdk", api_dirs: ["14", "28", "30", "current"], imports_sdk_version: "none", imports_compile_dex:true,}`),

		"bin.py": nil,
		python.StubTemplateHost: []byte(`PYTHON_BINARY = '%interpreter%'
		MAIN_FILE = '%main%'`),

		// For java_sdk_library
		"api/module-lib-current.txt":    nil,
		"api/module-lib-removed.txt":    nil,
		"api/system-server-current.txt": nil,
		"api/system-server-removed.txt": nil,
	}

	levels := []string{"14", "28", "29", "30", "current"}
	libs := []string{
		"android", "foo", "bar", "sdklib", "barney", "betty", "foo-shared_library",
		"foo-no_shared_library", "core-for-system-modules", "quuz", "qux", "fred",
		"runtime-library",
	}
	for k, v := range prebuiltApisFilesForLibs(levels, libs) {
		mockFS[k] = v
	}

	cc.GatherRequiredFilesForTest(mockFS)

	for k, v := range fs {
		mockFS[k] = v
	}

	if env == nil {
		env = make(map[string]string)
	}
	if env["ANDROID_JAVA8_HOME"] == "" {
		env["ANDROID_JAVA8_HOME"] = "jdk8"
	}
	config := android.TestArchConfig(buildDir, env, bp, mockFS)

	return config
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

// Register build components provided by this package that are needed by tests.
//
// In particular this must register all the components that are used in the `Android.bp` snippet
// returned by GatherRequiredDepsForTest()
func RegisterRequiredBuildComponentsForTest(ctx android.RegistrationContext) {
	RegisterAARBuildComponents(ctx)
	RegisterAppBuildComponents(ctx)
	RegisterAppImportBuildComponents(ctx)
	RegisterAppSetBuildComponents(ctx)
	RegisterDexpreoptBootJarsComponents(ctx)
	RegisterDocsBuildComponents(ctx)
	RegisterGenRuleBuildComponents(ctx)
	RegisterJavaBuildComponents(ctx)
	RegisterPrebuiltApisBuildComponents(ctx)
	RegisterRuntimeResourceOverlayBuildComponents(ctx)
	RegisterSdkLibraryBuildComponents(ctx)
	RegisterStubsBuildComponents(ctx)
	RegisterSystemModulesBuildComponents(ctx)
}

// Gather the module definitions needed by tests that depend upon code from this package.
//
// Returns an `Android.bp` snippet that defines the modules that are needed by this package.
func GatherRequiredDepsForTest() string {
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

	// For class loader context and <uses-library> tests.
	dexpreoptModules := []string{"android.test.runner"}
	dexpreoptModules = append(dexpreoptModules, dexpreopt.CompatUsesLibs...)
	dexpreoptModules = append(dexpreoptModules, dexpreopt.OptionalCompatUsesLibs...)

	for _, extra := range dexpreoptModules {
		bp += fmt.Sprintf(`
			java_library {
				name: "%s",
				srcs: ["a.java"],
				sdk_version: "none",
				system_modules: "stable-core-platform-api-stubs-system-modules",
				compile_dex: true,
				installable: true,
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
