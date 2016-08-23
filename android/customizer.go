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

import "github.com/google/blueprint/proptools"

type CustomizePropertiesContext interface {
	BaseContext
	AppendProperties(...interface{})
	PrependProperties(...interface{})
}

type customizePropertiesContext struct {
	BaseContext

	module *ModuleBase
}

type PropertyCustomizer interface {
	CustomizeProperties(CustomizePropertiesContext)
}

func customizerMutator(ctx TopDownMutatorContext) {
	if m, ok := ctx.Module().(Module); ok {
		a := m.base()
		if len(a.customizers) > 0 {
			mctx := &customizePropertiesContext{
				BaseContext: ctx,
				module:      a,
			}
			for _, c := range a.customizers {
				c.CustomizeProperties(mctx)
				if mctx.Failed() {
					return
				}
			}
		}
	}
}

func (ctx *customizePropertiesContext) AppendProperties(props ...interface{}) {
	for _, p := range props {
		err := proptools.AppendMatchingProperties(ctx.module.customizableProperties, p, nil)
		if err != nil {
			if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
				ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
			} else {
				panic(err)
			}
		}
	}
}

func (ctx *customizePropertiesContext) PrependProperties(props ...interface{}) {
	for _, p := range props {
		err := proptools.PrependMatchingProperties(ctx.module.customizableProperties, p, nil)
		if err != nil {
			if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
				ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
			} else {
				panic(err)
			}
		}
	}
}
