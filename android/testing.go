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
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	mkparser "android/soong/androidmk/parser"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func newTestContextForFixture(config Config) *TestContext {
	ctx := &TestContext{
		Context: &Context{blueprint.NewContext(), config},
	}

	ctx.postDeps = append(ctx.postDeps, registerPathDepsMutator)

	ctx.SetFs(ctx.config.fs)
	if ctx.config.mockBpList != "" {
		ctx.SetModuleListFile(ctx.config.mockBpList)
	}

	return ctx
}

func NewTestContext(config Config) *TestContext {
	ctx := newTestContextForFixture(config)

	nameResolver := NewNameResolver(config)
	ctx.NameResolver = nameResolver
	ctx.SetNameInterface(nameResolver)

	return ctx
}

var PrepareForTestWithArchMutator = GroupFixturePreparers(
	// Configure architecture targets in the fixture config.
	FixtureModifyConfig(modifyTestConfigToSupportArchMutator),

	// Add the arch mutator to the context.
	FixtureRegisterWithContext(func(ctx RegistrationContext) {
		ctx.PreDepsMutators(registerArchMutator)
	}),
)

var PrepareForTestWithDefaults = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.PreArchMutators(RegisterDefaultsPreArchMutators)
})

var PrepareForTestWithComponentsMutator = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.PreArchMutators(RegisterComponentsMutator)
})

var PrepareForTestWithPrebuilts = FixtureRegisterWithContext(RegisterPrebuiltMutators)

var PrepareForTestWithOverrides = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.PostDepsMutators(RegisterOverridePostDepsMutators)
})

var PrepareForTestWithLicenses = GroupFixturePreparers(
	FixtureRegisterWithContext(RegisterLicenseKindBuildComponents),
	FixtureRegisterWithContext(RegisterLicenseBuildComponents),
	FixtureRegisterWithContext(registerLicenseMutators),
)

var PrepareForTestWithGenNotice = FixtureRegisterWithContext(RegisterGenNoticeBuildComponents)

func registerLicenseMutators(ctx RegistrationContext) {
	ctx.PreArchMutators(RegisterLicensesPackageMapper)
	ctx.PreArchMutators(RegisterLicensesPropertyGatherer)
	ctx.PostDepsMutators(RegisterLicensesDependencyChecker)
}

var PrepareForTestWithLicenseDefaultModules = GroupFixturePreparers(
	FixtureAddTextFile("build/soong/licenses/Android.bp", `
		license {
				name: "Android-Apache-2.0",
				package_name: "Android",
				license_kinds: ["SPDX-license-identifier-Apache-2.0"],
				copyright_notice: "Copyright (C) The Android Open Source Project",
				license_text: ["LICENSE"],
		}

		license_kind {
				name: "SPDX-license-identifier-Apache-2.0",
				conditions: ["notice"],
				url: "https://spdx.org/licenses/Apache-2.0.html",
		}

		license_kind {
				name: "legacy_unencumbered",
				conditions: ["unencumbered"],
		}
	`),
	FixtureAddFile("build/soong/licenses/LICENSE", nil),
)

var PrepareForTestWithNamespace = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	registerNamespaceBuildComponents(ctx)
	ctx.PreArchMutators(RegisterNamespaceMutator)
})

var PrepareForTestWithMakevars = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.RegisterSingletonType("makevars", makeVarsSingletonFunc)
})

// Test fixture preparer that will register most java build components.
//
// Singletons and mutators should only be added here if they are needed for a majority of java
// module types, otherwise they should be added under a separate preparer to allow them to be
// selected only when needed to reduce test execution time.
//
// Module types do not have much of an overhead unless they are used so this should include as many
// module types as possible. The exceptions are those module types that require mutators and/or
// singletons in order to function in which case they should be kept together in a separate
// preparer.
//
// The mutators in this group were chosen because they are needed by the vast majority of tests.
var PrepareForTestWithAndroidBuildComponents = GroupFixturePreparers(
	// Sorted alphabetically as the actual order does not matter as tests automatically enforce the
	// correct order.
	PrepareForTestWithArchMutator,
	PrepareForTestWithComponentsMutator,
	PrepareForTestWithDefaults,
	PrepareForTestWithFilegroup,
	PrepareForTestWithOverrides,
	PrepareForTestWithPackageModule,
	PrepareForTestWithPrebuilts,
	PrepareForTestWithVisibility,
)

// Prepares an integration test with all build components from the android package.
//
// This should only be used by tests that want to run with as much of the build enabled as possible.
var PrepareForIntegrationTestWithAndroid = GroupFixturePreparers(
	PrepareForTestWithAndroidBuildComponents,
)

// Prepares a test that may be missing dependencies by setting allow_missing_dependencies to
// true.
var PrepareForTestWithAllowMissingDependencies = GroupFixturePreparers(
	FixtureModifyProductVariables(func(variables FixtureProductVariables) {
		variables.Allow_missing_dependencies = proptools.BoolPtr(true)
	}),
	FixtureModifyContext(func(ctx *TestContext) {
		ctx.SetAllowMissingDependencies(true)
	}),
)

// Prepares a test that disallows non-existent paths.
var PrepareForTestDisallowNonExistentPaths = FixtureModifyConfig(func(config Config) {
	config.TestAllowNonExistentPaths = false
})

func NewTestArchContext(config Config) *TestContext {
	ctx := NewTestContext(config)
	ctx.preDeps = append(ctx.preDeps, registerArchMutator)
	return ctx
}

type TestContext struct {
	*Context
	preArch, preDeps, postDeps, finalDeps []RegisterMutatorFunc
	bp2buildPreArch, bp2buildMutators     []RegisterMutatorFunc
	NameResolver                          *NameResolver

	// The list of singletons registered for the test.
	singletons sortableComponents

	// The order in which the mutators and singletons will be run in this test
	// context; for debugging.
	mutatorOrder, singletonOrder []string
}

