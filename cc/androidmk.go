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
	vendorSuffix = ".vendor"
)

type AndroidMkContext interface {
	Target() android.Target
	subAndroidMk(*android.AndroidMkData, interface{})
	useVndk() bool
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
	if c.Properties.HideFromMake {
		return android.AndroidMkData{
			Disabled: true,
		}
	}

	ret := android.AndroidMkData{
		OutputFile: c.outputFile,
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				if len(c.Properties.Logtags) > 0 {
					fmt.Fprintln(w, "LOCAL_LOGTAGS_FILES :=", strings.Join(c.Properties.Logtags, " "))
				}
				fmt.Fprintln(w, "LOCAL_SANITIZE := never")
				if len(c.Properties.AndroidMkSharedLibs) > 0 {
					fmt.Fprintln(w, "LOCAL_SHARED_LIBRARIES := "+strings.Join(c.Properties.AndroidMkSharedLibs, " "))
				}
				if c.Target().Os == android.Android &&
					String(c.Properties.Sdk_version) != "" && !c.useVndk() {
					fmt.Fprintln(w, "LOCAL_SDK_VERSION := "+String(c.Properties.Sdk_version))
					fmt.Fprintln(w, "LOCAL_NDK_STL_VARIANT := none")
				} else {
					// These are already included in LOCAL_SHARED_LIBRARIES
					fmt.Fprintln(w, "LOCAL_CXX_STL := none")
				}
				if c.useVndk() {
					fmt.Fprintln(w, "LOCAL_USE_VNDK := true")
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

	if c.useVndk() && c.hasVendorVariant() {
		// .vendor suffix is added only when we will have two variants: core and vendor.
		// The suffix is not added for vendor-only module.
		ret.SubName += vendorSuffix
	}

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

func (library *libraryDecorator) androidMkWriteExportedFlags(w io.Writer) {
	exportedFlags := library.exportedFlags()
	if len(exportedFlags) > 0 {
		fmt.Fprintln(w, "LOCAL_EXPORT_CFLAGS :=", strings.Join(exportedFlags, " "))
	}
	exportedFlagsDeps := library.exportedFlagsDeps()
	if len(exportedFlagsDeps) > 0 {
		fmt.Fprintln(w, "LOCAL_EXPORT_C_INCLUDE_DEPS :=", strings.Join(exportedFlagsDeps.Strings(), " "))
	}
}

func (library *libraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	if library.static() {
		ret.Class = "STATIC_LIBRARIES"
	} else if library.shared() {
		ctx.subAndroidMk(ret, &library.stripper)
		ctx.subAndroidMk(ret, &library.relocationPacker)

		ret.Class = "SHARED_LIBRARIES"
	} else if library.header() {
		ret.Custom = func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
			fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
			fmt.Fprintln(w, "LOCAL_MODULE :=", name+data.SubName)

			archStr := ctx.Target().Arch.ArchType.String()
			var host bool
			switch ctx.Target().Os.Class {
			case android.Host:
				fmt.Fprintln(w, "LOCAL_MODULE_HOST_ARCH := ", archStr)
				host = true
			case android.HostCross:
				fmt.Fprintln(w, "LOCAL_MODULE_HOST_CROSS_ARCH := ", archStr)
				host = true
			case android.Device:
				fmt.Fprintln(w, "LOCAL_MODULE_TARGET_ARCH := ", archStr)
			}

			if host {
				makeOs := ctx.Target().Os.String()
				if ctx.Target().Os == android.Linux || ctx.Target().Os == android.LinuxBionic {
					makeOs = "linux"
				}
				fmt.Fprintln(w, "LOCAL_MODULE_HOST_OS :=", makeOs)
				fmt.Fprintln(w, "LOCAL_IS_HOST_MODULE := true")
			} else if ctx.useVndk() {
				fmt.Fprintln(w, "LOCAL_USE_VNDK := true")
			}
			if len(library.Properties.Additional_deps) > 0 {
				fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES := "+strings.Join(library.Properties.Additional_deps, " "))
			}

			library.androidMkWriteExportedFlags(w)
			fmt.Fprintln(w, "include $(BUILD_HEADER_LIBRARY)")
		}

		return
	}

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		library.androidMkWriteExportedFlags(w)
		fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES := ")
		if library.sAbiOutputFile.Valid() {
			fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES += ", library.sAbiOutputFile.String())
			if library.sAbiDiff.Valid() && !library.static() {
				fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES += ", library.sAbiDiff.String())
				fmt.Fprintln(w, "HEADER_ABI_DIFFS += ", library.sAbiDiff.String())
			}
		}

		fmt.Fprintln(w, "LOCAL_BUILT_MODULE_STEM := $(LOCAL_MODULE)"+outputFile.Ext())

		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")

		if library.coverageOutputFile.Valid() {
			fmt.Fprintln(w, "LOCAL_PREBUILT_COVERAGE_ARCHIVE :=", library.coverageOutputFile.String())
		}
	})

	if library.shared() {
		ctx.subAndroidMk(ret, library.baseInstaller)
	}
}

func (object *objectLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Custom = func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
		out := ret.OutputFile.Path()

		fmt.Fprintln(w, "\n$("+prefix+"OUT_INTERMEDIATE_LIBRARIES)/"+name+data.SubName+objectExtension+":", out.String())
		fmt.Fprintln(w, "\t$(copy-file-to-target)")
	}
}

