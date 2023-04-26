// Copyright 2022 Google Inc. All rights reserved.
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

package allowlists

// Configuration to decide if modules in a directory should default to true/false for bp2build_available
type Bp2BuildConfig map[string]BazelConversionConfigEntry
type BazelConversionConfigEntry int

const (
	// iota + 1 ensures that the int value is not 0 when used in the Bp2buildAllowlist map,
	// which can also mean that the key doesn't exist in a lookup.

	// all modules in this package and subpackages default to bp2build_available: true.
	// allows modules to opt-out.
	Bp2BuildDefaultTrueRecursively BazelConversionConfigEntry = iota + 1

	// all modules in this package (not recursively) default to bp2build_available: true.
	// allows modules to opt-out.
	Bp2BuildDefaultTrue

	// all modules in this package (not recursively) default to bp2build_available: false.
	// allows modules to opt-in.
	Bp2BuildDefaultFalse

	// all modules in this package and subpackages default to bp2build_available: false.
	// allows modules to opt-in.
	Bp2BuildDefaultFalseRecursively

	DEFAULT_NINJA_WEIGHT = 1000
)

var (
	Bp2buildDefaultConfig = Bp2BuildConfig{
		"art":                                   Bp2BuildDefaultTrue,
		"art/libartbase":                        Bp2BuildDefaultTrueRecursively,
		"art/libartpalette":                     Bp2BuildDefaultTrueRecursively,
		"art/libdexfile":                        Bp2BuildDefaultTrueRecursively,
		"art/libnativebridge":                   Bp2BuildDefaultTrueRecursively,
		"art/runtime":                           Bp2BuildDefaultTrueRecursively,
		"art/tools":                             Bp2BuildDefaultTrue,
		"bionic":                                Bp2BuildDefaultTrueRecursively,
		"bootable/recovery/applypatch":          Bp2BuildDefaultTrue,
		"bootable/recovery/minadbd":             Bp2BuildDefaultTrue,
		"bootable/recovery/minui":               Bp2BuildDefaultTrue,
		"bootable/recovery/recovery_utils":      Bp2BuildDefaultTrue,
		"bootable/recovery/tools/recovery_l10n": Bp2BuildDefaultTrue,

		"build/bazel":                        Bp2BuildDefaultTrueRecursively,
		"build/make/target/product/security": Bp2BuildDefaultTrue,
		"build/make/tools/protos":            Bp2BuildDefaultTrue,
		"build/make/tools/releasetools":      Bp2BuildDefaultTrue,
		"build/make/tools/sbom":              Bp2BuildDefaultTrue,
		"build/make/tools/signapk":           Bp2BuildDefaultTrue,
		"build/make/tools/zipalign":          Bp2BuildDefaultTrueRecursively,
		"build/soong":                        Bp2BuildDefaultTrue,
		"build/soong/cc/libbuildversion":     Bp2BuildDefaultTrue, // Skip tests subdir
		"build/soong/cc/ndkstubgen":          Bp2BuildDefaultTrue,
		"build/soong/cc/symbolfile":          Bp2BuildDefaultTrue,
		"build/soong/licenses":               Bp2BuildDefaultTrue,
		"build/soong/linkerconfig":           Bp2BuildDefaultTrueRecursively,
		"build/soong/scripts":                Bp2BuildDefaultTrueRecursively,

		"cts/common/device-side/nativetesthelper/jni": Bp2BuildDefaultTrueRecursively,

		"dalvik/tools/dexdeps": Bp2BuildDefaultTrueRecursively,

		"development/apps/DevelopmentSettings":        Bp2BuildDefaultTrue,
		"development/apps/Fallback":                   Bp2BuildDefaultTrue,
		"development/apps/WidgetPreview":              Bp2BuildDefaultTrue,
		"development/samples/BasicGLSurfaceView":      Bp2BuildDefaultTrue,
		"development/samples/BluetoothChat":           Bp2BuildDefaultTrue,
		"development/samples/BrokenKeyDerivation":     Bp2BuildDefaultTrue,
		"development/samples/Compass":                 Bp2BuildDefaultTrue,
		"development/samples/ContactManager":          Bp2BuildDefaultTrue,
		"development/samples/FixedGridLayout":         Bp2BuildDefaultTrue,
		"development/samples/HelloEffects":            Bp2BuildDefaultTrue,
		"development/samples/Home":                    Bp2BuildDefaultTrue,
		"development/samples/HoneycombGallery":        Bp2BuildDefaultTrue,
		"development/samples/JetBoy":                  Bp2BuildDefaultTrue,
		"development/samples/KeyChainDemo":            Bp2BuildDefaultTrue,
		"development/samples/LceDemo":                 Bp2BuildDefaultTrue,
		"development/samples/LunarLander":             Bp2BuildDefaultTrue,
		"development/samples/MultiResolution":         Bp2BuildDefaultTrue,
		"development/samples/MultiWindow":             Bp2BuildDefaultTrue,
		"development/samples/NotePad":                 Bp2BuildDefaultTrue,
		"development/samples/Obb":                     Bp2BuildDefaultTrue,
		"development/samples/RSSReader":               Bp2BuildDefaultTrue,
		"development/samples/ReceiveShareDemo":        Bp2BuildDefaultTrue,
		"development/samples/SearchableDictionary":    Bp2BuildDefaultTrue,
		"development/samples/SipDemo":                 Bp2BuildDefaultTrue,
		"development/samples/SkeletonApp":             Bp2BuildDefaultTrue,
		"development/samples/Snake":                   Bp2BuildDefaultTrue,
		"development/samples/SpellChecker/":           Bp2BuildDefaultTrueRecursively,
		"development/samples/ThemedNavBarKeyboard":    Bp2BuildDefaultTrue,
		"development/samples/ToyVpn":                  Bp2BuildDefaultTrue,
		"development/samples/TtsEngine":               Bp2BuildDefaultTrue,
		"development/samples/USB/AdbTest":             Bp2BuildDefaultTrue,
		"development/samples/USB/MissileLauncher":     Bp2BuildDefaultTrue,
		"development/samples/VoiceRecognitionService": Bp2BuildDefaultTrue,
		"development/samples/VoicemailProviderDemo":   Bp2BuildDefaultTrue,
		"development/samples/WiFiDirectDemo":          Bp2BuildDefaultTrue,
		"development/sdk":                             Bp2BuildDefaultTrueRecursively,

		"external/aac":                           Bp2BuildDefaultTrueRecursively,
		"external/arm-optimized-routines":        Bp2BuildDefaultTrueRecursively,
		"external/auto":                          Bp2BuildDefaultTrue,
		"external/auto/android-annotation-stubs": Bp2BuildDefaultTrueRecursively,
		"external/auto/common":                   Bp2BuildDefaultTrueRecursively,
		"external/auto/service":                  Bp2BuildDefaultTrueRecursively,
		"external/boringssl":                     Bp2BuildDefaultTrueRecursively,
		"external/bouncycastle":                  Bp2BuildDefaultTrue,
		"external/brotli":                        Bp2BuildDefaultTrue,
		"external/bsdiff":                        Bp2BuildDefaultTrueRecursively,
		"external/bzip2":                         Bp2BuildDefaultTrueRecursively,
		"external/conscrypt":                     Bp2BuildDefaultTrue,
		"external/e2fsprogs":                     Bp2BuildDefaultTrueRecursively,
		"external/eigen":                         Bp2BuildDefaultTrueRecursively,
		"external/erofs-utils":                   Bp2BuildDefaultTrueRecursively,
		"external/error_prone":                   Bp2BuildDefaultTrueRecursively,
		"external/escapevelocity":                Bp2BuildDefaultTrueRecursively,
		"external/expat":                         Bp2BuildDefaultTrueRecursively,
		"external/f2fs-tools":                    Bp2BuildDefaultTrue,
		"external/flac":                          Bp2BuildDefaultTrueRecursively,
		"external/fmtlib":                        Bp2BuildDefaultTrueRecursively,
		"external/guava":                         Bp2BuildDefaultTrueRecursively,
		"external/google-benchmark":              Bp2BuildDefaultTrueRecursively,
		"external/googletest":                    Bp2BuildDefaultTrueRecursively,
		"external/gwp_asan":                      Bp2BuildDefaultTrueRecursively,
		"external/hamcrest":                      Bp2BuildDefaultTrueRecursively,
		"external/icu":                           Bp2BuildDefaultTrueRecursively,
		"external/icu/android_icu4j":             Bp2BuildDefaultFalse, // java rules incomplete
		"external/icu/icu4j":                     Bp2BuildDefaultFalse, // java rules incomplete
		"external/jacoco":                        Bp2BuildDefaultTrueRecursively,
		"external/jarjar":                        Bp2BuildDefaultTrueRecursively,
		"external/javaparser":                    Bp2BuildDefaultTrueRecursively,
		"external/javapoet":                      Bp2BuildDefaultTrueRecursively,
		"external/javassist":                     Bp2BuildDefaultTrueRecursively,
		"external/jemalloc_new":                  Bp2BuildDefaultTrueRecursively,
		"external/jsoncpp":                       Bp2BuildDefaultTrueRecursively,
		"external/jsr305":                        Bp2BuildDefaultTrueRecursively,
		"external/jsr330":                        Bp2BuildDefaultTrueRecursively,
		"external/junit":                         Bp2BuildDefaultTrueRecursively,
		"external/kotlinc":                       Bp2BuildDefaultTrueRecursively,
		"external/libaom":                        Bp2BuildDefaultTrueRecursively,
		"external/libavc":                        Bp2BuildDefaultTrueRecursively,
		"external/libcap":                        Bp2BuildDefaultTrueRecursively,
		"external/libcxx":                        Bp2BuildDefaultTrueRecursively,
		"external/libcxxabi":                     Bp2BuildDefaultTrueRecursively,
		"external/libdivsufsort":                 Bp2BuildDefaultTrueRecursively,
		"external/libdrm":                        Bp2BuildDefaultTrue,
		"external/libevent":                      Bp2BuildDefaultTrueRecursively,
		"external/libgav1":                       Bp2BuildDefaultTrueRecursively,
		"external/libhevc":                       Bp2BuildDefaultTrueRecursively,
		"external/libjpeg-turbo":                 Bp2BuildDefaultTrueRecursively,
		"external/libmpeg2":                      Bp2BuildDefaultTrueRecursively,
		"external/libpng":                        Bp2BuildDefaultTrueRecursively,
		"external/libvpx":                        Bp2BuildDefaultTrueRecursively,
		"external/libyuv":                        Bp2BuildDefaultTrueRecursively,
		"external/lz4/lib":                       Bp2BuildDefaultTrue,
		"external/lz4/programs":                  Bp2BuildDefaultTrue,
		"external/lzma/C":                        Bp2BuildDefaultTrueRecursively,
		"external/mdnsresponder":                 Bp2BuildDefaultTrueRecursively,
		"external/minijail":                      Bp2BuildDefaultTrueRecursively,
		"external/musl":                          Bp2BuildDefaultTrueRecursively,
		"external/objenesis":                     Bp2BuildDefaultTrueRecursively,
		"external/openscreen":                    Bp2BuildDefaultTrueRecursively,
		"external/ow2-asm":                       Bp2BuildDefaultTrueRecursively,
		"external/pcre":                          Bp2BuildDefaultTrueRecursively,
		"external/protobuf":                      Bp2BuildDefaultTrueRecursively,
		"external/python/six":                    Bp2BuildDefaultTrueRecursively,
		"external/rappor":                        Bp2BuildDefaultTrueRecursively,
		"external/scudo":                         Bp2BuildDefaultTrueRecursively,
		"external/selinux/libselinux":            Bp2BuildDefaultTrueRecursively,
		"external/selinux/libsepol":              Bp2BuildDefaultTrueRecursively,
		"external/speex":                         Bp2BuildDefaultTrueRecursively,
		"external/tinyalsa":                      Bp2BuildDefaultTrueRecursively,
		"external/tinyalsa_new":                  Bp2BuildDefaultTrueRecursively,
		"external/toybox":                        Bp2BuildDefaultTrueRecursively,
		"external/zlib":                          Bp2BuildDefaultTrueRecursively,
		"external/zopfli":                        Bp2BuildDefaultTrueRecursively,
		"external/zstd":                          Bp2BuildDefaultTrueRecursively,

		"frameworks/av": Bp2BuildDefaultTrue,
		"frameworks/av/media/codec2/components/aom":          Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/codecs":                         Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/liberror":                       Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/libshmem":                       Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/audioaidlconversion":            Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/module/minijail":                Bp2BuildDefaultTrueRecursively,
		"frameworks/av/services/minijail":                    Bp2BuildDefaultTrueRecursively,
		"frameworks/base/libs/androidfw":                     Bp2BuildDefaultTrue,
		"frameworks/base/media/tests/MediaDump":              Bp2BuildDefaultTrue,
		"frameworks/base/services/tests/servicestests/aidl":  Bp2BuildDefaultTrue,
		"frameworks/base/proto":                              Bp2BuildDefaultTrue,
		"frameworks/base/startop/apps/test":                  Bp2BuildDefaultTrue,
		"frameworks/base/tests/appwidgets/AppWidgetHostTest": Bp2BuildDefaultTrueRecursively,
		"frameworks/base/tools/aapt2":                        Bp2BuildDefaultTrue,
		"frameworks/base/tools/codegen":                      Bp2BuildDefaultTrueRecursively,
		"frameworks/base/tools/streaming_proto":              Bp2BuildDefaultTrueRecursively,
		"frameworks/hardware/interfaces/stats/aidl":          Bp2BuildDefaultTrue,
		"frameworks/native/libs/adbd_auth":                   Bp2BuildDefaultTrueRecursively,
		"frameworks/native/libs/arect":                       Bp2BuildDefaultTrueRecursively,
		"frameworks/native/libs/gui":                         Bp2BuildDefaultTrue,
		"frameworks/native/libs/math":                        Bp2BuildDefaultTrueRecursively,
		"frameworks/native/libs/nativebase":                  Bp2BuildDefaultTrueRecursively,
		"frameworks/native/libs/vr":                          Bp2BuildDefaultTrueRecursively,
		"frameworks/native/opengl/tests/gl2_cameraeye":       Bp2BuildDefaultTrue,
		"frameworks/native/opengl/tests/gl2_java":            Bp2BuildDefaultTrue,
		"frameworks/native/opengl/tests/testLatency":         Bp2BuildDefaultTrue,
		"frameworks/native/opengl/tests/testPauseResume":     Bp2BuildDefaultTrue,
		"frameworks/native/opengl/tests/testViewport":        Bp2BuildDefaultTrue,
		"frameworks/native/libs/permission":                  Bp2BuildDefaultTrue,
		"frameworks/native/services/batteryservice":          Bp2BuildDefaultTrue,
		"frameworks/proto_logging/stats":                     Bp2BuildDefaultTrueRecursively,

		"hardware/interfaces":                                     Bp2BuildDefaultTrue,
		"hardware/interfaces/audio/aidl":                          Bp2BuildDefaultTrue,
		"hardware/interfaces/audio/aidl/common":                   Bp2BuildDefaultTrue,
		"hardware/interfaces/audio/aidl/default":                  Bp2BuildDefaultTrue,
		"hardware/interfaces/audio/aidl/sounddose":                Bp2BuildDefaultTrue,
		"hardware/interfaces/common/aidl":                         Bp2BuildDefaultTrue,
		"hardware/interfaces/common/fmq/aidl":                     Bp2BuildDefaultTrue,
		"hardware/interfaces/common/support":                      Bp2BuildDefaultTrue,
		"hardware/interfaces/configstore/1.0":                     Bp2BuildDefaultTrue,
		"hardware/interfaces/configstore/1.1":                     Bp2BuildDefaultTrue,
		"hardware/interfaces/configstore/utils":                   Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/allocator/2.0":              Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/allocator/3.0":              Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/allocator/4.0":              Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/allocator/aidl":             Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/bufferqueue/1.0":            Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/bufferqueue/2.0":            Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/common/1.0":                 Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/common/1.1":                 Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/common/1.2":                 Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/common/aidl":                Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/mapper/2.0":                 Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/mapper/2.1":                 Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/mapper/3.0":                 Bp2BuildDefaultTrue,
		"hardware/interfaces/graphics/mapper/4.0":                 Bp2BuildDefaultTrue,
		"hardware/interfaces/health/1.0":                          Bp2BuildDefaultTrue,
		"hardware/interfaces/health/1.0/default":                  Bp2BuildDefaultTrue,
		"hardware/interfaces/health/2.0":                          Bp2BuildDefaultTrue,
		"hardware/interfaces/health/2.0/default":                  Bp2BuildDefaultTrue,
		"hardware/interfaces/health/2.0/utils":                    Bp2BuildDefaultTrueRecursively,
		"hardware/interfaces/health/2.1":                          Bp2BuildDefaultTrue,
		"hardware/interfaces/health/aidl":                         Bp2BuildDefaultTrue,
		"hardware/interfaces/health/utils":                        Bp2BuildDefaultTrueRecursively,
		"hardware/interfaces/media/1.0":                           Bp2BuildDefaultTrue,
		"hardware/interfaces/media/bufferpool":                    Bp2BuildDefaultTrueRecursively,
		"hardware/interfaces/media/bufferpool/aidl/default/tests": Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/media/c2/1.0":                        Bp2BuildDefaultTrue,
		"hardware/interfaces/media/c2/1.1":                        Bp2BuildDefaultTrue,
		"hardware/interfaces/media/c2/1.2":                        Bp2BuildDefaultTrue,
		"hardware/interfaces/media/omx/1.0":                       Bp2BuildDefaultTrue,
		"hardware/interfaces/neuralnetworks":                      Bp2BuildDefaultTrueRecursively,
		"hardware/interfaces/neuralnetworks/aidl/vts":             Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.0/vts":              Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.1/vts":              Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.2/vts":              Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.3/vts":              Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.4/vts":              Bp2BuildDefaultFalseRecursively,

		"libnativehelper": Bp2BuildDefaultTrueRecursively,

		"packages/apps/DevCamera":                            Bp2BuildDefaultTrue,
		"packages/apps/HTMLViewer":                           Bp2BuildDefaultTrue,
		"packages/apps/Protips":                              Bp2BuildDefaultTrue,
		"packages/apps/SafetyRegulatoryInfo":                 Bp2BuildDefaultTrue,
		"packages/apps/WallpaperPicker":                      Bp2BuildDefaultTrue,
		"packages/modules/NeuralNetworks/driver/cache":       Bp2BuildDefaultTrueRecursively,
		"packages/modules/StatsD/lib/libstatssocket":         Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb":                               Bp2BuildDefaultTrue,
		"packages/modules/adb/apex":                          Bp2BuildDefaultTrue,
		"packages/modules/adb/crypto":                        Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/libs":                          Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/pairing_auth":                  Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/pairing_connection":            Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/proto":                         Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/tls":                           Bp2BuildDefaultTrueRecursively,
		"packages/modules/Gki/libkver":                       Bp2BuildDefaultTrue,
		"packages/modules/NetworkStack/common/captiveportal": Bp2BuildDefaultTrue,
		"packages/modules/NeuralNetworks/apex":               Bp2BuildDefaultTrue,
		"packages/providers/MediaProvider/tools/dialogs":     Bp2BuildDefaultFalse, // TODO(b/242834374)
		"packages/screensavers/Basic":                        Bp2BuildDefaultTrue,
		"packages/services/Car/tests/SampleRearViewCamera":   Bp2BuildDefaultFalse, // TODO(b/242834321)

		"platform_testing/tests/example": Bp2BuildDefaultTrueRecursively,

		"prebuilts/clang/host/linux-x86":                   Bp2BuildDefaultTrueRecursively,
		"prebuilts/gradle-plugin":                          Bp2BuildDefaultTrueRecursively,
		"prebuilts/runtime/mainline/platform/sdk":          Bp2BuildDefaultTrueRecursively,
		"prebuilts/sdk/current/androidx":                   Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/androidx-legacy":            Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/extras/constraint-layout-x": Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/extras/material-design-x":   Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/extras/app-toolkit":         Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/support":                    Bp2BuildDefaultTrue,
		"prebuilts/tools":                                  Bp2BuildDefaultTrue,
		"prebuilts/tools/common/m2":                        Bp2BuildDefaultTrue,

		"sdk/dumpeventlog":  Bp2BuildDefaultTrue,
		"sdk/eventanalyzer": Bp2BuildDefaultTrue,

		"system/apex":                                            Bp2BuildDefaultFalse, // TODO(b/207466993): flaky failures
		"system/apex/apexer":                                     Bp2BuildDefaultTrue,
		"system/apex/libs":                                       Bp2BuildDefaultTrueRecursively,
		"system/apex/proto":                                      Bp2BuildDefaultTrueRecursively,
		"system/apex/tools":                                      Bp2BuildDefaultTrueRecursively,
		"system/core/debuggerd":                                  Bp2BuildDefaultTrueRecursively,
		"system/core/diagnose_usb":                               Bp2BuildDefaultTrueRecursively,
		"system/core/healthd":                                    Bp2BuildDefaultTrue,
		"system/core/healthd/testdata":                           Bp2BuildDefaultTrue,
		"system/core/libasyncio":                                 Bp2BuildDefaultTrue,
		"system/core/libcrypto_utils":                            Bp2BuildDefaultTrueRecursively,
		"system/core/libcutils":                                  Bp2BuildDefaultTrueRecursively,
		"system/core/libpackagelistparser":                       Bp2BuildDefaultTrueRecursively,
		"system/core/libprocessgroup":                            Bp2BuildDefaultTrue,
		"system/core/libprocessgroup/cgrouprc":                   Bp2BuildDefaultTrue,
		"system/core/libprocessgroup/cgrouprc_format":            Bp2BuildDefaultTrue,
		"system/core/libsuspend":                                 Bp2BuildDefaultTrue,
		"system/core/libsystem":                                  Bp2BuildDefaultTrueRecursively,
		"system/core/libsysutils":                                Bp2BuildDefaultTrueRecursively,
		"system/core/libutils":                                   Bp2BuildDefaultTrueRecursively,
		"system/core/libvndksupport":                             Bp2BuildDefaultTrueRecursively,
		"system/core/mkbootfs":                                   Bp2BuildDefaultTrueRecursively,
		"system/core/property_service/libpropertyinfoparser":     Bp2BuildDefaultTrueRecursively,
		"system/core/property_service/libpropertyinfoserializer": Bp2BuildDefaultTrueRecursively,
		"system/extras/toolchain-extras":                         Bp2BuildDefaultTrue,
		"system/hardware/interfaces/media":                       Bp2BuildDefaultTrueRecursively,
		"system/incremental_delivery/incfs":                      Bp2BuildDefaultTrue,
		"system/libartpalette":                                   Bp2BuildDefaultTrueRecursively,
		"system/libbase":                                         Bp2BuildDefaultTrueRecursively,
		"system/libfmq":                                          Bp2BuildDefaultTrue,
		"system/libhidl/libhidlmemory":                           Bp2BuildDefaultTrue,
		"system/libhidl/transport":                               Bp2BuildDefaultTrue,
		"system/libhidl/transport/allocator/1.0":                 Bp2BuildDefaultTrue,
		"system/libhidl/transport/base/1.0":                      Bp2BuildDefaultTrue,
		"system/libhidl/transport/manager/1.0":                   Bp2BuildDefaultTrue,
		"system/libhidl/transport/manager/1.1":                   Bp2BuildDefaultTrue,
		"system/libhidl/transport/manager/1.2":                   Bp2BuildDefaultTrue,
		"system/libhidl/transport/memory/1.0":                    Bp2BuildDefaultTrue,
		"system/libhidl/transport/memory/token/1.0":              Bp2BuildDefaultTrue,
		"system/libhidl/transport/safe_union/1.0":                Bp2BuildDefaultTrue,
		"system/libhidl/transport/token/1.0":                     Bp2BuildDefaultTrue,
		"system/libhidl/transport/token/1.0/utils":               Bp2BuildDefaultTrue,
		"system/libhwbinder":                                     Bp2BuildDefaultTrueRecursively,
		"system/libprocinfo":                                     Bp2BuildDefaultTrue,
		"system/libvintf":                                        Bp2BuildDefaultTrue,
		"system/libziparchive":                                   Bp2BuildDefaultTrueRecursively,
		"system/logging":                                         Bp2BuildDefaultTrueRecursively,
		"system/media":                                           Bp2BuildDefaultTrue,
		"system/media/audio":                                     Bp2BuildDefaultTrueRecursively,
		"system/media/alsa_utils":                                Bp2BuildDefaultTrueRecursively,
		"system/media/audio_utils":                               Bp2BuildDefaultTrueRecursively,
		"system/memory/libion":                                   Bp2BuildDefaultTrueRecursively,
		"system/memory/libmemunreachable":                        Bp2BuildDefaultTrueRecursively,
		"system/sepolicy/apex":                                   Bp2BuildDefaultTrueRecursively,
		"system/testing/gtest_extras":                            Bp2BuildDefaultTrueRecursively,
		"system/timezone/apex":                                   Bp2BuildDefaultTrueRecursively,
		"system/timezone/output_data":                            Bp2BuildDefaultTrueRecursively,
		"system/timezone/testdata":                               Bp2BuildDefaultTrueRecursively,
		"system/timezone/testing":                                Bp2BuildDefaultTrueRecursively,
		"system/tools/aidl/build/tests_bp2build":                 Bp2BuildDefaultTrue,
		"system/tools/aidl/metadata":                             Bp2BuildDefaultTrue,
		"system/tools/hidl/metadata":                             Bp2BuildDefaultTrue,
		"system/tools/mkbootimg":                                 Bp2BuildDefaultTrueRecursively,
		"system/tools/sysprop":                                   Bp2BuildDefaultTrue,
		"system/tools/xsdc/utils":                                Bp2BuildDefaultTrueRecursively,
		"system/unwinding/libunwindstack":                        Bp2BuildDefaultTrueRecursively,

		"tools/apifinder":                            Bp2BuildDefaultTrue,
		"tools/apksig":                               Bp2BuildDefaultTrue,
		"tools/external_updater":                     Bp2BuildDefaultTrueRecursively,
		"tools/metalava":                             Bp2BuildDefaultTrue,
		"tools/platform-compat/java/android/compat":  Bp2BuildDefaultTrueRecursively,
		"tools/tradefederation/prebuilts/filegroups": Bp2BuildDefaultTrueRecursively,
	}

	Bp2buildKeepExistingBuildFile = map[string]bool{
		// This is actually build/bazel/build.BAZEL symlinked to ./BUILD
		".":/*recursive = */ false,

		"build/bazel":/* recursive = */ true,
		"build/make/core":/* recursive = */ false,
		"build/bazel_common_rules":/* recursive = */ true,
		"build/make/target/product/security":/* recursive = */ false,
		// build/make/tools/signapk BUILD file is generated, so build/make/tools is not recursive.
		"build/make/tools":/* recursive = */ false,
		"build/pesto":/* recursive = */ true,
		"build/soong":/* recursive = */ true,

		// external/bazelbuild-rules_android/... is needed by mixed builds, otherwise mixed builds analysis fails
		// e.g. ERROR: Analysis of target '@soong_injection//mixed_builds:buildroot' failed
		"external/bazelbuild-rules_android":/* recursive = */ true,
		"external/bazelbuild-rules_license":/* recursive = */ true,
		"external/bazelbuild-kotlin-rules":/* recursive = */ true,
		"external/bazel-skylib":/* recursive = */ true,
		"external/protobuf":/* recursive = */ false,
		"external/python/absl-py":/* recursive = */ true,

		"external/compiler-rt/lib/cfi":/* recursive = */ false,

		// this BUILD file is globbed by //external/icu/icu4c/source:icu4c_test_data's "data/**/*".
		"external/icu/icu4c/source/data/unidata/norm2":/* recursive = */ false,

		// Building manually due to b/179889880: resource files cross package boundary
		"packages/apps/Music":/* recursive = */ true,

		"prebuilts/abi-dumps/platform":/* recursive = */ true,
		"prebuilts/abi-dumps/ndk":/* recursive = */ true,
		"prebuilts/bazel":/* recursive = */ true,
		"prebuilts/bundletool":/* recursive = */ true,
		"prebuilts/clang/host/linux-x86":/* recursive = */ false,
		"prebuilts/clang-tools":/* recursive = */ true,
		"prebuilts/gcc":/* recursive = */ true,
		"prebuilts/build-tools":/* recursive = */ true,
		"prebuilts/jdk/jdk17":/* recursive = */ true,
		"prebuilts/misc":/* recursive = */ false, // not recursive because we need bp2build converted build files in prebuilts/misc/common/asm
		"prebuilts/sdk":/* recursive = */ false,
		"prebuilts/sdk/tools":/* recursive = */ false,
		"prebuilts/r8":/* recursive = */ false,
		"prebuilts/runtime":/* recursive = */ false,

		// not recursive due to conflicting workspace paths in tools/atest/bazel/rules
		"tools/asuite/atest":/* recursive = */ false,
		"tools/asuite/atest/bazel/reporter":/* recursive = */ true,

		// TODO(b/266459895): remove this and the placeholder BUILD file after re-enabling libunwindstack
		"external/rust/crates/rustc-demangle-capi":/* recursive = */ false,
	}

	Bp2buildModuleAlwaysConvertList = []string{
		"libidmap2_policies",
		"libSurfaceFlingerProp",
		// cc mainline modules
		"code_coverage.policy",
		"code_coverage.policy.other",
		"codec2_soft_exports",
		"codecs_g711dec",
		"com.android.media.swcodec",
		"com.android.media.swcodec-androidManifest",
		"com.android.media.swcodec-ld.config.txt",
		"com.android.media.swcodec-mediaswcodec.32rc",
		"com.android.media.swcodec-mediaswcodec.rc",
		"com.android.media.swcodec.certificate",
		"com.android.media.swcodec.key",
		"flatbuffer_headers",
		"framework-connectivity-protos",
		"gemmlowp_headers",
		"gl_headers",
		"libandroid_runtime_lazy",
		"libandroid_runtime_vm_headers",
		"libaudioclient_aidl_conversion_util",
		"libbinder",
		"libbinder_device_interface_sources",
		"libbinder_aidl",
		"libbinder_headers",
		"libbinder_headers_platform_shared",
		"libbinderthreadstateutils",
		"libbluetooth-types-header",
		"libcodec2",
		"libcodec2_headers",
		"libcodec2_internal",
		"libdmabufheap",
		"libgsm",
		"libgrallocusage",
		"libgralloctypes",
		"libnativewindow",
		"libneuralnetworks",
		"libneuralnetworks_static",
		"libgraphicsenv",
		"libhardware",
		"libhardware_headers",
		"libnativeloader-headers",
		"libnativewindow_headers",
		"libneuralnetworks_headers",
		"libneuralnetworks_packageinfo",
		"libopus",
		"libprocpartition",
		"libruy_static",
		"libandroidio",
		"libandroidio_srcs",
		"libserviceutils",
		"libstagefright_amrnbenc",
		"libstagefright_amrnbdec",
		"libstagefright_amrwbdec",
		"libstagefright_amrwbenc",
		"libstagefright_amrnb_common",
		"libstagefright_enc_common",
		"libstagefright_flacdec",
		"libstagefright_foundation",
		"libstagefright_foundation_headers",
		"libstagefright_headers",
		"libstagefright_m4vh263dec",
		"libstagefright_m4vh263enc",
		"libstagefright_mp3dec",
		"libstagefright_mp3dec_headers",
		"libsurfaceflinger_headers",
		"libsync",
		"libtextclassifier_hash_headers",
		"libtextclassifier_hash_static",
		"libtflite_kernel_utils",
		"libtinyxml2",
		"libui",
		"libui-types",
		"libui_headers",
		"libvorbisidec",
		"media_ndk_headers",
		"media_plugin_headers",
		"mediaswcodec.policy",
		"mediaswcodec.xml",
		"neuralnetworks_types",
		"libneuralnetworks_common",
		// packagemanager_aidl_interface is created implicitly in packagemanager_aidl module
		"packagemanager_aidl_interface",
		"philox_random",
		"philox_random_headers",
		"server_configurable_flags",
		"service-permission-streaming-proto-sources",
		"statslog_neuralnetworks.cpp",
		"statslog_neuralnetworks.h",
		"tensorflow_headers",

		"libstagefright_bufferpool@2.0",
		"libstagefright_bufferpool@2.0.1",
		"libSurfaceFlingerProp",

		// prebuilts
		"prebuilt_stats-log-api-gen",

		// fastboot
		"fastboot",
		"libfastboot",
		"liblp",
		"libstorage_literals_headers",

		//external/avb
		"avbtool",
		"libavb",
		"avb_headers",

		//external/libxml2
		"xmllint",
		"libxml2",

		//external/fec
		"libfec_rs",

		//system/core/libsparse
		"libsparse",

		//system/extras/ext4_utils
		"libext4_utils",
		"mke2fs_conf",

		//system/extras/libfec
		"libfec",

		//system/extras/squashfs_utils
		"libsquashfs_utils",

		//system/extras/verity/fec
		"fec",
		"boot_signer",

		//packages/apps/Car/libs/car-ui-lib/car-ui-androidx
		// genrule dependencies for java_imports
		"car-ui-androidx-annotation-nodeps",
		"car-ui-androidx-collection-nodeps",
		"car-ui-androidx-core-common-nodeps",
		"car-ui-androidx-lifecycle-common-nodeps",
		"car-ui-androidx-constraintlayout-solver-nodeps",

		//system/libhidl
		"libhidlbase", // needed by cc_hidl_library
		"libhidl_gtest_helper",

		//frameworks/native
		"framework_native_aidl_binder",
		"framework_native_aidl_gui",

		//frameworks/native/libs/input
		"inputconstants_aidl",

		// needed for aidl_interface's ndk backend
		"libbinder_ndk",

		"libusb",

		// needed by liblogd
		"ILogcatManagerService_aidl",
		"libincremental_aidl-cpp",
		"incremental_aidl",

		//frameworks/native/cmds/cmd
		"libcmd",

		//system/core/fs_mgr/libdm
		"libdm",

		//system/core/fs_mgr/libfiemap
		"libfiemap_headers",
		"libfiemap_passthrough_srcs",
		"libfiemap_srcs",

		//system/gsid
		"libgsi",
		"libgsi_headers",

		//system/core/libkeyutils
		"libkeyutils",

		//bootable/recovery/otautil
		"libotautil",

		//system/vold
		"libvold_headers",

		//system/extras/libfscrypt
		"libfscrypt",

		//system/core/fs_mgr
		"libfstab",

		//bootable/recovery/fuse_sideload
		"libfusesideload",

		//system/core/fs_mgr/libfs_avb
		"libfs_avb",

		//system/core/fs_mgr
		"libfs_mgr",

		"libcodec2_hidl@1.0",
		"libcodec2_hidl@1.1",
		"libcodec2_hidl@1.2",
		"libcodec2_hidl_plugin_stub",
		"libcodec2_hidl_plugin",
		"libstagefright_bufferqueue_helper_novndk",
		"libGLESv2",
		"libEGL",
		"libcodec2_vndk",
		"libnativeloader_lazy",
		"libnativeloader",
		"libEGL_getProcAddress",
		"libEGL_blobCache",

		"mediaswcodec",
		"libmedia_headers",
		"libmedia_codecserviceregistrant",
		"libsfplugin_ccodec_utils",
		"libcodec2_soft_aacenc",
		"libcodec2_soft_amrnbdec",
		"libcodec2_soft_amrnbenc",
		"libcodec2_soft_amrwbdec",
		"libcodec2_soft_amrwbenc",
		"libcodec2_soft_hevcdec",
		"libcodec2_soft_hevcenc",
		"libcodec2_soft_g711alawdec",
		"libcodec2_soft_g711mlawdec",
		"libcodec2_soft_mpeg2dec",
		"libcodec2_soft_h263dec",
		"libcodec2_soft_h263enc",
		"libcodec2_soft_mpeg4dec",
		"libcodec2_soft_mpeg4enc",
		"libcodec2_soft_mp3dec",
		"libcodec2_soft_vorbisdec",
		"libcodec2_soft_opusdec",
		"libcodec2_soft_opusenc",
		"libcodec2_soft_vp8dec",
		"libcodec2_soft_vp9dec",
		"libcodec2_soft_av1dec_gav1",
		"libcodec2_soft_vp8enc",
		"libcodec2_soft_vp9enc",
		"libcodec2_soft_rawdec",
		"libcodec2_soft_flacdec",
		"libcodec2_soft_flacenc",
		"libcodec2_soft_gsmdec",
		"libcodec2_soft_avcdec",
		"libcodec2_soft_avcenc",
		"libcodec2_soft_aacdec",
		"libcodec2_soft_common",

		// kotlin srcs in java libs
		"kotlinx_atomicfu",

		// kotlin srcs in java binary
		"AnalyzerKt",
		"trebuchet-core",

		// kotlin srcs in android_library
		"renderscript_toolkit",

		//kotlin srcs in android_binary
		"MusicKotlin",

		// java_library with prebuilt sdk_version
		"android-common",

		// checked in current.txt for merged_txts
		"non-updatable-current.txt",
		"non-updatable-system-current.txt",
		"non-updatable-module-lib-current.txt",
		"non-updatable-system-server-current.txt",

		// for api_fingerprint.txt generation
		"api_fingerprint",

		// allowlisting for kotlinx_coroutines
		"kotlinx_coroutines",
		"kotlinx_coroutines-device",
		"kotlinx_coroutines-host",
		"annotations",
		"kotlinx-coroutines-android-annotation-stubs",

		// for building com.android.neuralnetworks
		"libimapper_stablec",
		"libimapper_providerutils",

		// min_sdk_version in android_app
		"CtsShimUpgrade",

		// Mainline Module Apps
		"CaptivePortalLogin",
	}

	Bp2buildModuleTypeAlwaysConvertList = []string{
		"aidl_interface_headers",
		"bpf",
		"combined_apis",
		"license",
		"linker_config",
		"java_import",
		"java_import_host",
		"java_sdk_library",
		"sysprop_library",
	}

	// Add the names of modules that bp2build should never convert, if it is
	// in the package allowlist.  An error will be thrown if a module must
	// not be here and in the alwaysConvert lists.
	//
	// For prebuilt modules (e.g. android_library_import), remember to add
	// the "prebuilt_" prefix to the name, so that it's differentiable from
	// the source versions within Soong's module graph.
	Bp2buildModuleDoNotConvertList = []string{
		// Depends on unconverted libandroid, libgui
		"dvr_buffer_queue-test",
		"dvr_display-test",
		// Depends on unconverted libchrome
		"pdx_benchmarks",
		"buffer_hub_queue-test",
		"buffer_hub_queue_producer-test",

		// cc bugs
		"libactivitymanager_aidl", // TODO(b/207426160): Unsupported use of aidl sources (via Dactivity_manager_procstate_aidl) in a cc_library

		// TODO(b/198619163) module has same name as source
		"logtagd.rc",

		"libgtest_ndk_c++", "libgtest_main_ndk_c++", // TODO(b/201816222): Requires sdk_version support.

		// TODO(b/202876379): has arch-variant static_executable
		"linkerconfig",
		"mdnsd",
		"libcutils_test_static",
		"KernelLibcutilsTest",

		"linker",                 // TODO(b/228316882): cc_binary uses link_crt
		"versioner",              // TODO(b/228313961):  depends on prebuilt shared library libclang-cpp_host as a shared library, which does not supply expected providers for a shared library
		"art_libartbase_headers", // TODO(b/236268577): Header libraries do not support export_shared_libs_headers
		"apexer_test",            // Requires aapt2
		"apexer_test_host_tools",
		"host_apex_verifier",
		"tjbench", // TODO(b/240563612): Stem property

		// java bugs
		"libbase_ndk",  // TODO(b/186826477): fails to link libctscamera2_jni for device (required for CtsCameraTestCases)
		"bouncycastle", // TODO(b/274474005): Need support for custom system_modules.

		// python protos
		"libprotobuf-python", // Has a handcrafted alternative

		// genrule incompatibilities
		"brotli-fuzzer-corpus",                                       // TODO(b/202015218): outputs are in location incompatible with bazel genrule handling.
		"platform_tools_properties", "build_tools_source_properties", // TODO(b/203369847): multiple genrules in the same package creating the same file

		// aar support
		"prebuilt_car-ui-androidx-core-common", // TODO(b/224773339), genrule dependency creates an .aar, not a .jar
		// ERROR: The dependencies for the following 1 jar(s) are not complete.
		// 1.bazel-out/android_target-fastbuild/bin/prebuilts/tools/common/m2/_aar/robolectric-monitor-1.0.2-alpha1/classes_and_libs_merged.jar
		"prebuilt_robolectric-monitor-1.0.2-alpha1",

		// path property for filegroups
		"conscrypt",                        // TODO(b/210751803), we don't handle path property for filegroups
		"conscrypt-for-host",               // TODO(b/210751803), we don't handle path property for filegroups
		"host-libprotobuf-java-full",       // TODO(b/210751803), we don't handle path property for filegroups
		"libprotobuf-internal-python-srcs", // TODO(b/210751803), we don't handle path property for filegroups
		"libprotobuf-java-full",            // TODO(b/210751803), we don't handle path property for filegroups
		"libprotobuf-java-util-full",       // TODO(b/210751803), we don't handle path property for filegroups
		"auto_value_plugin_resources",      // TODO(b/210751803), we don't handle path property for filegroups

		// go deps:
		"analyze_bcpf",              // depends on bpmodify a blueprint_go_binary.
		"analyze_bcpf_test",         // depends on bpmodify a blueprint_go_binary.
		"host_bionic_linker_asm",    // depends on extract_linker, a go binary.
		"host_bionic_linker_script", // depends on extract_linker, a go binary.

		// in cmd attribute of genrule rule //system/timezone/output_data:robolectric_tzdata: label '//system/timezone/output_data:iana/tzdata' in $(location) expression is not a declared prerequisite of this rule
		"robolectric_tzdata",

		// rust support
		"libtombstoned_client_rust_bridge_code", "libtombstoned_client_wrapper", // rust conversions are not supported

		// unconverted deps
		"apexer_with_DCLA_preprocessing_test",                        // depends on unconverted modules: apexer_test_host_tools, com.android.example.apex
		"adb",                                                        // depends on unconverted modules: AdbWinApi, libandroidfw, libopenscreen-discovery, libopenscreen-platform-impl, libusb, bin2c_fastdeployagent, AdbWinUsbApi
		"android_icu4j_srcgen",                                       // depends on unconverted modules: currysrc
		"android_icu4j_srcgen_binary",                                // depends on unconverted modules: android_icu4j_srcgen, currysrc
		"apex_compression_test",                                      // depends on unconverted modules: soong_zip, com.android.example.apex
		"apex_manifest_proto_java",                                   // b/210751803, depends on libprotobuf-java-full
		"art-script",                                                 // depends on unconverted modules: dalvikvm, dex2oat
		"bin2c_fastdeployagent",                                      // depends on unconverted modules: deployagent
		"CarHTMLViewer",                                              // depends on unconverted modules android.car-stubs, car-ui-lib
		"com.android.runtime",                                        // depends on unconverted modules: bionic-linker-config, linkerconfig
		"currysrc",                                                   // depends on unconverted modules: currysrc_org.eclipse, guavalib, jopt-simple-4.9
		"dex2oat-script",                                             // depends on unconverted modules: dex2oat
		"generated_android_icu4j_resources",                          // depends on unconverted modules: android_icu4j_srcgen_binary
		"generated_android_icu4j_test_resources",                     // depends on unconverted modules: android_icu4j_srcgen_binary
		"host-libprotobuf-java-nano",                                 // b/220869005, depends on libprotobuf-java-nano
		"jacoco-stubs",                                               // b/245767077, depends on droidstubs
		"libapexutil",                                                // depends on unconverted modules: apex-info-list-tinyxml
		"libart",                                                     // depends on unconverted modules: apex-info-list-tinyxml, libtinyxml2, libnativeloader-headers, heapprofd_client_api, art_operator_srcs, libcpu_features, libodrstatslog, libelffile, art_cmdlineparser_headers, cpp-define-generator-definitions, libdexfile, libnativebridge, libnativeloader, libsigchain, libartbase, libprofile, cpp-define-generator-asm-support
		"libart-runtime-gtest",                                       // depends on unconverted modules: libgtest_isolated, libart-compiler, libdexfile, libprofile, libartbase, libartbase-art-gtest
		"libart_headers",                                             // depends on unconverted modules: art_libartbase_headers
		"libartbase-art-gtest",                                       // depends on unconverted modules: libgtest_isolated, libart, libart-compiler, libdexfile, libprofile
		"libartbased-art-gtest",                                      // depends on unconverted modules: libgtest_isolated, libartd, libartd-compiler, libdexfiled, libprofiled
		"libartd",                                                    // depends on unconverted modules: art_operator_srcs, libcpu_features, libodrstatslog, libelffiled, art_cmdlineparser_headers, cpp-define-generator-definitions, libdexfiled, libnativebridge, libnativeloader, libsigchain, libartbased, libprofiled, cpp-define-generator-asm-support, apex-info-list-tinyxml, libtinyxml2, libnativeloader-headers, heapprofd_client_api
		"libartd-runtime-gtest",                                      // depends on unconverted modules: libgtest_isolated, libartd-compiler, libdexfiled, libprofiled, libartbased, libartbased-art-gtest
		"libdebuggerd",                                               // depends on unconverted module: libdexfile
		"libdebuggerd_handler",                                       // depends on unconverted module libdebuggerd_handler_core
		"libdebuggerd_handler_core", "libdebuggerd_handler_fallback", // depends on unconverted module libdebuggerd
		"libdexfiled",                                             // depends on unconverted modules: dexfile_operator_srcs, libartbased, libartpalette
		"libfastdeploy_host",                                      // depends on unconverted modules: libandroidfw, libusb, AdbWinApi
		"libgmock_main_ndk",                                       // depends on unconverted modules: libgtest_ndk_c++
		"libgmock_ndk",                                            // depends on unconverted modules: libgtest_ndk_c++
		"libnativehelper_lazy_mts_jni", "libnativehelper_mts_jni", // depends on unconverted modules: libnativetesthelper_jni, libgmock_ndk
		"libnativetesthelper_jni",   // depends on unconverted modules: libgtest_ndk_c++
		"libstatslog",               // depends on unconverted modules: libstatspull, statsd-aidl-ndk
		"libstatslog_art",           // depends on unconverted modules: statslog_art.cpp, statslog_art.h
		"linker_reloc_bench_main",   // depends on unconverted modules: liblinker_reloc_bench_*
		"malloc-rss-benchmark",      // depends on unconverted modules: libmeminfo
		"pbtombstone", "crash_dump", // depends on libdebuggerd, libunwindstack
		"releasetools_test",             // depends on unconverted modules: com.android.apex.compressed.v1
		"robolectric-sqlite4java-0.282", // depends on unconverted modules: robolectric-sqlite4java-import, robolectric-sqlite4java-native
		"static_crasher",                // depends on unconverted modules: libdebuggerd_handler
		"test_fips",                     // depends on unconverted modules: adb
		"timezone-host",                 // depends on unconverted modules: art.module.api.annotations
		"truth-host-prebuilt",           // depends on unconverted modules: truth-prebuilt
		"truth-prebuilt",                // depends on unconverted modules: asm-7.0, guava

		// '//bionic/libc:libc_bp2build_cc_library_static' is duplicated in the 'deps' attribute of rule
		"toybox-static",

		// aidl files not created
		"overlayable_policy_aidl_interface",

		//prebuilts/tools/common/m2
		// depends on //external/okio:okio-lib, which uses kotlin
		"wire-runtime",

		// depends on adbd_system_api_recovery, which is a unconverted `phony` module type
		"minadbd",

		// depends on android.hardware.health-V2.0-java
		"android.hardware.health-translate-java",

		//system/libvintf
		// depends on apex-info-list-tinyxml, unconverted xsd_config Soong module type.
		"libvintf",
		"vintf",
		"libassemblevintf",
		"assemble_vintf",
		"libvintffm",
		"vintffm",
		"checkvintf",

		// depends on audio_policy_configuration_aidl_default, xsd_config module.
		"libaudioserviceexampleimpl",
		"android.hardware.audio.service-aidl.example",

		// depends on //system/tools/aidl/build:aidl_metadata_json, which is an aidl_interfaces_metadata custom Soong type.
		"aidl_metadata_in_cpp",
		"libaidlmetadata",
		"libaidlmetadata_test",

		// depends on //system/tools/hidl/build:hidl_metadata_json, which is an hidl_interfaces_metadata custom Soong type.
		"hidl_metadata_in_cpp",
		"libhidlmetadata",
		"hidl_metadata_test",

		// cc_test related.
		// b/274164834 "Could not open Configuration file test.cfg"
		"svcenc", "svcdec",

		// Failing host cc_tests
		"memunreachable_unit_test",
		"libprocinfo_test",
		"ziparchive-tests",
		"gtest_isolated_tests",
		"libunwindstack_unit_test",
		"task_profiles_test",
		"power_tests", // failing test on server, but not on host

		// reflect: call of reflect.Value.NumField on interface Value
		// affects all cc_tests that depend on art_defaults
		"libnativebridge-tests",
		"libnativeloader_test",
		"art_libnativebridge_cts_tests",
		"art_standalone_libdexfile_external_tests",
		"art_standalone_libdexfile_support_tests",
		"libnativebridge-lazy-tests",
		"libnativebridge-test-case",
		"libnativebridge2-test-case",
		"libnativebridge3-test-case",
		"libnativebridge6-test-case",
		"libnativebridge6prezygotefork",

		"libandroidfw_tests", "aapt2_tests", // failing due to data path issues

		// cc_test with unconverted deps, or are device-only (and not verified to pass yet)
		"AMRWBEncTest",
		"AmrnbDecoderTest",     // depends on unconverted modules: libaudioutils, libsndfile
		"AmrnbEncoderTest",     // depends on unconverted modules: libaudioutils, libsndfile
		"AmrwbDecoderTest",     // depends on unconverted modules: libsndfile, libaudioutils
		"AmrwbEncoderTest",     // depends on unconverted modules: libaudioutils, libsndfile
		"Mp3DecoderTest",       // depends on unconverted modules: libsndfile, libaudioutils
		"Mpeg4H263DecoderTest", // depends on unconverted modules: libstagefright_foundation
		"Mpeg4H263EncoderTest",
		"avcdec",
		"avcenc",
		"bionic-benchmarks-tests",
		"bionic-fortify-runtime-asan-test",
		"bionic-stress-tests",
		"bionic-unit-tests",
		"bionic-unit-tests-glibc",
		"bionic-unit-tests-static",
		"boringssl_crypto_test",
		"boringssl_ssl_test",
		"cfi_test_helper",
		"cfi_test_helper2",
		"cintltst32",
		"cintltst64",
		"compare",
		"cpuid",
		"debuggerd_test", // depends on unconverted modules: libdebuggerd
		"elftls_dlopen_ie_error_helper",
		"exec_linker_helper",
		"fastdeploy_test", // depends on unconverted modules: AdbWinApi, libadb_host, libandroidfw, libfastdeploy_host, libopenscreen-discovery, libopenscreen-platform-impl, libusb
		"fdtrack_test",
		"google-benchmark-test",
		"googletest-param-test-test_ndk", // depends on unconverted modules: libgtest_ndk_c++
		"gtest-typed-test_test",
		"gtest-typed-test_test_ndk", // depends on unconverted modules: libgtest_ndk_c++, libgtest_main_ndk_c++
		"gtest_ndk_tests",           // depends on unconverted modules: libgtest_ndk_c++, libgtest_main_ndk_c++
		"gtest_ndk_tests_no_main",   // depends on unconverted modules: libgtest_ndk_c++
		"gtest_prod_test_ndk",       // depends on unconverted modules: libgtest_ndk_c++, libgtest_main_ndk_c++
		"gtest_tests",
		"gtest_tests_no_main",
		"gwp_asan_unittest",
		"half_test",
		"hashcombine_test",
		"hevcdec",
		"hevcenc",
		"hwbinderThroughputTest", // depends on unconverted modules: android.hardware.tests.libhwbinder@1.0-impl.test, android.hardware.tests.libhwbinder@1.0
		"i444tonv12_eg",
		"icu4c_sample_break",
		"intltest32",
		"intltest64",
		"ion-unit-tests",
		"jemalloc5_integrationtests",
		"jemalloc5_unittests",
		"ld_config_test_helper",
		"ld_preload_test_helper",
		"libBionicCtsGtestMain", // depends on unconverted modules: libgtest_isolated
		"libBionicLoaderTests",  // depends on unconverted modules: libmeminfo
		"libapexutil_tests",     // depends on unconverted modules: apex-info-list-tinyxml, libapexutil
		"libcutils_sockets_test",
		"libexpectedutils_test",
		"libhwbinder_latency",
		"liblog-host-test", // failing tests
		"libminijail_test",
		"libminijail_unittest_gtest",
		"libpackagelistparser_test",
		"libprotobuf_vendor_suffix_test",
		"libstagefright_amrnbdec_test", // depends on unconverted modules: libsndfile, libaudioutils
		"libstagefright_amrnbenc_test",
		"libstagefright_amrwbdec_test", // depends on unconverted modules: libsndfile, libaudioutils
		"libstagefright_m4vh263enc_test",
		"libstagefright_mp3dec_test", // depends on unconverted modules: libsndfile, libaudioutils
		"libstatssocket_test",
		"libvndksupport-tests",
		"libyuv_unittest",
		"linker-unit-tests",
		"malloc_debug_system_tests",
		"malloc_debug_unit_tests",
		"malloc_hooks_system_tests",
		"mat_test",
		"mathtest",
		"memunreachable_binder_test", // depends on unconverted modules: libbinder
		"memunreachable_test",
		"metadata_tests",
		"minijail0_cli_unittest_gtest",
		"mpeg2dec",
		"mvcdec",
		"ns_hidden_child_helper",
		"pngtest",
		"preinit_getauxval_test_helper",
		"preinit_syscall_test_helper",
		"psnr",
		"quat_test",
		"rappor-tests", // depends on unconverted modules: jsr305, guava
		"scudo_unit_tests",
		"stats-log-api-gen-test", // depends on unconverted modules: libstats_proto_host
		"syscall_filter_unittest_gtest",
		"sysprop_test", // depends on unconverted modules: libcom.android.sysprop.tests
		"thread_exit_cb_helper",
		"tls_properties_helper",
		"ulp",
		"vec_test",
		"yuvconstants",
		"yuvconvert",
		"zipalign_tests",

		// cc_test_library
		"clang_diagnostic_tests",
		"exec_linker_helper_lib",
		"fortify_disabled_for_tidy",
		"ld_config_test_helper_lib1",
		"ld_config_test_helper_lib2",
		"ld_config_test_helper_lib3",
		"ld_preload_test_helper_lib1",
		"ld_preload_test_helper_lib2",
		"libBionicElfTlsLoaderTests",
		"libBionicElfTlsTests",
		"libBionicElfTlsTests",
		"libBionicFramePointerTests",
		"libBionicFramePointerTests",
		"libBionicStandardTests",
		"libBionicStandardTests",
		"libBionicTests",
		"libart-broken",
		"libatest_simple_zip",
		"libcfi-test",
		"libcfi-test-bad",
		"libcrash_test",
		// "libcrypto_fuzz_unsafe",
		"libdl_preempt_test_1",
		"libdl_preempt_test_2",
		"libdl_test_df_1_global",
		"libdlext_test",
		"libdlext_test_different_soname",
		"libdlext_test_fd",
		"libdlext_test_norelro",
		"libdlext_test_recursive",
		"libdlext_test_zip",
		"libdvrcommon_test",
		"libfortify1-new-tests-clang",
		"libfortify1-new-tests-clang",
		"libfortify1-tests-clang",
		"libfortify1-tests-clang",
		"libfortify2-new-tests-clang",
		"libfortify2-new-tests-clang",
		"libfortify2-tests-clang",
		"libfortify2-tests-clang",
		"libgnu-hash-table-library",
		"libicutest_static",
		"liblinker_reloc_bench_000",
		"liblinker_reloc_bench_001",
		"liblinker_reloc_bench_002",
		"liblinker_reloc_bench_003",
		"liblinker_reloc_bench_004",
		"liblinker_reloc_bench_005",
		"liblinker_reloc_bench_006",
		"liblinker_reloc_bench_007",
		"liblinker_reloc_bench_008",
		"liblinker_reloc_bench_009",
		"liblinker_reloc_bench_010",
		"liblinker_reloc_bench_011",
		"liblinker_reloc_bench_012",
		"liblinker_reloc_bench_013",
		"liblinker_reloc_bench_014",
		"liblinker_reloc_bench_015",
		"liblinker_reloc_bench_016",
		"liblinker_reloc_bench_017",
		"liblinker_reloc_bench_018",
		"liblinker_reloc_bench_019",
		"liblinker_reloc_bench_020",
		"liblinker_reloc_bench_021",
		"liblinker_reloc_bench_022",
		"liblinker_reloc_bench_023",
		"liblinker_reloc_bench_024",
		"liblinker_reloc_bench_025",
		"liblinker_reloc_bench_026",
		"liblinker_reloc_bench_027",
		"liblinker_reloc_bench_028",
		"liblinker_reloc_bench_029",
		"liblinker_reloc_bench_030",
		"liblinker_reloc_bench_031",
		"liblinker_reloc_bench_032",
		"liblinker_reloc_bench_033",
		"liblinker_reloc_bench_034",
		"liblinker_reloc_bench_035",
		"liblinker_reloc_bench_036",
		"liblinker_reloc_bench_037",
		"liblinker_reloc_bench_038",
		"liblinker_reloc_bench_039",
		"liblinker_reloc_bench_040",
		"liblinker_reloc_bench_041",
		"liblinker_reloc_bench_042",
		"liblinker_reloc_bench_043",
		"liblinker_reloc_bench_044",
		"liblinker_reloc_bench_045",
		"liblinker_reloc_bench_046",
		"liblinker_reloc_bench_047",
		"liblinker_reloc_bench_048",
		"liblinker_reloc_bench_049",
		"liblinker_reloc_bench_050",
		"liblinker_reloc_bench_051",
		"liblinker_reloc_bench_052",
		"liblinker_reloc_bench_053",
		"liblinker_reloc_bench_054",
		"liblinker_reloc_bench_055",
		"liblinker_reloc_bench_056",
		"liblinker_reloc_bench_057",
		"liblinker_reloc_bench_058",
		"liblinker_reloc_bench_059",
		"liblinker_reloc_bench_060",
		"liblinker_reloc_bench_061",
		"liblinker_reloc_bench_062",
		"liblinker_reloc_bench_063",
		"liblinker_reloc_bench_064",
		"liblinker_reloc_bench_065",
		"liblinker_reloc_bench_066",
		"liblinker_reloc_bench_067",
		"liblinker_reloc_bench_068",
		"liblinker_reloc_bench_069",
		"liblinker_reloc_bench_070",
		"liblinker_reloc_bench_071",
		"liblinker_reloc_bench_072",
		"liblinker_reloc_bench_073",
		"liblinker_reloc_bench_074",
		"liblinker_reloc_bench_075",
		"liblinker_reloc_bench_076",
		"liblinker_reloc_bench_077",
		"liblinker_reloc_bench_078",
		"liblinker_reloc_bench_079",
		"liblinker_reloc_bench_080",
		"liblinker_reloc_bench_081",
		"liblinker_reloc_bench_082",
		"liblinker_reloc_bench_083",
		"liblinker_reloc_bench_084",
		"liblinker_reloc_bench_085",
		"liblinker_reloc_bench_086",
		"liblinker_reloc_bench_087",
		"liblinker_reloc_bench_088",
		"liblinker_reloc_bench_089",
		"liblinker_reloc_bench_090",
		"liblinker_reloc_bench_091",
		"liblinker_reloc_bench_092",
		"liblinker_reloc_bench_093",
		"liblinker_reloc_bench_094",
		"liblinker_reloc_bench_095",
		"liblinker_reloc_bench_096",
		"liblinker_reloc_bench_097",
		"liblinker_reloc_bench_098",
		"liblinker_reloc_bench_099",
		"liblinker_reloc_bench_100",
		"liblinker_reloc_bench_101",
		"liblinker_reloc_bench_102",
		"liblinker_reloc_bench_103",
		"liblinker_reloc_bench_104",
		"liblinker_reloc_bench_105",
		"liblinker_reloc_bench_106",
		"liblinker_reloc_bench_107",
		"liblinker_reloc_bench_108",
		"liblinker_reloc_bench_109",
		"liblinker_reloc_bench_110",
		"liblinker_reloc_bench_111",
		"liblinker_reloc_bench_112",
		"liblinker_reloc_bench_113",
		"liblinker_reloc_bench_114",
		"liblinker_reloc_bench_115",
		"liblinker_reloc_bench_116",
		"liblinker_reloc_bench_117",
		"liblinker_reloc_bench_118",
		"liblinker_reloc_bench_119",
		"liblinker_reloc_bench_120",
		"liblinker_reloc_bench_121",
		"liblinker_reloc_bench_122",
		"liblinker_reloc_bench_123",
		"liblinker_reloc_bench_124",
		"liblinker_reloc_bench_125",
		"liblinker_reloc_bench_126",
		"liblinker_reloc_bench_127",
		"liblinker_reloc_bench_128",
		"liblinker_reloc_bench_129",
		"liblinker_reloc_bench_130",
		"liblinker_reloc_bench_131",
		"liblinker_reloc_bench_132",
		"liblinker_reloc_bench_133",
		"liblinker_reloc_bench_134",
		"liblinker_reloc_bench_135",
		"liblinker_reloc_bench_136",
		"liblinker_reloc_bench_137",
		"liblinker_reloc_bench_138",
		"liblinker_reloc_bench_139",
		"liblinker_reloc_bench_140",
		"liblinker_reloc_bench_141",
		"liblinker_reloc_bench_142",
		"liblinker_reloc_bench_143",
		"liblinker_reloc_bench_144",
		"liblinker_reloc_bench_145",
		"liblinker_reloc_bench_146",
		"liblinker_reloc_bench_147",
		"liblinker_reloc_bench_148",
		"liblinker_reloc_bench_149",
		"liblinker_reloc_bench_150",
		"liblinker_reloc_bench_151",
		"liblinker_reloc_bench_152",
		"liblinker_reloc_bench_153",
		"liblinker_reloc_bench_154",
		"liblinker_reloc_bench_155",
		"liblinker_reloc_bench_156",
		"liblinker_reloc_bench_157",
		"liblinker_reloc_bench_158",
		"liblinker_reloc_bench_159",
		"liblinker_reloc_bench_160",
		"liblinker_reloc_bench_161",
		"liblinker_reloc_bench_162",
		"liblinker_reloc_bench_163",
		"liblinker_reloc_bench_164",
		"liblinker_reloc_bench_165",
		"liblinker_reloc_bench_166",
		"liblinker_reloc_bench_167",
		"liblinker_reloc_bench_168",
		"libns_hidden_child_app",
		"libns_hidden_child_global",
		"libns_hidden_child_internal",
		"libns_hidden_child_public",
		"libnstest_dlopened",
		"libnstest_ns_a_public1",
		"libnstest_ns_a_public1_internal",
		"libnstest_ns_b_public2",
		"libnstest_ns_b_public3",
		"libnstest_private",
		"libnstest_private_external",
		"libnstest_public",
		"libnstest_public_internal",
		"libnstest_root",
		"libnstest_root_not_isolated",
		"librelocations-ANDROID_REL",
		"librelocations-ANDROID_RELR",
		"librelocations-RELR",
		"librelocations-fat",
		"libsegment_gap_inner",
		"libsegment_gap_outer",
		// "libssl_fuzz_unsafe",
		"libstatssocket_private",
		"libsysv-hash-table-library",
		"libtest_atexit",
		"libtest_check_order_dlsym",
		"libtest_check_order_dlsym_1_left",
		"libtest_check_order_dlsym_2_right",
		"libtest_check_order_dlsym_3_c",
		"libtest_check_order_dlsym_a",
		"libtest_check_order_dlsym_b",
		"libtest_check_order_dlsym_d",
		"libtest_check_order_reloc_root",
		"libtest_check_order_reloc_root_1",
		"libtest_check_order_reloc_root_2",
		"libtest_check_order_reloc_siblings",
		"libtest_check_order_reloc_siblings_1",
		"libtest_check_order_reloc_siblings_2",
		"libtest_check_order_reloc_siblings_3",
		"libtest_check_order_reloc_siblings_a",
		"libtest_check_order_reloc_siblings_b",
		"libtest_check_order_reloc_siblings_c",
		"libtest_check_order_reloc_siblings_c_1",
		"libtest_check_order_reloc_siblings_c_2",
		"libtest_check_order_reloc_siblings_d",
		"libtest_check_order_reloc_siblings_e",
		"libtest_check_order_reloc_siblings_f",
		"libtest_check_rtld_next_from_library",
		"libtest_dlopen_df_1_global",
		"libtest_dlopen_from_ctor",
		"libtest_dlopen_from_ctor_main",
		"libtest_dlopen_weak_undefined_func",
		"libtest_dlsym_df_1_global",
		"libtest_dlsym_from_this",
		"libtest_dlsym_from_this_child",
		"libtest_dlsym_from_this_grandchild",
		"libtest_dlsym_weak_func",
		"libtest_dt_runpath_a",
		"libtest_dt_runpath_b",
		"libtest_dt_runpath_c",
		"libtest_dt_runpath_d",
		"libtest_dt_runpath_d_zip",
		"libtest_dt_runpath_x",
		"libtest_dt_runpath_y",
		"libtest_elftls_dynamic",
		"libtest_elftls_dynamic_filler_1",
		"libtest_elftls_dynamic_filler_2",
		"libtest_elftls_dynamic_filler_3",
		"libtest_elftls_shared_var",
		"libtest_elftls_shared_var_ie",
		"libtest_elftls_tprel",
		"libtest_empty",
		"libtest_ifunc",
		"libtest_ifunc_variable",
		"libtest_ifunc_variable_impl",
		"libtest_indirect_thread_local_dtor",
		"libtest_init_fini_order_child",
		"libtest_init_fini_order_grand_child",
		"libtest_init_fini_order_root",
		"libtest_init_fini_order_root2",
		"libtest_missing_symbol",
		"libtest_missing_symbol_child_private",
		"libtest_missing_symbol_child_public",
		"libtest_missing_symbol_root",
		"libtest_nodelete_1",
		"libtest_nodelete_2",
		"libtest_nodelete_dt_flags_1",
		"libtest_pthread_atfork",
		"libtest_relo_check_dt_needed_order",
		"libtest_relo_check_dt_needed_order_1",
		"libtest_relo_check_dt_needed_order_2",
		"libtest_simple",
		"libtest_thread_local_dtor",
		"libtest_thread_local_dtor2",
		"libtest_two_parents_child",
		"libtest_two_parents_parent1",
		"libtest_two_parents_parent2",
		"libtest_versioned_lib",
		"libtest_versioned_libv1",
		"libtest_versioned_libv2",
		"libtest_versioned_otherlib",
		"libtest_versioned_otherlib_empty",
		"libtest_versioned_uselibv1",
		"libtest_versioned_uselibv2",
		"libtest_versioned_uselibv2_other",
		"libtest_versioned_uselibv3_other",
		"libtest_with_dependency",
		"libtest_with_dependency_loop",
		"libtest_with_dependency_loop_a",
		"libtest_with_dependency_loop_b",
		"libtest_with_dependency_loop_b_tmp",
		"libtest_with_dependency_loop_c",
		"libtestshared",

		// depends on unconverted libprotobuf-java-nano
		"dnsresolverprotosnano",
		"launcherprotosnano",
		"datastallprotosnano",
		"devicepolicyprotosnano",
		"ota_metadata_proto_java",
		"merge_ota",

		// releasetools
		"releasetools_fsverity_metadata_generator",
		"verity_utils",
		"check_ota_package_signature",
		"check_target_files_vintf",
		"releasetools_check_target_files_vintf",
		"releasetools_verity_utils",
		"build_image",
		"ota_from_target_files",
		"releasetools_ota_from_target_files",
		"releasetools_build_image",
		"add_img_to_target_files",
		"releasetools_add_img_to_target_files",
		"fsverity_metadata_generator",
		"sign_target_files_apks",

		// depends on the support of yacc file
		"libapplypatch",
		"libapplypatch_modes",
		"applypatch",

		// TODO(b/254476335): disable the following due to this bug
		"libapexinfo",
		"libapexinfo_tests",

		// uses glob in $(locations)
		"libc_musl_sysroot",

		// TODO(b/266459895): depends on libunwindstack
		"libutils_test",

		// Has dependencies on other tools like ziptool, bp2build'd data properties don't work with these tests atm
		"ziparchive_tests_large",
		"mkbootimg_test",
		"certify_bootimg_test",

		// Despite being _host module types, these require devices to run
		"logd_integration_test",
		"mobly-hello-world-test",
		"mobly-multidevice-test",

		// TODO(b/274805756): Support core_platform and current java APIs
		"fake-framework",

		// TODO(b/277616982): These modules depend on private java APIs, but maybe they don't need to.
		"StreamingProtoTest",
		"textclassifierprotoslite",
		"styleprotoslite",
		"CtsPkgInstallerConstants",
		"guava-android-testlib",

		// python_test_host with test data
		"sbom_writers_test",
	}

	MixedBuildsDisabledList = []string{
		"libruy_static", "libtflite_kernel_utils", // TODO(b/237315968); Depend on prebuilt stl, not from source

		"art_libdexfile_dex_instruction_list_header", // breaks libart_mterp.armng, header not found

		"libbrotli",               // http://b/198585397, ld.lld: error: bionic/libc/arch-arm64/generic/bionic/memmove.S:95:(.text+0x10): relocation R_AARCH64_CONDBR19 out of range: -1404176 is not in [-1048576, 1048575]; references __memcpy
		"minijail_constants_json", // http://b/200899432, bazel-built cc_genrule does not work in mixed build when it is a dependency of another soong module.

		"cap_names.h",                                  // TODO(b/204913827) runfiles need to be handled in mixed builds
		"libcap",                                       // TODO(b/204913827) runfiles need to be handled in mixed builds
		"libprotobuf-cpp-full", "libprotobuf-cpp-lite", // Unsupported product&vendor suffix. b/204811222 and b/204810610.

		// Depends on libprotobuf-cpp-*
		"libadb_pairing_connection",
		"libadb_pairing_connection_static",
		"libadb_pairing_server", "libadb_pairing_server_static",

		// TODO(b/240563612) Needing `stem` selection support for cc_binary
		"crasher",

		// java_import[_host] issues
		// tradefed prebuilts depend on libprotobuf
		"prebuilt_tradefed",
		"prebuilt_tradefed-test-framework",
		// handcrafted BUILD.bazel files in //prebuilts/...
		"prebuilt_r8lib-prebuilt",
		"prebuilt_sdk-core-lambda-stubs",
		"prebuilt_android-support-collections-nodeps",
		"prebuilt_android-arch-core-common-nodeps",
		"prebuilt_android-arch-lifecycle-common-java8-nodeps",
		"prebuilt_android-arch-lifecycle-common-nodeps",
		"prebuilt_android-support-annotations-nodeps",
		"prebuilt_android-arch-paging-common-nodeps",
		"prebuilt_android-arch-room-common-nodeps",
		// TODO(b/217750501) exclude_dirs property not supported
		"prebuilt_kotlin-reflect",
		"prebuilt_kotlin-stdlib",
		"prebuilt_kotlin-stdlib-jdk7",
		"prebuilt_kotlin-stdlib-jdk8",
		"prebuilt_kotlin-test",
		// TODO(b/217750501) exclude_files property not supported
		"prebuilt_currysrc_org.eclipse",

		// TODO(b/266459895): re-enable libunwindstack
		"libunwindstack",
		"libunwindstack_stdout_log",
		"libunwindstack_no_dex",
		"libunwindstack_utils",
		"unwind_reg_info",
		"libunwindstack_local",
		"unwind_for_offline",
		"unwind",
		"unwind_info",
		"unwind_symbols",
		"libEGL",
		"libGLESv2",
		"libc_malloc_debug",
		"libcodec2_hidl@1.0",
		"libcodec2_hidl@1.1",
		"libcodec2_hidl@1.2",
		"libfdtrack",
		"libgui",
		"libgui_bufferqueue_static",
		"libmedia_codecserviceregistrant",
		"libstagefright_bufferqueue_helper_novndk",
		"libutils_test",
		"libutilscallstack",
		"mediaswcodec",
	}

	// Bazel prod-mode allowlist. Modules in this list are built by Bazel
	// in either prod mode or staging mode.
	ProdMixedBuildsEnabledList = []string{
		// M5: tzdata launch
		"com.android.tzdata",
		"test1_com.android.tzdata",
		"test3_com.android.tzdata",
		// M7: adbd launch
		"com.android.adbd",
		"test_com.android.adbd",
		"adbd_test",
		"adb_crypto_test",
		"adb_pairing_auth_test",
		"adb_pairing_connection_test",
		"adb_tls_connection_test",
		// M9: mixed builds for mainline trains launch
		"api_fingerprint",
	}

	// Staging-mode allowlist. Modules in this list are only built
	// by Bazel with --bazel-mode-staging. This list should contain modules
	// which will soon be added to the prod allowlist.
	// It is implicit that all modules in ProdMixedBuildsEnabledList will
	// also be built - do not add them to this list.
	StagingMixedBuildsEnabledList = []string{
		"com.android.neuralnetworks",
		"libneuralnetworks",
		"libneuralnetworks_static",
	}

	// These should be the libs that are included by the apexes in the ProdMixedBuildsEnabledList
	ProdDclaMixedBuildsEnabledList = []string{
		"libbase",
		"libc++",
		"libcrypto",
		"libcutils",
	}

	// These should be the libs that are included by the apexes in the StagingMixedBuildsEnabledList
	StagingDclaMixedBuildsEnabledList = []string{}

	// TODO(b/269342245): Enable the rest of the DCLA libs
	// "libssl",
	// "libstagefright_flacdec",
	// "libutils",

	// TODO(b/273282046): Make this list customizable to support various targets.
	// The list of modules which are expected to spend lots of build time.
	// With `--ninja_weight_source=soong`, ninja builds these modules and deps first.
	HugeModulesMap = map[string]int{
		"AccountManagementApp":                          DEFAULT_NINJA_WEIGHT,
		"ActivityManagerPerfTestsStubApp1":              DEFAULT_NINJA_WEIGHT,
		"ActivityManagerPerfTestsStubApp2":              DEFAULT_NINJA_WEIGHT,
		"ActivityManagerPerfTestsStubApp3":              DEFAULT_NINJA_WEIGHT,
		"api-stubs-docs-non-updatable":                  DEFAULT_NINJA_WEIGHT,
		"AppCompatibilityTest":                          DEFAULT_NINJA_WEIGHT,
		"AppTransitionTests":                            DEFAULT_NINJA_WEIGHT,
		"art_compiler_tests":                            DEFAULT_NINJA_WEIGHT,
		"art.module.intra.core.api.stubs.source":        DEFAULT_NINJA_WEIGHT,
		"art.module.public.api.stubs.source":            DEFAULT_NINJA_WEIGHT,
		"AttestationVerificationTest":                   DEFAULT_NINJA_WEIGHT,
		"BatteryUsageStatsProtoTests":                   DEFAULT_NINJA_WEIGHT,
		"bluetooth_test_gd_unit":                        DEFAULT_NINJA_WEIGHT,
		"Bluetooth":                                     DEFAULT_NINJA_WEIGHT,
		"BluetoothInstrumentationTests":                 DEFAULT_NINJA_WEIGHT,
		"Calendar":                                      DEFAULT_NINJA_WEIGHT,
		"CalendarProvider":                              DEFAULT_NINJA_WEIGHT,
		"Camera2":                                       DEFAULT_NINJA_WEIGHT,
		"CarRotaryControllerUnitTests":                  DEFAULT_NINJA_WEIGHT,
		"CarSettingsForUnitTesting":                     DEFAULT_NINJA_WEIGHT,
		"CarSettingsUnitTests":                          DEFAULT_NINJA_WEIGHT,
		"CarSystemUI-tests":                             DEFAULT_NINJA_WEIGHT,
		"CellBroadcastApp":                              DEFAULT_NINJA_WEIGHT,
		"CellBroadcastLegacyApp":                        DEFAULT_NINJA_WEIGHT,
		"CellBroadcastReceiverOemUnitTests":             DEFAULT_NINJA_WEIGHT,
		"CellBroadcastServiceModule":                    DEFAULT_NINJA_WEIGHT,
		"CompanionDeviceManager":                        DEFAULT_NINJA_WEIGHT,
		"ConnectivityChecker":                           DEFAULT_NINJA_WEIGHT,
		"Contacts":                                      DEFAULT_NINJA_WEIGHT,
		"ContactsProvider":                              DEFAULT_NINJA_WEIGHT,
		"ContentCapturePerfTests":                       DEFAULT_NINJA_WEIGHT,
		"CorePerfTests":                                 DEFAULT_NINJA_WEIGHT,
		"crosvm":                                        DEFAULT_NINJA_WEIGHT,
		"CtsDomainVerificationDeviceMultiUserTestCases": DEFAULT_NINJA_WEIGHT,
		"CtsLogdTestCases":                              DEFAULT_NINJA_WEIGHT,
		"CtsMediaProviderTranscodeTests":                DEFAULT_NINJA_WEIGHT,
		"CtsRollbackManagerHostTestHelperApp":           DEFAULT_NINJA_WEIGHT,
		"CtsRollbackManagerHostTestHelperApp2":          DEFAULT_NINJA_WEIGHT,
		"CtsRootPackageInstallerTestCases":              DEFAULT_NINJA_WEIGHT,
		"CtsRootRollbackManagerHostTestHelperApp":       DEFAULT_NINJA_WEIGHT,
		"CtsTranscodeTestAppSupportsHevc":               DEFAULT_NINJA_WEIGHT,
		"CtsTranscodeTestAppSupportsSlowMotion":         DEFAULT_NINJA_WEIGHT,
		"CuttlefishDisplayHotplugHelperApp":             DEFAULT_NINJA_WEIGHT,
		"cvd-host_package":                              DEFAULT_NINJA_WEIGHT,
		"DelegateTestApp":                               DEFAULT_NINJA_WEIGHT,
		"DeskClock":                                     DEFAULT_NINJA_WEIGHT,
		"Development":                                   DEFAULT_NINJA_WEIGHT,
		"DeviceAdminTestApp":                            DEFAULT_NINJA_WEIGHT,
		"DevicePolicyManagementRoleHolderTestApp":       DEFAULT_NINJA_WEIGHT,
		"dex2oatd":                                      DEFAULT_NINJA_WEIGHT,
		"DocumentsUI":                                   DEFAULT_NINJA_WEIGHT,
		"EasterEgg":                                     DEFAULT_NINJA_WEIGHT,
		"EffectProxyTest":                               DEFAULT_NINJA_WEIGHT,
		"EmergencyInfo":                                 DEFAULT_NINJA_WEIGHT,
		"EmptyTestApp":                                  DEFAULT_NINJA_WEIGHT,
		"ExtServices":                                   DEFAULT_NINJA_WEIGHT,
		"FacebookAppsScenarioTests":                     DEFAULT_NINJA_WEIGHT,
		"flickerlib-core":                               DEFAULT_NINJA_WEIGHT,
		"flickerlib":                                    DEFAULT_NINJA_WEIGHT,
		"FlickerLibTest":                                DEFAULT_NINJA_WEIGHT,
		"FlickerTests":                                  DEFAULT_NINJA_WEIGHT,
		"framework-minus-apex":                          DEFAULT_NINJA_WEIGHT,
		"framework-res":                                 DEFAULT_NINJA_WEIGHT,
		"FrameworksCoreTests":                           DEFAULT_NINJA_WEIGHT,
		"FrameworksMockingCoreTests":                    DEFAULT_NINJA_WEIGHT,
		"FrameworksMockingServicesTests":                DEFAULT_NINJA_WEIGHT,
		"FrameworksNetSmokeTests":                       DEFAULT_NINJA_WEIGHT,
		"FrameworksNetTests":                            DEFAULT_NINJA_WEIGHT,
		"FrameworksServicesTests":                       DEFAULT_NINJA_WEIGHT,
		"FrameworksTelephonyTests":                      DEFAULT_NINJA_WEIGHT,
		"FrameworksUiServicesTests":                     DEFAULT_NINJA_WEIGHT,
		"FrameworksVcnTests":                            DEFAULT_NINJA_WEIGHT,
		"Gallery2":                                      DEFAULT_NINJA_WEIGHT,
		"GameCoreDevice":                                DEFAULT_NINJA_WEIGHT,
		"GoogleBluetoothInstrumentationTests":           DEFAULT_NINJA_WEIGHT,
		"guice_munged_srcs":                             DEFAULT_NINJA_WEIGHT,
		"HalfSheetUX":                                   DEFAULT_NINJA_WEIGHT,
		"ImePerfTests":                                  DEFAULT_NINJA_WEIGHT,
		"imgdiag":                                       DEFAULT_NINJA_WEIGHT,
		"ImsServiceEntitlement":                         DEFAULT_NINJA_WEIGHT,
		"ImsServiceEntitlementUnitTests":                DEFAULT_NINJA_WEIGHT,
		"InputTests":                                    DEFAULT_NINJA_WEIGHT,
		"InstallTest":                                   DEFAULT_NINJA_WEIGHT,
		"IntentResolver":                                DEFAULT_NINJA_WEIGHT,
		"JankBench":                                     DEFAULT_NINJA_WEIGHT,
		"jsilver":                                       DEFAULT_NINJA_WEIGHT,
		"KeyChain":                                      DEFAULT_NINJA_WEIGHT,
		"KeyChainTests":                                 DEFAULT_NINJA_WEIGHT,
		"keystore2":                                     DEFAULT_NINJA_WEIGHT,
		"LargeResourcesCompressed":                      DEFAULT_NINJA_WEIGHT,
		"LatinIME":                                      DEFAULT_NINJA_WEIGHT,
		"Launcher3QuickStepLib":                         DEFAULT_NINJA_WEIGHT,
		"libaom":                                        DEFAULT_NINJA_WEIGHT,
		"libart-broken":                                 DEFAULT_NINJA_WEIGHT,
		"libart-compiler":                               DEFAULT_NINJA_WEIGHT,
		"libart-disassembler":                           DEFAULT_NINJA_WEIGHT,
		"libart":                                        DEFAULT_NINJA_WEIGHT,
		"libartd":                                       DEFAULT_NINJA_WEIGHT,
		"libaudiohal@7.1":                               DEFAULT_NINJA_WEIGHT,
		"libbluetooth_core_rs":                          DEFAULT_NINJA_WEIGHT,
		"libbluetooth_gd_unit_tests":                    DEFAULT_NINJA_WEIGHT,
		"libbluetooth_gd":                               DEFAULT_NINJA_WEIGHT,
		"libbluetooth_rs":                               DEFAULT_NINJA_WEIGHT,
		"libbluetooth-for-tests":                        DEFAULT_NINJA_WEIGHT,
		"libbt_common":                                  DEFAULT_NINJA_WEIGHT,
		"libbt_packets_nonapex":                         DEFAULT_NINJA_WEIGHT,
		"libbt_packets":                                 DEFAULT_NINJA_WEIGHT,
		"libbt_shim_ffi":                                DEFAULT_NINJA_WEIGHT,
		"libbt_shim":                                    DEFAULT_NINJA_WEIGHT,
		"libbt-audio-hal-interface":                     DEFAULT_NINJA_WEIGHT,
		"libbt-bta-core":                                DEFAULT_NINJA_WEIGHT,
		"libbt-bta":                                     DEFAULT_NINJA_WEIGHT,
		"libbt-common":                                  DEFAULT_NINJA_WEIGHT,
		"libbt-hci":                                     DEFAULT_NINJA_WEIGHT,
		"libbt-platform-protos-lite":                    DEFAULT_NINJA_WEIGHT,
		"libbt-protos-lite":                             DEFAULT_NINJA_WEIGHT,
		"libbt-sbc-decoder":                             DEFAULT_NINJA_WEIGHT,
		"libc":                                          DEFAULT_NINJA_WEIGHT,
		"libclap":                                       DEFAULT_NINJA_WEIGHT,
		"libcodec2_soft_av1dec_gav1":                    DEFAULT_NINJA_WEIGHT,
		"libcompositionengine_test":                     DEFAULT_NINJA_WEIGHT,
		"libdevices":                                    DEFAULT_NINJA_WEIGHT,
		"libfrontend_proto":                             DEFAULT_NINJA_WEIGHT,
		"libhwtrust":                                    DEFAULT_NINJA_WEIGHT,
		"libjni":                                        DEFAULT_NINJA_WEIGHT,
		"libkeystore2":                                  DEFAULT_NINJA_WEIGHT,
		"libkmr_ta":                                     DEFAULT_NINJA_WEIGHT,
		"liblmp":                                        DEFAULT_NINJA_WEIGHT,
		"libopenjdkjvmtid":                              DEFAULT_NINJA_WEIGHT,
		"libprotobuf_deprecated":                        DEFAULT_NINJA_WEIGHT,
		"libprotobuf":                                   DEFAULT_NINJA_WEIGHT,
		"libregex":                                      DEFAULT_NINJA_WEIGHT,
		"LibStatsPullTests":                             DEFAULT_NINJA_WEIGHT,
		"libstd":                                        DEFAULT_NINJA_WEIGHT,
		"libsurfaceflinger_unittest":                    DEFAULT_NINJA_WEIGHT,
		"libsyn":                                        DEFAULT_NINJA_WEIGHT,
		"libtokio":                                      DEFAULT_NINJA_WEIGHT,
		"libuwb_core":                                   DEFAULT_NINJA_WEIGHT,
		"libuwb_uci_jni_rust":                           DEFAULT_NINJA_WEIGHT,
		"libuwb_uci_packets":                            DEFAULT_NINJA_WEIGHT,
		"libvpx":                                        DEFAULT_NINJA_WEIGHT,
		"libvulkan_enc":                                 DEFAULT_NINJA_WEIGHT,
		"libwebrtc":                                     DEFAULT_NINJA_WEIGHT,
		"LiveWallpapersPicker":                          DEFAULT_NINJA_WEIGHT,
		"LockTaskApp":                                   DEFAULT_NINJA_WEIGHT,
		"LongevityPlatformLibTests":                     DEFAULT_NINJA_WEIGHT,
		"ManagedProvisioning":                           DEFAULT_NINJA_WEIGHT,
		"ManagedProvisioningTests":                      DEFAULT_NINJA_WEIGHT,
		"MediaProvider":                                 DEFAULT_NINJA_WEIGHT,
		"MediaProviderClientTests":                      DEFAULT_NINJA_WEIGHT,
		"MediaProviderLegacy":                           DEFAULT_NINJA_WEIGHT,
		"messaging":                                     DEFAULT_NINJA_WEIGHT,
		"metalava":                                      DEFAULT_NINJA_WEIGHT,
		"MicrobenchmarkRunnerTests":                     DEFAULT_NINJA_WEIGHT,
		"microdroid_manager":                            DEFAULT_NINJA_WEIGHT,
		"minikin_tests":                                 DEFAULT_NINJA_WEIGHT,
		"MLCTestApp":                                    DEFAULT_NINJA_WEIGHT,
		"MmsService":                                    DEFAULT_NINJA_WEIGHT,
		"MmsServiceTests":                               DEFAULT_NINJA_WEIGHT,
		"module-lib-api-stubs-docs-non-updatable":       DEFAULT_NINJA_WEIGHT,
		"motion_tool_lib_tests":                         DEFAULT_NINJA_WEIGHT,
		"MtpService":                                    DEFAULT_NINJA_WEIGHT,
		"MultiUserTests":                                DEFAULT_NINJA_WEIGHT,
		"NearbyIntegrationUiTests":                      DEFAULT_NINJA_WEIGHT,
		"net_test_bluetooth":                            DEFAULT_NINJA_WEIGHT,
		"net_test_btif":                                 DEFAULT_NINJA_WEIGHT,
		"net_test_main_shim":                            DEFAULT_NINJA_WEIGHT,
		"net_test_stack":                                DEFAULT_NINJA_WEIGHT,
		"net-tests-utils":                               DEFAULT_NINJA_WEIGHT,
		"NetworkStackCoverageTests":                     DEFAULT_NINJA_WEIGHT,
		"NetworkStackIntegrationTests":                  DEFAULT_NINJA_WEIGHT,
		"NetworkStackNext":                              DEFAULT_NINJA_WEIGHT,
		"NfcNci":                                        DEFAULT_NINJA_WEIGHT,
		"NfcNciUnitTests":                               DEFAULT_NINJA_WEIGHT,
		"NotEmptyTestApp":                               DEFAULT_NINJA_WEIGHT,
		"NotificationFunctionalTests":                   DEFAULT_NINJA_WEIGHT,
		"oatdumpd":                                      DEFAULT_NINJA_WEIGHT,
		"OsuLogin":                                      DEFAULT_NINJA_WEIGHT,
		"PackageInstaller":                              DEFAULT_NINJA_WEIGHT,
		"PackageManagerComponentOverrideTests":          DEFAULT_NINJA_WEIGHT,
		"PackageManagerPerfTests":                       DEFAULT_NINJA_WEIGHT,
		"PackageManagerServiceServerTests":              DEFAULT_NINJA_WEIGHT,
		"PackageManagerServiceUnitTests":                DEFAULT_NINJA_WEIGHT,
		"PackageWatchdogTest":                           DEFAULT_NINJA_WEIGHT,
		"PandoraServerLib":                              DEFAULT_NINJA_WEIGHT,
		"pdl":                                           DEFAULT_NINJA_WEIGHT,
		"perfetto_trace_java_protos":                    DEFAULT_NINJA_WEIGHT,
		"perfetto_trace-full":                           DEFAULT_NINJA_WEIGHT,
		"PermissionController":                          DEFAULT_NINJA_WEIGHT,
		"PermissionControllerMockingTests":              DEFAULT_NINJA_WEIGHT,
		"PixelAppCompTests":                             DEFAULT_NINJA_WEIGHT,
		"platform-bootclasspath":                        DEFAULT_NINJA_WEIGHT,
		"PlatformCommonScenarioTests":                   DEFAULT_NINJA_WEIGHT,
		"PlatformComposeCoreTests":                      DEFAULT_NINJA_WEIGHT,
		"platformprotoslite":                            DEFAULT_NINJA_WEIGHT,
		"PlatformRuleTests":                             DEFAULT_NINJA_WEIGHT,
		"precompiled_sepolicy-without_apex":             DEFAULT_NINJA_WEIGHT,
		"PresencePolling":                               DEFAULT_NINJA_WEIGHT,
		"PrintSpooler":                                  DEFAULT_NINJA_WEIGHT,
		"QuickSearchBox":                                DEFAULT_NINJA_WEIGHT,
		"RemoteDPCTestApp":                              DEFAULT_NINJA_WEIGHT,
		"RemoteProvisioningServiceTests":                DEFAULT_NINJA_WEIGHT,
		"RkpdAppUnitTests":                              DEFAULT_NINJA_WEIGHT,
		"Robolectric_shadows_framework":                 DEFAULT_NINJA_WEIGHT,
		"RoleHolderApp":                                 DEFAULT_NINJA_WEIGHT,
		"SdkSandbox":                                    DEFAULT_NINJA_WEIGHT,
		"service-appsearch":                             DEFAULT_NINJA_WEIGHT,
		"service-connectivity":                          DEFAULT_NINJA_WEIGHT,
		"service-uwb":                                   DEFAULT_NINJA_WEIGHT,
		"service-wifi":                                  DEFAULT_NINJA_WEIGHT,
		"services-non-updatable-stubs":                  DEFAULT_NINJA_WEIGHT,
		"services":                                      DEFAULT_NINJA_WEIGHT,
		"Settings-core":                                 DEFAULT_NINJA_WEIGHT,
		"Settings":                                      DEFAULT_NINJA_WEIGHT,
		"SettingsIntelligence":                          DEFAULT_NINJA_WEIGHT,
		"SettingsLibTests":                              DEFAULT_NINJA_WEIGHT,
		"SettingsProvider":                              DEFAULT_NINJA_WEIGHT,
		"Shell":                                         DEFAULT_NINJA_WEIGHT,
		"SimAppDialog":                                  DEFAULT_NINJA_WEIGHT,
		"sl4a":                                          DEFAULT_NINJA_WEIGHT,
		"SmsApp":                                        DEFAULT_NINJA_WEIGHT,
		"SoundPicker":                                   DEFAULT_NINJA_WEIGHT,
		"StagedInstallTest":                             DEFAULT_NINJA_WEIGHT,
		"StatementService":                              DEFAULT_NINJA_WEIGHT,
		"StatsdFrameworkTestApp":                        DEFAULT_NINJA_WEIGHT,
		"StatsdFrameworkTestAppNoPermission":            DEFAULT_NINJA_WEIGHT,
		"statsdprotolite":                               DEFAULT_NINJA_WEIGHT,
		"Stk":                                           DEFAULT_NINJA_WEIGHT,
		"StorageManager":                                DEFAULT_NINJA_WEIGHT,
		"system-api-stubs-docs-non-updatable":           DEFAULT_NINJA_WEIGHT,
		"SystemUI-core":                                 DEFAULT_NINJA_WEIGHT,
		"SystemUI-tests-base":                           DEFAULT_NINJA_WEIGHT,
		"SystemUI-tests":                                DEFAULT_NINJA_WEIGHT,
		"SystemUI":                                      DEFAULT_NINJA_WEIGHT,
		"SystemUIComposeFeatures":                       DEFAULT_NINJA_WEIGHT,
		"SystemUIComposeFeaturesTests":                  DEFAULT_NINJA_WEIGHT,
		"SystemUITests":                                 DEFAULT_NINJA_WEIGHT,
		"Tag":                                           DEFAULT_NINJA_WEIGHT,
		"Telecom":                                       DEFAULT_NINJA_WEIGHT,
		"TelecomUnitTests":                              DEFAULT_NINJA_WEIGHT,
		"telephony-common":                              DEFAULT_NINJA_WEIGHT,
		"TelephonyProvider":                             DEFAULT_NINJA_WEIGHT,
		"TeleService":                                   DEFAULT_NINJA_WEIGHT,
		"test-api-stubs-docs-non-updatable":             DEFAULT_NINJA_WEIGHT,
		"TetheringIntegrationTests":                     DEFAULT_NINJA_WEIGHT,
		"TetheringNext":                                 DEFAULT_NINJA_WEIGHT,
		"ThemePickerTests":                              DEFAULT_NINJA_WEIGHT,
		"Traceur":                                       DEFAULT_NINJA_WEIGHT,
		"UsbManagerTests":                               DEFAULT_NINJA_WEIGHT,
		"UsbTests":                                      DEFAULT_NINJA_WEIGHT,
		"virtmgr":                                       DEFAULT_NINJA_WEIGHT,
		"WallpaperPicker2TestLib":                       DEFAULT_NINJA_WEIGHT,
		"WallpaperPicker2Tests":                         DEFAULT_NINJA_WEIGHT,
		"WifiDialog":                                    DEFAULT_NINJA_WEIGHT,
		"wm-proto-parsers":                              DEFAULT_NINJA_WEIGHT,
		"WMShellFlickerTests":                           DEFAULT_NINJA_WEIGHT,
		"WmTests":                                       DEFAULT_NINJA_WEIGHT,
		"wpa_supplicant":                                DEFAULT_NINJA_WEIGHT,
	}
)
