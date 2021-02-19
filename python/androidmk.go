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

package python

import (
	"path/filepath"
	"strings"

	"android/soong/android"
)

type subAndroidMkProvider interface {
	AndroidMk(*Module, *android.AndroidMkEntries)
}

func (p *Module) subAndroidMk(entries *android.AndroidMkEntries, obj interface{}) {
	if p.subAndroidMkOnce == nil {
		p.subAndroidMkOnce = make(map[subAndroidMkProvider]bool)
	}
	if androidmk, ok := obj.(subAndroidMkProvider); ok {
		if !p.subAndroidMkOnce[androidmk] {
			p.subAndroidMkOnce[androidmk] = true
			androidmk.AndroidMk(p, entries)
		}
	}
}

func (p *Module) AndroidMkEntries() []android.AndroidMkEntries {
	entries := android.AndroidMkEntries{OutputFile: p.installSource}

	p.subAndroidMk(&entries, p.installer)

	return []android.AndroidMkEntries{entries}
}

func (p *binaryDecorator) AndroidMk(base *Module, entries *android.AndroidMkEntries) {
	entries.Class = "EXECUTABLES"

	entries.ExtraEntries = append(entries.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.AddCompatibilityTestSuites(p.binaryProperties.Test_suites...)
		})
	base.subAndroidMk(entries, p.pythonInstaller)
}

func (p *testDecorator) AndroidMk(base *Module, entries *android.AndroidMkEntries) {
	entries.Class = "NATIVE_TESTS"

	entries.ExtraEntries = append(entries.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			entries.AddCompatibilityTestSuites(p.binaryDecorator.binaryProperties.Test_suites...)
			if p.testConfig != nil {
				entries.SetString("LOCAL_FULL_TEST_CONFIG", p.testConfig.String())
			}

			entries.SetBoolIfTrue("LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG", !BoolDefault(p.binaryProperties.Auto_gen_config, true))

			entries.AddStrings("LOCAL_TEST_DATA", android.AndroidMkDataPaths(p.data)...)

			entries.SetBoolIfTrue("LOCAL_IS_UNIT_TEST", Bool(p.testProperties.Test_options.Unit_test))
		})
	base.subAndroidMk(entries, p.binaryDecorator.pythonInstaller)
}

func (installer *pythonInstaller) AndroidMk(base *Module, entries *android.AndroidMkEntries) {
	// Soong installation is only supported for host modules. Have Make
	// installation trigger Soong installation.
	if base.Target().Os.Class == android.Host {
		entries.OutputFile = android.OptionalPathForPath(installer.path)
	}

	entries.Required = append(entries.Required, "libc++")
	entries.ExtraEntries = append(entries.ExtraEntries,
		func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
			path, file := filepath.Split(installer.path.ToMakePath().String())
			stem := strings.TrimSuffix(file, filepath.Ext(file))

			entries.SetString("LOCAL_MODULE_SUFFIX", filepath.Ext(file))
			entries.SetString("LOCAL_MODULE_PATH", path)
			entries.SetString("LOCAL_MODULE_STEM", stem)
			entries.AddStrings("LOCAL_SHARED_LIBRARIES", installer.androidMkSharedLibs...)
			entries.SetBool("LOCAL_CHECK_ELF_FILES", false)
		})
}
