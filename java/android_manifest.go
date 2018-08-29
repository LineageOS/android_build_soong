// Copyright 2018 Google Inc. All rights reserved.
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

import (
	"android/soong/java/config"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

var manifestFixerRule = pctx.AndroidStaticRule("manifestFixer",
	blueprint.RuleParams{
		Command:     `${config.ManifestFixerCmd} --minSdkVersion ${minSdkVersion} $args $in $out`,
		CommandDeps: []string{"${config.ManifestFixerCmd}"},
	},
	"minSdkVersion", "args")

var manifestMergerRule = pctx.AndroidStaticRule("manifestMerger",
	blueprint.RuleParams{
		Command: `${config.JavaCmd} -classpath ${config.ManifestMergerClasspath} com.android.manifmerger.Merger ` +
			`--main $in $libs --out $out`,
		CommandDeps: config.ManifestMergerClasspath,
	},
	"libs")

func manifestMerger(ctx android.ModuleContext, manifest android.Path, sdkContext sdkContext,
	staticLibManifests android.Paths, isLibrary bool) android.Path {

	var args []string
	if isLibrary {
		args = append(args, "--library")
	}

	// Inject minSdkVersion into the manifest
	fixedManifest := android.PathForModuleOut(ctx, "manifest_fixer", "AndroidManifest.xml")
	ctx.Build(pctx, android.BuildParams{
		Rule:   manifestFixerRule,
		Input:  manifest,
		Output: fixedManifest,
		Args: map[string]string{
			"minSdkVersion": sdkVersionOrDefault(ctx, sdkContext.minSdkVersion()),
			"args":          strings.Join(args, " "),
		},
	})
	manifest = fixedManifest

	// Merge static aar dependency manifests if necessary
	if len(staticLibManifests) > 0 {
		mergedManifest := android.PathForModuleOut(ctx, "manifest_merger", "AndroidManifest.xml")
		ctx.Build(pctx, android.BuildParams{
			Rule:      manifestMergerRule,
			Input:     manifest,
			Implicits: staticLibManifests,
			Output:    mergedManifest,
			Args: map[string]string{
				"libs": android.JoinWithPrefix(staticLibManifests.Strings(), "--libs "),
			},
		})
		manifest = mergedManifest
	}

	return manifest
}
