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
	"bufio"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"android/soong/ui/metrics"
	"android/soong/ui/status"
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

	common_args := []string{}
	common_args = append(common_args, config.NinjaArgs()...)
	common_args = append(common_args, "-f", config.CombinedNinjaFile())
	args := append(common_args, "-t", "targets", "rule")

	cmd := Command(ctx, config, "ninja", executable, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		ctx.Fatal(err)
	}

	cmd.StartOrFatal()

	outDir := config.OutDir()
	bootstrapDir := filepath.Join(outDir, "soong", ".bootstrap")
	miniBootstrapDir := filepath.Join(outDir, "soong", ".minibootstrap")

	danglingRules := make(map[string]bool)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, outDir) {
			// Leaf node is not in the out directory.
			continue
		}
		if strings.HasPrefix(line, bootstrapDir) || strings.HasPrefix(line, miniBootstrapDir) {
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

		report_lines := 1
		for i, dep := range danglingRulesList {
			if report_lines > 20 {
				fmt.Fprintf(sb, "  ... and %d more\n", len(danglingRulesList)-i)
				break
			}
			// It's helpful to see the reverse dependencies. ninja -t query is the
			// best tool we got for that. Its output starts with the dependency
			// itself.
			query_cmd := Command(ctx, config, "ninja", executable,
				append(common_args, "-t", "query", dep)...)
			query_stdout, err := query_cmd.StdoutPipe()
			if err != nil {
				ctx.Fatal(err)
			}
			query_cmd.StartOrFatal()
			scanner := bufio.NewScanner(query_stdout)
			for scanner.Scan() {
				report_lines++
				fmt.Fprintln(sb, " ", scanner.Text())
			}
			query_cmd.WaitOrFatal()
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
