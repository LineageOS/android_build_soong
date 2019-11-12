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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"android/soong/androidmk/androidmk"

	_ "android/soong/partner/bpfix/extensions"
)

var usage = func() {
	fmt.Fprintf(os.Stderr, "usage: %s [flags] <inputFile>\n"+
		"\n%s parses <inputFile> as an Android.mk file and attempts to output an analogous Android.bp file (to standard out)\n", os.Args[0], os.Args[0])
	flag.PrintDefaults()
	os.Exit(1)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if len(flag.Args()) != 1 {
		usage()
	}
	filePathToRead := flag.Arg(0)
	b, err := ioutil.ReadFile(filePathToRead)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	output, errs := androidmk.ConvertFile(os.Args[1], bytes.NewBuffer(b))
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, "ERROR: ", err)
		}
		os.Exit(1)
	}

	fmt.Print(output)
}
