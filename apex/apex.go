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

package apex

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
	"android/soong/python"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/proptools"
)

const (
	imageApexSuffix = ".apex"
	zipApexSuffix   = ".zipapex"
	flattenedSuffix = ".flattened"

	imageApexType     = "image"
	zipApexType       = "zip"
	flattenedApexType = "flattened"
)

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var (
	sharedLibTag   = dependencyTag{name: "sharedLib"}
	executableTag  = dependencyTag{name: "executable"}
	javaLibTag     = dependencyTag{name: "javaLib"}
	prebuiltTag    = dependencyTag{name: "prebuilt"}
	testTag        = dependencyTag{name: "test"}
	keyTag         = dependencyTag{name: "key"}
	certificateTag = dependencyTag{name: "certificate"}
	usesTag        = dependencyTag{name: "uses"}
	androidAppTag  = dependencyTag{name: "androidApp"}
	apexAvailWl    = makeApexAvailableWhitelist()
)

// This is a map from apex to modules, which overrides the
// apex_available setting for that particular module to make
// it available for the apex regardless of its setting.
// TODO(b/147364041): remove this
func makeApexAvailableWhitelist() map[string][]string {
	// The "Module separator"s below are employed to minimize merge conflicts.
	m := make(map[string][]string)
	//
	// Module separator
	//
	m["com.android.adbd"] = []string{"adbd", "libcrypto"}
	//
	// Module separator
	//
	m["com.android.art"] = []string{
		"jacocoagent",
		"libadbconnection_server",
		"libartd-disassembler",
		"libbacktrace",
		"libbase",
		"libc++",
		"libcrypto",
		"libdexfile_support",
		"libexpat",
		"libicuuc",
		"liblzma",
		"libmeminfo",
		"libprocinfo",
		"libunwindstack",
		"libvixl",
		"libvixld",
		"libz",
		"libziparchive",
		"prebuilt_libclang_rt",
	}
	//
	// Module separator
	//
	m["com.android.bluetooth.updatable"] = []string{
		"android.hardware.audio.common@5.0",
		"android.hardware.bluetooth@1.0",
		"android.hardware.bluetooth@1.1",
		"android.hardware.bluetooth.a2dp@1.0",
		"android.hardware.bluetooth.audio@2.0",
		"android.hidl.safe_union@1.0",
		"libbase",
		"libbinderthreadstate",
		"libbluetooth",
		"libbluetooth_jni",
		"libc++",
		"libchrome",
		"libcrypto",
		"libcutils",
		"libevent",
		"libfmq",
		"libhidlbase",
		"libprocessgroup",
		"libprotobuf-cpp-lite",
		"libstatslog",
		"libtinyxml2",
		"libutils",
		"libz",
	}
	//
	// Module separator
	//
	m["com.android.cellbroadcast"] = []string{"CellBroadcastApp", "CellBroadcastServiceModule"}
	//
	// Module separator
	//
	m["com.android.conscrypt"] = []string{"boringssl_self_test", "libc++", "libcrypto", "libssl"}
	//
	// Module separator
	//
	m["com.android.cronet"] = []string{"org.chromium.net.cronet", "prebuilt_libcronet.80.0.3986.0"}
	//
	// Module separator
	//
	m["com.android.media"] = []string{
		"android.hardware.cas@1.0",
		"android.hardware.cas.native@1.0",
		"android.hidl.allocator@1.0",
		"android.hidl.memory@1.0",
		"android.hidl.memory.token@1.0",
		"android.hidl.token@1.0",
		"android.hidl.token@1.0-utils",
		"libaacextractor",
		"libamrextractor",
		"libaudioutils",
		"libbase",
		"libbinderthreadstate",
		"libc++",
		"libcrypto",
		"libcutils",
		"libflacextractor",
		"libhidlbase",
		"libhidlmemory",
		"libmidiextractor",
		"libmkvextractor",
		"libmp3extractor",
		"libmp4extractor",
		"libmpeg2extractor",
		"liboggextractor",
		"libprocessgroup",
		"libspeexresampler",
		"libstagefright_flacdec",
		"libutils",
		"libwavextractor",
		"updatable-media",
	}
	//
	// Module separator
	//
	m["com.android.media.swcodec"] = []string{
		"android.frameworks.bufferhub@1.0",
		"android.hardware.common-ndk_platform",
		"android.hardware.graphics.allocator@2.0",
		"android.hardware.graphics.allocator@3.0",
		"android.hardware.graphics.allocator@4.0",
		"android.hardware.graphics.bufferqueue@1.0",
		"android.hardware.graphics.bufferqueue@2.0",
		"android.hardware.graphics.common@1.0",
		"android.hardware.graphics.common@1.1",
		"android.hardware.graphics.common@1.2",
		"android.hardware.graphics.common-ndk_platform",
		"android.hardware.graphics.mapper@2.0",
		"android.hardware.graphics.mapper@2.1",
		"android.hardware.graphics.mapper@3.0",
		"android.hardware.graphics.mapper@4.0",
		"android.hardware.media@1.0",
		"android.hardware.media.bufferpool@2.0",
		"android.hardware.media.c2@1.0",
		"android.hardware.media.c2@1.1",
		"android.hardware.media.omx@1.0",
		"android.hidl.memory@1.0",
		"android.hidl.memory.token@1.0",
		"android.hidl.safe_union@1.0",
		"android.hidl.token@1.0",
		"android.hidl.token@1.0-utils",
		"libaudioutils",
		"libavservices_minijail",
		"libbase",
		"libbinderthreadstate",
		"libc++",
		"libcap",
		"libcodec2",
		"libcodec2_hidl@1.0",
		"libcodec2_hidl@1.1",
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
		"libc_scudo",
		"libcutils",
		"libfmq",
		"libgralloctypes",
		"libhardware",
		"libhidlbase",
		"libhidlmemory",
		"libion",
		"libmedia_codecserviceregistrant",
		"libminijail",
		"libopus",
		"libprocessgroup",
		"libscudo_wrapper",
		"libsfplugin_ccodec_utils",
		"libspeexresampler",
		"libstagefright_amrnb_common",
		"libstagefright_bufferpool@2.0.1",
		"libstagefright_bufferqueue_helper",
		"libstagefright_enc_common",
		"libstagefright_flacdec",
		"libstagefright_foundation",
		"libsync",
		"libui",
		"libutils",
		"libvorbisidec",
		"libvpx",
		"mediaswcodec",
		"prebuilt_libclang_rt",
	}
	//
	// Module separator
	//
	m["com.android.mediaprovider"] = []string{"libfuse", "MediaProvider", "MediaProviderGoogle"}
	//
	// Module separator
	//
	m["com.android.permission"] = []string{"GooglePermissionController", "PermissionController"}
	//
	// Module separator
	//
	m["com.android.runtime"] = []string{
		"libbase",
		"libc++",
		"libdexfile_support",
		"liblzma",
		"libunwindstack",
		"prebuilt_libclang_rt",
	}
	//
	// Module separator
	//
	m["com.android.resolv"] = []string{"libcrypto", "libnetd_resolv", "libssl"}
	//
	// Module separator
	//
	m["com.android.tethering"] = []string{"libbase", "libc++", "libnativehelper_compat_libc++"}
	//
	// Module separator
	//
	m["com.android.wifi"] = []string{
		"libbase",
		"libc++",
		"libcutils",
		"libprocessgroup",
		"libutils",
		"libwifi-jni",
		"wifi-service-resources",
	}
	//
	// Module separator
	//
	return m
}

