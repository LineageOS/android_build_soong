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
	"testing"
)

func TestAqueryMultiArchGenrule(t *testing.T) {
	// This input string is retrieved from a real build of bionic-related genrules.
	const inputString = `
{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 1
  }, {
    "id": 2,
    "pathFragmentId": 6
  }, {
    "id": 3,
    "pathFragmentId": 8
  }, {
    "id": 4,
    "pathFragmentId": 12
  }, {
    "id": 5,
    "pathFragmentId": 19
  }, {
    "id": 6,
    "pathFragmentId": 20
  }, {
    "id": 7,
    "pathFragmentId": 21
  }],
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
  "targets": [{
    "id": 1,
    "label": "@sourceroot//bionic/libc:syscalls-arm",
    "ruleClassId": 1
  }, {
    "id": 2,
    "label": "@sourceroot//bionic/libc:syscalls-x86",
    "ruleClassId": 1
  }, {
    "id": 3,
    "label": "@sourceroot//bionic/libc:syscalls-x86_64",
    "ruleClassId": 1
  }, {
    "id": 4,
    "label": "@sourceroot//bionic/libc:syscalls-arm64",
    "ruleClassId": 1
  }],
  "depSetOfFiles": [{
    "id": 1,
    "directArtifactIds": [1, 2, 3]
  }, {
    "id": 2,
    "directArtifactIds": [1, 2, 3]
  }, {
    "id": 3,
    "directArtifactIds": [1, 2, 3]
  }, {
    "id": 4,
    "directArtifactIds": [1, 2, 3]
  }],
  "configuration": [{
    "id": 1,
    "mnemonic": "k8-fastbuild",
    "platformName": "k8",
    "checksum": "485c362832c178e367d972177f68e69e0981e51e67ef1c160944473db53fe046"
  }],
  "ruleClasses": [{
    "id": 1,
    "name": "genrule"
  }],
  "pathFragments": [{
    "id": 5,
    "label": ".."
  }, {
    "id": 4,
    "label": "sourceroot",
    "parentId": 5
  }, {
    "id": 3,
    "label": "bionic",
    "parentId": 4
  }, {
    "id": 2,
    "label": "libc",
    "parentId": 3
  }, {
    "id": 1,
    "label": "SYSCALLS.TXT",
    "parentId": 2
  }, {
    "id": 7,
    "label": "tools",
    "parentId": 2
  }, {
    "id": 6,
    "label": "gensyscalls.py",
    "parentId": 7
  }, {
    "id": 11,
    "label": "bazel_tools",
    "parentId": 5
  }, {
    "id": 10,
    "label": "tools",
    "parentId": 11
  }, {
    "id": 9,
    "label": "genrule",
    "parentId": 10
  }, {
    "id": 8,
    "label": "genrule-setup.sh",
    "parentId": 9
  }, {
    "id": 18,
    "label": "bazel-out"
  }, {
    "id": 17,
    "label": "sourceroot",
    "parentId": 18
  }, {
    "id": 16,
    "label": "k8-fastbuild",
    "parentId": 17
  }, {
    "id": 15,
    "label": "bin",
    "parentId": 16
  }, {
    "id": 14,
    "label": "bionic",
    "parentId": 15
  }, {
    "id": 13,
    "label": "libc",
    "parentId": 14
  }, {
    "id": 12,
    "label": "syscalls-arm.S",
    "parentId": 13
  }, {
    "id": 19,
    "label": "syscalls-x86.S",
    "parentId": 13
  }, {
    "id": 20,
    "label": "syscalls-x86_64.S",
    "parentId": 13
  }, {
    "id": 21,
    "label": "syscalls-arm64.S",
    "parentId": 13
  }]
}`
	actualbuildStatements, _ := AqueryBuildStatements([]byte(inputString))
	expectedBuildStatements := []BuildStatement{}
	for _, arch := range []string{"arm", "arm64", "x86", "x86_64"} {
		expectedBuildStatements = append(expectedBuildStatements,
			BuildStatement{
				Command: fmt.Sprintf(
					"/bin/bash -c 'source ../bazel_tools/tools/genrule/genrule-setup.sh; ../sourceroot/bionic/libc/tools/gensyscalls.py %s ../sourceroot/bionic/libc/SYSCALLS.TXT > bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-%s.S'",
					arch, arch),
				OutputPaths: []string{
					fmt.Sprintf("bazel-out/sourceroot/k8-fastbuild/bin/bionic/libc/syscalls-%s.S", arch),
				},
				InputPaths: []string{
					"../sourceroot/bionic/libc/SYSCALLS.TXT",
					"../sourceroot/bionic/libc/tools/gensyscalls.py",
					"../bazel_tools/tools/genrule/genrule-setup.sh",
				},
				Env: []KeyValuePair{
					KeyValuePair{Key: "PATH", Value: "/bin:/usr/bin:/usr/local/bin"},
				},
				Mnemonic: "Genrule",
			})
	}
	assertBuildStatements(t, expectedBuildStatements, actualbuildStatements)
}

