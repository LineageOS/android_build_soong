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
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func TestAqueryMultiArchGenrule(t *testing.T) {
	// This input string is retrieved from a real build of bionic-related genrules.
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 6 },
    { "id": 3, "pathFragmentId": 8 },
    { "id": 4, "pathFragmentId": 12 },
    { "id": 5, "pathFragmentId": 19 },
    { "id": 6, "pathFragmentId": 20 },
    { "id": 7, "pathFragmentId": 21 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "ab53f6ecbdc2ee8cb8812613b63205464f1f5083f6dca87081a0a398c0f1ecf7",
    "mnemonic": "Genrule",
    "configurationId": 1,
    "arguments": ["/bin/bash", "-c", "source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py arm ../sourceroot/bionic/libc/SYSCALLS.TXT \u003e bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-arm.S"],
    "environmentVariables": [{
      "key": "PATH",
      "value": "/bin:/usr/bin:/usr/local/bin"
    }],
    "inputDepSetIds": [1],
    "outputIds": [4],
    "primaryOutputId": 4
  }, {
    "targetId": 2,
    "actionKey": "9f4309ce165dac458498cb92811c18b0b7919782cc37b82a42d2141b8cc90826",
    "mnemonic": "Genrule",
    "configurationId": 1,
    "arguments": ["/bin/bash", "-c", "source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py x86 ../sourceroot/bionic/libc/SYSCALLS.TXT \u003e bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-x86.S"],
    "environmentVariables": [{
      "key": "PATH",
      "value": "/bin:/usr/bin:/usr/local/bin"
    }],
    "inputDepSetIds": [2],
    "outputIds": [5],
    "primaryOutputId": 5
  }, {
    "targetId": 3,
    "actionKey": "50d6c586103ebeed3a218195540bcc30d329464eae36377eb82f8ce7c36ac342",
    "mnemonic": "Genrule",
    "configurationId": 1,
    "arguments": ["/bin/bash", "-c", "source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py x86_64 ../sourceroot/bionic/libc/SYSCALLS.TXT \u003e bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-x86_64.S"],
    "environmentVariables": [{
      "key": "PATH",
      "value": "/bin:/usr/bin:/usr/local/bin"
    }],
    "inputDepSetIds": [3],
    "outputIds": [6],
    "primaryOutputId": 6
  }, {
    "targetId": 4,
    "actionKey": "f30cbe442f5216f4223cf16a39112cad4ec56f31f49290d85cff587e48647ffa",
    "mnemonic": "Genrule",
    "configurationId": 1,
    "arguments": ["/bin/bash", "-c", "source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py arm64 ../sourceroot/bionic/libc/SYSCALLS.TXT \u003e bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-arm64.S"],
    "environmentVariables": [{
      "key": "PATH",
      "value": "/bin:/usr/bin:/usr/local/bin"
    }],
    "inputDepSetIds": [4],
    "outputIds": [7],
    "primaryOutputId": 7
  }],
  "targets": [
    { "id": 1, "label": "@sourceroot//bionic/libc:syscalls-arm", "ruleClassId": 1 },
    { "id": 2, "label": "@sourceroot//bionic/libc:syscalls-x86", "ruleClassId": 1 },
    { "id": 3, "label": "@sourceroot//bionic/libc:syscalls-x86_64", "ruleClassId": 1 },
    { "id": 4, "label": "@sourceroot//bionic/libc:syscalls-arm64", "ruleClassId": 1 }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1, 2, 3] },
    { "id": 2, "directArtifactIds": [1, 2, 3] },
    { "id": 3, "directArtifactIds": [1, 2, 3] },
    { "id": 4, "directArtifactIds": [1, 2, 3] }],
  "configuration": [{
    "id": 1,
    "mnemonic": "k8-fastbuild",
    "platformName": "k8",
    "checksum": "485c362832c178e367d972177f68e69e0981e51e67ef1c160944473db53fe046"
  }],
  "ruleClasses": [{ "id": 1, "name": "genrule"}],
  "pathFragments": [
    { "id": 5, "label": ".." },
    { "id": 4, "label": "sourceroot", "parentId": 5 },
    { "id": 3, "label": "bionic", "parentId": 4 },
    { "id": 2, "label": "libc", "parentId": 3 },
    { "id": 1, "label": "SYSCALLS.TXT", "parentId": 2 },
    { "id": 7, "label": "tools", "parentId": 2 },
    { "id": 6, "label": "gensyscalls.py", "parentId": 7 },
    { "id": 11, "label": "bazel_tools", "parentId": 5 },
    { "id": 10, "label": "tools", "parentId": 11 },
    { "id": 9, "label": "genrule", "parentId": 10 },
    { "id": 8, "label": "genrule-setup.sh", "parentId": 9 },
    { "id": 18, "label": "bazel-out" },
    { "id": 17, "label": "sourceroot", "parentId": 18 },
    { "id": 16, "label": "k8-fastbuild", "parentId": 17 },
    { "id": 15, "label": "bin", "parentId": 16 },
    { "id": 14, "label": "bionic", "parentId": 15 },
    { "id": 13, "label": "libc", "parentId": 14 },
    { "id": 12, "label": "syscalls-arm.S", "parentId": 13 },
    { "id": 19, "label": "syscalls-x86.S", "parentId": 13 },
    { "id": 20, "label": "syscalls-x86_64.S", "parentId": 13 },
    { "id": 21, "label": "syscalls-arm64.S", "parentId": 13 }]
}`
	actualbuildStatements, actualDepsets, _ := AqueryBuildStatements([]byte(inputString))
	var expectedBuildStatements []BuildStatement
	for _, arch := range []string{"arm", "arm64", "x86", "x86_64"} {
		expectedBuildStatements = append(expectedBuildStatements,
			BuildStatement{
				Command: fmt.Sprintf(
					"/bin/bash -c 'source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py %s ../sourceroot/bionic/libc/SYSCALLS.TXT > bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-%s.S'",
					arch, arch),
				OutputPaths: []string{
					fmt.Sprintf("bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-%s.S", arch),
				},
				Env: []KeyValuePair{
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
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [3],
    "primaryOutputId": 3
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1, 2] }],
  "pathFragments": [
    { "id": 1, "label": "one" },
    { "id": 2, "label": "two" }]
}`

	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, "undefined outputId 3")
}