func init() {
	android.RegisterModuleType("apex", BundleFactory)
	android.RegisterModuleType("apex_test", testApexBundleFactory)
	android.RegisterModuleType("apex_vndk", vndkApexBundleFactory)
	android.RegisterModuleType("apex_defaults", defaultsFactory)
	android.RegisterModuleType("prebuilt_apex", PrebuiltFactory)
	android.RegisterModuleType("override_apex", overrideApexFactory)

	android.PreDepsMutators(RegisterPreDepsMutators)
	android.PostDepsMutators(RegisterPostDepsMutators)

	android.RegisterMakeVarsProvider(pctx, func(ctx android.MakeVarsContext) {
		apexFileContextsInfos := apexFileContextsInfos(ctx.Config())
		sort.Strings(*apexFileContextsInfos)
		ctx.Strict("APEX_FILE_CONTEXTS_INFOS", strings.Join(*apexFileContextsInfos, " "))
	})
}

func RegisterPreDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.TopDown("apex_vndk", apexVndkMutator).Parallel()
	ctx.BottomUp("apex_vndk_deps", apexVndkDepsMutator).Parallel()
}

func RegisterPostDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.BottomUp("apex_deps", apexDepsMutator)
	ctx.BottomUp("apex", apexMutator).Parallel()
	ctx.BottomUp("apex_flattened", apexFlattenedMutator).Parallel()
	ctx.BottomUp("apex_uses", apexUsesMutator).Parallel()
}

// Mark the direct and transitive dependencies of apex bundles so that they
// can be built for the apex bundles.
func apexDepsMutator(mctx android.BottomUpMutatorContext) {
	if a, ok := mctx.Module().(*apexBundle); ok {
		apexBundleName := mctx.ModuleName()
		mctx.WalkDeps(func(child, parent android.Module) bool {
			depName := mctx.OtherModuleName(child)
			// If the parent is apexBundle, this child is directly depended.
			_, directDep := parent.(*apexBundle)
			if a.installable() && !a.testApex {
				// TODO(b/123892969): Workaround for not having any way to annotate test-apexs
				// non-installable apex's cannot be installed and so should not prevent libraries from being
				// installed to the system.
				android.UpdateApexDependency(apexBundleName, depName, directDep)
			}

			if am, ok := child.(android.ApexModule); ok && am.CanHaveApexVariants() &&
				(directDep || am.DepIsInSameApex(mctx, child)) {
				am.BuildForApex(apexBundleName)
				return true
			} else {
				return false
			}
		})
	}
}