func (ctx *TestContext) PreArchMutators(f RegisterMutatorFunc) {
	ctx.preArch = append(ctx.preArch, f)
}

func (ctx *TestContext) HardCodedPreArchMutators(f RegisterMutatorFunc) {
	// Register mutator function as normal for testing.
	ctx.PreArchMutators(f)
}

func (ctx *TestContext) ModuleProvider(m blueprint.Module, p blueprint.ProviderKey) interface{} {
	return ctx.Context.ModuleProvider(m, p)
}

func (ctx *TestContext) PreDepsMutators(f RegisterMutatorFunc) {
	ctx.preDeps = append(ctx.preDeps, f)
}

func (ctx *TestContext) PostDepsMutators(f RegisterMutatorFunc) {
	ctx.postDeps = append(ctx.postDeps, f)
}

func (ctx *TestContext) FinalDepsMutators(f RegisterMutatorFunc) {
	ctx.finalDeps = append(ctx.finalDeps, f)
}

// PreArchBp2BuildMutators adds mutators to be register for converting Android Blueprint modules
// into Bazel BUILD targets that should run prior to deps and conversion.
func (ctx *TestContext) PreArchBp2BuildMutators(f RegisterMutatorFunc) {
	ctx.bp2buildPreArch = append(ctx.bp2buildPreArch, f)
}

// registeredComponentOrder defines the order in which a sortableComponent type is registered at
// runtime and provides support for reordering the components registered for a test in the same
// way.
type registeredComponentOrder struct {
	// The name of the component type, used for error messages.
	componentType string

	// The names of the registered components in the order in which they were registered.
	namesInOrder []string

	// Maps from the component name to its position in the runtime ordering.
	namesToIndex map[string]int

	// A function that defines the order between two named components that can be used to sort a slice
	// of component names into the same order as they appear in namesInOrder.
	less func(string, string) bool
}

// registeredComponentOrderFromExistingOrder takes an existing slice of sortableComponents and
// creates a registeredComponentOrder that contains a less function that can be used to sort a
// subset of that list of names so it is in the same order as the original sortableComponents.
func registeredComponentOrderFromExistingOrder(componentType string, existingOrder sortableComponents) registeredComponentOrder {
	// Only the names from the existing order are needed for this so create a list of component names
	// in the correct order.
	namesInOrder := componentsToNames(existingOrder)

	// Populate the map from name to position in the list.
	nameToIndex := make(map[string]int)
	for i, n := range namesInOrder {
		nameToIndex[n] = i
	}

	// A function to use to map from a name to an index in the original order.
	indexOf := func(name string) int {
		index, ok := nameToIndex[name]
		if !ok {
			// Should never happen as tests that use components that are not known at runtime do not sort
			// so should never use this function.
			panic(fmt.Errorf("internal error: unknown %s %q should be one of %s", componentType, name, strings.Join(namesInOrder, ", ")))
		}
		return index
	}

	// The less function.
	less := func(n1, n2 string) bool {
		i1 := indexOf(n1)
		i2 := indexOf(n2)
		return i1 < i2
	}

	return registeredComponentOrder{
		componentType: componentType,
		namesInOrder:  namesInOrder,
		namesToIndex:  nameToIndex,
		less:          less,
	}
}

// componentsToNames maps from the slice of components to a slice of their names.
func componentsToNames(components sortableComponents) []string {
	names := make([]string, len(components))
	for i, c := range components {
		names[i] = c.componentName()
	}
	return names
}

// enforceOrdering enforces the supplied components are in the same order as is defined in this
// object.
//
// If the supplied components contains any components that are not registered at runtime, i.e. test
// specific components, then it is impossible to sort them into an order that both matches the
// runtime and also preserves the implicit ordering defined in the test. In that case it will not
// sort the components, instead it will just check that the components are in the correct order.
//
// Otherwise, this will sort the supplied components in place.
func (o *registeredComponentOrder) enforceOrdering(components sortableComponents) {
	// Check to see if the list of components contains any components that are
	// not registered at runtime.
	var unknownComponents []string
	testOrder := componentsToNames(components)
	for _, name := range testOrder {
		if _, ok := o.namesToIndex[name]; !ok {
			unknownComponents = append(unknownComponents, name)
			break
		}
	}

	// If the slice contains some unknown components then it is not possible to
	// sort them into an order that matches the runtime while also preserving the
	// order expected from the test, so in that case don't sort just check that
	// the order of the known mutators does match.
	if len(unknownComponents) > 0 {
		// Check order.
		o.checkTestOrder(testOrder, unknownComponents)
	} else {
		// Sort the components.
		sort.Slice(components, func(i, j int) bool {
			n1 := components[i].componentName()
			n2 := components[j].componentName()
			return o.less(n1, n2)
		})
	}
}

