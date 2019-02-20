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
)

// Implements vts_config module

func init() {
	RegisterModuleType("vts_config", VtsConfigFactory)
}

type vtsConfigProperties struct {
	// Test manifest file name if different from AndroidTest.xml.
	Test_config *string
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
		Include:    "$(BUILD_SYSTEM)/android_vts_host_config.mk",
		OutputFile: OptionalPathForPath(me.OutputFilePath),
	}
	if me.properties.Test_config != nil {
		androidMkData.Extra = []AndroidMkExtraFunc{
			func(w io.Writer, outputFile Path) {
				fmt.Fprintf(w, "LOCAL_TEST_CONFIG := %s\n",
					*me.properties.Test_config)
			},
		}
	}
	return androidMkData
}

func InitVtsConfigModule(me *VtsConfig) {
	me.AddProperties(&me.properties)
}

// Defines VTS configuration.
func VtsConfigFactory() Module {
	module := &VtsConfig{}
	InitVtsConfigModule(module)
	InitAndroidArchModule(module /*TODO: or HostAndDeviceSupported? */, HostSupported, MultilibFirst)
	return module
}
