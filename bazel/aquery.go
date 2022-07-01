// Copyright 2020 Google Inc. All rights reserved.
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

package bazel

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/google/blueprint/proptools"
)

type artifactId int
type depsetId int
type pathFragmentId int

// artifact contains relevant portions of Bazel's aquery proto, Artifact.
// Represents a single artifact, whether it's a source file or a derived output file.
type artifact struct {
	Id             artifactId
	PathFragmentId pathFragmentId
}

type pathFragment struct {
	Id       pathFragmentId
	Label    string
	ParentId pathFragmentId
}

// KeyValuePair represents Bazel's aquery proto, KeyValuePair.
type KeyValuePair struct {
	Key   string
	Value string
}

// AqueryDepset is a depset definition from Bazel's aquery response. This is
// akin to the `depSetOfFiles` in the response proto, except:
//   * direct artifacts are enumerated by full path instead of by ID
//   * it has a hash of the depset contents, instead of an int ID (for determinism)
// A depset is a data structure for efficient transitive handling of artifact
// paths. A single depset consists of one or more artifact paths and one or
// more "child" depsets.
type AqueryDepset struct {
	ContentHash            string
	DirectArtifacts        []string
	TransitiveDepSetHashes []string
}

// depSetOfFiles contains relevant portions of Bazel's aquery proto, DepSetOfFiles.
// Represents a data structure containing one or more files. Depsets in Bazel are an efficient
// data structure for storing large numbers of file paths.
type depSetOfFiles struct {
	Id                  depsetId
	DirectArtifactIds   []artifactId
	TransitiveDepSetIds []depsetId
}

// action contains relevant portions of Bazel's aquery proto, Action.
// Represents a single command line invocation in the Bazel build graph.
type action struct {
	Arguments            []string
	EnvironmentVariables []KeyValuePair
	InputDepSetIds       []depsetId
	Mnemonic             string
	OutputIds            []artifactId
	TemplateContent      string
	Substitutions        []KeyValuePair
}

// actionGraphContainer contains relevant portions of Bazel's aquery proto, ActionGraphContainer.
// An aquery response from Bazel contains a single ActionGraphContainer proto.
type actionGraphContainer struct {
	Artifacts     []artifact
	Actions       []action
	DepSetOfFiles []depSetOfFiles
	PathFragments []pathFragment
}

// BuildStatement contains information to register a build statement corresponding (one to one)
// with a Bazel action from Bazel's action graph.
type BuildStatement struct {
	Command      string
	Depfile      *string
	OutputPaths  []string
	SymlinkPaths []string
	Env          []KeyValuePair
	Mnemonic     string

	// Inputs of this build statement, either as unexpanded depsets or expanded
	// input paths. There should be no overlap between these fields; an input
	// path should either be included as part of an unexpanded depset or a raw
	// input path string, but not both.
	InputDepsetHashes []string
	InputPaths        []string
}

// A helper type for aquery processing which facilitates retrieval of path IDs from their
// less readable Bazel structures (depset and path fragment).
type aqueryArtifactHandler struct {
	// Switches to true if any depset contains only `bazelToolsDependencySentinel`
	bazelToolsDependencySentinelNeeded bool
	// Maps depset id to AqueryDepset, a representation of depset which is
	// post-processed for middleman artifact handling, unhandled artifact
	// dropping, content hashing, etc.
	depsetIdToAqueryDepset map[depsetId]AqueryDepset
	// Maps content hash to AqueryDepset.
	depsetHashToAqueryDepset map[string]AqueryDepset

	// depsetIdToArtifactIdsCache is a memoization of depset flattening, because flattening
	// may be an expensive operation.
	depsetHashToArtifactPathsCache map[string][]string
	// Maps artifact ids to fully expanded paths.
	artifactIdToPath map[artifactId]string
}

// The tokens should be substituted with the value specified here, instead of the
// one returned in 'substitutions' of TemplateExpand action.
var templateActionOverriddenTokens = map[string]string{
	// Uses "python3" for %python_binary% instead of the value returned by aquery
	// which is "py3wrapper.sh". See removePy3wrapperScript.
	"%python_binary%": "python3",
}

// This pattern matches the MANIFEST file created for a py_binary target.
var manifestFilePattern = regexp.MustCompile(".*/.+\\.runfiles/MANIFEST$")

