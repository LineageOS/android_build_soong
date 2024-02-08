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
	"fmt"
	"io"
	"reflect"

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
	Exportable                  bool
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

// CollectDependencyAconfigFiles is used by some module types to provide finer dependency graphing than
// we can do in ModuleBase.
func CollectDependencyAconfigFiles(ctx ModuleContext, mergedAconfigFiles *map[string]Paths) {
	if *mergedAconfigFiles == nil {
		*mergedAconfigFiles = make(map[string]Paths)
	}
	ctx.VisitDirectDepsIgnoreBlueprint(func(module Module) {
		if dep, _ := OtherModuleProvider(ctx, module, AconfigDeclarationsProviderKey); dep.IntermediateCacheOutputPath != nil {
			(*mergedAconfigFiles)[dep.Container] = append((*mergedAconfigFiles)[dep.Container], dep.IntermediateCacheOutputPath)
			return
		}
		if dep, ok := OtherModuleProvider(ctx, module, aconfigPropagatingProviderKey); ok {
			for container, v := range dep.AconfigFiles {
				(*mergedAconfigFiles)[container] = append((*mergedAconfigFiles)[container], v...)
			}
		}
		// We process these last, so that they determine the final value, eliminating any duplicates that we picked up
		// from UpdateAndroidBuildActions.
		if dep, ok := OtherModuleProvider(ctx, module, AconfigTransitiveDeclarationsInfoProvider); ok {
			for container, v := range dep.AconfigFiles {
				(*mergedAconfigFiles)[container] = append((*mergedAconfigFiles)[container], v...)
			}
		}
	})

	for container, aconfigFiles := range *mergedAconfigFiles {
		(*mergedAconfigFiles)[container] = mergeAconfigFiles(ctx, container, aconfigFiles, false)
	}

	SetProvider(ctx, AconfigTransitiveDeclarationsInfoProvider, AconfigTransitiveDeclarationsInfo{
		AconfigFiles: *mergedAconfigFiles,
	})
}

func SetAconfigFileMkEntries(m *ModuleBase, entries *AndroidMkEntries, aconfigFiles map[string]Paths) {
	setAconfigFileMkEntries(m, entries, aconfigFiles)
}

type aconfigPropagatingDeclarationsInfo struct {
	AconfigFiles map[string]Paths
}

var aconfigPropagatingProviderKey = blueprint.NewProvider[aconfigPropagatingDeclarationsInfo]()

func aconfigUpdateAndroidBuildActions(ctx ModuleContext) {
	mergedAconfigFiles := make(map[string]Paths)
	ctx.VisitDirectDepsIgnoreBlueprint(func(module Module) {
		// If any of our dependencies have aconfig declarations (directly or propagated), then merge those and provide them.
		if dep, ok := OtherModuleProvider(ctx, module, AconfigDeclarationsProviderKey); ok {
			mergedAconfigFiles[dep.Container] = append(mergedAconfigFiles[dep.Container], dep.IntermediateCacheOutputPath)
		}
		if dep, ok := OtherModuleProvider(ctx, module, aconfigPropagatingProviderKey); ok {
			for container, v := range dep.AconfigFiles {
				mergedAconfigFiles[container] = append(mergedAconfigFiles[container], v...)
			}
		}
		if dep, ok := OtherModuleProvider(ctx, module, AconfigTransitiveDeclarationsInfoProvider); ok {
			for container, v := range dep.AconfigFiles {
				mergedAconfigFiles[container] = append(mergedAconfigFiles[container], v...)
			}
		}
	})
	// We only need to set the provider if we have aconfig files.
	if len(mergedAconfigFiles) > 0 {
		for container, aconfigFiles := range mergedAconfigFiles {
			mergedAconfigFiles[container] = mergeAconfigFiles(ctx, container, aconfigFiles, true)
		}

		SetProvider(ctx, aconfigPropagatingProviderKey, aconfigPropagatingDeclarationsInfo{
			AconfigFiles: mergedAconfigFiles,
		})
	}
}

