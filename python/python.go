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

// This file contains the "Base" module type for building Python program.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	registerPythonMutators(android.InitRegistrationContext)
}

func registerPythonMutators(ctx android.RegistrationContext) {
	ctx.PreDepsMutators(RegisterPythonPreDepsMutators)
}

// Exported to support other packages using Python modules in tests.
func RegisterPythonPreDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.BottomUp("python_version", versionSplitMutator()).Parallel()
}

// the version-specific properties that apply to python modules.
type VersionProperties struct {
	// whether the module is required to be built with this version.
	// Defaults to true for Python 3, and false otherwise.
	Enabled *bool

	// list of source files specific to this Python version.
	// Using the syntax ":module", srcs may reference the outputs of other modules that produce source files,
	// e.g. genrule or filegroup.
	Srcs []string `android:"path,arch_variant"`

	// list of source files that should not be used to build the Python module for this version.
	// This is most useful to remove files that are not common to all Python versions.
	Exclude_srcs []string `android:"path,arch_variant"`

	// list of the Python libraries used only for this Python version.
	Libs []string `android:"arch_variant"`

	// whether the binary is required to be built with embedded launcher for this version, defaults to false.
	Embedded_launcher *bool // TODO(b/174041232): Remove this property
}

// properties that apply to all python modules
type BaseProperties struct {
	// the package path prefix within the output artifact at which to place the source/data
	// files of the current module.
	// eg. Pkg_path = "a/b/c"; Other packages can reference this module by using
	// (from a.b.c import ...) statement.
	// if left unspecified, all the source/data files path is unchanged within zip file.
	Pkg_path *string

	// true, if the Python module is used internally, eg, Python std libs.
	Is_internal *bool

	// list of source (.py) files compatible both with Python2 and Python3 used to compile the
	// Python module.
	// srcs may reference the outputs of other modules that produce source files like genrule
	// or filegroup using the syntax ":module".
	// Srcs has to be non-empty.
	Srcs []string `android:"path,arch_variant"`

	// list of source files that should not be used to build the C/C++ module.
	// This is most useful in the arch/multilib variants to remove non-common files
	Exclude_srcs []string `android:"path,arch_variant"`

	// list of files or filegroup modules that provide data that should be installed alongside
	// the test. the file extension can be arbitrary except for (.py).
	Data []string `android:"path,arch_variant"`

	// list of java modules that provide data that should be installed alongside the test.
	Java_data []string

	// list of the Python libraries compatible both with Python2 and Python3.
	Libs []string `android:"arch_variant"`

	Version struct {
		// Python2-specific properties, including whether Python2 is supported for this module
		// and version-specific sources, exclusions and dependencies.
		Py2 VersionProperties `android:"arch_variant"`

		// Python3-specific properties, including whether Python3 is supported for this module
		// and version-specific sources, exclusions and dependencies.
		Py3 VersionProperties `android:"arch_variant"`
	} `android:"arch_variant"`

	// the actual version each module uses after variations created.
	// this property name is hidden from users' perspectives, and soong will populate it during
	// runtime.
	Actual_version string `blueprint:"mutated"`

	// whether the module is required to be built with actual_version.
	// this is set by the python version mutator based on version-specific properties
	Enabled *bool `blueprint:"mutated"`

	// whether the binary is required to be built with embedded launcher for this actual_version.
	// this is set by the python version mutator based on version-specific properties
	Embedded_launcher *bool `blueprint:"mutated"`
}

// Used to store files of current module after expanding dependencies
type pathMapping struct {
	dest string
	src  android.Path
}

type PythonLibraryModule struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties      BaseProperties
	protoProperties android.ProtoProperties

	// initialize before calling Init
	hod      android.HostOrDeviceSupported
	multilib android.Multilib

	// the Python files of current module after expanding source dependencies.
	// pathMapping: <dest: runfile_path, src: source_path>
	srcsPathMappings []pathMapping

	// the data files of current module after expanding source dependencies.
	// pathMapping: <dest: runfile_path, src: source_path>
	dataPathMappings []pathMapping

	// The zip file containing the current module's source/data files.
	srcsZip android.Path

	// The zip file containing the current module's source/data files, with the
	// source files precompiled.
	precompiledSrcsZip android.Path
}

