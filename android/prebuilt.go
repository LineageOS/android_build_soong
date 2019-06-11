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
	"fmt"
	"reflect"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// This file implements common functionality for handling modules that may exist as prebuilts,
// source, or both.

type prebuiltDependencyTag struct {
	blueprint.BaseDependencyTag
}

var prebuiltDepTag prebuiltDependencyTag

type PrebuiltProperties struct {
	// When prefer is set to true the prebuilt will be used instead of any source module with
	// a matching name.
	Prefer *bool `android:"arch_variant"`

	SourceExists bool `blueprint:"mutated"`
	UsePrebuilt  bool `blueprint:"mutated"`
}

type Prebuilt struct {
	properties PrebuiltProperties
	module     Module
	srcs       *[]string

	// Metadata for single source Prebuilt modules.
	srcProps reflect.Value
	srcField reflect.StructField
}

func (p *Prebuilt) Name(name string) string {
	return "prebuilt_" + name
}

// The below source-related functions and the srcs, src fields are based on an assumption that
// prebuilt modules have a static source property at the moment. Currently there is only one
// exception, android_app_import, which chooses a source file depending on the product's DPI
// preference configs. We'll want to add native support for dynamic source cases if we end up having
// more modules like this.
func (p *Prebuilt) SingleSourcePath(ctx ModuleContext) Path {
	if p.srcs != nil {
		if len(*p.srcs) == 0 {
			ctx.PropertyErrorf("srcs", "missing prebuilt source file")
			return nil
		}

		if len(*p.srcs) > 1 {
			ctx.PropertyErrorf("srcs", "multiple prebuilt source files")
			return nil
		}

		// Return the singleton source after expanding any filegroup in the
		// sources.
		return PathForModuleSrc(ctx, (*p.srcs)[0])
	} else {
		if !p.srcProps.IsValid() {
			ctx.ModuleErrorf("prebuilt source was not set")
		}
		src := p.getSingleSourceFieldValue()
		if src == "" {
			ctx.PropertyErrorf(proptools.FieldNameForProperty(p.srcField.Name),
				"missing prebuilt source file")
			return nil
		}
		return PathForModuleSrc(ctx, src)
	}
}

func (p *Prebuilt) UsePrebuilt() bool {
	return p.properties.UsePrebuilt
}

func InitPrebuiltModule(module PrebuiltInterface, srcs *[]string) {
	p := module.Prebuilt()
	module.AddProperties(&p.properties)
	p.srcs = srcs
}

func InitSingleSourcePrebuiltModule(module PrebuiltInterface, srcProps interface{}, srcField string) {
	p := module.Prebuilt()
	module.AddProperties(&p.properties)
	p.srcProps = reflect.ValueOf(srcProps).Elem()
	p.srcField, _ = p.srcProps.Type().FieldByName(srcField)
	p.checkSingleSourceProperties()
}

type PrebuiltInterface interface {
	Module
	Prebuilt() *Prebuilt
}

func RegisterPrebuiltsPreArchMutators(ctx RegisterMutatorsContext) {
	ctx.BottomUp("prebuilts", PrebuiltMutator).Parallel()
}

func RegisterPrebuiltsPostDepsMutators(ctx RegisterMutatorsContext) {
	ctx.TopDown("prebuilt_select", PrebuiltSelectModuleMutator).Parallel()
	ctx.BottomUp("prebuilt_postdeps", PrebuiltPostDepsMutator).Parallel()
}

// PrebuiltMutator ensures that there is always a module with an undecorated name, and marks
// prebuilt modules that have both a prebuilt and a source module.
func PrebuiltMutator(ctx BottomUpMutatorContext) {
	if m, ok := ctx.Module().(PrebuiltInterface); ok && m.Prebuilt() != nil {
		p := m.Prebuilt()
		name := m.base().BaseModuleName()
		if ctx.OtherModuleExists(name) {
			ctx.AddReverseDependency(ctx.Module(), prebuiltDepTag, name)
			p.properties.SourceExists = true
		} else {
			ctx.Rename(name)
		}
	}
}

// PrebuiltSelectModuleMutator marks prebuilts that are used, either overriding source modules or
// because the source module doesn't exist.  It also disables installing overridden source modules.
func PrebuiltSelectModuleMutator(ctx TopDownMutatorContext) {
	if m, ok := ctx.Module().(PrebuiltInterface); ok && m.Prebuilt() != nil {
		p := m.Prebuilt()
		if p.srcs == nil && !p.srcProps.IsValid() {
			panic(fmt.Errorf("prebuilt module did not have InitPrebuiltModule called on it"))
		}
		if !p.properties.SourceExists {
			p.properties.UsePrebuilt = p.usePrebuilt(ctx, nil)
		}
	} else if s, ok := ctx.Module().(Module); ok {
		ctx.VisitDirectDepsWithTag(prebuiltDepTag, func(m Module) {
			p := m.(PrebuiltInterface).Prebuilt()
			if p.usePrebuilt(ctx, s) {
				p.properties.UsePrebuilt = true
				s.SkipInstall()
			}
		})
	}
}

// PrebuiltPostDepsMutator does two operations.  It replace dependencies on the
// source module with dependencies on the prebuilt when both modules exist and
// the prebuilt should be used.  When the prebuilt should not be used, disable
// installing it.  Secondly, it also adds a sourcegroup to any filegroups found
// in the prebuilt's 'Srcs' property.
func PrebuiltPostDepsMutator(ctx BottomUpMutatorContext) {
	if m, ok := ctx.Module().(PrebuiltInterface); ok && m.Prebuilt() != nil {
		p := m.Prebuilt()
		name := m.base().BaseModuleName()
		if p.properties.UsePrebuilt {
			if p.properties.SourceExists {
				ctx.ReplaceDependencies(name)
			}
		} else {
			m.SkipInstall()
		}
	}
}

// usePrebuilt returns true if a prebuilt should be used instead of the source module.  The prebuilt
// will be used if it is marked "prefer" or if the source module is disabled.
func (p *Prebuilt) usePrebuilt(ctx TopDownMutatorContext, source Module) bool {
	if p.srcs != nil && len(*p.srcs) == 0 {
		return false
	}

	if p.srcProps.IsValid() && p.getSingleSourceFieldValue() == "" {
		return false
	}

	// TODO: use p.Properties.Name and ctx.ModuleDir to override preference
	if Bool(p.properties.Prefer) {
		return true
	}

	return source == nil || !source.Enabled()
}

func (p *Prebuilt) SourceExists() bool {
	return p.properties.SourceExists
}

func (p *Prebuilt) checkSingleSourceProperties() {
	if !p.srcProps.IsValid() || p.srcField.Name == "" {
		panic(fmt.Errorf("invalid single source prebuilt %+v", p))
	}

	if p.srcProps.Kind() != reflect.Struct && p.srcProps.Kind() != reflect.Interface {
		panic(fmt.Errorf("invalid single source prebuilt %+v", p.srcProps))
	}
}

func (p *Prebuilt) getSingleSourceFieldValue() string {
	value := p.srcProps.FieldByIndex(p.srcField.Index)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}
	if value.Kind() != reflect.String {
		panic(fmt.Errorf("prebuilt src field %q should be a string or a pointer to one", p.srcField.Name))
	}
	return value.String()
}
