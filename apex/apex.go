// Copyright (C) 2018 The Android Open Source Project
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

// package apex implements build rules for creating the APEX files which are container for
// lower-level system components. See https://source.android.com/devices/tech/ota/apex
package apex

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/bpf"
	"android/soong/cc"
	"android/soong/dexpreopt"
	prebuilt_etc "android/soong/etc"
	"android/soong/filesystem"
	"android/soong/java"
	"android/soong/python"
	"android/soong/rust"
	"android/soong/sh"
)

func init() {
	android.RegisterModuleType("apex", BundleFactory)
	android.RegisterModuleType("apex_test", testApexBundleFactory)
	android.RegisterModuleType("apex_vndk", vndkApexBundleFactory)
	android.RegisterModuleType("apex_defaults", defaultsFactory)
	android.RegisterModuleType("prebuilt_apex", PrebuiltFactory)
	android.RegisterModuleType("override_apex", overrideApexFactory)
	android.RegisterModuleType("apex_set", apexSetFactory)

	android.PreDepsMutators(RegisterPreDepsMutators)
	android.PostDepsMutators(RegisterPostDepsMutators)
}

func RegisterPreDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.TopDown("apex_vndk", apexVndkMutator).Parallel()
	ctx.BottomUp("apex_vndk_deps", apexVndkDepsMutator).Parallel()
}

func RegisterPostDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.TopDown("apex_info", apexInfoMutator).Parallel()
	ctx.BottomUp("apex_unique", apexUniqueVariationsMutator).Parallel()
	ctx.BottomUp("apex_test_for_deps", apexTestForDepsMutator).Parallel()
	ctx.BottomUp("apex_test_for", apexTestForMutator).Parallel()
	ctx.BottomUp("apex", apexMutator).Parallel()
	ctx.BottomUp("apex_directly_in_any", apexDirectlyInAnyMutator).Parallel()
	ctx.BottomUp("apex_flattened", apexFlattenedMutator).Parallel()
	ctx.BottomUp("mark_platform_availability", markPlatformAvailability).Parallel()
}

type apexBundleProperties struct {
	// Json manifest file describing meta info of this APEX bundle. Refer to
	// system/apex/proto/apex_manifest.proto for the schema. Default: "apex_manifest.json"
	Manifest *string `android:"path"`

	// AndroidManifest.xml file used for the zip container of this APEX bundle. If unspecified,
	// a default one is automatically generated.
	AndroidManifest *string `android:"path"`

	// Canonical name of this APEX bundle. Used to determine the path to the activated APEX on
	// device (/apex/<apex_name>). If unspecified, follows the name property.
	Apex_name *string

	// Determines the file contexts file for setting the security contexts to files in this APEX
	// bundle. For platform APEXes, this should points to a file under /system/sepolicy Default:
	// /system/sepolicy/apex/<module_name>_file_contexts.
	File_contexts *string `android:"path"`

	ApexNativeDependencies

	Multilib apexMultilibProperties

	// List of java libraries that are embedded inside this APEX bundle.
	Java_libs []string

	// List of prebuilt files that are embedded inside this APEX bundle.
	Prebuilts []string

	// List of BPF programs inside this APEX bundle.
	Bpfs []string

	// List of filesystem images that are embedded inside this APEX bundle.
	Filesystems []string

	// Name of the apex_key module that provides the private key to sign this APEX bundle.
	Key *string

	// Specifies the certificate and the private key to sign the zip container of this APEX. If
	// this is "foo", foo.x509.pem and foo.pk8 under PRODUCT_DEFAULT_DEV_CERTIFICATE are used
	// as the certificate and the private key, respectively. If this is ":module", then the
	// certificate and the private key are provided from the android_app_certificate module
	// named "module".
	Certificate *string

	// The minimum SDK version that this APEX must support at minimum. This is usually set to
	// the SDK version that the APEX was first introduced.
	Min_sdk_version *string

	// Whether this APEX is considered updatable or not. When set to true, this will enforce
	// additional rules for making sure that the APEX is truly updatable. To be updatable,
	// min_sdk_version should be set as well. This will also disable the size optimizations like
	// symlinking to the system libs. Default is false.
	Updatable *bool

	// Whether this APEX is installable to one of the partitions like system, vendor, etc.
	// Default: true.
	Installable *bool

	// Whether this APEX can be compressed or not. Setting this property to false means this
	// APEX will never be compressed. When set to true, APEX will be compressed if other
	// conditions, e.g, target device needs to support APEX compression, are also fulfilled.
	// Default: true.
	Compressible *bool

	// For native libraries and binaries, use the vendor variant instead of the core (platform)
	// variant. Default is false. DO NOT use this for APEXes that are installed to the system or
	// system_ext partition.
	Use_vendor *bool

	// If set true, VNDK libs are considered as stable libs and are not included in this APEX.
	// Should be only used in non-system apexes (e.g. vendor: true). Default is false.
	Use_vndk_as_stable *bool

	// List of SDKs that are used to build this APEX. A reference to an SDK should be either
	// `name#version` or `name` which is an alias for `name#current`. If left empty,
	// `platform#current` is implied. This value affects all modules included in this APEX. In
	// other words, they are also built with the SDKs specified here.
	Uses_sdks []string

	// The type of APEX to build. Controls what the APEX payload is. Either 'image', 'zip' or
	// 'both'. When set to image, contents are stored in a filesystem image inside a zip
	// container. When set to zip, contents are stored in a zip container directly. This type is
	// mostly for host-side debugging. When set to both, the two types are both built. Default
	// is 'image'.
	Payload_type *string

	// The type of filesystem to use when the payload_type is 'image'. Either 'ext4' or 'f2fs'.
	// Default 'ext4'.
	Payload_fs_type *string

	// For telling the APEX to ignore special handling for system libraries such as bionic.
	// Default is false.
	Ignore_system_library_special_case *bool

	// Whenever apex_payload.img of the APEX should include dm-verity hashtree. Should be only
	// used in tests.
	Test_only_no_hashtree *bool

	// Whenever apex_payload.img of the APEX should not be dm-verity signed. Should be only
	// used in tests.
	Test_only_unsigned_payload *bool

	// Whenever apex should be compressed, regardless of product flag used. Should be only
	// used in tests.
	Test_only_force_compression *bool

	IsCoverageVariant bool `blueprint:"mutated"`

	// List of sanitizer names that this APEX is enabled for
	SanitizerNames []string `blueprint:"mutated"`

	PreventInstall bool `blueprint:"mutated"`

	HideFromMake bool `blueprint:"mutated"`

	// Internal package method for this APEX. When payload_type is image, this can be either
	// imageApex or flattenedApex depending on Config.FlattenApex(). When payload_type is zip,
	// this becomes zipApex.
	ApexType apexPackaging `blueprint:"mutated"`
}

type ApexNativeDependencies struct {
	// List of native libraries that are embedded inside this APEX.
	Native_shared_libs []string

	// List of JNI libraries that are embedded inside this APEX.
	Jni_libs []string

	// List of rust dyn libraries
	Rust_dyn_libs []string

	// List of native executables that are embedded inside this APEX.
	Binaries []string

	// List of native tests that are embedded inside this APEX.
	Tests []string
}

type apexMultilibProperties struct {
	// Native dependencies whose compile_multilib is "first"
	First ApexNativeDependencies

	// Native dependencies whose compile_multilib is "both"
	Both ApexNativeDependencies

	// Native dependencies whose compile_multilib is "prefer32"
	Prefer32 ApexNativeDependencies

	// Native dependencies whose compile_multilib is "32"
	Lib32 ApexNativeDependencies

	// Native dependencies whose compile_multilib is "64"
	Lib64 ApexNativeDependencies
}

type apexTargetBundleProperties struct {
	Target struct {
		// Multilib properties only for android.
		Android struct {
			Multilib apexMultilibProperties
		}

		// Multilib properties only for host.
		Host struct {
			Multilib apexMultilibProperties
		}

		// Multilib properties only for host linux_bionic.
		Linux_bionic struct {
			Multilib apexMultilibProperties
		}

		// Multilib properties only for host linux_glibc.
		Linux_glibc struct {
			Multilib apexMultilibProperties
		}
	}
}

type apexArchBundleProperties struct {
	Arch struct {
		Arm struct {
			ApexNativeDependencies
		}
		Arm64 struct {
			ApexNativeDependencies
		}
		X86 struct {
			ApexNativeDependencies
		}
		X86_64 struct {
			ApexNativeDependencies
		}
	}
}

// These properties can be used in override_apex to override the corresponding properties in the
// base apex.
type overridableProperties struct {
	// List of APKs that are embedded inside this APEX.
	Apps []string

	// List of runtime resource overlays (RROs) that are embedded inside this APEX.
	Rros []string

	// Names of modules to be overridden. Listed modules can only be other binaries (in Make or
	// Soong). This does not completely prevent installation of the overridden binaries, but if
	// both binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will
	// be removed from PRODUCT_PACKAGES.
	Overrides []string

	// Logging parent value.
	Logging_parent string

	// Apex Container package name. Override value for attribute package:name in
	// AndroidManifest.xml
	Package_name string

	// A txt file containing list of files that are allowed to be included in this APEX.
	Allowed_files *string `android:"path"`
}

type apexBundle struct {
	// Inherited structs
	android.ModuleBase
	android.DefaultableModuleBase
	android.OverridableModuleBase
	android.SdkBase

	// Properties
	properties            apexBundleProperties
	targetProperties      apexTargetBundleProperties
	archProperties        apexArchBundleProperties
	overridableProperties overridableProperties
	vndkProperties        apexVndkProperties // only for apex_vndk modules

	///////////////////////////////////////////////////////////////////////////////////////////
	// Inputs

	// Keys for apex_paylaod.img
	publicKeyFile  android.Path
	privateKeyFile android.Path

	// Cert/priv-key for the zip container
	containerCertificateFile android.Path
	containerPrivateKeyFile  android.Path

	// Flags for special variants of APEX
	testApex bool
	vndkApex bool
	artApex  bool

	// Tells whether this variant of the APEX bundle is the primary one or not. Only the primary
	// one gets installed to the device.
	primaryApexType bool

	// Suffix of module name in Android.mk ".flattened", ".apex", ".zipapex", or ""
	suffix string

	// File system type of apex_payload.img
	payloadFsType fsType

	// Whether to create symlink to the system file instead of having a file inside the apex or
	// not
	linkToSystemLib bool

	// List of files to be included in this APEX. This is filled in the first part of
	// GenerateAndroidBuildActions.
	filesInfo []apexFile

	// List of other module names that should be installed when this APEX gets installed.
	requiredDeps []string

	///////////////////////////////////////////////////////////////////////////////////////////
	// Outputs (final and intermediates)

	// Processed apex manifest in JSONson format (for Q)
	manifestJsonOut android.WritablePath

	// Processed apex manifest in PB format (for R+)
	manifestPbOut android.WritablePath

	// Processed file_contexts files
	fileContexts android.WritablePath

	// Struct holding the merged notice file paths in different formats
	mergedNotices android.NoticeOutputs

	// The built APEX file. This is the main product.
	outputFile android.WritablePath

	// The built APEX file in app bundle format. This file is not directly installed to the
	// device. For an APEX, multiple app bundles are created each of which is for a specific ABI
	// like arm, arm64, x86, etc. Then they are processed again (outside of the Android build
	// system) to be merged into a single app bundle file that Play accepts. See
	// vendor/google/build/build_unbundled_mainline_module.sh for more detail.
	bundleModuleFile android.WritablePath

	// Target path to install this APEX. Usually out/target/product/<device>/<partition>/apex.
	installDir android.InstallPath

	// List of commands to create symlinks for backward compatibility. These commands will be
	// attached as LOCAL_POST_INSTALL_CMD to apex package itself (for unflattened build) or
	// apex_manifest (for flattened build) so that compat symlinks are always installed
	// regardless of TARGET_FLATTEN_APEX setting.
	compatSymlinks []string

	// Text file having the list of individual files that are included in this APEX. Used for
	// debugging purpose.
	installedFilesFile android.WritablePath

	// List of module names that this APEX is including (to be shown via *-deps-info target).
	// Used for debugging purpose.
	android.ApexBundleDepsInfo

	// Optional list of lint report zip files for apexes that contain java or app modules
	lintReports android.Paths

	prebuiltFileToDelete string

	isCompressed bool

	// Path of API coverage generate file
	apisUsedByModuleFile   android.ModuleOutPath
	apisBackedByModuleFile android.ModuleOutPath
}

// apexFileClass represents a type of file that can be included in APEX.
type apexFileClass int

const (
	app apexFileClass = iota
	appSet
	etc
	goBinary
	javaSharedLib
	nativeExecutable
	nativeSharedLib
	nativeTest
	pyBinary
	shBinary
)

