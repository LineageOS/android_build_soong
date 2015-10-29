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

package common

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"android/soong"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	soong.RegisterEarlyMutator("host_or_device", HostOrDeviceMutator)
	soong.RegisterEarlyMutator("arch", ArchMutator)
}

var (
	Arm    = newArch("arm", "lib32")
	Arm64  = newArch("arm64", "lib64")
	Mips   = newArch("mips", "lib32")
	Mips64 = newArch("mips64", "lib64")
	X86    = newArch("x86", "lib32")
	X86_64 = newArch("x86_64", "lib64")

	Common = ArchType{
		Name: "common",
	}
)

var archTypeMap = map[string]ArchType{
	"arm":    Arm,
	"arm64":  Arm64,
	"mips":   Mips,
	"misp64": Mips64,
	"x86":    X86,
	"x86_64": X86_64,
}

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
        mips: {
            // Host or device variants with mips architecture
        },
        mips64: {
            // Host or device variants with mips64 architecture
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
            // Device variants
        },
        host: {
            // Host variants
        },
        linux: {
            // Linux host variants
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
    },
}
*/

type archProperties struct {
	// Properties to vary by target architecture
	Arch struct {
		// Properties for module variants being built to run on arm (host or device)
		Arm interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on arm64 (host or device)
		Arm64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on mips (host or device)
		Mips interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on mips64 (host or device)
		Mips64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on x86 (host or device)
		X86 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on x86_64 (host or device)
		X86_64 interface{} `blueprint:"filter(android:\"arch_variant\")"`

		// Arm arch variants
		Armv5te      interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Armv7_a      interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Armv7_a_neon interface{} `blueprint:"filter(android:\"arch_variant\")"`

		// Arm cpu variants
		Cortex_a7      interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Cortex_a8      interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Cortex_a9      interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Cortex_a15     interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Cortex_a53     interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Cortex_a53_a57 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Krait          interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Denver         interface{} `blueprint:"filter(android:\"arch_variant\")"`

		// Arm64 cpu variants
		Cortex_a53_64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Denver64      interface{} `blueprint:"filter(android:\"arch_variant\")"`

		// Mips arch variants
		Mips_rev6 interface{} `blueprint:"filter(android:\"arch_variant\")"`

		// X86 arch variants
		X86_ssse3 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		X86_sse4  interface{} `blueprint:"filter(android:\"arch_variant\")"`

		// X86 cpu variants
		Atom       interface{} `blueprint:"filter(android:\"arch_variant\")"`
		Silvermont interface{} `blueprint:"filter(android:\"arch_variant\")"`
	}

	// Properties to vary by 32-bit or 64-bit
	Multilib struct {
		// Properties for module variants being built to run on 32-bit devices
		Lib32 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on 64-bit devices
		Lib64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
	}
	// Properties to vary by build target (host or device, os, os+archictecture)
	Target struct {
		// Properties for module variants being built to run on the host
		Host interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on the device
		Android interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on arm devices
		Android_arm interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on arm64 devices
		Android_arm64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on mips devices
		Android_mips interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on mips64 devices
		Android_mips64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on x86 devices
		Android_x86 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on x86_64 devices
		Android_x86_64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on devices that support 64-bit
		Android64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on devices that do not support 64-bit
		Android32 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on linux hosts
		Linux interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on linux x86 hosts
		Linux_x86 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on linux x86_64 hosts
		Linux_x86_64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on darwin hosts
		Darwin interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on darwin x86 hosts
		Darwin_x86 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on darwin x86_64 hosts
		Darwin_x86_64 interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on windows hosts
		Windows interface{} `blueprint:"filter(android:\"arch_variant\")"`
		// Properties for module variants being built to run on linux or darwin hosts
		Not_windows interface{} `blueprint:"filter(android:\"arch_variant\")"`
	}
}

// An Arch indicates a single CPU architecture.
type Arch struct {
	ArchType    ArchType
	ArchVariant string
	CpuVariant  string
	Abi         []string
}

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

type ArchType struct {
	Name     string
	Multilib string
}

func newArch(name, multilib string) ArchType {
	return ArchType{
		Name:     name,
		Multilib: multilib,
	}
}

func (a ArchType) String() string {
	return a.Name
}

type HostOrDeviceSupported int

