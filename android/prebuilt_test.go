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
	name     string
	modules  string
	prebuilt bool
}{
	{
		name: "no prebuilt",
		modules: `
			source {
				name: "bar",
			}`,
		prebuilt: false,
	},
	{
		name: "no source prebuilt not preferred",
		modules: `
			prebuilt {
				name: "bar",
				prefer: false,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: true,
	},
	{
		name: "no source prebuilt preferred",
		modules: `
			prebuilt {
				name: "bar",
				prefer: true,
				srcs: ["prebuilt_file"],
			}`,
		prebuilt: true,
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
		prebuilt: false,
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
		prebuilt: true,
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
		prebuilt: false,
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
		prebuilt: false,
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
		prebuilt: true,
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
		prebuilt: false,
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
		prebuilt: true,
	},
}

func TestPrebuilts(t *testing.T) {
	fs := map[string][]byte{
		"prebuilt_file": nil,
		"source_file":   nil,
	}

	for _, test := range prebuiltsTests {
		t.Run(test.name, func(t *testing.T) {
			bp := `
				source {
					name: "foo",
					deps: [":bar"],
				}
				` + test.modules
			config := TestConfig(buildDir, nil, bp, fs)

			ctx := NewTestContext()
			registerTestPrebuiltBuildComponents(ctx)
			ctx.RegisterModuleType("filegroup", FileGroupFactory)
			ctx.Register(config)

			_, errs := ctx.ParseBlueprintsFiles("Android.bp")
			FailIfErrored(t, errs)
			_, errs = ctx.PrepareBuildActions(config)
			FailIfErrored(t, errs)

			foo := ctx.ModuleForTests("foo", "")

			var dependsOnSourceModule, dependsOnPrebuiltModule bool
			ctx.VisitDirectDeps(foo.Module(), func(m blueprint.Module) {
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

			deps := foo.Module().(*sourceModule).deps
			if deps == nil || len(deps) != 1 {
				t.Errorf("deps does not have single path, but is %v", deps)
			}
			var usingSourceFile, usingPrebuiltFile bool
			if deps[0].String() == "source_file" {
				usingSourceFile = true
			}
			if deps[0].String() == "prebuilt_file" {
				usingPrebuiltFile = true
			}

			if test.prebuilt {
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
			} else {
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
}

func registerTestPrebuiltBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("prebuilt", newPrebuiltModule)
	ctx.RegisterModuleType("source", newSourceModule)
	ctx.RegisterModuleType("override_source", newOverrideSourceModule)

	RegisterPrebuiltMutators(ctx)
	ctx.PostDepsMutators(RegisterOverridePostDepsMutators)
}

type prebuiltModule struct {
	ModuleBase
	prebuilt   Prebuilt
	properties struct {
		Srcs []string `android:"path"`
	}
	src Path
}

func newPrebuiltModule() Module {
	m := &prebuiltModule{}
	m.AddProperties(&m.properties)
	InitPrebuiltModule(m, &m.properties.Srcs)
	InitAndroidModule(m)
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
	Deps []string `android:"path"`
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
	InitAndroidModule(m)
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

	InitAndroidModule(m)
	InitOverrideModule(m)
	return m
}