// apexFile represents a file in an APEX bundle. This is created during the first half of
// GenerateAndroidBuildActions by traversing the dependencies of the APEX. Then in the second half
// of the function, this is used to create commands that copies the files into a staging directory,
// where they are packaged into the APEX file. This struct is also used for creating Make modules
// for each of the files in case when the APEX is flattened.
type apexFile struct {
	// buildFile is put in the installDir inside the APEX.
	builtFile   android.Path
	noticeFiles android.Paths
	installDir  string
	customStem  string
	symlinks    []string // additional symlinks

	// Info for Android.mk Module name of `module` in AndroidMk. Note the generated AndroidMk
	// module for apexFile is named something like <AndroidMk module name>.<apex name>[<apex
	// suffix>]
	androidMkModuleName       string             // becomes LOCAL_MODULE
	class                     apexFileClass      // becomes LOCAL_MODULE_CLASS
	moduleDir                 string             // becomes LOCAL_PATH
	requiredModuleNames       []string           // becomes LOCAL_REQUIRED_MODULES
	targetRequiredModuleNames []string           // becomes LOCAL_TARGET_REQUIRED_MODULES
	hostRequiredModuleNames   []string           // becomes LOCAL_HOST_REQUIRED_MODULES
	dataPaths                 []android.DataPath // becomes LOCAL_TEST_DATA

	jacocoReportClassesFile android.Path     // only for javalibs and apps
	lintDepSets             java.LintDepSets // only for javalibs and apps
	certificate             java.Certificate // only for apps
	overriddenPackageName   string           // only for apps

	transitiveDep bool
	isJniLib      bool

	multilib string

	// TODO(jiyong): remove this
	module android.Module
}

// TODO(jiyong): shorten the arglist using an option struct
func newApexFile(ctx android.BaseModuleContext, builtFile android.Path, androidMkModuleName string, installDir string, class apexFileClass, module android.Module) apexFile {
	ret := apexFile{
		builtFile:           builtFile,
		installDir:          installDir,
		androidMkModuleName: androidMkModuleName,
		class:               class,
		module:              module,
	}
	if module != nil {
		ret.noticeFiles = module.NoticeFiles()
		ret.moduleDir = ctx.OtherModuleDir(module)
		ret.requiredModuleNames = module.RequiredModuleNames()
		ret.targetRequiredModuleNames = module.TargetRequiredModuleNames()
		ret.hostRequiredModuleNames = module.HostRequiredModuleNames()
		ret.multilib = module.Target().Arch.ArchType.Multilib
	}
	return ret
}

func (af *apexFile) ok() bool {
	return af.builtFile != nil && af.builtFile.String() != ""
}

// apexRelativePath returns the relative path of the given path from the install directory of this
// apexFile.
// TODO(jiyong): rename this
func (af *apexFile) apexRelativePath(path string) string {
	return filepath.Join(af.installDir, path)
}

// path returns path of this apex file relative to the APEX root
func (af *apexFile) path() string {
	return af.apexRelativePath(af.stem())
}

// stem returns the base filename of this apex file
func (af *apexFile) stem() string {
	if af.customStem != "" {
		return af.customStem
	}
	return af.builtFile.Base()
}

// symlinkPaths returns paths of the symlinks (if any) relative to the APEX root
func (af *apexFile) symlinkPaths() []string {
	var ret []string
	for _, symlink := range af.symlinks {
		ret = append(ret, af.apexRelativePath(symlink))
	}
	return ret
}

// availableToPlatform tests whether this apexFile is from a module that can be installed to the
// platform.
func (af *apexFile) availableToPlatform() bool {
	if af.module == nil {
		return false
	}
	if am, ok := af.module.(android.ApexModule); ok {
		return am.AvailableFor(android.AvailableToPlatform)
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Mutators
//
// Brief description about mutators for APEX. The following three mutators are the most important
// ones.
//
// 1) DepsMutator: from the properties like native_shared_libs, java_libs, etc., modules are added
// to the (direct) dependencies of this APEX bundle.
//
// 2) apexInfoMutator: this is a post-deps mutator, so runs after DepsMutator. Its goal is to
// collect modules that are direct and transitive dependencies of each APEX bundle. The collected
// modules are marked as being included in the APEX via BuildForApex().
//
// 3) apexMutator: this is a post-deps mutator that runs after apexInfoMutator. For each module that
// are marked by the apexInfoMutator, apex variations are created using CreateApexVariations().

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string

	// Determines if the dependent will be part of the APEX payload. Can be false for the
	// dependencies to the signing key module, etc.
	payload bool
}

var (
	androidAppTag  = dependencyTag{name: "androidApp", payload: true}
	bpfTag         = dependencyTag{name: "bpf", payload: true}
	certificateTag = dependencyTag{name: "certificate"}
	executableTag  = dependencyTag{name: "executable", payload: true}
	fsTag          = dependencyTag{name: "filesystem", payload: true}
	javaLibTag     = dependencyTag{name: "javaLib", payload: true}
	jniLibTag      = dependencyTag{name: "jniLib", payload: true}
	keyTag         = dependencyTag{name: "key"}
	prebuiltTag    = dependencyTag{name: "prebuilt", payload: true}
	rroTag         = dependencyTag{name: "rro", payload: true}
	sharedLibTag   = dependencyTag{name: "sharedLib", payload: true}
	testForTag     = dependencyTag{name: "test for"}
	testTag        = dependencyTag{name: "test", payload: true}
)

// TODO(jiyong): shorten this function signature
func addDependenciesForNativeModules(ctx android.BottomUpMutatorContext, nativeModules ApexNativeDependencies, target android.Target, imageVariation string) {
	binVariations := target.Variations()
	libVariations := append(target.Variations(), blueprint.Variation{Mutator: "link", Variation: "shared"})
	rustLibVariations := append(target.Variations(), blueprint.Variation{Mutator: "rust_libraries", Variation: "dylib"})

	if ctx.Device() {
		binVariations = append(binVariations, blueprint.Variation{Mutator: "image", Variation: imageVariation})
		libVariations = append(libVariations, blueprint.Variation{Mutator: "image", Variation: imageVariation})
		rustLibVariations = append(rustLibVariations, blueprint.Variation{Mutator: "image", Variation: imageVariation})
	}

	// Use *FarVariation* to be able to depend on modules having conflicting variations with
	// this module. This is required since arch variant of an APEX bundle is 'common' but it is
	// 'arm' or 'arm64' for native shared libs.
	ctx.AddFarVariationDependencies(binVariations, executableTag, nativeModules.Binaries...)
	ctx.AddFarVariationDependencies(binVariations, testTag, nativeModules.Tests...)
	ctx.AddFarVariationDependencies(libVariations, jniLibTag, nativeModules.Jni_libs...)
	ctx.AddFarVariationDependencies(libVariations, sharedLibTag, nativeModules.Native_shared_libs...)
	ctx.AddFarVariationDependencies(rustLibVariations, sharedLibTag, nativeModules.Rust_dyn_libs...)
}

func (a *apexBundle) combineProperties(ctx android.BottomUpMutatorContext) {
	if ctx.Device() {
		proptools.AppendProperties(&a.properties.Multilib, &a.targetProperties.Target.Android.Multilib, nil)
	} else {
		proptools.AppendProperties(&a.properties.Multilib, &a.targetProperties.Target.Host.Multilib, nil)
		if ctx.Os().Bionic() {
			proptools.AppendProperties(&a.properties.Multilib, &a.targetProperties.Target.Linux_bionic.Multilib, nil)
		} else {
			proptools.AppendProperties(&a.properties.Multilib, &a.targetProperties.Target.Linux_glibc.Multilib, nil)
		}
	}
}

// getImageVariation returns the image variant name for this apexBundle. In most cases, it's simply
// android.CoreVariation, but gets complicated for the vendor APEXes and the VNDK APEX.
func (a *apexBundle) getImageVariation(ctx android.BottomUpMutatorContext) string {
	deviceConfig := ctx.DeviceConfig()
	if a.vndkApex {
		return cc.VendorVariationPrefix + a.vndkVersion(deviceConfig)
	}

	var prefix string
	var vndkVersion string
	if deviceConfig.VndkVersion() != "" {
		if proptools.Bool(a.properties.Use_vendor) {
			prefix = cc.VendorVariationPrefix
			vndkVersion = deviceConfig.PlatformVndkVersion()
		} else if a.SocSpecific() || a.DeviceSpecific() {
			prefix = cc.VendorVariationPrefix
			vndkVersion = deviceConfig.VndkVersion()
		} else if a.ProductSpecific() {
			prefix = cc.ProductVariationPrefix
			vndkVersion = deviceConfig.ProductVndkVersion()
		}
	}
	if vndkVersion == "current" {
		vndkVersion = deviceConfig.PlatformVndkVersion()
	}
	if vndkVersion != "" {
		return prefix + vndkVersion
	}

	return android.CoreVariation // The usual case
}

func (a *apexBundle) DepsMutator(ctx android.BottomUpMutatorContext) {
	// TODO(jiyong): move this kind of checks to GenerateAndroidBuildActions?
	checkUseVendorProperty(ctx, a)

	// apexBundle is a multi-arch targets module. Arch variant of apexBundle is set to 'common'.
	// arch-specific targets are enabled by the compile_multilib setting of the apex bundle. For
	// each target os/architectures, appropriate dependencies are selected by their
	// target.<os>.multilib.<type> groups and are added as (direct) dependencies.
	targets := ctx.MultiTargets()
	config := ctx.DeviceConfig()
	imageVariation := a.getImageVariation(ctx)

	a.combineProperties(ctx)

	has32BitTarget := false
	for _, target := range targets {
		if target.Arch.ArchType.Multilib == "lib32" {
			has32BitTarget = true
		}
	}
	for i, target := range targets {
		// Don't include artifacts for the host cross targets because there is no way for us
		// to run those artifacts natively on host
		if target.HostCross {
			continue
		}

		var depsList []ApexNativeDependencies

		// Add native modules targeting both ABIs. When multilib.* is omitted for
		// native_shared_libs/jni_libs/tests, it implies multilib.both
		depsList = append(depsList, a.properties.Multilib.Both)
		depsList = append(depsList, ApexNativeDependencies{
			Native_shared_libs: a.properties.Native_shared_libs,
			Tests:              a.properties.Tests,
			Jni_libs:           a.properties.Jni_libs,
			Binaries:           nil,
		})

		// Add native modules targeting the first ABI When multilib.* is omitted for
		// binaries, it implies multilib.first
		isPrimaryAbi := i == 0
		if isPrimaryAbi {
			depsList = append(depsList, a.properties.Multilib.First)
			depsList = append(depsList, ApexNativeDependencies{
				Native_shared_libs: nil,
				Tests:              nil,
				Jni_libs:           nil,
				Binaries:           a.properties.Binaries,
			})
		}

		// Add native modules targeting either 32-bit or 64-bit ABI
		switch target.Arch.ArchType.Multilib {
		case "lib32":
			depsList = append(depsList, a.properties.Multilib.Lib32)
			depsList = append(depsList, a.properties.Multilib.Prefer32)
		case "lib64":
			depsList = append(depsList, a.properties.Multilib.Lib64)
			if !has32BitTarget {
				depsList = append(depsList, a.properties.Multilib.Prefer32)
			}
		}

		// Add native modules targeting a specific arch variant
		switch target.Arch.ArchType {
		case android.Arm:
			depsList = append(depsList, a.archProperties.Arch.Arm.ApexNativeDependencies)
		case android.Arm64:
			depsList = append(depsList, a.archProperties.Arch.Arm64.ApexNativeDependencies)
		case android.X86:
			depsList = append(depsList, a.archProperties.Arch.X86.ApexNativeDependencies)
		case android.X86_64:
			depsList = append(depsList, a.archProperties.Arch.X86_64.ApexNativeDependencies)
		default:
			panic(fmt.Errorf("unsupported arch %v\n", ctx.Arch().ArchType))
		}

		for _, d := range depsList {
			addDependenciesForNativeModules(ctx, d, target, imageVariation)
		}
	}

	// For prebuilt_etc, use the first variant (64 on 64/32bit device, 32 on 32bit device)
	// regardless of the TARGET_PREFER_* setting. See b/144532908
	archForPrebuiltEtc := config.Arches()[0]
	for _, arch := range config.Arches() {
		// Prefer 64-bit arch if there is any
		if arch.ArchType.Multilib == "lib64" {
			archForPrebuiltEtc = arch
			break
		}
	}
	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "os", Variation: ctx.Os().String()},
		{Mutator: "arch", Variation: archForPrebuiltEtc.String()},
	}, prebuiltTag, a.properties.Prebuilts...)

	// Common-arch dependencies come next
	commonVariation := ctx.Config().AndroidCommonTarget.Variations()
	ctx.AddFarVariationDependencies(commonVariation, javaLibTag, a.properties.Java_libs...)
	ctx.AddFarVariationDependencies(commonVariation, bpfTag, a.properties.Bpfs...)
	ctx.AddFarVariationDependencies(commonVariation, fsTag, a.properties.Filesystems...)

	if a.artApex {
		// With EMMA_INSTRUMENT_FRAMEWORK=true the ART boot image includes jacoco library.
		if ctx.Config().IsEnvTrue("EMMA_INSTRUMENT_FRAMEWORK") {
			ctx.AddFarVariationDependencies(commonVariation, javaLibTag, "jacocoagent")
		}
		// The ART boot image depends on dex2oat to compile it.
		if !java.SkipDexpreoptBootJars(ctx) {
			dexpreopt.RegisterToolDeps(ctx)
		}
	}

	// Dependencies for signing
	if String(a.properties.Key) == "" {
		ctx.PropertyErrorf("key", "missing")
		return
	}
	ctx.AddDependency(ctx.Module(), keyTag, String(a.properties.Key))

	cert := android.SrcIsModule(a.getCertString(ctx))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
		// empty cert is not an error. Cert and private keys will be directly found under
		// PRODUCT_DEFAULT_DEV_CERTIFICATE
	}

	// Marks that this APEX (in fact all the modules in it) has to be built with the given SDKs.
	// This field currently isn't used.
	// TODO(jiyong): consider dropping this feature
	// TODO(jiyong): ensure that all apexes are with non-empty uses_sdks
	if len(a.properties.Uses_sdks) > 0 {
		sdkRefs := []android.SdkRef{}
		for _, str := range a.properties.Uses_sdks {
			parsed := android.ParseSdkRef(ctx, str, "uses_sdks")
			sdkRefs = append(sdkRefs, parsed)
		}
		a.BuildWithSdks(sdkRefs)
	}
}

