// Copyright 2016 Google Inc. All rights reserved.
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

// This file implements hooks that external module types can use to inject logic into existing
// module types.  Each hook takes an interface as a parameter so that new methods can be added
// to the interface without breaking existing module types.

// Load hooks are run after the module's properties have been filled from the blueprint file, but
// before the module has been split into architecture variants, and before defaults modules have
// been applied.
type LoadHookContext interface {
	EarlyModuleContext

	AppendProperties(...interface{})
	PrependProperties(...interface{})
	CreateModule(ModuleFactory, ...interface{}) Module

	registerScopedModuleType(name string, factory blueprint.ModuleFactory)
	moduleFactories() map[string]blueprint.ModuleFactory
}

// Add a hook that will be called once the module has been loaded, i.e. its
// properties have been initialized from the Android.bp file.
//
// Consider using SetDefaultableHook to register a hook for any module that implements
// DefaultableModule as the hook is called after any defaults have been applied to the
// module which could reduce duplication and make it easier to use.
func AddLoadHook(m blueprint.Module, hook func(LoadHookContext)) {
	blueprint.AddLoadHook(m, func(ctx blueprint.LoadHookContext) {
		actx := &loadHookContext{
			earlyModuleContext: m.(Module).base().earlyModuleContextFactory(ctx),
			bp:                 ctx,
		}
		hook(actx)
	})
}

type loadHookContext struct {
	earlyModuleContext
	bp     blueprint.LoadHookContext
	module Module
}

func (l *loadHookContext) moduleFactories() map[string]blueprint.ModuleFactory {
	return l.bp.ModuleFactories()
}

func (l *loadHookContext) AppendProperties(props ...interface{}) {
	for _, p := range props {
		err := proptools.AppendMatchingProperties(l.Module().base().customizableProperties,
			p, nil)
		if err != nil {
			if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
				l.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
			} else {
				panic(err)
			}
		}
	}
}

func (l *loadHookContext) PrependProperties(props ...interface{}) {
	for _, p := range props {
		err := proptools.PrependMatchingProperties(l.Module().base().customizableProperties,
			p, nil)
		if err != nil {
			if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
				l.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
			} else {
				panic(err)
			}
		}
	}
}

func (l *loadHookContext) CreateModule(factory ModuleFactory, props ...interface{}) Module {
	inherited := []interface{}{&l.Module().base().commonProperties}
	module := l.bp.CreateModule(ModuleFactoryAdaptor(factory), append(inherited, props...)...).(Module)

	if l.Module().base().variableProperties != nil && module.base().variableProperties != nil {
		src := l.Module().base().variableProperties
		dst := []interface{}{
			module.base().variableProperties,
			// Put an empty copy of the src properties into dst so that properties in src that are not in dst
			// don't cause a "failed to find property to extend" error.
			proptools.CloneEmptyProperties(reflect.ValueOf(src)).Interface(),
		}
		err := proptools.AppendMatchingProperties(dst, src, nil)
		if err != nil {
			panic(err)
		}
	}

	return module
}

func (l *loadHookContext) registerScopedModuleType(name string, factory blueprint.ModuleFactory) {
	l.bp.RegisterScopedModuleType(name, factory)
}

type InstallHookContext interface {
	ModuleContext
	Path() InstallPath
	Symlink() bool
}

// Install hooks are run after a module creates a rule to install a file or symlink.
// The installed path is available from InstallHookContext.Path(), and
// InstallHookContext.Symlink() will be true if it was a symlink.
func AddInstallHook(m blueprint.Module, hook func(InstallHookContext)) {
	h := &m.(Module).base().hooks
	h.install = append(h.install, hook)
}

type installHookContext struct {
	ModuleContext
	path    InstallPath
	symlink bool
}

func (x *installHookContext) Path() InstallPath {
	return x.path
}

func (x *installHookContext) Symlink() bool {
	return x.symlink
}

func (x *hooks) runInstallHooks(ctx ModuleContext, path InstallPath, symlink bool) {
	if len(x.install) > 0 {
		mctx := &installHookContext{
			ModuleContext: ctx,
			path:          path,
			symlink:       symlink,
		}
		for _, x := range x.install {
			x(mctx)
			if mctx.Failed() {
				return
			}
		}
	}
}

type hooks struct {
	install []func(InstallHookContext)
}
