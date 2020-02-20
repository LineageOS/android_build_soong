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
	subAndroidMk(*android.AndroidMkData, interface{})
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
	AndroidMk(AndroidMkContext, *android.AndroidMkData)
}

func (c *Module) subAndroidMk(data *android.AndroidMkData, obj interface{}) {
	if c.subAndroidMkOnce == nil {
		c.subAndroidMkOnce = make(map[subAndroidMkProvider]bool)
	}
	if androidmk, ok := obj.(subAndroidMkProvider); ok {
		if !c.subAndroidMkOnce[androidmk] {
			c.subAndroidMkOnce[androidmk] = true
			androidmk.AndroidMk(c, data)
		}
	}
}

func (c *Module) AndroidMk() android.AndroidMkData {
	if c.Properties.HideFromMake || !c.IsForPlatform() {
		return android.AndroidMkData{
			Disabled: true,
		}
	}

	ret := android.AndroidMkData{
		OutputFile: c.outputFile,
		// TODO(jiyong): add the APEXes providing shared libs to the required modules
		// Currently, adding c.Properties.ApexesProvidingSharedLibs is causing multiple
		// ART APEXes (com.android.art.debug|release) to be installed. And this
		// is breaking some older devices (like marlin) where system.img is small.
		Required: c.Properties.AndroidMkRuntimeLibs,
		Include:  "$(BUILD_SYSTEM)/soong_cc_prebuilt.mk",

		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				if len(c.Properties.Logtags) > 0 {
					fmt.Fprintln(w, "LOCAL_LOGTAGS_FILES :=", strings.Join(c.Properties.Logtags, " "))
				}
				if len(c.Properties.AndroidMkSharedLibs) > 0 {
					fmt.Fprintln(w, "LOCAL_SHARED_LIBRARIES := "+strings.Join(c.Properties.AndroidMkSharedLibs, " "))
				}
				if len(c.Properties.AndroidMkStaticLibs) > 0 {
					fmt.Fprintln(w, "LOCAL_STATIC_LIBRARIES := "+strings.Join(c.Properties.AndroidMkStaticLibs, " "))
				}
				if len(c.Properties.AndroidMkWholeStaticLibs) > 0 {
					fmt.Fprintln(w, "LOCAL_WHOLE_STATIC_LIBRARIES := "+strings.Join(c.Properties.AndroidMkWholeStaticLibs, " "))
				}
				fmt.Fprintln(w, "LOCAL_SOONG_LINK_TYPE :=", c.makeLinkType)
				if c.UseVndk() {
					fmt.Fprintln(w, "LOCAL_USE_VNDK := true")
					if c.IsVndk() && !c.static() {
						fmt.Fprintln(w, "LOCAL_SOONG_VNDK_VERSION := "+c.VndkVersion())
						// VNDK libraries available to vendor are not installed because
						// they are packaged in VNDK APEX and installed by APEX packages (apex/apex.go)
						if !c.isVndkExt() {
							fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
						}
					}
				}
			},
		},
	}

	for _, feature := range c.features {
		c.subAndroidMk(&ret, feature)
	}

	c.subAndroidMk(&ret, c.compiler)
	c.subAndroidMk(&ret, c.linker)
	if c.sanitize != nil {
		c.subAndroidMk(&ret, c.sanitize)
	}
	c.subAndroidMk(&ret, c.installer)

	ret.SubName += c.Properties.SubName

	return ret
}

