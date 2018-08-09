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

package java

// This file contains the module implementations for android_app, android_test, and some more
// related module types, including their override variants.

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/depset"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/dexpreopt"
	"android/soong/tradefed"
)

//go:generate go run ../../blueprint/gobtools/codegen

func init() {
	RegisterAppBuildComponents(android.InitRegistrationContext)
	pctx.HostBinToolVariable("ModifyAllowlistCmd", "modify_permissions_allowlist")
	pctx.HostBinToolVariable("ModifyPreinstallAllowlistCmd", "modify_preinstall_allowlist")
}

var (
	modifyAllowlist = pctx.AndroidStaticRule("modifyAllowlist",
		blueprint.RuleParams{
			Command:         "${ModifyAllowlistCmd} $in $packageName $out",
			CommandDeps:     []string{"${ModifyAllowlistCmd}"},
			SandboxDisabled: true,
		}, "packageName")

	modifyPreinstallAllowlist = pctx.AndroidStaticRule("modifyPreinstallAllowlist",
		blueprint.RuleParams{
			Command:         "${ModifyPreinstallAllowlistCmd} $in $packageName $out",
			CommandDeps:     []string{"${ModifyPreinstallAllowlistCmd}"},
			SandboxDisabled: true,
		}, "packageName")
)

// @auto-generate: gob
type FlagsPackages struct {
	// Paths to the aconfig dump output text files that are consumed by aapt2
	AconfigTextFiles android.Paths
}

var FlagsPackagesProvider = blueprint.NewProvider[FlagsPackages]()

func RegisterAppBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_app", AndroidAppFactory)
	ctx.RegisterModuleType("android_test", AndroidTestFactory)
	ctx.RegisterModuleType("android_test_helper_app", AndroidTestHelperAppFactory)
	ctx.RegisterModuleType("android_app_certificate", AndroidAppCertificateFactory)
	ctx.RegisterModuleType("override_android_app", OverrideAndroidAppModuleFactory)
	ctx.RegisterModuleType("override_android_test", OverrideAndroidTestModuleFactory)
}

// @auto-generate: gob
type AppInfo struct {
	// Updatable is set to the value of the updatable property
	Updatable bool

	// TestHelperApp is true if the module is a android_test_helper_app
	TestHelperApp bool

	// EmbeddedJNILibs is the list of paths to JNI libraries that were embedded in the APK.
	EmbeddedJNILibs android.Paths

	MergedManifestFile android.Path

	Prebuilt                      bool
	AppSet                        bool
	Privileged                    bool
	OutputFile                    android.Path
	InstallApkName                string
	JacocoInfo                    JacocoInfo
	Certificate                   Certificate
	PrivAppAllowlist              android.OptionalPath
	PreinstallAllowlist           android.OptionalPath
	OverriddenManifestPackageName *string
	ApkCertsFile                  android.Path
	JniLibs                       []jniLib
	JniCoverageOutputs            android.Paths
	PackedAdditionalOutputs       android.Path
}

var AppInfoProvider = blueprint.NewProvider[*AppInfo]()

type AppInfos []AppInfo

// AndroidManifest.xml merging
// package splits

type appProperties struct {
	// Names of extra android_app_certificate modules to sign the apk with in the form ":module".
	Additional_certificates []string

	// If set, create package-export.apk, which other packages can
	// use to get PRODUCT-agnostic resource data like IDs and type definitions.
	Export_package_resources *bool

	// If set, aapt2 will emit a file containing a list of resource type names and their ID
	// mappings, which can be used by `--stable-ids` flag in `aapt2 link` command to assign
	// fixed IDs for resources. The output file will be named res-ids.txt.
	Emit_res_ids *bool

	// Specifies that this app should be installed to the priv-app directory,
	// where the system will grant it additional privileges not available to
	// normal apps.
	Privileged *bool

	// list of resource labels to generate individual resource packages
	Package_splits []string

	// list of native libraries that will be provided in or alongside the resulting jar
	Jni_libs proptools.Configurable[[]string] `android:"arch_variant"`

	// if true, use JNI libraries that link against platform APIs even if this module sets
	// sdk_version.
	Jni_uses_platform_apis *bool

	// if true, use JNI libraries that link against SDK APIs even if this module does not set
	// sdk_version.
	Jni_uses_sdk_apis *bool

	// STL library to use for JNI libraries.
	Stl *string `android:"arch_variant"`

	// Store native libraries uncompressed in the APK and set the android:extractNativeLibs="false" manifest
	// flag so that they are used from inside the APK at runtime.  Defaults to true for android_test modules unless
	// sdk_version or min_sdk_version is set to a version that doesn't support it (<23), defaults to true for
	// android_app modules that are embedded to APEXes, defaults to false for other module types where the native
	// libraries are generally preinstalled outside the APK.
	Use_embedded_native_libs *bool

	// Store dex files uncompressed in the APK and set the android:useEmbeddedDex="true" manifest attribute so that
	// they are used from inside the APK at runtime.
	Use_embedded_dex *bool

	// Forces native libraries to always be packaged into the APK,
	// Use_embedded_native_libs still selects whether they are stored uncompressed and aligned or compressed.
	// True for android_test* modules.
	AlwaysPackageNativeLibs bool `blueprint:"mutated"`

	// If set, find and merge all NOTICE files that this module and its dependencies have and store
	// it in the APK as an asset.
	Embed_notices *bool

	// It can be set to test the behaviour of default target sdk version.
	// Only required when updatable: false. It is an error if updatable: true and this is false.
	Enforce_default_target_sdk_version *bool

	// If set, the targetSdkVersion for the target is set to the latest default API level.
	// This would be by default false, unless updatable: true or
	// enforce_default_target_sdk_version: true in which case this defaults to true.
	EnforceDefaultTargetSdkVersion bool `blueprint:"mutated"`

	// Whether this app is considered mainline updatable or not. When set to true, this will enforce
	// additional rules to make sure an app can safely be updated. Default is false.
	// Prefer using other specific properties if build behaviour must be changed; avoid using this
	// flag for anything but neverallow rules (unless the behaviour change is invisible to owners).
	Updatable *bool

	// Specifies the file that contains the allowlist for this app.
	Privapp_allowlist *string `android:"path"`

	// Specifies the file that contains the preinstall allowlist for this app. To be used by apps in apexes.
	Preinstall_allowlist *string `android:"path"`

	// If set, create an RRO package which contains only resources having PRODUCT_CHARACTERISTICS
	// and install the RRO package to /product partition, instead of passing --product argument
	// to aapt2. Default is false.
	// Setting this will make this APK identical to all targets, regardless of
	// PRODUCT_CHARACTERISTICS.
	Generate_product_characteristics_rro *bool

	ProductCharacteristicsRROPackageName        *string `blueprint:"mutated"`
	ProductCharacteristicsRROManifestModuleName *string `blueprint:"mutated"`
}

// android_app properties that can be overridden by override_android_app
type overridableAppProperties struct {
	// The name of a certificate in the default certificate directory, blank to use the default product certificate,
	// or an android_app_certificate module name in the form ":module".
	Certificate proptools.Configurable[string] `android:"replace_instead_of_append"`

	// Name of the signing certificate lineage file or filegroup module.
	Lineage *string `android:"path"`

	// For overriding the --rotation-min-sdk-version property of apksig
	RotationMinSdkVersion *string

	// the package name of this app. The package name in the manifest file is used if one was not given.
	Package_name proptools.Configurable[string]

	// the logging parent of this app.
	Logging_parent *string

	// Whether to rename the package in resources to the override name rather than the base name. Defaults to true.
	Rename_resources_package *bool

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string
}

type AndroidApp struct {
	Library
	aapt
	android.OverridableModuleBase

	certificate Certificate

	appProperties appProperties

	overridableAppProperties overridableAppProperties

	jniLibs                  []jniLib
	installPathForJNISymbols android.Path
	embeddedJniLibs          bool
	jniCoverageOutputs       android.Paths

	bundleFile android.Path

	// the install APK name is normally the same as the module name, but can be overridden with PRODUCT_PACKAGE_NAME_OVERRIDES.
	installApkName string

	installDir android.InstallPath

	onDeviceDir string

	additionalAaptFlags []string

	overriddenManifestPackageName string

	android.ApexBundleDepsInfo

	privAppAllowlist android.OptionalPath

	preinstallAllowlist android.OptionalPath

	requiredModuleNames []string
}

func (a *AndroidApp) IsInstallable() bool {
	return Bool(a.properties.Installable)
}

func (a *AndroidApp) ResourcesNodeDepSet() depset.DepSet[resourcesNode] {
	return a.aapt.resourcesNodesDepSet
}

func (a *AndroidApp) OutputFile() android.Path {
	return a.outputFile
}

func (a *AndroidApp) Certificate() Certificate {
	return a.certificate
}

func (a *AndroidApp) JniCoverageOutputs() android.Paths {
	return a.jniCoverageOutputs
}

func (a *AndroidApp) PrivAppAllowlist() android.OptionalPath {
	return a.privAppAllowlist
}

func (a *AndroidApp) PreinstallAllowlist() android.OptionalPath {
	return a.preinstallAllowlist
}

var _ AndroidLibraryDependency = (*AndroidApp)(nil)

// @auto-generate: gob
type Certificate struct {
	Pem, Key  android.Path
	presigned bool
}

var PresignedCertificate = Certificate{presigned: true}

func (c Certificate) AndroidMkString() string {
	if c.presigned {
		return "PRESIGNED"
	} else {
		return c.Pem.String()
	}
}

func (a *AndroidApp) DepsMutator(ctx android.BottomUpMutatorContext) {
	if String(a.appProperties.Stl) == "c++_shared" && !a.SdkVersion(ctx).Specified() {
		ctx.PropertyErrorf("stl", "sdk_version must be set in order to use c++_shared")
	}

	sdkDep := decodeSdkDep(ctx, android.SdkContext(a))
	a.usesLibrary.deps(ctx, sdkDep.hasFrameworkLibs())
	a.Module.deps(ctx)
	if sdkDep.hasFrameworkLibs() {
		a.aapt.deps(ctx, sdkDep)
	}

	usesSDK := a.SdkVersion(ctx).Specified() && a.SdkVersion(ctx).Kind != android.SdkCorePlatform

	if usesSDK && Bool(a.appProperties.Jni_uses_sdk_apis) {
		ctx.PropertyErrorf("jni_uses_sdk_apis",
			"can only be set for modules that do not set sdk_version")
	} else if !usesSDK && Bool(a.appProperties.Jni_uses_platform_apis) {
		ctx.PropertyErrorf("jni_uses_platform_apis",
			"can only be set for modules that set sdk_version")
	}

	for _, jniTarget := range ctx.MultiTargets() {
		variation := append(jniTarget.Variations(),
			blueprint.Variation{Mutator: "link", Variation: "shared"})

		// Test whether to use the SDK variant or the non-SDK variant of JNI dependencies.
		// Many factors are considered here.
		// 1. Basically, the selection follows whether the app has sdk_version set or not.
		jniUsesSdkVariant := usesSDK
		// 2. However, jni_uses_platform_apis and jni_uses_sdk_apis can override it
		if Bool(a.appProperties.Jni_uses_sdk_apis) {
			jniUsesSdkVariant = true
		}
		if Bool(a.appProperties.Jni_uses_platform_apis) {
			jniUsesSdkVariant = false
		}
		// 3. Then the use of SDK variant is again prohibited for the following cases:
		// 3.1. the app is shipped on unbundled partitions like vendor. Since the entire
		// partition (not only the app) is considered unbudled, there's no need to use the
		// SDK variant.
		// 3.2. the app doesn't support embedding the JNI libs
		if a.RequiresStableAPIs(ctx) || !a.shouldEmbedJnis(ctx) {
			jniUsesSdkVariant = false
		}
		if jniUsesSdkVariant {
			variation = append(variation, blueprint.Variation{Mutator: "sdk", Variation: "sdk"})
		}

		// Use the installable dep tag when the JNIs are not embedded
		var tag dependencyTag
		if a.shouldEmbedJnis(ctx) {
			tag = jniLibTag
		} else {
			tag = jniInstallTag
		}
		ctx.AddFarVariationDependencies(variation, tag, a.appProperties.Jni_libs.GetOrDefault(ctx, nil)...)
	}
	for _, aconfig_declaration := range a.aaptProperties.Flags_packages {
		ctx.AddDependency(ctx.Module(), aconfigDeclarationTag, aconfig_declaration)
	}

	// Only require APEX symbol tools if we are doing a full tree build,
	// or if this app is explicitly requested in an unbundled build.
	if !ctx.Config().UnbundledBuild() || slices.Contains(ctx.Config().UnbundledBuildApps(), a.Name()) {
		ctx.AddHostToolDependencies("gen_apex_symbols", "dexdeps")
	}
}

