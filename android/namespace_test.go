// Copyright 2017 Google Inc. All rights reserved.
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
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/blueprint"
)

func TestDependingOnModuleInSameNamespace(t *testing.T) {
	result := GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
				}
				test_module {
					name: "b",
					deps: ["a"],
				}
			`,
		}),
	).RunTest(t)

	a := getModule(result, "a")
	b := getModule(result, "b")
	if !dependsOn(result, b, a) {
		t.Errorf("module b does not depend on module a in the same namespace")
	}
}

func TestDependingOnModuleInRootNamespace(t *testing.T) {
	result := GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			".": `
				test_module {
					name: "b",
					deps: ["a"],
				}
				test_module {
					name: "a",
				}
			`,
		}),
	).RunTest(t)

	a := getModule(result, "a")
	b := getModule(result, "b")
	if !dependsOn(result, b, a) {
		t.Errorf("module b in root namespace does not depend on module a in the root namespace")
	}
}

func TestImplicitlyImportRootNamespace(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			".": `
				test_module {
					name: "a",
				}
			`,
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "b",
					deps: ["a"],
				}
			`,
		}),
	).RunTest(t)

	// RunTest will report any errors
}

func TestDependingOnBlueprintModuleInRootNamespace(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			".": `
				blueprint_test_module {
					name: "a",
				}
			`,
			"dir1": `
				soong_namespace {
				}
				blueprint_test_module {
					name: "b",
					deps: ["a"],
				}
			`,
		}),
	).RunTest(t)

	// RunTest will report any errors
}

func TestDependingOnModuleInImportedNamespace(t *testing.T) {
	result := GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
				}
			`,
			"dir2": `
				soong_namespace {
					imports: ["dir1"],
				}
				test_module {
					name: "b",
					deps: ["a"],
				}
			`,
		}),
	).RunTest(t)

	a := getModule(result, "a")
	b := getModule(result, "b")
	if !dependsOn(result, b, a) {
		t.Errorf("module b does not depend on module a in the same namespace")
	}
}

func TestDependingOnModuleInNonImportedNamespace(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
				}
			`,
			"dir2": `
				soong_namespace {
				}
				test_module {
					name: "a",
				}
			`,
			"dir3": `
				soong_namespace {
				}
				test_module {
					name: "b",
					deps: ["a"],
				}
			`,
		}),
	).
		ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(`\Qdir3/Android.bp:4:5: "b" depends on undefined module "a"
Module "b" is defined in namespace "dir3" which can read these 2 namespaces: ["dir3" "."]
Module "a" can be found in these namespaces: ["dir1" "dir2"]\E`)).
		RunTest(t)
}

func TestDependingOnModuleByFullyQualifiedReference(t *testing.T) {
	result := GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
				}
			`,
			"dir2": `
				soong_namespace {
				}
				test_module {
					name: "b",
					deps: ["//dir1:a"],
				}
			`,
		}),
	).RunTest(t)

	a := getModule(result, "a")
	b := getModule(result, "b")
	if !dependsOn(result, b, a) {
		t.Errorf("module b does not depend on module a")
	}
}

func TestSameNameInTwoNamespaces(t *testing.T) {
	result := GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
					id: "1",
				}
				test_module {
					name: "b",
					deps: ["a"],
					id: "2",
				}
			`,
			"dir2": `
				soong_namespace {
				}
				test_module {
					name: "a",
					id:"3",
				}
				test_module {
					name: "b",
					deps: ["a"],
					id:"4",
				}
			`,
		}),
	).RunTest(t)

	one := findModuleById(result, "1")
	two := findModuleById(result, "2")
	three := findModuleById(result, "3")
	four := findModuleById(result, "4")
	if !dependsOn(result, two, one) {
		t.Fatalf("Module 2 does not depend on module 1 in its namespace")
	}
	if dependsOn(result, two, three) {
		t.Fatalf("Module 2 depends on module 3 in another namespace")
	}
	if !dependsOn(result, four, three) {
		t.Fatalf("Module 4 does not depend on module 3 in its namespace")
	}
	if dependsOn(result, four, one) {
		t.Fatalf("Module 4 depends on module 1 in another namespace")
	}
}

