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

package common

import (
	"blueprint"
)

func CheckbuildSingleton() blueprint.Singleton {
	return &checkbuildSingleton{}
}

type checkbuildSingleton struct{}

func (c *checkbuildSingleton) GenerateBuildActions(ctx blueprint.SingletonContext) {
	deps := []string{}
	ctx.VisitAllModules(func(module blueprint.Module) {
		if a, ok := module.(AndroidModule); ok {
			if len(a.base().checkbuildFiles) > 0 {
				deps = append(deps, ctx.ModuleName(module)+"-checkbuild")
			}
		}
	})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      blueprint.Phony,
		Outputs:   []string{"checkbuild"},
		Implicits: deps,
		Optional:  true,
	})
}