func (a *AndroidApp) OverridablePropertiesDepsMutator(ctx android.BottomUpMutatorContext) {
	cert := android.SrcIsModule(a.getCertString(ctx))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
	}

	if preinstallAllowlist := a.appProperties.Preinstall_allowlist; preinstallAllowlist != nil {
		if moduleName := android.SrcIsModule(*preinstallAllowlist); moduleName != "" {
			ctx.AddDependency(ctx.Module(), nil, moduleName)
		}
	}

	if a.appProperties.Privapp_allowlist != nil && !Bool(a.appProperties.Privileged) {
		// There are a few uids that are explicitly considered privileged regardless of their
		// app's location. Bluetooth is one such app. It should arguably be moved to priv-app,
		// but for now, allow it not to be in priv-app.
		privilegedBecauseOfUid := ctx.ModuleName() == "Bluetooth"
		if !privilegedBecauseOfUid {
			ctx.PropertyErrorf("privapp_allowlist", "privileged must be set in order to use privapp_allowlist (with a few exceptions)")
		}
	}

	for _, cert := range a.appProperties.Additional_certificates {
		cert = android.SrcIsModule(cert)
		if cert != "" {
			ctx.AddDependency(ctx.Module(), certificateTag, cert)
		} else {
			ctx.PropertyErrorf("additional_certificates",
				`must be names of android_app_certificate modules in the form ":module"`)
		}
	}
}

// TODO(b/156476221): Remove this allowlist
var (
	missingMinSdkVersionMtsAllowlist = []string{
		"CellBroadcastReceiverGoogleUnitTests",
		"CellBroadcastReceiverUnitTests",
		"CtsBatterySavingTestCases",
		"CtsDeviceAndProfileOwnerApp23",
		"CtsDeviceAndProfileOwnerApp30",
		"CtsIntentSenderApp",
		"CtsJobSchedulerTestCases",
		"CtsMimeMapTestCases",
		"CtsTareTestCases",
		"LibStatsPullTests",
		"MediaProviderClientTests",
		"TeleServiceTests",
		"TestExternalImsServiceApp",
		"TestSmsRetrieverApp",
		"TetheringPrivilegedTests",
	}
)

func checkMinSdkVersionMts(ctx android.ModuleContext, minSdkVersion android.ApiLevel) {
	if includedInMts(ctx.Module()) && !minSdkVersion.Specified() && !android.InList(ctx.ModuleName(), missingMinSdkVersionMtsAllowlist) {
		ctx.PropertyErrorf("min_sdk_version", "min_sdk_version is a required property for tests included in MTS")
	}
}

func (a *AndroidTestHelperApp) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	checkMinSdkVersionMts(ctx, a.MinSdkVersion(ctx))
	applicationId := a.appTestHelperAppProperties.Manifest_values.ApplicationId
	if applicationId != nil {
		packageName := a.overridableAppProperties.Package_name.Get(ctx)
		if packageName.IsPresent() {
			ctx.PropertyErrorf("manifest_values.applicationId", "property is not supported when property package_name is set.")
		}
		a.aapt.manifestValues.applicationId = *applicationId
	}

	if Bool(a.appTestHelperAppProperties.Enable_static_aconfig_pb) {
		addStaticAconfigProto(ctx, &a.extraResources)
	}
	a.generateAndroidBuildActions(ctx)
	if ctx.Failed() {
		return
	}
	ctx.SetTestModuleInfo(&android.TestModuleInformation{
		TestOnly: true,
	})
	appInfo := &AppInfo{
		Updatable:          Bool(a.appProperties.Updatable),
		TestHelperApp:      true,
		JniLibs:            a.jniLibs,
		JniCoverageOutputs: a.jniCoverageOutputs,
	}
	setCommonAppInfo(appInfo, a)
	android.SetProvider(ctx, AppInfoProvider, appInfo)
	android.SetProvider(ctx, ApkCertInfoProvider, ApkCertInfo{
		Certificate: appInfo.Certificate,
		Name:        appInfo.InstallApkName + ".apk",
		Test:        true,
	})

	moduleInfoJSON := ctx.ModuleInfoJSON()
	moduleInfoJSON.Tags = append(moduleInfoJSON.Tags, "tests")
	if len(a.appTestHelperAppProperties.Test_suites) > 0 {
		moduleInfoJSON.CompatibilitySuites = append(moduleInfoJSON.CompatibilitySuites, a.appTestHelperAppProperties.Test_suites...)
	} else {
		moduleInfoJSON.CompatibilitySuites = append(moduleInfoJSON.CompatibilitySuites, "null-suite")
	}

	var data []android.DataPath
	for _, d := range a.extraOutputFiles {
		data = append(data, android.DataPath{SrcPath: d})
	}
	ctx.SetTestSuiteInfo(android.TestSuiteInfo{
		TestSuites:           a.appTestHelperAppProperties.Test_suites,
		MainFile:             a.outputFile,
		MainFileStem:         a.installApkName,
		MainFileExt:          ".apk",
		NeedsArchFolder:      true,
		NonArchData:          data,
		PerTestcaseDirectory: proptools.Bool(a.appTestHelperAppProperties.Per_testcase_directory),
		DisableTestConfig:    true,
	})
}

func (a *AndroidApp) baseSymbolInfo(ctx android.ModuleContext) *cc.SymbolInfo {
	return &cc.SymbolInfo{
		Name:          a.BaseModuleName(),
		ModuleDir:     ctx.ModuleDir(),
		Uninstallable: a.IsSkipInstall() || !proptools.BoolDefault(a.properties.Installable, true) || a.NoFullInstall(),
	}
}

func (a *AndroidApp) GetJniSymbolInfos(ctx android.ModuleContext, JniSymbolInstallPath android.Path) []*cc.SymbolInfo {
	infos := []*cc.SymbolInfo{}
	for _, install := range JNISymbolsInstalls(a.jniLibs, JniSymbolInstallPath.String()) {
		info := a.baseSymbolInfo(ctx)
		info.UnstrippedBinaryPath = install.From
		info.ModuleDir = filepath.Dir(install.To)
		info.InstalledStem = filepath.Base(install.To)

		infos = append(infos, info)
	}
	return infos
}

func (a *AndroidApp) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.checkAppSdkVersions(ctx)
	a.checkEmbedJnis(ctx)
	a.generateAndroidBuildActions(ctx)
	if ctx.Failed() {
		return
	}
	a.generateJavaUsedByApex(ctx)

	var embeddedJniLibs []android.Path

	if a.embeddedJniLibs {
		for _, jni := range a.jniLibs {
			embeddedJniLibs = append(embeddedJniLibs, jni.path)
		}
	}
	overriddenName := a.OverriddenManifestPackageName()
	appInfo := &AppInfo{
		Updatable:                     Bool(a.appProperties.Updatable),
		TestHelperApp:                 false,
		EmbeddedJNILibs:               embeddedJniLibs,
		MergedManifestFile:            a.mergedManifest,
		OverriddenManifestPackageName: &overriddenName,
		JniLibs:                       a.jniLibs,
		JniCoverageOutputs:            a.jniCoverageOutputs,
	}
	setCommonAppInfo(appInfo, a)
	android.SetProvider(ctx, AppInfoProvider, appInfo)
	android.SetProvider(ctx, ApkCertInfoProvider, ApkCertInfo{
		Certificate: appInfo.Certificate,
		Name:        appInfo.InstallApkName + ".apk",
	})

	a.requiredModuleNames = a.getRequiredModuleNames(ctx)

	if a.dexer.proguardDictionary.Valid() {
		android.SetProvider(ctx, ProguardProvider, ProguardInfos{{
			ModuleName:         android.ModuleNameWithPossibleOverride(ctx),
			Class:              "APPS",
			ProguardDictionary: a.dexer.proguardDictionary.Path(),
			ProguardUsageZip:   a.dexer.proguardUsageZip.Path(),
			ClassesJar:         a.implementationAndResourcesJar,
		}})
	}
}

func (a *AndroidApp) getRequiredModuleNames(ctx android.ModuleContext) []string {
	var required []string
	if proptools.Bool(a.appProperties.Generate_product_characteristics_rro) {
		required = []string{a.productCharacteristicsRROPackageName()}
	}
	// Install the vendor overlay variant if this app is installed.
	if len(filterRRO(a.rroDirsDepSet, device)) > 0 {
		required = append(required, AutogeneratedRroModuleName(ctx, ctx.Module().Name(), "vendor"))
	}
	// Install the product overlay variant if this app is installed.
	if len(filterRRO(a.rroDirsDepSet, product)) > 0 {
		required = append(required, AutogeneratedRroModuleName(ctx, ctx.Module().Name(), "product"))
	}
	return required
}

func (a *AndroidApp) checkAppSdkVersions(ctx android.ModuleContext) {
	if a.Updatable() {
		if !a.SdkVersion(ctx).Stable() {
			ctx.PropertyErrorf("sdk_version", "Updatable apps must use stable SDKs, found %v", a.SdkVersion(ctx))
		}
		if a.overridableProperties.Min_sdk_version.GetOrDefault(ctx, "") == "" {
			ctx.PropertyErrorf("updatable", "updatable apps must set min_sdk_version.")
		}

		if minSdkVersion, err := a.MinSdkVersion(ctx).EffectiveVersion(ctx); err == nil {
			a.checkJniLibsSdkVersion(ctx, minSdkVersion)
			android.CheckMinSdkVersion(ctx, minSdkVersion, a.WalkPayloadDeps)
		} else {
			ctx.PropertyErrorf("min_sdk_version", "%s", err.Error())
		}

		if !BoolDefault(a.appProperties.Enforce_default_target_sdk_version, true) {
			ctx.PropertyErrorf("enforce_default_target_sdk_version", "Updatable apps must enforce default target sdk version")
		}
		// TODO(b/227460469) after all the modules removes the target sdk version, throw an error if the target sdk version is explicitly set.
		if a.deviceProperties.Target_sdk_version == nil {
			a.SetEnforceDefaultTargetSdkVersion(true)
		}
	}

	a.checkPlatformAPI(ctx)
	a.checkSdkVersions(ctx)
}

// Ensures that use_embedded_native_libs are set for apk-in-apex
func (a *AndroidApp) checkEmbedJnis(ctx android.BaseModuleContext) {
	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	apkInApex := !apexInfo.IsForPlatform()
	hasJnis := len(a.appProperties.Jni_libs.GetOrDefault(ctx, nil)) > 0

	if apkInApex && hasJnis && !Bool(a.appProperties.Use_embedded_native_libs) {
		ctx.ModuleErrorf("APK in APEX should have use_embedded_native_libs: true")
	}
}

// If an updatable APK sets min_sdk_version, min_sdk_vesion of JNI libs should match with it.
// This check is enforced for "updatable" APKs (including APK-in-APEX).
func (a *AndroidApp) checkJniLibsSdkVersion(ctx android.ModuleContext, minSdkVersion android.ApiLevel) {
	// It's enough to check direct JNI deps' sdk_version because all transitive deps from JNI deps are checked in cc.checkLinkType()
	ctx.VisitDirectDepsProxy(func(m android.ModuleProxy) {
		if !IsJniDepTag(ctx.OtherModuleDependencyTag(m)) {
			return
		}
		if _, ok := android.OtherModuleProvider(ctx, m, cc.CcInfoProvider); !ok {
			panic(fmt.Errorf("jni dependency is not a cc module: %v", m))
		}
		commonInfo, ok := android.OtherModuleProvider(ctx, m, android.CommonModuleInfoProvider)
		if !ok {
			panic(fmt.Errorf("jni dependency doesn't have CommonModuleInfo provider: %v", m))
		}
		// The domain of cc.sdk_version is "current" and <number>
		// We can rely on android.SdkSpec to convert it to <number> so that "current" is
		// handled properly regardless of sdk finalization.
		ver := ""
		if !commonInfo.MinSdkVersion.IsPlatform {
			ver = commonInfo.MinSdkVersion.ApiLevel.String()
		}
		jniSdkVersion, err := android.SdkSpecFrom(ctx, ver).EffectiveVersion(ctx)
		if err != nil || minSdkVersion.LessThan(jniSdkVersion) {
			ctx.OtherModuleErrorf(m, "min_sdk_version(%v) is higher than min_sdk_version(%v) of the containing android_app(%v)",
				ver, minSdkVersion, ctx.ModuleName())
			return
		}

	})
}

// Returns true if the native libraries should be stored in the APK uncompressed and the
// extractNativeLibs application flag should be set to false in the manifest.
func (a *AndroidApp) useEmbeddedNativeLibs(ctx android.ModuleContext) bool {
	minSdkVersion, err := a.MinSdkVersion(ctx).EffectiveVersion(ctx)
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "invalid value %q: %s", a.MinSdkVersion(ctx), err)
	}

	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	return (minSdkVersion.FinalOrFutureInt() >= 23 && Bool(a.appProperties.Use_embedded_native_libs)) ||
		!apexInfo.IsForPlatform()
}

// Returns whether this module should have the dex file stored uncompressed in the APK.
func (a *AndroidApp) shouldUncompressDex(ctx android.ModuleContext) bool {
	if Bool(a.appProperties.Use_embedded_dex) {
		return true
	}

	// Uncompress dex in APKs of privileged apps (even for unbundled builds, they may
	// be preinstalled as prebuilts).
	if ctx.Config().UncompressPrivAppDex() && a.Privileged() {
		return true
	}

	if ctx.Config().UnbundledBuild() {
		return false
	}

	return shouldUncompressDex(ctx, android.RemoveOptionalPrebuiltPrefix(ctx.ModuleName()), &a.dexpreopter)
}

