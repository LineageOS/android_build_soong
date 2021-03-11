// Copyright 2017 Google Inc. All rights reserved.
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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"android/soong/android"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_cc_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}

var ccFixtureFactory = android.NewFixtureFactory(
	&buildDir,
	PrepareForTestWithCcIncludeVndk,

	android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.DeviceVndkVersion = StringPtr("current")
		variables.ProductVndkVersion = StringPtr("current")
		variables.Platform_vndk_version = StringPtr("VER")
	}),
)

// testCcWithConfig runs tests using the ccFixtureFactory
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testCcWithConfig(t *testing.T, config android.Config) *android.TestContext {
	t.Helper()
	result := ccFixtureFactory.RunTestWithConfig(t, config)
	return result.TestContext
}

// testCc runs tests using the ccFixtureFactory
//
// Do not add any new usages of this, instead use the ccFixtureFactory directly as it makes it much
// easier to customize the test behavior.
//
// If it is necessary to customize the behavior of an existing test that uses this then please first
// convert the test to using ccFixtureFactory first and then in a following change add the
// appropriate fixture preparers. Keeping the conversion change separate makes it easy to verify
// that it did not change the test behavior unexpectedly.
//
// deprecated
func testCc(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	result := ccFixtureFactory.RunTestWithBp(t, bp)
	return result.TestContext
}

// testCcNoVndk runs tests using the ccFixtureFactory
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testCcNoVndk(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")

	return testCcWithConfig(t, config)
}

// testCcNoProductVndk runs tests using the ccFixtureFactory
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testCcNoProductVndk(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")

	return testCcWithConfig(t, config)
}

// testCcErrorWithConfig runs tests using the ccFixtureFactory
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testCcErrorWithConfig(t *testing.T, pattern string, config android.Config) {
	t.Helper()

	ccFixtureFactory.Extend().
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(pattern)).
		RunTestWithConfig(t, config)
}

// testCcError runs tests using the ccFixtureFactory
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testCcError(t *testing.T, pattern string, bp string) {
	t.Helper()
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	testCcErrorWithConfig(t, pattern, config)
	return
}

// testCcErrorProductVndk runs tests using the ccFixtureFactory
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testCcErrorProductVndk(t *testing.T, pattern string, bp string) {
	t.Helper()
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.ProductVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	testCcErrorWithConfig(t, pattern, config)
	return
}

const (
	coreVariant     = "android_arm64_armv8-a_shared"
	vendorVariant   = "android_vendor.VER_arm64_armv8-a_shared"
	productVariant  = "android_product.VER_arm64_armv8-a_shared"
	recoveryVariant = "android_recovery_arm64_armv8-a_shared"
)

func TestFuchsiaDeps(t *testing.T) {
	t.Helper()

	bp := `
		cc_library {
			name: "libTest",
			srcs: ["foo.c"],
			target: {
				fuchsia: {
					srcs: ["bar.c"],
				},
			},
		}`

	result := ccFixtureFactory.Extend(PrepareForTestOnFuchsia).RunTestWithBp(t, bp)

	rt := false
	fb := false

	ld := result.ModuleForTests("libTest", "fuchsia_arm64_shared").Rule("ld")
	implicits := ld.Implicits
	for _, lib := range implicits {
		if strings.Contains(lib.Rel(), "libcompiler_rt") {
			rt = true
		}

		if strings.Contains(lib.Rel(), "libbioniccompat") {
			fb = true
		}
	}

	if !rt || !fb {
		t.Errorf("fuchsia libs must link libcompiler_rt and libbioniccompat")
	}
}

func TestFuchsiaTargetDecl(t *testing.T) {
	t.Helper()

	bp := `
		cc_library {
			name: "libTest",
			srcs: ["foo.c"],
			target: {
				fuchsia: {
					srcs: ["bar.c"],
				},
			},
		}`

	result := ccFixtureFactory.Extend(PrepareForTestOnFuchsia).RunTestWithBp(t, bp)
	ld := result.ModuleForTests("libTest", "fuchsia_arm64_shared").Rule("ld")
	var objs []string
	for _, o := range ld.Inputs {
		objs = append(objs, o.Base())
	}
	result.AssertArrayString("libTest inputs", []string{"foo.o", "bar.o"}, objs)
}

func TestVendorSrc(t *testing.T) {
	ctx := testCc(t, `
		cc_library {
			name: "libTest",
			srcs: ["foo.c"],
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			vendor_available: true,
			target: {
				vendor: {
					srcs: ["bar.c"],
				},
			},
		}
	`)

	ld := ctx.ModuleForTests("libTest", vendorVariant).Rule("ld")
	var objs []string
	for _, o := range ld.Inputs {
		objs = append(objs, o.Base())
	}
	if len(objs) != 2 || objs[0] != "foo.o" || objs[1] != "bar.o" {
		t.Errorf("inputs of libTest must be []string{\"foo.o\", \"bar.o\"}, but was %#v.", objs)
	}
}

func checkVndkModule(t *testing.T, ctx *android.TestContext, name, subDir string,
	isVndkSp bool, extends string, variant string) {

	t.Helper()

	mod := ctx.ModuleForTests(name, variant).Module().(*Module)

	// Check library properties.
	lib, ok := mod.compiler.(*libraryDecorator)
	if !ok {
		t.Errorf("%q must have libraryDecorator", name)
	} else if lib.baseInstaller.subDir != subDir {
		t.Errorf("%q must use %q as subdir but it is using %q", name, subDir,
			lib.baseInstaller.subDir)
	}

	// Check VNDK properties.
	if mod.vndkdep == nil {
		t.Fatalf("%q must have `vndkdep`", name)
	}
	if !mod.IsVndk() {
		t.Errorf("%q IsVndk() must equal to true", name)
	}
	if mod.isVndkSp() != isVndkSp {
		t.Errorf("%q isVndkSp() must equal to %t", name, isVndkSp)
	}

	// Check VNDK extension properties.
	isVndkExt := extends != ""
	if mod.IsVndkExt() != isVndkExt {
		t.Errorf("%q IsVndkExt() must equal to %t", name, isVndkExt)
	}

	if actualExtends := mod.getVndkExtendsModuleName(); actualExtends != extends {
		t.Errorf("%q must extend from %q but get %q", name, extends, actualExtends)
	}
}

