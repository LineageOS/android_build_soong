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

// prebuilt_etc is for prebuilts that will be installed to
// <partition>/etc/<subdir>

func init() {
	RegisterModuleType("prebuilt_etc", PrebuiltEtcFactory)

	PreDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("prebuilt_etc", prebuiltEtcMutator).Parallel()
	})
}

type prebuiltEtcProperties struct {
	// Source file of this prebuilt.
	Src *string `android:"arch_variant"`

	// optional subdirectory under which this file is installed into
	Sub_dir *string `android:"arch_variant"`

	// Make this module available when building for recovery.
	Recovery_available *bool

	InRecovery bool `blueprint:"mutated"`
}

type PrebuiltEtc struct {
	ModuleBase

	properties prebuiltEtcProperties

	sourceFilePath         Path
	installDirPath         OutputPath
	additionalDependencies *Paths
}

func (p *PrebuiltEtc) inRecovery() bool {
	return p.properties.InRecovery || p.ModuleBase.InstallInRecovery()
}

func (p *PrebuiltEtc) onlyInRecovery() bool {
	return p.ModuleBase.InstallInRecovery()
}

func (p *PrebuiltEtc) InstallInRecovery() bool {
	return p.inRecovery()
}

func (p *PrebuiltEtc) DepsMutator(ctx BottomUpMutatorContext) {
	if p.properties.Src == nil {
		ctx.PropertyErrorf("src", "missing prebuilt source file")
	}

	// To support ":modulename" in src
	ExtractSourceDeps(ctx, p.properties.Src)
}

func (p *PrebuiltEtc) SourceFilePath(ctx ModuleContext) Path {
	return ctx.ExpandSource(String(p.properties.Src), "src")
}

// This allows other derivative modules (e.g. prebuilt_etc_xml) to perform
// additional steps (like validating the src) before the file is installed.
func (p *PrebuiltEtc) SetAdditionalDependencies(paths Paths) {
	p.additionalDependencies = &paths
}

func (p *PrebuiltEtc) GenerateAndroidBuildActions(ctx ModuleContext) {
	p.sourceFilePath = ctx.ExpandSource(String(p.properties.Src), "src")
	p.installDirPath = PathForModuleInstall(ctx, "etc", String(p.properties.Sub_dir))
}

func (p *PrebuiltEtc) AndroidMk() AndroidMkData {
	return AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data AndroidMkData) {
			nameSuffix := ""
			if p.inRecovery() && !p.onlyInRecovery() {
				nameSuffix = ".recovery"
			}
			fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
			fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
			fmt.Fprintln(w, "LOCAL_MODULE :=", name+nameSuffix)
			fmt.Fprintln(w, "LOCAL_MODULE_CLASS := ETC")
			fmt.Fprintln(w, "LOCAL_MODULE_TAGS := optional")
			fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", p.sourceFilePath.String())
			fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", "$(OUT_DIR)/"+p.installDirPath.RelPathString())
			if p.additionalDependencies != nil {
				fmt.Fprint(w, "LOCAL_ADDITIONAL_DEPENDENCIES :=")
				for _, path := range *p.additionalDependencies {
					fmt.Fprint(w, " "+path.String())
				}
				fmt.Fprintln(w, "")
			}
			fmt.Fprintln(w, "include $(BUILD_PREBUILT)")
		},
	}
}

func InitPrebuiltEtcModule(p *PrebuiltEtc) {
	p.AddProperties(&p.properties)
}

func PrebuiltEtcFactory() Module {
	module := &PrebuiltEtc{}
	InitPrebuiltEtcModule(module)
	// This module is device-only
	InitAndroidArchModule(module, DeviceSupported, MultilibCommon)
	return module
}

const (
	// coreMode is the variant for modules to be installed to system.
	coreMode = "core"

	// recoveryMode means a module to be installed to recovery image.
	recoveryMode = "recovery"
)

// prebuiltEtcMutator creates the needed variants to install the module to
// system or recovery.
func prebuiltEtcMutator(mctx BottomUpMutatorContext) {
	m, ok := mctx.Module().(*PrebuiltEtc)
	if !ok {
		return
	}

	var coreVariantNeeded bool = true
	var recoveryVariantNeeded bool = false
	if Bool(m.properties.Recovery_available) {
		recoveryVariantNeeded = true
	}

	if m.ModuleBase.InstallInRecovery() {
		recoveryVariantNeeded = true
		coreVariantNeeded = false
	}

	var variants []string
	if coreVariantNeeded {
		variants = append(variants, coreMode)
	}
	if recoveryVariantNeeded {
		variants = append(variants, recoveryMode)
	}
	mod := mctx.CreateVariations(variants...)
	for i, v := range variants {
		if v == recoveryMode {
			m := mod[i].(*PrebuiltEtc)
			m.properties.InRecovery = true
		}
	}
}
