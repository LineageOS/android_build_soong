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
	"strings"

	"android/soong/common"
)

func (c *CCLibrary) AndroidMk() (ret common.AndroidMkData, err error) {
	if c.static() {
		ret.Class = "STATIC_LIBRARIES"
	} else {
		ret.Class = "SHARED_LIBRARIES"
	}
	ret.OutputFile = c.outputFile()
	ret.Extra = func(w io.Writer, outputFile common.Path) error {
		exportedIncludes := []string{}
		for _, flag := range c.exportedFlags() {
			if flag != "" {
				exportedIncludes = append(exportedIncludes, strings.TrimPrefix(flag, "-I"))
			}
		}
		if len(exportedIncludes) > 0 {
			fmt.Fprintln(w, "LOCAL_EXPORT_C_INCLUDE_DIRS :=", strings.Join(exportedIncludes, " "))
		}

		fmt.Fprintln(w, "LOCAL_MODULE_SUFFIX :=", outputFile.Ext())
		if len(c.savedDepNames.SharedLibs) > 0 {
			fmt.Fprintln(w, "LOCAL_SHARED_LIBRARIES :=", strings.Join(c.savedDepNames.SharedLibs, " "))
		}

		if c.Properties.Relative_install_path != "" {
			fmt.Fprintln(w, "LOCAL_MODULE_RELATIVE_PATH :=", c.Properties.Relative_install_path)
		}

		// These are already included in LOCAL_SHARED_LIBRARIES
		fmt.Fprintln(w, "LOCAL_CXX_STL := none")
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")

		return nil
	}
	return
}

func (c *ccObject) AndroidMk() (ret common.AndroidMkData, err error) {
	ret.OutputFile = c.outputFile()
	ret.Custom = func(w io.Writer, name, prefix string) error {
		out := c.outputFile().Path()

		fmt.Fprintln(w, "\n$("+prefix+"OUT_INTERMEDIATE_LIBRARIES)/"+name+objectExtension+":", out.String(), "| $(ACP)")
		fmt.Fprintln(w, "\t$(copy-file-to-target)")

		return nil
	}
	return
}

func (c *CCBinary) AndroidMk() (ret common.AndroidMkData, err error) {
	ret.Class = "EXECUTABLES"
	ret.Extra = func(w io.Writer, outputFile common.Path) error {
		fmt.Fprintln(w, "LOCAL_CXX_STL := none")
		fmt.Fprintln(w, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")
		fmt.Fprintln(w, "LOCAL_SHARED_LIBRARIES :=", strings.Join(c.savedDepNames.SharedLibs, " "))
		if c.Properties.Relative_install_path != "" {
			fmt.Fprintln(w, "LOCAL_MODULE_RELATIVE_PATH :=", c.Properties.Relative_install_path)
		}

		return nil
	}
	ret.OutputFile = c.outputFile()
	return
}