// checkTestOrder checks that the supplied testOrder matches the one defined by this object,
// panicking if it does not.
func (o *registeredComponentOrder) checkTestOrder(testOrder []string, unknownComponents []string) {
	lastMatchingTest := -1
	matchCount := 0
	// Take a copy of the runtime order as it is modified during the comparison.
	runtimeOrder := append([]string(nil), o.namesInOrder...)
	componentType := o.componentType
	for i, j := 0, 0; i < len(testOrder) && j < len(runtimeOrder); {
		test := testOrder[i]
		runtime := runtimeOrder[j]

		if test == runtime {
			testOrder[i] = test + fmt.Sprintf(" <-- matched with runtime %s %d", componentType, j)
			runtimeOrder[j] = runtime + fmt.Sprintf(" <-- matched with test %s %d", componentType, i)
			lastMatchingTest = i
			i += 1
			j += 1
			matchCount += 1
		} else if _, ok := o.namesToIndex[test]; !ok {
			// The test component is not registered globally so assume it is the correct place, treat it
			// as having matched and skip it.
			i += 1
			matchCount += 1
		} else {
			// Assume that the test list is in the same order as the runtime list but the runtime list
			// contains some components that are not present in the tests. So, skip the runtime component
			// to try and find the next one that matches the current test component.
			j += 1
		}
	}

	// If every item in the test order was either test specific or matched one in the runtime then
	// it is in the correct order. Otherwise, it was not so fail.
	if matchCount != len(testOrder) {
		// The test component names were not all matched with a runtime component name so there must
		// either be a component present in the test that is not present in the runtime or they must be
		// in the wrong order.
		testOrder[lastMatchingTest+1] = testOrder[lastMatchingTest+1] + " <--- unmatched"
		panic(fmt.Errorf("the tests uses test specific components %q and so cannot be automatically sorted."+
			" Unfortunately it uses %s components in the wrong order.\n"+
			"test order:\n    %s\n"+
			"runtime order\n    %s\n",
			SortedUniqueStrings(unknownComponents),
			componentType,
			strings.Join(testOrder, "\n    "),
			strings.Join(runtimeOrder, "\n    ")))
	}
}

// registrationSorter encapsulates the information needed to ensure that the test mutators are
// registered, and thereby executed, in the same order as they are at runtime.
//
// It MUST be populated lazily AFTER all package initialization has been done otherwise it will
// only define the order for a subset of all the registered build components that are available for
// the packages being tested.
//
// e.g if this is initialized during say the cc package initialization then any tests run in the
// java package will not sort build components registered by the java package's init() functions.
type registrationSorter struct {
	// Used to ensure that this is only created once.
	once sync.Once

	// The order of mutators
	mutatorOrder registeredComponentOrder

	// The order of singletons
	singletonOrder registeredComponentOrder
}

// populate initializes this structure from globally registered build components.
//
// Only the first call has any effect.
func (s *registrationSorter) populate() {
	s.once.Do(func() {
		// Created an ordering from the globally registered mutators.
		globallyRegisteredMutators := collateGloballyRegisteredMutators()
		s.mutatorOrder = registeredComponentOrderFromExistingOrder("mutator", globallyRegisteredMutators)

		// Create an ordering from the globally registered singletons.
		globallyRegisteredSingletons := collateGloballyRegisteredSingletons()
		s.singletonOrder = registeredComponentOrderFromExistingOrder("singleton", globallyRegisteredSingletons)
	})
}

// Provides support for enforcing the same order in which build components are registered globally
// to the order in which they are registered during tests.
//
// MUST only be accessed via the globallyRegisteredComponentsOrder func.
var globalRegistrationSorter registrationSorter

// globallyRegisteredComponentsOrder returns the globalRegistrationSorter after ensuring it is
// correctly populated.
func globallyRegisteredComponentsOrder() *registrationSorter {
	globalRegistrationSorter.populate()
	return &globalRegistrationSorter
}

func (ctx *TestContext) Register() {
	globalOrder := globallyRegisteredComponentsOrder()

	mutators := collateRegisteredMutators(ctx.preArch, ctx.preDeps, ctx.postDeps, ctx.finalDeps)
	// Ensure that the mutators used in the test are in the same order as they are used at runtime.
	globalOrder.mutatorOrder.enforceOrdering(mutators)
	mutators.registerAll(ctx.Context)

	// Ensure that the singletons used in the test are in the same order as they are used at runtime.
	globalOrder.singletonOrder.enforceOrdering(ctx.singletons)
	ctx.singletons.registerAll(ctx.Context)

	// Save the sorted components order away to make them easy to access while debugging.
	ctx.mutatorOrder = componentsToNames(mutators)
	ctx.singletonOrder = componentsToNames(singletons)
}

func (ctx *TestContext) ParseFileList(rootDir string, filePaths []string) (deps []string, errs []error) {
	// This function adapts the old style ParseFileList calls that are spread throughout the tests
	// to the new style that takes a config.
	return ctx.Context.ParseFileList(rootDir, filePaths, ctx.config)
}

func (ctx *TestContext) ParseBlueprintsFiles(rootDir string) (deps []string, errs []error) {
	// This function adapts the old style ParseBlueprintsFiles calls that are spread throughout the
	// tests to the new style that takes a config.
	return ctx.Context.ParseBlueprintsFiles(rootDir, ctx.config)
}

func (ctx *TestContext) RegisterModuleType(name string, factory ModuleFactory) {
	ctx.Context.RegisterModuleType(name, ModuleFactoryAdaptor(factory))
}

func (ctx *TestContext) RegisterSingletonModuleType(name string, factory SingletonModuleFactory) {
	s, m := SingletonModuleFactoryAdaptor(name, factory)
	ctx.RegisterSingletonType(name, s)
	ctx.RegisterModuleType(name, m)
}

func (ctx *TestContext) RegisterParallelSingletonModuleType(name string, factory SingletonModuleFactory) {
	s, m := SingletonModuleFactoryAdaptor(name, factory)
	ctx.RegisterParallelSingletonType(name, s)
	ctx.RegisterModuleType(name, m)
}

func (ctx *TestContext) RegisterSingletonType(name string, factory SingletonFactory) {
	ctx.singletons = append(ctx.singletons, newSingleton(name, factory, false))
}

func (ctx *TestContext) RegisterParallelSingletonType(name string, factory SingletonFactory) {
	ctx.singletons = append(ctx.singletons, newSingleton(name, factory, true))
}

