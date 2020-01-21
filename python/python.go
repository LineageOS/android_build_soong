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
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	android.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("version_split", versionSplitMutator()).Parallel()
	})
}

// the version properties that apply to python libraries and binaries.
type VersionProperties struct {
	// true, if the module is required to be built with this version.
	Enabled *bool `android:"arch_variant"`

	// non-empty list of .py files under this strict Python version.
	// srcs may reference the outputs of other modules that produce source files like genrule
	// or filegroup using the syntax ":module".
	Srcs []string `android:"path,arch_variant"`

	// list of source files that should not be used to build the Python module.
	// This is most useful in the arch/multilib variants to remove non-common files
	Exclude_srcs []string `android:"path,arch_variant"`

	// list of the Python libraries under this Python version.
	Libs []string `android:"arch_variant"`

	// true, if the binary is required to be built with embedded launcher.
	// TODO(nanzhang): Remove this flag when embedded Python3 is supported later.
	Embedded_launcher *bool `android:"arch_variant"`
}

// properties that apply to python libraries and binaries.
type BaseProperties struct {
	// the package path prefix within the output artifact at which to place the source/data
	// files of the current module.
	// eg. Pkg_path = "a/b/c"; Other packages can reference this module by using
	// (from a.b.c import ...) statement.
	// if left unspecified, all the source/data files path is unchanged within zip file.
	Pkg_path *string `android:"arch_variant"`

	// true, if the Python module is used internally, eg, Python std libs.
	Is_internal *bool `android:"arch_variant"`

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

	// list of the Python libraries compatible both with Python2 and Python3.
	Libs []string `android:"arch_variant"`

	Version struct {
		// all the "srcs" or Python dependencies that are to be used only for Python2.
		Py2 VersionProperties `android:"arch_variant"`

		// all the "srcs" or Python dependencies that are to be used only for Python3.
		Py3 VersionProperties `android:"arch_variant"`
	} `android:"arch_variant"`

	// the actual version each module uses after variations created.
	// this property name is hidden from users' perspectives, and soong will populate it during
	// runtime.
	Actual_version string `blueprint:"mutated"`
}

type pathMapping struct {
	dest string
	src  android.Path
}

type Module struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties      BaseProperties
	protoProperties android.ProtoProperties

	// initialize before calling Init
	hod      android.HostOrDeviceSupported
	multilib android.Multilib

	// the bootstrapper is used to bootstrap .par executable.
	// bootstrapper might be nil (Python library module).
	bootstrapper bootstrapper

	// the installer might be nil.
	installer installer

	// the Python files of current module after expanding source dependencies.
	// pathMapping: <dest: runfile_path, src: source_path>
	srcsPathMappings []pathMapping

	// the data files of current module after expanding source dependencies.
	// pathMapping: <dest: runfile_path, src: source_path>
	dataPathMappings []pathMapping

	// the zip filepath for zipping current module source/data files.
	srcsZip android.Path

	// dependency modules' zip filepath for zipping current module source/data files.
	depsSrcsZips android.Paths

	// (.intermediate) module output path as installation source.
	installSource android.OptionalPath

	subAndroidMkOnce map[subAndroidMkProvider]bool
}

func newModule(hod android.HostOrDeviceSupported, multilib android.Multilib) *Module {
	return &Module{
		hod:      hod,
		multilib: multilib,
	}
}

type bootstrapper interface {
	bootstrapperProps() []interface{}
	bootstrap(ctx android.ModuleContext, ActualVersion string, embeddedLauncher bool,
		srcsPathMappings []pathMapping, srcsZip android.Path,
		depsSrcsZips android.Paths) android.OptionalPath

	autorun() bool
}

type installer interface {
	install(ctx android.ModuleContext, path android.Path)
	setAndroidMkSharedLibs(sharedLibs []string)
}

type PythonDependency interface {
	GetSrcsPathMappings() []pathMapping
	GetDataPathMappings() []pathMapping
	GetSrcsZip() android.Path
}

func (p *Module) GetSrcsPathMappings() []pathMapping {
	return p.srcsPathMappings
}

func (p *Module) GetDataPathMappings() []pathMapping {
	return p.dataPathMappings
}

func (p *Module) GetSrcsZip() android.Path {
	return p.srcsZip
}

