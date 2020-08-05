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

package rust

import (
	"encoding/json"
	"fmt"
	"path"

	"android/soong/android"
)

// This singleton collects Rust crate definitions and generates a JSON file
// (${OUT_DIR}/soong/rust-project.json) which can be use by external tools,
// such as rust-analyzer. It does so when either make, mm, mma, mmm or mmma is
// called.  This singleton is enabled only if SOONG_GEN_RUST_PROJECT is set.
// For example,
//
//   $ SOONG_GEN_RUST_PROJECT=1 m nothing

func init() {
	android.RegisterSingletonType("rust_project_generator", rustProjectGeneratorSingleton)
}

func rustProjectGeneratorSingleton() android.Singleton {
	return &projectGeneratorSingleton{}
}

type projectGeneratorSingleton struct{}

const (
	// Environment variables used to control the behavior of this singleton.
	envVariableCollectRustDeps = "SOONG_GEN_RUST_PROJECT"
	rustProjectJsonFileName    = "rust-project.json"
)

// The format of rust-project.json is not yet finalized. A current description is available at:
// https://github.com/rust-analyzer/rust-analyzer/blob/master/docs/user/manual.adoc#non-cargo-based-projects
type rustProjectDep struct {
	Crate int    `json:"crate"`
	Name  string `json:"name"`
}

type rustProjectCrate struct {
	RootModule string           `json:"root_module"`
	Edition    string           `json:"edition,omitempty"`
	Deps       []rustProjectDep `json:"deps"`
	Cfgs       []string         `json:"cfgs"`
}

type rustProjectJson struct {
	Roots  []string           `json:"roots"`
	Crates []rustProjectCrate `json:"crates"`
}

// crateInfo is used during the processing to keep track of the known crates.
type crateInfo struct {
	ID   int
	Deps map[string]int
}

func mergeDependencies(ctx android.SingletonContext, project *rustProjectJson,
	knownCrates map[string]crateInfo, module android.Module,
	crate *rustProjectCrate, deps map[string]int) {

	ctx.VisitDirectDeps(module, func(child android.Module) {
		childId, childCrateName, ok := appendLibraryAndDeps(ctx, project, knownCrates, child)
		if !ok {
			return
		}
		if _, ok = deps[ctx.ModuleName(child)]; ok {
			return
		}
		crate.Deps = append(crate.Deps, rustProjectDep{Crate: childId, Name: childCrateName})
		deps[ctx.ModuleName(child)] = childId
	})
}

// appendLibraryAndDeps creates a rustProjectCrate for the module argument and
// appends it to the rustProjectJson struct.  It visits the dependencies of the
// module depth-first. If the current module is already in knownCrates, its
// dependencies are merged. Returns a tuple (id, crate_name, ok).
func appendLibraryAndDeps(ctx android.SingletonContext, project *rustProjectJson,
	knownCrates map[string]crateInfo, module android.Module) (int, string, bool) {
	rModule, ok := module.(*Module)
	if !ok {
		return 0, "", false
	}
	if rModule.compiler == nil {
		return 0, "", false
	}
	rustLib, ok := rModule.compiler.(*libraryDecorator)
	if !ok {
		return 0, "", false
	}
	moduleName := ctx.ModuleName(module)
	crateName := rModule.CrateName()
	if cInfo, ok := knownCrates[moduleName]; ok {
		// We have seen this crate already; merge any new dependencies.
		crate := project.Crates[cInfo.ID]
		mergeDependencies(ctx, project, knownCrates, module, &crate, cInfo.Deps)
		project.Crates[cInfo.ID] = crate
		return cInfo.ID, crateName, true
	}
	crate := rustProjectCrate{Deps: make([]rustProjectDep, 0), Cfgs: make([]string, 0)}
	srcs := rustLib.baseCompiler.Properties.Srcs
	if len(srcs) == 0 {
		return 0, "", false
	}
	crate.RootModule = path.Join(ctx.ModuleDir(rModule), srcs[0])
	crate.Edition = rustLib.baseCompiler.edition()

	deps := make(map[string]int)
	mergeDependencies(ctx, project, knownCrates, module, &crate, deps)

	id := len(project.Crates)
	knownCrates[moduleName] = crateInfo{ID: id, Deps: deps}
	project.Crates = append(project.Crates, crate)
	// rust-analyzer requires that all crates belong to at least one root:
	// https://github.com/rust-analyzer/rust-analyzer/issues/4735.
	project.Roots = append(project.Roots, path.Dir(crate.RootModule))
	return id, crateName, true
}

func (r *projectGeneratorSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	if !ctx.Config().IsEnvTrue(envVariableCollectRustDeps) {
		return
	}

	project := rustProjectJson{}
	knownCrates := make(map[string]crateInfo)
	ctx.VisitAllModules(func(module android.Module) {
		appendLibraryAndDeps(ctx, &project, knownCrates, module)
	})

	path := android.PathForOutput(ctx, rustProjectJsonFileName)
	err := createJsonFile(project, path)
	if err != nil {
		ctx.Errorf(err.Error())
	}
}

func createJsonFile(project rustProjectJson, rustProjectPath android.WritablePath) error {
	buf, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON marshal of rustProjectJson failed: %s", err)
	}
	err = android.WriteFileToOutputDir(rustProjectPath, buf, 0666)
	if err != nil {
		return fmt.Errorf("Writing rust-project to %s failed: %s", rustProjectPath.String(), err)
	}
	return nil
}
