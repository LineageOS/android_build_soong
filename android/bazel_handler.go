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
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/google/blueprint/bootstrap"
)

type CqueryRequestType int

const (
	getAllFiles CqueryRequestType = iota
)

// Map key to describe bazel cquery requests.
type cqueryKey struct {
	label       string
	requestType CqueryRequestType
}

type BazelContext interface {
	// The below methods involve queuing cquery requests to be later invoked
	// by bazel. If any of these methods return (_, false), then the request
	// has been queued to be run later.

	// Returns result files built by building the given bazel target label.
	GetAllFiles(label string) ([]string, bool)

	// TODO(cparsons): Other cquery-related methods should be added here.
	// ** End cquery methods

	// Issues commands to Bazel to receive results for all cquery requests
	// queued in the BazelContext.
	InvokeBazel() error

	// Returns true if bazel is enabled for the given configuration.
	BazelEnabled() bool
}

// A context object which tracks queued requests that need to be made to Bazel,
// and their results after the requests have been made.
type bazelContext struct {
	homeDir      string
	bazelPath    string
	outputBase   string
	workspaceDir string
	buildDir     string

	requests     map[cqueryKey]bool // cquery requests that have not yet been issued to Bazel
	requestMutex sync.Mutex         // requests can be written in parallel

	results map[cqueryKey]string // Results of cquery requests after Bazel invocations
}

var _ BazelContext = &bazelContext{}

// A bazel context to use when Bazel is disabled.
type noopBazelContext struct{}

var _ BazelContext = noopBazelContext{}

// A bazel context to use for tests.
type MockBazelContext struct {
	AllFiles map[string][]string
}

func (m MockBazelContext) GetAllFiles(label string) ([]string, bool) {
	result, ok := m.AllFiles[label]
	return result, ok
}

func (m MockBazelContext) InvokeBazel() error {
	panic("unimplemented")
}

func (m MockBazelContext) BazelEnabled() bool {
	return true
}

var _ BazelContext = MockBazelContext{}

func (bazelCtx *bazelContext) GetAllFiles(label string) ([]string, bool) {
	result, ok := bazelCtx.cquery(label, getAllFiles)
	if ok {
		bazelOutput := strings.TrimSpace(result)
		return strings.Split(bazelOutput, ", "), true
	} else {
		return nil, false
	}
}

func (n noopBazelContext) GetAllFiles(label string) ([]string, bool) {
	panic("unimplemented")
}

func (n noopBazelContext) InvokeBazel() error {
	panic("unimplemented")
}

func (n noopBazelContext) BazelEnabled() bool {
	return false
}

func NewBazelContext(c *config) (BazelContext, error) {
	// TODO(cparsons): Assess USE_BAZEL=1 instead once "mixed Soong/Bazel builds"
	// are production ready.
	if c.Getenv("USE_BAZEL_ANALYSIS") != "1" {
		return noopBazelContext{}, nil
	}

	bazelCtx := bazelContext{buildDir: c.buildDir, requests: make(map[cqueryKey]bool)}
	missingEnvVars := []string{}
	if len(c.Getenv("BAZEL_HOME")) > 1 {
		bazelCtx.homeDir = c.Getenv("BAZEL_HOME")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_HOME")
	}
	if len(c.Getenv("BAZEL_PATH")) > 1 {
		bazelCtx.bazelPath = c.Getenv("BAZEL_PATH")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_PATH")
	}
	if len(c.Getenv("BAZEL_OUTPUT_BASE")) > 1 {
		bazelCtx.outputBase = c.Getenv("BAZEL_OUTPUT_BASE")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_OUTPUT_BASE")
	}
	if len(c.Getenv("BAZEL_WORKSPACE")) > 1 {
		bazelCtx.workspaceDir = c.Getenv("BAZEL_WORKSPACE")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_WORKSPACE")
	}
	if len(missingEnvVars) > 0 {
		return nil, errors.New(fmt.Sprintf("missing required env vars to use bazel: %s", missingEnvVars))
	} else {
		return &bazelCtx, nil
	}
}

func (context *bazelContext) BazelEnabled() bool {
	return true
}

// Adds a cquery request to the Bazel request queue, to be later invoked, or
// returns the result of the given request if the request was already made.
// If the given request was already made (and the results are available), then
// returns (result, true). If the request is queued but no results are available,
// then returns ("", false).
func (context *bazelContext) cquery(label string, requestType CqueryRequestType) (string, bool) {
	key := cqueryKey{label, requestType}
	if result, ok := context.results[key]; ok {
		return result, true
	} else {
		context.requestMutex.Lock()
		defer context.requestMutex.Unlock()
		context.requests[key] = true
		return "", false
	}
}

func pwdPrefix() string {
	// Darwin doesn't have /proc
	if runtime.GOOS != "darwin" {
		return "PWD=/proc/self/cwd"
	}
	return ""
}

func (context *bazelContext) issueBazelCommand(command string, labels []string,
	extraFlags ...string) (string, error) {

	cmdFlags := []string{"--bazelrc=build/bazel/common.bazelrc",
		"--output_base=" + context.outputBase, command}
	cmdFlags = append(cmdFlags, labels...)
	cmdFlags = append(cmdFlags, extraFlags...)

	bazelCmd := exec.Command(context.bazelPath, cmdFlags...)
	bazelCmd.Dir = context.workspaceDir
	bazelCmd.Env = append(os.Environ(), "HOME="+context.homeDir, pwdPrefix())

	stderr := &bytes.Buffer{}
	bazelCmd.Stderr = stderr

	if output, err := bazelCmd.Output(); err != nil {
		return "", fmt.Errorf("bazel command failed. command: [%s], error [%s]", bazelCmd, stderr)
	} else {
		return string(output), nil
	}
}