// The file name of py3wrapper.sh, which is used by py_binary targets.
const py3wrapperFileName = "/py3wrapper.sh"

// A file to be put into depsets that are otherwise empty
const bazelToolsDependencySentinel = "BAZEL_TOOLS_DEPENDENCY_SENTINEL"

func indexBy[K comparable, V any](values []V, keyFn func(v V) K) map[K]V {
	m := map[K]V{}
	for _, v := range values {
		m[keyFn(v)] = v
	}
	return m
}

func newAqueryHandler(aqueryResult actionGraphContainer) (*aqueryArtifactHandler, error) {
	pathFragments := indexBy(aqueryResult.PathFragments, func(pf pathFragment) pathFragmentId {
		return pf.Id
	})

	artifactIdToPath := map[artifactId]string{}
	for _, artifact := range aqueryResult.Artifacts {
		artifactPath, err := expandPathFragment(artifact.PathFragmentId, pathFragments)
		if err != nil {
			return nil, err
		}
		artifactIdToPath[artifact.Id] = artifactPath
	}

	// Map middleman artifact ContentHash to input artifact depset ID.
	// Middleman artifacts are treated as "substitute" artifacts for mixed builds. For example,
	// if we find a middleman action which has inputs [foo, bar], and output [baz_middleman], then,
	// for each other action which has input [baz_middleman], we add [foo, bar] to the inputs for
	// that action instead.
	middlemanIdToDepsetIds := map[artifactId][]depsetId{}
	for _, actionEntry := range aqueryResult.Actions {
		if actionEntry.Mnemonic == "Middleman" {
			for _, outputId := range actionEntry.OutputIds {
				middlemanIdToDepsetIds[outputId] = actionEntry.InputDepSetIds
			}
		}
	}

	depsetIdToDepset := indexBy(aqueryResult.DepSetOfFiles, func(d depSetOfFiles) depsetId {
		return d.Id
	})

	aqueryHandler := aqueryArtifactHandler{
		depsetIdToAqueryDepset:         map[depsetId]AqueryDepset{},
		depsetHashToAqueryDepset:       map[string]AqueryDepset{},
		depsetHashToArtifactPathsCache: map[string][]string{},
		artifactIdToPath:               artifactIdToPath,
	}

	// Validate and adjust aqueryResult.DepSetOfFiles values.
	for _, depset := range aqueryResult.DepSetOfFiles {
		_, err := aqueryHandler.populateDepsetMaps(depset, middlemanIdToDepsetIds, depsetIdToDepset)
		if err != nil {
			return nil, err
		}
	}

	return &aqueryHandler, nil
}

