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

	// Modules with build time of more than half a minute should have high priority.
	DEFAULT_PRIORITIZED_WEIGHT = 1000
	// Modules with build time of more than a few minute should have higher priority.
	HIGH_PRIORITIZED_WEIGHT = 10 * DEFAULT_PRIORITIZED_WEIGHT
	// Modules with inputs greater than the threshold should have high priority.
	// Adjust this threshold if there are lots of wrong predictions.
	INPUT_SIZE_THRESHOLD = 50
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
		"build/make/tools":                   Bp2BuildDefaultTrue,
		"build/make/tools/protos":            Bp2BuildDefaultTrue,
		"build/make/tools/releasetools":      Bp2BuildDefaultTrue,
		"build/make/tools/sbom":              Bp2BuildDefaultTrue,
		"build/make/tools/signapk":           Bp2BuildDefaultTrue,
		"build/make/tools/zipalign":          Bp2BuildDefaultTrueRecursively,
		"build/soong":                        Bp2BuildDefaultTrue,
		"build/soong/cc/libbuildversion":     Bp2BuildDefaultTrue, // Skip tests subdir
		"build/soong/cc/ndkstubgen":          Bp2BuildDefaultTrue,
		"build/soong/cc/symbolfile":          Bp2BuildDefaultTrue,
		"build/soong/jar":                    Bp2BuildDefaultTrue,
		"build/soong/licenses":               Bp2BuildDefaultTrue,
		"build/soong/linkerconfig":           Bp2BuildDefaultTrueRecursively,
		"build/soong/response":               Bp2BuildDefaultTrue,
		"build/soong/scripts":                Bp2BuildDefaultTrueRecursively,
		"build/soong/third_party/zip":        Bp2BuildDefaultTrue,

		"cts/common/device-side/nativetesthelper/jni": Bp2BuildDefaultTrueRecursively,
		"cts/flags/cc_tests":                          Bp2BuildDefaultTrueRecursively,
		"cts/libs/json":                               Bp2BuildDefaultTrueRecursively,
		"cts/tests/tests/gesture":                     Bp2BuildDefaultTrueRecursively,

		"dalvik/tools/dexdeps": Bp2BuildDefaultTrueRecursively,

		"development/apps/DevelopmentSettings":        Bp2BuildDefaultTrue,
		"development/apps/Fallback":                   Bp2BuildDefaultTrue,
		"development/apps/WidgetPreview":              Bp2BuildDefaultTrue,
		"development/python-packages/adb":             Bp2BuildDefaultTrueRecursively,
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

		"external/aac":                             Bp2BuildDefaultTrueRecursively,
		"external/abseil-cpp":                      Bp2BuildDefaultTrueRecursively,
		"external/arm-optimized-routines":          Bp2BuildDefaultTrueRecursively,
		"external/auto":                            Bp2BuildDefaultTrue,
		"external/auto/android-annotation-stubs":   Bp2BuildDefaultTrueRecursively,
		"external/auto/common":                     Bp2BuildDefaultTrueRecursively,
		"external/auto/service":                    Bp2BuildDefaultTrueRecursively,
		"external/auto/value":                      Bp2BuildDefaultTrueRecursively,
		"external/boringssl":                       Bp2BuildDefaultTrueRecursively,
		"external/bouncycastle":                    Bp2BuildDefaultTrue,
		"external/brotli":                          Bp2BuildDefaultTrue,
		"external/bsdiff":                          Bp2BuildDefaultTrueRecursively,
		"external/bzip2":                           Bp2BuildDefaultTrueRecursively,
		"external/clang/lib":                       Bp2BuildDefaultTrue,
		"external/conscrypt":                       Bp2BuildDefaultTrue,
		"external/dexmaker":                        Bp2BuildDefaultTrueRecursively,
		"external/e2fsprogs":                       Bp2BuildDefaultTrueRecursively,
		"external/eigen":                           Bp2BuildDefaultTrueRecursively,
		"external/erofs-utils":                     Bp2BuildDefaultTrueRecursively,
		"external/error_prone":                     Bp2BuildDefaultTrueRecursively,
		"external/escapevelocity":                  Bp2BuildDefaultTrueRecursively,
		"external/expat":                           Bp2BuildDefaultTrueRecursively,
		"external/f2fs-tools":                      Bp2BuildDefaultTrue,
		"external/flac":                            Bp2BuildDefaultTrueRecursively,
		"external/flatbuffers":                     Bp2BuildDefaultTrueRecursively,
		"external/fmtlib":                          Bp2BuildDefaultTrueRecursively,
		"external/fsverity-utils":                  Bp2BuildDefaultTrueRecursively,
		"external/gflags":                          Bp2BuildDefaultTrueRecursively,
		"external/google-benchmark":                Bp2BuildDefaultTrueRecursively,
		"external/googletest":                      Bp2BuildDefaultTrueRecursively,
		"external/guava":                           Bp2BuildDefaultTrueRecursively,
		"external/gwp_asan":                        Bp2BuildDefaultTrueRecursively,
		"external/hamcrest":                        Bp2BuildDefaultTrueRecursively,
		"external/icu":                             Bp2BuildDefaultTrueRecursively,
		"external/icu/android_icu4j":               Bp2BuildDefaultFalse, // java rules incomplete
		"external/icu/icu4j":                       Bp2BuildDefaultFalse, // java rules incomplete
		"external/jacoco":                          Bp2BuildDefaultTrueRecursively,
		"external/jarjar":                          Bp2BuildDefaultTrueRecursively,
		"external/javaparser":                      Bp2BuildDefaultTrueRecursively,
		"external/javapoet":                        Bp2BuildDefaultTrueRecursively,
		"external/javassist":                       Bp2BuildDefaultTrueRecursively,
		"external/jemalloc_new":                    Bp2BuildDefaultTrueRecursively,
		"external/jsoncpp":                         Bp2BuildDefaultTrueRecursively,
		"external/jsr305":                          Bp2BuildDefaultTrueRecursively,
		"external/jsr330":                          Bp2BuildDefaultTrueRecursively,
		"external/junit":                           Bp2BuildDefaultTrueRecursively,
		"external/kotlinc":                         Bp2BuildDefaultTrueRecursively,
		"external/kotlinx.coroutines":              Bp2BuildDefaultTrueRecursively,
		"external/libaom":                          Bp2BuildDefaultTrueRecursively,
		"external/libavc":                          Bp2BuildDefaultTrueRecursively,
		"external/libcap":                          Bp2BuildDefaultTrueRecursively,
		"external/libcxx":                          Bp2BuildDefaultTrueRecursively,
		"external/libcxxabi":                       Bp2BuildDefaultTrueRecursively,
		"external/libdivsufsort":                   Bp2BuildDefaultTrueRecursively,
		"external/libdrm":                          Bp2BuildDefaultTrue,
		"external/libevent":                        Bp2BuildDefaultTrueRecursively,
		"external/libgav1":                         Bp2BuildDefaultTrueRecursively,
		"external/libdav1d":                        Bp2BuildDefaultTrueRecursively,
		"external/libhevc":                         Bp2BuildDefaultTrueRecursively,
		"external/libjpeg-turbo":                   Bp2BuildDefaultTrueRecursively,
		"external/libmpeg2":                        Bp2BuildDefaultTrueRecursively,
		"external/libphonenumber":                  Bp2BuildDefaultTrueRecursively,
		"external/libpng":                          Bp2BuildDefaultTrueRecursively,
		"external/libvpx":                          Bp2BuildDefaultTrueRecursively,
		"external/libyuv":                          Bp2BuildDefaultTrueRecursively,
		"external/lz4/lib":                         Bp2BuildDefaultTrue,
		"external/lz4/programs":                    Bp2BuildDefaultTrue,
		"external/lzma/C":                          Bp2BuildDefaultTrueRecursively,
		"external/mdnsresponder":                   Bp2BuildDefaultTrueRecursively,
		"external/minijail":                        Bp2BuildDefaultTrueRecursively,
		"external/mockito":                         Bp2BuildDefaultTrueRecursively,
		"external/musl":                            Bp2BuildDefaultTrueRecursively,
		"external/objenesis":                       Bp2BuildDefaultTrueRecursively,
		"external/openscreen":                      Bp2BuildDefaultTrueRecursively,
		"external/ow2-asm":                         Bp2BuildDefaultTrueRecursively,
		"external/pcre":                            Bp2BuildDefaultTrueRecursively,
		"external/perfmark/api":                    Bp2BuildDefaultTrueRecursively,
		"external/perfetto":                        Bp2BuildDefaultTrue,
		"external/protobuf":                        Bp2BuildDefaultTrueRecursively,
		"external/python/jinja/src":                Bp2BuildDefaultTrueRecursively,
		"external/python/markupsafe/src":           Bp2BuildDefaultTrueRecursively,
		"external/python/pyfakefs/pyfakefs":        Bp2BuildDefaultTrueRecursively,
		"external/python/pyyaml/lib/yaml":          Bp2BuildDefaultTrueRecursively,
		"external/python/setuptools":               Bp2BuildDefaultTrueRecursively,
		"external/python/six":                      Bp2BuildDefaultTrueRecursively,
		"external/rappor":                          Bp2BuildDefaultTrueRecursively,
		"external/rust/crates/rustc-demangle":      Bp2BuildDefaultTrueRecursively,
		"external/rust/crates/rustc-demangle-capi": Bp2BuildDefaultTrueRecursively,
		"external/scudo":                           Bp2BuildDefaultTrueRecursively,
		"external/selinux/checkpolicy":             Bp2BuildDefaultTrueRecursively,
		"external/selinux/libselinux":              Bp2BuildDefaultTrueRecursively,
		"external/selinux/libsepol":                Bp2BuildDefaultTrueRecursively,
		"external/speex":                           Bp2BuildDefaultTrueRecursively,
		"external/sqlite":                          Bp2BuildDefaultTrueRecursively,
		"external/tinyalsa":                        Bp2BuildDefaultTrueRecursively,
		"external/tinyalsa_new":                    Bp2BuildDefaultTrueRecursively,
		"external/toybox":                          Bp2BuildDefaultTrueRecursively,
		"external/truth":                           Bp2BuildDefaultTrueRecursively,
		"external/xz-java":                         Bp2BuildDefaultTrueRecursively,
		"external/zlib":                            Bp2BuildDefaultTrueRecursively,
		"external/zopfli":                          Bp2BuildDefaultTrueRecursively,
		"external/zstd":                            Bp2BuildDefaultTrueRecursively,

		"frameworks/av": Bp2BuildDefaultTrue,
		"frameworks/av/media/audioaidlconversion":                              Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/codec2/components/aom":                            Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/codecs":                                           Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/liberror":                                         Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/libmediahelper":                                   Bp2BuildDefaultTrue,
		"frameworks/av/media/libshmem":                                         Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/module/codecs":                                    Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/module/foundation":                                Bp2BuildDefaultTrueRecursively,
		"frameworks/av/media/module/minijail":                                  Bp2BuildDefaultTrueRecursively,
		"frameworks/av/services/minijail":                                      Bp2BuildDefaultTrueRecursively,
		"frameworks/base/apex/jobscheduler/service/jni":                        Bp2BuildDefaultTrueRecursively,
		"frameworks/base/core/java":                                            Bp2BuildDefaultTrue,
		"frameworks/base/core/res":                                             Bp2BuildDefaultTrueRecursively,
		"frameworks/base/errorprone":                                           Bp2BuildDefaultTrueRecursively,
		"frameworks/base/libs/androidfw":                                       Bp2BuildDefaultTrue,
		"frameworks/base/libs/services":                                        Bp2BuildDefaultTrue,
		"frameworks/base/media/tests/MediaDump":                                Bp2BuildDefaultTrue,
		"frameworks/base/mime":                                                 Bp2BuildDefaultTrueRecursively,
		"frameworks/base/proto":                                                Bp2BuildDefaultTrue,
		"frameworks/base/services/tests/servicestests/aidl":                    Bp2BuildDefaultTrue,
		"frameworks/base/startop/apps/test":                                    Bp2BuildDefaultTrue,
		"frameworks/base/tests/appwidgets/AppWidgetHostTest":                   Bp2BuildDefaultTrueRecursively,
		"frameworks/base/tools/aapt":                                           Bp2BuildDefaultTrue,
		"frameworks/base/tools/aapt2":                                          Bp2BuildDefaultTrue,
		"frameworks/base/tools/codegen":                                        Bp2BuildDefaultTrueRecursively,
		"frameworks/base/tools/locked_region_code_injection":                   Bp2BuildDefaultTrueRecursively,
		"frameworks/base/tools/streaming_proto":                                Bp2BuildDefaultTrueRecursively,
		"frameworks/hardware/interfaces":                                       Bp2BuildDefaultTrue,
		"frameworks/hardware/interfaces/displayservice":                        Bp2BuildDefaultTrueRecursively,
		"frameworks/hardware/interfaces/stats/aidl":                            Bp2BuildDefaultTrue,
		"frameworks/libs/modules-utils/build":                                  Bp2BuildDefaultTrueRecursively,
		"frameworks/libs/modules-utils/java":                                   Bp2BuildDefaultTrueRecursively,
		"frameworks/libs/modules-utils/java/com/android/modules/utils/testing": Bp2BuildDefaultFalseRecursively,
		"frameworks/native":                                                    Bp2BuildDefaultTrue,
		"frameworks/native/libs/adbd_auth":                                     Bp2BuildDefaultTrueRecursively,
		"frameworks/native/libs/arect":                                         Bp2BuildDefaultTrueRecursively,
		"frameworks/native/libs/binder":                                        Bp2BuildDefaultTrue,
		"frameworks/native/libs/gui":                                           Bp2BuildDefaultTrue,
		"frameworks/native/libs/math":                                          Bp2BuildDefaultTrueRecursively,
		"frameworks/native/libs/nativebase":                                    Bp2BuildDefaultTrueRecursively,
		"frameworks/native/libs/permission":                                    Bp2BuildDefaultTrueRecursively,
		"frameworks/native/libs/ui":                                            Bp2BuildDefaultTrue,
		"frameworks/native/libs/vr":                                            Bp2BuildDefaultTrueRecursively,
		"frameworks/native/opengl/tests/gl2_cameraeye":                         Bp2BuildDefaultTrue,
		"frameworks/native/opengl/tests/gl2_java":                              Bp2BuildDefaultTrue,
		"frameworks/native/opengl/tests/testLatency":                           Bp2BuildDefaultTrue,
		"frameworks/native/opengl/tests/testPauseResume":                       Bp2BuildDefaultTrue,
		"frameworks/native/opengl/tests/testViewport":                          Bp2BuildDefaultTrue,
		"frameworks/native/services/batteryservice":                            Bp2BuildDefaultTrue,
		"frameworks/proto_logging/stats":                                       Bp2BuildDefaultTrueRecursively,

		"hardware/interfaces":                                     Bp2BuildDefaultTrue,
		"hardware/interfaces/audio/aidl":                          Bp2BuildDefaultTrue,
		"hardware/interfaces/audio/aidl/common":                   Bp2BuildDefaultTrue,
		"hardware/interfaces/audio/aidl/default":                  Bp2BuildDefaultTrue,
		"hardware/interfaces/audio/aidl/sounddose":                Bp2BuildDefaultTrue,
		"hardware/interfaces/camera/metadata/aidl":                Bp2BuildDefaultTrueRecursively,
		"hardware/interfaces/common/aidl":                         Bp2BuildDefaultTrue,
		"hardware/interfaces/common/fmq/aidl":                     Bp2BuildDefaultTrue,
		"hardware/interfaces/common/support":                      Bp2BuildDefaultTrue,
		"hardware/interfaces/configstore/1.0":                     Bp2BuildDefaultTrue,
		"hardware/interfaces/configstore/1.1":                     Bp2BuildDefaultTrue,
		"hardware/interfaces/configstore/utils":                   Bp2BuildDefaultTrue,
		"hardware/interfaces/contexthub/aidl":                     Bp2BuildDefaultTrue,
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
		"hardware/interfaces/media":                               Bp2BuildDefaultTrueRecursively,
		"hardware/interfaces/media/bufferpool/aidl/default/tests": Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/media/omx/1.0/vts":                   Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks":                      Bp2BuildDefaultTrueRecursively,
		"hardware/interfaces/neuralnetworks/aidl/vts":             Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.0/vts":              Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.1/vts":              Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.2/vts":              Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.3/vts":              Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/neuralnetworks/1.4/vts":              Bp2BuildDefaultFalseRecursively,
		"hardware/interfaces/tests":                               Bp2BuildDefaultTrueRecursively,
		"hardware/interfaces/tests/extension":                     Bp2BuildDefaultFalseRecursively, // missing deps
		"hardware/interfaces/tests/msgq":                          Bp2BuildDefaultFalseRecursively, // missing deps

		"libnativehelper": Bp2BuildDefaultTrueRecursively,

		"packages/apps/DevCamera":                                    Bp2BuildDefaultTrue,
		"packages/apps/HTMLViewer":                                   Bp2BuildDefaultTrue,
		"packages/apps/Protips":                                      Bp2BuildDefaultTrue,
		"packages/apps/SafetyRegulatoryInfo":                         Bp2BuildDefaultTrue,
		"packages/apps/WallpaperPicker":                              Bp2BuildDefaultTrue,
		"packages/modules/Connectivity/bpf_progs":                    Bp2BuildDefaultTrueRecursively,
		"packages/modules/Connectivity/service-t":                    Bp2BuildDefaultTrueRecursively,
		"packages/modules/Connectivity/service/native":               Bp2BuildDefaultTrueRecursively,
		"packages/modules/Connectivity/staticlibs/native":            Bp2BuildDefaultTrueRecursively,
		"packages/modules/Connectivity/staticlibs/netd":              Bp2BuildDefaultTrueRecursively,
		"packages/modules/Connectivity/staticlibs/netd/libnetdutils": Bp2BuildDefaultTrueRecursively,
		"packages/modules/Connectivity/tests/unit/jni":               Bp2BuildDefaultTrueRecursively,
		"packages/modules/Gki/libkver":                               Bp2BuildDefaultTrue,
		"packages/modules/NetworkStack/common/captiveportal":         Bp2BuildDefaultTrue,
		"packages/modules/NeuralNetworks/apex":                       Bp2BuildDefaultTrue,
		"packages/modules/NeuralNetworks/apex/testing":               Bp2BuildDefaultTrue,
		"packages/modules/NeuralNetworks/driver/cache":               Bp2BuildDefaultTrueRecursively,
		"packages/modules/SdkExtensions/gen_sdk":                     Bp2BuildDefaultTrue,
		"packages/modules/StatsD/lib/libstatssocket":                 Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb":                                       Bp2BuildDefaultTrue,
		"packages/modules/adb/apex":                                  Bp2BuildDefaultTrue,
		"packages/modules/adb/crypto":                                Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/fastdeploy":                            Bp2BuildDefaultTrue,
		"packages/modules/adb/libs":                                  Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/pairing_auth":                          Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/pairing_connection":                    Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/proto":                                 Bp2BuildDefaultTrueRecursively,
		"packages/modules/adb/tls":                                   Bp2BuildDefaultTrueRecursively,
		"packages/modules/common/proto":                              Bp2BuildDefaultTrue,
		"packages/providers/MediaProvider/tools/dialogs":             Bp2BuildDefaultFalse, // TODO(b/242834374)
		"packages/screensavers/Basic":                                Bp2BuildDefaultTrue,
		"packages/services/Car/tests/SampleRearViewCamera":           Bp2BuildDefaultFalse, // TODO(b/242834321)

		"platform_testing/libraries/annotations":              Bp2BuildDefaultTrueRecursively,
		"platform_testing/libraries/flag-helpers/libflagtest": Bp2BuildDefaultTrueRecursively,
		"platform_testing/tests/example":                      Bp2BuildDefaultTrueRecursively,

		"prebuilts/clang/host/linux-x86":                   Bp2BuildDefaultTrueRecursively,
		"prebuilts/gradle-plugin":                          Bp2BuildDefaultTrueRecursively,
		"prebuilts/module_sdk":                             Bp2BuildDefaultTrueRecursively,
		"prebuilts/runtime/mainline/platform/sdk":          Bp2BuildDefaultTrueRecursively,
		"prebuilts/sdk":                                    Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/androidx":                   Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/androidx-legacy":            Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/extras/app-toolkit":         Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/extras/constraint-layout-x": Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/extras/material-design-x":   Bp2BuildDefaultTrue,
		"prebuilts/sdk/current/support":                    Bp2BuildDefaultTrue,
		"prebuilts/tools":                                  Bp2BuildDefaultTrue,
		"prebuilts/tools/common/m2":                        Bp2BuildDefaultTrue,
		"prebuilts/r8":                                     Bp2BuildDefaultTrueRecursively,

		"sdk/annotations":   Bp2BuildDefaultTrueRecursively,
		"sdk/dumpeventlog":  Bp2BuildDefaultTrue,
		"sdk/eventanalyzer": Bp2BuildDefaultTrue,

		"system/apex":                                            Bp2BuildDefaultFalse, // TODO(b/207466993): flaky failures
		"system/apex/apexer":                                     Bp2BuildDefaultTrue,
		"system/apex/libs":                                       Bp2BuildDefaultTrueRecursively,
		"system/apex/libs/libapexsupport":                        Bp2BuildDefaultFalseRecursively, // TODO(b/267572288): depends on rust
		"system/apex/proto":                                      Bp2BuildDefaultTrueRecursively,
		"system/apex/tools":                                      Bp2BuildDefaultTrueRecursively,
		"system/core/debuggerd":                                  Bp2BuildDefaultTrueRecursively,
		"system/core/diagnose_usb":                               Bp2BuildDefaultTrueRecursively,
		"system/core/fs_mgr":                                     Bp2BuildDefaultTrueRecursively,
		"system/core/healthd":                                    Bp2BuildDefaultTrue,
		"system/core/healthd/testdata":                           Bp2BuildDefaultTrue,
		"system/core/libasyncio":                                 Bp2BuildDefaultTrue,
		"system/core/libcrypto_utils":                            Bp2BuildDefaultTrueRecursively,
		"system/core/libcutils":                                  Bp2BuildDefaultTrueRecursively,
		"system/core/libpackagelistparser":                       Bp2BuildDefaultTrueRecursively,
		"system/core/libprocessgroup":                            Bp2BuildDefaultTrue,
		"system/core/libprocessgroup/cgrouprc":                   Bp2BuildDefaultTrue,
		"system/core/libprocessgroup/cgrouprc_format":            Bp2BuildDefaultTrue,
		"system/core/libsparse":                                  Bp2BuildDefaultTrueRecursively,
		"system/core/libstats/expresslog":                        Bp2BuildDefaultTrueRecursively,
		"system/core/libsuspend":                                 Bp2BuildDefaultTrue,
		"system/core/libsystem":                                  Bp2BuildDefaultTrueRecursively,
		"system/core/libsysutils":                                Bp2BuildDefaultTrueRecursively,
		"system/core/libutils":                                   Bp2BuildDefaultTrueRecursively,
		"system/core/libvndksupport":                             Bp2BuildDefaultTrueRecursively,
		"system/core/mkbootfs":                                   Bp2BuildDefaultTrueRecursively,
		"system/core/property_service/libpropertyinfoparser":     Bp2BuildDefaultTrueRecursively,
		"system/core/property_service/libpropertyinfoserializer": Bp2BuildDefaultTrueRecursively,
		"system/core/trusty/libtrusty":                           Bp2BuildDefaultTrue,
		"system/extras/f2fs_utils":                               Bp2BuildDefaultTrueRecursively,
		"system/extras/toolchain-extras":                         Bp2BuildDefaultTrue,
		"system/extras/verity":                                   Bp2BuildDefaultTrueRecursively,
		"system/hardware/interfaces/media":                       Bp2BuildDefaultTrueRecursively,
		"system/incremental_delivery/incfs":                      Bp2BuildDefaultTrue,
		"system/libartpalette":                                   Bp2BuildDefaultTrueRecursively,
		"system/libbase":                                         Bp2BuildDefaultTrueRecursively,
		"system/libfmq":                                          Bp2BuildDefaultTrue,
		"system/libhidl":                                         Bp2BuildDefaultTrue,
		"system/libhidl/libhidlmemory":                           Bp2BuildDefaultTrue,
		"system/libhidl/transport":                               Bp2BuildDefaultTrue,
		"system/libhidl/transport/allocator/1.0":                 Bp2BuildDefaultTrue,
		"system/libhidl/transport/base/1.0":                      Bp2BuildDefaultTrue,
		"system/libhidl/transport/manager/1.0":                   Bp2BuildDefaultTrue,
		"system/libhidl/transport/manager/1.1":                   Bp2BuildDefaultTrue,
		"system/libhidl/transport/manager/1.2":                   Bp2BuildDefaultTrue,
		"system/libhidl/transport/memory":                        Bp2BuildDefaultTrueRecursively,
		"system/libhidl/transport/safe_union/1.0":                Bp2BuildDefaultTrue,
		"system/libhidl/transport/token/1.0":                     Bp2BuildDefaultTrue,
		"system/libhidl/transport/token/1.0/utils":               Bp2BuildDefaultTrue,
		"system/libhwbinder":                                     Bp2BuildDefaultTrueRecursively,
		"system/libprocinfo":                                     Bp2BuildDefaultTrue,
		"system/libvintf":                                        Bp2BuildDefaultTrue,
		"system/libziparchive":                                   Bp2BuildDefaultTrueRecursively,
		"system/linkerconfig":                                    Bp2BuildDefaultTrueRecursively,
		"system/logging":                                         Bp2BuildDefaultTrueRecursively,
		"system/media":                                           Bp2BuildDefaultTrue,
		"system/media/alsa_utils":                                Bp2BuildDefaultTrueRecursively,
		"system/media/audio":                                     Bp2BuildDefaultTrueRecursively,
		"system/media/audio_utils":                               Bp2BuildDefaultTrueRecursively,
		"system/media/camera":                                    Bp2BuildDefaultTrueRecursively,
		"system/memory/libion":                                   Bp2BuildDefaultTrueRecursively,
		"system/memory/libmemunreachable":                        Bp2BuildDefaultTrueRecursively,
		"system/netd":                                            Bp2BuildDefaultTrue,
		"system/security/fsverity":                               Bp2BuildDefaultTrueRecursively,
		"system/sepolicy/apex":                                   Bp2BuildDefaultTrueRecursively,
		"system/testing/gtest_extras":                            Bp2BuildDefaultTrueRecursively,
		"system/timezone/apex":                                   Bp2BuildDefaultTrueRecursively,
		"system/timezone/output_data":                            Bp2BuildDefaultTrueRecursively,
		"system/timezone/testdata":                               Bp2BuildDefaultTrueRecursively,
		"system/timezone/testing":                                Bp2BuildDefaultTrueRecursively,
		"system/tools/aidl/build/tests_bp2build":                 Bp2BuildDefaultTrue,
		"system/tools/aidl/metadata":                             Bp2BuildDefaultTrue,
		"system/tools/hidl":                                      Bp2BuildDefaultTrueRecursively,
		"system/tools/mkbootimg":                                 Bp2BuildDefaultTrueRecursively,
		"system/tools/sysprop":                                   Bp2BuildDefaultTrue,
		"system/tools/xsdc/utils":                                Bp2BuildDefaultTrueRecursively,
		"system/unwinding/libunwindstack":                        Bp2BuildDefaultTrueRecursively,

		"test/vts/vts_hal_hidl_target": Bp2BuildDefaultTrueRecursively,

		"toolchain/pgo-profiles":                      Bp2BuildDefaultTrueRecursively,
		"tools/apifinder":                             Bp2BuildDefaultTrue,
		"tools/apksig":                                Bp2BuildDefaultTrue,
		"tools/dexter/slicer":                         Bp2BuildDefaultTrueRecursively,
		"tools/external_updater":                      Bp2BuildDefaultTrueRecursively,
		"tools/metalava":                              Bp2BuildDefaultTrueRecursively,
		"tools/platform-compat/java/android/compat":   Bp2BuildDefaultTrueRecursively,
		"tools/platform-compat/java/androidprocessor": Bp2BuildDefaultTrueRecursively,
		"tools/tradefederation/core/util_apps":        Bp2BuildDefaultTrueRecursively,
		"tools/tradefederation/prebuilts/filegroups":  Bp2BuildDefaultTrueRecursively,
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
		"external/bazelbuild-rules_cc":/* recursive = */ true,
		"external/bazelbuild-rules_java":/* recursive = */ true,
		"external/bazelbuild-rules_license":/* recursive = */ true,
		"external/bazelbuild-rules_go":/* recursive = */ true,
		"external/bazelbuild-rules_python":/* recursive = */ true,
		"external/bazelbuild-rules_rust":/* recursive = */ true,
		"external/bazelbuild-rules_testing":/* recursive = */ true,
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
		"prebuilts/jdk/jdk8":/* recursive = */ true,
		"prebuilts/jdk/jdk17":/* recursive = */ true,
		"prebuilts/misc":/* recursive = */ false, // not recursive because we need bp2build converted build files in prebuilts/misc/common/asm
		"prebuilts/sdk":/* recursive = */ false,
		"prebuilts/sdk/tools":/* recursive = */ false,
		"prebuilts/r8":/* recursive = */ false,
		"prebuilts/runtime":/* recursive = */ false,
		"prebuilts/rust":/* recursive = */ true,

		// not recursive due to conflicting workspace paths in tools/atest/bazel/rules
		"tools/asuite/atest":/* recursive = */ false,
		"tools/asuite/atest/bazel/reporter":/* recursive = */ true,

		// Used for testing purposes only. Should not actually exist in the real source tree.
		"testpkg/keep_build_file":/* recursive = */ false,
	}

	Bp2buildModuleAlwaysConvertList = []string{
		"aconfig.test.cpp",
		"AconfigJavaHostTest",
		// aconfig
		"libonce_cell",
		"libanyhow",
		"libunicode_segmentation",
		"libmemchr",
		"libbitflags-1.3.2",
		"libryu",
		"libitoa",
		"libos_str_bytes",
		"libheck",
		"libclap_lex",
		"libsyn",
		"libquote",
		"libunicode_ident",
		"libproc_macro2",
		"libthiserror_impl",
		"libserde_derive",
		"libclap_derive",
		"libthiserror",
		"libserde",
		"libclap",
		"libbytes",
		"libprotobuf_support",
		"libtinytemplate",
		"libserde_json",
		"libprotobuf",

		"protoc-gen-rust",
		"libprotobuf_codegen",
		"libprotobuf_parse",
		"libregex",
		"libtempfile",
		"libwhich",
		"libregex_syntax",
		"libfastrand",
		"libeither",
		"libaho_corasick",
		"liblibc",
		"libcfg_if",
		"liblog_rust",
		"libgetrandom",
		"libremove_dir_all",
		"libahash",
		"libhashbrown",
		"libindexmap",
		"libaconfig_protos",
		"libpaste",
		"aconfig",

		// ext
		"tagsoup",

		// framework-minus-apex
		"AndroidFrameworkLintChecker",
		"ImmutabilityAnnotationProcessor",
		"debian.mime.types.minimized",
		"framework-javastream-protos",
		"libview-inspector-annotation-processor",

		// services
		"apache-commons-math",
		"cbor-java",
		"icu4j_calendar_astronomer",
		"statslog-art-java-gen",

		"AndroidCommonLint",
		"ImmutabilityAnnotation",
		"ImmutabilityAnnotationProcessorHostLibrary",

		"libidmap2_policies",
		"libSurfaceFlingerProp",
		"toolbox_input_labels",

		// cc mainline modules

		// com.android.media.swcodec
		"com.android.media.swcodec",
		"com.android.media.swcodec-androidManifest",
		"com.android.media.swcodec-ld.config.txt",
		"com.android.media.swcodec-mediaswcodec.32rc",
		"com.android.media.swcodec-mediaswcodec.rc",
		"com.android.media.swcodec.certificate",
		"com.android.media.swcodec.key",
		"test_com.android.media.swcodec",

		// deps
		"code_coverage.policy",
		"code_coverage.policy.other",
		"codec2_soft_exports",
		"compatibility_matrix_schema",
		"framework-connectivity-protos",
		"framework-connectivity-javastream-protos",
		"gemmlowp_headers",
		"gl_headers",
		"libandroid_runtime_lazy",
		"libandroid_runtime_vm_headers",
		"libaudioclient_aidl_conversion_util",
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
		"libsurfaceflinger_headers",
		"libsync",
		"libtextclassifier_hash_headers",
		"libtextclassifier_hash_static",
		"libtflite_kernel_utils",
		"libtinyxml2",
		"libvorbisidec",
		"media_ndk_headers",
		"media_plugin_headers",
		"mediaswcodec.policy",
		"mediaswcodec.xml",
		"neuralnetworks_types",
		"libneuralnetworks_common",
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
		"prebuilt_aapt2",

		// fastboot
		"fastboot",
		"libfastboot",

		"PluginCoreLib",
		"dagger2",
		"dagger2-android-annotation-stubs",
		"dagger2-bootstrap-compiler",
		"dagger2-producers",
		"okio-lib",
		"setupdesign-strings",

		//external/avb
		"avbtool",
		"libavb",
		"avb_headers",

		//external/libxml2
		"xmllint",
		"libxml2",

		//external/fec
		"libfec_rs",

		//system/extras/ext4_utils
		"libext4_utils",
		"mke2fs_conf",
		"mkuserimg_mke2fs",
		"blk_alloc_to_base_fs",

		//system/extras/libfec
		"libfec",

		//system/extras/squashfs_utils
		"libsquashfs_utils",

		//packages/apps/Car/libs/car-ui-lib/car-ui-androidx
		// genrule dependencies for java_imports
		"car-ui-androidx-annotation-nodeps",
		"car-ui-androidx-collection-nodeps",
		"car-ui-androidx-core-common-nodeps",
		"car-ui-androidx-lifecycle-common-nodeps",
		"car-ui-androidx-constraintlayout-solver-nodeps",

		//frameworks/native/libs/input
		"inputconstants_aidl",

		// needed for aidl_interface's ndk backend
		"libbinder_ndk",

		"libusb",

		//frameworks/native/cmds/cmd
		"libcmd",

		//system/chre
		"chre_api",

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

		//bootable/recovery/fuse_sideload
		"libfusesideload",

		"libcodec2_aidl",
		"libcodec2_hidl@1.0",
		"libcodec2_hidl@1.1",
		"libcodec2_hidl@1.2",
		"libcodec2_hidl_plugin_stub",
		"libcodec2_hidl_plugin",
		"libcodec2_hal_common",
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
		"libcodec2_soft_av1dec_dav1d",
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

		// for building com.android.neuralnetworks
		"libimapper_stablec",
		"libimapper_providerutils",

		// min_sdk_version in android_app
		"CtsShimUpgrade",

		"art_cmdlineparser_headers",

		// Mainline Module Apps
		"CaptivePortalLogin",
		"ModuleMetadata",

		"libstagefright_headers",

		// Apps with JNI libs
		"SimpleJNI",
		"libsimplejni",

		// aidl
		"aidl",
		"libaidl-common",

		// Used by xsd_config
		"xsdc",

		// cc_test that can be run by b test
		"binderRpcWireProtocolTest",
		"binderUnitTest",
		"cpu_features-bit_utils_test",
		"android.hardware.audio.common.test.utility_tests",
		"HalAudioStreamWorkerTest",
		"libjavacore-unit-tests",
		"NeuralNetworksTest_utils",
		"NeuralNetworksTest_logtag",
		"NeuralNetworksTest_operations",
		"nanoapp_chqts_shared_tests",
		"fakeservicemanager_test",
		"tristate_test",
		"binderUtilsHostTest",
		"run_dex2oat_test",
		"bluetooth-address-unit-tests",

		// for platform_compat_config
		"process-compat-config",

		// cc_* modules with rscript srcs
		"rstest-latency",
		"libRScpp_static",
		"rs-headers",
		"rs_script_api",
		"libRSDispatch",

		// hal_unit_tests and deps
		"chre_flatbuffers",
		"event_logger",
		"hal_unit_tests",

		"merge_annotation_zips_test",

		// java_resources with multiple resource_dirs
		"emma",

		// NDK STL
		"ndk_libc++abi",
		"ndk_libunwind",
		"ndk_libc++_static",
		"ndk_libc++_shared",
		"ndk_system",

		// allowlist //prebuilts/common/misc/androidx-test/...
		"androidx.test.runner",
		"androidx.test.runner-nodeps",
		"androidx.test.services.storage",
		"androidx.test.services.storage-nodeps",
		"androidx.test.monitor",
		"androidx.test.monitor-nodeps",
		"androidx.test.annotation",
		"androidx.test.annotation-nodeps",

		// jni deps of an internal android_test (b/297405812)
		"libopenjdkjvmti_headers",

		// tradefed deps
		"apache-commons-compress",
		"tradefed-protos",
		"grpc-java",
		"grpc-java-api",
		"grpc-java-auth",
		"grpc-java-context",
		"grpc-java-core",
		"grpc-java-core-inprocess",
		"grpc-java-core-internal",
		"grpc-java-core-util",
		"grpc-java-protobuf",
		"grpc-java-protobuf-lite",
		"grpc-java-stub",
		"grpc-java-annotation-stubs",
		"grpc-java-annotation-stubs-srcjar",
		"gen_annotations",
		"opencensus-java-contrib-grpc-metrics",
		"opencensus-java-api",
		"gson",
		"GsonBuildConfig.java",
		"gson_version_generator",
		"lab-resource-grpc",
		"blueprint-deptools",
		"protoc-gen-grpc-java-plugin",
		"tf-remote-client",
		"tradefed-lite",
		"tradefed-isolation-protos",
		"snakeyaml_patched_src_files",
		"asuite_proto_java",
		"tradefed-service-grpc-lib",
		"tradefed-invocation-grpc",
		"tradefed-external-dependencies",
		"tradefed-dynamic-sharding-grpc",
		"tradefed-device-manager-grpc",
		"statsd_internal_protos",
		"snakeyaml",
		"loganalysis",
		"junit-params",
		"grpc-java-testing",
		"grpc-java-netty-shaded",
		"aoa-helper",
		"test-services.apk",
		"test-composers",
		"py3-stdlib-prebuilt-srcs",
		"platformprotos",
		"test-services-normalized.apk",
		"tradefed-common-util",
		"tradefed-clearcut-client",
		"tradefed-result-interfaces",
		"tradefed-device-build-interfaces",
		"tradefed-invocation-interfaces",
		"tradefed-lib-core",

		"libandroid_net_connectivity_com_android_net_module_util_jni",
		"libservice-connectivity",

		"mainline_modules_sdks_test",

		"fake_device_config",
	}

	Bp2buildModuleTypeAlwaysConvertList = []string{
		// go/keep-sorted start
		"aconfig_declarations",
		"aconfig_value_set",
		"aconfig_values",
		"aidl_interface_headers",
		"bpf",
		"cc_aconfig_library",
		"cc_prebuilt_library",
		"cc_prebuilt_library_headers",
		"cc_prebuilt_library_shared",
		"cc_prebuilt_library_static",
		"combined_apis",
		"droiddoc_exported_dir",
		"java_aconfig_library",
		"java_import",
		"java_import_host",
		"java_sdk_library",
		"java_sdk_library_import",
		"license",
		"linker_config",
		"ndk_headers",
		"ndk_library",
		"sysprop_library",
		"versioned_ndk_headers",
		"xsd_config",
		// go/keep-sorted end
	}

	// Add the names of modules that bp2build should never convert, if it is
	// in the package allowlist.  An error will be thrown if a module must
	// not be here and in the alwaysConvert lists.
	//
	// For prebuilt modules (e.g. android_library_import), remember to add
	// the "prebuilt_" prefix to the name, so that it's differentiable from
	// the source versions within Soong's module graph.
	Bp2buildModuleDoNotConvertList = []string{

		// rust modules that have cc deps
		"liblogger",
		"libbssl_ffi",
		"libbssl_ffi_nostd",
		"pull_rust",
		"libstatslog_rust",
		"libstatslog_rust_header",
		"libflatbuffers",
		"liblog_event_list",
		"libminijail_rust",
		"libminijail_sys",
		"libfsverity_rs",
		"libtombstoned_client_rust",

		// TODO(b/263326760): Failed already.
		"minijail_compiler_unittest",
		"minijail_parser_unittest",

		// cc bugs

		// TODO(b/198619163) module has same name as source
		"logtagd.rc",

		"libgtest_ndk_c++", "libgtest_main_ndk_c++", // TODO(b/201816222): Requires sdk_version support.

		// TODO(b/202876379): has arch-variant static_executable
		"linkerconfig",
		"mdnsd",
		"libcutils_test_static",
		"KernelLibcutilsTest",

		"linker",    // TODO(b/228316882): cc_binary uses link_crt
		"versioner", // TODO(b/228313961):  depends on prebuilt shared library libclang-cpp_host as a shared library, which does not supply expected providers for a shared library

		// requires host tools for apexer
		"apexer_test", "apexer_test_host_tools", "host_apex_verifier", "host-apex-verifier",

		// java bugs
		"libbase_ndk",           // TODO(b/186826477): fails to link libctscamera2_jni for device (required for CtsCameraTestCases)
		"bouncycastle",          // TODO(b/274474005): Need support for custom system_modules.
		"bouncycastle-test-lib", // TODO(b/274474005): Reverse dependency of bouncycastle

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

		// go deps.
		// TODO: b/305091740 - Rely on bp2build_deps to remove these dependencies.
		"analyze_bcpf",              // depends on bpmodify a blueprint_go_binary.
		"analyze_bcpf_test",         // depends on bpmodify a blueprint_go_binary.
		"host_bionic_linker_asm",    // depends on extract_linker, a go binary.
		"host_bionic_linker_script", // depends on extract_linker, a go binary.

		// rust support
		"libtombstoned_client_rust_bridge_code", "libtombstoned_client_wrapper", // rust conversions are not supported

		// TODO: b/303474748 - aidl rules for java are incompatible with parcelable declarations
		"modules-utils-list-slice",
		"modules-utils-os",
		"modules-utils-synchronous-result-receiver",

		// aidl files not created
		"overlayable_policy_aidl_interface",

		// cc_test related.
		// b/274164834 "Could not open Configuration file test.cfg"
		"svcenc", "svcdec",

		// Failing host cc_tests
		"gtest_isolated_tests",
		"libunwindstack_unit_test",
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

		"libandroidfw_tests", // failing due to data path issues

		// error: overriding commands for target
		// `out/host/linux-x86/nativetest64/gmock_tests/gmock_tests__cc_runner_test',
		// previously defined at out/soong/installs-aosp_arm.mk:64919`
		"gmock_tests",

		// cc_test with unconverted deps, or are device-only (and not verified to pass yet)
		"AMRWBEncTest",
		"avcdec",
		"avcenc",
		"boringssl_test_support", //b/244431896
		"cfi_test_helper",
		"cintltst32",
		"cintltst64",
		"compare",
		"cpuid",
		"elftls_dlopen_ie_error_helper",
		"fdtrack_test",
		"google-benchmark-test",
		"gtest-typed-test_test",
		"gtest_tests",
		"gtest_tests_no_main",
		"gwp_asan_unittest",
		"half_test",
		"hashcombine_test",
		"hevcdec",
		"hevcenc",
		"i444tonv12_eg",
		"icu4c_sample_break",
		"intltest32",
		"intltest64",
		"ion-unit-tests",
		"jemalloc5_integrationtests",
		"jemalloc5_unittests",
		"jemalloc5_stresstests", // run by run_jemalloc_tests.sh and will be deleted after V
		"libcutils_sockets_test",
		"libhwbinder_latency",
		"liblog-host-test", // failing tests
		"libminijail_test",
		"libminijail_unittest_gtest",
		"libpackagelistparser_test",
		"libprotobuf_vendor_suffix_test",
		"libstagefright_amrnbenc_test",
		"libstagefright_amrwbdec_test", // error: did not report any run
		"libstagefright_m4vh263enc_test",
		"libvndksupport-tests",
		"libyuv_unittest",
		"linker-unit-tests",
		"malloc_debug_system_tests",
		"malloc_debug_unit_tests",
		"malloc_hooks_system_tests",
		"mat_test",
		"mathtest",
		"memunreachable_test",
		"metadata_tests",
		"mpeg2dec",
		"mvcdec",
		"pngtest",
		"preinit_getauxval_test_helper",
		"preinit_syscall_test_helper",
		"psnr",
		"quat_test",
		"scudo_unit_tests",
		"thread_exit_cb_helper",
		"ulp",
		"vec_test",
		"yuvconstants",
		"yuvconvert",

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
		"libcrypto_fuzz_unsafe",
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
		"libssl_fuzz_unsafe",
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

		// releasetools
		"verity_utils",
		"check_ota_package_signature",
		"check_target_files_vintf",
		"releasetools_check_target_files_vintf",
		"ota_from_target_files",
		"releasetools_ota_from_target_files",
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
		"hidl_test",

		// TODO(B/283193845): Remove tradefed from this list.
		"tradefed",

		"libprotobuf-full-test", // TODO(b/246997908): cannot convert proto_libraries which implicitly include other srcs in the same directory
		"libprotobuf-lite-test", // TODO(b/246997908): cannot convert proto_libraries which implicitly include other srcs in the same directory

		"logcat", // TODO(b/246997908): cannot convert proto_libraries which implicitly include other srcs in the same directory

		"expresscatalogvalidator", // TODO(b/246997908): cannot convert proto_libraries which implicitly include other srcs in the same directory

		// r8 is a java_binary, which creates an implicit "r8.jar" target, but the
		// same package contains a "r8.jar" file which gets overshadowed by the implicit target.
		// We don't need this target as we're not using the Soong wrapper for now
		"r8",

		// TODO(b/299924782): Fix linking error
		"libbinder_on_trusty_mock",

		// TODO(b/299943581): Depends on aidl filegroups with implicit headers
		"libdataloader_aidl-cpp",
		"libincremental_manager_aidl-cpp",

		// TODO(b/299974637) Fix linking error
		"libbinder_rpc_unstable",

		// TODO(b/297356704) sdk_version is unset.
		"VendorAtomCodeGenJavaTest",

		// TODO: b/305223367 - Missing dep on android.test.base-neverlink
		"ObjenesisTck",

		// TODO - b/306197073: Sets different STL for host and device variants
		"trace_processor_shell",

		// TODO - b/303713102: duplicate deps added by cc_lite_proto_library
		"perfetto_unittests",
		"perfetto_integrationtests",

		// TODO - b/306194966: Depends on an empty filegroup
		"libperfetto_c",
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
		// M11: neuralnetworks launch
		"com.android.neuralnetworks",
		"test_com.android.neuralnetworks",
		"libneuralnetworks",
		"libneuralnetworks_static",
		// M13: media.swcodec launch
		// TODO(b/307389608) Relaunch swcodec after fixing rust dependencies
		// "com.android.media.swcodec",
		// "test_com.android.media.swcodec",
		// "libstagefright_foundation",
		// "libcodec2_hidl@1.0",
	}

	// Staging-mode allowlist. Modules in this list are only built
	// by Bazel with --bazel-mode-staging. This list should contain modules
	// which will soon be added to the prod allowlist.
	// It is implicit that all modules in ProdMixedBuildsEnabledList will
	// also be built - do not add them to this list.
	StagingMixedBuildsEnabledList = []string{}

	// These should be the libs that are included by the apexes in the ProdMixedBuildsEnabledList
	ProdDclaMixedBuildsEnabledList = []string{
		"libbase",
		"libc++",
		"libcrypto",
		"libcutils",
		"libstagefright_flacdec",
		"libutils",
	}

	// These should be the libs that are included by the apexes in the StagingMixedBuildsEnabledList
	StagingDclaMixedBuildsEnabledList = []string{}

	// TODO(b/269342245): Enable the rest of the DCLA libs
	// "libssl",

	// The list of module types which are expected to spend lots of build time.
	// With `--ninja_weight_source=soong`, ninja builds these module types and deps first.
	HugeModuleTypePrefixMap = map[string]int{
		"rust_":       HIGH_PRIORITIZED_WEIGHT,
		"droidstubs":  DEFAULT_PRIORITIZED_WEIGHT,
		"art_":        DEFAULT_PRIORITIZED_WEIGHT,
		"ndk_library": DEFAULT_PRIORITIZED_WEIGHT,
	}
)
