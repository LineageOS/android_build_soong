// Copyright 2019 Google Inc. All rights reserved.
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

package main

import (
	"errors"
	"path/filepath"
	"strings"
)

// Match returns true if name matches pattern using the same rules as filepath.Match, but supporting
// recursive globs (**).
func Match(pattern, name string) (bool, error) {
	if filepath.Base(pattern) == "**" {
		return false, errors.New("pattern has '**' as last path element")
	}

	patternDir := pattern[len(pattern)-1] == '/'
	nameDir := name[len(name)-1] == '/'

	if patternDir != nameDir {
		return false, nil
	}

	if nameDir {
		name = name[:len(name)-1]
		pattern = pattern[:len(pattern)-1]
	}

	for {
		var patternFile, nameFile string
		pattern, patternFile = filepath.Dir(pattern), filepath.Base(pattern)

		if patternFile == "**" {
			if strings.Contains(pattern, "**") {
				return false, errors.New("pattern contains multiple '**'")
			}
			// Test if the any prefix of name matches the part of the pattern before **
			for {
				if name == "." || name == "/" {
					return name == pattern, nil
				}
				if match, err := filepath.Match(pattern, name); err != nil {
					return false, err
				} else if match {
					return true, nil
				}
				name = filepath.Dir(name)
			}
		} else if strings.Contains(patternFile, "**") {
			return false, errors.New("pattern contains other characters between '**' and path separator")
		}

		name, nameFile = filepath.Dir(name), filepath.Base(name)

		if nameFile == "." && patternFile == "." {
			return true, nil
		} else if nameFile == "/" && patternFile == "/" {
			return true, nil
		} else if nameFile == "." || patternFile == "." || nameFile == "/" || patternFile == "/" {
			return false, nil
		}

		match, err := filepath.Match(patternFile, nameFile)
		if err != nil || !match {
			return match, err
		}
	}
}
