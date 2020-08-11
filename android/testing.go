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
	"strings"
	"testing"

	"github.com/google/blueprint"
)

func NewTestContext() *TestContext {
	namespaceExportFilter := func(namespace *Namespace) bool {
		return true
	}

	nameResolver := NewNameResolver(namespaceExportFilter)
	ctx := &TestContext{
		Context:      &Context{blueprint.NewContext()},
		NameResolver: nameResolver,
	}

	ctx.SetNameInterface(nameResolver)

	ctx.postDeps = append(ctx.postDeps, registerPathDepsMutator)

	return ctx
}

func NewTestArchContext() *TestContext {
	ctx := NewTestContext()
	ctx.preDeps = append(ctx.preDeps, registerArchMutator)
	return ctx
}

type TestContext struct {
	*Context
	preArch, preDeps, postDeps, finalDeps []RegisterMutatorFunc
	NameResolver                          *NameResolver
	config                                Config
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

func (ctx *TestContext) Register(config Config) {
	ctx.SetFs(config.fs)
	if config.mockBpList != "" {
		ctx.SetModuleListFile(config.mockBpList)
	}
	registerMutators(ctx.Context.Context, ctx.preArch, ctx.preDeps, ctx.postDeps, ctx.finalDeps)

	ctx.RegisterSingletonType("env", EnvSingleton)

	ctx.config = config
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

func (ctx *TestContext) RegisterSingletonType(name string, factory SingletonFactory) {
	ctx.Context.RegisterSingletonType(name, SingletonFactoryAdaptor(factory))
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
		allModuleNames := []string{}
		ctx.VisitAllModules(func(m blueprint.Module) {
			allModuleNames = append(allModuleNames, m.(Module).Name()+"("+ctx.ModuleSubDir(m)+")")
		})

		panic(fmt.Errorf("failed to find module %q variant %q."+
			"\nall modules: %v", name, variant, allModuleNames))
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

func maybeBuildParamsFromRule(provider testBuildProvider, rule string) TestingBuildParams {
	for _, p := range provider.BuildParamsForTests() {
		if strings.Contains(p.Rule.String(), rule) {
			return newTestingBuildParams(provider, p)
		}
	}
	return TestingBuildParams{}
}

func buildParamsFromRule(provider testBuildProvider, rule string) TestingBuildParams {
	p := maybeBuildParamsFromRule(provider, rule)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find rule %q", rule))
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
	return maybeBuildParamsFromRule(m.module, rule)
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
	return maybeBuildParamsFromRule(s.provider, rule)
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

func FailIfNoMatchingErrors(t *testing.T, pattern string, errs []error) {
	t.Helper()

	matcher, err := regexp.Compile(pattern)
	if err != nil {
		t.Errorf("failed to compile regular expression %q because %s", pattern, err)
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
		}
	}

}

func SetInMakeForTests(config Config) {
	config.inMake = true
}

func AndroidMkEntriesForTest(t *testing.T, config Config, bpPath string, mod blueprint.Module) []AndroidMkEntries {
	var p AndroidMkEntriesProvider
	var ok bool
	if p, ok = mod.(AndroidMkEntriesProvider); !ok {
		t.Errorf("module does not implement AndroidMkEntriesProvider: " + mod.Name())
	}

	entriesList := p.AndroidMkEntries()
	for i, _ := range entriesList {
		entriesList[i].fillInEntries(config, bpPath, mod)
	}
	return entriesList
}

func AndroidMkDataForTest(t *testing.T, config Config, bpPath string, mod blueprint.Module) AndroidMkData {
	var p AndroidMkDataProvider
	var ok bool
	if p, ok = mod.(AndroidMkDataProvider); !ok {
		t.Errorf("module does not implement AndroidMkDataProvider: " + mod.Name())
	}
	data := p.AndroidMk()
	data.fillInData(config, bpPath, mod)
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
