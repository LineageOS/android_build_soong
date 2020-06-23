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
	"sort"
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

func testCcWithConfig(t *testing.T, config android.Config) *android.TestContext {
	t.Helper()
	ctx := CreateTestContext()
	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	return ctx
}

func testCc(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")

	return testCcWithConfig(t, config)
}

func testCcNoVndk(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")

	return testCcWithConfig(t, config)
}

func testCcErrorWithConfig(t *testing.T, pattern string, config android.Config) {
	t.Helper()

	ctx := CreateTestContext()
	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}

	_, errs = ctx.PrepareBuildActions(config)
	if len(errs) > 0 {
		android.FailIfNoMatchingErrors(t, pattern, errs)
		return
	}

	t.Fatalf("missing expected error %q (0 errors are returned)", pattern)
}

func testCcError(t *testing.T, pattern string, bp string) {
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	testCcErrorWithConfig(t, pattern, config)
	return
}

func testCcErrorProductVndk(t *testing.T, pattern string, bp string) {
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

	config := TestConfig(buildDir, android.Fuchsia, nil, bp, nil)
	ctx := testCcWithConfig(t, config)

	rt := false
	fb := false

	ld := ctx.ModuleForTests("libTest", "fuchsia_arm64_shared").Rule("ld")
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

	config := TestConfig(buildDir, android.Fuchsia, nil, bp, nil)
	ctx := testCcWithConfig(t, config)
	ld := ctx.ModuleForTests("libTest", "fuchsia_arm64_shared").Rule("ld")
	var objs []string
	for _, o := range ld.Inputs {
		objs = append(objs, o.Base())
	}
	if len(objs) != 2 || objs[0] != "foo.o" || objs[1] != "bar.o" {
		t.Errorf("inputs of libTest must be []string{\"foo.o\", \"bar.o\"}, but was %#v.", objs)
	}
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
	if !mod.HasVendorVariant() {
		t.Errorf("%q must have variant %q", name, variant)
	}

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
	if mod.isVndkExt() != isVndkExt {
		t.Errorf("%q isVndkExt() must equal to %t", name, isVndkExt)
	}

	if actualExtends := mod.getVndkExtendsModuleName(); actualExtends != extends {
		t.Errorf("%q must extend from %q but get %q", name, extends, actualExtends)
	}
}

func checkSnapshot(t *testing.T, ctx *android.TestContext, singleton android.TestingSingleton, moduleName, snapshotFilename, subDir, variant string) {
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

	out := singleton.Output(snapshotPath)
	if out.Input.String() != outputFiles[0].String() {
		t.Errorf("The input of snapshot %q must be %q, but %q", moduleName, out.Input.String(), outputFiles[0])
	}
}

func checkWriteFileOutput(t *testing.T, params android.TestingBuildParams, expected []string) {
	t.Helper()
	assertString(t, params.Rule.String(), android.WriteFile.String())
	actual := strings.FieldsFunc(strings.ReplaceAll(params.Args["content"], "\\n", "\n"), func(r rune) bool { return r == '\n' })
	assertArrayString(t, actual, expected)
}

func checkVndkOutput(t *testing.T, ctx *android.TestContext, output string, expected []string) {
	t.Helper()
	vndkSnapshot := ctx.SingletonForTests("vndk-snapshot")
	checkWriteFileOutput(t, vndkSnapshot.Output(output), expected)
}

