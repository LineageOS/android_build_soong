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
	"regexp"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/bpf"
	"android/soong/cc"
	prebuilt_etc "android/soong/etc"
	"android/soong/filesystem"
	"android/soong/java"
	"android/soong/multitree"
	"android/soong/rust"
	"android/soong/sh"
)

func init() {
	registerApexBuildComponents(android.InitRegistrationContext)
}

func registerApexBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("apex", BundleFactory)
	ctx.RegisterModuleType("apex_test", TestApexBundleFactory)
	ctx.RegisterModuleType("apex_vndk", vndkApexBundleFactory)
	ctx.RegisterModuleType("apex_defaults", DefaultsFactory)
	ctx.RegisterModuleType("prebuilt_apex", PrebuiltFactory)
	ctx.RegisterModuleType("override_apex", OverrideApexFactory)
	ctx.RegisterModuleType("apex_set", apexSetFactory)

	ctx.PreArchMutators(registerPreArchMutators)
	ctx.PreDepsMutators(RegisterPreDepsMutators)
	ctx.PostDepsMutators(RegisterPostDepsMutators)
}

func registerPreArchMutators(ctx android.RegisterMutatorsContext) {
	ctx.TopDown("prebuilt_apex_module_creator", prebuiltApexModuleCreatorMutator).Parallel()
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
	// Run mark_platform_availability before the apexMutator as the apexMutator needs to know whether
	// it should create a platform variant.
	ctx.BottomUp("mark_platform_availability", markPlatformAvailability).Parallel()
	ctx.Transition("apex", &apexTransitionMutator{})
	ctx.BottomUp("apex_directly_in_any", apexDirectlyInAnyMutator).Parallel()
	ctx.BottomUp("apex_dcla_deps", apexDCLADepsMutator).Parallel()
	// Register after apex_info mutator so that it can use ApexVariationName
	ctx.TopDown("apex_strict_updatability_lint", apexStrictUpdatibilityLintMutator).Parallel()
}

type apexBundleProperties struct {
	// Json manifest file describing meta info of this APEX bundle. Refer to
	// system/apex/proto/apex_manifest.proto for the schema. Default: "apex_manifest.json"
	Manifest *string `android:"path"`

	// AndroidManifest.xml file used for the zip container of this APEX bundle. If unspecified,
	// a default one is automatically generated.
	AndroidManifest *string `android:"path"`

	// Determines the file contexts file for setting the security contexts to files in this APEX
	// bundle. For platform APEXes, this should points to a file under /system/sepolicy Default:
	// /system/sepolicy/apex/<module_name>_file_contexts.
	File_contexts *string `android:"path"`

	// By default, file_contexts is amended by force-labelling / and /apex_manifest.pb as system_file
	// to avoid mistakes. When set as true, no force-labelling.
	Use_file_contexts_as_is *bool

	// Path to the canned fs config file for customizing file's
	// uid/gid/mod/capabilities. The content of this file is appended to the
	// default config, so that the custom entries are preferred. The format is
	// /<path_or_glob> <uid> <gid> <mode> [capabilities=0x<cap>], where
	// path_or_glob is a path or glob pattern for a file or set of files,
	// uid/gid are numerial values of user ID and group ID, mode is octal value
	// for the file mode, and cap is hexadecimal value for the capability.
	Canned_fs_config *string `android:"path"`

	ApexNativeDependencies

	Multilib apexMultilibProperties

	// List of runtime resource overlays (RROs) that are embedded inside this APEX.
	Rros []string

	// List of bootclasspath fragments that are embedded inside this APEX bundle.
	Bootclasspath_fragments []string

	// List of systemserverclasspath fragments that are embedded inside this APEX bundle.
	Systemserverclasspath_fragments []string

	// List of java libraries that are embedded inside this APEX bundle.
	Java_libs []string

	// List of sh binaries that are embedded inside this APEX bundle.
	Sh_binaries []string

	// List of platform_compat_config files that are embedded inside this APEX bundle.
	Compat_configs []string

	// List of filesystem images that are embedded inside this APEX bundle.
	Filesystems []string

	// List of module names which we don't want to add as transitive deps. This can be used as
	// a workaround when the current implementation collects more than necessary. For example,
	// Rust binaries with prefer_rlib:true add unnecessary dependencies.
	Unwanted_transitive_deps []string

	// Whether this APEX is considered updatable or not. When set to true, this will enforce
	// additional rules for making sure that the APEX is truly updatable. To be updatable,
	// min_sdk_version should be set as well. This will also disable the size optimizations like
	// symlinking to the system libs. Default is true.
	Updatable *bool

	// Marks that this APEX is designed to be updatable in the future, although it's not
	// updatable yet. This is used to mimic some of the build behaviors that are applied only to
	// updatable APEXes. Currently, this disables the size optimization, so that the size of
	// APEX will not increase when the APEX is actually marked as truly updatable. Default is
	// false.
	Future_updatable *bool

	// Whether this APEX can use platform APIs or not. Can be set to true only when `updatable:
	// false`. Default is false.
	Platform_apis *bool

	// Whether this APEX is installable to one of the partitions like system, vendor, etc.
	// Default: true.
	Installable *bool

	// If set true, VNDK libs are considered as stable libs and are not included in this APEX.
	// Should be only used in non-system apexes (e.g. vendor: true). Default is false.
	Use_vndk_as_stable *bool

	// The type of filesystem to use. Either 'ext4', 'f2fs' or 'erofs'. Default 'ext4'.
	Payload_fs_type *string

	// For telling the APEX to ignore special handling for system libraries such as bionic.
	// Default is false.
	Ignore_system_library_special_case *bool

	// Whenever apex_payload.img of the APEX should include dm-verity hashtree.
	// Default value is true.
	Generate_hashtree *bool

	// Whenever apex_payload.img of the APEX should not be dm-verity signed. Should be only
	// used in tests.
	Test_only_unsigned_payload *bool

	// Whenever apex should be compressed, regardless of product flag used. Should be only
	// used in tests.
	Test_only_force_compression *bool

	// Put extra tags (signer=<value>) to apexkeys.txt, so that release tools can sign this apex
	// with the tool to sign payload contents.
	Custom_sign_tool *string

	// Whether this is a dynamic common lib apex, if so the native shared libs will be placed
	// in a special way that include the digest of the lib file under /lib(64)?
	Dynamic_common_lib_apex *bool

	// Canonical name of this APEX bundle. Used to determine the path to the
	// activated APEX on device (i.e. /apex/<apexVariationName>), and used for the
	// apex mutator variations. For override_apex modules, this is the name of the
	// overridden base module.
	ApexVariationName string `blueprint:"mutated"`

	IsCoverageVariant bool `blueprint:"mutated"`

	// List of sanitizer names that this APEX is enabled for
	SanitizerNames []string `blueprint:"mutated"`

	PreventInstall bool `blueprint:"mutated"`

	HideFromMake bool `blueprint:"mutated"`

	// Name that dependencies can specify in their apex_available properties to refer to this module.
	// If not specified, this defaults to Soong module name. This must be the name of a Soong module.
	Apex_available_name *string

	// Variant version of the mainline module. Must be an integer between 0-9
	Variant_version *string
}

type ApexNativeDependencies struct {
	// List of native libraries that are embedded inside this APEX.
	Native_shared_libs []string

	// List of JNI libraries that are embedded inside this APEX.
	Jni_libs []string

	// List of rust dyn libraries that are embedded inside this APEX.
	Rust_dyn_libs []string

	// List of native executables that are embedded inside this APEX.
	Binaries []string

	// List of native tests that are embedded inside this APEX.
	Tests []string

	// List of filesystem images that are embedded inside this APEX bundle.
	Filesystems []string

	// List of prebuilt_etcs that are embedded inside this APEX bundle.
	Prebuilts []string

	// List of native libraries to exclude from this APEX.
	Exclude_native_shared_libs []string

	// List of JNI libraries to exclude from this APEX.
	Exclude_jni_libs []string

	// List of rust dyn libraries to exclude from this APEX.
	Exclude_rust_dyn_libs []string

	// List of native executables to exclude from this APEX.
	Exclude_binaries []string

	// List of native tests to exclude from this APEX.
	Exclude_tests []string

	// List of filesystem images to exclude from this APEX bundle.
	Exclude_filesystems []string

	// List of prebuilt_etcs to exclude from this APEX bundle.
	Exclude_prebuilts []string
}

