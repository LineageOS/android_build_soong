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

package java

import (
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// =================================================================================================
// WIP - see http://b/177892522 for details
//
// The build support for boot images is currently being migrated away from singleton to modules so
// the documentation may not be strictly accurate. Rather than update the documentation at every
// step which will create a lot of churn the changes that have been made will be listed here and the
// documentation will be updated once it is closer to the final result.
//
// Changes:
// 1) dex_bootjars is now a singleton module and not a plain singleton.
// 2) Boot images are now represented by the boot_image module type.
// 3) The art boot image is called "art-boot-image", the framework boot image is called
//    "framework-boot-image".
// 4) They are defined in art/build/boot/Android.bp and frameworks/base/boot/Android.bp
//    respectively.
// 5) Each boot_image retrieves the appropriate boot image configuration from the map returned by
//    genBootImageConfigs() using the image_name specified in the boot_image module.
// =================================================================================================

// This comment describes:
//   1. ART boot images in general (their types, structure, file layout, etc.)
//   2. build system support for boot images
//
// 1. ART boot images
// ------------------
//
// A boot image in ART is a set of files that contain AOT-compiled native code and a heap snapshot
// of AOT-initialized classes for the bootclasspath Java libraries. A boot image is compiled from a
// set of DEX jars by the dex2oat compiler. A boot image is used for two purposes: 1) it is
// installed on device and loaded at runtime, and 2) other Java libraries and apps are compiled
// against it (compilation may take place either on host, known as "dexpreopt", or on device, known
// as "dexopt").
//
// A boot image is not a single file, but a collection of interrelated files. Each boot image has a
// number of components that correspond to the Java libraries that constitute it. For each component
// there are multiple files:
//   - *.oat or *.odex file with native code (architecture-specific, one per instruction set)
//   - *.art file with pre-initialized Java classes (architecture-specific, one per instruction set)
//   - *.vdex file with verification metadata for the DEX bytecode (architecture independent)
//
// *.vdex files for the boot images do not contain the DEX bytecode itself, because the
// bootclasspath DEX files are stored on disk in uncompressed and aligned form. Consequently a boot
// image is not self-contained and cannot be used without its DEX files. To simplify the management
// of boot image files, ART uses a certain naming scheme and associates the following metadata with
// each boot image:
//   - A stem, which is a symbolic name that is prepended to boot image file names.
//   - A location (on-device path to the boot image files).
//   - A list of boot image locations (on-device paths to dependency boot images).
//   - A set of DEX locations (on-device paths to the DEX files, one location for one DEX file used
//     to compile the boot image).
//
// There are two kinds of boot images:
//   - primary boot images
//   - boot image extensions
//
// 1.1. Primary boot images
// ------------------------
//
// A primary boot image is compiled for a core subset of bootclasspath Java libraries. It does not
// depend on any other images, and other boot images may depend on it.
//
// For example, assuming that the stem is "boot", the location is /apex/com.android.art/javalib/,
// the set of core bootclasspath libraries is A B C, and the boot image is compiled for ARM targets
// (32 and 64 bits), it will have three components with the following files:
//   - /apex/com.android.art/javalib/{arm,arm64}/boot.{art,oat,vdex}
//   - /apex/com.android.art/javalib/{arm,arm64}/boot-B.{art,oat,vdex}
//   - /apex/com.android.art/javalib/{arm,arm64}/boot-C.{art,oat,vdex}
//
// The files of the first component are special: they do not have the component name appended after
// the stem. This naming convention dates back to the times when the boot image was not split into
// components, and there were just boot.oat and boot.art. The decision to split was motivated by
// licensing reasons for one of the bootclasspath libraries.
//
// As of November 2020 the only primary boot image in Android is the image in the ART APEX
// com.android.art. The primary ART boot image contains the Core libraries that are part of the ART
// module. When the ART module gets updated, the primary boot image will be updated with it, and all
// dependent images will get invalidated (the checksum of the primary image stored in dependent
// images will not match), unless they are updated in sync with the ART module.
//
// 1.2. Boot image extensions
// --------------------------
//
// A boot image extension is compiled for a subset of bootclasspath Java libraries (in particular,
// this subset does not include the Core bootclasspath libraries that go into the primary boot
// image). A boot image extension depends on the primary boot image and optionally some other boot
// image extensions. Other images may depend on it. In other words, boot image extensions can form
// acyclic dependency graphs.
//
// The motivation for boot image extensions comes from the Mainline project. Consider a situation
// when the list of bootclasspath libraries is A B C, and both A and B are parts of the Android
// platform, but C is part of an updatable APEX com.android.C. When the APEX is updated, the Java
// code for C might have changed compared to the code that was used to compile the boot image.
// Consequently, the whole boot image is obsolete and invalidated (even though the code for A and B
// that does not depend on C is up to date). To avoid this, the original monolithic boot image is
// split in two parts: the primary boot image that contains A B, and the boot image extension that
// contains C and depends on the primary boot image (extends it).
//
// For example, assuming that the stem is "boot", the location is /system/framework, the set of
// bootclasspath libraries is D E (where D is part of the platform and is located in
// /system/framework, and E is part of a non-updatable APEX com.android.E and is located in
// /apex/com.android.E/javalib), and the boot image is compiled for ARM targets (32 and 64 bits),
// it will have two components with the following files:
//   - /system/framework/{arm,arm64}/boot-D.{art,oat,vdex}
//   - /system/framework/{arm,arm64}/boot-E.{art,oat,vdex}
//
// As of November 2020 the only boot image extension in Android is the Framework boot image
// extension. It extends the primary ART boot image and contains Framework libraries and other
// bootclasspath libraries from the platform and non-updatable APEXes that are not included in the
// ART image. The Framework boot image extension is updated together with the platform. In the
// future other boot image extensions may be added for some updatable modules.
//
//
// 2. Build system support for boot images
// ---------------------------------------
//
// The primary ART boot image needs to be compiled with one dex2oat invocation that depends on DEX
// jars for the core libraries. Framework boot image extension needs to be compiled with one dex2oat
// invocation that depends on the primary ART boot image and all bootclasspath DEX jars except the
// core libraries as they are already part of the primary ART boot image.
//
// 2.1. Libraries that go in the boot images
// -----------------------------------------
//
// The contents of each boot image are determined by the PRODUCT variables. The primary ART APEX
// boot image contains libraries listed in the ART_APEX_JARS variable in the AOSP makefiles. The
// Framework boot image extension contains libraries specified in the PRODUCT_BOOT_JARS and
// PRODUCT_BOOT_JARS_EXTRA variables. The AOSP makefiles specify some common Framework libraries,
// but more product-specific libraries can be added in the product makefiles.
//
// Each component of the PRODUCT_BOOT_JARS and PRODUCT_BOOT_JARS_EXTRA variables is a
// colon-separated pair <apex>:<library>, where <apex> is the variant name of a non-updatable APEX,
// "platform" if the library is a part of the platform in the system partition, or "system_ext" if
// it's in the system_ext partition.
//
// In these variables APEXes are identified by their "variant names", i.e. the names they get
// mounted as in /apex on device. In Soong modules that is the name set in the "apex_name"
// properties, which default to the "name" values. For example, many APEXes have both
// com.android.xxx and com.google.android.xxx modules in Soong, but take the same place
// /apex/com.android.xxx at runtime. In these cases the variant name is always com.android.xxx,
// regardless which APEX goes into the product. See also android.ApexInfo.ApexVariationName and
// apex.apexBundleProperties.Apex_name.
//
// A related variable PRODUCT_APEX_BOOT_JARS contains bootclasspath libraries that are in APEXes.
// They are not included in the boot image. The only exception here are ART jars and core-icu4j.jar
// that have been historically part of the boot image and are now in apexes; they are in boot images
// and core-icu4j.jar is generally treated as being part of PRODUCT_BOOT_JARS.
//
// One exception to the above rules are "coverage" builds (a special build flavor which requires
// setting environment variable EMMA_INSTRUMENT_FRAMEWORK=true). In coverage builds the Java code in
// boot image libraries is instrumented, which means that the instrumentation library (jacocoagent)
// needs to be added to the list of bootclasspath DEX jars.
//
// In general, there is a requirement that the source code for a boot image library must be
// available at build time (e.g. it cannot be a stub that has a separate implementation library).
//
// 2.2. Static configs
// -------------------
//
// Because boot images are used to dexpreopt other Java modules, the paths to boot image files must
// be known by the time dexpreopt build rules for the dependent modules are generated. Boot image
// configs are constructed very early during the build, before build rule generation. The configs
// provide predefined paths to boot image files (these paths depend only on static build
// configuration, such as PRODUCT variables, and use hard-coded directory names).
//
// 2.3. Singleton
// --------------
//
// Build rules for the boot images are generated with a Soong singleton. Because a singleton has no
// dependencies on other modules, it has to find the modules for the DEX jars using VisitAllModules.
// Soong loops through all modules and compares each module against a list of bootclasspath library
// names. Then it generates build rules that copy DEX jars from their intermediate module-specific
// locations to the hard-coded locations predefined in the boot image configs.
//
// It would be possible to use a module with proper dependencies instead, but that would require
// changes in the way Soong generates variables for Make: a singleton can use one MakeVars() method
// that writes variables to out/soong/make_vars-*.mk, which is included early by the main makefile,
// but module(s) would have to use out/soong/Android-*.mk which has a group of LOCAL_* variables
// for each module, and is included later.
//
// 2.4. Install rules
// ------------------
//
// The primary boot image and the Framework extension are installed in different ways. The primary
// boot image is part of the ART APEX: it is copied into the APEX intermediate files, packaged
// together with other APEX contents, extracted and mounted on device. The Framework boot image
// extension is installed by the rules defined in makefiles (make/core/dex_preopt_libart.mk). Soong
// writes out a few DEXPREOPT_IMAGE_* variables for Make; these variables contain boot image names,
// paths and so on.
//

var artApexNames = []string{
	"com.android.art",
	"com.android.art.debug",
	"com.android.art.testing",
	"com.google.android.art",
	"com.google.android.art.debug",
	"com.google.android.art.testing",
}

var (
	dexpreoptBootJarDepTag          = bootclasspathDependencyTag{name: "dexpreopt-boot-jar"}
	dexBootJarsFragmentsKey         = android.NewOnceKey("dexBootJarsFragments")
	apexContributionsMetadataDepTag = dependencyTag{name: "all_apex_contributions"}
)

func init() {
	RegisterDexpreoptBootJarsComponents(android.InitRegistrationContext)
}

// Target-independent description of a boot image.
//
// WARNING: All fields in this struct should be initialized in the genBootImageConfigs function.
// Failure to do so can lead to data races if there is no synchronization enforced ordering between
// the writer and the reader.
type bootImageConfig struct {
	// If this image is an extension, the image that it extends.
	extends *bootImageConfig

	// Image name (used in directory names and ninja rule names).
	name string

	// If the module with the given name exists, this config is enabled.
	enabledIfExists string

	// Basename of the image: the resulting filenames are <stem>[-<jar>].{art,oat,vdex}.
	stem string

	// Output directory for the image files.
	dir android.OutputPath

	// Output directory for the image files with debug symbols.
	symbolsDir android.OutputPath

	// The relative location where the image files are installed. On host, the location is relative to
	// $ANDROID_PRODUCT_OUT.
	//
	// Only the configs that are built by platform_bootclasspath are installable on device. On device,
	// the location is relative to "/".
	installDir string

	// A list of (location, jar) pairs for the Java modules in this image.
	modules android.ConfiguredJarList

	// File paths to jars.
	dexPaths     android.WritablePaths // for this image
	dexPathsDeps android.WritablePaths // for the dependency images and in this image

	// Map from module name (without prebuilt_ prefix) to the predefined build path.
	dexPathsByModule map[string]android.WritablePath

	// File path to a zip archive with all image files (or nil, if not needed).
	zip android.WritablePath

	// Target-dependent fields.
	variants []*bootImageVariant

	// Path of the preloaded classes file.
	preloadedClassesFile string

	// The "--compiler-filter" argument.
	compilerFilter string

	// The "--single-image" argument.
	singleImage bool

	// Profiles imported from APEXes, in addition to the profile at the default path. Each entry must
	// be the name of an APEX module.
	profileImports []string
}

// Target-dependent description of a boot image.
//
// WARNING: The warning comment on bootImageConfig applies here too.
type bootImageVariant struct {
	*bootImageConfig

	// Target for which the image is generated.
	target android.Target

	// The "locations" of jars.
	dexLocations     []string // for this image
	dexLocationsDeps []string // for the dependency images and in this image

	// Paths to image files.
	imagePathOnHost   android.OutputPath // first image file path on host
	imagePathOnDevice string             // first image file path on device

	// All the files that constitute this image variant, i.e. .art, .oat and .vdex files.
	imagesDeps android.OutputPaths

	// The path to the base image variant's imagePathOnHost field, where base image variant
	// means the image variant that this extends.
	//
	// This is only set for a variant of an image that extends another image.
	baseImages android.OutputPaths

	// The paths to the base image variant's imagesDeps field, where base image variant
	// means the image variant that this extends.
	//
	// This is only set for a variant of an image that extends another image.
	baseImagesDeps android.Paths

	// Rules which should be used in make to install the outputs on host.
	//
	// Deprecated: Not initialized correctly, see struct comment.
	installs android.RuleBuilderInstalls

	// Rules which should be used in make to install the vdex outputs on host.
	//
	// Deprecated: Not initialized correctly, see struct comment.
	vdexInstalls android.RuleBuilderInstalls

	// Rules which should be used in make to install the unstripped outputs on host.
	//
	// Deprecated: Not initialized correctly, see struct comment.
	unstrippedInstalls android.RuleBuilderInstalls

	// Path to the license metadata file for the module that built the image.
	//
	// Deprecated: Not initialized correctly, see struct comment.
	licenseMetadataFile android.OptionalPath
}

// Get target-specific boot image variant for the given boot image config and target.
func (image bootImageConfig) getVariant(target android.Target) *bootImageVariant {
	for _, variant := range image.variants {
		if variant.target.Os == target.Os && variant.target.Arch.ArchType == target.Arch.ArchType {
			return variant
		}
	}
	return nil
}

// Return any (the first) variant which is for the device (as opposed to for the host).
func (image bootImageConfig) getAnyAndroidVariant() *bootImageVariant {
	for _, variant := range image.variants {
		if variant.target.Os == android.Android {
			return variant
		}
	}
	return nil
}

// Return the name of a boot image module given a boot image config and a component (module) index.
// A module name is a combination of the Java library name, and the boot image stem (that is stored
// in the config).
func (image bootImageConfig) moduleName(ctx android.PathContext, idx int) string {
	// The first module of the primary boot image is special: its module name has only the stem, but
	// not the library name. All other module names are of the form <stem>-<library name>
	m := image.modules.Jar(idx)
	name := image.stem
	if idx != 0 || image.extends != nil {
		name += "-" + android.ModuleStem(ctx.Config(), image.modules.Apex(idx), m)
	}
	return name
}

// Return the name of the first boot image module, or stem if the list of modules is empty.
func (image bootImageConfig) firstModuleNameOrStem(ctx android.PathContext) string {
	if image.modules.Len() > 0 {
		return image.moduleName(ctx, 0)
	} else {
		return image.stem
	}
}

// Return filenames for the given boot image component, given the output directory and a list of
// extensions.
func (image bootImageConfig) moduleFiles(ctx android.PathContext, dir android.OutputPath, exts ...string) android.OutputPaths {
	ret := make(android.OutputPaths, 0, image.modules.Len()*len(exts))
	for i := 0; i < image.modules.Len(); i++ {
		name := image.moduleName(ctx, i)
		for _, ext := range exts {
			ret = append(ret, dir.Join(ctx, name+ext))
		}
		if image.singleImage {
			break
		}
	}
	return ret
}

// apexVariants returns a list of all *bootImageVariant that could be included in an apex.
func (image *bootImageConfig) apexVariants() []*bootImageVariant {
	variants := []*bootImageVariant{}
	for _, variant := range image.variants {
		// We also generate boot images for host (for testing), but we don't need those in the apex.
		// TODO(b/177892522) - consider changing this to check Os.OsClass = android.Device
		if variant.target.Os == android.Android {
			variants = append(variants, variant)
		}
	}
	return variants
}

// Return boot image locations (as a list of symbolic paths).
//
// The image "location" is a symbolic path that, with multiarchitecture support, doesn't really
// exist on the device. Typically it is /apex/com.android.art/javalib/boot.art and should be the
// same for all supported architectures on the device. The concrete architecture specific files
// actually end up in architecture-specific sub-directory such as arm, arm64, x86, or x86_64.
//
// For example a physical file /apex/com.android.art/javalib/x86/boot.art has "image location"
// /apex/com.android.art/javalib/boot.art (which is not an actual file).
//
// For a primary boot image the list of locations has a single element.
//
// For a boot image extension the list of locations contains a location for all dependency images
// (including the primary image) and the location of the extension itself. For example, for the
// Framework boot image extension that depends on the primary ART boot image the list contains two
// elements.
//
// The location is passed as an argument to the ART tools like dex2oat instead of the real path.
// ART tools will then reconstruct the architecture-specific real path.
func (image *bootImageVariant) imageLocations() (imageLocationsOnHost []string, imageLocationsOnDevice []string) {
	if image.extends != nil {
		imageLocationsOnHost, imageLocationsOnDevice = image.extends.getVariant(image.target).imageLocations()
	}
	return append(imageLocationsOnHost, dexpreopt.PathToLocation(image.imagePathOnHost, image.target.Arch.ArchType)),
		append(imageLocationsOnDevice, dexpreopt.PathStringToLocation(image.imagePathOnDevice, image.target.Arch.ArchType))
}

func (image *bootImageConfig) isProfileGuided() bool {
	return image.compilerFilter == "speed-profile"
}

func (image *bootImageConfig) isEnabled(ctx android.BaseModuleContext) bool {
	return ctx.OtherModuleExists(image.enabledIfExists)
}

func dexpreoptBootJarsFactory() android.SingletonModule {
	m := &dexpreoptBootJars{}
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibCommon)
	return m
}