func (a *AndroidApp) shouldEmbedJnis(ctx android.BaseModuleContext) bool {
	return ctx.Config().UnbundledBuild() || Bool(a.appProperties.Use_embedded_native_libs) ||
		Bool(a.appProperties.Updatable) ||
		a.appProperties.AlwaysPackageNativeLibs
}

func generateAaptRenamePackageFlags(packageName string, renameResourcesPackage bool) []string {
	aaptFlags := []string{"--rename-manifest-package " + packageName}
	if renameResourcesPackage {
		// Required to rename the package name in the resources table.
		aaptFlags = append(aaptFlags, "--rename-resources-package "+packageName)
	}
	return aaptFlags
}

func (a *AndroidApp) OverriddenManifestPackageName() string {
	return a.overriddenManifestPackageName
}

func (a *AndroidApp) renameResourcesPackage() bool {
	return proptools.BoolDefault(a.overridableAppProperties.Rename_resources_package, true)
}

func getAconfigFilePaths(ctx android.ModuleContext) (aconfigTextFilePaths android.Paths) {
	ctx.VisitDirectDepsProxy(func(dep android.ModuleProxy) {
		tag := ctx.OtherModuleDependencyTag(dep)
		switch tag {
		case staticLibTag, rroDepTag:
			if flagPackages, ok := android.OtherModuleProvider(ctx, dep, FlagsPackagesProvider); ok {
				aconfigTextFilePaths = append(aconfigTextFilePaths, flagPackages.AconfigTextFiles...)
			}

		case aconfigDeclarationTag:
			if provider, ok := android.OtherModuleProvider(ctx, dep, android.AconfigDeclarationsProviderKey); ok {
				aconfigTextFilePaths = append(aconfigTextFilePaths, provider.IntermediateDumpOutputPath)
			} else {
				ctx.ModuleErrorf("Only aconfig_declarations module type is allowed for "+
					"flags_packages property, but %s is not aconfig_declarations module type",
					dep.Name(),
				)
			}
		}
	})

	return android.FirstUniquePaths(aconfigTextFilePaths)
}

func (a *AndroidApp) aaptBuildActions(ctx android.ModuleContext) {
	usePlatformAPI := proptools.Bool(a.Module.deviceProperties.Platform_apis)
	if ctx.Module().(android.SdkContext).SdkVersion(ctx).Kind == android.SdkModule {
		usePlatformAPI = true
	}
	a.aapt.usesNonSdkApis = usePlatformAPI

	// Ask manifest_fixer to add or update the application element indicating this app has no code.
	a.aapt.hasNoCode = !a.hasCode(ctx)

	aaptLinkFlags := []string{}

	// Add TARGET_AAPT_CHARACTERISTICS values to AAPT link flags if they exist and --product flags were not provided.
	autogenerateRRO := proptools.Bool(a.appProperties.Generate_product_characteristics_rro)
	hasProduct := android.PrefixInList(a.aaptProperties.Aaptflags, "--product")
	characteristics := ctx.Config().ProductAAPTCharacteristics()
	if !autogenerateRRO && !hasProduct && len(characteristics) > 0 && characteristics != "default" {
		aaptLinkFlags = append(aaptLinkFlags, "--product", characteristics)
	}

	if !Bool(a.aaptProperties.Aapt_include_all_resources) {
		// Product AAPT config
		for _, aaptConfig := range ctx.Config().ProductAAPTConfig() {
			aaptLinkFlags = append(aaptLinkFlags, "-c", aaptConfig)
		}

		// Product AAPT preferred config
		if len(ctx.Config().ProductAAPTPreferredConfig()) > 0 {
			aaptLinkFlags = append(aaptLinkFlags, "--preferred-density", ctx.Config().ProductAAPTPreferredConfig())
		}
	}

	manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(ctx.ModuleName())
	packageNameProp := a.overridableAppProperties.Package_name.Get(ctx)
	if overridden || packageNameProp.IsPresent() {
		// The product override variable has a priority over the package_name property.
		if !overridden {
			manifestPackageName = packageNameProp.Get()
		}
		aaptLinkFlags = append(aaptLinkFlags, generateAaptRenamePackageFlags(manifestPackageName, a.renameResourcesPackage())...)
		a.overriddenManifestPackageName = manifestPackageName
	}

	aaptLinkFlags = append(aaptLinkFlags, a.additionalAaptFlags...)

	a.aapt.splitNames = a.appProperties.Package_splits
	a.aapt.LoggingParent = String(a.overridableAppProperties.Logging_parent)
	if a.Updatable() {
		if override := ctx.Config().Getenv("OVERRIDE_APEX_MANIFEST_DEFAULT_VERSION"); override != "" {
			a.aapt.defaultManifestVersion = override
		} else {
			a.aapt.defaultManifestVersion = ctx.Config().ReleaseDefaultUpdatableModuleVersion()
		}
	}

	// Use non final ids if we are doing optimized shrinking and are using R8.
	nonFinalIds := a.dexer.optimizedResourceShrinkingEnabled(ctx) && a.dexer.effectiveOptimizeEnabled(ctx)

	aconfigTextFilePaths := getAconfigFilePaths(ctx)

	a.aapt.buildActions(ctx,
		aaptBuildActionOptions{
			sdkContext:                     android.SdkContext(a),
			classLoaderContexts:            a.classLoaderContexts,
			excludedLibs:                   a.usesLibraryProperties.Exclude_uses_libs,
			enforceDefaultTargetSdkVersion: a.enforceDefaultTargetSdkVersion(),
			forceNonFinalResourceIDs:       nonFinalIds,
			emitResIds:                     Bool(a.appProperties.Emit_res_ids),
			extraLinkFlags:                 aaptLinkFlags,
			aconfigTextFiles:               aconfigTextFilePaths,
			usesLibrary:                    &a.usesLibrary,
		},
	)

	// apps manifests are handled by aapt, don't let Module see them
	a.properties.Manifest = proptools.Configurable[string]{}

	android.SetProvider(ctx, FlagsPackagesProvider, FlagsPackages{
		AconfigTextFiles: aconfigTextFilePaths,
	})
}

func (a *AndroidApp) proguardBuildActions(ctx android.ModuleContext) {
	var staticLibProguardFlagFiles android.Paths
	var includedStaticLibProguardFlagFiles android.Paths
	ctx.VisitDirectDepsProxy(func(m android.ModuleProxy) {
		depProguardInfo, _ := android.OtherModuleProvider(ctx, m, ProguardSpecInfoProvider)
		staticLibProguardFlagFiles = append(staticLibProguardFlagFiles, depProguardInfo.UnconditionallyExportedProguardFlags.ToList()...)
		includedStaticLibProguardFlagFiles = append(includedStaticLibProguardFlagFiles, depProguardInfo.IncludedUnconditionallyExportedProguardFlags.ToList()...)
		if ctx.OtherModuleDependencyTag(m) == staticLibTag {
			staticLibProguardFlagFiles = append(staticLibProguardFlagFiles, depProguardInfo.ProguardFlagsFiles.ToList()...)
			includedStaticLibProguardFlagFiles = append(includedStaticLibProguardFlagFiles, depProguardInfo.IncludedProguardFlagsFiles.ToList()...)
		}
	})

	staticLibProguardFlagFiles = android.FirstUniquePaths(staticLibProguardFlagFiles)
	includedStaticLibProguardFlagFiles = android.FirstUniquePaths(includedStaticLibProguardFlagFiles)

	a.Module.extraProguardFlagsFiles = append(a.Module.extraProguardFlagsFiles, staticLibProguardFlagFiles...)
	a.Module.extraIncludedProguardFlagsFiles = append(a.Module.extraIncludedProguardFlagsFiles, includedStaticLibProguardFlagFiles...)
	if !(a.dexer.optimizedResourceShrinkingEnabled(ctx)) {
		// When using the optimized shrinking the R8 enqueuer will traverse the xml files that become
		// live for code references and (transitively) mark these as live.
		// In this case we explicitly don't wan't the aapt2 generated keep files (which would keep the now
		// dead code alive)
		a.Module.extraProguardFlagsFiles = append(a.Module.extraProguardFlagsFiles, a.proguardOptionsFile)
	}
}

func (a *AndroidApp) installPath(ctx android.ModuleContext) android.InstallPath {
	var installDir string
	if ctx.ModuleName() == "framework-res" {
		// framework-res.apk is installed as system/framework/framework-res.apk
		installDir = "framework"
	} else if ctx.ModuleName() == "org.lineageos.platform-res" {
		// org.lineageos.platform-res.apk is installed as system/framework/org.lineageos.platform-res.apk
		installDir = "framework"
	} else if a.Privileged() {
		installDir = filepath.Join("priv-app", a.installApkName)
	} else {
		installDir = filepath.Join("app", a.installApkName)
	}

	return android.PathForModuleInstall(ctx, installDir, a.installApkName+".apk")
}

func (a *AndroidApp) dexBuildActions(ctx android.ModuleContext) (android.Path, android.Path, *JavaInfo) {
	a.dexpreopter.installPath = a.installPath(ctx)
	a.dexpreopter.isApp = true
	if a.dexProperties.Uncompress_dex == nil {
		// If the value was not force-set by the user, use reasonable default based on the module.
		a.dexProperties.Uncompress_dex = proptools.BoolPtr(a.shouldUncompressDex(ctx))
	}
	a.dexpreopter.uncompressedDex = *a.dexProperties.Uncompress_dex
	a.dexpreopter.enforceUsesLibs = a.usesLibrary.enforceUsesLibraries(ctx)
	a.dexpreopter.classLoaderContexts = a.classLoaderContexts
	a.dexpreopter.manifestFile = a.mergedManifestFile

	var packageResources = a.exportPackage

	javaInfo := &JavaInfo{}
	if ctx.ModuleName() != "framework-res" && ctx.ModuleName() != "org.lineageos.platform-res" {
		if a.dexer.resourceShrinkingEnabled(ctx) {
			protoFile := android.PathForModuleOut(ctx, packageResources.Base()+".proto.apk")
			aapt2Convert(ctx, protoFile, packageResources, "proto")
			a.dexer.resourcesInput = android.OptionalPathForPath(protoFile)
		}

		if a.useResourceProcessorBusyBox(ctx) {
			// When building an app with ResourceProcessorBusyBox enabled ResourceProcessorBusyBox has already
			// created R.class files that provide IDs for resources in busybox/R.jar.  Pass that file in the
			// classpath when compiling everything else, and add it to the final classes jar.
			a.Module.extraClasspathJars = append(a.Module.extraClasspathJars, a.aapt.rJar)
			a.Module.extraCombinedJars = append(a.Module.extraCombinedJars, a.aapt.rJar)
		} else {
			// When building an app without ResourceProcessorBusyBox the aapt2 rule creates R.srcjar containing
			// R.java files for the app's package and the packages from all transitive static android_library
			// dependencies.  Compile the srcjar alongside the rest of the sources.
			a.Module.extraSrcJars = append(a.Module.extraSrcJars, a.aapt.aaptSrcJar)
		}

		javaInfo = a.Module.compile(ctx)
		if a.dexer.resourceShrinkingEnabled(ctx) {
			binaryResources := android.PathForModuleOut(ctx, packageResources.Base()+".binary.out.apk")
			aapt2Convert(ctx, binaryResources, a.dexer.resourcesOutput.Path(), "binary")
			packageResources = binaryResources
		}
	}

	return a.dexJarFile.PathOrNil(), packageResources, javaInfo
}

func (a *AndroidApp) jniBuildActions(jniLibs []jniLib, prebuiltJniPackages android.Paths, ctx android.ModuleContext) android.WritablePath {
	var jniJarFile android.WritablePath
	if len(jniLibs) > 0 || len(prebuiltJniPackages) > 0 {
		a.jniLibs = jniLibs
		if a.shouldEmbedJnis(ctx) {
			jniJarFile = android.PathForModuleOut(ctx, "jnilibs.zip")
			a.installPathForJNISymbols = a.installPath(ctx)
			TransformJniLibsToJar(ctx, jniJarFile, jniLibs, prebuiltJniPackages, a.useEmbeddedNativeLibs(ctx))
			for _, jni := range jniLibs {
				if jni.coverageFile.Valid() {
					// Only collect coverage for the first target arch if this is a multilib target.
					// TODO(jungjw): Ideally, we want to collect both reports, but that would cause coverage
					// data file path collisions since the current coverage file path format doesn't contain
					// arch-related strings. This is fine for now though; the code coverage team doesn't use
					// multi-arch targets such as test_suite_* for coverage collections yet.
					//
					// Work with the team to come up with a new format that handles multilib modules properly
					// and change this.
					if len(ctx.Config().Targets[android.Android]) == 1 ||
						ctx.Config().AndroidFirstDeviceTarget.Arch.ArchType == jni.target.Arch.ArchType {
						a.jniCoverageOutputs = append(a.jniCoverageOutputs, jni.coverageFile.Path())
					}
				}
			}
			a.embeddedJniLibs = true
		}
	}
	return jniJarFile
}

