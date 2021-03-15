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
	Cfgs        []string          `json:"cfgs"`
	Env         map[string]string `json:"env"`
}

type rustProjectJson struct {
	Roots  []string           `json:"roots"`
	Crates []rustProjectCrate `json:"crates"`
}

// crateInfo is used during the processing to keep track of the known crates.
type crateInfo struct {
	Idx  int            // Index of the crate in rustProjectJson.Crates slice.
	Deps map[string]int // The keys are the module names and not the crate names.
}

type projectGeneratorSingleton struct {
	project     rustProjectJson
	knownCrates map[string]crateInfo // Keys are module names.
}

func rustProjectGeneratorSingleton() android.Singleton {
	return &projectGeneratorSingleton{}
}

func init() {
	android.RegisterSingletonType("rust_project_generator", rustProjectGeneratorSingleton)
}

// sourceProviderVariantSource returns the path to the source file if this
// module variant should be used as a priority.
//
// SourceProvider modules may have multiple variants considered as source
// (e.g., x86_64 and armv8). For a module available on device, use the source
// generated for the target. For a host-only module, use the source generated
// for the host.
func sourceProviderVariantSource(ctx android.SingletonContext, rModule *Module) (string, bool) {
	rustLib, ok := rModule.compiler.(*libraryDecorator)
	if !ok {
		return "", false
	}
	if rustLib.source() {
		switch rModule.hod {
		case android.HostSupported, android.HostSupportedNoCross:
			if rModule.Target().String() == ctx.Config().BuildOSTarget.String() {
				src := rustLib.sourceProvider.Srcs()[0]
				return src.String(), true
			}
		default:
			if rModule.Target().String() == ctx.Config().AndroidFirstDeviceTarget.String() {
				src := rustLib.sourceProvider.Srcs()[0]
				return src.String(), true
			}
		}
	}
	return "", false
}

// sourceProviderSource finds the main source file of a source-provider crate.
func sourceProviderSource(ctx android.SingletonContext, rModule *Module) (string, bool) {
	rustLib, ok := rModule.compiler.(*libraryDecorator)
	if !ok {
		return "", false
	}
	if rustLib.source() {
		// This is a source-variant, check if we are the right variant
		// depending on the module configuration.
		if src, ok := sourceProviderVariantSource(ctx, rModule); ok {
			return src, true
		}
	}
	foundSource := false
	sourceSrc := ""
	// Find the variant with the source and return its.
	ctx.VisitAllModuleVariants(rModule, func(variant android.Module) {
		if foundSource {
			return
		}
		// All variants of a source provider library are libraries.
		rVariant, _ := variant.(*Module)
		variantLib, _ := rVariant.compiler.(*libraryDecorator)
		if variantLib.source() {
			sourceSrc, ok = sourceProviderVariantSource(ctx, rVariant)
			if ok {
				foundSource = true
			}
		}
	})
	if !foundSource {
		ctx.Errorf("No valid source for source provider found: %v\n", rModule)
	}
	return sourceSrc, foundSource
}

// crateSource finds the main source file (.rs) for a crate.
func crateSource(ctx android.SingletonContext, rModule *Module, comp *baseCompiler) (string, bool) {
	// Basic libraries, executables and tests.
	srcs := comp.Properties.Srcs
	if len(srcs) != 0 {
		return path.Join(ctx.ModuleDir(rModule), srcs[0]), true
	}
	// SourceProvider libraries.
	if rModule.sourceProvider != nil {
		return sourceProviderSource(ctx, rModule)
	}
	return "", false
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
		rChild, compChild, ok := isModuleSupported(ctx, child)
		if !ok {
			return
		}
		// For unknown dependency, add it first.
		var childId int
		cInfo, known := singleton.knownCrates[rChild.Name()]
		if !known {
			childId, ok = singleton.addCrate(ctx, rChild, compChild)
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

// isModuleSupported returns the RustModule and baseCompiler if the module
// should be considered for inclusion in rust-project.json.
func isModuleSupported(ctx android.SingletonContext, module android.Module) (*Module, *baseCompiler, bool) {
	rModule, ok := module.(*Module)
	if !ok {
		return nil, nil, false
	}
	if rModule.compiler == nil {
		return nil, nil, false
	}
	var comp *baseCompiler
	switch c := rModule.compiler.(type) {
	case *libraryDecorator:
		comp = c.baseCompiler
	case *binaryDecorator:
		comp = c.baseCompiler
	case *testDecorator:
		comp = c.binaryDecorator.baseCompiler
	default:
		return nil, nil, false
	}
	return rModule, comp, true
}

// addCrate adds a crate to singleton.project.Crates ensuring that required
// dependencies are also added. It returns the index of the new crate in
// singleton.project.Crates
func (singleton *projectGeneratorSingleton) addCrate(ctx android.SingletonContext, rModule *Module, comp *baseCompiler) (int, bool) {
	rootModule, ok := crateSource(ctx, rModule, comp)
	if !ok {
		ctx.Errorf("Unable to find source for valid module: %v", rModule)
		return 0, false
	}

	crate := rustProjectCrate{
		DisplayName: rModule.Name(),
		RootModule:  rootModule,
		Edition:     comp.edition(),
		Deps:        make([]rustProjectDep, 0),
		Cfgs:        make([]string, 0),
		Env:         make(map[string]string),
	}

	if comp.CargoOutDir().Valid() {
		crate.Env["OUT_DIR"] = comp.CargoOutDir().String()
	}

	deps := make(map[string]int)
	singleton.mergeDependencies(ctx, rModule, &crate, deps)

	idx := len(singleton.project.Crates)
	singleton.knownCrates[rModule.Name()] = crateInfo{Idx: idx, Deps: deps}
	singleton.project.Crates = append(singleton.project.Crates, crate)
	// rust-analyzer requires that all crates belong to at least one root:
	// https://github.com/rust-analyzer/rust-analyzer/issues/4735.
	singleton.project.Roots = append(singleton.project.Roots, path.Dir(crate.RootModule))
	return idx, true
}

// appendCrateAndDependencies creates a rustProjectCrate for the module argument and appends it to singleton.project.
// It visits the dependencies of the module depth-first so the dependency ID can be added to the current module. If the
// current module is already in singleton.knownCrates, its dependencies are merged.
func (singleton *projectGeneratorSingleton) appendCrateAndDependencies(ctx android.SingletonContext, module android.Module) {
	rModule, comp, ok := isModuleSupported(ctx, module)
	if !ok {
		return
	}
	// If we have seen this crate already; merge any new dependencies.
	if cInfo, ok := singleton.knownCrates[module.Name()]; ok {
		crate := singleton.project.Crates[cInfo.Idx]
		singleton.mergeDependencies(ctx, rModule, &crate, cInfo.Deps)
		singleton.project.Crates[cInfo.Idx] = crate
		return
	}
	singleton.addCrate(ctx, rModule, comp)
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
