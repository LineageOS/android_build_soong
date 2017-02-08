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
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"slices"
	"strconv"
	"strings"

	"android/soong/ui/metrics"
	"android/soong/ui/status"
)

type dumpvarsType int

const (
	_ dumpvarsType = iota
	DUMPVARS_PRE_CONFIG
	DUMPVARS_PRODUCT_CONFIG
	DUMPVARS_USER
)

// These are needed to configure Siso's RBE rules.
var sisoStringVars = []string{
	"JAVA_HOME",
	"OUT_DIR",
	"RBE_container_image",
	"RELEASE_BUILD_CLANG_VERSION",
}

// DumpMakeVars can be used to extract the values of Make variables after the
// product configurations are loaded. This is roughly equivalent to the
// `get_build_var` bash function.
//
// goals can be used to set MAKECMDGOALS, which emulates passing arguments to
// Make without actually building them. So all the variables based on
// MAKECMDGOALS can be read.
//
// vars is the list of variables to read. The values will be put in the
// returned map.
//
// variables controlled by soong_ui directly are now returned without needing
// to call into make, to retain compatibility.
func DumpMakeVars(ctx Context, config Config, goals, vars []string) (map[string]string, error) {
	soongUiVars := map[string]func() string{
		"OUT_DIR":     func() string { return config.OutDir() },
		"DIST_DIR":    func() string { return config.DistDir() },
		"TMPDIR":      func() string { return absPath(ctx, config.TempDir()) },
		"SOONG_NINJA": func() string { return config.ninjaCommand.String() },
		"SOONG_ONLY":  func() string { return strconv.FormatBool(config.soongOnlyRequested) },
	}

	makeVars := make([]string, 0, len(vars))
	for _, v := range vars {
		if _, ok := soongUiVars[v]; !ok {
			makeVars = append(makeVars, v)
		}
	}

	var ret map[string]string
	if len(makeVars) > 0 {
		// It's not safe to use the same TMPDIR as the build, as that can be removed.
		tmpDir, err := ioutil.TempDir("", "dumpvars")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmpDir)

		SetupLitePath(ctx, config, tmpDir)
		CollectEarlyReleaseConfig(ctx, config, QueryEarlyReleaseConfig(ctx, config))

		ret, err = dumpMakeVars(ctx, config, goals, makeVars, tmpDir, DUMPVARS_USER)
		if err != nil {
			return ret, err
		}
	} else {
		ret = make(map[string]string)
	}

	for _, v := range vars {
		if f, ok := soongUiVars[v]; ok {
			ret[v] = f()
		}
	}

	return ret, nil
}