func checkSnapshotIncludeExclude(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, moduleName, snapshotFilename, subDir, variant string, include bool, fake bool) {
	t.Helper()
	mod, ok := ctx.ModuleForTests(moduleName, variant).Module().(android.OutputFileProducer)
	if !ok {
		t.Errorf("%q must have output\n", moduleName)
		return
	}
	outputFiles, err := mod.OutputFiles("")
	if err != nil || len(outputFiles) != 1 {
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

func checkSnapshot(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, moduleName, snapshotFilename, subDir, variant string) {
	checkSnapshotIncludeExclude(t, ctx, singleton, moduleName, snapshotFilename, subDir, variant, true, false)
}

func checkSnapshotExclude(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, moduleName, snapshotFilename, subDir, variant string) {
	checkSnapshotIncludeExclude(t, ctx, singleton, moduleName, snapshotFilename, subDir, variant, false, false)
}

func checkSnapshotRule(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, moduleName, snapshotFilename, subDir, variant string) {
	checkSnapshotIncludeExclude(t, ctx, singleton, moduleName, snapshotFilename, subDir, variant, true, true)
}

func checkWriteFileOutput(t *testing.T, params android.TestingBuildParams, expected []string) {
	t.Helper()
	content := android.ContentFromFileRuleForTests(t, params)
	actual := strings.FieldsFunc(content, func(r rune) bool { return r == '\n' })
	assertArrayString(t, actual, expected)
}

func checkVndkOutput(t *testing.T, ctx *android.TestContext, output string, expected []string) {
	t.Helper()
	vndkSnapshot := ctx.SingletonForTests("vndk-snapshot")
	checkWriteFileOutput(t, vndkSnapshot.Output(output), expected)
}

func checkVndkLibrariesOutput(t *testing.T, ctx *android.TestContext, module string, expected []string) {
	t.Helper()
	got := ctx.ModuleForTests(module, "").Module().(*vndkLibrariesTxt).fileNames
	assertArrayString(t, got, expected)
}

func TestVndk(t *testing.T) {
	bp := `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_private",
			vendor_available: true,
			vndk: {
				enabled: true,
				private: true,
			},
			nocrt: true,
			stem: "libvndk-private",
		}

		cc_library {
			name: "libvndk_product",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
			target: {
				vendor: {
					cflags: ["-DTEST"],
				},
				product: {
					cflags: ["-DTEST"],
				},
			},
		}

		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
			suffix: "-x",
		}

		cc_library {
			name: "libvndk_sp_private",
			vendor_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
				private: true,
			},
			nocrt: true,
			target: {
				vendor: {
					suffix: "-x",
				},
			},
		}

		cc_library {
			name: "libvndk_sp_product_private",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
				private: true,
			},
			nocrt: true,
			target: {
				vendor: {
					suffix: "-x",
				},
				product: {
					suffix: "-x",
				},
			},
		}

		llndk_libraries_txt {
			name: "llndk.libraries.txt",
		}
		vndkcore_libraries_txt {
			name: "vndkcore.libraries.txt",
		}
		vndksp_libraries_txt {
			name: "vndksp.libraries.txt",
		}
		vndkprivate_libraries_txt {
			name: "vndkprivate.libraries.txt",
		}
		vndkproduct_libraries_txt {
			name: "vndkproduct.libraries.txt",
		}
		vndkcorevariant_libraries_txt {
			name: "vndkcorevariant.libraries.txt",
			insert_vndk_version: false,
		}
	`

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.ProductVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")

	ctx := testCcWithConfig(t, config)

	// subdir == "" because VNDK libs are not supposed to be installed separately.
	// They are installed as part of VNDK APEX instead.
	checkVndkModule(t, ctx, "libvndk", "", false, "", vendorVariant)
	checkVndkModule(t, ctx, "libvndk_private", "", false, "", vendorVariant)
	checkVndkModule(t, ctx, "libvndk_product", "", false, "", vendorVariant)
	checkVndkModule(t, ctx, "libvndk_sp", "", true, "", vendorVariant)
	checkVndkModule(t, ctx, "libvndk_sp_private", "", true, "", vendorVariant)
	checkVndkModule(t, ctx, "libvndk_sp_product_private", "", true, "", vendorVariant)

	checkVndkModule(t, ctx, "libvndk_product", "", false, "", productVariant)
	checkVndkModule(t, ctx, "libvndk_sp_product_private", "", true, "", productVariant)

	// Check VNDK snapshot output.
	snapshotDir := "vndk-snapshot"
	snapshotVariantPath := filepath.Join(buildDir, snapshotDir, "arm64")

	vndkLibPath := filepath.Join(snapshotVariantPath, fmt.Sprintf("arch-%s-%s",
		"arm64", "armv8-a"))
	vndkLib2ndPath := filepath.Join(snapshotVariantPath, fmt.Sprintf("arch-%s-%s",
		"arm", "armv7-a-neon"))

	vndkCoreLibPath := filepath.Join(vndkLibPath, "shared", "vndk-core")
	vndkSpLibPath := filepath.Join(vndkLibPath, "shared", "vndk-sp")
	vndkCoreLib2ndPath := filepath.Join(vndkLib2ndPath, "shared", "vndk-core")
	vndkSpLib2ndPath := filepath.Join(vndkLib2ndPath, "shared", "vndk-sp")

	variant := "android_vendor.VER_arm64_armv8-a_shared"
	variant2nd := "android_vendor.VER_arm_armv7-a-neon_shared"

	snapshotSingleton := ctx.SingletonForTests("vndk-snapshot")

	checkSnapshot(t, ctx, snapshotSingleton, "libvndk", "libvndk.so", vndkCoreLibPath, variant)
	checkSnapshot(t, ctx, snapshotSingleton, "libvndk", "libvndk.so", vndkCoreLib2ndPath, variant2nd)
	checkSnapshot(t, ctx, snapshotSingleton, "libvndk_product", "libvndk_product.so", vndkCoreLibPath, variant)
	checkSnapshot(t, ctx, snapshotSingleton, "libvndk_product", "libvndk_product.so", vndkCoreLib2ndPath, variant2nd)
	checkSnapshot(t, ctx, snapshotSingleton, "libvndk_sp", "libvndk_sp-x.so", vndkSpLibPath, variant)
	checkSnapshot(t, ctx, snapshotSingleton, "libvndk_sp", "libvndk_sp-x.so", vndkSpLib2ndPath, variant2nd)

	snapshotConfigsPath := filepath.Join(snapshotVariantPath, "configs")
	checkSnapshot(t, ctx, snapshotSingleton, "llndk.libraries.txt", "llndk.libraries.txt", snapshotConfigsPath, "")
	checkSnapshot(t, ctx, snapshotSingleton, "vndkcore.libraries.txt", "vndkcore.libraries.txt", snapshotConfigsPath, "")
	checkSnapshot(t, ctx, snapshotSingleton, "vndksp.libraries.txt", "vndksp.libraries.txt", snapshotConfigsPath, "")
	checkSnapshot(t, ctx, snapshotSingleton, "vndkprivate.libraries.txt", "vndkprivate.libraries.txt", snapshotConfigsPath, "")
	checkSnapshot(t, ctx, snapshotSingleton, "vndkproduct.libraries.txt", "vndkproduct.libraries.txt", snapshotConfigsPath, "")

	checkVndkOutput(t, ctx, "vndk/vndk.libraries.txt", []string{
		"LLNDK: libc.so",
		"LLNDK: libdl.so",
		"LLNDK: libft2.so",
		"LLNDK: libm.so",
		"VNDK-SP: libc++.so",
		"VNDK-SP: libvndk_sp-x.so",
		"VNDK-SP: libvndk_sp_private-x.so",
		"VNDK-SP: libvndk_sp_product_private-x.so",
		"VNDK-core: libvndk-private.so",
		"VNDK-core: libvndk.so",
		"VNDK-core: libvndk_product.so",
		"VNDK-private: libft2.so",
		"VNDK-private: libvndk-private.so",
		"VNDK-private: libvndk_sp_private-x.so",
		"VNDK-private: libvndk_sp_product_private-x.so",
		"VNDK-product: libc++.so",
		"VNDK-product: libvndk_product.so",
		"VNDK-product: libvndk_sp_product_private-x.so",
	})
	checkVndkLibrariesOutput(t, ctx, "llndk.libraries.txt", []string{"libc.so", "libdl.so", "libft2.so", "libm.so"})
	checkVndkLibrariesOutput(t, ctx, "vndkcore.libraries.txt", []string{"libvndk-private.so", "libvndk.so", "libvndk_product.so"})
	checkVndkLibrariesOutput(t, ctx, "vndksp.libraries.txt", []string{"libc++.so", "libvndk_sp-x.so", "libvndk_sp_private-x.so", "libvndk_sp_product_private-x.so"})
	checkVndkLibrariesOutput(t, ctx, "vndkprivate.libraries.txt", []string{"libft2.so", "libvndk-private.so", "libvndk_sp_private-x.so", "libvndk_sp_product_private-x.so"})
	checkVndkLibrariesOutput(t, ctx, "vndkproduct.libraries.txt", []string{"libc++.so", "libvndk_product.so", "libvndk_sp_product_private-x.so"})
	checkVndkLibrariesOutput(t, ctx, "vndkcorevariant.libraries.txt", nil)
}

func TestVndkWithHostSupported(t *testing.T) {
	ctx := testCc(t, `
		cc_library {
			name: "libvndk_host_supported",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			host_supported: true,
		}

		cc_library {
			name: "libvndk_host_supported_but_disabled_on_device",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			host_supported: true,
			enabled: false,
			target: {
				host: {
					enabled: true,
				}
			}
		}

		vndkcore_libraries_txt {
			name: "vndkcore.libraries.txt",
		}
	`)

	checkVndkLibrariesOutput(t, ctx, "vndkcore.libraries.txt", []string{"libvndk_host_supported.so"})
}

func TestVndkLibrariesTxtAndroidMk(t *testing.T) {
	bp := `
		llndk_libraries_txt {
			name: "llndk.libraries.txt",
			insert_vndk_version: true,
		}`
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	ctx := testCcWithConfig(t, config)

	module := ctx.ModuleForTests("llndk.libraries.txt", "")
	entries := android.AndroidMkEntriesForTest(t, ctx, module.Module())[0]
	assertArrayString(t, entries.EntryMap["LOCAL_MODULE_STEM"], []string{"llndk.libraries.VER.txt"})
}

func TestVndkUsingCoreVariant(t *testing.T) {
	bp := `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk2",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				private: true,
			},
			nocrt: true,
		}

		vndkcorevariant_libraries_txt {
			name: "vndkcorevariant.libraries.txt",
			insert_vndk_version: false,
		}
	`

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	config.TestProductVariables.VndkUseCoreVariant = BoolPtr(true)

	setVndkMustUseVendorVariantListForTest(config, []string{"libvndk"})

	ctx := testCcWithConfig(t, config)

	checkVndkLibrariesOutput(t, ctx, "vndkcorevariant.libraries.txt", []string{"libc++.so", "libvndk2.so", "libvndk_sp.so"})
}

func TestDataLibs(t *testing.T) {
	bp := `
		cc_test_library {
			name: "test_lib",
			srcs: ["test_lib.cpp"],
			gtest: false,
		}

		cc_test {
			name: "main_test",
			data_libs: ["test_lib"],
			gtest: false,
		}
 `

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	config.TestProductVariables.VndkUseCoreVariant = BoolPtr(true)

	ctx := testCcWithConfig(t, config)
	module := ctx.ModuleForTests("main_test", "android_arm_armv7-a-neon").Module()
	testBinary := module.(*Module).linker.(*testBinary)
	outputFiles, err := module.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Expected cc_test to produce output files, error: %s", err)
		return
	}
	if len(outputFiles) != 1 {
		t.Errorf("expected exactly one output file. output files: [%s]", outputFiles)
		return
	}
	if len(testBinary.dataPaths()) != 1 {
		t.Errorf("expected exactly one test data file. test data files: [%s]", testBinary.dataPaths())
		return
	}

	outputPath := outputFiles[0].String()
	testBinaryPath := testBinary.dataPaths()[0].SrcPath.String()

	if !strings.HasSuffix(outputPath, "/main_test") {
		t.Errorf("expected test output file to be 'main_test', but was '%s'", outputPath)
		return
	}
	if !strings.HasSuffix(testBinaryPath, "/test_lib.so") {
		t.Errorf("expected test data file to be 'test_lib.so', but was '%s'", testBinaryPath)
		return
	}
}

func TestDataLibsRelativeInstallPath(t *testing.T) {
	bp := `
		cc_test_library {
			name: "test_lib",
			srcs: ["test_lib.cpp"],
			relative_install_path: "foo/bar/baz",
			gtest: false,
		}

		cc_test {
			name: "main_test",
			data_libs: ["test_lib"],
			gtest: false,
		}
 `

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	config.TestProductVariables.VndkUseCoreVariant = BoolPtr(true)

	ctx := testCcWithConfig(t, config)
	module := ctx.ModuleForTests("main_test", "android_arm_armv7-a-neon").Module()
	testBinary := module.(*Module).linker.(*testBinary)
	outputFiles, err := module.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Fatalf("Expected cc_test to produce output files, error: %s", err)
	}
	if len(outputFiles) != 1 {
		t.Errorf("expected exactly one output file. output files: [%s]", outputFiles)
	}
	if len(testBinary.dataPaths()) != 1 {
		t.Errorf("expected exactly one test data file. test data files: [%s]", testBinary.dataPaths())
	}

	outputPath := outputFiles[0].String()

	if !strings.HasSuffix(outputPath, "/main_test") {
		t.Errorf("expected test output file to be 'main_test', but was '%s'", outputPath)
	}
	entries := android.AndroidMkEntriesForTest(t, ctx, module)[0]
	if !strings.HasSuffix(entries.EntryMap["LOCAL_TEST_DATA"][0], ":test_lib.so:foo/bar/baz") {
		t.Errorf("expected LOCAL_TEST_DATA to end with `:test_lib.so:foo/bar/baz`,"+
			" but was '%s'", entries.EntryMap["LOCAL_TEST_DATA"][0])
	}
}

func TestVndkWhenVndkVersionIsNotSet(t *testing.T) {
	ctx := testCcNoVndk(t, `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
		cc_library {
			name: "libvndk-private",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				private: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libllndk",
			llndk_stubs: "libllndk.llndk",
		}

		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
			export_llndk_headers: ["libllndk_headers"],
		}

		llndk_headers {
			name: "libllndk_headers",
			export_include_dirs: ["include"],
		}
	`)

	checkVndkOutput(t, ctx, "vndk/vndk.libraries.txt", []string{
		"LLNDK: libc.so",
		"LLNDK: libdl.so",
		"LLNDK: libft2.so",
		"LLNDK: libllndk.so",
		"LLNDK: libm.so",
		"VNDK-SP: libc++.so",
		"VNDK-core: libvndk-private.so",
		"VNDK-core: libvndk.so",
		"VNDK-private: libft2.so",
		"VNDK-private: libvndk-private.so",
		"VNDK-product: libc++.so",
		"VNDK-product: libvndk-private.so",
		"VNDK-product: libvndk.so",
	})
}

func TestVndkModuleError(t *testing.T) {
	// Check the error message for vendor_available and product_available properties.
	testCcErrorProductVndk(t, "vndk: vendor_available must be set to true when `vndk: {enabled: true}`", `
		cc_library {
			name: "libvndk",
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
	`)

	testCcErrorProductVndk(t, "vndk: vendor_available must be set to true when `vndk: {enabled: true}`", `
		cc_library {
			name: "libvndk",
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
	`)

	testCcErrorProductVndk(t, "product properties must have the same values with the vendor properties for VNDK modules", `
		cc_library {
			name: "libvndkprop",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
			target: {
				vendor: {
					cflags: ["-DTEST",],
				},
			},
		}
	`)
}

