// Copyright 2015 Google Inc. All rights reserved.
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
	"github.com/google/blueprint/proptools"

	"fmt"
	"io"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/multitree"
)

var (
	NativeBridgeSuffix  = ".native_bridge"
	ProductSuffix       = ".product"
	VendorSuffix        = ".vendor"
	RamdiskSuffix       = ".ramdisk"
	VendorRamdiskSuffix = ".vendor_ramdisk"
	RecoverySuffix      = ".recovery"
	sdkSuffix           = ".sdk"
)

type AndroidMkContext interface {
	BaseModuleName() string
	Target() android.Target
	subAndroidMk(*android.AndroidMkEntries, interface{})
	Arch() android.Arch
	Os() android.OsType
	Host() bool
	UseVndk() bool
	VndkVersion() string
	static() bool
	InRamdisk() bool
	InVendorRamdisk() bool
	InRecovery() bool
	NotInPlatform() bool
	InVendorOrProduct() bool
}

type subAndroidMkProvider interface {
	AndroidMkEntries(AndroidMkContext, *android.AndroidMkEntries)
}

func (c *Module) subAndroidMk(entries *android.AndroidMkEntries, obj interface{}) {
	if c.subAndroidMkOnce == nil {
		c.subAndroidMkOnce = make(map[subAndroidMkProvider]bool)
	}
	if androidmk, ok := obj.(subAndroidMkProvider); ok {
		if !c.subAndroidMkOnce[androidmk] {
			c.subAndroidMkOnce[androidmk] = true
			androidmk.AndroidMkEntries(c, entries)
		}
	}
}

func (c *Module) AndroidMkEntries() []android.AndroidMkEntries {
	if c.hideApexVariantFromMake || c.Properties.HideFromMake {
		return []android.AndroidMkEntries{{
			Disabled: true,
		}}
	}

	entries := android.AndroidMkEntries{
		OutputFile: c.outputFile,
		// TODO(jiyong): add the APEXes providing shared libs to the required
		// modules Currently, adding c.Properties.ApexesProvidingSharedLibs is
		// causing multiple ART APEXes (com.android.art and com.android.art.debug)
		// to be installed. And this is breaking some older devices (like marlin)
		// where system.img is small.
		Required:     c.Properties.AndroidMkRuntimeLibs,
		OverrideName: c.BaseModuleName(),
		Include:      "$(BUILD_SYSTEM)/soong_cc_rust_prebuilt.mk",

		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				if len(c.Properties.Logtags) > 0 {
					entries.AddStrings("LOCAL_LOGTAGS_FILES", c.Properties.Logtags...)
				}
				// Note: Pass the exact value of AndroidMkSystemSharedLibs to the Make
				// world, even if it is an empty list. In the Make world,
				// LOCAL_SYSTEM_SHARED_LIBRARIES defaults to "none", which is expanded
				// to the default list of system shared libs by the build system.
				// Soong computes the exact list of system shared libs, so we have to
				// override the default value when the list of libs is actually empty.
				entries.SetString("LOCAL_SYSTEM_SHARED_LIBRARIES", strings.Join(c.Properties.AndroidMkSystemSharedLibs, " "))
				if len(c.Properties.AndroidMkSharedLibs) > 0 {
					entries.AddStrings("LOCAL_SHARED_LIBRARIES", c.Properties.AndroidMkSharedLibs...)
				}
				if len(c.Properties.AndroidMkRuntimeLibs) > 0 {
					entries.AddStrings("LOCAL_RUNTIME_LIBRARIES", c.Properties.AndroidMkRuntimeLibs...)
				}
				entries.SetString("LOCAL_SOONG_LINK_TYPE", c.makeLinkType)
				if c.InVendorOrProduct() {
					if c.IsVndk() && !c.static() {
						entries.SetString("LOCAL_SOONG_VNDK_VERSION", c.VndkVersion())
						// VNDK libraries available to vendor are not installed because
						// they are packaged in VNDK APEX and installed by APEX packages (apex/apex.go)
						if !c.IsVndkExt() {
							entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
						}
					}
				}
				if c.InVendor() {
					entries.SetBool("LOCAL_IN_VENDOR", true)
				} else if c.InProduct() {
					entries.SetBool("LOCAL_IN_PRODUCT", true)
				}
				if c.Properties.IsSdkVariant && c.Properties.SdkAndPlatformVariantVisibleToMake {
					// Make the SDK variant uninstallable so that there are not two rules to install
					// to the same location.
					entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
					// Add the unsuffixed name to SOONG_SDK_VARIANT_MODULES so that Make can rewrite
					// dependencies to the .sdk suffix when building a module that uses the SDK.
					entries.SetString("SOONG_SDK_VARIANT_MODULES",
						"$(SOONG_SDK_VARIANT_MODULES) $(patsubst %.sdk,%,$(LOCAL_MODULE))")
				}
				android.SetAconfigFileMkEntries(c.AndroidModuleBase(), entries, c.mergedAconfigFiles)
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string) {
				if c.Properties.IsSdkVariant && c.Properties.SdkAndPlatformVariantVisibleToMake &&
					c.CcLibraryInterface() && c.Shared() {
					// Using the SDK variant as a JNI library needs a copy of the .so that
					// is not named .sdk.so so that it can be packaged into the APK with
					// the right name.
					fmt.Fprintln(w, "$(eval $(call copy-one-file,",
						"$(LOCAL_BUILT_MODULE),",
						"$(patsubst %.sdk.so,%.so,$(LOCAL_BUILT_MODULE))))")
				}
			},
		},
	}

	for _, feature := range c.features {
		c.subAndroidMk(&entries, feature)
	}

	c.subAndroidMk(&entries, c.compiler)
	c.subAndroidMk(&entries, c.linker)
	if c.sanitize != nil {
		c.subAndroidMk(&entries, c.sanitize)
	}
	c.subAndroidMk(&entries, c.installer)

	entries.SubName += c.Properties.SubName

	return []android.AndroidMkEntries{entries}
}

