// Copyright 2019 Google Inc. All rights reserved.
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
	"runtime"
	"strings"

	"android/soong/remoteexec"
	"android/soong/ui/metrics"
)

const (
	rbeLeastNProcs = 2500
	rbeLeastNFiles = 16000

	// prebuilt RBE binaries
	bootstrapCmd = "bootstrap"

	// RBE metrics proto buffer file
	rbeMetricsPBFilename = "rbe_metrics.pb"

	defaultOutDir = "out"
)

func rbeCommand(ctx Context, config Config, rbeCmd string) string {
	var cmdPath string
	if rbeDir := config.rbeDir(); rbeDir != "" {
		cmdPath = filepath.Join(rbeDir, rbeCmd)
	} else {
		ctx.Fatalf("rbe command path not found")
	}

	if _, err := os.Stat(cmdPath); err != nil && os.IsNotExist(err) {
		ctx.Fatalf("rbe command %q not found", rbeCmd)
	}

	return cmdPath
}

func getRBEVars(ctx Context, config Config) map[string]string {
	vars := map[string]string{
		"RBE_log_dir":          config.rbeProxyLogsDir(),
		"RBE_re_proxy":         config.rbeReproxy(),
		"RBE_exec_root":        config.rbeExecRoot(),
		"RBE_output_dir":       config.rbeProxyLogsDir(),
		"RBE_proxy_log_dir":    config.rbeProxyLogsDir(),
		"RBE_cache_dir":        config.rbeCacheDir(),
		"RBE_download_tmp_dir": config.rbeDownloadTmpDir(),
		"RBE_platform":         "container-image=" + remoteexec.DefaultImage,
	}
	if config.StartRBE() {
		name, err := config.rbeSockAddr(absPath(ctx, config.TempDir()))
		if err != nil {
			ctx.Fatalf("Error retrieving socket address: %v", err)
			return nil
		}
		vars["RBE_server_address"] = fmt.Sprintf("unix://%v", name)
	}

	rf := 1.0
	if config.Parallel() < runtime.NumCPU() {
		rf = float64(config.Parallel()) / float64(runtime.NumCPU())
	}
	vars["RBE_local_resource_fraction"] = fmt.Sprintf("%.2f", rf)

	k, v := config.rbeAuth()
	vars[k] = v
	return vars
}

func cleanupRBELogsDir(ctx Context, config Config) {
	if !config.shouldCleanupRBELogsDir() {
		return
	}

	rbeTmpDir := config.rbeProxyLogsDir()
	if err := os.RemoveAll(rbeTmpDir); err != nil {
		fmt.Fprintln(ctx.Writer, "\033[33mUnable to remove RBE log directory: ", err, "\033[0m")
	}
}

func checkRBERequirements(ctx Context, config Config) {
	if !config.GoogleProdCredsExist() && prodCredsAuthType(config) {
		ctx.Fatalf("Unable to start RBE reproxy\nFAILED: Missing LOAS credentials.")
	}

	if u := ulimitOrFatal(ctx, config, "-u"); u < rbeLeastNProcs {
		ctx.Fatalf("max user processes is insufficient: %d; want >= %d.\n", u, rbeLeastNProcs)
	}
	if n := ulimitOrFatal(ctx, config, "-n"); n < rbeLeastNFiles {
		ctx.Fatalf("max open files is insufficient: %d; want >= %d.\n", n, rbeLeastNFiles)
	}
	if _, err := os.Stat(config.rbeProxyLogsDir()); os.IsNotExist(err) {
		if err := os.MkdirAll(config.rbeProxyLogsDir(), 0744); err != nil {
			ctx.Fatalf("Unable to create logs dir (%v) for RBE: %v", config.rbeProxyLogsDir, err)
		}
	}
}

func startRBE(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunSetupTool, "rbe_bootstrap")
	defer ctx.EndTrace()

	ctx.Status.Status("Starting rbe...")

	cmd := Command(ctx, config, "startRBE bootstrap", rbeCommand(ctx, config, bootstrapCmd))

	if output, err := cmd.CombinedOutput(); err != nil {
		ctx.Fatalf("Unable to start RBE reproxy\nFAILED: RBE bootstrap failed with: %v\n%s\n", err, output)
	}
}