func RegisterDexpreoptBootJarsComponents(ctx android.RegistrationContext) {
	ctx.RegisterParallelSingletonModuleType("dex_bootjars", dexpreoptBootJarsFactory)
	ctx.FinalDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("dex_bootjars_deps", DexpreoptBootJarsMutator).Parallel()
	})
}

func SkipDexpreoptBootJars(ctx android.PathContext) bool {
	global := dexpreopt.GetGlobalConfig(ctx)
	return global.DisablePreoptBootImages || !shouldBuildBootImages(ctx.Config(), global)
}

// Singleton module for generating boot image build rules.
type dexpreoptBootJars struct {
	android.SingletonModuleBase

	// Default boot image config (currently always the Framework boot image extension). It should be
	// noted that JIT-Zygote builds use ART APEX image instead of the Framework boot image extension,
	// but the switch is handled not here, but in the makefiles (triggered with
	// DEXPREOPT_USE_ART_IMAGE=true).
	defaultBootImage *bootImageConfig

	// Other boot image configs (currently the list contains only the primary ART APEX image. It
	// used to contain an experimental JIT-Zygote image (now replaced with the ART APEX image). In
	// the future other boot image extensions may be added.
	otherImages []*bootImageConfig

	// Build path to a config file that Soong writes for Make (to be used in makefiles that install
	// the default boot image).
	dexpreoptConfigForMake android.WritablePath
}

