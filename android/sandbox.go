// Copyright 2020 Google Inc. All rights reserved.
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

package android

import (
	"fmt"
	"os"
)

func init() {
	// Stash the working directory in a private variable and then change the working directory
	// to "/", which will prevent untracked accesses to files by Go Soong plugins. The
	// SOONG_SANDBOX_SOONG_BUILD environment variable is set by soong_ui, and is not
	// overrideable on the command line.

	orig, err := os.Getwd()
	if err != nil {
		panic(fmt.Errorf("failed to get working directory: %s", err))
	}
	absSrcDir = orig

	if getenv("SOONG_SANDBOX_SOONG_BUILD") == "true" {
		err = os.Chdir("/")
		if err != nil {
			panic(fmt.Errorf("failed to change working directory to '/': %s", err))
		}
	}
}

// DO NOT USE THIS FUNCTION IN NEW CODE.
// Deprecated: This function will be removed as soon as the existing use cases that use it have been
// replaced.
func AbsSrcDirForExistingUseCases() string {
	return absSrcDir
}
