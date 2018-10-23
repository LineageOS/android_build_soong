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
	"flag"
	"fmt"
	"os"

	"android/soong/symbol_inject"
)

var (
	input  = flag.String("i", "", "input file")
	output = flag.String("o", "", "output file")
	symbol = flag.String("s", "", "symbol to inject into")
	from   = flag.String("from", "", "optional existing value of the symbol for verification")
	value  = flag.String("v", "", "value to inject into symbol")

	dump = flag.Bool("dump", false, "dump the symbol table for copying into a test")
)

func main() {
	flag.Parse()

	usageError := func(s string) {
		fmt.Fprintln(os.Stderr, s)
		flag.Usage()
		os.Exit(1)
	}

	if *input == "" {
		usageError("-i is required")
	}

	if !*dump {
		if *output == "" {
			usageError("-o is required")
		}

		if *symbol == "" {
			usageError("-s is required")
		}

		if *value == "" {
			usageError("-v is required")
		}
	}

	r, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	defer r.Close()

	if *dump {
		err := symbol_inject.DumpSymbols(r)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(6)
		}
		return
	}

	w, err := os.OpenFile(*output, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}
	defer w.Close()

	file, err := symbol_inject.OpenFile(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(4)
	}

	err = symbol_inject.InjectStringSymbol(file, w, *symbol, *value, *from)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Remove(*output)
		os.Exit(5)
	}
}
