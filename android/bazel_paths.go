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
	"android/soong/bazel"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
)

// bazel_paths contains methods to:
//   * resolve Soong path and module references into bazel.LabelList
//   * resolve Bazel path references into Soong-compatible paths
//
// There is often a similar method for Bazel as there is for Soong path handling and should be used
// in similar circumstances
//
// Bazel                                Soong
//
// BazelLabelForModuleSrc               PathForModuleSrc
// BazelLabelForModuleSrcExcludes       PathForModuleSrcExcludes
// BazelLabelForModuleDeps              n/a
// tbd                                  PathForSource
// tbd                                  ExistentPathsForSources
// PathForBazelOut                      PathForModuleOut
//
// Use cases:
//  * Module contains a property (often tagged `android:"path"`) that expects paths *relative to the
//    module directory*:
//     * BazelLabelForModuleSrcExcludes, if the module also contains an excludes_<propname> property
//     * BazelLabelForModuleSrc, otherwise
//  * Converting references to other modules to Bazel Labels:
//     BazelLabelForModuleDeps
//  * Converting a path obtained from bazel_handler cquery results:
//     PathForBazelOut
//
// NOTE: all Soong globs are expanded within Soong rather than being converted to a Bazel glob
//       syntax. This occurs because Soong does not have a concept of crossing package boundaries,
//       so the glob as computed by Soong may contain paths that cross package-boundaries. These
//       would be unknowingly omitted if the glob were handled by Bazel. By expanding globs within
//       Soong, we support identification and detection (within Bazel) use of paths that cross
//       package boundaries.
//
// Path resolution:
// * filepath/globs: resolves as itself or is converted to an absolute Bazel label (e.g.
//   //path/to/dir:<filepath>) if path exists in a separate package or subpackage.
// * references to other modules (using the ":name{.tag}" syntax). These resolve as a Bazel label
//   for a target. If the Bazel target is in the local module directory, it will be returned
//   relative to the current package (e.g.  ":<target>"). Otherwise, it will be returned as an
//   absolute Bazel label (e.g.  "//path/to/dir:<target>"). If the reference to another module
//   cannot be resolved,the function will panic. This is often due to the dependency not being added
//   via an AddDependency* method.

// A subset of the ModuleContext methods which are sufficient to resolve references to paths/deps in
// order to form a Bazel-compatible label for conversion.
type BazelConversionPathContext interface {
	EarlyModulePathContext

	GetDirectDep(name string) (blueprint.Module, blueprint.DependencyTag)
	Module() Module
	ModuleType() string
	OtherModuleName(m blueprint.Module) string
	OtherModuleDir(m blueprint.Module) string
}

// BazelLabelForModuleDeps expects a list of reference to other modules, ("<module>"
// or ":<module>") and returns a Bazel-compatible label which corresponds to dependencies on the
// module within the given ctx.
func BazelLabelForModuleDeps(ctx BazelConversionPathContext, modules []string) bazel.LabelList {
	var labels bazel.LabelList
	for _, module := range modules {
		bpText := module
		if m := SrcIsModule(module); m == "" {
			module = ":" + module
		}
		if m, t := SrcIsModuleWithTag(module); m != "" {
			l := getOtherModuleLabel(ctx, m, t)
			l.OriginalModuleName = bpText
			labels.Includes = append(labels.Includes, l)
		} else {
			ctx.ModuleErrorf("%q, is not a module reference", module)
		}
	}
	return labels
}

func BazelLabelForModuleSrcSingle(ctx BazelConversionPathContext, path string) bazel.Label {
	return BazelLabelForModuleSrcExcludes(ctx, []string{path}, []string(nil)).Includes[0]
}

// BazelLabelForModuleSrc expects a list of path (relative to local module directory) and module
// references (":<module>") and returns a bazel.LabelList{} containing the resolved references in
// paths, relative to the local module, or Bazel-labels (absolute if in a different package or
// relative if within the same package).
// Properties must have been annotated with struct tag `android:"path"` so that dependencies modules
// will have already been handled by the path_deps mutator.
func BazelLabelForModuleSrc(ctx BazelConversionPathContext, paths []string) bazel.LabelList {
	return BazelLabelForModuleSrcExcludes(ctx, paths, []string(nil))
}

// BazelLabelForModuleSrc expects lists of path and excludes (relative to local module directory)
// and module references (":<module>") and returns a bazel.LabelList{} containing the resolved
// references in paths, minus those in excludes, relative to the local module, or Bazel-labels
// (absolute if in a different package or relative if within the same package).
// Properties must have been annotated with struct tag `android:"path"` so that dependencies modules
// will have already been handled by the path_deps mutator.
func BazelLabelForModuleSrcExcludes(ctx BazelConversionPathContext, paths, excludes []string) bazel.LabelList {
	excludeLabels := expandSrcsForBazel(ctx, excludes, []string(nil))
	excluded := make([]string, 0, len(excludeLabels.Includes))
	for _, e := range excludeLabels.Includes {
		excluded = append(excluded, e.Label)
	}
	labels := expandSrcsForBazel(ctx, paths, excluded)
	labels.Excludes = excludeLabels.Includes
	labels = transformSubpackagePaths(ctx, labels)
	return labels
}

