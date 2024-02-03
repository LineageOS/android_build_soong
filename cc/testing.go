// Copyright (C) 2019 The Android Open Source Project
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

package cc

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"android/soong/android"
	"android/soong/genrule"
	"android/soong/multitree"
	"android/soong/snapshot"
)

func RegisterRequiredBuildComponentsForTest(ctx android.RegistrationContext) {
	RegisterPrebuiltBuildComponents(ctx)
	RegisterCCBuildComponents(ctx)
	RegisterBinaryBuildComponents(ctx)
	RegisterLibraryBuildComponents(ctx)
	RegisterLibraryHeadersBuildComponents(ctx)
	RegisterLibraryStubBuildComponents(ctx)

	multitree.RegisterApiImportsModule(ctx)

	ctx.RegisterModuleType("prebuilt_build_tool", android.NewPrebuiltBuildTool)
	ctx.RegisterModuleType("cc_benchmark", BenchmarkFactory)
	ctx.RegisterModuleType("cc_object", ObjectFactory)
	ctx.RegisterModuleType("cc_genrule", GenRuleFactory)
	ctx.RegisterModuleType("ndk_prebuilt_shared_stl", NdkPrebuiltSharedStlFactory)
	ctx.RegisterModuleType("ndk_prebuilt_static_stl", NdkPrebuiltStaticStlFactory)
	ctx.RegisterModuleType("ndk_library", NdkLibraryFactory)
	ctx.RegisterModuleType("ndk_headers", NdkHeadersFactory)
}

func GatherRequiredDepsForTest(oses ...android.OsType) string {
	ret := commonDefaultModules()

	supportLinuxBionic := false
	for _, os := range oses {
		if os == android.Windows {
			ret += withWindowsModules()
		}
		if os == android.LinuxBionic {
			supportLinuxBionic = true
			ret += withLinuxBionic()
		}
	}

	if !supportLinuxBionic {
		ret += withoutLinuxBionic()
	}

	return ret
}

