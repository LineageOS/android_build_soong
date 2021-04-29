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
	"strings"

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

	// Set if the module has been renamed to remove the "prebuilt_" prefix.
	PrebuiltRenamedToSource bool `blueprint:"mutated"`
}

type Prebuilt struct {
	properties PrebuiltProperties

	srcsSupplier     PrebuiltSrcsSupplier
	srcsPropertyName string
}

// RemoveOptionalPrebuiltPrefix returns the result of removing the "prebuilt_" prefix from the
// supplied name if it has one, or returns the name unmodified if it does not.
func RemoveOptionalPrebuiltPrefix(name string) string {
	return strings.TrimPrefix(name, "prebuilt_")
}

func (p *Prebuilt) Name(name string) string {
	return PrebuiltNameFromSource(name)
}

// PrebuiltNameFromSource returns the result of prepending the "prebuilt_" prefix to the supplied
// name.
func PrebuiltNameFromSource(name string) string {
	return "prebuilt_" + name
}

func (p *Prebuilt) ForcePrefer() {
	p.properties.Prefer = proptools.BoolPtr(true)
}

func (p *Prebuilt) Prefer() bool {
	return proptools.Bool(p.properties.Prefer)
}

// SingleSourcePathFromSupplier invokes the supplied supplier for the current module in the
// supplied context to retrieve a list of file paths, ensures that the returned list of file paths
// contains a single value and then assumes that is a module relative file path and converts it to
// a Path accordingly.
//
// Any issues, such as nil supplier or not exactly one file path will be reported as errors on the
// supplied context and this will return nil.
func SingleSourcePathFromSupplier(ctx ModuleContext, srcsSupplier PrebuiltSrcsSupplier, srcsPropertyName string) Path {
	if srcsSupplier != nil {
		srcs := srcsSupplier(ctx, ctx.Module())

		if len(srcs) == 0 {
			ctx.PropertyErrorf(srcsPropertyName, "missing prebuilt source file")
			return nil
		}

		if len(srcs) > 1 {
			ctx.PropertyErrorf(srcsPropertyName, "multiple prebuilt source files")
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

// The below source-related functions and the srcs, src fields are based on an assumption that
// prebuilt modules have a static source property at the moment. Currently there is only one
// exception, android_app_import, which chooses a source file depending on the product's DPI
// preference configs. We'll want to add native support for dynamic source cases if we end up having
// more modules like this.
func (p *Prebuilt) SingleSourcePath(ctx ModuleContext) Path {
	return SingleSourcePathFromSupplier(ctx, p.srcsSupplier, p.srcsPropertyName)
}

func (p *Prebuilt) UsePrebuilt() bool {
	return p.properties.UsePrebuilt
}

// Called to provide the srcs value for the prebuilt module.
//
// This can be called with a context for any module not just the prebuilt one itself. It can also be
// called concurrently.
//
// Return the src value or nil if it is not available.
type PrebuiltSrcsSupplier func(ctx BaseModuleContext, prebuilt Module) []string

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

	srcsSupplier := func(ctx BaseModuleContext, _ Module) []string {
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

	srcsSupplier := func(ctx BaseModuleContext, _ Module) []string {
		if !module.Enabled() {
			return nil
		}
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

// IsModulePreferred returns true if the given module is preferred.
//
// A source module is preferred if there is no corresponding prebuilt module or the prebuilt module
// does not have "prefer: true".
//
// A prebuilt module is preferred if there is no corresponding source module or the prebuilt module
// has "prefer: true".
func IsModulePreferred(module Module) bool {
	if module.IsReplacedByPrebuilt() {
		// A source module that has been replaced by a prebuilt counterpart.
		return false
	}
	if p := GetEmbeddedPrebuilt(module); p != nil {
		return p.UsePrebuilt()
	}
	return true
}

// IsModulePrebuilt returns true if the module implements PrebuiltInterface and
// has been initialized as a prebuilt and so returns a non-nil value from the
// PrebuiltInterface.Prebuilt() method.
func IsModulePrebuilt(module Module) bool {
	return GetEmbeddedPrebuilt(module) != nil
}

// GetEmbeddedPrebuilt returns a pointer to the embedded Prebuilt structure or
// nil if the module does not implement PrebuiltInterface or has not been
// initialized as a prebuilt module.
func GetEmbeddedPrebuilt(module Module) *Prebuilt {
	if p, ok := module.(PrebuiltInterface); ok {
		return p.Prebuilt()
	}

	return nil
}

func RegisterPrebuiltsPreArchMutators(ctx RegisterMutatorsContext) {
	ctx.BottomUp("prebuilt_rename", PrebuiltRenameMutator).Parallel()
}

func RegisterPrebuiltsPostDepsMutators(ctx RegisterMutatorsContext) {
	ctx.BottomUp("prebuilt_source", PrebuiltSourceDepsMutator).Parallel()
	ctx.TopDown("prebuilt_select", PrebuiltSelectModuleMutator).Parallel()
	ctx.BottomUp("prebuilt_postdeps", PrebuiltPostDepsMutator).Parallel()
}

// PrebuiltRenameMutator ensures that there always is a module with an
// undecorated name.
func PrebuiltRenameMutator(ctx BottomUpMutatorContext) {
	m := ctx.Module()
	if p := GetEmbeddedPrebuilt(m); p != nil {
		name := m.base().BaseModuleName()
		if !ctx.OtherModuleExists(name) {
			ctx.Rename(name)
			p.properties.PrebuiltRenamedToSource = true
		}
	}
}

// PrebuiltSourceDepsMutator adds dependencies to the prebuilt module from the
// corresponding source module, if one exists for the same variant.
func PrebuiltSourceDepsMutator(ctx BottomUpMutatorContext) {
	m := ctx.Module()
	// If this module is a prebuilt, is enabled and has not been renamed to source then add a
	// dependency onto the source if it is present.
	if p := GetEmbeddedPrebuilt(m); p != nil && m.Enabled() && !p.properties.PrebuiltRenamedToSource {
		name := m.base().BaseModuleName()
		if ctx.OtherModuleReverseDependencyVariantExists(name) {
			ctx.AddReverseDependency(ctx.Module(), PrebuiltDepTag, name)
			p.properties.SourceExists = true
		}
	}
}

// PrebuiltSelectModuleMutator marks prebuilts that are used, either overriding source modules or
// because the source module doesn't exist.  It also disables installing overridden source modules.
func PrebuiltSelectModuleMutator(ctx TopDownMutatorContext) {
	m := ctx.Module()
	if p := GetEmbeddedPrebuilt(m); p != nil {
		if p.srcsSupplier == nil {
			panic(fmt.Errorf("prebuilt module did not have InitPrebuiltModule called on it"))
		}
		if !p.properties.SourceExists {
			p.properties.UsePrebuilt = p.usePrebuilt(ctx, nil, m)
		}
	} else if s, ok := ctx.Module().(Module); ok {
		ctx.VisitDirectDepsWithTag(PrebuiltDepTag, func(prebuiltModule Module) {
			p := GetEmbeddedPrebuilt(prebuiltModule)
			if p.usePrebuilt(ctx, s, prebuiltModule) {
				p.properties.UsePrebuilt = true
				s.ReplacedByPrebuilt()
			}
		})
	}
}

// PrebuiltPostDepsMutator replaces dependencies on the source module with dependencies on the
// prebuilt when both modules exist and the prebuilt should be used.  When the prebuilt should not
// be used, disable installing it.
func PrebuiltPostDepsMutator(ctx BottomUpMutatorContext) {
	m := ctx.Module()
	if p := GetEmbeddedPrebuilt(m); p != nil {
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
			m.HideFromMake()
		}
	}
}

// usePrebuilt returns true if a prebuilt should be used instead of the source module.  The prebuilt
// will be used if it is marked "prefer" or if the source module is disabled.
func (p *Prebuilt) usePrebuilt(ctx TopDownMutatorContext, source Module, prebuilt Module) bool {
	if p.srcsSupplier != nil && len(p.srcsSupplier(ctx, prebuilt)) == 0 {
		return false
	}

	// Skip prebuilt modules under unexported namespaces so that we won't
	// end up shadowing non-prebuilt module when prebuilt module under same
	// name happens to have a `Prefer` property set to true.
	if ctx.Config().KatiEnabled() && !prebuilt.ExportedToMake() {
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
