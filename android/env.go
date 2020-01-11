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
	"os"
	"os/exec"
	"strings"

	"android/soong/env"
)

// This file supports dependencies on environment variables.  During build manifest generation,
// any dependency on an environment variable is added to a list.  During the singleton phase
// a JSON file is written containing the current value of all used environment variables.
// The next time the top-level build script is run, it uses the soong_env executable to
// compare the contents of the environment variables, rewriting the file if necessary to cause
// a manifest regeneration.

var originalEnv map[string]string
var SoongDelveListen string
var SoongDelvePath string

func init() {
	// Delve support needs to read this environment variable very early, before NewConfig has created a way to
	// access originalEnv with dependencies.  Store the value where soong_build can find it, it will manually
	// ensure the dependencies are created.
	SoongDelveListen = os.Getenv("SOONG_DELVE")
	SoongDelvePath, _ = exec.LookPath("dlv")

	originalEnv = make(map[string]string)
	for _, env := range os.Environ() {
		idx := strings.IndexRune(env, '=')
		if idx != -1 {
			originalEnv[env[:idx]] = env[idx+1:]
		}
	}
	// Clear the environment to prevent use of os.Getenv(), which would not provide dependencies on environment
	// variable values.  The environment is available through ctx.Config().Getenv, ctx.Config().IsEnvTrue, etc.
	os.Clearenv()
}

// getenv checks either os.Getenv or originalEnv so that it works before or after the init()
// function above.  It doesn't add any dependencies on the environment variable, so it should
// only be used for values that won't change.  For values that might change use ctx.Config().Getenv.
func getenv(key string) string {
	if originalEnv == nil {
		return os.Getenv(key)
	} else {
		return originalEnv[key]
	}
}

func EnvSingleton() Singleton {
	return &envSingleton{}
}

type envSingleton struct{}

func (c *envSingleton) GenerateBuildActions(ctx SingletonContext) {
	envDeps := ctx.Config().EnvDeps()

	envFile := PathForOutput(ctx, ".soong.environment")
	if ctx.Failed() {
		return
	}

	data, err := env.EnvFileContents(envDeps)
	if err != nil {
		ctx.Errorf(err.Error())
	}

	err = WriteFileToOutputDir(envFile, data, 0666)
	if err != nil {
		ctx.Errorf(err.Error())
	}

	ctx.AddNinjaFileDeps(envFile.String())
}
