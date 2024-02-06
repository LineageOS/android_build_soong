// Copyright 2021 Google Inc. All rights reserved.
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
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type SdkContext interface {
	// SdkVersion returns SdkSpec that corresponds to the sdk_version property of the current module
	SdkVersion(ctx EarlyModuleContext) SdkSpec
	// SystemModules returns the system_modules property of the current module, or an empty string if it is not set.
	SystemModules() string
	// MinSdkVersion returns ApiLevel that corresponds to the min_sdk_version property of the current module,
	// or from sdk_version if it is not set.
	MinSdkVersion(ctx EarlyModuleContext) ApiLevel
	// ReplaceMaxSdkVersionPlaceholder returns Apilevel to replace the maxSdkVersion property of permission and
	// uses-permission tags if it is set.
	ReplaceMaxSdkVersionPlaceholder(ctx EarlyModuleContext) ApiLevel
	// TargetSdkVersion returns the ApiLevel that corresponds to the target_sdk_version property of the current module,
	// or from sdk_version if it is not set.
	TargetSdkVersion(ctx EarlyModuleContext) ApiLevel
}

// SdkKind represents a particular category of an SDK spec like public, system, test, etc.
type SdkKind int

const (
	SdkInvalid SdkKind = iota
	SdkNone
	SdkCore
	SdkCorePlatform
	SdkIntraCore // API surface provided by one core module to another
	SdkPublic
	SdkSystem
	SdkTest
	SdkTestFrameworksCore
	SdkModule
	SdkSystemServer
	SdkPrivate
	SdkToolchain // API surface provided by ART to compile other API domains
)

// String returns the string representation of this SdkKind
func (k SdkKind) String() string {
	switch k {
	case SdkPrivate:
		return "private"
	case SdkNone:
		return "none"
	case SdkPublic:
		return "public"
	case SdkSystem:
		return "system"
	case SdkTest:
		return "test"
	case SdkTestFrameworksCore:
		return "test_frameworks_core"
	case SdkCore:
		return "core"
	case SdkCorePlatform:
		return "core_platform"
	case SdkIntraCore:
		return "intracore"
	case SdkModule:
		return "module-lib"
	case SdkSystemServer:
		return "system-server"
	case SdkToolchain:
		return "toolchain"
	default:
		return "invalid"
	}
}

func (k SdkKind) DefaultJavaLibraryName() string {
	switch k {
	case SdkPublic:
		return "android_stubs_current"
	case SdkSystem:
		return "android_system_stubs_current"
	case SdkTest:
		return "android_test_stubs_current"
	case SdkTestFrameworksCore:
		return "android_test_frameworks_core_stubs_current"
	case SdkCore:
		return "core.current.stubs"
	case SdkModule:
		return "android_module_lib_stubs_current"
	case SdkSystemServer:
		return "android_system_server_stubs_current"
	default:
		panic(fmt.Errorf("APIs of API surface %v cannot be provided by a single Soong module\n", k))
	}
}

func (k SdkKind) DefaultExportableJavaLibraryName() string {
	switch k {
	case SdkPublic, SdkSystem, SdkTest, SdkModule, SdkSystemServer:
		return k.DefaultJavaLibraryName() + "_exportable"
	case SdkCore:
		return k.DefaultJavaLibraryName() + ".exportable"
	default:
		panic(fmt.Errorf("API surface %v does not provide exportable stubs", k))
	}
}

// SdkSpec represents the kind and the version of an SDK for a module to build against
type SdkSpec struct {
	Kind     SdkKind
	ApiLevel ApiLevel
	Raw      string
}

func (s SdkSpec) String() string {
	return fmt.Sprintf("%s_%s", s.Kind, s.ApiLevel)
}

// Valid checks if this SdkSpec is well-formed. Note however that true doesn't mean that the
// specified SDK actually exists.
func (s SdkSpec) Valid() bool {
	return s.Kind != SdkInvalid
}

// Specified checks if this SdkSpec is well-formed and is not "".
func (s SdkSpec) Specified() bool {
	return s.Valid() && s.Kind != SdkPrivate
}

// whether the API surface is managed and versioned, i.e. has .txt file that
// get frozen on SDK freeze and changes get reviewed by API council.
func (s SdkSpec) Stable() bool {
	if !s.Specified() {
		return false
	}
	switch s.Kind {
	case SdkNone:
		// there is nothing to manage and version in this case; de facto stable API.
		return true
	case SdkCore, SdkPublic, SdkSystem, SdkModule, SdkSystemServer:
		return true
	case SdkCorePlatform, SdkTest, SdkTestFrameworksCore, SdkPrivate:
		return false
	default:
		panic(fmt.Errorf("unknown SdkKind=%v", s.Kind))
	}
	return false
}

// PrebuiltSdkAvailableForUnbundledBuild tells whether this SdkSpec can have a prebuilt SDK
// that can be used for unbundled builds.
func (s SdkSpec) PrebuiltSdkAvailableForUnbundledBuild() bool {
	// "", "none", and "core_platform" are not available for unbundled build
	// as we don't/can't have prebuilt stub for the versions
	return s.Kind != SdkPrivate && s.Kind != SdkNone && s.Kind != SdkCorePlatform
}

