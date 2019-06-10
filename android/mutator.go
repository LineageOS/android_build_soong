// Copyright 2015 Google Inc. All rights reserved.
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
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// Phases:
//   run Pre-arch mutators
//   run archMutator
//   run Pre-deps mutators
//   run depsMutator
//   run PostDeps mutators
//   continue on to GenerateAndroidBuildActions

func registerMutatorsToContext(ctx *blueprint.Context, mutators []*mutator) {
	for _, t := range mutators {
		var handle blueprint.MutatorHandle
		if t.bottomUpMutator != nil {
			handle = ctx.RegisterBottomUpMutator(t.name, t.bottomUpMutator)
		} else if t.topDownMutator != nil {
			handle = ctx.RegisterTopDownMutator(t.name, t.topDownMutator)
		}
		if t.parallel {
			handle.Parallel()
		}
	}
}

func registerMutators(ctx *blueprint.Context, preArch, preDeps, postDeps []RegisterMutatorFunc) {
	mctx := &registerMutatorsContext{}

	register := func(funcs []RegisterMutatorFunc) {
		for _, f := range funcs {
			f(mctx)
		}
	}

	register(preArch)

	register(preDeps)

	mctx.BottomUp("deps", depsMutator).Parallel()

	register(postDeps)

	registerMutatorsToContext(ctx, mctx.mutators)
}

type registerMutatorsContext struct {
	mutators []*mutator
}

type RegisterMutatorsContext interface {
	TopDown(name string, m TopDownMutator) MutatorHandle
	BottomUp(name string, m BottomUpMutator) MutatorHandle
}

type RegisterMutatorFunc func(RegisterMutatorsContext)

var preArch = []RegisterMutatorFunc{
	registerLoadHookMutator,
	RegisterNamespaceMutator,
	RegisterPrebuiltsPreArchMutators,
	registerVisibilityRuleChecker,
	RegisterDefaultsPreArchMutators,
	registerVisibilityRuleGatherer,
}

func registerArchMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("arch", archMutator).Parallel()
	ctx.TopDown("arch_hooks", archHookMutator).Parallel()
}

var preDeps = []RegisterMutatorFunc{
	registerArchMutator,
}

var postDeps = []RegisterMutatorFunc{
	registerPathDepsMutator,
	RegisterPrebuiltsPostDepsMutators,
	registerVisibilityRuleEnforcer,
	registerNeverallowMutator,
	RegisterOverridePostDepsMutators,
}

func PreArchMutators(f RegisterMutatorFunc) {
	preArch = append(preArch, f)
}

func PreDepsMutators(f RegisterMutatorFunc) {
	preDeps = append(preDeps, f)
}

func PostDepsMutators(f RegisterMutatorFunc) {
	postDeps = append(postDeps, f)
}

type TopDownMutator func(TopDownMutatorContext)

type TopDownMutatorContext interface {
	BaseModuleContext

	OtherModuleExists(name string) bool
	Rename(name string)
	Module() Module

	OtherModuleName(m blueprint.Module) string
	OtherModuleDir(m blueprint.Module) string
	OtherModuleErrorf(m blueprint.Module, fmt string, args ...interface{})
	OtherModuleDependencyTag(m blueprint.Module) blueprint.DependencyTag

	CreateModule(blueprint.ModuleFactory, ...interface{})

	GetDirectDepWithTag(name string, tag blueprint.DependencyTag) blueprint.Module
	GetDirectDep(name string) (blueprint.Module, blueprint.DependencyTag)

	VisitDirectDeps(visit func(Module))
	VisitDirectDepsWithTag(tag blueprint.DependencyTag, visit func(Module))
	VisitDirectDepsIf(pred func(Module) bool, visit func(Module))
	VisitDepsDepthFirst(visit func(Module))
	VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module))
	WalkDeps(visit func(Module, Module) bool)
	// GetWalkPath is supposed to be called in visit function passed in WalkDeps()
	// and returns a top-down dependency path from a start module to current child module.
	GetWalkPath() []Module
}

type topDownMutatorContext struct {
	blueprint.TopDownMutatorContext
	baseModuleContext
	walkPath []Module
}

type BottomUpMutator func(BottomUpMutatorContext)

type BottomUpMutatorContext interface {
	BaseModuleContext

	OtherModuleExists(name string) bool
	Rename(name string)
	Module() blueprint.Module

	AddDependency(module blueprint.Module, tag blueprint.DependencyTag, name ...string)
	AddReverseDependency(module blueprint.Module, tag blueprint.DependencyTag, name string)
	CreateVariations(...string) []blueprint.Module
	CreateLocalVariations(...string) []blueprint.Module
	SetDependencyVariation(string)
	AddVariationDependencies([]blueprint.Variation, blueprint.DependencyTag, ...string)
	AddFarVariationDependencies([]blueprint.Variation, blueprint.DependencyTag, ...string)
	AddInterVariantDependency(tag blueprint.DependencyTag, from, to blueprint.Module)
	ReplaceDependencies(string)
}

type bottomUpMutatorContext struct {
	blueprint.BottomUpMutatorContext
	baseModuleContext
}

