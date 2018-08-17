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
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
)

// PathContext is the subset of a (Module|Singleton)Context required by the
// Path methods.
type PathContext interface {
	Fs() pathtools.FileSystem
	Config() Config
	AddNinjaFileDeps(deps ...string)
}

type PathGlobContext interface {
	GlobWithDeps(globPattern string, excludes []string) ([]string, error)
}

var _ PathContext = SingletonContext(nil)
var _ PathContext = ModuleContext(nil)

type ModuleInstallPathContext interface {
	PathContext

	androidBaseContext

	InstallInData() bool
	InstallInSanitizerDir() bool
}

var _ ModuleInstallPathContext = ModuleContext(nil)

// errorfContext is the interface containing the Errorf method matching the
// Errorf method in blueprint.SingletonContext.
type errorfContext interface {
	Errorf(format string, args ...interface{})
}

var _ errorfContext = blueprint.SingletonContext(nil)

// moduleErrorf is the interface containing the ModuleErrorf method matching
// the ModuleErrorf method in blueprint.ModuleContext.
type moduleErrorf interface {
	ModuleErrorf(format string, args ...interface{})
}

var _ moduleErrorf = blueprint.ModuleContext(nil)

// reportPathError will register an error with the attached context. It
// attempts ctx.ModuleErrorf for a better error message first, then falls
// back to ctx.Errorf.
func reportPathError(ctx PathContext, err error) {
	reportPathErrorf(ctx, "%s", err.Error())
}

// reportPathErrorf will register an error with the attached context. It
// attempts ctx.ModuleErrorf for a better error message first, then falls
// back to ctx.Errorf.
func reportPathErrorf(ctx PathContext, format string, args ...interface{}) {
	if mctx, ok := ctx.(moduleErrorf); ok {
		mctx.ModuleErrorf(format, args...)
	} else if ectx, ok := ctx.(errorfContext); ok {
		ectx.Errorf(format, args...)
	} else {
		panic(fmt.Sprintf(format, args...))
	}
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
}

// WritablePath is a type of path that can be used as an output for build rules.
type WritablePath interface {
	Path

	// the writablePath method doesn't directly do anything,
	// but it allows a struct to distinguish between whether or not it implements the WritablePath interface
	writablePath()
}

type genPathProvider interface {
	genPathWithExt(ctx ModuleContext, subdir, ext string) ModuleGenPath
}
type objPathProvider interface {
	objPathWithExt(ctx ModuleContext, subdir, ext string) ModuleObjPath
}
type resPathProvider interface {
	resPathWithName(ctx ModuleContext, name string) ModuleResPath
}

// GenPathWithExt derives a new file path in ctx's generated sources directory
// from the current path, but with the new extension.
func GenPathWithExt(ctx ModuleContext, subdir string, p Path, ext string) ModuleGenPath {
	if path, ok := p.(genPathProvider); ok {
		return path.genPathWithExt(ctx, subdir, ext)
	}
	reportPathErrorf(ctx, "Tried to create generated file from unsupported path: %s(%s)", reflect.TypeOf(p).Name(), p)
	return PathForModuleGen(ctx)
}

// ObjPathWithExt derives a new file path in ctx's object directory from the
// current path, but with the new extension.
func ObjPathWithExt(ctx ModuleContext, subdir string, p Path, ext string) ModuleObjPath {
	if path, ok := p.(objPathProvider); ok {
		return path.objPathWithExt(ctx, subdir, ext)
	}
	reportPathErrorf(ctx, "Tried to create object file from unsupported path: %s (%s)", reflect.TypeOf(p).Name(), p)
	return PathForModuleObj(ctx)
}

// ResPathWithName derives a new path in ctx's output resource directory, using
// the current path to create the directory name, and the `name` argument for
// the filename.
func ResPathWithName(ctx ModuleContext, p Path, name string) ModuleResPath {
	if path, ok := p.(resPathProvider); ok {
		return path.resPathWithName(ctx, name)
	}
	reportPathErrorf(ctx, "Tried to create res file from unsupported path: %s (%s)", reflect.TypeOf(p).Name(), p)
	return PathForModuleRes(ctx)
}