func (dbj *dexpreoptBootJars) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Create a dependency on all_apex_contributions to determine the selected mainline module
	ctx.AddDependency(ctx.Module(), apexContributionsMetadataDepTag, "all_apex_contributions")
}

func DexpreoptBootJarsMutator(ctx android.BottomUpMutatorContext) {
	if _, ok := ctx.Module().(*dexpreoptBootJars); !ok {
		return
	}

	if dexpreopt.IsDex2oatNeeded(ctx) {
		// Add a dependency onto the dex2oat tool which is needed for creating the boot image. The
		// path is retrieved from the dependency by GetGlobalSoongConfig(ctx).
		dexpreopt.RegisterToolDeps(ctx)
	}

	imageConfigs := genBootImageConfigs(ctx)
	for _, config := range imageConfigs {
		if !config.isEnabled(ctx) {
			continue
		}
		// For accessing the boot jars.
		addDependenciesOntoBootImageModules(ctx, config.modules, dexpreoptBootJarDepTag)
		// Create a dependency on the apex selected using RELEASE_APEX_CONTRIBUTIONS_*
		// TODO: b/308174306 - Remove the direct depedendency edge to the java_library (source/prebuilt) once all mainline modules
		// have been flagged using RELEASE_APEX_CONTRIBUTIONS_*
		apexes := []string{}
		for i := 0; i < config.modules.Len(); i++ {
			apexes = append(apexes, config.modules.Apex(i))
		}
		addDependenciesOntoSelectedBootImageApexes(ctx, android.FirstUniqueStrings(apexes)...)
	}

	if ctx.OtherModuleExists("platform-bootclasspath") {
		// For accessing all bootclasspath fragments.
		addDependencyOntoApexModulePair(ctx, "platform", "platform-bootclasspath", platformBootclasspathDepTag)
	} else if ctx.OtherModuleExists("art-bootclasspath-fragment") {
		// For accessing the ART bootclasspath fragment on a thin manifest (e.g., master-art) where
		// platform-bootclasspath doesn't exist.
		addDependencyOntoApexModulePair(ctx, "com.android.art", "art-bootclasspath-fragment", bootclasspathFragmentDepTag)
	}
}

// Create a dependency from dex_bootjars to the specific apexes selected using all_apex_contributions
// This dependency will be used to get the path to the deapexed dex boot jars and profile (via a provider)
func addDependenciesOntoSelectedBootImageApexes(ctx android.BottomUpMutatorContext, apexes ...string) {
	psi := android.PrebuiltSelectionInfoMap{}
	ctx.VisitDirectDepsWithTag(apexContributionsMetadataDepTag, func(am android.Module) {
		if info, exists := android.OtherModuleProvider(ctx, am, android.PrebuiltSelectionInfoProvider); exists {
			psi = info
		}
	})
	for _, apex := range apexes {
		for _, selected := range psi.GetSelectedModulesForApiDomain(apex) {
			// We need to add a dep on only the apex listed in `contents` of the selected apex_contributions module
			// This is not available in a structured format in `apex_contributions`, so this hack adds a dep on all `contents`
			// (some modules like art.module.public.api do not have an apex variation since it is a pure stub module that does not get installed)
			apexVariationOfSelected := append(ctx.Target().Variations(), blueprint.Variation{Mutator: "apex", Variation: apex})
			if ctx.OtherModuleDependencyVariantExists(apexVariationOfSelected, selected) {
				ctx.AddFarVariationDependencies(apexVariationOfSelected, dexpreoptBootJarDepTag, selected)
			} else if ctx.OtherModuleDependencyVariantExists(apexVariationOfSelected, android.RemoveOptionalPrebuiltPrefix(selected)) {
				// The prebuilt might have been renamed by prebuilt_rename mutator if the source module does not exist.
				// Remove the prebuilt_ prefix.
				ctx.AddFarVariationDependencies(apexVariationOfSelected, dexpreoptBootJarDepTag, android.RemoveOptionalPrebuiltPrefix(selected))
			}
		}
	}
}

