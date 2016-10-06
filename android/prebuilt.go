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

import "github.com/google/blueprint"

// This file implements common functionality for handling modules that may exist as prebuilts,
// source, or both.

var prebuiltDependencyTag blueprint.BaseDependencyTag

func SourceModuleHasPrebuilt(ctx ModuleContext) OptionalPath {
	var path Path
	ctx.VisitDirectDeps(func(m blueprint.Module) {
		if ctx.OtherModuleDependencyTag(m) == prebuiltDependencyTag {
			p := m.(PrebuiltInterface).Prebuilt()
			if p.usePrebuilt(ctx) {
				path = p.Path(ctx)
			}
		}
	})

	return OptionalPathForPath(path)
}

type Prebuilt struct {
	Properties struct {
		Srcs []string `android:"arch_variant"`
		// When prefer is set to true the prebuilt will be used instead of any source module with
		// a matching name.
		Prefer bool `android:"arch_variant"`

		SourceExists bool `blueprint:"mutated"`
	}
	module Module
}

func (p *Prebuilt) Name(name string) string {
	return "prebuilt_" + name
}

func (p *Prebuilt) Path(ctx ModuleContext) Path {
	if len(p.Properties.Srcs) == 0 {
		ctx.PropertyErrorf("srcs", "missing prebuilt source file")
		return nil
	}

	if len(p.Properties.Srcs) > 1 {
		ctx.PropertyErrorf("srcs", "multiple prebuilt source files")
		return nil
	}

	return PathForModuleSrc(ctx, p.Properties.Srcs[0])
}

type PrebuiltInterface interface {
	Module
	Prebuilt() *Prebuilt
}

type PrebuiltSourceInterface interface {
	SkipInstall()
}

// prebuiltMutator ensures that there is always a module with an undecorated name, and marks
// prebuilt modules that have both a prebuilt and a source module.
func prebuiltMutator(ctx BottomUpMutatorContext) {
	if m, ok := ctx.Module().(PrebuiltInterface); ok && m.Prebuilt() != nil {
		p := m.Prebuilt()
		name := m.base().BaseModuleName()
		if ctx.OtherModuleExists(name) {
			ctx.AddReverseDependency(ctx.Module(), prebuiltDependencyTag, name)
			p.Properties.SourceExists = true
		} else {
			ctx.Rename(name)
		}
	}
}

// PrebuiltReplaceMutator replaces dependencies on the source module with dependencies on the prebuilt
// when both modules exist and the prebuilt should be used.
func PrebuiltReplaceMutator(ctx BottomUpMutatorContext) {
	if m, ok := ctx.Module().(PrebuiltInterface); ok && m.Prebuilt() != nil {
		p := m.Prebuilt()
		name := m.base().BaseModuleName()
		if p.Properties.SourceExists && p.usePrebuilt(ctx) {
			ctx.ReplaceDependencies(name)
		}
	}
}

// PrebuiltDisableMutator disables source modules that have prebuilts that should be used instead.
func PrebuiltDisableMutator(ctx TopDownMutatorContext) {
	if s, ok := ctx.Module().(PrebuiltSourceInterface); ok {
		ctx.VisitDirectDeps(func(m blueprint.Module) {
			if ctx.OtherModuleDependencyTag(m) == prebuiltDependencyTag {
				p := m.(PrebuiltInterface).Prebuilt()
				if p.usePrebuilt(ctx) {
					s.SkipInstall()
				}
			}
		})
	}
}

func (p *Prebuilt) usePrebuilt(ctx BaseContext) bool {
	// TODO: use p.Properties.Name and ctx.ModuleDir to override prefer
	return p.Properties.Prefer && len(p.Properties.Srcs) > 0
}
