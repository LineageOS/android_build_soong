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
	"fmt"

	"android/soong/android"
	"android/soong/bazel"
	"github.com/google/blueprint/proptools"
)

func init() {
	registerPythonLibraryComponents(android.InitRegistrationContext)
	android.RegisterBp2BuildMutator("python_library", PythonLibraryBp2Build)
}

func registerPythonLibraryComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("python_library_host", PythonLibraryHostFactory)
	ctx.RegisterModuleType("python_library", PythonLibraryFactory)
}

func PythonLibraryHostFactory() android.Module {
	module := newModule(android.HostSupported, android.MultilibFirst)

	return module.init()
}

type bazelPythonLibraryAttributes struct {
	Srcs         bazel.LabelListAttribute
	Data         bazel.LabelListAttribute
	Srcs_version string
}

func PythonLibraryBp2Build(ctx android.TopDownMutatorContext) {
	m, ok := ctx.Module().(*Module)
	if !ok || !m.ConvertWithBp2build(ctx) {
		return
	}

	// a Module can be something other than a python_library
	if ctx.ModuleType() != "python_library" {
		return
	}

	// TODO(b/182306917): this doesn't fully handle all nested props versioned
	// by the python version, which would have been handled by the version split
	// mutator. This is sufficient for very simple python_library modules under
	// Bionic.
	py3Enabled := proptools.BoolDefault(m.properties.Version.Py3.Enabled, true)
	py2Enabled := proptools.BoolDefault(m.properties.Version.Py2.Enabled, false)
	var python_version string
	if py2Enabled && !py3Enabled {
		python_version = "PY2"
	} else if !py2Enabled && py3Enabled {
		python_version = "PY3"
	} else if !py2Enabled && !py3Enabled {
		panic(fmt.Errorf(
			"error for '%s' module: bp2build's python_library converter doesn't understand having "+
				"neither py2 nor py3 enabled", m.Name()))
	} else {
		// do nothing, since python_version defaults to PY2ANDPY3
	}

	srcs := android.BazelLabelForModuleSrcExcludes(ctx, m.properties.Srcs, m.properties.Exclude_srcs)
	data := android.BazelLabelForModuleSrc(ctx, m.properties.Data)

	attrs := &bazelPythonLibraryAttributes{
		Srcs:         bazel.MakeLabelListAttribute(srcs),
		Data:         bazel.MakeLabelListAttribute(data),
		Srcs_version: python_version,
	}

	props := bazel.BazelTargetModuleProperties{
		// Use the native py_library rule.
		Rule_class: "py_library",
	}

	ctx.CreateBazelTargetModule(m.Name(), props, attrs)
}

func PythonLibraryFactory() android.Module {
	module := newModule(android.HostAndDeviceSupported, android.MultilibBoth)

	android.InitBazelModule(module)

	return module.init()
}
