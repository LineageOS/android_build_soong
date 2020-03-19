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

	"android/soong/android"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
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

var staticAndSharedLibrarySdkMemberType = &librarySdkMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName: "native_libs",
		SupportsSdk:  true,
	},
	prebuiltModuleType: "cc_prebuilt_library",
	linkTypes:          []string{"static", "shared"},
}

func init() {
	// Register sdk member types.
	android.RegisterSdkMemberType(sharedLibrarySdkMemberType)
	android.RegisterSdkMemberType(staticLibrarySdkMemberType)
	android.RegisterSdkMemberType(staticAndSharedLibrarySdkMemberType)
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

func (mt *librarySdkMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	pbm := ctx.SnapshotBuilder().AddPrebuiltModule(member, mt.prebuiltModuleType)

	ccModule := member.Variants()[0].(*Module)

	sdkVersion := ccModule.SdkVersion()
	if sdkVersion != "" {
		pbm.AddProperty("sdk_version", sdkVersion)
	}

	stl := ccModule.stl.Properties.Stl
	if stl != nil {
		pbm.AddProperty("stl", proptools.String(stl))
	}
	return pbm
}

func (mt *librarySdkMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &nativeLibInfoProperties{memberType: mt}
}

func isGeneratedHeaderDirectory(p android.Path) bool {
	_, gen := p.(android.WritablePath)
	return gen
}

type includeDirsProperty struct {
	// Accessor to retrieve the paths
	pathsGetter func(libInfo *nativeLibInfoProperties) android.Paths

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
		pathsGetter:  func(libInfo *nativeLibInfoProperties) android.Paths { return libInfo.ExportedIncludeDirs },
		propertyName: "export_include_dirs",
		snapshotDir:  nativeIncludeDir,
		copy:         true,
		dirs:         true,
	},
	{
		// ExportedSystemIncludeDirs lists directories that contains some system header files to
		// be copied into a directory in the snapshot. The snapshot directories must be added to
		// the export_system_include_dirs property in the prebuilt module in the snapshot.
		pathsGetter:  func(libInfo *nativeLibInfoProperties) android.Paths { return libInfo.ExportedSystemIncludeDirs },
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
		pathsGetter:  func(libInfo *nativeLibInfoProperties) android.Paths { return libInfo.exportedGeneratedIncludeDirs },
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
		pathsGetter:  func(libInfo *nativeLibInfoProperties) android.Paths { return libInfo.exportedGeneratedHeaders },
		propertyName: "",
		snapshotDir:  nativeGeneratedIncludeDir,
		copy:         true,
		dirs:         false,
	},
}

// Add properties that may, or may not, be arch specific.
func addPossiblyArchSpecificProperties(sdkModuleContext android.ModuleContext, builder android.SnapshotBuilder, libInfo *nativeLibInfoProperties, outputProperties android.BpPropertySet) {

	// Copy the generated library to the snapshot and add a reference to it in the .bp module.
	if libInfo.outputFile != nil {
		nativeLibraryPath := nativeLibraryPathFor(libInfo)
		builder.CopyToSnapshot(libInfo.outputFile, nativeLibraryPath)
		outputProperties.AddProperty("srcs", []string{nativeLibraryPath})
	}

	if len(libInfo.SharedLibs) > 0 {
		outputProperties.AddPropertyWithTag("shared_libs", libInfo.SharedLibs, builder.SdkMemberReferencePropertyTag(false))
	}

	if len(libInfo.SystemSharedLibs) > 0 {
		outputProperties.AddPropertyWithTag("system_shared_libs", libInfo.SystemSharedLibs, builder.SdkMemberReferencePropertyTag(false))
	}

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
func nativeLibraryPathFor(lib *nativeLibInfoProperties) string {
	return filepath.Join(lib.OsPrefix(), lib.archType,
		nativeStubDir, lib.outputFile.Base())
}

// nativeLibInfoProperties represents properties of a native lib
//
// The exported (capitalized) fields will be examined and may be changed during common value extraction.
// The unexported fields will be left untouched.
type nativeLibInfoProperties struct {
	android.SdkMemberPropertiesBase

	memberType *librarySdkMemberType

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

	// The set of shared libraries
	//
	// This field is exported as its contents may not be arch specific.
	SharedLibs []string

	// The set of system shared libraries
	//
	// This field is exported as its contents may not be arch specific.
	SystemSharedLibs []string

	// outputFile is not exported as it is always arch specific.
	outputFile android.Path
}

func (p *nativeLibInfoProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	ccModule := variant.(*Module)

	// If the library has some link types then it produces an output binary file, otherwise it
	// is header only.
	if p.memberType.linkTypes != nil {
		p.outputFile = ccModule.OutputFile().Path()
	}

	// Separate out the generated include dirs (which are arch specific) from the
	// include dirs (which may not be).
	exportedIncludeDirs, exportedGeneratedIncludeDirs := android.FilterPathListPredicate(
		ccModule.ExportedIncludeDirs(), isGeneratedHeaderDirectory)

	p.name = variant.Name()
	p.archType = ccModule.Target().Arch.ArchType.String()

	// Make sure that the include directories are unique.
	p.ExportedIncludeDirs = android.FirstUniquePaths(exportedIncludeDirs)
	p.exportedGeneratedIncludeDirs = android.FirstUniquePaths(exportedGeneratedIncludeDirs)
	p.ExportedSystemIncludeDirs = android.FirstUniquePaths(ccModule.ExportedSystemIncludeDirs())

	p.ExportedFlags = ccModule.ExportedFlags()
	if ccModule.linker != nil {
		specifiedDeps := specifiedDeps{}
		specifiedDeps = ccModule.linker.linkerSpecifiedDeps(specifiedDeps)

		p.SharedLibs = specifiedDeps.sharedLibs
		p.SystemSharedLibs = specifiedDeps.systemSharedLibs
	}
	p.exportedGeneratedHeaders = ccModule.ExportedGeneratedHeaders()
}

func (p *nativeLibInfoProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	addPossiblyArchSpecificProperties(ctx.SdkModuleContext(), ctx.SnapshotBuilder(), p, propertySet)
}
