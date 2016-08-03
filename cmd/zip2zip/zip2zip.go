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

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"android/soong/third_party/zip"
)

var (
	input  = flag.String("i", "", "zip file to read from")
	output = flag.String("o", "", "output file")
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: zip2zip -i zipfile -o zipfile [filespec]...")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "  filespec:")
	fmt.Fprintln(os.Stderr, "    <name>")
	fmt.Fprintln(os.Stderr, "    <in_name>:<out_name>")
	fmt.Fprintln(os.Stderr, "    <glob>:<out_dir>/")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Files will be copied with their existing compression from the input zipfile to")
	fmt.Fprintln(os.Stderr, "the output zipfile, in the order of filespec arguments")
	os.Exit(2)
}

func main() {
	flag.Parse()

	if flag.NArg() == 0 || *input == "" || *output == "" {
		usage()
	}

	reader, err := zip.OpenReader(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	defer reader.Close()

	output, err := os.Create(*output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
	defer output.Close()

	writer := zip.NewWriter(output)
	defer func() {
		err := writer.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(5)
		}
	}()

	for _, arg := range flag.Args() {
		var input string
		var output string

		// Reserve escaping for future implementation, so make sure no
		// one is using \ and expecting a certain behavior.
		if strings.Contains(arg, "\\") {
			fmt.Fprintln(os.Stderr, "\\ characters are not currently supported")
			os.Exit(6)
		}

		args := strings.SplitN(arg, ":", 2)
		input = args[0]
		if len(args) == 2 {
			output = args[1]
		}

		if strings.IndexAny(input, "*?[") >= 0 {
			for _, file := range reader.File {
				if match, err := filepath.Match(input, file.Name); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(7)
				} else if match {
					var newFileName string
					if output == "" {
						newFileName = file.Name
					} else {
						_, name := filepath.Split(file.Name)
						newFileName = filepath.Join(output, name)
					}
					err = writer.CopyFrom(file, newFileName)
					if err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(8)
					}
				}
			}
		} else {
			if output == "" {
				output = input
			}
			for _, file := range reader.File {
				if input == file.Name {
					err = writer.CopyFrom(file, output)
					if err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(8)
					}
					break
				}
			}
		}
	}
}
