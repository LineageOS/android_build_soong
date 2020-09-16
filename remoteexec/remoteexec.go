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
	// ToolchainInputs is a list of paths or ninja variables pointing to the location of
	// toolchain binaries used by the rule.
	ToolchainInputs []string
}

func init() {
	pctx.VariableFunc("Wrapper", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("RBE_WRAPPER"); override != "" {
			return override
		}
		return DefaultWrapperPath
	})
}

// Generate the remote execution wrapper template to be added as a prefix to the rule's command.
func (r *REParams) Template() string {
	template := "${remoteexec.Wrapper}"

	var kvs []string
	labels := r.Labels
	if len(labels) == 0 {
		labels = defaultLabels
	}
	for k, v := range labels {
		kvs = append(kvs, k+"="+v)
	}
	sort.Strings(kvs)
	template += " --labels=" + strings.Join(kvs, ",")

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
		template += " --platform=\"" + strings.Join(platform, ",") + "\""
	}

	strategy := r.ExecStrategy
	if strategy == "" {
		strategy = defaultExecStrategy
	}
	template += " --exec_strategy=" + strategy

	if len(r.Inputs) > 0 {
		template += " --inputs=" + strings.Join(r.Inputs, ",")
	}

	if r.RSPFile != "" {
		template += " --input_list_paths=" + r.RSPFile
	}

	if len(r.OutputFiles) > 0 {
		template += " --output_files=" + strings.Join(r.OutputFiles, ",")
	}

	if len(r.ToolchainInputs) > 0 {
		template += " --toolchain_inputs=" + strings.Join(r.ToolchainInputs, ",")
	}

	return template + " -- "
}

// StaticRules returns a pair of rules based on the given RuleParams, where the first rule is a
// locally executable rule and the second rule is a remotely executable rule.
func StaticRules(ctx android.PackageContext, name string, ruleParams blueprint.RuleParams, reParams *REParams, commonArgs []string, reArgs []string) (blueprint.Rule, blueprint.Rule) {
	ruleParamsRE := ruleParams
	ruleParamsRE.Command = reParams.Template() + ruleParamsRE.Command

	return ctx.AndroidStaticRule(name, ruleParams, commonArgs...),
		ctx.AndroidRemoteStaticRule(name+"RE", android.RemoteRuleSupports{RBE: true}, ruleParamsRE, append(commonArgs, reArgs...)...)
}
