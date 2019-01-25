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
		Context:      blueprint.NewContext(),
		NameResolver: nameResolver,
	}

	ctx.SetNameInterface(nameResolver)

	return ctx
}

func NewTestArchContext() *TestContext {
	ctx := NewTestContext()
	ctx.preDeps = append(ctx.preDeps, registerArchMutator)
	return ctx
}

type TestContext struct {
	*blueprint.Context
	preArch, preDeps, postDeps []RegisterMutatorFunc
	NameResolver               *NameResolver
}

func (ctx *TestContext) PreArchMutators(f RegisterMutatorFunc) {
	ctx.preArch = append(ctx.preArch, f)
}

func (ctx *TestContext) PreDepsMutators(f RegisterMutatorFunc) {
	ctx.preDeps = append(ctx.preDeps, f)
}

func (ctx *TestContext) PostDepsMutators(f RegisterMutatorFunc) {
	ctx.postDeps = append(ctx.postDeps, f)
}

func (ctx *TestContext) Register() {
	registerMutators(ctx.Context, ctx.preArch, ctx.preDeps, ctx.postDeps)

	ctx.RegisterSingletonType("env", SingletonFactoryAdaptor(EnvSingleton))
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

// MockFileSystem causes the Context to replace all reads with accesses to the provided map of
// filenames to contents stored as a byte slice.
func (ctx *TestContext) MockFileSystem(files map[string][]byte) {
	// no module list file specified; find every file named Blueprints or Android.bp
	pathsToParse := []string{}
	for candidate := range files {
		base := filepath.Base(candidate)
		if base == "Blueprints" || base == "Android.bp" {
			pathsToParse = append(pathsToParse, candidate)
		}
	}
	if len(pathsToParse) < 1 {
		panic(fmt.Sprintf("No Blueprint or Android.bp files found in mock filesystem: %v\n", files))
	}
	files[blueprint.MockModuleListFile] = []byte(strings.Join(pathsToParse, "\n"))

	ctx.Context.MockFileSystem(files)
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
func (m TestingModule) MaybeRule(rule string) BuildParams {
	for _, p := range m.module.BuildParamsForTests() {
		if strings.Contains(p.Rule.String(), rule) {
			return p
		}
	}
	return BuildParams{}
}

// Rule finds a call to ctx.Build with BuildParams.Rule set to a rule with the given name.  Panics if no rule is found.
func (m TestingModule) Rule(rule string) BuildParams {
	p := m.MaybeRule(rule)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find rule %q", rule))
	}
	return p
}

// MaybeDescription finds a call to ctx.Build with BuildParams.Description set to a the given string.  Returns an empty
// BuildParams if no rule is found.
func (m TestingModule) MaybeDescription(desc string) BuildParams {
	for _, p := range m.module.BuildParamsForTests() {
		if p.Description == desc {
			return p
		}
	}
	return BuildParams{}
}

// Description finds a call to ctx.Build with BuildParams.Description set to a the given string.  Panics if no rule is
// found.
func (m TestingModule) Description(desc string) BuildParams {
	p := m.MaybeDescription(desc)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find description %q", desc))
	}
	return p
}

func (m TestingModule) maybeOutput(file string) (BuildParams, []string) {
	var searchedOutputs []string
	for _, p := range m.module.BuildParamsForTests() {
		outputs := append(WritablePaths(nil), p.Outputs...)
		if p.Output != nil {
			outputs = append(outputs, p.Output)
		}
		for _, f := range outputs {
			if f.String() == file || f.Rel() == file {
				return p, nil
			}
			searchedOutputs = append(searchedOutputs, f.Rel())
		}
	}
	return BuildParams{}, searchedOutputs
}

// MaybeOutput finds a call to ctx.Build with a BuildParams.Output or BuildParams.Outputs whose String() or Rel()
// value matches the provided string.  Returns an empty BuildParams if no rule is found.
func (m TestingModule) MaybeOutput(file string) BuildParams {
	p, _ := m.maybeOutput(file)
	return p
}

// Output finds a call to ctx.Build with a BuildParams.Output or BuildParams.Outputs whose String() or Rel()
// value matches the provided string.  Panics if no rule is found.
func (m TestingModule) Output(file string) BuildParams {
	p, searchedOutputs := m.maybeOutput(file)
	if p.Rule == nil {
		panic(fmt.Errorf("couldn't find output %q.\nall outputs: %v",
			file, searchedOutputs))
	}
	return p
}

// AllOutputs returns all 'BuildParams.Output's and 'BuildParams.Outputs's in their full path string forms.
func (m TestingModule) AllOutputs() []string {
	var outputFullPaths []string
	for _, p := range m.module.BuildParamsForTests() {
		outputs := append(WritablePaths(nil), p.Outputs...)
		if p.Output != nil {
			outputs = append(outputs, p.Output)
		}
		outputFullPaths = append(outputFullPaths, outputs.Strings()...)
	}
	return outputFullPaths
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
			t.Errorf("errs[%d] = %s", i, err)
		}
	}
}
