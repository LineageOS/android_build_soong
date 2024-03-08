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

func (mod *Module) AndroidMkSuffix() string {
	return mod.Properties.RustSubName + mod.Properties.SubName
}

func (mod *Module) AndroidMkEntries() []android.AndroidMkEntries {
	if mod.Properties.HideFromMake || mod.hideApexVariantFromMake {

		return []android.AndroidMkEntries{android.AndroidMkEntries{Disabled: true}}
	}

	ret := android.AndroidMkEntries{
		OutputFile: android.OptionalPathForPath(mod.UnstrippedOutputFile()),
		Include:    "$(BUILD_SYSTEM)/soong_cc_rust_prebuilt.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.AddStrings("LOCAL_RLIB_LIBRARIES", mod.Properties.AndroidMkRlibs...)
				entries.AddStrings("LOCAL_DYLIB_LIBRARIES", mod.Properties.AndroidMkDylibs...)
				entries.AddStrings("LOCAL_PROC_MACRO_LIBRARIES", mod.Properties.AndroidMkProcMacroLibs...)
				entries.AddStrings("LOCAL_SHARED_LIBRARIES", mod.transitiveAndroidMkSharedLibs.ToList()...)
				entries.AddStrings("LOCAL_STATIC_LIBRARIES", mod.Properties.AndroidMkStaticLibs...)
				entries.AddStrings("LOCAL_SOONG_LINK_TYPE", mod.makeLinkType)
				if mod.UseVndk() {
					entries.SetBool("LOCAL_USE_VNDK", true)
				}
				// TODO(b/311155208): The container here should be system.
				entries.SetPaths("LOCAL_ACONFIG_FILES", mod.mergedAconfigFiles[""])
			},
		},
	}

	if mod.compiler != nil && !mod.compiler.Disabled() {
		mod.SubAndroidMk(&ret, mod.compiler)
	} else if mod.sourceProvider != nil {
		// If the compiler is disabled, this is a SourceProvider.
		mod.SubAndroidMk(&ret, mod.sourceProvider)
	}

	if mod.sanitize != nil {
		mod.SubAndroidMk(&ret, mod.sanitize)
	}

	ret.SubName += mod.AndroidMkSuffix()

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
			if test.Properties.Data_bins != nil {
				entries.AddStrings("LOCAL_TEST_DATA_BINS", test.Properties.Data_bins...)
			}

			test.Properties.Test_options.SetAndroidMkEntries(entries)
		})
}

func (benchmark *benchmarkDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	benchmark.binaryDecorator.AndroidMk(ctx, ret)
	ret.Class = "NATIVE_TESTS"
	ret.ExtraEntries = append(ret.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.AddCompatibilityTestSuites(benchmark.Properties.Test_suites...)
			if benchmark.testConfig != nil {
				entries.SetString("LOCAL_FULL_TEST_CONFIG", benchmark.testConfig.String())
			}
			entries.SetBool("LOCAL_NATIVE_BENCHMARK", true)
			entries.SetBoolIfTrue("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", !BoolDefault(benchmark.Properties.Auto_gen_config, true))
		})
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
	ret.ExtraEntries = append(ret.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			if library.tocFile.Valid() {
				entries.SetString("LOCAL_SOONG_TOC", library.tocFile.String())
			}
		})
}

func (library *snapshotLibraryDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	ctx.SubAndroidMk(ret, library.libraryDecorator)
	ret.SubName = library.SnapshotAndroidMkSuffix()
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

	if compiler.strippedOutputFile.Valid() {
		ret.OutputFile = compiler.strippedOutputFile
	}

	ret.ExtraEntries = append(ret.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.SetPath("LOCAL_SOONG_UNSTRIPPED_BINARY", compiler.unstrippedOutputFile)
			path, file := filepath.Split(compiler.path.String())
			stem, suffix, _ := android.SplitFileExt(file)
			entries.SetString("LOCAL_MODULE_SUFFIX", suffix)
			entries.SetString("LOCAL_MODULE_PATH", path)
			entries.SetString("LOCAL_MODULE_STEM", stem)
		})
}

func (fuzz *fuzzDecorator) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkEntries) {
	ctx.SubAndroidMk(ret, fuzz.binaryDecorator)

	ret.ExtraEntries = append(ret.ExtraEntries, func(ctx android.AndroidMkExtraEntriesContext,
		entries *android.AndroidMkEntries) {
		entries.SetBool("LOCAL_IS_FUZZ_TARGET", true)
		if fuzz.installedSharedDeps != nil {
			entries.AddStrings("LOCAL_FUZZ_INSTALLED_SHARED_DEPS", fuzz.installedSharedDeps...)
		}
	})
}
