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
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/blueprint"
)

func TestDependingOnModuleInSameNamespace(t *testing.T) {
	ctx := setupTest(t,
		map[string]string{
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
		},
	)

	a := getModule(ctx, "a")
	b := getModule(ctx, "b")
	if !dependsOn(ctx, b, a) {
		t.Errorf("module b does not depend on module a in the same namespace")
	}
}

func TestDependingOnModuleInRootNamespace(t *testing.T) {
	ctx := setupTest(t,
		map[string]string{
			".": `
			test_module {
				name: "b",
				deps: ["a"],
			}
			test_module {
				name: "a",
			}
			`,
		},
	)

	a := getModule(ctx, "a")
	b := getModule(ctx, "b")
	if !dependsOn(ctx, b, a) {
		t.Errorf("module b in root namespace does not depend on module a in the root namespace")
	}
}

func TestImplicitlyImportRootNamespace(t *testing.T) {
	_ = setupTest(t,
		map[string]string{
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
		},
	)

	// setupTest will report any errors
}

func TestDependingOnBlueprintModuleInRootNamespace(t *testing.T) {
	_ = setupTest(t,
		map[string]string{
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
		},
	)

	// setupTest will report any errors
}

func TestDependingOnModuleInImportedNamespace(t *testing.T) {
	ctx := setupTest(t,
		map[string]string{
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
		},
	)

	a := getModule(ctx, "a")
	b := getModule(ctx, "b")
	if !dependsOn(ctx, b, a) {
		t.Errorf("module b does not depend on module a in the same namespace")
	}
}

func TestDependingOnModuleInNonImportedNamespace(t *testing.T) {
	_, errs := setupTestExpectErrs(
		map[string]string{
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
		},
	)

	expectedErrors := []error{
		errors.New(
			`dir3/Android.bp:4:4: "b" depends on undefined module "a"
Module "b" is defined in namespace "dir3" which can read these 2 namespaces: ["dir3" "."]
Module "a" can be found in these namespaces: ["dir1" "dir2"]`),
	}

	if len(errs) != 1 || errs[0].Error() != expectedErrors[0].Error() {
		t.Errorf("Incorrect errors. Expected:\n%v\n, got:\n%v\n", expectedErrors, errs)
	}
}

func TestDependingOnModuleByFullyQualifiedReference(t *testing.T) {
	ctx := setupTest(t,
		map[string]string{
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
		},
	)
	a := getModule(ctx, "a")
	b := getModule(ctx, "b")
	if !dependsOn(ctx, b, a) {
		t.Errorf("module b does not depend on module a")
	}
}

func TestSameNameInTwoNamespaces(t *testing.T) {
	ctx := setupTest(t,
		map[string]string{
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
		},
	)

	one := findModuleById(ctx, "1")
	two := findModuleById(ctx, "2")
	three := findModuleById(ctx, "3")
	four := findModuleById(ctx, "4")
	if !dependsOn(ctx, two, one) {
		t.Fatalf("Module 2 does not depend on module 1 in its namespace")
	}
	if dependsOn(ctx, two, three) {
		t.Fatalf("Module 2 depends on module 3 in another namespace")
	}
	if !dependsOn(ctx, four, three) {
		t.Fatalf("Module 4 does not depend on module 3 in its namespace")
	}
	if dependsOn(ctx, four, one) {
		t.Fatalf("Module 4 depends on module 1 in another namespace")
	}
}

func TestSearchOrder(t *testing.T) {
	ctx := setupTest(t,
		map[string]string{
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
		},
	)

	testMe := findModuleById(ctx, "0")
	if !dependsOn(ctx, testMe, findModuleById(ctx, "1")) {
		t.Errorf("test_me doesn't depend on id 1")
	}
	if !dependsOn(ctx, testMe, findModuleById(ctx, "3")) {
		t.Errorf("test_me doesn't depend on id 3")
	}
	if !dependsOn(ctx, testMe, findModuleById(ctx, "6")) {
		t.Errorf("test_me doesn't depend on id 6")
	}
	if !dependsOn(ctx, testMe, findModuleById(ctx, "10")) {
		t.Errorf("test_me doesn't depend on id 10")
	}
	if numDeps(ctx, testMe) != 4 {
		t.Errorf("num dependencies of test_me = %v, not 4\n", numDeps(ctx, testMe))
	}
}

func TestTwoNamespacesCanImportEachOther(t *testing.T) {
	_ = setupTest(t,
		map[string]string{
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
		},
	)

	// setupTest will report any errors
}