// ModuleVariantForTests selects a specific variant of the module with the given
// name by matching the variations map against the variations of each module
// variant. A module variant matches the map if every variation that exists in
// both have the same value. Both the module and the map are allowed to have
// extra variations that the other doesn't have. Panics if not exactly one
// module variant matches.
func (ctx *TestContext) ModuleVariantForTests(name string, matchVariations map[string]string) TestingModule {
	modules := []Module{}
	ctx.VisitAllModules(func(m blueprint.Module) {
		if ctx.ModuleName(m) == name {
			am := m.(Module)
			amMut := am.base().commonProperties.DebugMutators
			amVar := am.base().commonProperties.DebugVariations
			matched := true
			for i, mut := range amMut {
				if wantedVar, found := matchVariations[mut]; found && amVar[i] != wantedVar {
					matched = false
					break
				}
			}
			if matched {
				modules = append(modules, am)
			}
		}
	})

	if len(modules) == 0 {
		// Show all the modules or module variants that do exist.
		var allModuleNames []string
		var allVariants []string
		ctx.VisitAllModules(func(m blueprint.Module) {
			allModuleNames = append(allModuleNames, ctx.ModuleName(m))
			if ctx.ModuleName(m) == name {
				allVariants = append(allVariants, m.(Module).String())
			}
		})

		if len(allVariants) == 0 {
			panic(fmt.Errorf("failed to find module %q. All modules:\n  %s",
				name, strings.Join(SortedUniqueStrings(allModuleNames), "\n  ")))
		} else {
			sort.Strings(allVariants)
			panic(fmt.Errorf("failed to find module %q matching %v. All variants:\n  %s",
				name, matchVariations, strings.Join(allVariants, "\n  ")))
		}
	}

	if len(modules) > 1 {
		moduleStrings := []string{}
		for _, m := range modules {
			moduleStrings = append(moduleStrings, m.String())
		}
		sort.Strings(moduleStrings)
		panic(fmt.Errorf("module %q has more than one variant that match %v:\n  %s",
			name, matchVariations, strings.Join(moduleStrings, "\n  ")))
	}

	return newTestingModule(ctx.config, modules[0])
}

func (ctx *TestContext) ModuleForTests(name, variant string) TestingModule {
	var module Module
	ctx.VisitAllModules(func(m blueprint.Module) {
		if ctx.ModuleName(m) == name && ctx.ModuleSubDir(m) == variant {
			module = m.(Module)
		}
	})

	if module == nil {
		// find all the modules that do exist
		var allModuleNames []string
		var allVariants []string
		ctx.VisitAllModules(func(m blueprint.Module) {
			allModuleNames = append(allModuleNames, ctx.ModuleName(m))
			if ctx.ModuleName(m) == name {
				allVariants = append(allVariants, ctx.ModuleSubDir(m))
			}
		})
		sort.Strings(allVariants)

		if len(allVariants) == 0 {
			panic(fmt.Errorf("failed to find module %q. All modules:\n  %s",
				name, strings.Join(SortedUniqueStrings(allModuleNames), "\n  ")))
		} else {
			panic(fmt.Errorf("failed to find module %q variant %q. All variants:\n  %s",
				name, variant, strings.Join(allVariants, "\n  ")))
		}
	}

	return newTestingModule(ctx.config, module)
}

func (ctx *TestContext) ModuleVariantsForTests(name string) []string {
	var variants []string
	ctx.VisitAllModules(func(m blueprint.Module) {
		if ctx.ModuleName(m) == name {
			variants = append(variants, ctx.ModuleSubDir(m))
		}
	})
	return variants
}

// SingletonForTests returns a TestingSingleton for the singleton registered with the given name.
func (ctx *TestContext) SingletonForTests(name string) TestingSingleton {
	allSingletonNames := []string{}
	for _, s := range ctx.Singletons() {
		n := ctx.SingletonName(s)
		if n == name {
			return TestingSingleton{
				baseTestingComponent: newBaseTestingComponent(ctx.config, s.(testBuildProvider)),
				singleton:            s.(*singletonAdaptor).Singleton,
			}
		}
		allSingletonNames = append(allSingletonNames, n)
	}

	panic(fmt.Errorf("failed to find singleton %q."+
		"\nall singletons: %v", name, allSingletonNames))
}

type InstallMakeRule struct {
	Target        string
	Deps          []string
	OrderOnlyDeps []string
}

func parseMkRules(t *testing.T, config Config, nodes []mkparser.Node) []InstallMakeRule {
	var rules []InstallMakeRule
	for _, node := range nodes {
		if mkParserRule, ok := node.(*mkparser.Rule); ok {
			var rule InstallMakeRule

			if targets := mkParserRule.Target.Words(); len(targets) == 0 {
				t.Fatalf("no targets for rule %s", mkParserRule.Dump())
			} else if len(targets) > 1 {
				t.Fatalf("unsupported multiple targets for rule %s", mkParserRule.Dump())
			} else if !targets[0].Const() {
				t.Fatalf("unsupported non-const target for rule %s", mkParserRule.Dump())
			} else {
				rule.Target = normalizeStringRelativeToTop(config, targets[0].Value(nil))
			}

			prereqList := &rule.Deps
			for _, prereq := range mkParserRule.Prerequisites.Words() {
				if !prereq.Const() {
					t.Fatalf("unsupported non-const prerequisite for rule %s", mkParserRule.Dump())
				}

				if prereq.Value(nil) == "|" {
					prereqList = &rule.OrderOnlyDeps
					continue
				}

				*prereqList = append(*prereqList, normalizeStringRelativeToTop(config, prereq.Value(nil)))
			}

			rules = append(rules, rule)
		}
	}

	return rules
}

