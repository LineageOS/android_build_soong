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
	"encoding/json"
	"fmt"
	"strconv"
)

func init() {
	RegisterSingletonType("api_levels", ApiLevelsSingleton)
}

// An API level, which may be a finalized (numbered) API, a preview (codenamed)
// API, or the future API level (10000). Can be parsed from a string with
// ApiLevelFromUser or ApiLevelOrPanic.
//
// The different *types* of API levels are handled separately. Currently only
// Java has these, and they're managed with the sdkKind enum of the sdkSpec. A
// future cleanup should be to migrate sdkSpec to using ApiLevel instead of its
// sdkVersion int, and to move sdkSpec into this package.
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

func (this ApiLevel) FinalOrFutureInt() int {
	if this.IsPreview() {
		return FutureApiLevelInt
	} else {
		return this.number
	}
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

var NoneApiLevel = ApiLevel{
	value: "(no version)",
	// Not 0 because we don't want this to compare equal with the first preview.
	number:    -1,
	isPreview: true,
}

// The first version that introduced 64-bit ABIs.
var FirstLp64Version = uncheckedFinalApiLevel(21)

// The first API level that does not require NDK code to link
// libandroid_support.
var FirstNonLibAndroidSupportVersion = uncheckedFinalApiLevel(21)

// If the `raw` input is the codename of an API level has been finalized, this
// function returns the API level number associated with that API level. If the
// input is *not* a finalized codename, the input is returned unmodified.
//
// For example, at the time of writing, R has been finalized as API level 30,
// but S is in development so it has no number assigned. For the following
// inputs:
//
// * "30" -> "30"
// * "R" -> "30"
// * "S" -> "S"
func ReplaceFinalizedCodenames(ctx PathContext, raw string) string {
	num, ok := getFinalCodenamesMap(ctx.Config())[raw]
	if !ok {
		return raw
	}

	return strconv.Itoa(num)
}

// Converts the given string `raw` to an ApiLevel, possibly returning an error.
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
	if raw == "" {
		panic("API level string must be non-empty")
	}

	if raw == "current" {
		return FutureApiLevel, nil
	}

	for _, preview := range ctx.Config().PreviewApiLevels() {
		if raw == preview.String() {
			return preview, nil
		}
	}

	canonical := ReplaceFinalizedCodenames(ctx, raw)
	asInt, err := strconv.Atoi(canonical)
	if err != nil {
		return NoneApiLevel, fmt.Errorf("%q could not be parsed as an integer and is not a recognized codename", canonical)
	}

	apiLevel := uncheckedFinalApiLevel(asInt)
	return apiLevel, nil
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

var finalCodenamesMapKey = NewOnceKey("FinalCodenamesMap")

func getFinalCodenamesMap(config Config) map[string]int {
	return config.Once(finalCodenamesMapKey, func() interface{} {
		apiLevelsMap := map[string]int{
			"G":     9,
			"I":     14,
			"J":     16,
			"J-MR1": 17,
			"J-MR2": 18,
			"K":     19,
			"L":     21,
			"L-MR1": 22,
			"M":     23,
			"N":     24,
			"N-MR1": 25,
			"O":     26,
			"O-MR1": 27,
			"P":     28,
			"Q":     29,
			"R":     30,
		}

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
		if Bool(config.productVariables.Platform_sdk_final) {
			apiLevelsMap["current"] = config.PlatformSdkVersion().FinalOrFutureInt()
		}

		return apiLevelsMap
	}).(map[string]int)
}

var apiLevelsMapKey = NewOnceKey("ApiLevelsMap")

func getApiLevelsMap(config Config) map[string]int {
	return config.Once(apiLevelsMapKey, func() interface{} {
		baseApiLevel := 9000
		apiLevelsMap := map[string]int{
			"G":     9,
			"I":     14,
			"J":     16,
			"J-MR1": 17,
			"J-MR2": 18,
			"K":     19,
			"L":     21,
			"L-MR1": 22,
			"M":     23,
			"N":     24,
			"N-MR1": 25,
			"O":     26,
			"O-MR1": 27,
			"P":     28,
			"Q":     29,
			"R":     30,
		}
		for i, codename := range config.PlatformVersionActiveCodenames() {
			apiLevelsMap[codename] = baseApiLevel + i
		}

		return apiLevelsMap
	}).(map[string]int)
}

func (a *apiLevelsSingleton) GenerateBuildActions(ctx SingletonContext) {
	apiLevelsMap := getApiLevelsMap(ctx.Config())
	apiLevelsJson := GetApiLevelsJson(ctx)
	createApiLevelsJson(ctx, apiLevelsJson, apiLevelsMap)
}
