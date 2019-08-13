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
	"strconv"
	"strings"

	"github.com/google/blueprint/proptools"
)

var (
	archTypeList []ArchType

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
	"mips64": Mips64,
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
        linux_glibc: {
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

var archVariants = map[ArchType][]string{
	Arm: {
		"armv7-a",
		"armv7-a-neon",
		"armv8-a",
		"armv8-2a",
		"cortex-a7",
		"cortex-a8",
		"cortex-a9",
		"cortex-a15",
		"cortex-a53",
		"cortex-a53-a57",
		"cortex-a55",
		"cortex-a72",
		"cortex-a73",
		"cortex-a75",
		"cortex-a76",
		"krait",
		"kryo",
		"kryo385",
		"exynos-m1",
		"exynos-m2",
	},
	Arm64: {
		"armv8_a",
		"armv8_2a",
		"cortex-a53",
		"cortex-a55",
		"cortex-a72",
		"cortex-a73",
		"cortex-a75",
		"cortex-a76",
		"kryo",
		"kryo385",
		"exynos-m1",
		"exynos-m2",
	},
	Mips: {
		"mips32_fp",
		"mips32r2_fp",
		"mips32r2_fp_xburst",
		"mips32r2dsp_fp",
		"mips32r2dspr2_fp",
		"mips32r6",
	},
	Mips64: {
		"mips64r2",
		"mips64r6",
	},
	X86: {
		"amberlake",
		"atom",
		"broadwell",
		"haswell",
		"icelake",
		"ivybridge",
		"kabylake",
		"sandybridge",
		"silvermont",
		"skylake",
		"stoneyridge",
		"tigerlake",
		"whiskeylake",
		"x86_64",
	},
	X86_64: {
		"amberlake",
		"broadwell",
		"haswell",
		"icelake",
		"ivybridge",
		"kabylake",
		"sandybridge",
		"silvermont",
		"skylake",
		"stoneyridge",
		"tigerlake",
		"whiskeylake",
	},
}

var archFeatures = map[ArchType][]string{
	Arm: {
		"neon",
	},
	Mips: {
		"dspr2",
		"rev6",
		"msa",
	},
	Mips64: {
		"rev6",
		"msa",
	},
	X86: {
		"ssse3",
		"sse4",
		"sse4_1",
		"sse4_2",
		"aes_ni",
		"avx",
		"avx2",
		"avx512",
		"popcnt",
		"movbe",
	},
	X86_64: {
		"ssse3",
		"sse4",
		"sse4_1",
		"sse4_2",
		"aes_ni",
		"avx",
		"avx2",
		"avx512",
		"popcnt",
	},
}

var archFeatureMap = map[ArchType]map[string][]string{
	Arm: {
		"armv7-a-neon": {
			"neon",
		},
		"armv8-a": {
			"neon",
		},
		"armv8-2a": {
			"neon",
		},
	},
	Mips: {
		"mips32r2dspr2_fp": {
			"dspr2",
		},
		"mips32r6": {
			"rev6",
		},
	},
	Mips64: {
		"mips64r6": {
			"rev6",
		},
	},
	X86: {
		"amberlake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"atom": {
			"ssse3",
			"movbe",
		},
		"broadwell": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"haswell": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"popcnt",
			"movbe",
		},
		"icelake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"ivybridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"popcnt",
		},
		"kabylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"sandybridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"popcnt",
		},
		"silvermont": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"popcnt",
			"movbe",
		},
		"skylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"stoneyridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"avx2",
			"popcnt",
			"movbe",
		},
		"tigerlake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"whiskeylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"x86_64": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"popcnt",
		},
	},
	X86_64: {
		"amberlake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"broadwell": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"haswell": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"popcnt",
		},
		"icelake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"ivybridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"popcnt",
		},
		"kabylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"sandybridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"popcnt",
		},
		"silvermont": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"popcnt",
		},
		"skylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"stoneyridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"avx2",
			"popcnt",
		},
		"tigerlake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"whiskeylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
	},
}

var defaultArchFeatureMap = map[OsType]map[ArchType][]string{}

func RegisterDefaultArchVariantFeatures(os OsType, arch ArchType, features ...string) {
	checkCalledFromInit()

	for _, feature := range features {
		if !InList(feature, archFeatures[arch]) {
			panic(fmt.Errorf("Invalid feature %q for arch %q variant \"\"", feature, arch))
		}
	}

	if defaultArchFeatureMap[os] == nil {
		defaultArchFeatureMap[os] = make(map[ArchType][]string)
	}
	defaultArchFeatureMap[os][arch] = features
}

