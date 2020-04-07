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
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"android/soong/android"
)

var (
	nativeBridgeSuffix = ".native_bridge"
	productSuffix      = ".product"
	vendorSuffix       = ".vendor"
	ramdiskSuffix      = ".ramdisk"
	recoverySuffix     = ".recovery"
)

type AndroidMkContext interface {
	Name() string
	Target() android.Target
	subAndroidMk(*android.AndroidMkEntries, interface{})
	Arch() android.Arch
	Os() android.OsType
	Host() bool
	UseVndk() bool
	VndkVersion() string
	static() bool
	InRamdisk() bool
	InRecovery() bool
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
	if c.Properties.HideFromMake || !c.IsForPlatform() {
		return []android.AndroidMkEntries{{
			Disabled: true,
		}}
	}

	entries := android.AndroidMkEntries{
		OutputFile: c.outputFile,
		// TODO(jiyong): add the APEXes providing shared libs to the required modules
		// Currently, adding c.Properties.ApexesProvidingSharedLibs is causing multiple
		// ART APEXes (com.android.art.debug|release) to be installed. And this
		// is breaking some older devices (like marlin) where system.img is small.
		Required: c.Properties.AndroidMkRuntimeLibs,
		Include:  "$(BUILD_SYSTEM)/soong_cc_prebuilt.mk",

		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				if len(c.Properties.Logtags) > 0 {
					entries.AddStrings("LOCAL_LOGTAGS_FILES", c.Properties.Logtags...)
				}
				if len(c.Properties.AndroidMkSharedLibs) > 0 {
					entries.AddStrings("LOCAL_SHARED_LIBRARIES", c.Properties.AndroidMkSharedLibs...)
				}
				if len(c.Properties.AndroidMkStaticLibs) > 0 {
					entries.AddStrings("LOCAL_STATIC_LIBRARIES", c.Properties.AndroidMkStaticLibs...)
				}
				if len(c.Properties.AndroidMkWholeStaticLibs) > 0 {
					entries.AddStrings("LOCAL_WHOLE_STATIC_LIBRARIES", c.Properties.AndroidMkWholeStaticLibs...)
				}
				entries.SetString("LOCAL_SOONG_LINK_TYPE", c.makeLinkType)
				if c.UseVndk() {
					entries.SetBool("LOCAL_USE_VNDK", true)
					if c.IsVndk() && !c.static() {
						entries.SetString("LOCAL_SOONG_VNDK_VERSION", c.VndkVersion())
						// VNDK libraries available to vendor are not installed because
						// they are packaged in VNDK APEX and installed by APEX packages (apex/apex.go)
						if !c.isVndkExt() {
							entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
						}
					}
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

func androidMkWriteTestData(data android.Paths, ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	var testFiles []string
	for _, d := range data {
		rel := d.Rel()
		path := d.String()
		if !strings.HasSuffix(path, rel) {
			panic(fmt.Errorf("path %q does not end with %q", path, rel))
		}
		path = strings.TrimSuffix(path, rel)
		testFiles = append(testFiles, path+":"+rel)
	}
	if len(testFiles) > 0 {
		entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
			entries.AddStrings("LOCAL_TEST_DATA", testFiles...)
		})
	}
}

func makeOverrideModuleNames(ctx AndroidMkContext, overrides []string) []string {
	if ctx.Target().NativeBridge == android.NativeBridgeEnabled {
		var result []string
		for _, override := range overrides {
			result = append(result, override+nativeBridgeSuffix)
		}
		return result
	}

	return overrides
}

func (library *libraryDecorator) androidMkWriteExportedFlags(entries *android.AndroidMkEntries) {
	exportedFlags := library.exportedFlags()
	for _, dir := range library.exportedDirs() {
		exportedFlags = append(exportedFlags, "-I"+dir.String())
	}
	for _, dir := range library.exportedSystemDirs() {
		exportedFlags = append(exportedFlags, "-isystem "+dir.String())
	}
	if len(exportedFlags) > 0 {
		entries.AddStrings("LOCAL_EXPORT_CFLAGS", exportedFlags...)
	}
	exportedDeps := library.exportedDeps()
	if len(exportedDeps) > 0 {
		entries.AddStrings("LOCAL_EXPORT_C_INCLUDE_DEPS", exportedDeps.Strings()...)
	}
}

func (library *libraryDecorator) androidMkEntriesWriteAdditionalDependenciesForSourceAbiDiff(entries *android.AndroidMkEntries) {
	if library.sAbiOutputFile.Valid() {
		entries.SetString("LOCAL_ADDITIONAL_DEPENDENCIES",
			"$(LOCAL_ADDITIONAL_DEPENDENCIES) "+library.sAbiOutputFile.String())
		if library.sAbiDiff.Valid() && !library.static() {
			entries.SetString("LOCAL_ADDITIONAL_DEPENDENCIES",
				"$(LOCAL_ADDITIONAL_DEPENDENCIES) "+library.sAbiDiff.String())
			entries.SetString("HEADER_ABI_DIFFS",
				"$(HEADER_ABI_DIFFS) "+library.sAbiDiff.String())
		}
	}
}

// TODO(ccross): remove this once apex/androidmk.go is converted to AndroidMkEntries
func (library *libraryDecorator) androidMkWriteAdditionalDependenciesForSourceAbiDiff(w io.Writer) {
	if library.sAbiOutputFile.Valid() {
		fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES := $(LOCAL_ADDITIONAL_DEPENDENCIES) ",
			library.sAbiOutputFile.String())
		if library.sAbiDiff.Valid() && !library.static() {
			fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES := $(LOCAL_ADDITIONAL_DEPENDENCIES) ",
				library.sAbiDiff.String())
			fmt.Fprintln(w, "HEADER_ABI_DIFFS := $(HEADER_ABI_DIFFS) ",
				library.sAbiDiff.String())
		}
	}
}

