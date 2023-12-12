// Copyright 2021 Google Inc. All rights reserved.
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

package cc

import (
	"fmt"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// This flag needs to be in both CFlags and LdFlags to ensure correct symbol ordering
const afdoFlagsFormat = "-fprofile-sample-use=%s -fprofile-sample-accurate"

func recordMissingAfdoProfileFile(ctx android.BaseModuleContext, missing string) {
	getNamedMapForConfig(ctx.Config(), modulesMissingProfileFileKey).Store(missing, true)
}

type afdoRdep struct {
	VariationName *string
	ProfilePath   *string
}

type AfdoProperties struct {
	// Afdo allows developers self-service enroll for
	// automatic feedback-directed optimization using profile data.
	Afdo bool

	FdoProfilePath *string `blueprint:"mutated"`

	AfdoRDeps []afdoRdep `blueprint:"mutated"`
}

type afdo struct {
	Properties AfdoProperties
}

func (afdo *afdo) props() []interface{} {
	return []interface{}{&afdo.Properties}
}

func (afdo *afdo) begin(ctx BaseModuleContext) {
	// Disable on eng builds for faster build.
	if ctx.Config().Eng() {
		afdo.Properties.Afdo = false
	}
}

// afdoEnabled returns true for binaries and shared libraries
// that set afdo prop to True and there is a profile available
func (afdo *afdo) afdoEnabled() bool {
	return afdo != nil && afdo.Properties.Afdo
}

func (afdo *afdo) flags(ctx ModuleContext, flags Flags) Flags {
	if afdo.Properties.Afdo {
		// We use `-funique-internal-linkage-names` to associate profiles to the right internal
		// functions. This option should be used before generating a profile. Because a profile
		// generated for a binary without unique names doesn't work well building a binary with
		// unique names (they have different internal function names).
		// To avoid a chicken-and-egg problem, we enable `-funique-internal-linkage-names` when
		// `afdo=true`, whether a profile exists or not.
		// The profile can take effect in three steps:
		// 1. Add `afdo: true` in Android.bp, and build the binary.
		// 2. Collect an AutoFDO profile for the binary.
		// 3. Make the profile searchable by the build system. So it's used the next time the binary
		//	  is built.
		flags.Local.CFlags = append([]string{"-funique-internal-linkage-names"}, flags.Local.CFlags...)
		// Flags for Flow Sensitive AutoFDO
		flags.Local.CFlags = append([]string{"-mllvm", "-enable-fs-discriminator=true"}, flags.Local.CFlags...)
		// TODO(b/266595187): Remove the following feature once it is enabled in LLVM by default.
		flags.Local.CFlags = append([]string{"-mllvm", "-improved-fs-discriminator=true"}, flags.Local.CFlags...)
	}
	if path := afdo.Properties.FdoProfilePath; path != nil {
		// The flags are prepended to allow overriding.
		profileUseFlag := fmt.Sprintf(afdoFlagsFormat, *path)
		flags.Local.CFlags = append([]string{profileUseFlag}, flags.Local.CFlags...)
		flags.Local.LdFlags = append([]string{profileUseFlag, "-Wl,-mllvm,-no-warn-sample-unused=true"}, flags.Local.LdFlags...)

		// Update CFlagsDeps and LdFlagsDeps so the module is rebuilt
		// if profileFile gets updated
		pathForSrc := android.PathForSource(ctx, *path)
		flags.CFlagsDeps = append(flags.CFlagsDeps, pathForSrc)
		flags.LdFlagsDeps = append(flags.LdFlagsDeps, pathForSrc)
	}

	return flags
}

func (afdo *afdo) addDep(ctx BaseModuleContext, actx android.BottomUpMutatorContext) {
	if ctx.Host() {
		return
	}

	if ctx.static() && !ctx.staticBinary() {
		return
	}

	if c, ok := ctx.Module().(*Module); ok && c.Enabled() {
		if fdoProfileName, err := actx.DeviceConfig().AfdoProfile(actx.ModuleName()); fdoProfileName != nil && err == nil {
			actx.AddFarVariationDependencies(
				[]blueprint.Variation{
					{Mutator: "arch", Variation: actx.Target().ArchVariation()},
					{Mutator: "os", Variation: "android"},
				},
				FdoProfileTag,
				[]string{*fdoProfileName}...,
			)
		}
	}
}

// FdoProfileMutator reads the FdoProfileProvider from a direct dep with FdoProfileTag
// assigns FdoProfileInfo.Path to the FdoProfilePath mutated property
func (c *Module) fdoProfileMutator(ctx android.BottomUpMutatorContext) {
	if !c.Enabled() {
		return
	}

	if !c.afdo.afdoEnabled() {
		return
	}

	ctx.VisitDirectDepsWithTag(FdoProfileTag, func(m android.Module) {
		if ctx.OtherModuleHasProvider(m, FdoProfileProvider) {
			info := ctx.OtherModuleProvider(m, FdoProfileProvider).(FdoProfileInfo)
			c.afdo.Properties.FdoProfilePath = proptools.StringPtr(info.Path.String())
		}
	})
}

var _ FdoProfileMutatorInterface = (*Module)(nil)

// Propagate afdo requirements down from binaries and shared libraries
func afdoDepsMutator(mctx android.TopDownMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.afdo.afdoEnabled() {
		path := m.afdo.Properties.FdoProfilePath
		mctx.WalkDeps(func(dep android.Module, parent android.Module) bool {
			tag := mctx.OtherModuleDependencyTag(dep)
			libTag, isLibTag := tag.(libraryDependencyTag)

			// Do not recurse down non-static dependencies
			if isLibTag {
				if !libTag.static() {
					return false
				}
			} else {
				if tag != objDepTag && tag != reuseObjTag {
					return false
				}
			}

			if dep, ok := dep.(*Module); ok {
				dep.afdo.Properties.AfdoRDeps = append(
					dep.afdo.Properties.AfdoRDeps,
					afdoRdep{
						VariationName: proptools.StringPtr(encodeTarget(m.Name())),
						ProfilePath:   path,
					},
				)
			}

			return true
		})
	}
}

// Create afdo variants for modules that need them
func afdoMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.afdo != nil {
		if !m.static() && m.afdo.Properties.Afdo {
			mctx.SetDependencyVariation(encodeTarget(m.Name()))
			return
		}

		variationNames := []string{""}

		variantNameToProfilePath := make(map[string]*string)

		for _, afdoRDep := range m.afdo.Properties.AfdoRDeps {
			variantName := *afdoRDep.VariationName
			// An rdep can be set twice in AfdoRDeps because there can be
			// more than one path from an afdo-enabled module to
			// a static dep such as
			// afdo_enabled_foo -> static_bar ----> static_baz
			//                   \                      ^
			//                    ----------------------|
			// We only need to create one variant per unique rdep
			if _, exists := variantNameToProfilePath[variantName]; !exists {
				variationNames = append(variationNames, variantName)
				variantNameToProfilePath[variantName] = afdoRDep.ProfilePath
			}
		}

		if len(variationNames) > 1 {
			modules := mctx.CreateVariations(variationNames...)
			for i, name := range variationNames {
				if name == "" {
					continue
				}
				variation := modules[i].(*Module)
				variation.Properties.PreventInstall = true
				variation.Properties.HideFromMake = true
				variation.afdo.Properties.Afdo = true
				variation.afdo.Properties.FdoProfilePath = variantNameToProfilePath[name]
			}
		}
	}
}

// Encode target name to variation name.
func encodeTarget(target string) string {
	if target == "" {
		return ""
	}
	return "afdo-" + target
}

// Decode target name from variation name.
func decodeTarget(variation string) string {
	if variation == "" {
		return ""
	}
	return strings.TrimPrefix(variation, "afdo-")
}
