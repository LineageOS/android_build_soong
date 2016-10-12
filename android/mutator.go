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

import "github.com/google/blueprint"

type AndroidTopDownMutator func(TopDownMutatorContext)

type TopDownMutatorContext interface {
	blueprint.TopDownMutatorContext
	androidBaseContext
}

type androidTopDownMutatorContext struct {
	blueprint.TopDownMutatorContext
	androidBaseContextImpl
}

type AndroidBottomUpMutator func(BottomUpMutatorContext)

type BottomUpMutatorContext interface {
	blueprint.BottomUpMutatorContext
	androidBaseContext
}

type androidBottomUpMutatorContext struct {
	blueprint.BottomUpMutatorContext
	androidBaseContextImpl
}

func RegisterBottomUpMutator(name string, m AndroidBottomUpMutator) MutatorHandle {
	f := func(ctx blueprint.BottomUpMutatorContext) {
		if a, ok := ctx.Module().(Module); ok {
			actx := &androidBottomUpMutatorContext{
				BottomUpMutatorContext: ctx,
				androidBaseContextImpl: a.base().androidBaseContextFactory(ctx),
			}
			m(actx)
		}
	}
	mutator := &mutator{name: name, bottomUpMutator: f}
	mutators = append(mutators, mutator)
	return mutator
}

func RegisterTopDownMutator(name string, m AndroidTopDownMutator) MutatorHandle {
	f := func(ctx blueprint.TopDownMutatorContext) {
		if a, ok := ctx.Module().(Module); ok {
			actx := &androidTopDownMutatorContext{
				TopDownMutatorContext:  ctx,
				androidBaseContextImpl: a.base().androidBaseContextFactory(ctx),
			}
			m(actx)
		}
	}
	mutator := &mutator{name: name, topDownMutator: f}
	mutators = append(mutators, mutator)
	return mutator
}

type MutatorHandle interface {
	Parallel() MutatorHandle
}

func (mutator *mutator) Parallel() MutatorHandle {
	mutator.parallel = true
	return mutator
}
