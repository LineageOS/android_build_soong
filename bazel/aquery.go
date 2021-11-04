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
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/proptools"
)

// artifact contains relevant portions of Bazel's aquery proto, Artifact.
// Represents a single artifact, whether it's a source file or a derived output file.
type artifact struct {
	Id             int
	PathFragmentId int
}

type pathFragment struct {
	Id       int
	Label    string
	ParentId int
}

// KeyValuePair represents Bazel's aquery proto, KeyValuePair.
type KeyValuePair struct {
	Key   string
	Value string
}

// depSetOfFiles contains relevant portions of Bazel's aquery proto, DepSetOfFiles.
// Represents a data structure containing one or more files. Depsets in Bazel are an efficient
// data structure for storing large numbers of file paths.
type depSetOfFiles struct {
	Id                  int
	DirectArtifactIds   []int
	TransitiveDepSetIds []int
}

// action contains relevant portions of Bazel's aquery proto, Action.
// Represents a single command line invocation in the Bazel build graph.
type action struct {
	Arguments            []string
	EnvironmentVariables []KeyValuePair
	InputDepSetIds       []int
	Mnemonic             string
	OutputIds            []int
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
	InputPaths   []string
	SymlinkPaths []string
	Env          []KeyValuePair
	Mnemonic     string
}

// A helper type for aquery processing which facilitates retrieval of path IDs from their
// less readable Bazel structures (depset and path fragment).
type aqueryArtifactHandler struct {
	// Maps middleman artifact Id to input artifact depset ID.
	// Middleman artifacts are treated as "substitute" artifacts for mixed builds. For example,
	// if we find a middleman action which has outputs [foo, bar], and output [baz_middleman], then,
	// for each other action which has input [baz_middleman], we add [foo, bar] to the inputs for
	// that action instead.
	middlemanIdToDepsetIds map[int][]int
	// Maps depset Id to depset struct.
	depsetIdToDepset map[int]depSetOfFiles
	// depsetIdToArtifactIdsCache is a memoization of depset flattening, because flattening
	// may be an expensive operation.
	depsetIdToArtifactIdsCache map[int][]int
	// Maps artifact Id to fully expanded path.
	artifactIdToPath map[int]string
}

func newAqueryHandler(aqueryResult actionGraphContainer) (*aqueryArtifactHandler, error) {
	pathFragments := map[int]pathFragment{}
	for _, pathFragment := range aqueryResult.PathFragments {
		pathFragments[pathFragment.Id] = pathFragment
	}

	artifactIdToPath := map[int]string{}
	for _, artifact := range aqueryResult.Artifacts {
		artifactPath, err := expandPathFragment(artifact.PathFragmentId, pathFragments)
		if err != nil {
			return nil, err
		}
		artifactIdToPath[artifact.Id] = artifactPath
	}

	depsetIdToDepset := map[int]depSetOfFiles{}
	for _, depset := range aqueryResult.DepSetOfFiles {
		depsetIdToDepset[depset.Id] = depset
	}

	// Do a pass through all actions to identify which artifacts are middleman artifacts.
	middlemanIdToDepsetIds := map[int][]int{}
	for _, actionEntry := range aqueryResult.Actions {
		if actionEntry.Mnemonic == "Middleman" {
			for _, outputId := range actionEntry.OutputIds {
				middlemanIdToDepsetIds[outputId] = actionEntry.InputDepSetIds
			}
		}
	}
	return &aqueryArtifactHandler{
		middlemanIdToDepsetIds:     middlemanIdToDepsetIds,
		depsetIdToDepset:           depsetIdToDepset,
		depsetIdToArtifactIdsCache: map[int][]int{},
		artifactIdToPath:           artifactIdToPath,
	}, nil
}

func (a *aqueryArtifactHandler) getInputPaths(depsetIds []int) ([]string, error) {
	inputPaths := []string{}

	for _, inputDepSetId := range depsetIds {
		inputArtifacts, err := a.artifactIdsFromDepsetId(inputDepSetId)
		if err != nil {
			return nil, err
		}
		for _, inputId := range inputArtifacts {
			if middlemanInputDepsetIds, isMiddlemanArtifact := a.middlemanIdToDepsetIds[inputId]; isMiddlemanArtifact {
				// Add all inputs from middleman actions which created middleman artifacts which are
				// in the inputs for this action.
				swappedInputPaths, err := a.getInputPaths(middlemanInputDepsetIds)
				if err != nil {
					return nil, err
				}
				inputPaths = append(inputPaths, swappedInputPaths...)
			} else {
				inputPath, exists := a.artifactIdToPath[inputId]
				if !exists {
					return nil, fmt.Errorf("undefined input artifactId %d", inputId)
				}
				inputPaths = append(inputPaths, inputPath)
			}
		}
	}
	return inputPaths, nil
}

