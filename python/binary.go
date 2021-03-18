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

	"android/soong/android"
	"android/soong/bazel"

	"github.com/google/blueprint/proptools"
)

func init() {
	registerPythonBinaryComponents(android.InitRegistrationContext)
	android.RegisterBp2BuildMutator("python_binary_host", PythonBinaryBp2Build)
}

func registerPythonBinaryComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("python_binary_host", PythonBinaryHostFactory)
}

type bazelPythonBinaryAttributes struct {
	Main           string
	Srcs           bazel.LabelList
	Data           bazel.LabelList
	Python_version string
}

type bazelPythonBinary struct {
	android.BazelTargetModuleBase
	bazelPythonBinaryAttributes
}

func BazelPythonBinaryFactory() android.Module {
	module := &bazelPythonBinary{}
	module.AddProperties(&module.bazelPythonBinaryAttributes)
	android.InitBazelTargetModule(module)
	return module
}

func (m *bazelPythonBinary) Name() string {
	return m.BaseModuleName()
}

func (m *bazelPythonBinary) GenerateAndroidBuildActions(ctx android.ModuleContext) {}

func PythonBinaryBp2Build(ctx android.TopDownMutatorContext) {
	m, ok := ctx.Module().(*Module)
	if !ok || !m.ConvertWithBp2build() {
		return
	}

	// a Module can be something other than a python_binary_host
	if ctx.ModuleType() != "python_binary_host" {
		return
	}

	var main string
	for _, propIntf := range m.GetProperties() {
		if props, ok := propIntf.(*BinaryProperties); ok {
			// main is optional.
			if props.Main != nil {
				main = *props.Main
				break
			}
		}
	}
	// TODO(b/182306917): this doesn't fully handle all nested props versioned
	// by the python version, which would have been handled by the version split
	// mutator. This is sufficient for very simple python_binary_host modules
	// under Bionic.
	py3Enabled := proptools.BoolDefault(m.properties.Version.Py3.Enabled, false)
	py2Enabled := proptools.BoolDefault(m.properties.Version.Py2.Enabled, false)
	var python_version string
	if py3Enabled && py2Enabled {
		panic(fmt.Errorf(
			"error for '%s' module: bp2build's python_binary_host converter does not support "+
				"converting a module that is enabled for both Python 2 and 3 at the same time.", m.Name()))
	} else if py2Enabled {
		python_version = "PY2"
	} else {
		// do nothing, since python_version defaults to PY3.
	}

	attrs := &bazelPythonBinaryAttributes{
		Main:           main,
		Srcs:           android.BazelLabelForModuleSrcExcludes(ctx, m.properties.Srcs, m.properties.Exclude_srcs),
		Data:           android.BazelLabelForModuleSrc(ctx, m.properties.Data),
		Python_version: python_version,
	}

	props := bazel.BazelTargetModuleProperties{
		// Use the native py_binary rule.
		Rule_class: "py_binary",
	}

	ctx.CreateBazelTargetModule(BazelPythonBinaryFactory, m.Name(), props, attrs)
}

type BinaryProperties struct {
	// the name of the source file that is the main entry point of the program.
	// this file must also be listed in srcs.
	// If left unspecified, module name is used instead.
	// If name doesn’t match any filename in srcs, main must be specified.
	Main *string `android:"arch_variant"`

	// set the name of the output binary.
	Stem *string `android:"arch_variant"`

	// append to the name of the output binary.
	Suffix *string `android:"arch_variant"`

	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// whether to use `main` when starting the executable. The default is true, when set to
	// false it will act much like the normal `python` executable, but with the sources and
	// libraries automatically included in the PYTHONPATH.
	Autorun *bool `android:"arch_variant"`

	// Flag to indicate whether or not to create test config automatically. If AndroidTest.xml
	// doesn't exist next to the Android.bp, this attribute doesn't need to be set to true
	// explicitly.
	Auto_gen_config *bool
}

type binaryDecorator struct {
	binaryProperties BinaryProperties

	*pythonInstaller
}

type IntermPathProvider interface {
	IntermPathForModuleOut() android.OptionalPath
}

var (
	StubTemplateHost = "build/soong/python/scripts/stub_template_host.txt"
)

func NewBinary(hod android.HostOrDeviceSupported) (*Module, *binaryDecorator) {
	module := newModule(hod, android.MultilibFirst)
	decorator := &binaryDecorator{pythonInstaller: NewPythonInstaller("bin", "")}

	module.bootstrapper = decorator
	module.installer = decorator

	return module, decorator
}

func PythonBinaryHostFactory() android.Module {
	module, _ := NewBinary(android.HostSupported)

	android.InitBazelModule(module)

	return module.init()
}

func (binary *binaryDecorator) autorun() bool {
	return BoolDefault(binary.binaryProperties.Autorun, true)
}

func (binary *binaryDecorator) bootstrapperProps() []interface{} {
	return []interface{}{&binary.binaryProperties}
}

func (binary *binaryDecorator) bootstrap(ctx android.ModuleContext, actualVersion string,
	embeddedLauncher bool, srcsPathMappings []pathMapping, srcsZip android.Path,
	depsSrcsZips android.Paths) android.OptionalPath {

	main := ""
	if binary.autorun() {
		main = binary.getPyMainFile(ctx, srcsPathMappings)
	}

	var launcherPath android.OptionalPath
	if embeddedLauncher {
		ctx.VisitDirectDepsWithTag(launcherTag, func(m android.Module) {
			if provider, ok := m.(IntermPathProvider); ok {
				if launcherPath.Valid() {
					panic(fmt.Errorf("launcher path was found before: %q",
						launcherPath))
				}
				launcherPath = provider.IntermPathForModuleOut()
			}
		})
	}

	binFile := registerBuildActionForParFile(ctx, embeddedLauncher, launcherPath,
		binary.getHostInterpreterName(ctx, actualVersion),
		main, binary.getStem(ctx), append(android.Paths{srcsZip}, depsSrcsZips...))

	return android.OptionalPathForPath(binFile)
}

// get host interpreter name.
func (binary *binaryDecorator) getHostInterpreterName(ctx android.ModuleContext,
	actualVersion string) string {
	var interp string
	switch actualVersion {
	case pyVersion2:
		interp = "python2.7"
	case pyVersion3:
		interp = "python3"
	default:
		panic(fmt.Errorf("unknown Python actualVersion: %q for module: %q.",
			actualVersion, ctx.ModuleName()))
	}

	return interp
}

// find main program path within runfiles tree.
func (binary *binaryDecorator) getPyMainFile(ctx android.ModuleContext,
	srcsPathMappings []pathMapping) string {
	var main string
	if String(binary.binaryProperties.Main) == "" {
		main = ctx.ModuleName() + pyExt
	} else {
		main = String(binary.binaryProperties.Main)
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
	if String(binary.binaryProperties.Stem) != "" {
		stem = String(binary.binaryProperties.Stem)
	}

	return stem + String(binary.binaryProperties.Suffix)
}
