// Copyright 2015 Google Inc. All rights reserved.
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
	"encoding"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/proptools"
)

/*
Example blueprints file containing all variant property groups, with comment listing what type
of variants get properties in that group:

module {
    arch: {
        arm: {
            // Host or device variants with arm architecture
        },
        arm64: {
            // Host or device variants with arm64 architecture
        },
        x86: {
            // Host or device variants with x86 architecture
        },
        x86_64: {
            // Host or device variants with x86_64 architecture
        },
    },
    multilib: {
        lib32: {
            // Host or device variants for 32-bit architectures
        },
        lib64: {
            // Host or device variants for 64-bit architectures
        },
    },
    target: {
        android: {
            // Device variants (implies Bionic)
        },
        host: {
            // Host variants
        },
        bionic: {
            // Bionic (device and host) variants
        },
        linux_bionic: {
            // Bionic host variants
        },
        linux: {
            // Bionic (device and host) and Linux glibc variants
        },
        linux_glibc: {
            // Linux host variants (using non-Bionic libc)
        },
        darwin: {
            // Darwin host variants
        },
        windows: {
            // Windows host variants
        },
        not_windows: {
            // Non-windows host variants
        },
        android_arm: {
            // Any <os>_<arch> combination restricts to that os and arch
        },
    },
}
*/

// An Arch indicates a single CPU architecture.
type Arch struct {
	// The type of the architecture (arm, arm64, x86, or x86_64).
	ArchType ArchType

	// The variant of the architecture, for example "armv7-a" or "armv7-a-neon" for arm.
	ArchVariant string

	// The variant of the CPU, for example "cortex-a53" for arm64.
	CpuVariant string

	// The list of Android app ABIs supported by the CPU architecture, for example "arm64-v8a".
	Abi []string

	// The list of arch-specific features supported by the CPU architecture, for example "neon".
	ArchFeatures []string
}

// String returns the Arch as a string.  The value is used as the name of the variant created
// by archMutator.
func (a Arch) String() string {
	s := a.ArchType.String()
	if a.ArchVariant != "" {
		s += "_" + a.ArchVariant
	}
	if a.CpuVariant != "" {
		s += "_" + a.CpuVariant
	}
	return s
}

// ArchType is used to define the 4 supported architecture types (arm, arm64, x86, x86_64), as
// well as the "common" architecture used for modules that support multiple architectures, for
// example Java modules.
type ArchType struct {
	// Name is the name of the architecture type, "arm", "arm64", "x86", or "x86_64".
	Name string

	// Field is the name of the field used in properties that refer to the architecture, e.g. "Arm64".
	Field string

	// Multilib is either "lib32" or "lib64" for 32-bit or 64-bit architectures.
	Multilib string
}

// String returns the name of the ArchType.
func (a ArchType) String() string {
	return a.Name
}

const COMMON_VARIANT = "common"

var (
	archTypeList []ArchType

	Arm    = newArch("arm", "lib32")
	Arm64  = newArch("arm64", "lib64")
	X86    = newArch("x86", "lib32")
	X86_64 = newArch("x86_64", "lib64")

	Common = ArchType{
		Name: COMMON_VARIANT,
	}
)

var archTypeMap = map[string]ArchType{}

func newArch(name, multilib string) ArchType {
	archType := ArchType{
		Name:     name,
		Field:    proptools.FieldNameForProperty(name),
		Multilib: multilib,
	}
	archTypeList = append(archTypeList, archType)
	archTypeMap[name] = archType
	return archType
}

// ArchTypeList returns the a slice copy of the 4 supported ArchTypes for arm,
// arm64, x86 and x86_64.
func ArchTypeList() []ArchType {
	return append([]ArchType(nil), archTypeList...)
}

// MarshalText allows an ArchType to be serialized through any encoder that supports
// encoding.TextMarshaler.
func (a ArchType) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

var _ encoding.TextMarshaler = ArchType{}

// UnmarshalText allows an ArchType to be deserialized through any decoder that supports
// encoding.TextUnmarshaler.
func (a *ArchType) UnmarshalText(text []byte) error {
	if u, ok := archTypeMap[string(text)]; ok {
		*a = u
		return nil
	}

	return fmt.Errorf("unknown ArchType %q", text)
}

var _ encoding.TextUnmarshaler = &ArchType{}

// OsClass is an enum that describes whether a variant of a module runs on the host, on the device,
// or is generic.
type OsClass int

const (
	// Generic is used for variants of modules that are not OS-specific.
	Generic OsClass = iota
	// Device is used for variants of modules that run on the device.
	Device
	// Host is used for variants of modules that run on the host.
	Host
)

// String returns the OsClass as a string.
func (class OsClass) String() string {
	switch class {
	case Generic:
		return "generic"
	case Device:
		return "device"
	case Host:
		return "host"
	default:
		panic(fmt.Errorf("unknown class %d", class))
	}
}

// OsType describes an OS variant of a module.
type OsType struct {
	// Name is the name of the OS.  It is also used as the name of the property in Android.bp
	// files.
	Name string

	// Field is the name of the OS converted to an exported field name, i.e. with the first
	// character capitalized.
	Field string

	// Class is the OsClass of the OS.
	Class OsClass

	// DefaultDisabled is set when the module variants for the OS should not be created unless
	// the module explicitly requests them.  This is used to limit Windows cross compilation to
	// only modules that need it.
	DefaultDisabled bool
}

// String returns the name of the OsType.
func (os OsType) String() string {
	return os.Name
}

// Bionic returns true if the OS uses the Bionic libc runtime, i.e. if the OS is Android or
// is Linux with Bionic.
func (os OsType) Bionic() bool {
	return os == Android || os == LinuxBionic
}

// Linux returns true if the OS uses the Linux kernel, i.e. if the OS is Android or is Linux
// with or without the Bionic libc runtime.
func (os OsType) Linux() bool {
	return os == Android || os == Linux || os == LinuxBionic
}

// newOsType constructs an OsType and adds it to the global lists.
func newOsType(name string, class OsClass, defDisabled bool, archTypes ...ArchType) OsType {
	checkCalledFromInit()
	os := OsType{
		Name:  name,
		Field: proptools.FieldNameForProperty(name),
		Class: class,

		DefaultDisabled: defDisabled,
	}
	osTypeList = append(osTypeList, os)

	if _, found := commonTargetMap[name]; found {
		panic(fmt.Errorf("Found Os type duplicate during OsType registration: %q", name))
	} else {
		commonTargetMap[name] = Target{Os: os, Arch: CommonArch}
	}
	osArchTypeMap[os] = archTypes

	return os
}

// osByName returns the OsType that has the given name, or NoOsType if none match.
func osByName(name string) OsType {
	for _, os := range osTypeList {
		if os.Name == name {
			return os
		}
	}

	return NoOsType
}

// BuildOs returns the OsType for the OS that the build is running on.
var BuildOs = func() OsType {
	switch runtime.GOOS {
	case "linux":
		return Linux
	case "darwin":
		return Darwin
	default:
		panic(fmt.Sprintf("unsupported OS: %s", runtime.GOOS))
	}
}()

// BuildArch returns the ArchType for the CPU that the build is running on.
var BuildArch = func() ArchType {
	switch runtime.GOARCH {
	case "amd64":
		return X86_64
	default:
		panic(fmt.Sprintf("unsupported Arch: %s", runtime.GOARCH))
	}
}()

var (
	// osTypeList contains a list of all the supported OsTypes, including ones not supported
	// by the current build host or the target device.
	osTypeList []OsType
	// commonTargetMap maps names of OsTypes to the corresponding common Target, i.e. the
	// Target with the same OsType and the common ArchType.
	commonTargetMap = make(map[string]Target)
	// osArchTypeMap maps OsTypes to the list of supported ArchTypes for that OS.
	osArchTypeMap = map[OsType][]ArchType{}

	// NoOsType is a placeholder for when no OS is needed.
	NoOsType OsType
	// Linux is the OS for the Linux kernel plus the glibc runtime.
	Linux = newOsType("linux_glibc", Host, false, X86, X86_64)
	// Darwin is the OS for MacOS/Darwin host machines.
	Darwin = newOsType("darwin", Host, false, X86_64)
	// LinuxBionic is the OS for the Linux kernel plus the Bionic libc runtime, but without the
	// rest of Android.
	LinuxBionic = newOsType("linux_bionic", Host, false, Arm64, X86_64)
	// Windows the OS for Windows host machines.
	Windows = newOsType("windows", Host, true, X86, X86_64)
	// Android is the OS for target devices that run all of Android, including the Linux kernel
	// and the Bionic libc runtime.
	Android = newOsType("android", Device, false, Arm, Arm64, X86, X86_64)
	// Fuchsia is the OS for target devices that run Fuchsia.
	Fuchsia = newOsType("fuchsia", Device, false, Arm64, X86_64)

	// CommonOS is a pseudo OSType for a common OS variant, which is OsType agnostic and which
	// has dependencies on all the OS variants.
	CommonOS = newOsType("common_os", Generic, false)

	// CommonArch is the Arch for all modules that are os-specific but not arch specific,
	// for example most Java modules.
	CommonArch = Arch{ArchType: Common}
)