// An Arch indicates a single CPU architecture.
type Arch struct {
	ArchType     ArchType
	ArchVariant  string
	CpuVariant   string
	Abi          []string
	ArchFeatures []string
	Native       bool
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
	Field    string
	Multilib string
}

func newArch(name, multilib string) ArchType {
	archType := ArchType{
		Name:     name,
		Field:    proptools.FieldNameForProperty(name),
		Multilib: multilib,
	}
	archTypeList = append(archTypeList, archType)
	return archType
}

func ArchTypeList() []ArchType {
	return append([]ArchType(nil), archTypeList...)
}

func (a ArchType) String() string {
	return a.Name
}

var _ encoding.TextMarshaler = ArchType{}

func (a ArchType) MarshalText() ([]byte, error) {
	return []byte(strconv.Quote(a.String())), nil
}

var _ encoding.TextUnmarshaler = &ArchType{}

func (a *ArchType) UnmarshalText(text []byte) error {
	if u, ok := archTypeMap[string(text)]; ok {
		*a = u
		return nil
	}

	return fmt.Errorf("unknown ArchType %q", text)
}

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

var (
	osTypeList      []OsType
	commonTargetMap = make(map[string]Target)

	NoOsType    OsType
	Linux       = NewOsType("linux_glibc", Host, false)
	Darwin      = NewOsType("darwin", Host, false)
	LinuxBionic = NewOsType("linux_bionic", Host, false)
	Windows     = NewOsType("windows", HostCross, true)
	Android     = NewOsType("android", Device, false)
	Fuchsia     = NewOsType("fuchsia", Device, false)

	osArchTypeMap = map[OsType][]ArchType{
		Linux:       []ArchType{X86, X86_64},
		LinuxBionic: []ArchType{X86_64},
		Darwin:      []ArchType{X86_64},
		Windows:     []ArchType{X86, X86_64},
		Android:     []ArchType{Arm, Arm64, Mips, Mips64, X86, X86_64},
		Fuchsia:     []ArchType{Arm64, X86_64},
	}
)

type OsType struct {
	Name, Field string
	Class       OsClass

	DefaultDisabled bool
}

type OsClass int

const (
	Generic OsClass = iota
	Device
	Host
	HostCross
)

func (class OsClass) String() string {
	switch class {
	case Generic:
		return "generic"
	case Device:
		return "device"
	case Host:
		return "host"
	case HostCross:
		return "host cross"
	default:
		panic(fmt.Errorf("unknown class %d", class))
	}
}

func (os OsType) String() string {
	return os.Name
}

func (os OsType) Bionic() bool {
	return os == Android || os == LinuxBionic
}

func (os OsType) Linux() bool {
	return os == Android || os == Linux || os == LinuxBionic
}

func NewOsType(name string, class OsClass, defDisabled bool) OsType {
	os := OsType{
		Name:  name,
		Field: strings.Title(name),
		Class: class,

		DefaultDisabled: defDisabled,
	}
	osTypeList = append(osTypeList, os)

	if _, found := commonTargetMap[name]; found {
		panic(fmt.Errorf("Found Os type duplicate during OsType registration: %q", name))
	} else {
		commonTargetMap[name] = Target{Os: os, Arch: Arch{ArchType: Common}}
	}

	return os
}

func osByName(name string) OsType {
	for _, os := range osTypeList {
		if os.Name == name {
			return os
		}
	}

	return NoOsType
}

type Target struct {
	Os   OsType
	Arch Arch
}

func (target Target) String() string {
	return target.Os.String() + "_" + target.Arch.String()
}

