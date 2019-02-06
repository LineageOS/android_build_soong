// Copyright 2019 Google Inc. All rights reserved.
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
	"android/soong/android"
	"android/soong/java/config"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func init() {
	android.RegisterPreSingletonType("sdk", sdkSingletonFactory)
}

var sdkSingletonKey = android.NewOnceKey("sdkSingletonKey")

type sdkContext interface {
	// sdkVersion eturns the sdk_version property of the current module, or an empty string if it is not set.
	sdkVersion() string
	// minSdkVersion returns the min_sdk_version property of the current module, or sdkVersion() if it is not set.
	minSdkVersion() string
	// targetSdkVersion returns the target_sdk_version property of the current module, or sdkVersion() if it is not set.
	targetSdkVersion() string
}

func sdkVersionOrDefault(ctx android.BaseContext, v string) string {
	switch v {
	case "", "current", "system_current", "test_current", "core_current":
		return ctx.Config().DefaultAppTargetSdk()
	default:
		return v
	}
}

// Returns a sdk version as a number.  For modules targeting an unreleased SDK (meaning it does not yet have a number)
// it returns android.FutureApiLevel (10000).
func sdkVersionToNumber(ctx android.BaseContext, v string) (int, error) {
	switch v {
	case "", "current", "test_current", "system_current", "core_current":
		return ctx.Config().DefaultAppTargetSdkInt(), nil
	default:
		n := android.GetNumericSdkVersion(v)
		if i, err := strconv.Atoi(n); err != nil {
			return -1, fmt.Errorf("invalid sdk version %q", n)
		} else {
			return i, nil
		}
	}
}

func sdkVersionToNumberAsString(ctx android.BaseContext, v string) (string, error) {
	n, err := sdkVersionToNumber(ctx, v)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(n), nil
}

func decodeSdkDep(ctx android.BaseContext, sdkContext sdkContext) sdkDep {
	v := sdkContext.sdkVersion()
	// For PDK builds, use the latest SDK version instead of "current"
	if ctx.Config().IsPdkBuild() && (v == "" || v == "current") {
		sdkVersions := ctx.Config().Get(sdkSingletonKey).([]int)
		latestSdkVersion := 0
		if len(sdkVersions) > 0 {
			latestSdkVersion = sdkVersions[len(sdkVersions)-1]
		}
		v = strconv.Itoa(latestSdkVersion)
	}

	i, err := sdkVersionToNumber(ctx, v)
	if err != nil {
		ctx.PropertyErrorf("sdk_version", "%s", err)
		return sdkDep{}
	}

	toPrebuilt := func(sdk string) sdkDep {
		var api, v string
		if strings.Contains(sdk, "_") {
			t := strings.Split(sdk, "_")
			api = t[0]
			v = t[1]
		} else {
			api = "public"
			v = sdk
		}
		dir := filepath.Join("prebuilts", "sdk", v, api)
		jar := filepath.Join(dir, "android.jar")
		// There's no aidl for other SDKs yet.
		// TODO(77525052): Add aidl files for other SDKs too.
		public_dir := filepath.Join("prebuilts", "sdk", v, "public")
		aidl := filepath.Join(public_dir, "framework.aidl")
		jarPath := android.ExistentPathForSource(ctx, jar)
		aidlPath := android.ExistentPathForSource(ctx, aidl)
		lambdaStubsPath := android.PathForSource(ctx, config.SdkLambdaStubsPath)

		if (!jarPath.Valid() || !aidlPath.Valid()) && ctx.Config().AllowMissingDependencies() {
			return sdkDep{
				invalidVersion: true,
				modules:        []string{fmt.Sprintf("sdk_%s_%s_android", api, v)},
			}
		}

		if !jarPath.Valid() {
			ctx.PropertyErrorf("sdk_version", "invalid sdk version %q, %q does not exist", v, jar)
			return sdkDep{}
		}

		if !aidlPath.Valid() {
			ctx.PropertyErrorf("sdk_version", "invalid sdk version %q, %q does not exist", v, aidl)
			return sdkDep{}
		}

		return sdkDep{
			useFiles: true,
			jars:     android.Paths{jarPath.Path(), lambdaStubsPath},
			aidl:     aidlPath.Path(),
		}
	}

	toModule := func(m, r string) sdkDep {
		ret := sdkDep{
			useModule:          true,
			modules:            []string{m, config.DefaultLambdaStubsLibrary},
			systemModules:      m + "_system_modules",
			frameworkResModule: r,
		}
		if m == "core.current.stubs" {
			ret.systemModules = "core-system-modules"
		} else if m == "core.platform.api.stubs" {
			ret.systemModules = "core-platform-api-stubs-system-modules"
		}
		return ret
	}

	// Ensures that the specificed system SDK version is one of BOARD_SYSTEMSDK_VERSIONS (for vendor apks)
	// or PRODUCT_SYSTEMSDK_VERSIONS (for other apks or when BOARD_SYSTEMSDK_VERSIONS is not set)
	if strings.HasPrefix(v, "system_") && i != android.FutureApiLevel {
		allowed_versions := ctx.DeviceConfig().PlatformSystemSdkVersions()
		if ctx.DeviceSpecific() || ctx.SocSpecific() {
			if len(ctx.DeviceConfig().SystemSdkVersions()) > 0 {
				allowed_versions = ctx.DeviceConfig().SystemSdkVersions()
			}
		}
		version := strings.TrimPrefix(v, "system_")
		if len(allowed_versions) > 0 && !android.InList(version, allowed_versions) {
			ctx.PropertyErrorf("sdk_version", "incompatible sdk version %q. System SDK version should be one of %q",
				v, allowed_versions)
		}
	}

	if ctx.Config().UnbundledBuildPrebuiltSdks() && v != "" {
		return toPrebuilt(v)
	}

	switch v {
	case "":
		return sdkDep{
			useDefaultLibs:     true,
			frameworkResModule: "framework-res",
		}
	case "current":
		return toModule("android_stubs_current", "framework-res")
	case "system_current":
		return toModule("android_system_stubs_current", "framework-res")
	case "test_current":
		return toModule("android_test_stubs_current", "framework-res")
	case "core_current":
		return toModule("core.current.stubs", "")
	default:
		return toPrebuilt(v)
	}
}

func sdkSingletonFactory() android.Singleton {
	return sdkSingleton{}
}

type sdkSingleton struct{}

func (sdkSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	sdkJars, err := ctx.GlobWithDeps("prebuilts/sdk/*/public/android.jar", nil)
	if err != nil {
		ctx.Errorf("failed to glob prebuilts/sdk/*/public/android.jar: %s", err.Error())
	}

	var sdkVersions []int
	for _, sdkJar := range sdkJars {
		dir := filepath.Base(filepath.Dir(filepath.Dir(sdkJar)))
		v, err := strconv.Atoi(dir)
		if scerr, ok := err.(*strconv.NumError); ok && scerr.Err == strconv.ErrSyntax {
			continue
		} else if err != nil {
			ctx.Errorf("invalid sdk jar %q, %s, %v", sdkJar, err.Error())
		}
		sdkVersions = append(sdkVersions, v)
	}

	sort.Ints(sdkVersions)

	ctx.Config().Once(sdkSingletonKey, func() interface{} { return sdkVersions })
}
