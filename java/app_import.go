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

package java

// This file contains the module implementations for android_app_import and android_test_import.

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/google/blueprint"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/provenance"
)

func init() {
	RegisterAppImportBuildComponents(android.InitRegistrationContext)

	initAndroidAppImportVariantGroupTypes()
}

var (
	uncompressEmbeddedJniLibsRule = pctx.AndroidStaticRule("uncompress-embedded-jni-libs", blueprint.RuleParams{
		Command: `if (zipinfo $in 'lib/*.so' 2>/dev/null | grep -v ' stor ' >/dev/null) ; then ` +
			`${config.Zip2ZipCmd} -i $in -o $out -0 'lib/**/*.so'` +
			`; else cp -f $in $out; fi`,
		CommandDeps: []string{"${config.Zip2ZipCmd}"},
		Description: "Uncompress embedded JNI libs",
	})

	uncompressDexRule = pctx.AndroidStaticRule("uncompress-dex", blueprint.RuleParams{
		Command: `if (zipinfo $in '*.dex' 2>/dev/null | grep -v ' stor ' >/dev/null) ; then ` +
			`${config.Zip2ZipCmd} -i $in -o $out -0 'classes*.dex'` +
			`; else cp -f $in $out; fi`,
		CommandDeps: []string{"${config.Zip2ZipCmd}"},
		Description: "Uncompress dex files",
	})

	checkPresignedApkRule = pctx.AndroidStaticRule("check-presigned-apk", blueprint.RuleParams{
		Command:     "build/soong/scripts/check_prebuilt_presigned_apk.py --aapt2 ${config.Aapt2Cmd} --zipalign ${config.ZipAlign} $extraArgs $in $out",
		CommandDeps: []string{"build/soong/scripts/check_prebuilt_presigned_apk.py", "${config.Aapt2Cmd}", "${config.ZipAlign}"},
		Description: "Check presigned apk",
	}, "extraArgs")
)

func RegisterAppImportBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_app_import", AndroidAppImportFactory)
	ctx.RegisterModuleType("android_test_import", AndroidTestImportFactory)
}

type AndroidAppImport struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase
	prebuilt android.Prebuilt

	properties       AndroidAppImportProperties
	dpiVariants      interface{}
	archVariants     interface{}
	arch_dpiVariants interface{}

	outputFile  android.Path
	certificate Certificate

	dexpreopter

	usesLibrary usesLibrary

	installPath android.InstallPath

	hideApexVariantFromMake bool

	provenanceMetaDataFile android.OutputPath

	// Single aconfig "cache file" merged from this module and all dependencies.
	mergedAconfigFiles map[string]android.Paths
}

type AndroidAppImportProperties struct {
	// A prebuilt apk to import
	Apk *string `android:"path"`

	// The name of a certificate in the default certificate directory or an android_app_certificate
	// module name in the form ":module". Should be empty if presigned or default_dev_cert is set.
	Certificate *string

	// Names of extra android_app_certificate modules to sign the apk with in the form ":module".
	Additional_certificates []string

	// Set this flag to true if the prebuilt apk is already signed. The certificate property must not
	// be set for presigned modules.
	Presigned *bool

	// Name of the signing certificate lineage file or filegroup module.
	Lineage *string `android:"path"`

	// For overriding the --rotation-min-sdk-version property of apksig
	RotationMinSdkVersion *string

	// Sign with the default system dev certificate. Must be used judiciously. Most imported apps
	// need to either specify a specific certificate or be presigned.
	Default_dev_cert *bool

	// Specifies that this app should be installed to the priv-app directory,
	// where the system will grant it additional privileges not available to
	// normal apps.
	Privileged *bool

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string

	// Optional name for the installed app. If unspecified, it is derived from the module name.
	Filename *string

	// If set, create package-export.apk, which other packages can
	// use to get PRODUCT-agnostic resource data like IDs and type definitions.
	Export_package_resources *bool

	// Optional. Install to a subdirectory of the default install path for the module
	Relative_install_path *string

	// Whether the prebuilt apk can be installed without additional processing. Default is false.
	Preprocessed *bool

	// Whether or not to skip checking the preprocessed apk for proper alignment and uncompressed
	// JNI libs and dex files. Default is false
	Skip_preprocessed_apk_checks *bool

	// Name of the source soong module that gets shadowed by this prebuilt
	// If unspecified, follows the naming convention that the source module of
	// the prebuilt is Name() without "prebuilt_" prefix
	Source_module_name *string
}

