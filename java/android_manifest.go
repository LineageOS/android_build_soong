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
	"strconv"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
	"android/soong/dexpreopt"
)

var manifestFixerRule = pctx.AndroidStaticRule("manifestFixer",
	blueprint.RuleParams{
		Command: `${config.ManifestFixerCmd} ` +
			`$args $in $out`,
		CommandDeps: []string{"${config.ManifestFixerCmd}"},
	},
	"args")

var manifestMergerRule = pctx.AndroidStaticRule("manifestMerger",
	blueprint.RuleParams{
		Command:     `${config.ManifestMergerCmd} $args --main $in $libs --out $out`,
		CommandDeps: []string{"${config.ManifestMergerCmd}"},
	},
	"args", "libs")

// targetSdkVersion for manifest_fixer
// When TARGET_BUILD_APPS is not empty, this method returns 10000 for modules targeting an unreleased SDK
// This enables release builds (that run with TARGET_BUILD_APPS=[val...]) to target APIs that have not yet been finalized as part of an SDK
func targetSdkVersionForManifestFixer(ctx android.ModuleContext, params ManifestFixerParams) string {
	targetSdkVersionLevel := params.SdkContext.TargetSdkVersion(ctx)

	// Check if we want to return 10000
	// TODO(b/240294501): Determine the rules for handling test apexes
	if shouldReturnFinalOrFutureInt(ctx, targetSdkVersionLevel, params.EnforceDefaultTargetSdkVersion) {
		return strconv.Itoa(android.FutureApiLevel.FinalOrFutureInt())
	}
	targetSdkVersion, err := targetSdkVersionLevel.EffectiveVersionString(ctx)
	if err != nil {
		ctx.ModuleErrorf("invalid targetSdkVersion: %s", err)
	}
	return targetSdkVersion
}

// Return true for modules targeting "current" if either
// 1. The module is built in unbundled mode (TARGET_BUILD_APPS not empty)
// 2. The module is run as part of MTS, and should be testable on stable branches
// Do not return 10000 if we are enforcing default targetSdkVersion and sdk has been finalised
func shouldReturnFinalOrFutureInt(ctx android.ModuleContext, targetSdkVersionLevel android.ApiLevel, enforceDefaultTargetSdkVersion bool) bool {
	// If this is a REL branch, do not return 10000
	if ctx.Config().PlatformSdkFinal() {
		return false
	}
	// If this a module targeting an unreleased SDK (MTS or unbundled builds), return 10000
	return targetSdkVersionLevel.IsPreview() && (ctx.Config().UnbundledBuildApps() || includedInMts(ctx.Module()))
}

// Helper function that casts android.Module to java.androidTestApp
// If this type conversion is possible, it queries whether the test app is included in an MTS suite
func includedInMts(module android.Module) bool {
	if test, ok := module.(androidTestApp); ok {
		return test.includedInTestSuite("mts")
	}
	return false
}

type ManifestFixerParams struct {
	SdkContext                     android.SdkContext
	ClassLoaderContexts            dexpreopt.ClassLoaderContextMap
	IsLibrary                      bool
	DefaultManifestVersion         string
	UseEmbeddedNativeLibs          bool
	UsesNonSdkApis                 bool
	UseEmbeddedDex                 bool
	HasNoCode                      bool
	TestOnly                       bool
	LoggingParent                  string
	EnforceDefaultTargetSdkVersion bool
}