const (
	_ HostOrDeviceSupported = iota
	HostSupported
	DeviceSupported
	HostAndDeviceSupported
)

type HostOrDevice int

const (
	_ HostOrDevice = iota
	Host
	Device
)

func (hod HostOrDevice) String() string {
	switch hod {
	case Device:
		return "device"
	case Host:
		return "host"
	default:
		panic(fmt.Sprintf("unexpected HostOrDevice value %d", hod))
	}
}

func (hod HostOrDevice) Property() string {
	switch hod {
	case Device:
		return "android"
	case Host:
		return "host"
	default:
		panic(fmt.Sprintf("unexpected HostOrDevice value %d", hod))
	}
}

func (hod HostOrDevice) Host() bool {
	if hod == 0 {
		panic("HostOrDevice unset")
	}
	return hod == Host
}

func (hod HostOrDevice) Device() bool {
	if hod == 0 {
		panic("HostOrDevice unset")
	}
	return hod == Device
}

var hostOrDeviceName = map[HostOrDevice]string{
	Device: "device",
	Host:   "host",
}

var (
	commonArch = Arch{
		ArchType: Common,
	}
)

func HostOrDeviceMutator(mctx blueprint.EarlyMutatorContext) {
	var module AndroidModule
	var ok bool
	if module, ok = mctx.Module().(AndroidModule); !ok {
		return
	}

	hods := []HostOrDevice{}

	if module.base().HostSupported() {
		hods = append(hods, Host)
	}

	if module.base().DeviceSupported() {
		hods = append(hods, Device)
	}

	if len(hods) == 0 {
		return
	}

	hodNames := []string{}
	for _, hod := range hods {
		hodNames = append(hodNames, hod.String())
	}

	modules := mctx.CreateVariations(hodNames...)
	for i, m := range modules {
		m.(AndroidModule).base().SetHostOrDevice(hods[i])
	}
}

func ArchMutator(mctx blueprint.EarlyMutatorContext) {
	var module AndroidModule
	var ok bool
	if module, ok = mctx.Module().(AndroidModule); !ok {
		return
	}

	hostArches, deviceArches, err := decodeArchProductVariables(mctx.Config().(Config).ProductVariables)
	if err != nil {
		mctx.ModuleErrorf("%s", err.Error())
	}

	moduleArches := []Arch{}
	multilib := module.base().commonProperties.Compile_multilib

	if module.base().HostSupported() && module.base().HostOrDevice().Host() {
		hostModuleArches, err := decodeMultilib(multilib, hostArches)
		if err != nil {
			mctx.ModuleErrorf("%s", err.Error())
		}

		moduleArches = append(moduleArches, hostModuleArches...)
	}

	if module.base().DeviceSupported() && module.base().HostOrDevice().Device() {
		deviceModuleArches, err := decodeMultilib(multilib, deviceArches)
		if err != nil {
			mctx.ModuleErrorf("%s", err.Error())
		}

		moduleArches = append(moduleArches, deviceModuleArches...)
	}

	if len(moduleArches) == 0 {
		return
	}

	archNames := []string{}
	for _, arch := range moduleArches {
		archNames = append(archNames, arch.String())
	}

	modules := mctx.CreateVariations(archNames...)

	for i, m := range modules {
		m.(AndroidModule).base().SetArch(moduleArches[i])
		m.(AndroidModule).base().setArchProperties(mctx)
	}
}

func InitArchModule(m AndroidModule, defaultMultilib Multilib,
	propertyStructs ...interface{}) (blueprint.Module, []interface{}) {

	base := m.base()

	base.commonProperties.Compile_multilib = string(defaultMultilib)

	base.generalProperties = append(base.generalProperties,
		propertyStructs...)

	for _, properties := range base.generalProperties {
		propertiesValue := reflect.ValueOf(properties)
		if propertiesValue.Kind() != reflect.Ptr {
			panic("properties must be a pointer to a struct")
		}

		propertiesValue = propertiesValue.Elem()
		if propertiesValue.Kind() != reflect.Struct {
			panic("properties must be a pointer to a struct")
		}

		archProperties := &archProperties{}
		forEachInterface(reflect.ValueOf(archProperties), func(v reflect.Value) {
			newValue := proptools.CloneEmptyProperties(propertiesValue)
			v.Set(newValue)
		})

		base.archProperties = append(base.archProperties, archProperties)
	}

	var allProperties []interface{}
	allProperties = append(allProperties, base.generalProperties...)
	for _, asp := range base.archProperties {
		allProperties = append(allProperties, asp)
	}

	return m, allProperties
}