func JNISymbolsInstalls(jniLibs []jniLib, installPath string) android.RuleBuilderInstalls {
	var jniSymbols android.RuleBuilderInstalls
	for _, jniLib := range jniLibs {
		if jniLib.unstrippedFile != nil {
			jniSymbols = append(jniSymbols, android.RuleBuilderInstall{
				From: jniLib.unstrippedFile,
				To:   filepath.Join(installPath, targetToJniDir(jniLib.target), jniLib.unstrippedFile.Base()),
			})
		}
	}
	return jniSymbols
}

// Reads and prepends a main cert from the default cert dir if it hasn't been set already, i.e. it
// isn't a cert module reference. Also checks and enforces system cert restriction if applicable.
func processMainCert(m android.ModuleBase, certPropValue string, certificates []Certificate,
	ctx android.ModuleContext) (mainCertificate Certificate, allCertificates []Certificate) {
	if android.SrcIsModule(certPropValue) == "" {
		var mainCert Certificate
		if certPropValue != "" {
			defaultDir := ctx.Config().DefaultAppCertificateDir(ctx)
			mainCert = Certificate{
				Pem: defaultDir.Join(ctx, certPropValue+".x509.pem"),
				Key: defaultDir.Join(ctx, certPropValue+".pk8"),
			}
		} else {
			pem, key := ctx.Config().DefaultAppCertificate(ctx)
			mainCert = Certificate{
				Pem: pem,
				Key: key,
			}
		}
		certificates = append([]Certificate{mainCert}, certificates...)
	}

	if len(certificates) > 0 {
		mainCertificate = certificates[0]
	} else {
		// This can be reached with an empty certificate list if AllowMissingDependencies is set
		// and the certificate property for this module is a module reference to a missing module.
		if !ctx.Config().AllowMissingDependencies() && len(ctx.GetMissingDependencies()) > 0 {
			panic("Should only get here if AllowMissingDependencies set and there are missing dependencies")
		}
		// Set a certificate to avoid panics later when accessing it.
		mainCertificate = Certificate{
			Key: android.PathForModuleOut(ctx, "missing.pk8"),
			Pem: android.PathForModuleOut(ctx, "missing.x509.pem"),
		}
	}

	if !m.Platform() {
		certPath := mainCertificate.Pem.String()
		systemCertPath := ctx.Config().DefaultAppCertificateDir(ctx).String()
		if strings.HasPrefix(certPath, systemCertPath) {
			enforceSystemCert := ctx.Config().EnforceSystemCertificate()
			allowed := ctx.Config().EnforceSystemCertificateAllowList()

			if enforceSystemCert && !inList(m.Name(), allowed) {
				ctx.PropertyErrorf("certificate", "The module in product partition cannot be signed with certificate in system.")
			}
		}
	}

	return mainCertificate, certificates
}

func (a *AndroidApp) InstallApkName() string {
	return a.installApkName
}

func (a *AndroidApp) createPrivappAllowlist(ctx android.ModuleContext) android.Path {
	if a.appProperties.Privapp_allowlist == nil {
		return nil
	}

	isOverrideApp := a.GetOverriddenBy() != ""
	if !isOverrideApp {
		// if this is not an override, we don't need to rewrite the existing privapp allowlist
		return android.PathForModuleSrc(ctx, *a.appProperties.Privapp_allowlist)
	}

	packageNameProp := a.overridableAppProperties.Package_name.Get(ctx)
	if packageNameProp.IsEmpty() {
		ctx.PropertyErrorf("privapp_allowlist", "package_name must be set to use privapp_allowlist")
	}

	packageName := packageNameProp.Get()
	fileName := "privapp_allowlist_" + packageName + ".xml"
	outPath := android.PathForModuleOut(ctx, fileName)
	ctx.Build(pctx, android.BuildParams{
		Rule:   modifyAllowlist,
		Input:  android.PathForModuleSrc(ctx, *a.appProperties.Privapp_allowlist),
		Output: outPath,
		Args: map[string]string{
			"packageName": packageName,
		},
	})
	return outPath
}

func (a *AndroidApp) createPreinstallAllowlist(ctx android.ModuleContext) android.Path {
	if a.appProperties.Preinstall_allowlist == nil {
		return nil
	}

	isOverrideApp := a.GetOverriddenBy() != ""
	if !isOverrideApp {
		// if this is not an override, we don't need to rewrite the existing preinstall allowlist
		return android.PathForModuleSrc(ctx, *a.appProperties.Preinstall_allowlist)
	}

	packageNameProp := a.overridableAppProperties.Package_name.Get(ctx)
	if packageNameProp.IsEmpty() {
		ctx.PropertyErrorf("preinstall_allowlist", "package_name must be set to use preinstall_allowlist")
	}

	packageName := packageNameProp.Get()
	fileName := "preinstall_allowlist_" + packageName + ".xml"
	outPath := android.PathForModuleOut(ctx, fileName)
	ctx.Build(pctx, android.BuildParams{
		Rule:   modifyPreinstallAllowlist,
		Input:  android.PathForModuleSrc(ctx, *a.appProperties.Preinstall_allowlist),
		Output: outPath,
		Args: map[string]string{
			"packageName": packageName,
		},
	})
	return outPath
}

func (a *AndroidApp) generateAndroidBuildActions(ctx android.ModuleContext) {
	a.jacocoInfo.Class = "APPS"

	var apkDeps android.Paths

	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	if !apexInfo.IsForPlatform() {
		a.HideFromMake()
	}

	a.aapt.useEmbeddedNativeLibs = a.useEmbeddedNativeLibs(ctx)
	a.aapt.useEmbeddedDex = Bool(a.appProperties.Use_embedded_dex)

	// Unlike installApkName, a.stem should respect base module name for override_android_app.
	// Therefore, use ctx.ModuleName() instead of a.Name().
	a.stem = proptools.StringDefault(a.overridableProperties.Stem, ctx.ModuleName())

	// Check if the install APK name needs to be overridden.
	// Both android_app and override_android_app module are expected to possess
	// its module bound apk path. However, override_android_app inherits ctx.ModuleName()
	// from the base module. Therefore, use a.Name() which represents
	// the module name for both android_app and override_android_app.
	a.installApkName = ctx.DeviceConfig().OverridePackageNameFor(
		proptools.StringDefault(a.overridableProperties.Stem, a.Name()))

	if ctx.ModuleName() == "framework-res" {
		// framework-res.apk is installed as system/framework/framework-res.apk
		a.installDir = android.PathForModuleInstall(ctx, "framework")
	} else if ctx.ModuleName() == "org.lineageos.platform-res" {
		// org.lineageos.platform-res.apk is installed as system/framework/org.lineageos.platform-res.apk
		a.installDir = android.PathForModuleInstall(ctx, "framework")
	} else if a.Privileged() {
		a.installDir = android.PathForModuleInstall(ctx, "priv-app", a.installApkName)
	} else if ctx.InstallInTestcases() {
		a.installDir = android.PathForModuleInstall(ctx, a.installApkName, ctx.DeviceConfig().DeviceArch())
	} else {
		a.installDir = android.PathForModuleInstall(ctx, "app", a.installApkName)
	}
	a.onDeviceDir = android.InstallPathToOnDevicePath(ctx, a.installDir)

	a.classLoaderContexts = a.usesLibrary.classLoaderContextForUsesLibDeps(ctx)
	if a.usesLibrary.shouldDisableDexpreopt {
		a.dexpreopter.disableDexpreopt()
	}

	var noticeAssetPath android.WritablePath
	if Bool(a.appProperties.Embed_notices) || ctx.Config().IsEnvTrue("ALWAYS_EMBED_NOTICES") {
		// The rule to create the notice file can't be generated yet, as the final output path
		// for the apk isn't known yet.  Add the path where the notice file will be generated to the
		// aapt rules now before calling aaptBuildActions, the rule to create the notice file will
		// be generated later.
		noticeAssetPath = android.PathForModuleOut(ctx, "NOTICE", "NOTICE.html.gz")
		a.aapt.noticeFile = android.OptionalPathForPath(noticeAssetPath)
	}

	// For apps targeting latest target_sdk_version
	if Bool(a.appProperties.Enforce_default_target_sdk_version) {
		a.SetEnforceDefaultTargetSdkVersion(true)
	}

	// Process all building blocks, from AAPT to certificates.
	a.aaptBuildActions(ctx)
	// The decision to enforce <uses-library> checks is made before adding implicit SDK libraries.
	a.usesLibrary.freezeEnforceUsesLibraries(ctx)

	// Check that the <uses-library> list is coherent with the manifest.
	if a.usesLibrary.enforceUsesLibraries(ctx) {
		manifestCheckFile := a.usesLibrary.verifyUsesLibrariesManifest(
			ctx, a.mergedManifestFile, &a.classLoaderContexts)
		apkDeps = append(apkDeps, manifestCheckFile)
	}

	a.proguardBuildActions(ctx)

	a.linter.mergedManifest = a.aapt.mergedManifestFile
	a.linter.manifest = a.aapt.manifestPath
	a.linter.resources = a.aapt.resourceFiles
	a.linter.buildModuleReportZip = ctx.Config().HasUnbundledBuildApps()

	dexJarFile, packageResources, javaInfo := a.dexBuildActions(ctx)

	// No need to check the SDK version of the JNI deps unless we embed them
	checkNativeSdkVersion := a.shouldEmbedJnis(ctx) && !Bool(a.appProperties.Jni_uses_platform_apis)
	jniLibs, prebuiltJniPackages, certificates := collectAppDeps(ctx, a, a.shouldEmbedJnis(ctx), checkNativeSdkVersion)
	jniJarFile := a.jniBuildActions(jniLibs, prebuiltJniPackages, ctx)

	if ctx.Failed() {
		return
	}

	a.certificate, certificates = processMainCert(a.ModuleBase, a.getCertString(ctx), certificates, ctx)

	// Build a final signed app package.
	packageFile := android.PathForModuleOut(ctx, a.installApkName+".apk")
	v4SigningRequested := Bool(a.Module.deviceProperties.V4_signature)
	var v4SignatureFile android.WritablePath = nil
	if v4SigningRequested {
		v4SignatureFile = android.PathForModuleOut(ctx, a.installApkName+".apk.idsig")
	}
	var lineageFile android.Path
	if lineage := String(a.overridableAppProperties.Lineage); lineage != "" {
		lineageFile = android.PathForModuleSrc(ctx, lineage)
	}
	rotationMinSdkVersion := String(a.overridableAppProperties.RotationMinSdkVersion)

	CreateAndSignAppPackage(ctx, packageFile, packageResources, jniJarFile, dexJarFile, certificates, apkDeps,
		v4SignatureFile, lineageFile, rotationMinSdkVersion, a.MinSdkVersion(ctx))
	a.outputFile = packageFile
	if v4SigningRequested {
		a.extraOutputFiles = append(a.extraOutputFiles, v4SignatureFile)
	}

	if a.aapt.noticeFile.Valid() {
		// Generating the notice file rule has to be here after a.outputFile is known.
		noticeFile := android.PathForModuleOut(ctx, "NOTICE.html.gz")
		android.BuildNoticeHtmlOutputFromLicenseMetadata(
			ctx, noticeFile, "", "",
			android.BuildNoticeFromLicenseDataArgs{
				StripPrefix: []string{
					a.installDir.String() + "/",
					android.PathForModuleInstall(ctx).String() + "/",
					a.outputFile.String(),
				},
			},
			ctx.ModuleProxy(),
		)
		builder := android.NewRuleBuilder(pctx, ctx).SandboxDisabled()
		builder.Command().Text("cp").
			Input(noticeFile).
			Output(noticeAssetPath)
		builder.Build("notice_dir", "Building notice dir")
	}

	for _, split := range a.aapt.splits {
		// Sign the split APKs
		packageFile := android.PathForModuleOut(ctx, a.installApkName+"_"+split.suffix+".apk")
		if v4SigningRequested {
			v4SignatureFile = android.PathForModuleOut(ctx, a.installApkName+"_"+split.suffix+".apk.idsig")
		}
		CreateAndSignAppPackage(ctx, packageFile, split.path, nil, nil, certificates, apkDeps,
			v4SignatureFile, lineageFile, rotationMinSdkVersion, a.MinSdkVersion(ctx))
		a.extraOutputFiles = append(a.extraOutputFiles, packageFile)
		if v4SigningRequested {
			a.extraOutputFiles = append(a.extraOutputFiles, v4SignatureFile)
		}
	}

	// Build an app bundle.
	bundleFile := android.PathForModuleOut(ctx, "base.zip")
	BuildBundleModule(ctx, bundleFile, a.exportPackage, jniJarFile, dexJarFile)
	a.bundleFile = bundleFile
	android.SetProvider(ctx, BundleProvider, BundleInfo{Bundle: bundleFile})

	allowlist := a.createPrivappAllowlist(ctx)
	complianceMetadataInfo := ctx.ComplianceMetadataInfo()
	if allowlist != nil {
		a.privAppAllowlist = android.OptionalPathForPath(allowlist)
		complianceMetadataInfo.AddBuiltFiles(a.privAppAllowlist.Path().String())
	}

	preinstallAllowlist := a.createPreinstallAllowlist(ctx)
	if preinstallAllowlist != nil {
		a.preinstallAllowlist = android.OptionalPathForPath(preinstallAllowlist)
		complianceMetadataInfo.AddBuiltFiles(a.preinstallAllowlist.Path().String())
	}

	// Install the app package.
	shouldInstallAppPackage := (Bool(a.Module.properties.Installable) || ctx.Host()) && apexInfo.IsForPlatform()
	if shouldInstallAppPackage {
		if a.privAppAllowlist.Valid() {
			allowlistInstallPath := android.PathForModuleInstall(ctx, "etc", "permissions")
			allowlistInstallFilename := a.installApkName + ".xml"
			ctx.InstallFile(allowlistInstallPath, allowlistInstallFilename, a.privAppAllowlist.Path())
		}
		if a.preinstallAllowlist.Valid() {
			allowlistInstallPath := android.PathForModuleInstall(ctx, "etc", "sysconfig")
			allowlistInstallFilename := a.installApkName + "_preinstall_allowlist.xml"
			ctx.InstallFile(allowlistInstallPath, allowlistInstallFilename, a.preinstallAllowlist.Path())
		}

		var extraInstalledPaths android.InstallPaths
		for _, extra := range a.extraOutputFiles {
			installed := ctx.InstallFile(a.installDir, extra.Base(), extra)
			extraInstalledPaths = append(extraInstalledPaths, installed)
		}
		// If we don't embed jni libs, make sure that those are installed along with the
		// app, and also place symlinks to the installed paths under the lib/<arch>
		// directory of the app installation directory. ex:
		// /system/app/MyApp/lib/arm64/libfoo.so -> /system/lib64/libfoo.so
		if !a.embeddedJniLibs {
			for _, jniLib := range jniLibs {
				archStr := jniLib.target.Arch.ArchType.String()
				symlinkDir := a.installDir.Join(ctx, "lib", archStr)
				for _, installedLib := range jniLib.installPaths {
					// install the symlink itself
					symlinkName := installedLib.Base()
					symlinkTarget := android.InstallPathToOnDevicePath(ctx, installedLib)
					ctx.InstallAbsoluteSymlink(symlinkDir, symlinkName, symlinkTarget)
				}
			}
		}
		a.installedOutputFile = ctx.InstallFile(a.installDir, a.outputFile.Base(), a.outputFile, extraInstalledPaths...)
	}

	ctx.CheckbuildFile(a.outputFile)

	a.buildAppDependencyInfo(ctx)

	providePrebuiltInfo(ctx,
		prebuiltInfoProps{
			baseModuleName: a.BaseModuleName(),
			isPrebuilt:     false,
		},
	)

	if javaInfo != nil {
		javaInfo.OutputFile = a.outputFile
		setExtraJavaInfo(ctx, a, javaInfo)
		android.SetProvider(ctx, JavaInfoProvider, javaInfo)
	}

	moduleInfoJSON := ctx.ModuleInfoJSON()
	moduleInfoJSON.Class = []string{"APPS"}
	if !a.embeddedJniLibs {
		for _, jniLib := range a.jniLibs {
			moduleInfoJSON.ExtraRequired = append(moduleInfoJSON.ExtraRequired, jniLib.name)
		}
	}

	a.setOutputFiles(ctx)

	buildComplianceMetadata(ctx)
	if shouldInstallAppPackage && slices.Contains(ctx.Config().UnbundledBuildApps(), a.Name()) {
		complianceMetadataInfo.SetFilesContained([]string{a.installedOutputFile.String()})
		complianceMetadataInfo.SetBuildOutputPathsOfFilesContained([]string{a.outputFile.String()})
		complianceMetadataInfo.AddBuiltFiles(a.outputFile.String())
	}

	if !a.IsHideFromMake() {
		if a.embeddedJniLibs {
			cc.CopySymbolsAndSetSymbolsInfoProvider(ctx, &cc.SymbolInfos{
				Symbols: a.GetJniSymbolInfos(ctx, a.installPathForJNISymbols),
			})
		}
	}
}