func gatherBootclasspathFragments(ctx android.ModuleContext) map[string]android.Module {
	return ctx.Config().Once(dexBootJarsFragmentsKey, func() interface{} {
		fragments := make(map[string]android.Module)
		ctx.WalkDeps(func(child, parent android.Module) bool {
			if !isActiveModule(ctx, child) {
				return false
			}
			tag := ctx.OtherModuleDependencyTag(child)
			if tag == platformBootclasspathDepTag {
				return true
			}
			if tag == bootclasspathFragmentDepTag {
				apexInfo, _ := android.OtherModuleProvider(ctx, child, android.ApexInfoProvider)
				for _, apex := range apexInfo.InApexVariants {
					fragments[apex] = child
				}
				return false
			}
			return false
		})
		return fragments
	}).(map[string]android.Module)
}

func getBootclasspathFragmentByApex(ctx android.ModuleContext, apexName string) android.Module {
	return gatherBootclasspathFragments(ctx)[apexName]
}

// GenerateAndroidBuildActions generates the build rules for boot images.
func (d *dexpreoptBootJars) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	imageConfigs := genBootImageConfigs(ctx)
	d.defaultBootImage = defaultBootImageConfig(ctx)
	d.otherImages = make([]*bootImageConfig, 0, len(imageConfigs)-1)
	var profileInstalls android.RuleBuilderInstalls
	for _, name := range getImageNames() {
		config := imageConfigs[name]
		if config != d.defaultBootImage {
			d.otherImages = append(d.otherImages, config)
		}
		if !config.isEnabled(ctx) {
			continue
		}
		installs := generateBootImage(ctx, config)
		profileInstalls = append(profileInstalls, installs...)
		if config == d.defaultBootImage {
			_, installs := bootFrameworkProfileRule(ctx, config)
			profileInstalls = append(profileInstalls, installs...)
		}
	}
	if len(profileInstalls) > 0 {
		android.SetProvider(ctx, profileInstallInfoProvider, profileInstallInfo{
			profileInstalls:            profileInstalls,
			profileLicenseMetadataFile: android.OptionalPathForPath(ctx.LicenseMetadataFile()),
		})
		for _, install := range profileInstalls {
			packageFile(ctx, install)
		}
	}
}

// GenerateSingletonBuildActions generates build rules for the dexpreopt config for Make.
func (d *dexpreoptBootJars) GenerateSingletonBuildActions(ctx android.SingletonContext) {
	d.dexpreoptConfigForMake =
		android.PathForOutput(ctx, dexpreopt.GetDexpreoptDirName(ctx), "dexpreopt.config")
	writeGlobalConfigForMake(ctx, d.dexpreoptConfigForMake)
}

// shouldBuildBootImages determines whether boot images should be built.
func shouldBuildBootImages(config android.Config, global *dexpreopt.GlobalConfig) bool {
	// Skip recompiling the boot image for the second sanitization phase. We'll get separate paths
	// and invalidate first-stage artifacts which are crucial to SANITIZE_LITE builds.
	// Note: this is technically incorrect. Compiled code contains stack checks which may depend
	//       on ASAN settings.
	if len(config.SanitizeDevice()) == 1 && config.SanitizeDevice()[0] == "address" && global.SanitizeLite {
		return false
	}
	return true
}

func generateBootImage(ctx android.ModuleContext, imageConfig *bootImageConfig) android.RuleBuilderInstalls {
	apexJarModulePairs := getModulesForImage(ctx, imageConfig)

	// Copy module dex jars to their predefined locations.
	bootDexJarsByModule := extractEncodedDexJarsFromModulesOrBootclasspathFragments(ctx, apexJarModulePairs)
	copyBootJarsToPredefinedLocations(ctx, bootDexJarsByModule, imageConfig.dexPathsByModule)

	// Build a profile for the image config from the profile at the default path. The profile will
	// then be used along with profiles imported from APEXes to build the boot image.
	profile, profileInstalls := bootImageProfileRule(ctx, imageConfig)

	// If dexpreopt of boot image jars should be skipped, stop after generating a profile.
	global := dexpreopt.GetGlobalConfig(ctx)
	if SkipDexpreoptBootJars(ctx) || (global.OnlyPreoptArtBootImage && imageConfig.name != "art") {
		return profileInstalls
	}

	// Build boot image files for the android variants.
	androidBootImageFiles := buildBootImageVariantsForAndroidOs(ctx, imageConfig, profile)

	// Zip the android variant boot image files up.
	buildBootImageZipInPredefinedLocation(ctx, imageConfig, androidBootImageFiles.byArch)

	// Build boot image files for the host variants. There are use directly by ART host side tests.
	buildBootImageVariantsForBuildOs(ctx, imageConfig, profile)

	// Create a `dump-oat-<image-name>` rule that runs `oatdump` for debugging purposes.
	dumpOatRules(ctx, imageConfig)

	return profileInstalls
}

type apexJarModulePair struct {
	apex      string
	jarModule android.Module
}

func getModulesForImage(ctx android.ModuleContext, imageConfig *bootImageConfig) []apexJarModulePair {
	modules := make([]apexJarModulePair, 0, imageConfig.modules.Len())
	for i := 0; i < imageConfig.modules.Len(); i++ {
		found := false
		for _, module := range gatherApexModulePairDepsWithTag(ctx, dexpreoptBootJarDepTag) {
			name := android.RemoveOptionalPrebuiltPrefix(module.Name())
			if name == imageConfig.modules.Jar(i) {
				modules = append(modules, apexJarModulePair{
					apex:      imageConfig.modules.Apex(i),
					jarModule: module,
				})
				found = true
				break
			}
		}
		if !found && !ctx.Config().AllowMissingDependencies() {
			ctx.ModuleErrorf(
				"Boot image '%s' module '%s' not added as a dependency of dex_bootjars",
				imageConfig.name,
				imageConfig.modules.Jar(i))
			return []apexJarModulePair{}
		}
	}
	return modules
}

// extractEncodedDexJarsFromModulesOrBootclasspathFragments gets the hidden API encoded dex jars for
// the given modules.
func extractEncodedDexJarsFromModulesOrBootclasspathFragments(ctx android.ModuleContext, apexJarModulePairs []apexJarModulePair) bootDexJarByModule {
	apexNameToApexExportInfoMap := getApexNameToApexExportsInfoMap(ctx)
	encodedDexJarsByModuleName := bootDexJarByModule{}
	for _, pair := range apexJarModulePairs {
		dexJarPath := getDexJarForApex(ctx, pair, apexNameToApexExportInfoMap)
		encodedDexJarsByModuleName.addPath(pair.jarModule, dexJarPath)
	}
	return encodedDexJarsByModuleName
}

type apexNameToApexExportsInfoMap map[string]android.ApexExportsInfo

// javaLibraryPathOnHost returns the path to the java library which is exported by the apex for hiddenapi and dexpreopt and a boolean indicating whether the java library exists
// For prebuilt apexes, this is created by deapexing the prebuilt apex
func (m *apexNameToApexExportsInfoMap) javaLibraryDexPathOnHost(ctx android.ModuleContext, apex string, javalib string) (android.Path, bool) {
	if info, exists := (*m)[apex]; exists {
		if dex, exists := info.LibraryNameToDexJarPathOnHost[javalib]; exists {
			return dex, true
		} else {
			ctx.ModuleErrorf("Apex %s does not provide a dex boot jar for library %s\n", apex, javalib)
		}
	}
	// An apex entry could not be found. Return false.
	// TODO: b/308174306 - When all the mainline modules have been flagged, make this a hard error
	return nil, false
}

