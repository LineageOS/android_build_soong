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
	"android/soong/ui/metrics"
	"android/soong/ui/status"
	"bufio"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Checks for files in the out directory that have a rule that depends on them but no rule to
// create them. This catches a common set of build failures where a rule to generate a file is
// deleted (either by deleting a module in an Android.mk file, or by modifying the build system
// incorrectly).  These failures are often not caught by a local incremental build because the
// previously built files are still present in the output directory.
func testForDanglingRules(ctx Context, config Config) {
	// Many modules are disabled on mac.  Checking for dangling rules would cause lots of build
	// breakages, and presubmit wouldn't catch them, so just disable the check.
	if runtime.GOOS != "linux" {
		return
	}

	ctx.BeginTrace(metrics.TestRun, "test for dangling rules")
	defer ctx.EndTrace()

	ts := ctx.Status.StartTool()
	action := &status.Action{
		Description: "Test for dangling rules",
	}
	ts.StartAction(action)

	// Get a list of leaf nodes in the dependency graph from ninja
	executable := config.PrebuiltBuildTool("ninja")

	commonArgs := []string{}
	commonArgs = append(commonArgs, "-f", config.CombinedNinjaFile())
	args := append(commonArgs, "-t", "targets", "rule")

	cmd := Command(ctx, config, "ninja", executable, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		ctx.Fatal(err)
	}

	cmd.StartOrFatal()

	outDir := config.OutDir()
	modulePathsDir := filepath.Join(outDir, ".module_paths")
	variablesFilePath := filepath.Join(outDir, "soong", "soong.variables")

	// dexpreopt.config is an input to the soong_docs action, which runs the
	// soong_build primary builder. However, this file is created from $(shell)
	// invocation at Kati parse time, so it's not an explicit output of any
	// Ninja action, but it is present during the build itself and can be
	// treated as an source file.
	dexpreoptConfigFilePath := filepath.Join(outDir, "soong", "dexpreopt.config")

	// out/build_date.txt is considered a "source file"
	buildDatetimeFilePath := filepath.Join(outDir, "build_date.txt")

	// bpglob is built explicitly using Microfactory
	bpglob := filepath.Join(config.SoongOutDir(), "bpglob")

	danglingRules := make(map[string]bool)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, outDir) {
			// Leaf node is not in the out directory.
			continue
		}
		if strings.HasPrefix(line, modulePathsDir) ||
			line == variablesFilePath ||
			line == dexpreoptConfigFilePath ||
			line == buildDatetimeFilePath ||
			line == bpglob {
			// Leaf node is in one of Soong's bootstrap directories, which do not have
			// full build rules in the primary build.ninja file.
			continue
		}

		danglingRules[line] = true
	}

	cmd.WaitOrFatal()

	var danglingRulesList []string
	for rule := range danglingRules {
		danglingRulesList = append(danglingRulesList, rule)
	}
	sort.Strings(danglingRulesList)

	if len(danglingRulesList) > 0 {
		sb := &strings.Builder{}
		title := "Dependencies in out found with no rule to create them:"
		fmt.Fprintln(sb, title)

		reportLines := 1
		for i, dep := range danglingRulesList {
			if reportLines > 20 {
				fmt.Fprintf(sb, "  ... and %d more\n", len(danglingRulesList)-i)
				break
			}
			// It's helpful to see the reverse dependencies. ninja -t query is the
			// best tool we got for that. Its output starts with the dependency
			// itself.
			queryCmd := Command(ctx, config, "ninja", executable,
				append(commonArgs, "-t", "query", dep)...)
			queryStdout, err := queryCmd.StdoutPipe()
			if err != nil {
				ctx.Fatal(err)
			}
			queryCmd.StartOrFatal()
			scanner := bufio.NewScanner(queryStdout)
			for scanner.Scan() {
				reportLines++
				fmt.Fprintln(sb, " ", scanner.Text())
			}
			queryCmd.WaitOrFatal()
		}

		ts.FinishAction(status.ActionResult{
			Action: action,
			Error:  fmt.Errorf(title),
			Output: sb.String(),
		})
		ctx.Fatal("stopping")
	}
	ts.FinishAction(status.ActionResult{Action: action})
}
