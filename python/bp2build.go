// Copyright 2023 Google Inc. All rights reserved.
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

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/bazel"
)

type bazelPythonLibraryAttributes struct {
	Srcs         bazel.LabelListAttribute
	Deps         bazel.LabelListAttribute
	Imports      bazel.StringListAttribute
	Srcs_version *string
}

type bazelPythonProtoLibraryAttributes struct {
	Deps bazel.LabelListAttribute
}

type baseAttributes struct {
	// TODO(b/200311466): Probably not translate b/c Bazel has no good equiv
	//Pkg_path    bazel.StringAttribute
	// TODO: Related to Pkg_bath and similarLy gated
	//Is_internal bazel.BoolAttribute
	// Combines Srcs and Exclude_srcs
	Srcs bazel.LabelListAttribute
	Deps bazel.LabelListAttribute
	// Combines Data and Java_data (invariant)
	Data    bazel.LabelListAttribute
	Imports bazel.StringListAttribute
}

func (m *PythonLibraryModule) makeArchVariantBaseAttributes(ctx android.TopDownMutatorContext) baseAttributes {
	var attrs baseAttributes
	archVariantBaseProps := m.GetArchVariantProperties(ctx, &BaseProperties{})
	for axis, configToProps := range archVariantBaseProps {
		for config, props := range configToProps {
			if baseProps, ok := props.(*BaseProperties); ok {
				attrs.Srcs.SetSelectValue(axis, config,
					android.BazelLabelForModuleSrcExcludes(ctx, baseProps.Srcs, baseProps.Exclude_srcs))
				attrs.Deps.SetSelectValue(axis, config,
					android.BazelLabelForModuleDeps(ctx, baseProps.Libs))
				data := android.BazelLabelForModuleSrc(ctx, baseProps.Data)
				data.Append(android.BazelLabelForModuleSrc(ctx, baseProps.Java_data))
				attrs.Data.SetSelectValue(axis, config, data)
			}
		}
	}

	partitionedSrcs := bazel.PartitionLabelListAttribute(ctx, &attrs.Srcs, bazel.LabelPartitions{
		"proto": android.ProtoSrcLabelPartition,
		"py":    bazel.LabelPartition{Keep_remainder: true},
	})
	attrs.Srcs = partitionedSrcs["py"]

	if !partitionedSrcs["proto"].IsEmpty() {
		protoInfo, _ := android.Bp2buildProtoProperties(ctx, &m.ModuleBase, partitionedSrcs["proto"])
		protoLabel := bazel.Label{Label: ":" + protoInfo.Name}

		pyProtoLibraryName := m.Name() + "_py_proto"
		ctx.CreateBazelTargetModule(bazel.BazelTargetModuleProperties{
			Rule_class:        "py_proto_library",
			Bzl_load_location: "//build/bazel/rules/python:py_proto.bzl",
		}, android.CommonAttributes{
			Name: pyProtoLibraryName,
		}, &bazelPythonProtoLibraryAttributes{
			Deps: bazel.MakeSingleLabelListAttribute(protoLabel),
		})

		attrs.Deps.Add(bazel.MakeLabelAttribute(":" + pyProtoLibraryName))
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
		}

		if !strings.HasSuffix(ctx.ModuleDir(), "/"+pkg_path) && ctx.ModuleDir() != pkg_path {
			ctx.ModuleErrorf("Currently, bp2build only supports pkg_paths that are the same as the folders the Android.bp file is in. pkg_path: %s, module directory: %s", pkg_path, ctx.ModuleDir())
		}
		numFolders := strings.Count(pkg_path, "/") + 1
		dots := make([]string, numFolders)
		for i := 0; i < numFolders; i++ {
			dots[i] = ".."
		}
		imports = strings.Join(dots, "/")
	}
	attrs.Imports = bazel.MakeStringListAttribute([]string{imports})

	return attrs
}

func pythonLibBp2Build(ctx android.TopDownMutatorContext, m *PythonLibraryModule) {
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

	baseAttrs := m.makeArchVariantBaseAttributes(ctx)

	attrs := &bazelPythonLibraryAttributes{
		Srcs:         baseAttrs.Srcs,
		Deps:         baseAttrs.Deps,
		Srcs_version: python_version,
		Imports:      baseAttrs.Imports,
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

type bazelPythonBinaryAttributes struct {
	Main           *bazel.Label
	Srcs           bazel.LabelListAttribute
	Deps           bazel.LabelListAttribute
	Python_version *string
	Imports        bazel.StringListAttribute
}

func pythonBinaryBp2Build(ctx android.TopDownMutatorContext, m *PythonBinaryModule) {
	// TODO(b/182306917): this doesn't fully handle all nested props versioned
	// by the python version, which would have been handled by the version split
	// mutator. This is sufficient for very simple python_binary_host modules
	// under Bionic.
	py3Enabled := proptools.BoolDefault(m.properties.Version.Py3.Enabled, false)
	py2Enabled := proptools.BoolDefault(m.properties.Version.Py2.Enabled, false)
	var python_version *string
	if py3Enabled && py2Enabled {
		panic(fmt.Errorf(
			"error for '%s' module: bp2build's python_binary_host converter does not support "+
				"converting a module that is enabled for both Python 2 and 3 at the same time.", m.Name()))
	} else if py2Enabled {
		python_version = &pyVersion2
	} else {
		// do nothing, since python_version defaults to PY3.
	}

	baseAttrs := m.makeArchVariantBaseAttributes(ctx)
	attrs := &bazelPythonBinaryAttributes{
		Main:           nil,
		Srcs:           baseAttrs.Srcs,
		Deps:           baseAttrs.Deps,
		Python_version: python_version,
		Imports:        baseAttrs.Imports,
	}

	for _, propIntf := range m.GetProperties() {
		if props, ok := propIntf.(*BinaryProperties); ok {
			// main is optional.
			if props.Main != nil {
				main := android.BazelLabelForModuleSrcSingle(ctx, *props.Main)
				attrs.Main = &main
				break
			}
		}
	}

	props := bazel.BazelTargetModuleProperties{
		// Use the native py_binary rule.
		Rule_class: "py_binary",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{
		Name: m.Name(),
		Data: baseAttrs.Data,
	}, attrs)
}

func (p *PythonLibraryModule) ConvertWithBp2build(ctx android.TopDownMutatorContext) {
	pythonLibBp2Build(ctx, p)
}

func (p *PythonBinaryModule) ConvertWithBp2build(ctx android.TopDownMutatorContext) {
	pythonBinaryBp2Build(ctx, p)
}

func (p *PythonTestModule) ConvertWithBp2build(_ android.TopDownMutatorContext) {
	// Tests are currently unsupported
}
