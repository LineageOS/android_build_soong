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
	"reflect"
)

// A sortable component is one whose registration order affects the order in which it is executed
// and so affects the behavior of the build system. As a result it is important for the order in
// which they are registered during tests to match the order used at runtime and so the test
// infrastructure will sort them to match.
//
// The sortable components are mutators, singletons and pre-singletons. Module types are not
// sortable because their order of registration does not affect the runtime behavior.
type sortableComponent interface {
	// componentName returns the name of the component.
	//
	// Uniquely identifies the components within the set of components used at runtime and during
	// tests.
	componentName() string

	// registers this component in the supplied context.
	register(ctx *Context)
}

type sortableComponents []sortableComponent

// registerAll registers all components in this slice with the supplied context.
func (r sortableComponents) registerAll(ctx *Context) {
	for _, c := range r {
		c.register(ctx)
	}
}

type moduleType struct {
	name    string
	factory ModuleFactory
}

func (t moduleType) register(ctx *Context) {
	ctx.RegisterModuleType(t.name, ModuleFactoryAdaptor(t.factory))
}

var moduleTypes []moduleType
var moduleTypesForDocs = map[string]reflect.Value{}
var moduleTypeByFactory = map[reflect.Value]string{}

type singleton struct {
	// True if this should be registered as a parallel singleton.
	parallel bool

	name    string
	factory SingletonFactory
}

func newSingleton(name string, factory SingletonFactory, parallel bool) singleton {
	return singleton{parallel: parallel, name: name, factory: factory}
}

func (s singleton) componentName() string {
	return s.name
}

func (s singleton) register(ctx *Context) {
	adaptor := SingletonFactoryAdaptor(ctx, s.factory)
	ctx.RegisterSingletonType(s.name, adaptor, s.parallel)
}

var _ sortableComponent = singleton{}

var singletons sortableComponents

type mutator struct {
	name              string
	bottomUpMutator   blueprint.BottomUpMutator
	topDownMutator    blueprint.TopDownMutator
	transitionMutator blueprint.TransitionMutator
	parallel          bool
}

var _ sortableComponent = &mutator{}

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
func SingletonFactoryAdaptor(ctx *Context, factory SingletonFactory) blueprint.SingletonFactory {
	return func() blueprint.Singleton {
		singleton := factory()
		if makevars, ok := singleton.(SingletonMakeVarsProvider); ok {
			ctx.registerSingletonMakeVarsProvider(makevars)
		}
		return &singletonAdaptor{Singleton: singleton}
	}
}

func RegisterModuleType(name string, factory ModuleFactory) {
	moduleTypes = append(moduleTypes, moduleType{name, factory})
	RegisterModuleTypeForDocs(name, reflect.ValueOf(factory))
}

// RegisterModuleTypeForDocs associates a module type name with a reflect.Value of the factory
// function that has documentation for the module type.  It is normally called automatically
// by RegisterModuleType, but can be called manually after RegisterModuleType in order to
// override the factory method used for documentation, for example if the method passed to
// RegisterModuleType was a lambda.
func RegisterModuleTypeForDocs(name string, factory reflect.Value) {
	moduleTypesForDocs[name] = factory
	moduleTypeByFactory[factory] = name
}

func registerSingletonType(name string, factory SingletonFactory, parallel bool) {
	singletons = append(singletons, newSingleton(name, factory, parallel))
}

func RegisterSingletonType(name string, factory SingletonFactory) {
	registerSingletonType(name, factory, false)
}

func RegisterParallelSingletonType(name string, factory SingletonFactory) {
	registerSingletonType(name, factory, true)
}

type Context struct {
	*blueprint.Context
	config Config
}

func NewContext(config Config) *Context {
	ctx := &Context{blueprint.NewContext(), config}
	ctx.SetSrcDir(absSrcDir)
	ctx.AddIncludeTags(config.IncludeTags()...)
	ctx.AddSourceRootDirs(config.SourceRootDirs()...)
	return ctx
}

// Register the pipeline of singletons, module types, and mutators for
// generating build.ninja and other files for Kati, from Android.bp files.
func (ctx *Context) Register() {
	for _, t := range moduleTypes {
		t.register(ctx)
	}

	mutators := collateGloballyRegisteredMutators()
	mutators.registerAll(ctx)

	singletons := collateGloballyRegisteredSingletons()
	singletons.registerAll(ctx)
}

func (ctx *Context) Config() Config {
	return ctx.config
}

func (ctx *Context) registerSingletonMakeVarsProvider(makevars SingletonMakeVarsProvider) {
	registerSingletonMakeVarsProvider(ctx.config, makevars)
}