// Create apex variations if a module is included in APEX(s).
func apexMutator(mctx android.BottomUpMutatorContext) {
	if am, ok := mctx.Module().(android.ApexModule); ok && am.CanHaveApexVariants() {
		am.CreateApexVariations(mctx)
	} else if _, ok := mctx.Module().(*apexBundle); ok {
		// apex bundle itself is mutated so that it and its modules have same
		// apex variant.
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

var (
	apexFileContextsInfosKey   = android.NewOnceKey("apexFileContextsInfosKey")
	apexFileContextsInfosMutex sync.Mutex
)

func apexFileContextsInfos(config android.Config) *[]string {
	return config.Once(apexFileContextsInfosKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func addFlattenedFileContextsInfos(ctx android.BaseModuleContext, fileContextsInfo string) {
	apexFileContextsInfosMutex.Lock()
	defer apexFileContextsInfosMutex.Unlock()
	apexFileContextsInfos := apexFileContextsInfos(ctx.Config())
	*apexFileContextsInfos = append(*apexFileContextsInfos, fileContextsInfo)
}

func apexFlattenedMutator(mctx android.BottomUpMutatorContext) {
	if ab, ok := mctx.Module().(*apexBundle); ok {
		var variants []string
		switch proptools.StringDefault(ab.properties.Payload_type, "image") {
		case "image":
			variants = append(variants, imageApexType, flattenedApexType)
		case "zip":
			variants = append(variants, zipApexType)
		case "both":
			variants = append(variants, imageApexType, zipApexType, flattenedApexType)
		default:
			mctx.PropertyErrorf("type", "%q is not one of \"image\", \"zip\", or \"both\".", *ab.properties.Payload_type)
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
				if !mctx.Config().FlattenApex() && ab.Platform() {
					modules[i].(*apexBundle).MakeAsSystemExt()
				}
			}
		}
	} else if _, ok := mctx.Module().(*OverrideApex); ok {
		mctx.CreateVariations(imageApexType, flattenedApexType)
	}
}

func apexUsesMutator(mctx android.BottomUpMutatorContext) {
	if ab, ok := mctx.Module().(*apexBundle); ok {
		mctx.AddFarVariationDependencies(nil, usesTag, ab.properties.Uses...)
	}
}

var (
	useVendorWhitelistKey = android.NewOnceKey("useVendorWhitelist")
)

// useVendorWhitelist returns the list of APEXes which are allowed to use_vendor.
// When use_vendor is used, native modules are built with __ANDROID_VNDK__ and __ANDROID_APEX__,
// which may cause compatibility issues. (e.g. libbinder)
// Even though libbinder restricts its availability via 'apex_available' property and relies on
// yet another macro __ANDROID_APEX_<NAME>__, we restrict usage of "use_vendor:" from other APEX modules
// to avoid similar problems.
func useVendorWhitelist(config android.Config) []string {
	return config.Once(useVendorWhitelistKey, func() interface{} {
		return []string{
			// swcodec uses "vendor" variants for smaller size
			"com.android.media.swcodec",
			"test_com.android.media.swcodec",
		}
	}).([]string)
}

// setUseVendorWhitelistForTest overrides useVendorWhitelist and must be
// called before the first call to useVendorWhitelist()
func setUseVendorWhitelistForTest(config android.Config, whitelist []string) {
	config.Once(useVendorWhitelistKey, func() interface{} {
		return whitelist
	})
}

type apexNativeDependencies struct {
	// List of native libraries
	Native_shared_libs []string

	// List of native executables
	Binaries []string

	// List of native tests
	Tests []string
}

type apexMultilibProperties struct {
	// Native dependencies whose compile_multilib is "first"
	First apexNativeDependencies

	// Native dependencies whose compile_multilib is "both"
	Both apexNativeDependencies

	// Native dependencies whose compile_multilib is "prefer32"
	Prefer32 apexNativeDependencies

	// Native dependencies whose compile_multilib is "32"
	Lib32 apexNativeDependencies

	// Native dependencies whose compile_multilib is "64"
	Lib64 apexNativeDependencies
}

type apexBundleProperties struct {
	// Json manifest file describing meta info of this APEX bundle. Default:
	// "apex_manifest.json"
	Manifest *string `android:"path"`

	// AndroidManifest.xml file used for the zip container of this APEX bundle.
	// If unspecified, a default one is automatically generated.
	AndroidManifest *string `android:"path"`

	// Canonical name of the APEX bundle. Used to determine the path to the activated APEX on
	// device (/apex/<apex_name>).
	// If unspecified, defaults to the value of name.
	Apex_name *string

	// Determines the file contexts file for setting security context to each file in this APEX bundle.
	// For platform APEXes, this should points to a file under /system/sepolicy
	// Default: /system/sepolicy/apex/<module_name>_file_contexts.
	File_contexts *string `android:"path"`

	// List of native shared libs that are embedded inside this APEX bundle
	Native_shared_libs []string

	// List of executables that are embedded inside this APEX bundle
	Binaries []string

	// List of java libraries that are embedded inside this APEX bundle
	Java_libs []string

	// List of prebuilt files that are embedded inside this APEX bundle
	Prebuilts []string

	// List of tests that are embedded inside this APEX bundle
	Tests []string

	// Name of the apex_key module that provides the private key to sign APEX
	Key *string

	// The type of APEX to build. Controls what the APEX payload is. Either
	// 'image', 'zip' or 'both'. Default: 'image'.
	Payload_type *string

	// The name of a certificate in the default certificate directory, blank to use the default product certificate,
	// or an android_app_certificate module name in the form ":module".
	Certificate *string

	// Whether this APEX is installable to one of the partitions. Default: true.
	Installable *bool

	// For native libraries and binaries, use the vendor variant instead of the core (platform) variant.
	// Default is false.
	Use_vendor *bool

	// For telling the apex to ignore special handling for system libraries such as bionic. Default is false.
	Ignore_system_library_special_case *bool

	Multilib apexMultilibProperties

	// List of sanitizer names that this APEX is enabled for
	SanitizerNames []string `blueprint:"mutated"`

	PreventInstall bool `blueprint:"mutated"`

	HideFromMake bool `blueprint:"mutated"`

	// Indicates this APEX provides C++ shared libaries to other APEXes. Default: false.
	Provide_cpp_shared_libs *bool

	// List of providing APEXes' names so that this APEX can depend on provided shared libraries.
	Uses []string

	// A txt file containing list of files that are whitelisted to be included in this APEX.
	Whitelisted_files *string

	// package format of this apex variant; could be non-flattened, flattened, or zip.
	// imageApex, zipApex or flattened
	ApexType apexPackaging `blueprint:"mutated"`

	// List of SDKs that are used to build this APEX. A reference to an SDK should be either
	// `name#version` or `name` which is an alias for `name#current`. If left empty, `platform#current`
	// is implied. This value affects all modules included in this APEX. In other words, they are
	// also built with the SDKs specified here.
	Uses_sdks []string

	// Whenever apex_payload.img of the APEX should include dm-verity hashtree.
	// Should be only used in tests#.
	Test_only_no_hashtree *bool

	// Whether this APEX should support Android10. Default is false. If this is set true, then apex_manifest.json is bundled as well
	// because Android10 requires legacy apex_manifest.json instead of apex_manifest.pb
	Legacy_android10_support *bool

	IsCoverageVariant bool `blueprint:"mutated"`
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

type overridableProperties struct {
	// List of APKs to package inside APEX
	Apps []string

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string
}

type apexPackaging int

const (
	imageApex apexPackaging = iota
	zipApex
	flattenedApex
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

type apexFileClass int

const (
	etc apexFileClass = iota
	nativeSharedLib
	nativeExecutable
	shBinary
	pyBinary
	goBinary
	javaSharedLib
	nativeTest
	app
)

func (class apexFileClass) NameInMake() string {
	switch class {
	case etc:
		return "ETC"
	case nativeSharedLib:
		return "SHARED_LIBRARIES"
	case nativeExecutable, shBinary, pyBinary, goBinary:
		return "EXECUTABLES"
	case javaSharedLib:
		return "JAVA_LIBRARIES"
	case nativeTest:
		return "NATIVE_TESTS"
	case app:
		// b/142537672 Why isn't this APP? We want to have full control over
		// the paths and file names of the apk file under the flattend APEX.
		// If this is set to APP, then the paths and file names are modified
		// by the Make build system. For example, it is installed to
		// /system/apex/<apexname>/app/<Appname>/<apexname>.<Appname>/ instead of
		// /system/apex/<apexname>/app/<Appname> because the build system automatically
		// appends module name (which is <apexname>.<Appname> to the path.
		return "ETC"
	default:
		panic(fmt.Errorf("unknown class %d", class))
	}
}

// apexFile represents a file in an APEX bundle
type apexFile struct {
	builtFile  android.Path
	moduleName string
	installDir string
	class      apexFileClass
	module     android.Module
	// list of symlinks that will be created in installDir that point to this apexFile
	symlinks      []string
	transitiveDep bool
	moduleDir     string

	requiredModuleNames       []string
	targetRequiredModuleNames []string
	hostRequiredModuleNames   []string

	jacocoReportClassesFile android.Path // only for javalibs and apps
}

func newApexFile(ctx android.BaseModuleContext, builtFile android.Path, moduleName string, installDir string, class apexFileClass, module android.Module) apexFile {
	ret := apexFile{
		builtFile:  builtFile,
		moduleName: moduleName,
		installDir: installDir,
		class:      class,
		module:     module,
	}
	if module != nil {
		ret.moduleDir = ctx.OtherModuleDir(module)
		ret.requiredModuleNames = module.RequiredModuleNames()
		ret.targetRequiredModuleNames = module.TargetRequiredModuleNames()
		ret.hostRequiredModuleNames = module.HostRequiredModuleNames()
	}
	return ret
}

func (af *apexFile) Ok() bool {
	return af.builtFile != nil && af.builtFile.String() != ""
}

// Path() returns path of this apex file relative to the APEX root
func (af *apexFile) Path() string {
	return filepath.Join(af.installDir, af.builtFile.Base())
}

// SymlinkPaths() returns paths of the symlinks (if any) relative to the APEX root
func (af *apexFile) SymlinkPaths() []string {
	var ret []string
	for _, symlink := range af.symlinks {
		ret = append(ret, filepath.Join(af.installDir, symlink))
	}
	return ret
}

func (af *apexFile) AvailableToPlatform() bool {
	if af.module == nil {
		return false
	}
	if am, ok := af.module.(android.ApexModule); ok {
		return am.AvailableFor(android.AvailableToPlatform)
	}
	return false
}

type apexBundle struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.OverridableModuleBase
	android.SdkBase

	properties            apexBundleProperties
	targetProperties      apexTargetBundleProperties
	overridableProperties overridableProperties

	// specific to apex_vndk modules
	vndkProperties apexVndkProperties

	bundleModuleFile android.WritablePath
	outputFile       android.WritablePath
	installDir       android.InstallPath

	prebuiltFileToDelete string

	public_key_file  android.Path
	private_key_file android.Path

	container_certificate_file android.Path
	container_private_key_file android.Path

	fileContexts android.Path

	// list of files to be included in this apex
	filesInfo []apexFile

	// list of module names that should be installed along with this APEX
	requiredDeps []string

	// list of module names that this APEX is depending on (to be shown via *-deps-info target)
	externalDeps []string
	// list of module names that this APEX is including (to be shown via *-deps-info target)
	internalDeps []string

	testApex        bool
	vndkApex        bool
	artApex         bool
	primaryApexType bool

	manifestJsonOut android.WritablePath
	manifestPbOut   android.WritablePath

	// list of commands to create symlinks for backward compatibility.
	// these commands will be attached as LOCAL_POST_INSTALL_CMD to
	// apex package itself(for unflattened build) or apex_manifest(for flattened build)
	// so that compat symlinks are always installed regardless of TARGET_FLATTEN_APEX setting.
	compatSymlinks []string

	// Suffix of module name in Android.mk
	// ".flattened", ".apex", ".zipapex", or ""
	suffix string

	installedFilesFile android.WritablePath

	// Whether to create symlink to the system file instead of having a file
	// inside the apex or not
	linkToSystemLib bool
}

func addDependenciesForNativeModules(ctx android.BottomUpMutatorContext,
	native_shared_libs []string, binaries []string, tests []string,
	target android.Target, imageVariation string) {
	// Use *FarVariation* to be able to depend on modules having
	// conflicting variations with this module. This is required since
	// arch variant of an APEX bundle is 'common' but it is 'arm' or 'arm64'
	// for native shared libs.
	ctx.AddFarVariationDependencies(append(target.Variations(), []blueprint.Variation{
		{Mutator: "image", Variation: imageVariation},
		{Mutator: "link", Variation: "shared"},
		{Mutator: "version", Variation: ""}, // "" is the non-stub variant
	}...), sharedLibTag, native_shared_libs...)

	ctx.AddFarVariationDependencies(append(target.Variations(),
		blueprint.Variation{Mutator: "image", Variation: imageVariation}),
		executableTag, binaries...)

	ctx.AddFarVariationDependencies(append(target.Variations(), []blueprint.Variation{
		{Mutator: "image", Variation: imageVariation},
		{Mutator: "test_per_src", Variation: ""}, // "" is the all-tests variant
	}...), testTag, tests...)
}

func (a *apexBundle) combineProperties(ctx android.BottomUpMutatorContext) {
	if ctx.Os().Class == android.Device {
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

func (a *apexBundle) DepsMutator(ctx android.BottomUpMutatorContext) {
	if proptools.Bool(a.properties.Use_vendor) && !android.InList(a.Name(), useVendorWhitelist(ctx.Config())) {
		ctx.PropertyErrorf("use_vendor", "not allowed to set use_vendor: true")
	}

	targets := ctx.MultiTargets()
	config := ctx.DeviceConfig()

	a.combineProperties(ctx)

	has32BitTarget := false
	for _, target := range targets {
		if target.Arch.ArchType.Multilib == "lib32" {
			has32BitTarget = true
		}
	}
	for i, target := range targets {
		// When multilib.* is omitted for native_shared_libs, it implies
		// multilib.both.
		ctx.AddFarVariationDependencies(append(target.Variations(), []blueprint.Variation{
			{Mutator: "image", Variation: a.getImageVariation(config)},
			{Mutator: "link", Variation: "shared"},
		}...), sharedLibTag, a.properties.Native_shared_libs...)

		// When multilib.* is omitted for tests, it implies
		// multilib.both.
		ctx.AddFarVariationDependencies(append(target.Variations(), []blueprint.Variation{
			{Mutator: "image", Variation: a.getImageVariation(config)},
			{Mutator: "test_per_src", Variation: ""}, // "" is the all-tests variant
		}...), testTag, a.properties.Tests...)

		// Add native modules targetting both ABIs
		addDependenciesForNativeModules(ctx,
			a.properties.Multilib.Both.Native_shared_libs,
			a.properties.Multilib.Both.Binaries,
			a.properties.Multilib.Both.Tests,
			target,
			a.getImageVariation(config))

		isPrimaryAbi := i == 0
		if isPrimaryAbi {
			// When multilib.* is omitted for binaries, it implies
			// multilib.first.
			ctx.AddFarVariationDependencies(append(target.Variations(),
				blueprint.Variation{Mutator: "image", Variation: a.getImageVariation(config)}),
				executableTag, a.properties.Binaries...)

			// Add native modules targetting the first ABI
			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.First.Native_shared_libs,
				a.properties.Multilib.First.Binaries,
				a.properties.Multilib.First.Tests,
				target,
				a.getImageVariation(config))
		}

		switch target.Arch.ArchType.Multilib {
		case "lib32":
			// Add native modules targetting 32-bit ABI
			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.Lib32.Native_shared_libs,
				a.properties.Multilib.Lib32.Binaries,
				a.properties.Multilib.Lib32.Tests,
				target,
				a.getImageVariation(config))

			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.Prefer32.Native_shared_libs,
				a.properties.Multilib.Prefer32.Binaries,
				a.properties.Multilib.Prefer32.Tests,
				target,
				a.getImageVariation(config))
		case "lib64":
			// Add native modules targetting 64-bit ABI
			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.Lib64.Native_shared_libs,
				a.properties.Multilib.Lib64.Binaries,
				a.properties.Multilib.Lib64.Tests,
				target,
				a.getImageVariation(config))

			if !has32BitTarget {
				addDependenciesForNativeModules(ctx,
					a.properties.Multilib.Prefer32.Native_shared_libs,
					a.properties.Multilib.Prefer32.Binaries,
					a.properties.Multilib.Prefer32.Tests,
					target,
					a.getImageVariation(config))
			}

			if strings.HasPrefix(ctx.ModuleName(), "com.android.runtime") && target.Os.Class == android.Device {
				for _, sanitizer := range ctx.Config().SanitizeDevice() {
					if sanitizer == "hwaddress" {
						addDependenciesForNativeModules(ctx,
							[]string{"libclang_rt.hwasan-aarch64-android"},
							nil, nil, target, a.getImageVariation(config))
						break
					}
				}
			}
		}

	}

	// For prebuilt_etc, use the first variant (64 on 64/32bit device,
	// 32 on 32bit device) regardless of the TARGET_PREFER_* setting.
	// b/144532908
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

	ctx.AddFarVariationDependencies(ctx.Config().AndroidCommonTarget.Variations(),
		javaLibTag, a.properties.Java_libs...)

	// With EMMA_INSTRUMENT_FRAMEWORK=true the ART boot image includes jacoco library.
	if a.artApex && ctx.Config().IsEnvTrue("EMMA_INSTRUMENT_FRAMEWORK") {
		ctx.AddFarVariationDependencies(ctx.Config().AndroidCommonTarget.Variations(),
			javaLibTag, "jacocoagent")
	}

	if String(a.properties.Key) == "" {
		ctx.ModuleErrorf("key is missing")
		return
	}
	ctx.AddDependency(ctx.Module(), keyTag, String(a.properties.Key))

	cert := android.SrcIsModule(a.getCertString(ctx))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
	}

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

func (a *apexBundle) OverridablePropertiesDepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddFarVariationDependencies(ctx.Config().AndroidCommonTarget.Variations(),
		androidAppTag, a.overridableProperties.Apps...)
}

func (a *apexBundle) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	// direct deps of an APEX bundle are all part of the APEX bundle
	return true
}