func (binary *binaryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, binary.baseInstaller)
	ctx.subAndroidMk(ret, &binary.stripper)

	ret.Class = "EXECUTABLES"
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")
		if Bool(binary.Properties.Static_executable) {
			fmt.Fprintln(w, "LOCAL_FORCE_STATIC_EXECUTABLE := true")
		}

		if len(binary.symlinks) > 0 {
			fmt.Fprintln(w, "LOCAL_MODULE_SYMLINKS := "+strings.Join(binary.symlinks, " "))
		}

		if binary.coverageOutputFile.Valid() {
			fmt.Fprintln(w, "LOCAL_PREBUILT_COVERAGE_ARCHIVE :=", binary.coverageOutputFile.String())
		}

		if len(binary.Properties.Overrides) > 0 {
			fmt.Fprintln(w, "LOCAL_OVERRIDES_MODULES := "+strings.Join(binary.Properties.Overrides, " "))
		}
		if len(binary.Properties.Additional_deps) > 0 {
			fmt.Fprintln(w, "LOCAL_ADDITIONAL_DEPENDENCIES := "+strings.Join(binary.Properties.Additional_deps, " "))
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
	})

	androidMkWriteTestData(test.data, ctx, ret)
}

func (test *testLibrary) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, test.libraryDecorator)
}

func (library *toolchainLibraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "STATIC_LIBRARIES"
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+outputFile.Ext())
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")
	})
}

func (stripper *stripper) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	// Make only supports stripping target modules
	if ctx.Target().Os != android.Android {
		return
	}

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		if Bool(stripper.StripProperties.Strip.None) {

			fmt.Fprintln(w, "LOCAL_STRIP_MODULE := false")
		} else if Bool(stripper.StripProperties.Strip.Keep_symbols) {
			fmt.Fprintln(w, "LOCAL_STRIP_MODULE := keep_symbols")
		} else {
			fmt.Fprintln(w, "LOCAL_STRIP_MODULE := mini-debug-info")
		}
	})
}

func (packer *relocationPacker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		if packer.Properties.PackingRelocations {
			fmt.Fprintln(w, "LOCAL_PACK_MODULE_RELOCATIONS := true")
		}
	})
}

func (installer *baseInstaller) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	// Soong installation is only supported for host modules. Have Make
	// installation trigger Soong installation.
	if ctx.Target().Os.Class == android.Host {
		ret.OutputFile = android.OptionalPathForPath(installer.path)
	}

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		path := installer.path.RelPathString()
		dir, file := filepath.Split(path)
		stem := strings.TrimSuffix(file, filepath.Ext(file))
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+filepath.Ext(file))
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := $(OUT_DIR)/"+filepath.Clean(dir))
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
	})
}

func (c *stubDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.SubName = ndkLibrarySuffix + "." + c.properties.ApiLevel
	ret.Class = "SHARED_LIBRARIES"

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		path, file := filepath.Split(c.installPath.String())
		stem := strings.TrimSuffix(file, filepath.Ext(file))
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+outputFile.Ext())
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := "+path)
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
		fmt.Fprintln(w, "LOCAL_NO_NOTICE_FILE := true")

		// Prevent make from installing the libraries to obj/lib (since we have
		// dozens of libraries with the same name, they'll clobber each other
		// and the real versions of the libraries from the platform).
		fmt.Fprintln(w, "LOCAL_COPY_TO_INTERMEDIATE_LIBRARIES := false")
	})
}

func (c *llndkStubDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "SHARED_LIBRARIES"
	ret.SubName = ".vendor"

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		c.libraryDecorator.androidMkWriteExportedFlags(w)

		fmt.Fprintln(w, "LOCAL_BUILT_MODULE_STEM := $(LOCAL_MODULE)"+outputFile.Ext())
		fmt.Fprintln(w, "LOCAL_STRIP_MODULE := false")
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")
		fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
		fmt.Fprintln(w, "LOCAL_NO_NOTICE_FILE := true")
		fmt.Fprintln(w, "LOCAL_USE_VNDK := true")
	})
}

func (c *vndkPrebuiltLibraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "SHARED_LIBRARIES"

	ret.SubName = c.NameSuffix()

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		c.libraryDecorator.androidMkWriteExportedFlags(w)

		path := c.path.RelPathString()
		dir, file := filepath.Split(path)
		stem := strings.TrimSuffix(file, filepath.Ext(file))
		fmt.Fprintln(w, "LOCAL_STRIP_MODULE := false")
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")
		fmt.Fprintln(w, "LOCAL_USE_VNDK := true")
		fmt.Fprintln(w, "LOCAL_BUILT_MODULE_STEM := $(LOCAL_MODULE)"+outputFile.Ext())
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+filepath.Ext(file))
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := $(OUT_DIR)/"+filepath.Clean(dir))
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
	})
}

func (c *ndkPrebuiltStlLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "SHARED_LIBRARIES"

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		// Prevent make from installing the libraries to obj/lib (since we have
		// dozens of libraries with the same name, they'll clobber each other
		// and the real versions of the libraries from the platform).
		fmt.Fprintln(w, "LOCAL_COPY_TO_INTERMEDIATE_LIBRARIES := false")
	})
}

func (c *vendorPublicLibraryStubDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Class = "SHARED_LIBRARIES"
	ret.SubName = vendorPublicLibrarySuffix

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		c.libraryDecorator.androidMkWriteExportedFlags(w)

		fmt.Fprintln(w, "LOCAL_BUILT_MODULE_STEM := $(LOCAL_MODULE)"+outputFile.Ext())
		fmt.Fprintln(w, "LOCAL_STRIP_MODULE := false")
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")
		fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
		fmt.Fprintln(w, "LOCAL_NO_NOTICE_FILE := true")
	})
}