func checkVndkLibrariesOutput(t *testing.T, ctx *android.TestContext, module string, expected []string) {
	t.Helper()
	vndkLibraries := ctx.ModuleForTests(module, "")

	var output string
	if module != "vndkcorevariant.libraries.txt" {
		output = insertVndkVersion(module, "VER")
	} else {
		output = module
	}

	checkWriteFileOutput(t, vndkLibraries.Output(output), expected)
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
			vendor_available: false,
			vndk: {
				enabled: true,
			},
			nocrt: true,
			stem: "libvndk-private",
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
			vendor_available: false,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
			target: {
				vendor: {
					suffix: "-x",
				},
			},
		}
		vndk_libraries_txt {
			name: "llndk.libraries.txt",
		}
		vndk_libraries_txt {
			name: "vndkcore.libraries.txt",
		}
		vndk_libraries_txt {
			name: "vndksp.libraries.txt",
		}
		vndk_libraries_txt {
			name: "vndkprivate.libraries.txt",
		}
		vndk_libraries_txt {
			name: "vndkcorevariant.libraries.txt",
		}
	`

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")

	ctx := testCcWithConfig(t, config)

	checkVndkModule(t, ctx, "libvndk", "vndk-VER", false, "", vendorVariant)
	checkVndkModule(t, ctx, "libvndk_private", "vndk-VER", false, "", vendorVariant)
	checkVndkModule(t, ctx, "libvndk_sp", "vndk-sp-VER", true, "", vendorVariant)
	checkVndkModule(t, ctx, "libvndk_sp_private", "vndk-sp-VER", true, "", vendorVariant)

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
	checkSnapshot(t, ctx, snapshotSingleton, "libvndk_sp", "libvndk_sp-x.so", vndkSpLibPath, variant)
	checkSnapshot(t, ctx, snapshotSingleton, "libvndk_sp", "libvndk_sp-x.so", vndkSpLib2ndPath, variant2nd)

	snapshotConfigsPath := filepath.Join(snapshotVariantPath, "configs")
	checkSnapshot(t, ctx, snapshotSingleton, "llndk.libraries.txt", "llndk.libraries.txt", snapshotConfigsPath, "")
	checkSnapshot(t, ctx, snapshotSingleton, "vndkcore.libraries.txt", "vndkcore.libraries.txt", snapshotConfigsPath, "")
	checkSnapshot(t, ctx, snapshotSingleton, "vndksp.libraries.txt", "vndksp.libraries.txt", snapshotConfigsPath, "")
	checkSnapshot(t, ctx, snapshotSingleton, "vndkprivate.libraries.txt", "vndkprivate.libraries.txt", snapshotConfigsPath, "")

	checkVndkOutput(t, ctx, "vndk/vndk.libraries.txt", []string{
		"LLNDK: libc.so",
		"LLNDK: libdl.so",
		"LLNDK: libft2.so",
		"LLNDK: libm.so",
		"VNDK-SP: libc++.so",
		"VNDK-SP: libvndk_sp-x.so",
		"VNDK-SP: libvndk_sp_private-x.so",
		"VNDK-core: libvndk-private.so",
		"VNDK-core: libvndk.so",
		"VNDK-private: libft2.so",
		"VNDK-private: libvndk-private.so",
		"VNDK-private: libvndk_sp_private-x.so",
	})
	checkVndkLibrariesOutput(t, ctx, "llndk.libraries.txt", []string{"libc.so", "libdl.so", "libft2.so", "libm.so"})
	checkVndkLibrariesOutput(t, ctx, "vndkcore.libraries.txt", []string{"libvndk-private.so", "libvndk.so"})
	checkVndkLibrariesOutput(t, ctx, "vndkprivate.libraries.txt", []string{"libft2.so", "libvndk-private.so", "libvndk_sp_private-x.so"})
	checkVndkLibrariesOutput(t, ctx, "vndksp.libraries.txt", []string{"libc++.so", "libvndk_sp-x.so", "libvndk_sp_private-x.so"})
	checkVndkLibrariesOutput(t, ctx, "vndkcorevariant.libraries.txt", nil)
}

func TestVndkWithHostSupported(t *testing.T) {
	ctx := testCc(t, `
		cc_library {
			name: "libvndk_host_supported",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			host_supported: true,
		}

		cc_library {
			name: "libvndk_host_supported_but_disabled_on_device",
			vendor_available: true,
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

		vndk_libraries_txt {
			name: "vndkcore.libraries.txt",
		}
	`)

	checkVndkLibrariesOutput(t, ctx, "vndkcore.libraries.txt", []string{"libvndk_host_supported.so"})
}

func TestVndkLibrariesTxtAndroidMk(t *testing.T) {
	bp := `
		vndk_libraries_txt {
			name: "llndk.libraries.txt",
		}`
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	ctx := testCcWithConfig(t, config)

	module := ctx.ModuleForTests("llndk.libraries.txt", "")
	entries := android.AndroidMkEntriesForTest(t, config, "", module.Module())[0]
	assertArrayString(t, entries.EntryMap["LOCAL_MODULE_STEM"], []string{"llndk.libraries.VER.txt"})
}

func TestVndkUsingCoreVariant(t *testing.T) {
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
			name: "libvndk_sp",
			vendor_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk2",
			vendor_available: false,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		vndk_libraries_txt {
			name: "vndkcorevariant.libraries.txt",
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

func TestVndkWhenVndkVersionIsNotSet(t *testing.T) {
	ctx := testCcNoVndk(t, `
		cc_library {
			name: "libvndk",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
	`)

	checkVndkOutput(t, ctx, "vndk/vndk.libraries.txt", []string{
		"LLNDK: libc.so",
		"LLNDK: libdl.so",
		"LLNDK: libft2.so",
		"LLNDK: libm.so",
		"VNDK-SP: libc++.so",
		"VNDK-core: libvndk.so",
		"VNDK-private: libft2.so",
	})
}

func TestVndkDepError(t *testing.T) {
	// Check whether an error is emitted when a VNDK lib depends on a system lib.
	testCcError(t, "dependency \".*\" of \".*\" missing variant", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
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
			vendor_available: false,
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

	// Check whether an error is emitted when a VNDK-sp lib depends on a non-VNDK lib.
	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndksp",
			vendor_available: true,
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
			vendor_available: false,
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
}

func TestDoubleLoadbleDep(t *testing.T) {
	// okay to link : LLNDK -> double_loadable VNDK
	testCc(t, `
		cc_library {
			name: "libllndk",
			shared_libs: ["libdoubleloadable"],
		}

		llndk_library {
			name: "libllndk",
			symbol_file: "",
		}

		cc_library {
			name: "libdoubleloadable",
			vendor_available: true,
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
		}

		llndk_library {
			name: "libllndk",
			symbol_file: "",
		}

		cc_library {
			name: "libvndksp",
			vendor_available: true,
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
			vndk: {
				enabled: true,
			},
			double_loadable: true,
			shared_libs: ["libnondoubleloadable"],
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: false,
			vndk: {
				enabled: true,
			},
			double_loadable: true,
		}
	`)
	// okay to link : LLNDK -> core-only -> vendor_available & double_loadable
	testCc(t, `
		cc_library {
			name: "libllndk",
			shared_libs: ["libcoreonly"],
		}

		llndk_library {
			name: "libllndk",
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

func TestVendorSnapshot(t *testing.T) {
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
		name: "libvendor",
		vendor: true,
		nocrt: true,
	}

	cc_library {
		name: "libvendor_available",
		vendor_available: true,
		nocrt: true,
	}

	cc_library_headers {
		name: "libvendor_headers",
		vendor_available: true,
		nocrt: true,
	}

	cc_binary {
		name: "vendor_bin",
		vendor: true,
		nocrt: true,
	}

	cc_binary {
		name: "vendor_available_bin",
		vendor_available: true,
		nocrt: true,
	}

	toolchain_library {
		name: "libb",
		vendor_available: true,
		src: "libb.a",
	}

	cc_object {
		name: "obj",
		vendor_available: true,
	}
`
	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	ctx := testCcWithConfig(t, config)

	// Check Vendor snapshot output.

	snapshotDir := "vendor-snapshot"
	snapshotVariantPath := filepath.Join(buildDir, snapshotDir, "arm64")
	snapshotSingleton := ctx.SingletonForTests("vendor-snapshot")

	var jsonFiles []string

	for _, arch := range [][]string{
		[]string{"arm64", "armv8-a"},
		[]string{"arm", "armv7-a-neon"},
	} {
		archType := arch[0]
		archVariant := arch[1]
		archDir := fmt.Sprintf("arch-%s-%s", archType, archVariant)

		// For shared libraries, only non-VNDK vendor_available modules are captured
		sharedVariant := fmt.Sprintf("android_vendor.VER_%s_%s_shared", archType, archVariant)
		sharedDir := filepath.Join(snapshotVariantPath, archDir, "shared")
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.so", sharedDir, sharedVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.so", sharedDir, sharedVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(sharedDir, "libvendor.so.json"),
			filepath.Join(sharedDir, "libvendor_available.so.json"))

		// For static libraries, all vendor:true and vendor_available modules (including VNDK) are captured.
		staticVariant := fmt.Sprintf("android_vendor.VER_%s_%s_static", archType, archVariant)
		staticDir := filepath.Join(snapshotVariantPath, archDir, "static")
		checkSnapshot(t, ctx, snapshotSingleton, "libb", "libb.a", staticDir, staticVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvndk", "libvndk.a", staticDir, staticVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor", "libvendor.a", staticDir, staticVariant)
		checkSnapshot(t, ctx, snapshotSingleton, "libvendor_available", "libvendor_available.a", staticDir, staticVariant)
		jsonFiles = append(jsonFiles,
			filepath.Join(staticDir, "libb.a.json"),
			filepath.Join(staticDir, "libvndk.a.json"),
			filepath.Join(staticDir, "libvendor.a.json"),
			filepath.Join(staticDir, "libvendor_available.a.json"))

		// For binary executables, all vendor:true and vendor_available modules are captured.
		if archType == "arm64" {
			binaryVariant := fmt.Sprintf("android_vendor.VER_%s_%s", archType, archVariant)
			binaryDir := filepath.Join(snapshotVariantPath, archDir, "binary")
			checkSnapshot(t, ctx, snapshotSingleton, "vendor_bin", "vendor_bin", binaryDir, binaryVariant)
			checkSnapshot(t, ctx, snapshotSingleton, "vendor_available_bin", "vendor_available_bin", binaryDir, binaryVariant)
			jsonFiles = append(jsonFiles,
				filepath.Join(binaryDir, "vendor_bin.json"),
				filepath.Join(binaryDir, "vendor_available_bin.json"))
		}

		// For header libraries, all vendor:true and vendor_available modules are captured.
		headerDir := filepath.Join(snapshotVariantPath, archDir, "header")
		jsonFiles = append(jsonFiles, filepath.Join(headerDir, "libvendor_headers.json"))

		// For object modules, all vendor:true and vendor_available modules are captured.
		objectVariant := fmt.Sprintf("android_vendor.VER_%s_%s", archType, archVariant)
		objectDir := filepath.Join(snapshotVariantPath, archDir, "object")
		checkSnapshot(t, ctx, snapshotSingleton, "obj", "obj.o", objectDir, objectVariant)
		jsonFiles = append(jsonFiles, filepath.Join(objectDir, "obj.o.json"))
	}

	for _, jsonFile := range jsonFiles {
		// verify all json files exist
		if snapshotSingleton.MaybeOutput(jsonFile).Rule == nil {
			t.Errorf("%q expected but not found", jsonFile)
		}
	}
}

func TestDoubleLoadableDepError(t *testing.T) {
	// Check whether an error is emitted when a LLNDK depends on a non-double_loadable VNDK lib.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libllndk",
			shared_libs: ["libnondoubleloadable"],
		}

		llndk_library {
			name: "libllndk",
			symbol_file: "",
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
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
		}

		llndk_library {
			name: "libllndk",
			symbol_file: "",
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
		}
	`)

	// Check whether an error is emitted when a double_loadable lib depends on a non-double_loadable vendor_available lib.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libdoubleloadable",
			vendor_available: true,
			double_loadable: true,
			shared_libs: ["libnondoubleloadable"],
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
		}
	`)

	// Check whether an error is emitted when a double_loadable lib depends on a non-double_loadable VNDK lib.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libdoubleloadable",
			vendor_available: true,
			double_loadable: true,
			shared_libs: ["libnondoubleloadable"],
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
		}
	`)

	// Check whether an error is emitted when a double_loadable VNDK depends on a non-double_loadable VNDK private lib.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libdoubleloadable",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			double_loadable: true,
			shared_libs: ["libnondoubleloadable"],
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: false,
			vndk: {
				enabled: true,
			},
		}
	`)

	// Check whether an error is emitted when a LLNDK depends on a non-double_loadable indirectly.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libllndk",
			shared_libs: ["libcoreonly"],
		}

		llndk_library {
			name: "libllndk",
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
}

func TestVndkExt(t *testing.T) {
	// This test checks the VNDK-Ext properties.
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
			name: "libvndk2",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			target: {
				vendor: {
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
	ctx := testCc(t, `
		cc_library {
			name: "libvndk",
			vendor_available: true,
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
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_ext_product",
			product_specific: true,
			vendor_available: true,
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
	// with `vendor_available: false`.
	testCcError(t, "`extends` refers module \".*\" which does not have `vendor_available: true`", `
		cc_library {
			name: "libvndk",
			vendor_available: false,
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

	testCcErrorProductVndk(t, "`extends` refers module \".*\" which does not have `vendor_available: true`", `
		cc_library {
			name: "libvndk",
			vendor_available: false,
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
}

func TestVendorModuleUseVndkExt(t *testing.T) {
	// This test ensures a vendor module can depend on a VNDK-Ext library.
	testCc(t, `
		cc_library {
			name: "libvndk",
			vendor_available: true,
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
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}

		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
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
			shared_libs: ["libvndk_ext"],
			nocrt: true,
		}
	`)

	testCcError(t, "module \".*\" variant \".*\": \\(.*\\) should not link to \".*\"", `
		cc_library {
			name: "libvndk",
			vendor_available: true,
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
		}
		llndk_library {
			name: "libllndk",
			symbol_file: "",
		}
		cc_library {
			name: "libvndk",
			vendor_available: true,
			vndk: {
				enabled: true,
			},
			nocrt: true,
		}
		cc_library {
			name: "libvndk_sp",
			vendor_available: true,
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
				"libva",
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

	checkVndkModule(t, ctx, "libvndk", "vndk-VER", false, "", productVariant)
	checkVndkModule(t, ctx, "libvndk_sp", "vndk-sp-VER", true, "", productVariant)
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
	testCcErrorProductVndk(t, "Vendor module that is not VNDK should not link to \".*\" which is marked as `vendor_available: false`", `
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
			vendor_available: false,
			vndk: {
				enabled: true,
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
			vndk: {
				enabled: true,
			},
		}
		cc_library {
			name: "libvndksp",
			vendor_available: true,
			vndk: {
				enabled: true,
				support_system_process: true,
			},
		}
		cc_library {
			name: "libvndkprivate",
			vendor_available: false,
			vndk: {
				enabled: true,
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
		}
		llndk_library {
			name: "libllndk",
			symbol_file: "",
		}
		cc_library {
			name: "libllndkprivate",
		}
		llndk_library {
			name: "libllndkprivate",
			vendor_available: false,
			symbol_file: "",
		}`

	config := TestConfig(buildDir, android.Android, nil, bp, nil)
	config.TestProductVariables.DeviceVndkVersion = StringPtr("current")
	config.TestProductVariables.Platform_vndk_version = StringPtr("VER")
	// native:vndk
	ctx := testCcWithConfig(t, config)

	assertMapKeys(t, vndkCoreLibraries(config),
		[]string{"libvndk", "libvndkprivate"})
	assertMapKeys(t, vndkSpLibraries(config),
		[]string{"libc++", "libvndksp"})
	assertMapKeys(t, llndkLibraries(config),
		[]string{"libc", "libdl", "libft2", "libllndk", "libllndkprivate", "libm"})
	assertMapKeys(t, vndkPrivateLibraries(config),
		[]string{"libft2", "libllndkprivate", "libvndkprivate"})

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
		{vendorVariant, "libllndk.llndk", "native:vndk"},
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

var (
	str11 = "01234567891"
	str10 = str11[:10]
	str9  = str11[:9]
	str5  = str11[:5]
	str4  = str11[:4]
)

var splitListForSizeTestCases = []struct {
	in   []string
	out  [][]string
	size int
}{
	{
		in:   []string{str10},
		out:  [][]string{{str10}},
		size: 10,
	},
	{
		in:   []string{str9},
		out:  [][]string{{str9}},
		size: 10,
	},
	{
		in:   []string{str5},
		out:  [][]string{{str5}},
		size: 10,
	},
	{
		in:   []string{str11},
		out:  nil,
		size: 10,
	},
	{
		in:   []string{str10, str10},
		out:  [][]string{{str10}, {str10}},
		size: 10,
	},
	{
		in:   []string{str9, str10},
		out:  [][]string{{str9}, {str10}},
		size: 10,
	},
	{
		in:   []string{str10, str9},
		out:  [][]string{{str10}, {str9}},
		size: 10,
	},
	{
		in:   []string{str5, str4},
		out:  [][]string{{str5, str4}},
		size: 10,
	},
	{
		in:   []string{str5, str4, str5},
		out:  [][]string{{str5, str4}, {str5}},
		size: 10,
	},
	{
		in:   []string{str5, str4, str5, str4},
		out:  [][]string{{str5, str4}, {str5, str4}},
		size: 10,
	},
	{
		in:   []string{str5, str4, str5, str5},
		out:  [][]string{{str5, str4}, {str5}, {str5}},
		size: 10,
	},
	{
		in:   []string{str5, str5, str5, str4},
		out:  [][]string{{str5}, {str5}, {str5, str4}},
		size: 10,
	},
	{
		in:   []string{str9, str11},
		out:  nil,
		size: 10,
	},
	{
		in:   []string{str11, str9},
		out:  nil,
		size: 10,
	},
}

func TestSplitListForSize(t *testing.T) {
	for _, testCase := range splitListForSizeTestCases {
		out, _ := splitListForSize(android.PathsForTesting(testCase.in...), testCase.size)

		var outStrings [][]string

		if len(out) > 0 {
			outStrings = make([][]string, len(out))
			for i, o := range out {
				outStrings[i] = o.Strings()
			}
		}

		if !reflect.DeepEqual(outStrings, testCase.out) {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("      size: %d", testCase.size)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", outStrings)
		}
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

func TestLinkReordering(t *testing.T) {
	for _, testCase := range staticLinkDepOrderTestCases {
		errs := []string{}

		// parse testcase
		_, givenTransitiveDeps := parseModuleDeps(testCase.inStatic)
		expectedModuleNames, expectedTransitiveDeps := parseModuleDeps(testCase.outOrdered)
		if testCase.allOrdered == "" {
			// allow the test case to skip specifying allOrdered
			testCase.allOrdered = testCase.outOrdered
		}
		_, expectedAllDeps := parseModuleDeps(testCase.allOrdered)
		_, givenAllSharedDeps := parseModuleDeps(testCase.inShared)

		// For each module whose post-reordered dependencies were specified, validate that
		// reordering the inputs produces the expected outputs.
		for _, moduleName := range expectedModuleNames {
			moduleDeps := givenTransitiveDeps[moduleName]
			givenSharedDeps := givenAllSharedDeps[moduleName]
			orderedAllDeps, orderedDeclaredDeps := orderDeps(moduleDeps, givenSharedDeps, givenTransitiveDeps)

			correctAllOrdered := expectedAllDeps[moduleName]
			if !reflect.DeepEqual(orderedAllDeps, correctAllOrdered) {
				errs = append(errs, fmt.Sprintf("orderDeps returned incorrect orderedAllDeps."+
					"\nin static:%q"+
					"\nin shared:%q"+
					"\nmodule:   %v"+
					"\nexpected: %s"+
					"\nactual:   %s",
					testCase.inStatic, testCase.inShared, moduleName, correctAllOrdered, orderedAllDeps))
			}

			correctOutputDeps := expectedTransitiveDeps[moduleName]
			if !reflect.DeepEqual(correctOutputDeps, orderedDeclaredDeps) {
				errs = append(errs, fmt.Sprintf("orderDeps returned incorrect orderedDeclaredDeps."+
					"\nin static:%q"+
					"\nin shared:%q"+
					"\nmodule:   %v"+
					"\nexpected: %s"+
					"\nactual:   %s",
					testCase.inStatic, testCase.inShared, moduleName, correctOutputDeps, orderedDeclaredDeps))
			}
		}

		if len(errs) > 0 {
			sort.Strings(errs)
			for _, err := range errs {
				t.Error(err)
			}
		}
	}
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
	actual := moduleA.depsInLinkOrder
	expected := getOutputPaths(ctx, variant, []string{"c", "b", "d"})

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
	actual := moduleA.depsInLinkOrder
	expected := getOutputPaths(ctx, variant, []string{"c", "b"})

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
	}
	llndk_library {
		name: "libllndk",
	}
	`)
	actual := ctx.ModuleVariantsForTests("libllndk.llndk")
	expected := []string{
		"android_vendor.VER_arm64_armv8-a_shared",
		"android_vendor.VER_arm64_armv8-a_shared_1",
		"android_vendor.VER_arm64_armv8-a_shared_2",
		"android_vendor.VER_arm_armv7-a-neon_shared",
		"android_vendor.VER_arm_armv7-a-neon_shared_1",
		"android_vendor.VER_arm_armv7-a-neon_shared_2",
	}
	checkEquals(t, "variants for llndk stubs", expected, actual)

	params := ctx.ModuleForTests("libllndk.llndk", "android_vendor.VER_arm_armv7-a-neon_shared").Description("generate stub")
	checkEquals(t, "use VNDK version for default stubs", "current", params.Args["apiLevel"])

	params = ctx.ModuleForTests("libllndk.llndk", "android_vendor.VER_arm_armv7-a-neon_shared_1").Description("generate stub")
	checkEquals(t, "override apiLevel for versioned stubs", "1", params.Args["apiLevel"])
}

func TestLlndkHeaders(t *testing.T) {
	ctx := testCc(t, `
	llndk_headers {
		name: "libllndk_headers",
		export_include_dirs: ["my_include"],
	}
	llndk_library {
		name: "libllndk",
		export_llndk_headers: ["libllndk_headers"],
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
		name: "libvendor_available1",
		vendor_available: true,
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor_available2",
		vendor_available: true,
		runtime_libs: ["libvendor_available1"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor_available3",
		vendor_available: true,
		runtime_libs: ["libvendor_available1"],
		target: {
			vendor: {
				exclude_runtime_libs: ["libvendor_available1"],
			}
		},
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libcore",
		runtime_libs: ["libvendor_available1"],
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
		runtime_libs: ["libvendor_available1", "libvendor1"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
`

func TestRuntimeLibs(t *testing.T) {
	ctx := testCc(t, runtimeLibAndroidBp)

	// runtime_libs for core variants use the module names without suffixes.
	variant := "android_arm64_armv8-a_shared"

	module := ctx.ModuleForTests("libvendor_available2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"libvendor_available1"}, module)

	module = ctx.ModuleForTests("libcore", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"libvendor_available1"}, module)

	// runtime_libs for vendor variants have '.vendor' suffixes if the modules have both core
	// and vendor variants.
	variant = "android_vendor.VER_arm64_armv8-a_shared"

	module = ctx.ModuleForTests("libvendor_available2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"libvendor_available1.vendor"}, module)

	module = ctx.ModuleForTests("libvendor2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"libvendor_available1.vendor", "libvendor1"}, module)
}

func TestExcludeRuntimeLibs(t *testing.T) {
	ctx := testCc(t, runtimeLibAndroidBp)

	variant := "android_arm64_armv8-a_shared"
	module := ctx.ModuleForTests("libvendor_available3", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"libvendor_available1"}, module)

	variant = "android_vendor.VER_arm64_armv8-a_shared"
	module = ctx.ModuleForTests("libvendor_available3", variant).Module().(*Module)
	checkRuntimeLibs(t, nil, module)
}

func TestRuntimeLibsNoVndk(t *testing.T) {
	ctx := testCcNoVndk(t, runtimeLibAndroidBp)

	// If DeviceVndkVersion is not defined, then runtime_libs are copied as-is.

	variant := "android_arm64_armv8-a_shared"

	module := ctx.ModuleForTests("libvendor_available2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"libvendor_available1"}, module)

	module = ctx.ModuleForTests("libvendor2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"libvendor_available1", "libvendor1"}, module)
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
			static_libs: ["libfooB"],
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

	mybin := ctx.ModuleForTests("mybin", "android_arm64_armv8-a").Module().(*Module)
	actual := mybin.depsInLinkOrder
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

	ctx := CreateTestContext()
	ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("variable", android.VariableMutator).Parallel()
	})
	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)

	libfoo := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_static").Module().(*Module)
	if !android.InList("-DBAR", libfoo.flags.Local.CppFlags) {
		t.Errorf("expected -DBAR in cppflags, got %q", libfoo.flags.Local.CppFlags)
	}
}
