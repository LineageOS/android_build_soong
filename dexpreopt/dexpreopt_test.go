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
	return &ModuleConfig{
		Name:                            name,
		DexLocation:                     fmt.Sprintf("/%s/app/test/%s.apk", partition, name),
		BuildPath:                       android.PathForOutput(ctx, fmt.Sprintf("%s/%s.apk", name, name)),
		DexPath:                         android.PathForOutput(ctx, fmt.Sprintf("%s/dex/%s.jar", name, name)),
		UncompressedDex:                 false,
		HasApkLibraries:                 false,
		PreoptFlags:                     nil,
		ProfileClassListing:             android.OptionalPath{},
		ProfileIsTextListing:            false,
		EnforceUsesLibraries:            false,
		PresentOptionalUsesLibraries:    nil,
		UsesLibraries:                   nil,
		LibraryPaths:                    nil,
		Archs:                           []android.ArchType{android.Arm},
		DexPreoptImages:                 android.Paths{android.PathForTesting("system/framework/arm/boot.art")},
		DexPreoptImagesDeps:             []android.OutputPaths{android.OutputPaths{}},
		DexPreoptImageLocations:         []string{},
		PreoptBootClassPathDexFiles:     nil,
		PreoptBootClassPathDexLocations: nil,
		PreoptExtractedApk:              false,
		NoCreateAppImage:                false,
		ForceCreateAppImage:             false,
		PresignedPrebuilt:               false,
	}
}

func TestDexPreopt(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.PathContextForTesting(config)
	globalSoong := GlobalSoongConfigForTests(config)
	global := GlobalConfigForTests(ctx)
	module := testSystemModuleConfig(ctx, "test")

	rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, module)
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
	ctx := android.PathContextForTesting(config)
	globalSoong := GlobalSoongConfigForTests(config)
	global := GlobalConfigForTests(ctx)
	systemModule := testSystemModuleConfig(ctx, "Stest")
	systemProductModule := testSystemProductModuleConfig(ctx, "SPtest")
	productModule := testProductModuleConfig(ctx, "Ptest")

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
			rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, mt.module)
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

func TestDexPreoptProfile(t *testing.T) {
	config := android.TestConfig("out", nil, "", nil)
	ctx := android.PathContextForTesting(config)
	globalSoong := GlobalSoongConfigForTests(config)
	global := GlobalConfigForTests(ctx)
	module := testSystemModuleConfig(ctx, "test")

	module.ProfileClassListing = android.OptionalPathForPath(android.PathForTesting("profile"))

	rule, err := GenerateDexpreoptRule(ctx, globalSoong, global, module)
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