func (a *AndroidApp) setOutputFiles(ctx android.ModuleContext) {
	ctx.SetOutputFiles([]android.Path{a.proguardOptionsFile}, ".aapt.proguardOptionsFile")
	if a.aaptSrcJar != nil {
		ctx.SetOutputFiles([]android.Path{a.aaptSrcJar}, ".aapt.srcjar")
	}
	if a.rJar != nil {
		ctx.SetOutputFiles([]android.Path{a.rJar}, ".aapt.jar")
	}
	if a.aapt.resIdsFile.Valid() {
		ctx.SetOutputFiles([]android.Path{a.aapt.resIdsFile.Path()}, ".res-ids.txt")
	}
	ctx.SetOutputFiles([]android.Path{a.outputFile}, ".apk")
	ctx.SetOutputFiles([]android.Path{a.exportPackage}, ".export-package.apk")
	ctx.SetOutputFiles([]android.Path{a.aapt.manifestPath}, ".manifest.xml")
	setOutputFiles(ctx, &a.Library.Module)
}

type appDepsInterface interface {
	SdkVersion(ctx android.ConfigContext) android.SdkSpec
	MinSdkVersion(ctx android.MinSdkVersionFromValueContext) android.ApiLevel
	RequiresStableAPIs(ctx android.BaseModuleContext) bool
}

func collectAppDeps(ctx android.ModuleContext, app appDepsInterface,
	shouldCollectRecursiveNativeDeps bool,
	checkNativeSdkVersion bool) ([]jniLib, android.Paths, []Certificate) {

	if checkNativeSdkVersion {
		checkNativeSdkVersion = app.SdkVersion(ctx).Specified() &&
			app.SdkVersion(ctx).Kind != android.SdkCorePlatform && !app.RequiresStableAPIs(ctx)
	}
	jniLib, prebuiltJniPackages := collectJniDeps(ctx, shouldCollectRecursiveNativeDeps,
		checkNativeSdkVersion, func(parent, child android.ModuleProxy) bool {
			apkInApex := ctx.Module().(android.ApexModule).NotInPlatform()
			childLinkable, _ := android.OtherModuleProvider(ctx, child, cc.LinkableInfoProvider)
			parentIsLinkable := false
			if android.EqualModules(ctx.Module(), parent) {
				parentLinkable, _ := ctx.Module().(cc.LinkableInterface)
				parentIsLinkable = parentLinkable != nil
			} else {
				_, parentIsLinkable = android.OtherModuleProvider(ctx, parent, cc.LinkableInfoProvider)
			}
			useStubsOfDep := childLinkable.IsStubs
			if apkInApex && parentIsLinkable {
				// APK-in-APEX
				// If the parent is a linkable interface, use stubs if the dependency edge crosses an apex boundary.
				useStubsOfDep = useStubsOfDep || (childLinkable.HasStubsVariants && cc.ShouldUseStubForApex(ctx, parent, child))
			}
			return !childLinkable.IsNdk && !useStubsOfDep
		})

	var certificates []Certificate

	var directImplementationDeps android.Paths
	var transitiveImplementationDeps []depset.DepSet[android.Path]
	ctx.VisitDirectDepsProxy(func(module android.ModuleProxy) {
		otherName := ctx.OtherModuleName(module)
		tag := ctx.OtherModuleDependencyTag(module)

		if tag == certificateTag {
			if dep, ok := android.OtherModuleProvider(ctx, module, AndroidAppCertificateInfoProvider); ok {
				certificates = append(certificates, dep.Certificate)
			} else {
				ctx.ModuleErrorf("certificate dependency %q must be an android_app_certificate module", otherName)
			}
		}

		if IsJniDepTag(tag) {
			directImplementationDeps = append(directImplementationDeps, android.OutputFileForModule(ctx, module, ""))
			if info, ok := android.OtherModuleProvider(ctx, module, cc.ImplementationDepInfoProvider); ok {
				transitiveImplementationDeps = append(transitiveImplementationDeps, info.ImplementationDeps)
			}
		}
	})
	android.SetProvider(ctx, cc.ImplementationDepInfoProvider, &cc.ImplementationDepInfo{
		ImplementationDeps: depset.New(depset.PREORDER, directImplementationDeps, transitiveImplementationDeps),
	})

	return jniLib, prebuiltJniPackages, certificates
}

func collectJniDeps(ctx android.ModuleContext,
	shouldCollectRecursiveNativeDeps bool,
	checkNativeSdkVersion bool,
	filter func(parent, child android.ModuleProxy) bool) ([]jniLib, android.Paths) {
	var jniLibs []jniLib
	var prebuiltJniPackages android.Paths
	seenModulePaths := make(map[string]bool)

	ctx.WalkDepsProxy(func(module, parent android.ModuleProxy) bool {
		if !android.OtherModulePointerProviderOrDefault(ctx, module, android.CommonModuleInfoProvider).Enabled {
			return false
		}
		otherName := ctx.OtherModuleName(module)
		tag := ctx.OtherModuleDependencyTag(module)

		if IsJniDepTag(tag) || cc.IsSharedDepTag(tag) {
			if dep, ok := android.OtherModuleProvider(ctx, module, cc.LinkableInfoProvider); ok {
				if filter != nil && !filter(parent, module) {
					return false
				}

				lib := dep.OutputFile
				if lib.Valid() {
					path := lib.Path()
					if seenModulePaths[path.String()] {
						return false
					}
					seenModulePaths[path.String()] = true

					commonInfo := android.OtherModulePointerProviderOrDefault(ctx, module, android.CommonModuleInfoProvider)
					if checkNativeSdkVersion && commonInfo.SdkVersion == "" {
						ctx.PropertyErrorf("jni_libs", "JNI dependency %q uses platform APIs, but this module does not",
							otherName)
					}

					jniLibs = append(jniLibs, jniLib{
						name:           ctx.OtherModuleName(module),
						path:           path,
						target:         commonInfo.Target,
						coverageFile:   dep.CoverageOutputFile,
						unstrippedFile: dep.UnstrippedOutputFile,
						partition:      dep.Partition,
						installPaths:   android.GetInstallFilesCommon(commonInfo).InstallFiles,
					})
				} else if ctx.Config().AllowMissingDependencies() {
					ctx.AddMissingDependencies([]string{otherName})
				} else {
					ctx.ModuleErrorf("dependency %q missing output file", otherName)
				}
			} else {
				ctx.ModuleErrorf("jni_libs dependency %q must be a cc library", otherName)
			}

			return shouldCollectRecursiveNativeDeps
		}

		if info, ok := android.OtherModuleProvider(ctx, module, JniPackageProvider); ok {
			prebuiltJniPackages = append(prebuiltJniPackages, info.JniPackages...)
		}

		return false
	})

	return jniLibs, prebuiltJniPackages
}

func (a *AndroidApp) WalkPayloadDeps(ctx android.BaseModuleContext, do android.PayloadDepsCallback) {
	ctx.WalkDepsProxy(func(child, parent android.ModuleProxy) bool {
		// TODO(ccross): Should this use android.DepIsInSameApex?  Right now it is applying the android app
		// heuristics to every transitive dependency, when it should probably be using the heuristics of the
		// immediate parent.
		isExternal := !a.GetDepInSameApexChecker().OutgoingDepIsInSameApex(ctx.OtherModuleDependencyTag(child))
		if am, ok := android.OtherModuleProvider(ctx, child, android.CommonModuleInfoProvider); ok && am.IsApexModule {
			if !do(ctx, parent, child, isExternal) {
				return false
			}
		}
		return !isExternal
	})
}

func (a *AndroidApp) buildAppDependencyInfo(ctx android.ModuleContext) {
	if ctx.Host() {
		return
	}

	depsInfo := android.DepNameToDepInfoMap{}
	a.WalkPayloadDeps(ctx, func(ctx android.BaseModuleContext, from, to android.ModuleProxy, externalDep bool) bool {
		depName := to.Name()

		// Skip dependencies that are only available to APEXes; they are developed with updatability
		// in mind and don't need manual approval.
		if android.OtherModuleProviderOrDefault(ctx, to, android.PlatformAvailabilityInfoProvider).NotAvailableToPlatform {
			return true
		}

		if info, exist := depsInfo[depName]; exist {
			info.From = append(info.From, from.Name())
			info.IsExternal = info.IsExternal && externalDep
			depsInfo[depName] = info
		} else {
			toMinSdkVersion := "(no version)"
			if info, ok := android.OtherModuleProvider(ctx, to, android.CommonModuleInfoProvider); ok &&
				!info.MinSdkVersion.IsPlatform && info.MinSdkVersion.ApiLevel != nil {
				toMinSdkVersion = info.MinSdkVersion.ApiLevel.String()
			}
			depsInfo[depName] = android.ApexModuleDepInfo{
				To:            depName,
				From:          []string{from.Name()},
				IsExternal:    externalDep,
				MinSdkVersion: toMinSdkVersion,
			}
		}
		return true
	})

	a.ApexBundleDepsInfo.BuildDepsInfoLists(ctx, a.MinSdkVersion(ctx).String(), depsInfo)
}

func (a *AndroidApp) enforceDefaultTargetSdkVersion() bool {
	return a.appProperties.EnforceDefaultTargetSdkVersion
}

