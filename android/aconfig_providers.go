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

package android

import (
	"github.com/google/blueprint"
)

var (
	mergeAconfigFilesRule = pctx.AndroidStaticRule("mergeAconfigFilesRule",
		blueprint.RuleParams{
			Command:     `${aconfig} dump --dedup --format protobuf --out $out $flags`,
			CommandDeps: []string{"${aconfig}"},
		}, "flags")
	_ = pctx.HostBinToolVariable("aconfig", "aconfig")
)

// Provider published by aconfig_value_set
type AconfigDeclarationsProviderData struct {
	Package                     string
	Container                   string
	IntermediateCacheOutputPath WritablePath
	IntermediateDumpOutputPath  WritablePath
}

var AconfigDeclarationsProviderKey = blueprint.NewProvider[AconfigDeclarationsProviderData]()

// This is used to collect the aconfig declarations info on the transitive closure,
// the data is keyed on the container.
type AconfigTransitiveDeclarationsInfo struct {
	AconfigFiles map[string]Paths
}

var AconfigTransitiveDeclarationsInfoProvider = blueprint.NewProvider[AconfigTransitiveDeclarationsInfo]()

func CollectDependencyAconfigFiles(ctx ModuleContext, mergedAconfigFiles *map[string]Paths) {
	if *mergedAconfigFiles == nil {
		*mergedAconfigFiles = make(map[string]Paths)
	}
	ctx.VisitDirectDeps(func(module Module) {
		if dep, _ := OtherModuleProvider(ctx, module, AconfigDeclarationsProviderKey); dep.IntermediateCacheOutputPath != nil {
			(*mergedAconfigFiles)[dep.Container] = append((*mergedAconfigFiles)[dep.Container], dep.IntermediateCacheOutputPath)
			return
		}
		if dep, _ := OtherModuleProvider(ctx, module, AconfigTransitiveDeclarationsInfoProvider); len(dep.AconfigFiles) > 0 {
			for container, v := range dep.AconfigFiles {
				(*mergedAconfigFiles)[container] = append((*mergedAconfigFiles)[container], v...)
			}
		}
	})

	for container, aconfigFiles := range *mergedAconfigFiles {
		(*mergedAconfigFiles)[container] = mergeAconfigFiles(ctx, aconfigFiles)
	}

	SetProvider(ctx, AconfigTransitiveDeclarationsInfoProvider, AconfigTransitiveDeclarationsInfo{
		AconfigFiles: *mergedAconfigFiles,
	})
}

func mergeAconfigFiles(ctx ModuleContext, inputs Paths) Paths {
	inputs = LastUniquePaths(inputs)
	if len(inputs) == 1 {
		return Paths{inputs[0]}
	}

	output := PathForModuleOut(ctx, "aconfig_merged.pb")

	ctx.Build(pctx, BuildParams{
		Rule:        mergeAconfigFilesRule,
		Description: "merge aconfig files",
		Inputs:      inputs,
		Output:      output,
		Args: map[string]string{
			"flags": JoinWithPrefix(inputs.Strings(), "--cache "),
		},
	})

	return Paths{output}
}
