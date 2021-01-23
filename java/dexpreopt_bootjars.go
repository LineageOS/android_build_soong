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
	installSubdir string

	// A list of (location, jar) pairs for the Java modules in this image.
	modules android.ConfiguredJarList

	// File paths to jars.
	dexPaths     android.WritablePaths // for this image
	dexPathsDeps android.WritablePaths // for the dependency images and in this image

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
	images     android.OutputPath  // first image file
	imagesDeps android.OutputPaths // all files

	// Only for extensions, paths to the primary boot images.
	primaryImages android.OutputPath

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
func (image *bootImageVariant) imageLocations() (imageLocations []string) {
	if image.extends != nil {
		imageLocations = image.extends.getVariant(image.target).imageLocations()
	}
	return append(imageLocations, dexpreopt.PathToLocation(image.images, image.target.Arch.ArchType))
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

// Accessor function for the apex package. Returns nil if dexpreopt is disabled.
func DexpreoptedArtApexJars(ctx android.BuilderContext) map[android.ArchType]android.OutputPaths {
	if SkipDexpreoptBootJars(ctx) {
		return nil
	}
	// Include dexpreopt files for the primary boot image.
	files := map[android.ArchType]android.OutputPaths{}
	for _, variant := range artBootImageConfig(ctx).variants {
		// We also generate boot images for host (for testing), but we don't need those in the apex.
		if variant.target.Os == android.Android {
			files[variant.target.Arch.ArchType] = variant.imagesDeps
		}
	}
	return files
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

	// Skip recompiling the boot image for the second sanitization phase. We'll get separate paths
	// and invalidate first-stage artifacts which are crucial to SANITIZE_LITE builds.
	// Note: this is technically incorrect. Compiled code contains stack checks which may depend
	//       on ASAN settings.
	if len(ctx.Config().SanitizeDevice()) == 1 &&
		ctx.Config().SanitizeDevice()[0] == "address" &&
		global.SanitizeLite {
		return
	}

	// Always create the default boot image first, to get a unique profile rule for all images.
	d.defaultBootImage = buildBootImage(ctx, defaultBootImageConfig(ctx))
	// Create boot image for the ART apex (build artifacts are accessed via the global boot image config).
	d.otherImages = append(d.otherImages, buildBootImage(ctx, artBootImageConfig(ctx)))

	dumpOatRules(ctx, d.defaultBootImage)
}

// Inspect this module to see if it contains a bootclasspath dex jar.
// Note that the same jar may occur in multiple modules.
// This logic is tested in the apex package to avoid import cycle apex <-> java.
func getBootImageJar(ctx android.SingletonContext, image *bootImageConfig, module android.Module) (int, android.Path) {
	name := ctx.ModuleName(module)

	// Strip a prebuilt_ prefix so that this can access the dex jar from a prebuilt module.
	name = android.RemoveOptionalPrebuiltPrefix(name)

	// Ignore any module that is not listed in the boot image configuration.
	index := image.modules.IndexOfJar(name)
	if index == -1 {
		return -1, nil
	}

	// It is an error if a module configured in the boot image does not support accessing the dex jar.
	// This is safe because every module that has the same name has to have the same module type.
	jar, hasJar := module.(interface{ DexJarBuildPath() android.Path })
	if !hasJar {
		ctx.Errorf("module %q configured in boot image %q does not support accessing dex jar", module, image.name)
		return -1, nil
	}

	// It is also an error if the module is not an ApexModule.
	if _, ok := module.(android.ApexModule); !ok {
		ctx.Errorf("module %q configured in boot image %q does not support being added to an apex", module, image.name)
		return -1, nil
	}

	apexInfo := ctx.ModuleProvider(module, android.ApexInfoProvider).(android.ApexInfo)

	// Now match the apex part of the boot image configuration.
	requiredApex := image.modules.Apex(index)
	if requiredApex == "platform" {
		if len(apexInfo.InApexes) != 0 {
			// A platform variant is required but this is for an apex so ignore it.
			return -1, nil
		}
	} else if !android.InList(requiredApex, apexInfo.InApexes) {
		// An apex variant for a specific apex is required but this is the wrong apex.
		return -1, nil
	}

	// Check that this module satisfies any boot image specific constraints.
	fromUpdatableApex := apexInfo.Updatable

	switch image.name {
	case artBootImageName:
		if len(apexInfo.InApexes) > 0 && allHavePrefix(apexInfo.InApexes, "com.android.art") {
			// ok: found the jar in the ART apex
		} else if name == "jacocoagent" && ctx.Config().IsEnvTrue("EMMA_INSTRUMENT_FRAMEWORK") {
			// exception (skip and continue): Jacoco platform variant for a coverage build
			return -1, nil
		} else if fromUpdatableApex {
			// error: this jar is part of an updatable apex other than ART
			ctx.Errorf("module %q from updatable apexes %q is not allowed in the ART boot image", name, apexInfo.InApexes)
		} else {
			// error: this jar is part of the platform or a non-updatable apex
			ctx.Errorf("module %q is not allowed in the ART boot image", name)
		}

	case frameworkBootImageName:
		if !fromUpdatableApex {
			// ok: this jar is part of the platform or a non-updatable apex
		} else {
			// error: this jar is part of an updatable apex
			ctx.Errorf("module %q from updatable apexes %q is not allowed in the framework boot image", name, apexInfo.InApexes)
		}
	default:
		panic("unknown boot image: " + image.name)
	}

	return index, jar.DexJarBuildPath()
}

func allHavePrefix(list []string, prefix string) bool {
	for _, s := range list {
		if s != prefix && !strings.HasPrefix(s, prefix+".") {
			return false
		}
	}
	return true
}

// buildBootImage takes a bootImageConfig, creates rules to build it, and returns the image.
func buildBootImage(ctx android.SingletonContext, image *bootImageConfig) *bootImageConfig {
	// Collect dex jar paths for the boot image modules.
	// This logic is tested in the apex package to avoid import cycle apex <-> java.
	bootDexJars := make(android.Paths, image.modules.Len())
	ctx.VisitAllModules(func(module android.Module) {
		if i, j := getBootImageJar(ctx, image, module); i != -1 {
			if existing := bootDexJars[i]; existing != nil {
				ctx.Errorf("Multiple dex jars found for %s:%s - %s and %s",
					image.modules.Apex(i), image.modules.Jar(i), existing, j)
				return
			}

			bootDexJars[i] = j
		}
	})

	var missingDeps []string
	// Ensure all modules were converted to paths
	for i := range bootDexJars {
		if bootDexJars[i] == nil {
			m := image.modules.Jar(i)
			if ctx.Config().AllowMissingDependencies() {
				missingDeps = append(missingDeps, m)
				bootDexJars[i] = android.PathForOutput(ctx, "missing/module", m, "from/apex", image.modules.Apex(i))
			} else {
				ctx.Errorf("failed to find a dex jar path for module '%s'"+
					", note that some jars may be filtered out by module constraints", m)
			}
		}
	}

	// The paths to bootclasspath DEX files need to be known at module GenerateAndroidBuildAction
	// time, before the boot images are built (these paths are used in dexpreopt rule generation for
	// Java libraries and apps). Generate rules that copy bootclasspath DEX jars to the predefined
	// paths.
	for i := range bootDexJars {
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.Cp,
			Input:  bootDexJars[i],
			Output: image.dexPaths[i],
		})
	}

	profile := bootImageProfileRule(ctx, image, missingDeps)
	bootFrameworkProfileRule(ctx, image, missingDeps)
	updatableBcpPackagesRule(ctx, image, missingDeps)

	var zipFiles android.Paths
	for _, variant := range image.variants {
		files := buildBootImageVariant(ctx, variant, profile, missingDeps)
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

	return image
}

