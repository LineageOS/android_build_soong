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
	"blueprint"
	"blueprint/proptools"
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

var (
	Arm    = newArch32("Arm")
	Arm64  = newArch64("Arm64")
	Mips   = newArch32("Mips")
	Mips64 = newArch64("Mips64")
	X86    = newArch32("X86")
	X86_64 = newArch64("X86_64")
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
	Arch struct {
		Arm    interface{}
		Arm64  interface{}
		Mips   interface{}
		Mips64 interface{}
		X86    interface{}
		X86_64 interface{}
	}
	Multilib struct {
		Lib32 interface{}
		Lib64 interface{}
	}
	Target struct {
		Host        interface{}
		Android     interface{}
		Linux       interface{}
		Darwin      interface{}
		Windows     interface{}
		Not_windows interface{}
	}
}

// An Arch indicates a single CPU architecture.
type Arch struct {
	HostOrDevice HostOrDevice
	ArchType     ArchType
	ArchVariant  string
	CpuVariant   string
}

func (a Arch) String() string {
	s := a.HostOrDevice.String() + "_" + a.ArchType.String()
	if a.ArchVariant != "" {
		s += "_" + a.ArchVariant
	}
	if a.CpuVariant != "" {
		s += "_" + a.CpuVariant
	}
	return s
}

type ArchType struct {
	Name          string
	Field         string
	Multilib      string
	MultilibField string
}

func newArch32(field string) ArchType {
	return ArchType{
		Name:          strings.ToLower(field),
		Field:         field,
		Multilib:      "lib32",
		MultilibField: "Lib32",
	}
}