// OptionalPath is a container that may or may not contain a valid Path.
type OptionalPath struct {
	valid bool
	path  Path
}

// OptionalPathForPath returns an OptionalPath containing the path.
func OptionalPathForPath(path Path) OptionalPath {
	if path == nil {
		return OptionalPath{}
	}
	return OptionalPath{valid: true, path: path}
}

// Valid returns whether there is a valid path
func (p OptionalPath) Valid() bool {
	return p.valid
}

// Path returns the Path embedded in this OptionalPath. You must be sure that
// there is a valid path, since this method will panic if there is not.
func (p OptionalPath) Path() Path {
	if !p.valid {
		panic("Requesting an invalid path")
	}
	return p.path
}

// String returns the string version of the Path, or "" if it isn't valid.
func (p OptionalPath) String() string {
	if p.valid {
		return p.path.String()
	} else {
		return ""
	}
}

// Paths is a slice of Path objects, with helpers to operate on the collection.
type Paths []Path

// PathsForSource returns Paths rooted from SrcDir
func PathsForSource(ctx PathContext, paths []string) Paths {
	ret := make(Paths, len(paths))
	for i, path := range paths {
		ret[i] = PathForSource(ctx, path)
	}
	return ret
}

// ExistentPathsForSources returns a list of Paths rooted from SrcDir that are
// found in the tree. If any are not found, they are omitted from the list,
// and dependencies are added so that we're re-run when they are added.
func ExistentPathsForSources(ctx PathContext, paths []string) Paths {
	ret := make(Paths, 0, len(paths))
	for _, path := range paths {
		p := ExistentPathForSource(ctx, path)
		if p.Valid() {
			ret = append(ret, p.Path())
		}
	}
	return ret
}

// PathsForModuleSrc returns Paths rooted from the module's local source
// directory
func PathsForModuleSrc(ctx ModuleContext, paths []string) Paths {
	ret := make(Paths, len(paths))
	for i, path := range paths {
		ret[i] = PathForModuleSrc(ctx, path)
	}
	return ret
}

// pathsForModuleSrcFromFullPath returns Paths rooted from the module's local
// source directory, but strip the local source directory from the beginning of
// each string. If incDirs is false, strip paths with a trailing '/' from the list.
func pathsForModuleSrcFromFullPath(ctx ModuleContext, paths []string, incDirs bool) Paths {
	prefix := filepath.Join(ctx.Config().srcDir, ctx.ModuleDir()) + "/"
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
			reportPathErrorf(ctx, "Path '%s' is not in module source directory '%s'", p, prefix)
			continue
		}
		ret = append(ret, PathForModuleSrc(ctx, path[len(prefix):]))
	}
	return ret
}