func androidMkWriteExtraTestConfigs(extraTestConfigs android.Paths, entries *android.AndroidMkEntries) {
	if len(extraTestConfigs) > 0 {
		entries.ExtraEntries = append(entries.ExtraEntries,
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.AddStrings("LOCAL_EXTRA_FULL_TEST_CONFIGS", extraTestConfigs.Strings()...)
			})
	}
}

func makeOverrideModuleNames(ctx AndroidMkContext, overrides []string) []string {
	if ctx.Target().NativeBridge == android.NativeBridgeEnabled {
		var result []string
		for _, override := range overrides {
			result = append(result, override+NativeBridgeSuffix)
		}
		return result
	}

	return overrides
}

func (library *libraryDecorator) androidMkWriteExportedFlags(entries *android.AndroidMkEntries) {
	var exportedFlags []string
	var includeDirs android.Paths
	var systemIncludeDirs android.Paths
	var exportedDeps android.Paths

	if library.flagExporterInfo != nil {
		exportedFlags = library.flagExporterInfo.Flags
		includeDirs = library.flagExporterInfo.IncludeDirs
		systemIncludeDirs = library.flagExporterInfo.SystemIncludeDirs
		exportedDeps = library.flagExporterInfo.Deps
	} else {
		exportedFlags = library.flagExporter.flags
		includeDirs = library.flagExporter.dirs
		systemIncludeDirs = library.flagExporter.systemDirs
		exportedDeps = library.flagExporter.deps
	}
	for _, dir := range includeDirs {
		exportedFlags = append(exportedFlags, "-I"+dir.String())
	}
	for _, dir := range systemIncludeDirs {
		exportedFlags = append(exportedFlags, "-isystem "+dir.String())
	}
	if len(exportedFlags) > 0 {
		entries.AddStrings("LOCAL_EXPORT_CFLAGS", exportedFlags...)
	}
	if len(exportedDeps) > 0 {
		entries.AddStrings("LOCAL_EXPORT_C_INCLUDE_DEPS", exportedDeps.Strings()...)
	}
}