// newModule generates new Python base module
func newModule(hod android.HostOrDeviceSupported, multilib android.Multilib) *PythonLibraryModule {
	return &PythonLibraryModule{
		hod:      hod,
		multilib: multilib,
	}
}

// interface implemented by Python modules to provide source and data mappings and zip to python
// modules that depend on it
type pythonDependency interface {
	getSrcsPathMappings() []pathMapping
	getDataPathMappings() []pathMapping
	getSrcsZip() android.Path
	getPrecompiledSrcsZip() android.Path
	getPkgPath() string
}

// getSrcsPathMappings gets this module's path mapping of src source path : runfiles destination
func (p *PythonLibraryModule) getSrcsPathMappings() []pathMapping {
	return p.srcsPathMappings
}

// getSrcsPathMappings gets this module's path mapping of data source path : runfiles destination
func (p *PythonLibraryModule) getDataPathMappings() []pathMapping {
	return p.dataPathMappings
}

// getSrcsZip returns the filepath where the current module's source/data files are zipped.
func (p *PythonLibraryModule) getSrcsZip() android.Path {
	return p.srcsZip
}

// getSrcsZip returns the filepath where the current module's source/data files are zipped.
func (p *PythonLibraryModule) getPrecompiledSrcsZip() android.Path {
	return p.precompiledSrcsZip
}

// getPkgPath returns the pkg_path value
func (p *PythonLibraryModule) getPkgPath() string {
	return String(p.properties.Pkg_path)
}

func (p *PythonLibraryModule) getBaseProperties() *BaseProperties {
	return &p.properties
}

var _ pythonDependency = (*PythonLibraryModule)(nil)

func (p *PythonLibraryModule) init() android.Module {
	p.AddProperties(&p.properties, &p.protoProperties)
	android.InitAndroidArchModule(p, p.hod, p.multilib)
	android.InitDefaultableModule(p)
	return p
}

// Python-specific tag to transfer information on the purpose of a dependency.
// This is used when adding a dependency on a module, which can later be accessed when visiting
// dependencies.
type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

// Python-specific tag that indicates that installed files of this module should depend on installed
// files of the dependency
type installDependencyTag struct {
	blueprint.BaseDependencyTag
	// embedding this struct provides the installation dependency requirement
	android.InstallAlwaysNeededDependencyTag
	name string
}

var (
	pythonLibTag = dependencyTag{name: "pythonLib"}
	javaDataTag  = dependencyTag{name: "javaData"}
	// The python interpreter, with soong module name "py3-launcher" or "py3-launcher-autorun".
	launcherTag          = dependencyTag{name: "launcher"}
	launcherSharedLibTag = installDependencyTag{name: "launcherSharedLib"}
	// The python interpreter built for host so that we can precompile python sources.
	// This only works because the precompiled sources don't vary by architecture.
	// The soong module name is "py3-launcher".
	hostLauncherTag          = dependencyTag{name: "hostLauncher"}
	hostlauncherSharedLibTag = dependencyTag{name: "hostlauncherSharedLib"}
	hostStdLibTag            = dependencyTag{name: "hostStdLib"}
	pathComponentRegexp      = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)
	pyExt                    = ".py"
	protoExt                 = ".proto"
	pyVersion2               = "PY2"
	pyVersion3               = "PY3"
	pyVersion2And3           = "PY2ANDPY3"
	internalPath             = "internal"
)

type basePropertiesProvider interface {
	getBaseProperties() *BaseProperties
}

