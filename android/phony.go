// Copyright 2020 Google Inc. All rights reserved.
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
	"sync"

	"github.com/google/blueprint"
)

var phonyMapOnceKey = NewOnceKey("phony")

type phonyMap map[string]Paths

var phonyMapLock sync.Mutex

func getPhonyMap(config Config) phonyMap {
	return config.Once(phonyMapOnceKey, func() interface{} {
		return make(phonyMap)
	}).(phonyMap)
}

func addPhony(config Config, name string, deps ...Path) {
	phonyMap := getPhonyMap(config)
	phonyMapLock.Lock()
	defer phonyMapLock.Unlock()
	phonyMap[name] = append(phonyMap[name], deps...)
}

type phonySingleton struct {
	phonyMap  phonyMap
	phonyList []string
}

var _ SingletonMakeVarsProvider = (*phonySingleton)(nil)

func (p *phonySingleton) GenerateBuildActions(ctx SingletonContext) {
	p.phonyMap = getPhonyMap(ctx.Config())
	p.phonyList = SortedStringKeys(p.phonyMap)
	for _, phony := range p.phonyList {
		p.phonyMap[phony] = SortedUniquePaths(p.phonyMap[phony])
	}

	if !ctx.Config().EmbeddedInMake() {
		for _, phony := range p.phonyList {
			ctx.Build(pctx, BuildParams{
				Rule:      blueprint.Phony,
				Outputs:   []WritablePath{PathForPhony(ctx, phony)},
				Implicits: p.phonyMap[phony],
			})
		}
	}
}

func (p phonySingleton) MakeVars(ctx MakeVarsContext) {
	for _, phony := range p.phonyList {
		ctx.Phony(phony, p.phonyMap[phony]...)
	}
}

func phonySingletonFactory() Singleton {
	return &phonySingleton{}
}
