// Copyright 2018 Google Inc. All rights reserved.
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

package dexpreopt

import (
	"android/soong/android"
	"fmt"
	"testing"
)

func testSystemModuleConfig(ctx android.PathContext, name string) *ModuleConfig {
	return testModuleConfig(ctx, name, "system")
}

func testSystemProductModuleConfig(ctx android.PathContext, name string) *ModuleConfig {
	return testModuleConfig(ctx, name, "system/product")
}

func testProductModuleConfig(ctx android.PathContext, name string) *ModuleConfig {
	return testModuleConfig(ctx, name, "product")
}

func testModuleConfig(ctx android.PathContext, name, partition string) *ModuleConfig {
	return createTestModuleConfig(
		name,
		fmt.Sprintf("/%s/app/test/%s.apk", partition, name),
		android.PathForOutput(ctx, fmt.Sprintf("%s/%s.apk", name, name)),
		android.PathForOutput(ctx, fmt.Sprintf("%s/dex/%s.jar", name, name)),
		android.PathForOutput(ctx, fmt.Sprintf("%s/enforce_uses_libraries.status", name)))
}

func testApexModuleConfig(ctx android.PathContext, name, apexName string) *ModuleConfig {
	return createTestModuleConfig(
		name,
		fmt.Sprintf("/apex/%s/javalib/%s.jar", apexName, name),
		android.PathForOutput(ctx, fmt.Sprintf("%s/dexpreopt/%s.jar", name, name)),
		android.PathForOutput(ctx, fmt.Sprintf("%s/aligned/%s.jar", name, name)),
		android.PathForOutput(ctx, fmt.Sprintf("%s/enforce_uses_libraries.status", name)))
}

func testPlatformSystemServerModuleConfig(ctx android.PathContext, name string) *ModuleConfig {
	return createTestModuleConfig(
		name,
		fmt.Sprintf("/system/framework/%s.jar", name),
		android.PathForOutput(ctx, fmt.Sprintf("%s/dexpreopt/%s.jar", name, name)),
		android.PathForOutput(ctx, fmt.Sprintf("%s/aligned/%s.jar", name, name)),
		android.PathForOutput(ctx, fmt.Sprintf("%s/enforce_uses_libraries.status", name)))
}

func testSystemExtSystemServerModuleConfig(ctx android.PathContext, name string) *ModuleConfig {
	return createTestModuleConfig(
		name,
		fmt.Sprintf("/system_ext/framework/%s.jar", name),
		android.PathForOutput(ctx, fmt.Sprintf("%s/dexpreopt/%s.jar", name, name)),
		android.PathForOutput(ctx, fmt.Sprintf("%s/aligned/%s.jar", name, name)),
		android.PathForOutput(ctx, fmt.Sprintf("%s/enforce_uses_libraries.status", name)))
}

func createTestModuleConfig(name, dexLocation string, buildPath, dexPath, enforceUsesLibrariesStatusFile android.OutputPath) *ModuleConfig {
	return &ModuleConfig{
		Name:                            name,
		DexLocation:                     dexLocation,
		BuildPath:                       buildPath,
		DexPath:                         dexPath,
		UncompressedDex:                 false,
		HasApkLibraries:                 false,
		PreoptFlags:                     nil,
		ProfileClassListing:             android.OptionalPath{},
		ProfileIsTextListing:            false,
		EnforceUsesLibrariesStatusFile:  enforceUsesLibrariesStatusFile,
		EnforceUsesLibraries:            false,
		ClassLoaderContexts:             nil,
		Archs:                           []android.ArchType{android.Arm},
		DexPreoptImagesDeps:             []android.OutputPaths{android.OutputPaths{}},
		DexPreoptImageLocationsOnHost:   []string{},
		PreoptBootClassPathDexFiles:     nil,
		PreoptBootClassPathDexLocations: nil,
		NoCreateAppImage:                false,
		ForceCreateAppImage:             false,
		PresignedPrebuilt:               false,
	}
}

func TestDexPreopt(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.BuilderContextForTesting(config)
	globalSoong := globalSoongConfigForTests()
	global := GlobalConfigForTests(ctx)
	module := testSystemModuleConfig(ctx, "test")
	productPackages := android.PathForTesting("product_packages.txt")

	rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, module, productPackages)
	if err != nil {
		t.Fatal(err)
	}

	wantInstalls := android.RuleBuilderInstalls{
		{android.PathForOutput(ctx, "test/oat/arm/package.odex"), "/system/app/test/oat/arm/test.odex"},
		{android.PathForOutput(ctx, "test/oat/arm/package.vdex"), "/system/app/test/oat/arm/test.vdex"},
	}

	if rule.Installs().String() != wantInstalls.String() {
		t.Errorf("\nwant installs:\n   %v\ngot:\n   %v", wantInstalls, rule.Installs())
	}
}

