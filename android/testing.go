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
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/blueprint"
)

func NewTestContext(config Config) *TestContext {
	namespaceExportFilter := func(namespace *Namespace) bool {
		return true
	}

	nameResolver := NewNameResolver(namespaceExportFilter)
	ctx := &TestContext{
		Context:      &Context{blueprint.NewContext(), config},
		NameResolver: nameResolver,
	}

	ctx.SetNameInterface(nameResolver)

	ctx.postDeps = append(ctx.postDeps, registerPathDepsMutator)

	ctx.SetFs(ctx.config.fs)
	if ctx.config.mockBpList != "" {
		ctx.SetModuleListFile(ctx.config.mockBpList)
	}

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

// Prepares an integration test with build components from the android package.
var PrepareForIntegrationTestWithAndroid = GroupFixturePreparers(
	// Mutators. Must match order in mutator.go.
	PrepareForTestWithArchMutator,
	PrepareForTestWithDefaults,
	PrepareForTestWithComponentsMutator,
	PrepareForTestWithPrebuilts,
	PrepareForTestWithOverrides,

	// Modules
	PrepareForTestWithFilegroup,
)

func NewTestArchContext(config Config) *TestContext {
	ctx := NewTestContext(config)
	ctx.preDeps = append(ctx.preDeps, registerArchMutator)
	return ctx
}

type TestContext struct {
	*Context
	preArch, preDeps, postDeps, finalDeps           []RegisterMutatorFunc
	bp2buildPreArch, bp2buildDeps, bp2buildMutators []RegisterMutatorFunc
	NameResolver                                    *NameResolver

	// The list of pre-singletons and singletons registered for the test.
	preSingletons, singletons sortableComponents

	// The order in which the pre-singletons, mutators and singletons will be run in this test
	// context; for debugging.
	preSingletonOrder, mutatorOrder, singletonOrder []string
}

func (ctx *TestContext) PreArchMutators(f RegisterMutatorFunc) {
	ctx.preArch = append(ctx.preArch, f)
}

func (ctx *TestContext) HardCodedPreArchMutators(f RegisterMutatorFunc) {
	// Register mutator function as normal for testing.
	ctx.PreArchMutators(f)
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

// RegisterBp2BuildMutator registers a BazelTargetModule mutator for converting a module
// type to the equivalent Bazel target.
func (ctx *TestContext) RegisterBp2BuildMutator(moduleType string, m func(TopDownMutatorContext)) {
	f := func(ctx RegisterMutatorsContext) {
		ctx.TopDown(moduleType, m)
	}
	ctx.bp2buildMutators = append(ctx.bp2buildMutators, f)
}

// PreArchBp2BuildMutators adds mutators to be register for converting Android Blueprint modules
// into Bazel BUILD targets that should run prior to deps and conversion.
func (ctx *TestContext) PreArchBp2BuildMutators(f RegisterMutatorFunc) {
	ctx.bp2buildPreArch = append(ctx.bp2buildPreArch, f)
}

// DepsBp2BuildMutators adds mutators to be register for converting Android Blueprint modules into
// Bazel BUILD targets that should run prior to conversion to resolve dependencies.
func (ctx *TestContext) DepsBp2BuildMutators(f RegisterMutatorFunc) {
	ctx.bp2buildDeps = append(ctx.bp2buildDeps, f)
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

	// The order of pre-singletons
	preSingletonOrder registeredComponentOrder

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
		// Create an ordering from the globally registered pre-singletons.
		s.preSingletonOrder = registeredComponentOrderFromExistingOrder("pre-singleton", preSingletons)

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

	// Ensure that the pre-singletons used in the test are in the same order as they are used at
	// runtime.
	globalOrder.preSingletonOrder.enforceOrdering(ctx.preSingletons)
	ctx.preSingletons.registerAll(ctx.Context)

	mutators := collateRegisteredMutators(ctx.preArch, ctx.preDeps, ctx.postDeps, ctx.finalDeps)
	// Ensure that the mutators used in the test are in the same order as they are used at runtime.
	globalOrder.mutatorOrder.enforceOrdering(mutators)
	mutators.registerAll(ctx.Context)

	// Register the env singleton with this context before sorting.
	ctx.RegisterSingletonType("env", EnvSingleton)

	// Ensure that the singletons used in the test are in the same order as they are used at runtime.
	globalOrder.singletonOrder.enforceOrdering(ctx.singletons)
	ctx.singletons.registerAll(ctx.Context)

	// Save the sorted components order away to make them easy to access while debugging.
	ctx.preSingletonOrder = componentsToNames(preSingletons)
	ctx.mutatorOrder = componentsToNames(mutators)
	ctx.singletonOrder = componentsToNames(singletons)
}

// RegisterForBazelConversion prepares a test context for bp2build conversion.
func (ctx *TestContext) RegisterForBazelConversion() {
	RegisterMutatorsForBazelConversion(ctx.Context, ctx.bp2buildPreArch, ctx.bp2buildDeps, ctx.bp2buildMutators)
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

func (ctx *TestContext) RegisterSingletonType(name string, factory SingletonFactory) {
	ctx.singletons = append(ctx.singletons, newSingleton(name, factory))
}

func (ctx *TestContext) RegisterPreSingletonType(name string, factory SingletonFactory) {
	ctx.preSingletons = append(ctx.preSingletons, newPreSingleton(name, factory))
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
		sort.Strings(allModuleNames)
		sort.Strings(allVariants)

		if len(allVariants) == 0 {
			panic(fmt.Errorf("failed to find module %q. All modules:\n  %s",
				name, strings.Join(allModuleNames, "\n  ")))
		} else {
			panic(fmt.Errorf("failed to find module %q variant %q. All variants:\n  %s",
				name, variant, strings.Join(allVariants, "\n  ")))
		}
	}

	return TestingModule{module}
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
				singleton: s.(*singletonAdaptor).Singleton,
				provider:  s.(testBuildProvider),
			}
		}
		allSingletonNames = append(allSingletonNames, n)
	}

	panic(fmt.Errorf("failed to find singleton %q."+
		"\nall singletons: %v", name, allSingletonNames))
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
}