func TestInvalidOutputId(t *testing.T) {
	const inputString = `
{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 1
  }, {
    "id": 2,
    "pathFragmentId": 2
  }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [3],
    "primaryOutputId": 3
  }],
  "depSetOfFiles": [{
    "id": 1,
    "directArtifactIds": [1, 2]
  }],
  "pathFragments": [{
    "id": 1,
    "label": "one"
  }, {
    "id": 2,
    "label": "two"
  }]
}`

	_, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, "undefined outputId 3")
}

func TestInvalidInputDepsetId(t *testing.T) {
	const inputString = `
{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 1
  }, {
    "id": 2,
    "pathFragmentId": 2
  }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [2],
    "outputIds": [1],
    "primaryOutputId": 1
  }],
  "depSetOfFiles": [{
    "id": 1,
    "directArtifactIds": [1, 2]
  }],
  "pathFragments": [{
    "id": 1,
    "label": "one"
  }, {
    "id": 2,
    "label": "two"
  }]
}`

	_, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, "undefined input depsetId 2")
}

func TestInvalidInputArtifactId(t *testing.T) {
	const inputString = `
{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 1
  }, {
    "id": 2,
    "pathFragmentId": 2
  }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [1],
    "primaryOutputId": 1
  }],
  "depSetOfFiles": [{
    "id": 1,
    "directArtifactIds": [1, 3]
  }],
  "pathFragments": [{
    "id": 1,
    "label": "one"
  }, {
    "id": 2,
    "label": "two"
  }]
}`

	_, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, "undefined input artifactId 3")
}

func TestInvalidPathFragmentId(t *testing.T) {
	const inputString = `
{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 1
  }, {
    "id": 2,
    "pathFragmentId": 2
  }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [1],
    "primaryOutputId": 1
  }],
  "depSetOfFiles": [{
    "id": 1,
    "directArtifactIds": [1, 2]
  }],
  "pathFragments": [{
    "id": 1,
    "label": "one"
  }, {
    "id": 2,
    "label": "two",
		"parentId": 3
  }]
}`

	_, err := AqueryBuildStatements([]byte(inputString))
	assertError(t, err, "undefined path fragment id 3")
}