// OsTypeList returns a slice copy of the supported OsTypes.
func OsTypeList() []OsType {
	return append([]OsType(nil), osTypeList...)
}

// Target specifies the OS and architecture that a module is being compiled for.
type Target struct {
	// Os the OS that the module is being compiled for (e.g. "linux_glibc", "android").
	Os OsType
	// Arch is the architecture that the module is being compiled for.
	Arch Arch
	// NativeBridge is NativeBridgeEnabled if the architecture is supported using NativeBridge
	// (i.e. arm on x86) for this device.
	NativeBridge NativeBridgeSupport
	// NativeBridgeHostArchName is the name of the real architecture that is used to implement
	// the NativeBridge architecture.  For example, for arm on x86 this would be "x86".
	NativeBridgeHostArchName string
	// NativeBridgeRelativePath is the name of the subdirectory that will contain NativeBridge
	// libraries and binaries.
	NativeBridgeRelativePath string

	// HostCross is true when the target cannot run natively on the current build host.
	// For example, linux_glibc_x86 returns true on a regular x86/i686/Linux machines, but returns false
	// on Mac (different OS), or on 64-bit only i686/Linux machines (unsupported arch).
	HostCross bool
}

// NativeBridgeSupport is an enum that specifies if a Target supports NativeBridge.
type NativeBridgeSupport bool

const (
	NativeBridgeDisabled NativeBridgeSupport = false
	NativeBridgeEnabled  NativeBridgeSupport = true
)

// String returns the OS and arch variations used for the Target.
func (target Target) String() string {
	return target.OsVariation() + "_" + target.ArchVariation()
}

// OsVariation returns the name of the variation used by the osMutator for the Target.
func (target Target) OsVariation() string {
	return target.Os.String()
}

// ArchVariation returns the name of the variation used by the archMutator for the Target.
func (target Target) ArchVariation() string {
	var variation string
	if target.NativeBridge {
		variation = "native_bridge_"
	}
	variation += target.Arch.String()

	return variation
}

// Variations returns a list of blueprint.Variations for the osMutator and archMutator for the
// Target.
func (target Target) Variations() []blueprint.Variation {
	return []blueprint.Variation{
		{Mutator: "os", Variation: target.OsVariation()},
		{Mutator: "arch", Variation: target.ArchVariation()},
	}
}

func registerBp2buildArchPathDepsMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("bp2build-arch-pathdeps", bp2buildArchPathDepsMutator).Parallel()
}

// add dependencies for architecture specific properties tagged with `android:"path"`
func bp2buildArchPathDepsMutator(ctx BottomUpMutatorContext) {
	var module Module
	module = ctx.Module()

	m := module.base()
	if !m.ArchSpecific() {
		return
	}

	// addPathDepsForProps does not descend into sub structs, so we need to descend into the
	// arch-specific properties ourselves
	properties := []interface{}{}
	for _, archProperties := range m.archProperties {
		for _, archProps := range archProperties {
			archPropValues := reflect.ValueOf(archProps).Elem()
			// there are three "arch" variations, descend into each
			for _, variant := range []string{"Arch", "Multilib", "Target"} {
				// The properties are an interface, get the value (a pointer) that it points to
				archProps := archPropValues.FieldByName(variant).Elem()
				if archProps.IsNil() {
					continue
				}
				// And then a pointer to a struct
				archProps = archProps.Elem()
				for i := 0; i < archProps.NumField(); i += 1 {
					f := archProps.Field(i)
					// If the value of the field is a struct (as opposed to a pointer to a struct) then step
					// into the BlueprintEmbed field.
					if f.Kind() == reflect.Struct {
						f = f.FieldByName("BlueprintEmbed")
					}
					if f.IsZero() {
						continue
					}
					props := f.Interface().(interface{})
					properties = append(properties, props)
				}
			}
		}
	}
	addPathDepsForProps(ctx, properties)
}

// osMutator splits an arch-specific module into a variant for each OS that is enabled for the
// module.  It uses the HostOrDevice value passed to InitAndroidArchModule and the
// device_supported and host_supported properties to determine which OsTypes are enabled for this
// module, then searches through the Targets to determine which have enabled Targets for this
// module.
func osMutator(bpctx blueprint.BottomUpMutatorContext) {
	var module Module
	var ok bool
	if module, ok = bpctx.Module().(Module); !ok {
		// The module is not a Soong module, it is a Blueprint module.
		if bootstrap.IsBootstrapModule(bpctx.Module()) {
			// Bootstrap Go modules are always the build OS or linux bionic.
			config := bpctx.Config().(Config)
			osNames := []string{config.BuildOSTarget.OsVariation()}
			for _, hostCrossTarget := range config.Targets[LinuxBionic] {
				if hostCrossTarget.Arch.ArchType == config.BuildOSTarget.Arch.ArchType {
					osNames = append(osNames, hostCrossTarget.OsVariation())
				}
			}
			osNames = FirstUniqueStrings(osNames)
			bpctx.CreateVariations(osNames...)
		}
		return
	}

	// Bootstrap Go module support above requires this mutator to be a
	// blueprint.BottomUpMutatorContext because android.BottomUpMutatorContext
	// filters out non-Soong modules.  Now that we've handled them, create a
	// normal android.BottomUpMutatorContext.
	mctx := bottomUpMutatorContextFactory(bpctx, module, false, false)

	base := module.base()

	// Nothing to do for modules that are not architecture specific (e.g. a genrule).
	if !base.ArchSpecific() {
		return
	}

	// Collect a list of OSTypes supported by this module based on the HostOrDevice value
	// passed to InitAndroidArchModule and the device_supported and host_supported properties.
	var moduleOSList []OsType
	for _, os := range osTypeList {
		for _, t := range mctx.Config().Targets[os] {
			if base.supportsTarget(t) {
				moduleOSList = append(moduleOSList, os)
				break
			}
		}
	}

	// If there are no supported OSes then disable the module.
	if len(moduleOSList) == 0 {
		base.Disable()
		return
	}

	// Convert the list of supported OsTypes to the variation names.
	osNames := make([]string, len(moduleOSList))
	for i, os := range moduleOSList {
		osNames[i] = os.String()
	}

	createCommonOSVariant := base.commonProperties.CreateCommonOSVariant
	if createCommonOSVariant {
		// A CommonOS variant was requested so add it to the list of OS variants to
		// create. It needs to be added to the end because it needs to depend on the
		// the other variants in the list returned by CreateVariations(...) and inter
		// variant dependencies can only be created from a later variant in that list to
		// an earlier one. That is because variants are always processed in the order in
		// which they are returned from CreateVariations(...).
		osNames = append(osNames, CommonOS.Name)
		moduleOSList = append(moduleOSList, CommonOS)
	}

	// Create the variations, annotate each one with which OS it was created for, and
	// squash the appropriate OS-specific properties into the top level properties.
	modules := mctx.CreateVariations(osNames...)
	for i, m := range modules {
		m.base().commonProperties.CompileOS = moduleOSList[i]
		m.base().setOSProperties(mctx)
	}

	if createCommonOSVariant {
		// A CommonOS variant was requested so add dependencies from it (the last one in
		// the list) to the OS type specific variants.
		last := len(modules) - 1
		commonOSVariant := modules[last]
		commonOSVariant.base().commonProperties.CommonOSVariant = true
		for _, module := range modules[0:last] {
			// Ignore modules that are enabled. Note, this will only avoid adding
			// dependencies on OsType variants that are explicitly disabled in their
			// properties. The CommonOS variant will still depend on disabled variants
			// if they are disabled afterwards, e.g. in archMutator if
			if module.Enabled() {
				mctx.AddInterVariantDependency(commonOsToOsSpecificVariantTag, commonOSVariant, module)
			}
		}
	}
}

