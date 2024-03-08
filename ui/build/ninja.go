// Copyright 2017 Google Inc. All rights reserved.
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

package build

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"android/soong/shared"
	"android/soong/ui/metrics"
	"android/soong/ui/status"
)

const (
	// File containing the environment state when ninja is executed
	ninjaEnvFileName        = "ninja.environment"
	ninjaLogFileName        = ".ninja_log"
	ninjaWeightListFileName = ".ninja_weight_list"
)

// Constructs and runs the Ninja command line with a restricted set of
// environment variables. It's important to restrict the environment Ninja runs
// for hermeticity reasons, and to avoid spurious rebuilds.
func runNinjaForBuild(ctx Context, config Config) {
	ctx.BeginTrace(metrics.PrimaryNinja, "ninja")
	defer ctx.EndTrace()

	// Sets up the FIFO status updater that reads the Ninja protobuf output, and
	// translates it to the soong_ui status output, displaying real-time
	// progress of the build.
	fifo := filepath.Join(config.OutDir(), ".ninja_fifo")
	nr := status.NewNinjaReader(ctx, ctx.Status.StartTool(), fifo)
	defer nr.Close()

	executable := config.PrebuiltBuildTool("ninja")
	args := []string{
		"-d", "keepdepfile",
		"-d", "keeprsp",
		"-d", "stats",
		"--frontend_file", fifo,
	}

	args = append(args, config.NinjaArgs()...)

	var parallel int
	if config.UseRemoteBuild() {
		parallel = config.RemoteParallel()
	} else {
		parallel = config.Parallel()
	}
	args = append(args, "-j", strconv.Itoa(parallel))
	if config.keepGoing != 1 {
		args = append(args, "-k", strconv.Itoa(config.keepGoing))
	}

	args = append(args, "-f", config.CombinedNinjaFile())

	args = append(args,
		"-o", "usesphonyoutputs=yes",
		"-w", "dupbuild=err",
		"-w", "missingdepfile=err")

	cmd := Command(ctx, config, "ninja", executable, args...)

	// Set up the nsjail sandbox Ninja runs in.
	cmd.Sandbox = ninjaSandbox
	if config.HasKatiSuffix() {
		// Reads and executes a shell script from Kati that sets/unsets the
		// environment Ninja runs in.
		cmd.Environment.AppendFromKati(config.KatiEnvFile())
	}

	switch config.NinjaWeightListSource() {
	case NINJA_LOG:
		cmd.Args = append(cmd.Args, "-o", "usesninjalogasweightlist=yes")
	case EVENLY_DISTRIBUTED:
		// pass empty weight list means ninja considers every tasks's weight as 1(default value).
		cmd.Args = append(cmd.Args, "-o", "usesweightlist=/dev/null")
	case EXTERNAL_FILE:
		fallthrough
	case HINT_FROM_SOONG:
		// The weight list is already copied/generated.
		ninjaWeightListPath := filepath.Join(config.OutDir(), ninjaWeightListFileName)
		cmd.Args = append(cmd.Args, "-o", "usesweightlist="+ninjaWeightListPath)
	}

	// Allow both NINJA_ARGS and NINJA_EXTRA_ARGS, since both have been
	// used in the past to specify extra ninja arguments.
	if extra, ok := cmd.Environment.Get("NINJA_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extra)...)
	}
	if extra, ok := cmd.Environment.Get("NINJA_EXTRA_ARGS"); ok {
		cmd.Args = append(cmd.Args, strings.Fields(extra)...)
	}

	ninjaHeartbeatDuration := time.Minute * 5
	// Get the ninja heartbeat interval from the environment before it's filtered away later.
	if overrideText, ok := cmd.Environment.Get("NINJA_HEARTBEAT_INTERVAL"); ok {
		// For example, "1m"
		overrideDuration, err := time.ParseDuration(overrideText)
		if err == nil && overrideDuration.Seconds() > 0 {
			ninjaHeartbeatDuration = overrideDuration
		}
	}

	// Filter the environment, as ninja does not rebuild files when environment
	// variables change.
	//
	// Anything listed here must not change the output of rules/actions when the
	// value changes, otherwise incremental builds may be unsafe. Vars
	// explicitly set to stable values elsewhere in soong_ui are fine.
	//
	// For the majority of cases, either Soong or the makefiles should be
	// replicating any necessary environment variables in the command line of
	// each action that needs it.
	if cmd.Environment.IsEnvTrue("ALLOW_NINJA_ENV") {
		ctx.Println("Allowing all environment variables during ninja; incremental builds may be unsafe.")
	} else {
		cmd.Environment.Allow(append([]string{
			// Set the path to a symbolizer (e.g. llvm-symbolizer) so ASAN-based
			// tools can symbolize crashes.
			"ASAN_SYMBOLIZER_PATH",
			"HOME",
			"JAVA_HOME",
			"LANG",
			"LC_MESSAGES",
			"OUT_DIR",
			"PATH",
			"PWD",
			// https://docs.python.org/3/using/cmdline.html#envvar-PYTHONDONTWRITEBYTECODE
			"PYTHONDONTWRITEBYTECODE",
			"TMPDIR",
			"USER",

			// TODO: remove these carefully
			// Options for the address sanitizer.
			"ASAN_OPTIONS",
			// The list of Android app modules to be built in an unbundled manner.
			"TARGET_BUILD_APPS",
			// The variant of the product being built. e.g. eng, userdebug, debug.
			"TARGET_BUILD_VARIANT",
			// The product name of the product being built, e.g. aosp_arm, aosp_flame.
			"TARGET_PRODUCT",
			// b/147197813 - used by art-check-debug-apex-gen
			"EMMA_INSTRUMENT_FRAMEWORK",

			// RBE client
			"RBE_compare",
			"RBE_num_local_reruns",
			"RBE_num_remote_reruns",
			"RBE_exec_root",
			"RBE_exec_strategy",
			"RBE_invocation_id",
			"RBE_log_dir",
			"RBE_num_retries_if_mismatched",
			"RBE_platform",
			"RBE_remote_accept_cache",
			"RBE_remote_update_cache",
			"RBE_server_address",
			// TODO: remove old FLAG_ variables.
			"FLAG_compare",
			"FLAG_exec_root",
			"FLAG_exec_strategy",
			"FLAG_invocation_id",
			"FLAG_log_dir",
			"FLAG_platform",
			"FLAG_remote_accept_cache",
			"FLAG_remote_update_cache",
			"FLAG_server_address",

			// ccache settings
			"CCACHE_COMPILERCHECK",
			"CCACHE_SLOPPINESS",
			"CCACHE_BASEDIR",
			"CCACHE_CPP2",
			"CCACHE_DIR",

			// LLVM compiler wrapper options
			"TOOLCHAIN_RUSAGE_OUTPUT",

			// We don't want this build broken flag to cause reanalysis, so allow it through to the
			// actions.
			"BUILD_BROKEN_INCORRECT_PARTITION_IMAGES",
		}, config.BuildBrokenNinjaUsesEnvVars()...)...)
	}

	cmd.Environment.Set("DIST_DIR", config.DistDir())
	cmd.Environment.Set("SHELL", "/bin/bash")

	// Print the environment variables that Ninja is operating in.
	ctx.Verboseln("Ninja environment: ")
	envVars := cmd.Environment.Environ()
	sort.Strings(envVars)
	for _, envVar := range envVars {
		ctx.Verbosef("  %s", envVar)
	}

	// Write the env vars available during ninja execution to a file
	ninjaEnvVars := cmd.Environment.AsMap()
	data, err := shared.EnvFileContents(ninjaEnvVars)
	if err != nil {
		ctx.Panicf("Could not parse environment variables for ninja run %s", err)
	}
	// Write the file in every single run. This is fine because
	// 1. It is not a dep of Soong analysis, so will not retrigger Soong analysis.
	// 2. Is is fairly lightweight (~1Kb)
	ninjaEnvVarsFile := shared.JoinPath(config.SoongOutDir(), ninjaEnvFileName)
	err = os.WriteFile(ninjaEnvVarsFile, data, 0666)
	if err != nil {
		ctx.Panicf("Could not write ninja environment file %s", err)
	}

	// Poll the Ninja log for updates regularly based on the heartbeat
	// frequency. If it isn't updated enough, then we want to surface the
	// possibility that Ninja is stuck, to the user.
	done := make(chan struct{})
	defer close(done)
	ticker := time.NewTicker(ninjaHeartbeatDuration)
	defer ticker.Stop()
	ninjaChecker := &ninjaStucknessChecker{
		logPath: filepath.Join(config.OutDir(), ninjaLogFileName),
	}
	go func() {
		for {
			select {
			case <-ticker.C:
				ninjaChecker.check(ctx, config)
			case <-done:
				return
			}
		}
	}()

	ctx.Status.Status("Starting ninja...")
	cmd.RunAndStreamOrFatal()
}