// DepsMutator for the overridden properties.
func (a *apexBundle) OverridablePropertiesDepsMutator(ctx android.BottomUpMutatorContext) {
	if a.overridableProperties.Allowed_files != nil {
		android.ExtractSourceDeps(ctx, a.overridableProperties.Allowed_files)
	}

	commonVariation := ctx.Config().AndroidCommonTarget.Variations()
	ctx.AddFarVariationDependencies(commonVariation, androidAppTag, a.overridableProperties.Apps...)
	ctx.AddFarVariationDependencies(commonVariation, rroTag, a.overridableProperties.Rros...)
}

type ApexBundleInfo struct {
	Contents *android.ApexContents
}

var ApexBundleInfoProvider = blueprint.NewMutatorProvider(ApexBundleInfo{}, "apex_info")

var _ ApexInfoMutator = (*apexBundle)(nil)

// ApexInfoMutator is responsible for collecting modules that need to have apex variants. They are
// identified by doing a graph walk starting from an apexBundle. Basically, all the (direct and
// indirect) dependencies are collected. But a few types of modules that shouldn't be included in
// the apexBundle (e.g. stub libraries) are not collected. Note that a single module can be depended
// on by multiple apexBundles. In that case, the module is collected for all of the apexBundles.
//
// For each dependency between an apex and an ApexModule an ApexInfo object describing the apex
// is passed to that module's BuildForApex(ApexInfo) method which collates them all in a list.
// The apexMutator uses that list to create module variants for the apexes to which it belongs.
// The relationship between module variants and apexes is not one-to-one as variants will be
// shared between compatible apexes.
func (a *apexBundle) ApexInfoMutator(mctx android.TopDownMutatorContext) {

	// The VNDK APEX is special. For the APEX, the membership is described in a very different
	// way. There is no dependency from the VNDK APEX to the VNDK libraries. Instead, VNDK
	// libraries are self-identified by their vndk.enabled properties. There is no need to run
	// this mutator for the APEX as nothing will be collected. So, let's return fast.
	if a.vndkApex {
		return
	}

	// Special casing for APEXes on non-system (e.g., vendor, odm, etc.) partitions. They are
	// provided with a property named use_vndk_as_stable, which when set to true doesn't collect
	// VNDK libraries as transitive dependencies. This option is useful for reducing the size of
	// the non-system APEXes because the VNDK libraries won't be included (and duped) in the
	// APEX, but shared across APEXes via the VNDK APEX.
	useVndk := a.SocSpecific() || a.DeviceSpecific() || (a.ProductSpecific() && mctx.Config().EnforceProductPartitionInterface())
	excludeVndkLibs := useVndk && proptools.Bool(a.properties.Use_vndk_as_stable)
	if !useVndk && proptools.Bool(a.properties.Use_vndk_as_stable) {
		mctx.PropertyErrorf("use_vndk_as_stable", "not supported for system/system_ext APEXes")
		return
	}

	continueApexDepsWalk := func(child, parent android.Module) bool {
		am, ok := child.(android.ApexModule)
		if !ok || !am.CanHaveApexVariants() {
			return false
		}
		if !parent.(android.DepIsInSameApex).DepIsInSameApex(mctx, child) {
			return false
		}
		if excludeVndkLibs {
			if c, ok := child.(*cc.Module); ok && c.IsVndk() {
				return false
			}
		}
		// By default, all the transitive dependencies are collected, unless filtered out
		// above.
		return true
	}

	// Records whether a certain module is included in this apexBundle via direct dependency or
	// inndirect dependency.
	contents := make(map[string]android.ApexMembership)
	mctx.WalkDeps(func(child, parent android.Module) bool {
		if !continueApexDepsWalk(child, parent) {
			return false
		}
		// If the parent is apexBundle, this child is directly depended.
		_, directDep := parent.(*apexBundle)
		depName := mctx.OtherModuleName(child)
		contents[depName] = contents[depName].Add(directDep)
		return true
	})

	// The membership information is saved for later access
	apexContents := android.NewApexContents(contents)
	mctx.SetProvider(ApexBundleInfoProvider, ApexBundleInfo{
		Contents: apexContents,
	})

	minSdkVersion := a.minSdkVersion(mctx)
	// When min_sdk_version is not set, the apex is built against FutureApiLevel.
	if minSdkVersion.IsNone() {
		minSdkVersion = android.FutureApiLevel
	}

	// This is the main part of this mutator. Mark the collected dependencies that they need to
	// be built for this apexBundle.
	apexInfo := android.ApexInfo{
		ApexVariationName: mctx.ModuleName(),
		MinSdkVersionStr:  minSdkVersion.String(),
		RequiredSdks:      a.RequiredSdks(),
		Updatable:         a.Updatable(),
		InApexes:          []string{mctx.ModuleName()},
		ApexContents:      []*android.ApexContents{apexContents},
	}
	mctx.WalkDeps(func(child, parent android.Module) bool {
		if !continueApexDepsWalk(child, parent) {
			return false
		}
		child.(android.ApexModule).BuildForApex(apexInfo) // leave a mark!
		return true
	})
}

type ApexInfoMutator interface {
	// ApexInfoMutator implementations must call BuildForApex(ApexInfo) on any modules that are
	// depended upon by an apex and which require an apex specific variant.
	ApexInfoMutator(android.TopDownMutatorContext)
}

// apexInfoMutator delegates the work of identifying which modules need an ApexInfo and apex
// specific variant to modules that support the ApexInfoMutator.
func apexInfoMutator(mctx android.TopDownMutatorContext) {
	if !mctx.Module().Enabled() {
		return
	}

	if a, ok := mctx.Module().(ApexInfoMutator); ok {
		a.ApexInfoMutator(mctx)
		return
	}
}

// apexUniqueVariationsMutator checks if any dependencies use unique apex variations. If so, use
// unique apex variations for this module. See android/apex.go for more about unique apex variant.
// TODO(jiyong): move this to android/apex.go?
func apexUniqueVariationsMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Module().Enabled() {
		return
	}
	if am, ok := mctx.Module().(android.ApexModule); ok {
		android.UpdateUniqueApexVariationsForDeps(mctx, am)
	}
}

// apexTestForDepsMutator checks if this module is a test for an apex. If so, add a dependency on
// the apex in order to retrieve its contents later.
// TODO(jiyong): move this to android/apex.go?
func apexTestForDepsMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Module().Enabled() {
		return
	}
	if am, ok := mctx.Module().(android.ApexModule); ok {
		if testFor := am.TestFor(); len(testFor) > 0 {
			mctx.AddFarVariationDependencies([]blueprint.Variation{
				{Mutator: "os", Variation: am.Target().OsVariation()},
				{"arch", "common"},
			}, testForTag, testFor...)
		}
	}
}

// TODO(jiyong): move this to android/apex.go?
func apexTestForMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Module().Enabled() {
		return
	}
	if _, ok := mctx.Module().(android.ApexModule); ok {
		var contents []*android.ApexContents
		for _, testFor := range mctx.GetDirectDepsWithTag(testForTag) {
			abInfo := mctx.OtherModuleProvider(testFor, ApexBundleInfoProvider).(ApexBundleInfo)
			contents = append(contents, abInfo.Contents)
		}
		mctx.SetProvider(android.ApexTestForInfoProvider, android.ApexTestForInfo{
			ApexContents: contents,
		})
	}
}

// markPlatformAvailability marks whether or not a module can be available to platform. A module
// cannot be available to platform if 1) it is explicitly marked as not available (i.e.
// "//apex_available:platform" is absent) or 2) it depends on another module that isn't (or can't
// be) available to platform
// TODO(jiyong): move this to android/apex.go?
func markPlatformAvailability(mctx android.BottomUpMutatorContext) {
	// Host and recovery are not considered as platform
	if mctx.Host() || mctx.Module().InstallInRecovery() {
		return
	}

	am, ok := mctx.Module().(android.ApexModule)
	if !ok {
		return
	}

	availableToPlatform := am.AvailableFor(android.AvailableToPlatform)

	// If any of the dep is not available to platform, this module is also considered as being
	// not available to platform even if it has "//apex_available:platform"
	mctx.VisitDirectDeps(func(child android.Module) {
		if !am.DepIsInSameApex(mctx, child) {
			// if the dependency crosses apex boundary, don't consider it
			return
		}
		if dep, ok := child.(android.ApexModule); ok && dep.NotAvailableForPlatform() {
			availableToPlatform = false
			// TODO(b/154889534) trigger an error when 'am' has
			// "//apex_available:platform"
		}
	})

	// Exception 1: stub libraries and native bridge libraries are always available to platform
	if cc, ok := mctx.Module().(*cc.Module); ok &&
		(cc.IsStubs() || cc.Target().NativeBridge == android.NativeBridgeEnabled) {
		availableToPlatform = true
	}

	// Exception 2: bootstrap bionic libraries are also always available to platform
	if cc.InstallToBootstrap(mctx.ModuleName(), mctx.Config()) {
		availableToPlatform = true
	}

	if !availableToPlatform {
		am.SetNotAvailableForPlatform()
	}
}

// apexMutator visits each module and creates apex variations if the module was marked in the
// previous run of apexInfoMutator.
func apexMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Module().Enabled() {
		return
	}

	// This is the usual path.
	if am, ok := mctx.Module().(android.ApexModule); ok && am.CanHaveApexVariants() {
		android.CreateApexVariations(mctx, am)
		return
	}

	// apexBundle itself is mutated so that it and its dependencies have the same apex variant.
	// TODO(jiyong): document the reason why the VNDK APEX is an exception here.
	if a, ok := mctx.Module().(*apexBundle); ok && !a.vndkApex {
		apexBundleName := mctx.ModuleName()
		mctx.CreateVariations(apexBundleName)
	} else if o, ok := mctx.Module().(*OverrideApex); ok {
		apexBundleName := o.GetOverriddenModuleName()
		if apexBundleName == "" {
			mctx.ModuleErrorf("base property is not set")
			return
		}
		mctx.CreateVariations(apexBundleName)
	}
}

// See android.UpdateDirectlyInAnyApex
// TODO(jiyong): move this to android/apex.go?
func apexDirectlyInAnyMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Module().Enabled() {
		return
	}
	if am, ok := mctx.Module().(android.ApexModule); ok {
		android.UpdateDirectlyInAnyApex(mctx, am)
	}
}

// apexPackaging represents a specific packaging method for an APEX.
type apexPackaging int

const (
	// imageApex is a packaging method where contents are included in a filesystem image which
	// is then included in a zip container. This is the most typical way of packaging.
	imageApex apexPackaging = iota

	// zipApex is a packaging method where contents are directly included in the zip container.
	// This is used for host-side testing - because the contents are easily accessible by
	// unzipping the container.
	zipApex

	// flattendApex is a packaging method where contents are not included in the APEX file, but
	// installed to /apex/<apexname> directory on the device. This packaging method is used for
	// old devices where the filesystem-based APEX file can't be supported.
	flattenedApex
)

const (
	// File extensions of an APEX for different packaging methods
	imageApexSuffix = ".apex"
	zipApexSuffix   = ".zipapex"
	flattenedSuffix = ".flattened"

	// variant names each of which is for a packaging method
	imageApexType     = "image"
	zipApexType       = "zip"
	flattenedApexType = "flattened"

	ext4FsType = "ext4"
	f2fsFsType = "f2fs"
)

// The suffix for the output "file", not the module
func (a apexPackaging) suffix() string {
	switch a {
	case imageApex:
		return imageApexSuffix
	case zipApex:
		return zipApexSuffix
	default:
		panic(fmt.Errorf("unknown APEX type %d", a))
	}
}

func (a apexPackaging) name() string {
	switch a {
	case imageApex:
		return imageApexType
	case zipApex:
		return zipApexType
	default:
		panic(fmt.Errorf("unknown APEX type %d", a))
	}
}

// apexFlattenedMutator creates one or more variations each of which is for a packaging method.
// TODO(jiyong): give a better name to this mutator
func apexFlattenedMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Module().Enabled() {
		return
	}
	if ab, ok := mctx.Module().(*apexBundle); ok {
		var variants []string
		switch proptools.StringDefault(ab.properties.Payload_type, "image") {
		case "image":
			// This is the normal case. Note that both image and flattend APEXes are
			// created. The image type is installed to the system partition, while the
			// flattened APEX is (optionally) installed to the system_ext partition.
			// This is mostly for GSI which has to support wide range of devices. If GSI
			// is installed on a newer (APEX-capable) device, the image APEX in the
			// system will be used. However, if the same GSI is installed on an old
			// device which can't support image APEX, the flattened APEX in the
			// system_ext partion (which still is part of GSI) is used instead.
			variants = append(variants, imageApexType, flattenedApexType)
		case "zip":
			variants = append(variants, zipApexType)
		case "both":
			variants = append(variants, imageApexType, zipApexType, flattenedApexType)
		default:
			mctx.PropertyErrorf("payload_type", "%q is not one of \"image\", \"zip\", or \"both\".", *ab.properties.Payload_type)
			return
		}

		modules := mctx.CreateLocalVariations(variants...)

		for i, v := range variants {
			switch v {
			case imageApexType:
				modules[i].(*apexBundle).properties.ApexType = imageApex
			case zipApexType:
				modules[i].(*apexBundle).properties.ApexType = zipApex
			case flattenedApexType:
				modules[i].(*apexBundle).properties.ApexType = flattenedApex
				// See the comment above for why system_ext.
				if !mctx.Config().FlattenApex() && ab.Platform() {
					modules[i].(*apexBundle).MakeAsSystemExt()
				}
			}
		}
	} else if _, ok := mctx.Module().(*OverrideApex); ok {
		// payload_type is forcibly overridden to "image"
		// TODO(jiyong): is this the right decision?
		mctx.CreateVariations(imageApexType, flattenedApexType)
	}
}