func (ctx *TestContext) InstallMakeRulesForTesting(t *testing.T) []InstallMakeRule {
	installs := ctx.SingletonForTests("makevars").Singleton().(*makeVarsSingleton).installsForTesting
	buf := bytes.NewBuffer(append([]byte(nil), installs...))
	parser := mkparser.NewParser("makevars", buf)

	nodes, errs := parser.Parse()
	if len(errs) > 0 {
		t.Fatalf("error parsing install rules: %s", errs[0])
	}

	return parseMkRules(t, ctx.config, nodes)
}

// MakeVarVariable provides access to make vars that will be written by the makeVarsSingleton
type MakeVarVariable interface {
	// Name is the name of the variable.
	Name() string

	// Value is the value of the variable.
	Value() string
}

func (v makeVarsVariable) Name() string {
	return v.name
}

func (v makeVarsVariable) Value() string {
	return v.value
}

// PrepareForTestAccessingMakeVars sets up the test so that MakeVarsForTesting will work.
var PrepareForTestAccessingMakeVars = GroupFixturePreparers(
	PrepareForTestWithAndroidMk,
	PrepareForTestWithMakevars,
)

// MakeVarsForTesting returns a filtered list of MakeVarVariable objects that represent the
// variables that will be written out.
//
// It is necessary to use PrepareForTestAccessingMakeVars in tests that want to call this function.
// Along with any other preparers needed to add the make vars.
func (ctx *TestContext) MakeVarsForTesting(filter func(variable MakeVarVariable) bool) []MakeVarVariable {
	vars := ctx.SingletonForTests("makevars").Singleton().(*makeVarsSingleton).varsForTesting
	result := make([]MakeVarVariable, 0, len(vars))
	for _, v := range vars {
		if filter(v) {
			result = append(result, v)
		}
	}

	return result
}

func (ctx *TestContext) Config() Config {
	return ctx.config
}

type testBuildProvider interface {
	BuildParamsForTests() []BuildParams
	RuleParamsForTests() map[blueprint.Rule]blueprint.RuleParams
}

type TestingBuildParams struct {
	BuildParams
	RuleParams blueprint.RuleParams

	config Config
}

// RelativeToTop creates a new instance of this which has had any usages of the current test's
// temporary and test specific build directory replaced with a path relative to the notional top.
//
// The parts of this structure which are changed are:
// * BuildParams
//   - Args
//   - All Path, Paths, WritablePath and WritablePaths fields.
//
// * RuleParams
//   - Command
//   - Depfile
//   - Rspfile
//   - RspfileContent
//   - SymlinkOutputs
//   - CommandDeps
//   - CommandOrderOnly
//
// See PathRelativeToTop for more details.
//
// deprecated: this is no longer needed as TestingBuildParams are created in this form.
func (p TestingBuildParams) RelativeToTop() TestingBuildParams {
	// If this is not a valid params then just return it back. That will make it easy to use with the
	// Maybe...() methods.
	if p.Rule == nil {
		return p
	}
	if p.config.config == nil {
		return p
	}
	// Take a copy of the build params and replace any args that contains test specific temporary
	// paths with paths relative to the top.
	bparams := p.BuildParams
	bparams.Depfile = normalizeWritablePathRelativeToTop(bparams.Depfile)
	bparams.Output = normalizeWritablePathRelativeToTop(bparams.Output)
	bparams.Outputs = bparams.Outputs.RelativeToTop()
	bparams.SymlinkOutput = normalizeWritablePathRelativeToTop(bparams.SymlinkOutput)
	bparams.SymlinkOutputs = bparams.SymlinkOutputs.RelativeToTop()
	bparams.ImplicitOutput = normalizeWritablePathRelativeToTop(bparams.ImplicitOutput)
	bparams.ImplicitOutputs = bparams.ImplicitOutputs.RelativeToTop()
	bparams.Input = normalizePathRelativeToTop(bparams.Input)
	bparams.Inputs = bparams.Inputs.RelativeToTop()
	bparams.Implicit = normalizePathRelativeToTop(bparams.Implicit)
	bparams.Implicits = bparams.Implicits.RelativeToTop()
	bparams.OrderOnly = bparams.OrderOnly.RelativeToTop()
	bparams.Validation = normalizePathRelativeToTop(bparams.Validation)
	bparams.Validations = bparams.Validations.RelativeToTop()
	bparams.Args = normalizeStringMapRelativeToTop(p.config, bparams.Args)

	// Ditto for any fields in the RuleParams.
	rparams := p.RuleParams
	rparams.Command = normalizeStringRelativeToTop(p.config, rparams.Command)
	rparams.Depfile = normalizeStringRelativeToTop(p.config, rparams.Depfile)
	rparams.Rspfile = normalizeStringRelativeToTop(p.config, rparams.Rspfile)
	rparams.RspfileContent = normalizeStringRelativeToTop(p.config, rparams.RspfileContent)
	rparams.SymlinkOutputs = normalizeStringArrayRelativeToTop(p.config, rparams.SymlinkOutputs)
	rparams.CommandDeps = normalizeStringArrayRelativeToTop(p.config, rparams.CommandDeps)
	rparams.CommandOrderOnly = normalizeStringArrayRelativeToTop(p.config, rparams.CommandOrderOnly)

	return TestingBuildParams{
		BuildParams: bparams,
		RuleParams:  rparams,
	}
}

func normalizeWritablePathRelativeToTop(path WritablePath) WritablePath {
	if path == nil {
		return nil
	}
	return path.RelativeToTop().(WritablePath)
}

func normalizePathRelativeToTop(path Path) Path {
	if path == nil {
		return nil
	}
	return path.RelativeToTop()
}

func allOutputs(p BuildParams) []string {
	outputs := append(WritablePaths(nil), p.Outputs...)
	outputs = append(outputs, p.ImplicitOutputs...)
	if p.Output != nil {
		outputs = append(outputs, p.Output)
	}
	return outputs.Strings()
}