// Returns true if a prefix + components[:i] + /Android.bp exists
// TODO(b/185358476) Could check for BUILD file instead of checking for Android.bp file, or ensure BUILD is always generated?
func directoryHasBlueprint(fs pathtools.FileSystem, prefix string, components []string, componentIndex int) bool {
	blueprintPath := prefix
	if blueprintPath != "" {
		blueprintPath = blueprintPath + "/"
	}
	blueprintPath = blueprintPath + strings.Join(components[:componentIndex+1], "/")
	blueprintPath = blueprintPath + "/Android.bp"
	if exists, _, _ := fs.Exists(blueprintPath); exists {
		return true
	} else {
		return false
	}
}

// Transform a path (if necessary) to acknowledge package boundaries
//
// e.g. something like
//   async_safe/include/async_safe/CHECK.h
// might become
//   //bionic/libc/async_safe:include/async_safe/CHECK.h
// if the "async_safe" directory is actually a package and not just a directory.
//
// In particular, paths that extend into packages are transformed into absolute labels beginning with //.
func transformSubpackagePath(ctx BazelConversionPathContext, path bazel.Label) bazel.Label {
	var newPath bazel.Label

	// Don't transform OriginalModuleName
	newPath.OriginalModuleName = path.OriginalModuleName

	if strings.HasPrefix(path.Label, "//") {
		// Assume absolute labels are already correct (e.g. //path/to/some/package:foo.h)
		newPath.Label = path.Label
		return newPath
	}

	newLabel := ""
	pathComponents := strings.Split(path.Label, "/")
	foundBlueprint := false
	// Check the deepest subdirectory first and work upwards
	for i := len(pathComponents) - 1; i >= 0; i-- {
		pathComponent := pathComponents[i]
		var sep string
		if !foundBlueprint && directoryHasBlueprint(ctx.Config().fs, ctx.ModuleDir(), pathComponents, i) {
			sep = ":"
			foundBlueprint = true
		} else {
			sep = "/"
		}
		if newLabel == "" {
			newLabel = pathComponent
		} else {
			newLabel = pathComponent + sep + newLabel
		}
	}
	if foundBlueprint {
		// Ensure paths end up looking like //bionic/... instead of //./bionic/...
		moduleDir := ctx.ModuleDir()
		if strings.HasPrefix(moduleDir, ".") {
			moduleDir = moduleDir[1:]
		}
		// Make the path into an absolute label (e.g. //bionic/libc/foo:bar.h instead of just foo:bar.h)
		if moduleDir == "" {
			newLabel = "//" + newLabel
		} else {
			newLabel = "//" + moduleDir + "/" + newLabel
		}
	}
	newPath.Label = newLabel

	return newPath
}

// Transform paths to acknowledge package boundaries
// See transformSubpackagePath() for more information
func transformSubpackagePaths(ctx BazelConversionPathContext, paths bazel.LabelList) bazel.LabelList {
	var newPaths bazel.LabelList
	for _, include := range paths.Includes {
		newPaths.Includes = append(newPaths.Includes, transformSubpackagePath(ctx, include))
	}
	for _, exclude := range paths.Excludes {
		newPaths.Excludes = append(newPaths.Excludes, transformSubpackagePath(ctx, exclude))
	}
	return newPaths
}

// expandSrcsForBazel returns bazel.LabelList with paths rooted from the module's local source
// directory and Bazel target labels, excluding those included in the excludes argument (which
// should already be expanded to resolve references to Soong-modules). Valid elements of paths
// include:
// * filepath, relative to local module directory, resolves as a filepath relative to the local
//   source directory
// * glob, relative to the local module directory, resolves as filepath(s), relative to the local
//    module directory. Because Soong does not have a concept of crossing package boundaries, the
//    glob as computed by Soong may contain paths that cross package-boundaries that would be
//    unknowingly omitted if the glob were handled by Bazel. To allow identification and detect
//    (within Bazel) use of paths that cross package boundaries, we expand globs within Soong rather
//    than converting Soong glob syntax to Bazel glob syntax. **Invalid for excludes.**
// * other modules using the ":name{.tag}" syntax. These modules must implement SourceFileProducer
//    or OutputFileProducer. These resolve as a Bazel label for a target. If the Bazel target is in
//    the local module directory, it will be returned relative to the current package (e.g.
//    ":<target>"). Otherwise, it will be returned as an absolute Bazel label (e.g.
//    "//path/to/dir:<target>"). If the reference to another module cannot be resolved,the function
//    will panic.
// Properties passed as the paths or excludes argument must have been annotated with struct tag
// `android:"path"` so that dependencies on other modules will have already been handled by the
// path_deps mutator.
func expandSrcsForBazel(ctx BazelConversionPathContext, paths, expandedExcludes []string) bazel.LabelList {
	if paths == nil {
		return bazel.LabelList{}
	}
	labels := bazel.LabelList{
		Includes: []bazel.Label{},
	}

	// expandedExcludes contain module-dir relative paths, but root-relative paths
	// are needed for GlobFiles later.
	var rootRelativeExpandedExcludes []string
	for _, e := range expandedExcludes {
		rootRelativeExpandedExcludes = append(rootRelativeExpandedExcludes, filepath.Join(ctx.ModuleDir(), e))
	}

	for _, p := range paths {
		if m, tag := SrcIsModuleWithTag(p); m != "" {
			l := getOtherModuleLabel(ctx, m, tag)
			if !InList(l.Label, expandedExcludes) {
				l.OriginalModuleName = fmt.Sprintf(":%s", m)
				labels.Includes = append(labels.Includes, l)
			}
		} else {
			var expandedPaths []bazel.Label
			if pathtools.IsGlob(p) {
				// e.g. turn "math/*.c" in
				// external/arm-optimized-routines to external/arm-optimized-routines/math/*.c
				rootRelativeGlobPath := pathForModuleSrc(ctx, p).String()
				globbedPaths := GlobFiles(ctx, rootRelativeGlobPath, rootRelativeExpandedExcludes)
				globbedPaths = PathsWithModuleSrcSubDir(ctx, globbedPaths, "")
				for _, path := range globbedPaths {
					s := path.Rel()
					expandedPaths = append(expandedPaths, bazel.Label{Label: s})
				}
			} else {
				if !InList(p, expandedExcludes) {
					expandedPaths = append(expandedPaths, bazel.Label{Label: p})
				}
			}
			labels.Includes = append(labels.Includes, expandedPaths...)
		}
	}
	return labels
}

