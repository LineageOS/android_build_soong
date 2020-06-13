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

package remoteexec

import (
	"sort"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
)

const (
	// ContainerImageKey is the key identifying the container image in the platform spec.
	ContainerImageKey = "container-image"

	// PoolKey is the key identifying the pool to use for remote execution.
	PoolKey = "Pool"

	// DefaultImage is the default container image used for Android remote execution. The
	// image was built with the Dockerfile at
	// https://android.googlesource.com/platform/prebuilts/remoteexecution-client/+/refs/heads/master/docker/Dockerfile
	DefaultImage = "docker://gcr.io/androidbuild-re-dockerimage/android-build-remoteexec-image@sha256:582efb38f0c229ea39952fff9e132ccbe183e14869b39888010dacf56b360d62"

	// DefaultWrapperPath is the default path to the remote execution wrapper.
	DefaultWrapperPath = "prebuilts/remoteexecution-client/live/rewrapper"

	// DefaultPool is the name of the pool to use for remote execution when none is specified.
	DefaultPool = "default"

	// LocalExecStrategy is the exec strategy to indicate that the action should be run locally.
	LocalExecStrategy = "local"

	// RemoteExecStrategy is the exec strategy to indicate that the action should be run
	// remotely.
	RemoteExecStrategy = "remote"

	// RemoteLocalFallbackExecStrategy is the exec strategy to indicate that the action should
	// be run remotely and fallback to local execution if remote fails.
	RemoteLocalFallbackExecStrategy = "remote_local_fallback"
)

var (
	defaultLabels       = map[string]string{"type": "tool"}
	defaultExecStrategy = LocalExecStrategy
	pctx                = android.NewPackageContext("android/soong/remoteexec")
)

// REParams holds information pertinent to the remote execution of a rule.
type REParams struct {
	// Platform is the key value pair used for remotely executing the action.
	Platform map[string]string
	// Labels is a map of labels that identify the rule.
	Labels map[string]string
	// ExecStrategy is the remote execution strategy: remote, local, or remote_local_fallback.
	ExecStrategy string
	// Inputs is a list of input paths or ninja variables.
	Inputs []string
	// RSPFile is the name of the ninja variable used by the rule as a placeholder for an rsp
	// input.
	RSPFile string
	// OutputFiles is a list of output file paths or ninja variables as placeholders for rule
	// outputs.
	OutputFiles []string
	// OutputDirectories is a list of output directories or ninja variables as placeholders for
	// rule output directories.
	OutputDirectories []string
	// ToolchainInputs is a list of paths or ninja variables pointing to the location of
	// toolchain binaries used by the rule.
	ToolchainInputs []string
}

func init() {
	pctx.VariableFunc("Wrapper", func(ctx android.PackageVarContext) string {
		return wrapper(ctx.Config())
	})
}

func wrapper(cfg android.Config) string {
	if override := cfg.Getenv("RBE_WRAPPER"); override != "" {
		return override
	}
	return DefaultWrapperPath
}

// Template generates the remote execution wrapper template to be added as a prefix to the rule's
// command.
func (r *REParams) Template() string {
	return "${remoteexec.Wrapper}" + r.wrapperArgs()
}

// NoVarTemplate generates the remote execution wrapper template without variables, to be used in
// RuleBuilder.
func (r *REParams) NoVarTemplate(cfg android.Config) string {
	return wrapper(cfg) + r.wrapperArgs()
}

func (r *REParams) wrapperArgs() string {
	args := ""
	var kvs []string
	labels := r.Labels
	if len(labels) == 0 {
		labels = defaultLabels
	}
	for k, v := range labels {
		kvs = append(kvs, k+"="+v)
	}
	sort.Strings(kvs)
	args += " --labels=" + strings.Join(kvs, ",")

	var platform []string
	for k, v := range r.Platform {
		if v == "" {
			continue
		}
		platform = append(platform, k+"="+v)
	}
	if _, ok := r.Platform[ContainerImageKey]; !ok {
		platform = append(platform, ContainerImageKey+"="+DefaultImage)
	}
	if platform != nil {
		sort.Strings(platform)
		args += " --platform=\"" + strings.Join(platform, ",") + "\""
	}

	strategy := r.ExecStrategy
	if strategy == "" {
		strategy = defaultExecStrategy
	}
	args += " --exec_strategy=" + strategy

	if len(r.Inputs) > 0 {
		args += " --inputs=" + strings.Join(r.Inputs, ",")
	}

	if r.RSPFile != "" {
		args += " --input_list_paths=" + r.RSPFile
	}

	if len(r.OutputFiles) > 0 {
		args += " --output_files=" + strings.Join(r.OutputFiles, ",")
	}

	if len(r.OutputDirectories) > 0 {
		args += " --output_directories=" + strings.Join(r.OutputDirectories, ",")
	}

	if len(r.ToolchainInputs) > 0 {
		args += " --toolchain_inputs=" + strings.Join(r.ToolchainInputs, ",")
	}

	return args + " -- "
}

// StaticRules returns a pair of rules based on the given RuleParams, where the first rule is a
// locally executable rule and the second rule is a remotely executable rule. commonArgs are args
// used for both the local and remotely executable rules. reArgs are used only for remote
// execution.
func StaticRules(ctx android.PackageContext, name string, ruleParams blueprint.RuleParams, reParams *REParams, commonArgs []string, reArgs []string) (blueprint.Rule, blueprint.Rule) {
	ruleParamsRE := ruleParams
	ruleParams.Command = strings.ReplaceAll(ruleParams.Command, "$reTemplate", "")
	ruleParamsRE.Command = strings.ReplaceAll(ruleParamsRE.Command, "$reTemplate", reParams.Template())

	return ctx.AndroidStaticRule(name, ruleParams, commonArgs...),
		ctx.AndroidRemoteStaticRule(name+"RE", android.RemoteRuleSupports{RBE: true}, ruleParamsRE, append(commonArgs, reArgs...)...)
}

// MultiCommandStaticRules returns a pair of rules based on the given RuleParams, where the first
// rule is a locally executable rule and the second rule is a remotely executable rule. This
// function supports multiple remote execution wrappers placed in the template when commands are
// chained together with &&. commonArgs are args used for both the local and remotely executable
// rules. reArgs are args used only for remote execution.
func MultiCommandStaticRules(ctx android.PackageContext, name string, ruleParams blueprint.RuleParams, reParams map[string]*REParams, commonArgs []string, reArgs []string) (blueprint.Rule, blueprint.Rule) {
	ruleParamsRE := ruleParams
	for k, v := range reParams {
		ruleParams.Command = strings.ReplaceAll(ruleParams.Command, k, "")
		ruleParamsRE.Command = strings.ReplaceAll(ruleParamsRE.Command, k, v.Template())
	}

	return ctx.AndroidStaticRule(name, ruleParams, commonArgs...),
		ctx.AndroidRemoteStaticRule(name+"RE", android.RemoteRuleSupports{RBE: true}, ruleParamsRE, append(commonArgs, reArgs...)...)
}

// EnvOverrideFunc retrieves a variable func that evaluates to the value of the given environment
// variable if set, otherwise the given default.
func EnvOverrideFunc(envVar, defaultVal string) func(ctx android.PackageVarContext) string {
	return func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv(envVar); override != "" {
			return override
		}
		return defaultVal
	}
}