func TestSearchOrder(t *testing.T) {
	result := GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
					id: "1",
				}
			`,
			"dir2": `
				soong_namespace {
				}
				test_module {
					name: "a",
					id:"2",
				}
				test_module {
					name: "b",
					id:"3",
				}
			`,
			"dir3": `
				soong_namespace {
				}
				test_module {
					name: "a",
					id:"4",
				}
				test_module {
					name: "b",
					id:"5",
				}
				test_module {
					name: "c",
					id:"6",
				}
			`,
			".": `
				test_module {
					name: "a",
					id: "7",
				}
				test_module {
					name: "b",
					id: "8",
				}
				test_module {
					name: "c",
					id: "9",
				}
				test_module {
					name: "d",
					id: "10",
				}
			`,
			"dir4": `
				soong_namespace {
					imports: ["dir1", "dir2", "dir3"]
				}
				test_module {
					name: "test_me",
					id:"0",
					deps: ["a", "b", "c", "d"],
				}
			`,
		}),
	).RunTest(t)

	testMe := findModuleById(result, "0")
	if !dependsOn(result, testMe, findModuleById(result, "1")) {
		t.Errorf("test_me doesn't depend on id 1")
	}
	if !dependsOn(result, testMe, findModuleById(result, "3")) {
		t.Errorf("test_me doesn't depend on id 3")
	}
	if !dependsOn(result, testMe, findModuleById(result, "6")) {
		t.Errorf("test_me doesn't depend on id 6")
	}
	if !dependsOn(result, testMe, findModuleById(result, "10")) {
		t.Errorf("test_me doesn't depend on id 10")
	}
	if numDeps(result, testMe) != 4 {
		t.Errorf("num dependencies of test_me = %v, not 4\n", numDeps(result, testMe))
	}
}

func TestTwoNamespacesCanImportEachOther(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
					imports: ["dir2"]
				}
				test_module {
					name: "a",
				}
				test_module {
					name: "c",
					deps: ["b"],
				}
			`,
			"dir2": `
				soong_namespace {
					imports: ["dir1"],
				}
				test_module {
					name: "b",
					deps: ["a"],
				}
			`,
		}),
	).RunTest(t)

	// RunTest will report any errors
}

func TestImportingNonexistentNamespace(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
					imports: ["a_nonexistent_namespace"]
				}
				test_module {
					name: "a",
					deps: ["a_nonexistent_module"]
				}
			`,
		}),
	).
		// should complain about the missing namespace and not complain about the unresolvable dependency
		ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(`\Qdir1/Android.bp:2:5: module "soong_namespace": namespace a_nonexistent_namespace does not exist\E`)).
		RunTest(t)
}

func TestNamespacesDontInheritParentNamespaces(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
				}
			`,
			"dir1/subdir1": `
				soong_namespace {
				}
				test_module {
					name: "b",
					deps: ["a"],
				}
			`,
		}),
	).
		ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(`\Qdir1/subdir1/Android.bp:4:5: "b" depends on undefined module "a"
Module "b" is defined in namespace "dir1/subdir1" which can read these 2 namespaces: ["dir1/subdir1" "."]
Module "a" can be found in these namespaces: ["dir1"]\E`)).
		RunTest(t)
}

func TestModulesDoReceiveParentNamespace(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
				}
			`,
			"dir1/subdir": `
				test_module {
					name: "b",
					deps: ["a"],
				}
			`,
		}),
	).RunTest(t)

	// RunTest will report any errors
}

func TestNamespaceImportsNotTransitive(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
				}
			`,
			"dir2": `
				soong_namespace {
					imports: ["dir1"],
				}
				test_module {
					name: "b",
					deps: ["a"],
				}
			`,
			"dir3": `
				soong_namespace {
					imports: ["dir2"],
				}
				test_module {
					name: "c",
					deps: ["a"],
				}
			`,
		}),
	).
		ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(`\Qdir3/Android.bp:5:5: "c" depends on undefined module "a"
Module "c" is defined in namespace "dir3" which can read these 3 namespaces: ["dir3" "dir2" "."]
Module "a" can be found in these namespaces: ["dir1"]\E`)).
		RunTest(t)
}

func TestTwoNamepacesInSameDir(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				soong_namespace {
				}
			`,
		}),
	).
		ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(`\Qdir1/Android.bp:4:5: namespace dir1 already exists\E`)).
		RunTest(t)
}

func TestNamespaceNotAtTopOfFile(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				test_module {
					name: "a"
				}
				soong_namespace {
				}
			`,
		}),
	).
		ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(`\Qdir1/Android.bp:5:5: a namespace must be the first module in the file\E`)).
		RunTest(t)
}

