// Copyright 2022 The Android Open Source Project
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

package rust

import (
	"fmt"

	"android/soong/android"
	"android/soong/cc"

	"github.com/google/blueprint"
)

const afdoFlagFormat = "-Zprofile-sample-use=%s"

type afdo struct {
	Properties cc.AfdoProperties
}

func (afdo *afdo) props() []interface{} {
	return []interface{}{&afdo.Properties}
}

func (afdo *afdo) addDep(ctx BaseModuleContext, actx android.BottomUpMutatorContext) {
	// afdo is not supported outside of Android
	if ctx.Host() {
		return
	}

	if mod, ok := ctx.Module().(*Module); ok && mod.Enabled() {
		fdoProfileName, err := actx.DeviceConfig().AfdoProfile(actx.ModuleName())
		if err != nil {
			ctx.ModuleErrorf("%s", err.Error())
		}
		if fdoProfileName != "" {
			actx.AddFarVariationDependencies(
				[]blueprint.Variation{
					{Mutator: "arch", Variation: actx.Target().ArchVariation()},
					{Mutator: "os", Variation: "android"},
				},
				cc.FdoProfileTag,
				[]string{fdoProfileName}...,
			)
		}
	}
}

func (afdo *afdo) flags(ctx android.ModuleContext, flags Flags, deps PathDeps) (Flags, PathDeps) {
	if ctx.Host() {
		return flags, deps
	}

	if !afdo.Properties.Afdo {
		return flags, deps
	}

	ctx.VisitDirectDepsWithTag(cc.FdoProfileTag, func(m android.Module) {
		if info, ok := android.OtherModuleProvider(ctx, m, cc.FdoProfileProvider); ok {
			path := info.Path
			profileUseFlag := fmt.Sprintf(afdoFlagFormat, path.String())
			flags.RustFlags = append(flags.RustFlags, profileUseFlag)

			deps.AfdoProfiles = append(deps.AfdoProfiles, path)
		}
	})

	return flags, deps
}
