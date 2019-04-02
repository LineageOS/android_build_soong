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
	"flag"
	"fmt"
	"os"
	"strings"
)

var (
	whitelists     = newMultiString("whitelist", "whitelist patterns in the form <pattern>[:<regex of line to ignore>]")
	whitelistFiles = newMultiString("whitelist_file", "files containing whitelist definitions")

	filters = newMultiString("filter", "filter patterns to apply to files in target-files.zip before comparing")
)

func newMultiString(name, usage string) *multiString {
	var f multiString
	flag.Var(&f, name, usage)
	return &f
}

type multiString []string

func (ms *multiString) String() string     { return strings.Join(*ms, ", ") }
func (ms *multiString) Set(s string) error { *ms = append(*ms, s); return nil }

func main() {
	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "Error, exactly two arguments are required\n")
		os.Exit(1)
	}

	whitelists, err := parseWhitelists(*whitelists, *whitelistFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing whitelists: %v\n", err)
		os.Exit(1)
	}

	priZip, err := NewLocalZipArtifact(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening zip file %v: %v\n", flag.Arg(0), err)
		os.Exit(1)
	}
	defer priZip.Close()

	refZip, err := NewLocalZipArtifact(flag.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening zip file %v: %v\n", flag.Arg(1), err)
		os.Exit(1)
	}
	defer refZip.Close()

	diff, err := compareTargetFiles(priZip, refZip, targetFilesPattern, whitelists, *filters)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error comparing zip files: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(diff.String())

	if len(diff.modified) > 0 || len(diff.onlyInA) > 0 || len(diff.onlyInB) > 0 {
		fmt.Fprintln(os.Stderr, "differences found")
		os.Exit(1)
	}
}
