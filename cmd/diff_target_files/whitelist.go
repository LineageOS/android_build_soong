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
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
)

type jsonWhitelist struct {
	Paths               []string
	IgnoreMatchingLines []string
}

type whitelist struct {
	path                string
	ignoreMatchingLines []string
}

func parseWhitelists(whitelists []string, whitelistFiles []string) ([]whitelist, error) {
	var ret []whitelist

	add := func(path string, ignoreMatchingLines []string) {
		for _, x := range ret {
			if x.path == path {
				x.ignoreMatchingLines = append(x.ignoreMatchingLines, ignoreMatchingLines...)
				return
			}
		}

		ret = append(ret, whitelist{
			path:                path,
			ignoreMatchingLines: ignoreMatchingLines,
		})
	}

	for _, file := range whitelistFiles {
		newWhitelists, err := parseWhitelistFile(file)
		if err != nil {
			return nil, err
		}

		for _, w := range newWhitelists {
			add(w.path, w.ignoreMatchingLines)
		}
	}

	for _, s := range whitelists {
		colon := strings.IndexRune(s, ':')
		var ignoreMatchingLines []string
		if colon >= 0 {
			ignoreMatchingLines = []string{s[colon+1:]}
		}
		add(s, ignoreMatchingLines)
	}

	return ret, nil
}

func parseWhitelistFile(file string) ([]whitelist, error) {
	r, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	d := json.NewDecoder(newJSONCommentStripper(r))

	var jsonWhitelists []jsonWhitelist

	err = d.Decode(&jsonWhitelists)

	var whitelists []whitelist
	for _, w := range jsonWhitelists {
		for _, p := range w.Paths {
			whitelists = append(whitelists, whitelist{
				path:                p,
				ignoreMatchingLines: w.IgnoreMatchingLines,
			})
		}
	}

	return whitelists, err
}

func filterModifiedPaths(l [][2]*ZipArtifactFile, whitelists []whitelist) ([][2]*ZipArtifactFile, error) {
outer:
	for i := 0; i < len(l); i++ {
		for _, w := range whitelists {
			if match, err := Match(w.path, l[i][0].Name); err != nil {
				return l, err
			} else if match {
				if match, err := diffIgnoringMatchingLines(l[i][0], l[i][1], w.ignoreMatchingLines); err != nil {
					return l, err
				} else if match || len(w.ignoreMatchingLines) == 0 {
					l = append(l[:i], l[i+1:]...)
					i--
				}
				continue outer
			}
		}
	}

	if len(l) == 0 {
		l = nil
	}

	return l, nil
}

func filterNewPaths(l []*ZipArtifactFile, whitelists []whitelist) ([]*ZipArtifactFile, error) {
outer:
	for i := 0; i < len(l); i++ {
		for _, w := range whitelists {
			if match, err := Match(w.path, l[i].Name); err != nil {
				return l, err
			} else if match && len(w.ignoreMatchingLines) == 0 {
				l = append(l[:i], l[i+1:]...)
				i--
			}
			continue outer
		}
	}

	if len(l) == 0 {
		l = nil
	}

	return l, nil
}

func diffIgnoringMatchingLines(a *ZipArtifactFile, b *ZipArtifactFile, ignoreMatchingLines []string) (match bool, err error) {
	lineMatchesIgnores := func(b []byte) (bool, error) {
		for _, m := range ignoreMatchingLines {
			if match, err := regexp.Match(m, b); err != nil {
				return false, err
			} else if match {
				return match, nil
			}
		}
		return false, nil
	}

	filter := func(z *ZipArtifactFile) ([]byte, error) {
		var ret []byte

		r, err := z.Open()
		if err != nil {
			return nil, err
		}
		s := bufio.NewScanner(r)

		for s.Scan() {
			if match, err := lineMatchesIgnores(s.Bytes()); err != nil {
				return nil, err
			} else if !match {
				ret = append(ret, "\n"...)
				ret = append(ret, s.Bytes()...)
			}
		}

		return ret, nil
	}

	bufA, err := filter(a)
	if err != nil {
		return false, err
	}
	bufB, err := filter(b)
	if err != nil {
		return false, err
	}

	return bytes.Compare(bufA, bufB) == 0, nil
}

func applyWhitelists(diff zipDiff, whitelists []whitelist) (zipDiff, error) {
	var err error

	diff.modified, err = filterModifiedPaths(diff.modified, whitelists)
	if err != nil {
		return diff, err
	}
	diff.onlyInA, err = filterNewPaths(diff.onlyInA, whitelists)
	if err != nil {
		return diff, err
	}
	diff.onlyInB, err = filterNewPaths(diff.onlyInB, whitelists)
	if err != nil {
		return diff, err
	}

	return diff, nil
}

func newJSONCommentStripper(r io.Reader) *jsonCommentStripper {
	return &jsonCommentStripper{
		r: bufio.NewReader(r),
	}
}

type jsonCommentStripper struct {
	r   *bufio.Reader
	b   []byte
	err error
}

func (j *jsonCommentStripper) Read(buf []byte) (int, error) {
	for len(j.b) == 0 {
		if j.err != nil {
			return 0, j.err
		}

		j.b, j.err = j.r.ReadBytes('\n')

		if isComment(j.b) {
			j.b = nil
		}
	}

	n := copy(buf, j.b)
	j.b = j.b[n:]
	return n, nil
}

var commentPrefix = []byte("//")

func isComment(b []byte) bool {
	for len(b) > 0 && unicode.IsSpace(rune(b[0])) {
		b = b[1:]
	}
	return bytes.HasPrefix(b, commentPrefix)
}
