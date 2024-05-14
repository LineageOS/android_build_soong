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
	"maps"
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

type ModeInfo struct {
	Container string
	Mode      string
}
type CodegenInfo struct {
	// AconfigDeclarations is the name of the aconfig_declarations modules that
	// the codegen module is associated with
	AconfigDeclarations []string

	// Paths to the cache files of the associated aconfig_declaration modules
	IntermediateCacheOutputPaths Paths

	// Paths to the srcjar files generated from the java_aconfig_library modules
	Srcjars Paths

	ModeInfos map[string]ModeInfo
}

var CodegenInfoProvider = blueprint.NewProvider[CodegenInfo]()

func propagateModeInfos(ctx ModuleContext, module Module, to, from map[string]ModeInfo) {
	if len(from) > 0 {
		depTag := ctx.OtherModuleDependencyTag(module)
		if tag, ok := depTag.(PropagateAconfigValidationDependencyTag); ok && tag.PropagateAconfigValidation() {
			maps.Copy(to, from)
		}
	}
}

type aconfigPropagatingDeclarationsInfo struct {
	AconfigFiles map[string]Paths
	ModeInfos    map[string]ModeInfo
}

var AconfigPropagatingProviderKey = blueprint.NewProvider[aconfigPropagatingDeclarationsInfo]()

func VerifyAconfigBuildMode(ctx ModuleContext, container string, module blueprint.Module, asError bool) {
	if dep, ok := OtherModuleProvider(ctx, module, AconfigPropagatingProviderKey); ok {
		for k, v := range dep.ModeInfos {
			msg := fmt.Sprintf("%s/%s depends on %s/%s/%s across containers\n",
				module.Name(), container, k, v.Container, v.Mode)
			if v.Container != container && v.Mode != "exported" && v.Mode != "force-read-only" {
				if asError {
					ctx.ModuleErrorf(msg)
				} else {
					fmt.Printf("WARNING: " + msg)
				}
			} else {
				if !asError {
					fmt.Printf("PASSED: " + msg)
				}
			}
		}
	}
}

func aconfigUpdateAndroidBuildActions(ctx ModuleContext) {
	mergedAconfigFiles := make(map[string]Paths)
	mergedModeInfos := make(map[string]ModeInfo)

	ctx.VisitDirectDepsIgnoreBlueprint(func(module Module) {
		if aconfig_dep, ok := OtherModuleProvider(ctx, module, CodegenInfoProvider); ok && len(aconfig_dep.ModeInfos) > 0 {
			maps.Copy(mergedModeInfos, aconfig_dep.ModeInfos)
		}

		// If any of our dependencies have aconfig declarations (directly or propagated), then merge those and provide them.
		if dep, ok := OtherModuleProvider(ctx, module, AconfigDeclarationsProviderKey); ok {
			mergedAconfigFiles[dep.Container] = append(mergedAconfigFiles[dep.Container], dep.IntermediateCacheOutputPath)
		}
		if dep, ok := OtherModuleProvider(ctx, module, AconfigPropagatingProviderKey); ok {
			for container, v := range dep.AconfigFiles {
				mergedAconfigFiles[container] = append(mergedAconfigFiles[container], v...)
			}
			propagateModeInfos(ctx, module, mergedModeInfos, dep.ModeInfos)
		}
	})
	// We only need to set the provider if we have aconfig files.
	if len(mergedAconfigFiles) > 0 {
		for _, container := range SortedKeys(mergedAconfigFiles) {
			aconfigFiles := mergedAconfigFiles[container]
			mergedAconfigFiles[container] = mergeAconfigFiles(ctx, container, aconfigFiles, true)
		}

		SetProvider(ctx, AconfigPropagatingProviderKey, aconfigPropagatingDeclarationsInfo{
			AconfigFiles: mergedAconfigFiles,
			ModeInfos:    mergedModeInfos,
		})
		ctx.Module().base().aconfigFilePaths = getAconfigFilePaths(ctx.Module().base(), mergedAconfigFiles)
	}
}

func aconfigUpdateAndroidMkData(ctx fillInEntriesContext, mod Module, data *AndroidMkData) {
	info, ok := SingletonModuleProvider(ctx, mod, AconfigPropagatingProviderKey)
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
	info, ok := SingletonModuleProvider(ctx, mod, AconfigPropagatingProviderKey)
	if !ok || len(info.AconfigFiles) == 0 {
		return
	}
	// All of the files in the module potentially depend on the aconfig flag values.
	for idx, _ := range *entries {
		(*entries)[idx].ExtraEntries = append((*entries)[idx].ExtraEntries,
			func(ctx AndroidMkExtraEntriesContext, entries *AndroidMkEntries) {
				entries.AddPaths("LOCAL_ACONFIG_FILES", getAconfigFilePaths(mod.base(), info.AconfigFiles))
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
