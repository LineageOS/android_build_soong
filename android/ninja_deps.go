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
	"android/soong/starlark_import"
	"sort"
)

func (c *config) addNinjaFileDeps(deps ...string) {
	for _, dep := range deps {
		c.ninjaFileDepsSet.Store(dep, true)
	}
}

func (c *config) ninjaFileDeps() []string {
	var deps []string
	c.ninjaFileDepsSet.Range(func(key, value interface{}) bool {
		deps = append(deps, key.(string))
		return true
	})
	sort.Strings(deps)
	return deps
}

func ninjaDepsSingletonFactory() Singleton {
	return &ninjaDepsSingleton{}
}

type ninjaDepsSingleton struct{}

func (ninjaDepsSingleton) GenerateBuildActions(ctx SingletonContext) {
	ctx.AddNinjaFileDeps(ctx.Config().ninjaFileDeps()...)

	deps, err := starlark_import.GetNinjaDeps()
	if err != nil {
		ctx.Errorf("Error running starlark code: %s", err)
	} else {
		ctx.AddNinjaFileDeps(deps...)
	}
}