// Uses manifest_fixer.py to inject minSdkVersion, etc. into an AndroidManifest.xml
func ManifestFixer(ctx android.ModuleContext, manifest android.Path,
	params ManifestFixerParams) android.Path {
	var args []string

	if params.IsLibrary {
		args = append(args, "--library")
	} else if params.SdkContext != nil {
		minSdkVersion, err := params.SdkContext.MinSdkVersion(ctx).EffectiveVersion(ctx)
		if err != nil {
			ctx.ModuleErrorf("invalid minSdkVersion: %s", err)
		}
		if minSdkVersion.FinalOrFutureInt() >= 23 {
			args = append(args, fmt.Sprintf("--extract-native-libs=%v", !params.UseEmbeddedNativeLibs))
		} else if params.UseEmbeddedNativeLibs {
			ctx.ModuleErrorf("module attempted to store uncompressed native libraries, but minSdkVersion=%s doesn't support it",
				minSdkVersion.String())
		}
	}

	if params.UsesNonSdkApis {
		args = append(args, "--uses-non-sdk-api")
	}

	if params.UseEmbeddedDex {
		args = append(args, "--use-embedded-dex")
	}

	if params.ClassLoaderContexts != nil {
		// Libraries propagated via `uses_libs`/`optional_uses_libs` are also added (they may be
		// propagated from dependencies).
		requiredUsesLibs, optionalUsesLibs := params.ClassLoaderContexts.UsesLibs()

		for _, usesLib := range requiredUsesLibs {
			args = append(args, "--uses-library", usesLib)
		}
		for _, usesLib := range optionalUsesLibs {
			args = append(args, "--optional-uses-library", usesLib)
		}
	}

	if params.HasNoCode {
		args = append(args, "--has-no-code")
	}

	if params.TestOnly {
		args = append(args, "--test-only")
	}

	if params.LoggingParent != "" {
		args = append(args, "--logging-parent", params.LoggingParent)
	}
	var deps android.Paths
	var argsMapper = make(map[string]string)

	if params.SdkContext != nil {
		targetSdkVersion := targetSdkVersionForManifestFixer(ctx, params)

		if UseApiFingerprint(ctx) && ctx.ModuleName() != "framework-res" {
			targetSdkVersion = ctx.Config().PlatformSdkCodename() + fmt.Sprintf(".$$(cat %s)", ApiFingerprintPath(ctx).String())
			deps = append(deps, ApiFingerprintPath(ctx))
		}

		args = append(args, "--targetSdkVersion ", targetSdkVersion)

		minSdkVersion, err := params.SdkContext.MinSdkVersion(ctx).EffectiveVersionString(ctx)
		if err != nil {
			ctx.ModuleErrorf("invalid minSdkVersion: %s", err)
		}

		replaceMaxSdkVersionPlaceholder, err := params.SdkContext.ReplaceMaxSdkVersionPlaceholder(ctx).EffectiveVersion(ctx)
		if err != nil {
			ctx.ModuleErrorf("invalid ReplaceMaxSdkVersionPlaceholder: %s", err)
		}

		if UseApiFingerprint(ctx) && ctx.ModuleName() != "framework-res" {
			minSdkVersion = ctx.Config().PlatformSdkCodename() + fmt.Sprintf(".$$(cat %s)", ApiFingerprintPath(ctx).String())
			deps = append(deps, ApiFingerprintPath(ctx))
		}

		if err != nil {
			ctx.ModuleErrorf("invalid minSdkVersion: %s", err)
		}
		args = append(args, "--minSdkVersion ", minSdkVersion)
		args = append(args, "--replaceMaxSdkVersionPlaceholder ", strconv.Itoa(replaceMaxSdkVersionPlaceholder.FinalOrFutureInt()))
		args = append(args, "--raise-min-sdk-version")
	}
	if params.DefaultManifestVersion != "" {
		args = append(args, "--override-placeholder-version", params.DefaultManifestVersion)
	}

	fixedManifest := android.PathForModuleOut(ctx, "manifest_fixer", "AndroidManifest.xml")
	argsMapper["args"] = strings.Join(args, " ")

	ctx.Build(pctx, android.BuildParams{
		Rule:        manifestFixerRule,
		Description: "fix manifest",
		Input:       manifest,
		Implicits:   deps,
		Output:      fixedManifest,
		Args:        argsMapper,
	})

	return fixedManifest.WithoutRel()
}

type ManifestMergerParams struct {
	staticLibManifests android.Paths
	isLibrary          bool
	packageName        string
}

func manifestMerger(ctx android.ModuleContext, manifest android.Path,
	params ManifestMergerParams) android.Path {

	var args []string
	if !params.isLibrary {
		// Follow Gradle's behavior, only pass --remove-tools-declarations when merging app manifests.
		args = append(args, "--remove-tools-declarations")
	}

	packageName := params.packageName
	if packageName != "" {
		args = append(args, "--property PACKAGE="+packageName)
	}

	mergedManifest := android.PathForModuleOut(ctx, "manifest_merger", "AndroidManifest.xml")
	ctx.Build(pctx, android.BuildParams{
		Rule:        manifestMergerRule,
		Description: "merge manifest",
		Input:       manifest,
		Implicits:   params.staticLibManifests,
		Output:      mergedManifest,
		Args: map[string]string{
			"libs": android.JoinWithPrefix(params.staticLibManifests.Strings(), "--libs "),
			"args": strings.Join(args, " "),
		},
	})

	return mergedManifest.WithoutRel()
}