// Ensures that the handler's depsetIdToAqueryDepset map contains an entry for the given
// depset.
func (a *aqueryArtifactHandler) populateDepsetMaps(depset depSetOfFiles, middlemanIdToDepsetIds map[artifactId][]depsetId, depsetIdToDepset map[depsetId]depSetOfFiles) (AqueryDepset, error) {
	if aqueryDepset, containsDepset := a.depsetIdToAqueryDepset[depset.Id]; containsDepset {
		return aqueryDepset, nil
	}
	transitiveDepsetIds := depset.TransitiveDepSetIds
	var directArtifactPaths []string
	for _, artifactId := range depset.DirectArtifactIds {
		path, pathExists := a.artifactIdToPath[artifactId]
		if !pathExists {
			return AqueryDepset{}, fmt.Errorf("undefined input artifactId %d", artifactId)
		}
		// Filter out any inputs which are universally dropped, and swap middleman
		// artifacts with their corresponding depsets.
		if depsetsToUse, isMiddleman := middlemanIdToDepsetIds[artifactId]; isMiddleman {
			// Swap middleman artifacts with their corresponding depsets and drop the middleman artifacts.
			transitiveDepsetIds = append(transitiveDepsetIds, depsetsToUse...)
		} else if strings.HasSuffix(path, py3wrapperFileName) ||
			manifestFilePattern.MatchString(path) ||
			strings.HasPrefix(path, "../bazel_tools") {
			// Drop these artifacts.
			// See go/python-binary-host-mixed-build for more details.
			// 1) For py3wrapper.sh, there is no action for creating py3wrapper.sh in the aquery output of
			// Bazel py_binary targets, so there is no Ninja build statements generated for creating it.
			// 2) For MANIFEST file, SourceSymlinkManifest action is in aquery output of Bazel py_binary targets,
			// but it doesn't contain sufficient information so no Ninja build statements are generated
			// for creating it.
			// So in mixed build mode, when these two are used as input of some Ninja build statement,
			// since there is no build statement to create them, they should be removed from input paths.
			// TODO(b/197135294): Clean up this custom runfiles handling logic when
			// SourceSymlinkManifest and SymlinkTree actions are supported.
			// 3) ../bazel_tools: they have MODIFY timestamp 10years in the future and would cause the
			// containing depset to always be considered newer than their outputs.
		} else {
			directArtifactPaths = append(directArtifactPaths, path)
		}
	}

	var childDepsetHashes []string
	for _, childDepsetId := range transitiveDepsetIds {
		childDepset, exists := depsetIdToDepset[childDepsetId]
		if !exists {
			return AqueryDepset{}, fmt.Errorf("undefined input depsetId %d (referenced by depsetId %d)", childDepsetId, depset.Id)
		}
		childAqueryDepset, err := a.populateDepsetMaps(childDepset, middlemanIdToDepsetIds, depsetIdToDepset)
		if err != nil {
			return AqueryDepset{}, err
		}
		childDepsetHashes = append(childDepsetHashes, childAqueryDepset.ContentHash)
	}
	if len(directArtifactPaths) == 0 && len(childDepsetHashes) == 0 {
		// We could omit this depset altogether but that requires cleanup on
		// transitive dependents.
		// As a simpler alternative, we use this sentinel file as a dependency.
		directArtifactPaths = append(directArtifactPaths, bazelToolsDependencySentinel)
		a.bazelToolsDependencySentinelNeeded = true
	}
	aqueryDepset := AqueryDepset{
		ContentHash:            depsetContentHash(directArtifactPaths, childDepsetHashes),
		DirectArtifacts:        directArtifactPaths,
		TransitiveDepSetHashes: childDepsetHashes,
	}
	a.depsetIdToAqueryDepset[depset.Id] = aqueryDepset
	a.depsetHashToAqueryDepset[aqueryDepset.ContentHash] = aqueryDepset
	return aqueryDepset, nil
}

// getInputPaths flattens the depsets of the given IDs and returns all transitive
// input paths contained in these depsets.
// This is a potentially expensive operation, and should not be invoked except
// for actions which need specialized input handling.
func (a *aqueryArtifactHandler) getInputPaths(depsetIds []depsetId) ([]string, error) {
	var inputPaths []string

	for _, inputDepSetId := range depsetIds {
		depset := a.depsetIdToAqueryDepset[inputDepSetId]
		inputArtifacts, err := a.artifactPathsFromDepsetHash(depset.ContentHash)
		if err != nil {
			return nil, err
		}
		for _, inputPath := range inputArtifacts {
			inputPaths = append(inputPaths, inputPath)
		}
	}

	return inputPaths, nil
}

func (a *aqueryArtifactHandler) artifactPathsFromDepsetHash(depsetHash string) ([]string, error) {
	if result, exists := a.depsetHashToArtifactPathsCache[depsetHash]; exists {
		return result, nil
	}
	if depset, exists := a.depsetHashToAqueryDepset[depsetHash]; exists {
		result := depset.DirectArtifacts
		for _, childHash := range depset.TransitiveDepSetHashes {
			childArtifactIds, err := a.artifactPathsFromDepsetHash(childHash)
			if err != nil {
				return nil, err
			}
			result = append(result, childArtifactIds...)
		}
		a.depsetHashToArtifactPathsCache[depsetHash] = result
		return result, nil
	} else {
		return nil, fmt.Errorf("undefined input depset hash %s", depsetHash)
	}
}

