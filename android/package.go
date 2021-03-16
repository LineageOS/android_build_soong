// Copyright 2019 Google Inc. All rights reserved.
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
	"sync/atomic"

	"github.com/google/blueprint"
)

func init() {
	RegisterModuleType("package", PackageFactory)
}

type packageProperties struct {
	// Specifies the default visibility for all modules defined in this package.
	Default_visibility []string
	// Specifies the names of the default licenses for all modules defined in this package.
	Default_applicable_licenses []string
}

type packageModule struct {
	ModuleBase

	properties packageProperties
	// The module dir
	name string `blueprint:"mutated"`
}

func (p *packageModule) GenerateAndroidBuildActions(ModuleContext) {
	// Nothing to do.
}

func (p *packageModule) GenerateBuildActions(ctx blueprint.ModuleContext) {
	// Nothing to do.
}

func (p *packageModule) DepsMutator(ctx BottomUpMutatorContext) {
	// Nothing to do.
}

func (p *packageModule) Name() string {
	return p.name
}

func registerPackageRenamer(ctx RegisterMutatorsContext) {
	ctx.BottomUp("packages", packageRenamer).Parallel()
}

// packageRenamer ensures that every package gets named
func packageRenamer(ctx BottomUpMutatorContext) {
	if p, ok := ctx.Module().(*packageModule); ok {
		p.name = "//" + ctx.ModuleDir()
		ctx.Rename("//" + ctx.ModuleDir())
	}
}

// Counter to ensure package modules are created with a unique name within whatever namespace they
// belong.
var packageCount uint32 = 0

func PackageFactory() Module {
	module := &packageModule{}

	// Get a unique if for the package. Has to be done atomically as the creation of the modules are
	// done in parallel.
	id := atomic.AddUint32(&packageCount, 1)
	module.name = fmt.Sprintf("soong_package_%d", id)

	module.AddProperties(&module.properties)

	// The name is the relative path from build root to the directory containing this
	// module. Set that name at the earliest possible moment that information is available
	// which is in a LoadHook.
	AddLoadHook(module, func(ctx LoadHookContext) {
		module.name = "//" + ctx.ModuleDir()
	})

	return module
}
