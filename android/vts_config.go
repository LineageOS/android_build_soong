// Copyright 2016 Google Inc. All rights reserved.
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

package android

import (
	"fmt"
	"io"
	"strings"
)

func init() {
	RegisterModuleType("vts_config", VtsConfigFactory)
}

type vtsConfigProperties struct {
	// Override the default (AndroidTest.xml) test manifest file name.
	Test_config *string
	// Additional test suites to add the test to.
	Test_suites []string `android:"arch_variant"`
}

type VtsConfig struct {
	ModuleBase
	properties     vtsConfigProperties
	OutputFilePath OutputPath
}

func (me *VtsConfig) GenerateAndroidBuildActions(ctx ModuleContext) {
	me.OutputFilePath = PathForModuleOut(ctx, me.BaseModuleName()).OutputPath
}

func (me *VtsConfig) AndroidMk() AndroidMkData {
	androidMkData := AndroidMkData{
		Class:      "FAKE",
		Include:    "$(BUILD_SYSTEM)/suite_host_config.mk",
		OutputFile: OptionalPathForPath(me.OutputFilePath),
	}
	androidMkData.Extra = []AndroidMkExtraFunc{
		func(w io.Writer, outputFile Path) {
			if me.properties.Test_config != nil {
				fmt.Fprintf(w, "LOCAL_TEST_CONFIG := %s\n",
					*me.properties.Test_config)
			}
			fmt.Fprintf(w, "LOCAL_COMPATIBILITY_SUITE := vts %s\n",
				strings.Join(me.properties.Test_suites, " "))
		},
	}
	return androidMkData
}

func InitVtsConfigModule(me *VtsConfig) {
	me.AddProperties(&me.properties)
}

// vts_config generates a Vendor Test Suite (VTS) configuration file from the
// <test_config> xml file and stores it in a subdirectory of $(HOST_OUT).
func VtsConfigFactory() Module {
	module := &VtsConfig{}
	InitVtsConfigModule(module)
	InitAndroidArchModule(module /*TODO: or HostAndDeviceSupported? */, HostSupported, MultilibFirst)
	return module
}
