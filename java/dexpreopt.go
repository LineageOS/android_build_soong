// Copyright 2018 Google Inc. All rights reserved.
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

package java

import (
	"path/filepath"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/dexpreopt"
)

type dexpreopter struct {
	dexpreoptProperties DexpreoptProperties

	installPath     android.OutputPath
	uncompressedDex bool
	isSDKLibrary    bool
	isTest          bool
	isInstallable   bool

	builtInstalled []string
}

type DexpreoptProperties struct {
	Dex_preopt struct {
		// If false, prevent dexpreopting and stripping the dex file from the final jar.  Defaults to
		// true.
		Enabled *bool

		// If true, never strip the dex files from the final jar when dexpreopting.  Defaults to false.
		No_stripping *bool

		// If true, generate an app image (.art file) for this module.
		App_image *bool

		// If true, use a checked-in profile to guide optimization.  Defaults to false unless
		// a matching profile is set or a profile is found in PRODUCT_DEX_PREOPT_PROFILE_DIR
		// that matches the name of this module, in which case it is defaulted to true.
		Profile_guided *bool

		// If set, provides the path to profile relative to the Android.bp file.  If not set,
		// defaults to searching for a file that matches the name of this module in the default
		// profile location set by PRODUCT_DEX_PREOPT_PROFILE_DIR, or empty if not found.
		Profile *string
	}
}

func (d *dexpreopter) dexpreoptDisabled(ctx android.ModuleContext) bool {
	if ctx.Config().DisableDexPreopt(ctx.ModuleName()) {
		return true
	}

	if ctx.Config().UnbundledBuild() {
		return true
	}

	if d.isTest {
		return true
	}

	if !BoolDefault(d.dexpreoptProperties.Dex_preopt.Enabled, true) {
		return true
	}

	if !d.isInstallable {
		return true
	}

	// TODO: contains no java code

	return false
}