func (a *AndroidApp) SetEnforceDefaultTargetSdkVersion(val bool) {
	a.appProperties.EnforceDefaultTargetSdkVersion = val
}

func (a *AndroidApp) Updatable() bool {
	return Bool(a.appProperties.Updatable)
}

func (a *AndroidApp) getCertString(ctx android.BaseModuleContext) string {
	certificate, overridden := ctx.DeviceConfig().OverrideCertificateFor(ctx.ModuleName())
	if overridden {
		return ":" + certificate
	}
	return a.overridableAppProperties.Certificate.GetOrDefault(ctx, "")
}

func (m *AndroidApp) GetDepInSameApexChecker() android.DepInSameApexChecker {
	return AppDepInSameApexChecker{}
}

// @auto-generate: gob
type AppDepInSameApexChecker struct {
	android.BaseDepInSameApexChecker
}

func (m AppDepInSameApexChecker) OutgoingDepIsInSameApex(tag blueprint.DependencyTag) bool {
	if IsJniDepTag(tag) {
		return true
	}
	return depIsInSameApex(tag)
}

func (a *AndroidApp) Privileged() bool {
	return Bool(a.appProperties.Privileged)
}

func (a *AndroidApp) IsNativeCoverageNeeded(ctx cc.IsNativeCoverageNeededContext) bool {
	return ctx.Device() && ctx.DeviceConfig().NativeCoverageEnabled()
}

var _ cc.UseCoverage = &AndroidApp{}

func (a *AndroidApp) IDEInfo(ctx android.BaseModuleContext, dpInfo *android.IdeInfo) {
	a.Library.IDEInfo(ctx, dpInfo)
	a.aapt.IDEInfo(ctx, dpInfo)
}

func (a *AndroidApp) productCharacteristicsRROPackageName() string {
	return proptools.String(a.appProperties.ProductCharacteristicsRROPackageName)
}

func (a *AndroidApp) productCharacteristicsRROManifestModuleName() string {
	return proptools.String(a.appProperties.ProductCharacteristicsRROManifestModuleName)
}

// android_app compiles sources and Android resources into an Android application package `.apk` file.
func AndroidAppFactory() android.Module {
	module := &AndroidApp{}

	module.Module.dexProperties.Optimize.EnabledByDefault = true
	module.Module.dexProperties.Optimize.ShrinkByDefault = true

	module.Module.properties.Instrument = true
	module.Module.properties.Supports_static_instrumentation = true
	module.Module.properties.Installable = proptools.BoolPtr(true)

	module.addHostAndDeviceProperties()
	module.AddProperties(
		&module.aaptProperties,
		&module.appProperties,
		&module.overridableAppProperties,
		&module.Library.sourceProperties)

	module.usesLibrary.enforce = true

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.overridableAppProperties.Overrides)
	android.InitApexModule(module)

	android.AddLoadHook(module, func(ctx android.LoadHookContext) {
		a := ctx.Module().(*AndroidApp)

		// Normally this would be set alongisde other `Optimize` properties in the outer factory
		// init, but we require a context to fetch the build flag value. It could also go in
		// GenerateAndroidBuildActions, but that is shared with other app variants (e.g., test apps)
		// where we don't necessarily want this default applied.
		a.dexProperties.Optimize.OptimizeByDefault = ctx.Config().EnableAppOptimizationByDefault()

		characteristics := ctx.Config().ProductAAPTCharacteristics()
		if characteristics == "default" || characteristics == "" {
			module.appProperties.Generate_product_characteristics_rro = nil
			// no need to create RRO
			return
		}

		if !proptools.Bool(module.appProperties.Generate_product_characteristics_rro) {
			return
		}

		rroPackageName := AutogeneratedProductCharacteristicsRroModuleName(ctx, a.Name())
		rroManifestName := rroPackageName + "_manifest"

		a.appProperties.ProductCharacteristicsRROPackageName = proptools.StringPtr(rroPackageName)
		a.appProperties.ProductCharacteristicsRROManifestModuleName = proptools.StringPtr(rroManifestName)

		rroManifestProperties := struct {
			Name  *string
			Tools []string
			Out   []string
			Srcs  []string
			Cmd   *string
		}{
			Name:  proptools.StringPtr(rroManifestName),
			Tools: []string{"characteristics_rro_generator", "aapt2"},
			Out:   []string{"AndroidManifest.xml"},
			Srcs:  []string{":" + a.Name() + "{.apk}"},
			Cmd:   proptools.StringPtr("$(location characteristics_rro_generator) $$($(location aapt2) dump packagename $(in)) $(out)"),
		}
		ctx.CreateModule(GenRuleFactory, &rroManifestProperties)

		rroProperties := struct {
			Name           *string
			Filter_product *string
			Aaptflags      []string
			Manifest       *string
			Resource_dirs  proptools.Configurable[[]string]
			Flags_packages []string
		}{
			Name:           proptools.StringPtr(rroPackageName),
			Filter_product: proptools.StringPtr(characteristics),
			Aaptflags:      []string{"--auto-add-overlay"},
			Manifest:       proptools.StringPtr(":" + rroManifestName),
			Resource_dirs:  a.aaptProperties.Resource_dirs,
			Flags_packages: a.aaptProperties.Flags_packages,
		}
		if !Bool(a.aaptProperties.Aapt_include_all_resources) {
			for _, aaptConfig := range ctx.Config().ProductAAPTConfig() {
				rroProperties.Aaptflags = append(rroProperties.Aaptflags, "-c", aaptConfig)
			}
		}
		ctx.CreateModule(RuntimeResourceOverlayFactory, &rroProperties)

	})

	module.SetDefaultableHook(func(ctx android.DefaultableHookContext) {
		createInternalRuntimeOverlays(ctx, module.ModuleBase)
	})

	return module
}

func AutogeneratedRroModuleName(ctx android.EarlyModuleContext, moduleName, partition string) string {
	// auto_generated_rro is installed to the product or vendor partition and
	// are specific to target product. To add the auto_generated_rro to the
	// required list of the original app module, the target product name is
	// required.
	// TODO(b/412526509): Remove the rro dependency from the system partition.
	// So, the original system module does not need to know the target product
	// name. Instead, add a reverse dependency from the rro to the original
	// system module to see if the module exists.
	return fmt.Sprintf("%s__%s__auto_generated_rro_%s", moduleName, ctx.Config().Getenv("TARGET_PRODUCT"), partition)
}

func AutogeneratedProductCharacteristicsRroModuleName(ctx android.EarlyModuleContext, moduleName string) string {
	return moduleName + "__" + strings.ReplaceAll(ctx.Config().ProductAAPTCharacteristics(), ",", "_") + "__auto_generated_characteristics_rro"
}

type createModuleContext interface {
	android.EarlyModuleContext
	CreateModule(android.ModuleFactory, ...interface{}) android.Module
}

func createInternalRuntimeOverlays(ctx createModuleContext, a android.ModuleBase) {
	if !ctx.Config().HasDeviceProduct() {
		return
	}
	// vendor
	vendorOverlayProps := struct {
		Name                *string
		Base                *string
		Vendor              *bool
		Product_specific    *bool
		System_ext_specific *bool
		Manifest            *string
		Sdk_version         *string
		Compile_multilib    *string
		Enabled             proptools.Configurable[bool]
	}{
		Name:                proptools.StringPtr(AutogeneratedRroModuleName(ctx, a.Name(), "vendor")),
		Base:                proptools.StringPtr(a.Name()),
		Vendor:              proptools.BoolPtr(true),
		Product_specific:    proptools.BoolPtr(false),
		System_ext_specific: proptools.BoolPtr(false),
		Manifest:            proptools.StringPtr(":" + a.Name() + "{.manifest.xml}"),
		Sdk_version:         proptools.StringPtr("current"),
		Compile_multilib:    proptools.StringPtr("first"),
		Enabled:             a.EnabledProperty().Clone(),
	}
	ctx.CreateModule(AutogenRuntimeResourceOverlayFactory, &vendorOverlayProps)

	// product
	productOverlayProps := struct {
		Name                *string
		Base                *string
		Vendor              *bool
		Proprietary         *bool
		Soc_specific        *bool
		Product_specific    *bool
		System_ext_specific *bool
		Manifest            *string
		Sdk_version         *string
		Compile_multilib    *string
		Enabled             proptools.Configurable[bool]
	}{
		Name:                proptools.StringPtr(AutogeneratedRroModuleName(ctx, a.Name(), "product")),
		Base:                proptools.StringPtr(a.Name()),
		Vendor:              proptools.BoolPtr(false),
		Proprietary:         proptools.BoolPtr(false),
		Soc_specific:        proptools.BoolPtr(false),
		Product_specific:    proptools.BoolPtr(true),
		System_ext_specific: proptools.BoolPtr(false),
		Manifest:            proptools.StringPtr(":" + a.Name() + "{.manifest.xml}"),
		Sdk_version:         proptools.StringPtr("current"),
		Compile_multilib:    proptools.StringPtr("first"),
		Enabled:             a.EnabledProperty().Clone(),
	}
	ctx.CreateModule(AutogenRuntimeResourceOverlayFactory, &productOverlayProps)
}

// A dictionary of values to be overridden in the manifest.
type Manifest_values struct {
	// Overrides the value of package_name in the manifest
	ApplicationId *string
}

type appTestProperties struct {
	// The name of the android_app module that the tests will run against.
	Instrumentation_for *string

	// If specified, the instrumentation target package name in the manifest is overwritten by it.
	Instrumentation_target_package *string

	// If specified, the mainline module package name in the test config is overwritten by it.
	Mainline_package_name *string

	Manifest_values Manifest_values
}

type AndroidTest struct {
	AndroidApp

	appTestProperties appTestProperties

	testProperties testProperties

	testConfig       android.Path
	extraTestConfigs android.Paths
	data             android.Paths
}

func (a *AndroidTest) InstallInTestcases() bool {
	return true
}

type androidTestApp interface {
	includedInTestSuite(searchPrefix string) bool
}

func (a *AndroidTest) includedInTestSuite(searchPrefix string) bool {
	return android.PrefixInList(a.testProperties.Test_suites, searchPrefix)
}

func (a *AndroidTestHelperApp) includedInTestSuite(searchPrefix string) bool {
	return android.PrefixInList(a.appTestHelperAppProperties.Test_suites, searchPrefix)
}