// AqueryBuildStatements returns a slice of BuildStatements and a slice of AqueryDepset
// which should be registered (and output to a ninja file) to correspond with Bazel's
// action graph, as described by the given action graph json proto.
// BuildStatements are one-to-one with actions in the given action graph, and AqueryDepsets
// are one-to-one with Bazel's depSetOfFiles objects.
func AqueryBuildStatements(aqueryJsonProto []byte) ([]BuildStatement, []AqueryDepset, error) {
	var aqueryResult actionGraphContainer
	err := json.Unmarshal(aqueryJsonProto, &aqueryResult)
	if err != nil {
		return nil, nil, err
	}
	aqueryHandler, err := newAqueryHandler(aqueryResult)
	if err != nil {
		return nil, nil, err
	}

	var buildStatements []BuildStatement
	if aqueryHandler.bazelToolsDependencySentinelNeeded {
		buildStatements = append(buildStatements, BuildStatement{
			Command:     fmt.Sprintf("touch '%s'", bazelToolsDependencySentinel),
			OutputPaths: []string{bazelToolsDependencySentinel},
			Mnemonic:    bazelToolsDependencySentinel,
		})
	}

	for _, actionEntry := range aqueryResult.Actions {
		if shouldSkipAction(actionEntry) {
			continue
		}

		var buildStatement BuildStatement
		if isSymlinkAction(actionEntry) {
			buildStatement, err = aqueryHandler.symlinkActionBuildStatement(actionEntry)
		} else if isTemplateExpandAction(actionEntry) && len(actionEntry.Arguments) < 1 {
			buildStatement, err = aqueryHandler.templateExpandActionBuildStatement(actionEntry)
		} else if isPythonZipperAction(actionEntry) {
			buildStatement, err = aqueryHandler.pythonZipperActionBuildStatement(actionEntry, buildStatements)
		} else if len(actionEntry.Arguments) < 1 {
			return nil, nil, fmt.Errorf("received action with no command: [%s]", actionEntry.Mnemonic)
		} else {
			buildStatement, err = aqueryHandler.normalActionBuildStatement(actionEntry)
		}

		if err != nil {
			return nil, nil, err
		}
		buildStatements = append(buildStatements, buildStatement)
	}

	depsetsByHash := map[string]AqueryDepset{}
	var depsets []AqueryDepset
	for _, aqueryDepset := range aqueryHandler.depsetIdToAqueryDepset {
		if prevEntry, hasKey := depsetsByHash[aqueryDepset.ContentHash]; hasKey {
			// Two depsets collide on hash. Ensure that their contents are identical.
			if !reflect.DeepEqual(aqueryDepset, prevEntry) {
				return nil, nil, fmt.Errorf("two different depsets have the same hash: %v, %v", prevEntry, aqueryDepset)
			}
		} else {
			depsetsByHash[aqueryDepset.ContentHash] = aqueryDepset
			depsets = append(depsets, aqueryDepset)
		}
	}

	// Build Statements and depsets must be sorted by their content hash to
	// preserve determinism between builds (this will result in consistent ninja file
	// output). Note they are not sorted by their original IDs nor their Bazel ordering,
	// as Bazel gives nondeterministic ordering / identifiers in aquery responses.
	sort.Slice(buildStatements, func(i, j int) bool {
		// For build statements, compare output lists. In Bazel, each output file
		// may only have one action which generates it, so this will provide
		// a deterministic ordering.
		outputs_i := buildStatements[i].OutputPaths
		outputs_j := buildStatements[j].OutputPaths
		if len(outputs_i) != len(outputs_j) {
			return len(outputs_i) < len(outputs_j)
		}
		if len(outputs_i) == 0 {
			// No outputs for these actions, so compare commands.
			return buildStatements[i].Command < buildStatements[j].Command
		}
		// There may be multiple outputs, but the output ordering is deterministic.
		return outputs_i[0] < outputs_j[0]
	})
	sort.Slice(depsets, func(i, j int) bool {
		return depsets[i].ContentHash < depsets[j].ContentHash
	})
	return buildStatements, depsets, nil
}

// depsetContentHash computes and returns a SHA256 checksum of the contents of
// the given depset. This content hash may serve as the depset's identifier.
// Using a content hash for an identifier is superior for determinism. (For example,
// using an integer identifier which depends on the order in which the depsets are
// created would result in nondeterministic depset IDs.)
func depsetContentHash(directPaths []string, transitiveDepsetHashes []string) string {
	h := sha256.New()
	// Use newline as delimiter, as paths cannot contain newline.
	h.Write([]byte(strings.Join(directPaths, "\n")))
	h.Write([]byte(strings.Join(transitiveDepsetHashes, "")))
	fullHash := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return fullHash
}

