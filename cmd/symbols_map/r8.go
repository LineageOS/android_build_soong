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

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

const hashPrefix = "# pg_map_hash: "
const hashTypePrefix = "SHA-256 "
const commentPrefix = "#"

// r8Identifier extracts the hash from the comments of a dictionary produced by R8. It returns
// an empty identifier if no matching comment was found before the first non-comment line.
func r8Identifier(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %w", filename, err)
	}
	defer f.Close()

	return extractR8CompilerHash(f)
}

func extractR8CompilerHash(r io.Reader) (string, error) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, hashPrefix) {
			hash := strings.TrimPrefix(line, hashPrefix)
			if !strings.HasPrefix(hash, hashTypePrefix) {
				return "", fmt.Errorf("invalid hash type found in %q", line)
			}
			return strings.TrimPrefix(hash, hashTypePrefix), nil
		} else if !strings.HasPrefix(line, commentPrefix) {
			break
		}
	}
	return "", nil
}
