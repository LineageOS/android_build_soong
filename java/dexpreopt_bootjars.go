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
	"sort"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"

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
// Each component of the PRODUCT_BOOT_JARS and PRODUCT_BOOT_JARS_EXTRA variables is either a simple
// name (if the library is a part of the Platform), or a colon-separated pair <apex, name> (if the
// library is a part of a non-updatable APEX).
//
// A related variable PRODUCT_UPDATABLE_BOOT_JARS contains bootclasspath libraries that are in
// updatable APEXes. They are not included in the boot image.
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
// 2.5. JIT-Zygote configuration
// -----------------------------
//
// One special configuration is JIT-Zygote build, when the primary ART image is used for compiling
// apps instead of the Framework boot image extension (see DEXPREOPT_USE_ART_IMAGE and UseArtImage).
//

var artApexNames = []string{
	"com.android.art",
	"com.android.art.debug",
	"com.android.art.testing",
	"com.google.android.art",
	"com.google.android.art.debug",
	"com.google.android.art.testing",
}

func init() {
	RegisterDexpreoptBootJarsComponents(android.InitRegistrationContext)
}

// Target-independent description of a boot image.
type bootImageConfig struct {
	// If this image is an extension, the image that it extends.
	extends *bootImageConfig

	// Image name (used in directory names and ninja rule names).
	name string

	// Basename of the image: the resulting filenames are <stem>[-<jar>].{art,oat,vdex}.
	stem string

	// Output directory for the image files.
	dir android.OutputPath

	// Output directory for the image files with debug symbols.
	symbolsDir android.OutputPath

	// Subdirectory where the image files are installed.
	installDirOnHost string

	// Subdirectory where the image files on device are installed.
	installDirOnDevice string

	// A list of (location, jar) pairs for the Java modules in this image.
	modules android.ConfiguredJarList

	// File paths to jars.
	dexPaths     android.WritablePaths // for this image
	dexPathsDeps android.WritablePaths // for the dependency images and in this image

	// Map from module name (without prebuilt_ prefix) to the predefined build path.
	dexPathsByModule map[string]android.WritablePath

	// File path to a zip archive with all image files (or nil, if not needed).
	zip android.WritablePath

	// Rules which should be used in make to install the outputs.
	profileInstalls android.RuleBuilderInstalls

	// Target-dependent fields.
	variants []*bootImageVariant
}

// Target-dependent description of a boot image.
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

	// The path to the primary image variant's imagePathOnHost field, where primary image variant
	// means the image variant that this extends.
	//
	// This is only set for a variant of an image that extends another image.
	primaryImages android.OutputPath

	// The paths to the primary image variant's imagesDeps field, where primary image variant
	// means the image variant that this extends.
	//
	// This is only set for a variant of an image that extends another image.
	primaryImagesDeps android.Paths

	// Rules which should be used in make to install the outputs.
	installs           android.RuleBuilderInstalls
	vdexInstalls       android.RuleBuilderInstalls
	unstrippedInstalls android.RuleBuilderInstalls
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
		name += "-" + android.ModuleStem(m)
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
	}
	return ret
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
//
func (image *bootImageVariant) imageLocations() (imageLocationsOnHost []string, imageLocationsOnDevice []string) {
	if image.extends != nil {
		imageLocationsOnHost, imageLocationsOnDevice = image.extends.getVariant(image.target).imageLocations()
	}
	return append(imageLocationsOnHost, dexpreopt.PathToLocation(image.imagePathOnHost, image.target.Arch.ArchType)),
		append(imageLocationsOnDevice, dexpreopt.PathStringToLocation(image.imagePathOnDevice, image.target.Arch.ArchType))
}

func dexpreoptBootJarsFactory() android.SingletonModule {
	m := &dexpreoptBootJars{}
	android.InitAndroidModule(m)
	return m
}

func RegisterDexpreoptBootJarsComponents(ctx android.RegistrationContext) {
	ctx.RegisterSingletonModuleType("dex_bootjars", dexpreoptBootJarsFactory)
}

func SkipDexpreoptBootJars(ctx android.PathContext) bool {
	return dexpreopt.GetGlobalConfig(ctx).DisablePreoptBootImages
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

// Provide paths to boot images for use by modules that depend upon them.
//
// The build rules are created in GenerateSingletonBuildActions().
func (d *dexpreoptBootJars) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Placeholder for now.
}