func TestDexPreoptSystemOther(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.BuilderContextForTesting(config)
	globalSoong := globalSoongConfigForTests()
	global := GlobalConfigForTests(ctx)
	systemModule := testSystemModuleConfig(ctx, "Stest")
	systemProductModule := testSystemProductModuleConfig(ctx, "SPtest")
	productModule := testProductModuleConfig(ctx, "Ptest")
	productPackages := android.PathForTesting("product_packages.txt")

	global.HasSystemOther = true

	type moduleTest struct {
		module            *ModuleConfig
		expectedPartition string
	}
	tests := []struct {
		patterns    []string
		moduleTests []moduleTest
	}{
		{
			patterns: []string{"app/%"},
			moduleTests: []moduleTest{
				{module: systemModule, expectedPartition: "system_other/system"},
				{module: systemProductModule, expectedPartition: "system/product"},
				{module: productModule, expectedPartition: "product"},
			},
		},
		// product/app/% only applies to product apps inside the system partition
		{
			patterns: []string{"app/%", "product/app/%"},
			moduleTests: []moduleTest{
				{module: systemModule, expectedPartition: "system_other/system"},
				{module: systemProductModule, expectedPartition: "system_other/system/product"},
				{module: productModule, expectedPartition: "product"},
			},
		},
	}

	for _, test := range tests {
		global.PatternsOnSystemOther = test.patterns
		for _, mt := range test.moduleTests {
			rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, mt.module, productPackages)
			if err != nil {
				t.Fatal(err)
			}

			name := mt.module.Name
			wantInstalls := android.RuleBuilderInstalls{
				{android.PathForOutput(ctx, name+"/oat/arm/package.odex"), fmt.Sprintf("/%s/app/test/oat/arm/%s.odex", mt.expectedPartition, name)},
				{android.PathForOutput(ctx, name+"/oat/arm/package.vdex"), fmt.Sprintf("/%s/app/test/oat/arm/%s.vdex", mt.expectedPartition, name)},
			}

			if rule.Installs().String() != wantInstalls.String() {
				t.Errorf("\nwant installs:\n   %v\ngot:\n   %v", wantInstalls, rule.Installs())
			}
		}
	}

}

func TestDexPreoptApexSystemServerJars(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.BuilderContextForTesting(config)
	globalSoong := globalSoongConfigForTests()
	global := GlobalConfigForTests(ctx)
	module := testApexModuleConfig(ctx, "service-A", "com.android.apex1")
	productPackages := android.PathForTesting("product_packages.txt")

	global.ApexSystemServerJars = android.CreateTestConfiguredJarList(
		[]string{"com.android.apex1:service-A"})

	rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, module, productPackages)
	if err != nil {
		t.Fatal(err)
	}

	wantInstalls := android.RuleBuilderInstalls{
		{android.PathForOutput(ctx, "service-A/dexpreopt/oat/arm/javalib.odex"), "/system/framework/oat/arm/apex@com.android.apex1@javalib@service-A.jar@classes.odex"},
		{android.PathForOutput(ctx, "service-A/dexpreopt/oat/arm/javalib.vdex"), "/system/framework/oat/arm/apex@com.android.apex1@javalib@service-A.jar@classes.vdex"},
	}

	android.AssertStringEquals(t, "installs", wantInstalls.String(), rule.Installs().String())
}

func TestDexPreoptStandaloneSystemServerJars(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.BuilderContextForTesting(config)
	globalSoong := globalSoongConfigForTests()
	global := GlobalConfigForTests(ctx)
	module := testPlatformSystemServerModuleConfig(ctx, "service-A")
	productPackages := android.PathForTesting("product_packages.txt")

	global.StandaloneSystemServerJars = android.CreateTestConfiguredJarList(
		[]string{"platform:service-A"})

	rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, module, productPackages)
	if err != nil {
		t.Fatal(err)
	}

	wantInstalls := android.RuleBuilderInstalls{
		{android.PathForOutput(ctx, "service-A/dexpreopt/oat/arm/javalib.odex"), "/system/framework/oat/arm/service-A.odex"},
		{android.PathForOutput(ctx, "service-A/dexpreopt/oat/arm/javalib.vdex"), "/system/framework/oat/arm/service-A.vdex"},
	}

	android.AssertStringEquals(t, "installs", wantInstalls.String(), rule.Installs().String())
}

