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

package android

import (
	"android/soong/shared"
)

// This file supports dependencies on environment variables.  During build
// manifest generation, any dependency on an environment variable is added to a
// list.  At the end of the build, a JSON file called soong.environment.used is
// written containing the current value of all used environment variables. The
// next time the top-level build script is run, soong_ui parses the compare the
// contents of the used environment variables, then, if they changed, deletes
// soong.environment.used to cause a rebuild.
//
// The dependency of build.ninja on soong.environment.used is declared in
// build.ninja.d

var originalEnv map[string]string

func InitEnvironment(envFile string) {
	var err error
	originalEnv, err = shared.EnvFromFile(envFile)
	if err != nil {
		panic(err)
	}
}