func newTestingBuildParams(provider testBuildProvider, bparams BuildParams) TestingBuildParams {
	return TestingBuildParams{
		BuildParams: bparams,
		RuleParams:  provider.RuleParamsForTests()[bparams.Rule],
	}
}

func maybeBuildParamsFromRule(provider testBuildProvider, rule string) (TestingBuildParams, []string) {
	var searchedRules []string
	for _, p := range provider.BuildParamsForTests() {
		searchedRules = append(searchedRules, p.Rule.String())
		if strings.Contains(p.Rule.String(), rule) {
			return newTestingBuildParams(provider, p), searchedRules
		}
	}
	return TestingBuildParams{}, searchedRules
}

func buildParamsFromRule(provider testBuildProvider, rule string) TestingBuildParams {
	p, searchRules := maybeBuildParamsFromRule(provider, rule)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find rule %q.\nall rules: %v", rule, searchRules))
	}
	return p
}

func maybeBuildParamsFromDescription(provider testBuildProvider, desc string) TestingBuildParams {
	for _, p := range provider.BuildParamsForTests() {
		if strings.Contains(p.Description, desc) {
			return newTestingBuildParams(provider, p)
		}
	}
	return TestingBuildParams{}
}

func buildParamsFromDescription(provider testBuildProvider, desc string) TestingBuildParams {
	p := maybeBuildParamsFromDescription(provider, desc)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find description %q", desc))
	}
	return p
}

func maybeBuildParamsFromOutput(provider testBuildProvider, file string) (TestingBuildParams, []string) {
	var searchedOutputs []string
	for _, p := range provider.BuildParamsForTests() {
		outputs := append(WritablePaths(nil), p.Outputs...)
		outputs = append(outputs, p.ImplicitOutputs...)
		if p.Output != nil {
			outputs = append(outputs, p.Output)
		}
		for _, f := range outputs {
			if f.String() == file || f.Rel() == file {
				return newTestingBuildParams(provider, p), nil
			}
			searchedOutputs = append(searchedOutputs, f.Rel())
		}
	}
	return TestingBuildParams{}, searchedOutputs
}

func buildParamsFromOutput(provider testBuildProvider, file string) TestingBuildParams {
	p, searchedOutputs := maybeBuildParamsFromOutput(provider, file)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find output %q.\nall outputs: %v",
			file, searchedOutputs))
	}
	return p
}

func allOutputs(provider testBuildProvider) []string {
	var outputFullPaths []string
	for _, p := range provider.BuildParamsForTests() {
		outputs := append(WritablePaths(nil), p.Outputs...)
		outputs = append(outputs, p.ImplicitOutputs...)
		if p.Output != nil {
			outputs = append(outputs, p.Output)
		}
		outputFullPaths = append(outputFullPaths, outputs.Strings()...)
	}
	return outputFullPaths
}

// TestingModule is wrapper around an android.Module that provides methods to find information about individual
// ctx.Build parameters for verification in tests.
type TestingModule struct {
	module Module
}

// Module returns the Module wrapped by the TestingModule.
func (m TestingModule) Module() Module {
	return m.module
}

// MaybeRule finds a call to ctx.Build with BuildParams.Rule set to a rule with the given name.  Returns an empty
// BuildParams if no rule is found.
func (m TestingModule) MaybeRule(rule string) TestingBuildParams {
	r, _ := maybeBuildParamsFromRule(m.module, rule)
	return r
}