func (a *apexBundle) getCertString(ctx android.BaseModuleContext) string {
	moduleName := ctx.ModuleName()
	// VNDK APEXes share the same certificate. To avoid adding a new VNDK version to the OVERRIDE_* list,
	// we check with the pseudo module name to see if its certificate is overridden.
	if a.vndkApex {
		moduleName = vndkApexName
	}
	certificate, overridden := ctx.DeviceConfig().OverrideCertificateFor(moduleName)
	if overridden {
		return ":" + certificate
	}
	return String(a.properties.Certificate)
}

func (a *apexBundle) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{a.outputFile}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (a *apexBundle) installable() bool {
	return !a.properties.PreventInstall && (a.properties.Installable == nil || proptools.Bool(a.properties.Installable))
}

func (a *apexBundle) testOnlyShouldSkipHashtreeGeneration() bool {
	return proptools.Bool(a.properties.Test_only_no_hashtree)
}

func (a *apexBundle) getImageVariation(config android.DeviceConfig) string {
	if a.vndkApex {
		return cc.VendorVariationPrefix + a.vndkVersion(config)
	}
	if config.VndkVersion() != "" && proptools.Bool(a.properties.Use_vendor) {
		return cc.VendorVariationPrefix + config.PlatformVndkVersion()
	} else {
		return android.CoreVariation
	}
}

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