func (x *registerMutatorsContext) BottomUp(name string, m BottomUpMutator) MutatorHandle {
	f := func(ctx blueprint.BottomUpMutatorContext) {
		if a, ok := ctx.Module().(Module); ok {
			actx := &bottomUpMutatorContext{
				BottomUpMutatorContext: ctx,
				baseModuleContext:      a.base().baseModuleContextFactory(ctx),
			}
			m(actx)
		}
	}
	mutator := &mutator{name: name, bottomUpMutator: f}
	x.mutators = append(x.mutators, mutator)
	return mutator
}

func (x *registerMutatorsContext) TopDown(name string, m TopDownMutator) MutatorHandle {
	f := func(ctx blueprint.TopDownMutatorContext) {
		if a, ok := ctx.Module().(Module); ok {
			actx := &topDownMutatorContext{
				TopDownMutatorContext: ctx,
				baseModuleContext:     a.base().baseModuleContextFactory(ctx),
			}
			m(actx)
		}
	}
	mutator := &mutator{name: name, topDownMutator: f}
	x.mutators = append(x.mutators, mutator)
	return mutator
}

type MutatorHandle interface {
	Parallel() MutatorHandle
}

func (mutator *mutator) Parallel() MutatorHandle {
	mutator.parallel = true
	return mutator
}

func depsMutator(ctx BottomUpMutatorContext) {
	if m, ok := ctx.Module().(Module); ok && m.Enabled() {
		m.DepsMutator(ctx)
	}
}

func (t *topDownMutatorContext) Config() Config {
	return t.config
}

func (b *bottomUpMutatorContext) Config() Config {
	return b.config
}

func (t *topDownMutatorContext) Module() Module {
	module, _ := t.TopDownMutatorContext.Module().(Module)
	return module
}

func (t *topDownMutatorContext) VisitDirectDeps(visit func(Module)) {
	t.TopDownMutatorContext.VisitDirectDeps(func(module blueprint.Module) {
		if aModule, _ := module.(Module); aModule != nil {
			visit(aModule)
		}
	})
}

func (t *topDownMutatorContext) VisitDirectDepsWithTag(tag blueprint.DependencyTag, visit func(Module)) {
	t.TopDownMutatorContext.VisitDirectDeps(func(module blueprint.Module) {
		if aModule, _ := module.(Module); aModule != nil {
			if t.TopDownMutatorContext.OtherModuleDependencyTag(aModule) == tag {
				visit(aModule)
			}
		}
	})
}

func (t *topDownMutatorContext) VisitDirectDepsIf(pred func(Module) bool, visit func(Module)) {
	t.TopDownMutatorContext.VisitDirectDepsIf(
		// pred
		func(module blueprint.Module) bool {
			if aModule, _ := module.(Module); aModule != nil {
				return pred(aModule)
			} else {
				return false
			}
		},
		// visit
		func(module blueprint.Module) {
			visit(module.(Module))
		})
}

func (t *topDownMutatorContext) VisitDepsDepthFirst(visit func(Module)) {
	t.TopDownMutatorContext.VisitDepsDepthFirst(func(module blueprint.Module) {
		if aModule, _ := module.(Module); aModule != nil {
			visit(aModule)
		}
	})
}

func (t *topDownMutatorContext) VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module)) {
	t.TopDownMutatorContext.VisitDepsDepthFirstIf(
		// pred
		func(module blueprint.Module) bool {
			if aModule, _ := module.(Module); aModule != nil {
				return pred(aModule)
			} else {
				return false
			}
		},
		// visit
		func(module blueprint.Module) {
			visit(module.(Module))
		})
}

func (t *topDownMutatorContext) WalkDeps(visit func(Module, Module) bool) {
	t.walkPath = []Module{t.Module()}
	t.TopDownMutatorContext.WalkDeps(func(child, parent blueprint.Module) bool {
		childAndroidModule, _ := child.(Module)
		parentAndroidModule, _ := parent.(Module)
		if childAndroidModule != nil && parentAndroidModule != nil {
			// record walkPath before visit
			for t.walkPath[len(t.walkPath)-1] != parentAndroidModule {
				t.walkPath = t.walkPath[0 : len(t.walkPath)-1]
			}
			t.walkPath = append(t.walkPath, childAndroidModule)
			return visit(childAndroidModule, parentAndroidModule)
		} else {
			return false
		}
	})
}

func (t *topDownMutatorContext) GetWalkPath() []Module {
	return t.walkPath
}

func (t *topDownMutatorContext) AppendProperties(props ...interface{}) {
	for _, p := range props {
		err := proptools.AppendMatchingProperties(t.Module().base().customizableProperties,
			p, nil)
		if err != nil {
			if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
				t.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
			} else {
				panic(err)
			}
		}
	}
}

func (t *topDownMutatorContext) PrependProperties(props ...interface{}) {
	for _, p := range props {
		err := proptools.PrependMatchingProperties(t.Module().base().customizableProperties,
			p, nil)
		if err != nil {
			if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
				t.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
			} else {
				panic(err)
			}
		}
	}
}