func TestTransitiveInputDepsets(t *testing.T) {
	// The input aquery for this test comes from a proof-of-concept starlark rule which registers
	// a single action with many inputs given via a deep depset.
	const inputString = `
{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 1
  }, {
    "id": 2,
    "pathFragmentId": 7
  }, {
    "id": 3,
    "pathFragmentId": 8
  }, {
    "id": 4,
    "pathFragmentId": 9
  }, {
    "id": 5,
    "pathFragmentId": 10
  }, {
    "id": 6,
    "pathFragmentId": 11
  }, {
    "id": 7,
    "pathFragmentId": 12
  }, {
    "id": 8,
    "pathFragmentId": 13
  }, {
    "id": 9,
    "pathFragmentId": 14
  }, {
    "id": 10,
    "pathFragmentId": 15
  }, {
    "id": 11,
    "pathFragmentId": 16
  }, {
    "id": 12,
    "pathFragmentId": 17
  }, {
    "id": 13,
    "pathFragmentId": 18
  }, {
    "id": 14,
    "pathFragmentId": 19
  }, {
    "id": 15,
    "pathFragmentId": 20
  }, {
    "id": 16,
    "pathFragmentId": 21
  }, {
    "id": 17,
    "pathFragmentId": 22
  }, {
    "id": 18,
    "pathFragmentId": 23
  }, {
    "id": 19,
    "pathFragmentId": 24
  }, {
    "id": 20,
    "pathFragmentId": 25
  }, {
    "id": 21,
    "pathFragmentId": 26
  }],
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
  "depSetOfFiles": [{
    "id": 3,
    "directArtifactIds": [1, 2, 3, 4, 5]
  }, {
    "id": 4,
    "directArtifactIds": [6, 7, 8, 9, 10]
  }, {
    "id": 2,
    "transitiveDepSetIds": [3, 4],
    "directArtifactIds": [11, 12, 13, 14, 15]
  }, {
    "id": 5,
    "directArtifactIds": [16, 17, 18, 19]
  }, {
    "id": 1,
    "transitiveDepSetIds": [2, 5],
    "directArtifactIds": [20]
  }],
  "pathFragments": [{
    "id": 6,
    "label": "bazel-out"
  }, {
    "id": 5,
    "label": "sourceroot",
    "parentId": 6
  }, {
    "id": 4,
    "label": "k8-fastbuild",
    "parentId": 5
  }, {
    "id": 3,
    "label": "bin",
    "parentId": 4
  }, {
    "id": 2,
    "label": "testpkg",
    "parentId": 3
  }, {
    "id": 1,
    "label": "test_1",
    "parentId": 2
  }, {
    "id": 7,
    "label": "test_2",
    "parentId": 2
  }, {
    "id": 8,
    "label": "test_3",
    "parentId": 2
  }, {
    "id": 9,
    "label": "test_4",
    "parentId": 2
  }, {
    "id": 10,
    "label": "test_5",
    "parentId": 2
  }, {
    "id": 11,
    "label": "test_6",
    "parentId": 2
  }, {
    "id": 12,
    "label": "test_7",
    "parentId": 2
  }, {
    "id": 13,
    "label": "test_8",
    "parentId": 2
  }, {
    "id": 14,
    "label": "test_9",
    "parentId": 2
  }, {
    "id": 15,
    "label": "test_10",
    "parentId": 2
  }, {
    "id": 16,
    "label": "test_11",
    "parentId": 2
  }, {
    "id": 17,
    "label": "test_12",
    "parentId": 2
  }, {
    "id": 18,
    "label": "test_13",
    "parentId": 2
  }, {
    "id": 19,
    "label": "test_14",
    "parentId": 2
  }, {
    "id": 20,
    "label": "test_15",
    "parentId": 2
  }, {
    "id": 21,
    "label": "test_16",
    "parentId": 2
  }, {
    "id": 22,
    "label": "test_17",
    "parentId": 2
  }, {
    "id": 23,
    "label": "test_18",
    "parentId": 2
  }, {
    "id": 24,
    "label": "test_19",
    "parentId": 2
  }, {
    "id": 25,
    "label": "test_root",
    "parentId": 2
  }, {
    "id": 26,
    "label": "test_out",
    "parentId": 2
  }]
}`

	actualbuildStatements, _ := AqueryBuildStatements([]byte(inputString))
	// Inputs for the action are test_{i} from 1 to 20, and test_root. These inputs
	// are given via a deep depset, but the depset is flattened when returned as a
	// BuildStatement slice.
	inputPaths := []string{"bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_root"}
	for i := 1; i < 20; i++ {
		inputPaths = append(inputPaths, fmt.Sprintf("bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_%d", i))
	}
	expectedBuildStatements := []BuildStatement{
		BuildStatement{
			Command:     "/bin/bash -c touch bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_out",
			OutputPaths: []string{"bazel-out/sourceroot/k8-fastbuild/bin/testpkg/test_out"},
			InputPaths:  inputPaths,
			Mnemonic:    "Action",
		},
	}
	assertBuildStatements(t, expectedBuildStatements, actualbuildStatements)
}

func assertError(t *testing.T, err error, expected string) {
	if err == nil {
		t.Errorf("expected error '%s', but got no error", expected)
	} else if err.Error() != expected {
		t.Errorf("expected error '%s', but got: %s", expected, err.Error())
	}
}

// Asserts that the given actual build statements match the given expected build statements.
// Build statement equivalence is determined using buildStatementEquals.
func assertBuildStatements(t *testing.T, expected []BuildStatement, actual []BuildStatement) {
	if len(expected) != len(actual) {
		t.Errorf("expected %d build statements, but got %d,\n expected: %s,\n actual: %s",
			len(expected), len(actual), expected, actual)
		return
	}
ACTUAL_LOOP:
	for _, actualStatement := range actual {
		for _, expectedStatement := range expected {
			if buildStatementEquals(actualStatement, expectedStatement) {
				continue ACTUAL_LOOP
			}
		}
		t.Errorf("unexpected build statement %s.\n expected: %s",
			actualStatement, expected)
		return
	}
}

func buildStatementEquals(first BuildStatement, second BuildStatement) bool {
	if first.Mnemonic != second.Mnemonic {
		return false
	}
	if first.Command != second.Command {
		return false
	}
	// Ordering is significant for environment variables.
	if !reflect.DeepEqual(first.Env, second.Env) {
		return false
	}
	// Ordering is irrelevant for input and output paths, so compare sets.
	if !reflect.DeepEqual(stringSet(first.InputPaths), stringSet(second.InputPaths)) {
		return false
	}
	if !reflect.DeepEqual(stringSet(first.OutputPaths), stringSet(second.OutputPaths)) {
		return false
	}
	return true
}

func stringSet(stringSlice []string) map[string]struct{} {
	stringMap := make(map[string]struct{})
	for _, s := range stringSlice {
		stringMap[s] = struct{}{}
	}
	return stringMap
}