type archDepTag struct {
	blueprint.BaseDependencyTag
	name string
}

// Identifies the dependency from CommonOS variant to the os specific variants.
var commonOsToOsSpecificVariantTag = archDepTag{name: "common os to os specific"}

// Get the OsType specific variants for the current CommonOS variant.
//
// The returned list will only contain enabled OsType specific variants of the
// module referenced in the supplied context. An empty list is returned if there
// are no enabled variants or the supplied context is not for an CommonOS
// variant.
func GetOsSpecificVariantsOfCommonOSVariant(mctx BaseModuleContext) []Module {
	var variants []Module
	mctx.VisitDirectDeps(func(m Module) {
		if mctx.OtherModuleDependencyTag(m) == commonOsToOsSpecificVariantTag {
			if m.Enabled() {
				variants = append(variants, m)
			}
		}
	})
	return variants
}

// archMutator splits a module into a variant for each Target requested by the module.  Target selection
// for a module is in three levels, OsClass, multilib, and then Target.
// OsClass selection is determined by:
//    - The HostOrDeviceSupported value passed in to InitAndroidArchModule by the module type factory, which selects
//      whether the module type can compile for host, device or both.
//    - The host_supported and device_supported properties on the module.
// If host is supported for the module, the Host and HostCross OsClasses are selected.  If device is supported
// for the module, the Device OsClass is selected.
// Within each selected OsClass, the multilib selection is determined by:
//    - The compile_multilib property if it set (which may be overridden by target.android.compile_multilib or
//      target.host.compile_multilib).
//    - The default multilib passed to InitAndroidArchModule if compile_multilib was not set.
// Valid multilib values include:
//    "both": compile for all Targets supported by the OsClass (generally x86_64 and x86, or arm64 and arm).
//    "first": compile for only a single preferred Target supported by the OsClass.  This is generally x86_64 or arm64,
//        but may be arm for a 32-bit only build.
//    "32": compile for only a single 32-bit Target supported by the OsClass.
//    "64": compile for only a single 64-bit Target supported by the OsClass.
//    "common": compile a for a single Target that will work on all Targets supported by the OsClass (for example Java).
//    "common_first": compile a for a Target that will work on all Targets supported by the OsClass
//        (same as "common"), plus a second Target for the preferred Target supported by the OsClass
//        (same as "first").  This is used for java_binary that produces a common .jar and a wrapper
//        executable script.
//
// Once the list of Targets is determined, the module is split into a variant for each Target.
//
// Modules can be initialized with InitAndroidMultiTargetsArchModule, in which case they will be split by OsClass,
// but will have a common Target that is expected to handle all other selected Targets via ctx.MultiTargets().
func archMutator(bpctx blueprint.BottomUpMutatorContext) {
	var module Module
	var ok bool
	if module, ok = bpctx.Module().(Module); !ok {
		if bootstrap.IsBootstrapModule(bpctx.Module()) {
			// Bootstrap Go modules are always the build architecture.
			bpctx.CreateVariations(bpctx.Config().(Config).BuildOSTarget.ArchVariation())
		}
		return
	}

	// Bootstrap Go module support above requires this mutator to be a
	// blueprint.BottomUpMutatorContext because android.BottomUpMutatorContext
	// filters out non-Soong modules.  Now that we've handled them, create a
	// normal android.BottomUpMutatorContext.
	mctx := bottomUpMutatorContextFactory(bpctx, module, false, false)

	base := module.base()

	if !base.ArchSpecific() {
		return
	}

	os := base.commonProperties.CompileOS
	if os == CommonOS {
		// Make sure that the target related properties are initialized for the
		// CommonOS variant.
		addTargetProperties(module, commonTargetMap[os.Name], nil, true)

		// Do not create arch specific variants for the CommonOS variant.
		return
	}

	osTargets := mctx.Config().Targets[os]
	image := base.commonProperties.ImageVariation
	// Filter NativeBridge targets unless they are explicitly supported.
	// Skip creating native bridge variants for non-core modules.
	if os == Android &&
		!(Bool(base.commonProperties.Native_bridge_supported) && image == CoreVariation) {

		var targets []Target
		for _, t := range osTargets {
			if !t.NativeBridge {
				targets = append(targets, t)
			}
		}

		osTargets = targets
	}

	// only the primary arch in the ramdisk / vendor_ramdisk / recovery partition
	if os == Android && (module.InstallInRecovery() || module.InstallInRamdisk() || module.InstallInVendorRamdisk() || module.InstallInDebugRamdisk()) {
		osTargets = []Target{osTargets[0]}
	}

	// Windows builds always prefer 32-bit
	prefer32 := os == Windows

	// Determine the multilib selection for this module.
	multilib, extraMultilib := decodeMultilib(base, os.Class)

	// Convert the multilib selection into a list of Targets.
	targets, err := decodeMultilibTargets(multilib, osTargets, prefer32)
	if err != nil {
		mctx.ModuleErrorf("%s", err.Error())
	}

	// If the module is using extraMultilib, decode the extraMultilib selection into
	// a separate list of Targets.
	var multiTargets []Target
	if extraMultilib != "" {
		multiTargets, err = decodeMultilibTargets(extraMultilib, osTargets, prefer32)
		if err != nil {
			mctx.ModuleErrorf("%s", err.Error())
		}
	}

	// Recovery is always the primary architecture, filter out any other architectures.
	// Common arch is also allowed
	if image == RecoveryVariation {
		primaryArch := mctx.Config().DevicePrimaryArchType()
		targets = filterToArch(targets, primaryArch, Common)
		multiTargets = filterToArch(multiTargets, primaryArch, Common)
	}

	// If there are no supported targets disable the module.
	if len(targets) == 0 {
		base.Disable()
		return
	}

	// Convert the targets into a list of arch variation names.
	targetNames := make([]string, len(targets))
	for i, target := range targets {
		targetNames[i] = target.ArchVariation()
	}

	// Create the variations, annotate each one with which Target it was created for, and
	// squash the appropriate arch-specific properties into the top level properties.
	modules := mctx.CreateVariations(targetNames...)
	for i, m := range modules {
		addTargetProperties(m, targets[i], multiTargets, i == 0)
		m.base().setArchProperties(mctx)
	}
}

// addTargetProperties annotates a variant with the Target is is being compiled for, the list
// of additional Targets it is supporting (if any), and whether it is the primary Target for
// the module.
func addTargetProperties(m Module, target Target, multiTargets []Target, primaryTarget bool) {
	m.base().commonProperties.CompileTarget = target
	m.base().commonProperties.CompileMultiTargets = multiTargets
	m.base().commonProperties.CompilePrimary = primaryTarget
}

// decodeMultilib returns the appropriate compile_multilib property for the module, or the default
// multilib from the factory's call to InitAndroidArchModule if none was set.  For modules that
// called InitAndroidMultiTargetsArchModule it always returns "common" for multilib, and returns
// the actual multilib in extraMultilib.
func decodeMultilib(base *ModuleBase, class OsClass) (multilib, extraMultilib string) {
	// First check the "android.compile_multilib" or "host.compile_multilib" properties.
	switch class {
	case Device:
		multilib = String(base.commonProperties.Target.Android.Compile_multilib)
	case Host:
		multilib = String(base.commonProperties.Target.Host.Compile_multilib)
	}

	// If those aren't set, try the "compile_multilib" property.
	if multilib == "" {
		multilib = String(base.commonProperties.Compile_multilib)
	}

	// If that wasn't set, use the default multilib set by the factory.
	if multilib == "" {
		multilib = base.commonProperties.Default_multilib
	}

	if base.commonProperties.UseTargetVariants {
		return multilib, ""
	} else {
		// For app modules a single arch variant will be created per OS class which is expected to handle all the
		// selected arches.  Return the common-type as multilib and any Android.bp provided multilib as extraMultilib
		if multilib == base.commonProperties.Default_multilib {
			multilib = "first"
		}
		return base.commonProperties.Default_multilib, multilib
	}
}

// filterToArch takes a list of Targets and an ArchType, and returns a modified list that contains
// only Targets that have the specified ArchTypes.
func filterToArch(targets []Target, archs ...ArchType) []Target {
	for i := 0; i < len(targets); i++ {
		found := false
		for _, arch := range archs {
			if targets[i].Arch.ArchType == arch {
				found = true
				break
			}
		}
		if !found {
			targets = append(targets[:i], targets[i+1:]...)
			i--
		}
	}
	return targets
}

