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

// This file contains the module types for building Python library.

import (
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/bazel"

	"github.com/google/blueprint/proptools"
)

func init() {
	registerPythonLibraryComponents(android.InitRegistrationContext)
}

func registerPythonLibraryComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("python_library_host", PythonLibraryHostFactory)
	ctx.RegisterModuleType("python_library", PythonLibraryFactory)
}

func PythonLibraryHostFactory() android.Module {
	module := newModule(android.HostSupported, android.MultilibFirst)

	android.InitBazelModule(module)

	return module.init()
}

type bazelPythonLibraryAttributes struct {
	Srcs         bazel.LabelListAttribute
	Deps         bazel.LabelListAttribute
	Imports      bazel.StringListAttribute
	Srcs_version *string
}

func pythonLibBp2Build(ctx android.TopDownMutatorContext, m *Module) {
	// TODO(b/182306917): this doesn't fully handle all nested props versioned
	// by the python version, which would have been handled by the version split
	// mutator. This is sufficient for very simple python_library modules under
	// Bionic.
	py3Enabled := proptools.BoolDefault(m.properties.Version.Py3.Enabled, true)
	py2Enabled := proptools.BoolDefault(m.properties.Version.Py2.Enabled, false)
	var python_version *string
	if py2Enabled && !py3Enabled {
		python_version = &pyVersion2
	} else if !py2Enabled && py3Enabled {
		python_version = &pyVersion3
	} else if !py2Enabled && !py3Enabled {
		ctx.ModuleErrorf("bp2build converter doesn't understand having neither py2 nor py3 enabled")
	} else {
		// do nothing, since python_version defaults to PY2ANDPY3
	}

	// Bazel normally requires `import path.from.top.of.tree` statements in
	// python code, but with soong you can directly import modules from libraries.
	// Add "imports" attributes to the bazel library so it matches soong's behavior.
	imports := "."
	if m.properties.Pkg_path != nil {
		// TODO(b/215119317) This is a hack to handle the fact that we don't convert
		// pkg_path properly right now. If the folder structure that contains this
		// Android.bp file matches pkg_path, we can set imports to an appropriate
		// number of ../..s to emulate moving the files under a pkg_path folder.
		pkg_path := filepath.Clean(*m.properties.Pkg_path)
		if strings.HasPrefix(pkg_path, "/") {
			ctx.ModuleErrorf("pkg_path cannot start with a /: %s", pkg_path)
			return
		}

		if !strings.HasSuffix(ctx.ModuleDir(), "/"+pkg_path) && ctx.ModuleDir() != pkg_path {
			ctx.ModuleErrorf("Currently, bp2build only supports pkg_paths that are the same as the folders the Android.bp file is in. pkg_path: %s, module directory: %s", pkg_path, ctx.ModuleDir())
			return
		}
		numFolders := strings.Count(pkg_path, "/") + 1
		dots := make([]string, numFolders)
		for i := 0; i < numFolders; i++ {
			dots[i] = ".."
		}
		imports = strings.Join(dots, "/")
	}

	baseAttrs := m.makeArchVariantBaseAttributes(ctx)
	attrs := &bazelPythonLibraryAttributes{
		Srcs:         baseAttrs.Srcs,
		Deps:         baseAttrs.Deps,
		Srcs_version: python_version,
		Imports:      bazel.MakeStringListAttribute([]string{imports}),
	}

	props := bazel.BazelTargetModuleProperties{
		// Use the native py_library rule.
		Rule_class: "py_library",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{
		Name: m.Name(),
		Data: baseAttrs.Data,
	}, attrs)
}

func PythonLibraryFactory() android.Module {
	module := newModule(android.HostAndDeviceSupported, android.MultilibBoth)

	android.InitBazelModule(module)

	return module.init()
}
