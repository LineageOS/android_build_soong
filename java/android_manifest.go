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
		Command:     `${config.ManifestMergerCmd} $args --main $in $libs --out $out`,
		CommandDeps: []string{"${config.ManifestMergerCmd}"},
	},
	"args", "libs")

// These two libs are added as optional dependencies (<uses-library> with
// android:required set to false). This is because they haven't existed in pre-P
// devices, but classes in them were in bootclasspath jars, etc. So making them
// hard dependencies (android:required=true) would prevent apps from being
// installed to such legacy devices.
var optionalUsesLibs = []string{
	"android.test.base",
	"android.test.mock",
}

// Uses manifest_fixer.py to inject minSdkVersion, etc. into an AndroidManifest.xml
func manifestFixer(ctx android.ModuleContext, manifest android.Path, sdkContext sdkContext, sdkLibraries []string,
	isLibrary, useEmbeddedNativeLibs, usesNonSdkApis, useEmbeddedDex, hasNoCode bool, loggingParent string) android.Path {

	var args []string
	if isLibrary {
		args = append(args, "--library")
	} else {
		minSdkVersion, err := sdkContext.minSdkVersion().effectiveVersion(ctx)
		if err != nil {
			ctx.ModuleErrorf("invalid minSdkVersion: %s", err)
		}
		if minSdkVersion >= 23 {
			args = append(args, fmt.Sprintf("--extract-native-libs=%v", !useEmbeddedNativeLibs))
		} else if useEmbeddedNativeLibs {
			ctx.ModuleErrorf("module attempted to store uncompressed native libraries, but minSdkVersion=%d doesn't support it",
				minSdkVersion)
		}
	}

	if usesNonSdkApis {
		args = append(args, "--uses-non-sdk-api")
	}

	if useEmbeddedDex {
		args = append(args, "--use-embedded-dex")
	}

	for _, usesLib := range sdkLibraries {
		if inList(usesLib, optionalUsesLibs) {
			args = append(args, "--optional-uses-library", usesLib)
		} else {
			args = append(args, "--uses-library", usesLib)
		}
	}

	if hasNoCode {
		args = append(args, "--has-no-code")
	}

	if loggingParent != "" {
		args = append(args, "--logging-parent", loggingParent)
	}
	var deps android.Paths
	targetSdkVersion, err := sdkContext.targetSdkVersion().effectiveVersionString(ctx)
	if err != nil {
		ctx.ModuleErrorf("invalid targetSdkVersion: %s", err)
	}
	if UseApiFingerprint(ctx) {
		targetSdkVersion = ctx.Config().PlatformSdkCodename() + fmt.Sprintf(".$$(cat %s)", ApiFingerprintPath(ctx).String())
		deps = append(deps, ApiFingerprintPath(ctx))
	}

	minSdkVersion, err := sdkContext.minSdkVersion().effectiveVersionString(ctx)
	if err != nil {
		ctx.ModuleErrorf("invalid minSdkVersion: %s", err)
	}
	if UseApiFingerprint(ctx) {
		minSdkVersion = ctx.Config().PlatformSdkCodename() + fmt.Sprintf(".$$(cat %s)", ApiFingerprintPath(ctx).String())
		deps = append(deps, ApiFingerprintPath(ctx))
	}

	fixedManifest := android.PathForModuleOut(ctx, "manifest_fixer", "AndroidManifest.xml")
	if err != nil {
		ctx.ModuleErrorf("invalid minSdkVersion: %s", err)
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        manifestFixerRule,
		Description: "fix manifest",
		Input:       manifest,
		Implicits:   deps,
		Output:      fixedManifest,
		Args: map[string]string{
			"minSdkVersion":    minSdkVersion,
			"targetSdkVersion": targetSdkVersion,
			"args":             strings.Join(args, " "),
		},
	})

	return fixedManifest
}

func manifestMerger(ctx android.ModuleContext, manifest android.Path, staticLibManifests android.Paths,
	isLibrary bool) android.Path {

	var args string
	if !isLibrary {
		// Follow Gradle's behavior, only pass --remove-tools-declarations when merging app manifests.
		args = "--remove-tools-declarations"
	}

	mergedManifest := android.PathForModuleOut(ctx, "manifest_merger", "AndroidManifest.xml")
	ctx.Build(pctx, android.BuildParams{
		Rule:        manifestMergerRule,
		Description: "merge manifest",
		Input:       manifest,
		Implicits:   staticLibManifests,
		Output:      mergedManifest,
		Args: map[string]string{
			"libs": android.JoinWithPrefix(staticLibManifests.Strings(), "--libs "),
			"args": args,
		},
	})

	return mergedManifest
}