var _ PythonDependency = (*Module)(nil)

var _ android.AndroidMkDataProvider = (*Module)(nil)

func (p *Module) Init() android.Module {

	p.AddProperties(&p.properties, &p.protoProperties)
	if p.bootstrapper != nil {
		p.AddProperties(p.bootstrapper.bootstrapperProps()...)
	}

	android.InitAndroidArchModule(p, p.hod, p.multilib)
	android.InitDefaultableModule(p)

	return p
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var (
	pythonLibTag         = dependencyTag{name: "pythonLib"}
	launcherTag          = dependencyTag{name: "launcher"}
	launcherSharedLibTag = dependencyTag{name: "launcherSharedLib"}
	pyIdentifierRegexp   = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)
	pyExt                = ".py"
	protoExt             = ".proto"
	pyVersion2           = "PY2"
	pyVersion3           = "PY3"
	initFileName         = "__init__.py"
	mainFileName         = "__main__.py"
	entryPointFile       = "entry_point.txt"
	parFileExt           = ".zip"
	internal             = "internal"
)

// create version variants for modules.
func versionSplitMutator() func(android.BottomUpMutatorContext) {
	return func(mctx android.BottomUpMutatorContext) {
		if base, ok := mctx.Module().(*Module); ok {
			versionNames := []string{}
			if base.properties.Version.Py2.Enabled != nil &&
				*(base.properties.Version.Py2.Enabled) == true {
				versionNames = append(versionNames, pyVersion2)
			}
			if !(base.properties.Version.Py3.Enabled != nil &&
				*(base.properties.Version.Py3.Enabled) == false) {
				versionNames = append(versionNames, pyVersion3)
			}
			modules := mctx.CreateVariations(versionNames...)
			for i, v := range versionNames {
				// set the actual version for Python module.
				modules[i].(*Module).properties.Actual_version = v
			}
		}
	}
}

func (p *Module) HostToolPath() android.OptionalPath {
	if p.installer == nil {
		// python_library is just meta module, and doesn't have any installer.
		return android.OptionalPath{}
	}
	return android.OptionalPathForPath(p.installer.(*binaryDecorator).path)
}

func (p *Module) isEmbeddedLauncherEnabled(actual_version string) bool {
	switch actual_version {
	case pyVersion2:
		return Bool(p.properties.Version.Py2.Embedded_launcher)
	case pyVersion3:
		return Bool(p.properties.Version.Py3.Embedded_launcher)
	}

	return false
}

func hasSrcExt(srcs []string, ext string) bool {
	for _, src := range srcs {
		if filepath.Ext(src) == ext {
			return true
		}
	}

	return false
}

func (p *Module) hasSrcExt(ctx android.BottomUpMutatorContext, ext string) bool {
	if hasSrcExt(p.properties.Srcs, protoExt) {
		return true
	}
	switch p.properties.Actual_version {
	case pyVersion2:
		return hasSrcExt(p.properties.Version.Py2.Srcs, protoExt)
	case pyVersion3:
		return hasSrcExt(p.properties.Version.Py3.Srcs, protoExt)
	default:
		panic(fmt.Errorf("unknown Python Actual_version: %q for module: %q.",
			p.properties.Actual_version, ctx.ModuleName()))
	}
}

