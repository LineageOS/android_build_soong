// Copyright 2023 Google Inc. All rights reserved.
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

package aconfig

import (
	"android/soong/android"

	"github.com/google/blueprint"
)

var (
	pctx = android.NewPackageContext("android/soong/aconfig")

	// For aconfig_declarations: Generate cache file
	aconfigRule = pctx.AndroidStaticRule("aconfig",
		blueprint.RuleParams{
			Command: `${aconfig} create-cache` +
				` --package ${package}` +
				` ${declarations}` +
				` ${values}` +
				` ${default-permission}` +
				` --cache ${out}.tmp` +
				` && ( if cmp -s ${out}.tmp ${out} ; then rm ${out}.tmp ; else mv ${out}.tmp ${out} ; fi )`,
			//				` --build-id ${release_version}` +
			CommandDeps: []string{
				"${aconfig}",
			},
			Restat: true,
		}, "release_version", "package", "declarations", "values", "default-permission")

	// For all_aconfig_declarations: Combine all parsed_flags proto files
	AllDeclarationsRule = pctx.AndroidStaticRule("All_aconfig_declarations_dump",
		blueprint.RuleParams{
			Command: `${aconfig} dump --format protobuf --out ${out} ${cache_files}`,
			CommandDeps: []string{
				"${aconfig}",
			},
		}, "cache_files")
)

func init() {
	RegisterBuildComponents(android.InitRegistrationContext)
	pctx.HostBinToolVariable("aconfig", "aconfig")
	pctx.HostBinToolVariable("soong_zip", "soong_zip")
}

func RegisterBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("aconfig_declarations", DeclarationsFactory)
	ctx.RegisterModuleType("aconfig_values", ValuesFactory)
	ctx.RegisterModuleType("aconfig_value_set", ValueSetFactory)
	ctx.RegisterParallelSingletonType("all_aconfig_declarations", AllAconfigDeclarationsFactory)
}