func (a *AndroidTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	checkMinSdkVersionMts(ctx, a.MinSdkVersion(ctx))
	var configs []tradefed.Config
	if a.appTestProperties.Instrumentation_target_package != nil {
		a.additionalAaptFlags = append(a.additionalAaptFlags,
			"--rename-instrumentation-target-package "+*a.appTestProperties.Instrumentation_target_package)
	} else if a.appTestProperties.Instrumentation_for != nil {
		// Check if the instrumentation target package is overridden.
		manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(*a.appTestProperties.Instrumentation_for)
		if overridden {
			a.additionalAaptFlags = append(a.additionalAaptFlags, "--rename-instrumentation-target-package "+manifestPackageName)
		}
	}
	applicationId := a.appTestProperties.Manifest_values.ApplicationId
	if applicationId != nil {
		packageNameProp := a.overridableAppProperties.Package_name.Get(ctx)
		if packageNameProp.IsPresent() {
			ctx.PropertyErrorf("manifest_values.applicationId", "property is not supported when property package_name is set.")
		}
		a.aapt.manifestValues.applicationId = *applicationId
	}

	if Bool(a.testProperties.Enable_static_aconfig_pb) {
		addStaticAconfigProto(ctx, &a.extraResources)
	}

	a.generateAndroidBuildActions(ctx)
	if ctx.Failed() {
		return
	}

	android.SetProvider(ctx, ApkCertInfoProvider, ApkCertInfo{
		Certificate: a.Certificate(),
		Name:        a.InstallApkName() + ".apk",
		Test:        true,
	})

	for _, c := range a.testProperties.Test_options.Tradefed_options {
		configs = append(configs, c)
	}
	for _, module := range a.testProperties.Test_mainline_modules {
		configs = append(configs, tradefed.Option{Name: "config-descriptor:metadata", Key: "mainline-param", Value: module})
	}

	testConfig := tradefed.AutoGenInstrumentationTestConfig(ctx, a.testProperties.Test_config,
		a.testProperties.Test_config_template, a.manifestPath, a.testProperties.Test_suites,
		a.testProperties.Auto_gen_config, configs, a.testProperties.Test_options.Test_runner_options)
	a.testConfig = a.FixTestConfig(ctx, testConfig)
	a.extraTestConfigs = android.PathsForModuleSrc(ctx, a.testProperties.Test_options.Extra_test_configs)
	a.data = android.PathsForModuleSrc(ctx, a.testProperties.Data.GetOrDefault(ctx, nil))
	a.data = append(a.data, android.PathsForModuleSrc(ctx, a.testProperties.Device_common_data)...)
	a.data = append(a.data, android.PathsForModuleSrc(ctx, a.testProperties.Device_first_data)...)
	a.data = append(a.data, android.PathsForModuleSrc(ctx, a.testProperties.Device_first_prefer32_data)...)
	a.data = append(a.data, android.PathsForModuleSrc(ctx, a.testProperties.Host_common_data)...)
	a.data = append(a.data, android.PathsForModuleSrc(ctx, a.testProperties.Host_first_data)...)

	a.data = android.SortedUniquePaths(a.data)
	a.extraTestConfigs = android.SortedUniquePaths(a.extraTestConfigs)

	var testData []android.DataPath
	for _, data := range a.data {
		dataPath := android.DataPath{SrcPath: data}
		testData = append(testData, dataPath)
	}
	for _, d := range a.extraOutputFiles {
		testData = append(testData, android.DataPath{SrcPath: d})
	}
	ctx.SetTestSuiteInfo(android.TestSuiteInfo{
		TestSuites:                a.testProperties.Test_suites,
		MainFile:                  a.outputFile,
		MainFileStem:              a.installApkName,
		MainFileExt:               ".apk",
		ConfigFile:                a.testConfig,
		ExtraConfigs:              a.extraTestConfigs,
		NonArchData:               testData,
		CompatibilitySupportFiles: a.data,
		NeedsArchFolder:           true,
		PerTestcaseDirectory:      proptools.Bool(a.testProperties.Per_testcase_directory),
		IsUnitTest:                Bool(a.testProperties.Test_options.Unit_test),
	})

	android.SetProvider(ctx, tradefed.BaseTestProviderKey, tradefed.BaseTestProviderData{
		TestcaseRelDataFiles:    testcaseRel(a.data),
		OutputFile:              a.OutputFile(),
		TestConfig:              a.testConfig,
		HostRequiredModuleNames: a.HostRequiredModuleNames(),
		TestSuites:              a.testProperties.Test_suites,
		IsHost:                  false,
		LocalCertificate:        a.certificate.AndroidMkString(),
		IsUnitTest:              Bool(a.testProperties.Test_options.Unit_test),
		MkInclude:               "$(BUILD_SYSTEM)/soong_app_prebuilt.mk",
		MkAppClass:              "APPS",
	})
	ctx.SetTestModuleInfo(&android.TestModuleInformation{
		TestOnly:       true,
		TopLevelTarget: true,
	})

	moduleInfoJSON := ctx.ModuleInfoJSON()
	moduleInfoJSON.Tags = append(moduleInfoJSON.Tags, "tests")
	if a.testConfig != nil {
		moduleInfoJSON.TestConfig = append(moduleInfoJSON.TestConfig, a.testConfig.String())
	}
	moduleInfoJSON.TestConfig = append(moduleInfoJSON.TestConfig, a.extraTestConfigs.Strings()...)
	if len(a.testProperties.Test_suites) > 0 {
		moduleInfoJSON.CompatibilitySuites = append(moduleInfoJSON.CompatibilitySuites, a.testProperties.Test_suites...)
	} else {
		moduleInfoJSON.CompatibilitySuites = append(moduleInfoJSON.CompatibilitySuites, "null-suite")
	}

	if _, ok := testConfig.(android.WritablePath); ok {
		moduleInfoJSON.AutoTestConfig = []string{"true"}
	}
	moduleInfoJSON.TestMainlineModules = append(moduleInfoJSON.TestMainlineModules, a.testProperties.Test_mainline_modules...)
}

func testcaseRel(paths android.Paths) []string {
	relPaths := []string{}
	for _, p := range paths {
		relPaths = append(relPaths, p.Rel())
	}
	return relPaths
}

func (a *AndroidTest) FixTestConfig(ctx android.ModuleContext, testConfig android.Path) android.Path {
	if testConfig == nil {
		return nil
	}

	fixedConfig := android.PathForModuleOut(ctx, "test_config_fixer", "AndroidTest.xml")
	rule := android.NewRuleBuilder(pctx, ctx).SandboxDisabled()
	command := rule.Command().BuiltTool("test_config_fixer").Input(testConfig).Output(fixedConfig)
	fixNeeded := false

	// Auto-generated test config uses `ModuleName` as the APK name. So fix it if it is not the case.
	if ctx.ModuleName() != a.installApkName {
		fixNeeded = true
		command.FlagWithArg("--test-file-name ", a.installApkName+".apk")
	}

	packageNameProp := a.overridableAppProperties.Package_name.Get(ctx)
	if packageNameProp.IsPresent() {
		fixNeeded = true
		command.FlagWithInput("--manifest ", a.manifestPath).
			FlagWithArg("--package-name ", packageNameProp.Get())
	}

	if a.appTestProperties.Mainline_package_name != nil {
		fixNeeded = true
		command.FlagWithArg("--mainline-package-name ", *a.appTestProperties.Mainline_package_name)
	}

	if fixNeeded {
		rule.Build("fix_test_config", "fix test config")
		return fixedConfig
	}
	return testConfig
}

func (a *AndroidTestHelperApp) DepsMutator(ctx android.BottomUpMutatorContext) {
	if len(a.ApexProperties.Apex_available) == 0 && ctx.Config().IsEnvTrue("EMMA_API_MAPPER") {
		// Instrument the android_test_helper target to log potential API calls at the run time.
		// Contact android-xts-infra team before using the environment var EMMA_API_MAPPER.
		ctx.AddVariationDependencies(nil, staticLibTag, "apimapper-helper-device-lib")
		a.setApiMapper(true)
	}
	a.AndroidApp.DepsMutator(ctx)
}

func (a *AndroidTest) DepsMutator(ctx android.BottomUpMutatorContext) {
	if len(a.ApexProperties.Apex_available) == 0 && ctx.Config().IsEnvTrue("EMMA_API_MAPPER") {
		// Instrument the android_test_helper target to log potential API calls at the run time.
		// Contact android-xts-infra team before using the environment var EMMA_API_MAPPER.
		ctx.AddVariationDependencies(nil, staticLibTag, "apimapper-helper-device-lib")
		a.setApiMapper(true)
	}
	a.AndroidApp.DepsMutator(ctx)
}

func (a *AndroidTest) OverridablePropertiesDepsMutator(ctx android.BottomUpMutatorContext) {
	a.AndroidApp.OverridablePropertiesDepsMutator(ctx)
	if a.appTestProperties.Instrumentation_for != nil {
		// The android_app dependency listed in instrumentation_for needs to be added to the classpath for javac,
		// but not added to the aapt2 link includes like a normal android_app or android_library dependency, so
		// use instrumentationForTag instead of libTag.
		ctx.AddVariationDependencies(nil, instrumentationForTag, String(a.appTestProperties.Instrumentation_for))
	}
}

// android_test compiles test sources and Android resources into an Android application package `.apk` file and
// creates an `AndroidTest.xml` file to allow running the test with `atest` or a `TEST_MAPPING` file.
func AndroidTestFactory() android.Module {
	module := &AndroidTest{}

	module.Module.dexProperties.Optimize.EnabledByDefault = false

	module.Module.properties.Instrument = true
	module.Module.properties.Supports_static_instrumentation = true
	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.appProperties.Use_embedded_native_libs = proptools.BoolPtr(true)
	module.appProperties.AlwaysPackageNativeLibs = true
	module.Module.dexpreopter.isTest = true
	module.Module.linter.properties.Lint.Test_module_type = proptools.BoolPtr(true)

	module.addHostAndDeviceProperties()
	module.AddProperties(
		&module.aaptProperties,
		&module.appProperties,
		&module.appTestProperties,
		&module.overridableAppProperties,
		&module.testProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.overridableAppProperties.Overrides)

	return module
}

type appTestHelperAppProperties struct {
	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// Flag to indicate whether or not to create test config automatically. If AndroidTest.xml
	// doesn't exist next to the Android.bp, this attribute doesn't need to be set to true
	// explicitly.
	Auto_gen_config *bool

	// Install the test into a folder named for the module in all test suites.
	Per_testcase_directory *bool

	Manifest_values Manifest_values

	// Whether to include a static aconfig pb as a test resource. If the pb is included and a
	// flag is read-only, tests read the flag value from the static pb.
	Enable_static_aconfig_pb *bool
}

type AndroidTestHelperApp struct {
	AndroidApp

	appTestHelperAppProperties appTestHelperAppProperties
}

func (a *AndroidTestHelperApp) InstallInTestcases() bool {
	return true
}

// android_test_helper_app compiles sources and Android resources into an Android application package `.apk` file that
// will be used by tests, but does not produce an `AndroidTest.xml` file so the module will not be run directly as a
// test.
func AndroidTestHelperAppFactory() android.Module {
	module := &AndroidTestHelperApp{}

	// TODO(b/192032291): Disable by default after auditing downstream usage.
	module.Module.dexProperties.Optimize.EnabledByDefault = true
	module.Module.dexProperties.Optimize.Ignore_library_extends_program = proptools.BoolPtr(true)

	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.appProperties.Use_embedded_native_libs = proptools.BoolPtr(true)
	module.appProperties.AlwaysPackageNativeLibs = true
	module.Module.dexpreopter.isTest = true
	module.Module.linter.properties.Lint.Test_module_type = proptools.BoolPtr(true)

	module.addHostAndDeviceProperties()
	module.AddProperties(
		&module.aaptProperties,
		&module.appProperties,
		&module.appTestHelperAppProperties,
		&module.overridableAppProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitApexModule(module)
	return module
}

type AndroidAppCertificate struct {
	android.ModuleBase

	properties  AndroidAppCertificateProperties
	Certificate Certificate
}

type AndroidAppCertificateProperties struct {
	// Name of the certificate files.  Extensions .x509.pem and .pk8 will be added to the name.
	Certificate *string
}

// @auto-generate: gob
type AndroidAppCertificateInfo struct {
	Certificate Certificate
}

var AndroidAppCertificateInfoProvider = blueprint.NewProvider[AndroidAppCertificateInfo]()

// android_app_certificate modules can be referenced by the certificates property of android_app modules to select
// the signing key.
func AndroidAppCertificateFactory() android.Module {
	module := &AndroidAppCertificate{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

func (c *AndroidAppCertificate) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	cert := String(c.properties.Certificate)
	c.Certificate = Certificate{
		Pem: android.PathForModuleSrc(ctx, cert+".x509.pem"),
		Key: android.PathForModuleSrc(ctx, cert+".pk8"),
	}

	android.SetProvider(ctx, AndroidAppCertificateInfoProvider, AndroidAppCertificateInfo{
		Certificate: c.Certificate,
	})
}

type OverrideAndroidApp struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (i *OverrideAndroidApp) GenerateAndroidBuildActions(_ android.ModuleContext) {
	// All the overrides happen in the base module.
	// TODO(jungjw): Check the base module type.
}

// override_android_app is used to create an android_app module based on another android_app by overriding
// some of its properties.
func OverrideAndroidAppModuleFactory() android.Module {
	m := &OverrideAndroidApp{}
	m.AddProperties(
		&OverridableProperties{},
		&overridableAppProperties{},
	)

	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, android.MultilibCommon)
	android.InitOverrideModule(m)
	android.AddLoadHookWithPriority(m, func(ctx android.LoadHookContext) {
		createInternalRuntimeOverlays(ctx, m.ModuleBase)
	}, 1) // Run after soong config load hoook

	return m
}

type OverrideAndroidTest struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (i *OverrideAndroidTest) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// All the overrides happen in the base module.
	// TODO(jungjw): Check the base module type.
	ctx.SetTestModuleInfo(&android.TestModuleInformation{
		TestOnly:       true,
		TopLevelTarget: true,
	})
}

// override_android_test is used to create an android_app module based on another android_test by overriding
// some of its properties.
func OverrideAndroidTestModuleFactory() android.Module {
	m := &OverrideAndroidTest{}
	m.AddProperties(&overridableAppProperties{})
	m.AddProperties(&appTestProperties{})

	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, android.MultilibCommon)
	android.InitOverrideModule(m)
	return m
}

type UsesLibraryProperties struct {
	// A list of shared library modules that will be listed in uses-library tags in the AndroidManifest.xml file.
	Uses_libs proptools.Configurable[[]string]

	// A list of shared library modules that will be listed in uses-library tags in the AndroidManifest.xml file with
	// required=false.
	Optional_uses_libs proptools.Configurable[[]string]

	// If true, the list of uses_libs and optional_uses_libs modules must match the AndroidManifest.xml file.  Defaults
	// to true if either uses_libs or optional_uses_libs is set.  Will unconditionally default to true in the future.
	Enforce_uses_libs *bool

	// Optional name of the <uses-library> provided by this module. This is needed for non-SDK
	// libraries, because SDK ones are automatically picked up by Soong. The <uses-library> name
	// normally is the same as the module name, but there are exceptions.
	Provides_uses_lib *string

	// A list of shared library names to exclude from the classpath of the APK. Adding a library here
	// will prevent it from being used when precompiling the APK and prevent it from being implicitly
	// added to the APK's manifest's <uses-library> elements.
	//
	// Care must be taken when using this as it could result in runtime errors if the APK actually
	// uses classes provided by the library and which are not provided in any other way.
	//
	// This is primarily intended for use by various CTS tests that check the runtime handling of the
	// android.test.base shared library (and related libraries) but which depend on some common
	// libraries that depend on the android.test.base library. Without this those tests will end up
	// with a <uses-library android:name="android.test.base"/> in their manifest which would either
	// render the tests worthless (as they would be testing the wrong behavior), or would break the
	// test altogether by providing access to classes that the tests were not expecting. Those tests
	// provide the android.test.base statically and use jarjar to rename them so they do not collide
	// with the classes provided by the android.test.base library.
	Exclude_uses_libs []string

	// The module names of optional uses-library libraries that are missing from the source tree.
	Missing_optional_uses_libs []string `blueprint:"mutated"`
}