// PathsWithOptionalDefaultForModuleSrc returns Paths rooted from the module's
// local source directory. If none are provided, use the default if it exists.
func PathsWithOptionalDefaultForModuleSrc(ctx ModuleContext, input []string, def string) Paths {
	if len(input) > 0 {
		return PathsForModuleSrc(ctx, input)
	}
	// Use Glob so that if the default doesn't exist, a dependency is added so that when it
	// is created, we're run again.
	path := filepath.Join(ctx.Config().srcDir, ctx.ModuleDir(), def)
	return ctx.Glob(path, nil)
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

// FirstUniquePaths returns all unique elements of a Paths, keeping the first copy of each.  It
// modifies the Paths slice contents in place, and returns a subslice of the original slice.
func FirstUniquePaths(list Paths) Paths {
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
	for _, l := range list {
		if inPathList(l, filter) {
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

// WritablePaths is a slice of WritablePaths, used for multiple outputs.
type WritablePaths []WritablePath

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
	path   string
	config Config
	rel    string
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
func safePathForSource(ctx PathContext, path string) SourcePath {
	p, err := validateSafePath(path)
	if err != nil {
		reportPathError(ctx, err)
	}
	ret := SourcePath{basePath{p, ctx.Config(), ""}}

	abs, err := filepath.Abs(ret.String())
	if err != nil {
		reportPathError(ctx, err)
		return ret
	}
	buildroot, err := filepath.Abs(ctx.Config().buildDir)
	if err != nil {
		reportPathError(ctx, err)
		return ret
	}
	if strings.HasPrefix(abs, buildroot) {
		reportPathErrorf(ctx, "source path %s is in output", abs)
		return ret
	}

	return ret
}

// pathForSource creates a SourcePath from pathComponents, but does not check that it exists.
func pathForSource(ctx PathContext, pathComponents ...string) (SourcePath, error) {
	p, err := validatePath(pathComponents...)
	ret := SourcePath{basePath{p, ctx.Config(), ""}}
	if err != nil {
		return ret, err
	}

	abs, err := filepath.Abs(ret.String())
	if err != nil {
		return ret, err
	}
	buildroot, err := filepath.Abs(ctx.Config().buildDir)
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

// pathForSourceRelaxed creates a SourcePath from pathComponents, but does not check that it exists.
// It differs from pathForSource in that the path is allowed to exist outside of the PathContext.
func pathForSourceRelaxed(ctx PathContext, pathComponents ...string) (SourcePath, error) {
	p := filepath.Join(pathComponents...)
	ret := SourcePath{basePath{p, ctx.Config(), ""}}

	abs, err := filepath.Abs(ret.String())
	if err != nil {
		return ret, err
	}
	buildroot, err := filepath.Abs(ctx.Config().buildDir)
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
func existsWithDependencies(ctx PathContext, path SourcePath) (exists bool, err error) {
	var files []string

	if gctx, ok := ctx.(PathGlobContext); ok {
		// Use glob to produce proper dependencies, even though we only want
		// a single file.
		files, err = gctx.GlobWithDeps(path.String(), nil)
	} else {
		var deps []string
		// We cannot add build statements in this context, so we fall back to
		// AddNinjaFileDeps
		files, deps, err = pathtools.Glob(path.String(), nil)
		ctx.AddNinjaFileDeps(deps...)
	}

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

	if modCtx, ok := ctx.(ModuleContext); ok && ctx.Config().AllowMissingDependencies() {
		exists, err := existsWithDependencies(ctx, path)
		if err != nil {
			reportPathError(ctx, err)
		}
		if !exists {
			modCtx.AddMissingDependencies([]string{path.String()})
		}
	} else if exists, _, err := ctx.Fs().Exists(path.String()); err != nil {
		reportPathErrorf(ctx, "%s: %s", path, err.Error())
	} else if !exists {
		reportPathErrorf(ctx, "source path %s does not exist", path)
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
		exists, err := existsWithDependencies(ctx, path)
		if err != nil {
			reportPathError(ctx, err)
		}
		if !exists {
			modCtx.AddMissingDependencies([]string{path.String()})
		}
	} else if exists, _, err := ctx.Fs().Exists(path.String()); err != nil {
		reportPathErrorf(ctx, "%s: %s", path, err.Error())
	} else if !exists {
		reportPathErrorf(ctx, "source path %s does not exist", path)
	}
	return path
}

func ApplySourceOverlays(ctx ModuleContext, directives string, allowedModules []string, srcFiles []Path) []Path {
	// Multiple overlay directives should be white space separated.
	// Individual directive format is:
	// <modulename>|<overlaydir>|<globwithinoverlaydir>
	// Example:
	// org.lineageos.hardware|device/oneplus/msm8998-common/lineagehw|**/*.java
	// If there is a file match by multiple directives, the first wins.

	// Create map of expanded glob paths (globwithinoverlaydir component only)
	// to full android.Path mappings.
	overlayMap := make(map[string]Path)

	// Split directives string into distinct directives.
	for _, directive := range strings.Fields(directives) {
		// Split directive string into whitespace fields.
		fields := strings.SplitN(directive, "|", 3)
		if len(fields) != 3 {
			ctx.ModuleErrorf("could not parse source overlay directive %s", directive)
			continue
		}

		// Name the per-directive fields.
		module, dir, glob := fields[0], fields[1], fields[2]

		// Skip overlay directives that don't apply to this module.
		if module != ctx.ModuleName() {
			continue
		}

		// Check whether sources overlays are permitted for this module.
		allowed := false
		for _, allowedModule := range allowedModules {
			if allowedModule == module {
				allowed = true
				break
			}
		}
		if !allowed {
			// Source overlays for this module are not
			// allowed, skip.
			ctx.ModuleErrorf("not allowed to sources overlay module %s", module)
			return srcFiles
		}

		addPathsToOverlayMap(ctx, dir, glob, overlayMap)
	}

	// Calculate the length of the path to the module root dir (including trailing slash).
	modulePathLen := len(ctx.ModuleDir()) + 1

	// Replace entries in srcFiles where the path within the module matches a key in overlayMap.
	for i := range srcFiles {
		srcFilePath := srcFiles[i].String()
		if len(srcFilePath) < modulePathLen {
			continue
		}
		pathWithinModule := srcFilePath[modulePathLen:]
		if overlayFile, found := overlayMap[pathWithinModule]; found {
			srcFiles[i] = overlayFile
			delete(overlayMap, pathWithinModule)
		}
	}
	// and create new srcFiles entries for those that don't.
	for _, path := range overlayMap {
		srcFiles = append(srcFiles, path)
	}

	return srcFiles
}

func addPathsToOverlayMap(ctx ModuleContext, dir, glob string, overlayMap map[string]Path) {
	// Glob dir/glob to establish which overlay files exist.
	globPath := filepath.Join(dir, glob)
	paths, err := ctx.GlobWithDeps(globPath, nil)
	if err != nil {
		ctx.ModuleErrorf("unable to glob %s: %s", globPath, err.Error())
		return
	}
	// Add globbed paths to overlayMap.  If an overlay already exists, it is
	// not overwritten.
	for _, path := range paths {
		// Skip directories.
		if strings.HasSuffix(path, "/") {
			continue
		}
		// Ensure that the globbed match points to something inside the module.
		if !strings.HasPrefix(path, dir+"/") {
			continue
		}
		// Note: the preceding checks tells us that len(path) > len(dir)+1
		pathWithinModule := path[len(dir)+1:] // Account for trailing slash.
		if _, found := overlayMap[pathWithinModule]; !found {
			overlayMap[pathWithinModule] = PathForSourceRelaxed(ctx, path)
		}
	}
}

// ExistentPathForSource returns an OptionalPath with the SourcePath if the
// path exists, or an empty OptionalPath if it doesn't exist. Dependencies are added
// so that the ninja file will be regenerated if the state of the path changes.
func ExistentPathForSource(ctx PathContext, pathComponents ...string) OptionalPath {
	path, err := pathForSource(ctx, pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
		return OptionalPath{}
	}

	exists, err := existsWithDependencies(ctx, path)
	if err != nil {
		reportPathError(ctx, err)
		return OptionalPath{}
	}
	if !exists {
		return OptionalPath{}
	}
	return OptionalPathForPath(path)
}

func (p SourcePath) String() string {
	return filepath.Join(p.config.srcDir, p.path)
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

// OverlayPath returns the overlay for `path' if it exists. This assumes that the
// SourcePath is the path to a resource overlay directory.
func (p SourcePath) OverlayPath(ctx ModuleContext, path Path) OptionalPath {
	var relDir string
	if moduleSrcPath, ok := path.(ModuleSrcPath); ok {
		relDir = moduleSrcPath.path
	} else if srcPath, ok := path.(SourcePath); ok {
		relDir = srcPath.path
	} else {
		reportPathErrorf(ctx, "Cannot find relative path for %s(%s)", reflect.TypeOf(path).Name(), path)
		return OptionalPath{}
	}
	dir := filepath.Join(p.config.srcDir, p.path, relDir)
	// Use Glob so that we are run again if the directory is added.
	if pathtools.IsGlob(dir) {
		reportPathErrorf(ctx, "Path may not contain a glob: %s", dir)
	}
	paths, err := ctx.GlobWithDeps(dir, nil)
	if err != nil {
		reportPathErrorf(ctx, "glob: %s", err.Error())
		return OptionalPath{}
	}
	if len(paths) == 0 {
		return OptionalPath{}
	}
	relPath, err := filepath.Rel(p.config.srcDir, paths[0])
	if err != nil {
		reportPathError(ctx, err)
		return OptionalPath{}
	}
	return OptionalPathForPath(PathForSource(ctx, relPath))
}

// OutputPath is a Path representing a file path rooted from the build directory
type OutputPath struct {
	basePath
}

func (p OutputPath) withRel(rel string) OutputPath {
	p.basePath = p.basePath.withRel(rel)
	return p
}

var _ Path = OutputPath{}

// PathForOutput joins the provided paths and returns an OutputPath that is
// validated to not escape the build dir.
// On error, it will return a usable, but invalid OutputPath, and report a ModuleError.
func PathForOutput(ctx PathContext, pathComponents ...string) OutputPath {
	path, err := validatePath(pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return OutputPath{basePath{path, ctx.Config(), ""}}
}

func (p OutputPath) writablePath() {}

func (p OutputPath) String() string {
	return filepath.Join(p.config.buildDir, p.path)
}

func (p OutputPath) RelPathString() string {
	return p.path
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

// PathForIntermediates returns an OutputPath representing the top-level
// intermediates directory.
func PathForIntermediates(ctx PathContext, paths ...string) OutputPath {
	path, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return PathForOutput(ctx, ".intermediates", path)
}

// DistPath is a Path representing a file path rooted from the dist directory
type DistPath struct {
	basePath
}

func (p DistPath) withRel(rel string) DistPath {
	p.basePath = p.basePath.withRel(rel)
	return p
}

var _ Path = DistPath{}

// PathForDist joins the provided paths and returns a DistPath that is
// validated to not escape the dist dir.
// On error, it will return a usable, but invalid DistPath, and report a ModuleError.
func PathForDist(ctx PathContext, pathComponents ...string) DistPath {
	path, err := validatePath(pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return DistPath{basePath{path, ctx.Config(), ""}}
}

func (p DistPath) writablePath() {}

func (p DistPath) Valid() bool {
	return p.config.productVariables.DistDir != nil && *p.config.productVariables.DistDir != ""
}

func (p DistPath) String() string {
	if !p.Valid() {
		panic("Requesting an invalid path")
	}
	return filepath.Join(*p.config.productVariables.DistDir, p.path)
}

func (p DistPath) RelPathString() string {
	return p.path
}

// ModuleSrcPath is a Path representing a file rooted from a module's local source dir
type ModuleSrcPath struct {
	SourcePath
}

var _ Path = ModuleSrcPath{}
var _ genPathProvider = ModuleSrcPath{}
var _ objPathProvider = ModuleSrcPath{}
var _ resPathProvider = ModuleSrcPath{}

// PathForModuleSrc returns a ModuleSrcPath representing the paths... under the
// module's local source directory.
func PathForModuleSrc(ctx ModuleContext, paths ...string) ModuleSrcPath {
	p, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}

	srcPath, err := pathForSource(ctx, ctx.ModuleDir(), p)
	if err != nil {
		reportPathError(ctx, err)
	}

	path := ModuleSrcPath{srcPath}
	path.basePath.rel = p

	if exists, _, err := ctx.Fs().Exists(path.String()); err != nil {
		reportPathErrorf(ctx, "%s: %s", path, err.Error())
	} else if !exists {
		reportPathErrorf(ctx, "module source path %s does not exist", path)
	}

	return path
}

// OptionalPathForModuleSrc returns an OptionalPath. The OptionalPath contains a
// valid path if p is non-nil.
func OptionalPathForModuleSrc(ctx ModuleContext, p *string) OptionalPath {
	if p == nil {
		return OptionalPath{}
	}
	return OptionalPathForPath(PathForModuleSrc(ctx, *p))
}

func (p ModuleSrcPath) genPathWithExt(ctx ModuleContext, subdir, ext string) ModuleGenPath {
	return PathForModuleGen(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

func (p ModuleSrcPath) objPathWithExt(ctx ModuleContext, subdir, ext string) ModuleObjPath {
	return PathForModuleObj(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

func (p ModuleSrcPath) resPathWithName(ctx ModuleContext, name string) ModuleResPath {
	// TODO: Use full directory if the new ctx is not the current ctx?
	return PathForModuleRes(ctx, p.path, name)
}

func (p ModuleSrcPath) WithSubDir(ctx ModuleContext, subdir string) ModuleSrcPath {
	subdir = PathForModuleSrc(ctx, subdir).String()
	var err error
	rel, err := filepath.Rel(subdir, p.path)
	if err != nil {
		ctx.ModuleErrorf("source file %q is not under path %q", p.path, subdir)
		return p
	}
	p.rel = rel
	return p
}

// ModuleOutPath is a Path representing a module's output directory.
type ModuleOutPath struct {
	OutputPath
}

var _ Path = ModuleOutPath{}

func pathForModule(ctx ModuleContext) OutputPath {
	return PathForOutput(ctx, ".intermediates", ctx.ModuleDir(), ctx.ModuleName(), ctx.ModuleSubDir())
}

// PathForVndkRefDump returns an OptionalPath representing the path of the reference
// abi dump for the given module. This is not guaranteed to be valid.
func PathForVndkRefAbiDump(ctx ModuleContext, version, fileName string, vndkOrNdk, isSourceDump bool) OptionalPath {
	arches := ctx.DeviceConfig().Arches()
	currentArch := ctx.Arch()
	archNameAndVariant := currentArch.ArchType.String()
	if currentArch.ArchVariant != "" {
		archNameAndVariant += "_" + currentArch.ArchVariant
	}
	var sourceOrBinaryDir string
	var vndkOrNdkDir string
	var ext string
	if isSourceDump {
		ext = ".lsdump.gz"
		sourceOrBinaryDir = "source-based"
	} else {
		ext = ".bdump.gz"
		sourceOrBinaryDir = "binary-based"
	}
	if vndkOrNdk {
		vndkOrNdkDir = "vndk"
	} else {
		vndkOrNdkDir = "ndk"
	}
	if len(arches) == 0 {
		panic("device build with no primary arch")
	}
	binderBitness := ctx.DeviceConfig().BinderBitness()
	refDumpFileStr := "prebuilts/abi-dumps/" + vndkOrNdkDir + "/" + version + "/" + binderBitness + "/" +
		archNameAndVariant + "/" + sourceOrBinaryDir + "/" + fileName + ext
	return ExistentPathForSource(ctx, refDumpFileStr)
}

// PathForModuleOut returns a Path representing the paths... under the module's
// output directory.
func PathForModuleOut(ctx ModuleContext, paths ...string) ModuleOutPath {
	p, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return ModuleOutPath{
		OutputPath: pathForModule(ctx).withRel(p),
	}
}

// ModuleGenPath is a Path representing the 'gen' directory in a module's output
// directory. Mainly used for generated sources.
type ModuleGenPath struct {
	ModuleOutPath
}

var _ Path = ModuleGenPath{}
var _ genPathProvider = ModuleGenPath{}
var _ objPathProvider = ModuleGenPath{}

// PathForModuleGen returns a Path representing the paths... under the module's
// `gen' directory.
func PathForModuleGen(ctx ModuleContext, paths ...string) ModuleGenPath {
	p, err := validatePath(paths...)
	if err != nil {
		reportPathError(ctx, err)
	}
	return ModuleGenPath{
		ModuleOutPath: ModuleOutPath{
			OutputPath: pathForModule(ctx).withRel("gen").withRel(p),
		},
	}
}

func (p ModuleGenPath) genPathWithExt(ctx ModuleContext, subdir, ext string) ModuleGenPath {
	// TODO: make a different path for local vs remote generated files?
	return PathForModuleGen(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

func (p ModuleGenPath) objPathWithExt(ctx ModuleContext, subdir, ext string) ModuleObjPath {
	return PathForModuleObj(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

// ModuleObjPath is a Path representing the 'obj' directory in a module's output
// directory. Used for compiled objects.
type ModuleObjPath struct {
	ModuleOutPath
}

var _ Path = ModuleObjPath{}

// PathForModuleObj returns a Path representing the paths... under the module's
// 'obj' directory.
func PathForModuleObj(ctx ModuleContext, pathComponents ...string) ModuleObjPath {
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

var _ Path = ModuleResPath{}

// PathForModuleRes returns a Path representing the paths... under the module's
// 'res' directory.
func PathForModuleRes(ctx ModuleContext, pathComponents ...string) ModuleResPath {
	p, err := validatePath(pathComponents...)
	if err != nil {
		reportPathError(ctx, err)
	}

	return ModuleResPath{PathForModuleOut(ctx, "res", p)}
}

// PathForModuleInstall returns a Path representing the install path for the
// module appended with paths...
func PathForModuleInstall(ctx ModuleInstallPathContext, pathComponents ...string) OutputPath {
	var outPaths []string
	if ctx.Device() {
		var partition string
		if ctx.InstallInData() {
			partition = "data"
		} else if ctx.SocSpecific() {
			partition = ctx.DeviceConfig().VendorPath()
		} else if ctx.DeviceSpecific() {
			partition = ctx.DeviceConfig().OdmPath()
		} else if ctx.ProductSpecific() {
			partition = ctx.DeviceConfig().ProductPath()
		} else {
			partition = "system"
		}

		if ctx.InstallInSanitizerDir() {
			partition = "data/asan/" + partition
		}
		outPaths = []string{"target", "product", ctx.Config().DeviceName(), partition}
	} else {
		switch ctx.Os() {
		case Linux:
			outPaths = []string{"host", "linux-x86"}
		case LinuxBionic:
			// TODO: should this be a separate top level, or shared with linux-x86?
			outPaths = []string{"host", "linux_bionic-x86"}
		default:
			outPaths = []string{"host", ctx.Os().String() + "-x86"}
		}
	}
	if ctx.Debug() {
		outPaths = append([]string{"debug"}, outPaths...)
	}
	outPaths = append(outPaths, pathComponents...)
	return PathForOutput(ctx, outPaths...)
}

// validateSafePath validates a path that we trust (may contain ninja variables).
// Ensures that each path component does not attempt to leave its component.
func validateSafePath(pathComponents ...string) (string, error) {
	for _, path := range pathComponents {
		path := filepath.Clean(path)
		if path == ".." || strings.HasPrefix(path, "../") || strings.HasPrefix(path, "/") {
			return "", fmt.Errorf("Path is outside directory: %s", path)
		}
	}
	// TODO: filepath.Join isn't necessarily correct with embedded ninja
	// variables. '..' may remove the entire ninja variable, even if it
	// will be expanded to multiple nested directories.
	return filepath.Join(pathComponents...), nil
}

// validatePath validates that a path does not include ninja variables, and that
// each path component does not attempt to leave its component. Returns a joined
// version of each path component.
func validatePath(pathComponents ...string) (string, error) {
	for _, path := range pathComponents {
		if strings.Contains(path, "$") {
			return "", fmt.Errorf("Path contains invalid character($): %s", path)
		}
	}
	return validateSafePath(pathComponents...)
}

func PathForPhony(ctx PathContext, phony string) WritablePath {
	if strings.ContainsAny(phony, "$/") {
		reportPathErrorf(ctx, "Phony target contains invalid character ($ or /): %s", phony)
	}
	return PhonyPath{basePath{phony, ctx.Config(), ""}}
}

type PhonyPath struct {
	basePath
}

func (p PhonyPath) writablePath() {}

var _ Path = PhonyPath{}
var _ WritablePath = PhonyPath{}

type testPath struct {
	basePath
}

func (p testPath) String() string {
	return p.path
}

func PathForTesting(paths ...string) Path {
	p, err := validateSafePath(paths...)
	if err != nil {
		panic(err)
	}
	return testPath{basePath{path: p, rel: p}}
}

func PathsForTesting(strs []string) Paths {
	p := make(Paths, len(strs))
	for i, s := range strs {
		p[i] = PathForTesting(s)
	}

	return p
}