// versionSplitMutator creates version variants for modules and appends the version-specific
// properties for a given variant to the properties in the variant module
func versionSplitMutator() func(android.BottomUpMutatorContext) {
	return func(mctx android.BottomUpMutatorContext) {
		if base, ok := mctx.Module().(basePropertiesProvider); ok {
			props := base.getBaseProperties()
			var versionNames []string
			// collect version specific properties, so that we can merge version-specific properties
			// into the module's overall properties
			var versionProps []VersionProperties
			// PY3 is first so that we alias the PY3 variant rather than PY2 if both
			// are available
			if proptools.BoolDefault(props.Version.Py3.Enabled, true) {
				versionNames = append(versionNames, pyVersion3)
				versionProps = append(versionProps, props.Version.Py3)
			}
			if proptools.BoolDefault(props.Version.Py2.Enabled, false) {
				if !mctx.DeviceConfig().BuildBrokenUsesSoongPython2Modules() &&
					mctx.ModuleName() != "py2-cmd" &&
					mctx.ModuleName() != "py2-stdlib" {
					mctx.PropertyErrorf("version.py2.enabled", "Python 2 is no longer supported, please convert to python 3. This error can be temporarily overridden by setting BUILD_BROKEN_USES_SOONG_PYTHON2_MODULES := true in the product configuration")
				}
				versionNames = append(versionNames, pyVersion2)
				versionProps = append(versionProps, props.Version.Py2)
			}
			modules := mctx.CreateLocalVariations(versionNames...)
			// Alias module to the first variant
			if len(versionNames) > 0 {
				mctx.AliasVariation(versionNames[0])
			}
			for i, v := range versionNames {
				// set the actual version for Python module.
				newProps := modules[i].(basePropertiesProvider).getBaseProperties()
				newProps.Actual_version = v
				// append versioned properties for the Python module to the overall properties
				err := proptools.AppendMatchingProperties([]interface{}{newProps}, &versionProps[i], nil)
				if err != nil {
					panic(err)
				}
			}
		}
	}
}

func anyHasExt(paths []string, ext string) bool {
	for _, p := range paths {
		if filepath.Ext(p) == ext {
			return true
		}
	}

	return false
}

func (p *PythonLibraryModule) anySrcHasExt(ctx android.BottomUpMutatorContext, ext string) bool {
	return anyHasExt(p.properties.Srcs, ext)
}

// DepsMutator mutates dependencies for this module:
//   - handles proto dependencies,
//   - if required, specifies launcher and adds launcher dependencies,
//   - applies python version mutations to Python dependencies
func (p *PythonLibraryModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	android.ProtoDeps(ctx, &p.protoProperties)

	versionVariation := []blueprint.Variation{
		{"python_version", p.properties.Actual_version},
	}

	// If sources contain a proto file, add dependency on libprotobuf-python
	if p.anySrcHasExt(ctx, protoExt) && p.Name() != "libprotobuf-python" {
		ctx.AddVariationDependencies(versionVariation, pythonLibTag, "libprotobuf-python")
	}

	// Add python library dependencies for this python version variation
	ctx.AddVariationDependencies(versionVariation, pythonLibTag, android.LastUniqueStrings(p.properties.Libs)...)

	// Emulate the data property for java_data but with the arch variation overridden to "common"
	// so that it can point to java modules.
	javaDataVariation := []blueprint.Variation{{"arch", android.Common.String()}}
	ctx.AddVariationDependencies(javaDataVariation, javaDataTag, p.properties.Java_data...)

	p.AddDepsOnPythonLauncherAndStdlib(ctx, hostStdLibTag, hostLauncherTag, hostlauncherSharedLibTag, false, ctx.Config().BuildOSTarget)
}