func TestTwoModulesWithSameNameInSameNamespace(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a"
				}
				test_module {
					name: "a"
				}
			`,
		}),
	).
		ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(`\Qdir1/Android.bp:7:5: module "a" already defined
       dir1/Android.bp:4:5 <-- previous definition here\E`)).
		RunTest(t)
}

func TestDeclaringNamespaceInNonAndroidBpFile(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		FixtureWithRootAndroidBp(`
				build = ["include.bp"]
		`),
		FixtureAddTextFile("include.bp", `
				soong_namespace {
				}
		`),
	).
		ExtendWithErrorHandler(FixtureExpectsOneErrorPattern(
			`\Qinclude.bp:2:5: A namespace may only be declared in a file named Android.bp\E`,
		)).
		RunTest(t)
}

// so that the generated .ninja file will have consistent names
func TestConsistentNamespaceNames(t *testing.T) {
	result := GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": "soong_namespace{}",
			"dir2": "soong_namespace{}",
			"dir3": "soong_namespace{}",
		}),
	).RunTest(t)

	ns1, _ := result.NameResolver.namespaceAt("dir1")
	ns2, _ := result.NameResolver.namespaceAt("dir2")
	ns3, _ := result.NameResolver.namespaceAt("dir3")
	actualIds := []string{ns1.id, ns2.id, ns3.id}
	expectedIds := []string{"1", "2", "3"}
	if !reflect.DeepEqual(actualIds, expectedIds) {
		t.Errorf("Incorrect namespace ids.\nactual: %s\nexpected: %s\n", actualIds, expectedIds)
	}
}

// so that the generated .ninja file will have consistent names
func TestRename(t *testing.T) {
	GroupFixturePreparers(
		prepareForTestWithNamespace,
		dirBpToPreparer(map[string]string{
			"dir1": `
				soong_namespace {
				}
				test_module {
					name: "a",
					deps: ["c"],
				}
				test_module {
					name: "b",
					rename: "c",
				}
			`,
		}),
	).RunTest(t)

	// RunTest will report any errors
}

// some utils to support the tests

var prepareForTestWithNamespace = GroupFixturePreparers(
	FixtureRegisterWithContext(registerNamespaceBuildComponents),
	FixtureRegisterWithContext(func(ctx RegistrationContext) {
		ctx.PreArchMutators(RegisterNamespaceMutator)
	}),
	FixtureModifyContext(func(ctx *TestContext) {
		ctx.RegisterModuleType("test_module", newTestModule)
		ctx.Context.RegisterModuleType("blueprint_test_module", newBlueprintTestModule)
		ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
			ctx.BottomUp("rename", renameMutator)
		})
	}),
)

// dirBpToPreparer takes a map from directory to the contents of the Android.bp file and produces a
// FixturePreparer.
func dirBpToPreparer(bps map[string]string) FixturePreparer {
	files := make(MockFS, len(bps))
	files["Android.bp"] = []byte("")
	for dir, text := range bps {
		files[filepath.Join(dir, "Android.bp")] = []byte(text)
	}
	return files.AddToFixture()
}

func dependsOn(result *TestResult, module TestingModule, possibleDependency TestingModule) bool {
	depends := false
	visit := func(dependency blueprint.Module) {
		if dependency == possibleDependency.module {
			depends = true
		}
	}
	result.VisitDirectDeps(module.module, visit)
	return depends
}

func numDeps(result *TestResult, module TestingModule) int {
	count := 0
	visit := func(dependency blueprint.Module) {
		count++
	}
	result.VisitDirectDeps(module.module, visit)
	return count
}

func getModule(result *TestResult, moduleName string) TestingModule {
	return result.ModuleForTests(moduleName, "")
}

func findModuleById(result *TestResult, id string) (module TestingModule) {
	visit := func(candidate blueprint.Module) {
		testModule, ok := candidate.(*testModule)
		if ok {
			if testModule.properties.Id == id {
				module = newTestingModule(result.config, testModule)
			}
		}
	}
	result.VisitAllModules(visit)
	return module
}

type testModule struct {
	ModuleBase
	properties struct {
		Rename string
		Deps   []string
		Id     string
	}
}

func (m *testModule) DepsMutator(ctx BottomUpMutatorContext) {
	if m.properties.Rename != "" {
		ctx.Rename(m.properties.Rename)
	}
	for _, d := range m.properties.Deps {
		ctx.AddDependency(ctx.Module(), nil, d)
	}
}

func (m *testModule) GenerateAndroidBuildActions(ModuleContext) {
}

func renameMutator(ctx BottomUpMutatorContext) {
	if m, ok := ctx.Module().(*testModule); ok {
		if m.properties.Rename != "" {
			ctx.Rename(m.properties.Rename)
		}
	}
}

func newTestModule() Module {
	m := &testModule{}
	m.AddProperties(&m.properties)
	InitAndroidModule(m)
	return m
}

type blueprintTestModule struct {
	blueprint.SimpleName
	properties struct {
		Deps []string
	}
}

func (b *blueprintTestModule) DynamicDependencies(_ blueprint.DynamicDependerModuleContext) []string {
	return b.properties.Deps
}

func (b *blueprintTestModule) GenerateBuildActions(blueprint.ModuleContext) {
}

func newBlueprintTestModule() (blueprint.Module, []interface{}) {
	m := &blueprintTestModule{}
	return m, []interface{}{&m.properties, &m.SimpleName.Properties}
}