// checkUseVendorProperty checks if the use of `use_vendor` property is allowed for the given APEX.
// When use_vendor is used, native modules are built with __ANDROID_VNDK__ and __ANDROID_APEX__,
// which may cause compatibility issues. (e.g. libbinder) Even though libbinder restricts its
// availability via 'apex_available' property and relies on yet another macro
// __ANDROID_APEX_<NAME>__, we restrict usage of "use_vendor:" from other APEX modules to avoid
// similar problems.
func checkUseVendorProperty(ctx android.BottomUpMutatorContext, a *apexBundle) {
	if proptools.Bool(a.properties.Use_vendor) && !android.InList(a.Name(), useVendorAllowList(ctx.Config())) {
		ctx.PropertyErrorf("use_vendor", "not allowed to set use_vendor: true")
	}
}

var (
	useVendorAllowListKey = android.NewOnceKey("useVendorAllowList")
)

func useVendorAllowList(config android.Config) []string {
	return config.Once(useVendorAllowListKey, func() interface{} {
		return []string{
			// swcodec uses "vendor" variants for smaller size
			"com.android.media.swcodec",
			"test_com.android.media.swcodec",
		}
	}).([]string)
}

// setUseVendorAllowListForTest overrides useVendorAllowList and must be called before the first
// call to useVendorAllowList()
func setUseVendorAllowListForTest(config android.Config, allowList []string) {
	config.Once(useVendorAllowListKey, func() interface{} {
		return allowList
	})
}

var _ android.DepIsInSameApex = (*apexBundle)(nil)

// Implements android.DepInInSameApex
func (a *apexBundle) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	// direct deps of an APEX bundle are all part of the APEX bundle
	// TODO(jiyong): shouldn't we look into the payload field of the dependencyTag?
	return true
}

var _ android.OutputFileProducer = (*apexBundle)(nil)

// Implements android.OutputFileProducer
func (a *apexBundle) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "", android.DefaultDistTag:
		// This is the default dist path.
		return android.Paths{a.outputFile}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

var _ cc.Coverage = (*apexBundle)(nil)

// Implements cc.Coverage
func (a *apexBundle) IsNativeCoverageNeeded(ctx android.BaseModuleContext) bool {
	return ctx.Device() && ctx.DeviceConfig().NativeCoverageEnabled()
}

// Implements cc.Coverage
func (a *apexBundle) PreventInstall() {
	a.properties.PreventInstall = true
}

// Implements cc.Coverage
func (a *apexBundle) HideFromMake() {
	a.properties.HideFromMake = true
	// This HideFromMake is shadowing the ModuleBase one, call through to it for now.
	// TODO(ccross): untangle these
	a.ModuleBase.HideFromMake()
}

// Implements cc.Coverage
func (a *apexBundle) MarkAsCoverageVariant(coverage bool) {
	a.properties.IsCoverageVariant = coverage
}

// Implements cc.Coverage
func (a *apexBundle) EnableCoverageIfNeeded() {}

var _ android.ApexBundleDepsInfoIntf = (*apexBundle)(nil)

// Implements android.ApexBudleDepsInfoIntf
func (a *apexBundle) Updatable() bool {
	return proptools.Bool(a.properties.Updatable)
}

// getCertString returns the name of the cert that should be used to sign this APEX. This is
// basically from the "certificate" property, but could be overridden by the device config.
func (a *apexBundle) getCertString(ctx android.BaseModuleContext) string {
	moduleName := ctx.ModuleName()
	// VNDK APEXes share the same certificate. To avoid adding a new VNDK version to the
	// OVERRIDE_* list, we check with the pseudo module name to see if its certificate is
	// overridden.
	if a.vndkApex {
		moduleName = vndkApexName
	}
	certificate, overridden := ctx.DeviceConfig().OverrideCertificateFor(moduleName)
	if overridden {
		return ":" + certificate
	}
	return String(a.properties.Certificate)
}

// See the installable property
func (a *apexBundle) installable() bool {
	return !a.properties.PreventInstall && (a.properties.Installable == nil || proptools.Bool(a.properties.Installable))
}

// See the test_only_no_hashtree property
func (a *apexBundle) testOnlyShouldSkipHashtreeGeneration() bool {
	return proptools.Bool(a.properties.Test_only_no_hashtree)
}

// See the test_only_unsigned_payload property
func (a *apexBundle) testOnlyShouldSkipPayloadSign() bool {
	return proptools.Bool(a.properties.Test_only_unsigned_payload)
}

// See the test_only_force_compression property
func (a *apexBundle) testOnlyShouldForceCompression() bool {
	return proptools.Bool(a.properties.Test_only_force_compression)
}

// These functions are interfacing with cc/sanitizer.go. The entire APEX (along with all of its
// members) can be sanitized, either forcibly, or by the global configuration. For some of the
// sanitizers, extra dependencies can be forcibly added as well.

func (a *apexBundle) EnableSanitizer(sanitizerName string) {
	if !android.InList(sanitizerName, a.properties.SanitizerNames) {
		a.properties.SanitizerNames = append(a.properties.SanitizerNames, sanitizerName)
	}
}

func (a *apexBundle) IsSanitizerEnabled(ctx android.BaseModuleContext, sanitizerName string) bool {
	if android.InList(sanitizerName, a.properties.SanitizerNames) {
		return true
	}

	// Then follow the global setting
	globalSanitizerNames := []string{}
	if a.Host() {
		globalSanitizerNames = ctx.Config().SanitizeHost()
	} else {
		arches := ctx.Config().SanitizeDeviceArch()
		if len(arches) == 0 || android.InList(a.Arch().ArchType.Name, arches) {
			globalSanitizerNames = ctx.Config().SanitizeDevice()
		}
	}
	return android.InList(sanitizerName, globalSanitizerNames)
}

func (a *apexBundle) AddSanitizerDependencies(ctx android.BottomUpMutatorContext, sanitizerName string) {
	// TODO(jiyong): move this info (the sanitizer name, the lib name, etc.) to cc/sanitize.go
	// Keep only the mechanism here.
	if ctx.Device() && sanitizerName == "hwaddress" && strings.HasPrefix(a.Name(), "com.android.runtime") {
		imageVariation := a.getImageVariation(ctx)
		for _, target := range ctx.MultiTargets() {
			if target.Arch.ArchType.Multilib == "lib64" {
				addDependenciesForNativeModules(ctx, ApexNativeDependencies{
					Native_shared_libs: []string{"libclang_rt.hwasan-aarch64-android"},
					Tests:              nil,
					Jni_libs:           nil,
					Binaries:           nil,
				}, target, imageVariation)
				break
			}
		}
	}
}

// apexFileFor<Type> functions below create an apexFile struct for a given Soong module. The
// returned apexFile saves information about the Soong module that will be used for creating the
// build rules.
func apexFileForNativeLibrary(ctx android.BaseModuleContext, ccMod *cc.Module, handleSpecialLibs bool) apexFile {
	// Decide the APEX-local directory by the multilib of the library In the future, we may
	// query this to the module.
	// TODO(jiyong): use the new PackagingSpec
	var dirInApex string
	switch ccMod.Arch().ArchType.Multilib {
	case "lib32":
		dirInApex = "lib"
	case "lib64":
		dirInApex = "lib64"
	}
	if ccMod.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, ccMod.Target().NativeBridgeRelativePath)
	}
	dirInApex = filepath.Join(dirInApex, ccMod.RelativeInstallPath())
	if handleSpecialLibs && cc.InstallToBootstrap(ccMod.BaseModuleName(), ctx.Config()) {
		// Special case for Bionic libs and other libs installed with them. This is to
		// prevent those libs from being included in the search path
		// /apex/com.android.runtime/${LIB}. This exclusion is required because those libs
		// in the Runtime APEX are available via the legacy paths in /system/lib/. By the
		// init process, the libs in the APEX are bind-mounted to the legacy paths and thus
		// will be loaded into the default linker namespace (aka "platform" namespace). If
		// the libs are directly in /apex/com.android.runtime/${LIB} then the same libs will
		// be loaded again into the runtime linker namespace, which will result in double
		// loading of them, which isn't supported.
		dirInApex = filepath.Join(dirInApex, "bionic")
	}

	fileToCopy := ccMod.OutputFile().Path()
	androidMkModuleName := ccMod.BaseModuleName() + ccMod.Properties.SubName
	return newApexFile(ctx, fileToCopy, androidMkModuleName, dirInApex, nativeSharedLib, ccMod)
}

func apexFileForExecutable(ctx android.BaseModuleContext, cc *cc.Module) apexFile {
	dirInApex := "bin"
	if cc.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, cc.Target().NativeBridgeRelativePath)
	}
	dirInApex = filepath.Join(dirInApex, cc.RelativeInstallPath())
	fileToCopy := cc.OutputFile().Path()
	androidMkModuleName := cc.BaseModuleName() + cc.Properties.SubName
	af := newApexFile(ctx, fileToCopy, androidMkModuleName, dirInApex, nativeExecutable, cc)
	af.symlinks = cc.Symlinks()
	af.dataPaths = cc.DataPaths()
	return af
}

func apexFileForRustExecutable(ctx android.BaseModuleContext, rustm *rust.Module) apexFile {
	dirInApex := "bin"
	if rustm.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, rustm.Target().NativeBridgeRelativePath)
	}
	fileToCopy := rustm.OutputFile().Path()
	androidMkModuleName := rustm.BaseModuleName() + rustm.Properties.SubName
	af := newApexFile(ctx, fileToCopy, androidMkModuleName, dirInApex, nativeExecutable, rustm)
	return af
}

func apexFileForRustLibrary(ctx android.BaseModuleContext, rustm *rust.Module) apexFile {
	// Decide the APEX-local directory by the multilib of the library
	// In the future, we may query this to the module.
	var dirInApex string
	switch rustm.Arch().ArchType.Multilib {
	case "lib32":
		dirInApex = "lib"
	case "lib64":
		dirInApex = "lib64"
	}
	if rustm.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, rustm.Target().NativeBridgeRelativePath)
	}
	fileToCopy := rustm.OutputFile().Path()
	androidMkModuleName := rustm.BaseModuleName() + rustm.Properties.SubName
	return newApexFile(ctx, fileToCopy, androidMkModuleName, dirInApex, nativeSharedLib, rustm)
}

func apexFileForPyBinary(ctx android.BaseModuleContext, py *python.Module) apexFile {
	dirInApex := "bin"
	fileToCopy := py.HostToolPath().Path()
	return newApexFile(ctx, fileToCopy, py.BaseModuleName(), dirInApex, pyBinary, py)
}

func apexFileForGoBinary(ctx android.BaseModuleContext, depName string, gb bootstrap.GoBinaryTool) apexFile {
	dirInApex := "bin"
	s, err := filepath.Rel(android.PathForOutput(ctx).String(), gb.InstallPath())
	if err != nil {
		ctx.ModuleErrorf("Unable to use compiled binary at %s", gb.InstallPath())
		return apexFile{}
	}
	fileToCopy := android.PathForOutput(ctx, s)
	// NB: Since go binaries are static we don't need the module for anything here, which is
	// good since the go tool is a blueprint.Module not an android.Module like we would
	// normally use.
	return newApexFile(ctx, fileToCopy, depName, dirInApex, goBinary, nil)
}

func apexFileForShBinary(ctx android.BaseModuleContext, sh *sh.ShBinary) apexFile {
	dirInApex := filepath.Join("bin", sh.SubDir())
	fileToCopy := sh.OutputFile()
	af := newApexFile(ctx, fileToCopy, sh.BaseModuleName(), dirInApex, shBinary, sh)
	af.symlinks = sh.Symlinks()
	return af
}

func apexFileForPrebuiltEtc(ctx android.BaseModuleContext, prebuilt prebuilt_etc.PrebuiltEtcModule, depName string) apexFile {
	dirInApex := filepath.Join(prebuilt.BaseDir(), prebuilt.SubDir())
	fileToCopy := prebuilt.OutputFile()
	return newApexFile(ctx, fileToCopy, depName, dirInApex, etc, prebuilt)
}

func apexFileForCompatConfig(ctx android.BaseModuleContext, config java.PlatformCompatConfigIntf, depName string) apexFile {
	dirInApex := filepath.Join("etc", config.SubDir())
	fileToCopy := config.CompatConfig()
	return newApexFile(ctx, fileToCopy, depName, dirInApex, etc, config)
}

// javaModule is an interface to handle all Java modules (java_library, dex_import, etc) in the same
// way.
type javaModule interface {
	android.Module
	BaseModuleName() string
	DexJarBuildPath() android.Path
	JacocoReportClassesFile() android.Path
	LintDepSets() java.LintDepSets
	Stem() string
}

var _ javaModule = (*java.Library)(nil)
var _ javaModule = (*java.Import)(nil)
var _ javaModule = (*java.SdkLibrary)(nil)
var _ javaModule = (*java.DexImport)(nil)
var _ javaModule = (*java.SdkLibraryImport)(nil)