func (library *libraryDecorator) androidMkEntriesWriteAdditionalDependenciesForSourceAbiDiff(entries *android.AndroidMkEntries) {
	if !library.static() {
		entries.AddPaths("LOCAL_ADDITIONAL_DEPENDENCIES", library.sAbiDiff)
	}
}

// TODO(ccross): remove this once apex/androidmk.go is converted to AndroidMkEntries
func (library *libraryDecorator) androidMkWriteAdditionalDependenciesForSourceAbiDiff(w io.Writer) {
	if !library.static() {
		fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES +=", strings.Join(library.sAbiDiff.Strings(), " "))
	}
}

func (library *libraryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	if library.static() {
		entries.Class = "STATIC_LIBRARIES"
	} else if library.shared() {
		entries.Class = "SHARED_LIBRARIES"
		entries.ExtraEntries = append(entries.ExtraEntries, func(_ android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.SetString("LOCAL_SOONG_TOC", library.toc().String())
			if !library.buildStubs() && library.unstrippedOutputFile != nil {
				entries.SetString("LOCAL_SOONG_UNSTRIPPED_BINARY", library.unstrippedOutputFile.String())
			}
			if len(library.Properties.Overrides) > 0 {
				entries.SetString("LOCAL_OVERRIDES_MODULES", strings.Join(makeOverrideModuleNames(ctx, library.Properties.Overrides), " "))
			}
			if len(library.postInstallCmds) > 0 {
				entries.SetString("LOCAL_POST_INSTALL_CMD", strings.Join(library.postInstallCmds, "&& "))
			}
		})
	} else if library.header() {
		entries.Class = "HEADER_LIBRARIES"
	}

	if library.distFile != nil {
		entries.DistFiles = android.MakeDefaultDistFiles(library.distFile)
	}

	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		library.androidMkWriteExportedFlags(entries)
		library.androidMkEntriesWriteAdditionalDependenciesForSourceAbiDiff(entries)

		if entries.OutputFile.Valid() {
			_, _, ext := android.SplitFileExt(entries.OutputFile.Path().Base())
			entries.SetString("LOCAL_BUILT_MODULE_STEM", "$(LOCAL_MODULE)"+ext)
		}

		if library.coverageOutputFile.Valid() {
			entries.SetString("LOCAL_PREBUILT_COVERAGE_ARCHIVE", library.coverageOutputFile.String())
		}

		if library.useCoreVariant {
			entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
			entries.SetBool("LOCAL_NO_NOTICE_FILE", true)
			entries.SetBool("LOCAL_VNDK_DEPEND_ON_CORE_VARIANT", true)
		}
		if library.checkSameCoreVariant {
			entries.SetBool("LOCAL_CHECK_SAME_VNDK_VARIANTS", true)
		}
	})

	if library.shared() && !library.buildStubs() {
		ctx.subAndroidMk(entries, library.baseInstaller)
	} else {
		if library.buildStubs() && library.stubsVersion() != "" {
			entries.SubName = "." + library.stubsVersion()
		}
		entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			// library.makeUninstallable() depends on this to bypass HideFromMake() for
			// static libraries.
			entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
			if library.buildStubs() {
				entries.SetBool("LOCAL_NO_NOTICE_FILE", true)
			}
		})
	}
	// If a library providing a stub is included in an APEX, the private APIs of the library
	// is accessible only inside the APEX. From outside of the APEX, clients can only use the
	// public APIs via the stub. To enforce this, the (latest version of the) stub gets the
	// name of the library. The impl library instead gets the `.bootstrap` suffix to so that
	// they can be exceptionally used directly when APEXes are not available (e.g. during the
	// very early stage in the boot process).
	if len(library.Properties.Stubs.Versions) > 0 && !ctx.Host() && ctx.NotInPlatform() &&
		!ctx.InRamdisk() && !ctx.InVendorRamdisk() && !ctx.InRecovery() && !ctx.InVendorOrProduct() && !ctx.static() {
		if library.buildStubs() && library.isLatestStubVersion() {
			entries.SubName = ""
		}
		if !library.buildStubs() {
			entries.SubName = ".bootstrap"
		}
	}
}

