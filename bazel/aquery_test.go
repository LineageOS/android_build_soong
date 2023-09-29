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
	"reflect"
	"sort"
	"testing"

	analysis_v2_proto "prebuilts/bazel/common/proto/analysis_v2"

	"github.com/google/blueprint/metrics"
	"google.golang.org/protobuf/proto"
)

func TestAqueryMultiArchGenrule(t *testing.T) {
	// This input string is retrieved from a real build of bionic-related genrules.
	const inputString = `
{
 "Artifacts": [
   { "Id": 1, "path_fragment_id": 1 },
   { "Id": 2, "path_fragment_id": 6 },
   { "Id": 3, "path_fragment_id": 8 },
   { "Id": 4, "path_fragment_id": 12 },
   { "Id": 5, "path_fragment_id": 19 },
   { "Id": 6, "path_fragment_id": 20 },
   { "Id": 7, "path_fragment_id": 21 }],
 "Actions": [{
   "target_id": 1,
   "action_key": "ab53f6ecbdc2ee8cb8812613b63205464f1f5083f6dca87081a0a398c0f1ecf7",
   "Mnemonic": "Genrule",
   "configuration_id": 1,
   "Arguments": ["/bin/bash", "-c", "source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py arm ../sourceroot/bionic/libc/SYSCALLS.TXT \u003e bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-arm.S"],
   "environment_variables": [{
     "Key": "PATH",
     "Value": "/bin:/usr/bin:/usr/local/bin"
   }],
   "input_dep_set_ids": [1],
   "output_ids": [4],
   "primary_output_id": 4
 }, {
   "target_id": 2,
   "action_key": "9f4309ce165dac458498cb92811c18b0b7919782cc37b82a42d2141b8cc90826",
   "Mnemonic": "Genrule",
   "configuration_id": 1,
   "Arguments": ["/bin/bash", "-c", "source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py x86 ../sourceroot/bionic/libc/SYSCALLS.TXT \u003e bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-x86.S"],
   "environment_variables": [{
     "Key": "PATH",
     "Value": "/bin:/usr/bin:/usr/local/bin"
   }],
   "input_dep_set_ids": [2],
   "output_ids": [5],
   "primary_output_id": 5
 }, {
   "target_id": 3,
   "action_key": "50d6c586103ebeed3a218195540bcc30d329464eae36377eb82f8ce7c36ac342",
   "Mnemonic": "Genrule",
   "configuration_id": 1,
   "Arguments": ["/bin/bash", "-c", "source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py x86_64 ../sourceroot/bionic/libc/SYSCALLS.TXT \u003e bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-x86_64.S"],
   "environment_variables": [{
     "Key": "PATH",
     "Value": "/bin:/usr/bin:/usr/local/bin"
   }],
   "input_dep_set_ids": [3],
   "output_ids": [6],
   "primary_output_id": 6
 }, {
   "target_id": 4,
   "action_key": "f30cbe442f5216f4223cf16a39112cad4ec56f31f49290d85cff587e48647ffa",
   "Mnemonic": "Genrule",
   "configuration_id": 1,
   "Arguments": ["/bin/bash", "-c", "source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py arm64 ../sourceroot/bionic/libc/SYSCALLS.TXT \u003e bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-arm64.S"],
   "environment_variables": [{
     "Key": "PATH",
     "Value": "/bin:/usr/bin:/usr/local/bin"
   }],
   "input_dep_set_ids": [4],
   "output_ids": [7],
   "primary_output_id": 7
 }],
 "Targets": [
   { "Id": 1, "Label": "@sourceroot//bionic/libc:syscalls-arm", "rule_class_id": 1 },
   { "Id": 2, "Label": "@sourceroot//bionic/libc:syscalls-x86", "rule_class_id": 1 },
   { "Id": 3, "Label": "@sourceroot//bionic/libc:syscalls-x86_64", "rule_class_id": 1 },
   { "Id": 4, "Label": "@sourceroot//bionic/libc:syscalls-arm64", "rule_class_id": 1 }],
 "dep_set_of_files": [
   { "Id": 1, "direct_artifact_ids": [1, 2, 3] },
   { "Id": 2, "direct_artifact_ids": [1, 2, 3] },
   { "Id": 3, "direct_artifact_ids": [1, 2, 3] },
   { "Id": 4, "direct_artifact_ids": [1, 2, 3] }],
 "Configuration": [{
   "Id": 1,
   "Mnemonic": "k8-fastbuild",
   "platform_name": "k8",
   "Checksum": "485c362832c178e367d972177f68e69e0981e51e67ef1c160944473db53fe046"
 }],
 "rule_classes": [{ "Id": 1, "Name": "genrule"}],
 "path_fragments": [
   { "Id": 5, "Label": ".." },
   { "Id": 4, "Label": "sourceroot", "parent_id": 5 },
   { "Id": 3, "Label": "bionic", "parent_id": 4 },
   { "Id": 2, "Label": "libc", "parent_id": 3 },
   { "Id": 1, "Label": "SYSCALLS.TXT", "parent_id": 2 },
   { "Id": 7, "Label": "tools", "parent_id": 2 },
   { "Id": 6, "Label": "gensyscalls.py", "parent_id": 7 },
   { "Id": 11, "Label": "bazel_tools", "parent_id": 5 },
   { "Id": 10, "Label": "tools", "parent_id": 11 },
   { "Id": 9, "Label": "genrule", "parent_id": 10 },
   { "Id": 8, "Label": "genrule-setup.sh", "parent_id": 9 },
   { "Id": 18, "Label": "bazel-out" },
   { "Id": 17, "Label": "sourceroot", "parent_id": 18 },
   { "Id": 16, "Label": "k8-fastbuild", "parent_id": 17 },
   { "Id": 15, "Label": "bin", "parent_id": 16 },
   { "Id": 14, "Label": "bionic", "parent_id": 15 },
   { "Id": 13, "Label": "libc", "parent_id": 14 },
   { "Id": 12, "Label": "syscalls-arm.S", "parent_id": 13 },
   { "Id": 19, "Label": "syscalls-x86.S", "parent_id": 13 },
   { "Id": 20, "Label": "syscalls-x86_64.S", "parent_id": 13 },
   { "Id": 21, "Label": "syscalls-arm64.S", "parent_id": 13 }]
}
`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actualbuildStatements, actualDepsets, _ := AqueryBuildStatements(data, &metrics.EventHandler{})
	var expectedBuildStatements []*BuildStatement
	for _, arch := range []string{"arm", "arm64", "x86", "x86_64"} {
		expectedBuildStatements = append(expectedBuildStatements,
			&BuildStatement{
				Command: fmt.Sprintf(
					"/bin/bash -c 'source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py %s ../sourceroot/bionic/libc/SYSCALLS.TXT > bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-%s.S'",
					arch, arch),
				OutputPaths: []string{
					fmt.Sprintf("bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-%s.S", arch),
				},
				Env: []*analysis_v2_proto.KeyValuePair{
					{Key: "PATH", Value: "/bin:/usr/bin:/usr/local/bin"},
				},
				Mnemonic: "Genrule",
			})
	}
	assertBuildStatements(t, expectedBuildStatements, actualbuildStatements)

	expectedFlattenedInputs := []string{
		"../sourceroot/bionic/libc/SYSCALLS.TXT",
		"../sourceroot/bionic/libc/tools/gensyscalls.py",
	}
	// In this example, each depset should have the same expected inputs.
	for _, actualDepset := range actualDepsets {
		actualFlattenedInputs := flattenDepsets([]string{actualDepset.ContentHash}, actualDepsets)
		if !reflect.DeepEqual(actualFlattenedInputs, expectedFlattenedInputs) {
			t.Errorf("Expected flattened inputs %v, but got %v", expectedFlattenedInputs, actualFlattenedInputs)
		}
	}
}