func aconfigUpdateAndroidMkData(ctx fillInEntriesContext, mod Module, data *AndroidMkData) {
	info, ok := SingletonModuleProvider(ctx, mod, aconfigPropagatingProviderKey)
	// If there is no aconfigPropagatingProvider, or there are no AconfigFiles, then we are done.
	if !ok || len(info.AconfigFiles) == 0 {
		return
	}
	data.Extra = append(data.Extra, func(w io.Writer, outputFile Path) {
		AndroidMkEmitAssignList(w, "LOCAL_ACONFIG_FILES", getAconfigFilePaths(mod.base(), info.AconfigFiles).Strings())
	})
	// If there is a Custom writer, it needs to support this provider.
	if data.Custom != nil {
		switch reflect.TypeOf(mod).String() {
		case "*aidl.aidlApi": // writes non-custom before adding .phony
		case "*android_sdk.sdkRepoHost": // doesn't go through base_rules
		case "*apex.apexBundle": // aconfig_file properties written
		case "*bpf.bpf": // properties written (both for module and objs)
		case "*genrule.Module": // writes non-custom before adding .phony
		case "*java.SystemModules": // doesn't go through base_rules
		case "*phony.phony": // properties written
		case "*phony.PhonyRule": // writes phony deps and acts like `.PHONY`
		case "*sysprop.syspropLibrary": // properties written
		default:
			panic(fmt.Errorf("custom make rules do not handle aconfig files for %q (%q) module %q", ctx.ModuleType(mod), reflect.TypeOf(mod), mod))
		}
	}
}

func aconfigUpdateAndroidMkEntries(ctx fillInEntriesContext, mod Module, entries *[]AndroidMkEntries) {
	// If there are no entries, then we can ignore this module, even if it has aconfig files.
	if len(*entries) == 0 {
		return
	}
	info, ok := SingletonModuleProvider(ctx, mod, aconfigPropagatingProviderKey)
	if !ok || len(info.AconfigFiles) == 0 {
		return
	}
	// All of the files in the module potentially depend on the aconfig flag values.
	for idx, _ := range *entries {
		(*entries)[idx].ExtraEntries = append((*entries)[idx].ExtraEntries,
			func(ctx AndroidMkExtraEntriesContext, entries *AndroidMkEntries) {
				setAconfigFileMkEntries(mod.base(), entries, info.AconfigFiles)
			},
		)

	}
}

func mergeAconfigFiles(ctx ModuleContext, container string, inputs Paths, generateRule bool) Paths {
	inputs = SortedUniquePaths(inputs)
	if len(inputs) == 1 {
		return Paths{inputs[0]}
	}

	output := PathForModuleOut(ctx, container, "aconfig_merged.pb")

	if generateRule {
		ctx.Build(pctx, BuildParams{
			Rule:        mergeAconfigFilesRule,
			Description: "merge aconfig files",
			Inputs:      inputs,
			Output:      output,
			Args: map[string]string{
				"flags": JoinWithPrefix(inputs.Strings(), "--cache "),
			},
		})
	}

	return Paths{output}
}

func setAconfigFileMkEntries(m *ModuleBase, entries *AndroidMkEntries, aconfigFiles map[string]Paths) {
	entries.AddPaths("LOCAL_ACONFIG_FILES", getAconfigFilePaths(m, aconfigFiles))
}

func getAconfigFilePaths(m *ModuleBase, aconfigFiles map[string]Paths) (paths Paths) {
	// TODO(b/311155208): The default container here should be system.
	container := "system"

	if m.SocSpecific() {
		container = "vendor"
	} else if m.ProductSpecific() {
		container = "product"
	} else if m.SystemExtSpecific() {
		container = "system_ext"
	}

	paths = append(paths, aconfigFiles[container]...)
	if container == "system" {
		// TODO(b/311155208): Once the default container is system, we can drop this.
		paths = append(paths, aconfigFiles[""]...)
	}
	if container != "system" {
		if len(aconfigFiles[container]) == 0 && len(aconfigFiles[""]) > 0 {
			// TODO(b/308625757): Either we guessed the container wrong, or the flag is misdeclared.
			// For now, just include the system (aka "") container if we get here.
			//fmt.Printf("container_mismatch: module=%v container=%v files=%v\n", m, container, aconfigFiles)
		}
		paths = append(paths, aconfigFiles[""]...)
	}
	return
}