// archPropRoot is a struct type used as the top level of the arch-specific properties.  It
// contains the "arch", "multilib", and "target" property structs.  It is used to split up the
// property structs to limit how much is allocated when a single arch-specific property group is
// used.  The types are interface{} because they will hold instances of runtime-created types.
type archPropRoot struct {
	Arch, Multilib, Target interface{}
}

// archPropTypeDesc holds the runtime-created types for the property structs to instantiate to
// create an archPropRoot property struct.
type archPropTypeDesc struct {
	arch, multilib, target reflect.Type
}

// createArchPropTypeDesc takes a reflect.Type that is either a struct or a pointer to a struct, and
// returns lists of reflect.Types that contains the arch-variant properties inside structs for each
// arch, multilib and target property.
//
// This is a relatively expensive operation, so the results are cached in the global
// archPropTypeMap.  It is constructed entirely based on compile-time data, so there is no need
// to isolate the results between multiple tests running in parallel.
func createArchPropTypeDesc(props reflect.Type) []archPropTypeDesc {
	// Each property struct shard will be nested many times under the runtime generated arch struct,
	// which can hit the limit of 64kB for the name of runtime generated structs.  They are nested
	// 97 times now, which may grow in the future, plus there is some overhead for the containing
	// type.  This number may need to be reduced if too many are added, but reducing it too far
	// could cause problems if a single deeply nested property no longer fits in the name.
	const maxArchTypeNameSize = 500

	// Convert the type to a new set of types that contains only the arch-specific properties
	// (those that are tagged with `android:"arch_specific"`), and sharded into multiple types
	// to keep the runtime-generated names under the limit.
	propShards, _ := proptools.FilterPropertyStructSharded(props, maxArchTypeNameSize, filterArchStruct)

	// If the type has no arch-specific properties there is nothing to do.
	if len(propShards) == 0 {
		return nil
	}

	var ret []archPropTypeDesc
	for _, props := range propShards {

		// variantFields takes a list of variant property field names and returns a list the
		// StructFields with the names and the type of the current shard.
		variantFields := func(names []string) []reflect.StructField {
			ret := make([]reflect.StructField, len(names))

			for i, name := range names {
				ret[i].Name = name
				ret[i].Type = props
			}

			return ret
		}

		// Create a type that contains the properties in this shard repeated for each
		// architecture, architecture variant, and architecture feature.
		archFields := make([]reflect.StructField, len(archTypeList))
		for i, arch := range archTypeList {
			var variants []string

			for _, archVariant := range archVariants[arch] {
				archVariant := variantReplacer.Replace(archVariant)
				variants = append(variants, proptools.FieldNameForProperty(archVariant))
			}
			for _, feature := range archFeatures[arch] {
				feature := variantReplacer.Replace(feature)
				variants = append(variants, proptools.FieldNameForProperty(feature))
			}

			// Create the StructFields for each architecture variant architecture feature
			// (e.g. "arch.arm.cortex-a53" or "arch.arm.neon").
			fields := variantFields(variants)

			// Create the StructField for the architecture itself (e.g. "arch.arm").  The special
			// "BlueprintEmbed" name is used by Blueprint to put the properties in the
			// parent struct.
			fields = append([]reflect.StructField{{
				Name:      "BlueprintEmbed",
				Type:      props,
				Anonymous: true,
			}}, fields...)

			archFields[i] = reflect.StructField{
				Name: arch.Field,
				Type: reflect.StructOf(fields),
			}
		}

		// Create the type of the "arch" property struct for this shard.
		archType := reflect.StructOf(archFields)

		// Create the type for the "multilib" property struct for this shard, containing the
		// "multilib.lib32" and "multilib.lib64" property structs.
		multilibType := reflect.StructOf(variantFields([]string{"Lib32", "Lib64"}))

		// Start with a list of the special targets
		targets := []string{
			"Host",
			"Android64",
			"Android32",
			"Bionic",
			"Linux",
			"Not_windows",
			"Arm_on_x86",
			"Arm_on_x86_64",
			"Native_bridge",
		}
		for _, os := range osTypeList {
			// Add all the OSes.
			targets = append(targets, os.Field)

			// Add the OS/Arch combinations, e.g. "android_arm64".
			for _, archType := range osArchTypeMap[os] {
				targets = append(targets, os.Field+"_"+archType.Name)

				// Also add the special "linux_<arch>" and "bionic_<arch>" property structs.
				if os.Linux() {
					target := "Linux_" + archType.Name
					if !InList(target, targets) {
						targets = append(targets, target)
					}
				}
				if os.Bionic() {
					target := "Bionic_" + archType.Name
					if !InList(target, targets) {
						targets = append(targets, target)
					}
				}
			}
		}

		// Create the type for the "target" property struct for this shard.
		targetType := reflect.StructOf(variantFields(targets))

		// Return a descriptor of the 3 runtime-created types.
		ret = append(ret, archPropTypeDesc{
			arch:     reflect.PtrTo(archType),
			multilib: reflect.PtrTo(multilibType),
			target:   reflect.PtrTo(targetType),
		})
	}
	return ret
}

// variantReplacer converts architecture variant or architecture feature names into names that
// are valid for an Android.bp file.
var variantReplacer = strings.NewReplacer("-", "_", ".", "_")

// filterArchStruct returns true if the given field is an architecture specific property.
func filterArchStruct(field reflect.StructField, prefix string) (bool, reflect.StructField) {
	if proptools.HasTag(field, "android", "arch_variant") {
		// The arch_variant field isn't necessary past this point
		// Instead of wasting space, just remove it. Go also has a
		// 16-bit limit on structure name length. The name is constructed
		// based on the Go source representation of the structure, so
		// the tag names count towards that length.

		androidTag := field.Tag.Get("android")
		values := strings.Split(androidTag, ",")

		if string(field.Tag) != `android:"`+strings.Join(values, ",")+`"` {
			panic(fmt.Errorf("unexpected tag format %q", field.Tag))
		}
		// don't delete path tag as it is needed for bp2build
		// these tags don't need to be present in the runtime generated struct type.
		values = RemoveListFromList(values, []string{"arch_variant", "variant_prepend"})
		if len(values) > 0 && values[0] != "path" {
			panic(fmt.Errorf("unknown tags %q in field %q", values, prefix+field.Name))
		} else if len(values) == 1 {
			field.Tag = reflect.StructTag(`android:"` + strings.Join(values, ",") + `"`)
		} else {
			field.Tag = ``
		}

		return true, field
	}
	return false, field
}

// archPropTypeMap contains a cache of the results of createArchPropTypeDesc for each type.  It is
// shared across all Contexts, but is constructed based only on compile-time information so there
// is no risk of contaminating one Context with data from another.
var archPropTypeMap OncePer

// initArchModule adds the architecture-specific property structs to a Module.
func initArchModule(m Module) {

	base := m.base()

	// Store the original list of top level property structs
	base.generalProperties = m.GetProperties()

	for _, properties := range base.generalProperties {
		propertiesValue := reflect.ValueOf(properties)
		t := propertiesValue.Type()
		if propertiesValue.Kind() != reflect.Ptr {
			panic(fmt.Errorf("properties must be a pointer to a struct, got %T",
				propertiesValue.Interface()))
		}

		propertiesValue = propertiesValue.Elem()
		if propertiesValue.Kind() != reflect.Struct {
			panic(fmt.Errorf("properties must be a pointer to a struct, got %T",
				propertiesValue.Interface()))
		}

		// Get or create the arch-specific property struct types for this property struct type.
		archPropTypes := archPropTypeMap.Once(NewCustomOnceKey(t), func() interface{} {
			return createArchPropTypeDesc(t)
		}).([]archPropTypeDesc)

		// Instantiate one of each arch-specific property struct type and add it to the
		// properties for the Module.
		var archProperties []interface{}
		for _, t := range archPropTypes {
			archProperties = append(archProperties, &archPropRoot{
				Arch:     reflect.Zero(t.arch).Interface(),
				Multilib: reflect.Zero(t.multilib).Interface(),
				Target:   reflect.Zero(t.target).Interface(),
			})
		}
		base.archProperties = append(base.archProperties, archProperties)
		m.AddProperties(archProperties...)
	}

	// Update the list of properties that can be set by a defaults module or a call to
	// AppendMatchingProperties or PrependMatchingProperties.
	base.customizableProperties = m.GetProperties()
}

