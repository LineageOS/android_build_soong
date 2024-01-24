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

type AfdoProperties struct {
	// Afdo allows developers self-service enroll for
	// automatic feedback-directed optimization using profile data.
	Afdo bool

	FdoProfilePath *string `blueprint:"mutated"`
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

// FdoProfileMutator reads the FdoProfileProvider from a direct dep with FdoProfileTag
// assigns FdoProfileInfo.Path to the FdoProfilePath mutated property
func (c *Module) fdoProfileMutator(ctx android.BottomUpMutatorContext) {
	if !c.Enabled() {
		return
	}

	if !c.afdo.afdoEnabled() {
		return
	}

	if c.Host() {
		return
	}

	if c.static() && !c.staticBinary() {
		return
	}

	if c, ok := ctx.Module().(*Module); ok && c.Enabled() {
		if fdoProfileName, err := ctx.DeviceConfig().AfdoProfile(ctx.ModuleName()); fdoProfileName != "" && err == nil {
			deps := ctx.AddFarVariationDependencies(
				[]blueprint.Variation{
					{Mutator: "arch", Variation: ctx.Target().ArchVariation()},
					{Mutator: "os", Variation: "android"},
				},
				FdoProfileTag,
				fdoProfileName)
			if len(deps) > 0 && deps[0] != nil {
				if info, ok := android.OtherModuleProvider(ctx, deps[0], FdoProfileProvider); ok {
					c.afdo.Properties.FdoProfilePath = proptools.StringPtr(info.Path.String())
				}
			}
		}
	}
}

var _ FdoProfileMutatorInterface = (*Module)(nil)

func afdoPropagateViaDepTag(tag blueprint.DependencyTag) bool {
	libTag, isLibTag := tag.(libraryDependencyTag)
	// Do not recurse down non-static dependencies
	if isLibTag {
		return libTag.static()
	} else {
		return tag == objDepTag || tag == reuseObjTag || tag == staticVariantTag
	}
}

// afdoTransitionMutator creates afdo variants of cc modules.
type afdoTransitionMutator struct{}

func (a *afdoTransitionMutator) Split(ctx android.BaseModuleContext) []string {
	return []string{""}
}

func (a *afdoTransitionMutator) OutgoingTransition(ctx android.OutgoingTransitionContext, sourceVariation string) string {
	if m, ok := ctx.Module().(*Module); ok && m.afdo != nil {
		if !afdoPropagateViaDepTag(ctx.DepTag()) {
			return ""
		}

		if sourceVariation != "" {
			return sourceVariation
		}

		if m.afdo.afdoEnabled() && !(m.static() && !m.staticBinary()) && !m.Host() {
			return encodeTarget(ctx.Module().Name())
		}
	}
	return ""
}

func (a *afdoTransitionMutator) IncomingTransition(ctx android.IncomingTransitionContext, incomingVariation string) string {
	if m, ok := ctx.Module().(*Module); ok && m.afdo != nil {
		return incomingVariation
	}
	return ""
}

func (a *afdoTransitionMutator) Mutate(ctx android.BottomUpMutatorContext, variation string) {
	if variation == "" {
		return
	}

	if m, ok := ctx.Module().(*Module); ok && m.afdo != nil {
		m.Properties.PreventInstall = true
		m.Properties.HideFromMake = true
		m.afdo.Properties.Afdo = true
		if fdoProfileName, err := ctx.DeviceConfig().AfdoProfile(decodeTarget(variation)); fdoProfileName != "" && err == nil {
			deps := ctx.AddFarVariationDependencies(
				[]blueprint.Variation{
					{Mutator: "arch", Variation: ctx.Target().ArchVariation()},
					{Mutator: "os", Variation: "android"},
				},
				FdoProfileTag,
				fdoProfileName)
			if len(deps) > 0 && deps[0] != nil {
				if info, ok := android.OtherModuleProvider(ctx, deps[0], FdoProfileProvider); ok {
					m.afdo.Properties.FdoProfilePath = proptools.StringPtr(info.Path.String())
				}
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