func (a *AndroidAppImport) IsInstallable() bool {
	return true
}

// Updates properties with variant-specific values.
// This happens as a DefaultableHook instead of a LoadHook because we want to run it after
// soong config variables are applied.
func (a *AndroidAppImport) processVariants(ctx android.DefaultableHookContext) {
	config := ctx.Config()
	dpiProps := reflect.ValueOf(a.dpiVariants).Elem().FieldByName(DpiGroupName)

	// Try DPI variant matches in the reverse-priority order so that the highest priority match
	// overwrites everything else.
	// TODO(jungjw): Can we optimize this by making it priority order?
	for i := len(config.ProductAAPTPrebuiltDPI()) - 1; i >= 0; i-- {
		MergePropertiesFromVariant(ctx, &a.properties, dpiProps, config.ProductAAPTPrebuiltDPI()[i])
	}
	if config.ProductAAPTPreferredConfig() != "" {
		MergePropertiesFromVariant(ctx, &a.properties, dpiProps, config.ProductAAPTPreferredConfig())
	}
	archProps := reflect.ValueOf(a.archVariants).Elem().FieldByName(ArchGroupName)
	archType := ctx.Config().AndroidFirstDeviceTarget.Arch.ArchType
	MergePropertiesFromVariant(ctx, &a.properties, archProps, archType.Name)

	// Process "arch" includes "dpi_variants"
	archStructPtr := reflect.ValueOf(a.arch_dpiVariants).Elem().FieldByName(ArchGroupName)
	if archStruct := archStructPtr.Elem(); archStruct.IsValid() {
		archPartPropsPtr := archStruct.FieldByName(proptools.FieldNameForProperty(archType.Name))
		if archPartProps := archPartPropsPtr.Elem(); archPartProps.IsValid() {
			archDpiPropsPtr := archPartProps.FieldByName(DpiGroupName)
			if archDpiProps := archDpiPropsPtr.Elem(); archDpiProps.IsValid() {
				for i := len(config.ProductAAPTPrebuiltDPI()) - 1; i >= 0; i-- {
					MergePropertiesFromVariant(ctx, &a.properties, archDpiProps, config.ProductAAPTPrebuiltDPI()[i])
				}
				if config.ProductAAPTPreferredConfig() != "" {
					MergePropertiesFromVariant(ctx, &a.properties, archDpiProps, config.ProductAAPTPreferredConfig())
				}
			}
		}
	}

	if String(a.properties.Apk) == "" {
		// Disable this module since the apk property is still empty after processing all matching
		// variants. This likely means there is no matching variant, and the default variant doesn't
		// have an apk property value either.
		a.Disable()
	}
}

func MergePropertiesFromVariant(ctx android.EarlyModuleContext,
	dst interface{}, variantGroup reflect.Value, variant string) {
	src := variantGroup.FieldByName(proptools.FieldNameForProperty(variant))
	if !src.IsValid() {
		return
	}

	err := proptools.ExtendMatchingProperties([]interface{}{dst}, src.Interface(), nil, proptools.OrderAppend)
	if err != nil {
		if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
			ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
		} else {
			panic(err)
		}
	}
}

func (a *AndroidAppImport) DepsMutator(ctx android.BottomUpMutatorContext) {
	cert := android.SrcIsModule(String(a.properties.Certificate))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
	}

	for _, cert := range a.properties.Additional_certificates {
		cert = android.SrcIsModule(cert)
		if cert != "" {
			ctx.AddDependency(ctx.Module(), certificateTag, cert)
		} else {
			ctx.PropertyErrorf("additional_certificates",
				`must be names of android_app_certificate modules in the form ":module"`)
		}
	}

	a.usesLibrary.deps(ctx, true)
}