func dumpMakeVars(ctx Context, config Config, goals, vars []string, tmpDir string, which dumpvarsType) (map[string]string, error) {
	e := ctx.BeginTrace(metrics.RunKati, "dumpvars")
	defer e.End()

	tool := ctx.Status.StartTool()
	if which == DUMPVARS_PRODUCT_CONFIG {
		// Only print this during product config.
		tool.Status("Running product configuration...")
	}
	defer tool.Finish()

	cmd := Command(ctx, config, e, "dumpvars",
		config.KatiBin(),
		"-f", "build/make/core/config.mk",
		"--color_warnings",
		"--kati_stats",
		"dump-many-vars",
		"MAKECMDGOALS="+strings.Join(goals, " "))
	switch which {
	case DUMPVARS_PRE_CONFIG:
		cmd.Environment.Set("CALLED_PRE_PRODUCT_CONFIG", "true")
	case DUMPVARS_PRODUCT_CONFIG:
		cmd.Environment.Set("WRITE_SOONG_VARIABLES", "true")
	default:
	}
	cmd.Environment.Set("CALLED_FROM_SETUP", "true")
	cmd.Environment.Set("DUMP_MANY_VARS", strings.Join(vars, " "))
	if tmpDir != "" {
		cmd.Environment.Set("TMPDIR", tmpDir)
	}
	cmd.Sandbox = dumpvarsSandbox
	output := bytes.Buffer{}
	cmd.Stdout = &output
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("Error getting output pipe for kati: %s", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// TODO: error out when Stderr contains any content
	status.KatiReader(tool, pipe)
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	ret := make(map[string]string, len(vars))
	for _, line := range strings.Split(output.String(), "\n") {
		if len(line) == 0 {
			continue
		}

		if key, value, ok := decodeKeyValue(line); ok {
			if value, ok = singleUnquote(value); ok {
				ret[key] = value
				ctx.Verboseln(key, value)
			} else {
				return nil, fmt.Errorf("Failed to parse make line: %q", line)
			}
		} else {
			return nil, fmt.Errorf("Failed to parse make line: %q", line)
		}
	}
	if ctx.Metrics != nil {
		// Also include TARGET_RELEASE in the metrics.  Do this first
		// so that it gets overwritten if dumpvars ever spits it out.
		if release, found := os.LookupEnv("TARGET_RELEASE"); found {
			ctx.Metrics.SetMetadataMetrics(
				map[string]string{"TARGET_RELEASE": release})
		}
		ctx.Metrics.SetMetadataMetrics(ret)
	}

	return ret, nil
}

// Variables to print out in the top banner
var BannerVars = []string{
	"PLATFORM_VERSION_CODENAME",
	"PLATFORM_VERSION",
	"LINEAGE_VERSION",
	"PRODUCT_SOURCE_ROOT_DIRS",
	"TARGET_PRODUCT",
	"TARGET_BUILD_VARIANT",
	"TARGET_BUILD_APPS",
	"TARGET_BUILD_UNBUNDLED",
	"TARGET_ARCH",
	"TARGET_ARCH_VARIANT",
	"TARGET_CPU_VARIANT",
	"TARGET_2ND_ARCH",
	"TARGET_2ND_ARCH_VARIANT",
	"TARGET_2ND_CPU_VARIANT",
	"HOST_OS",
	"HOST_OS_EXTRA",
	"HOST_CROSS_OS",
	"BUILD_ID",
	"OUT_DIR",
	"SOONG_SDK_SNAPSHOT_TARGET_BUILD_RELEASE",
	"WITH_SU",
}

func Banner(config Config, make_vars map[string]string) string {
	b := &bytes.Buffer{}

	fmt.Fprintln(b, "============================================")
	for _, name := range BannerVars {
		if make_vars[name] != "" {
			fmt.Fprintf(b, "%s=%s\n", name, make_vars[name])
		}
	}

	if use, _ := config.environ.Get("SOONG_USE_PARTIAL_COMPILE"); use == "true" {
		if partialCompile, ok := config.environ.Get("SOONG_PARTIAL_COMPILE"); ok {
			fmt.Fprintf(b, "SOONG_PARTIAL_COMPILE=%s\n", partialCompile)
		}
	}

	// Only show USE_RBE and USE_REWRAPPER when the user has explicitly set SOONG_NINJA
	if config.ninjaCommand != NINJA_DEFAULT {
		fmt.Fprintf(b, "SOONG_NINJA=%s\n", config.ninjaCommand.String())
	}
	if config.UseRBE() {
		fmt.Fprintf(b, "USE_RBE=%t\n", config.UseRBE())
		if config.ninjaCommand == NINJA_SISO {
			fmt.Fprintf(b, "USE_REWRAPPER=%t\n", config.UseRewrapper())
		}
	}

	// Normally config.soongOnlyRequested already takes into account PRODUCT_SOONG_ONLY,
	// except when doing `get_build_var report_config`, which is run during envsetup.
	if config.skipKatiControlledByFlags {
		fmt.Fprintf(b, "SOONG_ONLY=%t\n", config.soongOnlyRequested)
	} else { // default for this product
		fmt.Fprintf(b, "SOONG_ONLY=%t\n", make_vars["PRODUCT_SOONG_ONLY"] == "true")
	}

	fmt.Fprintf(b, "SOONG_INCREMENTAL_ANALYSIS=%t\n", config.incrementalBuildActions)

	if len(config.partialAnalysisTargets) > 0 {
		fmt.Fprintf(b, "SOONG_PARTIAL_ANALYSIS=%s\n", config.partialAnalysisTargets)
	}

	fmt.Fprint(b, "============================================")

	return b.String()
}

func runMakeProductConfig(ctx Context, config Config) {
	// Variables to export into the environment of Kati/Ninja
	exportEnvVars := []string{
		// So that we can use the correct TARGET_PRODUCT if it's been
		// modified by a buildspec.mk
		"TARGET_PRODUCT",
		"TARGET_BUILD_VARIANT",
		"TARGET_BUILD_APPS",
		"TARGET_BUILD_UNBUNDLED",

		// Additional release config maps
		"PRODUCT_RELEASE_CONFIG_MAPS",

		// compiler wrappers set up by make
		"CC_WRAPPER",
		"CXX_WRAPPER",
		"RBE_WRAPPER",
		"JAVAC_WRAPPER",
		"R8_WRAPPER",
		"D8_WRAPPER",

		// ccache settings
		"CCACHE_COMPILERCHECK",
		"CCACHE_SLOPPINESS",
		"CCACHE_BASEDIR",
		"CCACHE_CPP2",

		// LLVM compiler wrapper options
		"TOOLCHAIN_RUSAGE_OUTPUT",
	}

	allVars := slices.Concat([]string{
		// Used to execute Kati and Ninja
		"NINJA_GOALS",
		"KATI_GOALS",

		// To find target/product/<DEVICE>
		"TARGET_DEVICE",

		// So that later Kati runs can find BoardConfig.mk faster
		"TARGET_DEVICE_DIR",

		// Whether --werror_overriding_commands will work
		"BUILD_BROKEN_DUP_RULES",

		// Whether to enable the network during the build
		"BUILD_BROKEN_USES_NETWORK",

		// Extra environment variables to be exported to ninja
		"BUILD_BROKEN_NINJA_USES_ENV_VARS",

		// Used to restrict write access to source tree
		"BUILD_BROKEN_SRC_DIR_IS_WRITABLE",
		"BUILD_BROKEN_SRC_DIR_RW_ALLOWLIST",

		// Whether missing outputs should be treated as warnings
		// instead of errors.
		// `true` will relegate missing outputs to warnings.
		"BUILD_BROKEN_MISSING_OUTPUTS",

		"PRODUCT_SOONG_ONLY",
		"PRODUCT_SOONG_INCREMENTAL_ANALYSIS",

		// Not used, but useful to be in the soong.log
		"TARGET_BUILD_TYPE",
		"HOST_ARCH",
		"HOST_2ND_ARCH",
		"HOST_CROSS_ARCH",
		"HOST_CROSS_2ND_ARCH",
		"HOST_BUILD_TYPE",
		"PRODUCT_SOONG_NAMESPACES",

		"DEFAULT_WARNING_BUILD_MODULE_TYPES",
		"DEFAULT_ERROR_BUILD_MODULE_TYPES",
		"BUILD_BROKEN_PREBUILT_ELF_FILES",
		"BUILD_BROKEN_TREBLE_SYSPROP_NEVERALLOW",
		"BUILD_BROKEN_USES_BUILD_COPY_HEADERS",
		"BUILD_BROKEN_USES_BUILD_EXECUTABLE",
		"BUILD_BROKEN_USES_BUILD_FUZZ_TEST",
		"BUILD_BROKEN_USES_BUILD_HEADER_LIBRARY",
		"BUILD_BROKEN_USES_BUILD_HOST_EXECUTABLE",
		"BUILD_BROKEN_USES_BUILD_HOST_JAVA_LIBRARY",
		"BUILD_BROKEN_USES_BUILD_HOST_PREBUILT",
		"BUILD_BROKEN_USES_BUILD_HOST_SHARED_LIBRARY",
		"BUILD_BROKEN_USES_BUILD_HOST_STATIC_LIBRARY",
		"BUILD_BROKEN_USES_BUILD_JAVA_LIBRARY",
		"BUILD_BROKEN_USES_BUILD_MULTI_PREBUILT",
		"BUILD_BROKEN_USES_BUILD_NATIVE_TEST",
		"BUILD_BROKEN_USES_BUILD_NOTICE_FILE",
		"BUILD_BROKEN_USES_BUILD_PACKAGE",
		"BUILD_BROKEN_USES_BUILD_PHONY_PACKAGE",
		"BUILD_BROKEN_USES_BUILD_PREBUILT",
		"BUILD_BROKEN_USES_BUILD_RRO_PACKAGE",
		"BUILD_BROKEN_USES_BUILD_SHARED_LIBRARY",
		"BUILD_BROKEN_USES_BUILD_STATIC_JAVA_LIBRARY",
		"BUILD_BROKEN_USES_BUILD_STATIC_LIBRARY",
		"RELEASE_BUILD_EXECUTION_METRICS",
		"RELEASE_USE_RKATI",
		"RELEASE_BUILD_WITH_JDK_25",
		"RELEASE_SOONG_INCREMENTAL_ANALYSIS",
		"SOONG_INCREMENTAL_ANALYSIS",
	}, exportEnvVars, BannerVars, sisoStringVars, earlyReleaseConfigVars)

	makeVars, err := dumpMakeVars(ctx, config, config.Arguments(), allVars, "", DUMPVARS_PRODUCT_CONFIG)
	if err != nil {
		ctx.Fatalln("Error dumping make vars:", err)
	}

	// Verify that none of the earlyVars changed values.
	for _, name := range earlyReleaseConfigVars {
		if config.earlyVars[name] != makeVars[name] {
			ctx.Fatalf("Product config value for %q is not consistent: %q (early call) became %q (final call).\n",
				name, config.earlyVars[name], makeVars[name])

		}
	}

	// Populate the environment
	env := config.Environment()
	for _, name := range exportEnvVars {
		if makeVars[name] == "" {
			env.Unset(name)
		} else {
			env.Set(name, makeVars[name])
		}
	}

	config.useJdk25 = makeVars["RELEASE_BUILD_WITH_JDK_25"] == "true"
	if config.useJdk25 {
		ConfigJavaEnvironment(ctx, config.configImpl)
	}

	config.SetKatiArgs(strings.Fields(makeVars["KATI_GOALS"]))
	config.SetNinjaArgs(strings.Fields(makeVars["NINJA_GOALS"]))
	config.SetTargetDevice(makeVars["TARGET_DEVICE"])
	config.SetTargetDeviceDir(makeVars["TARGET_DEVICE_DIR"])
	config.useRkati = makeVars["RELEASE_USE_RKATI"] == "true" || os.Getenv("SOONG_USE_RKATI") == "true"
	config.setupSandboxConfig(ctx, makeVars)

	config.updateSisoConfigVars(makeVars)
	config.SetBuildBrokenDupRules(makeVars["BUILD_BROKEN_DUP_RULES"] == "true")
	config.SetBuildBrokenUsesNetwork(makeVars["BUILD_BROKEN_USES_NETWORK"] == "true")
	config.SetBuildBrokenNinjaUsesEnvVars(strings.Fields(makeVars["BUILD_BROKEN_NINJA_USES_ENV_VARS"]))
	config.SetSourceRootDirs(strings.Fields(makeVars["PRODUCT_SOURCE_ROOT_DIRS"]))
	config.SetBuildBrokenMissingOutputs(makeVars["BUILD_BROKEN_MISSING_OUTPUTS"] == "true")

	if !config.skipKatiControlledByFlags {
		if makeVars["PRODUCT_SOONG_ONLY"] == "true" {
			config.soongOnlyRequested = true
			config.skipKati = true
			config.skipKatiNinja = true
		}
	}

	ctx.Metrics.SetSoongOnly(config.soongOnlyRequested)

	// Enable incremental analysis by default if requested by the build flag, unless it
	// was set explicitly in the environment.
	if !config.incrementalBuildActionsSetInEnv && makeVars["RELEASE_SOONG_INCREMENTAL_ANALYSIS"] == "true" {
		config.incrementalBuildActions = true
	}

	// Print the banner like make did
	if !env.IsEnvTrue("ANDROID_QUIET_BUILD") {
		fmt.Fprintln(ctx.Writer, Banner(config, makeVars))
	}
}
