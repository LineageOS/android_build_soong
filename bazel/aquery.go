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
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	analysis_v2_proto "prebuilts/bazel/common/proto/analysis_v2"

	"github.com/google/blueprint/metrics"
	"github.com/google/blueprint/proptools"
	"google.golang.org/protobuf/proto"
)

type artifactId int
type depsetId int
type pathFragmentId int

// KeyValuePair represents Bazel's aquery proto, KeyValuePair.
type KeyValuePair struct {
	Key   string
	Value string
}

// AqueryDepset is a depset definition from Bazel's aquery response. This is
// akin to the `depSetOfFiles` in the response proto, except:
//   - direct artifacts are enumerated by full path instead of by ID
//   - it has a hash of the depset contents, instead of an int ID (for determinism)
//
// A depset is a data structure for efficient transitive handling of artifact
// paths. A single depset consists of one or more artifact paths and one or
// more "child" depsets.
type AqueryDepset struct {
	ContentHash            string
	DirectArtifacts        []string
	TransitiveDepSetHashes []string
}

// BuildStatement contains information to register a build statement corresponding (one to one)
// with a Bazel action from Bazel's action graph.
type BuildStatement struct {
	Command      string
	Depfile      *string
	OutputPaths  []string
	SymlinkPaths []string
	Env          []*analysis_v2_proto.KeyValuePair
	Mnemonic     string

	// Inputs of this build statement, either as unexpanded depsets or expanded
	// input paths. There should be no overlap between these fields; an input
	// path should either be included as part of an unexpanded depset or a raw
	// input path string, but not both.
	InputDepsetHashes []string
	InputPaths        []string
	FileContents      string
	// If ShouldRunInSbox is true, Soong will use sbox to created an isolated environment
	// and run the mixed build action there
	ShouldRunInSbox bool
	// A list of files to add as implicit deps to the outputs of this BuildStatement.
	// Unlike most properties in BuildStatement, these paths must be relative to the root of
	// the whole out/ folder, instead of relative to ctx.Config().BazelContext.OutputBase()
	ImplicitDeps []string
	IsExecutable bool
}