func (a *aqueryArtifactHandler) depsetContentHashes(inputDepsetIds []depsetId) ([]string, error) {
	var hashes []string
	for _, depsetId := range inputDepsetIds {
		if aqueryDepset, exists := a.depsetIdToAqueryDepset[depsetId]; !exists {
			return nil, fmt.Errorf("undefined input depsetId %d", depsetId)
		} else {
			hashes = append(hashes, aqueryDepset.ContentHash)
		}
	}
	return hashes, nil
}

func (a *aqueryArtifactHandler) normalActionBuildStatement(actionEntry action) (BuildStatement, error) {
	command := strings.Join(proptools.ShellEscapeListIncludingSpaces(actionEntry.Arguments), " ")
	inputDepsetHashes, err := a.depsetContentHashes(actionEntry.InputDepSetIds)
	if err != nil {
		return BuildStatement{}, err
	}
	outputPaths, depfile, err := a.getOutputPaths(actionEntry)
	if err != nil {
		return BuildStatement{}, err
	}

	buildStatement := BuildStatement{
		Command:           command,
		Depfile:           depfile,
		OutputPaths:       outputPaths,
		InputDepsetHashes: inputDepsetHashes,
		Env:               actionEntry.EnvironmentVariables,
		Mnemonic:          actionEntry.Mnemonic,
	}
	return buildStatement, nil
}

func (a *aqueryArtifactHandler) pythonZipperActionBuildStatement(actionEntry action, prevBuildStatements []BuildStatement) (BuildStatement, error) {
	inputPaths, err := a.getInputPaths(actionEntry.InputDepSetIds)
	if err != nil {
		return BuildStatement{}, err
	}
	outputPaths, depfile, err := a.getOutputPaths(actionEntry)
	if err != nil {
		return BuildStatement{}, err
	}

	if len(inputPaths) < 1 || len(outputPaths) != 1 {
		return BuildStatement{}, fmt.Errorf("Expect 1+ input and 1 output to python zipper action, got: input %q, output %q", inputPaths, outputPaths)
	}
	command := strings.Join(proptools.ShellEscapeListIncludingSpaces(actionEntry.Arguments), " ")
	inputPaths, command = removePy3wrapperScript(inputPaths, command)
	command = addCommandForPyBinaryRunfilesDir(command, outputPaths[0])
	// Add the python zip file as input of the corresponding python binary stub script in Ninja build statements.
	// In Ninja build statements, the outputs of dependents of a python binary have python binary stub script as input,
	// which is not sufficient without the python zip file from which runfiles directory is created for py_binary.
	//
	// The following logic relies on that Bazel aquery output returns actions in the order that
	// PythonZipper is after TemplateAction of creating Python binary stub script. If later Bazel doesn't return actions
	// in that order, the following logic might not find the build statement generated for Python binary
	// stub script and the build might fail. So the check of pyBinaryFound is added to help debug in case later Bazel might change aquery output.
	// See go/python-binary-host-mixed-build for more details.
	pythonZipFilePath := outputPaths[0]
	pyBinaryFound := false
	for i := range prevBuildStatements {
		if len(prevBuildStatements[i].OutputPaths) == 1 && prevBuildStatements[i].OutputPaths[0]+".zip" == pythonZipFilePath {
			prevBuildStatements[i].InputPaths = append(prevBuildStatements[i].InputPaths, pythonZipFilePath)
			pyBinaryFound = true
		}
	}
	if !pyBinaryFound {
		return BuildStatement{}, fmt.Errorf("Could not find the correspondinging Python binary stub script of PythonZipper: %q", outputPaths)
	}

	buildStatement := BuildStatement{
		Command:     command,
		Depfile:     depfile,
		OutputPaths: outputPaths,
		InputPaths:  inputPaths,
		Env:         actionEntry.EnvironmentVariables,
		Mnemonic:    actionEntry.Mnemonic,
	}
	return buildStatement, nil
}