func commonDefaultModules() string {
	return `
		cc_defaults {
			name: "toolchain_libs_defaults",
			host_supported: true,
			vendor_available: true,
			product_available: true,
			recovery_available: true,
			no_libcrt: true,
			sdk_version: "minimum",
			nocrt: true,
			no_crt_pad_segment: true,
			system_shared_libs: [],
			stl: "none",
			check_elf_files: false,
			sanitize: {
				never: true,
			},
			apex_available: [
				"//apex_available:anyapex",
				"//apex_available:platform",
			],
		}

		cc_prebuilt_library_static {
			name: "libcompiler_rt-extras",
			defaults: ["toolchain_libs_defaults"],
			vendor_ramdisk_available: true,
			srcs: [""],
		}

		cc_prebuilt_library_static {
			name: "libclang_rt.builtins",
			defaults: ["toolchain_libs_defaults"],
			host_supported: true,
	        vendor_available: true,
			vendor_ramdisk_available: true,
			native_bridge_supported: true,
			srcs: [""],
		}

		cc_prebuilt_library_shared {
			name: "libclang_rt.hwasan",
			defaults: ["toolchain_libs_defaults"],
			srcs: [""],
		}

		cc_prebuilt_library_static {
			name: "libunwind",
			defaults: [
				"linux_bionic_supported",
				"toolchain_libs_defaults",
			],
			vendor_ramdisk_available: true,
			native_bridge_supported: true,
			srcs: [""],
		}

		cc_prebuilt_library_static {
			name: "libclang_rt.fuzzer",
			defaults: [
				"linux_bionic_supported",
				"toolchain_libs_defaults",
			],
			srcs: [""],
		}

		// Needed for sanitizer
		cc_prebuilt_library_shared {
			name: "libclang_rt.ubsan_standalone",
			defaults: ["toolchain_libs_defaults"],
			srcs: [""],
		}

		cc_prebuilt_library_static {
			name: "libclang_rt.ubsan_standalone.static",
			defaults: ["toolchain_libs_defaults"],
			srcs: [""],
		}

		cc_prebuilt_library_static {
			name: "libclang_rt.ubsan_minimal",
			defaults: ["toolchain_libs_defaults"],
			host_supported: true,
			target: {
				android_arm64: {
					srcs: ["libclang_rt.ubsan_minimal.android_arm64.a"],
				},
				android_arm: {
					srcs: ["libclang_rt.ubsan_minimal.android_arm.a"],
				},
				linux_glibc_x86_64: {
					srcs: ["libclang_rt.ubsan_minimal.x86_64.a"],
				},
				linux_glibc_x86: {
					srcs: ["libclang_rt.ubsan_minimal.x86.a"],
				},
				linux_musl_x86_64: {
					srcs: ["libclang_rt.ubsan_minimal.x86_64.a"],
				},
				linux_musl_x86: {
					srcs: ["libclang_rt.ubsan_minimal.x86.a"],
				},
			},
		}

		cc_library {
			name: "libc",
			defaults: ["linux_bionic_supported"],
			no_libcrt: true,
			nocrt: true,
			stl: "none",
			system_shared_libs: [],
			recovery_available: true,
			stubs: {
				versions: ["27", "28", "29"],
			},
			llndk: {
				symbol_file: "libc.map.txt",
			},
		}
		cc_library {
			name: "libm",
			defaults: ["linux_bionic_supported"],
			no_libcrt: true,
			nocrt: true,
			stl: "none",
			system_shared_libs: [],
			recovery_available: true,
			stubs: {
				versions: ["27", "28", "29"],
			},
			apex_available: [
				"//apex_available:platform",
				"myapex"
			],
			llndk: {
				symbol_file: "libm.map.txt",
			},
		}

		// Coverage libraries
		cc_library {
			name: "libprofile-extras",
			vendor_available: true,
			vendor_ramdisk_available: true,
			product_available: true,
			recovery_available: true,
			native_coverage: false,
			system_shared_libs: [],
			stl: "none",
		}
		cc_library {
			name: "libprofile-clang-extras",
			vendor_available: true,
			vendor_ramdisk_available: true,
			product_available: true,
			recovery_available: true,
			native_coverage: false,
			system_shared_libs: [],
			stl: "none",
		}
		cc_library {
			name: "libprofile-extras_ndk",
			vendor_available: true,
			product_available: true,
			native_coverage: false,
			system_shared_libs: [],
			stl: "none",
			sdk_version: "current",
		}
		cc_library {
			name: "libprofile-clang-extras_ndk",
			vendor_available: true,
			product_available: true,
			native_coverage: false,
			system_shared_libs: [],
			stl: "none",
			sdk_version: "current",
		}

		cc_library {
			name: "libdl",
			defaults: ["linux_bionic_supported"],
			no_libcrt: true,
			nocrt: true,
			stl: "none",
			system_shared_libs: [],
			recovery_available: true,
			stubs: {
				versions: ["27", "28", "29"],
			},
			apex_available: [
				"//apex_available:platform",
				"myapex"
			],
			llndk: {
				symbol_file: "libdl.map.txt",
			},
		}
		cc_library {
			name: "libft2",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			recovery_available: true,
			llndk: {
				symbol_file: "libft2.map.txt",
				private: true,
			}
		}
		cc_library {
			name: "libc++_static",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			vendor_available: true,
			vendor_ramdisk_available: true,
			product_available: true,
			recovery_available: true,
			host_supported: true,
			min_sdk_version: "29",
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}
		cc_library {
			name: "libc++",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			vendor_available: true,
			product_available: true,
			recovery_available: true,
			host_supported: true,
			min_sdk_version: "29",
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}
		cc_library {
			name: "libc++demangle",
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
			host_supported: false,
			vendor_available: true,
			vendor_ramdisk_available: true,
			product_available: true,
			recovery_available: true,
			min_sdk_version: "29",
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}

		cc_defaults {
			name: "crt_defaults",
			defaults: ["linux_bionic_supported"],
			recovery_available: true,
			vendor_available: true,
			vendor_ramdisk_available: true,
			product_available: true,
			native_bridge_supported: true,
			stl: "none",
			min_sdk_version: "16",
			crt: true,
			system_shared_libs: [],
			apex_available: [
				"//apex_available:platform",
				"//apex_available:anyapex",
			],
		}

		cc_object {
			name: "crtbegin_so",
			defaults: ["crt_defaults"],
			srcs: ["crtbegin_so.c"],
			objs: ["crtbrand"],
		}

		cc_object {
			name: "crtbegin_dynamic",
			defaults: ["crt_defaults"],
			srcs: ["crtbegin.c"],
			objs: ["crtbrand"],
		}

		cc_object {
			name: "crtbegin_static",
			defaults: ["crt_defaults"],
			srcs: ["crtbegin.c"],
			objs: ["crtbrand"],
		}

		cc_object {
			name: "crtend_so",
			defaults: ["crt_defaults"],
			srcs: ["crtend_so.c"],
			objs: ["crtbrand"],
		}

		cc_object {
			name: "crtend_android",
			defaults: ["crt_defaults"],
			srcs: ["crtend.c"],
			objs: ["crtbrand"],
		}

		cc_object {
			name: "crt_pad_segment",
			defaults: ["crt_defaults"],
		}

		cc_object {
			name: "crtbrand",
			defaults: ["crt_defaults"],
			srcs: ["crtbrand.c"],
		}

		cc_library {
			name: "libprotobuf-cpp-lite",
		}

		cc_library {
			name: "ndk_libunwind",
			sdk_version: "minimum",
			stl: "none",
			system_shared_libs: [],
		}

		ndk_library {
			name: "libc",
			first_version: "minimum",
			symbol_file: "libc.map.txt",
		}

		ndk_library {
			name: "libm",
			first_version: "minimum",
			symbol_file: "libm.map.txt",
		}

		ndk_library {
			name: "libdl",
			first_version: "minimum",
			symbol_file: "libdl.map.txt",
		}

		ndk_prebuilt_shared_stl {
			name: "ndk_libc++_shared",
			export_include_dirs: ["ndk_libc++_shared"],
		}

		cc_library_static {
			name: "libgoogle-benchmark",
			sdk_version: "current",
			stl: "none",
			system_shared_libs: [],
		}

		cc_library_static {
			name: "note_memtag_heap_async",
		}

		cc_library_static {
			name: "note_memtag_heap_sync",
		}

		cc_library {
			name: "libc_musl",
			host_supported: true,
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			stl: "none",
		}
	`
}