func (object *objectLinker) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "STATIC_LIBRARIES"
	entries.ExtraFooters = append(entries.ExtraFooters,
		func(w io.Writer, name, prefix, moduleDir string) {
			out := entries.OutputFile.Path()
			varname := fmt.Sprintf("SOONG_%sOBJECT_%s%s", prefix, name, entries.SubName)

			fmt.Fprintf(w, "\n%s := %s\n", varname, out.String())
			fmt.Fprintln(w, ".KATI_READONLY: "+varname)
		})
}

func (test *testDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		if len(test.InstallerProperties.Test_suites) > 0 {
			entries.AddCompatibilityTestSuites(test.InstallerProperties.Test_suites...)
		}
	})
}

func (binary *binaryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, binary.baseInstaller)

	entries.Class = "EXECUTABLES"
	entries.DistFiles = binary.distFiles
	entries.ExtraEntries = append(entries.ExtraEntries, func(_ android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		entries.SetString("LOCAL_SOONG_UNSTRIPPED_BINARY", binary.unstrippedOutputFile.String())
		if len(binary.symlinks) > 0 {
			entries.AddStrings("LOCAL_MODULE_SYMLINKS", binary.symlinks...)
		}

		if binary.coverageOutputFile.Valid() {
			entries.SetString("LOCAL_PREBUILT_COVERAGE_ARCHIVE", binary.coverageOutputFile.String())
		}

		if len(binary.Properties.Overrides) > 0 {
			entries.SetString("LOCAL_OVERRIDES_MODULES", strings.Join(makeOverrideModuleNames(ctx, binary.Properties.Overrides), " "))
		}
		if len(binary.postInstallCmds) > 0 {
			entries.SetString("LOCAL_POST_INSTALL_CMD", strings.Join(binary.postInstallCmds, "&& "))
		}
	})
}

func (benchmark *benchmarkDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, benchmark.binaryDecorator)
	entries.Class = "NATIVE_TESTS"
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		if len(benchmark.Properties.Test_suites) > 0 {
			entries.AddCompatibilityTestSuites(benchmark.Properties.Test_suites...)
		}
		if benchmark.testConfig != nil {
			entries.SetString("LOCAL_FULL_TEST_CONFIG", benchmark.testConfig.String())
		}
		entries.SetBool("LOCAL_NATIVE_BENCHMARK", true)
		if !BoolDefault(benchmark.Properties.Auto_gen_config, true) {
			entries.SetBool("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", true)
		}
	})
}

func (test *testBinary) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, test.binaryDecorator)
	ctx.subAndroidMk(entries, test.testDecorator)

	entries.Class = "NATIVE_TESTS"
	if Bool(test.Properties.Test_per_src) {
		entries.SubName = "_" + String(test.binaryDecorator.Properties.Stem)
	}
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		if test.testConfig != nil {
			entries.SetString("LOCAL_FULL_TEST_CONFIG", test.testConfig.String())
		}
		if !BoolDefault(test.Properties.Auto_gen_config, true) {
			entries.SetBool("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", true)
		}
		entries.AddStrings("LOCAL_TEST_MAINLINE_MODULES", test.Properties.Test_mainline_modules...)

		entries.SetBoolIfTrue("LOCAL_COMPATIBILITY_PER_TESTCASE_DIRECTORY", Bool(test.Properties.Per_testcase_directory))
		if len(test.Properties.Data_bins) > 0 {
			entries.AddStrings("LOCAL_TEST_DATA_BINS", test.Properties.Data_bins...)
		}

		test.Properties.Test_options.CommonTestOptions.SetAndroidMkEntries(entries)
	})

	androidMkWriteExtraTestConfigs(test.extraTestConfigs, entries)
}

