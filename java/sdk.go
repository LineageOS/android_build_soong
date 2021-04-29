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
	"fmt"
	"path/filepath"
	"sort"
	"strconv"

	"android/soong/android"
	"android/soong/java/config"

	"github.com/google/blueprint/pathtools"
)

func init() {
	android.RegisterPreSingletonType("sdk_versions", sdkPreSingletonFactory)
	android.RegisterSingletonType("sdk", sdkSingletonFactory)
	android.RegisterMakeVarsProvider(pctx, sdkMakeVars)
}

var sdkVersionsKey = android.NewOnceKey("sdkVersionsKey")
var sdkFrameworkAidlPathKey = android.NewOnceKey("sdkFrameworkAidlPathKey")
var nonUpdatableFrameworkAidlPathKey = android.NewOnceKey("nonUpdatableFrameworkAidlPathKey")
var apiFingerprintPathKey = android.NewOnceKey("apiFingerprintPathKey")

func UseApiFingerprint(ctx android.BaseModuleContext) bool {
	if ctx.Config().UnbundledBuild() &&
		!ctx.Config().AlwaysUsePrebuiltSdks() &&
		ctx.Config().IsEnvTrue("UNBUNDLED_BUILD_TARGET_SDK_WITH_API_FINGERPRINT") {
		return true
	}
	return false
}

func defaultJavaLanguageVersion(ctx android.EarlyModuleContext, s android.SdkSpec) javaVersion {
	sdk, err := s.EffectiveVersion(ctx)
	if err != nil {
		ctx.PropertyErrorf("sdk_version", "%s", err)
	}
	if sdk.FinalOrFutureInt() <= 23 {
		return JAVA_VERSION_7
	} else if sdk.FinalOrFutureInt() <= 29 {
		return JAVA_VERSION_8
	} else {
		return JAVA_VERSION_9
	}
}

func decodeSdkDep(ctx android.EarlyModuleContext, sdkContext android.SdkContext) sdkDep {
	sdkVersion := sdkContext.SdkVersion(ctx)
	if !sdkVersion.Valid() {
		ctx.PropertyErrorf("sdk_version", "invalid version %q", sdkVersion.Raw)
		return sdkDep{}
	}

	if ctx.DeviceSpecific() || ctx.SocSpecific() {
		sdkVersion = sdkVersion.ForVendorPartition(ctx)
	}

	if !sdkVersion.ValidateSystemSdk(ctx) {
		return sdkDep{}
	}

	if sdkVersion.UsePrebuilt(ctx) {
		dir := filepath.Join("prebuilts", "sdk", sdkVersion.ApiLevel.String(), sdkVersion.Kind.String())
		jar := filepath.Join(dir, "android.jar")
		// There's no aidl for other SDKs yet.
		// TODO(77525052): Add aidl files for other SDKs too.
		publicDir := filepath.Join("prebuilts", "sdk", sdkVersion.ApiLevel.String(), "public")
		aidl := filepath.Join(publicDir, "framework.aidl")
		jarPath := android.ExistentPathForSource(ctx, jar)
		aidlPath := android.ExistentPathForSource(ctx, aidl)
		lambdaStubsPath := android.PathForSource(ctx, config.SdkLambdaStubsPath)

		if (!jarPath.Valid() || !aidlPath.Valid()) && ctx.Config().AllowMissingDependencies() {
			return sdkDep{
				invalidVersion: true,
				bootclasspath:  []string{fmt.Sprintf("sdk_%s_%s_android", sdkVersion.Kind, sdkVersion.ApiLevel.String())},
			}
		}

		if !jarPath.Valid() {
			ctx.PropertyErrorf("sdk_version", "invalid sdk version %q, %q does not exist", sdkVersion.Raw, jar)
			return sdkDep{}
		}

		if !aidlPath.Valid() {
			ctx.PropertyErrorf("sdk_version", "invalid sdk version %q, %q does not exist", sdkVersion.Raw, aidl)
			return sdkDep{}
		}

		var systemModules string
		if defaultJavaLanguageVersion(ctx, sdkVersion).usesJavaModules() {
			systemModules = "sdk_public_" + sdkVersion.ApiLevel.String() + "_system_modules"
		}

		return sdkDep{
			useFiles:      true,
			jars:          android.Paths{jarPath.Path(), lambdaStubsPath},
			aidl:          android.OptionalPathForPath(aidlPath.Path()),
			systemModules: systemModules,
		}
	}

	toModule := func(modules []string, res string, aidl android.Path) sdkDep {
		return sdkDep{
			useModule:          true,
			bootclasspath:      append(modules, config.DefaultLambdaStubsLibrary),
			systemModules:      "core-current-stubs-system-modules",
			java9Classpath:     modules,
			frameworkResModule: res,
			aidl:               android.OptionalPathForPath(aidl),
		}
	}

	switch sdkVersion.Kind {
	case android.SdkPrivate:
		return sdkDep{
			useModule:          true,
			systemModules:      corePlatformSystemModules(ctx),
			bootclasspath:      corePlatformBootclasspathLibraries(ctx),
			classpath:          config.FrameworkLibraries,
			frameworkResModule: "framework-res",
		}
	case android.SdkNone:
		systemModules := sdkContext.SystemModules()
		if systemModules == "" {
			ctx.PropertyErrorf("sdk_version",
				`system_modules is required to be set to a non-empty value when sdk_version is "none", did you mean sdk_version: "core_platform"?`)
		} else if systemModules == "none" {
			return sdkDep{
				noStandardLibs: true,
			}
		}

		return sdkDep{
			useModule:      true,
			noStandardLibs: true,
			systemModules:  systemModules,
			bootclasspath:  []string{systemModules},
		}
	case android.SdkCorePlatform:
		return sdkDep{
			useModule:        true,
			systemModules:    corePlatformSystemModules(ctx),
			bootclasspath:    corePlatformBootclasspathLibraries(ctx),
			noFrameworksLibs: true,
		}
	case android.SdkPublic:
		return toModule([]string{"android_stubs_current"}, "framework-res", sdkFrameworkAidlPath(ctx))
	case android.SdkSystem:
		return toModule([]string{"android_system_stubs_current"}, "framework-res", sdkFrameworkAidlPath(ctx))
	case android.SdkTest:
		return toModule([]string{"android_test_stubs_current"}, "framework-res", sdkFrameworkAidlPath(ctx))
	case android.SdkCore:
		return sdkDep{
			useModule:        true,
			bootclasspath:    []string{"core.current.stubs", config.DefaultLambdaStubsLibrary},
			systemModules:    "core-current-stubs-system-modules",
			noFrameworksLibs: true,
		}
	case android.SdkModule:
		// TODO(146757305): provide .apk and .aidl that have more APIs for modules
		return toModule([]string{"android_module_lib_stubs_current"}, "framework-res", nonUpdatableFrameworkAidlPath(ctx))
	case android.SdkSystemServer:
		// TODO(146757305): provide .apk and .aidl that have more APIs for modules
		return toModule([]string{"android_system_server_stubs_current"}, "framework-res", sdkFrameworkAidlPath(ctx))
	default:
		panic(fmt.Errorf("invalid sdk %q", sdkVersion.Raw))
	}
}