func TestVndkDepError(t *testing.T) {
	// Check whether an error is emitted when a VNDK lib depends on a system lib.
	testCcError(t, "dependency \".*\" of \".*\" missing variant", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			shared_libs: ["libfwk"],  // Cause error
			nocrt: true,
		}

		cc_library {
			name: "libfwk",
			nocrt: true,
		}
	`)

	// Check whether an error is emitted when a VNDK lib depends on a vendor lib.
	testCcError(t, "dependency \".*\" of \".*\" missing variant", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			shared_libs: ["libvendor"],  // Cause error
			nocrt: true,
		}

		cc_library {
			name: "libvendor",
			vendor: true,
			nocrt: true,
		}
	`)

	// Check whether an error is emitted when a VNDK-SP lib depends on a system lib.
	testCcError(t, "dependency \".*\" of \".*\" missing variant", `
		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			shared_libs: ["libfwk"],  // Cause error
			nocrt: true,
		}

		cc_library {
			name: "libfwk",
			nocrt: true,
		}
	`)

	// Check whether an error is emitted when a VNDK-SP lib depends on a vendor lib.
	testCcError(t, "dependency \".*\" of \".*\" missing variant", `
		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			shared_libs: ["libvendor"],  // Cause error
			nocrt: true,
		}

		cc_library {
			name: "libvendor",
			vendor: true,
			nocrt: true,
		}
	`)

	// Check whether an error is emitted when a VNDK-SP lib depends on a VNDK lib.
	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			shared_libs: ["libvndk"],  // Cause error
			nocrt: true,
		}

		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
	`)

	// Check whether an error is emitted when a VNDK lib depends on a non-VNDK lib.
	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			shared_libs: ["libnonvndk"],
			nocrt: true,
		}

		cc_library {
			name: "libnonvndk",
			vendor_available: true,
			nocrt: true,
		}
	`)

	// Check whether an error is emitted when a VNDK-private lib depends on a non-VNDK lib.
	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndkprivate",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				private: true,
			},
			shared_libs: ["libnonvndk"],
			nocrt: true,
		}

		cc_library {
			name: "libnonvndk",
			vendor_available: true,
			nocrt: true,
		}
	`)

	// Check whether an error is emitted when a VNDK-sp lib depends on a non-VNDK lib.
	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndksp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			shared_libs: ["libnonvndk"],
			nocrt: true,
		}

		cc_library {
			name: "libnonvndk",
			vendor_available: true,
			nocrt: true,
		}
	`)

	// Check whether an error is emitted when a VNDK-sp-private lib depends on a non-VNDK lib.
	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndkspprivate",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
				private: true,
			},
			shared_libs: ["libnonvndk"],
			nocrt: true,
		}

		cc_library {
			name: "libnonvndk",
			vendor_available: true,
			nocrt: true,
		}
	`)
}

func TestDoubleLoadbleDep(t *testing.T) {
	// okay to link : LLNDK -> double_loadable VNDK
	testCc(t, `
		cc_library {
			name: "libllndk",
			shared_libs: ["libdoubleloadable"],
			llndk_stubs: "libllndk.llndk",
		}

		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
		}

		cc_library {
			name: "libdoubleloadable",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			double_loadable: true,
		}
	`)
	// okay to link : LLNDK -> VNDK-SP
	testCc(t, `
		cc_library {
			name: "libllndk",
			shared_libs: ["libvndksp"],
			llndk_stubs: "libllndk.llndk",
		}

		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
		}

		cc_library {
			name: "libvndksp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
		}
	`)
	// okay to link : double_loadable -> double_loadable
	testCc(t, `
		cc_library {
			name: "libdoubleloadable1",
			shared_libs: ["libdoubleloadable2"],
			vendor_available: true,
			double_loadable: true,
		}

		cc_library {
			name: "libdoubleloadable2",
			vendor_available: true,
			double_loadable: true,
		}
	`)
	// okay to link : double_loadable VNDK -> double_loadable VNDK private
	testCc(t, `
		cc_library {
			name: "libdoubleloadable",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			double_loadable: true,
			shared_libs: ["libnondoubleloadable"],
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				private: true,
			},
			double_loadable: true,
		}
	`)
	// okay to link : LLNDK -> core-only -> vendor_available & double_loadable
	testCc(t, `
		cc_library {
			name: "libllndk",
			shared_libs: ["libcoreonly"],
			llndk_stubs: "libllndk.llndk",
		}

		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
		}

		cc_library {
			name: "libcoreonly",
			shared_libs: ["libvendoravailable"],
		}

		// indirect dependency of LLNDK
		cc_library {
			name: "libvendoravailable",
			vendor_available: true,
			double_loadable: true,
		}
	`)
}

func TestDoubleLoadableDepError(t *testing.T) {
	// Check whether an error is emitted when a LLNDK depends on a non-double_loadable VNDK lib.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libllndk",
			shared_libs: ["libnondoubleloadable"],
			llndk_stubs: "libllndk.llndk",
		}

		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
		}
	`)

	// Check whether an error is emitted when a LLNDK depends on a non-double_loadable vendor_available lib.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libllndk",
			no_libcrt: true,
			shared_libs: ["libnondoubleloadable"],
			llndk_stubs: "libllndk.llndk",
		}

		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
		}
	`)

	// Check whether an error is emitted when a LLNDK depends on a non-double_loadable indirectly.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libllndk",
			shared_libs: ["libcoreonly"],
			llndk_stubs: "libllndk.llndk",
		}

		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
		}

		cc_library {
			name: "libcoreonly",
			shared_libs: ["libvendoravailable"],
		}

		// indirect dependency of LLNDK
		cc_library {
			name: "libvendoravailable",
			vendor_available: true,
		}
	`)

	// The error is not from 'client' but from 'libllndk'
	testCcError(t, "module \"libllndk\".* links a library \"libnondoubleloadable\".*double_loadable", `
		cc_library {
			name: "client",
			vendor_available: true,
			double_loadable: true,
			shared_libs: ["libllndk"],
		}
		cc_library {
			name: "libllndk",
			shared_libs: ["libnondoubleloadable"],
			llndk_stubs: "libllndk.llndk",
		}
		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
		}
		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
		}
	`)
}

func TestCheckVndkMembershipBeforeDoubleLoadable(t *testing.T) {
	testCcError(t, "module \"libvndksp\" variant .*: .*: VNDK-SP must only depend on VNDK-SP", `
		cc_library {
			name: "libvndksp",
			shared_libs: ["libanothervndksp"],
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			}
		}

		cc_library {
			name: "libllndk",
			shared_libs: ["libanothervndksp"],
		}

		llndk_library {
			name: "libllndk",
			symbol_file: "",
		}

		cc_library {
			name: "libanothervndksp",
			vendor_available: true,
		}
	`)
}

func TestVndkExt(t *testing.T) {
	// This test checks the VNDK-Ext properties.
	bp := `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
		cc_library {
			name: "libvndk2",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			target: {
				vendor: {
					suffix: "-suffix",
				},
				product: {
					suffix: "-suffix",
				},
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk2_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk2",
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext_product",
			product_specific: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk2_ext_product",
			product_specific: true,
			vndk: {
				enabled: true,
				extends: "libvndk2",
			},
			nocrt: true,
		}
	`
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.ProductVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")

	ctx := testCcWithConfig(t, config)

	checkVndkModule(t, ctx, "libvndk_ext", "vndk", false, "libvndk", vendorVariant)
	checkVndkModule(t, ctx, "libvndk_ext_product", "vndk", false, "libvndk", productVariant)

	mod_vendor := ctx.ModuleForTests("libvndk2_ext", vendorVariant).Module().(*Module)
	assertString(t, mod_vendor.outputFile.Path().Base(), "libvndk2-suffix.so")

	mod_product := ctx.ModuleForTests("libvndk2_ext_product", productVariant).Module().(*Module)
	assertString(t, mod_product.outputFile.Path().Base(), "libvndk2-suffix.so")
}

func TestVndkExtWithoutBoardVndkVersion(t *testing.T) {
	// This test checks the VNDK-Ext properties when BOARD_VNDK_VERSION is not set.
	ctx := testCcNoVndk(t, `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}
	`)

	// Ensures that the core variant of "libvndk_ext" can be found.
	mod := ctx.ModuleForTests("libvndk_ext", coreVariant).Module().(*Module)
	if extends := mod.getVndkExtendsModuleName(); extends != "libvndk" {
		t.Errorf("\"libvndk_ext\" must extend from \"libvndk\" but get %q", extends)
	}
}

func TestVndkExtWithoutProductVndkVersion(t *testing.T) {
	// This test checks the VNDK-Ext properties when PRODUCT_PRODUCT_VNDK_VERSION is not set.
	ctx := testCcNoProductVndk(t, `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext_product",
			product_specific: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}
	`)

	// Ensures that the core variant of "libvndk_ext_product" can be found.
	mod := ctx.ModuleForTests("libvndk_ext_product", coreVariant).Module().(*Module)
	if extends := mod.getVndkExtendsModuleName(); extends != "libvndk" {
		t.Errorf("\"libvndk_ext_product\" must extend from \"libvndk\" but get %q", extends)
	}
}

func TestVndkExtError(t *testing.T) {
	// This test ensures an error is emitted in ill-formed vndk-ext definition.
	testCcError(t, "must set `vendor: true` or `product_specific: true` to set `extends: \".*\"`", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}
	`)

	testCcError(t, "must set `extends: \"\\.\\.\\.\"` to vndk extension", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
	`)

	testCcErrorProductVndk(t, "must set `extends: \"\\.\\.\\.\"` to vndk extension", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext_product",
			product_specific: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
	`)

	testCcErrorProductVndk(t, "must not set at the same time as `vndk: {extends: \"\\.\\.\\.\"}`", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext_product",
			product_specific: true,
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}
	`)
}

func TestVndkExtInconsistentSupportSystemProcessError(t *testing.T) {
	// This test ensures an error is emitted for inconsistent support_system_process.
	testCcError(t, "module \".*\" with mismatched support_system_process", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
				support_system_process: true,
			},
			nocrt: true,
		}
	`)

	testCcError(t, "module \".*\" with mismatched support_system_process", `
		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk_sp",
			},
			nocrt: true,
		}
	`)
}

func TestVndkExtVendorAvailableFalseError(t *testing.T) {
	// This test ensures an error is emitted when a VNDK-Ext library extends a VNDK library
	// with `private: true`.
	testCcError(t, "`extends` refers module \".*\" which has `private: true`", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				private: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}
	`)

	testCcErrorProductVndk(t, "`extends` refers module \".*\" which has `private: true`", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				private: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext_product",
			product_specific: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}
	`)
}

func TestVendorModuleUseVndkExt(t *testing.T) {
	// This test ensures a vendor module can depend on a VNDK-Ext library.
	testCc(t, `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk_sp",
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvendor",
			vendor: true,
			shared_libs: ["libvndk_ext", "libvndk_sp_ext"],
			nocrt: true,
		}
	`)
}

func TestVndkExtUseVendorLib(t *testing.T) {
	// This test ensures a VNDK-Ext library can depend on a vendor library.
	testCc(t, `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			shared_libs: ["libvendor"],
			nocrt: true,
		}

		cc_library {
			name: "libvendor",
			vendor: true,
			nocrt: true,
		}
	`)

	// This test ensures a VNDK-SP-Ext library can depend on a vendor library.
	testCc(t, `
		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk_sp",
				support_system_process: true,
			},
			shared_libs: ["libvendor"],  // Cause an error
			nocrt: true,
		}

		cc_library {
			name: "libvendor",
			vendor: true,
			nocrt: true,
		}
	`)
}

func TestProductVndkExtDependency(t *testing.T) {
	bp := `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext_product",
			product_specific: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			shared_libs: ["libproduct_for_vndklibs"],
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp_ext_product",
			product_specific: true,
			vndk: {
				enabled: true,
				extends: "libvndk_sp",
				support_system_process: true,
			},
			shared_libs: ["libproduct_for_vndklibs"],
			nocrt: true,
		}

		cc_library {
			name: "libproduct",
			product_specific: true,
			shared_libs: ["libvndk_ext_product", "libvndk_sp_ext_product"],
			nocrt: true,
		}

		cc_library {
			name: "libproduct_for_vndklibs",
			product_specific: true,
			nocrt: true,
		}
	`
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.ProductVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")

	testCcWithConfig(t, config)
}

func TestVndkSpExtUseVndkError(t *testing.T) {
	// This test ensures an error is emitted if a VNDK-SP-Ext library depends on a VNDK
	// library.
	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk_sp",
				support_system_process: true,
			},
			shared_libs: ["libvndk"],  // Cause an error
			nocrt: true,
		}
	`)

	// This test ensures an error is emitted if a VNDK-SP-Ext library depends on a VNDK-Ext
	// library.
	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk_sp",
				support_system_process: true,
			},
			shared_libs: ["libvndk_ext"],  // Cause an error
			nocrt: true,
		}
	`)
}

