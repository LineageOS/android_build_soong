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

	"android/soong/bazel/cquery"

	"github.com/google/blueprint/bootstrap"

	"android/soong/bazel"
	"android/soong/shared"
)

type cqueryRequest interface {
	// Name returns a string name for this request type. Such request type names must be unique,
	// and must only consist of alphanumeric characters.
	Name() string

	// StarlarkFunctionBody returns a starlark function body to process this request type.
	// The returned string is the body of a Starlark function which obtains
	// all request-relevant information about a target and returns a string containing
	// this information.
	// The function should have the following properties:
	//   - `target` is the only parameter to this function (a configured target).
	//   - The return value must be a string.
	//   - The function body should not be indented outside of its own scope.
	StarlarkFunctionBody() string
}

// Map key to describe bazel cquery requests.
type cqueryKey struct {
	label       string
	requestType cqueryRequest
	archType    ArchType
}

type BazelContext interface {
	// The below methods involve queuing cquery requests to be later invoked
	// by bazel. If any of these methods return (_, false), then the request
	// has been queued to be run later.

	// Returns result files built by building the given bazel target label.
	GetOutputFiles(label string, archType ArchType) ([]string, bool)

	// TODO(cparsons): Other cquery-related methods should be added here.
	// Returns the results of GetOutputFiles and GetCcObjectFiles in a single query (in that order).
	GetCcInfo(label string, archType ArchType) (cquery.CcInfo, bool, error)

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

type bazelRunner interface {
	issueBazelCommand(paths *bazelPaths, runName bazel.RunName, command bazelCommand, extraFlags ...string) (string, string, error)
}

type bazelPaths struct {
	homeDir      string
	bazelPath    string
	outputBase   string
	workspaceDir string
	buildDir     string
	metricsDir   string
}

// A context object which tracks queued requests that need to be made to Bazel,
// and their results after the requests have been made.
type bazelContext struct {
	bazelRunner
	paths        *bazelPaths
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
	OutputBaseDir string

	LabelToOutputFiles map[string][]string
	LabelToCcInfo      map[string]cquery.CcInfo
}

func (m MockBazelContext) GetOutputFiles(label string, archType ArchType) ([]string, bool) {
	result, ok := m.LabelToOutputFiles[label]
	return result, ok
}

func (m MockBazelContext) GetCcInfo(label string, archType ArchType) (cquery.CcInfo, bool, error) {
	result, ok := m.LabelToCcInfo[label]
	return result, ok, nil
}

func (m MockBazelContext) InvokeBazel() error {
	panic("unimplemented")
}

func (m MockBazelContext) BazelEnabled() bool {
	return true
}

func (m MockBazelContext) OutputBase() string { return m.OutputBaseDir }

func (m MockBazelContext) BuildStatementsToRegister() []bazel.BuildStatement {
	return []bazel.BuildStatement{}
}

var _ BazelContext = MockBazelContext{}

func (bazelCtx *bazelContext) GetOutputFiles(label string, archType ArchType) ([]string, bool) {
	rawString, ok := bazelCtx.cquery(label, cquery.GetOutputFiles, archType)
	var ret []string
	if ok {
		bazelOutput := strings.TrimSpace(rawString)
		ret = cquery.GetOutputFiles.ParseResult(bazelOutput)
	}
	return ret, ok
}

func (bazelCtx *bazelContext) GetCcInfo(label string, archType ArchType) (cquery.CcInfo, bool, error) {
	result, ok := bazelCtx.cquery(label, cquery.GetCcInfo, archType)
	if !ok {
		return cquery.CcInfo{}, ok, nil
	}

	bazelOutput := strings.TrimSpace(result)
	ret, err := cquery.GetCcInfo.ParseResult(bazelOutput)
	return ret, ok, err
}

func (n noopBazelContext) GetOutputFiles(label string, archType ArchType) ([]string, bool) {
	panic("unimplemented")
}

func (n noopBazelContext) GetCcInfo(label string, archType ArchType) (cquery.CcInfo, bool, error) {
	panic("unimplemented")
}

func (n noopBazelContext) GetPrebuiltCcStaticLibraryFiles(label string, archType ArchType) ([]string, bool) {
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

	p, err := bazelPathsFromConfig(c)
	if err != nil {
		return nil, err
	}
	return &bazelContext{
		bazelRunner: &builtinBazelRunner{},
		paths:       p,
		requests:    make(map[cqueryKey]bool),
	}, nil
}

func bazelPathsFromConfig(c *config) (*bazelPaths, error) {
	p := bazelPaths{
		buildDir: c.buildDir,
	}
	missingEnvVars := []string{}
	if len(c.Getenv("BAZEL_HOME")) > 1 {
		p.homeDir = c.Getenv("BAZEL_HOME")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_HOME")
	}
	if len(c.Getenv("BAZEL_PATH")) > 1 {
		p.bazelPath = c.Getenv("BAZEL_PATH")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_PATH")
	}
	if len(c.Getenv("BAZEL_OUTPUT_BASE")) > 1 {
		p.outputBase = c.Getenv("BAZEL_OUTPUT_BASE")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_OUTPUT_BASE")
	}
	if len(c.Getenv("BAZEL_WORKSPACE")) > 1 {
		p.workspaceDir = c.Getenv("BAZEL_WORKSPACE")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_WORKSPACE")
	}
	if len(c.Getenv("BAZEL_METRICS_DIR")) > 1 {
		p.metricsDir = c.Getenv("BAZEL_METRICS_DIR")
	} else {
		missingEnvVars = append(missingEnvVars, "BAZEL_METRICS_DIR")
	}
	if len(missingEnvVars) > 0 {
		return nil, errors.New(fmt.Sprintf("missing required env vars to use bazel: %s", missingEnvVars))
	} else {
		return &p, nil
	}
}

func (p *bazelPaths) BazelMetricsDir() string {
	return p.metricsDir
}

func (context *bazelContext) BazelEnabled() bool {
	return true
}

// Adds a cquery request to the Bazel request queue, to be later invoked, or
// returns the result of the given request if the request was already made.
// If the given request was already made (and the results are available), then
// returns (result, true). If the request is queued but no results are available,
// then returns ("", false).
func (context *bazelContext) cquery(label string, requestType cqueryRequest,
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

type bazelCommand struct {
	command string
	// query or label
	expression string
}

type mockBazelRunner struct {
	bazelCommandResults map[bazelCommand]string
	commands            []bazelCommand
}

func (r *mockBazelRunner) issueBazelCommand(paths *bazelPaths,
	runName bazel.RunName,
	command bazelCommand,
	extraFlags ...string) (string, string, error) {
	r.commands = append(r.commands, command)
	if ret, ok := r.bazelCommandResults[command]; ok {
		return ret, "", nil
	}
	return "", "", nil
}

type builtinBazelRunner struct{}

// Issues the given bazel command with given build label and additional flags.
// Returns (stdout, stderr, error). The first and second return values are strings
// containing the stdout and stderr of the run command, and an error is returned if
// the invocation returned an error code.
func (r *builtinBazelRunner) issueBazelCommand(paths *bazelPaths, runName bazel.RunName, command bazelCommand,
	extraFlags ...string) (string, string, error) {
	cmdFlags := []string{"--output_base=" + absolutePath(paths.outputBase), command.command}
	cmdFlags = append(cmdFlags, command.expression)
	cmdFlags = append(cmdFlags, "--profile="+shared.BazelMetricsFilename(paths, runName))

	// Set default platforms to canonicalized values for mixed builds requests.
	// If these are set in the bazelrc, they will have values that are
	// non-canonicalized to @sourceroot labels, and thus be invalid when
	// referenced from the buildroot.
	//
	// The actual platform values here may be overridden by configuration
	// transitions from the buildroot.
	cmdFlags = append(cmdFlags,
		fmt.Sprintf("--platforms=%s", "//build/bazel/platforms:android_x86_64"))
	cmdFlags = append(cmdFlags,
		fmt.Sprintf("--extra_toolchains=%s", "//prebuilts/clang/host/linux-x86:all"))
	// This should be parameterized on the host OS, but let's restrict to linux
	// to keep things simple for now.
	cmdFlags = append(cmdFlags,
		fmt.Sprintf("--host_platform=%s", "//build/bazel/platforms:linux_x86_64"))

	// Explicitly disable downloading rules (such as canonical C++ and Java rules) from the network.
	cmdFlags = append(cmdFlags, "--experimental_repository_disable_download")
	cmdFlags = append(cmdFlags, extraFlags...)

	bazelCmd := exec.Command(paths.bazelPath, cmdFlags...)
	bazelCmd.Dir = absolutePath(paths.syntheticWorkspaceDir())
	bazelCmd.Env = append(os.Environ(),
		"HOME="+paths.homeDir,
		pwdPrefix(),
		"BUILD_DIR="+absolutePath(paths.buildDir),
		// Disables local host detection of gcc; toolchain information is defined
		// explicitly in BUILD files.
		"BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=1")
	stderr := &bytes.Buffer{}
	bazelCmd.Stderr = stderr

	if output, err := bazelCmd.Output(); err != nil {
		return "", string(stderr.Bytes()),
			fmt.Errorf("bazel command failed. command: [%s], env: [%s], error [%s]", bazelCmd, bazelCmd.Env, stderr)
	} else {
		return string(output), string(stderr.Bytes()), nil
	}
}

func (context *bazelContext) mainBzlFileContents() []byte {
	// TODO(cparsons): Define configuration transitions programmatically based
	// on available archs.
	contents := `
#####################################################
# This file is generated by soong_build. Do not edit.
#####################################################

def _config_node_transition_impl(settings, attr):
    return {
        "//command_line_option:platforms": "@//build/bazel/platforms:android_%s" % attr.arch,
    }

_config_node_transition = transition(
    implementation = _config_node_transition_impl,
    inputs = [],
    outputs = [
        "//command_line_option:platforms",
    ],
)

def _passthrough_rule_impl(ctx):
    return [DefaultInfo(files = depset(ctx.files.deps))]

config_node = rule(
    implementation = _passthrough_rule_impl,
    attrs = {
        "arch" : attr.string(mandatory = True),
        "deps" : attr.label_list(cfg = _config_node_transition),
        "_allowlist_function_transition": attr.label(default = "@bazel_tools//tools/allowlists/function_transition_allowlist"),
    },
)


# Rule representing the root of the build, to depend on all Bazel targets that
# are required for the build. Building this target will build the entire Bazel
# build tree.
mixed_build_root = rule(
    implementation = _passthrough_rule_impl,
    attrs = {
        "deps" : attr.label_list(),
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

func (context *bazelContext) mainBuildFileContents() []byte {
	// TODO(cparsons): Map label to attribute programmatically; don't use hard-coded
	// architecture mapping.
	formatString := `
# This file is generated by soong_build. Do not edit.
load(":main.bzl", "config_node", "mixed_build_root", "phony_root")

%s

mixed_build_root(name = "buildroot",
    deps = [%s],
)

phony_root(name = "phonyroot",
    deps = [":buildroot"],
)
`
	configNodeFormatString := `
config_node(name = "%s",
    arch = "%s",
    deps = [%s],
)
`

	configNodesSection := ""

	labelsByArch := map[string][]string{}
	for val, _ := range context.requests {
		labelString := fmt.Sprintf("\"@%s\"", val.label)
		archString := getArchString(val)
		labelsByArch[archString] = append(labelsByArch[archString], labelString)
	}

	configNodeLabels := []string{}
	for archString, labels := range labelsByArch {
		configNodeLabels = append(configNodeLabels, fmt.Sprintf("\":%s\"", archString))
		labelsString := strings.Join(labels, ",\n            ")
		configNodesSection += fmt.Sprintf(configNodeFormatString, archString, archString, labelsString)
	}

	return []byte(fmt.Sprintf(formatString, configNodesSection, strings.Join(configNodeLabels, ",\n            ")))
}

func indent(original string) string {
	result := ""
	for _, line := range strings.Split(original, "\n") {
		result += "  " + line + "\n"
	}
	return result
}

// Returns the file contents of the buildroot.cquery file that should be used for the cquery
// expression in order to obtain information about buildroot and its dependencies.
// The contents of this file depend on the bazelContext's requests; requests are enumerated
// and grouped by their request type. The data retrieved for each label depends on its
// request type.
func (context *bazelContext) cqueryStarlarkFileContents() []byte {
	requestTypeToCqueryIdEntries := map[cqueryRequest][]string{}
	for val, _ := range context.requests {
		cqueryId := getCqueryId(val)
		mapEntryString := fmt.Sprintf("%q : True", cqueryId)
		requestTypeToCqueryIdEntries[val.requestType] =
			append(requestTypeToCqueryIdEntries[val.requestType], mapEntryString)
	}
	labelRegistrationMapSection := ""
	functionDefSection := ""
	mainSwitchSection := ""

	mapDeclarationFormatString := `
%s = {
  %s
}
`
	functionDefFormatString := `
def %s(target):
%s
`
	mainSwitchSectionFormatString := `
  if id_string in %s:
    return id_string + ">>" + %s(target)
`

	for requestType, _ := range requestTypeToCqueryIdEntries {
		labelMapName := requestType.Name() + "_Labels"
		functionName := requestType.Name() + "_Fn"
		labelRegistrationMapSection += fmt.Sprintf(mapDeclarationFormatString,
			labelMapName,
			strings.Join(requestTypeToCqueryIdEntries[requestType], ",\n  "))
		functionDefSection += fmt.Sprintf(functionDefFormatString,
			functionName,
			indent(requestType.StarlarkFunctionBody()))
		mainSwitchSection += fmt.Sprintf(mainSwitchSectionFormatString,
			labelMapName, functionName)
	}

	formatString := `
# This file is generated by soong_build. Do not edit.

# Label Map Section
%s

# Function Def Section
%s

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
  elif not platform_name.startswith("android_"):
    fail("expected platform name of the form 'android_<arch>', but was " + str(platforms))
    return "UNKNOWN"
  return platform_name[len("android_"):]

def format(target):
  id_string = str(target.label) + "|" + get_arch(target)

  # Main switch section
  %s
  # This target was not requested via cquery, and thus must be a dependency
  # of a requested target.
  return id_string + ">>NONE"
`

	return []byte(fmt.Sprintf(formatString, labelRegistrationMapSection, functionDefSection,
		mainSwitchSection))
}

// Returns a path containing build-related metadata required for interfacing
// with Bazel. Example: out/soong/bazel.
func (p *bazelPaths) intermediatesDir() string {
	return filepath.Join(p.buildDir, "bazel")
}

// Returns the path where the contents of the @soong_injection repository live.
// It is used by Soong to tell Bazel things it cannot over the command line.
func (p *bazelPaths) injectedFilesDir() string {
	return filepath.Join(p.buildDir, "soong_injection")
}

// Returns the path of the synthetic Bazel workspace that contains a symlink
// forest composed the whole source tree and BUILD files generated by bp2build.
func (p *bazelPaths) syntheticWorkspaceDir() string {
	return filepath.Join(p.buildDir, "workspace")
}

// Issues commands to Bazel to receive results for all cquery requests
// queued in the BazelContext.
func (context *bazelContext) InvokeBazel() error {
	context.results = make(map[cqueryKey]string)

	var cqueryOutput string
	var cqueryErr string
	var err error

	soongInjectionPath := absolutePath(context.paths.injectedFilesDir())
	mixedBuildsPath := filepath.Join(soongInjectionPath, "mixed_builds")
	if _, err := os.Stat(mixedBuildsPath); os.IsNotExist(err) {
		err = os.MkdirAll(mixedBuildsPath, 0777)
	}
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(soongInjectionPath, "WORKSPACE.bazel"), []byte{}, 0666)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(
		filepath.Join(mixedBuildsPath, "main.bzl"),
		context.mainBzlFileContents(), 0666)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(
		filepath.Join(mixedBuildsPath, "BUILD.bazel"),
		context.mainBuildFileContents(), 0666)
	if err != nil {
		return err
	}
	cqueryFileRelpath := filepath.Join(context.paths.injectedFilesDir(), "buildroot.cquery")
	err = ioutil.WriteFile(
		absolutePath(cqueryFileRelpath),
		context.cqueryStarlarkFileContents(), 0666)
	if err != nil {
		return err
	}
	buildrootLabel := "@soong_injection//mixed_builds:buildroot"
	cqueryOutput, cqueryErr, err = context.issueBazelCommand(
		context.paths,
		bazel.CqueryBuildRootRunName,
		bazelCommand{"cquery", fmt.Sprintf("kind(rule, deps(%s))", buildrootLabel)},
		"--output=starlark",
		"--starlark:file="+absolutePath(cqueryFileRelpath))
	err = ioutil.WriteFile(filepath.Join(soongInjectionPath, "cquery.out"),
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
			return fmt.Errorf("missing result for bazel target %s. query output: [%s], cquery err: [%s]",
				getCqueryId(val), cqueryOutput, cqueryErr)
		}
	}

	// Issue an aquery command to retrieve action information about the bazel build tree.
	//
	// TODO(cparsons): Use --target_pattern_file to avoid command line limits.
	var aqueryOutput string
	aqueryOutput, _, err = context.issueBazelCommand(
		context.paths,
		bazel.AqueryBuildRootRunName,
		bazelCommand{"aquery", fmt.Sprintf("deps(%s)", buildrootLabel)},
		// Use jsonproto instead of proto; actual proto parsing would require a dependency on Bazel's
		// proto sources, which would add a number of unnecessary dependencies.
		"--output=jsonproto")

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
	_, _, err = context.issueBazelCommand(
		context.paths,
		bazel.BazelBuildPhonyRootRunName,
		bazelCommand{"build", "@soong_injection//mixed_builds:phonyroot"})

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
	return context.paths.outputBase
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
		filepath.Dir(bootstrap.CmdlineArgs.ModuleListFile), "bazel.list"))
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
			panic(fmt.Sprintf("unhandled build statement: %v", buildStatement))
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

		if depfile := buildStatement.Depfile; depfile != nil {
			cmd.ImplicitDepFile(PathForBazelOut(ctx, *depfile))
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
	return key.label + "|" + getArchString(key)
}

func getArchString(key cqueryKey) string {
	arch := key.archType.Name
	if len(arch) > 0 {
		return arch
	} else {
		return "x86_64"
	}
}