var dashToUnderscoreReplacer = strings.NewReplacer("-", "_")

func (a *AndroidModuleBase) appendProperties(ctx blueprint.EarlyMutatorContext,
	dst, src interface{}, field, srcPrefix string) {

	src = reflect.ValueOf(src).FieldByName(field).Elem().Interface()

	filter := func(property string,
		dstField, srcField reflect.StructField,
		dstValue, srcValue interface{}) (bool, error) {

		srcProperty := srcPrefix + "." + property

		if !proptools.HasTag(dstField, "android", "arch_variant") {
			if ctx.ContainsProperty(srcProperty) {
				return false, fmt.Errorf("can't be specific to a build variant")
			} else {
				return false, nil
			}
		}

		return true, nil
	}

	err := proptools.AppendProperties(dst, src, filter)
	if err != nil {
		if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
			ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
		} else {
			panic(err)
		}
	}
}

// Rewrite the module's properties structs to contain arch-specific values.
func (a *AndroidModuleBase) setArchProperties(ctx blueprint.EarlyMutatorContext) {
	arch := a.commonProperties.CompileArch
	hod := a.commonProperties.CompileHostOrDevice

	if arch.ArchType == Common {
		return
	}

	for i := range a.generalProperties {
		genProps := a.generalProperties[i]
		archProps := a.archProperties[i]
		// Handle arch-specific properties in the form:
		// arch: {
		//     arm64: {
		//         key: value,
		//     },
		// },
		t := arch.ArchType

		field := proptools.FieldNameForProperty(t.Name)
		prefix := "arch." + t.Name
		a.appendProperties(ctx, genProps, archProps.Arch, field, prefix)

		// Handle arch-variant-specific properties in the form:
		// arch: {
		//     variant: {
		//         key: value,
		//     },
		// },
		v := dashToUnderscoreReplacer.Replace(arch.ArchVariant)
		if v != "" {
			field := proptools.FieldNameForProperty(v)
			prefix := "arch." + v
			a.appendProperties(ctx, genProps, archProps.Arch, field, prefix)
		}

		// Handle cpu-variant-specific properties in the form:
		// arch: {
		//     variant: {
		//         key: value,
		//     },
		// },
		c := dashToUnderscoreReplacer.Replace(arch.CpuVariant)
		if c != "" {
			field := proptools.FieldNameForProperty(c)
			prefix := "arch." + c
			a.appendProperties(ctx, genProps, archProps.Arch, field, prefix)
		}

		// Handle multilib-specific properties in the form:
		// multilib: {
		//     lib32: {
		//         key: value,
		//     },
		// },
		field = proptools.FieldNameForProperty(t.Multilib)
		prefix = "multilib." + t.Multilib
		a.appendProperties(ctx, genProps, archProps.Multilib, field, prefix)

		// Handle host-or-device-specific properties in the form:
		// target: {
		//     host: {
		//         key: value,
		//     },
		// },
		hodProperty := hod.Property()
		field = proptools.FieldNameForProperty(hodProperty)
		prefix = "target." + hodProperty
		a.appendProperties(ctx, genProps, archProps.Target, field, prefix)

		// Handle host target properties in the form:
		// target: {
		//     linux: {
		//         key: value,
		//     },
		//     not_windows: {
		//         key: value,
		//     },
		//     linux_x86: {
		//         key: value,
		//     },
		//     linux_arm: {
		//         key: value,
		//     },
		// },
		var osList = []struct {
			goos  string
			field string
		}{
			{"darwin", "Darwin"},
			{"linux", "Linux"},
			{"windows", "Windows"},
		}

		if hod.Host() {
			for _, v := range osList {
				if v.goos == runtime.GOOS {
					field := v.field
					prefix := "target." + v.goos
					a.appendProperties(ctx, genProps, archProps.Target, field, prefix)
					t := arch.ArchType
					field = v.field + "_" + t.Name
					prefix = "target." + v.goos + "_" + t.Name
					a.appendProperties(ctx, genProps, archProps.Target, field, prefix)
				}
			}
			field := "Not_windows"
			prefix := "target.not_windows"
			a.appendProperties(ctx, genProps, archProps.Target, field, prefix)
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
		if hod.Device() {
			if true /* && target_is_64_bit */ {
				field := "Android64"
				prefix := "target.android64"
				a.appendProperties(ctx, genProps, archProps.Target, field, prefix)
			} else {
				field := "Android32"
				prefix := "target.android32"
				a.appendProperties(ctx, genProps, archProps.Target, field, prefix)
			}
		}

		// Handle device architecture properties in the form:
		// target {
		//     android_arm {
		//         key: value,
		//     },
		//     android_x86 {
		//         key: value,
		//     },
		// },
		if hod.Device() {
			t := arch.ArchType
			field := "Android_" + t.Name
			prefix := "target.android_" + t.Name
			a.appendProperties(ctx, genProps, archProps.Target, field, prefix)
		}

		if ctx.Failed() {
			return
		}
	}
}

func forEachInterface(v reflect.Value, f func(reflect.Value)) {
	switch v.Kind() {
	case reflect.Interface:
		f(v)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			forEachInterface(v.Field(i), f)
		}
	case reflect.Ptr:
		forEachInterface(v.Elem(), f)
	default:
		panic(fmt.Errorf("Unsupported kind %s", v.Kind()))
	}
}

