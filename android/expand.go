// Copyright 2016 Google Inc. All rights reserved.
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

package android

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/google/blueprint/proptools"
)

// ExpandNinjaEscaped substitutes $() variables in a string
// $(var) is passed to mapping(var), which should return the expanded value, a bool for whether the result should
// be left unescaped when using in a ninja value (generally false, true if the expanded value is a ninja variable like
// '${in}'), and an error.
// $$ is converted to $, which is escaped back to $$.
func ExpandNinjaEscaped(s string, mapping func(string) (string, bool, error)) (string, error) {
	return expand(s, true, mapping)
}

// Expand substitutes $() variables in a string
// $(var) is passed to mapping(var), which should return the expanded value and an error.
// $$ is converted to $.
func Expand(s string, mapping func(string) (string, error)) (string, error) {
	return expand(s, false, func(s string) (string, bool, error) {
		s, err := mapping(s)
		return s, false, err
	})
}

func expand(s string, ninjaEscape bool, mapping func(string) (string, bool, error)) (string, error) {
	// based on os.Expand
	buf := make([]byte, 0, 2*len(s))
	i := 0
	for j := 0; j < len(s); j++ {
		if s[j] == '$' {
			if j+1 >= len(s) {
				return "", fmt.Errorf("expected character after '$'")
			}
			buf = append(buf, s[i:j]...)
			value, ninjaVariable, w, err := getMapping(s[j+1:], mapping)
			if err != nil {
				return "", err
			}
			if !ninjaVariable && ninjaEscape {
				value = proptools.NinjaEscape(value)
			}
			buf = append(buf, value...)
			j += w
			i = j + 1
		}
	}
	return string(buf) + s[i:], nil
}

func getMapping(s string, mapping func(string) (string, bool, error)) (string, bool, int, error) {
	switch s[0] {
	case '(':
		// Scan to closing brace
		for i := 1; i < len(s); i++ {
			if s[i] == ')' {
				ret, ninjaVariable, err := mapping(strings.TrimSpace(s[1:i]))
				return ret, ninjaVariable, i + 1, err
			}
		}
		return "", false, len(s), fmt.Errorf("missing )")
	case '$':
		return "$", false, 1, nil
	default:
		i := strings.IndexFunc(s, unicode.IsSpace)
		if i == 0 {
			return "", false, 0, fmt.Errorf("unexpected character '%c' after '$'", s[0])
		} else if i == -1 {
			i = len(s)
		}
		return "", false, 0, fmt.Errorf("expected '(' after '$', did you mean $(%s)?", s[:i])
	}
}
