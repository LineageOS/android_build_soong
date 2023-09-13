// Copyright 2022 Google Inc. All rights reserved.
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

package bp2build

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"android/soong/shared"

	"github.com/google/blueprint/pathtools"
)

// A tree structure that describes what to do at each directory in the created
// symlink tree. Currently it is used to enumerate which files/directories
// should be excluded from symlinking. Each instance of "node" represents a file
// or a directory. If excluded is true, then that file/directory should be
// excluded from symlinking. Otherwise, the node is not excluded, but one of its
// descendants is (otherwise the node in question would not exist)

// This is a version int written to a file called symlink_forest_version at the root of the
// symlink forest. If the version here does not match the version in the file, then we'll
// clean the whole symlink forest and recreate it. This number can be bumped whenever there's
// an incompatible change to the forest layout or a bug in incrementality that needs to be fixed
// on machines that may still have the bug present in their forest.
const symlinkForestVersion = 2

type instructionsNode struct {
	name     string
	excluded bool // If false, this is just an intermediate node
	children map[string]*instructionsNode
}

type symlinkForestContext struct {
	verbose bool
	topdir  string // $TOPDIR

	// State
	wg           sync.WaitGroup
	depCh        chan string
	mkdirCount   atomic.Uint64
	symlinkCount atomic.Uint64
}

// Ensures that the node for the given path exists in the tree and returns it.
func ensureNodeExists(root *instructionsNode, path string) *instructionsNode {
	if path == "" {
		return root
	}

	if path[len(path)-1] == '/' {
		path = path[:len(path)-1] // filepath.Split() leaves a trailing slash
	}

	dir, base := filepath.Split(path)

	// First compute the parent node...
	dn := ensureNodeExists(root, dir)

	// then create the requested node as its direct child, if needed.
	if child, ok := dn.children[base]; ok {
		return child
	} else {
		dn.children[base] = &instructionsNode{base, false, make(map[string]*instructionsNode)}
		return dn.children[base]
	}
}

// Turns a list of paths to be excluded into a tree
func instructionsFromExcludePathList(paths []string) *instructionsNode {
	result := &instructionsNode{"", false, make(map[string]*instructionsNode)}

	for _, p := range paths {
		ensureNodeExists(result, p).excluded = true
	}

	return result
}

func mergeBuildFiles(output string, srcBuildFile string, generatedBuildFile string, verbose bool) error {

	srcBuildFileContent, err := os.ReadFile(srcBuildFile)
	if err != nil {
		return err
	}

	generatedBuildFileContent, err := os.ReadFile(generatedBuildFile)
	if err != nil {
		return err
	}

	// There can't be a package() call in both the source and generated BUILD files.
	// bp2build will generate a package() call for licensing information, but if
	// there's no licensing information, it will still generate a package() call
	// that just sets default_visibility=public. If the handcrafted build file
	// also has a package() call, we'll allow it to override the bp2build
	// generated one if it doesn't have any licensing information. If the bp2build
	// one has licensing information and the handcrafted one exists, we'll leave
	// them both in for bazel to throw an error.
	packageRegex := regexp.MustCompile(`(?m)^package\s*\(`)
	packageDefaultVisibilityRegex := regexp.MustCompile(`(?m)^package\s*\(\s*default_visibility\s*=\s*\[\s*"//visibility:public",?\s*]\s*\)`)
	if packageRegex.Find(srcBuildFileContent) != nil {
		if verbose && packageDefaultVisibilityRegex.Find(generatedBuildFileContent) != nil {
			fmt.Fprintf(os.Stderr, "Both '%s' and '%s' have a package() target, removing the first one\n",
				generatedBuildFile, srcBuildFile)
		}
		generatedBuildFileContent = packageDefaultVisibilityRegex.ReplaceAll(generatedBuildFileContent, []byte{})
	}

	newContents := generatedBuildFileContent
	if newContents[len(newContents)-1] != '\n' {
		newContents = append(newContents, '\n')
	}
	newContents = append(newContents, srcBuildFileContent...)

	// Say you run bp2build 4 times:
	// - The first time there's only an Android.bp file. bp2build will convert it to a build file
	//   under out/soong/bp2build, then symlink from the forest to that generated file
	// - Then you add a handcrafted BUILD file in the same directory. bp2build will merge this with
	//   the generated one, and write the result to the output file in the forest. But the output
	//   file was a symlink to out/soong/bp2build from the previous step! So we erroneously update
	//   the file in out/soong/bp2build instead. So far this doesn't cause any problems...
	// - You run a 3rd bp2build with no relevant changes. Everything continues to work.
	// - You then add a comment to the handcrafted BUILD file. This causes a merge with the
	//   generated file again. But since we wrote to the generated file in step 2, the generated
	//   file has an old copy of the handcrafted file in it! This probably causes duplicate bazel
	//   targets.
	// To solve this, if we see that the output file is a symlink from a previous build, remove it.
	stat, err := os.Lstat(output)
	if err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		if stat.Mode()&os.ModeSymlink == os.ModeSymlink {
			if verbose {
				fmt.Fprintf(os.Stderr, "Removing symlink so that we can replace it with a merged file: %s\n", output)
			}
			err = os.Remove(output)
			if err != nil {
				return err
			}
		}
	}

	return pathtools.WriteFileIfChanged(output, newContents, 0666)
}