// Merge combines another ApexNativeDependencies into this one
func (a *ApexNativeDependencies) Merge(b ApexNativeDependencies) {
	a.Native_shared_libs = append(a.Native_shared_libs, b.Native_shared_libs...)
	a.Jni_libs = append(a.Jni_libs, b.Jni_libs...)
	a.Rust_dyn_libs = append(a.Rust_dyn_libs, b.Rust_dyn_libs...)
	a.Binaries = append(a.Binaries, b.Binaries...)
	a.Tests = append(a.Tests, b.Tests...)
	a.Filesystems = append(a.Filesystems, b.Filesystems...)
	a.Prebuilts = append(a.Prebuilts, b.Prebuilts...)

	a.Exclude_native_shared_libs = append(a.Exclude_native_shared_libs, b.Exclude_native_shared_libs...)
	a.Exclude_jni_libs = append(a.Exclude_jni_libs, b.Exclude_jni_libs...)
	a.Exclude_rust_dyn_libs = append(a.Exclude_rust_dyn_libs, b.Exclude_rust_dyn_libs...)
	a.Exclude_binaries = append(a.Exclude_binaries, b.Exclude_binaries...)
	a.Exclude_tests = append(a.Exclude_tests, b.Exclude_tests...)
	a.Exclude_filesystems = append(a.Exclude_filesystems, b.Exclude_filesystems...)
	a.Exclude_prebuilts = append(a.Exclude_prebuilts, b.Exclude_prebuilts...)
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
		Riscv64 struct {
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

	// List of prebuilt files that are embedded inside this APEX bundle.
	Prebuilts []string

	// List of BPF programs inside this APEX bundle.
	Bpfs []string

	// Names of modules to be overridden. Listed modules can only be other binaries (in Make or
	// Soong). This does not completely prevent installation of the overridden binaries, but if
	// both binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will
	// be removed from PRODUCT_PACKAGES.
	Overrides []string

	Multilib apexMultilibProperties

	// Logging parent value.
	Logging_parent string

	// Apex Container package name. Override value for attribute package:name in
	// AndroidManifest.xml
	Package_name string

	// A txt file containing list of files that are allowed to be included in this APEX.
	Allowed_files *string `android:"path"`

	// Name of the apex_key module that provides the private key to sign this APEX bundle.
	Key *string

	// Specifies the certificate and the private key to sign the zip container of this APEX. If
	// this is "foo", foo.x509.pem and foo.pk8 under PRODUCT_DEFAULT_DEV_CERTIFICATE are used
	// as the certificate and the private key, respectively. If this is ":module", then the
	// certificate and the private key are provided from the android_app_certificate module
	// named "module".
	Certificate *string

	// Whether this APEX can be compressed or not. Setting this property to false means this
	// APEX will never be compressed. When set to true, APEX will be compressed if other
	// conditions, e.g., target device needs to support APEX compression, are also fulfilled.
	// Default: false.
	Compressible *bool

	// Trim against a specific Dynamic Common Lib APEX
	Trim_against *string

	// The minimum SDK version that this APEX must support at minimum. This is usually set to
	// the SDK version that the APEX was first introduced.
	Min_sdk_version *string
}

type apexBundle struct {
	// Inherited structs
	android.ModuleBase
	android.DefaultableModuleBase
	android.OverridableModuleBase
	multitree.ExportableModuleBase

	// Properties
	properties            apexBundleProperties
	targetProperties      apexTargetBundleProperties
	archProperties        apexArchBundleProperties
	overridableProperties overridableProperties
	vndkProperties        apexVndkProperties // only for apex_vndk modules

	///////////////////////////////////////////////////////////////////////////////////////////
	// Inputs

	// Keys for apex_payload.img
	publicKeyFile  android.Path
	privateKeyFile android.Path

	// Cert/priv-key for the zip container
	containerCertificateFile android.Path
	containerPrivateKeyFile  android.Path

	// Flags for special variants of APEX
	testApex bool
	vndkApex bool

	// File system type of apex_payload.img
	payloadFsType fsType

	// Whether to create symlink to the system file instead of having a file inside the apex or
	// not
	linkToSystemLib bool

	// List of files to be included in this APEX. This is filled in the first part of
	// GenerateAndroidBuildActions.
	filesInfo []apexFile

	// List of other module names that should be installed when this APEX gets installed (LOCAL_REQUIRED_MODULES).
	makeModulesToInstall []string

	///////////////////////////////////////////////////////////////////////////////////////////
	// Outputs (final and intermediates)

	// Processed apex manifest in JSONson format (for Q)
	manifestJsonOut android.WritablePath

	// Processed apex manifest in PB format (for R+)
	manifestPbOut android.WritablePath

	// Processed file_contexts files
	fileContexts android.WritablePath

	// The built APEX file. This is the main product.
	// Could be .apex or .capex
	outputFile android.WritablePath

	// The built uncompressed .apex file.
	outputApexFile android.WritablePath

	// The built APEX file in app bundle format. This file is not directly installed to the
	// device. For an APEX, multiple app bundles are created each of which is for a specific ABI
	// like arm, arm64, x86, etc. Then they are processed again (outside of the Android build
	// system) to be merged into a single app bundle file that Play accepts. See
	// vendor/google/build/build_unbundled_mainline_module.sh for more detail.
	bundleModuleFile android.WritablePath

	// Target directory to install this APEX. Usually out/target/product/<device>/<partition>/apex.
	installDir android.InstallPath

	// Path where this APEX was installed.
	installedFile android.InstallPath

	// fragment for this apex for apexkeys.txt
	apexKeysPath android.WritablePath

	// Installed locations of symlinks for backward compatibility.
	compatSymlinks android.InstallPaths

	// Text file having the list of individual files that are included in this APEX. Used for
	// debugging purpose.
	installedFilesFile android.WritablePath

	// List of module names that this APEX is including (to be shown via *-deps-info target).
	// Used for debugging purpose.
	android.ApexBundleDepsInfo

	// Optional list of lint report zip files for apexes that contain java or app modules
	lintReports android.Paths

	isCompressed bool

	// Path of API coverage generate file
	nativeApisUsedByModuleFile   android.ModuleOutPath
	nativeApisBackedByModuleFile android.ModuleOutPath
	javaApisUsedByModuleFile     android.ModuleOutPath

	aconfigFiles []android.Path

	// Required modules, filled out during GenerateAndroidBuildActions and used in AndroidMk
	required []string
}

// apexFileClass represents a type of file that can be included in APEX.
type apexFileClass int

const (
	app apexFileClass = iota
	appSet
	etc
	javaSharedLib
	nativeExecutable
	nativeSharedLib
	nativeTest
	shBinary
)

var (
	classes = map[string]apexFileClass{
		"app":              app,
		"appSet":           appSet,
		"etc":              etc,
		"javaSharedLib":    javaSharedLib,
		"nativeExecutable": nativeExecutable,
		"nativeSharedLib":  nativeSharedLib,
		"nativeTest":       nativeTest,
		"shBinary":         shBinary,
	}
)

// apexFile represents a file in an APEX bundle. This is created during the first half of
// GenerateAndroidBuildActions by traversing the dependencies of the APEX. Then in the second half
// of the function, this is used to create commands that copies the files into a staging directory,
// where they are packaged into the APEX file.
type apexFile struct {
	// buildFile is put in the installDir inside the APEX.
	builtFile  android.Path
	installDir string
	partition  string
	customStem string
	symlinks   []string // additional symlinks

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
		ret.moduleDir = ctx.OtherModuleDir(module)
		ret.partition = module.PartitionTag(ctx.DeviceConfig())
		ret.requiredModuleNames = module.RequiredModuleNames(ctx)
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

	// True if the dependent can only be a source module, false if a prebuilt module is a suitable
	// replacement. This is needed because some prebuilt modules do not provide all the information
	// needed by the apex.
	sourceOnly bool

	// If not-nil and an APEX is a member of an SDK then dependencies of that APEX with this tag will
	// also be added as exported members of that SDK.
	memberType android.SdkMemberType
}

func (d *dependencyTag) SdkMemberType(_ android.Module) android.SdkMemberType {
	return d.memberType
}

func (d *dependencyTag) ExportMember() bool {
	return true
}

func (d *dependencyTag) String() string {
	return fmt.Sprintf("apex.dependencyTag{%q}", d.name)
}

func (d *dependencyTag) ReplaceSourceWithPrebuilt() bool {
	return !d.sourceOnly
}

var _ android.ReplaceSourceWithPrebuilt = &dependencyTag{}
var _ android.SdkMemberDependencyTag = &dependencyTag{}

var (
	androidAppTag   = &dependencyTag{name: "androidApp", payload: true}
	bpfTag          = &dependencyTag{name: "bpf", payload: true}
	certificateTag  = &dependencyTag{name: "certificate"}
	dclaTag         = &dependencyTag{name: "dcla"}
	executableTag   = &dependencyTag{name: "executable", payload: true}
	fsTag           = &dependencyTag{name: "filesystem", payload: true}
	bcpfTag         = &dependencyTag{name: "bootclasspathFragment", payload: true, sourceOnly: true, memberType: java.BootclasspathFragmentSdkMemberType}
	sscpfTag        = &dependencyTag{name: "systemserverclasspathFragment", payload: true, sourceOnly: true, memberType: java.SystemServerClasspathFragmentSdkMemberType}
	compatConfigTag = &dependencyTag{name: "compatConfig", payload: true, sourceOnly: true, memberType: java.CompatConfigSdkMemberType}
	javaLibTag      = &dependencyTag{name: "javaLib", payload: true}
	jniLibTag       = &dependencyTag{name: "jniLib", payload: true}
	keyTag          = &dependencyTag{name: "key"}
	prebuiltTag     = &dependencyTag{name: "prebuilt", payload: true}
	rroTag          = &dependencyTag{name: "rro", payload: true}
	sharedLibTag    = &dependencyTag{name: "sharedLib", payload: true}
	testForTag      = &dependencyTag{name: "test for"}
	testTag         = &dependencyTag{name: "test", payload: true}
	shBinaryTag     = &dependencyTag{name: "shBinary", payload: true}
)

// TODO(jiyong): shorten this function signature
func addDependenciesForNativeModules(ctx android.BottomUpMutatorContext, nativeModules ApexNativeDependencies, target android.Target, imageVariation string) {
	binVariations := target.Variations()
	libVariations := append(target.Variations(), blueprint.Variation{Mutator: "link", Variation: "shared"})
	rustLibVariations := append(
		target.Variations(), []blueprint.Variation{
			{Mutator: "rust_libraries", Variation: "dylib"},
			{Mutator: "link", Variation: ""},
		}...,
	)

	// Append "image" variation
	binVariations = append(binVariations, blueprint.Variation{Mutator: "image", Variation: imageVariation})
	libVariations = append(libVariations, blueprint.Variation{Mutator: "image", Variation: imageVariation})
	rustLibVariations = append(rustLibVariations, blueprint.Variation{Mutator: "image", Variation: imageVariation})

	// Use *FarVariation* to be able to depend on modules having conflicting variations with
	// this module. This is required since arch variant of an APEX bundle is 'common' but it is
	// 'arm' or 'arm64' for native shared libs.
	ctx.AddFarVariationDependencies(binVariations, executableTag,
		android.RemoveListFromList(nativeModules.Binaries, nativeModules.Exclude_binaries)...)
	ctx.AddFarVariationDependencies(binVariations, testTag,
		android.RemoveListFromList(nativeModules.Tests, nativeModules.Exclude_tests)...)
	ctx.AddFarVariationDependencies(libVariations, jniLibTag,
		android.RemoveListFromList(nativeModules.Jni_libs, nativeModules.Exclude_jni_libs)...)
	ctx.AddFarVariationDependencies(libVariations, sharedLibTag,
		android.RemoveListFromList(nativeModules.Native_shared_libs, nativeModules.Exclude_native_shared_libs)...)
	ctx.AddFarVariationDependencies(rustLibVariations, sharedLibTag,
		android.RemoveListFromList(nativeModules.Rust_dyn_libs, nativeModules.Exclude_rust_dyn_libs)...)
	ctx.AddFarVariationDependencies(target.Variations(), fsTag,
		android.RemoveListFromList(nativeModules.Filesystems, nativeModules.Exclude_filesystems)...)
	ctx.AddFarVariationDependencies(target.Variations(), prebuiltTag,
		android.RemoveListFromList(nativeModules.Prebuilts, nativeModules.Exclude_prebuilts)...)
}

func (a *apexBundle) combineProperties(ctx android.BottomUpMutatorContext) {
	proptools.AppendProperties(&a.properties.Multilib, &a.targetProperties.Target.Android.Multilib, nil)
}

// getImageVariationPair returns a pair for the image variation name as its
// prefix and suffix. The prefix indicates whether it's core/vendor/product and the
// suffix indicates the vndk version for vendor/product if vndk is enabled.
// getImageVariation can simply join the result of this function to get the
// image variation name.
func (a *apexBundle) getImageVariationPair() (string, string) {
	if a.vndkApex {
		return cc.VendorVariationPrefix, a.vndkVersion()
	}

	prefix := android.CoreVariation
	if a.SocSpecific() || a.DeviceSpecific() {
		prefix = cc.VendorVariation
	} else if a.ProductSpecific() {
		prefix = cc.ProductVariation
	}

	return prefix, ""
}

// getImageVariation returns the image variant name for this apexBundle. In most cases, it's simply
// android.CoreVariation, but gets complicated for the vendor APEXes and the VNDK APEX.
func (a *apexBundle) getImageVariation() string {
	prefix, vndkVersion := a.getImageVariationPair()
	return prefix + vndkVersion
}

func (a *apexBundle) DepsMutator(ctx android.BottomUpMutatorContext) {
	// apexBundle is a multi-arch targets module. Arch variant of apexBundle is set to 'common'.
	// arch-specific targets are enabled by the compile_multilib setting of the apex bundle. For
	// each target os/architectures, appropriate dependencies are selected by their
	// target.<os>.multilib.<type> groups and are added as (direct) dependencies.
	targets := ctx.MultiTargets()
	imageVariation := a.getImageVariation()

	a.combineProperties(ctx)

	has32BitTarget := false
	for _, target := range targets {
		if target.Arch.ArchType.Multilib == "lib32" {
			has32BitTarget = true
		}
	}
	for i, target := range targets {
		var deps ApexNativeDependencies

		// Add native modules targeting both ABIs. When multilib.* is omitted for
		// native_shared_libs/jni_libs/tests, it implies multilib.both
		deps.Merge(a.properties.Multilib.Both)
		deps.Merge(ApexNativeDependencies{
			Native_shared_libs: a.properties.Native_shared_libs,
			Tests:              a.properties.Tests,
			Jni_libs:           a.properties.Jni_libs,
			Binaries:           nil,
		})

		// Add native modules targeting the first ABI When multilib.* is omitted for
		// binaries, it implies multilib.first
		isPrimaryAbi := i == 0
		if isPrimaryAbi {
			deps.Merge(a.properties.Multilib.First)
			deps.Merge(ApexNativeDependencies{
				Native_shared_libs: nil,
				Tests:              nil,
				Jni_libs:           nil,
				Binaries:           a.properties.Binaries,
			})
		}

		// Add native modules targeting either 32-bit or 64-bit ABI
		switch target.Arch.ArchType.Multilib {
		case "lib32":
			deps.Merge(a.properties.Multilib.Lib32)
			deps.Merge(a.properties.Multilib.Prefer32)
		case "lib64":
			deps.Merge(a.properties.Multilib.Lib64)
			if !has32BitTarget {
				deps.Merge(a.properties.Multilib.Prefer32)
			}
		}

		// Add native modules targeting a specific arch variant
		switch target.Arch.ArchType {
		case android.Arm:
			deps.Merge(a.archProperties.Arch.Arm.ApexNativeDependencies)
		case android.Arm64:
			deps.Merge(a.archProperties.Arch.Arm64.ApexNativeDependencies)
		case android.Riscv64:
			deps.Merge(a.archProperties.Arch.Riscv64.ApexNativeDependencies)
		case android.X86:
			deps.Merge(a.archProperties.Arch.X86.ApexNativeDependencies)
		case android.X86_64:
			deps.Merge(a.archProperties.Arch.X86_64.ApexNativeDependencies)
		default:
			panic(fmt.Errorf("unsupported arch %v\n", ctx.Arch().ArchType))
		}

		addDependenciesForNativeModules(ctx, deps, target, imageVariation)
		if isPrimaryAbi {
			ctx.AddFarVariationDependencies([]blueprint.Variation{
				{Mutator: "os", Variation: target.OsVariation()},
				{Mutator: "arch", Variation: target.ArchVariation()},
			}, shBinaryTag, a.properties.Sh_binaries...)
		}
	}

	// Common-arch dependencies come next
	commonVariation := ctx.Config().AndroidCommonTarget.Variations()
	ctx.AddFarVariationDependencies(commonVariation, rroTag, a.properties.Rros...)
	ctx.AddFarVariationDependencies(commonVariation, bcpfTag, a.properties.Bootclasspath_fragments...)
	ctx.AddFarVariationDependencies(commonVariation, sscpfTag, a.properties.Systemserverclasspath_fragments...)
	ctx.AddFarVariationDependencies(commonVariation, javaLibTag, a.properties.Java_libs...)
	ctx.AddFarVariationDependencies(commonVariation, fsTag, a.properties.Filesystems...)
	ctx.AddFarVariationDependencies(commonVariation, compatConfigTag, a.properties.Compat_configs...)
}

// DepsMutator for the overridden properties.
func (a *apexBundle) OverridablePropertiesDepsMutator(ctx android.BottomUpMutatorContext) {
	if a.overridableProperties.Allowed_files != nil {
		android.ExtractSourceDeps(ctx, a.overridableProperties.Allowed_files)
	}

	commonVariation := ctx.Config().AndroidCommonTarget.Variations()
	ctx.AddFarVariationDependencies(commonVariation, androidAppTag, a.overridableProperties.Apps...)
	ctx.AddFarVariationDependencies(commonVariation, bpfTag, a.overridableProperties.Bpfs...)
	if prebuilts := a.overridableProperties.Prebuilts; len(prebuilts) > 0 {
		// For prebuilt_etc, use the first variant (64 on 64/32bit device, 32 on 32bit device)
		// regardless of the TARGET_PREFER_* setting. See b/144532908
		arches := ctx.DeviceConfig().Arches()
		if len(arches) != 0 {
			archForPrebuiltEtc := arches[0]
			for _, arch := range arches {
				// Prefer 64-bit arch if there is any
				if arch.ArchType.Multilib == "lib64" {
					archForPrebuiltEtc = arch
					break
				}
			}
			ctx.AddFarVariationDependencies([]blueprint.Variation{
				{Mutator: "os", Variation: ctx.Os().String()},
				{Mutator: "arch", Variation: archForPrebuiltEtc.String()},
			}, prebuiltTag, prebuilts...)
		}
	}

	// Dependencies for signing
	if String(a.overridableProperties.Key) == "" {
		ctx.PropertyErrorf("key", "missing")
		return
	}
	ctx.AddDependency(ctx.Module(), keyTag, String(a.overridableProperties.Key))

	cert := android.SrcIsModule(a.getCertString(ctx))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
		// empty cert is not an error. Cert and private keys will be directly found under
		// PRODUCT_DEFAULT_DEV_CERTIFICATE
	}
}

func apexDCLADepsMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Config().ApexTrimEnabled() {
		return
	}
	if a, ok := mctx.Module().(*apexBundle); ok && a.overridableProperties.Trim_against != nil {
		commonVariation := mctx.Config().AndroidCommonTarget.Variations()
		mctx.AddFarVariationDependencies(commonVariation, dclaTag, String(a.overridableProperties.Trim_against))
	} else if o, ok := mctx.Module().(*OverrideApex); ok {
		for _, p := range o.GetProperties() {
			properties, ok := p.(*overridableProperties)
			if !ok {
				continue
			}
			if properties.Trim_against != nil {
				commonVariation := mctx.Config().AndroidCommonTarget.Variations()
				mctx.AddFarVariationDependencies(commonVariation, dclaTag, String(properties.Trim_against))
			}
		}
	}
}

type DCLAInfo struct {
	ProvidedLibs []string
}

var DCLAInfoProvider = blueprint.NewMutatorProvider[DCLAInfo]("apex_info")

var _ ApexInfoMutator = (*apexBundle)(nil)

func (a *apexBundle) ApexVariationName() string {
	return a.properties.ApexVariationName
}

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
	if proptools.Bool(a.properties.Use_vndk_as_stable) {
		if !useVndk {
			mctx.PropertyErrorf("use_vndk_as_stable", "not supported for system/system_ext APEXes")
		}
		if a.minSdkVersionValue(mctx) != "" {
			mctx.PropertyErrorf("use_vndk_as_stable", "not supported when min_sdk_version is set")
		}
		if mctx.Failed() {
			return
		}
	}

	continueApexDepsWalk := func(child, parent android.Module) bool {
		am, ok := child.(android.ApexModule)
		if !ok || !am.CanHaveApexVariants() {
			return false
		}
		depTag := mctx.OtherModuleDependencyTag(child)

		// Check to see if the tag always requires that the child module has an apex variant for every
		// apex variant of the parent module. If it does not then it is still possible for something
		// else, e.g. the DepIsInSameApex(...) method to decide that a variant is required.
		if required, ok := depTag.(android.AlwaysRequireApexVariantTag); ok && required.AlwaysRequireApexVariant() {
			return true
		}
		if !android.IsDepInSameApex(mctx, parent, child) {
			return false
		}

		if useVndk && child.Name() == "libbinder" {
			mctx.ModuleErrorf("Module %s in the vendor APEX %s should not use libbinder. Use libbinder_ndk instead.", parent.Name(), a.Name())
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
	android.SetProvider(mctx, android.ApexBundleInfoProvider, android.ApexBundleInfo{
		Contents: apexContents,
	})

	minSdkVersion := a.minSdkVersion(mctx)
	// When min_sdk_version is not set, the apex is built against FutureApiLevel.
	if minSdkVersion.IsNone() {
		minSdkVersion = android.FutureApiLevel
	}

	// This is the main part of this mutator. Mark the collected dependencies that they need to
	// be built for this apexBundle.

	apexVariationName := mctx.ModuleName() // could be com.android.foo
	if overridable, ok := mctx.Module().(android.OverridableModule); ok && overridable.GetOverriddenBy() != "" {
		// use the overridden name com.mycompany.android.foo
		apexVariationName = overridable.GetOverriddenBy()
	}

	a.properties.ApexVariationName = apexVariationName
	testApexes := []string{}
	if a.testApex {
		testApexes = []string{apexVariationName}
	}
	apexInfo := android.ApexInfo{
		ApexVariationName: apexVariationName,
		MinSdkVersion:     minSdkVersion,
		Updatable:         a.Updatable(),
		UsePlatformApis:   a.UsePlatformApis(),
		InApexVariants:    []string{apexVariationName},
		InApexModules:     []string{a.Name()}, // could be com.mycompany.android.foo
		ApexContents:      []*android.ApexContents{apexContents},
		TestApexes:        testApexes,
	}
	mctx.WalkDeps(func(child, parent android.Module) bool {
		if !continueApexDepsWalk(child, parent) {
			return false
		}
		child.(android.ApexModule).BuildForApex(apexInfo) // leave a mark!
		return true
	})

	if a.dynamic_common_lib_apex() {
		android.SetProvider(mctx, DCLAInfoProvider, DCLAInfo{
			ProvidedLibs: a.properties.Native_shared_libs,
		})
	}
}

type ApexInfoMutator interface {
	// ApexVariationName returns the name of the APEX variation to use in the apex
	// mutator etc. It is the same name as ApexInfo.ApexVariationName.
	ApexVariationName() string

	// ApexInfoMutator implementations must call BuildForApex(ApexInfo) on any modules that are
	// depended upon by an apex and which require an apex specific variant.
	ApexInfoMutator(android.TopDownMutatorContext)
}

// apexInfoMutator delegates the work of identifying which modules need an ApexInfo and apex
// specific variant to modules that support the ApexInfoMutator.
// It also propagates updatable=true to apps of updatable apexes
func apexInfoMutator(mctx android.TopDownMutatorContext) {
	if !mctx.Module().Enabled(mctx) {
		return
	}

	if a, ok := mctx.Module().(ApexInfoMutator); ok {
		a.ApexInfoMutator(mctx)
	}

	if am, ok := mctx.Module().(android.ApexModule); ok {
		android.ApexInfoMutator(mctx, am)
	}
	enforceAppUpdatability(mctx)
}

// apexStrictUpdatibilityLintMutator propagates strict_updatability_linting to transitive deps of a mainline module
// This check is enforced for updatable modules
func apexStrictUpdatibilityLintMutator(mctx android.TopDownMutatorContext) {
	if !mctx.Module().Enabled(mctx) {
		return
	}
	if apex, ok := mctx.Module().(*apexBundle); ok && apex.checkStrictUpdatabilityLinting(mctx) {
		mctx.WalkDeps(func(child, parent android.Module) bool {
			// b/208656169 Do not propagate strict updatability linting to libcore/
			// These libs are available on the classpath during compilation
			// These libs are transitive deps of the sdk. See java/sdk.go:decodeSdkDep
			// Only skip libraries defined in libcore root, not subdirectories
			if mctx.OtherModuleDir(child) == "libcore" {
				// Do not traverse transitive deps of libcore/ libs
				return false
			}
			if android.InList(child.Name(), skipLintJavalibAllowlist) {
				return false
			}
			if lintable, ok := child.(java.LintDepSetsIntf); ok {
				lintable.SetStrictUpdatabilityLinting(true)
			}
			// visit transitive deps
			return true
		})
	}
}

// enforceAppUpdatability propagates updatable=true to apps of updatable apexes
func enforceAppUpdatability(mctx android.TopDownMutatorContext) {
	if !mctx.Module().Enabled(mctx) {
		return
	}
	if apex, ok := mctx.Module().(*apexBundle); ok && apex.Updatable() {
		// checking direct deps is sufficient since apex->apk is a direct edge, even when inherited via apex_defaults
		mctx.VisitDirectDeps(func(module android.Module) {
			// ignore android_test_app
			if app, ok := module.(*java.AndroidApp); ok {
				app.SetUpdatable(true)
			}
		})
	}
}

// TODO: b/215736885 Whittle the denylist
// Transitive deps of certain mainline modules baseline NewApi errors
// Skip these mainline modules for now
var (
	skipStrictUpdatabilityLintAllowlist = []string{
		// go/keep-sorted start
		"PackageManagerTestApex",
		"com.android.adservices",
		"com.android.appsearch",
		"com.android.art",
		"com.android.art.debug",
		"com.android.btservices",
		"com.android.cellbroadcast",
		"com.android.configinfrastructure",
		"com.android.conscrypt",
		"com.android.extservices",
		"com.android.extservices_tplus",
		"com.android.healthfitness",
		"com.android.ipsec",
		"com.android.media",
		"com.android.mediaprovider",
		"com.android.ondevicepersonalization",
		"com.android.os.statsd",
		"com.android.permission",
		"com.android.profiling",
		"com.android.rkpd",
		"com.android.scheduling",
		"com.android.tethering",
		"com.android.uwb",
		"com.android.wifi",
		"test_com.android.art",
		"test_com.android.cellbroadcast",
		"test_com.android.conscrypt",
		"test_com.android.extservices",
		"test_com.android.ipsec",
		"test_com.android.media",
		"test_com.android.mediaprovider",
		"test_com.android.os.statsd",
		"test_com.android.permission",
		"test_com.android.wifi",
		"test_jitzygote_com.android.art",
		// go/keep-sorted end
	}

	// TODO: b/215736885 Remove this list
	skipLintJavalibAllowlist = []string{
		"conscrypt.module.platform.api.stubs",
		"conscrypt.module.public.api.stubs",
		"conscrypt.module.public.api.stubs.system",
		"conscrypt.module.public.api.stubs.module_lib",
		"framework-media.stubs",
		"framework-media.stubs.system",
		"framework-media.stubs.module_lib",
	}
)

func (a *apexBundle) checkStrictUpdatabilityLinting(mctx android.TopDownMutatorContext) bool {
	// The allowlist contains the base apex name, so use that instead of the ApexVariationName
	return a.Updatable() && !android.InList(mctx.ModuleName(), skipStrictUpdatabilityLintAllowlist)
}

// apexUniqueVariationsMutator checks if any dependencies use unique apex variations. If so, use
// unique apex variations for this module. See android/apex.go for more about unique apex variant.
// TODO(jiyong): move this to android/apex.go?
func apexUniqueVariationsMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Module().Enabled(mctx) {
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
	if !mctx.Module().Enabled(mctx) {
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
	if !mctx.Module().Enabled(mctx) {
		return
	}
	if _, ok := mctx.Module().(android.ApexModule); ok {
		var contents []*android.ApexContents
		for _, testFor := range mctx.GetDirectDepsWithTag(testForTag) {
			abInfo, _ := android.OtherModuleProvider(mctx, testFor, android.ApexBundleInfoProvider)
			contents = append(contents, abInfo.Contents)
		}
		android.SetProvider(mctx, android.ApexTestForInfoProvider, android.ApexTestForInfo{
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
	// Recovery is not considered as platform
	if mctx.Module().InstallInRecovery() {
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
		if !android.IsDepInSameApex(mctx, am, child) {
			// if the dependency crosses apex boundary, don't consider it
			return
		}
		if dep, ok := child.(android.ApexModule); ok && dep.NotAvailableForPlatform() {
			availableToPlatform = false
			// TODO(b/154889534) trigger an error when 'am' has
			// "//apex_available:platform"
		}
	})

	// Exception 1: check to see if the module always requires it.
	if am.AlwaysRequiresPlatformApexVariant() {
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

type apexTransitionMutator struct{}

func (a *apexTransitionMutator) Split(ctx android.BaseModuleContext) []string {
	// apexBundle itself is mutated so that it and its dependencies have the same apex variant.
	if ai, ok := ctx.Module().(ApexInfoMutator); ok && apexModuleTypeRequiresVariant(ai) {
		if overridable, ok := ctx.Module().(android.OverridableModule); ok && overridable.GetOverriddenBy() != "" {
			return []string{overridable.GetOverriddenBy()}
		}
		return []string{ai.ApexVariationName()}
	} else if _, ok := ctx.Module().(*OverrideApex); ok {
		return []string{ctx.ModuleName()}
	}
	return []string{""}
}

func (a *apexTransitionMutator) OutgoingTransition(ctx android.OutgoingTransitionContext, sourceVariation string) string {
	return sourceVariation
}

func (a *apexTransitionMutator) IncomingTransition(ctx android.IncomingTransitionContext, incomingVariation string) string {
	if am, ok := ctx.Module().(android.ApexModule); ok && am.CanHaveApexVariants() {
		return android.IncomingApexTransition(ctx, incomingVariation)
	} else if ai, ok := ctx.Module().(ApexInfoMutator); ok {
		if overridable, ok := ctx.Module().(android.OverridableModule); ok && overridable.GetOverriddenBy() != "" {
			return overridable.GetOverriddenBy()
		}
		return ai.ApexVariationName()
	} else if _, ok := ctx.Module().(*OverrideApex); ok {
		return ctx.Module().Name()
	}

	return ""
}

func (a *apexTransitionMutator) Mutate(ctx android.BottomUpMutatorContext, variation string) {
	if am, ok := ctx.Module().(android.ApexModule); ok && am.CanHaveApexVariants() {
		android.MutateApexTransition(ctx, variation)
	}
}

// apexModuleTypeRequiresVariant determines whether the module supplied requires an apex specific
// variant.
func apexModuleTypeRequiresVariant(module ApexInfoMutator) bool {
	if a, ok := module.(*apexBundle); ok {
		// TODO(jiyong): document the reason why the VNDK APEX is an exception here.
		return !a.vndkApex
	}

	return true
}

// See android.UpdateDirectlyInAnyApex
// TODO(jiyong): move this to android/apex.go?
func apexDirectlyInAnyMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Module().Enabled(mctx) {
		return
	}
	if am, ok := mctx.Module().(android.ApexModule); ok {
		android.UpdateDirectlyInAnyApex(mctx, am)
	}
}

const (
	// File extensions of an APEX for different packaging methods
	imageApexSuffix  = ".apex"
	imageCapexSuffix = ".capex"

	// variant names each of which is for a packaging method
	imageApexType = "image"

	ext4FsType  = "ext4"
	f2fsFsType  = "f2fs"
	erofsFsType = "erofs"
)

var _ android.DepIsInSameApex = (*apexBundle)(nil)

// Implements android.DepInInSameApex
func (a *apexBundle) DepIsInSameApex(_ android.BaseModuleContext, _ android.Module) bool {
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
	case imageApexSuffix:
		// uncompressed one
		if a.outputApexFile != nil {
			return android.Paths{a.outputApexFile}, nil
		}
		fallthrough
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

var _ multitree.Exportable = (*apexBundle)(nil)

func (a *apexBundle) Exportable() bool {
	return true
}

func (a *apexBundle) TaggedOutputs() map[string]android.Paths {
	ret := make(map[string]android.Paths)
	ret["apex"] = android.Paths{a.outputFile}
	return ret
}

var _ cc.Coverage = (*apexBundle)(nil)

// Implements cc.Coverage
func (a *apexBundle) IsNativeCoverageNeeded(ctx android.IncomingTransitionContext) bool {
	return ctx.DeviceConfig().NativeCoverageEnabled()
}

// Implements cc.Coverage
func (a *apexBundle) SetPreventInstall() {
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

// Implements android.ApexBundleDepsInfoIntf
func (a *apexBundle) Updatable() bool {
	return proptools.BoolDefault(a.properties.Updatable, true)
}

func (a *apexBundle) FutureUpdatable() bool {
	return proptools.BoolDefault(a.properties.Future_updatable, false)
}

func (a *apexBundle) UsePlatformApis() bool {
	return proptools.BoolDefault(a.properties.Platform_apis, false)
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
	return String(a.overridableProperties.Certificate)
}

// See the installable property
func (a *apexBundle) installable() bool {
	return !a.properties.PreventInstall && (a.properties.Installable == nil || proptools.Bool(a.properties.Installable))
}

// See the generate_hashtree property
func (a *apexBundle) shouldGenerateHashtree() bool {
	return proptools.BoolDefault(a.properties.Generate_hashtree, true)
}

// See the test_only_unsigned_payload property
func (a *apexBundle) testOnlyShouldSkipPayloadSign() bool {
	return proptools.Bool(a.properties.Test_only_unsigned_payload)
}

// See the test_only_force_compression property
func (a *apexBundle) testOnlyShouldForceCompression() bool {
	return proptools.Bool(a.properties.Test_only_force_compression)
}

// See the dynamic_common_lib_apex property
func (a *apexBundle) dynamic_common_lib_apex() bool {
	return proptools.BoolDefault(a.properties.Dynamic_common_lib_apex, false)
}

// See the list of libs to trim
func (a *apexBundle) libs_to_trim(ctx android.ModuleContext) []string {
	dclaModules := ctx.GetDirectDepsWithTag(dclaTag)
	if len(dclaModules) > 1 {
		panic(fmt.Errorf("expected exactly at most one dcla dependency, got %d", len(dclaModules)))
	}
	if len(dclaModules) > 0 {
		DCLAInfo, _ := android.OtherModuleProvider(ctx, dclaModules[0], DCLAInfoProvider)
		return DCLAInfo.ProvidedLibs
	}
	return []string{}
}

// These functions are interfacing with cc/sanitizer.go. The entire APEX (along with all of its
// members) can be sanitized, either forcibly, or by the global configuration. For some of the
// sanitizers, extra dependencies can be forcibly added as well.

func (a *apexBundle) EnableSanitizer(sanitizerName string) {
	if !android.InList(sanitizerName, a.properties.SanitizerNames) {
		a.properties.SanitizerNames = append(a.properties.SanitizerNames, sanitizerName)
	}
}

func (a *apexBundle) IsSanitizerEnabled(config android.Config, sanitizerName string) bool {
	if android.InList(sanitizerName, a.properties.SanitizerNames) {
		return true
	}

	// Then follow the global setting
	var globalSanitizerNames []string
	arches := config.SanitizeDeviceArch()
	if len(arches) == 0 || android.InList(a.Arch().ArchType.Name, arches) {
		globalSanitizerNames = config.SanitizeDevice()
	}
	return android.InList(sanitizerName, globalSanitizerNames)
}

func (a *apexBundle) AddSanitizerDependencies(ctx android.BottomUpMutatorContext, sanitizerName string) {
	// TODO(jiyong): move this info (the sanitizer name, the lib name, etc.) to cc/sanitize.go
	// Keep only the mechanism here.
	if sanitizerName == "hwaddress" && strings.HasPrefix(a.Name(), "com.android.runtime") {
		imageVariation := a.getImageVariation()
		for _, target := range ctx.MultiTargets() {
			if target.Arch.ArchType.Multilib == "lib64" {
				addDependenciesForNativeModules(ctx, ApexNativeDependencies{
					Native_shared_libs: []string{"libclang_rt.hwasan"},
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
	// This needs to go after the runtime APEX handling because otherwise we would get
	// weird paths like lib64/rel_install_path/bionic rather than
	// lib64/bionic/rel_install_path.
	dirInApex = filepath.Join(dirInApex, ccMod.RelativeInstallPath())

	fileToCopy := android.OutputFileForModule(ctx, ccMod, "")
	androidMkModuleName := ccMod.BaseModuleName() + ccMod.Properties.SubName
	return newApexFile(ctx, fileToCopy, androidMkModuleName, dirInApex, nativeSharedLib, ccMod)
}

func apexFileForExecutable(ctx android.BaseModuleContext, cc *cc.Module) apexFile {
	dirInApex := "bin"
	if cc.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, cc.Target().NativeBridgeRelativePath)
	}
	dirInApex = filepath.Join(dirInApex, cc.RelativeInstallPath())
	fileToCopy := android.OutputFileForModule(ctx, cc, "")
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
	dirInApex = filepath.Join(dirInApex, rustm.RelativeInstallPath())
	fileToCopy := android.OutputFileForModule(ctx, rustm, "")
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
	dirInApex = filepath.Join(dirInApex, rustm.RelativeInstallPath())
	fileToCopy := android.OutputFileForModule(ctx, rustm, "")
	androidMkModuleName := rustm.BaseModuleName() + rustm.Properties.SubName
	return newApexFile(ctx, fileToCopy, androidMkModuleName, dirInApex, nativeSharedLib, rustm)
}

func apexFileForShBinary(ctx android.BaseModuleContext, sh *sh.ShBinary) apexFile {
	dirInApex := filepath.Join("bin", sh.SubDir())
	if sh.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, sh.Target().NativeBridgeRelativePath)
	}
	fileToCopy := sh.OutputFile()
	af := newApexFile(ctx, fileToCopy, sh.BaseModuleName(), dirInApex, shBinary, sh)
	af.symlinks = sh.Symlinks()
	return af
}

func apexFileForPrebuiltEtc(ctx android.BaseModuleContext, prebuilt prebuilt_etc.PrebuiltEtcModule, outputFile android.Path) apexFile {
	dirInApex := filepath.Join(prebuilt.BaseDir(), prebuilt.SubDir())
	makeModuleName := strings.ReplaceAll(filepath.Join(dirInApex, outputFile.Base()), "/", "_")
	return newApexFile(ctx, outputFile, makeModuleName, dirInApex, etc, prebuilt)
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
	DexJarBuildPath(ctx android.ModuleErrorfContext) java.OptionalDexJarPath
	JacocoReportClassesFile() android.Path
	LintDepSets() java.LintDepSets
	Stem() string
}

var _ javaModule = (*java.Library)(nil)
var _ javaModule = (*java.Import)(nil)
var _ javaModule = (*java.SdkLibrary)(nil)
var _ javaModule = (*java.DexImport)(nil)
var _ javaModule = (*java.SdkLibraryImport)(nil)

// apexFileForJavaModule creates an apexFile for a java module's dex implementation jar.
func apexFileForJavaModule(ctx android.ModuleContext, module javaModule) apexFile {
	return apexFileForJavaModuleWithFile(ctx, module, module.DexJarBuildPath(ctx).PathOrNil())
}

// apexFileForJavaModuleWithFile creates an apexFile for a java module with the supplied file.
func apexFileForJavaModuleWithFile(ctx android.ModuleContext, module javaModule, dexImplementationJar android.Path) apexFile {
	dirInApex := "javalib"
	af := newApexFile(ctx, dexImplementationJar, module.BaseModuleName(), dirInApex, javaSharedLib, module)
	af.jacocoReportClassesFile = module.JacocoReportClassesFile()
	af.lintDepSets = module.LintDepSets()
	af.customStem = module.Stem() + ".jar"
	// TODO: b/338641779 - Remove special casing of sdkLibrary once bcpf and sscpf depends
	// on the implementation library
	if sdkLib, ok := module.(*java.SdkLibrary); ok {
		for _, install := range sdkLib.BuiltInstalledForApex() {
			af.requiredModuleNames = append(af.requiredModuleNames, install.FullModuleName())
			install.PackageFile(ctx)
		}
	} else if dexpreopter, ok := module.(java.DexpreopterInterface); ok {
		for _, install := range dexpreopter.DexpreoptBuiltInstalledForApex() {
			af.requiredModuleNames = append(af.requiredModuleNames, install.FullModuleName())
			install.PackageFile(ctx)
		}
	}
	return af
}

func apexFileForJavaModuleProfile(ctx android.BaseModuleContext, module javaModule) *apexFile {
	if dexpreopter, ok := module.(java.DexpreopterInterface); ok {
		if profilePathOnHost := dexpreopter.OutputProfilePathOnHost(); profilePathOnHost != nil {
			dirInApex := "javalib"
			af := newApexFile(ctx, profilePathOnHost, module.BaseModuleName()+"-profile", dirInApex, etc, nil)
			af.customStem = module.Stem() + ".jar.prof"
			return &af
		}
	}
	return nil
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
	LintDepSets() java.LintDepSets
	PrivAppAllowlist() android.OptionalPath
}

var _ androidApp = (*java.AndroidApp)(nil)
var _ androidApp = (*java.AndroidAppImport)(nil)

func sanitizedBuildIdForPath(ctx android.BaseModuleContext) string {
	buildId := ctx.Config().BuildId()

	// The build ID is used as a suffix for a filename, so ensure that
	// the set of characters being used are sanitized.
	// - any word character: [a-zA-Z0-9_]
	// - dots: .
	// - dashes: -
	validRegex := regexp.MustCompile(`^[\w\.\-\_]+$`)
	if !validRegex.MatchString(buildId) {
		ctx.ModuleErrorf("Unable to use build id %s as filename suffix, valid characters are [a-z A-Z 0-9 _ . -].", buildId)
	}
	return buildId
}

func apexFilesForAndroidApp(ctx android.BaseModuleContext, aapp androidApp) []apexFile {
	appDir := "app"
	if aapp.Privileged() {
		appDir = "priv-app"
	}

	// TODO(b/224589412, b/226559955): Ensure that the subdirname is suffixed
	// so that PackageManager correctly invalidates the existing installed apk
	// in favour of the new APK-in-APEX.  See bugs for more information.
	dirInApex := filepath.Join(appDir, aapp.InstallApkName()+"@"+sanitizedBuildIdForPath(ctx))
	fileToCopy := aapp.OutputFile()

	af := newApexFile(ctx, fileToCopy, aapp.BaseModuleName(), dirInApex, app, aapp)
	af.jacocoReportClassesFile = aapp.JacocoReportClassesFile()
	af.lintDepSets = aapp.LintDepSets()
	af.certificate = aapp.Certificate()

	if app, ok := aapp.(interface {
		OverriddenManifestPackageName() string
	}); ok {
		af.overriddenPackageName = app.OverriddenManifestPackageName()
	}

	apexFiles := []apexFile{}

	if allowlist := aapp.PrivAppAllowlist(); allowlist.Valid() {
		dirInApex := filepath.Join("etc", "permissions")
		privAppAllowlist := newApexFile(ctx, allowlist.Path(), aapp.BaseModuleName()+"_privapp", dirInApex, etc, aapp)
		apexFiles = append(apexFiles, privAppAllowlist)
	}

	apexFiles = append(apexFiles, af)

	return apexFiles
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

func apexFileForBpfProgram(ctx android.BaseModuleContext, builtFile android.Path, apex_sub_dir string, bpfProgram bpf.BpfModule) apexFile {
	dirInApex := filepath.Join("etc", "bpf", apex_sub_dir)
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
		if dt, ok := depTag.(*dependencyTag); ok && !dt.payload {
			return false
		}
		if depTag == android.RequiredDepTag {
			return false
		}

		ai, _ := android.OtherModuleProvider(ctx, child, android.ApexInfoProvider)
		externalDep := !android.InList(ctx.ModuleName(), ai.InApexVariants)

		// Visit actually
		return do(ctx, parent, am, externalDep)
	})
}

// filesystem type of the apex_payload.img inside the APEX. Currently, ext4 and f2fs are supported.
type fsType int

const (
	ext4 fsType = iota
	f2fs
	erofs
)

func (f fsType) string() string {
	switch f {
	case ext4:
		return ext4FsType
	case f2fs:
		return f2fsFsType
	case erofs:
		return erofsFsType
	default:
		panic(fmt.Errorf("unknown APEX payload type %d", f))
	}
}

func (a *apexBundle) setCompression(ctx android.ModuleContext) {
	if a.testOnlyShouldForceCompression() {
		a.isCompressed = true
	} else {
		a.isCompressed = ctx.Config().ApexCompressionEnabled() && a.isCompressable()
	}
}

func (a *apexBundle) setSystemLibLink(ctx android.ModuleContext) {
	// Optimization. If we are building bundled APEX, for the files that are gathered due to the
	// transitive dependencies, don't place them inside the APEX, but place a symlink pointing
	// the same library in the system partition, thus effectively sharing the same libraries
	// across the APEX boundary. For unbundled APEX, all the gathered files are actually placed
	// in the APEX.
	a.linkToSystemLib = !ctx.Config().UnbundledBuild() && a.installable()

	// APEXes targeting other than system/system_ext partitions use vendor/product variants.
	// So we can't link them to /system/lib libs which are core variants.
	if a.SocSpecific() || a.DeviceSpecific() || (a.ProductSpecific() && ctx.Config().EnforceProductPartitionInterface()) {
		a.linkToSystemLib = false
	}

	forced := ctx.Config().ForceApexSymlinkOptimization()
	updatable := a.Updatable() || a.FutureUpdatable()

	// We don't need the optimization for updatable APEXes, as it might give false signal
	// to the system health when the APEXes are still bundled (b/149805758).
	if !forced && updatable {
		a.linkToSystemLib = false
	}
}

func (a *apexBundle) setPayloadFsType(ctx android.ModuleContext) {
	switch proptools.StringDefault(a.properties.Payload_fs_type, ext4FsType) {
	case ext4FsType:
		a.payloadFsType = ext4
	case f2fsFsType:
		a.payloadFsType = f2fs
	case erofsFsType:
		a.payloadFsType = erofs
	default:
		ctx.PropertyErrorf("payload_fs_type", "%q is not a valid filesystem for apex [ext4, f2fs, erofs]", *a.properties.Payload_fs_type)
	}
}

func (a *apexBundle) isCompressable() bool {
	return proptools.BoolDefault(a.overridableProperties.Compressible, false) && !a.testApex
}

func (a *apexBundle) commonBuildActions(ctx android.ModuleContext) bool {
	a.checkApexAvailability(ctx)
	a.checkUpdatable(ctx)
	a.CheckMinSdkVersion(ctx)
	a.checkStaticLinkingToStubLibraries(ctx)
	a.checkStaticExecutables(ctx)
	if len(a.properties.Tests) > 0 && !a.testApex {
		ctx.PropertyErrorf("tests", "property allowed only in apex_test module type")
		return false
	}
	return true
}

type visitorContext struct {
	// all the files that will be included in this APEX
	filesInfo []apexFile

	// native lib dependencies
	provideNativeLibs []string
	requireNativeLibs []string

	handleSpecialLibs bool

	// if true, raise error on duplicate apexFile
	checkDuplicate bool

	// visitor skips these from this list of module names
	unwantedTransitiveDeps []string

	aconfigFiles []android.Path
}

func (vctx *visitorContext) normalizeFileInfo(mctx android.ModuleContext) {
	encountered := make(map[string]apexFile)
	for _, f := range vctx.filesInfo {
		// Skips unwanted transitive deps. This happens, for example, with Rust binaries with prefer_rlib:true.
		// TODO(b/295593640)
		// Needs additional verification for the resulting APEX to ensure that skipped artifacts don't make problems.
		// For example, DT_NEEDED modules should be found within the APEX unless they are marked in `requiredNativeLibs`.
		if f.transitiveDep && f.module != nil && android.InList(mctx.OtherModuleName(f.module), vctx.unwantedTransitiveDeps) {
			continue
		}
		dest := filepath.Join(f.installDir, f.builtFile.Base())
		if e, ok := encountered[dest]; !ok {
			encountered[dest] = f
		} else {
			if vctx.checkDuplicate && f.builtFile.String() != e.builtFile.String() {
				mctx.ModuleErrorf("apex file %v is provided by two different files %v and %v",
					dest, e.builtFile, f.builtFile)
				return
			}
			// If a module is directly included and also transitively depended on
			// consider it as directly included.
			e.transitiveDep = e.transitiveDep && f.transitiveDep
			// If a module is added as both a JNI library and a regular shared library, consider it as a
			// JNI library.
			e.isJniLib = e.isJniLib || f.isJniLib
			encountered[dest] = e
		}
	}
	vctx.filesInfo = vctx.filesInfo[:0]
	for _, v := range encountered {
		vctx.filesInfo = append(vctx.filesInfo, v)
	}
	sort.Slice(vctx.filesInfo, func(i, j int) bool {
		// Sort by destination path so as to ensure consistent ordering even if the source of the files
		// changes.
		return vctx.filesInfo[i].path() < vctx.filesInfo[j].path()
	})
}

func (a *apexBundle) depVisitor(vctx *visitorContext, ctx android.ModuleContext, child, parent blueprint.Module) bool {
	depTag := ctx.OtherModuleDependencyTag(child)
	if _, ok := depTag.(android.ExcludeFromApexContentsTag); ok {
		return false
	}
	if mod, ok := child.(android.Module); ok && !mod.Enabled(ctx) {
		return false
	}
	depName := ctx.OtherModuleName(child)
	if _, isDirectDep := parent.(*apexBundle); isDirectDep {
		switch depTag {
		case sharedLibTag, jniLibTag:
			isJniLib := depTag == jniLibTag
			propertyName := "native_shared_libs"
			if isJniLib {
				propertyName = "jni_libs"
			}
			switch ch := child.(type) {
			case *cc.Module:
				if ch.IsStubs() {
					ctx.PropertyErrorf(propertyName, "%q is a stub. Remove it from the list.", depName)
				}
				fi := apexFileForNativeLibrary(ctx, ch, vctx.handleSpecialLibs)
				fi.isJniLib = isJniLib
				vctx.filesInfo = append(vctx.filesInfo, fi)
				addAconfigFiles(vctx, ctx, child)
				// Collect the list of stub-providing libs except:
				// - VNDK libs are only for vendors
				// - bootstrap bionic libs are treated as provided by system
				if ch.HasStubsVariants() && !a.vndkApex && !cc.InstallToBootstrap(ch.BaseModuleName(), ctx.Config()) {
					vctx.provideNativeLibs = append(vctx.provideNativeLibs, fi.stem())
				}
				return true // track transitive dependencies
			case *rust.Module:
				fi := apexFileForRustLibrary(ctx, ch)
				fi.isJniLib = isJniLib
				vctx.filesInfo = append(vctx.filesInfo, fi)
				addAconfigFiles(vctx, ctx, child)
				return true // track transitive dependencies
			default:
				ctx.PropertyErrorf(propertyName, "%q is not a cc_library or cc_library_shared module", depName)
			}
		case executableTag:
			switch ch := child.(type) {
			case *cc.Module:
				vctx.filesInfo = append(vctx.filesInfo, apexFileForExecutable(ctx, ch))
				addAconfigFiles(vctx, ctx, child)
				return true // track transitive dependencies
			case *rust.Module:
				vctx.filesInfo = append(vctx.filesInfo, apexFileForRustExecutable(ctx, ch))
				addAconfigFiles(vctx, ctx, child)
				return true // track transitive dependencies
			default:
				ctx.PropertyErrorf("binaries",
					"%q is neither cc_binary, rust_binary, (embedded) py_binary, (host) blueprint_go_binary, nor (host) bootstrap_go_binary", depName)
			}
		case shBinaryTag:
			if csh, ok := child.(*sh.ShBinary); ok {
				vctx.filesInfo = append(vctx.filesInfo, apexFileForShBinary(ctx, csh))
			} else {
				ctx.PropertyErrorf("sh_binaries", "%q is not a sh_binary module", depName)
			}
		case bcpfTag:
			_, ok := child.(*java.BootclasspathFragmentModule)
			if !ok {
				ctx.PropertyErrorf("bootclasspath_fragments", "%q is not a bootclasspath_fragment module", depName)
				return false
			}

			vctx.filesInfo = append(vctx.filesInfo, apexBootclasspathFragmentFiles(ctx, child)...)
			return true
		case sscpfTag:
			if _, ok := child.(*java.SystemServerClasspathModule); !ok {
				ctx.PropertyErrorf("systemserverclasspath_fragments",
					"%q is not a systemserverclasspath_fragment module", depName)
				return false
			}
			if af := apexClasspathFragmentProtoFile(ctx, child); af != nil {
				vctx.filesInfo = append(vctx.filesInfo, *af)
			}
			return true
		case javaLibTag:
			switch child.(type) {
			case *java.Library, *java.SdkLibrary, *java.DexImport, *java.SdkLibraryImport, *java.Import:
				af := apexFileForJavaModule(ctx, child.(javaModule))
				if !af.ok() {
					ctx.PropertyErrorf("java_libs", "%q is not configured to be compiled into dex", depName)
					return false
				}
				vctx.filesInfo = append(vctx.filesInfo, af)
				addAconfigFiles(vctx, ctx, child)
				return true // track transitive dependencies
			default:
				ctx.PropertyErrorf("java_libs", "%q of type %q is not supported", depName, ctx.OtherModuleType(child))
			}
		case androidAppTag:
			switch ap := child.(type) {
			case *java.AndroidApp:
				vctx.filesInfo = append(vctx.filesInfo, apexFilesForAndroidApp(ctx, ap)...)
				addAconfigFiles(vctx, ctx, child)
				return true // track transitive dependencies
			case *java.AndroidAppImport:
				vctx.filesInfo = append(vctx.filesInfo, apexFilesForAndroidApp(ctx, ap)...)
				addAconfigFiles(vctx, ctx, child)
			case *java.AndroidTestHelperApp:
				vctx.filesInfo = append(vctx.filesInfo, apexFilesForAndroidApp(ctx, ap)...)
				addAconfigFiles(vctx, ctx, child)
			case *java.AndroidAppSet:
				appDir := "app"
				if ap.Privileged() {
					appDir = "priv-app"
				}
				// TODO(b/224589412, b/226559955): Ensure that the dirname is
				// suffixed so that PackageManager correctly invalidates the
				// existing installed apk in favour of the new APK-in-APEX.
				// See bugs for more information.
				appDirName := filepath.Join(appDir, ap.BaseModuleName()+"@"+sanitizedBuildIdForPath(ctx))
				af := newApexFile(ctx, ap.OutputFile(), ap.BaseModuleName(), appDirName, appSet, ap)
				af.certificate = java.PresignedCertificate
				vctx.filesInfo = append(vctx.filesInfo, af)
				addAconfigFiles(vctx, ctx, child)
			default:
				ctx.PropertyErrorf("apps", "%q is not an android_app module", depName)
			}
		case rroTag:
			if rro, ok := child.(java.RuntimeResourceOverlayModule); ok {
				vctx.filesInfo = append(vctx.filesInfo, apexFileForRuntimeResourceOverlay(ctx, rro))
			} else {
				ctx.PropertyErrorf("rros", "%q is not an runtime_resource_overlay module", depName)
			}
		case bpfTag:
			if bpfProgram, ok := child.(bpf.BpfModule); ok {
				filesToCopy := android.OutputFilesForModule(ctx, bpfProgram, "")
				apex_sub_dir := bpfProgram.SubDir()
				for _, bpfFile := range filesToCopy {
					vctx.filesInfo = append(vctx.filesInfo, apexFileForBpfProgram(ctx, bpfFile, apex_sub_dir, bpfProgram))
				}
			} else {
				ctx.PropertyErrorf("bpfs", "%q is not a bpf module", depName)
			}
		case fsTag:
			if fs, ok := child.(filesystem.Filesystem); ok {
				vctx.filesInfo = append(vctx.filesInfo, apexFileForFilesystem(ctx, fs.OutputPath(), fs))
			} else {
				ctx.PropertyErrorf("filesystems", "%q is not a filesystem module", depName)
			}
		case prebuiltTag:
			if prebuilt, ok := child.(prebuilt_etc.PrebuiltEtcModule); ok {
				filesToCopy := android.OutputFilesForModule(ctx, prebuilt, "")
				for _, etcFile := range filesToCopy {
					vctx.filesInfo = append(vctx.filesInfo, apexFileForPrebuiltEtc(ctx, prebuilt, etcFile))
				}
				addAconfigFiles(vctx, ctx, child)
			} else {
				ctx.PropertyErrorf("prebuilts", "%q is not a prebuilt_etc module", depName)
			}
		case compatConfigTag:
			if compatConfig, ok := child.(java.PlatformCompatConfigIntf); ok {
				vctx.filesInfo = append(vctx.filesInfo, apexFileForCompatConfig(ctx, compatConfig, depName))
			} else {
				ctx.PropertyErrorf("compat_configs", "%q is not a platform_compat_config module", depName)
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
					vctx.filesInfo = append(vctx.filesInfo, af)
					addAconfigFiles(vctx, ctx, child)
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
		case certificateTag:
			if dep, ok := child.(*java.AndroidAppCertificate); ok {
				a.containerCertificateFile = dep.Certificate.Pem
				a.containerPrivateKeyFile = dep.Certificate.Key
			} else {
				ctx.ModuleErrorf("certificate dependency %q must be an android_app_certificate module", depName)
			}
		}
		return false
	}

	if a.vndkApex {
		return false
	}

	// indirect dependencies
	am, ok := child.(android.ApexModule)
	if !ok {
		return false
	}
	// We cannot use a switch statement on `depTag` here as the checked
	// tags used below are private (e.g. `cc.sharedDepTag`).
	if cc.IsSharedDepTag(depTag) || cc.IsRuntimeDepTag(depTag) {
		if ch, ok := child.(*cc.Module); ok {
			af := apexFileForNativeLibrary(ctx, ch, vctx.handleSpecialLibs)
			af.transitiveDep = true

			abInfo, _ := android.ModuleProvider(ctx, android.ApexBundleInfoProvider)
			if !abInfo.Contents.DirectlyInApex(depName) && (ch.IsStubs() || ch.HasStubsVariants()) {
				// If the dependency is a stubs lib, don't include it in this APEX,
				// but make sure that the lib is installed on the device.
				// In case no APEX is having the lib, the lib is installed to the system
				// partition.
				//
				// Always include if we are a host-apex however since those won't have any
				// system libraries.
				//
				// Skip the dependency in unbundled builds where the device image is not
				// being built.
				if ch.IsStubsImplementationRequired() && !am.DirectlyInAnyApex() && !ctx.Config().UnbundledBuild() {
					// we need a module name for Make
					name := ch.ImplementationModuleNameForMake(ctx) + ch.Properties.SubName
					if !android.InList(name, a.makeModulesToInstall) {
						a.makeModulesToInstall = append(a.makeModulesToInstall, name)
					}
				}
				vctx.requireNativeLibs = append(vctx.requireNativeLibs, af.stem())
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
			//
			// TODO(jiyong): explain why the same module is passed in twice.
			// Switching the first am to parent breaks lots of tests.
			if !android.IsDepInSameApex(ctx, am, am) {
				return false
			}

			vctx.filesInfo = append(vctx.filesInfo, af)
			addAconfigFiles(vctx, ctx, child)
			return true // track transitive dependencies
		} else if rm, ok := child.(*rust.Module); ok {
			af := apexFileForRustLibrary(ctx, rm)
			af.transitiveDep = true
			vctx.filesInfo = append(vctx.filesInfo, af)
			addAconfigFiles(vctx, ctx, child)
			return true // track transitive dependencies
		}
	} else if cc.IsTestPerSrcDepTag(depTag) {
		if ch, ok := child.(*cc.Module); ok {
			af := apexFileForExecutable(ctx, ch)
			// Handle modules created as `test_per_src` variations of a single test module:
			// use the name of the generated test binary (`fileToCopy`) instead of the name
			// of the original test module (`depName`, shared by all `test_per_src`
			// variations of that module).
			af.androidMkModuleName = filepath.Base(af.builtFile.String())
			// these are not considered transitive dep
			af.transitiveDep = false
			vctx.filesInfo = append(vctx.filesInfo, af)
			return true // track transitive dependencies
		}
	} else if cc.IsHeaderDepTag(depTag) {
		// nothing
	} else if java.IsJniDepTag(depTag) {
		// Because APK-in-APEX embeds jni_libs transitively, we don't need to track transitive deps
	} else if java.IsXmlPermissionsFileDepTag(depTag) {
		if prebuilt, ok := child.(prebuilt_etc.PrebuiltEtcModule); ok {
			filesToCopy := android.OutputFilesForModule(ctx, prebuilt, "")
			for _, etcFile := range filesToCopy {
				vctx.filesInfo = append(vctx.filesInfo, apexFileForPrebuiltEtc(ctx, prebuilt, etcFile))
			}
		}
	} else if rust.IsDylibDepTag(depTag) {
		if rustm, ok := child.(*rust.Module); ok && rustm.IsInstallableToApex() {
			af := apexFileForRustLibrary(ctx, rustm)
			af.transitiveDep = true
			vctx.filesInfo = append(vctx.filesInfo, af)
			addAconfigFiles(vctx, ctx, child)
			return true // track transitive dependencies
		}
	} else if rust.IsRlibDepTag(depTag) {
		// Rlib is statically linked, but it might have shared lib
		// dependencies. Track them.
		return true
	} else if java.IsBootclasspathFragmentContentDepTag(depTag) {
		// Add the contents of the bootclasspath fragment to the apex.
		switch child.(type) {
		case *java.Library, *java.SdkLibrary:
			javaModule := child.(javaModule)
			af := apexFileForBootclasspathFragmentContentModule(ctx, parent, javaModule)
			if !af.ok() {
				ctx.PropertyErrorf("bootclasspath_fragments",
					"bootclasspath_fragment content %q is not configured to be compiled into dex", depName)
				return false
			}
			vctx.filesInfo = append(vctx.filesInfo, af)
			addAconfigFiles(vctx, ctx, child)
			return true // track transitive dependencies
		default:
			ctx.PropertyErrorf("bootclasspath_fragments",
				"bootclasspath_fragment content %q of type %q is not supported", depName, ctx.OtherModuleType(child))
		}
	} else if java.IsSystemServerClasspathFragmentContentDepTag(depTag) {
		// Add the contents of the systemserverclasspath fragment to the apex.
		switch child.(type) {
		case *java.Library, *java.SdkLibrary:
			af := apexFileForJavaModule(ctx, child.(javaModule))
			vctx.filesInfo = append(vctx.filesInfo, af)
			if profileAf := apexFileForJavaModuleProfile(ctx, child.(javaModule)); profileAf != nil {
				vctx.filesInfo = append(vctx.filesInfo, *profileAf)
			}
			addAconfigFiles(vctx, ctx, child)
			return true // track transitive dependencies
		default:
			ctx.PropertyErrorf("systemserverclasspath_fragments",
				"systemserverclasspath_fragment content %q of type %q is not supported", depName, ctx.OtherModuleType(child))
		}
	} else if _, ok := depTag.(android.CopyDirectlyInAnyApexTag); ok {
		// nothing
	} else if depTag == android.DarwinUniversalVariantTag {
		// nothing
	} else if depTag == android.RequiredDepTag {
		// nothing
	} else if am.CanHaveApexVariants() && am.IsInstallableToApex() {
		ctx.ModuleErrorf("unexpected tag %s for indirect dependency %q", android.PrettyPrintTag(depTag), depName)
	}
	return false
}

func addAconfigFiles(vctx *visitorContext, ctx android.ModuleContext, module blueprint.Module) {
	if dep, ok := android.OtherModuleProvider(ctx, module, android.AconfigPropagatingProviderKey); ok {
		if len(dep.AconfigFiles) > 0 && dep.AconfigFiles[ctx.ModuleName()] != nil {
			vctx.aconfigFiles = append(vctx.aconfigFiles, dep.AconfigFiles[ctx.ModuleName()]...)
		}
	}

	validationFlag := ctx.DeviceConfig().AconfigContainerValidation()
	if validationFlag == "error" || validationFlag == "warning" {
		android.VerifyAconfigBuildMode(ctx, ctx.ModuleName(), module, validationFlag == "error")
	}
}

func (a *apexBundle) shouldCheckDuplicate(ctx android.ModuleContext) bool {
	// TODO(b/263308293) remove this
	if a.properties.IsCoverageVariant {
		return false
	}
	if ctx.DeviceConfig().DeviceArch() == "" {
		return false
	}
	return true
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
	if !a.commonBuildActions(ctx) {
		return
	}
	////////////////////////////////////////////////////////////////////////////////////////////
	// 2) traverse the dependency tree to collect apexFile structs from them.

	// TODO(jiyong): do this using WalkPayloadDeps
	// TODO(jiyong): make this clean!!!
	vctx := visitorContext{
		handleSpecialLibs:      !android.Bool(a.properties.Ignore_system_library_special_case),
		checkDuplicate:         a.shouldCheckDuplicate(ctx),
		unwantedTransitiveDeps: a.properties.Unwanted_transitive_deps,
	}
	ctx.WalkDepsBlueprint(func(child, parent blueprint.Module) bool { return a.depVisitor(&vctx, ctx, child, parent) })
	vctx.normalizeFileInfo(ctx)
	if a.privateKeyFile == nil {
		if ctx.Config().AllowMissingDependencies() {
			// TODO(b/266099037): a better approach for slim manifests.
			ctx.AddMissingDependencies([]string{String(a.overridableProperties.Key)})
			// Create placeholder paths for later stages that expect to see those paths,
			// though they won't be used.
			var unusedPath = android.PathForModuleOut(ctx, "nonexistentprivatekey")
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.ErrorRule,
				Output: unusedPath,
				Args: map[string]string{
					"error": "Private key not available",
				},
			})
			a.privateKeyFile = unusedPath
		} else {
			ctx.PropertyErrorf("key", "private_key for %q could not be found", String(a.overridableProperties.Key))
			return
		}
	}

	if a.publicKeyFile == nil {
		if ctx.Config().AllowMissingDependencies() {
			// TODO(b/266099037): a better approach for slim manifests.
			ctx.AddMissingDependencies([]string{String(a.overridableProperties.Key)})
			// Create placeholder paths for later stages that expect to see those paths,
			// though they won't be used.
			var unusedPath = android.PathForModuleOut(ctx, "nonexistentpublickey")
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.ErrorRule,
				Output: unusedPath,
				Args: map[string]string{
					"error": "Public key not available",
				},
			})
			a.publicKeyFile = unusedPath
		} else {
			ctx.PropertyErrorf("key", "public_key for %q could not be found", String(a.overridableProperties.Key))
			return
		}
	}

	////////////////////////////////////////////////////////////////////////////////////////////
	// 3) some fields in apexBundle struct are configured
	a.installDir = android.PathForModuleInstall(ctx, "apex")
	a.filesInfo = vctx.filesInfo
	a.aconfigFiles = android.FirstUniquePaths(vctx.aconfigFiles)

	a.setPayloadFsType(ctx)
	a.setSystemLibLink(ctx)
	a.compatSymlinks = makeCompatSymlinks(a.BaseModuleName(), ctx)

	////////////////////////////////////////////////////////////////////////////////////////////
	// 4) generate the build rules to create the APEX. This is done in builder.go.
	a.buildManifest(ctx, vctx.provideNativeLibs, vctx.requireNativeLibs)
	a.buildApex(ctx)
	a.buildApexDependencyInfo(ctx)
	a.buildLintReports(ctx)

	// Set a provider for dexpreopt of bootjars
	a.provideApexExportsInfo(ctx)

	a.providePrebuiltInfo(ctx)

	a.required = a.RequiredModuleNames(ctx)
}

// Set prebuiltInfoProvider. This will be used by `apex_prebuiltinfo_singleton` to print out a metadata file
// with information about whether source or prebuilt of an apex was used during the build.
func (a *apexBundle) providePrebuiltInfo(ctx android.ModuleContext) {
	info := android.PrebuiltInfo{
		Name:        a.Name(),
		Is_prebuilt: false,
	}
	android.SetProvider(ctx, android.PrebuiltInfoProvider, info)
}

// Set a provider containing information about the jars and .prof provided by the apex
// Apexes built from source retrieve this information by visiting `bootclasspath_fragments`
// Used by dex_bootjars to generate the boot image
func (a *apexBundle) provideApexExportsInfo(ctx android.ModuleContext) {
	ctx.VisitDirectDepsWithTag(bcpfTag, func(child android.Module) {
		if info, ok := android.OtherModuleProvider(ctx, child, java.BootclasspathFragmentApexContentInfoProvider); ok {
			exports := android.ApexExportsInfo{
				ApexName:                      a.ApexVariationName(),
				ProfilePathOnHost:             info.ProfilePathOnHost(),
				LibraryNameToDexJarPathOnHost: info.DexBootJarPathMap(),
			}
			android.SetProvider(ctx, android.ApexExportsInfoProvider, exports)
		}
	})
}

// apexBootclasspathFragmentFiles returns the list of apexFile structures defining the files that
// the bootclasspath_fragment contributes to the apex.
func apexBootclasspathFragmentFiles(ctx android.ModuleContext, module blueprint.Module) []apexFile {
	bootclasspathFragmentInfo, _ := android.OtherModuleProvider(ctx, module, java.BootclasspathFragmentApexContentInfoProvider)
	var filesToAdd []apexFile

	// Add classpaths.proto config.
	if af := apexClasspathFragmentProtoFile(ctx, module); af != nil {
		filesToAdd = append(filesToAdd, *af)
	}

	pathInApex := bootclasspathFragmentInfo.ProfileInstallPathInApex()
	if pathInApex != "" {
		pathOnHost := bootclasspathFragmentInfo.ProfilePathOnHost()
		tempPath := android.PathForModuleOut(ctx, "boot_image_profile", pathInApex)

		if pathOnHost != nil {
			// We need to copy the profile to a temporary path with the right filename because the apexer
			// will take the filename as is.
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.Cp,
				Input:  pathOnHost,
				Output: tempPath,
			})
		} else {
			// At this point, the boot image profile cannot be generated. It is probably because the boot
			// image profile source file does not exist on the branch, or it is not available for the
			// current build target.
			// However, we cannot enforce the boot image profile to be generated because some build
			// targets (such as module SDK) do not need it. It is only needed when the APEX is being
			// built. Therefore, we create an error rule so that an error will occur at the ninja phase
			// only if the APEX is being built.
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.ErrorRule,
				Output: tempPath,
				Args: map[string]string{
					"error": "Boot image profile cannot be generated",
				},
			})
		}

		androidMkModuleName := filepath.Base(pathInApex)
		af := newApexFile(ctx, tempPath, androidMkModuleName, filepath.Dir(pathInApex), etc, nil)
		filesToAdd = append(filesToAdd, af)
	}

	return filesToAdd
}

// apexClasspathFragmentProtoFile returns *apexFile structure defining the classpath.proto config that
// the module contributes to the apex; or nil if the proto config was not generated.
func apexClasspathFragmentProtoFile(ctx android.ModuleContext, module blueprint.Module) *apexFile {
	info, _ := android.OtherModuleProvider(ctx, module, java.ClasspathFragmentProtoContentInfoProvider)
	if !info.ClasspathFragmentProtoGenerated {
		return nil
	}
	classpathProtoOutput := info.ClasspathFragmentProtoOutput
	af := newApexFile(ctx, classpathProtoOutput, classpathProtoOutput.Base(), info.ClasspathFragmentProtoInstallDir.Rel(), etc, nil)
	return &af
}

// apexFileForBootclasspathFragmentContentModule creates an apexFile for a bootclasspath_fragment
// content module, i.e. a library that is part of the bootclasspath.
func apexFileForBootclasspathFragmentContentModule(ctx android.ModuleContext, fragmentModule blueprint.Module, javaModule javaModule) apexFile {
	bootclasspathFragmentInfo, _ := android.OtherModuleProvider(ctx, fragmentModule, java.BootclasspathFragmentApexContentInfoProvider)

	// Get the dexBootJar from the bootclasspath_fragment as that is responsible for performing the
	// hidden API encpding.
	dexBootJar, err := bootclasspathFragmentInfo.DexBootJarPathForContentModule(javaModule)
	if err != nil {
		ctx.ModuleErrorf("%s", err)
	}

	// Create an apexFile as for a normal java module but with the dex boot jar provided by the
	// bootclasspath_fragment.
	af := apexFileForJavaModuleWithFile(ctx, javaModule, dexBootJar)
	return af
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

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.overridableProperties.Overrides)
	multitree.InitExportableModule(module)
	return module
}