func (a *aqueryArtifactHandler) templateExpandActionBuildStatement(actionEntry action) (BuildStatement, error) {
	outputPaths, depfile, err := a.getOutputPaths(actionEntry)
	if err != nil {
		return BuildStatement{}, err
	}
	if len(outputPaths) != 1 {
		return BuildStatement{}, fmt.Errorf("Expect 1 output to template expand action, got: output %q", outputPaths)
	}
	expandedTemplateContent := expandTemplateContent(actionEntry)
	// The expandedTemplateContent is escaped for being used in double quotes and shell unescape,
	// and the new line characters (\n) are also changed to \\n which avoids some Ninja escape on \n, which might
	// change \n to space and mess up the format of Python programs.
	// sed is used to convert \\n back to \n before saving to output file.
	// See go/python-binary-host-mixed-build for more details.
	command := fmt.Sprintf(`/bin/bash -c 'echo "%[1]s" | sed "s/\\\\n/\\n/g" > %[2]s && chmod a+x %[2]s'`,
		escapeCommandlineArgument(expandedTemplateContent), outputPaths[0])
	inputDepsetHashes, err := a.depsetContentHashes(actionEntry.InputDepSetIds)
	if err != nil {
		return BuildStatement{}, err
	}

	buildStatement := BuildStatement{
		Command:           command,
		Depfile:           depfile,
		OutputPaths:       outputPaths,
		InputDepsetHashes: inputDepsetHashes,
		Env:               actionEntry.EnvironmentVariables,
		Mnemonic:          actionEntry.Mnemonic,
	}
	return buildStatement, nil
}

func (a *aqueryArtifactHandler) symlinkActionBuildStatement(actionEntry action) (BuildStatement, error) {
	outputPaths, depfile, err := a.getOutputPaths(actionEntry)
	if err != nil {
		return BuildStatement{}, err
	}

	inputPaths, err := a.getInputPaths(actionEntry.InputDepSetIds)
	if err != nil {
		return BuildStatement{}, err
	}
	if len(inputPaths) != 1 || len(outputPaths) != 1 {
		return BuildStatement{}, fmt.Errorf("Expect 1 input and 1 output to symlink action, got: input %q, output %q", inputPaths, outputPaths)
	}
	out := outputPaths[0]
	outDir := proptools.ShellEscapeIncludingSpaces(filepath.Dir(out))
	out = proptools.ShellEscapeIncludingSpaces(out)
	in := filepath.Join("$PWD", proptools.ShellEscapeIncludingSpaces(inputPaths[0]))
	// Use absolute paths, because some soong actions don't play well with relative paths (for example, `cp -d`).
	command := fmt.Sprintf("mkdir -p %[1]s && rm -f %[2]s && ln -sf %[3]s %[2]s", outDir, out, in)
	symlinkPaths := outputPaths[:]

	buildStatement := BuildStatement{
		Command:      command,
		Depfile:      depfile,
		OutputPaths:  outputPaths,
		InputPaths:   inputPaths,
		Env:          actionEntry.EnvironmentVariables,
		Mnemonic:     actionEntry.Mnemonic,
		SymlinkPaths: symlinkPaths,
	}
	return buildStatement, nil
}

func (a *aqueryArtifactHandler) getOutputPaths(actionEntry action) (outputPaths []string, depfile *string, err error) {
	for _, outputId := range actionEntry.OutputIds {
		outputPath, exists := a.artifactIdToPath[outputId]
		if !exists {
			err = fmt.Errorf("undefined outputId %d", outputId)
			return
		}
		ext := filepath.Ext(outputPath)
		if ext == ".d" {
			if depfile != nil {
				err = fmt.Errorf("found multiple potential depfiles %q, %q", *depfile, outputPath)
				return
			} else {
				depfile = &outputPath
			}
		} else {
			outputPaths = append(outputPaths, outputPath)
		}
	}
	return
}

// expandTemplateContent substitutes the tokens in a template.
func expandTemplateContent(actionEntry action) string {
	replacerString := []string{}
	for _, pair := range actionEntry.Substitutions {
		value := pair.Value
		if val, ok := templateActionOverriddenTokens[pair.Key]; ok {
			value = val
		}
		replacerString = append(replacerString, pair.Key, value)
	}
	replacer := strings.NewReplacer(replacerString...)
	return replacer.Replace(actionEntry.TemplateContent)
}

func escapeCommandlineArgument(str string) string {
	// \->\\, $->\$, `->\`, "->\", \n->\\n, '->'"'"'
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`$`, `\$`,
		"`", "\\`",
		`"`, `\"`,
		"\n", "\\n",
		`'`, `'"'"'`,
	)
	return replacer.Replace(str)
}