func TestInvalidOutputId(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 }],
 "actions": [{
   "target_id": 1,
   "action_key": "action_x",
   "mnemonic": "X",
   "arguments": ["touch", "foo"],
   "input_dep_set_ids": [1],
   "output_ids": [3],
   "primary_output_id": 3
 }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1, 2] }],
 "path_fragments": [
   { "id": 1, "label": "one" },
   { "id": 2, "label": "two" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = AqueryBuildStatements(data, &metrics.EventHandler{})
	assertError(t, err, "undefined outputId 3: [X] []")
}

func TestInvalidInputDepsetIdFromAction(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 }],
 "actions": [{
   "target_id": 1,
   "action_key": "action_x",
   "mnemonic": "X",
   "arguments": ["touch", "foo"],
   "input_dep_set_ids": [2],
   "output_ids": [1],
   "primary_output_id": 1
 }],
 "targets": [{
   "id": 1,
   "label": "target_x"
 }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1, 2] }],
 "path_fragments": [
   { "id": 1, "label": "one" },
   { "id": 2, "label": "two" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = AqueryBuildStatements(data, &metrics.EventHandler{})
	assertError(t, err, "undefined (not even empty) input depsetId 2: [X] [target_x]")
}

func TestInvalidInputDepsetIdFromDepset(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "x",
   "arguments": ["touch", "foo"],
   "input_dep_set_ids": [1],
   "output_ids": [1],
   "primary_output_id": 1
 }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1, 2], "transitive_dep_set_ids": [42] }],
 "path_fragments": [
   { "id": 1, "label": "one"},
   { "id": 2, "label": "two" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = AqueryBuildStatements(data, &metrics.EventHandler{})
	assertError(t, err, "undefined input depsetId 42 (referenced by depsetId 1)")
}

func TestInvalidInputArtifactId(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "x",
   "arguments": ["touch", "foo"],
   "input_dep_set_ids": [1],
   "output_ids": [1],
   "primary_output_id": 1
 }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1, 3] }],
 "path_fragments": [
   { "id": 1, "label": "one" },
   { "id": 2, "label": "two" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = AqueryBuildStatements(data, &metrics.EventHandler{})
	assertError(t, err, "undefined input artifactId 3")
}

func TestInvalidPathFragmentId(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "x",
   "arguments": ["touch", "foo"],
   "input_dep_set_ids": [1],
   "output_ids": [1],
   "primary_output_id": 1
 }],
 "dep_set_of_files": [
    { "id": 1, "direct_artifact_ids": [1, 2] }],
 "path_fragments": [
   {  "id": 1, "label": "one" },
   {  "id": 2, "label": "two", "parent_id": 3 }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = AqueryBuildStatements(data, &metrics.EventHandler{})
	assertError(t, err, "undefined path fragment id 3")
}