func maybeBlueprintEmbed(src reflect.Value) reflect.Value {
	// If the value of the field is a struct (as opposed to a pointer to a struct) then step
	// into the BlueprintEmbed field.
	if src.Kind() == reflect.Struct {
		return src.FieldByName("BlueprintEmbed")
	} else {
		return src
	}
}

// Merges the property struct in srcValue into dst.
func mergePropertyStruct(ctx ArchVariantContext, dst interface{}, srcValue reflect.Value) {
	src := maybeBlueprintEmbed(srcValue).Interface()

	// order checks the `android:"variant_prepend"` tag to handle properties where the
	// arch-specific value needs to come before the generic value, for example for lists of
	// include directories.
	order := func(property string,
		dstField, srcField reflect.StructField,
		dstValue, srcValue interface{}) (proptools.Order, error) {
		if proptools.HasTag(dstField, "android", "variant_prepend") {
			return proptools.Prepend, nil
		} else {
			return proptools.Append, nil
		}
	}

	// Squash the located property struct into the destination property struct.
	err := proptools.ExtendMatchingProperties([]interface{}{dst}, src, nil, order)
	if err != nil {
		if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
			ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
		} else {
			panic(err)
		}
	}
}

// Returns the immediate child of the input property struct that corresponds to
// the sub-property "field".
func getChildPropertyStruct(ctx ArchVariantContext,
	src reflect.Value, field, userFriendlyField string) reflect.Value {

	// Step into non-nil pointers to structs in the src value.
	if src.Kind() == reflect.Ptr {
		if src.IsNil() {
			return src
		}
		src = src.Elem()
	}

	// Find the requested field in the src struct.
	src = src.FieldByName(proptools.FieldNameForProperty(field))
	if !src.IsValid() {
		ctx.ModuleErrorf("field %q does not exist", userFriendlyField)
		return src
	}

	return src
}

// Squash the appropriate OS-specific property structs into the matching top level property structs
// based on the CompileOS value that was annotated on the variant.
func (m *ModuleBase) setOSProperties(ctx BottomUpMutatorContext) {
	os := m.commonProperties.CompileOS

	for i := range m.generalProperties {
		genProps := m.generalProperties[i]
		if m.archProperties[i] == nil {
			continue
		}
		for _, archProperties := range m.archProperties[i] {
			archPropValues := reflect.ValueOf(archProperties).Elem()

			targetProp := archPropValues.FieldByName("Target").Elem()

			// Handle host-specific properties in the form:
			// target: {
			//     host: {
			//         key: value,
			//     },
			// },
			if os.Class == Host {
				field := "Host"
				prefix := "target.host"
				hostProperties := getChildPropertyStruct(ctx, targetProp, field, prefix)
				mergePropertyStruct(ctx, genProps, hostProperties)
			}

			// Handle target OS generalities of the form:
			// target: {
			//     bionic: {
			//         key: value,
			//     },
			// }
			if os.Linux() {
				field := "Linux"
				prefix := "target.linux"
				linuxProperties := getChildPropertyStruct(ctx, targetProp, field, prefix)
				mergePropertyStruct(ctx, genProps, linuxProperties)
			}

			if os.Bionic() {
				field := "Bionic"
				prefix := "target.bionic"
				bionicProperties := getChildPropertyStruct(ctx, targetProp, field, prefix)
				mergePropertyStruct(ctx, genProps, bionicProperties)
			}

			// Handle target OS properties in the form:
			// target: {
			//     linux_glibc: {
			//         key: value,
			//     },
			//     not_windows: {
			//         key: value,
			//     },
			//     android {
			//         key: value,
			//     },
			// },
			field := os.Field
			prefix := "target." + os.Name
			osProperties := getChildPropertyStruct(ctx, targetProp, field, prefix)
			mergePropertyStruct(ctx, genProps, osProperties)

			if os.Class == Host && os != Windows {
				field := "Not_windows"
				prefix := "target.not_windows"
				notWindowsProperties := getChildPropertyStruct(ctx, targetProp, field, prefix)
				mergePropertyStruct(ctx, genProps, notWindowsProperties)
			}

			// Handle 64-bit device properties in the form:
			// target {
			//     android64 {
			//         key: value,
			//     },
			//     android32 {
			//         key: value,
			//     },
			// },
			// WARNING: this is probably not what you want to use in your blueprints file, it selects
			// options for all targets on a device that supports 64-bit binaries, not just the targets
			// that are being compiled for 64-bit.  Its expected use case is binaries like linker and
			// debuggerd that need to know when they are a 32-bit process running on a 64-bit device
			if os.Class == Device {
				if ctx.Config().Android64() {
					field := "Android64"
					prefix := "target.android64"
					android64Properties := getChildPropertyStruct(ctx, targetProp, field, prefix)
					mergePropertyStruct(ctx, genProps, android64Properties)
				} else {
					field := "Android32"
					prefix := "target.android32"
					android32Properties := getChildPropertyStruct(ctx, targetProp, field, prefix)
					mergePropertyStruct(ctx, genProps, android32Properties)
				}
			}
		}
	}
}

// Returns the struct containing the properties specific to the given
// architecture type. These look like this in Blueprint files:
// arch: {
//     arm64: {
//         key: value,
//     },
// },
// This struct will also contain sub-structs containing to the architecture/CPU
// variants and features that themselves contain properties specific to those.
func getArchTypeStruct(ctx ArchVariantContext, archProperties interface{}, archType ArchType) reflect.Value {
	archPropValues := reflect.ValueOf(archProperties).Elem()
	archProp := archPropValues.FieldByName("Arch").Elem()
	prefix := "arch." + archType.Name
	archStruct := getChildPropertyStruct(ctx, archProp, archType.Name, prefix)
	return archStruct
}

// Returns the struct containing the properties specific to a given multilib
// value. These look like this in the Blueprint file:
// multilib: {
//     lib32: {
//         key: value,
//     },
// },
func getMultilibStruct(ctx ArchVariantContext, archProperties interface{}, archType ArchType) reflect.Value {
	archPropValues := reflect.ValueOf(archProperties).Elem()
	multilibProp := archPropValues.FieldByName("Multilib").Elem()
	multilibProperties := getChildPropertyStruct(ctx, multilibProp, archType.Multilib, "multilib."+archType.Multilib)
	return multilibProperties
}

