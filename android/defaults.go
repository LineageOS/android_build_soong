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
	"reflect"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type defaultsDependencyTag struct {
	blueprint.BaseDependencyTag
}

var DefaultsDepTag defaultsDependencyTag

type defaultsProperties struct {
	Defaults []string
}

type DefaultableModuleBase struct {
	defaultsProperties            defaultsProperties
	defaultableProperties         []interface{}
	defaultableVariableProperties interface{}
}

func (d *DefaultableModuleBase) defaults() *defaultsProperties {
	return &d.defaultsProperties
}

func (d *DefaultableModuleBase) setProperties(props []interface{}, variableProperties interface{}) {
	d.defaultableProperties = props
	d.defaultableVariableProperties = variableProperties
}

// Interface that must be supported by any module to which defaults can be applied.
type Defaultable interface {
	// Get a pointer to the struct containing the Defaults property.
	defaults() *defaultsProperties

	// Set the property structures into which defaults will be added.
	setProperties(props []interface{}, variableProperties interface{})

	// Apply defaults from the supplied Defaults to the property structures supplied to
	// setProperties(...).
	applyDefaults(TopDownMutatorContext, []Defaults)
}

type DefaultableModule interface {
	Module
	Defaultable
}

var _ Defaultable = (*DefaultableModuleBase)(nil)

func InitDefaultableModule(module DefaultableModule) {
	if module.(Module).base().module == nil {
		panic("InitAndroidModule must be called before InitDefaultableModule")
	}
	module.setProperties(module.(Module).GetProperties(), module.(Module).base().variableProperties)

	module.AddProperties(module.defaults())
}

// The Defaults_visibility property.
type DefaultsVisibilityProperties struct {

	// Controls the visibility of the defaults module itself.
	Defaults_visibility []string
}

type DefaultsModuleBase struct {
	DefaultableModuleBase

	// Container for defaults of the common properties
	commonProperties commonProperties

	defaultsVisibilityProperties DefaultsVisibilityProperties
}

// The common pattern for defaults modules is to register separate instances of
// the xxxProperties structs in the AddProperties calls, rather than reusing the
// ones inherited from Module.
//
// The effect is that e.g. myDefaultsModuleInstance.base().xxxProperties won't
// contain the values that have been set for the defaults module. Rather, to
// retrieve the values it is necessary to iterate over properties(). E.g. to get
// the commonProperties instance that have the real values:
//
//   d := myModule.(Defaults)
//   for _, props := range d.properties() {
//     if cp, ok := props.(*commonProperties); ok {
//       ... access property values in cp ...
//     }
//   }
//
// The rationale is that the properties on a defaults module apply to the
// defaultable modules using it, not to the defaults module itself. E.g. setting
// the "enabled" property false makes inheriting modules disabled by default,
// rather than disabling the defaults module itself.
type Defaults interface {
	Defaultable

	// Although this function is unused it is actually needed to ensure that only modules that embed
	// DefaultsModuleBase will type-assert to the Defaults interface.
	isDefaults() bool

	// Get the structures containing the properties for which defaults can be provided.
	properties() []interface{}

	productVariableProperties() interface{}

	// Return the defaults common properties.
	common() *commonProperties

	// Return the defaults visibility properties.
	defaultsVisibility() *DefaultsVisibilityProperties
}

func (d *DefaultsModuleBase) isDefaults() bool {
	return true
}

type DefaultsModule interface {
	Module
	Defaults
}

func (d *DefaultsModuleBase) properties() []interface{} {
	return d.defaultableProperties
}

func (d *DefaultsModuleBase) productVariableProperties() interface{} {
	return d.defaultableVariableProperties
}

func (d *DefaultsModuleBase) common() *commonProperties {
	return &d.commonProperties
}

func (d *DefaultsModuleBase) defaultsVisibility() *DefaultsVisibilityProperties {
	return &d.defaultsVisibilityProperties
}

func (d *DefaultsModuleBase) GenerateAndroidBuildActions(ctx ModuleContext) {
}