func (fuzz *fuzzBinary) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, fuzz.binaryDecorator)

	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		entries.SetBool("LOCAL_IS_FUZZ_TARGET", true)
		if fuzz.installedSharedDeps != nil {
			// TOOD: move to install dep
			entries.AddStrings("LOCAL_FUZZ_INSTALLED_SHARED_DEPS", fuzz.installedSharedDeps...)
		}
	})
}

func (test *testLibrary) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, test.libraryDecorator)
	ctx.subAndroidMk(entries, test.testDecorator)
}

func (installer *baseInstaller) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	if installer.path == (android.InstallPath{}) {
		return
	}

	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		path, file := filepath.Split(installer.path.String())
		stem, suffix, _ := android.SplitFileExt(file)
		entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
		entries.SetString("LOCAL_MODULE_PATH", path)
		entries.SetString("LOCAL_MODULE_STEM", stem)
	})
}

func (c *stubDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.SubName = ndkLibrarySuffix + "." + c.apiLevel.String()
	entries.Class = "SHARED_LIBRARIES"

	if !c.buildStubs() {
		entries.Disabled = true
		return
	}

	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		path, file := filepath.Split(c.installPath.String())
		stem, suffix, _ := android.SplitFileExt(file)
		entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
		entries.SetString("LOCAL_MODULE_PATH", path)
		entries.SetString("LOCAL_MODULE_STEM", stem)
		entries.SetBool("LOCAL_NO_NOTICE_FILE", true)
		if c.parsedCoverageXmlPath.String() != "" {
			entries.SetString("SOONG_NDK_API_XML", "$(SOONG_NDK_API_XML) "+c.parsedCoverageXmlPath.String())
		}
	})
}

func (c *vndkPrebuiltLibraryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "SHARED_LIBRARIES"

	entries.SubName = c.androidMkSuffix

	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		c.libraryDecorator.androidMkWriteExportedFlags(entries)

		// Specifying stem is to pass check_elf_files when vendor modules link against vndk prebuilt.
		// We can't use install path because VNDKs are not installed. Instead, Srcs is directly used.
		_, file := filepath.Split(c.properties.Srcs[0])
		stem, suffix, ext := android.SplitFileExt(file)
		entries.SetString("LOCAL_BUILT_MODULE_STEM", "$(LOCAL_MODULE)"+ext)
		entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
		entries.SetString("LOCAL_MODULE_STEM", stem)

		if c.tocFile.Valid() {
			entries.SetString("LOCAL_SOONG_TOC", c.tocFile.String())
		}

		// VNDK libraries available to vendor are not installed because
		// they are packaged in VNDK APEX and installed by APEX packages (apex/apex.go)
		entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
	})
}

func (c *snapshotLibraryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	// Each vendor snapshot is exported to androidMk only when BOARD_VNDK_VERSION != current
	// and the version of the prebuilt is same as BOARD_VNDK_VERSION.
	if c.shared() {
		entries.Class = "SHARED_LIBRARIES"
	} else if c.static() {
		entries.Class = "STATIC_LIBRARIES"
	} else if c.header() {
		entries.Class = "HEADER_LIBRARIES"
	}

	entries.SubName = ""

	if c.IsSanitizerEnabled(cfi) {
		entries.SubName += ".cfi"
	} else if c.IsSanitizerEnabled(Hwasan) {
		entries.SubName += ".hwasan"
	}

	entries.SubName += c.baseProperties.Androidmk_suffix

	entries.ExtraEntries = append(entries.ExtraEntries, func(_ android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		c.libraryDecorator.androidMkWriteExportedFlags(entries)

		if c.shared() || c.static() {
			src := c.path.String()
			// For static libraries which aren't installed, directly use Src to extract filename.
			// This is safe: generated snapshot modules have a real path as Src, not a module
			if c.static() {
				src = proptools.String(c.properties.Src)
			}
			path, file := filepath.Split(src)
			stem, suffix, ext := android.SplitFileExt(file)
			entries.SetString("LOCAL_BUILT_MODULE_STEM", "$(LOCAL_MODULE)"+ext)
			entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
			entries.SetString("LOCAL_MODULE_STEM", stem)
			if c.shared() {
				entries.SetString("LOCAL_MODULE_PATH", path)
			}
			if c.tocFile.Valid() {
				entries.SetString("LOCAL_SOONG_TOC", c.tocFile.String())
			}

			if c.shared() && len(c.Properties.Overrides) > 0 {
				entries.SetString("LOCAL_OVERRIDES_MODULES", strings.Join(makeOverrideModuleNames(ctx, c.Properties.Overrides), " "))
			}
		}

		if !c.shared() { // static or header
			entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
		}
	})
}

