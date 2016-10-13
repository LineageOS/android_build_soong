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

type DefaultableModule struct {
	defaultsProperties    defaultsProperties
	defaultableProperties []interface{}
}

func (d *DefaultableModule) defaults() *defaultsProperties {
	return &d.defaultsProperties
}

func (d *DefaultableModule) setProperties(props []interface{}) {
	d.defaultableProperties = props
}

type Defaultable interface {
	defaults() *defaultsProperties
	setProperties([]interface{})
	applyDefaults(TopDownMutatorContext, []Defaults)
}

var _ Defaultable = (*DefaultableModule)(nil)

func InitDefaultableModule(module Module, d Defaultable,
	props ...interface{}) (blueprint.Module, []interface{}) {

	d.setProperties(props)

	props = append(props, d.defaults())

	return module, props
}

type DefaultsModule struct {
	DefaultableModule
	defaultProperties []interface{}
}

type Defaults interface {
	Defaultable
	isDefaults() bool
	properties() []interface{}
}

func (d *DefaultsModule) isDefaults() bool {
	return true
}

func (d *DefaultsModule) properties() []interface{} {
	return d.defaultableProperties
}

func InitDefaultsModule(module Module, d Defaults, props ...interface{}) (blueprint.Module, []interface{}) {
	props = append(props,
		&hostAndDeviceProperties{},
		&commonProperties{},
		&variableProperties{})

	_, props = InitArchModule(module, props...)

	_, props = InitDefaultableModule(module, d, props...)

	props = append(props, &module.base().nameProperties)

	module.base().module = module

	return module, props
}

var _ Defaults = (*DefaultsModule)(nil)

func (defaultable *DefaultableModule) applyDefaults(ctx TopDownMutatorContext,
	defaultsList []Defaults) {

	for _, defaults := range defaultsList {
		for _, prop := range defaultable.defaultableProperties {
			for _, def := range defaults.properties() {
				if proptools.TypeEqual(prop, def) {
					err := proptools.PrependProperties(prop, def, nil)
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
	}
}

func defaultsDepsMutator(ctx BottomUpMutatorContext) {
	if defaultable, ok := ctx.Module().(Defaultable); ok {
		ctx.AddDependency(ctx.Module(), DefaultsDepTag, defaultable.defaults().Defaults...)
	}
}

func defaultsMutator(ctx TopDownMutatorContext) {
	if defaultable, ok := ctx.Module().(Defaultable); ok && len(defaultable.defaults().Defaults) > 0 {
		var defaultsList []Defaults
		ctx.WalkDeps(func(module, parent blueprint.Module) bool {
			if ctx.OtherModuleDependencyTag(module) == DefaultsDepTag {
				if defaults, ok := module.(Defaults); ok {
					defaultsList = append(defaultsList, defaults)
					return len(defaults.defaults().Defaults) > 0
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
