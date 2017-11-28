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

package java

// Rules for instrumenting classes using jacoco

import (
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

var (
	jacoco = pctx.AndroidStaticRule("jacoco", blueprint.RuleParams{
		Command: `${config.Zip2ZipCmd} -i $in -o $strippedJar $stripSpec && ` +
			`${config.JavaCmd} -jar ${config.JacocoCLIJar} instrument -quiet -dest $instrumentedJar $strippedJar && ` +
			`${config.Ziptime} $instrumentedJar && ` +
			`${config.MergeZipsCmd} --ignore-duplicates -j $out $instrumentedJar $in`,
		CommandDeps: []string{
			"${config.Zip2ZipCmd}",
			"${config.JavaCmd}",
			"${config.JacocoCLIJar}",
			"${config.Ziptime}",
			"${config.MergeZipsCmd}",
		},
	},
		"strippedJar", "stripSpec", "instrumentedJar")
)

func jacocoInstrumentJar(ctx android.ModuleContext, outputJar, strippedJar android.WritablePath,
	inputJar android.Path, stripSpec string) {
	instrumentedJar := android.PathForModuleOut(ctx, "jacoco/instrumented.jar")

	ctx.Build(pctx, android.BuildParams{
		Rule:           jacoco,
		Description:    "jacoco",
		Output:         outputJar,
		ImplicitOutput: strippedJar,
		Input:          inputJar,
		Args: map[string]string{
			"strippedJar":     strippedJar.String(),
			"stripSpec":       stripSpec,
			"instrumentedJar": instrumentedJar.String(),
		},
	})
}

func (j *Module) jacocoStripSpecs(ctx android.ModuleContext) string {
	includes := jacocoFiltersToSpecs(ctx,
		j.properties.Jacoco.Include_filter, "jacoco.include_filter")
	excludes := jacocoFiltersToSpecs(ctx,
		j.properties.Jacoco.Exclude_filter, "jacoco.exclude_filter")

	specs := ""
	if len(excludes) > 0 {
		specs += android.JoinWithPrefix(excludes, "-x") + " "
	}

	if len(includes) > 0 {
		specs += strings.Join(includes, " ")
	} else {
		specs += "**/*.class"
	}

	return specs
}

func jacocoFiltersToSpecs(ctx android.ModuleContext, filters []string, property string) []string {
	specs := make([]string, len(filters))
	for i, f := range filters {
		specs[i] = jacocoFilterToSpec(ctx, f, property)
	}
	return specs
}

func jacocoFilterToSpec(ctx android.ModuleContext, filter string, property string) string {
	wildcard := strings.HasSuffix(filter, "*")
	filter = strings.TrimSuffix(filter, "*")
	recursiveWildcard := wildcard && (strings.HasSuffix(filter, ".") || filter == "")

	if strings.ContainsRune(filter, '*') {
		ctx.PropertyErrorf(property, "'*' is only supported as the last character in a filter")
	}

	spec := strings.Replace(filter, ".", "/", -1)

	if recursiveWildcard {
		spec += "**/*.class"
	} else if wildcard {
		spec += "*.class"
	}

	return spec
}
