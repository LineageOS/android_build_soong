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
	"fmt"

	"github.com/google/blueprint"
)

type moduleType struct {
	name    string
	factory ModuleFactory
}

var moduleTypes []moduleType

type singleton struct {
	name    string
	factory blueprint.SingletonFactory
}

var singletons []singleton
var preSingletons []singleton

type mutator struct {
	name            string
	bottomUpMutator blueprint.BottomUpMutator
	topDownMutator  blueprint.TopDownMutator
	parallel        bool
}

type ModuleFactory func() Module

// ModuleFactoryAdaptor wraps a ModuleFactory into a blueprint.ModuleFactory by converting a Module
// into a blueprint.Module and a list of property structs
func ModuleFactoryAdaptor(factory ModuleFactory) blueprint.ModuleFactory {
	return func() (blueprint.Module, []interface{}) {
		module := factory()
		return module, module.GetProperties()
	}
}

type SingletonFactory func() Singleton

// SingletonFactoryAdaptor wraps a SingletonFactory into a blueprint.SingletonFactory by converting
// a Singleton into a blueprint.Singleton
func SingletonFactoryAdaptor(factory SingletonFactory) blueprint.SingletonFactory {
	return func() blueprint.Singleton {
		singleton := factory()
		if makevars, ok := singleton.(SingletonMakeVarsProvider); ok {
			registerSingletonMakeVarsProvider(makevars)
		}
		return &singletonAdaptor{Singleton: singleton}
	}
}

func RegisterModuleType(name string, factory ModuleFactory) {
	moduleTypes = append(moduleTypes, moduleType{name, factory})
}

func RegisterSingletonType(name string, factory SingletonFactory) {
	singletons = append(singletons, singleton{name, SingletonFactoryAdaptor(factory)})
}

func RegisterPreSingletonType(name string, factory SingletonFactory) {
	preSingletons = append(preSingletons, singleton{name, SingletonFactoryAdaptor(factory)})
}

type Context struct {
	*blueprint.Context
}

func NewContext() *Context {
	ctx := &Context{blueprint.NewContext()}
	ctx.SetSrcDir(absSrcDir)
	return ctx
}

func (ctx *Context) Register() {
	for _, t := range preSingletons {
		ctx.RegisterPreSingletonType(t.name, t.factory)
	}

	for _, t := range moduleTypes {
		ctx.RegisterModuleType(t.name, ModuleFactoryAdaptor(t.factory))
	}

	for _, t := range singletons {
		ctx.RegisterSingletonType(t.name, t.factory)
	}

	registerMutators(ctx.Context, preArch, preDeps, postDeps, finalDeps)

	// Register phony just before makevars so it can write out its phony rules as Make rules
	ctx.RegisterSingletonType("phony", SingletonFactoryAdaptor(phonySingletonFactory))

	// Register makevars after other singletons so they can export values through makevars
	ctx.RegisterSingletonType("makevars", SingletonFactoryAdaptor(makeVarsSingletonFunc))

	// Register env last so that it can track all used environment variables
	ctx.RegisterSingletonType("env", SingletonFactoryAdaptor(EnvSingleton))
}

func ModuleTypeFactories() map[string]ModuleFactory {
	ret := make(map[string]ModuleFactory)
	for _, t := range moduleTypes {
		ret[t.name] = t.factory
	}
	return ret
}

// Interface for registering build components.
//
// Provided to allow registration of build components to be shared between the runtime
// and test environments.
type RegistrationContext interface {
	RegisterModuleType(name string, factory ModuleFactory)
	RegisterSingletonType(name string, factory SingletonFactory)
	PreArchMutators(f RegisterMutatorFunc)

	// Register pre arch mutators that are hard coded into mutator.go.
	//
	// Only registers mutators for testing, is a noop on the InitRegistrationContext.
	HardCodedPreArchMutators(f RegisterMutatorFunc)

	PreDepsMutators(f RegisterMutatorFunc)
	PostDepsMutators(f RegisterMutatorFunc)
	FinalDepsMutators(f RegisterMutatorFunc)
}

// Used to register build components from an init() method, e.g.
//
// init() {
//   RegisterBuildComponents(android.InitRegistrationContext)
// }
//
// func RegisterBuildComponents(ctx android.RegistrationContext) {
//   ctx.RegisterModuleType(...)
//   ...
// }
//
// Extracting the actual registration into a separate RegisterBuildComponents(ctx) function
// allows it to be used to initialize test context, e.g.
//
//   ctx := android.NewTestContext()
//   RegisterBuildComponents(ctx)
var InitRegistrationContext RegistrationContext = &initRegistrationContext{
	moduleTypes:    make(map[string]ModuleFactory),
	singletonTypes: make(map[string]SingletonFactory),
}

// Make sure the TestContext implements RegistrationContext.
var _ RegistrationContext = (*TestContext)(nil)

type initRegistrationContext struct {
	moduleTypes    map[string]ModuleFactory
	singletonTypes map[string]SingletonFactory
}

func (ctx *initRegistrationContext) RegisterModuleType(name string, factory ModuleFactory) {
	if _, present := ctx.moduleTypes[name]; present {
		panic(fmt.Sprintf("module type %q is already registered", name))
	}
	ctx.moduleTypes[name] = factory
	RegisterModuleType(name, factory)
}

func (ctx *initRegistrationContext) RegisterSingletonType(name string, factory SingletonFactory) {
	if _, present := ctx.singletonTypes[name]; present {
		panic(fmt.Sprintf("singleton type %q is already registered", name))
	}
	ctx.singletonTypes[name] = factory
	RegisterSingletonType(name, factory)
}

func (ctx *initRegistrationContext) PreArchMutators(f RegisterMutatorFunc) {
	PreArchMutators(f)
}

func (ctx *initRegistrationContext) HardCodedPreArchMutators(f RegisterMutatorFunc) {
	// Nothing to do as the mutators are hard code in preArch in mutator.go
}

func (ctx *initRegistrationContext) PreDepsMutators(f RegisterMutatorFunc) {
	PreDepsMutators(f)
}

func (ctx *initRegistrationContext) PostDepsMutators(f RegisterMutatorFunc) {
	PostDepsMutators(f)
}

func (ctx *initRegistrationContext) FinalDepsMutators(f RegisterMutatorFunc) {
	FinalDepsMutators(f)
}
