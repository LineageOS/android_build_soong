// Copyright 2020 Google Inc. All rights reserved.
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

package dexpreopt

import (
	"android/soong/android"
)

type dummyToolBinary struct {
	android.ModuleBase
}

func (m *dummyToolBinary) GenerateAndroidBuildActions(ctx android.ModuleContext) {}

func (m *dummyToolBinary) HostToolPath() android.OptionalPath {
	return android.OptionalPathForPath(android.PathForTesting("dex2oat"))
}

func dummyToolBinaryFactory() android.Module {
	module := &dummyToolBinary{}
	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibFirst)
	return module
}

func RegisterToolModulesForTest(ctx *android.TestContext) {
	ctx.RegisterModuleType("dummy_tool_binary", dummyToolBinaryFactory)
}

func BpToolModulesForTest() string {
	return `
		dummy_tool_binary {
			name: "dex2oatd",
		}
	`
}
