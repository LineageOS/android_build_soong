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

package cc

import (
	"path/filepath"
	"reflect"

	"android/soong/android"
	"github.com/google/blueprint"
)

// This file contains support for using cc library modules within an sdk.

var sharedLibrarySdkMemberType = &librarySdkMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName: "native_shared_libs",
		SupportsSdk:  true,
	},
	prebuiltModuleType: "cc_prebuilt_library_shared",
	linkTypes:          []string{"shared"},
}

var staticLibrarySdkMemberType = &librarySdkMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName: "native_static_libs",
		SupportsSdk:  true,
	},
	prebuiltModuleType: "cc_prebuilt_library_static",
	linkTypes:          []string{"static"},
}

func init() {
	// Register sdk member types.
	android.RegisterSdkMemberType(sharedLibrarySdkMemberType)
	android.RegisterSdkMemberType(staticLibrarySdkMemberType)
}

type librarySdkMemberType struct {
	android.SdkMemberTypeBase

	prebuiltModuleType string

	// The set of link types supported, set of "static", "shared".
	linkTypes []string
}

func (mt *librarySdkMemberType) AddDependencies(mctx android.BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string) {
	targets := mctx.MultiTargets()
	for _, lib := range names {
		for _, target := range targets {
			name, version := StubsLibNameAndVersion(lib)
			if version == "" {
				version = LatestStubsVersionFor(mctx.Config(), name)
			}
			for _, linkType := range mt.linkTypes {
				mctx.AddFarVariationDependencies(append(target.Variations(), []blueprint.Variation{
					{Mutator: "image", Variation: android.CoreVariation},
					{Mutator: "link", Variation: linkType},
					{Mutator: "version", Variation: version},
				}...), dependencyTag, name)
			}
		}
	}
}