// A helper type for aquery processing which facilitates retrieval of path IDs from their
// less readable Bazel structures (depset and path fragment).
type aqueryArtifactHandler struct {
	// Maps depset id to AqueryDepset, a representation of depset which is
	// post-processed for middleman artifact handling, unhandled artifact
	// dropping, content hashing, etc.
	depsetIdToAqueryDepset map[depsetId]AqueryDepset
	emptyDepsetIds         map[depsetId]struct{}
	// Maps content hash to AqueryDepset.
	depsetHashToAqueryDepset map[string]AqueryDepset

	// depsetIdToArtifactIdsCache is a memoization of depset flattening, because flattening
	// may be an expensive operation.
	depsetHashToArtifactPathsCache sync.Map
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

const (
	middlemanMnemonic = "Middleman"
	// The file name of py3wrapper.sh, which is used by py_binary targets.
	py3wrapperFileName = "/py3wrapper.sh"
)

func indexBy[K comparable, V any](values []V, keyFn func(v V) K) map[K]V {
	m := map[K]V{}
	for _, v := range values {
		m[keyFn(v)] = v
	}
	return m
}

func newAqueryHandler(aqueryResult *analysis_v2_proto.ActionGraphContainer) (*aqueryArtifactHandler, error) {
	pathFragments := indexBy(aqueryResult.PathFragments, func(pf *analysis_v2_proto.PathFragment) pathFragmentId {
		return pathFragmentId(pf.Id)
	})

	artifactIdToPath := make(map[artifactId]string, len(aqueryResult.Artifacts))
	for _, artifact := range aqueryResult.Artifacts {
		artifactPath, err := expandPathFragment(pathFragmentId(artifact.PathFragmentId), pathFragments)
		if err != nil {
			return nil, err
		}
		if artifact.IsTreeArtifact &&
			!strings.HasPrefix(artifactPath, "bazel-out/io_bazel_rules_go/") &&
			!strings.HasPrefix(artifactPath, "bazel-out/rules_java_builtin/") {
			// Since we're using ninja as an executor, we can't use tree artifacts. Ninja only
			// considers a file/directory "dirty" when it's mtime changes. Directories' mtimes will
			// only change when a file in the directory is added/removed, but not when files in
			// the directory are changed, or when files in subdirectories are changed/added/removed.
			// Bazel handles this by walking the directory and generating a hash for it after the
			// action runs, which we would have to do as well if we wanted to support these
			// artifacts in mixed builds.
			//
			// However, there are some bazel built-in rules that use tree artifacts. Allow those,
			// but keep in mind that they'll have incrementality issues.
			return nil, fmt.Errorf("tree artifacts are currently not supported in mixed builds: " + artifactPath)
		}
		artifactIdToPath[artifactId(artifact.Id)] = artifactPath
	}

	// Map middleman artifact ContentHash to input artifact depset ID.
	// Middleman artifacts are treated as "substitute" artifacts for mixed builds. For example,
	// if we find a middleman action which has inputs [foo, bar], and output [baz_middleman], then,
	// for each other action which has input [baz_middleman], we add [foo, bar] to the inputs for
	// that action instead.
	middlemanIdToDepsetIds := map[artifactId][]uint32{}
	for _, actionEntry := range aqueryResult.Actions {
		if actionEntry.Mnemonic == middlemanMnemonic {
			for _, outputId := range actionEntry.OutputIds {
				middlemanIdToDepsetIds[artifactId(outputId)] = actionEntry.InputDepSetIds
			}
		}
	}

	depsetIdToDepset := indexBy(aqueryResult.DepSetOfFiles, func(d *analysis_v2_proto.DepSetOfFiles) depsetId {
		return depsetId(d.Id)
	})

	aqueryHandler := aqueryArtifactHandler{
		depsetIdToAqueryDepset:         map[depsetId]AqueryDepset{},
		depsetHashToAqueryDepset:       map[string]AqueryDepset{},
		depsetHashToArtifactPathsCache: sync.Map{},
		emptyDepsetIds:                 make(map[depsetId]struct{}, 0),
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
func (a *aqueryArtifactHandler) populateDepsetMaps(depset *analysis_v2_proto.DepSetOfFiles, middlemanIdToDepsetIds map[artifactId][]uint32, depsetIdToDepset map[depsetId]*analysis_v2_proto.DepSetOfFiles) (*AqueryDepset, error) {
	if aqueryDepset, containsDepset := a.depsetIdToAqueryDepset[depsetId(depset.Id)]; containsDepset {
		return &aqueryDepset, nil
	}
	transitiveDepsetIds := depset.TransitiveDepSetIds
	directArtifactPaths := make([]string, 0, len(depset.DirectArtifactIds))
	for _, id := range depset.DirectArtifactIds {
		aId := artifactId(id)
		path, pathExists := a.artifactIdToPath[aId]
		if !pathExists {
			return nil, fmt.Errorf("undefined input artifactId %d", aId)
		}
		// Filter out any inputs which are universally dropped, and swap middleman
		// artifacts with their corresponding depsets.
		if depsetsToUse, isMiddleman := middlemanIdToDepsetIds[aId]; isMiddleman {
			// Swap middleman artifacts with their corresponding depsets and drop the middleman artifacts.
			transitiveDepsetIds = append(transitiveDepsetIds, depsetsToUse...)
		} else if strings.HasSuffix(path, py3wrapperFileName) ||
			strings.HasPrefix(path, "../bazel_tools") {
			continue
			// Drop these artifacts.
			// See go/python-binary-host-mixed-build for more details.
			// 1) Drop py3wrapper.sh, just use python binary, the launcher script generated by the
			// TemplateExpandAction handles everything necessary to launch a Pythin application.
			// 2) ../bazel_tools: they have MODIFY timestamp 10years in the future and would cause the
			// containing depset to always be considered newer than their outputs.
		} else {
			directArtifactPaths = append(directArtifactPaths, path)
		}
	}

	childDepsetHashes := make([]string, 0, len(transitiveDepsetIds))
	for _, id := range transitiveDepsetIds {
		childDepsetId := depsetId(id)
		childDepset, exists := depsetIdToDepset[childDepsetId]
		if !exists {
			if _, empty := a.emptyDepsetIds[childDepsetId]; empty {
				continue
			} else {
				return nil, fmt.Errorf("undefined input depsetId %d (referenced by depsetId %d)", childDepsetId, depset.Id)
			}
		}
		if childAqueryDepset, err := a.populateDepsetMaps(childDepset, middlemanIdToDepsetIds, depsetIdToDepset); err != nil {
			return nil, err
		} else if childAqueryDepset == nil {
			continue
		} else {
			childDepsetHashes = append(childDepsetHashes, childAqueryDepset.ContentHash)
		}
	}
	if len(directArtifactPaths) == 0 && len(childDepsetHashes) == 0 {
		a.emptyDepsetIds[depsetId(depset.Id)] = struct{}{}
		return nil, nil
	}
	aqueryDepset := AqueryDepset{
		ContentHash:            depsetContentHash(directArtifactPaths, childDepsetHashes),
		DirectArtifacts:        directArtifactPaths,
		TransitiveDepSetHashes: childDepsetHashes,
	}
	a.depsetIdToAqueryDepset[depsetId(depset.Id)] = aqueryDepset
	a.depsetHashToAqueryDepset[aqueryDepset.ContentHash] = aqueryDepset
	return &aqueryDepset, nil
}

// getInputPaths flattens the depsets of the given IDs and returns all transitive
// input paths contained in these depsets.
// This is a potentially expensive operation, and should not be invoked except
// for actions which need specialized input handling.
func (a *aqueryArtifactHandler) getInputPaths(depsetIds []uint32) ([]string, error) {
	var inputPaths []string

	for _, id := range depsetIds {
		inputDepSetId := depsetId(id)
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
	if result, exists := a.depsetHashToArtifactPathsCache.Load(depsetHash); exists {
		return result.([]string), nil
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
		a.depsetHashToArtifactPathsCache.Store(depsetHash, result)
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
func AqueryBuildStatements(aqueryJsonProto []byte, eventHandler *metrics.EventHandler) ([]*BuildStatement, []AqueryDepset, error) {
	aqueryProto := &analysis_v2_proto.ActionGraphContainer{}
	err := proto.Unmarshal(aqueryJsonProto, aqueryProto)
	if err != nil {
		return nil, nil, err
	}

	var aqueryHandler *aqueryArtifactHandler
	{
		eventHandler.Begin("init_handler")
		defer eventHandler.End("init_handler")
		aqueryHandler, err = newAqueryHandler(aqueryProto)
		if err != nil {
			return nil, nil, err
		}
	}

	// allocate both length and capacity so each goroutine can write to an index independently without
	// any need for synchronization for slice access.
	buildStatements := make([]*BuildStatement, len(aqueryProto.Actions))
	{
		eventHandler.Begin("build_statements")
		defer eventHandler.End("build_statements")
		wg := sync.WaitGroup{}
		var errOnce sync.Once
		id2targets := make(map[uint32]string, len(aqueryProto.Targets))
		for _, t := range aqueryProto.Targets {
			id2targets[t.GetId()] = t.GetLabel()
		}
		for i, actionEntry := range aqueryProto.Actions {
			wg.Add(1)
			go func(i int, actionEntry *analysis_v2_proto.Action) {
				if strings.HasPrefix(id2targets[actionEntry.TargetId], "@bazel_tools//") {
					// bazel_tools are removed depsets in `populateDepsetMaps()` so skipping
					// conversion to build statements as well
					buildStatements[i] = nil
				} else if buildStatement, aErr := aqueryHandler.actionToBuildStatement(actionEntry); aErr != nil {
					errOnce.Do(func() {
						aErr = fmt.Errorf("%s: [%s] [%s]", aErr.Error(), actionEntry.GetMnemonic(), id2targets[actionEntry.TargetId])
						err = aErr
					})
				} else {
					// set build statement at an index rather than appending such that each goroutine does not
					// impact other goroutines
					buildStatements[i] = buildStatement
				}
				wg.Done()
			}(i, actionEntry)
		}
		wg.Wait()
	}
	if err != nil {
		return nil, nil, err
	}

	depsetsByHash := map[string]AqueryDepset{}
	depsets := make([]AqueryDepset, 0, len(aqueryHandler.depsetIdToAqueryDepset))
	{
		eventHandler.Begin("depsets")
		defer eventHandler.End("depsets")
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
	}

	eventHandler.Do("build_statement_sort", func() {
		// Build Statements and depsets must be sorted by their content hash to
		// preserve determinism between builds (this will result in consistent ninja file
		// output). Note they are not sorted by their original IDs nor their Bazel ordering,
		// as Bazel gives nondeterministic ordering / identifiers in aquery responses.
		sort.Slice(buildStatements, func(i, j int) bool {
			// Sort all nil statements to the end of the slice
			if buildStatements[i] == nil {
				return false
			} else if buildStatements[j] == nil {
				return true
			}
			//For build statements, compare output lists. In Bazel, each output file
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
	})
	eventHandler.Do("depset_sort", func() {
		sort.Slice(depsets, func(i, j int) bool {
			return depsets[i].ContentHash < depsets[j].ContentHash
		})
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

func (a *aqueryArtifactHandler) depsetContentHashes(inputDepsetIds []uint32) ([]string, error) {
	var hashes []string
	for _, id := range inputDepsetIds {
		dId := depsetId(id)
		if aqueryDepset, exists := a.depsetIdToAqueryDepset[dId]; !exists {
			if _, empty := a.emptyDepsetIds[dId]; !empty {
				return nil, fmt.Errorf("undefined (not even empty) input depsetId %d", dId)
			}
		} else {
			hashes = append(hashes, aqueryDepset.ContentHash)
		}
	}
	return hashes, nil
}

// escapes the args received from aquery and creates a command string
func commandString(actionEntry *analysis_v2_proto.Action) string {
	argsEscaped := make([]string, len(actionEntry.Arguments))
	for i, arg := range actionEntry.Arguments {
		if arg == "" {
			// If this is an empty string, add ''
			// And not
			// 1. (literal empty)
			// 2. `''\'''\'''` (escaped version of '')
			//
			// If we had used (1), then this would appear as a whitespace when we strings.Join
			argsEscaped[i] = "''"
		} else {
			argsEscaped[i] = proptools.ShellEscapeIncludingSpaces(arg)
		}
	}
	return strings.Join(argsEscaped, " ")
}

func (a *aqueryArtifactHandler) normalActionBuildStatement(actionEntry *analysis_v2_proto.Action) (*BuildStatement, error) {
	command := commandString(actionEntry)
	inputDepsetHashes, err := a.depsetContentHashes(actionEntry.InputDepSetIds)
	if err != nil {
		return nil, err
	}
	outputPaths, depfile, err := a.getOutputPaths(actionEntry)
	if err != nil {
		return nil, err
	}

	buildStatement := &BuildStatement{
		Command:           command,
		Depfile:           depfile,
		OutputPaths:       outputPaths,
		InputDepsetHashes: inputDepsetHashes,
		Env:               actionEntry.EnvironmentVariables,
		Mnemonic:          actionEntry.Mnemonic,
	}
	if buildStatement.Mnemonic == "GoToolchainBinaryBuild" {
		// Unlike b's execution root, mixed build execution root contains a symlink to prebuilts/go
		// This causes issues for `GOCACHE=$(mktemp -d) go build ...`
		// To prevent this, sandbox this action in mixed builds as well
		buildStatement.ShouldRunInSbox = true
	}
	return buildStatement, nil
}

func (a *aqueryArtifactHandler) templateExpandActionBuildStatement(actionEntry *analysis_v2_proto.Action) (*BuildStatement, error) {
	outputPaths, depfile, err := a.getOutputPaths(actionEntry)
	if err != nil {
		return nil, err
	}
	if len(outputPaths) != 1 {
		return nil, fmt.Errorf("Expect 1 output to template expand action, got: output %q", outputPaths)
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
		return nil, err
	}

	buildStatement := &BuildStatement{
		Command:           command,
		Depfile:           depfile,
		OutputPaths:       outputPaths,
		InputDepsetHashes: inputDepsetHashes,
		Env:               actionEntry.EnvironmentVariables,
		Mnemonic:          actionEntry.Mnemonic,
	}
	return buildStatement, nil
}

func (a *aqueryArtifactHandler) fileWriteActionBuildStatement(actionEntry *analysis_v2_proto.Action) (*BuildStatement, error) {
	outputPaths, _, err := a.getOutputPaths(actionEntry)
	var depsetHashes []string
	if err == nil {
		depsetHashes, err = a.depsetContentHashes(actionEntry.InputDepSetIds)
	}
	if err != nil {
		return nil, err
	}
	return &BuildStatement{
		Depfile:           nil,
		OutputPaths:       outputPaths,
		Env:               actionEntry.EnvironmentVariables,
		Mnemonic:          actionEntry.Mnemonic,
		InputDepsetHashes: depsetHashes,
		FileContents:      actionEntry.FileContents,
		IsExecutable:      actionEntry.IsExecutable,
	}, nil
}

func (a *aqueryArtifactHandler) symlinkTreeActionBuildStatement(actionEntry *analysis_v2_proto.Action) (*BuildStatement, error) {
	outputPaths, _, err := a.getOutputPaths(actionEntry)
	if err != nil {
		return nil, err
	}
	inputPaths, err := a.getInputPaths(actionEntry.InputDepSetIds)
	if err != nil {
		return nil, err
	}
	if len(inputPaths) != 1 || len(outputPaths) != 1 {
		return nil, fmt.Errorf("Expect 1 input and 1 output to symlink action, got: input %q, output %q", inputPaths, outputPaths)
	}
	// The actual command is generated in bazelSingleton.GenerateBuildActions
	return &BuildStatement{
		Depfile:     nil,
		OutputPaths: outputPaths,
		Env:         actionEntry.EnvironmentVariables,
		Mnemonic:    actionEntry.Mnemonic,
		InputPaths:  inputPaths,
	}, nil
}

type bazelSandwichJson struct {
	Target         string   `json:"target"`
	DependOnTarget *bool    `json:"depend_on_target,omitempty"`
	ImplicitDeps   []string `json:"implicit_deps"`
}

func (a *aqueryArtifactHandler) unresolvedSymlinkActionBuildStatement(actionEntry *analysis_v2_proto.Action) (*BuildStatement, error) {
	outputPaths, depfile, err := a.getOutputPaths(actionEntry)
	if err != nil {
		return nil, err
	}
	if len(actionEntry.InputDepSetIds) != 0 || len(outputPaths) != 1 {
		return nil, fmt.Errorf("expected 0 inputs and 1 output to symlink action, got: input %q, output %q", actionEntry.InputDepSetIds, outputPaths)
	}
	target := actionEntry.UnresolvedSymlinkTarget
	if target == "" {
		return nil, fmt.Errorf("expected an unresolved_symlink_target, but didn't get one")
	}
	if filepath.Clean(target) != target {
		return nil, fmt.Errorf("expected %q, got %q", filepath.Clean(target), target)
	}
	if strings.HasPrefix(target, "/") {
		return nil, fmt.Errorf("no absolute symlinks allowed: %s", target)
	}

	out := outputPaths[0]
	outDir := filepath.Dir(out)
	var implicitDeps []string
	if strings.HasPrefix(target, "bazel_sandwich:") {
		j := bazelSandwichJson{}
		err := json.Unmarshal([]byte(target[len("bazel_sandwich:"):]), &j)
		if err != nil {
			return nil, err
		}
		if proptools.BoolDefault(j.DependOnTarget, true) {
			implicitDeps = append(implicitDeps, j.Target)
		}
		implicitDeps = append(implicitDeps, j.ImplicitDeps...)
		dotDotsToReachCwd := ""
		if outDir != "." {
			dotDotsToReachCwd = strings.Repeat("../", strings.Count(outDir, "/")+1)
		}
		target = proptools.ShellEscapeIncludingSpaces(j.Target)
		target = "{DOTDOTS_TO_OUTPUT_ROOT}" + dotDotsToReachCwd + target
	} else {
		target = proptools.ShellEscapeIncludingSpaces(target)
	}

	outDir = proptools.ShellEscapeIncludingSpaces(outDir)
	out = proptools.ShellEscapeIncludingSpaces(out)
	// Use absolute paths, because some soong actions don't play well with relative paths (for example, `cp -d`).
	command := fmt.Sprintf("mkdir -p %[1]s && rm -f %[2]s && ln -sf %[3]s %[2]s", outDir, out, target)
	symlinkPaths := outputPaths[:]

	buildStatement := &BuildStatement{
		Command:      command,
		Depfile:      depfile,
		OutputPaths:  outputPaths,
		Env:          actionEntry.EnvironmentVariables,
		Mnemonic:     actionEntry.Mnemonic,
		SymlinkPaths: symlinkPaths,
		ImplicitDeps: implicitDeps,
	}
	return buildStatement, nil
}

func (a *aqueryArtifactHandler) symlinkActionBuildStatement(actionEntry *analysis_v2_proto.Action) (*BuildStatement, error) {
	outputPaths, depfile, err := a.getOutputPaths(actionEntry)
	if err != nil {
		return nil, err
	}

	inputPaths, err := a.getInputPaths(actionEntry.InputDepSetIds)
	if err != nil {
		return nil, err
	}
	if len(inputPaths) != 1 || len(outputPaths) != 1 {
		return nil, fmt.Errorf("Expect 1 input and 1 output to symlink action, got: input %q, output %q", inputPaths, outputPaths)
	}
	out := outputPaths[0]
	outDir := proptools.ShellEscapeIncludingSpaces(filepath.Dir(out))
	out = proptools.ShellEscapeIncludingSpaces(out)
	in := filepath.Join("$PWD", proptools.ShellEscapeIncludingSpaces(inputPaths[0]))
	// Use absolute paths, because some soong actions don't play well with relative paths (for example, `cp -d`).
	command := fmt.Sprintf("mkdir -p %[1]s && rm -f %[2]s && ln -sf %[3]s %[2]s", outDir, out, in)
	symlinkPaths := outputPaths[:]

	buildStatement := &BuildStatement{
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

func (a *aqueryArtifactHandler) getOutputPaths(actionEntry *analysis_v2_proto.Action) (outputPaths []string, depfile *string, err error) {
	for _, outputId := range actionEntry.OutputIds {
		outputPath, exists := a.artifactIdToPath[artifactId(outputId)]
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
func expandTemplateContent(actionEntry *analysis_v2_proto.Action) string {
	replacerString := make([]string, len(actionEntry.Substitutions)*2)
	for i, pair := range actionEntry.Substitutions {
		value := pair.Value
		if val, ok := templateActionOverriddenTokens[pair.Key]; ok {
			value = val
		}
		replacerString[i*2] = pair.Key
		replacerString[i*2+1] = value
	}
	replacer := strings.NewReplacer(replacerString...)
	return replacer.Replace(actionEntry.TemplateContent)
}

// \->\\, $->\$, `->\`, "->\", \n->\\n, '->'"'"'
var commandLineArgumentReplacer = strings.NewReplacer(
	`\`, `\\`,
	`$`, `\$`,
	"`", "\\`",
	`"`, `\"`,
	"\n", "\\n",
	`'`, `'"'"'`,
)

func escapeCommandlineArgument(str string) string {
	return commandLineArgumentReplacer.Replace(str)
}

func (a *aqueryArtifactHandler) actionToBuildStatement(actionEntry *analysis_v2_proto.Action) (*BuildStatement, error) {
	switch actionEntry.Mnemonic {
	// Middleman actions are not handled like other actions; they are handled separately as a
	// preparatory step so that their inputs may be relayed to actions depending on middleman
	// artifacts.
	case middlemanMnemonic:
		return nil, nil
	// PythonZipper is bogus action returned by aquery, ignore it (b/236198693)
	case "PythonZipper":
		return nil, nil
	// Skip "Fail" actions, which are placeholder actions designed to always fail.
	case "Fail":
		return nil, nil
	case "BaselineCoverage":
		return nil, nil
	case "Symlink", "SolibSymlink", "ExecutableSymlink":
		return a.symlinkActionBuildStatement(actionEntry)
	case "TemplateExpand":
		if len(actionEntry.Arguments) < 1 {
			return a.templateExpandActionBuildStatement(actionEntry)
		}
	case "FileWrite", "SourceSymlinkManifest", "RepoMappingManifest":
		return a.fileWriteActionBuildStatement(actionEntry)
	case "SymlinkTree":
		return a.symlinkTreeActionBuildStatement(actionEntry)
	case "UnresolvedSymlink":
		return a.unresolvedSymlinkActionBuildStatement(actionEntry)
	}

	if len(actionEntry.Arguments) < 1 {
		return nil, errors.New("received action with no command")
	}
	return a.normalActionBuildStatement(actionEntry)

}

func expandPathFragment(id pathFragmentId, pathFragmentsMap map[pathFragmentId]*analysis_v2_proto.PathFragment) (string, error) {
	var labels []string
	currId := id
	// Only positive IDs are valid for path fragments. An ID of zero indicates a terminal node.
	for currId > 0 {
		currFragment, ok := pathFragmentsMap[currId]
		if !ok {
			return "", fmt.Errorf("undefined path fragment id %d", currId)
		}
		labels = append([]string{currFragment.Label}, labels...)
		parentId := pathFragmentId(currFragment.ParentId)
		if currId == parentId {
			return "", fmt.Errorf("fragment cannot refer to itself as parent %#v", currFragment)
		}
		currId = parentId
	}
	return filepath.Join(labels...), nil
}
