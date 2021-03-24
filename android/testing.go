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
	"github.com/google/blueprint/proptools"
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
//   * Args
//   * Path instances are intentionally not modified, use AssertPathRelativeToTopEquals or
//     AssertPathsRelativeToTopEquals instead which do something similar.
//
// * RuleParams
//   * Command
//   * Depfile
//   * Rspfile
//   * RspfileContent
//   * SymlinkOutputs
//   * CommandDeps
//   * CommandOrderOnly
//
// See PathRelativeToTop for more details.
func (p TestingBuildParams) RelativeToTop() TestingBuildParams {
	// If this is not a valid params then just return it back. That will make it easy to use with the
	// Maybe...() methods.
	if p.Rule == nil {
		return p
	}
	if p.config.config == nil {
		panic("cannot call RelativeToTop() on a TestingBuildParams previously returned by RelativeToTop()")
	}
	// Take a copy of the build params and replace any args that contains test specific temporary
	// paths with paths relative to the top.
	bparams := p.BuildParams
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
	// The buildDir usually looks something like: /tmp/testFoo2345/001
	//
	// Replace any usage of the buildDir with out/soong, e.g. replace "/tmp/testFoo2345/001" with
	// "out/soong".
	outSoongDir := filepath.Clean(config.buildDir)
	re := regexp.MustCompile(`\Q` + outSoongDir + `\E\b`)
	s = re.ReplaceAllString(s, "out/soong")

	// Replace any usage of the buildDir/.. with out, e.g. replace "/tmp/testFoo2345" with
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
	}
}

func (b baseTestingComponent) maybeBuildParamsFromRule(rule string) (TestingBuildParams, []string) {
	var searchedRules []string
	for _, p := range b.provider.BuildParamsForTests() {
		searchedRules = append(searchedRules, p.Rule.String())
		if strings.Contains(p.Rule.String(), rule) {
			return b.newTestingBuildParams(p), searchedRules
		}
	}
	return TestingBuildParams{}, searchedRules
}

func (b baseTestingComponent) buildParamsFromRule(rule string) TestingBuildParams {
	p, searchRules := b.maybeBuildParamsFromRule(rule)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find rule %q.\nall rules: %v", rule, searchRules))
	}
	return p
}

func (b baseTestingComponent) maybeBuildParamsFromDescription(desc string) TestingBuildParams {
	for _, p := range b.provider.BuildParamsForTests() {
		if strings.Contains(p.Description, desc) {
			return b.newTestingBuildParams(p)
		}
	}
	return TestingBuildParams{}
}

func (b baseTestingComponent) buildParamsFromDescription(desc string) TestingBuildParams {
	p := b.maybeBuildParamsFromDescription(desc)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find description %q", desc))
	}
	return p
}

func (b baseTestingComponent) maybeBuildParamsFromOutput(file string) (TestingBuildParams, []string) {
	var searchedOutputs []string
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
			searchedOutputs = append(searchedOutputs, f.Rel())
		}
	}
	return TestingBuildParams{}, searchedOutputs
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
		outputs := append(WritablePaths(nil), p.Outputs...)
		outputs = append(outputs, p.ImplicitOutputs...)
		if p.Output != nil {
			outputs = append(outputs, p.Output)
		}
		outputFullPaths = append(outputFullPaths, outputs.Strings()...)
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
	return b.maybeBuildParamsFromDescription(desc)
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
//
// deprecated: use PathRelativeToTop instead as it handles make install paths and differentiates
// between output and source properly.
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
// For a WritablePath it applies StringPathRelativeToTop to it, using the buildDir returned from the
// WritablePath's buildDir() method. For all other paths, i.e. source paths, that are already
// relative to the top it just returns their string representation.
func PathRelativeToTop(path Path) string {
	if path == nil {
		return "<nil path>"
	}
	p := path.String()
	if w, ok := path.(WritablePath); ok {
		buildDir := w.buildDir()
		return StringPathRelativeToTop(buildDir, p)
	}
	return p
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
// A standard build has the following structure:
//   ../top/
//          out/ - make install files go here.
//          out/soong - this is the buildDir passed to NewTestConfig()
//          ... - the source files
//
// This function converts a path so that it appears relative to the ../top/ directory, i.e.
// * Make install paths, which have the pattern "buildDir/../<path>" are converted into the top
//   relative path "out/<path>"
// * Soong install paths and other writable paths, which have the pattern "buildDir/<path>" are
//   converted into the top relative path "out/soong/<path>".
// * Source paths are already relative to the top.
//
// This is provided for processing paths that have already been converted into a string, e.g. paths
// in AndroidMkEntries structures. As a result it needs to be supplied the soong output dir against
// which it can try and relativize paths. PathRelativeToTop must be used for process Path objects.
func StringPathRelativeToTop(soongOutDir string, path string) string {

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