func apexFileForJavaModule(ctx android.BaseModuleContext, module javaModule) apexFile {
	dirInApex := "javalib"
	fileToCopy := module.DexJarBuildPath()
	af := newApexFile(ctx, fileToCopy, module.BaseModuleName(), dirInApex, javaSharedLib, module)
	af.jacocoReportClassesFile = module.JacocoReportClassesFile()
	af.lintDepSets = module.LintDepSets()
	af.customStem = module.Stem() + ".jar"
	return af
}

// androidApp is an interface to handle all app modules (android_app, android_app_import, etc.) in
// the same way.
type androidApp interface {
	android.Module
	Privileged() bool
	InstallApkName() string
	OutputFile() android.Path
	JacocoReportClassesFile() android.Path
	Certificate() java.Certificate
	BaseModuleName() string
}

var _ androidApp = (*java.AndroidApp)(nil)
var _ androidApp = (*java.AndroidAppImport)(nil)

func apexFileForAndroidApp(ctx android.BaseModuleContext, aapp androidApp) apexFile {
	appDir := "app"
	if aapp.Privileged() {
		appDir = "priv-app"
	}
	dirInApex := filepath.Join(appDir, aapp.InstallApkName())
	fileToCopy := aapp.OutputFile()
	af := newApexFile(ctx, fileToCopy, aapp.BaseModuleName(), dirInApex, app, aapp)
	af.jacocoReportClassesFile = aapp.JacocoReportClassesFile()
	af.certificate = aapp.Certificate()

	if app, ok := aapp.(interface {
		OverriddenManifestPackageName() string
	}); ok {
		af.overriddenPackageName = app.OverriddenManifestPackageName()
	}
	return af
}

func apexFileForRuntimeResourceOverlay(ctx android.BaseModuleContext, rro java.RuntimeResourceOverlayModule) apexFile {
	rroDir := "overlay"
	dirInApex := filepath.Join(rroDir, rro.Theme())
	fileToCopy := rro.OutputFile()
	af := newApexFile(ctx, fileToCopy, rro.Name(), dirInApex, app, rro)
	af.certificate = rro.Certificate()

	if a, ok := rro.(interface {
		OverriddenManifestPackageName() string
	}); ok {
		af.overriddenPackageName = a.OverriddenManifestPackageName()
	}
	return af
}

func apexFileForBpfProgram(ctx android.BaseModuleContext, builtFile android.Path, bpfProgram bpf.BpfModule) apexFile {
	dirInApex := filepath.Join("etc", "bpf")
	return newApexFile(ctx, builtFile, builtFile.Base(), dirInApex, etc, bpfProgram)
}

func apexFileForFilesystem(ctx android.BaseModuleContext, buildFile android.Path, fs filesystem.Filesystem) apexFile {
	dirInApex := filepath.Join("etc", "fs")
	return newApexFile(ctx, buildFile, buildFile.Base(), dirInApex, etc, fs)
}

// WalkPayloadDeps visits dependencies that contributes to the payload of this APEX. For each of the
// visited module, the `do` callback is executed. Returning true in the callback continues the visit
// to the child modules. Returning false makes the visit to continue in the sibling or the parent
// modules. This is used in check* functions below.
func (a *apexBundle) WalkPayloadDeps(ctx android.ModuleContext, do android.PayloadDepsCallback) {
	ctx.WalkDeps(func(child, parent android.Module) bool {
		am, ok := child.(android.ApexModule)
		if !ok || !am.CanHaveApexVariants() {
			return false
		}

		// Filter-out unwanted depedendencies
		depTag := ctx.OtherModuleDependencyTag(child)
		if _, ok := depTag.(android.ExcludeFromApexContentsTag); ok {
			return false
		}
		if dt, ok := depTag.(dependencyTag); ok && !dt.payload {
			return false
		}
		if depTag == dexpreopt.Dex2oatDepTag {
			return false
		}

		ai := ctx.OtherModuleProvider(child, android.ApexInfoProvider).(android.ApexInfo)
		externalDep := !android.InList(ctx.ModuleName(), ai.InApexes)

		// Visit actually
		return do(ctx, parent, am, externalDep)
	})
}

// filesystem type of the apex_payload.img inside the APEX. Currently, ext4 and f2fs are supported.
type fsType int

const (
	ext4 fsType = iota
	f2fs
)

func (f fsType) string() string {
	switch f {
	case ext4:
		return ext4FsType
	case f2fs:
		return f2fsFsType
	default:
		panic(fmt.Errorf("unknown APEX payload type %d", f))
	}
}

