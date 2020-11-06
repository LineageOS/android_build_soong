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

package android

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/blueprint"
)

// The Bazel QueryView singleton is responsible for generating the Ninja actions
// for calling the soong_build primary builder in the main build.ninja file.
func init() {
	RegisterSingletonType("bazel_queryView", BazelQueryViewSingleton)
}

func BazelQueryViewSingleton() Singleton {
	return &bazelQueryViewSingleton{}
}

type bazelQueryViewSingleton struct{}

func (c *bazelQueryViewSingleton) GenerateBuildActions(ctx SingletonContext) {
	// Create a build and rule statement, using the Bazel QueryView's WORKSPACE
	// file as the output file marker.
	var deps Paths
	moduleListFilePath := pathForBuildToolDep(ctx, ctx.Config().moduleListFile)
	deps = append(deps, moduleListFilePath)
	deps = append(deps, pathForBuildToolDep(ctx, ctx.Config().ProductVariablesFileName))

	bazelQueryViewDirectory := PathForOutput(ctx, "queryview")
	bazelQueryViewWorkspaceFile := bazelQueryViewDirectory.Join(ctx, "WORKSPACE")
	primaryBuilder := primaryBuilderPath(ctx)
	bazelQueryView := ctx.Rule(pctx, "bazelQueryView",
		blueprint.RuleParams{
			Command: fmt.Sprintf(
				"rm -rf ${outDir}/* && %s --bazel_queryview_dir ${outDir} %s && echo WORKSPACE: `cat %s` > ${outDir}/.queryview-depfile.d",
				primaryBuilder.String(),
				strings.Join(os.Args[1:], " "),
				moduleListFilePath.String(), // Use the contents of Android.bp.list as the depfile.
			),
			CommandDeps: []string{primaryBuilder.String()},
			Description: fmt.Sprintf(
				"Creating the Bazel QueryView workspace with %s at $outDir",
				primaryBuilder.Base()),
			Deps:    blueprint.DepsGCC,
			Depfile: "${outDir}/.queryview-depfile.d",
		},
		"outDir")

	ctx.Build(pctx, BuildParams{
		Rule:   bazelQueryView,
		Output: bazelQueryViewWorkspaceFile,
		Inputs: deps,
		Args: map[string]string{
			"outDir": bazelQueryViewDirectory.String(),
		},
	})

	// Add a phony target for building the Bazel QueryView
	ctx.Phony("queryview", bazelQueryViewWorkspaceFile)
}
