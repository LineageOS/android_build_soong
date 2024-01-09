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
	registerPythonBinaryComponents(android.InitRegistrationContext)
}

func registerPythonBinaryComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("python_binary_host", PythonBinaryHostFactory)
}

type BinaryProperties struct {
	// the name of the source file that is the main entry point of the program.
	// this file must also be listed in srcs.
	// If left unspecified, module name is used instead.
	// If name doesn’t match any filename in srcs, main must be specified.
	Main *string

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

type PythonBinaryModule struct {
	PythonLibraryModule
	binaryProperties BinaryProperties

	// (.intermediate) module output path as installation source.
	installSource android.Path

	// Final installation path.
	installedDest android.Path

	androidMkSharedLibs []string

	// Aconfig files for all transitive deps.  Also exposed via TransitiveDeclarationsInfo
	mergedAconfigFiles map[string]android.Paths
}

var _ android.AndroidMkEntriesProvider = (*PythonBinaryModule)(nil)
var _ android.Module = (*PythonBinaryModule)(nil)

type IntermPathProvider interface {
	IntermPathForModuleOut() android.OptionalPath
}

func NewBinary(hod android.HostOrDeviceSupported) *PythonBinaryModule {
	return &PythonBinaryModule{
		PythonLibraryModule: *newModule(hod, android.MultilibFirst),
	}
}

func PythonBinaryHostFactory() android.Module {
	return NewBinary(android.HostSupported).init()
}

func (p *PythonBinaryModule) init() android.Module {
	p.AddProperties(&p.properties, &p.protoProperties)
	p.AddProperties(&p.binaryProperties)
	android.InitAndroidArchModule(p, p.hod, p.multilib)
	android.InitDefaultableModule(p)
	return p
}

func (p *PythonBinaryModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.PythonLibraryModule.GenerateAndroidBuildActions(ctx)
	p.buildBinary(ctx)
	p.installedDest = ctx.InstallFile(installDir(ctx, "bin", "", ""),
		p.installSource.Base(), p.installSource)
	android.CollectDependencyAconfigFiles(ctx, &p.mergedAconfigFiles)
}

func (p *PythonBinaryModule) buildBinary(ctx android.ModuleContext) {
	embeddedLauncher := p.isEmbeddedLauncherEnabled()
	depsSrcsZips := p.collectPathsFromTransitiveDeps(ctx, embeddedLauncher)
	main := ""
	if p.autorun() {
		main = p.getPyMainFile(ctx, p.srcsPathMappings)
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
	srcsZips := make(android.Paths, 0, len(depsSrcsZips)+1)
	if embeddedLauncher {
		srcsZips = append(srcsZips, p.precompiledSrcsZip)
	} else {
		srcsZips = append(srcsZips, p.srcsZip)
	}
	srcsZips = append(srcsZips, depsSrcsZips...)
	p.installSource = registerBuildActionForParFile(ctx, embeddedLauncher, launcherPath,
		p.getHostInterpreterName(ctx, p.properties.Actual_version),
		main, p.getStem(ctx), srcsZips)

	var sharedLibs []string
	// if embedded launcher is enabled, we need to collect the shared library dependencies of the
	// launcher
	for _, dep := range ctx.GetDirectDepsWithTag(launcherSharedLibTag) {
		sharedLibs = append(sharedLibs, ctx.OtherModuleName(dep))
	}
	p.androidMkSharedLibs = sharedLibs
}

func (p *PythonBinaryModule) AndroidMkEntries() []android.AndroidMkEntries {
	entries := android.AndroidMkEntries{OutputFile: android.OptionalPathForPath(p.installSource)}

	entries.Class = "EXECUTABLES"

	entries.ExtraEntries = append(entries.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.AddCompatibilityTestSuites(p.binaryProperties.Test_suites...)
		})

	entries.Required = append(entries.Required, "libc++")
	entries.ExtraEntries = append(entries.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			path, file := filepath.Split(p.installedDest.String())
			stem := strings.TrimSuffix(file, filepath.Ext(file))

			entries.SetString("LOCAL_MODULE_SUFFIX", filepath.Ext(file))
			entries.SetString("LOCAL_MODULE_PATH", path)
			entries.SetString("LOCAL_MODULE_STEM", stem)
			entries.AddStrings("LOCAL_SHARED_LIBRARIES", p.androidMkSharedLibs...)
			entries.SetBool("LOCAL_CHECK_ELF_FILES", false)
			android.SetAconfigFileMkEntries(&p.ModuleBase, entries, p.mergedAconfigFiles)
		})

	return []android.AndroidMkEntries{entries}
}

func (p *PythonBinaryModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	p.PythonLibraryModule.DepsMutator(ctx)

	if p.isEmbeddedLauncherEnabled() {
		p.AddDepsOnPythonLauncherAndStdlib(ctx, pythonLibTag, launcherTag, launcherSharedLibTag, p.autorun(), ctx.Target())
	}
}

// HostToolPath returns a path if appropriate such that this module can be used as a host tool,
// fulfilling the android.HostToolProvider interface.
func (p *PythonBinaryModule) HostToolPath() android.OptionalPath {
	// TODO: This should only be set when building host binaries -- tests built for device would be
	// setting this incorrectly.
	return android.OptionalPathForPath(p.installedDest)
}

// OutputFiles returns output files based on given tag, returns an error if tag is unsupported.
func (p *PythonBinaryModule) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{p.installSource}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (p *PythonBinaryModule) isEmbeddedLauncherEnabled() bool {
	return Bool(p.properties.Embedded_launcher)
}

func (b *PythonBinaryModule) autorun() bool {
	return BoolDefault(b.binaryProperties.Autorun, true)
}

// get host interpreter name.
func (p *PythonBinaryModule) getHostInterpreterName(ctx android.ModuleContext,
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
func (p *PythonBinaryModule) getPyMainFile(ctx android.ModuleContext,
	srcsPathMappings []pathMapping) string {
	var main string
	if String(p.binaryProperties.Main) == "" {
		main = ctx.ModuleName() + pyExt
	} else {
		main = String(p.binaryProperties.Main)
	}

	for _, path := range srcsPathMappings {
		if main == path.src.Rel() {
			return path.dest
		}
	}
	ctx.PropertyErrorf("main", "%q is not listed in srcs.", main)

	return ""
}

func (p *PythonBinaryModule) getStem(ctx android.ModuleContext) string {
	stem := ctx.ModuleName()
	if String(p.binaryProperties.Stem) != "" {
		stem = String(p.binaryProperties.Stem)
	}

	return stem + String(p.binaryProperties.Suffix)
}

func installDir(ctx android.ModuleContext, dir, dir64, relative string) android.InstallPath {
	if ctx.Arch().ArchType.Multilib == "lib64" && dir64 != "" {
		dir = dir64
	}
	if !ctx.Host() && ctx.Config().HasMultilibConflict(ctx.Arch().ArchType) {
		dir = filepath.Join(dir, ctx.Arch().ArchType.String())
	}
	return android.PathForModuleInstall(ctx, dir, relative)
}