func (d *dexpreopter) dexpreopt(ctx android.ModuleContext, dexJarFile android.ModuleOutPath) android.ModuleOutPath {
	if d.dexpreoptDisabled(ctx) {
		return dexJarFile
	}

	globalConfig := ctx.Config().Once("DexpreoptGlobalConfig", func() interface{} {
		if f := ctx.Config().DexpreoptGlobalConfig(); f != "" {
			ctx.AddNinjaFileDeps(f)
			globalConfig, err := dexpreopt.LoadGlobalConfig(f)
			if err != nil {
				panic(err)
			}
			return globalConfig
		}
		return dexpreopt.GlobalConfig{}
	}).(dexpreopt.GlobalConfig)

	var archs []string
	for _, a := range ctx.MultiTargets() {
		archs = append(archs, a.Arch.ArchType.String())
	}
	if len(archs) == 0 {
		// assume this is a java library, dexpreopt for all arches for now
		for _, target := range ctx.Config().Targets[android.Android] {
			archs = append(archs, target.Arch.ArchType.String())
		}
		if inList(ctx.ModuleName(), globalConfig.SystemServerJars) && !d.isSDKLibrary {
			// If the module is not an SDK library and it's a system server jar, only preopt the primary arch.
			archs = archs[:1]
		}
	}
	if ctx.Config().SecondArchIsTranslated() {
		// Only preopt primary arch for translated arch since there is only an image there.
		archs = archs[:1]
	}

	dexLocation := android.InstallPathToOnDevicePath(ctx, d.installPath)

	strippedDexJarFile := android.PathForModuleOut(ctx, "dexpreopt", dexJarFile.Base())

	deps := android.Paths{dexJarFile}

	var profileClassListing android.OptionalPath
	profileIsTextListing := false
	if BoolDefault(d.dexpreoptProperties.Dex_preopt.Profile_guided, true) {
		// If dex_preopt.profile_guided is not set, default it based on the existence of the
		// dexprepot.profile option or the profile class listing.
		if String(d.dexpreoptProperties.Dex_preopt.Profile) != "" {
			profileClassListing = android.OptionalPathForPath(
				android.PathForModuleSrc(ctx, String(d.dexpreoptProperties.Dex_preopt.Profile)))
			profileIsTextListing = true
		} else {
			profileClassListing = android.ExistentPathForSource(ctx,
				ctx.Config().DexPreoptProfileDir(), ctx.ModuleName()+".prof")
		}
	}

	if profileClassListing.Valid() {
		deps = append(deps, profileClassListing.Path())
	}

	dexpreoptConfig := dexpreopt.ModuleConfig{
		Name:                ctx.ModuleName(),
		DexLocation:         dexLocation,
		BuildPath:           android.PathForModuleOut(ctx, "dexpreopt", ctx.ModuleName()+".jar").String(),
		DexPath:             dexJarFile.String(),
		PreferCodeIntegrity: false,
		UncompressedDex:     d.uncompressedDex,
		HasApkLibraries:     false,
		PreoptFlags:         nil,

		ProfileClassListing:  profileClassListing.String(),
		ProfileIsTextListing: profileIsTextListing,

		EnforceUsesLibraries:  false,
		OptionalUsesLibraries: nil,
		UsesLibraries:         nil,
		LibraryPaths:          nil,

		Archs:                  archs,
		DexPreoptImageLocation: "",

		PreoptExtractedApk: false,

		NoCreateAppImage:    !BoolDefault(d.dexpreoptProperties.Dex_preopt.App_image, true),
		ForceCreateAppImage: BoolDefault(d.dexpreoptProperties.Dex_preopt.App_image, false),

		NoStripping:     Bool(d.dexpreoptProperties.Dex_preopt.No_stripping),
		StripInputPath:  dexJarFile.String(),
		StripOutputPath: strippedDexJarFile.String(),
	}

	dexpreoptRule, err := dexpreopt.GenerateDexpreoptRule(globalConfig, dexpreoptConfig)
	if err != nil {
		ctx.ModuleErrorf("error generating dexpreopt rule: %s", err.Error())
		return dexJarFile
	}

	var inputs android.Paths
	for _, input := range dexpreoptRule.Inputs() {
		if input == "" {
			// Tests sometimes have empty configuration values that lead to empty inputs
			continue
		}
		rel, isRel := android.MaybeRel(ctx, android.PathForModuleOut(ctx).String(), input)
		if isRel {
			inputs = append(inputs, android.PathForModuleOut(ctx, rel))
		} else {
			// TODO: use PathForOutput once boot image is moved to where PathForOutput can find it.
			inputs = append(inputs, &bootImagePath{input})
		}
	}

	var outputs android.WritablePaths
	for _, output := range dexpreoptRule.Outputs() {
		rel := android.Rel(ctx, android.PathForModuleOut(ctx).String(), output)
		outputs = append(outputs, android.PathForModuleOut(ctx, rel))
	}

	for _, install := range dexpreoptRule.Installs() {
		d.builtInstalled = append(d.builtInstalled, install.From+":"+install.To)
	}

	if len(dexpreoptRule.Commands()) > 0 {
		ctx.Build(pctx, android.BuildParams{
			Rule: ctx.Rule(pctx, "dexpreopt", blueprint.RuleParams{
				Command:     strings.Join(proptools.NinjaEscape(dexpreoptRule.Commands()), " && "),
				CommandDeps: dexpreoptRule.Tools(),
			}),
			Implicits:   inputs,
			Outputs:     outputs,
			Description: "dexpreopt",
		})
	}

	stripRule, err := dexpreopt.GenerateStripRule(globalConfig, dexpreoptConfig)
	if err != nil {
		ctx.ModuleErrorf("error generating dexpreopt strip rule: %s", err.Error())
		return dexJarFile
	}

	ctx.Build(pctx, android.BuildParams{
		Rule: ctx.Rule(pctx, "dexpreopt_strip", blueprint.RuleParams{
			Command:     strings.Join(proptools.NinjaEscape(stripRule.Commands()), " && "),
			CommandDeps: stripRule.Tools(),
		}),
		Input:       dexJarFile,
		Output:      strippedDexJarFile,
		Description: "dexpreopt strip",
	})

	return strippedDexJarFile
}

type bootImagePath struct {
	path string
}

var _ android.Path = (*bootImagePath)(nil)

func (p *bootImagePath) String() string { return p.path }
func (p *bootImagePath) Ext() string    { return filepath.Ext(p.path) }
func (p *bootImagePath) Base() string   { return filepath.Base(p.path) }
func (p *bootImagePath) Rel() string    { return p.path }
