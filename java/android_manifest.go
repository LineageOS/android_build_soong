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
	"fmt"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

var manifestFixerRule = pctx.AndroidStaticRule("manifestFixer",
	blueprint.RuleParams{
		Command: `${config.ManifestFixerCmd} ` +
			`--minSdkVersion ${minSdkVersion} ` +
			`--targetSdkVersion ${targetSdkVersion} ` +
			`--raise-min-sdk-version ` +
			`$args $in $out`,
		CommandDeps: []string{"${config.ManifestFixerCmd}"},
	},
	"minSdkVersion", "targetSdkVersion", "args")

var manifestMergerRule = pctx.AndroidStaticRule("manifestMerger",
	blueprint.RuleParams{
		Command:     `${config.ManifestMergerCmd} --main $in $libs --out $out`,
		CommandDeps: []string{"${config.ManifestMergerCmd}"},
	},
	"libs")

// Uses manifest_fixer.py to inject minSdkVersion, etc. into an AndroidManifest.xml
func manifestFixer(ctx android.ModuleContext, manifest android.Path, sdkContext sdkContext,
	isLibrary, uncompressedJNI, usesNonSdkApis, useEmbeddedDex bool) android.Path {

	var args []string
	if isLibrary {
		args = append(args, "--library")
	} else {
		minSdkVersion, err := sdkVersionToNumber(ctx, sdkContext.minSdkVersion())
		if err != nil {
			ctx.ModuleErrorf("invalid minSdkVersion: %s", err)
		}
		if minSdkVersion >= 23 {
			args = append(args, fmt.Sprintf("--extract-native-libs=%v", !uncompressedJNI))
		} else if uncompressedJNI {
			ctx.ModuleErrorf("module attempted to store uncompressed native libraries, but minSdkVersion=%d doesn't support it",
				minSdkVersion)
		}
	}

	if usesNonSdkApis {
		args = append(args, "--uses-non-sdk-api")
	}

	if useEmbeddedDex {
		args = append(args, "--use-embedded-dex=true")
	}

	var deps android.Paths
	targetSdkVersion := sdkVersionOrDefault(ctx, sdkContext.targetSdkVersion())
	if targetSdkVersion == ctx.Config().PlatformSdkCodename() &&
		ctx.Config().UnbundledBuild() &&
		!ctx.Config().UnbundledBuildUsePrebuiltSdks() &&
		ctx.Config().IsEnvTrue("UNBUNDLED_BUILD_TARGET_SDK_WITH_API_FINGERPRINT") {
		apiFingerprint := ApiFingerprintPath(ctx)
		targetSdkVersion += fmt.Sprintf(".$$(cat %s)", apiFingerprint.String())
		deps = append(deps, apiFingerprint)
	}

	fixedManifest := android.PathForModuleOut(ctx, "manifest_fixer", "AndroidManifest.xml")
	ctx.Build(pctx, android.BuildParams{
		Rule:        manifestFixerRule,
		Description: "fix manifest",
		Input:       manifest,
		Implicits:   deps,
		Output:      fixedManifest,
		Args: map[string]string{
			"minSdkVersion":    sdkVersionOrDefault(ctx, sdkContext.minSdkVersion()),
			"targetSdkVersion": targetSdkVersion,
			"args":             strings.Join(args, " "),
		},
	})

	return fixedManifest
}

func manifestMerger(ctx android.ModuleContext, manifest android.Path, staticLibManifests android.Paths) android.Path {
	mergedManifest := android.PathForModuleOut(ctx, "manifest_merger", "AndroidManifest.xml")
	ctx.Build(pctx, android.BuildParams{
		Rule:        manifestMergerRule,
		Description: "merge manifest",
		Input:       manifest,
		Implicits:   staticLibManifests,
		Output:      mergedManifest,
		Args: map[string]string{
			"libs": android.JoinWithPrefix(staticLibManifests.Strings(), "--libs "),
		},
	})

	return mergedManifest
}
