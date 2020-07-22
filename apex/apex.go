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
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	prebuilt_etc "android/soong/etc"
	"android/soong/java"
	"android/soong/python"
	"android/soong/sh"
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

	// determines if the dependent will be part of the APEX payload
	payload bool
}

var (
	sharedLibTag   = dependencyTag{name: "sharedLib", payload: true}
	executableTag  = dependencyTag{name: "executable", payload: true}
	javaLibTag     = dependencyTag{name: "javaLib", payload: true}
	prebuiltTag    = dependencyTag{name: "prebuilt", payload: true}
	testTag        = dependencyTag{name: "test", payload: true}
	keyTag         = dependencyTag{name: "key"}
	certificateTag = dependencyTag{name: "certificate"}
	usesTag        = dependencyTag{name: "uses"}
	androidAppTag  = dependencyTag{name: "androidApp", payload: true}

	apexAvailBaseline = makeApexAvailableBaseline()

	inverseApexAvailBaseline = invertApexBaseline(apexAvailBaseline)
)

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

// This is a map from apex to modules, which overrides the
// apex_available setting for that particular module to make
// it available for the apex regardless of its setting.
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
	m["com.android.conscrypt"] = []string{
		"libnativehelper_header_only",
	}
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
		"libdexfile_external_headers",
		"libdexfile_support",
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
		"libnativebridge-headers",
		"libnativebridge_lazy",
		"libnativeloader-headers",
		"libnativeloader_lazy",
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
		"libc_scudo",
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
		"libdexfile_support",
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
		"libnativebridge_lazy",
		"libnativeloader_lazy",
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
		"libstagefright_bufferqueue_helper",
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
		"libnativehelper_header_only",
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
		"libdexfile_external_headers",
		"libdexfile_support",
		"libdexfile_support_static",
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
		"ipmemorystore-aidl-interfaces-java",
		"libcgrouprc",
		"libcgrouprc_format",
		"libnativehelper_compat_libc++",
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
		"libunwind_llvm",
	}
	return m
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

	android.RegisterMakeVarsProvider(pctx, func(ctx android.MakeVarsContext) {
		apexFileContextsInfos := apexFileContextsInfos(ctx.Config())
		sort.Strings(*apexFileContextsInfos)
		ctx.Strict("APEX_FILE_CONTEXTS_INFOS", strings.Join(*apexFileContextsInfos, " "))
	})

	android.AddNeverAllowRules(createApexPermittedPackagesRules(qModulesPackages())...)
	android.AddNeverAllowRules(createApexPermittedPackagesRules(rModulesPackages())...)
}

func createApexPermittedPackagesRules(modules_packages map[string][]string) []android.Rule {
	rules := make([]android.Rule, 0, len(modules_packages))
	for module_name, module_packages := range modules_packages {
		permitted_packages_rule := android.NeverAllow().
			BootclasspathJar().
			With("apex_available", module_name).
			WithMatcher("permitted_packages", android.NotInList(module_packages)).
			Because("jars that are part of the " + module_name +
				" module may only allow these packages: " + strings.Join(module_packages, ",") +
				". Please jarjar or move code around.")
		rules = append(rules, permitted_packages_rule)
	}
	return rules
}

func RegisterPreDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.TopDown("apex_vndk", apexVndkMutator).Parallel()
	ctx.BottomUp("apex_vndk_deps", apexVndkDepsMutator).Parallel()
}

func RegisterPostDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.TopDown("apex_deps", apexDepsMutator).Parallel()
	ctx.BottomUp("apex", apexMutator).Parallel()
	ctx.BottomUp("apex_flattened", apexFlattenedMutator).Parallel()
	ctx.BottomUp("apex_uses", apexUsesMutator).Parallel()
	ctx.BottomUp("mark_platform_availability", markPlatformAvailability).Parallel()
}

// Mark the direct and transitive dependencies of apex bundles so that they
// can be built for the apex bundles.
func apexDepsMutator(mctx android.TopDownMutatorContext) {
	if !mctx.Module().Enabled() {
		return
	}
	a, ok := mctx.Module().(*apexBundle)
	if !ok || a.vndkApex {
		return
	}
	apexInfo := android.ApexInfo{
		ApexName:      mctx.ModuleName(),
		MinSdkVersion: a.minSdkVersion(mctx),
		Updatable:     a.Updatable(),
	}
	mctx.WalkDeps(func(child, parent android.Module) bool {
		am, ok := child.(android.ApexModule)
		if !ok || !am.CanHaveApexVariants() {
			return false
		}
		if !parent.(android.DepIsInSameApex).DepIsInSameApex(mctx, child) {
			return false
		}

		depName := mctx.OtherModuleName(child)
		// If the parent is apexBundle, this child is directly depended.
		_, directDep := parent.(*apexBundle)
		android.UpdateApexDependency(apexInfo, depName, directDep)
		am.BuildForApex(apexInfo)
		return true
	})
}

