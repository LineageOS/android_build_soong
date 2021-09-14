// Copyright 2021 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package snapshot

import (
	"path/filepath"
	"testing"

	"android/soong/android"
)

// host_snapshot and host-fake-snapshot test functions

type hostTestModule struct {
	android.ModuleBase
	props struct {
		Deps []string
	}
}

func hostTestBinOut(bin string) string {
	return filepath.Join("out", "bin", bin)
}

func (c *hostTestModule) HostToolPath() android.OptionalPath {
	return (android.OptionalPathForPath(android.PathForTesting(hostTestBinOut(c.Name()))))
}

func hostTestModuleFactory() android.Module {
	m := &hostTestModule{}
	m.AddProperties(&m.props)
	android.InitAndroidArchModule(m, android.HostSupported, android.MultilibFirst)
	return m
}
func (m *hostTestModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	builtFile := android.PathForModuleOut(ctx, m.Name())
	dir := ctx.Target().Arch.ArchType.Multilib
	installDir := android.PathForModuleInstall(ctx, dir)
	ctx.InstallFile(installDir, m.Name(), builtFile)
}

// Common blueprint used for testing
var hostTestBp = `
		license_kind {
			name: "test_notice",
			conditions: ["notice"],
		}
		license {
			name: "host_test_license",
			visibility: ["//visibility:public"],
			license_kinds: [
				"test_notice"
			],
			license_text: [
				"NOTICE",
			],
		}
		component {
			name: "foo",
			deps: ["bar"],
		}
		component {
			name: "bar",
			licenses: ["host_test_license"],
		}
		`

var hostTestModBp = `
		host_snapshot {
			name: "test-host-snapshot",
			deps: [
				"foo",
			],
		}
		`

var prepareForHostTest = android.GroupFixturePreparers(
	android.PrepareForTestWithAndroidBuildComponents,
	android.PrepareForTestWithLicenses,
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("component", hostTestModuleFactory)
	}),
)

// Prepare for host_snapshot test
var prepareForHostModTest = android.GroupFixturePreparers(
	prepareForHostTest,
	android.FixtureWithRootAndroidBp(hostTestBp+hostTestModBp),
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		registerHostBuildComponents(ctx)
	}),
)

// Prepare for fake host snapshot test disabled
var prepareForFakeHostTest = android.GroupFixturePreparers(
	prepareForHostTest,
	android.FixtureWithRootAndroidBp(hostTestBp),
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		registerHostSnapshotComponents(ctx)
	}),
)

// Prepare for fake host snapshot test enabled
var prepareForFakeHostTestEnabled = android.GroupFixturePreparers(
	prepareForFakeHostTest,
	android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.HostFakeSnapshotEnabled = true
	}),
)

// Validate that a hostSnapshot object is created containing zip files and JSON file
// content of zip file is not validated as this is done by PackagingSpecs
func TestHostSnapshot(t *testing.T) {
	result := prepareForHostModTest.RunTest(t)
	t.Helper()
	ctx := result.TestContext.ModuleForTests("test-host-snapshot", result.Config.BuildOS.String()+"_common")
	mod := ctx.Module().(*hostSnapshot)
	if ctx.MaybeOutput("host_snapshot.json").Rule == nil {
		t.Error("Manifest file not found")
	}
	zips := []string{"_deps.zip", "_mods.zip", ".zip"}

	for _, zip := range zips {
		zFile := mod.Name() + zip
		if ctx.MaybeOutput(zFile).Rule == nil {
			t.Error("Zip file ", zFile, "not found")
		}

	}
}

// Validate fake host snapshot contains binary modules as well as the JSON meta file
func TestFakeHostSnapshotEnable(t *testing.T) {
	result := prepareForFakeHostTestEnabled.RunTest(t)
	t.Helper()
	bins := []string{"foo", "bar"}
	ctx := result.TestContext.SingletonForTests("host-fake-snapshot")
	if ctx.MaybeOutput(filepath.Join("host-fake-snapshot", "host_snapshot.json")).Rule == nil {
		t.Error("Manifest file not found")
	}
	for _, bin := range bins {
		if ctx.MaybeOutput(filepath.Join("host-fake-snapshot", hostTestBinOut(bin))).Rule == nil {
			t.Error("Binary file ", bin, "not found")
		}

	}
}

// Validate not fake host snapshot if HostFakeSnapshotEnabled has not been set to true
func TestFakeHostSnapshotDisable(t *testing.T) {
	result := prepareForFakeHostTest.RunTest(t)
	t.Helper()
	ctx := result.TestContext.SingletonForTests("host-fake-snapshot")
	if len(ctx.AllOutputs()) != 0 {
		t.Error("Fake host snapshot not empty when disabled")
	}

}