func androidMkWriteTestData(data android.Paths, ctx AndroidMkContext, ret *android.AndroidMkData) {
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
		ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
			fmt.Fprintln(w, "LOCAL_TEST_DATA := "+strings.Join(testFiles, " "))
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

func (library *libraryDecorator) androidMkWriteExportedFlags(w io.Writer) {
	exportedFlags := library.exportedFlags()
	for _, dir := range library.exportedDirs() {
		exportedFlags = append(exportedFlags, "-I"+dir.String())
	}
	for _, dir := range library.exportedSystemDirs() {
		exportedFlags = append(exportedFlags, "-isystem "+dir.String())
	}
	if len(exportedFlags) > 0 {
		fmt.Fprintln(w, "LOCAL_EXPORT_CFLAGS :=", strings.Join(exportedFlags, " "))
	}
	exportedDeps := library.exportedDeps()
	if len(exportedDeps) > 0 {
		fmt.Fprintln(w, "LOCAL_EXPORT_C_INCLUDE_DEPS :=", strings.Join(exportedDeps.Strings(), " "))
	}
}

func (library *libraryDecorator) androidMkWriteAdditionalDependenciesForSourceAbiDiff(w io.Writer) {
	if library.sAbiOutputFile.Valid() {
		fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES +=", library.sAbiOutputFile.String())
		if library.sAbiDiff.Valid() && !library.static() {
			fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES +=", library.sAbiDiff.String())
			fmt.Fprintln(w, "HEADER_ABI_DIFFS +=", library.sAbiDiff.String())
		}
	}
}

func (library *libraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	if library.static() {
		ret.Class = "STATIC_LIBRARIES"
	} else if library.shared() {
		ret.Class = "SHARED_LIBRARIES"
		ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
			fmt.Fprintln(w, "LOCAL_SOONG_TOC :=", library.toc().String())
			if !library.buildStubs() {
				fmt.Fprintln(w, "LOCAL_SOONG_UNSTRIPPED_BINARY :=", library.unstrippedOutputFile.String())
			}
			if len(library.Properties.Overrides) > 0 {
				fmt.Fprintln(w, "LOCAL_OVERRIDES_MODULES := "+strings.Join(makeOverrideModuleNames(ctx, library.Properties.Overrides), " "))
			}
			if len(library.post_install_cmds) > 0 {
				fmt.Fprintln(w, "LOCAL_POST_INSTALL_CMD := "+strings.Join(library.post_install_cmds, "&& "))
			}
		})
	} else if library.header() {
		ret.Class = "HEADER_LIBRARIES"
	}

	ret.DistFile = library.distFile
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		library.androidMkWriteExportedFlags(w)
		library.androidMkWriteAdditionalDependenciesForSourceAbiDiff(w)

		_, _, ext := android.SplitFileExt(outputFile.Base())

		fmt.Fprintln(w, "LOCAL_BUILT_MODULE_STEM := $(LOCAL_MODULE)"+ext)

		if library.coverageOutputFile.Valid() {
			fmt.Fprintln(w, "LOCAL_PREBUILT_COVERAGE_ARCHIVE :=", library.coverageOutputFile.String())
		}

		if library.useCoreVariant {
			fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
			fmt.Fprintln(w, "LOCAL_NO_NOTICE_FILE := true")
			fmt.Fprintln(w, "LOCAL_VNDK_DEPEND_ON_CORE_VARIANT := true")
		}
		if library.checkSameCoreVariant {
			fmt.Fprintln(w, "LOCAL_CHECK_SAME_VNDK_VARIANTS := true")
		}
	})

	if library.shared() && !library.buildStubs() {
		ctx.subAndroidMk(ret, library.baseInstaller)
	} else {
		ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
			fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
			if library.buildStubs() {
				fmt.Fprintln(w, "LOCAL_NO_NOTICE_FILE := true")
			}
		})
	}
	if len(library.Properties.Stubs.Versions) > 0 &&
		android.DirectlyInAnyApex(ctx, ctx.Name()) && !ctx.InRamdisk() && !ctx.InRecovery() && !ctx.UseVndk() &&
		!ctx.static() {
		if !library.buildStubs() {
			ret.SubName = ".bootstrap"
		}
	}
}

func (object *objectLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Custom = func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
		out := ret.OutputFile.Path()
		varname := fmt.Sprintf("SOONG_%sOBJECT_%s%s", prefix, name, data.SubName)

		fmt.Fprintf(w, "\n%s := %s\n", varname, out.String())
		fmt.Fprintln(w, ".KATI_READONLY: "+varname)
	}
}

func (binary *binaryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, binary.baseInstaller)

	ret.Class = "EXECUTABLES"
	ret.DistFile = binary.distFile
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		fmt.Fprintln(w, "LOCAL_SOONG_UNSTRIPPED_BINARY :=", binary.unstrippedOutputFile.String())
		if len(binary.symlinks) > 0 {
			fmt.Fprintln(w, "LOCAL_MODULE_SYMLINKS := "+strings.Join(binary.symlinks, " "))
		}

		if binary.coverageOutputFile.Valid() {
			fmt.Fprintln(w, "LOCAL_PREBUILT_COVERAGE_ARCHIVE :=", binary.coverageOutputFile.String())
		}

		if len(binary.Properties.Overrides) > 0 {
			fmt.Fprintln(w, "LOCAL_OVERRIDES_MODULES := "+strings.Join(makeOverrideModuleNames(ctx, binary.Properties.Overrides), " "))
		}
		if len(binary.post_install_cmds) > 0 {
			fmt.Fprintln(w, "LOCAL_POST_INSTALL_CMD := "+strings.Join(binary.post_install_cmds, "&& "))
		}
	})
}