// Calls readdir() and returns it as a map from the basename of the files in dir
// to os.FileInfo.
func readdirToMap(dir string) map[string]os.FileInfo {
	entryList, err := ioutil.ReadDir(dir)
	result := make(map[string]os.FileInfo)

	if err != nil {
		if os.IsNotExist(err) {
			// It's okay if a directory doesn't exist; it just means that one of the
			// trees to be merged contains parts the other doesn't
			return result
		} else {
			fmt.Fprintf(os.Stderr, "Cannot readdir '%s': %s\n", dir, err)
			os.Exit(1)
		}
	}

	for _, fi := range entryList {
		result[fi.Name()] = fi
	}

	return result
}

// Creates a symbolic link at dst pointing to src
func symlinkIntoForest(topdir, dst, src string) uint64 {
	srcPath := shared.JoinPath(topdir, src)
	dstPath := shared.JoinPath(topdir, dst)

	// Check if a symlink already exists.
	if dstInfo, err := os.Lstat(dstPath); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Failed to lstat '%s': %s", dst, err)
			os.Exit(1)
		}
	} else {
		if dstInfo.Mode()&os.ModeSymlink != 0 {
			// Assume that the link's target is correct, i.e. no manual tampering.
			// E.g. OUT_DIR could have been previously used with a different source tree check-out!
			return 0
		} else {
			if err := os.RemoveAll(dstPath); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to remove '%s': %s", dst, err)
				os.Exit(1)
			}
		}
	}

	// Create symlink.
	if err := os.Symlink(srcPath, dstPath); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create symlink at '%s' pointing to '%s': %s", dst, src, err)
		os.Exit(1)
	}
	return 1
}

func isDir(path string, fi os.FileInfo) bool {
	if (fi.Mode() & os.ModeSymlink) != os.ModeSymlink {
		return fi.IsDir()
	}

	fi2, statErr := os.Stat(path)
	if statErr == nil {
		return fi2.IsDir()
	}

	// Check if this is a dangling symlink. If so, treat it like a file, not a dir.
	_, lstatErr := os.Lstat(path)
	if lstatErr != nil {
		fmt.Fprintf(os.Stderr, "Cannot stat or lstat '%s': %s\n%s\n", path, statErr, lstatErr)
		os.Exit(1)
	}

	return false
}

