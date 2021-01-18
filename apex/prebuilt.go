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
	"strconv"
	"strings"

	"android/soong/android"
	"android/soong/java"
	"github.com/google/blueprint"

	"github.com/google/blueprint/proptools"
)

var (
	extractMatchingApex = pctx.StaticRule(
		"extractMatchingApex",
		blueprint.RuleParams{
			Command: `rm -rf "$out" && ` +
				`${extract_apks} -o "${out}" -allow-prereleased=${allow-prereleased} ` +
				`-sdk-version=${sdk-version} -abis=${abis} -screen-densities=all -extract-single ` +
				`${in}`,
			CommandDeps: []string{"${extract_apks}"},
		},
		"abis", "allow-prereleased", "sdk-version")
)

type prebuilt interface {
	isForceDisabled() bool
	InstallFilename() string
}

type prebuiltCommon struct {
	prebuilt   android.Prebuilt
	properties prebuiltCommonProperties
}

type sanitizedPrebuilt interface {
	hasSanitizedSource(sanitizer string) bool
}

type prebuiltCommonProperties struct {
	ForceDisable bool `blueprint:"mutated"`
}

func (p *prebuiltCommon) Prebuilt() *android.Prebuilt {
	return &p.prebuilt
}

func (p *prebuiltCommon) isForceDisabled() bool {
	return p.properties.ForceDisable
}

func (p *prebuiltCommon) checkForceDisable(ctx android.ModuleContext) bool {
	// If the device is configured to use flattened APEX, force disable the prebuilt because
	// the prebuilt is a non-flattened one.
	forceDisable := ctx.Config().FlattenApex()

	// Force disable the prebuilts when we are doing unbundled build. We do unbundled build
	// to build the prebuilts themselves.
	forceDisable = forceDisable || ctx.Config().UnbundledBuild()

	// Force disable the prebuilts when coverage is enabled.
	forceDisable = forceDisable || ctx.DeviceConfig().NativeCoverageEnabled()
	forceDisable = forceDisable || ctx.Config().IsEnvTrue("EMMA_INSTRUMENT")

	// b/137216042 don't use prebuilts when address sanitizer is on, unless the prebuilt has a sanitized source
	sanitized := ctx.Module().(sanitizedPrebuilt)
	forceDisable = forceDisable || (android.InList("address", ctx.Config().SanitizeDevice()) && !sanitized.hasSanitizedSource("address"))
	forceDisable = forceDisable || (android.InList("hwaddress", ctx.Config().SanitizeDevice()) && !sanitized.hasSanitizedSource("hwaddress"))

	if forceDisable && p.prebuilt.SourceExists() {
		p.properties.ForceDisable = true
		return true
	}
	return false
}

type Prebuilt struct {
	android.ModuleBase
	prebuiltCommon

	properties PrebuiltProperties

	inputApex       android.Path
	installDir      android.InstallPath
	installFilename string
	outputApex      android.WritablePath

	// list of commands to create symlinks for backward compatibility.
	// these commands will be attached as LOCAL_POST_INSTALL_CMD
	compatSymlinks []string
}

type ApexFileProperties struct {
	// the path to the prebuilt .apex file to import.
	Source string `blueprint:"mutated"`

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
}

func (p *ApexFileProperties) selectSource(ctx android.BottomUpMutatorContext) error {
	// This is called before prebuilt_select and prebuilt_postdeps mutators
	// The mutators requires that src to be set correctly for each arch so that
	// arch variants are disabled when src is not provided for the arch.
	if len(ctx.MultiTargets()) != 1 {
		return fmt.Errorf("compile_multilib shouldn't be \"both\" for prebuilt_apex")
	}
	var src string
	switch ctx.MultiTargets()[0].Arch.ArchType {
	case android.Arm:
		src = String(p.Arch.Arm.Src)
	case android.Arm64:
		src = String(p.Arch.Arm64.Src)
	case android.X86:
		src = String(p.Arch.X86.Src)
	case android.X86_64:
		src = String(p.Arch.X86_64.Src)
	default:
		return fmt.Errorf("prebuilt_apex does not support %q", ctx.MultiTargets()[0].Arch.String())
	}
	if src == "" {
		src = String(p.Src)
	}
	p.Source = src

	return nil
}

type PrebuiltProperties struct {
	ApexFileProperties

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

func (a *Prebuilt) hasSanitizedSource(sanitizer string) bool {
	return false
}

func (p *Prebuilt) installable() bool {
	return p.properties.Installable == nil || proptools.Bool(p.properties.Installable)
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

func (p *Prebuilt) Name() string {
	return p.prebuiltCommon.prebuilt.Name(p.ModuleBase.Name())
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
	if err := p.properties.selectSource(ctx); err != nil {
		ctx.ModuleErrorf("%s", err)
		return
	}
}

func (p *Prebuilt) GenerateAndroidBuildActions(ctx android.ModuleContext) {
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

	if p.prebuiltCommon.checkForceDisable(ctx) {
		p.HideFromMake()
		return
	}

	if p.installable() {
		ctx.InstallFile(p.installDir, p.installFilename, p.inputApex)
	}

	// in case that prebuilt_apex replaces source apex (using prefer: prop)
	p.compatSymlinks = makeCompatSymlinks(p.BaseModuleName(), ctx)
	// or that prebuilt_apex overrides other apexes (using overrides: prop)
	for _, overridden := range p.properties.Overrides {
		p.compatSymlinks = append(p.compatSymlinks, makeCompatSymlinks(overridden, ctx)...)
	}
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
				if len(p.compatSymlinks) > 0 {
					entries.SetString("LOCAL_POST_INSTALL_CMD", strings.Join(p.compatSymlinks, " && "))
				}
			},
		},
	}}
}

