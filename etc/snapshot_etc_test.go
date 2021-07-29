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

package etc

import (
	"android/soong/android"
	"testing"

	"github.com/google/blueprint"
)

var registerSourceModule = func(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("source", newSourceModule)
}

type sourceModuleProperties struct {
	Deps []string `android:"path,arch_variant"`
}

type sourceModule struct {
	android.ModuleBase
	android.OverridableModuleBase

	properties                                     sourceModuleProperties
	dependsOnSourceModule, dependsOnPrebuiltModule bool
	deps                                           android.Paths
	src                                            android.Path
}

func newSourceModule() android.Module {
	m := &sourceModule{}
	m.AddProperties(&m.properties)
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibFirst)
	android.InitOverridableModule(m, nil)
	return m
}

func (s *sourceModule) OverridablePropertiesDepsMutator(ctx android.BottomUpMutatorContext) {
	// s.properties.Deps are annotated with android:path, so they are
	// automatically added to the dependency by pathDeps mutator
}

func (s *sourceModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	s.deps = android.PathsForModuleSrc(ctx, s.properties.Deps)
	s.src = android.PathForModuleSrc(ctx, "source_file")
}

func (s *sourceModule) Srcs() android.Paths {
	return android.Paths{s.src}
}

var prepareForSnapshotEtcTest = android.GroupFixturePreparers(
	android.PrepareForTestWithArchMutator,
	android.PrepareForTestWithPrebuilts,
	PrepareForTestWithPrebuiltEtc,
	android.FixtureRegisterWithContext(RegisterSnapshotEtcModule),
	android.FixtureRegisterWithContext(registerSourceModule),
	android.FixtureMergeMockFs(android.MockFS{
		"foo.conf": nil,
		"bar.conf": nil,
	}),
)

func TestSnapshotWithFilename(t *testing.T) {
	var androidBp = `
	snapshot_etc {
		name: "etc_module",
		src: "foo.conf",
		filename: "bar.conf",
	}
	`

	result := prepareForSnapshotEtcTest.RunTestWithBp(t, androidBp)
	for _, variant := range result.ModuleVariantsForTests("etc_module") {
		module := result.ModuleForTests("etc_module", variant)
		s, ok := module.Module().(*SnapshotEtc)
		if !ok {
			t.Errorf("Expected snapshot_etc module type")
		}
		if s.outputFilePath.Base() != "bar.conf" {
			t.Errorf("Output file path does not match with specified filename")
		}
	}
}

func TestSnapshotEtcWithOrigin(t *testing.T) {
	var androidBp = `
	prebuilt_etc {
		name: "etc_module",
		src: "foo.conf",
	}

	snapshot_etc {
		name: "etc_module",
		src: "bar.conf",
	}

	source {
		name: "source",
		deps: [":etc_module"],
	}
	`

	result := prepareForSnapshotEtcTest.RunTestWithBp(t, androidBp)

	for _, variant := range result.ModuleVariantsForTests("source") {
		source := result.ModuleForTests("source", variant)

		result.VisitDirectDeps(source.Module(), func(m blueprint.Module) {
			if _, ok := m.(*PrebuiltEtc); !ok {
				t.Errorf("Original prebuilt_etc module expected.")
			}
		})
	}
}

func TestSnapshotEtcWithOriginAndPrefer(t *testing.T) {
	var androidBp = `
	prebuilt_etc {
		name: "etc_module",
		src: "foo.conf",
	}

	snapshot_etc {
		name: "etc_module",
		src: "bar.conf",
		prefer: true,
	}

	source {
		name: "source",
		deps: [":etc_module"],
	}
	`

	result := prepareForSnapshotEtcTest.RunTestWithBp(t, androidBp)

	for _, variant := range result.ModuleVariantsForTests("source") {
		source := result.ModuleForTests("source", variant)

		result.VisitDirectDeps(source.Module(), func(m blueprint.Module) {
			if _, ok := m.(*SnapshotEtc); !ok {
				t.Errorf("Preferred snapshot_etc module expected.")
			}
		})
	}
}

func TestSnapshotEtcWithoutOrigin(t *testing.T) {
	var androidBp = `
	snapshot_etc {
		name: "etc_module",
		src: "bar.conf",
	}

	source {
		name: "source",
		deps: [":etc_module"],
	}
	`

	result := prepareForSnapshotEtcTest.RunTestWithBp(t, androidBp)

	for _, variant := range result.ModuleVariantsForTests("source") {
		source := result.ModuleForTests("source", variant)

		result.VisitDirectDeps(source.Module(), func(m blueprint.Module) {
			if _, ok := m.(*SnapshotEtc); !ok {
				t.Errorf("Only source snapshot_etc module expected.")
			}
		})
	}
}
