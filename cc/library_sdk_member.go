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
		PropertyName:    "native_shared_libs",
		SupportsSdk:     true,
		HostOsDependent: true,
	},
	prebuiltModuleType: "cc_prebuilt_library_shared",
	linkTypes:          []string{"shared"},
}

var staticLibrarySdkMemberType = &librarySdkMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName:    "native_static_libs",
		SupportsSdk:     true,
		HostOsDependent: true,
	},
	prebuiltModuleType: "cc_prebuilt_library_static",
	linkTypes:          []string{"static"},
}

var staticAndSharedLibrarySdkMemberType = &librarySdkMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName:    "native_libs",
		SupportsSdk:     true,
		HostOsDependent: true,
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

	noOutputFiles bool // True if there are no srcs files.

	// The set of link types supported. A set of "static", "shared", or nil to
	// skip link type variations.
	linkTypes []string
}

func (mt *librarySdkMemberType) AddDependencies(mctx android.BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string) {
	targets := mctx.MultiTargets()
	for _, lib := range names {
		for _, target := range targets {
			name, version := StubsLibNameAndVersion(lib)
			if version == "" {
				version = "latest"
			}
			variations := target.Variations()
			if mctx.Device() {
				variations = append(variations,
					blueprint.Variation{Mutator: "image", Variation: android.CoreVariation})
			}
			if mt.linkTypes == nil {
				mctx.AddFarVariationDependencies(variations, dependencyTag, name)
			} else {
				for _, linkType := range mt.linkTypes {
					libVariations := append(variations,
						blueprint.Variation{Mutator: "link", Variation: linkType})
					if mctx.Device() && linkType == "shared" {
						libVariations = append(libVariations,
							blueprint.Variation{Mutator: "version", Variation: version})
					}
					mctx.AddFarVariationDependencies(libVariations, dependencyTag, name)
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

	if proptools.Bool(ccModule.Properties.Recovery_available) {
		pbm.AddProperty("recovery_available", true)
	}

	if proptools.Bool(ccModule.VendorProperties.Vendor_available) {
		pbm.AddProperty("vendor_available", true)
	}

	if proptools.Bool(ccModule.VendorProperties.Odm_available) {
		pbm.AddProperty("odm_available", true)
	}

	if proptools.Bool(ccModule.VendorProperties.Product_available) {
		pbm.AddProperty("product_available", true)
	}

	sdkVersion := ccModule.SdkVersion()
	if sdkVersion != "" {
		pbm.AddProperty("sdk_version", sdkVersion)
	}

	stl := ccModule.stl.Properties.Stl
	if stl != nil {
		pbm.AddProperty("stl", proptools.String(stl))
	}

	if lib, ok := ccModule.linker.(*libraryDecorator); ok {
		uhs := lib.Properties.Unique_host_soname
		if uhs != nil {
			pbm.AddProperty("unique_host_soname", proptools.Bool(uhs))
		}
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
		// ExportedGeneratedIncludeDirs lists directories that contains some header files
		// that are explicitly listed in the ExportedGeneratedHeaders property. So, the contents
		// of these directories do not need to be copied, but these directories do need adding to
		// the export_include_dirs property in the prebuilt module in the snapshot.
		pathsGetter:  func(libInfo *nativeLibInfoProperties) android.Paths { return libInfo.ExportedGeneratedIncludeDirs },
		propertyName: "export_include_dirs",
		snapshotDir:  nativeGeneratedIncludeDir,
		copy:         false,
		dirs:         true,
	},
	{
		// ExportedGeneratedHeaders lists header files that are in one of the directories
		// specified in ExportedGeneratedIncludeDirs must be copied into the snapshot.
		// As they are in a directory in ExportedGeneratedIncludeDirs they do not need adding to a
		// property in the prebuilt module in the snapshot.
		pathsGetter:  func(libInfo *nativeLibInfoProperties) android.Paths { return libInfo.ExportedGeneratedHeaders },
		propertyName: "",
		snapshotDir:  nativeGeneratedIncludeDir,
		copy:         true,
		dirs:         false,
	},
}

// Add properties that may, or may not, be arch specific.
func addPossiblyArchSpecificProperties(sdkModuleContext android.ModuleContext, builder android.SnapshotBuilder, libInfo *nativeLibInfoProperties, outputProperties android.BpPropertySet) {

	outputProperties.AddProperty("sanitize", &libInfo.Sanitize)

	// Copy the generated library to the snapshot and add a reference to it in the .bp module.
	if libInfo.outputFile != nil {
		nativeLibraryPath := nativeLibraryPathFor(libInfo)
		builder.CopyToSnapshot(libInfo.outputFile, nativeLibraryPath)
		outputProperties.AddProperty("srcs", []string{nativeLibraryPath})
	}

	if len(libInfo.SharedLibs) > 0 {
		outputProperties.AddPropertyWithTag("shared_libs", libInfo.SharedLibs, builder.SdkMemberReferencePropertyTag(false))
	}

	// SystemSharedLibs needs to be propagated if it's a list, even if it's empty,
	// so check for non-nil instead of nonzero length.
	if libInfo.SystemSharedLibs != nil {
		outputProperties.AddPropertyWithTag("system_shared_libs", libInfo.SystemSharedLibs, builder.SdkMemberReferencePropertyTag(false))
	}

	// Map from property name to the include dirs to add to the prebuilt module in the snapshot.
	includeDirs := make(map[string][]string)

	// Iterate over each include directory property, copying files and collating property
	// values where necessary.
	for _, propertyInfo := range includeDirProperties {
		// Calculate the base directory in the snapshot into which the files will be copied.
		// lib.archType is "" for common properties.
		targetDir := filepath.Join(libInfo.OsPrefix(), libInfo.archType, propertyInfo.snapshotDir)

		propertyName := propertyInfo.propertyName

		// Iterate over each path in one of the include directory properties.
		for _, path := range propertyInfo.pathsGetter(libInfo) {
			inputPath := path.String()

			// Map the input path to a snapshot relative path. The mapping is independent of the module
			// that references them so that if multiple modules within the same snapshot export the same
			// header files they end up in the same place in the snapshot and so do not get duplicated.
			targetRelativePath := inputPath
			if isGeneratedHeaderDirectory(path) {
				// Remove everything up to the .intermediates/ from the generated output directory to
				// leave a module relative path.
				base := android.PathForIntermediates(sdkModuleContext, "")
				targetRelativePath = android.Rel(sdkModuleContext, base.String(), inputPath)
			}

			snapshotRelativePath := filepath.Join(targetDir, targetRelativePath)

			// Copy the files/directories when necessary.
			if propertyInfo.copy {
				if propertyInfo.dirs {
					// When copying a directory glob and copy all the headers within it.
					// TODO(jiyong) copy headers having other suffixes
					headers, _ := sdkModuleContext.GlobWithDeps(inputPath+"/**/*.h", nil)
					for _, file := range headers {
						src := android.PathForSource(sdkModuleContext, file)

						// The destination path in the snapshot is constructed from the snapshot relative path
						// of the input directory and the input directory relative path of the header file.
						inputRelativePath := android.Rel(sdkModuleContext, inputPath, file)
						dest := filepath.Join(snapshotRelativePath, inputRelativePath)
						builder.CopyToSnapshot(src, dest)
					}
				} else {
					// Otherwise, just copy the file to its snapshot relative path.
					builder.CopyToSnapshot(path, snapshotRelativePath)
				}
			}

			// Only directories are added to a property.
			if propertyInfo.dirs {
				includeDirs[propertyName] = append(includeDirs[propertyName], snapshotRelativePath)
			}
		}
	}

	// Add the collated include dir properties to the output.
	for _, property := range android.SortedStringKeys(includeDirs) {
		outputProperties.AddProperty(property, includeDirs[property])
	}

	if len(libInfo.StubsVersions) > 0 {
		stubsSet := outputProperties.AddPropertySet("stubs")
		stubsSet.AddProperty("versions", libInfo.StubsVersions)
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

	// archType is not exported as if set (to a non default value) it is always arch specific.
	// This is "" for common properties.
	archType string

	// The list of possibly common exported include dirs.
	//
	// This field is exported as its contents may not be arch specific.
	ExportedIncludeDirs android.Paths `android:"arch_variant"`

	// The list of arch specific exported generated include dirs.
	//
	// This field is exported as its contents may not be arch specific, e.g. protos.
	ExportedGeneratedIncludeDirs android.Paths `android:"arch_variant"`

	// The list of arch specific exported generated header files.
	//
	// This field is exported as its contents may not be arch specific, e.g. protos.
	ExportedGeneratedHeaders android.Paths `android:"arch_variant"`

	// The list of possibly common exported system include dirs.
	//
	// This field is exported as its contents may not be arch specific.
	ExportedSystemIncludeDirs android.Paths `android:"arch_variant"`

	// The list of possibly common exported flags.
	//
	// This field is exported as its contents may not be arch specific.
	ExportedFlags []string `android:"arch_variant"`

	// The set of shared libraries
	//
	// This field is exported as its contents may not be arch specific.
	SharedLibs []string `android:"arch_variant"`

	// The set of system shared libraries. Note nil and [] are semantically
	// distinct - see BaseLinkerProperties.System_shared_libs.
	//
	// This field is exported as its contents may not be arch specific.
	SystemSharedLibs []string `android:"arch_variant"`

	// The specific stubs version for the lib variant, or empty string if stubs
	// are not in use.
	//
	// Marked 'ignored-on-host' as the AllStubsVersions() from which this is
	// initialized is not set on host and the stubs.versions property which this
	// is written to does not vary by arch so cannot be android specific.
	StubsVersions []string `sdk:"ignored-on-host"`

	// Value of SanitizeProperties.Sanitize. Several - but not all - of these
	// affect the expanded variants. All are propagated to avoid entangling the
	// sanitizer logic with the snapshot generation.
	Sanitize SanitizeUserProps `android:"arch_variant"`

	// outputFile is not exported as it is always arch specific.
	outputFile android.Path
}

func (p *nativeLibInfoProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	addOutputFile := true
	ccModule := variant.(*Module)

	if s := ccModule.sanitize; s != nil {
		// We currently do not capture sanitizer flags for libs with sanitizers
		// enabled, because they may vary among variants that cannot be represented
		// in the input blueprint files. In particular, sanitizerDepsMutator enables
		// various sanitizers on dependencies, but in many cases only on static
		// ones, and we cannot specify sanitizer flags at the link type level (i.e.
		// in StaticOrSharedProperties).
		if s.isUnsanitizedVariant() {
			// This still captures explicitly disabled sanitizers, which may be
			// necessary to avoid cyclic dependencies.
			p.Sanitize = s.Properties.Sanitize
		} else {
			// Do not add the output file to the snapshot if we don't represent it
			// properly.
			addOutputFile = false
		}
	}

	exportedInfo := ctx.SdkModuleContext().OtherModuleProvider(variant, FlagExporterInfoProvider).(FlagExporterInfo)

	// Separate out the generated include dirs (which are arch specific) from the
	// include dirs (which may not be).
	exportedIncludeDirs, exportedGeneratedIncludeDirs := android.FilterPathListPredicate(
		exportedInfo.IncludeDirs, isGeneratedHeaderDirectory)

	p.archType = ccModule.Target().Arch.ArchType.String()

	// Make sure that the include directories are unique.
	p.ExportedIncludeDirs = android.FirstUniquePaths(exportedIncludeDirs)
	p.ExportedGeneratedIncludeDirs = android.FirstUniquePaths(exportedGeneratedIncludeDirs)

	// Take a copy before filtering out duplicates to avoid changing the slice owned by the
	// ccModule.
	dirs := append(android.Paths(nil), exportedInfo.SystemIncludeDirs...)
	p.ExportedSystemIncludeDirs = android.FirstUniquePaths(dirs)

	p.ExportedFlags = exportedInfo.Flags
	if ccModule.linker != nil {
		specifiedDeps := specifiedDeps{}
		specifiedDeps = ccModule.linker.linkerSpecifiedDeps(specifiedDeps)

		if lib := ccModule.library; lib != nil {
			if !lib.hasStubsVariants() {
				// Propagate dynamic dependencies for implementation libs, but not stubs.
				p.SharedLibs = specifiedDeps.sharedLibs
			} else {
				// TODO(b/169373910): 1. Only output the specific version (from
				// ccModule.StubsVersion()) if the module is versioned. 2. Ensure that all
				// the versioned stub libs are retained in the prebuilt tree; currently only
				// the stub corresponding to ccModule.StubsVersion() is.
				p.StubsVersions = lib.allStubsVersions()
			}
		}
		p.SystemSharedLibs = specifiedDeps.systemSharedLibs
	}
	p.ExportedGeneratedHeaders = exportedInfo.GeneratedHeaders

	if !p.memberType.noOutputFiles && addOutputFile {
		p.outputFile = getRequiredMemberOutputFile(ctx, ccModule)
	}
}

func getRequiredMemberOutputFile(ctx android.SdkMemberContext, ccModule *Module) android.Path {
	var path android.Path
	outputFile := ccModule.OutputFile()
	if outputFile.Valid() {
		path = outputFile.Path()
	} else {
		ctx.SdkModuleContext().ModuleErrorf("member variant %s does not have a valid output file", ccModule)
	}
	return path
}

func (p *nativeLibInfoProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	addPossiblyArchSpecificProperties(ctx.SdkModuleContext(), ctx.SnapshotBuilder(), p, propertySet)
}
