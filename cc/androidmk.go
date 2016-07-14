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

type AndroidMkContext interface {
	Target() android.Target
}

func (c *Module) AndroidMk() (ret android.AndroidMkData, err error) {
	if c.Properties.HideFromMake {
		ret.Disabled = true
		return ret, nil
	}

	ret.OutputFile = c.outputFile
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) (err error) {
		fmt.Fprintln(w, "LOCAL_SANITIZE := never")
		if len(c.Properties.AndroidMkSharedLibs) > 0 {
			fmt.Fprintln(w, "LOCAL_SHARED_LIBRARIES := "+strings.Join(c.Properties.AndroidMkSharedLibs, " "))
		}
		if c.Target().Os == android.Android && c.Properties.Sdk_version != "" {
			fmt.Fprintln(w, "LOCAL_SDK_VERSION := "+c.Properties.Sdk_version)
			fmt.Fprintln(w, "LOCAL_NDK_STL_VARIANT := none")
		} else {
			// These are already included in LOCAL_SHARED_LIBRARIES
			fmt.Fprintln(w, "LOCAL_CXX_STL := none")
		}
		return nil
	})

	callSubAndroidMk := func(obj interface{}) {
		if obj != nil {
			if androidmk, ok := obj.(interface {
				AndroidMk(AndroidMkContext, *android.AndroidMkData)
			}); ok {
				androidmk.AndroidMk(c, &ret)
			}
		}
	}

	for _, feature := range c.features {
		callSubAndroidMk(feature)
	}

	callSubAndroidMk(c.compiler)
	callSubAndroidMk(c.linker)
	if c.linker.installable() {
		callSubAndroidMk(c.installer)
	}

	return ret, nil
}

func (library *baseLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	if library.static() {
		ret.Class = "STATIC_LIBRARIES"
	} else {
		ret.Class = "SHARED_LIBRARIES"
	}
}

func (library *libraryLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	library.baseLinker.AndroidMk(ctx, ret)

	if !library.static() {
		library.stripper.AndroidMk(ctx, ret)
	}

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) error {
		var exportedIncludes []string
		for _, flag := range library.exportedFlags() {
			if strings.HasPrefix(flag, "-I") {
				exportedIncludes = append(exportedIncludes, strings.TrimPrefix(flag, "-I"))
			}
		}
		if len(exportedIncludes) > 0 {
			fmt.Fprintln(w, "LOCAL_EXPORT_C_INCLUDE_DIRS :=", strings.Join(exportedIncludes, " "))
		}

		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+outputFile.Ext())

		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")

		return nil
	})
}

func (object *objectLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Custom = func(w io.Writer, name, prefix string) error {
		out := ret.OutputFile.Path()

		fmt.Fprintln(w, "\n$("+prefix+"OUT_INTERMEDIATE_LIBRARIES)/"+name+objectExtension+":", out.String())
		fmt.Fprintln(w, "\t$(copy-file-to-target)")

		return nil
	}
}

func (binary *binaryLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	binary.stripper.AndroidMk(ctx, ret)

	ret.Class = "EXECUTABLES"
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) error {
		fmt.Fprintln(w, "LOCAL_CXX_STL := none")
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")
		return nil
	})
}

func (test *testBinaryLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	test.binaryLinker.AndroidMk(ctx, ret)
	if Bool(test.testLinker.Properties.Test_per_src) {
		ret.SubName = test.binaryLinker.Properties.Stem
	}
}

func (library *toolchainLibraryLinker) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	library.baseLinker.AndroidMk(ctx, ret)

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) error {
		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX := "+outputFile.Ext())
		fmt.Fprintln(w, "LOCAL_CXX_STL := none")
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")

		return nil
	})
}

func (stripper *stripper) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	// Make only supports stripping target modules
	if ctx.Target().Os != android.Android {
		return
	}

	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) error {
		if stripper.StripProperties.Strip.None {
			fmt.Fprintln(w, "LOCAL_STRIP_MODULE := false")
		} else if stripper.StripProperties.Strip.Keep_symbols {
			fmt.Fprintln(w, "LOCAL_STRIP_MODULE := keep_symbols")
		} else {
			fmt.Fprintln(w, "LOCAL_STRIP_MODULE := mini-debug-info")
		}

		return nil
	})
}

func (installer *baseInstaller) AndroidMk(ctx AndroidMkContext, ret *android.AndroidMkData) {
	ret.Extra = append(ret.Extra, func(w io.Writer, outputFile android.Path) error {
		path := installer.path.RelPathString()
		dir, file := filepath.Split(path)
		stem := strings.TrimSuffix(file, filepath.Ext(file))
		fmt.Fprintln(w, "LOCAL_MODULE_PATH := $(OUT_DIR)/"+filepath.Clean(dir))
		fmt.Fprintln(w, "LOCAL_MODULE_STEM := "+stem)
		return nil
	})
}