func (context *bazelContext) mainBzlFileContents() []byte {
	contents := `
# This file is generated by soong_build. Do not edit.
def _mixed_build_root_impl(ctx):
    return [DefaultInfo(files = depset(ctx.files.deps))]

mixed_build_root = rule(
    implementation = _mixed_build_root_impl,
    attrs = {"deps" : attr.label_list()},
)
`
	return []byte(contents)
}

func (context *bazelContext) mainBuildFileContents() []byte {
	formatString := `
# This file is generated by soong_build. Do not edit.
load(":main.bzl", "mixed_build_root")

mixed_build_root(name = "buildroot",
    deps = [%s],
)
`
	var buildRootDeps []string = nil
	for val, _ := range context.requests {
		buildRootDeps = append(buildRootDeps, fmt.Sprintf("\"%s\"", val.label))
	}
	buildRootDepsString := strings.Join(buildRootDeps, ",\n            ")

	return []byte(fmt.Sprintf(formatString, buildRootDepsString))
}

func (context *bazelContext) cqueryStarlarkFileContents() []byte {
	formatString := `
# This file is generated by soong_build. Do not edit.
getAllFilesLabels = {
  %s
}

def format(target):
  if str(target.label) in getAllFilesLabels:
    return str(target.label) + ">>" + ', '.join([f.path for f in target.files.to_list()])
  else:
    # This target was not requested via cquery, and thus must be a dependency
    # of a requested target.
    return ""
`
	var buildRootDeps []string = nil
	// TODO(cparsons): Sort by request type instead of assuming all requests
	// are of GetAllFiles type.
	for val, _ := range context.requests {
		buildRootDeps = append(buildRootDeps, fmt.Sprintf("\"%s\" : True", val.label))
	}
	buildRootDepsString := strings.Join(buildRootDeps, ",\n  ")

	return []byte(fmt.Sprintf(formatString, buildRootDepsString))
}

// Issues commands to Bazel to receive results for all cquery requests
// queued in the BazelContext.
func (context *bazelContext) InvokeBazel() error {
	context.results = make(map[cqueryKey]string)

	var cqueryOutput string
	var err error
	err = ioutil.WriteFile(
		absolutePath(filepath.Join(context.buildDir, "main.bzl")),
		context.mainBzlFileContents(), 0666)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(
		absolutePath(filepath.Join(context.buildDir, "BUILD.bazel")),
		context.mainBuildFileContents(), 0666)
	if err != nil {
		return err
	}
	cquery_file_relpath := filepath.Join(context.buildDir, "buildroot.cquery")
	err = ioutil.WriteFile(
		absolutePath(cquery_file_relpath),
		context.cqueryStarlarkFileContents(), 0666)
	if err != nil {
		return err
	}
	buildroot_label := fmt.Sprintf("//%s:buildroot", context.buildDir)
	cqueryOutput, err = context.issueBazelCommand("cquery",
		[]string{fmt.Sprintf("deps(%s)", buildroot_label)},
		"--output=starlark",
		"--starlark:file="+cquery_file_relpath)

	if err != nil {
		return err
	}

	cqueryResults := map[string]string{}
	for _, outputLine := range strings.Split(cqueryOutput, "\n") {
		if strings.Contains(outputLine, ">>") {
			splitLine := strings.SplitN(outputLine, ">>", 2)
			cqueryResults[splitLine[0]] = splitLine[1]
		}
	}

	for val, _ := range context.requests {
		if cqueryResult, ok := cqueryResults[val.label]; ok {
			context.results[val] = string(cqueryResult)
		} else {
			return fmt.Errorf("missing result for bazel target %s", val.label)
		}
	}

	// Issue a build command.
	// TODO(cparsons): Invoking bazel execution during soong_build should be avoided;
	// bazel actions should either be added to the Ninja file and executed later,
	// or bazel should handle execution.
	// TODO(cparsons): Use --target_pattern_file to avoid command line limits.
	_, err = context.issueBazelCommand("build", []string{buildroot_label})

	if err != nil {
		return err
	}

	// Clear requests.
	context.requests = map[cqueryKey]bool{}
	return nil
}

// Singleton used for registering BUILD file ninja dependencies (needed
// for correctness of builds which use Bazel.
func BazelSingleton() Singleton {
	return &bazelSingleton{}
}

type bazelSingleton struct{}

func (c *bazelSingleton) GenerateBuildActions(ctx SingletonContext) {
	if ctx.Config().BazelContext.BazelEnabled() {
		bazelBuildList := absolutePath(filepath.Join(
			filepath.Dir(bootstrap.ModuleListFile), "bazel.list"))
		ctx.AddNinjaFileDeps(bazelBuildList)

		data, err := ioutil.ReadFile(bazelBuildList)
		if err != nil {
			ctx.Errorf(err.Error())
		}
		files := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, file := range files {
			ctx.AddNinjaFileDeps(file)
		}
	}
}
