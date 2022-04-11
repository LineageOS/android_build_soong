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

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

var (
	globalAfdoProfileProjects = []string{
		"vendor/google_data/pgo_profile/sampling/",
		"toolchain/pgo-profiles/sampling/",
	}
)

var afdoProfileProjectsConfigKey = android.NewOnceKey("AfdoProfileProjects")

const afdoCFlagsFormat = "-funique-internal-linkage-names -fprofile-sample-accurate -fprofile-sample-use=%s"

func getAfdoProfileProjects(config android.DeviceConfig) []string {
	return config.OnceStringSlice(afdoProfileProjectsConfigKey, func() []string {
		return append(globalAfdoProfileProjects, config.AfdoAdditionalProfileDirs()...)
	})
}

func recordMissingAfdoProfileFile(ctx android.BaseModuleContext, missing string) {
	getNamedMapForConfig(ctx.Config(), modulesMissingProfileFileKey).Store(missing, true)
}

type AfdoProperties struct {
	// Afdo allows developers self-service enroll for
	// automatic feedback-directed optimization using profile data.
	Afdo bool

	AfdoTarget *string  `blueprint:"mutated"`
	AfdoDeps   []string `blueprint:"mutated"`
}

type afdo struct {
	Properties AfdoProperties
}

func (afdo *afdo) props() []interface{} {
	return []interface{}{&afdo.Properties}
}

func (afdo *afdo) AfdoEnabled() bool {
	return afdo != nil && afdo.Properties.Afdo && afdo.Properties.AfdoTarget != nil
}

// Get list of profile file names, ordered by level of specialisation. For example:
//   1. libfoo_arm64.afdo
//   2. libfoo.afdo
// Add more specialisation as needed.
func getProfileFiles(ctx android.BaseModuleContext, moduleName string) []string {
	var files []string
	files = append(files, moduleName+"_"+ctx.Arch().ArchType.String()+".afdo")
	files = append(files, moduleName+".afdo")
	return files
}

func (props *AfdoProperties) GetAfdoProfileFile(ctx android.BaseModuleContext, module string) android.OptionalPath {
	// Test if the profile_file is present in any of the Afdo profile projects
	for _, profileFile := range getProfileFiles(ctx, module) {
		for _, profileProject := range getAfdoProfileProjects(ctx.DeviceConfig()) {
			path := android.ExistentPathForSource(ctx, profileProject, profileFile)
			if path.Valid() {
				return path
			}
		}
	}

	// Record that this module's profile file is absent
	missing := ctx.ModuleDir() + ":" + module
	recordMissingAfdoProfileFile(ctx, missing)

	return android.OptionalPathForPath(nil)
}

func (afdo *afdo) begin(ctx BaseModuleContext) {
	if ctx.Host() {
		return
	}
	if ctx.static() && !ctx.staticBinary() {
		return
	}
	if afdo.Properties.Afdo {
		module := ctx.ModuleName()
		if afdo.Properties.GetAfdoProfileFile(ctx, module).Valid() {
			afdo.Properties.AfdoTarget = proptools.StringPtr(module)
		}
	}
}

func (afdo *afdo) flags(ctx ModuleContext, flags Flags) Flags {
	if profile := afdo.Properties.AfdoTarget; profile != nil {
		if profileFile := afdo.Properties.GetAfdoProfileFile(ctx, *profile); profileFile.Valid() {
			profileFilePath := profileFile.Path()

			profileUseFlag := fmt.Sprintf(afdoCFlagsFormat, profileFile)
			flags.Local.CFlags = append(flags.Local.CFlags, profileUseFlag)
			flags.Local.LdFlags = append(flags.Local.LdFlags, profileUseFlag)
			flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,-mllvm,-no-warn-sample-unused=true")

			// Update CFlagsDeps and LdFlagsDeps so the module is rebuilt
			// if profileFile gets updated
			flags.CFlagsDeps = append(flags.CFlagsDeps, profileFilePath)
			flags.LdFlagsDeps = append(flags.LdFlagsDeps, profileFilePath)
		}
	}

	return flags
}

// Propagate afdo requirements down from binaries
func afdoDepsMutator(mctx android.TopDownMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.afdo.AfdoEnabled() {
		afdoTarget := *m.afdo.Properties.AfdoTarget
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
				dep.afdo.Properties.AfdoDeps = append(dep.afdo.Properties.AfdoDeps, afdoTarget)
			}

			return true
		})
	}
}

// Create afdo variants for modules that need them
func afdoMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.afdo != nil {
		if m.afdo.AfdoEnabled() && !m.static() {
			afdoTarget := *m.afdo.Properties.AfdoTarget
			mctx.SetDependencyVariation(encodeTarget(afdoTarget))
		}

		variationNames := []string{""}
		afdoDeps := android.FirstUniqueStrings(m.afdo.Properties.AfdoDeps)
		for _, dep := range afdoDeps {
			variationNames = append(variationNames, encodeTarget(dep))
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
				variation.afdo.Properties.AfdoTarget = proptools.StringPtr(decodeTarget(name))
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