func (a *aqueryArtifactHandler) artifactIdsFromDepsetId(depsetId int) ([]int, error) {
	if result, exists := a.depsetIdToArtifactIdsCache[depsetId]; exists {
		return result, nil
	}
	if depset, exists := a.depsetIdToDepset[depsetId]; exists {
		result := depset.DirectArtifactIds
		for _, childId := range depset.TransitiveDepSetIds {
			childArtifactIds, err := a.artifactIdsFromDepsetId(childId)
			if err != nil {
				return nil, err
			}
			result = append(result, childArtifactIds...)
		}
		a.depsetIdToArtifactIdsCache[depsetId] = result
		return result, nil
	} else {
		return nil, fmt.Errorf("undefined input depsetId %d", depsetId)
	}
}

// AqueryBuildStatements returns an array of BuildStatements which should be registered (and output
// to a ninja file) to correspond one-to-one with the given action graph json proto (from a bazel
// aquery invocation).
func AqueryBuildStatements(aqueryJsonProto []byte) ([]BuildStatement, error) {
	buildStatements := []BuildStatement{}

	var aqueryResult actionGraphContainer
	err := json.Unmarshal(aqueryJsonProto, &aqueryResult)
	if err != nil {
		return nil, err
	}
	aqueryHandler, err := newAqueryHandler(aqueryResult)
	if err != nil {
		return nil, err
	}

	for _, actionEntry := range aqueryResult.Actions {
		if shouldSkipAction(actionEntry) {
			continue
		}
		outputPaths := []string{}
		var depfile *string
		for _, outputId := range actionEntry.OutputIds {
			outputPath, exists := aqueryHandler.artifactIdToPath[outputId]
			if !exists {
				return nil, fmt.Errorf("undefined outputId %d", outputId)
			}
			ext := filepath.Ext(outputPath)
			if ext == ".d" {
				if depfile != nil {
					return nil, fmt.Errorf("found multiple potential depfiles %q, %q", *depfile, outputPath)
				} else {
					depfile = &outputPath
				}
			} else {
				outputPaths = append(outputPaths, outputPath)
			}
		}
		inputPaths, err := aqueryHandler.getInputPaths(actionEntry.InputDepSetIds)
		if err != nil {
			return nil, err
		}

		buildStatement := BuildStatement{
			Command:     strings.Join(proptools.ShellEscapeList(actionEntry.Arguments), " "),
			Depfile:     depfile,
			OutputPaths: outputPaths,
			InputPaths:  inputPaths,
			Env:         actionEntry.EnvironmentVariables,
			Mnemonic:    actionEntry.Mnemonic,
		}

		if isSymlinkAction(actionEntry) {
			if len(inputPaths) != 1 || len(outputPaths) != 1 {
				return nil, fmt.Errorf("Expect 1 input and 1 output to symlink action, got: input %q, output %q", inputPaths, outputPaths)
			}
			out := outputPaths[0]
			outDir := proptools.ShellEscapeIncludingSpaces(filepath.Dir(out))
			out = proptools.ShellEscapeIncludingSpaces(out)
			in := filepath.Join("$PWD", proptools.ShellEscapeIncludingSpaces(inputPaths[0]))
			// Use absolute paths, because some soong actions don't play well with relative paths (for example, `cp -d`).
			buildStatement.Command = fmt.Sprintf("mkdir -p %[1]s && rm -f %[2]s && ln -sf %[3]s %[2]s", outDir, out, in)
			buildStatement.SymlinkPaths = outputPaths[:]
		} else if len(actionEntry.Arguments) < 1 {
			return nil, fmt.Errorf("received action with no command: [%v]", buildStatement)
		}
		buildStatements = append(buildStatements, buildStatement)
	}

	return buildStatements, nil
}

func isSymlinkAction(a action) bool {
	return a.Mnemonic == "Symlink" || a.Mnemonic == "SolibSymlink"
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
	return false
}

func expandPathFragment(id int, pathFragmentsMap map[int]pathFragment) (string, error) {
	labels := []string{}
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