// usesLibrary provides properties and helper functions for AndroidApp and AndroidAppImport to verify that the
// <uses-library> tags that end up in the manifest of an APK match the ones known to the build system through the
// uses_libs and optional_uses_libs properties.  The build system's values are used by dexpreopt to preopt apps
// with knowledge of their shared libraries.
type usesLibrary struct {
	usesLibraryProperties UsesLibraryProperties

	// Whether to enforce verify_uses_library check.
	enforce bool

	// Whether dexpreopt should be disabled
	shouldDisableDexpreopt bool
}

func (u *usesLibrary) deps(ctx android.BottomUpMutatorContext, addCompatDeps bool) {
	if !ctx.Config().UnbundledBuild() || ctx.Config().UnbundledBuildImage() {
		ctx.AddVariationDependencies(nil, usesLibReqTag, u.usesLibraryProperties.Uses_libs.GetOrDefault(ctx, nil)...)
		presentOptionalUsesLibs := u.presentOptionalUsesLibs(ctx)
		ctx.AddVariationDependencies(nil, usesLibOptTag, presentOptionalUsesLibs...)
		// Only add these extra dependencies if the module is an app that depends on framework
		// libs. This avoids creating a cyclic dependency:
		//     e.g. framework-res -> org.apache.http.legacy -> ... -> framework-res.
		if addCompatDeps {
			// Dexpreopt needs paths to the dex jars of these libraries in order to construct
			// class loader context for dex2oat. Add them as a dependency with a special tag.
			ctx.AddVariationDependencies(nil, usesLibCompat29ReqTag, dexpreopt.CompatUsesLibs29...)
			ctx.AddVariationDependencies(nil, usesLibCompat28OptTag, dexpreopt.OptionalCompatUsesLibs28...)
			ctx.AddVariationDependencies(nil, usesLibCompat30OptTag, dexpreopt.OptionalCompatUsesLibs30...)
		}
		_, diff, _ := android.ListSetDifference(u.usesLibraryProperties.Optional_uses_libs.GetOrDefault(ctx, nil), presentOptionalUsesLibs)
		u.usesLibraryProperties.Missing_optional_uses_libs = diff
	} else {
		ctx.AddVariationDependencies(nil, r8LibraryJarTag, u.usesLibraryProperties.Uses_libs.GetOrDefault(ctx, nil)...)
		ctx.AddVariationDependencies(nil, r8LibraryJarTag, u.presentOptionalUsesLibs(ctx)...)
	}
}

func (u *usesLibrary) depsFromLibs(ctx android.BottomUpMutatorContext, libDeps []android.ModuleProxy) {
	// For library dependencies that are component libraries (like stubs), add the implementation
	// as a dependency (dexpreopt needs to be against the implementation library, not stubs).
	for _, dep := range libDeps {
		if !dep.IsNil() {
			if component, ok := android.OtherModuleProvider(ctx, dep, SdkLibraryComponentDependencyInfoProvider); ok {
				if lib := component.OptionalSdkLibraryImplementation; lib != nil {
					// Add library as optional if it's one of the optional compatibility libs or it's
					// explicitly listed in the optional_uses_libs property.
					tag := usesLibReqTag
					if android.InList(*lib, dexpreopt.OptionalCompatUsesLibs) ||
						android.InList(*lib, u.usesLibraryProperties.Optional_uses_libs.GetOrDefault(ctx, nil)) {
						tag = usesLibOptTag
					}
					ctx.AddVariationDependencies(nil, tag, *lib)
				}
			}
		}
	}
}

// presentOptionalUsesLibs returns optional_uses_libs after filtering out libraries that don't exist in the source tree.
func (u *usesLibrary) presentOptionalUsesLibs(ctx android.BaseModuleContext) []string {
	optionalUsesLibs := android.FilterListPred(u.usesLibraryProperties.Optional_uses_libs.GetOrDefault(ctx, nil), func(s string) bool {
		return ctx.OtherModuleExists(s)
	})
	return optionalUsesLibs
}

// Returns a map of module names of shared library dependencies to the paths to their dex jars on
// host and on device.
func (u *usesLibrary) classLoaderContextForUsesLibDeps(ctx android.ModuleContext) dexpreopt.ClassLoaderContextMap {
	clcMap := make(dexpreopt.ClassLoaderContextMap)

	// Skip when UnbundledBuild() is true, but UnbundledBuildImage() is false. With
	// UnbundledBuildImage() it is necessary to generate dexpreopt.config for post-dexpreopting.
	if ctx.Config().UnbundledBuild() && !ctx.Config().UnbundledBuildImage() {
		return clcMap
	}

	ctx.VisitDirectDepsProxy(func(m android.ModuleProxy) {
		tag, isUsesLibTag := ctx.OtherModuleDependencyTag(m).(usesLibraryDependencyTag)
		if !isUsesLibTag {
			return
		}

		dep := android.RemoveOptionalPrebuiltPrefix(ctx.OtherModuleName(m))

		javaInfo, ok := android.OtherModuleProvider(ctx, m, JavaInfoProvider)
		if !ok {
			return
		}
		// Skip stub libraries. A dependency on the implementation library has been added earlier,
		// so it will be added to CLC, but the stub shouldn't be. Stub libraries can be distingushed
		// from implementation libraries by their name, which is different as it has a suffix.
		if comp, ok := android.OtherModuleProvider(ctx, m, SdkLibraryComponentDependencyInfoProvider); ok {
			if impl := comp.OptionalSdkLibraryImplementation; impl != nil && *impl != dep {
				return
			}
		}

		if lib := javaInfo.UsesLibraryDependencyInfo; lib != nil {
			if _, ok := android.OtherModuleProvider(ctx, m, SdkLibraryInfoProvider); ok {
				// Skip java_sdk_library dependencies that provide stubs, but not an implementation.
				// This will be restricted to optional_uses_libs
				if tag == usesLibOptTag && javaInfo.DexJarBuildPath.PathOrNil() == nil {
					u.shouldDisableDexpreopt = true
					return
				}
			}
			libName := dep
			if ulib := javaInfo.ProvidesUsesLibInfo; ulib != nil && ulib.ProvidesUsesLib != nil {
				libName = *ulib.ProvidesUsesLib
			}
			clcMap.AddContext(ctx, tag.sdkVersion, libName, tag.optional,
				javaInfo.DexJarBuildPath.PathOrNil(), lib.DexJarInstallPath,
				lib.GetClassLoaderContexts())
		} else if ctx.Config().AllowMissingDependencies() {
			ctx.AddMissingDependencies([]string{dep})
		} else {
			ctx.ModuleErrorf("module %q in uses_libs or optional_uses_libs must be a java library", dep)
		}
	})
	return clcMap
}

// enforceUsesLibraries returns true of <uses-library> tags should be checked against uses_libs and optional_uses_libs
// properties.  Defaults to true if either of uses_libs or optional_uses_libs is specified.  Will default to true
// unconditionally in the future.
func (u *usesLibrary) enforceUsesLibraries(ctx android.ModuleContext) bool {
	defaultEnforceUsesLibs := len(u.usesLibraryProperties.Uses_libs.GetOrDefault(ctx, nil)) > 0 ||
		len(u.usesLibraryProperties.Optional_uses_libs.GetOrDefault(ctx, nil)) > 0
	return BoolDefault(u.usesLibraryProperties.Enforce_uses_libs, u.enforce || defaultEnforceUsesLibs)
}

// Freeze the value of `enforce_uses_libs` based on the current values of `uses_libs` and `optional_uses_libs`.
func (u *usesLibrary) freezeEnforceUsesLibraries(ctx android.ModuleContext) {
	enforce := u.enforceUsesLibraries(ctx)
	u.usesLibraryProperties.Enforce_uses_libs = &enforce
}

// verifyUsesLibraries checks the <uses-library> tags in the manifest against the ones specified
// in the `uses_libs`/`optional_uses_libs` properties. The input can be either an XML manifest, or
// an APK with the manifest embedded in it (manifest_check will know which one it is by the file
// extension: APKs are supposed to end with '.apk').
func (u *usesLibrary) verifyUsesLibraries(ctx android.ModuleContext, inputFile android.Path,
	outputFile android.WritablePath, classLoaderContexts *dexpreopt.ClassLoaderContextMap) android.Path {

	statusFile := dexpreopt.UsesLibrariesStatusFile(ctx)

	// Disable verify_uses_libraries check if dexpreopt is globally disabled. Without dexpreopt the
	// check is not necessary, and although it is good to have, it is difficult to maintain on
	// non-linux build platforms where dexpreopt is generally disabled (the check may fail due to
	// various unrelated reasons, such as a failure to get manifest from an APK).
	global := dexpreopt.GetGlobalConfig(ctx)
	if global.DisablePreopt || global.OnlyPreoptArtBootImage {
		return inputFile
	}

	rule := android.NewRuleBuilder(pctx, ctx).SandboxDisabled()
	cmd := rule.Command().BuiltTool("manifest_check").
		Flag("--enforce-uses-libraries").
		Input(inputFile).
		FlagWithOutput("--enforce-uses-libraries-status ", statusFile).
		FlagWithInput("--aapt ", ctx.Config().HostToolPath(ctx, "aapt2"))

	if outputFile != nil {
		cmd.FlagWithOutput("-o ", outputFile)
	}

	if dexpreopt.GetGlobalConfig(ctx).RelaxUsesLibraryCheck {
		cmd.Flag("--enforce-uses-libraries-relax")
	}

	requiredUsesLibs, optionalUsesLibs := classLoaderContexts.UsesLibs()
	for _, lib := range requiredUsesLibs {
		cmd.FlagWithArg("--uses-library ", lib)
	}
	for _, lib := range optionalUsesLibs {
		cmd.FlagWithArg("--optional-uses-library ", lib)
	}

	// Also add missing optional uses libs, as the manifest check expects them.
	// Note that what we add here are the module names of those missing libs, not library names, while
	// the manifest check actually expects library names. However, the case where a library is missing
	// and the module name != the library name is too rare for us to handle.
	for _, lib := range u.usesLibraryProperties.Missing_optional_uses_libs {
		cmd.FlagWithArg("--missing-optional-uses-library ", lib)
	}

	for _, lib := range ctx.Config().BootJars() {
		cmd.FlagWithArg("--bootclasspath-libs ", lib)
	}

	rule.Build("verify_uses_libraries", "verify <uses-library>")
	return outputFile
}

// verifyUsesLibrariesManifest checks the <uses-library> tags in an AndroidManifest.xml against
// the build system and returns the path to a copy of the manifest.
func (u *usesLibrary) verifyUsesLibrariesManifest(ctx android.ModuleContext, manifest android.Path,
	classLoaderContexts *dexpreopt.ClassLoaderContextMap) android.Path {
	outputFile := android.PathForModuleOut(ctx, "manifest_check", "AndroidManifest.xml")
	return u.verifyUsesLibraries(ctx, manifest, outputFile, classLoaderContexts)
}

// verifyUsesLibrariesAPK checks the <uses-library> tags in the manifest of an APK against the build
// system and returns the path to a copy of the APK.
func (u *usesLibrary) verifyUsesLibrariesAPK(ctx android.ModuleContext, apk android.Path,
	classLoaderContexts *dexpreopt.ClassLoaderContextMap) {
	u.verifyUsesLibraries(ctx, apk, nil, classLoaderContexts) // for APKs manifest_check does not write output file
}

// androidApp is an interface to handle all app modules (android_app, android_app_import, etc.) in
// the same way.
type androidApp interface {
	android.Module
	Privileged() bool
	InstallApkName() string
	OutputFile() android.Path
	JacocoInfo() JacocoInfo
	Certificate() Certificate
	BaseModuleName() string
	PrivAppAllowlist() android.OptionalPath
	PreinstallAllowlist() android.OptionalPath
}

var _ androidApp = (*AndroidApp)(nil)
var _ androidApp = (*AndroidAppImport)(nil)
var _ androidApp = (*AndroidTestHelperApp)(nil)

func setCommonAppInfo(appInfo *AppInfo, m androidApp) {
	appInfo.Privileged = m.Privileged()
	appInfo.OutputFile = m.OutputFile()
	appInfo.InstallApkName = m.InstallApkName()
	appInfo.JacocoInfo = m.JacocoInfo()
	appInfo.Certificate = m.Certificate()
	appInfo.PrivAppAllowlist = m.PrivAppAllowlist()
	appInfo.PreinstallAllowlist = m.PreinstallAllowlist()
}

// @auto-generate: gob
type BundleInfo struct {
	Bundle android.Path
}

var BundleProvider = blueprint.NewProvider[BundleInfo]()
