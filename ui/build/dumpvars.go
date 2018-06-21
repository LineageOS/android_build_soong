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
	"strings"
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
func DumpMakeVars(ctx Context, config Config, goals, vars []string) (map[string]string, error) {
	return dumpMakeVars(ctx, config, goals, vars, false)
}

func dumpMakeVars(ctx Context, config Config, goals, vars []string, write_soong_vars bool) (map[string]string, error) {
	ctx.BeginTrace("dumpvars")
	defer ctx.EndTrace()

	cmd := Command(ctx, config, "dumpvars",
		config.PrebuiltBuildTool("ckati"),
		"-f", "build/make/core/config.mk",
		"--color_warnings",
		"--kati_stats",
		"dump-many-vars",
		"MAKECMDGOALS="+strings.Join(goals, " "))
	cmd.Environment.Set("CALLED_FROM_SETUP", "true")
	cmd.Environment.Set("BUILD_SYSTEM", "build/make/core")
	if write_soong_vars {
		cmd.Environment.Set("WRITE_SOONG_VARIABLES", "true")
	}
	cmd.Environment.Set("DUMP_MANY_VARS", strings.Join(vars, " "))
	cmd.Sandbox = dumpvarsSandbox
	output := bytes.Buffer{}
	cmd.Stdout = &output
	pipe, err := cmd.StderrPipe()
	if err != nil {
		ctx.Fatalln("Error getting output pipe for ckati:", err)
	}
	cmd.StartOrFatal()
	// TODO: error out when Stderr contains any content
	katiRewriteOutput(ctx, pipe)
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

	return ret, nil
}

// Variables to print out in the top banner
var BannerVars = []string{
	"PLATFORM_VERSION_CODENAME",
	"PLATFORM_VERSION",
	"TARGET_PRODUCT",
	"TARGET_BUILD_VARIANT",
	"TARGET_BUILD_TYPE",
	"TARGET_BUILD_APPS",
	"TARGET_ARCH",
	"TARGET_ARCH_VARIANT",
	"TARGET_CPU_VARIANT",
	"TARGET_2ND_ARCH",
	"TARGET_2ND_ARCH_VARIANT",
	"TARGET_2ND_CPU_VARIANT",
	"HOST_ARCH",
	"HOST_2ND_ARCH",
	"HOST_OS",
	"HOST_OS_EXTRA",
	"HOST_CROSS_OS",
	"HOST_CROSS_ARCH",
	"HOST_CROSS_2ND_ARCH",
	"HOST_BUILD_TYPE",
	"BUILD_ID",
	"OUT_DIR",
	"AUX_OS_VARIANT_LIST",
	"TARGET_BUILD_PDK",
	"PDK_FUSION_PLATFORM_ZIP",
	"PRODUCT_SOONG_NAMESPACES",
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
		// modified by PRODUCT-*/APP-* arguments
		"TARGET_PRODUCT",
		"TARGET_BUILD_VARIANT",
		"TARGET_BUILD_APPS",

		// compiler wrappers set up by make
		"CC_WRAPPER",
		"CXX_WRAPPER",
		"JAVAC_WRAPPER",

		// ccache settings
		"CCACHE_COMPILERCHECK",
		"CCACHE_SLOPPINESS",
		"CCACHE_BASEDIR",
		"CCACHE_CPP2",
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
	}, exportEnvVars...), BannerVars...)

	make_vars, err := dumpMakeVars(ctx, config, config.Arguments(), allVars, true)
	if err != nil {
		ctx.Fatalln("Error dumping make vars:", err)
	}

	// Print the banner like make does
	fmt.Fprintln(ctx.Stdout(), Banner(make_vars))

	// Populate the environment
	env := config.Environment()
	for _, name := range exportEnvVars {
		if make_vars[name] == "" {
			env.Unset(name)
		} else {
			env.Set(name, make_vars[name])
		}
	}

	config.SetKatiArgs(strings.Fields(make_vars["KATI_GOALS"]))
	config.SetNinjaArgs(strings.Fields(make_vars["NINJA_GOALS"]))
	config.SetTargetDevice(make_vars["TARGET_DEVICE"])
	config.SetTargetDeviceDir(make_vars["TARGET_DEVICE_DIR"])

	config.SetPdkBuild(make_vars["TARGET_BUILD_PDK"] == "true")
	config.SetBuildBrokenDupRules(make_vars["BUILD_BROKEN_DUP_RULES"] != "false")
}
