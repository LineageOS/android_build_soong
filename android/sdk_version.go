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
	"strconv"
	"strings"
)

type SdkContext interface {
	// SdkVersion returns SdkSpec that corresponds to the sdk_version property of the current module
	SdkVersion() SdkSpec
	// SystemModules returns the system_modules property of the current module, or an empty string if it is not set.
	SystemModules() string
	// MinSdkVersion returns SdkSpec that corresponds to the min_sdk_version property of the current module,
	// or from sdk_version if it is not set.
	MinSdkVersion() SdkSpec
	// TargetSdkVersion returns the SdkSpec that corresponds to the target_sdk_version property of the current module,
	// or from sdk_version if it is not set.
	TargetSdkVersion() SdkSpec
}

// SdkKind represents a particular category of an SDK spec like public, system, test, etc.
type SdkKind int

const (
	SdkInvalid SdkKind = iota
	SdkNone
	SdkCore
	SdkCorePlatform
	SdkPublic
	SdkSystem
	SdkTest
	SdkModule
	SdkSystemServer
	SdkPrivate
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
	case SdkCore:
		return "core"
	case SdkCorePlatform:
		return "core_platform"
	case SdkModule:
		return "module-lib"
	case SdkSystemServer:
		return "system-server"
	default:
		return "invalid"
	}
}

// SdkVersion represents a specific version number of an SDK spec of a particular kind
type SdkVersion int

const (
	// special version number for a not-yet-frozen SDK
	SdkVersionCurrent SdkVersion = SdkVersion(FutureApiLevelInt)
	// special version number to be used for SDK specs where version number doesn't
	// make sense, e.g. "none", "", etc.
	SdkVersionNone SdkVersion = SdkVersion(0)
)

// IsCurrent checks if the SdkVersion refers to the not-yet-published version of an SdkKind
func (v SdkVersion) IsCurrent() bool {
	return v == SdkVersionCurrent
}

// IsNumbered checks if the SdkVersion refers to the published (a.k.a numbered) version of an SdkKind
func (v SdkVersion) IsNumbered() bool {
	return !v.IsCurrent() && v != SdkVersionNone
}

// String returns the string representation of this SdkVersion.
func (v SdkVersion) String() string {
	if v.IsCurrent() {
		return "current"
	} else if v.IsNumbered() {
		return strconv.Itoa(int(v))
	}
	return "(no version)"
}

func (v SdkVersion) ApiLevel(ctx EarlyModuleContext) ApiLevel {
	return ApiLevelOrPanic(ctx, v.String())
}

// AsNumberString directly converts the numeric value of this sdk version as a string.
// When isNumbered() is true, this method is the same as String(). However, for SdkVersionCurrent
// and SdkVersionNone, this returns 10000 and 0 while String() returns "current" and "(no version"),
// respectively.
func (v SdkVersion) AsNumberString() string {
	return strconv.Itoa(int(v))
}

// SdkSpec represents the kind and the version of an SDK for a module to build against
type SdkSpec struct {
	Kind    SdkKind
	Version SdkVersion
	Raw     string
}

func (s SdkSpec) String() string {
	return fmt.Sprintf("%s_%s", s.Kind, s.Version)
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
	case SdkCorePlatform, SdkTest, SdkPrivate:
		return false
	default:
		panic(fmt.Errorf("unknown SdkKind=%v", s.Kind))
	}
	return false
}

// PrebuiltSdkAvailableForUnbundledBuilt tells whether this SdkSpec can have a prebuilt SDK
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
	if currentSdkVersion == "current" {
		return s
	}

	if s.Kind == SdkPublic || s.Kind == SdkSystem {
		if s.Version.IsCurrent() {
			if i, err := strconv.Atoi(currentSdkVersion); err == nil {
				version := SdkVersion(i)
				return SdkSpec{s.Kind, version, s.Raw}
			}
			panic(fmt.Errorf("BOARD_CURRENT_API_LEVEL_FOR_VENDOR_MODULES must be either \"current\" or a number, but was %q", currentSdkVersion))
		}
	}
	return s
}

