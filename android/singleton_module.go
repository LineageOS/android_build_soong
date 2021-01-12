// Copyright 2020 Google Inc. All rights reserved.
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
	"sync"

	"github.com/google/blueprint"
)

// A SingletonModule is halfway between a Singleton and a Module.  It has access to visiting
// other modules via its GenerateSingletonBuildActions method, but must be defined in an Android.bp
// file and can also be depended on like a module.  It must be used zero or one times in an
// Android.bp file, and it can only have a single variant.
//
// The SingletonModule's GenerateAndroidBuildActions method will be called before any normal or
// singleton module that depends on it, but its GenerateSingletonBuildActions method will be called
// after all modules, in registration order with other singletons and singleton modules.
// GenerateAndroidBuildActions and GenerateSingletonBuildActions will not be called if the
// SingletonModule was not instantiated in an Android.bp file.
//
// Since the SingletonModule rules likely depend on the modules visited during
// GenerateSingletonBuildActions, the GenerateAndroidBuildActions is unlikely to produce any
// rules directly.  Instead, it will probably set some providers to paths that will later have rules
// generated to produce them in GenerateSingletonBuildActions.
//
// The expected use case for a SingletonModule is a module that produces files that depend on all
// modules in the tree and will be used by other modules.  For example it could produce a text
// file that lists all modules that meet a certain criteria, and that text file could be an input
// to another module.  Care must be taken that the ninja rules produced by the SingletonModule
// don't produce a cycle by referencing output files of rules of modules that depend on the
// SingletonModule.
//
// A SingletonModule must embed a SingletonModuleBase struct, and its factory method must be
// registered with RegisterSingletonModuleType from an init() function.
//
// A SingletonModule can also implement SingletonMakeVarsProvider to export values to Make.
type SingletonModule interface {
	Module
	GenerateSingletonBuildActions(SingletonContext)
	singletonModuleBase() *SingletonModuleBase
}

// SingletonModuleBase must be embedded into implementers of the SingletonModule interface.
type SingletonModuleBase struct {
	ModuleBase

	lock    sync.Mutex
	bp      string
	variant string
}

// GenerateBuildActions wraps the ModuleBase GenerateBuildActions method, verifying it was only
// called once to prevent multiple variants of a SingletonModule.
func (smb *SingletonModuleBase) GenerateBuildActions(ctx blueprint.ModuleContext) {
	smb.lock.Lock()
	if smb.variant != "" {
		ctx.ModuleErrorf("GenerateAndroidBuildActions already called for variant %q, SingletonModules can only  have one variant", smb.variant)
	}
	smb.variant = ctx.ModuleSubDir()
	smb.lock.Unlock()

	smb.ModuleBase.GenerateBuildActions(ctx)
}

// InitAndroidSingletonModule must be called from the SingletonModule's factory function to
// initialize SingletonModuleBase.
func InitAndroidSingletonModule(sm SingletonModule) {
	InitAndroidModule(sm)
}

// singletonModuleBase retrieves the embedded SingletonModuleBase from a SingletonModule.
func (smb *SingletonModuleBase) singletonModuleBase() *SingletonModuleBase { return smb }

// SingletonModuleFactory is a factory method that returns a SingletonModule.
type SingletonModuleFactory func() SingletonModule

// SingletonModuleFactoryAdaptor converts a SingletonModuleFactory into a SingletonFactory and a
// ModuleFactory.
func SingletonModuleFactoryAdaptor(name string, factory SingletonModuleFactory) (SingletonFactory, ModuleFactory) {
	// The sm variable acts as a static holder of the only SingletonModule instance.  Calls to the
	// returned SingletonFactory and ModuleFactory lambdas will always return the same sm value.
	// The SingletonFactory is only expected to be called once, but the ModuleFactory may be
	// called multiple times if the module is replaced with a clone of itself at the end of
	// blueprint.ResolveDependencies.
	var sm SingletonModule
	s := func() Singleton {
		sm = factory()
		return &singletonModuleSingletonAdaptor{sm}
	}
	m := func() Module {
		if sm == nil {
			panic(fmt.Errorf("Singleton %q for SingletonModule was not instantiated", name))
		}

		// Check for multiple uses of a SingletonModule in a LoadHook.  Checking directly in the
		// factory would incorrectly flag when the factory was called again when the module is
		// replaced with a clone of itself at the end of blueprint.ResolveDependencies.
		AddLoadHook(sm, func(ctx LoadHookContext) {
			smb := sm.singletonModuleBase()
			smb.lock.Lock()
			defer smb.lock.Unlock()
			if smb.bp != "" {
				ctx.ModuleErrorf("Duplicate SingletonModule %q, previously used in %s", name, smb.bp)
			}
			smb.bp = ctx.BlueprintsFile()
		})
		return sm
	}
	return s, m
}

// singletonModuleSingletonAdaptor makes a SingletonModule into a Singleton by translating the
// GenerateSingletonBuildActions method to Singleton.GenerateBuildActions.
type singletonModuleSingletonAdaptor struct {
	sm SingletonModule
}

// GenerateBuildActions calls the SingletonModule's GenerateSingletonBuildActions method, but only
// if the module was defined in an Android.bp file.
func (smsa *singletonModuleSingletonAdaptor) GenerateBuildActions(ctx SingletonContext) {
	if smsa.sm.singletonModuleBase().bp != "" {
		smsa.sm.GenerateSingletonBuildActions(ctx)
	}
}

func (smsa *singletonModuleSingletonAdaptor) MakeVars(ctx MakeVarsContext) {
	if smsa.sm.singletonModuleBase().bp != "" {
		if makeVars, ok := smsa.sm.(SingletonMakeVarsProvider); ok {
			makeVars.MakeVars(ctx)
		}
	}
}
