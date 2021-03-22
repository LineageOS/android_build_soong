// Copyright 2020 Google Inc. All rights reserved.
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
	"testing"

	"github.com/google/blueprint"
)

type testInstallDependencyTagModule struct {
	ModuleBase
	Properties struct {
		Install_deps []string
		Deps         []string
	}
}

func (t *testInstallDependencyTagModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	outputFile := PathForModuleOut(ctx, "out")
	ctx.Build(pctx, BuildParams{
		Rule:   Touch,
		Output: outputFile,
	})
	ctx.InstallFile(PathForModuleInstall(ctx), ctx.ModuleName(), outputFile)
}

var testInstallDependencyTagAlwaysDepTag = struct {
	blueprint.DependencyTag
	InstallAlwaysNeededDependencyTag
}{}

var testInstallDependencyTagNeverDepTag = struct {
	blueprint.DependencyTag
}{}

func (t *testInstallDependencyTagModule) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddVariationDependencies(nil, testInstallDependencyTagAlwaysDepTag, t.Properties.Install_deps...)
	ctx.AddVariationDependencies(nil, testInstallDependencyTagNeverDepTag, t.Properties.Deps...)
}

func testInstallDependencyTagModuleFactory() Module {
	module := &testInstallDependencyTagModule{}
	InitAndroidArchModule(module, HostAndDeviceDefault, MultilibCommon)
	module.AddProperties(&module.Properties)
	return module
}

func TestInstallDependencyTag(t *testing.T) {
	bp := `
		test_module {
			name: "foo",
			deps: ["dep"],
			install_deps: ["install_dep"],
		}

		test_module {
			name: "install_dep",
			install_deps: ["transitive"],
		}

		test_module {
			name: "transitive",
		}

		test_module {
			name: "dep",
		}
	`

	result := GroupFixturePreparers(
		PrepareForTestWithArchMutator,
		FixtureWithRootAndroidBp(bp),
		FixtureRegisterWithContext(func(ctx RegistrationContext) {
			ctx.RegisterModuleType("test_module", testInstallDependencyTagModuleFactory)
		}),
	).RunTest(t)

	config := result.Config

	hostFoo := result.ModuleForTests("foo", config.BuildOSCommonTarget.String()).Description("install")
	hostInstallDep := result.ModuleForTests("install_dep", config.BuildOSCommonTarget.String()).Description("install")
	hostTransitive := result.ModuleForTests("transitive", config.BuildOSCommonTarget.String()).Description("install")
	hostDep := result.ModuleForTests("dep", config.BuildOSCommonTarget.String()).Description("install")

	if g, w := hostFoo.Implicits.Strings(), hostInstallDep.Output.String(); !InList(w, g) {
		t.Errorf("expected host dependency %q, got %q", w, g)
	}

	if g, w := hostFoo.Implicits.Strings(), hostTransitive.Output.String(); !InList(w, g) {
		t.Errorf("expected host dependency %q, got %q", w, g)
	}

	if g, w := hostInstallDep.Implicits.Strings(), hostTransitive.Output.String(); !InList(w, g) {
		t.Errorf("expected host dependency %q, got %q", w, g)
	}

	if g, w := hostFoo.Implicits.Strings(), hostDep.Output.String(); InList(w, g) {
		t.Errorf("expected no host dependency %q, got %q", w, g)
	}

	deviceFoo := result.ModuleForTests("foo", "android_common").Description("install")
	deviceInstallDep := result.ModuleForTests("install_dep", "android_common").Description("install")
	deviceTransitive := result.ModuleForTests("transitive", "android_common").Description("install")
	deviceDep := result.ModuleForTests("dep", "android_common").Description("install")

	if g, w := deviceFoo.OrderOnly.Strings(), deviceInstallDep.Output.String(); !InList(w, g) {
		t.Errorf("expected device dependency %q, got %q", w, g)
	}

	if g, w := deviceFoo.OrderOnly.Strings(), deviceTransitive.Output.String(); !InList(w, g) {
		t.Errorf("expected device dependency %q, got %q", w, g)
	}

	if g, w := deviceInstallDep.OrderOnly.Strings(), deviceTransitive.Output.String(); !InList(w, g) {
		t.Errorf("expected device dependency %q, got %q", w, g)
	}

	if g, w := deviceFoo.OrderOnly.Strings(), deviceDep.Output.String(); InList(w, g) {
		t.Errorf("expected no device dependency %q, got %q", w, g)
	}
}