// UsePrebuilt determines whether prebuilt SDK should be used for this SdkSpec with the given context.
func (s SdkSpec) UsePrebuilt(ctx EarlyModuleContext) bool {
	if s.Version.IsCurrent() {
		// "current" can be built from source and be from prebuilt SDK
		return ctx.Config().AlwaysUsePrebuiltSdks()
	} else if s.Version.IsNumbered() {
		// validation check
		if s.Kind != SdkPublic && s.Kind != SdkSystem && s.Kind != SdkTest && s.Kind != SdkModule {
			panic(fmt.Errorf("prebuilt SDK is not not available for SdkKind=%q", s.Kind))
			return false
		}
		// numbered SDKs are always from prebuilt
		return true
	}
	// "", "none", "core_platform" fall here
	return false
}

// EffectiveVersion converts an SdkSpec into the concrete SdkVersion that the module
// should use. For modules targeting an unreleased SDK (meaning it does not yet have a number)
// it returns FutureApiLevel(10000).
func (s SdkSpec) EffectiveVersion(ctx EarlyModuleContext) (SdkVersion, error) {
	if !s.Valid() {
		return s.Version, fmt.Errorf("invalid sdk version %q", s.Raw)
	}

	if ctx.DeviceSpecific() || ctx.SocSpecific() {
		s = s.ForVendorPartition(ctx)
	}
	if s.Version.IsNumbered() {
		return s.Version, nil
	}
	return SdkVersion(ctx.Config().DefaultAppTargetSdk(ctx).FinalOrFutureInt()), nil
}

// EffectiveVersionString converts an SdkSpec into the concrete version string that the module
// should use. For modules targeting an unreleased SDK (meaning it does not yet have a number)
// it returns the codename (P, Q, R, etc.)
func (s SdkSpec) EffectiveVersionString(ctx EarlyModuleContext) (string, error) {
	ver, err := s.EffectiveVersion(ctx)
	if err == nil && int(ver) == ctx.Config().DefaultAppTargetSdk(ctx).FinalOrFutureInt() {
		return ctx.Config().DefaultAppTargetSdk(ctx).String(), nil
	}
	return ver.String(), err
}

func SdkSpecFrom(str string) SdkSpec {
	switch str {
	// special cases first
	case "":
		return SdkSpec{SdkPrivate, SdkVersionNone, str}
	case "none":
		return SdkSpec{SdkNone, SdkVersionNone, str}
	case "core_platform":
		return SdkSpec{SdkCorePlatform, SdkVersionNone, str}
	default:
		// the syntax is [kind_]version
		sep := strings.LastIndex(str, "_")

		var kindString string
		if sep == 0 {
			return SdkSpec{SdkInvalid, SdkVersionNone, str}
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
		case "module":
			kind = SdkModule
		case "system_server":
			kind = SdkSystemServer
		default:
			return SdkSpec{SdkInvalid, SdkVersionNone, str}
		}

		var version SdkVersion
		if versionString == "current" {
			version = SdkVersionCurrent
		} else if i, err := strconv.Atoi(versionString); err == nil {
			version = SdkVersion(i)
		} else {
			return SdkSpec{SdkInvalid, SdkVersionNone, str}
		}

		return SdkSpec{kind, version, str}
	}
}

func (s SdkSpec) ValidateSystemSdk(ctx EarlyModuleContext) bool {
	// Ensures that the specified system SDK version is one of BOARD_SYSTEMSDK_VERSIONS (for vendor/product Java module)
	// Assuming that BOARD_SYSTEMSDK_VERSIONS := 28 29,
	// sdk_version of the modules in vendor/product that use system sdk must be either system_28, system_29 or system_current
	if s.Kind != SdkSystem || !s.Version.IsNumbered() {
		return true
	}
	allowedVersions := ctx.DeviceConfig().PlatformSystemSdkVersions()
	if ctx.DeviceSpecific() || ctx.SocSpecific() || (ctx.ProductSpecific() && ctx.Config().EnforceProductPartitionInterface()) {
		systemSdkVersions := ctx.DeviceConfig().SystemSdkVersions()
		if len(systemSdkVersions) > 0 {
			allowedVersions = systemSdkVersions
		}
	}
	if len(allowedVersions) > 0 && !InList(s.Version.String(), allowedVersions) {
		ctx.PropertyErrorf("sdk_version", "incompatible sdk version %q. System SDK version should be one of %q",
			s.Raw, allowedVersions)
		return false
	}
	return true
}
