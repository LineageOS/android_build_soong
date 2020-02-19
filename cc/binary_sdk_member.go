// Copyright 2020 Google Inc. All rights reserved.
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

package cc

import (
	"path/filepath"

	"android/soong/android"
	"github.com/google/blueprint"
)

func init() {
	android.RegisterSdkMemberType(ccBinarySdkMemberType)
}

var ccBinarySdkMemberType = &binarySdkMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName: "native_binaries",
	},
}

type binarySdkMemberType struct {
	android.SdkMemberTypeBase
}

func (mt *binarySdkMemberType) AddDependencies(mctx android.BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string) {
	targets := mctx.MultiTargets()
	for _, lib := range names {
		for _, target := range targets {
			name, version := StubsLibNameAndVersion(lib)
			if version == "" {
				version = LatestStubsVersionFor(mctx.Config(), name)
			}
			mctx.AddFarVariationDependencies(append(target.Variations(), []blueprint.Variation{
				{Mutator: "version", Variation: version},
			}...), dependencyTag, name)
		}
	}
}

func (mt *binarySdkMemberType) IsInstance(module android.Module) bool {
	// Check the module to see if it can be used with this module type.
	if m, ok := module.(*Module); ok {
		for _, allowableMemberType := range m.sdkMemberTypes {
			if allowableMemberType == mt {
				return true
			}
		}
	}

	return false
}

func (mt *binarySdkMemberType) BuildSnapshot(sdkModuleContext android.ModuleContext, builder android.SnapshotBuilder, member android.SdkMember) {
	info := mt.organizeVariants(member)
	buildSharedNativeBinarySnapshot(info, builder, member)
}

// Organize the variants by architecture.
func (mt *binarySdkMemberType) organizeVariants(member android.SdkMember) *nativeBinaryInfo {
	memberName := member.Name()
	info := &nativeBinaryInfo{
		name:       memberName,
		memberType: mt,
	}

	for _, variant := range member.Variants() {
		ccModule := variant.(*Module)

		info.archVariantProperties = append(info.archVariantProperties, nativeBinaryInfoProperties{
			name:       memberName,
			archType:   ccModule.Target().Arch.ArchType.String(),
			outputFile: ccModule.OutputFile().Path(),
		})
	}

	// Initialize the unexported properties that will not be set during the
	// extraction process.
	info.commonProperties.name = memberName

	// Extract common properties from the arch specific properties.
	extractCommonProperties(&info.commonProperties, info.archVariantProperties)

	return info
}

func buildSharedNativeBinarySnapshot(info *nativeBinaryInfo, builder android.SnapshotBuilder, member android.SdkMember) {
	pbm := builder.AddPrebuiltModule(member, "cc_prebuilt_binary")
	pbm.AddProperty("compile_multilib", "both")
	archProperties := pbm.AddPropertySet("arch")
	for _, av := range info.archVariantProperties {
		archTypeProperties := archProperties.AddPropertySet(av.archType)
		archTypeProperties.AddProperty("srcs", []string{nativeBinaryPathFor(av)})

		builder.CopyToSnapshot(av.outputFile, nativeBinaryPathFor(av))
	}
}

const (
	nativeBinaryDir = "bin"
)

// path to the native binary. Relative to <sdk_root>/<api_dir>
func nativeBinaryPathFor(lib nativeBinaryInfoProperties) string {
	return filepath.Join(lib.archType,
		nativeBinaryDir, lib.outputFile.Base())
}

// nativeBinaryInfoProperties represents properties of a native binary
//
// The exported (capitalized) fields will be examined and may be changed during common value extraction.
// The unexported fields will be left untouched.
type nativeBinaryInfoProperties struct {
	// The name of the library, is not exported as this must not be changed during optimization.
	name string

	// archType is not exported as if set (to a non default value) it is always arch specific.
	// This is "" for common properties.
	archType string

	// outputFile is not exported as it is always arch specific.
	outputFile android.Path
}

// nativeBinaryInfo represents a collection of arch-specific modules having the same name
type nativeBinaryInfo struct {
	name                  string
	memberType            *binarySdkMemberType
	archVariantProperties []nativeBinaryInfoProperties
	commonProperties      nativeBinaryInfoProperties
}