// AddDepsOnPythonLauncherAndStdlib will make the current module depend on the python stdlib,
// launcher (interpreter), and the launcher's shared libraries. If autorun is true, it will use
// the autorun launcher instead of the regular one. This function acceps a targetForDeps argument
// as the target to use for these dependencies. For embedded launcher python binaries, the launcher
// that will be embedded will be under the same target as the python module itself. But when
// precompiling python code, we need to get the python launcher built for host, even if we're
// compiling the python module for device, so we pass a different target to this function.
func (p *PythonLibraryModule) AddDepsOnPythonLauncherAndStdlib(ctx android.BottomUpMutatorContext,
	stdLibTag, launcherTag, launcherSharedLibTag blueprint.DependencyTag,
	autorun bool, targetForDeps android.Target) {
	var stdLib string
	var launcherModule string
	// Add launcher shared lib dependencies. Ideally, these should be
	// derived from the `shared_libs` property of the launcher. TODO: read these from
	// the python launcher itself using ctx.OtherModuleProvider() or similar on the result
	// of ctx.AddFarVariationDependencies()
	launcherSharedLibDeps := []string{
		"libsqlite",
	}
	// Add launcher-specific dependencies for bionic
	if targetForDeps.Os.Bionic() {
		launcherSharedLibDeps = append(launcherSharedLibDeps, "libc", "libdl", "libm")
	}
	if targetForDeps.Os == android.LinuxMusl && !ctx.Config().HostStaticBinaries() {
		launcherSharedLibDeps = append(launcherSharedLibDeps, "libc_musl")
	}

	switch p.properties.Actual_version {
	case pyVersion2:
		stdLib = "py2-stdlib"

		launcherModule = "py2-launcher"
		if autorun {
			launcherModule = "py2-launcher-autorun"
		}

		launcherSharedLibDeps = append(launcherSharedLibDeps, "libc++")
	case pyVersion3:
		var prebuiltStdLib bool
		if targetForDeps.Os.Bionic() {
			prebuiltStdLib = false
		} else if ctx.Config().VendorConfig("cpython3").Bool("force_build_host") {
			prebuiltStdLib = false
		} else {
			prebuiltStdLib = true
		}

		if prebuiltStdLib {
			stdLib = "py3-stdlib-prebuilt"
		} else {
			stdLib = "py3-stdlib"
		}

		launcherModule = "py3-launcher"
		if autorun {
			launcherModule = "py3-launcher-autorun"
		}
		if ctx.Config().HostStaticBinaries() && targetForDeps.Os == android.LinuxMusl {
			launcherModule += "-static"
		}
		if ctx.Device() {
			launcherSharedLibDeps = append(launcherSharedLibDeps, "liblog")
		}
	default:
		panic(fmt.Errorf("unknown Python Actual_version: %q for module: %q.",
			p.properties.Actual_version, ctx.ModuleName()))
	}
	targetVariations := targetForDeps.Variations()
	if ctx.ModuleName() != stdLib {
		stdLibVariations := make([]blueprint.Variation, 0, len(targetVariations)+1)
		stdLibVariations = append(stdLibVariations, blueprint.Variation{Mutator: "python_version", Variation: p.properties.Actual_version})
		stdLibVariations = append(stdLibVariations, targetVariations...)
		// Using AddFarVariationDependencies for all of these because they can be for a different
		// platform, like if the python module itself was being compiled for device, we may want
		// the python interpreter built for host so that we can precompile python sources.
		ctx.AddFarVariationDependencies(stdLibVariations, stdLibTag, stdLib)
	}
	ctx.AddFarVariationDependencies(targetVariations, launcherTag, launcherModule)
	ctx.AddFarVariationDependencies(targetVariations, launcherSharedLibTag, launcherSharedLibDeps...)
}

// GenerateAndroidBuildActions performs build actions common to all Python modules
func (p *PythonLibraryModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	expandedSrcs := android.PathsForModuleSrcExcludes(ctx, p.properties.Srcs, p.properties.Exclude_srcs)
	android.SetProvider(ctx, blueprint.SrcsFileProviderKey, blueprint.SrcsFileProviderData{SrcPaths: expandedSrcs.Strings()})

	// expand data files from "data" property.
	expandedData := android.PathsForModuleSrc(ctx, p.properties.Data)

	// Emulate the data property for java_data dependencies.
	for _, javaData := range ctx.GetDirectDepsWithTag(javaDataTag) {
		expandedData = append(expandedData, android.OutputFilesForModule(ctx, javaData, "")...)
	}

	// Validate pkg_path property
	pkgPath := String(p.properties.Pkg_path)
	if pkgPath != "" {
		// TODO: export validation from android/paths.go handling to replace this duplicated functionality
		pkgPath = filepath.Clean(String(p.properties.Pkg_path))
		if pkgPath == ".." || strings.HasPrefix(pkgPath, "../") ||
			strings.HasPrefix(pkgPath, "/") {
			ctx.PropertyErrorf("pkg_path",
				"%q must be a relative path contained in par file.",
				String(p.properties.Pkg_path))
			return
		}
	}
	// If property Is_internal is set, prepend pkgPath with internalPath
	if proptools.BoolDefault(p.properties.Is_internal, false) {
		pkgPath = filepath.Join(internalPath, pkgPath)
	}

	// generate src:destination path mappings for this module
	p.genModulePathMappings(ctx, pkgPath, expandedSrcs, expandedData)

	// generate the zipfile of all source and data files
	p.srcsZip = p.createSrcsZip(ctx, pkgPath)
	p.precompiledSrcsZip = p.precompileSrcs(ctx)
}