func (p *Module) DepsMutator(ctx android.BottomUpMutatorContext) {
	android.ProtoDeps(ctx, &p.protoProperties)

	if p.hasSrcExt(ctx, protoExt) && p.Name() != "libprotobuf-python" {
		ctx.AddVariationDependencies(nil, pythonLibTag, "libprotobuf-python")
	}
	switch p.properties.Actual_version {
	case pyVersion2:
		ctx.AddVariationDependencies(nil, pythonLibTag,
			uniqueLibs(ctx, p.properties.Libs, "version.py2.libs",
				p.properties.Version.Py2.Libs)...)

		if p.bootstrapper != nil && p.isEmbeddedLauncherEnabled(pyVersion2) {
			ctx.AddVariationDependencies(nil, pythonLibTag, "py2-stdlib")

			launcherModule := "py2-launcher"
			if p.bootstrapper.autorun() {
				launcherModule = "py2-launcher-autorun"
			}
			ctx.AddFarVariationDependencies(ctx.Target().Variations(), launcherTag, launcherModule)

			// Add py2-launcher shared lib dependencies. Ideally, these should be
			// derived from the `shared_libs` property of "py2-launcher". However, we
			// cannot read the property at this stage and it will be too late to add
			// dependencies later.
			ctx.AddFarVariationDependencies(ctx.Target().Variations(), launcherSharedLibTag, "libsqlite")

			if ctx.Target().Os.Bionic() {
				ctx.AddFarVariationDependencies(ctx.Target().Variations(), launcherSharedLibTag,
					"libc", "libdl", "libm")
			}
		}

	case pyVersion3:
		ctx.AddVariationDependencies(nil, pythonLibTag,
			uniqueLibs(ctx, p.properties.Libs, "version.py3.libs",
				p.properties.Version.Py3.Libs)...)

		if p.bootstrapper != nil && p.isEmbeddedLauncherEnabled(pyVersion3) {
			ctx.AddVariationDependencies(nil, pythonLibTag, "py3-stdlib")

			launcherModule := "py3-launcher"
			if p.bootstrapper.autorun() {
				launcherModule = "py3-launcher-autorun"
			}
			ctx.AddFarVariationDependencies(ctx.Target().Variations(), launcherTag, launcherModule)

			// Add py3-launcher shared lib dependencies. Ideally, these should be
			// derived from the `shared_libs` property of "py3-launcher". However, we
			// cannot read the property at this stage and it will be too late to add
			// dependencies later.
			ctx.AddFarVariationDependencies(ctx.Target().Variations(), launcherSharedLibTag, "libsqlite")

			if ctx.Device() {
				ctx.AddFarVariationDependencies(ctx.Target().Variations(), launcherSharedLibTag,
					"liblog")
			}

			if ctx.Target().Os.Bionic() {
				ctx.AddFarVariationDependencies(ctx.Target().Variations(), launcherSharedLibTag,
					"libc", "libdl", "libm")
			}
		}
	default:
		panic(fmt.Errorf("unknown Python Actual_version: %q for module: %q.",
			p.properties.Actual_version, ctx.ModuleName()))
	}
}

// check "libs" duplicates from current module dependencies.
func uniqueLibs(ctx android.BottomUpMutatorContext,
	commonLibs []string, versionProp string, versionLibs []string) []string {
	set := make(map[string]string)
	ret := []string{}

	// deps from "libs" property.
	for _, l := range commonLibs {
		if _, found := set[l]; found {
			ctx.PropertyErrorf("libs", "%q has duplicates within libs.", l)
		} else {
			set[l] = "libs"
			ret = append(ret, l)
		}
	}
	// deps from "version.pyX.libs" property.
	for _, l := range versionLibs {
		if _, found := set[l]; found {
			ctx.PropertyErrorf(versionProp, "%q has duplicates within %q.", set[l])
		} else {
			set[l] = versionProp
			ret = append(ret, l)
		}
	}

	return ret
}

func (p *Module) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.GeneratePythonBuildActions(ctx)

	// Only Python binaries and test has non-empty bootstrapper.
	if p.bootstrapper != nil {
		p.walkTransitiveDeps(ctx)
		embeddedLauncher := false
		if p.properties.Actual_version == pyVersion2 {
			embeddedLauncher = p.isEmbeddedLauncherEnabled(pyVersion2)
		} else {
			embeddedLauncher = p.isEmbeddedLauncherEnabled(pyVersion3)
		}
		p.installSource = p.bootstrapper.bootstrap(ctx, p.properties.Actual_version,
			embeddedLauncher, p.srcsPathMappings, p.srcsZip, p.depsSrcsZips)
	}

	if p.installer != nil {
		var sharedLibs []string
		ctx.VisitDirectDeps(func(dep android.Module) {
			if ctx.OtherModuleDependencyTag(dep) == launcherSharedLibTag {
				sharedLibs = append(sharedLibs, ctx.OtherModuleName(dep))
			}
		})
		p.installer.setAndroidMkSharedLibs(sharedLibs)

		if p.installSource.Valid() {
			p.installer.install(ctx, p.installSource.Path())
		}
	}

}