func TestDepfiles(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "path_fragment_id": 1 },
    { "id": 2, "path_fragment_id": 2 },
    { "id": 3, "path_fragment_id": 3 }],
  "actions": [{
    "target_Id": 1,
    "action_Key": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "input_dep_set_ids": [1],
    "output_ids": [2, 3],
    "primary_output_id": 2
  }],
  "dep_set_of_files": [
    { "id": 1, "direct_Artifact_Ids": [1, 2, 3] }],
  "path_fragments": [
    { "id": 1, "label": "one" },
    { "id": 2, "label": "two" },
    { "id": 3, "label": "two.d" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}
	if expected := 1; len(actual) != expected {
		t.Fatalf("Expected %d build statements, got %d", expected, len(actual))
		return
	}

	bs := actual[0]
	expectedDepfile := "two.d"
	if bs.Depfile == nil {
		t.Errorf("Expected depfile %q, but there was none found", expectedDepfile)
	} else if *bs.Depfile != expectedDepfile {
		t.Errorf("Expected depfile %q, but got %q", expectedDepfile, *bs.Depfile)
	}
}

func TestMultipleDepfiles(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 },
   { "id": 3, "path_fragment_id": 3 },
   { "id": 4, "path_fragment_id": 4 }],
 "actions": [{
   "target_id": 1,
   "action_key": "action_x",
   "mnemonic": "X",
   "arguments": ["touch", "foo"],
   "input_dep_set_ids": [1],
   "output_ids": [2,3,4],
   "primary_output_id": 2
 }],
 "dep_set_of_files": [{
   "id": 1,
   "direct_artifact_ids": [1, 2, 3, 4]
 }],
 "path_fragments": [
   { "id": 1, "label": "one" },
   { "id": 2, "label": "two" },
   { "id": 3, "label": "two.d" },
   { "id": 4, "label": "other.d" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = AqueryBuildStatements(data, &metrics.EventHandler{})
	assertError(t, err, `found multiple potential depfiles "two.d", "other.d": [X] []`)
}

func TestTransitiveInputDepsets(t *testing.T) {
	// The input aquery for this test comes from a proof-of-concept starlark rule which registers
	// a single action with many inputs given via a deep depset.
	const inputString = `
{
 "artifacts": [
  { "id": 1, "path_fragment_id": 1 },
  { "id": 2, "path_fragment_id": 7 },
  { "id": 3, "path_fragment_id": 8 },
  { "id": 4, "path_fragment_id": 9 },
  { "id": 5, "path_fragment_id": 10 },
  { "id": 6, "path_fragment_id": 11 },
  { "id": 7, "path_fragment_id": 12 },
  { "id": 8, "path_fragment_id": 13 },
  { "id": 9, "path_fragment_id": 14 },
  { "id": 10, "path_fragment_id": 15 },
  { "id": 11, "path_fragment_id": 16 },
  { "id": 12, "path_fragment_id": 17 },
  { "id": 13, "path_fragment_id": 18 },
  { "id": 14, "path_fragment_id": 19 },
  { "id": 15, "path_fragment_id": 20 },
  { "id": 16, "path_fragment_id": 21 },
  { "id": 17, "path_fragment_id": 22 },
  { "id": 18, "path_fragment_id": 23 },
  { "id": 19, "path_fragment_id": 24 },
  { "id": 20, "path_fragment_id": 25 },
  { "id": 21, "path_fragment_id": 26 }],
 "actions": [{
   "target_id": 1,
   "action_key": "3b826d17fadbbbcd8313e456b90ec47c078c438088891dd45b4adbcd8889dc50",
   "mnemonic": "Action",
   "configuration_id": 1,
   "arguments": ["/bin/bash", "-c", "touch bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_out"],
   "input_dep_set_ids": [1],
   "output_ids": [21],
   "primary_output_id": 21
 }],
 "dep_set_of_files": [
   { "id": 3, "direct_artifact_ids": [1, 2, 3, 4, 5] },
   { "id": 4, "direct_artifact_ids": [6, 7, 8, 9, 10] },
   { "id": 2, "transitive_dep_set_ids": [3, 4], "direct_artifact_ids": [11, 12, 13, 14, 15] },
   { "id": 5, "direct_artifact_ids": [16, 17, 18, 19] },
   { "id": 1, "transitive_dep_set_ids": [2, 5], "direct_artifact_ids": [20] }],
 "path_fragments": [
   { "id": 6, "label": "bazel-out" },
   { "id": 5, "label": "sourceroot", "parent_id": 6 },
   { "id": 4, "label": "k8-fastbuild", "parent_id": 5 },
   { "id": 3, "label": "bin", "parent_id": 4 },
   { "id": 2, "label": "testpkg", "parent_id": 3 },
   { "id": 1, "label": "test_1", "parent_id": 2 },
   { "id": 7, "label": "test_2", "parent_id": 2 },
   { "id": 8, "label": "test_3", "parent_id": 2 },
   { "id": 9, "label": "test_4", "parent_id": 2 },
   { "id": 10, "label": "test_5", "parent_id": 2 },
   { "id": 11, "label": "test_6", "parent_id": 2 },
   { "id": 12, "label": "test_7", "parent_id": 2 },
	 { "id": 13, "label": "test_8", "parent_id": 2 },
   { "id": 14, "label": "test_9", "parent_id": 2 },
   { "id": 15, "label": "test_10", "parent_id": 2 },
   { "id": 16, "label": "test_11", "parent_id": 2 },
   { "id": 17, "label": "test_12", "parent_id": 2 },
   { "id": 18, "label": "test_13", "parent_id": 2 },
   { "id": 19, "label": "test_14", "parent_id": 2 },
   { "id": 20, "label": "test_15", "parent_id": 2 },
   { "id": 21, "label": "test_16", "parent_id": 2 },
   { "id": 22, "label": "test_17", "parent_id": 2 },
   { "id": 23, "label": "test_18", "parent_id": 2 },
   { "id": 24, "label": "test_19", "parent_id": 2 },
   { "id": 25, "label": "test_root", "parent_id": 2 },
   { "id": 26,"label": "test_out", "parent_id": 2 }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actualbuildStatements, actualDepsets, _ := AqueryBuildStatements(data, &metrics.EventHandler{})

	expectedBuildStatements := []*BuildStatement{
		&BuildStatement{
			Command:      "/bin/bash -c 'touch bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_out'",
			OutputPaths:  []string{"bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_out"},
			Mnemonic:     "Action",
			SymlinkPaths: []string{},
		},
	}
	assertBuildStatements(t, expectedBuildStatements, actualbuildStatements)

	// Inputs for the action are test_{i} from 1 to 20, and test_root. These inputs
	// are given via a deep depset, but the depset is flattened when returned as a
	// BuildStatement slice.
	var expectedFlattenedInputs []string
	for i := 1; i < 20; i++ {
		expectedFlattenedInputs = append(expectedFlattenedInputs, fmt.Sprintf("bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_%d", i))
	}
	expectedFlattenedInputs = append(expectedFlattenedInputs, "bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_root")

	actualDepsetHashes := actualbuildStatements[0].InputDepsetHashes
	actualFlattenedInputs := flattenDepsets(actualDepsetHashes, actualDepsets)
	if !reflect.DeepEqual(actualFlattenedInputs, expectedFlattenedInputs) {
		t.Errorf("Expected flattened inputs %v, but got %v", expectedFlattenedInputs, actualFlattenedInputs)
	}
}

func TestSymlinkTree(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "SymlinkTree",
   "configuration_id": 1,
   "input_dep_set_ids": [1],
   "output_ids": [2],
   "primary_output_id": 2,
   "execution_platform": "//build/bazel/platforms:linux_x86_64"
 }],
 "path_fragments": [
   { "id": 1, "label": "foo.manifest" },
   { "id": 2, "label": "foo.runfiles/MANIFEST" }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1] }]
}
`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}
	assertBuildStatements(t, []*BuildStatement{
		&BuildStatement{
			Command:      "",
			OutputPaths:  []string{"foo.runfiles/MANIFEST"},
			Mnemonic:     "SymlinkTree",
			InputPaths:   []string{"foo.manifest"},
			SymlinkPaths: []string{},
		},
	}, actual)
}