// Creates build rules for an APEX. It consists of the following major steps:
//
// 1) do some validity checks such as apex_available, min_sdk_version, etc.
// 2) traverse the dependency tree to collect apexFile structs from them.
// 3) some fields in apexBundle struct are configured
// 4) generate the build rules to create the APEX. This is mostly done in builder.go.
func (a *apexBundle) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	////////////////////////////////////////////////////////////////////////////////////////////
	// 1) do some validity checks such as apex_available, min_sdk_version, etc.
	a.checkApexAvailability(ctx)
	a.checkUpdatable(ctx)
	a.checkMinSdkVersion(ctx)
	a.checkStaticLinkingToStubLibraries(ctx)
	if len(a.properties.Tests) > 0 && !a.testApex {
		ctx.PropertyErrorf("tests", "property allowed only in apex_test module type")
		return
	}

	////////////////////////////////////////////////////////////////////////////////////////////
	// 2) traverse the dependency tree to collect apexFile structs from them.

	// all the files that will be included in this APEX
	var filesInfo []apexFile

	// native lib dependencies
	var provideNativeLibs []string
	var requireNativeLibs []string

	handleSpecialLibs := !android.Bool(a.properties.Ignore_system_library_special_case)

	// TODO(jiyong): do this using WalkPayloadDeps
	// TODO(jiyong): make this clean!!!
	ctx.WalkDepsBlueprint(func(child, parent blueprint.Module) bool {
		depTag := ctx.OtherModuleDependencyTag(child)
		if _, ok := depTag.(android.ExcludeFromApexContentsTag); ok {
			return false
		}
		depName := ctx.OtherModuleName(child)
		if _, isDirectDep := parent.(*apexBundle); isDirectDep {
			switch depTag {
			case sharedLibTag, jniLibTag:
				isJniLib := depTag == jniLibTag
				if c, ok := child.(*cc.Module); ok {
					fi := apexFileForNativeLibrary(ctx, c, handleSpecialLibs)
					fi.isJniLib = isJniLib
					filesInfo = append(filesInfo, fi)
					// Collect the list of stub-providing libs except:
					// - VNDK libs are only for vendors
					// - bootstrap bionic libs are treated as provided by system
					if c.HasStubsVariants() && !a.vndkApex && !cc.InstallToBootstrap(c.BaseModuleName(), ctx.Config()) {
						provideNativeLibs = append(provideNativeLibs, fi.stem())
					}
					return true // track transitive dependencies
				} else if r, ok := child.(*rust.Module); ok {
					fi := apexFileForRustLibrary(ctx, r)
					filesInfo = append(filesInfo, fi)
				} else {
					propertyName := "native_shared_libs"
					if isJniLib {
						propertyName = "jni_libs"
					}
					ctx.PropertyErrorf(propertyName, "%q is not a cc_library or cc_library_shared module", depName)
				}
			case executableTag:
				if cc, ok := child.(*cc.Module); ok {
					filesInfo = append(filesInfo, apexFileForExecutable(ctx, cc))
					return true // track transitive dependencies
				} else if sh, ok := child.(*sh.ShBinary); ok {
					filesInfo = append(filesInfo, apexFileForShBinary(ctx, sh))
				} else if py, ok := child.(*python.Module); ok && py.HostToolPath().Valid() {
					filesInfo = append(filesInfo, apexFileForPyBinary(ctx, py))
				} else if gb, ok := child.(bootstrap.GoBinaryTool); ok && a.Host() {
					filesInfo = append(filesInfo, apexFileForGoBinary(ctx, depName, gb))
				} else if rust, ok := child.(*rust.Module); ok {
					filesInfo = append(filesInfo, apexFileForRustExecutable(ctx, rust))
					return true // track transitive dependencies
				} else {
					ctx.PropertyErrorf("binaries", "%q is neither cc_binary, rust_binary, (embedded) py_binary, (host) blueprint_go_binary, (host) bootstrap_go_binary, nor sh_binary", depName)
				}
			case javaLibTag:
				switch child.(type) {
				case *java.Library, *java.SdkLibrary, *java.DexImport, *java.SdkLibraryImport, *java.Import:
					af := apexFileForJavaModule(ctx, child.(javaModule))
					if !af.ok() {
						ctx.PropertyErrorf("java_libs", "%q is not configured to be compiled into dex", depName)
						return false
					}
					filesInfo = append(filesInfo, af)
					return true // track transitive dependencies
				default:
					ctx.PropertyErrorf("java_libs", "%q of type %q is not supported", depName, ctx.OtherModuleType(child))
				}
			case androidAppTag:
				if ap, ok := child.(*java.AndroidApp); ok {
					filesInfo = append(filesInfo, apexFileForAndroidApp(ctx, ap))
					return true // track transitive dependencies
				} else if ap, ok := child.(*java.AndroidAppImport); ok {
					filesInfo = append(filesInfo, apexFileForAndroidApp(ctx, ap))
				} else if ap, ok := child.(*java.AndroidTestHelperApp); ok {
					filesInfo = append(filesInfo, apexFileForAndroidApp(ctx, ap))
				} else if ap, ok := child.(*java.AndroidAppSet); ok {
					appDir := "app"
					if ap.Privileged() {
						appDir = "priv-app"
					}
					af := newApexFile(ctx, ap.OutputFile(), ap.BaseModuleName(),
						filepath.Join(appDir, ap.BaseModuleName()), appSet, ap)
					af.certificate = java.PresignedCertificate
					filesInfo = append(filesInfo, af)
				} else {
					ctx.PropertyErrorf("apps", "%q is not an android_app module", depName)
				}
			case rroTag:
				if rro, ok := child.(java.RuntimeResourceOverlayModule); ok {
					filesInfo = append(filesInfo, apexFileForRuntimeResourceOverlay(ctx, rro))
				} else {
					ctx.PropertyErrorf("rros", "%q is not an runtime_resource_overlay module", depName)
				}
			case bpfTag:
				if bpfProgram, ok := child.(bpf.BpfModule); ok {
					filesToCopy, _ := bpfProgram.OutputFiles("")
					for _, bpfFile := range filesToCopy {
						filesInfo = append(filesInfo, apexFileForBpfProgram(ctx, bpfFile, bpfProgram))
					}
				} else {
					ctx.PropertyErrorf("bpfs", "%q is not a bpf module", depName)
				}
			case fsTag:
				if fs, ok := child.(filesystem.Filesystem); ok {
					filesInfo = append(filesInfo, apexFileForFilesystem(ctx, fs.OutputPath(), fs))
				} else {
					ctx.PropertyErrorf("filesystems", "%q is not a filesystem module", depName)
				}
			case prebuiltTag:
				if prebuilt, ok := child.(prebuilt_etc.PrebuiltEtcModule); ok {
					filesInfo = append(filesInfo, apexFileForPrebuiltEtc(ctx, prebuilt, depName))
				} else if prebuilt, ok := child.(java.PlatformCompatConfigIntf); ok {
					filesInfo = append(filesInfo, apexFileForCompatConfig(ctx, prebuilt, depName))
				} else {
					ctx.PropertyErrorf("prebuilts", "%q is not a prebuilt_etc and not a platform_compat_config module", depName)
				}
			case testTag:
				if ccTest, ok := child.(*cc.Module); ok {
					if ccTest.IsTestPerSrcAllTestsVariation() {
						// Multiple-output test module (where `test_per_src: true`).
						//
						// `ccTest` is the "" ("all tests") variation of a `test_per_src` module.
						// We do not add this variation to `filesInfo`, as it has no output;
						// however, we do add the other variations of this module as indirect
						// dependencies (see below).
					} else {
						// Single-output test module (where `test_per_src: false`).
						af := apexFileForExecutable(ctx, ccTest)
						af.class = nativeTest
						filesInfo = append(filesInfo, af)
					}
					return true // track transitive dependencies
				} else {
					ctx.PropertyErrorf("tests", "%q is not a cc module", depName)
				}
			case keyTag:
				if key, ok := child.(*apexKey); ok {
					a.privateKeyFile = key.privateKeyFile
					a.publicKeyFile = key.publicKeyFile
				} else {
					ctx.PropertyErrorf("key", "%q is not an apex_key module", depName)
				}
				return false
			case certificateTag:
				if dep, ok := child.(*java.AndroidAppCertificate); ok {
					a.containerCertificateFile = dep.Certificate.Pem
					a.containerPrivateKeyFile = dep.Certificate.Key
				} else {
					ctx.ModuleErrorf("certificate dependency %q must be an android_app_certificate module", depName)
				}
			case android.PrebuiltDepTag:
				// If the prebuilt is force disabled, remember to delete the prebuilt file
				// that might have been installed in the previous builds
				if prebuilt, ok := child.(prebuilt); ok && prebuilt.isForceDisabled() {
					a.prebuiltFileToDelete = prebuilt.InstallFilename()
				}
			}
		} else if !a.vndkApex {
			// indirect dependencies
			if am, ok := child.(android.ApexModule); ok {
				// We cannot use a switch statement on `depTag` here as the checked
				// tags used below are private (e.g. `cc.sharedDepTag`).
				if cc.IsSharedDepTag(depTag) || cc.IsRuntimeDepTag(depTag) {
					if cc, ok := child.(*cc.Module); ok {
						if cc.UseVndk() && proptools.Bool(a.properties.Use_vndk_as_stable) && cc.IsVndk() {
							requireNativeLibs = append(requireNativeLibs, ":vndk")
							return false
						}
						af := apexFileForNativeLibrary(ctx, cc, handleSpecialLibs)
						af.transitiveDep = true

						// Always track transitive dependencies for host.
						if a.Host() {
							filesInfo = append(filesInfo, af)
							return true
						}

						abInfo := ctx.Provider(ApexBundleInfoProvider).(ApexBundleInfo)
						if !abInfo.Contents.DirectlyInApex(depName) && (cc.IsStubs() || cc.HasStubsVariants()) {
							// If the dependency is a stubs lib, don't include it in this APEX,
							// but make sure that the lib is installed on the device.
							// In case no APEX is having the lib, the lib is installed to the system
							// partition.
							//
							// Always include if we are a host-apex however since those won't have any
							// system libraries.
							if !am.DirectlyInAnyApex() {
								// we need a module name for Make
								name := cc.ImplementationModuleNameForMake(ctx)

								if !proptools.Bool(a.properties.Use_vendor) {
									// we don't use subName(.vendor) for a "use_vendor: true" apex
									// which is supposed to be installed in /system
									name += cc.Properties.SubName
								}
								if !android.InList(name, a.requiredDeps) {
									a.requiredDeps = append(a.requiredDeps, name)
								}
							}
							requireNativeLibs = append(requireNativeLibs, af.stem())
							// Don't track further
							return false
						}

						// If the dep is not considered to be in the same
						// apex, don't add it to filesInfo so that it is not
						// included in this APEX.
						// TODO(jiyong): move this to at the top of the
						// else-if clause for the indirect dependencies.
						// Currently, that's impossible because we would
						// like to record requiredNativeLibs even when
						// DepIsInSameAPex is false. We also shouldn't do
						// this for host.
						if !am.DepIsInSameApex(ctx, am) {
							return false
						}

						filesInfo = append(filesInfo, af)
						return true // track transitive dependencies
					} else if rm, ok := child.(*rust.Module); ok {
						af := apexFileForRustLibrary(ctx, rm)
						af.transitiveDep = true
						filesInfo = append(filesInfo, af)
						return true // track transitive dependencies
					}
				} else if cc.IsTestPerSrcDepTag(depTag) {
					if cc, ok := child.(*cc.Module); ok {
						af := apexFileForExecutable(ctx, cc)
						// Handle modules created as `test_per_src` variations of a single test module:
						// use the name of the generated test binary (`fileToCopy`) instead of the name
						// of the original test module (`depName`, shared by all `test_per_src`
						// variations of that module).
						af.androidMkModuleName = filepath.Base(af.builtFile.String())
						// these are not considered transitive dep
						af.transitiveDep = false
						filesInfo = append(filesInfo, af)
						return true // track transitive dependencies
					}
				} else if cc.IsHeaderDepTag(depTag) {
					// nothing
				} else if java.IsJniDepTag(depTag) {
					// Because APK-in-APEX embeds jni_libs transitively, we don't need to track transitive deps
					return false
				} else if java.IsXmlPermissionsFileDepTag(depTag) {
					if prebuilt, ok := child.(prebuilt_etc.PrebuiltEtcModule); ok {
						filesInfo = append(filesInfo, apexFileForPrebuiltEtc(ctx, prebuilt, depName))
					}
				} else if rust.IsDylibDepTag(depTag) {
					if rustm, ok := child.(*rust.Module); ok && rustm.IsInstallableToApex() {
						af := apexFileForRustLibrary(ctx, rustm)
						af.transitiveDep = true
						filesInfo = append(filesInfo, af)
						return true // track transitive dependencies
					}
				} else if _, ok := depTag.(android.CopyDirectlyInAnyApexTag); ok {
					// nothing
				} else if am.CanHaveApexVariants() && am.IsInstallableToApex() {
					ctx.ModuleErrorf("unexpected tag %s for indirect dependency %q", android.PrettyPrintTag(depTag), depName)
				}
			}
		}
		return false
	})
	if a.privateKeyFile == nil {
		ctx.PropertyErrorf("key", "private_key for %q could not be found", String(a.properties.Key))
		return
	}

	if a.artApex {
		// Specific to the ART apex: dexpreopt artifacts for libcore Java libraries. Build rules are
		// generated by the dexpreopt singleton, and here we access build artifacts via the global
		// boot image config.
		for arch, files := range java.DexpreoptedArtApexJars(ctx) {
			dirInApex := filepath.Join("javalib", arch.String())
			for _, f := range files {
				localModule := "javalib_" + arch.String() + "_" + filepath.Base(f.String())
				af := newApexFile(ctx, f, localModule, dirInApex, etc, nil)
				filesInfo = append(filesInfo, af)
			}
		}
		// Call GetGlobalSoongConfig to initialize it, which may be necessary if dexpreopt is
		// disabled for libraries/apps, but boot images are still needed.
		if !java.SkipDexpreoptBootJars(ctx) {
			dexpreopt.GetGlobalSoongConfig(ctx)
		}
	}

	// Remove duplicates in filesInfo
	removeDup := func(filesInfo []apexFile) []apexFile {
		encountered := make(map[string]apexFile)
		for _, f := range filesInfo {
			dest := filepath.Join(f.installDir, f.builtFile.Base())
			if e, ok := encountered[dest]; !ok {
				encountered[dest] = f
			} else {
				// If a module is directly included and also transitively depended on
				// consider it as directly included.
				e.transitiveDep = e.transitiveDep && f.transitiveDep
				encountered[dest] = e
			}
		}
		var result []apexFile
		for _, v := range encountered {
			result = append(result, v)
		}
		return result
	}
	filesInfo = removeDup(filesInfo)

	// Sort to have consistent build rules
	sort.Slice(filesInfo, func(i, j int) bool {
		return filesInfo[i].builtFile.String() < filesInfo[j].builtFile.String()
	})

	////////////////////////////////////////////////////////////////////////////////////////////
	// 3) some fields in apexBundle struct are configured
	a.installDir = android.PathForModuleInstall(ctx, "apex")
	a.filesInfo = filesInfo

	// Set suffix and primaryApexType depending on the ApexType
	buildFlattenedAsDefault := ctx.Config().FlattenApex() && !ctx.Config().UnbundledBuildApps()
	switch a.properties.ApexType {
	case imageApex:
		if buildFlattenedAsDefault {
			a.suffix = imageApexSuffix
		} else {
			a.suffix = ""
			a.primaryApexType = true

			if ctx.Config().InstallExtraFlattenedApexes() {
				a.requiredDeps = append(a.requiredDeps, a.Name()+flattenedSuffix)
			}
		}
	case zipApex:
		if proptools.String(a.properties.Payload_type) == "zip" {
			a.suffix = ""
			a.primaryApexType = true
		} else {
			a.suffix = zipApexSuffix
		}
	case flattenedApex:
		if buildFlattenedAsDefault {
			a.suffix = ""
			a.primaryApexType = true
		} else {
			a.suffix = flattenedSuffix
		}
	}

	switch proptools.StringDefault(a.properties.Payload_fs_type, ext4FsType) {
	case ext4FsType:
		a.payloadFsType = ext4
	case f2fsFsType:
		a.payloadFsType = f2fs
	default:
		ctx.PropertyErrorf("payload_fs_type", "%q is not a valid filesystem for apex [ext4, f2fs]", *a.properties.Payload_fs_type)
	}

	// Optimization. If we are building bundled APEX, for the files that are gathered due to the
	// transitive dependencies, don't place them inside the APEX, but place a symlink pointing
	// the same library in the system partition, thus effectively sharing the same libraries
	// across the APEX boundary. For unbundled APEX, all the gathered files are actually placed
	// in the APEX.
	a.linkToSystemLib = !ctx.Config().UnbundledBuild() && a.installable() && !proptools.Bool(a.properties.Use_vendor)

	// APEXes targeting other than system/system_ext partitions use vendor/product variants.
	// So we can't link them to /system/lib libs which are core variants.
	if a.SocSpecific() || a.DeviceSpecific() || (a.ProductSpecific() && ctx.Config().EnforceProductPartitionInterface()) {
		a.linkToSystemLib = false
	}

	forced := ctx.Config().ForceApexSymlinkOptimization()

	// We don't need the optimization for updatable APEXes, as it might give false signal
	// to the system health when the APEXes are still bundled (b/149805758).
	if !forced && a.Updatable() && a.properties.ApexType == imageApex {
		a.linkToSystemLib = false
	}

	// We also don't want the optimization for host APEXes, because it doesn't make sense.
	if ctx.Host() {
		a.linkToSystemLib = false
	}

	a.compatSymlinks = makeCompatSymlinks(a.BaseModuleName(), ctx)

	////////////////////////////////////////////////////////////////////////////////////////////
	// 4) generate the build rules to create the APEX. This is done in builder.go.
	a.buildManifest(ctx, provideNativeLibs, requireNativeLibs)
	if a.properties.ApexType == flattenedApex {
		a.buildFlattenedApex(ctx)
	} else {
		a.buildUnflattenedApex(ctx)
	}
	a.buildApexDependencyInfo(ctx)
	a.buildLintReports(ctx)

	// Append meta-files to the filesInfo list so that they are reflected in Android.mk as well.
	if a.installable() {
		// For flattened APEX, make sure that APEX manifest and apex_pubkey are also copied
		// along with other ordinary files. (Note that this is done by apexer for
		// non-flattened APEXes)
		a.filesInfo = append(a.filesInfo, newApexFile(ctx, a.manifestPbOut, "apex_manifest.pb", ".", etc, nil))

		// Place the public key as apex_pubkey. This is also done by apexer for
		// non-flattened APEXes case.
		// TODO(jiyong): Why do we need this CP rule?
		copiedPubkey := android.PathForModuleOut(ctx, "apex_pubkey")
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.Cp,
			Input:  a.publicKeyFile,
			Output: copiedPubkey,
		})
		a.filesInfo = append(a.filesInfo, newApexFile(ctx, copiedPubkey, "apex_pubkey", ".", etc, nil))
	}
}

///////////////////////////////////////////////////////////////////////////////////////////////////
// Factory functions
//

func newApexBundle() *apexBundle {
	module := &apexBundle{}

	module.AddProperties(&module.properties)
	module.AddProperties(&module.targetProperties)
	module.AddProperties(&module.archProperties)
	module.AddProperties(&module.overridableProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitSdkAwareModule(module)
	android.InitOverridableModule(module, &module.overridableProperties.Overrides)
	return module
}

func ApexBundleFactory(testApex bool, artApex bool) android.Module {
	bundle := newApexBundle()
	bundle.testApex = testApex
	bundle.artApex = artApex
	return bundle
}

// apex_test is an APEX for testing. The difference from the ordinary apex module type is that
// certain compatibility checks such as apex_available are not done for apex_test.
func testApexBundleFactory() android.Module {
	bundle := newApexBundle()
	bundle.testApex = true
	return bundle
}

// apex packages other modules into an APEX file which is a packaging format for system-level
// components like binaries, shared libraries, etc.
func BundleFactory() android.Module {
	return newApexBundle()
}

type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

// apex_defaults provides defaultable properties to other apex modules.
func defaultsFactory() android.Module {
	return DefaultsFactory()
}

func DefaultsFactory(props ...interface{}) android.Module {
	module := &Defaults{}

	module.AddProperties(props...)
	module.AddProperties(
		&apexBundleProperties{},
		&apexTargetBundleProperties{},
		&overridableProperties{},
	)

	android.InitDefaultsModule(module)
	return module
}

type OverrideApex struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (o *OverrideApex) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// All the overrides happen in the base module.
}

// override_apex is used to create an apex module based on another apex module by overriding some of
// its properties.
func overrideApexFactory() android.Module {
	m := &OverrideApex{}

	m.AddProperties(&overridableProperties{})

	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, android.MultilibCommon)
	android.InitOverrideModule(m)
	return m
}

///////////////////////////////////////////////////////////////////////////////////////////////////
// Vality check routines
//
// These are called in at the very beginning of GenerateAndroidBuildActions to flag an error when
// certain conditions are not met.
//
// TODO(jiyong): move these checks to a separate go file.

// Entures that min_sdk_version of the included modules are equal or less than the min_sdk_version
// of this apexBundle.
func (a *apexBundle) checkMinSdkVersion(ctx android.ModuleContext) {
	if a.testApex || a.vndkApex {
		return
	}
	// Meaningless to check min_sdk_version when building use_vendor modules against non-Trebleized targets
	if proptools.Bool(a.properties.Use_vendor) && ctx.DeviceConfig().VndkVersion() == "" {
		return
	}
	// apexBundle::minSdkVersion reports its own errors.
	minSdkVersion := a.minSdkVersion(ctx)
	android.CheckMinSdkVersion(a, ctx, minSdkVersion)
}

func (a *apexBundle) minSdkVersion(ctx android.BaseModuleContext) android.ApiLevel {
	ver := proptools.String(a.properties.Min_sdk_version)
	if ver == "" {
		return android.NoneApiLevel
	}
	apiLevel, err := android.ApiLevelFromUser(ctx, ver)
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "%s", err.Error())
		return android.NoneApiLevel
	}
	return apiLevel
}

