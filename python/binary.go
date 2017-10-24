// Copyright 2017 Google Inc. All rights reserved.
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

package python

// This file contains the module types for building Python binary.

import (
	"fmt"
	"path/filepath"
	"strings"

	"android/soong/android"
)

func init() {
	android.RegisterModuleType("python_binary_host", PythonBinaryHostFactory)
}

type BinaryProperties struct {
	// the name of the source file that is the main entry point of the program.
	// this file must also be listed in srcs.
	// If left unspecified, module name is used instead.
	// If name doesn’t match any filename in srcs, main must be specified.
	Main string `android:"arch_variant"`

	// set the name of the output binary.
	Stem string `android:"arch_variant"`

	// append to the name of the output binary.
	Suffix string `android:"arch_variant"`
}

type binaryDecorator struct {
	binaryProperties BinaryProperties

	baseInstaller *pythonInstaller
}

type IntermPathProvider interface {
	IntermPathForModuleOut() android.OptionalPath
}

func (binary *binaryDecorator) install(ctx android.ModuleContext, file android.Path) {
	binary.baseInstaller.install(ctx, file)
}

var (
	stubTemplateHost = "build/soong/python/scripts/stub_template_host.txt"
)

func NewBinary(hod android.HostOrDeviceSupported) (*Module, *binaryDecorator) {
	module := newModule(hod, android.MultilibFirst)
	decorator := &binaryDecorator{baseInstaller: NewPythonInstaller("bin")}

	module.bootstrapper = decorator
	module.installer = decorator

	return module, decorator
}

func PythonBinaryHostFactory() android.Module {
	module, _ := NewBinary(android.HostSupportedNoCross)

	return module.Init()
}

func (binary *binaryDecorator) bootstrapperProps() []interface{} {
	return []interface{}{&binary.binaryProperties}
}

func (binary *binaryDecorator) bootstrap(ctx android.ModuleContext, actual_version string,
	embedded_launcher bool, srcsPathMappings []pathMapping, parSpec parSpec,
	depsPyRunfiles []string, depsParSpecs []parSpec) android.OptionalPath {
	// no Python source file for compiling .par file.
	if len(srcsPathMappings) == 0 {
		return android.OptionalPath{}
	}

	// the runfiles packages needs to be populated with "__init__.py".
	newPyPkgs := []string{}
	// the set to de-duplicate the new Python packages above.
	newPyPkgSet := make(map[string]bool)
	// the runfiles dirs have been treated as packages.
	existingPyPkgSet := make(map[string]bool)

	wholePyRunfiles := []string{}
	for _, path := range srcsPathMappings {
		wholePyRunfiles = append(wholePyRunfiles, path.dest)
	}
	wholePyRunfiles = append(wholePyRunfiles, depsPyRunfiles...)

	// find all the runfiles dirs which have been treated as packages.
	for _, path := range wholePyRunfiles {
		if filepath.Base(path) != initFileName {
			continue
		}
		existingPyPkg := PathBeforeLastSlash(path)
		if _, found := existingPyPkgSet[existingPyPkg]; found {
			panic(fmt.Errorf("found init file path duplicates: %q for module: %q.",
				path, ctx.ModuleName()))
		} else {
			existingPyPkgSet[existingPyPkg] = true
		}
		parentPath := PathBeforeLastSlash(existingPyPkg)
		populateNewPyPkgs(parentPath, existingPyPkgSet, newPyPkgSet, &newPyPkgs)
	}

	// create new packages under runfiles tree.
	for _, path := range wholePyRunfiles {
		if filepath.Base(path) == initFileName {
			continue
		}
		parentPath := PathBeforeLastSlash(path)
		populateNewPyPkgs(parentPath, existingPyPkgSet, newPyPkgSet, &newPyPkgs)
	}

	main := binary.getPyMainFile(ctx, srcsPathMappings)
	if main == "" {
		return android.OptionalPath{}
	}

	var launcher_path android.Path
	if embedded_launcher {
		ctx.VisitDirectDeps(func(m android.Module) {
			if ctx.OtherModuleDependencyTag(m) != launcherTag {
				return
			}
			if provider, ok := m.(IntermPathProvider); ok {
				if launcher_path != nil {
					panic(fmt.Errorf("launcher path was found before: %q",
						launcher_path))
				}
				launcher_path = provider.IntermPathForModuleOut().Path()
			}
		})
	}

	binFile := registerBuildActionForParFile(ctx, embedded_launcher, launcher_path,
		binary.getHostInterpreterName(ctx, actual_version),
		main, binary.getStem(ctx), newPyPkgs, append(depsParSpecs, parSpec))

	return android.OptionalPathForPath(binFile)
}

// get host interpreter name.
func (binary *binaryDecorator) getHostInterpreterName(ctx android.ModuleContext,
	actual_version string) string {
	var interp string
	switch actual_version {
	case pyVersion2:
		interp = "python2.7"
	case pyVersion3:
		interp = "python3"
	default:
		panic(fmt.Errorf("unknown Python actualVersion: %q for module: %q.",
			actual_version, ctx.ModuleName()))
	}

	return interp
}

// find main program path within runfiles tree.
func (binary *binaryDecorator) getPyMainFile(ctx android.ModuleContext,
	srcsPathMappings []pathMapping) string {
	var main string
	if binary.binaryProperties.Main == "" {
		main = ctx.ModuleName() + pyExt
	} else {
		main = binary.binaryProperties.Main
	}

	for _, path := range srcsPathMappings {
		if main == path.src.Rel() {
			return path.dest
		}
	}
	ctx.PropertyErrorf("main", "%q is not listed in srcs.", main)

	return ""
}

func (binary *binaryDecorator) getStem(ctx android.ModuleContext) string {
	stem := ctx.ModuleName()
	if binary.binaryProperties.Stem != "" {
		stem = binary.binaryProperties.Stem
	}

	return stem + binary.binaryProperties.Suffix
}

// Sets the given directory and all its ancestor directories as Python packages.
func populateNewPyPkgs(pkgPath string, existingPyPkgSet,
	newPyPkgSet map[string]bool, newPyPkgs *[]string) {
	for pkgPath != "" {
		if _, found := existingPyPkgSet[pkgPath]; found {
			break
		}
		if _, found := newPyPkgSet[pkgPath]; !found {
			newPyPkgSet[pkgPath] = true
			*newPyPkgs = append(*newPyPkgs, pkgPath)
			// Gets its ancestor directory by trimming last slash.
			pkgPath = PathBeforeLastSlash(pkgPath)
		} else {
			break
		}
	}
}

// filepath.Dir("abc") -> "." and filepath.Dir("/abc") -> "/". However,
// the PathBeforeLastSlash() will return "" for both cases above.
func PathBeforeLastSlash(path string) string {
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		return path[:idx]
	}
	return ""
}