// Generate build rules for boot images.
func (d *dexpreoptBootJars) GenerateSingletonBuildActions(ctx android.SingletonContext) {
	if SkipDexpreoptBootJars(ctx) {
		return
	}
	if dexpreopt.GetCachedGlobalSoongConfig(ctx) == nil {
		// No module has enabled dexpreopting, so we assume there will be no boot image to make.
		return
	}

	d.dexpreoptConfigForMake = android.PathForOutput(ctx, ctx.Config().DeviceName(), "dexpreopt.config")
	writeGlobalConfigForMake(ctx, d.dexpreoptConfigForMake)

	global := dexpreopt.GetGlobalConfig(ctx)
	if !shouldBuildBootImages(ctx.Config(), global) {
		return
	}

	defaultImageConfig := defaultBootImageConfig(ctx)
	d.defaultBootImage = defaultImageConfig
	artBootImageConfig := artBootImageConfig(ctx)
	d.otherImages = []*bootImageConfig{artBootImageConfig}
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

// copyBootJarsToPredefinedLocations generates commands that will copy boot jars to predefined
// paths in the global config.
func copyBootJarsToPredefinedLocations(ctx android.ModuleContext, srcBootDexJarsByModule bootDexJarByModule, dstBootJarsByModule map[string]android.WritablePath) {
	// Create the super set of module names.
	names := []string{}
	names = append(names, android.SortedStringKeys(srcBootDexJarsByModule)...)
	names = append(names, android.SortedStringKeys(dstBootJarsByModule)...)
	names = android.SortedUniqueStrings(names)
	for _, name := range names {
		src := srcBootDexJarsByModule[name]
		dst := dstBootJarsByModule[name]

		if src == nil {
			ctx.ModuleErrorf("module %s does not provide a dex boot jar", name)
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

// buildBootImage takes a bootImageConfig, creates rules to build it, and returns the image.
func buildBootImage(ctx android.ModuleContext, image *bootImageConfig, profile android.WritablePath) {
	var zipFiles android.Paths
	for _, variant := range image.variants {
		files := buildBootImageVariant(ctx, variant, profile)
		if variant.target.Os == android.Android {
			zipFiles = append(zipFiles, files.Paths()...)
		}
	}

	if image.zip != nil {
		rule := android.NewRuleBuilder(pctx, ctx)
		rule.Command().
			BuiltTool("soong_zip").
			FlagWithOutput("-o ", image.zip).
			FlagWithArg("-C ", image.dir.Join(ctx, android.Android.String()).String()).
			FlagWithInputList("-f ", zipFiles, " -f ")

		rule.Build("zip_"+image.name, "zip "+image.name+" image")
	}
}

// Generate boot image build rules for a specific target.
func buildBootImageVariant(ctx android.ModuleContext, image *bootImageVariant, profile android.Path) android.WritablePaths {

	globalSoong := dexpreopt.GetCachedGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)

	arch := image.target.Arch.ArchType
	os := image.target.Os.String() // We need to distinguish host-x86 and device-x86.
	symbolsDir := image.symbolsDir.Join(ctx, os, image.installDirOnHost, arch.String())
	symbolsFile := symbolsDir.Join(ctx, image.stem+".oat")
	outputDir := image.dir.Join(ctx, os, image.installDirOnHost, arch.String())
	outputPath := outputDir.Join(ctx, image.stem+".oat")
	oatLocation := dexpreopt.PathToLocation(outputPath, arch)
	imagePath := outputPath.ReplaceExtension(ctx, "art")

	rule := android.NewRuleBuilder(pctx, ctx)

	rule.Command().Text("mkdir").Flag("-p").Flag(symbolsDir.String())
	rule.Command().Text("rm").Flag("-f").
		Flag(symbolsDir.Join(ctx, "*.art").String()).
		Flag(symbolsDir.Join(ctx, "*.oat").String()).
		Flag(symbolsDir.Join(ctx, "*.invocation").String())
	rule.Command().Text("rm").Flag("-f").
		Flag(outputDir.Join(ctx, "*.art").String()).
		Flag(outputDir.Join(ctx, "*.oat").String()).
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

	cmd.Tool(globalSoong.Dex2oat).
		Flag("--avoid-storing-invocation").
		FlagWithOutput("--write-invocation-to=", invocationPath).ImplicitOutput(invocationPath).
		Flag("--runtime-arg").FlagWithArg("-Xms", global.Dex2oatImageXms).
		Flag("--runtime-arg").FlagWithArg("-Xmx", global.Dex2oatImageXmx)

	if profile != nil {
		cmd.FlagWithArg("--compiler-filter=", "speed-profile")
		cmd.FlagWithInput("--profile-file=", profile)
	}

	dirtyImageFile := "frameworks/base/config/dirty-image-objects"
	dirtyImagePath := android.ExistentPathForSource(ctx, dirtyImageFile)
	if dirtyImagePath.Valid() {
		cmd.FlagWithInput("--dirty-image-objects=", dirtyImagePath.Path())
	}

	if image.extends != nil {
		// It is a boot image extension, so it needs the boot image it depends on (in this case the
		// primary ART APEX image).
		artImage := image.primaryImages
		cmd.
			Flag("--runtime-arg").FlagWithInputList("-Xbootclasspath:", image.dexPathsDeps.Paths(), ":").
			Flag("--runtime-arg").FlagWithList("-Xbootclasspath-locations:", image.dexLocationsDeps, ":").
			// Add the path to the first file in the boot image with the arch specific directory removed,
			// dex2oat will reconstruct the path to the actual file when it needs it. As the actual path
			// to the file cannot be passed to the command make sure to add the actual path as an Implicit
			// dependency to ensure that it is built before the command runs.
			FlagWithArg("--boot-image=", dexpreopt.PathToLocation(artImage, arch)).Implicit(artImage).
			// Similarly, the dex2oat tool will automatically find the paths to other files in the base
			// boot image so make sure to add them as implicit dependencies to ensure that they are built
			// before this command is run.
			Implicits(image.primaryImagesDeps)
	} else {
		// It is a primary image, so it needs a base address.
		cmd.FlagWithArg("--base=", ctx.Config().LibartImgDeviceBaseAddress())
	}

	cmd.
		FlagForEachInput("--dex-file=", image.dexPaths.Paths()).
		FlagForEachArg("--dex-location=", image.dexLocations).
		Flag("--generate-debug-info").
		Flag("--generate-build-id").
		Flag("--image-format=lz4hc").
		FlagWithArg("--oat-symbols=", symbolsFile.String()).
		Flag("--strip").
		FlagWithArg("--oat-file=", outputPath.String()).
		FlagWithArg("--oat-location=", oatLocation).
		FlagWithArg("--image=", imagePath.String()).
		FlagWithArg("--instruction-set=", arch.String()).
		FlagWithArg("--android-root=", global.EmptyDirectory).
		FlagWithArg("--no-inline-from=", "core-oj.jar").
		Flag("--force-determinism").
		Flag("--abort-on-hard-verifier-error")

	// Use the default variant/features for host builds.
	// The map below contains only device CPU info (which might be x86 on some devices).
	if image.target.Os == android.Android {
		cmd.FlagWithArg("--instruction-set-variant=", global.CpuVariant[arch])
		cmd.FlagWithArg("--instruction-set-features=", global.InstructionSetFeatures[arch])
	}

	if global.BootFlags != "" {
		cmd.Flag(global.BootFlags)
	}

	if extraFlags != "" {
		cmd.Flag(extraFlags)
	}

	cmd.Textf(`|| ( echo %s ; false )`, proptools.ShellEscape(failureMessage))

	installDir := filepath.Join("/", image.installDirOnHost, arch.String())

	var vdexInstalls android.RuleBuilderInstalls
	var unstrippedInstalls android.RuleBuilderInstalls

	var zipFiles android.WritablePaths

	for _, artOrOat := range image.moduleFiles(ctx, outputDir, ".art", ".oat") {
		cmd.ImplicitOutput(artOrOat)
		zipFiles = append(zipFiles, artOrOat)

		// Install the .oat and .art files
		rule.Install(artOrOat, filepath.Join(installDir, artOrOat.Base()))
	}

	for _, vdex := range image.moduleFiles(ctx, outputDir, ".vdex") {
		cmd.ImplicitOutput(vdex)
		zipFiles = append(zipFiles, vdex)

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
	image.installs = rule.Installs()
	image.vdexInstalls = vdexInstalls
	image.unstrippedInstalls = unstrippedInstalls

	return zipFiles
}

const failureMessage = `ERROR: Dex2oat failed to compile a boot image.
It is likely that the boot classpath is inconsistent.
Rebuild with ART_BOOT_IMAGE_EXTRA_ARGS="--runtime-arg -verbose:verifier" to see verification errors.`

func bootImageProfileRule(ctx android.ModuleContext, image *bootImageConfig) android.WritablePath {
	globalSoong := dexpreopt.GetCachedGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)

	if global.DisableGenerateProfile {
		return nil
	}

	defaultProfile := "frameworks/base/config/boot-image-profile.txt"

	rule := android.NewRuleBuilder(pctx, ctx)

	var bootImageProfile android.Path
	if len(global.BootImageProfiles) > 1 {
		combinedBootImageProfile := image.dir.Join(ctx, "boot-image-profile.txt")
		rule.Command().Text("cat").Inputs(global.BootImageProfiles).Text(">").Output(combinedBootImageProfile)
		bootImageProfile = combinedBootImageProfile
	} else if len(global.BootImageProfiles) == 1 {
		bootImageProfile = global.BootImageProfiles[0]
	} else if path := android.ExistentPathForSource(ctx, defaultProfile); path.Valid() {
		bootImageProfile = path.Path()
	} else {
		// No profile (not even a default one, which is the case on some branches
		// like master-art-host that don't have frameworks/base).
		// Return nil and continue without profile.
		return nil
	}

	profile := image.dir.Join(ctx, "boot.prof")

	rule.Command().
		Text(`ANDROID_LOG_TAGS="*:e"`).
		Tool(globalSoong.Profman).
		Flag("--output-profile-type=boot").
		FlagWithInput("--create-profile-from=", bootImageProfile).
		FlagForEachInput("--apk=", image.dexPathsDeps.Paths()).
		FlagForEachArg("--dex-location=", image.getAnyAndroidVariant().dexLocationsDeps).
		FlagWithOutput("--reference-profile-file=", profile)

	rule.Install(profile, "/system/etc/boot-image.prof")

	rule.Build("bootJarsProfile", "profile boot jars")

	image.profileInstalls = append(image.profileInstalls, rule.Installs()...)

	return profile
}

// bootFrameworkProfileRule generates the rule to create the boot framework profile and
// returns a path to the generated file.
func bootFrameworkProfileRule(ctx android.ModuleContext, image *bootImageConfig) android.WritablePath {
	globalSoong := dexpreopt.GetGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)

	if global.DisableGenerateProfile || ctx.Config().UnbundledBuild() {
		return nil
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
	image.profileInstalls = append(image.profileInstalls, rule.Installs()...)

	return profile
}

// generateUpdatableBcpPackagesRule generates the rule to create the updatable-bcp-packages.txt file
// and returns a path to the generated file.
func generateUpdatableBcpPackagesRule(ctx android.ModuleContext, image *bootImageConfig, updatableModules []android.Module) android.WritablePath {
	// Collect `permitted_packages` for updatable boot jars.
	var updatablePackages []string
	for _, module := range updatableModules {
		if j, ok := module.(PermittedPackagesForUpdatableBootJars); ok {
			pp := j.PermittedPackagesForUpdatableBootJars()
			if len(pp) > 0 {
				updatablePackages = append(updatablePackages, pp...)
			} else {
				ctx.OtherModuleErrorf(module, "Missing permitted_packages")
			}
		}
	}

	// Sort updatable packages to ensure deterministic ordering.
	sort.Strings(updatablePackages)

	updatableBcpPackagesName := "updatable-bcp-packages.txt"
	updatableBcpPackages := image.dir.Join(ctx, updatableBcpPackagesName)

	// WriteFileRule automatically adds the last end-of-line.
	android.WriteFileRule(ctx, updatableBcpPackages, strings.Join(updatablePackages, "\n"))

	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Install(updatableBcpPackages, "/system/etc/"+updatableBcpPackagesName)
	// TODO: Rename `profileInstalls` to `extraInstalls`?
	// Maybe even move the field out of the bootImageConfig into some higher level type?
	image.profileInstalls = append(image.profileInstalls, rule.Installs()...)

	return updatableBcpPackages
}

func dumpOatRules(ctx android.ModuleContext, image *bootImageConfig) {
	var allPhonies android.Paths
	for _, image := range image.variants {
		arch := image.target.Arch.ArchType
		suffix := arch.String()
		// Host and target might both use x86 arch. We need to ensure the names are unique.
		if image.target.Os.Class == android.Host {
			suffix = "host-" + suffix
		}
		// Create a rule to call oatdump.
		output := android.PathForOutput(ctx, "boot."+suffix+".oatdump.txt")
		rule := android.NewRuleBuilder(pctx, ctx)
		imageLocationsOnHost, _ := image.imageLocations()
		rule.Command().
			BuiltTool("oatdump").
			FlagWithInputList("--runtime-arg -Xbootclasspath:", image.dexPathsDeps.Paths(), ":").
			FlagWithList("--runtime-arg -Xbootclasspath-locations:", image.dexLocationsDeps, ":").
			FlagWithArg("--image=", strings.Join(imageLocationsOnHost, ":")).Implicits(image.imagesDeps.Paths()).
			FlagWithOutput("--output=", output).
			FlagWithArg("--instruction-set=", arch.String())
		rule.Build("dump-oat-boot-"+suffix, "dump oat boot "+arch.String())

		// Create a phony rule that depends on the output file and prints the path.
		phony := android.PathForPhony(ctx, "dump-oat-boot-"+suffix)
		rule = android.NewRuleBuilder(pctx, ctx)
		rule.Command().
			Implicit(output).
			ImplicitOutput(phony).
			Text("echo").FlagWithArg("Output in ", output.String())
		rule.Build("phony-dump-oat-boot-"+suffix, "dump oat boot "+arch.String())

		allPhonies = append(allPhonies, phony)
	}

	phony := android.PathForPhony(ctx, "dump-oat-boot")
	ctx.Build(pctx, android.BuildParams{
		Rule:        android.Phony,
		Output:      phony,
		Inputs:      allPhonies,
		Description: "dump-oat-boot",
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
	if d.dexpreoptConfigForMake != nil {
		ctx.Strict("DEX_PREOPT_CONFIG_FOR_MAKE", d.dexpreoptConfigForMake.String())
		ctx.Strict("DEX_PREOPT_SOONG_CONFIG_FOR_MAKE", android.PathForOutput(ctx, "dexpreopt_soong.config").String())
	}

	image := d.defaultBootImage
	if image != nil {
		ctx.Strict("DEXPREOPT_IMAGE_PROFILE_BUILT_INSTALLED", image.profileInstalls.String())

		global := dexpreopt.GetGlobalConfig(ctx)
		dexPaths, dexLocations := bcpForDexpreopt(ctx, global.PreoptWithUpdatableBcp)
		ctx.Strict("DEXPREOPT_BOOTCLASSPATH_DEX_FILES", strings.Join(dexPaths.Strings(), " "))
		ctx.Strict("DEXPREOPT_BOOTCLASSPATH_DEX_LOCATIONS", strings.Join(dexLocations, " "))

		var imageNames []string
		// TODO: the primary ART boot image should not be exposed to Make, as it is installed in a
		// different way as a part of the ART APEX. However, there is a special JIT-Zygote build
		// configuration which uses the primary ART image instead of the Framework boot image
		// extension, and it relies on the ART image being exposed to Make. To fix this, it is
		// necessary to rework the logic in makefiles.
		for _, current := range append(d.otherImages, image) {
			imageNames = append(imageNames, current.name)
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
			}
			imageLocationsOnHost, _ := current.getAnyAndroidVariant().imageLocations()
			ctx.Strict("DEXPREOPT_IMAGE_LOCATIONS_"+current.name, strings.Join(imageLocationsOnHost, ":"))
			ctx.Strict("DEXPREOPT_IMAGE_ZIP_"+current.name, current.zip.String())
		}
		ctx.Strict("DEXPREOPT_IMAGE_NAMES", strings.Join(imageNames, " "))
	}
}
