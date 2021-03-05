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
	"fmt"
	"os"
	"strings"
	"syscall"

	"android/soong/shared"
)

// This file supports dependencies on environment variables.  During build manifest generation,
// any dependency on an environment variable is added to a list.  During the singleton phase
// a JSON file is written containing the current value of all used environment variables.
// The next time the top-level build script is run, it uses the soong_env executable to
// compare the contents of the environment variables, rewriting the file if necessary to cause
// a manifest regeneration.

var originalEnv map[string]string
var soongDelveListen string
var soongDelvePath string
var isDebugging bool

func InitEnvironment(envFile string) {
	var err error
	originalEnv, err = shared.EnvFromFile(envFile)
	if err != nil {
		panic(err)
	}

	soongDelveListen = originalEnv["SOONG_DELVE"]
	soongDelvePath = originalEnv["SOONG_DELVE_PATH"]
}

// Returns whether the current process is running under Delve due to
// ReexecWithDelveMaybe().
func IsDebugging() bool {
	return isDebugging
}
func ReexecWithDelveMaybe() {
	isDebugging = os.Getenv("SOONG_DELVE_REEXECUTED") == "true"
	if isDebugging || soongDelveListen == "" {
		return
	}

	if soongDelvePath == "" {
		fmt.Fprintln(os.Stderr, "SOONG_DELVE is set but failed to find dlv")
		os.Exit(1)
	}

	soongDelveEnv := []string{}
	for _, env := range os.Environ() {
		idx := strings.IndexRune(env, '=')
		if idx != -1 {
			soongDelveEnv = append(soongDelveEnv, env)
		}
	}

	soongDelveEnv = append(soongDelveEnv, "SOONG_DELVE_REEXECUTED=true")

	dlvArgv := []string{
		soongDelvePath,
		"--listen=:" + soongDelveListen,
		"--headless=true",
		"--api-version=2",
		"exec",
		os.Args[0],
		"--",
	}
	dlvArgv = append(dlvArgv, os.Args[1:]...)
	os.Chdir(absSrcDir)
	syscall.Exec(soongDelvePath, dlvArgv, soongDelveEnv)
	fmt.Fprintln(os.Stderr, "exec() failed while trying to reexec with Delve")
	os.Exit(1)
}

func EnvSingleton() Singleton {
	return &envSingleton{}
}

type envSingleton struct{}

func (c *envSingleton) GenerateBuildActions(ctx SingletonContext) {
	envDeps := ctx.Config().EnvDeps()

	envFile := PathForOutput(ctx, "soong.environment.used")
	if ctx.Failed() {
		return
	}

	data, err := shared.EnvFileContents(envDeps)
	if err != nil {
		ctx.Errorf(err.Error())
	}

	err = WriteFileToOutputDir(envFile, data, 0666)
	if err != nil {
		ctx.Errorf(err.Error())
	}

	ctx.AddNinjaFileDeps(envFile.String())
}