func isValidPythonPath(path string) error {
	identifiers := strings.Split(strings.TrimSuffix(path, filepath.Ext(path)), "/")
	for _, token := range identifiers {
		if !pathComponentRegexp.MatchString(token) {
			return fmt.Errorf("the path %q contains invalid subpath %q. "+
				"Subpaths must be at least one character long. "+
				"The first character must an underscore or letter. "+
				"Following characters may be any of: letter, digit, underscore, hyphen.",
				path, token)
		}
	}
	return nil
}

// For this module, generate unique pathMappings: <dest: runfiles_path, src: source_path>
// for python/data files expanded from properties.
func (p *PythonLibraryModule) genModulePathMappings(ctx android.ModuleContext, pkgPath string,
	expandedSrcs, expandedData android.Paths) {
	// fetch <runfiles_path, source_path> pairs from "src" and "data" properties to
	// check current module duplicates.
	destToPySrcs := make(map[string]string)
	destToPyData := make(map[string]string)

	// Disable path checks for the stdlib, as it includes a "." in the version string
	isInternal := proptools.BoolDefault(p.properties.Is_internal, false)

	for _, s := range expandedSrcs {
		if s.Ext() != pyExt && s.Ext() != protoExt {
			ctx.PropertyErrorf("srcs", "found non (.py|.proto) file: %q!", s.String())
			continue
		}
		runfilesPath := filepath.Join(pkgPath, s.Rel())
		if !isInternal {
			if err := isValidPythonPath(runfilesPath); err != nil {
				ctx.PropertyErrorf("srcs", err.Error())
			}
		}
		if !checkForDuplicateOutputPath(ctx, destToPySrcs, runfilesPath, s.String(), p.Name(), p.Name()) {
			p.srcsPathMappings = append(p.srcsPathMappings, pathMapping{dest: runfilesPath, src: s})
		}
	}

	for _, d := range expandedData {
		if d.Ext() == pyExt || d.Ext() == protoExt {
			ctx.PropertyErrorf("data", "found (.py|.proto) file: %q!", d.String())
			continue
		}
		runfilesPath := filepath.Join(pkgPath, d.Rel())
		if !checkForDuplicateOutputPath(ctx, destToPyData, runfilesPath, d.String(), p.Name(), p.Name()) {
			p.dataPathMappings = append(p.dataPathMappings,
				pathMapping{dest: runfilesPath, src: d})
		}
	}
}