func TestBazelToolsRemovalFromInputDepsets(t *testing.T) {
	const inputString = `{
 "artifacts": [
   { "id": 1, "path_fragment_id": 10 },
   { "id": 2, "path_fragment_id": 20 },
   { "id": 3, "path_fragment_id": 30 },
   { "id": 4, "path_fragment_id": 40 }],
 "dep_set_of_files": [{
   "id": 1111,
   "direct_artifact_ids": [3 , 4]
 }, {
   "id": 2222,
   "direct_artifact_ids": [3]
 }],
 "actions": [{
   "target_id": 100,
   "action_key": "x",
   "input_dep_set_ids": [1111, 2222],
   "mnemonic": "x",
   "arguments": ["bogus", "command"],
   "output_ids": [2],
   "primary_output_id": 1
 }],
 "path_fragments": [
   { "id": 10, "label": "input" },
   { "id": 20, "label": "output" },
   { "id": 30, "label": "dep1", "parent_id": 50 },
   { "id": 40, "label": "dep2", "parent_id": 60 },
   { "id": 50, "label": "bazel_tools", "parent_id": 60 },
   { "id": 60, "label": ".."}
 ]
}`
	/* depsets
	       1111  2222
	       /  \   |
	../dep2    ../bazel_tools/dep1
	*/
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actualBuildStatements, actualDepsets, _ := AqueryBuildStatements(data, &metrics.EventHandler{})
	if len(actualDepsets) != 1 {
		t.Errorf("expected 1 depset but found %#v", actualDepsets)
		return
	}
	dep2Found := false
	for _, dep := range flattenDepsets([]string{actualDepsets[0].ContentHash}, actualDepsets) {
		if dep == "../bazel_tools/dep1" {
			t.Errorf("dependency %s expected to be removed but still exists", dep)
		} else if dep == "../dep2" {
			dep2Found = true
		}
	}
	if !dep2Found {
		t.Errorf("dependency ../dep2 expected but not found")
	}

	expectedBuildStatement := &BuildStatement{
		Command:      "bogus command",
		OutputPaths:  []string{"output"},
		Mnemonic:     "x",
		SymlinkPaths: []string{},
	}
	buildStatementFound := false
	for _, actualBuildStatement := range actualBuildStatements {
		if buildStatementEquals(actualBuildStatement, expectedBuildStatement) == "" {
			buildStatementFound = true
			break
		}
	}
	if !buildStatementFound {
		t.Errorf("expected but missing %#v in %#v", expectedBuildStatement, actualBuildStatements)
		return
	}
}

