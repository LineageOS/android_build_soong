// Copyright 2017 Google Inc. All rights reserved.
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

package build

import (
	"os"
	"path/filepath"
	"strings"
)

func absPath(ctx Context, p string) string {
	ret, err := filepath.Abs(p)
	if err != nil {
		ctx.Fatalf("Failed to get absolute path: %v", err)
	}
	return ret
}

// indexList finds the index of a string in a []string
func indexList(s string, list []string) int {
	for i, l := range list {
		if l == s {
			return i
		}
	}

	return -1
}

// inList determines whether a string is in a []string
func inList(s string, list []string) bool {
	return indexList(s, list) != -1
}

// removeFromlist removes all occurrences of the string in list.
func removeFromList(s string, list []string) []string {
	filteredList := make([]string, 0, len(list))
	for _, ls := range list {
		if s != ls {
			filteredList = append(filteredList, ls)
		}
	}
	return filteredList
}

// ensureDirectoriesExist is a shortcut to os.MkdirAll, sending errors to the ctx logger.
func ensureDirectoriesExist(ctx Context, dirs ...string) {
	for _, dir := range dirs {
		err := os.MkdirAll(dir, 0777)
		if err != nil {
			ctx.Fatalf("Error creating %s: %q\n", dir, err)
		}
	}
}

// ensureEmptyDirectoriesExist ensures that the given directories exist and are empty
func ensureEmptyDirectoriesExist(ctx Context, dirs ...string) {
	// remove all the directories
	for _, dir := range dirs {
		seenErr := map[string]bool{}
		for {
			err := os.RemoveAll(dir)
			if err == nil {
				break
			}

			if pathErr, ok := err.(*os.PathError); !ok ||
				dir == pathErr.Path || seenErr[pathErr.Path] {

				ctx.Fatalf("Error removing %s: %q\n", dir, err)
			} else {
				seenErr[pathErr.Path] = true
				err = os.Chmod(filepath.Dir(pathErr.Path), 0700)
				if err != nil {
					ctx.Fatal(err)
				}
			}
		}
	}
	// recreate all the directories
	ensureDirectoriesExist(ctx, dirs...)
}

// ensureEmptyFileExists ensures that the containing directory exists, and the
// specified file exists. If it doesn't exist, it will write an empty file.
func ensureEmptyFileExists(ctx Context, file string) {
	ensureDirectoriesExist(ctx, filepath.Dir(file))
	if _, err := os.Stat(file); os.IsNotExist(err) {
		f, err := os.Create(file)
		if err != nil {
			ctx.Fatalf("Error creating %s: %q\n", file, err)
		}
		f.Close()
	} else if err != nil {
		ctx.Fatalf("Error checking %s: %q\n", file, err)
	}
}

// singleUnquote is similar to strconv.Unquote, but can handle multi-character strings inside single quotes.
func singleUnquote(str string) (string, bool) {
	if len(str) < 2 || str[0] != '\'' || str[len(str)-1] != '\'' {
		return "", false
	}
	return str[1 : len(str)-1], true
}

// decodeKeyValue decodes a key=value string
func decodeKeyValue(str string) (string, string, bool) {
	idx := strings.IndexRune(str, '=')
	if idx == -1 {
		return "", "", false
	}
	return str[:idx], str[idx+1:], true
}