func sdkPreSingletonFactory() android.Singleton {
	return sdkPreSingleton{}
}

type sdkPreSingleton struct{}

func (sdkPreSingleton) GenerateBuildActions(ctx android.SingletonContext) {
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

	ctx.Config().Once(sdkVersionsKey, func() interface{} { return sdkVersions })
}

func LatestSdkVersionInt(ctx android.EarlyModuleContext) int {
	sdkVersions := ctx.Config().Get(sdkVersionsKey).([]int)
	latestSdkVersion := 0
	if len(sdkVersions) > 0 {
		latestSdkVersion = sdkVersions[len(sdkVersions)-1]
	}
	return latestSdkVersion
}

func sdkSingletonFactory() android.Singleton {
	return sdkSingleton{}
}

type sdkSingleton struct{}

func (sdkSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	if ctx.Config().AlwaysUsePrebuiltSdks() {
		return
	}

	createSdkFrameworkAidl(ctx)
	createNonUpdatableFrameworkAidl(ctx)
	createAPIFingerprint(ctx)
}

// Create framework.aidl by extracting anything that implements android.os.Parcelable from the SDK stubs modules.
func createSdkFrameworkAidl(ctx android.SingletonContext) {
	stubsModules := []string{
		"android_stubs_current",
		"android_test_stubs_current",
		"android_system_stubs_current",
	}

	combinedAidl := sdkFrameworkAidlPath(ctx)
	tempPath := tempPathForRestat(ctx, combinedAidl)

	rule := createFrameworkAidl(stubsModules, tempPath, ctx)

	commitChangeForRestat(rule, tempPath, combinedAidl)

	rule.Build("framework_aidl", "generate framework.aidl")
}

// Creates a version of framework.aidl for the non-updatable part of the platform.
func createNonUpdatableFrameworkAidl(ctx android.SingletonContext) {
	stubsModules := []string{"android_module_lib_stubs_current"}

	combinedAidl := nonUpdatableFrameworkAidlPath(ctx)
	tempPath := tempPathForRestat(ctx, combinedAidl)

	rule := createFrameworkAidl(stubsModules, tempPath, ctx)

	commitChangeForRestat(rule, tempPath, combinedAidl)

	rule.Build("framework_non_updatable_aidl", "generate framework_non_updatable.aidl")
}