func TestInvalidInputDepsetIdFromAction(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [2],
    "outputIds": [1],
    "primaryOutputId": 1
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1, 2] }],
  "pathFragments": [
    { "id": 1, "label": "one" },
    { "id": 2, "label": "two" }]
}`

	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, "undefined input depsetId 2")
}

func TestInvalidInputDepsetIdFromDepset(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [1],
    "primaryOutputId": 1
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1, 2], "transitiveDepSetIds": [42] }],
  "pathFragments": [
    { "id": 1, "label": "one"},
    { "id": 2, "label": "two" }]
}`

	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, "undefined input depsetId 42 (referenced by depsetId 1)")
}

func TestInvalidInputArtifactId(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [1],
    "primaryOutputId": 1
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1, 3] }],
  "pathFragments": [
    { "id": 1, "label": "one" },
    { "id": 2, "label": "two" }]
}`

	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, "undefined input artifactId 3")
}

func TestInvalidPathFragmentId(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [1],
    "primaryOutputId": 1
  }],
  "depSetOfFiles": [
     { "id": 1, "directArtifactIds": [1, 2] }],
  "pathFragments": [
    {  "id": 1, "label": "one" },
    {  "id": 2, "label": "two", "parentId": 3 }]
}`

	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, "undefined path fragment id 3")
}