func withWindowsModules() string {
	return `
		cc_prebuilt_library_static {
			name: "libwinpthread",
			host_supported: true,
			enabled: false,
			target: {
				windows: {
					enabled: true,
				},
			},
			stl: "none",
			srcs:[""],
		}
		`
}

func withLinuxBionic() string {
	return `
				cc_binary {
					name: "linker",
					defaults: ["linux_bionic_supported"],
					recovery_available: true,
					stl: "none",
					nocrt: true,
					static_executable: true,
					native_coverage: false,
					system_shared_libs: [],
				}

				cc_genrule {
					name: "host_bionic_linker_script",
					host_supported: true,
					device_supported: false,
					target: {
						host: {
							enabled: false,
						},
						linux_bionic: {
							enabled: true,
						},
					},
					out: ["linker.script"],
				}

				cc_defaults {
					name: "linux_bionic_supported",
					host_supported: true,
					target: {
						host: {
							enabled: false,
						},
						linux_bionic: {
							enabled: true,
						},
					},
				}
			`
}

func withoutLinuxBionic() string {
	return `
			cc_defaults {
				name: "linux_bionic_supported",
			}
		`
}

func GatherRequiredFilesForTest(fs map[string][]byte) {
}

// The directory in which cc linux bionic default modules will be defined.
//
// Placing them here ensures that their location does not conflict with default test modules
// defined by other packages.
const linuxBionicDefaultsPath = "defaults/cc/linux-bionic/Android.bp"

