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

	"android/soong/android"
	"android/soong/java/config"

	"github.com/google/blueprint/pathtools"
)

func init() {
	android.RegisterParallelSingletonType("sdk", sdkSingletonFactory)
	android.RegisterMakeVarsProvider(pctx, sdkMakeVars)
}

var sdkFrameworkAidlPathKey = android.NewOnceKey("sdkFrameworkAidlPathKey")
var nonUpdatableFrameworkAidlPathKey = android.NewOnceKey("nonUpdatableFrameworkAidlPathKey")
var apiFingerprintPathKey = android.NewOnceKey("apiFingerprintPathKey")

func UseApiFingerprint(ctx android.BaseModuleContext) (useApiFingerprint bool, fingerprintSdkVersion string, fingerprintDeps android.OutputPath) {
	if ctx.Config().UnbundledBuild() && !ctx.Config().AlwaysUsePrebuiltSdks() {
		apiFingerprintTrue := ctx.Config().IsEnvTrue("UNBUNDLED_BUILD_TARGET_SDK_WITH_API_FINGERPRINT")
		dessertShaIsSet := ctx.Config().Getenv("UNBUNDLED_BUILD_TARGET_SDK_WITH_DESSERT_SHA") != ""

		// Error when both UNBUNDLED_BUILD_TARGET_SDK_WITH_API_FINGERPRINT and UNBUNDLED_BUILD_TARGET_SDK_WITH_DESSERT_SHA are set
		if apiFingerprintTrue && dessertShaIsSet {
			ctx.ModuleErrorf("UNBUNDLED_BUILD_TARGET_SDK_WITH_API_FINGERPRINT=true cannot be set alongside with UNBUNDLED_BUILD_TARGET_SDK_WITH_DESSERT_SHA")
		}

		useApiFingerprint = apiFingerprintTrue || dessertShaIsSet
		if apiFingerprintTrue {
			fingerprintSdkVersion = ctx.Config().PlatformSdkCodename() + fmt.Sprintf(".$$(cat %s)", ApiFingerprintPath(ctx).String())
			fingerprintDeps = ApiFingerprintPath(ctx)
		}
		if dessertShaIsSet {
			fingerprintSdkVersion = ctx.Config().Getenv("UNBUNDLED_BUILD_TARGET_SDK_WITH_DESSERT_SHA")
		}
	}
	return useApiFingerprint, fingerprintSdkVersion, fingerprintDeps
}

func defaultJavaLanguageVersion(ctx android.EarlyModuleContext, s android.SdkSpec) javaVersion {
	sdk, err := s.EffectiveVersion(ctx)
	if err != nil {
		ctx.PropertyErrorf("sdk_version", "%s", err)
	}
	if sdk.FinalOrFutureInt() <= 29 {
		return JAVA_VERSION_8
	} else if sdk.FinalOrFutureInt() <= 31 {
		return JAVA_VERSION_9
	} else if sdk.FinalOrFutureInt() <= 33 {
		return JAVA_VERSION_11
	} else {
		return JAVA_VERSION_17
	}
}

// systemModuleKind returns the kind of system modules to use for the supplied combination of sdk
// kind and API level.
func systemModuleKind(sdkKind android.SdkKind, apiLevel android.ApiLevel) android.SdkKind {
	systemModuleKind := sdkKind
	if apiLevel.LessThanOrEqualTo(android.LastWithoutModuleLibCoreSystemModules) {
		// API levels less than or equal to 31 did not provide a core-for-system-modules.jar
		// specifically for the module-lib API. So, always use the public system modules for them.
		systemModuleKind = android.SdkPublic
	} else if systemModuleKind == android.SdkCore {
		// Core is by definition what is included in the system module for the public API so should
		// just use its system modules.
		systemModuleKind = android.SdkPublic
	} else if systemModuleKind == android.SdkSystem || systemModuleKind == android.SdkTest ||
		systemModuleKind == android.SdkTestFrameworksCore {
		// The core system and test APIs are currently the same as the public API so they should use
		// its system modules.
		systemModuleKind = android.SdkPublic
	} else if systemModuleKind == android.SdkSystemServer {
		// The core system server API is the same as the core module-lib API.
		systemModuleKind = android.SdkModule
	}

	return systemModuleKind
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
			systemModuleKind := systemModuleKind(sdkVersion.Kind, sdkVersion.ApiLevel)
			systemModules = fmt.Sprintf("sdk_%s_%s_system_modules", systemModuleKind, sdkVersion.ApiLevel)
		}

		return sdkDep{
			useFiles:      true,
			jars:          android.Paths{jarPath.Path(), lambdaStubsPath},
			aidl:          android.OptionalPathForPath(aidlPath.Path()),
			systemModules: systemModules,
		}
	}

	toModule := func(module string, aidl android.Path) sdkDep {
		// Select the kind of system modules needed for the sdk version.
		systemModulesKind := systemModuleKind(sdkVersion.Kind, android.FutureApiLevel)
		systemModules := fmt.Sprintf("core-%s-stubs-system-modules", systemModulesKind)
		return sdkDep{
			useModule:          true,
			bootclasspath:      []string{module, config.DefaultLambdaStubsLibrary},
			systemModules:      systemModules,
			java9Classpath:     []string{module},
			frameworkResModule: "framework-res",
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
	case android.SdkPublic, android.SdkSystem, android.SdkTest, android.SdkTestFrameworksCore:
		return toModule(sdkVersion.Kind.DefaultJavaLibraryName(), sdkFrameworkAidlPath(ctx))
	case android.SdkCore:
		return sdkDep{
			useModule:        true,
			bootclasspath:    []string{android.SdkCore.DefaultJavaLibraryName(), config.DefaultLambdaStubsLibrary},
			systemModules:    "core-public-stubs-system-modules",
			noFrameworksLibs: true,
		}
	case android.SdkModule:
		// TODO(146757305): provide .apk and .aidl that have more APIs for modules
		return toModule(sdkVersion.Kind.DefaultJavaLibraryName(), nonUpdatableFrameworkAidlPath(ctx))
	case android.SdkSystemServer:
		// TODO(146757305): provide .apk and .aidl that have more APIs for modules
		return toModule(sdkVersion.Kind.DefaultJavaLibraryName(), sdkFrameworkAidlPath(ctx))
	default:
		panic(fmt.Errorf("invalid sdk %q", sdkVersion.Raw))
	}
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
		android.SdkPublic.DefaultJavaLibraryName(),
		android.SdkTest.DefaultJavaLibraryName(),
		android.SdkSystem.DefaultJavaLibraryName(),
	}

	combinedAidl := sdkFrameworkAidlPath(ctx)
	tempPath := tempPathForRestat(ctx, combinedAidl)

	rule := createFrameworkAidl(stubsModules, tempPath, ctx)

	commitChangeForRestat(rule, tempPath, combinedAidl)

	rule.Build("framework_aidl", "generate framework.aidl")
}

// Creates a version of framework.aidl for the non-updatable part of the platform.
func createNonUpdatableFrameworkAidl(ctx android.SingletonContext) {
	stubsModules := []string{android.SdkModule.DefaultJavaLibraryName()}

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
		if j, ok := android.SingletonModuleProvider(ctx, module, JavaInfoProvider); ok {
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
			"api_fingerprint",
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
			ctx.Errorf("Could not find expected API module %v, found %d\n", apiTxtFileModules, count)
			return
		}
		cmd.Text(">").
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
