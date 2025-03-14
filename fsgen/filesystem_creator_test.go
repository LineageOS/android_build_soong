// Copyright 2024 Google Inc. All rights reserved.
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

package fsgen

import (
	"android/soong/android"
	"android/soong/etc"
	"android/soong/filesystem"
	"android/soong/java"
	"testing"

	"github.com/google/blueprint/proptools"
)

var prepareForTestWithFsgenBuildComponents = android.FixtureRegisterWithContext(registerBuildComponents)

func TestFileSystemCreatorSystemImageProps(t *testing.T) {
	result := android.GroupFixturePreparers(
		android.PrepareForIntegrationTestWithAndroid,
		android.PrepareForTestWithAndroidBuildComponents,
		android.PrepareForTestWithAllowMissingDependencies,
		filesystem.PrepareForTestWithFilesystemBuildComponents,
		prepareForTestWithFsgenBuildComponents,
		android.FixtureModifyConfig(func(config android.Config) {
			config.TestProductVariables.PartitionVarsForSoongMigrationOnlyDoNotUse.BoardAvbEnable = true
			config.TestProductVariables.PartitionVarsForSoongMigrationOnlyDoNotUse.PartitionQualifiedVariables =
				map[string]android.PartitionQualifiedVariablesType{
					"system": {
						BoardAvbKeyPath:       "external/avb/test/data/testkey_rsa4096.pem",
						BoardAvbAlgorithm:     "SHA256_RSA4096",
						BoardAvbRollbackIndex: "0",
						BoardFileSystemType:   "ext4",
					},
				}
		}),
		android.FixtureMergeMockFs(android.MockFS{
			"external/avb/test/data/testkey_rsa4096.pem": nil,
			"external/avb/test/Android.bp": []byte(`
			filegroup {
				name: "avb_testkey_rsa4096",
				srcs: ["data/testkey_rsa4096.pem"],
			}
			`),
			"build/soong/fsgen/Android.bp": []byte(`
			soong_filesystem_creator {
				name: "foo",
			}
			`),
		}),
	).RunTest(t)

	fooSystem := result.ModuleForTests("test_product_generated_system_image", "android_common").Module().(interface {
		FsProps() filesystem.FilesystemProperties
	})
	android.AssertBoolEquals(
		t,
		"Property expected to match the product variable 'BOARD_AVB_ENABLE'",
		true,
		proptools.Bool(fooSystem.FsProps().Use_avb),
	)
	android.AssertStringEquals(
		t,
		"Property the avb_private_key property to be set to the existing filegroup",
		":avb_testkey_rsa4096",
		proptools.String(fooSystem.FsProps().Avb_private_key),
	)
	android.AssertStringEquals(
		t,
		"Property expected to match the product variable 'BOARD_AVB_ALGORITHM'",
		"SHA256_RSA4096",
		proptools.String(fooSystem.FsProps().Avb_algorithm),
	)
	android.AssertIntEquals(
		t,
		"Property expected to match the product variable 'BOARD_AVB_SYSTEM_ROLLBACK_INDEX'",
		0,
		proptools.Int(fooSystem.FsProps().Rollback_index),
	)
	android.AssertStringEquals(
		t,
		"Property expected to match the product variable 'BOARD_SYSTEMIMAGE_FILE_SYSTEM_TYPE'",
		"ext4",
		proptools.String(fooSystem.FsProps().Type),
	)
}