// The directory in which the default cc common test modules will be defined.
//
// Placing them here ensures that their location does not conflict with default test modules
// defined by other packages.
const DefaultCcCommonTestModulesDir = "defaults/cc/common/"

// Test fixture preparer that will register most cc build components.
//
// Singletons and mutators should only be added here if they are needed for a majority of cc
// module types, otherwise they should be added under a separate preparer to allow them to be
// selected only when needed to reduce test execution time.
//
// Module types do not have much of an overhead unless they are used so this should include as many
// module types as possible. The exceptions are those module types that require mutators and/or
// singletons in order to function in which case they should be kept together in a separate
// preparer.
var PrepareForTestWithCcBuildComponents = android.GroupFixturePreparers(
	android.PrepareForTestWithAndroidBuildComponents,
	android.FixtureRegisterWithContext(RegisterRequiredBuildComponentsForTest),
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("cc_fuzz", LibFuzzFactory)
		ctx.RegisterModuleType("cc_test", TestFactory)
		ctx.RegisterModuleType("cc_test_library", TestLibraryFactory)
		ctx.RegisterModuleType("vndk_prebuilt_shared", VndkPrebuiltSharedFactory)

		RegisterVndkLibraryTxtTypes(ctx)
	}),

	// Additional files needed in tests that disallow non-existent source files.
	// This includes files that are needed by all, or at least most, instances of a cc module type.
	android.MockFS{
		// Needed for ndk_prebuilt_(shared|static)_stl.
		"defaults/cc/common/current/sources/cxx-stl/llvm-libc++/libs": nil,
	}.AddToFixture(),
)

// Preparer that will define default cc modules, e.g. standard prebuilt modules.
var PrepareForTestWithCcDefaultModules = android.GroupFixturePreparers(
	PrepareForTestWithCcBuildComponents,

	// Additional files needed in tests that disallow non-existent source.
	android.MockFS{
		"defaults/cc/common/libc.map.txt":      nil,
		"defaults/cc/common/libdl.map.txt":     nil,
		"defaults/cc/common/libft2.map.txt":    nil,
		"defaults/cc/common/libm.map.txt":      nil,
		"defaults/cc/common/ndk_libc++_shared": nil,
		"defaults/cc/common/crtbegin_so.c":     nil,
		"defaults/cc/common/crtbegin.c":        nil,
		"defaults/cc/common/crtend_so.c":       nil,
		"defaults/cc/common/crtend.c":          nil,
		"defaults/cc/common/crtbrand.c":        nil,

		"defaults/cc/common/libclang_rt.ubsan_minimal.android_arm64.a": nil,
		"defaults/cc/common/libclang_rt.ubsan_minimal.android_arm.a":   nil,
		"defaults/cc/common/libclang_rt.ubsan_minimal.x86_64.a":        nil,
		"defaults/cc/common/libclang_rt.ubsan_minimal.x86.a":           nil,
	}.AddToFixture(),

	// Place the default cc test modules that are common to all platforms in a location that will not
	// conflict with default test modules defined by other packages.
	android.FixtureAddTextFile(DefaultCcCommonTestModulesDir+"Android.bp", commonDefaultModules()),
	// Disable linux bionic by default.
	android.FixtureAddTextFile(linuxBionicDefaultsPath, withoutLinuxBionic()),
)

// Prepare a fixture to use all cc module types, mutators and singletons fully.
//
// This should only be used by tests that want to run with as much of the build enabled as possible.
var PrepareForIntegrationTestWithCc = android.GroupFixturePreparers(
	android.PrepareForIntegrationTestWithAndroid,
	genrule.PrepareForIntegrationTestWithGenrule,
	PrepareForTestWithCcDefaultModules,
)

// The preparer to include if running a cc related test for windows.
var PrepareForTestOnWindows = android.GroupFixturePreparers(
	// Place the default cc test modules for windows platforms in a location that will not conflict
	// with default test modules defined by other packages.
	android.FixtureAddTextFile("defaults/cc/windows/Android.bp", withWindowsModules()),
)