func TestDepfiles(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 },
    { "id": 3, "pathFragmentId": 3 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [2, 3],
    "primaryOutputId": 2
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1, 2, 3] }],
  "pathFragments": [
    { "id": 1, "label": "one" },
    { "id": 2, "label": "two" },
    { "id": 3, "label": "two.d" }]
}`

	actual, _, err := AqueryBuildStatements([]byte(inputString))
	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}
	if expected := 1; len(actual) != expected {
		t.Fatalf("Expected %d build statements, got %d", expected, len(actual))
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
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 },
    { "id": 3, "pathFragmentId": 3 },
    { "id": 4, "pathFragmentId": 4 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [2,3,4],
    "primaryOutputId": 2
  }],
  "depSetOfFiles": [{
    "id": 1,
    "directArtifactIds": [1, 2, 3, 4]
  }],
  "pathFragments": [
    { "id": 1, "label": "one" },
    { "id": 2, "label": "two" },
    { "id": 3, "label": "two.d" },
    { "id": 4, "label": "other.d" }]
}`

	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, `found multiple potential depfiles "two.d", "other.d"`)
}

func TestTransitiveInputDepsets(t *testing.T) {
	// The input aquery for this test comes from a proof-of-concept starlark rule which registers
	// a single action with many inputs given via a deep depset.
	const inputString = `
{
  "artifacts": [
   { "id": 1, "pathFragmentId": 1 },
   { "id": 2, "pathFragmentId": 7 },
   { "id": 3, "pathFragmentId": 8 },
   { "id": 4, "pathFragmentId": 9 },
   { "id": 5, "pathFragmentId": 10 },
   { "id": 6, "pathFragmentId": 11 },
   { "id": 7, "pathFragmentId": 12 },
   { "id": 8, "pathFragmentId": 13 },
   { "id": 9, "pathFragmentId": 14 },
   { "id": 10, "pathFragmentId": 15 },
   { "id": 11, "pathFragmentId": 16 },
   { "id": 12, "pathFragmentId": 17 },
   { "id": 13, "pathFragmentId": 18 },
   { "id": 14, "pathFragmentId": 19 },
   { "id": 15, "pathFragmentId": 20 },
   { "id": 16, "pathFragmentId": 21 },
   { "id": 17, "pathFragmentId": 22 },
   { "id": 18, "pathFragmentId": 23 },
   { "id": 19, "pathFragmentId": 24 },
   { "id": 20, "pathFragmentId": 25 },
   { "id": 21, "pathFragmentId": 26 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "3b826d17fadbbbcd8313e456b90ec47c078c438088891dd45b4adbcd8889dc50",
    "mnemonic": "Action",
    "configurationId": 1,
    "arguments": ["/bin/bash", "-c", "touch bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_out"],
    "inputDepSetIds": [1],
    "outputIds": [21],
    "primaryOutputId": 21
  }],
  "depSetOfFiles": [
    { "id": 3, "directArtifactIds": [1, 2, 3, 4, 5] },
    { "id": 4, "directArtifactIds": [6, 7, 8, 9, 10] },
    { "id": 2, "transitiveDepSetIds": [3, 4], "directArtifactIds": [11, 12, 13, 14, 15] },
    { "id": 5, "directArtifactIds": [16, 17, 18, 19] },
    { "id": 1, "transitiveDepSetIds": [2, 5], "directArtifactIds": [20] }],
  "pathFragments": [
    { "id": 6, "label": "bazel-out" },
    { "id": 5, "label": "sourceroot", "parentId": 6 },
    { "id": 4, "label": "k8-fastbuild", "parentId": 5 },
    { "id": 3, "label": "bin", "parentId": 4 },
    { "id": 2, "label": "testpkg", "parentId": 3 },
    { "id": 1, "label": "test_1", "parentId": 2 },
    { "id": 7, "label": "test_2", "parentId": 2 },
    { "id": 8, "label": "test_3", "parentId": 2 },
    { "id": 9, "label": "test_4", "parentId": 2 },
    { "id": 10, "label": "test_5", "parentId": 2 },
    { "id": 11, "label": "test_6", "parentId": 2 },
    { "id": 12, "label": "test_7", "parentId": 2 },
    { "id": 13, "label": "test_8", "parentId": 2 },
    { "id": 14, "label": "test_9", "parentId": 2 },
    { "id": 15, "label": "test_10", "parentId": 2 },
    { "id": 16, "label": "test_11", "parentId": 2 },
    { "id": 17, "label": "test_12", "parentId": 2 },
    { "id": 18, "label": "test_13", "parentId": 2 },
    { "id": 19, "label": "test_14", "parentId": 2 },
    { "id": 20, "label": "test_15", "parentId": 2 },
    { "id": 21, "label": "test_16", "parentId": 2 },
    { "id": 22, "label": "test_17", "parentId": 2 },
    { "id": 23, "label": "test_18", "parentId": 2 },
    { "id": 24, "label": "test_19", "parentId": 2 },
    { "id": 25, "label": "test_root", "parentId": 2 },
    { "id": 26,"label": "test_out", "parentId": 2 }]
}`

	actualbuildStatements, actualDepsets, _ := AqueryBuildStatements([]byte(inputString))

	expectedBuildStatements := []BuildStatement{
		{
			Command:     "/bin/bash -c 'touch bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_out'",
			OutputPaths: []string{"bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_out"},
			Mnemonic:    "Action",
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

func TestBazelOutRemovalFromInputDepsets(t *testing.T) {
	const inputString = `{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 10
  }, {
    "id": 2,
    "pathFragmentId": 20
  }, {
    "id": 3,
    "pathFragmentId": 30
  }, {
    "id": 4,
    "pathFragmentId": 40
  }],
  "depSetOfFiles": [{
    "id": 1111,
    "directArtifactIds": [3 , 4]
  }],
  "actions": [{
    "targetId": 100,
    "actionKey": "x",
    "inputDepSetIds": [1111],
    "mnemonic": "x",
    "arguments": ["bogus", "command"],
    "outputIds": [2],
    "primaryOutputId": 1
  }],
  "pathFragments": [{
    "id": 10,
    "label": "input"
  }, {
    "id": 20,
    "label": "output"
  }, {
    "id": 30,
    "label": "dep1",
    "parentId": 50
  }, {
    "id": 40,
    "label": "dep2",
    "parentId": 60
  }, {
    "id": 50,
    "label": "bazel_tools",
    "parentId": 60
  }, {
    "id": 60,
    "label": ".."
  }]
}`
	actualBuildStatements, actualDepsets, _ := AqueryBuildStatements([]byte(inputString))
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

	expectedBuildStatement := BuildStatement{
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
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 },
    { "id": 3, "pathFragmentId": 3 },
    { "id": 4, "pathFragmentId": 4 },
    { "id": 5, "pathFragmentId": 5 },
    { "id": 6, "pathFragmentId": 6 }],
  "pathFragments": [
    { "id": 1, "label": "middleinput_one" },
    { "id": 2, "label": "middleinput_two" },
    { "id": 3, "label": "middleman_artifact" },
    { "id": 4, "label": "maininput_one" },
    { "id": 5, "label": "maininput_two" },
    { "id": 6, "label": "output" }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1, 2] },
    { "id": 2, "directArtifactIds": [3, 4, 5] }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "Middleman",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [3],
    "primaryOutputId": 3
  }, {
    "targetId": 2,
    "actionKey": "y",
    "mnemonic": "Main action",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [2],
    "outputIds": [6],
    "primaryOutputId": 6
  }]
}`

	actualBuildStatements, actualDepsets, err := AqueryBuildStatements([]byte(inputString))
	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}
	if expected := 1; len(actualBuildStatements) != expected {
		t.Fatalf("Expected %d build statements, got %d", expected, len(actualBuildStatements))
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
    { "id": 1, "pathFragmentId": 3 },
    { "id": 2, "pathFragmentId": 5 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "Symlink",
    "inputDepSetIds": [1],
    "outputIds": [2],
    "primaryOutputId": 2
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1] }],
  "pathFragments": [
    { "id": 1, "label": "one" },
    { "id": 2, "label": "file_subdir", "parentId": 1 },
    { "id": 3, "label": "file", "parentId": 2 },
    { "id": 4, "label": "symlink_subdir", "parentId": 1 },
    { "id": 5, "label": "symlink", "parentId": 4 }]
}`

	actual, _, err := AqueryBuildStatements([]byte(inputString))

	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}

	expectedBuildStatements := []BuildStatement{
		{
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
    { "id": 1, "pathFragmentId": 3 },
    { "id": 2, "pathFragmentId": 5 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "SolibSymlink",
    "inputDepSetIds": [1],
    "outputIds": [2],
    "primaryOutputId": 2
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1] }],
  "pathFragments": [
    { "id": 1, "label": "one" },
    { "id": 2, "label": "file subdir", "parentId": 1 },
    { "id": 3, "label": "file", "parentId": 2 },
    { "id": 4, "label": "symlink subdir", "parentId": 1 },
    { "id": 5, "label": "symlink", "parentId": 4 }]
}`

	actual, _, err := AqueryBuildStatements([]byte(inputString))

	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}

	expectedBuildStatements := []BuildStatement{
		{
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
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 },
    { "id": 3, "pathFragmentId": 3 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "Symlink",
    "inputDepSetIds": [1],
    "outputIds": [3],
    "primaryOutputId": 3
  }],
  "depSetOfFiles": [{ "id": 1, "directArtifactIds": [1,2] }],
  "pathFragments": [
    { "id": 1, "label": "file" },
    { "id": 2, "label": "other_file" },
    { "id": 3, "label": "symlink" }]
}`

	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, `Expect 1 input and 1 output to symlink action, got: input ["file" "other_file"], output ["symlink"]`)
}