func (c *snapshotBinaryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "EXECUTABLES"
	entries.SubName = c.baseProperties.Androidmk_suffix
}

func (c *snapshotObjectLinker) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "STATIC_LIBRARIES"
	entries.SubName = c.baseProperties.Androidmk_suffix

	entries.ExtraFooters = append(entries.ExtraFooters,
		func(w io.Writer, name, prefix, moduleDir string) {
			out := entries.OutputFile.Path()
			varname := fmt.Sprintf("SOONG_%sOBJECT_%s%s", prefix, name, entries.SubName)

			fmt.Fprintf(w, "\n%s := %s\n", varname, out.String())
			fmt.Fprintln(w, ".KATI_READONLY: "+varname)
		})
}

func (c *ndkPrebuiltStlLinker) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "SHARED_LIBRARIES"
}

func (p *prebuiltLinker) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		if p.properties.Check_elf_files != nil {
			entries.SetBool("LOCAL_CHECK_ELF_FILES", *p.properties.Check_elf_files)
		} else {
			// soong_cc_rust_prebuilt.mk does not include check_elf_file.mk by default
			// because cc_library_shared and cc_binary use soong_cc_rust_prebuilt.mk as well.
			// In order to turn on prebuilt ABI checker, set `LOCAL_CHECK_ELF_FILES` to
			// true if `p.properties.Check_elf_files` is not specified.
			entries.SetBool("LOCAL_CHECK_ELF_FILES", true)
		}
	})
}

func (p *prebuiltLibraryLinker) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, p.libraryDecorator)
	if p.shared() {
		ctx.subAndroidMk(entries, &p.prebuiltLinker)
		androidMkWriteAllowUndefinedSymbols(p.baseLinker, entries)
	}
}

func (p *prebuiltBinaryLinker) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, p.binaryDecorator)
	ctx.subAndroidMk(entries, &p.prebuiltLinker)
	androidMkWriteAllowUndefinedSymbols(p.baseLinker, entries)
}

func (a *apiLibraryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "SHARED_LIBRARIES"
	entries.SubName += multitree.GetApiImportSuffix()

	entries.ExtraEntries = append(entries.ExtraEntries, func(_ android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		a.libraryDecorator.androidMkWriteExportedFlags(entries)
		src := *a.properties.Src
		path, file := filepath.Split(src)
		stem, suffix, ext := android.SplitFileExt(file)
		entries.SetString("LOCAL_BUILT_MODULE_STEM", "$(LOCAL_MODULE)"+ext)
		entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
		entries.SetString("LOCAL_MODULE_STEM", stem)
		entries.SetString("LOCAL_MODULE_PATH", path)
		entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
		entries.SetString("LOCAL_SOONG_TOC", a.toc().String())
	})
}

func (a *apiHeadersDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "HEADER_LIBRARIES"
	entries.SubName += multitree.GetApiImportSuffix()

	entries.ExtraEntries = append(entries.ExtraEntries, func(_ android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
		a.libraryDecorator.androidMkWriteExportedFlags(entries)
		entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
	})
}

func androidMkWriteAllowUndefinedSymbols(linker *baseLinker, entries *android.AndroidMkEntries) {
	allow := linker.Properties.Allow_undefined_symbols
	if allow != nil {
		entries.ExtraEntries = append(entries.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.SetBool("LOCAL_ALLOW_UNDEFINED_SYMBOLS", *allow)
		})
	}
}
