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
	"path/filepath"

	"android/soong/android"
	"android/soong/cc"
)

type AndroidMkContext interface {
	Name() string
	Target() android.Target
	SubAndroidMk(*android.AndroidMkEntries, interface{})
}

type SubAndroidMkProvider interface {
	AndroidMk(AndroidMkContext, *android.AndroidMkEntries)
}

func (mod *Module) SubAndroidMk(data *android.AndroidMkEntries, obj interface{}) {
	if mod.subAndroidMkOnce == nil {
		mod.subAndroidMkOnce = make(map[SubAndroidMkProvider]bool)
	}
	if androidmk, ok := obj.(SubAndroidMkProvider); ok {
		if !mod.subAndroidMkOnce[androidmk] {
			mod.subAndroidMkOnce[androidmk] = true
			androidmk.AndroidMk(mod, data)
		}
	}
}

func (mod *Module) AndroidMkEntries() []android.AndroidMkEntries {
	if mod.Properties.HideFromMake || mod.hideApexVariantFromMake {

		return []android.AndroidMkEntries{android.AndroidMkEntries{Disabled: true}}
	}

	ret := android.AndroidMkEntries{
		OutputFile: mod.outputFile,
		Include:    "$(BUILD_SYSTEM)/soong_rust_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.AddStrings("LOCAL_RLIB_LIBRARIES", mod.Properties.AndroidMkRlibs...)
				entries.AddStrings("LOCAL_DYLIB_LIBRARIES", mod.Properties.AndroidMkDylibs...)
				entries.AddStrings("LOCAL_PROC_MACRO_LIBRARIES", mod.Properties.AndroidMkProcMacroLibs...)
				entries.AddStrings("LOCAL_SHARED_LIBRARIES", mod.Properties.AndroidMkSharedLibs...)
				entries.AddStrings("LOCAL_STATIC_LIBRARIES", mod.Properties.AndroidMkStaticLibs...)
				entries.AddStrings("LOCAL_SOONG_LINK_TYPE", mod.makeLinkType)
			},
		},
	}

	if mod.compiler != nil && !mod.compiler.Disabled() {
		mod.SubAndroidMk(&ret, mod.compiler)
	} else if mod.sourceProvider != nil {
		// If the compiler is disabled, this is a SourceProvider.
		mod.SubAndroidMk(&ret, mod.sourceProvider)
	}
	ret.SubName += mod.Properties.SubName

	return []android.AndroidMkEntries{ret}
}

func (binary *binaryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	ctx.SubAndroidMk(ret, binary.baseCompiler)

	if binary.distFile.Valid() {
		ret.DistFiles = android.MakeDefaultDistFiles(binary.distFile.Path())
	}

	ret.Class = "EXECUTABLES"
}

func (test *testDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	ctx.SubAndroidMk(ret, test.binaryDecorator)

	ret.Class = "NATIVE_TESTS"
	ret.ExtraEntries = append(ret.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.AddCompatibilityTestSuites(test.Properties.Test_suites...)
			if test.testConfig != nil {
				entries.SetString("LOCAL_FULL_TEST_CONFIG", test.testConfig.String())
			}
			entries.SetBoolIfTrue("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", !BoolDefault(test.Properties.Auto_gen_config, true))
			entries.SetBoolIfTrue("LOCAL_IS_UNIT_TEST", Bool(test.Properties.Test_options.Unit_test))
		})

	cc.AndroidMkWriteTestData(test.data, ret)
}

func (library *libraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	ctx.SubAndroidMk(ret, library.baseCompiler)

	if library.rlib() {
		ret.Class = "RLIB_LIBRARIES"
	} else if library.dylib() {
		ret.Class = "DYLIB_LIBRARIES"
	} else if library.static() {
		ret.Class = "STATIC_LIBRARIES"
	} else if library.shared() {
		ret.Class = "SHARED_LIBRARIES"
	}

	if library.distFile.Valid() {
		ret.DistFiles = android.MakeDefaultDistFiles(library.distFile.Path())
	}
}

func (procMacro *procMacroDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	ctx.SubAndroidMk(ret, procMacro.baseCompiler)

	ret.Class = "PROC_MACRO_LIBRARIES"
	if procMacro.distFile.Valid() {
		ret.DistFiles = android.MakeDefaultDistFiles(procMacro.distFile.Path())
	}

}

func (sourceProvider *BaseSourceProvider) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	outFile := sourceProvider.OutputFiles[0]
	ret.Class = "ETC"
	ret.OutputFile = android.OptionalPathForPath(outFile)
	ret.SubName += sourceProvider.subName
	ret.ExtraEntries = append(ret.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			_, file := filepath.Split(outFile.String())
			stem, suffix, _ := android.SplitFileExt(file)
			entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
			entries.SetString("LOCAL_MODULE_STEM", stem)
			entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
		})
}

func (bindgen *bindgenDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	ctx.SubAndroidMk(ret, bindgen.BaseSourceProvider)
}

func (proto *protobufDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	ctx.SubAndroidMk(ret, proto.BaseSourceProvider)
}

func (compiler *baseCompiler) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	if compiler.path == (android.InstallPath{}) {
		return
	}

	var unstrippedOutputFile android.OptionalPath
	// Soong installation is only supported for host modules. Have Make
	// installation trigger Soong installation.
	if ctx.Target().Os.Class == android.Host {
		ret.OutputFile = android.OptionalPathForPath(compiler.path)
	} else if compiler.strippedOutputFile.Valid() {
		unstrippedOutputFile = ret.OutputFile
		ret.OutputFile = compiler.strippedOutputFile
	}
	ret.ExtraEntries = append(ret.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.SetOptionalPath("LOCAL_SOONG_UNSTRIPPED_BINARY", unstrippedOutputFile)
			path, file := filepath.Split(compiler.path.ToMakePath().String())
			stem, suffix, _ := android.SplitFileExt(file)
			entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
			entries.SetString("LOCAL_MODULE_PATH", path)
			entries.SetString("LOCAL_MODULE_STEM", stem)
		})
}