// getOtherModuleLabel returns a bazel.Label for the given dependency/tag combination for the
// module. The label will be relative to the current directory if appropriate. The dependency must
// already be resolved by either deps mutator or path deps mutator.
func getOtherModuleLabel(ctx BazelConversionPathContext, dep, tag string) bazel.Label {
	m, _ := ctx.GetDirectDep(dep)
	if m == nil {
		panic(fmt.Errorf(`Cannot get direct dep %q of %q.
		This is likely because it was not added via AddDependency().
		This may be due a mutator skipped during bp2build.`, dep, ctx.Module().Name()))
	}
	otherLabel := bazelModuleLabel(ctx, m, tag)
	label := bazelModuleLabel(ctx, ctx.Module(), "")
	if samePackage(label, otherLabel) {
		otherLabel = bazelShortLabel(otherLabel)
	}

	return bazel.Label{
		Label: otherLabel,
	}
}

func bazelModuleLabel(ctx BazelConversionPathContext, module blueprint.Module, tag string) string {
	// TODO(b/165114590): Convert tag (":name{.tag}") to corresponding Bazel implicit output targets.
	b, ok := module.(Bazelable)
	// TODO(b/181155349): perhaps return an error here if the module can't be/isn't being converted
	if !ok || !b.ConvertedToBazel(ctx) {
		return bp2buildModuleLabel(ctx, module)
	}
	return b.GetBazelLabel(ctx, module)
}

func bazelShortLabel(label string) string {
	i := strings.Index(label, ":")
	return label[i:]
}

func bazelPackage(label string) string {
	i := strings.Index(label, ":")
	return label[0:i]
}

func samePackage(label1, label2 string) bool {
	return bazelPackage(label1) == bazelPackage(label2)
}

func bp2buildModuleLabel(ctx BazelConversionPathContext, module blueprint.Module) string {
	moduleName := ctx.OtherModuleName(module)
	moduleDir := ctx.OtherModuleDir(module)
	return fmt.Sprintf("//%s:%s", moduleDir, moduleName)
}

// BazelOutPath is a Bazel output path compatible to be used for mixed builds within Soong/Ninja.
type BazelOutPath struct {
	OutputPath
}

var _ Path = BazelOutPath{}
var _ objPathProvider = BazelOutPath{}

func (p BazelOutPath) objPathWithExt(ctx ModuleOutPathContext, subdir, ext string) ModuleObjPath {
	return PathForModuleObj(ctx, subdir, pathtools.ReplaceExtension(p.path, ext))
}

// PathForBazelOut returns a Path representing the paths... under an output directory dedicated to
// bazel-owned outputs.
func PathForBazelOut(ctx PathContext, paths ...string) BazelOutPath {
	execRootPathComponents := append([]string{"execroot", "__main__"}, paths...)
	execRootPath := filepath.Join(execRootPathComponents...)
	validatedExecRootPath, err := validatePath(execRootPath)
	if err != nil {
		reportPathError(ctx, err)
	}

	outputPath := OutputPath{basePath{"", ""},
		ctx.Config().buildDir,
		ctx.Config().BazelContext.OutputBase()}

	return BazelOutPath{
		OutputPath: outputPath.withRel(validatedExecRootPath),
	}
}

// PathsForBazelOut returns a list of paths representing the paths under an output directory
// dedicated to Bazel-owned outputs.
func PathsForBazelOut(ctx PathContext, paths []string) Paths {
	outs := make(Paths, 0, len(paths))
	for _, p := range paths {
		outs = append(outs, PathForBazelOut(ctx, p))
	}
	return outs
}
