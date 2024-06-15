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
)

// This flag needs to be in both CFlags and LdFlags to ensure correct symbol ordering
const afdoFlagsFormat = "-fprofile-sample-use=%s -fprofile-sample-accurate"

type AfdoProperties struct {
	// Afdo allows developers self-service enroll for
	// automatic feedback-directed optimization using profile data.
	Afdo bool

	AfdoDep bool `blueprint:"mutated"`
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
// that set afdo prop to True.
func (afdo *afdo) afdoEnabled() bool {
	return afdo != nil && afdo.Properties.Afdo
}

func (afdo *afdo) isAfdoCompile(ctx ModuleContext) bool {
	fdoProfilePath := getFdoProfilePathFromDep(ctx)
	return !ctx.Host() && (afdo.Properties.Afdo || afdo.Properties.AfdoDep) && (fdoProfilePath != "")
}

func getFdoProfilePathFromDep(ctx ModuleContext) string {
	fdoProfileDeps := ctx.GetDirectDepsWithTag(FdoProfileTag)
	if len(fdoProfileDeps) > 0 && fdoProfileDeps[0] != nil {
		if info, ok := android.OtherModuleProvider(ctx, fdoProfileDeps[0], FdoProfileProvider); ok {
			return info.Path.String()
		}
	}
	return ""
}

func (afdo *afdo) flags(ctx ModuleContext, flags Flags) Flags {
	if ctx.Host() {
		return flags
	}

	if afdo.Properties.Afdo || afdo.Properties.AfdoDep {
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
	if fdoProfilePath := getFdoProfilePathFromDep(ctx); fdoProfilePath != "" {
		// The flags are prepended to allow overriding.
		profileUseFlag := fmt.Sprintf(afdoFlagsFormat, fdoProfilePath)
		flags.Local.CFlags = append([]string{profileUseFlag}, flags.Local.CFlags...)
		flags.Local.LdFlags = append([]string{profileUseFlag, "-Wl,-mllvm,-no-warn-sample-unused=true"}, flags.Local.LdFlags...)

		// Update CFlagsDeps and LdFlagsDeps so the module is rebuilt
		// if profileFile gets updated
		pathForSrc := android.PathForSource(ctx, fdoProfilePath)
		flags.CFlagsDeps = append(flags.CFlagsDeps, pathForSrc)
		flags.LdFlagsDeps = append(flags.LdFlagsDeps, pathForSrc)
	}

	return flags
}

func (a *afdo) addDep(ctx android.BottomUpMutatorContext, fdoProfileTarget string) {
	if fdoProfileName, err := ctx.DeviceConfig().AfdoProfile(fdoProfileTarget); fdoProfileName != "" && err == nil {
		ctx.AddFarVariationDependencies(
			[]blueprint.Variation{
				{Mutator: "arch", Variation: ctx.Target().ArchVariation()},
				{Mutator: "os", Variation: "android"},
			},
			FdoProfileTag,
			fdoProfileName)
	}
}

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
	if ctx.Host() {
		return ""
	}

	if m, ok := ctx.Module().(*Module); ok && m.afdo != nil {
		if !afdoPropagateViaDepTag(ctx.DepTag()) {
			return ""
		}

		if sourceVariation != "" {
			return sourceVariation
		}

		if !m.afdo.afdoEnabled() {
			return ""
		}

		// TODO(b/324141705): this is designed to prevent propagating AFDO from static libraries that have afdo: true set, but
		//  it should be m.static() && !m.staticBinary() so that static binaries use AFDO variants of dependencies.
		if m.static() {
			return ""
		}

		return encodeTarget(ctx.Module().Name())
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
	if m, ok := ctx.Module().(*Module); ok && m.afdo != nil {
		if variation == "" {
			// The empty variation is either a module that has enabled AFDO for itself, or the non-AFDO
			// variant of a dependency.
			if m.afdo.afdoEnabled() && !(m.static() && !m.staticBinary()) && !m.Host() {
				m.afdo.addDep(ctx, ctx.ModuleName())
			}
		} else {
			// The non-empty variation is the AFDO variant of a dependency of a module that enabled AFDO
			// for itself.
			m.Properties.PreventInstall = true
			m.Properties.HideFromMake = true
			m.afdo.Properties.AfdoDep = true
			m.afdo.addDep(ctx, decodeTarget(variation))
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