func TestImportingNonexistentNamespace(t *testing.T) {
	_, errs := setupTestExpectErrs(
		map[string]string{
			"dir1": `
			soong_namespace {
				imports: ["a_nonexistent_namespace"]
			}
			test_module {
				name: "a",
				deps: ["a_nonexistent_module"]
			}
			`,
		},
	)

	// should complain about the missing namespace and not complain about the unresolvable dependency
	expectedErrors := []error{
		errors.New(`dir1/Android.bp:2:4: module "soong_namespace": namespace a_nonexistent_namespace does not exist`),
	}
	if len(errs) != 1 || errs[0].Error() != expectedErrors[0].Error() {
		t.Errorf("Incorrect errors. Expected:\n%v\n, got:\n%v\n", expectedErrors, errs)
	}
}

func TestNamespacesDontInheritParentNamespaces(t *testing.T) {
	_, errs := setupTestExpectErrs(
		map[string]string{
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
		},
	)

	expectedErrors := []error{
		errors.New(`dir1/subdir1/Android.bp:4:4: "b" depends on undefined module "a"
Module "b" is defined in namespace "dir1/subdir1" which can read these 2 namespaces: ["dir1/subdir1" "."]
Module "a" can be found in these namespaces: ["dir1"]`),
	}
	if len(errs) != 1 || errs[0].Error() != expectedErrors[0].Error() {
		t.Errorf("Incorrect errors. Expected:\n%v\n, got:\n%v\n", expectedErrors, errs)
	}
}

func TestModulesDoReceiveParentNamespace(t *testing.T) {
	_ = setupTest(t,
		map[string]string{
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
		},
	)

	// setupTest will report any errors
}

func TestNamespaceImportsNotTransitive(t *testing.T) {
	_, errs := setupTestExpectErrs(
		map[string]string{
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
		},
	)

	expectedErrors := []error{
		errors.New(`dir3/Android.bp:5:4: "c" depends on undefined module "a"
Module "c" is defined in namespace "dir3" which can read these 3 namespaces: ["dir3" "dir2" "."]
Module "a" can be found in these namespaces: ["dir1"]`),
	}
	if len(errs) != 1 || errs[0].Error() != expectedErrors[0].Error() {
		t.Errorf("Incorrect errors. Expected:\n%v\n, got:\n%v\n", expectedErrors, errs)
	}
}

func TestTwoNamepacesInSameDir(t *testing.T) {
	_, errs := setupTestExpectErrs(
		map[string]string{
			"dir1": `
			soong_namespace {
			}
			soong_namespace {
			}
			`,
		},
	)

	expectedErrors := []error{
		errors.New(`dir1/Android.bp:4:4: namespace dir1 already exists`),
	}
	if len(errs) != 1 || errs[0].Error() != expectedErrors[0].Error() {
		t.Errorf("Incorrect errors. Expected:\n%v\n, got:\n%v\n", expectedErrors, errs)
	}
}

func TestNamespaceNotAtTopOfFile(t *testing.T) {
	_, errs := setupTestExpectErrs(
		map[string]string{
			"dir1": `
			test_module {
				name: "a"
			}
			soong_namespace {
			}
			`,
		},
	)

	expectedErrors := []error{
		errors.New(`dir1/Android.bp:5:4: a namespace must be the first module in the file`),
	}
	if len(errs) != 1 || errs[0].Error() != expectedErrors[0].Error() {
		t.Errorf("Incorrect errors. Expected:\n%v\n, got:\n%v\n", expectedErrors, errs)
	}
}

func TestTwoModulesWithSameNameInSameNamespace(t *testing.T) {
	_, errs := setupTestExpectErrs(
		map[string]string{
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
		},
	)

	expectedErrors := []error{
		errors.New(`dir1/Android.bp:7:4: module "a" already defined
       dir1/Android.bp:4:4 <-- previous definition here`),
	}
	if len(errs) != 1 || errs[0].Error() != expectedErrors[0].Error() {
		t.Errorf("Incorrect errors. Expected:\n%v\n, got:\n%v\n", expectedErrors, errs)
	}
}

func TestDeclaringNamespaceInNonAndroidBpFile(t *testing.T) {
	_, errs := setupTestFromFiles(
		map[string][]byte{
			"Android.bp": []byte(`
				build = ["include.bp"]
			`),
			"include.bp": []byte(`
				soong_namespace {
				}
			`),
		},
	)

	expectedErrors := []error{
		errors.New(`include.bp:2:5: A namespace may only be declared in a file named Android.bp`),
	}

	if len(errs) != 1 || errs[0].Error() != expectedErrors[0].Error() {
		t.Errorf("Incorrect errors. Expected:\n%v\n, got:\n%v\n", expectedErrors, errs)
	}
}

