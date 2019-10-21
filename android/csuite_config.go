// Copyright 2019 Google Inc. All rights reserved.
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
)

func init() {
	RegisterModuleType("csuite_config", CSuiteConfigFactory)
}

type csuiteConfigProperties struct {
	// Override the default (AndroidTest.xml) test manifest file name.
	Test_config *string
}

type CSuiteConfig struct {
	ModuleBase
	properties     csuiteConfigProperties
	OutputFilePath OutputPath
}

func (me *CSuiteConfig) GenerateAndroidBuildActions(ctx ModuleContext) {
	me.OutputFilePath = PathForModuleOut(ctx, me.BaseModuleName()).OutputPath
}

func (me *CSuiteConfig) AndroidMk() AndroidMkData {
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
			fmt.Fprintln(w, "LOCAL_COMPATIBILITY_SUITE := csuite")
		},
	}
	return androidMkData
}

func InitCSuiteConfigModule(me *CSuiteConfig) {
	me.AddProperties(&me.properties)
}

// csuite_config generates an App Compatibility Test Suite (C-Suite) configuration file from the
// <test_config> xml file and stores it in a subdirectory of $(HOST_OUT).
func CSuiteConfigFactory() Module {
	module := &CSuiteConfig{}
	InitCSuiteConfigModule(module)
	InitAndroidArchModule(module, HostSupported, MultilibFirst)
	return module
}
