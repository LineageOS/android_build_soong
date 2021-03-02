// Copyright 2016 Google Inc. All rights reserved.
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

package android

import (
	"fmt"
	"testing"

	"github.com/google/blueprint"
)

var prebuiltsTests = []struct {
	name      string
	replaceBp bool // modules is added to default bp boilerplate if false.
	modules   string
	prebuilt  []OsType
}{
	{
		name: "no prebuilt",
		modules: `
			source {
				name: "bar",
			}`,
		prebuilt: nil,
	},
	{
		name: "no source prebuilt not preferred",
		modules: `
			prebuilt {
				name: "bar",
				prefer: false,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: []OsType{Android, BuildOs},
	},
	{
		name: "no source prebuilt preferred",
		modules: `
			prebuilt {
				name: "bar",
				prefer: true,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: []OsType{Android, BuildOs},
	},
	{
		name: "prebuilt not preferred",
		modules: `
			source {
				name: "bar",
			}

			prebuilt {
				name: "bar",
				prefer: false,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: nil,
	},
	{
		name: "prebuilt preferred",
		modules: `
			source {
				name: "bar",
			}

			prebuilt {
				name: "bar",
				prefer: true,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: []OsType{Android, BuildOs},
	},
	{
		name: "prebuilt no file not preferred",
		modules: `
			source {
				name: "bar",
			}

			prebuilt {
				name: "bar",
				prefer: false,
			}`,
		prebuilt: nil,
	},
	{
		name: "prebuilt no file preferred",
		modules: `
			source {
				name: "bar",
			}

			prebuilt {
				name: "bar",
				prefer: true,
			}`,
		prebuilt: nil,
	},
	{
		name: "prebuilt file from filegroup preferred",
		modules: `
			filegroup {
				name: "fg",
				srcs: ["prebuilt_file"],
			}
			prebuilt {
				name: "bar",
				prefer: true,
				srcs: [":fg"],
			}`,
		prebuilt: []OsType{Android, BuildOs},
	},
	{
		name: "prebuilt module for device only",
		modules: `
			source {
				name: "bar",
			}

			prebuilt {
				name: "bar",
				host_supported: false,
				prefer: true,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: []OsType{Android},
	},
	{
		name: "prebuilt file for host only",
		modules: `
			source {
				name: "bar",
			}

			prebuilt {
				name: "bar",
				prefer: true,
				target: {
					host: {
						srcs: ["prebuilt_file"],
					},
				},
			}`,
		prebuilt: []OsType{BuildOs},
	},
	{
		name: "prebuilt override not preferred",
		modules: `
			source {
				name: "baz",
			}

			override_source {
				name: "bar",
				base: "baz",
			}

			prebuilt {
				name: "bar",
				prefer: false,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: nil,
	},
	{
		name: "prebuilt override preferred",
		modules: `
			source {
				name: "baz",
			}

			override_source {
				name: "bar",
				base: "baz",
			}

			prebuilt {
				name: "bar",
				prefer: true,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: []OsType{Android, BuildOs},
	},
	{
		name:      "prebuilt including default-disabled OS",
		replaceBp: true,
		modules: `
			source {
				name: "foo",
				deps: [":bar"],
				target: {
					windows: {
						enabled: true,
					},
				},
			}

			source {
				name: "bar",
				target: {
					windows: {
						enabled: true,
					},
				},
			}

			prebuilt {
				name: "bar",
				prefer: true,
				srcs: ["prebuilt_file"],
				target: {
					windows: {
						enabled: true,
					},
				},
			}`,
		prebuilt: []OsType{Android, BuildOs, Windows},
	},
	{
		name:      "fall back to source for default-disabled OS",
		replaceBp: true,
		modules: `
			source {
				name: "foo",
				deps: [":bar"],
				target: {
					windows: {
						enabled: true,
					},
				},
			}

			source {
				name: "bar",
				target: {
					windows: {
						enabled: true,
					},
				},
			}

			prebuilt {
				name: "bar",
				prefer: true,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: []OsType{Android, BuildOs},
	},
}

func TestPrebuilts(t *testing.T) {
	fs := MockFS{
		"prebuilt_file": nil,
		"source_file":   nil,
	}

	for _, test := range prebuiltsTests {
		t.Run(test.name, func(t *testing.T) {
			bp := test.modules
			if !test.replaceBp {
				bp = bp + `
					source {
						name: "foo",
						deps: [":bar"],
					}`
			}

			// Add windows to the target list to test the logic when a variant is
			// disabled by default.
			if !Windows.DefaultDisabled {
				t.Errorf("windows is assumed to be disabled by default")
			}

			result := emptyTestFixtureFactory.Extend(
				PrepareForTestWithArchMutator,
				PrepareForTestWithPrebuilts,
				PrepareForTestWithOverrides,
				PrepareForTestWithFilegroup,
				// Add a Windows target to the configuration.
				FixtureModifyConfig(func(config Config) {
					config.Targets[Windows] = []Target{
						{Windows, Arch{ArchType: X86_64}, NativeBridgeDisabled, "", "", true},
					}
				}),
				fs.AddToFixture(),
				FixtureRegisterWithContext(registerTestPrebuiltModules),
			).RunTestWithBp(t, bp)

			for _, variant := range result.ModuleVariantsForTests("foo") {
				foo := result.ModuleForTests("foo", variant)
				t.Run(foo.Module().Target().Os.String(), func(t *testing.T) {
					var dependsOnSourceModule, dependsOnPrebuiltModule bool
					result.VisitDirectDeps(foo.Module(), func(m blueprint.Module) {
						if _, ok := m.(*sourceModule); ok {
							dependsOnSourceModule = true
						}
						if p, ok := m.(*prebuiltModule); ok {
							dependsOnPrebuiltModule = true
							if !p.Prebuilt().properties.UsePrebuilt {
								t.Errorf("dependency on prebuilt module not marked used")
							}
						}
					})

					moduleIsDisabled := !foo.Module().Enabled()
					deps := foo.Module().(*sourceModule).deps
					if moduleIsDisabled {
						if len(deps) > 0 {
							t.Errorf("disabled module got deps: %v", deps)
						}
					} else {
						if len(deps) != 1 {
							t.Errorf("deps does not have single path, but is %v", deps)
						}
					}

					var usingSourceFile, usingPrebuiltFile bool
					if len(deps) > 0 && deps[0].String() == "source_file" {
						usingSourceFile = true
					}
					if len(deps) > 0 && deps[0].String() == "prebuilt_file" {
						usingPrebuiltFile = true
					}

					prebuilt := false
					for _, os := range test.prebuilt {
						if os == foo.Module().Target().Os {
							prebuilt = true
						}
					}

					if prebuilt {
						if moduleIsDisabled {
							t.Errorf("dependent module for prebuilt is disabled")
						}

						if !dependsOnPrebuiltModule {
							t.Errorf("doesn't depend on prebuilt module")
						}
						if !usingPrebuiltFile {
							t.Errorf("doesn't use prebuilt_file")
						}

						if dependsOnSourceModule {
							t.Errorf("depends on source module")
						}
						if usingSourceFile {
							t.Errorf("using source_file")
						}
					} else if !moduleIsDisabled {
						if dependsOnPrebuiltModule {
							t.Errorf("depends on prebuilt module")
						}
						if usingPrebuiltFile {
							t.Errorf("using prebuilt_file")
						}

						if !dependsOnSourceModule {
							t.Errorf("doesn't depend on source module")
						}
						if !usingSourceFile {
							t.Errorf("doesn't use source_file")
						}
					}
				})
			}
		})
	}
}

func registerTestPrebuiltBuildComponents(ctx RegistrationContext) {
	registerTestPrebuiltModules(ctx)

	RegisterPrebuiltMutators(ctx)
	ctx.PostDepsMutators(RegisterOverridePostDepsMutators)
}

func registerTestPrebuiltModules(ctx RegistrationContext) {
	ctx.RegisterModuleType("prebuilt", newPrebuiltModule)
	ctx.RegisterModuleType("source", newSourceModule)
	ctx.RegisterModuleType("override_source", newOverrideSourceModule)
}

type prebuiltModule struct {
	ModuleBase
	prebuilt   Prebuilt
	properties struct {
		Srcs []string `android:"path,arch_variant"`
	}
	src Path
}

func newPrebuiltModule() Module {
	m := &prebuiltModule{}
	m.AddProperties(&m.properties)
	InitPrebuiltModule(m, &m.properties.Srcs)
	InitAndroidArchModule(m, HostAndDeviceDefault, MultilibCommon)
	return m
}

func (p *prebuiltModule) Name() string {
	return p.prebuilt.Name(p.ModuleBase.Name())
}

func (p *prebuiltModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	if len(p.properties.Srcs) >= 1 {
		p.src = p.prebuilt.SingleSourcePath(ctx)
	}
}

func (p *prebuiltModule) Prebuilt() *Prebuilt {
	return &p.prebuilt
}

func (p *prebuiltModule) OutputFiles(tag string) (Paths, error) {
	switch tag {
	case "":
		return Paths{p.src}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

type sourceModuleProperties struct {
	Deps []string `android:"path,arch_variant"`
}

type sourceModule struct {
	ModuleBase
	OverridableModuleBase

	properties                                     sourceModuleProperties
	dependsOnSourceModule, dependsOnPrebuiltModule bool
	deps                                           Paths
	src                                            Path
}

func newSourceModule() Module {
	m := &sourceModule{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, HostAndDeviceDefault, MultilibCommon)
	InitOverridableModule(m, nil)
	return m
}

func (s *sourceModule) OverridablePropertiesDepsMutator(ctx BottomUpMutatorContext) {
	// s.properties.Deps are annotated with android:path, so they are
	// automatically added to the dependency by pathDeps mutator
}

func (s *sourceModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	s.deps = PathsForModuleSrc(ctx, s.properties.Deps)
	s.src = PathForModuleSrc(ctx, "source_file")
}

func (s *sourceModule) Srcs() Paths {
	return Paths{s.src}
}

type overrideSourceModule struct {
	ModuleBase
	OverrideModuleBase
}

func (o *overrideSourceModule) GenerateAndroidBuildActions(_ ModuleContext) {
}

func newOverrideSourceModule() Module {
	m := &overrideSourceModule{}
	m.AddProperties(&sourceModuleProperties{})

	InitAndroidArchModule(m, HostAndDeviceDefault, MultilibCommon)
	InitOverrideModule(m)
	return m
}