// createSrcsZip registers build actions to zip current module's sources and data.
func (p *PythonLibraryModule) createSrcsZip(ctx android.ModuleContext, pkgPath string) android.Path {
	relativeRootMap := make(map[string]android.Paths)
	var protoSrcs android.Paths
	addPathMapping := func(path pathMapping) {
		// handle proto sources separately
		if path.src.Ext() == protoExt {
			protoSrcs = append(protoSrcs, path.src)
		} else {
			relativeRoot := strings.TrimSuffix(path.src.String(), path.src.Rel())
			relativeRootMap[relativeRoot] = append(relativeRootMap[relativeRoot], path.src)
		}
	}

	// "srcs" or "data" properties may contain filegroups so it might happen that
	// the root directory for each source path is different.
	for _, path := range p.srcsPathMappings {
		addPathMapping(path)
	}
	for _, path := range p.dataPathMappings {
		addPathMapping(path)
	}

	var zips android.Paths
	if len(protoSrcs) > 0 {
		protoFlags := android.GetProtoFlags(ctx, &p.protoProperties)
		protoFlags.OutTypeFlag = "--python_out"

		if pkgPath != "" {
			pkgPathStagingDir := android.PathForModuleGen(ctx, "protos_staged_for_pkg_path")
			rule := android.NewRuleBuilder(pctx, ctx)
			var stagedProtoSrcs android.Paths
			for _, srcFile := range protoSrcs {
				stagedProtoSrc := pkgPathStagingDir.Join(ctx, pkgPath, srcFile.Rel())
				rule.Command().Text("mkdir -p").Flag(filepath.Base(stagedProtoSrc.String()))
				rule.Command().Text("cp -f").Input(srcFile).Output(stagedProtoSrc)
				stagedProtoSrcs = append(stagedProtoSrcs, stagedProtoSrc)
			}
			rule.Build("stage_protos_for_pkg_path", "Stage protos for pkg_path")
			protoSrcs = stagedProtoSrcs
		}

		for _, srcFile := range protoSrcs {
			zip := genProto(ctx, srcFile, protoFlags)
			zips = append(zips, zip)
		}
	}

	if len(relativeRootMap) > 0 {
		// in order to keep stable order of soong_zip params, we sort the keys here.
		roots := android.SortedKeys(relativeRootMap)

		// Use -symlinks=false so that the symlinks in the bazel output directory are followed
		parArgs := []string{"-symlinks=false"}
		if pkgPath != "" {
			// use package path as path prefix
			parArgs = append(parArgs, `-P `+pkgPath)
		}
		paths := android.Paths{}
		for _, root := range roots {
			// specify relative root of file in following -f arguments
			parArgs = append(parArgs, `-C `+root)
			for _, path := range relativeRootMap[root] {
				parArgs = append(parArgs, `-f `+path.String())
				paths = append(paths, path)
			}
		}

		origSrcsZip := android.PathForModuleOut(ctx, ctx.ModuleName()+".py.srcszip")
		ctx.Build(pctx, android.BuildParams{
			Rule:        zip,
			Description: "python library archive",
			Output:      origSrcsZip,
			// as zip rule does not use $in, there is no real need to distinguish between Inputs and Implicits
			Implicits: paths,
			Args: map[string]string{
				"args": strings.Join(parArgs, " "),
			},
		})
		zips = append(zips, origSrcsZip)
	}
	// we may have multiple zips due to separate handling of proto source files
	if len(zips) == 1 {
		return zips[0]
	} else {
		combinedSrcsZip := android.PathForModuleOut(ctx, ctx.ModuleName()+".srcszip")
		ctx.Build(pctx, android.BuildParams{
			Rule:        combineZip,
			Description: "combine python library archive",
			Output:      combinedSrcsZip,
			Inputs:      zips,
		})
		return combinedSrcsZip
	}
}

func (p *PythonLibraryModule) precompileSrcs(ctx android.ModuleContext) android.Path {
	// To precompile the python sources, we need a python interpreter and stdlib built
	// for host. We then use those to compile the python sources, which may be used on either
	// host of device. Python bytecode is architecture agnostic, so we're essentially
	// "cross compiling" for device here purely by virtue of host and device python bytecode
	// being the same.
	var stdLib android.Path
	var stdLibPkg string
	var launcher android.Path
	if proptools.BoolDefault(p.properties.Is_internal, false) {
		stdLib = p.srcsZip
		stdLibPkg = p.getPkgPath()
	} else {
		ctx.VisitDirectDepsWithTag(hostStdLibTag, func(module android.Module) {
			if dep, ok := module.(pythonDependency); ok {
				stdLib = dep.getPrecompiledSrcsZip()
				stdLibPkg = dep.getPkgPath()
			}
		})
	}
	ctx.VisitDirectDepsWithTag(hostLauncherTag, func(module android.Module) {
		if dep, ok := module.(IntermPathProvider); ok {
			optionalLauncher := dep.IntermPathForModuleOut()
			if optionalLauncher.Valid() {
				launcher = optionalLauncher.Path()
			}
		}
	})
	var launcherSharedLibs android.Paths
	var ldLibraryPath []string
	ctx.VisitDirectDepsWithTag(hostlauncherSharedLibTag, func(module android.Module) {
		if dep, ok := module.(IntermPathProvider); ok {
			optionalPath := dep.IntermPathForModuleOut()
			if optionalPath.Valid() {
				launcherSharedLibs = append(launcherSharedLibs, optionalPath.Path())
				ldLibraryPath = append(ldLibraryPath, filepath.Dir(optionalPath.Path().String()))
			}
		}
	})

	out := android.PathForModuleOut(ctx, ctx.ModuleName()+".srcszipprecompiled")
	if stdLib == nil || launcher == nil {
		// This shouldn't happen in a real build because we'll error out when adding dependencies
		// on the stdlib and launcher if they don't exist. But some tests set
		// AllowMissingDependencies.
		return out
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        precompile,
		Input:       p.srcsZip,
		Output:      out,
		Implicits:   launcherSharedLibs,
		Description: "Precompile the python sources of " + ctx.ModuleName(),
		Args: map[string]string{
			"stdlibZip":     stdLib.String(),
			"stdlibPkg":     stdLibPkg,
			"launcher":      launcher.String(),
			"ldLibraryPath": strings.Join(ldLibraryPath, ":"),
		},
	})
	return out
}

