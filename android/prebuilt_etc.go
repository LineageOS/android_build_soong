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
}

type prebuiltEtcProperties struct {
	// Source file of this prebuilt.
	Srcs []string `android:"arch_variant"`

	// optional subdirectory under which this file is installed into
	Sub_dir *string `android:"arch_variant"`
}

type prebuiltEtc struct {
	ModuleBase
	prebuilt Prebuilt

	properties prebuiltEtcProperties

	sourceFilePath Path
	installDirPath OutputPath
}

func (p *prebuiltEtc) Prebuilt() *Prebuilt {
	return &p.prebuilt
}

func (p *prebuiltEtc) DepsMutator(ctx BottomUpMutatorContext) {
	if len(p.properties.Srcs) == 0 {
		ctx.PropertyErrorf("srcs", "missing prebuilt source file")
	}

	if len(p.properties.Srcs) > 1 {
		ctx.PropertyErrorf("srcs", "multiple prebuilt source files")
	}

	// To support ":modulename" in src
	ExtractSourceDeps(ctx, &(p.properties.Srcs)[0])
}

func (p *prebuiltEtc) GenerateAndroidBuildActions(ctx ModuleContext) {
	p.sourceFilePath = ctx.ExpandSource(p.properties.Srcs[0], "srcs")
	p.installDirPath = PathForModuleInstall(ctx, "etc", String(p.properties.Sub_dir))
}

func (p *prebuiltEtc) AndroidMk() AndroidMkData {
	return AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data AndroidMkData) {
			fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
			fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
			fmt.Fprintln(w, "LOCAL_MODULE :=", name)
			fmt.Fprintln(w, "LOCAL_MODULE_CLASS := ETC")
			fmt.Fprintln(w, "LOCAL_MODULE_TAGS := optional")
			fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", p.sourceFilePath.String())
			fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", "$(OUT_DIR)/"+p.installDirPath.RelPathString())
			fmt.Fprintln(w, "include $(BUILD_PREBUILT)")
		},
	}
}

func PrebuiltEtcFactory() Module {
	module := &prebuiltEtc{}
	module.AddProperties(&module.properties)

	InitPrebuiltModule(module, &(module.properties.Srcs))
	InitAndroidModule(module)
	return module
}
