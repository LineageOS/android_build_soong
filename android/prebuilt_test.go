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

func TestPrebuilts(t *testing.T) {
	buildOS := TestArchConfig(t.TempDir(), nil, "", nil).BuildOS

	var prebuiltsTests = []struct {
		name      string
		replaceBp bool // modules is added to default bp boilerplate if false.
		modules   string
		prebuilt  []OsType
		preparer  FixturePreparer
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
			prebuilt: []OsType{Android, buildOS},
		},
		{
			name: "no source prebuilt preferred",
			modules: `
				prebuilt {
					name: "bar",
					prefer: true,
					srcs: ["prebuilt_file"],
				}`,
			prebuilt: []OsType{Android, buildOS},
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
			prebuilt: []OsType{Android, buildOS},
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
			prebuilt: []OsType{Android, buildOS},
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
			prebuilt: []OsType{buildOS},
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
			prebuilt: []OsType{Android, buildOS},
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
			prebuilt: []OsType{Android, buildOS, Windows},
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
			prebuilt: []OsType{Android, buildOS},
		},
		{
			name:      "prebuilt properties customizable",
			replaceBp: true,
			modules: `
				source {
					name: "foo",
					deps: [":bar"],
				}

				soong_config_module_type {
					name: "prebuilt_with_config",
					module_type: "prebuilt",
					config_namespace: "any_namespace",
					bool_variables: ["bool_var"],
					properties: ["prefer"],
				}

				prebuilt_with_config {
					name: "bar",
					prefer: true,
					srcs: ["prebuilt_file"],
					soong_config_variables: {
						bool_var: {
							prefer: false,
							conditions_default: {
								prefer: true,
							},
						},
					},
				}`,
			prebuilt: []OsType{Android, buildOS},
		},
		{
			name: "prebuilt use_source_config_var={acme, use_source} - no var specified",
			modules: `
				source {
					name: "bar",
				}

				prebuilt {
					name: "bar",
					use_source_config_var: {config_namespace: "acme", var_name: "use_source"},
					srcs: ["prebuilt_file"],
				}`,
			// When use_source_env is specified then it will use the prebuilt by default if the environment
			// variable is not set.
			prebuilt: []OsType{Android, buildOS},
		},
		{
			name: "prebuilt use_source_config_var={acme, use_source} - acme_use_source=false",
			modules: `
				source {
					name: "bar",
				}

				prebuilt {
					name: "bar",
					use_source_config_var: {config_namespace: "acme", var_name: "use_source"},
					srcs: ["prebuilt_file"],
				}`,
			preparer: FixtureModifyProductVariables(func(variables FixtureProductVariables) {
				variables.VendorVars = map[string]map[string]string{
					"acme": {
						"use_source": "false",
					},
				}
			}),
			// Setting the environment variable named in use_source_env to false will cause the prebuilt to
			// be used.
			prebuilt: []OsType{Android, buildOS},
		},
		{
			name: "apex_contributions supersedes any source preferred via use_source_config_var",
			modules: `
				source {
					name: "bar",
				}

				prebuilt {
					name: "bar",
					use_source_config_var: {config_namespace: "acme", var_name: "use_source"},
					srcs: ["prebuilt_file"],
				}
				apex_contributions {
					name: "my_mainline_module_contribution",
					api_domain: "apexfoo",
					// this metadata module contains prebuilt
					contents: ["prebuilt_bar"],
				}
				all_apex_contributions {
					name: "all_apex_contributions",
				}
				`,
			preparer: FixtureModifyProductVariables(func(variables FixtureProductVariables) {
				variables.VendorVars = map[string]map[string]string{
					"acme": {
						"use_source": "true",
					},
				}
				variables.BuildFlags = map[string]string{
					"RELEASE_APEX_CONTRIBUTIONS_ADSERVICES": "my_mainline_module_contribution",
				}
			}),
			// use_source_config_var indicates that source should be used
			// but this is superseded by `my_mainline_module_contribution`
			prebuilt: []OsType{Android, buildOS},
		},
		{
			name: "apex_contributions supersedes any prebuilt preferred via use_source_config_var",
			modules: `
				source {
					name: "bar",
				}

				prebuilt {
					name: "bar",
					use_source_config_var: {config_namespace: "acme", var_name: "use_source"},
					srcs: ["prebuilt_file"],
				}
				apex_contributions {
					name: "my_mainline_module_contribution",
					api_domain: "apexfoo",
					// this metadata module contains source
					contents: ["bar"],
				}
				all_apex_contributions {
					name: "all_apex_contributions",
				}
				`,
			preparer: FixtureModifyProductVariables(func(variables FixtureProductVariables) {
				variables.VendorVars = map[string]map[string]string{
					"acme": {
						"use_source": "false",
					},
				}
				variables.BuildFlags = map[string]string{
					"RELEASE_APEX_CONTRIBUTIONS_ADSERVICES": "my_mainline_module_contribution",
				}
			}),
			// use_source_config_var indicates that prebuilt should be used
			// but this is superseded by `my_mainline_module_contribution`
			prebuilt: nil,
		},
		{
			name: "prebuilt use_source_config_var={acme, use_source} - acme_use_source=true",
			modules: `
				source {
					name: "bar",
				}

				prebuilt {
					name: "bar",
					use_source_config_var: {config_namespace: "acme", var_name: "use_source"},
					srcs: ["prebuilt_file"],
				}`,
			preparer: FixtureModifyProductVariables(func(variables FixtureProductVariables) {
				variables.VendorVars = map[string]map[string]string{
					"acme": {
						"use_source": "true",
					},
				}
			}),
			// Setting the environment variable named in use_source_env to true will cause the source to be
			// used.
			prebuilt: nil,
		},
		{
			name: "prebuilt use_source_config_var={acme, use_source} - acme_use_source=true, no source",
			modules: `
				prebuilt {
					name: "bar",
					use_source_config_var: {config_namespace: "acme", var_name: "use_source"},
					srcs: ["prebuilt_file"],
				}`,
			preparer: FixtureModifyProductVariables(func(variables FixtureProductVariables) {
				variables.VendorVars = map[string]map[string]string{
					"acme": {
						"use_source": "true",
					},
				}
			}),
			// Although the environment variable says to use source there is no source available.
			prebuilt: []OsType{Android, buildOS},
		},
	}

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

			result := GroupFixturePreparers(
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
				OptionalFixturePreparer(test.preparer),
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

func testPrebuiltErrorWithFixture(t *testing.T, expectedError, bp string, fixture FixturePreparer) {
	t.Helper()
	fs := MockFS{
		"prebuilt_file": nil,
	}
	GroupFixturePreparers(
		PrepareForTestWithArchMutator,
		PrepareForTestWithPrebuilts,
		PrepareForTestWithOverrides,
		fs.AddToFixture(),
		FixtureRegisterWithContext(registerTestPrebuiltModules),
		OptionalFixturePreparer(fixture),
	).
		ExtendWithErrorHandler(FixtureExpectsAtLeastOneErrorMatchingPattern(expectedError)).
		RunTestWithBp(t, bp)

}

func testPrebuiltError(t *testing.T, expectedError, bp string) {
	testPrebuiltErrorWithFixture(t, expectedError, bp, nil)
}

func TestPrebuiltShouldNotChangePartition(t *testing.T) {
	testPrebuiltError(t, `partition is different`, `
		source {
			name: "foo",
			vendor: true,
		}
		prebuilt {
			name: "foo",
			prefer: true,
			srcs: ["prebuilt_file"],
		}`)
}

func TestPrebuiltShouldNotChangePartition_WithOverride(t *testing.T) {
	testPrebuiltError(t, `partition is different`, `
		source {
			name: "foo",
			vendor: true,
		}
		override_source {
			name: "bar",
			base: "foo",
		}
		prebuilt {
			name: "bar",
			prefer: true,
			srcs: ["prebuilt_file"],
		}`)
}

func registerTestPrebuiltBuildComponents(ctx RegistrationContext) {
	registerTestPrebuiltModules(ctx)

	RegisterPrebuiltMutators(ctx)
	ctx.PostDepsMutators(RegisterOverridePostDepsMutators)
}

var prepareForTestWithFakePrebuiltModules = FixtureRegisterWithContext(registerTestPrebuiltModules)

func registerTestPrebuiltModules(ctx RegistrationContext) {
	ctx.RegisterModuleType("prebuilt", newPrebuiltModule)
	ctx.RegisterModuleType("source", newSourceModule)
	ctx.RegisterModuleType("override_source", newOverrideSourceModule)
	ctx.RegisterModuleType("soong_config_module_type", SoongConfigModuleTypeFactory)
	ctx.RegisterModuleType("soong_config_string_variable", SoongConfigStringVariableDummyFactory)
	ctx.RegisterModuleType("soong_config_bool_variable", SoongConfigBoolVariableDummyFactory)
	RegisterApexContributionsBuildComponents(ctx)
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

func TestPrebuiltErrorCannotListBothSourceAndPrebuiltInContributions(t *testing.T) {
	selectMainlineModuleContritbutions := GroupFixturePreparers(
		FixtureModifyProductVariables(func(variables FixtureProductVariables) {
			variables.BuildFlags = map[string]string{
				"RELEASE_APEX_CONTRIBUTIONS_ADSERVICES": "my_apex_contributions",
			}
		}),
	)
	testPrebuiltErrorWithFixture(t, `Cannot use Soong module: prebuilt_foo from apex_contributions: my_apex_contributions because it has been added previously as: foo from apex_contributions: my_apex_contributions`, `
		source {
			name: "foo",
		}
		prebuilt {
			name: "foo",
			srcs: ["prebuilt_file"],
		}
		apex_contributions {
			name: "my_apex_contributions",
			api_domain: "my_mainline_module",
			contents: [
			  "foo",
			  "prebuilt_foo",
			],
		}
		all_apex_contributions {
			name: "all_apex_contributions",
		}
		`, selectMainlineModuleContritbutions)
}