// isPythonLibModule returns whether the given module is a Python library PythonLibraryModule or not
func isPythonLibModule(module blueprint.Module) bool {
	if _, ok := module.(*PythonLibraryModule); ok {
		if _, ok := module.(*PythonBinaryModule); !ok {
			return true
		}
	}
	return false
}

// collectPathsFromTransitiveDeps checks for source/data files for duplicate paths
// for module and its transitive dependencies and collects list of data/source file
// zips for transitive dependencies.
func (p *PythonLibraryModule) collectPathsFromTransitiveDeps(ctx android.ModuleContext, precompiled bool) android.Paths {
	// fetch <runfiles_path, source_path> pairs from "src" and "data" properties to
	// check duplicates.
	destToPySrcs := make(map[string]string)
	destToPyData := make(map[string]string)
	for _, path := range p.srcsPathMappings {
		destToPySrcs[path.dest] = path.src.String()
	}
	for _, path := range p.dataPathMappings {
		destToPyData[path.dest] = path.src.String()
	}

	seen := make(map[android.Module]bool)

	var result android.Paths

	// visit all its dependencies in depth first.
	ctx.WalkDeps(func(child, parent android.Module) bool {
		// we only collect dependencies tagged as python library deps
		if ctx.OtherModuleDependencyTag(child) != pythonLibTag {
			return false
		}
		if seen[child] {
			return false
		}
		seen[child] = true
		// Python modules only can depend on Python libraries.
		if !isPythonLibModule(child) {
			ctx.PropertyErrorf("libs",
				"the dependency %q of module %q is not Python library!",
				ctx.OtherModuleName(child), ctx.ModuleName())
		}
		// collect source and data paths, checking that there are no duplicate output file conflicts
		if dep, ok := child.(pythonDependency); ok {
			srcs := dep.getSrcsPathMappings()
			for _, path := range srcs {
				checkForDuplicateOutputPath(ctx, destToPySrcs,
					path.dest, path.src.String(), ctx.ModuleName(), ctx.OtherModuleName(child))
			}
			data := dep.getDataPathMappings()
			for _, path := range data {
				checkForDuplicateOutputPath(ctx, destToPyData,
					path.dest, path.src.String(), ctx.ModuleName(), ctx.OtherModuleName(child))
			}
			if precompiled {
				result = append(result, dep.getPrecompiledSrcsZip())
			} else {
				result = append(result, dep.getSrcsZip())
			}
		}
		return true
	})
	return result
}

// chckForDuplicateOutputPath checks whether outputPath has already been included in map m, which
// would result in two files being placed in the same location.
// If there is a duplicate path, an error is thrown and true is returned
// Otherwise, outputPath: srcPath is added to m and returns false
func checkForDuplicateOutputPath(ctx android.ModuleContext, m map[string]string, outputPath, srcPath, curModule, otherModule string) bool {
	if oldSrcPath, found := m[outputPath]; found {
		ctx.ModuleErrorf("found two files to be placed at the same location within zip %q."+
			" First file: in module %s at path %q."+
			" Second file: in module %s at path %q.",
			outputPath, curModule, oldSrcPath, otherModule, srcPath)
		return true
	}
	m[outputPath] = srcPath

	return false
}

// InstallInData returns true as Python is not supported in the system partition
func (p *PythonLibraryModule) InstallInData() bool {
	return true
}

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
var String = proptools.String
