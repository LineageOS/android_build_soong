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
	"android/soong/android"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type subAndroidMkProvider interface {
	AndroidMk(*Module, *android.AndroidMkData)
}

func (p *Module) subAndroidMk(data *android.AndroidMkData, obj interface{}) {
	if p.subAndroidMkOnce == nil {
		p.subAndroidMkOnce = make(map[subAndroidMkProvider]bool)
	}
	if androidmk, ok := obj.(subAndroidMkProvider); ok {
		if !p.subAndroidMkOnce[androidmk] {
			p.subAndroidMkOnce[androidmk] = true
			androidmk.AndroidMk(p, data)
		}
	}
}

func (p *Module) AndroidMk() android.AndroidMkData {
	ret := android.AndroidMkData{OutputFile: p.installSource}

	p.subAndroidMk(&ret, p.installer)

	return ret
}

func (p *binaryDecorator) AndroidMk(base *Module, ret *android.AndroidMkData) {
	ret.Class = "EXECUTABLES"

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		if len(p.binaryProperties.Test_suites) > 0 {
			fmt.Fprintln(w, "LOCAL_COMPATIBILITY_SUITE :=",
				strings.Join(p.binaryProperties.Test_suites, " "))
		}
	})
	base.subAndroidMk(ret, p.pythonInstaller)
}

func (p *testDecorator) AndroidMk(base *Module, ret *android.AndroidMkData) {
	ret.Class = "NATIVE_TESTS"

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		if len(p.binaryDecorator.binaryProperties.Test_suites) > 0 {
			fmt.Fprintln(w, "LOCAL_COMPATIBILITY_SUITE :=",
				strings.Join(p.binaryDecorator.binaryProperties.Test_suites, " "))
		}
		// If the test config has an explicit config specified use it.
		if p.testProperties.Test_config != nil {
			fmt.Fprintln(w, "LOCAL_TEST_CONFIG :=",
				*p.testProperties.Test_config)
		} else {
			if p.testConfig != nil {
				fmt.Fprintln(w, "LOCAL_FULL_TEST_CONFIG :=",
					p.testConfig.String())
			}
		}

		if !BoolDefault(p.binaryProperties.Auto_gen_config, true) {
			fmt.Fprintln(w, "LOCAL_DISABLE_AUTO_GENERATE_TEST_CONFIG := true")
		}
	})
	base.subAndroidMk(ret, p.binaryDecorator.pythonInstaller)
}

func (installer *pythonInstaller) AndroidMk(base *Module, ret *android.AndroidMkData) {
	// Soong installation is only supported for host modules. Have Make
	// installation trigger Soong installation.
	if base.Target().Os.Class == android.Host {
		ret.OutputFile = android.OptionalPathForPath(installer.path)
	}

	ret.Required = append(ret.Required, "libc++")
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) {
		path, file := filepath.Split(installer.path.ToMakePath().String())
		stem := strings.TrimSuffix(file, filepath.Ext(file))

		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+filepath.Ext(file))
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := "+path)
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
		fmt.Fprintln(w, "LOCAL_SHARED_LIBRARIES := "+strings.Join(installer.androidMkSharedLibs, " "))
		fmt.Fprintln(w, "LOCAL_CHECK_ELF_FILES := false")
	})
}
