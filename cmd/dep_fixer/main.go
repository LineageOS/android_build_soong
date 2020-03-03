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

// This tool reads "make"-like dependency files, and outputs a canonical version
// that can be used by ninja. Ninja doesn't support multiple output files (even
// though it doesn't care what the output file is, or whether it matches what is
// expected).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"android/soong/makedeps"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-o <output>] <depfile.d> [<depfile.d>...]", os.Args[0])
		flag.PrintDefaults()
	}
	output := flag.String("o", "", "Optional output file (defaults to rewriting source if necessary)")
	flag.Parse()

	if flag.NArg() < 1 {
		log.Fatal("Expected at least one input file as an argument")
	}

	var mergedDeps *makedeps.Deps
	var firstInput []byte

	for i, arg := range flag.Args() {
		input, err := ioutil.ReadFile(arg)
		if err != nil {
			log.Fatalf("Error opening %q: %v", arg, err)
		}

		deps, err := makedeps.Parse(arg, bytes.NewBuffer(append([]byte(nil), input...)))
		if err != nil {
			log.Fatalf("Failed to parse: %v", err)
		}

		if i == 0 {
			mergedDeps = deps
			firstInput = input
		} else {
			mergedDeps.Inputs = append(mergedDeps.Inputs, deps.Inputs...)
		}
	}

	new := mergedDeps.Print()

	if *output == "" || *output == flag.Arg(0) {
		if !bytes.Equal(firstInput, new) {
			err := ioutil.WriteFile(flag.Arg(0), new, 0666)
			if err != nil {
				log.Fatalf("Failed to write: %v", err)
			}
		}
	} else {
		err := ioutil.WriteFile(*output, new, 0666)
		if err != nil {
			log.Fatalf("Failed to write to %q: %v", *output, err)
		}
	}
}