func (library *libraryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	if library.static() {
		entries.Class = "STATIC_LIBRARIES"
	} else if library.shared() {
		entries.Class = "SHARED_LIBRARIES"
		entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
			entries.SetString("LOCAL_SOONG_TOC", library.toc().String())
			if !library.buildStubs() {
				entries.SetString("LOCAL_SOONG_UNSTRIPPED_BINARY", library.unstrippedOutputFile.String())
			}
			if len(library.Properties.Overrides) > 0 {
				entries.SetString("LOCAL_OVERRIDES_MODULES", strings.Join(makeOverrideModuleNames(ctx, library.Properties.Overrides), " "))
			}
			if len(library.post_install_cmds) > 0 {
				entries.SetString("LOCAL_POST_INSTALL_CMD", strings.Join(library.post_install_cmds, "&& "))
			}
		})
	} else if library.header() {
		entries.Class = "HEADER_LIBRARIES"
	}

	entries.DistFile = library.distFile
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		library.androidMkWriteExportedFlags(entries)
		library.androidMkEntriesWriteAdditionalDependenciesForSourceAbiDiff(entries)

		_, _, ext := android.SplitFileExt(entries.OutputFile.Path().Base())

		entries.SetString("LOCAL_BUILT_MODULE_STEM", "$(LOCAL_MODULE)"+ext)

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
		if library.buildStubs() {
			entries.SubName = "." + library.stubsVersion()
		}
		entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
			entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
			if library.buildStubs() {
				entries.SetBool("LOCAL_NO_NOTICE_FILE", true)
			}
		})
	}
	if len(library.Properties.Stubs.Versions) > 0 &&
		android.DirectlyInAnyApex(ctx, ctx.Name()) && !ctx.InRamdisk() && !ctx.InRecovery() && !ctx.UseVndk() &&
		!ctx.static() {
		if library.buildStubs() && library.isLatestStubVersion() {
			// reference the latest version via its name without suffix when it is provided by apex
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
		func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
			out := entries.OutputFile.Path()
			varname := fmt.Sprintf("SOONG_%sOBJECT_%s%s", prefix, name, entries.SubName)

			fmt.Fprintf(w, "\n%s := %s\n", varname, out.String())
			fmt.Fprintln(w, ".KATI_READONLY: "+varname)
		})
}

