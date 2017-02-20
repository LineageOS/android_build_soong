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

// soong_javac_filter expects the output of javac on stdin, and produces
// an ANSI colorized version of the output on stdout.
//
// It also hides the unhelpful and unhideable "warning there is a warning"
// messages.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
)

// Regular expressions are based on
// https://chromium.googlesource.com/chromium/src/+/master/build/android/gyp/javac.py
// Colors are based on clang's output
var (
	filelinePrefix = `^[-.\w/\\]+.java:[0-9]+:`
	warningRe      = regexp.MustCompile(filelinePrefix + ` (warning:) .*$`)
	errorRe        = regexp.MustCompile(filelinePrefix + ` (.*?:) .*$`)
	markerRe       = regexp.MustCompile(`\s*(\^)\s*$`)

	escape  = "\x1b"
	reset   = escape + "[0m"
	bold    = escape + "[1m"
	red     = escape + "[31m"
	green   = escape + "[32m"
	magenta = escape + "[35m"
)

func main() {
	err := process(bufio.NewReader(os.Stdin), os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
		os.Exit(-1)
	}
}

func process(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	// Some javac wrappers output the entire list of java files being
	// compiled on a single line, which can be very large, set the maximum
	// buffer size to 2MB.
	scanner.Buffer(nil, 2*1024*1024)
	for scanner.Scan() {
		processLine(w, scanner.Text())
	}
	return scanner.Err()
}

func processLine(w io.Writer, line string) {
	for _, f := range filters {
		if f.MatchString(line) {
			return
		}
	}
	for _, p := range colorPatterns {
		var matched bool
		if line, matched = applyColor(line, p.color, p.re); matched {
			break
		}
	}
	fmt.Fprintln(w, line)
}

// If line matches re, make it bold and apply color to the first submatch
// Returns line, modified if it matched, and true if it matched.
func applyColor(line, color string, re *regexp.Regexp) (string, bool) {
	if m := re.FindStringSubmatchIndex(line); m != nil {
		tagStart, tagEnd := m[2], m[3]
		line = bold + line[:tagStart] +
			color + line[tagStart:tagEnd] + reset + bold +
			line[tagEnd:] + reset
		return line, true
	}
	return line, false
}

var colorPatterns = []struct {
	re    *regexp.Regexp
	color string
}{
	{warningRe, magenta},
	{errorRe, red},
	{markerRe, green},
}

var filters = []*regexp.Regexp{
	regexp.MustCompile(`Note: (Some input files|.*\.java) uses? or overrides? a deprecated API.`),
	regexp.MustCompile(`Note: Recompile with -Xlint:deprecation for details.`),
	regexp.MustCompile(`Note: (Some input files|.*\.java) uses? unchecked or unsafe operations.`),
	regexp.MustCompile(`Note: Recompile with -Xlint:unchecked for details.`),
}
