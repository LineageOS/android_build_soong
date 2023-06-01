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

package device_config

import (
	"android/soong/android"
	"github.com/google/blueprint"
)

var (
	pctx = android.NewPackageContext("android/soong/device_config")

	// For device_config_definitions: Generate cache file
	aconfigRule = pctx.AndroidStaticRule("aconfig",
		blueprint.RuleParams{
			Command: `${aconfig} create-cache` +
				` --package ${namespace}` +
				` --declarations ${in}` +
				` ${values}` +
				` --cache ${out}.tmp` +
				` && ( if cmp -s ${out}.tmp ; then rm ${out}.tmp ; else mv ${out}.tmp ${out} ; fi )`,
			//				` --build-id ${release_version}` +
			CommandDeps: []string{
				"${aconfig}",
			},
			Restat: true,
		}, "release_version", "namespace", "values")

	// For java_device_config_definitions_library: Generate java file
	srcJarRule = pctx.AndroidStaticRule("aconfig_srcjar",
		blueprint.RuleParams{
			Command: `rm -rf ${out}.tmp` +
				` && mkdir -p ${out}.tmp` +
				` && ${aconfig} create-java-lib` +
				`    --cache ${in}` +
				`    --out ${out}.tmp` +
				` && $soong_zip -write_if_changed -jar -o ${out} -C ${out}.tmp -D ${out}.tmp` +
				` && rm -rf ${out}.tmp`,
			CommandDeps: []string{
				"$aconfig",
				"$soong_zip",
			},
			Restat: true,
		})
)

func init() {
	registerBuildComponents(android.InitRegistrationContext)
	pctx.HostBinToolVariable("aconfig", "aconfig")
	pctx.HostBinToolVariable("soong_zip", "soong_zip")
}

func registerBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("device_config_definitions", DefinitionsFactory)
	ctx.RegisterModuleType("device_config_values", ValuesFactory)
	ctx.RegisterModuleType("device_config_value_set", ValueSetFactory)
	ctx.RegisterModuleType("java_device_config_definitions_library", JavaDefinitionsLibraryFactory)
}
