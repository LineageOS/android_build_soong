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
	"strings"

	"github.com/google/blueprint"
)

func NewTestContext() *TestContext {
	ctx := &TestContext{
		Context: blueprint.NewContext(),
	}

	namespaceExportFilter := func(namespace *Namespace) bool {
		return true
	}
	ctx.SetNameInterface(NewNameResolver(namespaceExportFilter))

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

type TestingModule struct {
	module Module
}

func (m TestingModule) Module() Module {
	return m.module
}

func (m TestingModule) Rule(rule string) BuildParams {
	for _, p := range m.module.BuildParamsForTests() {
		if strings.Contains(p.Rule.String(), rule) {
			return p
		}
	}
	panic(fmt.Errorf("couldn't find rule %q", rule))
}

func (m TestingModule) Description(desc string) BuildParams {
	for _, p := range m.module.BuildParamsForTests() {
		if p.Description == desc {
			return p
		}
	}
	panic(fmt.Errorf("couldn't find description %q", desc))
}

func (m TestingModule) Output(file string) BuildParams {
	for _, p := range m.module.BuildParamsForTests() {
		outputs := append(WritablePaths(nil), p.Outputs...)
		if p.Output != nil {
			outputs = append(outputs, p.Output)
		}
		for _, f := range outputs {
			if f.Rel() == file {
				return p
			}
		}
	}
	panic(fmt.Errorf("couldn't find output %q", file))
}