// Generate boot image build rules for a specific target.
func buildBootImageVariant(ctx android.SingletonContext, image *bootImageVariant,
	profile android.Path, missingDeps []string) android.WritablePaths {

	globalSoong := dexpreopt.GetCachedGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)

	arch := image.target.Arch.ArchType
	os := image.target.Os.String() // We need to distinguish host-x86 and device-x86.
	symbolsDir := image.symbolsDir.Join(ctx, os, image.installSubdir, arch.String())
	symbolsFile := symbolsDir.Join(ctx, image.stem+".oat")
	outputDir := image.dir.Join(ctx, os, image.installSubdir, arch.String())
	outputPath := outputDir.Join(ctx, image.stem+".oat")
	oatLocation := dexpreopt.PathToLocation(outputPath, arch)
	imagePath := outputPath.ReplaceExtension(ctx, "art")

	rule := android.NewRuleBuilder(pctx, ctx)
	rule.MissingDeps(missingDeps)

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
			FlagWithArg("--boot-image=", dexpreopt.PathToLocation(artImage, arch)).Implicit(artImage)
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

	installDir := filepath.Join("/", image.installSubdir, arch.String())

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

func bootImageProfileRule(ctx android.SingletonContext, image *bootImageConfig, missingDeps []string) android.WritablePath {
	globalSoong := dexpreopt.GetCachedGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)

	if global.DisableGenerateProfile {
		return nil
	}
	profile := ctx.Config().Once(bootImageProfileRuleKey, func() interface{} {
		defaultProfile := "frameworks/base/config/boot-image-profile.txt"

		rule := android.NewRuleBuilder(pctx, ctx)
		rule.MissingDeps(missingDeps)

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
			FlagWithInput("--create-profile-from=", bootImageProfile).
			FlagForEachInput("--apk=", image.dexPathsDeps.Paths()).
			FlagForEachArg("--dex-location=", image.getAnyAndroidVariant().dexLocationsDeps).
			FlagWithOutput("--reference-profile-file=", profile)

		rule.Install(profile, "/system/etc/boot-image.prof")

		rule.Build("bootJarsProfile", "profile boot jars")

		image.profileInstalls = rule.Installs()

		return profile
	})
	if profile == nil {
		return nil // wrap nil into a typed pointer with value nil
	}
	return profile.(android.WritablePath)
}