func (a *apexBundle) IsNativeCoverageNeeded(ctx android.BaseModuleContext) bool {
	return ctx.Device() && ctx.DeviceConfig().NativeCoverageEnabled()
}

func (a *apexBundle) PreventInstall() {
	a.properties.PreventInstall = true
}

func (a *apexBundle) HideFromMake() {
	a.properties.HideFromMake = true
}

func (a *apexBundle) MarkAsCoverageVariant(coverage bool) {
	a.properties.IsCoverageVariant = coverage
}

// TODO(jiyong) move apexFileFor* close to the apexFile type definition
func apexFileForNativeLibrary(ctx android.BaseModuleContext, ccMod *cc.Module, handleSpecialLibs bool) apexFile {
	// Decide the APEX-local directory by the multilib of the library
	// In the future, we may query this to the module.
	var dirInApex string
	switch ccMod.Arch().ArchType.Multilib {
	case "lib32":
		dirInApex = "lib"
	case "lib64":
		dirInApex = "lib64"
	}
	dirInApex = filepath.Join(dirInApex, ccMod.RelativeInstallPath())
	if ccMod.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, ccMod.Target().NativeBridgeRelativePath)
	}
	if handleSpecialLibs && cc.InstallToBootstrap(ccMod.BaseModuleName(), ctx.Config()) {
		// Special case for Bionic libs and other libs installed with them. This is
		// to prevent those libs from being included in the search path
		// /apex/com.android.runtime/${LIB}. This exclusion is required because
		// those libs in the Runtime APEX are available via the legacy paths in
		// /system/lib/. By the init process, the libs in the APEX are bind-mounted
		// to the legacy paths and thus will be loaded into the default linker
		// namespace (aka "platform" namespace). If the libs are directly in
		// /apex/com.android.runtime/${LIB} then the same libs will be loaded again
		// into the runtime linker namespace, which will result in double loading of
		// them, which isn't supported.
		dirInApex = filepath.Join(dirInApex, "bionic")
	}

	fileToCopy := ccMod.OutputFile().Path()
	return newApexFile(ctx, fileToCopy, ccMod.Name(), dirInApex, nativeSharedLib, ccMod)
}