func (a *AndroidAppImport) uncompressEmbeddedJniLibs(
	ctx android.ModuleContext, inputPath android.Path, outputPath android.OutputPath) {
	// Test apps don't need their JNI libraries stored uncompressed. As a matter of fact, messing
	// with them may invalidate pre-existing signature data.
	if ctx.InstallInTestcases() && (Bool(a.properties.Presigned) || Bool(a.properties.Preprocessed)) {
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.Cp,
			Output: outputPath,
			Input:  inputPath,
		})
		return
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:   uncompressEmbeddedJniLibsRule,
		Input:  inputPath,
		Output: outputPath,
	})
}

// Returns whether this module should have the dex file stored uncompressed in the APK.
func (a *AndroidAppImport) shouldUncompressDex(ctx android.ModuleContext) bool {
	if ctx.Config().UnbundledBuild() || proptools.Bool(a.properties.Preprocessed) {
		return false
	}

	// Uncompress dex in APKs of priv-apps if and only if DONT_UNCOMPRESS_PRIV_APPS_DEXS is false.
	if a.Privileged() {
		return ctx.Config().UncompressPrivAppDex()
	}

	return shouldUncompressDex(ctx, android.RemoveOptionalPrebuiltPrefix(ctx.ModuleName()), &a.dexpreopter)
}

func (a *AndroidAppImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.generateAndroidBuildActions(ctx)
}

func (a *AndroidAppImport) InstallApkName() string {
	return a.BaseModuleName()
}

func (a *AndroidAppImport) BaseModuleName() string {
	return proptools.StringDefault(a.properties.Source_module_name, a.ModuleBase.Name())
}