func TestBazelToolsRemovalFromTargets(t *testing.T) {
	const inputString = `{
 "artifacts": [{ "id": 1, "path_fragment_id": 10 }],
 "targets": [
   { "id": 100, "label": "targetX" },
   { "id": 200, "label": "@bazel_tools//tool_y" }
],
 "actions": [{
   "target_id": 100,
   "action_key": "actionX",
   "arguments": ["bogus", "command"],
   "mnemonic" : "x",
   "output_ids": [1]
 }, {
   "target_id": 200,
   "action_key": "y"
 }],
 "path_fragments": [{ "id": 10, "label": "outputX"}]
}`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actualBuildStatements, actualDepsets, _ := AqueryBuildStatements(data, &metrics.EventHandler{})
	if len(actualDepsets) != 0 {
		t.Errorf("expected 0 depset but found %#v", actualDepsets)
		return
	}
	expectedBuildStatement := &BuildStatement{
		Command:      "bogus command",
		OutputPaths:  []string{"outputX"},
		Mnemonic:     "x",
		SymlinkPaths: []string{},
	}
	buildStatementFound := false
	for _, actualBuildStatement := range actualBuildStatements {
		if buildStatementEquals(actualBuildStatement, expectedBuildStatement) == "" {
			buildStatementFound = true
			break
		}
	}
	if !buildStatementFound {
		t.Errorf("expected but missing %#v in %#v build statements", expectedBuildStatement, len(actualBuildStatements))
		return
	}
}

func TestBazelToolsRemovalFromTransitiveInputDepsets(t *testing.T) {
	const inputString = `{
 "artifacts": [
   { "id": 1, "path_fragment_id": 10 },
   { "id": 2, "path_fragment_id": 20 },
   { "id": 3, "path_fragment_id": 30 }],
 "dep_set_of_files": [{
   "id": 1111,
   "transitive_dep_set_ids": [2222]
 }, {
   "id": 2222,
   "direct_artifact_ids": [3]
 }, {
   "id": 3333,
   "direct_artifact_ids": [3]
 }, {
   "id": 4444,
   "transitive_dep_set_ids": [3333]
 }],
 "actions": [{
   "target_id": 100,
   "action_key": "x",
   "input_dep_set_ids": [1111, 4444],
   "mnemonic": "x",
   "arguments": ["bogus", "command"],
   "output_ids": [2],
   "primary_output_id": 1
 }],
 "path_fragments": [
   { "id": 10, "label": "input" },
   { "id": 20, "label": "output" },
   { "id": 30, "label": "dep", "parent_id": 50 },
   { "id": 50, "label": "bazel_tools", "parent_id": 60 },
   { "id": 60, "label": ".."}
 ]
}`
	/* depsets
	    1111    4444
	     ||      ||
	    2222    3333
	      |      |
	../bazel_tools/dep
	Note: in dep_set_of_files:
	  1111 appears BEFORE its dependency,2222 while
	  4444 appears AFTER its dependency 3333
	and this test shows that that order doesn't affect empty depset pruning
	*/
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actualBuildStatements, actualDepsets, _ := AqueryBuildStatements(data, &metrics.EventHandler{})
	if len(actualDepsets) != 0 {
		t.Errorf("expected 0 depsets but found %#v", actualDepsets)
		return
	}

	expectedBuildStatement := &BuildStatement{
		Command:     "bogus command",
		OutputPaths: []string{"output"},
		Mnemonic:    "x",
	}
	buildStatementFound := false
	for _, actualBuildStatement := range actualBuildStatements {
		if buildStatementEquals(actualBuildStatement, expectedBuildStatement) == "" {
			buildStatementFound = true
			break
		}
	}
	if !buildStatementFound {
		t.Errorf("expected but missing %#v in %#v", expectedBuildStatement, actualBuildStatements)
		return
	}
}