// Ensures that a lib providing stub isn't statically linked
func (a *apexBundle) checkStaticLinkingToStubLibraries(ctx android.ModuleContext) {
	// Practically, we only care about regular APEXes on the device.
	if ctx.Host() || a.testApex || a.vndkApex {
		return
	}

	abInfo := ctx.Provider(ApexBundleInfoProvider).(ApexBundleInfo)

	a.WalkPayloadDeps(ctx, func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool) bool {
		if ccm, ok := to.(*cc.Module); ok {
			apexName := ctx.ModuleName()
			fromName := ctx.OtherModuleName(from)
			toName := ctx.OtherModuleName(to)

			// If `to` is not actually in the same APEX as `from` then it does not need
			// apex_available and neither do any of its dependencies.
			if am, ok := from.(android.DepIsInSameApex); ok && !am.DepIsInSameApex(ctx, to) {
				// As soon as the dependency graph crosses the APEX boundary, don't go further.
				return false
			}

			// The dynamic linker and crash_dump tool in the runtime APEX is the only
			// exception to this rule. It can't make the static dependencies dynamic
			// because it can't do the dynamic linking for itself.
			// Same rule should be applied to linkerconfig, because it should be executed
			// only with static linked libraries before linker is available with ld.config.txt
			if apexName == "com.android.runtime" && (fromName == "linker" || fromName == "crash_dump" || fromName == "linkerconfig") {
				return false
			}

			isStubLibraryFromOtherApex := ccm.HasStubsVariants() && !abInfo.Contents.DirectlyInApex(toName)
			if isStubLibraryFromOtherApex && !externalDep {
				ctx.ModuleErrorf("%q required by %q is a native library providing stub. "+
					"It shouldn't be included in this APEX via static linking. Dependency path: %s", to.String(), fromName, ctx.GetPathString(false))
			}

		}
		return true
	})
}

// Enforce that Java deps of the apex are using stable SDKs to compile
func (a *apexBundle) checkUpdatable(ctx android.ModuleContext) {
	if a.Updatable() {
		if String(a.properties.Min_sdk_version) == "" {
			ctx.PropertyErrorf("updatable", "updatable APEXes should set min_sdk_version as well")
		}
		a.checkJavaStableSdkVersion(ctx)
	}
}

func (a *apexBundle) checkJavaStableSdkVersion(ctx android.ModuleContext) {
	// Visit direct deps only. As long as we guarantee top-level deps are using stable SDKs,
	// java's checkLinkType guarantees correct usage for transitive deps
	ctx.VisitDirectDepsBlueprint(func(module blueprint.Module) {
		tag := ctx.OtherModuleDependencyTag(module)
		switch tag {
		case javaLibTag, androidAppTag:
			if m, ok := module.(interface{ CheckStableSdkVersion() error }); ok {
				if err := m.CheckStableSdkVersion(); err != nil {
					ctx.ModuleErrorf("cannot depend on \"%v\": %v", ctx.OtherModuleName(module), err)
				}
			}
		}
	})
}

// Ensures that the all the dependencies are marked as available for this APEX
func (a *apexBundle) checkApexAvailability(ctx android.ModuleContext) {
	// Let's be practical. Availability for test, host, and the VNDK apex isn't important
	if ctx.Host() || a.testApex || a.vndkApex {
		return
	}

	// Because APEXes targeting other than system/system_ext partitions can't set
	// apex_available, we skip checks for these APEXes
	if a.SocSpecific() || a.DeviceSpecific() || (a.ProductSpecific() && ctx.Config().EnforceProductPartitionInterface()) {
		return
	}

	// Coverage build adds additional dependencies for the coverage-only runtime libraries.
	// Requiring them and their transitive depencies with apex_available is not right
	// because they just add noise.
	if ctx.Config().IsEnvTrue("EMMA_INSTRUMENT") || a.IsNativeCoverageNeeded(ctx) {
		return
	}

	a.WalkPayloadDeps(ctx, func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool) bool {
		// As soon as the dependency graph crosses the APEX boundary, don't go further.
		if externalDep {
			return false
		}

		apexName := ctx.ModuleName()
		fromName := ctx.OtherModuleName(from)
		toName := ctx.OtherModuleName(to)

		// If `to` is not actually in the same APEX as `from` then it does not need
		// apex_available and neither do any of its dependencies.
		if am, ok := from.(android.DepIsInSameApex); ok && !am.DepIsInSameApex(ctx, to) {
			// As soon as the dependency graph crosses the APEX boundary, don't go
			// further.
			return false
		}

		if to.AvailableFor(apexName) || baselineApexAvailable(apexName, toName) {
			return true
		}
		ctx.ModuleErrorf("%q requires %q that doesn't list the APEX under 'apex_available'. Dependency path:%s",
			fromName, toName, ctx.GetPathString(true))
		// Visit this module's dependencies to check and report any issues with their availability.
		return true
	})
}

var (
	apexAvailBaseline        = makeApexAvailableBaseline()
	inverseApexAvailBaseline = invertApexBaseline(apexAvailBaseline)
)

func baselineApexAvailable(apex, moduleName string) bool {
	key := apex
	moduleName = normalizeModuleName(moduleName)

	if val, ok := apexAvailBaseline[key]; ok && android.InList(moduleName, val) {
		return true
	}

	key = android.AvailableToAnyApex
	if val, ok := apexAvailBaseline[key]; ok && android.InList(moduleName, val) {
		return true
	}

	return false
}

func normalizeModuleName(moduleName string) string {
	// Prebuilt modules (e.g. java_import, etc.) have "prebuilt_" prefix added by the build
	// system. Trim the prefix for the check since they are confusing
	moduleName = android.RemoveOptionalPrebuiltPrefix(moduleName)
	if strings.HasPrefix(moduleName, "libclang_rt.") {
		// This module has many arch variants that depend on the product being built.
		// We don't want to list them all
		moduleName = "libclang_rt"
	}
	if strings.HasPrefix(moduleName, "androidx.") {
		// TODO(b/156996905) Set apex_available/min_sdk_version for androidx support libraries
		moduleName = "androidx"
	}
	return moduleName
}

// Transform the map of apex -> modules to module -> apexes.
func invertApexBaseline(m map[string][]string) map[string][]string {
	r := make(map[string][]string)
	for apex, modules := range m {
		for _, module := range modules {
			r[module] = append(r[module], apex)
		}
	}
	return r
}

// Retrieve the baseline of apexes to which the supplied module belongs.
func BaselineApexAvailable(moduleName string) []string {
	return inverseApexAvailBaseline[normalizeModuleName(moduleName)]
}