func apexFileForExecutable(ctx android.BaseModuleContext, cc *cc.Module) apexFile {
	dirInApex := filepath.Join("bin", cc.RelativeInstallPath())
	if cc.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, cc.Target().NativeBridgeRelativePath)
	}
	fileToCopy := cc.OutputFile().Path()
	af := newApexFile(ctx, fileToCopy, cc.Name(), dirInApex, nativeExecutable, cc)
	af.symlinks = cc.Symlinks()
	return af
}

func apexFileForPyBinary(ctx android.BaseModuleContext, py *python.Module) apexFile {
	dirInApex := "bin"
	fileToCopy := py.HostToolPath().Path()
	return newApexFile(ctx, fileToCopy, py.Name(), dirInApex, pyBinary, py)
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

func apexFileForShBinary(ctx android.BaseModuleContext, sh *android.ShBinary) apexFile {
	dirInApex := filepath.Join("bin", sh.SubDir())
	fileToCopy := sh.OutputFile()
	af := newApexFile(ctx, fileToCopy, sh.Name(), dirInApex, shBinary, sh)
	af.symlinks = sh.Symlinks()
	return af
}

// TODO(b/146586360): replace javaLibrary(in apex/apex.go) with java.Dependency
type javaLibrary interface {
	android.Module
	java.Dependency
}

func apexFileForJavaLibrary(ctx android.BaseModuleContext, lib javaLibrary) apexFile {
	dirInApex := "javalib"
	fileToCopy := lib.DexJar()
	af := newApexFile(ctx, fileToCopy, lib.Name(), dirInApex, javaSharedLib, lib)
	af.jacocoReportClassesFile = lib.JacocoReportClassesFile()
	return af
}

func apexFileForPrebuiltEtc(ctx android.BaseModuleContext, prebuilt android.PrebuiltEtcModule, depName string) apexFile {
	dirInApex := filepath.Join("etc", prebuilt.SubDir())
	fileToCopy := prebuilt.OutputFile()
	return newApexFile(ctx, fileToCopy, depName, dirInApex, etc, prebuilt)
}

func apexFileForAndroidApp(ctx android.BaseModuleContext, aapp interface {
	android.Module
	Privileged() bool
	OutputFile() android.Path
	JacocoReportClassesFile() android.Path
}, pkgName string) apexFile {
	appDir := "app"
	if aapp.Privileged() {
		appDir = "priv-app"
	}
	dirInApex := filepath.Join(appDir, pkgName)
	fileToCopy := aapp.OutputFile()
	af := newApexFile(ctx, fileToCopy, aapp.Name(), dirInApex, app, aapp)
	af.jacocoReportClassesFile = aapp.JacocoReportClassesFile()
	return af
}

// Context "decorator", overriding the InstallBypassMake method to always reply `true`.
type flattenedApexContext struct {
	android.ModuleContext
}

func (c *flattenedApexContext) InstallBypassMake() bool {
	return true
}

func (a *apexBundle) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	buildFlattenedAsDefault := ctx.Config().FlattenApex() && !ctx.Config().UnbundledBuild()
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

	if len(a.properties.Tests) > 0 && !a.testApex {
		ctx.PropertyErrorf("tests", "property not allowed in apex module type")
		return
	}

	handleSpecialLibs := !android.Bool(a.properties.Ignore_system_library_special_case)

	// native lib dependencies
	var provideNativeLibs []string
	var requireNativeLibs []string

	// Check if "uses" requirements are met with dependent apexBundles
	var providedNativeSharedLibs []string
	useVendor := proptools.Bool(a.properties.Use_vendor)
	ctx.VisitDirectDepsBlueprint(func(m blueprint.Module) {
		if ctx.OtherModuleDependencyTag(m) != usesTag {
			return
		}
		otherName := ctx.OtherModuleName(m)
		other, ok := m.(*apexBundle)
		if !ok {
			ctx.PropertyErrorf("uses", "%q is not a provider", otherName)
			return
		}
		if proptools.Bool(other.properties.Use_vendor) != useVendor {
			ctx.PropertyErrorf("use_vendor", "%q has different value of use_vendor", otherName)
			return
		}
		if !proptools.Bool(other.properties.Provide_cpp_shared_libs) {
			ctx.PropertyErrorf("uses", "%q does not provide native_shared_libs", otherName)
			return
		}
		providedNativeSharedLibs = append(providedNativeSharedLibs, other.properties.Native_shared_libs...)
	})

	var filesInfo []apexFile
	ctx.WalkDepsBlueprint(func(child, parent blueprint.Module) bool {
		depTag := ctx.OtherModuleDependencyTag(child)
		depName := ctx.OtherModuleName(child)
		if _, isDirectDep := parent.(*apexBundle); isDirectDep {
			if depTag != keyTag && depTag != certificateTag {
				a.internalDeps = append(a.internalDeps, depName)
			}
			switch depTag {
			case sharedLibTag:
				if cc, ok := child.(*cc.Module); ok {
					if cc.HasStubsVariants() {
						provideNativeLibs = append(provideNativeLibs, cc.OutputFile().Path().Base())
					}
					filesInfo = append(filesInfo, apexFileForNativeLibrary(ctx, cc, handleSpecialLibs))
					return true // track transitive dependencies
				} else {
					ctx.PropertyErrorf("native_shared_libs", "%q is not a cc_library or cc_library_shared module", depName)
				}
			case executableTag:
				if cc, ok := child.(*cc.Module); ok {
					filesInfo = append(filesInfo, apexFileForExecutable(ctx, cc))
					return true // track transitive dependencies
				} else if sh, ok := child.(*android.ShBinary); ok {
					filesInfo = append(filesInfo, apexFileForShBinary(ctx, sh))
				} else if py, ok := child.(*python.Module); ok && py.HostToolPath().Valid() {
					filesInfo = append(filesInfo, apexFileForPyBinary(ctx, py))
				} else if gb, ok := child.(bootstrap.GoBinaryTool); ok && a.Host() {
					filesInfo = append(filesInfo, apexFileForGoBinary(ctx, depName, gb))
				} else {
					ctx.PropertyErrorf("binaries", "%q is neither cc_binary, (embedded) py_binary, (host) blueprint_go_binary, (host) bootstrap_go_binary, nor sh_binary", depName)
				}
			case javaLibTag:
				if javaLib, ok := child.(*java.Library); ok {
					af := apexFileForJavaLibrary(ctx, javaLib)
					if !af.Ok() {
						ctx.PropertyErrorf("java_libs", "%q is not configured to be compiled into dex", depName)
					} else {
						filesInfo = append(filesInfo, af)
						return true // track transitive dependencies
					}
				} else if sdkLib, ok := child.(*java.SdkLibrary); ok {
					af := apexFileForJavaLibrary(ctx, sdkLib)
					if !af.Ok() {
						ctx.PropertyErrorf("java_libs", "%q is not configured to be compiled into dex", depName)
						return false
					}
					filesInfo = append(filesInfo, af)

					pf, _ := sdkLib.OutputFiles(".xml")
					if len(pf) != 1 {
						ctx.PropertyErrorf("java_libs", "%q failed to generate permission XML", depName)
						return false
					}
					filesInfo = append(filesInfo, newApexFile(ctx, pf[0], pf[0].Base(), "etc/permissions", etc, nil))
					return true // track transitive dependencies
				} else {
					ctx.PropertyErrorf("java_libs", "%q of type %q is not supported", depName, ctx.OtherModuleType(child))
				}
			case androidAppTag:
				pkgName := ctx.DeviceConfig().OverridePackageNameFor(depName)
				if ap, ok := child.(*java.AndroidApp); ok {
					filesInfo = append(filesInfo, apexFileForAndroidApp(ctx, ap, pkgName))
					return true // track transitive dependencies
				} else if ap, ok := child.(*java.AndroidAppImport); ok {
					filesInfo = append(filesInfo, apexFileForAndroidApp(ctx, ap, pkgName))
				} else if ap, ok := child.(*java.AndroidTestHelperApp); ok {
					filesInfo = append(filesInfo, apexFileForAndroidApp(ctx, ap, pkgName))
				} else {
					ctx.PropertyErrorf("apps", "%q is not an android_app module", depName)
				}
			case prebuiltTag:
				if prebuilt, ok := child.(android.PrebuiltEtcModule); ok {
					filesInfo = append(filesInfo, apexFileForPrebuiltEtc(ctx, prebuilt, depName))
				} else {
					ctx.PropertyErrorf("prebuilts", "%q is not a prebuilt_etc module", depName)
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
						return true
					} else {
						// Single-output test module (where `test_per_src: false`).
						af := apexFileForExecutable(ctx, ccTest)
						af.class = nativeTest
						filesInfo = append(filesInfo, af)
					}
				} else {
					ctx.PropertyErrorf("tests", "%q is not a cc module", depName)
				}
			case keyTag:
				if key, ok := child.(*apexKey); ok {
					a.private_key_file = key.private_key_file
					a.public_key_file = key.public_key_file
				} else {
					ctx.PropertyErrorf("key", "%q is not an apex_key module", depName)
				}
				return false
			case certificateTag:
				if dep, ok := child.(*java.AndroidAppCertificate); ok {
					a.container_certificate_file = dep.Certificate.Pem
					a.container_private_key_file = dep.Certificate.Key
				} else {
					ctx.ModuleErrorf("certificate dependency %q must be an android_app_certificate module", depName)
				}
			case android.PrebuiltDepTag:
				// If the prebuilt is force disabled, remember to delete the prebuilt file
				// that might have been installed in the previous builds
				if prebuilt, ok := child.(*Prebuilt); ok && prebuilt.isForceDisabled() {
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
						if android.InList(cc.Name(), providedNativeSharedLibs) {
							// If we're using a shared library which is provided from other APEX,
							// don't include it in this APEX
							return false
						}
						if !a.Host() && !android.DirectlyInApex(ctx.ModuleName(), ctx.OtherModuleName(cc)) && (cc.IsStubs() || cc.HasStubsVariants()) {
							// If the dependency is a stubs lib, don't include it in this APEX,
							// but make sure that the lib is installed on the device.
							// In case no APEX is having the lib, the lib is installed to the system
							// partition.
							//
							// Always include if we are a host-apex however since those won't have any
							// system libraries.
							if !android.DirectlyInAnyApex(ctx, cc.Name()) && !android.InList(cc.Name(), a.requiredDeps) {
								a.requiredDeps = append(a.requiredDeps, cc.Name())
							}
							a.externalDeps = append(a.externalDeps, depName)
							requireNativeLibs = append(requireNativeLibs, cc.OutputFile().Path().Base())
							// Don't track further
							return false
						}
						af := apexFileForNativeLibrary(ctx, cc, handleSpecialLibs)
						af.transitiveDep = true
						filesInfo = append(filesInfo, af)
						a.internalDeps = append(a.internalDeps, depName)
						a.internalDeps = append(a.internalDeps, cc.AllStaticDeps()...)
						return true // track transitive dependencies
					}
				} else if cc.IsTestPerSrcDepTag(depTag) {
					if cc, ok := child.(*cc.Module); ok {
						af := apexFileForExecutable(ctx, cc)
						// Handle modules created as `test_per_src` variations of a single test module:
						// use the name of the generated test binary (`fileToCopy`) instead of the name
						// of the original test module (`depName`, shared by all `test_per_src`
						// variations of that module).
						af.moduleName = filepath.Base(af.builtFile.String())
						// these are not considered transitive dep
						af.transitiveDep = false
						filesInfo = append(filesInfo, af)
						return true // track transitive dependencies
					}
				} else if java.IsJniDepTag(depTag) {
					a.externalDeps = append(a.externalDeps, depName)
					return true
				} else if java.IsStaticLibDepTag(depTag) {
					a.internalDeps = append(a.internalDeps, depName)
				} else if am.CanHaveApexVariants() && am.IsInstallableToApex() {
					ctx.ModuleErrorf("unexpected tag %q for indirect dependency %q", depTag, depName)
				}
			}
		}
		return false
	})

	// Specific to the ART apex: dexpreopt artifacts for libcore Java libraries.
	// Build rules are generated by the dexpreopt singleton, and here we access build artifacts
	// via the global boot image config.
	if a.artApex {
		for arch, files := range java.DexpreoptedArtApexJars(ctx) {
			dirInApex := filepath.Join("javalib", arch.String())
			for _, f := range files {
				localModule := "javalib_" + arch.String() + "_" + filepath.Base(f.String())
				af := newApexFile(ctx, f, localModule, dirInApex, etc, nil)
				filesInfo = append(filesInfo, af)
			}
		}
	}

	if a.private_key_file == nil {
		ctx.PropertyErrorf("key", "private_key for %q could not be found", String(a.properties.Key))
		return
	}

	// remove duplicates in filesInfo
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

	// to have consistent build rules
	sort.Slice(filesInfo, func(i, j int) bool {
		return filesInfo[i].builtFile.String() < filesInfo[j].builtFile.String()
	})

	// check apex_available requirements
	if !ctx.Host() && !a.testApex {
		for _, fi := range filesInfo {
			if am, ok := fi.module.(android.ApexModule); ok {
				// vndk {enabled:true} implies visibility to the vndk apex
				if ccm, ok := fi.module.(*cc.Module); ok && ccm.IsVndk() && a.vndkApex {
					continue
				}

				if !am.AvailableFor(ctx.ModuleName()) && !whitelistedApexAvailable(ctx.ModuleName(), a.vndkApex, fi.module) {
					ctx.ModuleErrorf("requires %q that is not available for the APEX", fi.module.Name())
					// don't stop so that we can report other violations in the same run
				}
			}
		}
	}

	a.installDir = android.PathForModuleInstall(ctx, "apex")
	a.filesInfo = filesInfo

	if a.properties.ApexType != zipApex {
		if a.properties.File_contexts == nil {
			a.fileContexts = android.PathForSource(ctx, "system/sepolicy/apex", ctx.ModuleName()+"-file_contexts")
		} else {
			a.fileContexts = android.PathForModuleSrc(ctx, *a.properties.File_contexts)
			if a.Platform() {
				if matched, err := path.Match("system/sepolicy/**/*", a.fileContexts.String()); err != nil || !matched {
					ctx.PropertyErrorf("file_contexts", "should be under system/sepolicy, but %q", a.fileContexts)
				}
			}
		}
		if !android.ExistentPathForSource(ctx, a.fileContexts.String()).Valid() {
			ctx.PropertyErrorf("file_contexts", "cannot find file_contexts file: %q", a.fileContexts)
			return
		}
	}
	// Optimization. If we are building bundled APEX, for the files that are gathered due to the
	// transitive dependencies, don't place them inside the APEX, but place a symlink pointing
	// the same library in the system partition, thus effectively sharing the same libraries
	// across the APEX boundary. For unbundled APEX, all the gathered files are actually placed
	// in the APEX.
	a.linkToSystemLib = !ctx.Config().UnbundledBuild() &&
		a.installable() &&
		!proptools.Bool(a.properties.Use_vendor)

	// prepare apex_manifest.json
	a.buildManifest(ctx, provideNativeLibs, requireNativeLibs)

	a.setCertificateAndPrivateKey(ctx)
	if a.properties.ApexType == flattenedApex {
		a.buildFlattenedApex(ctx)
	} else {
		a.buildUnflattenedApex(ctx)
	}

	a.compatSymlinks = makeCompatSymlinks(a.BaseModuleName(), ctx)

	a.buildApexDependencyInfo(ctx)
}