// Returns the stem of an artifact inside a prebuilt apex
func ModuleStemForDeapexing(m android.Module) string {
	bmn, _ := m.(interface{ BaseModuleName() string })
	return bmn.BaseModuleName()
}

// Returns the java libraries exported by the apex for hiddenapi and dexpreopt
// This information can come from two mechanisms
// 1. New: Direct deps to _selected_ apexes. The apexes return a ApexExportsInfo
// 2. Legacy: An edge to java_library or java_import (java_sdk_library) module. For prebuilt apexes, this serves as a hook and is populated by deapexers of prebuilt apxes
// TODO: b/308174306 - Once all mainline modules have been flagged, drop (2)
func getDexJarForApex(ctx android.ModuleContext, pair apexJarModulePair, apexNameToApexExportsInfoMap apexNameToApexExportsInfoMap) android.Path {
	if dex, found := apexNameToApexExportsInfoMap.javaLibraryDexPathOnHost(ctx, pair.apex, ModuleStemForDeapexing(pair.jarModule)); found {
		return dex
	}
	// TODO: b/308174306 - Remove the legacy mechanism
	if android.IsConfiguredJarForPlatform(pair.apex) || android.IsModulePrebuilt(pair.jarModule) {
		// This gives us the dex jar with the hidden API flags encoded from the monolithic hidden API
		// files or the dex jar extracted from a prebuilt APEX. We can't use this for a boot jar for
		// a source APEX because there is no guarantee that it is the same as the jar packed into the
		// APEX. In practice, they are the same when we are building from a full source tree, but they
		// are different when we are building from a thin manifest (e.g., master-art), where there is
		// no monolithic hidden API files at all.
		return retrieveEncodedBootDexJarFromModule(ctx, pair.jarModule)
	} else {
		// Use exactly the same jar that is packed into the APEX.
		fragment := getBootclasspathFragmentByApex(ctx, pair.apex)
		if fragment == nil {
			ctx.ModuleErrorf("Boot jar '%[1]s' is from APEX '%[2]s', but a bootclasspath_fragment for "+
				"APEX '%[2]s' doesn't exist or is not added as a dependency of dex_bootjars",
				pair.jarModule.Name(),
				pair.apex)
		}
		bootclasspathFragmentInfo, _ := android.OtherModuleProvider(ctx, fragment, BootclasspathFragmentApexContentInfoProvider)
		jar, err := bootclasspathFragmentInfo.DexBootJarPathForContentModule(pair.jarModule)
		if err != nil {
			ctx.ModuleErrorf("%s", err)
		}
		return jar
	}
	return nil
}

// copyBootJarsToPredefinedLocations generates commands that will copy boot jars to predefined
// paths in the global config.
func copyBootJarsToPredefinedLocations(ctx android.ModuleContext, srcBootDexJarsByModule bootDexJarByModule, dstBootJarsByModule map[string]android.WritablePath) {
	// Create the super set of module names.
	names := []string{}
	names = append(names, android.SortedKeys(srcBootDexJarsByModule)...)
	names = append(names, android.SortedKeys(dstBootJarsByModule)...)
	names = android.SortedUniqueStrings(names)
	for _, name := range names {
		src := srcBootDexJarsByModule[name]
		dst := dstBootJarsByModule[name]

		if src == nil {
			// A dex boot jar should be provided by the source java module. It needs to be installable or
			// have compile_dex=true - cf. assignments to java.Module.dexJarFile.
			//
			// However, the source java module may be either replaced or overridden (using prefer:true) by
			// a prebuilt java module with the same name. In that case the dex boot jar needs to be
			// provided by the corresponding prebuilt APEX module. That APEX is the one that refers
			// through a exported_(boot|systemserver)classpath_fragments property to a
			// prebuilt_(boot|systemserver)classpath_fragment module, which in turn lists the prebuilt
			// java module in the contents property. If that chain is broken then this dependency will
			// fail.
			if !ctx.Config().AllowMissingDependencies() {
				ctx.ModuleErrorf("module %s does not provide a dex boot jar (see comment next to this message in Soong for details)", name)
			} else {
				ctx.AddMissingDependencies([]string{name})
			}
		} else if dst == nil {
			ctx.ModuleErrorf("module %s is not part of the boot configuration", name)
		} else {
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.Cp,
				Input:  src,
				Output: dst,
			})
		}
	}
}

// buildBootImageVariantsForAndroidOs generates rules to build the boot image variants for the
// android.Android OsType and returns a map from the architectures to the paths of the generated
// boot image files.
//
// The paths are returned because they are needed elsewhere in Soong, e.g. for populating an APEX.
func buildBootImageVariantsForAndroidOs(ctx android.ModuleContext, image *bootImageConfig, profile android.WritablePath) bootImageOutputs {
	return buildBootImageForOsType(ctx, image, profile, android.Android)
}

// buildBootImageVariantsForBuildOs generates rules to build the boot image variants for the
// config.BuildOS OsType, i.e. the type of OS on which the build is being running.
//
// The files need to be generated into their predefined location because they are used from there
// both within Soong and outside, e.g. for ART based host side testing and also for use by some
// cloud based tools. However, they are not needed by callers of this function and so the paths do
// not need to be returned from this func, unlike the buildBootImageVariantsForAndroidOs func.
func buildBootImageVariantsForBuildOs(ctx android.ModuleContext, image *bootImageConfig, profile android.WritablePath) {
	buildBootImageForOsType(ctx, image, profile, ctx.Config().BuildOS)
}

// bootImageFilesByArch is a map from android.ArchType to the paths to the boot image files.
//
// The paths include the .art, .oat and .vdex files, one for each of the modules from which the boot
// image is created.
type bootImageFilesByArch map[android.ArchType]android.Paths

// bootImageOutputs encapsulates information about boot images that were created/obtained by
// commonBootclasspathFragment.produceBootImageFiles.
type bootImageOutputs struct {
	// Map from arch to the paths to the boot image files created/obtained for that arch.
	byArch bootImageFilesByArch

	variants []bootImageVariantOutputs

	// The path to the profile file created/obtained for the boot image.
	profile android.WritablePath
}

// buildBootImageForOsType takes a bootImageConfig, a profile file and an android.OsType
// boot image files are required for and it creates rules to build the boot image
// files for all the required architectures for them.
//
// It returns a map from android.ArchType to the predefined paths of the boot image files.
func buildBootImageForOsType(ctx android.ModuleContext, image *bootImageConfig, profile android.WritablePath, requiredOsType android.OsType) bootImageOutputs {
	filesByArch := bootImageFilesByArch{}
	imageOutputs := bootImageOutputs{
		byArch:  filesByArch,
		profile: profile,
	}
	for _, variant := range image.variants {
		if variant.target.Os == requiredOsType {
			variantOutputs := buildBootImageVariant(ctx, variant, profile)
			imageOutputs.variants = append(imageOutputs.variants, variantOutputs)
			filesByArch[variant.target.Arch.ArchType] = variant.imagesDeps.Paths()
		}
	}

	return imageOutputs
}