func (mt *librarySdkMemberType) IsInstance(module android.Module) bool {
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

// copy exported header files and stub *.so files
func (mt *librarySdkMemberType) BuildSnapshot(sdkModuleContext android.ModuleContext, builder android.SnapshotBuilder, member android.SdkMember) {
	info := mt.organizeVariants(member)
	buildSharedNativeLibSnapshot(sdkModuleContext, info, builder, member)
}

// Organize the variants by architecture.
func (mt *librarySdkMemberType) organizeVariants(member android.SdkMember) *nativeLibInfo {
	memberName := member.Name()
	info := &nativeLibInfo{
		name:       memberName,
		memberType: mt,
	}

	for _, variant := range member.Variants() {
		ccModule := variant.(*Module)

		// Separate out the generated include dirs (which are arch specific) from the
		// include dirs (which may not be).
		exportedIncludeDirs, exportedGeneratedIncludeDirs := android.FilterPathListPredicate(
			ccModule.ExportedIncludeDirs(), isGeneratedHeaderDirectory)

		info.archVariantProperties = append(info.archVariantProperties, nativeLibInfoProperties{
			name:                         memberName,
			archType:                     ccModule.Target().Arch.ArchType.String(),
			ExportedIncludeDirs:          exportedIncludeDirs,
			exportedGeneratedIncludeDirs: exportedGeneratedIncludeDirs,
			ExportedSystemIncludeDirs:    ccModule.ExportedSystemIncludeDirs(),
			ExportedFlags:                ccModule.ExportedFlags(),
			exportedGeneratedHeaders:     ccModule.ExportedGeneratedHeaders(),
			outputFile:                   ccModule.OutputFile().Path(),
		})
	}

	// Initialize the unexported properties that will not be set during the
	// extraction process.
	info.commonProperties.name = memberName

	// Extract common properties from the arch specific properties.
	extractCommonProperties(&info.commonProperties, info.archVariantProperties)

	return info
}

func isGeneratedHeaderDirectory(p android.Path) bool {
	_, gen := p.(android.WritablePath)
	return gen
}

// Extract common properties from a slice of property structures of the same type.
//
// All the property structures must be of the same type.
// commonProperties - must be a pointer to the structure into which common properties will be added.
// inputPropertiesSlice - must be a slice of input properties structures.
//
// Iterates over each exported field (capitalized name) and checks to see whether they
// have the same value (using DeepEquals) across all the input properties. If it does not then no
// change is made. Otherwise, the common value is stored in the field in the commonProperties
// and the field in each of the input properties structure is set to its default value.
func extractCommonProperties(commonProperties interface{}, inputPropertiesSlice interface{}) {
	commonStructValue := reflect.ValueOf(commonProperties).Elem()
	propertiesStructType := commonStructValue.Type()

	// Create an empty structure from which default values for the field can be copied.
	emptyStructValue := reflect.New(propertiesStructType).Elem()

	for f := 0; f < propertiesStructType.NumField(); f++ {
		// Check to see if all the structures have the same value for the field. The commonValue
		// is nil on entry to the loop and if it is nil on exit then there is no common value,
		// otherwise it points to the common value.
		var commonValue *reflect.Value
		sliceValue := reflect.ValueOf(inputPropertiesSlice)

		for i := 0; i < sliceValue.Len(); i++ {
			structValue := sliceValue.Index(i)
			fieldValue := structValue.Field(f)
			if !fieldValue.CanInterface() {
				// The field is not exported so ignore it.
				continue
			}

			if commonValue == nil {
				// Use the first value as the commonProperties value.
				commonValue = &fieldValue
			} else {
				// If the value does not match the current common value then there is
				// no value in common so break out.
				if !reflect.DeepEqual(fieldValue.Interface(), commonValue.Interface()) {
					commonValue = nil
					break
				}
			}
		}

		// If the fields all have a common value then store it in the common struct field
		// and set the input struct's field to the empty value.
		if commonValue != nil {
			emptyValue := emptyStructValue.Field(f)
			commonStructValue.Field(f).Set(*commonValue)
			for i := 0; i < sliceValue.Len(); i++ {
				structValue := sliceValue.Index(i)
				fieldValue := structValue.Field(f)
				fieldValue.Set(emptyValue)
			}
		}
	}
}

func buildSharedNativeLibSnapshot(sdkModuleContext android.ModuleContext, info *nativeLibInfo, builder android.SnapshotBuilder, member android.SdkMember) {
	// a function for emitting include dirs
	addExportedDirCopyCommandsForNativeLibs := func(lib nativeLibInfoProperties) {
		// Do not include exportedGeneratedIncludeDirs in the list of directories whose
		// contents are copied as they are copied from exportedGeneratedHeaders below.
		includeDirs := lib.ExportedIncludeDirs
		includeDirs = append(includeDirs, lib.ExportedSystemIncludeDirs...)
		for _, dir := range includeDirs {
			// lib.ArchType is "" for common properties.
			targetDir := filepath.Join(lib.archType, nativeIncludeDir)

			// TODO(jiyong) copy headers having other suffixes
			headers, _ := sdkModuleContext.GlobWithDeps(dir.String()+"/**/*.h", nil)
			for _, file := range headers {
				src := android.PathForSource(sdkModuleContext, file)
				dest := filepath.Join(targetDir, file)
				builder.CopyToSnapshot(src, dest)
			}
		}

		genHeaders := lib.exportedGeneratedHeaders
		for _, file := range genHeaders {
			// lib.ArchType is "" for common properties.
			targetDir := filepath.Join(lib.archType, nativeGeneratedIncludeDir)

			dest := filepath.Join(targetDir, lib.name, file.Rel())
			builder.CopyToSnapshot(file, dest)
		}
	}

	addExportedDirCopyCommandsForNativeLibs(info.commonProperties)

	// for each architecture
	for _, av := range info.archVariantProperties {
		builder.CopyToSnapshot(av.outputFile, nativeLibraryPathFor(av))

		addExportedDirCopyCommandsForNativeLibs(av)
	}

	info.generatePrebuiltLibrary(sdkModuleContext, builder, member)
}

func (info *nativeLibInfo) generatePrebuiltLibrary(sdkModuleContext android.ModuleContext, builder android.SnapshotBuilder, member android.SdkMember) {

	pbm := builder.AddPrebuiltModule(member, info.memberType.prebuiltModuleType)

	addPossiblyArchSpecificProperties(info.commonProperties, pbm)

	archProperties := pbm.AddPropertySet("arch")
	for _, av := range info.archVariantProperties {
		archTypeProperties := archProperties.AddPropertySet(av.archType)
		// Add any arch specific properties inside the appropriate arch: {<arch>: {...}} block
		archTypeProperties.AddProperty("srcs", []string{nativeLibraryPathFor(av)})

		addPossiblyArchSpecificProperties(av, archTypeProperties)
	}
	pbm.AddProperty("stl", "none")
	pbm.AddProperty("system_shared_libs", []string{})
}

// Add properties that may, or may not, be arch specific.
func addPossiblyArchSpecificProperties(libInfo nativeLibInfoProperties, outputProperties android.BpPropertySet) {
	addExportedDirsForNativeLibs(libInfo, outputProperties, false /*systemInclude*/)
	addExportedDirsForNativeLibs(libInfo, outputProperties, true /*systemInclude*/)
}

// a function for emitting include dirs
func addExportedDirsForNativeLibs(lib nativeLibInfoProperties, properties android.BpPropertySet, systemInclude bool) {
	includeDirs := nativeIncludeDirPathsFor(lib, systemInclude)
	if len(includeDirs) == 0 {
		return
	}
	var propertyName string
	if !systemInclude {
		propertyName = "export_include_dirs"
	} else {
		propertyName = "export_system_include_dirs"
	}
	properties.AddProperty(propertyName, includeDirs)
}

const (
	nativeIncludeDir          = "include"
	nativeGeneratedIncludeDir = "include_gen"
	nativeStubDir             = "lib"
)

// path to the native library. Relative to <sdk_root>/<api_dir>
func nativeLibraryPathFor(lib nativeLibInfoProperties) string {
	return filepath.Join(lib.archType,
		nativeStubDir, lib.outputFile.Base())
}

// paths to the include dirs of a native shared library. Relative to <sdk_root>/<api_dir>
func nativeIncludeDirPathsFor(lib nativeLibInfoProperties, systemInclude bool) []string {
	var result []string
	var includeDirs []android.Path
	if !systemInclude {
		// Include the generated include dirs in the exported include dirs.
		includeDirs = append(lib.ExportedIncludeDirs, lib.exportedGeneratedIncludeDirs...)
	} else {
		includeDirs = lib.ExportedSystemIncludeDirs
	}
	for _, dir := range includeDirs {
		var path string
		if isGeneratedHeaderDirectory(dir) {
			path = filepath.Join(nativeGeneratedIncludeDir, lib.name)
		} else {
			path = filepath.Join(nativeIncludeDir, dir.String())
		}

		// lib.ArchType is "" for common properties.
		path = filepath.Join(lib.archType, path)
		result = append(result, path)
	}
	return result
}

// nativeLibInfoProperties represents properties of a native lib
//
// The exported (capitalized) fields will be examined and may be changed during common value extraction.
// The unexported fields will be left untouched.
type nativeLibInfoProperties struct {
	// The name of the library, is not exported as this must not be changed during optimization.
	name string

	// archType is not exported as if set (to a non default value) it is always arch specific.
	// This is "" for common properties.
	archType string

	// The list of possibly common exported include dirs.
	//
	// This field is exported as its contents may not be arch specific.
	ExportedIncludeDirs android.Paths

	// The list of arch specific exported generated include dirs.
	//
	// This field is not exported as its contents are always arch specific.
	exportedGeneratedIncludeDirs android.Paths

	// The list of arch specific exported generated header files.
	//
	// This field is not exported as its contents are is always arch specific.
	exportedGeneratedHeaders android.Paths

	// The list of possibly common exported system include dirs.
	//
	// This field is exported as its contents may not be arch specific.
	ExportedSystemIncludeDirs android.Paths

	// The list of possibly common exported flags.
	//
	// This field is exported as its contents may not be arch specific.
	ExportedFlags []string

	// outputFile is not exported as it is always arch specific.
	outputFile android.Path
}

// nativeLibInfo represents a collection of arch-specific modules having the same name
type nativeLibInfo struct {
	name                  string
	memberType            *librarySdkMemberType
	archVariantProperties []nativeLibInfoProperties
	commonProperties      nativeLibInfoProperties
}