func TestFileSystemCreatorSetPartitionDeps(t *testing.T) {
	result := android.GroupFixturePreparers(
		android.PrepareForIntegrationTestWithAndroid,
		android.PrepareForTestWithAndroidBuildComponents,
		android.PrepareForTestWithAllowMissingDependencies,
		filesystem.PrepareForTestWithFilesystemBuildComponents,
		prepareForTestWithFsgenBuildComponents,
		java.PrepareForTestWithJavaBuildComponents,
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureModifyConfig(func(config android.Config) {
			config.TestProductVariables.PartitionVarsForSoongMigrationOnlyDoNotUse.ProductPackages = []string{"bar", "baz"}
			config.TestProductVariables.PartitionVarsForSoongMigrationOnlyDoNotUse.PartitionQualifiedVariables =
				map[string]android.PartitionQualifiedVariablesType{
					"system": {
						BoardFileSystemType: "ext4",
					},
				}
		}),
		android.FixtureMergeMockFs(android.MockFS{
			"external/avb/test/data/testkey_rsa4096.pem": nil,
			"build/soong/fsgen/Android.bp": []byte(`
			soong_filesystem_creator {
				name: "foo",
			}
			`),
		}),
	).RunTestWithBp(t, `
	java_library {
		name: "bar",
		srcs: ["A.java"],
	}
	java_library {
		name: "baz",
		srcs: ["A.java"],
		product_specific: true,
	}
	`)

	android.AssertBoolEquals(
		t,
		"Generated system image expected to depend on system partition installed \"bar\"",
		true,
		java.CheckModuleHasDependency(t, result.TestContext, "test_product_generated_system_image", "android_common", "bar"),
	)
	android.AssertBoolEquals(
		t,
		"Generated system image expected to not depend on product partition installed \"baz\"",
		false,
		java.CheckModuleHasDependency(t, result.TestContext, "test_product_generated_system_image", "android_common", "baz"),
	)
}

func TestFileSystemCreatorDepsWithNamespace(t *testing.T) {
	result := android.GroupFixturePreparers(
		android.PrepareForIntegrationTestWithAndroid,
		android.PrepareForTestWithAndroidBuildComponents,
		android.PrepareForTestWithAllowMissingDependencies,
		android.PrepareForTestWithNamespace,
		android.PrepareForTestWithArchMutator,
		filesystem.PrepareForTestWithFilesystemBuildComponents,
		prepareForTestWithFsgenBuildComponents,
		java.PrepareForTestWithJavaBuildComponents,
		java.PrepareForTestWithJavaDefaultModules,
		android.FixtureModifyConfig(func(config android.Config) {
			config.TestProductVariables.PartitionVarsForSoongMigrationOnlyDoNotUse.ProductPackages = []string{"bar"}
			config.TestProductVariables.NamespacesToExport = []string{"a/b"}
			config.TestProductVariables.PartitionVarsForSoongMigrationOnlyDoNotUse.PartitionQualifiedVariables =
				map[string]android.PartitionQualifiedVariablesType{
					"system": {
						BoardFileSystemType: "ext4",
					},
				}
		}),
		android.PrepareForNativeBridgeEnabled,
		android.FixtureMergeMockFs(android.MockFS{
			"external/avb/test/data/testkey_rsa4096.pem": nil,
			"build/soong/fsgen/Android.bp": []byte(`
			soong_filesystem_creator {
				name: "foo",
			}
			`),
			"a/b/Android.bp": []byte(`
			soong_namespace{
			}
			java_library {
				name: "bar",
				srcs: ["A.java"],
				compile_multilib: "64",
			}
			`),
			"c/d/Android.bp": []byte(`
			soong_namespace{
			}
			java_library {
				name: "bar",
				srcs: ["A.java"],
			}
			`),
		}),
	).RunTest(t)

	var packagingProps android.PackagingProperties
	for _, prop := range result.ModuleForTests("test_product_generated_system_image", "android_common").Module().GetProperties() {
		if packagingPropStruct, ok := prop.(*android.PackagingProperties); ok {
			packagingProps = *packagingPropStruct
		}
	}
	moduleDeps := packagingProps.Multilib.Lib64.Deps

	eval := result.ModuleForTests("test_product_generated_system_image", "android_common").Module().ConfigurableEvaluator(android.PanickingConfigAndErrorContext(result.TestContext))
	android.AssertStringListContains(
		t,
		"Generated system image expected to depend on \"bar\" defined in \"a/b\" namespace",
		moduleDeps.GetOrDefault(eval, nil),
		"//a/b:bar",
	)
	android.AssertStringListDoesNotContain(
		t,
		"Generated system image expected to not depend on \"bar\" defined in \"c/d\" namespace",
		moduleDeps.GetOrDefault(eval, nil),
		"//c/d:bar",
	)
}

