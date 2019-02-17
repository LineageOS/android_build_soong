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
	"github.com/google/blueprint"
)

// This file implements hooks that external module types can use to inject logic into existing
// module types.  Each hook takes an interface as a parameter so that new methods can be added
// to the interface without breaking existing module types.

// Load hooks are run after the module's properties have been filled from the blueprint file, but
// before the module has been split into architecture variants, and before defaults modules have
// been applied.
type LoadHookContext interface {
	// TODO: a new context that includes Config() but not Target(), etc.?
	BaseContext
	AppendProperties(...interface{})
	PrependProperties(...interface{})
	CreateModule(blueprint.ModuleFactory, ...interface{})
}

// Arch hooks are run after the module has been split into architecture variants, and can be used
// to add architecture-specific properties.
type ArchHookContext interface {
	BaseContext
	AppendProperties(...interface{})
	PrependProperties(...interface{})
}

func AddLoadHook(m blueprint.Module, hook func(LoadHookContext)) {
	h := &m.(Module).base().hooks
	h.load = append(h.load, hook)
}

func AddArchHook(m blueprint.Module, hook func(ArchHookContext)) {
	h := &m.(Module).base().hooks
	h.arch = append(h.arch, hook)
}

func (x *hooks) runLoadHooks(ctx LoadHookContext, m *ModuleBase) {
	if len(x.load) > 0 {
		for _, x := range x.load {
			x(ctx)
			if ctx.Failed() {
				return
			}
		}
	}
}

func (x *hooks) runArchHooks(ctx ArchHookContext, m *ModuleBase) {
	if len(x.arch) > 0 {
		for _, x := range x.arch {
			x(ctx)
			if ctx.Failed() {
				return
			}
		}
	}
}

type InstallHookContext interface {
	ModuleContext
	Path() OutputPath
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
	path    OutputPath
	symlink bool
}

func (x *installHookContext) Path() OutputPath {
	return x.path
}

func (x *installHookContext) Symlink() bool {
	return x.symlink
}

func (x *hooks) runInstallHooks(ctx ModuleContext, path OutputPath, symlink bool) {
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
	load    []func(LoadHookContext)
	arch    []func(ArchHookContext)
	install []func(InstallHookContext)
}

func LoadHookMutator(ctx TopDownMutatorContext) {
	if m, ok := ctx.Module().(Module); ok {
		// Cast through *androidTopDownMutatorContext because AppendProperties is implemented
		// on *androidTopDownMutatorContext but not exposed through TopDownMutatorContext
		var loadHookCtx LoadHookContext = ctx.(*androidTopDownMutatorContext)
		m.base().hooks.runLoadHooks(loadHookCtx, m.base())
	}
}

func archHookMutator(ctx TopDownMutatorContext) {
	if m, ok := ctx.Module().(Module); ok {
		// Cast through *androidTopDownMutatorContext because AppendProperties is implemented
		// on *androidTopDownMutatorContext but not exposed through TopDownMutatorContext
		var archHookCtx ArchHookContext = ctx.(*androidTopDownMutatorContext)
		m.base().hooks.runArchHooks(archHookCtx, m.base())
	}
}