func (binary *binaryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, binary.baseInstaller)

	entries.Class = "EXECUTABLES"
	entries.DistFile = binary.distFile
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
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
		if len(binary.post_install_cmds) > 0 {
			entries.SetString("LOCAL_POST_INSTALL_CMD", strings.Join(binary.post_install_cmds, "&& "))
		}
	})
}

func (benchmark *benchmarkDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, benchmark.binaryDecorator)
	entries.Class = "NATIVE_TESTS"
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		if len(benchmark.Properties.Test_suites) > 0 {
			entries.SetString("LOCAL_COMPATIBILITY_SUITE",
				strings.Join(benchmark.Properties.Test_suites, " "))
		}
		if benchmark.testConfig != nil {
			entries.SetString("LOCAL_FULL_TEST_CONFIG", benchmark.testConfig.String())
		}
		entries.SetBool("LOCAL_NATIVE_BENCHMARK", true)
		if !BoolDefault(benchmark.Properties.Auto_gen_config, true) {
			entries.SetBool("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", true)
		}
	})

	androidMkWriteTestData(benchmark.data, ctx, entries)
}

func (test *testBinary) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, test.binaryDecorator)
	entries.Class = "NATIVE_TESTS"
	if Bool(test.Properties.Test_per_src) {
		entries.SubName = "_" + String(test.binaryDecorator.Properties.Stem)
	}
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		if len(test.Properties.Test_suites) > 0 {
			entries.SetString("LOCAL_COMPATIBILITY_SUITE",
				strings.Join(test.Properties.Test_suites, " "))
		}
		if test.testConfig != nil {
			entries.SetString("LOCAL_FULL_TEST_CONFIG", test.testConfig.String())
		}
		if !BoolDefault(test.Properties.Auto_gen_config, true) {
			entries.SetBool("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", true)
		}
	})

	androidMkWriteTestData(test.data, ctx, entries)
}

func (fuzz *fuzzBinary) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, fuzz.binaryDecorator)

	var fuzzFiles []string
	for _, d := range fuzz.corpus {
		fuzzFiles = append(fuzzFiles,
			filepath.Dir(fuzz.corpusIntermediateDir.String())+":corpus/"+d.Base())
	}

	for _, d := range fuzz.data {
		fuzzFiles = append(fuzzFiles,
			filepath.Dir(fuzz.dataIntermediateDir.String())+":data/"+d.Rel())
	}

	if fuzz.dictionary != nil {
		fuzzFiles = append(fuzzFiles,
			filepath.Dir(fuzz.dictionary.String())+":"+fuzz.dictionary.Base())
	}

	if fuzz.config != nil {
		fuzzFiles = append(fuzzFiles,
			filepath.Dir(fuzz.config.String())+":config.json")
	}

	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		entries.SetBool("LOCAL_IS_FUZZ_TARGET", true)
		if len(fuzzFiles) > 0 {
			entries.AddStrings("LOCAL_TEST_DATA", fuzzFiles...)
		}
		if fuzz.installedSharedDeps != nil {
			entries.AddStrings("LOCAL_FUZZ_INSTALLED_SHARED_DEPS", fuzz.installedSharedDeps...)
		}
	})
}

func (test *testLibrary) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	ctx.subAndroidMk(entries, test.libraryDecorator)
}

func (library *toolchainLibraryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "STATIC_LIBRARIES"
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		_, suffix, _ := android.SplitFileExt(entries.OutputFile.Path().Base())
		entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
	})
}

func (installer *baseInstaller) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	// Soong installation is only supported for host modules. Have Make
	// installation trigger Soong installation.
	if ctx.Target().Os.Class == android.Host {
		entries.OutputFile = android.OptionalPathForPath(installer.path)
	}

	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		path, file := filepath.Split(installer.path.ToMakePath().String())
		stem, suffix, _ := android.SplitFileExt(file)
		entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
		entries.SetString("LOCAL_MODULE_PATH", path)
		entries.SetString("LOCAL_MODULE_STEM", stem)
	})
}

func (c *stubDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.SubName = ndkLibrarySuffix + "." + c.properties.ApiLevel
	entries.Class = "SHARED_LIBRARIES"

	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		path, file := filepath.Split(c.installPath.String())
		stem, suffix, _ := android.SplitFileExt(file)
		entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
		entries.SetString("LOCAL_MODULE_PATH", path)
		entries.SetString("LOCAL_MODULE_STEM", stem)
		entries.SetBool("LOCAL_NO_NOTICE_FILE", true)
	})
}