// Returns the structs corresponding to the properties specific to the given
// architecture and OS in archProperties.
func getArchProperties(ctx BaseMutatorContext, archProperties interface{}, arch Arch, os OsType, nativeBridgeEnabled bool) []reflect.Value {
	result := make([]reflect.Value, 0)
	archPropValues := reflect.ValueOf(archProperties).Elem()

	targetProp := archPropValues.FieldByName("Target").Elem()

	archType := arch.ArchType

	if arch.ArchType != Common {
		archStruct := getArchTypeStruct(ctx, archProperties, arch.ArchType)
		result = append(result, archStruct)

		// Handle arch-variant-specific properties in the form:
		// arch: {
		//     arm: {
		//         variant: {
		//             key: value,
		//         },
		//     },
		// },
		v := variantReplacer.Replace(arch.ArchVariant)
		if v != "" {
			prefix := "arch." + archType.Name + "." + v
			variantProperties := getChildPropertyStruct(ctx, archStruct, v, prefix)
			result = append(result, variantProperties)
		}

		// Handle cpu-variant-specific properties in the form:
		// arch: {
		//     arm: {
		//         variant: {
		//             key: value,
		//         },
		//     },
		// },
		if arch.CpuVariant != arch.ArchVariant {
			c := variantReplacer.Replace(arch.CpuVariant)
			if c != "" {
				prefix := "arch." + archType.Name + "." + c
				cpuVariantProperties := getChildPropertyStruct(ctx, archStruct, c, prefix)
				result = append(result, cpuVariantProperties)
			}
		}

		// Handle arch-feature-specific properties in the form:
		// arch: {
		//     arm: {
		//         feature: {
		//             key: value,
		//         },
		//     },
		// },
		for _, feature := range arch.ArchFeatures {
			prefix := "arch." + archType.Name + "." + feature
			featureProperties := getChildPropertyStruct(ctx, archStruct, feature, prefix)
			result = append(result, featureProperties)
		}

		multilibProperties := getMultilibStruct(ctx, archProperties, archType)
		result = append(result, multilibProperties)

		// Handle combined OS-feature and arch specific properties in the form:
		// target: {
		//     bionic_x86: {
		//         key: value,
		//     },
		// }
		if os.Linux() {
			field := "Linux_" + arch.ArchType.Name
			userFriendlyField := "target.linux_" + arch.ArchType.Name
			linuxProperties := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField)
			result = append(result, linuxProperties)
		}

		if os.Bionic() {
			field := "Bionic_" + archType.Name
			userFriendlyField := "target.bionic_" + archType.Name
			bionicProperties := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField)
			result = append(result, bionicProperties)
		}

		// Handle combined OS and arch specific properties in the form:
		// target: {
		//     linux_glibc_x86: {
		//         key: value,
		//     },
		//     linux_glibc_arm: {
		//         key: value,
		//     },
		//     android_arm {
		//         key: value,
		//     },
		//     android_x86 {
		//         key: value,
		//     },
		// },
		field := os.Field + "_" + archType.Name
		userFriendlyField := "target." + os.Name + "_" + archType.Name
		osArchProperties := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField)
		result = append(result, osArchProperties)
	}

	// Handle arm on x86 properties in the form:
	// target {
	//     arm_on_x86 {
	//         key: value,
	//     },
	//     arm_on_x86_64 {
	//         key: value,
	//     },
	// },
	if os.Class == Device {
		if arch.ArchType == X86 && (hasArmAbi(arch) ||
			hasArmAndroidArch(ctx.Config().Targets[Android])) {
			field := "Arm_on_x86"
			userFriendlyField := "target.arm_on_x86"
			armOnX86Properties := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField)
			result = append(result, armOnX86Properties)
		}
		if arch.ArchType == X86_64 && (hasArmAbi(arch) ||
			hasArmAndroidArch(ctx.Config().Targets[Android])) {
			field := "Arm_on_x86_64"
			userFriendlyField := "target.arm_on_x86_64"
			armOnX8664Properties := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField)
			result = append(result, armOnX8664Properties)
		}
		if os == Android && nativeBridgeEnabled {
			userFriendlyField := "Native_bridge"
			prefix := "target.native_bridge"
			nativeBridgeProperties := getChildPropertyStruct(ctx, targetProp, userFriendlyField, prefix)
			result = append(result, nativeBridgeProperties)
		}
	}

	return result
}

// Squash the appropriate arch-specific property structs into the matching top level property
// structs based on the CompileTarget value that was annotated on the variant.
func (m *ModuleBase) setArchProperties(ctx BottomUpMutatorContext) {
	arch := m.Arch()
	os := m.Os()

	for i := range m.generalProperties {
		genProps := m.generalProperties[i]
		if m.archProperties[i] == nil {
			continue
		}

		propStructs := make([]reflect.Value, 0)
		for _, archProperty := range m.archProperties[i] {
			propStructShard := getArchProperties(ctx, archProperty, arch, os, m.Target().NativeBridge == NativeBridgeEnabled)
			propStructs = append(propStructs, propStructShard...)
		}

		for _, propStruct := range propStructs {
			mergePropertyStruct(ctx, genProps, propStruct)
		}
	}
}

// Convert the arch product variables into a list of targets for each OsType.
func decodeTargetProductVariables(config *config) (map[OsType][]Target, error) {
	variables := config.productVariables

	targets := make(map[OsType][]Target)
	var targetErr error

	addTarget := func(os OsType, archName string, archVariant, cpuVariant *string, abi []string,
		nativeBridgeEnabled NativeBridgeSupport, nativeBridgeHostArchName *string,
		nativeBridgeRelativePath *string) {
		if targetErr != nil {
			return
		}

		arch, err := decodeArch(os, archName, archVariant, cpuVariant, abi)
		if err != nil {
			targetErr = err
			return
		}
		nativeBridgeRelativePathStr := String(nativeBridgeRelativePath)
		nativeBridgeHostArchNameStr := String(nativeBridgeHostArchName)

		// Use guest arch as relative install path by default
		if nativeBridgeEnabled && nativeBridgeRelativePathStr == "" {
			nativeBridgeRelativePathStr = arch.ArchType.String()
		}

		// A target is considered as HostCross if it's a host target which can't run natively on
		// the currently configured build machine (either because the OS is different or because of
		// the unsupported arch)
		hostCross := false
		if os.Class == Host {
			var osSupported bool
			if os == BuildOs {
				osSupported = true
			} else if BuildOs.Linux() && os.Linux() {
				// LinuxBionic and Linux are compatible
				osSupported = true
			} else {
				osSupported = false
			}

			var archSupported bool
			if arch.ArchType == Common {
				archSupported = true
			} else if arch.ArchType.Name == *variables.HostArch {
				archSupported = true
			} else if variables.HostSecondaryArch != nil && arch.ArchType.Name == *variables.HostSecondaryArch {
				archSupported = true
			} else {
				archSupported = false
			}
			if !osSupported || !archSupported {
				hostCross = true
			}
		}

		targets[os] = append(targets[os],
			Target{
				Os:                       os,
				Arch:                     arch,
				NativeBridge:             nativeBridgeEnabled,
				NativeBridgeHostArchName: nativeBridgeHostArchNameStr,
				NativeBridgeRelativePath: nativeBridgeRelativePathStr,
				HostCross:                hostCross,
			})
	}

	if variables.HostArch == nil {
		return nil, fmt.Errorf("No host primary architecture set")
	}

	// The primary host target, which must always exist.
	addTarget(BuildOs, *variables.HostArch, nil, nil, nil, NativeBridgeDisabled, nil, nil)

	// An optional secondary host target.
	if variables.HostSecondaryArch != nil && *variables.HostSecondaryArch != "" {
		addTarget(BuildOs, *variables.HostSecondaryArch, nil, nil, nil, NativeBridgeDisabled, nil, nil)
	}

	// Optional cross-compiled host targets, generally Windows.
	if String(variables.CrossHost) != "" {
		crossHostOs := osByName(*variables.CrossHost)
		if crossHostOs == NoOsType {
			return nil, fmt.Errorf("Unknown cross host OS %q", *variables.CrossHost)
		}

		if String(variables.CrossHostArch) == "" {
			return nil, fmt.Errorf("No cross-host primary architecture set")
		}

		// The primary cross-compiled host target.
		addTarget(crossHostOs, *variables.CrossHostArch, nil, nil, nil, NativeBridgeDisabled, nil, nil)

		// An optional secondary cross-compiled host target.
		if variables.CrossHostSecondaryArch != nil && *variables.CrossHostSecondaryArch != "" {
			addTarget(crossHostOs, *variables.CrossHostSecondaryArch, nil, nil, nil, NativeBridgeDisabled, nil, nil)
		}
	}

	// Optional device targets
	if variables.DeviceArch != nil && *variables.DeviceArch != "" {
		var target = Android
		if Bool(variables.Fuchsia) {
			target = Fuchsia
		}

		// The primary device target.
		addTarget(target, *variables.DeviceArch, variables.DeviceArchVariant,
			variables.DeviceCpuVariant, variables.DeviceAbi, NativeBridgeDisabled, nil, nil)

		// An optional secondary device target.
		if variables.DeviceSecondaryArch != nil && *variables.DeviceSecondaryArch != "" {
			addTarget(Android, *variables.DeviceSecondaryArch,
				variables.DeviceSecondaryArchVariant, variables.DeviceSecondaryCpuVariant,
				variables.DeviceSecondaryAbi, NativeBridgeDisabled, nil, nil)
		}

		// An optional NativeBridge device target.
		if variables.NativeBridgeArch != nil && *variables.NativeBridgeArch != "" {
			addTarget(Android, *variables.NativeBridgeArch,
				variables.NativeBridgeArchVariant, variables.NativeBridgeCpuVariant,
				variables.NativeBridgeAbi, NativeBridgeEnabled, variables.DeviceArch,
				variables.NativeBridgeRelativePath)
		}

		// An optional secondary NativeBridge device target.
		if variables.DeviceSecondaryArch != nil && *variables.DeviceSecondaryArch != "" &&
			variables.NativeBridgeSecondaryArch != nil && *variables.NativeBridgeSecondaryArch != "" {
			addTarget(Android, *variables.NativeBridgeSecondaryArch,
				variables.NativeBridgeSecondaryArchVariant,
				variables.NativeBridgeSecondaryCpuVariant,
				variables.NativeBridgeSecondaryAbi,
				NativeBridgeEnabled,
				variables.DeviceSecondaryArch,
				variables.NativeBridgeSecondaryRelativePath)
		}
	}

	if targetErr != nil {
		return nil, targetErr
	}

	return targets, nil
}