func TestVndkUseVndkExtError(t *testing.T) {
	// This test ensures an error is emitted if a VNDK/VNDK-SP library depends on a
	// VNDK-Ext/VNDK-SP-Ext library.
	testCcError(t, "dependency \".*\" of \".*\" missing variant", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk2",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			shared_libs: ["libvndk_ext"],
			nocrt: true,
		}
	`)

	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk2",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			target: {
				vendor: {
					shared_libs: ["libvndk_ext"],
				},
			},
			nocrt: true,
		}
	`)

	testCcError(t, "dependency \".*\" of \".*\" missing variant", `
		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk_sp",
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp_2",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			shared_libs: ["libvndk_sp_ext"],
			nocrt: true,
		}
	`)

	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp_ext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk_sp",
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp2",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			target: {
				vendor: {
					shared_libs: ["libvndk_sp_ext"],
				},
			},
			nocrt: true,
		}
	`)
}

func TestEnforceProductVndkVersion(t *testing.T) {
	bp := `
		cc_library {
			name: "libllndk",
			llndk_stubs: "libllndk.llndk",
		}
		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
		}
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}
		cc_library {
			name: "libva",
			vendor_available: true,
			nocrt: true,
		}
		cc_library {
			name: "libpa",
			product_available: true,
			nocrt: true,
		}
		cc_library {
			name: "libboth_available",
			vendor_available: true,
			product_available: true,
			nocrt: true,
			target: {
				vendor: {
					suffix: "-vendor",
				},
				product: {
					suffix: "-product",
				},
			}
		}
		cc_library {
			name: "libproduct_va",
			product_specific: true,
			vendor_available: true,
			nocrt: true,
		}
		cc_library {
			name: "libprod",
			product_specific: true,
			shared_libs: [
				"libllndk",
				"libvndk",
				"libvndk_sp",
				"libpa",
				"libboth_available",
				"libproduct_va",
			],
			nocrt: true,
		}
		cc_library {
			name: "libvendor",
			vendor: true,
			shared_libs: [
				"libllndk",
				"libvndk",
				"libvndk_sp",
				"libva",
				"libboth_available",
				"libproduct_va",
			],
			nocrt: true,
		}
	`

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.ProductVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")

	ctx := testCcWithConfig(t, config)

	checkVndkModule(t, ctx, "libvndk", "", false, "", productVariant)
	checkVndkModule(t, ctx, "libvndk_sp", "", true, "", productVariant)

	mod_vendor := ctx.ModuleForTests("libboth_available", vendorVariant).Module().(*Module)
	assertString(t, mod_vendor.outputFile.Path().Base(), "libboth_available-vendor.so")

	mod_product := ctx.ModuleForTests("libboth_available", productVariant).Module().(*Module)
	assertString(t, mod_product.outputFile.Path().Base(), "libboth_available-product.so")
}

func TestEnforceProductVndkVersionErrors(t *testing.T) {
	testCcErrorProductVndk(t, "dependency \".*\" of \".*\" missing variant:\n.*image:product.VER", `
		cc_library {
			name: "libprod",
			product_specific: true,
			shared_libs: [
				"libvendor",
			],
			nocrt: true,
		}
		cc_library {
			name: "libvendor",
			vendor: true,
			nocrt: true,
		}
	`)
	testCcErrorProductVndk(t, "dependency \".*\" of \".*\" missing variant:\n.*image:product.VER", `
		cc_library {
			name: "libprod",
			product_specific: true,
			shared_libs: [
				"libsystem",
			],
			nocrt: true,
		}
		cc_library {
			name: "libsystem",
			nocrt: true,
		}
	`)
	testCcErrorProductVndk(t, "dependency \".*\" of \".*\" missing variant:\n.*image:product.VER", `
		cc_library {
			name: "libprod",
			product_specific: true,
			shared_libs: [
				"libva",
			],
			nocrt: true,
		}
		cc_library {
			name: "libva",
			vendor_available: true,
			nocrt: true,
		}
	`)
	testCcErrorProductVndk(t, "non-VNDK module should not link to \".*\" which has `private: true`", `
		cc_library {
			name: "libprod",
			product_specific: true,
			shared_libs: [
				"libvndk_private",
			],
			nocrt: true,
		}
		cc_library {
			name: "libvndk_private",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				private: true,
			},
			nocrt: true,
		}
	`)
	testCcErrorProductVndk(t, "dependency \".*\" of \".*\" missing variant:\n.*image:product.VER", `
		cc_library {
			name: "libprod",
			product_specific: true,
			shared_libs: [
				"libsystem_ext",
			],
			nocrt: true,
		}
		cc_library {
			name: "libsystem_ext",
			system_ext_specific: true,
			nocrt: true,
		}
	`)
	testCcErrorProductVndk(t, "dependency \".*\" of \".*\" missing variant:\n.*image:", `
		cc_library {
			name: "libsystem",
			shared_libs: [
				"libproduct_va",
			],
			nocrt: true,
		}
		cc_library {
			name: "libproduct_va",
			product_specific: true,
			vendor_available: true,
			nocrt: true,
		}
	`)
}

func TestMakeLinkType(t *testing.T) {
	bp := `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
		}
		cc_library {
			name: "libvndksp",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
		}
		cc_library {
			name: "libvndkprivate",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
				private: true,
			},
		}
		cc_library {
			name: "libvendor",
			vendor: true,
		}
		cc_library {
			name: "libvndkext",
			vendor: true,
			vndk: {
				enabled: true,
				extends: "libvndk",
			},
		}
		vndk_prebuilt_shared {
			name: "prevndk",
			version: "27",
			target_arch: "arm",
			binder32bit: true,
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			arch: {
				arm: {
					srcs: ["liba.so"],
				},
			},
		}
		cc_library {
			name: "libllndk",
			llndk_stubs: "libllndk.llndk",
		}
		llndk_library {
			name: "libllndk.llndk",
			symbol_file: "",
		}
		cc_library {
			name: "libllndkprivate",
			llndk_stubs: "libllndkprivate.llndk",
		}
		llndk_library {
			name: "libllndkprivate.llndk",
			private: true,
			symbol_file: "",
		}

		llndk_libraries_txt {
			name: "llndk.libraries.txt",
		}
		vndkcore_libraries_txt {
			name: "vndkcore.libraries.txt",
		}
		vndksp_libraries_txt {
			name: "vndksp.libraries.txt",
		}
		vndkprivate_libraries_txt {
			name: "vndkprivate.libraries.txt",
		}
		vndkcorevariant_libraries_txt {
			name: "vndkcorevariant.libraries.txt",
			insert_vndk_version: false,
		}
	`

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	// native:vndk
	ctx := testCcWithConfig(t, config)

	checkVndkLibrariesOutput(t, ctx, "vndkcore.libraries.txt",
		[]string{"libvndk.so", "libvndkprivate.so"})
	checkVndkLibrariesOutput(t, ctx, "vndksp.libraries.txt",
		[]string{"libc++.so", "libvndksp.so"})
	checkVndkLibrariesOutput(t, ctx, "llndk.libraries.txt",
		[]string{"libc.so", "libdl.so", "libft2.so", "libllndk.so", "libllndkprivate.so", "libm.so"})
	checkVndkLibrariesOutput(t, ctx, "vndkprivate.libraries.txt",
		[]string{"libft2.so", "libllndkprivate.so", "libvndkprivate.so"})

	vendorVariant27 := "android_vendor.27_arm64_armv8-a_shared"

	tests := []struct {
		variant  string
		name     string
		expected string
	}{
		{vendorVariant, "libvndk", "native:vndk"},
		{vendorVariant, "libvndksp", "native:vndk"},
		{vendorVariant, "libvndkprivate", "native:vndk_private"},
		{vendorVariant, "libvendor", "native:vendor"},
		{vendorVariant, "libvndkext", "native:vendor"},
		{vendorVariant, "libllndk", "native:vndk"},
		{vendorVariant27, "prevndk.vndk.27.arm.binder32", "native:vndk"},
		{coreVariant, "libvndk", "native:platform"},
		{coreVariant, "libvndkprivate", "native:platform"},
		{coreVariant, "libllndk", "native:platform"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			module := ctx.ModuleForTests(test.name, test.variant).Module().(*Module)
			assertString(t, module.makeLinkType, test.expected)
		})
	}
}

var staticLinkDepOrderTestCases = []struct {
	// This is a string representation of a map[moduleName][]moduleDependency .
	// It models the dependencies declared in an Android.bp file.
	inStatic string

	// This is a string representation of a map[moduleName][]moduleDependency .
	// It models the dependencies declared in an Android.bp file.
	inShared string

	// allOrdered is a string representation of a map[moduleName][]moduleDependency .
	// The keys of allOrdered specify which modules we would like to check.
	// The values of allOrdered specify the expected result (of the transitive closure of all
	// dependencies) for each module to test
	allOrdered string

	// outOrdered is a string representation of a map[moduleName][]moduleDependency .
	// The keys of outOrdered specify which modules we would like to check.
	// The values of outOrdered specify the expected result (of the ordered linker command line)
	// for each module to test.
	outOrdered string
}{
	// Simple tests
	{
		inStatic:   "",
		outOrdered: "",
	},
	{
		inStatic:   "a:",
		outOrdered: "a:",
	},
	{
		inStatic:   "a:b; b:",
		outOrdered: "a:b; b:",
	},
	// Tests of reordering
	{
		// diamond example
		inStatic:   "a:d,b,c; b:d; c:d; d:",
		outOrdered: "a:b,c,d; b:d; c:d; d:",
	},
	{
		// somewhat real example
		inStatic:   "bsdiff_unittest:b,c,d,e,f,g,h,i; e:b",
		outOrdered: "bsdiff_unittest:c,d,e,b,f,g,h,i; e:b",
	},
	{
		// multiple reorderings
		inStatic:   "a:b,c,d,e; d:b; e:c",
		outOrdered: "a:d,b,e,c; d:b; e:c",
	},
	{
		// should reorder without adding new transitive dependencies
		inStatic:   "bin:lib2,lib1;             lib1:lib2,liboptional",
		allOrdered: "bin:lib1,lib2,liboptional; lib1:lib2,liboptional",
		outOrdered: "bin:lib1,lib2;             lib1:lib2,liboptional",
	},
	{
		// multiple levels of dependencies
		inStatic:   "a:b,c,d,e,f,g,h; f:b,c,d; b:c,d; c:d",
		allOrdered: "a:e,f,b,c,d,g,h; f:b,c,d; b:c,d; c:d",
		outOrdered: "a:e,f,b,c,d,g,h; f:b,c,d; b:c,d; c:d",
	},
	// shared dependencies
	{
		// Note that this test doesn't recurse, to minimize the amount of logic it tests.
		// So, we don't actually have to check that a shared dependency of c will change the order
		// of a library that depends statically on b and on c.  We only need to check that if c has
		// a shared dependency on b, that that shows up in allOrdered.
		inShared:   "c:b",
		allOrdered: "c:b",
		outOrdered: "c:",
	},
	{
		// This test doesn't actually include any shared dependencies but it's a reminder of what
		// the second phase of the above test would look like
		inStatic:   "a:b,c; c:b",
		allOrdered: "a:c,b; c:b",
		outOrdered: "a:c,b; c:b",
	},
	// tiebreakers for when two modules specifying different orderings and there is no dependency
	// to dictate an order
	{
		// if the tie is between two modules at the end of a's deps, then a's order wins
		inStatic:   "a1:b,c,d,e; a2:b,c,e,d; b:d,e; c:e,d",
		outOrdered: "a1:b,c,d,e; a2:b,c,e,d; b:d,e; c:e,d",
	},
	{
		// if the tie is between two modules at the start of a's deps, then c's order is used
		inStatic:   "a1:d,e,b1,c1; b1:d,e; c1:e,d;   a2:d,e,b2,c2; b2:d,e; c2:d,e",
		outOrdered: "a1:b1,c1,e,d; b1:d,e; c1:e,d;   a2:b2,c2,d,e; b2:d,e; c2:d,e",
	},
	// Tests involving duplicate dependencies
	{
		// simple duplicate
		inStatic:   "a:b,c,c,b",
		outOrdered: "a:c,b",
	},
	{
		// duplicates with reordering
		inStatic:   "a:b,c,d,c; c:b",
		outOrdered: "a:d,c,b",
	},
	// Tests to confirm the nonexistence of infinite loops.
	// These cases should never happen, so as long as the test terminates and the
	// result is deterministic then that should be fine.
	{
		inStatic:   "a:a",
		outOrdered: "a:a",
	},
	{
		inStatic:   "a:b;   b:c;   c:a",
		allOrdered: "a:b,c; b:c,a; c:a,b",
		outOrdered: "a:b;   b:c;   c:a",
	},
	{
		inStatic:   "a:b,c;   b:c,a;   c:a,b",
		allOrdered: "a:c,a,b; b:a,b,c; c:b,c,a",
		outOrdered: "a:c,b;   b:a,c;   c:b,a",
	},
}

// converts from a string like "a:b,c; d:e" to (["a","b"], {"a":["b","c"], "d":["e"]}, [{"a", "a.o"}, {"b", "b.o"}])
func parseModuleDeps(text string) (modulesInOrder []android.Path, allDeps map[android.Path][]android.Path) {
	// convert from "a:b,c; d:e" to "a:b,c;d:e"
	strippedText := strings.Replace(text, " ", "", -1)
	if len(strippedText) < 1 {
		return []android.Path{}, make(map[android.Path][]android.Path, 0)
	}
	allDeps = make(map[android.Path][]android.Path, 0)

	// convert from "a:b,c;d:e" to ["a:b,c", "d:e"]
	moduleTexts := strings.Split(strippedText, ";")

	outputForModuleName := func(moduleName string) android.Path {
		return android.PathForTesting(moduleName)
	}

	for _, moduleText := range moduleTexts {
		// convert from "a:b,c" to ["a", "b,c"]
		components := strings.Split(moduleText, ":")
		if len(components) != 2 {
			panic(fmt.Sprintf("illegal module dep string %q from larger string %q; must contain one ':', not %v", moduleText, text, len(components)-1))
		}
		moduleName := components[0]
		moduleOutput := outputForModuleName(moduleName)
		modulesInOrder = append(modulesInOrder, moduleOutput)

		depString := components[1]
		// convert from "b,c" to ["b", "c"]
		depNames := strings.Split(depString, ",")
		if len(depString) < 1 {
			depNames = []string{}
		}
		var deps []android.Path
		for _, depName := range depNames {
			deps = append(deps, outputForModuleName(depName))
		}
		allDeps[moduleOutput] = deps
	}
	return modulesInOrder, allDeps
}

func getOutputPaths(ctx *android.TestContext, variant string, moduleNames []string) (paths android.Paths) {
	for _, moduleName := range moduleNames {
		module := ctx.ModuleForTests(moduleName, variant).Module().(*Module)
		output := module.outputFile.Path()
		paths = append(paths, output)
	}
	return paths
}

func TestStaticLibDepReordering(t *testing.T) {
	ctx := testCc(t, `
	cc_library {
		name: "a",
		static_libs: ["b", "c", "d"],
		stl: "none",
	}
	cc_library {
		name: "b",
		stl: "none",
	}
	cc_library {
		name: "c",
		static_libs: ["b"],
		stl: "none",
	}
	cc_library {
		name: "d",
		stl: "none",
	}

	`)

	variant := "android_arm64_armv8-a_static"
	moduleA := ctx.ModuleForTests("a", variant).Module().(*Module)
	actual := ctx.ModuleProvider(moduleA, StaticLibraryInfoProvider).(StaticLibraryInfo).TransitiveStaticLibrariesForOrdering.ToList()
	expected := getOutputPaths(ctx, variant, []string{"a", "c", "b", "d"})

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("staticDeps orderings were not propagated correctly"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

func TestStaticLibDepReorderingWithShared(t *testing.T) {
	ctx := testCc(t, `
	cc_library {
		name: "a",
		static_libs: ["b", "c"],
		stl: "none",
	}
	cc_library {
		name: "b",
		stl: "none",
	}
	cc_library {
		name: "c",
		shared_libs: ["b"],
		stl: "none",
	}

	`)

	variant := "android_arm64_armv8-a_static"
	moduleA := ctx.ModuleForTests("a", variant).Module().(*Module)
	actual := ctx.ModuleProvider(moduleA, StaticLibraryInfoProvider).(StaticLibraryInfo).TransitiveStaticLibrariesForOrdering.ToList()
	expected := getOutputPaths(ctx, variant, []string{"a", "c", "b"})

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("staticDeps orderings did not account for shared libs"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

func checkEquals(t *testing.T, message string, expected, actual interface{}) {
	t.Helper()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf(message+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

func TestLlndkLibrary(t *testing.T) {
	ctx := testCc(t, `
	cc_library {
		name: "libllndk",
		stubs: { versions: ["1", "2"] },
		llndk_stubs: "libllndk.llndk",
	}
	llndk_library {
		name: "libllndk.llndk",
	}

	cc_prebuilt_library_shared {
		name: "libllndkprebuilt",
		stubs: { versions: ["1", "2"] },
		llndk_stubs: "libllndkprebuilt.llndk",
	}
	llndk_library {
		name: "libllndkprebuilt.llndk",
	}

	cc_library {
		name: "libllndk_with_external_headers",
		stubs: { versions: ["1", "2"] },
		llndk_stubs: "libllndk_with_external_headers.llndk",
		header_libs: ["libexternal_headers"],
		export_header_lib_headers: ["libexternal_headers"],
	}
	llndk_library {
		name: "libllndk_with_external_headers.llndk",
	}
	cc_library_headers {
		name: "libexternal_headers",
		export_include_dirs: ["include"],
		vendor_available: true,
	}
	`)
	actual := ctx.ModuleVariantsForTests("libllndk")
	for i := 0; i < len(actual); i++ {
		if !strings.HasPrefix(actual[i], "android_vendor.VER_") {
			actual = append(actual[:i], actual[i+1:]...)
			i--
		}
	}
	expected := []string{
		"android_vendor.VER_arm64_armv8-a_shared_1",
		"android_vendor.VER_arm64_armv8-a_shared_2",
		"android_vendor.VER_arm64_armv8-a_shared",
		"android_vendor.VER_arm_armv7-a-neon_shared_1",
		"android_vendor.VER_arm_armv7-a-neon_shared_2",
		"android_vendor.VER_arm_armv7-a-neon_shared",
	}
	checkEquals(t, "variants for llndk stubs", expected, actual)

	params := ctx.ModuleForTests("libllndk", "android_vendor.VER_arm_armv7-a-neon_shared").Description("generate stub")
	checkEquals(t, "use VNDK version for default stubs", "current", params.Args["apiLevel"])

	params = ctx.ModuleForTests("libllndk", "android_vendor.VER_arm_armv7-a-neon_shared_1").Description("generate stub")
	checkEquals(t, "override apiLevel for versioned stubs", "1", params.Args["apiLevel"])
}

func TestLlndkHeaders(t *testing.T) {
	ctx := testCc(t, `
	llndk_headers {
		name: "libllndk_headers",
		export_include_dirs: ["my_include"],
	}
	llndk_library {
		name: "libllndk.llndk",
		export_llndk_headers: ["libllndk_headers"],
	}
	cc_library {
		name: "libllndk",
		llndk_stubs: "libllndk.llndk",
	}

	cc_library {
		name: "libvendor",
		shared_libs: ["libllndk"],
		vendor: true,
		srcs: ["foo.c"],
		no_libcrt: true,
		nocrt: true,
	}
	`)

	// _static variant is used since _shared reuses *.o from the static variant
	cc := ctx.ModuleForTests("libvendor", "android_vendor.VER_arm_armv7-a-neon_static").Rule("cc")
	cflags := cc.Args["cFlags"]
	if !strings.Contains(cflags, "-Imy_include") {
		t.Errorf("cflags for libvendor must contain -Imy_include, but was %#v.", cflags)
	}
}

func checkRuntimeLibs(t *testing.T, expected []string, module *Module) {
	actual := module.Properties.AndroidMkRuntimeLibs
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("incorrect runtime_libs for shared libs"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

const runtimeLibAndroidBp = `
	cc_library {
		name: "liball_available",
		vendor_available: true,
		product_available: true,
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor_available1",
		vendor_available: true,
		runtime_libs: ["liball_available"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor_available2",
		vendor_available: true,
		runtime_libs: ["liball_available"],
		target: {
			vendor: {
				exclude_runtime_libs: ["liball_available"],
			}
		},
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libproduct_vendor",
		product_specific: true,
		vendor_available: true,
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libcore",
		runtime_libs: ["liball_available"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor1",
		vendor: true,
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor2",
		vendor: true,
		runtime_libs: ["liball_available", "libvendor1", "libproduct_vendor"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libproduct_available1",
		product_available: true,
		runtime_libs: ["liball_available"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libproduct1",
		product_specific: true,
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libproduct2",
		product_specific: true,
		runtime_libs: ["liball_available", "libproduct1", "libproduct_vendor"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
`

func TestRuntimeLibs(t *testing.T) {
	ctx := testCc(t, runtimeLibAndroidBp)

	// runtime_libs for core variants use the module names without suffixes.
	variant := "android_arm64_armv8-a_shared"

	module := ctx.ModuleForTests("libvendor_available1", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available"}, module)

	module = ctx.ModuleForTests("libproduct_available1", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available"}, module)

	module = ctx.ModuleForTests("libcore", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available"}, module)

	// runtime_libs for vendor variants have '.vendor' suffixes if the modules have both core
	// and vendor variants.
	variant = "android_vendor.VER_arm64_armv8-a_shared"

	module = ctx.ModuleForTests("libvendor_available1", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available.vendor"}, module)

	module = ctx.ModuleForTests("libvendor2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available.vendor", "libvendor1", "libproduct_vendor.vendor"}, module)

	// runtime_libs for product variants have '.product' suffixes if the modules have both core
	// and product variants.
	variant = "android_product.VER_arm64_armv8-a_shared"

	module = ctx.ModuleForTests("libproduct_available1", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available.product"}, module)

	module = ctx.ModuleForTests("libproduct2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available.product", "libproduct1", "libproduct_vendor"}, module)
}

func TestExcludeRuntimeLibs(t *testing.T) {
	ctx := testCc(t, runtimeLibAndroidBp)

	variant := "android_arm64_armv8-a_shared"
	module := ctx.ModuleForTests("libvendor_available2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available"}, module)

	variant = "android_vendor.VER_arm64_armv8-a_shared"
	module = ctx.ModuleForTests("libvendor_available2", variant).Module().(*Module)
	checkRuntimeLibs(t, nil, module)
}

func TestRuntimeLibsNoVndk(t *testing.T) {
	ctx := testCcNoVndk(t, runtimeLibAndroidBp)

	// If DeviceVndkVersion is not defined, then runtime_libs are copied as-is.

	variant := "android_arm64_armv8-a_shared"

	module := ctx.ModuleForTests("libvendor_available1", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available"}, module)

	module = ctx.ModuleForTests("libvendor2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available", "libvendor1", "libproduct_vendor"}, module)

	module = ctx.ModuleForTests("libproduct2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available", "libproduct1", "libproduct_vendor"}, module)
}

func checkStaticLibs(t *testing.T, expected []string, module *Module) {
	t.Helper()
	actual := module.Properties.AndroidMkStaticLibs
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("incorrect static_libs"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

const staticLibAndroidBp = `
	cc_library {
		name: "lib1",
	}
	cc_library {
		name: "lib2",
		static_libs: ["lib1"],
	}
`

func TestStaticLibDepExport(t *testing.T) {
	ctx := testCc(t, staticLibAndroidBp)

	// Check the shared version of lib2.
	variant := "android_arm64_armv8-a_shared"
	module := ctx.ModuleForTests("lib2", variant).Module().(*Module)
	checkStaticLibs(t, []string{"lib1", "libc++demangle", "libclang_rt.builtins-aarch64-android", "libatomic"}, module)

	// Check the static version of lib2.
	variant = "android_arm64_armv8-a_static"
	module = ctx.ModuleForTests("lib2", variant).Module().(*Module)
	// libc++_static is linked additionally.
	checkStaticLibs(t, []string{"lib1", "libc++_static", "libc++demangle", "libclang_rt.builtins-aarch64-android", "libatomic"}, module)
}

var compilerFlagsTestCases = []struct {
	in  string
	out bool
}{
	{
		in:  "a",
		out: false,
	},
	{
		in:  "-a",
		out: true,
	},
	{
		in:  "-Ipath/to/something",
		out: false,
	},
	{
		in:  "-isystempath/to/something",
		out: false,
	},
	{
		in:  "--coverage",
		out: false,
	},
	{
		in:  "-include a/b",
		out: true,
	},
	{
		in:  "-include a/b c/d",
		out: false,
	},
	{
		in:  "-DMACRO",
		out: true,
	},
	{
		in:  "-DMAC RO",
		out: false,
	},
	{
		in:  "-a -b",
		out: false,
	},
	{
		in:  "-DMACRO=definition",
		out: true,
	},
	{
		in:  "-DMACRO=defi nition",
		out: true, // TODO(jiyong): this should be false
	},
	{
		in:  "-DMACRO(x)=x + 1",
		out: true,
	},
	{
		in:  "-DMACRO=\"defi nition\"",
		out: true,
	},
}

type mockContext struct {
	BaseModuleContext
	result bool
}

func (ctx *mockContext) PropertyErrorf(property, format string, args ...interface{}) {
	// CheckBadCompilerFlags calls this function when the flag should be rejected
	ctx.result = false
}

func TestCompilerFlags(t *testing.T) {
	for _, testCase := range compilerFlagsTestCases {
		ctx := &mockContext{result: true}
		CheckBadCompilerFlags(ctx, "", []string{testCase.in})
		if ctx.result != testCase.out {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", ctx.result)
		}
	}
}

func TestVendorPublicLibraries(t *testing.T) {
	ctx := testCc(t, `
	cc_library_headers {
		name: "libvendorpublic_headers",
		export_include_dirs: ["my_include"],
	}
	vendor_public_library {
		name: "libvendorpublic",
		symbol_file: "",
		export_public_headers: ["libvendorpublic_headers"],
	}
	cc_library {
		name: "libvendorpublic",
		srcs: ["foo.c"],
		vendor: true,
		no_libcrt: true,
		nocrt: true,
	}

	cc_library {
		name: "libsystem",
		shared_libs: ["libvendorpublic"],
		vendor: false,
		srcs: ["foo.c"],
		no_libcrt: true,
		nocrt: true,
	}
	cc_library {
		name: "libvendor",
		shared_libs: ["libvendorpublic"],
		vendor: true,
		srcs: ["foo.c"],
		no_libcrt: true,
		nocrt: true,
	}
	`)

	coreVariant := "android_arm64_armv8-a_shared"
	vendorVariant := "android_vendor.VER_arm64_armv8-a_shared"

	// test if header search paths are correctly added
	// _static variant is used since _shared reuses *.o from the static variant
	cc := ctx.ModuleForTests("libsystem", strings.Replace(coreVariant, "_shared", "_static", 1)).Rule("cc")
	cflags := cc.Args["cFlags"]
	if !strings.Contains(cflags, "-Imy_include") {
		t.Errorf("cflags for libsystem must contain -Imy_include, but was %#v.", cflags)
	}

	// test if libsystem is linked to the stub
	ld := ctx.ModuleForTests("libsystem", coreVariant).Rule("ld")
	libflags := ld.Args["libFlags"]
	stubPaths := getOutputPaths(ctx, coreVariant, []string{"libvendorpublic" + vendorPublicLibrarySuffix})
	if !strings.Contains(libflags, stubPaths[0].String()) {
		t.Errorf("libflags for libsystem must contain %#v, but was %#v", stubPaths[0], libflags)
	}

	// test if libvendor is linked to the real shared lib
	ld = ctx.ModuleForTests("libvendor", vendorVariant).Rule("ld")
	libflags = ld.Args["libFlags"]
	stubPaths = getOutputPaths(ctx, vendorVariant, []string{"libvendorpublic"})
	if !strings.Contains(libflags, stubPaths[0].String()) {
		t.Errorf("libflags for libvendor must contain %#v, but was %#v", stubPaths[0], libflags)
	}

}

func TestRecovery(t *testing.T) {
	ctx := testCc(t, `
		cc_library_shared {
			name: "librecovery",
			recovery: true,
		}
		cc_library_shared {
			name: "librecovery32",
			recovery: true,
			compile_multilib:"32",
		}
		cc_library_shared {
			name: "libHalInRecovery",
			recovery_available: true,
			vendor: true,
		}
	`)

	variants := ctx.ModuleVariantsForTests("librecovery")
	const arm64 = "android_recovery_arm64_armv8-a_shared"
	if len(variants) != 1 || !android.InList(arm64, variants) {
		t.Errorf("variants of librecovery must be \"%s\" only, but was %#v", arm64, variants)
	}

	variants = ctx.ModuleVariantsForTests("librecovery32")
	if android.InList(arm64, variants) {
		t.Errorf("multilib was set to 32 for librecovery32, but its variants has %s.", arm64)
	}

	recoveryModule := ctx.ModuleForTests("libHalInRecovery", recoveryVariant).Module().(*Module)
	if !recoveryModule.Platform() {
		t.Errorf("recovery variant of libHalInRecovery must not specific to device, soc, or product")
	}
}

func TestDataLibsPrebuiltSharedTestLibrary(t *testing.T) {
	bp := `
		cc_prebuilt_test_library_shared {
			name: "test_lib",
			relative_install_path: "foo/bar/baz",
			srcs: ["srcpath/dontusethispath/baz.so"],
		}

		cc_test {
			name: "main_test",
			data_libs: ["test_lib"],
			gtest: false,
		}
 `

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	config.TestProductVariables.VndkUseCoreVariant = BoolPtr(true)

	ctx := testCcWithConfig(t, config)
	module := ctx.ModuleForTests("main_test", "android_arm_armv7-a-neon").Module()
	testBinary := module.(*Module).linker.(*testBinary)
	outputFiles, err := module.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Fatalf("Expected cc_test to produce output files, error: %s", err)
	}
	if len(outputFiles) != 1 {
		t.Errorf("expected exactly one output file. output files: [%s]", outputFiles)
	}
	if len(testBinary.dataPaths()) != 1 {
		t.Errorf("expected exactly one test data file. test data files: [%s]", testBinary.dataPaths())
	}

	outputPath := outputFiles[0].String()

	if !strings.HasSuffix(outputPath, "/main_test") {
		t.Errorf("expected test output file to be 'main_test', but was '%s'", outputPath)
	}
	entries := android.AndroidMkEntriesForTest(t, ctx, module)[0]
	if !strings.HasSuffix(entries.EntryMap["LOCAL_TEST_DATA"][0], ":test_lib.so:foo/bar/baz") {
		t.Errorf("expected LOCAL_TEST_DATA to end with `:test_lib.so:foo/bar/baz`,"+
			" but was '%s'", entries.EntryMap["LOCAL_TEST_DATA"][0])
	}
}

func TestVersionedStubs(t *testing.T) {
	ctx := testCc(t, `
		cc_library_shared {
			name: "libFoo",
			srcs: ["foo.c"],
			stubs: {
				symbol_file: "foo.map.txt",
				versions: ["1", "2", "3"],
			},
		}

		cc_library_shared {
			name: "libBar",
			srcs: ["bar.c"],
			shared_libs: ["libFoo#1"],
		}`)

	variants := ctx.ModuleVariantsForTests("libFoo")
	expectedVariants := []string{
		"android_arm64_armv8-a_shared",
		"android_arm64_armv8-a_shared_1",
		"android_arm64_armv8-a_shared_2",
		"android_arm64_armv8-a_shared_3",
		"android_arm_armv7-a-neon_shared",
		"android_arm_armv7-a-neon_shared_1",
		"android_arm_armv7-a-neon_shared_2",
		"android_arm_armv7-a-neon_shared_3",
	}
	variantsMismatch := false
	if len(variants) != len(expectedVariants) {
		variantsMismatch = true
	} else {
		for _, v := range expectedVariants {
			if !inList(v, variants) {
				variantsMismatch = false
			}
		}
	}
	if variantsMismatch {
		t.Errorf("variants of libFoo expected:\n")
		for _, v := range expectedVariants {
			t.Errorf("%q\n", v)
		}
		t.Errorf(", but got:\n")
		for _, v := range variants {
			t.Errorf("%q\n", v)
		}
	}

	libBarLinkRule := ctx.ModuleForTests("libBar", "android_arm64_armv8-a_shared").Rule("ld")
	libFlags := libBarLinkRule.Args["libFlags"]
	libFoo1StubPath := "libFoo/android_arm64_armv8-a_shared_1/libFoo.so"
	if !strings.Contains(libFlags, libFoo1StubPath) {
		t.Errorf("%q is not found in %q", libFoo1StubPath, libFlags)
	}

	libBarCompileRule := ctx.ModuleForTests("libBar", "android_arm64_armv8-a_shared").Rule("cc")
	cFlags := libBarCompileRule.Args["cFlags"]
	libFoo1VersioningMacro := "-D__LIBFOO_API__=1"
	if !strings.Contains(cFlags, libFoo1VersioningMacro) {
		t.Errorf("%q is not found in %q", libFoo1VersioningMacro, cFlags)
	}
}

func TestVersioningMacro(t *testing.T) {
	for _, tc := range []struct{ moduleName, expected string }{
		{"libc", "__LIBC_API__"},
		{"libfoo", "__LIBFOO_API__"},
		{"libfoo@1", "__LIBFOO_1_API__"},
		{"libfoo-v1", "__LIBFOO_V1_API__"},
		{"libfoo.v1", "__LIBFOO_V1_API__"},
	} {
		checkEquals(t, tc.moduleName, tc.expected, versioningMacroName(tc.moduleName))
	}
}

func TestStaticExecutable(t *testing.T) {
	ctx := testCc(t, `
		cc_binary {
			name: "static_test",
			srcs: ["foo.c", "baz.o"],
			static_executable: true,
		}`)

	variant := "android_arm64_armv8-a"
	binModuleRule := ctx.ModuleForTests("static_test", variant).Rule("ld")
	libFlags := binModuleRule.Args["libFlags"]
	systemStaticLibs := []string{"libc.a", "libm.a"}
	for _, lib := range systemStaticLibs {
		if !strings.Contains(libFlags, lib) {
			t.Errorf("Static lib %q was not found in %q", lib, libFlags)
		}
	}
	systemSharedLibs := []string{"libc.so", "libm.so", "libdl.so"}
	for _, lib := range systemSharedLibs {
		if strings.Contains(libFlags, lib) {
			t.Errorf("Shared lib %q was found in %q", lib, libFlags)
		}
	}
}

func TestStaticDepsOrderWithStubs(t *testing.T) {
	ctx := testCc(t, `
		cc_binary {
			name: "mybin",
			srcs: ["foo.c"],
			static_libs: ["libfooC", "libfooB"],
			static_executable: true,
			stl: "none",
		}

		cc_library {
			name: "libfooB",
			srcs: ["foo.c"],
			shared_libs: ["libfooC"],
			stl: "none",
		}

		cc_library {
			name: "libfooC",
			srcs: ["foo.c"],
			stl: "none",
			stubs: {
				versions: ["1"],
			},
		}`)

	mybin := ctx.ModuleForTests("mybin", "android_arm64_armv8-a").Rule("ld")
	actual := mybin.Implicits[:2]
	expected := getOutputPaths(ctx, "android_arm64_armv8-a_static", []string{"libfooB", "libfooC"})

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("staticDeps orderings were not propagated correctly"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

func TestErrorsIfAModuleDependsOnDisabled(t *testing.T) {
	testCcError(t, `module "libA" .* depends on disabled module "libB"`, `
		cc_library {
			name: "libA",
			srcs: ["foo.c"],
			shared_libs: ["libB"],
			stl: "none",
		}

		cc_library {
			name: "libB",
			srcs: ["foo.c"],
			enabled: false,
			stl: "none",
		}
	`)
}

// Simple smoke test for the cc_fuzz target that ensures the rule compiles
// correctly.
func TestFuzzTarget(t *testing.T) {
	ctx := testCc(t, `
		cc_fuzz {
			name: "fuzz_smoke_test",
			srcs: ["foo.c"],
		}`)

	variant := "android_arm64_armv8-a_fuzzer"
	ctx.ModuleForTests("fuzz_smoke_test", variant).Rule("cc")
}

func TestAidl(t *testing.T) {
}

func assertString(t *testing.T, got, expected string) {
	t.Helper()
	if got != expected {
		t.Errorf("expected %q got %q", expected, got)
	}
}

func assertArrayString(t *testing.T, got, expected []string) {
	t.Helper()
	if len(got) != len(expected) {
		t.Errorf("expected %d (%q) got (%d) %q", len(expected), expected, len(got), got)
		return
	}
	for i := range got {
		if got[i] != expected[i] {
			t.Errorf("expected %d-th %q (%q) got %q (%q)",
				i, expected[i], expected, got[i], got)
			return
		}
	}
}

func assertMapKeys(t *testing.T, m map[string]string, expected []string) {
	t.Helper()
	assertArrayString(t, android.SortedStringKeys(m), expected)
}

func TestDefaults(t *testing.T) {
	ctx := testCc(t, `
		cc_defaults {
			name: "defaults",
			srcs: ["foo.c"],
			static: {
				srcs: ["bar.c"],
			},
			shared: {
				srcs: ["baz.c"],
			},
		}

		cc_library_static {
			name: "libstatic",
			defaults: ["defaults"],
		}

		cc_library_shared {
			name: "libshared",
			defaults: ["defaults"],
		}

		cc_library {
			name: "libboth",
			defaults: ["defaults"],
		}

		cc_binary {
			name: "binary",
			defaults: ["defaults"],
		}`)

	pathsToBase := func(paths android.Paths) []string {
		var ret []string
		for _, p := range paths {
			ret = append(ret, p.Base())
		}
		return ret
	}

	shared := ctx.ModuleForTests("libshared", "android_arm64_armv8-a_shared").Rule("ld")
	if g, w := pathsToBase(shared.Inputs), []string{"foo.o", "baz.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("libshared ld rule wanted %q, got %q", w, g)
	}
	bothShared := ctx.ModuleForTests("libboth", "android_arm64_armv8-a_shared").Rule("ld")
	if g, w := pathsToBase(bothShared.Inputs), []string{"foo.o", "baz.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("libboth ld rule wanted %q, got %q", w, g)
	}
	binary := ctx.ModuleForTests("binary", "android_arm64_armv8-a").Rule("ld")
	if g, w := pathsToBase(binary.Inputs), []string{"foo.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("binary ld rule wanted %q, got %q", w, g)
	}

	static := ctx.ModuleForTests("libstatic", "android_arm64_armv8-a_static").Rule("ar")
	if g, w := pathsToBase(static.Inputs), []string{"foo.o", "bar.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("libstatic ar rule wanted %q, got %q", w, g)
	}
	bothStatic := ctx.ModuleForTests("libboth", "android_arm64_armv8-a_static").Rule("ar")
	if g, w := pathsToBase(bothStatic.Inputs), []string{"foo.o", "bar.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("libboth ar rule wanted %q, got %q", w, g)
	}
}

func TestProductVariableDefaults(t *testing.T) {
	bp := `
		cc_defaults {
			name: "libfoo_defaults",
			srcs: ["foo.c"],
			cppflags: ["-DFOO"],
			product_variables: {
				debuggable: {
					cppflags: ["-DBAR"],
				},
			},
		}

		cc_library {
			name: "libfoo",
			defaults: ["libfoo_defaults"],
		}
	`

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.Debuggable = BoolPtr(true)

	ctx := CreateTestContext(config)
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("variable", android.VariableMutator).Parallel()
	})
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_static").Module().(*Module)
	if !android.InList("-DBAR", libfoo.flags.Local.CppFlags) {
		t.Errorf("expected -DBAR in cppflags, got %q", libfoo.flags.Local.CppFlags)
	}
}

func TestEmptyWholeStaticLibsAllowMissingDependencies(t *testing.T) {
	t.Parallel()
	bp := `
		cc_library_static {
			name: "libfoo",
			srcs: ["foo.c"],
			whole_static_libs: ["libbar"],
		}

		cc_library_static {
			name: "libbar",
			whole_static_libs: ["libmissing"],
		}
	`

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.Allow_missing_dependencies = BoolPtr(true)

	ctx := CreateTestContext(config)
	ctx.SetAllowMissingDependencies(true)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	libbar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_static").Output("libbar.a")
	if g, w := libbar.Rule, android.ErrorRule; g != w {
		t.Fatalf("Expected libbar rule to be %q, got %q", w, g)
	}

	if g, w := libbar.Args["error"], "missing dependencies: libmissing"; !strings.Contains(g, w) {
		t.Errorf("Expected libbar error to contain %q, was %q", w, g)
	}

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_static").Output("libfoo.a")
	if g, w := libfoo.Inputs.Strings(), libbar.Output.String(); !android.InList(w, g) {
		t.Errorf("Expected libfoo.a to depend on %q, got %q", w, g)
	}

}

func TestInstallSharedLibs(t *testing.T) {
	bp := `
		cc_binary {
			name: "bin",
			host_supported: true,
			shared_libs: ["libshared"],
			runtime_libs: ["libruntime"],
			srcs: [":gen"],
		}

		cc_library_shared {
			name: "libshared",
			host_supported: true,
			shared_libs: ["libtransitive"],
		}

		cc_library_shared {
			name: "libtransitive",
			host_supported: true,
		}

		cc_library_shared {
			name: "libruntime",
			host_supported: true,
		}

		cc_binary_host {
			name: "tool",
			srcs: ["foo.cpp"],
		}

		genrule {
			name: "gen",
			tools: ["tool"],
			out: ["gen.cpp"],
			cmd: "$(location tool) $(out)",
		}
	`

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	ctx := testCcWithConfig(t, config)

	hostBin := ctx.ModuleForTests("bin", config.BuildOSTarget.String()).Description("install")
	hostShared := ctx.ModuleForTests("libshared", config.BuildOSTarget.String()+"_shared").Description("install")
	hostRuntime := ctx.ModuleForTests("libruntime", config.BuildOSTarget.String()+"_shared").Description("install")
	hostTransitive := ctx.ModuleForTests("libtransitive", config.BuildOSTarget.String()+"_shared").Description("install")
	hostTool := ctx.ModuleForTests("tool", config.BuildOSTarget.String()).Description("install")

	if g, w := hostBin.Implicits.Strings(), hostShared.Output.String(); !android.InList(w, g) {
		t.Errorf("expected host bin dependency %q, got %q", w, g)
	}

	if g, w := hostBin.Implicits.Strings(), hostTransitive.Output.String(); !android.InList(w, g) {
		t.Errorf("expected host bin dependency %q, got %q", w, g)
	}

	if g, w := hostShared.Implicits.Strings(), hostTransitive.Output.String(); !android.InList(w, g) {
		t.Errorf("expected host bin dependency %q, got %q", w, g)
	}

	if g, w := hostBin.Implicits.Strings(), hostRuntime.Output.String(); !android.InList(w, g) {
		t.Errorf("expected host bin dependency %q, got %q", w, g)
	}

	if g, w := hostBin.Implicits.Strings(), hostTool.Output.String(); android.InList(w, g) {
		t.Errorf("expected no host bin dependency %q, got %q", w, g)
	}

	deviceBin := ctx.ModuleForTests("bin", "android_arm64_armv8-a").Description("install")
	deviceShared := ctx.ModuleForTests("libshared", "android_arm64_armv8-a_shared").Description("install")
	deviceTransitive := ctx.ModuleForTests("libtransitive", "android_arm64_armv8-a_shared").Description("install")
	deviceRuntime := ctx.ModuleForTests("libruntime", "android_arm64_armv8-a_shared").Description("install")

	if g, w := deviceBin.OrderOnly.Strings(), deviceShared.Output.String(); !android.InList(w, g) {
		t.Errorf("expected device bin dependency %q, got %q", w, g)
	}

	if g, w := deviceBin.OrderOnly.Strings(), deviceTransitive.Output.String(); !android.InList(w, g) {
		t.Errorf("expected device bin dependency %q, got %q", w, g)
	}

	if g, w := deviceShared.OrderOnly.Strings(), deviceTransitive.Output.String(); !android.InList(w, g) {
		t.Errorf("expected device bin dependency %q, got %q", w, g)
	}

	if g, w := deviceBin.OrderOnly.Strings(), deviceRuntime.Output.String(); !android.InList(w, g) {
		t.Errorf("expected device bin dependency %q, got %q", w, g)
	}

	if g, w := deviceBin.OrderOnly.Strings(), hostTool.Output.String(); android.InList(w, g) {
		t.Errorf("expected no device bin dependency %q, got %q", w, g)
	}

}

func TestStubsLibReexportsHeaders(t *testing.T) {
	ctx := testCc(t, `
		cc_library_shared {
			name: "libclient",
			srcs: ["foo.c"],
			shared_libs: ["libfoo#1"],
		}

		cc_library_shared {
			name: "libfoo",
			srcs: ["foo.c"],
			shared_libs: ["libbar"],
			export_shared_lib_headers: ["libbar"],
			stubs: {
				symbol_file: "foo.map.txt",
				versions: ["1", "2", "3"],
			},
		}

		cc_library_shared {
			name: "libbar",
			export_include_dirs: ["include/libbar"],
			srcs: ["foo.c"],
		}`)

	cFlags := ctx.ModuleForTests("libclient", "android_arm64_armv8-a_shared").Rule("cc").Args["cFlags"]

	if !strings.Contains(cFlags, "-Iinclude/libbar") {
		t.Errorf("expected %q in cflags, got %q", "-Iinclude/libbar", cFlags)
	}
}

func TestAidlFlagsPassedToTheAidlCompiler(t *testing.T) {
	ctx := testCc(t, `
		cc_library {
			name: "libfoo",
			srcs: ["a/Foo.aidl"],
			aidl: { flags: ["-Werror"], },
		}
	`)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_static")
	manifest := android.RuleBuilderSboxProtoForTests(t, libfoo.Output("aidl.sbox.textproto"))
	aidlCommand := manifest.Commands[0].GetCommand()
	expectedAidlFlag := "-Werror"
	if !strings.Contains(aidlCommand, expectedAidlFlag) {
		t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlag)
	}
}

type MemtagNoteType int

const (
	None MemtagNoteType = iota + 1
	Sync
	Async
)

func (t MemtagNoteType) str() string {
	switch t {
	case None:
		return "none"
	case Sync:
		return "sync"
	case Async:
		return "async"
	default:
		panic("invalid note type")
	}
}

func checkHasMemtagNote(t *testing.T, m android.TestingModule, expected MemtagNoteType) {
	note_async := "note_memtag_heap_async"
	note_sync := "note_memtag_heap_sync"

	found := None
	implicits := m.Rule("ld").Implicits
	for _, lib := range implicits {
		if strings.Contains(lib.Rel(), note_async) {
			found = Async
			break
		} else if strings.Contains(lib.Rel(), note_sync) {
			found = Sync
			break
		}
	}

	if found != expected {
		t.Errorf("Wrong Memtag note in target %q: found %q, expected %q", m.Module().(*Module).Name(), found.str(), expected.str())
	}
}

func makeMemtagTestConfig(t *testing.T) android.Config {
	templateBp := `
		cc_test {
			name: "%[1]s_test",
			gtest: false,
		}

		cc_test {
			name: "%[1]s_test_false",
			gtest: false,
			sanitize: { memtag_heap: false },
		}

		cc_test {
			name: "%[1]s_test_true",
			gtest: false,
			sanitize: { memtag_heap: true },
		}

		cc_test {
			name: "%[1]s_test_true_nodiag",
			gtest: false,
			sanitize: { memtag_heap: true, diag: { memtag_heap: false }  },
		}

		cc_test {
			name: "%[1]s_test_true_diag",
			gtest: false,
			sanitize: { memtag_heap: true, diag: { memtag_heap: true }  },
		}

		cc_binary {
			name: "%[1]s_binary",
		}

		cc_binary {
			name: "%[1]s_binary_false",
			sanitize: { memtag_heap: false },
		}

		cc_binary {
			name: "%[1]s_binary_true",
			sanitize: { memtag_heap: true },
		}

		cc_binary {
			name: "%[1]s_binary_true_nodiag",
			sanitize: { memtag_heap: true, diag: { memtag_heap: false }  },
		}

		cc_binary {
			name: "%[1]s_binary_true_diag",
			sanitize: { memtag_heap: true, diag: { memtag_heap: true }  },
		}
		`
	subdirDefaultBp := fmt.Sprintf(templateBp, "default")
	subdirExcludeBp := fmt.Sprintf(templateBp, "exclude")
	subdirSyncBp := fmt.Sprintf(templateBp, "sync")
	subdirAsyncBp := fmt.Sprintf(templateBp, "async")

	mockFS := map[string][]byte{
		"subdir_default/Android.bp": []byte(subdirDefaultBp),
		"subdir_exclude/Android.bp": []byte(subdirExcludeBp),
		"subdir_sync/Android.bp":    []byte(subdirSyncBp),
		"subdir_async/Android.bp":   []byte(subdirAsyncBp),
	}

	return TestConfig(buildDir, android.Android, nil, "", mockFS)
}

func TestSanitizeMemtagHeap(t *testing.T) {
	variant := "android_arm64_armv8-a"

	config := makeMemtagTestConfig(t)
	config.TestProductVariables.MemtagHeapExcludePaths = []string{"subdir_exclude"}
	config.TestProductVariables.MemtagHeapSyncIncludePaths = []string{"subdir_sync"}
	config.TestProductVariables.MemtagHeapAsyncIncludePaths = []string{"subdir_async"}
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"Android.bp", "subdir_default/Android.bp", "subdir_exclude/Android.bp", "subdir_sync/Android.bp", "subdir_async/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	checkHasMemtagNote(t, ctx.ModuleForTests("default_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("async_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_true_diag", variant), Sync)
}

func TestSanitizeMemtagHeapWithSanitizeDevice(t *testing.T) {
	variant := "android_arm64_armv8-a"

	config := makeMemtagTestConfig(t)
	config.TestProductVariables.MemtagHeapExcludePaths = []string{"subdir_exclude"}
	config.TestProductVariables.MemtagHeapSyncIncludePaths = []string{"subdir_sync"}
	config.TestProductVariables.MemtagHeapAsyncIncludePaths = []string{"subdir_async"}
	config.TestProductVariables.SanitizeDevice = []string{"memtag_heap"}
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"Android.bp", "subdir_default/Android.bp", "subdir_exclude/Android.bp", "subdir_sync/Android.bp", "subdir_async/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	checkHasMemtagNote(t, ctx.ModuleForTests("default_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("async_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_true", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_true_diag", variant), Sync)
}

func TestSanitizeMemtagHeapWithSanitizeDeviceDiag(t *testing.T) {
	variant := "android_arm64_armv8-a"

	config := makeMemtagTestConfig(t)
	config.TestProductVariables.MemtagHeapExcludePaths = []string{"subdir_exclude"}
	config.TestProductVariables.MemtagHeapSyncIncludePaths = []string{"subdir_sync"}
	config.TestProductVariables.MemtagHeapAsyncIncludePaths = []string{"subdir_async"}
	config.TestProductVariables.SanitizeDevice = []string{"memtag_heap"}
	config.TestProductVariables.SanitizeDeviceDiag = []string{"memtag_heap"}
	ctx := CreateTestContext(config)
	ctx.Register()

	_, errs := ctx.ParseFileList(".", []string{"Android.bp", "subdir_default/Android.bp", "subdir_exclude/Android.bp", "subdir_sync/Android.bp", "subdir_async/Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	checkHasMemtagNote(t, ctx.ModuleForTests("default_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("default_binary_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("exclude_binary_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("async_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("async_binary_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_test_true_diag", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_false", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_true", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_true_nodiag", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("sync_binary_true_diag", variant), Sync)
}

func TestIncludeDirsExporting(t *testing.T) {

	// Trim spaces from the beginning, end and immediately after any newline characters. Leaves
	// embedded newline characters alone.
	trimIndentingSpaces := func(s string) string {
		return strings.TrimSpace(regexp.MustCompile("(^|\n)\\s+").ReplaceAllString(s, "$1"))
	}

	checkPaths := func(t *testing.T, message string, expected string, paths android.Paths) {
		t.Helper()
		expected = trimIndentingSpaces(expected)
		actual := trimIndentingSpaces(strings.Join(android.FirstUniqueStrings(android.NormalizePathsForTesting(paths)), "\n"))
		if expected != actual {
			t.Errorf("%s: expected:\n%s\n actual:\n%s\n", message, expected, actual)
		}
	}

	type exportedChecker func(t *testing.T, name string, exported FlagExporterInfo)

	checkIncludeDirs := func(t *testing.T, ctx *android.TestContext, module android.Module, checkers ...exportedChecker) {
		t.Helper()
		exported := ctx.ModuleProvider(module, FlagExporterInfoProvider).(FlagExporterInfo)
		name := module.Name()

		for _, checker := range checkers {
			checker(t, name, exported)
		}
	}

	expectedIncludeDirs := func(expectedPaths string) exportedChecker {
		return func(t *testing.T, name string, exported FlagExporterInfo) {
			t.Helper()
			checkPaths(t, fmt.Sprintf("%s: include dirs", name), expectedPaths, exported.IncludeDirs)
		}
	}

	expectedSystemIncludeDirs := func(expectedPaths string) exportedChecker {
		return func(t *testing.T, name string, exported FlagExporterInfo) {
			t.Helper()
			checkPaths(t, fmt.Sprintf("%s: system include dirs", name), expectedPaths, exported.SystemIncludeDirs)
		}
	}

	expectedGeneratedHeaders := func(expectedPaths string) exportedChecker {
		return func(t *testing.T, name string, exported FlagExporterInfo) {
			t.Helper()
			checkPaths(t, fmt.Sprintf("%s: generated headers", name), expectedPaths, exported.GeneratedHeaders)
		}
	}

	expectedOrderOnlyDeps := func(expectedPaths string) exportedChecker {
		return func(t *testing.T, name string, exported FlagExporterInfo) {
			t.Helper()
			checkPaths(t, fmt.Sprintf("%s: order only deps", name), expectedPaths, exported.Deps)
		}
	}

	genRuleModules := `
		genrule {
			name: "genrule_foo",
			cmd: "generate-foo",
			out: [
				"generated_headers/foo/generated_header.h",
			],
			export_include_dirs: [
				"generated_headers",
			],
		}

		genrule {
			name: "genrule_bar",
			cmd: "generate-bar",
			out: [
				"generated_headers/bar/generated_header.h",
			],
			export_include_dirs: [
				"generated_headers",
			],
		}
	`

	t.Run("ensure exported include dirs are not automatically re-exported from shared_libs", func(t *testing.T) {
		ctx := testCc(t, genRuleModules+`
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			export_include_dirs: ["foo/standard"],
			export_system_include_dirs: ["foo/system"],
			generated_headers: ["genrule_foo"],
			export_generated_headers: ["genrule_foo"],
		}

		cc_library {
			name: "libbar",
			srcs: ["bar.c"],
			shared_libs: ["libfoo"],
			export_include_dirs: ["bar/standard"],
			export_system_include_dirs: ["bar/system"],
			generated_headers: ["genrule_bar"],
			export_generated_headers: ["genrule_bar"],
		}
		`)
		foo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				foo/standard
				.intermediates/genrule_foo/gen/generated_headers
			`),
			expectedSystemIncludeDirs(`foo/system`),
			expectedGeneratedHeaders(`.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h`),
			expectedOrderOnlyDeps(`.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h`),
		)

		bar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, bar,
			expectedIncludeDirs(`
				bar/standard
				.intermediates/genrule_bar/gen/generated_headers
			`),
			expectedSystemIncludeDirs(`bar/system`),
			expectedGeneratedHeaders(`.intermediates/genrule_bar/gen/generated_headers/bar/generated_header.h`),
			expectedOrderOnlyDeps(`.intermediates/genrule_bar/gen/generated_headers/bar/generated_header.h`),
		)
	})

	t.Run("ensure exported include dirs are automatically re-exported from whole_static_libs", func(t *testing.T) {
		ctx := testCc(t, genRuleModules+`
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			export_include_dirs: ["foo/standard"],
			export_system_include_dirs: ["foo/system"],
			generated_headers: ["genrule_foo"],
			export_generated_headers: ["genrule_foo"],
		}

		cc_library {
			name: "libbar",
			srcs: ["bar.c"],
			whole_static_libs: ["libfoo"],
			export_include_dirs: ["bar/standard"],
			export_system_include_dirs: ["bar/system"],
			generated_headers: ["genrule_bar"],
			export_generated_headers: ["genrule_bar"],
		}
		`)
		foo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				foo/standard
				.intermediates/genrule_foo/gen/generated_headers
			`),
			expectedSystemIncludeDirs(`foo/system`),
			expectedGeneratedHeaders(`.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h`),
			expectedOrderOnlyDeps(`.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h`),
		)

		bar := ctx.ModuleForTests("libbar", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, bar,
			expectedIncludeDirs(`
				bar/standard
				foo/standard
				.intermediates/genrule_foo/gen/generated_headers
				.intermediates/genrule_bar/gen/generated_headers
			`),
			expectedSystemIncludeDirs(`
				bar/system
				foo/system
			`),
			expectedGeneratedHeaders(`
				.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h
				.intermediates/genrule_bar/gen/generated_headers/bar/generated_header.h
			`),
			expectedOrderOnlyDeps(`
				.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h
				.intermediates/genrule_bar/gen/generated_headers/bar/generated_header.h
			`),
		)
	})

	t.Run("ensure only aidl headers are exported", func(t *testing.T) {
		ctx := testCc(t, genRuleModules+`
		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				"b.aidl",
				"a.proto",
			],
			aidl: {
				export_aidl_headers: true,
			}
		}
		`)
		foo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl
			`),
			expectedSystemIncludeDirs(``),
			expectedGeneratedHeaders(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/b.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/Bnb.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/Bpb.h
			`),
			expectedOrderOnlyDeps(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/b.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/Bnb.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/Bpb.h
			`),
		)
	})

	t.Run("ensure only proto headers are exported", func(t *testing.T) {
		ctx := testCc(t, genRuleModules+`
		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				"b.aidl",
				"a.proto",
			],
			proto: {
				export_proto_headers: true,
			}
		}
		`)
		foo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/proto
			`),
			expectedSystemIncludeDirs(``),
			expectedGeneratedHeaders(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/proto/a.pb.h
			`),
			expectedOrderOnlyDeps(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/proto/a.pb.h
			`),
		)
	})

	t.Run("ensure only sysprop headers are exported", func(t *testing.T) {
		ctx := testCc(t, genRuleModules+`
		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				"a.sysprop",
				"b.aidl",
				"a.proto",
			],
		}
		`)
		foo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/sysprop/include
			`),
			expectedSystemIncludeDirs(``),
			expectedGeneratedHeaders(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/sysprop/include/a.sysprop.h
			`),
			expectedOrderOnlyDeps(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/sysprop/include/a.sysprop.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/sysprop/public/include/a.sysprop.h
			`),
		)
	})
}