// mark if a module cannot be available to platform. A module cannot be available
// to platform if 1) it is explicitly marked as not available (i.e. "//apex_available:platform"
// is absent) or 2) it depends on another module that isn't (or can't be) available to platform
func markPlatformAvailability(mctx android.BottomUpMutatorContext) {
	// Host and recovery are not considered as platform
	if mctx.Host() || mctx.Module().InstallInRecovery() {
		return
	}

	if am, ok := mctx.Module().(android.ApexModule); ok {
		availableToPlatform := am.AvailableFor(android.AvailableToPlatform)

		// In a rare case when a lib is marked as available only to an apex
		// but the apex doesn't exist. This can happen in a partial manifest branch
		// like master-art. Currently, libstatssocket in the stats APEX is causing
		// this problem.
		// Include the lib in platform because the module SDK that ought to provide
		// it doesn't exist, so it would otherwise be left out completely.
		// TODO(b/154888298) remove this by adding those libraries in module SDKS and skipping
		// this check for libraries provided by SDKs.
		if !availableToPlatform && !android.InAnyApex(am.Name()) {
			availableToPlatform = true
		}

		// If any of the dep is not available to platform, this module is also considered
		// as being not available to platform even if it has "//apex_available:platform"
		mctx.VisitDirectDeps(func(child android.Module) {
			if !am.DepIsInSameApex(mctx, child) {
				// if the dependency crosses apex boundary, don't consider it
				return
			}
			if dep, ok := child.(android.ApexModule); ok && dep.NotAvailableForPlatform() {
				availableToPlatform = false
				// TODO(b/154889534) trigger an error when 'am' has "//apex_available:platform"
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
}

// If a module in an APEX depends on a module from an SDK then it needs an APEX
// specific variant created for it. Refer to sdk.sdkDepsReplaceMutator.
func inAnySdk(module android.Module) bool {
	if sa, ok := module.(android.SdkAware); ok {
		return sa.IsInAnySdk()
	}

	return false
}

// Create apex variations if a module is included in APEX(s).
func apexMutator(mctx android.BottomUpMutatorContext) {
	if !mctx.Module().Enabled() {
		return
	}
	if am, ok := mctx.Module().(android.ApexModule); ok && am.CanHaveApexVariants() {
		am.CreateApexVariations(mctx)
	} else if a, ok := mctx.Module().(*apexBundle); ok && !a.vndkApex {
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
	if !mctx.Module().Enabled() {
		return
	}
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
	useVendorAllowListKey = android.NewOnceKey("useVendorAllowList")
)

// useVendorAllowList returns the list of APEXes which are allowed to use_vendor.
// When use_vendor is used, native modules are built with __ANDROID_VNDK__ and __ANDROID_APEX__,
// which may cause compatibility issues. (e.g. libbinder)
// Even though libbinder restricts its availability via 'apex_available' property and relies on
// yet another macro __ANDROID_APEX_<NAME>__, we restrict usage of "use_vendor:" from other APEX modules
// to avoid similar problems.
func useVendorAllowList(config android.Config) []string {
	return config.Once(useVendorAllowListKey, func() interface{} {
		return []string{
			// swcodec uses "vendor" variants for smaller size
			"com.android.media.swcodec",
			"test_com.android.media.swcodec",
		}
	}).([]string)
}

// setUseVendorAllowListForTest overrides useVendorAllowList and must be
// called before the first call to useVendorAllowList()
func setUseVendorAllowListForTest(config android.Config, allowList []string) {
	config.Once(useVendorAllowListKey, func() interface{} {
		return allowList
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

	// Whenever apex_payload.img of the APEX should not be dm-verity signed.
	// Should be only used in tests#.
	Test_only_unsigned_payload *bool

	IsCoverageVariant bool `blueprint:"mutated"`

	// Whether this APEX is considered updatable or not. When set to true, this will enforce additional
	// rules for making sure that the APEX is truly updatable.
	// - To be updatable, min_sdk_version should be set as well
	// This will also disable the size optimizations like symlinking to the system libs.
	// Default is false.
	Updatable *bool

	// The minimum SDK version that this apex must be compatible with.
	Min_sdk_version *string
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

	// Logging Parent value
	Logging_parent string

	// Apex Container Package Name.
	// Override value for attribute package:name in AndroidManifest.xml
	Package_name string

	// A txt file containing list of files that are allowed to be included in this APEX.
	Allowed_files *string `android:"path"`
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
	appSet
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
	case app, appSet:
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
	stem       string
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

	jacocoReportClassesFile android.Path     // only for javalibs and apps
	lintDepSets             java.LintDepSets // only for javalibs and apps
	certificate             java.Certificate // only for apps
	overriddenPackageName   string           // only for apps
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

func (af *apexFile) apexRelativePath(path string) string {
	return filepath.Join(af.installDir, path)
}

// Path() returns path of this apex file relative to the APEX root
func (af *apexFile) Path() string {
	return af.apexRelativePath(af.Stem())
}

func (af *apexFile) Stem() string {
	if af.stem != "" {
		return af.stem
	}
	return af.builtFile.Base()
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

	// list of module names that this APEX is including (to be shown via *-deps-info target)
	android.ApexBundleDepsInfo

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

	// Struct holding the merged notice file paths in different formats
	mergedNotices android.NoticeOutputs

	// Optional list of lint report zip files for apexes that contain java or app modules
	lintReports android.Paths
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
	if proptools.Bool(a.properties.Use_vendor) && !android.InList(a.Name(), useVendorAllowList(ctx.Config())) {
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
	if a.overridableProperties.Allowed_files != nil {
		android.ExtractSourceDeps(ctx, a.overridableProperties.Allowed_files)
	}
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

func (a *apexBundle) testOnlyShouldSkipPayloadSign() bool {
	return proptools.Bool(a.properties.Test_only_unsigned_payload)
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

func apexFileForShBinary(ctx android.BaseModuleContext, sh *sh.ShBinary) apexFile {
	dirInApex := filepath.Join("bin", sh.SubDir())
	fileToCopy := sh.OutputFile()
	af := newApexFile(ctx, fileToCopy, sh.Name(), dirInApex, shBinary, sh)
	af.symlinks = sh.Symlinks()
	return af
}

type javaDependency interface {
	DexJar() android.Path
	JacocoReportClassesFile() android.Path
	LintDepSets() java.LintDepSets

	Stem() string
}

var _ javaDependency = (*java.Library)(nil)
var _ javaDependency = (*java.SdkLibrary)(nil)
var _ javaDependency = (*java.DexImport)(nil)
var _ javaDependency = (*java.SdkLibraryImport)(nil)

func apexFileForJavaLibrary(ctx android.BaseModuleContext, lib javaDependency, module android.Module) apexFile {
	dirInApex := "javalib"
	fileToCopy := lib.DexJar()
	// Remove prebuilt_ if necessary so the source and prebuilt modules have the same name.
	name := strings.TrimPrefix(module.Name(), "prebuilt_")
	af := newApexFile(ctx, fileToCopy, name, dirInApex, javaSharedLib, module)
	af.jacocoReportClassesFile = lib.JacocoReportClassesFile()
	af.lintDepSets = lib.LintDepSets()
	af.stem = lib.Stem() + ".jar"
	return af
}

func apexFileForPrebuiltEtc(ctx android.BaseModuleContext, prebuilt prebuilt_etc.PrebuiltEtcModule, depName string) apexFile {
	dirInApex := filepath.Join("etc", prebuilt.SubDir())
	fileToCopy := prebuilt.OutputFile()
	return newApexFile(ctx, fileToCopy, depName, dirInApex, etc, prebuilt)
}

func apexFileForCompatConfig(ctx android.BaseModuleContext, config java.PlatformCompatConfigIntf, depName string) apexFile {
	dirInApex := filepath.Join("etc", config.SubDir())
	fileToCopy := config.CompatConfig()
	return newApexFile(ctx, fileToCopy, depName, dirInApex, etc, config)
}

func apexFileForAndroidApp(ctx android.BaseModuleContext, aapp interface {
	android.Module
	Privileged() bool
	InstallApkName() string
	OutputFile() android.Path
	JacocoReportClassesFile() android.Path
	Certificate() java.Certificate
}) apexFile {
	appDir := "app"
	if aapp.Privileged() {
		appDir = "priv-app"
	}
	dirInApex := filepath.Join(appDir, aapp.InstallApkName())
	fileToCopy := aapp.OutputFile()
	af := newApexFile(ctx, fileToCopy, aapp.Name(), dirInApex, app, aapp)
	af.jacocoReportClassesFile = aapp.JacocoReportClassesFile()
	af.certificate = aapp.Certificate()

	if app, ok := aapp.(interface {
		OverriddenManifestPackageName() string
	}); ok {
		af.overriddenPackageName = app.OverriddenManifestPackageName()
	}
	return af
}

// Context "decorator", overriding the InstallBypassMake method to always reply `true`.
type flattenedApexContext struct {
	android.ModuleContext
}

func (c *flattenedApexContext) InstallBypassMake() bool {
	return true
}

// Function called while walking an APEX's payload dependencies.
//
// Return true if the `to` module should be visited, false otherwise.
type payloadDepsCallback func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool) bool

// Visit dependencies that contributes to the payload of this APEX
func (a *apexBundle) walkPayloadDeps(ctx android.ModuleContext, do payloadDepsCallback) {
	ctx.WalkDeps(func(child, parent android.Module) bool {
		am, ok := child.(android.ApexModule)
		if !ok || !am.CanHaveApexVariants() {
			return false
		}

		// Check for the direct dependencies that contribute to the payload
		if dt, ok := ctx.OtherModuleDependencyTag(child).(dependencyTag); ok {
			if dt.payload {
				return do(ctx, parent, am, false /* externalDep */)
			}
			// As soon as the dependency graph crosses the APEX boundary, don't go further.
			return false
		}

		// Check for the indirect dependencies if it is considered as part of the APEX
		if am.ApexName() != "" {
			return do(ctx, parent, am, false /* externalDep */)
		}

		return do(ctx, parent, am, true /* externalDep */)
	})
}

func (a *apexBundle) minSdkVersion(ctx android.BaseModuleContext) int {
	ver := proptools.StringDefault(a.properties.Min_sdk_version, "current")
	intVer, err := android.ApiStrToNum(ctx, ver)
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "%s", err.Error())
	}
	return intVer
}

// A regexp for removing boilerplate from BaseDependencyTag from the string representation of
// a dependency tag.
var tagCleaner = regexp.MustCompile(`\QBaseDependencyTag:blueprint.BaseDependencyTag{}\E(, )?`)

func PrettyPrintTag(tag blueprint.DependencyTag) string {
	// Use tag's custom String() method if available.
	if stringer, ok := tag.(fmt.Stringer); ok {
		return stringer.String()
	}

	// Otherwise, get a default string representation of the tag's struct.
	tagString := fmt.Sprintf("%#v", tag)

	// Remove the boilerplate from BaseDependencyTag as it adds no value.
	tagString = tagCleaner.ReplaceAllString(tagString, "")
	return tagString
}

func (a *apexBundle) Updatable() bool {
	return proptools.Bool(a.properties.Updatable)
}

var _ android.ApexBundleDepsInfoIntf = (*apexBundle)(nil)

// Ensures that the dependencies are marked as available for this APEX
func (a *apexBundle) checkApexAvailability(ctx android.ModuleContext) {
	// Let's be practical. Availability for test, host, and the VNDK apex isn't important
	if ctx.Host() || a.testApex || a.vndkApex {
		return
	}

	// Coverage build adds additional dependencies for the coverage-only runtime libraries.
	// Requiring them and their transitive depencies with apex_available is not right
	// because they just add noise.
	if ctx.Config().IsEnvTrue("EMMA_INSTRUMENT") || a.IsNativeCoverageNeeded(ctx) {
		return
	}

	a.walkPayloadDeps(ctx, func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool) bool {
		if externalDep {
			// As soon as the dependency graph crosses the APEX boundary, don't go further.
			return false
		}

		apexName := ctx.ModuleName()
		fromName := ctx.OtherModuleName(from)
		toName := ctx.OtherModuleName(to)

		// If `to` is not actually in the same APEX as `from` then it does not need apex_available and neither
		// do any of its dependencies.
		if am, ok := from.(android.DepIsInSameApex); ok && !am.DepIsInSameApex(ctx, to) {
			// As soon as the dependency graph crosses the APEX boundary, don't go further.
			return false
		}

		if to.AvailableFor(apexName) || baselineApexAvailable(apexName, toName) {
			return true
		}
		message := ""
		tagPath := ctx.GetTagPath()
		// Skip the first module as that will be added at the start of the error message by ctx.ModuleErrorf().
		walkPath := ctx.GetWalkPath()[1:]
		for i, m := range walkPath {
			message = fmt.Sprintf("%s\n           via tag %s\n    -> %s", message, PrettyPrintTag(tagPath[i]), m.String())
		}
		ctx.ModuleErrorf("%q requires %q that is not available for the APEX. Dependency path:%s", fromName, toName, message)
		// Visit this module's dependencies to check and report any issues with their availability.
		return true
	})
}

func (a *apexBundle) checkUpdatable(ctx android.ModuleContext) {
	if a.Updatable() {
		if String(a.properties.Min_sdk_version) == "" {
			ctx.PropertyErrorf("updatable", "updatable APEXes should set min_sdk_version as well")
		}

		a.checkJavaStableSdkVersion(ctx)
	}
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

	a.checkApexAvailability(ctx)
	a.checkUpdatable(ctx)

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
	// TODO(jiyong) do this using walkPayloadDeps
	ctx.WalkDepsBlueprint(func(child, parent blueprint.Module) bool {
		depTag := ctx.OtherModuleDependencyTag(child)
		if _, ok := depTag.(android.ExcludeFromApexContentsTag); ok {
			return false
		}
		depName := ctx.OtherModuleName(child)
		if _, isDirectDep := parent.(*apexBundle); isDirectDep {
			switch depTag {
			case sharedLibTag:
				if c, ok := child.(*cc.Module); ok {
					fi := apexFileForNativeLibrary(ctx, c, handleSpecialLibs)
					filesInfo = append(filesInfo, fi)
					// bootstrap bionic libs are treated as provided by system
					if c.HasStubsVariants() && !cc.InstallToBootstrap(c.BaseModuleName(), ctx.Config()) {
						provideNativeLibs = append(provideNativeLibs, fi.Stem())
					}
					return true // track transitive dependencies
				} else {
					ctx.PropertyErrorf("native_shared_libs", "%q is not a cc_library or cc_library_shared module", depName)
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
				} else {
					ctx.PropertyErrorf("binaries", "%q is neither cc_binary, (embedded) py_binary, (host) blueprint_go_binary, (host) bootstrap_go_binary, nor sh_binary", depName)
				}
			case javaLibTag:
				switch child.(type) {
				case *java.Library, *java.SdkLibrary, *java.DexImport, *java.SdkLibraryImport:
					af := apexFileForJavaLibrary(ctx, child.(javaDependency), child.(android.Module))
					if !af.Ok() {
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
					af := newApexFile(ctx, ap.OutputFile(), ap.Name(),
						filepath.Join(appDir, ap.BaseModuleName()), appSet, ap)
					af.certificate = java.PresignedCertificate
					filesInfo = append(filesInfo, af)
				} else {
					ctx.PropertyErrorf("apps", "%q is not an android_app module", depName)
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
						if android.InList(cc.Name(), providedNativeSharedLibs) {
							// If we're using a shared library which is provided from other APEX,
							// don't include it in this APEX
							return false
						}
						af := apexFileForNativeLibrary(ctx, cc, handleSpecialLibs)
						af.transitiveDep = true
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
							requireNativeLibs = append(requireNativeLibs, af.Stem())
							// Don't track further
							return false
						}
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
						af.moduleName = filepath.Base(af.builtFile.String())
						// these are not considered transitive dep
						af.transitiveDep = false
						filesInfo = append(filesInfo, af)
						return true // track transitive dependencies
					}
				} else if java.IsJniDepTag(depTag) {
					// Because APK-in-APEX embeds jni_libs transitively, we don't need to track transitive deps
					return false
				} else if java.IsXmlPermissionsFileDepTag(depTag) {
					if prebuilt, ok := child.(prebuilt_etc.PrebuiltEtcModule); ok {
						filesInfo = append(filesInfo, apexFileForPrebuiltEtc(ctx, prebuilt, depName))
					}
				} else if am.CanHaveApexVariants() && am.IsInstallableToApex() {
					ctx.ModuleErrorf("unexpected tag %s for indirect dependency %q", PrettyPrintTag(depTag), depName)
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

	// We don't need the optimization for updatable APEXes, as it might give false signal
	// to the system health when the APEXes are still bundled (b/149805758)
	if a.Updatable() && a.properties.ApexType == imageApex {
		a.linkToSystemLib = false
	}

	// We also don't want the optimization for host APEXes, because it doesn't make sense.
	if ctx.Host() {
		a.linkToSystemLib = false
	}

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

	a.buildLintReports(ctx)
}

// Enforce that Java deps of the apex are using stable SDKs to compile
func (a *apexBundle) checkJavaStableSdkVersion(ctx android.ModuleContext) {
	// Visit direct deps only. As long as we guarantee top-level deps are using
	// stable SDKs, java's checkLinkType guarantees correct usage for transitive deps
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
	moduleName = strings.TrimPrefix(moduleName, "prebuilt_")
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