var bootImageProfileRuleKey = android.NewOnceKey("bootImageProfileRule")

func bootFrameworkProfileRule(ctx android.SingletonContext, image *bootImageConfig, missingDeps []string) android.WritablePath {
	globalSoong := dexpreopt.GetCachedGlobalSoongConfig(ctx)
	global := dexpreopt.GetGlobalConfig(ctx)

	if global.DisableGenerateProfile || ctx.Config().UnbundledBuild() {
		return nil
	}
	return ctx.Config().Once(bootFrameworkProfileRuleKey, func() interface{} {
		rule := android.NewRuleBuilder(pctx, ctx)
		rule.MissingDeps(missingDeps)

		// Some branches like master-art-host don't have frameworks/base, so manually
		// handle the case that the default is missing.  Those branches won't attempt to build the profile rule,
		// and if they do they'll get a missing deps error.
		defaultProfile := "frameworks/base/config/boot-profile.txt"
		path := android.ExistentPathForSource(ctx, defaultProfile)
		var bootFrameworkProfile android.Path
		if path.Valid() {
			bootFrameworkProfile = path.Path()
		} else {
			missingDeps = append(missingDeps, defaultProfile)
			bootFrameworkProfile = android.PathForOutput(ctx, "missing", defaultProfile)
		}

		profile := image.dir.Join(ctx, "boot.bprof")

		rule.Command().
			Text(`ANDROID_LOG_TAGS="*:e"`).
			Tool(globalSoong.Profman).
			Flag("--generate-boot-profile").
			FlagWithInput("--create-profile-from=", bootFrameworkProfile).
			FlagForEachInput("--apk=", image.dexPathsDeps.Paths()).
			FlagForEachArg("--dex-location=", image.getAnyAndroidVariant().dexLocationsDeps).
			FlagWithOutput("--reference-profile-file=", profile)

		rule.Install(profile, "/system/etc/boot-image.bprof")
		rule.Build("bootFrameworkProfile", "profile boot framework jars")
		image.profileInstalls = append(image.profileInstalls, rule.Installs()...)

		return profile
	}).(android.WritablePath)
}

