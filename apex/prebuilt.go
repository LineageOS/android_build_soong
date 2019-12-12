// Copyright (C) 2019 The Android Open Source Project
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

package apex

import (
	"fmt"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

type Prebuilt struct {
	android.ModuleBase
	prebuilt android.Prebuilt

	properties PrebuiltProperties

	inputApex       android.Path
	installDir      android.InstallPath
	installFilename string
	outputApex      android.WritablePath
}

type PrebuiltProperties struct {
	// the path to the prebuilt .apex file to import.
	Source       string `blueprint:"mutated"`
	ForceDisable bool   `blueprint:"mutated"`

	Src  *string
	Arch struct {
		Arm struct {
			Src *string
		}
		Arm64 struct {
			Src *string
		}
		X86 struct {
			Src *string
		}
		X86_64 struct {
			Src *string
		}
	}

	Installable *bool
	// Optional name for the installed apex. If unspecified, name of the
	// module is used as the file name
	Filename *string

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string
}

func (p *Prebuilt) installable() bool {
	return p.properties.Installable == nil || proptools.Bool(p.properties.Installable)
}

func (p *Prebuilt) isForceDisabled() bool {
	return p.properties.ForceDisable
}

func (p *Prebuilt) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{p.outputApex}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (p *Prebuilt) InstallFilename() string {
	return proptools.StringDefault(p.properties.Filename, p.BaseModuleName()+imageApexSuffix)
}

func (p *Prebuilt) Prebuilt() *android.Prebuilt {
	return &p.prebuilt
}

func (p *Prebuilt) Name() string {
	return p.prebuilt.Name(p.ModuleBase.Name())
}

// prebuilt_apex imports an `.apex` file into the build graph as if it was built with apex.
func PrebuiltFactory() android.Module {
	module := &Prebuilt{}
	module.AddProperties(&module.properties)
	android.InitSingleSourcePrebuiltModule(module, &module.properties, "Source")
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func (p *Prebuilt) DepsMutator(ctx android.BottomUpMutatorContext) {
	// If the device is configured to use flattened APEX, force disable the prebuilt because
	// the prebuilt is a non-flattened one.
	forceDisable := ctx.Config().FlattenApex()

	// Force disable the prebuilts when we are doing unbundled build. We do unbundled build
	// to build the prebuilts themselves.
	forceDisable = forceDisable || ctx.Config().UnbundledBuild()

	// Force disable the prebuilts when coverage is enabled.
	forceDisable = forceDisable || ctx.DeviceConfig().NativeCoverageEnabled()
	forceDisable = forceDisable || ctx.Config().IsEnvTrue("EMMA_INSTRUMENT")

	// b/137216042 don't use prebuilts when address sanitizer is on
	forceDisable = forceDisable || android.InList("address", ctx.Config().SanitizeDevice()) ||
		android.InList("hwaddress", ctx.Config().SanitizeDevice())

	if forceDisable && p.prebuilt.SourceExists() {
		p.properties.ForceDisable = true
		return
	}

	// This is called before prebuilt_select and prebuilt_postdeps mutators
	// The mutators requires that src to be set correctly for each arch so that
	// arch variants are disabled when src is not provided for the arch.
	if len(ctx.MultiTargets()) != 1 {
		ctx.ModuleErrorf("compile_multilib shouldn't be \"both\" for prebuilt_apex")
		return
	}
	var src string
	switch ctx.MultiTargets()[0].Arch.ArchType {
	case android.Arm:
		src = String(p.properties.Arch.Arm.Src)
	case android.Arm64:
		src = String(p.properties.Arch.Arm64.Src)
	case android.X86:
		src = String(p.properties.Arch.X86.Src)
	case android.X86_64:
		src = String(p.properties.Arch.X86_64.Src)
	default:
		ctx.ModuleErrorf("prebuilt_apex does not support %q", ctx.MultiTargets()[0].Arch.String())
		return
	}
	if src == "" {
		src = String(p.properties.Src)
	}
	p.properties.Source = src
}

func (p *Prebuilt) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if p.properties.ForceDisable {
		return
	}

	// TODO(jungjw): Check the key validity.
	p.inputApex = p.Prebuilt().SingleSourcePath(ctx)
	p.installDir = android.PathForModuleInstall(ctx, "apex")
	p.installFilename = p.InstallFilename()
	if !strings.HasSuffix(p.installFilename, imageApexSuffix) {
		ctx.ModuleErrorf("filename should end in %s for prebuilt_apex", imageApexSuffix)
	}
	p.outputApex = android.PathForModuleOut(ctx, p.installFilename)
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Input:  p.inputApex,
		Output: p.outputApex,
	})
	if p.installable() {
		ctx.InstallFile(p.installDir, p.installFilename, p.inputApex)
	}

	// TODO(b/143192278): Add compat symlinks for prebuilt_apex
}

func (p *Prebuilt) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(p.inputApex),
		Include:    "$(BUILD_PREBUILT)",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", p.installDir.ToMakePath().String())
				entries.SetString("LOCAL_MODULE_STEM", p.installFilename)
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !p.installable())
				entries.AddStrings("LOCAL_OVERRIDES_MODULES", p.properties.Overrides...)
			},
		},
	}}
}
