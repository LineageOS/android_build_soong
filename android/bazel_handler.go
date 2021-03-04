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

	"android/soong/bazel"
	"android/soong/shared"
)

type CqueryRequestType int

const (
	getAllFiles CqueryRequestType = iota
	getCcObjectFiles
)

// Map key to describe bazel cquery requests.
type cqueryKey struct {
	label       string
	requestType CqueryRequestType
	archType    ArchType
}

type BazelContext interface {
	// The below methods involve queuing cquery requests to be later invoked
	// by bazel. If any of these methods return (_, false), then the request
	// has been queued to be run later.

	// Returns result files built by building the given bazel target label.
	GetAllFiles(label string, archType ArchType) ([]string, bool)

	// Returns object files produced by compiling the given cc-related target.
	// Retrieves these files from Bazel's CcInfo provider.
	GetCcObjectFiles(label string, archType ArchType) ([]string, bool)

	// TODO(cparsons): Other cquery-related methods should be added here.
	// ** End cquery methods

	// Issues commands to Bazel to receive results for all cquery requests
	// queued in the BazelContext.
	InvokeBazel() error

	// Returns true if bazel is enabled for the given configuration.
	BazelEnabled() bool

	// Returns the bazel output base (the root directory for all bazel intermediate outputs).
	OutputBase() string

	// Returns build statements which should get registered to reflect Bazel's outputs.
	BuildStatementsToRegister() []bazel.BuildStatement
}

// A context object which tracks queued requests that need to be made to Bazel,
// and their results after the requests have been made.
type bazelContext struct {
	homeDir      string
	bazelPath    string
	outputBase   string
	workspaceDir string
	buildDir     string
	metricsDir   string

	requests     map[cqueryKey]bool // cquery requests that have not yet been issued to Bazel
	requestMutex sync.Mutex         // requests can be written in parallel

	results map[cqueryKey]string // Results of cquery requests after Bazel invocations

	// Build statements which should get registered to reflect Bazel's outputs.
	buildStatements []bazel.BuildStatement
}

var _ BazelContext = &bazelContext{}

// A bazel context to use when Bazel is disabled.
type noopBazelContext struct{}

var _ BazelContext = noopBazelContext{}

// A bazel context to use for tests.
type MockBazelContext struct {
	AllFiles map[string][]string
}

func (m MockBazelContext) GetAllFiles(label string, archType ArchType) ([]string, bool) {
	result, ok := m.AllFiles[label]
	return result, ok
}

func (m MockBazelContext) GetCcObjectFiles(label string, archType ArchType) ([]string, bool) {
	result, ok := m.AllFiles[label]
	return result, ok
}

func (m MockBazelContext) InvokeBazel() error {
	panic("unimplemented")
}

func (m MockBazelContext) BazelEnabled() bool {
	return true
}

func (m MockBazelContext) OutputBase() string {
	return "outputbase"
}

func (m MockBazelContext) BuildStatementsToRegister() []bazel.BuildStatement {
	return []bazel.BuildStatement{}
}

var _ BazelContext = MockBazelContext{}

func (bazelCtx *bazelContext) GetAllFiles(label string, archType ArchType) ([]string, bool) {
	result, ok := bazelCtx.cquery(label, getAllFiles, archType)
	if ok {
		bazelOutput := strings.TrimSpace(result)
		return strings.Split(bazelOutput, ", "), true
	} else {
		return nil, false
	}
}

func (bazelCtx *bazelContext) GetCcObjectFiles(label string, archType ArchType) ([]string, bool) {
	result, ok := bazelCtx.cquery(label, getCcObjectFiles, archType)
	if ok {
		bazelOutput := strings.TrimSpace(result)
		return strings.Split(bazelOutput, ", "), true
	} else {
		return nil, false
	}
}

func (n noopBazelContext) GetAllFiles(label string, archType ArchType) ([]string, bool) {
	panic("unimplemented")
}

func (n noopBazelContext) GetCcObjectFiles(label string, archType ArchType) ([]string, bool) {
	panic("unimplemented")
}

func (n noopBazelContext) InvokeBazel() error {
	panic("unimplemented")
}

func (m noopBazelContext) OutputBase() string {
	return ""
}