func (benchmark *benchmarkDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, benchmark.binaryDecorator)
	ret.Class = "NATIVE_TESTS"
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		if len(benchmark.Properties.Test_suites) > 0 {
			fmt.Fprintln(w, "LOCAL_COMPATIBILITY_SUITE :=",
				strings.Join(benchmark.Properties.Test_suites, " "))
		}
		if benchmark.testConfig != nil {
			fmt.Fprintln(w, "LOCAL_FULL_TEST_CONFIG :=", benchmark.testConfig.String())
		}
		fmt.Fprintln(w, "LOCAL_NATIVE_BENCHMARK := true")
		if !BoolDefault(benchmark.Properties.Auto_gen_config, true) {
			fmt.Fprintln(w, "LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG := true")
		}
	})

	androidMkWriteTestData(benchmark.data, ctx, ret)
}

func (test *testBinary) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, test.binaryDecorator)
	ret.Class = "NATIVE_TESTS"
	if Bool(test.Properties.Test_per_src) {
		ret.SubName = "_" + String(test.binaryDecorator.Properties.Stem)
	}

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		if len(test.Properties.Test_suites) > 0 {
			fmt.Fprintln(w, "LOCAL_COMPATIBILITY_SUITE :=",
				strings.Join(test.Properties.Test_suites, " "))
		}
		if test.testConfig != nil {
			fmt.Fprintln(w, "LOCAL_FULL_TEST_CONFIG :=", test.testConfig.String())
		}
		if !BoolDefault(test.Properties.Auto_gen_config, true) {
			fmt.Fprintln(w, "LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG := true")
		}
	})

	androidMkWriteTestData(test.data, ctx, ret)
}

func (fuzz *fuzzBinary) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, fuzz.binaryDecorator)

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

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		fmt.Fprintln(w, "LOCAL_IS_FUZZ_TARGET := true")
		if len(fuzzFiles) > 0 {
			fmt.Fprintln(w, "LOCAL_TEST_DATA := "+strings.Join(fuzzFiles, " "))
		}
		if fuzz.installedSharedDeps != nil {
			fmt.Fprintln(w, "LOCAL_FUZZ_INSTALLED_SHARED_DEPS :="+
				strings.Join(fuzz.installedSharedDeps, " "))
		}
	})
}

func (test *testLibrary) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, test.libraryDecorator)
}

func (library *toolchainLibraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "STATIC_LIBRARIES"
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		_, suffix, _ := android.SplitFileExt(outputFile.Base())
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+suffix)
	})
}

func (installer *baseInstaller) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	// Soong installation is only supported for host modules. Have Make
	// installation trigger Soong installation.
	if ctx.Target().Os.Class == android.Host {
		ret.OutputFile = android.OptionalPathForPath(installer.path)
	}

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		path, file := filepath.Split(installer.path.ToMakePath().String())
		stem, suffix, _ := android.SplitFileExt(file)
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+suffix)
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := "+path)
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
	})
}

func (c *stubDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.SubName = ndkLibrarySuffix + "." + c.properties.ApiLevel
	ret.Class = "SHARED_LIBRARIES"

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		path, file := filepath.Split(c.installPath.String())
		stem, suffix, _ := android.SplitFileExt(file)
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+suffix)
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := "+path)
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
		fmt.Fprintln(w, "LOCAL_NO_NOTICE_FILE := true")
	})
}

func (c *llndkStubDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "SHARED_LIBRARIES"

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		c.libraryDecorator.androidMkWriteExportedFlags(w)
		_, _, ext := android.SplitFileExt(outputFile.Base())

		fmt.Fprintln(w, "LOCAL_BUILT_MODULE_STEM := $(LOCAL_MODULE)"+ext)
		fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
		fmt.Fprintln(w, "LOCAL_NO_NOTICE_FILE := true")
		fmt.Fprintln(w, "LOCAL_SOONG_TOC :=", c.toc().String())
	})
}

