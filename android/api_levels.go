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

package android

import (
	"android/soong/starlark_import"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func init() {
	RegisterParallelSingletonType("api_levels", ApiLevelsSingleton)
}

const previewAPILevelBase = 9000

// An API level, which may be a finalized (numbered) API, a preview (codenamed)
// API, or the future API level (10000). Can be parsed from a string with
// ApiLevelFromUser or ApiLevelOrPanic.
//
// The different *types* of API levels are handled separately. Currently only
// Java has these, and they're managed with the SdkKind enum of the SdkSpec. A
// future cleanup should be to migrate SdkSpec to using ApiLevel instead of its
// SdkVersion int, and to move SdkSpec into this package.
type ApiLevel struct {
	// The string representation of the API level.
	value string

	// A number associated with the API level. The exact value depends on
	// whether this API level is a preview or final API.
	//
	// For final API levels, this is the assigned version number.
	//
	// For preview API levels, this value has no meaning except to index known
	// previews to determine ordering.
	number int

	// Identifies this API level as either a preview or final API level.
	isPreview bool
}

func (this ApiLevel) FinalInt() int {
	if this.IsInvalid() {
		panic(fmt.Errorf("%v is not a recognized api_level\n", this))
	}
	if this.IsPreview() {
		panic("Requested a final int from a non-final ApiLevel")
	} else {
		return this.number
	}
}

func (this ApiLevel) FinalOrFutureInt() int {
	if this.IsInvalid() {
		panic(fmt.Errorf("%v is not a recognized api_level\n", this))
	}
	if this.IsPreview() {
		return FutureApiLevelInt
	} else {
		return this.number
	}
}

// FinalOrPreviewInt distinguishes preview versions from "current" (future).
// This is for "native" stubs and should be in sync with ndkstubgen/getApiLevelsMap().
// - "current" -> future (10000)
// - preview codenames -> preview base (9000) + index
// - otherwise -> cast to int
func (this ApiLevel) FinalOrPreviewInt() int {
	if this.IsInvalid() {
		panic(fmt.Errorf("%v is not a recognized api_level\n", this))
	}
	if this.IsCurrent() {
		return this.number
	}
	if this.IsPreview() {
		return previewAPILevelBase + this.number
	}
	return this.number
}

// Returns the canonical name for this API level. For a finalized API level
// this will be the API number as a string. For a preview API level this
// will be the codename, or "current".
func (this ApiLevel) String() string {
	return this.value
}

// Returns true if this is a non-final API level.
func (this ApiLevel) IsPreview() bool {
	return this.isPreview
}

// Returns true if the raw api level string is invalid
func (this ApiLevel) IsInvalid() bool {
	return this.EqualTo(InvalidApiLevel)
}

// Returns true if this is the unfinalized "current" API level. This means
// different things across Java and native. Java APIs do not use explicit
// codenames, so all non-final codenames are grouped into "current". For native
// explicit codenames are typically used, and current is the union of all
// non-final APIs, including those that may not yet be in any codename.
//
// Note that in a build where the platform is final, "current" will not be a
// preview API level but will instead be canonicalized to the final API level.
func (this ApiLevel) IsCurrent() bool {
	return this.value == "current"
}

func (this ApiLevel) IsNone() bool {
	return this.number == -1
}

// Returns true if an app is compiling against private apis.
// e.g. if sdk_version = "" in Android.bp, then the ApiLevel of that "sdk" is at PrivateApiLevel.
func (this ApiLevel) IsPrivate() bool {
	return this.number == PrivateApiLevel.number
}

// EffectiveVersion converts an ApiLevel into the concrete ApiLevel that the module should use. For
// modules targeting an unreleased SDK (meaning it does not yet have a number) it returns
// FutureApiLevel(10000).
func (l ApiLevel) EffectiveVersion(ctx EarlyModuleContext) (ApiLevel, error) {
	if l.EqualTo(InvalidApiLevel) {
		return l, fmt.Errorf("invalid version in sdk_version %q", l.value)
	}
	if !l.IsPreview() {
		return l, nil
	}
	ret := ctx.Config().DefaultAppTargetSdk(ctx)
	if ret.IsPreview() {
		return FutureApiLevel, nil
	}
	return ret, nil
}

// EffectiveVersionString converts an SdkSpec into the concrete version string that the module
// should use. For modules targeting an unreleased SDK (meaning it does not yet have a number)
// it returns the codename (P, Q, R, etc.)
func (l ApiLevel) EffectiveVersionString(ctx EarlyModuleContext) (string, error) {
	if l.EqualTo(InvalidApiLevel) {
		return l.value, fmt.Errorf("invalid version in sdk_version %q", l.value)
	}
	if !l.IsPreview() {
		return l.String(), nil
	}
	// Determine the default sdk
	ret := ctx.Config().DefaultAppTargetSdk(ctx)
	if !ret.IsPreview() {
		// If the default sdk has been finalized, return that
		return ret.String(), nil
	}
	// There can be more than one active in-development sdks
	// If an app is targeting an active sdk, but not the default one, return the requested active sdk.
	// e.g.
	// SETUP
	// In-development: UpsideDownCake, VanillaIceCream
	// Default: VanillaIceCream
	// Android.bp
	// min_sdk_version: `UpsideDownCake`
	// RETURN
	// UpsideDownCake and not VanillaIceCream
	for _, preview := range ctx.Config().PreviewApiLevels() {
		if l.String() == preview.String() {
			return preview.String(), nil
		}
	}
	// Otherwise return the default one
	return ret.String(), nil
}

// Specified returns true if the module is targeting a recognzized api_level.
// It returns false if either
// 1. min_sdk_version is not an int or a recognized codename
// 2. both min_sdk_version and sdk_version are empty. In this case, MinSdkVersion() defaults to SdkSpecPrivate.ApiLevel
func (this ApiLevel) Specified() bool {
	return !this.IsInvalid() && !this.IsPrivate()
}

// Returns -1 if the current API level is less than the argument, 0 if they
// are equal, and 1 if it is greater than the argument.
func (this ApiLevel) CompareTo(other ApiLevel) int {
	if this.IsPreview() && !other.IsPreview() {
		return 1
	} else if !this.IsPreview() && other.IsPreview() {
		return -1
	}

	if this.number < other.number {
		return -1
	} else if this.number == other.number {
		return 0
	} else {
		return 1
	}
}

func (this ApiLevel) EqualTo(other ApiLevel) bool {
	return this.CompareTo(other) == 0
}

func (this ApiLevel) GreaterThan(other ApiLevel) bool {
	return this.CompareTo(other) > 0
}

func (this ApiLevel) GreaterThanOrEqualTo(other ApiLevel) bool {
	return this.CompareTo(other) >= 0
}

func (this ApiLevel) LessThan(other ApiLevel) bool {
	return this.CompareTo(other) < 0
}

func (this ApiLevel) LessThanOrEqualTo(other ApiLevel) bool {
	return this.CompareTo(other) <= 0
}

func uncheckedFinalApiLevel(num int) ApiLevel {
	return ApiLevel{
		value:     strconv.Itoa(num),
		number:    num,
		isPreview: false,
	}
}

func uncheckedFinalIncrementalApiLevel(num int, increment int) ApiLevel {
	return ApiLevel{
		value:     strconv.Itoa(num) + "." + strconv.Itoa(increment),
		number:    num,
		isPreview: false,
	}
}

var NoneApiLevel = ApiLevel{
	value: "(no version)",
	// Not 0 because we don't want this to compare equal with the first preview.
	number:    -1,
	isPreview: true,
}

// Sentinel ApiLevel to validate that an apiLevel is either an int or a recognized codename.
var InvalidApiLevel = NewInvalidApiLevel("invalid")

// Returns an apiLevel object at the same level as InvalidApiLevel.
// The object contains the raw string provied in bp file, and can be used for error handling.
func NewInvalidApiLevel(raw string) ApiLevel {
	return ApiLevel{
		value:     raw,
		number:    -2, // One less than NoneApiLevel
		isPreview: true,
	}
}

// The first version that introduced 64-bit ABIs.
var FirstLp64Version = uncheckedFinalApiLevel(21)

// Android has had various kinds of packed relocations over the years
// (http://b/187907243).
//
// API level 30 is where the now-standard SHT_RELR is available.
var FirstShtRelrVersion = uncheckedFinalApiLevel(30)

// API level 28 introduced SHT_RELR when it was still Android-only, and used an
// Android-specific relocation.
var FirstAndroidRelrVersion = uncheckedFinalApiLevel(28)

// API level 23 was when we first had the Chrome relocation packer, which is
// obsolete and has been removed, but lld can now generate compatible packed
// relocations itself.
var FirstPackedRelocationsVersion = uncheckedFinalApiLevel(23)

// LastWithoutModuleLibCoreSystemModules is the last API level where prebuilts/sdk does not contain
// a core-for-system-modules.jar for the module-lib API scope.
var LastWithoutModuleLibCoreSystemModules = uncheckedFinalApiLevel(31)

var ApiLevelR = uncheckedFinalApiLevel(30)

// ReplaceFinalizedCodenames returns the API level number associated with that API level
// if the `raw` input is the codename of an API level has been finalized.
// If the input is *not* a finalized codename, the input is returned unmodified.
func ReplaceFinalizedCodenames(config Config, raw string) (string, error) {
	finalCodenamesMap, err := getFinalCodenamesMap(config)
	if err != nil {
		return raw, err
	}
	num, ok := finalCodenamesMap[raw]
	if !ok {
		return raw, nil
	}

	return strconv.Itoa(num), nil
}

// ApiLevelFrom converts the given string `raw` to an ApiLevel.
// If `raw` is invalid (empty string, unrecognized codename etc.) it returns an invalid ApiLevel
func ApiLevelFrom(ctx PathContext, raw string) ApiLevel {
	ret, err := ApiLevelFromUser(ctx, raw)
	if err != nil {
		return NewInvalidApiLevel(raw)
	}
	return ret
}

// ApiLevelFromUser converts the given string `raw` to an ApiLevel, possibly returning an error.
//
// `raw` must be non-empty. Passing an empty string results in a panic.
//
// "current" will return CurrentApiLevel, which is the ApiLevel associated with
// an arbitrary future release (often referred to as API level 10000).
//
// Finalized codenames will be interpreted as their final API levels, not the
// preview of the associated releases. R is now API 30, not the R preview.
//
// Future codenames return a preview API level that has no associated integer.
//
// Inputs that are not "current", known previews, or convertible to an integer
// will return an error.
func ApiLevelFromUser(ctx PathContext, raw string) (ApiLevel, error) {
	return ApiLevelFromUserWithConfig(ctx.Config(), raw)
}

// ApiLevelFromUserWithConfig implements ApiLevelFromUser, see comments for
// ApiLevelFromUser for more details.
func ApiLevelFromUserWithConfig(config Config, raw string) (ApiLevel, error) {
	// This logic is replicated in starlark, if changing logic here update starlark code too
	// https://cs.android.com/android/platform/superproject/+/main:build/bazel/rules/common/api.bzl;l=42;drc=231c7e8c8038fd478a79eb68aa5b9f5c64e0e061
	if raw == "" {
		panic("API level string must be non-empty")
	}

	if raw == "current" {
		return FutureApiLevel, nil
	}

	for _, preview := range config.PreviewApiLevels() {
		if raw == preview.String() {
			return preview, nil
		}
	}

	apiLevelsReleasedVersions, err := getApiLevelsMapReleasedVersions()
	if err != nil {
		return NoneApiLevel, err
	}
	canonical, ok := apiLevelsReleasedVersions[raw]
	if !ok {
		asInt, err := strconv.Atoi(raw)
		if err != nil {
			return NoneApiLevel, fmt.Errorf("%q could not be parsed as an integer and is not a recognized codename", raw)
		}
		return uncheckedFinalApiLevel(asInt), nil
	}

	return uncheckedFinalApiLevel(canonical), nil

}

// ApiLevelForTest returns an ApiLevel constructed from the supplied raw string.
//
// This only supports "current" and numeric levels, code names are not supported.
func ApiLevelForTest(raw string) ApiLevel {
	if raw == "" {
		panic("API level string must be non-empty")
	}

	if raw == "current" {
		return FutureApiLevel
	}

	if strings.Contains(raw, ".") {
		// Check prebuilt incremental API format MM.m for major (API level) and minor (incremental) revisions
		parts := strings.Split(raw, ".")
		if len(parts) != 2 {
			panic(fmt.Errorf("Found unexpected version '%s' for incremental API - expect MM.m format for incremental API with both major (MM) an minor (m) revision.", raw))
		}
		sdk, sdk_err := strconv.Atoi(parts[0])
		qpr, qpr_err := strconv.Atoi(parts[1])
		if sdk_err != nil || qpr_err != nil {
			panic(fmt.Errorf("Unable to read version number for incremental api '%s'", raw))
		}

		apiLevel := uncheckedFinalIncrementalApiLevel(sdk, qpr)
		return apiLevel
	}

	asInt, err := strconv.Atoi(raw)
	if err != nil {
		panic(fmt.Errorf("%q could not be parsed as an integer and is not a recognized codename", raw))
	}

	apiLevel := uncheckedFinalApiLevel(asInt)
	return apiLevel
}

// Converts an API level string `raw` into an ApiLevel in the same method as
// `ApiLevelFromUser`, but the input is assumed to have no errors and any errors
// will panic instead of returning an error.
func ApiLevelOrPanic(ctx PathContext, raw string) ApiLevel {
	value, err := ApiLevelFromUser(ctx, raw)
	if err != nil {
		panic(err.Error())
	}
	return value
}

func ApiLevelsSingleton() Singleton {
	return &apiLevelsSingleton{}
}

type apiLevelsSingleton struct{}

func createApiLevelsJson(ctx SingletonContext, file WritablePath,
	apiLevelsMap map[string]int) {

	jsonStr, err := json.Marshal(apiLevelsMap)
	if err != nil {
		ctx.Errorf(err.Error())
	}

	WriteFileRule(ctx, file, string(jsonStr))
}

func GetApiLevelsJson(ctx PathContext) WritablePath {
	return PathForOutput(ctx, "api_levels.json")
}

func getApiLevelsMapReleasedVersions() (map[string]int, error) {
	return starlark_import.GetStarlarkValue[map[string]int]("api_levels_released_versions")
}

var finalCodenamesMapKey = NewOnceKey("FinalCodenamesMap")

func getFinalCodenamesMap(config Config) (map[string]int, error) {
	type resultStruct struct {
		result map[string]int
		err    error
	}
	// This logic is replicated in starlark, if changing logic here update starlark code too
	// https://cs.android.com/android/platform/superproject/+/main:build/bazel/rules/common/api.bzl;l=30;drc=231c7e8c8038fd478a79eb68aa5b9f5c64e0e061
	result := config.Once(finalCodenamesMapKey, func() interface{} {
		apiLevelsMap, err := getApiLevelsMapReleasedVersions()

		// TODO: Differentiate "current" and "future".
		// The code base calls it FutureApiLevel, but the spelling is "current",
		// and these are really two different things. When defining APIs it
		// means the API has not yet been added to a specific release. When
		// choosing an API level to build for it means that the future API level
		// should be used, except in the case where the build is finalized in
		// which case the platform version should be used. This is *weird*,
		// because in the circumstance where API foo was added in R and bar was
		// added in S, both of these are usable when building for "current" when
		// neither R nor S are final, but the S APIs stop being available in a
		// final R build.
		if err == nil && Bool(config.productVariables.Platform_sdk_final) {
			apiLevelsMap["current"] = config.PlatformSdkVersion().FinalOrFutureInt()
		}

		return resultStruct{apiLevelsMap, err}
	}).(resultStruct)
	return result.result, result.err
}

var apiLevelsMapKey = NewOnceKey("ApiLevelsMap")

// ApiLevelsMap has entries for preview API levels
func GetApiLevelsMap(config Config) (map[string]int, error) {
	type resultStruct struct {
		result map[string]int
		err    error
	}
	// This logic is replicated in starlark, if changing logic here update starlark code too
	// https://cs.android.com/android/platform/superproject/+/main:build/bazel/rules/common/api.bzl;l=23;drc=231c7e8c8038fd478a79eb68aa5b9f5c64e0e061
	result := config.Once(apiLevelsMapKey, func() interface{} {
		apiLevelsMap, err := getApiLevelsMapReleasedVersions()
		if err == nil {
			for i, codename := range config.PlatformVersionAllPreviewCodenames() {
				apiLevelsMap[codename] = previewAPILevelBase + i
			}
		}

		return resultStruct{apiLevelsMap, err}
	}).(resultStruct)
	return result.result, result.err
}

func (a *apiLevelsSingleton) GenerateBuildActions(ctx SingletonContext) {
	apiLevelsMap, err := GetApiLevelsMap(ctx.Config())
	if err != nil {
		ctx.Errorf("%s\n", err)
		return
	}
	apiLevelsJson := GetApiLevelsJson(ctx)
	createApiLevelsJson(ctx, apiLevelsJson, apiLevelsMap)
}