// maybeCleanSymlinkForest will remove the whole symlink forest directory if the version recorded
// in the symlink_forest_version file is not equal to symlinkForestVersion.
func maybeCleanSymlinkForest(topdir, forest string, verbose bool) error {
	versionFilePath := shared.JoinPath(topdir, forest, "symlink_forest_version")
	versionFileContents, err := os.ReadFile(versionFilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	versionFileString := strings.TrimSpace(string(versionFileContents))
	symlinkForestVersionString := strconv.Itoa(symlinkForestVersion)
	if err != nil || versionFileString != symlinkForestVersionString {
		if verbose {
			fmt.Fprintf(os.Stderr, "Old symlink_forest_version was %q, current is %q. Cleaning symlink forest before recreating...\n", versionFileString, symlinkForestVersionString)
		}
		err = os.RemoveAll(shared.JoinPath(topdir, forest))
		if err != nil {
			return err
		}
	}
	return nil
}

// maybeWriteVersionFile will write the symlink_forest_version file containing symlinkForestVersion
// if it doesn't exist already. If it exists we know it must contain symlinkForestVersion because
// we checked for that already in maybeCleanSymlinkForest
func maybeWriteVersionFile(topdir, forest string) error {
	versionFilePath := shared.JoinPath(topdir, forest, "symlink_forest_version")
	_, err := os.Stat(versionFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		err = os.WriteFile(versionFilePath, []byte(strconv.Itoa(symlinkForestVersion)+"\n"), 0666)
		if err != nil {
			return err
		}
	}
	return nil
}

// Recursively plants a symlink forest at forestDir. The symlink tree will
// contain every file in buildFilesDir and srcDir excluding the files in
// instructions. Collects every directory encountered during the traversal of
// srcDir .
func plantSymlinkForestRecursive(context *symlinkForestContext, instructions *instructionsNode, forestDir string, buildFilesDir string, srcDir string) {
	defer context.wg.Done()

	if instructions != nil && instructions.excluded {
		// Excluded paths are skipped at the level of the non-excluded parent.
		fmt.Fprintf(os.Stderr, "may not specify a root-level exclude directory '%s'", srcDir)
		os.Exit(1)
	}

	// We don't add buildFilesDir here because the bp2build files marker files is
	// already a dependency which covers it. If we ever wanted to turn this into
	// a generic symlink forest creation tool, we'd need to add it, too.
	context.depCh <- srcDir

	srcDirMap := readdirToMap(shared.JoinPath(context.topdir, srcDir))
	buildFilesMap := readdirToMap(shared.JoinPath(context.topdir, buildFilesDir))

	renamingBuildFile := false
	if _, ok := srcDirMap["BUILD"]; ok {
		if _, ok := srcDirMap["BUILD.bazel"]; !ok {
			if _, ok := buildFilesMap["BUILD.bazel"]; ok {
				renamingBuildFile = true
				srcDirMap["BUILD.bazel"] = srcDirMap["BUILD"]
				delete(srcDirMap, "BUILD")
				if instructions != nil {
					if _, ok := instructions.children["BUILD"]; ok {
						instructions.children["BUILD.bazel"] = instructions.children["BUILD"]
						delete(instructions.children, "BUILD")
					}
				}
			}
		}
	}

	allEntries := make([]string, 0, len(srcDirMap)+len(buildFilesMap))
	for n := range srcDirMap {
		allEntries = append(allEntries, n)
	}
	for n := range buildFilesMap {
		if _, ok := srcDirMap[n]; !ok {
			allEntries = append(allEntries, n)
		}
	}
	// Tests read the error messages generated, so ensure their order is deterministic
	sort.Strings(allEntries)

	fullForestPath := shared.JoinPath(context.topdir, forestDir)
	createForestDir := false
	if fi, err := os.Lstat(fullForestPath); err != nil {
		if os.IsNotExist(err) {
			createForestDir = true
		} else {
			fmt.Fprintf(os.Stderr, "Could not read info for '%s': %s\n", forestDir, err)
		}
	} else if fi.Mode()&os.ModeDir == 0 {
		if err := os.RemoveAll(fullForestPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove '%s': %s", forestDir, err)
			os.Exit(1)
		}
		createForestDir = true
	}
	if createForestDir {
		if err := os.MkdirAll(fullForestPath, 0777); err != nil {
			fmt.Fprintf(os.Stderr, "Could not mkdir '%s': %s\n", forestDir, err)
			os.Exit(1)
		}
		context.mkdirCount.Add(1)
	}

	// Start with a list of items that already exist in the forest, and remove
	// each element as it is processed in allEntries. Any remaining items in
	// forestMapForDeletion must be removed. (This handles files which were
	// removed since the previous forest generation).
	forestMapForDeletion := readdirToMap(shared.JoinPath(context.topdir, forestDir))

	for _, f := range allEntries {
		if f[0] == '.' {
			continue // Ignore dotfiles
		}
		delete(forestMapForDeletion, f)
		// todo add deletionCount metric

		// The full paths of children in the input trees and in the output tree
		forestChild := shared.JoinPath(forestDir, f)
		srcChild := shared.JoinPath(srcDir, f)
		if f == "BUILD.bazel" && renamingBuildFile {
			srcChild = shared.JoinPath(srcDir, "BUILD")
		}
		buildFilesChild := shared.JoinPath(buildFilesDir, f)

		// Descend in the instruction tree if it exists
		var instructionsChild *instructionsNode
		if instructions != nil {
			instructionsChild = instructions.children[f]
		}

		srcChildEntry, sExists := srcDirMap[f]
		buildFilesChildEntry, bExists := buildFilesMap[f]

		if instructionsChild != nil && instructionsChild.excluded {
			if bExists {
				context.symlinkCount.Add(symlinkIntoForest(context.topdir, forestChild, buildFilesChild))
			}
			continue
		}

		sDir := sExists && isDir(shared.JoinPath(context.topdir, srcChild), srcChildEntry)
		bDir := bExists && isDir(shared.JoinPath(context.topdir, buildFilesChild), buildFilesChildEntry)

		if !sExists {
			if bDir && instructionsChild != nil {
				// Not in the source tree, but we have to exclude something from under
				// this subtree, so descend
				context.wg.Add(1)
				go plantSymlinkForestRecursive(context, instructionsChild, forestChild, buildFilesChild, srcChild)
			} else {
				// Not in the source tree, symlink BUILD file
				context.symlinkCount.Add(symlinkIntoForest(context.topdir, forestChild, buildFilesChild))
			}
		} else if !bExists {
			if sDir && instructionsChild != nil {
				// Not in the build file tree, but we have to exclude something from
				// under this subtree, so descend
				context.wg.Add(1)
				go plantSymlinkForestRecursive(context, instructionsChild, forestChild, buildFilesChild, srcChild)
			} else {
				// Not in the build file tree, symlink source tree, carry on
				context.symlinkCount.Add(symlinkIntoForest(context.topdir, forestChild, srcChild))
			}
		} else if sDir && bDir {
			// Both are directories. Descend.
			context.wg.Add(1)
			go plantSymlinkForestRecursive(context, instructionsChild, forestChild, buildFilesChild, srcChild)
		} else if !sDir && !bDir {
			// Neither is a directory. Merge them.
			srcBuildFile := shared.JoinPath(context.topdir, srcChild)
			generatedBuildFile := shared.JoinPath(context.topdir, buildFilesChild)
			// The Android.bp file that codegen used to produce `buildFilesChild` is
			// already a dependency, we can ignore `buildFilesChild`.
			context.depCh <- srcChild
			if err := mergeBuildFiles(shared.JoinPath(context.topdir, forestChild), srcBuildFile, generatedBuildFile, context.verbose); err != nil {
				fmt.Fprintf(os.Stderr, "Error merging %s and %s: %s",
					srcBuildFile, generatedBuildFile, err)
				os.Exit(1)
			}
		} else {
			// Both exist and one is a file. This is an error.
			fmt.Fprintf(os.Stderr,
				"Conflict in workspace symlink tree creation: both '%s' and '%s' exist and exactly one is a directory\n",
				srcChild, buildFilesChild)
			os.Exit(1)
		}
	}

	// Remove all files in the forest that exist in neither the source
	// tree nor the build files tree. (This handles files which were removed
	// since the previous forest generation).
	for f := range forestMapForDeletion {
		var instructionsChild *instructionsNode
		if instructions != nil {
			instructionsChild = instructions.children[f]
		}

		if instructionsChild != nil && instructionsChild.excluded {
			// This directory may be excluded because bazel writes to it under the
			// forest root. Thus this path is intentionally left alone.
			continue
		}
		forestChild := shared.JoinPath(context.topdir, forestDir, f)
		if err := os.RemoveAll(forestChild); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove '%s/%s': %s", forestDir, f, err)
			os.Exit(1)
		}
	}
}

// PlantSymlinkForest Creates a symlink forest by merging the directory tree at "buildFiles" and
// "srcDir" while excluding paths listed in "exclude". Returns the set of paths
// under srcDir on which readdir() had to be called to produce the symlink
// forest.
func PlantSymlinkForest(verbose bool, topdir string, forest string, buildFiles string, exclude []string) (deps []string, mkdirCount, symlinkCount uint64) {
	context := &symlinkForestContext{
		verbose:      verbose,
		topdir:       topdir,
		depCh:        make(chan string),
		mkdirCount:   atomic.Uint64{},
		symlinkCount: atomic.Uint64{},
	}

	err := maybeCleanSymlinkForest(topdir, forest, verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	instructions := instructionsFromExcludePathList(exclude)
	go func() {
		context.wg.Add(1)
		plantSymlinkForestRecursive(context, instructions, forest, buildFiles, ".")
		context.wg.Wait()
		close(context.depCh)
	}()

	for dep := range context.depCh {
		deps = append(deps, dep)
	}

	err = maybeWriteVersionFile(topdir, forest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return deps, context.mkdirCount.Load(), context.symlinkCount.Load()
}
