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

// Implements the environment JSON file handling for serializing the
// environment variables that were used in soong_build so that soong_ui can
// check whether they have changed
package shared

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
)

type envFileEntry struct{ Key, Value string }
type envFileData []envFileEntry

// Serializes the given environment variable name/value map into JSON formatted bytes by converting
// to envFileEntry values and marshaling them.
//
// e.g. OUT_DIR = "out"
// is converted to:
//
//	{
//	    "Key": "OUT_DIR",
//	    "Value": "out",
//	},
func EnvFileContents(envDeps map[string]string) ([]byte, error) {
	contents := make(envFileData, 0, len(envDeps))
	for key, value := range envDeps {
		contents = append(contents, envFileEntry{key, value})
	}

	sort.Sort(contents)

	data, err := json.MarshalIndent(contents, "", "    ")
	if err != nil {
		return nil, err
	}

	data = append(data, '\n')

	return data, nil
}

// Reads and deserializes a Soong environment file located at the given file
// path to determine its staleness. If any environment variable values have
// changed, it prints and returns changed environment variable values and
// returns true.
// Failing to read or parse the file also causes it to return true.
func StaleEnvFile(filepath string, getenv func(string) string) (isStale bool,
	changedEnvironmentVariable []string, err error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return true, nil, err
	}

	var contents envFileData

	err = json.Unmarshal(data, &contents)
	if err != nil {
		return true, nil, err
	}

	var changed []string
	for _, entry := range contents {
		key := entry.Key
		old := entry.Value
		cur := getenv(key)
		if old != cur {
			changed = append(changed, fmt.Sprintf("%s (%q -> %q)", key, old, cur))
			changedEnvironmentVariable = append(changedEnvironmentVariable, key)
		}
	}

	if len(changed) > 0 {
		fmt.Printf("environment variables changed value:\n")
		for _, s := range changed {
			fmt.Printf("   %s\n", s)
		}
		return true, changedEnvironmentVariable, nil
	}

	return false, nil, nil
}

// Deserializes and environment serialized by EnvFileContents() and returns it
// as a map[string]string.
func EnvFromFile(envFile string) (map[string]string, error) {
	result := make(map[string]string)
	data, err := ioutil.ReadFile(envFile)
	if err != nil {
		return result, err
	}

	var contents envFileData
	err = json.Unmarshal(data, &contents)
	if err != nil {
		return result, err
	}

	for _, entry := range contents {
		result[entry.Key] = entry.Value
	}

	return result, nil
}

// Implements sort.Interface so that we can use sort.Sort on envFileData arrays.
func (e envFileData) Len() int {
	return len(e)
}

func (e envFileData) Less(i, j int) bool {
	return e[i].Key < e[j].Key
}

func (e envFileData) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

var _ sort.Interface = envFileData{}