func collateGloballyRegisteredSingletons() sortableComponents {
	allSingletons := append(sortableComponents(nil), singletons...)
	allSingletons = append(allSingletons,
		// Register phony just before makevars so it can write out its phony rules as Make rules
		singleton{parallel: false, name: "phony", factory: phonySingletonFactory},

		// Register makevars after other singletons so they can export values through makevars
		singleton{parallel: false, name: "makevars", factory: makeVarsSingletonFunc},

		// Register rawfiles and ninjadeps last so that they can track all used environment variables and
		// Ninja file dependencies stored in the config.
		singleton{parallel: false, name: "rawfiles", factory: rawFilesSingletonFactory},
		singleton{parallel: false, name: "ninjadeps", factory: ninjaDepsSingletonFactory},
	)

	return allSingletons
}

func ModuleTypeFactories() map[string]ModuleFactory {
	ret := make(map[string]ModuleFactory)
	for _, t := range moduleTypes {
		ret[t.name] = t.factory
	}
	return ret
}

func ModuleTypeFactoriesForDocs() map[string]reflect.Value {
	return moduleTypesForDocs
}

func ModuleTypeByFactory() map[reflect.Value]string {
	return moduleTypeByFactory
}

// Interface for registering build components.
//
// Provided to allow registration of build components to be shared between the runtime
// and test environments.
type RegistrationContext interface {
	RegisterModuleType(name string, factory ModuleFactory)
	RegisterSingletonModuleType(name string, factory SingletonModuleFactory)
	RegisterParallelSingletonModuleType(name string, factory SingletonModuleFactory)
	RegisterParallelSingletonType(name string, factory SingletonFactory)
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
//	init() {
//	  RegisterBuildComponents(android.InitRegistrationContext)
//	}
//
//	func RegisterBuildComponents(ctx android.RegistrationContext) {
//	  ctx.RegisterModuleType(...)
//	  ...
//	}
//
// Extracting the actual registration into a separate RegisterBuildComponents(ctx) function
// allows it to be used to initialize test context, e.g.
//
//	ctx := android.NewTestContext(config)
//	RegisterBuildComponents(ctx)
var InitRegistrationContext RegistrationContext = &initRegistrationContext{
	moduleTypes:    make(map[string]ModuleFactory),
	singletonTypes: make(map[string]SingletonFactory),
}

// Make sure the TestContext implements RegistrationContext.
var _ RegistrationContext = (*TestContext)(nil)

type initRegistrationContext struct {
	moduleTypes        map[string]ModuleFactory
	singletonTypes     map[string]SingletonFactory
	moduleTypesForDocs map[string]reflect.Value
}

func (ctx *initRegistrationContext) RegisterModuleType(name string, factory ModuleFactory) {
	if _, present := ctx.moduleTypes[name]; present {
		panic(fmt.Sprintf("module type %q is already registered", name))
	}
	ctx.moduleTypes[name] = factory
	RegisterModuleType(name, factory)
	RegisterModuleTypeForDocs(name, reflect.ValueOf(factory))
}

func (ctx *initRegistrationContext) RegisterSingletonModuleType(name string, factory SingletonModuleFactory) {
	ctx.registerSingletonModuleType(name, factory, false)
}
func (ctx *initRegistrationContext) RegisterParallelSingletonModuleType(name string, factory SingletonModuleFactory) {
	ctx.registerSingletonModuleType(name, factory, true)
}

func (ctx *initRegistrationContext) registerSingletonModuleType(name string, factory SingletonModuleFactory, parallel bool) {
	s, m := SingletonModuleFactoryAdaptor(name, factory)
	ctx.registerSingletonType(name, s, parallel)
	ctx.RegisterModuleType(name, m)
	// Overwrite moduleTypesForDocs with the original factory instead of the lambda returned by
	// SingletonModuleFactoryAdaptor so that docs can find the module type documentation on the
	// factory method.
	RegisterModuleTypeForDocs(name, reflect.ValueOf(factory))
}

func (ctx *initRegistrationContext) registerSingletonType(name string, factory SingletonFactory, parallel bool) {
	if _, present := ctx.singletonTypes[name]; present {
		panic(fmt.Sprintf("singleton type %q is already registered", name))
	}
	ctx.singletonTypes[name] = factory
	registerSingletonType(name, factory, parallel)
}

func (ctx *initRegistrationContext) RegisterSingletonType(name string, factory SingletonFactory) {
	ctx.registerSingletonType(name, factory, false)
}

func (ctx *initRegistrationContext) RegisterParallelSingletonType(name string, factory SingletonFactory) {
	ctx.registerSingletonType(name, factory, true)
}

func (ctx *initRegistrationContext) PreArchMutators(f RegisterMutatorFunc) {
	PreArchMutators(f)
}

func (ctx *initRegistrationContext) HardCodedPreArchMutators(_ RegisterMutatorFunc) {
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