func (a *AndroidAppImport) generateAndroidBuildActions(ctx android.ModuleContext) {
	if a.Name() == "prebuilt_framework-res" {
		ctx.ModuleErrorf("prebuilt_framework-res found. This used to have special handling in soong, but was removed due to prebuilt_framework-res no longer existing. This check is to ensure it doesn't come back without readding the special handling.")
	}

	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	if !apexInfo.IsForPlatform() {
		a.hideApexVariantFromMake = true
	}

	if Bool(a.properties.Preprocessed) {
		if a.properties.Presigned != nil && !*a.properties.Presigned {
			ctx.ModuleErrorf("Setting preprocessed: true implies presigned: true, so you cannot set presigned to false")
		}
		t := true
		a.properties.Presigned = &t
	}

	numCertPropsSet := 0
	if String(a.properties.Certificate) != "" {
		numCertPropsSet++
	}
	if Bool(a.properties.Presigned) {
		numCertPropsSet++
	}
	if Bool(a.properties.Default_dev_cert) {
		numCertPropsSet++
	}
	if numCertPropsSet != 1 {
		ctx.ModuleErrorf("One and only one of certficate, presigned (implied by preprocessed), and default_dev_cert properties must be set")
	}

	// TODO: LOCAL_EXTRACT_APK/LOCAL_EXTRACT_DPI_APK
	// TODO: LOCAL_PACKAGE_SPLITS

	srcApk := a.prebuilt.SingleSourcePath(ctx)

	// TODO: Install or embed JNI libraries

	// Uncompress JNI libraries in the apk
	jnisUncompressed := android.PathForModuleOut(ctx, "jnis-uncompressed", ctx.ModuleName()+".apk")
	a.uncompressEmbeddedJniLibs(ctx, srcApk, jnisUncompressed.OutputPath)

	var pathFragments []string
	relInstallPath := String(a.properties.Relative_install_path)

	if Bool(a.properties.Privileged) {
		pathFragments = []string{"priv-app", relInstallPath, a.BaseModuleName()}
	} else if ctx.InstallInTestcases() {
		pathFragments = []string{relInstallPath, a.BaseModuleName(), ctx.DeviceConfig().DeviceArch()}
	} else {
		pathFragments = []string{"app", relInstallPath, a.BaseModuleName()}
	}

	installDir := android.PathForModuleInstall(ctx, pathFragments...)
	a.dexpreopter.isApp = true
	a.dexpreopter.installPath = installDir.Join(ctx, a.BaseModuleName()+".apk")
	a.dexpreopter.isPresignedPrebuilt = Bool(a.properties.Presigned)
	a.dexpreopter.uncompressedDex = a.shouldUncompressDex(ctx)

	a.dexpreopter.enforceUsesLibs = a.usesLibrary.enforceUsesLibraries()
	a.dexpreopter.classLoaderContexts = a.usesLibrary.classLoaderContextForUsesLibDeps(ctx)
	if a.usesLibrary.shouldDisableDexpreopt {
		a.dexpreopter.disableDexpreopt()
	}

	if a.usesLibrary.enforceUsesLibraries() {
		a.usesLibrary.verifyUsesLibrariesAPK(ctx, srcApk)
	}

	a.dexpreopter.dexpreopt(ctx, android.RemoveOptionalPrebuiltPrefix(ctx.ModuleName()), jnisUncompressed)
	if a.dexpreopter.uncompressedDex {
		dexUncompressed := android.PathForModuleOut(ctx, "dex-uncompressed", ctx.ModuleName()+".apk")
		ctx.Build(pctx, android.BuildParams{
			Rule:   uncompressDexRule,
			Input:  jnisUncompressed,
			Output: dexUncompressed,
		})
		jnisUncompressed = dexUncompressed
	}

	apkFilename := proptools.StringDefault(a.properties.Filename, a.BaseModuleName()+".apk")

	// TODO: Handle EXTERNAL

	// Sign or align the package if package has not been preprocessed

	if proptools.Bool(a.properties.Preprocessed) {
		validationStamp := a.validatePresignedApk(ctx, srcApk)
		output := android.PathForModuleOut(ctx, apkFilename)
		ctx.Build(pctx, android.BuildParams{
			Rule:       android.Cp,
			Input:      srcApk,
			Output:     output,
			Validation: validationStamp,
		})
		a.outputFile = output
		a.certificate = PresignedCertificate
	} else if !Bool(a.properties.Presigned) {
		// If the certificate property is empty at this point, default_dev_cert must be set to true.
		// Which makes processMainCert's behavior for the empty cert string WAI.
		_, _, certificates := collectAppDeps(ctx, a, false, false)
		a.certificate, certificates = processMainCert(a.ModuleBase, String(a.properties.Certificate), certificates, ctx)
		signed := android.PathForModuleOut(ctx, "signed", apkFilename)
		var lineageFile android.Path
		if lineage := String(a.properties.Lineage); lineage != "" {
			lineageFile = android.PathForModuleSrc(ctx, lineage)
		}

		rotationMinSdkVersion := String(a.properties.RotationMinSdkVersion)

		SignAppPackage(ctx, signed, jnisUncompressed, certificates, nil, lineageFile, rotationMinSdkVersion)
		a.outputFile = signed
	} else {
		validationStamp := a.validatePresignedApk(ctx, srcApk)
		alignedApk := android.PathForModuleOut(ctx, "zip-aligned", apkFilename)
		TransformZipAlign(ctx, alignedApk, jnisUncompressed, []android.Path{validationStamp})
		a.outputFile = alignedApk
		a.certificate = PresignedCertificate
	}

	// TODO: Optionally compress the output apk.

	if apexInfo.IsForPlatform() {
		a.installPath = ctx.InstallFile(installDir, apkFilename, a.outputFile)
		artifactPath := android.PathForModuleSrc(ctx, *a.properties.Apk)
		a.provenanceMetaDataFile = provenance.GenerateArtifactProvenanceMetaData(ctx, artifactPath, a.installPath)
	}
	android.CollectDependencyAconfigFiles(ctx, &a.mergedAconfigFiles)

	// TODO: androidmk converter jni libs
}