// hasArmAbi returns true if arch has at least one arm ABI
func hasArmAbi(arch Arch) bool {
	return PrefixInList(arch.Abi, "arm")
}

// hasArmAndroidArch returns true if targets has at least
// one arm Android arch (possibly native bridged)
func hasArmAndroidArch(targets []Target) bool {
	for _, target := range targets {
		if target.Os == Android &&
			(target.Arch.ArchType == Arm || target.Arch.ArchType == Arm64) {
			return true
		}
	}
	return false
}

// archConfig describes a built-in configuration.
type archConfig struct {
	arch        string
	archVariant string
	cpuVariant  string
	abi         []string
}

// getNdkAbisConfig returns a list of archConfigs for the ABIs supported by the NDK.
func getNdkAbisConfig() []archConfig {
	return []archConfig{
		{"arm", "armv7-a", "", []string{"armeabi-v7a"}},
		{"arm64", "armv8-a-branchprot", "", []string{"arm64-v8a"}},
		{"x86", "", "", []string{"x86"}},
		{"x86_64", "", "", []string{"x86_64"}},
	}
}

// getAmlAbisConfig returns a list of archConfigs for the ABIs supported by mainline modules.
func getAmlAbisConfig() []archConfig {
	return []archConfig{
		{"arm", "armv7-a-neon", "", []string{"armeabi-v7a"}},
		{"arm64", "armv8-a", "", []string{"arm64-v8a"}},
		{"x86", "", "", []string{"x86"}},
		{"x86_64", "", "", []string{"x86_64"}},
	}
}

// decodeArchSettings converts a list of archConfigs into a list of Targets for the given OsType.
func decodeArchSettings(os OsType, archConfigs []archConfig) ([]Target, error) {
	var ret []Target

	for _, config := range archConfigs {
		arch, err := decodeArch(os, config.arch, &config.archVariant,
			&config.cpuVariant, config.abi)
		if err != nil {
			return nil, err
		}

		ret = append(ret, Target{
			Os:   Android,
			Arch: arch,
		})
	}

	return ret, nil
}

// decodeArch converts a set of strings from product variables into an Arch struct.
func decodeArch(os OsType, arch string, archVariant, cpuVariant *string, abi []string) (Arch, error) {
	// Verify the arch is valid
	archType, ok := archTypeMap[arch]
	if !ok {
		return Arch{}, fmt.Errorf("unknown arch %q", arch)
	}

	a := Arch{
		ArchType:    archType,
		ArchVariant: String(archVariant),
		CpuVariant:  String(cpuVariant),
		Abi:         abi,
	}

	// Convert generic arch variants into the empty string.
	if a.ArchVariant == a.ArchType.Name || a.ArchVariant == "generic" {
		a.ArchVariant = ""
	}

	// Convert generic CPU variants into the empty string.
	if a.CpuVariant == a.ArchType.Name || a.CpuVariant == "generic" {
		a.CpuVariant = ""
	}

	// Filter empty ABIs out of the list.
	for i := 0; i < len(a.Abi); i++ {
		if a.Abi[i] == "" {
			a.Abi = append(a.Abi[:i], a.Abi[i+1:]...)
			i--
		}
	}

	if a.ArchVariant == "" {
		// Set ArchFeatures from the default arch features.
		if featureMap, ok := defaultArchFeatureMap[os]; ok {
			a.ArchFeatures = featureMap[archType]
		}
	} else {
		// Set ArchFeatures from the arch type.
		if featureMap, ok := archFeatureMap[archType]; ok {
			a.ArchFeatures = featureMap[a.ArchVariant]
		}
	}

	return a, nil
}

// filterMultilibTargets takes a list of Targets and a multilib value and returns a new list of
// Targets containing only those that have the given multilib value.
func filterMultilibTargets(targets []Target, multilib string) []Target {
	var ret []Target
	for _, t := range targets {
		if t.Arch.ArchType.Multilib == multilib {
			ret = append(ret, t)
		}
	}
	return ret
}

// getCommonTargets returns the set of Os specific common architecture targets for each Os in a list
// of targets.
func getCommonTargets(targets []Target) []Target {
	var ret []Target
	set := make(map[string]bool)

	for _, t := range targets {
		if _, found := set[t.Os.String()]; !found {
			set[t.Os.String()] = true
			ret = append(ret, commonTargetMap[t.Os.String()])
		}
	}

	return ret
}

// firstTarget takes a list of Targets and a list of multilib values and returns a list of Targets
// that contains zero or one Target for each OsType, selecting the one that matches the earliest
// filter.
func firstTarget(targets []Target, filters ...string) []Target {
	// find the first target from each OS
	var ret []Target
	hasHost := false
	set := make(map[OsType]bool)

	for _, filter := range filters {
		buildTargets := filterMultilibTargets(targets, filter)
		for _, t := range buildTargets {
			if _, found := set[t.Os]; !found {
				hasHost = hasHost || (t.Os.Class == Host)
				set[t.Os] = true
				ret = append(ret, t)
			}
		}
	}
	return ret
}

// decodeMultilibTargets uses the module's multilib setting to select one or more targets from a
// list of Targets.
func decodeMultilibTargets(multilib string, targets []Target, prefer32 bool) ([]Target, error) {
	var buildTargets []Target

	switch multilib {
	case "common":
		buildTargets = getCommonTargets(targets)
	case "common_first":
		buildTargets = getCommonTargets(targets)
		if prefer32 {
			buildTargets = append(buildTargets, firstTarget(targets, "lib32", "lib64")...)
		} else {
			buildTargets = append(buildTargets, firstTarget(targets, "lib64", "lib32")...)
		}
	case "both":
		if prefer32 {
			buildTargets = append(buildTargets, filterMultilibTargets(targets, "lib32")...)
			buildTargets = append(buildTargets, filterMultilibTargets(targets, "lib64")...)
		} else {
			buildTargets = append(buildTargets, filterMultilibTargets(targets, "lib64")...)
			buildTargets = append(buildTargets, filterMultilibTargets(targets, "lib32")...)
		}
	case "32":
		buildTargets = filterMultilibTargets(targets, "lib32")
	case "64":
		buildTargets = filterMultilibTargets(targets, "lib64")
	case "first":
		if prefer32 {
			buildTargets = firstTarget(targets, "lib32", "lib64")
		} else {
			buildTargets = firstTarget(targets, "lib64", "lib32")
		}
	case "first_prefer32":
		buildTargets = firstTarget(targets, "lib32", "lib64")
	case "prefer32":
		buildTargets = filterMultilibTargets(targets, "lib32")
		if len(buildTargets) == 0 {
			buildTargets = filterMultilibTargets(targets, "lib64")
		}
	default:
		return nil, fmt.Errorf(`compile_multilib must be "both", "first", "32", "64", "prefer32" or "first_prefer32" found %q`,
			multilib)
	}

	return buildTargets, nil
}

func (m *ModuleBase) getArchPropertySet(propertySet interface{}, archType ArchType) interface{} {
	archString := archType.Field
	for i := range m.archProperties {
		if m.archProperties[i] == nil {
			// Skip over nil properties
			continue
		}

		// Not archProperties are usable; this function looks for properties of a very specific
		// form, and ignores the rest.
		for _, archProperty := range m.archProperties[i] {
			// archPropValue is a property struct, we are looking for the form:
			// `arch: { arm: { key: value, ... }}`
			archPropValue := reflect.ValueOf(archProperty).Elem()

			// Unwrap src so that it should looks like a pointer to `arm: { key: value, ... }`
			src := archPropValue.FieldByName("Arch").Elem()

			// Step into non-nil pointers to structs in the src value.
			if src.Kind() == reflect.Ptr {
				if src.IsNil() {
					continue
				}
				src = src.Elem()
			}

			// Find the requested field (e.g. arm, x86) in the src struct.
			src = src.FieldByName(archString)

			// We only care about structs.
			if !src.IsValid() || src.Kind() != reflect.Struct {
				continue
			}

			// If the value of the field is a struct then step into the
			// BlueprintEmbed field. The special "BlueprintEmbed" name is
			// used by createArchPropTypeDesc to embed the arch properties
			// in the parent struct, so the src arch prop should be in this
			// field.
			//
			// See createArchPropTypeDesc for more details on how Arch-specific
			// module properties are processed from the nested props and written
			// into the module's archProperties.
			src = src.FieldByName("BlueprintEmbed")

			// Clone the destination prop, since we want a unique prop struct per arch.
			propertySetClone := reflect.New(reflect.ValueOf(propertySet).Elem().Type()).Interface()

			// Copy the located property struct into the cloned destination property struct.
			err := proptools.ExtendMatchingProperties([]interface{}{propertySetClone}, src.Interface(), nil, proptools.OrderReplace)
			if err != nil {
				// This is fine, it just means the src struct doesn't match the type of propertySet.
				continue
			}

			return propertySetClone
		}
	}
	// No property set was found specific to the given arch, so return an empty
	// property set.
	return reflect.New(reflect.ValueOf(propertySet).Elem().Type()).Interface()
}

