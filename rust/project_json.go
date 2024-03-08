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

	"android/soong/android"
)

// This singleton collects Rust crate definitions and generates a JSON file
// (${OUT_DIR}/soong/rust-project.json) which can be use by external tools,
// such as rust-analyzer. It does so when either make, mm, mma, mmm or mmma is
// called.  This singleton is enabled only if SOONG_GEN_RUST_PROJECT is set.
// For example,
//
//   $ SOONG_GEN_RUST_PROJECT=1 m nothing

const (
	// Environment variables used to control the behavior of this singleton.
	envVariableCollectRustDeps = "SOONG_GEN_RUST_PROJECT"
	rustProjectJsonFileName    = "rust-project.json"
)

// The format of rust-project.json is not yet finalized. A current description is available at:
// https://github.com/rust-analyzer/rust-analyzer/blob/master/docs/user/manual.adoc#non-cargo-based-projects
type rustProjectDep struct {
	// The Crate attribute is the index of the dependency in the Crates array in rustProjectJson.
	Crate int    `json:"crate"`
	Name  string `json:"name"`
}

type rustProjectCrate struct {
	DisplayName string            `json:"display_name"`
	RootModule  string            `json:"root_module"`
	Edition     string            `json:"edition,omitempty"`
	Deps        []rustProjectDep  `json:"deps"`
	Cfg         []string          `json:"cfg"`
	Env         map[string]string `json:"env"`
	ProcMacro   bool              `json:"is_proc_macro"`
}

type rustProjectJson struct {
	Crates []rustProjectCrate `json:"crates"`
}

// crateInfo is used during the processing to keep track of the known crates.
type crateInfo struct {
	Idx    int            // Index of the crate in rustProjectJson.Crates slice.
	Deps   map[string]int // The keys are the module names and not the crate names.
	Device bool           // True if the crate at idx was a device crate
}

type projectGeneratorSingleton struct {
	project     rustProjectJson
	knownCrates map[string]crateInfo // Keys are module names.
}

func rustProjectGeneratorSingleton() android.Singleton {
	return &projectGeneratorSingleton{}
}

func init() {
	android.RegisterParallelSingletonType("rust_project_generator", rustProjectGeneratorSingleton)
}

// mergeDependencies visits all the dependencies for module and updates crate and deps
// with any new dependency.
func (singleton *projectGeneratorSingleton) mergeDependencies(ctx android.SingletonContext,
	module *Module, crate *rustProjectCrate, deps map[string]int) {

	ctx.VisitDirectDeps(module, func(child android.Module) {
		// Skip intra-module dependencies (i.e., generated-source library depending on the source variant).
		if module.Name() == child.Name() {
			return
		}
		// Skip unsupported modules.
		rChild, ok := isModuleSupported(ctx, child)
		if !ok {
			return
		}
		// For unknown dependency, add it first.
		var childId int
		cInfo, known := singleton.knownCrates[rChild.Name()]
		if !known {
			childId, ok = singleton.addCrate(ctx, rChild, make(map[string]int))
			if !ok {
				return
			}
		} else {
			childId = cInfo.Idx
		}
		// Is this dependency known already?
		if _, ok = deps[child.Name()]; ok {
			return
		}
		crate.Deps = append(crate.Deps, rustProjectDep{Crate: childId, Name: rChild.CrateName()})
		deps[child.Name()] = childId
	})
}

// isModuleSupported returns the RustModule if the module
// should be considered for inclusion in rust-project.json.
func isModuleSupported(ctx android.SingletonContext, module android.Module) (*Module, bool) {
	rModule, ok := module.(*Module)
	if !ok {
		return nil, false
	}
	if !rModule.Enabled() {
		return nil, false
	}
	return rModule, true
}

// addCrate adds a crate to singleton.project.Crates ensuring that required
// dependencies are also added. It returns the index of the new crate in
// singleton.project.Crates
func (singleton *projectGeneratorSingleton) addCrate(ctx android.SingletonContext, rModule *Module, deps map[string]int) (int, bool) {
	rootModule, err := rModule.compiler.checkedCrateRootPath()
	if err != nil {
		return 0, false
	}

	_, procMacro := rModule.compiler.(*procMacroDecorator)

	crate := rustProjectCrate{
		DisplayName: rModule.Name(),
		RootModule:  rootModule.String(),
		Edition:     rModule.compiler.edition(),
		Deps:        make([]rustProjectDep, 0),
		Cfg:         make([]string, 0),
		Env:         make(map[string]string),
		ProcMacro:   procMacro,
	}

	if rModule.compiler.cargoOutDir().Valid() {
		crate.Env["OUT_DIR"] = rModule.compiler.cargoOutDir().String()
	}

	for _, feature := range rModule.compiler.features() {
		crate.Cfg = append(crate.Cfg, "feature=\""+feature+"\"")
	}

	singleton.mergeDependencies(ctx, rModule, &crate, deps)

	var idx int
	if cInfo, ok := singleton.knownCrates[rModule.Name()]; ok {
		idx = cInfo.Idx
		singleton.project.Crates[idx] = crate
	} else {
		idx = len(singleton.project.Crates)
		singleton.project.Crates = append(singleton.project.Crates, crate)
	}
	singleton.knownCrates[rModule.Name()] = crateInfo{Idx: idx, Deps: deps, Device: rModule.Device()}
	return idx, true
}

// appendCrateAndDependencies creates a rustProjectCrate for the module argument and appends it to singleton.project.
// It visits the dependencies of the module depth-first so the dependency ID can be added to the current module. If the
// current module is already in singleton.knownCrates, its dependencies are merged.
func (singleton *projectGeneratorSingleton) appendCrateAndDependencies(ctx android.SingletonContext, module android.Module) {
	rModule, ok := isModuleSupported(ctx, module)
	if !ok {
		return
	}
	// If we have seen this crate already; merge any new dependencies.
	if cInfo, ok := singleton.knownCrates[module.Name()]; ok {
		// If we have a new device variant, override the old one
		if !cInfo.Device && rModule.Device() {
			singleton.addCrate(ctx, rModule, cInfo.Deps)
			return
		}
		crate := singleton.project.Crates[cInfo.Idx]
		singleton.mergeDependencies(ctx, rModule, &crate, cInfo.Deps)
		singleton.project.Crates[cInfo.Idx] = crate
		return
	}
	singleton.addCrate(ctx, rModule, make(map[string]int))
}

func (singleton *projectGeneratorSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	if !ctx.Config().IsEnvTrue(envVariableCollectRustDeps) {
		return
	}

	singleton.knownCrates = make(map[string]crateInfo)
	ctx.VisitAllModules(func(module android.Module) {
		singleton.appendCrateAndDependencies(ctx, module)
	})

	path := android.PathForOutput(ctx, rustProjectJsonFileName)
	err := createJsonFile(singleton.project, path)
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