// buildBootImageZipInPredefinedLocation generates a zip file containing all the boot image files.
//
// The supplied filesByArch is nil when the boot image files have not been generated. Otherwise, it
// is a map from android.ArchType to the predefined locations.
func buildBootImageZipInPredefinedLocation(ctx android.ModuleContext, image *bootImageConfig, filesByArch bootImageFilesByArch) {
	if filesByArch == nil {
		return
	}

	// Compute the list of files from all the architectures.
	zipFiles := android.Paths{}
	for _, archType := range android.ArchTypeList() {
		zipFiles = append(zipFiles, filesByArch[archType]...)
	}

	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().
		BuiltTool("soong_zip").
		FlagWithOutput("-o ", image.zip).
		FlagWithArg("-C ", image.dir.Join(ctx, android.Android.String()).String()).
		FlagWithInputList("-f ", zipFiles, " -f ")

	rule.Build("zip_"+image.name, "zip "+image.name+" image")
}

type bootImageVariantOutputs struct {
	config *bootImageVariant
}

// Returns the profile file for an apex
// This information can come from two mechanisms
// 1. New: Direct deps to _selected_ apexes. The apexes return a BootclasspathFragmentApexContentInfo
// 2. Legacy: An edge to bootclasspath_fragment module. For prebuilt apexes, this serves as a hook and is populated by deapexers of prebuilt apxes
// TODO: b/308174306 - Once all mainline modules have been flagged, drop (2)
func getProfilePathForApex(ctx android.ModuleContext, apexName string, apexNameToBcpInfoMap map[string]android.ApexExportsInfo) android.Path {
	if info, exists := apexNameToBcpInfoMap[apexName]; exists {
		return info.ProfilePathOnHost
	}
	// TODO: b/308174306 - Remove the legacy mechanism
	fragment := getBootclasspathFragmentByApex(ctx, apexName)
	if fragment == nil {
		ctx.ModuleErrorf("Boot image config imports profile from '%[2]s', but a "+
			"bootclasspath_fragment for APEX '%[2]s' doesn't exist or is not added as a "+
			"dependency of dex_bootjars",
			apexName)
		return nil
	}
	return fragment.(commonBootclasspathFragment).getProfilePath()
}

func getApexNameToApexExportsInfoMap(ctx android.ModuleContext) apexNameToApexExportsInfoMap {
	apexNameToApexExportsInfoMap := apexNameToApexExportsInfoMap{}
	ctx.VisitDirectDepsWithTag(dexpreoptBootJarDepTag, func(am android.Module) {
		if info, exists := android.OtherModuleProvider(ctx, am, android.ApexExportsInfoProvider); exists {
			apexNameToApexExportsInfoMap[info.ApexName] = info
		}
	})
	return apexNameToApexExportsInfoMap
}

func packageFileForTargetImage(ctx android.ModuleContext, image *bootImageVariant) {
	if image.target.Os != ctx.Os() {
		// This is not for the target device.
		return
	}

	for _, install := range image.installs {
		packageFile(ctx, install)
	}

	for _, install := range image.vdexInstalls {
		if image.target.Arch.ArchType.Name != ctx.DeviceConfig().DeviceArch() {
			// Note that the vdex files are identical between architectures. If the target image is
			// not for the primary architecture create symlinks to share the vdex of the primary
			// architecture with the other architectures.
			//
			// Assuming that the install path has the architecture name with it, replace the
			// architecture name with the primary architecture name to find the source vdex file.
			installPath, relDir, name := getModuleInstallPathInfo(ctx, install.To)
			if name != "" {
				srcRelDir := strings.Replace(relDir, image.target.Arch.ArchType.Name, ctx.DeviceConfig().DeviceArch(), 1)
				ctx.InstallSymlink(installPath.Join(ctx, relDir), name, installPath.Join(ctx, srcRelDir, name))
			}
		} else {
			packageFile(ctx, install)
		}
	}
}