func (p *Module) GeneratePythonBuildActions(ctx android.ModuleContext) {
	// expand python files from "srcs" property.
	srcs := p.properties.Srcs
	exclude_srcs := p.properties.Exclude_srcs
	switch p.properties.Actual_version {
	case pyVersion2:
		srcs = append(srcs, p.properties.Version.Py2.Srcs...)
		exclude_srcs = append(exclude_srcs, p.properties.Version.Py2.Exclude_srcs...)
	case pyVersion3:
		srcs = append(srcs, p.properties.Version.Py3.Srcs...)
		exclude_srcs = append(exclude_srcs, p.properties.Version.Py3.Exclude_srcs...)
	default:
		panic(fmt.Errorf("unknown Python Actual_version: %q for module: %q.",
			p.properties.Actual_version, ctx.ModuleName()))
	}
	expandedSrcs := android.PathsForModuleSrcExcludes(ctx, srcs, exclude_srcs)
	requiresSrcs := true
	if p.bootstrapper != nil && !p.bootstrapper.autorun() {
		requiresSrcs = false
	}
	if len(expandedSrcs) == 0 && requiresSrcs {
		ctx.ModuleErrorf("doesn't have any source files!")
	}

	// expand data files from "data" property.
	expandedData := android.PathsForModuleSrc(ctx, p.properties.Data)

	// sanitize pkg_path.
	pkgPath := String(p.properties.Pkg_path)
	if pkgPath != "" {
		pkgPath = filepath.Clean(String(p.properties.Pkg_path))
		if pkgPath == ".." || strings.HasPrefix(pkgPath, "../") ||
			strings.HasPrefix(pkgPath, "/") {
			ctx.PropertyErrorf("pkg_path",
				"%q must be a relative path contained in par file.",
				String(p.properties.Pkg_path))
			return
		}
		if p.properties.Is_internal != nil && *p.properties.Is_internal {
			pkgPath = filepath.Join(internal, pkgPath)
		}
	} else {
		if p.properties.Is_internal != nil && *p.properties.Is_internal {
			pkgPath = internal
		}
	}

	p.genModulePathMappings(ctx, pkgPath, expandedSrcs, expandedData)

	p.srcsZip = p.createSrcsZip(ctx, pkgPath)
}

// generate current module unique pathMappings: <dest: runfiles_path, src: source_path>
// for python/data files.
func (p *Module) genModulePathMappings(ctx android.ModuleContext, pkgPath string,
	expandedSrcs, expandedData android.Paths) {
	// fetch <runfiles_path, source_path> pairs from "src" and "data" properties to
	// check current module duplicates.
	destToPySrcs := make(map[string]string)
	destToPyData := make(map[string]string)

	for _, s := range expandedSrcs {
		if s.Ext() != pyExt && s.Ext() != protoExt {
			ctx.PropertyErrorf("srcs", "found non (.py|.proto) file: %q!", s.String())
			continue
		}
		runfilesPath := filepath.Join(pkgPath, s.Rel())
		identifiers := strings.Split(strings.TrimSuffix(runfilesPath,
			filepath.Ext(runfilesPath)), "/")
		for _, token := range identifiers {
			if !pyIdentifierRegexp.MatchString(token) {
				ctx.PropertyErrorf("srcs", "the path %q contains invalid token %q.",
					runfilesPath, token)
			}
		}
		if fillInMap(ctx, destToPySrcs, runfilesPath, s.String(), p.Name(), p.Name()) {
			p.srcsPathMappings = append(p.srcsPathMappings,
				pathMapping{dest: runfilesPath, src: s})
		}
	}

	for _, d := range expandedData {
		if d.Ext() == pyExt || d.Ext() == protoExt {
			ctx.PropertyErrorf("data", "found (.py|.proto) file: %q!", d.String())
			continue
		}
		runfilesPath := filepath.Join(pkgPath, d.Rel())
		if fillInMap(ctx, destToPyData, runfilesPath, d.String(), p.Name(), p.Name()) {
			p.dataPathMappings = append(p.dataPathMappings,
				pathMapping{dest: runfilesPath, src: d})
		}
	}
}

