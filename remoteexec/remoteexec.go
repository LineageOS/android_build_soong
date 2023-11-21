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
	"fmt"
	"sort"
	"strings"
)

const (
	// ContainerImageKey is the key identifying the container image in the platform spec.
	ContainerImageKey = "container-image"

	// PoolKey is the key identifying the pool to use for remote execution.
	PoolKey = "Pool"

	// DefaultImage is the default container image used for Android remote execution. The
	// image was built with the Dockerfile at
	// https://android.googlesource.com/platform/prebuilts/remoteexecution-client/+/refs/heads/master/docker/Dockerfile
	DefaultImage = "docker://gcr.io/androidbuild-re-dockerimage/android-build-remoteexec-image@sha256:1eb7f64b9e17102b970bd7a1af7daaebdb01c3fb777715899ef462d6c6d01a45"

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
	defaultLabels               = map[string]string{"type": "tool"}
	defaultExecStrategy         = LocalExecStrategy
	defaultEnvironmentVariables = []string{
		// This is a subset of the allowlist in ui/build/ninja.go that makes sense remotely.
		"LANG",
		"LC_MESSAGES",
		"PYTHONDONTWRITEBYTECODE",
	}
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
	// RSPFiles is the name of the files used by the rule as a placeholder for an rsp input.
	RSPFiles []string
	// OutputFiles is a list of output file paths or ninja variables as placeholders for rule
	// outputs.
	OutputFiles []string
	// OutputDirectories is a list of output directories or ninja variables as placeholders for
	// rule output directories.
	OutputDirectories []string
	// ToolchainInputs is a list of paths or ninja variables pointing to the location of
	// toolchain binaries used by the rule.
	ToolchainInputs []string
	// EnvironmentVariables is a list of environment variables whose values should be passed through
	// to the remote execution.
	EnvironmentVariables []string
	// Boolean indicating whether to compare chosen exec strategy with local execution.
	Compare bool
	// Number of times the action should be rerun locally.
	NumLocalRuns int
	// Number of times the action should be rerun remotely.
	NumRemoteRuns int
	// Boolean indicating whether to update remote cache entry. Rewrapper defaults to true, so the name is negated here.
	NoRemoteUpdateCache bool
}

func init() {
}

// Template generates the remote execution wrapper template to be added as a prefix to the rule's
// command.
func (r *REParams) Template() string {
	return "${android.RBEWrapper}" + r.wrapperArgs()
}

// NoVarTemplate generates the remote execution wrapper template without variables, to be used in
// RuleBuilder.
func (r *REParams) NoVarTemplate(wrapper string) string {
	return wrapper + r.wrapperArgs()
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

	if r.Compare && r.NumLocalRuns >= 0 && r.NumRemoteRuns >= 0 {
		args += fmt.Sprintf(" --compare=true --num_local_reruns=%d --num_remote_reruns=%d", r.NumLocalRuns, r.NumRemoteRuns)
	}

	if r.NoRemoteUpdateCache {
		args += " --remote_update_cache=false"
	}

	if len(r.Inputs) > 0 {
		args += " --inputs=" + strings.Join(r.Inputs, ",")
	}

	if len(r.RSPFiles) > 0 {
		args += " --input_list_paths=" + strings.Join(r.RSPFiles, ",")
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

	envVarAllowlist := append(r.EnvironmentVariables, defaultEnvironmentVariables...)

	if len(envVarAllowlist) > 0 {
		args += " --env_var_allowlist=" + strings.Join(envVarAllowlist, ",")
	}

	return args + " -- "
}