func TestDexPreoptSystemExtSystemServerJars(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.BuilderContextForTesting(config)
	globalSoong := globalSoongConfigForTests()
	global := GlobalConfigForTests(ctx)
	module := testSystemExtSystemServerModuleConfig(ctx, "service-A")
	productPackages := android.PathForTesting("product_packages.txt")

	global.StandaloneSystemServerJars = android.CreateTestConfiguredJarList(
		[]string{"system_ext:service-A"})

	rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, module, productPackages)
	if err != nil {
		t.Fatal(err)
	}

	wantInstalls := android.RuleBuilderInstalls{
		{android.PathForOutput(ctx, "service-A/dexpreopt/oat/arm/javalib.odex"), "/system_ext/framework/oat/arm/service-A.odex"},
		{android.PathForOutput(ctx, "service-A/dexpreopt/oat/arm/javalib.vdex"), "/system_ext/framework/oat/arm/service-A.vdex"},
	}

	android.AssertStringEquals(t, "installs", wantInstalls.String(), rule.Installs().String())
}

func TestDexPreoptApexStandaloneSystemServerJars(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.BuilderContextForTesting(config)
	globalSoong := globalSoongConfigForTests()
	global := GlobalConfigForTests(ctx)
	module := testApexModuleConfig(ctx, "service-A", "com.android.apex1")
	productPackages := android.PathForTesting("product_packages.txt")

	global.ApexStandaloneSystemServerJars = android.CreateTestConfiguredJarList(
		[]string{"com.android.apex1:service-A"})

	rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, module, productPackages)
	if err != nil {
		t.Fatal(err)
	}

	wantInstalls := android.RuleBuilderInstalls{
		{android.PathForOutput(ctx, "service-A/dexpreopt/oat/arm/javalib.odex"), "/system/framework/oat/arm/apex@com.android.apex1@javalib@service-A.jar@classes.odex"},
		{android.PathForOutput(ctx, "service-A/dexpreopt/oat/arm/javalib.vdex"), "/system/framework/oat/arm/apex@com.android.apex1@javalib@service-A.jar@classes.vdex"},
	}

	android.AssertStringEquals(t, "installs", wantInstalls.String(), rule.Installs().String())
}

func TestDexPreoptProfile(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.BuilderContextForTesting(config)
	globalSoong := globalSoongConfigForTests()
	global := GlobalConfigForTests(ctx)
	module := testSystemModuleConfig(ctx, "test")
	productPackages := android.PathForTesting("product_packages.txt")

	module.ProfileClassListing = android.OptionalPathForPath(android.PathForTesting("profile"))

	rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, module, productPackages)
	if err != nil {
		t.Fatal(err)
	}

	wantInstalls := android.RuleBuilderInstalls{
		{android.PathForOutput(ctx, "test/profile.prof"), "/system/app/test/test.apk.prof"},
		{android.PathForOutput(ctx, "test/oat/arm/package.art"), "/system/app/test/oat/arm/test.art"},
		{android.PathForOutput(ctx, "test/oat/arm/package.odex"), "/system/app/test/oat/arm/test.odex"},
		{android.PathForOutput(ctx, "test/oat/arm/package.vdex"), "/system/app/test/oat/arm/test.vdex"},
	}

	if rule.Installs().String() != wantInstalls.String() {
		t.Errorf("\nwant installs:\n   %v\ngot:\n   %v", wantInstalls, rule.Installs())
	}
}

func TestDexPreoptConfigToJson(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.BuilderContextForTesting(config)
	module := testSystemModuleConfig(ctx, "test")
	data, err := moduleConfigToJSON(module)
	if err != nil {
		t.Errorf("Failed to convert module config data to JSON, %v", err)
	}
	parsed, err := ParseModuleConfig(ctx, data)
	if err != nil {
		t.Errorf("Failed to parse JSON, %v", err)
	}
	before := fmt.Sprintf("%v", module)
	after := fmt.Sprintf("%v", parsed)
	android.AssertStringEquals(t, "The result must be the same as the original after marshalling and unmarshalling it.", before, after)
}