func TestMiddlemenAction(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 },
   { "id": 3, "path_fragment_id": 3 },
   { "id": 4, "path_fragment_id": 4 },
   { "id": 5, "path_fragment_id": 5 },
   { "id": 6, "path_fragment_id": 6 }],
 "path_fragments": [
   { "id": 1, "label": "middleinput_one" },
   { "id": 2, "label": "middleinput_two" },
   { "id": 3, "label": "middleman_artifact" },
   { "id": 4, "label": "maininput_one" },
   { "id": 5, "label": "maininput_two" },
   { "id": 6, "label": "output" }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1, 2] },
   { "id": 2, "direct_artifact_ids": [3, 4, 5] }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "Middleman",
   "arguments": ["touch", "foo"],
   "input_dep_set_ids": [1],
   "output_ids": [3],
   "primary_output_id": 3
 }, {
   "target_id": 2,
   "action_key": "y",
   "mnemonic": "Main action",
   "arguments": ["touch", "foo"],
   "input_dep_set_ids": [2],
   "output_ids": [6],
   "primary_output_id": 6
 }]
}`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actualBuildStatements, actualDepsets, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}
	if expected := 2; len(actualBuildStatements) != expected {
		t.Fatalf("Expected %d build statements, got %d %#v", expected, len(actualBuildStatements), actualBuildStatements)
		return
	}

	expectedDepsetFiles := [][]string{
		{"middleinput_one", "middleinput_two", "maininput_one", "maininput_two"},
		{"middleinput_one", "middleinput_two"},
	}
	assertFlattenedDepsets(t, actualDepsets, expectedDepsetFiles)

	bs := actualBuildStatements[0]
	if len(bs.InputPaths) > 0 {
		t.Errorf("Expected main action raw inputs to be empty, but got %q", bs.InputPaths)
	}

	expectedOutputs := []string{"output"}
	if !reflect.DeepEqual(bs.OutputPaths, expectedOutputs) {
		t.Errorf("Expected main action outputs %q, but got %q", expectedOutputs, bs.OutputPaths)
	}

	expectedFlattenedInputs := []string{"middleinput_one", "middleinput_two", "maininput_one", "maininput_two"}
	actualFlattenedInputs := flattenDepsets(bs.InputDepsetHashes, actualDepsets)

	if !reflect.DeepEqual(actualFlattenedInputs, expectedFlattenedInputs) {
		t.Errorf("Expected flattened inputs %v, but got %v", expectedFlattenedInputs, actualFlattenedInputs)
	}

	bs = actualBuildStatements[1]
	if bs != nil {
		t.Errorf("Expected nil action for skipped")
	}
}

// Returns the contents of given depsets in concatenated post order.
func flattenDepsets(depsetHashesToFlatten []string, allDepsets []AqueryDepset) []string {
	depsetsByHash := map[string]AqueryDepset{}
	for _, depset := range allDepsets {
		depsetsByHash[depset.ContentHash] = depset
	}
	var result []string
	for _, depsetId := range depsetHashesToFlatten {
		result = append(result, flattenDepset(depsetId, depsetsByHash)...)
	}
	return result
}

// Returns the contents of a given depset in post order.
func flattenDepset(depsetHashToFlatten string, allDepsets map[string]AqueryDepset) []string {
	depset := allDepsets[depsetHashToFlatten]
	var result []string
	for _, depsetId := range depset.TransitiveDepSetHashes {
		result = append(result, flattenDepset(depsetId, allDepsets)...)
	}
	result = append(result, depset.DirectArtifacts...)
	return result
}

func assertFlattenedDepsets(t *testing.T, actualDepsets []AqueryDepset, expectedDepsetFiles [][]string) {
	t.Helper()
	if len(actualDepsets) != len(expectedDepsetFiles) {
		t.Errorf("Expected %d depsets, but got %d depsets", len(expectedDepsetFiles), len(actualDepsets))
	}
	for i, actualDepset := range actualDepsets {
		actualFlattenedInputs := flattenDepsets([]string{actualDepset.ContentHash}, actualDepsets)
		if !reflect.DeepEqual(actualFlattenedInputs, expectedDepsetFiles[i]) {
			t.Errorf("Expected depset files: %v, but got %v", expectedDepsetFiles[i], actualFlattenedInputs)
		}
	}
}

func TestSimpleSymlink(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 3 },
   { "id": 2, "path_fragment_id": 5 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "Symlink",
   "input_dep_set_ids": [1],
   "output_ids": [2],
   "primary_output_id": 2
 }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1] }],
 "path_fragments": [
   { "id": 1, "label": "one" },
   { "id": 2, "label": "file_subdir", "parent_id": 1 },
   { "id": 3, "label": "file", "parent_id": 2 },
   { "id": 4, "label": "symlink_subdir", "parent_id": 1 },
   { "id": 5, "label": "symlink", "parent_id": 4 }]
}`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})

	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}

	expectedBuildStatements := []*BuildStatement{
		&BuildStatement{
			Command: "mkdir -p one/symlink_subdir && " +
				"rm -f one/symlink_subdir/symlink && " +
				"ln -sf $PWD/one/file_subdir/file one/symlink_subdir/symlink",
			InputPaths:   []string{"one/file_subdir/file"},
			OutputPaths:  []string{"one/symlink_subdir/symlink"},
			SymlinkPaths: []string{"one/symlink_subdir/symlink"},
			Mnemonic:     "Symlink",
		},
	}
	assertBuildStatements(t, actual, expectedBuildStatements)
}

func TestSymlinkQuotesPaths(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 3 },
   { "id": 2, "path_fragment_id": 5 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "SolibSymlink",
   "input_dep_set_ids": [1],
   "output_ids": [2],
   "primary_output_id": 2
 }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1] }],
 "path_fragments": [
   { "id": 1, "label": "one" },
   { "id": 2, "label": "file subdir", "parent_id": 1 },
   { "id": 3, "label": "file", "parent_id": 2 },
   { "id": 4, "label": "symlink subdir", "parent_id": 1 },
   { "id": 5, "label": "symlink", "parent_id": 4 }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}

	expectedBuildStatements := []*BuildStatement{
		&BuildStatement{
			Command: "mkdir -p 'one/symlink subdir' && " +
				"rm -f 'one/symlink subdir/symlink' && " +
				"ln -sf $PWD/'one/file subdir/file' 'one/symlink subdir/symlink'",
			InputPaths:   []string{"one/file subdir/file"},
			OutputPaths:  []string{"one/symlink subdir/symlink"},
			SymlinkPaths: []string{"one/symlink subdir/symlink"},
			Mnemonic:     "SolibSymlink",
		},
	}
	assertBuildStatements(t, expectedBuildStatements, actual)
}