// getMultilibPropertySet returns a property set struct matching the type of
// `propertySet`, containing multilib-specific module properties for the given architecture.
// If no multilib-specific properties exist for the given architecture, returns an empty property
// set matching `propertySet`'s type.
func (m *ModuleBase) getMultilibPropertySet(propertySet interface{}, archType ArchType) interface{} {
	// archType.Multilib is lowercase (for example, lib32) but property struct field is
	// capitalized, such as Lib32, so use strings.Title to capitalize it.
	multiLibString := strings.Title(archType.Multilib)

	for i := range m.archProperties {
		if m.archProperties[i] == nil {
			// Skip over nil properties
			continue
		}

		// Not archProperties are usable; this function looks for properties of a very specific
		// form, and ignores the rest.
		for _, archProperties := range m.archProperties[i] {
			// archPropValue is a property struct, we are looking for the form:
			// `multilib: { lib32: { key: value, ... }}`
			archPropValue := reflect.ValueOf(archProperties).Elem()

			// Unwrap src so that it should looks like a pointer to `lib32: { key: value, ... }`
			src := archPropValue.FieldByName("Multilib").Elem()

			// Step into non-nil pointers to structs in the src value.
			if src.Kind() == reflect.Ptr {
				if src.IsNil() {
					// Ignore nil pointers.
					continue
				}
				src = src.Elem()
			}

			// Find the requested field (e.g. lib32) in the src struct.
			src = src.FieldByName(multiLibString)

			// We only care about valid struct pointers.
			if !src.IsValid() || src.Kind() != reflect.Ptr || src.Elem().Kind() != reflect.Struct {
				continue
			}

			// Get the zero value for the requested property set.
			propertySetClone := reflect.New(reflect.ValueOf(propertySet).Elem().Type()).Interface()

			// Copy the located property struct into the "zero" property set struct.
			err := proptools.ExtendMatchingProperties([]interface{}{propertySetClone}, src.Interface(), nil, proptools.OrderReplace)

			if err != nil {
				// This is fine, it just means the src struct doesn't match.
				continue
			}

			return propertySetClone
		}
	}

	// There were no multilib properties specifically matching the given archtype.
	// Return zeroed value.
	return reflect.New(reflect.ValueOf(propertySet).Elem().Type()).Interface()
}

// ArchVariantContext defines the limited context necessary to retrieve arch_variant properties.
type ArchVariantContext interface {
	ModuleErrorf(fmt string, args ...interface{})
	PropertyErrorf(property, fmt string, args ...interface{})
}

// GetArchProperties returns a map of architectures to the values of the
// properties of the 'propertySet' struct that are specific to that architecture.
//
// For example, passing a struct { Foo bool, Bar string } will return an
// interface{} that can be type asserted back into the same struct, containing
// the arch specific property value specified by the module if defined.
//
// Arch-specific properties may come from an arch stanza or a multilib stanza; properties
// in these stanzas are combined.
// For example: `arch: { x86: { Foo: ["bar"] } }, multilib: { lib32: {` Foo: ["baz"] } }`
// will result in `Foo: ["bar", "baz"]` being returned for architecture x86, if the given
// propertyset contains `Foo []string`.
func (m *ModuleBase) GetArchProperties(ctx ArchVariantContext, propertySet interface{}) map[ArchType]interface{} {
	// Return value of the arch types to the prop values for that arch.
	archToProp := map[ArchType]interface{}{}

	// Nothing to do for non-arch-specific modules.
	if !m.ArchSpecific() {
		return archToProp
	}

	dstType := reflect.ValueOf(propertySet).Type()
	var archProperties []interface{}

	// First find the property set in the module that corresponds to the requested
	// one. m.archProperties[i] corresponds to m.generalProperties[i].
	for i, generalProp := range m.generalProperties {
		srcType := reflect.ValueOf(generalProp).Type()
		if srcType == dstType {
			archProperties = m.archProperties[i]
			break
		}
	}

	if archProperties == nil {
		// This module does not have the property set requested
		return archToProp
	}

	// For each arch type (x86, arm64, etc.)
	for _, arch := range ArchTypeList() {
		// Arch properties are sometimes sharded (see createArchPropTypeDesc() ).
		// Iterate over ever shard and extract a struct with the same type as the
		// input one that contains the data specific to that arch.
		propertyStructs := make([]reflect.Value, 0)
		for _, archProperty := range archProperties {
			archTypeStruct := getArchTypeStruct(ctx, archProperty, arch)
			multilibStruct := getMultilibStruct(ctx, archProperty, arch)
			propertyStructs = append(propertyStructs, archTypeStruct, multilibStruct)
		}

		// Create a new instance of the requested property set
		value := reflect.New(reflect.ValueOf(propertySet).Elem().Type()).Interface()

		// Merge all the structs together
		for _, propertyStruct := range propertyStructs {
			mergePropertyStruct(ctx, value, propertyStruct)
		}

		archToProp[arch] = value
	}

	return archToProp
}

// GetTargetProperties returns a map of OS target (e.g. android, windows) to the
// values of the properties of the 'dst' struct that are specific to that OS
// target.
//
// For example, passing a struct { Foo bool, Bar string } will return an
// interface{} that can be type asserted back into the same struct, containing
// the os-specific property value specified by the module if defined.
//
// While this looks similar to GetArchProperties, the internal representation of
// the properties have a slightly different layout to warrant a standalone
// lookup function.
func (m *ModuleBase) GetTargetProperties(dst interface{}) map[OsType]interface{} {
	// Return value of the arch types to the prop values for that arch.
	osToProp := map[OsType]interface{}{}

	// Nothing to do for non-OS/arch-specific modules.
	if !m.ArchSpecific() {
		return osToProp
	}

	// archProperties has the type of [][]interface{}. Looks complicated, so
	// let's explain this step by step.
	//
	// Loop over the outer index, which determines the property struct that
	// contains a matching set of properties in dst that we're interested in.
	// For example, BaseCompilerProperties or BaseLinkerProperties.
	for i := range m.archProperties {
		if m.archProperties[i] == nil {
			continue
		}

		// Iterate over the supported OS types
		for _, os := range osTypeList {
			// e.g android, linux_bionic
			field := os.Field

			// If it's not nil, loop over the inner index, which determines the arch variant
			// of the prop type. In an Android.bp file, this is like looping over:
			//
			// target: { android: { key: value, ... }, linux_bionic: { key: value, ... } }
			for _, archProperties := range m.archProperties[i] {
				archPropValues := reflect.ValueOf(archProperties).Elem()

				// This is the archPropRoot struct. Traverse into the Targetnested struct.
				src := archPropValues.FieldByName("Target").Elem()

				// Step into non-nil pointers to structs in the src value.
				if src.Kind() == reflect.Ptr {
					if src.IsNil() {
						continue
					}
					src = src.Elem()
				}

				// Find the requested field (e.g. android, linux_bionic) in the src struct.
				src = src.FieldByName(field)

				// Validation steps. We want valid non-nil pointers to structs.
				if !src.IsValid() || src.IsNil() {
					continue
				}

				if src.Kind() != reflect.Ptr || src.Elem().Kind() != reflect.Struct {
					continue
				}

				// Clone the destination prop, since we want a unique prop struct per arch.
				dstClone := reflect.New(reflect.ValueOf(dst).Elem().Type()).Interface()

				// Copy the located property struct into the cloned destination property struct.
				err := proptools.ExtendMatchingProperties([]interface{}{dstClone}, src.Interface(), nil, proptools.OrderReplace)
				if err != nil {
					// This is fine, it just means the src struct doesn't match.
					continue
				}

				// Found the prop for the os, you have.
				osToProp[os] = dstClone

				// Go to the next prop.
				break
			}
		}
	}
	return osToProp
}