func TestSymlinkMultipleOutputs(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 },
    { "id": 3, "pathFragmentId": 3 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "Symlink",
    "inputDepSetIds": [1],
    "outputIds": [2,3],
    "primaryOutputId": 2
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [1] }],
  "pathFragments": [
    { "id": 1, "label": "file" },
    { "id": 2, "label": "symlink" },
    { "id": 3,  "label": "other_symlink" }]
}`

	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, `Expect 1 input and 1 output to symlink action, got: input ["file"], output ["symlink" "other_symlink"]`)
}

func TestTemplateExpandActionSubstitutions(t *testing.T) {
	const inputString = `
{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 1
  }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "TemplateExpand",
    "configurationId": 1,
    "outputIds": [1],
    "primaryOutputId": 1,
    "executionPlatform": "//build/bazel/platforms:linux_x86_64",
    "templateContent": "Test template substitutions: %token1%, %python_binary%",
    "substitutions": [
      { "key": "%token1%", "value": "abcd" },
      { "key": "%python_binary%", "value": "python3" }]
  }],
  "pathFragments": [
    { "id": 1, "label": "template_file" }]
}`

	actual, _, err := AqueryBuildStatements([]byte(inputString))

	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}

	expectedBuildStatements := []BuildStatement{
		{
			Command: "/bin/bash -c 'echo \"Test template substitutions: abcd, python3\" | sed \"s/\\\\\\\\n/\\\\n/g\" > template_file && " +
				"chmod a+x template_file'",
			OutputPaths: []string{"template_file"},
			Mnemonic:    "TemplateExpand",
		},
	}
	assertBuildStatements(t, expectedBuildStatements, actual)
}

func TestTemplateExpandActionNoOutput(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "TemplateExpand",
    "configurationId": 1,
    "primaryOutputId": 1,
    "executionPlatform": "//build/bazel/platforms:linux_x86_64",
    "templateContent": "Test template substitutions: %token1%, %python_binary%",
    "substitutions": [
      { "key": "%token1%", "value": "abcd" },
      { "key": "%python_binary%", "value": "python3" }]
  }],
  "pathFragments": [
    { "id": 1, "label": "template_file" }]
}`

	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, `Expect 1 output to template expand action, got: output []`)
}

