// Copyright 2021 Google Inc. All rights reserved.
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

package response

import (
	"io"
	"io/ioutil"
	"strings"
	"unicode"
)

const noQuote = '\x00'

// ReadRspFile reads a file in Ninja's response file format and returns its contents.
func ReadRspFile(r io.Reader) ([]string, error) {
	var files []string
	var file []byte

	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	isEscaping := false
	quotingStart := byte(noQuote)
	for _, c := range buf {
		switch {
		case isEscaping:
			if quotingStart == '"' {
				if !(c == '"' || c == '\\') {
					// '\"' or '\\' will be escaped under double quoting.
					file = append(file, '\\')
				}
			}
			file = append(file, c)
			isEscaping = false
		case c == '\\' && quotingStart != '\'':
			isEscaping = true
		case quotingStart == noQuote && (c == '\'' || c == '"'):
			quotingStart = c
		case quotingStart != noQuote && c == quotingStart:
			quotingStart = noQuote
		case quotingStart == noQuote && unicode.IsSpace(rune(c)):
			// Current character is a space outside quotes
			if len(file) != 0 {
				files = append(files, string(file))
			}
			file = file[:0]
		default:
			file = append(file, c)
		}
	}

	if len(file) != 0 {
		files = append(files, string(file))
	}

	return files, nil
}

func rspUnsafeChar(r rune) bool {
	switch {
	case 'A' <= r && r <= 'Z',
		'a' <= r && r <= 'z',
		'0' <= r && r <= '9',
		r == '_',
		r == '+',
		r == '-',
		r == '.',
		r == '/':
		return false
	default:
		return true
	}
}

var rspEscaper = strings.NewReplacer(`'`, `'\''`)

// WriteRspFile writes a list of files to a file in Ninja's response file format.
func WriteRspFile(w io.Writer, files []string) error {
	for i, f := range files {
		if i != 0 {
			_, err := io.WriteString(w, " ")
			if err != nil {
				return err
			}
		}

		if strings.IndexFunc(f, rspUnsafeChar) != -1 {
			f = `'` + rspEscaper.Replace(f) + `'`
		}

		_, err := io.WriteString(w, f)
		if err != nil {
			return err
		}
	}

	return nil
}