// A simple struct for checking if Ninja gets stuck, using timestamps.
type ninjaStucknessChecker struct {
	logPath     string
	prevModTime time.Time
}

// Check that a file has been modified since the last time it was checked. If
// the mod time hasn't changed, then assume that Ninja got stuck, and print
// diagnostics for debugging.
func (c *ninjaStucknessChecker) check(ctx Context, config Config) {
	info, err := os.Stat(c.logPath)
	var newModTime time.Time
	if err == nil {
		newModTime = info.ModTime()
	}
	if newModTime == c.prevModTime {
		// The Ninja file hasn't been modified since the last time it was
		// checked, so Ninja could be stuck. Output some diagnostics.
		ctx.Verbosef("ninja may be stuck; last update to %v was %v. dumping process tree...", c.logPath, newModTime)

		// The "pstree" command doesn't exist on Mac, but "pstree" on Linux
		// gives more convenient output than "ps" So, we try pstree first, and
		// ps second
		commandText := fmt.Sprintf("pstree -pal %v || ps -ef", os.Getpid())

		cmd := Command(ctx, config, "dump process tree", "bash", "-c", commandText)
		output := cmd.CombinedOutputOrFatal()
		ctx.Verbose(string(output))

		ctx.Verbosef("done\n")
	}
	c.prevModTime = newModTime
}