func stopRBE(ctx Context, config Config) {
	cmd := Command(ctx, config, "stopRBE bootstrap", rbeCommand(ctx, config, bootstrapCmd), "-shutdown")
	output, err := cmd.CombinedOutput()
	if err != nil {
		ctx.Fatalf("rbe bootstrap with shutdown failed with: %v\n%s\n", err, output)
	}

	if !config.Environment().IsEnvTrue("ANDROID_QUIET_BUILD") && len(output) > 0 {
		fmt.Fprintln(ctx.Writer, "")
		fmt.Fprintln(ctx.Writer, fmt.Sprintf("%s", output))
	}
}

func prodCredsAuthType(config Config) bool {
	authVar, val := config.rbeAuth()
	if strings.Contains(authVar, "use_google_prod_creds") && val != "" && val != "false" {
		return true
	}
	return false
}

// Check whether proper auth exists for RBE builds run within a
// Google dev environment.
func CheckProdCreds(ctx Context, config Config) {
	if !config.IsGooglerEnvironment() {
		return
	}
	if !config.StubbyExists() && prodCredsAuthType(config) {
		fmt.Fprintln(ctx.Writer, "")
		fmt.Fprintln(ctx.Writer, fmt.Sprintf("\033[33mWARNING: %q binary not found in $PATH, follow go/build-fast-without-stubby instead for authenticating with RBE.\033[0m", "stubby"))
		fmt.Fprintln(ctx.Writer, "")
		return
	}
	if config.GoogleProdCredsExist() {
		return
	}
	fmt.Fprintln(ctx.Writer, "")
	fmt.Fprintln(ctx.Writer, "\033[33mWARNING: Missing LOAS credentials, please run `gcert`. This will result in failing builds in the future, see go/rbe-android-default-announcement.\033[0m")
	fmt.Fprintln(ctx.Writer, "")
}

// DumpRBEMetrics creates a metrics protobuf file containing RBE related metrics.
// The protobuf file is created if RBE is enabled and the proxy service has
// started. The proxy service is shutdown in order to dump the RBE metrics to the
// protobuf file.
func DumpRBEMetrics(ctx Context, config Config, filename string) {
	ctx.BeginTrace(metrics.RunShutdownTool, "dump_rbe_metrics")
	defer ctx.EndTrace()

	// Remove the previous metrics file in case there is a failure or RBE has been
	// disable for this run.
	os.Remove(filename)

	// If RBE is not enabled then there are no metrics to generate.
	// If RBE does not require to start, the RBE proxy maybe started
	// manually for debugging purpose and can generate the metrics
	// afterwards.
	if !config.StartRBE() {
		return
	}

	outputDir := config.rbeProxyLogsDir()
	if outputDir == "" {
		ctx.Fatal("RBE output dir variable not defined. Aborting metrics dumping.")
	}
	metricsFile := filepath.Join(outputDir, rbeMetricsPBFilename)

	// Stop the proxy first in order to generate the RBE metrics protobuf file.
	stopRBE(ctx, config)

	if metricsFile == filename {
		return
	}
	if _, err := copyFile(metricsFile, filename); err != nil {
		ctx.Fatalf("failed to copy %q to %q: %v\n", metricsFile, filename, err)
	}
}

// PrintOutDirWarning prints a warning to indicate to the user that
// setting output directory to a path other than "out" in an RBE enabled
// build can cause slow builds.
func PrintOutDirWarning(ctx Context, config Config) {
	if config.UseRBE() && config.OutDir() != defaultOutDir {
		fmt.Fprintln(ctx.Writer, "")
		fmt.Fprintln(ctx.Writer, "\033[33mWARNING:\033[0m")
		fmt.Fprintln(ctx.Writer, fmt.Sprintf("Setting OUT_DIR to a path other than %v may result in slow RBE builds.", defaultOutDir))
		fmt.Fprintln(ctx.Writer, "See http://go/android_rbe_out_dir for a workaround.")
		fmt.Fprintln(ctx.Writer, "")
	}
}