// The preparer to include if running a cc related test for linux bionic.
var PrepareForTestOnLinuxBionic = android.GroupFixturePreparers(
	// Enable linux bionic
	//
	// Can be used after PrepareForTestWithCcDefaultModules to override its default behavior of
	// disabling linux bionic, hence why this uses FixtureOverrideTextFile.
	android.FixtureOverrideTextFile(linuxBionicDefaultsPath, withLinuxBionic()),
)

// This adds some additional modules and singletons which might negatively impact the performance
// of tests so they are not included in the PrepareForIntegrationTestWithCc.
var PrepareForTestWithCcIncludeVndk = android.GroupFixturePreparers(
	PrepareForIntegrationTestWithCc,
	android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
		snapshot.VendorSnapshotImageSingleton.Init(ctx)
		snapshot.RecoverySnapshotImageSingleton.Init(ctx)
		RegisterVendorSnapshotModules(ctx)
		RegisterRecoverySnapshotModules(ctx)
		ctx.RegisterSingletonType("vndk-snapshot", VndkSnapshotSingleton)
	}),
)

// PrepareForTestWithHostMusl sets the host configuration to musl libc instead of glibc.  It also disables the test
// on mac, which doesn't support musl libc, and adds musl modules.
var PrepareForTestWithHostMusl = android.GroupFixturePreparers(
	android.FixtureModifyConfig(android.ModifyTestConfigForMusl),
	android.PrepareForSkipTestOnMac,
	android.FixtureAddTextFile("external/musl/Android.bp", `
		cc_defaults {
			name: "libc_musl_crt_defaults",
			host_supported: true,
			device_supported: false,
		}

		cc_object {
			name: "libc_musl_crtbegin_so",
			defaults: ["libc_musl_crt_defaults"],
		}

		cc_object {
			name: "libc_musl_crtend_so",
			defaults: ["libc_musl_crt_defaults"],
		}

		cc_object {
			name: "libc_musl_crtbegin_dynamic",
			defaults: ["libc_musl_crt_defaults"],
		}

		cc_object {
			name: "libc_musl_crtbegin_static",
			defaults: ["libc_musl_crt_defaults"],
		}

		cc_object {
			name: "libc_musl_crtend",
			defaults: ["libc_musl_crt_defaults"],
		}
	`),
)

// PrepareForTestWithFdoProfile registers module types to test with fdo_profile
var PrepareForTestWithFdoProfile = android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("soong_namespace", android.NamespaceFactory)
	ctx.RegisterModuleType("fdo_profile", FdoProfileFactory)
})

// TestConfig is the legacy way of creating a test Config for testing cc modules.
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func TestConfig(buildDir string, os android.OsType, env map[string]string,
	bp string, fs map[string][]byte) android.Config {

	// add some modules that are required by the compiler and/or linker
	bp = bp + GatherRequiredDepsForTest(os)

	mockFS := map[string][]byte{}

	GatherRequiredFilesForTest(mockFS)

	for k, v := range fs {
		mockFS[k] = v
	}

	return android.TestArchConfig(buildDir, env, bp, mockFS)
}

// CreateTestContext is the legacy way of creating a TestContext for testing cc modules.
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func CreateTestContext(config android.Config) *android.TestContext {
	ctx := android.NewTestArchContext(config)
	genrule.RegisterGenruleBuildComponents(ctx)
	ctx.RegisterModuleType("cc_fuzz", LibFuzzFactory)
	ctx.RegisterModuleType("cc_test", TestFactory)
	ctx.RegisterModuleType("cc_test_library", TestLibraryFactory)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("vndk_prebuilt_shared", VndkPrebuiltSharedFactory)

	snapshot.VendorSnapshotImageSingleton.Init(ctx)
	snapshot.RecoverySnapshotImageSingleton.Init(ctx)
	RegisterVendorSnapshotModules(ctx)
	RegisterRecoverySnapshotModules(ctx)
	ctx.RegisterSingletonType("vndk-snapshot", VndkSnapshotSingleton)
	RegisterVndkLibraryTxtTypes(ctx)

	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	android.RegisterPrebuiltMutators(ctx)
	RegisterRequiredBuildComponentsForTest(ctx)

	return ctx
}