func (a *AndroidAppImport) validatePresignedApk(ctx android.ModuleContext, srcApk android.Path) android.Path {
	stamp := android.PathForModuleOut(ctx, "validated-prebuilt", "check.stamp")
	var extraArgs []string
	if a.Privileged() {
		extraArgs = append(extraArgs, "--privileged")
	}
	if proptools.Bool(a.properties.Skip_preprocessed_apk_checks) {
		extraArgs = append(extraArgs, "--skip-preprocessed-apk-checks")
	}
	if proptools.Bool(a.properties.Preprocessed) {
		extraArgs = append(extraArgs, "--preprocessed")
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:   checkPresignedApkRule,
		Input:  srcApk,
		Output: stamp,
		Args: map[string]string{
			"extraArgs": strings.Join(extraArgs, " "),
		},
	})
	return stamp
}

func (a *AndroidAppImport) Prebuilt() *android.Prebuilt {
	return &a.prebuilt
}

func (a *AndroidAppImport) Name() string {
	return a.prebuilt.Name(a.ModuleBase.Name())
}

func (a *AndroidAppImport) OutputFile() android.Path {
	return a.outputFile
}

func (a *AndroidAppImport) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return []android.Path{a.outputFile}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (a *AndroidAppImport) JacocoReportClassesFile() android.Path {
	return nil
}

func (a *AndroidAppImport) Certificate() Certificate {
	return a.certificate
}

func (a *AndroidAppImport) ProvenanceMetaDataFile() android.OutputPath {
	return a.provenanceMetaDataFile
}

func (a *AndroidAppImport) PrivAppAllowlist() android.OptionalPath {
	return android.OptionalPath{}
}

const (
	ArchGroupName = "Arch"
	DpiGroupName  = "Dpi_variants"
)

var dpiVariantGroupType reflect.Type
var archVariantGroupType reflect.Type
var archdpiVariantGroupType reflect.Type
var supportedDpis = []string{"ldpi", "mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"}

func initAndroidAppImportVariantGroupTypes() {
	dpiVariantGroupType = createVariantGroupType(supportedDpis, DpiGroupName)

	archNames := make([]string, len(android.ArchTypeList()))
	for i, archType := range android.ArchTypeList() {
		archNames[i] = archType.Name
	}
	archVariantGroupType = createVariantGroupType(archNames, ArchGroupName)
	archdpiVariantGroupType = createArchDpiVariantGroupType(archNames, supportedDpis)
}

// Populates all variant struct properties at creation time.
func (a *AndroidAppImport) populateAllVariantStructs() {
	a.dpiVariants = reflect.New(dpiVariantGroupType).Interface()
	a.AddProperties(a.dpiVariants)

	a.archVariants = reflect.New(archVariantGroupType).Interface()
	a.AddProperties(a.archVariants)

	a.arch_dpiVariants = reflect.New(archdpiVariantGroupType).Interface()
	a.AddProperties(a.arch_dpiVariants)
}

func (a *AndroidAppImport) Privileged() bool {
	return Bool(a.properties.Privileged)
}

func (a *AndroidAppImport) DepIsInSameApex(_ android.BaseModuleContext, _ android.Module) bool {
	// android_app_import might have extra dependencies via uses_libs property.
	// Don't track the dependency as we don't automatically add those libraries
	// to the classpath. It should be explicitly added to java_libs property of APEX
	return false
}

func (a *AndroidAppImport) SdkVersion(ctx android.EarlyModuleContext) android.SdkSpec {
	return android.SdkSpecPrivate
}

func (a *AndroidAppImport) MinSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel {
	return android.SdkSpecPrivate.ApiLevel
}

func (a *AndroidAppImport) LintDepSets() LintDepSets {
	return LintDepSets{}
}

var _ android.ApexModule = (*AndroidAppImport)(nil)

// Implements android.ApexModule
func (j *AndroidAppImport) ShouldSupportSdkVersion(ctx android.BaseModuleContext,
	sdkVersion android.ApiLevel) error {
	// Do not check for prebuilts against the min_sdk_version of enclosing APEX
	return nil
}