func (n noopBazelContext) BazelEnabled() bool {
	return false
}

func (m noopBazelContext) BuildStatementsToRegister() []bazel.BuildStatement {
	return []bazel.BuildStatement{}
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
	if len(c.Getenv("BAZEL_METRICS_DIR")) > 1 {
		bazelCtx.metricsDir = c.Getenv("BAZEL_METRICS_DIR")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_METRICS_DIR")
	}
	if len(missingEnvVars) > 0 {
		return nil, errors.New(fmt.Sprintf("missing required env vars to use bazel: %s", missingEnvVars))
	} else {
		return &bazelCtx, nil
	}
}

func (context *bazelContext) BazelMetricsDir() string {
	return context.metricsDir
}

func (context *bazelContext) BazelEnabled() bool {
	return true
}

// Adds a cquery request to the Bazel request queue, to be later invoked, or
// returns the result of the given request if the request was already made.
// If the given request was already made (and the results are available), then
// returns (result, true). If the request is queued but no results are available,
// then returns ("", false).
func (context *bazelContext) cquery(label string, requestType CqueryRequestType,
	archType ArchType) (string, bool) {
	key := cqueryKey{label, requestType, archType}
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

func (context *bazelContext) issueBazelCommand(runName bazel.RunName, command string, labels []string,
	extraFlags ...string) (string, error) {

	cmdFlags := []string{"--output_base=" + context.outputBase, command}
	cmdFlags = append(cmdFlags, labels...)
	cmdFlags = append(cmdFlags, "--package_path=%workspace%/"+context.intermediatesDir())
	cmdFlags = append(cmdFlags, "--profile="+shared.BazelMetricsFilename(context, runName))
	// Set default platforms to canonicalized values for mixed builds requests. If these are set
	// in the bazelrc, they will have values that are non-canonicalized, and thus be invalid.
	// The actual platform values here may be overridden by configuration transitions from the buildroot.
	cmdFlags = append(cmdFlags,
		fmt.Sprintf("--platforms=%s", canonicalizeLabel("//build/bazel/platforms:generic_x86_64")))
	cmdFlags = append(cmdFlags,
		fmt.Sprintf("--extra_toolchains=%s", canonicalizeLabel("//prebuilts/clang/host/linux-x86:all")))
	// Explicitly disable downloading rules (such as canonical C++ and Java rules) from the network.
	cmdFlags = append(cmdFlags, "--experimental_repository_disable_download")
	cmdFlags = append(cmdFlags, extraFlags...)

	bazelCmd := exec.Command(context.bazelPath, cmdFlags...)
	bazelCmd.Dir = context.workspaceDir
	bazelCmd.Env = append(os.Environ(), "HOME="+context.homeDir, pwdPrefix(),
		// Disables local host detection of gcc; toolchain information is defined
		// explicitly in BUILD files.
		"BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=1")
	stderr := &bytes.Buffer{}
	bazelCmd.Stderr = stderr

	if output, err := bazelCmd.Output(); err != nil {
		return "", fmt.Errorf("bazel command failed. command: [%s], env: [%s], error [%s]", bazelCmd, bazelCmd.Env, stderr)
	} else {
		return string(output), nil
	}
}

// Returns the string contents of a workspace file that should be output
// adjacent to the main bzl file and build file.
// This workspace file allows, via local_repository rule, sourcetree-level
// BUILD targets to be referenced via @sourceroot.
func (context *bazelContext) workspaceFileContents() []byte {
	formatString := `
# This file is generated by soong_build. Do not edit.
local_repository(
    name = "sourceroot",
    path = "%s",
)
`
	return []byte(fmt.Sprintf(formatString, context.workspaceDir))
}

func (context *bazelContext) mainBzlFileContents() []byte {
	// TODO(cparsons): Define configuration transitions programmatically based
	// on available archs.
	contents := `
#####################################################
# This file is generated by soong_build. Do not edit.
#####################################################

def _x86_64_transition_impl(settings, attr):
    return {
        "//command_line_option:platforms": "@sourceroot//build/bazel/platforms:generic_x86_64",
    }

def _x86_transition_impl(settings, attr):
    return {
        "//command_line_option:platforms": "@sourceroot//build/bazel/platforms:generic_x86",
    }

def _arm64_transition_impl(settings, attr):
    return {
        "//command_line_option:platforms": "@sourceroot//build/bazel/platforms:generic_arm64",
    }

def _arm_transition_impl(settings, attr):
    return {
        "//command_line_option:platforms": "@sourceroot//build/bazel/platforms:generic_arm",
    }

x86_64_transition = transition(
    implementation = _x86_64_transition_impl,
    inputs = [],
    outputs = [
        "//command_line_option:platforms",
    ],
)

x86_transition = transition(
    implementation = _x86_transition_impl,
    inputs = [],
    outputs = [
        "//command_line_option:platforms",
    ],
)

arm64_transition = transition(
    implementation = _arm64_transition_impl,
    inputs = [],
    outputs = [
        "//command_line_option:platforms",
    ],
)

arm_transition = transition(
    implementation = _arm_transition_impl,
    inputs = [],
    outputs = [
        "//command_line_option:platforms",
    ],
)

def _mixed_build_root_impl(ctx):
    all_files = ctx.files.deps_x86_64 + ctx.files.deps_x86 + ctx.files.deps_arm64 + ctx.files.deps_arm
    return [DefaultInfo(files = depset(all_files))]

# Rule representing the root of the build, to depend on all Bazel targets that
# are required for the build. Building this target will build the entire Bazel
# build tree.
mixed_build_root = rule(
    implementation = _mixed_build_root_impl,
    attrs = {
        "deps_x86_64" : attr.label_list(cfg = x86_64_transition),
        "deps_x86" : attr.label_list(cfg = x86_transition),
        "deps_arm64" : attr.label_list(cfg = arm64_transition),
        "deps_arm" : attr.label_list(cfg = arm_transition),
        "_allowlist_function_transition": attr.label(default = "@bazel_tools//tools/allowlists/function_transition_allowlist"),
    },
)

def _phony_root_impl(ctx):
    return []

# Rule to depend on other targets but build nothing.
# This is useful as follows: building a target of this rule will generate
# symlink forests for all dependencies of the target, without executing any
# actions of the build.
phony_root = rule(
    implementation = _phony_root_impl,
    attrs = {"deps" : attr.label_list()},
)
`
	return []byte(contents)
}

// Returns a "canonicalized" corresponding to the given sourcetree-level label.
// This abstraction is required because a sourcetree label such as //foo/bar:baz
// must be referenced via the local repository prefix, such as
// @sourceroot//foo/bar:baz.
func canonicalizeLabel(label string) string {
	if strings.HasPrefix(label, "//") {
		return "@sourceroot" + label
	} else {
		return "@sourceroot//" + label
	}
}

func (context *bazelContext) mainBuildFileContents() []byte {
	// TODO(cparsons): Map label to attribute programmatically; don't use hard-coded
	// architecture mapping.
	formatString := `
# This file is generated by soong_build. Do not edit.
load(":main.bzl", "mixed_build_root", "phony_root")

mixed_build_root(name = "buildroot",
    deps_x86_64 = [%s],
    deps_x86 = [%s],
    deps_arm64 = [%s],
    deps_arm = [%s],
)

phony_root(name = "phonyroot",
    deps = [":buildroot"],
)
`
	var deps_x86_64 []string = nil
	var deps_x86 []string = nil
	var deps_arm64 []string = nil
	var deps_arm []string = nil
	for val, _ := range context.requests {
		labelString := fmt.Sprintf("\"%s\"", canonicalizeLabel(val.label))
		switch getArchString(val) {
		case "x86_64":
			deps_x86_64 = append(deps_x86_64, labelString)
		case "x86":
			deps_x86 = append(deps_x86, labelString)
		case "arm64":
			deps_arm64 = append(deps_arm64, labelString)
		case "arm":
			deps_arm = append(deps_arm, labelString)
		default:
			panic(fmt.Sprintf("unhandled architecture %s for %s", getArchString(val), val))
		}
	}

	return []byte(fmt.Sprintf(formatString,
		strings.Join(deps_x86_64, ",\n            "),
		strings.Join(deps_x86, ",\n            "),
		strings.Join(deps_arm64, ",\n            "),
		strings.Join(deps_arm, ",\n            ")))
}

func (context *bazelContext) cqueryStarlarkFileContents() []byte {
	formatString := `
# This file is generated by soong_build. Do not edit.
getAllFilesLabels = {
  %s
}

getCcObjectFilesLabels = {
  %s
}

def get_cc_object_files(target):
  result = []
  linker_inputs = providers(target)["CcInfo"].linking_context.linker_inputs.to_list()

  for linker_input in linker_inputs:
    for library in linker_input.libraries:
      for object in library.objects:
        result += [object.path]
  return result

def get_arch(target):
  buildoptions = build_options(target)
  platforms = build_options(target)["//command_line_option:platforms"]
  if len(platforms) != 1:
    # An individual configured target should have only one platform architecture.
    # Note that it's fine for there to be multiple architectures for the same label,
    # but each is its own configured target.
    fail("expected exactly 1 platform for " + str(target.label) + " but got " + str(platforms))
  platform_name = build_options(target)["//command_line_option:platforms"][0].name
  if platform_name == "host":
    return "HOST"
  elif not platform_name.startswith("generic_"):
    fail("expected platform name of the form 'generic_<arch>', but was " + str(platforms))
    return "UNKNOWN"
  return platform_name[len("generic_"):]

def format(target):
  id_string = str(target.label) + "|" + get_arch(target)
  if id_string in getAllFilesLabels:
    return id_string + ">>" + ', '.join([f.path for f in target.files.to_list()])
  elif id_string in getCcObjectFilesLabels:
    return id_string + ">>" + ', '.join(get_cc_object_files(target))
  else:
    # This target was not requested via cquery, and thus must be a dependency
    # of a requested target.
    return id_string + ">>NONE"
`
	var getAllFilesDeps []string = nil
	var getCcObjectFilesDeps []string = nil

	for val, _ := range context.requests {
		labelWithArch := getCqueryId(val)
		mapEntryString := fmt.Sprintf("%q : True", labelWithArch)
		switch val.requestType {
		case getAllFiles:
			getAllFilesDeps = append(getAllFilesDeps, mapEntryString)
		case getCcObjectFiles:
			getCcObjectFilesDeps = append(getCcObjectFilesDeps, mapEntryString)
		}
	}
	getAllFilesDepsString := strings.Join(getAllFilesDeps, ",\n  ")
	getCcObjectFilesDepsString := strings.Join(getCcObjectFilesDeps, ",\n  ")

	return []byte(fmt.Sprintf(formatString, getAllFilesDepsString, getCcObjectFilesDepsString))
}

// Returns a workspace-relative path containing build-related metadata required
// for interfacing with Bazel. Example: out/soong/bazel.
func (context *bazelContext) intermediatesDir() string {
	return filepath.Join(context.buildDir, "bazel")
}

// Issues commands to Bazel to receive results for all cquery requests
// queued in the BazelContext.
func (context *bazelContext) InvokeBazel() error {
	context.results = make(map[cqueryKey]string)

	var cqueryOutput string
	var err error

	intermediatesDirPath := absolutePath(context.intermediatesDir())
	if _, err := os.Stat(intermediatesDirPath); os.IsNotExist(err) {
		err = os.Mkdir(intermediatesDirPath, 0777)
	}

	if err != nil {
		return err
	}
	err = ioutil.WriteFile(
		absolutePath(filepath.Join(context.intermediatesDir(), "main.bzl")),
		context.mainBzlFileContents(), 0666)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(
		absolutePath(filepath.Join(context.intermediatesDir(), "BUILD.bazel")),
		context.mainBuildFileContents(), 0666)
	if err != nil {
		return err
	}
	cqueryFileRelpath := filepath.Join(context.intermediatesDir(), "buildroot.cquery")
	err = ioutil.WriteFile(
		absolutePath(cqueryFileRelpath),
		context.cqueryStarlarkFileContents(), 0666)
	if err != nil {
		return err
	}
	workspaceFileRelpath := filepath.Join(context.intermediatesDir(), "WORKSPACE.bazel")
	err = ioutil.WriteFile(
		absolutePath(workspaceFileRelpath),
		context.workspaceFileContents(), 0666)
	if err != nil {
		return err
	}
	buildrootLabel := "//:buildroot"
	cqueryOutput, err = context.issueBazelCommand(bazel.CqueryBuildRootRunName, "cquery",
		[]string{fmt.Sprintf("kind(rule, deps(%s))", buildrootLabel)},
		"--output=starlark",
		"--starlark:file="+cqueryFileRelpath)
	err = ioutil.WriteFile(
		absolutePath(filepath.Join(context.intermediatesDir(), "cquery.out")),
		[]byte(cqueryOutput), 0666)
	if err != nil {
		return err
	}

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
		if cqueryResult, ok := cqueryResults[getCqueryId(val)]; ok {
			context.results[val] = string(cqueryResult)
		} else {
			return fmt.Errorf("missing result for bazel target %s. query output: [%s]", getCqueryId(val), cqueryOutput)
		}
	}

	// Issue an aquery command to retrieve action information about the bazel build tree.
	//
	// TODO(cparsons): Use --target_pattern_file to avoid command line limits.
	var aqueryOutput string
	aqueryOutput, err = context.issueBazelCommand(bazel.AqueryBuildRootRunName, "aquery",
		[]string{fmt.Sprintf("deps(%s)", buildrootLabel),
			// Use jsonproto instead of proto; actual proto parsing would require a dependency on Bazel's
			// proto sources, which would add a number of unnecessary dependencies.
			"--output=jsonproto"})

	if err != nil {
		return err
	}

	context.buildStatements, err = bazel.AqueryBuildStatements([]byte(aqueryOutput))
	if err != nil {
		return err
	}

	// Issue a build command of the phony root to generate symlink forests for dependencies of the
	// Bazel build. This is necessary because aquery invocations do not generate this symlink forest,
	// but some of symlinks may be required to resolve source dependencies of the build.
	_, err = context.issueBazelCommand(bazel.BazelBuildPhonyRootRunName, "build",
		[]string{"//:phonyroot"})

	if err != nil {
		return err
	}

	// Clear requests.
	context.requests = map[cqueryKey]bool{}
	return nil
}