// AllOutputs returns all 'BuildParams.Output's and 'BuildParams.Outputs's in their full path string forms.
func (p TestingBuildParams) AllOutputs() []string {
	return allOutputs(p.BuildParams)
}

// baseTestingComponent provides functionality common to both TestingModule and TestingSingleton.
type baseTestingComponent struct {
	config   Config
	provider testBuildProvider
}

func newBaseTestingComponent(config Config, provider testBuildProvider) baseTestingComponent {
	return baseTestingComponent{config, provider}
}

// A function that will normalize a string containing paths, e.g. ninja command, by replacing
// any references to the test specific temporary build directory that changes with each run to a
// fixed path relative to a notional top directory.
//
// This is similar to StringPathRelativeToTop except that assumes the string is a single path
// containing at most one instance of the temporary build directory at the start of the path while
// this assumes that there can be any number at any position.
func normalizeStringRelativeToTop(config Config, s string) string {
	// The soongOutDir usually looks something like: /tmp/testFoo2345/001
	//
	// Replace any usage of the soongOutDir with out/soong, e.g. replace "/tmp/testFoo2345/001" with
	// "out/soong".
	outSoongDir := filepath.Clean(config.soongOutDir)
	re := regexp.MustCompile(`\Q` + outSoongDir + `\E\b`)
	s = re.ReplaceAllString(s, "out/soong")

	// Replace any usage of the soongOutDir/.. with out, e.g. replace "/tmp/testFoo2345" with
	// "out". This must come after the previous replacement otherwise this would replace
	// "/tmp/testFoo2345/001" with "out/001" instead of "out/soong".
	outDir := filepath.Dir(outSoongDir)
	re = regexp.MustCompile(`\Q` + outDir + `\E\b`)
	s = re.ReplaceAllString(s, "out")

	return s
}

// normalizeStringArrayRelativeToTop creates a new slice constructed by applying
// normalizeStringRelativeToTop to each item in the slice.
func normalizeStringArrayRelativeToTop(config Config, slice []string) []string {
	newSlice := make([]string, len(slice))
	for i, s := range slice {
		newSlice[i] = normalizeStringRelativeToTop(config, s)
	}
	return newSlice
}

// normalizeStringMapRelativeToTop creates a new map constructed by applying
// normalizeStringRelativeToTop to each value in the map.
func normalizeStringMapRelativeToTop(config Config, m map[string]string) map[string]string {
	newMap := map[string]string{}
	for k, v := range m {
		newMap[k] = normalizeStringRelativeToTop(config, v)
	}
	return newMap
}

func (b baseTestingComponent) newTestingBuildParams(bparams BuildParams) TestingBuildParams {
	return TestingBuildParams{
		config:      b.config,
		BuildParams: bparams,
		RuleParams:  b.provider.RuleParamsForTests()[bparams.Rule],
	}.RelativeToTop()
}

func (b baseTestingComponent) maybeBuildParamsFromRule(rule string) (TestingBuildParams, []string) {
	var searchedRules []string
	buildParams := b.provider.BuildParamsForTests()
	for _, p := range buildParams {
		ruleAsString := p.Rule.String()
		searchedRules = append(searchedRules, ruleAsString)
		if strings.Contains(ruleAsString, rule) {
			return b.newTestingBuildParams(p), searchedRules
		}
	}
	return TestingBuildParams{}, searchedRules
}

func (b baseTestingComponent) buildParamsFromRule(rule string) TestingBuildParams {
	p, searchRules := b.maybeBuildParamsFromRule(rule)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find rule %q.\nall rules:\n%s", rule, strings.Join(searchRules, "\n")))
	}
	return p
}

func (b baseTestingComponent) maybeBuildParamsFromDescription(desc string) (TestingBuildParams, []string) {
	var searchedDescriptions []string
	for _, p := range b.provider.BuildParamsForTests() {
		searchedDescriptions = append(searchedDescriptions, p.Description)
		if strings.Contains(p.Description, desc) {
			return b.newTestingBuildParams(p), searchedDescriptions
		}
	}
	return TestingBuildParams{}, searchedDescriptions
}

func (b baseTestingComponent) buildParamsFromDescription(desc string) TestingBuildParams {
	p, searchedDescriptions := b.maybeBuildParamsFromDescription(desc)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find description %q\nall descriptions:\n%s", desc, strings.Join(searchedDescriptions, "\n")))
	}
	return p
}

func (b baseTestingComponent) maybeBuildParamsFromOutput(file string) (TestingBuildParams, []string) {
	searchedOutputs := WritablePaths(nil)
	for _, p := range b.provider.BuildParamsForTests() {
		outputs := append(WritablePaths(nil), p.Outputs...)
		outputs = append(outputs, p.ImplicitOutputs...)
		if p.Output != nil {
			outputs = append(outputs, p.Output)
		}
		for _, f := range outputs {
			if f.String() == file || f.Rel() == file || PathRelativeToTop(f) == file {
				return b.newTestingBuildParams(p), nil
			}
			searchedOutputs = append(searchedOutputs, f)
		}
	}

	formattedOutputs := []string{}
	for _, f := range searchedOutputs {
		formattedOutputs = append(formattedOutputs,
			fmt.Sprintf("%s (rel=%s)", PathRelativeToTop(f), f.Rel()))
	}

	return TestingBuildParams{}, formattedOutputs
}

func (b baseTestingComponent) buildParamsFromOutput(file string) TestingBuildParams {
	p, searchedOutputs := b.maybeBuildParamsFromOutput(file)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find output %q.\nall outputs:\n    %s\n",
			file, strings.Join(searchedOutputs, "\n    ")))
	}
	return p
}

