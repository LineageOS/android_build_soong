// Copyright 2024 Google Inc. All rights reserved.
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

import "github.com/google/blueprint/proptools"

func init() {
	ctx := InitRegistrationContext
	ctx.RegisterModuleType("product_config", productConfigFactory)
}

type productConfigModule struct {
	ModuleBase
}

func (p *productConfigModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	if ctx.ModuleName() != "product_config" || ctx.ModuleDir() != "build/soong" {
		ctx.ModuleErrorf("There can only be one product_config module in build/soong")
		return
	}
	outputFilePath := PathForModuleOut(ctx, p.Name()+".json").OutputPath

	// DeviceProduct can be null so calling ctx.Config().DeviceProduct() may cause null dereference
	targetProduct := proptools.String(ctx.Config().config.productVariables.DeviceProduct)
	if targetProduct != "" {
		targetProduct += "."
	}
	soongVariablesPath := PathForOutput(ctx, "soong."+targetProduct+"variables")
	extraVariablesPath := PathForOutput(ctx, "soong."+targetProduct+"extra.variables")

	rule := NewRuleBuilder(pctx, ctx)
	rule.Command().BuiltTool("merge_json").
		Output(outputFilePath).
		Input(soongVariablesPath).
		Input(extraVariablesPath).
		rule.Build("product_config.json", "building product_config.json")

	ctx.SetOutputFiles(Paths{outputFilePath}, "")
}

// product_config module exports product variables and extra variables as a JSON file.
func productConfigFactory() Module {
	module := &productConfigModule{}
	InitAndroidModule(module)
	return module
}