func InitDefaultsModule(module DefaultsModule) {
	commonProperties := module.common()

	module.AddProperties(
		&hostAndDeviceProperties{},
		commonProperties,
		&ApexProperties{})

	initAndroidModuleBase(module)
	initProductVariableModule(module)
	InitArchModule(module)
	InitDefaultableModule(module)

	// Add properties that will not have defaults applied to them.
	base := module.base()
	defaultsVisibility := module.defaultsVisibility()
	module.AddProperties(&base.nameProperties, defaultsVisibility)

	// The defaults_visibility property controls the visibility of a defaults module.
	base.primaryVisibilityProperty =
		newVisibilityProperty("defaults_visibility", &defaultsVisibility.Defaults_visibility)

	// Unlike non-defaults modules the visibility property is not stored in m.base().commonProperties.
	// Instead it is stored in a separate instance of commonProperties created above so use that.
	// The visibility property needs to be checked (but not parsed) by the visibility module during
	// its checking phase and parsing phase.
	base.visibilityPropertyInfo = []visibilityProperty{
		base.primaryVisibilityProperty,
		newVisibilityProperty("visibility", &commonProperties.Visibility),
	}

	base.module = module
}

var _ Defaults = (*DefaultsModuleBase)(nil)

func (defaultable *DefaultableModuleBase) applyDefaults(ctx TopDownMutatorContext,
	defaultsList []Defaults) {

	for _, defaults := range defaultsList {
		for _, prop := range defaultable.defaultableProperties {
			if prop == defaultable.defaultableVariableProperties {
				defaultable.applyDefaultVariableProperties(ctx, defaults, prop)
			} else {
				defaultable.applyDefaultProperties(ctx, defaults, prop)
			}
		}
	}
}

// Product variable properties need special handling, the type of the filtered product variable
// property struct may not be identical between the defaults module and the defaultable module.
// Use PrependMatchingProperties to apply whichever properties match.
func (defaultable *DefaultableModuleBase) applyDefaultVariableProperties(ctx TopDownMutatorContext,
	defaults Defaults, defaultableProp interface{}) {
	if defaultableProp == nil {
		return
	}

	defaultsProp := defaults.productVariableProperties()
	if defaultsProp == nil {
		return
	}

	dst := []interface{}{
		defaultableProp,
		// Put an empty copy of the src properties into dst so that properties in src that are not in dst
		// don't cause a "failed to find property to extend" error.
		proptools.CloneEmptyProperties(reflect.ValueOf(defaultsProp)).Interface(),
	}

	err := proptools.PrependMatchingProperties(dst, defaultsProp, nil)
	if err != nil {
		if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
			ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
		} else {
			panic(err)
		}
	}
}

func (defaultable *DefaultableModuleBase) applyDefaultProperties(ctx TopDownMutatorContext,
	defaults Defaults, defaultableProp interface{}) {

	for _, def := range defaults.properties() {
		if proptools.TypeEqual(defaultableProp, def) {
			err := proptools.PrependProperties(defaultableProp, def, nil)
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

func RegisterDefaultsPreArchMutators(ctx RegisterMutatorsContext) {
	ctx.BottomUp("defaults_deps", defaultsDepsMutator).Parallel()
	ctx.TopDown("defaults", defaultsMutator).Parallel()
}

func defaultsDepsMutator(ctx BottomUpMutatorContext) {
	if defaultable, ok := ctx.Module().(Defaultable); ok {
		ctx.AddDependency(ctx.Module(), DefaultsDepTag, defaultable.defaults().Defaults...)
	}
}

func defaultsMutator(ctx TopDownMutatorContext) {
	if defaultable, ok := ctx.Module().(Defaultable); ok && len(defaultable.defaults().Defaults) > 0 {
		var defaultsList []Defaults
		seen := make(map[Defaults]bool)

		ctx.WalkDeps(func(module, parent Module) bool {
			if ctx.OtherModuleDependencyTag(module) == DefaultsDepTag {
				if defaults, ok := module.(Defaults); ok {
					if !seen[defaults] {
						seen[defaults] = true
						defaultsList = append(defaultsList, defaults)
						return len(defaults.defaults().Defaults) > 0
					}
				} else {
					ctx.PropertyErrorf("defaults", "module %s is not an defaults module",
						ctx.OtherModuleName(module))
				}
			}
			return false
		})
		defaultable.applyDefaults(ctx, defaultsList)
	}
}
