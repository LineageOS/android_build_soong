// Copyright 2018 Google Inc. All rights reserved.
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
	"archive/zip"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	outputFile = flag.String("o", "", "output file")
	prefix     = flag.String("prefix", "", "prefix for each entry in the output file")
	inputFile  = flag.String("i", "", "input jar or srcjar")
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func fileToPackage(file string) string {
	dir := filepath.Dir(file)
	return strings.Replace(dir, "/", ".", -1)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: extract_jar_packages -i <input file> -o <output -file> [-prefix <prefix>]")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *outputFile == "" || *inputFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	pkgSet := make(map[string]bool)

	reader, err := zip.OpenReader(*inputFile)
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	for _, f := range reader.File {
		ext := filepath.Ext(f.Name)
		if ext == ".java" || ext == ".class" {
			pkgSet[fileToPackage(f.Name)] = true
		}
	}

	var pkgs []string
	for k := range pkgSet {
		pkgs = append(pkgs, k)
	}
	sort.Strings(pkgs)

	var data []byte
	for _, pkg := range pkgs {
		data = append(data, *prefix...)
		data = append(data, pkg...)
		data = append(data, "\n"...)
	}

	must(ioutil.WriteFile(*outputFile, data, 0666))
}
