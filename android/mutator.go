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
	"reflect"

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
	// Rename package module types.
	registerPackageRenamer,
	RegisterPrebuiltsPreArchMutators,
	registerVisibilityRuleChecker,
	RegisterDefaultsPreArchMutators,
	registerVisibilityRuleGatherer,
}

func registerArchMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("os", osMutator).Parallel()
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

	MutatorName() string

	Rename(name string)

	CreateModule(ModuleFactory, ...interface{}) Module
}

type topDownMutatorContext struct {
	bp blueprint.TopDownMutatorContext
	baseModuleContext
}

type BottomUpMutator func(BottomUpMutatorContext)

type BottomUpMutatorContext interface {
	BaseModuleContext

	MutatorName() string

	Rename(name string)

	AddDependency(module blueprint.Module, tag blueprint.DependencyTag, name ...string)
	AddReverseDependency(module blueprint.Module, tag blueprint.DependencyTag, name string)
	CreateVariations(...string) []blueprint.Module
	CreateLocalVariations(...string) []blueprint.Module
	SetDependencyVariation(string)
	SetDefaultDependencyVariation(*string)
	AddVariationDependencies([]blueprint.Variation, blueprint.DependencyTag, ...string)
	AddFarVariationDependencies([]blueprint.Variation, blueprint.DependencyTag, ...string)
	AddInterVariantDependency(tag blueprint.DependencyTag, from, to blueprint.Module)
	ReplaceDependencies(string)
	AliasVariation(variationName string)
}

type bottomUpMutatorContext struct {
	bp blueprint.BottomUpMutatorContext
	baseModuleContext
}

func (x *registerMutatorsContext) BottomUp(name string, m BottomUpMutator) MutatorHandle {
	f := func(ctx blueprint.BottomUpMutatorContext) {
		if a, ok := ctx.Module().(Module); ok {
			actx := &bottomUpMutatorContext{
				bp:                ctx,
				baseModuleContext: a.base().baseModuleContextFactory(ctx),
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
				bp:                ctx,
				baseModuleContext: a.base().baseModuleContextFactory(ctx),
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

// android.topDownMutatorContext either has to embed blueprint.TopDownMutatorContext, in which case every method that
// has an overridden version in android.BaseModuleContext has to be manually forwarded to BaseModuleContext to avoid
// ambiguous method errors, or it has to store a blueprint.TopDownMutatorContext non-embedded, in which case every
// non-overridden method has to be forwarded.  There are fewer non-overridden methods, so use the latter.  The following
// methods forward to the identical blueprint versions for topDownMutatorContext and bottomUpMutatorContext.

func (t *topDownMutatorContext) MutatorName() string {
	return t.bp.MutatorName()
}

func (t *topDownMutatorContext) Rename(name string) {
	t.bp.Rename(name)
	t.Module().base().commonProperties.DebugName = name
}

func (t *topDownMutatorContext) CreateModule(factory ModuleFactory, props ...interface{}) Module {
	inherited := []interface{}{&t.Module().base().commonProperties}
	module := t.bp.CreateModule(ModuleFactoryAdaptor(factory), append(inherited, props...)...).(Module)

	if t.Module().base().variableProperties != nil && module.base().variableProperties != nil {
		src := t.Module().base().variableProperties
		dst := []interface{}{
			module.base().variableProperties,
			// Put an empty copy of the src properties into dst so that properties in src that are not in dst
			// don't cause a "failed to find property to extend" error.
			proptools.CloneEmptyProperties(reflect.ValueOf(src).Elem()).Interface(),
		}
		err := proptools.AppendMatchingProperties(dst, src, nil)
		if err != nil {
			panic(err)
		}
	}

	return module
}

func (b *bottomUpMutatorContext) MutatorName() string {
	return b.bp.MutatorName()
}

func (b *bottomUpMutatorContext) Rename(name string) {
	b.bp.Rename(name)
	b.Module().base().commonProperties.DebugName = name
}

func (b *bottomUpMutatorContext) AddDependency(module blueprint.Module, tag blueprint.DependencyTag, name ...string) {
	b.bp.AddDependency(module, tag, name...)
}

func (b *bottomUpMutatorContext) AddReverseDependency(module blueprint.Module, tag blueprint.DependencyTag, name string) {
	b.bp.AddReverseDependency(module, tag, name)
}

func (b *bottomUpMutatorContext) CreateVariations(variations ...string) []blueprint.Module {
	modules := b.bp.CreateVariations(variations...)

	for i := range variations {
		base := modules[i].(Module).base()
		base.commonProperties.DebugMutators = append(base.commonProperties.DebugMutators, b.MutatorName())
		base.commonProperties.DebugVariations = append(base.commonProperties.DebugVariations, variations[i])
	}

	return modules
}

func (b *bottomUpMutatorContext) CreateLocalVariations(variations ...string) []blueprint.Module {
	modules := b.bp.CreateLocalVariations(variations...)

	for i := range variations {
		base := modules[i].(Module).base()
		base.commonProperties.DebugMutators = append(base.commonProperties.DebugMutators, b.MutatorName())
		base.commonProperties.DebugVariations = append(base.commonProperties.DebugVariations, variations[i])
	}

	return modules
}

func (b *bottomUpMutatorContext) SetDependencyVariation(variation string) {
	b.bp.SetDependencyVariation(variation)
}

func (b *bottomUpMutatorContext) SetDefaultDependencyVariation(variation *string) {
	b.bp.SetDefaultDependencyVariation(variation)
}

func (b *bottomUpMutatorContext) AddVariationDependencies(variations []blueprint.Variation, tag blueprint.DependencyTag,
	names ...string) {

	b.bp.AddVariationDependencies(variations, tag, names...)
}

func (b *bottomUpMutatorContext) AddFarVariationDependencies(variations []blueprint.Variation,
	tag blueprint.DependencyTag, names ...string) {

	b.bp.AddFarVariationDependencies(variations, tag, names...)
}

func (b *bottomUpMutatorContext) AddInterVariantDependency(tag blueprint.DependencyTag, from, to blueprint.Module) {
	b.bp.AddInterVariantDependency(tag, from, to)
}

func (b *bottomUpMutatorContext) ReplaceDependencies(name string) {
	b.bp.ReplaceDependencies(name)
}

func (b *bottomUpMutatorContext) AliasVariation(variationName string) {
	b.bp.AliasVariation(variationName)
}
