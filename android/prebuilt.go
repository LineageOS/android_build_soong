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

func RegisterPrebuiltMutators(ctx RegistrationContext) {
	ctx.PreArchMutators(RegisterPrebuiltsPreArchMutators)
	ctx.PostDepsMutators(RegisterPrebuiltsPostDepsMutators)
}

// Marks a dependency tag as possibly preventing a reference to a source from being
// replaced with the prebuilt.
type ReplaceSourceWithPrebuilt interface {
	blueprint.DependencyTag

	// Return true if the dependency defined by this tag should be replaced with the
	// prebuilt.
	ReplaceSourceWithPrebuilt() bool
}

type prebuiltDependencyTag struct {
	blueprint.BaseDependencyTag
}

var PrebuiltDepTag prebuiltDependencyTag

// Mark this tag so dependencies that use it are excluded from visibility enforcement.
func (t prebuiltDependencyTag) ExcludeFromVisibilityEnforcement() {}

// Mark this tag so dependencies that use it are excluded from APEX contents.
func (t prebuiltDependencyTag) ExcludeFromApexContents() {}

var _ ExcludeFromVisibilityEnforcementTag = PrebuiltDepTag
var _ ExcludeFromApexContentsTag = PrebuiltDepTag

type PrebuiltProperties struct {
	// When prefer is set to true the prebuilt will be used instead of any source module with
	// a matching name.
	Prefer *bool `android:"arch_variant"`

	SourceExists bool `blueprint:"mutated"`
	UsePrebuilt  bool `blueprint:"mutated"`
}

type Prebuilt struct {
	properties PrebuiltProperties

	srcsSupplier     PrebuiltSrcsSupplier
	srcsPropertyName string
}

func (p *Prebuilt) Name(name string) string {
	return "prebuilt_" + name
}

func (p *Prebuilt) ForcePrefer() {
	p.properties.Prefer = proptools.BoolPtr(true)
}

func (p *Prebuilt) Prefer() bool {
	return proptools.Bool(p.properties.Prefer)
}

// The below source-related functions and the srcs, src fields are based on an assumption that
// prebuilt modules have a static source property at the moment. Currently there is only one
// exception, android_app_import, which chooses a source file depending on the product's DPI
// preference configs. We'll want to add native support for dynamic source cases if we end up having
// more modules like this.
func (p *Prebuilt) SingleSourcePath(ctx ModuleContext) Path {
	if p.srcsSupplier != nil {
		srcs := p.srcsSupplier()

		if len(srcs) == 0 {
			ctx.PropertyErrorf(p.srcsPropertyName, "missing prebuilt source file")
			return nil
		}

		if len(srcs) > 1 {
			ctx.PropertyErrorf(p.srcsPropertyName, "multiple prebuilt source files")
			return nil
		}

		// Return the singleton source after expanding any filegroup in the
		// sources.
		src := srcs[0]
		return PathForModuleSrc(ctx, src)
	} else {
		ctx.ModuleErrorf("prebuilt source was not set")
		return nil
	}
}

func (p *Prebuilt) UsePrebuilt() bool {
	return p.properties.UsePrebuilt
}

// Called to provide the srcs value for the prebuilt module.
//
// Return the src value or nil if it is not available.
type PrebuiltSrcsSupplier func() []string

// Initialize the module as a prebuilt module that uses the provided supplier to access the
// prebuilt sources of the module.
//
// The supplier will be called multiple times and must return the same values each time it
// is called. If it returns an empty array (or nil) then the prebuilt module will not be used
// as a replacement for a source module with the same name even if prefer = true.
//
// If the Prebuilt.SingleSourcePath() is called on the module then this must return an array
// containing exactly one source file.
//
// The provided property name is used to provide helpful error messages in the event that
// a problem arises, e.g. calling SingleSourcePath() when more than one source is provided.
func InitPrebuiltModuleWithSrcSupplier(module PrebuiltInterface, srcsSupplier PrebuiltSrcsSupplier, srcsPropertyName string) {
	p := module.Prebuilt()
	module.AddProperties(&p.properties)

	if srcsSupplier == nil {
		panic(fmt.Errorf("srcsSupplier must not be nil"))
	}
	if srcsPropertyName == "" {
		panic(fmt.Errorf("srcsPropertyName must not be empty"))
	}

	p.srcsSupplier = srcsSupplier
	p.srcsPropertyName = srcsPropertyName
}

func InitPrebuiltModule(module PrebuiltInterface, srcs *[]string) {
	if srcs == nil {
		panic(fmt.Errorf("srcs must not be nil"))
	}

	srcsSupplier := func() []string {
		return *srcs
	}

	InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, "srcs")
}

func InitSingleSourcePrebuiltModule(module PrebuiltInterface, srcProps interface{}, srcField string) {
	srcPropsValue := reflect.ValueOf(srcProps).Elem()
	srcStructField, _ := srcPropsValue.Type().FieldByName(srcField)
	if !srcPropsValue.IsValid() || srcStructField.Name == "" {
		panic(fmt.Errorf("invalid single source prebuilt %+v", module))
	}

	if srcPropsValue.Kind() != reflect.Struct && srcPropsValue.Kind() != reflect.Interface {
		panic(fmt.Errorf("invalid single source prebuilt %+v", srcProps))
	}

	srcFieldIndex := srcStructField.Index
	srcPropertyName := proptools.PropertyNameForField(srcField)

	srcsSupplier := func() []string {
		value := srcPropsValue.FieldByIndex(srcFieldIndex)
		if value.Kind() == reflect.Ptr {
			value = value.Elem()
		}
		if value.Kind() != reflect.String {
			panic(fmt.Errorf("prebuilt src field %q should be a string or a pointer to one but was %d %q", srcPropertyName, value.Kind(), value))
		}
		src := value.String()
		if src == "" {
			return nil
		}
		return []string{src}
	}

	InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, srcPropertyName)
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
			ctx.AddReverseDependency(ctx.Module(), PrebuiltDepTag, name)
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
		if p.srcsSupplier == nil {
			panic(fmt.Errorf("prebuilt module did not have InitPrebuiltModule called on it"))
		}
		if !p.properties.SourceExists {
			p.properties.UsePrebuilt = p.usePrebuilt(ctx, nil)
		}
	} else if s, ok := ctx.Module().(Module); ok {
		ctx.VisitDirectDepsWithTag(PrebuiltDepTag, func(m Module) {
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
				ctx.ReplaceDependenciesIf(name, func(from blueprint.Module, tag blueprint.DependencyTag, to blueprint.Module) bool {
					if t, ok := tag.(ReplaceSourceWithPrebuilt); ok {
						return t.ReplaceSourceWithPrebuilt()
					}

					return true
				})
			}
		} else {
			m.SkipInstall()
		}
	}
}

// usePrebuilt returns true if a prebuilt should be used instead of the source module.  The prebuilt
// will be used if it is marked "prefer" or if the source module is disabled.
func (p *Prebuilt) usePrebuilt(ctx TopDownMutatorContext, source Module) bool {
	if p.srcsSupplier != nil && len(p.srcsSupplier()) == 0 {
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
