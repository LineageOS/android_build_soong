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

package build_flags

import (
	"android/soong/android"

	"github.com/google/blueprint"
)

var (
	pctx = android.NewPackageContext("android/soong/aconfig/build_flags")

	// For build_flag_declarations: Generate cache file
	buildFlagRule = pctx.AndroidStaticRule("build-flag-declarations",
		blueprint.RuleParams{
			Command: `${buildFlagDeclarations} ` +
				` ${declarations}` +
				` --format pb` +
				` --output ${out}.tmp` +
				` && ( if cmp -s ${out}.tmp ${out} ; then rm ${out}.tmp ; else mv ${out}.tmp ${out} ; fi )`,
			CommandDeps: []string{
				"${buildFlagDeclarations}",
			},
			Restat: true,
		}, "release_version", "declarations")

	buildFlagTextRule = pctx.AndroidStaticRule("build-flag-declarations-text",
		blueprint.RuleParams{
			Command: `${buildFlagDeclarations} --format=textproto` +
				` --intermediate ${in}` +
				` --format textproto` +
				` --output ${out}.tmp` +
				` && ( if cmp -s ${out}.tmp ${out} ; then rm ${out}.tmp ; else mv ${out}.tmp ${out} ; fi )`,
			CommandDeps: []string{
				"${buildFlagDeclarations}",
			},
			Restat: true,
		})

	allDeclarationsRule = pctx.AndroidStaticRule("all-build-flag-declarations-dump",
		blueprint.RuleParams{
			Command: `${buildFlagDeclarations} ${intermediates} --format pb --output ${out}`,
			CommandDeps: []string{
				"${buildFlagDeclarations}",
			},
		}, "intermediates")

	allDeclarationsRuleTextProto = pctx.AndroidStaticRule("All_build_flag_declarations_dump_textproto",
		blueprint.RuleParams{
			Command: `${buildFlagDeclarations} --intermediate ${in} --format textproto --output ${out}`,
			CommandDeps: []string{
				"${buildFlagDeclarations}",
			},
		})
)

func init() {
	RegisterBuildComponents(android.InitRegistrationContext)
	pctx.HostBinToolVariable("buildFlagDeclarations", "build-flag-declarations")
}

func RegisterBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("build_flag_declarations", DeclarationsFactory)
	ctx.RegisterParallelSingletonType("all_build_flag_declarations", AllBuildFlagDeclarationsFactory)
}