// register build actions to zip current module's sources.
func (p *Module) createSrcsZip(ctx android.ModuleContext, pkgPath string) android.Path {
	relativeRootMap := make(map[string]android.Paths)
	pathMappings := append(p.srcsPathMappings, p.dataPathMappings...)

	var protoSrcs android.Paths
	// "srcs" or "data" properties may have filegroup so it might happen that
	// the relative root for each source path is different.
	for _, path := range pathMappings {
		if path.src.Ext() == protoExt {
			protoSrcs = append(protoSrcs, path.src)
		} else {
			var relativeRoot string
			relativeRoot = strings.TrimSuffix(path.src.String(), path.src.Rel())
			if v, found := relativeRootMap[relativeRoot]; found {
				relativeRootMap[relativeRoot] = append(v, path.src)
			} else {
				relativeRootMap[relativeRoot] = android.Paths{path.src}
			}
		}
	}
	var zips android.Paths
	if len(protoSrcs) > 0 {
		protoFlags := android.GetProtoFlags(ctx, &p.protoProperties)
		protoFlags.OutTypeFlag = "--python_out"

		for _, srcFile := range protoSrcs {
			zip := genProto(ctx, srcFile, protoFlags, pkgPath)
			zips = append(zips, zip)
		}
	}

	if len(relativeRootMap) > 0 {
		var keys []string

		// in order to keep stable order of soong_zip params, we sort the keys here.
		for k := range relativeRootMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		parArgs := []string{}
		if pkgPath != "" {
			parArgs = append(parArgs, `-P `+pkgPath)
		}
		implicits := android.Paths{}
		for _, k := range keys {
			parArgs = append(parArgs, `-C `+k)
			for _, path := range relativeRootMap[k] {
				parArgs = append(parArgs, `-f `+path.String())
				implicits = append(implicits, path)
			}
		}

		origSrcsZip := android.PathForModuleOut(ctx, ctx.ModuleName()+".py.srcszip")
		ctx.Build(pctx, android.BuildParams{
			Rule:        zip,
			Description: "python library archive",
			Output:      origSrcsZip,
			Implicits:   implicits,
			Args: map[string]string{
				"args": strings.Join(parArgs, " "),
			},
		})
		zips = append(zips, origSrcsZip)
	}
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

func isPythonLibModule(module blueprint.Module) bool {
	if m, ok := module.(*Module); ok {
		// Python library has no bootstrapper or installer.
		if m.bootstrapper != nil || m.installer != nil {
			return false
		}
		return true
	}
	return false
}

// check Python source/data files duplicates for whole runfiles tree since Python binary/test
// need collect and zip all srcs of whole transitive dependencies to a final par file.
func (p *Module) walkTransitiveDeps(ctx android.ModuleContext) {
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

	// visit all its dependencies in depth first.
	ctx.WalkDeps(func(child, parent android.Module) bool {
		if ctx.OtherModuleDependencyTag(child) != pythonLibTag {
			return false
		}
		if seen[child] {
			return false
		}
		seen[child] = true
		// Python modules only can depend on Python libraries.
		if !isPythonLibModule(child) {
			panic(fmt.Errorf(
				"the dependency %q of module %q is not Python library!",
				ctx.ModuleName(), ctx.OtherModuleName(child)))
		}
		if dep, ok := child.(PythonDependency); ok {
			srcs := dep.GetSrcsPathMappings()
			for _, path := range srcs {
				if !fillInMap(ctx, destToPySrcs,
					path.dest, path.src.String(), ctx.ModuleName(), ctx.OtherModuleName(child)) {
					continue
				}
			}
			data := dep.GetDataPathMappings()
			for _, path := range data {
				fillInMap(ctx, destToPyData,
					path.dest, path.src.String(), ctx.ModuleName(), ctx.OtherModuleName(child))
			}
			p.depsSrcsZips = append(p.depsSrcsZips, dep.GetSrcsZip())
		}
		return true
	})
}

func fillInMap(ctx android.ModuleContext, m map[string]string,
	key, value, curModule, otherModule string) bool {
	if oldValue, found := m[key]; found {
		ctx.ModuleErrorf("found two files to be placed at the same location within zip %q."+
			" First file: in module %s at path %q."+
			" Second file: in module %s at path %q.",
			key, curModule, oldValue, otherModule, value)
		return false
	} else {
		m[key] = value
	}

	return true
}

func (p *Module) InstallInData() bool {
	return true
}

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
var String = proptools.String
