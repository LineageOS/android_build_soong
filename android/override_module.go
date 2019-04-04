// Copyright (C) 2019 The Android Open Source Project
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

// This file contains all the foundation components for override modules and their base module
// types. Override modules are a kind of opposite of default modules in that they override certain
// properties of an existing base module whereas default modules provide base module data to be
// overridden. However, unlike default and defaultable module pairs, both override and overridable
// modules generate and output build actions, and it is up to product make vars to decide which one
// to actually build and install in the end. In other words, default modules and defaultable modules
// can be compared to abstract classes and concrete classes in C++ and Java. By the same analogy,
// both override and overridable modules act like concrete classes.
//
// There is one more crucial difference from the logic perspective. Unlike default pairs, most Soong
// actions happen in the base (overridable) module by creating a local variant for each override
// module based on it.

import (
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// Interface for override module types, e.g. override_android_app, override_apex
type OverrideModule interface {
	Module

	getOverridingProperties() []interface{}
	setOverridingProperties(properties []interface{})

	getOverrideModuleProperties() *OverrideModuleProperties
}

// Base module struct for override module types
type OverrideModuleBase struct {
	moduleProperties OverrideModuleProperties

	overridingProperties []interface{}
}

type OverrideModuleProperties struct {
	// Name of the base module to be overridden
	Base *string

	// TODO(jungjw): Add an optional override_name bool flag.
}

func (o *OverrideModuleBase) getOverridingProperties() []interface{} {
	return o.overridingProperties
}

func (o *OverrideModuleBase) setOverridingProperties(properties []interface{}) {
	o.overridingProperties = properties
}

func (o *OverrideModuleBase) getOverrideModuleProperties() *OverrideModuleProperties {
	return &o.moduleProperties
}

func InitOverrideModule(m OverrideModule) {
	m.setOverridingProperties(m.GetProperties())

	m.AddProperties(m.getOverrideModuleProperties())
}

// Interface for overridable module types, e.g. android_app, apex
type OverridableModule interface {
	setOverridableProperties(prop []interface{})

	addOverride(o OverrideModule)
	getOverrides() []OverrideModule

	override(ctx BaseModuleContext, o OverrideModule)

	setOverridesProperty(overridesProperties *[]string)
}

// Base module struct for overridable module types
type OverridableModuleBase struct {
	ModuleBase

	// List of OverrideModules that override this base module
	overrides []OverrideModule
	// Used to parallelize registerOverrideMutator executions. Note that only addOverride locks this
	// mutex. It is because addOverride and getOverride are used in different mutators, and so are
	// guaranteed to be not mixed. (And, getOverride only reads from overrides, and so don't require
	// mutex locking.)
	overridesLock sync.Mutex

	overridableProperties []interface{}

	// If an overridable module has a property to list other modules that itself overrides, it should
	// set this to a pointer to the property through the InitOverridableModule function, so that
	// override information is propagated and aggregated correctly.
	overridesProperty *[]string
}

func InitOverridableModule(m OverridableModule, overridesProperty *[]string) {
	m.setOverridableProperties(m.(Module).GetProperties())
	m.setOverridesProperty(overridesProperty)
}

func (b *OverridableModuleBase) setOverridableProperties(prop []interface{}) {
	b.overridableProperties = prop
}

func (b *OverridableModuleBase) addOverride(o OverrideModule) {
	b.overridesLock.Lock()
	b.overrides = append(b.overrides, o)
	b.overridesLock.Unlock()
}

// Should NOT be used in the same mutator as addOverride.
func (b *OverridableModuleBase) getOverrides() []OverrideModule {
	return b.overrides
}

func (b *OverridableModuleBase) setOverridesProperty(overridesProperty *[]string) {
	b.overridesProperty = overridesProperty
}

// Overrides a base module with the given OverrideModule.
func (b *OverridableModuleBase) override(ctx BaseModuleContext, o OverrideModule) {
	// Adds the base module to the overrides property, if exists, of the overriding module. See the
	// comment on OverridableModuleBase.overridesProperty for details.
	if b.overridesProperty != nil {
		*b.overridesProperty = append(*b.overridesProperty, b.Name())
	}
	for _, p := range b.overridableProperties {
		for _, op := range o.getOverridingProperties() {
			if proptools.TypeEqual(p, op) {
				err := proptools.AppendProperties(p, op, nil)
				if err != nil {
					if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
						ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
					} else {
						panic(err)
					}
				}
			}
		}
	}
}

// Mutators for override/overridable modules. All the fun happens in these functions. It is critical
// to keep them in this order and not put any order mutators between them.
func RegisterOverridePreArchMutators(ctx RegisterMutatorsContext) {
	ctx.BottomUp("override_deps", overrideModuleDepsMutator).Parallel()
	ctx.TopDown("register_override", registerOverrideMutator).Parallel()
	ctx.BottomUp("perform_override", performOverrideMutator).Parallel()
}

type overrideBaseDependencyTag struct {
	blueprint.BaseDependencyTag
}

var overrideBaseDepTag overrideBaseDependencyTag

// Adds dependency on the base module to the overriding module so that they can be visited in the
// next phase.
func overrideModuleDepsMutator(ctx BottomUpMutatorContext) {
	if module, ok := ctx.Module().(OverrideModule); ok {
		ctx.AddDependency(ctx.Module(), overrideBaseDepTag, *module.getOverrideModuleProperties().Base)
	}
}

// Visits the base module added as a dependency above, checks the module type, and registers the
// overriding module.
func registerOverrideMutator(ctx TopDownMutatorContext) {
	ctx.VisitDirectDepsWithTag(overrideBaseDepTag, func(base Module) {
		if o, ok := base.(OverridableModule); ok {
			o.addOverride(ctx.Module().(OverrideModule))
		} else {
			ctx.PropertyErrorf("base", "unsupported base module type")
		}
	})
}

// Now, goes through all overridable modules, finds all modules overriding them, creates a local
// variant for each of them, and performs the actual overriding operation by calling override().
func performOverrideMutator(ctx BottomUpMutatorContext) {
	if b, ok := ctx.Module().(OverridableModule); ok {
		overrides := b.getOverrides()
		if len(overrides) == 0 {
			return
		}
		variants := make([]string, len(overrides)+1)
		// The first variant is for the original, non-overridden, base module.
		variants[0] = ""
		for i, o := range overrides {
			variants[i+1] = o.(Module).Name()
		}
		mods := ctx.CreateLocalVariations(variants...)
		for i, o := range overrides {
			mods[i+1].(OverridableModule).override(ctx, o)
		}
	}
}
