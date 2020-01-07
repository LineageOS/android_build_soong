// Copyright 2019 The Android Open Source Project
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

package rust

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"android/soong/android"
)

type AndroidMkContext interface {
	Name() string
	Target() android.Target
	subAndroidMk(*android.AndroidMkData, interface{})
}

type subAndroidMkProvider interface {
	AndroidMk(AndroidMkContext, *android.AndroidMkData)
}

func (mod *Module) subAndroidMk(data *android.AndroidMkData, obj interface{}) {
	if mod.subAndroidMkOnce == nil {
		mod.subAndroidMkOnce = make(map[subAndroidMkProvider]bool)
	}
	if androidmk, ok := obj.(subAndroidMkProvider); ok {
		if !mod.subAndroidMkOnce[androidmk] {
			mod.subAndroidMkOnce[androidmk] = true
			androidmk.AndroidMk(mod, data)
		}
	}
}

func (mod *Module) AndroidMk() android.AndroidMkData {
	ret := android.AndroidMkData{
		OutputFile: mod.outputFile,
		Include:    "$(BUILD_SYSTEM)/soong_rust_prebuilt.mk",
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				if len(mod.Properties.AndroidMkRlibs) > 0 {
					fmt.Fprintln(w, "LOCAL_RLIB_LIBRARIES := "+strings.Join(mod.Properties.AndroidMkRlibs, " "))
				}
				if len(mod.Properties.AndroidMkDylibs) > 0 {
					fmt.Fprintln(w, "LOCAL_DYLIB_LIBRARIES := "+strings.Join(mod.Properties.AndroidMkDylibs, " "))
				}
				if len(mod.Properties.AndroidMkProcMacroLibs) > 0 {
					fmt.Fprintln(w, "LOCAL_PROC_MACRO_LIBRARIES := "+strings.Join(mod.Properties.AndroidMkProcMacroLibs, " "))
				}
				if len(mod.Properties.AndroidMkSharedLibs) > 0 {
					fmt.Fprintln(w, "LOCAL_SHARED_LIBRARIES := "+strings.Join(mod.Properties.AndroidMkSharedLibs, " "))
				}
				if len(mod.Properties.AndroidMkStaticLibs) > 0 {
					fmt.Fprintln(w, "LOCAL_STATIC_LIBRARIES := "+strings.Join(mod.Properties.AndroidMkStaticLibs, " "))
				}
			},
		},
	}

	mod.subAndroidMk(&ret, mod.compiler)

	ret.SubName += mod.Properties.SubName

	return ret
}

func (binary *binaryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, binary.baseCompiler)

	ret.Class = "EXECUTABLES"
	ret.DistFile = binary.distFile
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		fmt.Fprintln(w, "LOCAL_SOONG_UNSTRIPPED_BINARY :=", binary.unstrippedOutputFile.String())
	})
}

func (test *testDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	test.binaryDecorator.AndroidMk(ctx, ret)
	ret.Class = "NATIVE_TESTS"
	ret.SubName = test.getMutatedModuleSubName(ctx.Name())
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
	// TODO(chh): add test data with androidMkWriteTestData(test.data, ctx, ret)
}

func (library *libraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, library.baseCompiler)

	if library.rlib() {
		ret.Class = "RLIB_LIBRARIES"
	} else if library.dylib() {
		ret.Class = "DYLIB_LIBRARIES"
	} else if library.static() {
		ret.Class = "STATIC_LIBRARIES"
	} else if library.shared() {
		ret.Class = "SHARED_LIBRARIES"
	}

	ret.DistFile = library.distFile
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		if !library.rlib() {
			fmt.Fprintln(w, "LOCAL_SOONG_UNSTRIPPED_BINARY :=", library.unstrippedOutputFile.String())
		}
	})
}

func (procMacro *procMacroDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ctx.subAndroidMk(ret, procMacro.baseCompiler)

	ret.Class = "PROC_MACRO_LIBRARIES"
	ret.DistFile = procMacro.distFile

}

func (compiler *baseCompiler) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	// Soong installation is only supported for host modules. Have Make
	// installation trigger Soong installation.
	if ctx.Target().Os.Class == android.Host {
		ret.OutputFile = android.OptionalPathForPath(compiler.path)
	}
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		path, file := filepath.Split(compiler.path.ToMakePath().String())
		stem, suffix, _ := android.SplitFileExt(file)
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+suffix)
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := "+path)
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
	})
}