func (context *bazelContext) BuildStatementsToRegister() []bazel.BuildStatement {
	return context.buildStatements
}

func (context *bazelContext) OutputBase() string {
	return context.outputBase
}

// Singleton used for registering BUILD file ninja dependencies (needed
// for correctness of builds which use Bazel.
func BazelSingleton() Singleton {
	return &bazelSingleton{}
}

type bazelSingleton struct{}

func (c *bazelSingleton) GenerateBuildActions(ctx SingletonContext) {
	// bazelSingleton is a no-op if mixed-soong-bazel-builds are disabled.
	if !ctx.Config().BazelContext.BazelEnabled() {
		return
	}

	// Add ninja file dependencies for files which all bazel invocations require.
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

	// Register bazel-owned build statements (obtained from the aquery invocation).
	for index, buildStatement := range ctx.Config().BazelContext.BuildStatementsToRegister() {
		if len(buildStatement.Command) < 1 {
			panic(fmt.Sprintf("unhandled build statement: %s", buildStatement))
		}
		rule := NewRuleBuilder(pctx, ctx)
		cmd := rule.Command()
		cmd.Text(fmt.Sprintf("cd %s/execroot/__main__ && %s",
			ctx.Config().BazelContext.OutputBase(), buildStatement.Command))

		for _, outputPath := range buildStatement.OutputPaths {
			cmd.ImplicitOutput(PathForBazelOut(ctx, outputPath))
		}
		for _, inputPath := range buildStatement.InputPaths {
			cmd.Implicit(PathForBazelOut(ctx, inputPath))
		}

		// This is required to silence warnings pertaining to unexpected timestamps. Particularly,
		// some Bazel builtins (such as files in the bazel_tools directory) have far-future
		// timestamps. Without restat, Ninja would emit warnings that the input files of a
		// build statement have later timestamps than the outputs.
		rule.Restat()

		rule.Build(fmt.Sprintf("bazel %d", index), buildStatement.Mnemonic)
	}
}

func getCqueryId(key cqueryKey) string {
	return canonicalizeLabel(key.label) + "|" + getArchString(key)
}

func getArchString(key cqueryKey) string {
	arch := key.archType.Name
	if len(arch) > 0 {
		return arch
	} else {
		return "x86_64"
	}
}