// This is a map from apex to modules, which overrides the apex_available setting for that
// particular module to make it available for the apex regardless of its setting.
// TODO(b/147364041): remove this
func makeApexAvailableBaseline() map[string][]string {
	// The "Module separator"s below are employed to minimize merge conflicts.
	m := make(map[string][]string)
	//
	// Module separator
	//
	m["com.android.appsearch"] = []string{
		"icing-java-proto-lite",
		"libprotobuf-java-lite",
	}
	//
	// Module separator
	//
	m["com.android.bluetooth.updatable"] = []string{
		"android.hardware.audio.common@5.0",
		"android.hardware.bluetooth.a2dp@1.0",
		"android.hardware.bluetooth.audio@2.0",
		"android.hardware.bluetooth@1.0",
		"android.hardware.bluetooth@1.1",
		"android.hardware.graphics.bufferqueue@1.0",
		"android.hardware.graphics.bufferqueue@2.0",
		"android.hardware.graphics.common@1.0",
		"android.hardware.graphics.common@1.1",
		"android.hardware.graphics.common@1.2",
		"android.hardware.media@1.0",
		"android.hidl.safe_union@1.0",
		"android.hidl.token@1.0",
		"android.hidl.token@1.0-utils",
		"avrcp-target-service",
		"avrcp_headers",
		"bluetooth-protos-lite",
		"bluetooth.mapsapi",
		"com.android.vcard",
		"dnsresolver_aidl_interface-V2-java",
		"ipmemorystore-aidl-interfaces-V5-java",
		"ipmemorystore-aidl-interfaces-java",
		"internal_include_headers",
		"lib-bt-packets",
		"lib-bt-packets-avrcp",
		"lib-bt-packets-base",
		"libFraunhoferAAC",
		"libaudio-a2dp-hw-utils",
		"libaudio-hearing-aid-hw-utils",
		"libbinder_headers",
		"libbluetooth",
		"libbluetooth-types",
		"libbluetooth-types-header",
		"libbluetooth_gd",
		"libbluetooth_headers",
		"libbluetooth_jni",
		"libbt-audio-hal-interface",
		"libbt-bta",
		"libbt-common",
		"libbt-hci",
		"libbt-platform-protos-lite",
		"libbt-protos-lite",
		"libbt-sbc-decoder",
		"libbt-sbc-encoder",
		"libbt-stack",
		"libbt-utils",
		"libbtcore",
		"libbtdevice",
		"libbte",
		"libbtif",
		"libchrome",
		"libevent",
		"libfmq",
		"libg722codec",
		"libgui_headers",
		"libmedia_headers",
		"libmodpb64",
		"libosi",
		"libstagefright_foundation_headers",
		"libstagefright_headers",
		"libstatslog",
		"libstatssocket",
		"libtinyxml2",
		"libudrv-uipc",
		"libz",
		"media_plugin_headers",
		"net-utils-services-common",
		"netd_aidl_interface-unstable-java",
		"netd_event_listener_interface-java",
		"netlink-client",
		"networkstack-client",
		"sap-api-java-static",
		"services.net",
	}
	//
	// Module separator
	//
	m["com.android.cellbroadcast"] = []string{"CellBroadcastApp", "CellBroadcastServiceModule"}
	//
	// Module separator
	//
	m["com.android.extservices"] = []string{
		"error_prone_annotations",
		"ExtServices-core",
		"ExtServices",
		"libtextclassifier-java",
		"libz_current",
		"textclassifier-statsd",
		"TextClassifierNotificationLibNoManifest",
		"TextClassifierServiceLibNoManifest",
	}
	//
	// Module separator
	//
	m["com.android.neuralnetworks"] = []string{
		"android.hardware.neuralnetworks@1.0",
		"android.hardware.neuralnetworks@1.1",
		"android.hardware.neuralnetworks@1.2",
		"android.hardware.neuralnetworks@1.3",
		"android.hidl.allocator@1.0",
		"android.hidl.memory.token@1.0",
		"android.hidl.memory@1.0",
		"android.hidl.safe_union@1.0",
		"libarect",
		"libbuildversion",
		"libmath",
		"libprocpartition",
		"libsync",
	}
	//
	// Module separator
	//
	m["com.android.media"] = []string{
		"android.frameworks.bufferhub@1.0",
		"android.hardware.cas.native@1.0",
		"android.hardware.cas@1.0",
		"android.hardware.configstore-utils",
		"android.hardware.configstore@1.0",
		"android.hardware.configstore@1.1",
		"android.hardware.graphics.allocator@2.0",
		"android.hardware.graphics.allocator@3.0",
		"android.hardware.graphics.bufferqueue@1.0",
		"android.hardware.graphics.bufferqueue@2.0",
		"android.hardware.graphics.common@1.0",
		"android.hardware.graphics.common@1.1",
		"android.hardware.graphics.common@1.2",
		"android.hardware.graphics.mapper@2.0",
		"android.hardware.graphics.mapper@2.1",
		"android.hardware.graphics.mapper@3.0",
		"android.hardware.media.omx@1.0",
		"android.hardware.media@1.0",
		"android.hidl.allocator@1.0",
		"android.hidl.memory.token@1.0",
		"android.hidl.memory@1.0",
		"android.hidl.token@1.0",
		"android.hidl.token@1.0-utils",
		"bionic_libc_platform_headers",
		"exoplayer2-extractor",
		"exoplayer2-extractor-annotation-stubs",
		"gl_headers",
		"jsr305",
		"libEGL",
		"libEGL_blobCache",
		"libEGL_getProcAddress",
		"libFLAC",
		"libFLAC-config",
		"libFLAC-headers",
		"libGLESv2",
		"libaacextractor",
		"libamrextractor",
		"libarect",
		"libaudio_system_headers",
		"libaudioclient",
		"libaudioclient_headers",
		"libaudiofoundation",
		"libaudiofoundation_headers",
		"libaudiomanager",
		"libaudiopolicy",
		"libaudioutils",
		"libaudioutils_fixedfft",
		"libbinder_headers",
		"libbluetooth-types-header",
		"libbufferhub",
		"libbufferhub_headers",
		"libbufferhubqueue",
		"libc_malloc_debug_backtrace",
		"libcamera_client",
		"libcamera_metadata",
		"libdvr_headers",
		"libexpat",
		"libfifo",
		"libflacextractor",
		"libgrallocusage",
		"libgraphicsenv",
		"libgui",
		"libgui_headers",
		"libhardware_headers",
		"libinput",
		"liblzma",
		"libmath",
		"libmedia",
		"libmedia_codeclist",
		"libmedia_headers",
		"libmedia_helper",
		"libmedia_helper_headers",
		"libmedia_midiiowrapper",
		"libmedia_omx",
		"libmediautils",
		"libmidiextractor",
		"libmkvextractor",
		"libmp3extractor",
		"libmp4extractor",
		"libmpeg2extractor",
		"libnativebase_headers",
		"libnativewindow_headers",
		"libnblog",
		"liboggextractor",
		"libpackagelistparser",
		"libpdx",
		"libpdx_default_transport",
		"libpdx_headers",
		"libpdx_uds",
		"libprocinfo",
		"libspeexresampler",
		"libspeexresampler",
		"libstagefright_esds",
		"libstagefright_flacdec",
		"libstagefright_flacdec",
		"libstagefright_foundation",
		"libstagefright_foundation_headers",
		"libstagefright_foundation_without_imemory",
		"libstagefright_headers",
		"libstagefright_id3",
		"libstagefright_metadatautils",
		"libstagefright_mpeg2extractor",
		"libstagefright_mpeg2support",
		"libsync",
		"libui",
		"libui_headers",
		"libunwindstack",
		"libvibrator",
		"libvorbisidec",
		"libwavextractor",
		"libwebm",
		"media_ndk_headers",
		"media_plugin_headers",
		"updatable-media",
	}
	//
	// Module separator
	//
	m["com.android.media.swcodec"] = []string{
		"android.frameworks.bufferhub@1.0",
		"android.hardware.common-ndk_platform",
		"android.hardware.configstore-utils",
		"android.hardware.configstore@1.0",
		"android.hardware.configstore@1.1",
		"android.hardware.graphics.allocator@2.0",
		"android.hardware.graphics.allocator@3.0",
		"android.hardware.graphics.allocator@4.0",
		"android.hardware.graphics.bufferqueue@1.0",
		"android.hardware.graphics.bufferqueue@2.0",
		"android.hardware.graphics.common-ndk_platform",
		"android.hardware.graphics.common@1.0",
		"android.hardware.graphics.common@1.1",
		"android.hardware.graphics.common@1.2",
		"android.hardware.graphics.mapper@2.0",
		"android.hardware.graphics.mapper@2.1",
		"android.hardware.graphics.mapper@3.0",
		"android.hardware.graphics.mapper@4.0",
		"android.hardware.media.bufferpool@2.0",
		"android.hardware.media.c2@1.0",
		"android.hardware.media.c2@1.1",
		"android.hardware.media.omx@1.0",
		"android.hardware.media@1.0",
		"android.hardware.media@1.0",
		"android.hidl.memory.token@1.0",
		"android.hidl.memory@1.0",
		"android.hidl.safe_union@1.0",
		"android.hidl.token@1.0",
		"android.hidl.token@1.0-utils",
		"libEGL",
		"libFLAC",
		"libFLAC-config",
		"libFLAC-headers",
		"libFraunhoferAAC",
		"libLibGuiProperties",
		"libarect",
		"libaudio_system_headers",
		"libaudioutils",
		"libaudioutils",
		"libaudioutils_fixedfft",
		"libavcdec",
		"libavcenc",
		"libavservices_minijail",
		"libavservices_minijail",
		"libbinder_headers",
		"libbinderthreadstateutils",
		"libbluetooth-types-header",
		"libbufferhub_headers",
		"libcodec2",
		"libcodec2_headers",
		"libcodec2_hidl@1.0",
		"libcodec2_hidl@1.1",
		"libcodec2_internal",
		"libcodec2_soft_aacdec",
		"libcodec2_soft_aacenc",
		"libcodec2_soft_amrnbdec",
		"libcodec2_soft_amrnbenc",
		"libcodec2_soft_amrwbdec",
		"libcodec2_soft_amrwbenc",
		"libcodec2_soft_av1dec_gav1",
		"libcodec2_soft_avcdec",
		"libcodec2_soft_avcenc",
		"libcodec2_soft_common",
		"libcodec2_soft_flacdec",
		"libcodec2_soft_flacenc",
		"libcodec2_soft_g711alawdec",
		"libcodec2_soft_g711mlawdec",
		"libcodec2_soft_gsmdec",
		"libcodec2_soft_h263dec",
		"libcodec2_soft_h263enc",
		"libcodec2_soft_hevcdec",
		"libcodec2_soft_hevcenc",
		"libcodec2_soft_mp3dec",
		"libcodec2_soft_mpeg2dec",
		"libcodec2_soft_mpeg4dec",
		"libcodec2_soft_mpeg4enc",
		"libcodec2_soft_opusdec",
		"libcodec2_soft_opusenc",
		"libcodec2_soft_rawdec",
		"libcodec2_soft_vorbisdec",
		"libcodec2_soft_vp8dec",
		"libcodec2_soft_vp8enc",
		"libcodec2_soft_vp9dec",
		"libcodec2_soft_vp9enc",
		"libcodec2_vndk",
		"libdvr_headers",
		"libfmq",
		"libfmq",
		"libgav1",
		"libgralloctypes",
		"libgrallocusage",
		"libgraphicsenv",
		"libgsm",
		"libgui_bufferqueue_static",
		"libgui_headers",
		"libhardware",
		"libhardware_headers",
		"libhevcdec",
		"libhevcenc",
		"libion",
		"libjpeg",
		"liblzma",
		"libmath",
		"libmedia_codecserviceregistrant",
		"libmedia_headers",
		"libmpeg2dec",
		"libnativebase_headers",
		"libnativewindow_headers",
		"libpdx_headers",
		"libscudo_wrapper",
		"libsfplugin_ccodec_utils",
		"libspeexresampler",
		"libstagefright_amrnb_common",
		"libstagefright_amrnbdec",
		"libstagefright_amrnbenc",
		"libstagefright_amrwbdec",
		"libstagefright_amrwbenc",
		"libstagefright_bufferpool@2.0.1",
		"libstagefright_enc_common",
		"libstagefright_flacdec",
		"libstagefright_foundation",
		"libstagefright_foundation_headers",
		"libstagefright_headers",
		"libstagefright_m4vh263dec",
		"libstagefright_m4vh263enc",
		"libstagefright_mp3dec",
		"libsync",
		"libui",
		"libui_headers",
		"libunwindstack",
		"libvorbisidec",
		"libvpx",
		"libyuv",
		"libyuv_static",
		"media_ndk_headers",
		"media_plugin_headers",
		"mediaswcodec",
	}
	//
	// Module separator
	//
	m["com.android.mediaprovider"] = []string{
		"MediaProvider",
		"MediaProviderGoogle",
		"fmtlib_ndk",
		"libbase_ndk",
		"libfuse",
		"libfuse_jni",
	}
	//
	// Module separator
	//
	m["com.android.permission"] = []string{
		"car-ui-lib",
		"iconloader",
		"kotlin-annotations",
		"kotlin-stdlib",
		"kotlin-stdlib-jdk7",
		"kotlin-stdlib-jdk8",
		"kotlinx-coroutines-android",
		"kotlinx-coroutines-android-nodeps",
		"kotlinx-coroutines-core",
		"kotlinx-coroutines-core-nodeps",
		"permissioncontroller-statsd",
		"GooglePermissionController",
		"PermissionController",
		"SettingsLibActionBarShadow",
		"SettingsLibAppPreference",
		"SettingsLibBarChartPreference",
		"SettingsLibLayoutPreference",
		"SettingsLibProgressBar",
		"SettingsLibSearchWidget",
		"SettingsLibSettingsTheme",
		"SettingsLibRestrictedLockUtils",
		"SettingsLibHelpUtils",
	}
	//
	// Module separator
	//
	m["com.android.runtime"] = []string{
		"bionic_libc_platform_headers",
		"libarm-optimized-routines-math",
		"libc_aeabi",
		"libc_bionic",
		"libc_bionic_ndk",
		"libc_bootstrap",
		"libc_common",
		"libc_common_shared",
		"libc_common_static",
		"libc_dns",
		"libc_dynamic_dispatch",
		"libc_fortify",
		"libc_freebsd",
		"libc_freebsd_large_stack",
		"libc_gdtoa",
		"libc_init_dynamic",
		"libc_init_static",
		"libc_jemalloc_wrapper",
		"libc_netbsd",
		"libc_nomalloc",
		"libc_nopthread",
		"libc_openbsd",
		"libc_openbsd_large_stack",
		"libc_openbsd_ndk",
		"libc_pthread",
		"libc_static_dispatch",
		"libc_syscalls",
		"libc_tzcode",
		"libc_unwind_static",
		"libdebuggerd",
		"libdebuggerd_common_headers",
		"libdebuggerd_handler_core",
		"libdebuggerd_handler_fallback",
		"libdl_static",
		"libjemalloc5",
		"liblinker_main",
		"liblinker_malloc",
		"liblz4",
		"liblzma",
		"libprocinfo",
		"libpropertyinfoparser",
		"libscudo",
		"libstdc++",
		"libsystemproperties",
		"libtombstoned_client_static",
		"libunwindstack",
		"libz",
		"libziparchive",
	}
	//
	// Module separator
	//
	m["com.android.tethering"] = []string{
		"android.hardware.tetheroffload.config-V1.0-java",
		"android.hardware.tetheroffload.control-V1.0-java",
		"android.hidl.base-V1.0-java",
		"libcgrouprc",
		"libcgrouprc_format",
		"libtetherutilsjni",
		"libvndksupport",
		"net-utils-framework-common",
		"netd_aidl_interface-V3-java",
		"netlink-client",
		"networkstack-aidl-interfaces-java",
		"tethering-aidl-interfaces-java",
		"TetheringApiCurrentLib",
	}
	//
	// Module separator
	//
	m["com.android.wifi"] = []string{
		"PlatformProperties",
		"android.hardware.wifi-V1.0-java",
		"android.hardware.wifi-V1.0-java-constants",
		"android.hardware.wifi-V1.1-java",
		"android.hardware.wifi-V1.2-java",
		"android.hardware.wifi-V1.3-java",
		"android.hardware.wifi-V1.4-java",
		"android.hardware.wifi.hostapd-V1.0-java",
		"android.hardware.wifi.hostapd-V1.1-java",
		"android.hardware.wifi.hostapd-V1.2-java",
		"android.hardware.wifi.supplicant-V1.0-java",
		"android.hardware.wifi.supplicant-V1.1-java",
		"android.hardware.wifi.supplicant-V1.2-java",
		"android.hardware.wifi.supplicant-V1.3-java",
		"android.hidl.base-V1.0-java",
		"android.hidl.manager-V1.0-java",
		"android.hidl.manager-V1.1-java",
		"android.hidl.manager-V1.2-java",
		"bouncycastle-unbundled",
		"dnsresolver_aidl_interface-V2-java",
		"error_prone_annotations",
		"framework-wifi-pre-jarjar",
		"framework-wifi-util-lib",
		"ipmemorystore-aidl-interfaces-V3-java",
		"ipmemorystore-aidl-interfaces-java",
		"ksoap2",
		"libnanohttpd",
		"libwifi-jni",
		"net-utils-services-common",
		"netd_aidl_interface-V2-java",
		"netd_aidl_interface-unstable-java",
		"netd_event_listener_interface-java",
		"netlink-client",
		"networkstack-client",
		"services.net",
		"wifi-lite-protos",
		"wifi-nano-protos",
		"wifi-service-pre-jarjar",
		"wifi-service-resources",
	}
	//
	// Module separator
	//
	m["com.android.sdkext"] = []string{
		"fmtlib_ndk",
		"libbase_ndk",
		"libprotobuf-cpp-lite-ndk",
	}
	//
	// Module separator
	//
	m["com.android.os.statsd"] = []string{
		"libstatssocket",
	}
	//
	// Module separator
	//
	m[android.AvailableToAnyApex] = []string{
		// TODO(b/156996905) Set apex_available/min_sdk_version for androidx/extras support libraries
		"androidx",
		"androidx-constraintlayout_constraintlayout",
		"androidx-constraintlayout_constraintlayout-nodeps",
		"androidx-constraintlayout_constraintlayout-solver",
		"androidx-constraintlayout_constraintlayout-solver-nodeps",
		"com.google.android.material_material",
		"com.google.android.material_material-nodeps",

		"libatomic",
		"libclang_rt",
		"libgcc_stripped",
		"libprofile-clang-extras",
		"libprofile-clang-extras_ndk",
		"libprofile-extras",
		"libprofile-extras_ndk",
		"libunwind",
	}
	return m
}

func init() {
	android.AddNeverAllowRules(createApexPermittedPackagesRules(qModulesPackages())...)
	android.AddNeverAllowRules(createApexPermittedPackagesRules(rModulesPackages())...)
}

func createApexPermittedPackagesRules(modules_packages map[string][]string) []android.Rule {
	rules := make([]android.Rule, 0, len(modules_packages))
	for module_name, module_packages := range modules_packages {
		permittedPackagesRule := android.NeverAllow().
			BootclasspathJar().
			With("apex_available", module_name).
			WithMatcher("permitted_packages", android.NotInList(module_packages)).
			Because("jars that are part of the " + module_name +
				" module may only allow these packages: " + strings.Join(module_packages, ",") +
				". Please jarjar or move code around.")
		rules = append(rules, permittedPackagesRule)
	}
	return rules
}

// DO NOT EDIT! These are the package prefixes that are exempted from being AOT'ed by ART.
// Adding code to the bootclasspath in new packages will cause issues on module update.
func qModulesPackages() map[string][]string {
	return map[string][]string{
		"com.android.conscrypt": []string{
			"android.net.ssl",
			"com.android.org.conscrypt",
		},
		"com.android.media": []string{
			"android.media",
		},
	}
}

// DO NOT EDIT! These are the package prefixes that are exempted from being AOT'ed by ART.
// Adding code to the bootclasspath in new packages will cause issues on module update.
func rModulesPackages() map[string][]string {
	return map[string][]string{
		"com.android.mediaprovider": []string{
			"android.provider",
		},
		"com.android.permission": []string{
			"android.permission",
			"android.app.role",
			"com.android.permission",
			"com.android.role",
		},
		"com.android.sdkext": []string{
			"android.os.ext",
		},
		"com.android.os.statsd": []string{
			"android.app",
			"android.os",
			"android.util",
			"com.android.internal.statsd",
			"com.android.server.stats",
		},
		"com.android.wifi": []string{
			"com.android.server.wifi",
			"com.android.wifi.x",
			"android.hardware.wifi",
			"android.net.wifi",
		},
		"com.android.tethering": []string{
			"android.net",
		},
	}
}