// archMutator splits a module into a variant for each Target requested by the module.  Target selection
// for a module is in three levels, OsClass, mulitlib, and then Target.
// OsClass selection is determined by:
//    - The HostOrDeviceSupported value passed in to InitAndroidArchModule by the module type factory, which selects
//      whether the module type can compile for host, device or both.
//    - The host_supported and device_supported properties on the module.
// If host is supported for the module, the Host and HostCross OsClasses are  are selected.  If device is supported
// for the module, the Device OsClass is selected.
// Within each selected OsClass, the multilib selection is determined by:
//    - The compile_multilib property if it set (which may be overriden by target.android.compile_multlib or
//      target.host.compile_multilib).
//    - The default multilib passed to InitAndroidArchModule if compile_multilib was not set.
// Valid multilib values include:
//    "both": compile for all Targets supported by the OsClass (generally x86_64 and x86, or arm64 and arm).
//    "first": compile for only a single preferred Target supported by the OsClass.  This is generally x86_64 or arm64,
//        but may be arm for a 32-bit only build or a build with TARGET_PREFER_32_BIT=true set.
//    "32": compile for only a single 32-bit Target supported by the OsClass.
//    "64": compile for only a single 64-bit Target supported by the OsClass.
//    "common": compile a for a single Target that will work on all Targets suported by the OsClass (for example Java).
//
// Once the list of Targets is determined, the module is split into a variant for each Target.
//
// Modules can be initialized with InitAndroidMultiTargetsArchModule, in which case they will be split by OsClass,
// but will have a common Target that is expected to handle all other selected Targets via ctx.MultiTargets().
func archMutator(mctx BottomUpMutatorContext) {
	var module Module
	var ok bool
	if module, ok = mctx.Module().(Module); !ok {
		return
	}

	base := module.base()

	if !base.ArchSpecific() {
		return
	}

	var moduleTargets []Target
	moduleMultiTargets := make(map[int][]Target)
	primaryModules := make(map[int]bool)
	osClasses := base.OsClassSupported()

	for _, os := range osTypeList {
		supportedClass := false
		for _, osClass := range osClasses {
			if os.Class == osClass {
				supportedClass = true
			}
		}
		if !supportedClass {
			continue
		}

		osTargets := mctx.Config().Targets[os]
		if len(osTargets) == 0 {
			continue
		}

		// only the primary arch in the recovery partition
		if os == Android && module.InstallInRecovery() {
			osTargets = []Target{osTargets[0]}
		}

		prefer32 := false
		if base.prefer32 != nil {
			prefer32 = base.prefer32(mctx, base, os.Class)
		}

		multilib, extraMultilib := decodeMultilib(base, os.Class)
		targets, err := decodeMultilibTargets(multilib, osTargets, prefer32)
		if err != nil {
			mctx.ModuleErrorf("%s", err.Error())
		}

		var multiTargets []Target
		if extraMultilib != "" {
			multiTargets, err = decodeMultilibTargets(extraMultilib, osTargets, prefer32)
			if err != nil {
				mctx.ModuleErrorf("%s", err.Error())
			}
		}

		if len(targets) > 0 {
			primaryModules[len(moduleTargets)] = true
			moduleMultiTargets[len(moduleTargets)] = multiTargets
			moduleTargets = append(moduleTargets, targets...)
		}
	}

	if len(moduleTargets) == 0 {
		base.commonProperties.Enabled = boolPtr(false)
		return
	}

	targetNames := make([]string, len(moduleTargets))

	for i, target := range moduleTargets {
		targetNames[i] = target.String()
	}

	modules := mctx.CreateVariations(targetNames...)
	for i, m := range modules {
		m.(Module).base().SetTarget(moduleTargets[i], moduleMultiTargets[i], primaryModules[i])
		m.(Module).base().setArchProperties(mctx)
	}
}