// Generate boot image build rules for a specific target.
func buildBootImageVariant(ctx android.ModuleContext, image *bootImageVariant, profile android.Path) bootImageVariantOutputs {

	globalSoong := dexpreopt.GetGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)

	arch := image.target.Arch.ArchType
	os := image.target.Os.String() // We need to distinguish host-x86 and device-x86.
	symbolsDir := image.symbolsDir.Join(ctx, os, image.installDir, arch.String())
	symbolsFile := symbolsDir.Join(ctx, image.stem+".oat")
	outputDir := image.dir.Join(ctx, os, image.installDir, arch.String())
	outputPath := outputDir.Join(ctx, image.stem+".oat")
	oatLocation := dexpreopt.PathToLocation(outputPath, arch)
	imagePath := outputPath.ReplaceExtension(ctx, "art")

	rule := android.NewRuleBuilder(pctx, ctx)

	rule.Command().Text("mkdir").Flag("-p").Flag(symbolsDir.String())
	rule.Command().Text("rm").Flag("-f").
		Flag(symbolsDir.Join(ctx, "*.art").String()).
		Flag(symbolsDir.Join(ctx, "*.oat").String()).
		Flag(symbolsDir.Join(ctx, "*.vdex").String()).
		Flag(symbolsDir.Join(ctx, "*.invocation").String())
	rule.Command().Text("rm").Flag("-f").
		Flag(outputDir.Join(ctx, "*.art").String()).
		Flag(outputDir.Join(ctx, "*.oat").String()).
		Flag(outputDir.Join(ctx, "*.vdex").String()).
		Flag(outputDir.Join(ctx, "*.invocation").String())

	cmd := rule.Command()

	extraFlags := ctx.Config().Getenv("ART_BOOT_IMAGE_EXTRA_ARGS")
	if extraFlags == "" {
		// Use ANDROID_LOG_TAGS to suppress most logging by default...
		cmd.Text(`ANDROID_LOG_TAGS="*:e"`)
	} else {
		// ...unless the boot image is generated specifically for testing, then allow all logging.
		cmd.Text(`ANDROID_LOG_TAGS="*:v"`)
	}

	invocationPath := outputPath.ReplaceExtension(ctx, "invocation")

	apexNameToApexExportsInfoMap := getApexNameToApexExportsInfoMap(ctx)

	cmd.Tool(globalSoong.Dex2oat).
		Flag("--avoid-storing-invocation").
		FlagWithOutput("--write-invocation-to=", invocationPath).ImplicitOutput(invocationPath).
		Flag("--runtime-arg").FlagWithArg("-Xms", global.Dex2oatImageXms).
		Flag("--runtime-arg").FlagWithArg("-Xmx", global.Dex2oatImageXmx)

	if image.isProfileGuided() && !global.DisableGenerateProfile {
		if profile != nil {
			cmd.FlagWithInput("--profile-file=", profile)
		}

		for _, apex := range image.profileImports {
			importedProfile := getProfilePathForApex(ctx, apex, apexNameToApexExportsInfoMap)
			if importedProfile == nil {
				ctx.ModuleErrorf("Boot image config '%[1]s' imports profile from '%[2]s', but '%[2]s' "+
					"doesn't provide a profile",
					image.name,
					apex)
				return bootImageVariantOutputs{}
			}
			cmd.FlagWithInput("--profile-file=", importedProfile)
		}
	}

	dirtyImageFile := "frameworks/base/config/dirty-image-objects"
	dirtyImagePath := android.ExistentPathForSource(ctx, dirtyImageFile)
	if dirtyImagePath.Valid() {
		cmd.FlagWithInput("--dirty-image-objects=", dirtyImagePath.Path())
	}

	if image.extends != nil {
		// It is a boot image extension, so it needs the boot images that it depends on.
		baseImageLocations := make([]string, 0, len(image.baseImages))
		for _, image := range image.baseImages {
			baseImageLocations = append(baseImageLocations, dexpreopt.PathToLocation(image, arch))
		}
		cmd.
			Flag("--runtime-arg").FlagWithInputList("-Xbootclasspath:", image.dexPathsDeps.Paths(), ":").
			Flag("--runtime-arg").FlagWithList("-Xbootclasspath-locations:", image.dexLocationsDeps, ":").
			// Add the path to the first file in the boot image with the arch specific directory removed,
			// dex2oat will reconstruct the path to the actual file when it needs it. As the actual path
			// to the file cannot be passed to the command make sure to add the actual path as an Implicit
			// dependency to ensure that it is built before the command runs.
			FlagWithList("--boot-image=", baseImageLocations, ":").Implicits(image.baseImages.Paths()).
			// Similarly, the dex2oat tool will automatically find the paths to other files in the base
			// boot image so make sure to add them as implicit dependencies to ensure that they are built
			// before this command is run.
			Implicits(image.baseImagesDeps)
	} else {
		// It is a primary image, so it needs a base address.
		cmd.FlagWithArg("--base=", ctx.Config().LibartImgDeviceBaseAddress())
	}

	if len(image.preloadedClassesFile) > 0 {
		// We always expect a preloaded classes file to be available. However, if we cannot find it, it's
		// OK to not pass the flag to dex2oat.
		preloadedClassesPath := android.ExistentPathForSource(ctx, image.preloadedClassesFile)
		if preloadedClassesPath.Valid() {
			cmd.FlagWithInput("--preloaded-classes=", preloadedClassesPath.Path())
		}
	}

	cmd.
		FlagForEachInput("--dex-file=", image.dexPaths.Paths()).
		FlagForEachArg("--dex-location=", image.dexLocations).
		Flag("--generate-debug-info").
		Flag("--generate-build-id").
		Flag("--image-format=lz4hc").
		FlagWithArg("--oat-symbols=", symbolsFile.String()).
		FlagWithArg("--oat-file=", outputPath.String()).
		FlagWithArg("--oat-location=", oatLocation).
		FlagWithArg("--image=", imagePath.String()).
		FlagWithArg("--instruction-set=", arch.String()).
		FlagWithArg("--android-root=", global.EmptyDirectory).
		FlagWithArg("--no-inline-from=", "core-oj.jar").
		Flag("--force-determinism").
		Flag("--abort-on-hard-verifier-error")

	// We don't strip on host to make perf tools work.
	if image.target.Os == android.Android {
		cmd.Flag("--strip")
	}

	// If the image is profile-guided but the profile is disabled, we omit "--compiler-filter" to
	// leave the decision to dex2oat to pick the compiler filter.
	if !(image.isProfileGuided() && global.DisableGenerateProfile) {
		cmd.FlagWithArg("--compiler-filter=", image.compilerFilter)
	}

	if image.singleImage {
		cmd.Flag("--single-image")
	}

	// Use the default variant/features for host builds.
	// The map below contains only device CPU info (which might be x86 on some devices).
	if image.target.Os == android.Android {
		cmd.FlagWithArg("--instruction-set-variant=", global.CpuVariant[arch])
		cmd.FlagWithArg("--instruction-set-features=", global.InstructionSetFeatures[arch])
	}

	if image.target.Os == android.Android {
		cmd.Text("$(cat").Input(globalSoong.UffdGcFlag).Text(")")
	}

	if global.BootFlags != "" {
		cmd.Flag(global.BootFlags)
	}

	if extraFlags != "" {
		cmd.Flag(extraFlags)
	}

	cmd.Textf(`|| ( echo %s ; false )`, proptools.ShellEscape(failureMessage))

	installDir := filepath.Dir(image.imagePathOnDevice)

	var vdexInstalls android.RuleBuilderInstalls
	var unstrippedInstalls android.RuleBuilderInstalls

	for _, artOrOat := range image.moduleFiles(ctx, outputDir, ".art", ".oat") {
		cmd.ImplicitOutput(artOrOat)

		// Install the .oat and .art files
		rule.Install(artOrOat, filepath.Join(installDir, artOrOat.Base()))
	}

	for _, vdex := range image.moduleFiles(ctx, outputDir, ".vdex") {
		cmd.ImplicitOutput(vdex)

		// Note that the vdex files are identical between architectures.
		// Make rules will create symlinks to share them between architectures.
		vdexInstalls = append(vdexInstalls,
			android.RuleBuilderInstall{vdex, filepath.Join(installDir, vdex.Base())})
	}

	for _, unstrippedOat := range image.moduleFiles(ctx, symbolsDir, ".oat") {
		cmd.ImplicitOutput(unstrippedOat)

		// Install the unstripped oat files.  The Make rules will put these in $(TARGET_OUT_UNSTRIPPED)
		unstrippedInstalls = append(unstrippedInstalls,
			android.RuleBuilderInstall{unstrippedOat, filepath.Join(installDir, unstrippedOat.Base())})
	}

	rule.Build(image.name+"JarsDexpreopt_"+image.target.String(), "dexpreopt "+image.name+" jars "+arch.String())

	// save output and installed files for makevars
	// TODO - these are always the same and so should be initialized in genBootImageConfigs
	image.installs = rule.Installs()
	image.vdexInstalls = vdexInstalls
	image.unstrippedInstalls = unstrippedInstalls
	packageFileForTargetImage(ctx, image)

	// Only set the licenseMetadataFile from the active module.
	if isActiveModule(ctx, ctx.Module()) {
		image.licenseMetadataFile = android.OptionalPathForPath(ctx.LicenseMetadataFile())
	}

	return bootImageVariantOutputs{
		image,
	}
}

const failureMessage = `ERROR: Dex2oat failed to compile a boot image.
It is likely that the boot classpath is inconsistent.
Rebuild with ART_BOOT_IMAGE_EXTRA_ARGS="--runtime-arg -verbose:verifier" to see verification errors.`

func bootImageProfileRuleCommon(ctx android.ModuleContext, name string, dexFiles android.Paths, dexLocations []string) android.WritablePath {
	globalSoong := dexpreopt.GetGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)

	if global.DisableGenerateProfile {
		return nil
	}

	defaultProfile := "frameworks/base/config/boot-image-profile.txt"
	extraProfile := "frameworks/base/config/boot-image-profile-extra.txt"

	rule := android.NewRuleBuilder(pctx, ctx)

	var profiles android.Paths
	if len(global.BootImageProfiles) > 0 {
		profiles = append(profiles, global.BootImageProfiles...)
	} else if path := android.ExistentPathForSource(ctx, defaultProfile); path.Valid() {
		profiles = append(profiles, path.Path())
	} else {
		// No profile (not even a default one, which is the case on some branches
		// like master-art-host that don't have frameworks/base).
		// Return nil and continue without profile.
		return nil
	}
	if path := android.ExistentPathForSource(ctx, extraProfile); path.Valid() {
		profiles = append(profiles, path.Path())
	}
	bootImageProfile := android.PathForModuleOut(ctx, name, "boot-image-profile.txt")
	rule.Command().Text("cat").Inputs(profiles).Text(">").Output(bootImageProfile)

	profile := android.PathForModuleOut(ctx, name, "boot.prof")

	rule.Command().
		Text(`ANDROID_LOG_TAGS="*:e"`).
		Tool(globalSoong.Profman).
		Flag("--output-profile-type=boot").
		FlagWithInput("--create-profile-from=", bootImageProfile).
		FlagForEachInput("--apk=", dexFiles).
		FlagForEachArg("--dex-location=", dexLocations).
		FlagWithOutput("--reference-profile-file=", profile)

	rule.Build("bootJarsProfile_"+name, "profile boot jars "+name)

	return profile
}

type profileInstallInfo struct {
	// Rules which should be used in make to install the outputs.
	profileInstalls android.RuleBuilderInstalls

	// Path to the license metadata file for the module that built the profile.
	profileLicenseMetadataFile android.OptionalPath
}

var profileInstallInfoProvider = blueprint.NewProvider[profileInstallInfo]()

