// Copyright 2015 Google Inc. All rights reserved.
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

// soong_env determines if the given soong environment file (usually ".soong.environment") is stale
// by comparing its contents to the current corresponding environment variable values.
// It fails if the file cannot be opened or corrupted, or its contents differ from the current
// values.

package main

import (
	"flag"
	"fmt"
	"os"

	"android/soong/env"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: soong_env env_file\n")
	fmt.Fprintf(os.Stderr, "exits with success if the environment varibles in env_file match\n")
	fmt.Fprintf(os.Stderr, "the current environment\n")
	flag.PrintDefaults()
	os.Exit(2)
}

// This is a simple executable packaging, and the real work happens in env.StaleEnvFile.
func main() {
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
	}

	stale, err := env.StaleEnvFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(1)
	}

	if stale {
		os.Exit(1)
	}

	os.Exit(0)
}
