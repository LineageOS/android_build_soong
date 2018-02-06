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
	"bytes"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
)

var (
	input  = flag.String("i", "", "input file")
	output = flag.String("o", "", "output file")
	symbol = flag.String("s", "", "symbol to inject into")
	from   = flag.String("from", "", "optional existing value of the symbol for verification")
	value  = flag.String("v", "", "value to inject into symbol")
)

var maxUint64 uint64 = math.MaxUint64

type cantParseError struct {
	error
}

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

	if *output == "" {
		usageError("-o is required")
	}

	if *symbol == "" {
		usageError("-s is required")
	}

	if *value == "" {
		usageError("-v is required")
	}

	r, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	defer r.Close()

	w, err := os.OpenFile(*output, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}
	defer w.Close()

	err = injectSymbol(r, w, *symbol, *value, *from)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Remove(*output)
		os.Exit(2)
	}
}

type ReadSeekerAt interface {
	io.ReaderAt
	io.ReadSeeker
}

func injectSymbol(r ReadSeekerAt, w io.Writer, symbol, value, from string) error {
	var offset, size uint64
	var err error

	offset, size, err = findElfSymbol(r, symbol)
	if elfError, ok := err.(cantParseError); ok {
		// Try as a mach-o file
		offset, size, err = findMachoSymbol(r, symbol)
		if _, ok := err.(cantParseError); ok {
			// Try as a windows PE file
			offset, size, err = findPESymbol(r, symbol)
			if _, ok := err.(cantParseError); ok {
				// Can't parse as elf, macho, or PE, return the elf error
				return elfError
			}
		}
	}
	if err != nil {
		return err
	}

	if uint64(len(value))+1 > size {
		return fmt.Errorf("value length %d overflows symbol size %d", len(value), size)
	}

	if from != "" {
		// Read the exsting symbol contents and verify they match the expected value
		expected := make([]byte, size)
		existing := make([]byte, size)
		copy(expected, from)
		_, err := r.ReadAt(existing, int64(offset))
		if err != nil {
			return err
		}
		if bytes.Compare(existing, expected) != 0 {
			return fmt.Errorf("existing symbol contents %q did not match expected value %q",
				string(existing), string(expected))
		}
	}

	return copyAndInject(r, w, offset, size, value)
}

func copyAndInject(r io.ReadSeeker, w io.Writer, offset, size uint64, value string) (err error) {
	// helper that asserts a two-value function returning an int64 and an error has err != nil
	must := func(n int64, err error) {
		if err != nil {
			panic(err)
		}
	}

	// helper that asserts a two-value function returning an int and an error has err != nil
	must2 := func(n int, err error) {
		must(int64(n), err)
	}

	// convert a panic into returning an error
	defer func() {
		if r := recover(); r != nil {
			err, _ = r.(error)
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			if err == nil {
				panic(r)
			}
		}
	}()

	buf := make([]byte, size)
	copy(buf, value)

	// Reset the input file
	must(r.Seek(0, io.SeekStart))
	// Copy the first bytes up to the symbol offset
	must(io.CopyN(w, r, int64(offset)))
	// Skip the symbol contents in the input file
	must(r.Seek(int64(size), io.SeekCurrent))
	// Write the injected value in the output file
	must2(w.Write(buf))
	// Write the remainder of the file
	must(io.Copy(w, r))

	return nil
}
