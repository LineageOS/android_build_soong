// Copyright 2015 Google Inc. All rights reserved.
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

package android

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/pathtools"
)

var absSrcDir string

// PathContext is the subset of a (Module|Singleton)Context required by the
// Path methods.
type PathContext interface {
	Config() Config
	AddNinjaFileDeps(deps ...string)
}

type PathGlobContext interface {
	PathContext
	GlobWithDeps(globPattern string, excludes []string) ([]string, error)
}

var _ PathContext = SingletonContext(nil)
var _ PathContext = ModuleContext(nil)

// "Null" path context is a minimal path context for a given config.
type NullPathContext struct {
	config Config
}

func (NullPathContext) AddNinjaFileDeps(...string) {}
func (ctx NullPathContext) Config() Config         { return ctx.config }

// EarlyModulePathContext is a subset of EarlyModuleContext methods required by the
// Path methods. These path methods can be called before any mutators have run.
type EarlyModulePathContext interface {
	PathGlobContext

	ModuleDir() string
	ModuleErrorf(fmt string, args ...interface{})
	OtherModulePropertyErrorf(module Module, property, fmt string, args ...interface{})
}

var _ EarlyModulePathContext = ModuleContext(nil)

// Glob globs files and directories matching globPattern relative to ModuleDir(),
// paths in the excludes parameter will be omitted.
func Glob(ctx EarlyModulePathContext, globPattern string, excludes []string) Paths {
	ret, err := ctx.GlobWithDeps(globPattern, excludes)
	if err != nil {
		ctx.ModuleErrorf("glob: %s", err.Error())
	}
	return pathsForModuleSrcFromFullPath(ctx, ret, true)
}

// GlobFiles globs *only* files (not directories) matching globPattern relative to ModuleDir().
// Paths in the excludes parameter will be omitted.
func GlobFiles(ctx EarlyModulePathContext, globPattern string, excludes []string) Paths {
	ret, err := ctx.GlobWithDeps(globPattern, excludes)
	if err != nil {
		ctx.ModuleErrorf("glob: %s", err.Error())
	}
	return pathsForModuleSrcFromFullPath(ctx, ret, false)
}

// ModuleWithDepsPathContext is a subset of *ModuleContext methods required by
// the Path methods that rely on module dependencies having been resolved.
type ModuleWithDepsPathContext interface {
	EarlyModulePathContext
	VisitDirectDepsBlueprint(visit func(blueprint.Module))
	OtherModuleDependencyTag(m blueprint.Module) blueprint.DependencyTag
}

// ModuleMissingDepsPathContext is a subset of *ModuleContext methods required by
// the Path methods that rely on module dependencies having been resolved and ability to report
// missing dependency errors.
type ModuleMissingDepsPathContext interface {
	ModuleWithDepsPathContext
	AddMissingDependencies(missingDeps []string)
}

type ModuleInstallPathContext interface {
	BaseModuleContext

	InstallInData() bool
	InstallInTestcases() bool
	InstallInSanitizerDir() bool
	InstallInRamdisk() bool
	InstallInVendorRamdisk() bool
	InstallInDebugRamdisk() bool
	InstallInRecovery() bool
	InstallInRoot() bool
	InstallInOdm() bool
	InstallInProduct() bool
	InstallInVendor() bool
	InstallForceOS() (*OsType, *ArchType)
}

var _ ModuleInstallPathContext = ModuleContext(nil)

