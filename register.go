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

package soong

import "github.com/google/blueprint"

type moduleType struct {
	name    string
	factory blueprint.ModuleFactory
}

var moduleTypes []moduleType

type singleton struct {
	name    string
	factory blueprint.SingletonFactory
}

var singletons []singleton

type mutator struct {
	name            string
	bottomUpMutator blueprint.BottomUpMutator
	topDownMutator  blueprint.TopDownMutator
	parallel        bool
}

var mutators []*mutator

func RegisterModuleType(name string, factory blueprint.ModuleFactory) {
	moduleTypes = append(moduleTypes, moduleType{name, factory})
}

func RegisterSingletonType(name string, factory blueprint.SingletonFactory) {
	singletons = append(singletons, singleton{name, factory})
}

func RegisterBottomUpMutator(name string, m blueprint.BottomUpMutator) BottomUpMutatorHandle {
	mutator := &mutator{name: name, bottomUpMutator: m}
	mutators = append(mutators, mutator)
	return mutator
}

func RegisterTopDownMutator(name string, m blueprint.TopDownMutator) {
	mutators = append(mutators, &mutator{name: name, topDownMutator: m})
}

type BottomUpMutatorHandle interface {
	Parallel() BottomUpMutatorHandle
}

func (mutator *mutator) Parallel() BottomUpMutatorHandle {
	mutator.parallel = true
	return mutator
}

func NewContext() *blueprint.Context {
	ctx := blueprint.NewContext()

	for _, t := range moduleTypes {
		ctx.RegisterModuleType(t.name, t.factory)
	}

	for _, t := range singletons {
		ctx.RegisterSingletonType(t.name, t.factory)
	}

	for _, t := range mutators {
		if t.bottomUpMutator != nil {
			handle := ctx.RegisterBottomUpMutator(t.name, t.bottomUpMutator)
			if t.parallel {
				handle.Parallel()
			}
		}
		if t.topDownMutator != nil {
			ctx.RegisterTopDownMutator(t.name, t.topDownMutator)
		}
	}

	return ctx
}
