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

	// A list of proto_library targets that the proto_library in `deps` depends on
	// This list is overestimation.
	// Overestimation is necessary since Soong includes other protos via proto.include_dirs and not
	// a specific .proto file module explicitly.
	Transitive_deps bazel.LabelListAttribute
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

func (m *PythonLibraryModule) makeArchVariantBaseAttributes(ctx android.Bp2buildMutatorContext) baseAttributes {
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

		pyProtoLibraryName := m.Name() + "_py_proto"
		ctx.CreateBazelTargetModule(bazel.BazelTargetModuleProperties{
			Rule_class:        "py_proto_library",
			Bzl_load_location: "//build/bazel/rules/python:py_proto.bzl",
		}, android.CommonAttributes{
			Name: pyProtoLibraryName,
		}, &bazelPythonProtoLibraryAttributes{
			Deps:            bazel.MakeLabelListAttribute(protoInfo.Proto_libs),
			Transitive_deps: bazel.MakeLabelListAttribute(protoInfo.Transitive_proto_libs),
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

func (m *PythonLibraryModule) bp2buildPythonVersion(ctx android.Bp2buildMutatorContext) *string {
	py3Enabled := proptools.BoolDefault(m.properties.Version.Py3.Enabled, true)
	py2Enabled := proptools.BoolDefault(m.properties.Version.Py2.Enabled, false)
	if py2Enabled && !py3Enabled {
		return &pyVersion2
	} else if !py2Enabled && py3Enabled {
		return &pyVersion3
	} else if !py2Enabled && !py3Enabled {
		ctx.ModuleErrorf("bp2build converter doesn't understand having neither py2 nor py3 enabled")
		return &pyVersion3
	} else {
		return &pyVersion2And3
	}
}

type bazelPythonBinaryAttributes struct {
	Main           *bazel.Label
	Srcs           bazel.LabelListAttribute
	Deps           bazel.LabelListAttribute
	Python_version *string
	Imports        bazel.StringListAttribute
}

func (p *PythonLibraryModule) ConvertWithBp2build(ctx android.Bp2buildMutatorContext) {
	// TODO(b/182306917): this doesn't fully handle all nested props versioned
	// by the python version, which would have been handled by the version split
	// mutator. This is sufficient for very simple python_library modules under
	// Bionic.
	baseAttrs := p.makeArchVariantBaseAttributes(ctx)
	pyVersion := p.bp2buildPythonVersion(ctx)
	if *pyVersion == pyVersion2And3 {
		// Libraries default to python 2 and 3
		pyVersion = nil
	}

	attrs := &bazelPythonLibraryAttributes{
		Srcs:         baseAttrs.Srcs,
		Deps:         baseAttrs.Deps,
		Srcs_version: pyVersion,
		Imports:      baseAttrs.Imports,
	}

	props := bazel.BazelTargetModuleProperties{
		// Use the native py_library rule.
		Rule_class: "py_library",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{
		Name: p.Name(),
		Data: baseAttrs.Data,
	}, attrs)
}

func (p *PythonBinaryModule) bp2buildBinaryProperties(ctx android.Bp2buildMutatorContext) (*bazelPythonBinaryAttributes, bazel.LabelListAttribute) {
	// TODO(b/182306917): this doesn't fully handle all nested props versioned
	// by the python version, which would have been handled by the version split
	// mutator. This is sufficient for very simple python_binary_host modules
	// under Bionic.

	baseAttrs := p.makeArchVariantBaseAttributes(ctx)
	pyVersion := p.bp2buildPythonVersion(ctx)
	if *pyVersion == pyVersion3 {
		// Binaries default to python 3
		pyVersion = nil
	} else if *pyVersion == pyVersion2And3 {
		ctx.ModuleErrorf("error for '%s' module: bp2build's python_binary_host converter "+
			"does not support converting a module that is enabled for both Python 2 and 3 at the "+
			"same time.", p.Name())
	}

	attrs := &bazelPythonBinaryAttributes{
		Main:           nil,
		Srcs:           baseAttrs.Srcs,
		Deps:           baseAttrs.Deps,
		Python_version: pyVersion,
		Imports:        baseAttrs.Imports,
	}

	// main is optional.
	if p.binaryProperties.Main != nil {
		main := android.BazelLabelForModuleSrcSingle(ctx, *p.binaryProperties.Main)
		attrs.Main = &main
	}
	return attrs, baseAttrs.Data
}

func (p *PythonBinaryModule) ConvertWithBp2build(ctx android.Bp2buildMutatorContext) {
	attrs, data := p.bp2buildBinaryProperties(ctx)

	props := bazel.BazelTargetModuleProperties{
		// Use the native py_binary rule.
		Rule_class: "py_binary",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{
		Name: p.Name(),
		Data: data,
	}, attrs)
}

func (p *PythonTestModule) ConvertWithBp2build(ctx android.Bp2buildMutatorContext) {
	// Python tests are currently exactly the same as binaries, but with a different module type
	attrs, data := p.bp2buildBinaryProperties(ctx)

	props := bazel.BazelTargetModuleProperties{
		// Use the native py_binary rule.
		Rule_class:        "py_test",
		Bzl_load_location: "//build/bazel/rules/python:py_test.bzl",
	}

	ctx.CreateBazelTargetModule(props, android.CommonAttributes{
		Name: p.Name(),
		Data: data,
	}, attrs)
}