func TestRemoveOverriddenModulesFromDeps(t *testing.T) {
	result := android.GroupFixturePreparers(
		android.PrepareForIntegrationTestWithAndroid,
		android.PrepareForTestWithAndroidBuildComponents,
		android.PrepareForTestWithAllowMissingDependencies,
		prepareForTestWithFsgenBuildComponents,
		java.PrepareForTestWithJavaBuildComponents,
		android.FixtureMergeMockFs(android.MockFS{
			"external/avb/test/data/testkey_rsa4096.pem": nil,
			"build/soong/fsgen/Android.bp": []byte(`
			soong_filesystem_creator {
				name: "foo",
			}
			`),
		}),
		android.FixtureModifyConfig(func(config android.Config) {
			config.TestProductVariables.PartitionVarsForSoongMigrationOnlyDoNotUse.ProductPackages = []string{"libfoo", "libbar"}
		}),
	).RunTestWithBp(t, `
java_library {
	name: "libfoo",
}
java_library {
	name: "libbar",
	required: ["libbaz"],
}
java_library {
	name: "libbaz",
	overrides: ["libfoo"], // overrides libfoo
}
	`)
	resolvedSystemDeps := result.TestContext.Config().Get(fsGenStateOnceKey).(*FsGenState).fsDeps["system"]
	_, libFooInDeps := (*resolvedSystemDeps)["libfoo"]
	android.AssertBoolEquals(t, "libfoo should not appear in deps because it has been overridden by libbaz. The latter is a required dep of libbar, which is listed in PRODUCT_PACKAGES", false, libFooInDeps)
}