func (s SdkSpec) ForVendorPartition(ctx EarlyModuleContext) SdkSpec {
	// If BOARD_CURRENT_API_LEVEL_FOR_VENDOR_MODULES has a numeric value,
	// use it instead of "current" for the vendor partition.
	currentSdkVersion := ctx.DeviceConfig().CurrentApiLevelForVendorModules()
	// b/314011075: special case for Java modules in vendor partition. They can no longer use
	// SDK 35 or later. Their maximum API level is limited to 34 (Android U). This is to
	// discourage the use of Java APIs in the vendor partition which hasn't been officially
	// supported since the Project Treble back in Android 10. We would like to eventually
	// evacuate all Java modules from the partition, but that shall be done progressively.
	// Note that the check for the availability of SDK 34 is to not break existing tests where
	// any of the frozen SDK version is unavailable.
	if isJava(ctx.Module()) && isSdkVersion34AvailableIn(ctx.Config()) {
		currentSdkVersion = "34"
	}

	if currentSdkVersion == "current" {
		return s
	}

	if s.Kind == SdkPublic || s.Kind == SdkSystem {
		if s.ApiLevel.IsCurrent() {
			if i, err := strconv.Atoi(currentSdkVersion); err == nil {
				apiLevel := uncheckedFinalApiLevel(i)
				return SdkSpec{s.Kind, apiLevel, s.Raw}
			}
			panic(fmt.Errorf("BOARD_CURRENT_API_LEVEL_FOR_VENDOR_MODULES must be either \"current\" or a number, but was %q", currentSdkVersion))
		}
	}
	return s
}

// UsePrebuilt determines whether prebuilt SDK should be used for this SdkSpec with the given context.
func (s SdkSpec) UsePrebuilt(ctx EarlyModuleContext) bool {
	switch s {
	case SdkSpecNone, SdkSpecCorePlatform, SdkSpecPrivate:
		return false
	}

	if s.ApiLevel.IsCurrent() {
		// "current" can be built from source and be from prebuilt SDK
		return ctx.Config().AlwaysUsePrebuiltSdks()
	} else if !s.ApiLevel.IsPreview() {
		// validation check
		if s.Kind != SdkPublic && s.Kind != SdkSystem && s.Kind != SdkTest &&
			s.Kind != SdkTestFrameworksCore && s.Kind != SdkModule && s.Kind != SdkSystemServer {
			panic(fmt.Errorf("prebuilt SDK is not not available for SdkKind=%q", s.Kind))
			return false
		}
		// numbered SDKs are always from prebuilt
		return true
	}
	return false
}

// EffectiveVersion converts an SdkSpec into the concrete ApiLevel that the module should use. For
// modules targeting an unreleased SDK (meaning it does not yet have a number) it returns
// FutureApiLevel(10000).
func (s SdkSpec) EffectiveVersion(ctx EarlyModuleContext) (ApiLevel, error) {
	if !s.Valid() {
		return s.ApiLevel, fmt.Errorf("invalid sdk version %q", s.Raw)
	}

	if ctx.DeviceSpecific() || ctx.SocSpecific() {
		s = s.ForVendorPartition(ctx)
	}
	return s.ApiLevel.EffectiveVersion(ctx)
}

// EffectiveVersionString converts an SdkSpec into the concrete version string that the module
// should use. For modules targeting an unreleased SDK (meaning it does not yet have a number)
// it returns the codename (P, Q, R, etc.)
func (s SdkSpec) EffectiveVersionString(ctx EarlyModuleContext) (string, error) {
	if !s.Valid() {
		return s.ApiLevel.String(), fmt.Errorf("invalid sdk version %q", s.Raw)
	}

	if ctx.DeviceSpecific() || ctx.SocSpecific() {
		s = s.ForVendorPartition(ctx)
	}
	return s.ApiLevel.EffectiveVersionString(ctx)
}

var (
	SdkSpecNone         = SdkSpec{SdkNone, NoneApiLevel, "(no version)"}
	SdkSpecPrivate      = SdkSpec{SdkPrivate, PrivateApiLevel, ""}
	SdkSpecCorePlatform = SdkSpec{SdkCorePlatform, FutureApiLevel, "core_platform"}
)

func SdkSpecFrom(ctx EarlyModuleContext, str string) SdkSpec {
	return SdkSpecFromWithConfig(ctx.Config(), str)
}