func TestSymlinkMultipleInputs(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 },
   { "id": 3, "path_fragment_id": 3 }],
 "actions": [{
   "target_id": 1,
   "action_key": "action_x",
   "mnemonic": "Symlink",
   "input_dep_set_ids": [1],
   "output_ids": [3],
   "primary_output_id": 3
 }],
 "dep_set_of_files": [{ "id": 1, "direct_artifact_ids": [1,2] }],
 "path_fragments": [
   { "id": 1, "label": "file" },
   { "id": 2, "label": "other_file" },
   { "id": 3, "label": "symlink" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = AqueryBuildStatements(data, &metrics.EventHandler{})
	assertError(t, err, `Expect 1 input and 1 output to symlink action, got: input ["file" "other_file"], output ["symlink"]: [Symlink] []`)
}

func TestSymlinkMultipleOutputs(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 3, "path_fragment_id": 3 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "Symlink",
   "input_dep_set_ids": [1],
   "output_ids": [2,3],
   "primary_output_id": 2
 }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1] }],
 "path_fragments": [
   { "id": 1, "label": "file" },
   { "id": 2, "label": "symlink" },
   { "id": 3,  "label": "other_symlink" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = AqueryBuildStatements(data, &metrics.EventHandler{})
	assertError(t, err, "undefined outputId 2: [Symlink] []")
}

func TestTemplateExpandActionSubstitutions(t *testing.T) {
	const inputString = `
{
 "artifacts": [{
   "id": 1,
   "path_fragment_id": 1
 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "TemplateExpand",
   "configuration_id": 1,
   "output_ids": [1],
   "primary_output_id": 1,
   "execution_platform": "//build/bazel/platforms:linux_x86_64",
   "template_content": "Test template substitutions: %token1%, %python_binary%",
   "substitutions": [
     { "key": "%token1%", "value": "abcd" },
     { "key": "%python_binary%", "value": "python3" }]
 }],
 "path_fragments": [
   { "id": 1, "label": "template_file" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}

	expectedBuildStatements := []*BuildStatement{
		&BuildStatement{
			Command: "/bin/bash -c 'echo \"Test template substitutions: abcd, python3\" | sed \"s/\\\\\\\\n/\\\\n/g\" > template_file && " +
				"chmod a+x template_file'",
			OutputPaths:  []string{"template_file"},
			Mnemonic:     "TemplateExpand",
			SymlinkPaths: []string{},
		},
	}
	assertBuildStatements(t, expectedBuildStatements, actual)
}

func TestTemplateExpandActionNoOutput(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "TemplateExpand",
   "configuration_id": 1,
   "primary_output_id": 1,
   "execution_platform": "//build/bazel/platforms:linux_x86_64",
   "templateContent": "Test template substitutions: %token1%, %python_binary%",
   "substitutions": [
     { "key": "%token1%", "value": "abcd" },
     { "key": "%python_binary%", "value": "python3" }]
 }],
 "path_fragments": [
   { "id": 1, "label": "template_file" }]
}`

	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = AqueryBuildStatements(data, &metrics.EventHandler{})
	assertError(t, err, `Expect 1 output to template expand action, got: output []: [TemplateExpand] []`)
}

func TestFileWrite(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "FileWrite",
   "configuration_id": 1,
   "output_ids": [1],
   "primary_output_id": 1,
   "execution_platform": "//build/bazel/platforms:linux_x86_64",
   "file_contents": "file data\n"
 }],
 "path_fragments": [
   { "id": 1, "label": "foo.manifest" }]
}
`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}
	assertBuildStatements(t, []*BuildStatement{
		&BuildStatement{
			OutputPaths:  []string{"foo.manifest"},
			Mnemonic:     "FileWrite",
			FileContents: "file data\n",
			SymlinkPaths: []string{},
		},
	}, actual)
}

func TestSourceSymlinkManifest(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 }],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "SourceSymlinkManifest",
   "configuration_id": 1,
   "output_ids": [1],
   "primary_output_id": 1,
   "execution_platform": "//build/bazel/platforms:linux_x86_64",
   "file_contents": "symlink target\n"
 }],
 "path_fragments": [
   { "id": 1, "label": "foo.manifest" }]
}
`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}
	assertBuildStatements(t, []*BuildStatement{
		&BuildStatement{
			OutputPaths:  []string{"foo.manifest"},
			Mnemonic:     "SourceSymlinkManifest",
			SymlinkPaths: []string{},
		},
	}, actual)
}

func TestUnresolvedSymlink(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 }
 ],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "UnresolvedSymlink",
   "configuration_id": 1,
   "output_ids": [1],
   "primary_output_id": 1,
   "execution_platform": "//build/bazel/platforms:linux_x86_64",
   "unresolved_symlink_target": "symlink/target"
 }],
 "path_fragments": [
   { "id": 1, "label": "path/to/symlink" }
 ]
}
`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}
	assertBuildStatements(t, []*BuildStatement{{
		Command:      "mkdir -p path/to && rm -f path/to/symlink && ln -sf symlink/target path/to/symlink",
		OutputPaths:  []string{"path/to/symlink"},
		Mnemonic:     "UnresolvedSymlink",
		SymlinkPaths: []string{"path/to/symlink"},
	}}, actual)
}

func TestUnresolvedSymlinkBazelSandwich(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 }
 ],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "UnresolvedSymlink",
   "configuration_id": 1,
   "output_ids": [1],
   "primary_output_id": 1,
   "execution_platform": "//build/bazel/platforms:linux_x86_64",
   "unresolved_symlink_target": "bazel_sandwich:{\"target\":\"target/product/emulator_x86_64/system\"}"
 }],
 "path_fragments": [
   { "id": 1, "label": "path/to/symlink" }
 ]
}
`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}
	assertBuildStatements(t, []*BuildStatement{{
		Command:      "mkdir -p path/to && rm -f path/to/symlink && ln -sf {DOTDOTS_TO_OUTPUT_ROOT}../../target/product/emulator_x86_64/system path/to/symlink",
		OutputPaths:  []string{"path/to/symlink"},
		Mnemonic:     "UnresolvedSymlink",
		SymlinkPaths: []string{"path/to/symlink"},
		ImplicitDeps: []string{"target/product/emulator_x86_64/system"},
	}}, actual)
}