func createFrameworkAidl(stubsModules []string, path android.WritablePath, ctx android.SingletonContext) *android.RuleBuilder {
	stubsJars := make([]android.Paths, len(stubsModules))

	ctx.VisitAllModules(func(module android.Module) {
		// Collect dex jar paths for the modules listed above.
		if ctx.ModuleHasProvider(module, JavaInfoProvider) {
			j := ctx.ModuleProvider(module, JavaInfoProvider).(JavaInfo)
			name := ctx.ModuleName(module)
			if i := android.IndexList(name, stubsModules); i != -1 {
				stubsJars[i] = j.HeaderJars
			}
		}
	})

	var missingDeps []string

	for i := range stubsJars {
		if stubsJars[i] == nil {
			if ctx.Config().AllowMissingDependencies() {
				missingDeps = append(missingDeps, stubsModules[i])
			} else {
				ctx.Errorf("failed to find dex jar path for module %q", stubsModules[i])
			}
		}
	}

	rule := android.NewRuleBuilder(pctx, ctx)
	rule.MissingDeps(missingDeps)

	var aidls android.Paths
	for _, jars := range stubsJars {
		for _, jar := range jars {
			aidl := android.PathForOutput(ctx, "aidl", pathtools.ReplaceExtension(jar.Base(), "aidl"))

			rule.Command().
				Text("rm -f").Output(aidl)
			rule.Command().
				BuiltTool("sdkparcelables").
				Input(jar).
				Output(aidl)

			aidls = append(aidls, aidl)
		}
	}

	rule.Command().
		Text("rm -f").Output(path)
	rule.Command().
		Text("cat").
		Inputs(aidls).
		Text("| sort -u >").
		Output(path)

	return rule
}

func sdkFrameworkAidlPath(ctx android.PathContext) android.OutputPath {
	return ctx.Config().Once(sdkFrameworkAidlPathKey, func() interface{} {
		return android.PathForOutput(ctx, "framework.aidl")
	}).(android.OutputPath)
}

func nonUpdatableFrameworkAidlPath(ctx android.PathContext) android.OutputPath {
	return ctx.Config().Once(nonUpdatableFrameworkAidlPathKey, func() interface{} {
		return android.PathForOutput(ctx, "framework_non_updatable.aidl")
	}).(android.OutputPath)
}

// Create api_fingerprint.txt
func createAPIFingerprint(ctx android.SingletonContext) {
	out := ApiFingerprintPath(ctx)

	rule := android.NewRuleBuilder(pctx, ctx)

	rule.Command().
		Text("rm -f").Output(out)
	cmd := rule.Command()

	if ctx.Config().PlatformSdkCodename() == "REL" {
		cmd.Text("echo REL >").Output(out)
	} else if ctx.Config().FrameworksBaseDirExists(ctx) && !ctx.Config().AlwaysUsePrebuiltSdks() {
		cmd.Text("cat")
		apiTxtFileModules := []string{
			"frameworks-base-api-current.txt",
			"frameworks-base-api-system-current.txt",
			"frameworks-base-api-module-lib-current.txt",
			"services-system-server-current.txt",
		}
		count := 0
		ctx.VisitAllModules(func(module android.Module) {
			name := ctx.ModuleName(module)
			if android.InList(name, apiTxtFileModules) {
				cmd.Inputs(android.OutputFilesForModule(ctx, module, ""))
				count++
			}
		})
		if count != len(apiTxtFileModules) {
			ctx.Errorf("Could not find all the expected API modules %v, found %d\n", apiTxtFileModules, count)
			return
		}
		cmd.Text("| md5sum | cut -d' ' -f1 >").
			Output(out)
	} else {
		// Unbundled build
		// TODO: use a prebuilt api_fingerprint.txt from prebuilts/sdk/current.txt once we have one
		cmd.Text("echo").
			Flag(ctx.Config().PlatformPreviewSdkVersion()).
			Text(">").
			Output(out)
	}

	rule.Build("api_fingerprint", "generate api_fingerprint.txt")
}

func ApiFingerprintPath(ctx android.PathContext) android.OutputPath {
	return ctx.Config().Once(apiFingerprintPathKey, func() interface{} {
		return android.PathForOutput(ctx, "api_fingerprint.txt")
	}).(android.OutputPath)
}

func sdkMakeVars(ctx android.MakeVarsContext) {
	if ctx.Config().AlwaysUsePrebuiltSdks() {
		return
	}

	ctx.Strict("FRAMEWORK_AIDL", sdkFrameworkAidlPath(ctx).String())
	ctx.Strict("API_FINGERPRINT", ApiFingerprintPath(ctx).String())
}