func (b baseTestingComponent) allOutputs() []string {
	var outputFullPaths []string
	for _, p := range b.provider.BuildParamsForTests() {
		outputFullPaths = append(outputFullPaths, allOutputs(p)...)
	}
	return outputFullPaths
}

// MaybeRule finds a call to ctx.Build with BuildParams.Rule set to a rule with the given name.  Returns an empty
// BuildParams if no rule is found.
func (b baseTestingComponent) MaybeRule(rule string) TestingBuildParams {
	r, _ := b.maybeBuildParamsFromRule(rule)
	return r
}

// Rule finds a call to ctx.Build with BuildParams.Rule set to a rule with the given name.  Panics if no rule is found.
func (b baseTestingComponent) Rule(rule string) TestingBuildParams {
	return b.buildParamsFromRule(rule)
}

// MaybeDescription finds a call to ctx.Build with BuildParams.Description set to a the given string.  Returns an empty
// BuildParams if no rule is found.
func (b baseTestingComponent) MaybeDescription(desc string) TestingBuildParams {
	p, _ := b.maybeBuildParamsFromDescription(desc)
	return p
}

// Description finds a call to ctx.Build with BuildParams.Description set to a the given string.  Panics if no rule is
// found.
func (b baseTestingComponent) Description(desc string) TestingBuildParams {
	return b.buildParamsFromDescription(desc)
}

// MaybeOutput finds a call to ctx.Build with a BuildParams.Output or BuildParams.Outputs whose String() or Rel()
// value matches the provided string.  Returns an empty BuildParams if no rule is found.
func (b baseTestingComponent) MaybeOutput(file string) TestingBuildParams {
	p, _ := b.maybeBuildParamsFromOutput(file)
	return p
}

// Output finds a call to ctx.Build with a BuildParams.Output or BuildParams.Outputs whose String() or Rel()
// value matches the provided string.  Panics if no rule is found.
func (b baseTestingComponent) Output(file string) TestingBuildParams {
	return b.buildParamsFromOutput(file)
}

// AllOutputs returns all 'BuildParams.Output's and 'BuildParams.Outputs's in their full path string forms.
func (b baseTestingComponent) AllOutputs() []string {
	return b.allOutputs()
}

// TestingModule is wrapper around an android.Module that provides methods to find information about individual
// ctx.Build parameters for verification in tests.
type TestingModule struct {
	baseTestingComponent
	module Module
}

func newTestingModule(config Config, module Module) TestingModule {
	return TestingModule{
		newBaseTestingComponent(config, module),
		module,
	}
}

// Module returns the Module wrapped by the TestingModule.
func (m TestingModule) Module() Module {
	return m.module
}

// VariablesForTestsRelativeToTop returns a copy of the Module.VariablesForTests() with every value
// having any temporary build dir usages replaced with paths relative to a notional top.
func (m TestingModule) VariablesForTestsRelativeToTop() map[string]string {
	return normalizeStringMapRelativeToTop(m.config, m.module.VariablesForTests())
}

// OutputFiles calls OutputFileProducer.OutputFiles on the encapsulated module, exits the test
// immediately if there is an error and otherwise returns the result of calling Paths.RelativeToTop
// on the returned Paths.
func (m TestingModule) OutputFiles(t *testing.T, tag string) Paths {
	producer, ok := m.module.(OutputFileProducer)
	if !ok {
		t.Fatalf("%q must implement OutputFileProducer\n", m.module.Name())
	}
	paths, err := producer.OutputFiles(tag)
	if err != nil {
		t.Fatal(err)
	}

	return paths.RelativeToTop()
}

// TestingSingleton is wrapper around an android.Singleton that provides methods to find information about individual
// ctx.Build parameters for verification in tests.
type TestingSingleton struct {
	baseTestingComponent
	singleton Singleton
}

// Singleton returns the Singleton wrapped by the TestingSingleton.
func (s TestingSingleton) Singleton() Singleton {
	return s.singleton
}

func FailIfErrored(t *testing.T, errs []error) {
	t.Helper()
	if len(errs) > 0 {
		for _, err := range errs {
			t.Error(err)
		}
		t.FailNow()
	}
}

// Fail if no errors that matched the regular expression were found.
//
// Returns true if a matching error was found, false otherwise.
func FailIfNoMatchingErrors(t *testing.T, pattern string, errs []error) bool {
	t.Helper()

	matcher, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("failed to compile regular expression %q because %s", pattern, err)
	}

	found := false
	for _, err := range errs {
		if matcher.FindStringIndex(err.Error()) != nil {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("could not match the expected error regex %q (checked %d error(s))", pattern, len(errs))
		for i, err := range errs {
			t.Errorf("errs[%d] = %q", i, err)
		}
	}

	return found
}

func CheckErrorsAgainstExpectations(t *testing.T, errs []error, expectedErrorPatterns []string) {
	t.Helper()

	if expectedErrorPatterns == nil {
		FailIfErrored(t, errs)
	} else {
		for _, expectedError := range expectedErrorPatterns {
			FailIfNoMatchingErrors(t, expectedError, errs)
		}
		if len(errs) > len(expectedErrorPatterns) {
			t.Errorf("additional errors found, expected %d, found %d",
				len(expectedErrorPatterns), len(errs))
			for i, expectedError := range expectedErrorPatterns {
				t.Errorf("expectedErrors[%d] = %s", i, expectedError)
			}
			for i, err := range errs {
				t.Errorf("errs[%d] = %s", i, err)
			}
			t.FailNow()
		}
	}
}

func SetKatiEnabledForTests(config Config) {
	config.katiEnabled = true
}

func SetTrimmedApexEnabledForTests(config Config) {
	config.productVariables.TrimmedApex = new(bool)
	*config.productVariables.TrimmedApex = true
}