// Convert the arch product variables into a list of host and device Arch structs
func decodeArchProductVariables(variables productVariables) ([]Arch, []Arch, error) {
	if variables.HostArch == nil {
		return nil, nil, fmt.Errorf("No host primary architecture set")
	}

	hostArch, err := decodeArch(*variables.HostArch, nil, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	hostArches := []Arch{hostArch}

	if variables.HostSecondaryArch != nil {
		hostSecondaryArch, err := decodeArch(*variables.HostSecondaryArch, nil, nil, nil)
		if err != nil {
			return nil, nil, err
		}
		hostArches = append(hostArches, hostSecondaryArch)
	}

	if variables.DeviceArch == nil {
		return nil, nil, fmt.Errorf("No device primary architecture set")
	}

	deviceArch, err := decodeArch(*variables.DeviceArch, variables.DeviceArchVariant,
		variables.DeviceCpuVariant, variables.DeviceAbi)
	if err != nil {
		return nil, nil, err
	}

	deviceArches := []Arch{deviceArch}

	if variables.DeviceSecondaryArch != nil {
		deviceSecondaryArch, err := decodeArch(*variables.DeviceSecondaryArch,
			variables.DeviceSecondaryArchVariant, variables.DeviceSecondaryCpuVariant,
			variables.DeviceSecondaryAbi)
		if err != nil {
			return nil, nil, err
		}
		deviceArches = append(deviceArches, deviceSecondaryArch)
	}

	return hostArches, deviceArches, nil
}

// Convert a set of strings from product variables into a single Arch struct
func decodeArch(arch string, archVariant, cpuVariant *string, abi *[]string) (Arch, error) {
	stringPtr := func(p *string) string {
		if p != nil {
			return *p
		}
		return ""
	}

	slicePtr := func(p *[]string) []string {
		if p != nil {
			return *p
		}
		return nil
	}

	archType := archTypeMap[arch]

	return Arch{
		ArchType:    archType,
		ArchVariant: stringPtr(archVariant),
		CpuVariant:  stringPtr(cpuVariant),
		Abi:         slicePtr(abi),
	}, nil
}

// Use the module multilib setting to select one or more arches from an arch list
func decodeMultilib(multilib string, arches []Arch) ([]Arch, error) {
	buildArches := []Arch{}
	switch multilib {
	case "common":
		buildArches = append(buildArches, commonArch)
	case "both":
		buildArches = append(buildArches, arches...)
	case "first":
		buildArches = append(buildArches, arches[0])
	case "32":
		for _, a := range arches {
			if a.ArchType.Multilib == "lib32" {
				buildArches = append(buildArches, a)
			}
		}
	case "64":
		for _, a := range arches {
			if a.ArchType.Multilib == "lib64" {
				buildArches = append(buildArches, a)
			}
		}
	default:
		return nil, fmt.Errorf(`compile_multilib must be "both", "first", "32", or "64", found %q`,
			multilib)
		//buildArches = append(buildArches, arches[0])
	}

	return buildArches, nil
}