type baseModuleContextToModuleInstallPathContext struct {
	BaseModuleContext
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInData() bool {
	return ctx.Module().InstallInData()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInTestcases() bool {
	return ctx.Module().InstallInTestcases()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInSanitizerDir() bool {
	return ctx.Module().InstallInSanitizerDir()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInRamdisk() bool {
	return ctx.Module().InstallInRamdisk()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInVendorRamdisk() bool {
	return ctx.Module().InstallInVendorRamdisk()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInDebugRamdisk() bool {
	return ctx.Module().InstallInDebugRamdisk()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInRecovery() bool {
	return ctx.Module().InstallInRecovery()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInRoot() bool {
	return ctx.Module().InstallInRoot()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInOdm() bool {
	return ctx.Module().InstallInOdm()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInProduct() bool {
	return ctx.Module().InstallInProduct()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallInVendor() bool {
	return ctx.Module().InstallInVendor()
}

func (ctx *baseModuleContextToModuleInstallPathContext) InstallForceOS() (*OsType, *ArchType) {
	return ctx.Module().InstallForceOS()
}

var _ ModuleInstallPathContext = (*baseModuleContextToModuleInstallPathContext)(nil)

// errorfContext is the interface containing the Errorf method matching the
// Errorf method in blueprint.SingletonContext.
type errorfContext interface {
	Errorf(format string, args ...interface{})
}

var _ errorfContext = blueprint.SingletonContext(nil)

// ModuleErrorfContext is the interface containing the ModuleErrorf method matching
// the ModuleErrorf method in blueprint.ModuleContext.
type ModuleErrorfContext interface {
	ModuleErrorf(format string, args ...interface{})
}

var _ ModuleErrorfContext = blueprint.ModuleContext(nil)

// reportPathError will register an error with the attached context. It
// attempts ctx.ModuleErrorf for a better error message first, then falls
// back to ctx.Errorf.
func reportPathError(ctx PathContext, err error) {
	ReportPathErrorf(ctx, "%s", err.Error())
}

// ReportPathErrorf will register an error with the attached context. It
// attempts ctx.ModuleErrorf for a better error message first, then falls
// back to ctx.Errorf.
func ReportPathErrorf(ctx PathContext, format string, args ...interface{}) {
	if mctx, ok := ctx.(ModuleErrorfContext); ok {
		mctx.ModuleErrorf(format, args...)
	} else if ectx, ok := ctx.(errorfContext); ok {
		ectx.Errorf(format, args...)
	} else {
		panic(fmt.Sprintf(format, args...))
	}
}

func pathContextName(ctx PathContext, module blueprint.Module) string {
	if x, ok := ctx.(interface{ ModuleName(blueprint.Module) string }); ok {
		return x.ModuleName(module)
	} else if x, ok := ctx.(interface{ OtherModuleName(blueprint.Module) string }); ok {
		return x.OtherModuleName(module)
	}
	return "unknown"
}

type Path interface {
	// Returns the path in string form
	String() string

	// Ext returns the extension of the last element of the path
	Ext() string

	// Base returns the last element of the path
	Base() string

	// Rel returns the portion of the path relative to the directory it was created from.  For
	// example, Rel on a PathsForModuleSrc would return the path relative to the module source
	// directory, and OutputPath.Join("foo").Rel() would return "foo".
	Rel() string

	// RelativeToTop returns a new path relative to the top, it is provided solely for use in tests.
	//
	// It is guaranteed to always return the same type as it is called on, e.g. if called on an
	// InstallPath then the returned value can be converted to an InstallPath.
	//
	// A standard build has the following structure:
	//   ../top/
	//          out/ - make install files go here.
	//          out/soong - this is the soongOutDir passed to NewTestConfig()
	//          ... - the source files
	//
	// This function converts a path so that it appears relative to the ../top/ directory, i.e.
	// * Make install paths, which have the pattern "soongOutDir/../<path>" are converted into the top
	//   relative path "out/<path>"
	// * Soong install paths and other writable paths, which have the pattern "soongOutDir/<path>" are
	//   converted into the top relative path "out/soong/<path>".
	// * Source paths are already relative to the top.
	// * Phony paths are not relative to anything.
	// * toolDepPath have an absolute but known value in so don't need making relative to anything in
	//   order to test.
	RelativeToTop() Path
}

const (
	OutDir      = "out"
	OutSoongDir = OutDir + "/soong"
)

// WritablePath is a type of path that can be used as an output for build rules.
type WritablePath interface {
	Path

	// return the path to the build directory.
	getSoongOutDir() string

	// the writablePath method doesn't directly do anything,
	// but it allows a struct to distinguish between whether or not it implements the WritablePath interface
	writablePath()

	ReplaceExtension(ctx PathContext, ext string) OutputPath
}

type genPathProvider interface {
	genPathWithExt(ctx ModuleOutPathContext, subdir, ext string) ModuleGenPath
	genPathWithExtAndTrimExt(ctx ModuleOutPathContext, subdir, ext string, trimExt string) ModuleGenPath
}
type objPathProvider interface {
	objPathWithExt(ctx ModuleOutPathContext, subdir, ext string) ModuleObjPath
}
type resPathProvider interface {
	resPathWithName(ctx ModuleOutPathContext, name string) ModuleResPath
}

// GenPathWithExt derives a new file path in ctx's generated sources directory
// from the current path, but with the new extension.
func GenPathWithExt(ctx ModuleOutPathContext, subdir string, p Path, ext string) ModuleGenPath {
	if path, ok := p.(genPathProvider); ok {
		return path.genPathWithExt(ctx, subdir, ext)
	}
	ReportPathErrorf(ctx, "Tried to create generated file from unsupported path: %s(%s)", reflect.TypeOf(p).Name(), p)
	return PathForModuleGen(ctx)
}

// GenPathWithExtAndTrimExt derives a new file path in ctx's generated sources directory
// from the current path, but with the new extension and trim the suffix.
func GenPathWithExtAndTrimExt(ctx ModuleOutPathContext, subdir string, p Path, ext string, trimExt string) ModuleGenPath {
	if path, ok := p.(genPathProvider); ok {
		return path.genPathWithExtAndTrimExt(ctx, subdir, ext, trimExt)
	}
	ReportPathErrorf(ctx, "Tried to create generated file from unsupported path: %s(%s)", reflect.TypeOf(p).Name(), p)
	return PathForModuleGen(ctx)
}

// ObjPathWithExt derives a new file path in ctx's object directory from the
// current path, but with the new extension.
func ObjPathWithExt(ctx ModuleOutPathContext, subdir string, p Path, ext string) ModuleObjPath {
	if path, ok := p.(objPathProvider); ok {
		return path.objPathWithExt(ctx, subdir, ext)
	}
	ReportPathErrorf(ctx, "Tried to create object file from unsupported path: %s (%s)", reflect.TypeOf(p).Name(), p)
	return PathForModuleObj(ctx)
}

// ResPathWithName derives a new path in ctx's output resource directory, using
// the current path to create the directory name, and the `name` argument for
// the filename.
func ResPathWithName(ctx ModuleOutPathContext, p Path, name string) ModuleResPath {
	if path, ok := p.(resPathProvider); ok {
		return path.resPathWithName(ctx, name)
	}
	ReportPathErrorf(ctx, "Tried to create res file from unsupported path: %s (%s)", reflect.TypeOf(p).Name(), p)
	return PathForModuleRes(ctx)
}

// OptionalPath is a container that may or may not contain a valid Path.
type OptionalPath struct {
	path          Path   // nil if invalid.
	invalidReason string // Not applicable if path != nil. "" if the reason is unknown.
}

// OptionalPathForPath returns an OptionalPath containing the path.
func OptionalPathForPath(path Path) OptionalPath {
	return OptionalPath{path: path}
}

// InvalidOptionalPath returns an OptionalPath that is invalid with the given reason.
func InvalidOptionalPath(reason string) OptionalPath {

	return OptionalPath{invalidReason: reason}
}

// Valid returns whether there is a valid path
func (p OptionalPath) Valid() bool {
	return p.path != nil
}

// Path returns the Path embedded in this OptionalPath. You must be sure that
// there is a valid path, since this method will panic if there is not.
func (p OptionalPath) Path() Path {
	if p.path == nil {
		msg := "Requesting an invalid path"
		if p.invalidReason != "" {
			msg += ": " + p.invalidReason
		}
		panic(msg)
	}
	return p.path
}

// InvalidReason returns the reason that the optional path is invalid, or "" if it is valid.
func (p OptionalPath) InvalidReason() string {
	if p.path != nil {
		return ""
	}
	if p.invalidReason == "" {
		return "unknown"
	}
	return p.invalidReason
}

// AsPaths converts the OptionalPath into Paths.
//
// It returns nil if this is not valid, or a single length slice containing the Path embedded in
// this OptionalPath.
func (p OptionalPath) AsPaths() Paths {
	if p.path == nil {
		return nil
	}
	return Paths{p.path}
}

// RelativeToTop returns an OptionalPath with the path that was embedded having been replaced by the
// result of calling Path.RelativeToTop on it.
func (p OptionalPath) RelativeToTop() OptionalPath {
	if p.path == nil {
		return p
	}
	p.path = p.path.RelativeToTop()
	return p
}

// String returns the string version of the Path, or "" if it isn't valid.
func (p OptionalPath) String() string {
	if p.path != nil {
		return p.path.String()
	} else {
		return ""
	}
}

// Paths is a slice of Path objects, with helpers to operate on the collection.
type Paths []Path

// RelativeToTop creates a new Paths containing the result of calling Path.RelativeToTop on each
// item in this slice.
func (p Paths) RelativeToTop() Paths {
	ensureTestOnly()
	if p == nil {
		return p
	}
	ret := make(Paths, len(p))
	for i, path := range p {
		ret[i] = path.RelativeToTop()
	}
	return ret
}

func (paths Paths) containsPath(path Path) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

// PathsForSource returns Paths rooted from SrcDir, *not* rooted from the module's local source
// directory
func PathsForSource(ctx PathContext, paths []string) Paths {
	ret := make(Paths, len(paths))
	for i, path := range paths {
		ret[i] = PathForSource(ctx, path)
	}
	return ret
}

// ExistentPathsForSources returns a list of Paths rooted from SrcDir, *not* rooted from the
// module's local source directory, that are found in the tree. If any are not found, they are
// omitted from the list, and dependencies are added so that we're re-run when they are added.
func ExistentPathsForSources(ctx PathGlobContext, paths []string) Paths {
	ret := make(Paths, 0, len(paths))
	for _, path := range paths {
		p := ExistentPathForSource(ctx, path)
		if p.Valid() {
			ret = append(ret, p.Path())
		}
	}
	return ret
}

// PathsForModuleSrc returns a Paths{} containing the resolved references in paths:
//   - filepath, relative to local module directory, resolves as a filepath relative to the local
//     source directory
//   - glob, relative to the local module directory, resolves as filepath(s), relative to the local
//     source directory.
//   - other modules using the ":name{.tag}" syntax. These modules must implement SourceFileProducer
//     or OutputFileProducer. These resolve as a filepath to an output filepath or generated source
//     filepath.
//
// Properties passed as the paths argument must have been annotated with struct tag
// `android:"path"` so that dependencies on SourceFileProducer modules will have already been handled by the
// pathdeps mutator.
// If a requested module is not found as a dependency:
//   - if ctx.Config().AllowMissingDependencies() is true, this module to be marked as having
//     missing dependencies
//   - otherwise, a ModuleError is thrown.
func PathsForModuleSrc(ctx ModuleMissingDepsPathContext, paths []string) Paths {
	return PathsForModuleSrcExcludes(ctx, paths, nil)
}

type SourceInput struct {
	Context      ModuleMissingDepsPathContext
	Paths        []string
	ExcludePaths []string
	IncludeDirs  bool
}

// PathsForModuleSrcExcludes returns a Paths{} containing the resolved references in paths, minus
// those listed in excludes. Elements of paths and excludes are resolved as:
//   - filepath, relative to local module directory, resolves as a filepath relative to the local
//     source directory
//   - glob, relative to the local module directory, resolves as filepath(s), relative to the local
//     source directory. Not valid in excludes.
//   - other modules using the ":name{.tag}" syntax. These modules must implement SourceFileProducer
//     or OutputFileProducer. These resolve as a filepath to an output filepath or generated source
//     filepath.
//
// excluding the items (similarly resolved
// Properties passed as the paths argument must have been annotated with struct tag
// `android:"path"` so that dependencies on SourceFileProducer modules will have already been handled by the
// pathdeps mutator.
// If a requested module is not found as a dependency:
//   - if ctx.Config().AllowMissingDependencies() is true, this module to be marked as having
//     missing dependencies
//   - otherwise, a ModuleError is thrown.
func PathsForModuleSrcExcludes(ctx ModuleMissingDepsPathContext, paths, excludes []string) Paths {
	return PathsRelativeToModuleSourceDir(SourceInput{
		Context:      ctx,
		Paths:        paths,
		ExcludePaths: excludes,
		IncludeDirs:  true,
	})
}

func PathsRelativeToModuleSourceDir(input SourceInput) Paths {
	ret, missingDeps := PathsAndMissingDepsRelativeToModuleSourceDir(input)
	if input.Context.Config().AllowMissingDependencies() {
		input.Context.AddMissingDependencies(missingDeps)
	} else {
		for _, m := range missingDeps {
			input.Context.ModuleErrorf(`missing dependency on %q, is the property annotated with android:"path"?`, m)
		}
	}
	return ret
}

// OutputPaths is a slice of OutputPath objects, with helpers to operate on the collection.
type OutputPaths []OutputPath

// Paths returns the OutputPaths as a Paths
func (p OutputPaths) Paths() Paths {
	if p == nil {
		return nil
	}
	ret := make(Paths, len(p))
	for i, path := range p {
		ret[i] = path
	}
	return ret
}

// Strings returns the string forms of the writable paths.
func (p OutputPaths) Strings() []string {
	if p == nil {
		return nil
	}
	ret := make([]string, len(p))
	for i, path := range p {
		ret[i] = path.String()
	}
	return ret
}

// PathForGoBinary returns the path to the installed location of a bootstrap_go_binary module.
func PathForGoBinary(ctx PathContext, goBinary bootstrap.GoBinaryTool) Path {
	goBinaryInstallDir := pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, "bin")
	rel := Rel(ctx, goBinaryInstallDir.String(), goBinary.InstallPath())
	return goBinaryInstallDir.Join(ctx, rel)
}

// Expands Paths to a SourceFileProducer or OutputFileProducer module dependency referenced via ":name" or ":name{.tag}" syntax.
// If the dependency is not found, a missingErrorDependency is returned.
// If the module dependency is not a SourceFileProducer or OutputFileProducer, appropriate errors will be returned.
func getPathsFromModuleDep(ctx ModuleWithDepsPathContext, path, moduleName, tag string) (Paths, error) {
	module := GetModuleFromPathDep(ctx, moduleName, tag)
	if module == nil {
		return nil, missingDependencyError{[]string{moduleName}}
	}
	if aModule, ok := module.(Module); ok && !aModule.Enabled(ctx) {
		return nil, missingDependencyError{[]string{moduleName}}
	}
	if goBinary, ok := module.(bootstrap.GoBinaryTool); ok && tag == "" {
		goBinaryPath := PathForGoBinary(ctx, goBinary)
		return Paths{goBinaryPath}, nil
	}
	outputFiles, err := outputFilesForModule(ctx, module, tag)
	if outputFiles != nil && err == nil {
		return outputFiles, nil
	} else {
		return nil, err
	}
}

// GetModuleFromPathDep will return the module that was added as a dependency automatically for
// properties tagged with `android:"path"` or manually using ExtractSourceDeps or
// ExtractSourcesDeps.
//
// The moduleName and tag supplied to this should be the values returned from SrcIsModuleWithTag.
// Or, if no tag is expected then the moduleName should be the value returned by  SrcIsModule and
// the tag must be "".
//
// If tag is "" then the returned module will be the dependency that was added for ":moduleName".
// Otherwise, it is the dependency that was added for ":moduleName{tag}".
func GetModuleFromPathDep(ctx ModuleWithDepsPathContext, moduleName, tag string) blueprint.Module {
	var found blueprint.Module
	// The sourceOrOutputDepTag uniquely identifies the module dependency as it contains both the
	// module name and the tag. Dependencies added automatically for properties tagged with
	// `android:"path"` are deduped so are guaranteed to be unique. It is possible for duplicate
	// dependencies to be added manually using ExtractSourcesDeps or ExtractSourceDeps but even then
	// it will always be the case that the dependencies will be identical, i.e. the same tag and same
	// moduleName referring to the same dependency module.
	//
	// It does not matter whether the moduleName is a fully qualified name or if the module
	// dependency is a prebuilt module. All that matters is the same information is supplied to
	// create the tag here as was supplied to create the tag when the dependency was added so that
	// this finds the matching dependency module.
	expectedTag := sourceOrOutputDepTag(moduleName, tag)
	ctx.VisitDirectDepsBlueprint(func(module blueprint.Module) {
		depTag := ctx.OtherModuleDependencyTag(module)
		if depTag == expectedTag {
			found = module
		}
	})
	return found
}

// PathsAndMissingDepsForModuleSrcExcludes returns a Paths{} containing the resolved references in
// paths, minus those listed in excludes. Elements of paths and excludes are resolved as:
//   - filepath, relative to local module directory, resolves as a filepath relative to the local
//     source directory
//   - glob, relative to the local module directory, resolves as filepath(s), relative to the local
//     source directory. Not valid in excludes.
//   - other modules using the ":name{.tag}" syntax. These modules must implement SourceFileProducer
//     or OutputFileProducer. These resolve as a filepath to an output filepath or generated source
//     filepath.
//
// and a list of the module names of missing module dependencies are returned as the second return.
// Properties passed as the paths argument must have been annotated with struct tag
// `android:"path"` so that dependencies on SourceFileProducer modules will have already been handled by the
// pathdeps mutator.
func PathsAndMissingDepsForModuleSrcExcludes(ctx ModuleMissingDepsPathContext, paths, excludes []string) (Paths, []string) {
	return PathsAndMissingDepsRelativeToModuleSourceDir(SourceInput{
		Context:      ctx,
		Paths:        paths,
		ExcludePaths: excludes,
		IncludeDirs:  true,
	})
}

func PathsAndMissingDepsRelativeToModuleSourceDir(input SourceInput) (Paths, []string) {
	prefix := pathForModuleSrc(input.Context).String()

	var expandedExcludes []string
	if input.ExcludePaths != nil {
		expandedExcludes = make([]string, 0, len(input.ExcludePaths))
	}

	var missingExcludeDeps []string
	for _, e := range input.ExcludePaths {
		if m, t := SrcIsModuleWithTag(e); m != "" {
			modulePaths, err := getPathsFromModuleDep(input.Context, e, m, t)
			if m, ok := err.(missingDependencyError); ok {
				missingExcludeDeps = append(missingExcludeDeps, m.missingDeps...)
			} else if err != nil {
				reportPathError(input.Context, err)
			} else {
				expandedExcludes = append(expandedExcludes, modulePaths.Strings()...)
			}
		} else {
			expandedExcludes = append(expandedExcludes, filepath.Join(prefix, e))
		}
	}

	if input.Paths == nil {
		return nil, missingExcludeDeps
	}

	var missingDeps []string

	expandedSrcFiles := make(Paths, 0, len(input.Paths))
	for _, s := range input.Paths {
		srcFiles, err := expandOneSrcPath(sourcePathInput{
			context:          input.Context,
			path:             s,
			expandedExcludes: expandedExcludes,
			includeDirs:      input.IncludeDirs,
		})
		if depErr, ok := err.(missingDependencyError); ok {
			missingDeps = append(missingDeps, depErr.missingDeps...)
		} else if err != nil {
			reportPathError(input.Context, err)
		}
		expandedSrcFiles = append(expandedSrcFiles, srcFiles...)
	}

	// TODO: b/334169722 - Replace with an error instead of implicitly removing duplicates.
	return FirstUniquePaths(expandedSrcFiles), append(missingDeps, missingExcludeDeps...)
}

type missingDependencyError struct {
	missingDeps []string
}

func (e missingDependencyError) Error() string {
	return "missing dependencies: " + strings.Join(e.missingDeps, ", ")
}

type sourcePathInput struct {
	context          ModuleWithDepsPathContext
	path             string
	expandedExcludes []string
	includeDirs      bool
}

// Expands one path string to Paths rooted from the module's local source
// directory, excluding those listed in the expandedExcludes.
// Expands globs, references to SourceFileProducer or OutputFileProducer modules using the ":name" and ":name{.tag}" syntax.
func expandOneSrcPath(input sourcePathInput) (Paths, error) {
	excludePaths := func(paths Paths) Paths {
		if len(input.expandedExcludes) == 0 {
			return paths
		}
		remainder := make(Paths, 0, len(paths))
		for _, p := range paths {
			if !InList(p.String(), input.expandedExcludes) {
				remainder = append(remainder, p)
			}
		}
		return remainder
	}
	if m, t := SrcIsModuleWithTag(input.path); m != "" {
		modulePaths, err := getPathsFromModuleDep(input.context, input.path, m, t)
		if err != nil {
			return nil, err
		} else {
			return excludePaths(modulePaths), nil
		}
	} else {
		p := pathForModuleSrc(input.context, input.path)
		if pathtools.IsGlob(input.path) {
			paths := GlobFiles(input.context, p.String(), input.expandedExcludes)
			return PathsWithModuleSrcSubDir(input.context, paths, ""), nil
		} else {
			if exists, _, err := input.context.Config().fs.Exists(p.String()); err != nil {
				ReportPathErrorf(input.context, "%s: %s", p, err.Error())
			} else if !exists && !input.context.Config().TestAllowNonExistentPaths {
				ReportPathErrorf(input.context, "module source path %q does not exist", p)
			} else if !input.includeDirs {
				if isDir, err := input.context.Config().fs.IsDir(p.String()); exists && err != nil {
					ReportPathErrorf(input.context, "%s: %s", p, err.Error())
				} else if isDir {
					ReportPathErrorf(input.context, "module source path %q is a directory", p)
				}
			}

			if InList(p.String(), input.expandedExcludes) {
				return nil, nil
			}
			return Paths{p}, nil
		}
	}
}

// pathsForModuleSrcFromFullPath returns Paths rooted from the module's local
// source directory, but strip the local source directory from the beginning of
// each string. If incDirs is false, strip paths with a trailing '/' from the list.
// It intended for use in globs that only list files that exist, so it allows '$' in
// filenames.
func pathsForModuleSrcFromFullPath(ctx EarlyModulePathContext, paths []string, incDirs bool) Paths {
	prefix := ctx.ModuleDir() + "/"
	if prefix == "./" {
		prefix = ""
	}
	ret := make(Paths, 0, len(paths))
	for _, p := range paths {
		if !incDirs && strings.HasSuffix(p, "/") {
			continue
		}
		path := filepath.Clean(p)
		if !strings.HasPrefix(path, prefix) {
			ReportPathErrorf(ctx, "Path %q is not in module source directory %q", p, prefix)
			continue
		}

		srcPath, err := safePathForSource(ctx, ctx.ModuleDir(), path[len(prefix):])
		if err != nil {
			reportPathError(ctx, err)
			continue
		}

		srcPath.basePath.rel = srcPath.path

		ret = append(ret, srcPath)
	}
	return ret
}

// PathsWithOptionalDefaultForModuleSrc returns Paths rooted from the module's local source
// directory. If input is nil, use the default if it exists.  If input is empty, returns nil.
func PathsWithOptionalDefaultForModuleSrc(ctx ModuleMissingDepsPathContext, input []string, def string) Paths {
	if input != nil {
		return PathsForModuleSrc(ctx, input)
	}
	// Use Glob so that if the default doesn't exist, a dependency is added so that when it
	// is created, we're run again.
	path := filepath.Join(ctx.ModuleDir(), def)
	return Glob(ctx, path, nil)
}

// Strings returns the Paths in string form
func (p Paths) Strings() []string {
	if p == nil {
		return nil
	}
	ret := make([]string, len(p))
	for i, path := range p {
		ret[i] = path.String()
	}
	return ret
}

func CopyOfPaths(paths Paths) Paths {
	return append(Paths(nil), paths...)
}

// FirstUniquePaths returns all unique elements of a Paths, keeping the first copy of each.  It
// modifies the Paths slice contents in place, and returns a subslice of the original slice.
func FirstUniquePaths(list Paths) Paths {
	// 128 was chosen based on BenchmarkFirstUniquePaths results.
	if len(list) > 128 {
		return firstUniquePathsMap(list)
	}
	return firstUniquePathsList(list)
}

// SortedUniquePaths returns all unique elements of a Paths in sorted order.  It modifies the
// Paths slice contents in place, and returns a subslice of the original slice.
func SortedUniquePaths(list Paths) Paths {
	unique := FirstUniquePaths(list)
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].String() < unique[j].String()
	})
	return unique
}

func firstUniquePathsList(list Paths) Paths {
	k := 0
outer:
	for i := 0; i < len(list); i++ {
		for j := 0; j < k; j++ {
			if list[i] == list[j] {
				continue outer
			}
		}
		list[k] = list[i]
		k++
	}
	return list[:k]
}

func firstUniquePathsMap(list Paths) Paths {
	k := 0
	seen := make(map[Path]bool, len(list))
	for i := 0; i < len(list); i++ {
		if seen[list[i]] {
			continue
		}
		seen[list[i]] = true
		list[k] = list[i]
		k++
	}
	return list[:k]
}

// FirstUniqueInstallPaths returns all unique elements of an InstallPaths, keeping the first copy of each.  It
// modifies the InstallPaths slice contents in place, and returns a subslice of the original slice.
func FirstUniqueInstallPaths(list InstallPaths) InstallPaths {
	// 128 was chosen based on BenchmarkFirstUniquePaths results.
	if len(list) > 128 {
		return firstUniqueInstallPathsMap(list)
	}
	return firstUniqueInstallPathsList(list)
}

func firstUniqueInstallPathsList(list InstallPaths) InstallPaths {
	k := 0
outer:
	for i := 0; i < len(list); i++ {
		for j := 0; j < k; j++ {
			if list[i] == list[j] {
				continue outer
			}
		}
		list[k] = list[i]
		k++
	}
	return list[:k]
}

func firstUniqueInstallPathsMap(list InstallPaths) InstallPaths {
	k := 0
	seen := make(map[InstallPath]bool, len(list))
	for i := 0; i < len(list); i++ {
		if seen[list[i]] {
			continue
		}
		seen[list[i]] = true
		list[k] = list[i]
		k++
	}
	return list[:k]
}

// LastUniquePaths returns all unique elements of a Paths, keeping the last copy of each.  It
// modifies the Paths slice contents in place, and returns a subslice of the original slice.
func LastUniquePaths(list Paths) Paths {
	totalSkip := 0
	for i := len(list) - 1; i >= totalSkip; i-- {
		skip := 0
		for j := i - 1; j >= totalSkip; j-- {
			if list[i] == list[j] {
				skip++
			} else {
				list[j+skip] = list[j]
			}
		}
		totalSkip += skip
	}
	return list[totalSkip:]
}

// ReversePaths returns a copy of a Paths in reverse order.
func ReversePaths(list Paths) Paths {
	if list == nil {
		return nil
	}
	ret := make(Paths, len(list))
	for i := range list {
		ret[i] = list[len(list)-1-i]
	}
	return ret
}

func indexPathList(s Path, list []Path) int {
	for i, l := range list {
		if l == s {
			return i
		}
	}

	return -1
}

func inPathList(p Path, list []Path) bool {
	return indexPathList(p, list) != -1
}

func FilterPathList(list []Path, filter []Path) (remainder []Path, filtered []Path) {
	return FilterPathListPredicate(list, func(p Path) bool { return inPathList(p, filter) })
}

func FilterPathListPredicate(list []Path, predicate func(Path) bool) (remainder []Path, filtered []Path) {
	for _, l := range list {
		if predicate(l) {
			filtered = append(filtered, l)
		} else {
			remainder = append(remainder, l)
		}
	}

	return
}

// HasExt returns true of any of the paths have extension ext, otherwise false
func (p Paths) HasExt(ext string) bool {
	for _, path := range p {
		if path.Ext() == ext {
			return true
		}
	}

	return false
}

// FilterByExt returns the subset of the paths that have extension ext
func (p Paths) FilterByExt(ext string) Paths {
	ret := make(Paths, 0, len(p))
	for _, path := range p {
		if path.Ext() == ext {
			ret = append(ret, path)
		}
	}
	return ret
}

// FilterOutByExt returns the subset of the paths that do not have extension ext
func (p Paths) FilterOutByExt(ext string) Paths {
	ret := make(Paths, 0, len(p))
	for _, path := range p {
		if path.Ext() != ext {
			ret = append(ret, path)
		}
	}
	return ret
}

// DirectorySortedPaths is a slice of paths that are sorted such that all files in a directory
// (including subdirectories) are in a contiguous subslice of the list, and can be found in
// O(log(N)) time using a binary search on the directory prefix.
type DirectorySortedPaths Paths

func PathsToDirectorySortedPaths(paths Paths) DirectorySortedPaths {
	ret := append(DirectorySortedPaths(nil), paths...)
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].String() < ret[j].String()
	})
	return ret
}

// PathsInDirectory returns a subslice of the DirectorySortedPaths as a Paths that contains all entries
// that are in the specified directory and its subdirectories.
func (p DirectorySortedPaths) PathsInDirectory(dir string) Paths {
	prefix := filepath.Clean(dir) + "/"
	start := sort.Search(len(p), func(i int) bool {
		return prefix < p[i].String()
	})

	ret := p[start:]

	end := sort.Search(len(ret), func(i int) bool {
		return !strings.HasPrefix(ret[i].String(), prefix)
	})

	ret = ret[:end]

	return Paths(ret)
}

// WritablePaths is a slice of WritablePath, used for multiple outputs.
type WritablePaths []WritablePath

// RelativeToTop creates a new WritablePaths containing the result of calling Path.RelativeToTop on
// each item in this slice.
func (p WritablePaths) RelativeToTop() WritablePaths {
	ensureTestOnly()
	if p == nil {
		return p
	}
	ret := make(WritablePaths, len(p))
	for i, path := range p {
		ret[i] = path.RelativeToTop().(WritablePath)
	}
	return ret
}

// Strings returns the string forms of the writable paths.
func (p WritablePaths) Strings() []string {
	if p == nil {
		return nil
	}
	ret := make([]string, len(p))
	for i, path := range p {
		ret[i] = path.String()
	}
	return ret
}

// Paths returns the WritablePaths as a Paths
func (p WritablePaths) Paths() Paths {
	if p == nil {
		return nil
	}
	ret := make(Paths, len(p))
	for i, path := range p {
		ret[i] = path
	}
	return ret
}

type basePath struct {
	path string
	rel  string
}

func (p basePath) Ext() string {
	return filepath.Ext(p.path)
}

func (p basePath) Base() string {
	return filepath.Base(p.path)
}

func (p basePath) Rel() string {
	if p.rel != "" {
		return p.rel
	}
	return p.path
}

func (p basePath) String() string {
	return p.path
}

func (p basePath) withRel(rel string) basePath {
	p.path = filepath.Join(p.path, rel)
	p.rel = rel
	return p
}

func (p basePath) RelativeToTop() Path {
	ensureTestOnly()
	return p
}

// SourcePath is a Path representing a file path rooted from SrcDir
type SourcePath struct {
	basePath
}

var _ Path = SourcePath{}

func (p SourcePath) withRel(rel string) SourcePath {
	p.basePath = p.basePath.withRel(rel)
	return p
}

// safePathForSource is for paths that we expect are safe -- only for use by go
// code that is embedding ninja variables in paths
func safePathForSource(ctx PathContext, pathComponents ...string) (SourcePath, error) {
	p, err := validateSafePath(pathComponents...)
	ret := SourcePath{basePath{p, ""}}
	if err != nil {
		return ret, err
	}

	// absolute path already checked by validateSafePath
	// special-case api surface gen files for now
	if strings.HasPrefix(ret.String(), ctx.Config().soongOutDir) && !strings.Contains(ret.String(), ctx.Config().soongOutDir+"/.export") {
		return ret, fmt.Errorf("source path %q is in output", ret.String())
	}

	return ret, err
}

// pathForSource creates a SourcePath from pathComponents, but does not check that it exists.
func pathForSource(ctx PathContext, pathComponents ...string) (SourcePath, error) {
	p, err := validatePath(pathComponents...)
	ret := SourcePath{basePath{p, ""}}
	if err != nil {
		return ret, err
	}

	// absolute path already checked by validatePath
	// special-case for now
	if strings.HasPrefix(ret.String(), ctx.Config().soongOutDir) && !strings.Contains(ret.String(), ctx.Config().soongOutDir+"/.export") {
		return ret, fmt.Errorf("source path %q is in output", ret.String())
	}

	return ret, nil
}

// pathForSourceRelaxed creates a SourcePath from pathComponents, but does not check that it exists.
// It differs from pathForSource in that the path is allowed to exist outside of the PathContext.
func pathForSourceRelaxed(ctx PathContext, pathComponents ...string) (SourcePath, error) {
	p := filepath.Join(pathComponents...)
	ret := SourcePath{basePath{p, ""}}

	abs, err := filepath.Abs(ret.String())
	if err != nil {
		return ret, err
	}
	buildroot, err := filepath.Abs(ctx.Config().soongOutDir)
	if err != nil {
		return ret, err
	}
	if strings.HasPrefix(abs, buildroot) {
		return ret, fmt.Errorf("source path %s is in output", abs)
	}

	if pathtools.IsGlob(ret.String()) {
		return ret, fmt.Errorf("path may not contain a glob: %s", ret.String())
	}

	return ret, nil
}

// existsWithDependencies returns true if the path exists, and adds appropriate dependencies to rerun if the
// path does not exist.
func existsWithDependencies(ctx PathGlobContext, path SourcePath) (exists bool, err error) {
	var files []string

	// Use glob to produce proper dependencies, even though we only want
	// a single file.
	files, err = ctx.GlobWithDeps(path.String(), nil)

	if err != nil {
		return false, fmt.Errorf("glob: %s", err.Error())
	}

	return len(files) > 0, nil
}

// PathForSource joins the provided path components and validates that the result
// neither escapes the source dir nor is in the out dir.
// On error, it will return a usable, but invalid SourcePath, and report a ModuleError.
func PathForSource(ctx PathContext, pathComponents ...string) SourcePath {
	path, err := pathForSource(ctx, pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}

	if pathtools.IsGlob(path.String()) {
		ReportPathErrorf(ctx, "path may not contain a glob: %s", path.String())
	}

	if modCtx, ok := ctx.(ModuleMissingDepsPathContext); ok && ctx.Config().AllowMissingDependencies() {
		exists, err := existsWithDependencies(modCtx, path)
		if err != nil {
			reportPathError(ctx, err)
		}
		if !exists {
			modCtx.AddMissingDependencies([]string{path.String()})
		}
	} else if exists, _, err := ctx.Config().fs.Exists(path.String()); err != nil {
		ReportPathErrorf(ctx, "%s: %s", path, err.Error())
	} else if !exists && !ctx.Config().TestAllowNonExistentPaths {
		ReportPathErrorf(ctx, "source path %q does not exist", path)
	}
	return path
}

// PathForSourceRelaxed joins the provided path components.  Unlike PathForSource,
// the result is allowed to exist outside of the source dir.
// On error, it will return a usable, but invalid SourcePath, and report a ModuleError.
func PathForSourceRelaxed(ctx PathContext, pathComponents ...string) SourcePath {
	path, err := pathForSourceRelaxed(ctx, pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}

	if modCtx, ok := ctx.(ModuleContext); ok && ctx.Config().AllowMissingDependencies() {
		exists, err := existsWithDependencies(modCtx, path)
		if err != nil {
			reportPathError(ctx, err)
		}
		if !exists {
			modCtx.AddMissingDependencies([]string{path.String()})
		}
	} else if exists, _, err := ctx.Config().fs.Exists(path.String()); err != nil {
		ReportPathErrorf(ctx, "%s: %s", path, err.Error())
	} else if !exists {
		ReportPathErrorf(ctx, "source path %s does not exist", path)
	}
	return path
}

// PathForArbitraryOutput creates a path for the given components. Unlike PathForOutput,
// the path is relative to the root of the output folder, not the out/soong folder.
func PathForArbitraryOutput(ctx PathContext, pathComponents ...string) Path {
	p, err := validatePath(pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return basePath{path: filepath.Join(ctx.Config().OutDir(), p)}
}

// MaybeExistentPathForSource joins the provided path components and validates that the result
// neither escapes the source dir nor is in the out dir.
// It does not validate whether the path exists.
func MaybeExistentPathForSource(ctx PathContext, pathComponents ...string) SourcePath {
	path, err := pathForSource(ctx, pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}

	if pathtools.IsGlob(path.String()) {
		ReportPathErrorf(ctx, "path may not contain a glob: %s", path.String())
	}
	return path
}

// ExistentPathForSource returns an OptionalPath with the SourcePath, rooted from SrcDir, *not*
// rooted from the module's local source directory, if the path exists, or an empty OptionalPath if
// it doesn't exist. Dependencies are added so that the ninja file will be regenerated if the state
// of the path changes.
func ExistentPathForSource(ctx PathGlobContext, pathComponents ...string) OptionalPath {
	path, err := pathForSource(ctx, pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
		// No need to put the error message into the returned path since it has been reported already.
		return OptionalPath{}
	}

	if pathtools.IsGlob(path.String()) {
		ReportPathErrorf(ctx, "path may not contain a glob: %s", path.String())
		return OptionalPath{}
	}

	exists, err := existsWithDependencies(ctx, path)
	if err != nil {
		reportPathError(ctx, err)
		return OptionalPath{}
	}
	if !exists {
		return InvalidOptionalPath(path.String() + " does not exist")
	}
	return OptionalPathForPath(path)
}

func (p SourcePath) String() string {
	if p.path == "" {
		return "."
	}
	return p.path
}

// Join creates a new SourcePath with paths... joined with the current path. The
// provided paths... may not use '..' to escape from the current path.
func (p SourcePath) Join(ctx PathContext, paths ...string) SourcePath {
	path, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return p.withRel(path)
}

// join is like Join but does less path validation.
func (p SourcePath) join(ctx PathContext, paths ...string) SourcePath {
	path, err := validateSafePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return p.withRel(path)
}

// OverlayPath returns the overlay for `path' if it exists. This assumes that the
// SourcePath is the path to a resource overlay directory.
func (p SourcePath) OverlayPath(ctx ModuleMissingDepsPathContext, path Path) OptionalPath {
	var relDir string
	if srcPath, ok := path.(SourcePath); ok {
		relDir = srcPath.path
	} else {
		ReportPathErrorf(ctx, "Cannot find relative path for %s(%s)", reflect.TypeOf(path).Name(), path)
		// No need to put the error message into the returned path since it has been reported already.
		return OptionalPath{}
	}
	dir := filepath.Join(p.path, relDir)
	// Use Glob so that we are run again if the directory is added.
	if pathtools.IsGlob(dir) {
		ReportPathErrorf(ctx, "Path may not contain a glob: %s", dir)
	}
	paths, err := ctx.GlobWithDeps(dir, nil)
	if err != nil {
		ReportPathErrorf(ctx, "glob: %s", err.Error())
		return OptionalPath{}
	}
	if len(paths) == 0 {
		return InvalidOptionalPath(dir + " does not exist")
	}
	return OptionalPathForPath(PathForSource(ctx, paths[0]))
}

// OutputPath is a Path representing an intermediates file path rooted from the build directory
type OutputPath struct {
	basePath

	// The soong build directory, i.e. Config.SoongOutDir()
	soongOutDir string

	fullPath string
}

func (p OutputPath) withRel(rel string) OutputPath {
	p.basePath = p.basePath.withRel(rel)
	p.fullPath = filepath.Join(p.fullPath, rel)
	return p
}

func (p OutputPath) WithoutRel() OutputPath {
	p.basePath.rel = filepath.Base(p.basePath.path)
	return p
}

func (p OutputPath) getSoongOutDir() string {
	return p.soongOutDir
}

func (p OutputPath) RelativeToTop() Path {
	return p.outputPathRelativeToTop()
}

func (p OutputPath) outputPathRelativeToTop() OutputPath {
	p.fullPath = StringPathRelativeToTop(p.soongOutDir, p.fullPath)
	p.soongOutDir = OutSoongDir
	return p
}

func (p OutputPath) objPathWithExt(ctx ModuleOutPathContext, subdir, ext string) ModuleObjPath {
	return PathForModuleObj(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

var _ Path = OutputPath{}
var _ WritablePath = OutputPath{}
var _ objPathProvider = OutputPath{}

// toolDepPath is a Path representing a dependency of the build tool.
type toolDepPath struct {
	basePath
}

func (t toolDepPath) RelativeToTop() Path {
	ensureTestOnly()
	return t
}

var _ Path = toolDepPath{}

// pathForBuildToolDep returns a toolDepPath representing the given path string.
// There is no validation for the path, as it is "trusted": It may fail
// normal validation checks. For example, it may be an absolute path.
// Only use this function to construct paths for dependencies of the build
// tool invocation.
func pathForBuildToolDep(ctx PathContext, path string) toolDepPath {
	return toolDepPath{basePath{path, ""}}
}

// PathForOutput joins the provided paths and returns an OutputPath that is
// validated to not escape the build dir.
// On error, it will return a usable, but invalid OutputPath, and report a ModuleError.
func PathForOutput(ctx PathContext, pathComponents ...string) OutputPath {
	path, err := validatePath(pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}
	fullPath := filepath.Join(ctx.Config().soongOutDir, path)
	path = fullPath[len(fullPath)-len(path):]
	return OutputPath{basePath{path, ""}, ctx.Config().soongOutDir, fullPath}
}

// PathsForOutput returns Paths rooted from soongOutDir
func PathsForOutput(ctx PathContext, paths []string) WritablePaths {
	ret := make(WritablePaths, len(paths))
	for i, path := range paths {
		ret[i] = PathForOutput(ctx, path)
	}
	return ret
}

func (p OutputPath) writablePath() {}

func (p OutputPath) String() string {
	return p.fullPath
}

// Join creates a new OutputPath with paths... joined with the current path. The
// provided paths... may not use '..' to escape from the current path.
func (p OutputPath) Join(ctx PathContext, paths ...string) OutputPath {
	path, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return p.withRel(path)
}

// ReplaceExtension creates a new OutputPath with the extension replaced with ext.
func (p OutputPath) ReplaceExtension(ctx PathContext, ext string) OutputPath {
	if strings.Contains(ext, "/") {
		ReportPathErrorf(ctx, "extension %q cannot contain /", ext)
	}
	ret := PathForOutput(ctx, pathtools.ReplaceExtension(p.path, ext))
	ret.rel = pathtools.ReplaceExtension(p.rel, ext)
	return ret
}

// InSameDir creates a new OutputPath from the directory of the current OutputPath joined with the elements in paths.
func (p OutputPath) InSameDir(ctx PathContext, paths ...string) OutputPath {
	path, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}

	ret := PathForOutput(ctx, filepath.Dir(p.path), path)
	ret.rel = filepath.Join(filepath.Dir(p.rel), path)
	return ret
}

// PathForIntermediates returns an OutputPath representing the top-level
// intermediates directory.
func PathForIntermediates(ctx PathContext, paths ...string) OutputPath {
	path, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return PathForOutput(ctx, ".intermediates", path)
}

var _ genPathProvider = SourcePath{}
var _ objPathProvider = SourcePath{}
var _ resPathProvider = SourcePath{}

// PathForModuleSrc returns a Path representing the paths... under the
// module's local source directory.
func PathForModuleSrc(ctx ModuleMissingDepsPathContext, pathComponents ...string) Path {
	// Just join the components textually just to make sure that it does not corrupt a fully qualified
	// module reference, e.g. if the pathComponents is "://other:foo" then using filepath.Join() or
	// validatePath() will corrupt it, e.g. replace "//" with "/". If the path is not a module
	// reference then it will be validated by expandOneSrcPath anyway when it calls expandOneSrcPath.
	p := strings.Join(pathComponents, string(filepath.Separator))
	paths, err := expandOneSrcPath(sourcePathInput{context: ctx, path: p, includeDirs: true})
	if err != nil {
		if depErr, ok := err.(missingDependencyError); ok {
			if ctx.Config().AllowMissingDependencies() {
				ctx.AddMissingDependencies(depErr.missingDeps)
			} else {
				ctx.ModuleErrorf(`%s, is the property annotated with android:"path"?`, depErr.Error())
			}
		} else {
			reportPathError(ctx, err)
		}
		return nil
	} else if len(paths) == 0 {
		ReportPathErrorf(ctx, "%q produced no files, expected exactly one", p)
		return nil
	} else if len(paths) > 1 {
		ReportPathErrorf(ctx, "%q produced %d files, expected exactly one", p, len(paths))
	}
	return paths[0]
}

func pathForModuleSrc(ctx EarlyModulePathContext, paths ...string) SourcePath {
	p, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}

	path, err := pathForSource(ctx, ctx.ModuleDir(), p)
	if err != nil {
		reportPathError(ctx, err)
	}

	path.basePath.rel = p

	return path
}

// PathsWithModuleSrcSubDir takes a list of Paths and returns a new list of Paths where Rel() on each path
// will return the path relative to subDir in the module's source directory.  If any input paths are not located
// inside subDir then a path error will be reported.
func PathsWithModuleSrcSubDir(ctx EarlyModulePathContext, paths Paths, subDir string) Paths {
	paths = append(Paths(nil), paths...)
	subDirFullPath := pathForModuleSrc(ctx, subDir)
	for i, path := range paths {
		rel := Rel(ctx, subDirFullPath.String(), path.String())
		paths[i] = subDirFullPath.join(ctx, rel)
	}
	return paths
}

// PathWithModuleSrcSubDir takes a Path and returns a Path where Rel() will return the path relative to subDir in the
// module's source directory.  If the input path is not located inside subDir then a path error will be reported.
func PathWithModuleSrcSubDir(ctx EarlyModulePathContext, path Path, subDir string) Path {
	subDirFullPath := pathForModuleSrc(ctx, subDir)
	rel := Rel(ctx, subDirFullPath.String(), path.String())
	return subDirFullPath.Join(ctx, rel)
}

// OptionalPathForModuleSrc returns an OptionalPath. The OptionalPath contains a
// valid path if p is non-nil.
func OptionalPathForModuleSrc(ctx ModuleMissingDepsPathContext, p *string) OptionalPath {
	if p == nil {
		return OptionalPath{}
	}
	return OptionalPathForPath(PathForModuleSrc(ctx, *p))
}

func (p SourcePath) genPathWithExt(ctx ModuleOutPathContext, subdir, ext string) ModuleGenPath {
	return PathForModuleGen(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

func (p SourcePath) genPathWithExtAndTrimExt(ctx ModuleOutPathContext, subdir, ext string, trimExt string) ModuleGenPath {
	// If Trim_extension being set, force append Output_extension without replace original extension.
	if trimExt != "" {
		if ext != "" {
			return PathForModuleGen(ctx, subdir, strings.TrimSuffix(p.path, trimExt)+"."+ext)
		}
		return PathForModuleGen(ctx, subdir, strings.TrimSuffix(p.path, trimExt))
	}
	return PathForModuleGen(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

func (p SourcePath) objPathWithExt(ctx ModuleOutPathContext, subdir, ext string) ModuleObjPath {
	return PathForModuleObj(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

func (p SourcePath) resPathWithName(ctx ModuleOutPathContext, name string) ModuleResPath {
	// TODO: Use full directory if the new ctx is not the current ctx?
	return PathForModuleRes(ctx, p.path, name)
}

// ModuleOutPath is a Path representing a module's output directory.
type ModuleOutPath struct {
	OutputPath
}

func (p ModuleOutPath) RelativeToTop() Path {
	p.OutputPath = p.outputPathRelativeToTop()
	return p
}

var _ Path = ModuleOutPath{}
var _ WritablePath = ModuleOutPath{}

func (p ModuleOutPath) objPathWithExt(ctx ModuleOutPathContext, subdir, ext string) ModuleObjPath {
	return PathForModuleObj(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

// ModuleOutPathContext Subset of ModuleContext functions necessary for output path methods.
type ModuleOutPathContext interface {
	PathContext

	ModuleName() string
	ModuleDir() string
	ModuleSubDir() string
	SoongConfigTraceHash() string
}

func pathForModuleOut(ctx ModuleOutPathContext) OutputPath {
	return PathForOutput(ctx, ".intermediates", ctx.ModuleDir(), ctx.ModuleName(), ctx.ModuleSubDir(), ctx.SoongConfigTraceHash())
}

// PathForModuleOut returns a Path representing the paths... under the module's
// output directory.
func PathForModuleOut(ctx ModuleOutPathContext, paths ...string) ModuleOutPath {
	p, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return ModuleOutPath{
		OutputPath: pathForModuleOut(ctx).withRel(p),
	}
}

// ModuleGenPath is a Path representing the 'gen' directory in a module's output
// directory. Mainly used for generated sources.
type ModuleGenPath struct {
	ModuleOutPath
}

func (p ModuleGenPath) RelativeToTop() Path {
	p.OutputPath = p.outputPathRelativeToTop()
	return p
}

var _ Path = ModuleGenPath{}
var _ WritablePath = ModuleGenPath{}
var _ genPathProvider = ModuleGenPath{}
var _ objPathProvider = ModuleGenPath{}

// PathForModuleGen returns a Path representing the paths... under the module's
// `gen' directory.
func PathForModuleGen(ctx ModuleOutPathContext, paths ...string) ModuleGenPath {
	p, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return ModuleGenPath{
		ModuleOutPath: ModuleOutPath{
			OutputPath: pathForModuleOut(ctx).withRel("gen").withRel(p),
		},
	}
}

func (p ModuleGenPath) genPathWithExt(ctx ModuleOutPathContext, subdir, ext string) ModuleGenPath {
	// TODO: make a different path for local vs remote generated files?
	return PathForModuleGen(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

func (p ModuleGenPath) genPathWithExtAndTrimExt(ctx ModuleOutPathContext, subdir, ext string, trimExt string) ModuleGenPath {
	// If Trim_extension being set, force append Output_extension without replace original extension.
	if trimExt != "" {
		if ext != "" {
			return PathForModuleGen(ctx, subdir, strings.TrimSuffix(p.path, trimExt)+"."+ext)
		}
		return PathForModuleGen(ctx, subdir, strings.TrimSuffix(p.path, trimExt))
	}
	return PathForModuleGen(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

func (p ModuleGenPath) objPathWithExt(ctx ModuleOutPathContext, subdir, ext string) ModuleObjPath {
	return PathForModuleObj(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

// ModuleObjPath is a Path representing the 'obj' directory in a module's output
// directory. Used for compiled objects.
type ModuleObjPath struct {
	ModuleOutPath
}

func (p ModuleObjPath) RelativeToTop() Path {
	p.OutputPath = p.outputPathRelativeToTop()
	return p
}

var _ Path = ModuleObjPath{}
var _ WritablePath = ModuleObjPath{}

// PathForModuleObj returns a Path representing the paths... under the module's
// 'obj' directory.
func PathForModuleObj(ctx ModuleOutPathContext, pathComponents ...string) ModuleObjPath {
	p, err := validatePath(pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return ModuleObjPath{PathForModuleOut(ctx, "obj", p)}
}

// ModuleResPath is a a Path representing the 'res' directory in a module's
// output directory.
type ModuleResPath struct {
	ModuleOutPath
}

func (p ModuleResPath) RelativeToTop() Path {
	p.OutputPath = p.outputPathRelativeToTop()
	return p
}

var _ Path = ModuleResPath{}
var _ WritablePath = ModuleResPath{}

// PathForModuleRes returns a Path representing the paths... under the module's
// 'res' directory.
func PathForModuleRes(ctx ModuleOutPathContext, pathComponents ...string) ModuleResPath {
	p, err := validatePath(pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}

	return ModuleResPath{PathForModuleOut(ctx, "res", p)}
}

// InstallPath is a Path representing a installed file path rooted from the build directory
type InstallPath struct {
	basePath

	// The soong build directory, i.e. Config.SoongOutDir()
	soongOutDir string

	// partitionDir is the part of the InstallPath that is automatically determined according to the context.
	// For example, it is host/<os>-<arch> for host modules, and target/product/<device>/<partition> for device modules.
	partitionDir string

	partition string

	// makePath indicates whether this path is for Soong (false) or Make (true).
	makePath bool

	fullPath string
}

// Will panic if called from outside a test environment.
func ensureTestOnly() {
	if PrefixInList(os.Args, "-test.") {
		return
	}
	panic(fmt.Errorf("Not in test. Command line:\n  %s", strings.Join(os.Args, "\n  ")))
}

func (p InstallPath) RelativeToTop() Path {
	ensureTestOnly()
	if p.makePath {
		p.soongOutDir = OutDir
	} else {
		p.soongOutDir = OutSoongDir
	}
	p.fullPath = filepath.Join(p.soongOutDir, p.path)
	return p
}

func (p InstallPath) getSoongOutDir() string {
	return p.soongOutDir
}

func (p InstallPath) ReplaceExtension(ctx PathContext, ext string) OutputPath {
	panic("Not implemented")
}

var _ Path = InstallPath{}
var _ WritablePath = InstallPath{}

func (p InstallPath) writablePath() {}

func (p InstallPath) String() string {
	return p.fullPath
}

// PartitionDir returns the path to the partition where the install path is rooted at. It is
// out/soong/target/product/<device>/<partition> for device modules, and out/soong/host/<os>-<arch> for host modules.
// The ./soong is dropped if the install path is for Make.
func (p InstallPath) PartitionDir() string {
	if p.makePath {
		return filepath.Join(p.soongOutDir, "../", p.partitionDir)
	} else {
		return filepath.Join(p.soongOutDir, p.partitionDir)
	}
}

func (p InstallPath) Partition() string {
	return p.partition
}

// Join creates a new InstallPath with paths... joined with the current path. The
// provided paths... may not use '..' to escape from the current path.
func (p InstallPath) Join(ctx PathContext, paths ...string) InstallPath {
	path, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return p.withRel(path)
}

func (p InstallPath) withRel(rel string) InstallPath {
	p.basePath = p.basePath.withRel(rel)
	p.fullPath = filepath.Join(p.fullPath, rel)
	return p
}

// Deprecated: ToMakePath is a noop, PathForModuleInstall always returns Make paths when building
// embedded in Make.
func (p InstallPath) ToMakePath() InstallPath {
	p.makePath = true
	return p
}

// PathForModuleInstall returns a Path representing the install path for the
// module appended with paths...
func PathForModuleInstall(ctx ModuleInstallPathContext, pathComponents ...string) InstallPath {
	os, arch := osAndArch(ctx)
	partition := modulePartition(ctx, os.Class == Device)
	return pathForInstall(ctx, os, arch, partition, pathComponents...)
}

// PathForHostDexInstall returns an InstallPath representing the install path for the
// module appended with paths...
func PathForHostDexInstall(ctx ModuleInstallPathContext, pathComponents ...string) InstallPath {
	return pathForInstall(ctx, ctx.Config().BuildOS, ctx.Config().BuildArch, "", pathComponents...)
}

// PathForModuleInPartitionInstall is similar to PathForModuleInstall but partition is provided by the caller
func PathForModuleInPartitionInstall(ctx ModuleInstallPathContext, partition string, pathComponents ...string) InstallPath {
	os, arch := osAndArch(ctx)
	return pathForInstall(ctx, os, arch, partition, pathComponents...)
}

func osAndArch(ctx ModuleInstallPathContext) (OsType, ArchType) {
	os := ctx.Os()
	arch := ctx.Arch().ArchType
	forceOS, forceArch := ctx.InstallForceOS()
	if forceOS != nil {
		os = *forceOS
	}
	if forceArch != nil {
		arch = *forceArch
	}
	return os, arch
}

func pathForPartitionInstallDir(ctx PathContext, partition, partitionPath string, makePath bool) InstallPath {
	fullPath := ctx.Config().SoongOutDir()
	if makePath {
		// Make path starts with out/ instead of out/soong.
		fullPath = filepath.Join(fullPath, "../", partitionPath)
	} else {
		fullPath = filepath.Join(fullPath, partitionPath)
	}

	return InstallPath{
		basePath:     basePath{partitionPath, ""},
		soongOutDir:  ctx.Config().soongOutDir,
		partitionDir: partitionPath,
		partition:    partition,
		makePath:     makePath,
		fullPath:     fullPath,
	}
}

func pathForInstall(ctx PathContext, os OsType, arch ArchType, partition string,
	pathComponents ...string) InstallPath {

	var partitionPaths []string

	if os.Class == Device {
		partitionPaths = []string{"target", "product", ctx.Config().DeviceName(), partition}
	} else {
		osName := os.String()
		if os == Linux {
			// instead of linux_glibc
			osName = "linux"
		}
		if os == LinuxMusl && ctx.Config().UseHostMusl() {
			// When using musl instead of glibc, use "linux" instead of "linux_musl".  When cross
			// compiling we will still use "linux_musl".
			osName = "linux"
		}

		// SOONG_HOST_OUT is set to out/host/$(HOST_OS)-$(HOST_PREBUILT_ARCH)
		// and HOST_PREBUILT_ARCH is forcibly set to x86 even on x86_64 hosts. We don't seem
		// to have a plan to fix it (see the comment in build/make/core/envsetup.mk).
		// Let's keep using x86 for the existing cases until we have a need to support
		// other architectures.
		archName := arch.String()
		if os.Class == Host && (arch == X86_64 || arch == Common) {
			archName = "x86"
		}
		partitionPaths = []string{"host", osName + "-" + archName, partition}
	}

	partitionPath, err := validatePath(partitionPaths...)
	if err != nil {
		reportPathError(ctx, err)
	}

	base := pathForPartitionInstallDir(ctx, partition, partitionPath, ctx.Config().KatiEnabled())
	return base.Join(ctx, pathComponents...)
}

func PathForNdkInstall(ctx PathContext, paths ...string) OutputPath {
	return PathForOutput(ctx, append([]string{"ndk"}, paths...)...)
}

func PathForMainlineSdksInstall(ctx PathContext, paths ...string) InstallPath {
	base := pathForPartitionInstallDir(ctx, "", "mainline-sdks", false)
	return base.Join(ctx, paths...)
}

func InstallPathToOnDevicePath(ctx PathContext, path InstallPath) string {
	rel := Rel(ctx, strings.TrimSuffix(path.PartitionDir(), path.partition), path.String())
	return "/" + rel
}

func modulePartition(ctx ModuleInstallPathContext, device bool) string {
	var partition string
	if ctx.InstallInTestcases() {
		// "testcases" install directory can be used for host or device modules.
		partition = "testcases"
	} else if device {
		if ctx.InstallInData() {
			partition = "data"
		} else if ctx.InstallInRamdisk() {
			if ctx.DeviceConfig().BoardUsesRecoveryAsBoot() {
				partition = "recovery/root/first_stage_ramdisk"
			} else {
				partition = "ramdisk"
			}
			if !ctx.InstallInRoot() {
				partition += "/system"
			}
		} else if ctx.InstallInVendorRamdisk() {
			// The module is only available after switching root into
			// /first_stage_ramdisk. To expose the module before switching root
			// on a device without a dedicated recovery partition, install the
			// recovery variant.
			if ctx.DeviceConfig().BoardMoveRecoveryResourcesToVendorBoot() {
				partition = "vendor_ramdisk/first_stage_ramdisk"
			} else {
				partition = "vendor_ramdisk"
			}
			if !ctx.InstallInRoot() {
				partition += "/system"
			}
		} else if ctx.InstallInDebugRamdisk() {
			partition = "debug_ramdisk"
		} else if ctx.InstallInRecovery() {
			if ctx.InstallInRoot() {
				partition = "recovery/root"
			} else {
				// the layout of recovery partion is the same as that of system partition
				partition = "recovery/root/system"
			}
		} else if ctx.SocSpecific() || ctx.InstallInVendor() {
			partition = ctx.DeviceConfig().VendorPath()
		} else if ctx.DeviceSpecific() || ctx.InstallInOdm() {
			partition = ctx.DeviceConfig().OdmPath()
		} else if ctx.ProductSpecific() || ctx.InstallInProduct() {
			partition = ctx.DeviceConfig().ProductPath()
		} else if ctx.SystemExtSpecific() {
			partition = ctx.DeviceConfig().SystemExtPath()
		} else if ctx.InstallInRoot() {
			partition = "root"
		} else {
			partition = "system"
		}
		if ctx.InstallInSanitizerDir() {
			partition = "data/asan/" + partition
		}
	}
	return partition
}

type InstallPaths []InstallPath

// Paths returns the InstallPaths as a Paths
func (p InstallPaths) Paths() Paths {
	if p == nil {
		return nil
	}
	ret := make(Paths, len(p))
	for i, path := range p {
		ret[i] = path
	}
	return ret
}

// Strings returns the string forms of the install paths.
func (p InstallPaths) Strings() []string {
	if p == nil {
		return nil
	}
	ret := make([]string, len(p))
	for i, path := range p {
		ret[i] = path.String()
	}
	return ret
}

// validatePathInternal ensures that a path does not leave its component, and
// optionally doesn't contain Ninja variables.
func validatePathInternal(allowNinjaVariables bool, pathComponents ...string) (string, error) {
	initialEmpty := 0
	finalEmpty := 0
	for i, path := range pathComponents {
		if !allowNinjaVariables && strings.Contains(path, "$") {
			return "", fmt.Errorf("Path contains invalid character($): %s", path)
		}

		path := filepath.Clean(path)
		if path == ".." || strings.HasPrefix(path, "../") || i != initialEmpty && strings.HasPrefix(path, "/") {
			return "", fmt.Errorf("Path is outside directory: %s", path)
		}

		if i == initialEmpty && pathComponents[i] == "" {
			initialEmpty++
		}
		if i == finalEmpty && pathComponents[len(pathComponents)-1-i] == "" {
			finalEmpty++
		}
	}
	// Optimization: filepath.Join("foo", "") returns a newly allocated copy
	// of "foo", while filepath.Join("foo") does not.  Strip out any empty
	// path components.
	if initialEmpty == len(pathComponents) {
		return "", nil
	}
	nonEmptyPathComponents := pathComponents[initialEmpty : len(pathComponents)-finalEmpty]
	// TODO: filepath.Join isn't necessarily correct with embedded ninja
	// variables. '..' may remove the entire ninja variable, even if it
	// will be expanded to multiple nested directories.
	return filepath.Join(nonEmptyPathComponents...), nil
}

// validateSafePath validates a path that we trust (may contain ninja
// variables).  Ensures that each path component does not attempt to leave its
// component. Returns a joined version of each path component.
func validateSafePath(pathComponents ...string) (string, error) {
	return validatePathInternal(true, pathComponents...)
}

// validatePath validates that a path does not include ninja variables, and that
// each path component does not attempt to leave its component. Returns a joined
// version of each path component.
func validatePath(pathComponents ...string) (string, error) {
	return validatePathInternal(false, pathComponents...)
}

func PathForPhony(ctx PathContext, phony string) WritablePath {
	if strings.ContainsAny(phony, "$/") {
		ReportPathErrorf(ctx, "Phony target contains invalid character ($ or /): %s", phony)
	}
	return PhonyPath{basePath{phony, ""}}
}

type PhonyPath struct {
	basePath
}

func (p PhonyPath) writablePath() {}

func (p PhonyPath) getSoongOutDir() string {
	// A phone path cannot contain any / so cannot be relative to the build directory.
	return ""
}

func (p PhonyPath) RelativeToTop() Path {
	ensureTestOnly()
	// A phony path cannot contain any / so does not have a build directory so switching to a new
	// build directory has no effect so just return this path.
	return p
}

func (p PhonyPath) ReplaceExtension(ctx PathContext, ext string) OutputPath {
	panic("Not implemented")
}

var _ Path = PhonyPath{}
var _ WritablePath = PhonyPath{}

type testPath struct {
	basePath
}

func (p testPath) RelativeToTop() Path {
	ensureTestOnly()
	return p
}

func (p testPath) String() string {
	return p.path
}

var _ Path = testPath{}

// PathForTesting returns a Path constructed from joining the elements of paths with '/'.  It should only be used from
// within tests.
func PathForTesting(paths ...string) Path {
	p, err := validateSafePath(paths...)
	if err != nil {
		panic(err)
	}
	return testPath{basePath{path: p, rel: p}}
}

func PathForTestingWithRel(path, rel string) Path {
	p, err := validateSafePath(path, rel)
	if err != nil {
		panic(err)
	}
	r, err := validatePath(rel)
	if err != nil {
		panic(err)
	}
	return testPath{basePath{path: p, rel: r}}
}

// PathsForTesting returns a Path constructed from each element in strs. It should only be used from within tests.
func PathsForTesting(strs ...string) Paths {
	p := make(Paths, len(strs))
	for i, s := range strs {
		p[i] = PathForTesting(s)
	}

	return p
}

type testPathContext struct {
	config Config
}

func (x *testPathContext) Config() Config             { return x.config }
func (x *testPathContext) AddNinjaFileDeps(...string) {}

// PathContextForTesting returns a PathContext that can be used in tests, for example to create an OutputPath with
// PathForOutput.
func PathContextForTesting(config Config) PathContext {
	return &testPathContext{
		config: config,
	}
}

type testModuleInstallPathContext struct {
	baseModuleContext

	inData          bool
	inTestcases     bool
	inSanitizerDir  bool
	inRamdisk       bool
	inVendorRamdisk bool
	inDebugRamdisk  bool
	inRecovery      bool
	inRoot          bool
	inOdm           bool
	inProduct       bool
	inVendor        bool
	forceOS         *OsType
	forceArch       *ArchType
}

func (m testModuleInstallPathContext) Config() Config {
	return m.baseModuleContext.config
}

func (testModuleInstallPathContext) AddNinjaFileDeps(deps ...string) {}

func (m testModuleInstallPathContext) InstallInData() bool {
	return m.inData
}

func (m testModuleInstallPathContext) InstallInTestcases() bool {
	return m.inTestcases
}

func (m testModuleInstallPathContext) InstallInSanitizerDir() bool {
	return m.inSanitizerDir
}

func (m testModuleInstallPathContext) InstallInRamdisk() bool {
	return m.inRamdisk
}

func (m testModuleInstallPathContext) InstallInVendorRamdisk() bool {
	return m.inVendorRamdisk
}

func (m testModuleInstallPathContext) InstallInDebugRamdisk() bool {
	return m.inDebugRamdisk
}

func (m testModuleInstallPathContext) InstallInRecovery() bool {
	return m.inRecovery
}

func (m testModuleInstallPathContext) InstallInRoot() bool {
	return m.inRoot
}

func (m testModuleInstallPathContext) InstallInOdm() bool {
	return m.inOdm
}

func (m testModuleInstallPathContext) InstallInProduct() bool {
	return m.inProduct
}

func (m testModuleInstallPathContext) InstallInVendor() bool {
	return m.inVendor
}

func (m testModuleInstallPathContext) InstallForceOS() (*OsType, *ArchType) {
	return m.forceOS, m.forceArch
}

// Construct a minimal ModuleInstallPathContext for testing. Note that baseModuleContext is
// default-initialized, which leaves blueprint.baseModuleContext set to nil, so methods that are
// delegated to it will panic.
func ModuleInstallPathContextForTesting(config Config) ModuleInstallPathContext {
	ctx := &testModuleInstallPathContext{}
	ctx.config = config
	ctx.os = Android
	return ctx
}

// Rel performs the same function as filepath.Rel, but reports errors to a PathContext, and reports an error if
// targetPath is not inside basePath.
func Rel(ctx PathContext, basePath string, targetPath string) string {
	rel, isRel := MaybeRel(ctx, basePath, targetPath)
	if !isRel {
		ReportPathErrorf(ctx, "path %q is not under path %q", targetPath, basePath)
		return ""
	}
	return rel
}

// MaybeRel performs the same function as filepath.Rel, but reports errors to a PathContext, and returns false if
// targetPath is not inside basePath.
func MaybeRel(ctx PathContext, basePath string, targetPath string) (string, bool) {
	rel, isRel, err := maybeRelErr(basePath, targetPath)
	if err != nil {
		reportPathError(ctx, err)
	}
	return rel, isRel
}

func maybeRelErr(basePath string, targetPath string) (string, bool, error) {
	// filepath.Rel returns an error if one path is absolute and the other is not, handle that case first.
	if filepath.IsAbs(basePath) != filepath.IsAbs(targetPath) {
		return "", false, nil
	}
	rel, err := filepath.Rel(basePath, targetPath)
	if err != nil {
		return "", false, err
	} else if rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return "", false, nil
	}
	return rel, true, nil
}

// Writes a file to the output directory.  Attempting to write directly to the output directory
// will fail due to the sandbox of the soong_build process.
// Only writes the file if the file doesn't exist or if it has different contents, to prevent
// updating the timestamp if no changes would be made. (This is better for incremental
// performance.)
func WriteFileToOutputDir(path WritablePath, data []byte, perm os.FileMode) error {
	absPath := absolutePath(path.String())
	err := os.MkdirAll(filepath.Dir(absPath), 0777)
	if err != nil {
		return err
	}
	return pathtools.WriteFileIfChanged(absPath, data, perm)
}

func RemoveAllOutputDir(path WritablePath) error {
	return os.RemoveAll(absolutePath(path.String()))
}

func CreateOutputDirIfNonexistent(path WritablePath, perm os.FileMode) error {
	dir := absolutePath(path.String())
	return createDirIfNonexistent(dir, perm)
}

func createDirIfNonexistent(dir string, perm os.FileMode) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, os.ModePerm)
	} else {
		return err
	}
}

// absolutePath is deliberately private so that Soong's Go plugins can't use it to find and
// read arbitrary files without going through the methods in the current package that track
// dependencies.
func absolutePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(absSrcDir, path)
}

// A DataPath represents the path of a file to be used as data, for example
// a test library to be installed alongside a test.
// The data file should be installed (copied from `<SrcPath>`) to
// `<install_root>/<RelativeInstallPath>/<filename>`, or
// `<install_root>/<filename>` if RelativeInstallPath is empty.
type DataPath struct {
	// The path of the data file that should be copied into the data directory
	SrcPath Path
	// The install path of the data file, relative to the install root.
	RelativeInstallPath string
	// If WithoutRel is true, use SrcPath.Base() instead of SrcPath.Rel() as the filename.
	WithoutRel bool
}

func (d *DataPath) ToRelativeInstallPath() string {
	relPath := d.SrcPath.Rel()
	if d.WithoutRel {
		relPath = d.SrcPath.Base()
	}
	if d.RelativeInstallPath != "" {
		relPath = filepath.Join(d.RelativeInstallPath, relPath)
	}
	return relPath
}

// PathsIfNonNil returns a Paths containing only the non-nil input arguments.
func PathsIfNonNil(paths ...Path) Paths {
	if len(paths) == 0 {
		// Fast path for empty argument list
		return nil
	} else if len(paths) == 1 {
		// Fast path for a single argument
		if paths[0] != nil {
			return paths
		} else {
			return nil
		}
	}
	ret := make(Paths, 0, len(paths))
	for _, path := range paths {
		if path != nil {
			ret = append(ret, path)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

var thirdPartyDirPrefixExceptions = []*regexp.Regexp{
	regexp.MustCompile("^vendor/[^/]*google[^/]*/"),
	regexp.MustCompile("^hardware/google/"),
	regexp.MustCompile("^hardware/interfaces/"),
	regexp.MustCompile("^hardware/libhardware[^/]*/"),
	regexp.MustCompile("^hardware/ril/"),
}

func IsThirdPartyPath(path string) bool {
	thirdPartyDirPrefixes := []string{"external/", "vendor/", "hardware/"}

	if HasAnyPrefix(path, thirdPartyDirPrefixes) {
		for _, prefix := range thirdPartyDirPrefixExceptions {
			if prefix.MatchString(path) {
				return false
			}
		}
		return true
	}
	return false
}