// Rule finds a call to ctx.Build with BuildParams.Rule set to a rule with the given name.  Panics if no rule is found.
func (m TestingModule) Rule(rule string) TestingBuildParams {
	return buildParamsFromRule(m.module, rule)
}

// MaybeDescription finds a call to ctx.Build with BuildParams.Description set to a the given string.  Returns an empty
// BuildParams if no rule is found.
func (m TestingModule) MaybeDescription(desc string) TestingBuildParams {
	return maybeBuildParamsFromDescription(m.module, desc)
}

// Description finds a call to ctx.Build with BuildParams.Description set to a the given string.  Panics if no rule is
// found.
func (m TestingModule) Description(desc string) TestingBuildParams {
	return buildParamsFromDescription(m.module, desc)
}

// MaybeOutput finds a call to ctx.Build with a BuildParams.Output or BuildParams.Outputs whose String() or Rel()
// value matches the provided string.  Returns an empty BuildParams if no rule is found.
func (m TestingModule) MaybeOutput(file string) TestingBuildParams {
	p, _ := maybeBuildParamsFromOutput(m.module, file)
	return p
}

// Output finds a call to ctx.Build with a BuildParams.Output or BuildParams.Outputs whose String() or Rel()
// value matches the provided string.  Panics if no rule is found.
func (m TestingModule) Output(file string) TestingBuildParams {
	return buildParamsFromOutput(m.module, file)
}

// AllOutputs returns all 'BuildParams.Output's and 'BuildParams.Outputs's in their full path string forms.
func (m TestingModule) AllOutputs() []string {
	return allOutputs(m.module)
}

// TestingSingleton is wrapper around an android.Singleton that provides methods to find information about individual
// ctx.Build parameters for verification in tests.
type TestingSingleton struct {
	singleton Singleton
	provider  testBuildProvider
}

// Singleton returns the Singleton wrapped by the TestingSingleton.
func (s TestingSingleton) Singleton() Singleton {
	return s.singleton
}

// MaybeRule finds a call to ctx.Build with BuildParams.Rule set to a rule with the given name.  Returns an empty
// BuildParams if no rule is found.
func (s TestingSingleton) MaybeRule(rule string) TestingBuildParams {
	r, _ := maybeBuildParamsFromRule(s.provider, rule)
	return r
}

// Rule finds a call to ctx.Build with BuildParams.Rule set to a rule with the given name.  Panics if no rule is found.
func (s TestingSingleton) Rule(rule string) TestingBuildParams {
	return buildParamsFromRule(s.provider, rule)
}

// MaybeDescription finds a call to ctx.Build with BuildParams.Description set to a the given string.  Returns an empty
// BuildParams if no rule is found.
func (s TestingSingleton) MaybeDescription(desc string) TestingBuildParams {
	return maybeBuildParamsFromDescription(s.provider, desc)
}

// Description finds a call to ctx.Build with BuildParams.Description set to a the given string.  Panics if no rule is
// found.
func (s TestingSingleton) Description(desc string) TestingBuildParams {
	return buildParamsFromDescription(s.provider, desc)
}

// MaybeOutput finds a call to ctx.Build with a BuildParams.Output or BuildParams.Outputs whose String() or Rel()
// value matches the provided string.  Returns an empty BuildParams if no rule is found.
func (s TestingSingleton) MaybeOutput(file string) TestingBuildParams {
	p, _ := maybeBuildParamsFromOutput(s.provider, file)
	return p
}

// Output finds a call to ctx.Build with a BuildParams.Output or BuildParams.Outputs whose String() or Rel()
// value matches the provided string.  Panics if no rule is found.
func (s TestingSingleton) Output(file string) TestingBuildParams {
	return buildParamsFromOutput(s.provider, file)
}

// AllOutputs returns all 'BuildParams.Output's and 'BuildParams.Outputs's in their full path string forms.
func (s TestingSingleton) AllOutputs() []string {
	return allOutputs(s.provider)
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
		t.Errorf("missing the expected error %q (checked %d error(s))", pattern, len(errs))
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

func AndroidMkEntriesForTest(t *testing.T, ctx *TestContext, mod blueprint.Module) []AndroidMkEntries {
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
	var p AndroidMkDataProvider
	var ok bool
	if p, ok = mod.(AndroidMkDataProvider); !ok {
		t.Errorf("module does not implement AndroidMkDataProvider: " + mod.Name())
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
func NormalizePathForTesting(path Path) string {
	if path == nil {
		return "<nil path>"
	}
	p := path.String()
	if w, ok := path.(WritablePath); ok {
		rel, err := filepath.Rel(w.buildDir(), p)
		if err != nil {
			panic(err)
		}
		return rel
	}
	return p
}

func NormalizePathsForTesting(paths Paths) []string {
	var result []string
	for _, path := range paths {
		relative := NormalizePathForTesting(path)
		result = append(result, relative)
	}
	return result
}