func bootImageProfileRule(ctx android.ModuleContext, image *bootImageConfig) (android.WritablePath, android.RuleBuilderInstalls) {
	if !image.isProfileGuided() {
		return nil, nil
	}

	profile := bootImageProfileRuleCommon(ctx, image.name, image.dexPathsDeps.Paths(), image.getAnyAndroidVariant().dexLocationsDeps)

	if image == defaultBootImageConfig(ctx) {
		rule := android.NewRuleBuilder(pctx, ctx)
		rule.Install(profile, "/system/etc/boot-image.prof")
		return profile, rule.Installs()
	}
	return profile, nil
}

// bootFrameworkProfileRule generates the rule to create the boot framework profile and
// returns a path to the generated file.
func bootFrameworkProfileRule(ctx android.ModuleContext, image *bootImageConfig) (android.WritablePath, android.RuleBuilderInstalls) {
	globalSoong := dexpreopt.GetGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)

	if global.DisableGenerateProfile || ctx.Config().UnbundledBuild() {
		return nil, nil
	}

	defaultProfile := "frameworks/base/config/boot-profile.txt"
	bootFrameworkProfile := android.PathForSource(ctx, defaultProfile)

	profile := image.dir.Join(ctx, "boot.bprof")

	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().
		Text(`ANDROID_LOG_TAGS="*:e"`).
		Tool(globalSoong.Profman).
		Flag("--output-profile-type=bprof").
		FlagWithInput("--create-profile-from=", bootFrameworkProfile).
		FlagForEachInput("--apk=", image.dexPathsDeps.Paths()).
		FlagForEachArg("--dex-location=", image.getAnyAndroidVariant().dexLocationsDeps).
		FlagWithOutput("--reference-profile-file=", profile)

	rule.Install(profile, "/system/etc/boot-image.bprof")
	rule.Build("bootFrameworkProfile", "profile boot framework jars")
	return profile, rule.Installs()
}

func dumpOatRules(ctx android.ModuleContext, image *bootImageConfig) {
	var allPhonies android.Paths
	name := image.name
	globalSoong := dexpreopt.GetGlobalSoongConfig(ctx)
	for _, image := range image.variants {
		arch := image.target.Arch.ArchType
		suffix := arch.String()
		// Host and target might both use x86 arch. We need to ensure the names are unique.
		if image.target.Os.Class == android.Host {
			suffix = "host-" + suffix
		}
		// Create a rule to call oatdump.
		output := android.PathForOutput(ctx, name+"."+suffix+".oatdump.txt")
		rule := android.NewRuleBuilder(pctx, ctx)
		imageLocationsOnHost, _ := image.imageLocations()

		cmd := rule.Command().
			BuiltTool("oatdump").
			FlagWithInputList("--runtime-arg -Xbootclasspath:", image.dexPathsDeps.Paths(), ":").
			FlagWithList("--runtime-arg -Xbootclasspath-locations:", image.dexLocationsDeps, ":").
			FlagWithArg("--image=", strings.Join(imageLocationsOnHost, ":")).Implicits(image.imagesDeps.Paths()).
			FlagWithOutput("--output=", output).
			FlagWithArg("--instruction-set=", arch.String())
		if image.target.Os == android.Android {
			cmd.Text("$(cat").Input(globalSoong.UffdGcFlag).Text(")")
		}
		rule.Build("dump-oat-"+name+"-"+suffix, "dump oat "+name+" "+arch.String())

		// Create a phony rule that depends on the output file and prints the path.
		phony := android.PathForPhony(ctx, "dump-oat-"+name+"-"+suffix)
		rule = android.NewRuleBuilder(pctx, ctx)
		rule.Command().
			Implicit(output).
			ImplicitOutput(phony).
			Text("echo").FlagWithArg("Output in ", output.String())
		rule.Build("phony-dump-oat-"+name+"-"+suffix, "dump oat "+name+" "+arch.String())

		allPhonies = append(allPhonies, phony)
	}

	phony := android.PathForPhony(ctx, "dump-oat-"+name)
	ctx.Build(pctx, android.BuildParams{
		Rule:        android.Phony,
		Output:      phony,
		Inputs:      allPhonies,
		Description: "dump-oat-" + name,
	})
}

func writeGlobalConfigForMake(ctx android.SingletonContext, path android.WritablePath) {
	data := dexpreopt.GetGlobalConfigRawData(ctx)

	android.WriteFileRule(ctx, path, string(data))
}

// Define Make variables for boot image names, paths, etc. These variables are used in makefiles
// (make/core/dex_preopt_libart.mk) to generate install rules that copy boot image files to the
// correct output directories.
func (d *dexpreoptBootJars) MakeVars(ctx android.MakeVarsContext) {
	if d.dexpreoptConfigForMake != nil && !SkipDexpreoptBootJars(ctx) {
		ctx.Strict("DEX_PREOPT_CONFIG_FOR_MAKE", d.dexpreoptConfigForMake.String())
		ctx.Strict("DEX_PREOPT_SOONG_CONFIG_FOR_MAKE", android.PathForOutput(ctx, "dexpreopt_soong.config").String())
	}

	image := d.defaultBootImage
	if image != nil {
		if profileInstallInfo, ok := android.SingletonModuleProvider(ctx, d, profileInstallInfoProvider); ok {
			ctx.Strict("DEXPREOPT_IMAGE_PROFILE_BUILT_INSTALLED", profileInstallInfo.profileInstalls.String())
			if profileInstallInfo.profileLicenseMetadataFile.Valid() {
				ctx.Strict("DEXPREOPT_IMAGE_PROFILE_LICENSE_METADATA", profileInstallInfo.profileLicenseMetadataFile.String())
			}
		}

		if SkipDexpreoptBootJars(ctx) {
			return
		}

		global := dexpreopt.GetGlobalConfig(ctx)
		dexPaths, dexLocations := bcpForDexpreopt(ctx, global.PreoptWithUpdatableBcp)
		ctx.Strict("DEXPREOPT_BOOTCLASSPATH_DEX_FILES", strings.Join(dexPaths.Strings(), " "))
		ctx.Strict("DEXPREOPT_BOOTCLASSPATH_DEX_LOCATIONS", strings.Join(dexLocations, " "))

		// The primary ART boot image is exposed to Make for testing (gtests) and benchmarking
		// (golem) purposes.
		for _, current := range append(d.otherImages, image) {
			for _, variant := range current.variants {
				suffix := ""
				if variant.target.Os.Class == android.Host {
					suffix = "_host"
				}
				sfx := variant.name + suffix + "_" + variant.target.Arch.ArchType.String()
				ctx.Strict("DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_"+sfx, variant.vdexInstalls.String())
				ctx.Strict("DEXPREOPT_IMAGE_"+sfx, variant.imagePathOnHost.String())
				ctx.Strict("DEXPREOPT_IMAGE_DEPS_"+sfx, strings.Join(variant.imagesDeps.Strings(), " "))
				ctx.Strict("DEXPREOPT_IMAGE_BUILT_INSTALLED_"+sfx, variant.installs.String())
				ctx.Strict("DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_"+sfx, variant.unstrippedInstalls.String())
				if variant.licenseMetadataFile.Valid() {
					ctx.Strict("DEXPREOPT_IMAGE_LICENSE_METADATA_"+sfx, variant.licenseMetadataFile.String())
				}
			}
			imageLocationsOnHost, imageLocationsOnDevice := current.getAnyAndroidVariant().imageLocations()
			ctx.Strict("DEXPREOPT_IMAGE_LOCATIONS_ON_HOST"+current.name, strings.Join(imageLocationsOnHost, ":"))
			ctx.Strict("DEXPREOPT_IMAGE_LOCATIONS_ON_DEVICE"+current.name, strings.Join(imageLocationsOnDevice, ":"))
			ctx.Strict("DEXPREOPT_IMAGE_ZIP_"+current.name, current.zip.String())
		}
		ctx.Strict("DEXPREOPT_IMAGE_NAMES", strings.Join(getImageNames(), " "))
	}
}
