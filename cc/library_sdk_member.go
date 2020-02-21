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
			if mt.linkTypes == nil {
				mctx.AddFarVariationDependencies(append(target.Variations(), []blueprint.Variation{
					{Mutator: "image", Variation: android.CoreVariation},
					{Mutator: "version", Variation: version},
				}...), dependencyTag, name)
			} else {
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
	info.generatePrebuiltLibrary(sdkModuleContext, builder, member)
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

func (info *nativeLibInfo) generatePrebuiltLibrary(sdkModuleContext android.ModuleContext, builder android.SnapshotBuilder, member android.SdkMember) {

	pbm := builder.AddPrebuiltModule(member, info.memberType.prebuiltModuleType)

	addPossiblyArchSpecificProperties(sdkModuleContext, builder, info.commonProperties, pbm)

	archProperties := pbm.AddPropertySet("arch")
	for _, av := range info.archVariantProperties {
		archTypeProperties := archProperties.AddPropertySet(av.archType)

		// If the library has some link types then it produces an output binary file, otherwise it
		// is header only.
		if info.memberType.linkTypes != nil {
			// Copy the generated library to the snapshot and add a reference to it in the .bp module.
			nativeLibraryPath := nativeLibraryPathFor(av)
			builder.CopyToSnapshot(av.outputFile, nativeLibraryPath)
			archTypeProperties.AddProperty("srcs", []string{nativeLibraryPath})
		}

		// Add any arch specific properties inside the appropriate arch: {<arch>: {...}} block
		addPossiblyArchSpecificProperties(sdkModuleContext, builder, av, archTypeProperties)
	}
	pbm.AddProperty("stl", "none")
	pbm.AddProperty("system_shared_libs", []string{})
}

type includeDirsProperty struct {
	// Accessor to retrieve the paths
	pathsGetter func(libInfo nativeLibInfoProperties) android.Paths

	// The name of the property in the prebuilt library, "" means there is no property.
	propertyName string

	// The directory within the snapshot directory into which items should be copied.
	snapshotDir string

	// True if the items on the path should be copied.
	copy bool

	// True if the paths represent directories, files if they represent files.
	dirs bool
}

var includeDirProperties = []includeDirsProperty{
	{
		// ExportedIncludeDirs lists directories that contains some header files to be
		// copied into a directory in the snapshot. The snapshot directories must be added to
		// the export_include_dirs property in the prebuilt module in the snapshot.
		pathsGetter:  func(libInfo nativeLibInfoProperties) android.Paths { return libInfo.ExportedIncludeDirs },
		propertyName: "export_include_dirs",
		snapshotDir:  nativeIncludeDir,
		copy:         true,
		dirs:         true,
	},
	{
		// ExportedSystemIncludeDirs lists directories that contains some system header files to
		// be copied into a directory in the snapshot. The snapshot directories must be added to
		// the export_system_include_dirs property in the prebuilt module in the snapshot.
		pathsGetter:  func(libInfo nativeLibInfoProperties) android.Paths { return libInfo.ExportedSystemIncludeDirs },
		propertyName: "export_system_include_dirs",
		snapshotDir:  nativeIncludeDir,
		copy:         true,
		dirs:         true,
	},
	{
		// exportedGeneratedIncludeDirs lists directories that contains some header files
		// that are explicitly listed in the exportedGeneratedHeaders property. So, the contents
		// of these directories do not need to be copied, but these directories do need adding to
		// the export_include_dirs property in the prebuilt module in the snapshot.
		pathsGetter:  func(libInfo nativeLibInfoProperties) android.Paths { return libInfo.exportedGeneratedIncludeDirs },
		propertyName: "export_include_dirs",
		snapshotDir:  nativeGeneratedIncludeDir,
		copy:         false,
		dirs:         true,
	},
	{
		// exportedGeneratedHeaders lists header files that are in one of the directories
		// specified in exportedGeneratedIncludeDirs must be copied into the snapshot.
		// As they are in a directory in exportedGeneratedIncludeDirs they do not need adding to a
		// property in the prebuilt module in the snapshot.
		pathsGetter:  func(libInfo nativeLibInfoProperties) android.Paths { return libInfo.exportedGeneratedHeaders },
		propertyName: "",
		snapshotDir:  nativeGeneratedIncludeDir,
		copy:         true,
		dirs:         false,
	},
}

// Add properties that may, or may not, be arch specific.
func addPossiblyArchSpecificProperties(sdkModuleContext android.ModuleContext, builder android.SnapshotBuilder, libInfo nativeLibInfoProperties, outputProperties android.BpPropertySet) {

	// Map from property name to the include dirs to add to the prebuilt module in the snapshot.
	includeDirs := make(map[string][]string)

	// Iterate over each include directory property, copying files and collating property
	// values where necessary.
	for _, propertyInfo := range includeDirProperties {
		// Calculate the base directory in the snapshot into which the files will be copied.
		// lib.ArchType is "" for common properties.
		targetDir := filepath.Join(libInfo.archType, propertyInfo.snapshotDir)

		propertyName := propertyInfo.propertyName

		// Iterate over each path in one of the include directory properties.
		for _, path := range propertyInfo.pathsGetter(libInfo) {

			// Copy the files/directories when necessary.
			if propertyInfo.copy {
				if propertyInfo.dirs {
					// When copying a directory glob and copy all the headers within it.
					// TODO(jiyong) copy headers having other suffixes
					headers, _ := sdkModuleContext.GlobWithDeps(path.String()+"/**/*.h", nil)
					for _, file := range headers {
						src := android.PathForSource(sdkModuleContext, file)
						dest := filepath.Join(targetDir, file)
						builder.CopyToSnapshot(src, dest)
					}
				} else {
					// Otherwise, just copy the files.
					dest := filepath.Join(targetDir, libInfo.name, path.Rel())
					builder.CopyToSnapshot(path, dest)
				}
			}

			// Only directories are added to a property.
			if propertyInfo.dirs {
				var snapshotPath string
				if isGeneratedHeaderDirectory(path) {
					snapshotPath = filepath.Join(targetDir, libInfo.name)
				} else {
					snapshotPath = filepath.Join(targetDir, path.String())
				}

				includeDirs[propertyName] = append(includeDirs[propertyName], snapshotPath)
			}
		}
	}

	// Add the collated include dir properties to the output.
	for property, dirs := range includeDirs {
		outputProperties.AddProperty(property, dirs)
	}
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