func TestUnresolvedSymlinkBazelSandwichWithAlternativeDeps(t *testing.T) {
	const inputString = `
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 }
 ],
 "actions": [{
   "target_id": 1,
   "action_key": "x",
   "mnemonic": "UnresolvedSymlink",
   "configuration_id": 1,
   "output_ids": [1],
   "primary_output_id": 1,
   "execution_platform": "//build/bazel/platforms:linux_x86_64",
   "unresolved_symlink_target": "bazel_sandwich:{\"depend_on_target\":false,\"implicit_deps\":[\"target/product/emulator_x86_64/obj/PACKAGING/systemimage_intermediates/staging_dir.stamp\"],\"target\":\"target/product/emulator_x86_64/system\"}"
 }],
 "path_fragments": [
   { "id": 1, "label": "path/to/symlink" }
 ]
}
`
	data, err := JsonToActionGraphContainer(inputString)
	if err != nil {
		t.Error(err)
		return
	}
	actual, _, err := AqueryBuildStatements(data, &metrics.EventHandler{})
	if err != nil {
		t.Errorf("Unexpected error %q", err)
		return
	}
	assertBuildStatements(t, []*BuildStatement{{
		Command:      "mkdir -p path/to && rm -f path/to/symlink && ln -sf {DOTDOTS_TO_OUTPUT_ROOT}../../target/product/emulator_x86_64/system path/to/symlink",
		OutputPaths:  []string{"path/to/symlink"},
		Mnemonic:     "UnresolvedSymlink",
		SymlinkPaths: []string{"path/to/symlink"},
		// Note that the target of the symlink, target/product/emulator_x86_64/system, is not listed here
		ImplicitDeps: []string{"target/product/emulator_x86_64/obj/PACKAGING/systemimage_intermediates/staging_dir.stamp"},
	}}, actual)
}

func assertError(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Errorf("expected error '%s', but got no error", expected)
	} else if err.Error() != expected {
		t.Errorf("expected error:\n\t'%s', but got:\n\t'%s'", expected, err.Error())
	}
}

// Asserts that the given actual build statements match the given expected build statements.
// Build statement equivalence is determined using buildStatementEquals.
func assertBuildStatements(t *testing.T, expected []*BuildStatement, actual []*BuildStatement) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("expected %d build statements, but got %d,\n expected: %#v,\n actual: %#v",
			len(expected), len(actual), expected, actual)
		return
	}
	type compareFn = func(i int, j int) bool
	byCommand := func(slice []*BuildStatement) compareFn {
		return func(i int, j int) bool {
			if slice[i] == nil {
				return false
			} else if slice[j] == nil {
				return false
			}
			return slice[i].Command < slice[j].Command
		}
	}
	sort.SliceStable(expected, byCommand(expected))
	sort.SliceStable(actual, byCommand(actual))
	for i, actualStatement := range actual {
		expectedStatement := expected[i]
		if differingField := buildStatementEquals(actualStatement, expectedStatement); differingField != "" {
			t.Errorf("%s differs\nunexpected build statement %#v.\nexpected: %#v",
				differingField, actualStatement, expectedStatement)
			return
		}
	}
}

func buildStatementEquals(first *BuildStatement, second *BuildStatement) string {
	if (first == nil) != (second == nil) {
		return "Nil"
	}
	if first.Mnemonic != second.Mnemonic {
		return "Mnemonic"
	}
	if first.Command != second.Command {
		return "Command"
	}
	// Ordering is significant for environment variables.
	if !reflect.DeepEqual(first.Env, second.Env) {
		return "Env"
	}
	// Ordering is irrelevant for input and output paths, so compare sets.
	if !reflect.DeepEqual(sortedStrings(first.InputPaths), sortedStrings(second.InputPaths)) {
		return "InputPaths"
	}
	if !reflect.DeepEqual(sortedStrings(first.OutputPaths), sortedStrings(second.OutputPaths)) {
		return "OutputPaths"
	}
	if !reflect.DeepEqual(sortedStrings(first.SymlinkPaths), sortedStrings(second.SymlinkPaths)) {
		return "SymlinkPaths"
	}
	if !reflect.DeepEqual(sortedStrings(first.ImplicitDeps), sortedStrings(second.ImplicitDeps)) {
		return "ImplicitDeps"
	}
	if first.Depfile != second.Depfile {
		return "Depfile"
	}
	return ""
}

func sortedStrings(stringSlice []string) []string {
	sorted := make([]string, len(stringSlice))
	copy(sorted, stringSlice)
	sort.Strings(sorted)
	return sorted
}

// Transform the json format to ActionGraphContainer
func JsonToActionGraphContainer(inputString string) ([]byte, error) {
	var aqueryProtoResult analysis_v2_proto.ActionGraphContainer
	err := json.Unmarshal([]byte(inputString), &aqueryProtoResult)
	if err != nil {
		return []byte(""), err
	}
	data, _ := proto.Marshal(&aqueryProtoResult)
	return data, err
}