func AndroidMkEntriesForTest(t *testing.T, ctx *TestContext, mod blueprint.Module) []AndroidMkEntries {
	t.Helper()
	var p AndroidMkEntriesProvider
	var ok bool
	if p, ok = mod.(AndroidMkEntriesProvider); !ok {
		t.Errorf("module does not implement AndroidMkEntriesProvider: " + mod.Name())
	}

	entriesList := p.AndroidMkEntries()
	for i, _ := range entriesList {
		entriesList[i].fillInEntries(ctx, mod)
	}
	return entriesList
}

func AndroidMkDataForTest(t *testing.T, ctx *TestContext, mod blueprint.Module) AndroidMkData {
	t.Helper()
	var p AndroidMkDataProvider
	var ok bool
	if p, ok = mod.(AndroidMkDataProvider); !ok {
		t.Fatalf("module does not implement AndroidMkDataProvider: " + mod.Name())
	}
	data := p.AndroidMk()
	data.fillInData(ctx, mod)
	return data
}

// Normalize the path for testing.
//
// If the path is relative to the build directory then return the relative path
// to avoid tests having to deal with the dynamically generated build directory.
//
// Otherwise, return the supplied path as it is almost certainly a source path
// that is relative to the root of the source tree.
//
// The build and source paths should be distinguishable based on their contents.
//
// deprecated: use PathRelativeToTop instead as it handles make install paths and differentiates
// between output and source properly.
func NormalizePathForTesting(path Path) string {
	if path == nil {
		return "<nil path>"
	}
	p := path.String()
	if w, ok := path.(WritablePath); ok {
		rel, err := filepath.Rel(w.getSoongOutDir(), p)
		if err != nil {
			panic(err)
		}
		return rel
	}
	return p
}

// NormalizePathsForTesting creates a slice of strings where each string is the result of applying
// NormalizePathForTesting to the corresponding Path in the input slice.
//
// deprecated: use PathsRelativeToTop instead as it handles make install paths and differentiates
// between output and source properly.
func NormalizePathsForTesting(paths Paths) []string {
	var result []string
	for _, path := range paths {
		relative := NormalizePathForTesting(path)
		result = append(result, relative)
	}
	return result
}

// PathRelativeToTop returns a string representation of the path relative to a notional top
// directory.
//
// It return "<nil path>" if the supplied path is nil, otherwise it returns the result of calling
// Path.RelativeToTop to obtain a relative Path and then calling Path.String on that to get the
// string representation.
func PathRelativeToTop(path Path) string {
	if path == nil {
		return "<nil path>"
	}
	return path.RelativeToTop().String()
}

// PathsRelativeToTop creates a slice of strings where each string is the result of applying
// PathRelativeToTop to the corresponding Path in the input slice.
func PathsRelativeToTop(paths Paths) []string {
	var result []string
	for _, path := range paths {
		relative := PathRelativeToTop(path)
		result = append(result, relative)
	}
	return result
}

// StringPathRelativeToTop returns a string representation of the path relative to a notional top
// directory.
//
// See Path.RelativeToTop for more details as to what `relative to top` means.
//
// This is provided for processing paths that have already been converted into a string, e.g. paths
// in AndroidMkEntries structures. As a result it needs to be supplied the soong output dir against
// which it can try and relativize paths. PathRelativeToTop must be used for process Path objects.
func StringPathRelativeToTop(soongOutDir string, path string) string {
	ensureTestOnly()

	// A relative path must be a source path so leave it as it is.
	if !filepath.IsAbs(path) {
		return path
	}

	// Check to see if the path is relative to the soong out dir.
	rel, isRel, err := maybeRelErr(soongOutDir, path)
	if err != nil {
		panic(err)
	}

	if isRel {
		// The path is in the soong out dir so indicate that in the relative path.
		return filepath.Join("out/soong", rel)
	}

	// Check to see if the path is relative to the top level out dir.
	outDir := filepath.Dir(soongOutDir)
	rel, isRel, err = maybeRelErr(outDir, path)
	if err != nil {
		panic(err)
	}

	if isRel {
		// The path is in the out dir so indicate that in the relative path.
		return filepath.Join("out", rel)
	}

	// This should never happen.
	panic(fmt.Errorf("internal error: absolute path %s is not relative to the out dir %s", path, outDir))
}

// StringPathsRelativeToTop creates a slice of strings where each string is the result of applying
// StringPathRelativeToTop to the corresponding string path in the input slice.
//
// This is provided for processing paths that have already been converted into a string, e.g. paths
// in AndroidMkEntries structures. As a result it needs to be supplied the soong output dir against
// which it can try and relativize paths. PathsRelativeToTop must be used for process Paths objects.
func StringPathsRelativeToTop(soongOutDir string, paths []string) []string {
	var result []string
	for _, path := range paths {
		relative := StringPathRelativeToTop(soongOutDir, path)
		result = append(result, relative)
	}
	return result
}

// StringRelativeToTop will normalize a string containing paths, e.g. ninja command, by replacing
// any references to the test specific temporary build directory that changes with each run to a
// fixed path relative to a notional top directory.
//
// This is similar to StringPathRelativeToTop except that assumes the string is a single path
// containing at most one instance of the temporary build directory at the start of the path while
// this assumes that there can be any number at any position.
func StringRelativeToTop(config Config, command string) string {
	return normalizeStringRelativeToTop(config, command)
}

// StringsRelativeToTop will return a new slice such that each item in the new slice is the result
// of calling StringRelativeToTop on the corresponding item in the input slice.
func StringsRelativeToTop(config Config, command []string) []string {
	return normalizeStringArrayRelativeToTop(config, command)
}

func EnsureListContainsSuffix(t *testing.T, result []string, expected string) {
	t.Helper()
	if !SuffixInList(result, expected) {
		t.Errorf("%q is not found in %v", expected, result)
	}
}