func createVariantGroupType(variants []string, variantGroupName string) reflect.Type {
	props := reflect.TypeOf((*AndroidAppImportProperties)(nil))

	variantFields := make([]reflect.StructField, len(variants))
	for i, variant := range variants {
		variantFields[i] = reflect.StructField{
			Name: proptools.FieldNameForProperty(variant),
			Type: props,
		}
	}

	variantGroupStruct := reflect.StructOf(variantFields)
	return reflect.StructOf([]reflect.StructField{
		{
			Name: variantGroupName,
			Type: variantGroupStruct,
		},
	})
}

func createArchDpiVariantGroupType(archNames []string, dpiNames []string) reflect.Type {
	props := reflect.TypeOf((*AndroidAppImportProperties)(nil))

	dpiVariantFields := make([]reflect.StructField, len(dpiNames))
	for i, variant_dpi := range dpiNames {
		dpiVariantFields[i] = reflect.StructField{
			Name: proptools.FieldNameForProperty(variant_dpi),
			Type: props,
		}
	}
	dpiVariantGroupStruct := reflect.StructOf(dpiVariantFields)
	dpi_struct := reflect.StructOf([]reflect.StructField{
		{
			Name: DpiGroupName,
			Type: reflect.PointerTo(dpiVariantGroupStruct),
		},
	})

	archVariantFields := make([]reflect.StructField, len(archNames))
	for i, variant_arch := range archNames {
		archVariantFields[i] = reflect.StructField{
			Name: proptools.FieldNameForProperty(variant_arch),
			Type: reflect.PointerTo(dpi_struct),
		}
	}
	archVariantGroupStruct := reflect.StructOf(archVariantFields)

	return_struct := reflect.StructOf([]reflect.StructField{
		{
			Name: ArchGroupName,
			Type: reflect.PointerTo(archVariantGroupStruct),
		},
	})
	return return_struct
}

// android_app_import imports a prebuilt apk with additional processing specified in the module.
// DPI-specific apk source files can be specified using dpi_variants. Example:
//
//	android_app_import {
//	    name: "example_import",
//	    apk: "prebuilts/example.apk",
//	    dpi_variants: {
//	        mdpi: {
//	            apk: "prebuilts/example_mdpi.apk",
//	        },
//	        xhdpi: {
//	            apk: "prebuilts/example_xhdpi.apk",
//	        },
//	    },
//	    presigned: true,
//	}
func AndroidAppImportFactory() android.Module {
	module := &AndroidAppImport{}
	module.AddProperties(&module.properties)
	module.AddProperties(&module.dexpreoptProperties)
	module.AddProperties(&module.usesLibrary.usesLibraryProperties)
	module.populateAllVariantStructs()
	module.SetDefaultableHook(func(ctx android.DefaultableHookContext) {
		module.processVariants(ctx)
	})

	android.InitApexModule(module)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitSingleSourcePrebuiltModule(module, &module.properties, "Apk")

	module.usesLibrary.enforce = true

	return module
}

type AndroidTestImport struct {
	AndroidAppImport

	testProperties struct {
		// list of compatibility suites (for example "cts", "vts") that the module should be
		// installed into.
		Test_suites []string `android:"arch_variant"`

		// list of files or filegroup modules that provide data that should be installed alongside
		// the test
		Data []string `android:"path"`

		// Install the test into a folder named for the module in all test suites.
		Per_testcase_directory *bool
	}

	data android.Paths
}

func (a *AndroidTestImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.generateAndroidBuildActions(ctx)

	a.data = android.PathsForModuleSrc(ctx, a.testProperties.Data)
}

func (a *AndroidTestImport) InstallInTestcases() bool {
	return true
}

// android_test_import imports a prebuilt test apk with additional processing specified in the
// module. DPI or arch variant configurations can be made as with android_app_import.
func AndroidTestImportFactory() android.Module {
	module := &AndroidTestImport{}
	module.AddProperties(&module.properties)
	module.AddProperties(&module.dexpreoptProperties)
	module.AddProperties(&module.testProperties)
	module.populateAllVariantStructs()
	module.SetDefaultableHook(func(ctx android.DefaultableHookContext) {
		module.processVariants(ctx)
	})

	module.dexpreopter.isTest = true

	android.InitApexModule(module)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitSingleSourcePrebuiltModule(module, &module.properties, "Apk")

	return module
}