// so that the generated .ninja file will have consistent names
func TestConsistentNamespaceNames(t *testing.T) {
	ctx := setupTest(t,
		map[string]string{
			"dir1": "soong_namespace{}",
			"dir2": "soong_namespace{}",
			"dir3": "soong_namespace{}",
		})

	ns1, _ := ctx.NameResolver.namespaceAt("dir1")
	ns2, _ := ctx.NameResolver.namespaceAt("dir2")
	ns3, _ := ctx.NameResolver.namespaceAt("dir3")
	actualIds := []string{ns1.id, ns2.id, ns3.id}
	expectedIds := []string{"1", "2", "3"}
	if !reflect.DeepEqual(actualIds, expectedIds) {
		t.Errorf("Incorrect namespace ids.\nactual: %s\nexpected: %s\n", actualIds, expectedIds)
	}
}

// so that the generated .ninja file will have consistent names
func TestRename(t *testing.T) {
	_ = setupTest(t,
		map[string]string{
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
		`})
	// setupTest will report any errors
}

// some utils to support the tests

func mockFiles(bps map[string]string) (files map[string][]byte) {
	files = make(map[string][]byte, len(bps))
	files["Android.bp"] = []byte("")
	for dir, text := range bps {
		files[filepath.Join(dir, "Android.bp")] = []byte(text)
	}
	return files
}

func setupTestFromFiles(bps map[string][]byte) (ctx *TestContext, errs []error) {
	buildDir, err := ioutil.TempDir("", "soong_namespace_test")
	if err != nil {
		return nil, []error{err}
	}
	defer os.RemoveAll(buildDir)

	config := TestConfig(buildDir, nil)

	ctx = NewTestContext()
	ctx.MockFileSystem(bps)
	ctx.RegisterModuleType("test_module", ModuleFactoryAdaptor(newTestModule))
	ctx.RegisterModuleType("soong_namespace", ModuleFactoryAdaptor(NamespaceFactory))
	ctx.RegisterModuleType("blueprint_test_module", newBlueprintTestModule)
	ctx.PreArchMutators(RegisterNamespaceMutator)
	ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("rename", renameMutator)
	})
	ctx.Register()

	_, errs = ctx.ParseBlueprintsFiles("Android.bp")
	if len(errs) > 0 {
		return ctx, errs
	}
	_, errs = ctx.PrepareBuildActions(config)
	return ctx, errs
}

func setupTestExpectErrs(bps map[string]string) (ctx *TestContext, errs []error) {
	files := make(map[string][]byte, len(bps))
	files["Android.bp"] = []byte("")
	for dir, text := range bps {
		files[filepath.Join(dir, "Android.bp")] = []byte(text)
	}
	return setupTestFromFiles(files)
}

func setupTest(t *testing.T, bps map[string]string) (ctx *TestContext) {
	t.Helper()
	ctx, errs := setupTestExpectErrs(bps)
	FailIfErrored(t, errs)
	return ctx
}

func dependsOn(ctx *TestContext, module TestingModule, possibleDependency TestingModule) bool {
	depends := false
	visit := func(dependency blueprint.Module) {
		if dependency == possibleDependency.module {
			depends = true
		}
	}
	ctx.VisitDirectDeps(module.module, visit)
	return depends
}

func numDeps(ctx *TestContext, module TestingModule) int {
	count := 0
	visit := func(dependency blueprint.Module) {
		count++
	}
	ctx.VisitDirectDeps(module.module, visit)
	return count
}

func getModule(ctx *TestContext, moduleName string) TestingModule {
	return ctx.ModuleForTests(moduleName, "")
}

func findModuleById(ctx *TestContext, id string) (module TestingModule) {
	visit := func(candidate blueprint.Module) {
		testModule, ok := candidate.(*testModule)
		if ok {
			if testModule.properties.Id == id {
				module = TestingModule{testModule}
			}
		}
	}
	ctx.VisitAllModules(visit)
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

func (b *blueprintTestModule) DynamicDependencies(ctx blueprint.DynamicDependerModuleContext) []string {
	return b.properties.Deps
}

func (b *blueprintTestModule) GenerateBuildActions(blueprint.ModuleContext) {
}

func newBlueprintTestModule() (blueprint.Module, []interface{}) {
	m := &blueprintTestModule{}
	return m, []interface{}{&m.properties, &m.SimpleName.Properties}
}
