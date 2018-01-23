// Copyright 2017 Google Inc. All rights reserved.
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
	"android/soong/cc/config"
)

var (
	// Add flags to ignore warnings that profiles are old or missing for
	// some functions
	profileUseOtherFlags = []string{"-Wno-backend-plugin"}

	pgoProfileProjects = []string{
		"toolchain/pgo-profiles",
		"vendor/google_data/pgo-profiles",
	}
)

const profileInstrumentFlag = "-fprofile-generate=/data/local/tmp"
const profileSamplingFlag = "-gline-tables-only"
const profileUseInstrumentFormat = "-fprofile-use=%s"
const profileUseSamplingFormat = "-fprofile-sample-use=%s"

type PgoProperties struct {
	Pgo struct {
		Instrumentation    *bool
		Sampling           *bool
		Profile_file       *string `android:"arch_variant"`
		Benchmarks         []string
		Enable_profile_use *bool `android:"arch_variant"`
		// Additional compiler flags to use when building this module
		// for profiling (either instrumentation or sampling).
		Cflags []string `android:"arch_variant"`
	} `android:"arch_variant"`

	PgoPresent          bool `blueprint:"mutated"`
	ShouldProfileModule bool `blueprint:"mutated"`
}

type pgo struct {
	Properties PgoProperties
}

func (props *PgoProperties) isInstrumentation() bool {
	return props.Pgo.Instrumentation != nil && *props.Pgo.Instrumentation == true
}

func (props *PgoProperties) isSampling() bool {
	return props.Pgo.Sampling != nil && *props.Pgo.Sampling == true
}

func (pgo *pgo) props() []interface{} {
	return []interface{}{&pgo.Properties}
}

func (props *PgoProperties) addProfileGatherFlags(ctx ModuleContext, flags Flags) Flags {
	flags.CFlags = append(flags.CFlags, props.Pgo.Cflags...)

	if props.isInstrumentation() {
		flags.CFlags = append(flags.CFlags, profileInstrumentFlag)
		// The profile runtime is added below in deps().  Add the below
		// flag, which is the only other link-time action performed by
		// the Clang driver during link.
		flags.LdFlags = append(flags.LdFlags, "-u__llvm_profile_runtime")
	}
	if props.isSampling() {
		flags.CFlags = append(flags.CFlags, profileSamplingFlag)
		flags.LdFlags = append(flags.LdFlags, profileSamplingFlag)
	}
	return flags
}

func (props *PgoProperties) getPgoProfileFile(ctx ModuleContext) android.OptionalPath {
	// Test if the profile_file is present in any of the pgoProfileProjects
	for _, profileProject := range pgoProfileProjects {
		path := android.ExistentPathForSource(ctx, "", profileProject, *props.Pgo.Profile_file)
		if path.Valid() {
			return path
		}
	}

	return android.OptionalPathForPath(nil)
}

func (props *PgoProperties) profileUseFlag(ctx ModuleContext, file string) string {
	if props.isInstrumentation() {
		return fmt.Sprintf(profileUseInstrumentFormat, file)
	}
	if props.isSampling() {
		return fmt.Sprintf(profileUseSamplingFormat, file)
	}
	return ""
}

func (props *PgoProperties) profileUseFlags(ctx ModuleContext, file string) []string {
	flags := []string{props.profileUseFlag(ctx, file)}
	flags = append(flags, profileUseOtherFlags...)
	return flags
}

func (props *PgoProperties) addProfileUseFlags(ctx ModuleContext, flags Flags) Flags {
	// Return if 'pgo' property is not present in this module.
	if !props.PgoPresent {
		return flags
	}

	// Skip -fprofile-use if 'enable_profile_use' property is set
	if props.Pgo.Enable_profile_use != nil && *props.Pgo.Enable_profile_use == false {
		return flags
	}

	// If the profile file is found, add flags to use the profile
	if profileFile := props.getPgoProfileFile(ctx); profileFile.Valid() {
		profileFilePath := profileFile.Path()
		profileUseFlags := props.profileUseFlags(ctx, profileFilePath.String())

		flags.CFlags = append(flags.CFlags, profileUseFlags...)
		flags.LdFlags = append(flags.LdFlags, profileUseFlags...)

		// Update CFlagsDeps and LdFlagsDeps so the module is rebuilt
		// if profileFile gets updated
		flags.CFlagsDeps = append(flags.CFlagsDeps, profileFilePath)
		flags.LdFlagsDeps = append(flags.LdFlagsDeps, profileFilePath)
	}
	return flags
}

