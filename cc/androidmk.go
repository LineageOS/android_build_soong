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
	"io"
	"strings"

	"android/soong/common"
)

func (c *CCLibrary) AndroidMk() (ret common.AndroidMkData) {
	if c.static() {
		ret.Class = "STATIC_LIBRARIES"
	} else {
		ret.Class = "SHARED_LIBRARIES"
	}
	ret.OutputFile = c.outputFile()
	ret.Extra = func(name, prefix string, outputFile common.Path, arch common.Arch) (ret []string) {
		exportedIncludes := c.exportedFlags()
		for i := range exportedIncludes {
			exportedIncludes[i] = strings.TrimPrefix(exportedIncludes[i], "-I")
		}
		if len(exportedIncludes) > 0 {
			ret = append(ret, "LOCAL_EXPORT_C_INCLUDE_DIRS := "+strings.Join(exportedIncludes, " "))
		}

		ret = append(ret, "LOCAL_MODULE_SUFFIX := "+outputFile.Ext())
		ret = append(ret, "LOCAL_SHARED_LIBRARIES_"+arch.ArchType.String()+" := "+strings.Join(c.savedDepNames.SharedLibs, " "))

		if c.Properties.Relative_install_path != "" {
			ret = append(ret, "LOCAL_MODULE_RELATIVE_PATH := "+c.Properties.Relative_install_path)
		}

		// These are already included in LOCAL_SHARED_LIBRARIES
		ret = append(ret, "LOCAL_CXX_STL := none")
		ret = append(ret, "LOCAL_SYSTEM_SHARED_LIBRARIES :=")

		return
	}
	return
}

func (c *ccObject) AndroidMk() (ret common.AndroidMkData) {
	ret.OutputFile = c.outputFile()
	ret.Custom = func(w io.Writer, name, prefix string) {
		out := c.outputFile().Path()

		io.WriteString(w, "$("+prefix+"TARGET_OUT_INTERMEDIATE_LIBRARIES)/"+name+objectExtension+": "+out.String()+" | $(ACP)\n")
		io.WriteString(w, "\t$(copy-file-to-target)\n")
	}
	return
}

func (c *CCBinary) AndroidMk() (ret common.AndroidMkData) {
	ret.Class = "EXECUTABLES"
	ret.Extra = func(name, prefix string, outputFile common.Path, arch common.Arch) []string {
		ret := []string{
			"LOCAL_CXX_STL := none",
			"LOCAL_SYSTEM_SHARED_LIBRARIES :=",
			"LOCAL_SHARED_LIBRARIES_" + arch.ArchType.String() + " += " + strings.Join(c.savedDepNames.SharedLibs, " "),
		}
		if c.Properties.Relative_install_path != "" {
			ret = append(ret, "LOCAL_MODULE_RELATIVE_PATH_"+arch.ArchType.String()+" := "+c.Properties.Relative_install_path)
		}
		return ret
	}
	ret.OutputFile = c.outputFile()
	return
}