var bootFrameworkProfileRuleKey = android.NewOnceKey("bootFrameworkProfileRule")

func updatableBcpPackagesRule(ctx android.SingletonContext, image *bootImageConfig, missingDeps []string) android.WritablePath {
	if ctx.Config().UnbundledBuild() {
		return nil
	}

	return ctx.Config().Once(updatableBcpPackagesRuleKey, func() interface{} {
		global := dexpreopt.GetGlobalConfig(ctx)
		updatableModules := global.UpdatableBootJars.CopyOfJars()

		// Collect `permitted_packages` for updatable boot jars.
		var updatablePackages []string
		ctx.VisitAllModules(func(module android.Module) {
			if j, ok := module.(PermittedPackagesForUpdatableBootJars); ok {
				name := ctx.ModuleName(module)
				if i := android.IndexList(name, updatableModules); i != -1 {
					pp := j.PermittedPackagesForUpdatableBootJars()
					if len(pp) > 0 {
						updatablePackages = append(updatablePackages, pp...)
					} else {
						ctx.Errorf("Missing permitted_packages for %s", name)
					}
					// Do not match the same library repeatedly.
					updatableModules = append(updatableModules[:i], updatableModules[i+1:]...)
				}
			}
		})

		// Sort updatable packages to ensure deterministic ordering.
		sort.Strings(updatablePackages)

		updatableBcpPackagesName := "updatable-bcp-packages.txt"
		updatableBcpPackages := image.dir.Join(ctx, updatableBcpPackagesName)

		// WriteFileRule automatically adds the last end-of-line.
		android.WriteFileRule(ctx, updatableBcpPackages, strings.Join(updatablePackages, "\n"))

		rule := android.NewRuleBuilder(pctx, ctx)
		rule.MissingDeps(missingDeps)
		rule.Install(updatableBcpPackages, "/system/etc/"+updatableBcpPackagesName)
		// TODO: Rename `profileInstalls` to `extraInstalls`?
		// Maybe even move the field out of the bootImageConfig into some higher level type?
		image.profileInstalls = append(image.profileInstalls, rule.Installs()...)

		return updatableBcpPackages
	}).(android.WritablePath)
}

var updatableBcpPackagesRuleKey = android.NewOnceKey("updatableBcpPackagesRule")

func dumpOatRules(ctx android.SingletonContext, image *bootImageConfig) {
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
		rule.Command().
			// TODO: for now, use the debug version for better error reporting
			BuiltTool("oatdumpd").
			FlagWithInputList("--runtime-arg -Xbootclasspath:", image.dexPathsDeps.Paths(), ":").
			FlagWithList("--runtime-arg -Xbootclasspath-locations:", image.dexLocationsDeps, ":").
			FlagWithArg("--image=", strings.Join(image.imageLocations(), ":")).Implicits(image.imagesDeps.Paths()).
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
		ctx.Strict("DEXPREOPT_BOOTCLASSPATH_DEX_FILES", strings.Join(image.dexPathsDeps.Strings(), " "))
		ctx.Strict("DEXPREOPT_BOOTCLASSPATH_DEX_LOCATIONS", strings.Join(image.getAnyAndroidVariant().dexLocationsDeps, " "))

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
				ctx.Strict("DEXPREOPT_IMAGE_"+sfx, variant.images.String())
				ctx.Strict("DEXPREOPT_IMAGE_DEPS_"+sfx, strings.Join(variant.imagesDeps.Strings(), " "))
				ctx.Strict("DEXPREOPT_IMAGE_BUILT_INSTALLED_"+sfx, variant.installs.String())
				ctx.Strict("DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_"+sfx, variant.unstrippedInstalls.String())
			}
			imageLocations := current.getAnyAndroidVariant().imageLocations()
			ctx.Strict("DEXPREOPT_IMAGE_LOCATIONS_"+current.name, strings.Join(imageLocations, ":"))
			ctx.Strict("DEXPREOPT_IMAGE_ZIP_"+current.name, current.zip.String())
		}
		ctx.Strict("DEXPREOPT_IMAGE_NAMES", strings.Join(imageNames, " "))
	}
}