func (props *PgoProperties) isPGO(ctx BaseModuleContext) bool {
	isInstrumentation := props.isInstrumentation()
	isSampling := props.isSampling()

	profileKindPresent := isInstrumentation || isSampling
	filePresent := props.Pgo.Profile_file != nil
	benchmarksPresent := len(props.Pgo.Benchmarks) > 0

	// If all three properties are absent, PGO is OFF for this module
	if !profileKindPresent && !filePresent && !benchmarksPresent {
		return false
	}

	// If at least one property exists, validate that all properties exist
	if !profileKindPresent || !filePresent || !benchmarksPresent {
		var missing []string
		if !profileKindPresent {
			missing = append(missing, "profile kind (either \"instrumentation\" or \"sampling\" property)")
		}
		if !filePresent {
			missing = append(missing, "profile_file property")
		}
		if !benchmarksPresent {
			missing = append(missing, "non-empty benchmarks property")
		}
		missingProps := strings.Join(missing, ", ")
		ctx.ModuleErrorf("PGO specification is missing properties: " + missingProps)
	}

	// Sampling not supported yet
	if isSampling {
		ctx.PropertyErrorf("pgo.sampling", "\"sampling\" is not supported yet)")
	}

	if isSampling && isInstrumentation {
		ctx.PropertyErrorf("pgo", "Exactly one of \"instrumentation\" and \"sampling\" properties must be set")
	}

	return true
}

func (pgo *pgo) begin(ctx BaseModuleContext) {
	// TODO Evaluate if we need to support PGO for host modules
	if ctx.Host() {
		return
	}

	// Check if PGO is needed for this module
	pgo.Properties.PgoPresent = pgo.Properties.isPGO(ctx)

	if !pgo.Properties.PgoPresent {
		return
	}

	// This module should be instrumented if ANDROID_PGO_INSTRUMENT is set
	// and includes 'all', 'ALL' or a benchmark listed for this module.
	//
	// TODO Validate that each benchmark instruments at least one module
	pgo.Properties.ShouldProfileModule = false
	pgoBenchmarks := ctx.Config().Getenv("ANDROID_PGO_INSTRUMENT")
	pgoBenchmarksMap := make(map[string]bool)
	for _, b := range strings.Split(pgoBenchmarks, ",") {
		pgoBenchmarksMap[b] = true
	}

	if pgoBenchmarksMap["all"] == true || pgoBenchmarksMap["ALL"] == true {
		pgo.Properties.ShouldProfileModule = true
	} else {
		for _, b := range pgo.Properties.Pgo.Benchmarks {
			if pgoBenchmarksMap[b] == true {
				pgo.Properties.ShouldProfileModule = true
				break
			}
		}
	}
}

func (pgo *pgo) deps(ctx BaseModuleContext, deps Deps) Deps {
	if pgo.Properties.ShouldProfileModule {
		runtimeLibrary := config.ProfileRuntimeLibrary(ctx.toolchain())
		deps.LateStaticLibs = append(deps.LateStaticLibs, runtimeLibrary)
	}
	return deps
}

func (pgo *pgo) flags(ctx ModuleContext, flags Flags) Flags {
	if ctx.Host() {
		return flags
	}

	props := pgo.Properties

	// Add flags to profile this module based on its profile_kind
	if props.ShouldProfileModule {
		return props.addProfileGatherFlags(ctx, flags)
	}

	if !ctx.Config().IsEnvTrue("ANDROID_PGO_NO_PROFILE_USE") {
		return props.addProfileUseFlags(ctx, flags)
	}

	return flags
}