func whitelistedApexAvailable(apex string, is_vndk bool, module android.Module) bool {
	key := apex
	key = strings.Replace(key, "test_", "", 1)
	key = strings.Replace(key, "com.android.art.debug", "com.android.art", 1)
	key = strings.Replace(key, "com.android.art.release", "com.android.art", 1)

	moduleName := module.Name()
	if strings.Contains(moduleName, "prebuilt_libclang_rt") {
		// This module has variants that depend on the product being built.
		moduleName = "prebuilt_libclang_rt"
	}

	if val, ok := apexAvailWl[key]; ok && android.InList(moduleName, val) {
		return true
	}

	return false
}

func newApexBundle() *apexBundle {
	module := &apexBundle{}
	module.AddProperties(&module.properties)
	module.AddProperties(&module.targetProperties)
	module.AddProperties(&module.overridableProperties)
	module.Prefer32(func(ctx android.BaseModuleContext, base *android.ModuleBase, class android.OsClass) bool {
		return class == android.Device && ctx.Config().DevicePrefer32BitExecutables()
	})
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

func testApexBundleFactory() android.Module {
	bundle := newApexBundle()
	bundle.testApex = true
	return bundle
}

func BundleFactory() android.Module {
	return newApexBundle()
}

//
// Defaults
//
type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

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

//
// OverrideApex
//
type OverrideApex struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (o *OverrideApex) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// All the overrides happen in the base module.
}

// override_apex is used to create an apex module based on another apex module
// by overriding some of its properties.
func overrideApexFactory() android.Module {
	m := &OverrideApex{}
	m.AddProperties(&overridableProperties{})

	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, android.MultilibCommon)
	android.InitOverrideModule(m)
	return m
}