func (c *llndkStubDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "SHARED_LIBRARIES"

	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		c.libraryDecorator.androidMkWriteExportedFlags(entries)
		_, _, ext := android.SplitFileExt(entries.OutputFile.Path().Base())

		entries.SetString("LOCAL_BUILT_MODULE_STEM", "$(LOCAL_MODULE)"+ext)
		entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
		entries.SetBool("LOCAL_NO_NOTICE_FILE", true)
		entries.SetString("LOCAL_SOONG_TOC", c.toc().String())
	})
}

func (c *vndkPrebuiltLibraryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "SHARED_LIBRARIES"

	entries.SubName = c.androidMkSuffix

	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		c.libraryDecorator.androidMkWriteExportedFlags(entries)

		path, file := filepath.Split(c.path.ToMakePath().String())
		stem, suffix, ext := android.SplitFileExt(file)
		entries.SetString("LOCAL_BUILT_MODULE_STEM", "$(LOCAL_MODULE)"+ext)
		entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
		entries.SetString("LOCAL_MODULE_PATH", path)
		entries.SetString("LOCAL_MODULE_STEM", stem)
		if c.tocFile.Valid() {
			entries.SetString("LOCAL_SOONG_TOC", c.tocFile.String())
		}
	})
}

func (c *vendorSnapshotLibraryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	// Each vendor snapshot is exported to androidMk only when BOARD_VNDK_VERSION != current
	// and the version of the prebuilt is same as BOARD_VNDK_VERSION.
	if c.shared() {
		entries.Class = "SHARED_LIBRARIES"
	} else if c.static() {
		entries.Class = "STATIC_LIBRARIES"
	} else if c.header() {
		entries.Class = "HEADER_LIBRARIES"
	}

	if c.androidMkVendorSuffix {
		entries.SubName = vendorSuffix
	} else {
		entries.SubName = ""
	}

	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		c.libraryDecorator.androidMkWriteExportedFlags(entries)

		if c.shared() || c.static() {
			path, file := filepath.Split(c.path.ToMakePath().String())
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
		}

		if !c.shared() { // static or header
			entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
		}
	})
}

func (c *vendorSnapshotBinaryDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "EXECUTABLES"

	if c.androidMkVendorSuffix {
		entries.SubName = vendorSuffix
	} else {
		entries.SubName = ""
	}

	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		entries.AddStrings("LOCAL_MODULE_SYMLINKS", c.Properties.Symlinks...)
	})
}

func (c *ndkPrebuiltStlLinker) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "SHARED_LIBRARIES"
}

func (c *vendorPublicLibraryStubDecorator) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.Class = "SHARED_LIBRARIES"
	entries.SubName = vendorPublicLibrarySuffix

	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		c.libraryDecorator.androidMkWriteExportedFlags(entries)
		_, _, ext := android.SplitFileExt(entries.OutputFile.Path().Base())

		entries.SetString("LOCAL_BUILT_MODULE_STEM", "$(LOCAL_MODULE)"+ext)
		entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
		entries.SetBool("LOCAL_NO_NOTICE_FILE", true)
	})
}

func (p *prebuiltLinker) AndroidMkEntries(ctx AndroidMkContext, entries *android.AndroidMkEntries) {
	entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
		if p.properties.Check_elf_files != nil {
			entries.SetBool("LOCAL_CHECK_ELF_FILES", *p.properties.Check_elf_files)
		} else {
			// soong_cc_prebuilt.mk does not include check_elf_file.mk by default
			// because cc_library_shared and cc_binary use soong_cc_prebuilt.mk as well.
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

func androidMkWriteAllowUndefinedSymbols(linker *baseLinker, entries *android.AndroidMkEntries) {
	allow := linker.Properties.Allow_undefined_symbols
	if allow != nil {
		entries.ExtraEntries = append(entries.ExtraEntries, func(entries *android.AndroidMkEntries) {
			entries.SetBool("LOCAL_ALLOW_UNDEFINED_SYMBOLS", *allow)
		})
	}
}