func checkSnapshotIncludeExclude(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, moduleName, snapshotFilename, subDir, variant string, include bool, fake bool) {
	t.Helper()
	mod := ctx.ModuleForTests(moduleName, variant)
	outputFiles := mod.OutputFiles(t, "")
	if len(outputFiles) != 1 {
		t.Errorf("%q must have single output\n", moduleName)
		return
	}
	snapshotPath := filepath.Join(subDir, snapshotFilename)

	if include {
		out := singleton.Output(snapshotPath)
		if fake {
			if out.Rule == nil {
				t.Errorf("Missing rule for module %q output file %q", moduleName, outputFiles[0])
			}
		} else {
			if out.Input.String() != outputFiles[0].String() {
				t.Errorf("The input of snapshot %q must be %q, but %q", moduleName, out.Input.String(), outputFiles[0])
			}
		}
	} else {
		out := singleton.MaybeOutput(snapshotPath)
		if out.Rule != nil {
			t.Errorf("There must be no rule for module %q output file %q", moduleName, outputFiles[0])
		}
	}
}

func CheckSnapshot(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, moduleName, snapshotFilename, subDir, variant string) {
	t.Helper()
	checkSnapshotIncludeExclude(t, ctx, singleton, moduleName, snapshotFilename, subDir, variant, true, false)
}

func CheckSnapshotExclude(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, moduleName, snapshotFilename, subDir, variant string) {
	t.Helper()
	checkSnapshotIncludeExclude(t, ctx, singleton, moduleName, snapshotFilename, subDir, variant, false, false)
}

func CheckSnapshotRule(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, moduleName, snapshotFilename, subDir, variant string) {
	t.Helper()
	checkSnapshotIncludeExclude(t, ctx, singleton, moduleName, snapshotFilename, subDir, variant, true, true)
}

func AssertExcludeFromVendorSnapshotIs(t *testing.T, ctx *android.TestContext, name string, expected bool, variant string) {
	t.Helper()
	m := ctx.ModuleForTests(name, variant).Module().(LinkableInterface)
	if m.ExcludeFromVendorSnapshot() != expected {
		t.Errorf("expected %q ExcludeFromVendorSnapshot to be %t", m.String(), expected)
	}
}

func GetOutputPaths(ctx *android.TestContext, variant string, moduleNames []string) (paths android.Paths) {
	for _, moduleName := range moduleNames {
		module := ctx.ModuleForTests(moduleName, variant).Module().(*Module)
		output := module.outputFile.Path().RelativeToTop()
		paths = append(paths, output)
	}
	return paths
}

func AssertExcludeFromRecoverySnapshotIs(t *testing.T, ctx *android.TestContext, name string, expected bool, variant string) {
	t.Helper()
	m := ctx.ModuleForTests(name, variant).Module().(LinkableInterface)
	if m.ExcludeFromRecoverySnapshot() != expected {
		t.Errorf("expected %q ExcludeFromRecoverySnapshot to be %t", m.String(), expected)
	}
}

func checkOverrides(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, jsonPath string, expected []string) {
	t.Helper()
	out := singleton.MaybeOutput(jsonPath)
	content := android.ContentFromFileRuleForTests(t, ctx, out)

	var flags snapshotJsonFlags
	if err := json.Unmarshal([]byte(content), &flags); err != nil {
		t.Errorf("Error while unmarshalling json %q: %s", jsonPath, err.Error())
		return
	}

	for _, moduleName := range expected {
		if !android.InList(moduleName, flags.Overrides) {
			t.Errorf("expected %q to be in %q: %q", moduleName, flags.Overrides, content)
			return
		}
	}
}