type ApexSet struct {
	android.ModuleBase
	prebuiltCommon

	properties ApexSetProperties

	installDir      android.InstallPath
	installFilename string
	outputApex      android.WritablePath

	// list of commands to create symlinks for backward compatibility.
	// these commands will be attached as LOCAL_POST_INSTALL_CMD
	compatSymlinks []string
}

type ApexSetProperties struct {
	// the .apks file path that contains prebuilt apex files to be extracted.
	Set *string

	Sanitized struct {
		None struct {
			Set *string
		}
		Address struct {
			Set *string
		}
		Hwaddress struct {
			Set *string
		}
	}

	// whether the extracted apex file installable.
	Installable *bool

	// optional name for the installed apex. If unspecified, name of the
	// module is used as the file name
	Filename *string

	// names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string

	// apexes in this set use prerelease SDK version
	Prerelease *bool
}

func (a *ApexSet) prebuiltSrcs(ctx android.BaseModuleContext) []string {
	var srcs []string
	if a.properties.Set != nil {
		srcs = append(srcs, *a.properties.Set)
	}

	var sanitizers []string
	if ctx.Host() {
		sanitizers = ctx.Config().SanitizeHost()
	} else {
		sanitizers = ctx.Config().SanitizeDevice()
	}

	if android.InList("address", sanitizers) && a.properties.Sanitized.Address.Set != nil {
		srcs = append(srcs, *a.properties.Sanitized.Address.Set)
	} else if android.InList("hwaddress", sanitizers) && a.properties.Sanitized.Hwaddress.Set != nil {
		srcs = append(srcs, *a.properties.Sanitized.Hwaddress.Set)
	} else if a.properties.Sanitized.None.Set != nil {
		srcs = append(srcs, *a.properties.Sanitized.None.Set)
	}

	return srcs
}

func (a *ApexSet) hasSanitizedSource(sanitizer string) bool {
	if sanitizer == "address" {
		return a.properties.Sanitized.Address.Set != nil
	}
	if sanitizer == "hwaddress" {
		return a.properties.Sanitized.Hwaddress.Set != nil
	}

	return false
}

func (a *ApexSet) installable() bool {
	return a.properties.Installable == nil || proptools.Bool(a.properties.Installable)
}

func (a *ApexSet) InstallFilename() string {
	return proptools.StringDefault(a.properties.Filename, a.BaseModuleName()+imageApexSuffix)
}

func (a *ApexSet) Name() string {
	return a.prebuiltCommon.prebuilt.Name(a.ModuleBase.Name())
}

func (a *ApexSet) Overrides() []string {
	return a.properties.Overrides
}

// prebuilt_apex imports an `.apex` file into the build graph as if it was built with apex.
func apexSetFactory() android.Module {
	module := &ApexSet{}
	module.AddProperties(&module.properties)

	srcsSupplier := func(ctx android.BaseModuleContext) []string {
		return module.prebuiltSrcs(ctx)
	}

	android.InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, "set")
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func (a *ApexSet) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	a.installFilename = a.InstallFilename()
	if !strings.HasSuffix(a.installFilename, imageApexSuffix) {
		ctx.ModuleErrorf("filename should end in %s for apex_set", imageApexSuffix)
	}

	apexSet := a.prebuiltCommon.prebuilt.SingleSourcePath(ctx)
	a.outputApex = android.PathForModuleOut(ctx, a.installFilename)
	ctx.Build(pctx,
		android.BuildParams{
			Rule:        extractMatchingApex,
			Description: "Extract an apex from an apex set",
			Inputs:      android.Paths{apexSet},
			Output:      a.outputApex,
			Args: map[string]string{
				"abis":              strings.Join(java.SupportedAbis(ctx), ","),
				"allow-prereleased": strconv.FormatBool(proptools.Bool(a.properties.Prerelease)),
				"sdk-version":       ctx.Config().PlatformSdkVersion().String(),
			},
		})

	if a.prebuiltCommon.checkForceDisable(ctx) {
		a.HideFromMake()
		return
	}

	a.installDir = android.PathForModuleInstall(ctx, "apex")
	if a.installable() {
		ctx.InstallFile(a.installDir, a.installFilename, a.outputApex)
	}

	// in case that apex_set replaces source apex (using prefer: prop)
	a.compatSymlinks = makeCompatSymlinks(a.BaseModuleName(), ctx)
	// or that apex_set overrides other apexes (using overrides: prop)
	for _, overridden := range a.properties.Overrides {
		a.compatSymlinks = append(a.compatSymlinks, makeCompatSymlinks(overridden, ctx)...)
	}
}

func (a *ApexSet) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(a.outputApex),
		Include:    "$(BUILD_PREBUILT)",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", a.installDir.ToMakePath().String())
				entries.SetString("LOCAL_MODULE_STEM", a.installFilename)
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !a.installable())
				entries.AddStrings("LOCAL_OVERRIDES_MODULES", a.properties.Overrides...)
				if len(a.compatSymlinks) > 0 {
					entries.SetString("LOCAL_POST_INSTALL_CMD", strings.Join(a.compatSymlinks, " && "))
				}
			},
		},
	}}
}
