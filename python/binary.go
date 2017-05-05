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
	"io"
	"path/filepath"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

func init() {
	android.RegisterModuleType("python_binary_host", PythonBinaryHostFactory)
}

type PythonBinaryProperties struct {
	// the name of the source file that is the main entry point of the program.
	// this file must also be listed in srcs.
	// If left unspecified, module name is used instead.
	// If name doesn’t match any filename in srcs, main must be specified.
	Main string

	// set the name of the output binary.
	Stem string

	// append to the name of the output binary.
	Suffix string
}

type PythonBinary struct {
	pythonBaseModule

	binaryProperties PythonBinaryProperties

	// soong_zip arguments from all its dependencies.
	depsParSpecs []parSpec

	// Python runfiles paths from all its dependencies.
	depsPyRunfiles []string

	// the installation path for Python binary.
	installPath android.OutputPath
}

var _ PythonSubModule = (*PythonBinary)(nil)

var (
	stubTemplateHost = "build/soong/python/scripts/stub_template_host.txt"
)

func PythonBinaryHostFactory() (blueprint.Module, []interface{}) {
	module := &PythonBinary{}

	return InitPythonBaseModule(&module.pythonBaseModule, module, android.HostSupportedNoCross,
		&module.binaryProperties)
}

func (p *PythonBinary) GeneratePythonBuildActions(ctx android.ModuleContext) {
	p.pythonBaseModule.GeneratePythonBuildActions(ctx)

	// no Python source file for compiling par file.
	if len(p.pythonBaseModule.srcsPathMappings) == 0 && len(p.depsPyRunfiles) == 0 {
		return
	}

	// the runfiles packages needs to be populated with "__init__.py".
	newPyPkgs := []string{}
	// the set to de-duplicate the new Python packages above.
	newPyPkgSet := make(map[string]bool)
	// the runfiles dirs have been treated as packages.
	existingPyPkgSet := make(map[string]bool)

	wholePyRunfiles := []string{}
	for _, path := range p.pythonBaseModule.srcsPathMappings {
		wholePyRunfiles = append(wholePyRunfiles, path.dest)
	}
	wholePyRunfiles = append(wholePyRunfiles, p.depsPyRunfiles...)

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

	main := p.getPyMainFile(ctx)
	if main == "" {
		return
	}
	interp := p.getInterpreter(ctx)
	if interp == "" {
		return
	}

	// we need remove "runfiles/" suffix since stub script starts
	// searching for main file in each sub-dir of "runfiles" directory tree.
	binFile := registerBuildActionForParFile(ctx, p.getInterpreter(ctx),
		strings.TrimPrefix(main, runFiles+"/"), p.getStem(ctx),
		newPyPkgs, append(p.depsParSpecs, p.pythonBaseModule.parSpec))

	// install par file.
	p.installPath = ctx.InstallFile(
		android.PathForModuleInstall(ctx, "bin"), binFile)
}

// get interpreter path.
func (p *PythonBinary) getInterpreter(ctx android.ModuleContext) string {
	var interp string
	switch p.pythonBaseModule.properties.ActualVersion {
	case pyVersion2:
		interp = "python2"
	case pyVersion3:
		interp = "python3"
	default:
		panic(fmt.Errorf("unknown Python actualVersion: %q for module: %q.",
			p.properties.ActualVersion, ctx.ModuleName()))
	}

	return interp
}

// find main program path within runfiles tree.
func (p *PythonBinary) getPyMainFile(ctx android.ModuleContext) string {
	var main string
	if p.binaryProperties.Main == "" {
		main = p.BaseModuleName() + pyExt
	} else {
		main = p.binaryProperties.Main
	}

	for _, path := range p.pythonBaseModule.srcsPathMappings {
		if main == path.src.Rel() {
			return path.dest
		}
	}
	ctx.PropertyErrorf("main", "%q is not listed in srcs.", main)

	return ""
}

func (p *PythonBinary) getStem(ctx android.ModuleContext) string {
	stem := ctx.ModuleName()
	if p.binaryProperties.Stem != "" {
		stem = p.binaryProperties.Stem
	}

	return stem + p.binaryProperties.Suffix
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

func (p *PythonBinary) GeneratePythonAndroidMk() (ret android.AndroidMkData, err error) {
	// Soong installation is only supported for host modules. Have Make
	// installation trigger Soong installation.
	if p.pythonBaseModule.Target().Os.Class == android.Host {
		ret.OutputFile = android.OptionalPathForPath(p.installPath)
	}
	ret.Class = "EXECUTABLES"

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) error {
		path := p.installPath.RelPathString()
		dir, file := filepath.Split(path)
		stem := strings.TrimSuffix(file, filepath.Ext(file))

		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+filepath.Ext(file))
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := $(OUT_DIR)/"+filepath.Clean(dir))
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)

		return nil
	})

	return

}
