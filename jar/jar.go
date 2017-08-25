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

package jar

import (
	"fmt"
	"strings"
)

// EntryNamesLess tells whether <filepathA> should precede <filepathB> in
// the order of files with a .jar
func EntryNamesLess(filepathA string, filepathB string) (less bool) {
	diff := index(filepathA) - index(filepathB)
	if diff == 0 {
		return filepathA < filepathB
	}
	return diff < 0
}

// Treats trailing * as a prefix match
func patternMatch(pattern, name string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	} else {
		return name == pattern
	}
}

var jarOrder = []string{
	"META-INF/",
	"META-INF/MANIFEST.MF",
	"META-INF/*",
	"*",
}

func index(name string) int {
	for i, pattern := range jarOrder {
		if patternMatch(pattern, name) {
			return i
		}
	}
	panic(fmt.Errorf("file %q did not match any pattern", name))
}