func newArch64(field string) ArchType {
	return ArchType{
		Name:          strings.ToLower(field),
		Field:         field,
		Multilib:      "lib64",
		MultilibField: "Lib64",
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

func (hod HostOrDevice) FieldLower() string {
	switch hod {
	case Device:
		return "android"
	case Host:
		return "host"
	default:
		panic(fmt.Sprintf("unexpected HostOrDevice value %d", hod))
	}
}

func (hod HostOrDevice) Field() string {
	switch hod {
	case Device:
		return "Android"
	case Host:
		return "Host"
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
	armArch = Arch{
		HostOrDevice: Device,
		ArchType:     Arm,
		ArchVariant:  "armv7-a-neon",
		CpuVariant:   "cortex-a15",
	}
	arm64Arch = Arch{
		HostOrDevice: Device,
		ArchType:     Arm64,
		ArchVariant:  "armv8-a",
		CpuVariant:   "denver",
	}
	hostArch = Arch{
		HostOrDevice: Host,
		ArchType:     X86,
	}
	host64Arch = Arch{
		HostOrDevice: Host,
		ArchType:     X86_64,
	}
)

func ArchMutator(mctx blueprint.EarlyMutatorContext) {
	var module AndroidModule
	var ok bool
	if module, ok = mctx.Module().(AndroidModule); !ok {
		return
	}

	// TODO: this is all hardcoded for arm64 primary, arm secondary for now
	// Replace with a configuration file written by lunch or bootstrap

	arches := []Arch{}

	if module.base().HostSupported() {
		arches = append(arches, host64Arch)
	}

	if module.base().DeviceSupported() {
		switch module.base().commonProperties.Compile_multilib {
		case "both":
			arches = append(arches, arm64Arch, armArch)
		case "first", "64":
			arches = append(arches, arm64Arch)
		case "32":
			arches = append(arches, armArch)
		default:
			mctx.ModuleErrorf(`compile_multilib must be "both", "first", "32", or "64", found %q`,
				module.base().commonProperties.Compile_multilib)
		}
	}

	if len(arches) == 0 {
		return
	}

	archNames := []string{}
	for _, arch := range arches {
		archNames = append(archNames, arch.String())
	}

	modules := mctx.CreateVariations(archNames...)

	for i, m := range modules {
		m.(AndroidModule).base().SetArch(arches[i])
		m.(AndroidModule).base().setArchProperties(mctx, arches[i])
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
			newValue := proptools.CloneProperties(propertiesValue)
			proptools.ZeroProperties(newValue.Elem())
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

// Rewrite the module's properties structs to contain arch-specific values.
func (a *AndroidModuleBase) setArchProperties(ctx blueprint.EarlyMutatorContext, arch Arch) {
	for i := range a.generalProperties {
		generalPropsValue := reflect.ValueOf(a.generalProperties[i]).Elem()

		// Handle arch-specific properties in the form:
		// arch {
		//     arm64 {
		//         key: value,
		//     },
		// },
		t := arch.ArchType
		extendProperties(ctx, "arch", t.Name, generalPropsValue,
			reflect.ValueOf(a.archProperties[i].Arch).FieldByName(t.Field).Elem().Elem())

		// Handle multilib-specific properties in the form:
		// multilib {
		//     lib32 {
		//         key: value,
		//     },
		// },
		extendProperties(ctx, "multilib", t.Multilib, generalPropsValue,
			reflect.ValueOf(a.archProperties[i].Multilib).FieldByName(t.MultilibField).Elem().Elem())

		// Handle host-or-device-specific properties in the form:
		// target {
		//     host {
		//         key: value,
		//     },
		// },
		hod := arch.HostOrDevice
		extendProperties(ctx, "target", hod.FieldLower(), generalPropsValue,
			reflect.ValueOf(a.archProperties[i].Target).FieldByName(hod.Field()).Elem().Elem())

		// Handle host target properties in the form:
		// target {
		//     linux {
		//         key: value,
		//     },
		//     not_windows {
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
					extendProperties(ctx, "target", v.goos, generalPropsValue,
						reflect.ValueOf(a.archProperties[i].Target).FieldByName(v.field).Elem().Elem())
				}
			}
			extendProperties(ctx, "target", "not_windows", generalPropsValue,
				reflect.ValueOf(a.archProperties[i].Target).FieldByName("Not_windows").Elem().Elem())
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

// TODO: move this to proptools
func extendProperties(ctx blueprint.EarlyMutatorContext, variationType, variationName string,
	dstValue, srcValue reflect.Value) {
	extendPropertiesRecursive(ctx, variationType, variationName, dstValue, srcValue, "")
}

func extendPropertiesRecursive(ctx blueprint.EarlyMutatorContext, variationType, variationName string,
	dstValue, srcValue reflect.Value, recursePrefix string) {

	typ := dstValue.Type()
	if srcValue.Type() != typ {
		panic(fmt.Errorf("can't extend mismatching types (%s <- %s)",
			dstValue.Kind(), srcValue.Kind()))
	}

	for i := 0; i < srcValue.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			// The field is not exported so just skip it.
			continue
		}

		srcFieldValue := srcValue.Field(i)
		dstFieldValue := dstValue.Field(i)

		localPropertyName := proptools.PropertyNameForField(field.Name)
		propertyName := fmt.Sprintf("%s.%s.%s%s", variationType, variationName,
			recursePrefix, localPropertyName)
		propertyPresentInVariation := ctx.ContainsProperty(propertyName)

		if !propertyPresentInVariation {
			continue
		}

		tag := field.Tag.Get("android")
		tags := map[string]bool{}
		for _, entry := range strings.Split(tag, ",") {
			if entry != "" {
				tags[entry] = true
			}
		}

		if !tags["arch_variant"] {
			ctx.PropertyErrorf(propertyName, "property %q can't be specific to a build variant",
				recursePrefix+proptools.PropertyNameForField(field.Name))
			continue
		}

		switch srcFieldValue.Kind() {
		case reflect.Bool:
			// Replace the original value.
			dstFieldValue.Set(srcFieldValue)
		case reflect.String:
			// Append the extension string.
			dstFieldValue.SetString(dstFieldValue.String() +
				srcFieldValue.String())
		case reflect.Struct:
			// Recursively extend the struct's fields.
			newRecursePrefix := fmt.Sprintf("%s%s.", recursePrefix, strings.ToLower(field.Name))
			extendPropertiesRecursive(ctx, variationType, variationName,
				dstFieldValue, srcFieldValue,
				newRecursePrefix)
		case reflect.Slice:
			val, err := archCombineSlices(dstFieldValue, srcFieldValue, tags["arch_subtract"])
			if err != nil {
				ctx.PropertyErrorf(propertyName, err.Error())
				continue
			}
			dstFieldValue.Set(val)
		case reflect.Ptr, reflect.Interface:
			// Recursively extend the pointed-to struct's fields.
			if dstFieldValue.IsNil() != srcFieldValue.IsNil() {
				panic(fmt.Errorf("can't extend field %q: nilitude mismatch"))
			}
			if dstFieldValue.Type() != srcFieldValue.Type() {
				panic(fmt.Errorf("can't extend field %q: type mismatch"))
			}
			if !dstFieldValue.IsNil() {
				newRecursePrefix := fmt.Sprintf("%s.%s", recursePrefix, field.Name)
				extendPropertiesRecursive(ctx, variationType, variationName,
					dstFieldValue.Elem(), srcFieldValue.Elem(),
					newRecursePrefix)
			}
		default:
			panic(fmt.Errorf("unexpected kind for property struct field %q: %s",
				field.Name, srcFieldValue.Kind()))
		}
	}
}

func archCombineSlices(general, arch reflect.Value, canSubtract bool) (reflect.Value, error) {
	if !canSubtract {
		// Append the extension slice.
		return reflect.AppendSlice(general, arch), nil
	}

	// Support -val in arch list to subtract a value from original list
	l := general.Interface().([]string)
	for archIndex := 0; archIndex < arch.Len(); archIndex++ {
		archString := arch.Index(archIndex).String()
		if strings.HasPrefix(archString, "-") {
			generalIndex := findStringInSlice(archString[1:], l)
			if generalIndex == -1 {
				return reflect.Value{},
					fmt.Errorf("can't find %q to subtract from general properties", archString[1:])
			}
			l = append(l[:generalIndex], l[generalIndex+1:]...)
		} else {
			l = append(l, archString)
		}
	}

	return reflect.ValueOf(l), nil
}

func findStringInSlice(str string, slice []string) int {
	for i, s := range slice {
		if s == str {
			return i
		}
	}

	return -1
}