func TestPythonZipperActionSuccess(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 },
    { "id": 3, "pathFragmentId": 3 },
    { "id": 4, "pathFragmentId": 4 },
    { "id": 5, "pathFragmentId": 10 },
    { "id": 10, "pathFragmentId": 20 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "TemplateExpand",
    "configurationId": 1,
    "outputIds": [1],
    "primaryOutputId": 1,
    "executionPlatform": "//build/bazel/platforms:linux_x86_64",
    "templateContent": "Test template substitutions: %token1%, %python_binary%",
    "substitutions": [{
      "key": "%token1%",
      "value": "abcd"
    },{
      "key": "%python_binary%",
      "value": "python3"
    }]
  },{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "PythonZipper",
    "configurationId": 1,
    "arguments": ["../bazel_tools/tools/zip/zipper/zipper", "cC", "python_binary.zip", "__main__.py\u003dbazel-out/k8-fastbuild/bin/python_binary.temp", "__init__.py\u003d", "runfiles/__main__/__init__.py\u003d", "runfiles/__main__/python_binary.py\u003dpython_binary.py", "runfiles/bazel_tools/tools/python/py3wrapper.sh\u003dbazel-out/bazel_tools/k8-fastbuild/bin/tools/python/py3wrapper.sh"],
    "outputIds": [2],
    "inputDepSetIds": [1],
    "primaryOutputId": 2
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [4, 3, 5] }],
  "pathFragments": [
    { "id": 1, "label": "python_binary" },
    { "id": 2, "label": "python_binary.zip" },
    { "id": 3, "label": "python_binary.py" },
    { "id": 9, "label": ".." },
    { "id": 8, "label": "bazel_tools", "parentId": 9 },
    { "id": 7, "label": "tools", "parentId": 8 },
    { "id": 6, "label": "zip", "parentId": 7  },
    { "id": 5, "label": "zipper", "parentId": 6 },
    { "id": 4, "label": "zipper", "parentId": 5 },
    { "id": 16, "label": "bazel-out" },
    { "id": 15, "label": "bazel_tools", "parentId": 16 },
    { "id": 14, "label": "k8-fastbuild", "parentId": 15 },
    { "id": 13, "label": "bin", "parentId": 14 },
    { "id": 12, "label": "tools", "parentId": 13 },
    { "id": 11, "label": "python", "parentId": 12 },
    { "id": 10, "label": "py3wrapper.sh", "parentId": 11 },
    { "id": 20, "label": "python_binary" }]
}`
	actual, _, err := AqueryBuildStatements([]byte(inputString))

	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}

	expectedBuildStatements := []BuildStatement{
		{
			Command: "/bin/bash -c 'echo \"Test template substitutions: abcd, python3\" | sed \"s/\\\\\\\\n/\\\\n/g\" > python_binary && " +
				"chmod a+x python_binary'",
			InputPaths:  []string{"python_binary.zip"},
			OutputPaths: []string{"python_binary"},
			Mnemonic:    "TemplateExpand",
		},
		{
			Command: "../bazel_tools/tools/zip/zipper/zipper cC python_binary.zip __main__.py=bazel-out/k8-fastbuild/bin/python_binary.temp " +
				"__init__.py= runfiles/__main__/__init__.py= runfiles/__main__/python_binary.py=python_binary.py  && " +
				"../bazel_tools/tools/zip/zipper/zipper x python_binary.zip -d python_binary.runfiles && ln -sf runfiles/__main__ python_binary.runfiles",
			InputPaths:  []string{"python_binary.py"},
			OutputPaths: []string{"python_binary.zip"},
			Mnemonic:    "PythonZipper",
		},
	}
	assertBuildStatements(t, expectedBuildStatements, actual)
}

func TestPythonZipperActionNoInput(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "PythonZipper",
    "configurationId": 1,
    "arguments": ["../bazel_tools/tools/zip/zipper/zipper", "cC", "python_binary.zip", "__main__.py\u003dbazel-out/k8-fastbuild/bin/python_binary.temp", "__init__.py\u003d", "runfiles/__main__/__init__.py\u003d", "runfiles/__main__/python_binary.py\u003dpython_binary.py", "runfiles/bazel_tools/tools/python/py3wrapper.sh\u003dbazel-out/bazel_tools/k8-fastbuild/bin/tools/python/py3wrapper.sh"],
    "outputIds": [2],
    "primaryOutputId": 2
  }],
  "pathFragments": [
    { "id": 1, "label": "python_binary" },
    { "id": 2, "label": "python_binary.zip" }]
}`
	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, `Expect 1+ input and 1 output to python zipper action, got: input [], output ["python_binary.zip"]`)
}