func decodeMultilib(base *ModuleBase, class OsClass) (multilib, extraMultilib string) {
	switch class {
	case Device:
		multilib = String(base.commonProperties.Target.Android.Compile_multilib)
	case Host, HostCross:
		multilib = String(base.commonProperties.Target.Host.Compile_multilib)
	}
	if multilib == "" {
		multilib = String(base.commonProperties.Compile_multilib)
	}
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

func filterArchStructFields(fields []reflect.StructField) (filteredFields []reflect.StructField, filtered bool) {
	for _, field := range fields {
		if !proptools.HasTag(field, "android", "arch_variant") {
			filtered = true
			continue
		}

		// The arch_variant field isn't necessary past this point
		// Instead of wasting space, just remove it. Go also has a
		// 16-bit limit on structure name length. The name is constructed
		// based on the Go source representation of the structure, so
		// the tag names count towards that length.
		//
		// TODO: handle the uncommon case of other tags being involved
		if field.Tag == `android:"arch_variant"` {
			field.Tag = ""
		}

		// Recurse into structs
		switch field.Type.Kind() {
		case reflect.Struct:
			var subFiltered bool
			field.Type, subFiltered = filterArchStruct(field.Type)
			filtered = filtered || subFiltered
			if field.Type == nil {
				continue
			}
		case reflect.Ptr:
			if field.Type.Elem().Kind() == reflect.Struct {
				nestedType, subFiltered := filterArchStruct(field.Type.Elem())
				filtered = filtered || subFiltered
				if nestedType == nil {
					continue
				}
				field.Type = reflect.PtrTo(nestedType)
			}
		case reflect.Interface:
			panic("Interfaces are not supported in arch_variant properties")
		}

		filteredFields = append(filteredFields, field)
	}

	return filteredFields, filtered
}

// filterArchStruct takes a reflect.Type that is either a sturct or a pointer to a struct, and returns a reflect.Type
// that only contains the fields in the original type that have an `android:"arch_variant"` struct tag, and a bool
// that is true if the new struct type has fewer fields than the original type.  If there are no fields in the
// original type with the struct tag it returns nil and true.
func filterArchStruct(prop reflect.Type) (filteredProp reflect.Type, filtered bool) {
	var fields []reflect.StructField

	ptr := prop.Kind() == reflect.Ptr
	if ptr {
		prop = prop.Elem()
	}

	for i := 0; i < prop.NumField(); i++ {
		fields = append(fields, prop.Field(i))
	}

	filteredFields, filtered := filterArchStructFields(fields)

	if len(filteredFields) == 0 {
		return nil, true
	}

	if !filtered {
		if ptr {
			return reflect.PtrTo(prop), false
		}
		return prop, false
	}

	ret := reflect.StructOf(filteredFields)
	if ptr {
		ret = reflect.PtrTo(ret)
	}

	return ret, true
}

// filterArchStruct takes a reflect.Type that is either a sturct or a pointer to a struct, and returns a list of
// reflect.Type that only contains the fields in the original type that have an `android:"arch_variant"` struct tag,
// and a bool that is true if the new struct type has fewer fields than the original type.  If there are no fields in
// the original type with the struct tag it returns nil and true.  Each returned struct type will have a maximum of
// 10 top level fields in it to attempt to avoid hitting the reflect.StructOf name length limit, although the limit
// can still be reached with a single struct field with many fields in it.
func filterArchStructSharded(prop reflect.Type) (filteredProp []reflect.Type, filtered bool) {
	var fields []reflect.StructField

	ptr := prop.Kind() == reflect.Ptr
	if ptr {
		prop = prop.Elem()
	}

	for i := 0; i < prop.NumField(); i++ {
		fields = append(fields, prop.Field(i))
	}

	fields, filtered = filterArchStructFields(fields)
	if !filtered {
		if ptr {
			return []reflect.Type{reflect.PtrTo(prop)}, false
		}
		return []reflect.Type{prop}, false
	}

	if len(fields) == 0 {
		return nil, true
	}

	shards := shardFields(fields, 10)

	for _, shard := range shards {
		s := reflect.StructOf(shard)
		if ptr {
			s = reflect.PtrTo(s)
		}
		filteredProp = append(filteredProp, s)
	}

	return filteredProp, true
}

func shardFields(fields []reflect.StructField, shardSize int) [][]reflect.StructField {
	ret := make([][]reflect.StructField, 0, (len(fields)+shardSize-1)/shardSize)
	for len(fields) > shardSize {
		ret = append(ret, fields[0:shardSize])
		fields = fields[shardSize:]
	}
	if len(fields) > 0 {
		ret = append(ret, fields)
	}
	return ret
}

// createArchType takes a reflect.Type that is either a struct or a pointer to a struct, and returns a list of
// reflect.Type that contains the arch-variant properties inside structs for each architecture, os, target, multilib,
// etc.
func createArchType(props reflect.Type) []reflect.Type {
	propShards, _ := filterArchStructSharded(props)
	if len(propShards) == 0 {
		return nil
	}

	var ret []reflect.Type
	for _, props := range propShards {

		variantFields := func(names []string) []reflect.StructField {
			ret := make([]reflect.StructField, len(names))

			for i, name := range names {
				ret[i].Name = name
				ret[i].Type = props
			}

			return ret
		}

		archFields := make([]reflect.StructField, len(archTypeList))
		for i, arch := range archTypeList {
			variants := []string{}

			for _, archVariant := range archVariants[arch] {
				archVariant := variantReplacer.Replace(archVariant)
				variants = append(variants, proptools.FieldNameForProperty(archVariant))
			}
			for _, feature := range archFeatures[arch] {
				feature := variantReplacer.Replace(feature)
				variants = append(variants, proptools.FieldNameForProperty(feature))
			}

			fields := variantFields(variants)

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
		archType := reflect.StructOf(archFields)

		multilibType := reflect.StructOf(variantFields([]string{"Lib32", "Lib64"}))

		targets := []string{
			"Host",
			"Android64",
			"Android32",
			"Bionic",
			"Linux",
			"Not_windows",
			"Arm_on_x86",
			"Arm_on_x86_64",
		}
		for _, os := range osTypeList {
			targets = append(targets, os.Field)

			for _, archType := range osArchTypeMap[os] {
				targets = append(targets, os.Field+"_"+archType.Name)

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

		targetType := reflect.StructOf(variantFields(targets))
		ret = append(ret, reflect.StructOf([]reflect.StructField{
			{
				Name: "Arch",
				Type: archType,
			},
			{
				Name: "Multilib",
				Type: multilibType,
			},
			{
				Name: "Target",
				Type: targetType,
			},
		}))
	}
	return ret
}

var archPropTypeMap OncePer

func InitArchModule(m Module) {

	base := m.base()

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

		archPropTypes := archPropTypeMap.Once(NewCustomOnceKey(t), func() interface{} {
			return createArchType(t)
		}).([]reflect.Type)

		var archProperties []interface{}
		for _, t := range archPropTypes {
			archProperties = append(archProperties, reflect.New(t).Interface())
		}
		base.archProperties = append(base.archProperties, archProperties)
		m.AddProperties(archProperties...)
	}

	base.customizableProperties = m.GetProperties()
}

var variantReplacer = strings.NewReplacer("-", "_", ".", "_")

func (a *ModuleBase) appendProperties(ctx BottomUpMutatorContext,
	dst interface{}, src reflect.Value, field, srcPrefix string) reflect.Value {

	src = src.FieldByName(field)
	if !src.IsValid() {
		ctx.ModuleErrorf("field %q does not exist", srcPrefix)
		return src
	}

	ret := src

	if src.Kind() == reflect.Struct {
		src = src.FieldByName("BlueprintEmbed")
	}

	order := func(property string,
		dstField, srcField reflect.StructField,
		dstValue, srcValue interface{}) (proptools.Order, error) {
		if proptools.HasTag(dstField, "android", "variant_prepend") {
			return proptools.Prepend, nil
		} else {
			return proptools.Append, nil
		}
	}

	err := proptools.ExtendMatchingProperties([]interface{}{dst}, src.Interface(), nil, order)
	if err != nil {
		if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
			ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
		} else {
			panic(err)
		}
	}

	return ret
}

// Rewrite the module's properties structs to contain arch-specific values.
func (a *ModuleBase) setArchProperties(ctx BottomUpMutatorContext) {
	arch := a.Arch()
	os := a.Os()

	for i := range a.generalProperties {
		genProps := a.generalProperties[i]
		if a.archProperties[i] == nil {
			continue
		}
		for _, archProperties := range a.archProperties[i] {
			archPropValues := reflect.ValueOf(archProperties).Elem()

			archProp := archPropValues.FieldByName("Arch")
			multilibProp := archPropValues.FieldByName("Multilib")
			targetProp := archPropValues.FieldByName("Target")

			var field string
			var prefix string

			// Handle arch-specific properties in the form:
			// arch: {
			//     arm64: {
			//         key: value,
			//     },
			// },
			t := arch.ArchType

			if arch.ArchType != Common {
				field := proptools.FieldNameForProperty(t.Name)
				prefix := "arch." + t.Name
				archStruct := a.appendProperties(ctx, genProps, archProp, field, prefix)

				// Handle arch-variant-specific properties in the form:
				// arch: {
				//     variant: {
				//         key: value,
				//     },
				// },
				v := variantReplacer.Replace(arch.ArchVariant)
				if v != "" {
					field := proptools.FieldNameForProperty(v)
					prefix := "arch." + t.Name + "." + v
					a.appendProperties(ctx, genProps, archStruct, field, prefix)
				}

				// Handle cpu-variant-specific properties in the form:
				// arch: {
				//     variant: {
				//         key: value,
				//     },
				// },
				if arch.CpuVariant != arch.ArchVariant {
					c := variantReplacer.Replace(arch.CpuVariant)
					if c != "" {
						field := proptools.FieldNameForProperty(c)
						prefix := "arch." + t.Name + "." + c
						a.appendProperties(ctx, genProps, archStruct, field, prefix)
					}
				}

				// Handle arch-feature-specific properties in the form:
				// arch: {
				//     feature: {
				//         key: value,
				//     },
				// },
				for _, feature := range arch.ArchFeatures {
					field := proptools.FieldNameForProperty(feature)
					prefix := "arch." + t.Name + "." + feature
					a.appendProperties(ctx, genProps, archStruct, field, prefix)
				}

				// Handle multilib-specific properties in the form:
				// multilib: {
				//     lib32: {
				//         key: value,
				//     },
				// },
				field = proptools.FieldNameForProperty(t.Multilib)
				prefix = "multilib." + t.Multilib
				a.appendProperties(ctx, genProps, multilibProp, field, prefix)
			}

			// Handle host-specific properties in the form:
			// target: {
			//     host: {
			//         key: value,
			//     },
			// },
			if os.Class == Host || os.Class == HostCross {
				field = "Host"
				prefix = "target.host"
				a.appendProperties(ctx, genProps, targetProp, field, prefix)
			}

			// Handle target OS generalities of the form:
			// target: {
			//     bionic: {
			//         key: value,
			//     },
			//     bionic_x86: {
			//         key: value,
			//     },
			// }
			if os.Linux() {
				field = "Linux"
				prefix = "target.linux"
				a.appendProperties(ctx, genProps, targetProp, field, prefix)

				if arch.ArchType != Common {
					field = "Linux_" + arch.ArchType.Name
					prefix = "target.linux_" + arch.ArchType.Name
					a.appendProperties(ctx, genProps, targetProp, field, prefix)
				}
			}

			if os.Bionic() {
				field = "Bionic"
				prefix = "target.bionic"
				a.appendProperties(ctx, genProps, targetProp, field, prefix)

				if arch.ArchType != Common {
					field = "Bionic_" + t.Name
					prefix = "target.bionic_" + t.Name
					a.appendProperties(ctx, genProps, targetProp, field, prefix)
				}
			}

			// Handle target OS properties in the form:
			// target: {
			//     linux_glibc: {
			//         key: value,
			//     },
			//     not_windows: {
			//         key: value,
			//     },
			//     linux_glibc_x86: {
			//         key: value,
			//     },
			//     linux_glibc_arm: {
			//         key: value,
			//     },
			//     android {
			//         key: value,
			//     },
			//     android_arm {
			//         key: value,
			//     },
			//     android_x86 {
			//         key: value,
			//     },
			// },
			field = os.Field
			prefix = "target." + os.Name
			a.appendProperties(ctx, genProps, targetProp, field, prefix)

			if arch.ArchType != Common {
				field = os.Field + "_" + t.Name
				prefix = "target." + os.Name + "_" + t.Name
				a.appendProperties(ctx, genProps, targetProp, field, prefix)
			}

			if (os.Class == Host || os.Class == HostCross) && os != Windows {
				field := "Not_windows"
				prefix := "target.not_windows"
				a.appendProperties(ctx, genProps, targetProp, field, prefix)
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
					a.appendProperties(ctx, genProps, targetProp, field, prefix)
				} else {
					field := "Android32"
					prefix := "target.android32"
					a.appendProperties(ctx, genProps, targetProp, field, prefix)
				}

				if (arch.ArchType == X86 && (hasArmAbi(arch) ||
					hasArmAndroidArch(ctx.Config().Targets[Android]))) ||
					(arch.ArchType == Arm &&
						hasX86AndroidArch(ctx.Config().Targets[Android])) {
					field := "Arm_on_x86"
					prefix := "target.arm_on_x86"
					a.appendProperties(ctx, genProps, targetProp, field, prefix)
				}
				if (arch.ArchType == X86_64 && (hasArmAbi(arch) ||
					hasArmAndroidArch(ctx.Config().Targets[Android]))) ||
					(arch.ArchType == Arm &&
						hasX8664AndroidArch(ctx.Config().Targets[Android])) {
					field := "Arm_on_x86_64"
					prefix := "target.arm_on_x86_64"
					a.appendProperties(ctx, genProps, targetProp, field, prefix)
				}
			}
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

// Convert the arch product variables into a list of targets for each os class structs
func decodeTargetProductVariables(config *config) (map[OsType][]Target, error) {
	variables := config.productVariables

	targets := make(map[OsType][]Target)
	var targetErr error

	addTarget := func(os OsType, archName string, archVariant, cpuVariant *string, abi []string) {
		if targetErr != nil {
			return
		}

		arch, err := decodeArch(os, archName, archVariant, cpuVariant, abi)
		if err != nil {
			targetErr = err
			return
		}

		targets[os] = append(targets[os],
			Target{
				Os:   os,
				Arch: arch,
			})
	}

	if variables.HostArch == nil {
		return nil, fmt.Errorf("No host primary architecture set")
	}

	addTarget(BuildOs, *variables.HostArch, nil, nil, nil)

	if variables.HostSecondaryArch != nil && *variables.HostSecondaryArch != "" {
		addTarget(BuildOs, *variables.HostSecondaryArch, nil, nil, nil)
	}

	if Bool(config.Host_bionic) {
		addTarget(LinuxBionic, "x86_64", nil, nil, nil)
	}

	if String(variables.CrossHost) != "" {
		crossHostOs := osByName(*variables.CrossHost)
		if crossHostOs == NoOsType {
			return nil, fmt.Errorf("Unknown cross host OS %q", *variables.CrossHost)
		}

		if String(variables.CrossHostArch) == "" {
			return nil, fmt.Errorf("No cross-host primary architecture set")
		}

		addTarget(crossHostOs, *variables.CrossHostArch, nil, nil, nil)

		if variables.CrossHostSecondaryArch != nil && *variables.CrossHostSecondaryArch != "" {
			addTarget(crossHostOs, *variables.CrossHostSecondaryArch, nil, nil, nil)
		}
	}

	if variables.DeviceArch != nil && *variables.DeviceArch != "" {
		var target = Android
		if Bool(variables.Fuchsia) {
			target = Fuchsia
		}

		addTarget(target, *variables.DeviceArch, variables.DeviceArchVariant,
			variables.DeviceCpuVariant, variables.DeviceAbi)

		if variables.DeviceSecondaryArch != nil && *variables.DeviceSecondaryArch != "" {
			addTarget(Android, *variables.DeviceSecondaryArch,
				variables.DeviceSecondaryArchVariant, variables.DeviceSecondaryCpuVariant,
				variables.DeviceSecondaryAbi)

			deviceArches := targets[Android]
			if deviceArches[0].Arch.ArchType.Multilib == deviceArches[1].Arch.ArchType.Multilib {
				deviceArches[1].Arch.Native = false
			}
		}
	}

	if targetErr != nil {
		return nil, targetErr
	}

	return targets, nil
}

// hasArmAbi returns true if arch has at least one arm ABI
func hasArmAbi(arch Arch) bool {
	for _, abi := range arch.Abi {
		if strings.HasPrefix(abi, "arm") {
			return true
		}
	}
	return false
}

// hasArmArch returns true if targets has at least arm Android arch
func hasArmAndroidArch(targets []Target) bool {
	for _, target := range targets {
		if target.Os == Android && target.Arch.ArchType == Arm {
			return true
		}
	}
	return false
}

// hasX86Arch returns true if targets has at least x86 Android arch
func hasX86AndroidArch(targets []Target) bool {
	for _, target := range targets {
		if target.Os == Android && target.Arch.ArchType == X86 {
			return true
		}
	}
	return false
}

// hasX8664Arch returns true if targets has at least x86_64 Android arch
func hasX8664AndroidArch(targets []Target) bool {
	for _, target := range targets {
		if target.Os == Android && target.Arch.ArchType == X86_64 {
			return true
		}
	}
	return false
}

type archConfig struct {
	arch        string
	archVariant string
	cpuVariant  string
	abi         []string
}

func getMegaDeviceConfig() []archConfig {
	return []archConfig{
		{"arm", "armv7-a", "generic", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "generic", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a7", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a8", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a9", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a15", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a53", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a53.a57", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a72", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a73", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a75", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "cortex-a76", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "krait", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "kryo", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "kryo385", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "exynos-m1", []string{"armeabi-v7a"}},
		{"arm", "armv7-a-neon", "exynos-m2", []string{"armeabi-v7a"}},
		{"arm64", "armv8-a", "cortex-a53", []string{"arm64-v8a"}},
		{"arm64", "armv8-a", "cortex-a72", []string{"arm64-v8a"}},
		{"arm64", "armv8-a", "cortex-a73", []string{"arm64-v8a"}},
		{"arm64", "armv8-a", "kryo", []string{"arm64-v8a"}},
		{"arm64", "armv8-a", "exynos-m1", []string{"arm64-v8a"}},
		{"arm64", "armv8-a", "exynos-m2", []string{"arm64-v8a"}},
		{"arm64", "armv8-2a", "cortex-a75", []string{"arm64-v8a"}},
		{"arm64", "armv8-2a", "cortex-a76", []string{"arm64-v8a"}},
		{"arm64", "armv8-2a", "kryo385", []string{"arm64-v8a"}},
		{"mips", "mips32-fp", "", []string{"mips"}},
		{"mips", "mips32r2-fp", "", []string{"mips"}},
		{"mips", "mips32r2-fp-xburst", "", []string{"mips"}},
		//{"mips", "mips32r6", "", []string{"mips"}},
		{"mips", "mips32r2dsp-fp", "", []string{"mips"}},
		{"mips", "mips32r2dspr2-fp", "", []string{"mips"}},
		// mips64r2 is mismatching 64r2 and 64r6 libraries during linking to libgcc
		//{"mips64", "mips64r2", "", []string{"mips64"}},
		{"mips64", "mips64r6", "", []string{"mips64"}},
		{"x86", "", "", []string{"x86"}},
		{"x86", "atom", "", []string{"x86"}},
		{"x86", "haswell", "", []string{"x86"}},
		{"x86", "ivybridge", "", []string{"x86"}},
		{"x86", "sandybridge", "", []string{"x86"}},
		{"x86", "silvermont", "", []string{"x86"}},
		{"x86", "stoneyridge", "", []string{"x86"}},
		{"x86", "x86_64", "", []string{"x86"}},
		{"x86_64", "", "", []string{"x86_64"}},
		{"x86_64", "haswell", "", []string{"x86_64"}},
		{"x86_64", "ivybridge", "", []string{"x86_64"}},
		{"x86_64", "sandybridge", "", []string{"x86_64"}},
		{"x86_64", "silvermont", "", []string{"x86_64"}},
		{"x86_64", "stoneyridge", "", []string{"x86_64"}},
	}
}

func getNdkAbisConfig() []archConfig {
	return []archConfig{
		{"arm", "armv7-a", "", []string{"armeabi"}},
		{"arm64", "armv8-a", "", []string{"arm64-v8a"}},
		{"x86", "", "", []string{"x86"}},
		{"x86_64", "", "", []string{"x86_64"}},
	}
}

func decodeArchSettings(os OsType, archConfigs []archConfig) ([]Target, error) {
	var ret []Target

	for _, config := range archConfigs {
		arch, err := decodeArch(os, config.arch, &config.archVariant,
			&config.cpuVariant, config.abi)
		if err != nil {
			return nil, err
		}
		arch.Native = false
		ret = append(ret, Target{
			Os:   Android,
			Arch: arch,
		})
	}

	return ret, nil
}

// Convert a set of strings from product variables into a single Arch struct
func decodeArch(os OsType, arch string, archVariant, cpuVariant *string, abi []string) (Arch, error) {
	stringPtr := func(p *string) string {
		if p != nil {
			return *p
		}
		return ""
	}

	archType, ok := archTypeMap[arch]
	if !ok {
		return Arch{}, fmt.Errorf("unknown arch %q", arch)
	}

	a := Arch{
		ArchType:    archType,
		ArchVariant: stringPtr(archVariant),
		CpuVariant:  stringPtr(cpuVariant),
		Abi:         abi,
		Native:      true,
	}

	if a.ArchVariant == a.ArchType.Name || a.ArchVariant == "generic" {
		a.ArchVariant = ""
	}

	if a.CpuVariant == a.ArchType.Name || a.CpuVariant == "generic" {
		a.CpuVariant = ""
	}

	for i := 0; i < len(a.Abi); i++ {
		if a.Abi[i] == "" {
			a.Abi = append(a.Abi[:i], a.Abi[i+1:]...)
			i--
		}
	}

	if a.ArchVariant == "" {
		if featureMap, ok := defaultArchFeatureMap[os]; ok {
			a.ArchFeatures = featureMap[archType]
		}
	} else {
		if featureMap, ok := archFeatureMap[archType]; ok {
			a.ArchFeatures = featureMap[a.ArchVariant]
		}
	}

	return a, nil
}

func filterMultilibTargets(targets []Target, multilib string) []Target {
	var ret []Target
	for _, t := range targets {
		if t.Arch.ArchType.Multilib == multilib {
			ret = append(ret, t)
		}
	}
	return ret
}

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

func firstTarget(targets []Target, filters ...string) []Target {
	for _, filter := range filters {
		buildTargets := filterMultilibTargets(targets, filter)
		if len(buildTargets) > 0 {
			return buildTargets[:1]
		}
	}
	return nil
}

// Use the module multilib setting to select one or more targets from a target list
func decodeMultilibTargets(multilib string, targets []Target, prefer32 bool) ([]Target, error) {
	buildTargets := []Target{}

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
	case "prefer32":
		buildTargets = filterMultilibTargets(targets, "lib32")
		if len(buildTargets) == 0 {
			buildTargets = filterMultilibTargets(targets, "lib64")
		}
	default:
		return nil, fmt.Errorf(`compile_multilib must be "both", "first", "32", "64", or "prefer32" found %q`,
			multilib)
	}

	return buildTargets, nil
}