func SdkSpecFromWithConfig(config Config, str string) SdkSpec {
	switch str {
	// special cases first
	case "":
		return SdkSpecPrivate
	case "none":
		return SdkSpecNone
	case "core_platform":
		return SdkSpecCorePlatform
	default:
		// the syntax is [kind_]version
		sep := strings.LastIndex(str, "_")

		var kindString string
		if sep == 0 {
			return SdkSpec{SdkInvalid, NewInvalidApiLevel(str), str}
		} else if sep == -1 {
			kindString = ""
		} else {
			kindString = str[0:sep]
		}
		versionString := str[sep+1 : len(str)]

		var kind SdkKind
		switch kindString {
		case "":
			kind = SdkPublic
		case "core":
			kind = SdkCore
		case "system":
			kind = SdkSystem
		case "test":
			kind = SdkTest
		case "test_frameworks_core":
			kind = SdkTestFrameworksCore
		case "module":
			kind = SdkModule
		case "system_server":
			kind = SdkSystemServer
		default:
			return SdkSpec{SdkInvalid, NoneApiLevel, str}
		}

		apiLevel, err := ApiLevelFromUserWithConfig(config, versionString)
		if err != nil {
			return SdkSpec{SdkInvalid, NewInvalidApiLevel(versionString), str}
		}
		return SdkSpec{kind, apiLevel, str}
	}
}

// Checks if the use of this SDK `s` is valid for the given module context `ctx`.
func (s SdkSpec) ValidateSystemSdk(ctx EarlyModuleContext) bool {
	// Do some early checks. This check is currently only for Java modules. And our only concern
	// is the use of "system" SDKs.
	if !isJava(ctx.Module()) || s.Kind != SdkSystem || ctx.DeviceConfig().BuildBrokenDontCheckSystemSdk() {
		return true
	}

	inVendor := ctx.DeviceSpecific() || ctx.SocSpecific()
	inProduct := ctx.ProductSpecific()
	isProductUnbundled := ctx.Config().EnforceProductPartitionInterface()
	inApex := false
	if am, ok := ctx.Module().(ApexModule); ok {
		inApex = am.InAnyApex()
	}
	isUnbundled := inVendor || (inProduct && isProductUnbundled) || inApex

	// Bundled modules can use any SDK
	if !isUnbundled {
		return true
	}

	// Unbundled modules are allowed to use BOARD_SYSTEMSDK_VERSIONS
	supportedVersions := ctx.DeviceConfig().SystemSdkVersions()

	// b/314011075: special case for vendor modules. Java modules in the vendor partition can
	// not use SDK 35 or later. This is to discourage the use of Java APIs in the vendor
	// partition which hasn't been officially supported since the Project Treble back in Android
	// 10. We would like to eventually evacuate all Java modules from the partition, but that
	// shall be done progressively.
	if inVendor {
		// 28 was the API when BOARD_SYSTEMSDK_VERSIONS was introduced, so that's the oldest
		// we should allow.
		supportedVersions = []string{}
		for v := 28; v <= 34; v++ {
			supportedVersions = append(supportedVersions, strconv.Itoa(v))
		}
	}

	// APEXes in the system partition are still considered as part of the platform, thus can use
	// more SDKs from PLATFORM_SYSTEMSDK_VERSIONS
	if inApex && !inVendor {
		supportedVersions = ctx.DeviceConfig().PlatformSystemSdkVersions()
	}

	thisVer, err := s.EffectiveVersion(ctx)
	if err != nil {
		ctx.PropertyErrorf("sdk_version", "invalid sdk version %q", s.Raw)
		return false
	}

	thisVerString := strconv.Itoa(thisVer.FinalOrPreviewInt())
	if thisVer.IsPreview() {
		thisVerString = *ctx.Config().productVariables.Platform_sdk_version_or_codename
	}

	if !InList(thisVerString, supportedVersions) {
		ctx.PropertyErrorf("sdk_version", "incompatible sdk version %q. System SDK version should be one of %q",
			s.Raw, supportedVersions)
		return false
	}
	return true
}

func isJava(m Module) bool {
	moduleType := reflect.TypeOf(m).String()
	return strings.HasPrefix(moduleType, "*java.")
}

func isSdkVersion34AvailableIn(c Config) bool {
	return c.PlatformSdkVersion().FinalInt() >= 34
}

func init() {
	RegisterMakeVarsProvider(pctx, javaSdkMakeVars)
}

// Export the name of the soong modules representing the various Java API surfaces.
func javaSdkMakeVars(ctx MakeVarsContext) {
	ctx.Strict("ANDROID_PUBLIC_STUBS", SdkPublic.DefaultJavaLibraryName())
	ctx.Strict("ANDROID_PUBLIC_EXPORTABLE_STUBS", SdkPublic.DefaultExportableJavaLibraryName())
	ctx.Strict("ANDROID_SYSTEM_STUBS", SdkSystem.DefaultJavaLibraryName())
	ctx.Strict("ANDROID_TEST_STUBS", SdkTest.DefaultJavaLibraryName())
	ctx.Strict("ANDROID_MODULE_LIB_STUBS", SdkModule.DefaultJavaLibraryName())
	ctx.Strict("ANDROID_SYSTEM_SERVER_STUBS", SdkSystemServer.DefaultJavaLibraryName())
	ctx.Strict("ANDROID_CORE_STUBS", SdkCore.DefaultJavaLibraryName())
}