func (c *vndkPrebuiltLibraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "SHARED_LIBRARIES"

	ret.SubName = c.androidMkSuffix

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		c.libraryDecorator.androidMkWriteExportedFlags(w)

		path, file := filepath.Split(c.path.ToMakePath().String())
		stem, suffix, ext := android.SplitFileExt(file)
		fmt.Fprintln(w, "LOCAL_BUILT_MODULE_STEM := $(LOCAL_MODULE)"+ext)
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+suffix)
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := "+path)
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
		if c.tocFile.Valid() {
			fmt.Fprintln(w, "LOCAL_SOONG_TOC := "+c.tocFile.String())
		}
	})
}

func (c *vendorSnapshotLibraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	// Each vendor snapshot is exported to androidMk only when BOARD_VNDK_VERSION != current
	// and the version of the prebuilt is same as BOARD_VNDK_VERSION.
	if c.shared() {
		ret.Class = "SHARED_LIBRARIES"
	} else if c.static() {
		ret.Class = "STATIC_LIBRARIES"
	} else if c.header() {
		ret.Class = "HEADER_LIBRARIES"
	}

	if c.androidMkVendorSuffix {
		ret.SubName = vendorSuffix
	} else {
		ret.SubName = ""
	}

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		c.libraryDecorator.androidMkWriteExportedFlags(w)

		if c.shared() || c.static() {
			path, file := filepath.Split(c.path.ToMakePath().String())
			stem, suffix, ext := android.SplitFileExt(file)
			fmt.Fprintln(w, "LOCAL_BUILT_MODULE_STEM := $(LOCAL_MODULE)"+ext)
			fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+suffix)
			fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
			if c.shared() {
				fmt.Fprintln(w, "LOCAL_MODULE_PATH := "+path)
			}
			if c.tocFile.Valid() {
				fmt.Fprintln(w, "LOCAL_SOONG_TOC := "+c.tocFile.String())
			}
		}

		if !c.shared() { // static or header
			fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
		}
	})
}

func (c *vendorSnapshotBinaryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "EXECUTABLES"

	if c.androidMkVendorSuffix {
		ret.SubName = vendorSuffix
	} else {
		ret.SubName = ""
	}

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		fmt.Fprintln(w, "LOCAL_MODULE_SYMLINKS := "+strings.Join(c.Properties.Symlinks, " "))
	})
}

func (c *ndkPrebuiltStlLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "SHARED_LIBRARIES"
}

func (c *vendorPublicLibraryStubDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "SHARED_LIBRARIES"
	ret.SubName = vendorPublicLibrarySuffix

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		c.libraryDecorator.androidMkWriteExportedFlags(w)
		_, _, ext := android.SplitFileExt(outputFile.Base())

		fmt.Fprintln(w, "LOCAL_BUILT_MODULE_STEM := $(LOCAL_MODULE)"+ext)
		fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
		fmt.Fprintln(w, "LOCAL_NO_NOTICE_FILE := true")
	})
}

func (p *prebuiltLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		if p.properties.Check_elf_files != nil {
			fmt.Fprintln(w, "LOCAL_CHECK_ELF_FILES :=", *p.properties.Check_elf_files)
		} else {
			// soong_cc_prebuilt.mk does not include check_elf_file.mk by default
			// because cc_library_shared and cc_binary use soong_cc_prebuilt.mk as well.
			// In order to turn on prebuilt ABI checker, set `LOCAL_CHECK_ELF_FILES` to
			// true if `p.properties.Check_elf_files` is not specified.
			fmt.Fprintln(w, "LOCAL_CHECK_ELF_FILES := true")
		}
	})
}

func (p *prebuiltLibraryLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, p.libraryDecorator)
	if p.shared() {
		ctx.subAndroidMk(ret, &p.prebuiltLinker)
		androidMkWriteAllowUndefinedSymbols(p.baseLinker, ret)
	}
}

func (p *prebuiltBinaryLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, p.binaryDecorator)
	ctx.subAndroidMk(ret, &p.prebuiltLinker)
	androidMkWriteAllowUndefinedSymbols(p.baseLinker, ret)
}

func androidMkWriteAllowUndefinedSymbols(linker *baseLinker, ret *android.AndroidMkData) {
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		allow := linker.Properties.Allow_undefined_symbols
		if allow != nil {
			fmt.Fprintln(w, "LOCAL_ALLOW_UNDEFINED_SYMBOLS :=", *allow)
		}
	})
}