func TestPythonZipperActionNoOutput(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 },
    { "id": 2, "pathFragmentId": 2 },
    { "id": 3, "pathFragmentId": 3 },
    { "id": 4, "pathFragmentId": 4 },
    { "id": 5, "pathFragmentId": 10 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "PythonZipper",
    "configurationId": 1,
    "arguments": ["../bazel_tools/tools/zip/zipper/zipper", "cC", "python_binary.zip", "__main__.py\u003dbazel-out/k8-fastbuild/bin/python_binary.temp", "__init__.py\u003d", "runfiles/__main__/__init__.py\u003d", "runfiles/__main__/python_binary.py\u003dpython_binary.py", "runfiles/bazel_tools/tools/python/py3wrapper.sh\u003dbazel-out/bazel_tools/k8-fastbuild/bin/tools/python/py3wrapper.sh"],
    "inputDepSetIds": [1]
  }],
  "depSetOfFiles": [
    { "id": 1, "directArtifactIds": [4, 3, 5]}],
  "pathFragments": [
    { "id": 1, "label": "python_binary" },
    { "id": 2, "label": "python_binary.zip" },
    { "id": 3, "label": "python_binary.py" },
    { "id": 9, "label": ".." },
    { "id": 8, "label": "bazel_tools", "parentId": 9 },
    { "id": 7, "label": "tools", "parentId": 8 },
    { "id": 6, "label": "zip", "parentId": 7 },
    { "id": 5, "label": "zipper", "parentId": 6 },
    { "id": 4, "label": "zipper", "parentId": 5 },
    { "id": 16, "label": "bazel-out" },
    { "id": 15, "label": "bazel_tools", "parentId": 16 },
    { "id": 14, "label": "k8-fastbuild", "parentId": 15 },
    { "id": 13, "label": "bin", "parentId": 14 },
    { "id": 12, "label": "tools", "parentId": 13 },
    { "id": 11, "label": "python", "parentId": 12 },
    { "id": 10, "label": "py3wrapper.sh", "parentId": 11 }]
}`
	_, _, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, `Expect 1+ input and 1 output to python zipper action, got: input ["python_binary.py"], output []`)
}

func TestFileWrite(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "FileWrite",
    "configurationId": 1,
    "outputIds": [1],
    "primaryOutputId": 1,
    "executionPlatform": "//build/bazel/platforms:linux_x86_64",
    "fileContents": "file data\n"
  }],
  "pathFragments": [
    { "id": 1, "label": "foo.manifest" }]
}
`
	actual, _, err := AqueryBuildStatements([]byte(inputString))
	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}
	assertBuildStatements(t, []BuildStatement{
		{
			OutputPaths:  []string{"foo.manifest"},
			Mnemonic:     "FileWrite",
			FileContents: "file data\n",
		},
	}, actual)
}

func TestSourceSymlinkManifest(t *testing.T) {
	const inputString = `
{
  "artifacts": [
    { "id": 1, "pathFragmentId": 1 }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "SourceSymlinkManifest",
    "configurationId": 1,
    "outputIds": [1],
    "primaryOutputId": 1,
    "executionPlatform": "//build/bazel/platforms:linux_x86_64",
    "fileContents": "symlink target\n"
  }],
  "pathFragments": [
    { "id": 1, "label": "foo.manifest" }]
}
`
	actual, _, err := AqueryBuildStatements([]byte(inputString))
	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}
	assertBuildStatements(t, []BuildStatement{
		{
			OutputPaths: []string{"foo.manifest"},
			Mnemonic:    "SourceSymlinkManifest",
		},
	}, actual)
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
func assertBuildStatements(t *testing.T, expected []BuildStatement, actual []BuildStatement) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("expected %d build statements, but got %d,\n expected: %#v,\n actual: %#v",
			len(expected), len(actual), expected, actual)
		return
	}
	type compareFn = func(i int, j int) bool
	byCommand := func(slice []BuildStatement) compareFn {
		return func(i int, j int) bool {
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

func buildStatementEquals(first BuildStatement, second BuildStatement) string {
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