func TestPrebuiltEtcModuleGen(t *testing.T) {
	result := android.GroupFixturePreparers(
		android.PrepareForIntegrationTestWithAndroid,
		android.PrepareForTestWithAndroidBuildComponents,
		android.PrepareForTestWithAllowMissingDependencies,
		filesystem.PrepareForTestWithFilesystemBuildComponents,
		prepareForTestWithFsgenBuildComponents,
		android.FixtureModifyConfig(func(config android.Config) {
			config.TestProductVariables.PartitionVarsForSoongMigrationOnlyDoNotUse.ProductCopyFiles = []string{
				"frameworks/base/config/preloaded-classes:system/etc/preloaded-classes",
				"frameworks/base/data/keyboards/Vendor_0079_Product_0011.kl:system/usr/keylayout/subdir/Vendor_0079_Product_0011.kl",
				"frameworks/base/data/keyboards/Vendor_0079_Product_18d4.kl:system/usr/keylayout/subdir/Vendor_0079_Product_18d4.kl",
				"some/non/existing/file.txt:system/etc/file.txt",
				"device/sample/etc/apns-full-conf.xml:product/etc/apns-conf.xml:google",
				"device/sample/etc/apns-full-conf.xml:product/etc/apns-conf-2.xml",
				"device/sample/etc/apns-full-conf.xml:system/foo/file.txt",
				"device/sample/etc/apns-full-conf.xml:system/foo/apns-full-conf.xml",
			}
			config.TestProductVariables.PartitionVarsForSoongMigrationOnlyDoNotUse.PartitionQualifiedVariables =
				map[string]android.PartitionQualifiedVariablesType{
					"system": {
						BoardFileSystemType: "ext4",
					},
				}
		}),
		android.FixtureMergeMockFs(android.MockFS{
			"external/avb/test/data/testkey_rsa4096.pem": nil,
			"build/soong/fsgen/Android.bp": []byte(`
			soong_filesystem_creator {
				name: "foo",
			}
			`),
			"frameworks/base/config/preloaded-classes":                   nil,
			"frameworks/base/data/keyboards/Vendor_0079_Product_0011.kl": nil,
			"frameworks/base/data/keyboards/Vendor_0079_Product_18d4.kl": nil,
			"device/sample/etc/apns-full-conf.xml":                       nil,
		}),
	).RunTest(t)

	getModuleProp := func(m android.Module, matcher func(actual interface{}) string) string {
		for _, prop := range m.GetProperties() {

			if str := matcher(prop); str != "" {
				return str
			}
		}
		return ""
	}

	// check generated prebuilt_* module type install path and install partition
	generatedModule := result.ModuleForTests("system-frameworks_base_config-etc-0", "android_common").Module()
	etcModule, _ := generatedModule.(*etc.PrebuiltEtc)
	android.AssertStringEquals(
		t,
		"module expected to have . install path",
		".",
		etcModule.BaseDir(),
	)
	android.AssertBoolEquals(
		t,
		"module expected to be installed in system partition",
		true,
		!generatedModule.InstallInProduct() &&
			!generatedModule.InstallInVendor() &&
			!generatedModule.InstallInSystemExt(),
	)

	// check generated prebuilt_* module specifies correct relative_install_path property
	generatedModule = result.ModuleForTests("system-frameworks_base_data_keyboards-usr_keylayout_subdir-0", "android_common").Module()
	eval := generatedModule.ConfigurableEvaluator(android.PanickingConfigAndErrorContext(result.TestContext))
	android.AssertStringEquals(
		t,
		"module expected to set correct srcs property",
		"Vendor_0079_Product_0011.kl",
		getModuleProp(generatedModule, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltEtcProperties); ok {
				srcs := p.Srcs.GetOrDefault(eval, nil)
				if len(srcs) == 2 {
					return srcs[0];
				}
			}
			return ""
		}),
	)
	android.AssertStringEquals(
		t,
		"module expected to set correct srcs property",
		"Vendor_0079_Product_18d4.kl",
		getModuleProp(generatedModule, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltEtcProperties); ok {
				srcs := p.Srcs.GetOrDefault(eval, nil)
				if len(srcs) == 2 {
					return srcs[1];
				}
			}
			return ""
		}),
	)
	android.AssertStringEquals(
		t,
		"module expected to set correct dsts property",
		"usr/keylayout/subdir/Vendor_0079_Product_0011.kl",
		getModuleProp(generatedModule, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltDstsProperties); ok {
				dsts := p.Dsts.GetOrDefault(eval, nil)
				if len(dsts) == 2 {
					return dsts[0];
				}
			}
			return ""
		}),
	)
	android.AssertStringEquals(
		t,
		"module expected to set correct dsts property",
		"usr/keylayout/subdir/Vendor_0079_Product_18d4.kl",
		getModuleProp(generatedModule, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltDstsProperties); ok {
				dsts := p.Dsts.GetOrDefault(eval, nil)
				if len(dsts) == 2 {
					return dsts[1];
				}
			}
			return ""
		}),
	)


	// check that prebuilt_* module is not generated for non existing source file
	android.AssertPanicMessageContains(
		t,
		"prebuilt_* module not generated for non existing source file",
		"failed to find module \"system-some_non_existing-etc-0\"",
		func() { result.ModuleForTests("system-some_non_existing-etc-0", "android_arm64_armv8-a") },
	)

	// check that duplicate src file can exist in PRODUCT_COPY_FILES and generates separate modules
	generatedModule0 := result.ModuleForTests("product-device_sample_etc-etc-0", "android_common").Module()
	generatedModule1 := result.ModuleForTests("product-device_sample_etc-etc-1", "android_common").Module()

	// check that generated prebuilt_* module sets correct srcs and dsts property
	eval = generatedModule0.ConfigurableEvaluator(android.PanickingConfigAndErrorContext(result.TestContext))
	android.AssertStringEquals(
		t,
		"module expected to set correct srcs property",
		"apns-full-conf.xml",
		getModuleProp(generatedModule0, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltEtcProperties); ok {
				srcs := p.Srcs.GetOrDefault(eval, nil)
				if len(srcs) == 1 {
					return srcs[0];
				}
			}
			return ""
		}),
	)
	android.AssertStringEquals(
		t,
		"module expected to set correct dsts property",
		"etc/apns-conf.xml",
		getModuleProp(generatedModule0, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltDstsProperties); ok {
				dsts := p.Dsts.GetOrDefault(eval, nil)
				if len(dsts) == 1 {
					return dsts[0];
				}
			}
			return ""
		}),
	)

	// check that generated prebuilt_* module sets correct srcs and dsts property
	eval = generatedModule1.ConfigurableEvaluator(android.PanickingConfigAndErrorContext(result.TestContext))
	android.AssertStringEquals(
		t,
		"module expected to set correct srcs property",
		"apns-full-conf.xml",
		getModuleProp(generatedModule1, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltEtcProperties); ok {
				srcs := p.Srcs.GetOrDefault(eval, nil)
				if len(srcs) == 1 {
					return srcs[0];
				}
			}
			return ""
		}),
	)
	android.AssertStringEquals(
		t,
		"module expected to set correct dsts property",
		"etc/apns-conf-2.xml",
		getModuleProp(generatedModule1, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltDstsProperties); ok {
				dsts := p.Dsts.GetOrDefault(eval, nil)
				if len(dsts) == 1 {
					return dsts[0];
				}
			}
			return ""
		}),
	)

	generatedModule2 := result.ModuleForTests("system-device_sample_etc-foo-0", "android_common").Module()
	generatedModule3 := result.ModuleForTests("system-device_sample_etc-foo-1", "android_common").Module()

	// check that generated prebuilt_* module sets correct srcs and dsts property
	eval = generatedModule2.ConfigurableEvaluator(android.PanickingConfigAndErrorContext(result.TestContext))
	android.AssertStringEquals(
		t,
		"module expected to set correct srcs property",
		"apns-full-conf.xml",
		getModuleProp(generatedModule2, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltEtcProperties); ok {
				srcs := p.Srcs.GetOrDefault(eval, nil)
				if len(srcs) == 1 {
					return srcs[0];
				}
			}
			return ""
		}),
	)
	android.AssertStringEquals(
		t,
		"module expected to set correct dsts property",
		"foo/file.txt",
		getModuleProp(generatedModule2, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltDstsProperties); ok {
				dsts := p.Dsts.GetOrDefault(eval, nil)
				if len(dsts) == 1 {
					return dsts[0];
				}
			}
			return ""
		}),
	)

	// check that generated prebuilt_* module sets correct srcs and dsts property
	eval = generatedModule3.ConfigurableEvaluator(android.PanickingConfigAndErrorContext(result.TestContext))
	android.AssertStringEquals(
		t,
		"module expected to set correct srcs property",
		"apns-full-conf.xml",
		getModuleProp(generatedModule2, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltEtcProperties); ok {
				srcs := p.Srcs.GetOrDefault(eval, nil)
				if len(srcs) == 1 {
					return srcs[0];
				}
			}
			return ""
		}),
	)
	android.AssertStringEquals(
		t,
		"module expected to set correct dsts property",
		"foo/apns-full-conf.xml",
		getModuleProp(generatedModule3, func(actual interface{}) string {
			if p, ok := actual.(*etc.PrebuiltDstsProperties); ok {
				dsts := p.Dsts.GetOrDefault(eval, nil)
				if len(dsts) == 1 {
					return dsts[0];
				}
			}
			return ""
		}),
	)
}
