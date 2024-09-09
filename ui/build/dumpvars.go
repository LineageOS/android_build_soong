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
	"strings"

	"android/soong/ui/metrics"
	"android/soong/ui/status"
)

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
		"OUT_DIR":  func() string { return config.OutDir() },
		"DIST_DIR": func() string { return config.DistDir() },
		"TMPDIR":   func() string { return absPath(ctx, config.TempDir()) },
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

		ret, err = dumpMakeVars(ctx, config, goals, makeVars, false, tmpDir)
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

func dumpMakeVars(ctx Context, config Config, goals, vars []string, write_soong_vars bool, tmpDir string) (map[string]string, error) {
	ctx.BeginTrace(metrics.RunKati, "dumpvars")
	defer ctx.EndTrace()

	tool := ctx.Status.StartTool()
	if write_soong_vars {
		// only print this when write_soong_vars is true so that it's not printed when using
		// the get_build_var command.
		tool.Status("Running product configuration...")
	}
	defer tool.Finish()

	cmd := Command(ctx, config, "dumpvars",
		config.PrebuiltBuildTool("ckati"),
		"-f", "build/make/core/config.mk",
		"--color_warnings",
		"--kati_stats",
		"dump-many-vars",
		"MAKECMDGOALS="+strings.Join(goals, " "))
	cmd.Environment.Set("CALLED_FROM_SETUP", "true")
	if write_soong_vars {
		cmd.Environment.Set("WRITE_SOONG_VARIABLES", "true")
	}
	cmd.Environment.Set("DUMP_MANY_VARS", strings.Join(vars, " "))
	if tmpDir != "" {
		cmd.Environment.Set("TMPDIR", tmpDir)
	}
	cmd.Sandbox = dumpvarsSandbox
	output := bytes.Buffer{}
	cmd.Stdout = &output
	pipe, err := cmd.StderrPipe()
	if err != nil {
		ctx.Fatalln("Error getting output pipe for ckati:", err)
	}
	cmd.StartOrFatal()
	// TODO: error out when Stderr contains any content
	status.KatiReader(tool, pipe)
	cmd.WaitOrFatal()

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
	"WITH_GMS",
	"WITH_GMS_CAR",
	"WITH_GMS_TV",
	"GMS_MAKEFILE",
	"MAINLINE_MODULES_MAKEFILE",
}

func Banner(make_vars map[string]string) string {
	b := &bytes.Buffer{}

	fmt.Fprintln(b, "============================================")
	for _, name := range BannerVars {
		if make_vars[name] != "" {
			fmt.Fprintf(b, "%s=%s\n", name, make_vars[name])
		}
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

	allVars := append(append([]string{
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
	}, exportEnvVars...), BannerVars...)

	makeVars, err := dumpMakeVars(ctx, config, config.Arguments(), allVars, true, "")
	if err != nil {
		ctx.Fatalln("Error dumping make vars:", err)
	}

	env := config.Environment()
	// Print the banner like make does
	if !env.IsEnvTrue("ANDROID_QUIET_BUILD") {
		fmt.Fprintln(ctx.Writer, Banner(makeVars))
	}

	// Populate the environment
	for _, name := range exportEnvVars {
		if makeVars[name] == "" {
			env.Unset(name)
		} else {
			env.Set(name, makeVars[name])
		}
	}

	config.SetKatiArgs(strings.Fields(makeVars["KATI_GOALS"]))
	config.SetNinjaArgs(strings.Fields(makeVars["NINJA_GOALS"]))
	config.SetTargetDevice(makeVars["TARGET_DEVICE"])
	config.SetTargetDeviceDir(makeVars["TARGET_DEVICE_DIR"])
	config.sandboxConfig.SetSrcDirIsRO(makeVars["BUILD_BROKEN_SRC_DIR_IS_WRITABLE"] == "false")
	config.sandboxConfig.SetSrcDirRWAllowlist(strings.Fields(makeVars["BUILD_BROKEN_SRC_DIR_RW_ALLOWLIST"]))

	config.SetBuildBrokenDupRules(makeVars["BUILD_BROKEN_DUP_RULES"] == "true")
	config.SetBuildBrokenUsesNetwork(makeVars["BUILD_BROKEN_USES_NETWORK"] == "true")
	config.SetBuildBrokenNinjaUsesEnvVars(strings.Fields(makeVars["BUILD_BROKEN_NINJA_USES_ENV_VARS"]))
	config.SetSourceRootDirs(strings.Fields(makeVars["PRODUCT_SOURCE_ROOT_DIRS"]))
}