func ApexBundleFactory(testApex bool) android.Module {
	bundle := newApexBundle()
	bundle.testApex = testApex
	return bundle
}

// apex_test is an APEX for testing. The difference from the ordinary apex module type is that
// certain compatibility checks such as apex_available are not done for apex_test.
func TestApexBundleFactory() android.Module {
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
func DefaultsFactory() android.Module {
	module := &Defaults{}

	module.AddProperties(
		&apexBundleProperties{},
		&apexTargetBundleProperties{},
		&apexArchBundleProperties{},
		&overridableProperties{},
	)

	android.InitDefaultsModule(module)
	return module
}

type OverrideApex struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (o *OverrideApex) GenerateAndroidBuildActions(_ android.ModuleContext) {
	// All the overrides happen in the base module.
}

// override_apex is used to create an apex module based on another apex module by overriding some of
// its properties.
func OverrideApexFactory() android.Module {
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

var _ android.ModuleWithMinSdkVersionCheck = (*apexBundle)(nil)

// Ensures that min_sdk_version of the included modules are equal or less than the min_sdk_version
// of this apexBundle.
func (a *apexBundle) CheckMinSdkVersion(ctx android.ModuleContext) {
	if a.testApex || a.vndkApex {
		return
	}
	// apexBundle::minSdkVersion reports its own errors.
	minSdkVersion := a.minSdkVersion(ctx)
	android.CheckMinSdkVersion(ctx, minSdkVersion, a.WalkPayloadDeps)
}

// Returns apex's min_sdk_version string value, honoring overrides
func (a *apexBundle) minSdkVersionValue(ctx android.EarlyModuleContext) string {
	// Only override the minSdkVersion value on Apexes which already specify
	// a min_sdk_version (it's optional for non-updatable apexes), and that its
	// min_sdk_version value is lower than the one to override with.
	minApiLevel := android.MinSdkVersionFromValue(ctx, proptools.String(a.overridableProperties.Min_sdk_version))
	if minApiLevel.IsNone() {
		return ""
	}

	overrideMinSdkValue := ctx.DeviceConfig().ApexGlobalMinSdkVersionOverride()
	overrideApiLevel := android.MinSdkVersionFromValue(ctx, overrideMinSdkValue)
	if !overrideApiLevel.IsNone() && overrideApiLevel.CompareTo(minApiLevel) > 0 {
		minApiLevel = overrideApiLevel
	}

	return minApiLevel.String()
}

// Returns apex's min_sdk_version SdkSpec, honoring overrides
func (a *apexBundle) MinSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel {
	return a.minSdkVersion(ctx)
}

// Returns apex's min_sdk_version ApiLevel, honoring overrides
func (a *apexBundle) minSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel {
	return android.MinSdkVersionFromValue(ctx, a.minSdkVersionValue(ctx))
}

// Ensures that a lib providing stub isn't statically linked
func (a *apexBundle) checkStaticLinkingToStubLibraries(ctx android.ModuleContext) {
	// Practically, we only care about regular APEXes on the device.
	if a.testApex || a.vndkApex {
		return
	}

	abInfo, _ := android.ModuleProvider(ctx, android.ApexBundleInfoProvider)

	a.WalkPayloadDeps(ctx, func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool) bool {
		if ccm, ok := to.(*cc.Module); ok {
			apexName := ctx.ModuleName()
			fromName := ctx.OtherModuleName(from)
			toName := ctx.OtherModuleName(to)

			// If `to` is not actually in the same APEX as `from` then it does not need
			// apex_available and neither do any of its dependencies.
			//
			// It is ok to call DepIsInSameApex() directly from within WalkPayloadDeps().
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

// checkUpdatable enforces APEX and its transitive dep properties to have desired values for updatable APEXes.
func (a *apexBundle) checkUpdatable(ctx android.ModuleContext) {
	if a.Updatable() {
		if a.minSdkVersionValue(ctx) == "" {
			ctx.PropertyErrorf("updatable", "updatable APEXes should set min_sdk_version as well")
		}
		if a.UsePlatformApis() {
			ctx.PropertyErrorf("updatable", "updatable APEXes can't use platform APIs")
		}
		if proptools.Bool(a.properties.Use_vndk_as_stable) {
			ctx.PropertyErrorf("use_vndk_as_stable", "updatable APEXes can't use external VNDK libs")
		}
		if a.FutureUpdatable() {
			ctx.PropertyErrorf("future_updatable", "Already updatable. Remove `future_updatable: true:`")
		}
		a.checkJavaStableSdkVersion(ctx)
		a.checkClasspathFragments(ctx)
	}
}

// checkClasspathFragments enforces that all classpath fragments in deps generate classpaths.proto config.
func (a *apexBundle) checkClasspathFragments(ctx android.ModuleContext) {
	ctx.VisitDirectDeps(func(module android.Module) {
		if tag := ctx.OtherModuleDependencyTag(module); tag == bcpfTag || tag == sscpfTag {
			info, _ := android.OtherModuleProvider(ctx, module, java.ClasspathFragmentProtoContentInfoProvider)
			if !info.ClasspathFragmentProtoGenerated {
				ctx.OtherModuleErrorf(module, "is included in updatable apex %v, it must not set generate_classpaths_proto to false", ctx.ModuleName())
			}
		}
	})
}

// checkJavaStableSdkVersion enforces that all Java deps are using stable SDKs to compile.
func (a *apexBundle) checkJavaStableSdkVersion(ctx android.ModuleContext) {
	// Visit direct deps only. As long as we guarantee top-level deps are using stable SDKs,
	// java's checkLinkType guarantees correct usage for transitive deps
	ctx.VisitDirectDepsBlueprint(func(module blueprint.Module) {
		tag := ctx.OtherModuleDependencyTag(module)
		switch tag {
		case javaLibTag, androidAppTag:
			if m, ok := module.(interface {
				CheckStableSdkVersion(ctx android.BaseModuleContext) error
			}); ok {
				if err := m.CheckStableSdkVersion(ctx); err != nil {
					ctx.ModuleErrorf("cannot depend on \"%v\": %v", ctx.OtherModuleName(module), err)
				}
			}
		}
	})
}

// checkApexAvailability ensures that the all the dependencies are marked as available for this APEX.
func (a *apexBundle) checkApexAvailability(ctx android.ModuleContext) {
	// Let's be practical. Availability for test, host, and the VNDK apex isn't important
	if a.testApex || a.vndkApex {
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
		for _, props := range ctx.Module().GetProperties() {
			if apexProps, ok := props.(*apexBundleProperties); ok {
				if apexProps.Apex_available_name != nil {
					apexName = *apexProps.Apex_available_name
				}
			}
		}
		fromName := ctx.OtherModuleName(from)
		toName := ctx.OtherModuleName(to)

		// If `to` is not actually in the same APEX as `from` then it does not need
		// apex_available and neither do any of its dependencies.
		//
		// It is ok to call DepIsInSameApex() directly from within WalkPayloadDeps().
		if am, ok := from.(android.DepIsInSameApex); ok && !am.DepIsInSameApex(ctx, to) {
			// As soon as the dependency graph crosses the APEX boundary, don't go
			// further.
			return false
		}

		if to.AvailableFor(apexName) || baselineApexAvailable(apexName, toName) {
			return true
		}
		ctx.ModuleErrorf("%q requires %q that doesn't list the APEX under 'apex_available'."+
			"\n\nDependency path:%s\n\n"+
			"Consider adding %q to 'apex_available' property of %q",
			fromName, toName, ctx.GetPathString(true), apexName, toName)
		// Visit this module's dependencies to check and report any issues with their availability.
		return true
	})
}

// checkStaticExecutable ensures that executables in an APEX are not static.
func (a *apexBundle) checkStaticExecutables(ctx android.ModuleContext) {
	ctx.VisitDirectDepsBlueprint(func(module blueprint.Module) {
		if ctx.OtherModuleDependencyTag(module) != executableTag {
			return
		}

		if l, ok := module.(cc.LinkableInterface); ok && l.StaticExecutable() {
			apex := a.ApexVariationName()
			exec := ctx.OtherModuleName(module)
			if isStaticExecutableAllowed(apex, exec) {
				return
			}
			ctx.ModuleErrorf("executable %s is static", ctx.OtherModuleName(module))
		}
	})
}

// A small list of exceptions where static executables are allowed in APEXes.
func isStaticExecutableAllowed(apex string, exec string) bool {
	m := map[string][]string{
		"com.android.runtime": {
			"linker",
			"linkerconfig",
		},
	}
	execNames, ok := m[apex]
	return ok && android.InList(exec, execNames)
}

// Collect information for opening IDE project files in java/jdeps.go.
func (a *apexBundle) IDEInfo(dpInfo *android.IdeInfo) {
	dpInfo.Deps = append(dpInfo.Deps, a.properties.Java_libs...)
	dpInfo.Deps = append(dpInfo.Deps, a.properties.Bootclasspath_fragments...)
	dpInfo.Deps = append(dpInfo.Deps, a.properties.Systemserverclasspath_fragments...)
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
	m["com.android.runtime"] = []string{
		"libz",
	}
	return m
}

func init() {
	android.AddNeverAllowRules(createBcpPermittedPackagesRules(qBcpPackages())...)
	android.AddNeverAllowRules(createBcpPermittedPackagesRules(rBcpPackages())...)
}

func createBcpPermittedPackagesRules(bcpPermittedPackages map[string][]string) []android.Rule {
	rules := make([]android.Rule, 0, len(bcpPermittedPackages))
	for jar, permittedPackages := range bcpPermittedPackages {
		permittedPackagesRule := android.NeverAllow().
			With("name", jar).
			WithMatcher("permitted_packages", android.NotInList(permittedPackages)).
			Because(jar +
				" bootjar may only use these package prefixes: " + strings.Join(permittedPackages, ",") +
				". Please consider the following alternatives:\n" +
				"    1. If the offending code is from a statically linked library, consider " +
				"removing that dependency and using an alternative already in the " +
				"bootclasspath, or perhaps a shared library." +
				"    2. Move the offending code into an allowed package.\n" +
				"    3. Jarjar the offending code. Please be mindful of the potential system " +
				"health implications of bundling that code, particularly if the offending jar " +
				"is part of the bootclasspath.")

		rules = append(rules, permittedPackagesRule)
	}
	return rules
}

// DO NOT EDIT! These are the package prefixes that are exempted from being AOT'ed by ART.
// Adding code to the bootclasspath in new packages will cause issues on module update.
func qBcpPackages() map[string][]string {
	return map[string][]string{
		"conscrypt": {
			"android.net.ssl",
			"com.android.org.conscrypt",
		},
		"updatable-media": {
			"android.media",
		},
	}
}

// DO NOT EDIT! These are the package prefixes that are exempted from being AOT'ed by ART.
// Adding code to the bootclasspath in new packages will cause issues on module update.
func rBcpPackages() map[string][]string {
	return map[string][]string{
		"framework-mediaprovider": {
			"android.provider",
		},
		"framework-permission": {
			"android.permission",
			"android.app.role",
			"com.android.permission",
			"com.android.role",
		},
		"framework-sdkextensions": {
			"android.os.ext",
		},
		"framework-statsd": {
			"android.app",
			"android.os",
			"android.util",
			"com.android.internal.statsd",
			"com.android.server.stats",
		},
		"framework-wifi": {
			"com.android.server.wifi",
			"com.android.wifi.x",
			"android.hardware.wifi",
			"android.net.wifi",
		},
		"framework-tethering": {
			"android.net",
		},
	}
}

func (a *apexBundle) IsTestApex() bool {
	return a.testApex
}