// removePy3wrapperScript removes py3wrapper.sh from the input paths and command of the action of
// creating python zip file in mixed build mode. py3wrapper.sh is returned as input by aquery but
// there is no action returned by aquery for creating it. So in mixed build "python3" is used
// as the PYTHON_BINARY in python binary stub script, and py3wrapper.sh is not needed and should be
// removed from input paths and command of creating python zip file.
// See go/python-binary-host-mixed-build for more details.
// TODO(b/205879240) remove this after py3wrapper.sh could be created in the mixed build mode.
func removePy3wrapperScript(inputPaths []string, command string) (newInputPaths []string, newCommand string) {
	// Remove from inputs
	var filteredInputPaths []string
	for _, path := range inputPaths {
		if !strings.HasSuffix(path, py3wrapperFileName) {
			filteredInputPaths = append(filteredInputPaths, path)
		}
	}
	newInputPaths = filteredInputPaths

	// Remove from command line
	var re = regexp.MustCompile(`\S*` + py3wrapperFileName)
	newCommand = re.ReplaceAllString(command, "")
	return
}

// addCommandForPyBinaryRunfilesDir adds commands creating python binary runfiles directory.
// runfiles directory is created by using MANIFEST file and MANIFEST file is the output of
// SourceSymlinkManifest action is in aquery output of Bazel py_binary targets,
// but since SourceSymlinkManifest doesn't contain sufficient information
// so MANIFEST file could not be created, which also blocks the creation of runfiles directory.
// See go/python-binary-host-mixed-build for more details.
// TODO(b/197135294) create runfiles directory from MANIFEST file once it can be created from SourceSymlinkManifest action.
func addCommandForPyBinaryRunfilesDir(oldCommand string, zipFilePath string) string {
	// Unzip the zip file, zipFilePath looks like <python_binary>.zip
	runfilesDirName := zipFilePath[0:len(zipFilePath)-4] + ".runfiles"
	command := fmt.Sprintf("%s x %s -d %s", "../bazel_tools/tools/zip/zipper/zipper", zipFilePath, runfilesDirName)
	// Create a symbolic link in <python_binary>.runfiles/, which is the expected structure
	// when running the python binary stub script.
	command += fmt.Sprintf(" && ln -sf runfiles/__main__ %s", runfilesDirName)
	return oldCommand + " && " + command
}

func isSymlinkAction(a action) bool {
	return a.Mnemonic == "Symlink" || a.Mnemonic == "SolibSymlink" || a.Mnemonic == "ExecutableSymlink"
}

func isTemplateExpandAction(a action) bool {
	return a.Mnemonic == "TemplateExpand"
}

func isPythonZipperAction(a action) bool {
	return a.Mnemonic == "PythonZipper"
}

func shouldSkipAction(a action) bool {
	// TODO(b/180945121): Handle complex symlink actions.
	if a.Mnemonic == "SymlinkTree" || a.Mnemonic == "SourceSymlinkManifest" {
		return true
	}
	// Middleman actions are not handled like other actions; they are handled separately as a
	// preparatory step so that their inputs may be relayed to actions depending on middleman
	// artifacts.
	if a.Mnemonic == "Middleman" {
		return true
	}
	// Skip "Fail" actions, which are placeholder actions designed to always fail.
	if a.Mnemonic == "Fail" {
		return true
	}
	// TODO(b/180946980): Handle FileWrite. The aquery proto currently contains no information
	// about the contents that are written.
	if a.Mnemonic == "FileWrite" {
		return true
	}
	if a.Mnemonic == "BaselineCoverage" {
		return true
	}
	return false
}

func expandPathFragment(id pathFragmentId, pathFragmentsMap map[pathFragmentId]pathFragment) (string, error) {
	var labels []string
	currId := id
	// Only positive IDs are valid for path fragments. An ID of zero indicates a terminal node.
	for currId > 0 {
		currFragment, ok := pathFragmentsMap[currId]
		if !ok {
			return "", fmt.Errorf("undefined path fragment id %d", currId)
		}
		labels = append([]string{currFragment.Label}, labels...)
		if currId == currFragment.ParentId {
			return "", fmt.Errorf("Fragment cannot refer to itself as parent %#v", currFragment)
		}
		currId = currFragment.ParentId
	}
	return filepath.Join(labels...), nil
}
